package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
)

const maxInlineValueLength = 500

// Value is one side of an evaluation, the in-code default value or the value that came back, in
// the form telemetry reports it: the value spelled out, or the id of a value that is a JSON
// document or too long to spell out. Build one with ValueOf.
type Value struct {
	text     string
	valueID  string
	document bool
}

// ValueOf renders value for a report. A JSON document (asJSON) is rendered as compact JSON, any
// other value as the text the SDK read it from. serverValueID is the id the SDK server sent with
// a served value, if any, and stands in for a value that travels by id.
func ValueOf(value any, asJSON bool, serverValueID string) Value {
	if asJSON && serverValueID != "" {
		return Value{valueID: serverValueID}
	}

	rendered := Value{text: renderedText(value, asJSON), document: asJSON}
	if serverValueID != "" && rendered.travelsByID() {
		return Value{valueID: serverValueID}
	}
	return rendered
}

func (value Value) travelsByID() bool {
	return value.document || isLong(value.text)
}

func (value Value) wire() wireValue {
	switch {
	case value.valueID != "":
		return wireValue{ValueID: value.valueID}
	case value.travelsByID():
		return wireValue{ValueID: ValueIDFor(value.text)}
	default:
		return wireValue{Value: &value.text}
	}
}

type wireValue struct {
	Value   *string `json:"value,omitempty"`
	ValueID string  `json:"valueId,omitempty"`
}

func renderedText(value any, asJSON bool) string {
	if asJSON {
		return documentText(value)
	}
	return scalarText(value)
}

func scalarText(value any) string {
	switch scalar := value.(type) {
	case string:
		return scalar
	case bool:
		return strconv.FormatBool(scalar)
	case int64:
		return strconv.FormatInt(scalar, 10)
	case float64:
		return evaluation.NumberText(scalar)
	default:
		return fmt.Sprint(value)
	}
}

func documentText(value any) string {
	var compact bytes.Buffer
	encoder := json.NewEncoder(&compact)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Sprint(value)
	}
	return string(bytes.TrimSuffix(compact.Bytes(), []byte("\n")))
}

func isLong(text string) bool {
	return len(text) > maxInlineValueLength && utf8.RuneCountInString(text) > maxInlineValueLength
}
