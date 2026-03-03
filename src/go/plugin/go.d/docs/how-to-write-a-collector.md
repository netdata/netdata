# How to Write a go.d Collector (V2)

This guide is intentionally minimal. The source of truth is existing collectors in
`src/go/plugin/go.d/collector/`.

## Scope

- New collectors should use `collectorapi.CollectorV2`.
- `CollectorV1` is legacy compatibility for existing collectors; do not use it for new work.

## Start from a Real Collector

- Minimal V2 example: `src/go/plugin/go.d/collector/ping/collector.go`.
- V2 with function methods: `src/go/plugin/go.d/collector/mysql/collector.go`.

## Required Collector Pieces

For a new collector `foo`, create `src/go/plugin/go.d/collector/foo/` with:

- `collector.go`:
    - `collectorapi.Register("foo", collectorapi.Creator{...})`
    - `CreateV2`, `Config`, optional `JobConfigSchema`, optional function wiring
    - collector struct embedding `collectorapi.Base`
    - `Configuration()`, `Init()`, `Check()`, `Collect()`, `Cleanup()`
    - `MetricStore()` and `ChartTemplateYAML()`
- `charts.yaml`: chart template consumed by the V2 chart engine.
- `config_schema.json`: JSON schema for collector job config.
- Any helper files you need (`collect.go`, `init.go`, `types.go`, `testdata/`, tests).

## Repository Wiring Checklist

1. Import the collector in `src/go/plugin/go.d/collector/init.go`.
2. Add default config file: `src/go/plugin/go.d/config/go.d/foo.conf`.
3. Add toggle in `src/go/plugin/go.d/config/go.d.conf` under `modules:` (legacy key name kept for compatibility).
4. Add/update the entry in `src/go/plugin/go.d/README.md` (available collectors).

## V2 Runtime Contract

- `Init(context.Context) error`
- `Check(context.Context) error`
- `Collect(context.Context) error`
- `Cleanup(context.Context)`
- `MetricStore() metrix.CollectorStore`
- `ChartTemplateYAML() string`

Collector output is produced via `metrix` + chart templates, not by returning raw metric maps.

## Validate Locally

From `src/go`:

- `go test ./plugin/go.d/collector/foo/...`
- `go test ./plugin/go.d/collector/...`
- `go test ./cmd/godplugin`

If you run the binary manually, CLI uses legacy flag naming: `-m/--modules`.
