---
name: collectors-prometheus-profiles
description: Create, review, validate, prove, iterate, or install Netdata Prometheus collector chart profiles (`go.d/prometheus.profiles/*.yaml`). Use for exporter dashboard design, profile schema/runtime behavior, selector/relabel/fallback policy, chart coverage and cardinality, stock semantic proof artifacts, live profile verification, or the authoring scripts (`validate-profile.py`, `profile-toc.py`, `proof-bundle.py`).
---

# Prometheus profile authoring

A profile is an operator dashboard design backed by source evidence, not a mechanical metric-name translation. The
author decides the entity grain, chart comparisons, cardinality boundary, hierarchy, and collection policy; the
validator proves only the contracts code can establish.

## Authorities

When this skill and the code disagree, these shipped documents win; the skill states nothing they already state:

- `src/go/plugin/go.d/collector/prometheus/profile-format.md`: the envelope, the runtime processing order, the stock
  contribution policy for `autogen.selector` and relabeling, chart-template rules, job-side profile selection.
- `src/go/plugin/framework/charttpl/README.md`: every group and chart field, the validation rules, engine-derived
  behavior.
- `src/go/plugin/go.d/collector/prometheus/relabel/README.md`: relabel actions, stage order, histogram and summary
  safety, profile precedence.
- `src/go/tools/prometheus-profile-validation/README.md`: the validator CLI, safe job policy, what `PASS` establishes
  (one finding code per objective check), the warning classes.
- `src/go/internal/promprofile/README.md`: framework boundary, authority model, compile and replay flow, support
  composition, the latest-testdata model.
- `src/go/plugin/go.d/collector/prometheus/profile-proofs/README.md` and
  `src/go/tools/prometheus-profile-proof/README.md`: proof artifact contract, evidence boundary, external testdata
  contract, the `evidence-dirs` and `verify` commands.
- `docs/NIDL-Framework.md`: the NIDL model (families, contexts, instances, dimensions, labels).

`src/go/plugin/go.d/collector/prometheus/README.md` is a symlink to a generated integration page and is not an
authority.

## Choose the workflow

Read the row for the work at hand, not the whole skill.

| Work | Read |
|---|---|
| Create or redesign a user profile | `chart-design.md`, `metric-types.md`; `profile-schema.md` for where each field is documented |
| Create or change a stock profile | the above, then `ownership-proof.md`, `proof-authoring.md`, and the rule sheet below |
| Build stock fixtures | `how-tos/build-synthetic-fixture.md`, `proof-authoring.md` |
| Capture private evidence | `how-tos/capture-metrics-dump.md` |
| Answer a schema or runtime question | the owner named in `profile-schema.md`, then the authority itself |
| Install or live-test a profile | "Delivery and live verification"; `sqlite-metadata-reset.md` only when a reset is proposed |
| Relate a stock profile to integration metadata | `.agents/skills/integrations-lifecycle/how-tos/prometheus-profile-metadata.md` |
| Change the scripts | "Scripts" below |

## Authoring workflow

1. **Establish the evidence boundary.** Inventory metric families, types, labels, optional modes, and source revisions.
   Separate exposition facts (names, `HELP`, `TYPE`, labels, observed values), source facts (lifecycle, units, label
   domains, relationships, optionality), and design judgments (grain, composition, hierarchy, exclusions). A captured
   endpoint is private input: never commit it unless deliberately sanitized from public source contracts, and never
   commit credentials, customer identities, private endpoints, or deployment data. Missing source evidence is a real
   limitation; one observed fixture is not a universal exporter contract.
2. **Design the operator model before YAML** (`chart-design.md`). Answer, in causal order: is the service available and
   doing useful work; what load is it serving; is latency, error rate, or saturation worsening; which bounded entity or
   category explains it; which resource or dependency is responsible. For each view state one operator question, one
   entity grain, the smallest stable identity, the labels compared as dimensions, the labels kept for filtering, the
   labels omitted with the reducer that keeps the omission truthful, and the exact source signals with units,
   lifecycle, and inter-dimension relationship. Do not build an aggregate view Netdata derives by grouping the detailed
   one; choose the finest operator-useful grain whose cardinality and churn stay acceptable.
