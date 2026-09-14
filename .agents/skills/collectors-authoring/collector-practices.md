# Collector Contracts And Practices

Detailed reference for the shared contracts and task routes in `./SKILL.md`. Read the sections relevant to the
change. Implementation checks below describe authorized work; review uses the existing evidence and the action
boundaries in `AGENTS.md#skill-selection`. Framework and domain owners supply their specific mechanisms.

## 1. Mental model

How to think about Netdata data collection. Internalize this before designing anything.

### 1.1 Frequent collection at scale

Collection work repeats at each configured interval across large fleets and many platforms. Allocation, logging,
reconnection, retries, parsing and formatting therefore multiply with collection frequency and instance count.
Account for those costs when choosing a design; a population estimate is not performance evidence for a specific
change. Agent profile intervals are set in `src/daemon/config/netdata-conf-profile.c`; collector overrides include
`Defaults.UpdateEvery` in `src/go/plugin/go.d/collector/ping/collector.go` and
`src/go/plugin/go.d/collector/snmp/collector.go`. Use those source defaults and benchmarks rather than a fixed
install-count claim.

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

End an entity's chart lifecycle when the source establishes that it was removed or the operator intentionally
excludes it: a process exited, a container was removed, a profile target was dropped, or an authoritative discovery
snapshot no longer includes an interface. A temporarily unreachable managed device is not known removal. Preserve
missing measurements as gaps; do not retire its charts merely because a poll failed.

This truthfulness rule applies even to one instance. Obsoletion changes chart and alert lifecycle; it is not a way to
turn failed collection into recovery. Health behavior is owned by `.agents/skills/health-alert-authoring/SKILL.md`.

Mechanisms:

- C: `rrdset_is_obsolete___safe_from_collector_thread()` in `src/database/rrdset.c` sets `RRDSET_FLAG_OBSOLETE`;
  `rrdset_isnot_obsolete___safe_from_collector_thread()` handles reappearance.
- go.d V1: `Obsolete` and `MarkRemove()` are defined in `src/go/plugin/framework/collectorapi/charts.go`.
- go.d V2: `charts.yaml` lifecycle policy and `src/go/plugin/framework/chartengine/` own expiry. Failed collection
  attempts do not advance the planner's lifecycle transitions; absence in successful observations is different.
  Use the V2 authoring or migration guide for the applicable collector contract.
- Avoid lifecycle churn when absence can be transient. Choose the source-appropriate successful-observation/expiry
  policy instead of a universal one-minute timer; do not delay a known permanent removal solely to satisfy that timer.

### 1.6 Verify The Supported Source Contract

Before designing a collector or interpreting a changed payload, verify the relevant supported protocol/application
version against official documentation, release notes, or representative source behavior. Latest upstream is not
necessarily the version under review. Record which revision and behavior the evidence establishes; do not attribute
working-tree edits to an unmodified commit. Source identity and acquisition follow
`AGENTS.md#open-source-reference-evidence` and `.agents/skills/repo-mirror-sources/SKILL.md#inspect-existing-source`.

Do not rely on remembered binary formats, OID semantics, endianness or HTTP/JSON shapes. Recurring pitfalls include
NetFlow v5 versus v9/IPFIX fields, vendor MIB interpretation, PostgreSQL `pg_stat_*` changes and Kubernetes API removal.
Report an inaccessible source or untested version explicitly. Live probes, application setup and source acquisition
follow the actual task's authorization, not an instruction to always fetch the latest source.

### 1.7 Resolve Ambiguity With Independent Implementations

When an ambiguity could change supported behavior, compare relevant established collectors and the protocol's
reference implementation when available. Useful sources include Prometheus exporters, Zabbix templates, Datadog Agent
integrations, ntopng, LibreNMS, OpenNMS, Akvorado, collectd, pmacct and nfdump. Compare parsers, field interpretation,
edge cases and device quirks against the supported version; another collector is evidence, not the specification.

Use as many independent sources as the uncertainty warrants, rather than a fixed two-or-three-project quota. Inspect
available source first; acquire another checkout only when needed and authorized. Preserve source provenance for
quirks and fixtures instead of presenting unverified historical behavior as current fact.

### 1.8 Mirror an existing Netdata collector

The repo holds many go.d modules and internal C plugins. Maintainer patterns
and their documented framework contracts must agree. After you've reality-checked the upstream
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
- Potentially thousands of instances require evidence of operator usefulness and bounded resource cost. Cardinality
  alone does not prove a metric is useless; unconstrained growth still burdens streaming, ML, alerts and queries.
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

