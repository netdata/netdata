import type { AccountingEntry, LogEntry } from './types.js';

interface TreeCallbacks {
  onLog?: (entry: LogEntry) => void;
  onAccounting?: (entry: AccountingEntry) => void;
}

interface SessionMeta {
  sessionId: string;
  agentId?: string;
  callPath?: string;
  maxTurns?: number;
}

interface TreeSnapshot {
  sessionId: string;
  agentId?: string;
  callPath?: string;
  startedAt?: number;
  endedAt?: number;
  success?: boolean;
  error?: string;
  counters: {
    logs: number;
    accounting: number;
    llmRequests: number;
    llmResponses: number;
    toolRequests: number;
    toolResponses: number;
    errors: number;
  };
}

interface Span {
  id: string;
  name: string;
  kind: string;
  startedAt: number;
  endedAt?: number;
  attributes?: Record<string, unknown>;
}

// Central execution tree for a single session. Source of truth for logs/accounting.
export class ExecutionTree {
  private readonly meta: SessionMeta;
  private readonly callbacks: TreeCallbacks;
  private readonly logs: LogEntry[] = [];
  private readonly accounting: AccountingEntry[] = [];
  private startedAt?: number;
  private endedAt?: number;
  private success?: boolean;
  private error?: string;
  private spans: Map<string, Span> = new Map<string, Span>();

  private counters = {
    logs: 0,
    accounting: 0,
    llmRequests: 0,
    llmResponses: 0,
    toolRequests: 0,
    toolResponses: 0,
    errors: 0,
  };

  constructor(meta: SessionMeta, callbacks?: TreeCallbacks) {
    this.meta = meta;
    this.callbacks = callbacks ?? {};
  }

  startSession(): void {
    this.startedAt ??= Date.now();
  }

  endSession(success: boolean, error?: string): void {
    this.endedAt = Date.now();
    this.success = success;
    this.error = error;
  }

  // Record a structured log line; acts as the central dispatcher.
  recordLog(entry: LogEntry): void {
    this.logs.push(entry);
    this.counters.logs += 1;

    // Increment basic request/response counters for LLM vs tool
    switch (entry.type) {
      case 'llm':
        if (entry.direction === 'request') this.counters.llmRequests += 1;
        else this.counters.llmResponses += 1;
        break;
      case 'tool':
        if (entry.direction === 'request') this.counters.toolRequests += 1;
        else this.counters.toolResponses += 1;
        break;
      default:
        break;
    }
    if (entry.severity === 'ERR') this.counters.errors += 1;

    // Fan out to external sink for live rendering
    try { this.callbacks.onLog?.(entry); } catch { /* ignore external log errors */ }
  }

  // Record an accounting entry; centralized for cost/latency aggregation.
  recordAccounting(entry: AccountingEntry): void {
    this.accounting.push(entry);
    this.counters.accounting += 1;
    try { this.callbacks.onAccounting?.(entry); } catch { /* ignore external accounting errors */ }
  }

  getSnapshot(): TreeSnapshot {
    return {
      sessionId: this.meta.sessionId,
      agentId: this.meta.agentId,
      callPath: this.meta.callPath,
      startedAt: this.startedAt,
      endedAt: this.endedAt,
      success: this.success,
      error: this.error,
      counters: { ...this.counters },
    };
  }

  // Expose raw buffers for serialization or downstream consumers.
  getLogs(): LogEntry[] { return [...this.logs]; }
  getAccounting(): AccountingEntry[] { return [...this.accounting]; }

  // Spans API for structured tracing
  startSpan(name: string, kind: string, attributes?: Record<string, unknown>): string {
    const id = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
    const span: Span = { id, name, kind, startedAt: Date.now(), attributes };
    this.spans.set(id, span);
    // Emit a lightweight trace log for span start
    const startLog: LogEntry = {
      timestamp: span.startedAt,
      severity: 'TRC',
      turn: 0,
      subturn: 0,
      direction: 'request',
      type: kind === 'llm' ? 'llm' : 'tool',
      remoteIdentifier: `span:${kind}:${name}`,
      fatal: false,
      message: 'start',
      agentId: this.meta.agentId,
      callPath: this.meta.callPath,
      ...(typeof this.meta.maxTurns === 'number' ? { 'max_turns': this.meta.maxTurns } : {})
    };
    this.recordLog(startLog);
    return id;
  }

  endSpan(id: string, status?: 'ok' | 'failed', attributes?: Record<string, unknown>): void {
    const span = this.spans.get(id);
    const endedAt = Date.now();
    if (span !== undefined) {
      span.endedAt = endedAt;
      if (attributes !== undefined) {
        span.attributes = { ...(span.attributes ?? {}), ...attributes };
      }
      this.spans.set(id, span);
    }
    const dur = (span?.startedAt !== undefined) ? (endedAt - span.startedAt) : undefined;
    const msg = (dur !== undefined) ? `end duration=${String(dur)}ms${status !== undefined ? ` status=${status}` : ''}` : `end${status !== undefined ? ` status=${status}` : ''}`;
    const endLog: LogEntry = {
      timestamp: endedAt,
      severity: (status === 'failed') ? 'ERR' : 'TRC',
      turn: 0,
      subturn: 0,
      direction: 'response',
      type: span?.kind === 'llm' ? 'llm' : 'tool',
      remoteIdentifier: `span:${span?.kind ?? 'unknown'}:${span?.name ?? id}`,
      fatal: false,
      message: msg,
      agentId: this.meta.agentId,
      callPath: this.meta.callPath,
      ...(typeof this.meta.maxTurns === 'number' ? { 'max_turns': this.meta.maxTurns } : {})
    };
    this.recordLog(endLog);
  }

