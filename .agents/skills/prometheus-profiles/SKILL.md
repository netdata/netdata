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
13. `applications/<app>.md`, when present

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

## Separate evidence, inference, and product judgment

Maintain three mental buckets:

- **Observed:** metric names, `TYPE`, `HELP`, label keys/values, cardinality,
  and runtime behavior present in the supplied dump.
- **Authoritative external fact:** exporter/application documentation or source
  that explains what a metric means or what optional surfaces exist.
- **Design judgment:** the operator story, hierarchy, instance identity,
  dimension choice, chart composition, and ordering.

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

## Reason from the operator experience

Before writing YAML, answer:

1. **What will the operator ask first?**
   Examples: Is traffic flowing? Are users seeing failures? Where is work
   waiting? Which resource is saturated?
2. **What does the application do?**
   Use its domain language and pipeline/subsystems, not metric types such as
   “Latencies” or “Counters.”
3. **What entity does each series describe?**
   Distinguish service, node, model, queue, database, table, endpoint, and other
   hierarchy levels.
4. **Which labels identify that entity, and which describe an aspect of it?**
   Identity labels usually belong in `instances.by_labels`; bounded aspect
   labels often become dimensions; useful non-identity metadata may be
   promoted.
5. **Which signals are genuinely comparable?**
   Shared units are necessary but not sufficient. Dimensions must answer one
   coherent question and remain readable on one scale.

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
- Event, token, request, count, state, and time charts MUST use `line`.
  `area`/`stacked` MAY be used only when fill represents physical volume,
  space, bandwidth, or I/O rather than merely categories that add to a total.
- Prefer unique, increasing priorities when the dashboard has a total order.
  Treat duplicates or source-order divergence as review prompts, not automatic
  proof that the design is wrong.

Why: omitted or zero priorities all become `70000`; the planner does not derive
priority from YAML order. Explicit values force the author to decide the
operator journey. Matching source and presentation order makes that reasoning
reviewable; a deliberate tie or divergence is valid when its UX is explained.

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
- every chart has an explicit positive priority.

The report lists raw families absent after selector/relabel/writer processing
under `pipeline_excluded`; they are not misreported as chart coverage.
A job-policy exclusion summary shows how much otherwise writer-capable evidence
was removed before coverage was measured. A `PASS` over a deliberately reduced
denominator is mechanically valid but is not a complete dashboard unless every
exclusion satisfies the policy above.

Warnings are prompts for model review, not policy decisions. They include
generic auto-selection signatures, observed labels with no authored role,
the job-policy exclusion summary, observed per-rule allow/deny impact, unused
metric declarations, authored/runtime heatmap divergence, filled non-volume
charts, and sibling identity mismatch. Each can be intentional, but its UX or
diagnostic trade-off must be explained. Do not mechanically add identity,
promotion, or dimensions merely to silence a label warning; explain intentional
aggregation when losing that comparison is the correct design.

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
- Does every context represent one homogeneous instance type?
- Are dimensions bounded aspects rather than hidden entity explosions?
- Do sibling sections share parent identity labels where section-wide
  filtering is intended?
- Do chart titles promise only what the selected data computes?
- Are units, algorithms, and conversions honest?
- Are distribution shape, observation count, and observed-value sum kept as
  different semantic/unit roles rather than combined under one chart unit?
- Are smaller signals still visible, or flattened by a much larger dimension?
- Are stacked/area charts limited to meaningful volumes/occupancy, with line
  charts used for rates, counts, latency, and state?
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
- `applications/` — application facts; evidence aids, not schema authority.
- `how-tos/capture-metrics-dump.md` — safe evidence capture.
- `sqlite-metadata-reset.md` — destructive metadata-reset decision boundary.
- `scripts/validate-profile.py` — thin launcher for the authoritative Go tool.
