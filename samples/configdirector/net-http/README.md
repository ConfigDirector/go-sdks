# net/http sample

A minimal app using the ConfigDirector Go server SDK with nothing but the standard library's
[`net/http`](https://pkg.go.dev/net/http). It is the [Gin sample](../gin/) without a framework: a
single `/configs` route that evaluates a handful of configs and returns them as JSON.

## Running it

```bash
cd samples/configdirector/net-http
cp .env.example .env      # optional, the sample runs without a real key
GOWORK=off go run .
```

`GOWORK=off` keeps this repository's root workspace, which the sample is deliberately not part
of, out of the way. A copy of the sample outside the repository runs with a plain `go run .`.

Then:

```bash
curl 'http://localhost:3601/configs?id=user-123&plan=pro'
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

Query parameters double as the user the configs are evaluated for: `id`, `name` and `anonymous`
map to the matching `UserContext` fields, and anything else becomes a trait:

```
/configs?id=user-123&name=Ada&plan=pro&region=eu
```

Stop it with Ctrl-C or `SIGTERM`; the app finishes in-flight requests and closes the client.

Run the tests with `GOWORK=off go test ./...`.

## One client per process

This is the single most important thing the sample shows. [`main.go`](main.go) creates one client
at startup, waits for its first config state, hands it to every request handler, and closes it on
shutdown.

```go
client, err := configdirector.NewClient(settings.options)
if err != nil {
	return err
}
defer closeClient(client, logger)

if err := client.WaitForReady(ctx); err != nil {
	logger.Warn("ConfigDirector is not ready; every config resolves to its in-code default value until it is", slog.Any("error", err))
} else {
	logger.Info("ConfigDirector is ready")
}

return serve(ctx, newRouter(client), logger)
```

Never create a client inside a request handler. Each client opens its own connection to
ConfigDirector, starts out not ready (so it serves the in-code default values), and keeps
connecting in the background for as long as it lives. A `Client` is safe to share between
goroutines, so every request handler uses this one.

Evaluation itself is cheap. The typed reads evaluate config state the client already holds in
memory, with no network call on the request path, which is what makes it safe to call several of
them per request.

## Routing without a framework

[`configs.go`](configs.go) registers two patterns on an `http.ServeMux`:

```go
mux.HandleFunc("GET /configs", func(w http.ResponseWriter, r *http.Request) {
	configs, err := configsFor(client, userContextFrom(r.URL.Query()))
	...
})

mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found. Try GET /configs"})
})
```

`GET /configs` matches the method and the path, and `/` catches every other request, a `POST` to
`/configs` included, with a hint. `writeJSON` sets the content type and status and encodes the
body with `encoding/json`.

## What else it demonstrates

**Readiness is explicit and non-fatal.** `WaitForReady` blocks until the first config state
arrives or the configured timeout elapses, and an error only says that the wait ended. The sample
logs a warning and carries on serving the in-code default values; the client becomes ready when
config state arrives.

**In-code default values are the fallback.** Every read passes the value to serve when
ConfigDirector is unreachable, so it should be the safe choice. The method decides the type the
config value is read as: `BoolValue`, `IntValue`, `StringValue`, and `JSONValue` into a target of
your own.

**The user is per request; the client is not.** `userContextFrom` in [`configs.go`](configs.go)
maps query parameters onto a `UserContext`; a real app would build this from the authenticated
session.

**Logging is yours to configure.** The sample builds a `slog.Logger` at the level
`CONFIGDIRECTOR_LOG_LEVEL` names and passes it to the client, so SDK output lands in the
application's own log alongside its records, tagged `component=configdirector`:

```
time=2026-10-10T14:22:24.233-07:00 level=DEBUG msg="Received config state" component=configdirector keys="[...]" removedKeys=[]
time=2026-10-10T14:22:24.233-07:00 level=INFO msg="ConfigDirector is ready"
```

Leave `Options.Logger` unset and the SDK logs through `slog.Default()` instead. Set
`CONFIGDIRECTOR_LOG_LEVEL=DEBUG` to see every config update the client receives.

**Settings come from the environment.** [`settings.go`](settings.go) reads the SDK key,
connection mode, timeout and base URL from `CONFIGDIRECTOR_*` variables, with a `.env` file
loaded at startup for local runs. A zero value in `Options` is the SDK's default, so the sample
only sets what the environment names.

**Shutdown is clean.** `SIGTERM` and Ctrl-C stop the HTTP server, let in-flight requests finish,
and close the client, which stops its connection.

The SDK also supports watching configs for changes and registering for the client's events; see
the [package documentation](../../../configdirector/doc.go) for `WatchBool` and its siblings,
`OnReady`, `OnConfigsUpdated` and `OnConfigEvaluated`.

## Testing

[`main_test.go`](main_test.go) tests the app the way an application using the SDK tests itself:
it builds the router around a test client from
[`configdirectortest`](../../../configdirector/configdirectortest/), the SDK's real client wired
to an in-memory connection the test controls, and drives the router with `httptest`, so nothing
reaches the network. The test seeds the five configs with values other than the in-code defaults,
asserts the response, and changes one with `SetValue` to see the next response follow:

```go
testClient := newTestClient(t, servedValues)
router := newRouter(testClient.Client())
get(t, router, "/configs?id=user-123")

require.NoError(t, testClient.SetValue("integer-config", 30))
response := get(t, router, "/configs?id=user-123")
```

`newTestClient` wraps `configdirectortest.NewTestClient` so that `t.Cleanup` closes the client. An
`OnConfigEvaluated` hook, attached through `configdirectortest.WithHooks`, shows the user each
config was evaluated for and why it got the value it did.

`newRouter` takes the client as a parameter rather than reaching for a package-level one, which
is what lets the test hand it a client it controls.

## Running without a server SDK key

When the client cannot reach ConfigDirector it stays not ready and every config falls back to the
in-code default value this app passes in. That is the path a production app takes with a wrong
key or no network, so it is worth seeing: the app keeps serving, on the defaults you chose.

With `fake-sample-key` the SDK server refuses the connection. The client logs that once at error
level, stays not ready, and the app answers every request with the in-code default values above.
