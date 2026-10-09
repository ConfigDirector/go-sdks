package eventsource

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
)

func readStream(t *testing.T, stream string) []Event {
	t.Helper()
	events, err := readEvents(t, NewReader(strings.NewReader(stream), Options{}))
	require.ErrorIs(t, err, io.EOF)
	return events
}

func readChunkedStream(t *testing.T, chunks ...string) []Event {
	t.Helper()
	readers := make([]io.Reader, 0, len(chunks))
	for _, chunk := range chunks {
		readers = append(readers, strings.NewReader(chunk))
	}

	events, err := readEvents(t, NewReader(io.MultiReader(readers...), Options{}))
	require.ErrorIs(t, err, io.EOF)
	return events
}

func readEvents(t *testing.T, reader *Reader) ([]Event, error) {
	t.Helper()
	var events []Event
	for {
		event, err := reader.Next()
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
}

func dataOf(events []Event) []string {
	data := make([]string, 0, len(events))
	for _, event := range events {
		data = append(data, event.Data)
	}
	return data
}

func openPipe(t *testing.T) (*io.PipeReader, *io.PipeWriter) {
	t.Helper()
	pipeReader, pipeWriter := io.Pipe()
	t.Cleanup(func() { _ = pipeReader.Close() })
	return pipeReader, pipeWriter
}

func dataLineOf(valueLength int) string {
	return "data: " + strings.Repeat("x", valueLength) + "\n"
}

const fullDataLineValueLength = maxLineBytes - len("data: ")

const sixteenFullDataLinesJoinedLength = 16*fullDataLineValueLength + 15

func sixteenFullDataLines() string {
	return strings.Repeat(dataLineOf(fullDataLineValueLength), 16)
}

func readingGoroutines() int {
	buffer := make([]byte, 1<<16)
	for {
		written := runtime.Stack(buffer, true)
		if written < len(buffer) {
			return strings.Count(string(buffer[:written]), "(*Reader).readSource")
		}
		buffer = make([]byte, 2*len(buffer))
	}
}

func TestNextReturnsAnEventOnTheBlankLineAfterData(t *testing.T) {
	events := readStream(t, "data: hello\n\n")

	require.Equal(t, []Event{{Data: "hello"}}, events)
}

func TestNextJoinsMultipleDataLinesWithNewlines(t *testing.T) {
	events := readStream(t, "data: YHOO\ndata: +2\ndata: 10\n\n")

	require.Equal(t, []string{"YHOO\n+2\n10"}, dataOf(events))
}

func TestNextDoesNotDispatchABlankLineWithoutData(t *testing.T) {
	events := readStream(t, "\n\n\n")

	require.Empty(t, events)
}

func TestNextDoesNotDispatchAnEventWithOnlyAnIdAndAType(t *testing.T) {
	events := readStream(t, "id: 1\nevent: test\n\n")

	require.Empty(t, events)
}

func TestNextResetsTheEventTypeAfterDispatching(t *testing.T) {
	events := readStream(t, "event: custom\ndata: first\n\ndata: second\n\n")

	require.Equal(t, []Event{{Type: "custom", Data: "first"}, {Data: "second"}}, events)
}

func TestNextReturnsSeveralEventsFromOneChunkInOrder(t *testing.T) {
	events := readStream(t, "data: one\n\ndata: two\n\ndata: three\n\n")

	require.Equal(t, []string{"one", "two", "three"}, dataOf(events))
}

func TestNextDropsAnEventCutOffByTheEndOfTheStream(t *testing.T) {
	events := readStream(t, "data: complete\n\ndata: cut off")

	require.Equal(t, []string{"complete"}, dataOf(events))
}

func TestNextDropsAnEventEndingOnASingleNewline(t *testing.T) {
	events := readStream(t, "data: hello\n")

	require.Empty(t, events)
}

func TestNextKeepsReturningTheSameErrorAfterTheStreamEnds(t *testing.T) {
	reader := NewReader(strings.NewReader("data: hello\n\n"), Options{})
	_, firstErr := readEvents(t, reader)

	_, err := reader.Next()

	require.ErrorIs(t, firstErr, io.EOF)
	require.ErrorIs(t, err, io.EOF)
}

func TestNextReturnsTheSourceReadErrorAfterTheEventsBeforeIt(t *testing.T) {
	errBroken := errors.New("connection reset")
	source := io.MultiReader(strings.NewReader("data: first\n\n"), iotest.ErrReader(errBroken))

	events, err := readEvents(t, NewReader(source, Options{}))

	require.ErrorIs(t, err, errBroken)
	require.Equal(t, []string{"first"}, dataOf(events))
}

func TestNextStripsASingleLeadingSpaceFromAValue(t *testing.T) {
	events := readStream(t, "data: value\n\n")

	require.Equal(t, []string{"value"}, dataOf(events))
}

func TestNextKeepsASecondLeadingSpaceInAValue(t *testing.T) {
	events := readStream(t, "data:  two spaces\n\n")

	require.Equal(t, []string{" two spaces"}, dataOf(events))
}

func TestNextAcceptsAValueWithoutASpaceAfterTheColon(t *testing.T) {
	events := readStream(t, "data:value\n\n")

	require.Equal(t, []string{"value"}, dataOf(events))
}

func TestNextTreatsAnEmptyDataLineAsANewline(t *testing.T) {
	events := readStream(t, "data:\ndata: second\n\n")

	require.Equal(t, []string{"\nsecond"}, dataOf(events))
}

func TestNextDoesNotDispatchADataFieldWithoutAColon(t *testing.T) {
	events := readStream(t, "data\n\n")

	require.Empty(t, events)
}

func TestNextKeepsColonsInsideAValue(t *testing.T) {
	events := readStream(t, "data: key:value\n\n")

	require.Equal(t, []string{"key:value"}, dataOf(events))
}

func TestNextIgnoresUnknownFields(t *testing.T) {
	events := readStream(t, "unknown: value\ndata: hello\n\n")

	require.Equal(t, []Event{{Data: "hello"}}, events)
}

func TestNextNamesTheEventFromTheEventField(t *testing.T) {
	events := readStream(t, "event: add\ndata: 73857293\n\nevent: remove\ndata: 2153\n\n")

	require.Equal(t, []Event{{Type: "add", Data: "73857293"}, {Type: "remove", Data: "2153"}}, events)
}

func TestNextUsesTheLastEventFieldOfAnEvent(t *testing.T) {
	events := readStream(t, "event: first\nevent: second\ndata: hello\n\n")

	require.Equal(t, "second", events[0].Type)
}

func TestLastEventIDTracksTheIdField(t *testing.T) {
	reader := NewReader(strings.NewReader("id: 123\ndata: hello\n\n"), Options{})

	events, _ := readEvents(t, reader)

	require.Equal(t, "123", events[0].LastEventID)
	require.Equal(t, "123", reader.LastEventID())
}

func TestLastEventIDCarriesToLaterEventsWithoutAnId(t *testing.T) {
	events := readStream(t, "id: 1\ndata: first\n\nid: 2\ndata: second\n\ndata: third\n\n")

	require.Equal(t, []Event{
		{Data: "first", LastEventID: "1"},
		{Data: "second", LastEventID: "2"},
		{Data: "third", LastEventID: "2"},
	}, events)
}

func TestLastEventIDIgnoresAnIdContainingANUL(t *testing.T) {
	reader := NewReader(strings.NewReader("id: 1\ndata: first\n\nid: abc\x00def\ndata: second\n\n"), Options{})

	events, _ := readEvents(t, reader)

	require.Equal(t, "1", events[1].LastEventID)
	require.Equal(t, "1", reader.LastEventID())
}

func TestLastEventIDAcceptsAnEmptyId(t *testing.T) {
	reader := NewReader(strings.NewReader("id: 1\ndata: first\n\nid:\ndata: second\n\n"), Options{})

	events, _ := readEvents(t, reader)

	require.Equal(t, "", events[1].LastEventID)
	require.Equal(t, "", reader.LastEventID())
}

func TestLastEventIDStartsFromTheOptions(t *testing.T) {
	reader := NewReader(strings.NewReader("data: hello\n\n"), Options{LastEventID: "41"})

	events, _ := readEvents(t, reader)

	require.Equal(t, "41", events[0].LastEventID)
	require.Equal(t, "41", reader.LastEventID())
}

func TestLastEventIDUpdatesFromABlockThatDispatchesNoEvent(t *testing.T) {
	reader := NewReader(strings.NewReader("id: 7\n\n"), Options{})

	events, _ := readEvents(t, reader)

	require.Empty(t, events)
	require.Equal(t, "7", reader.LastEventID())
}

func TestRetryDelayReportsANumericRetryField(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: 3000\n\n"), Options{})

	_, _ = readEvents(t, reader)

	delay, known := reader.RetryDelay()
	require.True(t, known)
	require.Equal(t, 3*time.Second, delay)
}

func TestRetryDelayIsUnknownUntilTheServerSendsOne(t *testing.T) {
	reader := NewReader(strings.NewReader("data: hello\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresAUnitSuffix(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: 3000ms\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresADecimal(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: 1.5\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresANegativeNumber(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: -1\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresASecondLeadingSpace(t *testing.T) {
	reader := NewReader(strings.NewReader("retry:  5\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresNonASCIIDigits(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: ٣\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresANumberTooLargeForADuration(t *testing.T) {
	reader := NewReader(strings.NewReader("retry: 99999999999999999999\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestRetryDelayIgnoresAnEmptyValue(t *testing.T) {
	reader := NewReader(strings.NewReader("retry:\n\n"), Options{})

	_, _ = readEvents(t, reader)

	_, known := reader.RetryDelay()
	require.False(t, known)
}

func TestNextDoesNotDispatchAnEventForARetryAlone(t *testing.T) {
	events := readStream(t, "retry: 5000\n\n")

	require.Empty(t, events)
}

func TestNextSkipsCommentLines(t *testing.T) {
	events := readStream(t, ": keepalive\ndata: hello\n: another\n\n")

	require.Equal(t, []Event{{Data: "hello"}}, events)
}

func TestNextReturnsNoEventForACommentOnlyStream(t *testing.T) {
	events := readStream(t, ": ping\n: pong\n\n:\n")

	require.Empty(t, events)
}

func TestNextTreatsCommentsAsKeepalivesThatResetTheStallTimeout(t *testing.T) {
	pipeReader, pipeWriter := openPipe(t)
	reader := NewReader(pipeReader, Options{StallTimeout: 400 * time.Millisecond})
	go func() {
		for range 12 {
			time.Sleep(60 * time.Millisecond)
			_, _ = io.WriteString(pipeWriter, ": keepalive\n")
		}
		_, _ = io.WriteString(pipeWriter, "data: late\n\n")
	}()

	event, err := reader.Next()

	require.NoError(t, err)
	require.Equal(t, "late", event.Data)
}

func TestNextReportsAStallWhenNoBytesArriveWithinTheStallTimeout(t *testing.T) {
	pipeReader, _ := openPipe(t)
	reader := NewReader(pipeReader, Options{StallTimeout: 50 * time.Millisecond})

	_, err := reader.Next()

	require.ErrorIs(t, err, errStreamStalled)
}

func TestNextReturnsAnEventThatArrivedWhileTheCallerWasAway(t *testing.T) {
	pipeReader, pipeWriter := openPipe(t)
	reader := NewReader(pipeReader, Options{StallTimeout: 50 * time.Millisecond})
	go func() { _, _ = io.WriteString(pipeWriter, "data: hello\n\n") }()
	time.Sleep(150 * time.Millisecond)

	event, err := reader.Next()

	require.NoError(t, err)
	require.Equal(t, "hello", event.Data)
}

func TestNextMeasuresTheSilenceFromTheLastArrivalNotFromTheLastCall(t *testing.T) {
	pipeReader, pipeWriter := openPipe(t)
	reader := NewReader(pipeReader, Options{StallTimeout: 200 * time.Millisecond})
	go func() { _, _ = io.WriteString(pipeWriter, "data: hello\n\n") }()
	time.Sleep(150 * time.Millisecond)
	_, err := reader.Next()
	require.NoError(t, err)

	stalledAt := time.Now()
	_, err = reader.Next()

	require.ErrorIs(t, err, errStreamStalled)
	require.Less(t, time.Since(stalledAt), 150*time.Millisecond)
}

func TestNextReleasesTheReadingGoroutineOnceAStalledSourceIsClosed(t *testing.T) {
	pipeReader, _ := openPipe(t)
	reader := NewReader(pipeReader, Options{StallTimeout: 50 * time.Millisecond})
	_, err := reader.Next()

	require.NoError(t, pipeReader.Close())

	require.ErrorIs(t, err, errStreamStalled)
	require.Eventually(t, func() bool { return readingGoroutines() == 0 }, time.Second, 10*time.Millisecond)
}

func TestNextReleasesTheReadingGoroutineAfterAProtocolErrorWithBytesStillPending(t *testing.T) {
	source := strings.NewReader(strings.Repeat("x", 4*maxLineBytes) + "\ndata: after\n\n")
	reader := NewReader(source, Options{})

	_, err := reader.Next()

	require.ErrorIs(t, err, errLineTooLong)
	require.Eventually(t, func() bool { return readingGoroutines() == 0 }, time.Second, 10*time.Millisecond)
}

func TestNextAcceptsCRLFLineEndings(t *testing.T) {
	events := readStream(t, "data: hello\r\ndata: world\r\n\r\n")

	require.Equal(t, []string{"hello\nworld"}, dataOf(events))
}

func TestNextAcceptsCRLineEndings(t *testing.T) {
	events := readStream(t, "data: hello\rdata: world\r\r")

	require.Equal(t, []string{"hello\nworld"}, dataOf(events))
}

func TestNextAcceptsMixedLineEndingsInOneEvent(t *testing.T) {
	events := readStream(t, "data: line1\r\ndata: line2\n\r\n")

	require.Equal(t, []string{"line1\nline2"}, dataOf(events))
}

func TestNextTreatsACRLFSplitAcrossChunksAsOneTerminator(t *testing.T) {
	events := readChunkedStream(t, "data: hello\r", "\ndata: world\r\n\r\n")

	require.Equal(t, []string{"hello\nworld"}, dataOf(events))
}

func TestNextStripsALeadingByteOrderMark(t *testing.T) {
	events := readStream(t, "\xEF\xBB\xBFdata: hello\n\n")

	require.Equal(t, []string{"hello"}, dataOf(events))
}

func TestNextStripsAByteOrderMarkSplitAcrossChunks(t *testing.T) {
	events := readChunkedStream(t, "\xEF", "\xBB\xBFdata: hello\n\n")

	require.Equal(t, []string{"hello"}, dataOf(events))
}

func TestNextKeepsAByteOrderMarkInsideAValue(t *testing.T) {
	events := readStream(t, "data: \xEF\xBB\xBFhello\n\n")

	require.Equal(t, []string{"\xEF\xBB\xBFhello"}, dataOf(events))
}

func TestNextJoinsAFieldSplitAcrossChunks(t *testing.T) {
	events := readChunkedStream(t, "data: hel", "lo\n", "\n")

	require.Equal(t, []string{"hello"}, dataOf(events))
}

func TestNextReadsAStreamDeliveredOneByteAtATime(t *testing.T) {
	source := iotest.OneByteReader(strings.NewReader("id: 1\ndata: hello\r\ndata: world\r\n\r\n"))

	events, err := readEvents(t, NewReader(source, Options{}))

	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, []Event{{Data: "hello\nworld", LastEventID: "1"}}, events)
}

func TestNextRejectsALineOfOneMiBPlusOneByte(t *testing.T) {
	stream := "data: first\n\n" + dataLineOf(fullDataLineValueLength+1) + "\n"

	events, err := readEvents(t, NewReader(strings.NewReader(stream), Options{}))

	require.ErrorIs(t, err, errLineTooLong)
	require.Equal(t, []string{"first"}, dataOf(events))
}

func TestNextAcceptsALineOfExactlyOneMiB(t *testing.T) {
	events := readStream(t, dataLineOf(fullDataLineValueLength)+"\n")

	require.Len(t, events, 1)
	require.Len(t, events[0].Data, fullDataLineValueLength)
}

func TestNextRejectsALineThatNeverTerminates(t *testing.T) {
	source := strings.NewReader(strings.Repeat("x", 2*maxLineBytes))

	_, err := readEvents(t, NewReader(source, Options{}))

	require.ErrorIs(t, err, errLineTooLong)
}

func TestNextRejectsAnEventOfSixteenMiBPlusOneByte(t *testing.T) {
	stream := sixteenFullDataLines() + dataLineOf(maxEventBytes+1-sixteenFullDataLinesJoinedLength-1) + "\n"

	events, err := readEvents(t, NewReader(strings.NewReader(stream), Options{}))

	require.ErrorIs(t, err, errEventTooLarge)
	require.Empty(t, events)
}

func TestNextAcceptsAnEventOfExactlySixteenMiB(t *testing.T) {
	stream := sixteenFullDataLines() + dataLineOf(maxEventBytes-sixteenFullDataLinesJoinedLength-1) + "\n"

	events := readStream(t, stream)

	require.Len(t, events, 1)
	require.Len(t, events[0].Data, maxEventBytes)
}
