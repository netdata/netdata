# Native script development

The `native` collector runs language-neutral executable packages through scripts.d.
A package supplies a manifest, an executable, and optionally a V2 chart template.
Scripts report labeled metrics and named service checks. The development health
template turns warning and critical checks into notifications.

This contract is WIP. It supports one-shot and persistent collection, scalar gauges
and cumulative counters, package-specific DynCfg forms, and script-provided Functions.
Additional metric kinds and production delivery remain later steps. The manifest
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
auto-discovery. Explicit package registration below provides individual DynCfg forms. Initialization reads and validates local files without running
the executable. Editing package files requires restarting or
reconfiguring the job; charts and declarations are fixed for that job instance.

## Manifest v1

```yaml
version: v1
mode: oneshot
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
  literal. Netdata appends `collect` or `function` for one-shot calls, or `serve` for persistent mode.
- `config_schema` optionally names a local JSON form, relative to the manifest
  directory. It enables configuration input on stdin in both modes; see below.
- `mode` is `oneshot` (the default when omitted) or `persistent`. Persistent jobs
  retain one process across collection attempts; their state resets on restart.
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
- `functions` optionally declares interactive methods; see Functions below. Packages
  with Functions MUST use startup registration in `scripts.d.packages.yaml`.
- At least one metric, check or Function MUST be declared. Packages with Functions
  and no metrics/checks are function-only: no periodic collection or charts run.
  Function-only manifests MUST NOT specify `charts`.
- Unknown manifest fields are errors.

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

## Package registration and DynCfg

For a package-specific form, create `scripts.d.packages.yaml` in the Netdata
configuration directory (next to `scripts.d.conf`):

```yaml
version: v1
packages:
  - name: queue
    manifest: /usr/local/lib/netdata/custom/queue/manifest.yaml
```

The first matching inventory in the configured directory search order wins.
Package names contain lowercase letters, digits and single hyphens between words,
beginning with a letter. Each entry registers module `native-<name>`, for example
`native-queue`. Duplicate names or invalid packages reject startup. Ordinary
production builds ignore this inventory entirely.

The package's manifest, chart templates and configuration schema are read once at
plugin startup. Changes to these files or the inventory require a plugin restart.
The registered module binds that package: job configuration cannot replace its
manifest or executable. Package registration never runs the executable and never
creates a default job. Add a job through DynCfg or `scripts.d/native-queue.conf`:

```yaml
jobs:
  - name: local
    update_every: 10
    timeout: 5
    config:
      queue: mail
      depth: 15
```

Normal module enablement applies: `default_run: yes` exposes registered package
modules unless disabled individually. With `default_run: no`, enable
`native-queue` under `modules` in `scripts.d.conf`. Enabling the generic `native`
module does not enable package modules. `-m native-queue` selects that module.

DynCfg serves the package form under `config`, alongside collection settings.
GET preserves submitted secret references; effective configuration also includes
package defaults. The generic `native` module remains usable with a manifest path
and a `config` object, but its form cannot reflect a particular package schema.
The generic module accepts collection-only packages; Function declarations require
startup registration. Generic jobs load package files during initialization; their pre-initialization
configuration retrieval contains only submitted package values.

### Package configuration schema

Add `config_schema: config_schema.json` to the manifest. The file uses the existing
DynCfg form envelope; its `jsonSchema` describes only the package's `config` object:

```json
{
  "jsonSchema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "title": "Queue settings",
    "description": "Configure the queue to monitor.",
    "additionalProperties": false,
    "properties": {
      "queue": {
        "type": "string",
        "title": "Queue",
        "description": "Name of the queue to monitor.",
        "default": "mail"
      }
    }
  },
  "uiSchema": {}
}
```

Use Draft 7 and local fragment references (`#` or `#/definitions/...`); external
resources and `$id` are rejected. References must target standard Draft 7 schema
locations, such as `definitions` or `properties`; annotation objects cannot be
reference targets. Nested dialect declarations must also select Draft 7. Local
references are rebased when the form is embedded in the job schema. Use `ui:widget: password` for credential fields.
Schema compilation and runtime errors omit submitted values. Configuration numbers
use IEEE-754 double precision; represent exact large identifiers as strings.

