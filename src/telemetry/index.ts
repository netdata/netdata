import { createRequire } from 'node:module';

import { trace as otelTrace, SpanStatusCode } from '@opentelemetry/api';
import { logs as otelLogs } from '@opentelemetry/api-logs';

import type { StructuredLogEvent } from '../logging/structured-log-event.js';
import type { TelemetryLogExtra, TelemetryLogFormat, TelemetryTraceSampler } from '../types.js';
import type { Attributes, Span, SpanKind } from '@opentelemetry/api';
import type * as OtelApi from '@opentelemetry/api';
import type * as OtelLogsApi from '@opentelemetry/api-logs';
import type * as OtelCore from '@opentelemetry/core';
import type * as OtelLogsOtlp from '@opentelemetry/exporter-logs-otlp-grpc';
import type * as OtelOtlp from '@opentelemetry/exporter-metrics-otlp-grpc';
import type * as OtelProm from '@opentelemetry/exporter-prometheus';
import type * as OtelTraceOtlp from '@opentelemetry/exporter-trace-otlp-grpc';
import type * as OtelResources from '@opentelemetry/resources';
import type * as OtelLogs from '@opentelemetry/sdk-logs';
import type * as OtelMetrics from '@opentelemetry/sdk-metrics';
import type * as OtelTraceBase from '@opentelemetry/sdk-trace-base';
import type * as OtelTrace from '@opentelemetry/sdk-trace-node';
import type * as OtelSemantic from '@opentelemetry/semantic-conventions';

export type TelemetryMode = 'cli' | 'server';

export interface TelemetryRuntimeTracesConfig {
  enabled: boolean;
  sampler: TelemetryTraceSampler;
  ratio?: number;
}

export interface TelemetryRuntimeLoggingConfig {
  formats?: TelemetryLogFormat[];
  extra?: TelemetryLogExtra[];
  otlpEndpoint?: string;
  otlpTimeoutMs?: number;
}

export interface TelemetryRuntimeConfig {
  enabled: boolean;
  mode: TelemetryMode;
  otlpEndpoint?: string;
  otlpTimeoutMs?: number;
  prometheus?: {
    enabled?: boolean;
    host?: string;
    port?: number;
  };
  labels?: Record<string, string>;
  traces?: TelemetryRuntimeTracesConfig;
  logging?: TelemetryRuntimeLoggingConfig;
}

export interface LlmMetricsRecord {
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

export interface ToolMetricsRecord {
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

export interface QueueDepthRecord {
  queue: string;
  capacity: number;
  inUse: number;
  waiting: number;
}

export interface QueueWaitRecord {
  queue: string;
  waitMs: number;
}

export interface ContextGuardMetricsRecord {
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

export interface FinalReportMetricsRecord {
  agentId?: string;
  callPath?: string;
  headendId?: string;
  source: 'tool-call' | 'tool-message' | 'synthetic';
  turnsCompleted: number;
  finalReportAttempts: number;
  forcedFinalReason?: 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion';
  syntheticReason?: string;
  customLabels?: Record<string, string>;
}

export interface RetryCollapseMetricsRecord {
  agentId?: string;
  callPath?: string;
  headendId?: string;
  reason: 'final_report_attempt' | 'incomplete_final_report' | 'xml_wrapper_as_tool';
  turn: number;
  previousMaxTurns: number;
  newMaxTurns: number;
  customLabels?: Record<string, string>;
}

interface TelemetryRecorder {
  recordLlmMetrics: (record: LlmMetricsRecord) => void;
  recordToolMetrics: (record: ToolMetricsRecord) => void;
  recordContextGuardMetrics: (record: ContextGuardMetricsRecord) => void;
  recordQueueDepth: (record: QueueDepthRecord) => void;
  recordQueueWait: (record: QueueWaitRecord) => void;
  recordFinalReportMetrics: (record: FinalReportMetricsRecord) => void;
  recordRetryCollapseMetrics: (record: RetryCollapseMetricsRecord) => void;
  shutdown: () => Promise<void>;
}

class NoopRecorder implements TelemetryRecorder {
  recordLlmMetrics = (_record: LlmMetricsRecord): void => { /* noop */ };

  recordToolMetrics = (_record: ToolMetricsRecord): void => { /* noop */ };

  recordContextGuardMetrics = (_record: ContextGuardMetricsRecord): void => { /* noop */ };

  recordQueueDepth = (_record: QueueDepthRecord): void => { /* noop */ };

  recordQueueWait = (_record: QueueWaitRecord): void => { /* noop */ };

  recordFinalReportMetrics = (_record: FinalReportMetricsRecord): void => { /* noop */ };

  recordRetryCollapseMetrics = (_record: RetryCollapseMetricsRecord): void => { /* noop */ };

