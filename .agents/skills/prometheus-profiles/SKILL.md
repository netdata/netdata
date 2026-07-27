---
name: prometheus-profiles
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

- **Domain proof:** source-backed capabilities, causal stages, entity
  boundaries, and support subsystems explain what the application does
  independently of its metric inventory.
- **Navigation proof:** every first- and second-level family names the closest
  domain owner of its charts. The author can state what work that owner
  performs, receives, produces, stores, routes, or controls.
- **Holistic-diagnosis proof:** available workload, outcome, error, latency,
  saturation, capacity, and resource signals remain with the function that can
  cause them, except for a deliberately small service-impact overview.
- **Unit proof:** every dimension on one axis has the same rendered algebra and
  the same counted or measured object.
- **Evidence proof:** every writer-capable family is curated or excluded only
  under one of the binding cases, with the lost operator question stated.
- **Runtime proof:** the exact profile, dump, and job policy pass the objective
  validator.

Why: each proof catches a different failure class. Runtime coverage cannot prove
that a `Latency` branch teaches causality; a good family tree cannot make
operations and their produced items the same unit; a curated dashboard cannot
recover a signal that job policy discarded. Model judgment chooses the design,
but none of these contradictions is an acceptable exercise of judgment.

## Research the application from evidence

Maintain three mental buckets:

- **Observed:** metric names, `TYPE`, `HELP`, label keys/values, cardinality,
  and runtime behavior present in the supplied dump.
- **Authoritative external fact:** exporter/application documentation or source
  that explains what a metric means or what optional surfaces exist.
- **Design judgment:** the operator story, hierarchy, instance identity,
  dimension choice, chart composition, and ordering.

The supplied dump is the observed inventory, not the complete application
model. Research unfamiliar or ambiguous semantics before designing:

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

Use every research capability available for the task: supplied source trees,
web browsing, repository search, and mirrored open-source repositories. Do not
stop at `HELP` when it leaves the observation population, identity, reset
behavior, or unit meaning ambiguous. Conversely, do not copy another
dashboard's hierarchy merely because it already exists; it may encode a
different deployment model or repeat an upstream mistake.

Do not present an inference as observed fact. One dump proves only one
configuration and moment:

- Zero values are still observed metrics and may matter under load.
- An optional feature absent from the dump is not validated. Obtain a
  representative dump/fixture before claiming it works.
- Do not invent metric names, label keys, or types from application concepts.

If `TYPE` is missing, do not guess silently. The collector can recover only
selected untyped scalar families through `fallback_type`; it cannot reconstruct
histogram/summary structure that the exposition did not declare. Use
authoritative exporter evidence or obtain a better dump.

Treat dumps as potentially sensitive. Keep them in ignored `.local/` or user
temporary storage, and never copy customer/user label values into committed
fixtures or documentation.

## Build the semantic inventory before the YAML

First build a source-backed domain map without using metric type, unit, or name
shape as its organizing principle. Record:

- what the application does in operator vocabulary;
- its capabilities, subsystems, and causal or processing stages;
- how work and failures propagate between them;
- its entity types and containment relationships; and
- the runtime/platform surfaces that support the application but are not one of
  its domain capabilities.

Why: starting from a sorted metric list invites the exporter to design the
dashboard accidentally. A domain map provides independent semantic owners for
the signals. Metrics can then explain the application instead of becoming the
application's navigation.

Represent the important causal relationships, not just a bag of nouns. For each
candidate navigation owner, be able to state:

- what work or state it owns;
- what it receives from and hands to adjacent owners;
- what it can delay, reject, corrupt, exhaust, or saturate; and
- which domain entity an operator filters there.

If the only explanation is “these metrics all measure latency,” “these all
count the same object,” or “these are distributions,” the candidate is a
measurement category, not a causal owner.

Classify every observed writer-capable family against that map before grouping
it. The format is up to the author, but the reasoning must make these fields
explicit:

- **Operator capability:** the application function, subsystem, or causal stage
  this signal explains.
- **Entity type:** the thing one series describes, such as service, cluster,
  server, database, table, worker, endpoint, device, or queue.
- **Identity labels:** the smallest stable label set that identifies that
  entity, including every parent identity label it inherits.
- **Signal role:** workload, outcome, error, latency, saturation, capacity,
  utilization, resource use, configuration, or another domain role.
- **Observation population:** what one counter increment, histogram
  observation, summary observation, or gauge value represents.
- **Unit algebra:** the raw unit, algorithm, conversion, final rendered unit,
  and exact counted or measured object. A broad noun such as `events`,
  `operations`, `items`, or `observations` does not make different objects
  interchangeable merely because all become rates.
- **Label roles and cardinality:** identity, bounded dimension, promoted
  metadata, selector routing, or intentional aggregation.
- **Evidence and uncertainty:** the observed or authoritative source supporting
  the classification and any unresolved limitation.
- **Destination:** the intended functional family/chart, or one of the binding
  exclusion cases below.

This inventory is a reasoning aid, not a fixed worksheet or dashboard
generator. Its purpose is to prevent the author from discovering semantic
conflicts only after metrics have already been grouped by name or unit.

Then answer the operator questions:

1. What does the application do, in the operator's vocabulary and causal order?
2. What will the operator ask first: Is useful work flowing? Are outcomes
   failing? Where is work waiting? Which resource or limit is saturated?
3. Which entity level should a filter select at each navigation level?
4. Which signal roles give a holistic view of each function?
5. Which signals are genuinely comparable on one chart?

Functional capability drives navigation. Signal role and unit constrain chart
composition *inside* that story; they are not application-wide navigation
categories. A top-level “Latency,” “Errors,” or “Bytes” section forces the
operator to reconstruct the application's causal path.

Before writing YAML, audit the first two levels of the proposed family tree:

- Read only the family names, without metric names. They SHOULD describe what
  the application does, where work is, or which domain entity owns it.
- A family whose main meaning is `Latency`, `Workload`, `Errors`,
  `Distributions`, `Parameters`, `Counters`, or a unit is an observability or
  exposition taxonomy, not a domain owner. Move those signals under the
  capability they explain. A deliberately small service-level SLI overview is
  the exception, not a coverage bucket.
- For each capability, check whether its available workload, outcomes, errors,
  latency, saturation, capacity, and resource-pressure signals are close enough
  to form one diagnosis. If they have been scattered into application-wide role
  sections, the tree is not yet an operator mental model.
- Every writer-capable signal MUST have a domain owner or a documented
  service/runtime boundary before its signal role or unit affects placement.
  “It is a histogram,” “it has the same units,” or “it is a request parameter”
  is not an owner.

This audit is a semantic guardrail, not a prescribed tree. Different authors may
choose different defensible capability boundaries and orders; they must not
replace application reasoning with a measurement taxonomy.

Resolve signals that span the domain by causal ownership:

- Put a stage-specific workload, outcome, latency, saturation, or resource
  signal with the stage that produces, consumes, queues, or controls it.
- Put a true end-to-end signal with the nearest lifecycle owner common to all
  the stages it spans, usually the domain operation or service-impact view.
- Put a client-requested limit or option with the intake/admission/operation
  owner whose behavior it shapes, not in an application-wide `Parameters`
  drawer.
- Use a shared resource as a navigation owner only when operators manage it as
  its own subsystem or entity. Otherwise keep pressure and consumption with the
  capability that causes them.
- Treat a domain object name with the same skepticism as a unit. Measurements
  of an object at different lifecycle stages belong with those stages unless
  the object itself is the filterable operator entity.

Why: nearest ownership preserves causal adjacency without pretending every
signal belongs to exactly one physical component. It also gives overlapping and
end-to-end metrics a principled home instead of recreating global `Latency`,
`Throughput`, `Parameters`, or `Resources` buckets.

