import { jsonrepair } from 'jsonrepair';

import type { AIAgentResult, AccountingEntry, ConversationMessage } from './types.js';

import { truncateToBytes } from './truncation.js';

/**
 * Special string values that indicate "do not send this parameter to the model".
 * When any of these strings are used, the parameter should resolve to `null`.
 */
const UNSET_PARAM_VALUES = new Set(['none', 'off', 'unset', 'default', 'null']);

/**
 * Check if a value is a special "unset" string that means "do not send to model".
 * Case-insensitive comparison.
 */
export function isUnsetParamValue(value: unknown): boolean {
  return typeof value === 'string' && UNSET_PARAM_VALUES.has(value.toLowerCase().trim());
}

/**
 * Parse a numeric parameter that supports special "unset" strings.
 * Returns:
 * - `null` if value is a special unset string (meaning "do not send")
 * - `undefined` if value is undefined/null (meaning "no override, use default")
 * - the number if value is a valid number
 * - `undefined` if value cannot be parsed as a number
 */
export function parseNumericParam(value: unknown): number | null | undefined {
  if (value === undefined || value === null) return undefined;
  if (isUnsetParamValue(value)) return null;
  if (typeof value === 'number') return Number.isNaN(value) ? undefined : value;
  if (typeof value === 'string') {
    const parsed = Number.parseFloat(value);
    return Number.isNaN(parsed) ? undefined : parsed;
  }
  return undefined;
}

function bytesLen(s: string): number {
  return Buffer.byteLength(s, 'utf8');
}

function bytesPreview(s: string, maxBytes: number): string {
  const truncated = truncateToBytes(s, maxBytes) ?? s;
  // Collapse newlines for single-line preview
  return truncated.replace(/[\r\n]+/g, ' ').trim();
}

function ensureTrailingNewline(s: string): string { return s.endsWith('\n') ? s : (s + '\n'); }

const TOOL_NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_\-]*(?:__[A-Za-z0-9_\-]+)*/;
const TOOL_NAME_INVALID_CHARS = /[^A-Za-z0-9_\-]+/g;
const TOOL_NAME_FALLBACK = 'tool_call';
export const TOOL_NAME_MAX_LENGTH = 200;

export const sanitizeToolName = (raw: string): string => {
  const withoutPrefix = raw.replace(/<\|[^|]+\|>/g, '').trim();
  if (withoutPrefix.length === 0) return TOOL_NAME_FALLBACK;
  const match = TOOL_NAME_PATTERN.exec(withoutPrefix);
  if (match !== null) return match[0];
  const replaced = withoutPrefix.replace(TOOL_NAME_INVALID_CHARS, '_');
  const stripped = replaced.replace(/^[^A-Za-z0-9]+/, '');
  return stripped.length > 0 ? stripped : TOOL_NAME_FALLBACK;
};

export function clampToolName(name: string, maxLength = TOOL_NAME_MAX_LENGTH): { name: string; truncated: boolean } {
  if (maxLength <= 0) {
    return { name: '', truncated: name.length > 0 };
  }
  if (name.length <= maxLength) {
    return { name, truncated: false };
  }
  return { name: name.slice(0, maxLength), truncated: true };
}

export const isPlainObject = (value: unknown): value is Record<string, unknown> => (
  value !== null && typeof value === 'object' && !Array.isArray(value)
);

const tryParseJson = (value: string): unknown => {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
};

const stripSurroundingCodeFence = (value: string): string | undefined => {
  const match = /^```(?:json)?\s*([\s\S]*?)\s*```$/iu.exec(value);
  return match !== null ? match[1] : undefined;
};

const stripTrailingEllipsis = (value: string): string | undefined => {
  const trimmed = value.replace(/(?:,\s*)?(?:\.{3}|…)(?:\s*\([^)]*\))?\s*$/u, '');
  return trimmed.length !== value.length ? trimmed : undefined;
};

const normalizeHexEscapes = (value: unknown): unknown => {
  const replaceHex = (s: string): string => {
    const toChar = (hex: unknown): string => {
      const hexStr = typeof hex === 'string' ? hex : String(hex);
      return String.fromCharCode(Number.parseInt(hexStr, 16));
    };
    return s
      .replace(/\\x([0-9a-fA-F]{2})/g, (_m, hex) => toChar(hex))
      .replace(/\bx([0-9a-fA-F]{2})\b/g, (_m, hex) => toChar(hex));
  };
  if (typeof value === 'string') return replaceHex(value);
  if (Array.isArray(value)) return value.map((v) => normalizeHexEscapes(v));
  if (isPlainObject(value)) {
    const entries = Object.entries(value).map(([k, v]) => [k, normalizeHexEscapes(v)]);
    return Object.fromEntries(entries);
  }
  return value;
};

