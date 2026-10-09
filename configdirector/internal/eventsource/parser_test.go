package eventsource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type parseOutcome struct {
	events      []Event
	lastEventID string
	retryDelay  time.Duration
	retryKnown  bool
	err         error
}

func parseInChunksOf(stream []byte, chunkBytes int) parseOutcome {
	var streamParser parser
	var outcome parseOutcome
	for start := 0; start < len(stream) && outcome.err == nil; start += chunkBytes {
		end := min(start+chunkBytes, len(stream))
		outcome.err = streamParser.feed(stream[start:end])
	}

	for {
		event, ok := streamParser.nextEvent()
		if !ok {
			break
		}
		outcome.events = append(outcome.events, event)
	}
	outcome.lastEventID = streamParser.lastEventID
	outcome.retryDelay, outcome.retryKnown = streamParser.retryDelay, streamParser.retryKnown
	return outcome
}

// FuzzParse checks that parsing never panics, never dispatches an event without data, and
// does not depend on how the stream is split into chunks.
func FuzzParse(f *testing.F) {
	f.Add([]byte("data: hello\n\n"))
	f.Add([]byte("id: 1\nevent: update\ndata: a\ndata: b\n\n: keepalive\nretry: 3000\n\n"))
	f.Add([]byte("data: hello\r\ndata: world\r\n\r\n"))
	f.Add([]byte("\xEF\xBB\xBFdata: x\r\r"))
	f.Add([]byte("id: a\x00b\ndata\ndata:\n\nunknown: 1\n"))

	f.Fuzz(func(t *testing.T, stream []byte) {
		wholeStream := parseInChunksOf(stream, max(len(stream), 1))
		byteByByte := parseInChunksOf(stream, 1)

		require.Equal(t, wholeStream, byteByByte)
		for _, event := range wholeStream.events {
			require.NotEmpty(t, event.Data)
		}
	})
}
