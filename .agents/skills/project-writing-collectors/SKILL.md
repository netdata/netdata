---
name: project-writing-collectors
description: Best practices and orientation for AI assistants authoring or modifying Netdata data-collection plugins or modules in any language. Read before adding a new collector, modifying an existing one, working on logs, topology, NetFlow/sFlow/IPFIX, OTEL ingestion, SNMP profiles, statsd, Prometheus scraping, or interactive Functions. Covers the mental model, framework-agnostic best practices, dashboard-shaping mechanisms (NIDL, SNMP profiles, statsd synthetic_charts, OTEL mappings, Prometheus exposition), production quality criteria, the plugin landscape, per-data-type patterns (metrics, logs, snapshots, topology, enrichment), per-domain common practices, and a pre-PR self-check.
type: project
---

# Writing Netdata data collection plugins and modules

## What this skill is

You are about to add or modify data collection in the Netdata Agent. This skill is a manifesto and a routing map. It tells you the mindset to apply, the principles you cannot violate, the ways the dashboard gets shaped from upstream data, the quality bar that separates a draft from a shippable collector, and where to look for depth. It is not a tutorial — the deep references already exist in the repo. Your job is to know they exist, pick the right one, and produce work that blends with the patterns the maintainers already accept.

The skill is organized as: mental model → best practices → dashboard shaping → quality bar → environment reference → applied per data type → applied per domain. Read top to bottom on your first pass; come back to specific sections as the task narrows.

## 1. Mental model

How to think about Netdata data collection. Internalize this before designing anything.

### 1.1 Frequent collection at scale

The Agent ships on >1.5M new daily installs across physical servers, VMs, containers, IoT devices, embedded systems, and exotic Unixes. Default collection is 1-second; many collectors raise it (`ping` 5s, SNMP 10s) when the source warrants it. Anything you do inside the collection cycle — allocate, log, reconnect, retry, parse, format — is multiplied by that population. Hot-path discipline is the entry ticket, not an optimization.

### 1.2 Metric structure is dashboard UX

How dimensions group into charts and how labels attach to instances *is* the dashboard the user sees. Mirroring upstream data structures one-to-one produces a chart per metric, which is unusable. **NIDL** — Nodes, Instances, Dimensions, Labels — is the model. Every dashboard-shaping mechanism (§3) feeds into it.

### 1.3 IDs are public contracts

Chart `context`, chart IDs, dimension IDs, instance labels — once shipped, they bind health alerts, dashboards, exports, anomaly detection, ML jobs, streaming consumers, and Netdata Cloud. Renaming silently breaks all of them. Treat them as permanent.

### 1.4 Gaps are data

When you cannot measure a value this iteration, emit nothing for that dimension. The dashboard renders the gap; the user knows collection is broken. Defaulting to `0` fabricates a working state and hides the bug. Past pain in `src/collectors/proc.plugin/proc_net_dev.c` (search `shouldn't use 0 value, but NULL`).

### 1.5 Obsolete what's gone

When the collector knows an entity has gone away — a process exited, a container was removed, a profile target was dropped, a network interface disappeared, a managed device went offline — mark its chart obsolete. The dashboard then renders it as historical, not as actively collected; alerts stop binding to it; streaming and ML stop costing for it.

This is a truthfulness principle, not a cardinality one. It applies at any cardinality, including a single instance. Without obsoletion, the chart looks alive on the dashboard, alerts may continue evaluating against frozen data, and the user is misled about what is and isn't being collected.

Mechanics:
- C: `rrdset_is_obsolete___safe_from_collector_thread()` in `src/database/rrdset.c:116` flags `RRDSET_FLAG_OBSOLETE`. Reverse with `rrdset_isnot_obsolete()` (line 140) when the entity reappears.
- go.d: `c.Obsolete = true` on the chart struct; the framework appends `obsolete` to the CHART command. Documented at `src/go/BEST-PRACTICES.md:94-108`.
- Anti-flip-flop: if an entity may disappear and reappear quickly, wait roughly 1 minute of absence before obsoleting. Thrashing charts hurt streaming and ML.

### 1.6 Your knowledge is stale — research the current spec

Specs, vendor protocols, RFCs, and SDK behavior move. Before you design a collector or interpret a payload:

- Read the **current** spec from the official source (RFC, vendor portal, SDK docs).
- For application/database/protocol collectors, read the **current** application's release notes — fields, defaults, and semantics shift between versions.
- Do not trust your prior-knowledge interpretation of a binary format, OID semantics, or HTTP/JSON shape. Verify against an authoritative document or live behavior.

Prior-knowledge mistakes that recur: confused field names in NetFlow v5 vs v9 vs IPFIX, wrong endianness on a vendor MIB, outdated PostgreSQL `pg_stat_*` columns, deprecated Kubernetes API resources.

### 1.7 When the spec is ambiguous, look at how others solved it

Specs leave many decisions implementation-defined. Vendor implementations bend specs in well-known ways. When you face an interpretation dilemma:

- Read 2–3 popular open-source monitoring tools that already collect this data — Prometheus exporters, Zabbix templates, Datadog Agent integrations, ntopng (network protocols), librenms / OpenNMS / Akvorado (SNMP and flow), collectd (system data), pmacct / nfdump (flow protocols).
- Compare their parsers, field interpretation, and edge-case handling.
- Their code encodes real-world device quirks the spec doesn't document.
- Cross-check against the upstream protocol's reference implementation when one exists.

