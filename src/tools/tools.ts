import type { ExecutionTree } from '../execution-tree.js';
import type { SessionTreeBuilder, SessionNode } from '../session-tree.js';
import type { MCPTool, LogEntry, AccountingEntry } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult, ToolKind, ToolProvider, ToolExecutionContext } from './types.js';

import { truncateUtf8WithNotice, formatToolRequestCompact } from '../utils.js';

export class ToolsOrchestrator {
  private readonly providers: ToolProvider[] = [];
  private readonly mapping = new Map<string, { provider: ToolProvider; kind: ToolKind }>();
  // Minimal alias support to smooth tool naming mismatches across prompts/providers
  private readonly aliases = new Map<string, string>([
    // Common alias: map 'netdata' to the configured REST tool 'rest__ask-netdata'
    ['netdata', 'rest__ask-netdata'],
  ]);
  private slotsInUse = 0;
  private waiters: (() => void)[] = [];
  private canceled = false;

  constructor(
    private readonly tree: ExecutionTree,
    private readonly opts: { toolTimeout?: number; toolResponseMaxBytes?: number; maxConcurrentTools?: number; parallelToolCalls?: boolean; traceTools?: boolean },
    private readonly opTree?: SessionTreeBuilder,
    private readonly onOpTreeSnapshot?: (tree: SessionNode) => void
  ) {}

  register(provider: ToolProvider): void {
    this.providers.push(provider);
    // Populate mapping lazily on list to avoid stale entries
  }

  // Warmup providers that require async initialization (e.g., MCP) and refresh tool mapping
  async warmup(): Promise<void> {
    if (this.canceled) return;
    await Promise.all(this.providers.map(async (p) => { try { await p.warmup(); } catch { /* ignore */ } }));
    // Refresh mapping now that providers are warmed
    this.listTools();
  }

  listTools(): MCPTool[] {
    if (this.canceled) return [];
    const tools = this.providers.flatMap((p) => p.listTools());
    // Refresh mapping every time we list
    this.mapping.clear();
    this.providers.forEach((p) => {
      p.listTools().forEach((t) => {
        this.mapping.set(t.name, { provider: p, kind: p.kind });
      });
    });
    return tools;
  }

  private resolveName(name: string): string {
    if (this.mapping.has(name)) return name;
    const alias = this.aliases.get(name);
    if (alias !== undefined && this.mapping.has(alias)) return alias;
    // Heuristics: try common prefixes for sub-agents and REST tools when the model omits them
    const tryAgent = `agent__${name}`;
    if (this.mapping.has(tryAgent)) return tryAgent;
    const tryRest = `rest__${name}`;
    if (this.mapping.has(tryRest)) return tryRest;
    return name;
  }

  hasTool(name: string): boolean {
    if (this.canceled) return false;
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    return this.mapping.has(effective);
  }

  async execute(name: string, args: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    if (this.canceled) throw new Error('canceled');
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    let entry = this.mapping.get(effective);
    if (entry === undefined) {
      // Ensure mapping is fully refreshed before failing (all providers/sub-agents loaded)
      this.listTools();
      const effective2 = this.resolveName(name);
      entry = this.mapping.get(effective2);
    }
    if (entry === undefined) {
      // Fail fast for sub-agent names that are not part of the static registry snapshot
      if (name.startsWith('agent__') || effective.startsWith('agent__')) {
        throw new Error(`unknown_subagent_tool: '${name}' is not registered in this session's agent registry`);
      }
      throw new Error(`Unknown tool: ${name}`);
    }
    return await entry.provider.execute(effective, args, opts);
  }

