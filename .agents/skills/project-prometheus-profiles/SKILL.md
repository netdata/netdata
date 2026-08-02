---
name: project-prometheus-profiles
description: Create, review, validate, iterate, or install Netdata Prometheus collector chart profiles (`go.d/prometheus.profiles/*.yaml`) from Prometheus exposition dumps. Use for exporter dashboard design, profile schema/runtime problems, selector/relabel/fallback policy, chart coverage, NIDL hierarchy, or profile installation and live verification.
---

# Prometheus profile authoring

A Prometheus profile is a dashboard design, not a metric-name translation.
It decides:

- what an operator sees first;
- which entities become chart instances;
- which labels become comparable dimensions or filterable metadata;
- which signals share a chart and scale;
- which raw series are intentionally excluded before charting.

Use model judgment for those decisions. Use deterministic tooling only for
facts that code can prove. A schema-valid but poorly reasoned profile is not a
good result; neither is a beautiful design that the runtime cannot materialize.

A request to create a profile delegates routine research and dashboard-design
judgment to the author. Complete the profile, job policy, reasoning summary, and
validation without asking the user to choose ordinary family names, chart
boundaries, identities, or priorities. Ask only when evidence cannot resolve a
real product boundary, the requested evidence is unavailable, or the next step
would change a production system. Otherwise, stopping for confirmation defeats
the purpose of using model judgment.

## Ground in the current implementation

Read these files **in full, without skimming, before authoring or materially
reviewing a profile**:

Repository-relative sources (resolve from the repository root):

1. `docs/NIDL-Framework.md`
2. `src/go/plugin/framework/charttpl/README.md`
3. `src/go/plugin/go.d/collector/prometheus/README.md`
4. `src/go/plugin/go.d/collector/prometheus/relabel/README.md`
5. `src/go/plugin/go.d/collector/prometheus/profile-format.md`
6. `src/go/pkg/prometheus/selector/README.md`
7. `src/go/plugin/framework/chartengine/README.md`
8. `src/go/pkg/metrix/README.md`
9. `src/go/tools/prometheus-profile-validation/README.md`

Skill-relative sources (resolve from this skill directory):

10. `profile-schema.md`
11. `metric-types.md`
12. `chart-design.md`
13. `ownership-proof.md`

The repository root is `../../..` from this skill directory. Do not resolve
skill-relative sources from that root, and do not assume the task's working
directory is the repository.

Why this is mandatory:

- The profile envelope, selector grammar, collector policy, metrix flattening,
  and chartengine planner are separate contracts.
- A plausible shortcut in one layer can be invalid in another. For example, a
  summary with only `_sum` and `_count` parses as a summary but the writer
  rejects it because there are no quantiles.
- These contracts evolve with the repository. This skill explains how to
  reason about them; the source documentation remains authoritative.

When a reference and current source disagree, follow current source and repair
the stale reference as coupled work.

## Know when the profile is complete

A profile is complete only when both its runtime behavior and its semantic
design are proved. Do not describe or deliver a candidate as complete while any
of these proofs fails:

- **Operator-model proof:** `OPERATOR-MODEL.md` records source-backed
  entities/containment, modules/capabilities, and operations/processing stages
  before YAML, then reconciles those decisions against the emitted profile.
- **Holistic-surface proof:** the declared version/configuration support scope
  accounts for every source-defined writer-capable family, including optional
  capabilities absent from the available deployment.
- **Fixture proof:** observed dumps and provenance-stamped source-derived
  synthetic fixtures prove their distinct evidence classes without presenting
  structural emulation as runtime observation.
- **Navigation proof:** every first- and second-level family has one coherent
  operator owner. The author can state what entity/function it represents and
  what work or state it performs, receives, produces, stores, routes, or
  controls.
- **Holistic-diagnosis proof:** available workload, outcome, error, latency,
  saturation, capacity, and resource signals remain with the function that can
  cause them, except for a deliberately small service-impact overview.
- **Unit proof:** every dimension on one axis has the same rendered algebra and
  the same counted or measured object.
- **Evidence proof:** every writer-capable family is curated or excluded only
  under one of the binding cases, with the lost operator question stated.
