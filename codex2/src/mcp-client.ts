import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

import type { LogEntry, MCPServerConfig } from './types.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';

import { createWebSocketTransport } from './websocket-transport.js';

export interface MCPToolDef {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  instructions?: string;
}

export type ToolStatus =
  | { type: 'success' }
  | { type: 'mcp_server_error'; serverName: string; message: string }
  | { type: 'tool_not_found'; toolName: string; serverName: string }
  | { type: 'invalid_parameters'; toolName: string; message: string }
  | { type: 'execution_error'; toolName: string; message: string }
  | { type: 'timeout'; toolName: string; timeoutMs: number }
  | { type: 'connection_error'; serverName: string; message: string };

export interface MCPToolResult {
  toolCallId: string;
  status: ToolStatus;
  result: string;
  latencyMs: number;
  metadata: { mcpServer: string; command: string; inSize: number; outSize: number };
}

export interface ToolCallLike { id: string; name: string; parameters: Record<string, unknown> }

export class MCPClientManager {
  private clients = new Map<string, Client>();
  private servers = new Map<string, { name: string; config: MCPServerConfig; tools: MCPToolDef[]; instructions: string }>();

  constructor(private log?: (entry: LogEntry) => void, private trace = false, private verbose = false) {}

  private vrb(turn: number, subturn: number, dir: 'request'|'response', server: string, msg: string): void {
    if (!this.verbose) return;
    this.log?.({ timestamp: Date.now(), severity: 'VRB', turn, subturn, direction: dir, type: 'mcp', remoteIdentifier: server, fatal: false, message: msg });
  }
  private trc(turn: number, subturn: number, dir: 'request'|'response', server: string, msg: string): void {
    if (!this.trace) return;
    this.log?.({ timestamp: Date.now(), severity: 'TRC', turn, subturn, direction: dir, type: 'mcp', remoteIdentifier: server, fatal: false, message: msg });
  }
  private err(turn: number, subturn: number, server: string, msg: string, fatal = false): void {
    this.log?.({ timestamp: Date.now(), severity: 'ERR', turn, subturn, direction: 'response', type: 'mcp', remoteIdentifier: server, fatal, message: msg });
  }

  async initializeServers(defs: Record<string, MCPServerConfig>, turn = 0): Promise<void> {
    const entries = Object.entries(defs).filter(([, cfg]) => cfg.enabled !== false);
    await Promise.all(entries.map(([name, cfg]) => this.initializeServer(name, cfg, turn)));
  }

  private async initializeServer(name: string, config: MCPServerConfig, turn = 0): Promise<void> {
    this.vrb(turn, 0, 'request', name, 'connect ' + name);
    this.trc(turn, 0, 'request', name, "connect '" + name + "' type=" + config.type + ' ' + (config.command ?? config.url ?? ''));

    const client = new Client({ name: 'ai-agent', version: '1.0.0' }, { capabilities: { tools: {} } });
    let transport: Transport;
    switch (config.type) {
      case 'stdio': {
        if (typeof config.command !== 'string' || config.command.length === 0) throw new Error(`Stdio MCP server '${name}' requires a 'command'`);
        const env: Record<string, string> = {};
        Object.entries(config.env ?? {}).forEach(([k, v]) => { env[k] = v.replace(/\$\{([^}]+)\}/g, (_m: string, n: string) => process.env[n] ?? ''); });
        const stdio = new StdioClientTransport({ command: config.command, args: config.args ?? [], env, stderr: 'pipe' });
        try { if (stdio.stderr != null) stdio.stderr.on('data', (d: Buffer) => { const line = d.toString('utf-8').trimEnd(); this.trc(0, 0, 'response', name, 'stderr: ' + line); }); } catch { /* noop */ }
        transport = stdio;
        break;
      }
      case 'websocket': {
        const wsUrl = typeof config.url === 'string' ? config.url : '';
        transport = await createWebSocketTransport(wsUrl, this.resolveHeaders(config.headers));
        break;
      }
      case 'http': {
        if (typeof config.url !== 'string' || config.url.length === 0) throw new Error(`HTTP MCP server '${name}' requires a 'url'`);
        const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
        const reqInit: RequestInit = { headers: (this.resolveHeaders(config.headers) ?? {}) as HeadersInit };
        transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
        break;
      }
      case 'sse': {
        if (typeof config.url !== 'string' || config.url.length === 0) throw new Error(`SSE MCP server '${name}' requires a 'url'`);
        transport = new SSEClientTransport(new URL(config.url), this.resolveHeaders(config.headers));
        break;
      }
      default:
        throw new Error(`Unsupported MCP transport: ${config.type as string}`);
    }

