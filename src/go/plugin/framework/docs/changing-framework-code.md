# Changing Go Framework Code

Requirement language follows the root `AGENTS.md` definitions.

This guide applies to shared Go framework code, not one collector's private
implementation. Framework changes have high blast radius and MUST be designed
before implementation.

## Scope

This guide applies when changing or extending any of these areas:

- `src/go/plugin/framework/collectorapi`
- `src/go/plugin/framework/jobruntime`
- `src/go/plugin/framework/charttpl`
- `src/go/plugin/framework/chartengine`
- `src/go/plugin/framework/chartemit`
- `src/go/plugin/framework/functions`
- `src/go/plugin/framework/vnodes`
- `src/go/plugin/framework/vnoderegistry`
- `src/go/plugin/framework/dyncfg`
- `src/go/plugin/framework/confgroup`
- `src/go/plugin/framework/runtimecomp`
- `src/go/plugin/go.d/pkg/collecttest`
- `src/go/pkg/netdataapi`
- `src/go/pkg/metrix`
- `src/go/pkg/funcapi`
- `src/go/pkg/topology/v1`
- `src/go/pkg/matcher`
- `src/go/pkg/stm`
- shared collector/runtime helpers under `src/go/pkg/` when their semantics are
  used by go.d collectors or framework runtime code, such as `web`,
  `prometheus`, `tlscfg`, `netdataapi`, and `netipc`
- shared go.d helper packages under `src/go/plugin/go.d/pkg/`, such as
  `collecttest`, `ndexec`, `logs`, `sqlquery`, `cloudauth`, `pinger`,
  `snmputils`, `k8sclient`, and `dockerhost`

It also applies when a collector change requires a new shared framework
capability instead of collector-local code.

## Core Rule

Framework changes MUST optimize for the clean end state, not the smallest local
diff. If a collector exposes a general framework gap, the implementation MUST
consider a framework change before adding collector-local glue, package-level
globals, duplicate helpers, or private coupling.

Framework changes MUST NOT begin until the applicable approval tier below is
satisfied.

## Framework Vs Collector-Local

Use this split before designing:

- Collector-local code is appropriate when the behavior depends on one upstream
  product, one collector's private model, or one collector's artifact set.
- Calling existing framework APIs from a collector is collector-local work.
  Changing those APIs, or changing any package in the scope list above, is
  framework work.
- Framework code is appropriate when the behavior affects lifecycle,
  chart/template semantics, metric storage, host scopes, Functions, topology,
  dynamic config, shared tests, shared matchers, or multiple collectors.
- The test is the generality of the behavior, not only the directory touched.
  Collector-local globals, singletons, adapters, duplicated helpers, or package
  glue that substitute for a missing general framework capability are framework
  work for approval purposes.
- A framework extension is usually appropriate when two collectors would
  otherwise need the same helper or workaround.
- A collector-local workaround MUST NOT be used only because it is less churn.

When uncertain, pause and ask for a design decision with evidence.

## Approval Tiers

Use the smallest tier that honestly fits the change. If the risk is unclear,
use the full design gate.

### Full Design Gate

The full design gate is REQUIRED for framework changes that affect contracts,
runtime behavior, compatibility, lifecycle, chart output, metric storage,
Function protocol, topology payloads, host scopes/vnodes, dyncfg behavior, or
multiple collectors.

The full design gate requires the design note below and explicit user approval
before implementation.

### Short Decision Gate

The short decision gate is allowed only for additive, backward-compatible
framework changes that do not alter existing behavior or public contracts. This
includes narrow cases such as exposing an existing helper, adding an extension
interface that existing implementations do not need to satisfy, or adding a
test helper that preserves all existing caller semantics.

The short decision gate MUST NOT be used to disguise a collector-local
workaround, avoid a full design discussion, or reduce the apparent blast radius
of a change that really belongs under the full design gate. Before using this
tier, verify that the change still serves the clean end state. If the change is
a hack, it MUST NOT be implemented under this tier.

Before implementation, record the short decision in the active TODO or SOW. The
record MUST include:

1. Root cause.
2. Why collector-local code is the wrong place.
3. Why the change is additive and backward-compatible.
4. Why this is the clean framework shape rather than a tier-reducing hack.
5. Approval source: either the exact user request that already approved this
   framework addition or the explicit approval response after presenting this
   short-gate note.
