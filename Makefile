VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := port-selector
DIST_DIR ?= dist
# Strip symbols (-s) and debug info (-w) for smaller release binaries
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
INSTALL_PATH ?= /usr/local/bin

# Disable CGO for static binaries and macOS compatibility
export CGO_ENABLED=0

.PHONY: all build build-darwin-arm64 test clean install uninstall fmt lint release-snapshot release-check release-macos-silicon release-darwin-arm64

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/port-selector

build-darwin-arm64:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-darwin-arm64 ./cmd/port-selector

test:
	go test -v -race ./...

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)

install: build
	sudo install -m 755 $(BINARY) $(INSTALL_PATH)/$(BINARY)

uninstall:
	sudo rm -f $(INSTALL_PATH)/$(BINARY)

fmt:
	go fmt ./...

lint:
	golangci-lint run

# GoReleaser commands
release-snapshot:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check

# Local release artifact for macOS Apple Silicon.
release-macos-silicon: build-darwin-arm64

release-darwin-arm64: release-macos-silicon
