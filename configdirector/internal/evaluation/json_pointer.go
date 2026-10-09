package evaluation

import (
	"strconv"
	"strings"
)

func traitAtPointer(traits map[string]any, pointer string) any {
	if !strings.HasPrefix(pointer, "/") {
		return nil
	}

	var current any = traits
	for _, rawToken := range strings.Split(pointer[1:], "/") {
		token, valid := decodePointerToken(rawToken)
		if !valid {
			return nil
		}
		current = memberOf(current, token)
		if current == nil {
			return nil
		}
	}
	return current
}

func decodePointerToken(rawToken string) (string, bool) {
	for index := range len(rawToken) {
		if rawToken[index] != '~' {
			continue
		}
		if index+1 >= len(rawToken) || (rawToken[index+1] != '0' && rawToken[index+1] != '1') {
			return "", false
		}
	}
	return strings.ReplaceAll(strings.ReplaceAll(rawToken, "~1", "/"), "~0", "~"), true
}

func memberOf(current any, token string) any {
	switch container := current.(type) {
	case map[string]any:
		return container[token]
	case []any:
		index, valid := arrayIndex(token)
		if !valid || index >= len(container) {
			return nil
		}
		return container[index]
	default:
		return nil
	}
}

func arrayIndex(token string) (int, bool) {
	if token == "" || (token[0] == '0' && len(token) > 1) {
		return 0, false
	}
	for index := range len(token) {
		if token[index] < '0' || token[index] > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.Atoi(token)
	return parsed, err == nil
}
