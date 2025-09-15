import type { AIAgentResult, AccountingEntry, ConversationMessage } from './types.js';

function bytesLen(s: string): number {
  return Buffer.byteLength(s, 'utf8');
}

function bytesPreview(s: string, maxBytes: number): string {
  const enc = new TextEncoder();
  const dec = new TextDecoder('utf-8', { fatal: false });
  const b = enc.encode(s);
  const slice = b.subarray(0, Math.min(maxBytes, b.byteLength));
  const decoded = dec.decode(slice);
  // Collapse newlines for single-line preview
  return decoded.replace(/[\r\n]+/g, ' ').trim();
}

function ensureTrailingNewline(s: string): string { return s.endsWith('\n') ? s : (s + '\n'); }



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

// Truncate a UTF-8 string to a max byte length with a standard prefix.
// Mirrors the truncation semantics used by MCP client so behavior stays consistent.
export function truncateUtf8WithNotice(s: string, limitBytes: number, originalSizeBytes?: number): string {
  if (limitBytes <= 0) return '';
  const size = typeof originalSizeBytes === 'number' && Number.isFinite(originalSizeBytes)
    ? Math.max(0, Math.trunc(originalSizeBytes))
    : Buffer.byteLength(s, 'utf8');
  if (size <= limitBytes) return s;

  const prefix = `[TRUNCATED] Original size ${String(size)} bytes; truncated to ${String(limitBytes)} bytes.\n\n`;
  const enc = new TextEncoder();
  const dec = new TextDecoder('utf-8', { fatal: false });
  const prefixBytes = enc.encode(prefix);
  if (prefixBytes.byteLength >= limitBytes) {
    const slice = prefixBytes.subarray(0, limitBytes);
    return dec.decode(slice);
  }
  const budget = limitBytes - prefixBytes.byteLength;
  const resBytes = enc.encode(s);
  const contentSlice = resBytes.subarray(0, Math.min(budget, resBytes.byteLength));
  const truncated = dec.decode(contentSlice);
  return prefix + truncated;
}

// Consistent warning logger for stderr
export function warn(message: string): void {
  try {
    const prefix = '[warn] ';
    const out = process.stderr.isTTY ? `\x1b[33m${prefix}${message}\x1b[0m` : `${prefix}${message}`;
    process.stderr.write(out + '\n');
  } catch {
    try { console.error(`[warn] ${message}`); } catch { /* noop */ }
  }
}
