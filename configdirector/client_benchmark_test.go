package configdirector_test

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
)

func BenchmarkBoolValue(b *testing.B) {
	server := startSDKServer(b, standardPayload().json())
	client := newClientAgainst(b, server)
	waitForReady(b, client)
	user := configdirector.UserContext{ID: "u-1", Traits: map[string]any{"plan": "pro"}}

	b.ReportAllocs()
	for b.Loop() {
		client.BoolValue("new-checkout", false, user)
	}
}
