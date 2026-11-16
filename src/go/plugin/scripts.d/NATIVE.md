# Native script development

The `native` collector runs executables through scripts.d. A script can report
labeled gauges and counters using scalar lines, or metrics and service checks using
ordinary JSON, without a manifest, an SDK, a `describe` operation, or a chart file.
Packages can add static chart templates, configuration forms, and interactive Functions.

This contract is WIP. It supports one-shot and persistent collection, floating-point
gauges, cumulative counters, scalar line snapshots, enum and bitset StateSets,
package-specific DynCfg forms, and script-provided Functions. The development health template turns warning and
critical checks into notifications. Additional metric kinds and production delivery
remain later steps. The package and wire formats may change during the preview.

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
    command: [/usr/local/lib/netdata/custom/queue/collect.sh]
    update_every: 10
    timeout: 5
```

A direct `command` is an argv array with an absolute executable as its first element;
subsequent arguments are literal and there is no shell expansion. Netdata appends
`collect` for one-shot collection or `serve` for persistent mode. Job `mode` accepts
`auto` (the default), `oneshot`, or `persistent`. For a direct command, `auto` means
one-shot; set `mode: persistent` to retain one process. Initialization and
configuration tests validate the executable without running it, and never probe
`describe`. A script that prints this object when invoked with `collect` is sufficient:

```json
{"version":"v1","metrics":[{"name":"queue_depth","samples":[{"value":17.5}]}]}
```

This creates a floating-point gauge with units `value` and an automatic chart. Use a
meaningful `unit`, such as `jobs`, when the measurement has one. No special library is
required; use your language's JSON encoder.

Job `snapshot_format` accepts `auto` (the default), `json`, or `lines`. For a direct
command, `auto` selects JSON. To emit scalar lines, set `snapshot_format: lines`:

```text
queue_depth:17.5|gauge|unit:jobs
```

Format selection is explicit: the collector does not sniff output or fall back to
another decoder. Execution mode and snapshot format are independent; both formats
support one-shot and persistent collection. See [Scalar line snapshots](#scalar-line-snapshots)
for labels, metadata, quoting, and runnable examples.

For a file-backed package, replace `command` with its manifest path:

```yaml
jobs:
  - name: queue
    manifest: /usr/local/lib/netdata/custom/queue/manifest.yaml
    update_every: 10
    timeout: 5
```

Every generic job MUST select exactly one `manifest` or `command`. A manifest owns
its execution mode and snapshot format, so a job using it MUST leave `mode` and
`snapshot_format` omitted or set to `auto`; concrete overrides are rejected, even
when they match the manifest. Auto does not probe the script. A direct command has no
configuration schema and rejects a nonempty `config` object; use a package to supply
validated configuration on stdin. Use the development binary for these configurations.
There is no package auto-discovery. Explicit registration below provides individual
DynCfg forms and self-contained executable packages.

Generic manifest initialization reads local package files without running the
executable. Editing static package assets requires restarting or reconfiguring the
job. Metric and check families come from each collection response, not those assets.

## Manifest v1

```yaml
version: v1
mode: oneshot
command: [./collect.sh]
charts: charts.yaml
```

- `command` is an argv array, without shell expansion. Its first path is resolved
  relative to the manifest directory unless absolute. Subsequent arguments are
  literal. Netdata appends `collect` or `function` for one-shot calls, or `serve` for persistent mode.
- `config_schema` optionally names a local JSON form, relative to the manifest
  directory. It enables configuration input on stdin in both modes; see below.
- `mode` is `oneshot` (the default when omitted) or `persistent`. Persistent jobs
  retain one process across collection attempts; their state resets on restart.
- `snapshot_format` is `json` (the default when omitted) or `lines`. The package
  declaration does not accept `auto`. Function-only packages MUST use JSON or omit
  the field; `lines` requires periodic collection.
- `charts` is an optional path to the existing [V2 chart-template language](../framework/charttpl/README.md),
  resolved relative to the manifest. Without it, metrics get automatic charts.
  With it, its autogen setting determines whether unmatched metrics get charts.
  The `native_check_` chart-ID prefix is reserved. Global chart selectors are
  always rejected, so dynamically reported checks cannot be silently filtered.
- `functions` optionally declares interactive methods; see Functions below. Packages
  with Functions MUST use startup registration in `scripts.d.packages.yaml`.
- `collect` is an optional boolean, defaulting to true. Set `collect: false` for a
  Function-only package: it MUST declare at least one Function and MUST NOT specify
  `charts`. There is no job-level `collect` override. Merely declaring Functions
  does not disable collection.
- Metric and check declarations do not belong in the manifest; `metrics` and
  `checks` are rejected. Each response carries its complete families instead.
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

Each entry MUST specify exactly one `manifest` path or `command` argv. A
self-contained package uses an absolute executable path:

```yaml
version: v1
packages:
  - name: queue
    command: [/usr/local/lib/netdata/custom/queue/queue-plugin]
