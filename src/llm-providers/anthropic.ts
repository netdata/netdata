import { createAnthropic } from '@ai-sdk/anthropic';

import type { ReasoningLevel, TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage, TurnStatus, TurnRetryDirective } from '../types.js';
import type { LanguageModel } from 'ai';

import { warn } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

interface OptionsWithAnthropic extends Record<string, unknown> {
  anthropic?: Record<string, unknown>;
}

export class AnthropicProvider extends BaseLLMProvider {
  name = 'anthropic';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super({
      formatPolicy: { allowed: config.stringSchemaFormatsAllowed, denied: config.stringSchemaFormatsDenied },
      reasoningLimits: { min: 1024, max: 128_000 },
    });
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

      // Apply cache control to the last message only and remove any prior cache controls to stay within Anthropic limits
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);
      interface ProviderMessage { providerOptions?: Record<string, unknown>; }
      const extendedMessages = finalMessages as unknown as ProviderMessage[];
      const isRecord = (val: unknown): val is Record<string, unknown> => val !== null && typeof val === 'object' && !Array.isArray(val);
      const cloneOptions = (opts: Record<string, unknown>): OptionsWithAnthropic => ({ ...opts });
      const cachingMode = request.caching ?? 'full';

      // eslint-disable-next-line functional/no-loop-statements
      for (const msg of extendedMessages) {
        if (!isRecord(msg.providerOptions)) {
          msg.providerOptions = undefined;
          continue;
        }

        const nextOptions = cloneOptions(msg.providerOptions);
        const anthropicOptions = nextOptions.anthropic;
        if (isRecord(anthropicOptions)) {
          const rest: Record<string, unknown> = { ...anthropicOptions };
          delete rest.cacheControl;
          if (Object.keys(rest).length > 0) {
            nextOptions.anthropic = rest;
          } else {
            delete nextOptions.anthropic;
          }
        }

        msg.providerOptions = Object.keys(nextOptions).length > 0 ? nextOptions : undefined;
      }

      if (extendedMessages.length > 0 && cachingMode !== 'none') {
        const lastMessage = extendedMessages[extendedMessages.length - 1];
        const baseOptions: OptionsWithAnthropic = isRecord(lastMessage.providerOptions) ? cloneOptions(lastMessage.providerOptions) : {};
        const existingAnthropic = isRecord(baseOptions.anthropic) ? baseOptions.anthropic : {};
        baseOptions.anthropic = {
          ...existingAnthropic,
          cacheControl: { type: 'ephemeral' }
        };
        lastMessage.providerOptions = baseOptions;
      }

      const providerOptions = (() => {
        const base: Record<string, unknown> = { anthropic: {} };
        const a = (base.anthropic as Record<string, unknown>);
        const sendReasoning = request.sendReasoning ?? true;
        a.sendReasoning = sendReasoning;
        if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) a.maxTokens = Math.trunc(request.maxOutputTokens);
        // Anthropic has no generic repeat penalty; ignore if provided
        if (request.reasoningValue !== undefined) {
          if (request.reasoningValue === null) {
            // Explicitly disabled – do not set thinking options
          } else if (typeof request.reasoningValue === 'number' && Number.isFinite(request.reasoningValue)) {
            a.thinking = { type: 'enabled', budgetTokens: Math.trunc(request.reasoningValue) };
          } else if (typeof request.reasoningValue === 'string' && request.reasoningValue.trim().length > 0) {
            const parsed = Number(request.reasoningValue);
            if (Number.isFinite(parsed)) {
              a.thinking = { type: 'enabled', budgetTokens: Math.trunc(parsed) };
            } else {
              warn(`anthropic reasoning value '${request.reasoningValue}' is not a number; ignoring`);
            }
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

  public override shouldAutoEnableReasoningStream(level: ReasoningLevel | undefined): boolean {
    if (level === undefined) return false;
    const threshold = this.config.reasoningAutoStreamLevel;
    if (threshold === undefined) return false;
    const levelIdx = this.getReasoningLevelIndex(level);
    const thresholdIdx = this.getReasoningLevelIndex(threshold);
    return levelIdx >= thresholdIdx;
  }

  protected override shouldForceToolChoice(request: TurnRequest): boolean {
    if (request.reasoningLevel !== undefined) {
      return false;
    }
    return super.shouldForceToolChoice(request);
  }

  protected override buildRetryDirective(request: TurnRequest, status: TurnStatus): TurnRetryDirective | undefined {
    if (status.type === 'rate_limit') {
      const wait = typeof status.retryAfterMs === 'number' && Number.isFinite(status.retryAfterMs) ? status.retryAfterMs : undefined;
      const remoteId = `${request.provider}:${request.model}`;
      const message = `Anthropic rate limit; waiting ${wait !== undefined ? `${String(wait)}ms` : 'briefly'} before retry.`;
      return {
        action: 'retry',
        backoffMs: wait,
        logMessage: message,
        systemMessage: `System notice: Anthropic (${remoteId}) rate-limited the prior request. Retrying shortly; no changes required unless limits persist.`,
        sources: status.sources,
      };
    }
    return super.buildRetryDirective(request, status);
  }

  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }
}