  shutdown = async (): Promise<void> => {
    // noop
  };
}

const requireModule = createRequire(import.meta.url);
const { version: PACKAGE_VERSION } = requireModule('../../package.json') as { version: string };

let recorder: TelemetryRecorder = new NoopRecorder();
let globalLabels: Record<string, string> = {};
const TRACER_NAME = 'ai-agent';
let tracerProvider: OtelTrace.NodeTracerProvider | undefined;
let activeLoggingConfig: TelemetryRuntimeLoggingConfig | undefined;
let logProvider: OtelLogs.LoggerProvider | undefined;
let logEmitter: ((event: StructuredLogEvent) => void) | undefined;

export function getTelemetryLabels(): Record<string, string> {
  return { ...globalLabels };
}

export function getTelemetryLoggingConfig(): TelemetryRuntimeLoggingConfig | undefined {
  if (activeLoggingConfig === undefined) return undefined;
  const cloned: TelemetryRuntimeLoggingConfig = {};
  if (activeLoggingConfig.formats !== undefined) cloned.formats = [...activeLoggingConfig.formats];
  if (activeLoggingConfig.extra !== undefined) cloned.extra = [...activeLoggingConfig.extra];
  if (typeof activeLoggingConfig.otlpEndpoint === 'string') cloned.otlpEndpoint = activeLoggingConfig.otlpEndpoint;
  if (typeof activeLoggingConfig.otlpTimeoutMs === 'number') cloned.otlpTimeoutMs = activeLoggingConfig.otlpTimeoutMs;
  return cloned;
}

export async function initTelemetry(config: TelemetryRuntimeConfig): Promise<void> {
  const labels = { ...(config.labels ?? {}) };
  labels.mode = config.mode;
  globalLabels = labels;

  activeLoggingConfig = cloneLoggingConfig(config.logging);

  const suppressed = telemetryDisabledByEnv();
  const enableMetrics = config.enabled && !suppressed;
  const enablePrometheus = enableMetrics && Boolean(config.prometheus?.enabled);
  const enableTraces = enableMetrics && Boolean(config.traces?.enabled);
  const enableLogExporter = !suppressed && (activeLoggingConfig?.extra?.includes('otlp') ?? false);

  if (!enableMetrics && !enableLogExporter) {
    await recorder.shutdown();
    recorder = new NoopRecorder();
    await shutdownTracing();
    await shutdownLogExporter();
    return;
  }

  const otel = await loadOtelDependencies(enablePrometheus);

  otel.diag.setLogger(new otel.DiagConsoleLogger(), otel.DiagLogLevel.ERROR);

  const resourceAttrs: Record<string, string> = {
    [otel.serviceNameAttribute]: 'ai-agent',
    [otel.serviceVersionAttribute]: PACKAGE_VERSION,
  };
  Object.entries(labels).forEach(([key, value]) => {
    resourceAttrs[`telemetry.label.${key}`] = value;
  });
  const resource = otel.resourceFromAttributes(resourceAttrs);

  if (enableMetrics) {
    const exporterOptions: Record<string, unknown> = {};
    if (typeof config.otlpEndpoint === 'string' && config.otlpEndpoint.length > 0) {
      exporterOptions.url = config.otlpEndpoint;
    }
    if (typeof config.otlpTimeoutMs === 'number' && Number.isFinite(config.otlpTimeoutMs)) {
      exporterOptions.timeoutMillis = config.otlpTimeoutMs;
    }
    exporterOptions.temporalityPreference = otel.AggregationTemporality.DELTA;
    const otlpExporter = new otel.OTLPMetricExporter(exporterOptions);
    if (config.mode === 'cli') {
      wrapCliExporter(otlpExporter, otel);
    }
    const metricReader = new otel.PeriodicExportingMetricReader({
      exporter: otlpExporter,
      exportIntervalMillis: 5000,
      exportTimeoutMillis: typeof config.otlpTimeoutMs === 'number' && Number.isFinite(config.otlpTimeoutMs) ? config.otlpTimeoutMs : 2000,
    });
    let prometheusExporter: PrometheusExporter | undefined;
    if (enablePrometheus && otel.PrometheusExporter !== undefined) {
      prometheusExporter = new otel.PrometheusExporter({
        host: config.prometheus?.host ?? '127.0.0.1',
        port: config.prometheus?.port ?? 9464,
      });
      await prometheusExporter.startServer();
    }

    const readers: MetricReader[] = [metricReader];
    if (prometheusExporter !== undefined) {
      readers.push(prometheusExporter as unknown as MetricReader);
    }

    const meterProvider = new otel.MeterProvider({ resource, readers });
    const meter = meterProvider.getMeter('ai-agent');
    const recorderImpl = new OtelMetricsRecorder({
      meterProvider,
      meter,
      prometheusExporter,
      globalLabels: labels,
      deps: otel,
    });

    await recorder.shutdown();
    recorder = recorderImpl;
  } else {
    await recorder.shutdown();
    recorder = new NoopRecorder();
  }

  await shutdownTracing();
  if (enableTraces) {
    setupTracing(otel, resource, config);
  }

  if (enableLogExporter) {
    await setupLogExporter(otel, resource, config);
  } else {
    await shutdownLogExporter();
  }
}

export async function shutdownTelemetry(): Promise<void> {
  await recorder.shutdown();
  recorder = new NoopRecorder();
  await shutdownTracing();
  await shutdownLogExporter();
}

export function recordLlmMetrics(record: LlmMetricsRecord): void {
  recorder.recordLlmMetrics(record);
}

export function recordToolMetrics(record: ToolMetricsRecord): void {
  recorder.recordToolMetrics(record);
}

export function recordQueueDepthMetrics(record: QueueDepthRecord): void {
  recorder.recordQueueDepth(record);
}

export function recordQueueWaitMetrics(record: QueueWaitRecord): void {
  recorder.recordQueueWait(record);
}

export function recordContextGuardMetrics(record: ContextGuardMetricsRecord): void {
  recorder.recordContextGuardMetrics(record);
}

export function recordFinalReportMetrics(record: FinalReportMetricsRecord): void {
  recorder.recordFinalReportMetrics(record);
}

export function recordRetryCollapseMetrics(record: RetryCollapseMetricsRecord): void {
  recorder.recordRetryCollapseMetrics(record);
}

export function emitTelemetryLog(event: StructuredLogEvent): void {
  if (logEmitter !== undefined) {
    try {
      logEmitter(event);
    } catch {
      // ignore log export errors; logging sinks should not throw
    }
  }
}

function cloneLoggingConfig(config?: TelemetryRuntimeLoggingConfig): TelemetryRuntimeLoggingConfig | undefined {
  if (config === undefined) return undefined;
  const clone: TelemetryRuntimeLoggingConfig = {};
  if (Array.isArray(config.formats) && config.formats.length > 0) clone.formats = [...config.formats];
  if (Array.isArray(config.extra) && config.extra.length > 0) clone.extra = [...config.extra];
  if (typeof config.otlpEndpoint === 'string' && config.otlpEndpoint.length > 0) clone.otlpEndpoint = config.otlpEndpoint;
  if (
    typeof config.otlpTimeoutMs === 'number'
    && Number.isFinite(config.otlpTimeoutMs)
    && config.otlpTimeoutMs > 0
  ) {
    clone.otlpTimeoutMs = config.otlpTimeoutMs;
  }
  return Object.keys(clone).length > 0 ? clone : undefined;
}

async function setupLogExporter(
  otel: OtelDependencies,
  resource: OtelResource,
  config: TelemetryRuntimeConfig,
): Promise<void> {
  await shutdownLogExporter();

  const loggingEndpoint = activeLoggingConfig?.otlpEndpoint ?? config.logging?.otlpEndpoint ?? config.otlpEndpoint;
  const loggingTimeout = activeLoggingConfig?.otlpTimeoutMs ?? config.logging?.otlpTimeoutMs ?? config.otlpTimeoutMs;

  const exporterOptions: Record<string, unknown> = {};
  if (typeof loggingEndpoint === 'string' && loggingEndpoint.length > 0) {
    exporterOptions.url = loggingEndpoint;
  }
  if (typeof loggingTimeout === 'number' && Number.isFinite(loggingTimeout)) {
    exporterOptions.timeoutMillis = loggingTimeout;
  }

  const exporter = new otel.OTLPLogExporter(exporterOptions);
  if (config.mode === 'cli') {
    wrapCliLogExporter(exporter, otel);
  }

  const processorOptions = config.mode === 'cli'
    ? { scheduledDelayMillis: 200, exportTimeoutMillis: typeof loggingTimeout === 'number' ? loggingTimeout : 2000 }
    : undefined;

  const processor = new otel.BatchLogRecordProcessor(exporter, processorOptions);
  const provider = new otel.LoggerProvider({ resource, processors: [processor] });
  otelLogs.setGlobalLoggerProvider(provider);
  const logger = provider.getLogger('ai-agent');

  const severityMap = otel.SeverityNumber;
  logProvider = provider;
  logEmitter = (event) => {
    try {
      logger.emit({
        body: event.message,
        severityText: event.severity,
        severityNumber: mapSeverityNumber(event.severity, severityMap),
        timestamp: otel.millisToHrTime(event.timestamp),
        attributes: buildLogAttributes(event),
      });
    } catch {
      // drop log on exporter failure
    }
  };
}

async function shutdownLogExporter(): Promise<void> {
  if (logProvider !== undefined) {
    try {
      await logProvider.shutdown();
    } catch {
      // ignore shutdown errors
    }
    logProvider = undefined;
  }
  logEmitter = undefined;
  try {
    otelLogs.disable();
  } catch {
    // ignore disable errors
  }
}

function wrapCliLogExporter(
  exporter: InstanceType<OtelDependencies['OTLPLogExporter']>,
  deps: OtelDependencies,
): void {
  const originalExport = exporter.export.bind(exporter);
  exporter.export = (records, callback) => {
    const wrapped = (result: OtelCore.ExportResult) => {
      if (result.code === deps.ExportResultCode.FAILED) {
        deps.diag.error('OTLP log export failed (dropping batch)');
      }
      callback(result);
    };
    originalExport(records, wrapped);
  };
}

function mapSeverityNumber(
  severity: StructuredLogEvent['severity'],
  severityEnum: OtelDependencies['SeverityNumber'],
): OtelLogsApi.SeverityNumber {
  switch (severity) {
    case 'ERR':
      return severityEnum.ERROR;
    case 'WRN':
      return severityEnum.WARN;
    case 'FIN':
      return severityEnum.INFO;
    case 'TRC':
      return severityEnum.TRACE;
    case 'VRB':
    case 'THK':
    default:
      return severityEnum.DEBUG;
  }
}

function buildLogAttributes(event: StructuredLogEvent): OtelLogsApi.AnyValueMap {
  const attributes: OtelLogsApi.AnyValueMap = {
    type: event.type,
    direction: event.direction,
    turn: event.turn,
    subturn: event.subturn,
    priority: event.priority,
    severity: event.severity,
    ts: event.isoTimestamp,
  };
  if (event.messageId !== undefined) attributes.message_id = event.messageId;
  if (typeof event.toolKind === 'string') attributes.tool_kind = event.toolKind;
  if (typeof event.toolNamespace === 'string') attributes.tool_namespace = event.toolNamespace;
  if (typeof event.tool === 'string') attributes.tool = event.tool;
  if (typeof event.headendId === 'string') attributes.headend = event.headendId;
  if (typeof event.agentId === 'string') attributes.agent = event.agentId;
  if (typeof event.callPath === 'string') attributes.call_path = event.callPath;
  if (typeof event.txnId === 'string') attributes.txn_id = event.txnId;
  if (typeof event.parentTxnId === 'string') attributes.parent_txn_id = event.parentTxnId;
  if (typeof event.originTxnId === 'string') attributes.origin_txn_id = event.originTxnId;
  if (typeof event.remoteIdentifier === 'string') attributes.remote = event.remoteIdentifier;
  if (typeof event.provider === 'string') attributes.provider = event.provider;
  if (typeof event.model === 'string') attributes.model = event.model;
  Object.entries(event.labels).forEach(([key, value]) => {
    attributes[`label.${key}`] = value;
  });
  return attributes;
}

function telemetryDisabledByEnv(): boolean {
  const raw = process.env.AI_TELEMETRY_DISABLE;
  if (typeof raw !== 'string') return false;
  const normalized = raw.trim().toLowerCase();
  return normalized === '1' || normalized === 'true' || normalized === 'yes' || normalized === 'on';
}

type Meter = OtelApi.Meter;
type MeterProvider = OtelMetrics.MeterProvider;
type MetricReader = OtelMetrics.MetricReader;
type PrometheusExporter = OtelProm.PrometheusExporter;

type CounterInstrument = ReturnType<Meter['createCounter']>;
type HistogramInstrument = ReturnType<Meter['createHistogram']>;
type ObservableGaugeInstrument = ReturnType<Meter['createObservableGauge']>;

interface OtelDependencies {
  diag: OtelApi.DiagAPI;
  DiagConsoleLogger: typeof OtelApi.DiagConsoleLogger;
  DiagLogLevel: typeof OtelApi.DiagLogLevel;
  MeterProvider: typeof OtelMetrics.MeterProvider;
  PeriodicExportingMetricReader: typeof OtelMetrics.PeriodicExportingMetricReader;
  AggregationTemporality: typeof OtelMetrics.AggregationTemporality;
  OTLPMetricExporter: typeof OtelOtlp.OTLPMetricExporter;
  PrometheusExporter?: typeof OtelProm.PrometheusExporter;
  resourceFromAttributes: typeof OtelResources.resourceFromAttributes;
  defaultResource: typeof OtelResources.defaultResource;
  emptyResource: typeof OtelResources.emptyResource;
  serviceNameAttribute: string;
  serviceVersionAttribute: string;
  ExportResultCode: typeof OtelCore.ExportResultCode;
  millisToHrTime: typeof OtelCore.millisToHrTime;
  NodeTracerProvider: typeof OtelTrace.NodeTracerProvider;
  BatchSpanProcessor: typeof OtelTrace.BatchSpanProcessor;
  ParentBasedSampler: typeof OtelTraceBase.ParentBasedSampler;
  TraceIdRatioBasedSampler: typeof OtelTraceBase.TraceIdRatioBasedSampler;
  AlwaysOnSampler: typeof OtelTraceBase.AlwaysOnSampler;
  AlwaysOffSampler: typeof OtelTraceBase.AlwaysOffSampler;
  OTLPTraceExporter: typeof OtelTraceOtlp.OTLPTraceExporter;
  LoggerProvider: typeof OtelLogs.LoggerProvider;
  BatchLogRecordProcessor: typeof OtelLogs.BatchLogRecordProcessor;
  OTLPLogExporter: typeof OtelLogsOtlp.OTLPLogExporter;
  SeverityNumber: typeof OtelLogsApi.SeverityNumber;
}

type OtelResource = ReturnType<OtelDependencies['resourceFromAttributes']>;

async function loadOtelDependencies(includePrometheus: boolean): Promise<OtelDependencies> {
  const apiModule = (await import('@opentelemetry/api')) as typeof OtelApi;
  const metricsModule = (await import('@opentelemetry/sdk-metrics')) as typeof OtelMetrics;
  const metricExporterModule = (await import('@opentelemetry/exporter-metrics-otlp-grpc')) as typeof OtelOtlp;
  const resourcesModule = (await import('@opentelemetry/resources')) as typeof OtelResources;
  const semanticModule = (await import('@opentelemetry/semantic-conventions')) as typeof OtelSemantic;
  const coreModule = (await import('@opentelemetry/core')) as typeof OtelCore;
  const logsModule = (await import('@opentelemetry/sdk-logs')) as typeof OtelLogs;
  const logExporterModule = (await import('@opentelemetry/exporter-logs-otlp-grpc')) as typeof OtelLogsOtlp;
  const logsApiModule = (await import('@opentelemetry/api-logs')) as typeof OtelLogsApi;
  const promModule = includePrometheus
    ? ((await import('@opentelemetry/exporter-prometheus')) as typeof OtelProm)
    : undefined;
  const traceNodeModule = (await import('@opentelemetry/sdk-trace-node')) as typeof OtelTrace;
  const traceBaseModule = (await import('@opentelemetry/sdk-trace-base')) as typeof OtelTraceBase;
  const traceExporterModule = (await import('@opentelemetry/exporter-trace-otlp-grpc')) as typeof OtelTraceOtlp;

  return {
    diag: apiModule.diag,
    DiagConsoleLogger: apiModule.DiagConsoleLogger,
    DiagLogLevel: apiModule.DiagLogLevel,
    MeterProvider: metricsModule.MeterProvider,
    PeriodicExportingMetricReader: metricsModule.PeriodicExportingMetricReader,
    AggregationTemporality: metricsModule.AggregationTemporality,
    OTLPMetricExporter: metricExporterModule.OTLPMetricExporter,
    PrometheusExporter: promModule?.PrometheusExporter,
    resourceFromAttributes: resourcesModule.resourceFromAttributes,
    defaultResource: resourcesModule.defaultResource,
    emptyResource: resourcesModule.emptyResource,
    serviceNameAttribute: semanticModule.ATTR_SERVICE_NAME,
    serviceVersionAttribute: semanticModule.ATTR_SERVICE_VERSION,
    ExportResultCode: coreModule.ExportResultCode,
    millisToHrTime: coreModule.millisToHrTime,
    NodeTracerProvider: traceNodeModule.NodeTracerProvider,
    BatchSpanProcessor: traceNodeModule.BatchSpanProcessor,
    ParentBasedSampler: traceBaseModule.ParentBasedSampler,
    TraceIdRatioBasedSampler: traceBaseModule.TraceIdRatioBasedSampler,
    AlwaysOnSampler: traceBaseModule.AlwaysOnSampler,
    AlwaysOffSampler: traceBaseModule.AlwaysOffSampler,
    OTLPTraceExporter: traceExporterModule.OTLPTraceExporter,
    LoggerProvider: logsModule.LoggerProvider,
    BatchLogRecordProcessor: logsModule.BatchLogRecordProcessor,
    OTLPLogExporter: logExporterModule.OTLPLogExporter,
    SeverityNumber: logsApiModule.SeverityNumber,
  };
}

function wrapCliExporter(exporter: InstanceType<OtelDependencies['OTLPMetricExporter']>, deps: OtelDependencies): void {
  const originalExport = exporter.export.bind(exporter);
  exporter.export = (metrics, resultCallback) => {
    const wrapped = (result: OtelCore.ExportResult) => {
      if (result.code === deps.ExportResultCode.FAILED) {
        deps.diag.error('OTLP metric export failed (dropping batch)');
      }
      resultCallback(result);
    };
    originalExport(metrics, wrapped);
  };
}

class OtelMetricsRecorder implements TelemetryRecorder {
  private readonly meterProvider: MeterProvider;
  private readonly meter: Meter;
  private readonly prometheusExporter?: PrometheusExporter;
  private readonly labels: Record<string, string>;
  private readonly llm: {
    latency: HistogramInstrument;
    requests: CounterInstrument;
    promptTokens: CounterInstrument;
    completionTokens: CounterInstrument;
    cacheReadTokens: CounterInstrument;
    cacheWriteTokens: CounterInstrument;
    bytesIn: CounterInstrument;
    bytesOut: CounterInstrument;
    errors: CounterInstrument;
    retries: CounterInstrument;
    cost: CounterInstrument;
  };
  private readonly tool: {
    latency: HistogramInstrument;
    invocations: CounterInstrument;
    bytesIn: CounterInstrument;
    bytesOut: CounterInstrument;
    errors: CounterInstrument;
  };
  private readonly contextGuard: {
    events: CounterInstrument;
    remainingGauge: ObservableGaugeInstrument;
  };
  private readonly contextGuardGaugeValues = new Map<string, { labels: Attributes; value: number }>();
  private readonly finalReport: {
    outcomes: CounterInstrument;
    attempts: CounterInstrument;
    turns: HistogramInstrument;
  };
  private readonly retry: {
    collapse: CounterInstrument;
  };
  private readonly queue: {
    depthGauge: ObservableGaugeInstrument;
    inUseGauge: ObservableGaugeInstrument;
    waitGauge: ObservableGaugeInstrument;
    waitHistogram: HistogramInstrument;
  };
  private readonly queueDepthValues = new Map<string, { labels: Attributes; waiting: number; inUse: number }>();
  private readonly queueWaitGaugeValues = new Map<string, { labels: Attributes; value: number }>();
  private readonly deps: OtelDependencies;

