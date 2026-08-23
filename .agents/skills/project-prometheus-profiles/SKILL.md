---
name: project-prometheus-profiles
description: Create, review, validate, prove, iterate, or install Netdata Prometheus collector chart profiles (`go.d/prometheus.profiles/*.yaml`). Use for exporter dashboard design, profile schema/runtime behavior, selector/relabel/fallback policy, chart coverage and cardinality, stock semantic proof artifacts, or live profile verification.
---

# Prometheus profile authoring

A profile is an operator dashboard design backed by source evidence. It is not a mechanical metric-name translation.
The author decides the useful entity grain, chart comparisons, cardinality boundary, hierarchy, and collection policy;
the validator proves only contracts that code can establish.

## Choose the workflow

Read only the references needed for the request:

| Work | Required references |
|---|---|
| Create or redesign any profile | `chart-design.md`, `profile-schema.md`, `metric-types.md` |
| Capture private evidence | `how-tos/capture-metrics-dump.md` |
| Create or change a stock profile | all three design references, then `ownership-proof.md` and `proof-authoring.md` |
| Build stock fixtures | `how-tos/build-synthetic-fixture.md` and `proof-authoring.md` |
| Review a schema/runtime question | the relevant reference plus the authoritative repository document named there |
| Install or live-test a profile | the delivery section below; read `sqlite-metadata-reset.md` only if reset is proposed |

Repository architecture and runtime documents are the final authority when the skill and code disagree:

- `src/go/internal/promprofile/README.md` — framework boundary, authority model, package graph, and replay flow.
- `src/go/plugin/go.d/collector/prometheus/profile-format.md`
- `src/go/plugin/go.d/collector/prometheus/README.md`
- `src/go/plugin/go.d/collector/prometheus/relabel/README.md`
- `src/go/plugin/framework/charttpl/README.md`

Read `docs/NIDL-Framework.md` when choosing or reviewing a monitored component, instance grain, dimension set, or label
role. Then use the chart-template reference for the Prometheus-specific identity, label-promotion, aggregation, and
hierarchy contract that realizes that NIDL model.

## Authoring workflow

### 1. Establish the evidence boundary

- Inventory metric families, types, labels, optional modes, and source revisions.
- Separate facts:
  - exposition facts: names, `HELP`, `TYPE`, labels, and observed values;
  - source facts: lifecycle, units, label domains, relationships, and optionality;
  - design judgments: entity grain, chart composition, hierarchy, exclusions, and presentation.
- Treat a captured endpoint as private input. Do not commit it unless it was deliberately sanitized from public source
  contracts. Never commit credentials, customer identities, private endpoints, or deployment data.
- Missing source evidence is a real limitation. Do not turn one observed fixture into a universal exporter contract.

### 2. Design the operator model before YAML

Start with the questions an operator needs answered, in causal order:

1. Is the service available and doing useful work?
2. What load is it serving?
3. Is latency, error rate, or saturation worsening?
4. Which bounded entity or category explains the problem?
5. Which resource or dependency is responsible?

For each view, state:

- one operator question;
- one semantic entity grain;
- the smallest stable identity that creates that view;
- the labels compared as dimensions;
- the labels retained only for filtering/grouping;
- the labels intentionally omitted and the reducer that makes the omission truthful;
- the exact source signals, units, lifecycle, and relationship between dimensions.

Do not create duplicate aggregate and detailed views when Netdata can derive the aggregate by grouping the detailed
charts. Choose the finest operator-useful grain whose cardinality and churn remain acceptable.

### 3. Classify labels and cardinality

Assign every relevant label one or more explicit roles:

- **Required identity:** `instances.by_labels`; missing/blank means the series does not route.
- **Optional identity:** `instances.optional_by_labels`; present nonblank values refine the chart ID, while absent values
  use the base chart. Use only for a stable, operator-useful, bounded axis.
- **Dimension:** `name_from_label` or selector-specific static names; use for bounded comparable aspects such as CPU mode,
  read/write operation, or a closed state domain.
- **Promoted metadata:** `label_promotion`; use for stable useful labels that do not define identity.
- **Routing only:** label matchers select a subset without exposing the label as identity or metadata.
- **Omitted:** state the lost comparison and select an explicit reducer when several source series can collide.

Omitting `label_promotion` enables automatic intersection. When a high-cardinality or irrelevant label must never become
chart metadata, use `label_promotion: []` or an explicit safe allowlist; do not rely on one fixture having several values.

