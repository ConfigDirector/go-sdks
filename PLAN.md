# Go SDKs Plan

Umbrella card: https://trello.com/c/vO00qvNu. Each increment below is one Trello card at the top of the SDK Backlog, in this order. This file moved here from `config-director/product-documents/go-sdk-plan.md` with the bootstrap increment.

## 1. Goals

1. A Go server SDK, `github.com/ConfigDirector/go-sdks/configdirector`, with the same behavior as the Python, Java, .NET and JavaScript server SDKs: local evaluation of targeting rules and segments, streaming and polling connection modes, telemetry, events and watches, and a test client for consumers.
2. A Go OpenFeature server provider, `github.com/ConfigDirector/go-sdks/configdirector-openfeature`, built on the SDK and released on its own.
3. Both proven by the targeting-rules contract and the SDK functional tests, then listed everywhere ConfigDirector lists SDKs.

## 2. Guidelines

- Use the JavaScript, Python, .NET and Java server SDKs as references for behavior. Do not transliterate; follow Go semantics, idioms and standard practice.
- Every change is covered by tests proven effective: written first and seen failing for the right reason, or the implementation mutated afterwards until the test fails for the right reason.
- Comments only as doc comments on exported members.
- Small increments, each self contained, handed over for review and committed by the user before the next starts.
- Integration tests exercise the public API against a stubbed SDK server (`httptest`); no internal component is stubbed.

## 3. Decisions

| Area | Decision |
| --- | --- |
| Repository | Public monorepo `ConfigDirector/go-sdks`, checked out at `/Users/alejandro/Projects/ConfigDirector/go-sdks`. Root `go.work` holds both modules; samples are modules outside it, depending on released versions and checked against the tree in CI. |
| Modules | `configdirector/` (package `configdirector`), tags `configdirector/vX.Y.Z`. `configdirector-openfeature/` (package `configdirectoropenfeature`), tags `configdirector-openfeature/vX.Y.Z`. |
| SDK names | `go-server-sdk` and `go-openfeature-server-provider`, registered in the SDK server and core-api capability tables with segment-conditions minimum `0.1.0`. The provider identifies itself through a closed wrapper list in the SDK. |
| Toolchain | `go` directive is the oldest release Go still supports when the first increment lands (1.26 on 2026-10-09, with 1.27 current); CI tests that and current on ubuntu, macos and windows with `-race`. SDK module depends on the standard library only; the provider adds `github.com/open-feature/go-sdk`. Tests use `testing`, `testify` and `httptest`. Lint through `gofmt`, `go vet` and a pinned `golangci-lint`, all behind `make` targets a pre-push hook shares with CI. |
| Construction | `configdirector.NewClient(Options) (*Client, error)` with an options struct (`SDKKey`, `Metadata`, `ConnectionOptions`, `TelemetryOptions`, `Logger *slog.Logger`, `Hooks`). Zero values are defaults; validation happens in the constructor; connecting starts at once. |
| Readiness | `WaitForReady(ctx) error` returns the context error when the wait ends; the client keeps connecting. `Ready()`, `Closed()`, `Close() error`. |
| Context type | `UserContext{ID, Name, Traits, Anonymous}` (recorded in `config-director/GLOSSARY.md` under Context). |
| Reads | One method per kind: `BoolValue`, `StringValue`, `IntValue` (`int64`), `FloatValue` (`float64`), `JSONValue(key, target, userContext) error`; `*ValueDetails` twins returning `EvaluationDetails{Reason, ValueID, UsedDefault}`; `AllConfigs(userContext, keys...)`. One `Evaluation` type backs details, the evaluated event and telemetry. |
| Events | `Options.Hooks` for construction-time handlers; `OnReady`, `OnConfigsUpdated`, `OnConfigEvaluated` and per-kind `Watch*` return an unsubscribe func. Handlers run on the transport goroutine; panics are recovered and logged. |
| HTTP | `ConnectionOptions.HTTPClient *http.Client`; the functional-test probe counts polls through it. |
| Telemetry `requestedType` | Go type names: `bool`, `string`, `int64`, `float64`, and the reflected target type for JSON. |
| Logging | `*slog.Logger`, default `slog.Default()`, records tagged `component=configdirector`; no level option. |
| Testing package | `configdirector/configdirectortest` in the SDK module, shipped before `v1.0.0`. |
| Versions | `v0.x` until the functional tests pass in dev and the provider is built on it, then `v1.0.0`. `configdirector.Version` is the single source, sent in `User-Agent` and telemetry. |
| Release | Manual `workflow_dispatch` per module: resolve `Version`, refuse an existing tag, require the CHANGELOG section, run the suite, push the tag, create a GitHub Release, request the module from proxy.golang.org. |
| Samples | Gin for the SDK (port 3600) and for the provider (3610), same five keys and context mapping as the siblings, each with a test on `configdirectortest`. A standard-library sample is a follow-up card. |
| Hardening | Strict `golangci-lint` preset with `revive`; `govulncheck`, `gosec`, fuzz targets for the SSE parser, value parser, JSON pointer lookup and payload decoder, all in CI. |

