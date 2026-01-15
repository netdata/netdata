import type { LogDetailValue, LogEntry } from '../types.js';

export const buildCompletionsLogEntry = (
  remoteIdentifier: string,
  headendId: string,
  label: string,
  message: string,
  severity: LogEntry['severity'] = 'VRB',
  fatal = false,
  details?: Record<string, LogDetailValue>,
): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier,
  fatal,
  message: `${label}: ${message}`,
  headendId,
  details,
});
