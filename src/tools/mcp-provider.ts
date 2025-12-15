/* eslint-disable import/order */
/* eslint-disable perfectionist/sort-imports */
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

import { performance } from 'node:perf_hooks';

import type { MCPServerConfig, MCPTool, MCPServer, LogEntry } from '../types.js';
import type { ToolCancelOptions, ToolExecuteOptions, ToolExecuteResult } from './types.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';

import { createWebSocketTransport } from '../websocket-transport.js';
import { warn } from '../utils.js';
import { ToolProvider } from './types.js';
import { killProcessTree } from '../utils/process-tree.js';

interface MCPProcessHandle {
  pid: number;
  command: string;
  args: string[];
  startedAt: number;
}

type LogSeverity = 'VRB' | 'WRN' | 'ERR' | 'TRC';
export type LogFn = (severity: LogSeverity, message: string, remoteIdentifier: string, fatal?: boolean) => void;

interface MCPRestartErrorOptions {
  serverName: string;
  message: string;
}

export class MCPRestartError extends Error {
  readonly code: 'mcp_restart_failed' | 'mcp_restart_in_progress';

  constructor(code: 'mcp_restart_failed' | 'mcp_restart_in_progress', opts: MCPRestartErrorOptions) {
    super(opts.message);
    this.name = 'MCPRestartError';
    this.code = code;
  }
}

export class MCPRestartFailedError extends MCPRestartError {
  constructor(serverName: string, details: string) {
    super('mcp_restart_failed', { serverName, message: `mcp_restart_failed:${serverName}: ${details}` });
  }
}

export class MCPRestartInProgressError extends MCPRestartError {
  constructor(serverName: string, details: string) {
    super('mcp_restart_in_progress', { serverName, message: `mcp_restart_in_progress:${serverName}: ${details}` });
  }
}

/**
 * Format MCP server config for logging purposes. Sensitive values (env, headers) are redacted.
 */
function formatMCPConfigForLog(name: string, config: MCPServerConfig): string {
  const parts: string[] = [`server='${name}'`, `type=${config.type}`];
  switch (config.type) {
    case 'stdio':
      parts.push(`command='${config.command ?? '<missing>'}'`);
      if (config.args !== undefined && config.args.length > 0) {
        parts.push(`args=[${config.args.map((a) => `'${a}'`).join(', ')}]`);
      }
      if (config.env !== undefined) {
        const envKeys = Object.keys(config.env);
        if (envKeys.length > 0) {
          parts.push(`env_keys=[${envKeys.join(', ')}]`);
        }
      }
      break;
    case 'websocket':
    case 'http':
    case 'sse':
      parts.push(`url='${config.url ?? '<missing>'}'`);
      if (config.headers !== undefined) {
        const headerKeys = Object.keys(config.headers);
        if (headerKeys.length > 0) {
          parts.push(`header_keys=[${headerKeys.join(', ')}]`);
        }
      }
      break;
  }
  if (config.shared === false) {
    parts.push('shared=false');
  }
  if (config.healthProbe !== undefined) {
    parts.push(`healthProbe=${config.healthProbe}`);
  }
  if (config.requestTimeoutMs !== undefined) {
    parts.push(`requestTimeoutMs=${String(config.requestTimeoutMs)}`);
  }
  return parts.join(', ');
}

export interface SharedAcquireOptions {
  trace: boolean;
  verbose: boolean;
  requestTimeoutMs?: number;
  log: LogFn;
  filterTools: (raw: MCPTool[]) => MCPTool[];
}

interface SharedServerEntry {
  serverName: string;
  config: MCPServerConfig;
  client: Client;
  transport: Transport;
  pid: number | null;
  server: MCPServer;
  refCount: number;
  healthProbe: 'ping' | 'listTools';
  restartPromise?: Promise<void>;
  restartAttempt?: { promise: Promise<boolean>; resolve: (value: boolean) => void };
  restartError?: MCPRestartFailedError;
  trace: boolean;
  verbose: boolean;
  requestTimeoutMs?: number;
  log: LogFn;
  filterTools: (raw: MCPTool[]) => MCPTool[];
  transportClosing: boolean;
}

export interface SharedRegistryHandle {
  server: MCPServer;
  callTool: (name: string, parameters: Record<string, unknown>, requestOptions: Record<string, unknown> | undefined) => Promise<unknown>;
  handleCancel: (reason: 'timeout' | 'abort', logger: LogFn) => Promise<void>;
  release: () => void;
}

export interface SharedRegistry {
  acquire: (serverName: string, config: MCPServerConfig, opts: SharedAcquireOptions) => Promise<SharedRegistryHandle>;
  getRestartError?: (serverName: string) => MCPRestartFailedError | undefined;
  shutdown?: () => Promise<void> | void;
}

const DEFAULT_HEALTH_PROBE = 'ping' as const;
const PROBE_TIMEOUT_MS = 3000;
const SHARED_RESTART_BACKOFF_MS = [0, 1000, 2000, 5000, 10000, 30000, 60000] as const;
const SHARED_ACQUIRE_TIMEOUT_MS = 60000;

