VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := port-selector
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build test clean install

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/port-selector

test:
	go test -v -race ./...

clean:
	rm -f $(BINARY)

install: build
	mv $(BINARY) /usr/local/bin/

fmt:
	go fmt ./...

lint:
	golangci-lint run
