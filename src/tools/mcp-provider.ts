/* eslint-disable import/order */
/* eslint-disable perfectionist/sort-imports */
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

import type { ChildProcess } from 'node:child_process';
import { performance } from 'node:perf_hooks';

import type { MCPServerConfig, MCPTool, MCPServer, LogEntry } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';

import { createWebSocketTransport } from '../websocket-transport.js';
import { warn } from '../utils.js';
import { ToolProvider } from './types.js';

export class MCPProvider extends ToolProvider {
  readonly kind = 'mcp' as const;
  private readonly serversConfig: Record<string, MCPServerConfig>;
  private clients = new Map<string, Client>();
  private servers = new Map<string, MCPServer>();
  private failedServers = new Set<string>();
  private processes = new Map<string, ChildProcess>();
  private toolNameMap = new Map<string, { serverName: string; originalName: string }>();
  private initialized = false;
  private initializationPromise?: Promise<void>;
  private trace = false;
  private verbose = false;
  private requestTimeoutMs?: number;
  private onLog?: (entry: LogEntry) => void;
  private initConcurrency?: number;

  constructor(public readonly namespace: string, servers: Record<string, MCPServerConfig>, opts?: { trace?: boolean; verbose?: boolean; requestTimeoutMs?: number; onLog?: (entry: LogEntry) => void; initConcurrency?: number }) {
    super();
    this.serversConfig = servers;
    this.trace = opts?.trace === true;
    this.verbose = opts?.verbose === true;
    this.requestTimeoutMs = ((): number | undefined => {
      const v = opts?.requestTimeoutMs;
      return typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.trunc(v) : undefined;
    })();
    this.onLog = opts?.onLog;
    const limit = opts?.initConcurrency;
    if (typeof limit === 'number' && Number.isFinite(limit) && limit > 0) {
      this.initConcurrency = Math.trunc(limit);
    }
  }

  override getInstructions(): string {
    return this.getCombinedInstructions();
  }

  override resolveLogProvider(name: string): string {
    const mapping = this.toolNameMap.get(name);
    if (mapping !== undefined) {
      return `${this.namespace}:${mapping.serverName}`;
    }
    const prefix = name.includes('__') ? name.split('__')[0] : name;
    return `${this.namespace}:${prefix}`;
  }

  override resolveToolIdentity(name: string): { namespace: string; tool: string } {
    const mapping = this.toolNameMap.get(name);
    if (mapping !== undefined) {
      const sanitizedNamespace = name.includes('__') ? name.split('__')[0] : this.sanitizeNamespace(mapping.serverName);
      return { namespace: sanitizedNamespace, tool: mapping.originalName };
    }
    const sanitizedNamespace = name.includes('__') ? name.split('__')[0] : this.sanitizeNamespace(name);
    const tool = name.includes('__') ? name.slice(name.indexOf('__') + 2) : name;
    return { namespace: sanitizedNamespace, tool };
  }

  private sanitizeNamespace(name: string): string {
    if (typeof name !== 'string') {
      throw new Error('MCP server names must be strings.');
    }
    const trimmed = name.trim();
    if (trimmed.length === 0) {
      throw new Error('MCP server names must be non-empty strings.');
    }
    if (!/^[A-Za-z0-9_-]+$/.test(trimmed)) {
      throw new Error(`MCP server names may only contain letters, digits, '-' or '_': ${name}`);
    }
    return trimmed;
  }

  private async ensureInitialized(): Promise<void> {
    if (this.initialized) return;
    
    // If initialization is already in progress, wait for it
    if (this.initializationPromise !== undefined) {
      await this.initializationPromise;
      return;
    }
    
    // Start initialization and store the promise to prevent concurrent attempts
    this.initializationPromise = this.doInitialize();
    await this.initializationPromise;
  }
  
