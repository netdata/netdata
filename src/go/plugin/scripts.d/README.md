# scripts.d.plugin (preview)

`scripts.d.plugin` runs stock Nagios checks inside Netdata without modifying the
original plugins. Jobs are executed through a dedicated scheduler/executor,
perfdata metrics become native Netdata charts, and every run emits structured
logs (stdout/stderr/state transitions) over OTLP.

> **Status:** preview. The core execution pipeline, charts, and logging are in
> place, but configuration options and documentation may still change before GA.

## Configuration overview

Each YAML file under `/etc/netdata/scripts.d/*.conf` (or the stock directory
shipped in `usr/lib/netdata/conf.d/scripts.d/`) follows the standard go.d layout:

a top-level `jobs:` list where each entry is a configuration **shard**. A shard
can define:

- `defaults`: reusable timeout/scheduling settings
- `user_macros`: values for `$USER1` .. `$USER32`
- `jobs`: explicit Nagios check definitions
- `directories`: rules that dynamically discover every executable inside a
  directory (with include/exclude globs) and turn them into jobs

Example:

```yaml
jobs:
  - name: local_plugins
    defaults:
      timeout: 60s
      retry_interval: 1m
    user_macros:
      USER1: "/usr/lib/nagios/plugins"
    jobs:
      - name: ping_localhost
        plugin: "$USER1$/check_ping"
        args: ["-H", "$HOSTADDRESS$", "-p", "$ARG1$", "-w", "$ARG2$", "-c", "$ARG3$"]
        arg_values: ["80", "5", "10"]
    directories:
      - name_prefix: stock_
        path: /usr/lib/nagios/plugins
        include: ["check_*"]
        exclude: ["check_nrpe"]
```

Each shard is loaded by go.d's file discovery, normalized into `JobSpec`
structures (including directory expansions), and will eventually register with
the async scheduler.

#### Autodetection retry

If a shard fails the initial `Init`/`Check` cycle (missing plugin, bad
permissions, etc.), go.d re-runs autodetection after a delay. Set
`autodetection_retry` (seconds) at the shard level to control that cadence; the
default is 60 s, and `0` disables retries entirely.

### Argument macros

Jobs can bind up to 32 `$ARGn$` macros via the optional `arg_values` array.
Entries in `args` that reference `$ARG1$` … `$ARG32$` are replaced with the
corresponding values before execution, and the same values are exposed through
`NAGIOS_ARGn` environment variables for the plugin to consume.

### Time periods

`check_period` controls when a job is allowed to run. The stock configuration
ships with a `24x7` definition (all days `00:00-24:00`) and each job defaults to
that period, so you can ignore scheduling until you need finer control. Custom
periods follow a YAML structure matching Nagios semantics (per-day ranges,
exclusions, explicit dates) and will be documented alongside the schema once the
feature graduates from TODO status.

```yaml
time_periods:
  - name: 24x7
    alias: Always on
    rules:
      - type: weekly
        days: [sunday, monday, tuesday, wednesday, thursday, friday, saturday]
        ranges: ["00:00-24:00"]

jobs:
  - name: local_plugins
    defaults:
      check_period: 24x7

logging:
  enabled: true
  otlp:
    endpoint: 127.0.0.1:4317
    tls: false
    timeout: 5s
```

The `logging` block enables OTLP log forwarding to Netdata's `otel.plugin`
instance. The sample config keeps `tls: false` so local collectors can start
without certificates; set `tls: true` (and optionally `tls_ca` / `tls_cert` /
`tls_key`) to enforce encryption, even when emitting to `127.0.0.1:4317`.
Adjust the endpoint, timeout, or headers if your OTLP
pipeline lives elsewhere or requires authentication.

### Scheduling semantics & skip behavior

Every job owns a dedicated timer. When the timer fires the scheduler enqueues
the job unless it is already queued or executing—matching Nagios Core's
"single-flight" behavior. If a run is skipped this way, the skip counter for
that job increases and the next run occurs at the normal cadence (there is no
catch-up burst). Use the `nagios.runtime` chart's `skipped` dimension or the
stock `health.d/nagios_skipped.conf` rule to monitor how frequently this happens
and to page when a job repeatedly overlaps.

### Charts, labels, and units

