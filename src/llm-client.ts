import type { BaseLLMProvider } from './llm-providers/base.js';
import type { TurnRequest, TurnResult, ProviderConfig, LogEntry } from './types.js';

import { AnthropicProvider } from './llm-providers/anthropic.js';
import { GoogleProvider } from './llm-providers/google.js';
import { OllamaProvider } from './llm-providers/ollama.js';
import { OpenAIProvider } from './llm-providers/openai.js';
import { OpenRouterProvider } from './llm-providers/openrouter.js';
import { TestLLMProvider } from './llm-providers/test-llm.js';

export class LLMClient {
  private static readonly OPENROUTER_HOST = 'openrouter.ai';
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
      // Per-request routing/cost locals to avoid stale shared state
      let reqActualProvider: string | undefined;
      let reqActualModel: string | undefined;
      let reqCostUsd: number | undefined;
      let reqUpstreamCostUsd: number | undefined;
      let reqCacheWriteInputTokens: number | undefined;
      
      try {
        if (typeof input === 'string') url = input;
        else if (input instanceof URL) url = input.toString();
        else if (input instanceof Request) url = input.url;
        method = init?.method ?? 'GET';

        // Add default headers
        const headers = new Headers(init?.headers);
        if (!headers.has('Accept')) {
          headers.set('Accept', 'application/json');
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

        // Parse routing metadata (OpenRouter actual provider/model) and cost regardless of trace flag
        try {
          const contentType = response.headers.get('content-type') ?? '';
          if (url.includes(LLMClient.OPENROUTER_HOST) && contentType.includes('application/json')) {
            const clone = response.clone();
            const raw = await clone.text();
            try {
              const parsed = JSON.parse(raw) as { provider?: string; model?: string; choices?: { provider?: string; model?: string }[]; error?: { metadata?: { provider_name?: string } } };
              const actualProv = parsed.provider ?? parsed.choices?.[0]?.provider ?? parsed.error?.metadata?.provider_name;
              const actualModel = parsed.model ?? parsed.choices?.[0]?.model;
              reqActualProvider = typeof actualProv === 'string' && actualProv.length > 0 ? actualProv : undefined;
              reqActualModel = typeof actualModel === 'string' && actualModel.length > 0 ? actualModel : undefined;
              // Try to extract usage cost if present
              try {
                const pr2 = JSON.parse(raw) as { usage?: { cost?: number; cost_details?: { upstream_inference_cost?: number } } };
                const c = pr2.usage?.cost;
                const u = pr2.usage?.cost_details?.upstream_inference_cost;
                reqCostUsd = typeof c === 'number' ? c : undefined;
                reqUpstreamCostUsd = typeof u === 'number' ? u : undefined;
              } catch (e) { try { console.error(`[warn] openrouter extract usage cost failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
            } catch {
              // ignore parse errors
            }
          } else if (url.includes(LLMClient.OPENROUTER_HOST) && contentType.includes('text/event-stream')) {
            // Streaming: parse SSE for final usage/cost and provider info
            try {
              const clone = response.clone();
              const raw = await clone.text();
              // Extract last JSON payload from lines beginning with 'data: '
              const lines = raw.split(/\r?\n/).filter((l) => l.startsWith('data:'));
              // Iterate from end to find a JSON with usage or provider
              [...lines].reverse().some((line) => {
                const payload = line.slice(5).trim();
                if (!payload || payload === '[DONE]') return false;
                try {
                  const obj = JSON.parse(payload) as {
                    provider?: string;
                    model?: string;
                    choices?: { provider?: string; model?: string }[];
                    usage?: { cost?: number; cost_details?: { upstream_inference_cost?: number } };
                  };
                  const aProv = obj.provider ?? obj.choices?.[0]?.provider;
                  const aModel = obj.model ?? obj.choices?.[0]?.model;
                  if (typeof aProv === 'string' && aProv.length > 0 && reqActualProvider === undefined) reqActualProvider = aProv;
                  if (typeof aModel === 'string' && aModel.length > 0 && reqActualModel === undefined) reqActualModel = aModel;
                  const c = obj.usage?.cost;
                  const u = obj.usage?.cost_details?.upstream_inference_cost;
                  if (typeof c === 'number' && reqCostUsd === undefined) reqCostUsd = c;
                  if (typeof u === 'number' && reqUpstreamCostUsd === undefined) reqUpstreamCostUsd = u;
                } catch { /* skip non-JSON lines */ }
                return (reqActualProvider !== undefined && reqActualModel !== undefined && reqCostUsd !== undefined);
              });
            } catch (e) { try { console.error(`[warn] openrouter SSE parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
          } else {
            // Clear for non-openrouter URLs
            if (!url.includes(LLMClient.OPENROUTER_HOST)) {
              reqActualProvider = undefined;
              reqActualModel = undefined;
            }
            // For generic JSON responses, try to extract provider-agnostic usage signals we care about (e.g., Anthropic cache creation tokens)
            if (contentType.includes('application/json')) {
              try {
                const clone = response.clone();
                const raw = await clone.text();
                const isRecord = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object';
                const parsedUnknown: unknown = JSON.parse(raw);
                const parsedObj = isRecord(parsedUnknown) ? parsedUnknown : undefined;
                // eslint-disable-next-line @typescript-eslint/dot-notation
                const usageVal = parsedObj !== undefined ? parsedObj['usage'] : undefined;
                const u: Record<string, unknown> = isRecord(usageVal) ? usageVal : {};
                const asNum = (v: unknown): number | undefined => (typeof v === 'number' && Number.isFinite(v) && v > 0) ? v : undefined;
                const getNested = (obj: Record<string, unknown>, k: string): unknown => (Object.prototype.hasOwnProperty.call(obj, k) ? obj[k] : undefined);
                // Try common variants
                let w: unknown = getNested(u, 'cacheWriteInputTokens')
                  ?? getNested(u, 'cacheCreationInputTokens')
                  ?? getNested(u, 'cache_creation_input_tokens')
                  ?? getNested(u, 'cache_write_input_tokens');
                if (w === undefined) {
                  const cc = getNested(u, 'cacheCreation');
                  if (isRecord(cc)) { w = cc.ephemeral_5m_input_tokens; }
                }
                if (w === undefined) {
                  const cc2 = getNested(u, 'cache_creation');
                  if (isRecord(cc2)) { w = cc2.ephemeral_5m_input_tokens; }
                }
                const n = asNum(w);
                if (typeof n === 'number') reqCacheWriteInputTokens = n;
              } catch (e) { try { console.error(`[warn] generic JSON usage parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
            }
          }
        } catch (e) { try { console.error(`[warn] traced fetch post-response processing failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }

        // Commit per-request routing/cost to instance state for retrieval after this call
        this.lastActualProvider = reqActualProvider;
        this.lastActualModel = reqActualModel;
        this.lastCostUsd = reqCostUsd;
        this.lastUpstreamCostUsd = reqUpstreamCostUsd;
        this.lastCacheWriteInputTokens = reqCacheWriteInputTokens;

        // Log response details when trace is enabled
        if (this.traceLLM) {
          const respHeaders: Record<string, string> = {};
          response.headers.forEach((value, key) => {
            const k = key.toLowerCase();
            if (k === 'authorization' || k === 'x-api-key' || k === 'api-key' || k === 'x-goog-api-key') {
              if (value.startsWith('Bearer ')) {
                const token = value.substring(7);
                if (token.length > 8) {
                  respHeaders[k] = `Bearer ${token.substring(0, 4)}...REDACTED...${token.substring(token.length - 4)}`;
                } else {
                  respHeaders[k] = `Bearer [SHORT_TOKEN]`;
                }
              } else {
                respHeaders[k] = '[REDACTED]';
              }
            } else {
              respHeaders[k] = value;
            }
          });

          let traceMessage = `LLM response: ${String(response.status)} ${response.statusText}\nheaders: ${JSON.stringify(respHeaders, null, 2)}`;

          const contentType = response.headers.get('content-type') ?? '';
          if (contentType.includes('application/json')) {
            try {
              const clone = response.clone();
              let text = await clone.text();
              try {
                text = JSON.stringify(JSON.parse(text), null, 2);
              } catch { /* keep original */ }
              traceMessage += `\nbody: ${text}`;
            } catch {
              traceMessage += `\ncontent-type: ${contentType}`;
            }
          } else if (contentType.includes('text/event-stream')) {
            try {
              const clone = response.clone();
              const raw = await clone.text();
              traceMessage += `\nraw-sse: ${raw}`;
            } catch {
              traceMessage += `\ncontent-type: ${contentType}`;
            }
          } else {
            traceMessage += `\ncontent-type: ${contentType}`;
          }

          this.log('TRC', 'response', 'llm', `trace:${method}`, traceMessage);
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
      const message = `error [${result.status.type.toUpperCase()}] ${statusMessage} (waited ${String(latencyMs)} ms)`;
      
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