  private async doInitialize(): Promise<void> {
    const entries = Object.entries(this.serversConfig);
    if (entries.length === 0) {
      this.initialized = true;
      return;
    }

    const limit = this.initConcurrency ?? entries.length;
    let active = 0;
    const waiters: (() => void)[] = [];

    const acquire = async (): Promise<void> => {
      if (active < limit) {
        active += 1;
        return;
      }
      await new Promise<void>((resolve) => { waiters.push(resolve); });
      active += 1;
    };

    const release = (): void => {
      active = Math.max(0, active - 1);
      const next = waiters.shift();
      if (next !== undefined) {
        try { next(); } catch { /* noop */ }
      }
    };

    interface InitResult { name: string; elapsedMs: number; status: 'success' | 'failure'; error?: string }
    const results: InitResult[] = [];
    const failedServers: string[] = [];

    const warmupStart = performance.now();

    try {
      await Promise.all(entries.map(async ([name, config]) => {
        await acquire();
        const start = performance.now();
        try {
          this.log('TRC', `initializing '${name}' (${config.type})`, `mcp:${name}`);
          const server = await this.initializeServer(name, config);
          this.servers.set(name, server);
          this.log('TRC', `initialized '${name}' with ${String(server.tools.length)} tools`, `mcp:${name}`);
          const elapsedMs = Math.round(performance.now() - start);
          results.push({ name, elapsedMs, status: 'success' });
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          const elapsedMs = Math.round(performance.now() - start);
          this.log('ERR', `failed to initialize '${name}': ${msg}`, `mcp:${name}`, true);
          this.failedServers.add(name);
          failedServers.push(`${name} (${msg})`);
          results.push({ name, elapsedMs, status: 'failure', error: msg });
        } finally {
          release();
        }
      }));
    } finally {
      this.initializationPromise = undefined;
    }

    const totalElapsed = Math.round(performance.now() - warmupStart);
    const sorted = results.sort((a, b) => b.elapsedMs - a.elapsedMs);
    const summaryParts = sorted.length > 0
      ? sorted.map((r) => {
          const base = `${r.name}=${String(r.elapsedMs)}ms`;
          return r.status === 'success' ? base : `${base} FAILED (${r.error ?? 'unknown error'})`;
        })
      : ['<no servers>'];
    this.log('VRB', `MCP initialization latencies (total=${String(totalElapsed)}ms, desc): ${summaryParts.join(', ')}`, 'mcp:init');

    if (failedServers.length > 0) {
      const warnMessage = `Some MCP servers failed to initialize and will be unavailable: ${failedServers.join(', ')}`;
      this.log('WRN', warnMessage, 'mcp:init');
    }

    this.initialized = true;
  }

  async warmup(): Promise<void> {
    await this.ensureInitialized();
  }

  private filterToolsForServer(serverName: string, config: MCPServerConfig, tools: MCPTool[]): MCPTool[] {
    const allowedRaw = Array.isArray(config.toolsAllowed) && config.toolsAllowed.length > 0 ? config.toolsAllowed : ['*'];
    const deniedRaw = Array.isArray(config.toolsDenied) ? config.toolsDenied : [];

    const normalize = (values: string[]): { entries: Set<string>; wildcard: boolean } => {
      let wildcard = false;
      const entries = new Set<string>();
      values.forEach((item) => {
        if (typeof item !== 'string') return;
        const trimmed = item.trim();
        if (trimmed.length === 0) return;
        const lower = trimmed.toLowerCase();
        if (lower === '*' || lower === 'any') {
          wildcard = true;
          return;
        }
        entries.add(lower);
      });
      return { entries, wildcard };
    };

    const allowed = normalize(allowedRaw);
    const denied = normalize(deniedRaw);

    const isAllowed = (toolName: string): boolean => {
      const key = toolName.toLowerCase();
      if (allowed.wildcard) return true;
      return allowed.entries.has(key);
    };

    const isDenied = (toolName: string): boolean => {
      const key = toolName.toLowerCase();
      if (denied.wildcard) return true;
      return denied.entries.has(key);
    };

    const initialCount = tools.length;
    const filtered = tools.filter((tool) => {
      if (!isAllowed(tool.name)) return false;
      if (isDenied(tool.name)) return false;
      return true;
    });

    if (this.verbose && initialCount !== filtered.length) {
      const removedNames = tools
        .filter((tool) => !filtered.includes(tool))
        .map((tool) => tool.name)
        .join(', ');
      const msg = removedNames.length > 0
        ? `filtered tools for '${serverName}': removed [${removedNames}]`
        : `filtered tools for '${serverName}': no tools removed`;
      this.log('VRB', msg, `mcp:${serverName}`);
    }

    return filtered;
  }

