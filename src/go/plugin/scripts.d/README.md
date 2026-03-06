# scripts.d.plugin (preview)

`scripts.d.plugin` runs Nagios-style check scripts inside Netdata without changing
plugin output format. The active collector is `nagios` (single collector surface),
implemented as a normal V2 collector with collector-local scheduling/state.

> **Status:** preview. Core execution, retry/state tracking, and perfdata routing are
> implemented; config/docs may still evolve.

## Configuration

- Plugin-level toggles: `/etc/netdata/scripts.d.conf`
- Collector jobs: `/etc/netdata/scripts.d/nagios.conf`

Each job is a Nagios check definition.

Example:

```yaml
jobs:
  - name: ping_localhost
    plugin: "/usr/lib/nagios/plugins/check_ping"
    args: ["-H", "127.0.0.1", "-w", "100.0,20%", "-c", "200.0,40%"]
    timeout: 5s
    check_interval: 1m
    retry_interval: 30s
    max_check_attempts: 3
```

### Time Periods

`check_period` is supported. Custom periods are defined with `time_periods` inside
the same job definition.

```yaml
jobs:
  - name: local_plugins
    plugin: "/usr/lib/nagios/plugins/check_dummy"
    args: ["0", "ok"]
    check_period: 24x7
    time_periods:
      - name: 24x7
        alias: Always on
        rules:
          - type: weekly
            days: [sunday, monday, tuesday, wednesday, thursday, friday, saturday]
            ranges: ["00:00-24:00"]
```

## Writing Compatible Checks

A compatible check returns a Nagios state with its exit code and prints a status
line that Netdata can parse.

- Exit codes:
  - `0` = OK
  - `1` = WARNING
  - `2` = CRITICAL
  - `3` = UNKNOWN
- First-line output format:
  - `<summary text> | <perfdata>`
- The `|` separator is optional:
  - text before `|` is the human-readable summary
  - text after `|` is performance data used for auto-generated charts
- Each performance-data item follows:
  - `'label'=value[UOM];warn;crit;min;max`
- Separate multiple metrics with spaces.
- Common units include:
  - `%`, `s`, `ms`, `B`, `KB`, `MB`, `GB`, `c`
- If the script prints multiple lines:
  - the first line is the summary
  - the remaining lines are kept as long output

Minimal example:

```bash
#!/bin/sh
echo "CPU OK - 20% used | cpu=20%;80;90"
exit 0
```

## Execution Model

- Each `jobs:` entry becomes one V2 Nagios collector instance.
- Script execution happens during `Collect()` only when the job is due.
- `update_every` is the scheduling resolution.
- If `update_every` is slower than `check_interval` or `retry_interval`, Netdata
  logs a warning and the effective cadence is limited by `update_every`.
- Nagios semantics are preserved collector-side:
  - `check_interval`
  - `retry_interval`
  - `max_check_attempts`
  - `inter_check_jitter`
  - `check_period`
- If a check exceeds `timeout`, Netdata reports the job state as `timeout`.
- If a check is due but the current time is outside `check_period`, Netdata does not execute it and reports the public job state as `paused`.
- Non-due successful cycles replay the last cached perfdata values and threshold
  states so chartengine series stay alive between executions.
- When a due run is blocked by `check_period`, perfdata value charts remain at their last observed values, but threshold-state charts are zeroed until the next successful execution.
- Counter perfdata keeps counter semantics for the value series. Replayed raw
  totals naturally flatten to zero deltas between executions.

Defaults:

- `check_interval`: `5m`
- `retry_interval`: `1m`
- `timeout`: `5s`
- `max_check_attempts`: `3`

## Metrics and Charts

Static template charts:

- `nagios.job.state`
- `nagios.job.execution_duration`
- `nagios.job.execution_cpu_total`
- `nagios.job.execution_max_rss`

Perfdata is routed plugin-side and materialized via autogen (bounded lifecycle):

- Unit classes: `time`, `bytes`, `bits`, `percent`, `counter`, `generic`
- Metric identity: sanitized perfdata key (from Nagios perfdata label)
- Unit-class changes create a new metric identity
- Collision policy: deterministic keep-first, drop conflicting label
- Each perfdata metric creates one value chart.
- Non-counter perfdata also creates one derived threshold-state chart with:
  - `no_threshold`
  - `ok`
  - `warning`
  - `critical`
- Counter perfdata currently does not emit a threshold-state chart.
- Raw `min`, `max`, and raw threshold bounds are not charted.

## Logging

Checks log through the collector/job logger path. There is no separate public runtime
component or scheduler telemetry surface.

## Tests

```bash
cd src/go
go test ./plugin/scripts.d/collector/nagios/... -count=1
```

## Build

```bash
cmake -DENABLE_PLUGIN_SCRIPTS=On ..
cmake --build . --target scripts-plugin
```

Binary path:

- `usr/libexec/netdata/plugins.d/scripts.d.plugin`

Stock config path:

- `usr/lib/netdata/conf.d/scripts.d/`
