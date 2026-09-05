---
name: collectors-authoring
description: Entry point and routing for authoring or modifying any Netdata data-collection plugin or module (internal C plugins, go.d and ibm.d Go modules, Rust plugins, external PLUGINSD plugins). Read before adding a collector, changing one, or working on logs, topology, NetFlow/sFlow/IPFIX, OTEL ingestion, SNMP profiles, statsd, Prometheus scraping, or interactive Functions. Covers the mental model, universal practices, the production quality bar, routing by task, and canonical pointers; dashboard-shaping mechanisms and the plugin landscape live in its reference files.
---

# Writing Netdata data collection plugins and modules

## What this skill is

You are about to add or modify data collection in the Netdata Agent. This skill is the entry point and routing map:
the mindset to apply, the principles you cannot violate, the quality bar that separates a draft from a shippable
collector, and where the depth lives. It is not a tutorial; the deep references already exist in the repo. Your job is
to know they exist, pick the right one, and produce work that blends with the patterns the maintainers already accept.

Reading order: AI fast path -> mental model -> best practices -> quality bar -> routing -> pointers. Two reference files
hold the detail that only some tasks need: `dashboard-shaping.md` (how SNMP profiles, statsd `synthetic_charts`, OTEL
mappings, and Prometheus profiles turn upstream data into charts) and `landscape-and-domains.md` (the plugin table,
build/dev loop, data types, per-domain practices).

## AI Fast Path

For implementation agents, route to the concrete workflow first and use the
rest of this skill as background:

- New go.d collector, or a public-contract change to one (config option, mode,
  metric meaning, ownership of state, Functions, vnodes): read `src/go/AGENTS.md`,
  then `.agents/skills/collectors-go-design/SKILL.md` and fill its design
  note in the SOW gate, then `src/go/plugin/go.d/docs/how-to-write-a-collector.md`,
  `.agents/skills/collectors-go-framework-v2/SKILL.md`, and
  `.agents/skills/integrations-lifecycle/recipes/add-go-collector.md`.
- Existing go.d collector update: read `src/go/AGENTS.md`, the collector's
  local files, `.agents/skills/integrations-lifecycle/consistency.md`, and
  `.agents/skills/integrations-lifecycle/recipes/update-collector.md`.
- Writing or reviewing any collector's `metadata.yaml` (the integration
  page): `.agents/skills/collectors-metadata-yaml/SKILL.md`, field by
  field.
- V1-to-V2 migration: read `src/go/AGENTS.md`,
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`, and the V2 skill before
  changing code.
- Framework or shared-helper change: stop and satisfy
  `src/go/plugin/framework/docs/changing-framework-code.md` before writing
  code.
- Prometheus chart-profile authoring or review: read
  `.agents/skills/collectors-prometheus-profiles/SKILL.md` and every source it marks as
  mandatory, then run the repository real-pipeline validator it documents.

Do not use this broad skill as the only implementation guide for go.d work.

## 1. Mental model

How to think about Netdata data collection. Internalize this before designing anything.

### 1.1 Frequent collection at scale

The Agent ships on >1.5M new daily installs across physical servers, VMs, containers, IoT devices, embedded systems, and
exotic Unixes. Default collection is 1-second; many collectors raise it (`ping` 5s, SNMP 10s) when the source warrants
it. Anything you do inside the collection cycle — allocate, log, reconnect, retry, parse, format — is multiplied by that
population. Hot-path discipline is the entry ticket, not an optimization.

### 1.2 Metric structure is dashboard UX

How dimensions group into charts and how labels attach to instances *is* the dashboard the user sees. Mirroring upstream
data structures one-to-one produces a chart per metric, which is unusable. **NIDL** — Nodes, Instances, Dimensions,
Labels — is the model. Every dashboard-shaping mechanism (§3) feeds into it.

### 1.3 IDs are public contracts

Chart `context`, chart IDs, dimension IDs, instance labels — once shipped, they bind health alerts, dashboards, exports,
anomaly detection, ML jobs, streaming consumers, and Netdata Cloud. Renaming silently breaks all of them. Treat them as
permanent.

### 1.4 Gaps are data

When you cannot measure a value this iteration, emit nothing for that dimension. The dashboard renders the gap; the user
knows collection is broken. Defaulting to `0` fabricates a working state and hides the bug. Past pain in
`src/collectors/proc.plugin/proc_net_dev.c` (search `shouldn't use 0 value, but NULL`).