  private async initializeServer(name: string, config: MCPServerConfig): Promise<MCPServer> {
    const client = new Client(
      { name: 'ai-agent', version: '1.0.0' },
      // Pass extended options if supported by SDK; unknown keys are ignored gracefully
      { capabilities: { tools: {} }, ...(this.requestTimeoutMs !== undefined ? { requestTimeoutMs: this.requestTimeoutMs } : {}) } as unknown as Record<string, unknown>
    );
    let transport: Transport;
    switch (config.type) {
      case 'stdio':
        transport = this.createStdioTransport(name, config);
        break;
      case 'websocket': {
        if (config.url == null || config.url.length === 0) throw new Error(`WebSocket MCP server '${name}' requires a 'url'`);
        transport = await createWebSocketTransport(config.url, config.headers);
        break;
      }
      case 'http': {
        if (config.url == null || config.url.length === 0) throw new Error(`HTTP MCP server '${name}' requires a 'url'`);
        const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
        const reqInit: RequestInit = { headers: (config.headers ?? {}) as HeadersInit };
        transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
        break;
      }
      case 'sse': {
        if (config.url == null || config.url.length === 0) throw new Error(`SSE MCP server '${name}' requires a 'url'`);
        const resolvedHeaders = config.headers ?? {};
        const customFetch: typeof fetch = async (input, init) => {
          const headers = new Headers(init?.headers);
          Object.entries(resolvedHeaders).forEach(([k, v]) => { headers.set(k, v); });
          return fetch(input, { ...init, headers });
        };
        transport = new SSEClientTransport(new URL(config.url), { eventSourceInit: { fetch: customFetch }, requestInit: { headers: resolvedHeaders as HeadersInit }, fetch: customFetch });
        break;
      }
      default:
        throw new Error('Unsupported transport type');
    }
    try {
      await client.connect(transport);
      this.log('TRC', `connected to '${name}'`, `mcp:${name}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      this.log('ERR', `connect failed for '${name}': ${msg}`, `mcp:${name}`);
      throw e;
    }
    this.clients.set(name, client);

    const initInstructions = client.getInstructions() ?? '';
    if (this.trace) {
      const len = initInstructions.length;
      this.log('TRC', `instructions length for '${name}': ${String(len)}`, `mcp:${name}`);
    }
    const toolsResponse = await client.listTools();
    interface ToolItem { name: string; description?: string; inputSchema?: unknown; parameters?: unknown; instructions?: string }
    const rawTools: MCPTool[] = (toolsResponse.tools as ToolItem[]).map((t) => ({
      name: t.name,
      description: t.description ?? '',
      inputSchema: (t.inputSchema ?? t.parameters ?? {}) as Record<string, unknown>,
      instructions: t.instructions,
    }));
    const tools = this.filterToolsForServer(name, config, rawTools);
    if (this.trace) {
      const names = tools.map((t) => t.name).join(', ');
      this.log('TRC', `listTools('${name}') -> ${String(tools.length)} tools [${names}]`, `mcp:${name}`);
    }
    const ns = this.sanitizeNamespace(name);
    tools.forEach((t) => { this.toolNameMap.set(`${ns}__${t.name}`, { serverName: name, originalName: t.name }); });

    // Optional prompts
    let instructions = initInstructions;
    try {
      const pr = await client.listPrompts();
      const arr = (pr as { prompts?: { name: string; description?: string }[] } | undefined)?.prompts;
      const list = Array.isArray(arr) ? arr : [];
      if (list.length > 0) {
        const promptText = list.map((p) => `${p.name}: ${p.description ?? ''}`).join('\n');
        instructions = instructions.length > 0 ? `${instructions}\n${promptText}` : promptText;
      }
    } catch (e) {
      if (this.trace) {
        const msg = e instanceof Error ? e.message : String(e);
        this.log('WRN', `listPrompts failed for '${name}': ${msg}`, `mcp:${name}`);
      }
      /* prompts optional */ }

    this.servers.set(name, { name, config, tools, instructions });
    return { name, config, tools, instructions };
  }

  private createStdioTransport(name: string, config: MCPServerConfig): StdioClientTransport {
    if (typeof config.command !== 'string' || config.command.length === 0) {
      throw new Error(`Stdio MCP server '${name}' requires a string 'command'`);
    }
    const env: Record<string, string> = { ...(config.env ?? {}) };
    const transport = new StdioClientTransport({ command: config.command, args: config.args ?? [], env, stderr: 'pipe' });
    try {
      if (transport.stderr != null) {
        transport.stderr.on('data', (chunk: Buffer) => {
          try {
            const s = chunk.toString('utf8');
            this.log('ERR', `stderr '${name}': ${s.trim()}`, `mcp:${name}`);
          } catch (e) { warn(`mcp stdio stderr relay failed: ${e instanceof Error ? e.message : String(e)}`); }
        });
      }
    } catch (e) { warn(`mcp stdio transport setup failed: ${e instanceof Error ? e.message : String(e)}`); }
    return transport;
  }

  private log(severity: 'VRB' | 'WRN' | 'ERR' | 'TRC', message: string, remoteIdentifier: string, fatal = false): void {
    const entry: LogEntry = {
      timestamp: Date.now(), severity, turn: 0, subturn: 0, direction: 'response', type: 'tool', toolKind: 'mcp', remoteIdentifier, fatal, message,
    };
    try { this.onLog?.(entry); } catch (e) { warn(`mcp onLog failed: ${e instanceof Error ? e.message : String(e)}`); }
  }

  listTools(): MCPTool[] {
     
    if (!this.initialized) { void this.ensureInitialized(); }
    const out: MCPTool[] = [];
    this.servers.forEach((s, serverName) => {
      const ns = this.sanitizeNamespace(serverName);
      s.tools.forEach((t) => {
        out.push({ name: `${ns}__${t.name}`, description: t.description, inputSchema: t.inputSchema, instructions: t.instructions });
      });
    });
    return out;
  }

  hasTool(name: string): boolean {
     
    if (!this.initialized) { void this.ensureInitialized(); }
    return this.toolNameMap.has(name);
  }

  async execute(name: string, parameters: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    await this.ensureInitialized();
    const mapping = this.toolNameMap.get(name);
    if (mapping === undefined) throw new Error(`No server found for tool: ${name}`);
    const { serverName, originalName } = mapping;
    const sanitizedNamespace = name.includes('__') ? name.split('__')[0] : this.sanitizeNamespace(serverName);
    const client = this.clients.get(serverName);
    if (client === undefined) throw new Error(`MCP server not ready: ${serverName}`);
    const start = Date.now();
    // Use official SDK signature: client.callTool({ name, arguments })
    const requestOptions = ((): Record<string, unknown> => {
      const t = _opts?.timeoutMs;
      const has = typeof t === 'number' && Number.isFinite(t) && t > 0;
      return has ? { timeout: Math.trunc(t), resetTimeoutOnProgress: true, maxTotalTimeout: Math.trunc(t) } : {};
    })();
    // Pass per-call timeout options so we don't hit the SDK's 60s default
    const res = await client.callTool({ name: originalName, arguments: parameters }, undefined as unknown as never, requestOptions as never);
    const latency = Date.now() - start;
    // Try to normalize the result to a string for downstream handling
    const isRecord = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
    const text = (() => {
      let content: unknown;
      if (isRecord(res) && Object.prototype.hasOwnProperty.call(res, 'content')) {
        content = (res as Record<string, unknown>).content;
      }
      if (typeof content === 'string') return content;
      if (Array.isArray(content)) {
        const texts = (content as unknown[])
          .map((p) => (isRecord(p) && typeof p.text === 'string' ? p.text : undefined))
          .filter((t): t is string => typeof t === 'string');
        if (texts.length > 0) return texts.join('');
      }
      try { return JSON.stringify(res); } catch { return ''; }
    })();
    const rawJson = (() => { try { return JSON.stringify(res, null, 2); } catch { return undefined; } })();
    const rawReq = (() => { try { return JSON.stringify(parameters, null, 2); } catch { return undefined; } })();
    return {
      ok: true,
      result: text,
      latencyMs: latency,
      kind: this.kind,
      namespace: sanitizedNamespace,
      extras: { rawResponse: rawJson, rawRequest: rawReq }
    };
  }

  getCombinedInstructions(): string {
    const segments: string[] = [];
    this.servers.forEach((server, serverKey) => {
      const ns = this.sanitizeNamespace(serverKey);
      const blocks: string[] = [];
      const displayName = server.name || serverKey;
      blocks.push(`#### MCP Server: ${displayName}`);
      if (typeof server.instructions === 'string') {
        const trimmedServer = server.instructions.trim();
        if (trimmedServer.length > 0) {
          blocks.push(trimmedServer);
        }
      }
      server.tools.forEach((tool) => {
        if (typeof tool.instructions !== 'string') return;
        const trimmedTool = tool.instructions.trim();
        if (trimmedTool.length === 0) return;
        const exposed = `${ns}__${tool.name}`;
        blocks.push(`##### Tool: ${exposed}
${trimmedTool}`);
      });
      const nonEmpty = blocks.filter((block) => block.trim().length > 0);
      if (nonEmpty.length > 0) {
        segments.push(nonEmpty.join('\n\n'));
      }
    });
    return segments.join('\n\n');
  }

  async cleanup(): Promise<void> {
    await Promise.all(Array.from(this.clients.values()).map(async (c) => { try { await c.close(); } catch (e) { warn(`mcp client close failed: ${e instanceof Error ? e.message : String(e)}`); } }));
    await Promise.all(Array.from(this.processes.values()).map((proc) => new Promise<void>((resolve) => {
      try {
        if (proc.killed || proc.exitCode !== null) { resolve(); return; }
        const timeout = setTimeout(() => { try { if (!proc.killed && proc.exitCode === null) proc.kill('SIGKILL'); } catch { } }, 2000);
        proc.once('exit', () => { clearTimeout(timeout); resolve(); });
        proc.kill('SIGTERM');
      } catch { resolve(); }
    })));
    this.clients.clear();
    this.processes.clear();
    this.servers.clear();
    this.toolNameMap.clear();
    this.initialized = false;
  }
}