```

The manifest source reads metadata files without executing the package. The command
source executes `describe` once during plugin process startup. Both sources compile
the same package definition before registration. The registered module binds its
metadata and command: job configuration MUST NOT set `manifest` or `command`, or
concrete `mode` or `snapshot_format` values to replace them. Registered forms and
configuration GET omit these source-owned fields. Registration never
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
package defaults. The generic `native` module accepts direct commands, or a manifest path and a
`config` object, but its form cannot reflect a particular package schema.
The generic module accepts collection-only packages; Function declarations require
startup registration. Generic jobs load package files during initialization; their pre-initialization
configuration retrieval contains only submitted package values.

### Self-contained executable packages

The host appends the operation argument to the configured argv. For example,
`command: [/usr/bin/python3, /protected/queue.py]` runs
`/usr/bin/python3 /protected/queue.py describe`. The same configured command is used
for `collect`, `serve` and `function`. The description MUST NOT contain `command`
or select another executable.

`describe` MUST print exactly one UTF-8 YAML or JSON document and exit zero. JSON
strings support standard Unicode escapes, including surrogate pairs. Declaration
field names are case-sensitive, and unknown fields and duplicate keys are rejected.
It uses the manifest's `version`, `mode`, `snapshot_format`, `collect` and `functions` fields,
with these inline assets:

| Field | File manifest | Executable description |
|---|---|---|
| `charts` | Path to chart YAML | String containing the complete chart YAML |
| `config_schema` | Path to the configuration form | Object containing `jsonSchema` and `uiSchema` |

Both assets are optional under the same capability rules as file-backed packages.
YAML block scalars make inline chart templates convenient; JSON producers encode
the template as a string. Configuration forms use the same validation, default and
local-reference rules described below. External asset paths are not supported in a
description. Files are never extracted to disk.

Description runs unprivileged through `nd-run`, with EOF on stdin, its minimal
environment, and discarded stderr. It receives no job configuration or secrets.
It MUST return static metadata without contacting the monitored service or starting
background collection. This is an authoring contract, not a network sandbox.
Execution occurs for every command entry in the selected inventory, even before a
job exists or its module is enabled; only register trusted, administrator-controlled
commands. Merely placing an executable in a directory does not register or run it.

Each invocation has a five-second execution deadline and a separately defined
**64 MiB** stdout limit, including whitespace. This startup-only cutoff bounds a
broken producer while leaving generous room for embedded templates and forms;
it is not derived from the size of existing packages. Operational messages have
their own **64 MiB** ceiling. These are encoded-byte cutoffs, not memory budgets:
buffers grow on demand, and copies plus decoded structures can use more memory.
Timeout, nonzero exit, oversized output or invalid metadata rejects plugin startup;
partial definitions are not registered. Command failures report safe categories,
and metadata errors identify the failing document/declarations/form/template stage
without echoing script-produced values. The host terminates contained descendants
and joins its owned process/I/O before completing the operation.

The validated metadata stays in memory for the plugin process lifetime. Creating
jobs, DynCfg Test and configuration updates do not rerun `describe`. Replacing a
binary or changing its metadata requires a **plugin process restart**, not only an
in-process Agent restart. File-backed registered packages have the same metadata
refresh rule. Scripts MUST keep the operational protocol compatible with the loaded
description until that restart.

Runnable single-file examples:

- [`self-contained-bash/collect.sh`](development/self-contained-bash/collect.sh):
  fixed labeled metrics and checks, with no JSON encoder dependency.
- [`self-contained-python/collect.py`](development/self-contained-python/collect.py):
  inline form, chart and interactive Function in one script.
- [`self-contained-go`](development/self-contained-go/main.go): a Go binary using
  `go:embed`; only the resulting executable is needed at runtime.

All examples default to one-shot mode. Put `--persistent` before the appended
operation by adding it to the configured argv to select persistent mode. For the
Go example, build from `src/go` with:

```sh
go build -o native-queue ./plugin/scripts.d/development/self-contained-go
```

Install the resulting file under the protected path used in the inventory. Its
build-time `description.yaml` is embedded and is not a runtime sidecar. The Bash
example uses fixed values; use a proper JSON encoder when adding variable strings.

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

The envelope is at most 64 MiB including LF. One-shot stdin then reaches EOF;
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
Schema-free scripts, including direct commands, receive no configuration envelope.
A nonempty job `config` without a declared schema is rejected.

## Collection response

In one-shot mode, each invocation MUST exit zero and print one complete UTF-8
snapshot in its selected format: one JSON object or the scalar line records below.
The complete output MUST fit in 64 MiB, including whitespace and line endings.
Stdin is EOF for schema-free packages; configured packages receive one configuration
envelope followed by EOF. In persistent mode, the same snapshot is carried in a
correlated reply as described below. Stdout
is exclusively the protocol; redirect command chatter to stderr. The collector
discards stderr to avoid copying arbitrary script output into Agent logs. One-shot
Function failures log only safe command categories (including numeric exit status)
or an invalid-reply diagnostic; script output and request data are never logged.

```json
{
  "version": "v1",
  "metrics": [
    {
      "name": "queue_depth",
      "unit": "jobs",
      "chart_meta": {"title": "Queue depth", "family": "Queues", "priority": 1000},
      "samples": [
        {"value": 17.5, "labels": {"queue": "mail", "region": "east"}},
        {"value": 4, "labels": {"queue": "batch", "region": "east"}}
      ]
    },
    {
      "name": "processed_total",
      "type": "counter",
      "unit": "jobs",
      "samples": [{"value": 100, "labels": {"queue": "mail"}}]
    },
    {
      "name": "worker_state",
      "type": "stateset",
      "states": ["starting", "running", "stopped"],
      "samples": [{"active": ["running"], "labels": {"worker": "alpha"}}]
    }
  ],
  "checks": [
    {
      "id": "backlog",
      "title": "Queue backlog",
      "by_labels": ["queue"],
      "samples": [{"state": "critical", "labels": {"queue": "mail"}}]
    }
  ]
}
```

### JSON metric families

Each `metrics` entry defines one named family and all of its samples. `name` and
`samples` are required. `type` defaults to `gauge`; supported types are `gauge`,
`counter`, and `stateset`. Scalar `unit` defaults to `value`; use a meaningful unit
when available. Optional `chart_meta` supplies automatic-chart `title`, `family`,
and a positive integer `priority`. Authored chart templates control their own chart
presentation and take precedence over automatic charts.

Metric names match `[A-Za-z_][A-Za-z0-9_.]*`. Prefixes `native.` and `native_check_`,
and the exact name `check_state`, are reserved for built-in checks. Labels are
string-to-string objects whose keys match `[A-Za-z_][A-Za-z0-9_]*`; `_collect_job`
is reserved. Sample identity is the metric name plus all supplied labels.

Gauge and counter samples require a finite JSON number in `value`; zero is a real
observation. Counter values MUST be nonnegative cumulative totals, not interval
deltas. Counter resets use the existing incremental chart behavior. Values use
IEEE-754 double precision; integers above 2^53 may lose precision. Scalar families
MUST NOT declare `states` or a StateSet `mode`, and scalar samples MUST NOT contain
`active`.

A StateSet requires a complete nonempty `states` array of unique, nonblank strings.
State names become chart dimension IDs. They MUST NOT contain apostrophes,
backslashes, newlines, carriage returns, NUL, or surrounding whitespace; the
output protocol changes these characters and could merge distinct states.
Internal spaces, Unicode and double quotes are supported. Invalid names reject
the entire response rather than being renamed.
Its `mode` defaults to `enum`; use `bitset` for independent states. Each sample
requires `active`, an array of unique members of that domain, and MUST NOT contain
`value`. Enum samples select exactly one state. Bitset samples may select zero,
one, or several states; `active: []` records that every state is inactive. StateSet
units are fixed to `state`, so omit `unit`. A label key matching the StateSet's
metric name is reserved for its generated state label. StateSets alone do not
create severity alerts; use checks for built-in service health.

For example, these are independent capability flags:

```json
{"name":"worker_capabilities","type":"stateset","mode":"bitset","states":["read","write"],"samples":[{"active":["read","write"],"labels":{"worker":"alpha"}}]}
```

A metric name MUST retain its type and meaning, including unit, StateSet domain
and mode, and chart metadata. Use a new name for a different contract. This applies
after omissions, descriptor expiry, and process or Agent restarts. The collector
rejects known conflicts while the metric descriptor is retained; expiry of that
validation history does not authorize incompatible reuse. Failed collections or
failed metric commits do not establish a new contract. An empty family's metadata
does not establish a contract without a successfully committed sample.

The [snapshot JSON Schema](collector/native/snapshot_schema.json) supports editor
validation. The host additionally checks family/series uniqueness, active-state
membership, identity label values, floating-point range, duplicate JSON keys, and
retained metric contracts.

### Scalar line snapshots

With `snapshot_format: lines`, each non-comment record supplies one observation:

```text
queue_depth:17.5|gauge|#queue:mail,region:east|unit:jobs|title:"Queue depth"|family:Queues|priority:1000
processed_total:100|counter|#queue:mail|unit:jobs
queue_depth:4|gauge|#queue:batch,region:east
```

The required prefix is `name:value|type`. Type MUST be exactly `gauge` or `counter`;
values MUST be finite JSON numbers. Gauges are absolute observations; counters are
nonnegative cumulative totals. This is not StatsD: increments, sample rates, timers,
sets, and abbreviated types such as `g` or `c` are unsupported. Use JSON for StateSets
and service checks.

After the type, these optional fields may appear in any order, at most once each
per record:

| Field | Meaning |
|---|---|
| `#key:value,key:value` | Sample labels; keys MUST be unique within the record |
| `unit:value` | Metric unit; defaults to `value` |
| `title:value` | Chart title |
| `family:value` | Chart family |
| `priority:value` | Positive decimal integer chart priority |