    await client.connect(transport);
    this.clients.set(name, client);

    const getInstr = (c: unknown): string => {
      try {
        const fn = (c as { getInstructions?: () => string }).getInstructions;
        if (typeof fn === 'function') {
          const val = fn();
          return typeof val === 'string' ? val : '';
        }
        return '';
      } catch { return ''; }
    };
    const initInstr = getInstr(client);

    // tools/list
    this.vrb(turn, 0, 'request', name, 'tools/list ' + name);
    this.trc(turn, 0, 'request', name, 'tools/list');
    const toolsResp = await client.listTools();
    this.trc(turn, 0, 'response', name, 'tools/list -> ' + JSON.stringify(toolsResp, null, 2));
    interface ToolRaw { name?: unknown; description?: unknown; inputSchema?: unknown; parameters?: unknown; instructions?: unknown }
    const rawTools = Array.isArray((toolsResp as { tools?: unknown }).tools) ? ((toolsResp as { tools?: unknown[] }).tools ?? []) : [];
    const tools: MCPToolDef[] = rawTools.map((t: ToolRaw) => ({
      name: typeof t.name === 'string' ? t.name : '',
      description: typeof t.description === 'string' ? t.description : '',
      inputSchema: (t.inputSchema ?? t.parameters ?? { type: 'object' }) as Record<string, unknown>,
      instructions: typeof t.instructions === 'string' ? t.instructions : undefined,
    }));

    // prompts/list optional
    let instructions = initInstr;
    try {
      this.vrb(turn, 0, 'request', name, 'prompts/list ' + name);
      this.trc(turn, 0, 'request', name, 'prompts/list');
      const prompts = await client.listPrompts();
      this.trc(turn, 0, 'response', name, 'prompts/list -> ' + JSON.stringify(prompts, null, 2));
      const arr = Array.isArray((prompts as { prompts?: unknown }).prompts) ? (((prompts as { prompts?: unknown[] }).prompts) ?? []) as { name?: unknown; description?: unknown }[] : [];
      if (arr.length) {
        const promptText = arr.map((p) => (typeof p.name === 'string' ? p.name : '') + ': ' + (typeof p.description === 'string' ? p.description : '')).join('\n');
        instructions = instructions ? `${instructions}\n${promptText}` : promptText;
      }
    } catch {
      // optional
    }

