package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
)

type environmentSettings struct {
	options  configdirector.Options
	logLevel slog.Level
}

func settingsFromEnvironment() (environmentSettings, error) {
	timeout, err := timeoutFromEnvironment()
	if err != nil {
		return environmentSettings{}, err
	}
	logLevel, err := logLevelFromEnvironment()
	if err != nil {
		return environmentSettings{}, err
	}

	return environmentSettings{
		options: configdirector.Options{
			SDKKey:   environmentValueOr("CONFIGDIRECTOR_SERVER_KEY", "fake-sample-key"),
			Metadata: configdirector.Metadata{AppName: "gin-sample", AppVersion: "1.0.0"},
			Connection: configdirector.ConnectionOptions{
				Mode:    configdirector.ConnectionMode(os.Getenv("CONFIGDIRECTOR_MODE")),
				Timeout: timeout,
				URL:     os.Getenv("CONFIGDIRECTOR_BASE_URL"),
			},
		},
		logLevel: logLevel,
	}, nil
}

func timeoutFromEnvironment() (time.Duration, error) {
	configured := os.Getenv("CONFIGDIRECTOR_TIMEOUT")
	if configured == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(configured)
	if err != nil {
		return 0, fmt.Errorf("CONFIGDIRECTOR_TIMEOUT %q is not a duration such as 3s or 1500ms", configured)
	}
	return timeout, nil
}

func logLevelFromEnvironment() (slog.Level, error) {
	configured := os.Getenv("CONFIGDIRECTOR_LOG_LEVEL")
	if configured == "" {
		return slog.LevelInfo, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(configured)); err != nil {
		return 0, fmt.Errorf("CONFIGDIRECTOR_LOG_LEVEL %q is not one of DEBUG, INFO, WARN or ERROR", configured)
	}
	return level, nil
}

func environmentValueOr(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
