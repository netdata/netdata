import type { SessionNode } from '../session-tree.js';

interface AgentStatusLine {
  agentId: string;
  callPath?: string;
  status: 'Working' | 'Thinking' | 'Finished' | 'Failed';
  turn?: number;
  maxTurns?: number;
  subturn?: number;
  maxSubturns?: number;
  elapsedSec?: number;
  progressPct?: number;
  title?: string;
  latestStatus?: string;
}

interface SnapshotSummary {
  lines: AgentStatusLine[];
  totals: {
    tokensIn: number;
    tokensOut: number;
    tokensCacheRead: number;
    tokensCacheWrite: number;
    toolsRun: number;
    costUsd?: number;
    agentsRun: number;
  };
  // Optional overall timing when available (from opTree)
  runStartedAt?: number;
  runElapsedSec?: number;
  originTxnId?: string;
  sessionCount: number;
}

const formatThousands = (n: number): string => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n));

const formatClock = (elapsedSec: number): string => {
  const mm = Math.floor(elapsedSec / 60);
  const ss = elapsedSec % 60;
  return `${String(mm).padStart(2, '0')}:${String(ss).padStart(2, '0')}`;
};

interface FooterFormat {
  lines: [string, string];
  text: string;
}

interface FooterOptions {
  elapsedSec?: number;
  originTxnId?: string;
  agentsCount?: number;
  toolsCount?: number;
}

export function formatFooterLines(summary: SnapshotSummary, options?: FooterOptions): FooterFormat {
  const elapsedSec = options?.elapsedSec ?? summary.runElapsedSec ?? 0;
  const clock = formatClock(Math.max(0, elapsedSec));
  const costVal = summary.totals.costUsd ?? 0;
  const costStr = `$${costVal.toFixed(2)}`;
  const origin = options?.originTxnId ?? summary.originTxnId ?? 'unknown-txn';
  const agentsCount = typeof options?.agentsCount === 'number'
    ? options.agentsCount
    : (summary.totals.agentsRun ?? summary.sessionCount);
  const toolsCount = typeof options?.toolsCount === 'number' ? options.toolsCount : summary.totals.toolsRun;
  const line1 = `*${clock}* | cost: *${costStr}* | ${origin}`;
  const line2 = `agents ${agentsCount} | tools ${toolsCount} | tokens →${formatThousands(summary.totals.tokensIn)} ←${formatThousands(summary.totals.tokensOut)} c→${formatThousands(summary.totals.tokensCacheRead)} c←${formatThousands(summary.totals.tokensCacheWrite)}`;
  const lines: [string, string] = [line1, line2];
  return { lines, text: lines.join('\n') };
}