Name and label-key rules, reserved identities, number precision, and metric-contract
stability are the same as JSON. `nd_*` label names have no special meaning; they
are ordinary labels. Metadata fields are distinct from labels.

Records for the same metric name form one family, even when interleaved with other
names. Supply metadata on any record in that family; repeated declarations MUST
agree after string decoding. Defaults apply only after all records have been grouped,
so a later record can supply the unit or chart metadata omitted earlier. Each new
snapshot MUST supply its own metadata: nothing is inherited from earlier snapshots.
A metric MUST keep its meaning after expiry or restart, as described above.

Label and metadata values may be unquoted literals or JSON quoted strings. Use
unquoted literals for fixed values without surrounding whitespace or separators:
`|` ends a field, and `,` also ends a label value. Unquoted values are literal;
backslashes do not introduce escapes. For dynamic or external strings, always use
a JSON encoder, such as Python's `json.dumps()`, and emit its complete quoted result.
Quoted strings support all JSON escapes, including Unicode surrogate pairs and
escaped control characters. Raw control characters inside records are invalid. For example:

```text
queue_depth:2|gauge|#queue:"mail|urgent,west",region:"",note:"line one\nline two"|unit:"jobs"|title:"Queue \u03bb"
```

An empty quoted label value (`""`) is valid. Metadata values MUST be nonempty;
units MUST also contain a non-whitespace character. Unquoted values MUST be nonempty. Metric names, numbers, types, label keys, and
field names are not quoted. `priority` MUST decode to a positive decimal integer
without signs or leading zeros. Duplicate fields or sample identities, conflicting
metadata, malformed quoting, unknown fields, or unsupported types reject the whole
snapshot before any sample is published.