Defaults are applied along direct `properties` and homogeneous array `items`.
An absent object is created only when that property has a default, such as `{}`.
Existing objects receive defaults for missing properties; explicit false, zero,
and null property values are preserved. Omitted `config` or `config: null` uses
package defaults. Defaults inside conditional/composed schemas, definitions,
pattern properties, or tuple items are rejected: put a default directly on the
property where it is used. Schema validation still supports those constructs.
Defaults MUST NOT contain secret references. Put references in job configuration,
where the existing resolver can resolve and track them before initialization.

Full schema validation runs during Init, including DynCfg Test, active-update
preflight, and activation. Passive ADD/disabled edits retain the framework's
structural/reference-syntax admission; invalid values cannot start a script.
An invalid active update leaves the existing job running. An accepted valid update
replaces its process through the existing lifecycle; scripts do not hot-reload
configuration themselves.

### Configuration input

When `config_schema` is present, the host sends exactly one UTF-8 JSON object on
one LF-terminated line to stdin before the script performs collection or emits
persistent readiness:

```json
{"version":"v1","config":{"queue":"mail"}}
```

The envelope is at most 1 MiB including LF. One-shot stdin then reaches EOF;
persistent stdin continues with normal collection requests after readiness.
Initialization has the same timeout as persistent startup. Blocking or failing to
consume configuration fails startup/collection. Values are never passed through
argv, environment variables, or temporary files by the collector. Scripts MUST
keep credentials out of their logs, metric labels, and child command arguments.

Bash can call `nd_read_config` to read the envelope into the unexported shell
variable `ND_CONFIG`, then use a JSON decoder such as `jq`. Python can use
`json.loads(sys.stdin.readline())["config"]`. See the runnable synthetic examples
in `development/configured-bash` and `development/configured-python`; the Bash
example requires `jq`. Both example scripts support `collect` and `serve`.
Schema-free scripts keep their existing stdin and readiness behavior.

## Collection response

In one-shot mode, each invocation MUST exit zero and print exactly one UTF-8 JSON
object, at most 1 MiB including whitespace. Stdin is EOF for schema-free packages;
configured packages receive one configuration envelope followed by EOF. In persistent mode, the
same snapshot is carried in a correlated reply as described below. Stdout
is exclusively the protocol; redirect command chatter to stderr. The collector
discards stderr to avoid copying arbitrary script output into Agent logs. One-shot
Function failures log only safe command categories (including numeric exit status)
or an invalid-reply diagnostic; script output and request data are never logged.

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

Collection field names are case-sensitive; JSON null is invalid in every position
of a collection snapshot.
Each response is a full snapshot. Omit unavailable metric samples rather than
inventing zero. An empty metrics or checks array is valid; arrays may be omitted.
A sample name MUST be declared. Duplicate JSON keys, duplicate metric identities
(name plus all labels), duplicate check identities (ID plus `by_labels`), unknown
fields, missing/null values and unknown states invalidate the complete response.
Checks may have extra labels, but the built-in charts promote only identity labels.

A valid `warning`, `critical`, or `unknown` is a successful collection. In one-shot
mode the script MUST exit zero. Nonzero exit, timeout, oversized or malformed output fails collection
and publishes none of that response. It does not convert checks into OK or
unknown. Netdata's existing `plugin_data_collection_status` owns execution failures;
its stock notification route is silent.

The process MUST finish all children before returning and MUST NOT daemonize or
escape its process group. Cancellation terminates the command group on Unix using
the existing execution helper. One-shot scripts cannot retain process state across
collection attempts.

## Persistent sessions

Set `mode: persistent` in the manifest. Netdata starts the command with `serve`
after the preceding job has stopped. Local initialization and configuration tests
never start the process. After fallible local setup, the script MUST emit and flush:

```json
{"version":"v1","ready":true}
```

