# Changelog

All notable changes to the `configdirector` module (`github.com/ConfigDirector/go-sdks/configdirector`)
are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module uses
[semantic versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged
`configdirector/v<version>`.

## [Unreleased]

### Added

- `NewClient(Options)` creates a `Client` from an options struct: SDK key, application `Metadata`,
  `ConnectionOptions` (streaming or polling mode, polling interval with a 1 minute minimum,
  readiness timeout, SDK server URL, HTTP client), `TelemetryOptions`, an `*slog.Logger` and
  `Hooks`. Zero values are defaults; invalid options are returned as an error. The client connects
  in the background.
- `Client.WaitForReady(ctx)`, `Client.Ready()`, `Client.Closed()` and `Client.Close()`.
  `WaitForReady` returns the context's error, `context.DeadlineExceeded` once the connection
  timeout elapses, or `ErrClientClosed`; the client keeps connecting either way.
- `UserContext` for targeting rule evaluation, with `ID`, `Name`, `Traits` and `Anonymous`.
- Typed reads `BoolValue`, `StringValue`, `IntValue` (`int64`), `FloatValue` (`float64`) and
  `JSONValue`, which decodes into a pointer target and leaves it untouched when it falls back,
  each with a `*ValueDetails` twin returning `EvaluationDetails` (`Reason`, `ValueID`,
  `UsedDefault`). Every read returns the in-code default value with an `EvaluationReason` when the
  config is missing, the client is not ready or closed, or the value cannot be read as the
  requested type.
- `Client.AllConfigs(userContext, keys...)` returning the evaluated `ConfigState` of every config,
  or of the given keys, sorted by key.
- The connection is a built-in stand-in that serves a fixed set of configs until the transports
  land.
- Events. `Options.Hooks` takes `OnReady`, `OnConfigsUpdated` and `OnConfigEvaluated` handlers at
  construction, so the ready event cannot be missed; `Client.OnReady`, `Client.OnConfigsUpdated`
  and `Client.OnConfigEvaluated` register the same afterwards and return an unsubscribe func. An
  `OnReady` handler registered once the client is ready is called at once. `ConfigsUpdated`
  carries the `Keys` an update touched, the configs whose targeting rules use a segment it
  carried included, and the `RemovedKeys` a full update dropped. `Evaluation` (`Key`, `Value`,
  `UsedDefault`, `Reason`, `ValueID`, `UserContext`) describes every read and every watch that
  fires.
- Watches. `Client.WatchBool`, `WatchString`, `WatchInt`, `WatchFloat` and `WatchJSON` call a
  handler with the newly evaluated value of one key for one user context whenever an update
  carries the key or a segment its rules use, and with the in-code default value when a full
  update drops the key. Each returns an unsubscribe func; `UnwatchAll` cancels every watch.
- Handlers run on the connection's goroutine. A panic in a handler is recovered and logged at
  error level, and the other handlers and the connection continue. `Close` cancels every handler
  and watch.
