# Windows Storage And Fan Monitoring

## Scope

This spec records the current Netdata Agent contracts for Windows SMART disk
health, NVMe health, NVMe thermal-throttling signals, and fan telemetry.

## SMART Disk Health

- Windows SMART disk health is collected by the go.d `smartctl` collector, not
  by duplicating SMART parsing in `windows.plugin`.
- On Windows, the collector executes an installed `smartctl.exe` directly from
  `PATH` or the default smartmontools installation directory.
- Direct scan execution must use either `smartctl --json --scan` or
  `smartctl --json --scan-open`; it must not pass both `--scan` and
  `--scan-open` in one command.
- Linux and BSD continue to use the existing `ndsudo` execution path.

## NVMe Health And Thermal Signals

- Windows NVMe health is collected by the go.d `nvme` collector so it reuses
  the existing NVMe chart contexts, alert context, labels, and metric names.
- The native Windows backend discovers local physical drives as
  `\\.\PhysicalDriveN`, filters for `BusTypeNvme`, and queries the NVMe SMART /
  health information log through `IOCTL_STORAGE_QUERY_PROPERTY` with
  `StorageDeviceProtocolSpecificProperty`.
- The native backend maps the NVMe health log into the existing collector model,
  including critical warnings, composite temperature, warning/critical
  temperature time, thermal-management transition counts, and
  thermal-management total time.
- Windows may optionally fall back to an installed `nvme.exe` when native
  discovery does not produce devices and the CLI is available.
- Native Windows storage access must be read-first and fail closed; the backend
  must not perform destructive or state-changing storage operations.

## Fan Telemetry

- Windows fan telemetry is best-effort and collected by `windows.plugin`
  `GetFans` through the WMI `Win32_Fan` class.
- `Win32_Fan.DesiredSpeed` is requested speed, not a guaranteed actual
  tachometer RPM reading. Code, docs, metadata, and PR notes must not present it
  as universal actual fan speed.
- Fan charts may be absent on systems whose firmware or drivers do not expose
  `Win32_Fan` instances.
- Future true actual fan RPM support requires a separate reliable source, such
  as a vendor interface, EC/Super I/O integration, IPMI, or another explicitly
  supported provider.