### 1.5 Obsolete what's gone

When the collector knows an entity has gone away — a process exited, a container was removed, a profile target was
dropped, a network interface disappeared, a managed device went offline — mark its chart obsolete. The dashboard then
renders it as historical, not as actively collected; alerts stop binding to it; streaming and ML stop costing for it.

This is a truthfulness principle, not a cardinality one. It applies at any cardinality, including a single instance.
Without obsoletion, the chart looks alive on the dashboard, alerts may continue evaluating against frozen data, and the
user is misled about what is and isn't being collected.

Mechanics:
- C: `rrdset_is_obsolete___safe_from_collector_thread()` in `src/database/rrdset.c:116` flags `RRDSET_FLAG_OBSOLETE`.
  Reverse with `rrdset_isnot_obsolete___safe_from_collector_thread()` (line 155) when the entity reappears.
- go.d V1: `c.Obsolete = true` or `MarkRemove()` on the chart marks it obsolete. go.d V2: chart lifetime is controlled
  by `charts.yaml` lifecycle policy and `chartengine`; start from `src/go/plugin/go.d/docs/how-to-write-a-collector.md`
  for new collectors and `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` for migrations.
- Anti-flip-flop: if an entity may disappear and reappear quickly, wait roughly 1 minute of absence before obsoleting.
  Thrashing charts hurt streaming and ML.

### 1.6 Your knowledge is stale — research the current spec

Specs, vendor protocols, RFCs, and SDK behavior move. Before you design a collector or interpret a payload:

- Read the **current** spec from the official source (RFC, vendor portal, SDK docs).
- For application/database/protocol collectors, read the **current** application's release notes — fields, defaults, and
  semantics shift between versions.
- Do not trust your prior-knowledge interpretation of a binary format, OID semantics, or HTTP/JSON shape. Verify against
  an authoritative document or live behavior.

Prior-knowledge mistakes that recur: confused field names in NetFlow v5 vs v9 vs IPFIX, wrong endianness on a vendor
MIB, outdated PostgreSQL `pg_stat_*` columns, deprecated Kubernetes API resources.

### 1.7 When the spec is ambiguous, look at how others solved it

Specs leave many decisions implementation-defined. Vendor implementations bend specs in well-known ways. When you face
an interpretation dilemma:

- Read 2–3 popular open-source monitoring tools that already collect this data — Prometheus exporters, Zabbix templates,
  Datadog Agent integrations, ntopng (network protocols), librenms / OpenNMS / Akvorado (SNMP and flow), collectd
  (system data), pmacct / nfdump (flow protocols).
- Compare their parsers, field interpretation, and edge-case handling.
- Their code encodes real-world device quirks the spec doesn't document.
- Cross-check against the upstream protocol's reference implementation when one exists.

This is how you avoid shipping a parser that fails on the first real device. If you have a local mirror of monitoring
projects, use it; otherwise clone the relevant upstreams to `/tmp/` and read their source.

### 1.8 Mirror an existing Netdata collector

The repo holds many go.d modules and internal C plugins. Maintainer patterns
live there, not in any prose doc. After you've reality-checked the upstream
protocol, pick the closest existing Netdata collector by domain and mirror its
structure. New go.d modules MUST use framework V2 and start from the current V2 authoring guide — see §5.2.

### 1.9 Remote-monitored systems and vnodes

When a collector talks to a remote target (an SNMP device, a remote database, a cloud API, an IPMI host, a vCenter), the
operator MAY assign the job to a vnode so its metrics, alerts, and RBAC behave as a separate node in Netdata Cloud; the
`vnode` job option exists for that. Whether one job should generate N virtual nodes itself (one per discovered
resource) is a product decision, not an automatic consequence of having N targets: it multiplies nodes, alerts, and
Cloud cost, and it needs stable identity per node. For Go V2 collectors the mechanism is `metrix.HostScope`; the
decision and its bounds are owned by `.agents/skills/collectors-go-framework-v2/go-v2-host-scope.md`, and
`.agents/skills/collectors-go-design/SKILL.md` records it in the design note.