## 4. Increments and rollout

### 4.1 SDK

- [x] 0. Repository bootstrap: modules, `go.work`, Makefile, CI, CONTRIBUTING, README, CHANGELOGs, `PLAN.md`. https://trello.com/c/24uejNbJ
- [x] 1. Evaluator passing the targeting-rules contract (harness and `check-go` target in the contract repo). https://trello.com/c/ttMBEGPY
- [x] 2a. Client construction, readiness, typed reads, details, `AllConfigs`, over a stub connection. https://trello.com/c/Fhk1UiaC
- [x] 2b. Hooks, events and watches. https://trello.com/c/7xefH8eR
- [x] 4. Gin sample for the SDK. https://trello.com/c/IpVV28hU
- [x] 5. SSE event-stream parser. https://trello.com/c/XMmyx4lI
- [ ] 6a. Streaming transport with heartbeats and jittered backoff. https://trello.com/c/cm2oiAP0
- [ ] 6b. Polling transport with retry-once on network failure. https://trello.com/c/8F39Tzg8
- [ ] 7. Transports wired in, payload parsing, stub removed. https://trello.com/c/3hbN8uCc
- [ ] 7b. `configdirectortest` package. https://trello.com/c/7avX6BR6
- [ ] 8. Telemetry. https://trello.com/c/Twv97Aaw
- [ ] 9. Idiom pass. https://trello.com/c/haAgDhNW
- [ ] 10. Race, vulnerability, security and fuzz pass. https://trello.com/c/xLOphQL6
- [ ] 11. Release workflows; human tags `configdirector/v0.1.0`. https://trello.com/c/tFs1fhaM

### 4.2 Rollout of the SDK

- [ ] 12. Capability tables in the SDK server and core-api; human deploys to production. https://trello.com/c/PP976PtY
- [ ] 13. Functional-test probe for `go-server-sdk`; human runs it against dev. https://trello.com/c/4krrV5tl
- [ ] 14. Human tags `configdirector/v1.0.0` once 13 is green.

### 4.3 Provider

- [ ] 15. OpenFeature provider module, wrapper identity in the SDK, provider release workflow; human tags `0.1.0`. https://trello.com/c/ZEvhP9ct
- [ ] 16. Gin sample for the provider. https://trello.com/c/vbSa6nzs
- [ ] 17. Functional-test probe for the provider; human runs it, then tags `configdirector-openfeature/v1.0.0`. https://trello.com/c/RpHlwroX

### 4.4 Listings

- [ ] 18. Docs website pages, partials and every SDK listing. https://trello.com/c/JwriVKYd
- [ ] 19. Dashboard SDK samples. https://trello.com/c/9d8XIeoI
- [ ] 20. Website mentions and the LaunchDarkly comparison. https://trello.com/c/lY79at4Z
- [ ] 21. Claude plugin setup-skill rows. https://trello.com/c/JrvqOPZR
- [ ] 22. OpenFeature ecosystem dataset pull request (human). https://trello.com/c/WNgWxYfE

