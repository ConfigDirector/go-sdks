package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/eventsource"
)

const (
	streamPath    = "server/sse/v1"
	heartbeatPath = "server/heartbeat/v1"

	heartbeatRequestTimeout = 10 * time.Second

	maxBackoffExponent = 9
	quietAttempts      = 5
)

var (
	errConnectTimedOut = errors.New("transport: no response arrived within the connect timeout")
	errStreamEnded     = errors.New("transport: the server closed the stream")
)

// Streaming keeps an event stream from the SDK server open and delivers the data of every event
// it carries. While the stream is open it sends a heartbeat for the session every 90 seconds,
// the interval the protocol fixes, and it treats 45 seconds of silence, three missed server
// keepalives, as a dropped stream. When the stream drops it reconnects after 2^attempt seconds,
// capped at 512, with half of the delay drawn at random so that a fleet spreads out; the attempt
// count starts over once a stream opens. A delay the server names, in a retry field of the
// stream or in the Retry-After of a 429, is used as it is. A connection attempt counts as failed
// when no response, or no end to a refusal's answer, arrives within 30 seconds. The transport
// stops for good, logging it once at error level, on a 4xx status other than 429 and on an
// unusable URL.
type Streaming struct {
	*shutdown
	options      Options
	deliver      func(data string)
	timing       timing
	endpoints    streamEndpoints
	endpointsErr error
}

type streamEndpoints struct {
	stream    string
	heartbeat string
}

// OpenStreaming starts connecting at once and returns. deliver is called on the transport's
// goroutine with the data of every event the stream carries, so one that blocks delays the
// events after it.
func OpenStreaming(options Options, deliver func(data string)) *Streaming {
	return openStreaming(options, deliver, protocolTiming())
}

func openStreaming(options Options, deliver func(data string), timing timing) *Streaming {
	ctx, shutdown := newShutdown(timing.closeTimeout)
	streaming := &Streaming{
		shutdown: shutdown,
		options:  options,
		deliver:  deliver,
		timing:   timing,
	}
	streaming.endpoints, streaming.endpointsErr = streamEndpointsOf(options.BaseURL)
	go streaming.run(ctx)
	return streaming
}

func streamEndpointsOf(baseURL string) (streamEndpoints, error) {
	stream, err := endpointURL(baseURL, streamPath)
	if err != nil {
		return streamEndpoints{}, err
	}
	heartbeat, err := endpointURL(baseURL, heartbeatPath)
	if err != nil {
		return streamEndpoints{}, err
	}
	return streamEndpoints{stream: stream, heartbeat: heartbeat}, nil
}

func (streaming *Streaming) run(ctx context.Context) {
	defer streaming.goroutineEnded()

	attempt := 0
	lastEventID := ""
	for {
		end := streaming.streamOnce(ctx, lastEventID)
		lastEventID = end.lastEventID

		step := nextStepAfter(end, attempt, rand.Float64())
		switch step.kind {
		case nextStepStopClosed:
			return
		case nextStepStopFatal:
			streaming.options.Logger.Error("The stream cannot be opened and will not be retried", slog.Any("cause", end.err))
			return
		}

		attempt = step.attempt
		streaming.options.Logger.Log(ctx, step.logLevel, "Reconnecting to the stream",
			slog.Int("attempt", step.attempt),
			slog.Duration("delay", step.delay),
			slog.Any("cause", end.err),
		)
		if !streaming.timing.wait(ctx, step.delay) {
			return
		}
	}
}

type streamEndKind int

const (
	streamEndClosed streamEndKind = iota
	streamEndURLUnusable
	streamEndUnreachable
	streamEndRefused
	streamEndDropped
)

type streamEnd struct {
	kind                    streamEndKind
	status                  int
	err                     error
	delayNamedByServer      time.Duration
	delayNamedByServerKnown bool
	lastEventID             string
}

type nextStepKind int

const (
	nextStepStopClosed nextStepKind = iota
	nextStepStopFatal
	nextStepReconnect
)

type nextStep struct {
	kind     nextStepKind
	attempt  int
	delay    time.Duration
	logLevel slog.Level
}

func nextStepAfter(end streamEnd, previousAttempt int, random float64) nextStep {
	switch {
	case end.kind == streamEndClosed:
		return nextStep{kind: nextStepStopClosed}
	case end.kind == streamEndURLUnusable, end.kind == streamEndRefused && isFatalStatus(end.status):
		return nextStep{kind: nextStepStopFatal}
	case end.kind == streamEndDropped:
		return reconnectStep(1, end, random)
	default:
		return reconnectStep(previousAttempt+1, end, random)
	}
}

func reconnectStep(attempt int, end streamEnd, random float64) nextStep {
	delay := backoffDelay(attempt, random)
	if end.delayNamedByServerKnown {
		delay = end.delayNamedByServer
	}
	return nextStep{kind: nextStepReconnect, attempt: attempt, delay: delay, logLevel: reconnectLogLevel(attempt)}
}

