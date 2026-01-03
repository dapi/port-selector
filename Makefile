VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := port-selector
# Strip symbols (-s) and debug info (-w) for smaller release binaries
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
INSTALL_PATH ?= /usr/local/bin

# Disable CGO for static binaries and macOS compatibility
export CGO_ENABLED=0

.PHONY: all build test clean install uninstall release-snapshot

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/port-selector

test:
	go test -v -race ./...

clean:
	rm -f $(BINARY)

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
