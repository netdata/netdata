import { createOpenRouter } from '@openrouter/ai-sdk-provider';

import type { TurnRequest, TurnResult, ProviderConfig, ConversationMessage } from '../types.js';
import type { LanguageModel } from 'ai';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

export class OpenRouterProvider extends BaseLLMProvider {
  name = 'openrouter';
  private provider: (model: string) => LanguageModel;
  private config: ProviderConfig;

  constructor(config: ProviderConfig, tracedFetch?: typeof fetch) {
    super();
    this.config = config;
    const prov = createOpenRouter({
      apiKey: config.apiKey,
      fetch: tracedFetch,
      headers: {
        'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
        'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-claude',
        'User-Agent': 'ai-agent-claude/1.0',
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

      const providerOptions = request.parallelToolCalls !== undefined
        ? { openai: { parallelToolCalls: request.parallelToolCalls, toolChoice: 'required' }, openrouter: { usage: { include: true } } }
        : { openai: { toolChoice: 'required' }, openrouter: { usage: { include: true } } };

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
}