Blank lines and lines beginning with `#` after surrounding space/tab trimming are
ignored. LF and CRLF line endings are accepted. A comment-only snapshot, such as
`# no observations`, intentionally publishes an empty successful snapshot. Empty or
whitespace-only output fails collection. One-shot EOF completes the snapshot; a final
line need not end with LF. Persistent snapshots use the terminator described below.

Runnable examples support both `collect` and `serve`:

- [Bash scalar lines](development/lines-bash/collect.sh).
- [Python scalar lines](development/lines-python/collect.py).

Set `snapshot_format: lines` in the direct job or package declaration, and select
`mode: persistent` when using `serve`. The existing `native.sh` snapshot helpers
continue to emit JSON; they do not switch formats with the job setting.

### Check families and complete snapshots

Each `checks` entry requires `id`, a nonblank `title`, and `samples`. IDs match
`[A-Za-z_][A-Za-z0-9_]*`. Optional `by_labels` lists unique identity-label keys;
each sample MUST supply a nonblank value for every such key. Omit `by_labels` for
one instance of that check per job. Changing identity creates a different check
instance. Extra sample labels are accepted but ignored for built-in check series
and charts. Titles and identity definitions are supplied by the current response;
no manifest declaration is required.

Collection field names are case-sensitive; JSON null is invalid in every position
of a collection snapshot. Each response is a full snapshot. Omit unavailable
samples rather than inventing zero. `metrics` and `checks` may be omitted or empty.
Every included family MUST contain `samples`, which may be empty. Family metadata
without samples is not an observation and does not keep a chart alive.