class SharedServerHandle implements SharedRegistryHandle {
  constructor(private registry: MCPSharedRegistry, private key: string, private entry: SharedServerEntry) {}

  get server(): MCPServer {
    return this.entry.server;
  }

  async callTool(name: string, parameters: Record<string, unknown>, requestOptions: Record<string, unknown> | undefined): Promise<unknown> {
    this.registry.ensureReadyForCall(this.entry);
    try {
      return await this.entry.client.callTool({ name, arguments: parameters }, undefined, requestOptions);
    } catch (error) {
      const message = error instanceof Error ? error.message : undefined;
      const looksLikeTimeout = typeof message === 'string' && message.toLowerCase().includes('request timed out');
      if (looksLikeTimeout) {
        try {
          await this.registry.handleTimeout(this.key, this.entry.log);
        } catch (restartError) {
          throw restartError;
        }
      }
      this.registry.ensureReadyForCall(this.entry);
      throw error;
    }
  }

  async handleCancel(reason: 'timeout' | 'abort', logger: LogFn): Promise<void> {
    if (reason !== 'timeout') return;
    await this.registry.handleTimeout(this.key, logger);
  }

  release(): void {
    this.registry.release(this.key);
  }
}

class MCPSharedRegistry {
  private entries = new Map<string, SharedServerEntry>();
  private initializing = new Map<string, Promise<SharedServerEntry>>();
  private failedServers = new Map<string, number>(); // serverName -> failedAt timestamp
  private stopping = false;

  async acquire(serverName: string, config: MCPServerConfig, opts: SharedAcquireOptions): Promise<SharedServerHandle> {
    if (this.stopping) {
      throw new Error(`shared MCP registry is shutting down; cannot acquire '${serverName}'`);
    }

    // Already initialized - return immediately
    let entry = this.entries.get(serverName);
    if (entry !== undefined) {
      entry.refCount += 1;
      return new SharedServerHandle(this, serverName, entry);
    }

    // Short circuit: if server previously failed and hasn't recovered, fail immediately
    // (background init is still retrying, no point waiting another 60s)
    const failedAt = this.failedServers.get(serverName);
    if (failedAt !== undefined) {
      const elapsedSec = Math.round((Date.now() - failedAt) / 1000);
      throw new Error(`MCP server '${serverName}' is unavailable (initialization failed ${String(elapsedSec)}s ago, retrying in background)`);
    }

    // Start or join initialization
    let pending = this.initializing.get(serverName);
    if (pending === undefined) {
      pending = (async () => {
        try {
          const created = await this.initializeEntry(serverName, config, opts);
          this.entries.set(serverName, created);
          this.failedServers.delete(serverName); // Clear failed state on success
          return created;
        } finally {
          this.initializing.delete(serverName);
        }
      })();
      this.initializing.set(serverName, pending);
    }

    // Wait with timeout
    const timeoutPromise = (async (): Promise<never> => {
      await delay(SHARED_ACQUIRE_TIMEOUT_MS);
      throw new Error(`MCP server '${serverName}' initialization timed out after 60s`);
    })();

    try {
      entry = await Promise.race([pending, timeoutPromise]);
    } catch (error) {
      // On timeout, record failure timestamp for short-circuit, but don't stop background retries
      // Only set timestamp once - don't reset on subsequent failures (preserves TTL window)
      if (!this.failedServers.has(serverName)) {
        this.failedServers.set(serverName, Date.now());
      }
      throw error;
    }

    entry.refCount += 1;
    return new SharedServerHandle(this, serverName, entry);
  }

  release(serverName: string): void {
    const entry = this.entries.get(serverName);
    if (entry === undefined) return;
    entry.refCount = Math.max(0, entry.refCount - 1);
  }

  forceRemove(serverName: string): void {
    this.entries.delete(serverName);
    this.failedServers.delete(serverName);
  }

  getRestartError(serverName: string): MCPRestartFailedError | undefined {
    return this.entries.get(serverName)?.restartError;
  }

  async shutdown(): Promise<void> {
    if (this.stopping) return;
    this.stopping = true;
    this.initializing.clear();
    const entries = Array.from(this.entries.values());
    const closers = entries.map(async (entry) => {
      const latch = entry.restartAttempt;
      if (latch !== undefined) {
        try { latch.resolve(false); } catch { /* ignore */ }
        entry.restartAttempt = undefined;
      }
      entry.restartPromise = undefined;
      entry.transportClosing = true;
      try { entry.client.onclose = undefined; } catch { /* ignore */ }
      try { await entry.client.close(); } catch { /* ignore */ }
      try {
        if (typeof entry.transport.close === 'function') {
          await entry.transport.close();
        }
      } catch { /* ignore */ }
      if (entry.pid !== null) {
        try {
          await killProcessTree(entry.pid, {
            gracefulMs: 500,
            logger: (message) => { entry.log('WRN', message, `mcp:${entry.serverName}`); },
          });
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          entry.log('WRN', `failed to terminate pid for '${entry.serverName}': ${msg}`, `mcp:${entry.serverName}`);
        }
      }
    });
    await Promise.allSettled(closers);
    this.entries.clear();
    this.failedServers.clear();
  }

