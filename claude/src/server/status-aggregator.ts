import type { AccountingEntry, LogEntry } from '../types.js';

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
}

const arrowize = (s: string | undefined): string | undefined => s?.replace(/->/g, '→');

export function buildSnapshot(logs: LogEntry[], accounting: AccountingEntry[], nowTs: number): SnapshotSummary {
  interface TurnState { planned?: number; executed?: number; lastType?: 'llm'|'tool'; firstTs?: number; finished?: boolean }
  const perAgentTurns = new Map<string, Map<number, TurnState>>();
  const agentMeta = new Map<string, { callPath?: string; maxTurns?: number; origin?: string }>();

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
    agentMeta.set(agentId, meta);
  }

  const lines: AgentStatusLine[] = [];
  for (const [agentId, tmap] of perAgentTurns.entries()) {
    const latestTurn = Math.max(...Array.from(tmap.keys()));
    const st = tmap.get(latestTurn) ?? {};
    const meta = agentMeta.get(agentId) ?? {};
    const l: AgentStatusLine = { agentId, callPath: meta.callPath, maxTurns: meta.maxTurns, turn: latestTurn, status: 'Thinking' };
    if (typeof st.firstTs === 'number') l.elapsedSec = Math.max(0, Math.round((nowTs - st.firstTs) / 1000));
    const planned = st.planned ?? 0;
    const executed = st.executed ?? 0;
    if (planned > 0) {
      l.maxSubturns = planned;
      l.subturn = executed;
      l.progressPct = Math.max(0, Math.min(100, Math.round((executed / planned) * 100)));
      l.status = executed < planned ? 'Working' : (st.lastType === 'tool' ? 'Working' : 'Thinking');
    } else {
      l.status = st.lastType === 'tool' ? 'Working' : 'Thinking';
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
    lines,
    totals: { tokensIn, tokensOut, tokensCacheRead, tokensCacheWrite, toolsRun, costUsd: costUsd > 0 ? Number(costUsd.toFixed(4)) : undefined }
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
export function buildStatusBlocks(summary: SnapshotSummary, _masterAgentId?: string): any[] {
  const fmtK = (n: number): string => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n));
  const depthOf = (cp?: string): number => cp ? (cp.split('→').length - 1) : 0;
  const sorted = [...summary.lines]
    .filter((l) => l.status !== 'Finished')
    .sort((a, b) => depthOf(a.callPath) - depthOf(b.callPath) || a.agentId.localeCompare(b.agentId));
  if (sorted.length === 0) return [];

  const blocks: any[] = [];
  blocks.push({ type: 'header', text: { type: 'plain_text', text: 'Agent Status' } });
  blocks.push({ type: 'divider' });

  const mkProgress = (pct?: number): string => {
    if (typeof pct !== 'number') return '';
    const totalBars = 10;
    const filled = Math.max(0, Math.min(totalBars, Math.round((pct / 100) * totalBars)));
    return `${'█'.repeat(filled)}${'░'.repeat(totalBars - filled)} ${String(pct)}%`;
  };

  for (const l of sorted) {
    const d = depthOf(l.callPath);
    const indent = d > 0 ? '  '.repeat(d) : '';
    const emoji = l.status === 'Working' ? '🛠️' : '🤔';
    const bits: string[] = [];
    bits.push(l.status === 'Working' ? '*Working…*' : '*Thinking…*');
    if (typeof l.turn === 'number' && typeof l.maxTurns === 'number') bits.push(`${l.turn}/${l.maxTurns}`);
    if (typeof l.subturn === 'number' && typeof l.maxSubturns === 'number' && l.maxSubturns > 0) bits.push(`tools ${l.subturn}/${l.maxSubturns}`);
    if (typeof l.elapsedSec === 'number' && l.elapsedSec > 0) bits.push(`(${String(l.elapsedSec)}s)`);
    const progress = mkProgress(l.progressPct);
    const line = `${indent}${emoji} *${l.agentId}:* ${bits.join(', ')}${progress ? `\n${indent}${progress}` : ''}${l.callPath ? `\n${indent}${l.callPath}` : ''}`;
    blocks.push({ type: 'section', text: { type: 'mrkdwn', text: line } });
  }

  blocks.push({ type: 'divider' });
  const agentsCount2 = summary.lines.filter((l) => l.status !== 'Finished').length;
  const footer = `tokens →:${fmtK(summary.totals.tokensIn)} ←:${fmtK(summary.totals.tokensOut)} c→:${fmtK(summary.totals.tokensCacheRead)} c←:${fmtK(summary.totals.tokensCacheWrite)} | cost: $${(summary.totals.costUsd ?? 0).toFixed(2)} | tools ${summary.totals.toolsRun} | agents ${agentsCount2}`;
  blocks.push({ type: 'context', elements: [ { type: 'mrkdwn', text: footer } ] });
  return blocks;
}
