<!-- markdownlint-disable MD013 MD043 MD060 -->

# vLLM Prometheus operator model

## Responsibility

This document owns human semantic decisions: operator domains, entity identity, causal order, observation populations, unit
algebra, and the reasons for exclusions. `EVIDENCE.md` owns provenance. The external source inventory owns exact
family/selector dispositions. `proof.yaml` and replay tests own all machine-verifiable behavior and counts.

## Entity model

1. **vLLM service**
   - Identity: the collector job; no metric label is required.
   - Owns HTTP service-wide timing and process/Python runtime health.
2. **Model engine**
   - Identity: `{model_name, engine}`.
   - `engine` is an EngineCore/data-parallel index, not a tensor-parallel GPU rank.
   - Owns request lifecycle, scheduling, prefill, decode, execution, KV cache, and decoding-mode state.
3. **HTTP endpoint**
   - Identity: `{handler, method}`.
   - `status` is a bounded outcome dimension, not identity.
4. **Tool-parser operation**
   - Identity: `{model_name, request_type, mode}`.
   - It cannot require `engine`, which the exporter does not provide; `outcome` is a bounded dimension.
5. **Connector operation**
   - Core connector views inherit model-engine identity.
   - Mooncake operation/status views add only the source axes needed to distinguish their observation populations.

## Causal capability order

1. Request Lifecycle: outcomes, end-to-end/first-token latency, and request parameters.
2. Scheduler: running/waiting state, wait reasons, preemption, sleep state, and queue time.
3. Prefill: prompt sources, request size, computed KV work, and prefill time.
4. Decode: generated work, output size, inter-token/decode timing, and concurrency.
5. Engine Execution: inference/step work and estimated GPU compute/memory traffic.
6. KV Cache and Residency: occupancy, cache outcomes, block lifetime, idle time, and reuse gaps.
7. KV Offloading: transfers, timing, CPU-tier capacity/admission, and lookup delay.
8. NIXL, HF3FS, and Mooncake: connector-local volume, status, failures, and duration.
9. Speculative and Diffusion Decoding: feature-specific draft/acceptance or denoising/canvas/commit work.
10. WebSocket Service: realtime speech connection state and duration.
11. HTTP Endpoints and Service: per-route outcomes/coarse latency and global high-resolution latency.
12. Tool Parsing: parser invocation outcomes at parser-specific identity.
13. Runtime: process resources and Python garbage collection.

Signals remain with the component or lifecycle boundary that can explain them. The model intentionally avoids global
“Latency”, “Throughput”, “Distribution”, or “Errors” drawers.

## Population and unit rules

- Gauges use absolute semantics except fractional cache-use gauges, which are rendered as percentages.
- Counters use incremental semantics; Netdata owns rate calculation and reset detection.
- Histogram buckets render non-overlapping observation-rate heatmaps.
- Histogram counts retain the exact population counted by the source. Parent requests, engine requests/choices, connector
  calls, and block samples are not treated as interchangeable workload counts.
- Histogram sums retain accumulated source units per second. A duration sum is described as in-flight concurrency only when
  its source population is true elapsed phase or end-to-end time.
- Token/count histogram sums render their exact tokens or sequences per second.
- The profile cannot compute `_sum / _count`; no sum is presented as an average.
- Resident and virtual memory, and open descriptors and their limits, stay separate because they answer different questions.

## Identity and cardinality policy

- Core model-engine charts use `{model_name, engine}`.
- HTTP endpoint charts use `{handler, method}`; different routes are not aggregated.
- HTTP service, process, and Python runtime charts are service-level.
- Tool parser charts use `{model_name, request_type, mode}` because `engine` is absent.
- Dynamic dimensions are limited to source- or configuration-bounded outcomes such as finish/wait reason, sleep state,
  token source, parser outcome, speculative position, HTTP status, connector status, and histogram boundary.
- Labels needed for entity uniqueness stay in identity even when this increases chart instances. Cardinality is not reduced by
  combining unrelated model engines, endpoints, or workers.

## Optional capability semantics

- KV residency histograms sample eviction-related lifetimes, idle periods, and reuse gaps; they do not represent all blocks.
- KV offload families distinguish transfer direction, medium/tier, admission outcome, capacity, and lookup phase.
- NIXL, HF3FS, and Mooncake observations remain connector-local. Operation counts, touched keys, failed keys, bytes, and
  durations are distinct populations.
- Speculative decoding measures drafts and accepted tokens; diffusion decoding measures its native denoising/canvas/commit
  work instead of being forced into speculative terminology.
- Realtime speech is a service-level WebSocket surface, not a model-engine metric unless the exporter provides that identity.
- HTTP and tool-parser metrics keep identities distinct from model-engine metrics when their label contracts differ.

## Exclusion semantics

- Source-known creation epochs and process-start time cannot be transformed into age. The lost question is recorded rather
  than displaying a raw Unix timestamp as operational state.
- Writer-ineligible information gauges lose Python, cache-configuration, or LoRA metadata. Their questions require a
  supported metadata path, not a numeric chart.
- Pre-canonical KV-offload aliases are excluded because source updates them from the same observations as canonical load/store
  families. Unknown future offload names are not assumed to be duplicates.
- HTTP request/response-size summaries without quantiles are rejected as complete logical summaries by the writer. Their
  apparent sum/count suffixes are not independently charted.
- No metric is excluded solely because it is optional or absent from the observed runtime endpoint.

## Dashboard reconciliation

The authored dashboard follows the causal order above. Within each owner, state/workload/outcomes precede distributions,
exact observation counts, and accumulated sums. Core engine, endpoint, parser, connector-operation, and global service
surfaces use their separately proven identities rather than one universal label key.

The source inventory is the binding exact mapping from source family and logical component to chart, profile exclusion, or
writer limitation. It also records availability gates, lost questions, and source paths. This document intentionally does not repeat
that per-family ledger.

## Forward compatibility

Exact exclusions cover source-proven epochs, duplicates, and writer-ineligible routes only. Unknown future `vllm:*` families
remain eligible for generic fallback until evidence can assign ownership, identity, population, units, and a curated
destination.
