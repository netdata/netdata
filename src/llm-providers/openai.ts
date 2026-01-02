import { createOpenAI } from '@ai-sdk/openai';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage } from '../types.js';
import type { LanguageModel } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenAIProvider extends BaseLLMProvider {
  name = 'openai';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super({
      formatPolicy: { allowed: config.stringSchemaFormatsAllowed, denied: config.stringSchemaFormatsDenied },
      reasoningDefaults: {
        minimal: 'minimal',
        low: 'low',
        medium: 'medium',
        high: 'high',
      },
    });
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
      const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn);
      const tools = this.convertTools(filteredTools, request.toolExecutor);
      const messages = super.convertMessages(request.messages, { interleaved: request.interleaved });
      
      // Add final turn message if needed
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);

      const providerOptions = (() => {
        const resolvedToolChoice = this.resolveToolChoice(request);
        const base: Record<string, unknown> = { openai: {} };
        const o = (base.openai as Record<string, unknown>);
        if (resolvedToolChoice !== undefined) o.toolChoice = resolvedToolChoice;
        if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) o.maxTokens = Math.trunc(request.maxOutputTokens);
        if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) o.frequencyPenalty = request.repeatPenalty;
        if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
          o.reasoningEffort = typeof request.reasoningValue === 'string' ? request.reasoningValue : String(request.reasoningValue);
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
