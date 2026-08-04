<!-- markdownlint-disable MD013 MD043 -->

# vLLM Prometheus profile validation

## Bounded source scope

- Running-source base: `vllm-project/vllm @ adf15cadb9d0151663b001a7286674892c4daa3c`.
- Current-source comparison: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Evidence classes remain separate: private observed scrape, pinned primary source, and the committed sanitized structural-union
  fixture. The union combines mutually exclusive optional features and is not one realizable endpoint.

## Authoritative structural-union result

- Verdict: **PASS**.
- Input: **183 raw families / 262 logical series**.
- Writer/profile: **778 writer series**, **124 authored charts**, **142 runtime chart instances**, and
  **778 dimensions**.
- Loss boundary: **90 pipeline-excluded raw families / 130 logical identities**, all reconciled to binding job/writer
  dispositions.
- Generic fallback, unmatched series, dead charts/dimensions, lifecycle loss, and ID/context/dimension collisions: **0**.
- Exact semantic ledger: 253 rows; 163 chart routes, 85 job exclusions, and
  5 writer-ineligible routes; unresolved families/selectors **0**.

## Job-policy contract

The exact proof uses `VALIDATION-JOB.yaml`. It excludes the exact generated `_created` epochs in the declared source union, raw
process-start epoch, and exact pre-canonical KV-offload aliases. Canonical load/store families and process resources remain
charted. The profile retains the same exact known-family exclusions as a defensive fallback boundary. Unknown future
`vllm:*` families remain eligible for generic fallback until their semantics can be curated.

## Runtime evidence boundary

The pre-rollout private scrape proves the local native transport and enabled feature subset only. Strict validation of a
partial runtime scrape can report dead optional charts/dimensions after a holistic profile is authored; that is not a
collector failure. Live deployment validation is an operational rollout check, not part of this source-completeness claim.

## Reproducible artifacts

- Profile: `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm.yaml`.
- Fixture: `src/go/testdata/prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom`.
- Job input: `src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION-JOB.yaml`.
- Semantic proof: `OPERATOR-MODEL.md` and
  `src/go/testdata/prometheus/profiles/vllm/SOURCE-INVENTORY.tsv`.
- Evidence provenance: `EVIDENCE.md`.
- External evidence manifest: `src/go/testdata/prometheus/profiles/vllm/manifest.yaml`.
- Integrity manifest: `SHA256SUMS.tsv`.

From `src/go`, reproduce the authoritative result with:

```sh
go run ./tools/prometheus-profile-validation \
  --profile plugin/go.d/config/go.d/prometheus.profiles/default/vllm.yaml \
  --dump testdata/prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom \
  --job plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION-JOB.yaml \
  --output text
```
