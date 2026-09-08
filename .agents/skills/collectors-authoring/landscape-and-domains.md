# Plugin landscape, data types, and per-domain practices

Reference for `collectors-authoring`: the plugin landscape, the ibm.d / Rust / C / PLUGINSD notes, the build and
dev loop, the data types a collector ingests, and per-domain practices. Open when choosing where a collector lives,
which data type it produces, or how the closest existing collectors in its domain are shaped.

## The plugin landscape

| Family | Lang | Platforms | Where in repo | Scope |
|---|---|---|---|---|
| `proc.plugin` | C | Linux | `src/collectors/proc.plugin/` | Kernel `/proc` and `/sys` |
| `apps.plugin` | C | Linux/FreeBSD/macOS/Windows | `src/collectors/apps.plugin/` | Per-process and per-user/group; `processes` Function |
| `cgroups.plugin` | C | Linux | `src/collectors/cgroups.plugin/` | Containers and control groups |
| `ebpf.plugin` | C + eBPF | Linux | `src/collectors/ebpf.plugin/` | Kernel function tracing |
| `network-viewer.plugin` | C | Linux | `src/collectors/network-viewer.plugin/` | L3/L4 sockets; `topology:` Functions |
| `systemd-journal.plugin` / `windows-events.plugin` | C | Linux/Windows | `src/collectors/{systemd-journal,windows-events}.plugin/` | Log/event explorers via Functions |
| `systemd-units.plugin` | C | Linux | `src/collectors/systemd-units.plugin/` | systemd unit state |
| `windows.plugin` | C | Windows | `src/collectors/windows.plugin/` | Windows performance counters |
| `freebsd.plugin` / `macos.plugin` | C | platform-specific | `src/collectors/{freebsd,macos}.plugin/` | OS analogs of `proc.plugin` |
| `statsd.plugin` | C | All | `src/collectors/statsd.plugin/` | StatsD ingestion + synthetic_charts |
| `log2journal` | C | Linux | `src/collectors/log2journal/` | Parse application logs into the systemd journal |
| Niche C plugins | C | various | `src/collectors/<name>.plugin/` | freeipmi, nfacct, tc, xenstat, debugfs, diskspace, slabinfo, idlejitter, timex, cups, ioping, perf |
| `go.d.plugin` | Go (no CGO) | All | `src/go/plugin/go.d/` | Application integrations |
| `ibm.d.plugin` | Go + CGO | Linux, IBM i | `src/go/plugin/ibm.d/modules/` | IBM workloads (DB2, IBM i / AS-400, IBM MQ, WebSphere) |
| `netflow-plugin` | Rust | Linux | `src/crates/netflow-plugin/` | NetFlow v5/v9, IPFIX, sFlow |
| `otel-plugin` | Rust | Linux | `src/crates/otel-plugin/` | OpenTelemetry metrics + logs ingestion (logs queryable via the `otel-logs` Function) |
| `charts.d.plugin` / `python.d.plugin` | Bash / Python | All | `src/collectors/{charts,python}.d.plugin/` | **Legacy** — do not add new modules |

Path conventions: internal C plugins → `src/collectors/<name>.plugin/`; Go orchestrators →
`src/go/plugin/{go.d,ibm.d}/`; Rust plugins → `src/crates/<name>/`.

## ibm.d, Rust SDK, internal C, PLUGINSD

- **ibm.d** (CGO, IBM-vendor workloads) — use the ibm.d framework with `go generate` after touching `contexts.yaml`. See
  `src/go/plugin/ibm.d/AGENTS.md`. Don't reach for ibm.d for non-IBM CGO needs — the framework is shaped around vendor
  drivers; CGO outside the IBM ecosystem is a design discussion.
- **Rust SDK** at `src/crates/netdata-plugin/` — modules `bridge/`, `protocol/`, `rt/`, `charts-derive/`, `schema/`,
  `types/`, `error/`. Documentation lives in `lib.rs` doc-comments — there is no README. New Rust crates go into the
  `src/crates/Cargo.toml` workspace. Reference impl: `src/crates/netflow-plugin/`.
- **Internal C plugins** — mirror an adjacent collector under `src/collectors/<name>.plugin/`; reuse `src/libnetdata/`.
  `libnetdata.h` includes most of libnetdata so individual headers are usually unnecessary. Allocators with the `z`
  suffix (`mallocz`, `callocz`, `strdupz`, `freez`) handle failures via `fatal()`; `freez(NULL)` is safe. JSON parsing:
  json-c. JSON generation: `buffer_json_*`. Linked lists: `DOUBLE_LINKED_LIST_*` macros.
- **PLUGINSD external plugins (any language)** — spec at `src/plugins.d/README.md`. Useful when implementation language
  is dictated by an SDK that go.d / ibm.d / Rust cannot accommodate.

## Build / dev loop

- go.d unit tests: `cd src/go && go test ./plugin/go.d/collector/<name>/...`
- Single-module dev run: `timeout 15s go run ./cmd/godplugin -m <name> -d`
  from `src/go`; success means the module registers, starts a job, and keeps
  running until the timeout stops it.
