# Gin sample

A minimal [Gin](https://gin-gonic.com) app reading ConfigDirector configs through
[OpenFeature](https://openfeature.dev), using the ConfigDirector OpenFeature Go server provider.
It is the OpenFeature counterpart of the [server SDK's Gin sample](../../configdirector/gin/): the
same `/configs` route evaluating the same five configs for the user a request names, plus
`/configs/<key>` with the evaluation details of one boolean flag.

## Running it

```bash
cd samples/configdirector-openfeature/gin
cp .env.example .env      # optional, the sample runs without a real key
GOWORK=off go run .
```

`GOWORK=off` keeps this repository's root workspace, which the sample is deliberately not part
of, out of the way. A copy of the sample outside the repository runs with a plain `go run .`.

Then:

```bash
curl 'http://localhost:3610/configs?id=user-123&plan=pro'
```

```json
{
  "day-of-the-week-config": "Friday",
  "integer-config": 10,
  "json-value-config": {},
  "permanent-kill-switch": false,
  "temporary-feature-flag": true
}
```

Query parameters double as the evaluation context: `id` becomes the targeting key, `name` and
`anonymous` become the attributes of the same name, and anything else becomes a trait:

```
/configs?id=user-123&name=Ada&plan=pro&region=eu
```

`/configs/<key>` returns the full evaluation details of one boolean flag, which is the quickest
way to see why a value was served:

```bash
curl 'http://localhost:3610/configs/temporary-feature-flag?id=user-123'
```

```json
{
  "value": false,
  "variant": "",
  "reason": "ERROR",
  "errorCode": "PROVIDER_NOT_READY",
  "errorMessage": "The ConfigDirector client has not received any configs yet."
}
```

Stop it with Ctrl-C or `SIGTERM`; the app finishes in-flight requests and shuts OpenFeature down.

Run the tests with `GOWORK=off go test ./...`.

## One provider per process

This is the single most important thing the sample shows. [`main.go`](main.go) creates one
provider at startup, registers it with OpenFeature and waits for its first config state, builds
one OpenFeature client that every request handler shares, and shuts OpenFeature down on exit.

```go
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

return serve(ctx, newRouter(client), logger)
```

Never create a provider inside a request handler. Each provider owns a ConfigDirector client,
which opens its own connection to ConfigDirector, starts out without config state, and keeps
connecting in the background for as long as it lives. Every request handler uses the one client
built at startup.

Evaluation itself is cheap. Targeting rules are evaluated locally against config state the
provider already holds in memory, with no network call on the request path, which is what makes
it safe to read several configs per request.

## What else it demonstrates

**Initialization is explicit and non-fatal.** `SetProviderWithContextAndWait` returns once the
first config state arrives or the connection timeout elapses; Ctrl-C during the wait stops the
app instead. When no config state arrives in time, the call returns an error wrapping a
`*openfeature.ProviderInitError`, and the app logs `Starting on in-code default values until
ConfigDirector sends config` and starts anyway. OpenFeature reports `ErrorState`, every read
answers the in-code default value it passes in, and the details route says why: the error code
`PROVIDER_NOT_READY`. The provider keeps connecting, and the app logs `ConfigDirector is ready`
when config state arrives, from the `OnReady` handler in `Options.Hooks`, which the provider
passes on to its client; OpenFeature then reports `ReadyState`. Any other registration error
stops the app, and the deferred `openfeature.Shutdown` shuts the provider down however `run`
returns.

**In-code default values are the fallback.** Every read passes the value to serve when
ConfigDirector is unreachable, so it should be the safe choice. The method decides the type the
config value is read as: `Boolean`, `Int`, `String`, and `Object` for a JSON document.

**Details explain a value.** `/configs/<key>` in [`configs.go`](configs.go) reads one flag with
`BooleanValueDetails` and returns its value, variant, reason, error code and error message. A
value ConfigDirector evaluated has the reason `TARGETING_MATCH` and the value's id as its variant;
a flag no config has answers `FLAG_NOT_FOUND`.

**Events are yours to handle.** The client's `PROVIDER_CONFIGURATION_CHANGED` handler logs the
keys each config state update carried or removed. Register handlers before setting the provider,
so the first update cannot be missed.

**The evaluation context is per request; the provider is not.** `evaluationContextFrom` in
[`configs.go`](configs.go) maps query parameters onto an OpenFeature `EvaluationContext`; a real
app would build this from the authenticated session.

**Logging is yours to configure.** The sample builds a `slog.Logger` at the level
`CONFIGDIRECTOR_LOG_LEVEL` names and passes it in `Options.Logger`, so the provider's records land
in the application's own log tagged `component=configdirector-openfeature`, and the SDK client's
tagged `component=configdirector`. Set `CONFIGDIRECTOR_LOG_LEVEL=DEBUG` to see every config
update the client receives.

**Settings come from the environment.** [`settings.go`](settings.go) reads the SDK key,
connection mode, timeout and base URL from `CONFIGDIRECTOR_*` variables, the same ones the server
SDK's sample reads, with a `.env` file loaded at startup for local runs. A zero value in
`Options` is the SDK's default, so the sample only sets what the environment names.

**Shutdown is clean.** `SIGTERM` and Ctrl-C stop the HTTP server, let in-flight requests finish,
and shut OpenFeature down, which closes the provider and its connection.

## Testing

Code that reads flags through OpenFeature is tested by swapping the provider, so nothing from
ConfigDirector is involved and no network is used. [`main_test.go`](main_test.go) registers the
OpenFeature Go SDK's in-memory provider, `memprovider`, with the flags each test serves, builds
the router over an OpenFeature client the way `main` does, and drives it with `httptest`:

```go
require.NoError(t, openfeature.SetProviderAndWait(memprovider.NewInMemoryProvider(flags)))
t.Cleanup(openfeature.Shutdown)
router := newRouter(openfeature.NewClient(clientDomain))

response := get(t, router, "/configs?id=user-123&plan=pro")
```

A `ContextEvaluator` on every flag records the evaluation context each read carried, which is how
the test proves that the query string reaches the evaluation, traits included.

OpenFeature is one global, so the tests run one at a time and each shuts it down afterwards with
`openfeature.Shutdown()`, which lets the next test register its own provider.

`newRouter` takes the client as a parameter rather than reaching for a package-level one, which
is what lets the test hand it a client over the in-memory provider.

## Running without a server SDK key

When the provider cannot reach ConfigDirector it never receives config state and every read falls
back to the in-code default value this app passes in. That is the path a production app takes
with a wrong key or no network, so it is worth seeing: the app keeps serving, on the in-code
default values you chose, and `/configs/<key>` reports `PROVIDER_NOT_READY`.

With `fake-sample-key` the SDK server refuses the connection. The client logs that once at error
level, the provider stays without config state, and the app answers every request with the
in-code default values above.
