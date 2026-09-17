---
name: collectors-go-design
description: Design or review go.d collector and discoverer contracts, including capability scope, options, metric meaning and identity, vnodes, Functions, ownership, remote writes and durable state. Also author or review DynCfg config_schema.json forms. Contract-preserving fixes and migrations use framework guidance; integration prose uses the metadata skill.
---

# Go Collector Design

Use this skill to design or review what a collector or discoverer promises, who owns its state, what an operator
decides, what a sample means, and what proves it. Resolve applicable implementation design questions before code.
Collector mechanics live in `.agents/skills/collectors-go-framework-v2/SKILL.md` and
`src/go/plugin/go.d/docs/how-to-write-a-collector.md`; artifact delivery lives in
`.agents/skills/integrations-lifecycle/`. Do not restate those here.

Every rule below is written as: **When** it applies, **Do / Don't**, what counts as **Evidence**, and its **Boundary**
(the legitimate exception). A checked box or an approval phrase is never evidence.

## Review And Implementation

Apply `AGENTS.md#skill-selection`. During review, use the applicable items as questions about the changed contract,
source and existing design/validation evidence. Report concrete missing or inconsistent evidence; do not create a SOW,
design note, truth-table artifact or mutation test merely because this authoring workflow describes it. Required
acceptance evidence still matters. Loading a reference does not authorize its UI changes, remote operations or setup.

During authorized implementation, record the applicable design before code and retain the project's approval gates.
A pure form presentation edit uses the schema reference without a collector design note; an option/default/meaning
change remains design work even when implemented through the schema. An ordinary transient cache does not by itself
require durable-state machinery; ownership, lifecycle and cost changes still need their applicable design review.

## When This Skill Applies

