import { createOllama } from 'ollama-ai-provider-v2';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage } from '../types.js';
import type { LanguageModel, ModelMessage } from 'ai';

import { BaseLLMProvider } from './base.js';

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
      const tools = this.convertTools(request.tools, request.toolExecutor);
      const messages = this.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = request.isFinalTurn === true 
        ? messages.concat({ 
            role: 'user', 
            content: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate"
          } as ModelMessage)
        : messages;

      // Get provider options from config
      const providerOptions = this.getProviderOptions();

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

  private convertMessages(messages: ConversationMessage[]): ModelMessage[] {
    return messages
      .filter(m => m.role === 'system' || m.role === 'user' || m.role === 'assistant')
      .map(m => ({
        role: m.role as 'system' | 'user' | 'assistant',
        content: m.content,
        toolCalls: m.toolCalls
      })) as ModelMessage[];
  }

  protected convertResponseMessages(messages: {
    role?: string;
    content?: string | { type?: string; text?: string }[];
    toolCalls?: {
      id?: string;
      name?: string;
      arguments?: unknown;
      function?: { name?: string; arguments?: unknown };
      toolCallId?: string;
    }[];
  }[], provider: string, model: string, tokens: { inputTokens?: number; outputTokens?: number; cachedTokens?: number; totalTokens?: number }): ConversationMessage[] {
    return messages.map(m => ({
      role: (m.role ?? 'assistant') as ConversationMessage['role'],
      content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content),
      toolCalls: Array.isArray(m.toolCalls) ? m.toolCalls.map(tc => ({
        id: tc.id ?? tc.toolCallId ?? '',
        name: tc.name ?? tc.function?.name ?? '',
        parameters: (tc.arguments ?? tc.function?.arguments ?? {}) as Record<string, unknown>
      })) : undefined,
      toolCallId: 'toolCallId' in m ? (m as { toolCallId?: string }).toolCallId : undefined,
      metadata: {
        provider,
        model,
        tokens: {
          inputTokens: tokens.inputTokens ?? 0,
          outputTokens: tokens.outputTokens ?? 0,
          cachedTokens: tokens.cachedTokens,
          totalTokens: tokens.totalTokens ?? 0
        },
        timestamp: Date.now()
      }
    }));
  }
}
