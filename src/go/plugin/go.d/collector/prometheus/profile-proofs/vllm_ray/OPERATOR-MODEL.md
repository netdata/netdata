<!-- markdownlint-disable MD013 MD043 MD060 -->

# vLLM Ray Prometheus operator model

## Evidence boundary

- **vLLM source:** `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- **Ray transport source:** `ray-project/ray @ 03491225d59a1ffde99c3628969ccf456be13efd` (Ray 2.48.0).
- **Synthetic exposition:** the committed fixture is a sanitized structural union derived from both sources. It proves names,
  types, labels, optional capabilities, alias behavior, and routing; it is not a live observation.
- **Live boundary:** the local vLLM service uses the native transport. Ray-specific runtime enablement remains unobserved locally
  and is not claimed.

## Entity containment

1. **Ray vLLM application:** owns the profile and deployment/session metadata.
2. **Model engine replica worker:** identity `{model_name, engine, ReplicaId, WorkerId}`. Ray adds replica and worker identity to
   the native vLLM model-engine contract; these labels MUST remain instance identity rather than dimensions.
3. **Deployment metadata:** `Version`, `SessionName`, and `Component` describe the Ray owner and remain promoted chart labels.
4. **Bounded outcomes:** finish reason, wait reason, token source, transfer type, connector operation/status, and speculative
   position are dimensions because they classify work without changing the measured engine-replica-worker entity.

## Causal capability order

1. **Request Lifecycle:** outcomes, end-to-end/first-token latency, and request parameters.
2. **Scheduler:** running/waiting work, wait reasons, preemption, sleep state, and queue time.
3. **Prefill:** prompt sources, request size, computed KV work, and prefill time.
4. **Decode:** generated work, output size, inter-token/decode timing, and concurrency.
5. **Engine Execution:** inference/step work and estimated GPU compute and memory traffic.
6. **KV Cache and Residency:** occupancy, cache outcomes, and sampled block lifetime/idle/reuse.
7. **KV Offloading:** load/store traffic, timing, CPU-tier utilization/allocation, admission outcomes, and tiered lookup delay.
8. **NIXL, HF3FS, and Mooncake:** connector-local volume, status, failures, and duration.
9. **Speculative and Diffusion Decoding:** mutually feature-gated draft/acceptance work or denoising/canvas/commit work.

The Ray endpoint does not expose the native service's HTTP, Python-GC, or process families through the `ray_vllm_*` contract;
those are not fabricated in this profile.

## Population and unit rules

- Counters use `incremental`; Netdata owns rate calculation and reset detection.
- Gauges use `absolute`; fractional cache-use gauges are rendered as percentages.
- Histogram buckets render non-overlapping `observations/s` heatmaps; counts render their exact observation population; sums
  remain accumulated source units per second. The profile does not label `_sum / _count` as an average.
- Metrics from different engines, replicas, or workers are never silently merged.

## Alias and exclusion contract

- Ray 2.48 exports 33 deprecated unsuffixed counter-gauge aliases alongside canonical `_total` counters by default; setting
  `RAY_EXPORT_COUNTER_AS_GAUGE=0` disables those aliases. The recommended job and
  profile deny the aliases so the same observation is not shown twice.
- The pre-canonical KV-offload duplicates are denied by exact source-known names: the recommended job enumerates
  `ray_vllm_kv_offload_size_{bucket,count,sum}`, `ray_vllm_kv_offload_total_bytes`,
  `ray_vllm_kv_offload_total_bytes_total`, `ray_vllm_kv_offload_total_time`, and
  `ray_vllm_kv_offload_total_time_total`; the stock profile uses the corresponding exact source-family bases, including
  `ray_vllm_kv_offload_size`. Unknown future KV-offload families or suffixes remain eligible for generic fallback.
- `ray_vllm_cache_config_info` and `ray_vllm_lora_requests_info` are writer-ineligible information gauges. Their configuration
  question cannot be represented by the collector writer and is recorded as a pipeline limitation.
- Lost question for alias denial: the deprecated spelling itself. No distinct operational population is lost because the
  canonical family is sourced from the same observation.

## Reconciliation ledger

### Forward fallback boundary

- The source-complete fixture has zero generic fallback and zero unmatched series.
- The explicit deny list defensively suppresses compatibility gauges and pre-canonical aliases when job policy does not.
- Unknown future `ray_vllm_*` families remain generically visible until source evidence can assign identity, unit,
  population, owner, and a curated destination.

- `src/go/testdata/prometheus/profiles/vllm_ray/SOURCE-INVENTORY.tsv` accounts for all 116 declared source families and their
  final chart or exclusion disposition.
- The profile has 104 authored charts and 142 selector routes. The structural union materializes 113 chart instances and
  698 dimensions.
- The exact validation has zero fallback, unmatched series, dead charts, dead dimensions, materialization loss, or collisions.

## Binding per-family semantics

`src/go/testdata/prometheus/profiles/vllm_ray/SOURCE-INVENTORY.tsv` is the binding exact-selector ledger. Its 180 rows
account for all 116 source families and all 142
authored selectors. Each row states the closest operator owner, model-engine replica-worker identity, role, exact observation
population from source HELP/update evidence, alias or histogram relationship, rendered unit algebra, identity/dimension/routing
label roles, availability gate, evidence boundary, disposition, destination, and pinned source path. The 36 job exclusions are
compatibility aliases or pre-canonical duplicates; the two writer-ineligible information gauges retain their lost
configuration questions. Unresolved families and authored selectors: **0**.
