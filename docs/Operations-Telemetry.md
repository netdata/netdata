# Telemetry

Monitor AI Agent with Prometheus metrics, OpenTelemetry traces, and OTLP log export.

---

## Table of Contents

- [Overview](#overview) - Telemetry components and architecture
- [Prometheus Metrics](#prometheus-metrics) - Scraping endpoint and available metrics
- [OpenTelemetry Traces](#opentelemetry-traces) - Distributed tracing
- [OTLP Log Export](#otlp-log-export) - Structured log forwarding
- [Configuration Reference](#configuration-reference) - All telemetry options
- [Grafana Dashboards](#grafana-dashboards) - Example queries and panels
- [Alerting Examples](#alerting-examples) - Prometheus alerting rules
- [Troubleshooting](#troubleshooting) - Common telemetry issues
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent provides three telemetry signals:

| Signal      | Protocol   | Purpose                      |
| ----------- | ---------- | ---------------------------- |
| **Metrics** | Prometheus | Counters, gauges, histograms |
| **Traces**  | OTLP       | Distributed request tracing  |
| **Logs**    | OTLP       | Structured log forwarding    |

### Architecture

```
AI Agent
    │
    ├─── Prometheus Exporter ──→ :9464/metrics ──→ Prometheus
    │
    ├─── OTLP Metric Exporter ──→ Collector:4317
    │
    ├─── OTLP Trace Exporter ──→ Collector:4317 ──→ Jaeger/Tempo
    │
    └─── OTLP Log Exporter ──→ Collector:4317 ──→ Loki
```

### Disable Telemetry

Set environment variable to disable all telemetry:

```bash
AI_TELEMETRY_DISABLE=true ai-agent --agent test.ai "query"
```

---

## Prometheus Metrics

### Enable Prometheus Endpoint

```bash
ai-agent --agent test.ai \
  --telemetry-enabled \
  --telemetry-prometheus-enabled \
  --telemetry-prometheus-host 0.0.0.0 \
  --telemetry-prometheus-port 9464
```

Access metrics:

```bash
curl http://localhost:9464/metrics
```

### LLM Metrics

| Metric                                  | Type      | Description                          |
| --------------------------------------- | --------- | ------------------------------------ |
| `ai_agent_llm_requests_total`           | Counter   | Total LLM requests                   |
| `ai_agent_llm_latency_ms`               | Histogram | LLM request latency                  |
| `ai_agent_llm_prompt_tokens_total`      | Counter   | Total input tokens                   |
| `ai_agent_llm_completion_tokens_total`  | Counter   | Total output tokens                  |
| `ai_agent_llm_cache_read_tokens_total`  | Counter   | Cached input tokens (read)           |
| `ai_agent_llm_cache_write_tokens_total` | Counter   | Cached input tokens (write)          |
| `ai_agent_llm_bytes_in_total`           | Counter   | Request bytes                        |
| `ai_agent_llm_bytes_out_total`          | Counter   | Response bytes                       |
| `ai_agent_llm_errors_total`             | Counter   | LLM errors (with `error_type` label) |
| `ai_agent_llm_retries_total`            | Counter   | Retry attempts                       |
| `ai_agent_llm_cost_usd_total`           | Counter   | Total cost in USD                    |

**Labels**: `agent`, `call_path`, `provider`, `model`, `headend`, `status`, `reasoning_level`

### Tool Metrics

| Metric                            | Type      | Description                           |
| --------------------------------- | --------- | ------------------------------------- |
| `ai_agent_tool_invocations_total` | Counter   | Total tool calls                      |
| `ai_agent_tool_latency_ms`        | Histogram | Tool execution latency                |
| `ai_agent_tool_bytes_in_total`    | Counter   | Tool request bytes                    |
| `ai_agent_tool_bytes_out_total`   | Counter   | Tool response bytes                   |
| `ai_agent_tool_errors_total`      | Counter   | Tool errors (with `error_type` label) |

**Labels**: `agent`, `call_path`, `tool_name`, `tool_kind`, `provider`, `headend`, `status`

### Queue Metrics

| Metric                            | Type      | Description                                                                      |
| --------------------------------- | --------- | -------------------------------------------------------------------------------- |
| `ai_agent_queue_depth`            | Gauge     | Number of queued tool executions awaiting a slot                                 |
| `ai_agent_queue_in_use`           | Gauge     | Number of active tool executions consuming queue capacity                        |
| `ai_agent_queue_last_wait_ms`     | Gauge     | Most recent observed wait duration per queue (milliseconds)                      |
| `ai_agent_queue_wait_duration_ms` | Histogram | Latency between enqueue and start time for queued tool executions (milliseconds) |

**Labels**: `queue`, `capacity`

### Context Guard Metrics

| Metric                                    | Type    | Description                           |
| ----------------------------------------- | ------- | ------------------------------------- |
| `ai_agent_context_guard_events_total`     | Counter | Context guard activations             |
| `ai_agent_context_guard_remaining_tokens` | Gauge   | Tokens remaining when guard triggered |

**Labels**: `agent`, `provider`, `model`, `trigger`, `outcome`

### Final Report Metrics

| Metric                                 | Type      | Description                                      |
| -------------------------------------- | --------- | ------------------------------------------------ |
| `ai_agent_final_report_total`          | Counter   | Final reports emitted grouped by source          |
| `ai_agent_final_report_attempts_total` | Counter   | Final-report attempts observed before acceptance |
| `ai_agent_final_report_turns`          | Histogram | Turn index when final report was accepted        |

**Labels**: `agent`, `call_path`, `headend`, `source`, `forced_final_reason`, `synthetic_reason`

### Retry Metrics

| Metric                          | Type    | Description              |
| ------------------------------- | ------- | ------------------------ |
| `ai_agent_retry_collapse_total` | Counter | maxTurns collapse events |

---

## OpenTelemetry Traces

### Enable OTLP Tracing

```bash
ai-agent --agent test.ai \
  --telemetry-otlp-endpoint http://collector:4317 \
  "query"
```

### Trace Structure

Each session creates a trace with nested spans:

```
Session (root span)
├── Turn 1
│   ├── LLM Request
│   │   ├── Provider Call
│   │   └── Response Parse
│   └── Tool Calls
│       ├── Tool A
│       └── Tool B
├── Turn 2
│   └── ...
└── Final Report
```

### Trace Attributes

| Attribute                   | Description            |
| --------------------------- | ---------------------- |
| `ai.session.run_id`         | Session run ID         |
| `ai.session.source`         | Session source         |
| `ai.session.headend_id`     | Headend identifier     |
| `ai.session.thread_id`      | Thread/conversation ID |
| `ai.llm.provider`           | LLM provider           |
| `ai.llm.model`              | Model name             |
| `ai.llm.is_final_turn`      | Whether turn is final  |
| `ai.llm.status`             | LLM request status     |
| `ai.llm.latency_ms`         | Request latency (ms)   |
| `ai.llm.prompt_tokens`      | Input token count      |
| `ai.llm.completion_tokens`  | Output token count     |
| `ai.llm.cache_read_tokens`  | Cached input tokens    |
| `ai.llm.cache_write_tokens` | Cached output tokens   |
| `ai.llm.reasoning.level`    | Reasoning mode         |
| `ai.context.trigger`        | Context guard trigger  |
| `ai.context.outcome`        | Context guard outcome  |

### Sampling

Configure trace sampling:

```bash
# Sample all traces
ai-agent --telemetry-trace-sampler always_on "query"

# Sample 10% of traces
ai-agent --telemetry-trace-sampler ratio:0.1 "query"
# Or with separate ratio option:
ai-agent --telemetry-trace-sampler ratio --telemetry-trace-ratio 0.1 "query"

# Never sample (metrics only)
ai-agent --telemetry-trace-sampler always_off "query"

# Follow parent span decision (default)
ai-agent --telemetry-trace-sampler parent "query"
```

| Sampler      | Description                                         |
| ------------ | --------------------------------------------------- |
| `always_on`  | Sample all traces                                   |
| `always_off` | Sample no traces                                    |
| `ratio`      | Sample proportion defined by ratio option (0.0-1.0) |
| `parent`     | Use parent span's decision (default)                |

---

## OTLP Log Export

### Enable Log Export

```bash
ai-agent --agent test.ai \
  --telemetry-logging-otlp-endpoint http://collector:4317 \
  "query"
```

### Severity Mapping

| AI Agent Level | OTLP Severity |
| -------------- | ------------- |
| `ERR`          | ERROR         |
| `WRN`          | WARN          |
| `FIN`          | INFO          |
| `TRC`          | TRACE         |
| `VRB`          | DEBUG         |
| `THK`          | DEBUG         |

### Log Attributes

Exported logs include:

| Attribute   | Description         |
| ----------- | ------------------- |
| `type`      | Log type (llm/tool) |
| `direction` | request/response    |
| `turn`      | Turn number         |
| `subturn`   | Tool call index     |
| `agent`     | Agent ID            |
| `call_path` | Call hierarchy      |
| `remote`    | Remote identifier   |
| `provider`  | Provider name       |
| `model`     | Model name          |

---

## Configuration Reference

### CLI Flags

| Flag                                  | Description                                                   | Default                                    |
| ------------------------------------- | ------------------------------------------------------------- | ------------------------------------------ |
| `--telemetry-prometheus-enabled`      | Enable Prometheus /metrics endpoint                           | `false`                                    |
| `--telemetry-prometheus-host`         | Prometheus bind host                                          | `127.0.0.1` (used when host not specified) |
| `--telemetry-prometheus-port`         | Prometheus bind port                                          | `9464` (used when port not specified)      |
| `--telemetry-otlp-endpoint`           | OTLP collector endpoint                                       | None                                       |
| `--telemetry-otlp-timeout-ms`         | OTLP timeout (ms or duration like 5s/2m)                      | `2000` (used when timeout not specified)   |
| `--telemetry-traces-enabled`          | Enable OTLP tracing                                           | `false`                                    |
| `--telemetry-trace-sampler`           | Trace sampler                                                 | `parent`                                   |
| `--telemetry-trace-ratio`             | Sampling ratio (0-1) when sampler=ratio                       | `0.1`                                      |
| `--telemetry-label`                   | Additional labels (key=value, can specify multiple times)     | None                                       |
| `--telemetry-log-format`              | Preferred logging format(s): journald, logfmt, json, none     | None                                       |
| `--telemetry-log-extra`               | Additional log sinks (e.g., otlp, can specify multiple times) | None                                       |
| `--telemetry-logging-otlp-endpoint`   | Override OTLP endpoint for log exports                        | None                                       |
| `--telemetry-logging-otlp-timeout-ms` | Override OTLP timeout for log exports                         | None                                       |

### Config File

```json
{
  "telemetry": {
    "enabled": true,
    "otlp": {
      "endpoint": "http://collector:4317",
      "timeoutMs": 2000
    },
    "prometheus": {
      "enabled": true,
      "host": "0.0.0.0",
      "port": 9464
    },
    "traces": {
      "enabled": true,
      "sampler": "ratio",
      "ratio": 0.1
    },
    "logging": {
      "formats": ["journald", "logfmt"],
      "extra": ["otlp"],
      "otlp": {
        "endpoint": "http://collector:4317"
      }
    },
    "labels": {
      "environment": "production",
      "service": "my-agent"
    }
  }
}
```

### Environment Variables

| Variable               | Description                                   |
| ---------------------- | --------------------------------------------- |
| `AI_TELEMETRY_DISABLE` | Set to `1`, `true`, `yes`, or `on` to disable |

---

## Grafana Dashboards

### Request Rate

```promql
rate(ai_agent_llm_requests_total[5m])
```

### Error Rate

```promql
rate(ai_agent_llm_errors_total[5m])
```

### Success Rate

```promql
sum(rate(ai_agent_llm_requests_total{status="success"}[5m]))
/
sum(rate(ai_agent_llm_requests_total[5m]))
```

### Token Usage (Input vs Output)

```promql
rate(ai_agent_llm_prompt_tokens_total[5m])
rate(ai_agent_llm_completion_tokens_total[5m])
```

### Cache Hit Rate

```promql
rate(ai_agent_llm_cache_read_tokens_total[5m])
/
(rate(ai_agent_llm_prompt_tokens_total[5m]) + rate(ai_agent_llm_cache_read_tokens_total[5m]))
```

### Cost per Hour

```promql
rate(ai_agent_llm_cost_usd_total[1h]) * 3600
```

### Cost by Model

```promql
sum by (model) (rate(ai_agent_llm_cost_usd_total[1h])) * 3600
```

### P99 Latency

```promql
histogram_quantile(0.99, rate(ai_agent_llm_latency_ms_bucket[5m]))
```

### Queue Depth

```promql
ai_agent_queue_depth{queue="default"}
```

### Queue Wait Time (P95)

```promql
histogram_quantile(0.95, rate(ai_agent_queue_wait_duration_ms_bucket[5m]))
```

### Context Guard Events

```promql
rate(ai_agent_context_guard_events_total[5m])
```

### Tool Latency by Tool

```promql
histogram_quantile(0.95, sum by (tool_name, le) (rate(ai_agent_tool_latency_ms_bucket[5m])))
```

---

## Alerting Examples

### High Error Rate

```yaml
groups:
  - name: ai-agent
    rules:
      - alert: HighAgentErrorRate
        expr: rate(ai_agent_llm_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High AI Agent error rate
          description: "Error rate is {{ $value | humanize }} per second"
```

### Queue Depth Alert

```yaml
- alert: HighQueueDepth
  expr: ai_agent_queue_depth{queue="default"} > 10
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: AI Agent queue depth high
    description: "Queue depth: {{ $value }}"
```

### High Latency

```yaml
- alert: HighLLMLatency
  expr: histogram_quantile(0.95, rate(ai_agent_llm_latency_ms_bucket[5m])) > 30000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: LLM P95 latency above 30 seconds
```

### Budget Alert

```yaml
- alert: HighHourlyCost
  expr: rate(ai_agent_llm_cost_usd_total[1h]) * 3600 > 10
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: AI Agent cost exceeding $10/hour
```

### Context Guard Frequent

```yaml
- alert: FrequentContextGuard
  expr: rate(ai_agent_context_guard_events_total[5m]) > 0.5
  for: 5m
  labels:
    severity: info
  annotations:
    summary: Context guard triggering frequently
```

---

## Troubleshooting

### Problem: No metrics on /metrics endpoint

**Cause**: Prometheus exporter not enabled or wrong port.

**Solution**:

1. Verify flags: `--telemetry-prometheus-port 9464`
2. Check binding: `--telemetry-prometheus-host 0.0.0.0`
3. Test: `curl http://localhost:9464/metrics`

---

### Problem: OTLP export failing

**Cause**: Collector unreachable or wrong protocol.

**Solution**:

1. Verify collector is running: `curl http://collector:4317`
2. Check endpoint protocol (gRPC vs HTTP)
3. Check timeout: `--telemetry-otlp-timeout 5000`
4. Look for errors in stderr

---

### Problem: Missing metrics labels

**Cause**: Labels not configured or not propagated.

**Solution**: Add custom labels in config:

```json
{
  "telemetry": {
    "labels": {
      "environment": "production",
      "team": "platform"
    }
  }
}
```

---

### Problem: High cardinality warning

**Cause**: Too many unique label combinations (many agents/models).

**Solution**:

1. Reduce label cardinality
2. Use recording rules to aggregate
3. Drop high-cardinality labels at collector

---

### Problem: Traces not appearing in Jaeger

**Cause**: Sampling or export issue.

**Solution**:

1. Verify sampler: `--telemetry-trace-sampler always_on`
2. Check OTLP endpoint
3. Verify collector routing to trace backend

---

## See Also

- [Operations](Operations) - Operations overview
- [Accounting](Operations-Accounting) - Cost tracking
- [Logging](Operations-Logging) - Logging system
- [specs/telemetry-overview.md](specs/telemetry-overview.md) - Technical specification
