import type { ExecutionTree } from '../execution-tree.js';
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

  constructor(
    private readonly tree: ExecutionTree,
    private readonly opts: { toolTimeout?: number; toolResponseMaxBytes?: number; maxConcurrentTools?: number; parallelToolCalls?: boolean }
  ) {}

  register(provider: ToolProvider): void {
    this.providers.push(provider);
    // Populate mapping lazily on list to avoid stale entries
  }

  listTools(): MCPTool[] {
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
    return (alias !== undefined && this.mapping.has(alias)) ? alias : name;
  }

  hasTool(name: string): boolean {
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    return this.mapping.has(effective);
  }

  async execute(name: string, args: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    const entry = this.mapping.get(effective);
    if (entry === undefined) throw new Error(`Unknown tool: ${name}`);
    return await entry.provider.execute(effective, args, opts);
  }

  // Management wrapper: applies timeout, size cap, logging, and accounting for all providers
  async executeWithManagement(
    name: string,
    args: Record<string, unknown>,
    ctx: ToolExecutionContext,
    opts?: ToolExecuteOptions
  ): Promise<{ result: string; providerLabel: string; latency: number }>{
    const bypass = opts?.bypassConcurrency === true;
    if (!bypass) await this.acquireSlot();
    if (this.mapping.size === 0) { this.listTools(); }
    const effective = this.resolveName(name);
    const entry = this.mapping.get(effective);
    if (entry === undefined) throw new Error(`Unknown tool: ${name}`);

    const provider = entry.provider;
    const kind = entry.kind;
    const spanId = this.tree.startSpan(effective, 'tool', { kind, provider: provider.id, turn: ctx.turn, subturn: ctx.subturn });
    const requestMsg = formatToolRequestCompact(effective, args);
    // Log request
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

    let exec: ToolExecuteResult | undefined;
    let errorMessage: string | undefined;
    try {
      exec = await withTimeout(
        provider.execute(name, args, { ...opts, timeoutMs: opts?.timeoutMs ?? this.opts.toolTimeout }),
        this.opts.toolTimeout
      );
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
      this.releaseSlot();
      throw new Error(msg);
    }

    const safeExec = (() => { if (exec === undefined) { throw new Error('unexpected_undefined_execution_result'); } return exec; })();
    const raw = typeof safeExec.result === 'string' ? safeExec.result : '';
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
    // Optional: record child accounting when provided by provider (e.g., subagents)
    const extras = (safeExec.extras ?? {});
    const hasChildAcc = (val: unknown): val is { childAccounting?: unknown } => (val !== null && typeof val === 'object');
    const childAcc = hasChildAcc(extras) ? extras.childAccounting : undefined;
    if (Array.isArray(childAcc)) {
      const isAccountingEntry = (val: unknown): val is AccountingEntry => {
        if (val === null || typeof val !== 'object') return false;
        const obj = val as Record<string, unknown>;
        return typeof obj.type === 'string' && typeof obj.timestamp === 'number';
      };
      childAcc.forEach((a) => { if (isAccountingEntry(a)) { try { this.tree.recordAccounting(a); } catch { /* ignore */ } } });
    }
    this.tree.endSpan(spanId, 'ok', { latency, size: result.length });
    if (!bypass) this.releaseSlot();
    return { result, providerLabel, latency };
  }

  private async acquireSlot(): Promise<void> {
    const cap = Math.max(1, this.opts.maxConcurrentTools ?? 1);
    const effectiveCap = (this.opts.parallelToolCalls === false) ? 1 : cap;
    if (this.slotsInUse < effectiveCap) {
      this.slotsInUse += 1;
      return;
    }
    await new Promise<void>((resolve) => { this.waiters.push(resolve); });
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
    // eslint-disable-next-line functional/no-loop-statements
    for (const p of this.providers) {
      const maybe = p as unknown as { cleanup?: () => Promise<void> };
      if (typeof maybe.cleanup === 'function') {
        try { await maybe.cleanup(); } catch { /* ignore */ }
      }
    }
  }
}
