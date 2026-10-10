package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const pollingPath = "server/polling/v1"

var (
	errPollTimedOut        = errors.New("transport: no complete answer arrived within the request timeout")
	errResponseBodyTooLong = fmt.Errorf("transport: the response body is longer than %d bytes", maxResponseBodyBytes)
)

// Polling asks the SDK server for config state every polling interval and delivers the body of
// every answer that carries one. The first poll runs at once. Each poll echoes the timestamp of
// the last payload so the server can answer with a delta, or with a 204 when nothing changed. A
// poll that got no complete answer for a reason other than its own deadline, because the
// connection was refused, reset or closed, the host did not resolve, or the connection dropped
// while the body was being read, is sent once more right away; a second such failure is logged
// once at error level and the next interval is waited. An error status, a request that ran past
// its 30 second deadline and a body that is not JSON are logged at error level and not retried
// before the next interval. A 429 waits the longer of its Retry-After and the interval. The
// transport stops for good, logging it once at error level, on a 4xx status other than 429 and on
// an unusable URL.
type Polling struct {
	*shutdown
	options     Options
	interval    time.Duration
	deliver     func(data string)
	timing      timing
	endpoint    string
	endpointErr error
	sessionID   string
}

// OpenPolling sends the first poll at once and returns. interval is the polling interval the
// client has already validated. deliver is called on the transport's goroutine with the body of
// every answer that carries a payload, so one that blocks delays the polls after it.
func OpenPolling(options Options, interval time.Duration, deliver func(data string)) *Polling {
	return openPolling(options, interval, deliver, protocolTiming())
}

func openPolling(options Options, interval time.Duration, deliver func(data string), timing timing) *Polling {
	ctx, shutdown := newShutdown(timing.closeTimeout)
	polling := &Polling{
		shutdown:  shutdown,
		options:   options,
		interval:  interval,
		deliver:   deliver,
		timing:    timing,
		sessionID: newSessionID(),
	}
	polling.endpoint, polling.endpointErr = endpointURL(options.BaseURL, pollingPath)
	go polling.run(ctx)
	return polling
}

func (polling *Polling) run(ctx context.Context) {
	defer polling.goroutineEnded()

	logger := polling.options.Logger
	lastUpdateTimestamp := ""
	retriedAtOnce := false
	for {
		end := polling.pollOnce(ctx, lastUpdateTimestamp)
		lastUpdateTimestamp = end.lastUpdateTimestamp

		step := nextPollAfter(end, retriedAtOnce, polling.interval)
		retriedAtOnce = step.kind == pollStepRetryAtOnce
		switch step.kind {
		case pollStepStopClosed:
			return
		case pollStepStopFatal:
			logger.Error("Polling has stopped and will not be retried", slog.Any("cause", end.err))
			return
		case pollStepRetryAtOnce:
			logger.Debug("The poll got no answer and is sent again at once", slog.Any("cause", end.err))
			continue
		case pollStepWaitAfterFailure:
			logger.Error("The poll failed", slog.Any("cause", end.err), slog.Duration("nextPollIn", step.delay))
		case pollStepWaitForTheNext:
			logger.Debug("The poll is done", slog.Duration("nextPollIn", step.delay))
		}
		if !polling.timing.wait(ctx, step.delay) {
			return
		}
	}
}

type pollEndKind int

const (
	pollEndClosed pollEndKind = iota
	pollEndURLUnusable
	pollEndNetworkFailure
	pollEndTimedOut
	pollEndRefused
	pollEndUnparsable
	pollEndUnchanged
	pollEndDelivered
)

type pollEnd struct {
	kind                    pollEndKind
	status                  int
	err                     error
	delayNamedByServer      time.Duration
	delayNamedByServerKnown bool
	lastUpdateTimestamp     string
}

type pollStepKind int

const (
	pollStepStopClosed pollStepKind = iota
	pollStepStopFatal
	pollStepRetryAtOnce
	pollStepWaitAfterFailure
	pollStepWaitForTheNext
)

type pollStep struct {
	kind  pollStepKind
	delay time.Duration
}

func nextPollAfter(end pollEnd, retriedAtOnce bool, interval time.Duration) pollStep {
	switch {
	case end.kind == pollEndClosed:
		return pollStep{kind: pollStepStopClosed}
	case end.kind == pollEndURLUnusable, end.kind == pollEndRefused && isFatalStatus(end.status):
		return pollStep{kind: pollStepStopFatal}
	case end.kind == pollEndNetworkFailure && !retriedAtOnce:
		return pollStep{kind: pollStepRetryAtOnce}
	case end.kind == pollEndNetworkFailure, end.kind == pollEndTimedOut, end.kind == pollEndRefused, end.kind == pollEndUnparsable:
		return pollStep{kind: pollStepWaitAfterFailure, delay: delayBeforeTheNextPoll(end, interval)}
	default:
		return pollStep{kind: pollStepWaitForTheNext, delay: interval}
	}
}

