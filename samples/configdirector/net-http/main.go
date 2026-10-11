// A standard-library net/http app that evaluates a handful of ConfigDirector configs for the user
// a request names and answers GET /configs with their values.
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
	"github.com/joho/godotenv"
)

const (
	listenAddress     = ":3601"
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

	client, err := configdirector.NewClient(settings.options)
	if err != nil {
		return err
	}
	defer closeClient(client, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.WaitForReady(ctx); err != nil {
		logger.Warn("ConfigDirector is not ready; every config resolves to its in-code default value until it is", slog.Any("error", err))
	} else {
		logger.Info("ConfigDirector is ready")
	}

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

func closeClient(client *configdirector.Client, logger *slog.Logger) {
	if err := client.Close(); err != nil {
		logger.Error("The ConfigDirector client did not close cleanly", slog.Any("error", err))
	}
}
