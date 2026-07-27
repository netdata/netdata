# Application evidence guide: vLLM

Use this file to understand domain concepts, then verify every metric name,
type, label, and optional surface against the supplied dump and the matching
vLLM version. vLLM metrics evolve; these notes are not a metric-schema contract.

## Serving pipeline

A useful causal model is:

1. request arrival and scheduling/queueing;
2. prompt processing (prefill and KV-cache work);
3. autoregressive generation (decode);
4. completion outcome and end-to-end latency.

Common metric concepts observed across vLLM releases include:

- workload/outcome: request success/outcome counters, prompt tokens, generation
  tokens;
- queue/scheduler: running requests, waiting requests, waiting reasons,
  preemptions, sleep state;
- prefill: cached prompt tokens, prefill time, prompt/KV-computed tokens;
- decode: iteration tokens, inter-token latency, time per output token, decode
  time;
- end-to-end SLIs: time to first token and end-to-end request latency;
- resources: KV-cache utilization/configuration, process CPU/memory, Python/GC
  runtime surfaces.

Map actual families to these concepts from `HELP`, vLLM documentation/source,
and observed labels. Do not invent a pipeline chart merely because a concept is
listed here.

### Causal placement, not a generic latency bucket

vLLM exposes several timings because they locate different parts of the serving
path. Keep their causal role visible:

- scheduler waiting and queue time explain admission pressure;
- prompt tokens, cache work, TTFT, and prefill time explain prompt processing;
- generation/iteration tokens, ITL, TPOT, decode time, and speculative decode
  explain autoregressive generation;
- running-phase inference and E2E latency summarize broader request-lifecycle
  questions.

A compact service-level SLI overview can repeat selected end-to-end signals when
it answers the first operator question. A single “Latency” family containing
every phase is not that overview: it groups by measurement type and hides where
the delay occurs.

Do not keep `Latency` as a common parent and then put Queue, Prefill, and Decode
below it. Make the causal stages the navigation parents; latency is one aspect
inside each stage. Likewise, a global service entity should still nest HTTP,
process, and Python-runtime subfamilies instead of flattening their charts
together.

## Naming and identity cautions

- vLLM metric names commonly contain `:` (for example `vllm:...`). Quote such
  selectors in YAML where needed.
- `model_name` can be a useful model entity label when it is present and stable.
  Do not assume one model per endpoint: inspect all observed combinations and
  the deployment architecture.
- Some releases also expose an `engine` label. A dump with one observed engine
  cannot prove that `model_name` alone remains unique in a multi-engine
  deployment. Decide whether engine belongs in instance identity from the
  deployment contract and expected cardinality; too few labels merge engines,
  while unnecessary labels fragment one model.
- HTTP/process/Python runtime families may not carry `model_name`; that is an
  observed label-contract question, not a universal rule. Place global runtime
  charts at service level rather than forcing a missing model identity.
- `_info` families such as cache configuration are skipped by the current
  Prometheus writer. Their labels cannot be promoted by a profile.

## Distribution cautions

Some HTTP instrumentation exposes summary `_count`/`_sum` without quantiles.
The current writer rejects such a summary completely, so charts for those names
will be dead. Validate the exact dump before designing request/response-size or
latency summary charts.

Duration `_sum` rate is not automatically “average latency.” It may express
accumulated seconds per second and can be interpreted as concurrency only when
the underlying observation semantics justify that inference.

The following similarly named distributions represent distinct concepts in
known vLLM surfaces and MUST NOT be deduplicated from their names alone:

- `vllm:request_max_num_generation_tokens` describes the maximum requested
  generation-token quantity;
- `vllm:request_params_max_tokens` describes the request's `max_tokens`
  parameter;
- `vllm:request_params_n` describes the request's `n` parameter.

Verify the versioned HELP/source and observed bucket schemas. A representative
dump showed the first two with different HELP semantics and different observed
distributions despite equal observation counts.

Likewise, `vllm:request_inference_time_seconds` is the direct RUNNING-phase
request distribution. Separate prefill and decode distributions do not
reconstruct it after request-level correlation is lost. Keep the direct
surface when the operator needs the total running-phase question.

`vllm:spec_decode_num_accepted_tokens_per_pos_total` uses `position` for draft
position. Draft length is configuration-dependent; positions observed in one
dump are not a closed enum. Use dynamic dimension naming so a profile does not
silently omit additional draft positions.
