package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	acmeSegmentID    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	missingSegmentID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func textCondition(attribute string, trait string, operator string, targets ...string) AttributeCondition {
	return AttributeCondition{
		ID:           "condition",
		Attribute:    attribute,
		Trait:        trait,
		Operator:     operator,
		TargetType:   "text",
		TargetValues: targets,
	}
}

func emailEndsWith(domain string) AttributeCondition {
	return textCondition("traits", "/email", "ends with any of", domain)
}

func planIs(plan string) AttributeCondition {
	return textCondition("traits", "/plan", "equals", plan)
}

func acmeSegment(groups ...[]AttributeCondition) Segments {
	return Segments{acmeSegmentID: Segment{Groups: groups}}
}

func TestIdentifierConditionComparesTheContextIdentifier(t *testing.T) {
	condition := textCondition("identifier", "", "equals", "u1")

	require.True(t, condition.holdsFor(Context{Identifier: "u1"}, nil))
	require.False(t, condition.holdsFor(Context{Identifier: "u2"}, nil))
}

func TestNameConditionComparesTheContextName(t *testing.T) {
	condition := textCondition("name", "", "equals", "ann")

	require.True(t, condition.holdsFor(Context{Name: "ann"}, nil))
	require.False(t, condition.holdsFor(Context{Name: "bob"}, nil))
}

func TestAppNameConditionComparesTheApplicationName(t *testing.T) {
	condition := textCondition("appName", "", "equals", "shop")

	require.True(t, condition.holdsFor(Context{AppName: "shop"}, nil))
	require.False(t, condition.holdsFor(Context{AppName: "admin"}, nil))
}

func TestAppVersionConditionComparesTheApplicationVersion(t *testing.T) {
	condition := AttributeCondition{Attribute: "appVersion", Operator: ">=", TargetType: "semver", TargetValues: []string{"2.0.0"}}

	require.True(t, condition.holdsFor(Context{AppVersion: "v2.3.4"}, nil))
	require.False(t, condition.holdsFor(Context{AppVersion: "1.9.0"}, nil))
}

func TestTraitsConditionReadsTheTraitAtItsPointer(t *testing.T) {
	condition := textCondition("traits", "/team/name", "equals", "core")

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"team": map[string]any{"name": "core"}}}, nil))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"team": map[string]any{"name": "web"}}}, nil))
}

func TestTraitsConditionWithoutAPointerComparesAsEmpty(t *testing.T) {
	condition := textCondition("traits", "", "equals", "")

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"plan": "pro"}}, nil))
}

func TestAnAbsentAttributeComparesAsEmptyText(t *testing.T) {
	condition := textCondition("traits", "/plan", "is not one of", "free")

	require.True(t, condition.holdsFor(Context{}, nil))
}

func TestAnUnknownAttributeNeverMatches(t *testing.T) {
	condition := textCondition("email", "", "is not one of")

	require.False(t, condition.holdsFor(Context{}, nil))
}

func TestAnUnknownTargetTypeNeverMatches(t *testing.T) {
	condition := AttributeCondition{Attribute: "identifier", Operator: "equals", TargetType: "uuid", TargetValues: []string{"u1"}}

	require.False(t, condition.holdsFor(Context{Identifier: "u1"}, nil))
}

func TestOperatorsAreMatchedCaseInsensitively(t *testing.T) {
	condition := textCondition("identifier", "", "Is NOT One Of", "u2")

	require.True(t, condition.holdsFor(Context{Identifier: "u1"}, nil))
}

func TestABooleanTraitComparesAsItsTextSpelling(t *testing.T) {
	condition := textCondition("traits", "/beta", "equals", "true")

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"beta": true}}, nil))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"beta": false}}, nil))
}

func TestANumericTraitComparesAsANumber(t *testing.T) {
	condition := AttributeCondition{Attribute: "traits", Trait: "/seats", Operator: ">", TargetType: "number", TargetValues: []string{"10"}}

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"seats": 26.0}}, nil))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"seats": 5.0}}, nil))
}

func TestAStructuredTraitComparesAsEmptyText(t *testing.T) {
	condition := textCondition("traits", "/tags", "equals", "")

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"tags": []any{"a"}}}, nil))
}

func TestAnArrayTraitComparesItsElements(t *testing.T) {
	condition := AttributeCondition{Attribute: "traits", Trait: "/tags", Operator: "contains any of", TargetType: "array", TargetValues: []string{"blue"}}

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"tags": []any{"red", "blue"}}}, nil))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"tags": []any{"red"}}}, nil))
}

