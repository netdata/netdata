import { randomUUID } from 'node:crypto';
import http from 'node:http';
import { URL } from 'node:url';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, LogEntry } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

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

interface OpenAIChatRequestMessage {
  role: 'system' | 'user' | 'assistant';
  content: unknown;
  tool_calls?: unknown;
}

interface OpenAIChatRequest {
  model: string;
  messages: OpenAIChatRequestMessage[];
  stream?: boolean;
  format?: string;
  response_format?: { type?: string; json_schema?: unknown };
  payload?: Record<string, unknown>;
}

interface OpenAIChatResponse {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: {
    index: number;
    message: { role: 'assistant'; content: string };
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
  remoteIdentifier: 'headend:openai-completions',
  fatal,
  message,
  headendId,
});

const SYSTEM_PREFIX = 'System context:\n';
const HISTORY_PREFIX = 'Conversation so far:\n';
const USER_PREFIX = 'User request:\n';
const CHAT_COMPLETION_OBJECT = 'chat.completion';
const CHAT_CHUNK_OBJECT = 'chat.completion.chunk';
const CLIENT_CLOSED_ERROR = 'client_closed_request';

const collectUsage = (entries: AccountingEntry[]): { prompt: number; completion: number; total: number } => entries
  .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
  .reduce<{ prompt: number; completion: number; total: number }>((acc, entry) => {
    const tokens = entry.tokens;
    acc.prompt += tokens.inputTokens;
    acc.completion += tokens.outputTokens;
    acc.total += tokens.totalTokens;
    return acc;
  }, { prompt: 0, completion: 0, total: 0 });

interface FormatResolution {
  format: string;
  schema?: Record<string, unknown>;
}