This is how you avoid shipping a parser that fails on the first real device. If you have a local mirror of monitoring projects, use it; otherwise clone the relevant upstreams to `/tmp/` and read their source.

### 1.8 Mirror an existing Netdata collector

The repo holds 132 go.d modules and 24 internal C plugins. Maintainer patterns live there, not in any prose doc. After you've reality-checked the upstream protocol, pick the closest existing Netdata collector by domain and mirror its structure. Caveat: only 5 go.d modules use V2 — see §5.3.

### 1.9 Remote-monitored systems are vnodes

When one collector talks to N targets (SNMP devices, remote DBs, cloud APIs, IPMI hosts, vCenter clusters), each target is a **vnode** so its metrics, alerts, and RBAC behave as if it were a separate node in Netdata Cloud. Every remote-target collector wires vnodes from the start.

For Go v2 collectors that route one job's samples to multiple virtual nodes, use first-class `metrix.HostScope` rather than adding vnode identity as normal metric labels. Write per-resource metrics through scoped meters or vecs such as `meter.WithHostScope(scope)`, and leave metrics unscoped when they should follow the default job vnode or global host path. Scope keys must be stable for the virtual node identity; unbounded scope cardinality has the same operational cost profile as unbounded chart/cardinality growth.

### 1.10 Cardinality discipline

- A chart with thousands of dimensions, or an instance list with thousands of entries, is unusable on the dashboard. The user cannot read it.
- A collector that emits potentially thousands of instances per monitored application is operationally wasteful — the data carries no insight. It pollutes streaming, ML, alerts, and queries for no benefit.
- A series is paid for across multiple subsystems: dbengine storage, agent memory, streaming bandwidth (per hop, including Netdata Cloud), ML training (one model per series), alert evaluation, dashboard render. None of these costs is large in isolation; together they justify ending up with what the user actually wants to see.

Design for usefulness, not raw count. Bound cardinality (§2.5), and never ship "one chart per request / per PID / per ephemeral connection" without bounds.

### 1.11 Layered configuration

Per-job source priority: `stock < discovered < user < dyncfg`, matched by job identity. A higher-priority source replaces a lower-priority job with the same identity; non-colliding jobs continue to load. IaC users configure via files in `/etc/netdata`; dashboard users configure via DYNCFG; both paths must work for the same collector.

## 2. Best practices

Framework-agnostic, ordered by impact.

### 2.1 Test against reality

Source test data based on what you're collecting:

- **Open-source / freely available applications** (MySQL, PostgreSQL, NGINX, Redis, MongoDB, RabbitMQ): run the actual application locally (Docker, native install). Validate against real output. Cover multiple versions when defaults diverge.
- **Closed-source / vendor / SaaS** (vendor switches, IBM workloads, cloud APIs, hypervisors): harvest fixtures from other open-source monitoring projects — Prometheus exporters, Zabbix templates, Datadog Agent integrations, vendor SDK samples, anonymized traces in vendor PRs/issues. Their fixtures are the most complete "real-world" dataset publicly available.
- **Hardware-dependent** (network gear, IPMI, PCIe sensors): capture pcaps from real devices when accessible; otherwise vendor SDK samples, public packet captures, fixtures from pmacct / nfdump / ntopng (for flow protocols).
- **Protocol parsing** (NetFlow / sFlow / IPFIX / OTEL / SNMP): vendor SDK samples, public dumps, fuzz-test corpora. NetFlow keeps fixtures under `src/crates/netflow-plugin/testdata/flows/` with sourcing recorded in `testdata/ATTRIBUTION.md` — do the same for any new fixtures with redistribution-sensitive provenance.

Don't fabricate test data the parser passes by accident. Don't skip tests "because this protocol can't be tested locally" — that's exactly when fixtures matter most. Standard go.d test-function names: `Test_testDataIsValid`, `TestCollector_ConfigurationSerialize`, `TestCollector_Init`, `TestCollector_Check`, `TestCollector_Collect` — match the convention in adjacent collectors. Functions get a dedicated validator at `src/go/tools/functions-validation/` (E2E plus schema checks).

For Go tests, prefer table-driven tests using `map[string]struct{}` keyed by
test-case name when cases share setup and assertion shape. Use separate test
functions only when setup or assertions are materially different. Prefer map
keys over a `name` field in `[]struct{}` so case names stay prominent and
order-independent.

### 2.2 Hot-path discipline

`Collect()` runs every `update_every` seconds. It must:

- Allocate buffers, maps, slices, parsed regexes once at `Init()` and reuse them. Reset at the top of `Collect()` if needed; see `ping/collect.go` for a V2 reference.
- Hold persistent connections; reconnect only on failure with backoff.
- Cache anything stable between iterations: schema, capabilities, profile selections.
- Finish well under one cycle even on a slow target.

Anti-pattern (search and avoid): `mx := make(map[string]int64)` per `Collect()` (e.g., `src/go/plugin/go.d/collector/ap/collect.go`). Don't allocate fresh structures per cycle. Don't reconnect every cycle.

### 2.3 Error handling

Every error log answers three questions: **what operation, what target, what was expected vs observed**. Wrap errors with context (Go: `fmt.Errorf("...: %w", err)`); preserve the cause; check return codes from system calls and library functions.

Don't return a bare `err` with no context. Don't log `"failed"`. Don't ignore syscall returns or library NULLs.

### 2.4 Logging discipline