Duplicate JSON keys, duplicate metric names or check IDs, duplicate sample
identities within a family, unknown fields or states, missing/null required values,
and invalid scalar/StateSet sample shapes invalidate the complete response. Check
identity is its ID plus `by_labels`. No partial metric cycle is published when
validation fails, and failed responses cannot retain check definitions for a later
frame. Empty successful snapshots remove prior observations through the normal
chart lifecycle.

A valid `warning`, `critical`, or `unknown` is a successful collection. In one-shot
mode the script MUST exit zero. Nonzero exit, timeout, oversized or malformed output fails collection
and publishes none of that response. It does not convert checks into OK or
unknown. Netdata's existing `plugin_data_collection_status` owns execution failures;
its stock notification route is silent.

The process MUST finish all children before returning and MUST NOT daemonize or
escape its process group. Each invocation is an owned process with the containment
described under Persistent sessions: exit, timeout or cancellation terminates its
contained descendants, so background work never outlives the invocation, and a
descendant holding stdout cannot delay or fail the result. An invocation fails if
containment cannot be established. One-shot scripts cannot retain process state
across collection attempts.

## Persistent sessions

Set `mode: persistent` in the manifest, executable description, or direct command
job. Netdata starts the command with `serve`
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
correlation values. Reply exactly once, echoing the ID. For JSON snapshots, use:

```json
{"id":"1","result":{"version":"v1","metrics":[],"checks":[]}}
```

For scalar line snapshots, a successful collect reply is a block of records followed
by the exact line `# EOF <id>`, with the request ID substituted and a final LF:

```text
queue_depth:17.5|gauge|unit:jobs
# EOF 1
```

Flush the complete block immediately. CRLF is also accepted. The `# EOF` prefix is
reserved in persistent blocks; malformed terminators and wrong IDs terminate the
session. A comment-only block plus its terminator is a valid empty snapshot; a
terminator alone or preceded only by whitespace fails. The entire block, including
all raw bytes of comments, whitespace, CRLF endings, and the terminator, MUST fit in
64 MiB. EOF before the terminator is a protocol failure.

If the attempt failed and cannot produce a trustworthy full snapshot, reply:

```json
{"id":"1","error":"collection_failed"}
```