func TestADatetimeTraitComparesAsAnInstant(t *testing.T) {
	condition := AttributeCondition{Attribute: "traits", Trait: "/joined", Operator: "is before", TargetType: "datetime", TargetValues: []string{"2026-01-28"}}

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"joined": "2025-12-31"}}, nil))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"joined": "2026-02-01"}}, nil))
}

func TestInSegmentHoldsForAContextInTheSegment(t *testing.T) {
	condition := SegmentCondition{ID: "condition", Operator: "in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"email": "ann@acme.com"}}, segments))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"email": "bob@example.com"}}, segments))
}

func TestNotInSegmentHoldsForAContextOutsideTheSegment(t *testing.T) {
	condition := SegmentCondition{ID: "condition", Operator: "not in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"email": "bob@example.com"}}, segments))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"email": "ann@acme.com"}}, segments))
}

func TestSegmentOperatorsAreMatchedCaseInsensitively(t *testing.T) {
	condition := SegmentCondition{ID: "condition", Operator: "Not In", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"email": "bob@example.com"}}, segments))
}

func TestAnUnknownSegmentOperatorNeverHolds(t *testing.T) {
	condition := SegmentCondition{ID: "condition", Operator: "equals", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})

	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"email": "ann@acme.com"}}, segments))
}

func TestAMissingSegmentHoldsForNeitherInNorNotIn(t *testing.T) {
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})
	context := Context{Traits: map[string]any{"email": "ann@acme.com"}}

	require.False(t, SegmentCondition{Operator: "in", SegmentID: missingSegmentID}.holdsFor(context, segments))
	require.False(t, SegmentCondition{Operator: "not in", SegmentID: missingSegmentID}.holdsFor(context, segments))
}

func TestAPayloadWithoutSegmentsHoldsForNeitherInNorNotIn(t *testing.T) {
	context := Context{Traits: map[string]any{"email": "ann@acme.com"}}

	require.False(t, SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}.holdsFor(context, nil))
	require.False(t, SegmentCondition{Operator: "not in", SegmentID: acmeSegmentID}.holdsFor(context, nil))
}

func TestAContextIsInASegmentWhenAnyGroupMatches(t *testing.T) {
	condition := SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{planIs("pro")}, []AttributeCondition{emailEndsWith("@acme.com")})

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"email": "ann@acme.com"}}, segments))
	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"plan": "pro"}}, segments))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"plan": "free"}}, segments))
}

func TestAGroupMatchesOnlyWhenEveryConditionHolds(t *testing.T) {
	condition := SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{planIs("pro"), emailEndsWith("@acme.com")})

	require.True(t, condition.holdsFor(Context{Traits: map[string]any{"plan": "pro", "email": "ann@acme.com"}}, segments))
	require.False(t, condition.holdsFor(Context{Traits: map[string]any{"plan": "pro", "email": "ann@example.com"}}, segments))
}

func TestAnAbsentTraitComparesAsEmptyInsideAGroup(t *testing.T) {
	condition := SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{textCondition("traits", "/plan", "is not one of", "free")})

	require.True(t, condition.holdsFor(Context{}, segments))
}

func TestAGroupWithNoConditionsMatchesEveryContext(t *testing.T) {
	condition := SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}
	segments := acmeSegment([]AttributeCondition{})

	require.True(t, condition.holdsFor(Context{}, segments))
}

func TestASegmentWithNoGroupsMatchesNoContext(t *testing.T) {
	segments := acmeSegment()
	context := Context{Traits: map[string]any{"email": "ann@acme.com"}}

	require.False(t, SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}.holdsFor(context, segments))
	require.True(t, SegmentCondition{Operator: "not in", SegmentID: acmeSegmentID}.holdsFor(context, segments))
}

func TestKindConditionComparesTheContextKind(t *testing.T) {
	condition := textCondition("kind", "", "is one of", "device", "tenant")

	require.True(t, condition.holdsFor(Context{Kind: "device"}, nil))
	require.False(t, condition.holdsFor(Context{Kind: "organization"}, nil))
}

func TestAnOmittedKindComparesAsUser(t *testing.T) {
	condition := textCondition("kind", "", "equals", "user")

	require.True(t, condition.holdsFor(Context{}, nil))
	require.True(t, condition.holdsFor(Context{Kind: "user"}, nil))
	require.False(t, condition.holdsFor(Context{Kind: "device"}, nil))
}

func TestATraitNamedKindIsAnOrdinaryTrait(t *testing.T) {
	traitCondition := textCondition("traits", "/kind", "equals", "robot")
	kindCondition := textCondition("kind", "", "equals", "robot")
	context := Context{Kind: "device", Traits: map[string]any{"kind": "robot"}}

	require.True(t, traitCondition.holdsFor(context, nil))
	require.False(t, kindCondition.holdsFor(context, nil))
}