- `debug` inside the collection loop.
- `warn` or `error` once per known-recoverable condition, gated by an internal flag — never per cycle.
- `info` / `notice` for once-at-startup events.
- Reserve `error` severity for operator-actionable issues; transient conditions are `warn`.

Past pain: an `ebpf.plugin` regression flooded logs because the collection loop logged every PID allocation. Per-cycle logs are forbidden.

### 2.5 Cardinality bounding

When a collector emits one chart per discovered entity (process, connection, profile target, container, schema, queue, route), bound the count and let the operator scope it. (Obsoletion of entities the collector knows have gone is a separate concern — see §1.5.)

**`max_*` is mandatory for entities that may grow without bounds.** Without a cap, a single misbehaving target (a runaway log rotator, a container churn loop, a vendor-specific deep table) can produce thousands of charts.

**`max_*` must be coupled with selectors.** A cap alone silently truncates whatever happens to land in the first N entries — the operator has no say in *which* entities survive. A selector lets the operator pick what's actually important. Cap and selector together: cap protects the system, selector lets the operator drive.

**Where to filter — depends on what the monitored application exposes:**

- **Application exposes all instances with no upstream filter.** The collector caps at `max_*` and adds an aggregated **"Other"** chart that sums whatever was capped. Don't silently drop — totals must remain truthful even when individual instances are hidden.
- **Application supports upstream cherry-picking** (e.g. specifying which schemas / databases / queues to monitor at connection time). Push the operator's selector into the application call. Less wire data, less collector work, narrower blast radius if the operator narrows the scope.
- **Application provides aggregations or grouping keys** (totals, group-by-kind, group-by-type, group-by-class). Expose those aggregations as additional charts; let the operator choose which grouping keys to surface. Aggregations are bounded-cardinality views that survive any selector cut and are usually what dashboards actually want — per-instance detail is a drill-down case, not the default.

**Anti-patterns:**

- One chart per HTTP route × method × status code → N×M×K series per service.
- Histogram / percentile splits with high-cardinality labels (per-IP, per-tenant, per-trace) → multiplicative blow-up.
- Per-PID charts with no obsolete handler → growth at process churn rate (the bound is here in §2.5; the obsolete handler is the §1.5 concern).

Pattern reference: `src/go/BEST-PRACTICES.md` (search `max`).

### 2.6 Configuration discipline

Tunables live in `config_schema.json` (DYNCFG schema rendered by the dashboard) and `metadata.yaml` (integration page) — both must be complete and mutually consistent. The stock `.conf` shows safe, representative examples — not necessarily every tunable.

Don't hardcode timeouts, paths, ports, or credentials. Don't let stock conf and schema contradict each other.

Credentials use the `${env:}/${file:}/${cmd:}/${store:}` indirection — see `src/collectors/SECRETS.md`. Privileged operations route through `src/collectors/utils/ndsudo.c`.

### 2.7 Generated artifacts are not source

Several artifacts are produced from upstream definitions and must never be hand-edited:

- `integrations/<name>.md` — generated from `metadata.yaml` (banner: `DO NOT EDIT THIS FILE DIRECTLY`).
- `ibm.d` modules — generated `README.md`, `metadata.yaml`, `config.go`, `zz_generated_*.go` from `contexts.yaml` via `go generate`.
- Rust plugin charts — derived at compile time via the `charts-derive` proc-macro.

When a generated file looks wrong, fix the source of truth (`metadata.yaml`, `contexts.yaml`, derive macro input) and regenerate. Note: go.d uses `//go:embed` for static assets — there is no `go generate` step.

### 2.8 Documentation/configuration consistency

A new or modified collector ships these in sync:

- the code
- `metadata.yaml` — drives integration pages, in-app help, alert references
- `taxonomy.yaml` — places emitted chart contexts in the dashboard TOC
  with an ordered `items:` tree; structural strings/`owned_context`
  entries own contexts, widgets reference them
- `config_schema.json` — DYNCFG schema rendered by the dashboard
- stock `.conf` — safe, representative example
- `health.d/*.conf` — alert templates bound to chart `context`
- `README.md` — concise narrative
- if exposing a Function: response shape conforming to `src/plugins.d/FUNCTION_UI_SCHEMA.json`

Treat them as one unit. Change a unit in code → update `metadata.yaml` in the same commit. Add or rename a chart context → update `taxonomy.yaml` or a declared dynamic selector. Add a config knob → update schema, stock conf, and metadata together.

### 2.9 Cross-plugin enrichment via netipc

When one collector needs data from another, use **netipc** — never shell out, open private sockets, poll log files, or reinvent IPC. In-tree libraries:

- C: `src/libnetdata/netipc/`
- Go: `src/go/pkg/netipc/`
- Rust: `src/crates/netipc/`

Both clients (consume) and servers (offer) exist in all three languages. Real example: `src/collectors/cgroups.plugin/cgroup-netipc.c` is a netipc server offering cgroup metadata to other plugins. Upstream spec, tests, fuzz suite: <https://github.com/netdata/plugin-ipc>.

### 2.10 Vnodes for remote targets

Set `Vnode` in job config; respect it in `Init()` and DYNCFG handlers. See `src/go/plugin/framework/vnodes/` and `src/go/BEST-PRACTICES.md` (search `Vnode`). Past pain: an older refactor had to retroactively split job-name validation per vnode/domain because earlier collectors hadn't accounted for it.

