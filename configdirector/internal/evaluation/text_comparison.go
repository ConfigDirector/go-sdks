package evaluation

import (
	"slices"
	"strings"
)

func textMatches(value string, operator string, targets []string) bool {
	switch operator {
	case "=", "equals":
		return len(targets) > 0 && value == targets[0]
	case "!=", "does not equal":
		return len(targets) > 0 && value != targets[0]
	case "is one of":
		return slices.Contains(targets, value)
	case "is not one of":
		return !slices.Contains(targets, value)
	case "starts with any of":
		return startsWithAny(value, targets)
	case "does not start with any of":
		return !startsWithAny(value, targets)
	case "ends with any of":
		return endsWithAny(value, targets)
	case "does not end with any of":
		return !endsWithAny(value, targets)
	default:
		return false
	}
}

func startsWithAny(value string, targets []string) bool {
	return slices.ContainsFunc(targets, func(target string) bool { return strings.HasPrefix(value, target) })
}

func endsWithAny(value string, targets []string) bool {
	return slices.ContainsFunc(targets, func(target string) bool { return strings.HasSuffix(value, target) })
}
