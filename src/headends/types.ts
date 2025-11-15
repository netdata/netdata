import type { LogEntry } from '../types.js';

export type HeadendKind =
  | 'mcp'
  | 'openai-completions'
  | 'anthropic-completions'
  | 'api'
  | 'slack'
  | 'custom';

export interface HeadendDescription {
  id: string;
  kind: HeadendKind;
  label: string;
  details?: Record<string, unknown>;
}

export type HeadendClosedEvent =
  | { reason: 'stopped'; graceful: boolean }
  | { reason: 'error'; error: Error };

export type HeadendLogSink = (entry: LogEntry) => void;

export interface HeadendContext {
  log: HeadendLogSink;
  shutdownSignal: AbortSignal;
  stopRef: { stopping: boolean };
}

export interface Headend {
  readonly id: string;
  readonly kind: HeadendKind;
  readonly closed: Promise<HeadendClosedEvent>;
  describe: () => HeadendDescription;
  start: (context: HeadendContext) => Promise<void>;
  stop: () => Promise<void>;
}
