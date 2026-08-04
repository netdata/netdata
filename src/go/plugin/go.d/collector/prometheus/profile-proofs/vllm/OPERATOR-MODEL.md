<!-- markdownlint-disable MD013 MD043 MD060 -->

# vLLM Prometheus Operator Model

## Evidence boundary

- Runtime evidence: one read-only scrape from the running custom vLLM service, captured on 2026-07-30.
- Running source base: `vllm-project/vllm @ adf15cadb9d0151663b001a7286674892c4daa3c`.
- Current-source comparison: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Raw evidence is local-only because it contains a served-model label, real handler paths, configuration labels, and operating
  values.
- The observed tool-parser metric is an official feature-gated vLLM surface. It is curated because both the running endpoint
  and current upstream source prove it, but the profile does not use it as the automatic-selection signature.
- Source-derived structural evidence: the committed fixture is a sanitized synthetic union. It combines optional connectors,
  model modes, and feature gates that need not coexist in one deployment.
- Source-only optional families are curated from exact registrations and update callsites. Their fixture proves names, types,
  labels, histogram shape, identity planning, and profile routing; it does not prove live enablement, values, cadence, or
  cardinality.
- Deprecated CPU-offload aliases are emitted from the same observations as canonical load/store families. The recommended job
  policy excludes them to prevent double counting.

## Operator domain model

### Entity containment

1. **vLLM service**
   - Identity: the collector job; no metric label is required.
   - Owns HTTP service-wide timing and host-process/Python runtime health.
2. **Model engine**
   - Identity: `{model_name, engine}`.
   - `engine` is an EngineCore/data-parallel index, not a tensor-parallel GPU rank.
   - Owns request lifecycle, scheduler, prefill, decode, execution, KV cache, and speculative-decoding state.
3. **HTTP endpoint**
   - Identity: `{handler, method}`.
   - Parent: vLLM service.
   - `status` is a bounded outcome dimension, not identity.
4. **Tool-parser operation**
   - Identity: `{model_name, request_type, mode}`.
   - Parent relation is descriptive rather than an identity lattice because the exporter does not provide `engine`.
   - `outcome` is a bounded dimension.

### Causal capabilities and stages

1. **Request Lifecycle** owns request outcomes, end-to-end and time-to-first-token latency, and
   admission/request-parameter distributions.
2. **Scheduler** owns running/waiting state, waiting reasons, preemption, sleep state, and queue time.
3. **Prefill** owns prompt-token sourcing, prompt size, newly computed KV tokens, and prefill duration.
4. **Decode** owns generated-token work, output size, decode duration, ITL, and per-request mean output-token time.
5. **Engine Execution** owns aggregate inference duration, engine-step batch size, and estimated per-GPU compute/memory traffic.
6. **KV Cache** owns occupancy and local/external prefix-cache and multimodal-cache behavior.
7. **KV Cache Residency** owns sampled block lifetime, idle-before-eviction, and reuse-gap distributions.
8. **KV Offloading** owns GPU/offload-medium transfers, CPU-tier capacity/admission, and lookup delays.
9. **KV Connectors** own connector-local transfers and outcomes for NIXL, HF3FS, and Mooncake Store.
10. **Speculative Decoding** owns drafts, proposed tokens, accepted tokens, and position-specific acceptance.
11. **Diffusion Decoding** owns denoising steps, canvas positions, and committed tokens for diffusion models.
12. **WebSocket Service** owns realtime speech-to-text connection state and duration.
13. **HTTP Endpoints** owns per-handler/method request outcomes and coarse endpoint latency.
14. **HTTP Service** owns global high-resolution request latency.
15. **Tool Parsing** owns parsing invocation outcomes at its own observed identity.
16. **Runtime** owns process CPU/memory/file-descriptor and Python garbage-collector behavior.

The model deliberately does not use application-wide `Latency`, `Throughput`, `Distribution`, or `Errors` drawers. Each signal
stays with the component or lifecycle boundary that can explain it.

## Capability completeness matrix

