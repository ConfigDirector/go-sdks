# Contributing

## Layout

This repository publishes two Go modules, each in a directory named after the last element of its
module path:

- [configdirector](configdirector/), the server SDK: module
  `github.com/ConfigDirector/go-sdks/configdirector`, package `configdirector`. At runtime it
  depends on the standard library only; its tests use testify.
- [configdirector-openfeature](configdirector-openfeature/), the OpenFeature provider built on it:
  module `github.com/ConfigDirector/go-sdks/configdirector-openfeature`, package
  `configdirectoropenfeature`.

The root [go.work](go.work) lists both, so the provider always develops against the SDK in this
working tree. The root is not a module itself, so `./...` from the root matches nothing; the
Makefile names each module's packages instead, as in
`go build ./configdirector/... ./configdirector-openfeature/...`. Each module has its own version,
changelog, and release. Once the provider imports the SDK, its `go.mod` must require a released
version of the SDK from the module proxy, never this working tree, because `go mod tidy` ignores
`go.work`.

`samples/` is reserved for sample applications. They are modules outside the workspace and depend
on released versions, the way an application does.

## Working on the modules

```sh
make hooks     # pre-push hook that runs make check-all
make check     # lint and test: the fast loop
make check-all # everything CI runs
```

`make lint` runs `gofmt`, `go vet` and `golangci-lint`, installing the version the Makefile pins
under `.tools/` the first time. `make format` formats every module. `make test-race` runs the tests
with the race detector, as CI does, and `make verify-tidy` fails when a module's `go.mod` or
`go.sum` differs from what `go mod tidy` would write.

The checks live in the [Makefile](Makefile). CI and the pre-push hook only call `make` targets,
so add new checks there rather than in either of them.

## How the provider identifies itself

The SDK reports an SDK name and version in the `User-Agent` header and in telemetry. A module
built on the SDK, such as the provider, must report its own name and `Version` without the SDK
accepting an arbitrary identity from its caller: the SDK keeps a closed list of the wrapper
modules allowed to, and reads each one's version itself. This list does not exist yet; increment
15 in [PLAN.md](PLAN.md) adds it. Adding a wrapper module means adding it to that list, releasing
the SDK, and registering the new name in the ConfigDirector capability tables.

## Releasing

Each module is released on its own, and the steps are the same for both. The version lives in
exactly one place per module, the exported `Version` constant:

| Module | Version | Changelog |
|---|---|---|
| configdirector | [`version.go`](configdirector/version.go) | [CHANGELOG.md](configdirector/CHANGELOG.md) |
| configdirector-openfeature | [`version.go`](configdirector-openfeature/version.go) | [CHANGELOG.md](configdirector-openfeature/CHANGELOG.md) |

A Go module is released by a tag alone; there is nothing to upload. The tag is the module's
directory followed by its version, `configdirector/vX.Y.Z` or `configdirector-openfeature/vX.Y.Z`,
and the module proxy serves it from there.

1. In the module's changelog, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and open a
   fresh, empty `## [Unreleased]` above it.
2. Set `Version` to match and merge both to `main`.
3. Run the module's release workflow against `main`. The workflows are not written yet; they will
   be manual (`workflow_dispatch`) by design, release whatever version `main` currently declares,
   refuse a tag that already exists, require the changelog section, run the suite, push the tag,
   create a GitHub Release and request the module from proxy.golang.org.

### Releasing the provider after an SDK change

The provider's `go.mod` names the oldest SDK version it works with. When the provider starts using
something new in the SDK, raise that requirement and release the SDK first: the provider's tag
only resolves for consumers once the SDK version it requires is on the module proxy.
