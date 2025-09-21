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

interface AnthropicMessageContent {
  type?: string;
  text?: string;
}

interface AnthropicRequestMessage {
  role: 'user' | 'assistant';
  content: string | AnthropicMessageContent[];
}

interface AnthropicRequestBody {
  model: string;
  messages: AnthropicRequestMessage[];
  system?: string | string[];
  stream?: boolean;
  format?: string;
  payload?: Record<string, unknown>;
}

interface AnthropicResponseBody {
  id: string;
  type: 'message';
  role: 'assistant';
  model: string;
  content: { type: 'text'; text: string }[];
  stop_reason: 'end_turn' | 'error';
  usage: {
    input_tokens: number;
    output_tokens: number;
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
  remoteIdentifier: 'headend:anthropic-completions',
  fatal,
  message,
  headendId,
});

const SYSTEM_PREFIX = 'System context:\n';
const HISTORY_PREFIX = 'Conversation so far:\n';
const USER_PREFIX = 'User request:\n';

const collectUsage = (entries: AccountingEntry[]): { input: number; output: number; total: number } => entries
  .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
  .reduce<{ input: number; output: number; total: number }>((acc, entry) => {
    const tokens = entry.tokens;
    acc.input += tokens.inputTokens;
    acc.output += tokens.outputTokens;
    acc.total += tokens.totalTokens;
    return acc;
  }, { input: 0, output: 0, total: 0 });

export class AnthropicCompletionsHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'anthropic-completions';
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
    this.id = `anthropic-completions:${String(options.port)}`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : 4;
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: `Anthropic chat headend (port ${String(this.options.port)})` };
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
      if (req.method === 'POST' && pathname === '/v1/messages') {
        await this.handleMessages(req, res);
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
      type: 'model',
      display_name: meta.description ?? meta.id,
    }));
    writeJson(res, 200, { data: models });
  }

  private async handleMessages(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const body = await readJson<AnthropicRequestBody>(req);
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
    const prompt = this.composePrompt(body.system, body.messages);
    const format = typeof body.format === 'string' && body.format.length > 0 ? body.format : (agent.expectedOutput?.format ?? 'markdown');
    const schema = format === 'json' ? (agent.outputSchema ?? this.extractSchema(body.payload)) : undefined;
    if (format === 'json' && schema === undefined) {
      throw new HttpError(400, 'missing_schema', 'JSON format requires schema');
    }
    const payload: Record<string, unknown> = { prompt, format };
    if (schema !== undefined) payload.schema = schema;
    const additionalPayload = body.payload !== undefined && typeof body.payload === 'object' && !Array.isArray(body.payload)
      ? body.payload
      : undefined;
    if (additionalPayload !== undefined) {
      Object.entries(additionalPayload).forEach(([key, value]) => {
        if (!(key in payload)) payload[key] = value;
      });
    }

    const release = await this.limiter.acquire();
    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log(`anthropic request aborted model=${agent.id}`, 'WRN');
    };
    req.on('aborted', onAbort);
    res.on('close', onAbort);
    const cleanup = () => {
      req.removeListener('aborted', onAbort);
      res.removeListener('close', onAbort);
    };

    const requestId = randomUUID();
    const streamed = body.stream === true;
    let output = '';
    const accounting: AccountingEntry[] = [];
    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
        if (streamed) {
          const event = {
            type: 'content_block_delta',
            content_block: { type: 'text', text_delta: chunk },
          };
          writeSseChunk(res, event);
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
      const startEvent = {
        type: 'message_start',
        message: {
          id: requestId,
          type: 'message',
          role: 'assistant',
          model: body.model,
          content: [] as unknown[],
        },
      };
      writeSseChunk(res, startEvent);
      const contentBlockStart = { type: 'content_block_start', content_block: { type: 'text' as const } };
      writeSseChunk(res, contentBlockStart);
    }

    try {
      const session = await this.registry.spawnSession({
        agentId: agent.id,
        payload,
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
      });
      const result = await session.run();
      if (abortController.signal.aborted) {
        if (!res.writableEnded && !res.writableFinished) {
          if (streamed) { writeSseDone(res); } else { writeJson(res, 499, { error: 'client_closed_request' }); }
        }
        return;
      }
      const finalText = this.resolveContent(output, result.finalReport);
      const usage = collectUsage(accounting);
      if (streamed) {
        const contentStop = { type: 'content_block_stop' as const };
        writeSseChunk(res, contentStop);
        const stopEvent = { type: 'message_stop' as const };
        writeSseChunk(res, stopEvent);
        writeSseDone(res);
      } else {
        const response: AnthropicResponseBody = {
          id: requestId,
          type: 'message',
          role: 'assistant',
          model: body.model,
          content: [{ type: 'text', text: finalText }],
          stop_reason: result.success ? 'end_turn' : 'error',
          usage: {
            input_tokens: usage.input,
            output_tokens: usage.output,
            total_tokens: usage.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, response);
      }
      this.log(`anthropic completion model=${agent.id} ${result.success ? 'ok' : 'error'}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`anthropic completion failed model=${agent.id}: ${message}`, 'ERR');
      if (streamed) {
        const errorEvent = { type: 'error', message };
        writeSseChunk(res, errorEvent);
        const contentStop = { type: 'content_block_stop' as const };
        writeSseChunk(res, contentStop);
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
    const map = new Map<string, AgentMetadata>();
    list.forEach((meta) => { map.set(meta.id, meta); });
    return map.get(model);
  }

  private composePrompt(system: string | string[] | undefined, messages: AnthropicRequestMessage[]): string {
    const systemParts: string[] = [];
    if (typeof system === 'string' && system.length > 0) systemParts.push(system);
    if (Array.isArray(system)) {
      system.filter((s): s is string => typeof s === 'string' && s.length > 0).forEach((s) => systemParts.push(s));
    }
    const historyParts: string[] = [];
    let lastUser: string | undefined;
    messages.forEach((msg, idx) => {
      const text = this.stringifyContent(msg.content);
      if (msg.role === 'user') {
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
      throw new HttpError(400, 'missing_user_prompt', 'Final user message is required');
    }
    const segments: string[] = [];
    if (systemParts.length > 0) segments.push(`${SYSTEM_PREFIX}${systemParts.join('\n')}`);
    if (historyParts.length > 0) segments.push(`${HISTORY_PREFIX}${historyParts.join('\n')}`);
    segments.push(`${USER_PREFIX}${lastUser}`);
    return segments.join('\n\n');
  }

  private stringifyContent(content: string | AnthropicMessageContent[]): string {
    if (typeof content === 'string') return content;
    return content
      .map((item) => (typeof item.text === 'string' ? item.text : ''))
      .filter((s) => s.length > 0)
      .join('\n');
  }

  private extractSchema(payload?: Record<string, unknown>): Record<string, unknown> | undefined {
    if (payload === undefined) return undefined;
    const val = payload.schema;
    if (val !== null && typeof val === 'object' && !Array.isArray(val)) {
      return val as Record<string, unknown>;
    }
    return undefined;
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