| Owner | Workload/outcomes | Errors | Latency/saturation/capacity | Resource/config evidence |
|---|---|---|---|---|
| Request Lifecycle | observed | source-only corrupted requests | observed lifecycle distributions | not exported |
| Scheduler | observed | preemptions observed | observed queue/wait/sleep state | not exported |
| Prefill | observed token work/source | not exported separately | observed duration/concurrency | cache effects are separate |
| Decode | observed token work | not exported separately | observed duration/ITL/concurrency | not exported |
| Engine Execution | observed steps/work estimates | not exported separately | observed inference concurrency | source-defined MFU estimates |
| KV Cache | observed query/hit work | not exported separately | observed occupancy | cache config is writer-skipped |
| KV Cache Residency | source-only sampled observations | eviction is not classified as error | source-only lifetime/idle/reuse | sample rate is configuration-gated |
| KV Offloading | source-only transfers/lookups | source-only failure/skip outcomes | source-only CPU capacity and delays | connector/configuration-gated |
| NIXL Connector | source-only transfers | source-only transfer/notification/expiry errors | source-only transfer/post distributions | connector-gated |
| HF3FS Connector | source-only save/load work | source-only failures | source-only duration distributions | connector-gated |
| Mooncake Connector | source-only calls/keys/bytes | source-only failed keys/status | source-only operation duration | connector-gated |
| Speculative Decoding | observed drafts/tokens | not exported | acceptance can be derived downstream | speculative-config-gated |
| Diffusion Decoding | source-only steps/positions/tokens | not exported | not exported | diffusion-model-gated |
| WebSocket Service | source-only connections | not exported separately | source-only active/duration | realtime speech route-gated |
| HTTP/Tool Parsing | observed | outcomes observed | observed HTTP duration | endpoint/parser labels observed |
| Runtime | observed GC/process work | uncollectable GC objects observed | FD capacity observed | configuration labels are writer-skipped |

## Source-only optional family ledger

Unless stated otherwise, these families use `{model_name, engine}` identity. Every row is authoritative/source-derived and is
materialized in the synthetic structural union, not claimed as a live observation.

| Source family | Shape/population and relationship | Gate | Destination |
|---|---|---|---|
| `vllm:corrupted_requests` | counter; requests whose logits contain NaNs | `VLLM_COMPUTE_NANS_IN_LOGITS` | Request Lifecycle / Corrupted Requests |
| `vllm:kv_block_lifetime_seconds` | histogram; one sampled lifetime per evicted block | `--kv-cache-metrics` and sample rate | KV Cache Residency / Lifetime |
| `vllm:kv_block_idle_before_evict_seconds` | histogram; one sampled idle period for the same eviction population | same | KV Cache Residency / Idle Before Eviction |
| `vllm:kv_block_reuse_gap_seconds` | histogram; zero or more recent access gaps per sampled eviction; not additive with eviction counts | same | KV Cache Residency / Reuse Gap |
| `vllm:diffusion_num_denoising_steps` | counter; denoising steps | diffusion model; replaces speculative draft count | Diffusion Decoding / Denoising Steps |
| `vllm:diffusion_num_canvas_positions` | counter; evaluated canvas positions | diffusion model; replaces speculative draft-token count | Diffusion Decoding / Canvas Positions |
| `vllm:diffusion_num_committed_tokens` | counter; finalized tokens | diffusion model; replaces speculative accepted-token count | Diffusion Decoding / Committed Tokens |
| `vllm:websocket_connections_active` | gauge; currently active accepted realtime connections; global identity | realtime speech WebSocket route | WebSocket Service / Active |
| `vllm:websocket_connections_total` | counter; opened realtime connections; global identity | same | WebSocket Service / Lifecycle |
| `vllm:websocket_connection_duration_seconds` | histogram; one completed connection; `_count` is closed connections | same | WebSocket Service / Duration and Lifecycle |
| `vllm:kv_offload_load_bytes` | counter; bytes loaded to GPU | offloading connector | KV Offloading / Byte Accounting |
| `vllm:kv_offload_load_time` | counter; accumulated load seconds | same | KV Offloading / Transfer Concurrency |
| `vllm:kv_offload_load_size` | histogram; one load operation size; `_sum` equals canonical load-byte work | same | KV Offloading / Load Size, Operations, Byte Accounting |
| `vllm:kv_offload_store_bytes` | counter; bytes stored from GPU | same | KV Offloading / Byte Accounting |
| `vllm:kv_offload_store_time` | counter; accumulated store seconds | same | KV Offloading / Transfer Concurrency |
| `vllm:kv_offload_store_size` | histogram; one store operation size; `_sum` equals canonical store-byte work | same | KV Offloading / Store Size, Operations, Byte Accounting |
| `vllm:kv_offload_lookup_sync_delay_seconds` | histogram; one synchronous offload lookup call | offloading connector | KV Offloading / Sync Lookup |
| `vllm:kv_offload_lookup_async_delay_seconds` | histogram; one deferred request from first deferral until resolution/finish | same | KV Offloading / Async Lookup |
| `vllm:kv_offload_allocation_failure` | counter; failed store allocation attempts | same | KV Offloading / Admission Outcomes |
| `vllm:kv_offload_cpu_cache_usage_perc` | gauge; CPU KV space pinned by active transfers | CPU/tiered offloading | KV Offloading / CPU Cache Usage |
| `vllm:kv_offload_cpu_cache_write_usage_perc` | gauge; CPU KV space pinned by in-flight stores | current-upstream CPU/tiered offloading | same |
| `vllm:kv_offload_cpu_cache_read_usage_perc` | gauge; CPU KV space pinned by in-flight loads | current-upstream CPU/tiered offloading | same |
| `vllm:kv_offload_cpu_allocation_size` | histogram; CPU blocks requested by one `prepare_store` | CPU/tiered offloading | KV Offloading / CPU Allocation |
| `vllm:kv_offload_stores_skipped` | counter; stores rejected by the reuse threshold | CPU/tiered offloading with threshold ≥2 | KV Offloading / Admission Outcomes |
| `vllm:kv_offload_tiering_lookup_sync_delay_seconds` | histogram; per-request blocking time across secondary tiers | current-upstream tiered offloading | KV Offloading / Tiered Sync Lookup |
| `vllm:kv_offload_tiering_lookup_async_delay_seconds` | histogram; per-request deferred secondary-tier wall time | same | KV Offloading / Tiered Async Lookup |
| `vllm:kv_offload_total_bytes` | deprecated counter alias partitioned by `transfer_type`; duplicates canonical load/store bytes | CPU offloading compatibility | job-policy exclusion |
| `vllm:kv_offload_total_time` | deprecated counter alias partitioned by `transfer_type`; duplicates canonical load/store time | same | job-policy exclusion |
| `vllm:kv_offload_size` | deprecated histogram alias partitioned by `transfer_type`; duplicates canonical size histograms | same | job-policy exclusion |
| `vllm:nixl_xfer_time_seconds` | histogram; one NIXL transfer duration | NIXL connector | NIXL / Transfer Duration |
| `vllm:nixl_post_time_seconds` | histogram; one NIXL transfer-post duration | same | NIXL / Post Time |
| `vllm:nixl_bytes_transferred` | histogram; bytes in one transfer | same | NIXL / Size and Throughput |
| `vllm:nixl_num_descriptors` | histogram; descriptors in one transfer | same | NIXL / Descriptors |
| `vllm:nixl_num_failed_transfers` | counter; failed transfers | same | NIXL / Failures |
| `vllm:nixl_num_failed_notifications` | counter; failed notifications; independent from transfer failures | same | NIXL / Failures |
| `vllm:nixl_num_kv_expired_reqs` | counter; requests whose KV expired; P-instance population | same | NIXL / Expired Requests |
| `vllm:hf3fs_save_duration_seconds` | histogram; one successful save duration | HF3FS connector | HF3FS / Save Duration |
| `vllm:hf3fs_load_duration_seconds` | histogram; one successful load duration | same | HF3FS / Load Duration |
| `vllm:hf3fs_num_failed_save` | counter; failed saves | same | HF3FS / Failures |
| `vllm:hf3fs_num_failed_load` | counter; failed loads | same | HF3FS / Failures |
| `vllm:mooncake_store_operation_time_seconds` | histogram by `operation,status`; one store RPC | Mooncake Store connector | Mooncake / Timing |
| `vllm:mooncake_store_operation_total` | counter by `operation,status`; same RPC count as the duration `_count` | same | Mooncake / Measurements |
| `vllm:mooncake_store_operation_keys_total` | counter by `operation,status`; keys touched | same | Mooncake / Key Throughput |
| `vllm:mooncake_store_operation_bytes_total` | counter by `operation,status`; bytes transferred | same | Mooncake / Byte Throughput |
| `vllm:mooncake_store_operation_failed_keys_total` | counter by `operation,status`; failed keys, a subset of touched keys | same | Mooncake / Failed Keys |