### 1.10 Cardinality discipline

- A chart with thousands of dimensions, or an instance list with thousands of entries, is unusable on the dashboard. The
  user cannot read it.
- A collector that emits potentially thousands of instances per monitored application is operationally wasteful — the
  data carries no insight. It pollutes streaming, ML, alerts, and queries for no benefit.
- A series is paid for across multiple subsystems: dbengine storage, agent memory, streaming bandwidth (per hop,
  including Netdata Cloud), ML training (one model per series), alert evaluation, dashboard render. None of these costs
  is large in isolation; together they justify ending up with what the user actually wants to see.

Design for usefulness, not raw count. Bound cardinality by design (§2.5), and never ship "one chart per request / per
PID / per ephemeral connection" without a bound.

### 1.11 Layered configuration

Per-job source priority: `stock < discovered < user < dyncfg`, matched by job identity. A higher-priority source
replaces a lower-priority job with the same identity; non-colliding jobs continue to load. IaC users configure via files
in `/etc/netdata`; dashboard users configure via DYNCFG; both paths must work for the same collector.

## 2. Best practices

Framework-agnostic, ordered by impact. The mandatory clean-end-state and scope-discipline rules are in the root
`AGENTS.md`; they apply here without restatement.

### 2.1 Test against reality

Source test data based on what you're collecting:

- **Open-source / freely available applications** (MySQL, PostgreSQL, NGINX, Redis, MongoDB, RabbitMQ): run the actual
  application locally (Docker, native install). Validate against real output. Cover multiple versions when defaults
  diverge.
- **Closed-source / vendor / SaaS** (vendor switches, IBM workloads, cloud APIs, hypervisors): harvest fixtures from
  other open-source monitoring projects — Prometheus exporters, Zabbix templates, Datadog Agent integrations, vendor SDK
  samples, anonymized traces in vendor PRs/issues. Their fixtures are the most complete "real-world" dataset publicly
  available.
- **Hardware-dependent** (network gear, IPMI, PCIe sensors): capture pcaps from real devices when accessible; otherwise
  vendor SDK samples, public packet captures, fixtures from pmacct / nfdump / ntopng (for flow protocols).
- **Protocol parsing** (NetFlow / sFlow / IPFIX / OTEL / SNMP): vendor SDK samples, public dumps, fuzz-test corpora.
  NetFlow keeps fixtures under `src/crates/netflow-plugin/testdata/flows/` with sourcing recorded in
  `testdata/ATTRIBUTION.md` — do the same for any new fixtures with redistribution-sensitive provenance.

Don't fabricate test data the parser passes by accident. Don't skip tests "because this protocol can't be tested
locally" — that's exactly when fixtures matter most. Standard go.d test-function names: `Test_testDataIsValid`,
`TestCollector_ConfigurationSerialize`, `TestCollector_Init`, `TestCollector_Check`, `TestCollector_Collect` — match the
convention in adjacent collectors. Functions get a dedicated validator at `src/go/tools/functions-validation/` (E2E plus
schema checks). Go test shape (table-driven, `map[string]struct{}`) is the root `AGENTS.md` "Go Test Style" rule.

### 2.2 Hot-path discipline

`Collect()` runs every `update_every` seconds, multiplied by the install base (§1.1). It MUST:

- Create long-lived resources once at `Init()` / `New()` and reuse them: clients and connections, parsed regexes and
  matchers, metric instruments, and any buffer that is large or rebuilt identically every cycle. See
  `cato_networks/metrix.go` for the typed V2 metric-instrument pattern. A small bounded temporary that holds this
  cycle's results in a network-bound collector is not a defect; do not add pooling or retained state to satisfy a
  slogan. Allocation discipline for framework and per-sample code is `src/go/AGENTS.md` "Hot-Path And Benchmark
  Discipline".
- Hold persistent connections; reconnect only on failure, with backoff.
- Reuse what is stable between iterations (schema, capabilities, parsed profile selections, instrument handles) only
  when staleness is safe. Cache scope and its evidence are design decisions, not a default; for values that authorize a
  dangerous operation, `.agents/skills/collectors-go-design/mutating-collectors.md` §5 owns the rule.
