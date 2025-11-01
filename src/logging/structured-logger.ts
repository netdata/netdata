import type { LogEntry } from '../types.js';

import { emitTelemetryLog, getTelemetryLoggingConfig } from '../telemetry/index.js';

import { formatConsole } from './console-format.js';
import { acquireJournaldSink, isJournaldAvailable, type JournaldEmitter } from './journald-sink.js';
import { formatLogfmt } from './logfmt.js';
import { buildStructuredLogEvent, type StructuredLogEvent, type BuildStructuredEventOptions } from './structured-log-event.js';

export type LogFormat = 'journald' | 'logfmt' | 'json' | 'console';

type CandidateFormat = LogFormat | 'none';

export interface StructuredLoggerOptions {
  formats?: LogFormat[];
  labels?: Record<string, string>;
  color?: boolean;
  verbose?: boolean;
  logfmtWriter?: (line: string) => void;
  jsonWriter?: (line: string) => void;
  consoleWriter?: (line: string) => void;
}

export class StructuredLogger {
  private readonly labels: Record<string, string>;
  private readonly sinks: ((event: StructuredLogEvent) => void)[] = [];
  private readonly color: boolean;
  private readonly verbose: boolean;
  private readonly logfmtEmitter?: (event: StructuredLogEvent) => void;
  private logfmtRegistered = false;
  private journaldSink?: JournaldEmitter;
  private journaldDisabled = false;
  private journaldUnsubscribe?: () => void;

  constructor(options: StructuredLoggerOptions = {}) {
    this.labels = options.labels ?? {};
    this.color = options.color ?? false;
    this.verbose = options.verbose ?? false;
    const selectedFormat = selectLogFormat(options.formats);
    const logfmtWriter = options.logfmtWriter ?? defaultLogfmtWriter;
    this.logfmtEmitter = (event: StructuredLogEvent) => {
      const line = formatLogfmt(event, { color: this.color });
      logfmtWriter(`${line}\n`);
    };

    if (selectedFormat === 'journald') {
      const sink = acquireJournaldSink();
      if (sink !== undefined && !sink.isDisabled()) {
        this.journaldSink = sink;
        this.journaldUnsubscribe = sink.onDisable(() => {
          this.handleJournaldDisabled();
        });
        this.sinks.push((event) => {
          const success = sink.emit(event);
          if (!success) this.handleJournaldDisabled(event);
        });
      } else {
        this.handleJournaldDisabled();
      }
    }
    if (selectedFormat === 'logfmt') {
      this.registerLogfmtSink();
    }
    if (selectedFormat === 'json') {
      this.sinks.push((event) => {
        const writer = options.jsonWriter ?? defaultJsonWriter;
        writer(`${JSON.stringify(buildJsonPayload(event))}\n`);
      });
    }
    if (selectedFormat === 'console') {
      this.sinks.push((event) => {
        const writer = options.consoleWriter ?? defaultConsoleWriter;
        const line = formatConsole(event, { color: this.color, verbose: this.verbose });
        writer(`${line}\n`);
      });
    }
  }

  emit(entry: LogEntry): void {
    const options: BuildStructuredEventOptions = { labels: this.labels };
    const event = buildStructuredLogEvent(entry, options);
    emitTelemetryLog(event);
    this.sinks.forEach((sink) => {
      sink(event);
    });
  }

  shutdown(): void {
    if (this.journaldUnsubscribe !== undefined) {
      try { this.journaldUnsubscribe(); } catch { /* ignore */ }
      this.journaldUnsubscribe = undefined;
    }
  }

  private registerLogfmtSink(): void {
    if (this.logfmtEmitter === undefined) return;
    if (this.logfmtRegistered) return;
    this.logfmtRegistered = true;
    this.sinks.push(this.logfmtEmitter);
  }