func delayBeforeTheNextPoll(end pollEnd, interval time.Duration) time.Duration {
	if end.delayNamedByServerKnown {
		return max(interval, end.delayNamedByServer)
	}
	return interval
}

type pollRequestBody struct {
	ServerSDKKey        string      `json:"serverSdkKey"`
	SessionID           string      `json:"sessionId"`
	MetaContext         MetaContext `json:"metaContext"`
	LastUpdateTimestamp string      `json:"lastUpdateTimestamp,omitempty"`
}

type pollPayloadTimestamp struct {
	Timestamp string `json:"timestamp"`
}

func (polling *Polling) pollOnce(ctx context.Context, lastUpdateTimestamp string) pollEnd {
	end := polling.pollOnceUnlessClosed(ctx, lastUpdateTimestamp)
	if ctx.Err() != nil {
		end.kind = pollEndClosed
	}
	return end
}

func (polling *Polling) pollOnceUnlessClosed(ctx context.Context, lastUpdateTimestamp string) pollEnd {
	if polling.endpointErr != nil {
		return pollEnd{kind: pollEndURLUnusable, err: polling.endpointErr}
	}

	requestCtx, endRequest := context.WithTimeout(ctx, polling.timing.requestTimeout)
	defer endRequest()

	response, err := polling.post(requestCtx, lastUpdateTimestamp)
	if err != nil {
		return pollEndAfterRequestError(requestCtx, err, lastUpdateTimestamp)
	}
	defer discardResponseBody(response.Body)

	switch {
	case response.StatusCode == http.StatusNoContent:
		return pollEnd{kind: pollEndUnchanged, lastUpdateTimestamp: lastUpdateTimestamp}
	case response.StatusCode < 200 || response.StatusCode > 299:
		return refusedPollEnd(response, lastUpdateTimestamp)
	}

	body, err := readWholeBody(response.Body)
	if errors.Is(err, errResponseBodyTooLong) {
		return pollEnd{kind: pollEndUnparsable, err: err, lastUpdateTimestamp: lastUpdateTimestamp}
	}
	if err != nil {
		return pollEndAfterRequestError(requestCtx, err, lastUpdateTimestamp)
	}
	var payload pollPayloadTimestamp
	if err := json.Unmarshal(body, &payload); err != nil {
		return pollEnd{kind: pollEndUnparsable, err: fmt.Errorf("transport: the SDK server answered with a body that is not JSON: %w", err), lastUpdateTimestamp: lastUpdateTimestamp}
	}
	if payload.Timestamp != "" {
		lastUpdateTimestamp = payload.Timestamp
	}

	if ctx.Err() != nil {
		return pollEnd{kind: pollEndClosed}
	}
	polling.deliver(string(body))
	return pollEnd{kind: pollEndDelivered, lastUpdateTimestamp: lastUpdateTimestamp}
}

func (polling *Polling) post(ctx context.Context, lastUpdateTimestamp string) (*http.Response, error) {
	body := pollRequestBody{
		ServerSDKKey:        polling.options.SDKKey,
		SessionID:           polling.sessionID,
		MetaContext:         polling.options.MetaContext,
		LastUpdateTimestamp: lastUpdateTimestamp,
	}
	request, err := polling.options.jsonRequest(ctx, polling.endpoint, body)
	if err != nil {
		return nil, err
	}
	return polling.options.HTTPClient.Do(request)
}

func pollEndAfterRequestError(requestCtx context.Context, err error, lastUpdateTimestamp string) pollEnd {
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
		return pollEnd{kind: pollEndTimedOut, err: errPollTimedOut, lastUpdateTimestamp: lastUpdateTimestamp}
	}
	return pollEnd{kind: pollEndNetworkFailure, err: err, lastUpdateTimestamp: lastUpdateTimestamp}
}

func refusedPollEnd(response *http.Response, lastUpdateTimestamp string) pollEnd {
	end := pollEnd{
		kind:                pollEndRefused,
		status:              response.StatusCode,
		err:                 refusalError(response.StatusCode, responseDetail(response.Body)),
		lastUpdateTimestamp: lastUpdateTimestamp,
	}
	if response.StatusCode == http.StatusTooManyRequests {
		end.delayNamedByServer, end.delayNamedByServerKnown = retryAfterDelay(response.Header.Get("Retry-After"), time.Now())
	}
	return end
}

func readWholeBody(body io.Reader) ([]byte, error) {
	whole, err := io.ReadAll(io.LimitReader(body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(whole) > maxResponseBodyBytes {
		return nil, errResponseBodyTooLong
	}
	return whole, nil
}
