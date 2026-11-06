package telemetry

import (
	"encoding"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/transport"
)

const (
	trackedEventsPerBatch      = 1_000
	nameMaximumCharacters      = 150
	detailsMaximumTopLevelKeys = 20
	detailsMaximumJSONBytes    = 1_024

	leastJSONBytesPerValue       = 2
	valuesThatFitTheDetailsLimit = detailsMaximumJSONBytes / leastJSONBytesPerValue
)

type trackedEventWarning string

var (
	nameOutsideTheCharacterLimit = trackedEventWarning(fmt.Sprintf("A tracked event was not reported because its name does not have 1 to %d characters", nameMaximumCharacters))
	valueNotAFiniteNumber        = trackedEventWarning("A tracked event was reported without its value because the value is not a finite number")
	detailsNotReportable         = trackedEventWarning(fmt.Sprintf("A tracked event was reported without its details because they have more than %d top-level keys, are larger than %d bytes as JSON, or cannot be written as JSON", detailsMaximumTopLevelKeys, detailsMaximumJSONBytes))
	batchAlreadyFull             = trackedEventWarning(fmt.Sprintf("A tracked event was not reported because %d tracked events are already waiting for the next telemetry batch", trackedEventsPerBatch))
)

// TrackCall is one call to the client's Track, with its context already admitted.
type TrackCall struct {
	Name       string
	Context    Context
	Value      any
	Details    map[string]any
	HappenedAt time.Time
}

// TrackedEventAdmission is what a TrackCall becomes under the limits on tracked events.
type TrackedEventAdmission struct {
	Admitted bool
	Name     string
	Value    any
	Details  map[string]any
	Warnings []trackedEventWarning

	wire wireTrackedEvent
}

// AdmitTrackedEvent applies the limits on tracked events to call.
func AdmitTrackedEvent(call TrackCall) TrackedEventAdmission {
	if !isWithinTheCharacterLimit(call.Name) {
		return TrackedEventAdmission{Warnings: []trackedEventWarning{nameOutsideTheCharacterLimit}}
	}

	value := reportableValueOf(call.Value)
	details := reportableDetailsOf(call.Details)
	entry := wireTrackedEvent{
		Name:       call.Name,
		Value:      value.wireForm,
		Details:    details.wireForm,
		HappenedAt: wireTime(call.HappenedAt),
	}
	if call.Context.identified() {
		entry.ContextKind = call.Context.Kind
		entry.ContextIdentifier = call.Context.ID
	}
	return TrackedEventAdmission{
		Admitted: true,
		Name:     call.Name,
		Value:    value.recordedForm,
		Details:  details.recordedForm,
		Warnings: slices.Concat(value.warnings, details.warnings),
		wire:     entry,
	}
}

func isWithinTheCharacterLimit(name string) bool {
	characters := utf8.RuneCountInString(name)
	return characters >= 1 && characters <= nameMaximumCharacters
}

type reportable[T any] struct {
	recordedForm T
	wireForm     json.RawMessage
	warnings     []trackedEventWarning
}

func reportableValueOf(value any) reportable[any] {
	if value == nil {
		return reportable[any]{}
	}

	text := numberTextOf(value)
	if !isJSONNumber(text) {
		return reportable[any]{warnings: []trackedEventWarning{valueNotAFiniteNumber}}
	}
	return reportable[any]{recordedForm: value, wireForm: json.RawMessage(text)}
}

func numberTextOf(value any) (text string) {
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()

	if writtenText := textWrittenBy(value); isJSONNumber(writtenText) {
		return writtenText
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(reflected.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(reflected.Uint(), 10)
	case reflect.Float32:
		return floatText(float32(reflected.Float()))
	case reflect.Float64:
		return floatText(reflected.Float())
	default:
		return ""
	}
}

func floatText(number any) string {
	text, err := json.Marshal(number)
	if err != nil {
		return ""
	}
	return string(text)
}

func textWrittenBy(value any) string {
	switch number := value.(type) {
	case json.Number:
		return string(number)
	case encoding.TextMarshaler:
		text, err := number.MarshalText()
		if err != nil {
			return ""
		}
		return string(text)
	default:
		return ""
	}
}

func isJSONNumber(text string) bool {
	if text == "" {
		return false
	}
	startsAsANumber := text[0] == '-' || isDigit(text[0])
	return startsAsANumber && isDigit(text[len(text)-1]) && json.Valid([]byte(text))
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func reportableDetailsOf(details map[string]any) reportable[map[string]any] {
	if details == nil {
		return reportable[map[string]any]{}
	}

	leftOut := reportable[map[string]any]{warnings: []trackedEventWarning{detailsNotReportable}}
	if len(details) > detailsMaximumTopLevelKeys {
		return leftOut
	}
	copier := detailsCopier{valuesLeft: valuesThatFitTheDetailsLimit}
	detailsCopy := copier.copyOfObject(details)
	if copier.overspent {
		return leftOut
	}
	detailsJSON, writable := jsonOfDetails(detailsCopy)
	if !writable || len(detailsJSON) > detailsMaximumJSONBytes {
		return leftOut
	}
	return reportable[map[string]any]{recordedForm: detailsCopy, wireForm: detailsJSON}
}

type detailsCopier struct {
	valuesLeft int
	overspent  bool
}

func (copier *detailsCopier) spend(values int) bool {
	copier.valuesLeft -= values
	copier.overspent = copier.overspent || copier.valuesLeft < 0
	return !copier.overspent
}

func (copier *detailsCopier) copyOfObject(object map[string]any) map[string]any {
	if !copier.spend(len(object)) {
		return nil
	}
	objectCopy := maps.Clone(object)
	for key, member := range object {
		objectCopy[key] = copier.copyOf(member)
	}
	return objectCopy
}

func (copier *detailsCopier) copyOfArray(array []any) []any {
	if !copier.spend(len(array)) {
		return nil
	}
	arrayCopy := slices.Clone(array)
	for index, item := range array {
		arrayCopy[index] = copier.copyOf(item)
	}
	return arrayCopy
}

func (copier *detailsCopier) copyOf(value any) any {
	switch held := value.(type) {
	case map[string]any:
		return copier.copyOfObject(held)
	case []any:
		return copier.copyOfArray(held)
	case float64:
		return finiteOrNull(held)
	case float32:
		return finiteOrNull(held)
	default:
		return value
	}
}

func finiteOrNull[Number float32 | float64](number Number) any {
	if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
		return nil
	}
	return number
}

func jsonOfDetails(details map[string]any) (detailsJSON []byte, writable bool) {
	defer func() {
		if recover() != nil {
			detailsJSON, writable = nil, false
		}
	}()

	detailsJSON, err := transport.JSONWithoutHTMLEscaping(details)
	return detailsJSON, err == nil && utf8.Valid(detailsJSON)
}

type wireTrackedEvent struct {
	Name              string          `json:"name"`
	ContextKind       string          `json:"contextKind,omitempty"`
	ContextIdentifier string          `json:"contextIdentifier,omitempty"`
	Value             json.RawMessage `json:"value,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
	HappenedAt        wireTime        `json:"happenedAt"`
}
