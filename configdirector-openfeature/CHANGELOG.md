# Changelog

All notable changes to the `configdirector-openfeature` module
(`github.com/ConfigDirector/go-sdks/configdirector-openfeature`) are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module uses
[semantic versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged
`configdirector-openfeature/v<version>`.

## [Unreleased]

## [0.5.1] - 2026-11-07

### Fixed

- A context without a targeting key or `id` is now served by the targeting rules after a percentage
  rollout it skipped, with the reason `TARGETING_MATCH` when one of them applies. The default
  targeting rule value, with the reason `DEFAULT`, is served only when no rule after the skipped
  rollout applies. Since 0.5.0, the skipped rollout ended the evaluation and the rules after it were
  not tried. The fix is in `configdirector` v0.5.1, which this release requires.

## [0.5.0] - 2026-11-06

### Added

- The provider now implements OpenFeature's tracking. `client.Track(ctx, eventName, evaluationContext,
  details)` reports to ConfigDirector that something happened, such as a completed checkout, so
  ConfigDirector can show it on the live feed and count it for each variation of a running experiment.
  Before, a `Track` call did nothing. The details' value is sent as the event's value, their attributes
  as the event's details, and the targeting key (or `id` attribute) and `kind` of the merged evaluation
  context as the context the event happened for. A value of 0 is sent as no value, because OpenFeature
  Go has no way to say an event has none: attributes can only be added to details made with
  `openfeature.NewTrackingEventDetails(0)`.
- An event counts for an experiment only when its evaluation context has the targeting key the
  experiment's flag was evaluated with. Without one, the event is still reported. The targeting key of
  a context marked `anonymous` is not sent.
- Tracked events are sent with the next telemetry batch, at most 1,000 per batch; events beyond that
  are not reported. An event whose name does not have 1 to 150 characters is not reported. Attributes
  with more than 20 keys, larger than 1 KB as JSON, or holding a value `encoding/json` cannot write, and
  a value that is not a finite number, are left out and the event is still reported. Each of these logs
  one warning per telemetry batch. `Track` never panics, and does nothing in
  `configdirector.ConnectionModeOffline` or after `Shutdown`.
- The evaluation counts the provider sends to ConfigDirector now say which variation was served, and
  which percentage rollout and share served it, so ConfigDirector can count the contexts each
  variation of an experiment reached.

### Changed

- The `Variant` in evaluation details is now the variation's name when the config has variations and
  the variation served has a name, such as `treatment`. Before, it was always the value ID. It stays
  the value ID for a config without variations and for an unnamed variation. If you send `Variant` to
  an analytics tool, an experiment's evaluations now show up under the variation names.
- An evaluation context with neither a targeting key nor an `id` attribute now skips percentage
  rollouts. The flag resolves to the default targeting rule value with the reason `DEFAULT`, and the
  targeting rules after the rollout are not tried. Before, every such context was served the rollout's
  first share wider than 0%. Give an anonymous context a stable targeting key, such as a session ID,
  to keep it in a rollout. A rule that serves a single value still applies to a context without one.
- Requires `configdirector` 0.5.0 or later, which adds tracked events and variations.

### Fixed

- A telemetry report larger than ConfigDirector accepts is now discarded with a warning, and the next
  report is still sent. Before, the provider stopped sending telemetry until the process restarted.

## [0.4.0] - 2026-10-28

### Changed

- When no config arrives within the connection timeout, initialization now fails:
  `SetProviderAndWait` and `SetProviderWithContextAndWait` return an error, and the provider
  reports `ErrorState` until config arrives, then `ReadyState`. Before, it reported `ReadyState`
  while serving in-code default values, then ready again once config arrived. To start without
  waiting for ConfigDirector, call `SetProvider` instead, or handle the error.

### Fixed

- An object evaluation whose in-code default value is a map or a struct now resolves to the
  config's document only when that document is a JSON object, and one whose in-code default value
  is a slice or an array only when it is a JSON array. Any other document, such as `42`, `"text"`,
  `true` or `null`, resolves to the in-code default value as given with `TYPE_MISMATCH`, so error
  hooks run. Before, it reached the application as the flag's value, a `float64(42)` or a `nil`,
  with `TARGETING_MATCH`. With an in-code default value of any other kind, the flag still resolves
  to whichever document the config holds.

## [0.3.0] - 2026-10-24

### Added

- Offline mode: created with the connection `Mode` set to `configdirector.ConnectionModeOffline`
  and no SDK key, the provider never connects to ConfigDirector and sends nothing, serves the
  values in `ConnectionOptions.File` or `ConnectionOptions.Values` to every evaluation context, and
  `SetProviderAndWait` returns at once with the provider ready. `NewProvider` returns the error
  `configdirector.NewClient` returns when those values cannot be loaded.

### Changed

- Requires `configdirector` 0.4.0 or later, which adds offline mode.

## [0.2.0] - 2026-10-15

### Added

- A `kind` attribute in the evaluation context, beside the targeting key, becomes the
  `UserContext` kind, for example `openfeature.NewEvaluationContext("42", map[string]any{"kind":
  "device"})`; leaving it out means `user`. A `kind` inside `traits` stays an ordinary trait. A
  string, number or boolean `kind` is read as its text, like `id`. When the kind is invalid the
  whole context is ignored: the evaluation still resolves successfully, to the value for no
  context, and the SDK logs an error.

### Changed

- Requires `configdirector` 0.3.0 or later, which accepts a context kind.

## [0.1.1] - 2026-10-11

### Changed

- Requires `configdirector` 0.2.1 or later.

## [0.1.0] - 2026-10-10

### Added

- `NewProvider(options)`, an OpenFeature provider over the ConfigDirector Go server SDK. It takes
  the SDK's `configdirector.Options`, connects at once, and reports
  `go-openfeature-server-provider` and its `Version` to ConfigDirector. Register it with
  `openfeature.SetProviderAndWait`: it waits for the first config state for the connection
  timeout, and initialization completes whether or not it arrived. Handlers in `Options.Hooks`
  run after the provider's own.
- Boolean, string, int, float and object evaluations through the OpenFeature client. An evaluated
  value resolves with the reason `TARGETING_MATCH` and the value's id as the variant; a config
  without a value with `DEFAULT`; a missing config with the error code `FLAG_NOT_FOUND`; a value
  that does not match the requested type with `TYPE_MISMATCH`; and a client without config state
  yet with `PROVIDER_NOT_READY`. An object evaluation decodes the JSON document as `encoding/json`
  decodes into an `any`.
- The evaluation context's targeting key, or failing that its `id` attribute, becomes the
  `UserContext` id; a `name` attribute, a `traits` map and a boolean `anonymous` attribute become
  the matching fields.
- Events: `PROVIDER_CONFIGURATION_CHANGED` on every config state update, naming the flags the
  update carried followed by the flags that were removed, and `PROVIDER_READY` when config
  state arrives after initialization completed without it.
- Requires `configdirector` 0.2.0 or later, which adds the `wrapper` package the provider
  identifies itself through.
