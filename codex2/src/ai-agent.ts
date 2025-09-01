import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import { createOllama } from 'ollama-ai-provider-v2';

import type { MCPToolDef } from './mcp-client.js';
import type { AIAgentCreateOptions, AIAgentRunResult, AccountingEntry, Configuration, ConversationMessage, LogEntry } from './types.js';
import type { SharedV2ProviderOptions } from '@ai-sdk/provider';

import { LLMClient } from './llm-client.js';
import { MCPClientManager } from './mcp-client.js';

export const AIAgent = {
  create(opts: AIAgentCreateOptions): Promise<AIAgentSession> {
    return Promise.resolve(new AIAgentSession(opts));
  },
} as const;

export class AIAgentSession {
  private readonly config: Configuration;
  private readonly providers: string[];
  private readonly models: string[];
  private readonly tools: string[];
  private readonly systemPrompt: string;
  private readonly userPrompt: string;
  private readonly conversationHistory?: ConversationMessage[];
  private readonly options: Required<Pick<AIAgentCreateOptions, 'llmTimeout' | 'toolTimeout' | 'temperature' | 'topP' | 'parallelToolCalls' | 'verbose' | 'stream' | 'maxRetries' | 'maxToolTurns'>> & { traceLLM: boolean; traceMCP: boolean };
  private readonly callbacks: AIAgentCreateOptions['callbacks'];

  private logs: LogEntry[] = [];
  private mcp!: MCPClientManager;
  private llm!: LLMClient;

  constructor(opts: AIAgentCreateOptions) {
    this.config = opts.config;
    this.providers = opts.providers;
    this.models = opts.models;
    this.tools = opts.tools;
    this.systemPrompt = opts.systemPrompt;
    this.userPrompt = opts.userPrompt;
    this.conversationHistory = opts.conversationHistory;
    const d = opts.config.defaults ?? {};
    this.options = {
      llmTimeout: opts.llmTimeout ?? d.llmTimeout ?? 120000,
      toolTimeout: opts.toolTimeout ?? d.toolTimeout ?? 60000,
      temperature: opts.temperature ?? d.temperature ?? 0.7,
      topP: opts.topP ?? d.topP ?? 1.0,
      parallelToolCalls: opts.parallelToolCalls ?? (d.parallelToolCalls ?? true),
      verbose: opts.verbose ?? false,
      stream: opts.stream ?? (d.stream ?? true),
      maxRetries: opts.maxRetries ?? (opts.config.defaults?.maxRetries ?? 3),
      maxToolTurns: opts.maxToolTurns ?? (opts.config.defaults?.maxToolTurns ?? 30),
      traceLLM: opts.traceLLM ?? false,
      traceMCP: opts.traceMCP ?? false,
    };
    const onLog = (entry: LogEntry) => { this.logs.push(entry); this.callbacks?.onLog?.(entry); };
    this.callbacks = opts.callbacks ?? {};
    this.mcp = new MCPClientManager(onLog, this.options.traceMCP, this.options.verbose);
    this.llm = new LLMClient(onLog);
  }

