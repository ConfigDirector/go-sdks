package evaluation

import (
	"encoding/json"
	"math"
)

const deepestNestingWalked = 100

// TraitsInJSONDataModel returns traits as targeting rules read them: each one as the value
// encoding/json encodes it to, read back the way json.Unmarshal into any reads it. Traits already
// in that form are returned as they are, without a copy. A trait encoding/json cannot encode is
// left out, so it matches no condition, exactly as a missing trait does. A trait nested deeper
// than deepestNestingWalked levels, a map or slice that refers to itself included, is encoded
// rather than walked.
func TraitsInJSONDataModel(traits map[string]any) map[string]any {
	if allInJSONDataModel(traits, 0) {
		return traits
	}

	encodedTraits := make(map[string]any, len(traits))
	for name, trait := range traits {
		if value, encodable := traitInJSONDataModel(trait); encodable {
			encodedTraits[name] = value
		}
	}
	return encodedTraits
}

func allInJSONDataModel(members map[string]any, depth int) bool {
	for _, member := range members {
		if !inJSONDataModel(member, depth) {
			return false
		}
	}
	return true
}

func inJSONDataModel(value any, depth int) bool {
	if depth > deepestNestingWalked {
		return false
	}
	switch candidate := value.(type) {
	case nil, bool, string:
		return true
	case float64:
		return !math.IsNaN(candidate) && !math.IsInf(candidate, 0)
	case []any:
		for _, element := range candidate {
			if !inJSONDataModel(element, depth+1) {
				return false
			}
		}
		return true
	case map[string]any:
		return allInJSONDataModel(candidate, depth+1)
	default:
		return false
	}
}

func traitInJSONDataModel(trait any) (any, bool) {
	if inJSONDataModel(trait, 0) {
		return trait, true
	}
	encoded, err := json.Marshal(trait)
	if err != nil {
		return nil, false
	}
	var value any
	if json.Unmarshal(encoded, &value) != nil {
		return nil, false
	}
	return value, true
}
