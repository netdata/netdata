# Telemetry & Metrics

Monitor AI Agent with metrics, traces, and logs.

---

## Overview

AI Agent exports:
- **Metrics**: Prometheus counters and gauges
- **Traces**: OpenTelemetry spans
- **Logs**: Structured log export via OTLP

---

## Prometheus Metrics

### Enable Endpoint

```bash
ai-agent --agent test.ai \
  --telemetry-prometheus-host 0.0.0.0 \
  --telemetry-prometheus-port 9090 \
  --api 8080
```

Access metrics:
```bash
curl http://localhost:9090/metrics
```

### Available Metrics

#### Queue Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ai_agent_queue_depth` | Gauge | Current in-use + waiting slots |
| `ai_agent_queue_wait_duration_ms` | Histogram | Time waiting for slot |

#### Context Guard Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ai_agent_context_guard_events_total` | Counter | Guard activations |
| `ai_agent_context_guard_remaining_tokens` | Gauge | Remaining budget at activation |

#### Final Report Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ai_agent_final_report_attempts_total` | Counter | Final report attempts |
| `ai_agent_final_report_success_total` | Counter | Successful final reports |

#### Retry Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ai_agent_retry_collapse_total` | Counter | maxTurns collapse events |
| `ai_agent_retry_exhausted_total` | Counter | Retry exhaustion events |

#### LLM Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ai_agent_llm_input_tokens_total` | Counter | Total input tokens |
| `ai_agent_llm_output_tokens_total` | Counter | Total output tokens |
| `ai_agent_llm_cache_read_tokens_total` | Counter | Cache read tokens |
| `ai_agent_llm_cache_write_tokens_total` | Counter | Cache write tokens |
| `ai_agent_llm_cost_total` | Counter | Total cost (USD) |
| `ai_agent_llm_latency_ms` | Histogram | LLM call latency |

---

## OpenTelemetry Traces

### Enable OTLP Export

```bash
ai-agent --agent test.ai \
  --telemetry-otlp-endpoint http://collector:4317 \
  "query"
```

### Trace Structure

```
Session
├── Turn 1
│   ├── LLM Request
│   └── Tool Calls
│       ├── Tool A
│       └── Tool B
├── Turn 2
│   └── ...
└── Final Report
```

### Trace Attributes

| Attribute | Description |
|-----------|-------------|
| `ai_agent.session_id` | Session identifier |
| `ai_agent.agent_path` | Agent file path |
| `ai_agent.provider` | LLM provider |
| `ai_agent.model` | Model name |
| `ai_agent.turn` | Turn number |

---

## Log Export (OTLP)

```bash
ai-agent --agent test.ai \
  --telemetry-logging-otlp-endpoint http://collector:4317 \
  "query"
```

---

## Sampling

Configure trace sampling:

```bash
ai-agent --agent test.ai \
  --telemetry-trace-sampler ratio:0.1 \
  "query"
```

Options:
- `always` - Sample all traces
- `never` - Sample no traces
- `ratio:0.1` - Sample 10% of traces

---

## Grafana Dashboard

Example queries:

### Request Rate

```promql
rate(ai_agent_llm_input_tokens_total[5m])
```

### Error Rate

```promql
rate(ai_agent_retry_exhausted_total[5m])
```

### Cost per Hour

```promql
rate(ai_agent_llm_cost_total[1h]) * 3600
```

### Queue Depth

```promql
ai_agent_queue_depth{queue="default"}
```

### Context Guard Events

```promql
rate(ai_agent_context_guard_events_total[5m])
```

---

## Alerting Examples

### High Error Rate

```yaml
alert: HighAgentErrorRate
expr: rate(ai_agent_retry_exhausted_total[5m]) > 0.1
for: 5m
labels:
  severity: warning
annotations:
  summary: High agent error rate
```

### Queue Saturation

```yaml
alert: QueueSaturation
expr: ai_agent_queue_depth > 0.8 * ai_agent_queue_capacity
for: 2m
labels:
  severity: warning
```

---

## See Also

- [docs/specs/telemetry-overview.md](../docs/specs/telemetry-overview.md) - Technical spec
- [Accounting](Operations-Accounting) - Cost tracking