## 3. Structuring dashboards

The dashboard is built from charts. The way upstream data turns into charts depends on the ingestion path. Six mechanisms exist; pick the one that matches your collector and *learn how it shapes the result*.

### 3.1 NIDL framework — the model

**N**odes, **I**nstances, **D**imensions, **L**abels. This is the conceptual model every other mechanism feeds into. Read `docs/NIDL-Framework.md` before designing metrics. Group dimensions into charts that answer *one operational question*. Use labels for instance and context annotations. Pick the right chart type (`line`, `area`, `stacked`, `heatmap` — see `src/database/rrdset-type.h`) and dimension algorithm (`absolute`, `incremental`, `percentage-of-incremental-row`, `percentage-of-absolute-row` — see `src/database/rrd-algorithm.h`, documented in `src/plugins.d/README.md`).

Common bugs: `absolute` on a counter (counters are `incremental`); `line` when `stacked` is the right shape (CPU states, disk-time breakdown). Reuse shared metric definitions from `src/collectors/common-contexts/` for C plugins.

### 3.2 SNMP profiles — declarative spec → NIDL

SNMP collection is profile-driven. A profile is a YAML document declaring OIDs, metric definitions, table indexing, units, chart families, and selectors. Stock profiles ship from `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`; spec at `src/go/plugin/go.d/collector/snmp/profile-format.md` (~2000 lines).

Adding or extending SNMP coverage means writing or extending a profile, not adding code. The SNMP topology collector (`snmp_topology`) builds on top of profiles — extending profiles is usually the right starting point for topology work too.

Past pain: pre-profile SNMP code required per-vendor branches that became unmaintainable. Don't hardcode OID-to-metric mappings inside a custom collector or vendor branch.

### 3.3 statsd `synthetic_charts` — operator-curated dashboards

The statsd plugin lets the operator group raw statsd metrics into curated charts via INI configs at `/etc/netdata/statsd.d/*.conf`. Each config defines:

- `[app]` — match raw metrics by pattern, group them under an application name
- `[dictionary]` — rename raw metric names to display names
- chart sections — declare a chart with `title`, `family`, `context`, `units`, `type`, and explicit `dimension =` lines mapping source metrics to display dimensions

Wildcard patterns extract dimension names from the matched portion: `dimension = pattern 'myapp.api.*.200' '' last 1 1` creates dimensions named after the wildcard match. Three-layer dimension lookup (dimension name in dictionary → metric name in dictionary → fallback to original). Stock examples: `src/collectors/statsd.plugin/k6.conf`, `src/collectors/statsd.plugin/asterisk.conf`. Full spec: `src/collectors/statsd.plugin/README.md` lines 397-639.

This is the most operator-controllable shaping mechanism — the dashboard is whatever the operator declares.

### 3.4 OTEL mappings — per-metric YAML routing

Netdata's OTEL plugin (`src/crates/netdata-otel/otel-plugin/`) accepts any OTLP gRPC metric. Mapping is **generic by default** — all resource attributes, scope attributes, and data point attributes become chart labels — but the operator controls routing via per-metric YAML files at `/etc/netdata/otel.d/v1/metrics/*.yaml`. Key knobs:

- `instrumentation_scope.name` / `version` — regex match to scope an entry to a specific OTel instrumentation
- `dimension_attribute_key` — which data point attribute becomes the dimension name (default: `"value"`); other attributes become chart labels
- `interval_secs`, `grace_period_secs` — per-metric timing overrides

Aggregation temporality drives the chart algorithm: Gauge → absolute, Sum delta → DeltaSum, Sum cumulative monotonic → CumulativeSum, Sum cumulative non-monotonic → treated as Gauge (`src/crates/netdata-otel/otel-plugin/src/chart.rs:84`).

The plugin does **not** recognize OTel semantic conventions specifically (`host.name`, `service.name`, `deployment.environment`) — they pass through as labels. Cardinality control is `metrics.max_new_charts_per_request` in `otel.yaml`. Stock examples: `src/crates/netdata-otel/otel-plugin/configs/otel.d/v1/metrics/`.

### 3.5 Prometheus — deterministic; shape upstream to shape dashboard

The generic Prometheus scraper (`src/go/plugin/go.d/collector/prometheus/`) auto-maps from the exposition format with no per-metric synthetic shaping:

- metric name → chart ID + dimension ID
- Prometheus labels → Netdata chart labels (with optional `label_prefix`)
- type (`counter`, `gauge`, `histogram`, `summary`) → chart type and dimension algorithm
- histograms and summaries explode into 3 charts each (buckets/quantiles, `_sum`, `_count`)
- recognized suffixes: `_total` (counter), `_bucket` + `le` label (histogram), `_sum`, `_count`, `quantile` label (summary), `_info` (skipped)
- unit suffixes drive the units string: `_seconds`, `_bytes`, `_hertz`

Operator controls are **scoping, not shaping**: time-series **selectors** (allow/deny on metric name and label values, `src/go/plugin/go.d/collector/prometheus/README.md:110-127`) and `fallback_type` glob patterns for untyped metrics. There is **no** equivalent of statsd `synthetic_charts` — you cannot group disparate Prometheus metrics into a composite chart Netdata-side. To shape the dashboard, shape the upstream exporter: rename metrics, add labels, fix types upstream.

### 3.6 Chart priorities