// Build SnapshotSummary from the hierarchical operation tree (Option C)
export function buildSnapshotFromOpTree(root: SessionNode, nowTs: number): SnapshotSummary {
  const lines: AgentStatusLine[] = [];
  const tokens = { in: 0, out: 0, read: 0, write: 0 };
  let toolsRun = 0;
  let costUsd = 0;
  let sessionCount = 0;

  const visit = (node: SessionNode): void => {
    sessionCount += 1;
    const agentId = node.agentId ?? 'agent';
    const callPath = node.callPath;
    const started = node.startedAt;
    const ended = node.endedAt;
    const elapsedSec = Math.max(0, Math.round(((typeof ended === 'number' ? ended : nowTs) - started) / 1000));
    const status: AgentStatusLine['status'] = (typeof ended === 'number') ? 'Finished' : 'Working';
    const title = node.sessionTitle && node.sessionTitle.trim().length > 0 ? node.sessionTitle : undefined;
    const latestStatus = node.latestStatus && node.latestStatus.trim().length > 0 ? node.latestStatus : undefined;
    lines.push({ agentId, callPath, status, elapsedSec, title, latestStatus });

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

  const normalizedCost = Number(costUsd.toFixed(4));
  return {
    lines,
    totals: {
      tokensIn: tokens.in,
      tokensOut: tokens.out,
      tokensCacheRead: tokens.read,
      tokensCacheWrite: tokens.write,
      toolsRun,
      costUsd: Number.isFinite(normalizedCost) ? normalizedCost : undefined,
      agentsRun: sessionCount,
    },
    runStartedAt: root.startedAt,
    runElapsedSec: Math.max(0, Math.round((nowTs - root.startedAt) / 1000)),
    originTxnId: root.traceId ?? root.id,
    sessionCount
  };
}

export function formatSlackStatus(summary: SnapshotSummary, masterAgentId?: string): string {
  const depthOf = (cp?: string): number => cp ? (cp.split(':').length - 1) : 0;
  // Sort lines by depth, then by agentId for stability
  const sorted = [...summary.lines]
    .sort((a, b) => depthOf(a.callPath) - depthOf(b.callPath) || (a.agentId ?? '').localeCompare(b.agentId ?? ''));
  const seen = new Set<string>();
  const unique = sorted.filter((line) => {
    const key = line.callPath ?? line.agentId ?? '';
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  const master = masterAgentId ? unique.find((l) => l.agentId === masterAgentId) : unique[0];
  const others = unique.filter((l) => l !== master);
  const parts: string[] = [];

  const fmtLine = (l: AgentStatusLine): string => {
    const bits: string[] = [];
    if (l.status === 'Finished') return ''; // do not show finished
    bits.push(l.status === 'Working' ? 'Working...' : 'Thinking...');
    if (typeof l.turn === 'number' && typeof l.maxTurns === 'number') bits.push(`${l.turn}/${l.maxTurns}`);
    if (typeof l.subturn === 'number' && typeof l.maxSubturns === 'number' && l.maxSubturns > 0) bits.push(`tools ${l.subturn}/${l.maxSubturns}`);
    if (typeof l.elapsedSec === 'number' && l.elapsedSec > 0) bits.push(`(${String(l.elapsedSec)}s)`);
    const statusText = bits.join(', ');
    const latest = typeof l.latestStatus === 'string' && l.latestStatus.trim().length > 0 ? l.latestStatus.trim() : undefined;
    const details = latest !== undefined && latest.length > 0
      ? `${statusText} → ${latest}`
      : statusText;
    const d = depthOf(l.callPath);
    const indent = d > 0 ? '  '.repeat(d) : '';
    const namePrefix = `${l.agentId}: `; // always show agent name, including master
    const path = l.callPath ? `  ${l.callPath}` : '';
    return `${indent}${namePrefix}${details}${path}`.trim();
  };

  if (master) {
    const top = fmtLine(master);
    if (top.length > 0) parts.push(top);
  }
  others.forEach((l) => { const s = fmtLine(l); if (s.length > 0) parts.push(s); });

  const footer = formatFooterLines(summary);
  parts.push('', footer.lines[0], footer.lines[1]);

  return parts.join('\n');
}

// Build a richer Block Kit version for Slack with emojis and progress bars
export function buildStatusBlocks(summary: SnapshotSummary, rootAgentId?: string, runStartedAt?: number): any[] {
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

  // Title: For master agent, prefer latestStatus over title (since title is just "Working...")
  const master = typeof rootAgentId === 'string' ? summary.lines.find((l) => l.agentId === rootAgentId) : undefined;
  // For master agent: use latestStatus if available, otherwise title, otherwise "Working..."
  const headerTitle = (() => {
    if (master?.latestStatus && master.latestStatus.trim().length > 0) {
      return master.latestStatus.trim();
    }
    if (master?.title && master.title.trim().length > 0) {
      return master.title.trim();
    }
    return 'Working...';
  })();
  blocks.push({ type: 'header', text: { type: 'plain_text', text: headerTitle } });
  blocks.push({ type: 'divider' });

  
  // Build running agents list (exclude finished and root master)
  const running = summary.lines.filter((l) => l.status !== 'Finished');
  const prefix = (typeof rootAgentId === 'string' && rootAgentId.length > 0) ? `${rootAgentId}:` : undefined;
  const asClock = (n: number): string => {
    const mm = Math.floor(n / 60);
    const ss = n % 60;
    return `${String(mm).padStart(2, '0')}:${String(ss).padStart(2, '0')}`;
  };
  // Newest first ⇒ smaller elapsedSec first (dedup keeps the most recent entry)
  const ordered = running.sort((a, b) => (a.elapsedSec ?? 0) - (b.elapsedSec ?? 0));
  const seen = new Set<string>();
  const deduped = ordered.filter((line) => {
    const key = line.callPath ?? line.agentId ?? '';
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  const entries = deduped
    .flatMap((l) => {
      if (typeof rootAgentId === 'string' && l.agentId === rootAgentId) return []; // drop master
      const cpRaw = typeof l.callPath === 'string' && l.callPath.length > 0 ? l.callPath : l.agentId;
      const cpNorm = cpRaw.replace(/→/g, ':');
      const startsWithPrefix = typeof prefix === 'string' && (cpNorm.startsWith(prefix));
      const shownPath = startsWithPrefix ? cpNorm.slice(prefix.length) : cpNorm;
      // Show only the final segment (child agent name), strip 'agent__' prefix if present
      const segments = shownPath.split(':').filter((s) => s.length > 0);
      const last = segments.length > 0 ? segments[segments.length - 1] : shownPath;
      const shownName = last.startsWith('agent__') ? last.slice('agent__'.length) : last;
      const secs = l.elapsedSec ?? 0;
      const agentLine = `${shownName} — ${asClock(secs)}`;
      const title = typeof l.title === 'string' && l.title.length > 0 ? l.title : undefined;
      const latestStatus = typeof l.latestStatus === 'string' && l.latestStatus.length > 0 ? l.latestStatus : undefined;

      // Build context elements: latestStatus on first line, agent/duration on second line
      const contextElements: string[] = [];
      if (latestStatus) contextElements.push(latestStatus);
      contextElements.push(agentLine);
      const contextText = contextElements.join('\n');

      // If we have a title, show it as the main text with status and agent/duration as context
      // If no title, just show agent/duration as main text
      if (title) {
        return [
          { type: 'section', text: { type: 'mrkdwn', text: title } },
          { type: 'context', elements: [{ type: 'mrkdwn', text: contextText }] }
        ];
      } else {
        // No title, so if we have latestStatus show it with agent/duration as context
        if (latestStatus) {
          return [
            { type: 'section', text: { type: 'mrkdwn', text: latestStatus } },
            { type: 'context', elements: [{ type: 'mrkdwn', text: agentLine }] }
          ];
        } else {
          return [{ type: 'section', text: { type: 'mrkdwn', text: agentLine } }];
        }
      }
    })
    .filter((v): v is any => v !== undefined);

  if (entries.length === 0) {
    blocks.push({ type: 'section', text: { type: 'mrkdwn', text: 'Thinking... 🤔' } });
  } else {
    entries.forEach((b) => { blocks.push(b); });
  }

  blocks.push({ type: 'divider' });


  // Footer stats (smallest font)
  const footer = formatFooterLines(summary, { elapsedSec: elapsed });
  blocks.push({ type: 'context', elements: [ { type: 'mrkdwn', text: footer.text } ] });
  return blocks;
}