  // Management wrapper: applies timeout, size cap, logging, and accounting for all providers
  async executeWithManagement(
    name: string,
    args: Record<string, unknown>,
    ctx: ToolExecutionContext,
    opts?: ToolExecuteOptions
  ): Promise<{ result: string; providerLabel: string; latency: number }>{
    if (this.canceled) throw new Error('canceled');
    const bypass = opts?.bypassConcurrency === true;
    if (!bypass) await this.acquireSlot();
    if (this.mapping.size === 0) { this.listTools(); }
    const effective = this.resolveName(name);
    const entry = this.mapping.get(effective);
    if (entry === undefined) throw new Error(`Unknown tool: ${name}`);

    const provider = entry.provider;
    const kind = entry.kind;
    const spanId = this.tree.startSpan(effective, 'tool', { kind, provider: provider.id, turn: ctx.turn, subturn: ctx.subturn });
    // Begin hierarchical op (Option C).
    // Only treat actual sub-agents as child 'session' ops (provider.id === 'subagent').
    // Internal agent-scoped tools (provider.id === 'agent') should remain regular 'tool' ops to avoid ghost sessions.
    const opKind = (kind === 'agent' && provider.id === 'subagent') ? 'session' : 'tool';
    const opId = (() => {
      try { return this.opTree?.beginOp(ctx.turn, opKind, { name: effective, provider: provider.id, kind }); } catch { return undefined; }
    })();
    // For sub-agent session ops, attach a placeholder child session immediately so live views can show it
    try {
      if (opKind === 'session' && this.opTree !== undefined && opId !== undefined) {
        const parentSession = this.opTree.getSession();
        const childName = effective.startsWith('agent__') ? effective.slice('agent__'.length) : effective;
        const childCallPathBase = (typeof parentSession.callPath === 'string' && parentSession.callPath.length > 0)
          ? parentSession.callPath
          : (typeof parentSession.agentId === 'string' ? parentSession.agentId : 'agent');
        const stub: SessionNode = {
          id: `${Date.now().toString(36)}-stub`,
          traceId: parentSession.traceId,
          agentId: childName,
          callPath: `${childCallPathBase}->${childName}`,
          startedAt: Date.now(),
          turns: [],
        };
        this.opTree.attachChildSession(opId, stub);
        if (typeof this.onOpTreeSnapshot === 'function') this.onOpTreeSnapshot(this.opTree.getSession());
      }
    } catch { /* ignore */ }
    const requestMsg = formatToolRequestCompact(effective, args);
    // Log request (compact)
    const reqLog: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: ctx.turn,
      subturn: ctx.subturn,
      direction: 'request',
      type: 'tool',
      toolKind: kind,
      remoteIdentifier: `${kind}:${provider.id}`,
      fatal: false,
      message: requestMsg,
    };
    this.tree.recordLog(reqLog);
    // Optional full request trace (args JSON)
    if (this.opts.traceTools === true) {
      const fullArgs = (() => { try { return JSON.stringify(args, null, 2); } catch { return '[unserializable-args]'; } })();
      const traceReq: LogEntry = {
        timestamp: Date.now(),
        severity: 'TRC',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'request',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: `trace:${kind}:${provider.id}`,
        fatal: false,
        message: `REQUEST ${effective}\n${fullArgs}`,
      };
      this.tree.recordLog(traceReq);
    }

    const start = Date.now();
    const withTimeout = async <T>(p: Promise<T>, timeoutMs?: number): Promise<T> => {
      if (typeof timeoutMs !== 'number' || timeoutMs <= 0) return await p;
      let timer: ReturnType<typeof setTimeout> | undefined;
      try {
        return await Promise.race([
          p,
          new Promise<T>((_resolve, reject) => {
            timer = setTimeout(() => { reject(new Error('Tool execution timed out')); }, timeoutMs);
          })
        ]);
      } finally {
        if (timer !== undefined) clearTimeout(timer);
      }
    };

    // Normalize arguments for known tools to reduce model fragility
    const normalizeArgs = (toolName: string, a: Record<string, unknown>): Record<string, unknown> => {
      // GitHub MCP 'search_code' expects 'q' (string). Models often provide separate fields.
      if (toolName === 'github__search_code') {
        const getStr = (obj: unknown, k: string): string | undefined => {
          if (obj !== null && typeof obj === 'object') {
            const v = (obj as Record<string, unknown>)[k];
            if (typeof v === 'string') {
              const t = v.trim();
              return t.length > 0 ? t : undefined;
            }
          }
          return undefined;
        };
        const existing = getStr(a, 'q');
        if (typeof existing === 'string') return a;
        const parts: string[] = [];
        const query = getStr(a, 'query');
        const repo = getStr(a, 'repo');
        const path = getStr(a, 'path');
        const languageRaw = getStr(a, 'language');
        if (typeof query === 'string' && query.length > 0) parts.push(query);
        if (typeof repo === 'string' && repo.length > 0) parts.push(`repo:${repo}`);
        if (typeof path === 'string' && path.length > 0) parts.push(`path:${path}`);
        // Normalize languages; GitHub doesn't accept 'jsx' as a language qualifier. Use extension:jsx/tsx instead.
        if (typeof languageRaw === 'string' && languageRaw.length > 0) {
          const norm = languageRaw.replace(/\s+OR\s+/gi, ',').replace(/\|/g, ',');
          const toks = norm.split(/[,\s]+/).map((t) => t.trim()).filter((t) => t.length > 0);
          const langSet = new Set<string>();
          const extSet = new Set<string>();
          const mapSyn = (s: string): string => (s === 'js' ? 'javascript' : s === 'ts' ? 'typescript' : s);
          toks.forEach((t) => {
            const low = mapSyn(t.toLowerCase());
            if (low === 'jsx') { extSet.add('jsx'); return; }
            if (low === 'tsx') { extSet.add('tsx'); return; }
            // keep recognized languages; drop obviously invalid ones
            langSet.add(low);
          });
          langSet.forEach((l) => { parts.push(`language:${l}`); });
          extSet.forEach((e) => { parts.push(`extension:${e}`); });
        }
        const q = parts.join(' ').trim();
        if (q.length > 0) return { ...a, q };
        return a;
      }
      return a;
    };