func reconnectLogLevel(attempt int) slog.Level {
	if attempt > quietAttempts {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func backoffDelay(attempt int, random float64) time.Duration {
	exponent := min(max(attempt, 1), maxBackoffExponent)
	ceiling := time.Duration(1<<exponent) * time.Second
	fixed := ceiling / 2
	return fixed + time.Duration(random*float64(ceiling-fixed))
}

type streamRequestBody struct {
	ServerSDKKey string      `json:"serverSdkKey"`
	SessionID    string      `json:"sessionId"`
	MetaContext  MetaContext `json:"metaContext"`
}

type heartbeatRequestBody struct {
	ServerSDKKey string `json:"serverSdkKey"`
	SessionID    string `json:"sessionId"`
}

func (streaming *Streaming) streamOnce(ctx context.Context, lastEventID string) streamEnd {
	end := streaming.streamOnceUnlessClosed(ctx, lastEventID)
	if ctx.Err() != nil {
		end.kind = streamEndClosed
	}
	return end
}

func (streaming *Streaming) streamOnceUnlessClosed(ctx context.Context, lastEventID string) streamEnd {
	if streaming.endpointsErr != nil {
		return streamEnd{kind: streamEndURLUnusable, err: streaming.endpointsErr}
	}

	sessionID := newSessionID()
	requestCtx, endRequest := context.WithCancelCause(ctx)
	defer endRequest(nil)
	connectTimer := time.AfterFunc(streaming.timing.connectTimeout, func() { endRequest(errConnectTimedOut) })
	defer connectTimer.Stop()

	response, err := streaming.openStream(requestCtx, sessionID, lastEventID)
	if err != nil {
		if errors.Is(context.Cause(requestCtx), errConnectTimedOut) {
			err = errConnectTimedOut
		}
		return streamEnd{kind: streamEndUnreachable, err: err, lastEventID: lastEventID}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return refusedStreamEnd(response, lastEventID)
	}
	connectTimer.Stop()
	return streaming.readStream(ctx, response, sessionID, lastEventID)
}

func (streaming *Streaming) openStream(ctx context.Context, sessionID string, lastEventID string) (*http.Response, error) {
	body := streamRequestBody{ServerSDKKey: streaming.options.SDKKey, SessionID: sessionID, MetaContext: streaming.options.MetaContext}
	request, err := streaming.options.jsonRequest(ctx, streaming.endpoints.stream, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", eventStreamContentType)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	return streaming.options.HTTPClient.Do(request)
}

func refusedStreamEnd(response *http.Response, lastEventID string) streamEnd {
	end := streamEnd{
		kind:        streamEndRefused,
		status:      response.StatusCode,
		err:         refusalError(response.StatusCode, responseDetail(response.Body)),
		lastEventID: lastEventID,
	}
	if response.StatusCode == http.StatusTooManyRequests {
		end.delayNamedByServer, end.delayNamedByServerKnown = retryAfterDelay(response.Header.Get("Retry-After"), time.Now())
	}
	return end
}

func (streaming *Streaming) readStream(ctx context.Context, response *http.Response, sessionID string, lastEventID string) streamEnd {
	logger := streaming.options.Logger
	logger.Debug("The stream is open", slog.String("sessionId", sessionID))

	heartbeatCtx, stopHeartbeats := context.WithCancel(ctx)
	var heartbeats sync.WaitGroup
	heartbeats.Go(func() { streaming.sendHeartbeats(heartbeatCtx, sessionID) })

	reader := eventsource.NewReader(response.Body, eventsource.Options{LastEventID: lastEventID, StallTimeout: streaming.timing.stallTimeout})
	err := streaming.deliverEvents(ctx, reader)

	stopHeartbeats()
	heartbeats.Wait()
	_ = response.Body.Close()
	<-reader.Done()
	logger.Debug("The stream ended", slog.String("sessionId", sessionID), slog.Any("cause", err))

	end := streamEnd{kind: streamEndDropped, err: err, lastEventID: reader.LastEventID()}
	end.delayNamedByServer, end.delayNamedByServerKnown = reader.RetryDelay()
	return end
}

func (streaming *Streaming) deliverEvents(ctx context.Context, reader *eventsource.Reader) error {
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return errStreamEnded
		}
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		streaming.deliver(event.Data)
	}
}

func (streaming *Streaming) sendHeartbeats(ctx context.Context, sessionID string) {
	ticker := time.NewTicker(streaming.timing.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			streaming.sendHeartbeat(ctx, sessionID)
		}
	}
}

func (streaming *Streaming) sendHeartbeat(ctx context.Context, sessionID string) {
	requestCtx, cancel := context.WithTimeout(ctx, heartbeatRequestTimeout)
	defer cancel()

	err := streaming.postHeartbeat(requestCtx, sessionID)
	if err != nil && ctx.Err() == nil {
		streaming.options.Logger.Debug("The heartbeat failed", slog.String("sessionId", sessionID), slog.Any("cause", err))
	}
}

func (streaming *Streaming) postHeartbeat(ctx context.Context, sessionID string) error {
	body := heartbeatRequestBody{ServerSDKKey: streaming.options.SDKKey, SessionID: sessionID}
	request, err := streaming.options.jsonRequest(ctx, streaming.endpoints.heartbeat, body)
	if err != nil {
		return err
	}
	response, err := streaming.options.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	discardResponseBody(response.Body)
	if response.StatusCode != http.StatusNoContent {
		return refusalError(response.StatusCode, "")
	}
	return nil
}
