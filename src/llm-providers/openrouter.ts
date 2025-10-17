import { createOpenRouter } from '@openrouter/ai-sdk-provider';

import type { LanguageModel } from 'ai';
import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage } from '../types.js';

import { warn } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenRouterProvider extends BaseLLMProvider {
  name = 'openrouter';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super({ formatPolicy: { allowed: config.stringSchemaFormatsAllowed, denied: config.stringSchemaFormatsDenied } });
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

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const startTime = Date.now();
    
    try {
      const model = this.provider(request.model);
      const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn);
      const tools = this.convertTools(filteredTools, request.toolExecutor);
      const messages = super.convertMessages(request.messages);
      
      // Add final turn message if needed
      const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn);

      const openaiOpts: { toolChoice: 'required'; parallelToolCalls?: boolean; maxTokens?: number; frequencyPenalty?: number } = { toolChoice: 'required' };
      if (request.parallelToolCalls !== undefined) openaiOpts.parallelToolCalls = request.parallelToolCalls;
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

      if (request.stream === true) {
        return await super.executeStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      } else {
        return await super.executeNonStreamingTurn(model, finalMessages, tools, request, startTime, providerOptions);
      }
    } catch (error) {
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
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

  private isPlainObject(val: unknown): val is Record<string, unknown> {
    return val !== null && val !== undefined && typeof val === "object" && !Array.isArray(val);
  }

  // Read per-provider providerOptions from config.custom
  private getRouterProviderConfig(): Record<string, unknown> {
    try {
      const raw = this.config.custom;
      if (this.isPlainObject(raw)) {
        const v = (raw as { provider?: unknown }).provider;
        if (this.isPlainObject(v)) return v;
      }
    } catch (e) { try { warn(`openrouter provider cleanup failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
    return {};
  }
  

  // Read per-provider providerOptions from config.custom
  private getProviderOptions(): Record<string, unknown> {
    try {
      const raw = this.config.custom;
      if (this.isPlainObject(raw)) {
        const v = (raw as { providerOptions?: unknown }).providerOptions;
        if (this.isPlainObject(v)) return v;
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

}