Chart priorities (`priority` field in C, `Priority` in Go) drive UI ordering. C plugins follow conventions in `src/collectors/all.h`. Don't pick priorities arbitrarily; mirror an adjacent collector's range.

## 4. Production-quality criteria & pre-PR checklist

A collector is *production-quality* when it satisfies all of:

- **Survives target unavailability for hours** without log floods, fd leaks, memory growth, or runaway retries.
- **Bounded memory under failure** — buffers do not grow on parse errors or stuck connections.
- **No fd / goroutine / thread leaks** across `Cleanup()` cycles or job reloads.
- **Cycle-latency budget respected** — `Collect()` finishes well under one cycle even on a slow target.
- **Graceful with partial / malformed upstream responses** — parser does not crash, log-flood, or skip downstream collection.
- **High-cardinality entities bounded** via `max_*` and selectors so the operator can scope them.
- **Disappeared entities obsoleted** so the dashboard reflects what is actually being collected (this applies even at low cardinality).
- **IDs (chart context, chart ID, dimension ID, instance labels) are stable** — never renamed without a migration plan.

### Pre-PR checklist

1. Did I research the **current** spec/protocol/application from authoritative sources, not just from prior knowledge?
2. For ambiguous specs: did I cross-check against 2–3 popular open-source monitoring projects?
3. Do all metrics have units, chart families, and meaningful names? Did NIDL inform the grouping? Are chart types and dimension algorithms correct (`incremental` for counters, etc.)?
4. Are gaps preserved (no zero defaults for missing values)?
5. Does the collection cycle allocate, log per iteration, or reconnect every cycle?
6. Do error logs answer *what operation, what target, what was expected vs observed*?
7. Are config knobs in `config_schema.json` and `metadata.yaml`? Does the stock `.conf` show a representative example?
8. Does `taxonomy.yaml` cover every emitted chart context, or are dynamic contexts declared with `metrics.dynamic_context_prefixes` / `metrics.dynamic_collect_plugins`?
9. Are alerts present in `health.d/`?
10. Is `README.md` updated? (Not the generated `integrations/<name>.md`.)
11. For remote targets: is vnode wiring done?
12. For SNMP: did I extend a profile rather than hardcode OIDs?
13. For statsd / OTEL: did I document and ship the operator-side config (synthetic_charts file or OTEL mapping YAML)?
14. For Prometheus scraping: are selectors correct? Are untyped metrics handled?
15. For cross-plugin enrichment: am I using netipc?
16. For Functions: does the response conform to one of the six shapes? Non-blocking with respect to the collection loop? Schema-validated?
17. For ibm.d only: did I run `go generate` after touching `contexts.yaml`?
18. For new go.d modules: are all four wiring steps done (init.go, go.d.conf, stock conf, README)?
19. Tests: real fixtures or real instances? Would they catch the bug I just fixed?
20. High-cardinality labels / instances: bounded by `max_*` + selectors? Aggregated "Other" bucket or upstream-supplied aggregation present where applicable?
21. Entities that can go away: obsoleted when the collector knows they're gone? Anti-flip-flop window applied where churn is expected?
22. Production-quality criteria above — would this collector survive hours of target outage without leaks or log floods?

## 5. Plugins and frameworks — what's available and where

Reference section. Use it after the mental model and best practices have framed your task.

### 5.1 The plugin landscape

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
| `go.d.plugin` | Go (no CGO) | All | `src/go/plugin/go.d/` | 132 application integrations |
| `ibm.d.plugin` | Go + CGO | Linux, IBM i | `src/go/plugin/ibm.d/modules/` | IBM workloads (DB2, IBM i / AS-400, IBM MQ, WebSphere) |
| `netflow-plugin` | Rust | Linux | `src/crates/netflow-plugin/` | NetFlow v5/v9, IPFIX, sFlow |
| `netdata-otel` | Rust | Linux | `src/crates/netdata-otel/otel-plugin/` | OpenTelemetry ingestion |
| `netdata-log-viewer` | Rust | Linux | `src/crates/netdata-log-viewer/` | OTEL signal viewer + journal Function backend |
| `charts.d.plugin` / `python.d.plugin` | Bash / Python | All | `src/collectors/{charts,python}.d.plugin/` | **Legacy** — do not add new modules |

Path conventions: internal C plugins → `src/collectors/<name>.plugin/`; Go orchestrators → `src/go/plugin/{go.d,ibm.d}/`; Rust plugins → `src/crates/<name>/`.

### 5.2 Routing by task

