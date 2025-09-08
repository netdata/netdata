import type { AccountingEntry, LogEntry } from '../types.js';
import type { SessionNode } from '../session-tree.js';

export interface AgentStatusLine {
  agentId: string;
  callPath?: string;
  status: 'Working' | 'Thinking' | 'Finished' | 'Failed';
  turn?: number;
  maxTurns?: number;
  subturn?: number;
  maxSubturns?: number;
  elapsedSec?: number;
  progressPct?: number;
}

export interface SnapshotSummary {
  lines: AgentStatusLine[];
  totals: {
    tokensIn: number;
    tokensOut: number;
    tokensCacheRead: number;
    tokensCacheWrite: number;
    toolsRun: number;
    costUsd?: number;
  };
  // Optional overall timing when available (from opTree)
  runStartedAt?: number;
  runElapsedSec?: number;
}

const arrowize = (s: string | undefined): string | undefined => s?.replace(/->/g, '→');

export function buildSnapshot(logs: LogEntry[], accounting: AccountingEntry[], nowTs: number): SnapshotSummary {
  interface TurnState { planned?: number; executed?: number; lastType?: 'llm'|'tool'; firstTs?: number; finished?: boolean }
  const perAgentTurns = new Map<string, Map<number, TurnState>>();
  const agentMeta = new Map<string, { callPath?: string; maxTurns?: number; origin?: string; firstSeenTs?: number; toolCount?: number }>();

  for (const le of logs) {
    const agentId = le.agentId ?? 'agent';
    const turn = typeof le.turn === 'number' ? le.turn : 0;
    const turnMap = perAgentTurns.get(agentId) ?? new Map<number, TurnState>();
    const st = turnMap.get(turn) ?? {};
    st.firstTs = st.firstTs === undefined ? le.timestamp : Math.min(st.firstTs, le.timestamp);
    if (le.severity === 'FIN') st.finished = true;
    if (le.type === 'tool') st.lastType = 'tool';
    else if (le.type === 'llm' && st.lastType !== 'tool') st.lastType = 'llm';
    if (le.type === 'tool' && typeof le.subturn === 'number' && le.subturn > 0) {
      st.executed = Math.max(st.executed ?? 0, le.subturn);
    }
    const maxSubsVal = (le as unknown as { max_subturns?: number }).max_subturns;
    if (typeof maxSubsVal === 'number') st.planned = Math.max(st.planned ?? 0, maxSubsVal);
    turnMap.set(turn, st);
    perAgentTurns.set(agentId, turnMap);
    // meta
    const meta = agentMeta.get(agentId) ?? {};
    if (typeof le.callPath === 'string' && le.callPath.length > 0) meta.callPath = arrowize(le.callPath);
    const maxTurnsVal = (le as unknown as { max_turns?: number }).max_turns;
    if (typeof maxTurnsVal === 'number') meta.maxTurns = maxTurnsVal;
    if (typeof le.originTxnId === 'string') meta.origin = le.originTxnId;
    // Track earliest timestamp per agent for stable elapsed time
    meta.firstSeenTs = (typeof meta.firstSeenTs === 'number') ? Math.min(meta.firstSeenTs, le.timestamp) : le.timestamp;
    // Count tool logs per agent
    if (le.type === 'tool') meta.toolCount = (meta.toolCount ?? 0) + 1;
    agentMeta.set(agentId, meta);
  }

  const lines: AgentStatusLine[] = [];
  for (const [agentId, tmap] of perAgentTurns.entries()) {
    const latestTurn = Math.max(...Array.from(tmap.keys()));
    const st = tmap.get(latestTurn) ?? {};
    const meta = agentMeta.get(agentId) ?? {};
    const l: AgentStatusLine = { agentId, callPath: meta.callPath, maxTurns: meta.maxTurns, turn: latestTurn, status: 'Thinking' };
    const stableStart = typeof meta.firstSeenTs === 'number' ? meta.firstSeenTs : st.firstTs;
    if (typeof stableStart === 'number') l.elapsedSec = Math.max(0, Math.round((nowTs - stableStart) / 1000));
    const planned = st.planned ?? 0;
    const executed = st.executed ?? 0;
    if (planned > 0) {
      l.maxSubturns = planned;
      l.subturn = executed;
      l.progressPct = Math.max(0, Math.min(100, Math.round((executed / planned) * 100)));
      l.status = executed < planned ? 'Working' : (st.lastType === 'tool' ? 'Working' : 'Thinking');
    } else {
      // Without planned toolcalls, consider having emitted any tool logs as Working
      const hasTools = (meta.toolCount ?? 0) > 0;
      l.status = (st.lastType === 'tool' || hasTools) ? 'Working' : 'Thinking';
    }
    if (st.finished === true) l.status = 'Finished';
    lines.push(l);
  }

  // Aggregate master status from children by origin
  const byOrigin = new Map<string, AgentStatusLine[]>();
  for (const [agentId, meta] of agentMeta.entries()) {
    const origin = meta.origin ?? 'root';
    const line = lines.find((x) => x.agentId === agentId);
    if (!line) continue;
    const arr = byOrigin.get(origin) ?? [];
    arr.push(line);
    byOrigin.set(origin, arr);
  }
  for (const arr of byOrigin.values()) {
    const depth = (cp?: string) => (cp ? cp.split('→').length - 1 : 0);
    const master = arr.slice().sort((a, b) => depth(a.callPath) - depth(b.callPath))[0];
    if (!master) continue;
    const anyWorkingChild = arr.some((l) => l !== master && l.status === 'Working');
    if (anyWorkingChild) master.status = 'Working';
  }

  // Deduplicate per origin+agent: keep the shallowest callPath (master), drop duplicates
  const deduped: AgentStatusLine[] = [];
  const seen = new Set<string>();
  const depthOf = (cp?: string) => (cp ? cp.split('→').length - 1 : 0);
  for (const l of lines.sort((a, b) => depthOf(a.callPath) - depthOf(b.callPath))) {
    const origin = (agentMeta.get(l.agentId ?? '')?.origin) ?? 'root';
    const key = `${origin}::${l.agentId}`;
    if (seen.has(key)) continue;
    seen.add(key);
    deduped.push(l);
  }

  // Totals
  let tokensIn = 0; let tokensOut = 0; let tokensCacheRead = 0; let tokensCacheWrite = 0; let toolsRun = 0; let costUsd = 0;
  for (const a of accounting) {
    if (a.type === 'llm') {
      tokensIn += a.tokens.inputTokens;
      tokensOut += a.tokens.outputTokens;
      tokensCacheRead += a.tokens.cacheReadInputTokens ?? a.tokens.cachedTokens ?? 0;
      tokensCacheWrite += a.tokens.cacheWriteInputTokens ?? 0;
      if (typeof a.costUsd === 'number') costUsd += a.costUsd;
    } else if (a.type === 'tool') {
      toolsRun += 1;
    }
  }

  return {
    lines: deduped,
    totals: { tokensIn, tokensOut, tokensCacheRead, tokensCacheWrite, toolsRun, costUsd: costUsd > 0 ? Number(costUsd.toFixed(4)) : undefined }
  };
}