    let exec: ToolExecuteResult | undefined;
    let errorMessage: string | undefined;
    try {
      const preparedArgs = normalizeArgs(effective, args);
      // Do not apply parent-level withTimeout to sub-agents; they manage their own timing
      const isSubAgent = (kind === 'agent' && provider.id === 'subagent');
      if (isSubAgent) {
        exec = await provider.execute(effective, preparedArgs, { ...opts, timeoutMs: undefined });
      } else {
        exec = await withTimeout(
          provider.execute(effective, preparedArgs, { ...opts, timeoutMs: opts?.timeoutMs ?? this.opts.toolTimeout }),
          this.opts.toolTimeout
        );
      }
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : String(e);
    }

    const latency = Date.now() - start;
    const charactersIn = (() => { try { return JSON.stringify(args).length; } catch { return 0; } })();

    const isFailed = (() => {
      if (exec === undefined) return true;
      return !exec.ok;
    })();
    if (isFailed) {
      const msg = (exec?.error ?? errorMessage ?? 'execution_failed');
      if (this.opts.traceTools === true) {
        const traceErr: LogEntry = {
          timestamp: Date.now(),
          severity: 'TRC',
          turn: ctx.turn,
          subturn: ctx.subturn,
          direction: 'response',
          type: 'tool',
          toolKind: kind,
          remoteIdentifier: `trace:${kind}:${provider.id}`,
          fatal: false,
          message: `ERROR ${effective}\n${msg}`,
        };
        this.tree.recordLog(traceErr);
      }
      const errLog: LogEntry = {
        timestamp: Date.now(),
        severity: 'ERR',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: `${kind}:${provider.id}`,
        fatal: false,
        message: `error ${name}: ${msg}`,
      };
      this.tree.recordLog(errLog);
      const acc: AccountingEntry = {
        type: 'tool', timestamp: start, status: 'failed', latency,
        mcpServer: provider.id, command: name, charactersIn, charactersOut: 0, error: msg,
      };
      this.tree.recordAccounting(acc);
      this.tree.endSpan(spanId, 'failed', { error: msg });
      try {
        if (opId !== undefined) {
          this.opTree?.appendLog(opId, reqLog);
          this.opTree?.endOp(opId, 'failed', { latency, error: msg });
        }
      } catch { /* ignore */ }
      this.releaseSlot();
      throw new Error(msg);
    }