export class OpenAICompletionsHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'openai-completions';
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly options: { port: number; concurrency?: number };
  private readonly limiter: ConcurrencyLimiter;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private context?: HeadendContext;
  private server?: http.Server;
  private stopping = false;
  private closedSignaled = false;

  public constructor(registry: AgentRegistry, options: { port: number; concurrency?: number }) {
    this.registry = registry;
    this.options = options;
    this.id = `openai-completions:${String(options.port)}`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : 4;
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: `OpenAI chat headend (port ${String(this.options.port)})` };
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
      if (req.method === 'GET' && pathname === '/v1/models') {
        this.handleModels(res);
        return;
      }
      if (req.method === 'POST' && pathname === '/v1/chat/completions') {
        await this.handleChatCompletion(req, res);
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

  private handleModels(res: http.ServerResponse): void {
    const models = this.registry.list().map((meta) => ({
      id: meta.id,
      object: 'model',
      created: Math.floor(Date.now() / 1000),
      owned_by: 'ai-agent',
    }));
    writeJson(res, 200, { object: 'list', data: models });
  }

  private async handleChatCompletion(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const body = await readJson<OpenAIChatRequest>(req);
    if (typeof body.model !== 'string' || body.model.length === 0) {
      throw new HttpError(400, 'missing_model', 'model is required');
    }
    const agent = this.resolveAgent(body.model);
    if (agent === undefined) {
      throw new HttpError(404, 'unknown_model', `Model '${body.model}' not registered`);
    }
    if (!Array.isArray(body.messages) || body.messages.length === 0) {
      throw new HttpError(400, 'missing_messages', 'messages array is required');
    }
    const prompt = this.buildPrompt(body.messages);
    const formatInfo = this.resolveFormat(body, agent);
    const additionalPayload = body.payload !== undefined && typeof body.payload === 'object' && !Array.isArray(body.payload)
      ? body.payload
      : {};
    const agentPayload: Record<string, unknown> = { ...additionalPayload, prompt, format: formatInfo.format };
    if (formatInfo.schema !== undefined) agentPayload.schema = formatInfo.schema;

    const release = await this.limiter.acquire();
    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log(`chat completion aborted model=${agent.id}`, 'WRN');
    };
    req.on('aborted', onAbort);
    res.on('close', onAbort);
    const cleanup = () => {
      req.removeListener('aborted', onAbort);
      res.removeListener('close', onAbort);
    };

    const created = Math.floor(Date.now() / 1000);
    const responseId = randomUUID();
    const streamed = body.stream === true;
    let output = '';
    const accounting: AccountingEntry[] = [];
    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
      if (streamed) {
        const chunkPayload = {
          id: responseId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
                delta: { role: 'assistant', content: chunk },
                finish_reason: null,
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
      const session = await this.registry.spawnSession({
        agentId: agent.id,
        payload: agentPayload,
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
      const finalText = this.resolveContent(output, result.finalReport);
      const usage = collectUsage(accounting);
      if (streamed) {
        const finalChunk = {
          id: responseId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              delta: {},
              finish_reason: result.success ? 'stop' : 'error',
            },
          ],
        };
        writeSseChunk(res, finalChunk);
        writeSseDone(res);
      } else {
        const response: OpenAIChatResponse = {
          id: responseId,
          object: CHAT_COMPLETION_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              message: { role: 'assistant', content: finalText },
              finish_reason: result.success ? 'stop' : 'error',
            },
          ],
          usage: {
            prompt_tokens: usage.prompt,
            completion_tokens: usage.completion,
            total_tokens: usage.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, response);
      }
      this.log(`chat completion model=${agent.id} ${result.success ? 'ok' : 'error'}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`chat completion failed model=${agent.id}: ${message}`, 'ERR');
      if (streamed) {
        const chunk = {
          id: responseId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              delta: { role: 'assistant', content: message },
              finish_reason: 'error' as const,
            },
          ],
        };
        writeSseChunk(res, chunk);
        writeSseDone(res);
      } else {
        const status = err instanceof HttpError ? err.statusCode : 500;
        const code = err instanceof HttpError ? err.code : 'chat_failure';
        writeJson(res, status, { error: code, message });
      }
    } finally {
      cleanup();
      release();
    }
  }

  private resolveAgent(model: string): AgentMetadata | undefined {
    const list = this.registry.list();
    const index = new Map<string, AgentMetadata>();
    list.forEach((meta) => {
      index.set(meta.id, meta);
    });
    return index.get(model);
  }

  private buildPrompt(messages: OpenAIChatRequestMessage[]): string {
    const systemParts: string[] = [];
    const historyParts: string[] = [];
    let lastUser: string | undefined;
    messages.forEach((msg, idx) => {
      if (Object.prototype.hasOwnProperty.call(msg, 'tool_calls')) {
        throw new HttpError(400, 'tool_calls_unsupported', 'tool calls are not supported');
      }
      const text = this.stringifyContent(msg.content);
      if (msg.role === 'system') {
        if (text.length > 0) systemParts.push(text);
      } else if (msg.role === 'user') {
        if (idx === messages.length - 1) {
          lastUser = text;
        } else {
          historyParts.push(`User: ${text}`);
        }
      } else {
        historyParts.push(`Assistant: ${text}`);
      }
    });
    if (lastUser === undefined || lastUser.length === 0) {
      throw new HttpError(400, 'missing_user_prompt', 'A user message is required as the final turn');
    }
    const sections: string[] = [];
    if (systemParts.length > 0) sections.push(`${SYSTEM_PREFIX}${systemParts.join('\n')}`);
    if (historyParts.length > 0) sections.push(`${HISTORY_PREFIX}${historyParts.join('\n')}`);
    sections.push(`${USER_PREFIX}${lastUser}`);
    return sections.join('\n\n');
  }

  private resolveFormat(body: OpenAIChatRequest, agent: AgentMetadata): FormatResolution {
    const explicit = typeof body.format === 'string' && body.format.length > 0 ? body.format : undefined;
    const responseFormat = body.response_format;
    if (explicit !== undefined) {
      const schema = explicit === 'json' ? this.pickSchema(body, agent) : undefined;
      return { format: explicit, schema };
    }
    if (responseFormat?.type === 'json_object') {
      const schema = this.pickSchema(body, agent) ?? this.asObject(responseFormat.json_schema);
      if (schema === undefined) {
        throw new HttpError(400, 'missing_schema', 'JSON response_format requires json_schema');
      }
      return { format: 'json', schema };
    }
    if (agent.outputSchema !== undefined && agent.expectedOutput?.format === 'json') {
      return { format: 'json', schema: agent.outputSchema };
    }
    const fallback = agent.expectedOutput?.format ?? 'markdown';
    const schema = fallback === 'json' ? this.pickSchema(body, agent) : undefined;
    if (fallback === 'json' && schema === undefined) {
      throw new HttpError(400, 'missing_schema', 'Agent expects JSON output but no schema provided');
    }
    return { format: fallback, schema };
  }

  private pickSchema(body: OpenAIChatRequest, agent: AgentMetadata): Record<string, unknown> | undefined {
    if (agent.outputSchema !== undefined) return agent.outputSchema;
    const responseFormat = body.response_format;
    if (responseFormat?.json_schema !== undefined) {
      const obj = this.asObject(responseFormat.json_schema);
      if (obj !== undefined) return obj;
    }
    if (body.payload !== undefined) {
      const schemaCandidate = body.payload.schema;
      const obj = this.asObject(schemaCandidate);
      if (obj !== undefined) return obj;
    }
    return undefined;
  }

  private asObject(value: unknown): Record<string, unknown> | undefined {
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      return value as Record<string, unknown>;
    }
    return undefined;
  }

  private stringifyContent(content: unknown): string {
    if (typeof content === 'string') return content;
    if (Array.isArray(content)) {
      return content
        .map((item) => {
          if (typeof item === 'string') return item;
          if (item !== null && typeof item === 'object') {
            const text = (item as { text?: unknown }).text;
            if (typeof text === 'string') return text;
          }
          return '';
        })
        .filter((s) => s.length > 0)
        .join('\n');
    }
    return '';
  }

  private resolveContent(output: string, finalReport: unknown): string {
    if (finalReport !== undefined && finalReport !== null && typeof finalReport === 'object') {
      const report = finalReport as { format?: string; content?: string; content_json?: Record<string, unknown> };
      if (report.format === 'json' && report.content_json !== undefined) {
        try { return JSON.stringify(report.content_json); } catch { /* ignore */ }
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
}