  async handleTimeout(serverName: string, logger: LogFn): Promise<void> {
    if (this.stopping) return;
    const entry = this.entries.get(serverName);
    if (entry === undefined) return;
    const healthy = await this.runProbe(entry, logger).catch(() => false);
    if (healthy) {
      logger('VRB', `shared probe succeeded for '${serverName}'`, `mcp:${serverName}`);
      return;
    }
    logger('WRN', `shared probe failed for '${serverName}', scheduling restart sequence`, `mcp:${serverName}`);
    const firstAttemptSuccess = await this.startRestartLoop(entry, logger, 'probe-failure');
    if (!firstAttemptSuccess) {
      const err = entry.restartError ?? new MCPRestartFailedError(serverName, 'restart attempt failed');
      throw err;
    }
  }

  private attachClientLifecycleHandlers(entry: SharedServerEntry): void {
    entry.client.onclose = () => {
      void this.handleTransportExit(entry);
    };
  }

  private async handleTransportExit(entry: SharedServerEntry): Promise<void> {
    if (this.stopping) return;
    if (this.entries.get(entry.serverName) !== entry) {
      return;
    }
    if (entry.transportClosing) {
      entry.log('VRB', `shared transport closed for '${entry.serverName}' during managed restart`, `mcp:${entry.serverName}`);
      return;
    }
    entry.log('WRN', `shared transport closed for '${entry.serverName}', scheduling restart sequence`, `mcp:${entry.serverName}`);
    try {
      await this.startRestartLoop(entry, entry.log, 'transport-exit');
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      entry.log('ERR', `shared restart scheduling failed for '${entry.serverName}' after transport exit: ${message}`, `mcp:${entry.serverName}`, true);
    }
  }
  
  private createAttemptLatch(): { promise: Promise<boolean>; resolve: (value: boolean) => void } {
    let resolveFn!: (value: boolean) => void;
    const promise = new Promise<boolean>((resolve) => {
      resolveFn = resolve;
    });
    return { promise, resolve: resolveFn };
  }

  private async startRestartLoop(entry: SharedServerEntry, logger: LogFn, reason: string): Promise<boolean> {
    if (this.stopping) {
      const latch = entry.restartAttempt;
      if (latch !== undefined) {
        latch.resolve(false);
        entry.restartAttempt = undefined;
      }
      entry.restartPromise = undefined;
      return entry.restartError === undefined;
    }
    if (entry.restartPromise === undefined) {
      entry.restartAttempt = this.createAttemptLatch();
      entry.restartPromise = (async () => {
        try {
          await this.runRestartLoop(entry, logger, reason);
        } finally {
          entry.restartPromise = undefined;
        }
      })();
    }
    const latch = entry.restartAttempt;
    if (latch === undefined) {
      return entry.restartError === undefined;
    }
    return await latch.promise;
  }

  private runRestartLoop(entry: SharedServerEntry, logger: LogFn, reason: string): Promise<void> {
    return (async () => {
      let attempt = 0;
      // eslint-disable-next-line functional/no-loop-statements -- shared restart policy requires iterative retries.
      for (;;) {
        if (this.stopping) {
          const latch = entry.restartAttempt;
          if (latch !== undefined) {
            latch.resolve(false);
            entry.restartAttempt = undefined;
          }
          entry.restartPromise = undefined;
          return;
        }
        const backoffMs = SHARED_RESTART_BACKOFF_MS[Math.min(attempt, SHARED_RESTART_BACKOFF_MS.length - 1)];
        if (attempt > 0 && backoffMs > 0) {
          await delay(backoffMs);
        }
        const attemptLabel = attempt + 1;
        logger('WRN', `shared restart attempt ${String(attemptLabel)} for '${entry.serverName}' (${reason})`, `mcp:${entry.serverName}`);
        entry.restartError = undefined;
        try {
          await this.performRestartAttempt(entry, logger);
          logger('VRB', `shared restart succeeded for '${entry.serverName}' on attempt ${String(attemptLabel)}`, `mcp:${entry.serverName}`);
          const latch = entry.restartAttempt;
          if (latch !== undefined) {
            latch.resolve(true);
            entry.restartAttempt = undefined;
          }
          entry.restartError = undefined;
          return;
        } catch (error) {
          const err = error instanceof MCPRestartFailedError
            ? error
            : new MCPRestartFailedError(entry.serverName, error instanceof Error ? error.message : String(error));
          entry.restartError = err;
          const configDetails = formatMCPConfigForLog(entry.serverName, entry.config);
          logger('ERR', `shared MCP server restart failed (attempt ${String(attemptLabel)}): ${err.message} [${configDetails}]`, `mcp:${entry.serverName}`, true);
          const latch = entry.restartAttempt;
          if (latch !== undefined) {
            latch.resolve(false);
            entry.restartAttempt = undefined;
          }
          attempt += 1;
        }
      }
    })();
  }