  async run(): Promise<AIAgentRunResult> {
    // Initialize MCP and collect tools
    const selected = Object.fromEntries(this.tools.map((t) => [t, this.config.mcpServers[t]]));
    try {
      await this.mcp.initializeServers(selected, 0);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      this.callbacks?.onLog?.({ timestamp: Date.now(), severity: 'WRN', turn: 0, subturn: 0, direction: 'response', type: 'mcp', remoteIdentifier: 'init', fatal: false, message: 'Initialization failed: ' + msg });
    }

    const mcpTools = this.mcp.getAllTools();
    const mapping = this.mcp.getToolServerMapping();
    const toolDefs: MCPToolDef[] = mcpTools;

    // Enhance system prompt once
    const sys = this.enhanceSystemPrompt(this.systemPrompt, this.mcp.combinedInstructions());
    let conversation: ConversationMessage[];
    if (Array.isArray(this.conversationHistory) && this.conversationHistory.length > 0) {
      conversation = [...this.conversationHistory];
      if (conversation[0]?.role === 'system') conversation[0] = { role: 'system', content: sys };
      else conversation.unshift({ role: 'system', content: sys });
      // Always append the current user prompt for this run
      conversation.push({ role: 'user', content: this.userPrompt });
    } else {
      conversation = [ { role: 'system', content: sys }, { role: 'user', content: this.userPrompt } ];
    }

    // Provider/model attempt list
    const pairs = this.models.flatMap((m) => this.providers.map((p) => ({ provider: p, model: m })));

    // Turn loop
    // eslint-disable-next-line functional/no-loop-statements
    for (let turn = 0; turn < this.options.maxToolTurns; turn++) {
      // eslint-disable-next-line functional/no-loop-statements
      for (let attempt = 0; attempt < this.options.maxRetries; attempt++) {
        // eslint-disable-next-line functional/no-loop-statements
        for (const { provider, model } of pairs) {
          const getModel = this.getModelFactory(provider);
          const providerOptions = this.getProviderOptions(provider);
          const stream = this.getEffectiveStream(provider);

          const result = await this.llm.executeTurn({
            provider, model,
            messages: conversation,
            tools: toolDefs,
            isFinalTurn: turn >= (this.options.maxToolTurns - 1),
            temperature: this.options.temperature,
            topP: this.options.topP,
            stream,
            llmTimeout: this.options.llmTimeout,
            providerOptions,
            onOutput: (t) => this.callbacks?.onOutput?.(t),
            onReasoning: (t) => { this.callbacks?.onLog?.({ timestamp: Date.now(), severity: 'VRB', turn, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: provider + ':' + model, fatal: false, message: 'thinking: ' + t }); },
            onAccounting: (e) => this.callbacks?.onAccounting?.(e),
            log: (e) => this.callbacks?.onLog?.(e),
            toolExecutor: async (call) => {
              const server = mapping.get(call.name) ?? '';
              const res = await this.mcp.executeTool(server, { id: call.id, name: call.name, parameters: call.parameters }, this.options.toolTimeout, turn, 1);
              const t = res.status.type;
              const errorMsg = t === 'success' ? undefined : (t === 'execution_error' || t === 'invalid_parameters' || t === 'connection_error' || t === 'mcp_server_error' || t === 'timeout' ? (res.status as { message: string }).message : undefined);
              const entry: AccountingEntry = { type: 'tool', timestamp: Date.now(), status: res.status.type === 'success' ? 'ok' : 'failed', latency: res.latencyMs, mcpServer: res.metadata.mcpServer, command: res.metadata.command, charactersIn: res.metadata.inSize, charactersOut: res.metadata.outSize, ...(typeof errorMsg === 'string' ? { error: errorMsg } : {}) };
              this.callbacks?.onAccounting?.(entry);
              return { ok: res.status.type === 'success', text: res.result, serverName: server, error: errorMsg, latency: res.latencyMs, inSize: res.metadata.inSize, outSize: res.metadata.outSize };
            },
            getModel,
            traceLLM: this.options.traceLLM,
            verbose: this.options.verbose,
          });

          conversation = this.mergeConversation(conversation, result.responseMessages);

          if (result.status.type === 'success') {
            if (result.status.finalAnswer) return { success: true, conversation, logs: this.logs };
            if (result.status.hasToolCalls) break; // proceed to next turn with tool outputs integrated
          } else {
            continue; // next provider/model
          }
        }
        // After trying all pairs: if we saw tool calls, break to next turn; otherwise retry attempts
        const last = conversation.length > 0 ? conversation[conversation.length - 1] : undefined;
        const sawTool = ((last != null) && last.role === 'tool') || conversation.slice(-10).some((m) => Array.isArray(m.toolCalls) && m.toolCalls.length > 0);
        if (sawTool) break;
      }
    }

    return { success: false, error: 'Max tool turns exceeded', conversation: [], logs: this.logs };
  }

  private enhanceSystemPrompt(base: string, mcpInstructions: string): string {
    if (!mcpInstructions || mcpInstructions.trim().length === 0) return base;
    return `${base}\n\n## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`;
  }

  private mergeConversation(prev: ConversationMessage[], next: ConversationMessage[]): ConversationMessage[] {
    if (prev.length === 0) return next;
    const out = [...prev];
    // Replace initial system if exists
    if (out[0].role === 'system' && next[0]?.role === 'system') out[0] = next[0];
    else if (next[0]?.role === 'system') out.unshift(next[0]);
    // Append the rest
    out.push(...next.slice(next[0]?.role === 'system' ? 1 : 0));
    return out;
  }