  constructor(params: {
    meterProvider: MeterProvider;
    meter: Meter;
    prometheusExporter?: PrometheusExporter;
    globalLabels: Record<string, string>;
    deps: OtelDependencies;
  }) {
    this.meterProvider = params.meterProvider;
    this.meter = params.meter;
    this.prometheusExporter = params.prometheusExporter;
    this.labels = params.globalLabels;
    this.deps = params.deps;

    this.llm = {
      latency: this.meter.createHistogram('ai_agent_llm_latency_ms', { description: 'Latency of LLM calls (milliseconds)' }),
      requests: this.meter.createCounter('ai_agent_llm_requests_total', { description: 'Total number of LLM requests' }),
      promptTokens: this.meter.createCounter('ai_agent_llm_prompt_tokens_total', { description: 'Prompt tokens used by LLM calls' }),
      completionTokens: this.meter.createCounter('ai_agent_llm_completion_tokens_total', { description: 'Completion tokens used by LLM calls' }),
      cacheReadTokens: this.meter.createCounter('ai_agent_llm_cache_read_tokens_total', { description: 'Cache read tokens used by LLM calls' }),
      cacheWriteTokens: this.meter.createCounter('ai_agent_llm_cache_write_tokens_total', { description: 'Cache write tokens produced by LLM calls' }),
      bytesIn: this.meter.createCounter('ai_agent_llm_bytes_in_total', { description: 'Input bytes sent to LLM providers' }),
      bytesOut: this.meter.createCounter('ai_agent_llm_bytes_out_total', { description: 'Output bytes received from LLM providers' }),
      errors: this.meter.createCounter('ai_agent_llm_errors_total', { description: 'Total number of LLM errors' }),
      retries: this.meter.createCounter('ai_agent_llm_retries_total', { description: 'Total number of LLM retries' }),
      cost: this.meter.createCounter('ai_agent_llm_cost_usd_total', { description: 'Total reported LLM cost in USD' }),
    };

    this.tool = {
      latency: this.meter.createHistogram('ai_agent_tool_latency_ms', { description: 'Latency of tool executions (milliseconds)' }),
      invocations: this.meter.createCounter('ai_agent_tool_invocations_total', { description: 'Total number of tool executions' }),
      bytesIn: this.meter.createCounter('ai_agent_tool_bytes_in_total', { description: 'Input bytes passed to tools' }),
      bytesOut: this.meter.createCounter('ai_agent_tool_bytes_out_total', { description: 'Output bytes produced by tools' }),
      errors: this.meter.createCounter('ai_agent_tool_errors_total', { description: 'Total number of tool execution errors' }),
    };

    this.contextGuard = {
      events: this.meter.createCounter('ai_agent_context_guard_events_total', {
        description: 'Total number of context guard activations',
      }),
      remainingGauge: this.meter.createObservableGauge('ai_agent_context_guard_remaining_tokens', {
        description: 'Remaining token budget when the context guard activates',
      }),
    };

    this.contextGuard.remainingGauge.addCallback((observable) => {
      this.contextGuardGaugeValues.forEach((entry) => {
        observable.observe(entry.value, entry.labels);
      });
    });

    this.finalReport = {
      outcomes: this.meter.createCounter('ai_agent_final_report_total', {
        description: 'Final reports emitted grouped by source',
      }),
      attempts: this.meter.createCounter('ai_agent_final_report_attempts_total', {
        description: 'Final-report attempts observed before acceptance',
      }),
      turns: this.meter.createHistogram('ai_agent_final_report_turns', {
        description: 'Turn index when the final report was accepted',
      }),
    };

    this.retry = {
      collapse: this.meter.createCounter('ai_agent_retry_collapse_total', {
        description: 'Times remaining turns collapsed after a final-report attempt',
      }),
    };

    this.queue = {
      depthGauge: this.meter.createObservableGauge('ai_agent_queue_depth', {
        description: 'Number of queued tool executions awaiting a slot',
      }),
      inUseGauge: this.meter.createObservableGauge('ai_agent_queue_in_use', {
        description: 'Number of active tool executions consuming queue capacity',
      }),
      waitGauge: this.meter.createObservableGauge('ai_agent_queue_last_wait_ms', {
        description: 'Most recent observed wait duration per queue (milliseconds)',
      }),
      waitHistogram: this.meter.createHistogram('ai_agent_queue_wait_duration_ms', {
        description: 'Latency between enqueue and start time for queued tool executions (milliseconds)',
      }),
    };

    this.queue.depthGauge.addCallback((observable) => {
      this.queueDepthValues.forEach((entry) => {
        observable.observe(entry.waiting, entry.labels);
      });
    });
    this.queue.inUseGauge.addCallback((observable) => {
      this.queueDepthValues.forEach((entry) => {
        observable.observe(entry.inUse, entry.labels);
      });
    });
    this.queue.waitGauge.addCallback((observable) => {
      this.queueWaitGaugeValues.forEach((entry) => {
        observable.observe(entry.value, entry.labels);
      });
    });
  }

