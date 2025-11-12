# nagios.d.plugin (preview)

`nagios.d.plugin` executes stock Nagios checks inside Netdata without modifying the
original plugins. The current codebase focuses on scaffolding: building the
binary, parsing configuration, and normalizing Nagios job definitions so that
future scheduler/executor work can plug in.

> **Status:** experimental scaffolding. Jobs are parsed and validated but not yet
> executed.

## Configuration overview

Each YAML file under `/etc/netdata/nagios.d/*.conf` (or the stock directory
shipped in `usr/lib/netdata/conf.d/nagios.d/`) follows the standard go.d layout:

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
    insecure: true
    timeout: 5s
```

The `logging` block enables OTLP log forwarding to Netdata's `otel.plugin`
instance. By default the plugin emits to `127.0.0.1:4317` over insecure gRPC;
adjust the endpoint, TLS mode, timeout, or headers if your OTLP pipeline lives
elsewhere or requires authentication.

## Building

Enable the plugin during CMake configuration:

```bash
cmake -DENABLE_PLUGIN_NAGIOS=On ..
cmake --build . --target nagios-plugin
```

The resulting binary lives at `usr/libexec/netdata/plugins.d/nagios.d.plugin`.
Stock configuration ships in `usr/lib/netdata/conf.d/nagios.d/`.
