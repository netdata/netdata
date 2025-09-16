import { jsonSchema } from '@ai-sdk/provider-utils';
import { generateText, streamText } from 'ai';

import type { ConversationMessage, MCPTool, TokenUsage, TurnRequest, TurnResult, TurnStatus } from '../types.js';
import type { LanguageModel, ModelMessage, StreamTextResult, ToolSet } from 'ai';

interface LLMProviderInterface {
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
    const firstDefined = (...vals: unknown[]) => vals.find(v => v !== undefined && v !== null);
    const nested = (obj: unknown, path: string[]): unknown => {
      // Safe nested property access
      let cur: unknown = obj;
      // eslint-disable-next-line functional/no-loop-statements
      for (const key of path) {
        if (cur === null || cur === undefined) return undefined;
        if (typeof cur !== 'object') return undefined;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
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
          } catch (e) { try { console.error(`[warn] fetch body text read failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
        }
      } catch (e) { try { console.error(`[warn] provider traced fetch failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
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
        // eslint-disable-next-line no-console
        console.error('[DEBUG] mapError:', {
          name,
          status,
          statusText: statusText ?? null,
          code: codeStr ?? null,
          hasResponseBody: typeof body === 'string',
          responseBodyLen: bodyLen,
          hasErrorData: typeof (primary as { data?: unknown }).data === 'object',
          messageComposed: composedMessage,
        });
      } catch { /* ignore debug errors */ }
    }

    // Rate limit errors
    if (status === 429 || name.includes('RateLimit') || providerMessage.toLowerCase().includes('rate limit')) {
      const RETRY_AFTER_HEADER = 'retry-after';
      const headers = err.headers as Record<string, unknown> | undefined;
      const retryAfter = (typeof headers?.[RETRY_AFTER_HEADER] === 'string' || typeof headers?.[RETRY_AFTER_HEADER] === 'number') 
        ? headers[RETRY_AFTER_HEADER] 
        : typeof err.retryAfter === 'string' || typeof err.retryAfter === 'number' ? err.retryAfter : undefined;
      const retryAfterMs = retryAfter !== undefined ? Number(retryAfter) * 1000 : undefined;
      return { type: 'rate_limit', retryAfterMs };
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
      return { type: 'model_error', message: composedMessage, retryable };
    }

    // Timeout errors
    if (name.includes('Timeout') || name.includes('AbortError') || providerMessage.toLowerCase().includes('timeout') || providerMessage.toLowerCase().includes('aborted')) {
      return { type: 'timeout', message: composedMessage };
    }

    // Network errors
    if (status >= 500 || name.includes('Network') || name.includes('ECONNRESET') || name.includes('ENOTFOUND') || providerMessage.toLowerCase().includes('network') || providerMessage.toLowerCase().includes('connection')) {
      return { type: 'network_error', message: composedMessage, retryable: true };
    }

    // Default to model error with retryable flag
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
      return tools.filter((t) => t.name === 'agent__final_report');
    }
    return tools;
  }