- **Runtime proof:** the exact profile, dump, and job policy pass the objective
  validator. The default is zero fallback and zero unmatched series; a narrow,
  explicit profile fallback allow/suppress boundary is valid only under the
  evidence and exclusion rules below and remains visible as a warning.

Why: each proof catches a different failure class. Runtime coverage cannot prove
that a `Latency` branch teaches causality; a good family tree cannot make
operations and their produced items the same unit; a curated dashboard cannot
recover a signal that job policy discarded. Model judgment chooses the design,
but none of these contradictions is an acceptable exercise of judgment.

## Research the application from evidence

Maintain five explicitly labeled buckets:

- **Observed snapshot fact:** metric names, `TYPE`, `HELP`, label keys/values,
  cardinality, and state present in a captured exposition.
- **Authoritative external fact:** exporter/application documentation or source
  that explains what a metric means or what optional surfaces exist.
- **Source-derived synthetic evidence:** structurally faithful, sanitized
  exposition constructed from authoritative registrations, update callsites,
  tests, and documentation.
- **Comparative evidence:** upstream/community fixtures, mirrored repositories,
  and other monitoring implementations used to discover terminology, operator
  questions, version drift, or missing cases.
- **Design judgment:** the operator story, hierarchy, instance identity,
  dimension choice, chart composition, and ordering.

The supplied dump is an observed inventory, not the complete application model
or support surface. Before designing, declare the bounded application/exporter
versions and configurations the profile will support. The default stock-profile
scope is the observed/supplied revision plus the current upstream revision used
at authoring time; record materially different contracts separately.

Research the entire declared surface, not only unfamiliar names:

1. Read the matching application/exporter documentation.
2. Search the matching source revision for metric registration and update
   callsites. Registration shows declared meaning; update sites show what one
   increment or observation actually represents, which labels are attached,
   and at what lifecycle scope.
3. Use upstream issues, release notes, or version history when a contract has
   changed.
4. Compare other monitoring integrations or dashboards for questions and
   terminology, but verify their assumptions against primary evidence.
5. Treat model memory and naming intuition as hypotheses until corroborated.

If the local setup omits source-defined features or metrics in scope, fetching
the matching source and documentation is REQUIRED. Inventory every registration,
type/shape, label, update population, feature gate, alias, and replacement, then
build sanitized synthetic fixtures that exercise the final profile. Use
[the synthetic-fixture workflow](how-tos/build-synthetic-fixture.md).

Use every research capability available for the task: supplied source trees,
web browsing, repository search, and mirrored open-source repositories. Search
upstream tests/fixtures and other monitoring solutions proactively; they are
recommended for discovering missing feature states, fixture shapes, operator
questions, and version differences. Do not stop at `HELP` when it leaves the
observation population, identity, reset behavior, or unit meaning ambiguous.
Conversely, do not copy another fixture's contract or dashboard's hierarchy
until the target application's primary source verifies it. Comparative
integrations may be stale, collapse labels, or compute a different quantity.

Do not present an inference as observed fact. One dump proves only one
configuration and moment:

- Zero values are still observed metrics and may matter under load.
- An optional feature absent from the dump is not runtime-validated. If it is
  in the declared support scope, create a provenance-stamped source-derived
  fixture and label that proof as synthetic.
- Do not invent metric names, label keys, or types from application concepts.

If authoritative evidence cannot establish an in-scope shape, record the
surface as unresolved and preserve generic fallback where safe. Do not fabricate
it, silently narrow the support scope, or claim holistic completion.

Generic fallback is not a substitute for research. A profile-level
`autogen.selector.allow` MAY preserve a source-backed dynamic family only when
the source names cannot be enumerated or normalized faithfully and each generic
chart still represents a valid operator signal. Keep the allowlist as narrow as
the source contract. Record the owner, identity encoded in the name/labels,
cardinality boundary, and why an authored selector cannot represent it.

If `TYPE` is missing, do not guess silently. The collector can recover only
selected untyped scalar families through `fallback_type`; it cannot reconstruct
histogram/summary structure that the exposition did not declare. Use
authoritative exporter evidence or obtain a better dump.

Treat dumps as potentially sensitive. Keep them in ignored `.local/` or user
temporary storage, and never copy customer/user label values into committed
fixtures or documentation. Committed fixtures MUST be sanitized synthetic
artifacts, not edited operational dumps.

### Do not impose privacy policy on exported metrics

A Prometheus profile MUST NOT decide that an already-exported metric or label is
unfit for monitoring because its name or value may identify an API key, user,
team, organization, tenant, endpoint, file, route, or other customer-owned
entity. The application exporter has already placed that data on the monitored
endpoint; the profile's responsibility is to represent it correctly for the
administrators operating that deployment.

Apply these rules:

- **Do not censor by label meaning.** A profile MUST NOT exclude a family, drop
  a label, aggregate away an identity, hash a value, or refuse metadata solely
  because the data appears private, sensitive, identifying, or customer-specific.
- **Preserve semantic identity.** Labels required to distinguish the entity
  measured by a counter, gauge, histogram, or summary MUST remain available to
  the chart design. This includes tenant and credential identities when the
  source metric is defined at that scope.
- **Keep policy deployment-owned.** Exporter configuration, job selectors,
  relabeling, Netdata access controls, and the deployment's administrators own
  any privacy or disclosure policy. A stock profile MUST NOT silently impose
  one on every deployment.
- **Evaluate the actual profile concerns.** Identity and label choices MUST be
  judged by source semantics, mathematical correctness, operator usefulness,
  stability, cardinality, lifecycle, and resource cost—not presumed privacy.

This rule is separate from repository artifact hygiene. Operational dumps and
real label values still MUST remain out of committed fixtures, documentation,
skills, and test output. Sanitizing durable development artifacts prevents
publishing a specific deployment's data; it does not authorize the shipped
profile to hide data from that deployment's own administrators.

## Model what operators expect to see

The central design question is:

> What do operators expect to see about this application, entity, module, or
> operation when they investigate it?

Do not begin with “where do the latency metrics go?” Every metric is some form
of workload, outcome, error, latency, saturation, capacity, or resource use.
Those roles explain an owner; they do not identify the owner.

### Turn flat exposition into Netdata metadata

Prometheus exposition presents a flat set of labeled time series. In common
Prometheus dashboard workflows, panel queries and dashboard layout supply many
of the relationships between those series.

A Netdata profile must encode those relationships into the metrics metadata:

- recursive `family` paths define navigation;
- `context` identifies the semantic chart type;
- `instances.by_labels` defines the complete unique key for each monitored
  entity instance;
- dimensions define bounded aspects compared on one chart;
- promoted ownership and descriptive labels provide filterable metadata; and
- priority defines the reading order.

The complete instance key can include an ownership path without changing the
semantic leaf type. For example, `{database, table}` may identify one table when
table names repeat, while the context and dashboard instance count still
describe tables. Dimensions deliberately remove bounded aspects such as
operation or status from that instance identity.

Netdata's generic dashboards organize charts from this metadata. The profile is
therefore not one hand-authored panel layout; it is the reusable semantic model
the dashboard follows. A shared prefix, unit, Prometheus type, or label does not
by itself establish a Netdata relationship.

### Discover the operator's mental model

Most applications combine three kinds of structure:

1. **Entities and containment:** service, cluster, server, database, table,
   index, workload, pod, container, endpoint, worker, device, or queue.
2. **Modules and capabilities:** scheduler, cache, router, storage engine,
   frontend, backend pool, executor, or another operator-recognized subsystem.
3. **Operations and processing stages:** accept, authenticate, queue, plan,
   read, write, execute, persist, replicate, forward, retry, and respond.

Research documentation and source to discover which structures operators
actually use. Model hand-offs and containment, not just a bag of nouns:

- What work or state does each entity/module/stage own?
- What does it receive, process, store, control, and hand off?
- What can it delay, reject, corrupt, exhaust, or saturate?
- At which entity level does an operator expect filtering to work?

Labels are evidence for this model, not an automatic answer. For every observed
label, determine whether it represents:

- stable leaf-entity identity;
- an ownership path, including whether it is needed to make the leaf key
  unique;
- a bounded comparable aspect/dimension;
- descriptive detail/promoted metadata;
- selector/routing information; or
- detail that is intentionally aggregated.

Use source semantics, observed label combinations, stability, and cardinality
to decide. A label named `handler`, `database`, or `pod` may identify an entity;
`status`, `method`, or `operation` may instead be a bounded aspect. Ownership
and descriptive details must be stable for the chosen instance; if they vary
across its dimensions, revisit the model rather than promoting an arbitrary
value. Do not infer the role from the label name alone.

### Classify the evidence against that model

Create `OPERATOR-MODEL.md` in the deliverable directory **before** profile YAML.
Classify every in-scope writer-capable family before grouping it, whether
observed or source-derived. The document's layout is up to the author, but these
evidence fields are mandatory:

- **Owner:** the entity, module/capability, or operation/stage this signal
  explains.
- **Entity type:** the thing one series describes.
- **Identity labels:** the smallest stable labels identifying that entity,
  including inherited parent identity.
- **Signal role:** workload, outcome, error, latency, saturation, capacity,
  utilization, resource use, configuration, or another domain role.
- **Observation population:** what one increment, observation, or gauge value
  represents.
- **Cross-family relationship:** whether the signal is a whole, partition,
  subset, overlap, alias/replacement, or independent population relative to
  similarly named families, including source-defined invariants and
  non-additivity.
- **Unit algebra:** raw unit, algorithm, conversion, rendered unit, and exact
  counted/measured object.
- **Label roles/cardinality:** leaf identity, ownership path, dimension,
  descriptive metadata, routing, or intentional aggregation. Record whether an
  ownership label is also required in the technical instance key.
- **Optional-label behavior:** whether each label is always emitted, optional by
  configuration/version, or conditional per series, and how the profile keeps
  the context and instance type valid when it is absent.
- **Availability gate:** version, configuration, feature, connector, mode, or
  lifecycle condition controlling registration/update.
- **Evidence/uncertainty:** observed, authoritative, source-derived synthetic,
  comparative, or unresolved support and its proof boundary.
- **Destination:** intended family/chart or one binding exclusion case.

This proof is not a fixed worksheet or dashboard generator. The model chooses
the domain owners and navigation. The required evidence makes that judgment
traceable and prevents metric names, suffixes, and units from becoming the
information architecture accidentally.

After YAML authoring, reconcile every selector against the validator's generated
`authored_mapping`. An intended hierarchy in prose is not evidence that the
emitted family, identity, units, and priority actually implement it. See
`ownership-proof.md` for the required source evidence, conflict reasoning, and
selector-level reconciliation.

### Group diagnostic roles under their owner

Keep an owner's available workload, outcomes, errors, latency, saturation,
capacity, and resource pressure together or in nearby subfamilies. For example:

- stage-specific latency belongs with that processing stage;
- end-to-end latency belongs with the operation or nearest lifecycle owner it
  spans;
- requested limits/options belong with admission or the operation they shape;
- resource pressure belongs with its consumer unless the resource is itself an
  operator-managed entity/subsystem; and
- measurements of the same object at different lifecycle stages remain with
  those stages unless the object is itself the operator entity.

HTTP is not automatically one owner merely because every metric counts HTTP
requests. If operators manage two endpoints, modules, routes, or processing
paths as distinct entities/functions, their requests should not be mixed into
one context merely because the final unit is `requests/s`. Conversely, method or
status can be dimensions when they are bounded aspects of one entity and one
operator question.

An optional bounded aspect MAY remain a dimension, but it MUST NOT be the sole
route for its metric. Prove optionality from authoritative exporter source or
configuration; dynamic naming alone does not prove the label can be absent. Use
complementary present/nonblank and missing/blank selectors in the same chart,
with a fixed `unclassified` fallback dimension. This preserves the context and
instance count whether the exporter includes the label or not. Do not use this
fallback to merge non-additive gauge states or to pretend that a missing
entity-identity label is available; see `chart-design.md` and
`profile-schema.md` for the exact pattern.

Audit the first two family levels without looking at chart names:

- They SHOULD describe entities, modules/capabilities, operations, or processing
  stages that operators recognize.
- A global `Latency`, `Throughput`, `Errors`, `Distributions`, `Parameters`, or
  unit-based branch usually describes a diagnostic role, not what owns it.
- Every signal MUST have an owner or an explicit service/runtime boundary before
  its role or unit influences placement.

Different authors may choose different defensible owners and ordering. The
guardrail forbids replacing operator reasoning with a metric taxonomy; it does
not prescribe one universal tree.

See `chart-design.md` for database, proxy, and Kubernetes-hosted microservice
examples, UX consequences, and conflict resolution. See `metric-types.md` for
the collector behavior of each Prometheus type.

## Design the collection policy with the profile

The profile cannot drop raw metrics. The effective dashboard is the combination
of:

- job `selector` filtering;
- ordered job `relabeling`;
- job `fallback_type`;
- the selected profile;
- writer rules and limits.

Choose the least surprising mechanism:

- Use `selector` for efficient name/label allow/deny filtering.
- Use relabeling when series or labels must be transformed, normalized, kept,
  or dropped by ordered Prometheus-compatible rules.
- Use `fallback_type` only for untyped scalar families whose semantics are
  known.
- Do not “cover” a metric by declaring it under `metrics:`. Coverage exists
  only when a runtime dimension selector actually routes its written series.

Keep exclusions conservative and explain the lost diagnostic capability.
Typical justified policy exclusions include creation timestamps or frozen
epoch metadata that the schema cannot transform, and deprecated families with a
validated replacement. Writer-skipped `_info` families are pipeline
limitations to document, not evidence that a job deny improved the profile.
“Not interesting” and “zero in this dump” are not reasons.

For every in-scope writer-capable family that answers a distinct operator
question, the default is to curate it. This includes source-defined optional
families proven structurally by the synthetic fixture. A job exclusion is
acceptable only when at least one of these cases is evidenced:

1. **Unrenderable raw form:** the available value cannot answer its question
   without an unsupported transform, such as `now - epoch`.
2. **Authoritatively superseded:** a supported replacement answers the same
   question across the intended versions, types, labels, and distribution
   population.
3. **Concrete collection hazard:** bounded evidence shows a cardinality,
   correctness, or resource risk that cannot be represented safely in the
   profile.
4. **Verified scope delegation:** another enabled integration owns and answers
   the same question, and this dashboard intentionally defines that boundary.

Writer-rejected families are pipeline limitations, not successful job
exclusions. “Dashboard focus,” “deep-dive metric,” “too many charts,”
correlation with another signal, or making validation pass are not exclusion
cases. Achieve focus with hierarchy and priority; filtering changes the
evidence available for troubleshooting.

Treat redundancy as a claim that needs proof, not as a visual resemblance:

- Current zero, constant, or equal values describe one scrape state; they do
  not establish an invariant across load, configuration, or exporter versions.
- Similar names or HELP text do not prove that types, labels, distribution
  schemas, update timing, or semantics are interchangeable.
- A replacement is credible only when authoritative exporter evidence and the
  observed structure support the same operator meaning over the profile's
  intended deployment scope.
- Separate component distributions do not reconstruct a total distribution.
  Once per-request correlation is lost, adding buckets or quantiles can answer
  a different question from the directly exported aggregate.

For every exclusion, state the operator question that becomes impossible to
answer and why that loss is acceptable. The validator's per-deny warning shows
observed impact; it deliberately does not decide whether the trade-off is good.
Do not write “no question lost”: even a creation timestamp or process-start
gauge answers an age/restart question. The profile may intentionally leave that
question to another integration, but only when that replacement or scope choice
is stated honestly.

When the endpoint policy matters, write a structured validation job file. Do
not validate with a different selector/relabel/fallback policy from the one
being recommended or installed.

