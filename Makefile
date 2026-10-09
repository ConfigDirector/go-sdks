.PHONY: hooks format lint verify-tidy test test-race samples-local check check-all clean

# Every module this repository publishes, each in a directory named after the last element of its
# module path. The root go.work lists the same two; `./...` from here would match none of them,
# since the root is not a module itself, so the go commands below name each module's packages.
MODULES := configdirector configdirector-openfeature
PACKAGES := $(foreach module,$(MODULES),./$(module)/...)

# Sample applications, each a module outside the root go.work under samples/<module>/<framework>,
# depending on the SDK the way an application does. `samples-local` checks them against this
# working tree through a throwaway workspace under .tools/, so neither the root go.work nor a
# sample's go.mod has to change.
SAMPLES := samples/configdirector/gin
SAMPLES_WORKSPACE := $(CURDIR)/.tools/samples/go.work

# The binary's path carries the version, so bumping the version reinstalls it on the next run.
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT := .tools/golangci-lint/$(GOLANGCI_LINT_VERSION)/golangci-lint$(if $(filter Windows_NT,$(OS)),.exe)

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

test:
	@printf '\n\033[1m==> test\033[0m\n'
	go test $(PACKAGES)

test-race:
	@printf '\n\033[1m==> test-race\033[0m\n'
	go test -race $(PACKAGES)

# Lints and tests every sample against the modules in this working tree, which is what catches a
# breaking API change before it ships. From a sample's own directory, `GOWORK=off go test ./...`
# resolves the SDK the way the sample's go.mod says instead; without GOWORK=off the go command
# finds the root go.work, which does not list the sample, and refuses to build.
samples-local: $(GOLANGCI_LINT)
	@rm -f $(SAMPLES_WORKSPACE) && mkdir -p $(dir $(SAMPLES_WORKSPACE))
	GOWORK=$(SAMPLES_WORKSPACE) go work init $(addprefix $(CURDIR)/,$(MODULES) $(SAMPLES))
	@for sample in $(SAMPLES); do \
		printf '\n\033[1m==> sample %s (working-tree modules)\033[0m\n' "$$sample"; \
		(cd "$$sample" \
			&& GOWORK=$(SAMPLES_WORKSPACE) go vet ./... \
			&& GOWORK=$(SAMPLES_WORKSPACE) $(CURDIR)/$(GOLANGCI_LINT) run ./... \
			&& GOWORK=$(SAMPLES_WORKSPACE) go test -race ./...) || exit 1; \
	done

# The fast loop while working.
check: lint test

# Everything CI runs. The pre-push hook calls this.
check-all: verify-tidy lint test-race samples-local
	@printf '\n\033[1m✓ all checks passed\033[0m\n'

clean:
	rm -rf .tools
	go clean -testcache
