# golang-api-template — development tasks
GO      ?= go
BINARY  ?= golang-api-template

# Build metadata injected via -ldflags (single source of truth).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -w -s -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all fmt vet tidy lint test test-race cover cover-html build run clean image print-build

all: fmt vet lint test

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy && $(GO) mod verify

lint:
	golangci-lint run ./...

test:
	$(GO) test -count=1 -race -shuffle=on ./...

test-race: test

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

cover-html: cover
	open coverage.html 2>/dev/null || true

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/api
	@echo "built bin/$(BINARY) $(VERSION) ($(COMMIT))"

# Local run without telemetry envs boots in disabled mode (health/metrics OK).
run:
	$(GO) run ./cmd/api

clean:
	rm -f coverage.out coverage.html
	rm -rf bin/

image:
	podman build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t localhost/golang-api-template:dev .

print-build:
	@echo "VERSION=$(VERSION) COMMIT=$(COMMIT) DATE=$(DATE)"
