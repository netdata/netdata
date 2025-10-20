import { createGoogleGenerativeAI } from '@ai-sdk/google';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage } from '../types.js';
import type { LanguageModel } from 'ai';

import { warn } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class GoogleProvider extends BaseLLMProvider {
  name = 'google';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super({
      formatPolicy: { allowed: config.stringSchemaFormatsAllowed, denied: config.stringSchemaFormatsDenied },
      reasoningLimits: { min: 1024, max: 32_768 },
    });
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
      const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn);
      const tools = this.convertTools(filteredTools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);

      const providerOptions = (() => {
        const base: Record<string, unknown> = { google: {} };
        const g = (base.google as Record<string, unknown>);
        if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) g.maxOutputTokens = Math.trunc(request.maxOutputTokens);
        if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) g.frequencyPenalty = request.repeatPenalty;
        if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
          const budget = typeof request.reasoningValue === 'number'
            ? request.reasoningValue
            : Number(request.reasoningValue);
          if (Number.isFinite(budget)) {
            g.thinkingConfig = { thinkingBudget: Math.trunc(budget), includeThoughts: true };
          } else {
            warn(`google reasoning value '${String(request.reasoningValue)}' is not numeric; ignoring`);
          }
        }
        return base;
      })();

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(request, this.mapError(error), Date.now() - startTime);
    }
  }


  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }
}
