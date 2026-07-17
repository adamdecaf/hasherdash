# syntax=docker/dockerfile:1

# -----------------------------------------------------------------------------
# Stage 1: build asic-rs FFI (Rust) for linux
# -----------------------------------------------------------------------------
FROM rust:1-bookworm AS ffi
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
# Copy asic-rs-go from sibling path provided at build time via build context.
# Build with: docker build -f Dockerfile --build-context asicrsgo=../asic-rs-go .
COPY --from=asicrsgo / /src/asic-rs-go
WORKDIR /src/asic-rs-go/asic-rs-ffi
RUN cargo build --release \
 && mkdir -p /out/lib /out/include \
 && cp target/release/libasic_rs_ffi.so /out/lib/ \
 && cp target/release/libasic_rs_ffi.a /out/lib/ \
 && cp include/asic_rs_ffi.h /out/include/

# -----------------------------------------------------------------------------
# Stage 2: build minerdash (Go + cgo)
# -----------------------------------------------------------------------------
FROM golang:1.23-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY --from=asicrsgo / /src/asic-rs-go
COPY --from=ffi /out/lib/ /src/asic-rs-go/asicrs/lib/
COPY --from=ffi /out/include/ /src/asic-rs-go/asicrs/include/

WORKDIR /src/minerdash
COPY go.mod ./
# Local replace so Docker does not need the module on the public proxy.
RUN printf '\nreplace github.com/adamdecaf/asic-rs-go => /src/asic-rs-go\n' >> go.mod
COPY . .

ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-I/src/asic-rs-go/asicrs/include"
ENV CGO_LDFLAGS="-L/src/asic-rs-go/asicrs/lib -lasic_rs_ffi -lm -ldl -lpthread -Wl,-rpath,/usr/local/lib"
RUN go mod tidy \
 && go build -trimpath -ldflags="-s -w" -o /out/minerdash ./cmd/minerdash

# -----------------------------------------------------------------------------
# Stage 3: runtime
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /app --shell /usr/sbin/nologin minerdash

COPY --from=ffi /out/lib/libasic_rs_ffi.so /usr/local/lib/
RUN ldconfig
COPY --from=build /out/minerdash /usr/local/bin/minerdash

USER minerdash
WORKDIR /app
ENV HTTP_ADDR=:8080
ENV POLL_INTERVAL=30s
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/minerdash"]
