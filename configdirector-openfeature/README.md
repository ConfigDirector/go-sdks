# ConfigDirector OpenFeature Go Server Provider

[![Go Reference][reference-badge]][reference] [![CI][ci-badge]][ci]

[OpenFeature](https://openfeature.dev) Go server provider for [ConfigDirector](https://www.configdirector.com), remote config and feature flags with typed values, JSON Schema validation, and safe renames of live flags. It wraps the [ConfigDirector Go server SDK](../configdirector/) and requires Go 1.26 or newer.

## Install

```bash
go get github.com/ConfigDirector/go-sdks/configdirector-openfeature@latest
```

The OpenFeature Go SDK (`github.com/open-feature/go-sdk`) and the ConfigDirector Go server SDK come with it as dependencies.

## Retrieve a value

```go
import (
	"github.com/ConfigDirector/go-sdks/configdirector"
	configdirectoropenfeature "github.com/ConfigDirector/go-sdks/configdirector-openfeature"
	"github.com/open-feature/go-sdk/openfeature"
)

// The server SDK key is a secret. Read it from the environment rather than committing it.
provider, err := configdirectoropenfeature.NewProvider(configdirector.Options{SDKKey: os.Getenv("CONFIGDIRECTOR_SERVER_KEY")})
if err != nil {
	log.Fatal(err)
}
if err := openfeature.SetProviderAndWait(provider); err != nil {
	log.Fatal(err)
}
defer openfeature.Shutdown()

client := openfeature.NewClient("checkout")
newCheckout := client.Boolean(ctx, "new-checkout", false, openfeature.EvaluationContext{})
```

The provider takes the same `configdirector.Options` as the server SDK's `NewClient`: application metadata, connection mode, timeout and polling interval, telemetry tuning, a logger and hooks. Every type, method and option is described in the [package reference][reference].

## Evaluation context

The OpenFeature evaluation context maps onto the ConfigDirector user context used by targeting rules:

| OpenFeature | ConfigDirector |
|---|---|
| targeting key, or failing that an `id` attribute | `ID` |
| `name` attribute | `Name` |
| `traits` attribute, a `map[string]any` | `Traits` |
| `anonymous` attribute, a `bool` | `Anonymous` |

```go
user := openfeature.NewEvaluationContext("user-123", map[string]any{
	"name":   "Ada",
	"traits": map[string]any{"plan": "pro"},
})
newCheckout := client.Boolean(ctx, "new-checkout", false, user)
```

## Evaluation details

| Outcome | `Reason` | `ErrorCode` |
|---|---|---|
| A value was evaluated | `TARGETING_MATCH`, with the value's id as the `Variant` | |
| The config has no value | `DEFAULT` | |
| No config has that key | `ERROR` | `FLAG_NOT_FOUND` |
| The value does not match the requested type | `ERROR` | `TYPE_MISMATCH` |
| No config state has been received yet | `ERROR` | `PROVIDER_NOT_READY` |

An object evaluation decodes the config's JSON document the way `encoding/json` decodes into an `any`: an object is a `map[string]any`, an array a `[]any`, a number a `float64`.

If the first config state does not arrive within the connection timeout, initialization still completes and evaluations return their defaults. The provider keeps connecting, and emits `PROVIDER_READY` once config state arrives. It emits `PROVIDER_CONFIGURATION_CHANGED` whenever configs are updated, with the keys the update carried followed by the keys that were removed.

## Test your code

Tests of code that reads flags through OpenFeature swap the provider for the in-memory one the OpenFeature Go SDK ships, `memprovider`, so the test controls the values and nothing from ConfigDirector is involved:

```go
import (
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
)

func TestNewCheckout(t *testing.T) {
	provider := memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{
		"new-checkout": {Key: "new-checkout", State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": true, "off": false}},
	})
	require.NoError(t, openfeature.SetProviderAndWait(provider))
	defer openfeature.Shutdown()

	require.True(t, openfeature.NewClient("checkout").Boolean(t.Context(), "new-checkout", false, openfeature.EvaluationContext{}))
}
```

The in-memory provider has no method to change a flag, so a mid-test change sets a new provider. Shut OpenFeature down after each test with `openfeature.Shutdown()`.

## Documentation

Refer to the [official documentation for the OpenFeature Go provider](https://docs.configdirector.com/sdks/openfeature/go).

There is also [a quickstart guide for ConfigDirector and any of our SDKs](https://docs.configdirector.com/getting-started/quickstart).

## Sample apps

[`samples/`](../samples/) holds small, runnable applications built on the ConfigDirector Go modules.

## Getting Help

- [Ask a question in Discussions](https://github.com/orgs/ConfigDirector/discussions)
- [Contact support](https://www.configdirector.com/support)

[//]: # "links"
[reference-badge]: https://pkg.go.dev/badge/github.com/ConfigDirector/go-sdks/configdirector-openfeature.svg
[reference]: https://pkg.go.dev/github.com/ConfigDirector/go-sdks/configdirector-openfeature
[ci-badge]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml/badge.svg
[ci]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml
