---
name: project-health-alert-authoring
description: Author, adapt, modify, or review Netdata health alerts and alert templates. Use when translating alerts from another system; changing `src/health/health.d/*.conf`, lookup/calc/warn/crit expressions, lifecycle, timing, routing, ownership, or missing-data behavior; writing health-config tests; or selecting an alert's chart/context/label identity.
---

# Author Netdata Health Alerts

Use this skill before changing a health-alert definition. Alerts are production policy: a syntactically valid expression
can still page incorrectly, manufacture a recovery, duplicate an incident owner, or make a disappearing entity look healthy.

The normal goal when translating an alert from another system is **Netdata-adapted operational equivalence**, not
execution-engine emulation. Preserve the operator-visible incident as closely as Netdata can express it through NIDL
instances, alert beats, database lookups, native gap behavior, and chart obsoletion. Treat differences from the source
engine as explicit design facts to document and test, not as defects by default.

## Read The Right Sources

1. Read `AGENTS.md`, the active SOW, the target alert file, and its collector/profile source.
2. Read `docs/NIDL-Framework.md`, `src/health/REFERENCE.md`, `src/health/README.md`, and
   `src/health/alert-configuration-ordering.md`.
3. Read the existing alert owner and search for duplicate names, contexts, metric prefixes, and equivalent generic alerts.
4. When lifecycle, missing-data, expression, or lookup semantics matter, confirm them in current runtime source:
   - `src/health/health_event_loop.c`
   - `src/health/health_variable.c`
   - `src/web/api/queries/query-execute.c`
   - the selected grouping implementation below `src/web/api/queries/`
   - `src/libnetdata/eval/eval-evaluate.c`
5. When the source is a go.d collector or Prometheus profile, also load the matching collector/profile skill and its required
   references. Do not treat this skill as a replacement for collector or profile authoring guidance.

Never query or reconfigure a live Agent merely to validate an alert unless the user has explicitly authorized that access.

## Establish The Alert Contract Before Editing

Write down the following in the active SOW before changing a non-trivial alert:

- **Incident and owner:** What real incident does this alert represent? Which one source owns it? Do not create a
  product-named duplicate of an existing collection-failure, component-failure, or generic host alert just to change
  routing or severity.
- **Signal contract:** Identify every source value and its meanings, including zero, non-zero, tri-state values, absent
  dimensions, temporary collection failure, and known entity disappearance.
- **Source intent and adaptation:** Separate the source alert's operator intent from its engine syntax. Record which
  condition, scope, severity, persistence intent, identity, and recovery semantics Netdata preserves; record every
  deliberate difference. Use `NETDATA-ADAPTED` as the normal classification. Reserve `EXACT` for a proven coincidental
  match across timing, gaps, identity, recovery, and removal—not merely a similar expression.
- **NIDL instance map:** Record the monitored component, context, one instance type, RRDSET/chart ID identity, dimensions,
  stable identity labels, current metadata labels, and the collector's obsoletion condition. An alert attaches to one
  RRDSET/chart instance; a template applies that same rule independently to matching instances in one context.
- **Identity:** Choose an `alarm` only for a specific chart instance; choose a `template` for a context-wide rule. An
  identity label MUST identify the same monitored entity across its intended lifetime. A label whose source value can
  change for that entity is current metadata, not RRDSET identity: preserve the chart ID and update/promote the label.
  Confirm labels preserve the incident identity and do not create unbounded instances.
  Prove whether each promoted source label is live metadata, creation-time metadata, or source-frozen history. Do not
  describe a label as "current" merely because Netdata can update labels.
- **Variables and aggregation:** An unqualified variable resolves in the alert's local monitored component/instance before
  broader candidates. Use a fully qualified chart/dimension reference only after proving the other chart is the matching
  instance and labels select it unambiguously. Never synthesize an infrastructure-level alert by combining multiple
  RRDSET instances in alert configuration; require a source-owned aggregate RRDSET at the higher component level instead.
  The alert `component:` classification field does not replace this NIDL mapping.
- **Lifecycle:** Specify expected `UNINITIALIZED`, `CLEAR`, `WARNING`/`CRITICAL`, `UNDEFINED`, and `REMOVED` behavior.
  State whether an ordinary recovery emits a zero or whether the chart/dimension disappears instead.
- **Timing:** State the source update cadence, explicit alert cadence, lookup window, source persistence intent, selected
  Netdata adaptation, startup behavior, partial-gap behavior, stale-chart behavior, and notification policy. Do not call
  a lookup window an implementation of another engine's `for:` state machine.
