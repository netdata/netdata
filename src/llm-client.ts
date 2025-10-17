import type { BaseLLMProvider } from './llm-providers/base.js';
import type { TurnRequest, TurnResult, ProviderConfig, LogEntry } from './types.js';

import { AnthropicProvider } from './llm-providers/anthropic.js';
import { GoogleProvider } from './llm-providers/google.js';
import { OllamaProvider } from './llm-providers/ollama.js';
import { OpenAIProvider } from './llm-providers/openai.js';
import { OpenRouterProvider } from './llm-providers/openrouter.js';
import { TestLLMProvider } from './llm-providers/test-llm.js';
import { warn } from './utils.js';

export class LLMClient {
  private static readonly OPENROUTER_HOST = 'openrouter.ai';
  private static readonly CONTENT_TYPE_EVENT_STREAM = 'text/event-stream';
  private static readonly CONTENT_TYPE_JSON = 'application/json';
  private static readonly CONTENT_TYPE_HEADER = 'content-type';
  private providers = new Map<string, BaseLLMProvider>();
  private onLog?: (entry: LogEntry) => void;
  private traceLLM: boolean;
  private currentTurn = 0;
  private currentSubturn = 0;
  // Optional pricing table to compute cost per response
  private pricing?: Partial<Record<string, Partial<Record<string, { unit?: 'per_1k'|'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
  // Tracks actual routed provider/model when using routers like OpenRouter
  private lastActualProvider?: string;
  private lastActualModel?: string;
  // Tracks last reported costs when available (e.g., OpenRouter)
  private lastCostUsd?: number;
  private lastUpstreamCostUsd?: number;
  // Tracks last parsed cache creation input tokens from upstream (e.g., Anthropic raw JSON)
  private lastCacheWriteInputTokens?: number;
  private lastMetadataTask?: Promise<void>;
  private lastTraceTask?: Promise<void>;

  constructor(
    providerConfigs: Record<string, ProviderConfig>,
    options?: {
      traceLLM?: boolean;
      onLog?: (entry: LogEntry) => void;
      pricing?: Partial<Record<string, Partial<Record<string, { unit?: 'per_1k'|'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
    }
  ) {
    this.traceLLM = options?.traceLLM ?? false;
    this.onLog = options?.onLog;
    this.pricing = options?.pricing;

    // Create traced fetch if needed
    // Always wrap fetch so we can capture routing metadata (e.g., OpenRouter actual provider)
    const tracedFetch = this.createTracedFetch();

    // Initialize providers - necessary for side effects
    // eslint-disable-next-line functional/no-loop-statements
    for (const [name, config] of Object.entries(providerConfigs)) {
      this.providers.set(name, this.createProvider(name, config, tracedFetch));
    }
  }

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const provider = this.providers.get(request.provider);
    if (provider === undefined) {
      throw new Error(`Unknown provider: ${request.provider}`);
    }

    // Log request
    this.logRequest(request);

    const startTime = Date.now();
    try {
      const result = await provider.executeTurn(request);
      await this.awaitLastMetadataTask();
      // Enrich tokens with cache write info parsed at HTTP layer when providers/SDKs omit it
      try {
        if (result.status.type === 'success') {
          const extraW = this.lastCacheWriteInputTokens;
          if (typeof extraW === 'number' && extraW > 0) {
            if (result.tokens === undefined) {
              result.tokens = { inputTokens: 0, outputTokens: 0, totalTokens: 0, cacheWriteInputTokens: extraW };
            } else if (result.tokens.cacheWriteInputTokens === undefined || result.tokens.cacheWriteInputTokens === 0) {
              result.tokens.cacheWriteInputTokens = extraW;
            }
          }
        }
      } catch { /* ignore enrichment errors */ }
      // Log response
      this.logResponse(request, result, Date.now() - startTime);
      return result;
    } catch (error) {
      await this.awaitLastMetadataTask();
      const latencyMs = Date.now() - startTime;
      const errorResult: TurnResult = {
        status: provider.mapError(error),
        latencyMs,
        messages: []
      };

      // Log error response
      this.logResponse(request, errorResult, latencyMs);
      return errorResult;
    }
  }

  setTurn(turn: number, subturn = 0): void {
    this.currentTurn = turn;
    this.currentSubturn = subturn;
  }

  async waitForMetadataCapture(): Promise<void> {
    await this.awaitLastMetadataTask();
    await this.awaitLastTraceTask();
  }

  // Expose last routed provider/model for accounting/logging
  getLastActualRouting(): { provider?: string; model?: string } {
    return { provider: this.lastActualProvider, model: this.lastActualModel };
  }

  getLastCostInfo(): { costUsd?: number; upstreamInferenceCostUsd?: number } {
    return { costUsd: this.lastCostUsd, upstreamInferenceCostUsd: this.lastUpstreamCostUsd };
  }

  private createProvider(name: string, config: ProviderConfig, tracedFetch?: typeof fetch): BaseLLMProvider {
    const effectiveType = config.type ?? name;
    
    switch (effectiveType) {
      case 'openai':
        return new OpenAIProvider(config, tracedFetch);
      case 'anthropic':
        return new AnthropicProvider(config, tracedFetch);
      case 'google':
      case 'vertex':
        return new GoogleProvider(config, tracedFetch);
      case 'openrouter':
        return new OpenRouterProvider(config, tracedFetch);
      case 'ollama':
        return new OllamaProvider(config, tracedFetch);
      case 'test-llm':
        return new TestLLMProvider(config);
      default:
        throw new Error(`Unsupported provider type: ${effectiveType}`);
    }
  }

  private createTracedFetch(): typeof fetch {
    return async (input: RequestInfo | URL, init?: RequestInit) => {
      let url = '';
      let method = 'GET';
      try {
        if (typeof input === 'string') url = input;
        else if (input instanceof URL) url = input.toString();
        else if (input instanceof Request) url = input.url;
        method = init?.method ?? 'GET';

        // Add default headers
        const headers = new Headers(init?.headers);
        if (!headers.has('Accept')) {
          headers.set('Accept', LLMClient.CONTENT_TYPE_JSON);
        }

        // Add OpenRouter attribution headers if needed
        if (url.includes(LLMClient.OPENROUTER_HOST)) {
          const defaultReferer = 'https://ai-agent.local';
          const defaultTitle = 'ai-agent';
          if (!headers.has('HTTP-Referer')) headers.set('HTTP-Referer', defaultReferer);
          if (!headers.has('X-OpenRouter-Title')) headers.set('X-OpenRouter-Title', defaultTitle);
          if (!headers.has('User-Agent')) headers.set('User-Agent', `${defaultTitle}/1.0`);
        }

        const requestInit = { ...init, headers };

        // Log request details
        if (this.traceLLM) {
          const headerObj: Record<string, string> = {};
          headers.forEach((value, key) => {
            const k = key.toLowerCase();
            if (k === 'authorization' || k === 'x-api-key' || k === 'api-key' || k === 'x-goog-api-key') {
              if (value.startsWith('Bearer ')) {
                const token = value.substring(7);
                if (token.length > 8) {
                  headerObj[k] = `Bearer ${token.substring(0, 4)}...REDACTED...${token.substring(token.length - 4)}`;
                } else {
                  headerObj[k] = `Bearer [SHORT_TOKEN]`;
                }
              } else {
                // Redact non-Bearer style or API key headers entirely
                headerObj[k] = '[REDACTED]';
              }
            } else {
              headerObj[k] = value;
            }
          });

          let bodyPretty = '';
          if (typeof requestInit.body === 'string') {
            try {
              bodyPretty = JSON.stringify(JSON.parse(requestInit.body), null, 2);
            } catch {
              bodyPretty = requestInit.body;
            }
          }

          const traceMessage = `LLM request: ${method} ${url}\nheaders: ${JSON.stringify(headerObj, null, 2)}${bodyPretty ? `\nbody: ${bodyPretty}` : ''}`;
          this.log('TRC', 'request', 'llm', `trace:${method}`, traceMessage);
        }

        const response = await fetch(input, requestInit);

        let metadataTask: Promise<void> = Promise.resolve();
        try {
          const metadataClone = response.clone();
          metadataTask = this.captureResponseMetadata(url, metadataClone);
        } catch (cloneError) {
          this.handleCloneFailure('metadata', cloneError);
        }
        this.lastMetadataTask = metadataTask;

        if (this.traceLLM) {
          try {
            const traceClone = response.clone();
            this.lastTraceTask = this.logResponseTraceAsync(method, url, traceClone);
          } catch (cloneError) {
            this.handleCloneFailure('trace', cloneError);
          }
        }

        const contentTypeHeader = response.headers.get(LLMClient.CONTENT_TYPE_HEADER);
        const contentType = typeof contentTypeHeader === 'string' ? contentTypeHeader.toLowerCase() : '';
        const shouldAwaitMetadata = !contentType.includes(LLMClient.CONTENT_TYPE_EVENT_STREAM);
        if (shouldAwaitMetadata) {
          try {
            await metadataTask;
          } catch {
            /* ignore metadata errors for superseded requests */
          } finally {
            if (this.lastMetadataTask === metadataTask) {
              this.lastMetadataTask = undefined;
            }
          }
        }

        return response;
      } catch (error) {
        if (this.traceLLM) {
          const message = error instanceof Error ? error.message : String(error);
          this.log('TRC', 'response', 'llm', `trace:${method}`, `HTTP Error: ${message}`);
        }
        throw error;
      }
    };
  }

  private async awaitLastMetadataTask(): Promise<void> {
    const pending = this.lastMetadataTask;
    if (pending === undefined) {
      return;
    }
    try {
      await pending;
    } catch (error) {
      try {
        warn(`traced fetch metadata capture failed: ${error instanceof Error ? error.message : String(error)}`);
      } catch {
        /* ignore logging failures */
      }
    } finally {
      if (this.lastMetadataTask === pending) {
        this.lastMetadataTask = undefined;
      }
    }
  }

  private async awaitLastTraceTask(): Promise<void> {
    const pending = this.lastTraceTask;
    if (pending === undefined) {
      return;
    }
    try {
      await pending;
    } catch {
      /* ignore trace errors */
    } finally {
      if (this.lastTraceTask === pending) {
        this.lastTraceTask = undefined;
      }
    }
  }

  private captureResponseMetadata(url: string, response: Response): Promise<void> {
    return (async () => {
      const isOpenRouter = url.includes(LLMClient.OPENROUTER_HOST);
        const contentTypeHeader = response.headers.get(LLMClient.CONTENT_TYPE_HEADER) ?? '';
      const contentType = contentTypeHeader.toLowerCase();

      let actualProvider: string | undefined;
      let actualModel: string | undefined;
      let costUsd: number | undefined;
      let upstreamCostUsd: number | undefined;
      let cacheWriteInputTokens: number | undefined;

      const applyUsage = (usage: Record<string, unknown> | undefined): void => {
        if (usage === undefined) {
          return;
        }
        const tokens = this.extractCacheWriteTokens(usage);
        if (tokens !== undefined) {
          cacheWriteInputTokens = tokens;
        }
        const costs = this.extractUsageCosts(usage);
        if (costs.costUsd !== undefined) {
          costUsd = costs.costUsd;
        }
        if (costs.upstreamCostUsd !== undefined) {
          upstreamCostUsd = costs.upstreamCostUsd;
        }
      };

      try {
        if (isOpenRouter && contentType.includes(LLMClient.CONTENT_TYPE_JSON)) {
          const parsed = await this.parseJsonResponse(response.clone(), 'openrouter extract usage cost failed');
          if (parsed !== undefined) {
            const routing = this.extractOpenRouterRouting(parsed.data);
            actualProvider = routing.provider ?? actualProvider;
            actualModel = routing.model ?? actualModel;
            applyUsage(this.extractUsageRecord(parsed.data));
          }
        } else if (isOpenRouter && contentType.includes(LLMClient.CONTENT_TYPE_EVENT_STREAM)) {
          try {
            const meta = await this.parseOpenRouterSseStream(response.clone(), (partial) => {
              if (partial.provider !== undefined) {
                actualProvider = partial.provider;
                this.lastActualProvider = partial.provider;
              }
              if (partial.model !== undefined) {
                actualModel = partial.model;
                this.lastActualModel = partial.model;
              }
              if (partial.costUsd !== undefined) {
                costUsd = partial.costUsd;
                this.lastCostUsd = partial.costUsd;
              }
              if (partial.upstreamCostUsd !== undefined) {
                upstreamCostUsd = partial.upstreamCostUsd;
                this.lastUpstreamCostUsd = partial.upstreamCostUsd;
              }
              if (partial.cacheWriteInputTokens !== undefined) {
                cacheWriteInputTokens = partial.cacheWriteInputTokens;
                this.lastCacheWriteInputTokens = partial.cacheWriteInputTokens;
              }
            });
            actualProvider = meta.provider ?? actualProvider;
            actualModel = meta.model ?? actualModel;
            if (meta.costUsd !== undefined) {
              costUsd = meta.costUsd;
            }
            if (meta.upstreamCostUsd !== undefined) {
              upstreamCostUsd = meta.upstreamCostUsd;
            }
            if (meta.cacheWriteInputTokens !== undefined) {
              cacheWriteInputTokens = meta.cacheWriteInputTokens;
            }
          } catch (error) {
            try {
              warn(`openrouter streaming metadata parse failed: ${error instanceof Error ? error.message : String(error)}`);
            } catch {
              /* ignore logging failures */
            }
          }
        } else if (contentType.includes(LLMClient.CONTENT_TYPE_JSON)) {
          const parsed = await this.parseJsonResponse(response.clone(), 'generic JSON usage parse failed');
          if (parsed !== undefined) {
            applyUsage(this.extractUsageRecord(parsed.data));
          }
        }
      } catch (error) {
        try {
          warn(`traced fetch post-response processing failed: ${error instanceof Error ? error.message : String(error)}`);
        } catch {
          /* ignore logging failures */
        }
      }

      this.lastActualProvider = actualProvider;
      this.lastActualModel = actualModel;
      this.lastCostUsd = costUsd;
      this.lastUpstreamCostUsd = upstreamCostUsd;
      this.lastCacheWriteInputTokens = cacheWriteInputTokens;
    })();
  }

  private handleCloneFailure(context: 'metadata' | 'trace', error: unknown): void {
    try {
      warn(`traced fetch ${context} clone failed: ${error instanceof Error ? error.message : String(error)}`);
    } catch {
      /* ignore logging failures */
    }
  }

  private logResponseTraceAsync(method: string, _url: string, response: Response): Promise<void> {
    const task = (async () => {
      const respHeaders: Record<string, string> = {};
      response.headers.forEach((value, key) => {
        const lower = key.toLowerCase();
        if (lower === 'authorization' || lower === 'x-api-key' || lower === 'api-key' || lower === 'x-goog-api-key') {
          if (value.startsWith('Bearer ')) {
            const token = value.substring(7);
            respHeaders[lower] = token.length > 8
              ? `Bearer ${token.substring(0, 4)}...REDACTED...${token.substring(token.length - 4)}`
              : 'Bearer [SHORT_TOKEN]';
          } else {
            respHeaders[lower] = '[REDACTED]';
          }
        } else {
          respHeaders[lower] = value;
        }
      });

      let traceMessage = `LLM response: ${String(response.status)} ${response.statusText}\nheaders: ${JSON.stringify(respHeaders, null, 2)}`;
      const contentType = response.headers.get(LLMClient.CONTENT_TYPE_HEADER) ?? '';
      const lowered = contentType.toLowerCase();
      if (lowered.includes(LLMClient.CONTENT_TYPE_JSON)) {
        try {
          const text = await response.text();
          let body = text;
          try {
            body = JSON.stringify(JSON.parse(text), null, 2);
          } catch {
            /* leave body as raw text */
          }
          traceMessage += `\nbody: ${body}`;
        } catch {
          traceMessage += `\ncontent-type: ${contentType}`;
        }
      } else if (lowered.includes(LLMClient.CONTENT_TYPE_EVENT_STREAM)) {
        try {
          const raw = await response.text();
          traceMessage += `\nraw-sse: ${raw}`;
        } catch {
          traceMessage += `\ncontent-type: ${contentType}`;
        }
      } else {
        traceMessage += `\ncontent-type: ${contentType}`;
      }

      this.log('TRC', 'response', 'llm', `trace:${method}`, traceMessage);
    })();

    void (async () => {
      try {
        await task;
      } catch (error: unknown) {
        try {
          warn(`traced fetch response trace failed: ${error instanceof Error ? error.message : String(error)}`);
        } catch {
          /* ignore logging failures */
        }
      }
    })();
    return task;
  }

  private isPlainObject(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
  }

  private toPositiveInteger(value: unknown): number | undefined {
    if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
      return undefined;
    }
    return Math.trunc(value);
  }