| Task | Load | Design note depth |
|---|---|---|
| New go.d collector | this skill, then the V2 skill and the how-to guide | full note; one line per item for a small read-only collector |
| New go.d discoverer or changed discovery capability, ownership or lifecycle | this skill; implementation under `src/go/plugin/go.d/discovery/sdext/discoverer/` and shared engine under `src/go/plugin/agent/discovery/` | applicable product, ownership, lifecycle and option items; no collector metric or V2 requirements unless affected |
| New public config option, mode, or default change | `operator-surface.md` (the option's decision record) | the affected item only |
| Form presentation in `config_schema.json`, with option/default semantics unchanged | `config-schema.md` | none; a changed operator contract uses the relevant design row |
| New or changed metric meaning, new entity axis, vnodes | the Metric Semantics and Identity items | the affected item only |
| Collector that writes or deletes remotely, or persists durable local state | this skill plus applicable `mutating-collectors.md` sections | full note plus applicable state/mutation items |
| Reviewing any of the above | affected items as review questions and existing evidence | no new implementation artifact |
| Contract-preserving migration/fix or integration prose only | migration/V2 or metadata/integration guidance as applicable | none; schema forms remain routed above |

## The Collector Design Note

**When:** authorized design or implementation of a new collector, discoverer, or affected public, ownership or lifecycle
contract. **Do:** fill the applicable items below as a `Collector design:` block under
"Affected contracts and surfaces" in the SOW's Pre-Implementation Gate, before implementation. **Don't:** create
separate documents, or answer items the collector does not have; write "none" with the reason instead. **Evidence:**
each item cites its source (provider doc, existing code, framework contract, user decision). **Boundary:** a small
collector that only reads its source answers applicable items in one line each. Pure form edits and review-only
requests follow the exceptions above. Remote mutation or durable local state selects the applicable sections of
`mutating-collectors.md`; it does not make every item apply to every collector.

1. **Product boundary.** State the operational question, supported providers, versions and configurations, explicit
   non-goals, and whether each measurement is client-observed or a backend guarantee. Distinguish a repair of the
   contemporary contract, making that contract discoverable, and expanding it. Each added capability, including
   network discovery, log ingestion or a Function, MUST have an approved operator need; a request for monitoring does
   not imply these products. Protocol/SDK support, a sibling collector's features, and words such as "comprehensive"
   or "production-ready" do not establish that need. Approval already covering the capability remains valid; no
   separate approval per component is required. An explicitly excluded capability is not a defect.
2. **Provider contract.** For every operation the design depends on, name the permissions, consistency assumptions,
   retries, and error meanings, with a link to the applicable provider/version documentation. When more than one
   provider or mode is involved, fill a capability matrix: one row per operation, one column per provider, cells say
   supported / semantics / evidence. An S3-compatible request API does not imply interchangeable replication,
   versioning, or deletion semantics. Similar charts may share observations while provider operations differ; decide
   sharing from the matrix, not from vendor count or API naming.
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

For configured vnode acquisition and named attachment, use
`src/go/plugin/framework/vnodes/README.md#ownership` and
`src/go/plugin/framework/vnodes/README.md#collector-attachment` as the existing ownership contract before proposing
collector-owned identity polling or shared connection settings.

**When:** a proposal adds caching, durable state, scheduling, queues, cross-job dependencies, cleanup that can freeze
measurement, or a generic engine around provider-specific behavior. This includes machinery owned by just one
collector or discoverer. **Do:** answer the applicable questions below before code; if the justification is unknown,
investigate or ask rather than implement a placeholder. **Don't:** make an unjustified scan faster, raise its cap,
add retries, or write tests that expect the coupling. **Evidence:** a supported requirement and a concrete failure of
its direct implementation, using the evidence standard in `src/go/AGENTS.md#evidence-before-complexity`.
**Boundary:** state, locks, shared clients, framework infrastructure and protocol/security controls remain valid when
justified. "This looks complex" alone is not a finding. An ordinary transient cache may need only a brief source-based
explanation; it does not require persistence or a benchmark by default.

Designers and reviewers MUST evaluate the initiating decision before hardening its supporting layers. Trace
serialization, versioning, invalidation, locking and recovery back to the mechanism that requires them. Correctness
of those layers and passing tests do not prove that the mechanism is needed. If it is unjustified, propose removing
or redesigning it with its dependent machinery within the approved scope; do not weaken safeguards around a retained
requirement to reduce complexity.

1. **Requirement and direct alternative.** What approved operator need or correctness, liveness, protocol, security,
   compatibility or workload contract is being satisfied? Describe the direct implementation using existing helpers
   and independent ownership, without the proposed machinery.
2. **Necessity.** Which supported execution fails with that alternative? Identify the input, failure and requirement
   it violates. For persistence, explain why re-querying or reconstructing state after restart is insufficient. For
   a cost argument, show the expected workload and source-derived bounds or measurements; "avoids repeated work" is
   insufficient by itself.
3. **Narrowest boundary.** What state or resource needs the mechanism, who owns it, and for how long? For cross-job
   coordination, identify the shared object, namespace, limit or protocol; show the collision with independent
   ownership and why per-owner identity/exclusion is insufficient, using real keys. Sharing a directory, SDK or
   provider type is not itself a collision.
4. **Failure propagation and cost.** Can a stopped, corrupt, or unreachable job block a healthy one? State the cost
   variables: work per job per call, per retained item, remote calls, state serialization, lock scope, growth with
   jobs and backlog. Use source-derived bounds at design time; a shipped hot-path change still follows
   `src/go/AGENTS.md#hot-path-and-benchmark-discipline`. A cache that preserves failure coupling is not a fix.
5. **Decision.** Necessary machinery is exposed as an operational trade-off and gets the applicable design approval;
   otherwise use the direct implementation or revise the requirement with the user. For durable ownership also state
   recovery consequences: same versus different owner, label change, credential rotation, location change and rename.
   Isolation does not solve identity migration; say what a renamed job does and does not inherit.

Worked example from S3check: recovering ownership of created remote objects after a crash requires a journal. The
original proposal additionally scanned every job's ownership files under a global handoff lock before publishing a
probe. Object keys were already namespaced per Agent and job; the only real overlap was the same job's old and new
runtime during reload. A per-owner lock and journal (owner = Agent registry ID + job name) covered that overlap. The
global scan let an unrelated corrupt journal block a healthy job, cost O(jobs²), and turned growth into failure through
a file-count cap. Reject the cross-job coordination, retain the necessary owner-scoped journal and lock, and record
that a renamed job does not adopt old ownership. This follows from the supported execution and ownership keys.

## Measurement Truth Table

**When:** every new observation, and every change to what an existing one means. **Do:** fill one row per
observation, then map states to values, before writing metric names or `charts.yaml`. **Don't:** emit a value that
looks like a measurement for something not measured; a skipped operation has no duration, a failed attempt is not a lag
or success sample (its request duration may still be a valid measurement of the request), a waiting state is not a zero
(`.agents/skills/collectors-authoring/collector-practices.md#14-gaps-are-data`). **Evidence:** the table itself, plus a
test that drives each row's state through the real path and asserts emitted / omitted / retained. **Boundary:**
human-readable configuration does
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

1. Select affected items from the task table. Implementation records its applicable note; review checks existing
   contracts and evidence under `./SKILL.md#review-and-implementation`.
2. Read `operator-surface.md` for option/mode decisions and `config-schema.md` for form authoring or review.
   Read `mutating-collectors.md` for remote mutation or durable local state, using its scoped routes.
3. For implementation mechanics or their review, use `src/go/plugin/go.d/docs/how-to-write-a-collector.md` and the V2
   skill when that framework applies; contract-preserving migrations use the migration guide.
4. When metadata or delivery is affected, use `.agents/skills/collectors-metadata-yaml/SKILL.md` and
   `.agents/skills/integrations-lifecycle/consistency.md`. Reading delivery guidance does not request regeneration.
