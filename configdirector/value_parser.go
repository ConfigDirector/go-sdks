package configdirector

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
)

const decimalCharacters = "0123456789+-.eE"

var errJSONTargetNotAPointer = errors.New("configdirector: the JSON target must be a non-nil pointer")

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
