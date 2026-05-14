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
- On Windows, the collector executes an installed `nvme.exe` directly from
  `PATH` or the default `nvme-cli` installation directories. There is no native
  Windows storage-protocol backend in this contract.
- The Windows `nvme.exe` path maps the same `nvme list` and `nvme smart-log`
  JSON surfaces into the existing collector model, including critical warnings,
  composite temperature, warning/critical temperature time,
  thermal-management transition counts, and thermal-management total time.
- Linux and BSD continue to use the existing `ndsudo` execution path for the
  same `nvme` CLI.

## Fan Telemetry

- Windows fan telemetry is best-effort and collected by `windows.plugin`
  `GetFans` through the WMI `Win32_Fan` class.
- `Win32_Fan.DesiredSpeed` is requested speed, not a guaranteed actual
  tachometer RPM reading. Code, docs, metadata, and PR notes must not present it
  as universal actual fan speed.
- When `DesiredSpeed` is present, `GetFans` reports it with the existing
  hardware sensor fan speed context `system.hw.sensor.fan.input` and dimension
  `input` so dashboard placement and aggregation match other fan providers.
- When WMI `Availability` or `Status` is present, `GetFans` reports online/fault
  state with the existing hardware sensor fan alarm context
  `system.hw.sensor.fan.alarm` and dimensions `clear` and `fault`.
- Fan charts may be absent on systems whose firmware or drivers do not expose
  `Win32_Fan` instances.
- Future true actual fan RPM support requires a separate reliable source, such
  as a vendor interface, EC/Super I/O integration, IPMI, or another explicitly
  supported provider.
