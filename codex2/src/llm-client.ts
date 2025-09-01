import { dynamicTool, jsonSchema } from '@ai-sdk/provider-utils';
import { generateText, streamText, type ModelMessage, type ToolSet } from 'ai';

import type { AccountingEntry, ConversationMessage, LogEntry, TokenUsage, TurnRequest, TurnResult } from './types.js';

export class LLMClient {
  constructor(private log?: (entry: LogEntry) => void) {}

  private vrb(turn: number, subturn: number, dir: 'request'|'response', id: string, msg: string): void {
    this.log?.({ timestamp: Date.now(), severity: 'VRB', turn, subturn, direction: dir, type: 'llm', remoteIdentifier: id, fatal: false, message: msg });
  }
  private trc(turn: number, subturn: number, dir: 'request'|'response', id: string, msg: string): void {
    this.log?.({ timestamp: Date.now(), severity: 'TRC', turn, subturn, direction: dir, type: 'llm', remoteIdentifier: id, fatal: false, message: msg });
  }
  private err(turn: number, subturn: number, id: string, msg: string, fatal: boolean): void {
    this.log?.({ timestamp: Date.now(), severity: 'ERR', turn, subturn, direction: 'response', type: 'llm', remoteIdentifier: id, fatal, message: msg });
  }

