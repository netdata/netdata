import { createGoogleGenerativeAI } from '@ai-sdk/google';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage } from '../types.js';
import type { LanguageModel, ModelMessage } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class GoogleProvider extends BaseLLMProvider {
  name = 'google';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super();
    this.config = config;
    const prov = createGoogleGenerativeAI({ 
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
      const tools = this.convertTools(request.tools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = request.isFinalTurn === true 
        ? messages.concat({ 
            role: 'user', 
            content: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate"
          } as ModelMessage)
        : messages;

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime);
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
