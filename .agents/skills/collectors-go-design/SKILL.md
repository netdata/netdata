---
name: collectors-go-design
description: Early design decisions for Go go.d collectors, made before code. Use when designing a new go.d collector; when adding, renaming, removing, or changing the default of a config option or mode; when adding or changing a Function; when changing what a metric means or which entities become charts or vnodes; when a collector will write or delete remote objects, persist state across restarts, or coordinate with other jobs; when asking "should this be a config option", "who owns this state", "what does this sample mean", "should this be a vnode"; and when reviewing such a change. Not for V1-to-V2 migrations without approved enrichment, mechanical fixes, or docs-only work.
---

# Go Collector Design

This skill decides the shape of a collector before code exists: what it promises, who owns what, what an operator
decides, what a sample means, and what proves it. Implementation mechanics live in
`.agents/skills/collectors-go-framework-v2/SKILL.md` and
`src/go/plugin/go.d/docs/how-to-write-a-collector.md`; artifact delivery lives in
`.agents/skills/integrations-lifecycle/`. Do not restate those here.

Every rule below is written as: **When** it applies, **Do / Don't**, what counts as **Evidence**, and its **Boundary**
(the legitimate exception). A checked box or an approval phrase is never evidence.

## When This Skill Applies

| Task | Load | Design note depth |
|---|---|---|
| New go.d collector | this skill, then the V2 skill and the how-to guide | full note; one line per item for a small read-only collector |
| New public config option, mode, or default change | `operator-surface.md` (the option's decision record) | the affected item only |
| Writing or changing `config_schema.json` (the DynCfg form) | `config-schema.md` | none; it is authoring, not design |
| New or changed metric meaning, new entity axis, vnodes | the Metric Semantics and Identity items | the affected item only |
| Collector that writes or deletes remotely, or persists state | this skill plus `mutating-collectors.md` | full note plus the mutating items |
| Reviewing any of the above | the same items as review questions | — |
| V1-to-V2 migration without approved enrichment, mechanical fix, docs-only change | not this skill (`migrate-v1-to-v2.md`, V2 skill, `integrations-lifecycle`) | none |

## The Collector Design Note

**When:** any task in the table above. **Do:** fill the applicable items below as a `Collector design:` block under
"Affected contracts and surfaces" in the SOW's Pre-Implementation Gate, before implementation. **Don't:** create
separate documents, or answer items the collector does not have; write "none" with the reason instead. **Evidence:**
each item cites its source (provider doc, existing code, framework contract, user decision). **Boundary:** a small
read-only collector answers every item in one line each; only a collector that writes, deletes, or persists state
loads `mutating-collectors.md`.

1. **Product boundary.** State the operational question the collector answers, the supported providers, versions,
   and configurations, the explicit non-goals, and whether each measurement is client-observed or a backend
   guarantee. Separate three things that get confused: a repair (the contemporary contract was violated), a
   discoverability fix (the contract was hidden), and an expansion (a new capability, which needs the user's
   approval). An explicitly excluded capability is not a defect.
2. **Provider contract.** For every operation the design depends on, name the permissions, consistency assumptions,
   retries, and error meanings, with a link to the current provider documentation. When more than one provider or
   mode is involved, fill a capability matrix: one row per operation, one column per provider, cells say supported /
   semantics / evidence. An S3-compatible request API does not imply interchangeable replication, versioning, or
   deletion semantics. Similar charts may share observations while provider operations differ; decide sharing from
   the matrix, not from vendor count or API naming.
3. **Architecture and ownership.** Name what owns config, client transport, normalization, durable state, and
   presentation; which existing helpers fit (`src/go/plugin/go.d/docs/helper-packages.md`); what needs a boundary and
   what stays direct code. Any coupling across jobs or owners, any durable state, any scheduler or queue goes through
   the Architecture Gate below first.
4. **Identity and lifecycle.** Name what survives a cycle, a restart, and a reload; which identity is stable and which
   is display metadata; which concurrent owners actually exist (the same job's old and new runtime is one case,
   different jobs another). Trace one successful cycle and one cycle with unfinished cleanup before designing the
   engine. If housekeeping would stop measurement, either state the real dependency or separate the two state
   dimensions; do not shorten a safety interval to hide the stall.
5. **Operator surface.** One decision record row per proposed option, the mode form as a user task, and the
   consumer traces for defaults and null: `operator-surface.md`. List the implementation details you intentionally do
   not expose.
6. **Metric semantics.** One measurement truth table row per new observation (below). Derive names, units, and help
   text from the table, never the other way round.
7. **Evidence plan.** What proves the real path (real construction, real transitions, the shipped adapter), which
   fakes carry independent semantics, and what cannot be verified locally and is therefore stated as unverified. Test
   rules live in the V2 skill's Tests section.

## Architecture Gate

**When:** a proposal makes one job depend on another job's state (scanning its journals, waiting for its cleanup,
sharing an operational lock, consulting a registry for permission to run), introduces durable state for otherwise
independent reads, adds a scheduler or queue, lets cleanup freeze measurement, or builds a generic engine around one
provider's quirks. **Do:** answer the five questions in the design note before code; if the answer is "unknown",
investigate or ask, never implement a placeholder for later review. **Don't:** repair the proposal by making the scan
faster, raising a file cap, adding retries, or writing tests that expect the coupling; those preserve the wrong
dependency. **Evidence:** concrete object keys, owner identities, and a named invariant, not "there could be races".
**Boundary:** a genuinely shared resource may require coordination; the gate is not a ban on locks, shared clients,
or framework infrastructure, and "this looks complex" is not a finding without a named dependency and consequence.

