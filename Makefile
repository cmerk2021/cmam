BINARY  := cmam
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/cmerk2021/cmam/internal/version.Version=$(VERSION) \
	-X github.com/cmerk2021/cmam/internal/version.Commit=$(COMMIT) \
	-X github.com/cmerk2021/cmam/internal/version.Date=$(DATE)

.PHONY: build test lint install snapshot

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

test:
	go vet ./...
	go test ./...

lint:
	golangci-lint run

install: build
	mkdir -p $(HOME)/.local/bin
	install -m 0755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

snapshot:
	goreleaser release --snapshot --clean