  private async performRestartAttempt(entry: SharedServerEntry, logger: LogFn): Promise<void> {
    entry.transportClosing = true;
    try {
      if (entry.pid !== null) {
        await killProcessTree(entry.pid, { gracefulMs: 1000, logger: (message) => { logger('WRN', message, `mcp:${entry.serverName}`); } });
      }
    } catch (e) {
      logger('WRN', `failed to kill shared pid for '${entry.serverName}': ${e instanceof Error ? e.message : String(e)}`, `mcp:${entry.serverName}`);
    }
    try {
      await entry.client.close();
    } catch { /* ignore */ }
    try {
      if (typeof entry.transport.close === 'function') {
        await entry.transport.close();
      }
    } catch { /* ignore */ }
    try {
      await this.reinitializeEntry(entry, logger);
    } finally {
      entry.transportClosing = false;
    }
  }

  private async initializeEntry(serverName: string, config: MCPServerConfig, opts: SharedAcquireOptions): Promise<SharedServerEntry> {
    if (this.stopping) {
      throw new Error(`shared MCP registry is shutting down; cannot initialize '${serverName}'`);
    }
    let attempt = 0;
    // eslint-disable-next-line functional/no-loop-statements -- shared initialization retries until success per spec.
    for (;;) {
      if (attempt > 0) {
        const backoffMs = SHARED_RESTART_BACKOFF_MS[Math.min(attempt, SHARED_RESTART_BACKOFF_MS.length - 1)];
        const delayLabel = `${String(backoffMs)}ms`;
        const retryMsg = `shared server '${serverName}' initialization retry (attempt ${String(attempt + 1)}) in ${delayLabel}`;
        opts.log('WRN', retryMsg, `mcp:${serverName}`);
        if (backoffMs > 0) {
          await delay(backoffMs);
        }
      }
      try {
        return await this.tryInitializeEntry(serverName, config, opts);
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        const configDetails = formatMCPConfigForLog(serverName, config);
        opts.log('ERR', `shared MCP server initialization failed: ${msg} [${configDetails}]`, `mcp:${serverName}`, true);
        attempt += 1;
      }
    }
  }

  private async tryInitializeEntry(serverName: string, config: MCPServerConfig, opts: SharedAcquireOptions): Promise<SharedServerEntry> {
    const client = new Client(
      { name: 'ai-agent', version: '1.0.0' },
      { capabilities: { tools: {} }, ...(opts.requestTimeoutMs !== undefined ? { requestTimeoutMs: opts.requestTimeoutMs } : {}) } as unknown as Record<string, unknown>
    );
    let transport: Transport | undefined;
    let pid: number | null = null;
    try {
      const built = await this.createTransportForConfig(serverName, config, opts.log);
      transport = built.transport;
      pid = built.pid;
      await client.connect(transport);
      const server = await this.buildServerDescriptor(serverName, config, client, opts);
      const entry: SharedServerEntry = {
        serverName,
        config,
        client,
        transport,
        pid,
        server,
        refCount: 0,
        healthProbe: config.healthProbe ?? DEFAULT_HEALTH_PROBE,
        restartPromise: undefined,
        trace: opts.trace,
        verbose: opts.verbose,
        requestTimeoutMs: opts.requestTimeoutMs,
        log: opts.log,
        filterTools: opts.filterTools,
        transportClosing: false,
      };
      this.attachClientLifecycleHandlers(entry);
      return entry;
    } catch (error) {
      try { await client.close(); } catch { /* ignore */ }
      if (transport !== undefined && typeof (transport as { close?: () => Promise<void> }).close === 'function') {
        try { await (transport as { close: () => Promise<void> }).close(); } catch { /* ignore */ }
      }
      if (pid !== null) {
        try {
          await killProcessTree(pid, { gracefulMs: 1000, logger: (message) => { opts.log('WRN', message, `mcp:${serverName}`); } });
        } catch { /* ignore */ }
      }
      throw error;
    }
  }

  private async reinitializeEntry(entry: SharedServerEntry, logger: LogFn): Promise<void> {
    const client = new Client(
      { name: 'ai-agent', version: '1.0.0' },
      { capabilities: { tools: {} }, ...(entry.requestTimeoutMs !== undefined ? { requestTimeoutMs: entry.requestTimeoutMs } : {}) } as unknown as Record<string, unknown>
    );
    let transport: Transport | undefined;
    let pid: number | null = null;
    try {
      const built = await this.createTransportForConfig(entry.serverName, entry.config, logger);
      transport = built.transport;
      pid = built.pid;
      await client.connect(transport);
      const server = await this.buildServerDescriptor(entry.serverName, entry.config, client, {
        trace: entry.trace,
        verbose: entry.verbose,
        requestTimeoutMs: entry.requestTimeoutMs,
        log: entry.log,
        filterTools: entry.filterTools,
      });
      entry.client = client;
      entry.transport = transport;
      entry.pid = pid;
      entry.server = server;
      entry.restartError = undefined;
      entry.transportClosing = false;
      this.attachClientLifecycleHandlers(entry);
    } catch (error) {
      try { await client.close(); } catch { /* ignore */ }
      if (transport !== undefined && typeof (transport as { close?: () => Promise<void> }).close === 'function') {
        try { await (transport as { close: () => Promise<void> }).close(); } catch { /* ignore */ }
      }
      if (pid !== null) {
        try {
          await killProcessTree(pid, { gracefulMs: 1000, logger: (message) => { logger('WRN', message, `mcp:${entry.serverName}`); } });
        } catch { /* ignore */ }
      }
      throw error;
    }
  }