## Unit and histogram rules

- Gauge: `absolute`; preserve the raw unit except fractional cache use, which is multiplied by 100 and rendered as `%`.
- Counter: `incremental`; the displayed unit is the exact counted object per second.
- Duration histogram:
  - `_bucket`: non-overlapping `observations/s` heatmap.
  - `_count`: exact observation population per second.
  - `_sum`: accumulated `seconds/s`; render as `in-flight` concurrency only for true phase/end-to-end elapsed-time
    observations.
- Token/count histogram:
  - `_bucket`: `observations/s` heatmap.
  - `_count`: exact observation population per second.
  - `_sum`: exact tokens/sequences per second.
- The schema cannot compute `_sum / _count`; no `_sum` chart is labeled as an average.

## Writer-capable family ledger

Unless a row says otherwise, core vLLM metrics use model-engine identity `{model_name, engine}` and those two labels are
instance identity. Histogram `le` is an inferred bounded heatmap dimension.

### Request Lifecycle

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:request_success_total` | counter | Outcome; one finished engine request/choice, incremented on completion | requests → incremental → `requests/s` | `finished_reason` bounded dimension (5 observed) | Request Lifecycle / Outcomes |
| `vllm:time_to_first_token_seconds` | histogram | User-visible lifecycle latency; one engine request from frontend arrival/tokenization start to first output token | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` pre-response concurrency | identity + `le` | Request Lifecycle / Time to First Token, First-Token Requests, Pre-Response Concurrency |
| `vllm:e2e_request_latency_seconds` | histogram | End-to-end latency; one finished engine request from arrival through completion | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | Request Lifecycle / End-to-End Latency, Completed Requests, End-to-End Concurrency |
| `vllm:request_params_n` | histogram | Admission parameter; requested parallel sequence count for one parent request | sequences; bucket `observations/s`, count `parent requests/s`, sum `sequences/s` | identity + `le` | Request Lifecycle / Requested Sequences |
| `vllm:request_params_max_tokens` | histogram | Admission parameter; configured `max_tokens` for one parent request when supplied | tokens; bucket `observations/s`, count `parent requests/s`, sum `tokens/s` | identity + `le` | Request Lifecycle / Requested Token Limit |
| `vllm:request_max_num_generation_tokens` | histogram | Actual outcome; maximum generated tokens among a parent request's completed children | tokens; bucket `observations/s`, count `parent requests/s`, sum `tokens/s` | identity + `le` | Request Lifecycle / Maximum Generated Tokens |