3. **Classify labels and cardinality** (`chart-design.md`, "Assign labels by role"; `docs/NIDL-Framework.md` when
   choosing a monitored component, instance grain, dimension set, or label role). Give every relevant label a role:
   required identity, optional identity, dimension, promoted metadata, routing only, or omitted with a stated lost
   comparison. Estimate cardinality from the exporter contract, not the fixture; raw user IDs, addresses, request IDs,
   URLs, exception text, and hashes are normally too high-cardinality for identity or dimensions.
4. **Choose truthful aggregation** (`chart-design.md`, "Aggregation when labels are omitted"). One reducer per chart;
   never merge gauges and counters into one rendered dimension, although a chart may hold distinct authored dimensions
   of different kinds when the shared comparison is intentional, each keeping its runtime-derived algorithm; quantiles
   are not mergeable. In a stock profile write `aggregation` explicitly wherever a deliberate many-to-one projection can
   occur, including deliberate `sum`, and omit it for collision-free routes.
5. **Choose collection policy.** `match` detects the exporter and bounds the profile's source namespace; prefer
   exporter-unique families for detection: generic runtime families (`process_*`, `python_*`, `http_*`) may still be
   charted, but naming them in `match` makes unrelated endpoints eligible. Profile `relabeling` normalizes or drops
   exporter-owned families after selection (recover bounded identity encoded in metric names, normalize label values
   such as status classes, remove an established useless family class such as a source-wide `*_created`, or another
   source-backed transformation), only with a source-backed contract and a bounded result, never as a substitute for an
   identity or aggregation decision; the first applicable selected profile owns each original family and every selected
   template consumes the shared result. Profile `fallback_type` classifies untyped scalars the exporter owns, as
   narrowly as the evidence allows. `autogen.selector` shapes only the generic fallback charts. Job `selector`, job
   relabeling, and job `fallback_type` are deployment policy and are never a hidden prerequisite of a stock profile.
   Unknown future families inside a wildcard namespace stay eligible for generic fallback; a bounded, source-proven
   alias may route a future input to an authored metric only when an explicit `future_inputs` case proves that branch.
