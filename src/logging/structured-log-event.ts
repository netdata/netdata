import type { LogEntry } from '../types.js';

import { resolveMessageId } from './message-ids.js';

export interface StructuredLogEvent {
  timestamp: number;
  isoTimestamp: string;
  severity: LogEntry['severity'];
  priority: number;
  message: string;
  messageId?: string;
  type: LogEntry['type'];
  direction: LogEntry['direction'];
  turn: number;
  subturn: number;
  toolKind?: string;
  toolProvider?: string;
  tool?: string;
  headendId?: string;
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  remoteIdentifier?: string;
  provider?: string;
  model?: string;
  labels: Record<string, string>;
  stack?: string;
}

const PRIORITY_BY_SEVERITY: Partial<Record<LogEntry['severity'], number>> = {
  ERR: 3,
  WRN: 4,
  FIN: 5,
  VRB: 6,
  THK: 6,
  TRC: 7,
};

const DEFAULT_PRIORITY = 6;

export interface BuildStructuredEventOptions {
  labels?: Record<string, string>;
}

export function buildStructuredLogEvent(
  entry: LogEntry,
  options: BuildStructuredEventOptions = {}
): StructuredLogEvent {
  const priority = PRIORITY_BY_SEVERITY[entry.severity] ?? DEFAULT_PRIORITY;
  const isoTimestamp = new Date(entry.timestamp).toISOString();
  const messageId = resolveMessageId(entry);

  const labels: Record<string, string> = {};
  if (typeof entry.agentId === 'string' && entry.agentId.length > 0) labels.agent = entry.agentId;
  if (typeof entry.callPath === 'string' && entry.callPath.length > 0) labels.call_path = entry.callPath;
  labels.severity = entry.severity;
  labels.type = entry.type;
  labels.direction = entry.direction;
  labels.turn = String(entry.turn);
  labels.subturn = String(entry.subturn);
  if (typeof entry.toolKind === 'string' && entry.toolKind.length > 0) labels.tool_kind = entry.toolKind;
  if (typeof entry.headendId === 'string' && entry.headendId.length > 0) labels.headend = entry.headendId;
  if (typeof entry.remoteIdentifier === 'string' && entry.remoteIdentifier.length > 0) {
    labels.remote = entry.remoteIdentifier;
    const parsed = parseRemoteIdentifier(entry.remoteIdentifier, entry.type);
    if (parsed.provider !== undefined) {
      labels.provider = parsed.provider;
      if (entry.type === 'tool') labels.tool_provider = parsed.provider;
    }
    if (parsed.model !== undefined) labels.model = parsed.model;
    if (parsed.tool !== undefined) labels.tool = parsed.tool;
  }
  Object.entries(options.labels ?? {}).forEach(([key, value]) => {
    if (typeof value === 'string' && value.length > 0) {
      labels[key] = value;
    }
  });
  if (entry.details !== undefined) {
    Object.entries(entry.details).forEach(([key, value]) => {
      if (typeof value === 'string') {
        if (value.length === 0) return;
        if (!Object.prototype.hasOwnProperty.call(labels, key)) labels[key] = value;
        return;
      }
      if (typeof value === 'number' && Number.isFinite(value)) {
        if (!Object.prototype.hasOwnProperty.call(labels, key)) {
          labels[key] = Number.isInteger(value) ? String(value) : value.toString();
        }
        return;
      }
      if (typeof value === 'boolean') {
        if (!Object.prototype.hasOwnProperty.call(labels, key)) labels[key] = value ? 'true' : 'false';
      }
    });
  }

  return {
    timestamp: entry.timestamp,
    isoTimestamp,
    severity: entry.severity,
    priority,
    message: entry.message,
    messageId,
    type: entry.type,
    direction: entry.direction,
    turn: entry.turn,
    subturn: entry.subturn,
    toolKind: entry.toolKind,
    toolProvider: entry.type === 'tool' ? labels.tool_provider : undefined,
    tool: entry.type === 'tool' ? labels.tool : undefined,
    headendId: entry.headendId,
    agentId: entry.agentId,
    callPath: entry.callPath,
    txnId: entry.txnId,
    parentTxnId: entry.parentTxnId,
    originTxnId: entry.originTxnId,
    remoteIdentifier: entry.remoteIdentifier,
    provider: labels.provider,
    model: labels.model,
    labels,
    stack: entry.stack,
  };
}

function parseRemoteIdentifier(
  identifier: string,
  type: LogEntry['type'],
): { provider?: string; model?: string; tool?: string } {
  if (type === 'llm') {
    const idx = identifier.indexOf(':');
    if (idx !== -1) {
      const provider = identifier.slice(0, idx);
      const model = identifier.slice(idx + 1);
      return { provider, model };
    }
    return { provider: identifier };
  }
  const idx = identifier.indexOf(':');
  if (idx !== -1) {
    const provider = identifier.slice(0, idx);
    const tool = identifier.slice(idx + 1);
    return { provider, model: tool, tool };
  }
  return { provider: identifier, tool: identifier };
}
