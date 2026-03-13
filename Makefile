GO=go
GOLANGCI_LINT=golangci-lint
GO_TEST=$(GO) test
GO_BUILD=$(GO) build
GO_CLEAN=$(GO) clean
GO_MOD=$(GO) mod

GO_BUILD_DIR=./build
SH_TOOLS_DIR=./tools/sh

SOURCES=$(shell find . -type f -name "*.go" -print)
SOURCE_DIRS=$(shell go list -f "{{.Dir}}" ./...)
BUILD_SHA:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "0000000")
BUILD_VERSION:=$(shell cat ./VERSION 2>/dev/null || echo "v0.0.0")
DIST_DIR=./dist
MY_BIN=wile-goast

DOCKER_IMAGE ?= wile-goast
DOCKER_PLATFORM ?=
DOCKER_SHELL ?=

LDFLAGS=-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)"

# Detect host OS and architecture using Go conventions
HOST_OS := $(shell $(GO) env GOOS)
RAW_ARCH := $(shell uname -m)
ifeq ($(RAW_ARCH),x86_64)
HOST_ARCH := amd64
else
HOST_ARCH := $(RAW_ARCH)
endif

# Build the wile-goast binary for the current platform to ./dist/{os}/{arch}/wile-goast.
#   make build
.PHONY: build
build: $(DIST_DIR)/$(HOST_OS)/$(HOST_ARCH)/$(MY_BIN)
	@ln -sf $(HOST_OS)/$(HOST_ARCH)/$(MY_BIN) $(DIST_DIR)/$(MY_BIN)
	@echo "Created symlink: $(DIST_DIR)/$(MY_BIN) -> $(HOST_OS)/$(HOST_ARCH)/$(MY_BIN)"

$(DIST_DIR)/%/$(MY_BIN): $(SOURCES)
	$(eval OS_ARCH := $(subst /, ,$*))
	$(eval TARGET_OS := $(word 1,$(OS_ARCH)))
	$(eval TARGET_ARCH := $(word 2,$(OS_ARCH)))
	@mkdir -p $(DIST_DIR)/$*
	GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GO_BUILD) -o $(DIST_DIR)/$*/$(MY_BIN) $(LDFLAGS) ./cmd/wile-goast

.PHONY: build-darwin-arm64
build-darwin-arm64: $(DIST_DIR)/darwin/arm64/$(MY_BIN)

.PHONY: build-darwin-amd64
build-darwin-amd64: $(DIST_DIR)/darwin/amd64/$(MY_BIN)

.PHONY: build-linux-arm64
build-linux-arm64: $(DIST_DIR)/linux/arm64/$(MY_BIN)

.PHONY: build-linux-amd64
build-linux-amd64: $(DIST_DIR)/linux/amd64/$(MY_BIN)

.PHONY: build-all
build-all: build-darwin-arm64 build-darwin-amd64 build-linux-arm64 build-linux-amd64

# Compile tests for all packages without running them.
#   make buildtest
.PHONY: buildtest
buildtest:
	for dir in $(SOURCE_DIRS); do \
	    if [ -d "$$dir" ]; then \
	        $(GO_TEST) -c -o /dev/null $$dir/...; \
	    fi \
	done

# ── CI: everything that must pass before merge ──────────────────────
#   make ci
#   make ci SKIP_LINT=1
.PHONY: ci
ci: $(if $(SKIP_LINT),,lint) build test covercheck verify-mod
	@echo "CI passed"

# Run all tests with verbose output.
#   make test
.PHONY: test
test:
	$(GO_TEST) ./...

# Run all benchmarks with memory allocation statistics.
#   make bench
.PHONY: bench
bench:
	$(GO_TEST) -bench=. -benchmem ./...

# Run tests with coverage and print per-function coverage summary.
#   make cover
.PHONY: cover
cover:
	@mkdir -p $(GO_BUILD_DIR)
	$(GO_TEST) -coverprofile=$(GO_BUILD_DIR)/coverage.out ./...
	$(GO) tool cover -func=$(GO_BUILD_DIR)/coverage.out