// Build SnapshotSummary from the hierarchical operation tree (Option C)
export function buildSnapshotFromOpTree(root: SessionNode, nowTs: number): SnapshotSummary {
  const lines: AgentStatusLine[] = [];
  const tokens = { in: 0, out: 0, read: 0, write: 0 };
  let toolsRun = 0;
  let costUsd = 0;

  const visit = (node: SessionNode): void => {
    const agentId = node.agentId ?? 'agent';
    const callPath = node.callPath;
    const started = node.startedAt;
    const ended = node.endedAt;
    const elapsedSec = Math.max(0, Math.round(((typeof ended === 'number' ? ended : nowTs) - started) / 1000));
    const status: AgentStatusLine['status'] = (typeof ended === 'number') ? 'Finished' : 'Working';
    lines.push({ agentId, callPath, status, elapsedSec });

    // Walk turns/ops, aggregate tools + tokens
    const turns = Array.isArray(node.turns) ? node.turns : [];
    for (const t of turns) {
      const ops = Array.isArray(t.ops) ? t.ops : [];
      for (const o of ops) {
        if (o.kind === 'tool') toolsRun += 1;
        const acc = Array.isArray(o.accounting) ? o.accounting : [];
        for (const a of acc) {
          const typ = (a as unknown as { type?: string }).type;
          if (typ === 'llm') {
            const tk = (a as unknown as { tokens?: { inputTokens?: number; outputTokens?: number; cacheReadInputTokens?: number; cacheWriteInputTokens?: number; cachedTokens?: number } }).tokens ?? {};
            tokens.in += tk.inputTokens ?? 0;
            tokens.out += tk.outputTokens ?? 0;
            tokens.read += tk.cacheReadInputTokens ?? tk.cachedTokens ?? 0;
            tokens.write += tk.cacheWriteInputTokens ?? 0;
            const c = (a as unknown as { costUsd?: number }).costUsd;
            if (typeof c === 'number') costUsd += c;
          }
        }
        if (o.kind === 'session' && o.childSession) visit(o.childSession);
      }
    }
  };
  visit(root);

  return {
    lines,
    totals: { tokensIn: tokens.in, tokensOut: tokens.out, tokensCacheRead: tokens.read, tokensCacheWrite: tokens.write, toolsRun, costUsd: costUsd > 0 ? Number(costUsd.toFixed(4)) : undefined },
    runStartedAt: root.startedAt,
    runElapsedSec: Math.max(0, Math.round((nowTs - root.startedAt) / 1000))
  };
}