    const safeExec = (() => { if (exec === undefined) { throw new Error('unexpected_undefined_execution_result'); } return exec; })();
    const raw = typeof safeExec.result === 'string' ? safeExec.result : '';
    // Optional full response trace (raw, before truncation)
    if (this.opts.traceTools === true) {
      // Prefer provider-supplied raw payload when available
      const rawPayload = (() => {
        const extra = safeExec.extras;
        if (extra !== undefined && typeof extra === 'object') {
          const rp = (extra as { rawResponse?: unknown }).rawResponse;
          if (typeof rp === 'string' && rp.length > 0) return rp;
        }
        return raw;
      })();
      const traceRes: LogEntry = {
        timestamp: Date.now(),
        severity: 'TRC',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: `trace:${kind}:${provider.id}`,
        fatal: false,
        message: `RESPONSE ${effective}\n${rawPayload}`,
      };
      this.tree.recordLog(traceRes);
    }
    const sizeBytes = Buffer.byteLength(raw, 'utf8');
    const limit = this.opts.toolResponseMaxBytes;
    const providerLabel = kind === 'mcp' ? safeExec.providerId : kind; // 'rest' or server name
    let result = raw;
    if (typeof limit === 'number' && limit > 0 && sizeBytes > limit) {
      // Warn about truncation
      const warn: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: `${kind}:${provider.id}`,
        fatal: false,
        message: `response exceeded max size: ${String(sizeBytes)} bytes > limit ${String(limit)} bytes (truncated)`
      };
      this.tree.recordLog(warn);
      result = truncateUtf8WithNotice(raw, limit, sizeBytes);
    }

    // Ensure non-empty result for downstream providers that expect a non-empty tool output
    if (result.length === 0) result = ' ';

    const resLog: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: ctx.turn,
      subturn: ctx.subturn,
      direction: 'response',
      type: 'tool',
      toolKind: kind,
      remoteIdentifier: `${kind}:${provider.id}`,
      fatal: false,
      message: `ok ${effective}: ${String(result.length)} chars`,
    };
    this.tree.recordLog(resLog);

    const accOk: AccountingEntry = {
      type: 'tool', timestamp: start, status: 'ok', latency,
      mcpServer: providerLabel, command: name, charactersIn, charactersOut: result.length,
    };
    this.tree.recordAccounting(accOk);
    // Optional: child accounting (from sub-agents)
    // Do NOT forward child accounting to the parent execution tree here.
    // Sub-agents already emit their own onAccounting callbacks (wired to the same SessionManager),
    // and their full accounting is attached to the opTree via attachChildSession above.
    // Re-injecting entries here would double-count tokens/cost in Slack progress at finish.
    this.tree.endSpan(spanId, 'ok', { latency, size: result.length });
    try {
      if (opId !== undefined) {
        this.opTree?.appendLog(opId, reqLog);
        this.opTree?.appendLog(opId, resLog);
        // Attach child session tree when provider is 'agent'
        try {
          if (opKind === 'session') {
            const maybe = (safeExec.extras as { childOpTree?: SessionNode } | undefined)?.childOpTree;
            if (maybe !== undefined) {
              this.opTree?.attachChildSession(opId, maybe);
            }
          }
        } catch { /* ignore child attach errors */ }
        this.opTree?.endOp(opId, 'ok', { latency, size: result.length });
        try {
          if (typeof this.onOpTreeSnapshot === 'function' && this.opTree !== undefined) {
            this.onOpTreeSnapshot(this.opTree.getSession());
          }
        } catch { /* ignore */ }
      }
    } catch { /* ignore */ }
    if (!bypass) this.releaseSlot();
    return { result, providerLabel, latency };
  }

  private async acquireSlot(): Promise<void> {
    const cap = Math.max(1, this.opts.maxConcurrentTools ?? 1);
    const effectiveCap = (this.opts.parallelToolCalls === false) ? 1 : cap;
    if (this.canceled) throw new Error('canceled');
    if (this.slotsInUse < effectiveCap) {
      this.slotsInUse += 1;
      return;
    }
    await new Promise<void>((resolve, reject) => { this.waiters.push(() => { if (this.canceled) reject(new Error('canceled')); else resolve(); }); });
    this.slotsInUse += 1;
  }

  private releaseSlot(): void {
    this.slotsInUse = Math.max(0, this.slotsInUse - 1);
    const next = this.waiters.shift();
    if (next !== undefined) { try { next(); } catch { /* ignore */ } }
  }

  // Aggregate instructions from MCP providers
  getMCPInstructions(): string {
    const parts: string[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const p of this.providers) {
      if (p.kind === 'mcp' && typeof (p as unknown as { getCombinedInstructions?: () => string }).getCombinedInstructions === 'function') {
        try {
          const s = (p as unknown as { getCombinedInstructions: () => string }).getCombinedInstructions();
          if (s && s.length > 0) parts.push(s);
        } catch { /* ignore */ }
      }
    }
    return parts.join('\n\n');
  }

  async cleanup(): Promise<void> {
    this.canceled = true;
    // release all waiters
    try { this.waiters.splice(0).forEach((w) => { try { w(); } catch { /* ignore */ } }); } catch { /* ignore */ }
    // eslint-disable-next-line functional/no-loop-statements
    for (const p of this.providers) {
      const maybe = p as unknown as { cleanup?: () => Promise<void> };
      if (typeof maybe.cleanup === 'function') {
        try { await maybe.cleanup(); } catch { /* ignore */ }
      }
    }
  }

  // Explicit cancel entrypoint to abort queue and tear down providers
  cancel(): void {
    this.canceled = true;
    try { this.waiters.splice(0).forEach((w) => { try { w(); } catch { /* ignore */ } }); } catch { /* ignore */ }
    // Best-effort async cleanup; do not await here to avoid blocking callers
    void this.cleanup();
  }
}
