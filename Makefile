BINARY      := openobserve-cli
PKG         := github.com/angelmsger/openobserve-cli
CONSTANTS   := $(PKG)/pkg/constants
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X '$(CONSTANTS).Version=$(VERSION)' \
	-X '$(CONSTANTS).Commit=$(COMMIT)' \
	-X '$(CONSTANTS).BuildTime=$(BUILD_TIME)'

INSTALL_DIR := $(shell go env GOBIN)
ifeq ($(INSTALL_DIR),)
INSTALL_DIR := $(shell go env GOPATH)/bin
endif

SKILL_DIR ?= $(HOME)/.claude/skills

.PHONY: build test e2e lint fmt vet docs install install-skill tidy clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/openobserve-cli

test:
	go test ./...

e2e: build
	./scripts/e2e.sh

lint: fmt vet

fmt:
	gofmt -l -w .

vet:
	go vet ./...

tidy:
	go mod tidy

# Regenerate the CLI reference under docs/cli/ from the cobra command tree.
docs:
	go run ./cmd/gen-docs

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "installed $(INSTALL_DIR)/$(BINARY)"

# Install the companion Skill by copying it into the agent skills dir.
# Prefer `openobserve-cli skill install` (auto-detects agent dirs).
install-skill: build
	./bin/$(BINARY) skill install --dir $(SKILL_DIR)

clean:
	rm -rf bin
