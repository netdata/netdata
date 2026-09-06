# Authoring stock Prometheus semantic proofs

Stock proofs make a profile's source and dashboard claims executable. They are not snapshots generated from the
profile: source facts, operator intent, replay inputs, and production output stay independent authorities that the
compiler reconciles. Never generate expected routes or chart identities from the candidate profile; the point is to
detect disagreement between independently authored contracts and production behavior.

Owners this file does not restate:

- `src/go/internal/promprofile/README.md`: cross-repository ownership, the authority model, package boundaries, the
  compile and replay flow, support composition, the latest-testdata model and its paired-branch merge order.
- `src/go/plugin/go.d/collector/prometheus/profile-proofs/README.md`: the artifact layout in both repositories, the
  evidence boundary and what "sanitized" means, the external testdata contract, the verification workflow.
- `src/go/tools/prometheus-profile-proof/README.md`: the `evidence-dirs` and `verify` subcommands.
- The Go strict types are the field authority: `src/go/internal/promprofile/semantics/types_source.go` with
  `validate_source.go`, `types_design.go` with `validate_design.go`, and
  `src/go/internal/promprofile/proof/descriptor.go`. Copy a nearby stock contract rather than inventing field shapes.

## Artifact responsibilities

Local proof directory `src/go/plugin/go.d/collector/prometheus/profile-proofs/<profile>/` holds exactly
`OPERATOR-MODEL.md`, `PROFILE-DESIGN.yaml`, and `proof.yaml`; the bulky evidence lives in `netdata/testdata` under
`prometheus/profiles/<profile>/`, and `proof.yaml` fixture paths are relative to that testdata directory, not to the
local proof directory. The verifier fails on a missing local file, an undeclared consumed fixture, or an unreferenced or
unexpected external artifact.

| Artifact | Owns | Must not duplicate |
|---|---|---|
| `OPERATOR-MODEL.md` | human rationale, domain hierarchy, operator questions, causal flow, unresolved limitations | exact registrations, YAML routes, fixture outcomes |
| `SOURCE-SEMANTICS.yaml` (external, `netdata/testdata`) | public source-backed registrations, environments, components, labels, lifecycle, units, populations, relationships, source exclusions | private observed scrape data, dashboard destinations |
| `SOURCE-REGISTRY.yaml` (external) | mechanically extracted registrations and source locations for large or generated surfaces | operator grouping, view semantics |
| `PROFILE-DESIGN.yaml` | documentation, composition, entities, identity, label treatment, reducers, normalization, exclusions, limitations, views, presentation intent | replay inputs and results |
| `proof.yaml` | realizable environments, fixture and sequence inputs, expected verdict and findings, future inputs, metadata-example identity, coverage participation | support-profile ownership, generated semantic summaries |
| `fixtures/*.prom` (external) | sanitized realizable raw inputs with collision-relevant identities and values | semantic claims not present on the wire |

Support profiles are declared once, in `PROFILE-DESIGN.yaml` under `composition.supports`. The compiler derives the
active support closure from that design and each case environment. Do not copy a support list into `proof.yaml`, the
job configuration, or the source semantics.

All current schema versions are `v1`. The version labels the strict current format; do not remove or advance it
casually.

## Authoring sequence

1. Write `OPERATOR-MODEL.md` from application and source research before sorting metrics into charts.
2. Build `SOURCE-SEMANTICS.yaml` from pinned public upstream revisions and evidence locations.
3. Add the generated registry descriptor, output, and generator implementation with tests when registrations are too
   large or too mechanical for a trustworthy manual inventory.
4. Write `PROFILE-DESIGN.yaml` independently of the source contract: entities and operator questions first, then
   views, reductions, normalizations, exclusions.
5. Encode the production profile from the approved design.
6. Build source-derived sanitized fixtures (`how-tos/build-synthetic-fixture.md`). Split mutually exclusive modes into
   separate fixtures and cases.
7. Write `proof.yaml` with independently expected `PASS` or `FAIL` results and explicit environments.
8. Verify the targeted profile, then the complete catalog.

## `SOURCE-SEMANTICS.yaml`

The strict top level:

```yaml
version: v1
profile: exporter
upstreams: {}
evidence: {}
environment:
  axes: {}
  policies: {}
component_policies: {}
label_policies: {}
signals: {}
relationships: {}
state_encodings: {}
source_exclusions: {}
```

