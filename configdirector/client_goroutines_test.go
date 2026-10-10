package configdirector_test

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func sdkGoroutines() int {
	buffer := make([]byte, 1<<16)
	for {
		written := runtime.Stack(buffer, true)
		if written < len(buffer) {
			count := 0
			for goroutine := range strings.SplitSeq(string(buffer[:written]), "\n\n") {
				if strings.Contains(goroutine, "go-sdks/configdirector.") || strings.Contains(goroutine, "go-sdks/configdirector/internal/") {
					count++
				}
			}
			return count
		}
		buffer = make([]byte, 2*len(buffer))
	}
}

func requireNoSDKGoroutine(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return sdkGoroutines() == 0 }, 200*time.Millisecond, time.Millisecond)
}

func TestNoGoroutineOutlivesCloseAfterAStreamingSession(t *testing.T) {
	server := startSDKServer(t, standardPayload().json())
	client := newClientAgainst(t, server)
	waitForReady(t, client)
	server.send(deltaPayload(textConfig("greeting", "Hi")).json())
	require.Eventually(t, func() bool { return client.StringValue("greeting", "", noUser) == "Hi" }, awaitTimeout, 10*time.Millisecond)
	require.Positive(t, sdkGoroutines())

	require.NoError(t, client.Close())

	requireNoSDKGoroutine(t)
}

func TestNoGoroutineOutlivesCloseAfterAPollingSession(t *testing.T) {
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	client := newClient(t, options)
	waitForReady(t, client)
	require.Positive(t, sdkGoroutines())

	require.NoError(t, client.Close())

	requireNoSDKGoroutine(t)
}

func TestNoGoroutineOutlivesCloseAfterATelemetryFlush(t *testing.T) {
	server := startSDKServer(t, standardPayload().json())
	client := newClientAgainst(t, server)
	waitForReady(t, client)
	client.BoolValue("new-checkout", false, identifiedUser)
	server.telemetryReportWithin(t, firstScheduledFlushTimeout)
	require.Positive(t, sdkGoroutines())

	require.NoError(t, client.Close())

	requireNoSDKGoroutine(t)
}
