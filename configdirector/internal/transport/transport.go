// Package transport connects a client to the ConfigDirector SDK server.
package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxResponseBodyBytes = 32 << 20

const (
	jsonContentType        = "application/json"
	eventStreamContentType = "text/event-stream"
)

// Options configure a transport. The client validates and defaults them before handing them
// over, so every field is set.
type Options struct {
	SDKKey      string
	BaseURL     string
	MetaContext MetaContext
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

// MetaContext identifies the SDK and the application to the SDK server.
type MetaContext struct {
	SDKName    string `json:"sdkName"`
	SDKVersion string `json:"sdkVersion"`
	AppName    string `json:"appName,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
}

func (metaContext MetaContext) userAgent() string {
	return metaContext.SDKName + "/" + metaContext.SDKVersion
}

// PostJSON sends body as JSON to endpoint, reads the answer to its end and closes it so the
// connection can be reused, and returns the answer's status.
func (options Options) PostJSON(ctx context.Context, endpoint string, body any) (int, error) {
	request, err := options.jsonRequest(ctx, endpoint, body)
	if err != nil {
		return 0, err
	}
	response, err := options.HTTPClient.Do(request)
	if err != nil {
		return 0, err
	}
	discardResponseBody(response)
	return response.StatusCode, nil
}

func (options Options) jsonRequest(ctx context.Context, endpoint string, body any) (*http.Request, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", jsonContentType)
	request.Header.Set("User-Agent", options.MetaContext.userAgent())
	return request, nil
}

// EndpointURL joins path onto the SDK server's base URL, a proxy sub-path included, and fails for
// a base URL that cannot be requested.
func EndpointURL(baseURL string, path string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("transport: the connection URL %q is not usable: %w", baseURL, err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("transport: the connection URL %q is not usable: the scheme must be http or https", baseURL)
	}
	return base.JoinPath(path).String(), nil
}

func newSessionID() string {
	var random [16]byte
	_, _ = rand.Read(random[:])
	random[6] = random[6]&0x0f | 0x40
	random[8] = random[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", random[0:4], random[4:6], random[6:8], random[8:10], random[10:16])
}

// IsFatalStatus reports whether a response status means the SDK server will keep refusing the
// request, so that retrying is pointless: a 4xx other than 429.
func IsFatalStatus(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusTooManyRequests
}

func refusalError(status int, detail string) error {
	if detail == "" {
		return fmt.Errorf("transport: the SDK server answered with status %d", status)
	}
	return fmt.Errorf("transport: the SDK server answered with status %d: %s", status, detail)
}

func responseDetail(body io.Reader) string {
	detail, _ := io.ReadAll(io.LimitReader(body, maxResponseBodyBytes))
	return strings.TrimSpace(string(detail))
}

func discardResponseBody(response *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes))
	_ = response.Body.Close()
}

func retryAfterDelay(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil {
		if seconds < 0 || seconds > int64(math.MaxInt64/time.Second) {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	at, err := http.ParseTime(header)
	if err != nil {
		return 0, false
	}
	return max(at.Sub(now), 0), true
}

const (
	heartbeatInterval = 90 * time.Second
	stallTimeout      = 45 * time.Second
	connectTimeout    = 30 * time.Second
	requestTimeout    = 30 * time.Second
	closeTimeout      = 5 * time.Second
)

var errCloseTimedOut = errors.New("transport: the transport did not shut down within the close timeout")

type timing struct {
	heartbeatInterval time.Duration
	stallTimeout      time.Duration
	connectTimeout    time.Duration
	requestTimeout    time.Duration
	closeTimeout      time.Duration
	wait              func(ctx context.Context, delay time.Duration) bool
}

func protocolTiming() timing {
	return timing{
		heartbeatInterval: heartbeatInterval,
		stallTimeout:      stallTimeout,
		connectTimeout:    connectTimeout,
		requestTimeout:    requestTimeout,
		closeTimeout:      closeTimeout,
		wait:              waitFor,
	}
}

func waitFor(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// shutdown ends a transport's goroutine on Close and waits for it. The goroutine runs under the
// context newShutdown returns and calls goroutineEnded when it returns.
type shutdown struct {
	cancel       context.CancelFunc
	done         chan struct{}
	closeTimeout time.Duration
	closeOnce    sync.Once
	closeErr     error
}

func newShutdown(closeTimeout time.Duration) (context.Context, *shutdown) {
	ctx, cancel := context.WithCancel(context.Background())
	return ctx, &shutdown{cancel: cancel, done: make(chan struct{}), closeTimeout: closeTimeout}
}

func (shutdown *shutdown) goroutineEnded() {
	close(shutdown.done)
}

// Close ends the transport's goroutine, with the request, stream or wait it is in, and returns
// once the goroutine has ended, or with an error when that takes longer than the close timeout.
// Calling it again returns the first call's result.
func (shutdown *shutdown) Close() error {
	shutdown.closeOnce.Do(func() {
		shutdown.cancel()
		timer := time.NewTimer(shutdown.closeTimeout)
		defer timer.Stop()
		select {
		case <-shutdown.done:
		case <-timer.C:
			shutdown.closeErr = errCloseTimedOut
		}
	})
	return shutdown.closeErr
}