Each job now derives a deterministic chart identity from the shard plus the full
plugin command line (path + arguments). That signature feeds every chart ID and
context, so different executions of the same script (for different URLs, hosts,
etc.) keep separate time-series across restarts.

- **Contexts & families** – job charts live under `nagios.<script>.<measurement>`
  (for example `nagios.http_check.latency`). This keeps “apples with apples” in
  the Netdata menu even when multiple jobs share a script.
- **Labels** – all charts share the same label set: `nagios_job`, `nagios_shard`,
  and `nagios_cmdline` (the fully expanded plugin invocation). Perfdata charts
  add `perf_label`. The uniform labels make filtering dashboards and health
  alerts straightforward.
- **Titles** – chart titles describe the script + measurement (e.g. “Nagios
  http_check response time”) rather than the monitored endpoint, so all charts
  with a shared context render cleanly in the UI.

#### Unit normalization & scaling

Netdata stores integers, so scripts.d.plugin normalizes perfdata to base units
before emitting metrics:

| Input unit                    | Canonical unit | Scaling behaviour                                |
|------------------------------|----------------|---------------------------------------------------|
| Bytes, KB, MB, GB, TB        | bytes          | Converted to raw bytes, divider = 1               |
| Bytes per second (KB/s …)    | bytes/s        | Converted to bytes/s, divider = 1                 |
| Seconds, ms, µs, ns          | seconds        | Stored as nanoseconds, divider = 1 000 000 000    |
| Percent (`%`)                | %              | Value ×1000, divider = 1000                       |
| Counters (`c`)               | c              | Stored as-is                                      |
| Any other unit or unitless   | original text  | Value ×1000, divider = 1000                       |

When a plugin flips between `KB` and `MB` (or `ms` and `s`) the collector still
publishes a single chart in base units, so there are no spurious RRD resets. If
a metric genuinely changes semantics (for example, from bytes to seconds) or a
job’s cadence changes, scripts.d.plugin re-sends the CHART definition so Netdata
can flush/recreate the series with the new metadata.

The scheduler also exposes a shard-level `nagios.scheduler.next` chart showing
“time until the next job fires” in seconds (nanosecond precision). Expect it to
track the configured intervals; spikes indicate the executor is falling behind
(worker starvation, very long-running checks, etc.).

### Logging over OTLP (TLS support)

Structured logs are forwarded to OTEL via `logging.otlp`. Besides `endpoint`,
`timeout`, `tls`, and `headers`, you can now set:

- `tls_ca`: custom CA bundle for the collector
- `tls_cert` / `tls_key`: client certificate pair for mutual TLS
- `tls_server_name`: override the TLS SNI when the collector name differs from the
  endpoint host
- `tls_skip_verify`: disable server certificate verification (not
  recommended outside of lab environments)

When `tls` is `true`, the plugin always establishes a TLS 1.2 connection using
these settings. Set `tls: false` only for loopback collectors or other trusted
plaintext networks.

### Directory watcher tuning

Two shard-level knobs control how aggressively scripts.d.plugin reloads
configuration when files change:

- `watcher_debounce` (default `250ms`): how long inotify/fsnotify events are
  batched before a reload is triggered. Increase this value if large config
  syncs generate a flood of events; decrease it if you need near-real-time
  reloads.
- `directory_rescan_interval` (default `1m`): how often the collector polls the
  directories when fsnotify cannot be established (e.g., missing permissions or
  `ENOSPC`). Shortening this interval speeds up discovery at the cost of extra
  IO.

Both settings accept the usual Go duration syntax (`500ms`, `2s`, `1m30s`).

### Mock integration tests

A small suite of mock Nagios plugins lives under `tests/plugins/`. They exercise
state handling, perfdata parsing, macro substitution, long output logging, and
skip semantics without requiring external services. Run them with:

```bash
cd src/go
go test ./plugin/scripts.d/tests
```

The tests stub `nd-run`, execute the mock scripts through the scheduler, and
assert the emitted metrics/logs. Extend this suite whenever you add new
collector features so we keep a fast end-to-end signal in CI.

## Building

Enable the plugin during CMake configuration:

```bash
cmake -DENABLE_PLUGIN_SCRIPTS=On ..
cmake --build . --target scripts-plugin
```

The resulting binary lives at `usr/libexec/netdata/plugins.d/scripts.d.plugin`.
Stock configuration ships in `usr/lib/netdata/conf.d/scripts.d/`.
