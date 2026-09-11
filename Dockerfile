# syntax=docker/dockerfile:1

# Multi-stage image: clone asic-rs, build the Rust FFI, then link hasherdash with cgo.
# Until 256foundation/asic-rs#364 lands, default to the adamdecaf fork branch.

ARG ASIC_RS_REPO=https://github.com/adamdecaf/asic-rs.git
ARG ASIC_RS_REF=feat/go-bindings

# -----------------------------------------------------------------------------
# Stage 1: clone asic-rs (full tree — asic-rs-ffi is a workspace crate)
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS asicrs-src
ARG ASIC_RS_REPO
ARG ASIC_RS_REF
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${ASIC_RS_REF}" "${ASIC_RS_REPO}" /src/asic-rs

# -----------------------------------------------------------------------------
# Stage 2: build asic-rs FFI (Rust) for linux
# -----------------------------------------------------------------------------
FROM rust:1-bookworm AS ffi
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake pkg-config \
    && rm -rf /var/lib/apt/lists/*
COPY --from=asicrs-src /src/asic-rs /src/asic-rs
WORKDIR /src/asic-rs
RUN cargo build -p asic-rs-ffi --release \
 && mkdir -p /out/lib /out/include \
 && cp target/release/libasic_rs_ffi.so /out/lib/ \
 && cp target/release/libasic_rs_ffi.a /out/lib/ \
 && cp go/asicrs/include/asic_rs_ffi.h /out/include/

# -----------------------------------------------------------------------------
# Stage 3: build hasherdash (Go + cgo)
# -----------------------------------------------------------------------------
FROM golang:1.27-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=asicrs-src /src/asic-rs /src/asic-rs
COPY --from=ffi /out/lib/ /src/asic-rs/go/asicrs/lib/
COPY --from=ffi /out/include/ /src/asic-rs/go/asicrs/include/

WORKDIR /src/hasherdash
COPY . .
# Prefer the in-image asic-rs tree (with built .so/.a) over the module proxy.
RUN go mod edit -replace=github.com/256foundation/asic-rs/go=/src/asic-rs/go

ENV CGO_ENABLED=1
ENV GOTOOLCHAIN=auto
ENV CGO_CFLAGS="-I/src/asic-rs/go/asicrs/include"
ENV CGO_LDFLAGS="-L/src/asic-rs/go/asicrs/lib -lasic_rs_ffi -lm -ldl -lpthread -Wl,-rpath,/usr/local/lib"
RUN go mod download \
 && go build -trimpath -ldflags="-s -w" -o /out/hasherdash ./cmd/hasherdash

# -----------------------------------------------------------------------------
# Stage 4: runtime
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS runtime
# util-linux provides setpriv (drop privileges after fixing data dir ownership).
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates util-linux \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /app --shell /usr/sbin/nologin hasherdash \
    && mkdir -p /app/data \
    && chown hasherdash:hasherdash /app/data

COPY --from=ffi /out/lib/libasic_rs_ffi.so /usr/local/lib/
RUN ldconfig
COPY --from=build /out/hasherdash /usr/local/bin/hasherdash
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh

# Start as root so the entrypoint can chown bind-mounted ./data, then drop to hasherdash.
WORKDIR /app
ENV HTTP_ADDR=:8080
ENV POLL_INTERVAL=30s
ENV SQLITE_PATH=/app/data/hasherdash.db
ENV HASHERDASH_DATA_DIR=/app/data
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
