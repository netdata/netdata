import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

import type { MCPServerConfig, MCPTool, MCPServer, ToolCall, ToolResult, ToolStatus, LogEntry } from './types.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';
import type { ChildProcess } from 'node:child_process';

import { formatToolRequestCompact } from './utils.js';
import { createWebSocketTransport } from './websocket-transport.js';

export class MCPClientManager {
  private clients = new Map<string, Client>();
  private processes = new Map<string, ChildProcess>();
  private servers = new Map<string, MCPServer>();
  private trace = false;
  private verbose = false;
  private onLog?: (entry: LogEntry) => void;
  private currentTurn = 0;
  private currentSubturn = 0;
  // Map exposed tool name (namespaced) -> { serverName, originalName }
  private toolNameMap = new Map<string, { serverName: string; originalName: string }>();
  // Optional cap for tool response payload size (bytes). If exceeded, inject a tool error.
  private maxToolResponseBytes?: number;
  // Counter: number of times tool response size cap was exceeded
  private sizeCapHits = 0;
  private maxConcurrentInit = 4;

  constructor(opts?: { 
    trace?: boolean; 
    verbose?: boolean; 
    onLog?: (entry: LogEntry) => void;
    maxToolResponseBytes?: number;
    maxConcurrentInit?: number;
  }) {
    this.trace = opts?.trace === true;
    this.verbose = opts?.verbose === true;
    this.onLog = opts?.onLog;
    this.maxToolResponseBytes = opts?.maxToolResponseBytes;
    if (typeof opts?.maxConcurrentInit === "number" && opts.maxConcurrentInit > 0) this.maxConcurrentInit = opts.maxConcurrentInit;
  }

  setTurn(turn: number, subturn = 0): void {
    this.currentTurn = turn;
    this.currentSubturn = subturn;
  }

  async initializeServers(mcpServers: Record<string, MCPServerConfig>): Promise<MCPServer[]> {
    const entries = Object.entries(mcpServers);
    const results: MCPServer[] = [];
    const limit = Math.min(this.maxConcurrentInit, Math.max(entries.length, 1));

    let cursor = 0;
    const worker = async (): Promise<void> => {
      // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
      while (true) {
        const idx = cursor++;
        if (idx >= entries.length) break;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
        const [name, config] = entries[idx] as [string, MCPServerConfig];
        try {
          const server = await this.initializeServer(name, config);
          this.servers.set(name, server);
          results.push(server);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          this.log('ERR', 'response', 'mcp', `${name}:init`, `initialization failed: ${message}`, true);
        }
      }
    };

    await Promise.all(Array.from({ length: limit }).map(async () => { await worker(); }));
    return results;
  }

  private redactAuthHeader(headers: Record<string, string>): Record<string, string> {
    const safeHeaders = { ...headers };
    if (safeHeaders.Authorization) {
      const auth = safeHeaders.Authorization;
      if (auth.startsWith('Bearer ')) {
        const token = auth.substring(7);
        if (token.length > 8) {
          safeHeaders.Authorization = `Bearer ${token.substring(0, 4)}...REDACTED...${token.substring(token.length - 4)}`;
        } else {
          safeHeaders.Authorization = `Bearer [SHORT_TOKEN]`;
        }
      } else {
        safeHeaders.Authorization = '[REDACTED]';
      }
    }
    return safeHeaders;
  }

