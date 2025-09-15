import type { AccountingEntry, LogEntry } from './types.js';

import { warn } from './utils.js';

type OperationKind = 'llm' | 'tool' | 'session';

interface OperationNode {
  opId: string;
  kind: OperationKind;
  startedAt: number;
  endedAt?: number;
  status?: 'ok' | 'failed';
  attributes?: Record<string, unknown>;
  logs: LogEntry[];
  accounting: AccountingEntry[];
  // For kind === 'session', we embed a full child session tree
  childSession?: SessionNode;
  // Structured content
  reasoning?: { chunks: { text: string; ts: number }[]; final?: string };
  request?: { kind: 'llm'|'tool'; payload: unknown; size?: number };
  response?: { payload: unknown; size?: number; truncated?: boolean };
}

interface TurnNode {
  id: string;
  index: number; // 1-based
  startedAt: number;
  endedAt?: number;
  attributes?: Record<string, unknown>;
  ops: OperationNode[];
}

export interface SessionNode {
  id: string;
  traceId?: string;
  agentId?: string;
  callPath?: string;
  startedAt: number;
  endedAt?: number;
  success?: boolean;
  error?: string;
  attributes?: Record<string, unknown>;
  turns: TurnNode[];
}

function uid(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export class SessionTreeBuilder {
  private readonly session: SessionNode;
  private readonly turnIndex = new Map<number, TurnNode>();
  private readonly opIndex = new Map<string, OperationNode>();

  constructor(meta?: { traceId?: string; agentId?: string; callPath?: string; attributes?: Record<string, unknown> }) {
    this.session = {
      id: uid(),
      traceId: meta?.traceId,
      agentId: meta?.agentId,
      callPath: meta?.callPath,
      startedAt: Date.now(),
      attributes: meta?.attributes,
      turns: [],
    };
  }

  getSession(): SessionNode { return this.session; }

  endSession(success: boolean, error?: string): void {
    this.session.endedAt = Date.now();
    this.session.success = success;
    if (typeof error === 'string' && error.length > 0) this.session.error = error;
  }

  beginTurn(index: number, attributes?: Record<string, unknown>): string {
    const id = uid();
    const node: TurnNode = { id, index, startedAt: Date.now(), attributes, ops: [] };
    this.session.turns.push(node);
    this.turnIndex.set(index, node);
    return id;
  }

  endTurn(index: number, attributes?: Record<string, unknown>): void {
    const t = this.turnIndex.get(index);
    if (t !== undefined) {
      t.endedAt = Date.now();
      if (attributes !== undefined) t.attributes = { ...(t.attributes ?? {}), ...attributes };
    }
  }

  beginOp(turnIndex: number, kind: OperationKind, attributes?: Record<string, unknown>): string {
    const t = this.turnIndex.get(turnIndex);
    const id = uid();
    const node: OperationNode = { opId: id, kind, startedAt: Date.now(), attributes, logs: [], accounting: [] };
    if (t !== undefined) t.ops.push(node);
    this.opIndex.set(id, node);
    return id;
  }

  attachChildSession(opId: string, child: SessionNode): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) op.childSession = child;
  }

  appendLog(opId: string, log: LogEntry): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) {
      // Attach stable path label if missing
      try {
        if ((log as { path?: string }).path === undefined) {
          (log as { path?: string }).path = this.getOpPath(opId);
        }
      } catch (e) { warn(`session-tree: failed to compute path for op ${opId}: ${e instanceof Error ? e.message : String(e)}`); }
      op.logs.push(log);
    }
  }

  appendAccounting(opId: string, acc: AccountingEntry): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) op.accounting.push(acc);
  }

  // Compute a stable, bijective path label (e.g., 1.2 or 1.2.1.1) for a given opId
  getOpPath(opId: string): string {
    // const parts: string[] = [];
    const walk = (node: SessionNode, prefix: string[]): string | undefined => {
      // eslint-disable-next-line functional/no-loop-statements -- iterative traversal is clearer here
      for (const t of node.turns) {
        const base = [...prefix, String(t.index)];
        // eslint-disable-next-line functional/no-loop-statements -- index needed for stable path labels
        for (let i = 0; i < t.ops.length; i++) {
          const o = t.ops[i];
          const opIdx = String(i + 1);
          const label = [...base, opIdx];
          if (o.opId === opId) return label.join('.');
          if (o.kind === 'session' && o.childSession !== undefined) {
            const child = walk(o.childSession, label);
            if (child !== undefined) return child;
          }
        }
      }
      return undefined;
    };
    return walk(this.session, []) ?? '';
  }

  appendReasoningChunk(opId: string, text: string): void {
    const op = this.opIndex.get(opId);
    if (op === undefined) return;
    const r = op.reasoning ?? { chunks: [] as { text: string; ts: number }[] };
    r.chunks.push({ text, ts: Date.now() });
    op.reasoning = r;
  }

  setReasoningFinal(opId: string, text: string): void {
    const op = this.opIndex.get(opId);
    if (op === undefined) return;
    const r = op.reasoning ?? { chunks: [] as { text: string; ts: number }[] };
    r.final = text;
    op.reasoning = r;
  }

  setRequest(opId: string, req: { kind: 'llm'|'tool'; payload: unknown; size?: number }): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) op.request = req;
  }

  setResponse(opId: string, res: { payload: unknown; size?: number; truncated?: boolean }): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) op.response = res;
  }

  // Flatten all logs and accounting from the tree in timestamp order for legacy consumers
  flatten(): { logs: LogEntry[]; accounting: AccountingEntry[] } {
    const logs: LogEntry[] = [];
    const acc: AccountingEntry[] = [];
    const visit = (node: SessionNode): void => {
      node.turns.forEach((t) => {
        t.ops.forEach((o) => {
          logs.push(...o.logs);
          acc.push(...o.accounting);
          if (o.kind === 'session' && o.childSession !== undefined) visit(o.childSession);
        });
      });
    };
    visit(this.session);
    logs.sort((a, b) => a.timestamp - b.timestamp);
    acc.sort((a, b) => a.timestamp - b.timestamp);
    return { logs, accounting: acc };
  }

  endOp(opId: string, status: 'ok' | 'failed', attributes?: Record<string, unknown>): void {
    const op = this.opIndex.get(opId);
    if (op !== undefined) {
      op.endedAt = Date.now();
      op.status = status;
      if (attributes !== undefined) op.attributes = { ...(op.attributes ?? {}), ...attributes };
    }
  }

  // Simple ASCII renderer
  renderAscii(): string {
    const s = this.session;
    const lines: string[] = [];
    const fmtTs = (ts?: number): string => (typeof ts === 'number' ? new Date(ts).toISOString() : '');
    const dur = (a?: number, b?: number): string => (typeof a === 'number' && typeof b === 'number') ? `${String(b - a)}ms` : '';

    lines.push(`SessionTree ${s.id}${(typeof s.traceId === 'string' && s.traceId.length > 0) ? ` trace=${s.traceId}` : ''}${(typeof s.agentId === 'string' && s.agentId.length > 0) ? ` agent=${s.agentId}` : ''}${(typeof s.callPath === 'string' && s.callPath.length > 0) ? ` callPath=${s.callPath}` : ''}`);
    lines.push(`├─ started=${fmtTs(s.startedAt)}${(typeof s.endedAt === 'number') ? ` ended=${fmtTs(s.endedAt)} dur=${dur(s.startedAt, s.endedAt)}` : ''}${(typeof s.success === 'boolean') ? ` success=${String(s.success)}` : ''}${(typeof s.error === 'string' && s.error.length > 0) ? ` error=${s.error}` : ''}`);
    lines.push(`├─ turns=${String(s.turns.length)}`);
    s.turns.forEach((t, ti) => {
      const tLast = ti === s.turns.length - 1;
      const tp = tLast ? '└─' : '├─';
      lines.push(`${tp} Turn#${String(t.index)} ${fmtTs(t.startedAt)}${(typeof t.endedAt === 'number') ? ` → ${fmtTs(t.endedAt)} (${dur(t.startedAt, t.endedAt)})` : ''}`);
      const ops = t.ops;
      ops.forEach((o, oi) => {
        const oLast = oi === ops.length - 1;
        const opfx = tLast ? (oLast ? '   └─' : '   ├─') : (oLast ? '│  └─' : '│  ├─');
        const adur = dur(o.startedAt, o.endedAt);
        lines.push(`${opfx} ${o.kind.toUpperCase()} op=${o.opId} ${fmtTs(o.startedAt)}${(typeof o.endedAt === 'number') ? ` → ${fmtTs(o.endedAt)} (${adur})` : ''}${(typeof o.status === 'string') ? ` status=${o.status}` : ''}`);
        const lpfx = tLast ? '      ' : '│     ';
        lines.push(`${lpfx}logs=${String(o.logs.length)} accounting=${String(o.accounting.length)}`);
        if (o.childSession !== undefined) {
          lines.push(`${lpfx}child:`);
          // indent child tree
          const childLines = SessionTreeBuilder.indentAscii(this.renderChild(o.childSession)).split('\n');
          childLines.forEach((ln) => { if (ln.length > 0) lines.push(`${lpfx}${ln}`); });
        }
      });
    });
    return lines.join('\n');
  }

  // Render a child session using a temporary builder-like function
  private renderChild(child: SessionNode): string {
    // Minimal duplication: construct a fake builder around provided child
    const lines: string[] = [];
    const fmtTs = (ts?: number): string => (typeof ts === 'number' ? new Date(ts).toISOString() : '');
    const dur = (a?: number, b?: number): string => (typeof a === 'number' && typeof b === 'number') ? `${String(b - a)}ms` : '';
    lines.push(`SessionTree ${child.id}${(typeof child.traceId === 'string' && child.traceId.length > 0) ? ` trace=${child.traceId}` : ''}${(typeof child.agentId === 'string' && child.agentId.length > 0) ? ` agent=${child.agentId}` : ''}${(typeof child.callPath === 'string' && child.callPath.length > 0) ? ` callPath=${child.callPath}` : ''}`);
    lines.push(`├─ started=${fmtTs(child.startedAt)}${(typeof child.endedAt === 'number') ? ` ended=${fmtTs(child.endedAt)} dur=${dur(child.startedAt, child.endedAt)}` : ''}${(typeof child.success === 'boolean') ? ` success=${String(child.success)}` : ''}${(typeof child.error === 'string' && child.error.length > 0) ? ` error=${child.error}` : ''}`);
    lines.push(`├─ turns=${String(child.turns.length)}`);
    child.turns.forEach((t, ti) => {
      const tLast = ti === child.turns.length - 1;
      const tp = tLast ? '└─' : '├─';
      lines.push(`${tp} Turn#${String(t.index)} ${fmtTs(t.startedAt)}${(typeof t.endedAt === 'number') ? ` → ${fmtTs(t.endedAt)} (${dur(t.startedAt, t.endedAt)})` : ''}`);
      const ops = t.ops;
      ops.forEach((o, oi) => {
        const oLast = oi === ops.length - 1;
        const opfx = tLast ? (oLast ? '   └─' : '   ├─') : (oLast ? '│  └─' : '│  ├─');
        const adur = dur(o.startedAt, o.endedAt);
        lines.push(`${opfx} ${o.kind.toUpperCase()} op=${o.opId} ${fmtTs(o.startedAt)}${(typeof o.endedAt === 'number') ? ` → ${fmtTs(o.endedAt)} (${adur})` : ''}${(typeof o.status === 'string') ? ` status=${o.status}` : ''}`);
        const lpfx = tLast ? '      ' : '│     ';
        lines.push(`${lpfx}logs=${String(o.logs.length)} accounting=${String(o.accounting.length)}`);
      });
    });
    return lines.join('\n');
  }

  // Helper to indent a multi-line ASCII block
  static indentAscii(block: string): string {
    return block.split('\n').map((l) => `   ${l}`).join('\n');
  }
}
