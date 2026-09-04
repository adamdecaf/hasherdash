# hasherdash

Read-only fleet dashboard for ASIC miners. Go + [oat.ink](https://oat.ink/) UI, powered by [asic-rs-go](https://github.com/adamdecaf/asic-rs-go).

Compact table, filters, miner detail, and live charts for a wall monitor (20+ miners). Metric history is stored in **SQLite** so charts survive restarts.

![hasherdash dashboard](docs/images/hasherdash.png)

**Highlights**

- **Stable miner identity** — hostname (preferred) → MAC → serial → IP, so DHCP churn doesn’t split rows or chart history
- **Charts** — per-miner series plus **hashrate by type** (avg/min/max); gaps break lines instead of bridging outages
- **SQLite history** — samples survive restarts; departed miners stay on charts until retention ages them out
- **Docker Hub** — `adamdecaf/hasherdash:<version>` and `:latest` on every `v*` tag

## Quick start (Docker)

The image builds `asic-rs-go` from the public Go module proxy — no separate checkout.

Published images: **[adamdecaf/hasherdash](https://hub.docker.com/r/adamdecaf/hasherdash)** (pushed on git tags `v*`). Latest release: **[v1.1.0](https://github.com/adamdecaf/hasherdash/releases/tag/v1.1.0)**.

**Pull a release (no local build):**

```bash
mkdir -p data
docker pull adamdecaf/hasherdash:v1.1.0
# or: docker pull adamdecaf/hasherdash:latest
docker run --rm --network host \
  -e MINER_SUBNET=192.168.1.0/24 \
  -e SQLITE_PATH=/app/data/hasherdash.db \
  -v "$PWD/data:/app/data" \
  adamdecaf/hasherdash:v1.1.0
```

**Docker Compose (recommended — uses Hub image, no auto-pull on every up):**

```bash
mkdir -p data
# optional: MINER_SUBNET=192.168.1.0/24
docker compose up -d
# upgrade to newest latest:
# docker compose pull && docker compose up -d
# or build from this checkout instead of pulling:
# docker compose up -d --build
```

SQLite is bind-mounted to the host at **`./data/hasherdash.db`** (`./data` → `/app/data` in the container). Create the directory first if needed:

```bash
mkdir -p data
```

Open http://localhost:8080

**Plain `docker run` (local build):**

```bash
make docker
mkdir -p data

docker run --rm --network host \
  -e MINER_SUBNET=192.168.1.0/24 \
  -e SQLITE_PATH=/app/data/hasherdash.db \
  -v "$PWD/data:/app/data" \
  hasherdash:latest
```

**With a config file:**

```bash
cp hasherdash.example.yaml hasherdash.yaml
# edit subnets, then:

docker run --rm --network host \
  -v "$PWD/hasherdash.yaml:/app/hasherdash.yaml:ro" \
  -v "$PWD/data:/app/data" \
  -e CONFIG_FILE=/app/hasherdash.yaml \
  -e SQLITE_PATH=/app/data/hasherdash.db \
  hasherdash:latest
```

`--network host` lets the container scan your LAN. Always mount a host directory on `/app/data` (and keep `SQLITE_PATH` under that path) so metric history survives restarts and is easy to back up.

Subnets are re-scanned every `rescan_interval` (default 30m). Discovered miners stay in the fleet until `miner_ttl` (default 7d) after last successful poll, and are re-polled every `poll_interval` (default 30s).

### Stable identity & charts

Miners are tracked by a **stable identity** so DHCP IP changes don’t split one box into two rows or break chart history:

1. Distinctive **hostname** (preferred, e.g. `nerdqaxe_44C1`)
2. **MAC**
3. **Serial**
4. **IP** (last resort)

Generic factory hostnames like `bitaxe` / `nerdaxe` are ignored for identity so identical defaults don’t collapse separate units. Chart legends prefer hostname labels. When a miner drops off the live fleet (TTL prune), its metric samples stay in SQLite until `history_retention` ages them out, so charts keep the series.

The metrics chart supports per-miner lines and a **Hashrate by type** view that aggregates avg / min / max per make+model. Large gaps in samples break the line (and drop toward zero) instead of drawing a misleading bridge across missing data.

UI refresh interval is separate from backend poll (top-right control, `localStorage`). Chart range defaults to **1d** with options for 4h / 12h / 1d / 3d / 7d / custom. **Refresh** and **Rescan** kick the backend immediately (no separate “Now” control).

## Local run (no Docker)

Needs a built [asic-rs-go](https://github.com/adamdecaf/asic-rs-go) FFI (sibling checkout is simplest):

```bash
# optional: clone next to this repo if you don't already have it
# git clone https://github.com/adamdecaf/asic-rs-go ../asic-rs-go

make ffi   # builds FFI in ../asic-rs-go (override with ASIC_RS_GO=…)

export MINER_SUBNET=192.168.1.0/24
# or: cp hasherdash.example.yaml hasherdash.yaml  # edit subnets

make run
```

By default metrics are written to `./hasherdash.db` in the working directory.

## Metric storage (SQLite)

Successful polls append samples (hashrate, temps, power, efficiency, chips, …) to a SQLite database. Charts and `/api/history` read from that DB.

| Setting | Default | Notes |
|---------|---------|--------|
| `sqlite_path` / `SQLITE_PATH` | `hasherdash.db` (Docker: `/app/data/hasherdash.db`) | File path; parent dirs are created as needed |
| `history_retention` / `HISTORY_RETENTION` | `168h` | Samples older than this are deleted after each poll |
| `sqlite_path: off` / `SQLITE_PATH=off` | — | In-memory ring buffers only (lost on restart) |

**Docker data mount:** bind a host directory to `/app/data` and leave `SQLITE_PATH=/app/data/hasherdash.db`. With Compose this is `./data` → host file `./data/hasherdash.db`.

The image entrypoint runs briefly as root to `chown` the bind-mounted data dir to uid `10001`, then drops privileges. After upgrading the image, recreate the container (`docker compose pull && docker compose up -d`).

If you still hit permission errors on an old image:

```bash
mkdir -p data && sudo chown -R 10001:10001 data
```

Fleet snapshots (live table/detail) stay in memory and are refreshed by the poller. Only **time series** are durable.

## Configuration

**Precedence:** defaults → config file → environment (env wins).

### Config file

Auto-loaded from cwd: `hasherdash.yaml`, `hasherdash.yml`, `config.yaml`, `config.yml`, `hasherdash.json`, `config.json`.

Or set: `-config /path` / `CONFIG_FILE=/path`.

```yaml
http_addr: ":8080"
poll_interval: 30s
rescan_interval: 30m
miner_ttl: 168h
history_retention: 168h
sqlite_path: hasherdash.db
subnets:
  - 192.168.1.0/24
# ips:
#   - 192.168.1.10
```

Full template: `hasherdash.example.yaml`.

### Environment

| Env | Default | Description |
|-----|---------|-------------|
| `MINER_SUBNET` / `MINER_SUBNETS` | — | CIDR(s), comma-separated; re-scanned on `RESCAN_INTERVAL` |
| `MINER_IPS` | — | Comma-separated IPs to poll |
| `MINER_RANGES` | — | asic-rs range strings (e.g. `192.168.1.1-50`) |
| `CONFIG_FILE` | auto | Path to YAML/JSON config |
| `HTTP_ADDR` | `:8080` | Listen address |
| `POLL_INTERVAL` | `30s` | Backend poll interval for known miners |
| `RESCAN_INTERVAL` | `30m` | Full subnet/range discovery cadence (`0` = startup only) |
| `MINER_TTL` | `168h` | Keep offline miners this long after last success (`0` = forever) |
| `HISTORY_RETENTION` | `168h` | How long metric samples are kept for charts |
| `SQLITE_PATH` | `hasherdash.db` | SQLite file for metrics (`off` = memory only) |
| `HISTORY_POINTS` | auto | Optional in-memory ring size when SQLite is off |
| `SCAN_TIMEOUT_SEC` | `8` | Per-miner identify timeout |
| `SCAN_CONCURRENT` | `200` | Discovery concurrency |

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Liveness |
| GET | `/api/meta` | Fleet status + filter facets |
| GET | `/api/miners` | Compact snapshots (keyed by stable `id`) |
| GET | `/api/miners/{id}` | Detail (boards, fans, pools); `{id}` is stable identity, not always IP |
| GET | `/api/history?metric=hashrate&ids=a,b&window=1d` | Time series (`window`, or `since`/`until` RFC3339) |
| POST | `/api/rescan` | Kick a full subnet/range discovery + poll now |

Stored metrics: `hashrate`, `temp`, `asic_temp`, `asic_temp_min`, `vr_temp`, `vr_temp_min`, `wattage`, `efficiency`, `chips`, `best_diff`, `session_diff`.

The UI also offers **`hashrate_by_type`** (client-side avg/min/max aggregation over per-miner hashrate). **`best_diff`** / **`session_diff`** come from asic-rs `best_share` / `session_best_share`.

## Project layout

```
cmd/hasherdash/     entrypoint
internal/          api, config, poller, store (SQLite metrics)
web/static/        oat.ink UI
docs/              GitHub Pages site
Dockerfile         multi-stage (module proxy + Rust FFI + cgo)
```

## Notes

- **Read-only** — no restart / pool / power control in the UI.
- Canvas charts (no Chart.js); styling via [oat](https://github.com/knadh/oat).
- Metric history uses pure-Go SQLite ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)); no extra system library.
- Docker builds pull `github.com/adamdecaf/asic-rs-go` and compile the FFI inside the image.
- CI builds the binary and Docker image on every push/PR.
- Release tags `v*` publish two Hub tags: `adamdecaf/hasherdash:<version>` (e.g. `1.1.0` from `v1.1.0`) and `adamdecaf/hasherdash:latest`. Compose uses the Hub image; run `docker compose pull` to upgrade.
- Local publish: `make docker-push` (after `docker login`; override with `DOCKER_IMAGE=` / `VERSION=`).
- GitHub Actions secrets for Hub publish: `DOCKER_USERNAME`, `DOCKER_PASSWORD` (Hub password or access token with write access).