- Bound its work per call: a per-request timeout, honored context cancellation, and bounded fan-out. The scheduling
  interval is not a completion guarantee; if the collector promises a whole-cycle deadline, that promise is an
  explicit design decision with its own test.

Anti-pattern (search and avoid): `mx := make(map[string]int64)` per `Collect()` (e.g.,
`src/go/plugin/go.d/collector/ap/collect.go`). Don't rebuild the whole metric surface, parsers, or clients every cycle.
Don't reconnect every cycle.

### 2.3 Error handling

Every error log answers three questions: **what operation, what target, what was expected vs observed**. Wrap errors
with context (Go: `fmt.Errorf("...: %w", err)`); preserve the cause; check return codes from system calls and library
functions.

Don't return a bare `err` with no context. Don't log `"failed"`. Don't ignore syscall returns or library NULLs.

### 2.4 Logging discipline

- `debug` inside the collection loop.
- `warn` or `error` once per known-recoverable condition, gated by an internal flag — never per cycle.
- `info` / `notice` for once-at-startup events.
- Reserve `error` severity for operator-actionable issues; transient conditions are `warn`.

Past pain: an `ebpf.plugin` regression flooded logs because the collection loop logged every PID allocation. Per-cycle
logs are forbidden.

### 2.5 Cardinality bounding

When a collector emits one chart per discovered entity (process, connection, profile target, container, schema, queue,
route), the cardinality MUST be bounded by design. (Obsoletion of entities the collector knows have gone is a separate
concern; see §1.5.) The bound is a per-domain decision, recorded in the design note, among these mechanisms:

- **Upstream cherry-picking.** When the application can be told which schemas, databases, or queues to expose, push
  the operator's selector into the application call: less wire data, less collector work, narrower blast radius.
- **Upstream aggregations or grouping keys.** When the application provides totals or group-by views, expose those
  as charts and let the operator choose which grouping keys to surface. Aggregations are bounded views that survive
  any selector cut and are usually what dashboards want; per-instance detail is a drill-down, not the default.
