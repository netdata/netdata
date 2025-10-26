import type { Configuration, TelemetryLogExtra, TelemetryLogFormat, TelemetryTraceSampler } from '../types.js';
import type { TelemetryMode, TelemetryRuntimeConfig } from './index.js';

export interface TelemetryOverrides {
  enabled?: boolean;
  otlpEndpoint?: string;
  otlpTimeoutMs?: number;
  prometheusEnabled?: boolean;
  prometheusHost?: string;
  prometheusPort?: number;
  labels?: Record<string, string>;
  tracesEnabled?: boolean;
  traceSampler?: TelemetryTraceSampler;
  traceSamplerRatio?: number;
  logFormats?: TelemetryLogFormat[];
  logExtra?: TelemetryLogExtra[];
  logOtlpEndpoint?: string;
  logOtlpTimeoutMs?: number;
}

export function buildTelemetryRuntimeConfig(params: {
  configuration?: Configuration;
  overrides?: TelemetryOverrides;
  mode: TelemetryMode;
}): TelemetryRuntimeConfig {
  const base = params.configuration?.telemetry ?? {};
  const overrides = params.overrides ?? {};

  const enabled = overrides.enabled ?? base.enabled ?? false;
  const otlpEndpoint = overrides.otlpEndpoint ?? base.otlp?.endpoint;
  const otlpTimeoutMs = overrides.otlpTimeoutMs ?? base.otlp?.timeoutMs;
  const prometheusEnabled = overrides.prometheusEnabled ?? base.prometheus?.enabled ?? false;
  const prometheusHost = overrides.prometheusHost ?? base.prometheus?.host;
  const prometheusPort = overrides.prometheusPort ?? base.prometheus?.port;
  const tracesEnabled = overrides.tracesEnabled ?? base.traces?.enabled ?? false;
  const traceSampler = overrides.traceSampler ?? base.traces?.sampler;
  const traceSamplerRatio = overrides.traceSamplerRatio ?? base.traces?.ratio;

  const loggingFormats = overrides.logFormats ?? base.logging?.formats;
  const loggingExtra = overrides.logExtra ?? base.logging?.extra;
  const loggingOtlpEndpoint = overrides.logOtlpEndpoint ?? base.logging?.otlp?.endpoint;
  const loggingOtlpTimeoutMs = overrides.logOtlpTimeoutMs ?? base.logging?.otlp?.timeoutMs;

  const labels: Record<string, string> = { ...(base.labels ?? {}) };
  if (overrides.labels !== undefined) {
    Object.entries(overrides.labels).forEach(([key, value]) => {
      if (typeof value === 'string') {
        labels[key] = value;
      }
    });
  }

  const runtime: TelemetryRuntimeConfig = {
    enabled,
    mode: params.mode,
  };

  if (Object.keys(labels).length > 0) {
    runtime.labels = labels;
  }

  if (typeof otlpEndpoint === 'string' && otlpEndpoint.length > 0) {
    runtime.otlpEndpoint = otlpEndpoint;
  }
  if (typeof otlpTimeoutMs === 'number' && Number.isFinite(otlpTimeoutMs) && otlpTimeoutMs > 0) {
    runtime.otlpTimeoutMs = otlpTimeoutMs;
  }

  const hasPrometheusConfig =
    base.prometheus !== undefined
    || overrides.prometheusEnabled !== undefined
    || overrides.prometheusHost !== undefined
    || overrides.prometheusPort !== undefined;

  if (
    hasPrometheusConfig
    || prometheusEnabled
    || typeof prometheusHost === 'string'
    || typeof prometheusPort === 'number'
  ) {
    runtime.prometheus = {
      enabled: prometheusEnabled,
      host: prometheusHost,
      port: prometheusPort,
    };
  }

  const hasTraceConfig =
    base.traces !== undefined
    || overrides.tracesEnabled !== undefined
    || overrides.traceSampler !== undefined
    || overrides.traceSamplerRatio !== undefined;

  if (hasTraceConfig || tracesEnabled) {
    runtime.traces = {
      enabled: tracesEnabled,
      sampler: traceSampler ?? 'parent',
      ratio: traceSamplerRatio,
    };
  }

  const hasLoggingConfig =
    loggingFormats !== undefined
    || loggingExtra !== undefined
    || loggingOtlpEndpoint !== undefined
    || loggingOtlpTimeoutMs !== undefined;

  if (hasLoggingConfig) {
    runtime.logging = {};
    if (Array.isArray(loggingFormats) && loggingFormats.length > 0) {
      runtime.logging.formats = [...new Set(loggingFormats)];
    }
    if (Array.isArray(loggingExtra) && loggingExtra.length > 0) {
      runtime.logging.extra = [...new Set(loggingExtra)];
    }
    if (typeof loggingOtlpEndpoint === 'string' && loggingOtlpEndpoint.length > 0) {
      runtime.logging.otlpEndpoint = loggingOtlpEndpoint;
    }
    if (
      typeof loggingOtlpTimeoutMs === 'number'
      && Number.isFinite(loggingOtlpTimeoutMs)
      && loggingOtlpTimeoutMs > 0
    ) {
      runtime.logging.otlpTimeoutMs = loggingOtlpTimeoutMs;
    }
  }

  return runtime;
}
