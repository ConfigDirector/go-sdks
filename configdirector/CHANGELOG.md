# Changelog

All notable changes to the `configdirector` module (`github.com/ConfigDirector/go-sdks/configdirector`)
are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module uses
[semantic versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged
`configdirector/v<version>`.

## [Unreleased]

## [0.5.0] - 2026-11-06

### Added

- `Track(eventName, userContext, options)` on the client reports to ConfigDirector that something
  happened, such as a completed checkout, so ConfigDirector can show it on the live feed and count it
  for each variation of a running experiment. Every argument is required: `UserContext{}` means no
  context and `TrackOptions{}` means no value or details. `TrackOptions.Value` is a number of any
  kind, such as an amount of money, a count or a duration: any integer or float kind, a `json.Number`,
  or a type whose `MarshalText` writes a JSON number, such as a `*big.Int`, a `*big.Float` or a decimal
  library's type, which is sent with every digit. `TrackOptions.Details` is a map whose values are
  strings, numbers, booleans, or nested maps and slices, and accepts anything else `encoding/json` can
  write. The new `TrackOptions`, `TrackedEventDetails` and `TrackedEventDetailValue` types are
  exported.
- An event counts for an experiment only when it is tracked for a context with an `ID`, the one the
  experiment's config was evaluated for. With `UserContext{}`, or for a context without an `ID`, the
  event is still reported. The `ID` of a context marked `Anonymous` is not sent. When a context's
  `Kind` is invalid, the SDK logs an error as it does for an evaluation and sends the event without a
  context.
- Tracked events are sent with the next telemetry batch, at most 1,000 per batch; events beyond that
  are not reported. An event whose name does not have 1 to 150 characters is not reported. Details
  with more than 20 top-level keys, larger than 1 KB as JSON, or holding a value `encoding/json` cannot
  write, and a `Value` that is not a finite number, are left out and the event is still reported. NaN
  and infinity in a `map[string]any` or a `[]any` inside details are sent as null. Each of these logs
  one warning per telemetry batch, except the null. `Track` never panics, and does nothing in
  `ConnectionModeOffline` or after `Close`.
- `TrackedEvents()` and `ClearTrackedEvents()` on the test client from `configdirectortest`:
  `TrackedEvents` returns the events the code under test tracked, in call order, as
  `TrackedEvent{Name, UserContext, Value, Details}`, and `ClearTrackedEvents` forgets them. An event
  whose name the SDK would not report is not recorded, and a value or details the SDK would leave out
  are missing from the record. The limit of 1,000 events per telemetry batch does not apply to the
  record, and `ReplaceValues` leaves it alone. The new `TrackedEvent` type is exported from
  `configdirectortest`.
- `Evaluate(key, defaultValue, userContext)` on the client reads a config as the type of
  `defaultValue` and returns the `Evaluation`: the value, the reason it was served, its value ID and
  its variation. `Value` holds the type of `defaultValue`, which is a `bool`, a `string`, any integer
  kind, a `float32` or `float64`, or a map, slice, array or struct for a JSON document. Like every
  read, it reaches `OnConfigEvaluated` handlers and is counted in telemetry. It returns an error for
  a blank key and for an in-code default value it cannot read, such as `nil`, a pointer, a channel
  or a func.
- `Evaluation.Variation` and `EvaluationDetails.Variation`, on `Evaluate`, `OnConfigEvaluated` and
  the `*ValueDetails` reads, and the new `Variation` type: the `ID` and `Name` of the variation that
  was served. It is nil when the config has no variations or the in-code default value is returned,
  and an unnamed variation has an empty `Name`. A test client and offline mode serve plain values,
  so it is nil on their reads.
- `ConfigState.Variation`, `RuleID` and `ShareID` on each `AllConfigs` entry: the variation served
  when the config has variations, and the rule and share of the percentage rollout that served the
  value. State you encode with `encoding/json` carries them too, under those field names.
- The evaluation counts the SDK sends to ConfigDirector now say which variation was served, and which
  percentage rollout and share served it, so ConfigDirector can count the contexts each variation of
  an experiment reached.

### Changed

- A context without an `ID` now skips percentage rollouts. Reads, `Evaluate`, watches and `AllConfigs`
  serve it the default targeting rule value, and the targeting rules after the rollout are not tried.
  This includes `UserContext{}` and a context ignored for an invalid `Kind`. `Evaluate`, the
  `*ValueDetails` reads and `OnConfigEvaluated` report it with the new reason
  `EvaluationReasonRolloutSkippedNoIdentifier` and `UsedDefault` false. Before, every context without
  an `ID` was served the rollout's first share wider than 0%. Give an anonymous context a stable `ID`,
  such as a session or device ID, to keep it in a rollout. A rule that serves a single value still
  applies to a context without an `ID`.
- `Evaluation`, `EvaluationDetails` and `ConfigState` have new fields, so code that builds one of them
  without field names no longer compiles; add the field names. `==` on two `EvaluationDetails` or
  two `ConfigState` values compares `Variation` as a pointer, so two reads of the same variation
  compare unequal; compare `Variation.ID` instead.

### Fixed

- A telemetry report larger than ConfigDirector accepts is now discarded with a warning, and the next
  report is still sent. Before, the SDK stopped sending telemetry until the process restarted.

## [0.4.0] - 2026-10-24

### Added

- `ConnectionModeOffline`, a connection mode in which the client never connects to ConfigDirector
  and serves the values in `ConnectionOptions.File`, the path of a JSON file with one value per
  config key, or `ConnectionOptions.Values`, a `map[string]any`, to every context. The values are
  typed as `configdirectortest.TestClient.SetValue` types them, the client is ready when `NewClient`
  returns, nothing is sent, and no SDK key is needed. With neither `File` nor `Values`, every read
  returns its in-code default value. `NewClient` returns an error when the file is missing, cannot
  be read, is not a JSON object or holds a `null`, when an inline value is `nil` or of a kind that
  cannot be served, when `File` and `Values` are both given, or when either is given in streaming
  or polling mode. In offline mode `PollingInterval`, `Timeout`, `URL` and `HTTPClient` are ignored.

### Changed

- The errors `configdirectortest.TestClient.SetValue` and `ReplaceValues` return for a value that
  cannot be served, and the panic of `NewTestClient`, now start with `configdirector:` instead of
  `configdirectortest:`, and the message for a `nil` value no longer suggests `RemoveValue`, since
  the same rule now refuses a `null` in an offline values file.
- A blank SDK key is reported after an unknown connection mode, not before it, when both are wrong.

## [0.3.0] - 2026-10-14

### Added

- `UserContext.Kind`, the kind of entity the context represents, such as `device`,
  `organization` or `tenant`; leaving it empty means `user`. A kind has 1 to 64 characters, using
  only lowercase letters, digits, `-` and `_`, and is used exactly as given. Targeting rules and
  segments can match on a context's kind, and percentage rollouts give contexts of other kinds
  their own assignments while `user` contexts keep the ones they have today. ConfigDirector keeps
  a context per kind and identifier, so user 42 and device 42 are separate contexts.
- A context with an invalid kind never makes the client panic: every read, `AllConfigs` and every
  watch evaluate exactly as if `UserContext{}` had been passed, and the `Evaluation` handed to
  `OnConfigEvaluated` carries `UserContext{}`. The client logs an error for each distinct invalid
  kind once per telemetry flush interval, and a `configdirectortest` client logs every such read.
  ConfigDirector receives a count of these reads with a sample of each invalid kind, never the
  context's identifier, name or traits.

### Fixed

- When a full update carries a segment the client cannot read, the client now keeps the definition
  of that segment it already holds, as the warning it logs says and as it already did for configs
  and in delta updates. Before, it dropped the segment, and until a later update carried it in a
  readable form, a targeting rule using it matched no context, for `in` and `not in` conditions
  alike: every context the rule should have matched got the next rule's value or the default
  targeting rule value instead.

## [0.2.1] - 2026-10-10

### Fixed

- golangci-lint's `contextcheck` no longer reports a config read in code that has a request or a
  `context.Context` at hand, such as a `net/http` handler, as a call that should pass that
  context. Reads still take no context.

## [0.2.0] - 2026-10-10

### Added

- `wrapper`, a package for the modules ConfigDirector ships on top of this SDK.
  `wrapper.NewClient(module, version, options)` creates a client that reports the module's name
  and version to ConfigDirector in the SDK's place, from a closed list of modules;
  `wrapper.OpenFeatureProvider` is the OpenFeature provider. Applications keep creating their
  client with `NewClient`.

### Fixed

- A trait of any Go type now matches targeting rules the way the same value sent as JSON does: an
  `int`, `float32` or `json.Number` trait satisfies number conditions and text conditions on its
  decimal text, a `[]string` trait satisfies `contains any of`, a JSON pointer reaches into a typed
  map or a struct by its JSON field names, and a `time.Time` trait satisfies datetime conditions.
  Before, only `string`, `bool`, `float64`, `[]any` and `map[string]any` traits were read, so
  `Traits: map[string]any{"score": 92}` matched no rule while the dashboard showed `score: 92`.
  A trait `encoding/json` cannot encode, such as a channel, matches no condition, and the other
  traits in the context still match.

## [0.1.0] - 2026-10-10

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
- Streaming and polling connection modes. `ConnectionOptions.Mode` picks one; streaming is the
  default and keeps an event stream open, reconnecting with backoff when it drops; polling asks
  for config state on `ConnectionOptions.PollingInterval`. Both read full and delta server SDK
  payloads, segments included, and ignore fields they do not know. A config the client cannot read
  is skipped with a warning and the definition it already holds, if any, is kept.
- `ConnectionOptions.URL` routes every request through a proxy, a sub-path included, and
  `ConnectionOptions.HTTPClient` sends them.
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
- `configdirectortest`, a package for testing code that takes a `*Client` without a server.
  `NewTestClient(values, options...)` wires the real client to an in-memory connection the test
  controls, with no network, no telemetry and no goroutine; the config type follows from each Go
  value. `SetValue`, `RemoveValue` and `ReplaceValues` deliver their update on the calling
  goroutine before they return, so watches and `OnConfigsUpdated` handlers have run; a change made
  from inside a handler is delivered after the update being handled.
  `HoldInitialization()` keeps the client not ready until `CompleteInitialization()`, or
  `FailInitialization()` fails it as a refused SDK key would; `WithTimeout`, `WithLogger` and
  `WithHooks` adjust the client.
- Telemetry. Every read, and every watch that fires, is reported to ConfigDirector as an
  evaluated-config event, and every `UserContext` with an `ID` that is not `Anonymous` as a
  captured context; `AllConfigs` reports nothing. Identical events in a batch are aggregated with
  a count. A report is sent 5 seconds after the client is created, then every
  `TelemetryOptions.FlushInterval`, and once more on `Close`; nothing is sent when nothing was
  evaluated. `TelemetryOptions.EventQueueLimit` bounds what is held between reports, 70% for
  evaluations and 30% for distinct contexts; when a share is full the oldest entry is dropped and
  the drops are counted in the next report. A refusal the SDK server would repeat, a 4xx status
  other than 429, stops telemetry for good with one error log. The requested type is the Go type
  the read asked for (`bool`, `string`, `int64`, `float64`, or the target's type for `JSONValue`);
  JSON documents and values longer than 500 characters travel as a value id. A
  `configdirectortest` client sends no telemetry.

### Security

- The SDK key appears in no log record: a refusal whose body echoes it is logged with the key
  redacted, and a refusal's body is logged up to 1 KiB.