  recordLlmMetrics(record: LlmMetricsRecord): void {
    const labels = buildLabelSet({
      agent: record.agentId ?? 'unknown',
      call_path: record.callPath ?? 'unknown',
      provider: record.provider,
      model: record.model,
      headend: record.headendId ?? 'cli',
      status: record.status,
    }, this.labels, record.customLabels);

    if (record.reasoningLevel !== undefined) {
      labels.reasoning_level = record.reasoningLevel;
    }

    const latency = Number.isFinite(record.latencyMs) ? Math.max(0, record.latencyMs) : 0;
    this.llm.latency.record(latency, labels);
    this.llm.requests.add(1, labels);
    if (record.promptTokens > 0) this.llm.promptTokens.add(record.promptTokens, labels);
    if (record.completionTokens > 0) this.llm.completionTokens.add(record.completionTokens, labels);
    if (record.cacheReadTokens > 0) this.llm.cacheReadTokens.add(record.cacheReadTokens, labels);
    if (record.cacheWriteTokens > 0) this.llm.cacheWriteTokens.add(record.cacheWriteTokens, labels);
    if (record.requestBytes !== undefined && record.requestBytes > 0) this.llm.bytesIn.add(record.requestBytes, labels);
    if (record.responseBytes !== undefined && record.responseBytes > 0) this.llm.bytesOut.add(record.responseBytes, labels);
    if (record.retries !== undefined && record.retries > 0) this.llm.retries.add(record.retries, labels);
    if (record.costUsd !== undefined && Number.isFinite(record.costUsd) && record.costUsd > 0) {
      this.llm.cost.add(record.costUsd, labels);
    }
    if (record.status === 'error') {
      const errorLabels = { ...labels, error_type: record.errorType ?? 'unknown' };
      this.llm.errors.add(1, errorLabels);
    }
  }

