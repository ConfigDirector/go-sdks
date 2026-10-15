# Changelog

All notable changes to the `configdirector-openfeature` module
(`github.com/ConfigDirector/go-sdks/configdirector-openfeature`) are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module uses
[semantic versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged
`configdirector-openfeature/v<version>`.

## [Unreleased]

## [0.2.0] - 2026-10-14

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