const extractFirstJsonObject = (value: string): string | undefined => {
  let start = -1;
  let depth = 0;
  let inString = false;
  let escapeNext = false;
  // eslint-disable-next-line functional/no-loop-statements
  for (let i = 0; i < value.length; i += 1) {
    const ch = value[i] ?? '';
    if (inString) {
      if (escapeNext) {
        escapeNext = false;
        continue;
      }
      if (ch === '\\') {
        escapeNext = true;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === '{') {
      if (depth === 0) {
        start = i;
      }
      depth += 1;
      continue;
    }
    if (ch === '}') {
      if (depth === 0) {
        return undefined;
      }
      depth -= 1;
      if (depth === 0 && start !== -1) {
        return value.slice(start, i + 1);
      }
    }
  }
  return undefined;
};

const closeDanglingJson = (value: string): string | undefined => {
  let inString = false;
  let escapeNext = false;
  const stack: string[] = [];
  // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/prefer-for-of
  for (let i = 0; i < value.length; i += 1) {
    const ch = value[i] ?? '';
    if (inString) {
      if (escapeNext) {
        escapeNext = false;
        continue;
      }
      if (ch === '\\') {
        escapeNext = true;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === '{' || ch === '[') {
      stack.push(ch);
      continue;
    }
    if (ch === '}' || ch === ']') {
      if (stack.length === 0) {
        return undefined;
      }
      const expected = stack.pop();
      if ((ch === '}' && expected !== '{') || (ch === ']' && expected !== '[')) {
        return undefined;
      }
    }
  }
  if (inString || stack.length === 0) {
    return undefined;
  }
  const trimmed = value.replace(/[\s,]*$/u, '');
  const closers = stack.reverse().map((token) => (token === '{' ? '}' : ']')).join('');
  return `${trimmed}${closers}`;
};

export interface JsonParseDiagnostics {
  value?: unknown;
  repairs: string[];
  error?: string;
  originalText?: string;
  repairedText?: string;
}

const attemptRepairs = (text: string): { value?: unknown; repairedText?: string; steps: string[] } => {
  const steps: string[] = [];
  const parsed = tryParseJson(text);
  if (parsed !== undefined) {
    return { value: parsed, repairedText: text, steps };
  }
  try {
    const repaired = jsonrepair(text);
    const reparsed = tryParseJson(repaired);
    if (reparsed !== undefined) {
      return { value: reparsed, repairedText: repaired, steps: ['jsonrepair'] };
    }
  } catch {
    /* ignore jsonrepair failure; caller will handle */
  }
  return { steps: [] };
};

export const parseJsonValueDetailed = (raw: unknown): JsonParseDiagnostics => {
  if (isPlainObject(raw) || Array.isArray(raw)) {
    return { value: normalizeHexEscapes(raw), repairs: [] };
  }
  if (typeof raw !== 'string') {
    return { repairs: [], error: 'non_string' };
  }
  const originalText = raw.trim();
  if (originalText.length === 0) {
    return { repairs: [], error: 'empty', originalText };
  }

  const enqueue = (target: string | undefined, steps: string[]): { text: string; steps: string[] } | undefined => {
    if (target === undefined) return undefined;
    const normalized = target.trim();
    if (normalized.length === 0) return undefined;
    return { text: normalized, steps };
  };

  const queue: { text: string; steps: string[] }[] = [];
  const seen = new Set<string>();

  const hexFixed = originalText.replace(/\\x([0-9a-fA-F]{2})/g, (_match, hex) => `\\u00${String(hex).toUpperCase()}`);
  // Fix backslash-newline (line continuation) - models sometimes emit \ followed by literal newline
  // which is invalid JSON but intended as escaped newline
  const backslashNewlineFixed = originalText.replace(/\\\n/g, '\\n');
  const baseCandidates = [
    enqueue(originalText, []),
    enqueue(stripSurroundingCodeFence(originalText), ['stripCodeFence']),
    enqueue(stripTrailingEllipsis(originalText), ['stripTrailingEllipsis']),
    enqueue(hexFixed !== originalText ? hexFixed : undefined, ['hexEscapeFix']),
    enqueue(backslashNewlineFixed !== originalText ? backslashNewlineFixed : undefined, ['backslashNewlineFix']),
  ].filter((v): v is { text: string; steps: string[] } => v !== undefined);
  queue.push(...baseCandidates);

  let lastError: string | undefined;
  // eslint-disable-next-line functional/no-loop-statements
  while (queue.length > 0) {
    const candidate = queue.shift();
    if (candidate === undefined) break;
    if (seen.has(candidate.text)) continue;
    seen.add(candidate.text);

    const attempt = attemptRepairs(candidate.text);
    if (attempt.value !== undefined) {
      return {
        value: normalizeHexEscapes(attempt.value),
        repairs: [...candidate.steps, ...attempt.steps],
        originalText,
        repairedText: attempt.repairedText,
      };
    }

    // Explore secondary transforms only if parse failed
    const extracted = extractFirstJsonObject(candidate.text);
    if (extracted !== undefined) {
      const enriched = enqueue(extracted, [...candidate.steps, 'extractFirstObject']);
      if (enriched !== undefined) queue.push(enriched);
    }
    const closed = closeDanglingJson(candidate.text);
    if (closed !== undefined) {
      const enriched = enqueue(closed, [...candidate.steps, 'closeDangling']);
      if (enriched !== undefined) queue.push(enriched);
    }
    lastError = 'parse_failed';
    if (queue.length > 40) {
      break; // safety cap
    }
  }

  return { repairs: [], error: lastError ?? 'parse_failed', originalText };
};

export const parseJsonRecordDetailed = (raw: unknown): JsonParseDiagnostics & { value?: Record<string, unknown> } => {
  const detailed = parseJsonValueDetailed(raw);
  const value = isPlainObject(detailed.value) ? detailed.value : undefined;
  return { ...detailed, value };
};

export const parseJsonRecord = (raw: unknown): Record<string, unknown> | undefined => {
  const { value } = parseJsonRecordDetailed(raw);
  return value;
};



interface ToolCallSummary {
  request: string; // compact request line e.g., toolName(k:v, a:[3])
  outputPreview: string; // first N bytes as single line
  outputBytes: number;
}

interface TurnSummary {
  provider?: string;
  model?: string;
  actualProvider?: string;
  inputTokens?: number;
  outputTokens?: number;
  latencyMs?: number;
  toolCalls: ToolCallSummary[];
}

function segmentConversationByAssistant(conversation: ConversationMessage[]): { assistant: ConversationMessage; tools: ConversationMessage[] }[] {
  return conversation.reduce<{ assistant: ConversationMessage; tools: ConversationMessage[] }[]>((acc, m) => {
    if (m.role === 'assistant') {
      acc.push({ assistant: m, tools: [] });
      return acc;
    }
    if (m.role === 'tool') {
      if (acc.length > 0) {
        const last = acc[acc.length - 1];
        acc[acc.length - 1] = { assistant: last.assistant, tools: [...last.tools, m] };
      }
    }
    return acc;
  }, []);
}

function buildTurnSummaries(result: AIAgentResult): TurnSummary[] {
  const llmEntries = result.accounting.filter((e): e is Extract<AccountingEntry, { type: 'llm' }> => e.type === 'llm');
  const segments = segmentConversationByAssistant(result.conversation);

  // Pair attempts (accounting) with assistant/tool segments in order.
  // Not every attempt yields conversation messages (failed attempts don't),
  // so we advance over segments only when the attempt status is 'ok'.
  const turns: TurnSummary[] = [];
  let segIdx = 0;
  llmEntries.forEach((llm) => {
    const turn: TurnSummary = { toolCalls: [] };
    turn.provider = llm.provider;
    turn.model = llm.model;
    // actualProvider when routed (e.g., OpenRouter) – optional on llm accounting entries
    turn.actualProvider = (llm as unknown as { actualProvider?: string }).actualProvider;
    turn.inputTokens = llm.tokens.inputTokens;
    turn.outputTokens = llm.tokens.outputTokens;
    turn.latencyMs = llm.latency;

    // Attach tool calls from the next available assistant segment only for successful attempts
    if (llm.status === 'ok' && segIdx < segments.length) {
      const seg = segments[segIdx];
      segIdx += 1;
      const assistant = seg.assistant;
      const toolCalls = assistant.toolCalls ?? [];
      toolCalls.forEach((tc) => {
        const callId = tc.id;
        const toolMsg = seg.tools.find((t) => t.toolCallId === callId);
        const output = toolMsg?.content ?? '';
        const outputBytes = bytesLen(output);
        const outputPreview = bytesPreview(output, 160);
        const request = formatToolRequestCompact(tc.name, tc.parameters);
        turn.toolCalls.push({ request, outputPreview, outputBytes });
      });
    }

    turns.push(turn);
  });

  return turns;
}

function buildWhatWasDone(result: AIAgentResult): string {
  const turns = buildTurnSummaries(result);
  const lines: string[] = [];
  lines.push('Here is what the agent managed to do:');
  lines.push('');
  lines.push('Turns');
  turns.forEach((t, idx) => {
    const headParts: string[] = [];
    headParts.push(formatProviderModel(t.provider, t.model, t.actualProvider));
    if (t.inputTokens !== undefined && t.outputTokens !== undefined) {
      headParts.push(`tokens: input ${String(t.inputTokens)}, output ${String(t.outputTokens)}`);
    }
    if (typeof t.latencyMs === 'number') headParts.push(`latency ${String(t.latencyMs)}ms`);
    lines.push(`${String(idx + 1)}. ${headParts.join(', ')}`);
    t.toolCalls.forEach((c) => {
      lines.push(`  - ${c.request}`);
      lines.push(`    output: ${JSON.stringify(c.outputPreview)} ${String(c.outputBytes)} bytes`);
    });
  });
  return lines.join('\n');
}

export function formatAgentResultHumanReadable(result: AIAgentResult): string {
  // 1) Success with final output
  if (result.success && result.finalReport !== undefined) {
    const fr = result.finalReport;
    if (fr.format === 'json' && fr.content_json !== undefined) {
      let body = '';
      try { body = JSON.stringify(fr.content_json, null, 2); } catch { body = '[invalid json]'; }
      // Successful + non-empty: return only the final report (no appended activity section)
      return body;
    }
    if (typeof fr.content === 'string' && fr.content.trim().length > 0) {
      // Successful + non-empty: return only the final report (no appended activity section)
      return fr.content;
    }
    const serialized = (() => {
      try { return JSON.stringify(fr, null, 2); } catch { return undefined; }
    })();
    if (serialized !== undefined) return serialized;
    const keys = Object.keys(fr);
    const meta = keys.length > 0 ? `keys=${keys.join(', ')}` : 'empty object';
    return `[unserializable final report (${meta})]`;
  }

  // Build description of what has been done
  const description = buildWhatWasDone(result);

  // 2) Success but no final output
  if (result.success) {
    return ensureTrailingNewline([
      'AGENT COMPLETED WITHOUT OUTPUT',
      '',
      'The agent was able to complete successfully, but it did not generate any output.',
      '',
      description,
    ].join('\n'));
  }

  // 3) Failure with reason
  const reason = typeof result.error === 'string' && result.error.length > 0 ? result.error : 'Unknown error';
  return ensureTrailingNewline([
    'AGENT FAILED',
    '',
    'The agent was unable to complete. The exact reason of failure is:',
    '',
    reason,
    '',
    description,
  ].join('\n'));
}

// Compact tool request formatter usable across codepaths
export function formatToolRequestCompact(name: string, parameters: Record<string, unknown>): string {
  const fmtVal = (v: unknown): string => {
    if (v === null) return 'null';
    if (v === undefined) return 'undefined';
    const t = typeof v;
    if (t === 'string') {
      const s = (v as string).replace(/[\r\n]+/g, ' ').trim();
      return s.length > 160 ? `${s.slice(0, 160)}…` : s;
    }
    if (t === 'number') return (v as number).toString();
    if (t === 'bigint') return (v as bigint).toString();
    if (t === 'boolean') return (v as boolean) ? 'true' : 'false';
    if (Array.isArray(v)) return `[${String(v.length)}]`;
    return '{…}';
  };
  const entries = Object.entries(parameters);
  const paramStr = entries.length > 0
    ? '(' + entries.map(([k, v]) => `${k}:${fmtVal(v)}`).join(', ') + ')'
    : '()';
  return `${name}${paramStr}`;
}

// Standardized provider/model display: configured/actual:model
function formatProviderModel(provider?: string, model?: string, actualProvider?: string): string {
  const prov = provider ?? 'unknown';
  const mdl = model ?? 'unknown';
  if (actualProvider !== undefined && actualProvider.length > 0 && actualProvider !== prov) {
    return `${prov}/${actualProvider}:${mdl}`;
  }
  return `${prov}:${mdl}`;
}

let warningSink: ((message: string) => void) | undefined;

export function setWarningSink(handler?: (message: string) => void): void {
  warningSink = handler;
}

export function getWarningSink(): ((message: string) => void) | undefined {
  return warningSink;
}

// Consistent warning logger routed through injectable sink to keep core silent
export function warn(message: string): void {
  const sink = warningSink;
  if (sink === undefined) {
    return;
  }
  try {
    sink(message);
  } catch {
    /* ignore sink failures to keep core resilient */
  }
}

export { appendCallPathSegment, normalizeCallPath } from './utils/call-path.js';

export const safeJsonByteLength = (value: unknown): number => {
  try {
    return Buffer.byteLength(JSON.stringify(value), 'utf8');
  } catch {
    if (typeof value === 'string') return Buffer.byteLength(value, 'utf8');
    if (value === null || value === undefined) return 0;
    if (typeof value === 'object') {
      let total = 0;
      Object.values(value as Record<string, unknown>).forEach((nested) => {
        total += safeJsonByteLength(nested);
      });
      return total;
    }
    return 0;
  }
};

export const estimateMessagesBytes = (messages: readonly ConversationMessage[] | undefined): number => {
  if (messages === undefined || messages.length === 0) return 0;
  return messages.reduce((total, message) => total + safeJsonByteLength(message), 0);
};