- Pin every public upstream to a full commit and cite repository-relative paths and lines in evidence.
- Every evidence record has exactly one `kind` from the closed set the loader enforces (`validate_source.go`):
  `availability`, `registration`, `lifecycle`, `unit`, `population`, `label`, `relationship`, `state_encoding`,
  `normalization`, `identity`, `deprecation`, `collection_hazard`, `display_convention`. Any other value fails the load.
  Consumers may reference only compatible kinds.
- Every inline registration declares an exact or grammar family selector, the Prometheus type and shape, an optional
  environment condition, and registration evidence.
- Every signal declares what one observation describes, its components, label domains with stability and cardinality,
  functional dependencies, and contributor behavior where reduction depends on membership or reset semantics.
- Model lifecycle as source behavior (`current`, `cumulative`, `constant`), never as a guess from the metric name.
- State encodings own closed state domains. Relationships own equivalence, partition, subset, overlap, and sum
  projection; a view cannot manufacture those facts.
- A reusable component or label policy exists for actual reuse: the compiler rejects one with fewer than two
  consumers (`semantics/evidence.go`, also for the design's label and reduction policies).

## `PROFILE-DESIGN.yaml`

The strict top level (`types_design.go`; `documentation.title` and `documentation.summary` are required text):

```yaml
version: v1
profile: exporter
match: 'exporter_*'
app: exporter                 # optional; omit when runtime resolution is intended
namespace: exporter
documentation:
  title: Exporter
  summary: One operator-facing sentence on what the profile charts.
composition:
  supports: {}
entities: {}
label_policies: {}
reduction_policies: {}
normalizations: {}
exclusions: {}
limitations: {}
views: {}
```

- `documentation.title` and `summary` are the public profile copy the integrations projection renders. Every
  `composition.supports` entry carries an `activation` sentence saying when an operator sees that supporting profile;
  machine condition IDs are not public explanations.
- An entity records the operator grain plus required, alternative, and optional identity: the minimum non-aggregated
  view useful to the operator, not every label the source emits.
- A view records one `question`, one entity, the source signal and components, label roles, an optional reduction, and
  any non-default presentation intent. The `question` is internal authoring rationale and is never projected into
  generated documentation.
- `labels.dimensions` owns bounded comparison labels; `promote` is an allowlist of useful non-identity metadata; `omit`
  records each deliberately lost label and comparison.
- A reduction declares both `reducer` and `lost_comparison` and must agree with the production chart's `aggregation`
  and with source contributor semantics.
- A presentation declaration is required for non-default `area` or `stacked` intent; stacked relationships must be
  source-proven, not inferred from shared units.
- A normalization owns the semantic transformation that profile relabeling implements (category, finite or namespace
  alias, label rename, embedded identity repair or extraction, generated-component exclusion) only when its strict
  schema and public evidence fit the source behavior.
- Support profiles are declared here with a `when` policy for environment activation; the compiler verifies that the
  support environment is owner-qualified and compatible.

## `proof.yaml`

A single-fixture case:

```yaml
version: v1
profile: exporter
metadata_example:
  integration_id: collector-go.d.plugin-prometheus-exporter
  example_name: Exporter
  job_name: exporter
cases:
  default:
    environment:
      exporter: {mode: default}
    fixture: fixtures/exporter_default.prom
    coverage: true
    expected: {verdict: PASS}
```

- Every case supplies an explicit environment for the candidate and every active support profile.
- A candidate fixture is one endpoint scrape. It must contain the support-profile samples production needs for
  automatic selection in that environment; declaring a support environment injects no metric stream.
- `coverage: true` lets the case satisfy declaration-bounded source and design coverage. A `FAIL` case must set
  `coverage: false` (the descriptor loader rejects otherwise); a diagnostic `PASS` case normally does too.
- A standalone expected failure is `expected: {verdict: FAIL, findings: [...]}`. Ordered lifecycle `steps` expect
  `PASS`; a failed step is not a reusable session state.
- Use `steps` only when disappearance, contributor membership, reset, label replacement, or chart and dimension
  lifecycle must be proved across successive collection cycles.
- `future_inputs` add raw families absent from current evidence to exercise future-relevant `match` and relabel
  branches; they claim no real future semantics.
- Keep an untyped scalar genuinely untyped in the fixture and classify it with a narrow profile `fallback_type` backed
  by lifecycle evidence. A synthetic `# TYPE` line bypasses the path the proof exists to exercise.
- `job: minimal` uses the validator's minimal job; `job: {metadata_example: ...}` replays the exact integration
  metadata example; the top-level `metadata_example` names the stock example the catalog must reconcile.
- `observations` (inside a `steps` entry) assert declared semantic states and membership, aggregate, and identity
  predicates; they never restate generated chart snapshots.

## Generated source registries
 Use a registry when registration coverage is large, generated, or encoded by bounded source grammars. The descriptor
`SOURCE-REGISTRY.generator.yaml` (upstream repository, full commit, source paths, runner ID), the `generator/` directory
(only `*.py` files: `generate.py` plus at least one `test_negative_*.py` fail-closed test, checked by
`semantics/load.go`), and the `SOURCE-REGISTRY.yaml` output committed to `netdata/testdata` are one mechanical
authority; groups in the output are shorthand only, and source signals select exact registrations and assign semantic
ownership independently. Verify from the testdata repository root with `python3
prometheus/tools/source_registry_runner.py [<profile>]`, which downloads the pinned sources, runs the generator tests
and generation in a restricted sandbox, and compares exact output. Never hand-edit the generated registry without
updating the generator.

## Exclusions

Every writer-capable source signal renders through a view or has one binding design exclusion. The loader
(`validate_design.go`) enforces the `reason` set, each reason's required fields, and the two `outcome` literals
`drop_before_writer` and `retain_writable_unrendered`; only `metadata_only` is bound to one outcome. The compiler
(`semantics/evidence.go`) enforces which evidence kinds each reason must cite. Every other reason
takes either outcome, and shipped proofs use both (`haproxy` retains a `not_chartable` timestamp, `process_runtime`
drops one). Only `retain_writable_unrendered` discharges an `autogen.selector.deny` (`semantics/coverage.go`).

| `reason` | Required field | Required evidence kinds | Typical `outcome` |
|---|---|---|---|
| `equivalent_duplicate` | `covering_view` | `relationship` | `retain_writable_unrendered`, or `drop_before_writer` with a source-backed drop |
| `source_superseded` | `replacement` | `deprecation` | either, source-backed |
| `not_chartable` | `lost_question`, `required_operation: age_from_unix_epoch` | `lifecycle` and `unit` | either |
| `metadata_only` | none; the value must be a constant metadata carrier | `lifecycle`, `unit`, and `label` | `retain_writable_unrendered` (enforced) |
| `collection_hazard` | none | `collection_hazard` | either |

`not_chartable` is deliberately narrow: the only strict operation is deriving age from a Unix timestamp; it is not a
generic "not useful" escape. `metadata_only` needs the conditions in `metric-types.md`, "Info families", not a gauge
that happened to equal one in a fixture. Every production `autogen.selector.deny` family must be discharged by a
`retain_writable_unrendered` exclusion naming that family; a `drop_before_writer` exclusion does not discharge it
(`semantics/coverage.go`, `replay_route.go`).

## Verification

From any directory (the launcher resolves the repository root):

```bash
.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py evidence-dirs
.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py verify --profile exporter
.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py verify
```

The launcher injects `--repo-root`; `--testdata-root` names a checkout other than the ignored `src/go/testdata`. The
targeted command accelerates iteration; the full command proves catalog layout, support closure, source and design
schema, metadata examples, normalization, routes, chart plans, observations, public wire identities, and aggregate
semantic coverage for every stock proof. CI runs the same verification
(`.github/workflows/prometheus-profile-tests.yml`). For a change that touches both repositories, follow the
paired-branch order in `promprofile/README.md`, "Latest-testdata model": merge testdata first, then rerun complete
verification before merging Netdata.

## Examples to copy

- `process_runtime`: smallest source, design, and case contract; a `not_chartable` timestamp exclusion.
- `python_gc`: a bounded label used as chart identity; runtime support composition.
- `fastapi`: the smallest chart-heavy HTTP instrumentation contract; declared as a support by `vllm`.
- `litellm`: single and multiprocess environments, a future input, reduction and optional identity cases.
- `vllm`: support-profile composition, namespace aliasing, generated component exclusion, high-cardinality acceptance.
- `haproxy`: `equivalent_duplicate` and `not_chartable` exclusions.
- `ceph`: generated registries plus bounded metric-name identity extraction and repair.

Read only the relevant example files. Copying an entire large contract before understanding its source model creates
stale evidence and accidental policy.
