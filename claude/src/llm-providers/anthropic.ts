import { createAnthropic } from '@ai-sdk/anthropic';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage } from '../types.js';
import type { LanguageModel } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class AnthropicProvider extends BaseLLMProvider {
  name = 'anthropic';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super();
    this.config = config;
    const prov = createAnthropic({
      apiKey: config.apiKey,
      baseURL: config.baseUrl,
      fetch: tracedFetch
    });
    
    this.provider = (model: string) => prov(model);
  }

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const startTime = Date.now();
    
    try {
      const model = this.provider(request.model);
      const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn);
      const tools = this.convertTools(filteredTools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      // Inject cache control on system prompt to encourage caching of system content
      // AI SDK maps message.providerOptions.anthropic.cacheControl to cache_control on Anthropic messages
      // eslint-disable-next-line functional/no-loop-statements
      for (const m of messages as unknown as ({ role: string; providerOptions?: Record<string, unknown> }[])) {
        if (m.role === 'system') {
          m.providerOptions = { ...(m.providerOptions ?? {}), anthropic: { cacheControl: { type: 'ephemeral' } } };
          break;
        }
      }
      
      // Add final turn message if needed
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);

      const providerOptions = (() => {
        const base: Record<string, unknown> = { anthropic: {} };
        const a = (base.anthropic as Record<string, unknown>);
        if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) a.maxTokens = Math.trunc(request.maxOutputTokens);
        // Anthropic has no generic repeat penalty; ignore if provided
        return base;
      })();

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
    }
  }


  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }
}