  private async createTransportForConfig(
    serverName: string,
    config: MCPServerConfig,
    logFn: LogFn
  ): Promise<{ transport: Transport; pid: number | null }> {
    switch (config.type) {
      case 'stdio': {
        const transport = createStdioTransport(serverName, config, logFn);
        return { transport, pid: transport.pid ?? null };
      }
      case 'websocket': {
        if (config.url == null || config.url.length === 0) {
          throw new Error(`WebSocket MCP server '${serverName}' requires a 'url'`);
        }
        const transport = await createWebSocketTransport(config.url, config.headers);
        return { transport, pid: null };
      }
      case 'http': {
        if (config.url == null || config.url.length === 0) {
          throw new Error(`HTTP MCP server '${serverName}' requires a 'url'`);
        }
        const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
        const reqInit: RequestInit = { headers: (config.headers ?? {}) as HeadersInit };
        const transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
        return { transport, pid: null };
      }
      case 'sse': {
        if (config.url == null || config.url.length === 0) {
          throw new Error(`SSE MCP server '${serverName}' requires a 'url'`);
        }
        const resolvedHeaders = config.headers ?? {};
        const customFetch: typeof fetch = async (input, init) => {
          const headers = new Headers(init?.headers);
          Object.entries(resolvedHeaders).forEach(([k, v]) => { headers.set(k, v); });
          return fetch(input, { ...init, headers });
        };
        // eslint-disable-next-line @typescript-eslint/no-deprecated -- SSE transport kept for backwards compatibility.
        const transport = new SSEClientTransport(new URL(config.url), {
          eventSourceInit: { fetch: customFetch },
          requestInit: { headers: resolvedHeaders as HeadersInit },
          fetch: customFetch,
        });
        return { transport, pid: null };
      }
      default:
        throw new Error(`Unsupported MCP transport: ${config.type as string}`);
    }
  }

  ensureReadyForCall(entry: SharedServerEntry): void {
    if (entry.restartPromise !== undefined) {
      if (entry.restartError !== undefined) {
        throw entry.restartError;
      }
      throw new MCPRestartInProgressError(entry.serverName, 'restart still running');
    }
    if (entry.restartError !== undefined) {
      throw entry.restartError;
    }
  }

  private async buildServerDescriptor(serverName: string, config: MCPServerConfig, client: Client, opts: SharedAcquireOptions): Promise<MCPServer> {
    const initInstructions = client.getInstructions() ?? '';
    if (opts.trace) {
      const len = initInstructions.length;
      opts.log('TRC', `instructions length for '${serverName}': ${String(len)}`, `mcp:${serverName}`);
    }
    const toolsResponse = await client.listTools();
    interface ToolItem { name: string; description?: string; inputSchema?: unknown; parameters?: unknown; instructions?: string }
    const rawTools: MCPTool[] = (toolsResponse.tools as ToolItem[]).map((t) => ({
      name: t.name,
      description: t.description ?? '',
      inputSchema: (t.inputSchema ?? t.parameters ?? {}) as Record<string, unknown>,
      instructions: t.instructions,
    }));
    const tools = opts.filterTools(rawTools);
    if (opts.trace) {
      const names = tools.map((t) => t.name).join(', ');
      opts.log('TRC', `listTools('${serverName}') -> ${String(tools.length)} tools [${names}]`, `mcp:${serverName}`);
    }
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
      if (opts.trace) {
        const msg = e instanceof Error ? e.message : String(e);
        opts.log('WRN', `listPrompts failed for '${serverName}': ${msg}`, `mcp:${serverName}`);
      }
    }
    return { name: serverName, config, tools, instructions };
  }

  private async runProbe(entry: SharedServerEntry, logger: LogFn): Promise<boolean> {
    const mode = entry.healthProbe;
    if (mode === 'ping' && typeof entry.client.ping === 'function') {
      try {
        await withTimeout(entry.client.ping(), PROBE_TIMEOUT_MS);
        return true;
      } catch {
        // fall through
      }
    }
    try {
      await withTimeout(entry.client.listTools(), PROBE_TIMEOUT_MS);
      return true;
    } catch (e) {
      logger('WRN', `shared probe exception for '${entry.serverName}': ${e instanceof Error ? e.message : String(e)}`, `mcp:${entry.serverName}`);
      return false;
    }
  }
}

function filterToolsForServer(serverName: string, config: MCPServerConfig, tools: MCPTool[], verbose: boolean, logFn: LogFn): MCPTool[] {
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
  if (verbose && initialCount !== filtered.length) {
    const removedNames = tools
      .filter((tool) => !filtered.includes(tool))
      .map((tool) => tool.name)
      .join(', ');
    const msg = removedNames.length > 0
      ? `filtered tools for '${serverName}': removed [${removedNames}]`
      : `filtered tools for '${serverName}': no tools removed`;
    logFn('VRB', msg, `mcp:${serverName}`);
  }
  return filtered;
}