1. **Shared resource.** What concrete object, namespace, limit, or external protocol is shared? Sharing a directory,
   an SDK, or a provider type is not a collision.
2. **Necessity.** Which supported execution fails with independent per-job ownership? Show the collision.
3. **Narrowest boundary.** Why is per-owner identity and exclusion insufficient? Compare with the proposal using real
   keys and owner identities.
4. **Failure propagation and cost.** Can a stopped, corrupt, or unreachable job block a healthy one? State the cost
   variables: work per job per call, per retained item, remote calls, state serialization, lock scope, growth with
   jobs and backlog. Use source-derived bounds at design time; a shipped hot-path change still follows
   `src/go/AGENTS.md` "Hot-Path And Benchmark Discipline". A cache that preserves the failure coupling is not a fix.
5. **Decision.** Necessary coordination is exposed as an operational trade-off and gets the applicable design
   approval. Unnecessary coordination is redesigned around the actual owner boundary. For durable ownership also
   state the recovery consequences: same owner versus different owner, label change, credential rotation, location
   change, rename. Isolation does not solve identity migration; say what a renamed job does and does not inherit.

Worked example, from the S3check original: the proposal scanned every job's ownership files under a global handoff
lock before publishing a probe. Q1: object keys were already namespaced per Agent and per job, so no key collided;
a shared directory is not a shared resource. Q2: no supported execution failed with per-job ownership; the only real
overlap was the same job's old and new runtime during reload. Q3: a per-owner lock and journal (owner = Agent
registry ID + job name) covered that overlap. Q4: an unrelated corrupt journal blocked a healthy job, scanning was
O(jobs²), and a 256-file cap turned growth into a hard failure. Decision: reject the coordination, redesign around
owner identity, record that a renamed job does not adopt old ownership. All of this was decidable from the proposal.

## Measurement Truth Table

**When:** every new observation, and every change to what an existing one means. **Do:** fill one row per
observation, then map states to values, before writing metric names or `charts.yaml`. **Don't:** emit a value that
looks like a measurement for something not measured; a skipped operation has no duration, a failed attempt is not a lag
or success sample (its request duration may still be a valid measurement of the request), a waiting state is not a zero
(`collectors-authoring` §1.4, gaps are data). **Evidence:** the table itself, plus a test that drives each row's
state through the real path and asserts emitted / omitted / retained. **Boundary:** human-readable configuration does
not prohibit millisecond latency charts; the table decides units per chart.

| Column | Meaning |
|---|---|
| Eligibility | when the observation may be produced at all |
| Start event | what starts the measurement (for S3check delete lag: the successful source DELETE, not the first attempt) |
| Stop event | what ends it (observed destination absence; a timeout ends the attempt, a breach does not) |
| Scope | per probe, per target, per job |
| Aggregation | how repeated calls become one value or status before chart labels collapse them |
| Consequence | what the operator should conclude from it |

Then map each of: measured, missing, skipped, waiting, retrying, failed operation, failed collection, backpressured,
terminal, to one of: emitted value, omitted (gap), retained last terminal value, state-set state. Distinguish a
measured unhealthy target from an inability to collect; make sure the framework commits the intended failure
observations; check every early return against the table. Comparisons follow the wording ("exceeding" is strict).

## Lifecycle Entry Points

**When:** designing `Init`, `Check`, `Collect`, `Cleanup`, and any `Run`. **Do:** review every entry point, including
partial initialization, DynCfg `test`, autodetection, reload, and stop, not only `Collect` followed by a clean
shutdown. Decide, per entry point, what it may do, and record it; the V2 skill's Core Style owns the resulting rules
(`Check` detection-only, cleanup caller-cancelled or detached with a fixed budget independent of request and retry
settings; S3check chose five seconds). **Don't:** derive a shutdown budget from public tuning, or make orderly cleanup
the crash-recovery mechanism. **Evidence:** the trace per entry point in the design note; tests for cancellation and
partial init. **Boundary:** a read-only collector closing idle connections needs no journal or timeout machinery;
background contexts are not banned, unbounded ones are.

## Simplification As Engineering

**When:** throughout, and at the final pass. **Do:** prefer direct ownership, small types, clear transitions, and
existing helpers over generic engines and defensive layers; name files by responsibility and check that the content
matches; split along operations or state boundaries, not line counts; share only real semantics and keep distinct
provider logic distinct; run the V2 skill's Pre-PR final sweep before review. **Don't:** reject necessary state or
boundaries to minimize the diff, or add pooling and caching to satisfy a slogan. **Evidence:** per-cycle cost stated
from the source (a global O(N²) scan matters; a bounded map in a network-bound check does not). **Boundary:** file
length is a signal, not a limit; splitting one function into arbitrarily named helpers is not architecture.

## Reading Sequence

1. This skill: fill the design note in the SOW gate.
2. `operator-surface.md` for any option, mode, or form work; `config-schema.md` when writing the schema file itself;
   `mutating-collectors.md` only for side-effecting or persistent collectors.
3. `src/go/plugin/go.d/docs/how-to-write-a-collector.md` and the V2 skill for implementation.
4. `.agents/skills/collectors-metadata-yaml/SKILL.md` for the integration page (`metadata.yaml`), then
   `.agents/skills/integrations-lifecycle/consistency.md` for artifact delivery.