- **A cap with an aggregated "Other" bucket.** When the application exposes all instances with no upstream filter and
  the entity set can grow without bound, cap the count. When the observation is meaningfully mergeable (counts and
  additive gauges, using the reducer the V2 skill's aggregation rules allow), sum what was capped into an "Other" chart
  so totals stay truthful; for percentiles, temperatures, timestamps, or states, do not invent an aggregate: disclose
  the excluded coverage explicitly (for example a charted-versus-reported count). A cap alone silently truncates
  whatever lands in the first N entries, so pair it with a selector that lets the operator choose which entities
  survive.

A public `max_*` option or selector is a config option like any other: it MUST name the operator decision it enables
(`.agents/skills/collectors-go-design/operator-surface.md`). A bounded, low-cardinality entity set needs no knob.

Anti-patterns:

- One chart per HTTP route x method x status code: N x M x K series per service.
- Histogram / percentile splits with high-cardinality labels (per-IP, per-tenant, per-trace): multiplicative blow-up.
- Per-PID charts with no obsolete handler: growth at process churn rate (the bound is here; the obsolete handler is
  §1.5).

### 2.6 Configuration discipline

Public tunables are part of the collector consistency contract. When a config option is added, removed, renamed, or
given a new default, you MUST follow `.agents/skills/integrations-lifecycle/consistency.md`; you MUST NOT update only
the Go struct or only the docs. The stock `.conf` shows safe, representative examples, not necessarily every tunable.

Configuration holds operator decisions (connection identity, endpoints, credentials, the target request timeout,
cardinality selectors); internal policy (retries, paging, cadence, caches, fan-out) stays a constant unless a recorded
decision names the operator choice it enables. This applies to every collector family; the go.d item list is the how-to
guide's Config section (`src/go/plugin/go.d/docs/how-to-write-a-collector.md`). Stock config and schema MUST NOT
contradict each other.

For go.d collectors, the config decision record, option lifecycle and compatibility, the DynCfg form as a user task,
constructor defaults versus conditional branches, and which schema tests carry weight are owned by
`.agents/skills/collectors-go-design/operator-surface.md`; writing `config_schema.json` itself (text channels,
tabs, widgets, secrets, standard option wording, the repo-wide rule tests) is owned by its sibling `config-schema.md`.

Credentials use the `${env:}/${file:}/${cmd:}/${store:}` indirection; see `src/collectors/SECRETS.md`. Privileged
operations route through `src/collectors/utils/ndsudo.c`.

### 2.7 Generated artifacts are not source

Several artifacts are produced from upstream definitions and MUST NOT be hand-edited:

- `integrations/<name>.md` — generated from `metadata.yaml` (banner: `DO NOT EDIT THIS FILE DIRECTLY`).
- `ibm.d` modules — generated `README.md`, `metadata.yaml`, `config.go`, `zz_generated_*.go` from `contexts.yaml` via
  `go generate`.
- Rust plugin charts — derived at compile time via the `charts-derive` proc-macro.

When a generated file looks wrong, fix the source of truth (`metadata.yaml`, `contexts.yaml`, derive macro input) and
regenerate. Note: go.d uses `//go:embed` for static assets — there is no `go generate` step.

### 2.8 Documentation/configuration consistency

Collector consistency has one detailed checklist:
`.agents/skills/integrations-lifecycle/consistency.md`. Treat code,
integration metadata, taxonomy, config, stock examples, alerts, and generated
documentation as one unit, but do not maintain a second artifact matrix here.

If a collector exposes a Function, its response shape MUST also conform to the
relevant Function schema, such as `src/plugins.d/FUNCTION_UI_SCHEMA.json` or
`src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.

### 2.9 Cross-plugin enrichment via netipc

When one collector needs data from another, use **netipc** — never shell out, open private sockets, poll log files, or
reinvent IPC. In-tree libraries:

- C: `src/libnetdata/netipc/`
- Go: `src/go/pkg/netipc/`
- Rust: `src/crates/netipc/`

Both clients (consume) and servers (offer) exist in all three languages. Real example:
`src/collectors/cgroups.plugin/cgroup-netipc.c` is a netipc server offering cgroup metadata to other plugins. Upstream
spec, tests, fuzz suite: <https://github.com/netdata/plugin-ipc>.

## 3. Structuring dashboards

The dashboard is built from charts. The way upstream data turns into charts depends on the ingestion path. Pick the
mechanism that matches your collector and *learn how it shapes the result*.

### 3.1 NIDL framework — the model

**N**odes, **I**nstances, **D**imensions, **L**abels. This is the conceptual model every other mechanism feeds into.
Read `docs/NIDL-Framework.md` before designing metrics. Group dimensions into charts that answer *one operational
question*. Use labels for instance and context annotations. Pick the right chart type (`line`, `area`, `stacked`,
`heatmap` — see `src/database/rrdset-type.h`) and dimension algorithm (`absolute`, `incremental`,
`percentage-of-incremental-row`, `percentage-of-absolute-row` — see `src/database/rrd-algorithm.h`, documented in
`src/plugins.d/README.md`).

Common bugs: `absolute` on a counter (counters are `incremental`); `line` when `stacked` is the right shape (CPU states,
disk-time breakdown). Reuse shared metric definitions from `src/collectors/common-contexts/` for C plugins.

### 3.2 Mechanisms per ingestion path

SNMP profiles, statsd `synthetic_charts`, OTEL per-metric mappings, and Prometheus selectors, relabeling, and chart
profiles each shape charts differently. Read the matching section of `dashboard-shaping.md` before designing for one of
them; for SNMP, extend a profile rather than hardcode OIDs, and for Prometheus profiles load
`.agents/skills/collectors-prometheus-profiles/SKILL.md`.

### 3.3 Chart priorities

Chart priorities (`priority` field in C, `Priority` in Go) drive UI ordering. C plugins follow conventions in
`src/collectors/all.h`. Don't pick priorities arbitrarily; mirror an adjacent collector's range.

## 4. Production-quality criteria & pre-PR checklist

A collector is *production-quality* when it satisfies all of:

- **Survives target unavailability for hours** without log floods, fd leaks, memory growth, or runaway retries.
- **Bounded memory under failure** — buffers do not grow on parse errors or stuck connections.
- **No fd / goroutine / thread leaks** across `Cleanup()` cycles or job reloads.
- **Bounded work per call** — per-request timeouts and context cancellation are honored; a slow target cannot pin the
  collector, and any whole-cycle deadline the collector promises is explicit and tested.
- **Graceful with partial / malformed upstream responses** — parser does not crash, log-flood, or skip downstream
  collection.
- **High-cardinality entities bounded by design** (upstream selection, upstream aggregation, or cap plus "Other", per
  §2.5).
- **Disappeared entities obsoleted** so the dashboard reflects what is actually being collected (this applies even at
  low cardinality).
- **IDs (chart context, chart ID, dimension ID, instance labels) are stable** — never renamed without a migration plan.

### Pre-PR checklist

1. Did I research the **current** spec/protocol/application from authoritative sources, not just from prior knowledge?
2. For ambiguous specs: did I cross-check against 2–3 popular open-source monitoring projects?
3. Do all metrics have units, chart families, and meaningful names? Did NIDL inform the grouping? Are chart types and
   dimension algorithms correct (`incremental` for counters, etc.)?
4. Are gaps preserved (no zero defaults for missing values)?
5. Does the collection cycle allocate, log per iteration, or reconnect every cycle? Is every value that authorizes a
    dangerous operation re-checked at that operation?
6. Do error logs answer *what operation, what target, what was expected vs observed*?
7. Did I run the collector consistency checklist in `.agents/skills/integrations-lifecycle/consistency.md`, including
   the rule that generated integration pages are not hand-authored sources?
8. For remote targets: is the vnode decision recorded (job-level `vnode` option, or a product decision for generated
    nodes with bounded, stable identity)?
9. For SNMP: did I extend a profile rather than hardcode OIDs?
10. For statsd / OTEL: did I document and ship the operator-side config (synthetic_charts file or OTEL mapping YAML)?
11. For Prometheus scraping: are selectors and job relabeling correct? Is exporter-required normalization owned by the
    profile instead of duplicated in job examples? Are untyped metrics handled before profile normalization? Should
    the exporter get a stock chart profile (`profile-format.md`), and should that profile suppress unmatched fallback
    charts with a scoped `autogen.selector` while retaining their samples?
12. For cross-plugin enrichment: am I using netipc?
13. For Functions: does the response conform to one of the six shapes? Non-blocking with respect to the collection loop?
    Schema-validated?
14. For ibm.d only: did I run `go generate` after touching `contexts.yaml`?
15. For new go.d modules: are all four runtime-load wiring steps done (`collector/init.go` import, `go.d.conf`, stock
    conf, README)?
16. Tests: real fixtures or real instances? Would they catch the bug I just fixed?
17. High-cardinality labels / instances: which bounding mechanism from §2.5 applies, and does any public knob name an
    operator decision?
18. Entities that can go away: obsoleted when the collector knows they're gone? Anti-flip-flop window applied where
    churn is expected?
19. Production-quality criteria above — would this collector survive hours of target outage without leaks or log floods?

## 5. Plugins and frameworks: routing

The plugin table, the ibm.d / Rust / C / PLUGINSD notes, and the build/dev loop are in `landscape-and-domains.md`.

### 5.1 Routing by task

| If you are doing… | Start with |
|---|---|
| New off-the-shelf application integration (no CGO) | `.agents/skills/collectors-go-design/SKILL.md` (design note first), then `src/go/plugin/go.d/docs/how-to-write-a-collector.md`; primary V2 reference: `src/go/plugin/go.d/collector/cato_networks/` |
| Config option, mode, metric-meaning, or ownership change to a go.d collector | `.agents/skills/collectors-go-design/SKILL.md` (the affected note item) plus the V2 skill and `consistency.md` |
| Migrating existing go.d collector to V2 | `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`; V2 mechanics: `.agents/skills/collectors-go-framework-v2/SKILL.md` |
| New IBM workload integration (CGO) | `src/go/plugin/ibm.d/AGENTS.md`, `src/go/plugin/ibm.d/framework/README.md` |
| New Rust plugin | SDK at `src/crates/netdata-plugin/`; reference: `src/crates/netflow-plugin/` |
| New SNMP profile (no code change) | `.agents/skills/collectors-snmp-profiles/SKILL.md`; format spec: `src/go/plugin/go.d/collector/snmp/profile-format.md` |
| New or changed SNMP trap profile | `.agents/skills/collectors-snmp-trap-profiles/SKILL.md` |
| New interactive Function | `src/go/plugin/framework/functions/README.md`, `src/plugins.d/FUNCTION_UI_SCHEMA.json`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` |
| Topology work | `.agents/skills/topology-authoring/SKILL.md`, `src/go/pkg/topology/v1`, `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` |
| Auto-discovery for a new go.d module | rules under `src/go/plugin/go.d/config/go.d/sd/`; engine: `src/go/plugin/agent/discovery/` |
| OTEL ingestion | `src/crates/otel-plugin/` (ingest logic in `src/crates/otel-ingestor/`) |
| Log ingestion (parse → journal) | `src/collectors/log2journal/` and `log2journal.d/` rules |
| New external plugin in any language | `src/plugins.d/README.md` (PLUGINSD protocol) |
| New internal C plugin | `src/collectors/README.md`; mirror an adjacent collector |
| Cross-plugin data enrichment | netipc libraries (§2.9) |
| Privileged operations | `src/collectors/utils/ndsudo.c` |
| Credentials in config | `src/collectors/SECRETS.md` |

### 5.2 go.d V1 / V2 reality check

Most go.d collectors are still V1, but the broad V1 authoring docs have been
retired because they taught stale patterns from general Go paths. Do not use
existing V1 collectors as the shape for new work.

**New go.d modules MUST use V2.** Start with
`src/go/plugin/go.d/docs/how-to-write-a-collector.md`. Use
`src/go/plugin/go.d/collector/cato_networks/` as the primary modern reference,
but copy focused responsibilities rather than the entire collector. Copying a V1
module mirrors legacy patterns and the maintainers will ask you to migrate.

For migrating an existing V1 collector, start with
`src/go/plugin/go.d/docs/migrate-v1-to-v2.md`. Migration is compatibility work;
do not use the new-collector guide to justify chart, config, or lifecycle
contract changes. Temporary V1 parity bridges can help during development, but
the finished collector MUST NOT run through a V1-to-V2 bridge.

V2 imports: `github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi` and `.../pkg/metrix`. The
`CollectorV2` interface lives at `src/go/plugin/framework/collectorapi/collector.go`.

Lifecycle semantics: `Init()` is one-time setup (failure disables permanently); `Check()` is auto-detection probe
(failure disables, retried later); `Collect()` is the hot path (every `update_every` seconds); `Cleanup()` is guaranteed
on shutdown.

**Silent-failure trap (go.d).** A new go.d module compiles and tests pass even when it is *not loaded* by the plugin at
runtime. Runtime loading requires four wiring steps: import in `src/go/plugin/go.d/collector/init.go`, `modules:` toggle
in `src/go/plugin/go.d/config/go.d.conf`, stock job config at `src/go/plugin/go.d/config/go.d/<name>.conf`, and entry in
`src/go/plugin/go.d/README.md`. Same trap applies to `ibm.d`.

**Don't:**
- write new go.d modules against V1
- add modules to `charts.d.plugin` or `python.d.plugin`
- run `go generate` for go.d (no `//go:generate` directives — uses `//go:embed`)
- add new third-party Go modules or system-library dependencies casually — they ship to every Netdata install; check
  with maintainers if non-trivial

## 6. Canonical documentation pointers

| Topic | Open when | Path |
|---|---|---|
| NIDL framework | designing metrics, labels, charts | `docs/NIDL-Framework.md` |
| Chart types and dimension algorithms | choosing chart shape and metric algorithm | `src/database/rrdset-type.h`, `src/database/rrd-algorithm.h` |
| Chart priorities (C) | dashboard ordering convention | `src/collectors/all.h` |
| Shared metric definitions (C) | reusing common contexts | `src/collectors/common-contexts/` |
| Plugin types and privileges | choosing where to add a collector | `src/collectors/README.md` |
| External plugin protocol | non-Go external plugin | `src/plugins.d/README.md` |
| go.d collector design | deciding product boundary, ownership, options, metric semantics before code | `.agents/skills/collectors-go-design/SKILL.md` |
| go.d V2 authoring | adding a `go.d` module | `src/go/plugin/go.d/docs/how-to-write-a-collector.md` |
| go.d V1-to-V2 migration | migrating existing go.d collector | `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` |
| Functions backend (Go / Rust) | implementing a Function | `src/go/plugin/framework/functions/README.md`, `src/crates/netdata-plugin/rt/src/lib.rs` |
| Functions UI schema & guides | response shapes and patterns | `src/plugins.d/FUNCTION_UI_SCHEMA.json`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`, `src/plugins.d/FUNCTION_UI_REFERENCE.md` |
| Topology Function schema & guide | topology actors, links, evidence, overlays | `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`, `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`, `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` |
| Functions validator | E2E + schema validation | `src/go/tools/functions-validation/README.md` |
| ibm.d framework | starting `ibm.d` work | `src/go/plugin/ibm.d/AGENTS.md`, `src/go/plugin/ibm.d/framework/README.md` |
| Rust plugin SDK | new Rust plugin | `src/crates/netdata-plugin/` (`rt/`, `protocol/`, `bridge/`, `charts-derive/`, `schema/`, `types/`, `error/`) |
| Rust NetFlow plugin | NetFlow / sFlow / IPFIX work | `src/crates/netflow-plugin/` |
| OTEL ingestion mappings | per-metric YAML routing | `src/crates/otel-ingestor/` (configs under `configs/otel.d/v1/metrics/`) |
| SNMP profiles | adding/extending an SNMP profile or trap profile | `.agents/skills/collectors-snmp-profiles/SKILL.md`, `.agents/skills/collectors-snmp-trap-profiles/SKILL.md`; format spec: `src/go/plugin/go.d/collector/snmp/profile-format.md` |
| SNMP stock profiles | starting from a known device | `src/go/plugin/go.d/config/go.d/snmp.profiles/default/` |
| statsd synthetic_charts | operator-curated dashboards | `src/collectors/statsd.plugin/README.md` (lines 397-639) |
| Prometheus mapping | generic exposition scrape | `src/go/plugin/go.d/collector/prometheus/README.md` |
| Prometheus profile format | curated exporter dashboards + autogen fallback selectors | `src/go/plugin/go.d/collector/prometheus/profile-format.md` |
| Prometheus metric relabeling | rewriting scraped metric names/labels | `src/go/plugin/go.d/collector/prometheus/relabel/README.md` |
| log2journal | parsing application logs into the journal | `src/collectors/log2journal/log2journal.d/` |
| Auto-discovery rules | adding service-detection rules | `src/go/plugin/go.d/config/go.d/sd/{net_listeners,docker,snmp,http}.conf` |
| Topology library | topology producers in Go | `src/go/pkg/topology/v1` |
| netipc cross-plugin enrichment | C / Go / Rust | `src/libnetdata/netipc/`, `src/go/pkg/netipc/`, `src/crates/netipc/` |
| DYNCFG protocol | dynamic configuration | `src/plugins.d/DYNCFG.md`, `docs/developer-and-contributor-corner/dyncfg.md` |
| Health alerts | adding, changing, or reviewing an alert/template | `.agents/skills/health-alert-authoring/SKILL.md` |
| Integration page content | what each `metadata.yaml` field says and how it reads | `.agents/skills/collectors-metadata-yaml/SKILL.md` |
| Integrations pipeline | doc generation from `metadata.yaml` | `integrations/README.md` |
| Go framework changes | changing shared Go collector/runtime framework code | `src/go/plugin/framework/docs/changing-framework-code.md` |
| Credentials in config | `${env:}/${file:}/${cmd:}/${store:}` | `src/collectors/SECRETS.md` |
| Privileged operations | restricted setuid helper | `src/collectors/utils/ndsudo.c` |

## 7. Maintaining this skill

This skill is **live**. When you find a gap, an outdated pointer, a new pattern, or a bad practice not yet captured,
propose changes to this file in the same PR that exposed the issue. When fixing a wrong pointer, also record what was
misleading about the prior text — future readers see both the corrected map and the failure mode that produced it.
Mention the change in the PR description so it gets reviewed consciously rather than skimmed.