function createStdioTransport(name: string, config: MCPServerConfig, logFn: LogFn): StdioClientTransport {
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
          logFn('ERR', `stderr '${name}': ${s.trim()}`, `mcp:${name}`);
        } catch (e) { warn(`mcp stdio stderr relay failed: ${e instanceof Error ? e.message : String(e)}`); }
      });
    }
  } catch (e) { warn(`mcp stdio transport setup failed: ${e instanceof Error ? e.message : String(e)}`); }
  return transport;
}

const delay = (ms: number): Promise<void> => new Promise((resolve) => {
  if (ms <= 0) {
    resolve();
    return;
  }
  setTimeout(resolve, ms);
});

async function withTimeout<T>(promise: Promise<T>, timeoutMs: number): Promise<T> {
  let timeout: NodeJS.Timeout | undefined;
  const timerPromise = new Promise<never>((_, reject) => {
    timeout = setTimeout(() => {
      reject(new Error('timeout'));
    }, timeoutMs);
  });
  try {
    return await Promise.race([promise, timerPromise]);
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
  }
}

const defaultSharedRegistry: SharedRegistry = new MCPSharedRegistry();

export const shutdownSharedRegistry = async (): Promise<void> => {
  if (typeof (defaultSharedRegistry as MCPSharedRegistry).shutdown === 'function') {
    await (defaultSharedRegistry as MCPSharedRegistry).shutdown();
  }
};

export const forceRemoveSharedRegistryEntry = (serverName: string): void => {
  (defaultSharedRegistry as MCPSharedRegistry).forceRemove(serverName);
};

export class MCPProvider extends ToolProvider {
  readonly kind = 'mcp' as const;
  private readonly serversConfig: Record<string, MCPServerConfig>;
  private clients = new Map<string, Client>();
  private servers = new Map<string, MCPServer>();
  private failedServers = new Set<string>();
  private processes = new Map<string, MCPProcessHandle>();
  private sharedHandles = new Map<string, SharedRegistryHandle>();
  private serverInitPromises = new Map<string, Promise<void>>();
  private toolNameMap = new Map<string, { serverName: string; originalName: string }>();
  private toolQueue = new Map<string, string>();
  private initialized = false;
  private initializationPromise?: Promise<void>;
  private trace = false;
  private verbose = false;
  private requestTimeoutMs?: number;
  private onLog?: (entry: LogEntry) => void;
  private initConcurrency?: number;
  private readonly sharedRegistry: SharedRegistry;