During authorized implementation, source test data based on what you're collecting. Reviewers assess the available
evidence and gaps; these examples do not authorize starting services or harvesting live data:

- **Open-source / freely available applications** (MySQL, PostgreSQL, NGINX, Redis, MongoDB, RabbitMQ): run the actual
  application locally (Docker, native install). Validate against real output. Cover multiple versions when defaults
  diverge.
- **Closed-source / vendor / SaaS** (vendor switches, IBM workloads, cloud APIs, hypervisors): harvest fixtures from
  other open-source monitoring projects — Prometheus exporters, Zabbix templates, Datadog Agent integrations, vendor SDK
  samples, anonymized traces in vendor PRs/issues. Check provenance, redistribution rights and whether those fixtures
  represent the supported behavior; availability alone does not establish completeness.
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

- Reuse stable long-lived resources: clients and connections, parsed regexes and
  matchers, metric instruments, and any large buffer rebuilt identically every cycle. Set them up in `New()` or
  `Init()` when appropriate; lazy acquisition, reconnects and runtime-only work may have different lifecycle owners.
  See `src/go/plugin/go.d/collector/cato_networks/metrix.go` for typed V2 instruments. A bounded temporary holding this
  cycle's results in a network-bound collector is not a defect; do not add pooling or retained state to satisfy a
  slogan. Allocation discipline for framework and per-sample code is `src/go/AGENTS.md` "Hot-Path And Benchmark
  Discipline".
- Reuse connections or pools where the protocol supports it; reconnect with bounded backoff when needed. Do not
  force persistent connections onto a stateless or short-lived transport contract.
- Reuse what is stable between iterations (schema, capabilities, parsed profile selections, instrument handles) only
  when staleness is safe. Cache scope and its evidence are design decisions, not a default; for values that authorize a
  dangerous operation, `.agents/skills/collectors-go-design/mutating-collectors.md` §5 owns the rule.
- Bound its work per call: a per-request timeout, honored context cancellation, and bounded fan-out. The scheduling
  interval is not a completion guarantee; if the collector promises a whole-cycle deadline, that promise is an
  explicit design decision with its own test.

A fresh V1 result map is a current-cycle snapshot, not rebuilding the static metric surface.
`src/go/plugin/framework/collectorapi/collector.go` defines that return shape; AP
(`src/go/plugin/go.d/collector/ap/collect.go`) emits fields conditionally. Reusing an uncleared map can fabricate stale
measurements. Optimize measured material costs while preserving omission semantics; do not mandate pooling or
persistent result maps. V2 instrument reuse and expensive parser/client reuse remain separate requirements.

### 2.3 Error handling

Every error log answers three questions: **what operation, what target, what was expected vs observed**. Wrap errors
with context (Go: `fmt.Errorf("...: %w", err)`); preserve the cause; check return codes from system calls and library
functions.

Don't return a bare `err` with no context. Don't log `"failed"`. Don't ignore syscall returns or library NULLs.

### 2.4 Logging discipline

Keep repeated recoverable warning/error conditions bounded across collection cycles; do not flood logs per entity or
per poll. Use the existing framework's limiting mechanism rather than requiring a collector-local once flag. For Go,
`src/go/plugin/go.d/docs/helper-packages.md#limited-logging` owns `Limit`, stable low-cardinality keys and `Once`.
`Job` and `JobV2` reset Once state each cycle, so it is not cross-cycle suppression. V2 partial-versus-full failure
handling is in `.agents/skills/collectors-go-framework-v2/SKILL.md#hot-path-logging`.

Debug diagnostics MAY run inside the collection loop when enabled and proportionate. Use info/notice for lifecycle
milestones, reserve error logs for actionable failures, and preserve contextual full-collection errors for the job
runtime. A transient partial condition may warrant a limited warning. Do not suppress the failure signal merely to
avoid logs, and never log raw credentials. The repeated per-PID logging failure class matters; a blanket ban on every
per-cycle diagnostic does not express it correctly.

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

- Unbounded HTTP route x method x status code expansion: N x M x K series per service.
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

Generated outputs are not editing sources. Identify the actual input before correcting them:

