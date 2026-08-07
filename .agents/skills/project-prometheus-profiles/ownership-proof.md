# Operator-ownership proof

## Purpose

Create `OPERATOR-MODEL.md` in the deliverable directory **before** authoring profile YAML.

This artifact is not a dashboard template and does not prescribe one universal navigation tree. It preserves the semantic
reasoning that flat metric exposition cannot express:

- what operators manage;
- which component, stage, operation, or entity can cause each condition;
- what one series or observation describes;
- why each signal belongs at one place in the dashboard rather than another.

Without this proof, an author can describe a good application model and then accidentally emit a profile organized by metric
type, signal role, or unit. A completed checklist does not expose that contradiction. The exact source inventory and replay
reconciliation do.

## Model the domain before sorting metrics

Research and state the operator model independently of the metric inventory:

- **Entity containment:** Which entity types contain or manage which descendants? State the stable identity labels at each
  level and how child identity retains parent identity.
- **Capabilities and modules:** Which independently diagnosable functions own work, state, resources, or policy?
- **Operations and processing stages:** What enters, changes, waits, succeeds, fails, or leaves? Identify stage boundaries and
  hand-offs.
- **Operator questions:** What does an operator expect to see about each entity, capability, operation, or stage?

Metric names can suggest research questions, but they MUST NOT define this model by themselves. A prefix, suffix, unit, or
Prometheus type is evidence about a signal, not proof of its causal owner.

Draw the capability and processing-flow map before assigning source families.
Include optional modules and hand-offs in the declared support scope even when
the available deployment does not enable them.

For each operator owner, record whether workload, outcomes, errors, latency,
saturation, capacity, and resource evidence is:

- observed in a real exposition;
- defined by source but available only in a synthetic fixture;
- not exported by the application;
- intentionally delegated to another integration;
- unresolved after source/documentation research.

This diagnostic completeness matrix does not require every role to exist. It
prevents one quiet deployment from making an entire capability or failure mode
disappear from the operator model without explanation.

## Account for every writer-capable family

For every in-scope source family the writer could retain before the proposed job
policy removes it, make these facts explicit in `SOURCE-INVENTORY.tsv`. Include
source-defined optional families absent from the observed deployment. The exact
ledger belongs there once; `OPERATOR-MODEL.md` states the reusable domain rules
and MUST NOT copy the rows.

- **Source family:** Exact family name and Prometheus type/shape.
- **Owner:** Closest entity, capability/module, operation, stage, or deliberate service/runtime boundary that explains the
  signal.
- **Entity type and identity:** What one series describes and the smallest stable
  identity-label set, including parent labels.
- **Signal role:** Workload, outcome, error, latency, saturation, capacity, utilization, resource, configuration, or another
  domain role.
- **Observation population:** What one increment, gauge value, distribution observation, or state represents and when it is
  produced.
- **Cross-family relationship:** Whether the family is a whole, partition,
  subset, overlap, alias/replacement, or independent population relative to
  related families. Record source-defined sums/equalities and explicit
  non-additivity.
- **Unit algebra:** Raw observed unit, temporal algorithm, conversion, rendered unit, and exact counted or measured object.
- **Label roles and cardinality:** Identity, bounded dimension, promoted metadata, routing-only, or intentional aggregation.
- **Availability gate:** Version, configuration, feature, connector, mode, or
  lifecycle condition controlling registration or updates.
- **Evidence and uncertainty:** Observed snapshot, authoritative
  source/documentation, source-derived synthetic, comparative, or unresolved
  evidence and the exact limitation of each.
- **Destination or exclusion:** Intended displayed family/chart, or one binding
  exclusion case plus the operator question lost.

Do not collapse multiple source families into one ledger row when their owners,
populations, identities, or unit algebra differ. Shared units do not prove
shared ownership. Shared ownership does not prove that values belong on one
axis.

Use stable population identifiers when several families share the same noun.
For example, frontend requests, internal work items, emitted choices, retries,
and parser events can all be called “requests” while remaining non-comparable
populations.

## Resolve ownership conflicts

Use causal and operator evidence, not a fixed naming rule:

- **Observation location versus causal owner:** A signal observed at a boundary can describe one stage, an end-to-end operation,
  or the hand-off itself. Follow the update callsite and state which operator question the measurement answers.
- **End-to-end versus stage-local:** Place an end-to-end signal with the nearest common lifecycle owner. Keep stage-local work,
  delay, pressure, and resources with the stage that produces or consumes them.
- **Entity versus aspect:** A stable, filterable managed thing usually needs instance identity. A bounded comparison such as
  outcome or method may be a dimension. Label names alone do not decide the role.
- **Shared unit versus shared question:** Two rates can share an axis only when they count the same object for one coherent
  question. A common `/s` suffix does not turn unrelated operations or objects into one chart.
- **Source prefix versus operator boundary:** Exporters often use one prefix across several subsystems, endpoints, or entity
  levels. Preserve the operator boundary even when the metric namespace does not.

When two designs remain defensible, record the alternatives and why the chosen one better supports navigation, filtering, and
holistic diagnosis. This is model judgment, not a reason to omit the decision.

## Reconcile the emitted profile

After authoring, run the objective validator in text or JSON mode. Its
`authored_mapping` is generated from the effective merged template, including
inherited instance identity, and remains in YAML source order.

Reconcile the source-complete synthetic `authored_mapping` first so optional
selectors are included. The proof replay MUST compare its exact selector set
with `SOURCE-INVENTORY.tsv`. Then compare observed runtime evidence separately
and preserve which selectors, labels, and feature gates were actually seen.

Compare every mapping entry with the exact source inventory and the reusable
rules in `OPERATOR-MODEL.md`:

1. Map every exact dimension selector back to its source-family ledger entry.
2. Confirm the displayed family is owned by the entity, capability, operation, or stage recorded for that source evidence.
3. Confirm the effective `instance_by_labels` describes the recorded entity type and retains required parent identity.
4. Confirm chart title, context, units, algorithm intent, and naming mechanism tell the same operator story.
5. Re-audit every first- and second-level family using the emitted mapping, not the intended prose.

A contradiction requires one of two honest outcomes:

- change the profile to match the researched operator model; or
- revise the proof with new source-backed reasoning and re-evaluate all affected families.

Do not rationalize a metric-form taxonomy after authoring. Do not declare completion from checked boxes, a hand-written chart
summary, or validator `PASS` alone. Completion requires executable selector-level agreement between the source inventory and
the emitted mapping.
