# syntax=docker/dockerfile:1

# Multi-stage image: clone asic-rs, build the Rust FFI, then link hasherdash with cgo.
# Pin to the asic-rs#364 merge on master until a go/v* module tag exists.

ARG ASIC_RS_REPO=https://github.com/256foundation/asic-rs.git
ARG ASIC_RS_REF=master
# Pin so Docker layer cache invalidates when the Go bindings commit moves.
ARG ASIC_RS_SHA=6063638eb001dff85dc074dcc7473e68159721ab

# -----------------------------------------------------------------------------
# Stage 1: clone asic-rs (full tree — asic-rs-ffi is a workspace crate)
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS asicrs-src
ARG ASIC_RS_REPO
ARG ASIC_RS_REF
ARG ASIC_RS_SHA
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git \
    && rm -rf /var/lib/apt/lists/*
# Fetch the pinned SHA (not branch HEAD) so the image stays reproducible
# after asic-rs master moves.
RUN git init /src/asic-rs \
 && git -C /src/asic-rs remote add origin "${ASIC_RS_REPO}" \
 && git -C /src/asic-rs fetch --depth 1 origin "${ASIC_RS_SHA}" \
 && git -C /src/asic-rs checkout --detach FETCH_HEAD \
 && git -C /src/asic-rs rev-parse HEAD | grep -q "^${ASIC_RS_SHA}"

# -----------------------------------------------------------------------------
# Stage 2: build asic-rs FFI (Rust) for linux
# -----------------------------------------------------------------------------
FROM rust:1-bookworm AS ffi
# `make -C go ffi` runs `go run ./internal/buildffi` to copy Cargo artifacts.
COPY --from=golang:1.27-bookworm /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake pkg-config \
    && rm -rf /var/lib/apt/lists/*
COPY --from=asicrs-src /src/asic-rs /src/asic-rs
WORKDIR /src/asic-rs
# cbindgen writes asic-rs-ffi/include/; `make -C go ffi` copies that header
# and the built library into go/asic_go/{include,lib} for cgo.
RUN make -C go ffi PROFILE=release \
 && mkdir -p /out/lib /out/include \
 && cp go/asic_go/lib/libasic_rs_ffi.so /out/lib/ \
 && cp go/asic_go/lib/libasic_rs_ffi.a /out/lib/ \
 && cp go/asic_go/include/asic_rs_ffi.h /out/include/

# -----------------------------------------------------------------------------
# Stage 3: build hasherdash (Go + cgo)
# -----------------------------------------------------------------------------
FROM golang:1.27-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=asicrs-src /src/asic-rs /src/asic-rs
COPY --from=ffi /out/lib/ /src/asic-rs/go/asic_go/lib/
COPY --from=ffi /out/include/ /src/asic-rs/go/asic_go/include/

WORKDIR /src/hasherdash
COPY . .
# Prefer the in-image asic-rs tree (with built .so/.a) over the module proxy.
RUN go mod edit -replace=github.com/256foundation/asic-rs/go=/src/asic-rs/go

ENV CGO_ENABLED=1
ENV GOTOOLCHAIN=auto
ENV CGO_CFLAGS="-I/src/asic-rs/go/asic_go/include"
ENV CGO_LDFLAGS="-L/src/asic-rs/go/asic_go/lib -lasic_rs_ffi -lm -ldl -lpthread -Wl,-rpath,/usr/local/lib"
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