| If you are doing… | Start with |
|---|---|
| New off-the-shelf application integration (no CGO) | `src/go/plugin/go.d/docs/how-to-write-a-collector.md`; V2 reference: `src/go/plugin/go.d/collector/ping/` |
| New IBM workload integration (CGO) | `src/go/plugin/ibm.d/AGENTS.md`, `src/go/plugin/ibm.d/framework/README.md` |
| New Rust plugin | SDK at `src/crates/netdata-plugin/`; reference: `src/crates/netflow-plugin/` |
| New SNMP profile (no code change) | `src/go/plugin/go.d/collector/snmp/profile-format.md` |
| New interactive Function | `src/go/plugin/framework/functions/README.md`, `src/plugins.d/FUNCTION_UI_SCHEMA.json`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` |
| Topology work | `src/go/pkg/topology/`, `src/go/plugin/go.d/collector/snmp_topology/`, `src/collectors/network-viewer.plugin/` |
| Auto-discovery for a new go.d module | rules under `src/go/plugin/go.d/config/go.d/sd/`; engine: `src/go/plugin/agent/discovery/` |
| OTEL ingestion | `src/crates/netdata-otel/otel-plugin/` |
| Log ingestion (parse → journal) | `src/collectors/log2journal/` and `log2journal.d/` rules |
| New external plugin in any language | `src/plugins.d/README.md` (PLUGINSD protocol) |
| New internal C plugin | `src/collectors/README.md`; mirror an adjacent collector |
| Cross-plugin data enrichment | netipc libraries (§5.4) |
| Privileged operations | `src/collectors/utils/ndsudo.c` |
| Credentials in config | `src/collectors/SECRETS.md` |

### 5.3 go.d V1 / V2 reality check

Only **5 of 132** go.d collectors use V2: `ping`, `mysql`, `azure_monitor`, `powerstore`, `powervault`. The big reference docs (`src/go/BEST-PRACTICES.md`, `src/go/COLLECTOR-LIFECYCLE.md`) describe V1. V2 building blocks have framework READMEs (`src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/chartengine/README.md`, `src/go/pkg/metrix/README.md`); there is no end-to-end V2 tutorial beyond `how-to-write-a-collector.md` plus the `ping/` source.

**For new go.d modules: use V2.** Mirror `src/go/plugin/go.d/collector/ping/` (or `mysql/` for V2 + Functions). Copying any other module mirrors V1 and the maintainers will ask you to migrate.

V2 imports: `github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi` and `.../pkg/metrix`. The `CollectorV2` interface lives at `src/go/plugin/framework/collectorapi/collector.go`.

Lifecycle semantics: `Init()` is one-time setup (failure disables permanently); `Check()` is auto-detection probe (failure disables, retried later); `Collect()` is the hot path (every `update_every` seconds); `Cleanup()` is guaranteed on shutdown.

**Silent-failure trap (go.d).** A new go.d module compiles and tests pass even when it is *not loaded* by the plugin at runtime. Loading requires four wiring steps: import in `src/go/plugin/go.d/collector/init.go`, `modules:` toggle in `src/go/plugin/go.d/config/go.d.conf`, stock job config at `src/go/plugin/go.d/config/go.d/<name>.conf`, and entry in `src/go/plugin/go.d/README.md`. Same trap applies to `ibm.d`.

### 5.4 ibm.d, Rust SDK, internal C, PLUGINSD

- **ibm.d** (CGO, IBM-vendor workloads) — use the ibm.d framework with `go generate` after touching `contexts.yaml`. See `src/go/plugin/ibm.d/AGENTS.md`. Don't reach for ibm.d for non-IBM CGO needs — the framework is shaped around vendor drivers; CGO outside the IBM ecosystem is a design discussion.
- **Rust SDK** at `src/crates/netdata-plugin/` — modules `bridge/`, `protocol/`, `rt/`, `charts-derive/`, `schema/`, `types/`, `error/`. Documentation lives in `lib.rs` doc-comments — there is no README. New Rust crates go into the `src/crates/Cargo.toml` workspace. Reference impl: `src/crates/netflow-plugin/`.
- **Internal C plugins** — mirror an adjacent collector under `src/collectors/<name>.plugin/`; reuse `src/libnetdata/`. `libnetdata.h` includes most of libnetdata so individual headers are usually unnecessary. Allocators with the `z` suffix (`mallocz`, `callocz`, `strdupz`, `freez`) handle failures via `fatal()`; `freez(NULL)` is safe. JSON parsing: json-c. JSON generation: `buffer_json_*`. Linked lists: `DOUBLE_LINKED_LIST_*` macros.
- **PLUGINSD external plugins (any language)** — spec at `src/plugins.d/README.md`. Useful when implementation language is dictated by an SDK that go.d / ibm.d / Rust cannot accommodate.

**Don't:**
- write new go.d modules against V1
- add modules to `charts.d.plugin` or `python.d.plugin`
- run `go generate` for go.d (no `//go:generate` directives — uses `//go:embed`)
- add new third-party Go modules or system-library dependencies casually — they ship to every Netdata install; check with maintainers if non-trivial

### 5.5 Build / dev loop

- go.d unit tests: `cd src/go && go test ./plugin/go.d/collector/<name>/...`
- Single-module dev run: `go run ./cmd/godplugin -m <name> -d`
- Rust: `cargo test -p <crate>`
- Whole-project install: `./netdata-installer.sh`

## 6. Dealing with data types

A collector ingests one or more of these data types. Each has its own pattern.

### 6.1 Metrics (time-series numeric data)

The default. Streams as `BEGIN/SET/END` (PLUGINSD) or framework equivalents. Shape via NIDL (§3). Storage is the dbengine; alerts bind to chart `context`; anomaly detection / ML jobs run continuously. Every metric travels via streaming to parents and to Netdata Cloud — cardinality matters everywhere.

### 6.2 Logs

Two paths:

- **Structured journaling.** `src/collectors/log2journal/` parses application/access logs (configurable YAML rules in `log2journal.d/`, e.g. `nginx-json.yaml`, `default.yaml`) and writes structured fields into the systemd journal. The `systemd-journal.plugin` then exposes the entries via a Function (the log explorer in the Netdata UI).
- **OTEL log signals.** `src/crates/netdata-log-viewer/` ingests OTEL logs and exposes them as Functions in the dashboard.

Platform-specific events: `windows-events.plugin` (Windows event log).