  private async initializeServer(name: string, config: MCPServerConfig): Promise<MCPServer> {
    const client = new Client({ name: 'ai-agent', version: '1.0.0' }, { capabilities: { tools: {} } });

    let transport: Transport;
    const initStart = Date.now();
    
    this.log('VRB', 'request', 'mcp', `${name}:connect`, `type=${config.type} ${config.command ?? config.url ?? ''}`);
    
    try {
      switch (config.type) {
        case 'stdio':
          transport = this.createStdioTransport(name, config);
          break;
        case 'websocket': {
          if (config.url == null || config.url.length === 0) throw new Error(`WebSocket MCP server '${name}' requires a 'url'`);
          const resolvedHeaders = config.headers;
          
          // Log WebSocket connection details for tracing
          if (this.trace) {
            if (resolvedHeaders !== undefined) {
              const safeHeaders = this.redactAuthHeader(resolvedHeaders);
              this.log('TRC', 'request', 'mcp', `${name}:websocket-connect`, 
                `WebSocket URL: ${config.url}, Headers: ${JSON.stringify(safeHeaders)}`);
            } else {
              this.log('TRC', 'request', 'mcp', `${name}:websocket-connect`, 
                `WebSocket URL: ${config.url}, Headers: none`);
            }
          }
          
          transport = await createWebSocketTransport(config.url, resolvedHeaders);
          break;
        }
        case 'http': {
          if (config.url == null || config.url.length === 0) throw new Error(`HTTP MCP server '${name}' requires a 'url'`);
          const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
          const resolvedHeaders = config.headers;
          
          // Log HTTP connection details for tracing
          if (this.trace) {
            if (resolvedHeaders !== undefined) {
              const safeHeaders = this.redactAuthHeader(resolvedHeaders);
              this.log('TRC', 'request', 'mcp', `${name}:http-connect`, 
                `HTTP URL: ${config.url}, Headers: ${JSON.stringify(safeHeaders)}`);
            } else {
              this.log('TRC', 'request', 'mcp', `${name}:http-connect`, 
                `HTTP URL: ${config.url}, Headers: none`);
            }
          }
          
          const reqInit: RequestInit = { headers: resolvedHeaders as HeadersInit };
          transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
          break;
        }
        
        case 'sse':
          if (config.url == null || config.url.length === 0) throw new Error(`SSE MCP server '${name}' requires a 'url'`);
          const resolvedHeaders = config.headers;
          
          // Log the SSE connection details for tracing
          if (this.trace) {
            if (resolvedHeaders !== undefined) {
              const safeHeaders = this.redactAuthHeader(resolvedHeaders);
              this.log('TRC', 'request', 'mcp', `${name}:sse-connect`, 
                `SSE URL: ${config.url}, Headers: ${JSON.stringify(safeHeaders)}`);
            } else {
              this.log('TRC', 'request', 'mcp', `${name}:sse-connect`, 
                `SSE URL: ${config.url}, Headers: none`);
            }
          }
          
          // SSEClientTransport needs a custom fetch to include headers for EventSource
          // The requestInit headers are only used for POST requests, not the EventSource connection
          const customFetch: typeof fetch = async (input, init) => {
            // Merge our headers with any headers from init
            const headers = new Headers(init?.headers);
            if (resolvedHeaders !== undefined) {
              Object.entries(resolvedHeaders).forEach(([key, value]) => {
                headers.set(key, value);
              });
            }
            return fetch(input, { ...init, headers });
          };
          
          transport = new SSEClientTransport(new URL(config.url), {
            eventSourceInit: { fetch: customFetch },
            requestInit: { headers: resolvedHeaders as HeadersInit },
            fetch: customFetch
          });
          break;

        default:
          throw new Error('Unsupported transport type');
      }

      await client.connect(transport);
      this.clients.set(name, client);

      const connectLatency = Date.now() - initStart;
      this.log('VRB', 'response', 'mcp', `${name}:connect`, `connected in ${String(connectLatency)}ms`);

    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      this.log('ERR', 'response', 'mcp', `${name}:connect`, message, true);
      throw e;
    }

    // Get server instructions
    const initInstructions = client.getInstructions() ?? '';

    // List tools
    const toolsStart = Date.now();
    this.log('VRB', 'request', 'mcp', `${name}:tools/list`, 'requesting tools');
    
    let toolsResponse;
    try {
      toolsResponse = await client.listTools();
      const toolsLatency = Date.now() - toolsStart;
      this.log('VRB', 'response', 'mcp', `${name}:tools/list`, `${String(toolsResponse.tools.length)} tools in ${String(toolsLatency)}ms`);
      
      if (this.trace) {
        this.log('TRC', 'response', 'mcp', `${name}:tools/list`, JSON.stringify(toolsResponse, null, 2));
      }
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      this.log('ERR', 'response', 'mcp', `${name}:tools/list`, message, true);
      throw e;
    }

    interface ToolItem { name: string; description?: string; inputSchema?: unknown; parameters?: unknown; instructions?: string }
    const tools: MCPTool[] = (toolsResponse.tools as ToolItem[]).map((t) => ({
      name: t.name,
      description: t.description ?? '',
      inputSchema: (t.inputSchema ?? t.parameters ?? {}) as Record<string, unknown>,
      instructions: t.instructions,
    }));

    // Populate exposed name mapping for this server's tools
    const ns = this.sanitizeNamespace(name);
    // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
    for (const t of tools) {
      const exposed = `${ns}__${t.name}`;
      this.toolNameMap.set(exposed, { serverName: name, originalName: t.name });
    }

    // List prompts (optional)
    let instructions = initInstructions;
    try {
      const promptsStart = Date.now();
      this.log('VRB', 'request', 'mcp', `${name}:prompts/list`, 'requesting prompts');
      
      let promptsResponse;
      try {
        promptsResponse = await client.listPrompts();
        const promptsLatency = Date.now() - promptsStart;
        
        const pr = promptsResponse as { prompts?: { name: string; description?: string }[] } | undefined;
        const list = Array.isArray(pr?.prompts) ? pr.prompts : [];
        
        this.log('VRB', 'response', 'mcp', `${name}:prompts/list`, `${String(list.length)} prompts in ${String(promptsLatency)}ms`);
        
        if (this.trace) {
          this.log('TRC', 'response', 'mcp', `${name}:prompts/list`, JSON.stringify(promptsResponse, null, 2));
        }

        if (list.length > 0) {
          const promptText = list.map((p) => `${p.name}: ${p.description ?? ''}`).join('\n');
          instructions = instructions.length > 0 ? `${instructions}\n${promptText}` : promptText;
        }
      } catch (e) {
        // Prompts are optional, log as warning but continue
        const message = e instanceof Error ? e.message : String(e);
        this.log('WRN', 'response', 'mcp', `${name}:prompts/list`, `failed: ${message}`);
      }
    } catch {
      // Ignore prompts errors completely
    }

    const totalLatency = Date.now() - initStart;
    const toolNames = tools.map(t => t.name).join(', ');
    const hasInstructions = instructions.length > 0;
    this.log('VRB', 'response', 'mcp', `${name}:init`, 
      `${String(tools.length)} tools (${toolNames || '-'})${hasInstructions ? ', with instructions' : ''}, ${String(totalLatency)}ms total`);

    if (this.trace) {
      this.log('TRC', 'response', 'mcp', `${name}:init`, `ready with ${String(tools.length)} tools`);
    }
    
    return { name, config, tools, instructions };
  }