Profile fallback suppression is an exclusion too. A non-empty
`autogen.selector.allow` rejects every unmatched series outside its allowlist,
and `autogen.selector.deny` can reject an allowed series. Either path MUST
satisfy one binding exclusion case above and document the lost operator
question. The samples remain in the collector store; only their generic charts
are suppressed. For objective completion, enumerate intentional exclusions in
`deny`; do not rely on absence from `allow`. The validator keeps an
allow-boundary rejection as an unmatched-series error, because it cannot
distinguish the intended boundary from a newly omitted family. It accepts
explicit denies only when the allow side admits all writer evidence and a
counterfactual planner run proves that removing the exact profile rule turns
every unmatched series into fallback.

## Encode the design

Use `profile-schema.md` as a navigation aid and the chart-template README as
ground truth.

Required authoring policy:

- Every chart MUST have an explicit positive `priority`.
- YAML family/chart order MUST mirror intended dashboard presentation order.
- Explicit priorities MUST NOT decrease as the profile is read in YAML order.
- Discrete work/event, count, state, and time charts MUST use `line`.
  `area`/`stacked` MAY be used only when fill represents physical volume,
  space, bandwidth, or I/O rather than merely categories that add to a total.
- Prefer unique, increasing priorities when the dashboard has a total order.
  A deliberate tie is valid when a total order is unnecessary, but it deserves
  review because runtime placement then falls back to unrelated chart IDs.

Why: omitted or zero priorities all become `70000`; the planner does not derive
priority from YAML order. Explicit values force the author to decide the
operator journey. Matching source and presentation order makes that reasoning
reviewable; decreasing priorities contradict the journey expressed by the
file.

Coverage and chart composition are separate decisions. A complete profile MUST
account for every writer-surviving flattened bucket, count, sum, quantile,
counter, and gauge series through authored routing, an explicitly justified
generic-fallback boundary, or one binding exclusion case. It MUST NOT
mechanically create one chart for every metric or three charts for every
histogram:

- A dimension selector can curate a series inside a chart that also contains
  compatible series from related families.
- Same-unit count rates can share a chart when they compare one coherent
  workload or causal question.
- Same-unit sum rates can share a chart when their rendered meanings and scales
  are genuinely comparable.
- Bucket shape, observation count, and observed-value sum remain different
  roles. Never combine them under one unit merely to reduce chart count.
- Every dimension in one chart MUST have the same complete rendered unit
  algebra, including the counted object. Renaming different objects to
  `events/s`, `operations/s`, `items/s`, or `observations/s` does not make them
  comparable; research the object and split the chart when the nouns differ.
- An operation and the objects produced by that operation remain different
  units. One operation may produce zero, one, or many objects; placing its
  counter beside object counters under the object's unit invents a conversion
  ratio that the exporter did not supply.

The operator question decides the chart boundary; the validator's routing
result decides whether every written series was accounted for. Zero autogen and
zero unmatched remain the normal result. Explicit fallback warnings require a
source-backed explanation; they are not permission to replace useful curation
with generic charts or silent suppression. Conflating coverage with chart
composition produces repetitive metric-type dashboards instead of useful
comparisons.

Keep contexts and IDs stable. Changing them creates new chart identities and
can strand historical metadata. Different source charts that render the same
chart ID can suppress one another; use distinct semantic contexts and validate
observed instance values.

Design `match` as an exporter-detection signature, not as a list of charted
families. Prefer exporter-unique families. Generic `process_*`, `python_*`, or
`http_*` families can be charted without putting them in `match`; including
them can make unrelated endpoints eligible for the profile.

Automatic selection needs only one family hit. Never broaden `match` to make a
coverage failure disappear: `match` chooses the profile, while group `metrics`,
dimension selectors, job policy, and writer behavior determine routing. The
validator forces exact selection, so changing `match` cannot repair its
curation result; diagnose the actual scope or selector failure instead.

## Run the objective gate

From `src/go`:

```text
go run ./tools/prometheus-profile-validation \
  --profile /path/to/profile.yaml \
  --dump /path/to/metrics.txt \
  --job /path/to/job-policy.yaml \
  --output text
```

The optional compatibility launcher
`scripts/validate-profile.py` invokes this same Go tool; it contains no
independent validation logic.