6. Affected packages and callers searched.
7. Representative collectors selected for validation, or why none apply.
8. Tests that will prove no existing behavior changed.
9. Documentation, spec, skill, and integration-artifact update decision.

Ask for explicit user approval when the request does not already cover the
decision, when compatibility is uncertain, or when another package or collector
needs changes to consume the new framework capability.
If the task began as collector work, the short gate still requires explicit
user approval before implementation.

## Required Design Note

For full-gate changes, prepare a design note, record it in the active TODO or
SOW, and get user approval. The design note MUST cover:

1. Root cause.
   - What exactly is broken or missing?
   - Why is a collector-local fix insufficient?
2. Clean end state.
   - What is the intended framework shape after the work is complete?
   - Which current compromise or workaround will be removed or avoided?
3. Scope boundary.
   - What is included in this step?
   - What is explicitly deferred?
   - Does any independent prerequisite need to land first?
4. Affected contracts.
   - Public interfaces, runtime behavior, chart template semantics,
     `metrix` read/write semantics, Function protocol, host scopes/vnodes,
     topology payloads, generated docs, tests, and collector compatibility.
5. Compatibility.
   - Is this preserving existing contracts?
   - If not, what breaking change did the user explicitly accept?
6. Existing patterns.
   - Which framework packages or collectors already solve something similar?
   - Which pattern is being reused?
7. Implementation batches.
   - Split into coherent commits when the work is non-trivial.
   - Each batch SHOULD build on the previous batch and be reviewable alone.
8. Validation.
   - Framework unit tests.
   - Representative collector tests.
   - Docs/spec/skill updates.

## Scope Checkpoints

At every coherent batch boundary, you MUST re-check scope:

- If the branch now contains independent framework work, split it out or defer
  it.
- If the collector change is complete but a framework cleanup is separate,
  submit the collector change first and continue later.
- If a framework change blocks the clean end state, pause and get approval for
  the framework change before continuing.
- If the current branch depends on an independent change, land that change first
  and rebase on master before continuing.

## Contract Checklist

Use this checklist when the changed package is involved.

### collectorapi

- Collector interfaces MUST stay compatible unless a breaking change is
  explicitly approved.
- Backward-compatible new collector capabilities MUST be expressed as extension
  interfaces that existing collectors are not required to implement. Breaking
  collector contract changes require explicit approval.
- Registration behavior MUST be covered by tests when changed.

### jobruntime

- Lifecycle semantics MUST be explicit: `Init`, `Check`, `Collect`, `Cleanup`,
  commit, abort, cancellation, retry, and runtime metrics.
- Cancellation behavior MUST be tested when changed.
- V1 and V2 behavior MUST be considered separately.

### metrix

- Read/write semantics MUST be documented and tested: snapshot vs stateful,
  gauges, counters, StateSet, labels, host scopes, flattening, and cycle abort.
- The `BeginCycle`, `CommitCycleSuccess`, and `AbortCycle` contract MUST be
  preserved and tested when changed.
- Identity and label behavior MUST be stable.
- New instrument behavior MUST include tests for both typed and flattened
  readers when applicable.

### charttpl and chartengine

- Template schema changes MUST update docs, validation, and compile tests.
- Runtime chart behavior MUST be covered by planner/engine tests.
- Per-host-scope planning MUST keep chart coverage, lifecycle, and labels
  isolated per scope.
- Generated chart IDs, contexts, dimensions, labels, and lifecycle behavior are
  public contracts and MUST be treated as stable unless a breaking change is
  approved.

### chartemit

- Emitted chart and host commands MUST remain compatible with the plugin
  protocol.
- Host/vnode identity changes MUST be tested against invalid and edge-case
  host information.

### host scopes and vnodes

See `.agents/sow/specs/go-v2-host-scope.md`.

- Scope identity MUST use deterministic stable IDs.
- Framework changes MUST preserve collector-provided `_vnode_type` labels. The
  framework does not synthesize this label for collectors.
- Cardinality MUST be bounded and documented.
- Representative scoped and unscoped collectors MUST be checked when read/write
  behavior changes.

### Functions

- Function protocol changes MUST stay compatible with
  `src/plugins.d/FUNCTION_UI_SCHEMA.json` unless a breaking change is approved.
- Function handlers MUST remain isolated from collector internals through narrow
  dependencies.
- Manager, scheduler, cancellation, and cleanup behavior MUST be tested when
  touched.

