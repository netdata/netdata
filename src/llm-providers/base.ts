import { jsonSchema } from '@ai-sdk/provider-utils';
import { generateText, streamText } from 'ai';

import type {
  ConversationMessage,
  MCPTool,
  ProviderReasoningMapping,
  ProviderReasoningValue,
  ProviderTurnMetadata,
  ProviderParameterWarning,
  ReasoningLevel,
  TokenUsage,
  TurnRequest,
  TurnResult,
  TurnStatus,
  TurnRetryDirective,
} from '../types.js';
import type { ProviderOptions } from '@ai-sdk/provider-utils';
import type { LanguageModel, ModelMessage, ProviderMetadata, ReasoningOutput, StreamTextResult, ToolSet } from 'ai';

import { XML_WRAPPER_CALLED_AS_TOOL_RESULT, isXmlFinalReportTagName } from '../llm-messages.js';
import { ThinkTagStreamFilter } from '../think-tag-filter.js';
import { truncateToBytes } from '../truncation.js';
import { clampToolName, parseJsonRecord, sanitizeToolName, TOOL_NAME_MAX_LENGTH, warn } from '../utils.js';

const GUIDANCE_STRING_FORMATS = ['date-time', 'time', 'date', 'duration', 'email', 'hostname', 'ipv4', 'ipv6', 'uuid'] as const;
const TOOL_FAILED_PREFIX = '(tool failed: ' as const;
const TOOL_FAILED_SUFFIX = ')' as const;
const NO_TOOL_OUTPUT = '(no output)' as const;

interface LLMProviderInterface {
  name: string;
  executeTurn: (request: TurnRequest) => Promise<TurnResult>;
}

interface FormatList {
  tokens: Set<string>;
  wildcard: boolean;
  empty: boolean;
}

interface FormatPolicyNormalized {
  allowed: FormatList;
  denied: FormatList;
}

interface FormatPolicyInput {
  allowed?: string[];
  denied?: string[];
}

export abstract class BaseLLMProvider implements LLMProviderInterface {
  private readonly stringFormatPolicy: FormatPolicyNormalized;
  private pendingMetadata?: ProviderTurnMetadata;
  private static readonly STOP_REASON_KEYS = ['stopReason', 'stop_reason', 'finishReason', 'finish_reason'] as const;
  private static readonly REFUSAL_STOP_REASONS = new Set(['refusal', 'content-filter']);
  private static readonly REASONING_LEVELS: ReasoningLevel[] = ['minimal', 'low', 'medium', 'high'];
  private readonly reasoningDefaults?: Partial<Record<ReasoningLevel, ProviderReasoningValue>>;
  private readonly reasoningLimits?: { min: number; max: number };
  private static readonly RAW_PREVIEW_LIMIT_BYTES = 512;

  protected constructor(options?: {
    formatPolicy?: FormatPolicyInput;
    reasoningDefaults?: Partial<Record<ReasoningLevel, ProviderReasoningValue>>;
    reasoningLimits?: { min?: number; max?: number };
  }) {
    this.stringFormatPolicy = this.normalizeFormatPolicy(options?.formatPolicy);
    this.reasoningDefaults = options?.reasoningDefaults;
    const min = options?.reasoningLimits?.min;
    const max = options?.reasoningLimits?.max;
    if (min !== undefined || max !== undefined) {
      const normalizedMin = min !== undefined && Number.isFinite(min) ? Math.max(1, Math.trunc(min)) : 1;
      const normalizedMax = max !== undefined && Number.isFinite(max) ? Math.max(normalizedMin, Math.trunc(max)) : Number.MAX_SAFE_INTEGER;
      this.reasoningLimits = { min: normalizedMin, max: normalizedMax };
    }
  }

  abstract name: string;
  abstract executeTurn(request: TurnRequest): Promise<TurnResult>;

  public resolveReasoningValue(context: {
    level: ReasoningLevel;
    mapping?: ProviderReasoningMapping | null;
    maxOutputTokens?: number;
  }): ProviderReasoningValue | null | undefined {
    const { mapping } = context;
    if (mapping === null) {
      return null;
    }
    if (Array.isArray(mapping)) {
      const index = this.getReasoningLevelIndex(context.level);
      return mapping[index];
    }
    if (mapping !== undefined) {
      return mapping;
    }
    return this.getDefaultReasoningValue(context.level, context.maxOutputTokens);
  }

  public shouldAutoEnableReasoningStream(
    level: ReasoningLevel | undefined,
    _options?: { maxOutputTokens?: number; reasoningActive?: boolean; streamRequested?: boolean }
  ): boolean {
    void level;
    return false;
  }

  public shouldDisableReasoning(_context: { conversation: ConversationMessage[]; currentTurn: number; attempt: number; expectSignature: boolean }): { disable: boolean; normalized: ConversationMessage[] } {
    return { disable: false, normalized: _context.conversation };
  }

  protected shouldForceToolChoice(_request: TurnRequest): boolean {
    return false;
  }

  protected resolveToolChoice(request: TurnRequest): 'auto' | 'required' | undefined {
    if (request.toolChoice !== undefined) {
      return request.toolChoice;
    }
    const required = request.toolChoiceRequired ?? this.shouldForceToolChoice(request);
    return required ? 'required' : undefined;
  }

  protected traceSdkPayload(request: TurnRequest, stage: 'request' | 'response', payload: unknown): void {
    if (request.sdkTrace !== true) return;
    const logger = request.sdkTraceLogger;
    if (typeof logger !== 'function') return;
    try {
      logger({ stage, provider: request.provider, model: request.model, payload });
    } catch (error) {
      try { warn(`sdkTraceLogger failed: ${error instanceof Error ? error.message : String(error)}`); } catch { /* ignore */ }
    }
  }

  protected buildWireRequestSnapshot(options: {
    request: TurnRequest;
    messages: ModelMessage[];
    providerOptions: unknown;
    toolChoice: string | undefined;
  }): Record<string, unknown> {
    const { request, messages, providerOptions, toolChoice } = options;
    return {
      provider: request.provider,
      model: request.model,
      stream: request.stream === true,
      temperature: request.temperature ?? null,
      topP: request.topP ?? null,
      topK: request.topK ?? null,
      maxOutputTokens: request.maxOutputTokens ?? null,
      reasoningLevel: request.reasoningLevel ?? null,
      reasoningValue: request.reasoningValue ?? null,
      toolChoice: toolChoice ?? null,
      providerOptions,
      messages,
    };
  }

  protected normalizeResponseMessages(payload: unknown): ResponseMessage[] {
    const record = this.isPlainObject(payload) ? payload : undefined;

    const isResponseMessageArray = (value: unknown): value is ResponseMessage[] => Array.isArray(value);

    const extractMessages = (value: unknown): ResponseMessage[] | undefined => {
      if (isResponseMessageArray(value) && value.length > 0) {
        return value;
      }
      return undefined;
    };

    const buildFromChoices = (value: unknown): ResponseMessage[] => {
      const choices = Array.isArray(value) ? value : [];
      const normalized: ResponseMessage[] = [];
      choices.forEach((choice) => {
        const message = (choice as { message?: unknown }).message;
        if (message === undefined || message === null || typeof message !== 'object') return;
        const msgRecord = message as Record<string, unknown>;
        const role = typeof msgRecord.role === 'string' ? msgRecord.role as ResponseMessage['role'] : 'assistant';
        const content = msgRecord.content ?? '';
        const toolCallsRaw = msgRecord.tool_calls;
        const toolCalls = Array.isArray(toolCallsRaw)
          ? toolCallsRaw
              .map((entry, index) => {
                const recordEntry = entry as { id?: unknown; type?: unknown; function?: { name?: unknown; arguments?: unknown }; name?: unknown; arguments?: unknown };
                const rawName = typeof recordEntry.name === 'string'
                  ? recordEntry.name
                  : typeof recordEntry.function?.name === 'string'
                    ? recordEntry.function.name
                    : '';
                const sanitized = sanitizeToolName(rawName);
                const { name } = clampToolName(sanitized);
                const id = typeof recordEntry.id === 'string' ? recordEntry.id : `tool-call-${String(index + 1)}`;
                const rawArgs = recordEntry.arguments ?? recordEntry.function?.arguments;
                const parsedArgs = parseJsonRecord(rawArgs);
                const parameters = parsedArgs ?? ((rawArgs !== undefined ? rawArgs : {}) as Record<string, unknown>);
                return {
                  id,
                  name,
                  parameters,
                };
              })
              .filter((entry): entry is { id: string; name: string; parameters: Record<string, unknown> } => Boolean(entry))
          : undefined;
        const reasoning = this.collectReasoningSegmentsFromMessage(msgRecord);
        const normalizedMessage: ResponseMessage = {
          role,
          content: content as ResponseMessage['content'],
          ...(toolCalls !== undefined && toolCalls.length > 0 ? { toolCalls } : {}),
          ...(reasoning.length > 0 ? { reasoning } : {}),
        };
        normalized.push(normalizedMessage);
      });
      return normalized;
    };

    const directMessages = extractMessages(record?.messages);
    if (directMessages !== undefined) {
      return directMessages;
    }

    const directChoices = buildFromChoices(record?.choices);
    if (directChoices.length > 0) {
      return directChoices;
    }

    const bodyCandidate = record?.body;
    const body = this.isPlainObject(bodyCandidate) ? bodyCandidate : undefined;
    if (body !== undefined) {
      const bodyMessages = extractMessages(body.messages);
      if (bodyMessages !== undefined) {
        return bodyMessages;
      }
      const bodyChoices = buildFromChoices(body.choices);
      if (bodyChoices.length > 0) {
        return bodyChoices;
      }
    }

    return [];
  }

  protected collectReasoningSegmentsFromMessage(message: unknown): ReasoningOutput[] {
    if (message === null || typeof message !== 'object') return [];
    const msgRecord = message as Record<string, unknown>;
    const fromReasoning = this.collectReasoningOutputs(msgRecord.reasoning);
    const fromReasoningContent = this.collectReasoningOutputs(msgRecord.reasoning_content ?? msgRecord.reasoningContent);
    return this.mergeReasoningOutputs(fromReasoning, fromReasoningContent);
  }

  protected collectReasoningOutputs(value: unknown): ReasoningOutput[] {
    if (value === undefined || value === null) return [];
    if (Array.isArray(value)) {
      return this.mergeReasoningOutputs(...value.map((entry) => this.collectReasoningOutputs(entry)));
    }
    if (typeof value === 'string') {
      const text = value.trim();
      if (text.length === 0) return [];
      return [{ type: BaseLLMProvider.PART_REASONING, text }];
    }
    if (typeof value === 'object') {
      const record = value as {
        text?: unknown;
        delta?: unknown;
        providerMetadata?: ProviderMetadata;
        provider_metadata?: ProviderMetadata;
        providerOptions?: ProviderMetadata;
        provider_options?: ProviderMetadata;
        reasoning?: unknown;
        segments?: unknown;
        signature?: unknown;
      };
      if (Array.isArray(record.segments)) {
        return this.collectReasoningOutputs(record.segments);
      }
      const rawThinking = (record as { thinking?: unknown; redacted_thinking?: unknown }).thinking;
      const textCandidate = typeof record.text === 'string'
        ? record.text
        : typeof record.delta === 'string'
          ? record.delta
          : typeof rawThinking === 'string'
            ? rawThinking
            : typeof (record as { redacted_thinking?: unknown }).redacted_thinking === 'string'
              ? (record as { redacted_thinking?: unknown }).redacted_thinking as string
              : undefined;
      if (textCandidate !== undefined) {
        const text = textCandidate.trim();
        if (text.length === 0) return [];
        type AnnotatedReasoningOutput = ReasoningOutput & { signature?: string };
        const output: AnnotatedReasoningOutput = { type: BaseLLMProvider.PART_REASONING, text };
        const metadata = record.providerMetadata
          ?? record.provider_metadata
          ?? record.providerOptions
          ?? record.provider_options;
        if (metadata !== undefined) {
          output.providerMetadata = metadata;
          if (output.signature === undefined) {
            const metadataSignature = this.extractSignatureFromMetadata(metadata);
            if (metadataSignature !== undefined) {
              output.signature = metadataSignature;
            }
          }
        }
        if (typeof record.signature === 'string' && record.signature.trim().length > 0) {
          output.signature = record.signature;
        }
        return [output];
      }
      if (record.reasoning !== undefined) {
        return this.collectReasoningOutputs(record.reasoning);
      }
    }
    return [];
  }

  protected mergeReasoningOutputs(...groups: ReasoningOutput[][]): ReasoningOutput[] {
    const merged: ReasoningOutput[] = [];
    const seen = new Set<string>();
    groups.forEach((group) => {
      group.forEach((segment) => {
        const text = segment.text.trim();
        if (!text) return;
        const metadataKey = segment.providerMetadata !== undefined ? JSON.stringify(segment.providerMetadata) : '';
        const key = `${text}::${metadataKey}`;
        if (seen.has(key)) return;
        seen.add(key);
        merged.push(segment);
      });
    });
    return merged;
  }


  protected extractTurnMetadata(
    request: TurnRequest,
    context: { usage?: unknown; response?: unknown; latencyMs: number }
  ): ProviderTurnMetadata | undefined {
    const derived = this.deriveContextMetadata(request, context);
    const queued = this.consumeQueuedProviderMetadata();
    return this.mergeProviderMetadata(derived, queued);
  }

  protected deriveContextMetadata(
    _request: TurnRequest,
    context: { usage?: unknown; response?: unknown; latencyMs: number }
  ): ProviderTurnMetadata | undefined {
    void _request;
    const usageMetadata = this.extractMetadataFromUsage(context.usage);
    const responseMetadata = this.extractMetadataFromResponse(context.response);
    return this.mergeProviderMetadata(usageMetadata, responseMetadata);
  }

  protected enqueueProviderMetadata(metadata?: ProviderTurnMetadata): void {
    if (metadata === undefined) return;
    this.pendingMetadata = this.mergeProviderMetadata(this.pendingMetadata, metadata);
  }

  protected consumeQueuedProviderMetadata(): ProviderTurnMetadata | undefined {
    if (this.pendingMetadata === undefined) return undefined;
    const metadata = this.pendingMetadata;
    this.pendingMetadata = undefined;
    return { ...metadata };
  }

  protected mergeProviderMetadata(
    target: ProviderTurnMetadata | undefined,
    source?: ProviderTurnMetadata
  ): ProviderTurnMetadata | undefined {
    if (source === undefined) {
      return target !== undefined ? { ...target } : undefined;
    }
    const merged = (target !== undefined ? { ...target } : {}) as ProviderTurnMetadata;
    const recordView = merged as Record<string, unknown>;
    Object.entries(source).forEach(([key, value]) => {
      if (value === undefined) {
        return;
      }
      if (key === 'parameterWarnings') {
        const existing = Array.isArray(merged.parameterWarnings)
          ? merged.parameterWarnings
          : [];
        const incoming = Array.isArray(value) ? value as ProviderParameterWarning[] : [];
        if (incoming.length === 0) {
          return;
        }
        merged.parameterWarnings = [...existing, ...incoming];
        return;
      }
      recordView[key] = value;
    });
    return merged;
  }

  protected recordParameterWarning(warning: Omit<ProviderParameterWarning, 'rawPreview'> & { rawInput?: unknown }): void {
    const entry: ProviderParameterWarning = {
      toolCallId: warning.toolCallId,
      toolName: warning.toolName,
      reason: warning.reason,
      source: warning.source,
      rawPreview: this.previewRawValue(warning.rawInput),
    };
    this.enqueueProviderMetadata({ parameterWarnings: [entry] });
  }

  private previewRawValue(raw: unknown): string {
    if (raw === undefined) {
      return 'undefined';
    }
    if (typeof raw === 'string') {
      return truncateToBytes(raw, BaseLLMProvider.RAW_PREVIEW_LIMIT_BYTES) ?? raw;
    }
    try {
      const serialized = JSON.stringify(raw);
      return truncateToBytes(serialized, BaseLLMProvider.RAW_PREVIEW_LIMIT_BYTES) ?? serialized;
    } catch {
      const fallback = Object.prototype.toString.call(raw);
      return truncateToBytes(fallback, BaseLLMProvider.RAW_PREVIEW_LIMIT_BYTES) ?? fallback;
    }
  }

  private extractSignatureFromMetadata(metadata: ProviderMetadata): string | undefined {
    const record = metadata as Record<string, unknown>;
    const direct = record.signature;
    if (typeof direct === 'string' && direct.trim().length > 0) {
      return direct;
    }
    const anthropicEntry = record.anthropic ?? record.Anthropic;
    if (anthropicEntry !== undefined && typeof anthropicEntry === 'object' && anthropicEntry !== null) {
      const signature = (anthropicEntry as { signature?: unknown }).signature;
      if (typeof signature === 'string' && signature.trim().length > 0) {
        return signature;
      }
    }
    return undefined;
  }

  protected isPlainObject(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
  }

  protected isProviderMetadata(value: unknown): value is ProviderMetadata {
    return this.isPlainObject(value);
  }

  protected toProviderOptions(value: ProviderMetadata): ProviderOptions {
    return value as unknown as ProviderOptions;
  }

  private extractProviderMetadata(part: { providerMetadata?: unknown; providerOptions?: unknown }): ProviderMetadata | undefined {
    const direct = part.providerMetadata;
    if (this.isProviderMetadata(direct)) return direct;
    const opts = part.providerOptions;
    if (this.isPlainObject(opts) && this.isPlainObject(opts.anthropic)) {
      return { anthropic: opts.anthropic } as ProviderMetadata;
    }
    return undefined;
  }

  public prepareFetch(_details: { url: string; init: RequestInit }): { headers?: Record<string, string> } | undefined {
    return undefined;
  }

  public getResponseMetadataCollector(): ((payload: { url: string; response: Response }) => Promise<ProviderTurnMetadata | undefined>) | undefined {
    return undefined;
  }

  protected buildRetryDirective(_request: TurnRequest, _status: TurnStatus): TurnRetryDirective | undefined {
    return undefined;
  }

  private extractMetadataFromUsage(usage: unknown): ProviderTurnMetadata | undefined {
    if (!this.isPlainObject(usage)) return undefined;
    let metadata: ProviderTurnMetadata | undefined;
    const directKeys = [
      'cacheWriteInputTokens',
      'cacheCreationInputTokens',
      'cache_creation_input_tokens',
      'cache_write_input_tokens'
    ];
    // eslint-disable-next-line functional/no-loop-statements
    for (const key of directKeys) {
      const value = this.toFiniteNumber(usage[key]);
      if (value !== undefined && value > 0) {
        metadata = this.mergeProviderMetadata(metadata, { cacheWriteInputTokens: value });
        break;
      }
    }
    const nestedKeys = ['cacheCreation', 'cache_creation'];
    // eslint-disable-next-line functional/no-loop-statements
    for (const key of nestedKeys) {
      const nested = usage[key];
      if (!this.isPlainObject(nested)) continue;
      const value = this.toFiniteNumber(nested.ephemeral_5m_input_tokens);
      if (value !== undefined && value > 0) {
        metadata = this.mergeProviderMetadata(metadata, { cacheWriteInputTokens: value });
        break;
      }
    }
    return metadata;
  }

  private extractMetadataFromResponse(response: unknown): ProviderTurnMetadata | undefined {
    if (!this.isPlainObject(response)) return undefined;
    let metadata: ProviderTurnMetadata | undefined;
    const model = typeof response.model === 'string' && response.model.length > 0 ? response.model : undefined;
    if (model !== undefined) {
      metadata = this.mergeProviderMetadata(metadata, { actualModel: model });
    }
    const provider = typeof response.provider === 'string' && response.provider.length > 0 ? response.provider : undefined;
    if (provider !== undefined) {
      metadata = this.mergeProviderMetadata(metadata, { actualProvider: provider });
    }
    const data = response.data;
    if (this.isPlainObject(data)) {
      const dataProvider = typeof data.provider === 'string' && data.provider.length > 0 ? data.provider : undefined;
      const dataModel = typeof data.model === 'string' && data.model.length > 0 ? data.model : undefined;
      if (dataProvider !== undefined) {
        metadata = this.mergeProviderMetadata(metadata, { actualProvider: dataProvider });
      }
      if (dataModel !== undefined) {
        metadata = this.mergeProviderMetadata(metadata, { actualModel: dataModel });
      }
    }
    return metadata;
  }

  private toFiniteNumber(value: unknown): number | undefined {
    if (typeof value === 'number' && Number.isFinite(value)) {
      return value;
    }
    if (typeof value === 'string' && value.length > 0) {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
    return undefined;
  }

  protected getReasoningLevelIndex(level: ReasoningLevel): number {
    const idx = BaseLLMProvider.REASONING_LEVELS.indexOf(level);
    return idx >= 0 ? idx : 0;
  }

  private getDefaultReasoningValue(level: ReasoningLevel, maxOutputTokens?: number): ProviderReasoningValue | null | undefined {
    if (this.reasoningDefaults !== undefined) {
      const value = this.reasoningDefaults[level];
      if (value !== undefined) {
        return value;
      }
    }
    if (this.reasoningLimits !== undefined) {
      return this.computeDynamicReasoningBudget(level, maxOutputTokens);
    }
    return undefined;
  }

  private computeDynamicReasoningBudget(level: ReasoningLevel, maxOutputTokens?: number): number {
    const limits = this.reasoningLimits ?? { min: 1, max: Number.MAX_SAFE_INTEGER };
    const available = (typeof maxOutputTokens === 'number' && Number.isFinite(maxOutputTokens))
      ? Math.max(1, Math.trunc(maxOutputTokens))
      : limits.max;
    const minBudget = Math.min(Math.max(limits.min, 1), available);
    const cappedAvailable = Math.min(available, limits.max);
    if (level === 'minimal') return minBudget;
    const ratio = level === 'high' ? 0.8 : (level === 'medium' ? 0.5 : 0.2);
    const computed = Math.max(minBudget, Math.floor(cappedAvailable * ratio));
    const bounded = Math.min(computed, cappedAvailable);
    return Math.max(1, bounded);
  }

  /**
   * Log comprehensive debug info for MODEL_ERROR cases.
   * This runs unconditionally (not just DEBUG=true) because MODEL_ERROR
   * causes retries and we need full diagnostics to fix root causes.
   */
  private logModelErrorDiagnostics(
    error: unknown,
    context: {
      composedMessage: string;
      name: string;
      status: number;
      codeStr?: string;
      providerMessage?: string;
      source: 'explicit' | 'default';
    }
  ): void {
    try {
      const safeStringify = (obj: unknown, maxLen = 5000): string => {
        try {
          const seen = new WeakSet<object>();
          const str = JSON.stringify(obj, (_key, value: unknown) => {
            if (typeof value === 'object' && value !== null) {
              if (seen.has(value)) return '[Circular]';
              seen.add(value);
            }
            if (typeof value === 'function') return '[Function]';
            if (value instanceof Error) {
              return { name: value.name, message: value.message, stack: value.stack };
            }
            return value;
          }, 2);
          return str.length > maxLen ? `${str.slice(0, maxLen)}...[truncated]` : str;
        } catch {
          return '[unserializable]';
        }
      };

      const errObj = error as Record<string, unknown> | null | undefined;
      const ctorName = errObj?.constructor !== undefined ? (errObj.constructor as { name?: string }).name : undefined;
      // Extract error name safely - ensure string type to avoid [object Object]
      const rawName = errObj?.name;
      const errorName = typeof rawName === 'string' ? rawName : (typeof ctorName === 'string' ? ctorName : typeof error);
      const rawMessage = errObj?.message;
      const errorMessage = typeof rawMessage === 'string' ? rawMessage : String(error);
      const errorStack = errObj?.stack;
      const errorCause = errObj?.cause;
      const lastError = errObj?.lastError;

      warn(`[MODEL_ERROR_DIAGNOSTIC] ========== MODEL_ERROR DETECTED ==========`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Source: ${context.source}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Composed message: ${context.composedMessage}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Error name: ${errorName}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Error message: ${errorMessage}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] HTTP status: ${String(context.status)}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Code: ${context.codeStr ?? 'none'}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] Provider message: ${context.providerMessage ?? 'none'}`);
      if (errorStack !== undefined) {
        warn(`[MODEL_ERROR_DIAGNOSTIC] Stack trace:\n${typeof errorStack === 'string' ? errorStack : safeStringify(errorStack)}`);
      }
      if (errorCause !== undefined) {
        warn(`[MODEL_ERROR_DIAGNOSTIC] Cause: ${safeStringify(errorCause)}`);
      }
      if (lastError !== undefined) {
        warn(`[MODEL_ERROR_DIAGNOSTIC] LastError: ${safeStringify(lastError)}`);
      }
      warn(`[MODEL_ERROR_DIAGNOSTIC] Full error object: ${safeStringify(error)}`);
      warn(`[MODEL_ERROR_DIAGNOSTIC] ==========================================`);
    } catch (e) {
      warn(`[MODEL_ERROR_DIAGNOSTIC] Failed to log diagnostics: ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  public mapError(error: unknown): TurnStatus {
    const UNKNOWN_ERROR_MESSAGE = 'Unknown error';
    if (error === null || error === undefined) return { type: 'invalid_response', message: UNKNOWN_ERROR_MESSAGE };

    const err = error as Record<string, unknown>;
    const firstDefined = (...vals: unknown[]) => vals.find(v => v !== undefined && v !== null);
    const nested = (obj: unknown, path: string[]): unknown => {
      // Safe nested property access
      let cur: unknown = obj;
      // eslint-disable-next-line functional/no-loop-statements
      for (const key of path) {
        if (cur === null || cur === undefined) return undefined;
        if (typeof cur !== 'object') return undefined;
         
         
        const rec = cur as Record<string, unknown>;
        cur = rec[key];
      }
      return cur;
    };

    const asString = (v: unknown): string | undefined => typeof v === 'string' ? v : undefined;
    // const asNumber = (v: unknown): number | undefined => typeof v === 'number' && Number.isFinite(v) ? v : undefined;
    const isRecord = (v: unknown): v is Record<string, unknown> => v !== null && v !== undefined && typeof v === 'object';

    // Unwrap common wrapper errors (e.g., RetryError with lastError, nested causes)
    const unwrap = (e: unknown): Record<string, unknown> => {
      let cur: unknown = e;
      // eslint-disable-next-line functional/no-loop-statements
      for (let i = 0; i < 4; i++) {
        if (!isRecord(cur)) break;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
        const rec = cur as Record<string, unknown>;
        // Prefer lastError if available
        const last = rec.lastError;
        if (last !== undefined) { cur = last; continue; }
        // Else unwrap cause
        const cause = rec.cause;
        if (cause !== undefined) { cur = cause; continue; }
        // Else unwrap errors array last element
        const errs = rec.errors;
        if (Array.isArray(errs) && errs.length > 0) { cur = errs[errs.length - 1] as unknown; continue; }
        break;
      }
      return (isRecord(cur) ? cur : (e as Record<string, unknown>));
    };

    const primary = unwrap(err);

    const status = (firstDefined(primary.statusCode, primary.status, nested(primary, ['response', 'status'])) as number | undefined) ?? 0;
    const statusTextCandidate = firstDefined(
      // common locations
      nested(primary, ['statusText']),
      nested(primary, ['response', 'statusText'])
    );
    const statusTextRaw = typeof statusTextCandidate === 'string' ? statusTextCandidate : undefined;
    const codeCandidate = firstDefined(
      nested(primary, ['code']),
      nested(primary, ['data', 'error', 'status']),
      nested(primary, ['data', 'error', 'code']),
      nested(primary, ['response', 'data', 'error', 'code']),
      nested(primary, ['error', 'code'])
    );
    let codeStr = typeof codeCandidate === 'string' ? codeCandidate : (typeof codeCandidate === 'number' ? String(codeCandidate) : undefined);
    // Try multiple sources for provider error message
    let providerMessageRaw = firstDefined(
      // provider/SDK nested error messages
      nested(primary, ['data', 'error', 'message']),
      nested(primary, ['response', 'data', 'error', 'message']),
      nested(primary, ['error', 'message']),
      nested(primary, ['message'])
    ) as string | undefined;
    // Parse responseBody JSON (if present) and override with upstream nested details from error.metadata.raw
    {
      const rb = asString((primary as { responseBody?: unknown }).responseBody);
      if (rb !== undefined && rb.trim().length > 0) {
        try {
          const parsed = JSON.parse(rb) as { error?: { message?: string; metadata?: { raw?: string; provider_name?: string }; code?: number | string; status?: string } };
          // Prefer upstream nested raw error message if available
          const rawInner = parsed.error?.metadata?.raw;
          if (typeof rawInner === 'string' && rawInner.trim().length > 0) {
            try {
              const inner = JSON.parse(rawInner) as { error?: { message?: string; status?: string } };
              if (typeof inner.error?.message === 'string') providerMessageRaw = inner.error.message;
            } catch {
              // If raw is not JSON, use it verbatim as message
              providerMessageRaw = rawInner;
            }
          }
          // If still no message, fallback to top-level error.message
          providerMessageRaw ??= (typeof parsed.error?.message === 'string' ? parsed.error.message : undefined);
          providerMessageRaw ??= rb;
        } catch {
          // keep providerMessageRaw as-is when body isn't JSON
        }
      }
    }

    // Recompute code using responseBody metadata.raw inner status when available
    if ((codeStr === undefined || /^\d+$/.test(codeStr)) && typeof (primary as { responseBody?: unknown }).responseBody === 'string') {
      try {
        const parsed = JSON.parse((primary as { responseBody?: unknown }).responseBody as string) as { error?: { metadata?: { raw?: string } } };
        const rawInner = parsed.error?.metadata?.raw;
        if (typeof rawInner === 'string') {
          try {
            const inner = JSON.parse(rawInner) as { error?: { status?: string } };
            const innerStatus = inner.error?.status;
            if (typeof innerStatus === 'string' && innerStatus.length > 0) {
              codeStr ??= innerStatus;
            }
          } catch (e) { try { warn(`fetch body text read failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
        }
      } catch (e) { try { warn(`provider traced fetch failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
    }

    const nameVal = (primary as { name?: unknown }).name;
    const name = typeof nameVal === 'string' ? nameVal : 'Error';

    const sanitize = (s: string | undefined): string | undefined => {
      if (typeof s !== 'string') return undefined;
      // collapse whitespace and newlines; trim and cap length
      const oneLine = s.replace(/\s+/g, ' ').trim();
      return oneLine.length > 200 ? `${oneLine.slice(0, 197)}...` : oneLine;
    };
    const statusText = sanitize(statusTextRaw);
    const providerMessage = sanitize(providerMessageRaw) ?? UNKNOWN_ERROR_MESSAGE;
    const codeText = sanitize(codeStr);

    // Build a concise message with available fields
    const prefixParts: string[] = [];
    if (status > 0) prefixParts.push(String(status));
    if (statusText !== undefined && statusText.length > 0) prefixParts.push(statusText);
    if (codeText !== undefined && codeText.length > 0) prefixParts.push(`[${codeText}]`);
    const prefix = prefixParts.join(' ');
    const composedMessage = prefix.length > 0 ? `${prefix}: ${providerMessage}` : providerMessage;

    if (process.env.DEBUG === 'true') {
      try {
        const body = (primary as { responseBody?: unknown }).responseBody;
        const bodyLen = typeof body === 'string' ? body.length : 0;
        const debugPayload = {
          name,
          status,
          statusText: statusText ?? null,
          code: codeStr ?? null,
          hasResponseBody: typeof body === 'string',
          responseBodyLen: bodyLen,
          hasErrorData: typeof (primary as { data?: unknown }).data === 'object',
          messageComposed: composedMessage,
        };
        warn(`[DEBUG] mapError: ${JSON.stringify(debugPayload)}`);
      } catch { /* ignore debug errors */ }
    }

    // Rate limit errors
    if (status === 429 || name.includes('RateLimit') || providerMessage.toLowerCase().includes('rate limit')) {
      const toRecord = (val: unknown): Record<string, unknown> | undefined => (val !== null && typeof val === 'object' && !Array.isArray(val)) ? val as Record<string, unknown> : undefined;
      const hasGetter = (val: unknown): val is { get: (key: string) => unknown } => val !== null && typeof val === 'object' && typeof (val as { get?: unknown }).get === 'function';

      interface RetryCandidate { value: unknown; hint?: 'seconds' | 'milliseconds' | 'date'; origin: string; rawValue: unknown }
      const candidates: RetryCandidate[] = [];
      const addCandidate = (value: unknown, hint: RetryCandidate['hint'], origin: string) => {
        if (value !== undefined && value !== null) {
          candidates.push({ value, hint, origin, rawValue: value });
        }
      };

      const lowerKey = (key: string) => key.toLowerCase();
      const getHeaderValue = (headers: unknown, key: string): unknown => {
        if (headers === undefined || headers === null) return undefined;
        if (hasGetter(headers)) {
          try {
            const result = headers.get(key);
            if (result !== null && result !== undefined) return result;
          } catch {
            // Ignore getter failures
          }
        }
        const record = toRecord(headers);
        if (record === undefined) return undefined;
        const sought = lowerKey(key);
        const entry = Object.entries(record).find(([k]) => lowerKey(k) === sought);
        return entry?.[1];
      };

      const headerSources: { headers: unknown; label: string }[] = [
        { headers: err.headers, label: 'error.headers' },
        { headers: (primary as { headers?: unknown }).headers, label: 'headers' },
        { headers: (primary as { responseHeaders?: unknown }).responseHeaders, label: 'responseHeaders' },
        { headers: (primary as { response?: { headers?: unknown } }).response?.headers, label: 'response.headers' },
      ];
      const headerKeys: { key: string; hint?: RetryCandidate['hint'] }[] = [
        { key: 'retry-after', hint: 'seconds' },
        { key: 'retry_after', hint: 'seconds' },
        { key: 'retry-after-ms', hint: 'milliseconds' },
        { key: 'retry_after_ms', hint: 'milliseconds' },
        { key: 'x-ratelimit-reset', hint: 'date' },
        { key: 'x-ratelimit-retry-after', hint: 'seconds' },
        { key: 'anthropic-ratelimit-requests-reset', hint: 'date' },
        { key: 'anthropic-ratelimit-tokens-reset', hint: 'date' },
      ];
      headerSources.forEach(({ headers, label }) => {
        headerKeys.forEach(({ key, hint }) => {
          const value = getHeaderValue(headers, key);
          addCandidate(value, hint ?? (key.includes('ms') ? 'milliseconds' : undefined), `${label}.${key}`);
        });
      });

      const nestedMetadataPaths: string[][] = [
        ['data', 'error', 'metadata', 'retry_after_ms'],
        ['data', 'error', 'metadata', 'retry_after'],
        ['data', 'error', 'metadata', 'retry_after_seconds'],
        ['error', 'metadata', 'retry_after_ms'],
        ['error', 'metadata', 'retry_after'],
        ['error', 'metadata', 'retry_after_seconds'],
        ['metadata', 'retry_after_ms'],
        ['metadata', 'retry_after'],
        ['metadata', 'retry_after_seconds'],
      ];
      nestedMetadataPaths.forEach((path) => {
        const value = nested(primary, path);
        if (value !== undefined) {
          const hint = path[path.length - 1].includes('ms') ? 'milliseconds' : (path[path.length - 1].includes('seconds') ? 'seconds' : undefined);
          addCandidate(value, hint, `metadata.${path.join('.')}`);
        }
      });

      const retryAfterFields: { value: unknown; hint?: RetryCandidate['hint'] }[] = [
        { value: (primary as { retryAfterMs?: unknown }).retryAfterMs, hint: 'milliseconds' },
        { value: (primary as { retryAfter?: unknown }).retryAfter, hint: 'seconds' },
        { value: (err as { retryAfterMs?: unknown }).retryAfterMs, hint: 'milliseconds' },
        { value: (err as { retryAfter?: unknown }).retryAfter, hint: 'seconds' },
      ];
      retryAfterFields.forEach(({ value, hint }, index) => {
        addCandidate(value, hint, `field[${String(index)}]`);
      });

      const parseCandidate = (candidate: RetryCandidate): number | undefined => {
        const { value, hint } = candidate;
        const coerceNumber = (num: number, unitHint?: RetryCandidate['hint']): number => {
          if (!Number.isFinite(num)) return NaN;
          if (unitHint === 'milliseconds') return num;
          if (unitHint === 'seconds') return num * 1000;
          if (num >= 1_000_000) return num; // assume already milliseconds
          return num * 1000;
        };
        if (typeof value === 'number') {
          return coerceNumber(value, hint);
        }
        if (typeof value === 'string') {
          const trimmed = value.trim();
          if (trimmed.length === 0) return undefined;
          if (hint === 'date') {
            const dateTs = Date.parse(trimmed);
            if (!Number.isNaN(dateTs)) {
              const delta = dateTs - Date.now();
              return delta > 0 ? delta : 0;
            }
          }
          const numeric = Number(trimmed);
          if (!Number.isNaN(numeric)) {
            return coerceNumber(numeric, hint);
          }
          const parsedDate = Date.parse(trimmed);
          if (!Number.isNaN(parsedDate)) {
            const delta = parsedDate - Date.now();
            return delta > 0 ? delta : 0;
          }
          return undefined;
        }
        if (value instanceof Date) {
          const delta = value.getTime() - Date.now();
          return delta > 0 ? delta : 0;
        }
        return undefined;
      };

      const evaluated = candidates.map((candidate) => ({
        origin: candidate.origin,
        rawValue: candidate.rawValue,
        ms: parseCandidate(candidate),
      }));
      // Find the maximum retry delay to be conservative (longer wait = safer)
      const validEntries = evaluated.filter((entry) => typeof entry.ms === 'number' && Number.isFinite(entry.ms) && entry.ms > 0);
      const winner = validEntries.length > 0
        ? validEntries.reduce((max, entry) => (entry.ms !== undefined && entry.ms > (max.ms ?? 0) ? entry : max))
        : undefined;
      const retryAfterMs = winner?.ms;
      // Format source with value for clarity
      const formatSource = (origin: string, rawValue: unknown): string => {
        const valueStr = typeof rawValue === 'string' ? rawValue : JSON.stringify(rawValue);
        return `${origin}=${valueStr}`;
      };
      const sources = winner !== undefined ? [formatSource(winner.origin, winner.rawValue)] : undefined;
      return { type: 'rate_limit', retryAfterMs, sources };
    }

    // Authentication errors
    if (status === 401 || status === 403 || name.includes('Auth') || providerMessage.toLowerCase().includes('authentication') || providerMessage.toLowerCase().includes('unauthorized')) {
      return { type: 'auth_error', message: composedMessage };
    }

    // Quota exceeded
    if (status === 402 || providerMessage.toLowerCase().includes('quota') || providerMessage.toLowerCase().includes('billing') || providerMessage.toLowerCase().includes('insufficient')) {
      return { type: 'quota_exceeded', message: composedMessage };
    }

    // Model errors
    if (status === 400 || name.includes('BadRequest') || providerMessage.toLowerCase().includes('invalid') || providerMessage.toLowerCase().includes('model')) {
      const lower = providerMessage.toLowerCase();
      const retryable = !(lower.includes('permanently') || lower.includes('unsupported'));
      this.logModelErrorDiagnostics(error, { composedMessage, name, status, codeStr, providerMessage, source: 'explicit' });
      return { type: 'model_error', message: composedMessage, retryable };
    }

    // Timeout errors
    if (name.includes('Timeout') || name.includes('AbortError') || providerMessage.toLowerCase().includes('timeout') || providerMessage.toLowerCase().includes('aborted')) {
      return { type: 'timeout', message: composedMessage };
    }

    // Network errors
    if (status >= 500 || name.includes('Network') || name.includes('ECONNRESET') || name.includes('ENOTFOUND') || name.includes('ENETUNREACH') || name.includes('EHOSTUNREACH') || name.includes('ECONNREFUSED') || providerMessage.toLowerCase().includes('network') || providerMessage.toLowerCase().includes('connection')) {
      return { type: 'network_error', message: composedMessage, retryable: true };
    }

    // Default to model error with retryable flag
    this.logModelErrorDiagnostics(error, { composedMessage, name, status, codeStr, providerMessage, source: 'default' });
    return { type: 'model_error', message: composedMessage, retryable: true };
  }

  protected extractTokenUsage(usage: Record<string, unknown> | undefined): TokenUsage {
    const num = (v: unknown): number => {
      if (typeof v === 'number' && Number.isFinite(v)) return v;
      if (typeof v === 'string') {
        const n = Number(v);
        return Number.isFinite(n) ? n : 0;
      }
      return 0;
    };
    const isRecord = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object';
    const getNestedNum = (obj: Record<string, unknown>, path: string[]): number => {
      // Safe nested lookup for numeric fields; returns 0 if not found or not numeric
      let cur: unknown = obj;
      // eslint-disable-next-line functional/no-loop-statements
      for (const key of path) {
        if (cur === null || cur === undefined || typeof cur !== 'object') return 0;
        const rec = cur as Record<string, unknown>;
        cur = rec[key];
      }
      return typeof cur === 'number' && Number.isFinite(cur) ? cur : (typeof cur === 'string' && Number.isFinite(Number(cur)) ? Number(cur) : 0);
    };
    const u: Record<string, unknown> = isRecord(usage) ? usage : {};
    const get = (key: string): unknown => (Object.prototype.hasOwnProperty.call(u, key) ? u[key] : undefined);
    const inputTokens = num(get('inputTokens')) || num(get('prompt_tokens')) || num(get('input_tokens'));
    const outputTokens = num(get('outputTokens')) || num(get('completion_tokens')) || num(get('output_tokens'));
    // Cache reads reported by multiple providers in different fields:
    // - OpenAI: prompt_tokens_details.cached_tokens or input_tokens_details.cached_tokens (AI SDK normalizes to usage.cachedInputTokens)
    // - Anthropic: cache_read_input_tokens (AI SDK normalizes to usage.cachedInputTokens)
    // - Google: usageMetadata.cachedContentTokenCount (AI SDK normalizes to usage.cachedInputTokens)
    const cachedReadRaw = num(get('cachedInputTokens')) || num(get('cached_tokens'));
    const cacheReadInputTokens = cachedReadRaw > 0 ? cachedReadRaw : undefined;
    // Back-compat aggregate cachedTokens equals read tokens when available
    const cachedTokens = cacheReadInputTokens;
    // Some providers (Anthropic via AI SDK) may also report cache creation tokens on the usage object
    // Try multiple possible keys to keep things robust across SDK versions
    // Try multiple sources for cache write tokens (provider/SDK variants)
    let cacheWriteRaw =
      num(get('cacheWriteInputTokens'))
      || num(get('cacheCreationInputTokens'))
      // Anthropic API may expose snake_case creation field
      || num(get('cache_creation_input_tokens'))
      // Some providers/SDKs may report a generic write field
      || num(get('cache_write_input_tokens'));
    if (cacheWriteRaw === 0) {
      // Look into nested token details often used by OpenAI/AI SDK
      const nestedCandidates: string[][] = [
        ['promptTokensDetails', 'cacheCreationTokens'],
        ['prompt_tokens_details', 'cache_creation_tokens'],
        ['inputTokensDetails', 'cacheCreationTokens'],
        ['input_tokens_details', 'cache_creation_tokens'],
      ];
      const found = nestedCandidates
        .map((p) => getNestedNum(u, p))
        .find((v) => v > 0);
      if (typeof found === 'number' && found > 0) cacheWriteRaw = found;
    }
    const cacheWriteInputTokens = cacheWriteRaw > 0 ? cacheWriteRaw : undefined;
    const totalTokensRaw = num(u.totalTokens) || num(u.total_tokens);
    const totalTokens = totalTokensRaw > 0 ? totalTokensRaw : (inputTokens + outputTokens);

    return {
      inputTokens,
      outputTokens,
      cachedTokens: cachedTokens !== undefined && cachedTokens > 0 ? cachedTokens : undefined,
      cacheReadInputTokens,
      cacheWriteInputTokens,
      totalTokens
    };
  }

  protected createSuccessResult(
    messages: ConversationMessage[],
    tokens: TokenUsage,
    latencyMs: number,
    hasToolCalls: boolean,
    finalAnswer: boolean,
    response?: string,
    extras?: { hasReasoning?: boolean; hasContent?: boolean; stopReason?: string },
    metadata?: ProviderTurnMetadata
  ): TurnResult {
    const combinedMetadata = this.mergeProviderMetadata(
      this.consumeQueuedProviderMetadata(),
      metadata
    );

    const result: TurnResult = {
      status: { type: 'success', hasToolCalls, finalAnswer },
      response,
      toolCalls: hasToolCalls ? this.extractToolCalls(messages) : undefined,
      tokens,
      latencyMs,
      messages,
      hasReasoning: extras?.hasReasoning,
      hasContent: extras?.hasContent,
      stopReason: extras?.stopReason,
    };

    if (combinedMetadata !== undefined) {
      result.providerMetadata = combinedMetadata;
    };
    return result;
  }

  protected extractStopReason(value: unknown): string | undefined {
    const visited = new Set<unknown>();
    const helper = (val: unknown): string | undefined => {
      if (val === null || val === undefined) return undefined;
      if (typeof val === 'string' || typeof val === 'number' || typeof val === 'boolean') return undefined;
      if (visited.has(val)) return undefined;
      if (Array.isArray(val)) {
        visited.add(val);
        // eslint-disable-next-line functional/no-loop-statements
        for (const item of val) {
          const found = helper(item);
          if (found !== undefined) return found;
        }
        return undefined;
      }
      if (typeof val !== 'object') return undefined;
      visited.add(val);
      const record = val as Record<string, unknown>;
      // Direct keys first
      // eslint-disable-next-line functional/no-loop-statements
      for (const key of BaseLLMProvider.STOP_REASON_KEYS) {
        const candidate = record[key];
        if (typeof candidate === 'string' && candidate.length > 0) return candidate;
      }
      // Recurse into nested values
      // eslint-disable-next-line functional/no-loop-statements
      for (const entry of Object.values(record)) {
        const found = helper(entry);
        if (found !== undefined) return found;
      }
      return undefined;
    };
    return helper(value);
  }

  protected isRefusalStopReason(stopReason: string | undefined): stopReason is string {
    if (typeof stopReason !== 'string') return false;
    const normalized = stopReason.trim().toLowerCase();
    return BaseLLMProvider.REFUSAL_STOP_REASONS.has(normalized);
  }

  protected extractToolCalls(messages: ConversationMessage[]) {
    return messages
      .filter(m => m.role === 'assistant' && m.toolCalls !== undefined)
      .flatMap(m => m.toolCalls ?? []);
  }

  // DRY helpers for final-turn behavior across providers
  // The LLM uses XML wrapper to deliver final reports, so agent__final_report
  // is never exposed as a callable tool. It exists in the internal list only
  // for core's final-turn enforcement logic.
  protected filterToolsForFinalTurn(tools: MCPTool[], _isFinalTurn?: boolean): MCPTool[] {
    return tools.filter((t) => t.name !== 'agent__final_report');
  }

  protected buildFinalTurnMessages(messages: ModelMessage[], _isFinalTurn?: boolean): ModelMessage[] {
    return messages;
  }

  protected createFailureResult(request: TurnRequest, status: TurnStatus, latencyMs: number): TurnResult {
    const metadata = this.consumeQueuedProviderMetadata();
    const result: TurnResult = {
      status,
      latencyMs,
      messages: []
    };
    if (metadata !== undefined) {
      result.providerMetadata = metadata;
    }
    const retry = this.buildRetryDirective(request, status);
    if (retry !== undefined) {
      result.retry = retry;
    }
    return result;
  }

  protected convertTools(tools: MCPTool[], toolExecutor: (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }) => Promise<string>): ToolSet {
    return Object.fromEntries(
      tools.map(tool => [
        tool.name,
        {
          description: tool.description,
          inputSchema: jsonSchema(this.sanitizeStringFormatsInSchema(tool.inputSchema)),
          execute: async (args: unknown, opt?: { toolCallId?: string }) => {
            const parameters = args as Record<string, unknown>;
            return await toolExecutor(tool.name, parameters, { toolCallId: opt?.toolCallId });
          }
        }
      ])
    ) as unknown as ToolSet;
  }

  // Constants
  private static readonly PART_REASONING = 'reasoning' as const;
  private static readonly PART_TOOL_RESULT = 'tool_result' as const; // avoid duplicate literal warning
  private static readonly PART_TOOL_ERROR = 'tool_error' as const; // error variant from AI SDK
  private readonly REASONING_TYPE: string = BaseLLMProvider.PART_REASONING.replace('_','-');
  private readonly TOOL_RESULT_TYPE: string = BaseLLMProvider.PART_TOOL_RESULT.replace('_','-');
  private readonly TOOL_ERROR_TYPE: string = BaseLLMProvider.PART_TOOL_ERROR.replace('_','-');

  private static isClosedStreamControllerError(error: unknown): boolean {
    if (!(error instanceof Error)) return false;
    const msg = error.message.toLowerCase();
    return msg.includes('controller is already closed') || msg.includes('readablestream is already closed');
  }
  
  // Shared streaming utilities
  protected createTimeoutController(timeoutMs: number): { controller: AbortController; resetIdle: () => void; clearIdle: () => void; didTimeout: () => boolean } {
    const controller = new AbortController();
    let idleTimer: ReturnType<typeof setTimeout> | undefined;
    let timedOut = false;

    const resetIdle = () => {
      try { if (idleTimer !== undefined) clearTimeout(idleTimer); } catch {}
      timedOut = false;
      idleTimer = setTimeout(() => {
        timedOut = true;
        try { controller.abort(); } catch {}
      }, timeoutMs);
    };

    const clearIdle = () => { 
      try { if (idleTimer !== undefined) { clearTimeout(idleTimer); idleTimer = undefined; } } catch {}
      timedOut = false;
    };

    return { controller, resetIdle, clearIdle, didTimeout: () => timedOut };
  }

  protected async drainTextStream(stream: StreamTextResult<ToolSet, boolean>): Promise<string> {
    const iterator = stream.textStream[Symbol.asyncIterator]();
    let response = '';
    let step = await iterator.next();
    
    // eslint-disable-next-line functional/no-loop-statements
    while (step.done !== true) {
      const chunk = step.value;
      response += chunk;
      step = await iterator.next();
    }

    return response;
  }

  private normalizeFormatPolicy(policy?: FormatPolicyInput): FormatPolicyNormalized {
    const allowedRaw = Array.isArray(policy?.allowed) ? policy.allowed : ['*'];
    const deniedRaw = Array.isArray(policy?.denied) ? policy.denied : [];
    return {
      allowed: this.normalizeFormatList(allowedRaw),
      denied: this.normalizeFormatList(deniedRaw)
    };
  }

  private normalizeFormatList(values: string[]): FormatList {
    const expanded: string[] = [];
    values.forEach((raw) => {
      if (typeof raw !== 'string') return;
      const trimmed = raw.trim();
      if (trimmed.length === 0) return;
      if (trimmed.toLowerCase() === 'guidance') {
        GUIDANCE_STRING_FORMATS.forEach((fmt) => { expanded.push(fmt); });
      } else {
        expanded.push(trimmed);
      }
    });

    if (expanded.length === 0) return { tokens: new Set<string>(), wildcard: false, empty: true };

    let wildcard = false;
    const tokens = new Set<string>();

    expanded.forEach((value) => {
      const lower = value.toLowerCase();
      if (lower === '*' || lower === 'any') {
        wildcard = true;
        return;
      }
      tokens.add(lower);
    });

    return { tokens, wildcard, empty: false };
  }

  private sanitizeStringFormatsInSchema(schema: Record<string, unknown>): Record<string, unknown> {
    return this.sanitizeSchemaNode(schema) as Record<string, unknown>;
  }

  private sanitizeSchemaNode(node: unknown): unknown {
    if (Array.isArray(node)) {
      return node.map((item) => this.sanitizeSchemaNode(item));
    }
    if (node !== null && typeof node === 'object') {
      const obj = node as Record<string, unknown>;
      const out: Record<string, unknown> = {};
      Object.entries(obj).forEach(([key, value]) => {
        out[key] = this.sanitizeSchemaNode(value);
      });
      if (typeof out.format === 'string' && this.shouldStripStringFormat(out.type, out.format)) {
        delete out.format;
      }
      return out;
    }
    return node;
  }

  private shouldStripStringFormat(typeValue: unknown, formatValue: string): boolean {
    const normalizedFormat = formatValue.trim().toLowerCase();
    if (normalizedFormat.length === 0) return false;
    if (!this.schemaTypeIncludesString(typeValue)) return false;

    const allowed = this.stringFormatPolicy.allowed;
    const denied = this.stringFormatPolicy.denied;

    const isAllowed = allowed.empty ? false : (allowed.wildcard || allowed.tokens.has(normalizedFormat));
    const isDenied = denied.empty ? false : (denied.wildcard || denied.tokens.has(normalizedFormat));

    if (!isAllowed) return true;
    if (isDenied) return true;
    return false;
  }

  private schemaTypeIncludesString(typeValue: unknown): boolean {
    if (typeValue === undefined) return true;
    if (typeof typeValue === 'string') return typeValue === 'string';
    if (Array.isArray(typeValue)) {
      return typeValue.some((entry) => typeof entry === 'string' && entry === 'string');
    }
    return false;
  }

  protected async executeStreamingTurn(
    model: LanguageModel,
    messages: ModelMessage[],
    tools: ToolSet | undefined,
    request: TurnRequest,
    startTime: number,
    providerOptions?: unknown
  ): Promise<TurnResult> {
    const timeoutMs = request.llmTimeout ?? 600000;
    const { controller, resetIdle, clearIdle, didTimeout } = this.createTimeoutController(timeoutMs);
    try {
      if (request.abortSignal !== undefined) {
        if (request.abortSignal.aborted) { controller.abort(); }
        else {
          // Tie external abort to our controller
          const onAbort = () => { try { controller.abort(); } catch (e) { try { warn(`controller.abort failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} } };
          request.abortSignal.addEventListener('abort', onAbort, { once: true });
        }
      }
    } catch (e) { try { warn(`fetch finalization failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
    
    // Capture ALL stream errors (not just closed stream) so we can provide better diagnostics
    // when NoOutputGeneratedError is thrown. The SDK's default onError just logs to console.
    // Declared outside try so it's accessible in catch block.
    let capturedStreamError: Error | undefined;
    const captureStreamError = (error: unknown): void => {
      // Keep the first error (usually the root cause)
      if (capturedStreamError !== undefined) return;
      if (error instanceof Error) {
        capturedStreamError = error;
      } else {
        // Serialize objects properly - SDK passes { error: ... } wrapper
        try {
          capturedStreamError = new Error(JSON.stringify(error));
        } catch {
          capturedStreamError = new Error(String(error));
        }
      }
    };

    try {
      let stopReason: string | undefined;

      const toolChoice = this.resolveToolChoice(request);
      this.traceSdkPayload(request, 'request', messages);
      const result = streamText({
        model,
        messages,
        tools,
        ...(toolChoice !== undefined ? { toolChoice } : {}),
        maxOutputTokens: request.maxOutputTokens,
        // Only include nullable params if they have a value (null = don't send)
        ...(request.temperature !== null ? { temperature: request.temperature } : {}),
        ...(request.topP !== null ? { topP: request.topP } : {}),
        ...(request.topK !== null ? { topK: request.topK } : {}),
        ...(request.repeatPenalty !== null ? { frequencyPenalty: request.repeatPenalty } : {}),
        providerOptions: providerOptions as never,
        abortSignal: controller.signal,
        onError: (err) => { captureStreamError(err); },
        onAbort: (err) => { captureStreamError(err); },
      });

      // Drain both text and reasoning streams with real-time callbacks
      resetIdle();
      const fullIterator = result.fullStream[Symbol.asyncIterator]();
      let response = '';
      let fullStep = await fullIterator.next();

      // Track pending tool executions to avoid idle timeout during long-running tools.
      // When AI SDK invokes our execute callback (e.g., subagent), no chunks are emitted
      // until execution completes. Without this, the idle timer would abort the stream.
      let pendingToolCalls = 0;

      // eslint-disable-next-line functional/no-loop-statements
      while (fullStep.done !== true) {
        const part = fullStep.value;

        if (part.type === 'text-delta') {
          response += part.text;

          // Call chunk callback for real-time text streaming
          // Skip empty chunks to prevent unnecessary thinking block interruptions
          if (request.onChunk !== undefined && part.text.length > 0) {
            request.onChunk(part.text, 'content');
          }
        } else if (part.type === 'reasoning-delta') {
          // Call chunk callback for real-time reasoning streaming
          // Skip empty chunks to avoid emitting empty thinking deltas
          if (request.onChunk !== undefined && part.text.length > 0) {
            request.onChunk(part.text, 'thinking');
          }
        } else if (part.type === 'tool-call') {
          // Tool execution starting - suspend idle timer until result arrives
          pendingToolCalls++;
          clearIdle();
        } else if (part.type === 'tool-result') {
          // Tool execution completed - resume idle timer if no more pending
          pendingToolCalls = Math.max(0, pendingToolCalls - 1);
        } else if (part.type === 'finish') {
          const fin = part as { finishReason?: string };
          if (typeof fin.finishReason === 'string' && fin.finishReason.length > 0) {
            stopReason = stopReason ?? fin.finishReason;
          }
        }

        // Only reset idle timer if no tool executions are pending
        if (pendingToolCalls === 0) {
          resetIdle();
        }
        fullStep = await fullIterator.next();
      }

      clearIdle();

      const awaitWithSuppression = async <T>(promise: Promise<T>): Promise<T | undefined> => {
        try {
          return await promise;
        } catch (err) {
          captureStreamError(err);
          if (BaseLLMProvider.isClosedStreamControllerError(err)) return undefined;
          throw err;
        }
      };

      const usage = await awaitWithSuppression(result.usage);
      const resp = await awaitWithSuppression(result.response);
      // providerMetadata is a separate property on the result, not nested in response
      const resultProviderMetadata = await awaitWithSuppression(result.providerMetadata);
      this.traceSdkPayload(request, 'response', resp);
      stopReason = stopReason ?? this.extractStopReason(resp) ?? this.extractStopReason(result);
      if (process.env.DEBUG === 'true') {
        try {
          const msgs = (resp as { messages?: unknown[] } | undefined)?.messages ?? [];
          let toolCalls = 0; const callIds: string[] = [];
          let toolResults = 0; const resIds: string[] = [];
          msgs.forEach((msg) => {
            const m = msg as { role?: string; content?: unknown };
            if (m.role === 'assistant' && Array.isArray(m.content)) {
              (m.content as unknown[]).forEach((part) => {
                const p = part as { type?: string; toolCallId?: string };
                if (p.type === 'tool-call') {
                  toolCalls++;
                  if (typeof p.toolCallId === 'string' && p.toolCallId.length > 0) callIds.push(p.toolCallId);
                }
              });
            } else if (m.role === 'tool' && Array.isArray(m.content)) {
              (m.content as unknown[]).forEach((part) => {
                const p = part as { type?: string; toolCallId?: string };
                const t = (p.type ?? '').replace('_','-');
                if (t === this.TOOL_RESULT_TYPE || t === this.TOOL_ERROR_TYPE) {
                  toolResults++;
                  if (typeof p.toolCallId === 'string' && p.toolCallId.length > 0) resIds.push(p.toolCallId);
                }
              });
            }
          });
          const dbgLabel = 'tool-parity(stream)';
          warn(`[DEBUG] ${dbgLabel}: ${JSON.stringify({ toolCalls, callIds, toolResults, resIds })}`);
        } catch (e) { try { warn(`extract usage json failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      const tokens = this.extractTokenUsage(usage);
      // Try to enrich with provider metadata (e.g., Anthropic cache creation tokens)
      // Note: In streaming, result.providerMetadata contains this data, NOT result.response.providerMetadata
      try {
        const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object';
        let w: unknown = undefined;
        if (isObj(resultProviderMetadata)) {
          const pm: Record<string, unknown> = resultProviderMetadata;
          const a = pm.anthropic;
          if (isObj(a)) {
            const ar: Record<string, unknown> = a;
            let cand: unknown = ar.cacheCreationInputTokens
              ?? ar.cache_creation_input_tokens
              ?? ar.cacheCreationTokens
              ?? ar.cache_creation_tokens;
            if (cand === undefined) {
              const cc = ar.cacheCreation;
              if (isObj(cc)) { cand = cc.ephemeral_5m_input_tokens; }
            }
            if (cand === undefined) {
              const cc2 = ar.cache_creation;
              if (isObj(cc2)) { cand = cc2.ephemeral_5m_input_tokens; }
            }
            w = cand;
          }
        }
        if (typeof w === 'number' && Number.isFinite(w)) {
          tokens.cacheWriteInputTokens = Math.trunc(w);
        }
      } catch { /* ignore metadata errors */ }
      const latencyMs = Date.now() - startTime;

      // Log if we captured a stream error that was recovered from (closed controller, etc.)
      if (capturedStreamError !== undefined && BaseLLMProvider.isClosedStreamControllerError(capturedStreamError)) {
        warn(`streamText closed controller recovered: ${capturedStreamError.message}`);
      }

      // Debug: log the raw response structure
      if (process.env.DEBUG === 'true') {
        const snippet = JSON.stringify((resp as { messages?: unknown[] } | undefined)?.messages, null, 2).substring(0, 2000);
        warn(`[DEBUG] resp.messages structure: ${snippet}`);
      }
      
      // Backfill: Emit chunks for content that wasn't streamed (common with tool calls)
      // Many providers don't stream text-delta or reasoning-delta when producing tool calls
      if (request.onChunk !== undefined && Array.isArray((resp as { messages?: unknown[] } | undefined)?.messages)) {
        const normalizeWhitespace = (input: string): string => input.replace(/\s+/g, ' ').trim();
        const hasStreamedNormalized = (): string => normalizeWhitespace(response);
        // eslint-disable-next-line functional/no-loop-statements
        for (const msg of (resp as { messages?: unknown[] } | undefined)?.messages ?? []) {
          const m = msg as { role?: string; content?: unknown };
          if (m.role === 'assistant' && Array.isArray(m.content)) {
            let hasEmittedReasoning = false;
            let hasEmittedText = false;
            
             
            // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
              const p = part as { type?: string; text?: string };
              
              // Emit reasoning if we haven't streamed it yet
              if (p.type === this.REASONING_TYPE && p.text !== undefined && p.text.length > 0) {
                if (!hasEmittedReasoning) {
                  // Only emit if we didn't already stream reasoning-delta
                  // We track this by checking if response already contains the reasoning
                  // (This is a heuristic - ideally we'd track what was actually streamed)
                  request.onChunk(p.text, 'thinking');
                  hasEmittedReasoning = true;
                }
              }
              
              // Emit text content if we haven't streamed it yet
              if (p.type === 'text' && p.text !== undefined && p.text.length > 0) {
                const alreadyStreamed = normalizeWhitespace(p.text).length > 0
                  ? hasStreamedNormalized().includes(normalizeWhitespace(p.text))
                  : false;
                if (!hasEmittedText && !alreadyStreamed) {
                  // Only emit if this text wasn't already streamed as text-delta
                  request.onChunk(p.text, 'content');
                  response += p.text; // Add to response for completeness
                  hasEmittedText = true;
                }
              }
            }
          }
        }
      }
      
      const normalizedMessages = this.normalizeResponseMessages(resp);
      const rawConversationMessages = this.convertResponseMessages(normalizedMessages, request.provider, request.model, tokens);
      const conversationMessages = this.injectMissingToolResults(rawConversationMessages, request.provider, request.model, tokens);
      if (process.env.DEBUG === 'true') {
        try {
          const tMsgs = conversationMessages.filter((m) => m.role === 'tool');
          const ids = tMsgs.map((m) => (m.toolCallId ?? '[none]'));
          warn(`[DEBUG] converted-tool-messages(stream): ${JSON.stringify({ count: tMsgs.length, ids })}`);
        } catch (e) { try { warn(`extract usage json failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      
      // Find the LAST assistant message
      const lastAssistantMessageArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = lastAssistantMessageArr.length > 0 ? lastAssistantMessageArr[lastAssistantMessageArr.length - 1] : undefined;

      // Detect final report request strictly by tool name, after normalization
      const normalizeName = (name: string) => sanitizeToolName(name);

      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
      const hasNewToolCalls = lastAssistantMessage?.toolCalls !== undefined &&
        lastAssistantMessage.toolCalls.filter(tc => validToolNames.includes(normalizeName(tc.name))).length > 0;
      const hasAssistantText = (lastAssistantMessage?.content.trim().length ?? 0) > 0;

      // Check if we have tool result messages (which means tools were executed)
      const hasToolResults = conversationMessages.some(m => m.role === 'tool');
      
      // Detect if provider emitted any reasoning parts
      const hasReasoningFromRaw = normalizedMessages.some((msg) => {
        if (msg.role !== 'assistant') return false;
        if (Array.isArray(msg.content)) {
          return msg.content.some((part) => (part as { type?: string }).type === this.REASONING_TYPE);
        }
        const reasoningSegments = this.collectReasoningSegmentsFromMessage(msg);
        return reasoningSegments.length > 0;
      });
      const hasReasoningFromConversation = conversationMessages.some((message) => Array.isArray(message.reasoning) && message.reasoning.length > 0);
      const hasReasoning = hasReasoningFromRaw || hasReasoningFromConversation;
      
      // FINAL ANSWER POLICY: orchestrator determines final answer after sanitization (XML/text). Providers report false.
      const finalAnswer = false;

      // Do not error on reasoning-only responses; allow agent loop to decide retries.
      
      // Debug logging
      if (process.env.DEBUG === 'true') {
        warn(`[DEBUG] Stream: hasNewToolCalls: ${String(hasNewToolCalls)}, hasAssistantText: ${String(hasAssistantText)}, hasToolResults: ${String(hasToolResults)}, finalAnswer: ${String(finalAnswer)}, response length: ${String(response.length)}`);
        warn(`[DEBUG] lastAssistantMessage: ${JSON.stringify(lastAssistantMessage, null, 2)}`);
        warn(`[DEBUG] response text: ${response}`);
      }

      const metadata = this.extractTurnMetadata(request, { usage, response: resp, latencyMs });

      if (this.isRefusalStopReason(stopReason)) {
        return {
          status: { type: 'invalid_response', message: `refusal:${stopReason}` },
          stopReason,
          tokens,
          latencyMs,
          messages: [],
          providerMetadata: metadata,
        } satisfies TurnResult;
      }

      return this.createSuccessResult(
        conversationMessages,
        tokens,
        latencyMs,
        hasNewToolCalls,
        finalAnswer,
        response,
        { hasReasoning, hasContent: hasAssistantText, stopReason },
        metadata
      );
    } catch (error) {
      const timedOut = didTimeout();
      clearIdle();
      // If we caught NoOutputGeneratedError but have a captured stream error, use that for better diagnostics
      // The SDK throws NoOutputGeneratedError without preserving the underlying provider error
      let errorForMapping = error;
      if (capturedStreamError !== undefined) {
        // Attach the captured error as cause so mapError can extract HTTP status, provider message, etc.
        const enhancedError = new Error(
          `${error instanceof Error ? error.message : String(error)} [underlying: ${capturedStreamError.message}]`
        );
        (enhancedError as { cause?: unknown }).cause = capturedStreamError;
        (enhancedError as { name?: string }).name = error instanceof Error ? error.name : 'Error';
        // Copy any additional properties from original error
        if (error instanceof Error) {
          Object.assign(enhancedError, error);
        }
        errorForMapping = enhancedError;
      }
      let failure = this.mapError(errorForMapping);
      if (timedOut) {
        failure = { type: 'timeout', message: 'Streaming timed out while waiting for the next chunk.' };
      }
      return this.createFailureResult(request, failure, Date.now() - startTime);
    }
  }

  protected async executeNonStreamingTurn(
    model: LanguageModel,
    messages: ModelMessage[],
    tools: ToolSet | undefined,
    request: TurnRequest,
    startTime: number,
    providerOptions?: unknown
  ): Promise<TurnResult> {
    try {
      // Combine external abort signal with a timeout controller
      const { controller } = this.createTimeoutController(request.llmTimeout ?? 600000);
      try {
        if (request.abortSignal !== undefined) {
          if (request.abortSignal.aborted) { controller.abort(); }
          else {
            const onAbort = () => { try { controller.abort(); } catch (e) { try { warn(`controller.abort failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} } };
            request.abortSignal.addEventListener('abort', onAbort, { once: true });
          }
        }
      } catch (e) { try { warn(`streaming read failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      const toolChoice = this.resolveToolChoice(request);
      this.traceSdkPayload(request, 'request', messages);
      const result = await generateText({
        model,
        messages,
        tools,
        ...(toolChoice !== undefined ? { toolChoice } : {}),
        maxOutputTokens: request.maxOutputTokens,
        // Only include nullable params if they have a value (null = don't send)
        ...(request.temperature !== null ? { temperature: request.temperature } : {}),
        ...(request.topP !== null ? { topP: request.topP } : {}),
        ...(request.topK !== null ? { topK: request.topK } : {}),
        ...(request.repeatPenalty !== null ? { frequencyPenalty: request.repeatPenalty } : {}),
        providerOptions: providerOptions as never,
        abortSignal: controller.signal,
      });

      let stopReason = this.extractStopReason(result) ?? this.extractStopReason((result as { response?: unknown }).response);
      const tokens = this.extractTokenUsage(result.usage);
      const latencyMs = Date.now() - startTime;
      let response = result.text;

      this.traceSdkPayload(request, 'response', (result as { response?: unknown }).response ?? result);
      
      // Debug logging to understand the response structure
      if (process.env.DEBUG === 'true') {
        warn(`[DEBUG] Non-stream result.text: ${result.text}`);
        warn(`[DEBUG] Non-stream result.response: ${JSON.stringify(result.response, null, 2).substring(0, 500)}`);
      }

      const respObj = (result.response as { messages?: unknown[]; providerMetadata?: Record<string, unknown> } | undefined) ?? {};
      stopReason = stopReason ?? this.extractStopReason(respObj);
      // Enrich with provider metadata (e.g., Anthropic cache creation tokens)
      try {
        const provMeta = respObj.providerMetadata;
        const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object';
        let w: unknown = undefined;
        if (isObj(provMeta)) {
          const pm: Record<string, unknown> = provMeta;
          const a = pm.anthropic;
          if (isObj(a)) {
            const ar: Record<string, unknown> = a;
            let cand: unknown = ar.cacheCreationInputTokens
              ?? ar.cache_creation_input_tokens
              ?? ar.cacheCreationTokens
              ?? ar.cache_creation_tokens;
            if (cand === undefined) {
              const cc = ar.cacheCreation;
              if (isObj(cc)) { cand = cc.ephemeral_5m_input_tokens; }
            }
            if (cand === undefined) {
              const cc2 = ar.cache_creation;
              if (isObj(cc2)) { cand = cc2.ephemeral_5m_input_tokens; }
            }
            w = cand;
          }
        }
        if (typeof w === 'number' && Number.isFinite(w)) {
          tokens.cacheWriteInputTokens = Math.trunc(w);
        }
      } catch (e) { try { warn(`json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      if (process.env.DEBUG === 'true') {
        try {
          const msgs = Array.isArray(respObj.messages) ? respObj.messages : [];
          let toolCalls = 0; const callIds: string[] = [];
          let toolResults = 0; const resIds: string[] = [];
          msgs.forEach((msg) => {
            const m = msg as { role?: string; content?: unknown };
            if (m.role === 'assistant' && Array.isArray(m.content)) {
              (m.content as unknown[]).forEach((part) => {
                const p = part as { type?: string; toolCallId?: string };
                if (p.type === 'tool-call') {
                  toolCalls++;
                  if (typeof p.toolCallId === 'string' && p.toolCallId.length > 0) callIds.push(p.toolCallId);
                }
              });
            } else if (m.role === 'tool' && Array.isArray(m.content)) {
              (m.content as unknown[]).forEach((part) => {
                const p = part as { type?: string; toolCallId?: string };
                const t = (p.type ?? '').replace('_','-');
                if (t === this.TOOL_RESULT_TYPE || t === this.TOOL_ERROR_TYPE) {
                  toolResults++;
                  if (typeof p.toolCallId === 'string' && p.toolCallId.length > 0) resIds.push(p.toolCallId);
                }
              });
            }
          });
          const dbgLabel = 'tool-parity(nonstream)';
          warn(`[DEBUG] ${dbgLabel}: ${JSON.stringify({ toolCalls, callIds, toolResults, resIds })}`);
        } catch (e) { try { warn(`json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }

      // Extract reasoning from AI SDK's normalized messages (for non-streaming mode)
      // The AI SDK puts reasoning as ReasoningPart in the content array
      if (request.onChunk !== undefined && Array.isArray(respObj.messages)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const msg of respObj.messages) {
          const m = msg as { role?: string; content?: unknown };
          if (m.role === 'assistant' && Array.isArray(m.content)) {
             
            // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
              const p = part as { type?: string; text?: string };
              // Handle reasoning parts from AI SDK format
              if (p.type === this.REASONING_TYPE && p.text !== undefined && p.text.length > 0) {
                request.onChunk(p.text, 'thinking');
              }
              // Also handle text parts for content streaming in non-streaming mode
              if (p.type === 'text' && p.text !== undefined && p.text.length > 0) {
                // Don't call onChunk for content here - it's already in result.text
                // This is just for building the complete response
              }
            }
          }
        }
      }
      
      // Debug: log the raw response structure for non-streaming
      if (process.env.DEBUG === 'true') {
        warn(`[DEBUG] Non-stream respObj.messages: ${JSON.stringify(respObj.messages, null, 2).substring(0, 2000)}`);
      }
      
      const normalizedMessages = this.normalizeResponseMessages(respObj);
      if (process.env.DEBUG === 'true' && Array.isArray(respObj.messages)) {
        try {
          console.log('normalizedMessages:', JSON.stringify(normalizedMessages, null, 2));
        } catch { /* ignore */ }
      }
      const rawConversationMessages = this.convertResponseMessages(
        normalizedMessages,
        request.provider,
        request.model,
        tokens
      );
      const conversationMessages = this.injectMissingToolResults(rawConversationMessages, request.provider, request.model, tokens);
      if (process.env.DEBUG === 'true') {
        try {
          const tMsgs = conversationMessages.filter((m) => m.role === 'tool');
          const ids = tMsgs.map((m) => (m.toolCallId ?? '[none]'));
          warn(`[DEBUG] converted-tool-messages(nonstream): ${JSON.stringify({ count: tMsgs.length, ids })}`);
        } catch (e) { try { warn(`json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      
      // Find the LAST assistant message to determine if we need more tool calls
      const laArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = laArr.length > 0 ? laArr[laArr.length - 1] : undefined;
      
      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
      const normalizeName = (name: string) => sanitizeToolName(name);
      const hasNewToolCalls = lastAssistantMessage?.toolCalls !== undefined &&
        lastAssistantMessage.toolCalls.filter(tc => validToolNames.includes(normalizeName(tc.name))).length > 0;
      const hasAssistantText = (lastAssistantMessage?.content.trim().length ?? 0) > 0;

      // Detect reasoning parts in non-streaming response
      const hasReasoningFromRaw = normalizedMessages.some((msg) => {
        if (msg.role !== 'assistant') return false;
        if (Array.isArray(msg.content)) {
          return msg.content.some((part) => (part as { type?: string }).type === this.REASONING_TYPE);
        }
        const segments = this.collectReasoningSegmentsFromMessage(msg);
        return segments.length > 0;
      });
      const hasReasoningFromConversation = conversationMessages.some((message) => Array.isArray(message.reasoning) && message.reasoning.length > 0);
      const hasReasoning = hasReasoningFromRaw || hasReasoningFromConversation;
      
      // FINAL ANSWER POLICY: only when agent_final_report tool is requested (normalized), regardless of other conditions
      const finalAnswer = false;

      // Do not error on reasoning-only responses; allow agent loop to decide retries.
      
      // Always try to extract the actual response text from the assistant message
      // The AI SDK's result.text might be empty when there are tool calls
      if (lastAssistantMessage !== undefined && hasAssistantText) {
        // First, check if we already have the response text directly
        if (!response) {
          response = lastAssistantMessage.content;
        }
        
        // If content looks like JSON, try to extract from structured format
        if (response.startsWith('[') || response.startsWith('{')) {
          try {
            const content = JSON.parse(response) as unknown;
          if (Array.isArray(content)) {
            // Look for text content parts (not reasoning or tool-result)
            const textParts = content.filter((part: unknown) => {
              const p = part as { type?: string; text?: string };
              // Accept plain text parts or parts without a type (default text)
              return (p.type === 'text' || p.type === undefined || p.type === '') && p.text !== undefined;
            });
            
            if (textParts.length > 0) {
              response = textParts.map((part: unknown) => (part as { text: string }).text).join('');
            } else {
              // No text parts found - this could mean tool errors occurred
              // In this case, construct a response from available information
              const allParts = content.filter((part: unknown) => {
                const p = part as { type?: string; text?: string; output?: { value?: string }; error?: unknown; content?: unknown };
                const normalizedType = (p.type ?? '').replace('_','-');
                return p.text !== undefined || 
                       p.output?.value !== undefined || 
                       (normalizedType === this.TOOL_ERROR_TYPE) ||
                       p.error !== undefined ||
                       p.content !== undefined;
              });
              
              const messages: string[] = [];
              // eslint-disable-next-line functional/no-loop-statements
              for (const part of allParts) {
                const p = part as { type?: string; text?: string; output?: { value?: string }; error?: unknown; content?: unknown };
                const normalizedType = (p.type ?? '').replace('_','-');
                
                if (normalizedType === this.TOOL_RESULT_TYPE && p.output?.value !== undefined) {
                  messages.push(p.output.value);
                } else if (normalizedType === this.TOOL_ERROR_TYPE) {
                  // Handle tool errors - they may have different structures
                  let errorMsg = 'Tool execution failed';
                  if (typeof p.error === 'string') {
                    errorMsg = p.error;
                  } else if (typeof p.error === 'object' && p.error !== null) {
                    const e = p.error as { message?: string; error?: string };
                    errorMsg = e.message ?? e.error ?? JSON.stringify(p.error);
                  } else if (typeof p.content === 'string') {
                    errorMsg = p.content;
                  } else if (p.output?.value !== undefined) {
                    errorMsg = p.output.value;
                  }
                  messages.push(`${TOOL_FAILED_PREFIX}${errorMsg}${TOOL_FAILED_SUFFIX}`);
                } else if (p.type !== this.REASONING_TYPE && p.text !== undefined) {
                  messages.push(p.text);
                }
              }
              
              if (messages.length > 0) {
                response = messages.join('\n');
              } else {
                // Fallback: show reasoning if nothing else
                const reasoningParts = content.filter((part: unknown) => {
                  const p = part as { type?: string; text?: string };
                  return p.type === this.REASONING_TYPE && p.text !== undefined;
                });
                if (reasoningParts.length > 0) {
                  response = 'I encountered an error while searching. ' + 
                            reasoningParts.map((part: unknown) => (part as { text: string }).text).join('');
                }
              }
            }
          }
          } catch {
            // If it's not JSON, keep the response as-is
          }
        }
      }

      if (request.onChunk !== undefined && response.length > 0) {
        const thinkSplit = new ThinkTagStreamFilter().process(response);
        if (thinkSplit.thinking.length > 0) {
          request.onChunk(thinkSplit.thinking, 'thinking');
        }
      }

      const metadata = this.extractTurnMetadata(request, { usage: result.usage, response: respObj, latencyMs });

      if (this.isRefusalStopReason(stopReason)) {
        return {
          status: { type: 'invalid_response', message: `refusal:${stopReason}` },
          stopReason,
          tokens,
          latencyMs,
          messages: [],
          providerMetadata: metadata,
        } satisfies TurnResult;
      }

      return this.createSuccessResult(
        conversationMessages,
        tokens,
        latencyMs,
        hasNewToolCalls,
        finalAnswer,
        response,
        { hasReasoning, hasContent: hasAssistantText, stopReason },
        metadata
      );
    } catch (error) {
      return this.createFailureResult(request, this.mapError(error), Date.now() - startTime);
    }
  }

  // Abstract method that each provider must implement to convert response messages
  protected abstract convertResponseMessages(
    messages: ResponseMessage[], 
    provider: string, 
    model: string, 
    tokens: TokenUsage
  ): ConversationMessage[];

  /**
   * Generic converter that preserves ALL tool results, even when providers bundle
   * multiple tool-result parts into a single message. Providers can call this
   * to avoid duplicating splitting logic.
   */
  protected convertResponseMessagesGeneric(
    messages: ResponseMessage[],
    provider: string,
    model: string,
    tokens: TokenUsage
  ): ConversationMessage[] {
    const out: ConversationMessage[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const m of messages) {
      if (m.role === 'tool' && Array.isArray(m.content)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
          const p = part as { type?: string; toolCallId?: string; toolName?: string; output?: unknown; error?: unknown };
          {
            const t = (p.type ?? '').replace('_','-');
            if ((t === this.TOOL_RESULT_TYPE || t === this.TOOL_ERROR_TYPE) && typeof p.toolCallId === 'string') {
            let text = '';
            
            // Handle tool errors - ensure we always have valid output
            if (t === this.TOOL_ERROR_TYPE && p.error !== undefined) {
              if (typeof p.error === 'string') {
                text = `${TOOL_FAILED_PREFIX}${p.error}${TOOL_FAILED_SUFFIX}`;
              } else if (typeof p.error === 'object' && p.error !== null) {
                const e = p.error as { message?: string; error?: string };
                text = `${TOOL_FAILED_PREFIX}${e.message ?? e.error ?? JSON.stringify(p.error)}${TOOL_FAILED_SUFFIX}`;
              } else {
                text = `${TOOL_FAILED_PREFIX}${JSON.stringify(p.error)}${TOOL_FAILED_SUFFIX}`;
              }
            } 
            // Handle tool results - ensure we always have valid output
            else {
              const outObj = p.output as { type?: string; value?: unknown } | undefined;
              if (outObj !== undefined) {
                if (outObj.type === 'text' && typeof outObj.value === 'string') text = outObj.value;
                else if (outObj.type === 'json') text = JSON.stringify(outObj.value);
                else if (typeof (outObj as unknown as string) === 'string') text = outObj as unknown as string;
              }
              // Ensure we always have valid output
              if (!text || text.trim().length === 0) {
                text = NO_TOOL_OUTPUT;
              }
            }
            
            out.push({
              role: 'tool',
              content: text,
              toolCallId: p.toolCallId,
              metadata: {
                provider,
                model,
                tokens: {
                  inputTokens: tokens.inputTokens,
                  outputTokens: tokens.outputTokens,
                  cachedTokens: tokens.cachedTokens,
                  totalTokens: tokens.totalTokens,
                },
                timestamp: Date.now(),
              },
            });
          }
          }
        }
        continue;
      }

      if (m.role === 'assistant' && Array.isArray(m.content)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
          const p = part as { type?: string; toolCallId?: string; output?: unknown; error?: unknown };
          {
            const t = (p.type ?? '').replace('_','-');
            if ((t === this.TOOL_RESULT_TYPE || t === this.TOOL_ERROR_TYPE) && typeof p.toolCallId === 'string') {
            let text = '';
            
            // Handle tool errors - ensure we always have valid output
            if (t === this.TOOL_ERROR_TYPE && p.error !== undefined) {
              if (typeof p.error === 'string') {
                text = `${TOOL_FAILED_PREFIX}${p.error}${TOOL_FAILED_SUFFIX}`;
              } else if (typeof p.error === 'object' && p.error !== null) {
                const e = p.error as { message?: string; error?: string };
                text = `${TOOL_FAILED_PREFIX}${e.message ?? e.error ?? JSON.stringify(p.error)}${TOOL_FAILED_SUFFIX}`;
              } else {
                text = `${TOOL_FAILED_PREFIX}${JSON.stringify(p.error)}${TOOL_FAILED_SUFFIX}`;
              }
            } 
            // Handle tool results - ensure we always have valid output
            else {
              const outObj = p.output as { type?: string; value?: unknown } | undefined;
              if (outObj !== undefined) {
                if (outObj.type === 'text' && typeof outObj.value === 'string') text = outObj.value;
                else if (outObj.type === 'json') text = JSON.stringify(outObj.value);
                else if (typeof (outObj as unknown as string) === 'string') text = outObj as unknown as string;
              }
              // Ensure we always have valid output
                if (!text || text.trim().length === 0) {
                  text = NO_TOOL_OUTPUT;
                }
            }
            
            out.push({
              role: 'tool',
              content: text,
              toolCallId: p.toolCallId,
              metadata: {
                provider,
                model,
                tokens: {
                  inputTokens: tokens.inputTokens,
                  outputTokens: tokens.outputTokens,
                  cachedTokens: tokens.cachedTokens,
                  totalTokens: tokens.totalTokens,
                },
                timestamp: Date.now(),
              },
            });
          }
          }
        }
      }

      out.push(this.parseAISDKMessage(m, provider, model, tokens));
    }
    return out;
  }

  /**
   * Inject failure tool_result messages for tool calls that weren't executed.
   * This handles the case where a model emits tool_use for a tool not in the SDK's ToolSet
   * (e.g., agent__final_report which is filtered out). Without a matching tool_result,
   * providers like Anthropic reject the next request.
   */
  protected injectMissingToolResults(
    messages: ConversationMessage[],
    provider: string,
    model: string,
    tokens: TokenUsage
  ): ConversationMessage[] {
    const normalizeTool = (n: string): string => n.replace(/^<\|[^|]+\|>/, '').trim();
    // Collect all tool call IDs from assistant messages
    const toolCallIds = new Set<string>();
    const toolCallNames = new Map<string, string>();
    // eslint-disable-next-line functional/no-loop-statements
    for (const msg of messages) {
      if (msg.role === 'assistant' && Array.isArray(msg.toolCalls)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const tc of msg.toolCalls) {
          if (typeof tc.id === 'string' && tc.id.length > 0) {
            toolCallIds.add(tc.id);
            toolCallNames.set(tc.id, tc.name);
          }
        }
      }
    }

    // Collect all tool result IDs from tool messages
    const toolResultIds = new Set<string>();
    // eslint-disable-next-line functional/no-loop-statements
    for (const msg of messages) {
      if (msg.role === 'tool' && typeof msg.toolCallId === 'string' && msg.toolCallId.length > 0) {
        toolResultIds.add(msg.toolCallId);
      }
    }

    // Find orphaned tool calls (have tool_use but no tool_result)
    const orphanedIds: string[] = [];
    toolCallIds.forEach((id) => {
      if (!toolResultIds.has(id)) {
        orphanedIds.push(id);
      }
    });

    if (orphanedIds.length === 0) {
      return messages;
    }

    // Inject failure results for orphaned tool calls
    const injected: ConversationMessage[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const id of orphanedIds) {
      const toolName = toolCallNames.get(id) ?? 'unknown';
      const normalizedName = normalizeTool(toolName);
      const injectedReason = isXmlFinalReportTagName(normalizedName)
        ? 'xml_wrapper'
        : 'tool_missing';
      const errorMessage = injectedReason === 'xml_wrapper'
        ? XML_WRAPPER_CALLED_AS_TOOL_RESULT
        : `Tool not available: ${toolName}`;
      injected.push({
        role: 'tool',
        content: `${TOOL_FAILED_PREFIX}${errorMessage}${TOOL_FAILED_SUFFIX}`,
        toolCallId: id,
        metadata: {
          provider,
          model,
          tokens: {
            inputTokens: tokens.inputTokens,
            outputTokens: tokens.outputTokens,
            cachedTokens: tokens.cachedTokens,
            totalTokens: tokens.totalTokens,
          },
          timestamp: Date.now(),
          injectedToolResult: true,
          injectedReason,
        },
      });
    }

    // Insert injected messages after the last assistant message
    // (tool results must follow assistant message with tool_use)
    const result = [...messages];
    let lastAssistantIdx = -1;
    // eslint-disable-next-line functional/no-loop-statements
    for (let i = result.length - 1; i >= 0; i--) {
      if (result[i].role === 'assistant') {
        lastAssistantIdx = i;
        break;
      }
    }

    if (lastAssistantIdx >= 0) {
      // Insert after last assistant message and any existing tool messages
      let insertIdx = lastAssistantIdx + 1;
      // eslint-disable-next-line functional/no-loop-statements
      while (insertIdx < result.length && result[insertIdx].role === 'tool') {
        insertIdx++;
      }
      result.splice(insertIdx, 0, ...injected);
    } else {
      // No assistant message found (shouldn't happen), append at end
      result.push(...injected);
    }

    return result;
  }

  /**
   * Convert conversation messages to AI SDK format
   * Preserves tool messages which are critical for multi-turn tool conversations
   */
  protected convertMessages(messages: ConversationMessage[]): ModelMessage[] {
    // Build a lookup from toolCallId -> toolName using assistant messages
    const callIdToName = new Map<string, string>();
    // eslint-disable-next-line functional/no-loop-statements
    for (const msg of messages) {
      if (msg.role === 'assistant' && Array.isArray(msg.toolCalls)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const tc of msg.toolCalls) {
          if (tc.id) callIdToName.set(tc.id, tc.name);
        }
      }
    }

    const modelMessages: ModelMessage[] = [];
     
    // eslint-disable-next-line functional/no-loop-statements
    for (const m of messages) {
      if (m.role === 'system') {
        modelMessages.push({ role: 'system', content: m.content });
        continue;
      }
      if (m.role === 'user') {
        modelMessages.push({ role: 'user', content: m.content });
        continue;
      }
      if (m.role === 'assistant') {
        const reasoningFromMessage = this.collectReasoningSegmentsFromMessage(m);
        const contentReasoningSegments: ReasoningOutput[] = [];
        const hasToolCalls = Array.isArray(m.toolCalls) && m.toolCalls.length > 0;
        const parts: (
          { type: 'reasoning'; text: string; providerOptions?: Record<string, unknown> } |
          { type: 'text'; text: string } |
          { type: 'tool-call'; toolCallId: string; toolName: string; input: unknown }
        )[] = [];

        if (Array.isArray(m.content)) {
          // eslint-disable-next-line functional/no-loop-statements
          for (const part of m.content) {
            const entry = part as { type?: string; text?: string; toolCallId?: string; toolName?: string; input?: unknown; output?: unknown; error?: unknown };
            const normalizedType = (entry.type ?? '').replace('_', '-');
            if (normalizedType === 'text' || entry.type === undefined) {
              if (typeof entry.text === 'string' && entry.text.trim().length > 0) {
                parts.push({ type: 'text', text: entry.text });
              }
              continue;
            }
            if (normalizedType === 'tool-call' && entry.toolCallId !== undefined && entry.toolName !== undefined) {
              parts.push({ type: 'tool-call', toolCallId: entry.toolCallId, toolName: entry.toolName, input: entry.input ?? {} });
              continue;
            }
            const extractedReasoning = this.collectReasoningOutputs(entry);
            if (extractedReasoning.length > 0) {
              extractedReasoning.forEach((segment) => contentReasoningSegments.push(segment));
              continue;
            }
            if ((normalizedType === this.TOOL_RESULT_TYPE || normalizedType === this.TOOL_ERROR_TYPE) && entry.toolCallId !== undefined) {
              let text = '';
              if (normalizedType === this.TOOL_ERROR_TYPE && entry.error !== undefined) {
                if (typeof entry.error === 'string') {
              text = `${TOOL_FAILED_PREFIX}${entry.error}${TOOL_FAILED_SUFFIX}`;
                } else if (entry.error !== null && typeof entry.error === 'object') {
                  const errObj = entry.error as { message?: string; error?: string };
              text = `${TOOL_FAILED_PREFIX}${errObj.message ?? errObj.error ?? JSON.stringify(entry.error)}${TOOL_FAILED_SUFFIX}`;
                } else {
              text = `${TOOL_FAILED_PREFIX}${JSON.stringify(entry.error)}${TOOL_FAILED_SUFFIX}`;
                }
              } else {
                const outObj = entry.output as { type?: string; value?: unknown } | undefined;
                if (outObj !== undefined) {
                  if (outObj.type === 'text' && typeof outObj.value === 'string') text = outObj.value;
                  else if (outObj.type === 'json') text = JSON.stringify(outObj.value);
                  else if (typeof (outObj as unknown as string) === 'string') text = outObj as unknown as string;
                }
                if (!text || text.trim().length === 0) {
                  text = NO_TOOL_OUTPUT;
                }
              }
              parts.push({ type: 'text', text });
              continue;
            }
          }
        } else if (typeof m.content === 'string' && m.content.trim().length > 0) {
          parts.push({ type: 'text', text: m.content });
        }

        if (hasToolCalls) {
          // eslint-disable-next-line functional/no-loop-statements
          for (const tc of m.toolCalls ?? []) {
            parts.push({ type: 'tool-call', toolCallId: tc.id, toolName: tc.name, input: tc.parameters });
          }
        }

        const combinedReasoning = this.mergeReasoningOutputs(reasoningFromMessage, contentReasoningSegments);
        const metadataProvider = m.metadata?.provider;
        const providerSupportsReasoningReplay = this.supportsReasoningReplay()
          || metadataProvider === 'anthropic';
        const reasoningSegmentsToInclude = providerSupportsReasoningReplay
          ? combinedReasoning.filter((segment) => segment.providerMetadata !== undefined)
          : [];
        const shouldIncludeReasoning = reasoningSegmentsToInclude.length > 0;

        let finalParts = parts;
        if (shouldIncludeReasoning) {
          const reasoningParts = reasoningSegmentsToInclude.map((segment) => {
            const providerOptions = segment.providerMetadata !== undefined
              ? this.toProviderOptions(segment.providerMetadata)
              : undefined;
            const basePart: {
              type: 'reasoning';
              text: string;
              providerOptions?: Record<string, unknown>;
              signature?: string;
            } = {
              type: 'reasoning' as const,
              text: segment.text,
              ...(providerOptions !== undefined ? { providerOptions } : {}),
            };
            const signature = (segment as unknown as { signature?: unknown }).signature;
            if (typeof signature === 'string') {
              basePart.signature = signature;
            }
            return basePart;
          });
          finalParts = [...reasoningParts, ...parts];
        }

        if (finalParts.length === 0) {
          modelMessages.push({ role: 'assistant', content: '' });
        } else {
          modelMessages.push({ role: 'assistant', content: finalParts as never });
        }
        continue;
      }
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      if (m.role === 'tool') {
        // Handle tool messages
        const toolCallId = m.toolCallId ?? '';
        const toolName = callIdToName.get(toolCallId) ?? 'unknown';
        modelMessages.push({
          role: 'tool',
          content: [
            {
              type: this.TOOL_RESULT_TYPE,
              toolCallId,
              toolName,
              output: { type: 'text', value: m.content }
            }
          ] as never
        });
        continue;
      }
      // Skip any unknown message types
      // This could happen if we get unexpected roles from the conversation history
    }

    return modelMessages;
  }

  protected supportsReasoningReplay(): boolean {
    return false;
  }

  /**
   * Helper method to parse AI SDK response messages which embed tool calls in content array
   */
  protected parseAISDKMessage(m: ResponseMessage, provider: string, model: string, tokens: Partial<TokenUsage>): ConversationMessage {
    // Extract tool calls from content array (AI SDK format)
    let toolCallsFromContent: ConversationMessage['toolCalls'] = undefined;
    let textContent = '';
    let toolCallId: string | undefined;
    

    const toSafeToolName = (value: unknown): string => {
      const rawName = typeof value === 'string' ? value : '';
      const sanitized = sanitizeToolName(rawName);
      const { name, truncated } = clampToolName(sanitized);
      if (truncated) {
        const previewLength = Math.min(sanitized.length, 64);
        const preview = sanitized.slice(0, previewLength);
        const suffix = sanitized.length > previewLength ? '…' : '';
        warn(`Truncated tool name '${preview}${suffix}' (length ${String(sanitized.length)}) to ${String(TOOL_NAME_MAX_LENGTH)} characters before storing in history.`);
      }
      return name;
    };
    const toParameters = (
      candidate: unknown,
      context: { toolName: string; toolCallId?: string; source: 'content' | 'legacy' }
    ): Record<string, unknown> => {
      const parsed = parseJsonRecord(candidate);
      if (parsed !== undefined) {
        return parsed;
      }
      if (candidate !== undefined) {
        this.recordParameterWarning({
          toolCallId: context.toolCallId,
          toolName: context.toolName,
          source: context.source,
          reason: 'invalid_json_parameters',
          rawInput: candidate,
        });
        return candidate as Record<string, unknown>;
      }
      this.recordParameterWarning({
        toolCallId: context.toolCallId,
        toolName: context.toolName,
        source: context.source,
        reason: 'missing_parameters',
      });
      return {};
    };
    const reasoningSegments: ReasoningOutput[] = [...this.collectReasoningSegmentsFromMessage(m)];

    if (Array.isArray(m.content)) {
      const textParts: string[] = [];
      const toolCalls: { id: string; name: string; parameters: Record<string, unknown> }[] = [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const part of m.content) {
        if (part.type === 'text' || part.type === undefined) {
          if (part.text !== undefined && part.text !== '') {
            textParts.push(part.text);
          }
          continue;
        }
        const extractedReasoning = this.collectReasoningOutputs(part);
        if (extractedReasoning.length > 0) {
          extractedReasoning.forEach((segment) => reasoningSegments.push(segment));
          continue;
        }
        if (part.type === 'tool-call' && part.toolCallId !== undefined && part.toolName !== undefined) {
          // AI SDK embeds tool calls in content
          const safeName = toSafeToolName(part.toolName);
          toolCalls.push({
            id: part.toolCallId,
            name: safeName,
            parameters: toParameters(part.input, { toolName: safeName, toolCallId: part.toolCallId, source: 'content' }),
          });
          continue;
        }
        if (part.type === this.TOOL_RESULT_TYPE && part.toolCallId !== undefined) {
          // This is a tool result message
          toolCallId = part.toolCallId;
          // Tool results have their output in a nested structure
          if (part.output !== undefined && part.output !== null && typeof part.output === 'object' && 'value' in part.output) {
            textParts.push(String(part.output.value));
          }
        }
      }
      
      textContent = textParts.join('');
      if (toolCalls.length > 0) {
        toolCallsFromContent = toolCalls;
      }
    } else if (typeof m.content === 'string') {
      textContent = m.content;
    }
    
    // Prefer tool calls from content array (AI SDK format) over legacy toolCalls field
    const finalToolCalls = toolCallsFromContent ?? (Array.isArray(m.toolCalls) ? m.toolCalls.map(tc => {
      const rawName = tc.name ?? tc.function?.name ?? '';
      const safeName = toSafeToolName(rawName);
      return {
        id: tc.id ?? tc.toolCallId ?? '',
        name: safeName,
        parameters: toParameters(tc.arguments ?? tc.function?.arguments, {
          toolName: safeName,
          toolCallId: tc.id ?? tc.toolCallId,
          source: 'legacy',
        })
      };
    }) : undefined);
    
    const mergedReasoning = this.mergeReasoningOutputs(reasoningSegments);
    const reasoning = mergedReasoning.length > 0 ? mergedReasoning : undefined;

    return {
      role: (m.role ?? 'assistant') as ConversationMessage['role'],
      content: textContent,
      toolCalls: finalToolCalls,
      toolCallId: toolCallId ?? ('toolCallId' in m ? (m as { toolCallId?: string }).toolCallId : undefined),
      reasoning,
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
    };
  }
}

// Common response message interface for providers
export interface ResponseMessage {
  role?: string;
  content?: string | { 
    type?: string; 
    text?: string;
    providerMetadata?: Record<string, unknown>;
    // AI SDK embeds tool calls and results in content array
    toolCallId?: string;
    toolName?: string;
    input?: unknown;
    output?: unknown;
    value?: string;
  }[];
  toolCalls?: {
    id?: string;
    name?: string;
    arguments?: unknown;
    function?: { name?: string; arguments?: unknown };
    toolCallId?: string;
  }[];
}
