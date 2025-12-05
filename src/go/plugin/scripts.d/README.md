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
one top-level `jobs:` array, where each entry is a complete Nagios job
definition (plugin path, arguments, scheduler name, vnode, retries, etc.). This
matches what dyncfg exposes—when you edit a job in the UI you are editing the
same YAML object you would put on disk.

Example:

```yaml
jobs:
  - name: ping_localhost
    scheduler: default
    plugin: "/usr/lib/nagios/plugins/check_ping"
    args: ["-H", "127.0.0.1", "-w", "100.0,20%", "-c", "200.0,40%"]
    timeout: 60s
    retry_interval: 1m
    max_check_attempts: 3
```

There is no nested structure anymore—module files only list explicit
jobs, and every job is autonomous. Plugin-level toggles (for example enabling or
disabling modules) live in `/etc/netdata/scripts.d.conf`, but all scheduling,
macros, and chart definitions stay inside the module job files just like go.d.

### Argument macros

Jobs can bind up to 32 `$ARGn$` macros via the optional `arg_values` array.
Entries in `args` that reference `$ARG1$` … `$ARG32$` are replaced with the
corresponding values before execution, and the same values are exposed through
`NAGIOS_ARGn` environment variables for the plugin to consume.

### Time periods

`check_period` controls when a job is allowed to run. Define time periods inside
the module configuration (for example `/etc/netdata/scripts.d/nagios.conf`) and
reference them from individual jobs:

```yaml
# /etc/netdata/scripts.d/nagios.conf
time_periods:
  - name: 24x7
    alias: Always on
    rules:
      - type: weekly
        days: [sunday, monday, tuesday, wednesday, thursday, friday, saturday]
        ranges: ["00:00-24:00"]

jobs:
  - name: local_plugins
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

### Scheduler telemetry

The `scheduler` module publishes native charts so you can monitor queue depth,
job throughput, and the “time until next execution” for each worker pool. Look
for contexts such as `nagios.scheduler.jobs`, `nagios.scheduler.rate`, and
`nagios.scheduler.next` to confirm schedulers are keeping up with load. Changes
to scheduler definitions (worker counts or queue sizes) are applied live—when
you edit the scheduler job, the underlying runtime is recreated and existing
jobs are reattached automatically.

### Charts, labels, and units

Each job now derives a deterministic chart identity from its scheduler name and
the fully expanded plugin command line. That signature feeds every chart ID and
context, so different executions of the same script (for different URLs, hosts,
etc.) keep separate time-series across restarts.

- **Contexts & families** – job charts live under `nagios.<script>.<measurement>`
  (for example `nagios.http_check.latency`). This keeps “apples with apples” in
  the Netdata menu even when multiple jobs share a script.
- **Labels** – all charts share the same label set: `nagios_job`,
  `nagios_scheduler`, and `nagios_cmdline` (the fully expanded plugin
  invocation). Perfdata charts add `perf_label`. The uniform labels make filtering
  dashboards and health alerts straightforward.
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

The scheduler also exposes a `nagios.scheduler.next` chart showing
“time until the next job fires” in seconds (nanosecond precision). Expect it to
track the configured intervals; spikes indicate the executor is falling behind
(worker starvation, very long-running checks, etc.).

### Cadence (`update_every`)

Jobs follow the same cadence controls as go.d collectors: set `update_every`
(seconds) inside each job definition to run faster or slower than the default
60 s interval. When omitted, scripts.d picks a conservative default (currently
60 s). This field applies to both Nagios and Zabbix jobs so you can mirror your
existing polling schedules.

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

### LLD lifecycle

Zabbix-style jobs keep their LLD catalogs and missing-instance counters in
memory. Restarting the agent or reloading the plugin resets that state, mirroring
how go.d collectors reset transient discovery data. If you need longer retention,
keep the plugin running continuously; no on-disk cache is maintained.

### Zabbix preprocessing

Dependent pipelines share the same `.steps` schema as the standalone
`zabbix-preproc` library. Step `type` accepts human-readable tokens such as
`jsonpath`, `csv_to_json`, or `snmp_walk_value`. The optional `error_handler`
field understands the native Zabbix actions: `default`, `discard`, `set-value`,
and `set-error`. When you choose `set-value` or `set-error` provide the fallback
text via `error_handler_params`.

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