- **Validation:** List the boundary, transition, missing-data, recovery, and duplicate-ownership tests that prove the
  contract.

Pause for a user decision when the change creates a public alert contract, changes default notification policy, changes
the owner of an incident, needs shared health/query framework work, or cannot preserve the approved operator-visible
incident closely enough with Netdata-native behavior. A difference from Prometheus execution alone is not such a blocker.

## Combine Values Across Dimensions, Charts, And Alerts

Choose the smallest pattern that can express the condition truthfully. Do not introduce cross-chart or intermediate-alert
plumbing when the required values already share the alert's chart.

### Pattern 1 — dimensions on the alert's chart

Use unqualified dimension variables in `calc`, `warn`, or `crit` when every required value is a dimension on the chart
named by `on:`:

```text
template: example_ratio
      on: app.state
     calc: $total - $used
```

Runtime facts:

- Matching is by dimension ID or name in the alert's own chart.
- The default dimension value is `collector.last_stored_value`: the latest database-stored value after interpolation, not
  a database-window aggregate and not necessarily the raw last collector sample.
- A missing dimension makes the expression fail with an unknown variable and the alert value become `NaN`.
- Prefer explicit non-finite guards when `UNDEFINED` is the required missing-input state; do not rely on arithmetic or
  comparisons to preserve `NaN`.

Use `$dimension_raw` only when the contract specifically needs the last collected value before interpolation/storage
presentation, and `$dimension_last_collected_t` only when it needs that dimension's collection timestamp.

### Pattern 2 — dimensions from another chart or context

Use a dotted chart/context reference when values live on different charts:

```text
template: example_cross_chart
      on: app.usage
     calc: $this * 100 / ${app.capacity.total}
```

The dotted prefix is interpreted from right to left as chart/context plus the final dimension name. Runtime resolution:

1. An exact chart ID is checked first.
2. A chart name may match.
3. If the prefix is a context, every chart instance in that context contributes candidates for the final dimension.
4. Every candidate is scored against the alert's chart labels by counting equal key/value labels.
5. The candidate with the highest score wins. Equal scores are resolved by candidate traversal order, so avoid designs
   where a tie is possible.

Consequences:

- This is the natural way to compare related chart instances, but the matching labels must make the intended instance
  unique. A generic shared label such as only `component=ceph` is usually insufficient across many cluster instances.
- Like Pattern 1, the selected dimension value is the latest stored value, not a query over a time window.
- Fully qualified names containing punctuation must use `${...}` braces.
- A reference that resolves to no candidate fails as an unknown variable and produces `NaN`.

### Pattern 3 — an intermediate alert as a computed variable

Create a non-notifying helper alert when the required value itself needs a database lookup or multi-stage computation,
then reference that alert by name from the consuming alert:

```text
template: example_window
      on: app.work
   lookup: average -1h of requests
     calc: $this
      to: silent

template: example_consumes
      on: app.current
   lookup: average -1m of requests
     calc: $this / $example_window
     warn: $this > 1
      to: sysadmin
```

Runtime facts:

- Every running alert with the referenced name is a candidate.
- Candidates are selected by the same equal-label score used for cross-chart variables.
- The selected candidate contributes its current alert value (`rc->value`), after that helper's own lookup and `calc`.
- This is the only one of the three patterns that can combine database-window results such as “max of the last hour of X
  with the average of the last minute of Y”.
- Health evaluates all lookup/calculation phases before warning/critical phases, but helper snapshots are published as
  each alert completes. A consumer with a different cadence can therefore read the helper's previous published value on
  its first beat or after cadence drift. Align `every:` or disclose this timing difference.
- Prefer `to: silent` on helpers and document why they exist; a helper is instrumentation, not a second incident owner.

### Choosing a pattern

- Same chart, latest values: Pattern 1.
- Different charts/contexts, latest values: Pattern 2.
- Any input needs a database window or a staged calculation: Pattern 3.
- Never use Pattern 3 merely to avoid a qualified dotted reference; its added timing and lifecycle coupling must earn its
  place.
- Never combine multiple RRDSET instances into one infrastructure-level alert on the alert side. If no source-owned chart
  provides the required aggregate, that is a collector/profile/framework gap, not an alert-expression workaround.

## Model The Lifecycle Truthfully