This error reply remains JSON in both formats. In line mode it MUST be the first
reply line, with no preceding records or comments and no block terminator.
This fixed error code aborts the collection without publishing any samples; the
same process can answer the next request. It is not a health state. Use an explicit
`unknown` check when that is a valid observation of an existing service.

Handshakes, requests, configuration envelopes, Function replies, collection errors,
and successful JSON collection replies MUST each be one UTF-8 JSON object on one
physical line, terminated by LF and flushed immediately. Only successful line-format
collection replies use the block framing above. A frame is limited to 64 MiB,
including LF and other whitespace. Embedded newlines in strings MUST be escaped.
Exact field spelling, duplicate-key and null rules apply to envelopes as well as
snapshots. A JSON collection reply MUST contain exactly one of `result` or `error`;
the only error code is `collection_failed`. There is one outstanding request and no push channel.
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

Declare methods in the manifest or executable description and register the package
at startup. Set `collect: false` alongside `functions` only when periodic collection
is not supported:

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

The request, including base64 expansion and terminating LF, MUST fit in 64 MiB.
The host reserves space for the largest correlation ID when admitting a request;
oversized requests fail before executing or writing to a script. Replies use the
same 64 MiB bound. The Function context bounds how long the caller waits, including
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

Each reported check plus its identity labels produces a chart in
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
no `jq`, Python, or helper subprocess per sample. `nd_begin` starts one snapshot and
`nd_end` emits its grouped JSON. Declare families, save their handles from
`ND_FAMILY`, and add observations to those handles:

| Helper | Purpose |
|---|---|
| `nd_metric NAME [TYPE [UNIT]]` | Declare a scalar family; defaults are `gauge` and `value` |
| `nd_sample HANDLE VALUE [KEY VALUE ...]` | Add a scalar observation and optional labels |
| `nd_stateset NAME MODE STATE...` | Declare an `enum` or `bitset` family with its complete domain |
| `nd_state_sample HANDLE ACTIVE_COUNT ACTIVE... [KEY VALUE ...]` | Add active states followed by optional labels |
| `nd_check ID TITLE [BY_LABEL...]` | Declare a check family and its identity-label keys |
| `nd_check_sample HANDLE STATE [KEY VALUE ...]` | Add an `ok`, `warning`, `critical`, or `unknown` observation |
| `nd_chart HANDLE TITLE FAMILY PRIORITY` | Add metric chart metadata; blank arguments are omitted |

Family declarations set `ND_FAMILY`. Copy it immediately into a variable in the
current shell; command substitution runs a subshell and loses the declaration.
Handles belong to one snapshot and MUST NOT be reused after the next `nd_begin`.
Samples of different families may be interleaved. These helpers encode strings,
including quotes, backslashes and control characters supported by Bash; Bash
variables cannot represent NUL bytes. The collector validates the full response.

JSON string round-trips are separate from chart-label display: the Agent applies
its standard label normalization, including backslash-to-slash conversion and
normalization of punctuation and control characters. Choose stable entity IDs
that remain distinct after chart-ID and label normalization.

A runnable synthetic script for a direct command job or the manifest above:

```bash
#!/bin/bash
set -eu
source /absolute/path/to/netdata/src/go/plugin/scripts.d/lib/native.sh
[[ ${1:-} == collect ]] || exit 2
nd_begin
nd_metric queue_depth gauge jobs
depth=$ND_FAMILY
nd_chart "$depth" "Queue depth" Queues 1000
nd_sample "$depth" 17.5 queue mail region east
nd_sample "$depth" 4 queue batch region east
nd_metric processed_total counter jobs
processed=$ND_FAMILY
nd_sample "$processed" 100 queue mail
nd_stateset worker_state enum starting running stopped
worker=$ND_FAMILY
nd_state_sample "$worker" 1 running worker alpha
nd_check backlog "Queue backlog" queue
backlog=$ND_FAMILY
nd_check_sample "$backlog" critical queue mail
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

For self-contained packages, run `describe` to inspect metadata without job input.
For collection, supply the documented configuration envelope when a form is declared
and invoke `collect`, or start `serve` and exchange framed requests. Then configure
a development scripts.d job. The collector tests run real Bash commands through the production
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
