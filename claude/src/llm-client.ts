import type { BaseLLMProvider } from './llm-providers/base.js';
import type { TurnRequest, TurnResult, ProviderConfig, LogEntry } from './types.js';

import { AnthropicProvider } from './llm-providers/anthropic.js';
import { GoogleProvider } from './llm-providers/google.js';
import { OllamaProvider } from './llm-providers/ollama.js';
import { OpenAIProvider } from './llm-providers/openai.js';
import { OpenRouterProvider } from './llm-providers/openrouter.js';

export class LLMClient {
  private providers = new Map<string, BaseLLMProvider>();
  private onLog?: (entry: LogEntry) => void;
  private traceLLM: boolean;
  private currentTurn = 0;
  private currentSubturn = 0;

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
    const tracedFetch = this.traceLLM ? this.createTracedFetch() : undefined;

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
        if (url.includes('openrouter.ai')) {
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
            headerObj[key.toLowerCase()] = key.toLowerCase() === 'authorization' ? 'REDACTED' : value;
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

        // Log response details
        if (this.traceLLM) {
          const respHeaders: Record<string, string> = {};
          response.headers.forEach((value, key) => {
            respHeaders[key.toLowerCase()] = key.toLowerCase() === 'authorization' ? 'REDACTED' : value;
          });

          const contentType = response.headers.get('content-type') ?? '';
          let traceMessage = `LLM response: ${String(response.status)} ${response.statusText}\nheaders: ${JSON.stringify(respHeaders, null, 2)}`;

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
    
    this.log('VRB', 'request', 'llm', `${request.provider}:${request.model}`, message);
  }

  private logResponse(request: TurnRequest, result: TurnResult, latencyMs: number): void {
    const remoteId = `${request.provider}:${request.model}`;
    
    if (result.status.type === 'success') {
      const tokens = result.tokens;
      const inputTokens = tokens?.inputTokens ?? 0;
      const outputTokens = tokens?.outputTokens ?? 0;
      const cachedTokens = tokens?.cachedTokens ?? 0;
      
      const responseBytes = result.response !== undefined ? new TextEncoder().encode(result.response).length : 0;
      const message = `input ${String(inputTokens)}, output ${String(outputTokens)}, cached ${String(cachedTokens)} tokens, ${String(latencyMs)}ms, ${String(responseBytes)} bytes`;
      
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