### 4.5 Follow-ups

- [ ] Standard-library `net/http` sample. https://trello.com/c/FLFdzEjT

## 5. Progress log

Tick an item above and add one line here per completed increment: date, increment, deviation if any.

- 2026-10-09, increment 0. `go 1.26`, not the 1.25 the card named: Go 1.27 shipped on 2026-08-19 and `github.com/open-feature/go-sdk` v1.19.0 already requires 1.26. The provider's `go.mod` requires testify only for now: `go mod tidy` ignores `go.work` and drops a requirement nothing imports, and the SDK requirement cannot resolve until `configdirector/v0.1.0` is tagged (increment 11); increment 15 adds both.
- 2026-10-09, increment 1. `internal/evaluation` passes all 4,486 condition, 118 bucketing and 60 segment cases. The harness maps the wire shape onto the evaluator's types itself until increment 7 adds the payload parser; an empty context identifier is treated as no identifier (assigned 0.0), where Python and JavaScript hash the empty string.
- 2026-10-09, increment 2a. `WaitForReady` applies `ConnectionOptions.Timeout` as an upper bound on the caller's context and returns `ErrClientClosed` after `Close`; `JSONValue` decodes as `encoding/json` does, after a trial decode so a document that does not fit leaves the target untouched; the `Evaluation` type stays unexported until the events card returns it; a zero `Timeout` is the 3s default and only a negative one is rejected; the `connection` interface reports readiness by delivering its first payload rather than through a separate call. The stand-in connection delivers its payload after 200ms so readiness can be observed through the public API.
- 2026-10-09, increment 2b. `OnReady` registered once the client is ready calls the handler at once, as the card asked; the Java `onClientReady` the card cites never calls a late handler, so this is a deliberate difference. A watch that fires reaches the `OnConfigEvaluated` handlers, as the Python and Java watchers do. `WatchJSON` takes a `newTarget func() any` called for a fresh pointer on every notification and hands that pointer to the handler. A nil handler panics at registration, as `http.HandleFunc` does. The stand-in connection delivers one fixed payload, so the delta, drop and segment scenarios run through a test-only connection in `export_test.go` that delivers payloads on the test's goroutine; increment 7 replaces it with `httptest`.
- 2026-10-09, increment 4. `samples/configdirector/gin` reaches the SDK through a `replace` directive until increment 11 tags it; `make samples-local` lints and tests every sample through a throwaway workspace under `.tools/`, and the `samples` CI job runs it. Against the stand-in connection the five keys evaluate to their in-code default values, so the test asserts those and proves the context through an `OnConfigEvaluated` hook; increment 7b switches it to `configdirectortest` and served values. `.env` is loaded through `github.com/joho/godotenv`, as the Flask sample loads it through python-dotenv. Observation for the API review: `JSONValue` returns an error only for a target that is not a non-nil pointer, so every caller carries an error branch it cannot reach.
- 2026-10-09, increment 5. `internal/eventsource` is pull-based: `Reader.Next()` returns one event at a time, where Python and Java push through callbacks. A goroutine reads the source into 16 KiB chunks so `Next` can time the silence between them; a chunk that arrived while the caller was busy wins over an elapsed stall timer. The reader is seeded with the previous connection's last event id (`Options.LastEventID`), so the transport can carry it across reconnects without Python's None-versus-empty distinction. The 1 MiB line cap also applies to a line that arrives whole in one chunk, which the siblings never check, so parsing does not depend on chunking; `FuzzParse` asserts that by parsing every input whole and byte by byte. A byte order mark is stripped from the first line rather than the first chunk, so one split across chunks still goes. Bytes pass through undecoded; Python replaces invalid UTF-8.
