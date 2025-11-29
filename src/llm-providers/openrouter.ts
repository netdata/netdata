import { createOpenRouter } from '@openrouter/ai-sdk-provider';

import type { ProviderTurnMetadata, TurnRequest, TurnResult, ProviderConfig, ConversationMessage, TurnStatus, TurnRetryDirective } from '../types.js';
import type { LanguageModel } from 'ai';

import { warn } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenRouterProvider extends BaseLLMProvider {
  name = 'openrouter';
  private static readonly HOST = 'openrouter.ai';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(
    config: ProviderConfig,
    tracedFetch?: typeof fetch
  ) {
    super({
      formatPolicy: { allowed: config.stringSchemaFormatsAllowed, denied: config.stringSchemaFormatsDenied },
      reasoningDefaults: {
        minimal: 'minimal',
        low: 'low',
        medium: 'medium',
        high: 'high',
      },
      reasoningLimits: { min: 1024, max: 32_000 },
    });
    this.config = config;
    const prov = createOpenRouter({
      apiKey: config.apiKey,
      fetch: tracedFetch,
      headers: {
        'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
        'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent',
        'User-Agent': 'ai-agent/1.0',
        ...(config.headers ?? {}),
      },
    });
    
    this.provider = (model: string) => prov(model);

  }

  public override prepareFetch(details: { url: string; init: RequestInit }): { headers?: Record<string, string> } | undefined {
    if (!details.url.includes(OpenRouterProvider.HOST)) return undefined;
    const defaults = {
      'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
      'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent',
      'User-Agent': 'ai-agent/1.0',
    } as const;
    return { headers: defaults };
  }

  public override getResponseMetadataCollector(): ((payload: { url: string; response: Response }) => Promise<ProviderTurnMetadata | undefined>) | undefined {
    return async ({ url, response }) => {
      return await this.handleMetadataCapture(url, response);
    };
  }

  protected override buildRetryDirective(request: TurnRequest, status: TurnStatus): TurnRetryDirective | undefined {
    if (status.type === 'rate_limit') {
      const wait = typeof status.retryAfterMs === 'number' && Number.isFinite(status.retryAfterMs) ? status.retryAfterMs : undefined;
      const providerHint = `${request.provider}:${request.model}`;
      return {
        action: 'retry',
        backoffMs: wait,
        logMessage: `OpenRouter rate limit; backing off ${wait !== undefined ? `${String(wait)}ms` : 'briefly'} before retry.${(status.sources ?? []).length > 0 ? ` Sources: ${(status.sources ?? []).join(' | ')}` : ''}`.trim(),
        systemMessage: `OpenRouter rate limit for ${providerHint}; retrying.`,
        sources: status.sources,
      };
    }
    return super.buildRetryDirective(request, status);
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

      const resolvedToolChoice = this.resolveToolChoice(request);
      const openaiOpts: { toolChoice?: 'auto' | 'required'; maxTokens?: number; frequencyPenalty?: number } = {};
      if (resolvedToolChoice !== undefined) openaiOpts.toolChoice = resolvedToolChoice;
      if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) openaiOpts.maxTokens = Math.trunc(request.maxOutputTokens);
      if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) openaiOpts.frequencyPenalty = request.repeatPenalty;
      const baseProviderOptions: Record<string, unknown> = { openrouter: { usage: { include: true } }, openai: openaiOpts };
      const customProviderOptions = this.getProviderOptions();
      let providerOptions: Record<string, unknown> = this.deepMerge(baseProviderOptions, customProviderOptions);
      const routerProv = this.getRouterProviderConfig();
      // Merge router provider preferences into openrouter settings
      const ex = providerOptions.openrouter
      const curr = (ex !== null && ex !== undefined && typeof ex === 'object' && !Array.isArray(ex)) ? (ex as Record<string, unknown>) : {};
      providerOptions.openrouter = { ...curr, provider: routerProv };

      const routerOptions = providerOptions.openrouter as Record<string, unknown> | undefined;
      if (routerOptions !== undefined && request.reasoningValue !== undefined) {
        if (request.reasoningValue === null) {
          delete routerOptions.reasoning;
        } else if (typeof request.reasoningValue === 'string') {
          routerOptions.reasoning = { effort: request.reasoningValue };
        } else if (typeof request.reasoningValue === 'number' && Number.isFinite(request.reasoningValue)) {
          routerOptions.reasoning = { max_tokens: Math.trunc(request.reasoningValue) };
        }
      }

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(request, this.mapError(error), Date.now() - startTime);
    }
  }


  protected convertResponseMessages(
    messages: ResponseMessage[],
    provider: string,
    model: string,
    tokens: { inputTokens?: number; outputTokens?: number; cachedTokens?: number; totalTokens?: number }
  ): ConversationMessage[] {
    const out: ConversationMessage[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const m of messages) {
      // Split multiple tool results bundled into a single 'tool' role message
      if (m.role === 'tool' && Array.isArray(m.content)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
          const p = part as { type?: string; toolCallId?: string; toolName?: string; output?: unknown };
          if (p.type === 'tool-result' && typeof p.toolCallId === 'string') {
            // Extract string payload from output (AI SDK wraps as { type, value })
            let text = '';
            const outObj = p.output as { type?: string; value?: unknown } | undefined;
            if (outObj !== undefined) {
              if (outObj.type === 'text' && typeof outObj.value === 'string') text = outObj.value;
              else if (outObj.type === 'json') text = JSON.stringify(outObj.value);
              else if (typeof (outObj as unknown as string) === 'string') text = outObj as unknown as string;
            }
            out.push({
              role: 'tool',
              content: text,
              toolCallId: p.toolCallId,
              metadata: {
                provider,
                model,
                tokens: {
                  inputTokens: tokens.inputTokens ?? 0,
                  outputTokens: tokens.outputTokens ?? 0,
                  cachedTokens: tokens.cachedTokens,
                  totalTokens: tokens.totalTokens ?? 0,
                },
                timestamp: Date.now(),
              },
            });
          }
        }
        continue;
      }

      // Also handle tool-result parts accidentally embedded in assistant content (defensive)
      if (m.role === 'assistant' && Array.isArray(m.content)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const part of m.content) {
          const p = part as { type?: string; toolCallId?: string; output?: unknown };
          if (p.type === 'tool-result' && typeof p.toolCallId === 'string') {
            let text = '';
            const outObj = p.output as { type?: string; value?: unknown } | undefined;
            if (outObj !== undefined) {
              if (outObj.type === 'text' && typeof outObj.value === 'string') text = outObj.value;
              else if (outObj.type === 'json') text = JSON.stringify(outObj.value);
              else if (typeof (outObj as unknown as string) === 'string') text = outObj as unknown as string;
            }
            out.push({
              role: 'tool',
              content: text,
              toolCallId: p.toolCallId,
              metadata: {
                provider,
                model,
                tokens: {
                  inputTokens: tokens.inputTokens ?? 0,
                  outputTokens: tokens.outputTokens ?? 0,
                  cachedTokens: tokens.cachedTokens,
                  totalTokens: tokens.totalTokens ?? 0,
                },
                timestamp: Date.now(),
              },
            });
          }
        }
        // Continue to also parse the assistant message normally (to retain toolCalls, etc.)
      }

      out.push(this.parseAISDKMessage(m, provider, model, tokens));
    }
    return out;
  }

  protected override extractTurnMetadata(
    request: TurnRequest,
    context: { usage?: unknown; response?: unknown; latencyMs: number }
  ): ProviderTurnMetadata | undefined {
    let metadata = this.mergeProviderMetadata(
      this.deriveContextMetadata(request, context),
      this.consumeQueuedProviderMetadata()
    );

    const usageRecord = this.extractUsageRecord(context.usage);
    const cacheWrite = this.extractCacheWriteTokens(usageRecord);
    if (cacheWrite !== undefined && (metadata?.cacheWriteInputTokens ?? undefined) === undefined) {
      metadata = this.mergeProviderMetadata(metadata, { cacheWriteInputTokens: cacheWrite });
    }
    const costs = this.extractUsageCosts(usageRecord);
    if (costs.costUsd !== undefined && (metadata?.reportedCostUsd ?? undefined) === undefined) {
      metadata = this.mergeProviderMetadata(metadata, { reportedCostUsd: costs.costUsd });
    }
    if (costs.upstreamCostUsd !== undefined && (metadata?.upstreamCostUsd ?? undefined) === undefined) {
      metadata = this.mergeProviderMetadata(metadata, { upstreamCostUsd: costs.upstreamCostUsd });
    }

    return metadata;
  }

  private async handleMetadataCapture(url: string, response: Response): Promise<ProviderTurnMetadata | undefined> {
    if (!url.includes(OpenRouterProvider.HOST)) {
      return undefined;
    }
    const contentTypeHeader = response.headers.get('content-type') ?? '';
    const contentType = contentTypeHeader.toLowerCase();
    let metadata: ProviderTurnMetadata | undefined;

    try {
      if (contentType.includes('application/json')) {
        const parsed = await this.parseJsonResponse(response.clone());
        if (parsed !== undefined) {
          metadata = this.mergeProviderMetadata(metadata, this.metadataFromRecord(parsed));
        }
      } else if (contentType.includes('text/event-stream')) {
        const partial = await this.parseOpenRouterSseStream(response.clone());
        metadata = this.mergeProviderMetadata(metadata, partial);
      }
    } catch (error) {
      try {
        warn(`openrouter metadata capture failed: ${error instanceof Error ? error.message : String(error)}`);
      } catch {
        /* ignore logging failures */
      }
    }

    if (metadata !== undefined) {
      this.enqueueProviderMetadata(metadata);
    }
    return metadata !== undefined ? { ...metadata } : undefined;
  }

  private metadataFromRecord(record: Record<string, unknown>): ProviderTurnMetadata {
    const metadata: ProviderTurnMetadata = {};
    const routing = this.extractOpenRouterRouting(record);
    if (routing.provider !== undefined) metadata.actualProvider = routing.provider;
    if (routing.model !== undefined) metadata.actualModel = routing.model;
    const usage = this.extractUsageRecord(record);
    const costs = this.extractUsageCosts(usage);
    if (costs.costUsd !== undefined) metadata.reportedCostUsd = costs.costUsd;
    if (costs.upstreamCostUsd !== undefined) metadata.upstreamCostUsd = costs.upstreamCostUsd;
    const cacheWrite = this.extractCacheWriteTokens(usage);
    if (cacheWrite !== undefined) metadata.cacheWriteInputTokens = cacheWrite;
    return metadata;
  }

  private isArray(val: unknown): val is unknown[] {
    return Array.isArray(val);
  }

  // Read per-provider providerOptions from config.custom
  private getRouterProviderConfig(): Record<string, unknown> {
    try {
      const raw = this.config.custom;
      if (this.isPlainObject(raw)) {
        const providerConfig = raw.provider;
        if (this.isPlainObject(providerConfig)) return providerConfig;
      }
    } catch (e) { try { warn(`openrouter provider cleanup failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
    return {};
  }
  

  // Read per-provider providerOptions from config.custom
  private getProviderOptions(): Record<string, unknown> {
    try {
      const raw = this.config.custom;
      if (this.isPlainObject(raw)) {
        const providerOptions = raw.providerOptions;
        if (this.isPlainObject(providerOptions)) return providerOptions;
      }
      return {};
    } catch {
      return {};
    }
  }

  // Lightweight deep merge for plain objects
  private deepMerge(target: Record<string, unknown>, source: Record<string, unknown>): Record<string, unknown> {
    const out: Record<string, unknown> = { ...target };
    // eslint-disable-next-line functional/no-loop-statements
    for (const [k, v] of Object.entries(source)) {
      const tv = out[k];
      if (this.isPlainObject(v) && this.isPlainObject(tv)) {
        out[k] = this.deepMerge(tv, v);
      } else {
        out[k] = v;
      }
    }
    return out;
  }

  private extractUsageRecord(record: unknown): Record<string, unknown> | undefined {
    if (!this.isPlainObject(record)) {
      return undefined;
    }
    if (this.isPlainObject(record.usage)) {
      return record.usage;
    }
    if (this.isPlainObject(record.data) && this.isPlainObject(record.data.usage)) {
      return record.data.usage;
    }
    return undefined;
  }

  private extractUsageCosts(usage?: Record<string, unknown>): { costUsd?: number; upstreamCostUsd?: number } {
    if (!this.isPlainObject(usage)) {
      return {};
    }
    const cost = this.toNumber(usage.cost);
    const detailsCandidate = usage.cost_details;
    const details = this.isPlainObject(detailsCandidate) ? detailsCandidate : undefined;
    const upstream = this.toNumber(details?.upstream_inference_cost);
    return { costUsd: cost, upstreamCostUsd: upstream };
  }

  private extractCacheWriteTokens(usage?: Record<string, unknown>): number | undefined {
    if (!this.isPlainObject(usage)) {
      return undefined;
    }
    const directKeys = [
      'cacheWriteInputTokens',
      'cacheCreationInputTokens',
      'cache_creation_input_tokens',
      'cache_write_input_tokens'
    ];
    // eslint-disable-next-line functional/no-loop-statements
    for (const key of directKeys) {
      const value = this.toNumber(usage[key]);
      if (value !== undefined && value > 0) {
        return value;
      }
    }
    const nestedKeys = ['cacheCreation', 'cache_creation'];
    // eslint-disable-next-line functional/no-loop-statements
    for (const key of nestedKeys) {
      const nested = usage[key];
      if (this.isPlainObject(nested)) {
        const value = this.toNumber(nested.ephemeral_5m_input_tokens);
        if (value !== undefined && value > 0) {
          return value;
        }
      }
    }
    return undefined;
  }

  private extractOpenRouterRouting(record: Record<string, unknown>): { provider?: string; model?: string } {
    const targetCandidate = record.data;
    const target = this.isPlainObject(targetCandidate) ? targetCandidate : record;
    let provider = typeof target.provider === 'string' && target.provider.length > 0 ? target.provider : undefined;
    let model = typeof target.model === 'string' && target.model.length > 0 ? target.model : undefined;
    const choices = this.isArray(target.choices) ? target.choices : undefined;
    if (choices !== undefined && choices.length > 0) {
      const choice = choices[0];
      if (this.isPlainObject(choice)) {
        if (typeof choice.provider === 'string' && choice.provider.length > 0) {
          provider = choice.provider;
          model ??= provider;
        }
        if (typeof choice.model === 'string' && choice.model.length > 0) {
          model = choice.model;
        }
      }
    }
    return { provider, model };
  }

  private async parseJsonResponse(response: Response): Promise<Record<string, unknown> | undefined> {
    try {
      const text = await response.text();
      const parsed = this.parseJsonString(text);
      return parsed;
    } catch {
      return undefined;
    }
  }

  private parseJsonString(payload: string): Record<string, unknown> | undefined {
    try {
      const parsed: unknown = JSON.parse(payload);
      return this.isPlainObject(parsed) ? parsed : undefined;
    } catch {
      return undefined;
    }
  }

  private async parseOpenRouterSseStream(response: Response): Promise<ProviderTurnMetadata | undefined> {
    try {
      const text = await response.text();
      const lines = text.split('\n');
      let metadata: ProviderTurnMetadata | undefined;
      const apply = (partial?: ProviderTurnMetadata) => {
        metadata = this.mergeProviderMetadata(metadata, partial);
      };
      // eslint-disable-next-line functional/no-loop-statements
      for (const rawLine of lines) {
        const line = rawLine.trim();
        if (!line.startsWith('data:')) continue;
        const payload = line.slice(5).trim();
        if (payload.length === 0 || payload === '[DONE]') continue;
        const parsed = this.parseJsonString(payload);
        if (parsed === undefined) continue;
        const routing = this.extractOpenRouterRouting(parsed);
        const usage = this.extractUsageRecord(parsed);
        const costs = this.extractUsageCosts(usage);
        const cacheWrite = this.extractCacheWriteTokens(usage);
        apply({
          actualProvider: routing.provider,
          actualModel: routing.model,
          reportedCostUsd: costs.costUsd,
          upstreamCostUsd: costs.upstreamCostUsd,
          cacheWriteInputTokens: cacheWrite,
        });
      }
      return metadata;
    } catch {
      return undefined;
    }
  }

  private toNumber(value: unknown): number | undefined {
    if (typeof value === 'number' && Number.isFinite(value)) return value;
    if (typeof value === 'string' && value.trim().length > 0) {
      const parsed = Number(value);
      return Number.isFinite(parsed) ? parsed : undefined;
    }
    return undefined;
  }

}