  recordToolMetrics(record: ToolMetricsRecord): void {
    const labels = buildLabelSet({
      agent: record.agentId ?? 'unknown',
      call_path: record.callPath ?? 'unknown',
      tool: record.toolName,
      tool_kind: record.toolKind ?? 'unknown',
      provider: record.provider,
      headend: record.headendId ?? 'cli',
      status: record.status,
    }, this.labels, record.customLabels);

    const latency = Number.isFinite(record.latencyMs) ? Math.max(0, record.latencyMs) : 0;
    this.tool.latency.record(latency, labels);
    this.tool.invocations.add(1, labels);
    if (record.inputBytes !== undefined && record.inputBytes > 0) this.tool.bytesIn.add(record.inputBytes, labels);
    if (record.outputBytes !== undefined && record.outputBytes > 0) this.tool.bytesOut.add(record.outputBytes, labels);
    if (record.status === 'error') {
      const errorLabels = { ...labels, error_type: record.errorType ?? 'unknown' };
      this.tool.errors.add(1, errorLabels);
    }
  }

  recordContextGuardMetrics(record: ContextGuardMetricsRecord): void {
    const labels = buildLabelSet({
      agent: record.agentId ?? 'unknown',
      call_path: record.callPath ?? 'unknown',
      provider: record.provider,
      model: record.model,
      trigger: record.trigger,
      outcome: record.outcome,
      headend: record.headendId ?? 'cli',
    }, this.labels, record.customLabels);

    this.contextGuard.events.add(1, labels);

    const remaining = resolveRemainingTokens(record);
    const key = serializeLabelSet(labels);
    if (remaining !== undefined) {
      const gaugeLabels: Attributes = labels;
      this.contextGuardGaugeValues.set(key, { labels: gaugeLabels, value: remaining });
    } else {
      this.contextGuardGaugeValues.delete(key);
    }
  }

