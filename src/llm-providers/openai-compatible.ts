import { createOpenAICompatible } from '@ai-sdk/openai-compatible';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage } from '../types.js';
import type { LanguageModel } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenAICompatibleProvider extends BaseLLMProvider {
  name = 'openai-compatible';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;
  private providerId: string;

  constructor(providerId: string, config: ProviderConfig, tracedFetch?: typeof fetch) {
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
    this.providerId = providerId;
    const baseUrl = config.baseUrl;
    if (baseUrl === undefined || baseUrl.length === 0) {
      throw new Error(`openai-compatible provider '${providerId}' missing baseUrl`);
    }
    const includeUsage = this.resolveIncludeUsage(config.custom);
    const prov = createOpenAICompatible({
      apiKey: config.apiKey,
      baseURL: baseUrl,
      headers: config.headers,
      name: providerId,
      fetch: tracedFetch,
      includeUsage,
    });

    this.provider = (model: string) => prov.chatModel(model);
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
      const providerOptions = this.buildProviderOptions(request);
      const patchedRequest = request.topK !== null && request.topK !== undefined
        ? { ...request, topK: null }
        : request;

      if (patchedRequest.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, patchedRequest, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, patchedRequest, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(request, this.mapError(error), Date.now() - startTime);
    }
  }

  private buildProviderOptions(request: TurnRequest): Record<string, unknown> | undefined {
    const base = this.getProviderOptions();
    const extra: Record<string, unknown> = {};
    const compat: Record<string, unknown> = {};

    if (typeof request.topK === 'number' && Number.isFinite(request.topK)) {
      compat.top_k = Math.trunc(request.topK);
    }
    if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) {
      compat.repeat_penalty = request.repeatPenalty;
    }
    if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
      compat.reasoningEffort = typeof request.reasoningValue === 'string'
        ? request.reasoningValue
        : String(request.reasoningValue);
    }
    if (Object.keys(compat).length > 0) {
      extra[this.providerId] = compat;
    }

    if (Object.keys(extra).length === 0) return base ?? undefined;
    if (base === undefined) return extra;
    return this.deepMerge(base, extra);
  }

  private getProviderOptions(): Record<string, unknown> | undefined {
    try {
      const custom = this.config.custom ?? {};
      const providerOptions = (custom as { providerOptions?: unknown }).providerOptions;
      if (this.isPlainObject(providerOptions)) return providerOptions;
      if (providerOptions !== undefined) return providerOptions as Record<string, unknown>;
      return undefined;
    } catch {
      return undefined;
    }
  }

  private resolveIncludeUsage(custom: ProviderConfig['custom']): boolean {
    const candidate = this.isPlainObject(custom) ? custom.includeUsage : undefined;
    if (typeof candidate === 'boolean') return candidate;
    return true;
  }

  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }
}