  private toNonNegativeNumber(value: unknown): number | undefined {
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      return undefined;
    }
    return value;
  }

  private extractUsageRecord(record: Record<string, unknown>): Record<string, unknown> | undefined {
    const candidate = (record as { usage?: unknown }).usage;
    return this.isPlainObject(candidate) ? candidate : undefined;
  }

  private extractCacheWriteTokens(usage?: Record<string, unknown>): number | undefined {
    if (usage === undefined) {
      return undefined;
    }
    const directKeys = [
      'cacheWriteInputTokens',
      'cacheCreationInputTokens',
      'cache_creation_input_tokens',
      'cache_write_input_tokens'
    ];
    const direct = directKeys
      .map((key) => this.toPositiveInteger(usage[key]))
      .find((value) => value !== undefined);
    if (direct !== undefined) {
      return direct;
    }
    const nestedKeys = ['cacheCreation', 'cache_creation'];
    return nestedKeys
      .map((key) => {
        const nested = usage[key];
        if (!this.isPlainObject(nested)) {
          return undefined;
        }
        return this.toPositiveInteger((nested as { ephemeral_5m_input_tokens?: unknown }).ephemeral_5m_input_tokens);
      })
      .find((value) => value !== undefined);
  }

  private extractUsageCosts(usage?: Record<string, unknown>): { costUsd?: number; upstreamCostUsd?: number } {
    if (usage === undefined) {
      return {};
    }
    const cost = this.toNonNegativeNumber((usage as { cost?: unknown }).cost);
    const details = this.getNestedRecord(usage, 'cost_details');
    const upstream = details !== undefined
      ? this.toNonNegativeNumber((details as { upstream_inference_cost?: unknown }).upstream_inference_cost)
      : undefined;
    return { costUsd: cost, upstreamCostUsd: upstream };
  }

  private extractOpenRouterRouting(record: Record<string, unknown>): { provider?: string; model?: string } {
    const provider = this.getString(record, 'provider');
    const model = this.getString(record, 'model');
    const choice = this.getFirstRecord((record as { choices?: unknown }).choices);
    const choiceProvider = choice !== undefined ? this.getString(choice, 'provider') : undefined;
    const choiceModel = choice !== undefined ? this.getString(choice, 'model') : undefined;
    const errorRecord = this.getNestedRecord(record, 'error');
    const errorMetadata = errorRecord !== undefined ? this.getNestedRecord(errorRecord, 'metadata') : undefined;
    const errorProvider = errorMetadata !== undefined ? this.getString(errorMetadata, 'provider_name') : undefined;
    return {
      provider: provider ?? choiceProvider ?? errorProvider ?? undefined,
      model: model ?? choiceModel ?? undefined
    };
  }

  private getString(record: Record<string, unknown>, key: string): string | undefined {
    const value = record[key];
    return typeof value === 'string' && value.length > 0 ? value : undefined;
  }

  private getNestedRecord(record: Record<string, unknown>, key: string): Record<string, unknown> | undefined {
    const value = record[key];
    return this.isPlainObject(value) ? value : undefined;
  }

  private getFirstRecord(value: unknown): Record<string, unknown> | undefined {
    if (!Array.isArray(value)) {
      return undefined;
    }
    return value.find((item): item is Record<string, unknown> => this.isPlainObject(item));
  }

  private async parseOpenRouterSseStream(
    response: Response,
    onPartial: (partial: { provider?: string; model?: string; costUsd?: number; upstreamCostUsd?: number; cacheWriteInputTokens?: number }) => void
  ): Promise<{ provider?: string; model?: string; costUsd?: number; upstreamCostUsd?: number; cacheWriteInputTokens?: number }> {
    const body = response.body;
    if (body === null) {
      return {};
    }
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    const aggregate: { provider?: string; model?: string; costUsd?: number; upstreamCostUsd?: number; cacheWriteInputTokens?: number } = {};

    const applyPartial = (partial: { provider?: string; model?: string; costUsd?: number; upstreamCostUsd?: number; cacheWriteInputTokens?: number }): void => {
      if (partial.provider !== undefined) {
        aggregate.provider = partial.provider;
      }
      if (partial.model !== undefined) {
        aggregate.model = partial.model;
      }
      if (partial.costUsd !== undefined) {
        aggregate.costUsd = partial.costUsd;
      }
      if (partial.upstreamCostUsd !== undefined) {
        aggregate.upstreamCostUsd = partial.upstreamCostUsd;
      }
      if (partial.cacheWriteInputTokens !== undefined) {
        aggregate.cacheWriteInputTokens = partial.cacheWriteInputTokens;
      }
      onPartial(partial);
    };

    const processLine = (line: string): void => {
      const trimmed = line.trim();
      if (!trimmed.startsWith('data:')) {
        return;
      }
      const payload = trimmed.slice(5).trim();
      if (payload.length === 0 || payload === '[DONE]') {
        return;
      }
      const parsed = this.parseJsonString(payload);
      if (parsed === undefined) {
        return;
      }
      const partial: { provider?: string; model?: string; costUsd?: number; upstreamCostUsd?: number; cacheWriteInputTokens?: number } = {};
      const routing = this.extractOpenRouterRouting(parsed);
      if (routing.provider !== undefined) {
        partial.provider = routing.provider;
      }
      if (routing.model !== undefined) {
        partial.model = routing.model;
      }
      const usage = this.extractUsageRecord(parsed);
      const costs = this.extractUsageCosts(usage);
      if (costs.costUsd !== undefined) {
        partial.costUsd = costs.costUsd;
      }
      if (costs.upstreamCostUsd !== undefined) {
        partial.upstreamCostUsd = costs.upstreamCostUsd;
      }
      const tokens = this.extractCacheWriteTokens(usage);
      if (tokens !== undefined) {
        partial.cacheWriteInputTokens = tokens;
      }
      if (partial.provider !== undefined || partial.model !== undefined || partial.costUsd !== undefined || partial.upstreamCostUsd !== undefined || partial.cacheWriteInputTokens !== undefined) {
        applyPartial(partial);
      }
    };

    try {
      let done = false;
      // eslint-disable-next-line functional/no-loop-statements -- streaming parser requires manual iteration
      while (!done) {
        const { value, done: readerDone } = await reader.read();
        done = readerDone;
        if (value !== undefined) {
          buffer += decoder.decode(value, { stream: !done });
          const segments = buffer.split(/\r?\n/);
          buffer = segments.pop() ?? '';
          segments.forEach((segment) => { processLine(segment); });
        }
        const hasRouting = aggregate.provider !== undefined && aggregate.model !== undefined;
        const hasCostInfo = aggregate.costUsd !== undefined || aggregate.upstreamCostUsd !== undefined;
        if (hasRouting && hasCostInfo) {
          void (async () => {
            try {
              await reader.cancel();
            } catch {
              /* ignore cancel failure */
            }
          })();
          break;
        }
      }
      if (buffer.trim().length > 0) {
        processLine(buffer);
      }
    } finally {
      try {
        reader.releaseLock();
      } catch {
        /* ignore release errors */
      }
    }

    return aggregate;
  }

  private async parseJsonResponse(response: Response, warnLabel?: string): Promise<{ raw: string; data: Record<string, unknown> } | undefined> {
    try {
      const raw = await response.text();
      const parsed = JSON.parse(raw) as unknown;
      if (!this.isPlainObject(parsed)) {
        return undefined;
      }
      return { raw, data: parsed };
    } catch (error) {
      if (warnLabel !== undefined) {
        try {
          warn(`${warnLabel}: ${error instanceof Error ? error.message : String(error)}`);
        } catch {
          /* ignore logging failures */
        }
      }
      return undefined;
    }
  }

  private parseJsonString(payload: string): Record<string, unknown> | undefined {
    try {
      const parsed = JSON.parse(payload) as unknown;
      return this.isPlainObject(parsed) ? parsed : undefined;
    } catch {
      return undefined;
    }
  }

  private logRequest(request: TurnRequest): void {
    // Calculate payload size
    const messagesStr = JSON.stringify(request.messages);
    const totalBytes = new TextEncoder().encode(messagesStr).length;

    const isFinalTurn = request.isFinalTurn === true ? ' (final turn)' : '';
    const message = `messages ${String(request.messages.length)}, ${String(totalBytes)} bytes${isFinalTurn}`;
    
    // Reset routed provider info at the start of each request to avoid stale attribution
    this.lastActualProvider = undefined;
    this.lastActualModel = undefined;
    this.lastCostUsd = undefined;
    this.lastUpstreamCostUsd = undefined;
    this.lastCacheWriteInputTokens = undefined;

    this.log('VRB', 'request', 'llm', `${request.provider}:${request.model}`, message);
  }

  private logResponse(request: TurnRequest, result: TurnResult, latencyMs: number): void {
    const routed = this.getLastActualRouting();
    const remoteId = (request.provider === 'openrouter' && routed.provider !== undefined)
      ? `${request.provider}/${routed.provider}:${request.model}`
      : `${request.provider}:${request.model}`;
    
    if (result.status.type === 'success') {
      const tokens = result.tokens;
      const inputTokens = tokens?.inputTokens ?? 0;
      const outputTokens = tokens?.outputTokens ?? 0;
      const cachedTokens = tokens?.cachedTokens ?? 0;
      const cacheRead = tokens?.cacheReadInputTokens ?? cachedTokens;
      const cacheWrite = tokens?.cacheWriteInputTokens ?? 0;
      const responseBytes = result.response !== undefined ? new TextEncoder().encode(result.response).length : 0;
      // Do not include routed provider/model details here; keep log concise up to bytes
      let message = `input ${String(inputTokens)}, output ${String(outputTokens)}, cacheR ${String(cacheRead)}, cacheW ${String(cacheWrite)}, cached ${String(cachedTokens)} tokens, ${String(latencyMs)}ms, ${String(responseBytes)} bytes`;
      if (typeof result.stopReason === 'string' && result.stopReason.length > 0) {
        message += `, stop=${result.stopReason}`;
      }
      // Compute cost from pricing for all providers (5 decimals); prefer router-reported when present
      const computeCost = (): number | undefined => {
        try {
          if (this.pricing === undefined) return undefined;
          const routedProv = routed.provider;
          const routedModel = routed.model;
          const effProvider = (request.provider === 'openrouter' && typeof routedProv === 'string' && routedProv.length > 0) ? routedProv : request.provider;
          const effModel = (request.provider === 'openrouter' && typeof routedModel === 'string' && routedModel.length > 0) ? routedModel : request.model;
          const prov = this.pricing[effProvider];
          const model = prov !== undefined ? prov[effModel] : undefined;
          if (model === undefined) return undefined;
          const denom = (model.unit === 'per_1k') ? 1000 : 1_000_000;
          const pIn = model.prompt ?? 0;
          const pOut = model.completion ?? 0;
          const pR = model.cacheRead ?? 0;
          const pW = model.cacheWrite ?? 0;
          const cost = (pIn * inputTokens + pOut * outputTokens + pR * cacheRead + pW * cacheWrite) / denom;
          return Number.isFinite(cost) ? cost : undefined;
        } catch { return undefined; }
      };
      const reported = this.getLastCostInfo().costUsd;
      const computed = computeCost();
      const costToPrint = (typeof reported === 'number') ? reported : computed;
      if (typeof costToPrint === 'number') {
        message += `, cost $${costToPrint.toFixed(5)}`;
      }
      // Also append upstream cost for OpenRouter when available
      if (request.provider === 'openrouter') {
        const upstream = this.getLastCostInfo().upstreamInferenceCostUsd;
        if (typeof upstream === 'number' && upstream > 0) {
          message += `, upstream $${upstream.toFixed(5)}`;
        }
      }
      this.log('VRB', 'response', 'llm', remoteId, message);
    } else {
      const fatal = result.status.type === 'auth_error' || result.status.type === 'quota_exceeded';
      const statusMessage = 'message' in result.status ? result.status.message : result.status.type;
      let message = `error [${result.status.type.toUpperCase()}] ${statusMessage} (waited ${String(latencyMs)} ms)`;
      if (typeof result.stopReason === 'string' && result.stopReason.length > 0) {
        message += `, stop=${result.stopReason}`;
      }
      this.log(fatal ? 'ERR' : 'WRN', 'response', 'llm', remoteId, message, fatal);
    }
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
}
