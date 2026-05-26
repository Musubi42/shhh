# shhh — Phase 0 build and eval targets.
#
# Phase 1 ship criterion (PRD §10) is that a skeptic can `git clone`,
# `make bench`, and reproduce our published numbers without asking us
# any questions. These targets are that contract.

GO        ?= go
BIN_DIR   := bin
SHHH      := $(BIN_DIR)/shhh
SHHH_EVAL := $(BIN_DIR)/shhh-eval

.PHONY: all build test vet bench scan fixture-scan clean help demo update-gitleaks-license

all: build test

help: ## show available targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: $(SHHH) $(SHHH_EVAL) ## build both binaries

$(SHHH): $(shell find cmd/shhh internal -name '*.go')
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(SHHH) ./cmd/shhh

$(SHHH_EVAL): $(shell find cmd/shhh-eval internal -name '*.go')
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(SHHH_EVAL) ./cmd/shhh-eval

test: ## run unit tests
	$(GO) test ./... -count=1

vet: ## run go vet
	$(GO) vet ./...

bench: build ## run the shhh-eval benchmark suite
	@echo
	@echo '=== shhh-eval ==='
	$(SHHH_EVAL)

scan: build ## scan the current directory
	$(SHHH) scan .

fixture-scan: build ## scan the leaky-project fixture (screenshot-safe)
	$(SHHH) scan testdata/fixtures/leaky-project

demo: build ## end-to-end hook smoke test (simulates PreToolUse/Read)
	@./scripts/demo.sh

clean:
	rm -rf $(BIN_DIR)

# update-gitleaks-license refreshes cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt
# from the module cache. Run this whenever go.mod bumps the gitleaks version,
# otherwise `shhh licenses` ships a stale MIT notice from the previous
# release. The path resolves the exact version pinned in go.mod.
update-gitleaks-license: ## refresh embedded gitleaks LICENSE from module cache
	@GITLEAKS_VERSION=$$($(GO) list -m -f '{{.Version}}' github.com/zricethezav/gitleaks/v8); \
	GITLEAKS_DIR=$$($(GO) env GOMODCACHE)/github.com/zricethezav/gitleaks/v8@$$GITLEAKS_VERSION; \
	echo "refreshing from $$GITLEAKS_DIR"; \
	chmod u+w cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt 2>/dev/null || true; \
	cp "$$GITLEAKS_DIR/LICENSE" cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt; \
	chmod u+w cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt; \
	echo "ok — committed file now matches gitleaks $$GITLEAKS_VERSION"
