import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { jsonSchema } from '@ai-sdk/provider-utils';
import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import { streamText } from 'ai';
import { createOllama } from 'ollama-ai-provider-v2';

import type { AccountingEntry, AIAgentCallbacks, AIAgentOptions, AIAgentRunOptions, Configuration, ConversationMessage } from './types.js';
import type { SharedV2ProviderOptions } from '@ai-sdk/provider';
import type { LanguageModel, ModelMessage, ToolSet } from 'ai';

import { loadConfiguration, validateMCPServers, validatePrompts, validateProviders } from './config.js';
import { MCPClientManager } from './mcp-client.js';



type ToolArgs = Record<string, unknown>;

export class AIAgent {
  private config: Configuration;
  private mcpClient: MCPClientManager;
  private callbacks: AIAgentCallbacks;
  private options: {
    configPath: string;
    llmTimeout: number;
    toolTimeout: number;
    temperature: number;
    topP: number;
    traceLLM?: boolean;
    traceMCP?: boolean;
    parallelToolCalls?: boolean;
    maxToolTurns: number;
    verbose?: boolean;
  };

  constructor(options: AIAgentOptions = {}) {
    this.config = loadConfiguration(options.configPath);
    this.callbacks = options.callbacks ?? {};
    const defaults = this.config.defaults ?? {};
    this.options = {
      configPath: options.configPath ?? '',
      llmTimeout: options.llmTimeout ?? defaults.llmTimeout ?? 30000,
      toolTimeout: options.toolTimeout ?? defaults.toolTimeout ?? 10000,
      temperature: options.temperature ?? defaults.temperature ?? 0.7,
      topP: options.topP ?? defaults.topP ?? 1.0,
      traceLLM: options.traceLLM ?? false,
      traceMCP: options.traceMCP ?? false,
      verbose: options.verbose ?? false,
      parallelToolCalls: options.parallelToolCalls ?? defaults.parallelToolCalls ?? true,
      maxToolTurns: (options as { maxToolTurns?: number }).maxToolTurns ?? (defaults as { maxToolTurns?: number }).maxToolTurns ?? 30,
    };
    this.mcpClient = new MCPClientManager({ trace: this.options.traceMCP, verbose: this.options.verbose === true, logger: (msg) => { this.log('debug', msg); } });
  }

  async run(runOptions: AIAgentRunOptions): Promise<{ conversation: ConversationMessage[]; success: boolean; error?: string }>
  {
    try {
      validateProviders(this.config, runOptions.providers);
      validateMCPServers(this.config, runOptions.tools);
      validatePrompts(runOptions.systemPrompt, runOptions.userPrompt);

      const systemPrompt = runOptions.systemPrompt;
      const userPrompt = runOptions.userPrompt;

      if (runOptions.dryRun === true) {
        this.log('info', 'Dry run: skipping MCP initialization and LLM requests.');
        return { conversation: [], success: true };
      }

      try {
        const selected = Object.fromEntries(runOptions.tools.map((t) => [t, this.config.mcpServers[t]]));
        await this.mcpClient.initializeServers(selected);
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        this.log('warn', `MCP initialization failed and will be skipped: ${msg}`);
      }

      const enhancedSystemPrompt = this.enhanceSystemPrompt(systemPrompt, this.mcpClient.getCombinedInstructions());
      if (this.options.verbose === true) {
        try { process.stderr.write(`[prompt] created system prompt, size ${enhancedSystemPrompt.length} chars\n`); } catch {}
      }

      const conversation: ConversationMessage[] = [];
      if (Array.isArray(runOptions.conversationHistory) && runOptions.conversationHistory.length > 0) {
        const history = [...runOptions.conversationHistory];
        if (history[0].role === 'system') history[0] = { role: 'system', content: enhancedSystemPrompt };
        else history.unshift({ role: 'system', content: enhancedSystemPrompt });
        conversation.push(...history);
      } else {
        conversation.push({ role: 'system', content: enhancedSystemPrompt });
      }
      conversation.push({ role: 'user', content: userPrompt });

      const llm = await this.callLLMWithTools(conversation, runOptions.providers, runOptions.models);
      if (!llm.success) return { conversation, success: false, error: llm.error };
      conversation.push(...llm.appendMessages);
      return { conversation, success: true };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      this.log('error', `AI Agent failed: ${msg}`);
      return { conversation: [], success: false, error: msg };
    } finally {
      await this.mcpClient.cleanup();
    }
  }

