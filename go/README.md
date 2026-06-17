# perforce-prom-exporter (Go)

A direct port of the Python exporter as a single self-contained Go binary —
same metrics, same labels, same scrape behaviour. Designed to live next to
`p4d` on a Perforce host where installing Docker is awkward but dropping in
a ~10 MB binary is not.

The Python version (in the repo root) is still authoritative for the
docker-compose lab. This Go version exists to make field deployment trivial:

```sh
scp dist/perforce-exporter-linux-amd64 admin@p4-primary:/usr/local/bin/perforce-exporter
```

## Build

```sh
make            # vet + test + build (for the host architecture)
make dist       # cross-compile linux/amd64, linux/arm64, darwin/arm64, windows/amd64
```

All builds use `CGO_ENABLED=0` so the binaries are statically linked and
have no glibc runtime dependency.

## Run

```sh
export PERFORCE_TARGETS="primary=127.0.0.1:1666,replica=127.0.0.1:1667"
export P4_BIN=/path/to/p4
./perforce-exporter
```

| Variable             | Default | Purpose                                            |
| -------------------- | ------- | -------------------------------------------------- |
| `PERFORCE_TARGETS`   | —       | Comma-separated `name=host:port` list (required).  |
| `EXPORTER_PORT`      | `9117`  | HTTP listen port.                                  |
| `P4_BIN`             | `p4`    | Path to the p4 binary.                             |
| `P4_TIMEOUT_SECONDS` | `10`    | Per-command timeout.                               |

Other `P4*` environment variables are inherited by the underlying `p4`
process, so `P4USER`, `P4TICKETS`, `P4CONFIG` etc. work the way you'd
expect from the operator side.

## Layout

```
go/
├── cmd/perforce-exporter/    # main + portable signal handling
└── internal/
    ├── config/               # env → Config
    ├── p4/                   # exec wrapper, never panics on p4 failure
    ├── parsers/              # pure functions, tested via testdata/
    └── collector/            # prometheus.Collector implementation
```

## Tests

```sh
go test ./...
```

## Cutting a release

GoReleaser config lives at `.goreleaser.yaml`; the workflow is at
`../.github/workflows/release.yml` and fires on any `v*` tag push.

```sh
git tag v0.1.0
git push origin v0.1.0
```

That builds binaries for linux/{amd64,arm64}, darwin/arm64, windows/amd64,
tars/zips them with `README.md` and `LICENSE` alongside, generates a
`checksums.txt`, and publishes a GitHub Release with auto-generated notes.

To dry-run locally without publishing (requires
`brew install goreleaser`):

```sh
goreleaser release --snapshot --clean
```

Built binaries print `--version`, with the tag, short SHA and build date
baked in via `-ldflags="-X main.version=…"`.

The fixture files under `internal/parsers/testdata/` are the same captured
`p4 …` outputs the Python tests use, copied verbatim so the two
implementations parse identical inputs.

## What's the same as the Python version

- Metric names, label sets, scrape order
- Compatible with the dashboard at `dashboards/perforce-overview.json` and
  the rules at `prometheus/rules/perforce.rules.yml` in the repo root —
  point Prometheus at this exporter's `:9117/metrics` and nothing else
  needs to change.

## What's different

- Single binary, no Python runtime, no `pip install`.
- Cross-compiles to Windows.
- A bit chunkier on disk (~10 MB) than the Python source, but no runtime
  to install on the host.
