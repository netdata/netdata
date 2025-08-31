import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

import type { MCPServerConfig } from './types.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';
import type { ChildProcess } from 'node:child_process';

import { createWebSocketTransport } from './websocket-transport.js';






export interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  instructions?: string;
}

export interface MCPServer {
  name: string;
  config: MCPServerConfig;
  tools: MCPTool[];
  instructions: string;
}

export interface ToolCall {
  id: string;
  name: string;
  parameters: Record<string, unknown>;
}

export interface ToolResult {
  toolCallId: string;
  result: string;
  success: boolean;
  error?: string;
  metadata?: {
    latency: number;
    charactersIn: number;
    charactersOut: number;
    mcpServer: string;
    command: string;
  }
}

export class MCPClientManager {
  private clients = new Map<string, Client>();
  private processes = new Map<string, ChildProcess>();
  private servers = new Map<string, MCPServer>();
  private trace = false;
  private verbose = false;
  private logger: (msg: string) => void = (_msg: string) => { return; };
  private traceLog(message: string) {
    try { process.stderr.write(`[mcp] ${message}\n`); } catch {}
  }
  private colorVerbose(text: string): string {
    return process.stderr.isTTY ? `\x1b[90m${text}\x1b[0m` : text;
  }

  constructor(opts?: { trace?: boolean; verbose?: boolean; logger?: (msg: string) => void }) {
    this.trace = opts?.trace === true;
    this.verbose = opts?.verbose === true;
    if (typeof opts?.logger === 'function') this.logger = opts.logger;
  }

  private colorError(text: string): string {
    return process.stderr.isTTY ? `\x1b[31m${text}\x1b[0m` : text;
  }

  async initializeServers(mcpServers: Record<string, MCPServerConfig>): Promise<MCPServer[]> {
    const entries = Object.entries(mcpServers);
    const servers = await Promise.all(entries.map(async ([name, config]) => {
      const server = await this.initializeServer(name, config);
      this.servers.set(name, server);
      return server;
    }));
    return servers;
  }

  private async initializeServer(name: string, config: MCPServerConfig): Promise<MCPServer> {
    const client = new Client({ name: 'ai-agent', version: '1.0.0' }, { capabilities: { tools: {} } });

    let transport: Transport;
    const initStart = Date.now();
    if (this.verbose) { try { process.stderr.write(this.colorVerbose(`agent → [mcp] connect ${name}\n`)); } catch {} }
    if (this.trace) this.traceLog(`connect '${name}' type=${config.type} ${config.command ?? config.url ?? ''}`);
    switch (config.type) {
      case 'stdio':
        transport = this.createStdioTransport(name, config);
        break;
      case 'websocket':
        transport = await createWebSocketTransport(config.url ?? '', resolveHeaders(config.headers));
        break;
      case 'http': {
        if (config.url == null || config.url.length === 0) throw new Error(`HTTP MCP server '${name}' requires a 'url'`);
        const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
        const reqInit: RequestInit = { headers: resolveHeaders(config.headers) as HeadersInit };
        transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
        break;
      }
      case 'sse':
        if (config.url == null || config.url.length === 0) throw new Error(`SSE MCP server '${name}' requires a 'url'`);
        transport = new SSEClientTransport(new URL(config.url), resolveHeaders(config.headers));
        break;
      default:
        throw new Error('Unsupported transport type');
    }

    try {
      const connTransport: Transport = transport;
      await client.connect(connTransport);
    } catch (e) {
      const em = e instanceof Error ? e.message : String(e);
      try { process.stderr.write(this.colorError(`agent × [mcp] connect ${name} -> ${em}\n`)); } catch {}
      throw e;
    }
    this.clients.set(name, client);

    const initInstructions = client.getInstructions() ?? '';

    const toolsStart = Date.now();
    if (this.verbose) { try { process.stderr.write(this.colorVerbose(`agent → [mcp] tools/list ${name}\n`)); } catch {} }
    if (this.trace) this.traceLog(`request '${name}': tools/list`);
    let toolsResponse;
    try {
      toolsResponse = await client.listTools();
    } catch (e) {
      const em = e instanceof Error ? e.message : String(e);
      try { process.stderr.write(this.colorError(`agent × [mcp] tools/list ${name} -> ${em}\n`)); } catch {}
      throw e;
    }
    if (this.trace) this.traceLog(`response '${name}': tools/list -> ${JSON.stringify(toolsResponse, null, 2)}`);
    interface ToolItem { name: string; description?: string; inputSchema?: unknown; parameters?: unknown; instructions?: string }
    const tools: MCPTool[] = (toolsResponse.tools as ToolItem[]).map((t) => ({
      name: t.name,
      description: t.description ?? '',
      inputSchema: (t.inputSchema ?? t.parameters ?? {}) as Record<string, unknown>,
      instructions: t.instructions,
    }));

    let instructions = initInstructions;
    try {
      const promptsStart = Date.now();
      if (this.verbose) { try { process.stderr.write(this.colorVerbose(`agent → [mcp] prompts/list ${name}\n`)); } catch {} }
      if (this.trace) this.traceLog(`request '${name}': prompts/list`);
      let promptsResponse;
      try {
        promptsResponse = await client.listPrompts();
      } catch (e) {
        const em = e instanceof Error ? e.message : String(e);
      try { process.stderr.write(this.colorError(`agent × [mcp] prompts/list ${name} -> ${em}\n`)); } catch {}
        throw e;
      }
      if (this.trace) this.traceLog(`response '${name}': prompts/list -> ${JSON.stringify(promptsResponse, null, 2)}`);
      const pr = promptsResponse as { prompts?: { name: string; description?: string }[] } | undefined;
      const listRaw = pr?.prompts;
      const list = Array.isArray(listRaw) ? listRaw : [];
      if (list.length > 0) {
        const promptText = list.map((p) => `${p.name}: ${p.description ?? ''}`).join('\n');
        { const hasInstr = instructions.length > 0; instructions = hasInstr ? `${instructions}\n${promptText}` : promptText; }
      }
      if (this.verbose) { try { process.stderr.write(this.colorVerbose(`agent ← [mcp] prompts/list ${name}, prompts ${list.length}, latency ${Date.now() - promptsStart} ms\n`)); } catch {} }
    } catch {}

    if (this.verbose) {
      try {
        const latency = Date.now() - toolsStart;
        const toolCount = (toolsResponse as { tools?: unknown[] }).tools?.length ?? tools.length;
        process.stderr.write(this.colorVerbose(`agent ← [mcp] tools/list ${name}, tools ${toolCount}, latency ${latency} ms\n`));
      } catch {}
    }

    if (this.verbose) {
      try {
        const toolNames = tools.map((t) => t.name).join(', ');
        const schemaChars = tools.reduce((acc, t) => acc + JSON.stringify(t.inputSchema ?? {}).length, 0);
        const instrChars = (initInstructions?.length ?? 0) + tools.reduce((acc, t) => acc + (t.instructions?.length ?? 0), 0);
        const hasInstr = (initInstructions?.length ?? 0) > 0 || tools.some((t) => (t.instructions?.length ?? 0) > 0);
        const latency = Date.now() - initStart;
        process.stderr.write(this.colorVerbose(`agent ← [mcp] initialized ${name}, ${tools.length} tools (${toolNames || '-'})${hasInstr ? ', with instructions' : ', without instructions'} (schema ${schemaChars} chars, instructions ${instrChars} chars), latency ${latency} ms\n`));
      } catch {}
    }

    if (this.trace) this.traceLog(`ready '${name}' with ${tools.length.toString()} tools`);
    return { name, config, tools, instructions };
  }
  private createStdioTransport(name: string, config: MCPServerConfig): StdioClientTransport {
    if (typeof config.command !== 'string' || config.command.length === 0) throw new Error(`Stdio MCP server '${name}' requires a string 'command'`);

    const configured = config.env ?? {};
    const effectiveEnv: Record<string, string> = {};
    Object.entries(configured).forEach(([k, v]) => {
      const raw = v;
      const resolved = raw.replace(/\$\{([^}]+)\}/g, (_m: string, varName: string) => (process.env as Record<string, string | undefined>)[varName] ?? '');
      if (resolved.length > 0) effectiveEnv[k] = resolved;
    });

