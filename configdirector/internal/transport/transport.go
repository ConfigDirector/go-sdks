// Package transport connects a client to the ConfigDirector SDK server.
package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

func endpointURL(baseURL string, path string) (string, error) {
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

func isFatalStatus(status int) bool {
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

func discardResponseBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxResponseBodyBytes))
	_ = body.Close()
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
