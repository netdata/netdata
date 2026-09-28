# Native script development

The `native` collector runs language-neutral executable packages through scripts.d.
A package supplies a manifest, an executable, and optionally a V2 chart template.
Scripts report labeled metrics and named service checks. The development health
template turns warning and critical checks into notifications.

This contract is WIP. The first implementation supports one-shot collection,
scalar gauges and cumulative counters. Persistent processes, additional metric
kinds, script-specific DynCfg forms, and Functions are later steps. The manifest
and wire format may change during the preview.

## Development build and configuration

Native is excluded from ordinary scripts.d builds and its DynCfg registry. The
helper, example config and health template are not installed, and native metadata
is excluded from the integration catalog. Build explicitly for development:

```sh
cd src/go
go build -tags scripts_native_dev -o /tmp/scripts.d.plugin ./cmd/scriptsdplugin
```

The source fixtures in `development/native.conf` and
`development/native_script.conf` are retained for testing. For manual developer
tests, copy them into a scratch configuration tree as `scripts.d/native.conf` and
`health.d/native_script.conf`. The smoke test below prepares its own tree.
`collector/native/metadata.dev.yaml` is the future integration metadata fixture;
its name deliberately excludes it from normal catalog discovery.

Place the package in an administrator-controlled directory, for example
`/usr/local/lib/netdata/custom/queue/`. On Unix, the command executable and every
ancestor directory MUST be root-owned and MUST NOT be writable by group or others.
The executable MUST be a regular executable file. Symlinks are resolved. Package
files and sourced libraries SHOULD have the same ownership and write protection.
On Windows use an executable or an explicit interpreter command and appropriate
ACLs; Unix permission validation is not implemented there. The Bash helper requires
Bash 3.2 or newer.

Netdata executes the command unprivileged through `nd-run`. Scripts MUST work with
its sanitized environment and MUST NOT depend on the Agent's credentials or current
working directory. Use absolute paths for resources. An interpreter in `command`
is validated as an executable; files passed as arguments are the administrator's
responsibility. Arguments MUST NOT contain secrets.

Enable `native` in `scripts.d.conf` and add a job in `scripts.d/native.conf`:

```yaml
jobs:
  - name: queue
    manifest: /usr/local/lib/netdata/custom/queue/manifest.yaml
    update_every: 10
    timeout: 5
```

Use the development binary for this configuration. There is no package
auto-discovery. Initialization reads and validates local files without running
the executable. Editing package files requires restarting or
reconfiguring the job; charts and declarations are fixed for that job instance.

## Manifest v1

```yaml
version: v1
command: [./collect.sh]
charts: charts.yaml
metrics:
  - name: queue_depth
    type: gauge
    unit: jobs
  - name: processed_total
    type: counter
    unit: jobs
checks:
  - id: backlog
    title: Queue Backlog
    by_labels: [queue]
```

- `command` is an argv array, without shell expansion. Its first path is resolved
  relative to the manifest directory unless absolute. Subsequent arguments are
  literal. Netdata appends `collect` to invoke collection.
- `metrics` declares each name, type (`gauge` or `counter`) and nonempty unit.
  Names match `[A-Za-z_][A-Za-z0-9_.]*`; the `native.` prefix is reserved.
- `checks` declares each check ID, title, and stable identity label names.
  IDs and label keys match `[A-Za-z_][A-Za-z0-9_]*`. Omit `by_labels` for one
  instance of a check per job. Identity labels MUST be present and nonempty in
  each observation. Changing identity creates a different check instance.
- `charts` is an optional path to the existing [V2 chart-template language](../framework/charttpl/README.md),
  resolved relative to the manifest. Without it, metrics get automatic charts.
  With it, its autogen setting determines whether unmatched metrics get charts.
  The `native_check_` chart-ID prefix is reserved. Global chart selectors are
  rejected when checks are declared, so they cannot silently filter health data.
- At least one metric or check MUST be declared. Unknown manifest fields are errors.

Example `charts.yaml`, using an identity label and changeable chart metadata:

```yaml
version: v1
context_namespace: custom_queue
engine:
  autogen:
    enabled: true
    expire_after_success_cycles: 1
groups:
  - family: Queue
    metrics: [queue_depth]
    charts:
      - id: queue_depth
        title: Queue Depth
        context: depth
        units: jobs
        instances:
          by_labels: [queue]
        label_promotion: [region]
        dimensions:
          - selector: queue_depth
            name: depth
```

`region` can change without changing the chart ID. Metric names, labels and
instances SHOULD represent a bounded set of monitored entities, never request
IDs, timestamps, diagnostic messages or other continuously changing identifiers.

## Collection response

Each invocation MUST exit zero and print exactly one UTF-8 JSON object, at most
1 MiB including whitespace. Stdin is EOF and reserved for future requests. Stdout
is exclusively the protocol; redirect command chatter to stderr. The collector
discards stderr to avoid copying arbitrary script output into Agent logs.