    const transport = new StdioClientTransport({
      command: config.command,
      args: config.args ?? [],
      env: effectiveEnv,
      stderr: 'pipe',
    });
    try {
      if (transport.stderr != null) {
        transport.stderr.on('data', (d: Buffer) => {
          const line = d.toString('utf8').trimEnd();
          if (this.trace) this.traceLog(`stderr '${name}': ${line}`);
        });
      }
    } catch {}
    return transport;
  }
  async executeTool(serverName: string, toolCall: ToolCall, timeoutMs?: number): Promise<ToolResult> {
    const client = this.clients.get(serverName);
    if (client == null) throw new Error(`MCP server '${serverName}' not initialized`);
    const start = Date.now();
    const argsStr = JSON.stringify(toolCall.parameters);
    const call = async () => {
      if (this.trace) this.traceLog(`callTool '${serverName}': ${toolCall.name}(${JSON.stringify(toolCall.parameters)})`);
      const res = await client.callTool({ name: toolCall.name, arguments: toolCall.parameters });
      if (this.trace) this.traceLog(`result '${serverName}': ${toolCall.name} -> ${JSON.stringify(res, null, 2)}`);
      return res;
    };
    const withTimeout = async <T>(p: Promise<T>): Promise<T> => {
      if (typeof timeoutMs !== 'number' || timeoutMs <= 0) return p;
      const timeoutPromise = new Promise<T>((_resolve, reject) => { setTimeout(() => { reject(new Error('Tool execution timed out')); }, timeoutMs); });
      return (await Promise.race([p, timeoutPromise])) as T;
    };
    try {
      const resp = await withTimeout(call());
      let result = '';
      interface Block { type: string; text?: string }
      const content: Block[] = (resp as unknown as { content?: Block[] }).content ?? [];
      result = content.map((c) => (c.type === 'text' ? c.text ?? '' : c.type === 'image' ? '[Image]' : `[${c.type}]`)).join('');
      const isError = (resp as { isError?: boolean }).isError === true;
      if (isError) {
        try { process.stderr.write(this.colorError(`agent × [mcp] callTool ${serverName} ${toolCall.name} -> tool reported isError` + '\n')); } catch {}
      }
      // Verbose req/res one-liners for tools are logged by ai-agent to avoid duplication
      return {
        toolCallId: toolCall.id,
        result,
        success: !isError,
        error: isError ? 'Tool execution failed' : undefined,
        metadata: {
          latency: Date.now() - start,
          charactersIn: argsStr.length,
          charactersOut: result.length,
          mcpServer: serverName,
          command: toolCall.name,
        },
      };
    } catch (err) {
      // ai-agent logs the outbound call; only emit unconditional red error here
      try { process.stderr.write(this.colorError(`agent × [mcp] callTool ${serverName} ${toolCall.name} -> ${err instanceof Error ? err.message : String(err)}\n`)); } catch {}
      return {
        toolCallId: toolCall.id,
        result: '',
        success: false,
        error: err instanceof Error ? err.message : String(err),
        metadata: {
          latency: Date.now() - start,
          charactersIn: argsStr.length,
          charactersOut: 0,
          mcpServer: serverName,
          command: toolCall.name,
        },
      };
    }
  }

  async executeTools(
    toolCalls: { serverName: string; toolCall: ToolCall }[],
    maxConcurrent?: number | null,
    timeoutMs?: number,
  ): Promise<ToolResult[]> {
    if (typeof maxConcurrent === 'number' && maxConcurrent > 0) return this.executeWithConcurrency(toolCalls, maxConcurrent, timeoutMs);
    return Promise.all(toolCalls.map(({ serverName, toolCall }) => this.executeTool(serverName, toolCall, timeoutMs)));
  }

  private async executeWithConcurrency(
    toolCalls: { serverName: string; toolCall: ToolCall }[],
    maxConcurrent: number,
    timeoutMs?: number,
  ): Promise<ToolResult[]> {
    const results: ToolResult[] = new Array<ToolResult>(toolCalls.length);
    let cursor = 0;
    const runWorker = async (): Promise<void> => {
      const i = cursor;
      cursor += 1;
      if (i >= toolCalls.length) return;
      const { serverName, toolCall } = toolCalls[i];
      results[i] = await this.executeTool(serverName, toolCall, timeoutMs);
      await runWorker();
    };
    const workers = Array.from({ length: Math.min(maxConcurrent, toolCalls.length) }, () => { void runWorker(); });
    await Promise.all(workers);
    return results;
  }

  getAllTools(): MCPTool[] {
    return Array.from(this.servers.values()).flatMap((s) => s.tools);
  }

  getCombinedInstructions(): string {
    return Array.from(this.servers.values()).flatMap((s) => {
      const arr: string[] = [];
      if (typeof s.instructions === 'string' && s.instructions.length > 0) arr.push(`## TOOL ${s.name} INSTRUCTIONS\n${s.instructions}`);
      s.tools.forEach((t) => { if (typeof t.instructions === 'string' && t.instructions.length > 0) arr.push(`## TOOL ${t.name} INSTRUCTIONS\n${t.instructions}`); });
      return arr;
    }).join('\n\n');
  }

  getToolServerMapping(): Map<string, string> {
    const entries = Array.from(this.servers.entries()).flatMap(([serverName, s]) => s.tools.map((t) => [t.name, serverName] as const));
    return new Map(entries);
  }

  async cleanup(): Promise<void> {
    await Promise.all(Array.from(this.clients.values()).map(async (c) => { try { await c.close(); } catch { /* noop */ } }));
    await Promise.all(Array.from(this.processes.values()).map((proc) => { try { proc.kill('SIGTERM'); setTimeout(() => { if (!proc.killed) proc.kill('SIGKILL'); }, 5000); } catch { /* noop */ } }));
    this.clients.clear();
    this.processes.clear();
    this.servers.clear();
  }
}

function resolveHeaders(headers?: Record<string, string>): Record<string, string> | undefined {
  if (headers === undefined) return headers;
  return Object.entries(headers).reduce<Record<string, string>>((acc, [k, v]) => {
    const raw = v;
    const resolved = raw.replace(/\$\{([^}]+)\}/g, (_m: string, name: string) => (process.env as Record<string, string | undefined>)[name] ?? '');
    if (resolved.length > 0) acc[k] = resolved;
    return acc;
  }, {});
}
