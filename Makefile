.PHONY: hooks format lint verify-tidy test test-race check check-all clean

# Every module this repository publishes, each in a directory named after the last element of its
# module path. The root go.work lists the same two; `./...` from here would match none of them,
# since the root is not a module itself, so the go commands below name each module's packages.
MODULES := configdirector configdirector-openfeature
PACKAGES := $(foreach module,$(MODULES),./$(module)/...)

# The binary's path carries the version, so bumping the version reinstalls it on the next run.
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT := .tools/golangci-lint/$(GOLANGCI_LINT_VERSION)/golangci-lint$(if $(filter Windows_NT,$(OS)),.exe)

hooks:
	git config core.hooksPath .githooks
	@echo "pre-push hook installed. Bypass a single push with 'git push --no-verify'."

format:
	gofmt -w $(MODULES)

$(GOLANGCI_LINT):
	@printf '\n\033[1m==> install golangci-lint %s\033[0m\n' "$(GOLANGCI_LINT_VERSION)"
	curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh" \
		| sh -s -- -b "$(dir $@)" "$(GOLANGCI_LINT_VERSION)"

lint: $(GOLANGCI_LINT)
	@printf '\n\033[1m==> lint\033[0m\n'
	@unformatted=$$(gofmt -l $(MODULES)); \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt: these files are not formatted, run 'make format':"; echo "$$unformatted"; exit 1; \
		fi
	go vet $(PACKAGES)
	$(GOLANGCI_LINT) run $(PACKAGES)

# Asserts each module's go.mod and go.sum are what `go mod tidy` would write. Tidy ignores go.work,
# so this also proves every module resolves on its own, the way a consumer builds it.
verify-tidy:
	@for module in $(MODULES); do \
		printf '\n\033[1m==> verify-tidy %s\033[0m\n' "$$module"; \
		(cd "$$module" && go mod tidy -diff) || exit 1; \
	done

test:
	@printf '\n\033[1m==> test\033[0m\n'
	go test $(PACKAGES)

test-race:
	@printf '\n\033[1m==> test-race\033[0m\n'
	go test -race $(PACKAGES)

# The fast loop while working.
check: lint test

# Everything CI runs. The pre-push hook calls this.
check-all: verify-tidy lint test-race
	@printf '\n\033[1m✓ all checks passed\033[0m\n'

clean:
	rm -rf .tools
	go clean -testcache