Readiness does not require a successful target measurement. Netdata then writes
one request to stdin per collection. The canonical spelling is fixed so Bash can
read it without a general JSON parser:

```json
{"id":"1","method":"collect"}
```

IDs are positive decimal strings, increasing per process. Treat them as opaque
correlation values. Reply exactly once, echoing the ID and using the snapshot above:

```json
{"id":"1","result":{"version":"v1","metrics":[],"checks":[]}}
```

If the attempt failed and cannot produce a trustworthy full snapshot, reply:

```json
{"id":"1","error":"collection_failed"}
```

This fixed error code aborts the collection without publishing any samples; the
same process can answer the next request. It is not a health state. Use an explicit
`unknown` check when that is a valid observation of an existing service.

Every handshake, request and reply MUST be one UTF-8 JSON object on one physical
line, terminated by LF and flushed immediately. A frame is limited to 1 MiB,
including LF and other whitespace. Embedded newlines in strings MUST be escaped.
Exact field spelling, duplicate-key and null rules apply to envelopes as well as
snapshots. A reply MUST contain exactly one of `result` or `error`; the only error
code is `collection_failed`. There is one outstanding request and no push channel.
Scripts MAY perform background work, but MUST NOT emit unsolicited frames.

`timeout` bounds startup and the caller's collection wait, including time behind
an active Function. An admitted collection exchange gets a full `timeout` to
finish, independently of the caller's remaining wait budget. A caller timeout
publishes no sample; the host still drains and validates the outstanding reply
before accepting another request. An exchange exceeding its full budget stops
the session. Function callers use their own deadline, with bounded reply draining
after cancellation as described below. These are not process lifetime limits.
A mismatched ID, malformed/oversized frame, partial frame at EOF, closed stdout
or process exit terminates the session.
After readiness this marks the job Failed and requires an explicit restart or
reconfiguration. No private respawn loop retries it. Failures before readiness use
the normal `autodetection_retry` policy. In either case, no synthetic healthy sample
is published. Stop or replacement cancels in-flight I/O, requests termination of
contained processes, and waits for the leader and owned I/O to finish. Leader exit
also triggers descendant termination. On Linux, macOS and FreeBSD, the leader is
kept unreaped until its process group has been signaled; scripts MUST NOT daemonize
or escape that group. On Windows 10 or later, the process starts inside an owned
Job Object that contains its descendants. Startup fails if containment cannot be
established. Discarded stderr uses the null device, so inherited stderr cannot keep
a host-side copying goroutine alive. Descendant exits are not individually awaited.

## Functions

Declare methods in the manifest and register the package at startup:

```yaml
functions:
  - id: items
    name: Queue Items
    help: Inspect the selected queue.
    update_every: 10
    response_type: table
    accepted_params: [filter]
    required_params:
      - id: queue
        name: Queue
        type: select
        options:
          - {id: mail, name: Mail, defaultSelected: true}
```

IDs match `[a-z][a-z0-9_-]*` and are unique within the package. `name` and `help`
are required. `update_every` is the UI refresh interval in seconds (default 10);
`response_type` defaults to `table`. `has_history` advertises time-range support
but does not implement history. Parameter IDs match `[A-Za-z_][A-Za-z0-9_-]*`.
Selectors support `select` (default), `multiselect`, `help`, `unique_view`, and
options with `id`, `name`, `defaultSelected` and `disabled`.

For package `queue`, method `items` is published as `native-queue:items`. Netdata
owns registration, removal, request deadlines and the reserved `__job` selector.
Unscoped `info` is answered from the manifest without calling the script. Scoped
`info` selects a running job and calls its script. With multiple jobs, callers
select one through `__job`; a missing or invalid selection follows the standard
Function interface. Restart the plugin after changing declarations.

Scripts receive **raw input**. `required_params` and `accepted_params` describe
the UI; they do not validate or default domain values on the host. The script
MUST parse, default and validate its own args/payload, including duplicate or
unknown selector values. Netdata validates `__job`; scripts MUST NOT declare it.
This preview uses the existing read-oriented Function publication permissions.
It does not provide a separate authorization contract for remote mutations.

