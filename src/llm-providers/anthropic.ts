import { createAnthropic } from '@ai-sdk/anthropic';

import type { ReasoningLevel, TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TokenUsage, TurnStatus, TurnRetryDirective } from '../types.js';
import type { LanguageModel } from 'ai';

import { warn } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

const ANTHROPIC_REASONING_STREAM_TOKEN_THRESHOLD = 21_333;

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

      const isSystemNotice = (msg: ResponseMessage): boolean => {
        const content = (msg as { content?: unknown }).content;
        return typeof content === 'string' && content.trim().startsWith('System notice:');
      };

      // Check if message is ephemeral (changes every turn, should not be cached)
      // noticeType is lost during convertMessages, so check content patterns
      const isEphemeralMessage = (msg: ResponseMessage): boolean => {
        const content = (msg as { content?: unknown }).content;
        if (typeof content !== 'string') return false;
        const trimmed = content.trim();
        // XML-NEXT: "# System Notice\n\nThis is turn No X"
        // XML-PAST: "# System Notice\n\n## Previous Turn Tool Responses"
        if (trimmed.startsWith('# System Notice')) return true;
        return false;
      };

      const findCacheTargetIndex = (): number => {
        // eslint-disable-next-line functional/no-loop-statements
        for (let i = extendedMessages.length - 1; i >= 0; i -= 1) {
          const candidate = extendedMessages[i] as ResponseMessage;
          // Skip ephemeral messages that change every turn (breaks cache reads)
          if (!isSystemNotice(candidate) && !isEphemeralMessage(candidate)) {
            return i;
          }
        }
        return -1;
      };

      const cacheTargetIndex = cachingMode === 'none' ? -1 : findCacheTargetIndex();

      if (cacheTargetIndex !== -1) {
        const lastMessage = extendedMessages[cacheTargetIndex];
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

  public override shouldAutoEnableReasoningStream(
    level: ReasoningLevel | undefined,
    options?: { maxOutputTokens?: number; reasoningActive?: boolean; streamRequested?: boolean }
  ): boolean {
    const reasoningActive = options?.reasoningActive === true || level !== undefined;
    if (!reasoningActive) return false;
    if (options?.streamRequested === true) return false;
    const maxOutputTokens = options?.maxOutputTokens;
    if (typeof maxOutputTokens !== 'number' || !Number.isFinite(maxOutputTokens)) return false;
    if (maxOutputTokens < ANTHROPIC_REASONING_STREAM_TOKEN_THRESHOLD) return false;
    return true;
  }

  protected override shouldForceToolChoice(request: TurnRequest): boolean {
    if (request.reasoningLevel !== undefined) {
      return false;
    }
    return super.shouldForceToolChoice(request);
  }

  public override shouldDisableReasoning(context: { conversation: ConversationMessage[]; currentTurn: number; attempt: number; expectSignature: boolean }): { disable: boolean; normalized: ConversationMessage[] } {
    if (context.currentTurn <= 1 || !context.expectSignature) {
      return { disable: false, normalized: context.conversation };
    }
    const { normalized, missing } = this.stripReasoningWithoutSignature(context.conversation);
    return { disable: missing, normalized };
  }

  private stripReasoningWithoutSignature(messages: ConversationMessage[]): { normalized: ConversationMessage[]; missing: boolean } {
    let missing = false;
    const normalized = messages.map((message) => {
      if (message.role !== 'assistant' || !Array.isArray(message.toolCalls) || message.toolCalls.length === 0) {
        return message;
      }
      const segments = Array.isArray(message.reasoning) ? message.reasoning : [];
      if (segments.length === 0) {
        return message;
      }
      const hasSignedSegment = segments.some((segment) => this.segmentHasSignature(segment));
      if (hasSignedSegment) {
        return message;
      }
      missing = true;
      const cloned: ConversationMessage = { ...message };
      delete (cloned as { reasoning?: ConversationMessage['reasoning'] }).reasoning;
      return cloned;
    });
    return { normalized, missing };
  }

  private segmentHasSignature(segment: { providerMetadata?: unknown }): boolean {
    const metadata = segment.providerMetadata;
    if (metadata === null || typeof metadata !== 'object' || Array.isArray(metadata)) {
      return false;
    }
    const anthropic = (metadata as { anthropic?: unknown }).anthropic;
    if (anthropic === null || typeof anthropic !== 'object' || Array.isArray(anthropic)) {
      return false;
    }
    const record = anthropic as { signature?: unknown; redactedData?: unknown };
    const signature = typeof record.signature === 'string' ? record.signature.trim() : '';
    const redacted = typeof record.redactedData === 'string' ? record.redactedData.trim() : '';
    return signature.length > 0 || redacted.length > 0;
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
        systemMessage: `Anthropic rate limit for ${remoteId}; retrying.`,
        sources: status.sources,
      };
    }
    return super.buildRetryDirective(request, status);
  }

  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    // Use base class helper that handles AI SDK's content array format
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }

  protected override supportsReasoningReplay(): boolean {
    return true;
  }
}
