package eventsource

import (
	"bytes"
	"time"
)

const (
	maxLineBytes  = 1 << 20
	maxEventBytes = 1 << 24
)

var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

type parser struct {
	pastFirstLine   bool
	line            []byte
	pendingLineFeed bool

	eventType string
	data      []byte
	dataLines int
	events    []Event

	lastEventID string
	retryDelay  time.Duration
	retryKnown  bool
}

func (parser *parser) feed(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}

	start := 0
	if parser.pendingLineFeed {
		parser.pendingLineFeed = false
		if chunk[0] == '\n' {
			start = 1
		}
	}

	for {
		terminatorOffset := bytes.IndexAny(chunk[start:], "\r\n")
		if terminatorOffset < 0 {
			break
		}
		terminator := start + terminatorOffset
		if err := parser.finishLine(chunk[start:terminator]); err != nil {
			return err
		}

		start = terminator + 1
		if chunk[terminator] == '\r' {
			if start == len(chunk) {
				parser.pendingLineFeed = true
			} else if chunk[start] == '\n' {
				start++
			}
		}
	}

	return parser.bufferLine(chunk[start:])
}

func (parser *parser) nextEvent() (Event, bool) {
	if len(parser.events) == 0 {
		return Event{}, false
	}
	event := parser.events[0]
	parser.events = parser.events[1:]
	return event, true
}

func (parser *parser) bufferLine(fragment []byte) error {
	if len(parser.line)+len(fragment) > maxLineBytes {
		return errLineTooLong
	}
	parser.line = append(parser.line, fragment...)
	return nil
}

func (parser *parser) finishLine(fragment []byte) error {
	if err := parser.bufferLine(fragment); err != nil {
		return err
	}
	line := parser.line
	parser.line = parser.line[:0]
	return parser.dispatchLine(line)
}

func (parser *parser) dispatchLine(line []byte) error {
	if !parser.pastFirstLine {
		parser.pastFirstLine = true
		line = bytes.TrimPrefix(line, byteOrderMark)
	}

	if len(line) == 0 {
		parser.dispatchEvent()
		return nil
	}
	if line[0] == ':' {
		return nil
	}

	field, value, _ := bytes.Cut(line, []byte(":"))
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return parser.applyField(field, value)
}

func (parser *parser) applyField(field, value []byte) error {
	switch string(field) {
	case "event":
		parser.eventType = string(value)
	case "data":
		return parser.appendData(value)
	case "id":
		if bytes.IndexByte(value, 0) < 0 {
			parser.lastEventID = string(value)
		}
	case "retry":
		parser.applyRetry(value)
	}
	return nil
}

func (parser *parser) appendData(value []byte) error {
	joinedLength := len(parser.data) + len(value)
	if parser.dataLines > 0 {
		joinedLength++
	}
	if joinedLength > maxEventBytes {
		return errEventTooLarge
	}

	if parser.dataLines > 0 {
		parser.data = append(parser.data, '\n')
	}
	parser.data = append(parser.data, value...)
	parser.dataLines++
	return nil
}

func (parser *parser) applyRetry(value []byte) {
	if len(value) == 0 {
		return
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return
		}
	}

	delay, err := time.ParseDuration(string(value) + "ms")
	if err != nil {
		return
	}
	parser.retryDelay, parser.retryKnown = delay, true
}

func (parser *parser) dispatchEvent() {
	if len(parser.data) > 0 {
		parser.events = append(parser.events, Event{Type: parser.eventType, Data: string(parser.data), LastEventID: parser.lastEventID})
	}
	parser.eventType = ""
	parser.data = nil
	parser.dataLines = 0
}
