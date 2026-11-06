package configdirector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

const decimalCharacters = "0123456789+-.eE"

var (
	errJSONTargetNotAPointer = errors.New("configdirector: the JSON target must be a non-nil pointer")
	errBlankConfigKey        = errors.New("configdirector: the config key is blank; a read needs a config key")
)

func boolFrom(state ConfigState) (bool, EvaluationReason) {
	switch strings.ToLower(state.Value) {
	case "true":
		return true, EvaluationReasonFoundMatch
	case "false":
		return false, EvaluationReasonFoundMatch
	default:
		return false, EvaluationReasonInvalidBoolean
	}
}

func stringFrom(state ConfigState) (string, EvaluationReason) {
	switch state.Type {
	case ConfigTypeBoolean, ConfigTypeInteger, ConfigTypeFloat:
		return "", EvaluationReasonTypeMismatch
	case ConfigTypeCustom, ConfigTypeString, ConfigTypeEnum, ConfigTypeURL, ConfigTypeJSON:
		return state.Value, EvaluationReasonFoundMatch
	default:
		return state.Value, EvaluationReasonFoundMatch
	}
}

func int64From(state ConfigState) (int64, EvaluationReason) {
	if !hasOnlyDecimalCharacters(state.Value) {
		return 0, EvaluationReasonInvalidNumber
	}
	if whole, err := strconv.ParseInt(state.Value, 10, 64); err == nil {
		return whole, EvaluationReasonFoundMatch
	}
	number, isFinite := finiteFloat(state.Value)
	if !isFinite || number != math.Trunc(number) || number < math.MinInt64 || number >= math.MaxInt64 {
		return 0, EvaluationReasonInvalidNumber
	}
	return int64(number), EvaluationReasonFoundMatch
}

func float64From(state ConfigState) (float64, EvaluationReason) {
	if !hasOnlyDecimalCharacters(state.Value) {
		return 0, EvaluationReasonInvalidNumber
	}
	number, isFinite := finiteFloat(state.Value)
	if !isFinite {
		return 0, EvaluationReasonInvalidNumber
	}
	return number, EvaluationReasonFoundMatch
}

func hasOnlyDecimalCharacters(text string) bool {
	for _, character := range text {
		if !strings.ContainsRune(decimalCharacters, character) {
			return false
		}
	}
	return true
}

func finiteFloat(text string) (float64, bool) {
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func jsonTargetFrom(target any) (reflect.Value, error) {
	pointer := reflect.ValueOf(target)
	if pointer.Kind() != reflect.Pointer || pointer.IsNil() {
		return reflect.Value{}, errJSONTargetNotAPointer
	}
	return pointer, nil
}

func decodeJSONInto(state ConfigState, target reflect.Value) EvaluationReason {
	document := []byte(state.Value)
	trialTarget := reflect.New(target.Elem().Type())
	if err := json.Unmarshal(document, trialTarget.Interface()); err != nil {
		return EvaluationReasonInvalidJSON
	}
	if err := json.Unmarshal(document, target.Interface()); err != nil {
		return EvaluationReasonInvalidJSON
	}
	return EvaluationReasonFoundMatch
}

type valueReading struct {
	kind      readKind
	valueFrom func(ConfigState) (any, EvaluationReason)
}

func valueReadingOf(defaultValue any) (valueReading, error) {
	valueType := reflect.TypeOf(defaultValue)
	switch reflect.ValueOf(defaultValue).Kind() {
	case reflect.Bool:
		return valueReading{kind: typedRead, valueFrom: filledValueOfType(valueType, fillBool)}, nil
	case reflect.String:
		return valueReading{kind: typedRead, valueFrom: filledValueOfType(valueType, fillString)}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return valueReading{kind: typedRead, valueFrom: filledValueOfType(valueType, fillInt)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return valueReading{kind: typedRead, valueFrom: filledValueOfType(valueType, fillUint)}, nil
	case reflect.Float32, reflect.Float64:
		return valueReading{kind: typedRead, valueFrom: filledValueOfType(valueType, fillFloat)}, nil
	case reflect.Map, reflect.Struct:
		return valueReading{kind: documentRead, valueFrom: filledValueOfType(valueType, fillFromJSONObject)}, nil
	case reflect.Slice, reflect.Array:
		return valueReading{kind: documentRead, valueFrom: filledValueOfType(valueType, fillFromJSONArray)}, nil
	default:
		return valueReading{}, fmt.Errorf("configdirector: the in-code default value is a %T, which cannot be read; it must be a bool, a string, an integer, a float, or a map, slice, array or struct", defaultValue)
	}
}

func filledValueOfType(valueType reflect.Type, fill func(ConfigState, reflect.Value) EvaluationReason) func(ConfigState) (any, EvaluationReason) {
	return func(state ConfigState) (any, EvaluationReason) {
		value := reflect.New(valueType).Elem()
		if reason := fill(state, value); reason != EvaluationReasonFoundMatch {
			return nil, reason
		}
		return value.Interface(), EvaluationReasonFoundMatch
	}
}

func fillBool(state ConfigState, value reflect.Value) EvaluationReason {
	parsed, reason := boolFrom(state)
	value.SetBool(parsed)
	return reason
}

func fillString(state ConfigState, value reflect.Value) EvaluationReason {
	parsed, reason := stringFrom(state)
	value.SetString(parsed)
	return reason
}

func fillInt(state ConfigState, value reflect.Value) EvaluationReason {
	whole, reason := int64From(state)
	if reason != EvaluationReasonFoundMatch {
		return reason
	}
	if value.OverflowInt(whole) {
		return EvaluationReasonInvalidNumber
	}
	value.SetInt(whole)
	return EvaluationReasonFoundMatch
}

func fillUint(state ConfigState, value reflect.Value) EvaluationReason {
	whole, reason := uint64From(state)
	if reason != EvaluationReasonFoundMatch {
		return reason
	}
	if value.OverflowUint(whole) {
		return EvaluationReasonInvalidNumber
	}
	value.SetUint(whole)
	return EvaluationReasonFoundMatch
}

func uint64From(state ConfigState) (uint64, EvaluationReason) {
	if whole, err := strconv.ParseUint(state.Value, 10, 64); err == nil {
		return whole, EvaluationReasonFoundMatch
	}
	whole, reason := int64From(state)
	if reason != EvaluationReasonFoundMatch {
		return 0, reason
	}
	if whole < 0 {
		return 0, EvaluationReasonInvalidNumber
	}
	return uint64(whole), EvaluationReasonFoundMatch
}

func fillFloat(state ConfigState, value reflect.Value) EvaluationReason {
	number, reason := float64From(state)
	if reason != EvaluationReasonFoundMatch {
		return reason
	}
	if value.OverflowFloat(number) {
		return EvaluationReasonInvalidNumber
	}
	value.SetFloat(number)
	return EvaluationReasonFoundMatch
}

func fillFromJSONObject(state ConfigState, value reflect.Value) EvaluationReason {
	return fillFromDocumentOpeningWith("{", state, value)
}

func fillFromJSONArray(state ConfigState, value reflect.Value) EvaluationReason {
	return fillFromDocumentOpeningWith("[", state, value)
}

func fillFromDocumentOpeningWith(opening string, state ConfigState, value reflect.Value) EvaluationReason {
	document := []byte(state.Value)
	if !bytes.HasPrefix(bytes.TrimSpace(document), []byte(opening)) {
		return EvaluationReasonInvalidJSON
	}
	if err := json.Unmarshal(document, value.Addr().Interface()); err != nil {
		return EvaluationReasonInvalidJSON
	}
	return EvaluationReasonFoundMatch
}
