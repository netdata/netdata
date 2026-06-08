# macOS Storage Health Monitoring

## Purpose

This spec records the shipped contract for macOS S.M.A.R.T. and native NVMe
health monitoring in the Netdata Agent.

## Scope

- `src/go/plugin/go.d/collector/smartctl/`
- `src/collectors/macos.plugin/`
- `src/health/health.d/nvme.conf`
- macOS and storage collector integration documentation

## Generic S.M.A.R.T.

- Generic S.M.A.R.T. collection on macOS is provided by the existing
  `smartctl` go.d collector.
- The collector requires `smartmontools` version 7.0 or later because it
  consumes `smartctl --json` output.
- On macOS, `smartctl` execution follows the same non-Windows `ndsudo` path as
  Linux and BSD.
- The `smartctl` binary used by `ndsudo` must be discoverable from a
  root-controlled trusted path. Do not add user-writable package-manager
  prefixes such as a default Apple Silicon Homebrew prefix to the setuid helper
  search path.
- Apple internal NVMe/Apple Fabric storage may be visible to `smartctl` scans
  but still fail to open through smartmontools. In that case the generic
  `smartctl` collector cannot provide health metrics for the internal device.
- The collector keeps its existing bounded discovery and polling cadence:
  default chart update every 10 seconds, device discovery every 900 seconds,
  and device polling every 300 seconds.
- The collector's existing labels include `device_name`, `device_type`,
  `model_name`, and `serial_number`. Do not commit real label values, command
  output, or hardware dumps in durable artifacts.

## Native NVMe SMART

- Native macOS NVMe SMART collection is part of `macos.plugin`.
- The module name is `nvme smart`, configured under `[plugin:macos]`.
- Sampler options live under `[plugin:macos:nvme_smart]`.
- The data source is the public IOKit NVMe SMART user client from
  `IOKit/storage/nvme/NVMeSMARTLibExternal.h`.
- The collector must not execute `nvme-cli`, shell commands, or external NVMe
  tools for native macOS NVMe health.
- The collector discovers services that expose the documented
  `NVMe SMART Capable` IORegistry property.
- A service with the `NVMe SMART Capable` property is not sufficient by itself;
  the collector must be able to open the native SMART user client before adding
  the device.
- Discovery is rate-limited and defaults to every 300 seconds.
- SMART reads are rate-limited and default to every 10 seconds, matching the
  existing `nvme` collector cadence and the existing critical-warning alert.
- Device enumeration is capped to avoid unexpected chart cardinality.
- Chart identity uses sanitized ordinal names such as `nvme0`.
- Labels include `device`, `model_number`, and `source=iokit`.
- Labels must not include serial numbers, IORegistry paths, UUIDs, or other
  unique hardware identifiers.
- Apple internal Apple Fabric SSDs may expose generic disk I/O and APFS
  filesystem metrics while not exposing readable detailed NVMe health fields
  through the public SMART user client.

## Native NVMe Metrics

Native macOS NVMe SMART emits existing `nvme.*` contexts only when IOKit exposes
semantically equivalent fields:

- `nvme.device_estimated_endurance_perc`: dimension `used`.
- `nvme.device_available_spare_perc`: dimension `spare`.
- `nvme.device_composite_temperature`: dimension `temperature`, converted from
  Kelvin to Celsius.
- `nvme.device_io_transferred_count`: dimensions `read` and `written`, using
  NVMe data units converted to bytes with the same 1000 * 512 multiplier as the
  existing `nvme` collector.
- `nvme.device_power_cycles_count`: dimension `power`.
- `nvme.device_power_on_time`: dimension `power-on`, converted from hours to
  seconds.
- `nvme.device_unsafe_shutdowns_count`: dimension `unsafe`.
- `nvme.device_critical_warnings_state`: dimensions `available_spare`,
  `temp_threshold`, `nvm_subsystem_reliability`, `read_only`,
  `volatile_mem_backup_failed`, and `persistent_memory_read_only`.
- `nvme.device_media_errors_rate`: dimension `media`.
- `nvme.device_error_log_entries_rate`: dimension `error_log`.

The native macOS implementation must not emit these Linux `nvme-cli` contexts
unless Apple exposes named equivalent fields through the public API:

- `nvme.device_warning_composite_temperature_time`
- `nvme.device_critical_composite_temperature_time`
- `nvme.device_thermal_mgmt_temp1_transitions_rate`
- `nvme.device_thermal_mgmt_temp2_transitions_rate`
- `nvme.device_thermal_mgmt_temp1_time`
- `nvme.device_thermal_mgmt_temp2_time`

## Alerts

- The existing `nvme_device_critical_warnings_state` alert applies to native
  macOS NVMe charts because the native module reuses
  `nvme.device_critical_warnings_state` and preserves the `device` label.

## Sensitive Data Rules

- Do not commit real `smartctl`, `ioreg`, or NVMe SMART output.
- Do not write serial numbers, IORegistry paths, device UUIDs, hostnames,
  usernames, or raw hardware dumps into docs, SOWs, specs, labels, comments, or
  test fixtures.
- Validation evidence should record command/API success, failure class, service
  count, or schema behavior only.