### Input and execution

One-shot Function calls append `function` to `command`. Stdin contains the
configuration envelope first, if `config_schema` is declared, then one Function
request and EOF. Collection still appends `collect` and receives only optional
configuration. Each Function call owns an independent process and may overlap
one-shot collection or another Function call.

Persistent packages use the existing `serve` startup and ready handshake. After
readiness, collection and Function requests share one serial request/reply stream.
A long Function delays collection; keep interactive work short or use one-shot
mode when calls need independent execution. A function-only persistent package
still starts and signals readiness when enabled, but never receives `collect`.

Both modes receive this Function envelope as one compact JSON line:

```json
{"id":"1","method":"function","function":"items","info":false,"args":[],"payload_base64":"eyJzZWxlY3Rpb25zIjp7InF1ZXVlIjpbIm1haWwiXX19","content_type":"application/json","deadline_unix_ms":1800000000000,"permissions":"0xFFFF","source":"user=test"}
```

`args` is an array, empty when none were supplied. `payload_base64` preserves the
original payload bytes; decode it before parsing according to `content_type`.
The v3 UI sends selectors as `{"selections":{"queue":["mail"]}}` in its JSON
payload, including arrays for single-select fields. Scripts MUST handle this
shape when declaring selectors; see the [Function UI request flow](../../../plugins.d/FUNCTION_UI_REFERENCE.md#modern-flow-v3).
Payload, content type, permissions and source are omitted when empty. The absolute
Unix deadline in milliseconds is present when the caller supplies a deadline.
Arguments, payload and configuration stay on stdin, never in host-generated argv,
environment variables or diagnostics. Scripts MUST NOT expose credentials in
Function output or error messages; those results are sent to the requesting UI.

The request, including base64 expansion and terminating LF, MUST fit in 1 MiB.
The host reserves space for the largest correlation ID when admitting a request;
oversized requests fail before executing or writing to a script. Replies use the
same 1 MiB bound. The Function context bounds how long the caller waits, including
queue waiting. Cancellation before admission leaves the persistent peer usable.
After admission, cancellation or deadline expiry returns to the caller promptly;
the host allows up to `timeout` more seconds to finish writing and drain the
matching reply. It validates and discards that reply before accepting another
request. A missing, invalid or mismatched reply stops the session. While draining,
other requests keep waiting under their own deadlines. A script SHOULD promptly
return a valid error response when its supplied deadline expires. There is no
script cancellation frame or private process restart. One-shot cancellation still
terminates its owned process; job stop/replacement immediately terminates either mode.

### Results

Echo the request ID and return a `v1` result. One-shot scripts MUST exit zero;
persistent scripts MUST flush a single LF-terminated reply:

```json
{"id":"1","result":{"version":"v1","status":200,"columns":{"queue":{"index":0,"name":"Queue","type":"string"},"depth":{"index":1,"name":"Depth","type":"integer"}},"data":[["mail",17]],"default_sort_column":"depth"}}
```

A managed result supplies `status`, plus `columns` and `data` for successful data
requests. `data` is an array of rows ordered by column index. Scoped info can
return just `version` and `status`; optionally return `required_params` to update
domain selector options. Netdata adds metadata and the job selector, and merges
dynamic selectors with declarations. Other managed fields are `message`, `help`,
`type`, `default_sort_column`, `charts`, `default_charts`, and `group_by`, following
[the Function UI schema](../../../plugins.d/FUNCTION_UI_SCHEMA.json).

To report a request failure without destroying a healthy persistent session:

```json
{"id":"1","result":{"version":"v1","status":503,"message":"Queue is temporarily unavailable"}}
```

For formats requiring the complete UI envelope, use explicit raw response ownership:

```json
{"id":"1","result":{"version":"v1","raw_response":{"status":500,"errorMessage":"Queue is temporarily unavailable"}}}
```

`raw_response` MUST contain a JSON integer status from 100 through 599 and
MUST NOT be combined with managed result fields. Netdata sends this complete
object without adding metadata, selectors or table fields. The script owns its
UI-schema validity, including scoped info. Use managed results for ordinary tables.

Protocol field names are exact and duplicate JSON keys are rejected. Null values
and arbitrary JSON numbers are allowed inside table data, column definitions and
raw responses; integers are preserved through host serialization. The frontend's
numeric precision may differ. Invalid envelopes, versions, field types, IDs or
other protocol failures stop a persistent session. A valid script error status
is an ordinary Function result and leaves it available for another request.

Runnable examples are [`development/functions-bash`](development/functions-bash/collect.sh)
(mixed collection and Functions, using jq) and
[`development/functions-python`](development/functions-python/inspect.py)
(function-only). Both support either manifest mode. Tests route their actual
responses through Agent stdin dispatch and validate emitted info/data against the
canonical UI schema. Function-only DynCfg forms omit the collection interval;
the timeout field is shown only in persistent mode, for startup and reply draining
after cancellation. Function callers always use their own deadline.

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

For persistent Bash, call `nd_ready` once and loop over `nd_next`. Each successful
`nd_next` MUST be answered by `nd_begin` / observations / `nd_end`, or by `nd_fail`.
`nd_end` automatically wraps a pending request's snapshot with its ID. `nd_next`
reads the host's canonical **collection-only** request line; EOF or an invalid request ends the loop.
Do not pipe the loop into a subshell if its state must survive.

For packages with Functions, use `nd_read_request` to read a line into the shell
variable `ND_REQUEST`, parse it with jq or another JSON decoder, and call
`nd_reply ID COMPACT_RESULT_JSON`. This wraps a compact JSON object produced by
an encoder; it does not validate that object's schema. Do not use `nd_next` for
a mixed stream. `nd_read_config` still precedes request reading when configured.

Runnable development packages are in
[`development/persistent-bash`](development/persistent-bash/collect.sh) and
[`development/persistent-python`](development/persistent-python/collect.py), each
with a manifest. Both retain a counter, report critical on the first request,
report a collection failure on the second, and recover on the third. They use
synthetic observations. For manual inspection, send three request lines to `serve`;
for Agent use, copy the package/helper to administrator-controlled paths or use an
administrator-managed interpreter command. Python peers MUST flush each reply.

Replace the fixed observations with measurements of your service. Use safe command
argument arrays. When running consequential commands, show a redacted command on
stderr, preserve its exit status, and keep credentials out of diagnostics. A failed
measurement may report an explicit unknown check; a failure to produce a complete
protocol response MUST fail the attempt (`nd_fail` in a live session, or a nonzero
exit for a terminal failure). Helper errors MUST propagate (for example,
with `set -e` as above); do not emit a partial response after ignoring an error.

## Local validation

Run the executable with the `collect` operation to inspect its JSON, then configure
a scripts.d job. The collector tests run real Bash commands through the production
V2 job and command path with a test privilege-drop shim, and test encoding, complete
snapshot validation, startup in a critical state, persistent state/recovery, terminal
protocol failure, cancellation and descendant cleanup.

```sh
cd src/go
go test ./plugin/scripts.d/collector/native/...
```

For a real health-runtime smoke test, build an Agent and `nd-run` in a scratch
directory. Build scripts.d with `-tags scripts_native_dev` and
`pkg/buildinfo.NetdataBinDir` pointing to that directory, then run:

```sh
python3 src/go/plugin/scripts.d/tests/native_health.py --agent /path/to/netdata --plugin /path/to/scripts.d.plugin --mode persistent
```

The test launches its own unprivileged Agent on a loopback port, suppresses crash
reports and notification delivery, checks startup, unknown, recovery and removal,
and stops only that Agent. It retains its temporary artifacts for inspection.
It does not query or reconfigure an installed Agent. Allow roughly two minutes
for the stock health cadence and obsolete-chart cleanup. Omit `--mode persistent`
to exercise the default one-shot transport.
