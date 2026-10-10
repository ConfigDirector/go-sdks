package configdirector

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
	"github.com/stretchr/testify/require"
)

type wireObjects struct {
	Kind     string                     `json:"kind"`
	Configs  map[string]json.RawMessage `json:"configs"`
	Segments map[string]json.RawMessage `json:"segments"`
}

// FuzzParseServerPayload checks that parsing a message never panics, fails exactly when the
// message is not a JSON object with a configs object and a kind that is text when present,
// accounts for every config and segment of the message as read or unreadable, and that what it
// read can be held and evaluated.
func FuzzParseServerPayload(f *testing.F) {
	f.Add(`{"payloadVersion":1,"kind":"full","configs":{},"segments":{}}`)
	f.Add(`{"configs":{"a":{"id":"c-a","key":"a","type":"string","target":{"rules":[],"defaultValue":"x","defaultValueId":"v"}}}}`)
	f.Add(`{"kind":"delta","payloadVersion":"2","configs":{"a":{"id":"c-a","key":"a","type":"boolean","target":{"rules":[{"id":"r","order":0,"type":"conditional","target":"value","value":"true","valueId":"v","conditions":[{"id":"c","kind":"segment","operator":"in","segmentId":"s"}],"percentages":null}],"defaultValue":"false","defaultValueId":"d"}}},"segments":{"s":{"groups":[[{"id":"g","kind":"attribute","attribute":"traits","trait":"/plan","operator":"=","targetType":"text","targetValues":["beta"]}]]}}}`)
	f.Add(`{"configs":{"a":{"id":"c-a","key":"a","type":"integer","target":{"rules":[{"id":"r","type":"conditional","target":"percentage","percentages":[{"id":"p","percentage":50,"value":1,"valueId":"v"},{"id":"q","percentage":50,"value":2.5,"valueId":"w"}],"conditions":[{"id":"c","attribute":"identifier","operator":"=","targetType":"text","targetValues":["u1"]}]}],"defaultValue":{"nested":true},"defaultValueId":"d"}}}}`)
	f.Add(`{"configs":{"broken":{"id":"c","key":"broken"},"a":{"id":"c-a","key":"a","type":"string","target":{"rules":[{"id":"","conditions":[]}]}}},"segments":{"s":{},"t":{"groups":[[{"kind":"segment","operator":"in","segmentId":"s"}]]}}}`)
	f.Add(`{"configs":null}`)
	f.Add(`{"configs":[]}`)
	f.Add(`[]`)
	f.Add(`{not json`)

	f.Fuzz(func(t *testing.T, document string) {
		payload, err := parseServerPayload(document)

		var wire wireObjects
		wireErr := json.Unmarshal([]byte(document), &wire)
		if err != nil {
			require.True(t, wireErr != nil || wire.Configs == nil, "the parser failed on a message with a configs object and a text kind")
			return
		}
		require.NoError(t, wireErr)
		require.NotNil(t, wire.Configs)
		readableKeys := slices.Sorted(maps.Keys(payload.configs))
		require.ElementsMatch(t, slices.Sorted(maps.Keys(wire.Configs)), slices.Concat(readableKeys, payload.unreadableConfigKeys()))
		unreadableSegmentIDs := make([]string, 0, len(payload.unreadableSegments))
		for _, segment := range payload.unreadableSegments {
			unreadableSegmentIDs = append(unreadableSegmentIDs, segment.id)
		}
		require.ElementsMatch(t, slices.Sorted(maps.Keys(wire.Segments)), slices.Concat(slices.Sorted(maps.Keys(payload.segments)), unreadableSegmentIDs))
		for _, definition := range payload.configs {
			require.NotEmpty(t, definition.id)
			require.NotEmpty(t, definition.key)
			require.NotEmpty(t, definition.configType)
		}
		snapshot := (*configSnapshot)(nil).withPayload(payload)
		update := snapshot.changesFrom(nil, payload)
		require.ElementsMatch(t, readableKeys, update.Keys)
		require.Empty(t, update.RemovedKeys)
		states := snapshot.statesOf(snapshot.keys(), evaluation.Context{Identifier: "u1", Traits: map[string]any{"plan": "beta"}})
		require.Len(t, states, len(payload.configs))
	})
}