  async executeTurn(req: TurnRequest & { traceLLM?: boolean; verbose?: boolean }): Promise<TurnResult> {
    const { provider, model, messages, tools, isFinalTurn, temperature, topP, stream, llmTimeout, providerOptions, onOutput, onReasoning, onAccounting, getModel } = req;
    const id = `${provider}:${model}`;
    const start = Date.now();
    let executedTools: { name: string; output: string }[] = [];
    let inputTokens = 0, outputTokens = 0, cachedTokens: number | undefined;
    let responseMessages: ConversationMessage[] = [];

    const mMsgs: ModelMessage[] = messages.map((m) => ({
      role: m.role,
      content: m.content,
      ...(Array.isArray(m.toolCalls) ? { toolCalls: m.toolCalls } : {}),
      ...(typeof m.toolCallId === 'string' ? { toolCallId: m.toolCallId } : {}),
    }));
    const finalMsgs: ModelMessage[] = isFinalTurn
      ? mMsgs.concat({ role: 'user', content: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate" } as ModelMessage)
      : mMsgs;

    // Build ToolSet that delegates to external executor
    const toolSet = Object.fromEntries(tools.map((t) => [
      t.name,
      dynamicTool({
        description: t.description,
        inputSchema: jsonSchema(t.inputSchema),
        execute: async (args: Record<string, unknown>) => {
          try {
            const res = await req.toolExecutor({ id: String(Date.now()) + '-' + Math.random().toString(36).slice(2), name: t.name, parameters: args });
            let out = typeof res.text === 'string' ? res.text : '';
            if (!res.ok) {
              const errMsg = typeof res.error === 'string' && res.error.length > 0 ? res.error : 'Tool execution failed';
              if (out.length === 0) out = 'ERROR: ' + errMsg;
            } else if (out.length > 0) {
              executedTools.push({ name: t.name, output: out });
            }
            return out;
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            return 'ERROR: ' + msg;
          }
        },
        toModelOutput: (output: unknown) => ({ type: 'text', value: typeof output === 'string' ? output : JSON.stringify(output) }),
      }),
    ])) as ToolSet;

    const enc = new TextEncoder();
    const bytesSize = finalMsgs.reduce((acc, m) => {
      const s = typeof m.content === 'string' ? m.content : JSON.stringify(m.content);
      return acc + enc.encode(s).length;
    }, 0);
    this.vrb(0, 0, 'request', id, 'messages ' + String(finalMsgs.length) + ', ' + String(bytesSize) + ' bytes');

    try {
      if (stream) {
        const ac = new AbortController();
        let idle: ReturnType<typeof setTimeout> | undefined;
        const reset = (): void => { if (idle !== undefined) { clearTimeout(idle); } idle = setTimeout(() => { ac.abort(); }, llmTimeout); };
        const clear = (): void => { if (idle !== undefined) { clearTimeout(idle); } };
        reset();
        const st = streamText({ model: getModel(model), messages: finalMsgs, tools: isFinalTurn ? undefined : toolSet, temperature, topP, providerOptions, abortSignal: ac.signal, onChunk: (ev: unknown) => { try { const c = (ev as { chunk?: { type?: string; text?: string; delta?: string } } | undefined)?.chunk; if (c?.type === 'reasoning-delta') { onReasoning?.(c.text ?? c.delta ?? ''); } } catch { /* noop */ } } });
        let last: string | undefined;
        const it = st.textStream[Symbol.asyncIterator]();
        const pump = async (): Promise<void> => {
          const n = await it.next();
          if (n.done === true) return;
          const chunk = n.value;
          reset();
          if (typeof chunk === 'string' && chunk.length > 0) { onOutput?.(chunk); last = chunk; }
          await pump();
        };
        await pump();
        clear();
        if (typeof last === 'string' && last.length > 0 && !last.endsWith('\n')) onOutput?.('\n');
        const usage = (st as unknown as { usage?: TokenUsage }).usage;
        inputTokens = usage?.inputTokens ?? 0;
        outputTokens = usage?.outputTokens ?? 0;
        cachedTokens = (usage?.cachedTokens ?? undefined);
        const respObj = (st as unknown as { response?: { messages?: unknown[] } }).response ?? {};
        const raw = Array.isArray(respObj.messages) ? respObj.messages : [];
        const safeStr = (x: unknown): string => (typeof x === 'string' ? x : (typeof x === 'number' ? String(x) : ''));
        responseMessages = raw.map((mRaw) => {
          const m = mRaw as { role?: string; content?: unknown; toolCalls?: unknown; toolCallId?: unknown };
          const tc = Array.isArray(m.toolCalls) ? (m.toolCalls as unknown[]).map((x) => {
            const t = x as { id?: unknown; toolCallId?: unknown; name?: unknown; function?: { name?: unknown; arguments?: unknown }; arguments?: unknown };
            return { id: safeStr(t.id ?? t.toolCallId ?? ''), name: safeStr(t.name ?? (t.function?.name ?? '')), parameters: (t.arguments ?? t.function?.arguments ?? {}) as Record<string, unknown> };
          }) : undefined;
          const content = typeof m.content === 'string' ? m.content : JSON.stringify(m.content);
          const totalTokens = typeof usage?.totalTokens === 'number' ? usage.totalTokens : (inputTokens + outputTokens);
          return { role: (typeof m.role === 'string' ? m.role : 'assistant'), content, toolCalls: tc, toolCallId: typeof m.toolCallId === 'string' ? m.toolCallId : undefined, metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens, cachedTokens }, timestamp: Date.now() } } as ConversationMessage;
        });
      } else {
        const gt = await generateText({ model: getModel(model), messages: finalMsgs, tools: isFinalTurn ? undefined : toolSet, temperature, topP, providerOptions, abortSignal: AbortSignal.timeout(llmTimeout), onStepFinish: (s: unknown) => { try { if (typeof (s as { reasoningText?: unknown }).reasoningText === 'string') { onReasoning?.((s as { reasoningText: string }).reasoningText); } } catch { /* noop */ } } });
        if (typeof gt.text === 'string' && gt.text.length > 0) onOutput?.(gt.text + (gt.text.endsWith('\n') ? '' : '\n'));
        const usage = (gt as unknown as { usage?: TokenUsage }).usage;
        inputTokens = usage?.inputTokens ?? 0;
        outputTokens = usage?.outputTokens ?? 0;
        cachedTokens = (usage?.cachedTokens ?? undefined);
        const respObj = gt.response as { messages?: unknown[] };
        const raw = Array.isArray(respObj.messages) ? respObj.messages : [];
        const safeStr2 = (x: unknown): string => (typeof x === 'string' ? x : (typeof x === 'number' ? String(x) : ''));
        responseMessages = raw.map((mRaw) => {
          const m = mRaw as { role?: string; content?: unknown; toolCalls?: unknown; toolCallId?: unknown };
          const tc = Array.isArray(m.toolCalls) ? (m.toolCalls as unknown[]).map((x) => {
            const t = x as { id?: unknown; toolCallId?: unknown; name?: unknown; function?: { name?: unknown; arguments?: unknown }; arguments?: unknown };
            return { id: safeStr2(t.id ?? t.toolCallId ?? ''), name: safeStr2(t.name ?? (t.function?.name ?? '')), parameters: (t.arguments ?? t.function?.arguments ?? {}) as Record<string, unknown> };
          }) : undefined;
          const content = typeof m.content === 'string' ? m.content : JSON.stringify(m.content);
          const totalTokens = typeof usage?.totalTokens === 'number' ? usage.totalTokens : (inputTokens + outputTokens);
          return { role: (typeof m.role === 'string' ? m.role : 'assistant'), content, toolCalls: tc, toolCallId: typeof m.toolCallId === 'string' ? m.toolCallId : undefined, metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens, cachedTokens }, timestamp: Date.now() } } as ConversationMessage;
        });
      }

