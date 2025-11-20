.PHONY: build debug clean rpm

# Get the module path from go.mod
MODULE    := $(shell go list -m)

# Build metadata from VCS + shell
VERSION   := $(shell git describe --tags --always --dirty)
COMMIT    := $(shell git rev-parse --short HEAD)
BUILDDATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X '$(MODULE)/internal/config.Version=$(VERSION)' \
          -X '$(MODULE)/internal/config.GitCommit=$(COMMIT)' \
          -X '$(MODULE)/internal/config.BuildDate=$(BUILDDATE)'

build:
	go build -ldflags "$(LDFLAGS)" -o zmux-server ./cmd/zmux-server

debug:
	go build -gcflags "all=-N -l" -ldflags "$(LDFLAGS)" \
		-o zmux-server_g ./cmd/zmux-server

rpm: build
	VERSION=$(VERSION) $(HOME)/go/bin/nfpm package --packager rpm --config nfpm.yaml