  private async callLLMWithTools(
    conversation: ConversationMessage[],
    providers: string[],
    models: string[],
  ): Promise<{ success: boolean; appendMessages: ConversationMessage[]; error?: string }>
  {
    const mcpTools = this.mcpClient.getAllTools();
    const mapping = this.mcpClient.getToolServerMapping();
    const executedTools: { name: string; output: string }[] = [];

    const sdkTools = Object.fromEntries(
      mcpTools.map((t) => [
        t.name,
        {
          description: t.description,
          inputSchema: jsonSchema(t.inputSchema),
          execute: async (args: ToolArgs) => {
            const serverName = mapping.get(t.name);
            if (serverName == null || serverName === '') return '';
            const started = Date.now();
            const call = { id: `${Date.now().toString()}-${Math.random().toString(36).slice(2)}`, name: t.name, parameters: args };
            const res = await this.mcpClient.executeTool(serverName, call, this.options.toolTimeout);
            this.callbacks.onAccounting?.({
              type: 'tool', timestamp: Date.now(), status: res.success ? 'ok' : 'failed', latency: Date.now() - started,
              mcpServer: serverName, command: t.name, charactersIn: JSON.stringify(args).length, charactersOut: res.result.length, error: res.error,
            } as AccountingEntry);
            if (res.success && res.result.length > 0) executedTools.push({ name: t.name, output: res.result });
            // Verbose aggregation
            mcpRequests += 1;
            mcpPerServer.set(serverName, (mcpPerServer.get(serverName) ?? 0) + 1);
            return res.result
          },
          toModelOutput: (output: unknown) => ({ type: 'text', value: typeof output === 'string' ? output : JSON.stringify(output) }),
        },
      ] as const)
    ) as Record<string, {
      description: string;
      inputSchema: unknown;
      execute: (args: ToolArgs) => Promise<string>;
      toModelOutput: (output: unknown) => { type: 'text'; value: string };
    }>;

    const pairs = models.flatMap((model) => providers.map((provider) => ({ provider, model })));
    // Verbose aggregators (final [fin] sums across entire run)
    let llmRequests = 0;
    let aggInput = 0;
    let aggOutput = 0;
    let aggCached = 0;
    let aggAssistantChars = 0;
    let aggLLMLatency = 0;
    let aggToolCalls = 0;
    let mcpRequests = 0;
    const mcpPerServer = new Map<string, number>();

    const drainTextStream = async (it: AsyncIterable<string>): Promise<{ sawAny: boolean; totalChars: number }> => {
      const iterator = it[Symbol.asyncIterator]();
      let sawAny = false;
      let totalChars = 0;
      let lastChar: string | undefined = undefined;
      const step = async (): Promise<void> => {
        const next = await iterator.next();
        if (next.done === true) {
          if (sawAny && lastChar !== '\n') this.output('\n');
          return;
        }
        const chunk: string = next.value ?? '';
        if (chunk.length > 0) {
          sawAny = true;
          lastChar = chunk[chunk.length - 1];
          totalChars += chunk.length;
        }
        this.output(chunk);
        await step();
      };
      await step();
      return { sawAny, totalChars };
    };

    const tryPair = async (idx: number): Promise<{ success: boolean; appendMessages: ConversationMessage[]; error?: string }> => {
      if (idx >= pairs.length) return { success: false, appendMessages: [], error: 'All providers and models failed' };
      const pair = pairs[idx] as { provider: string; model: string };
      const provider = pair.provider;
      const model = pair.model;
      try {
        // Lower verbosity: only log provider/model attempts when verbose is enabled
        this.log('debug', `Trying ${provider}/${model}`);
        const llmProvider = this.getLLMProvider(provider);
const baseMessages: ModelMessage[] = conversation
  .filter((m) => m.role === 'system' || m.role === 'user' || m.role === 'assistant')
  .map((m) => ({ role: m.role as 'system' | 'user' | 'assistant', content: m.content }));
let currentMessages: ModelMessage[] = baseMessages;
// Build providerOptions per provider
let providerOptionsDecl: SharedV2ProviderOptions | undefined = undefined;
if (provider === 'openai' || provider === 'openrouter') {
  if (this.options.parallelToolCalls !== undefined) {
    providerOptionsDecl = { openai: { parallelToolCalls: this.options.parallelToolCalls } } as SharedV2ProviderOptions;
  }
} else if (provider === 'ollama') {
  try {
    const providerCfg = this.config.providers[provider] as { custom?: unknown } | undefined;
    const custom = (providerCfg?.custom ?? {}) as Record<string, unknown>;
    const po = custom.providerOptions as SharedV2ProviderOptions | undefined;
    if (po != null) providerOptionsDecl = po;
  } catch { /* ignore */ }
}

const runTurn = async (turn: number, msgs: ModelMessage[]): Promise<ConversationMessage[]> => {
  if (turn >= this.options.maxToolTurns) return [];
  executedTools.length = 0;
  const iterationStart = Date.now();
  if (this.options.verbose === true) {
    try {
      const chars = msgs.reduce((acc, m) => acc + (typeof m.content === 'string' ? m.content.length : JSON.stringify(m.content).length), 0);
      process.stderr.write(`[llm] req: ${provider}, ${model}, messages ${msgs.length}, ${chars} chars\n`);
      llmRequests += 1; // count all attempts, even if they fail later
    } catch {}
  }
  const result = streamText({
    model: llmProvider(model),
    messages: msgs,
    tools: sdkTools as unknown as ToolSet,
    temperature: this.options.temperature,
    topP: this.options.topP,
    providerOptions: providerOptionsDecl,
    abortSignal: AbortSignal.timeout(this.options.llmTimeout),
  });
  const streamInfo = await drainTextStream(result.textStream);
  const usageData: { inputTokens?: number; outputTokens?: number; totalTokens?: number; cachedTokens?: number } = await result.usage;
  const inputTokens = usageData.inputTokens ?? 0;
  const outputTokens = usageData.outputTokens ?? 0;
  const totalTokens = usageData.totalTokens ?? (inputTokens + outputTokens);
  const cachedTokens = usageData.cachedTokens;
  this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'ok', latency: Date.now() - iterationStart, provider, model, tokens: { inputTokens, outputTokens, totalTokens, cachedTokens } } as AccountingEntry);
  if (this.options.verbose === true) {
    try {
      // aggregate totals
      aggInput += inputTokens;
      aggOutput += outputTokens;
      aggCached += cachedTokens ?? 0;
      aggLLMLatency += (Date.now() - iterationStart);
    } catch {}
  }
  let resp: Awaited<typeof result.response>;
  try {
    resp = await result.response;
  } catch (err) {
    const name = err instanceof Error ? err.name : 'Error';
    const msg = err instanceof Error ? err.message : String(err);
    const hint = this.options.traceLLM === true ? '' : ' Enable --trace-llm to see raw HTTP/SSE.';
    const details = `streamed=${streamInfo.sawAny} (${streamInfo.totalChars} chars), tools=${executedTools.length}, tokens in=${inputTokens}, out=${outputTokens}, cached=${cachedTokens ?? 0}.`;
    const wrapped = `${name}: ${msg}.${hint} Details: ${details}`;
    throw new Error(wrapped);
  }
  const turnMessages: ConversationMessage[] = Array.isArray(resp.messages)
    ? (resp.messages as { role: ConversationMessage['role']; content: unknown }[]).map((m) => ({ role: m.role, content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content), metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens }, timestamp: Date.now() } }))
    : [];
  if (this.options.verbose === true) {
    try {
      const latency = Date.now() - iterationStart;
      const size = turnMessages.reduce((acc, m) => acc + m.content.length, 0);
      process.stderr.write(`[llm] res: input ${inputTokens}, output ${outputTokens}, cached ${cachedTokens ?? 0} tokens, tools ${executedTools.length}, latency ${latency} ms, size ${size} chars\n`);
      aggAssistantChars += size;
      aggToolCalls += executedTools.length;
    } catch {}
  }

  // If the model executed tools (tool-first response), continue the turn with tool outputs
  if (executedTools.length > 0) {
    const toolText = executedTools.map((x) => `- ${x.name}:\n${x.output}`).join('\n\n');
    const nextMsgs = msgs.concat({ role: 'user', content: `Using only the following tool results, continue and answer the user's request.\n\nTOOL RESULTS:\n\n${toolText}` } as ModelMessage);
    return runTurn(turn + 1, nextMsgs);
  }

  // If there is assistant text, return it; otherwise consider it a failure
  if (turnMessages.length > 0) return turnMessages;
  throw new Error('Empty response from model');
};
const finalAppend = await runTurn(0, currentMessages);
if (finalAppend.length === 0) {
  return { success: false, appendMessages: [], error: 'Max tool turns exceeded' };
}
return { success: true, appendMessages: finalAppend };


      } catch (err) {
        const em = err instanceof Error ? err.message : String(err ?? 'Unknown error');
        const en = err instanceof Error ? err.name : 'Error';
        const stackTop = err instanceof Error && typeof err.stack === 'string' ? err.stack.split('\n')[1]?.trim() : undefined;
        const extra = this.options.verbose === true && stackTop ? ` (${stackTop})` : '';
        const advice = this.options.traceLLM === true ? '' : ' Hint: run with --trace-llm to inspect HTTP/SSE.';
        this.log('warn', `${provider}/${model} failed: [${en}] ${em}${extra}${advice}`);
        this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'failed', latency: 0, provider, model, tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 }, error: em } as AccountingEntry);
        return tryPair(idx + 1);
      }
    };

    const outcome = await tryPair(0);
    if (this.options.verbose === true) {
      try {
        const mcpSummary = Array.from(mcpPerServer.entries()).map(([srv, cnt]) => `${srv} ${cnt}`).join(', ');
        process.stderr.write(`[fin] finally: llm requests ${llmRequests} (tokens: ${aggInput} in, ${aggOutput} out, ${aggCached} cached, tool-calls ${aggToolCalls}, output-size ${aggAssistantChars} chars, latency-sum ${aggLLMLatency} ms), mcp requests ${mcpRequests}${mcpSummary.length > 0 ? ` (${mcpSummary})` : ''}\n`);
      } catch {}
    }
    return outcome;
  }

  private getLLMProvider(providerName: string): (model: string) => LanguageModel {
    const cfg = this.config.providers[providerName];
    const tracedFetch = this.options.traceLLM === true
      ? async (input: RequestInfo | URL, init?: RequestInit) => {
          let requestInit: RequestInit | undefined;
          try {
            let url: string;
            if (typeof input === 'string') url = input;
            else if (input instanceof URL) url = input.toString();
            else if (typeof (input as { url?: unknown }).url === 'string') url = (input as { url?: string }).url ?? '';
            else url = '';
            const method = init?.method ?? 'GET';
            const headersObj = (init?.headers ?? {}) as unknown;
            const entries = typeof headersObj === 'object' && headersObj !== null ? Object.entries(headersObj as Record<string, unknown>) : [];
            const headersSend = entries.reduce<Record<string, string>>((acc, [k, v]) => {
              if (typeof v === 'string') acc[k] = v;
              else if (typeof v === 'number' || typeof v === 'boolean') acc[k] = String(v);
              return acc;
            }, {});
            if (!('Accept' in headersSend) && !('accept' in headersSend)) headersSend.Accept = 'application/json';
            if (url.includes('openrouter.ai')) {
              const defaultReferer = 'https://ai-agent.local';
              const defaultTitle = 'ai-agent-codex';
              if (!('HTTP-Referer' in headersSend) && !('http-referer' in headersSend)) headersSend['HTTP-Referer'] = defaultReferer;
              if (!('X-OpenRouter-Title' in headersSend) && !('x-openrouter-title' in headersSend)) headersSend['X-OpenRouter-Title'] = defaultTitle;
              if (!('X-Title' in headersSend) && !('x-title' in headersSend)) headersSend['X-Title'] = defaultTitle;
              if (!('User-Agent' in headersSend) && !('user-agent' in headersSend)) headersSend['User-Agent'] = `${defaultTitle}/1.0`;
            }
            requestInit = { ...(init ?? {}), headers: headersSend };
                        const isOpenRouter = typeof url === 'string' && url.includes('openrouter.ai')

            const cfgAny = cfg as unknown as { custom?: Record<string, unknown>; mergeStrategy?: 'overlay'|'override'|'deep' };
            if (isOpenRouter && cfgAny.custom != null && typeof requestInit.body === 'string') {
              try {
                const bodyObj = JSON.parse(requestInit.body) as Record<string, unknown>;
                const merged = (function mergeCustom(base: Record<string, unknown>, custom: Record<string, unknown>, strategy: 'overlay'|'override'|'deep') {
                  const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
                  if (strategy === 'override') return { ...base, ...custom };
                  if (strategy === 'overlay') {
                    const out = Object.entries(custom).reduce<Record<string, unknown>>((acc, [k, v]) => { if (!(k in acc)) acc[k] = v; return acc; }, { ...base });
                    return out;
                  }
                  const deepMerge = (a: Record<string, unknown>, b: Record<string, unknown>): Record<string, unknown> =>
                    Object.entries(b).reduce<Record<string, unknown>>((acc, [k, v]) => {
                      const av = acc[k];
                      acc[k] = (isObj(av) && isObj(v)) ? deepMerge(av, v) : v;
                      return acc;
                    }, { ...a });
                  return deepMerge(base, custom);
                })(bodyObj, cfgAny.custom, cfgAny.mergeStrategy ?? 'overlay');
                requestInit.body = JSON.stringify(merged);
              } catch {}
            }

            const headersRaw: Record<string, string> = Object.fromEntries(
              Object.entries(headersSend).map(([k, v]) => [k.toLowerCase(), v])
            );
            if (Object.prototype.hasOwnProperty.call(headersRaw, 'authorization')) headersRaw.authorization = 'REDACTED';
            const headersPretty = JSON.stringify(headersRaw, null, 2);
            const bodyString = typeof (requestInit.body) === 'string' ? requestInit.body : undefined;
            let bodyPretty = bodyString;
            if (bodyString !== undefined) { try { bodyPretty = JSON.stringify(JSON.parse(bodyString), null, 2); } catch { /* noop */ } }
            this.log('debug', `LLM request: ${method} ${url}\nheaders: ${headersPretty}${bodyPretty !== undefined ? `\nbody: ${bodyPretty}` : ''}`);
          } catch { /* noop */ }
          const res = await fetch(input, requestInit ?? init);
          try {
            const ct = res.headers.get('content-type') ?? '';
            const headersOut: Record<string, string> = {};
            res.headers.forEach((v, k) => { headersOut[k.toLowerCase()] = k.toLowerCase() === 'authorization' ? 'REDACTED' : v; });
            const headersOutPretty = JSON.stringify(headersOut, null, 2);
            const baseResp = `LLM response: ${res.status.toString()} ${res.statusText}\nheaders: ${headersOutPretty}`;
            if (ct.includes('application/json')) {
              const clone = res.clone();
              let txt = await clone.text();
              try { txt = JSON.stringify(JSON.parse(txt), null, 2); } catch { /* noop */ }
              this.log('debug', `${baseResp}\nbody: ${txt}`);
            } else if (ct.includes('text/event-stream')) {
              try {
                const clone = res.clone();
                try { const raw = await clone.text(); this.log('debug', `${baseResp}\nraw-sse: ${raw}`); } catch { this.log('debug', `${baseResp}\ncontent-type: ${ct}`); }
              } catch {
                this.log('debug', `${baseResp}\ncontent-type: ${ct}`);
              }
            } else {
              this.log('debug', `${baseResp}\ncontent-type: ${ct}`);
            }
          } catch { /* noop */ }
          return res;
        }
      : undefined;

    switch (providerName) {
      case 'openai': {
        const prov = createOpenAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => (prov as unknown as { responses: (m: string) => import('ai').LanguageModel }).responses(model);
      }
      case 'anthropic': {
        const prov = createAnthropic({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      case 'google':
      case 'vertex': {
        const prov = createGoogleGenerativeAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      case 'openrouter': {
        const prov = createOpenRouter({
          apiKey: cfg.apiKey,
          fetch: tracedFetch as typeof fetch,
          headers: {
            'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
            'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-codex',
            'User-Agent': 'ai-agent-codex/1.0',
            ...(cfg.headers ?? {}),
          },
        });
        return (model: string) => prov(model);
      }
      case 'ollama': {
        // Use community AI SDK 5 provider for Ollama (native API under /api)
        // Normalize base URL: users may specify OpenAI-compatible '/v1'; convert to native '/api'.
        const normalizeBaseUrl = (u?: string): string => {
          const def = 'http://localhost:11434/api';
          if (!u || typeof u !== 'string') return def;
          try {
            let v = u.replace(/\/$/, '');
            // Replace trailing /v1 with /api
            if (/\/v1\/?$/.test(v)) return v.replace(/\/v1\/?$/, '/api');
            // If already ends with /api, keep as-is
            if (/\/api\/?$/.test(v)) return v;
            // Otherwise, append /api
            return v + '/api';
          } catch {
            return def;
          }
        };
        const prov = createOllama({ baseURL: normalizeBaseUrl(cfg.baseUrl), fetch: tracedFetch as typeof fetch });
        return (model: string) => prov(model);
      }
      default:
        throw new Error(`Unsupported provider: ${providerName}`);
    }
  }

  private enhanceSystemPrompt(systemPrompt: string, mcpInstructions: string): string {
    if (mcpInstructions.trim().length === 0) return systemPrompt;
    return `${systemPrompt}\n\n## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`;
  }

  private log(level: 'debug' | 'info' | 'warn' | 'error', message: string): void {
    this.callbacks.onLog?.(level, message);
  }

  private output(text: string): void {
    this.callbacks.onOutput?.(text);
  }
}