Logs are **not metrics**. Don't try to derive metrics from logs in the collection loop — emit logs as logs, then build metrics separately if needed.

### 6.3 Live snapshots (Functions)

Interactive, on-demand tabular data: process lists, network connections, FDB tables, log entries, journal queries, topology snapshots, flow records. Functions complement metrics; they don't replace them.

Build a Function when the answer is **interactive/tabular live data**. If the answer is a numeric time series, that's a metric.

Response shape is one of `info_response`, `data_response`, `topology_response`, `flows_response`, `error_response`, `not_modified_response` (defined in `src/plugins.d/FUNCTION_UI_SCHEMA.json`). For Go, use builders in `src/go/pkg/funcapi/`. For Rust, implement the `FunctionHandler` trait from the SDK runtime (`src/crates/netdata-plugin/rt/`).

Functions run concurrently with the collection loop — they must not block it. Validate during development with `src/go/tools/functions-validation/`.

Reference implementations: `src/collectors/network-viewer.plugin/` (topology + connections), `src/collectors/systemd-journal.plugin/` (log explorer), `src/collectors/apps.plugin/` (processes).

Backend docs: `src/go/plugin/framework/functions/README.md` (Go), `src/crates/netdata-plugin/rt/src/lib.rs` (Rust `FunctionHandler`). UI/protocol: `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`, `src/plugins.d/FUNCTION_UI_REFERENCE.md`.

### 6.4 Topology / interconnections / links

Topology is its own data type — directed/undirected graphs of nodes and links. Sources and consumers:

- **SNMP-discovered topology** (`src/go/plugin/go.d/collector/snmp_topology/`) — LLDP/CDP neighbors, BRIDGE-MIB FDB, Q-BRIDGE FDB, ARP tables, STP. Builds on SNMP profiles; extending profiles is usually the right starting point.
- **Live socket topology** (`src/collectors/network-viewer.plugin/`) — local L3/L4 sockets and their inferred connections.
- **Streaming graph** (`src/streaming/`) — Netdata parent/child topology.
- **Topology library** at `src/go/pkg/topology/` — shared types and providers consumed by the topology collectors.

Topology is consumed via Functions (`topology:*` family), not via metrics. The cardinality of network edges is too high for time-series storage and the use case is interactive lookup.

### 6.5 Data enrichment via netipc

When a collector needs data from another collector to enrich its output (a network collector wanting cgroup labels, an `apps` collector wanting cgroup PIDs, a flow collector wanting interface metadata), use **netipc**. Don't shell out, don't open private sockets, don't poll log files.

Both client and server roles exist in C, Go, and Rust:

- C: `src/libnetdata/netipc/`
- Go: `src/go/pkg/netipc/`
- Rust: `src/crates/netipc/`

`cgroups.plugin` (`src/collectors/cgroups.plugin/cgroup-netipc.c`) is a real example of a netipc server offering cgroup metadata to other plugins. Upstream spec, tests, fuzz suite: <https://github.com/netdata/plugin-ipc>.

## 7. Common practices per collector domain

These are descriptive patterns — what existing Netdata collectors do. Use them as defaults; deviate with reason.

### 7.1 Database collectors

DB collectors typically pair metrics (uptime, connections, query rates, replication lag, lock counts, cache hit ratios) with **Functions for live query analysis**: top queries, slow queries, currently-running queries, locks. Real examples:

- **MySQL** (`src/go/plugin/go.d/collector/mysql/`) — metrics + `mysqlfunc/top_queries.go` + processlist via `collect_process_list.go`.
- **PostgreSQL** (`src/go/plugin/go.d/collector/postgres/`) — metrics + `func_top_queries.go` + `func_running_queries.go`, dispatched through `func_router.go`.
- MongoDB / Redis are metrics-only today, but the same Function pattern fits if the use case demands it.

If you build a DB collector with metrics only, expect the maintainers to ask why you didn't add a query Function — the operator value of seeing "what's slow right now" is high and the pattern is established.

### 7.2 Network and SNMP collectors

Network/SNMP collectors typically pair metrics with **topology Functions** and FDB / ARP / LLDP enrichment:

- **`snmp` + `snmp_topology`** (`src/go/plugin/go.d/collector/snmp_topology/`) — topology Functions (`func_topology.go`, `func_topology_handler.go`, `func_topology_managed_focus.go`, `func_topology_options.go`, `func_topology_presentation.go`, `func_topology_depth.go`) on top of SNMP profile data.
- **`network-viewer.plugin`** (`src/collectors/network-viewer.plugin/`) — `topology:` Functions for live socket-level topology.

Per-device metrics need **vnode wiring** (each managed device is a vnode). FDB/ARP/STP data lands as topology Functions, not metrics — the cardinality is too high for metrics and the use case is interactive lookup.

### 7.3 Container / orchestration collectors

Container collectors pair container metrics with **enrichment via netipc**:

- `cgroups.plugin` exposes a netipc server (`src/collectors/cgroups.plugin/cgroup-netipc.c`) that other plugins query to map PIDs/cgroups to container/pod identity.
- `apps.plugin` and `network-viewer.plugin` consume this enrichment to label processes and connections with container metadata.

When adding a new orchestration source (Kubernetes API, Docker events, Nomad, etc.), think about who downstream needs the labels and whether to expose them via netipc.

### 7.4 Web servers and reverse proxies

