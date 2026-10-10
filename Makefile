.PHONY: hooks format lint verify-tidy vulncheck test test-race fuzz benchmark test-release-preflight samples samples-local check check-all clean

# Every module this repository publishes, each in a directory named after the last element of its
# module path. The root go.work lists the same two; `./...` from here would match none of them,
# since the root is not a module itself, so the go commands below name each module's packages.
MODULES := configdirector configdirector-openfeature
PACKAGES := $(foreach module,$(MODULES),./$(module)/...)

# Sample applications, each a module outside the root go.work under samples/<module>/<framework>,
# depending on the SDK the way an application does. `samples` checks them against the released
# modules their go.mod names; `samples-local` checks them against this working tree through a
# throwaway workspace under .tools/, so neither the root go.work nor a sample's go.mod has to
# change.
SAMPLES := samples/configdirector/gin
SAMPLES_WORKSPACE := $(CURDIR)/.tools/samples/go.work

# Each binary's path carries its version, so bumping the version reinstalls it on the next run.
# govulncheck is built from source by the local Go and reads the standard library's sources with
# that Go's parser, so a binary built by an older Go fails on a newer one's sources; its path
# carries the Go version too, and a Go upgrade reinstalls it.
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT := .tools/golangci-lint/$(GOLANGCI_LINT_VERSION)/golangci-lint$(if $(filter Windows_NT,$(OS)),.exe)
GOVULNCHECK_VERSION := v1.8.0
GOVULNCHECK := .tools/govulncheck/$(GOVULNCHECK_VERSION)-$(shell go env GOVERSION)/govulncheck$(if $(filter Windows_NT,$(OS)),.exe)

# The fuzz targets, each as <package directory>:<target>. `go test -fuzz` takes one target in one
# package per run, and CI gives each one this budget.
FUZZ_TARGETS := \
	configdirector:FuzzParseValue \
	configdirector:FuzzParseServerPayload \
	configdirector/internal/evaluation:FuzzTraitAtPointer \
	configdirector/internal/eventsource:FuzzParse
FUZZ_TIME := 20s

hooks:
	git config core.hooksPath .githooks
	@echo "pre-push hook installed. Bypass a single push with 'git push --no-verify'."

format:
	gofmt -w $(MODULES) $(SAMPLES)

$(GOLANGCI_LINT):
	@printf '\n\033[1m==> install golangci-lint %s\033[0m\n' "$(GOLANGCI_LINT_VERSION)"
	curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh" \
		| sh -s -- -b "$(dir $@)" "$(GOLANGCI_LINT_VERSION)"

lint: $(GOLANGCI_LINT)
	@printf '\n\033[1m==> lint\033[0m\n'
	@unformatted=$$(gofmt -l $(MODULES) $(SAMPLES)); \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt: these files are not formatted, run 'make format':"; echo "$$unformatted"; exit 1; \
		fi
	go vet $(PACKAGES)
	$(GOLANGCI_LINT) run $(PACKAGES)

# Asserts each module's go.mod and go.sum are what `go mod tidy` would write. Tidy ignores go.work,
# so this also proves every module resolves on its own, the way a consumer builds it.
verify-tidy:
	@for module in $(MODULES) $(SAMPLES); do \
		printf '\n\033[1m==> verify-tidy %s\033[0m\n' "$$module"; \
		(cd "$$module" && go mod tidy -diff) || exit 1; \
	done

$(GOVULNCHECK):
	@printf '\n\033[1m==> install govulncheck %s\033[0m\n' "$(GOVULNCHECK_VERSION)"
	GOBIN="$(CURDIR)/$(dir $@)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Reports known vulnerabilities in code the modules reach, the standard library of the Go that
# runs the check included: a finding there means that Go is behind its latest patch release.
# govulncheck reads one module at a time, so each is scanned from its own directory.
vulncheck: $(GOVULNCHECK)
	@for module in $(MODULES); do \
		printf '\n\033[1m==> vulncheck %s\033[0m\n' "$$module"; \
		(cd "$$module" && "$(CURDIR)/$(GOVULNCHECK)" ./...) || exit 1; \
	done

test:
	@printf '\n\033[1m==> test\033[0m\n'
	go test $(PACKAGES)

test-race:
	@printf '\n\033[1m==> test-race\033[0m\n'
	go test -race $(PACKAGES)

# Runs every fuzz target for its budget, after its seed corpus. A failing input is written under
# the package's testdata/fuzz/<target>/ directory and replays as a regular test from then on.
fuzz:
	@for target in $(FUZZ_TARGETS); do \
		package="./$${target%%:*}"; name="$${target##*:}"; \
		printf '\n\033[1m==> fuzz %s in %s for %s\033[0m\n' "$$name" "$$package" "$(FUZZ_TIME)"; \
		go test -run='^$$' -fuzz='^'"$$name"'$$' -fuzztime=$(FUZZ_TIME) "$$package" || exit 1; \
	done

# A measurement, not a check: the smoke benchmark of a read.
benchmark:
	@printf '\n\033[1m==> benchmark\033[0m\n'
	go test -run='^$$' -bench=. -benchmem ./configdirector/

# The release workflow's preflight decides the version, tag and notes of a release and refuses one
# it must not make. This runs it against throwaway repositories, one per scenario.
test-release-preflight:
	@printf '\n\033[1m==> test-release-preflight\033[0m\n'
	.github/scripts/release-preflight-test.sh

# Lints and tests the sample in $(1) from its own directory, with the SDK resolved as the GOWORK
# in $(2) says: `off` for the sample's own go.mod, or the throwaway workspace for this working tree.
# Without a GOWORK the go command finds the root go.work, which does not list the sample, and
# refuses to build.
define check-sample
(cd "$(1)" \
	&& GOWORK=$(2) go vet ./... \
	&& GOWORK=$(2) $(CURDIR)/$(GOLANGCI_LINT) run ./... \
	&& GOWORK=$(2) go test -race ./...)
endef

# Lints and tests every sample against the released modules its go.mod names, the way an
# application builds it.
samples: $(GOLANGCI_LINT)
	@for sample in $(SAMPLES); do \
		printf '\n\033[1m==> sample %s (released modules)\033[0m\n' "$$sample"; \
		$(call check-sample,$$sample,off) || exit 1; \
	done

# Lints and tests every sample against the modules in this working tree, which is what catches a
# breaking API change before it ships.
samples-local: $(GOLANGCI_LINT)
	@rm -f $(SAMPLES_WORKSPACE) && mkdir -p $(dir $(SAMPLES_WORKSPACE))
	GOWORK=$(SAMPLES_WORKSPACE) go work init $(addprefix $(CURDIR)/,$(MODULES) $(SAMPLES))
	@for sample in $(SAMPLES); do \
		printf '\n\033[1m==> sample %s (working-tree modules)\033[0m\n' "$$sample"; \
		$(call check-sample,$$sample,$(SAMPLES_WORKSPACE)) || exit 1; \
	done

# The fast loop while working.
check: lint test

# Everything CI runs. The pre-push hook calls this.
check-all: verify-tidy lint vulncheck test-race fuzz test-release-preflight samples samples-local
	@printf '\n\033[1m✓ all checks passed\033[0m\n'

clean:
	rm -rf .tools
	go clean -testcache
