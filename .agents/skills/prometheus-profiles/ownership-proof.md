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
type, signal role, or unit. A completed checklist does not expose that contradiction. Selector-level traceability does.

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

## Account for every writer-capable family

For every source family the writer could retain before the proposed job policy removes it, make these facts explicit. The
layout may be a table, structured bullets, or another readable form; the evidence fields are mandatory, not the formatting.

- **Source family:** Exact family name and Prometheus type/shape.
- **Owner:** Closest entity, capability/module, operation, stage, or deliberate service/runtime boundary that explains the
  signal.
- **Entity type and identity:** What one series describes and the smallest stable identity-label set, including parent labels.
- **Signal role:** Workload, outcome, error, latency, saturation, capacity, utilization, resource, configuration, or another
  domain role.
- **Observation population:** What one increment, gauge value, distribution observation, or state represents and when it is
  produced.
- **Unit algebra:** Raw observed unit, temporal algorithm, conversion, rendered unit, and exact counted or measured object.
- **Label roles and cardinality:** Identity, bounded dimension, promoted metadata, routing-only, or intentional aggregation.
- **Evidence and uncertainty:** Dump evidence, authoritative source/documentation evidence, and unresolved limits.
- **Destination or exclusion:** Intended displayed family/chart, or one binding exclusion case plus the operator question lost.

Do not collapse multiple source families into one ledger row when their owners, populations, identities, or unit algebra differ.
Shared units do not prove shared ownership. Shared ownership does not prove that values belong on one axis.

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

After authoring, run the objective validator in text or JSON mode. Its `authored_mapping` is generated from the effective merged
template, including inherited instance identity, and remains in YAML source order.

Compare every mapping entry with `OPERATOR-MODEL.md`:

1. Map every exact dimension selector back to its source-family ledger entry.
2. Confirm the displayed family is owned by the entity, capability, operation, or stage recorded for that source evidence.
3. Confirm the effective `instance_by_labels` describes the recorded entity type and retains required parent identity.
4. Confirm chart title, context, units, algorithm intent, naming mechanism, and priority tell the same operator story.
5. Re-audit every first- and second-level family using the emitted mapping, not the intended prose.

A contradiction requires one of two honest outcomes:

- change the profile to match the researched operator model; or
- revise the proof with new source-backed reasoning and re-evaluate all affected families.

Do not rationalize a metric-form taxonomy after authoring. Do not declare completion from checked boxes, a hand-written chart
summary, or validator `PASS` alone. Completion requires selector-level agreement between the proof and the emitted mapping.
