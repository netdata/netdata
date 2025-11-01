import type { StructuredLogEvent } from './structured-log-event.js';

interface FormatOptions {
  color?: boolean;
}

const ANSI_RESET = '\u001B[0m';
const ANSI_RED = '\u001B[31m';
const ANSI_YELLOW = '\u001B[33m';
const ANSI_CYAN = '\u001B[36m';
const ANSI_GRAY = '\u001B[90m';

const COLOR_BY_SEVERITY: Partial<Record<StructuredLogEvent['severity'], string>> = {
  ERR: ANSI_RED,
  WRN: ANSI_YELLOW,
  FIN: ANSI_CYAN,
  VRB: ANSI_GRAY,
  THK: ANSI_GRAY,
  TRC: ANSI_GRAY,
};

function encodeValue(value: string): string {
  if (value === '') return '""';
  const needsQuotes = /\s|=|"/.test(value);
  const escaped = value.replace(/"/g, '\\"');
  return needsQuotes ? `"${escaped}"` : escaped;
}

function sanitizePayload(value: string): string {
  return value.replace(/\n/g, '\\n').replace(/\r/g, '\\r');
}

export function formatLogfmt(event: StructuredLogEvent, options: FormatOptions = {}): string {
  const pairs: [string, string][] = [];
  const seen = new Set<string>();
  const push = (key: string, value: string | undefined): void => {
    if (value === undefined || value.length === 0) return;
    if (seen.has(key)) return;
    pairs.push([key, value]);
    seen.add(key);
  };

  push('ts', event.isoTimestamp);
  push('level', event.severity.toLowerCase());
  push('priority', String(event.priority));
  push('type', event.type);
  push('direction', event.direction);
  push('turn', String(event.turn));
  push('subturn', String(event.subturn));
  if (event.messageId !== undefined) push('message_id', event.messageId);
  push('remote', event.remoteIdentifier);
  push('tool_kind', event.toolKind);
  push('tool_namespace', event.toolNamespace);
  push('tool', event.tool);
  push('headend', event.headendId);
  push('agent', event.agentId);
  push('agent_path', event.agentPath);
  push('call_path', event.callPath);
  push('turn_path', event.turnPath);
  push('txn_id', event.txnId);
  push('parent_txn_id', event.parentTxnId);
  push('origin_txn_id', event.originTxnId);
  push('provider', event.provider);
  push('model', event.model);
  if (event.llmRequestPayload !== undefined) {
    push('payload_request', sanitizePayload(event.llmRequestPayload.body));
    push('payload_request_format', event.llmRequestPayload.format);
  } else if (event.toolRequestPayload !== undefined) {
    push('payload_request', sanitizePayload(event.toolRequestPayload.body));
    push('payload_request_format', event.toolRequestPayload.format);
  }
  if (event.llmResponsePayload !== undefined) {
    push('payload_response', sanitizePayload(event.llmResponsePayload.body));
    push('payload_response_format', event.llmResponsePayload.format);
  } else if (event.toolResponsePayload !== undefined) {
    push('payload_response', sanitizePayload(event.toolResponsePayload.body));
    push('payload_response_format', event.toolResponsePayload.format);
  }

  Object.entries(event.labels).forEach(([key, value]) => {
    push(key, value);
  });

  // Render the free-form message last for readability.
  push('message', event.message);

  let line = pairs.map(([key, value]) => `${key}=${encodeValue(value)}`).join(' ');
  if (options.color === true) {
    const ansi = COLOR_BY_SEVERITY[event.severity];
    if (ansi !== undefined) {
      line = `${ansi}${line}${ANSI_RESET}`;
    }
  }
  return line;
}