- Rust: `cargo test -p <crate>`
- Whole-project install: `./netdata-installer.sh`
- ebpfgo (`src/collectors/ebpf.plugin/ebpfgo.plugin`) compiles in **two** configurations and you must
  validate both; a passing run in one proves nothing about the other:
  - The CO-RE code is gated on `__has_include("<name>.skel.h")` inside an outer
    `LIBBPF_MAJOR_VERSION >= 1 && __has_include(<linux/btf.h>)` guard. When either fails, the skeleton
    includes, the CO-RE-only runtime fields, and the `NETDATA_*_CORE_SUPPORTED` define all vanish and only
    the legacy kprobe path is compiled.
  - `.github/workflows/go-tests.yml` states outright that it leaves `NETDATA_*_HAS_SKELETON` absent, so the
    libbpf-tagged CI job exercises the **legacy path only** — generating skeletons needs a BPF toolchain CI
    does not have. The CMake packaging build, by contrast, passes `-I <build>/ebpf-co-re`. So CO-RE-only code
    reaches release builds having never been compiled by the Go job.
  - Legacy path (what CI runs):
    `CGO_CFLAGS="-I<repo>/externaldeps/libbpf/include" go test -tags netdata_ebpf_libbpf -race -count=1 ./...`
  - CO-RE path: add `-I<repo>/build/ebpf-co-re` to `CGO_CFLAGS` and repeat. Confirm the path actually
    switched rather than silently repeating the legacy build — `__has_include("dc.skel.h")` must be true, or
    `gcc -E` must show the CO-RE-only call sites surviving preprocessing.
  - Failure mode this catches: a runtime field declared inside the CO-RE guard but referenced outside it.
    It compiles wherever CO-RE is on and fails everywhere else with `has no member named '<field>'`, which
    surfaces as a distro build break (EL8 ships libbpf 0.x) long after the Go job went green.

## Dealing with data types

A collector ingests one or more of these data types. Each has its own pattern.

### Metrics (time-series numeric data)

The default. Streams as `BEGIN/SET/END` (PLUGINSD) or framework equivalents. Shape via NIDL (`SKILL.md` §3). Storage is
the dbengine; alerts bind to chart `context`; anomaly detection / ML jobs run continuously. Every metric travels via
streaming to parents and to Netdata Cloud — cardinality matters everywhere.

### Logs

Two paths:

- **Structured journaling.** `src/collectors/log2journal/` parses application/access logs (configurable YAML rules in
  `log2journal.d/`, e.g. `nginx-json.yaml`, `default.yaml`) and writes structured fields into the systemd journal. The
  `systemd-journal.plugin` then exposes the entries via a Function (the log explorer in the Netdata UI).
- **OTEL log signals.** `src/crates/otel-plugin/` ingests OTLP logs into a write-ahead log with indexed segments
  (`src/crates/otel-ingestor/`, `sfsq`), queryable via the `otel-logs` Function in the Logs tab.

Platform-specific events: `windows-events.plugin` (Windows event log).

Logs are **not metrics**. Don't try to derive metrics from logs in the collection loop — emit logs as logs, then build
metrics separately if needed.

### Live snapshots (Functions)

Interactive, on-demand tabular data: process lists, network connections, FDB tables, log entries, journal queries,
topology snapshots, flow records. Functions complement metrics; they don't replace them.

Build a Function when the answer is **interactive/tabular live data**. If the answer is a numeric time series, that's a
metric.

Response shape is one of `info_response`, `data_response`, `topology_response`, `flows_response`, `error_response`,
`not_modified_response` (defined in `src/plugins.d/FUNCTION_UI_SCHEMA.json`). New topology payloads use the dedicated
production topology contract in `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`. For Go, use builders in
`src/go/pkg/funcapi/`; Go topology producers should use `src/go/pkg/topology/v1` for the v1 response model and
compact-table helpers. For Rust, implement the `FunctionHandler` trait from the SDK runtime
(`src/crates/netdata-plugin/rt/`).

Functions run concurrently with the collection loop — they must not block it. Validate during development with
`src/go/tools/functions-validation/`.

Reference implementations: `src/collectors/network-viewer.plugin/` (topology + connections),
`src/collectors/systemd-journal.plugin/` (log explorer), `src/collectors/apps.plugin/` (processes).

Backend docs: `src/go/plugin/framework/functions/README.md` (Go), `src/crates/netdata-plugin/rt/src/lib.rs` (Rust
`FunctionHandler`). UI/protocol: `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`,
`src/plugins.d/FUNCTION_UI_REFERENCE.md`. Topology contract: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`,
`src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.

### Topology / interconnections / links

Topology is its own data type — directed/undirected graphs of nodes and links. Sources and consumers:

- **SNMP-discovered topology** (`src/go/plugin/go.d/collector/snmp_topology/`) — LLDP/CDP neighbors, BRIDGE-MIB FDB,
  Q-BRIDGE FDB, ARP tables, STP. Builds on SNMP profiles; extending profiles is usually the right starting point.
