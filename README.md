# minerdash

Read-only fleet dashboard for ASIC miners. Go + [oat.ink](https://oat.ink/) UI, powered by [asic-rs-go](https://github.com/adamdecaf/asic-rs-go).

Compact table, filters, miner detail, and live charts for a wall monitor (20+ miners).

![minerdash dashboard](docs/images/minerdash.png)

## Quick start (Docker)

The image builds `asic-rs-go` from the public Go module proxy — no separate checkout.

**Option A — env var (fastest):**

```bash
make docker

docker run --rm -p 8080:8080 --network host \
  -e MINER_SUBNET=192.168.1.0/24 \
  minerdash:latest
```

Open http://localhost:8080

**Option B — config file:**

```bash
cp minerdash.example.yaml minerdash.yaml
# set your CIDR under subnets:
#   subnets:
#     - 192.168.1.0/24

make docker

docker run --rm -p 8080:8080 --network host \
  -v "$PWD/minerdash.yaml:/app/minerdash.yaml:ro" \
  -e CONFIG_FILE=/app/minerdash.yaml \
  minerdash:latest
```

`--network host` lets the container scan your LAN.

Subnets are scanned **once** at startup; discovered miners are re-polled every `poll_interval` (default 30s).

## Local run (no Docker)

Needs a built [asic-rs-go](https://github.com/adamdecaf/asic-rs-go) FFI (sibling checkout is simplest):

```bash
# optional: clone next to this repo if you don't already have it
# git clone https://github.com/adamdecaf/asic-rs-go ../asic-rs-go

make ffi   # builds FFI in ../asic-rs-go (override with ASIC_RS_GO=…)

export MINER_SUBNET=192.168.1.0/24
# or: cp minerdash.example.yaml minerdash.yaml  # edit subnets

make run
```

## Configuration

**Precedence:** defaults → config file → environment (env wins).

### Config file

Auto-loaded from cwd: `minerdash.yaml`, `minerdash.yml`, `config.yaml`, `config.yml`, `minerdash.json`, `config.json`.

Or set: `-config /path` / `CONFIG_FILE=/path`.

```yaml
http_addr: ":8080"
poll_interval: 30s
subnets:
  - 192.168.1.0/24
# ips:
#   - 192.168.1.10
```

Full template: `minerdash.example.yaml`.

### Environment

| Env | Default | Description |
|-----|---------|-------------|
| `MINER_SUBNET` / `MINER_SUBNETS` | — | CIDR(s), comma-separated; scanned once |
| `MINER_IPS` | — | Comma-separated IPs to poll |
| `MINER_RANGES` | — | asic-rs range strings (e.g. `192.168.1.1-50`) |
| `CONFIG_FILE` | auto | Path to YAML/JSON config |
| `HTTP_ADDR` | `:8080` | Listen address |
| `POLL_INTERVAL` | `30s` | Backend poll interval |
| `HISTORY_POINTS` | `240` | History ring length per metric |
| `SCAN_TIMEOUT_SEC` | `8` | Per-miner identify timeout |
| `SCAN_CONCURRENT` | `200` | Discovery concurrency |

UI refresh interval is separate (top-right control, `localStorage`).

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
cmd/minerdash/     entrypoint
internal/          api, config, poller, store
web/static/        oat.ink UI
Dockerfile         multi-stage (module proxy + Rust FFI + cgo)
```

## Notes

- **Read-only** — no restart / pool / power control in the UI.
- Canvas charts (no Chart.js); styling via [oat](https://github.com/knadh/oat).
- Docker builds pull `github.com/adamdecaf/asic-rs-go` and compile the FFI inside the image.
- CI builds the binary and Docker image on every push/PR.