  // Render a human-readable ASCII dump of the entire tree for debugging
  renderAscii(): string {
    const snap = this.getSnapshot();
    const lines: string[] = [];

    const fmtTs = (ts?: number): string => (typeof ts === 'number' ? new Date(ts).toISOString() : '');
    const durMs = (s?: number, e?: number): string => (typeof s === 'number' && typeof e === 'number' ? `${String(e - s)}ms` : '');

    lines.push(`Session ${snap.sessionId}`);
    const metaBits: string[] = [];
    if (typeof snap.agentId === 'string' && snap.agentId.length > 0) metaBits.push(`agentId=${snap.agentId}`);
    if (typeof snap.callPath === 'string' && snap.callPath.length > 0) metaBits.push(`callPath=${snap.callPath}`);
    if (typeof snap.success === 'boolean') metaBits.push(`success=${String(snap.success)}`);
    if (typeof snap.error === 'string' && snap.error.length > 0) metaBits.push(`error=${snap.error}`);
    if (typeof snap.startedAt === 'number') metaBits.push(`startedAt=${fmtTs(snap.startedAt)}`);
    if (typeof snap.endedAt === 'number') metaBits.push(`endedAt=${fmtTs(snap.endedAt)}`);
    if (typeof snap.startedAt === 'number' && typeof snap.endedAt === 'number') metaBits.push(`duration=${durMs(snap.startedAt, snap.endedAt)}`);
    if (metaBits.length > 0) lines.push(`├─ Meta: ${metaBits.join(' ')}`);

    // Counters
    const c = snap.counters;
    lines.push('├─ Counters');
    lines.push(`│  ├─ logs=${String(c.logs)} accounting=${String(c.accounting)} errors=${String(c.errors)}`);
    lines.push(`│  ├─ llmRequests=${String(c.llmRequests)} llmResponses=${String(c.llmResponses)}`);
    lines.push(`│  └─ toolRequests=${String(c.toolRequests)} toolResponses=${String(c.toolResponses)}`);

    // Spans
    const spans = Array.from(this.spans.values()).sort((a, b) => a.startedAt - b.startedAt);
    lines.push(`├─ Spans (${String(spans.length)})`);
    spans.forEach((s, idx) => {
      const isLast = idx === spans.length - 1;
      const pfx = isLast ? '│  └─' : '│  ├─';
      const dur = typeof s.endedAt === 'number' ? `${String(s.endedAt - s.startedAt)}ms` : '';
      lines.push(`${pfx} [${s.id}] ${s.kind}:${s.name} start=${fmtTs(s.startedAt)}${(typeof s.endedAt === 'number') ? ` end=${fmtTs(s.endedAt)} dur=${dur}` : ''}`);
      if (s.attributes !== undefined && Object.keys(s.attributes).length > 0) {
        const apfx = isLast ? '│     ' : '│     ';
        lines.push(`${apfx}attributes: ${JSON.stringify(s.attributes)}`);
      }
    });

    // Logs
    const logs = this.getLogs();
    lines.push(`├─ Logs (${String(logs.length)})`);
    logs.forEach((l, idx) => {
      const isLast = idx === logs.length - 1;
      const pfx = isLast ? '│  └─' : '│  ├─';
      const sev = l.severity;
      const typ = l.type;
      const dir = l.direction;
      const rid = (typeof l.remoteIdentifier === 'string') ? l.remoteIdentifier : '';
      const txt = l.message;
      lines.push(`${pfx} [${fmtTs(l.timestamp)}] ${sev} ${typ}/${dir} ${rid} :: ${txt}`);
    });

    // Accounting
    const acc = this.getAccounting();
    lines.push(`└─ Accounting (${String(acc.length)})`);
    acc.forEach((a, idx) => {
      const isLast = idx === acc.length - 1;
      const pfx = isLast ? '   └─' : '   ├─';
      const common = `ts=${fmtTs(a.timestamp)} status=${a.status} latency=${String(a.latency)}ms`;
      if (a.type === 'llm') {
        const route = ((): string => {
          const ap = (typeof a.actualProvider === 'string' && a.actualProvider.length > 0) ? `/${a.actualProvider}` : '';
          const am = (typeof a.actualModel === 'string' && a.actualModel.length > 0) ? `:${a.actualModel}` : '';
          return `${a.provider}${ap}:${a.model}${am}`;
        })();
        const cin = a.tokens.inputTokens;
        const cout = a.tokens.outputTokens;
        const ccached = (typeof a.tokens.cachedTokens === 'number') ? a.tokens.cachedTokens
          : (typeof a.tokens.cacheReadInputTokens === 'number' ? a.tokens.cacheReadInputTokens : 0);
        const tokens = `tokens(i=${String(cin)} o=${String(cout)} c=${String(ccached)})`;
        const cost = (typeof a.costUsd === 'number') ? ` cost=$${a.costUsd.toFixed(4)}` : '';
        const errText = (typeof a.error === 'string' && a.error.length > 0) ? ` error=${a.error}` : '';
        lines.push(`${pfx} LLM ${route} ${common} ${tokens}${cost}${errText}`);
      } else {
        const errText = (typeof a.error === 'string' && a.error.length > 0) ? ` error=${a.error}` : '';
        lines.push(`${pfx} TOOL ${a.mcpServer}:${a.command} ${common} in=${String(a.charactersIn)} out=${String(a.charactersOut)}${errText}`);
      }
    });

    return lines.join('\n');
  }
}