  private createStdioTransport(name: string, config: MCPServerConfig): StdioClientTransport {
    if (typeof config.command !== 'string' || config.command.length === 0) {
      throw new Error(`Stdio MCP server '${name}' requires a string 'command'`);
    }

    // Use environment as provided by the unified configuration (already resolved by config-resolver)
    const effectiveEnv: Record<string, string> = { ...(config.env ?? {}) };

    const transport = new StdioClientTransport({
      command: config.command,
      args: config.args ?? [],
      env: effectiveEnv,
      stderr: 'pipe',
    });

    // Handle stderr logging
    try {
      if (transport.stderr != null) {
        transport.stderr.on('data', (d: Buffer) => {
          const line = d.toString('utf8').trimEnd();
          if (this.trace) {
            this.log('TRC', 'request', 'mcp', `${name}:stderr`, line);
          }
        });
      }
    } catch {
      // Ignore stderr setup errors
    }
    
    return transport;
  }

  async executeTool(serverName: string, toolCall: ToolCall, timeoutMs?: number): Promise<ToolResult> {
    const client = this.clients.get(serverName);
    if (client == null) {
      return {
        toolCallId: toolCall.id,
        status: { type: 'mcp_server_error', serverName, message: `MCP server '${serverName}' not initialized` },
        result: '',
        latencyMs: 0,
        metadata: {
          latency: 0,
          charactersIn: 0,
          charactersOut: 0,
          mcpServer: serverName,
          command: toolCall.name,
        }
      };
    }

    const start = Date.now();

    // Sanitize parameters: drop null/undefined so optional strings don't validate as null
    const sanitize = (val: unknown): unknown => {
      if (val === null || val === undefined) return undefined;
      if (Array.isArray(val)) {
        const arr = (val as unknown[]).map((v) => sanitize(v)).filter((v) => v !== undefined);
        return arr;
      }
      if (typeof val === 'object') {
        const obj = val as Record<string, unknown>;
        const out = Object.entries(obj).reduce<Record<string, unknown>>((acc, [k, v]) => {
          const sv = sanitize(v);
          if (sv !== undefined) acc[k] = sv as unknown;
          return acc;
        }, {});
        return out;
      }
      return val;
    };
    const originalParams = toolCall.parameters;
    const sanitizedParams = sanitize(originalParams) as Record<string, unknown>;
    const argsStr = JSON.stringify(sanitizedParams);
    // replaced by sanitized argsStr
    
    const compactReq = formatToolRequestCompact(toolCall.name, sanitizedParams);
    this.log('VRB', 'request', 'mcp', `${serverName}:${toolCall.name}`, compactReq);

    try {
      const before = JSON.stringify(originalParams);
      if (before !== argsStr) {
        this.log('WRN', 'request', 'mcp', `${serverName}:${toolCall.name}`, 'sanitized null/undefined fields from arguments');
      }
    } catch { /* ignore */ }


    if (this.trace) {
      this.log('TRC', 'request', 'mcp', `${serverName}:${toolCall.name}`, 
        `callTool ${JSON.stringify({ name: toolCall.name, arguments: sanitizedParams })}`);
    }

    const callWithTimeout = async () => {
      if (typeof timeoutMs !== 'number' || timeoutMs <= 0) {
        return await client.callTool({ name: toolCall.name, arguments: sanitizedParams });
      }
      
      const timeoutPromise = new Promise((_resolve, reject) => {
        setTimeout(() => { reject(new Error('Tool execution timed out')); }, timeoutMs);
      });
      
      const callPromise = client.callTool({ name: toolCall.name, arguments: sanitizedParams });
      return await Promise.race([callPromise, timeoutPromise]);
    };

    try {
      const resp = await callWithTimeout();
      const latencyMs = Date.now() - start;
      
      let result = '';
      interface Block { type: string; text?: string }
      const content: Block[] = (resp as { content?: Block[] }).content ?? [];
      result = content.map((c) => {
        if (c.type === 'text') return c.text ?? '';
        if (c.type === 'image') return '[Image]';
        return `[${c.type}]`;
      }).join('');

      const isError = (resp as { isError?: boolean }).isError === true;

      // Enforce response size cap if configured (truncate instead of reject)
      if (!isError && typeof this.maxToolResponseBytes === 'number' && this.maxToolResponseBytes > 0) {
        const sizeBytes = Buffer.byteLength(result, 'utf8');
        if (sizeBytes > this.maxToolResponseBytes) {
          // Mirror the VRB request formatting (name + summarized parameters)
          const requestInfo = compactReq;
          const limit = this.maxToolResponseBytes;
          const warnMsg = `${requestInfo} → response exceeded max size: ${String(sizeBytes)} bytes > limit ${String(limit)} bytes (truncated)`;
          this.log('WRN', 'response', 'mcp', `${serverName}:${toolCall.name}`, warnMsg);
          this.sizeCapHits += 1;

          // Prepare injected prefix message
          const prefix = `[TRUNCATED] Original size ${String(sizeBytes)} bytes; truncated to ${String(limit)} bytes.\n\n`;
          const enc = new TextEncoder();
          const dec = new TextDecoder('utf-8', { fatal: false });
          const prefixBytes = enc.encode(prefix);
          if (prefixBytes.byteLength >= limit) {
            // Edge case: prefix alone exceeds limit → return prefix trimmed to limit
            const slice = prefixBytes.subarray(0, limit);
            const truncated = dec.decode(slice);
            result = truncated;
          } else {
            // Compute remaining budget for original content
            const budget = limit - prefixBytes.byteLength;
            const resBytes = enc.encode(result);
            const contentSlice = resBytes.subarray(0, Math.min(budget, resBytes.byteLength));
            const truncated = dec.decode(contentSlice);
            result = prefix + truncated;
          }
        }
      }
      const status: ToolStatus = isError 
        ? { type: 'execution_error', toolName: toolCall.name, message: 'Tool execution failed' }
        : { type: 'success' };

      this.log('VRB', 'response', 'mcp', `${serverName}:${toolCall.name}`, 
        `${String(latencyMs)}ms, ${String(result.length)} chars${isError ? ' (error)' : ''}`);

      if (this.trace) {
        this.log('TRC', 'response', 'mcp', `${serverName}:${toolCall.name}`, 
          JSON.stringify(resp, null, 2));
      }

      return {
        toolCallId: toolCall.id,
        status,
        result,
        latencyMs,
        metadata: {
          latency: latencyMs,
          charactersIn: argsStr.length,
          charactersOut: result.length,
          mcpServer: serverName,
          command: toolCall.name,
        },
      };
    } catch (err) {
      const latencyMs = Date.now() - start;
      const message = err instanceof Error ? err.message : String(err);
      
      let status: ToolStatus;
      if (message.includes('timed out')) {
        status = { type: 'timeout', toolName: toolCall.name, timeoutMs: timeoutMs ?? 0 };
      } else if (message.includes('not found')) {
        status = { type: 'tool_not_found', toolName: toolCall.name, serverName };
      } else if (message.includes('invalid') || message.includes('parameters')) {
        status = { type: 'invalid_parameters', toolName: toolCall.name, message };
      } else if (message.includes('connection')) {
        status = { type: 'connection_error', serverName, message };
      } else {
        status = { type: 'execution_error', toolName: toolCall.name, message };
      }

      this.log('ERR', 'response', 'mcp', `${serverName}:${toolCall.name}`, 
        `error: ${message} (${String(latencyMs)}ms)`, false);

      return {
        toolCallId: toolCall.id,
        status,
        result: '',
        latencyMs,
        metadata: {
          latency: latencyMs,
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
    if (typeof maxConcurrent === 'number' && maxConcurrent > 0) {
      return this.executeWithConcurrency(toolCalls, maxConcurrent, timeoutMs);
    }
    return Promise.all(toolCalls.map(({ serverName, toolCall }) => 
      this.executeTool(serverName, toolCall, timeoutMs)
    ));
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

    const workers = Array.from({ length: Math.min(maxConcurrent, toolCalls.length) }, () => runWorker());
    await Promise.all(workers);
    return results;
  }

  getAllTools(): MCPTool[] {
    // Return tools with namespaced (exposed) names for the LLM
    const out: MCPTool[] = [];
    // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
    for (const [serverName, s] of this.servers.entries()) {
      const ns = this.sanitizeNamespace(serverName);
      // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
      for (const t of s.tools) {
        out.push({
          name: `${ns}__${t.name}`,
          description: t.description,
          inputSchema: t.inputSchema,
          instructions: t.instructions,
        });
      }
    }
    return out;
  }

  // Retrieve the JSON schema for a given server and original tool name
  // Returns undefined if the server or tool is unknown.
  getToolSchema(serverName: string, toolName: string): Record<string, unknown> | undefined {
    const server = this.servers.get(serverName);
    if (server === undefined) return undefined;
    const tool = server.tools.find((t) => t.name === toolName);
    return tool?.inputSchema;
  }

  // Resolve an exposed tool name (e.g., brave__brave_web_search) to its server and original tool name
  resolveExposedTool(exposedName: string): { serverName: string; originalName: string } | undefined {
    const m = this.toolNameMap.get(exposedName);
    if (m === undefined) return undefined;
    return { serverName: m.serverName, originalName: m.originalName };
  }

  getCombinedInstructions(): string {
    return Array.from(this.servers.entries()).flatMap(([serverName, s]) => {
      const arr: string[] = [];
      if (typeof s.instructions === 'string' && s.instructions.length > 0) {
        arr.push(`## TOOL ${s.name} INSTRUCTIONS\n${s.instructions}`);
      }
      const ns = this.sanitizeNamespace(serverName);
      s.tools.forEach((t) => {
        if (typeof t.instructions === 'string' && t.instructions.length > 0) {
          const exposed = `${ns}__${t.name}`;
          arr.push(`## TOOL ${exposed} INSTRUCTIONS\n${t.instructions}`);
        }
      });
      return arr;
    }).join('\n\n');
  }

  // Simple tool executor for AI SDK integration
  async executeToolByName(toolName: string, parameters: Record<string, unknown>): Promise<{ result: string; serverName: string }> {
    const mapping = this.toolNameMap.get(toolName);
    if (mapping === undefined) {
      throw new Error(`No server found for tool: ${toolName}`);
    }
    const { serverName, originalName } = mapping;

    // Create a temporary tool call - we need an ID for the interface
    const toolCall: ToolCall = {
      id: `temp_${toolName}_${String(Date.now())}`,
      // Use the ORIGINAL tool name for the MCP server and for logging
      name: originalName,
      parameters
    };

    const result = await this.executeTool(serverName, toolCall);
    
    if (result.status.type !== 'success') {
      const errorMsg = 'message' in result.status ? result.status.message : result.status.type;
      throw new Error(`Tool execution failed: ${errorMsg}`);
    }
    
    return { result: result.result, serverName };
  }

  getToolServerMapping(): Map<string, string> {
    // Exposed name -> server mapping
    const m = new Map<string, string>();
    // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
    for (const [exposed, info] of this.toolNameMap.entries()) {
      m.set(exposed, info.serverName);
    }
    return m;
  }

  async cleanup(): Promise<void> {
    await Promise.all(Array.from(this.clients.values()).map(async (c) => { 
      try { await c.close(); } catch { /* noop */ } 
    }));
    
    await Promise.all(Array.from(this.processes.values()).map((proc) => 
      new Promise<void>((resolve) => {
        try {
          if (proc.killed || proc.exitCode !== null) {
            resolve();
            return;
          }
          
          const timeout = setTimeout(() => {
            if (!proc.killed && proc.exitCode === null) {
              try { proc.kill('SIGKILL'); } catch { /* noop */ }
            }
          }, 2000);
          
          proc.once('exit', () => {
            clearTimeout(timeout);
            resolve();
          });
          
          proc.kill('SIGTERM');
        } catch {
          resolve();
        }
      })
    ));
    
    this.clients.clear();
    this.processes.clear();
    this.servers.clear();
    this.toolNameMap.clear();
  }

  private log(
    severity: LogEntry['severity'],
    direction: LogEntry['direction'],
    type: LogEntry['type'],
    remoteIdentifier: string,
    message: string,
    fatal = false
  ): void {
    if (this.onLog === undefined) return;

    const entry: LogEntry = {
      timestamp: Date.now(),
      severity,
      turn: this.currentTurn,
      subturn: this.currentSubturn,
      direction,
      type,
      remoteIdentifier,
      fatal,
      message
    };

    this.onLog(entry);
  }

  private sanitizeNamespace(name: string): string {
    return toUnderscore(name);
  }

  getSizeCapHits(): number { return this.sizeCapHits; }
}

// Helpers
function toUnderscore(s: string): string {
  // Replace non-alphanumeric with underscore and collapse repeats
  return s
    .replace(/[^A-Za-z0-9]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toLowerCase();
}