export function formatSlackStatus(summary: SnapshotSummary, masterAgentId?: string): string {
  const fmtK = (n: number): string => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n));
  const depthOf = (cp?: string): number => cp ? (cp.split('→').length - 1) : 0;
  // Sort lines by depth, then by agentId for stability
  const sorted = [...summary.lines].sort((a, b) => depthOf(a.callPath) - depthOf(b.callPath) || a.agentId.localeCompare(b.agentId));
  const master = masterAgentId ? sorted.find((l) => l.agentId === masterAgentId) : sorted[0];
  const others = sorted.filter((l) => l !== master);
  const parts: string[] = [];

  const fmtLine = (l: AgentStatusLine): string => {
    const bits: string[] = [];
    if (l.status === 'Finished') return ''; // do not show finished
    bits.push(l.status === 'Working' ? 'Working...' : 'Thinking...');
    if (typeof l.turn === 'number' && typeof l.maxTurns === 'number') bits.push(`${l.turn}/${l.maxTurns}`);
    if (typeof l.subturn === 'number' && typeof l.maxSubturns === 'number' && l.maxSubturns > 0) bits.push(`tools ${l.subturn}/${l.maxSubturns}`);
    if (typeof l.elapsedSec === 'number' && l.elapsedSec > 0) bits.push(`(${String(l.elapsedSec)}s)`);
    const core = bits.join(', ');
    const d = depthOf(l.callPath);
    const indent = d > 0 ? '  '.repeat(d) : '';
    const namePrefix = `${l.agentId}: `; // always show agent name, including master
    const path = l.callPath ? `  ${l.callPath}` : '';
    return `${indent}${namePrefix}${core}${path}`.trim();
  };

  if (master) {
    const top = fmtLine(master);
    if (top.length > 0) parts.push(top);
  }
  others.forEach((l) => { const s = fmtLine(l); if (s.length > 0) parts.push(s); });

  const tokenLine = `tokens →:${fmtK(summary.totals.tokensIn)} ←:${fmtK(summary.totals.tokensOut)} c→:${fmtK(summary.totals.tokensCacheRead)} c←:${fmtK(summary.totals.tokensCacheWrite)} | cost: $${(summary.totals.costUsd ?? 0).toFixed(2)} | tools ${summary.totals.toolsRun} | agents ${sorted.length}`;
  parts.push('', tokenLine);

  return parts.join('\n');
}

// Build a richer Block Kit version for Slack with emojis and progress bars
export function buildStatusBlocks(summary: SnapshotSummary, rootAgentId?: string, runStartedAt?: number): any[] {
  const fmtK = (n: number): string => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n));
  const blocks: any[] = [];

  // Compute overall elapsed: prefer provided start, else max elapsed among lines
  const elapsed = ((): number => {
    if (typeof runStartedAt === 'number') {
      const now = Date.now();
      return Math.max(0, Math.round((now - runStartedAt) / 1000));
    }
    const arr = summary.lines.map((l) => l.elapsedSec ?? 0);
    return arr.length > 0 ? Math.max(...arr) : 0;
  })();

  // Title
  const title = `Working... (${String(elapsed)}s)`;
  blocks.push({ type: 'header', text: { type: 'plain_text', text: title } });
  blocks.push({ type: 'divider' });

  // Build running agents list (exclude finished and root master)
  const running = summary.lines.filter((l) => l.status !== 'Finished');
  const prefix = (typeof rootAgentId === 'string' && rootAgentId.length > 0) ? `${rootAgentId}->` : undefined;
  // Oldest first ⇒ larger elapsedSec first
  const ordered = running.sort((a, b) => (b.elapsedSec ?? 0) - (a.elapsedSec ?? 0));
  const items = ordered
    .map((l) => {
      const cp = typeof l.callPath === 'string' && l.callPath.length > 0 ? l.callPath : l.agentId;
      if (typeof rootAgentId === 'string' && l.agentId === rootAgentId) return undefined; // drop master
      const shown = (prefix && cp.startsWith(prefix)) ? cp.slice(prefix.length) : cp;
      const secs = l.elapsedSec ?? 0;
      return `- ${shown} (${String(secs)}s)`;
    })
    .filter((v): v is string => typeof v === 'string' && v.length > 0);

  // Split into multiple sections to avoid Slack collapsing large sections ("Show more")
  if (items.length === 0) {
    blocks.push({ type: 'section', text: { type: 'mrkdwn', text: '- idle' } });
  } else {
    const CHUNK = 4; // keep sections small to avoid collapse
    for (let i = 0; i < items.length; i += CHUNK) {
      // eslint-disable-next-line functional/no-loop-statements
      const chunk = items.slice(i, i + CHUNK).join('\n');
      blocks.push({ type: 'section', text: { type: 'mrkdwn', text: chunk } });
    }
  }

  blocks.push({ type: 'divider' });

  // Footer stats (smallest font)
  const agentsCount = summary.lines.length;
  const footer = `tokens →:${fmtK(summary.totals.tokensIn)} ←:${fmtK(summary.totals.tokensOut)} c→:${fmtK(summary.totals.tokensCacheRead)} c←:${fmtK(summary.totals.tokensCacheWrite)} | cost: $${(summary.totals.costUsd ?? 0).toFixed(2)} | tools ${summary.totals.toolsRun} | agents ${agentsCount}`;
  blocks.push({ type: 'context', elements: [ { type: 'mrkdwn', text: footer } ] });
  return blocks;
}
