# go.d Helper Packages

This directory contains go.d-specific reusable helpers. go.d collectors also use
shared Go helpers from `src/go/pkg/*` and logging helpers from `src/go/logger`.

For new go.d collectors, start with
`src/go/plugin/go.d/docs/how-to-write-a-collector.md`. For when and why to use
helper packages, read `src/go/plugin/go.d/docs/helper-packages.md`.

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

- `src/go/plugin/go.d/pkg/ndexec` runs external commands.
- `src/go/plugin/go.d/pkg/iprange` parses and checks IP ranges.
- `src/go/plugin/go.d/pkg/logs` helps parse application log files.
- `src/go/pkg/matcher` provides selector/matcher implementations.
- `src/go/pkg/confopt` provides duration and tri-state config option types.
- `src/go/pkg/web` provides HTTP client configuration helpers.
- `src/go/pkg/prometheus` parses Prometheus endpoints; use it with
  `src/go/pkg/web`.
- `src/go/pkg/tlscfg` provides TLS support.
- `src/go/plugin/go.d/pkg/sqlquery` provides reusable SQL row/query helpers.
- `src/go/plugin/go.d/pkg/socket` provides TCP/UDP/Unix line-protocol clients.
- `src/go/plugin/go.d/pkg/cloudauth` provides shared cloud authentication
  config/credential helpers.
- `src/go/plugin/go.d/pkg/pinger` provides shared ping probing and
  latency/jitter calculations.

## Legacy V1 Helpers

- `src/go/pkg/stm` converts structs into `map[string]int64`. This is V1-shaped
  and MUST NOT be used as the metric path for new V2 collectors. It MAY be
  useful while maintaining legacy V1 collectors or building temporary migration
  parity tests that are removed from the final runtime path.
- `src/go/plugin/go.d/pkg/oldmetrix` provides V1 metric vector helper types
  used by existing V1 collectors. New V2 collectors SHOULD use
  `src/go/pkg/metrix` instead.
