package evaluation

import (
	"math"
	"strconv"
	"strings"
)

const decimalLiteralCharacters = "0123456789+-.eE"

type numberOperator struct {
	holds               func(number float64, target float64) bool
	holdsForUnparseable bool
}

var numberOperators = map[string]numberOperator{
	"=":              {holds: func(number, target float64) bool { return number == target }},
	"equals":         {holds: func(number, target float64) bool { return number == target }},
	"!=":             {holds: func(number, target float64) bool { return number != target }, holdsForUnparseable: true},
	"does not equal": {holds: func(number, target float64) bool { return number != target }, holdsForUnparseable: true},
	"<":              {holds: func(number, target float64) bool { return number < target }},
	"<=":             {holds: func(number, target float64) bool { return number <= target }},
	">":              {holds: func(number, target float64) bool { return number > target }},
	">=":             {holds: func(number, target float64) bool { return number >= target }},
}

func numberMatches(value any, operator string, targets []string) bool {
	comparison, known := numberOperators[operator]
	if !known {
		return false
	}

	number, parseable := finiteNumberOf(value)
	if !parseable {
		return comparison.holdsForUnparseable
	}

	if len(targets) == 0 {
		return false
	}
	target, parseable := finiteNumberInText(targets[0])
	if !parseable {
		return false
	}
	return comparison.holds(number, target)
}

func finiteNumberOf(value any) (float64, bool) {
	switch candidate := value.(type) {
	case float64:
		return candidate, !math.IsInf(candidate, 0) && !math.IsNaN(candidate)
	case string:
		return finiteNumberInText(candidate)
	default:
		return 0, false
	}
}

func finiteNumberInText(text string) (float64, bool) {
	if text == "" || strings.ContainsFunc(text, func(character rune) bool {
		return !strings.ContainsRune(decimalLiteralCharacters, character)
	}) {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}
