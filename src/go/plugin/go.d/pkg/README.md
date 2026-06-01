# go.d Helper Packages

This directory contains reusable helpers for go.d collectors. For new go.d
collectors, start with `src/go/plugin/go.d/docs/how-to-write-a-collector.md`;
use this file only to find existing helper packages.

## V2 Metrics And Charts

New go.d collectors MUST use framework V2:

- `src/go/pkg/metrix` for metric instruments and `CollectorStore`.
- `src/go/plugin/framework/charttpl` for `charts.yaml` templates.
- `src/go/plugin/framework/chartengine` for chart-template runtime behavior.
- `src/go/plugin/framework/collectorapi` for `CollectorV2` registration and
  lifecycle contracts.

If an existing V1 collector is being migrated, use
`src/go/plugin/go.d/docs/migrate-v1-to-v2.md`.

## Common Collector Helpers

- `ndexec` runs external commands.
- `iprange` parses and checks IP ranges.
- `logs` helps parse application log files.
- `src/go/pkg/matcher` provides selector/matcher implementations.
- `web` provides HTTP client configuration helpers.
- `prometheus` parses Prometheus endpoints; use it with `web`.
- `tlscfg` provides TLS support.

## Legacy V1 Helpers

- `stm` converts structs into `map[string]int64`. This is V1-shaped and MUST
  NOT be used as the metric path for new V2 collectors. It MAY be useful while
  maintaining legacy V1 collectors or building temporary migration parity tests
  that are removed from the final runtime path.
