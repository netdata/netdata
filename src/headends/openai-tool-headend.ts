import { randomUUID } from 'node:crypto';
import http from 'node:http';
import path from 'node:path';
import { URL } from 'node:url';

import type { AgentRegistry, AgentMetadata } from '../agent-registry.js';
import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, LogEntry } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

import { resolveToolName, toOpenAIToolDefinition } from '../schema-adapters.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { HttpError, readJson, writeJson, writeSseChunk, writeSseDone } from './http-utils.js';

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

const createDeferred = <T>(): Deferred<T> => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

interface OpenAIToolHeadendOptions {
  port: number;
  concurrency?: number;
}

interface ToolCallPayload {
  id: string;
  name: string;
  args: Record<string, unknown>;
}

interface ToolResponseBody {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: {
    index: number;
    message: {
      role: 'tool';
      content: string;
      tool_call_id: string;
    };
    finish_reason: 'stop' | 'error';
  }[];
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
}

const buildLog = (headendId: string, message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier: 'headend:openai-tool',
  fatal,
  message,
  headendId,
});

const CHAT_COMPLETION_OBJECT = 'chat.completion';
const CHAT_CHUNK_OBJECT = 'chat.completion.chunk';
const CLIENT_CLOSED_ERROR = 'client_closed_request';
const TOOL_CALL_ID_KEY = 'tool_call_id';

const collectTokenUsage = (entries: AccountingEntry[]): { prompt: number; completion: number; total: number } => {
  return entries
    .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
    .reduce<{ prompt: number; completion: number; total: number }>((acc, entry) => {
      const usage = entry.tokens;
      acc.prompt += usage.inputTokens;
      acc.completion += usage.outputTokens;
      acc.total += usage.totalTokens;
      return acc;
    }, { prompt: 0, completion: 0, total: 0 });
};

