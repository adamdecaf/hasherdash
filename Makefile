.PHONY: run build test docker tidy ffi

# Local path to asic-rs-go (override as needed)
ASIC_RS_GO ?= ../asic-rs-go

export CGO_ENABLED ?= 1

# Build asic-rs FFI in the sibling checkout (required for local run/build).
ffi:
	$(MAKE) -C $(ASIC_RS_GO) ffi

run: ## run against real miners (requires built FFI + config)
	go run ./cmd/minerdash $(if $(CONFIG),-config $(CONFIG),)

build:
	go build -o bin/minerdash ./cmd/minerdash

test:
	go test ./internal/config ./internal/store ./internal/axetemp -count=1

tidy:
	go mod tidy

docker:
	docker build \
		-f Dockerfile \
		--build-context asicrsgo=$(ASIC_RS_GO) \
		-t minerdash:latest .
