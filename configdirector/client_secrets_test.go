package configdirector_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func requireNoRecordMentions(t *testing.T, records []slog.Record, secret string) {
	t.Helper()
	require.NotEmpty(t, records)
	for _, record := range records {
		var rendered bytes.Buffer
		require.NoError(t, slog.NewTextHandler(&rendered, nil).Handle(context.Background(), record))
		require.NotContains(t, rendered.String(), secret)
	}
}

func urlNothingListensOn(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.Close())
	return "http://" + listener.Addr().String()
}

func TestTheSDKKeyIsNotLoggedWhenConnectingFails(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	client := newClient(t, configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{URL: urlNothingListensOn(t), Mode: configdirector.ConnectionModePolling, Timeout: 50 * time.Millisecond},
		Logger:     logger,
	})

	err := client.WaitForReady(context.Background())

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Eventually(t, func() bool { return len(recordsAt(slog.LevelError, records())) == 1 }, awaitTimeout, 10*time.Millisecond)
	requireNoRecordMentions(t, records(), testSDKKey)
}

func TestTheSDKKeyIsNotLoggedWhenTheSDKServerRefusesItAndEchoesIt(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.refuseNextRequestWith(http.StatusUnauthorized, `{"error":"`+testSDKKey+` is not a server SDK key"}`)
	logger, records := newLogger()
	options := server.options()
	options.Logger = logger

	newClient(t, options)

	await(t, server.requests)
	require.Eventually(t, func() bool { return len(recordsAt(slog.LevelError, records())) == 1 }, awaitTimeout, 10*time.Millisecond)
	requireNoRecordMentions(t, records(), testSDKKey)
}

func TestTheSDKKeyIsNotLoggedWhenATelemetryReportCannotBeSent(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	logger, records := newLogger()
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)
	client.BoolValue("new-checkout", false, identifiedUser)
	server.Close()

	require.NoError(t, client.Close())

	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	requireNoRecordMentions(t, records(), testSDKKey)
}
