# Telemetry System

## TL;DR
OpenTelemetry integration providing metrics, traces, and log export with OTLP, Prometheus, and configurable samplers for observability.

## Source Files
- `src/telemetry/index.ts` - Full implementation (951 lines)
- `src/telemetry/runtime-config.ts` - Runtime configuration utilities
- `@opentelemetry/*` - OpenTelemetry SDK dependencies

## System Architecture

### Components
1. **Metrics Recorder**: LLM and tool metrics collection
2. **Trace Provider**: Distributed tracing with spans
3. **Log Exporter**: Structured log forwarding to OTLP
4. **Prometheus Exporter**: Metrics scraping endpoint

### Recorder Pattern
```typescript
interface TelemetryRecorder {
  recordLlmMetrics: (record: LlmMetricsRecord) => void;
  recordToolMetrics: (record: ToolMetricsRecord) => void;
  recordContextGuardMetrics: (record: ContextGuardMetricsRecord) => void;
  recordQueueDepth: (record: QueueDepthRecord) => void;
  recordQueueWait: (record: QueueWaitRecord) => void;
  shutdown: () => Promise<void>;
}
```

NoopRecorder used when telemetry disabled.

## Configuration

### TelemetryRuntimeConfig
```typescript
interface TelemetryRuntimeConfig {
  enabled: boolean;
  mode: 'cli' | 'server';
  otlpEndpoint?: string;
  otlpTimeoutMs?: number;
  prometheus?: {
    enabled?: boolean;
    host?: string;         // Default: '127.0.0.1'
    port?: number;         // Default: 9464
  };
  labels?: Record<string, string>;
  traces?: {
    enabled: boolean;
    sampler: 'always_on' | 'always_off' | 'parent' | 'ratio';
    ratio?: number;
  };
  logging?: {
    formats?: TelemetryLogFormat[];
    extra?: TelemetryLogExtra[];  // 'otlp' enables log export
    otlpEndpoint?: string;
    otlpTimeoutMs?: number;
  };
}
```

## Initialization

**Location**: `src/telemetry/index.ts:165-258`

```typescript
async function initTelemetry(config: TelemetryRuntimeConfig): Promise<void> {
  globalLabels = { ...config.labels, mode: config.mode };

  // Check AI_TELEMETRY_DISABLE environment variable
  const suppressed = telemetryDisabledByEnv();
  const enableMetrics = config.enabled && !suppressed;
  const enablePrometheus = enableMetrics && config.prometheus?.enabled;
  const enableTraces = enableMetrics && config.traces?.enabled;
  const enableLogExporter = !suppressed && config.logging?.extra?.includes('otlp');

  // Load OpenTelemetry dependencies dynamically
  const otel = await loadOtelDependencies(enablePrometheus);

  // Create resource with service metadata
  const resource = otel.resourceFromAttributes({
    'service.name': 'ai-agent',
    'service.version': PACKAGE_VERSION,
    ...labelAttributes,
  });

  // Setup OTLP metric exporter
  const otlpExporter = new otel.OTLPMetricExporter({
    url: config.otlpEndpoint,
    timeoutMillis: config.otlpTimeoutMs,
    temporalityPreference: AggregationTemporality.DELTA,
  });

  // Create periodic metric reader (5 second intervals)
  const metricReader = new otel.PeriodicExportingMetricReader({
    exporter: otlpExporter,
    exportIntervalMillis: 5000,
    exportTimeoutMillis: config.otlpTimeoutMs ?? 2000,
  });

  // Optional Prometheus exporter
  if (enablePrometheus) {
    prometheusExporter = new otel.PrometheusExporter({ ... });
    await prometheusExporter.startServer();
  }

  // Create meter provider
  const meterProvider = new otel.MeterProvider({ resource, readers });
  recorder = new OtelMetricsRecorder({ meterProvider, ... });

  // Setup tracing if enabled
  if (enableTraces) setupTracing(otel, resource, config);

  // Setup log exporter if enabled
  if (enableLogExporter) await setupLogExporter(otel, resource, config);
}
```

## LLM Metrics

### Record Schema
```typescript
interface LlmMetricsRecord {
  agentId?: string;
  callPath?: string;
  headendId?: string;
  provider: string;
  model: string;
  status: 'success' | 'error';
  errorType?: string;
  latencyMs: number;
  promptTokens: number;
  completionTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  requestBytes?: number;
  responseBytes?: number;
  retries?: number;
  costUsd?: number;
  reasoningLevel?: string;
  customLabels?: Record<string, string>;
}
```

