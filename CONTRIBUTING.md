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

[samples/](samples/) holds sample applications, one module per framework under
`samples/<module>/<framework>`. They are modules outside the workspace and depend on the SDK the
way an application does, each requiring a released version of the module it shows. `make samples`
lints and tests every sample the way an application builds it, against the modules its `go.mod`
names, and `make samples-local` against the modules in this working tree, through a throwaway
workspace it writes under `.tools/`, so a breaking API change fails before it ships. Add a new
sample to `SAMPLES` in the Makefile.

## Working on the modules

```sh
make hooks         # pre-push hook that runs make check-all
make check         # lint and test: the fast loop
make samples-local # the sample apps against this working tree
make samples       # the sample apps against the released modules their go.mod names
make check-all     # everything CI runs
make benchmark     # the smoke benchmark of a read; a measurement, not a check
```

`make lint` runs `gofmt`, `go vet` and `golangci-lint`, installing the version the Makefile pins
under `.tools/` the first time. [.golangci.yml](.golangci.yml) enables a strict set of linters,
`gosec` among them, on top of the standard preset; a finding is fixed, not silenced. `make format`
formats every module and sample. `make test-race` runs the tests with the race detector, as CI
does, and `make verify-tidy` fails when a module's or a sample's `go.mod` or `go.sum` differs
from what `go mod tidy` would write.

`make vulncheck` runs `govulncheck` over each module, installing the pinned version under
`.tools/` the first time and again after a Go upgrade, since the binary is built by the local Go
and reads the standard library's sources with it. It also scans the standard library of the Go
that runs it, so it fails on a Go that is behind its latest patch release; upgrade Go rather than
silence it. `make fuzz`
runs every fuzz target listed in the Makefile for 20 seconds each: the parsers that take input
from the network, which are the event-stream parser, the server SDK payload decoder, the value
parser and the JSON pointer lookup into a context's traits. A failing input is written under the
package's `testdata/fuzz/` directory, where it replays as a regular test; fix the cause and keep
the file. Both run in `make check-all` and in CI.

`make test-release-preflight` runs the release workflow's preflight script (see
[Releasing](#releasing)) against throwaway repositories, one scenario per release it must allow
and per refusal it must make. It runs in `make check-all` and in CI too.

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

| Module | Version | Changelog | Workflow |
|---|---|---|---|
| configdirector | [`version.go`](configdirector/version.go) | [CHANGELOG.md](configdirector/CHANGELOG.md) | [Release configdirector](.github/workflows/release-configdirector.yml) |
| configdirector-openfeature | [`version.go`](configdirector-openfeature/version.go) | [CHANGELOG.md](configdirector-openfeature/CHANGELOG.md) | [Release configdirector-openfeature](.github/workflows/release-configdirector-openfeature.yml) |

A Go module is released by a tag alone; there is nothing to upload. The tag is the module's
directory followed by its version, `configdirector/vX.Y.Z` or `configdirector-openfeature/vX.Y.Z`,
and the module proxy serves it from there.

1. In the module's changelog, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and open a
   fresh, empty `## [Unreleased]` above it.
2. Set `Version` to match and merge both to `main`.
3. Run the module's release workflow against `main`. It is manual (`workflow_dispatch`) by
   design and releases whatever version `main` currently declares. It reads `Version`, refuses to
   go on when the tag already exists or when the changelog has no section for that version (an
   empty section counts as none), runs `make check-all`, pushes the tag, creates a GitHub Release
   with the changelog section as its notes, and requests the module from proxy.golang.org, so
   that the version is installable and indexed on pkg.go.dev at once rather than when the first
   consumer asks. A version with a prerelease suffix, such as `0.2.0-rc.1`, is marked as a
   prerelease.

The module proxy serves a released version forever, so a tag is never moved or deleted: to ship a
fix, bump `Version`, add a changelog section and release again. Both workflows call
[release-module.yml](.github/workflows/release-module.yml) with their module's name, so a change
to the release steps is made there once. The checks that run before anything is pushed live in
[release-preflight.sh](.github/scripts/release-preflight.sh), which
`make test-release-preflight` covers.

### Version line

The modules stay at `v0.x` until the SDK functional tests pass against them in dev, which is
when `v1.0.0` is tagged. From then on a breaking change needs a new major version, which Go
publishes as a new module path ending in `/v2` (`github.com/ConfigDirector/go-sdks/configdirector/v2`,
tagged `configdirector/v2.0.0`), and every consumer changes its import path to move. Breaking
changes are therefore batched into a major release rather than shipped one at a time, and
anything that can be added instead of changed is.

### Releasing the provider after an SDK change

The provider's `go.mod` names the oldest SDK version it works with. When the provider starts using
something new in the SDK, raise that requirement and release the SDK first: the provider's tag
only resolves for consumers once the SDK version it requires is on the module proxy.
