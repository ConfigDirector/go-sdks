package evaluation

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
)

var versionCoercion = regexp.MustCompile(`(^|[^0-9])([0-9]{1,16})(?:\.([0-9]{1,16}))?(?:\.([0-9]{1,16}))?(?:$|[^0-9])`)

type version struct {
	major, minor, patch int64
}

func semverMatches(value string, operator string, targets []string) bool {
	parsed := coerceVersion(value)
	firstTarget := ""
	if len(targets) > 0 {
		firstTarget = targets[0]
	}

	switch operator {
	case "=":
		return versionsEqual(parsed, coerceVersion(firstTarget))
	case "<":
		return versionsOrdered(parsed, coerceVersion(firstTarget), func(ordering int) bool { return ordering < 0 })
	case "<=":
		return versionsOrdered(parsed, coerceVersion(firstTarget), func(ordering int) bool { return ordering <= 0 })
	case ">":
		return versionsOrdered(parsed, coerceVersion(firstTarget), func(ordering int) bool { return ordering > 0 })
	case ">=":
		return versionsOrdered(parsed, coerceVersion(firstTarget), func(ordering int) bool { return ordering >= 0 })
	case "is one of":
		return equalsAnyVersion(parsed, targets)
	case "is not one of":
		return !equalsAnyVersion(parsed, targets)
	default:
		return false
	}
}

func coerceVersion(text string) *version {
	match := versionCoercion.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	return &version{
		major: versionComponent(match[2]),
		minor: versionComponent(match[3]),
		patch: versionComponent(match[4]),
	}
}

func versionComponent(digits string) int64 {
	if digits == "" {
		return 0
	}
	component, _ := strconv.ParseInt(digits, 10, 64)
	return component
}

func equalsAnyVersion(value *version, targets []string) bool {
	return slices.ContainsFunc(targets, func(target string) bool { return versionsEqual(value, coerceVersion(target)) })
}

func versionsEqual(value *version, target *version) bool {
	return value != nil && target != nil && *value == *target
}

func versionsOrdered(value *version, target *version, holds func(ordering int) bool) bool {
	return value != nil && target != nil && holds(value.compare(*target))
}

func (value version) compare(other version) int {
	return cmp.Or(
		cmp.Compare(value.major, other.major),
		cmp.Compare(value.minor, other.minor),
		cmp.Compare(value.patch, other.patch),
	)
}