The three parent-request histograms are not interchangeable with engine-request/choice counts. Their distributions stay
separate; their `_count` rates may share a chart only with explicit parent-request dimension names.

### Scheduler

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:num_requests_running` | gauge | Workload; engine requests currently in execution batches | requests → absolute → `requests` | identity | Scheduler / Request State |
| `vllm:num_requests_waiting` | gauge | Saturation; engine requests waiting for scheduling | requests → absolute → `requests` | identity | Scheduler / Request State |
| `vllm:num_requests_waiting_by_reason` | gauge | Saturation detail; current waiters by cause | requests → absolute → `requests` | `reason` bounded dimension (capacity, deferred) | Scheduler / Waiting Reasons |
| `vllm:num_preemptions_total` | counter | Pressure/outcome; one engine preemption | preemptions → incremental → `preemptions/s` | identity | Scheduler / Preemptions |
| `vllm:engine_sleep_state` | gauge state set | State; one mutually described engine sleep state | state → absolute → `state` | `sleep_state` bounded dimension (3 observed) | Scheduler / Sleep State |
| `vllm:request_queue_time_seconds` | histogram | Queue latency; one finished engine request's waiting phase | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | Scheduler / Queue Time, Queued Requests, Queue Concurrency |

`num_requests_waiting_by_reason` is a decomposition of waiting state, while `num_requests_waiting` is the total. They use
separate charts so the overlapping values do not imply additivity.

### Prefill

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:prompt_tokens_total` | counter | Workload; prompt tokens processed by prefill | tokens → incremental → `tokens/s` | identity | Prefill / Prompt Throughput |
| `vllm:prompt_tokens_by_source_total` | counter | Workload decomposition; prompt tokens from local compute, local cache, or external KV transfer | tokens → incremental → `tokens/s` | `source` bounded dimension (3 observed) | Prefill / Prompt Token Sources |
| `vllm:prompt_tokens_cached_total` | counter | Workload summary; cached prompt tokens from local plus external sources | tokens → incremental → `tokens/s` | identity | Prefill / Cached Prompt Throughput |
| `vllm:request_prompt_tokens` | histogram | Request size; prompt tokens in one finished engine request | tokens; bucket `observations/s`, count `requests/s`, sum `tokens/s` | identity + `le` | Prefill / Prompt Size, Measured Requests, Prompt Volume |
| `vllm:request_prefill_time_seconds` | histogram | Stage latency; prefill time for one finished engine request | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | Prefill / Prefill Time, Measured Requests, Prefill Concurrency |
| `vllm:request_prefill_kv_computed_tokens` | histogram | Stage work; newly computed, non-cached KV tokens for one finished engine request | tokens; bucket `observations/s`, count `requests/s`, sum `tokens/s` | identity + `le` | Prefill / Computed KV Tokens, Measured Requests, Computed KV Throughput |

Source proves `local_compute + local_cache_hit + external_kv_transfer = total`, and
`local_cache_hit + external_kv_transfer = cached`. The total and cached summaries remain separate from the source
decomposition to avoid a visually double-counted chart.

### Decode

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:generation_tokens_total` | counter | Workload; output tokens generated | tokens → incremental → `tokens/s` | identity | Decode / Generation Throughput |
| `vllm:request_generation_tokens` | histogram | Output size; generated tokens in one finished engine request | tokens; bucket `observations/s`, count `requests/s`, sum `tokens/s` | identity + `le` | Decode / Output Size, Measured Requests, Output Volume |
| `vllm:request_decode_time_seconds` | histogram | Stage latency; decode time for one finished engine request | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | Decode / Decode Time, Measured Requests, Decode Concurrency |
| `vllm:inter_token_latency_seconds` | histogram | Token-step latency; one interval between output tokens | seconds; bucket `observations/s`, count `intervals/s`, sum `seconds/s` accumulated interval time | identity + `le` | Decode / Inter-Token Latency, Token Intervals, Interval Time |
| `vllm:request_time_per_output_token_seconds` | histogram | Derived per-request mean; one finished request's mean output-token time | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` accumulated means | identity + `le` | Decode / Mean Output-Token Time, Measured Requests, Accumulated Means |

The last `_sum` is not concurrency: each observation is already a per-request mean. ITL counts intervals rather than
requests, so it is not mixed with request counts.