  private getModelFactory(providerName: string) {
    const cfg = this.config.providers[providerName] ?? {};
    const tracedFetch = this.makeTracedFetch(providerName);
    if (providerName === 'openai' || (cfg.type === 'openai')) {
      const defaultMode: 'responses'|'chat' = (typeof cfg.baseUrl === 'string' && cfg.baseUrl.includes('api.openai.com')) ? 'responses' : 'chat';
      const mode: 'responses'|'chat' = (cfg.openaiMode === 'responses' || cfg.openaiMode === 'chat') ? cfg.openaiMode : defaultMode;
      const prov = createOpenAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
      return (m: string) => {
        const p = prov as unknown as { responses?: (model: string) => unknown; chat?: (model: string) => unknown };
        const fn = mode === 'responses' ? p.responses : p.chat;
        return (fn as (model: string) => unknown)(m);
      };
    }
    if (providerName === 'anthropic') {
      const prov = createAnthropic({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
      return (m: string) => prov(m);
    }
    if (providerName === 'google' || providerName === 'vertex') {
      const prov = createGoogleGenerativeAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
      return (m: string) => prov(m);
    }
    if (providerName === 'openrouter') {
      const prov = createOpenRouter({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch, headers: { 'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local', 'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-codex2', 'User-Agent': 'ai-agent-codex2/1.0', ...(cfg.headers ?? {}) } });
      return (m: string) => prov(m);
    }
    if (providerName === 'ollama') {
      // normalize baseURL for native /api
      const normalize = (u?: string) => { const def = 'http://localhost:11434/api'; if (typeof u !== 'string' || u.length === 0) return def; const v0 = u.replace(/\/$/, ''); if (/\/v1\/?$/.test(v0)) return v0.replace(/\/v1\/?$/, '/api'); if (/\/api\/?$/.test(v0)) return v0; return v0 + '/api'; };
      const prov = createOllama({ baseURL: normalize(cfg.baseUrl), fetch: tracedFetch });
      return (m: string) => prov(m);
    }
    throw new Error(`Unsupported provider: ${providerName}`);
  }

  private getProviderOptions(provider: string): Record<string, unknown> | SharedV2ProviderOptions | undefined {
    const cfg = this.config.providers[provider];
    if (provider === 'openai' || provider === 'openrouter') {
      return { openai: { parallelToolCalls: this.options.parallelToolCalls } } as SharedV2ProviderOptions;
    }
    if (provider === 'ollama') {
      try {
        const custom = cfg.custom as { providerOptions?: Record<string, unknown> } | undefined;
        return custom?.providerOptions;
      } catch { return undefined; }
    }
    return undefined;
  }

  private getEffectiveStream(provider: string): boolean {
    try {
      const cfg = this.config.providers[provider];
      const c = cfg.custom;
      if (c !== undefined) {
        const maybe = (c as { stream?: unknown }).stream;
        if (typeof maybe === 'boolean') return maybe;
      }
    } catch { /* noop */ }
    return this.options.stream;
  }

  private makeTracedFetch(provider: string): typeof fetch {
    const trace = this.options.traceLLM;
    return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      if (!trace) return fetch(input, init);
      const method = (init?.method ?? 'GET').toUpperCase();
      let url: string;
      if (typeof input === 'string') url = input;
      else if (input instanceof URL) url = input.toString();
      else if (typeof (input as { url?: unknown }).url === 'string') url = (input as { url: string }).url;
      else url = 'unknown-url';
      const headers = new Headers(init?.headers);
      const redacted: Record<string, string> = {};
      headers.forEach((v, k) => { redacted[k.toLowerCase()] = k.toLowerCase() === 'authorization' ? 'REDACTED' : v; });
      const bodyStr = typeof init?.body === 'string' ? init.body : '';
      this.callbacks?.onLog?.({ timestamp: Date.now(), severity: 'TRC', turn: 0, subturn: 0, direction: 'request', type: 'llm', remoteIdentifier: provider, fatal: false, message: 'LLM request: ' + method + ' ' + url + ' headers: ' + JSON.stringify(redacted) + ' body: ' + bodyStr });
      const res = await fetch(input, init);
      const headersOut: Record<string, string> = {};
      res.headers.forEach((v, k) => { headersOut[k.toLowerCase()] = k.toLowerCase() === 'authorization' ? 'REDACTED' : v; });
      this.callbacks?.onLog?.({ timestamp: Date.now(), severity: 'TRC', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: provider, fatal: false, message: 'LLM response: ' + String(res.status) + ' ' + res.statusText + ' headers: ' + JSON.stringify(headersOut) });
      return res;
    };
  }
}
