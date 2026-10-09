package evaluation

import (
	"regexp"
	"strconv"
	"time"
)

var ecmascriptDatetime = regexp.MustCompile(
	`^([+-][0-9]{6}|[0-9]{4})` +
		`(?:-([0-9]{2})(?:-([0-9]{2}))?)?` +
		`(?:T([0-9]{2}):([0-9]{2})(?::([0-9]{2})(?:\.([0-9]+))?)?([Zz]|[+-][0-9]{2}:[0-9]{2})?)?$`,
)

func datetimeMatches(value string, operator string, targets []string) bool {
	if len(targets) == 0 {
		return false
	}

	instant, parseable := instantOf(value)
	target, targetParseable := instantOf(targets[0])
	if !parseable || !targetParseable {
		return false
	}

	switch operator {
	case "is before":
		return instant.Before(target)
	case "is after":
		return instant.After(target)
	default:
		return false
	}
}

func instantOf(text string) (time.Time, bool) {
	match := ecmascriptDatetime.FindStringSubmatch(text)
	if match == nil {
		return time.Time{}, false
	}
	year := digitsOrDefault(match[1], 0)
	month := digitsOrDefault(match[2], 1)
	day := digitsOrDefault(match[3], 1)
	hour := digitsOrDefault(match[4], 0)
	minute := digitsOrDefault(match[5], 0)
	second := digitsOrDefault(match[6], 0)
	millisecond := millisecondsIn(match[7])
	zone, zoneValid := zoneOf(match[8])

	if !zoneValid || month < 1 || month > 12 || day < 1 || day > daysIn(year, month) ||
		hour > 23 || minute > 59 || second > 59 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, millisecond*int(time.Millisecond), zone), true
}

func digitsOrDefault(digits string, fallback int) int {
	if digits == "" {
		return fallback
	}
	parsed, _ := strconv.Atoi(digits)
	return parsed
}

func millisecondsIn(fraction string) int {
	return digitsOrDefault((fraction + "000")[:3], 0)
}

func zoneOf(offset string) (*time.Location, bool) {
	if offset == "" || offset == "Z" || offset == "z" {
		return time.UTC, true
	}
	hours := digitsOrDefault(offset[1:3], 0)
	minutes := digitsOrDefault(offset[4:6], 0)
	if hours > 23 || minutes > 59 {
		return nil, false
	}
	seconds := hours*3600 + minutes*60
	if offset[0] == '-' {
		seconds = -seconds
	}
	return time.FixedZone(offset, seconds), true
}

func daysIn(year int, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
