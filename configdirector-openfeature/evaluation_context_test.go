package configdirectoropenfeature

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"
)

func TestTheTargetingKeyBecomesTheID(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{openfeature.TargetingKey: "user-1"})

	require.Equal(t, configdirector.UserContext{ID: "user-1"}, userContext)
}

func TestAnIDAttributeBecomesTheIDWithoutATargetingKey(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{"id": "user-2"})

	require.Equal(t, configdirector.UserContext{ID: "user-2"}, userContext)
}

func TestTheTargetingKeyWinsOverAnIDAttribute(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{openfeature.TargetingKey: "user-1", "id": "user-2"})

	require.Equal(t, "user-1", userContext.ID)
}

func TestNameTraitsAndAnonymousMapOntoTheirFields(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{
		"name":      "Ada",
		"traits":    map[string]any{"plan": "pro"},
		"anonymous": true,
	})

	require.Equal(t, configdirector.UserContext{Name: "Ada", Traits: map[string]any{"plan": "pro"}, Anonymous: true}, userContext)
}

func TestANumberOrBooleanIDIsItsText(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{"id": 42, "name": true})

	require.Equal(t, configdirector.UserContext{ID: "42", Name: "true"}, userContext)
}

func TestAStructuredIDOrNameIsLeftEmpty(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{"id": []any{"user-1"}, "name": map[string]any{"first": "Ada"}})

	require.Equal(t, configdirector.UserContext{}, userContext)
}

func TestTraitsThatAreNotAMapAndAnonymousThatIsNotABooleanAreIgnored(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{"traits": "pro", "anonymous": "yes"})

	require.Equal(t, configdirector.UserContext{}, userContext)
}

func TestEmptyTraitsAreNoTraits(t *testing.T) {
	t.Parallel()

	userContext := userContextFrom(openfeature.FlattenedContext{"traits": map[string]any{}})

	require.Nil(t, userContext.Traits)
}

func TestAnEmptyContextIsAnEmptyUserContext(t *testing.T) {
	t.Parallel()

	require.Equal(t, configdirector.UserContext{}, userContextFrom(openfeature.FlattenedContext{}))
}