See `chart-design.md` for the consequences and conflict-resolution guidance.
See `metric-types.md` for what the collector actually writes for each
Prometheus type.

Inventory every observed label key. For each one, record whether it is
identity, dimension, promoted metadata, selector-only routing, or intentionally
aggregated. A label omitted from the design review is not evidence that
aggregation is safe.

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

For every observed writer-capable family that answers a distinct operator
question, the default is to curate it. A job exclusion is acceptable only when
at least one of these cases is evidenced:

1. **Unrenderable raw form:** the available value cannot answer its question
   without an unsupported transform, such as `now - epoch`.
2. **Authoritatively superseded:** a supported replacement answers the same
   question across the intended versions, types, labels, and distribution
   population.
3. **Concrete collection hazard:** bounded evidence shows a privacy,
   cardinality, correctness, or resource risk that cannot be made safe in the
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
route every writer-surviving flattened bucket, count, sum, quantile, counter, and
gauge series, but it MUST NOT mechanically create one chart for every metric or
three charts for every histogram:

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

The operator question decides the chart boundary; zero autogen/unmatched
decides whether every written series was routed. Conflating those decisions
produces repetitive metric-type dashboards instead of useful comparisons.

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

Run the gate on the exact profile, dump, and structured job policy being
delivered. Re-run after every edit.

A `PASS` proves, for that evidence:

- strict catalog/profile/template decoding succeeds;
- the real collector completes `Init`, `Check`, and a committed `Collect`;
- the real writer and flattening behavior are exercised;
- every written series is curated, with zero autogen and zero unmatched;
- every authored chart and dimension materializes;
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
A job-policy exclusion summary shows how much otherwise writer-capable evidence
was removed before coverage was measured. A `PASS` over a deliberately reduced
denominator is mechanically valid but is not a complete dashboard unless every
exclusion satisfies the policy above.

Warnings are prompts for model review, not policy decisions. They include
generic auto-selection signatures, observed labels with no authored role,
the job-policy exclusion summary, observed per-rule allow/deny impact, unused
metric declarations, authored/runtime heatmap divergence, ambiguous or
physical-rate filled charts, repeated sibling family paths, mixed leaf identity,
parent identity loss, and sibling identity mismatch. Each can be intentional,
but its UX or diagnostic trade-off must be explained. Do not mechanically add
identity, promotion, or dimensions merely to silence a label warning; explain
intentional aggregation or an intentional entity-level boundary when it is the
correct design.

The gate cannot judge whether the dashboard is useful, and it cannot prove
behavior for unseen metrics or label values. Exact candidate selection also
does not prove that `match` safely auto-selects this profile against unrelated
endpoints. Read
`src/go/tools/prometheus-profile-validation/README.md` for the exact evidence
boundary.

## Perform the semantic review after `PASS`

Review the rendered design, not merely the YAML:

- Does the first screen answer the most urgent operator questions?
- Does the family tree follow application functions and entity hierarchy?
- If only the first two family levels are read, do they teach the application's
  capabilities and causal path rather than list signal roles, metric forms,
  parameter kinds, or units?
- Does each capability keep its available workload, outcomes, latency,
  saturation, and pressure signals together enough for holistic diagnosis?
- Does every context represent one homogeneous instance type?
- Does each displayed leaf family contain one effective entity identity?
- Are dimensions bounded aspects rather than hidden entity explosions?
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

## Deliver and install

Deliver:

- the profile YAML;
- the recommended structured job policy (or exact changes to an existing job);
- a concise mapping from operator questions to families/charts;
- validation inputs/hashes and the `PASS` summary;
- explicit evidence limitations or optional surfaces still needing fixtures.

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
- `how-tos/capture-metrics-dump.md` — safe evidence capture.
- `sqlite-metadata-reset.md` — destructive metadata-reset decision boundary.
- `scripts/validate-profile.py` — thin launcher for the authoritative Go tool.
