package configdirector

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
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

// FuzzParseValue checks that reading a served value as each type never panics, reads a boolean
// by lower-casing rather than case folding, accepts every whole number and decodable JSON
// document the standard library does, and that a JSON document that cannot be decoded leaves
// the target as it was.
func FuzzParseValue(f *testing.F) {
	f.Add("")
	f.Add("true")
	f.Add("FALSE")
	f.Add("yes")
	f.Add("1")
	f.Add("26")
	f.Add("26.0")
	f.Add("2.6e1")
	f.Add("-0")
	f.Add("1e400")
	f.Add("NaN")
	f.Add("Infinity")
	f.Add("1_000")
	f.Add("٢٦")
	f.Add("26abc")
	f.Add(" 26")
	f.Add("9223372036854775807")
	f.Add("9223372036854775808")
	f.Add("9007199254740993")
	f.Add("3.14")
	f.Add("10")
	f.Add("25")
	f.Add("abc")
	f.Add(`{"max":20,"min":1}`)
	f.Add(`["eu","us"]`)
	f.Add("{not json")
	f.Add(`"text"`)
	f.Add("null")
	f.Add(`{"Max":"x"}`)
	f.Add(`{"Max":9,"Min":"x"}`)

	f.Fuzz(func(t *testing.T, text string) {
		boolean, booleanReason := boolFrom(stateWith(ConfigTypeBoolean, text))
		lowered := strings.ToLower(text)
		if lowered == "true" || lowered == "false" {
			require.Equal(t, EvaluationReasonFoundMatch, booleanReason)
			require.Equal(t, lowered == "true", boolean)
		} else {
			require.Equal(t, EvaluationReasonInvalidBoolean, booleanReason)
		}

		whole, wholeReason := int64From(stateWith(ConfigTypeInteger, text))
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			require.Equal(t, EvaluationReasonFoundMatch, wholeReason)
			require.Equal(t, parsed, whole)
		}
		number, numberReason := float64From(stateWith(ConfigTypeFloat, text))
		if wholeReason == EvaluationReasonFoundMatch {
			require.Equal(t, EvaluationReasonFoundMatch, numberReason)
			require.Equal(t, float64(whole), number)
		}
		if numberReason == EvaluationReasonFoundMatch {
			require.False(t, math.IsNaN(number) || math.IsInf(number, 0))
			reparsed, err := strconv.ParseFloat(strconv.FormatFloat(number, 'g', -1, 64), 64)
			require.NoError(t, err)
			require.Equal(t, number, reparsed)
		}

		var document any
		documentReason := decodeJSONInto(stateWith(ConfigTypeJSON, text), reflect.ValueOf(&document))
		require.Equal(t, json.Unmarshal([]byte(text), new(any)) == nil, documentReason == EvaluationReasonFoundMatch)
		untouchedLimits := struct{ Max, Min int }{Max: 7, Min: 1}
		limits := untouchedLimits
		if decodeJSONInto(stateWith(ConfigTypeJSON, text), reflect.ValueOf(&limits)) != EvaluationReasonFoundMatch {
			require.Equal(t, untouchedLimits, limits)
		}
	})
}
