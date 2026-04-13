# shhh — Phase 0 build and eval targets.
#
# Phase 1 ship criterion (PRD §10) is that a skeptic can `git clone`,
# `make bench`, and reproduce our published numbers without asking us
# any questions. These targets are that contract.

GO        ?= go
BIN_DIR   := bin
SHHH      := $(BIN_DIR)/shhh
SHHH_EVAL := $(BIN_DIR)/shhh-eval

.PHONY: all build test vet bench scan fixture-scan clean help

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

clean:
	rm -rf $(BIN_DIR)
