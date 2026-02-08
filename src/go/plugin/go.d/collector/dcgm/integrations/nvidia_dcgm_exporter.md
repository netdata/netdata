# NVIDIA DCGM Exporter

Plugin: `go.d.plugin`  
Module: `dcgm`

## Overview

The `dcgm` collector scrapes NVIDIA `dcgm-exporter` Prometheus metrics (default `http://127.0.0.1:9400/metrics`) and maps them to static Netdata-native contexts.

- All numeric fields exported by `dcgm-exporter` are supported.
- Contexts are created lazily: only contexts with collected metrics are instantiated.
- v1 uses manual job configuration (no autodiscovery).

## Prerequisites

- NVIDIA driver + DCGM installed.
- `dcgm-exporter` running and reachable.
- Exporter field CSV configured with the fields you want to collect.
- Profiling fields may require additional capabilities/privileges in your runtime.

## Interval Coupling

Keep Netdata `update_every` aligned with `dcgm-exporter` collection interval.

- Exporter default collection interval: `30s`.
- Collector default `update_every`: `30`.
- If you change one side, change the other side too.

## Configuration

Example `go.d/dcgm.conf`:

```yaml
jobs:
  - name: local
    url: http://127.0.0.1:9400/metrics
    update_every: 30
```

## Field Profiles

`dcgm-exporter` ships with a small default field set. For production, use an explicit field CSV profile.
Netdata provides a recommended exporter profile file:
[`dcgm-exporter-netdata.csv`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dcgm/dcgm-exporter-netdata.csv)
(raw: `https://raw.githubusercontent.com/netdata/netdata/master/src/go/plugin/go.d/collector/dcgm/dcgm-exporter-netdata.csv`).

Example:
`dcgm-exporter -f /path/to/dcgm-exporter-netdata.csv`

The Netdata profile enables 127 high-value fields by default and keeps all other known DCGM fields in the same file as commented entries for easy customization.

Runtime validation artifact:
- `src/go/plugin/go.d/collector/dcgm/runtime-validation-driver-590.48.01-dcgm-exporter-4.4.1-4.5.2.md`
- `src/go/plugin/go.d/collector/dcgm/runtime-validation-driver-590.48.01-dcgm-exporter-4.4.1-4.5.2.json`

Validation results are primarily version-scoped (NVIDIA driver + DCGM/dcgm-exporter versions). Use the artifact as a concrete baseline, not as a universal compatibility guarantee.

Each field line includes a comment with:
- Netdata context
- Netdata family
- Netdata dimension

When customizing:
- Uncomment the field you need.
- Comment one currently enabled field.

## Alerts

Default alerts included for universally actionable conditions:

- XID errors
- Row remap failure
- New uncorrectable remapped rows
- Power violation duration
- Thermal violation duration

See: `src/health/health.d/dcgm.conf`.