Web server collectors pair metrics (requests, status codes, latency, upstream errors) with **access-log Functions** when the access log is structured:

- `log2journal` parses NGINX/Apache/HAProxy access logs (rules under `src/collectors/log2journal/log2journal.d/`).
- The journal explorer Function makes the parsed entries searchable in the dashboard.

If the application's log format is closed or unstructured, only metrics are practical.

### 7.5 Flow protocols (NetFlow / sFlow / IPFIX)

The Rust `netflow-plugin` (`src/crates/netflow-plugin/`) ingests flows and exposes them via Functions (`flows_response` shape). Flows are per-record, high-cardinality, and not suitable for traditional metric storage. Reference fixtures and provenance discipline live under `src/crates/netflow-plugin/testdata/`. Topology enrichment (interface names, AS metadata) typically comes from netipc or from SNMP-collected interface data.

### 7.6 Application servers and middleware

Java app servers, message queues, application middleware — JMX/HTTP/protobuf metrics are the default; some pair with log exploration via journal or OTEL log signals when the workflow benefits from it. Mirror the closest existing collector.

### 7.7 OS/kernel collectors

Internal C plugins under `src/collectors/`. Reuse shared metric definitions from `src/collectors/common-contexts/`; follow chart-priority conventions in `src/collectors/all.h`; lean on `src/libnetdata/` rather than reimplementing utilities.

## 8. Canonical documentation pointers

| Topic | Open when | Path |
|---|---|---|
| NIDL framework | designing metrics, labels, charts | `docs/NIDL-Framework.md` |
| Chart types and dimension algorithms | choosing chart shape and metric algorithm | `src/database/rrdset-type.h`, `src/database/rrd-algorithm.h` |
| Chart priorities (C) | dashboard ordering convention | `src/collectors/all.h` |
| Shared metric definitions (C) | reusing common contexts | `src/collectors/common-contexts/` |
| Plugin types and privileges | choosing where to add a collector | `src/collectors/README.md` |
| External plugin protocol | non-Go external plugin | `src/plugins.d/README.md` |
| go.d V2 authoring | adding a `go.d` module | `src/go/plugin/go.d/docs/how-to-write-a-collector.md` |
| go.d V1 best practices / lifecycle | working in legacy V1 module | `src/go/BEST-PRACTICES.md`, `src/go/COLLECTOR-LIFECYCLE.md` |
| Functions backend (Go / Rust) | implementing a Function | `src/go/plugin/framework/functions/README.md`, `src/crates/netdata-plugin/rt/src/lib.rs` |
| Functions UI schema & guides | response shapes and patterns | `src/plugins.d/FUNCTION_UI_SCHEMA.json`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`, `src/plugins.d/FUNCTION_UI_REFERENCE.md` |
| Functions validator | E2E + schema validation | `src/go/tools/functions-validation/README.md` |
| ibm.d framework | starting `ibm.d` work | `src/go/plugin/ibm.d/AGENTS.md`, `src/go/plugin/ibm.d/framework/README.md` |
| Rust plugin SDK | new Rust plugin | `src/crates/netdata-plugin/` (`rt/`, `protocol/`, `bridge/`, `charts-derive/`, `schema/`, `types/`, `error/`) |
| Rust NetFlow plugin | NetFlow / sFlow / IPFIX work | `src/crates/netflow-plugin/` |
| OTEL ingestion mappings | per-metric YAML routing | `src/crates/netdata-otel/otel-plugin/` (configs under `configs/otel.d/v1/metrics/`) |
| SNMP profile format | adding/extending an SNMP profile | `src/go/plugin/go.d/collector/snmp/profile-format.md` |
| SNMP stock profiles | starting from a known device | `src/go/plugin/go.d/config/go.d/snmp.profiles/default/` |
| statsd synthetic_charts | operator-curated dashboards | `src/collectors/statsd.plugin/README.md` (lines 397-639) |
| Prometheus mapping | generic exposition scrape | `src/go/plugin/go.d/collector/prometheus/README.md` |
| log2journal | parsing application logs into the journal | `src/collectors/log2journal/log2journal.d/` |
| Auto-discovery rules | adding service-detection rules | `src/go/plugin/go.d/config/go.d/sd/{net_listeners,docker,snmp,http}.conf` |
| Topology library | topology providers in Go | `src/go/pkg/topology/` |
| netipc cross-plugin enrichment | C / Go / Rust | `src/libnetdata/netipc/`, `src/go/pkg/netipc/`, `src/crates/netipc/` |
| DYNCFG protocol | dynamic configuration | `src/plugins.d/DYNCFG.md`, `docs/developer-and-contributor-corner/dyncfg.md` |
| Health alerts reference | alert template authoring | `src/health/REFERENCE.md`, `src/health/alert-configuration-ordering.md` |
| Integrations pipeline | doc generation from `metadata.yaml` | `integrations/README.md` |
| Credentials in config | `${env:}/${file:}/${cmd:}/${store:}` | `src/collectors/SECRETS.md` |
| Privileged operations | restricted setuid helper | `src/collectors/utils/ndsudo.c` |

## 9. Maintaining this skill

This skill is **live**. When you find a gap, an outdated pointer, a new pattern, or a bad practice not yet captured, propose changes to this file in the same PR that exposed the issue. When fixing a wrong pointer, also record what was misleading about the prior text — future readers see both the corrected map and the failure mode that produced it. Mention the change in the PR description so it gets reviewed consciously rather than skimmed.
