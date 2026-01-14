interface FetchResponse {
  ok: boolean;
  status: number;
  body: { getReader: () => { read: () => Promise<{ done: boolean; value?: Uint8Array }> } } | null;
  text: () => Promise<string>;
}

type FetchFn = (input: string, init: {
  method: string;
  headers?: Record<string, string>;
  body?: string;
  signal?: AbortSignal;
}) => Promise<FetchResponse>;

type AbortControllerConstructor = new () => { signal: AbortSignal; abort: () => void };

type TextDecoderConstructor = new () => { decode: (chunk?: Uint8Array, opts?: { stream?: boolean }) => string };

declare const fetch: FetchFn;
declare const AbortController: AbortControllerConstructor;
declare const TextDecoder: TextDecoderConstructor;

type EmbedRole = 'user' | 'assistant' | 'status';

interface EmbedHistoryEntry {
  role: EmbedRole;
  content: string;
}

interface EmbedStatusEventData {
  agent: string;
  agentPath: string;
  status: string;
  message: string;
  done?: string;
  pending?: string;
  now?: string;
  timestamp: number;
}

interface EmbedDoneData {
  success: boolean;
  metrics?: {
    durationMs: number;
    tokensIn: number;
    tokensOut: number;
    toolsRun: number;
  };
  reportLength: number;
}

type EmbedClientEvent =
  | { type: 'client'; clientId: string; isNew: boolean }
  | { type: 'meta'; sessionId: string; turn?: number; agentId?: string }
  | { type: 'status'; data: EmbedStatusEventData }
  | { type: 'report'; chunk: string; report: string; index: number }
  | { type: 'done'; data: EmbedDoneData }
  | { type: 'error'; error: { code: string; message: string } }
  | { type: 'turn'; turn: number; isUser: boolean };

interface NetdataSupportConfig {
  endpoint: string;
  agentId?: string;
  statusFields?: ('status' | 'now' | 'done' | 'pending')[];
  enableChat?: boolean;
  clientId?: string;
  markers?: {
    userClass?: string;
    assistantClass?: string;
    oldClass?: string;
    newClass?: string;
    statusClass?: string;
  };
  supportsMermaid?: boolean;
  onEvent?: (event: EmbedClientEvent) => void;
}

class NetdataSupport {
  private readonly config: NetdataSupportConfig;
  private history: EmbedHistoryEntry[] = [];
  private clientId: string | null;
  private sessionId: string | null = null;
  private loading = false;
  private abortController: { signal: AbortSignal; abort: () => void } | null = null;

  constructor(config: NetdataSupportConfig) {
    this.config = config;
    this.clientId = typeof config.clientId === 'string' ? config.clientId : null;
  }

  private emitEvent(event: EmbedClientEvent): void {
    if (this.config.onEvent === undefined) return;
    this.config.onEvent(event);
  }

  async ask(message: string): Promise<void> {
    if (this.loading) {
      throw new Error('request_in_progress');
    }
    const trimmed = message.trim();
    if (trimmed.length === 0) {
      throw new Error('message_required');
    }
    this.loading = true;
    const controller = new AbortController();
    this.abortController = controller;

    const historyPayload = this.history.filter((entry) => entry.role === 'user' || entry.role === 'assistant');
    const body = {
      message: trimmed,
      clientId: this.clientId ?? undefined,
      history: historyPayload,
      agentId: this.config.agentId,
    };

    let report = '';
    const userEntry: EmbedHistoryEntry = { role: 'user', content: trimmed };
    try {
      this.emitEvent({ type: 'turn', turn: this.history.length + 1, isUser: true });
      const response = await fetch(`${this.config.endpoint.replace(/\/+$/, '')}/v1/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal: controller.signal,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(`request_failed: ${String(response.status)} ${text}`);
      }
      if (response.body === null) {
        throw new Error('missing_response_body');
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      const processEvent = (raw: string): void => {
        const lines = raw.split('\n');
        let eventName = 'message';
        const dataLines: string[] = [];
        lines.forEach((line) => {
          if (line.startsWith('event:')) {
            eventName = line.slice(6).trim();
            return;
          }
          if (line.startsWith('data:')) {
            dataLines.push(line.slice(5).trim());
          }
        });
        const dataStr = dataLines.join('\n');
        if (dataStr.length === 0) return;
        try {
          const payload = JSON.parse(dataStr) as Record<string, unknown>;
          if (eventName === 'client') {
            const nextId = typeof payload.clientId === 'string' ? payload.clientId : '';
            const isNew = payload.isNew === true;
            if (nextId.length > 0) {
              this.clientId = nextId;
              this.emitEvent({ type: 'client', clientId: nextId, isNew });
            }
            return;
          }
          if (eventName === 'meta') {
            const sessionId = typeof payload.sessionId === 'string' ? payload.sessionId : '';
            const turn = typeof payload.turn === 'number' ? payload.turn : undefined;
            const agentId = typeof payload.agentId === 'string' ? payload.agentId : undefined;
            if (sessionId.length > 0) {
              this.sessionId = sessionId;
              this.emitEvent({ type: 'meta', sessionId, turn, agentId });
            }
            if (turn !== undefined) this.emitEvent({ type: 'turn', turn, isUser: false });
            return;
          }
          if (eventName === 'status') {
            const statusPayload = payload as unknown as EmbedStatusEventData;
            this.history.push({ role: 'status', content: statusPayload.message });
            this.emitEvent({ type: 'status', data: statusPayload });
            return;
          }
          if (eventName === 'report') {
            const chunk = typeof payload.chunk === 'string' ? payload.chunk : '';
            if (chunk.length > 0) {
              report += chunk;
              const index = typeof payload.index === 'number' ? payload.index : report.length;
              this.emitEvent({ type: 'report', chunk, report, index });
            }
            return;
          }
          if (eventName === 'done') {
            const donePayload = payload as unknown as EmbedDoneData;
            this.history.push(userEntry);
            this.history.push({ role: 'assistant', content: report });
            this.emitEvent({ type: 'done', data: donePayload });
            return;
          }
          if (eventName === 'error') {
            const code = typeof payload.code === 'string' ? payload.code : 'unknown';
            const msg = typeof payload.message === 'string' ? payload.message : 'unknown error';
            this.emitEvent({ type: 'error', error: { code, message: msg } });
            return;
          }
        } catch {
          // ignore malformed events
        }
      };

      // eslint-disable-next-line functional/no-loop-statements
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx = buffer.indexOf('\n\n');
        // eslint-disable-next-line functional/no-loop-statements
        while (idx !== -1) {
          const raw = buffer.slice(0, idx);
          buffer = buffer.slice(idx + 2);
          processEvent(raw);
          idx = buffer.indexOf('\n\n');
        }
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'unknown error';
      this.emitEvent({ type: 'error', error: { code: 'request_failed', message: msg } });
      throw err;
    } finally {
      this.loading = false;
      this.abortController = null;
    }
  }

  getSessionId(): string | null {
    return this.sessionId;
  }

  setSessionId(sessionId: string): void {
    this.sessionId = sessionId;
  }

  getClientId(): string | null {
    return this.clientId;
  }

  setClientId(clientId: string): void {
    this.clientId = clientId;
  }

  getHistory(): EmbedHistoryEntry[] {
    return [...this.history];
  }

  reset(): void {
    this.history = [];
    this.sessionId = null;
  }

  abort(): void {
    this.abortController?.abort();
  }

  isLoading(): boolean {
    return this.loading;
  }
}

const globalScope = (typeof globalThis !== 'undefined' ? globalThis : {}) as Record<string, unknown>;
globalScope.NetdataSupport = NetdataSupport;
