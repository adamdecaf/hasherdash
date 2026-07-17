# minerdash

Read-only fleet dashboard for ASIC miners. Go backend + [oat.ink](https://oat.ink/) frontend, powered by [asic-rs-go](https://github.com/adamdecaf/asic-rs-go).

Built for **20+ miners on a wall monitor**: compact sortable table, rich filters, selected-miner detail, and a live multi-series chart (hashrate / temp / power / …) that refreshes on a configurable interval (default 30s).

![minerdash dashboard](docs/images/minerdash.png)

## Layout

```
┌──────────────────┬────────────────────────────────────────────┐
│ Filters          │ Metrics chart (all / filtered / selected)  │
│ brand, TH/s, °C, ├────────────────────────────────────────────┤
│ chips, FW, …     │ Compact fleet table (sort any column)      │
├──────────────────┤                                            │
│ Miner detail     │                                            │
│ (selection)      │                                            │
└──────────────────┴────────────────────────────────────────────┘
```

## CI

GitHub Actions on every push/PR:

1. Builds the [asic-rs-go](https://github.com/adamdecaf/asic-rs-go) FFI, runs unit tests, and compiles `minerdash` with cgo
2. Builds the multi-stage Docker image (`Dockerfile` + `asicrsgo` build context) and smoke-tests the binary inside the image

## Quick start (real miners)

1. Build the FFI in a checkout of `asic-rs-go`:

   ```bash
   make ffi
   # or: make -C ../asic-rs-go ffi
   ```

2. Point this module at it (already the default via `replace` for local dev):

   ```bash
   go mod edit -replace=github.com/adamdecaf/asic-rs-go=../asic-rs-go
   go mod tidy
   ```

3. Create a config file:

   ```bash
   cp minerdash.example.yaml minerdash.yaml
   # edit subnets / ips
   make run
   # or: ./bin/minerdash -config /path/to/minerdash.yaml
   ```

   ```yaml
   # minerdash.yaml
   poll_interval: 30s
   subnets:
     - 192.168.1.0/24
   ```

   Environment variables still work and **override** the file when set:

   ```bash
   export MINER_SUBNET=192.168.1.0/24   # one or comma-separated CIDRs
   export POLL_INTERVAL=30s
   make run
   ```

Full multi-stage image (builds Rust FFI + Go binary):

```bash
# Docker BuildKit required (build-context for sibling asic-rs-go)
make docker && \
docker run --rm -p 8080:8080 --network host \
  -v "$PWD/minerdash.yaml:/app/minerdash.yaml:ro" \
  -e CONFIG_FILE=/app/minerdash.yaml \
  minerdash:latest
```

## Configuration

### Config file (YAML or JSON)

Looked up automatically (cwd): `minerdash.yaml`, `minerdash.yml`, `config.yaml`,
`config.yml`, `minerdash.json`, `config.json`.

Or set explicitly:

- CLI: `-config /path/to/minerdash.yaml`
- Env: `CONFIG_FILE=/path/to/minerdash.yaml`

Example:

```yaml
http_addr: ":8080"
poll_interval: 30s
history_points: 240
scan_timeout_sec: 8
scan_concurrent: 200

# Scan these CIDR blocks once, then poll discovered miners
subnets:
  - 192.168.1.0/24
  - 10.0.0.0/24

# Optional asic-rs range strings
# ranges:
#   - 192.168.1.1-50

# Optional always-on IPs
# ips:
#   - 192.168.1.10
```

See `minerdash.example.yaml` for a full template.

**Precedence:** defaults → config file → environment (env wins when set).

Subnets/ranges are scanned **once** at startup (or first poll); discovered IPs
are cached and re-polled every `poll_interval` without re-scanning the whole LAN.

### Environment variables

| Env | Default | Description |
|-----|---------|-------------|
| `CONFIG_FILE` | auto | Path to YAML/JSON config |
| `HTTP_ADDR` | `:8080` | Listen address |
| `POLL_INTERVAL` | `30s` | Backend poll interval (`30`, `30s`, `1m`) |
| `HISTORY_POINTS` | `240` | Ring buffer length per metric (~2h @ 30s) |
| `MINER_IPS` | — | Comma-separated IPs to poll |
| `MINER_SUBNET` / `MINER_SUBNETS` | — | CIDR(s), comma-separated; scanned once |
| `MINER_RANGES` | — | asic-rs range strings, comma-separated |
| `SCAN_TIMEOUT_SEC` | `8` | Per-miner identification timeout |
| `SCAN_CONCURRENT` | `200` | Discovery concurrency |

The UI refresh interval is independent (top-right control, stored in `localStorage`).

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Liveness |
| GET | `/api/meta` | Fleet status + filter facets |
| GET | `/api/miners` | Compact snapshots |
| GET | `/api/miners/{ip}` | Detail (boards, fans, pools) |
| GET | `/api/history?metric=hashrate&ids=a,b` | Time series |

Metrics: `hashrate`, `temp`, `asic_temp`, `vr_temp`, `wattage`, `efficiency`, `chips`.

## Project layout

```
cmd/minerdash/          entrypoint
internal/
  api/                  HTTP handlers
  config/               file + env config
  models/               JSON DTOs
  poller/               asic-rs discovery + poll
  store/                in-memory cache + history rings
web/static/             oat.ink UI (HTML/CSS/JS)
Dockerfile              multi-stage image (Rust FFI + cgo)
```

## Notes

- **Read-only** for now — no restart / pool / power control in the UI.
- Chart is canvas-based (no Chart.js) to stay vanilla.
- Styling via [oat](https://github.com/knadh/oat) CDN (`@knadh/oat`).
- Requires cgo and a built `asic-rs-go` FFI library for real miner access.
