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

Suggested baseline profile (comprehensive coverage with actionable throttling fields):

- Utilization and compute activity:
  - `DCGM_FI_DEV_GPU_UTIL`
  - `DCGM_FI_DEV_MEM_COPY_UTIL`
  - `DCGM_FI_DEV_ENC_UTIL`
  - `DCGM_FI_DEV_DEC_UTIL`
  - `DCGM_FI_PROF_GR_ENGINE_ACTIVE`
  - `DCGM_FI_PROF_SM_ACTIVE`
  - `DCGM_FI_PROF_SM_OCCUPANCY`
  - `DCGM_FI_PROF_PIPE_TENSOR_ACTIVE`
  - `DCGM_FI_PROF_DRAM_ACTIVE`
  - `DCGM_FI_PROF_PIPE_FP16_ACTIVE`
  - `DCGM_FI_PROF_PIPE_FP32_ACTIVE`
  - `DCGM_FI_PROF_PIPE_FP64_ACTIVE`
- Frame buffer and memory:
  - `DCGM_FI_DEV_FB_TOTAL`
  - `DCGM_FI_DEV_FB_USED`
  - `DCGM_FI_DEV_FB_FREE`
  - `DCGM_FI_DEV_FB_RESERVED`
  - `DCGM_FI_DEV_FB_USED_PERCENT`
- Thermals, clocks, and power:
  - `DCGM_FI_DEV_GPU_TEMP`
  - `DCGM_FI_DEV_MEMORY_TEMP`
  - `DCGM_FI_DEV_SLOWDOWN_TEMP`
  - `DCGM_FI_DEV_SM_CLOCK`
  - `DCGM_FI_DEV_MEM_CLOCK`
  - `DCGM_FI_DEV_FAN_SPEED`
  - `DCGM_FI_DEV_POWER_USAGE`
  - `DCGM_FI_DEV_POWER_MGMT_LIMIT`
  - `DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION`
  - `DCGM_FI_DEV_PSTATE`
  - `DCGM_FI_DEV_CLOCK_THROTTLE_REASONS`
- Reliability and violations:
  - `DCGM_FI_DEV_XID_ERRORS`
  - `DCGM_FI_DEV_ROW_REMAP_FAILURE`
  - `DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS`
  - `DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS`
  - `DCGM_FI_DEV_POWER_VIOLATION`
  - `DCGM_FI_DEV_THERMAL_VIOLATION`
- Interconnect:
  - `DCGM_FI_DEV_PCIE_REPLAY_COUNTER`
  - `DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL`
  - `DCGM_FI_PROF_PCIE_TX_BYTES`
  - `DCGM_FI_PROF_PCIE_RX_BYTES`
- Device and virtualization:
  - `DCGM_FI_DEV_COUNT`
  - `DCGM_FI_DEV_VGPU_LICENSE_STATUS`
  - `DCGM_FI_DEV_UUID`
  - `DCGM_FI_DRIVER_VERSION`
  - `DCGM_FI_DEV_NAME`
  - `DCGM_FI_DEV_SERIAL`
  - `DCGM_FI_DEV_BRAND`
  - `DCGM_FI_DEV_MINOR_NUMBER`

## Alerts

Default alerts included for universally actionable conditions:

- XID errors
- Row remap failure
- New uncorrectable remapped rows
- Power violation duration
- Thermal violation duration

See: `src/health/health.d/dcgm.conf`.
