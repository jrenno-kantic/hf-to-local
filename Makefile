.PHONY: build test fmt vet dist checksums clean

BINARY := hf-to-local
GO ?= go
GOCACHE ?= $(CURDIR)/.cache/go-build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST_DIR := dist
# Must match the build matrix in .goreleaser.yml, so `make checksums` is a
# faithful local rehearsal of a tagged release rather than a different artifact
# set with different names.
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
# GoReleaser's {{ .Version }} drops the leading "v" from the tag; mirror that in
# the archive names while keeping the richer describe output in --version.
ARCHIVE_VERSION := $(VERSION:v%=%)

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

dist: clean ## Build release archives for every target platform (mirrors GoReleaser)
	@set -e; \
	for target in $(TARGETS); do \
		goos=$${target%/*}; \
		goarch=$${target#*/}; \
		name="$(BINARY)_$(ARCHIVE_VERSION)_$${goos}_$${goarch}"; \
		ext=""; \
		if [ "$$goos" = "windows" ]; then ext=".exe"; fi; \
		mkdir -p "$(DIST_DIR)/$$name"; \
		echo "building $$name"; \
		GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0 GOCACHE="$(GOCACHE)" $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/$$name/$(BINARY)$$ext" ./cmd/hf-to-local; \
		cp README.md LICENSE "$(DIST_DIR)/$$name/"; \
		if [ "$$goos" = "windows" ]; then \
			(cd "$(DIST_DIR)" && zip -qr "$$name.zip" "$$name"); \
		else \
			(cd "$(DIST_DIR)" && tar -czf "$$name.tar.gz" "$$name"); \
		fi; \
		rm -rf "$(DIST_DIR)/$$name"; \
	done

checksums: dist ## Generate SHA256 checksums for the release archives
	cd "$(DIST_DIR)" && shasum -a 256 *.tar.gz *.zip > checksums.txt

clean: ### Clean up build artifacts
	rm -rf bin
	rm -rf dist