### topology

See `.agents/skills/project-create-topology/SKILL.md` and
`.agents/sow/specs/topology-function-schema.md`.

- New topology producers MUST use `src/go/pkg/topology/v1`.
- Payload changes MUST validate against
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Actor/link identity and table merge behavior MUST be treated as public
  contracts.

### matcher

- Existing matcher grammar and edge-case behavior MUST stay compatible unless a
  breaking change is approved.
- New selector behavior SHOULD use matcher package APIs instead of adding
  collector-local grammar.

### collecttest

- Test-helper changes MUST remain compatible with every current caller and MUST
  be validated with compile coverage plus representative callers.
- Shared assertions MUST NOT hide per-scope, per-label, or per-chart failures by
  over-aggregating results.

## Representative Collector Checks

When a framework change affects collectors, choose representative collectors
from the affected surface. Broad tests MUST NOT be used blindly as a substitute
for choosing the right representatives.

Common representatives:

- V2 metrics and chart templates: `cato_networks`, `azure_monitor`, `powerstore`,
  `powervault`, `ping`.
- V2 Functions: `mysql`, `cato_networks`.
- Host scopes/vnodes: `azure_monitor`, `cato_networks`.
- Topology Functions: `cato_networks`, `snmp_topology` when the legacy topology
  path is affected.
- Legacy V1 compatibility: pick a directly affected V1 collector and one simple
  V1 collector such as `apache` when changing shared V1/V2 runtime code.

The exact list SHOULD be justified in the design note.

## Validation

Validation MUST match the changed contract.

Examples:

- Framework package tests:
  - `go test -count=1 ./plugin/framework/...`
  - `go test -count=1 ./pkg/metrix/...`
  - `go test -count=1 ./pkg/matcher/...`
  - `go test -count=1 ./pkg/topology/...`
  - `go test -count=1 ./plugin/go.d/pkg/collecttest`
  - `go test -count=1 ./plugin/go.d/pkg/...` when shared go.d helper semantics
    change.
- Collector representatives:
  - `go test -count=1 ./plugin/go.d/collector/<name>/...`
  - `go test -race -count=1 ./plugin/go.d/collector/<name>/...` when
    concurrency, Functions, host scopes, or topology are involved.
  - HTTP/web helper changes: include at least one HTTP collector and its config
    serialization tests.
  - Matcher changes: include selector-using collectors.
  - `collecttest` changes: include several representative V2 collectors that use
    chart coverage, config serialization, and host scopes where relevant.
- Runtime components:
  - `go test -count=1 ./plugin/framework/jobruntime ./plugin/framework/runtimecomp`
- Runtime wiring and dyncfg lifecycle:
  - `go test -race -count=1 ./plugin/agent/jobmgr/...`
  - REQUIRED when changing `collectorapi`, `jobruntime`, `dyncfg`,
    `confgroup`, `vnoderegistry`, or runtime wiring behavior.
  - Representative files include `manager_v2_test.go`, `job_factory_test.go`,
    `sim_test.go`, `dyncfg_collector_test.go`, and `dyncfg_vnode_test.go`.
- Function/topology payloads:
  - schema validation tests in the affected collector or package.

Record exactly what ran. Full validation MUST NOT be claimed from a narrow
command.

## Artifact Updates

Framework changes often require durable artifact updates. Check each class:

- `AGENTS.md` and `src/go/AGENTS.md`
- project skills under `.agents/skills/`
- framework package READMEs
- specs under `.agents/sow/specs/`
- collector authoring docs under `src/go/plugin/go.d/docs/`
- integrations-lifecycle skill and artifacts if collector metadata/taxonomy
  changes
- public Function/topology schemas and guides if protocol behavior changes

If no artifact update is needed, record why in the active TODO/SOW.

If the framework work was discovered while writing a collector, return to
`src/go/plugin/go.d/docs/how-to-write-a-collector.md` after the framework
decision or change is complete.

## Anti-Patterns

- Framework behavior hidden behind a collector-local workaround.
- Package-level globals used to share state between framework packages.
- Shared test helpers that pass by aggregating away the failing dimension.
- Public interface changes without a compatibility decision.
- Runtime behavior changes without representative collector tests.
- New topology work using legacy topology payloads.
- Config knobs added to avoid designing the framework behavior.
- Continuing implementation after discovering independent scope that SHOULD be
  landed separately.
