package configdirector

import (
	"fmt"
	"io"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/connectionseam"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/identityseam"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/servedvalues"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/transport"
)

var ownIdentity = identityseam.Identity{SDKName: "go-server-sdk", SDKVersion: Version}

type connection = io.Closer

func init() {
	connectionseam.NewClientOverConnection = newClientOverConnection
	identityseam.NewClientIdentifiedAs = newClientIdentifiedAs
}

func newClientIdentifiedAs(identity identityseam.Identity, options Options) (*Client, error) {
	settings, err := settingsFrom(options)
	if err != nil {
		return nil, err
	}
	settings.identity = identity
	switch settings.mode.configStateSource() {
	case configStateFromConfigDirector:
		return newClient(settings, options.Hooks, func(deliver func(document string)) connection {
			return openTransport(settings, deliver)
		}, startTelemetry(settings)), nil
	case configStateFromOfflineValues:
		return newClientServingOfflineValues(settings, options.Hooks), nil
	}
	panic(fmt.Sprintf("configdirector: the connection mode %q passed validation without a client", settings.mode))
}

func newClientServingOfflineValues(settings clientSettings, hooks Hooks) *Client {
	offlineConnection := servedvalues.NewConnection(servedvalues.OfflineValues, settings.offlineConfigs)
	client := newClient(settings, hooks, func(deliver func(document string)) connection {
		return offlineConnection.Open(connectionseam.Receiver{Deliver: deliver, Logger: settings.logger})
	}, telemetry.Discarding{})
	offlineConnection.CompleteInitialization()
	return client
}

func newClientOverConnection(options Options, open connectionseam.OpenConnection) (*Client, error) {
	settings, err := settingsFrom(options)
	if err != nil {
		return nil, err
	}
	return newClient(settings, options.Hooks, func(deliver func(document string)) connection {
		return open(connectionseam.Receiver{Deliver: deliver, Logger: settings.logger})
	}, telemetry.Discarding{}), nil
}

func (settings clientSettings) transportOptions() transport.Options {
	return transport.Options{
		SDKKey:  settings.sdkKey,
		BaseURL: settings.baseURL,
		MetaContext: transport.MetaContext{
			SDKName:    settings.identity.SDKName,
			SDKVersion: settings.identity.SDKVersion,
			AppName:    settings.metadata.AppName,
			AppVersion: settings.metadata.AppVersion,
		},
		HTTPClient: settings.httpClient,
		Logger:     settings.logger,
	}
}

func openTransport(settings clientSettings, deliver func(document string)) connection {
	options := settings.transportOptions()
	switch settings.mode {
	case ConnectionModeStreaming:
		return transport.OpenStreaming(options, deliver)
	case ConnectionModePolling:
		return transport.OpenPolling(options, settings.pollingInterval, deliver)
	case ConnectionModeOffline:
		panic("configdirector: a client in offline mode has no transport to open")
	}
	panic(fmt.Sprintf("configdirector: the connection mode %q passed validation without a transport", settings.mode))
}
