<!-- markdownlint-disable MD013 MD043 -->

# LiteLLM Prometheus profile validation

## Bounded source scope

- Observed application: LiteLLM 1.92.0.
- Structural-union source: `BerriAI/litellm @ 23de7a15d9d40006ee596e617475ba101d60c5e9`.
- Current-source comparison: `BerriAI/litellm @ de706a35a6f1e9cb8c3cb527271df0b76a69f410`.
- Single-process runtime registry: `prometheus/client_python` 0.24.1 @
  `f417f6ea8f058165a1934e368fed245e91aafc14`.
- The committed fixture is a sanitized structural union of mutually optional callback, configuration, label, multiprocess, and
  single-process runtime shapes. It is not one realizable endpoint.

## Authoritative structural-union result

- `proof.yaml` is the authoritative machine-checked PASS verdict and complete objective count record.
- The exclusion ledger dispositions are 82 job-excluded routes and 1 writer-ineligible information route.
- Generic fallback, unmatched series, dead charts/dimensions, materialization loss, and collisions: **0**.
- `src/go/testdata/prometheus/profiles/litellm/SOURCE-INVENTORY.tsv` has 356 rows and maps all 273 exact authored selector routes; unresolved
  families/selectors **0**.

## Recommended-job requirement

The no-job structural run is deliberately **FAIL**, not completion evidence: it produces one generic process-start epoch
chart and leaves alias/created writer series outside authored curation. The recommended job excludes the exact generated
`_created` names in the declared source union, `process_start_time_seconds`, two deprecated request/failure aliases, and the
batch-cost raw timestamp. Process CPU, memory, file descriptors, and Python GC remain charted in single-process mode.

## Forward compatibility

- The source-complete fixture has zero generic fallback and zero unmatched series.
- The explicit deny list defensively suppresses generated epochs, deprecated aliases, and the raw batch-cost timestamp when
  job policy does not.
- Unknown future `litellm_*` families remain generically visible until source evidence can assign identity, unit,
  population, owner, and a curated destination.

## Identity and aggregation checks

- Additive service views remain complete when optional labels are absent; optional breakdown routes require their entity labels.
- Complementary nonblank/blank routes keep one context and use `unclassified` for proven optional bounded dimensions.
- Point-in-time gauges preserve their complete emitted identity, including multiprocess `pid` and deployment-defined labels.
- Runtime validation proves no instance/dimension collision or lifecycle-cap loss across the structural union.

## Runtime evidence boundary

The private observed scrape predates the final 159-chart profile and proves only the enabled local surface. It remains
local-only historical evidence, not the authoritative completion verdict. Live deployment validation is an operational
rollout check, not part of this source-completeness claim.

## Reproducible artifacts

- Profile: `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/litellm.yaml`.
- Fixture: `src/go/testdata/prometheus/profiles/litellm/fixtures/litellm_all_metrics.prom`.
- Job input: `src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION-JOB.yaml`.
- Semantic proof: `OPERATOR-MODEL.md` and
  `src/go/testdata/prometheus/profiles/litellm/SOURCE-INVENTORY.tsv`.
- Evidence provenance: `EVIDENCE.md`.
- External evidence manifest: `src/go/testdata/prometheus/profiles/litellm/manifest.yaml`.
- Machine descriptor and integrity metadata: `proof.yaml`.

From `src/go`, reproduce the authoritative result with:

```sh
go run ./tools/prometheus-profile-validation \
  --profile plugin/go.d/config/go.d/prometheus.profiles/default/litellm.yaml \
  --dump testdata/prometheus/profiles/litellm/fixtures/litellm_all_metrics.prom \
  --job plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION-JOB.yaml \
  --output text
```