    this.servers.set(name, { name, config, tools, instructions });
    this.vrb(turn, 0, 'response', name, 'initialized ' + name + ', tools ' + String(tools.length));
  }

  getAllTools(): MCPToolDef[] {
    return Array.from(this.servers.values()).flatMap((s) => s.tools);
  }

  getToolServerMapping(): Map<string, string> {
    const entries = Array.from(this.servers.entries()).flatMap(([serverName, s]) => s.tools.map((t) => [t.name, serverName] as const));
    return new Map(entries);
  }

  combinedInstructions(): string {
    return Array.from(this.servers.values())
      .flatMap((s) => {
        const out: string[] = [];
        if (typeof s.instructions === 'string' && s.instructions.length > 0) out.push(`## TOOL ${s.name} INSTRUCTIONS\n${s.instructions}`);
        s.tools.forEach((t) => { if (typeof t.instructions === 'string' && t.instructions.length > 0) out.push(`## TOOL ${t.name} INSTRUCTIONS\n${t.instructions}`); });
        return out;
      })
      .join('\n\n');
  }

  async executeTool(serverName: string, toolCall: ToolCallLike, timeoutMs: number, turn = 0, subturn = 0): Promise<MCPToolResult> {
    const client = this.clients.get(serverName);
    if (client == null) {
      this.err(turn, subturn, `${serverName}:${toolCall.name}`, `server not initialized`, false);
      return { toolCallId: toolCall.id, status: { type: 'connection_error', serverName, message: 'not initialized' }, result: '', latencyMs: 0, metadata: { mcpServer: serverName, command: toolCall.name, inSize: 0, outSize: 0 } };
    }
    const started = Date.now();
    const argsStr = JSON.stringify(toolCall.parameters);
    this.vrb(turn, subturn, 'request', serverName + ':' + toolCall.name, toolCall.name);
    this.trc(turn, subturn, 'request', serverName + ':' + toolCall.name, 'params: ' + argsStr);

    const withTimeout = async <T>(p: Promise<T>): Promise<T> => {
      if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) return p;
      const to = new Promise<T>((_r, rej) => { setTimeout(() => { rej(new Error('Tool execution timed out')); }, timeoutMs); });
      return (await Promise.race([p, to])) as T;
    };
    try {
      const resp: unknown = await withTimeout(client.callTool({ name: toolCall.name, arguments: toolCall.parameters }));
      const respBlocks = (resp as { content?: unknown[] } | undefined);
      const hasContent = (respBlocks !== undefined) && Array.isArray(respBlocks.content);
      const blocks = hasContent ? ((resp as { content?: { type?: unknown; text?: unknown }[] }).content ?? []) : [];
      let text = blocks.map((b) => {
        const typeVal = (b as { type?: unknown }).type;
        if (typeVal === 'text') {
          const tx = (b as { text?: unknown }).text;
          return typeof tx === 'string' ? tx : (typeof tx === 'number' ? String(tx) : '');
        }
        if (typeVal === 'image') return '[Image]';
        const ts = typeof typeVal === 'string' ? typeVal : (typeof typeVal === 'number' ? String(typeVal) : 'unknown');
        return '[' + ts + ']';
      }).join('');
      const isError = (resp as { isError?: unknown }).isError === true;
      if (isError && text.length === 0) {
        text = 'ERROR: Tool reported failure';
      }
      const latency = Date.now() - started;
      const respBytes = new TextEncoder().encode(text).length;
      this.vrb(turn, subturn, 'response', serverName + ':' + toolCall.name, 'latency ' + String(latency) + ' ms, size ' + String(respBytes) + ' bytes');
      this.trc(turn, subturn, 'response', serverName + ':' + toolCall.name, 'result: ' + JSON.stringify(resp));
      return {
        toolCallId: toolCall.id,
        status: isError ? { type: 'execution_error', toolName: toolCall.name, message: 'Tool execution failed' } : { type: 'success' },
        result: text,
        latencyMs: latency,
        metadata: { mcpServer: serverName, command: toolCall.name, inSize: argsStr.length, outSize: text.length },
      };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      this.err(turn, subturn, serverName + ':' + toolCall.name, msg, false);
      const latency = Date.now() - started;
      // Granular status mapping
      let status: ToolStatus;
      const lower = msg.toLowerCase();
      if (lower.includes('timed out')) status = { type: 'timeout', toolName: toolCall.name, timeoutMs: timeoutMs } as unknown as ToolStatus;
      else if (lower.includes('method not found') || lower.includes('not found')) status = { type: 'tool_not_found', toolName: toolCall.name, serverName } as ToolStatus;
      else if (lower.includes('invalid params') || lower.includes('invalid param') || (lower.includes('invalid') && lower.includes('param'))) status = { type: 'invalid_parameters', toolName: toolCall.name, message: msg } as ToolStatus;
      else if (lower.includes('econn') || lower.includes('enotfound') || lower.includes('network') || lower.includes('connection')) status = { type: 'connection_error', serverName, message: msg } as ToolStatus;
      else status = { type: 'execution_error', toolName: toolCall.name, message: msg } as ToolStatus;
      const resultText = 'ERROR: ' + msg;
      return { toolCallId: toolCall.id, status, result: resultText, latencyMs: latency, metadata: { mcpServer: serverName, command: toolCall.name, inSize: argsStr.length, outSize: resultText.length } };
    }
  }

  async cleanup(): Promise<void> {
    await Promise.all(Array.from(this.clients.values()).map(async (c) => { try { await c.close(); } catch { /* noop */ } }));
    this.clients.clear();
    this.servers.clear();
  }

  private resolveHeaders(headers?: Record<string, string>): Record<string, string> | undefined {
    if (headers === undefined) return headers;
    return Object.entries(headers).reduce<Record<string, string>>((acc, [k, v]) => {
      const resolved = v.replace(/\$\{([^}]+)\}/g, (_m: string, n: string) => process.env[n] ?? '');
      if (resolved.length > 0) acc[k] = resolved;
      return acc;
    }, {});
  }
}