- **Live socket topology** (`src/collectors/network-viewer.plugin/`) — local L3/L4 sockets and their inferred
  connections.
- **Streaming graph** (`src/streaming/`) — Netdata parent/child topology.
- **Topology library** at `src/go/pkg/topology/v1` — production Go payload
  helpers for new topology producers. The non-v1 root
  `src/go/pkg/topology/` payload model has been retired and must not be
  reintroduced for topology work.

Topology is consumed via Functions (`topology:*` family), not via metrics. The cardinality of network edges is too high
for time-series storage and the use case is interactive lookup.

### Data enrichment via netipc

When a collector needs data from another collector to enrich its output (a network collector wanting cgroup labels, an
`apps` collector wanting cgroup PIDs, a flow collector wanting interface metadata), use **netipc**. Don't shell out,
don't open private sockets, don't poll log files.

Both client and server roles exist in C, Go, and Rust:

- C: `src/libnetdata/netipc/`
- Go: `src/go/pkg/netipc/`
- Rust: `src/crates/netipc/`

`cgroups.plugin` (`src/collectors/cgroups.plugin/cgroup-netipc.c`) is a real example of a netipc server offering cgroup
metadata to other plugins. Upstream spec, tests, fuzz suite: <https://github.com/netdata/plugin-ipc>.

## Common practices per collector domain

These are descriptive patterns — what existing Netdata collectors do. Use them as defaults; deviate with reason.

### Database collectors

DB collectors often pair metrics (uptime, connections, query rates, replication lag, lock counts, cache hit ratios) with
**Functions for live query analysis**: top queries, slow queries, currently-running queries, locks. Real examples:

- **MySQL** (`src/go/plugin/go.d/collector/mysql/`) — metrics + `mysqlfunc/top_queries.go` + processlist via
  `collect_process_list.go`.
- **PostgreSQL** (`src/go/plugin/go.d/collector/postgres/`) — metrics + `func_top_queries.go` +
  `func_running_queries.go`, dispatched through `func_router.go`.
- MongoDB / Redis are metrics-only today, but the same Function pattern fits if the use case demands it.

Before adding a query Function, decide whether it is in scope for the current
work and record the product/design decision. The operator value of seeing
"what's slow right now" is high and the pattern is established, but Functions
are still a feature surface, not something to add accidentally during unrelated
metric work.

### Network and SNMP collectors

Network/SNMP collectors typically pair metrics with **topology Functions** and FDB / ARP / LLDP enrichment:

- **`snmp` + `snmp_topology`** (`src/go/plugin/go.d/collector/snmp_topology/`) — topology Functions (`func_topology.go`,
  `func_topology_handler.go`, `func_topology_managed_focus.go`, `func_topology_options.go`,
  `func_topology_presentation.go`, `func_topology_depth.go`) on top of SNMP profile data.
- **`network-viewer.plugin`** (`src/collectors/network-viewer.plugin/`) — `topology:` Functions for live socket-level
  topology.

Each managed device is normally its own job with a job-level `vnode`; emitting several virtual nodes from one job is the
product decision described in `SKILL.md` §1.9. FDB/ARP/STP data lands as topology Functions, not metrics — the
cardinality is too high for metrics and the use case is interactive lookup.

### Container / orchestration collectors

Container collectors pair container metrics with **enrichment via netipc**:

- `cgroups.plugin` exposes a netipc server (`src/collectors/cgroups.plugin/cgroup-netipc.c`) that other plugins query to
  map PIDs/cgroups to container/pod identity.
- `apps.plugin` and `network-viewer.plugin` consume this enrichment to label processes and connections with container
  metadata.

When adding a new orchestration source (Kubernetes API, Docker events, Nomad, etc.), think about who downstream needs
the labels and whether to expose them via netipc.

### Web servers and reverse proxies

Web server collectors pair metrics (requests, status codes, latency, upstream errors) with **access-log Functions** when
the access log is structured:

- `log2journal` parses NGINX/Apache/HAProxy access logs (rules under `src/collectors/log2journal/log2journal.d/`).
- The journal explorer Function makes the parsed entries searchable in the dashboard.

If the application's log format is closed or unstructured, only metrics are practical.

### Flow protocols (NetFlow / sFlow / IPFIX)

The Rust `netflow-plugin` (`src/crates/netflow-plugin/`) ingests flows and exposes them via Functions (`flows_response`
shape). Flows are per-record, high-cardinality, and not suitable for traditional metric storage. Reference fixtures and
provenance discipline live under `src/crates/netflow-plugin/testdata/`. Topology enrichment (interface names, AS
metadata) typically comes from netipc or from SNMP-collected interface data.

### Application servers and middleware

Java app servers, message queues, application middleware — JMX/HTTP/protobuf metrics are the default; some pair with log
exploration via journal or OTEL log signals when the workflow benefits from it. Mirror the closest existing collector.

### OS/kernel collectors

Internal C plugins under `src/collectors/`. Reuse shared metric definitions from `src/collectors/common-contexts/`;
follow chart-priority conventions in `src/collectors/all.h`; lean on `src/libnetdata/` rather than reimplementing
utilities.