  private handleJournaldDisabled(event?: StructuredLogEvent): void {
    if (this.journaldDisabled) {
      if (event !== undefined) this.logfmtEmitter?.(event);
      return;
    }
    this.journaldDisabled = true;
    this.journaldUnsubscribe?.();
    this.journaldUnsubscribe = undefined;
    this.registerLogfmtSink();
    if (event !== undefined) this.logfmtEmitter?.(event);
  }
}

export function createStructuredLogger(options: StructuredLoggerOptions): StructuredLogger {
  return new StructuredLogger(options);
}

function defaultLogfmtWriter(line: string): void {
  try {
    process.stderr.write(line);
  } catch {
    // ignore
  }
}

function defaultJsonWriter(line: string): void {
  try {
    process.stderr.write(line);
  } catch {
    // ignore
  }
}

function defaultConsoleWriter(line: string): void {
  try {
    process.stderr.write(line);
  } catch {
    // ignore
  }
}

function selectLogFormat(explicit?: LogFormat[]): CandidateFormat {
  const candidates = normalizeFormatPreference(explicit);
  const resolved = candidates.reduce<CandidateFormat | undefined>((current, candidate) => {
    if (current !== undefined) return current;
    if (candidate === 'none') return 'none';
    if (candidate === 'journald') return isJournaldAvailable() ? 'journald' : undefined;
    return candidate;
  }, undefined);

  if (resolved !== undefined) return resolved;
  return candidates.includes('none') ? 'none' : 'logfmt';
}

function normalizeFormatPreference(explicit?: LogFormat[]): CandidateFormat[] {
  const source: CandidateFormat[] = (() => {
    if (Array.isArray(explicit) && explicit.length > 0) return [...explicit];
    const config = getTelemetryLoggingConfig();
    if (config?.formats !== undefined && config.formats.length > 0) {
      return [...config.formats] as CandidateFormat[];
    }
    return isJournaldAvailable() ? ['journald', 'logfmt'] : ['logfmt'];
  })();

  return source.reduce<CandidateFormat[]>((acc, candidate) => {
    if (!acc.includes(candidate)) acc.push(candidate);
    return acc;
  }, []);
}

function buildJsonPayload(event: StructuredLogEvent): Record<string, unknown> {
  const entries: [string, unknown][] = [];
  const push = (key: string, value: unknown): void => {
    if (value === undefined) return;
    entries.push([key, value]);
  };

  push('ts', event.isoTimestamp);
  push('timestamp', event.timestamp);
  push('severity', event.severity);
  push('level', event.severity.toLowerCase());
  push('priority', event.priority);
  push('message_id', event.messageId);
  push('type', event.type);
  push('direction', event.direction);
  push('turn', event.turn);
  push('subturn', event.subturn);
  push('tool_kind', event.toolKind);
  push('tool_namespace', event.toolNamespace);
  push('tool', event.tool);
  push('headend', event.headendId);
  push('agent', event.agentId);
  push('call_path', event.callPath);
  push('txn_id', event.txnId);
  push('parent_txn_id', event.parentTxnId);
  push('origin_txn_id', event.originTxnId);
  push('remote', event.remoteIdentifier);
  push('provider', event.provider);
  push('model', event.model);
  if (event.llmRequestPayload !== undefined) {
    push('payload_request', event.llmRequestPayload.body);
    push('payload_request_format', event.llmRequestPayload.format);
  } else if (event.toolRequestPayload !== undefined) {
    push('payload_request', event.toolRequestPayload.body);
    push('payload_request_format', event.toolRequestPayload.format);
  }
  if (event.llmResponsePayload !== undefined) {
    push('payload_response', event.llmResponsePayload.body);
    push('payload_response_format', event.llmResponsePayload.format);
  } else if (event.toolResponsePayload !== undefined) {
    push('payload_response', event.toolResponsePayload.body);
    push('payload_response_format', event.toolResponsePayload.format);
  }
  push('labels', event.labels);

  entries.push(['message', event.message]);

  return Object.fromEntries(entries);
}