Estimate chart and dimension cardinality from the exporter contract, not only the fixture. Raw user IDs, IP addresses,
request IDs, URLs, arbitrary exception text, hashes, and similar axes are normally too high-cardinality for chart identity
or dimensions. Preserve useful values as labels only when they remain stable and operationally meaningful.

### 4. Choose truthful aggregation

Aggregation occurs only when multiple routed series render to the same chart ID and dimension. It does not reduce scrape
or store cardinality.

- Use `sum` for additive counters, histogram components, and gauges whose source meaning is an additive stock.
- Use `avg` for a typical value of a non-additive gauge only when an unweighted mean is meaningful.
- Use `min` or `max` for extrema and deliberately reduced 0/1 status populations.
- Preserve complete identity when no reducer preserves meaning.
- Never merge gauges and counters into the same rendered dimension. A chart may contain distinct authored dimensions of
  different kinds when the shared comparison is intentional; with no override, each dimension keeps its runtime-derived
  algorithm.
- Summary quantiles are not mergeable. Do not claim a global quantile from per-source quantiles.
- A chart has one reducer for all dimensions. Split views when dimensions require different reducers.

Omitting `aggregation` means `sum` for runtime compatibility. In stock profiles, write it explicitly whenever a deliberate
many-to-one projection can occur, including deliberate sum; omit it for collision-free routes.

### 5. Choose collection policy

Keep responsibilities separate:

- `match` selects profiles automatically and defines the profile's source namespace.
- Profile `relabeling` normalizes or drops exporter-owned families after profile selection. Use it to recover bounded
  identity encoded in metric names, normalize label values such as status classes, remove established useless families
  such as a source-wide `*_created` class, or implement another source-backed transformation.
- Profile `fallback_type` classifies untyped scalar families owned by the exporter.
- `autogen.selector` controls only generic fallback charts after profile processing. It matches the final family name and
  labels; it does not discard samples or change authored charts.
- Job `selector`, job relabeling, and job fallback policy are deployment/user policy. They may be used in user jobs but
  MUST NOT be hidden prerequisites of a contributed stock profile.

Relabel only when the transformation has a source-backed contract and a bounded result. Do not use relabeling as a
substitute for a chart-engine identity or aggregation decision. Profile normalizers run once per original source family;
all selected templates consume the same final stream, so one profile can affect another selected profile.

Unknown future families inside a wildcard profile namespace must remain eligible for generic fallback. A bounded,
source-proven alias or normalization may intentionally route a future input to an authored metric; prove that branch with
an explicit future input when the validator cannot derive it.

### 6. Encode the profile

- Use nested groups to express hierarchy. Child `context_namespace` segments join with `.` and child `family` segments
  join with `/`.
- Use base units. Convert bytes to bits only when the operator convention is bandwidth in bits/s; otherwise multiplier and
  divisor are usually unnecessary.
- Omit `algorithm` normally. Runtime counter kind resolves to `incremental`; gauges and other kinds resolve to `absolute`.
  Override only for a deliberate authored interpretation.
- Omit `type` for the normal `line` chart and for histogram buckets, whose runtime type is `heatmap`.
- Use explicit `area` only for deliberate filled-magnitude meaning.
- Use explicit `stacked` only when dimensions are an exact disjoint, exhaustive, additive partition of one whole.
- Status values are dimensions. Use the source's closed state mapping; do not turn every state into a chart instance.
- `_info` families are metadata signals. The writer skips gauge families with that suffix, so their labels cannot be
  charted or promoted. Reuse metadata only when the exporter also exposes it on a writer-eligible series.

Contributed stock profiles MUST keep authored YAML minimal and predictable:

- set chart `priority` only when operator navigation requires deliberate section ordering; otherwise omit it so the chart uses the runtime default;
- omit explicit chart `id` when the context-derived ID is sufficient;
- avoid `instances.by_labels: ['*']`; use a source-backed explicit identity;
- omit lifecycle caps; stock coverage must not depend on silently dropping observed or future entities;
- omit redundant `options.float` when the runtime metric is already floating point;
- omit redundant `algorithm`, `type`, multiplier, and divisor defaults.

The runtime format permits those fields for user profiles. The restrictions above are stock contribution policy, not
claims that the parser lacks the feature.

### 7. Run the objective validator

