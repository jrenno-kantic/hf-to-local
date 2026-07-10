.PHONY: build test fmt vet dist checksums clean

BINARY := hf-to-local
GO ?= go
GOCACHE ?= $(CURDIR)/.cache/go-build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST_DIR := dist
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

help: ## Show this help.
	@echo "Usage:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[33m%-14s\033[0m %s\n", $$1, $$2}'

#########################

build: ## Build the binary for the current platform
	mkdir -p bin
	GOCACHE="$(GOCACHE)" $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o "bin/$(BINARY)" ./cmd/hf-to-local

test: ## Run the tests
	GOCACHE="$(GOCACHE)" $(GO) test ./...

fmt: ## Format the code
	gofmt -w cmd internal

vet: ## Vet the code
	GOCACHE="$(GOCACHE)" $(GO) vet ./...

dist: clean ## Build the binary for all target platforms and create tar.gz archives
	@set -e; \
	for target in $(TARGETS); do \
		goos=$${target%/*}; \
		goarch=$${target#*/}; \
		name="$(BINARY)-$(VERSION)-$${goos}-$${goarch}"; \
		ext=""; \
		if [ "$$goos" = "windows" ]; then ext=".exe"; fi; \
		mkdir -p "$(DIST_DIR)/$$name"; \
		echo "building $$name"; \
		GOOS=$$goos GOARCH=$$goarch GOCACHE="$(GOCACHE)" $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/$$name/$(BINARY)$$ext" ./cmd/hf-to-local; \
		cp README.md LICENSE "$(DIST_DIR)/$$name/"; \
		(cd "$(DIST_DIR)" && tar -czf "$$name.tar.gz" "$$name"); \
		rm -rf "$(DIST_DIR)/$$name"; \
	done

checksums: dist ## Generate SHA256 checksums for the tar.gz archives
	cd "$(DIST_DIR)" && shasum -a 256 *.tar.gz > checksums.txt

clean: ### Clean up build artifacts
	rm -rf bin
	rm -rf dist
