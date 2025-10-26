import type { LogEntry } from './types.js';

import { formatLogfmt } from './logging/logfmt.js';
import { buildStructuredLogEvent } from './logging/structured-log-event.js';

interface LogFormatOptions {
  color?: boolean;
  labels?: Record<string, string>;
}

export function formatLog(entry: LogEntry, opts: LogFormatOptions = {}): string {
  const event = buildStructuredLogEvent(entry, { labels: opts.labels });
  return formatLogfmt(event, { color: opts.color });
}