```json
{
  "version": "v1",
  "metrics": [
    {"name": "queue_depth", "value": 17, "labels": {"queue": "mail", "region": "east"}},
    {"name": "processed_total", "value": 100, "labels": {"queue": "mail"}}
  ],
  "checks": [
    {"id": "backlog", "state": "critical", "labels": {"queue": "mail"}}
  ]
}
```

Values MUST be finite JSON numbers; zero is a real observation. Counter values
MUST be nonnegative cumulative totals, not interval deltas. Counter resets use
the existing incremental chart behavior. Values use IEEE-754 double precision;
integers above 2^53 may lose precision. Labels are string-to-string objects.

Field names are case-sensitive; JSON null is invalid in every position.
Each response is a full snapshot. Omit unavailable metric samples rather than
inventing zero. An empty metrics or checks array is valid; arrays may be omitted.
A sample name MUST be declared. Duplicate JSON keys, duplicate metric identities
(name plus all labels), duplicate check identities (ID plus `by_labels`), unknown
fields, missing/null values and unknown states invalidate the complete response.
Checks may have extra labels, but the built-in charts promote only identity labels.

A valid `warning`, `critical`, or `unknown` is a successful collection and MUST
exit zero. Nonzero exit, timeout, oversized or malformed output fails collection
and publishes none of that response. It does not convert checks into OK or
unknown. Netdata's existing `plugin_data_collection_status` owns execution failures;
its stock notification route is silent.

The process MUST finish all children before returning and MUST NOT daemonize or
escape its process group. Cancellation terminates the command group on Unix using
the existing execution helper. One-shot scripts cannot retain process state across
collection attempts.

## Checks and automatic alerts

Each declared check plus identity labels produces a chart in
`native_script.check_state`. It exposes `ok`, `warning`, `critical`, and `unknown`
as explicit zero/one dimensions. The development `native_script_check` template
evaluates every 10 seconds and routes warning/critical notifications to `sysadmin`.

| Observation | Health meaning |
|---|---|
| `ok` | CLEAR; explicit recovery |
| `warning` | WARNING |
| `critical` | CRITICAL, including the first observation |
| `unknown` | UNDEFINED; no trustworthy measurement, not recovery |
| Omitted in a successful response | Entity removed; chart becomes obsolete after one missing successful cycle |
| Execution/protocol failure | No new check sample; normal Agent stale-data behavior applies |

A check that still exists but cannot be measured MUST explicitly report `unknown`.
Omission means disappearance, not uncertainty. Obsolete charts are eventually
REMOVED by normal Agent cleanup, rather than recording a healthy result. These
are Netdata health semantics, including its storage interpolation, evaluation
cadence and notification policy; they do not emulate Nagios retries.

## Bash helper

The pure Bash library stays in the source tree at `lib/native.sh`; it is not
installed. Source it using an absolute path in development scripts. It requires
no `jq`, Python, or helper subprocess per sample. Use `nd_begin`,
`nd_metric NAME VALUE [KEY VALUE ...]`, `nd_check ID STATE [KEY VALUE ...]`, and
`nd_end`. These functions encode strings, including quotes, backslashes and control
characters supported by Bash; Bash variables cannot represent NUL bytes.

JSON string round-trips are separate from chart-label display: the Agent applies
its standard label normalization, including backslash-to-slash conversion and
normalization of punctuation and control characters. Choose stable entity IDs
that remain distinct after chart-ID and label normalization.

A runnable synthetic example for the manifest above:

```bash
#!/bin/bash
set -eu
source /absolute/path/to/netdata/src/go/plugin/scripts.d/lib/native.sh
[[ ${1:-} == collect ]] || exit 2
nd_begin
nd_metric queue_depth 17 queue mail region east
nd_metric processed_total 100 queue mail
nd_check backlog critical queue mail
nd_end
```

Replace the fixed observations with measurements of your service. Use safe command
argument arrays. When running consequential commands, show a redacted command on
stderr, preserve its exit status, and keep credentials out of diagnostics. A failed
measurement may report an explicit unknown check; a failure to produce a complete
protocol response MUST exit nonzero. Helper errors MUST propagate (for example,
with `set -e` as above); do not emit a partial response after ignoring an error.

## Local validation

Run the executable with the `collect` operation to inspect its JSON, then configure
a scripts.d job. The collector tests run real Bash commands through the production
V2 job and command path with a test privilege-drop shim, and test encoding, complete
snapshot validation, startup in a critical state and cancellation.

```sh
cd src/go
go test ./plugin/scripts.d/collector/native/...
```

For a real health-runtime smoke test, build an Agent and `nd-run` in a scratch
directory. Build scripts.d with `-tags scripts_native_dev` and
`pkg/buildinfo.NetdataBinDir` pointing to that directory, then run:

```sh
python3 src/go/plugin/scripts.d/tests/native_health.py --agent /path/to/netdata --plugin /path/to/scripts.d.plugin
```

The test launches its own unprivileged Agent on a loopback port, suppresses crash
reports and notification delivery, checks startup, unknown, recovery and removal,
and stops only that Agent. It retains its temporary artifacts for inspection.
It does not query or reconfigure an installed Agent. Allow roughly two minutes
for the stock health cadence and obsolete-chart cleanup.
