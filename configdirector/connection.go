package configdirector

import (
	"fmt"
	"io"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/connectionseam"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/transport"
)

const sdkName = "go-server-sdk"

type connection = io.Closer

func init() {
	connectionseam.NewClientOverConnection = newClientOverConnection
}

func newClientOverConnection(options Options, open connectionseam.OpenConnection) (*Client, error) {
	settings, err := settingsFrom(options)
	if err != nil {
		return nil, err
	}
	return newClient(settings, options.Hooks, func(deliver func(document string)) connection {
		return open(connectionseam.Receiver{Deliver: deliver, Logger: settings.logger})
	}), nil
}

func openTransport(settings clientSettings, deliver func(document string)) connection {
	options := transport.Options{
		SDKKey:  settings.sdkKey,
		BaseURL: settings.baseURL,
		MetaContext: transport.MetaContext{
			SDKName:    sdkName,
			SDKVersion: Version,
			AppName:    settings.metadata.AppName,
			AppVersion: settings.metadata.AppVersion,
		},
		HTTPClient: settings.httpClient,
		Logger:     settings.logger,
	}
	switch settings.mode {
	case ConnectionModeStreaming:
		return transport.OpenStreaming(options, deliver)
	case ConnectionModePolling:
		return transport.OpenPolling(options, settings.pollingInterval, deliver)
	}
	panic(fmt.Sprintf("configdirector: the connection mode %q passed validation without a transport", settings.mode))
}
