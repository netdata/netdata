# Authoring stock Prometheus semantic proofs

Stock proofs make the profile's source and dashboard claims executable. They are not snapshots generated from the profile:
source facts, operator intent, replay inputs, and production output remain independent authorities that the compiler
reconciles.

Read the [canonical framework architecture](../../../src/go/internal/promprofile/README.md) for the cross-repository
ownership, package boundaries, support compilation, replay, and compatibility model. This reference owns the artifact
authoring workflow and field navigation.

## Contents

- [Artifact layout](#artifact-layout)
- [Artifact responsibilities](#artifact-responsibilities)
- [Authoring sequence](#authoring-sequence)
- [SOURCE-SEMANTICS.yaml](#source-semanticsyaml)
- [PROFILE-DESIGN.yaml](#profile-designyaml)
- [proof.yaml](#proofyaml)
- [Generated source registries](#generated-source-registries)
- [Exclusions](#exclusions)
- [Verification](#verification)
- [Examples to copy](#examples-to-copy)

## Artifact layout

Netdata keeps compact reviewable intent with the profile:

```text
src/go/plugin/go.d/collector/prometheus/profile-proofs/<profile>/
  OPERATOR-MODEL.md
  PROFILE-DESIGN.yaml
  proof.yaml
```

Bulky source evidence and fixtures live in the latest `netdata/testdata` checkout:

```text
prometheus/profiles/<profile>/
  SOURCE-SEMANTICS.yaml
  fixtures/*.prom
  SOURCE-REGISTRY.yaml                 # optional; pair is indivisible
  SOURCE-REGISTRY.generator.yaml       # optional
  generator/*.py                       # required with the pair
```

`proof.yaml` paths such as `fixtures/haproxy_all_metrics.prom` are relative to
`prometheus/profiles/<profile>/` in testdata, not to the local proof directory.

The verifier enforces exact layouts: every local proof directory contains the three local files, every consumed fixture is
declared, and unreferenced or unexpected external artifacts fail verification.

## Artifact responsibilities

| Artifact | Owns | Must not duplicate |
|---|---|---|
| `OPERATOR-MODEL.md` | Human rationale, domain hierarchy, operator questions, causal flow, and unresolved limitations | Exact registrations, YAML routes, fixture outcomes |
| `SOURCE-SEMANTICS.yaml` | Public source-backed registrations, environments, components, labels, lifecycle, units, populations, relationships, and source exclusions | Private observed scrape data or dashboard destinations |
| `SOURCE-REGISTRY.yaml` | Mechanically extracted registrations and source locations for large/generated surfaces | Operator grouping or view semantics |
| `PROFILE-DESIGN.yaml` | Composition, entities, identity, label treatment, reducers, normalization, exclusions, limitations, views, units, and presentation intent | Replay inputs/results |
| `proof.yaml` | Realizable environments, fixture/sequence inputs, expected verdict/findings, future inputs, metadata-example identity, and coverage participation | Support-profile ownership or generated semantic summaries |
| `fixtures/*.prom` | Sanitized realizable raw inputs with collision-relevant identities and values | Semantic claims not present on the wire |

Support profiles are declared once in `PROFILE-DESIGN.yaml` under `composition.supports`. The proof compiler derives the
active support closure from that design and each case environment; do not copy a support list into `proof.yaml`.

All current schema versions are `v1`. The version labels the strict current format; it is not a historical residue and must
not be removed or advanced casually.

## Authoring sequence

1. Write `OPERATOR-MODEL.md` from application/source research before sorting metrics into charts.
2. Build `SOURCE-SEMANTICS.yaml` from pinned public upstream revisions and evidence locations.
3. Add the generated registry descriptor, output, and generator implementation/tests when registrations are too large or
   mechanically defined for trustworthy manual inventory.
4. Write `PROFILE-DESIGN.yaml` independently from the source contract: entities and operator questions first, then views,
   reductions, normalizations, and exclusions.
5. Encode the production profile from the approved design.
6. Build source-derived sanitized fixtures. Split mutually exclusive modes into separate fixtures/cases.
7. Write `proof.yaml` with independently expected PASS/FAIL results and explicit environments.
8. Verify the targeted profile, then the complete catalog.

Do not generate expected routes or chart identities from the candidate profile. The point is to detect disagreement between
independently authored contracts and production behavior.

## SOURCE-SEMANTICS.yaml

The strict top level is:

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

Key rules:

- Pin every public upstream to a full commit and cite repository-relative paths/lines in evidence.
- Evidence records have one typed `kind`; consumers may reference only compatible kinds. Common kinds include
  `registration`, `availability`, `population`, `lifecycle`, `unit`, `label`, `relationship`, `state_encoding`,
  `normalization`, `identity`, `deprecation`, `collection_hazard`, and `display_convention`.
- Every inline registration declares an exact or grammar family selector, Prometheus type/shape, optional environment
  condition, and registration evidence.
- Every signal declares what one observation describes, its components, label domains/stability/cardinality, functional
  dependencies, and contributor behavior when reduction depends on membership/reset semantics.
- Model lifecycle as source behavior (`current`, `cumulative`, or `constant`), not as a guess from the metric name.
- State encodings own closed state domains. Relationships own facts such as equivalence, partition, subset, overlap, and sum
  projection; a view cannot manufacture those facts.
- Reusable component/label policies are for actual reuse and must have at least two consumers.

The Go structures in `src/go/internal/promprofile/semantics/types_source.go` and strict validators in
`validate_source.go` are the executable field authority. Copy a nearby stock contract rather than inventing field shapes.

## PROFILE-DESIGN.yaml

The strict top level is:

```yaml
version: v1
profile: exporter
match: 'exporter_*'
app: exporter                 # optional; omit when runtime resolution is intended
namespace: exporter
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

Key rules:

- An entity records the operator grain plus required, alternative, and optional identity. Identity is the minimum
  non-aggregated view useful to the operator, not every label emitted by the source.
- A view records one question, one entity, source signal/components, label roles, optional reduction, and any non-default
  display/presentation intent.
- `labels.dimensions` owns bounded comparison labels; `promote` is an allowlist of useful non-identity metadata; `omit`
  records each deliberately lost label/comparison.
- A reduction declares both `reducer` and `lost_comparison`. It must agree with the production chart's `aggregation` and
  with source contributor semantics.
- A presentation declaration is required for non-default area/stacked intent. Stacked relationships must be source-proven,
  not inferred merely because dimensions share units.
- A normalization owns the semantic transformation that profile relabeling implements. Use category, finite/namespace
  alias, label rename, embedded identity repair/extract, or generated-component exclusion only when its strict schema and
  public evidence fit the source behavior.
- Declare support profiles here. A `when` policy controls environment activation; the compiler verifies that the support
  environment is owner-qualified and compatible.

The Go structures in `types_design.go` and validators in `validate_design.go` are the field authority.

## proof.yaml

A single-fixture case looks like:

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

Key rules:

- Every case supplies an explicit environment for the candidate and every active support profile.
- A candidate fixture is one endpoint scrape. It must contain the support-profile samples needed for production automatic
  selection in that environment; declaring a support environment does not inject another metric stream.
- `coverage: true` lets the case satisfy declaration-bounded source/design coverage. A focused negative or diagnostic case
  normally uses `coverage: false`.
- A standalone expected failure uses `expected: {verdict: FAIL, findings: [...]}`. Ordered lifecycle `steps` expect `PASS`;
  do not use a failed step as a reusable-session state transition.
- Use `steps` only when disappearance, contributor membership, reset, label replacement, or chart/dimension lifecycle must
  be proved across successive collection cycles.
- `future_inputs` add raw families absent from current source evidence to exercise future-relevant match/relabel branches.
  They do not claim those invented metrics have real future semantics.
- Keep an untyped scalar genuinely untyped in the fixture and classify it with a narrow profile `fallback_type` backed by
  lifecycle evidence. Adding a synthetic `# TYPE` line would bypass the behavior the proof is meant to exercise.
- `job: minimal` uses the validator's minimal job. `job: {metadata_example: ...}` replays the exact integration metadata
  example. The top-level `metadata_example` identifies the stock example the catalog must reconcile.
- `observations` assert declared semantic states and membership/aggregate/identity predicates; do not restate generated
  chart snapshots.

The strict descriptor lives in `src/go/internal/promprofile/proof/descriptor.go`. Use existing descriptors for syntax.

## Generated source registries

Use a registry when registration coverage is large, generated, or encoded by bounded source grammars. The descriptor,
generated output, and generator implementation/tests are one mechanical authority:

- `SOURCE-REGISTRY.generator.yaml` pins upstream repository, full commit, source paths, and runner ID.
- `generator/` contains deterministic extraction logic and tests.
- `SOURCE-REGISTRY.yaml` is the committed output. Groups are mechanical shorthand only; source signals select exact
  registrations/families and assign semantic ownership independently.

From the testdata repository root, verify one or all registries:

```bash
python3 prometheus/tools/source_registry_runner.py ceph
python3 prometheus/tools/source_registry_runner.py
```

The runner downloads pinned public sources, executes generator tests and generation in a network/file-write restricted
sandbox, and compares exact output. Do not hand-edit the generated registry without updating the generator.

## Exclusions

Every writer-capable source signal must render through a view or have one binding design exclusion. Allowed design reasons
and their additional fields are:

| Reason | Required additional field | Typical outcome |
|---|---|---|
| `equivalent_duplicate` | `covering_view` | retain unrendered or source-backed drop |
| `source_superseded` | `replacement` | source-backed drop/retain |
| `not_chartable` | `lost_question`, `required_operation: age_from_unix_epoch` | drop before writer |
| `metadata_only` | none; value must be a constant metadata carrier | `retain_writable_unrendered` |
| `collection_hazard` | source hazard evidence | drop before writer |

`not_chartable` is intentionally narrow: the current strict operation is only deriving age from a Unix timestamp. Do not
use it as a generic “not useful” escape hatch. `metadata_only` applies to a source-proven constant-one carrier with useful
metadata labels, not to any gauge that happened to equal one in a fixture.

## Verification

Use latest testdata `master`; the Netdata repository intentionally does not pin its commit. By default it is cloned at the
ignored `src/go/testdata`; `--testdata-root` can name another checkout.

For a coordinated source/profile change:

1. Use `src/go/testdata` as a checkout of the contributor's `netdata/testdata` fork.
2. Create one feature branch in testdata and one matching feature branch in Netdata.
3. Author and validate the source contract and fixtures on the testdata branch while the Netdata proof consumes that local
   checkout.
4. Open the paired pull requests and merge testdata first.
5. Update `src/go/testdata` to the merged testdata `master`, rerun complete Netdata verification, and then merge the Netdata
   pull request promptly.

The repositories cannot merge atomically under the latest-testdata model. This ordering keeps the unavoidable compatibility
window short without adding a testdata pin or weakening consumer verification.

From the Netdata repository root:

```bash
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py evidence-dirs
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py verify --profile exporter
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py verify
```

The targeted command accelerates iteration. The full command proves catalog layout, support closure, source/design schema,
metadata examples, normalization, routes, chart plans, observations, public wire identities, and aggregate semantic
coverage for every stock proof.

## Examples to copy

- `process_runtime`: smallest source/design/case contract and a `not_chartable` timestamp exclusion.
- `python_gc`: a bounded label used as chart identity and runtime support composition.
- `litellm`: single/multiprocess environments, future input, reduction and optional identity cases.
- `vllm`: support-profile composition, namespace aliasing, generated component exclusion, and high-cardinality acceptance.
- `haproxy`: `equivalent_duplicate` and `not_chartable` exclusions.
- `ceph`: generated registries plus bounded metric-name identity extraction/repair.

Read only the relevant example files. Copying an entire large contract before understanding its source model usually
creates stale evidence and accidental policy.
