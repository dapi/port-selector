VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := port-selector
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
INSTALL_PATH ?= /usr/local/bin

.PHONY: all build test clean install uninstall

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
