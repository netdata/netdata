import { jsonSchema } from '@ai-sdk/provider-utils';
import { generateText, streamText } from 'ai';

import type { ConversationMessage, MCPTool, TokenUsage, TurnRequest, TurnResult, TurnStatus } from '../types.js';
import type { LanguageModel, ModelMessage, StreamTextResult, ToolSet } from 'ai';

export interface LLMProviderInterface {
  name: string;
  executeTurn: (request: TurnRequest) => Promise<TurnResult>;
}

export abstract class BaseLLMProvider implements LLMProviderInterface {
  abstract name: string;
  abstract executeTurn(request: TurnRequest): Promise<TurnResult>;

  public mapError(error: unknown): TurnStatus {
    const UNKNOWN_ERROR_MESSAGE = 'Unknown error';
    if (error === null || error === undefined) return { type: 'invalid_response', message: UNKNOWN_ERROR_MESSAGE };

    const err = error as Record<string, unknown>;
    const message = typeof err.message === 'string' ? err.message : UNKNOWN_ERROR_MESSAGE;
    const status = typeof err.status === 'number' ? err.status : typeof err.statusCode === 'number' ? err.statusCode : 0;
    const name = typeof err.name === 'string' ? err.name : 'Error';

    // Rate limit errors
    if (status === 429 || name.includes('RateLimit') || message.includes('rate limit')) {
      const RETRY_AFTER_HEADER = 'retry-after';
      const headers = err.headers as Record<string, unknown> | undefined;
      const retryAfter = (typeof headers?.[RETRY_AFTER_HEADER] === 'string' || typeof headers?.[RETRY_AFTER_HEADER] === 'number') 
        ? headers[RETRY_AFTER_HEADER] 
        : typeof err.retryAfter === 'string' || typeof err.retryAfter === 'number' ? err.retryAfter : undefined;
      const retryAfterMs = retryAfter !== undefined ? Number(retryAfter) * 1000 : undefined;
      return { type: 'rate_limit', retryAfterMs };
    }

    // Authentication errors
    if (status === 401 || status === 403 || name.includes('Auth') || message.includes('authentication') || message.includes('unauthorized')) {
      return { type: 'auth_error', message };
    }

    // Quota exceeded
    if (status === 402 || message.includes('quota') || message.includes('billing') || message.includes('insufficient')) {
      return { type: 'quota_exceeded', message };
    }

    // Model errors
    if (status === 400 || name.includes('BadRequest') || message.includes('invalid') || message.includes('model')) {
      const retryable = !message.includes('permanently') && !message.includes('unsupported');
      return { type: 'model_error', message, retryable };
    }

    // Timeout errors
    if (name.includes('Timeout') || name.includes('AbortError') || message.includes('timeout') || message.includes('aborted')) {
      return { type: 'timeout', message };
    }

    // Network errors
    if (status >= 500 || name.includes('Network') || name.includes('ECONNRESET') || name.includes('ENOTFOUND') || message.includes('network') || message.includes('connection')) {
      return { type: 'network_error', message, retryable: true };
    }

    // Default to model error with retryable flag
    return { type: 'model_error', message, retryable: true };
  }

  protected extractTokenUsage(usage: Record<string, unknown>): TokenUsage {
    const inputTokens = typeof usage.inputTokens === 'number' ? usage.inputTokens 
      : typeof usage.prompt_tokens === 'number' ? usage.prompt_tokens 
      : typeof usage.input_tokens === 'number' ? usage.input_tokens : 0;
    const outputTokens = typeof usage.outputTokens === 'number' ? usage.outputTokens 
      : typeof usage.completion_tokens === 'number' ? usage.completion_tokens 
      : typeof usage.output_tokens === 'number' ? usage.output_tokens : 0;
    const cachedTokens = typeof usage.cachedTokens === 'number' ? usage.cachedTokens 
      : typeof usage.cached_tokens === 'number' ? usage.cached_tokens 
      : typeof usage.cache_creation_input_tokens === 'number' ? usage.cache_creation_input_tokens : undefined;
    const totalTokens = typeof usage.totalTokens === 'number' ? usage.totalTokens 
      : typeof usage.total_tokens === 'number' ? usage.total_tokens : inputTokens + outputTokens;

    return {
      inputTokens,
      outputTokens,
      cachedTokens: cachedTokens !== undefined && cachedTokens > 0 ? cachedTokens : undefined,
      totalTokens
    };
  }

  protected createSuccessResult(
    messages: ConversationMessage[],
    tokens: TokenUsage,
    latencyMs: number,
    hasToolCalls: boolean,
    finalAnswer: boolean,
    response?: string
  ): TurnResult {
    return {
      status: { type: 'success', hasToolCalls, finalAnswer },
      response,
      toolCalls: hasToolCalls ? this.extractToolCalls(messages) : undefined,
      tokens,
      latencyMs,
      messages
    };
  }

  protected extractToolCalls(messages: ConversationMessage[]) {
    return messages
      .filter(m => m.role === 'assistant' && m.toolCalls !== undefined)
      .flatMap(m => m.toolCalls ?? []);
  }

  protected createFailureResult(status: TurnStatus, latencyMs: number): TurnResult {
    return {
      status,
      latencyMs,
      messages: []
    };
  }

  protected convertTools(tools: MCPTool[], toolExecutor: (toolName: string, parameters: Record<string, unknown>) => Promise<string>): ToolSet {
    return Object.fromEntries(
      tools.map(tool => [
        tool.name,
        {
          description: tool.description,
          inputSchema: jsonSchema(tool.inputSchema),
          execute: async (args: unknown) => {
            const parameters = args as Record<string, unknown>;
            return await toolExecutor(tool.name, parameters);
          }
        }
      ])
    ) as ToolSet;
  }

  // Shared streaming utilities
  protected createTimeoutController(timeoutMs: number): { controller: AbortController; resetIdle: () => void; clearIdle: () => void } {
    const controller = new AbortController();
    let idleTimer: ReturnType<typeof setTimeout> | undefined;
    
    const resetIdle = () => {
      try { if (idleTimer !== undefined) clearTimeout(idleTimer); } catch {}
      idleTimer = setTimeout(() => { try { controller.abort(); } catch {} }, timeoutMs);
    };

    const clearIdle = () => { 
      try { if (idleTimer !== undefined) { clearTimeout(idleTimer); idleTimer = undefined; } } catch {} 
    };

    return { controller, resetIdle, clearIdle };
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

  protected async executeStreamingTurn(
    model: LanguageModel,
    messages: ModelMessage[],
    tools: ToolSet | undefined,
    request: TurnRequest,
    startTime: number,
    providerOptions?: unknown
  ): Promise<TurnResult> {
    const { controller, resetIdle, clearIdle } = this.createTimeoutController(request.llmTimeout ?? 120000);
    
    try {
      resetIdle();
      
      const result = streamText({
        model,
        messages,
        tools: request.isFinalTurn === true ? undefined : tools,
        temperature: request.temperature,
        topP: request.topP,
        providerOptions: providerOptions as never,
        abortSignal: controller.signal,
      });

      // Drain both text and reasoning streams with real-time callbacks
      const fullIterator = result.fullStream[Symbol.asyncIterator]();
      let response = '';
      let fullStep = await fullIterator.next();
      
      // eslint-disable-next-line functional/no-loop-statements
      while (fullStep.done !== true) {
        const part = fullStep.value;
        
        if (part.type === 'text-delta') {
          response += part.text;
          
          // Call chunk callback for real-time text streaming
          if (request.onChunk !== undefined) {
            request.onChunk(part.text, 'content');
          }
        } else if (part.type === 'reasoning-delta') {
          // Call chunk callback for real-time reasoning streaming
          if (request.onChunk !== undefined) {
            request.onChunk(part.text, 'thinking');
          }
        }
        
        resetIdle();
        fullStep = await fullIterator.next();
      }

      clearIdle();

      const usage = await result.usage;
      const resp = await result.response;
      const tokens = this.extractTokenUsage(usage);
      const latencyMs = Date.now() - startTime;

      const conversationMessages = this.convertResponseMessages(resp.messages, request.provider, request.model, tokens);
      const hasToolCalls = conversationMessages.some(m => m.toolCalls !== undefined && m.toolCalls.length > 0);
      const hasAssistantText = conversationMessages.some(m => m.role === 'assistant' && m.content.trim().length > 0);
      const finalAnswer = hasAssistantText && !hasToolCalls;

      return this.createSuccessResult(conversationMessages, tokens, latencyMs, hasToolCalls, finalAnswer, response);
    } catch (error) {
      clearIdle();
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
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
      const result = await generateText({
        model,
        messages,
        tools: request.isFinalTurn === true ? undefined : tools,
        temperature: request.temperature,
        topP: request.topP,
        providerOptions: providerOptions as never,
        abortSignal: AbortSignal.timeout(request.llmTimeout ?? 120000),
      });

      const tokens = this.extractTokenUsage(result.usage);
      const latencyMs = Date.now() - startTime;
      const response = result.text;

      const respObj = result.response as { messages?: unknown[] } | undefined ?? {};
      const conversationMessages = this.convertResponseMessages(
        (Array.isArray(respObj.messages) ? respObj.messages : []) as ResponseMessage[], 
        request.provider, 
        request.model, 
        tokens
      );
      const hasToolCalls = conversationMessages.some(m => m.toolCalls !== undefined && m.toolCalls.length > 0);
      const hasAssistantText = conversationMessages.some(m => m.role === 'assistant' && m.content.trim().length > 0);
      const finalAnswer = hasAssistantText && !hasToolCalls;

      return this.createSuccessResult(conversationMessages, tokens, latencyMs, hasToolCalls, finalAnswer, response);
    } catch (error) {
      return this.createFailureResult(this.mapError(error), Date.now() - startTime);
    }
  }

  // Abstract method that each provider must implement to convert response messages
  protected abstract convertResponseMessages(
    messages: ResponseMessage[], 
    provider: string, 
    model: string, 
    tokens: TokenUsage
  ): ConversationMessage[];
}

// Common response message interface for providers
interface ResponseMessage {
  role?: string;
  content?: string | { type?: string; text?: string }[];
  toolCalls?: {
    id?: string;
    name?: string;
    arguments?: unknown;
    function?: { name?: string; arguments?: unknown };
    toolCallId?: string;
  }[];
}