### Engine Execution

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:request_inference_time_seconds` | histogram | Execution latency; one finished engine request's total running phase | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | Engine Execution / Inference Time, Measured Requests, Inference Concurrency |
| `vllm:iteration_tokens_total` | histogram | Batch work; tokens processed in one engine step | tokens; bucket `observations/s`, count `steps/s`, sum `tokens/s` | identity + `le` | Engine Execution / Step Token Count, Engine Steps, Step Throughput |
| `vllm:estimated_flops_per_gpu_total` | counter | Per-GPU execution estimate; estimated floating-point operations performed | FLOP → incremental / 1e9 → `GFLOP/s/GPU` | model-engine identity; no GPU identity label | Engine Execution / Estimated Compute |
| `vllm:estimated_read_bytes_per_gpu_total` | counter | Per-GPU memory estimate; bytes read | bytes → incremental / 1e9 → `GB/s/GPU` | model-engine identity; no GPU identity label | Engine Execution / Estimated Memory Bandwidth |
| `vllm:estimated_write_bytes_per_gpu_total` | counter | Per-GPU memory estimate; bytes written | bytes → incremental / 1e9 → `GB/s/GPU` | model-engine identity; no GPU identity label | Engine Execution / Estimated Memory Bandwidth |

The estimates are reported per GPU but have no GPU label. The profile must not invent GPU instances or treat `engine` as a GPU.

### KV Cache

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:kv_cache_usage_perc` | gauge | Utilization; fraction of KV blocks in use | ratio → absolute ×100 → `%` | identity | KV Cache / Usage |
| `vllm:prefix_cache_queries_total` | counter | Local cache workload; queried prompt tokens | tokens → incremental → `tokens/s` | identity | KV Cache / Local Prefix Cache |
| `vllm:prefix_cache_hits_total` | counter | Local cache outcome; cached tokens served | tokens → incremental → `tokens/s` | identity | KV Cache / Local Prefix Cache |
| `vllm:external_prefix_cache_queries_total` | counter | Cross-instance cache workload; queried tokens | tokens → incremental → `tokens/s` | identity | KV Cache / External Prefix Cache |
| `vllm:external_prefix_cache_hits_total` | counter | Cross-instance cache outcome; cached tokens served | tokens → incremental → `tokens/s` | identity | KV Cache / External Prefix Cache |
| `vllm:mm_cache_queries_total` | counter | Multimodal cache workload; queried items | items → incremental → `items/s` | identity | KV Cache / Multimodal Cache |
| `vllm:mm_cache_hits_total` | counter | Multimodal cache outcome; cached items served | items → incremental → `items/s` | identity | KV Cache / Multimodal Cache |

Query and hit rates share charts only within the same cache and counted object. The exporter exposes counts, not a computed
hit ratio.

### Speculative Decoding

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `vllm:spec_decode_num_drafts_total` | counter | Workload; one speculative draft | drafts → incremental → `drafts/s` | identity | Speculative Decoding / Drafts |
| `vllm:spec_decode_num_draft_tokens_total` | counter | Workload; proposed draft tokens | tokens → incremental → `tokens/s` | identity | Speculative Decoding / Tokens |
| `vllm:spec_decode_num_accepted_tokens_total` | counter | Outcome; accepted draft tokens | tokens → incremental → `tokens/s` | identity | Speculative Decoding / Tokens |
| `vllm:spec_decode_num_accepted_tokens_per_pos_total` | counter | Outcome detail; accepted tokens by draft position | tokens → incremental → `tokens/s` | `position` bounded observed dimension; cardinality follows configured lookahead | Speculative Decoding / Accepted Tokens by Position |

The metrics permit a downstream acceptance ratio but the profile schema cannot compute it.

### HTTP Endpoints and HTTP Service

| Source family | Type/shape | Entity identity | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|---|
| `http_requests_total` | counter | endpoint `{handler, method}` | Outcome; one instrumented HTTP request | requests → incremental → `requests/s` | `status` bounded dimension | HTTP Endpoints / Requests |
| `http_request_duration_seconds` | histogram | endpoint `{handler, method}` | Endpoint latency; one instrumented HTTP request | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | identity + `le` | HTTP Endpoints / Request Duration, Measured Requests, Concurrency |
| `http_request_duration_highr_seconds` | histogram | global vLLM service | Service latency; one instrumented HTTP request, deliberately without endpoint labels | seconds; bucket `observations/s`, count `requests/s`, sum `seconds/s` concurrency | `le` only | HTTP Service / High-Resolution Duration, Measured Requests, Concurrency |
| `http_request_size_bytes` | summary without quantiles | endpoint-like handler only | Request content length observation | writer rejects family; no flattened series | `handler` observed, no `method` | Pipeline limitation; no chart |
| `http_response_size_bytes` | summary without quantiles | endpoint-like handler only | Response content length observation | writer rejects family; no flattened series | `handler` observed, no `method` | Pipeline limitation; no chart |