| Source situation | Alert-lifecycle consequence |
|---|---|
| Valid numeric input and both conditions are false | `CLEAR` |
| Valid numeric input and warning/critical condition is true | `WARNING` or `CRITICAL` |
| Some values exist in a lookup window and some are NULL | The lookup continues using numeric values; NULL is not automatically a failed condition |
| Collector misses a collection while the chart remains live | A runnable alert may evaluate a lookup based on stored data; no fresh sample is fabricated |
| Lookup runs and its selected window has no usable values | `$this` is `NaN`; the final state is `UNDEFINED` only when the condition preserves it |
| A lookup's newest stored sample is too old for its runnable-history gate | The health loop skips that evaluation; it does not manufacture `CLEAR` or guarantee an `UNDEFINED` transition |
| Collector knows the entity is gone and obsoletes the chart | The alert enters `REMOVED`, not an ordinary zero/CLEAR recovery |

Important rules:

- **A collection failure is not disappearance.** Collectors MUST preserve a gap when they cannot measure and MUST obsolete
  only an entity they know is gone. Do not add a false zero to make an alert recover.
- **A chart can remain alert-eligible during a gap.** The health loop skips an obsolete chart, but does not require a
  newly collected value for every scheduled evaluation. `$last_collected_t` and `$update_every` expose freshness when an
  alert is the deliberate stale-collection owner.
- **A live chart does not guarantee every lookup remains runnable forever.** For a relative database lookup, the health
  loop eventually skips evaluation when the newest stored point is too old for the requested window plus its bounded
  update-interval tolerance. Skipping evaluation preserves the prior alert state; it is different from evaluating an
  all-null result to `UNDEFINED`.
- **Obsoletion ends the instance alert.** Once the collector knows an entity is gone and obsoletes its RRDSET, the health
  rule no longer evaluates that instance. Do not attempt to emulate an infrastructure-level continuation in another
  alert; collect a source-owned aggregate instance if that is the required product signal.
- **A null result does not fabricate healthy state.** The health loop assigns `NaN` to an empty lookup result. A condition
  that itself evaluates to `NaN` is `UNDEFINED`.
- **Comparisons can consume `NaN`.** In the expression evaluator, `NaN` is false in boolean contexts and a comparison
  such as `$this == 0` produces a finite false result. That can yield `CLEAR`, not `UNDEFINED`. If the contract requires
  `UNDEFINED`, make and test an expression path that preserves `NaN`; never assume a comparison does so.
- **Some NULLs are not all NULLs.** Query aggregation accepts numeric points and omits non-numeric ones. Do not assume a
  partial collection gap invalidates a window unless the current runtime and the approved product contract say it does.
  A general strict-coverage rule is shared-framework work, not an alert-file shortcut.
- **Do not duplicate freshness ownership.** Use `$now - $last_collected_t` for an explicit collection/staleness alert only
  when no existing generic collection-failure alert already owns the incident. A data-state alert normally owns the
  measured condition, not the collector outage.

For a data-state alert that MUST become `UNDEFINED` when its lookup is non-finite, use and test this condition shape:

```text
warn: ($this == nan or $this == inf) ? (nan) : (<numeric predicate>)
```

Use `crit:` in the same way. This preserves the non-finite result only; it does not turn a partial-null window into a
collection-failure alert or replace a known-obsolete chart's `REMOVED` lifecycle.

## Adapt Timing To Netdata

### Current State

Use `calc` when the source's current value is the full condition. Test start-up, missing input, normal recovery, and chart
obsoletion separately. `delay:` controls Netdata alert transition/notification hysteresis; it is not a general substitute
for a Prometheus `for:` duration.

## Control Flapping With Three Combinable Layers

Stability is a signal-shaping problem, a threshold-boundary problem, and a transition-confirmation problem. Address them
deliberately in that order. Do not label every anti-flapping technique “hysteresis”: only `delay:` postpones an alert
transition.

### Layer 1 — stabilize the queried value

Use `lookup:` to make `$this` a stable aggregate over an explicit observation window:

```text
lookup: average -5m unaligned of latency
```

- `average` smooths noisy utilization, rate, latency, and utilization-like signals.
- `min` requires every observed numeric sample to remain active, appropriate for persisted binary fault states.
- `max` preserves worst-case excursions when the incident is defined by peaks.
- `countif` expresses percent-of-observed-time semantics.

This reduces noise but cannot prevent a stable aggregate from hovering near one threshold. A 5-minute average near 100 ms
can still cross a 100 ms threshold repeatedly.