  recordQueueDepth(record: QueueDepthRecord): void {
    const baseLabels = buildLabelSet({
      queue: record.queue,
      capacity: String(record.capacity),
    }, this.labels);
    const attrLabels: Attributes = baseLabels;
    const waiting = Number.isFinite(record.waiting) ? Math.max(0, record.waiting) : 0;
    const inUse = Number.isFinite(record.inUse) ? Math.max(0, record.inUse) : 0;
    this.queueDepthValues.set(record.queue, { labels: attrLabels, waiting, inUse });
  }

  recordQueueWait(record: QueueWaitRecord): void {
    const latency = Number.isFinite(record.waitMs) ? Math.max(0, record.waitMs) : 0;
    const labels = buildLabelSet({ queue: record.queue }, this.labels);
    this.queue.waitHistogram.record(latency, labels);
    const attrLabels: Attributes = labels;
    this.queueWaitGaugeValues.set(record.queue, { labels: attrLabels, value: latency });
  }

  recordFinalReportMetrics(record: FinalReportMetricsRecord): void {
    const baseLabels: Record<string, string> = {
      agent: record.agentId ?? 'unknown',
      call_path: record.callPath ?? 'unknown',
      headend: record.headendId ?? 'cli',
      source: record.source,
      forced_final_reason: record.forcedFinalReason ?? 'none',
    };
    if (record.syntheticReason !== undefined && record.syntheticReason.length > 0) {
      baseLabels.synthetic_reason = record.syntheticReason;
    }
    const labels = buildLabelSet(baseLabels, this.labels, record.customLabels);
    this.finalReport.outcomes.add(1, labels);
    const attempts = Number.isFinite(record.finalReportAttempts) ? Math.max(0, record.finalReportAttempts) : 0;
    if (attempts > 0) {
      this.finalReport.attempts.add(attempts, labels);
    }
    const turns = Number.isFinite(record.turnsCompleted) ? Math.max(0, record.turnsCompleted) : 0;
    if (turns > 0) {
      this.finalReport.turns.record(turns, labels);
    }
  }

