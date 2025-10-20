import type { BaseLLMProvider } from './llm-providers/base.js';
import type {
  TurnRequest,
  TurnResult,
  ProviderConfig,
  LogEntry,
  ReasoningLevel,
  ProviderReasoningMapping,
  ProviderReasoningValue,
  ProviderTurnMetadata,
} from './types.js';

import { AnthropicProvider } from './llm-providers/anthropic.js';
import { GoogleProvider } from './llm-providers/google.js';
import { OllamaProvider } from './llm-providers/ollama.js';
import { OpenAIProvider } from './llm-providers/openai.js';
import { OpenRouterProvider } from './llm-providers/openrouter.js';
import { TestLLMProvider } from './llm-providers/test-llm.js';
import { warn } from './utils.js';

export class LLMClient {
  private static readonly CONTENT_TYPE_EVENT_STREAM = 'text/event-stream';
  private static readonly CONTENT_TYPE_JSON = 'application/json';
  private static readonly CONTENT_TYPE_HEADER = 'content-type';
  private providers = new Map<string, BaseLLMProvider>();
  private metadataCollectors = new Map<string, (payload: { url: string; response: Response }) => Promise<ProviderTurnMetadata | undefined>>();
  private onLog?: (entry: LogEntry) => void;
  private traceLLM: boolean;
  private currentTurn = 0;
  private currentSubturn = 0;
  private lastRouting?: { provider?: string; model?: string };
  private lastRoutingFromMetadata?: { provider?: string; model?: string };
  private lastCostInfo?: { costUsd?: number; upstreamInferenceCostUsd?: number };
  private lastCacheWriteInputTokens?: number;
  // Optional pricing table to compute cost per response
  private pricing?: Partial<Record<string, Partial<Record<string, { unit?: 'per_1k'|'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
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

    // Initialize providers - necessary for side effects
    // eslint-disable-next-line functional/no-loop-statements
    for (const [name, config] of Object.entries(providerConfigs)) {
      const tracedFetch = this.createTracedFetch(name);
      const provider = this.createProvider(name, config, tracedFetch);
      const collector = provider.getResponseMetadataCollector();
      if (collector !== undefined) {
        this.metadataCollectors.set(name, async (payload) => {
          try {
            const result = await collector(payload);
            if (result !== undefined) {
              return result;
            }
            return undefined;
          } catch (error) {
            try {
              warn(`metadata collector failed for provider '${name}': ${error instanceof Error ? error.message : String(error)}`);
            } catch {
              /* ignore logging failures */
            }
            return undefined;
          }
        });
      }
      this.providers.set(name, provider);
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
      if (result.status.type === 'success') {
        const extraCacheWrite = this.lastCacheWriteInputTokens;
        if (typeof extraCacheWrite === 'number' && extraCacheWrite > 0) {
          if (result.tokens === undefined) {
            result.tokens = {
              inputTokens: 0,
              outputTokens: 0,
              totalTokens: 0,
              cacheWriteInputTokens: extraCacheWrite,
            };
          } else if (result.tokens.cacheWriteInputTokens === undefined || result.tokens.cacheWriteInputTokens === 0) {
            result.tokens.cacheWriteInputTokens = extraCacheWrite;
          }
        }
      }
      this.lastCacheWriteInputTokens = undefined;
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

  private createProvider(
    name: string,
    config: ProviderConfig,
    tracedFetch: typeof fetch
  ): BaseLLMProvider {
    switch (config.type) {
      case 'openai':
        return new OpenAIProvider(config, tracedFetch);
      case 'anthropic':
        return new AnthropicProvider(config, tracedFetch);
      case 'google':
        return new GoogleProvider(config, tracedFetch);
      case 'openrouter':
        return new OpenRouterProvider(config, tracedFetch);
      case 'ollama':
        return new OllamaProvider(config, tracedFetch);
      case 'test-llm':
        return new TestLLMProvider(config);
      default:
        const exhaustiveCheck: never = config.type;
        throw new Error(`Unsupported provider type: ${String(exhaustiveCheck)}`);
    }
  }

  private createTracedFetch(providerName: string): typeof fetch {
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

        const provider = this.providers.get(providerName);
        if (provider !== undefined && typeof provider.prepareFetch === 'function') {
          try {
            const prep = provider.prepareFetch({ url, init: init ?? {} });
            if (prep?.headers !== undefined) {
              Object.entries(prep.headers).forEach(([key, value]) => {
                if (!headers.has(key)) {
                  headers.set(key, value);
                }
              });
            }
          } catch (error) {
            try {
              warn(`provider prepareFetch failed for '${providerName}': ${error instanceof Error ? error.message : String(error)}`);
            } catch {
              /* ignore logging failures */
            }
          }
        }

        if (url.includes('openrouter.ai')) {
          const defaultReferer = process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local';
          const defaultTitle = process.env.OPENROUTER_TITLE ?? 'ai-agent';
          if (!headers.has('HTTP-Referer')) {
            headers.set('HTTP-Referer', defaultReferer);
          }
          if (!headers.has('X-OpenRouter-Title')) {
            headers.set('X-OpenRouter-Title', defaultTitle);
          }
          if (!headers.has('User-Agent')) {
            headers.set('User-Agent', `${defaultTitle}/1.0`);
          }
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

        const contentTypeHeader = response.headers.get(LLMClient.CONTENT_TYPE_HEADER);
        const contentType = typeof contentTypeHeader === 'string' ? contentTypeHeader.toLowerCase() : '';

        let metadataTask: Promise<void> | undefined;
        const collector = this.metadataCollectors.get(providerName);
        if (collector !== undefined) {
          try {
            const metadataClone = response.clone();
            metadataTask = (async () => {
              const metadataResult = await collector({ url, response: metadataClone });
              if (metadataResult !== undefined && this.isPlainObject(metadataResult)) {
                const candidate = metadataResult as ProviderTurnMetadata;
                if (candidate.actualProvider !== undefined || candidate.actualModel !== undefined) {
                  this.lastRouting = {
                    provider: candidate.actualProvider,
                    model: candidate.actualModel,
                  };
                  this.lastRoutingFromMetadata = {
                    provider: candidate.actualProvider,
                    model: candidate.actualModel,
                  };
                }
                if (candidate.reportedCostUsd !== undefined || candidate.upstreamCostUsd !== undefined) {
                  this.lastCostInfo = {
                    costUsd: candidate.reportedCostUsd,
                    upstreamInferenceCostUsd: candidate.upstreamCostUsd,
                  };
                }
                if (candidate.cacheWriteInputTokens !== undefined && candidate.cacheWriteInputTokens > 0) {
                  this.lastCacheWriteInputTokens = candidate.cacheWriteInputTokens;
                }
              }
            })();
          } catch (cloneError) {
            this.handleCloneFailure('metadata', cloneError);
            metadataTask = undefined;
          }
        }

        if (metadataTask === undefined && url.includes('openrouter.ai') && contentType.includes(LLMClient.CONTENT_TYPE_EVENT_STREAM)) {
          try {
            const sseClone = response.clone();
            metadataTask = (async () => {
              try {
                const sseText = await sseClone.text();
                const metadata = this.parseOpenRouterSsePayload(sseText);
                if (metadata !== undefined) {
                  if (metadata.actualProvider !== undefined || metadata.actualModel !== undefined) {
                    this.lastRouting = {
                      provider: metadata.actualProvider,
                      model: metadata.actualModel,
                    };
                    this.lastRoutingFromMetadata = {
                      provider: metadata.actualProvider,
                      model: metadata.actualModel,
                    };
                  }
                  if (metadata.reportedCostUsd !== undefined || metadata.upstreamCostUsd !== undefined) {
                    this.lastCostInfo = {
                      costUsd: metadata.reportedCostUsd,
                      upstreamInferenceCostUsd: metadata.upstreamCostUsd,
                    };
                  }
                  if (metadata.cacheWriteInputTokens !== undefined && metadata.cacheWriteInputTokens > 0) {
                    this.lastCacheWriteInputTokens = metadata.cacheWriteInputTokens;
                  }
                }
              } catch {
                /* ignore SSE parse fallback errors */
              }
            })();
          } catch (cloneError) {
            this.handleCloneFailure('metadata', cloneError);
            metadataTask = undefined;
          }
        }

        if (metadataTask !== undefined) {
          const pendingTask = metadataTask;
          const wrapperRef: { current?: Promise<void> } = {};
          wrapperRef.current = (async () => {
            try {
              await pendingTask;
            } finally {
              if (this.lastMetadataTask === wrapperRef.current) {
                this.lastMetadataTask = undefined;
              }
            }
          })();
          this.lastMetadataTask = wrapperRef.current;
        }

        if (this.traceLLM) {
          try {
            const traceClone = response.clone();
            this.lastTraceTask = this.logResponseTraceAsync(method, url, traceClone);
          } catch (cloneError) {
            this.handleCloneFailure('trace', cloneError);
          }
        }

        const shouldAwaitMetadata = metadataTask !== undefined && !contentType.includes(LLMClient.CONTENT_TYPE_EVENT_STREAM);
        if (shouldAwaitMetadata && metadataTask !== undefined) {
          try {
            await metadataTask;
          } catch {
            /* ignore metadata errors for superseded requests */
          }
        }

        if (this.lastRouting === undefined && contentType.includes(LLMClient.CONTENT_TYPE_JSON)) {
          try {
            const fallbackText = await response.clone().text();
            let parsedFallback: Record<string, unknown> | undefined;
            try {
              const candidate: unknown = JSON.parse(fallbackText);
              parsedFallback = this.isPlainObject(candidate) ? candidate : undefined;
            } catch {
              parsedFallback = undefined;
            }
            if (parsedFallback !== undefined) {
              const providerCandidate = this.selectProviderFromRecord(parsedFallback);
              if (providerCandidate !== undefined) {
                this.lastRouting = providerCandidate;
                this.lastRoutingFromMetadata = { ...providerCandidate };
              }
              const costCandidate = this.selectCostFromRecord(parsedFallback);
              if (costCandidate !== undefined) {
                this.lastCostInfo = costCandidate;
              }
              const cacheWriteCandidate = this.selectCacheWriteFromRecord(parsedFallback);
              if (cacheWriteCandidate !== undefined) {
                this.lastCacheWriteInputTokens = cacheWriteCandidate;
              }
            }
          } catch {
            /* ignore fallback parse errors */
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




  private logRequest(request: TurnRequest): void {
    // Calculate payload size
    const messagesStr = JSON.stringify(request.messages);
    const totalBytes = new TextEncoder().encode(messagesStr).length;

    const isFinalTurn = request.isFinalTurn === true ? ' (final turn)' : '';
    const message = `messages ${String(request.messages.length)}, ${String(totalBytes)} bytes${isFinalTurn}`;

    this.log('VRB', 'request', 'llm', `${request.provider}:${request.model}`, message);
  }

  private logResponse(request: TurnRequest, result: TurnResult, latencyMs: number): void {
    const metadata = result.providerMetadata;
    const providerSegment = metadata?.actualProvider !== undefined && metadata.actualProvider.length > 0 && metadata.actualProvider !== request.provider
      ? `${request.provider}/${metadata.actualProvider}`
      : request.provider;
    const modelSegment = metadata?.actualModel ?? request.model;
    const remoteId = `${providerSegment}:${modelSegment}`;
    if (metadata !== undefined && (metadata.actualProvider !== undefined || metadata.actualModel !== undefined)) {
      this.lastRouting = {
        provider: metadata.actualProvider,
        model: metadata.actualModel,
      };
      this.lastRoutingFromMetadata = {
        provider: metadata.actualProvider,
        model: metadata.actualModel,
      };
    }
    const computedCost = this.computeCostFromPricing(request, result, metadata);
    const reportedCost = metadata?.reportedCostUsd;
    const costToUse = typeof reportedCost === 'number' ? reportedCost : computedCost;

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
      if (typeof costToUse === 'number') {
        message += `, cost $${costToUse.toFixed(5)}`;
      }
      if (metadata?.upstreamCostUsd !== undefined && metadata.upstreamCostUsd > 0) {
        message += `, upstream $${metadata.upstreamCostUsd.toFixed(5)}`;
      }
      this.lastCostInfo = (costToUse !== undefined || metadata?.upstreamCostUsd !== undefined)
        ? { costUsd: costToUse, upstreamInferenceCostUsd: metadata?.upstreamCostUsd }
        : undefined;
      this.log('VRB', 'response', 'llm', remoteId, message);
    } else {
      const fatal = result.status.type === 'auth_error' || result.status.type === 'quota_exceeded';
      const statusMessage = 'message' in result.status ? result.status.message : result.status.type;
      let message = `error [${result.status.type.toUpperCase()}] ${statusMessage} (waited ${String(latencyMs)} ms)`;
      if (typeof result.stopReason === 'string' && result.stopReason.length > 0) {
        message += `, stop=${result.stopReason}`;
      }
      this.lastCostInfo = (costToUse !== undefined || metadata?.upstreamCostUsd !== undefined)
        ? { costUsd: costToUse, upstreamInferenceCostUsd: metadata?.upstreamCostUsd }
        : undefined;
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

  resolveReasoningValue(
    providerName: string,
    context: { level: ReasoningLevel; mapping?: ProviderReasoningMapping | null; maxOutputTokens?: number }
  ): ProviderReasoningValue | null | undefined {
    const provider = this.providers.get(providerName);
    if (provider === undefined) {
      const { mapping } = context;
      if (mapping === null) return null;
      if (Array.isArray(mapping)) {
        const levels: ReasoningLevel[] = ['minimal', 'low', 'medium', 'high'];
        const index = levels.indexOf(context.level);
        return mapping[index >= 0 ? index : 0];
      }
      return mapping;
    }
    return provider.resolveReasoningValue(context);
  }

  shouldAutoEnableReasoningStream(providerName: string, level: ReasoningLevel | undefined): boolean {
    const provider = this.providers.get(providerName);
    if (provider === undefined) return false;
    return provider.shouldAutoEnableReasoningStream(level);
  }

  private selectProviderFromRecord(record: Record<string, unknown>): { provider?: string; model?: string } | undefined {
    const directProvider = typeof record.provider === 'string' && record.provider.length > 0 ? record.provider : undefined;
    const directModel = typeof record.model === 'string' && record.model.length > 0 ? record.model : undefined;
    if (directProvider !== undefined || directModel !== undefined) {
      return { provider: directProvider, model: directModel };
    }
    const dataCandidate = record.data;
    if (this.isPlainObject(dataCandidate)) {
      return this.selectProviderFromRecord(dataCandidate);
    }
    return undefined;
  }

  private selectCostFromRecord(record: Record<string, unknown>): { costUsd?: number; upstreamInferenceCostUsd?: number } | undefined {
    const usageCandidate = record.usage;
    if (!this.isPlainObject(usageCandidate)) return undefined;
    const cost = typeof usageCandidate.cost === 'number' && Number.isFinite(usageCandidate.cost) ? usageCandidate.cost : undefined;
    const detailsCandidate = usageCandidate.cost_details;
    const details = this.isPlainObject(detailsCandidate) ? detailsCandidate : undefined;
    const upstream = details !== undefined && typeof details.upstream_inference_cost === 'number' && Number.isFinite(details.upstream_inference_cost)
      ? details.upstream_inference_cost
      : undefined;
    if (cost !== undefined || upstream !== undefined) {
      return { costUsd: cost, upstreamInferenceCostUsd: upstream };
    }
    return undefined;
  }

  private extractUsageRecord(record: Record<string, unknown>): Record<string, unknown> | undefined {
    const direct = record.usage;
    if (this.isPlainObject(direct)) {
      return direct;
    }
    const dataCandidate = record.data;
    if (this.isPlainObject(dataCandidate)) {
      const nestedUsage = dataCandidate.usage;
      if (this.isPlainObject(nestedUsage)) {
        return nestedUsage;
      }
    }
    return undefined;
  }

  private selectCacheWriteFromRecord(record: Record<string, unknown>): number | undefined {
    const usageCandidate = record.usage;
    if (!this.isPlainObject(usageCandidate)) return undefined;
    const usage = usageCandidate;
    const candidates: unknown[] = [
      usage.cacheWriteInputTokens,
      usage.cacheCreationInputTokens,
      usage.cache_creation_input_tokens,
      usage.cache_write_input_tokens,
    ];
    const parseCandidate = (value: unknown): number | undefined => {
      if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
        return value;
      }
      if (typeof value === 'string') {
        const parsed = Number(value);
        if (Number.isFinite(parsed) && parsed > 0) {
          return parsed;
        }
      }
      return undefined;
    };
    const direct = candidates
      .map((candidate) => parseCandidate(candidate))
      .find((value): value is number => value !== undefined);
    if (direct !== undefined) {
      return direct;
    }
    const nested = this.isPlainObject(usage.cacheCreation)
      ? usage.cacheCreation
      : this.isPlainObject(usage.cache_creation)
        ? usage.cache_creation
        : undefined;
    if (this.isPlainObject(nested)) {
      const nestedValue = parseCandidate(nested.ephemeral_5m_input_tokens);
      if (nestedValue !== undefined) {
        return nestedValue;
      }
    }
    return undefined;
  }

  private parseOpenRouterSsePayload(payload: string): ProviderTurnMetadata | undefined {
    const lines = payload.split('\n');
    let metadata: ProviderTurnMetadata | undefined;
    const apply = (partial: ProviderTurnMetadata): void => {
      metadata ??= {};
      if (partial.actualProvider !== undefined) {
        metadata.actualProvider = partial.actualProvider;
      }
      if (partial.actualModel !== undefined) {
        metadata.actualModel = partial.actualModel;
      }
      if (partial.reportedCostUsd !== undefined) {
        metadata.reportedCostUsd = partial.reportedCostUsd;
      }
      if (partial.upstreamCostUsd !== undefined) {
        metadata.upstreamCostUsd = partial.upstreamCostUsd;
      }
      if (partial.cacheWriteInputTokens !== undefined) {
        metadata.cacheWriteInputTokens = partial.cacheWriteInputTokens;
      }
    };
    lines.forEach((rawLine) => {
      const line = rawLine.trim();
      if (!line.startsWith('data:')) return;
      const body = line.slice(5).trim();
      if (body.length === 0 || body === '[DONE]') return;
      let parsed: Record<string, unknown> | undefined;
      try {
        const candidate: unknown = JSON.parse(body);
        parsed = this.isPlainObject(candidate) ? candidate : undefined;
      } catch {
        parsed = undefined;
      }
      if (parsed === undefined) return;
      const routing = this.selectProviderFromRecord(parsed);
      const usageRecord = this.extractUsageRecord(parsed);
      const costs = this.selectCostFromRecord(parsed);
      const cacheWrite = this.isPlainObject(usageRecord)
        ? this.selectCacheWriteFromRecord({ usage: usageRecord })
        : undefined;
      apply({
        actualProvider: routing?.provider,
        actualModel: routing?.model,
        reportedCostUsd: costs?.costUsd,
        upstreamCostUsd: costs?.upstreamInferenceCostUsd,
        cacheWriteInputTokens: cacheWrite,
      });
    });
    return metadata;
  }

  getLastActualRouting(): { provider?: string; model?: string } | undefined {
    if (this.lastRoutingFromMetadata !== undefined) {
      return { ...this.lastRoutingFromMetadata };
    }
    if (this.lastRouting === undefined) return undefined;
    return { ...this.lastRouting };
  }

  getLastCostInfo(): { costUsd?: number; upstreamInferenceCostUsd?: number } | undefined {
    if (this.lastCostInfo === undefined) return undefined;
    return { ...this.lastCostInfo };
  }

  private computeCostFromPricing(
    request: TurnRequest,
    result: TurnResult,
    metadata?: ProviderTurnMetadata
  ): number | undefined {
    try {
      if (this.pricing === undefined) return undefined;
      if (result.tokens === undefined) return undefined;
      const effectiveProvider = (metadata !== undefined && typeof metadata.actualProvider === 'string' && metadata.actualProvider.length > 0)
        ? metadata.actualProvider
        : request.provider;
      const effectiveModel = (metadata !== undefined && typeof metadata.actualModel === 'string' && metadata.actualModel.length > 0)
        ? metadata.actualModel
        : request.model;
      const providerPricing = this.pricing[effectiveProvider];
      const modelPricing = providerPricing !== undefined ? providerPricing[effectiveModel] : undefined;
      if (modelPricing === undefined) return undefined;
      const denom = modelPricing.unit === 'per_1k' ? 1000 : 1_000_000;
      const pIn = modelPricing.prompt ?? 0;
      const pOut = modelPricing.completion ?? 0;
      const pRead = modelPricing.cacheRead ?? 0;
      const pWrite = modelPricing.cacheWrite ?? 0;
      const tokens = result.tokens;
      const cacheReadTokens = typeof tokens.cacheReadInputTokens === 'number'
        ? tokens.cacheReadInputTokens
        : (tokens.cachedTokens ?? 0);
      const cacheWriteTokens = typeof tokens.cacheWriteInputTokens === 'number'
        ? tokens.cacheWriteInputTokens
        : 0;
      const cost = (
        pIn * tokens.inputTokens
        + pOut * tokens.outputTokens
        + pRead * cacheReadTokens
        + pWrite * cacheWriteTokens
      ) / denom;
      return Number.isFinite(cost) ? cost : undefined;
    } catch {
      return undefined;
    }
  }
}
