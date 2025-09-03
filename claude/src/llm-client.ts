import type { BaseLLMProvider } from './llm-providers/base.js';
import type { TurnRequest, TurnResult, ProviderConfig, LogEntry } from './types.js';

import { AnthropicProvider } from './llm-providers/anthropic.js';
import { GoogleProvider } from './llm-providers/google.js';
import { OllamaProvider } from './llm-providers/ollama.js';
import { OpenAIProvider } from './llm-providers/openai.js';
import { OpenRouterProvider } from './llm-providers/openrouter.js';

export class LLMClient {
  private static readonly OPENROUTER_HOST = 'openrouter.ai';
  private providers = new Map<string, BaseLLMProvider>();
  private onLog?: (entry: LogEntry) => void;
  private traceLLM: boolean;
  private currentTurn = 0;
  private currentSubturn = 0;
  // Tracks actual routed provider/model when using routers like OpenRouter
  private lastActualProvider?: string;
  private lastActualModel?: string;
  // Tracks last reported costs when available (e.g., OpenRouter)
  private lastCostUsd?: number;
  private lastUpstreamCostUsd?: number;

  constructor(
    providerConfigs: Record<string, ProviderConfig>,
    options?: {
      traceLLM?: boolean;
      onLog?: (entry: LogEntry) => void;
    }
  ) {
    this.traceLLM = options?.traceLLM ?? false;
    this.onLog = options?.onLog;

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
          const defaultTitle = 'ai-agent-claude';
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
              } catch { /* ignore */ }
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
            } catch { /* ignore SSE parse errors */ }
          } else {
            // Clear for non-openrouter URLs
            if (!url.includes(LLMClient.OPENROUTER_HOST)) {
              reqActualProvider = undefined;
              reqActualModel = undefined;
            }
          }
        } catch { /* ignore */ }

        // Commit per-request routing/cost to instance state for retrieval after this call
        this.lastActualProvider = reqActualProvider;
        this.lastActualModel = reqActualModel;
        this.lastCostUsd = reqCostUsd;
        this.lastUpstreamCostUsd = reqUpstreamCostUsd;

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
      
      const responseBytes = result.response !== undefined ? new TextEncoder().encode(result.response).length : 0;
      // Do not include routed provider/model details here; keep log concise up to bytes
      let message = `input ${String(inputTokens)}, output ${String(outputTokens)}, cached ${String(cachedTokens)} tokens, ${String(latencyMs)}ms, ${String(responseBytes)} bytes`;
      // Append cost info (5 decimals) only when non-zero and available (OpenRouter)
      if (request.provider === 'openrouter') {
        const costInfo = this.getLastCostInfo();
        const costParts: string[] = [];
        if (typeof costInfo.costUsd === 'number' && costInfo.costUsd > 0) {
          costParts.push(`cost $${costInfo.costUsd.toFixed(5)}`);
        }
        if (typeof costInfo.upstreamInferenceCostUsd === 'number' && costInfo.upstreamInferenceCostUsd > 0) {
          costParts.push(`upstream $${costInfo.upstreamInferenceCostUsd.toFixed(5)}`);
        }
        if (costParts.length > 0) {
          message += `, ${costParts.join(' ')}`;
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