- Collector `integrations/<slug>.md` pages are generated from metadata, not hand-authored integration sources.
- IBM inputs include `contexts/contexts.yaml`, editable `config.go` and `module.yaml`; generated outputs include
  context Go code, `config_schema.json`, metadata and README content. Use
  `src/go/plugin/ibm.d/AGENTS.md#auto-generated-files` and `.agents/skills/integrations-lifecycle/ibm-d.md` for the
  producer chain and safe preview; do not treat `config.go` as generated or ignore the runtime schema output.
- Rust chart definitions may be derived at compile time through `charts-derive`; edit its input.

Use `.agents/skills/integrations-lifecycle/consistency.md#delivery-boundary-source-pr-versus-post-merge-pr` for which
runtime outputs ship with the source change and which generated documentation follows after merge. Preserve existing
generated-file edits during validation. Current go.d assets use `//go:embed`, not a collector `go generate` step;
verify directives before inventing one. Loading this reference for review does not authorize regeneration.

### 2.8 Documentation/configuration consistency

Collector consistency has one detailed checklist:
`.agents/skills/integrations-lifecycle/consistency.md`. Treat code,
integration metadata, config, stock examples, alerts, and generated
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

1. Does authoritative evidence cover the supported spec/protocol/application revision, rather than memory or an
   unrelated latest version?
2. Were consequential ambiguities checked against relevant independent implementations and their limitations?
3. Do all metrics have units, chart families, and meaningful names? Did NIDL inform the grouping? Are chart types and
   dimension algorithms correct (`incremental` for counters, etc.)?
4. Are gaps preserved (no zero defaults for missing values)?
5. Does the cycle rebuild expensive stable state, incur material avoidable cost, flood logs, or reconnect needlessly?
   Are legitimate snapshots preserved, and values authorizing dangerous operations re-checked at that operation?
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
13. For Functions: does the response conform to its schema-defined shape? Is it non-blocking with respect to the
    collection loop and schema-validated?
14. For ibm.d: were generated runtime artifacts validated after changing contexts, config or module inputs, following
    the IBM owner and the isolated-generation/documentation delivery boundary?
15. For new go.d modules: are runtime registration and configuration wired, and the separate metadata/README/stock
    documentation consistency obligations complete under the repository-wiring guide?
16. Tests: real fixtures or real instances? Would they catch the bug I just fixed?
17. High-cardinality labels / instances: which bounding mechanism from §2.5 applies, and does any public knob name an
    operator decision?
18. Are known removals distinguished from failed observation, with source-appropriate lifecycle policy that avoids
    churn rather than a hardcoded universal delay?
19. Production-quality criteria above — would this collector survive hours of target outage without leaks or log floods?

## 5. Framework Details

### 5.2 go.d V1 / V2 reality check

Existing go.d V1 collectors remain supported. Do not use their interface as the shape for a new go.d module;
the current authoring guide owns new-collector patterns.

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

`Init()` prepares the job; non-retryable initialization errors disable autodetection, while errors explicitly marked
retryable can retain configured retry eligibility. `Check()` is the cheap detection probe; retries depend on job
policy. `Collect()` is the scheduled hot path. Cleanup belongs to the orderly runtime teardown, including the optional
V2 runner's shutdown ordering. Exact behavior is in `src/go/plugin/framework/jobruntime/job_v1.go`,
`src/go/plugin/framework/jobruntime/job_v2.go`, `src/go/plugin/framework/jobruntime/job_common.go` and
`src/go/plugin/framework/dyncfg/handler.go`; public lifecycle requirements are in
`src/go/plugin/go.d/docs/how-to-write-a-collector.md#registration-and-lifecycle`.

A collector can compile without being registered or enabled. Runtime imports and configuration are distinct from
README/metadata consistency: `src/go/plugin/go.d/docs/how-to-write-a-collector.md#repository-wiring` owns the go.d
checklist. IBM uses its own framework and imports in `src/go/cmd/ibmdplugin/main.go`; follow IBM instructions rather
than copying go.d wiring or applying the new-go.d V2 mandate to it.

**Don't:**
- write new go.d modules against V1
- add modules to `charts.d.plugin` or `python.d.plugin`
- assume go.d needs `go generate`; inspect actual directives rather than copying the IBM procedure
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
| statsd synthetic_charts | operator-curated dashboards | `src/collectors/statsd.plugin/README.md#synthetic-statsd-charts` |
| Prometheus mapping | generic exposition scrape and authored profiles | `src/go/plugin/go.d/collector/prometheus/profile-format.md`, `src/go/plugin/go.d/collector/prometheus/` source; generated README is an operator output |
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
