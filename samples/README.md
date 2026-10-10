# Sample apps

Small, self-contained applications showing how to use the ConfigDirector Go modules with
different web frameworks, grouped by the module they demonstrate.

## [configdirector](configdirector/)

| Sample | Framework | Description |
|---|---|---|
| [gin/](configdirector/gin/) | [Gin](https://gin-gonic.com) | Minimal HTTP app: one client per process, per-request evaluation, clean shutdown |

## [configdirector-openfeature](configdirector-openfeature/)

| Sample | Framework | Description |
|---|---|---|
| [gin/](configdirector-openfeature/gin/) | [Gin](https://gin-gonic.com) | The same app reading through an OpenFeature client: one provider per process, evaluation details on `/configs/<key>` |

Each sample is an independent module outside the root `go.work`, depending on a released SDK
exactly as a real app would. `make samples-local` also checks every sample against the modules in
this working tree.

To run one from this repository, keep the root workspace out of the way with `GOWORK=off`; a copy
of the sample outside the repository runs with a plain `go run .`:

```bash
cd samples/configdirector/gin
GOWORK=off go run .
```

> The samples need a real server SDK key to resolve configs. Without one the client stays not
> ready and every config falls back to the in-code default value the sample passes in, which is
> also what a production app sees when it cannot reach ConfigDirector.