From the repository root:

```bash
.agents/skills/project-prometheus-profiles/scripts/validate-profile.py \
  --profile /path/to/profile.yaml \
  --dump /path/to/metrics.prom \
  --job /path/to/job.yaml \
  --output text
```

Use repeatable `--support-profile` arguments when the candidate composes with other profiles. Relative paths are resolved
from the caller's directory. A user profile may use a minimal or deployment-specific job. Stock validation uses the jobs
declared by its proof cases.

`PASS` proves schema and the exercised production collector/planner/emitter path. It does not prove operator usefulness,
source semantics, cardinality outside the evidence, or that a relationship is additive. Resolve warnings with evidence;
do not mechanically silence them.

Before semantic review, render the actual family table of contents:

```bash
.agents/skills/project-prometheus-profiles/scripts/profile-toc.py \
  /path/to/profile.yaml \
  --app application-name
```

The helper prints the operator-visible family tree with contexts and effective priorities, then emits advisory UX
warnings. It is not a gate and does not make design decisions. Investigate each warning and either repair the hierarchy
or record why the warning is intentional:

- all top-level families sharing a prefix usually repeat the application/root;
- a leaf with more than 15 contexts usually needs intermediate owner structure;
- a one-context leaf may be unnecessary structure or a merge candidate;
- a one-character family segment is often a slash-containing label such as `I/O` split accidentally into `I` and `O`.

Do not remove structure solely to silence the warning when the parent is an operator entity, a module boundary, or a
release contract; do not add structure solely to divide by metric type.

### 8. Perform semantic review

After objective validation, review the result as an operator:

- Does the overview answer availability, load, errors, latency, and saturation before internals?
- Does every chart answer one question with honest units and scale?
- Are identity and dimension domains stable and bounded?
- Does every omitted label have a truthful reduction or a reason no collision can occur?
- Can charts with the same context be aggregated meaningfully in Netdata?
- Does nesting produce the intended family hierarchy and context path?
- Are optional modes and missing labels handled without silently losing useful metrics?
- Are relabeling, fallback classification, and exclusions owned by source semantics rather than one fixture?

For a stock profile, continue with `proof-authoring.md`. Stock work is incomplete until the source contract, profile
design, replay descriptor, fixtures, and production profile reconcile.

## Stock metadata examples

A stock integration example should demonstrate that profile auto-selection is sufficient. Do not add `profiles`, `app`,
job `selector`, job relabeling, or job `fallback_type` merely to make the stock profile work. Those fields remain valid for
deployment-specific jobs, but a stock example containing them is evidence that profile ownership is incomplete.

When editing `metadata.yaml`, use the `integrations-lifecycle` skill and regenerate its derived artifacts through the
repository workflow. Integration files under `integrations/` are generated and should not be hand-edited for this work.

## Delivery and live verification

- Put contributed profiles under
  `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/<name>.yaml`.
- Keep reusable runtime/instrumentation profiles independent. A service profile may declare them as support profiles in
  `PROFILE-DESIGN.composition.supports`; do not duplicate their charts.
- Install a user profile under the configured user profile directory and verify collector profile selection, advancing
  chart values, chart identity, labels, hierarchy, and cardinality against a live target.
- Do not reset Netdata's SQLite metadata as routine iteration. Existing identities expire through lifecycle/retention.
  Read `sqlite-metadata-reset.md` and obtain explicit production approval before destructive reset work.

## References

- `chart-design.md` — operator model, entity grain, labels, reducers, hierarchy, presentation, and cardinality.
- `profile-schema.md` — profile fields, runtime effects, stock-policy boundary, and relabeling consequences.
- `metric-types.md` — parser/writer behavior and per-type design choices.
- `ownership-proof.md` — source ownership inventory and reconciliation method.
- `proof-authoring.md` — exact stock artifact responsibilities, schemas, examples, and verification.
- `how-tos/capture-metrics-dump.md` — safe private evidence capture.
- `how-tos/build-synthetic-fixture.md` — public source-complete fixture construction.
- `sqlite-metadata-reset.md` — destructive metadata-reset boundary.
- `scripts/validate-profile.py` — compatibility launcher for the authoritative Go validator.
- `scripts/profile-toc.py` — render the operator family ToC and report advisory hierarchy UX warnings.
- `scripts/proof-bundle.py` — stock proof catalog and replay wrapper.
