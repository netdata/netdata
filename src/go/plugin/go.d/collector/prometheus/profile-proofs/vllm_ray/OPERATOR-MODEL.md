<!-- markdownlint-disable MD013 MD043 MD060 -->

# vLLM Ray Prometheus operator model

## Responsibility

This document owns human semantic decisions: operator domains, entity identity, causal order, observation populations, units,
and the reasons for exclusions. `EVIDENCE.md` owns provenance. The external source inventory owns exact family/selector
dispositions. `proof.yaml` and replay tests own all machine-verifiable behavior and counts.

## Entity containment

1. **Ray vLLM application** owns the profile and deployment/session metadata.
2. **Model-engine replica worker** uses `{model_name, engine, ReplicaId, WorkerId}` identity. Ray replica and worker labels are
   entity identity, not dimensions.
3. **Deployment metadata** such as version, session, and component describes the Ray owner and remains promoted chart
   metadata.
4. **Bounded outcomes** such as finish/wait reason, token source, transfer type, connector status, and speculative position are
   dimensions because they classify work without changing the engine-replica-worker entity.

## Causal capability order

1. Request Lifecycle: outcomes, end-to-end/first-token latency, and request parameters.
2. Scheduler: running/waiting work, wait reasons, preemption, sleep state, and queue time.
3. Prefill and Decode: input/output token work, request size, KV computation, and phase timing.
4. Engine Execution: inference/step work and estimated GPU compute/memory traffic.
5. KV Cache and Residency: occupancy, outcomes, and sampled lifetime/idle/reuse distributions.
6. KV Offloading: transfers, timing, CPU-tier capacity/admission, and lookup delay.
7. NIXL, HF3FS, and Mooncake: connector-local volume, status, failures, and duration.
8. Speculative and Diffusion Decoding: feature-specific draft/acceptance or denoising/canvas/commit work.

The Ray-vLLM transport does not imply native HTTP, Python-GC, or process families. The profile does not fabricate them.

## Population and unit rules

- Counters use incremental semantics; Netdata owns rate calculation and reset detection.
- Gauges use absolute semantics; fractional cache-use gauges render as percentages.
- Histogram buckets render observation-rate heatmaps. Counts retain their exact observation population, and sums retain
  accumulated source units per second.
- No histogram sum is labeled as an average because the profile cannot compute `_sum / _count`.
- Metrics from different engines, replicas, or workers are never silently merged.

## Alias and exclusion semantics

- Ray counter-as-gauge compatibility aliases are duplicate spellings of canonical counter observations. The recommended job
  removes the source-known aliases while retaining canonical families.
- Pre-canonical KV-offload aliases are excluded only where source proves that canonical load/store families represent the
  same observations.
- Writer-ineligible information gauges retain their lost cache/LoRA configuration questions in the source inventory rather
  than being presented as numeric state.
- Losing a deprecated spelling is acceptable; losing a distinct operational population is not.

## Dashboard reconciliation

The source inventory is the binding exact mapping from source family and logical component to chart, job exclusion, or writer
limitation. It records owner, engine-replica-worker identity, observation population, relationships, units, label roles,
availability gates, lost questions, destinations, and source paths. This document intentionally does not repeat that
per-family ledger.

## Forward compatibility

Exact exclusions cover source-proven compatibility aliases, duplicates, and writer-ineligible routes only. Unknown future
`ray_vllm_*` families remain eligible for generic fallback until evidence can assign ownership, identity, population, units,
and a curated destination.