### Layer 2 — separate raise and clear thresholds

For policy thresholds whose signal may hover near the boundary, use a state-dependent predicate:

```text
warn: $this > (($status >= $WARNING) ? (90) : (100))
crit: $this > (($status == $CRITICAL) ? (95) : (99))
```

This changes one threshold into two thresholds:

- clear-to-warning raises above 100;
- active-warning clears below 90;
- critical can independently use another raise/clear pair.

Important semantics:

- This is **not hysteresis** and does not delay any transition.
- The alert continues evaluating at its normal `every:` cadence.
- It only changes the boundary used by the current status, so a genuine crossing of the recovery threshold acts
  immediately.
- Preserve a non-finite guard around the complete expression when `UNDEFINED` must remain possible.

Prefer this when independent raise/clear boundaries are meaningful and the signal is expected to linger near one boundary.
Do not use it to redefine a categorical exact condition into a policy band.

### Layer 3 — confirm transitions with true hysteresis

Use `delay:` only when a transition must remain selected for a duration before Netdata executes the transition
notification:

```text
delay: down 5m multiplier 1.5 max 1h
```

- `up` delays a state escalation; `down` delays a recovery/de-escalation.
- `multiplier` grows the delay when the state changes during the delay.
- `max` caps the accumulated delay.

This is the only layer that postpones an alert transition notification. It can suppress rapid clear/reactivate
notification cycles, but it can also postpone a real transition. Use it sparingly and record the expected transition
delay in the alert contract.

### Selection procedure

1. Establish whether the incident is categorical, threshold policy, or derived arithmetic.
2. Select the smallest truthful `lookup:` window and aggregation first.
3. For a noisy policy threshold, choose explicit raise/clear thresholds before adding transition delay.
4. Add `delay:` only when rapid transition notifications are independently harmful and later notification is acceptable.
5. Test each layer: input noise, boundary crossing, recovery, reactivation, non-finite input, partial gap, and the exact
   expected notification time.

### Persistence Intent

Treat another system's `for: D` as an operator intent to suppress transient conditions. It is not a requirement to emulate
that system's pending-state machine. Choose the closest safe Netdata-native behavior from the source's actual numeric
state space, set an explicit `every:`, and disclose the differences:

- For a binary source where **1 means the active fault**, `min -5m unaligned` with an active predicate is true only when
  every **observed numeric value** in the selected window is active. This is a Netdata observation-window adaptation, not
  Prometheus `for: 5m`.
- For a binary source where **0 means the active fault**, use the complementary aggregation/predicate that requires all
  observed values to be zero; consult the lookup reference and prove it with state-sequence tests.
- For a tri-state or enumerated source, do not apply a binary `min`/`max` rule by analogy. Use an exact predicate over the
  full state space. For example, `countif(!=target)` returns the percentage of observed values outside `target`; zero means
  every observed value matched.
- `min`, `max`, and `countif` operate on the numeric samples they receive. Test active-to-other-state transitions in both
  directions and record the intended partial-gap behavior.
- A new chart may become runnable with up to one chart update interval less history than the requested lookup window.
  Therefore an already-active condition can raise up to roughly one source beat before `D` after chart creation. Do not
  conceal this with a hand-authored "window complete" flag.
- A missing sample does not reset an observation window. Values before and after a partial gap may contribute to the same
  lookup. Collection-failure ownership remains separate unless the alert contract explicitly owns freshness.
- One health rule has one historical database lookup. A predicate combining multiple dimensions cannot gain exact
  historical persistence by looking up one dimension and combining it with the others' current values. Prefer, in order:
  a source-owned derived dimension when the persisted compound condition is essential; otherwise a truthful current-state
  adaptation; never a mixed-time formula that changes the incident meaning.
- Do not lengthen the window mechanically by one assumed collector interval. Collector cadence is configurable, and that
  does not repair partial gaps or create a source-engine pending state.

For every persistence adaptation, prove: chart startup, observed active history, each recovery state, each higher/lower
enumerated state, a partial source gap, a stale non-runnable lookup, collection resumption, chart obsoletion, and the
configured evaluation cadence. Never invent a `pending` state that Netdata does not expose, and never declare fidelity
from the look of an expression alone.

## Keep Ownership And Identity Non-Duplicating

- Search existing stock alerts before adding one. Reuse an existing alert only when it really owns the same logical
  incident; disclose routing/severity/lifecycle differences rather than silently relabeling it.