  constructor(
    public readonly namespace: string,
    servers: Record<string, MCPServerConfig>,
    opts?: { trace?: boolean; verbose?: boolean; requestTimeoutMs?: number; onLog?: (entry: LogEntry) => void; initConcurrency?: number; sharedRegistry?: SharedRegistry }
  ) {
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
    this.sharedRegistry = opts?.sharedRegistry ?? defaultSharedRegistry;
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

  private shouldShareServer(config: MCPServerConfig): boolean {
    return config.shared !== false;
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
          const configDetails = formatMCPConfigForLog(name, config);
          this.log('ERR', `failed to initialize MCP server: ${msg} [${configDetails}]`, `mcp:${name}`, true);
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

  private async initializeServer(name: string, config: MCPServerConfig): Promise<MCPServer> {
    if (this.shouldShareServer(config)) {
      const handle = await this.sharedRegistry.acquire(name, config, {
        trace: this.trace,
        verbose: this.verbose,
        requestTimeoutMs: this.requestTimeoutMs,
        log: (severity, message, remoteIdentifier, fatal = false) => {
          this.log(severity, message, remoteIdentifier, fatal);
        },
        filterTools: (raw) => this.filterToolsForServer(name, config, raw),
      });
      this.sharedHandles.set(name, handle);
      const server = handle.server;
      this.servers.set(name, server);
      const ns = this.sanitizeNamespace(name);
      const queueName = typeof config.queue === 'string' && config.queue.length > 0 ? config.queue : 'default';
      server.tools.forEach((t) => {
        const exposed = `${ns}__${t.name}`;
        this.toolNameMap.set(exposed, { serverName: name, originalName: t.name });
        this.toolQueue.set(exposed, queueName);
      });
      return server;
    }

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
        // eslint-disable-next-line @typescript-eslint/no-deprecated -- SSE client transport kept for backwards compatibility with legacy MCP servers
        transport = new SSEClientTransport(new URL(config.url), { eventSourceInit: { fetch: customFetch }, requestInit: { headers: resolvedHeaders as HeadersInit }, fetch: customFetch });
        break;
      }
      default:
        throw new Error('Unsupported transport type');
    }
    try {
      await client.connect(transport);
      if (transport instanceof StdioClientTransport && config.type === 'stdio') {
        this.trackProcessHandle(name, transport, config);
      } else {
        this.processes.delete(name);
      }
      this.log('TRC', `connected to '${name}'`, `mcp:${name}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      const configDetails = formatMCPConfigForLog(name, config);
      this.log('ERR', `MCP server connect failed: ${msg} [${configDetails}]`, `mcp:${name}`, true);
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
    const queueName = typeof config.queue === 'string' && config.queue.length > 0 ? config.queue : 'default';
    tools.forEach((t) => {
      const exposed = `${ns}__${t.name}`;
      this.toolNameMap.set(exposed, { serverName: name, originalName: t.name });
      this.toolQueue.set(exposed, queueName);
    });

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

  private trackProcessHandle(name: string, transport: StdioClientTransport, config: MCPServerConfig): void {
    const pid = transport.pid;
    if (pid === null) return;
    const args = Array.isArray(config.args) ? [...config.args] : [];
    this.processes.set(name, { pid, command: config.command ?? '<unknown>', args, startedAt: Date.now() });
    if (this.trace) {
      const cmd = `${config.command ?? '<unknown>'} ${args.join(' ')}`.trim();
      this.log('TRC', `tracking stdio pid=${pid.toString()} '${cmd}'`, `mcp:${name}`);
    }
  }

  filterToolsForServer(name: string, config: MCPServerConfig, tools: MCPTool[]): MCPTool[] {
    return filterToolsForServer(name, config, tools, this.verbose, (severity, message, remoteIdentifier) => {
      this.log(severity, message, remoteIdentifier);
    });
  }

  createStdioTransport(name: string, config: MCPServerConfig): StdioClientTransport {
    return createStdioTransport(name, config, (severity, message, remoteIdentifier, fatal = false) => {
      this.log(severity, message, remoteIdentifier, fatal);
    });
  }

  private hasServerConfig(name: string): boolean {
    return Object.prototype.hasOwnProperty.call(this.serversConfig, name);
  }

  private log(severity: 'VRB' | 'WRN' | 'ERR' | 'TRC', message: string, remoteIdentifier: string, fatal = false): void {
    const entry: LogEntry = {
      timestamp: Date.now(), severity, turn: 0, subturn: 0, direction: 'response', type: 'tool', toolKind: 'mcp', remoteIdentifier, fatal, message,
    };
    try { this.onLog?.(entry); } catch (e) { warn(`mcp onLog failed: ${e instanceof Error ? e.message : String(e)}`); }
  }

  private async closeClient(serverName: string): Promise<void> {
    const client = this.clients.get(serverName);
    if (client === undefined) return;
    try {
      await client.close();
    } catch (e) {
      warn(`mcp client close failed for '${serverName}': ${e instanceof Error ? e.message : String(e)}`);
    }
    this.clients.delete(serverName);
  }

  private async killTrackedProcess(serverName: string, reason: string): Promise<void> {
    const handle = this.processes.get(serverName);
    if (handle === undefined) return;
    this.log('WRN', `terminating '${serverName}' pid=${handle.pid.toString()} (${reason})`, `mcp:${serverName}`);
    try {
      const outcome = await killProcessTree(handle.pid, {
        gracefulMs: 2000,
        logger: (msg) => { if (this.trace) this.log('TRC', msg, `mcp:${serverName}`); }
      });
      if (outcome.stillRunning.length > 0) {
        this.log('ERR', `pid(s) still alive after kill for '${serverName}': ${outcome.stillRunning.join(', ')}`, `mcp:${serverName}`);
      }
    } catch (e) {
      this.log('ERR', `failed to terminate '${serverName}': ${e instanceof Error ? e.message : String(e)}`, `mcp:${serverName}`);
    } finally {
      this.processes.delete(serverName);
    }
  }

  private async restartServer(serverName: string, reason: string): Promise<void> {
    if (!this.hasServerConfig(serverName)) return;
    await this.closeClient(serverName);
    await this.killTrackedProcess(serverName, reason);
    await this.ensureServerReady(serverName);
  }

  private async ensureServerReady(serverName: string): Promise<void> {
    if (this.sharedHandles.has(serverName)) return;
    if (this.clients.has(serverName)) return;
    let pending = this.serverInitPromises.get(serverName);
    if (pending !== undefined) {
      await pending;
      return;
    }
    if (!this.hasServerConfig(serverName)) {
      throw new Error(`No MCP server config found for '${serverName}'`);
    }
    const config = this.serversConfig[serverName];
    const promise = (async () => { await this.initializeServer(serverName, config); })();
    this.serverInitPromises.set(serverName, promise);
    try {
      await promise;
    } finally {
      this.serverInitPromises.delete(serverName);
    }
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

  override resolveQueueName(name: string): string | undefined {
    return this.toolQueue.get(name);
  }

  override async cancelTool(name: string, opts?: ToolCancelOptions): Promise<void> {
    const mapping = this.toolNameMap.get(name);
    if (mapping === undefined) return;
    if (!this.hasServerConfig(mapping.serverName)) return;
    const config = this.serversConfig[mapping.serverName];
    const reason = opts?.reason ?? 'timeout';
    const sharedHandle = this.sharedHandles.get(mapping.serverName);
    if (sharedHandle !== undefined) {
      await sharedHandle.handleCancel(reason, (severity, message, remoteIdentifier, fatal = false) => {
        this.log(severity, message, remoteIdentifier, fatal);
      });
      return;
    }
    if (config.type !== 'stdio' || reason !== 'timeout') return;
    try {
      await this.restartServer(mapping.serverName, reason);
    } catch (e) {
      this.log('ERR', `failed to restart '${mapping.serverName}' after ${reason}: ${e instanceof Error ? e.message : String(e)}`, `mcp:${mapping.serverName}`);
    }
  }

  async execute(name: string, parameters: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    await this.ensureInitialized();
    const mapping = this.toolNameMap.get(name);
    if (mapping === undefined) throw new Error(`No server found for tool: ${name}`);
    const { serverName, originalName } = mapping;
    const sanitizedNamespace = name.includes('__') ? name.split('__')[0] : this.sanitizeNamespace(serverName);
    const sharedHandle = this.sharedHandles.get(serverName);
    if (sharedHandle === undefined) {
      let client = this.clients.get(serverName);
      if (client === undefined) {
        await this.ensureServerReady(serverName);
        client = this.clients.get(serverName);
      }
      if (client === undefined) {
        throw new Error(`MCP server not ready: ${serverName}`);
      }
      const resolvedClient = client;
      const start = Date.now();
      const requestOptions = ((): Record<string, unknown> | undefined => {
        const t = _opts?.timeoutMs;
        const has = typeof t === 'number' && Number.isFinite(t) && t > 0;
        return has ? { timeout: Math.trunc(t), resetTimeoutOnProgress: true, maxTotalTimeout: Math.trunc(t) } : undefined;
      })();
      const res = await resolvedClient.callTool({ name: originalName, arguments: parameters }, undefined, requestOptions);
      return this.normalizeToolResult(res, parameters, sanitizedNamespace, start);
    }

    const start = Date.now();
    const requestOptions = ((): Record<string, unknown> | undefined => {
      const t = _opts?.timeoutMs;
      const has = typeof t === 'number' && Number.isFinite(t) && t > 0;
      return has ? { timeout: Math.trunc(t), resetTimeoutOnProgress: true, maxTotalTimeout: Math.trunc(t) } : undefined;
    })();
    try {
      const res = await sharedHandle.callTool(originalName, parameters, requestOptions);
      return this.normalizeToolResult(res, parameters, sanitizedNamespace, start);
    } catch (error) {
      const restartError = typeof this.sharedRegistry.getRestartError === 'function'
        ? this.sharedRegistry.getRestartError(serverName)
        : undefined;
      if (restartError !== undefined) {
        throw restartError;
      }
      throw error;
    }
  }

  private normalizeToolResult(res: unknown, parameters: Record<string, unknown>, namespace: string, start: number): ToolExecuteResult {
    const latency = Date.now() - start;
    const isRecord = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
    const text = (() => {
      let content: unknown;
      if (isRecord(res) && Object.prototype.hasOwnProperty.call(res, 'content')) {
        content = res.content;
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
      namespace,
      extras: { rawResponse: rawJson, rawRequest: rawReq }
    };
  }

  getCombinedInstructions(): string {
    const segments: string[] = [];
    this.servers.forEach((server, serverKey) => {
      const ns = this.sanitizeNamespace(serverKey);
      const blocks: string[] = [];
      const displayName = server.name || serverKey;
      blocks.push(`#### ${displayName}`);
      let hasInstructions = false;
      if (typeof server.instructions === 'string') {
        const trimmedServer = server.instructions.trim();
        if (trimmedServer.length > 0) {
          blocks.push(trimmedServer);
          hasInstructions = true;
        }
      }
      server.tools.forEach((tool) => {
        if (typeof tool.instructions !== 'string') return;
        const trimmedTool = tool.instructions.trim();
        if (trimmedTool.length === 0) return;
        const exposed = `${ns}__${tool.name}`;
        blocks.push(`${exposed}: ${trimmedTool}`);
        hasInstructions = true;
      });
      if (!hasInstructions) {
        blocks.push('No specific instructions for this tool provider, use the tool schemas to derive usage.');
      }
      segments.push(blocks.join('\n\n'));
    });
    return segments.join('\n\n');
  }

  async cleanup(): Promise<void> {
    await Promise.all(Array.from(this.clients.entries()).map(async ([serverName, client]) => {
      try {
        await client.close();
      } catch (e) {
        warn(`mcp client close failed for '${serverName}': ${e instanceof Error ? e.message : String(e)}`);
      }
    }));
    Array.from(this.sharedHandles.entries()).forEach(([serverName, handle]) => {
      try {
        handle.release();
      } catch (e) {
        warn(`shared mcp handle release failed for '${serverName}': ${e instanceof Error ? e.message : String(e)}`);
      }
    });
    const serversWithProcesses = Array.from(this.processes.keys());
    await Promise.all(serversWithProcesses.map(async (serverName) => {
      await this.killTrackedProcess(serverName, 'cleanup');
    }));
    this.clients.clear();
    this.sharedHandles.clear();
    this.processes.clear();
    this.servers.clear();
    this.toolNameMap.clear();
    this.toolQueue.clear();
    this.serverInitPromises.clear();
    this.initialized = false;
  }
}