HTTP counts are HTTP requests, not engine requests/choices. They never share request-rate charts with vLLM lifecycle metrics.
The coarse histogram preserves the two observed endpoint identities; the high-resolution histogram remains global because the
source deliberately omits endpoint labels.

### Tool Parsing

| Source family | Type/shape | Entity identity | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|---|
| `vllm:tool_call_parser_invocations_total` | counter | parser operation `{model_name, request_type, mode}` | Outcome; one non-streaming choice or one streaming delta parser invocation | invocations → incremental → `invocations/s` | `outcome` bounded dimension; no `engine` | Tool Parsing / Invocations |

The metric is observed in the running custom build and treated as optional. Its population is not interchangeable with HTTP
requests or engine requests.

### Runtime

| Source family | Type/shape | Role and observation population | Unit algebra | Label roles | Destination |
|---|---|---|---|---|---|
| `process_virtual_memory_bytes` | gauge | Resource; process virtual address space | bytes → absolute → `bytes` | global | Runtime / Virtual Memory |
| `process_resident_memory_bytes` | gauge | Resource; resident process memory | bytes → absolute → `bytes` | global | Runtime / Resident Memory |
| `process_cpu_seconds_total` | counter | Resource utilization; process CPU time | CPU seconds → incremental → `cores` | global | Runtime / CPU |
| `process_open_fds` | gauge | Resource use; open file descriptors | descriptors → absolute → `fds` | global | Runtime / Open File Descriptors |
| `process_max_fds` | gauge | Capacity; descriptor ceiling | descriptors → absolute → `fds` | global | Runtime / File Descriptor Limit |
| `python_gc_objects_collected_total` | counter | GC outcome; objects collected | objects → incremental → `objects/s` | `generation` bounded dimension | Runtime / Python GC / Collected Objects |
| `python_gc_objects_uncollectable_total` | counter | GC error; uncollectable objects found | objects → incremental → `objects/s` | `generation` bounded dimension | Runtime / Python GC / Uncollectable Objects |
| `python_gc_collections_total` | counter | GC workload; collection cycles | collections → incremental → `collections/s` | `generation` bounded dimension | Runtime / Python GC / Collections |

Resident and virtual memory remain separate because their values answer different questions and differ greatly in scale.
Open descriptors and their limit remain separate for the same reason.

## Excluded and writer-skipped source families

### Creation and process-start epoch gauges

The recommended job policy denies by exact name every creation-epoch family in the declared source union, plus
`process_start_time_seconds`. Each is a Unix epoch timestamp gauge. The profile schema cannot compute `now - epoch`; charting
the raw value would not answer restart age or metric age.

The stock profile defensively denies the same exact, source-known creation family names with the `vllm:` prefix. Unknown
future creation families are deliberately not covered and remain eligible for generic fallback. The job policy additionally
names the exact HTTP creation families and process-start epoch outside the profile's `vllm:*` match scope.

Exact observed creation families, grouped by causal owner:

- Request Lifecycle: `vllm:request_success_created`, `vllm:time_to_first_token_seconds_created`,
  `vllm:request_max_num_generation_tokens_created`,
  `vllm:request_params_n_created`, `vllm:request_params_max_tokens_created`,
  `vllm:e2e_request_latency_seconds_created`.
- Scheduler: `vllm:num_preemptions_created`, `vllm:request_queue_time_seconds_created`.
- Prefill: `vllm:prompt_tokens_created`, `vllm:prompt_tokens_by_source_created`,
  `vllm:prompt_tokens_cached_created`, `vllm:request_prompt_tokens_created`,
  `vllm:request_prefill_time_seconds_created`,
  `vllm:request_prefill_kv_computed_tokens_created`.
- Decode: `vllm:generation_tokens_created`, `vllm:request_generation_tokens_created`,
  `vllm:inter_token_latency_seconds_created`, `vllm:request_time_per_output_token_seconds_created`,
  `vllm:request_decode_time_seconds_created`.
- Engine Execution: `vllm:iteration_tokens_total_created`, `vllm:request_inference_time_seconds_created`,
  `vllm:estimated_flops_per_gpu_created`, `vllm:estimated_read_bytes_per_gpu_created`,
  `vllm:estimated_write_bytes_per_gpu_created`.
- KV Cache: `vllm:prefix_cache_queries_created`, `vllm:prefix_cache_hits_created`,
  `vllm:external_prefix_cache_queries_created`, `vllm:external_prefix_cache_hits_created`,
  `vllm:mm_cache_queries_created`, `vllm:mm_cache_hits_created`.
- Speculative Decoding: `vllm:spec_decode_num_drafts_created`, `vllm:spec_decode_num_draft_tokens_created`,
  `vllm:spec_decode_num_accepted_tokens_created`, `vllm:spec_decode_num_accepted_tokens_per_pos_created`.
