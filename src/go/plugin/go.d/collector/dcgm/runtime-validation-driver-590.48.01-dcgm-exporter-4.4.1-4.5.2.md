# DCGM Field Runtime Validation (Driver 590.48.01 / dcgm-exporter 4.4.1-4.5.2)

This file documents runtime validation of the Netdata DCGM field dataset against a live exporter.

## Environment

- Driver: `590.48.01`
- dcgm-exporter: `4.4.1-4.5.2`
- Validation scope: `driver version + dcgm-exporter version`
- Test host GPU (informational): `NVIDIA GeForce RTX 5090`
- Date: `2026-02-08`
- Method: start `dcgm-exporter` directly on port `19400` with generated 127-field profiles.
- Source dataset: `src/go/plugin/go.d/collector/dcgm/dcgm-exporter-netdata.csv` (`623` documented fields)

## Results

- Profiles executed: `6` (each configured with `127` fields)
- Startup-fail fields: `20`
- Non-numeric output fields: `18`
- Numeric-seen fields: `224`
- Unseen fields: `361`

## Startup-Fail Fields

- `DCGM_FI_BIND_UNBIND_EVENT`
- `DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT`
- `DCGM_FI_DEV_CPU_TEMP_CRITICAL`
- `DCGM_FI_DEV_DIAG_NCCL_TESTS_RESULT`
- `DCGM_FI_DEV_FABRIC_HEALTH_MASK`
- `DCGM_FI_DEV_FIRST_CONNECTX_FIELD_ID`
- `DCGM_FI_DEV_GET_GPU_RECOVERY_ACTION`
- `DCGM_FI_DEV_LAST_CONNECTX_FIELD_ID`
- `DCGM_FI_DEV_MEMORY_UNREPAIRABLE_FLAG`
- `DCGM_FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL`
- `DCGM_FI_DEV_NVLINK_GET_STATE`
- `DCGM_FI_DEV_NVLINK_PPCNT_IBPC_PORT_XMIT_WAIT`
- `DCGM_FI_DEV_PCIE_COUNT_CORRECTABLE_ERRORS`
- `DCGM_FI_FIRST_NVSWITCH_FIELD_ID`
- `DCGM_FI_IMEX_DAEMON_STATUS`
- `DCGM_FI_IMEX_DOMAIN_STATUS`
- `DCGM_FI_INTERNAL_FIELDS_0_END`
- `DCGM_FI_INTERNAL_FIELDS_0_START`
- `DCGM_FI_LAST_NVSWITCH_FIELD_ID`
- `DCGM_FI_LAST_VGPU_FIELD_ID`

## Non-Numeric Output Fields

- `DCGM_FI_DEV_CREATABLE_VGPU_TYPE_IDS`
- `DCGM_FI_DEV_ENC_STATS`
- `DCGM_FI_DEV_ENFORCED_POWER_PROFILE_MASK`
- `DCGM_FI_DEV_FBC_SESSIONS_INFO`
- `DCGM_FI_DEV_FBC_STATS`
- `DCGM_FI_DEV_MIG_ATTRIBUTES`
- `DCGM_FI_DEV_MIG_CI_INFO`
- `DCGM_FI_DEV_MIG_GI_INFO`
- `DCGM_FI_DEV_PLATFORM_INFINIBAND_GUID`
- `DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK`
- `DCGM_FI_DEV_SUPPORTED_CLOCKS`
- `DCGM_FI_DEV_SUPPORTED_VGPU_TYPE_IDS`
- `DCGM_FI_DEV_VALID_POWER_PROFILE_MASK`
- `DCGM_FI_DEV_VGPU_INSTANCE_IDS`
- `DCGM_FI_DEV_VGPU_PER_PROCESS_UTILIZATION`
- `DCGM_FI_DEV_VGPU_TYPE_CLASS`
- `DCGM_FI_DEV_VGPU_TYPE_LICENSE`
- `DCGM_FI_GPU_TOPOLOGY_AFFINITY`

## Full Data

- Full machine-readable report: `src/go/plugin/go.d/collector/dcgm/runtime-validation-driver-590.48.01-dcgm-exporter-4.4.1-4.5.2.json`
