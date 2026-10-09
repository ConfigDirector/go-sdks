# ConfigDirector Go SDKs

[![CI][ci-badge]][ci]

Go modules for [ConfigDirector](https://www.configdirector.com), remote config and feature flags with typed values, JSON Schema validation, and safe renames of live flags. Start free, no card required.

| Module | pkg.go.dev | Description |
|---|---|---|
| [configdirector](configdirector/) | [![Go Reference][sdk-badge]][sdk-reference] | The Go server SDK |
| [configdirector-openfeature](configdirector-openfeature/) | [![Go Reference][provider-badge]][provider-reference] | [OpenFeature](https://openfeature.dev) server provider, built on the server SDK |

Each module has its own changelog, version, and release. Pick the server SDK to use ConfigDirector directly, or the OpenFeature provider if your application reads flags through the OpenFeature API.

## Sample apps

Small, runnable applications will live under `samples/`, grouped by the module they demonstrate, starting with the first SDK release.

## Getting Help

- [Ask a question in Discussions](https://github.com/orgs/ConfigDirector/discussions)
- [Contact support](https://www.configdirector.com/support)

[//]: # "links"
[ci-badge]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml/badge.svg
[ci]: https://github.com/ConfigDirector/go-sdks/actions/workflows/ci.yml
[sdk-badge]: https://pkg.go.dev/badge/github.com/ConfigDirector/go-sdks/configdirector.svg
[sdk-reference]: https://pkg.go.dev/github.com/ConfigDirector/go-sdks/configdirector
[provider-badge]: https://pkg.go.dev/badge/github.com/ConfigDirector/go-sdks/configdirector-openfeature.svg
[provider-reference]: https://pkg.go.dev/github.com/ConfigDirector/go-sdks/configdirector-openfeature
