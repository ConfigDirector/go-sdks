// Package eventsource reads server-sent events (text/event-stream) from an io.Reader.
package eventsource

import (
	"bytes"
	"errors"
	"io"
	"time"
)

const defaultStallTimeout = 45 * time.Second

const readBufferBytes = 16 << 10

var (
	errLineTooLong   = errors.New("eventsource: a line exceeded 1 MiB without a terminator")
	errEventTooLarge = errors.New("eventsource: an event exceeded 16 MiB of data")
	errStreamStalled = errors.New("eventsource: the stream sent nothing for the stall timeout")
)

// Event is one dispatched server-sent event.
type Event struct {
	// Type is the value of the event field, empty when the event named none.
	Type string
	// Data is the value of the data fields joined with newlines. It is never empty: an event
	// without data is not dispatched.
	Data string
	// LastEventID is the stream's last event id as of this event, which an earlier event may
	// have set.
	LastEventID string
}

// Options configure a Reader. Zero values are defaults.
type Options struct {
	// LastEventID is the last event id a previous connection of the same stream reached, so the
	// reader reports it until the server sets another.
	LastEventID string
	// StallTimeout is how long the stream may stay silent before Next reports a stalled stream.
	// Zero means 45 seconds, three times the SDK server's keepalive interval.
	StallTimeout time.Duration
}

// Reader reads the events of one text/event-stream. It is not safe for concurrent use.
type Reader struct {
	parser       parser
	chunks       chan chunk
	done         chan struct{}
	stallTimeout time.Duration
	stallTimer   *time.Timer
	err          error
}

type chunk struct {
	content   []byte
	err       error
	arrivedAt time.Time
}

// NewReader starts reading source on its own goroutine. That goroutine ends when source
// returns an error or, once Next has returned an error, after its next read; close source to
// release it, which is what ends a stalled stream.
func NewReader(source io.Reader, options Options) *Reader {
	stallTimeout := options.StallTimeout
	if stallTimeout == 0 {
		stallTimeout = defaultStallTimeout
	}

	reader := &Reader{
		parser:       parser{lastEventID: options.LastEventID},
		chunks:       make(chan chunk, 1),
		done:         make(chan struct{}),
		stallTimeout: stallTimeout,
		stallTimer:   time.NewTimer(stallTimeout),
	}
	go reader.readSource(source)
	return reader
}

// Next returns the next event. It returns io.EOF once the stream has ended, the source's own
// error when its read fails, and an error of this package when no bytes arrived for the stall
// timeout, a line exceeded 1 MiB or an event exceeded 16 MiB of data. Events parsed before the
// error are returned first; after that every call returns the same error. An event that was cut
// off by the end of the stream is never returned.
func (reader *Reader) Next() (Event, error) {
	for {
		if event, ok := reader.parser.nextEvent(); ok {
			return event, nil
		}
		if reader.err != nil {
			return Event{}, reader.err
		}

		next, err := reader.nextChunk()
		if err == nil {
			err = reader.parser.feed(next.content)
		}
		if err == nil {
			err = next.err
		}
		if err != nil {
			reader.endStream(err)
		}
	}
}

// LastEventID is the last event id the stream has set, or the one the reader was created with.
func (reader *Reader) LastEventID() string {
	return reader.parser.lastEventID
}

// RetryDelay is the reconnection delay the server last requested through a retry field, and
// whether it requested one.
func (reader *Reader) RetryDelay() (time.Duration, bool) {
	return reader.parser.retryDelay, reader.parser.retryKnown
}

func (reader *Reader) nextChunk() (chunk, error) {
	var next chunk
	select {
	case next = <-reader.chunks:
	default:
		select {
		case next = <-reader.chunks:
		case <-reader.stallTimer.C:
			return chunk{}, errStreamStalled
		}
	}

	remainingSilence := reader.stallTimeout - time.Since(next.arrivedAt)
	reader.stallTimer.Reset(remainingSilence)
	return next, nil
}

func (reader *Reader) endStream(err error) {
	reader.err = err
	reader.stallTimer.Stop()
	close(reader.done)
}

func (reader *Reader) readSource(source io.Reader) {
	buffer := make([]byte, readBufferBytes)
	for {
		count, err := source.Read(buffer)
		if count == 0 && err == nil {
			continue
		}

		select {
		case reader.chunks <- chunk{content: bytes.Clone(buffer[:count]), err: err, arrivedAt: time.Now()}:
		case <-reader.done:
			return
		}
		if err != nil {
			return
		}
	}
}