### Instruments Created
- `ai_agent_llm_latency_ms`: Histogram
- `ai_agent_llm_requests_total`: Counter
- `ai_agent_llm_prompt_tokens_total`: Counter
- `ai_agent_llm_completion_tokens_total`: Counter
- `ai_agent_llm_cache_read_tokens_total`: Counter
- `ai_agent_llm_cache_write_tokens_total`: Counter
- `ai_agent_llm_bytes_in_total`: Counter
- `ai_agent_llm_bytes_out_total`: Counter
- `ai_agent_llm_errors_total`: Counter (with error_type label)
- `ai_agent_llm_retries_total`: Counter
- `ai_agent_llm_cost_usd_total`: Counter

### Labels Applied
- `agent`: agentId or 'unknown'
- `call_path`: callPath or 'unknown'
- `provider`: provider name
- `model`: model name
- `headend`: headendId or 'cli'
- `status`: 'success' or 'error'
- `reasoning_level`: if present
- Global labels from config

## Tool Metrics

### Record Schema
```typescript
interface ToolMetricsRecord {
  agentId?: string;
  callPath?: string;
  headendId?: string;
  toolName: string;
  toolKind?: string;
  provider: string;
  status: 'success' | 'error';
  errorType?: string;
  latencyMs: number;
  inputBytes?: number;
  outputBytes?: number;
  customLabels?: Record<string, string>;
}
```

### Instruments Created
- `ai_agent_tool_latency_ms`: Histogram
- `ai_agent_tool_invocations_total`: Counter
- `ai_agent_tool_bytes_in_total`: Counter
- `ai_agent_tool_bytes_out_total`: Counter
- `ai_agent_tool_errors_total`: Counter (with error_type label)

## Context Guard Metrics

### Record Schema
```typescript
interface ContextGuardMetricsRecord {
  agentId?: string;
  callPath?: string;
  headendId?: string;
  provider: string;
  model: string;
  trigger: 'tool_preflight' | 'turn_preflight';
  outcome: 'skipped_provider' | 'forced_final';
  limitTokens?: number;
  projectedTokens: number;
  remainingTokens?: number;
  customLabels?: Record<string, string>;
}
```

### Instruments Created
- `ai_agent_context_guard_events_total`: Counter
- `ai_agent_context_guard_remaining_tokens`: Observable Gauge

## Queue Metrics

### Depth Record
```typescript
interface QueueDepthRecord {
  queue: string;
  capacity: number;
  inUse: number;
  waiting: number;
}
```

### Wait Record
```typescript
interface QueueWaitRecord {
  queue: string;
  waitMs: number;
}
```

### Instruments Created
- `ai_agent_queue_depth`: Observable Gauge
- `ai_agent_queue_in_use`: Observable Gauge
- `ai_agent_queue_last_wait_ms`: Observable Gauge
- `ai_agent_queue_wait_duration_ms`: Histogram

## Tracing

### Setup
**Location**: `src/telemetry/index.ts:783-812`

```typescript
function setupTracing(otel, resource, config): void {
  const exporter = new otel.OTLPTraceExporter({
    url: config.otlpEndpoint,
    timeoutMillis: config.otlpTimeoutMs,
  });

  const spanProcessor = new otel.BatchSpanProcessor(exporter);
  const sampler = createSampler(otel, config.traces);
  const provider = new otel.NodeTracerProvider({
    resource,
    sampler,
    spanProcessors: [spanProcessor],
  });

  provider.register();
}
```

### Samplers
- `always_on`: Sample all traces
- `always_off`: Sample no traces
- `parent`: Follow parent span decision
- `ratio`: Sample based on trace ID ratio (0.1 default)

### Span API
```typescript
function runWithSpan<T>(name: string, fn: (span: Span) => T): Promise<T>;
function runWithSpan<T>(name: string, options: RunWithSpanOptions, fn: (span: Span) => T): Promise<T>;

interface RunWithSpanOptions {
  attributes?: Attributes;
  kind?: SpanKind;
  links?: Link[];
  startTime?: number;
}
```

Automatic error recording and status setting.

### Helper Functions
- `addSpanAttributes(attributes)`: Add attributes to active span
- `recordSpanError(error)`: Record exception on active span
- `addSpanEvent(name, attributes?)`: Add event to active span