- HTTP Endpoints: `http_requests_created`, `http_request_duration_seconds_created`,
  `http_request_size_bytes_created`, `http_response_size_bytes_created`.
- HTTP Service: `http_request_duration_highr_seconds_created`.
- Tool Parsing: `vllm:tool_call_parser_invocations_created`.
- Runtime: `process_start_time_seconds`.

Lost question: “When was this process or metric child created?” A future collector transform could expose age/uptime safely.

### Writer-skipped information gauges

- `python_info`: skipped because the family name ends in `_info`; Python version/implementation labels do not reach metrix.
- `vllm:cache_config_info`: skipped for the same reason; its version-dependent cache-configuration labels cannot be promoted or
  charted.
- `vllm:lora_requests_info`: skipped for the same reason; `max_lora`, waiting-adapter, and running-adapter labels do not reach
  metrix. Its source also warns that the values may be misleading with data parallelism.

Lost questions: “Which Python runtime is serving?”, “Which cache configuration produced this behavior?”, and “Which LoRA
adapters are active or waiting?” These need a supported metadata path, not a profile selector.

### Deprecated CPU-offload aliases

The recommended job selector denies the exact exposition names `vllm:kv_offload_total_bytes_total`,
`vllm:kv_offload_total_time_total`, and `vllm:kv_offload_size_{bucket,count,sum}`. The stock profile defensively denies the
corresponding exact source-family bases, including `vllm:kv_offload_size`. Current CPU offloading updates these compatibility
families from the same load/store observations as the canonical flat metrics. Retaining both would double-present the same
bytes, time, and operation sizes. Unknown future KV-offload families or suffixes remain eligible for generic fallback.

### Writer-rejected summaries

- `http_request_size_bytes`
- `http_response_size_bytes`

Both observed summaries contain `_sum` and `_count` but no quantiles. The Prometheus writer rejects the complete logical
summary, so the apparent scalar suffixes are not independently chartable.

Lost questions: endpoint request and response volume/size. The exporter would need a supported histogram, counters, or
quantile-bearing summary.

## Identity and cardinality decisions

### Forward fallback boundary

- The source-complete fixture has zero generic fallback and zero unmatched series.
- Exact known-family denies defensively suppress generated epochs and pre-canonical aliases when job policy does not.
- Unknown future `vllm:*` families remain generically visible until source evidence can assign identity, unit, population,
  owner, and a curated destination.

- Core model-engine charts: instances by `[model_name, engine]`.
- HTTP endpoint charts: instances by `[handler, method]`; the observed two endpoint identities must not be aggregated.
- HTTP service and process/Python runtime: one service-level aggregate chart.
- Tool parser: instances by `[model_name, request_type, mode]`; `engine` cannot be required because it is absent.
- Dynamic dimensions are limited to source-bounded or configuration-bounded values:
  - `finished_reason`: 5 documented values.
  - `reason`: 2 documented values.
  - `sleep_state`: 3 documented values.
  - `source`: 3 documented values.
  - `outcome`: 2 initialized values.
  - `position`: configured speculative lookahead; 2 observed.
  - `status`: HTTP status values; bounded operational domain but more than one may appear.
  - histogram `le`: exporter-defined bucket boundaries.
- No relabeling, label deletion, wildcard identity, label promotion, or lifecycle caps are needed for the representative dump.

## Expected navigation order

1. Request Lifecycle
2. Scheduler
3. Prefill
4. Decode
5. Engine Execution
6. KV Cache
7. KV Cache Residency
8. KV Offloading
9. NIXL Connector
10. HF3FS Connector
11. Mooncake Connector
12. Speculative Decoding
13. Diffusion Decoding
14. WebSocket Service
15. HTTP Endpoints
16. HTTP Service
17. Tool Parsing
18. Runtime

Within each owner, state/workload/outcomes precede latency distributions, exact observation counts, and accumulated sums.

## Source-surface reconciliation

Status: complete for the bounded source surface.

Primary source revisions:

- `vllm-project/vllm @ adf15cadb9d0151663b001a7286674892c4daa3c`
- `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`

Registration and update paths audited in both revisions include:

- `vllm/v1/metrics/loggers.py`
- `vllm/v1/metrics/perf.py`
- `vllm/v1/spec_decode/metrics.py`
- `vllm/parser/metrics.py`
- `vllm/entrypoints/speech_to_text/realtime/metrics.py`
- `vllm/distributed/kv_transfer/kv_connector/v1/offloading/metrics.py`
- `vllm/distributed/kv_transfer/kv_connector/v1/nixl/stats.py`
- `vllm/distributed/kv_transfer/kv_connector/v1/hf3fs/hf3fs_connector.py`
- `vllm/distributed/kv_transfer/kv_connector/v1/mooncake/store/metrics.py`
- `vllm/v1/kv_offload/cpu/common.py`
- `vllm/v1/kv_offload/tiering/base.py`

