package evaluation

import (
	"math"
	"strconv"
)

const largestNumberSpelledOut = 1e21

func textOf(value any) (text string, hasText bool) {
	switch scalar := value.(type) {
	case string:
		return scalar, true
	case bool:
		return strconv.FormatBool(scalar), true
	case float64:
		return numberText(scalar), true
	default:
		return "", false
	}
}

func numberText(number float64) string {
	if math.Abs(number) < largestNumberSpelledOut {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return strconv.FormatFloat(number, 'g', -1, 64)
}