Run the gate on the exact profile, source-complete fixture, and structured job
policy being delivered. Re-run after every edit. Keep separate observed-dump
diagnostics or collector regressions; a partial real dump can legitimately leave
source-defined optional charts dead under this strict authoring gate.

A `PASS` proves, for that evidence:

- strict catalog/profile/template decoding succeeds;
- the real collector completes `Init`, `Check`, and a committed `Collect`;
- the real writer and flattening behavior are exercised;
- every written series is accounted for, normally with zero autogen and zero
  unmatched; any nonzero result is proved to be the candidate profile's narrow,
  explicit fallback allow/suppress policy and is reported as a warning;
- every authored chart and dimension materializes;
- every selected writer series carries the labels required by the chart's
  effective explicit instance identity;
- optional dimension-label fixtures route both present/nonblank and missing/blank forms
  to the same context and instance type without overlap or fallback autogen;
- isolated planning finds no observed cross-template collision or
  same-template instance-ID collapse;
- observed per-instance dimensions are not discarded by lifecycle caps or
  planner normalization;
- public chart emission finds no chart-ID, context, or dynamic-dimension
  normalization collision or omission;
- required runtime coverage counters are present and valid;
- every chart has an explicit positive priority and priorities do not decrease
  in source order;
- every chart exposes at least one visible dimension;
- histogram bucket charts use incremental observation-rate semantics; and
- unambiguously non-volume charts do not use filled presentation.

The report lists raw families absent after selector/relabel/writer processing
under `pipeline_excluded`; they are not misreported as chart coverage.
The `authored_mapping` section lists the actual source-ordered
selector-to-displayed-family mapping, including effective inherited instance
identity, units, algorithm intent, naming mechanisms, and priority. It is the
objective input to the operator-ownership reconciliation; it does not score
whether the chosen application model is good.
A job-policy exclusion summary shows how much otherwise writer-capable evidence
was removed before coverage was measured. A `PASS` over a deliberately reduced
denominator is mechanically valid but is not a complete dashboard unless every
exclusion satisfies the policy above.

Warnings are prompts for model review, not policy decisions. They include
explicitly allowed generic fallback, explicitly suppressed fallback series,
generic auto-selection signatures, observed labels with no authored role,
the job-policy exclusion summary, observed per-rule allow/deny impact, unused
metric declarations, authored/runtime heatmap divergence, ambiguous or
physical-rate filled charts, repeated sibling family paths, mixed leaf identity,
parent identity loss, sibling identity mismatch, sample-discarding relabel
actions, and incremental counter/distribution charts whose units omit rate
semantics. Each can be intentional, but its UX or diagnostic trade-off must be
explained. Do not mechanically add identity, promotion, dimensions, or unit
suffixes merely to silence a warning; explain intentional aggregation,
entity-level boundaries, and truthful unit algebra when they are correct.

The gate cannot judge whether the dashboard is useful. A source-derived fixture
can prove unseen metric structure and routing, but not live registration,
values, update behavior, or cardinality. Exact candidate selection also does
not prove that `match` safely auto-selects this profile against unrelated
endpoints. Read
`src/go/tools/prometheus-profile-validation/README.md` for the exact evidence
boundary.

## Perform the semantic review after `PASS`

Review the rendered design, not merely the YAML:

- Does every `authored_mapping` selector agree with its source-family owner,
  identity, population, unit algebra, and destination in `OPERATOR-MODEL.md`?
- Does the first screen answer the most urgent operator questions?
- Does the family tree express the entities/containment, modules/capabilities,
  and operations/processing stages that operators actually use?
- For each first- and second-level family, is the answer to “what do operators
  expect to see about this?” coherent without relying on a shared signal role,
  metric form, parameter kind, or unit?
- Does each owner keep its available workload, outcomes, errors, latency,
  saturation, capacity, and resources close enough for holistic diagnosis?
- Does every context represent one homogeneous instance type?
- Does each displayed leaf family contain one effective entity identity?
- Are dimensions bounded aspects rather than hidden entity explosions?
- Does every optional dimension label have a mutually exclusive fixed fallback
  in the same chart, and does the fallback describe only the unlabeled subset?
- Are ownership and descriptive labels stable for each chosen instance, with
  ownership included in the instance key only where needed for uniqueness or
  the identity lattice?