  recordRetryCollapseMetrics(record: RetryCollapseMetricsRecord): void {
    const labels = buildLabelSet({
      agent: record.agentId ?? 'unknown',
      call_path: record.callPath ?? 'unknown',
      headend: record.headendId ?? 'cli',
      reason: record.reason,
    }, this.labels, record.customLabels);
    this.retry.collapse.add(1, labels);
  }

  async shutdown(): Promise<void> {
    const promises: Promise<unknown>[] = [this.meterProvider.shutdown()];
    if (this.prometheusExporter !== undefined) {
      if (typeof this.prometheusExporter.stopServer === 'function') {
        promises.push(this.prometheusExporter.stopServer());
      } else if (typeof this.prometheusExporter.shutdown === 'function') {
        promises.push(this.prometheusExporter.shutdown());
      }
    }
    await Promise.allSettled(promises);
  }
}

function setupTracing(
  otel: OtelDependencies,
  resource: OtelResource,
  config: TelemetryRuntimeConfig,
): void {
  if (config.traces?.enabled !== true) return;

  const exporterOptions: Record<string, unknown> = {};
  if (typeof config.otlpEndpoint === 'string' && config.otlpEndpoint.length > 0) {
    exporterOptions.url = config.otlpEndpoint;
  }
  if (typeof config.otlpTimeoutMs === 'number' && Number.isFinite(config.otlpTimeoutMs)) {
    exporterOptions.timeoutMillis = config.otlpTimeoutMs;
  }
  const exporter = new otel.OTLPTraceExporter(exporterOptions);

  const processorOptions = config.mode === 'cli'
    ? { scheduledDelayMillis: 200, exportTimeoutMillis: config.otlpTimeoutMs ?? 2000 }
    : undefined;
  const spanProcessor = new otel.BatchSpanProcessor(exporter, processorOptions);
  const sampler = createSampler(otel, config.traces);
  const provider = new otel.NodeTracerProvider({
    resource,
    sampler,
    spanProcessors: [spanProcessor],
  });

  provider.register();
  tracerProvider = provider;
}

async function shutdownTracing(): Promise<void> {
  if (tracerProvider === undefined) return;
  try {
    await tracerProvider.shutdown();
  } catch {
    // ignore shutdown errors
  }
  tracerProvider = undefined;
  try {
    otelTrace.disable();
  } catch {
    // ignore disable errors
  }
}

function createSampler(otel: OtelDependencies, config: TelemetryRuntimeTracesConfig): OtelTraceBase.Sampler {
  const sampler = config.sampler;
  switch (sampler) {
    case 'always_off':
      return new otel.AlwaysOffSampler();
    case 'always_on':
      return new otel.AlwaysOnSampler();
    case 'ratio': {
      const ratio = Math.min(Math.max(config.ratio ?? 0.1, 0), 1);
      const ratioSampler = new otel.TraceIdRatioBasedSampler(ratio);
      return new otel.ParentBasedSampler({ root: ratioSampler });
    }
    case 'parent':
    default:
      return new otel.ParentBasedSampler({ root: new otel.AlwaysOnSampler() });
  }
}

export interface RunWithSpanOptions {
  attributes?: Attributes;
  kind?: SpanKind;
  links?: OtelApi.Link[];
  startTime?: number;
}

export function runWithSpan<T>(name: string, fn: (span: Span) => Promise<T> | T): Promise<T>;
export function runWithSpan<T>(name: string, options: RunWithSpanOptions, fn: (span: Span) => Promise<T> | T): Promise<T>;
export function runWithSpan<T>(
  name: string,
  optionsOrFn: RunWithSpanOptions | ((span: Span) => Promise<T> | T),
  maybeFn?: (span: Span) => Promise<T> | T,
): Promise<T> {
  const options: RunWithSpanOptions = typeof optionsOrFn === 'function' ? {} : optionsOrFn;
  const handler: (span: Span) => Promise<T> | T = typeof optionsOrFn === 'function'
    ? optionsOrFn
    : (() => {
        if (maybeFn === undefined) {
          throw new Error('runWithSpan requires a callback');
        }
        return maybeFn;
      })();
  const tracer = otelTrace.getTracer(TRACER_NAME);
  return tracer.startActiveSpan(name, {
    kind: options.kind,
    attributes: options.attributes,
    links: options.links,
    startTime: options.startTime,
  }, async (span) => {
    try {
      const result = await handler(span);
      return result;
    } catch (error) {
      const err = error instanceof Error ? error : new Error(String(error));
      span.recordException(err);
      span.setStatus({ code: SpanStatusCode.ERROR, message: err.message });
      throw error;
    } finally {
      span.end();
    }
  });
}

export function addSpanAttributes(attributes: Attributes): void {
  const span = otelTrace.getActiveSpan();
  if (span === undefined) return;
  span.setAttributes(attributes);
}

export function recordSpanError(error: unknown): void {
  const span = otelTrace.getActiveSpan();
  if (span === undefined) return;
  const err = error instanceof Error ? error : new Error(String(error));
  span.recordException(err);
  span.setStatus({ code: SpanStatusCode.ERROR, message: err.message });
}

export function addSpanEvent(name: string, attributes?: Attributes): void {
  const span = otelTrace.getActiveSpan();
  if (span === undefined) return;
  span.addEvent(name, attributes);
}

function buildLabelSet(
  base: Record<string, string>,
  global: Record<string, string>,
  custom?: Record<string, string>
): Record<string, string> {
  const labels: Record<string, string> = { ...global, ...base };
  if (custom !== undefined) {
    Object.entries(custom).forEach(([key, value]) => {
      if (typeof value === 'string' && value.length > 0) {
        labels[key] = value;
      }
    });
  }
  return labels;
}

function serializeLabelSet(labels: Record<string, string>): string {
  return Object.keys(labels)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => `${key}=${labels[key]}`)
    .join('|');
}

function resolveRemainingTokens(record: ContextGuardMetricsRecord): number | undefined {
  const { remainingTokens, limitTokens, projectedTokens } = record;
  if (typeof remainingTokens === 'number' && Number.isFinite(remainingTokens)) {
    return Math.max(0, remainingTokens);
  }
  if (
    typeof limitTokens === 'number'
    && Number.isFinite(limitTokens)
    && Number.isFinite(projectedTokens)
  ) {
    const delta = limitTokens - projectedTokens;
    if (Number.isFinite(delta)) {
      return Math.max(0, delta);
    }
  }
  return undefined;
}
