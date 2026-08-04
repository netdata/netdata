<!-- markdownlint-disable MD013 MD043 -->

# vLLM Ray Prometheus profile validation

## Bounded source scope

- vLLM: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Ray transport: `ray-project/ray @ 03491225d59a1ffde99c3628969ccf456be13efd` (Ray 2.48.0).
- The committed fixture is source-derived synthetic evidence, not a live Ray observation.

## Authoritative structural-union result

- Verdict: **PASS**.
- Input: **116 raw families / 148 logical series**.
- Writer/profile: **698 writer series**, **104 authored charts**, **113 runtime chart instances**, and
  **698 dimensions**.
- Pipeline exclusions: **38 raw families / 50 logical identities**; exact ledger dispositions are 36 job-excluded
  compatibility/duplicate routes and 2 writer-ineligible information routes.
- Generic fallback, unmatched series, dead charts/dimensions, materialization loss, and collisions: **0**.
- `src/go/testdata/prometheus/profiles/vllm_ray/SOURCE-INVENTORY.tsv` maps all 142 exact authored selector routes; unresolved
  families/selectors **0**.

## Alias contract

Ray 2.48 exports unsuffixed counter-as-gauge compatibility aliases by default; `RAY_EXPORT_COUNTER_AS_GAUGE=0` disables them.
The recommended job denies the 33 aliases and the exact pre-canonical KV-offload exposition names recorded in
`VALIDATION-JOB.yaml`, retaining canonical `_total` and load/store families. This loses only deprecated spellings, not distinct
observations; unknown future names remain outside the exact deny list.

## Forward compatibility

- The source-complete fixture has zero generic fallback and zero unmatched series.
- The explicit deny list defensively suppresses compatibility gauges and pre-canonical aliases when job policy does not.
- Unknown future `ray_vllm_*` families remain generically visible until source evidence can assign identity, unit,
  population, owner, and a curated destination.

## Runtime evidence boundary

The local service uses the native vLLM transport. Ray-specific runtime enablement is unobserved and is not claimed. The
source-complete synthetic PASS proves the contract; live Ray evidence can be added when such an endpoint is available.

## Reproducible artifacts

- Profile: `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm_ray.yaml`.
- Fixture: `src/go/testdata/prometheus/profiles/vllm_ray/fixtures/vllm_ray_all_metrics.prom`.
- Job input: `src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION-JOB.yaml`.
- Semantic proof: `OPERATOR-MODEL.md` and
  `src/go/testdata/prometheus/profiles/vllm_ray/SOURCE-INVENTORY.tsv`.
- Evidence provenance: `EVIDENCE.md`.
- External evidence manifest: `src/go/testdata/prometheus/profiles/vllm_ray/manifest.yaml`.
- Integrity manifest: `SHA256SUMS.tsv`.

From `src/go`, reproduce the authoritative result with:

```sh
go run ./tools/prometheus-profile-validation \
  --profile plugin/go.d/config/go.d/prometheus.profiles/default/vllm_ray.yaml \
  --dump testdata/prometheus/profiles/vllm_ray/fixtures/vllm_ray_all_metrics.prom \
  --job plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION-JOB.yaml \
  --output text
```
