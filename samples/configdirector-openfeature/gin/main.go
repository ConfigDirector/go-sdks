// A Gin app that reads a handful of ConfigDirector configs through OpenFeature for the user a
// request names and answers GET /configs with their values.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	configdirectoropenfeature "github.com/ConfigDirector/go-sdks/configdirector-openfeature"
	"github.com/joho/godotenv"
	"github.com/open-feature/go-sdk/openfeature"
)

const (
	listenAddress     = ":3610"
	clientDomain      = "openfeature-gin-sample"
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the .env file cannot be read: %w", err)
	}
	settings, err := settingsFromEnvironment()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: settings.logLevel}))
	settings.options.Logger = logger
	settings.options.Hooks = configdirector.Hooks{OnReady: func() { logger.Info("ConfigDirector is ready") }}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provider, err := configdirectoropenfeature.NewProvider(settings.options)
	if err != nil {
		return err
	}
	client := openfeature.NewClient(clientDomain)
	logChangedFlags := func(details openfeature.EventDetails) {
		logger.Info("Configs updated", slog.Any("flags", details.FlagChanges))
	}
	client.AddHandler(openfeature.ProviderConfigChange, &logChangedFlags)

	defer openfeature.Shutdown()
	if err := openfeature.SetProviderWithContextAndWait(ctx, provider); err != nil {
		if _, noConfigInTime := errors.AsType[*openfeature.ProviderInitError](err); !noConfigInTime {
			return fmt.Errorf("the ConfigDirector provider could not be registered: %w", err)
		}
		logger.Warn("Starting on in-code default values until ConfigDirector sends config", slog.Any("cause", err))
	}
	logger.Info("The ConfigDirector provider is registered")

	return serve(ctx, newRouter(client), logger)
}

func serve(ctx context.Context, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{Addr: listenAddress, Handler: handler, ReadHeaderTimeout: readHeaderTimeout}
	listenFailed := make(chan error, 1)
	go func() {
		logger.Info("Listening", slog.String("address", listenAddress))
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			listenFailed <- err
		}
	}()

	select {
	case err := <-listenFailed:
		return err
	case <-ctx.Done():
	}

	logger.Info("Shutting down")
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownContext)
}