      const latency = Date.now() - start;
      const respBytes = responseMessages.reduce((acc, m) => acc + enc.encode(m.content).length, 0);
      this.vrb(0, 0, 'response', id, 'input ' + String(inputTokens) + ', output ' + String(outputTokens) + ', cached ' + String(cachedTokens ?? 0) + ' tokens, tools ' + String(executedTools.length) + ', latency ' + String(latency) + ' ms, size ' + String(respBytes) + ' bytes');
      onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'ok', latency, provider, model, tokens: { inputTokens, outputTokens, totalTokens: (inputTokens + outputTokens), cachedTokens } } as AccountingEntry);

      const hasToolArtifacts = responseMessages.some((m) => (m.role === 'tool') || (Array.isArray(m.toolCalls) && m.toolCalls.length > 0));
      const hasAssistantText = responseMessages.some((m) => m.role === 'assistant' && typeof m.content === 'string' && m.content.trim().length > 0);
      const status: TurnResult['status'] = { type: 'success', hasToolCalls: hasToolArtifacts, finalAnswer: hasAssistantText && !hasToolArtifacts };
      return { status, responseMessages, tokens: { inputTokens, outputTokens, totalTokens: inputTokens + outputTokens, cachedTokens }, latencyMs: latency, executedTools };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      const latency = Date.now() - start;
      // Error taxonomy mapping
      const lower = msg.toLowerCase();
      const status = (() => {
        // Timeout / abort
        if (lower.includes('timeout') || lower.includes('abort')) return { type: 'timeout', message: msg } as const;
        // Auth
        if (lower.includes('unauthorized') || lower.includes('invalid api key') || lower.includes('api key')) return { type: 'auth_error', message: msg } as const;
        // Quota
        if (lower.includes('quota')) return { type: 'quota_exceeded', message: msg } as const;
        // Rate limit
        if (lower.includes('rate') && lower.includes('limit')) return { type: 'rate_limit', retryAfterMs: undefined } as const;
        // Network-ish
        if (lower.includes('fetch failed') || lower.includes('ecconn') || lower.includes('enotfound') || lower.includes('network')) return { type: 'network_error', message: msg, retryable: true } as const;
        // Default model error
        return { type: 'model_error', message: msg, retryable: false } as const;
      })();
      const fatal = status.type === 'auth_error' || status.type === 'quota_exceeded';
      this.log?.({ timestamp: Date.now(), severity: fatal ? 'ERR' : 'WRN', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: id, fatal, message: msg });
      onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'failed', latency, provider, model, tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 }, error: msg } as AccountingEntry);
      return { status: status as unknown as TurnResult['status'], responseMessages: [], latencyMs: latency, executedTools: [] };
    }
  }
}