6. **Encode the profile.** Nested groups express hierarchy: child `context_namespace` segments join with `.`, child
   `family` segments with `/`. Omit the root `family` when it would only repeat the resolved application; the named
   child groups then become top-level families, nested groups still need `family`, and a chart directly under a
   transparent root needs its own `family`. Keep a meaningful root on reusable instrumentation profiles that compose
   into other applications. Use base units; convert bytes to bits only where the operator convention is bandwidth. Omit
   `algorithm` (the runtime kind resolves it), `type` for `line` charts and histogram buckets (forced to `heatmap`), and
   multiplier and divisor defaults; use `area` and `stacked` only for the meanings in `chart-design.md`, "Choose chart
   types for visual meaning". Status values are dimensions from the source's closed state mapping, not chart instances.
   Gauge families ending in `_info` are skipped by the writer and never reach metrix (`metric-types.md`, "Info
   families").
7. **Run the objective validator and the ToC** ("Scripts" below). `PASS` proves schema and the exercised production
   collector, planner, and emitter path; it does not prove operator usefulness, source semantics, cardinality outside
   the evidence, or additivity. Resolve every warning with evidence; never silence one mechanically.
8. **Review as an operator** (`chart-design.md`, "Semantic review"). For a stock profile continue with
   `ownership-proof.md` and `proof-authoring.md`; stock work is complete only when the source contract, design,
   descriptor, fixtures, production profile, and integration metadata reconcile.

## Stock contribution rule sheet

Stock profiles live under `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/<name>.yaml` with a proof
directory beside the collector. The "review by hand" table is contribution policy that the runtime format permits in
user profiles. The validator has no user mode: its contributor-policy checks below apply to every profile it validates.

### The code enforces

- Validator (`tools/prometheus-profile-validation`, finding codes in its README): no `autogen.selector.allow`; every
  `deny` names one exact family (`open_ended_profile_fallback_deny`), and under CLI validation that family must appear
  in the fixture (`unproven_profile_fallback_deny`; under proof replay the coverage discharge below covers it); current
  evidence yields zero generic fallback and zero unmatched series (unmatched series downgrade to the
  `profile_suppressed_series` warning when a selected profile's `autogen.selector` explains every one); the relabel
  grammar and name-provenance rules (under proof replay, three specific checks are deferred to warnings, one branch each
  of `open_ended_relabel_name_rewrite`, `unpreserved_relabel_name_identity`, and `unbounded_relabel_discard`; the other
  branches emitting the first two codes stay errors); a lifecycle cap that discards observed entities or dimensions; a
  chart with no visible dimension; bucket charts use `observations/s` and no algorithm other than `incremental`
  (omitting it is fine); explicit `area` or `stacked` raises a semantic-review warning; a selected series carrying a
  label the chart neither uses nor excludes raises a warning.
- Proof loaders and compiler (`internal/promprofile/semantics`): `documentation.title` and `summary` required; the
  closed evidence `kind` set; the `outcome` literals `drop_before_writer` and `retain_writable_unrendered`, with
  `metadata_only` bound to the latter; a reusable component, label, or reduction policy in either document needs two
  consumers (compile stage, `semantics/evidence.go`); every production `autogen.selector.deny` family is discharged by a
  `retain_writable_unrendered` exclusion naming it (`coverage.go`); `metadata_only` requires the conditions in
  `metric-types.md`, "Info families"; supports are accepted only in `PROFILE-DESIGN.composition.supports`; no other
  strict schema has the field.
- Integrations projection (`integrations/prometheus_profile_docs.py`, run by `gen_integrations.py` and
  `integrations/tests/test_prometheus_profile_docs.py`): every stock profile, supporting ones included, needs a direct
  row under some module in the top-level `profile_coverage.modules` of the Prometheus collector `metadata.yaml`
  (generation raises `Stock Prometheus profiles without an integration mapping` otherwise); the projection resolves the
  support closure from the design; the key is accepted only on `go.d.plugin/prometheus` modules (a Python check, not
  the JSON schema); an unknown module id under it is recorded as a warning by `_common.py`, and `gen_integrations.py`
  ends with `fail_on_warnings()`, so the generation run still fails; a semantic view and its runtime chart
  must agree on family and chart identity (also enforced by the proof compiler, `semantics/replay_route.go`); the view
  `question` is never rendered.

### Review by hand

| Surface | Stock rule |
|---|---|
| chart `id` | omit when the context-derived ID is sufficient |
| `priority` | set `chart_defaults.priority` at the nearest group only where operator navigation needs one order for that subtree; chart-local `priority` only for a deliberate exception; otherwise omit (runtime default `70000`) |
| `instances.by_labels: ['*']` | avoid; use explicit source-backed identity |
| `lifecycle` caps | omit; coverage must not depend on silently dropping observed or future entities |
| `options.float` | omit when the runtime metric is already floating point |
| `algorithm`, `type`, multiplier, divisor | omit defaults; `algorithm` only for a deliberate source-lifecycle override (`metric-types.md`) |
| `aggregation` | explicit wherever a many-to-one projection can occur; omitted for collision-free routes |
| root `family` | omit when it only repeats the application; keep for reusable instrumentation profiles |
| stock `metadata.yaml` example | must show that auto-selection suffices: no `profiles`, `app`, job `selector`, job relabeling, or job `fallback_type`; such a field in a stock example is evidence that profile ownership is incomplete |
| profile-required normalization | lives in profile `relabeling`, never duplicated as an optional job recipe |
| `profile_coverage.modules` row | lists the stock profiles the module owns, never a service's own supports: a repeated support is projected as `primary` with no activation sentence, and nothing rejects it |
| `PROFILE-DESIGN.yaml` `documentation` | operator-facing `title` and `summary`; every `composition.supports` entry has an `activation` sentence |
| integration copy | the generated coverage table groups rows by top-level family (metric, family and chart title, dimension, unit, entity scope); `metrics_description` carries a short operator-model brief, not the chart ledger |

## Scripts

Invoke them from the repository root as written below. The two launchers resolve the repository root themselves and make
caller-relative file arguments absolute before `go run` from `src/go`, so their file arguments may be given relative to
wherever you are; `profile-toc.py` is plain Python and takes the profile path as given. CI runs their unit tests
(`.github/workflows/prometheus-profile-tests.yml`, "Verify authoring launcher"); run them locally with
`.venv/bin/python3 -m unittest discover -s .agents/skills/collectors-prometheus-profiles/scripts -p 'test_*.py'`.

- `.agents/skills/collectors-prometheus-profiles/scripts/validate-profile.py --profile P --dump D [--job J]
  [--support-profile S]... [--output text|json]`:
  launcher for `tools/prometheus-profile-validation`. A user profile may use a minimal or deployment-specific job;
  stock validation uses the jobs its proof cases declare. A dash-prefixed token is never absolutized as an option's
  value, so a missing value reaches the tool unchanged and is rejected there instead of becoming a bogus path.
- `.venv/bin/python3 .agents/skills/collectors-prometheus-profiles/scripts/profile-toc.py PROFILE [--app APP] [--quiet]`
  (needs PyYAML, which the repository `.venv` provides): renders the operator-visible family tree with contexts and
  effective priorities, then advisory UX warnings; it is not a gate. `--app` defaults to the profile's `app:`; the root
  `context_namespace` is dropped when it equals the app, as the collector does, and the `prometheus.<app>.` prefix is
  omitted. A chart with its own `family` is placed under that child node, as chartengine composes it. Priority `0`
  inherits, non-positive resolves to `70000`. The six warnings, each to investigate and either repair or record as
  intentional: a one-character family segment (usually a slash label such as `I/O` split into path segments); all
  top-level families sharing a prefix, a top-level family equal to the application name, or one starting with it
  (usually a repeated application or root); a leaf with more than 15 contexts (usually missing intermediate owner
  structure); a one-context leaf (unnecessary structure or a merge candidate). Do not remove structure only to silence a
  warning when the parent is an operator entity, module boundary, or release contract; do not add structure only to
  divide by metric type.
- `.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py evidence-dirs | verify [--profile P]
  [--testdata-root DIR]`: launcher for
  `tools/prometheus-profile-proof`; injects `--repo-root`.

## Delivery and live verification

- Keep reusable runtime or instrumentation profiles independent; a service profile declares them in
  `PROFILE-DESIGN.composition.supports` and never duplicates their charts.
- Install a user profile under the configured user profile directory and verify profile selection, advancing values,
  chart identity, labels, hierarchy, and cardinality against a live target.
- Do not reset Netdata's SQLite metadata as routine iteration; identities expire through lifecycle and retention. Read
  `sqlite-metadata-reset.md` and obtain explicit production approval before any destructive reset.
- When editing `metadata.yaml`, follow `integrations-lifecycle`: validate with the generators, never hand-edit
  generated files under `integrations/`, and undo any regenerated tracked pages before committing.

## References

- `chart-design.md`: operator model, entity grain, label roles, reducers, hierarchy, presentation, cardinality, review.
- `metric-types.md`: parser and writer behavior per Prometheus type and the design consequences.
- `profile-schema.md`: which document owns each field group; the notes none of them states.
- `ownership-proof.md`: source ownership inventory and reconciliation.
- `proof-authoring.md`: stock artifact responsibilities, skeletons, exclusions, verification.
- `how-tos/capture-metrics-dump.md`: private evidence capture.
- `how-tos/build-synthetic-fixture.md`: public source-complete fixture construction.
- `sqlite-metadata-reset.md`: the destructive metadata-reset boundary.
