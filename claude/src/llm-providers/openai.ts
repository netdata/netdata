import { createOpenAI } from '@ai-sdk/openai';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage } from '../types.js';
import type { LanguageModel, ModelMessage } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenAIProvider extends BaseLLMProvider {
  name = 'openai';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super();
    this.config = config;
    const prov = createOpenAI({ 
      apiKey: config.apiKey, 
      baseURL: config.baseUrl, 
      fetch: tracedFetch 
    });
    
    const mode = config.openaiMode ?? 'responses';
    if (mode === 'responses') {
      this.provider = (model: string) => (prov as unknown as { responses: (m: string) => LanguageModel }).responses(model);
    } else {
      this.provider = (model: string) => (prov as unknown as { chat: (m: string) => LanguageModel }).chat(model);
    }
  }

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const startTime = Date.now();
    
    try {
      const model = this.provider(request.model);
      const tools = this.convertTools(request.tools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = request.isFinalTurn === true 
        ? messages.concat({ 
            role: 'user', 
            content: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate"
          } as ModelMessage)
        : messages;

      const providerOptions = request.parallelToolCalls !== undefined 
        ? { openai: { parallelToolCalls: request.parallelToolCalls } }
        : undefined;

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
    }
  }


  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: { inputTokens?: number; outputTokens?: number; cachedTokens?: number; totalTokens?: number }): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return messages.map(m => this.parseAISDKMessage(m, provider, model, tokens));
  }
}