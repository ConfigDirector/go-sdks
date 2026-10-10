package evaluation

import "slices"

func arrayMatches(value any, operator string, targets []string) bool {
	elements, isArray := value.([]any)
	containsAnyTarget := isArray && slices.ContainsFunc(elements, func(element any) bool {
		text, hasText := textOf(element)
		return hasText && slices.Contains(targets, text)
	})

	switch operator {
	case "contains any of":
		return containsAnyTarget
	case "does not contain any of":
		return !containsAnyTarget
	default:
		return false
	}
}