# Run tests with coverage and open an HTML report in the browser.
#   make coverhtml
.PHONY: coverhtml
coverhtml:
	@mkdir -p $(GO_BUILD_DIR)
	$(GO_TEST) -coverprofile=$(GO_BUILD_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(GO_BUILD_DIR)/coverage.out -o $(GO_BUILD_DIR)/coverage.html
	@echo "Coverage report: $(GO_BUILD_DIR)/coverage.html"
	open $(GO_BUILD_DIR)/coverage.html 2>/dev/null || xdg-open $(GO_BUILD_DIR)/coverage.html 2>/dev/null || echo "Open $(GO_BUILD_DIR)/coverage.html in your browser"

# Run tests with coverage and enforce per-package threshold (80%).
#   make covercheck
.PHONY: covercheck
covercheck:
	@mkdir -p $(GO_BUILD_DIR)
	$(GO_TEST) -coverprofile=$(GO_BUILD_DIR)/coverage.out ./...
	@bash $(SH_TOOLS_DIR)/covercheck.sh 80 $(GO_BUILD_DIR)/coverage.out

# Run golangci-lint on all packages.
#   make lint
.PHONY: lint
lint:
	$(GOLANGCI_LINT) -v run ./...

# Run golangci-lint with --fix to auto-correct fixable issues.
#   make fix
.PHONY: fix
fix:
	$(GOLANGCI_LINT) -v run --fix ./...

# Format all Go source files via golangci-lint.
#   make format
.PHONY: format
format:
	$(GOLANGCI_LINT) -v fmt -v ./...

# Tidy go.mod: add missing and remove unused dependencies.
#   make tidy
.PHONY: tidy
tidy:
	$(GO_MOD) tidy -e -x

# Remove all generated artifacts.
#   make clean
.PHONY: clean
clean: buildclean testclean modclean
	for dir in "$(DIST_DIR)" "$(GO_BUILD_DIR)"; do \
	    if [ -e "$$dir" ]; then rm -rvf "$$dir"; fi \
	done; \
	for dir in $(SOURCE_DIRS); do \
	    if [ -e "$$dir" ]; then find "$$dir" -name "*.test" -type f -exec rm -v \{\} \; ; fi \
	done

.PHONY: buildclean
buildclean:
	$(GO_CLEAN) -cache

.PHONY: testclean
testclean:
	$(GO_CLEAN) -testcache -fuzzcache

.PHONY: modclean
modclean:
	$(GO_CLEAN) -modcache

# Create an annotated git tag from the version in ./VERSION.
#   make tag
.PHONY: tag
tag:
	git tag -a $(BUILD_VERSION) -m "Release $(BUILD_VERSION)"
	@echo "Created tag $(BUILD_VERSION)"

.PHONY: bump-major
bump-major:
	$(SH_TOOLS_DIR)/bump-version.sh major

.PHONY: bump-minor
bump-minor:
	$(SH_TOOLS_DIR)/bump-version.sh minor

.PHONY: bump-patch
bump-patch:
	$(SH_TOOLS_DIR)/bump-version.sh patch

# Verify go.sum integrity.
#   make verify-mod
.PHONY: verify-mod
verify-mod:
	$(GO_MOD) verify

# Build the Docker image.
#   make docker-build
.PHONY: docker-build
docker-build:
	DOCKER_IMAGE=$(DOCKER_IMAGE) DOCKER_PLATFORM=$(DOCKER_PLATFORM) $(SH_TOOLS_DIR)/docker-build.sh

# Open an interactive shell inside the Docker container.
#   make docker-shell
.PHONY: docker-shell
docker-shell:
	DOCKER_IMAGE=$(DOCKER_IMAGE) $(SH_TOOLS_DIR)/docker-shell.sh $(DOCKER_SHELL)