  protected buildFinalTurnMessages(messages: ModelMessage[], isFinalTurn?: boolean): ModelMessage[] {
    if (isFinalTurn === true) {
      const content = [
        '**CRITICAL**: You cannot collect more data!\n',
        '\n',
        'You must call the tool `agent__final_report` with your report.\n',
        '\n',
        'Review the collected data, check your instructions, and call the tool `agent__final_report` with your final report in the `report_content` field:\n',
        '\n',
        '- If the data is completely irrelevant or missing, set `status` to `failure` and describe the situation.\n',
        '- If the data is severealy incomplete, set `status` to `partial` and describe what you found.\n',
        '- If the data is rich, set `status` to `success` and provide a detailed report.\n',
        '\n',
        'Follow your instructions carefully, think hard, ensure your final report is accurate.\n',
        '\n',
        'Provide now your report by calling the tool `agent__final_report`.'
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

  protected convertTools(tools: MCPTool[], toolExecutor: (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }) => Promise<string>): ToolSet {
    return Object.fromEntries(
      tools.map(tool => [
        tool.name,
        {
          description: tool.description,
          inputSchema: jsonSchema(tool.inputSchema),
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
      if (request.abortSignal !== undefined) {
        if (request.abortSignal.aborted) { controller.abort(); }
        else {
          // Tie external abort to our controller
          const onAbort = () => { try { controller.abort(); } catch (e) { try { console.error(`[warn] controller.abort failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} } };
          request.abortSignal.addEventListener('abort', onAbort, { once: true });
        }
      }
    } catch (e) { try { console.error(`[warn] fetch finalization failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
    
    try {
      resetIdle();
      
      const result = streamText({
        model,
        messages,
        tools,
        toolChoice: 'required',
        maxOutputTokens: request.maxOutputTokens,
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
          console.error(`[DEBUG] ${dbgLabel}:`, { toolCalls, callIds, toolResults, resIds });
        } catch (e) { try { console.error(`[warn] extract usage json failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      const tokens = this.extractTokenUsage(usage);
      // Try to enrich with provider metadata (e.g., Anthropic cache creation tokens)
      try {
        const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object';
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion, @typescript-eslint/dot-notation
        const provMeta = isObj(resp) && isObj((resp as Record<string, unknown>).providerMetadata)
          // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion, @typescript-eslint/dot-notation
          ? ((resp as Record<string, unknown>).providerMetadata as Record<string, unknown>)
          : undefined;
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
        // const anthropic = isObj(provMeta) && isObj(provMeta.anthropic) ? (provMeta.anthropic as Record<string, unknown>) : undefined;
        // const w = anthropic !== undefined ? anthropic.cacheCreationInputTokens : undefined;
        if (typeof w === 'number' && Number.isFinite(w)) {
          tokens.cacheWriteInputTokens = Math.trunc(w);
        }
      } catch { /* ignore metadata errors */ }
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
      if (process.env.DEBUG === 'true') {
        try {
          const tMsgs = conversationMessages.filter((m) => m.role === 'tool');
          const ids = tMsgs.map((m) => (m.toolCallId ?? '[none]'));
          // eslint-disable-next-line no-console
          console.error('[DEBUG] converted-tool-messages(stream):', { count: tMsgs.length, ids });
        } catch (e) { try { console.error(`[warn] extract usage json failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      
      // Find the LAST assistant message
      const lastAssistantMessageArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = lastAssistantMessageArr.length > 0 ? lastAssistantMessageArr[lastAssistantMessageArr.length - 1] : undefined;

      // Detect final report request strictly by tool name, after normalization
      const normalizeName = (name: string) => name.replace(/^<\|[^|]+\|>/, '').trim();
      const finalReportRequested = lastAssistantMessage?.toolCalls?.some(tc => normalizeName(tc.name) === 'agent__final_report') === true;

      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
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
      
      // FINAL ANSWER POLICY: only when agent_final_report tool is requested (normalized), regardless of other conditions
      const finalAnswer = finalReportRequested;

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
      // Combine external abort signal with a timeout controller
      const { controller } = this.createTimeoutController(request.llmTimeout ?? 120000);
      try {
        if (request.abortSignal !== undefined) {
          if (request.abortSignal.aborted) { controller.abort(); }
          else {
            const onAbort = () => { try { controller.abort(); } catch (e) { try { console.error(`[warn] controller.abort failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} } };
            request.abortSignal.addEventListener('abort', onAbort, { once: true });
          }
        }
      } catch (e) { try { console.error(`[warn] streaming read failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      const result = await generateText({
        model,
        messages,
        tools,
        toolChoice: 'required',
        maxOutputTokens: request.maxOutputTokens,
        temperature: request.temperature,
        topP: request.topP,
        providerOptions: providerOptions as never,
        abortSignal: controller.signal,
      });

      const tokens = this.extractTokenUsage(result.usage);
      const latencyMs = Date.now() - startTime;
      let response = result.text;
      
      // Debug logging to understand the response structure
      if (process.env.DEBUG === 'true') {
        console.error(`[DEBUG] Non-stream result.text:`, result.text);
        console.error(`[DEBUG] Non-stream result.response:`, JSON.stringify(result.response, null, 2).substring(0, 500));
      }

      const respObj = (result.response as { messages?: unknown[]; providerMetadata?: Record<string, unknown> } | undefined) ?? {};
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
      } catch (e) { try { console.error(`[warn] json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
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
          console.error(`[DEBUG] ${dbgLabel}:`, { toolCalls, callIds, toolResults, resIds });
        } catch (e) { try { console.error(`[warn] json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }

      // Extract reasoning from AI SDK's normalized messages (for non-streaming mode)
      // The AI SDK puts reasoning as ReasoningPart in the content array
      if (request.onChunk !== undefined && Array.isArray(respObj.messages)) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const msg of respObj.messages) {
          const m = msg as { role?: string; content?: unknown };
          if (m.role === 'assistant' && Array.isArray(m.content)) {
            // eslint-disable-next-line functional/no-loop-statements
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
      if (process.env.DEBUG === 'true') {
        try {
          const tMsgs = conversationMessages.filter((m) => m.role === 'tool');
          const ids = tMsgs.map((m) => (m.toolCallId ?? '[none]'));
          // eslint-disable-next-line no-console
          console.error('[DEBUG] converted-tool-messages(nonstream):', { count: tMsgs.length, ids });
        } catch (e) { try { console.error(`[warn] json parse failed: ${e instanceof Error ? e.message : String(e)}`); } catch {} }
      }
      
      // Find the LAST assistant message to determine if we need more tool calls
      const laArr = conversationMessages.filter(m => m.role === 'assistant');
      const lastAssistantMessage = laArr.length > 0 ? laArr[laArr.length - 1] : undefined;
      
      // Only count tool calls that target currently available tools
      const validToolNames = tools !== undefined ? Object.keys(tools) : [];
      const normalizeName = (name: string) => name.replace(/^<\|[^|]+\|>/, '').trim();
      const hasNewToolCalls = lastAssistantMessage?.toolCalls !== undefined &&
        lastAssistantMessage.toolCalls.filter(tc => validToolNames.includes(normalizeName(tc.name))).length > 0;
      const finalReportRequested = lastAssistantMessage?.toolCalls?.some(tc => normalizeName(tc.name) === 'agent__final_report') === true;
      const hasAssistantText = (lastAssistantMessage?.content.trim().length ?? 0) > 0;

      // Detect reasoning parts in non-streaming response
      const hasReasoning = Array.isArray(respObj.messages)
        && (respObj.messages).some((msg) => {
          const m = msg as { role?: string; content?: unknown };
          if (m.role !== 'assistant' || !Array.isArray(m.content)) return false;
          return (m.content as unknown[]).some((p) => (p as { type?: string }).type === this.REASONING_TYPE);
        });
      
      // FINAL ANSWER POLICY: only when agent_final_report tool is requested (normalized), regardless of other conditions
      const finalAnswer = finalReportRequested;

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
                  messages.push(`(tool failed: ${errorMsg})`);
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
                text = `(tool failed: ${p.error})`;
              } else if (typeof p.error === 'object' && p.error !== null) {
                const e = p.error as { message?: string; error?: string };
                text = `(tool failed: ${e.message ?? e.error ?? JSON.stringify(p.error)})`;
              } else {
                text = `(tool failed: ${JSON.stringify(p.error)})`;
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
                text = '(no output)';
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
                text = `(tool failed: ${p.error})`;
              } else if (typeof p.error === 'object' && p.error !== null) {
                const e = p.error as { message?: string; error?: string };
                text = `(tool failed: ${e.message ?? e.error ?? JSON.stringify(p.error)})`;
              } else {
                text = `(tool failed: ${JSON.stringify(p.error)})`;
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
                text = '(no output)';
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
    

    const normalizeToolName = (name: string): string => name.replace(/^<\|[^|]+\|>/, '').trim();
    if (Array.isArray(m.content)) {
      const textParts: string[] = [];
      const toolCalls: { id: string; name: string; parameters: Record<string, unknown> }[] = [];
      
      // eslint-disable-next-line functional/no-loop-statements
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
            name: normalizeToolName(typeof part.toolName === "string" ? part.toolName : ""),
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