## Log Export

### Setup
**Location**: `src/telemetry/index.ts:313-360`

```typescript
async function setupLogExporter(otel, resource, config): Promise<void> {
  const exporter = new otel.OTLPLogExporter({
    url: loggingEndpoint,
    timeoutMillis: loggingTimeout,
  });

  const processor = new otel.BatchLogRecordProcessor(exporter);
  const provider = new otel.LoggerProvider({ resource, processors: [processor] });
  const logger = provider.getLogger('ai-agent');

  logEmitter = (event) => {
    logger.emit({
      body: event.message,
      severityText: event.severity,
      severityNumber: mapSeverityNumber(event.severity),
      timestamp: millisToHrTime(event.timestamp),
      attributes: buildLogAttributes(event),
    });
  };
}
```

### Severity Mapping
- `ERR` → ERROR
- `WRN` → WARN
- `FIN` → INFO
- `TRC` → TRACE
- `VRB`, `THK` → DEBUG

### Log Attributes
- Standard fields: type, direction, turn, subturn, priority, severity, ts
- Optional fields: message_id, tool_kind, tool_namespace, tool, headend, agent, call_path, txn_id, parent_txn_id, origin_txn_id, remote, provider, model
- Label fields: `label.{key}` for each label

## Business Logic Coverage (Verified 2025-11-16)

- **Env override**: `AI_TELEMETRY_DISABLE` short-circuits initialization, returning a Noop recorder even if telemetry is enabled in config (`src/telemetry/index.ts:165-210`).
- **Lazy dependency loading**: OpenTelemetry packages are imported dynamically only when telemetry is active, avoiding CLI startup penalties when telemetry is disabled (`src/telemetry/index.ts:210-260`).
- **Optional log exporter**: Structured log export occurs only when `logging.extra` contains `'otlp'`; otherwise logs remain local even if metrics/traces are enabled (`src/telemetry/index.ts:260-330`).
- **Prometheus server**: When configured the exporter starts its own HTTP server (`host`/`port`), enabling Kubernetes scraping without conflicting with user headends (`src/telemetry/index.ts:300-360`).

## Environment Variables

| Variable | Effect |
|----------|--------|
| `AI_TELEMETRY_DISABLE` | Set to '1', 'true', 'yes', or 'on' to disable |

## CLI Mode Behavior

For CLI mode:
- Exporter errors logged (batch dropped)
- BatchLogRecordProcessor: 200ms delay, configurable timeout
- BatchSpanProcessor: 200ms delay, configurable timeout

## Shutdown

**Location**: `src/telemetry/index.ts:260-265`

```typescript
async function shutdownTelemetry(): Promise<void> {
  await recorder.shutdown();
  recorder = new NoopRecorder();
  await shutdownTracing();
  await shutdownLogExporter();
}
```

Graceful shutdown of all providers.

## Public API

### Exported Functions
- `getTelemetryLabels()`: Get current global labels
- `getTelemetryLoggingConfig()`: Get current logging config
- `initTelemetry(config)`: Initialize telemetry system
- `shutdownTelemetry()`: Shutdown all providers
- `recordLlmMetrics(record)`: Record LLM metrics
- `recordToolMetrics(record)`: Record tool metrics
- `recordQueueDepthMetrics(record)`: Record queue depth
- `recordQueueWaitMetrics(record)`: Record queue wait time
- `recordContextGuardMetrics(record)`: Record context guard event
- `emitTelemetryLog(event)`: Emit structured log
- `runWithSpan(name, fn)`: Execute function in traced span
- `addSpanAttributes(attributes)`: Add span attributes
- `recordSpanError(error)`: Record span error
- `addSpanEvent(name, attributes?)`: Add span event

## Invariants

1. **Noop fallback**: NoopRecorder when disabled
2. **Dynamic loading**: OpenTelemetry deps loaded at runtime
3. **Resource tagging**: All data tagged with service info
4. **Label merging**: Global + custom labels per record
5. **Graceful degradation**: Errors logged but don't crash
6. **Mode-aware**: CLI vs server behavior differences

## Test Coverage

**Phase 1**:
- Metric recording
- Label building
- Sampler creation
- Log severity mapping
- Environment variable parsing

**Gaps**:
- OTLP endpoint connectivity
- Prometheus scraping
- Trace propagation
- High cardinality label scenarios
- Shutdown race conditions
