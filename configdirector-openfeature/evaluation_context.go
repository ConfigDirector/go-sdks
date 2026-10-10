package configdirectoropenfeature

import (
	"fmt"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
)

const (
	idAttribute        = "id"
	nameAttribute      = "name"
	traitsAttribute    = "traits"
	anonymousAttribute = "anonymous"
)

func userContextFrom(flattenedContext openfeature.FlattenedContext) configdirector.UserContext {
	return configdirector.UserContext{
		ID:        identifierIn(flattenedContext),
		Name:      textOf(flattenedContext[nameAttribute]),
		Traits:    traitsOf(flattenedContext[traitsAttribute]),
		Anonymous: anonymousOf(flattenedContext[anonymousAttribute]),
	}
}

func identifierIn(flattenedContext openfeature.FlattenedContext) string {
	if targetingKey, ok := flattenedContext[openfeature.TargetingKey].(string); ok && targetingKey != "" {
		return targetingKey
	}
	return textOf(flattenedContext[idAttribute])
}

func textOf(attribute any) string {
	switch attribute.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(attribute)
	default:
		return ""
	}
}

func traitsOf(attribute any) map[string]any {
	traits, ok := attribute.(map[string]any)
	if !ok || len(traits) == 0 {
		return nil
	}
	return traits
}

func anonymousOf(attribute any) bool {
	anonymous, ok := attribute.(bool)
	return ok && anonymous
}