- Does descendant identity retain the parent labels and add only the labels
  needed for the narrower entity?
- Do sibling sections share the parent's identity where section-wide filtering
  is intended?
- Do chart titles promise only what the selected data computes?
- Are units, algorithms, and conversions honest?
- Does every dimension on a shared axis count or measure the same object, with
  no umbrella unit hiding different nouns?
- Are distribution shape, observation count, and observed-value sum kept as
  different semantic/unit roles rather than combined under one chart unit?
- Are smaller signals still visible, or flattened by a much larger dimension?
- Are stacked/area charts limited to physical volume, space, bandwidth, or I/O,
  with line charts used for rates, counts, ratios, latency, and state?
- Does each “by/per X” title identify the instance or dimension mechanism that
  actually preserves X, rather than aggregating it away?
- Does each exclusion identify the lost operator question, authoritative
  semantic evidence, and deployment scope over which the loss is safe?
- Are presentation order and priorities deliberately generic-to-detailed?

If the validator and semantic design conflict, diagnose the runtime fact first.
Do not weaken an objective gate to preserve an attractive but non-materializing
design. Conversely, do not let a heuristic warning mechanically override a
well-explained domain decision.

## Complete stock-profile artifacts

For a stock profile contribution, read the `integrations-lifecycle` skill and
assess the application catalog deliberately. The profile runtime catalog and
collector `metadata.yaml` are separate systems; one does not currently generate
the other.

Choose and record one metadata disposition:

1. Add/update a Prometheus application module when no equivalent integration
   page exists.
2. Update or cross-link an existing first-class integration when it already
   documents the same endpoint; do not create a duplicate catalog entry.
3. Keep only the generic Prometheus page when there is a concrete product reason
   that an application-specific catalog entry would be misleading.

When application-specific metadata is added or updated:

- keep `metadata.yaml` as the source of truth;
- distill the operator model into a brief public description of the entities,
  capabilities, and processing stages the profile organizes;
- place that brief in the integration overview rather than copying the complete
  family ledger;
- regenerate the integration page and required catalog documentation;
- validate the metadata/taxonomy disposition under the integrations lifecycle.

This requirement does not apply to a local user profile that is not being
contributed to the stock catalog.

## Deliver and install

Deliver:

- the profile YAML;
- the recommended structured job policy (or exact changes to an existing job);
- `OPERATOR-MODEL.md`, including the post-authoring selector-level
  reconciliation against `authored_mapping`;
- an evidence manifest identifying versions, source/documentation revisions,
  feature gates, observed versus synthetic families, and fixture provenance;
- sanitized source-complete fixture(s), validation inputs/hashes, and the
  authoritative `PASS` summary;
- separate observed-runtime and source-derived-synthetic validation claims;
- the stock integration metadata/generated-doc disposition when applicable;
- explicit unresolved evidence limitations; known source-defined optional
  surfaces in scope must not be deferred merely for lack of a local feature.

Do not install or reload production configuration merely because authoring
passed. Installation is a separate operational change:

- inspect the existing job/profile override and reference surface;
- plan the atomic cutover and rollback;
- obtain explicit production approval;
- verify hot reload/restart behavior and advancing chart identities.

Do not reset SQLite metadata as a routine iteration step. See
`sqlite-metadata-reset.md` for why stale metadata and old history are normally
left to lifecycle/retention, and for the exceptional approval boundary.

## Skill references

- `profile-schema.md` — schema navigation and runtime consequences.
- `metric-types.md` — collector/writer behavior and per-type design choices.
- `chart-design.md` — dashboard reasoning, hierarchy, conflicts, and UX.
- `ownership-proof.md` — source-backed ownership evidence and emitted-profile
  reconciliation.
- `how-tos/capture-metrics-dump.md` — safe evidence capture.
- `how-tos/build-synthetic-fixture.md` — source-complete sanitized fixture
  construction and proof boundaries.
- `sqlite-metadata-reset.md` — destructive metadata-reset decision boundary.
- `scripts/validate-profile.py` — thin launcher for the authoritative Go tool.
