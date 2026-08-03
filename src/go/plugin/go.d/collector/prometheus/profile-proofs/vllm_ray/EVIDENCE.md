<!-- markdownlint-disable MD013 MD043 -->

# vLLM Ray profile evidence manifest

## Supported source boundary

- vLLM metric contract: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Ray transport contract: `ray-project/ray @ 03491225d59a1ffde99c3628969ccf456be13efd` (Ray 2.48.0).
- Primary evidence: vLLM registration paths listed in the native vLLM proof plus Ray
  `src/ray/stats/metric_exporter.cc` and the Ray metrics-export contract at the pinned revisions.
- Upstream vLLM documentation revision: `docs/design/metrics.md` and `docs/usage/metrics.md` at the pinned vLLM revision.

## Feature and configuration gates

- Ray adds `ReplicaId`, `WorkerId`, `SessionName`, `Version`, and `Component` transport labels to enabled vLLM families.
- `RAY_EXPORT_COUNTER_AS_GAUGE=0` disables Ray 2.48's unsuffixed counter-as-gauge compatibility aliases. The default-enabled
  aliases are exact duplicate observations and are excluded by `VALIDATION-JOB.yaml`.
- Native vLLM feature and connector gates still determine which underlying families exist.
- The Ray node endpoint may contain unrelated Ray system families; the validation job admits the complete `ray_vllm_*`
  namespace rather than closing it to current names.

## Evidence classes and fixture provenance

- `src/go/plugin/go.d/collector/prometheus/testdata/vllm_ray_all_metrics.prom` is a sanitized source-derived structural
  union. No live Ray deployment was used or is claimed.
- The fixture preserves Ray replica/worker identity and includes mutually optional vLLM features plus the source-proven alias
  behavior with placeholder values.
- `SOURCE-INVENTORY.tsv` binds every declared family/component to source, owner, identity, population, unit algebra,
  availability gate, disposition, and authored destination.

## Reproduction and integrity

- `VALIDATION-JOB.yaml` is the sanitized objective-validator input corresponding to the recommended metadata job policy.
- `VALIDATION.md` contains the authoritative result and exact command.
- `SHA256SUMS.tsv` fingerprints every committed semantic and executable proof input.
- The result requires zero generic fallback and zero unmatched series for the declared source union. Unknown future
  `ray_vllm_*` families remain eligible for generic fallback.

## Explicit limitations

- The structural union is not one realizable Ray endpoint.
- Local runtime evidence covers native vLLM only; Ray-specific live enablement, values, cadence, and cardinality are unobserved.
- Live-Agent behavior is validated separately during an authorized rollout and cannot replace source-completeness evidence.