A production-source name reconciliation found no registration literal absent from the structural fixture after accounting for
Prometheus client counter suffixing. The only unmatched `vllm:`-shaped token was a comment-only legacy name in
`vllm/v1/metrics/reader.py`, not a registered family. HTTP instrumentation and default Python/process collector shapes are
proved by the observed exposition and matching upstream instrumentation behavior.

The current revision adds CPU read/write usage gauges and tiered lookup histograms relative to surfaces represented in the
running-source base. The fixture includes both revisions and labels mutually exclusive model modes/connectors as a structural
union. It does not claim that one deployment registers all families together.

## Post-authoring mapping reconciliation

Status: complete against the committed structural-union fixture and validation input summarized in `VALIDATION.md`.

The validator emitted 124 authored charts with 163 unique selectors. The structural union materialized 142 chart instances;
Mooncake operation/status and HTTP/parser identities account for the difference between authored definitions and instances.

| Displayed family | Charts | Selectors | Effective identity | Reconciliation |
|---|---:|---:|---|---|
| vLLM/Request Lifecycle | 14 | 17 | `{model_name, engine}` | outcomes, lifecycle, parameters, and corrupted-request roles agree |
| vLLM/Scheduler | 7 | 8 | `{model_name, engine}` | state, wait, preemption, sleep, and queue roles agree |
| vLLM/Prefill | 10 | 12 | `{model_name, engine}` | token sources, request size, KV compute, and phase time agree |
| vLLM/Decode | 11 | 13 | `{model_name, engine}` | generation, decode, ITL, and per-request populations remain distinct |
| vLLM/Engine Execution | 8 | 9 | `{model_name, engine}` | inference, step, and per-GPU estimate roles agree |
| vLLM/KV Cache | 4 | 7 | `{model_name, engine}` | local/external token caches and multimodal item cache remain separate |
| vLLM/KV Cache Residency | 5 | 9 | `{model_name, engine}` | lifetime, idle, and reuse-gap populations remain explicit |
| vLLM/KV Offloading | 18 | 30 | `{model_name, engine}` | canonical transfers, CPU capacity, admission, and lookup stages agree |
| vLLM/NIXL Connector | 10 | 15 | `{model_name, engine}` | transfer, descriptor, throughput, and failure roles agree |
| vLLM/HF3FS Connector | 5 | 8 | `{model_name, engine}` | save/load timing and failures remain distinct |
| vLLM/Mooncake Connector/Operation Timing | 3 | 4 | `{model_name, engine, operation, status}` | per-status timing populations are isolated |
| vLLM/Mooncake Connector/Operation Volume | 3 | 3 | `{model_name, engine, operation}` | `status` remains a bounded outcome dimension |
| vLLM/Speculative Decoding | 3 | 4 | `{model_name, engine}` | drafts, tokens, and bounded positions agree |
| vLLM/Diffusion Decoding | 3 | 3 | `{model_name, engine}` | diffusion-native work replaces speculative framing |
| vLLM/WebSocket Service | 4 | 5 | global | active/opened/closed and completed duration remain service-level |
| vLLM/HTTP Endpoints | 4 | 4 | `{handler, method}` | endpoint identities are preserved; status remains a dimension |
| vLLM/HTTP Service | 3 | 3 | global | high-resolution duration remains deliberately global |
| vLLM/Tool Parsing | 1 | 1 | `{model_name, request_type, mode}` | parser identity excludes absent `engine` |
| vLLM/Runtime | 5 | 5 | global | process resources remain service-level |
| vLLM/Runtime/Python GC | 3 | 3 | global | generation remains a bounded dimension |

Checks:

- Displayed families follow the causal-owner model rather than signal type or unit.
- Histogram bucket/count/sum roles remain with the recorded owner, population, and rendered unit.
- Core engine selectors use `{model_name, engine}`; endpoint, parser, Mooncake operation, and global surfaces use their separately
  proven identities.
- Priorities are positive, unique, and strictly increase from 1000 through 2220 in source order.
- Every chart explicitly uses `line` or `heatmap`; no discrete/state/time chart uses physical-volume presentation.
- Source-complete validation has zero autogen, unmatched, dead charts, dead dimensions, dimension losses, and collisions.

The partial observed runtime diagnostic now reports 52 dead optional charts and 78 dead optional dimensions. That strict
validator `FAIL` is expected evidence that one deployment does not expose the source-complete surface; the focused collector
regression proves the profile still collects the partial shape normally. `VALIDATION.md` records the separate verdicts, warning
dispositions, and evidence limits.

## Binding semantic ledger format

`src/go/testdata/prometheus/profiles/vllm/SOURCE-INVENTORY.tsv` is the exact source-family-to-authored-selector
reconciliation. Its 253 rows account for all 183
source families and all 163 authored selectors. Every chart row records owner, entity identity, signal role, observation
population, cross-family relationship, unit algebra, label roles and optionality, availability gate, evidence limitation,
destination, and source path. Exclusion rows preserve the same semantics and the lost question. Unresolved families and
authored selectors: **0**.
