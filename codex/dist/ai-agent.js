import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { jsonSchema } from '@ai-sdk/provider-utils';
import { streamText } from 'ai';
import { loadConfiguration, validateMCPServers, validatePrompts, validateProviders } from './config.js';
import { MCPClientManager } from './mcp-client.js';
export class AIAgent {
    config;
    mcpClient;
    callbacks;
    options;
    constructor(options = {}) {
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
            parallelToolCalls: options.parallelToolCalls ?? defaults.parallelToolCalls ?? true,
            maxToolTurns: options.maxToolTurns ?? defaults.maxToolTurns ?? 30,
        };
        this.mcpClient = new MCPClientManager({ trace: this.options.traceMCP, logger: (msg) => { this.log('debug', msg); } });
    }
    async run(runOptions) {
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
            }
            catch (e) {
                const msg = e instanceof Error ? e.message : String(e);
                this.log('warn', `MCP initialization failed and will be skipped: ${msg}`);
            }
            const enhancedSystemPrompt = this.enhanceSystemPrompt(systemPrompt, this.mcpClient.getCombinedInstructions());
            const conversation = [];
            if (Array.isArray(runOptions.conversationHistory) && runOptions.conversationHistory.length > 0) {
                const history = [...runOptions.conversationHistory];
                if (history[0].role === 'system')
                    history[0] = { role: 'system', content: enhancedSystemPrompt };
                else
                    history.unshift({ role: 'system', content: enhancedSystemPrompt });
                conversation.push(...history);
            }
            else {
                conversation.push({ role: 'system', content: enhancedSystemPrompt });
            }
            conversation.push({ role: 'user', content: userPrompt });
            const llm = await this.callLLMWithTools(conversation, runOptions.providers, runOptions.models);
            if (!llm.success)
                return { conversation, success: false, error: llm.error };
            conversation.push(...llm.appendMessages);
            return { conversation, success: true };
        }
        catch (error) {
            const msg = error instanceof Error ? error.message : 'Unknown error';
            this.log('error', `AI Agent failed: ${msg}`);
            return { conversation: [], success: false, error: msg };
        }
        finally {
            await this.mcpClient.cleanup();
        }
    }
    async callLLMWithTools(conversation, providers, models) {
        const mcpTools = this.mcpClient.getAllTools();
        const mapping = this.mcpClient.getToolServerMapping();
        const executedTools = [];
        const sdkTools = Object.fromEntries(mcpTools.map((t) => [
            t.name,
            {
                description: t.description,
                inputSchema: jsonSchema(t.inputSchema),
                execute: async (args) => {
                    const serverName = mapping.get(t.name);
                    if (serverName == null || serverName === '')
                        return '';
                    const started = Date.now();
                    const call = { id: `${Date.now().toString()}-${Math.random().toString(36).slice(2)}`, name: t.name, parameters: args };
                    const res = await this.mcpClient.executeTool(serverName, call, this.options.toolTimeout);
                    this.callbacks.onAccounting?.({
                        type: 'tool', timestamp: Date.now(), status: res.success ? 'ok' : 'failed', latency: Date.now() - started,
                        mcpServer: serverName, command: t.name, charactersIn: JSON.stringify(args).length, charactersOut: res.result.length, error: res.error,
                    });
                    if (res.success && res.result.length > 0)
                        executedTools.push({ name: t.name, output: res.result });
                    return res.result;
                },
                toModelOutput: (output) => ({ type: 'text', value: typeof output === 'string' ? output : JSON.stringify(output) }),
            },
        ]));
        const pairs = models.flatMap((model) => providers.map((provider) => ({ provider, model })));
        const drainTextStream = async (it) => {
            const iterator = it[Symbol.asyncIterator]();
            const step = async () => {
                const next = await iterator.next();
                if (next.done === true)
                    return;
                this.output(next.value);
                await step();
            };
            await step();
        };
        const tryPair = async (idx) => {
            if (idx >= pairs.length)
                return { success: false, appendMessages: [], error: 'All providers and models failed' };
            const pair = pairs[idx];
            const provider = pair.provider;
            const model = pair.model;
            try {
                this.log('info', `Trying ${provider}/${model}`);
                const llmProvider = this.getLLMProvider(provider);
                const baseMessages = conversation
                    .filter((m) => m.role === 'system' || m.role === 'user' || m.role === 'assistant')
                    .map((m) => ({ role: m.role, content: m.content }));
                let currentMessages = baseMessages;
                const providerOptionsDecl = this.options.parallelToolCalls !== undefined && (provider === 'openai' || provider === 'openrouter')
                    ? { openai: { parallelToolCalls: this.options.parallelToolCalls } }
                    : undefined;
                const runTurn = async (turn, msgs) => {
                    if (turn >= 16)
                        return [];
                    executedTools.length = 0;
                    const iterationStart = Date.now();
                    const result = streamText({
                        model: llmProvider(model),
                        messages: msgs,
                        tools: sdkTools,
                        temperature: this.options.temperature,
                        topP: this.options.topP,
                        providerOptions: providerOptionsDecl,
                        abortSignal: AbortSignal.timeout(this.options.llmTimeout),
                    });
                    await drainTextStream(result.textStream);
                    const usageData = await result.usage;
                    const inputTokens = usageData.inputTokens ?? 0;
                    const outputTokens = usageData.outputTokens ?? 0;
                    const totalTokens = usageData.totalTokens ?? (inputTokens + outputTokens);
                    const cachedTokens = usageData.cachedTokens;
                    this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'ok', latency: Date.now() - iterationStart, provider, model, tokens: { inputTokens, outputTokens, totalTokens, cachedTokens } });
                    const resp = await result.response;
                    const turnMessages = Array.isArray(resp.messages)
                        ? resp.messages.map((m) => ({ role: m.role, content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content), metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens }, timestamp: Date.now() } }))
                        : [];
                    if (executedTools.length === 0)
                        return turnMessages;
                    const toolText = executedTools.map((x) => `- ${x.name}:\n${x.output}`).join('\n\n');
                    const nextMsgs = msgs.concat({ role: 'user', content: `Using only the following tool results, continue and answer the user's request.\n\nTOOL RESULTS:\n\n${toolText}` });
                    return runTurn(turn + 1, nextMsgs);
                };
                const finalAppend = await runTurn(0, currentMessages);
                if (finalAppend.length === 0) {
                    return { success: false, appendMessages: [], error: 'Max tool turns exceeded' };
                }
                return { success: true, appendMessages: finalAppend };
            }
            catch (err) {
                const em = err instanceof Error ? err.message : 'Unknown error';
                this.log('warn', `${provider}/${model} failed: ${em}`);
                this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'failed', latency: 0, provider, model, tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 }, error: em });
                return tryPair(idx + 1);
            }
        };
        return tryPair(0);
    }
    getLLMProvider(providerName) {
        const cfg = this.config.providers[providerName];
        const tracedFetch = this.options.traceLLM === true
            ? async (input, init) => {
                let requestInit;
                try {
                    let url;
                    if (typeof input === 'string')
                        url = input;
                    else if (input instanceof URL)
                        url = input.toString();
                    else if (typeof input.url === 'string')
                        url = input.url ?? '';
                    else
                        url = '';
                    const method = init?.method ?? 'GET';
                    const headersObj = (init?.headers ?? {});
                    const entries = typeof headersObj === 'object' && headersObj !== null ? Object.entries(headersObj) : [];
                    const headersSend = entries.reduce((acc, [k, v]) => {
                        if (typeof v === 'string')
                            acc[k] = v;
                        else if (typeof v === 'number' || typeof v === 'boolean')
                            acc[k] = String(v);
                        return acc;
                    }, {});
                    if (!('Accept' in headersSend) && !('accept' in headersSend))
                        headersSend.Accept = 'application/json';
                    if (url.includes('openrouter.ai')) {
                        const defaultReferer = 'https://ai-agent.local';
                        const defaultTitle = 'ai-agent-codex';
                        if (!('HTTP-Referer' in headersSend) && !('http-referer' in headersSend))
                            headersSend['HTTP-Referer'] = defaultReferer;
                        if (!('X-OpenRouter-Title' in headersSend) && !('x-openrouter-title' in headersSend))
                            headersSend['X-OpenRouter-Title'] = defaultTitle;
                        if (!('X-Title' in headersSend) && !('x-title' in headersSend))
                            headersSend['X-Title'] = defaultTitle;
                        if (!('User-Agent' in headersSend) && !('user-agent' in headersSend))
                            headersSend['User-Agent'] = `${defaultTitle}/1.0`;
                    }
                    requestInit = { ...(init ?? {}), headers: headersSend };
                    const headersRaw = Object.fromEntries(Object.entries(headersSend).map(([k, v]) => [k.toLowerCase(), v]));
                    if (Object.prototype.hasOwnProperty.call(headersRaw, 'authorization'))
                        headersRaw.authorization = 'REDACTED';
                    const headersPretty = JSON.stringify(headersRaw, null, 2);
                    const bodyString = typeof (requestInit.body) === 'string' ? requestInit.body : undefined;
                    let bodyPretty = bodyString;
                    if (bodyString !== undefined) {
                        try {
                            bodyPretty = JSON.stringify(JSON.parse(bodyString), null, 2);
                        }
                        catch { /* noop */ }
                    }
                    this.log('debug', `LLM request: ${method} ${url}\nheaders: ${headersPretty}${bodyPretty !== undefined ? `\nbody: ${bodyPretty}` : ''}`);
                }
                catch { /* noop */ }
                const res = await fetch(input, requestInit ?? init);
                try {
                    const ct = res.headers.get('content-type') ?? '';
                    const headersOut = {};
                    res.headers.forEach((v, k) => { headersOut[k.toLowerCase()] = k.toLowerCase() === 'authorization' ? 'REDACTED' : v; });
                    const headersOutPretty = JSON.stringify(headersOut, null, 2);
                    const baseResp = `LLM response: ${res.status.toString()} ${res.statusText}\nheaders: ${headersOutPretty}`;
                    if (ct.includes('application/json')) {
                        const clone = res.clone();
                        let txt = await clone.text();
                        try {
                            txt = JSON.stringify(JSON.parse(txt), null, 2);
                        }
                        catch { /* noop */ }
                        this.log('debug', `${baseResp}\nbody: ${txt}`);
                    }
                    else if (ct.includes('text/event-stream')) {
                        try {
                            const clone = res.clone();
                            try {
                                const raw = await clone.text();
                                this.log('debug', `${baseResp}\nraw-sse: ${raw}`);
                            }
                            catch {
                                this.log('debug', `${baseResp}\ncontent-type: ${ct}`);
                            }
                        }
                        catch {
                            this.log('debug', `${baseResp}\ncontent-type: ${ct}`);
                        }
                    }
                    else {
                        this.log('debug', `${baseResp}\ncontent-type: ${ct}`);
                    }
                }
                catch { /* noop */ }
                return res;
            }
            : undefined;
        switch (providerName) {
            case 'openai': {
                const prov = createOpenAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
                return (model) => prov(model);
            }
            case 'anthropic': {
                const prov = createAnthropic({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
                return (model) => prov(model);
            }
            case 'google':
            case 'vertex': {
                const prov = createGoogleGenerativeAI({ apiKey: cfg.apiKey, baseURL: cfg.baseUrl, fetch: tracedFetch });
                return (model) => prov(model);
            }
            case 'openrouter': {
                const prov = createOpenAI({
                    apiKey: cfg.apiKey,
                    baseURL: cfg.baseUrl ?? 'https://openrouter.ai/api/v1',
                    fetch: tracedFetch,
                    headers: {
                        Accept: 'application/json',
                        'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
                        'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-codex',
                        'User-Agent': 'ai-agent-codex/1.0',
                    },
                    name: 'openrouter',
                });
                return (model) => prov.chat(model);
            }
            case 'ollama': {
                const prov = createOpenAI({ apiKey: cfg.apiKey ?? 'ollama', baseURL: cfg.baseUrl ?? 'http://localhost:11434/v1', fetch: tracedFetch });
                return (model) => prov(model);
            }
            default:
                throw new Error(`Unsupported provider: ${providerName}`);
        }
    }
    enhanceSystemPrompt(systemPrompt, mcpInstructions) {
        if (mcpInstructions.trim().length === 0)
            return systemPrompt;
        return `${systemPrompt}\n\n## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`;
    }
    log(level, message) {
        this.callbacks.onLog?.(level, message);
    }
    output(text) {
        this.callbacks.onOutput?.(text);
    }
}
