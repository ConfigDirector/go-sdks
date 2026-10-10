# ConfigDirector Go Server SDK

[![Go Reference][reference-badge]][reference] [![CI][ci-badge]][ci]

Go server SDK for [ConfigDirector](https://www.configdirector.com), remote config and feature flags with typed values, JSON Schema validation, and safe renames of live flags. Start free, no card required.

## Install

```bash
go get github.com/ConfigDirector/go-sdks/configdirector
```

## Retrieve a value

```go
import "github.com/ConfigDirector/go-sdks/configdirector"

// The server SDK key is a secret. Read it from the environment rather than committing it.
client, err := configdirector.NewClient(configdirector.Options{SDKKey: os.Getenv("CONFIGDIRECTOR_SERVER_KEY")})
if err != nil {
	log.Fatal(err)
}
defer client.Close()

if err := client.WaitForReady(ctx); err != nil {
	log.Printf("config state has not arrived yet, reads return their in-code default value: %v", err)
}

user := configdirector.UserContext{ID: "user-123", Traits: map[string]any{"plan": "pro"}}
newCheckout := client.BoolValue("new-checkout", false, user)
```

Every type, method and option is described in the [package reference][reference].

## Test your code

`configdirectortest` creates a **test client**: the SDK's real client wired to an in-memory
connection that your test controls, with no network connection, no telemetry and no goroutine.
Seed values, change them mid-test so watches and `OnConfigsUpdated` handlers fire, and hold or
fail initialization to test what your code does before config state arrives. It ships in the
module, so there is nothing else to install.

```go
import "github.com/ConfigDirector/go-sdks/configdirector/configdirectortest"

func TestNewCheckout(t *testing.T) {
	testClient := configdirectortest.NewTestClient(map[string]any{"new-checkout": true})
	defer testClient.Close()
	service := NewCheckoutService(testClient.Client())

	require.True(t, service.IsNewCheckoutEnabled("user-123"))

	require.NoError(t, testClient.SetValue("new-checkout", false))
	require.False(t, service.IsNewCheckoutEnabled("user-123"))
}
```

`testClient.Client()` is a `*configdirector.Client`, so it goes anywhere your code accepts one. It
is ready when `NewTestClient` returns, with the values delivered as its first update, so the code
under test can read at once; `HoldInitialization()` keeps it not ready instead, for testing what
your code does before config state arrives. `Close` closes the client.

### Values

`NewTestClient`, `SetValue` and `ReplaceValues` take Go values, and the config type follows from
each value: a `bool`; any integer kind, `time.Duration` included (an integer config); a `float32`
or `float64` (a float config); a `string`; or a map, slice, array or struct (a JSON config holding
what `encoding/json` writes for it). A `string` is always a string config, so JSON text meant as a
JSON config is given as a `json.RawMessage`. Every value is served as an unconditional config, so
every user context receives the same value; a test that needs different values per context is
written as one test per value. A `nil` value, a blank key, a non-finite number, a value that does
not encode as JSON, or a value of any other kind, such as a pointer or a channel, is an error from
`SetValue` and `ReplaceValues` and a panic from `NewTestClient`; in every case nothing changes.

Reads behave as they do in production: `SetValue("k", true)` read with `StringValue` returns the
in-code default value with the `type-mismatch` reason, `SetValue("k", 2.5)` read with `IntValue`
returns the default with the `invalid-number` reason, and `SetValue("k", "")` serves the in-code
default value with the `value-missing` reason. The `*ValueDetails` methods and `OnConfigEvaluated`
handlers report the reason.

### Controls

| Control | Effect |
| --- | --- |
| `SetValue(key, value)` | Stores the value and, once the client has received its first update, delivers an update carrying only `key`. Watches of `key`, `OnConfigsUpdated` handlers and reads see it before the call returns. |
| `RemoveValue(key)` | Removes the value and delivers a full update without it. Reads return the in-code default value with the `config-state-missing` reason, watches of `key` receive the default, and `OnConfigsUpdated` lists `key` in `RemovedKeys`. |
| `ReplaceValues(values)` | Replaces every stored value and delivers a full update. Use it to reset a test client shared across tests. |
| `CompleteInitialization()` | Delivers the stored values to a client built with `HoldInitialization()`, on the calling goroutine, so the client is ready and a blocked `WaitForReady` has returned when the call returns. On a ready client it does nothing. |
| `FailInitialization()` | Fails the held initialization the way a refused SDK key does: the failure is logged at error level and the client stays not ready, so reads return the in-code default value with the `client-not-ready` reason and a `WaitForReady` in flight ends with its timeout or its context. `CompleteInitialization()` still delivers the stored values afterwards. On a ready client it does nothing. |

### Options

`NewTestClient(values, options...)` takes `HoldInitialization()`, described above; `WithTimeout`,
the client's connection timeout, which bounds how long a held `WaitForReady` waits and defaults to
the SDK's production timeout; `WithLogger`, which defaults to `slog.Default()` as for a production
client; and `WithHooks`, handlers attached before the first update as `Options.Hooks` attaches
them, so an `OnConfigsUpdated` hook sees the values arrive and an `OnReady` hook sees the client
become ready.

### What to expect

- Watches and handlers run on the goroutine that calls `NewTestClient`, `SetValue`,
  `RemoveValue`, `ReplaceValues` or `CompleteInitialization`, not on a connection goroutine, so an
  assertion can follow the call directly. A change made from inside a handler is delivered after
  the update being handled, as it would be by the single connection goroutine in production.
- The first update fires `OnConfigsUpdated` before `OnReady`, as in production. With
  `HoldInitialization()`, handlers and watches registered before `CompleteInitialization` see it.
- A held `WaitForReady` that times out leaves the client not ready, and `CompleteInitialization`
  afterwards still makes it ready, as production does when config state arrives after the wait.
- Watches fire on every update carrying their key, whether or not the value changed, as in
  production. `RemoveValue` and `ReplaceValues` deliver a full update, so they also fire the
  watches of every remaining key.
- A panic in a watch or handler is recovered and logged, as with the production transports. A
  failed `require` assertion inside a handler ends the test as it does anywhere else, because the
  handler runs on the test's goroutine.
- After `Close`, reads return the in-code default value with the `client-not-ready` reason, every
  handler and watch is cancelled, the controls store without delivering, and no goroutine is left
  running, because none was started.

## Sample apps

The [Gin sample](../samples/configdirector/gin/) is a small runnable application using this SDK,
with a test on `configdirectortest`.

## Getting Help

- [Ask a question in Discussions](https://github.com/orgs/ConfigDirector/discussions)
- [Contact support](https://www.configdirector.com/support)

[//]: # "links"
[reference-badge]: https://pkg.go.dev/badge/github.com/ConfigDirector/go-sdks/configdirector.svg
[reference]: https://pkg.go.dev/github.com/ConfigDirector/go-sdks/configdirector
[ci-badge]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml/badge.svg
[ci]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml
