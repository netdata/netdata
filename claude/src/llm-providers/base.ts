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

  protected extractTokenUsage(usage: Record<string, unknown> | undefined): TokenUsage {
    const num = (v: unknown): number => {
      if (typeof v === 'number' && Number.isFinite(v)) return v;
      if (typeof v === 'string') {
        const n = Number(v);
        return Number.isFinite(n) ? n : 0;
      }
      return 0;
    };
    const u = usage ?? {} as Record<string, unknown>;
    const inputTokens = num(u.inputTokens) || num(u.prompt_tokens) || num(u.input_tokens);
    const outputTokens = num(u.outputTokens) || num(u.completion_tokens) || num(u.output_tokens);
    const cachedTokensRaw = num(u.cachedTokens) || num(u.cached_tokens) || num(u.cache_creation_input_tokens);
    const cachedTokens = cachedTokensRaw > 0 ? cachedTokensRaw : undefined;
    const totalTokensRaw = num(u.totalTokens) || num(u.total_tokens);
    const totalTokens = totalTokensRaw > 0 ? totalTokensRaw : (inputTokens + outputTokens);

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
    response?: string,
    extras?: { hasReasoning?: boolean; hasContent?: boolean }
  ): TurnResult {
    return {
      status: { type: 'success', hasToolCalls, finalAnswer },
      response,
      toolCalls: hasToolCalls ? this.extractToolCalls(messages) : undefined,
      tokens,
      latencyMs,
      messages,
      hasReasoning: extras?.hasReasoning,
      hasContent: extras?.hasContent,
    };
  }

  protected extractToolCalls(messages: ConversationMessage[]) {
    return messages
      .filter(m => m.role === 'assistant' && m.toolCalls !== undefined)
      .flatMap(m => m.toolCalls ?? []);
  }

  // DRY helpers for final-turn behavior across providers
  protected filterToolsForFinalTurn(tools: MCPTool[], isFinalTurn?: boolean): MCPTool[] {
    if (isFinalTurn === true) {
      return tools.filter((t) => t.name === 'agent_final_report');
    }
    return tools;
  }

  protected buildFinalTurnMessages(messages: ModelMessage[], isFinalTurn?: boolean): ModelMessage[] {
    if (isFinalTurn === true) {
      const content = [
        'FINAL TURN: You MUST finish now by calling the tool `agent_final_report` exactly once.',
        'Do NOT produce assistant text. Assistant text will be ignored.',
        'Only `agent_final_report` is available; do not attempt any other tools.',
        'If data is incomplete, set `status` to `partial` or `failure`, and include known gaps in the report.',
      ].join(' ');
      return messages.concat({ role: 'user', content } as ModelMessage);
    }
    return messages;
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

  // Constants
  private readonly REASONING_TYPE = 'reasoning';
  private readonly TOOL_RESULT_TYPE = 'tool-result';
  
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
        tools,
        toolChoice: 'required',
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

      // Debug: log the raw response structure
      if (process.env.DEBUG === 'true') {
        console.error(`[DEBUG] resp.messages structure:`, JSON.stringify(resp.messages, null, 2).substring(0, 2000));
      }
      
      // Backfill: Emit chunks for content that wasn't streamed (common with tool calls)
      // Many providers don't stream text-delta or reasoning-delta when producing tool calls
      if (request.onChunk !== undefined && Array.isArray(resp.messages)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const msg of resp.messages) {
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
                if (!hasEmittedText && !response.includes(p.text)) {
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
      
      const conversationMessages = this.convertResponseMessages(resp.messages, request.provider, request.model, tokens);
      
      // Find the LAST assistant message to determine if we need more tool calls
      const lastAssistantMessageArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = lastAssistantMessageArr.length > 0 ? lastAssistantMessageArr[lastAssistantMessageArr.length - 1] : undefined;
      
      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
      const normalizeName = (name: string) => name.replace(/^<\|[^|]+\|>/, '').trim();
      const hasNewToolCalls = lastAssistantMessage?.toolCalls !== undefined &&
        lastAssistantMessage.toolCalls.filter(tc => validToolNames.includes(normalizeName(tc.name))).length > 0;
      const hasAssistantText = (lastAssistantMessage?.content.trim().length ?? 0) > 0;
      
      // Check if we have tool result messages (which means tools were executed)
      const hasToolResults = conversationMessages.some(m => m.role === 'tool');
      // Detect if provider emitted any reasoning parts
      const respAny = resp as { messages?: unknown[] };
      const respMsgs: unknown[] = Array.isArray(respAny.messages) ? respAny.messages : [];
      const hasReasoning = respMsgs.some((msg) => {
          const m = msg as { role?: string; content?: unknown };
          if (m.role !== 'assistant' || !Array.isArray(m.content)) return false;
          return (m.content as unknown[]).some((p) => (p as { type?: string }).type === this.REASONING_TYPE);
        });
      
      // Only mark as final if we have text and no new tool calls AND no tool results
      // If we have tool results, we need to continue the conversation for the LLM to process them
      const finalAnswer = hasAssistantText && !hasNewToolCalls && !hasToolResults;

      // Do not error on reasoning-only responses; allow agent loop to decide retries.
      
      // Debug logging
      if (process.env.DEBUG === 'true') {
        console.error(`[DEBUG] Stream: hasNewToolCalls: ${String(hasNewToolCalls)}, hasAssistantText: ${String(hasAssistantText)}, hasToolResults: ${String(hasToolResults)}, finalAnswer: ${String(finalAnswer)}, response length: ${String(response.length)}`);
        console.error(`[DEBUG] lastAssistantMessage:`, JSON.stringify(lastAssistantMessage, null, 2));
        console.error(`[DEBUG] response text:`, response);
      }

      return this.createSuccessResult(
        conversationMessages,
        tokens,
        latencyMs,
        hasNewToolCalls,
        finalAnswer,
        response,
        { hasReasoning, hasContent: hasAssistantText }
      );
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
        tools,
        toolChoice: 'required',
        temperature: request.temperature,
        topP: request.topP,
        providerOptions: providerOptions as never,
        abortSignal: AbortSignal.timeout(request.llmTimeout ?? 120000),
      });

      const tokens = this.extractTokenUsage(result.usage);
      const latencyMs = Date.now() - startTime;
      let response = result.text;
      
      // Debug logging to understand the response structure
      if (process.env.DEBUG === 'true') {
        console.error(`[DEBUG] Non-stream result.text:`, result.text);
        console.error(`[DEBUG] Non-stream result.response:`, JSON.stringify(result.response, null, 2).substring(0, 500));
      }

      const respObj = (result.response as { messages?: unknown[] } | undefined) ?? {};
      
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
        console.error(`[DEBUG] Non-stream respObj.messages:`, JSON.stringify(respObj.messages, null, 2).substring(0, 2000));
      }
      
      const conversationMessages = this.convertResponseMessages(
        (Array.isArray(respObj.messages) ? respObj.messages : []) as ResponseMessage[], 
        request.provider, 
        request.model, 
        tokens
      );
      
      // Find the LAST assistant message to determine if we need more tool calls
      const laArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = laArr.length > 0 ? laArr[laArr.length - 1] : undefined;
      
      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
      const normalizeName = (name: string) => name.replace(/^<\|[^|]+\|>/, '').trim();
      const hasNewToolCalls = lastAssistantMessage?.toolCalls !== undefined &&
        lastAssistantMessage.toolCalls.filter(tc => validToolNames.includes(normalizeName(tc.name))).length > 0;
      const hasAssistantText = (lastAssistantMessage?.content.trim().length ?? 0) > 0;
      
      // Check if we have tool result messages (which means tools were executed)
      const hasToolResults = conversationMessages.some(m => m.role === 'tool');
      // Detect reasoning parts in non-streaming response
      const hasReasoning = Array.isArray(respObj.messages)
        && (respObj.messages).some((msg) => {
          const m = msg as { role?: string; content?: unknown };
          if (m.role !== 'assistant' || !Array.isArray(m.content)) return false;
          return (m.content as unknown[]).some((p) => (p as { type?: string }).type === this.REASONING_TYPE);
        });
      
      // Only mark as final if we have text and no new tool calls AND no tool results
      // If we have tool results, we need to continue the conversation for the LLM to process them
      const finalAnswer = hasAssistantText && !hasNewToolCalls && !hasToolResults;

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
                const p = part as { type?: string; text?: string; output?: { value?: string } };
                return p.text !== undefined || p.output?.value !== undefined;
              });
              
              const messages: string[] = [];
              // eslint-disable-next-line functional/no-loop-statements
              for (const part of allParts) {
                const p = part as { type?: string; text?: string; output?: { value?: string } };
                if (p.type === this.TOOL_RESULT_TYPE && p.output?.value !== undefined) {
                  messages.push(p.output.value);
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

      return this.createSuccessResult(
        conversationMessages,
        tokens,
        latencyMs,
        hasNewToolCalls,
        finalAnswer,
        response,
        { hasReasoning, hasContent: hasAssistantText }
      );
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
        const hasToolCalls = Array.isArray(m.toolCalls) && m.toolCalls.length > 0;
        if (hasToolCalls) {
          const parts: (
            { type: 'text'; text: string } |
            { type: 'tool-call'; toolCallId: string; toolName: string; input: unknown }
          )[] = [];
          if (m.content && m.content.trim().length > 0) {
            parts.push({ type: 'text', text: m.content });
          }
          // eslint-disable-next-line functional/no-loop-statements
          for (const tc of m.toolCalls ?? []) {
            parts.push({ type: 'tool-call', toolCallId: tc.id, toolName: tc.name, input: tc.parameters });
          }
          modelMessages.push({ role: 'assistant', content: parts as never });
        } else {
          modelMessages.push({ role: 'assistant', content: m.content });
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
  
  /**
   * Helper method to parse AI SDK response messages which embed tool calls in content array
   */
  protected parseAISDKMessage(m: ResponseMessage, provider: string, model: string, tokens: Partial<TokenUsage>): ConversationMessage {
    // Extract tool calls from content array (AI SDK format)
    let toolCallsFromContent: ConversationMessage['toolCalls'] = undefined;
    let textContent = '';
    let toolCallId: string | undefined;
    
    if (Array.isArray(m.content)) {
      const textParts: string[] = [];
      const toolCalls: { id: string; name: string; parameters: Record<string, unknown> }[] = [];
      
      // eslint-disable-next-line functional/no-loop-statements
      for (const part of m.content) {
        if (part.type === 'text' || part.type === undefined) {
          if (part.text !== undefined && part.text !== '') {
            textParts.push(part.text);
          }
        } else if (part.type === this.REASONING_TYPE) {
          // Skip reasoning parts - they should NOT be included in text content
          // Reasoning is only for thinking/telemetry, not for final answer detection
          // This prevents the agent from treating reasoning-only responses as final answers
        } else if (part.type === 'tool-call' && part.toolCallId !== undefined && part.toolName !== undefined) {
          // AI SDK embeds tool calls in content
          toolCalls.push({
            id: part.toolCallId,
            name: part.toolName,
            parameters: (part.input ?? {}) as Record<string, unknown>
          });
        } else if (part.type === this.TOOL_RESULT_TYPE && part.toolCallId !== undefined) {
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
    const finalToolCalls = toolCallsFromContent ?? (Array.isArray(m.toolCalls) ? m.toolCalls.map(tc => ({
      id: tc.id ?? tc.toolCallId ?? '',
      name: tc.name ?? tc.function?.name ?? '',
      parameters: (tc.arguments ?? tc.function?.arguments ?? {}) as Record<string, unknown>
    })) : undefined);
    
    return {
      role: (m.role ?? 'assistant') as ConversationMessage['role'],
      content: textContent,
      toolCalls: finalToolCalls,
      toolCallId: toolCallId ?? ('toolCallId' in m ? (m as { toolCallId?: string }).toolCallId : undefined),
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
