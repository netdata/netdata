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

Suggested baseline profile:

- Clocks: `DCGM_FI_DEV_SM_CLOCK`, `DCGM_FI_DEV_MEM_CLOCK`
- Temperatures: `DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_MEMORY_TEMP`
- Power: `DCGM_FI_DEV_POWER_USAGE`, `DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION`
- Utilization: `DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_MEM_COPY_UTIL`
- Reliability: `DCGM_FI_DEV_XID_ERRORS`, `DCGM_FI_DEV_ROW_REMAP_FAILURE`, `DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS`, `DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS`
- Throttle violations: `DCGM_FI_DEV_POWER_VIOLATION`, `DCGM_FI_DEV_THERMAL_VIOLATION`
- Interconnect: `DCGM_FI_DEV_PCIE_REPLAY_COUNTER`, `DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL`

Datadog-aligned expansion (for deeper compute efficiency analysis):

- `DCGM_FI_PROF_SM_ACTIVE`
- `DCGM_FI_PROF_SM_OCCUPANCY`
- `DCGM_FI_PROF_PIPE_TENSOR_ACTIVE`
- `DCGM_FI_PROF_DRAM_ACTIVE`
- `DCGM_FI_PROF_PIPE_FP16_ACTIVE`
- `DCGM_FI_PROF_PIPE_FP32_ACTIVE`
- `DCGM_FI_PROF_PIPE_FP64_ACTIVE`
- `DCGM_FI_PROF_PCIE_TX_BYTES`
- `DCGM_FI_PROF_PCIE_RX_BYTES`

## Alerts

Default alerts included for universally actionable conditions:

- XID errors
- Row remap failure
- New uncorrectable remapped rows
- Power violation duration
- Thermal violation duration

See: `src/health/health.d/dcgm.conf`.
