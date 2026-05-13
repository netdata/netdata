# macOS Hardware Monitoring

## Purpose

This spec records the shipped contract for macOS battery, thermal, and fan
monitoring in the Netdata Agent.

## Scope

- `src/collectors/macos.plugin/`
- `macos.plugin` power-source module
- `macos.plugin` thermal and fan sampler
- macOS collector integration documentation

## Power Sources

- Power-source collection is part of `macos.plugin`.
- The module name is `power sources`, configured under `[plugin:macos]`.
- Per-metric options live under `[plugin:macos:power_sources]`.
- The data source is the public IOKit power-source API:
  `IOPSCopyPowerSourcesInfo()`, `IOPSCopyPowerSourcesList()`, and
  `IOPSGetPowerSourceDescription()`.
- The collector must not publish serial numbers, adapter serials, or other
  unique hardware identifiers as labels.
- Chart identity uses sanitized power-source names only.
- The collector emits charts only for keys exposed by macOS for a present
  internal battery or UPS.
- Missing or removed power sources are marked obsolete.

## Power Source Metrics

- `powersupply.capacity`: battery capacity in percentage, dimension
  `capacity`, with `device` and `source=iokit` labels.
- `powersupply.voltage`: power-source voltage in volts, dimension `voltage`,
  with `device` and `source=iokit` labels.
- `powersupply.current`: power-source current in amperes, dimension `current`,
  with `device` and `source=iokit` labels.
- `powersupply.cycles`: battery cycle count, dimension `cycles`, with `device`
  and `source=iokit` labels.
- `system.hw.sensor.temperature.input`: battery temperature in degrees
  Celsius when macOS exposes it, dimension `input`, with `device`,
  `source=iokit`, and `sensor=battery_temperature` labels.

## Thermal And Fan Collection

- Exact thermal and fan collection is part of `macos.plugin`.
- The module name is `powermetrics`, configured under `[plugin:macos]`.
- Per-metric and sampler options live under `[plugin:macos:powermetrics]`.
- The data source is the native Apple `/usr/bin/powermetrics` command.
- The default installed path runs the command through Netdata's setuid
  `ndsudo` helper with a hard-coded allow-list for the thermal/SMC plist
  sampler.
- Direct command execution remains configurable for debugging or custom local
  privilege models, but must use Netdata spawn wrappers with an argv vector,
  not a shell.
- The command must run in a background sampler thread. It must not block the
  normal one-second macOS collector loop.
- The default sampler interval is 60 seconds.
- The default sampler window is 1000 milliseconds.
- The default command timeout is 5000 milliseconds and must be at least one
  second longer than the sampler window.
- Output is parsed as a CoreFoundation property list. The implementation must
  not scrape human-readable `powermetrics` text output.
- `powermetrics` requires sufficient macOS privileges. When repeated sampling
  failures occur, the collector disables this module and logs one actionable
  error instead of retrying every collection cycle.

## Thermal And Fan Metrics

- `macos.thermal_pressure`: one-hot macOS thermal pressure state, dimensions
  `nominal`, `moderate`, `heavy`, `sleeping`, `trapping`, and `undefined`.
- `system.hw.sensor.fan.input`: fan speed in rotations per minute, dimension
  `input`, with `source=powermetrics` and `sensor=fan` labels.
- `system.hw.sensor.temperature.input`: CPU and GPU die temperatures in
  degrees Celsius, dimension `input`, with `source=powermetrics` and
  `sensor=cpu_die` or `sensor=gpu_die` labels.
- `macos.smc_thermal_level`: SMC thermal levels, dimensions `cpu`, `gpu`, and
  `io`.
- `macos.smc_prochot`: processor-hot assertion flags, dimensions `cpu` and
  `smc`.

## Configuration Contract

- `[plugin:macos] power sources = yes|no` controls power-source collection.
- `[plugin:macos] powermetrics = yes|no` controls thermal and fan sampling.
- `[plugin:macos:power_sources]` controls individual power-source charts:
  `battery capacity`, `power supply voltage`, `power supply current`,
  `battery temperature`, and `battery cycle count`.
- `[plugin:macos:powermetrics]` controls sampler timing, command path, and
  individual chart groups: `thermal pressure`, `SMC fan speed`,
  `SMC temperatures`, `SMC thermal levels`, and `SMC prochot`.

## Sensitive Data Rules

- Do not commit raw `powermetrics` output from real systems.
- Do not write battery serials, adapter serials, model serials, hostnames,
  usernames, or unique hardware identifiers into docs, SOWs, specs, labels, or
  test fixtures.
- Validation evidence should record command success, failure class, or schema
  behavior only.
