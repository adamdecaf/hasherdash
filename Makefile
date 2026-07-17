.PHONY: run build test docker tidy ffi

# Optional local path for asic-rs-go when developing against a sibling checkout.
# Docker does not need this — it pulls github.com/adamdecaf/asic-rs-go via the module proxy.
ASIC_RS_GO ?= ../asic-rs-go

export CGO_ENABLED ?= 1

# Build asic-rs FFI for local run/build (sibling checkout; override with ASIC_RS_GO=).
# Docker does not use this target — the image builds FFI from the module proxy.
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

# Build image from this repo only (asic-rs-go comes from the public Go module proxy).
docker:
	docker build -f Dockerfile -t minerdash:latest .
