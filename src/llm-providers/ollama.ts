import { createOllama } from 'ollama-ai-provider-v2';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage } from '../types.js';
import type { LanguageModel } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OllamaProvider extends BaseLLMProvider {
  name = 'ollama';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super();
    this.config = config;
    const normalizedBaseUrl = this.normalizeBaseUrl(config.baseUrl);
    const prov = createOllama({ 
      baseURL: normalizedBaseUrl, 
      fetch: tracedFetch 
    });
    
    this.provider = (model: string) => prov(model);
  }

  private normalizeBaseUrl(url?: string): string {
    const def = 'http://localhost:11434/api';
    if (url === undefined || url.length === 0) return def;
    try {
      let v = url.replace(/\/$/, '');
      // Replace trailing /v1 with /api
      if (/\/v1\/?$/.test(v)) return v.replace(/\/v1\/?$/, '/api');
      // If already ends with /api, keep as-is
      if (/\/api\/?$/.test(v)) return v;
      // Otherwise, append /api
      return v + '/api';
    } catch {
      return def;
    }
  }

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const startTime = Date.now();
    
    try {
      const model = this.provider(request.model);
      const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn);
      const tools = this.convertTools(filteredTools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);

      // Get provider options from config and overlay dynamic knobs
      let providerOptions = this.getProviderOptions() as Record<string, unknown> | undefined;
      try {
        const dyn: Record<string, unknown> = {};
        if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) {
          const existing = (dyn.ollama as { options?: Record<string, unknown> } | undefined)?.options ?? {};
          dyn.ollama = { options: { ...existing, num_predict: Math.trunc(request.maxOutputTokens) } } as Record<string, unknown>;
        }
        if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) {
          const existing = (dyn.ollama as { options?: Record<string, unknown> } | undefined)?.options ?? {};
          dyn.ollama = { options: { ...existing, repeat_penalty: request.repeatPenalty } } as Record<string, unknown>;
        }
        if (Object.keys(dyn).length > 0) {
          providerOptions = { ...(providerOptions ?? {}), ...dyn };
        }
      } catch (e) { try { console.error(`[warn] ollama provider cleanup failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
    }
  }

  private getProviderOptions(): unknown {
    try {
      const custom = this.config.custom ?? {};
      return custom.providerOptions ?? undefined;
    } catch {
      return undefined;
    }
  }


  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }
}
