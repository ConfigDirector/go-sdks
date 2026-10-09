package configdirector

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func stateWith(configType ConfigType, value string) ConfigState {
	return ConfigState{ID: "config-1", Key: "a-key", Type: configType, Value: value, ValueID: "value-1"}
}

func TestABooleanReadsTrueInAnyCase(t *testing.T) {
	value, reason := boolFrom(stateWith(ConfigTypeBoolean, "TRUE"))

	require.True(t, value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestABooleanReadsFalseInAnyCase(t *testing.T) {
	value, reason := boolFrom(stateWith(ConfigTypeBoolean, "False"))

	require.False(t, value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestABooleanRejectsYes(t *testing.T) {
	_, reason := boolFrom(stateWith(ConfigTypeBoolean, "yes"))

	require.Equal(t, EvaluationReasonInvalidBoolean, reason)
}

func TestABooleanRejectsOne(t *testing.T) {
	_, reason := boolFrom(stateWith(ConfigTypeBoolean, "1"))

	require.Equal(t, EvaluationReasonInvalidBoolean, reason)
}

func TestAStringReadsAStringConfig(t *testing.T) {
	value, reason := stringFrom(stateWith(ConfigTypeString, "true"))

	require.Equal(t, "true", value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAStringReadsAnEnumConfig(t *testing.T) {
	value, reason := stringFrom(stateWith(ConfigTypeEnum, "true"))

	require.Equal(t, "true", value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAStringReadsAURLConfig(t *testing.T) {
	value, reason := stringFrom(stateWith(ConfigTypeURL, "true"))

	require.Equal(t, "true", value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAStringReadsACustomConfig(t *testing.T) {
	value, reason := stringFrom(stateWith(ConfigTypeCustom, "true"))

	require.Equal(t, "true", value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAStringReadsAJSONConfigAsItsRawDocument(t *testing.T) {
	value, reason := stringFrom(stateWith(ConfigTypeJSON, `{"a":1}`))

	require.Equal(t, `{"a":1}`, value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAStringRejectsABooleanConfig(t *testing.T) {
	_, reason := stringFrom(stateWith(ConfigTypeBoolean, "true"))

	require.Equal(t, EvaluationReasonTypeMismatch, reason)
}

func TestAStringRejectsAnIntegerConfig(t *testing.T) {
	_, reason := stringFrom(stateWith(ConfigTypeInteger, "26"))

	require.Equal(t, EvaluationReasonTypeMismatch, reason)
}

func TestAStringRejectsAFloatConfig(t *testing.T) {
	_, reason := stringFrom(stateWith(ConfigTypeFloat, "3.5"))

	require.Equal(t, EvaluationReasonTypeMismatch, reason)
}

func TestAnIntegerReadsAWholeNumber(t *testing.T) {
	value, reason := int64From(stateWith(ConfigTypeInteger, "-4"))

	require.Equal(t, int64(-4), value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAnIntegerReadsAWholeNumberWrittenWithADecimalPoint(t *testing.T) {
	value, reason := int64From(stateWith(ConfigTypeInteger, "26.0"))

	require.Equal(t, int64(26), value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAnIntegerReadsAWholeNumberWrittenWithAnExponent(t *testing.T) {
	value, reason := int64From(stateWith(ConfigTypeInteger, "1e3"))

	require.Equal(t, int64(1000), value)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAnIntegerRejectsAFraction(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "26.5"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsTrailingText(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "26abc"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsSurroundingWhitespace(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, " 26"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsDigitSeparators(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "1_000"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsNonASCIIDigits(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "١٢"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsInfinity(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "Infinity"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAnIntegerRejectsANumberBeyondInt64(t *testing.T) {
	_, reason := int64From(stateWith(ConfigTypeInteger, "9223372036854775808"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAFloatReadsADecimal(t *testing.T) {
	value, reason := float64From(stateWith(ConfigTypeFloat, "3.5"))

	require.InDelta(t, 3.5, value, 0)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAFloatReadsAWholeNumber(t *testing.T) {
	value, reason := float64From(stateWith(ConfigTypeFloat, "26"))

	require.InDelta(t, 26.0, value, 0)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAFloatReadsAnExponent(t *testing.T) {
	value, reason := float64From(stateWith(ConfigTypeFloat, "1e3"))

	require.InDelta(t, 1000.0, value, 0)
	require.Equal(t, EvaluationReasonFoundMatch, reason)
}

func TestAFloatRejectsNaN(t *testing.T) {
	_, reason := float64From(stateWith(ConfigTypeFloat, "nan"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAFloatRejectsInfinity(t *testing.T) {
	_, reason := float64From(stateWith(ConfigTypeFloat, "-Infinity"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAFloatRejectsAnOverflowingExponent(t *testing.T) {
	_, reason := float64From(stateWith(ConfigTypeFloat, "1e999"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAFloatRejectsTwoDecimalPoints(t *testing.T) {
	_, reason := float64From(stateWith(ConfigTypeFloat, "1.2.3"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAFloatRejectsDigitSeparators(t *testing.T) {
	_, reason := float64From(stateWith(ConfigTypeFloat, "1_0"))

	require.Equal(t, EvaluationReasonInvalidNumber, reason)
}

func TestAJSONTargetMustBeAPointer(t *testing.T) {
	_, err := jsonTargetFrom(map[string]any{})

	require.ErrorIs(t, err, errJSONTargetNotAPointer)
}

func TestAJSONTargetMustNotBeANilPointer(t *testing.T) {
	var target *map[string]any

	_, err := jsonTargetFrom(target)

	require.ErrorIs(t, err, errJSONTargetNotAPointer)
}

func TestJSONDecodesANestedDocument(t *testing.T) {
	target := map[string]any{}

	reason := decodeJSONInto(stateWith(ConfigTypeJSON, `{"nested":{"b":true}}`), reflect.ValueOf(&target))

	require.Equal(t, EvaluationReasonFoundMatch, reason)
	require.Equal(t, map[string]any{"nested": map[string]any{"b": true}}, target)
}

func TestJSONDecodesIntoAnInterfaceTarget(t *testing.T) {
	var target any

	reason := decodeJSONInto(stateWith(ConfigTypeJSON, `[1,2,3]`), reflect.ValueOf(&target))

	require.Equal(t, EvaluationReasonFoundMatch, reason)
	require.Equal(t, []any{float64(1), float64(2), float64(3)}, target)
}

func TestJSONKeepsTheTargetOnMalformedJSON(t *testing.T) {
	target := map[string]any{"a": 1}

	reason := decodeJSONInto(stateWith(ConfigTypeJSON, "{not json"), reflect.ValueOf(&target))

	require.Equal(t, EvaluationReasonInvalidJSON, reason)
	require.Equal(t, map[string]any{"a": 1}, target)
}

func TestJSONKeepsTheWholeTargetWhenOneFieldDoesNotFit(t *testing.T) {
	target := struct {
		Max int `json:"max"`
		Min int `json:"min"`
	}{Max: 5, Min: 5}

	reason := decodeJSONInto(stateWith(ConfigTypeJSON, `{"max":20,"min":"one"}`), reflect.ValueOf(&target))

	require.Equal(t, EvaluationReasonInvalidJSON, reason)
	require.Equal(t, 5, target.Max)
	require.Equal(t, 5, target.Min)
}

func TestJSONTreatsAPlainStringConfigAsInvalid(t *testing.T) {
	target := map[string]any{"a": 1}

	reason := decodeJSONInto(stateWith(ConfigTypeString, "hello"), reflect.ValueOf(&target))

	require.Equal(t, EvaluationReasonInvalidJSON, reason)
	require.Equal(t, map[string]any{"a": 1}, target)
}