export class OpenAIToolHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'openai-tool';
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly options: OpenAIToolHeadendOptions;
  private readonly limiter: ConcurrencyLimiter;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private readonly toolIdMap = new Map<string, string>();
  private context?: HeadendContext;
  private server?: http.Server;
  private stopping = false;
  private closedSignaled = false;

  public constructor(registry: AgentRegistry, options: OpenAIToolHeadendOptions) {
    this.registry = registry;
    this.options = options;
    this.id = `openai-tool:${String(options.port)}`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : 10;
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
    this.refreshToolMap();
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: `OpenAI tool headend (port ${String(this.options.port)})` };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.server !== undefined) return;
    this.context = context;
    const server = http.createServer((req, res) => { void this.handleRequest(req, res); });
    this.server = server;
    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`server error: ${message}`, 'ERR', true);
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
      }
    });
    server.on('close', () => {
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'stopped', graceful: true });
      }
    });
    await new Promise<void>((resolve, reject) => {
      const onListening = () => {
        server.off('error', onError);
        resolve();
      };
      const onError = (err: unknown) => {
        server.off('listening', onListening);
        reject(err instanceof Error ? err : new Error(String(err)));
      };
      server.once('listening', onListening);
      server.once('error', onError);
      server.listen(this.options.port);
    });
    this.log('listening');
  }

  public async stop(): Promise<void> {
    if (this.server === undefined) {
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'stopped', graceful: true });
      }
      return;
    }
    this.stopping = true;
    await new Promise<void>((resolve) => { this.server?.close(() => { resolve(); }); });
    this.server = undefined;
    this.signalClosed({ reason: 'stopped', graceful: true });
  }

  private async handleRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    try {
      const url = new URL(req.url ?? '/', `http://localhost:${String(this.options.port)}`);
      const pathname = url.pathname.replace(/\/+/g, '/');
      if (req.method === 'GET' && pathname === '/health') {
        writeJson(res, 200, { status: 'ok' });
        return;
      }
      if (req.method === 'GET' && pathname === '/v1/tools') {
        this.handleListTools(res);
        return;
      }
      if (req.method === 'POST' && pathname === '/v1/chat/completions') {
        await this.handleToolExecution(req, res);
        return;
      }
      writeJson(res, 404, { error: 'not_found' });
    } catch (err) {
      if (err instanceof HttpError) {
        writeJson(res, err.statusCode, { error: err.code });
        return;
      }
      const message = err instanceof Error ? err.message : String(err);
      this.log(`request failure: ${message}`, 'ERR');
      writeJson(res, 500, { error: 'internal_error' });
    }
  }

  private handleListTools(res: http.ServerResponse): void {
    this.refreshToolMap();
    const data: { object: 'tool'; agent_id: string; type: 'function'; function: { name: string; description?: string; parameters: Record<string, unknown> } }[] = [];
    this.toolIdMap.forEach((agentId, toolId) => {
      const meta = this.registry.getMetadata(agentId);
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- registry could change between list() and lookup
      if (meta === undefined) return;
      const def = toOpenAIToolDefinition({
        id: toolId,
        toolName: undefined,
        description: meta.description,
        usage: meta.usage,
        input: meta.input,
        outputSchema: meta.outputSchema,
      });
      data.push({
        object: 'tool',
        agent_id: toolId,
        type: def.type,
        function: def.function,
      });
    });
    writeJson(res, 200, { object: 'list', data });
  }

  private async handleToolExecution(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const payload = await readJson<Record<string, unknown>>(req);
    const toolCall = this.parseToolCall(payload);
    const agentMeta = this.resolveAgent(toolCall.name);
    if (agentMeta === undefined) {
      throw new HttpError(404, 'unknown_tool', `Tool '${toolCall.name}' not registered`);
    }

    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log(`tool_call ${toolCall.id} aborted`, 'WRN');
    };
    req.on('aborted', onAbort);
    res.on('close', onAbort);

    const cleanup = () => {
      req.removeListener('aborted', onAbort);
      res.removeListener('close', onAbort);
    };

    let release: (() => void) | undefined;
    try {
      release = await this.limiter.acquire({ signal: abortController.signal });
    } catch (err: unknown) {
      cleanup();
      if (abortController.signal.aborted) {
        return;
      }
      const message = err instanceof Error ? err.message : String(err);
      this.log(`concurrency acquire failed for tool_call ${toolCall.id}: ${message}`, 'ERR');
      writeJson(res, 503, { error: 'concurrency_unavailable', message: 'Concurrency limit reached' });
      return;
    }

    const created = Math.floor(Date.now() / 1000);
    const requestId = randomUUID();
    this.log(`tool_call start id=${toolCall.id} agent=${agentMeta.id}`, 'VRB');

    const streamed = payload.stream === true;
    let output = '';
    const accounting: AccountingEntry[] = [];
    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
        if (streamed) {
      const chunkPayload = {
        id: requestId,
        object: CHAT_CHUNK_OBJECT,
        created,
        model: toolCall.name,
        choices: [
          {
            index: 0,
                delta: { role: 'tool', content: chunk },
                finish_reason: null,
                [TOOL_CALL_ID_KEY]: toolCall.id,
              },
            ],
          };
          writeSseChunk(res, chunkPayload);
        }
      },
      onLog: (entry) => { this.logEntry(entry); },
      onAccounting: (entry) => { accounting.push(entry); },
    };

    if (streamed) {
      res.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      const flushHeaders = (res as http.ServerResponse & { flushHeaders?: () => void }).flushHeaders;
      if (typeof flushHeaders === 'function') flushHeaders.call(res);
    }

    try {
      this.validateArguments(toolCall.args);
      const session = await this.registry.spawnSession({
        agentId: agentMeta.id,
        payload: toolCall.args,
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
      });
      const result = await session.run();
      if (abortController.signal.aborted) {
        if (!res.writableEnded && !res.writableFinished) {
          if (streamed) { writeSseDone(res); } else { writeJson(res, 499, { error: CLIENT_CLOSED_ERROR }); }
        }
        return;
      }
      const finalText = this.resolveFinalText(output, result.finalReport);
      const usage = collectTokenUsage(accounting);
      if (streamed) {
        const finalChunk = {
          id: requestId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: toolCall.name,
          choices: [
            {
              index: 0,
              delta: {},
              finish_reason: result.success ? 'stop' : 'error',
              [TOOL_CALL_ID_KEY]: toolCall.id,
            },
          ],
        };
        writeSseChunk(res, finalChunk);
        writeSseDone(res);
      } else {
        const body: ToolResponseBody = {
          id: requestId,
          object: CHAT_COMPLETION_OBJECT,
          created,
          model: toolCall.name,
          choices: [
            {
              index: 0,
              message: {
                role: 'tool',
                content: finalText,
                [TOOL_CALL_ID_KEY]: toolCall.id,
              },
              finish_reason: result.success ? 'stop' : 'error',
            },
          ],
          usage: {
            prompt_tokens: usage.prompt,
            completion_tokens: usage.completion,
            total_tokens: usage.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, body);
      }
      this.log(`tool_call end id=${toolCall.id} status=${result.success ? 'ok' : 'error'}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`tool_call failed id=${toolCall.id}: ${message}`, 'ERR');
      if (streamed) {
        const chunk = {
          id: requestId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: toolCall.name,
          choices: [
            {
              index: 0,
              delta: { role: 'tool', content: message },
              finish_reason: 'error' as const,
              [TOOL_CALL_ID_KEY]: toolCall.id,
            },
          ],
        };
        writeSseChunk(res, chunk);
        writeSseDone(res);
      } else {
        const status = err instanceof HttpError ? err.statusCode : 500;
        const code = err instanceof HttpError ? err.code : 'tool_call_failed';
        writeJson(res, status, { error: code, message });
      }
    } finally {
      cleanup();
      release();
    }
  }

  private parseToolCall(body: Record<string, unknown>): ToolCallPayload {
    const toolCallRaw = body.tool_call;
    if (typeof toolCallRaw === 'object' && toolCallRaw !== null) {
      const call = toolCallRaw as Record<string, unknown>;
      const id = typeof call.id === 'string' ? call.id : randomUUID();
      const fn = call.function as Record<string, unknown> | undefined;
      const name = typeof fn?.name === 'string' ? fn.name : undefined;
      const argsRaw = fn?.arguments;
      if (name === undefined) throw new HttpError(400, 'invalid_tool_call', 'tool_call.function.name is required');
      const args = this.parseArguments(argsRaw);
      return { id, name, args };
    }
    const id = typeof body.tool_call_id === 'string' && body.tool_call_id.length > 0 ? body.tool_call_id : randomUUID();
    const name = typeof body.tool_name === 'string' && body.tool_name.length > 0 ? body.tool_name : undefined;
    if (name === undefined) throw new HttpError(400, 'invalid_tool_call', 'tool_name is required');
    const args = this.parseArguments(body.arguments);
    return { id, name, args };
  }

  private parseArguments(raw: unknown): Record<string, unknown> {
    if (typeof raw === 'string') {
      try {
        return JSON.parse(raw) as Record<string, unknown>;
      } catch (err) {
        throw new HttpError(400, 'invalid_arguments', err instanceof Error ? err.message : 'Invalid JSON arguments');
      }
    }
    if (raw !== null && typeof raw === 'object') {
      return raw as Record<string, unknown>;
    }
    throw new HttpError(400, 'invalid_arguments', 'Tool arguments must be an object or JSON string');
  }

  private validateArguments(args: Record<string, unknown>): void {
    const formatRaw = args.format;
    if (typeof formatRaw !== 'string' || formatRaw.length === 0) {
      throw new HttpError(400, 'missing_format', 'Tool arguments must include format');
    }
    if (formatRaw === 'json') {
      const schema = args.schema;
      if (schema === undefined || schema === null || typeof schema !== 'object' || Array.isArray(schema)) {
        throw new HttpError(400, 'missing_schema', 'JSON format requires schema property with object value');
      }
    }
    const prompt = args.prompt;
    if (typeof prompt !== 'string' || prompt.trim().length === 0) {
      throw new HttpError(400, 'missing_prompt', 'Tool arguments must include prompt');
    }
  }

  private resolveAgent(toolName: string): AgentMetadata | undefined {
    this.refreshToolMap();
    const direct = this.toolIdMap.get(toolName);
    if (direct !== undefined) {
      return this.registry.getMetadata(direct);
    }
    const resolved = this.registry.resolveAgentId(toolName);
    if (resolved !== undefined) {
      return this.registry.getMetadata(resolved);
    }
    return undefined;
  }

  private resolveFinalText(output: string, finalReport: unknown): string {
    if (typeof finalReport === 'object' && finalReport !== null) {
      const report = finalReport as { format?: string; content?: string; content_json?: Record<string, unknown> };
      if (report.format === 'json' && report.content_json !== undefined) {
        try { return JSON.stringify(report.content_json); } catch { return output; }
      }
      if (typeof report.content === 'string' && report.content.length > 0) return report.content;
    }
    return output;
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): void {
    if (this.context === undefined) return;
    this.context.log(buildLog(this.id, message, severity, fatal));
  }

  private logEntry(entry: LogEntry): void {
    if (this.context === undefined) return;
    this.context.log(entry);
  }

  private signalClosed(event: HeadendClosedEvent): void {
    if (this.closedSignaled) return;
    this.closedSignaled = true;
    this.closeDeferred.resolve(event);
  }

  private refreshToolMap(): void {
    this.toolIdMap.clear();
    const seen = new Set<string>();
    this.registry.list().forEach((meta) => {
      const toolId = this.buildToolId(meta, seen);
      this.toolIdMap.set(toolId, meta.id);
    });
  }

  private buildToolId(meta: AgentMetadata, seen: Set<string>): string {
    const preferred = [
      typeof meta.toolName === 'string' ? meta.toolName : undefined,
      typeof meta.promptPath === 'string' ? path.basename(meta.promptPath) : undefined,
      (() => {
        const parts = meta.id.split(/[/\\]/);
        return parts[parts.length - 1];
      })(),
    ].find((value): value is string => typeof value === 'string' && value.length > 0) ?? 'agent';
    const base = preferred.replace(/\.ai$/i, '') || 'agent';
    let suffix = 0;
    let sanitized = '';
    // eslint-disable-next-line functional/no-loop-statements
    do {
      const suffixLabel = suffix === 0 ? '' : `_${String(suffix + 1)}`;
      const candidate = `${base}${suffixLabel}`;
      const summary = {
        id: candidate,
        toolName: undefined,
        description: meta.description,
        usage: meta.usage,
        input: meta.input,
        outputSchema: meta.outputSchema,
      };
      sanitized = resolveToolName(summary);
      suffix += 1;
    } while (seen.has(sanitized));
    seen.add(sanitized);
    return sanitized;
  }
}