- Keep source collection failure, component/API collection failure, source data-state failure, and client-observed failure
  as separate owners when they identify different operator actions.
- Filter templates with chart labels only when they select the intended RRDSET instance without changing its identity.
  A current metadata label may be used as a filter only when the alert contract explicitly wants that current metadata;
  do not turn it into `instances.by_labels` merely to make filtering convenient. Exclude known named rules from generic
  fallbacks, including special sources whose recovery is chart removal rather than zero.
- Preserve ordinary zero recovery. Do not convert an active-to-zero source into disappearance, and do not treat a
  disappearing source as a normal CLEAR.
- Use the ordering guide for template/alarm precedence and user-versus-stock override behavior. Same-name definitions are
  an override mechanism; different names coexist and can therefore duplicate incidents.

## Validate The Actual Contract

Run the smallest relevant tests first, then the full affected suite. A complete alert change normally needs all applicable
items below:

1. Run `/usr/sbin/netdata -W healthconfigtest` for the built-in health parser and lookup suite.
2. Add or update a focused test that reads the shipped alert template and asserts its context, labels, lookup, units,
   cadence, expressions, routing, source ownership, and declared fidelity—not merely a copied expected string.
3. Test the signal's lifecycle through the real query/health runtime where practical. If a lower-level deterministic model
   is necessary, derive it directly from runtime timestamps and numeric-point selection; do not pass an arbitrary
   `windowComplete` flag or label skipped NULLs as continuous evaluation. Cover startup, active transition, normal zero
   recovery, a true all-null query, partial-null behavior, stale non-runnable behavior, collection resumption, known
   disappearance/`REMOVED`, and label identity.
4. For an expression that relies on `NaN`, test the evaluator result directly. Check both direct `NaN` propagation and any
   comparison/conditional branch; do not infer the result from ordinary floating-point intuition.
5. Run collector/profile validation when the alert depends on a collector or profile change. Include source absence and
   collision-bearing labels where relevant.
6. Search for same-incident alerts, duplicate contexts, old names, fallback overlap, and generated artifacts. Record the
   result in the SOW.
7. For an externally sourced alert pack, commit a source-pinned mapping that records the original condition, scope,
   severity, persistence intent, supported releases, Netdata owner, adaptation, and known differences. Tests MUST consume
   that mapping rather than restating the intended result independently.
8. Validate every published configuration example through the same job-construction prerequisites users need. In
   particular, a job referencing `vnode:` is incomplete unless the example defines or clearly links the required vnode.
9. Run `git diff --check` and the project-required validation/review gate before claiming completion.

`healthconfigtest` runs built-in health parser and lookup cases. It does not load every stock health template, prove that a
template attaches to the intended chart, or prove the runtime lifecycle. Cover those separately with source-aware contract
and transition tests.

## Completion Check

Before requesting review, confirm all of the following:

- The alert has exactly one logical owner and a stable scope.
- Its NIDL instance map shows one monitored component and one instance-level alert target; any required aggregate is a
  source-owned higher-level RRDSET, not an alert-side merge.
- Its source state values, gaps, absence, and recovery have been tested rather than assumed.
- Its `NaN`, `UNDEFINED`, `CLEAR`, and `REMOVED` transitions match the recorded contract.
- Its timing is the closest safe Netdata adaptation, with startup/gap/stale differences disclosed; `delay:` is not
  standing in for another engine's persistence state machine.
- Notification defaults follow the approved product policy.
- No shared health/query behavior was added or assumed without the required separate scope approval.

## Authoritative References

- Syntax, variables, lookups, and stock patterns: `src/health/REFERENCE.md`
- State model and missing-data summary: `src/health/README.md`
- NIDL component/instance/dimension/label model: `docs/NIDL-Framework.md`
- Template/alarm and user/stock precedence: `src/health/alert-configuration-ordering.md`
- Alert eligibility, lookup execution, and `REMOVED`: `src/health/health_event_loop.c`
- `$now`, `$last_collected_t`, `$update_every`, dimension freshness, same-chart variables, cross-chart/context
  variables, alert variables, and label-score selection: `src/health/health_variable.c`
- Equal-label score implementation: `src/database/rrdlabels.c:rrdlabels_common_count()`
- Query gaps and grouping: `src/web/api/queries/query-execute.c` and the relevant grouping implementation
- `NaN` expression semantics: `src/libnetdata/eval/eval-evaluate.c`
