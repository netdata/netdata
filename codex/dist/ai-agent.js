import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { jsonSchema } from '@ai-sdk/provider-utils';
import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import { streamText, generateText } from 'ai';
import { createOllama } from 'ollama-ai-provider-v2';
import { loadConfiguration, validateMCPServers, validatePrompts, validateProviders } from './config.js';
import { MCPClientManager } from './mcp-client.js';
export class AIAgent {
    config;
    mcpClient;
    callbacks;
    currentTurn = 0;
    toolSeq = 0;
    options;
    colorVerbose(text) {
        // Darker grey for verbose
        return process.stderr.isTTY ? `\x1b[90m${text}\x1b[0m` : text;
    }
    colorThinking(text) {
        // Lighter grey for thinking
        return process.stderr.isTTY ? `\x1b[37m${text}\x1b[0m` : text;
    }
    constructor(options = {}) {
        this.config = loadConfiguration(options.configPath);
        this.callbacks = options.callbacks ?? {};
        const defaults = this.config.defaults ?? {};
        this.options = {
            configPath: options.configPath ?? '',
            llmTimeout: options.llmTimeout ?? defaults.llmTimeout ?? 120000,
            toolTimeout: options.toolTimeout ?? defaults.toolTimeout ?? 60000,
            temperature: options.temperature ?? defaults.temperature ?? 0.7,
            topP: options.topP ?? defaults.topP ?? 1.0,
            traceLLM: options.traceLLM ?? false,
            traceMCP: options.traceMCP ?? false,
            verbose: options.verbose ?? false,
            stream: options.stream ?? defaults.stream ?? true,
            parallelToolCalls: options.parallelToolCalls ?? defaults.parallelToolCalls ?? true,
            maxToolTurns: options.maxToolTurns ?? defaults.maxToolTurns ?? 30,
            maxRetries: options.maxRetries ?? defaults.maxRetries ?? 3,
        };
        // We handle verbose MCP req/res lines ourselves; keep trace as-is.
        this.mcpClient = new MCPClientManager({ trace: this.options.traceMCP, verbose: false, logger: (msg) => { this.log('debug', msg); } });
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
            if (this.options.verbose === true) {
                try {
                    process.stderr.write(this.colorVerbose(`[prompt] created system prompt, size ${enhancedSystemPrompt.length} chars\n`));
                }
                catch { }
            }
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
                    if (this.options.verbose === true) {
                        try {
                            const fmtVal = (v) => {
                                if (v === null || v === undefined)
                                    return String(v);
                                const tpe = typeof v;
                                if (tpe === 'string') {
                                    const s = v;
                                    return s.length > 80 ? s.slice(0, 80) + '…' : s;
                                }
                                if (tpe === 'number' || tpe === 'boolean')
                                    return String(v);
                                if (Array.isArray(v))
                                    return `[${v.length}]`;
                                return '{…}';
                            };
                            const entries = Object.entries(args ?? {});
                            const paramStr = entries.length > 0
                                ? '(' + entries.map(([k, v]) => `${k}:${fmtVal(v)}`).join(', ') + ')'
                                : '()';
                            this.toolSeq += 1;
                            process.stderr.write(this.colorVerbose(`agent → mcp [${this.currentTurn + 1}.${this.toolSeq}] ${serverName}, ${t.name}${paramStr}\n`));
                        }
                        catch { }
                    }
                    const res = await this.mcpClient.executeTool(serverName, call, this.options.toolTimeout);
                    this.callbacks.onAccounting?.({
                        type: 'tool', timestamp: Date.now(), status: res.success ? 'ok' : 'failed', latency: Date.now() - started,
                        mcpServer: serverName, command: t.name, charactersIn: JSON.stringify(args).length, charactersOut: res.result.length, error: res.error,
                    });
                    if (this.options.verbose === true) {
                        try {
                            process.stderr.write(this.colorVerbose(`agent ← mcp [${this.currentTurn + 1}.${this.toolSeq}] ${serverName}, ${t.name}, latency ${Date.now() - started} ms, size ${res.result.length} chars\n`));
                        }
                        catch { }
                    }
                    if (res.success && res.result.length > 0)
                        executedTools.push({ name: t.name, output: res.result });
                    // Verbose aggregation
                    mcpRequests += 1;
                    mcpPerServer.set(serverName, (mcpPerServer.get(serverName) ?? 0) + 1);
                    return res.result;
                },
                toModelOutput: (output) => ({ type: 'text', value: typeof output === 'string' ? output : JSON.stringify(output) }),
            },
        ]));
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
        const mcpPerServer = new Map();
        const drainTextStream = async (it, onChunk) => {
            const iterator = it[Symbol.asyncIterator]();
            let sawAny = false;
            let totalChars = 0;
            let lastChar = undefined;
            const step = async () => {
                const next = await iterator.next();
                if (next.done === true) {
                    if (sawAny && lastChar !== '\n')
                        this.output('\n');
                    return;
                }
                const chunk = next.value ?? '';
                if (chunk.length > 0) {
                    sawAny = true;
                    lastChar = chunk[chunk.length - 1];
                    totalChars += chunk.length;
                    try {
                        onChunk?.(chunk.length);
                    }
                    catch { }
                }
                this.output(chunk);
                await step();
            };
            await step();
            return { sawAny, totalChars };
        };
        const tryPair = async (idx) => {
            const pair = pairs[idx];
            const provider = pair.provider;
            const model = pair.model;
            const attemptStart = Date.now();
            let lastLLMCallStart = 0;
            try {
                // Attempt provider/model (debug log removed per request)
                const llmProvider = this.getLLMProvider(provider);
                const baseMessages = conversation
                    .filter((m) => m.role === 'system' || m.role === 'user' || m.role === 'assistant')
                    .map((m) => ({ role: m.role, content: m.content }));
                let currentMessages = baseMessages;
                // Build providerOptions per provider
                let providerOptionsDecl = undefined;
                if (provider === 'openai' || provider === 'openrouter') {
                    if (this.options.parallelToolCalls !== undefined) {
                        providerOptionsDecl = { openai: { parallelToolCalls: this.options.parallelToolCalls } };
                    }
                }
                else if (provider === 'ollama') {
                    try {
                        const providerCfg = this.config.providers[provider];
                        const custom = (providerCfg?.custom ?? {});
                        const po = custom.providerOptions;
                        if (po != null)
                            providerOptionsDecl = po;
                    }
                    catch { /* ignore */ }
                }
                const runTurn = async (turn, msgs) => {
                    if (turn >= this.options.maxToolTurns)
                        return [];
                    executedTools.length = 0;
                    const iterationStart = Date.now();
                    let resLogged = false;
                    this.currentTurn = turn;
                    this.toolSeq = 0;
                    if (this.options.verbose === true) {
                        try {
                            const chars = msgs.reduce((acc, m) => acc + (typeof m.content === 'string' ? m.content.length : JSON.stringify(m.content).length), 0);
                            process.stderr.write(this.colorVerbose(`agent → llm [${turn + 1}] ${provider}, ${model}, messages ${msgs.length}, ${chars} chars\n`));
                            llmRequests += 1; // count all attempts, even if they fail later
                        }
                        catch { }
                    }
                    // Determine effective streaming for this provider
                    const providerCfgStream = (() => {
                        try {
                            const cfgp = this.config.providers[provider];
                            const v = cfgp?.custom?.stream;
                            return typeof v === 'boolean' ? v : undefined;
                        }
                        catch {
                            return undefined;
                        }
                    })();
                    const effectiveStream = typeof providerCfgStream === 'boolean' ? providerCfgStream : (typeof this.options.stream === 'boolean' ? this.options.stream : true);
                    let inputTokens = 0;
                    let outputTokens = 0;
                    let cachedTokens = undefined;
                    let turnMessages = [];
                    const isFinalTurn = turn >= (this.options.maxToolTurns - 1);
                    const msgsForThisCall = isFinalTurn
                        ? msgs.concat({ role: 'user', content: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate" })
                        : msgs;
                    if (effectiveStream) {
                        // Inactivity timeout: reset the timer on each streamed chunk
                        const ac = new AbortController();
                        let idleTimer;
                        const resetIdle = () => {
                            try {
                                if (idleTimer)
                                    clearTimeout(idleTimer);
                            }
                            catch { }
                            idleTimer = setTimeout(() => { try {
                                ac.abort();
                            }
                            catch { } }, this.options.llmTimeout);
                        };
                        const clearIdle = () => { try {
                            if (idleTimer) {
                                clearTimeout(idleTimer);
                                idleTimer = undefined;
                            }
                        }
                        catch { } };
                        let streamInfo = { sawAny: false, totalChars: 0 };
                        let result;
                        try {
                            resetIdle();
                            lastLLMCallStart = Date.now();
                            const stOpts = {
                                model: llmProvider(model),
                                messages: msgsForThisCall,
                                tools: isFinalTurn ? undefined : sdkTools,
                                temperature: this.options.temperature,
                                topP: this.options.topP,
                                providerOptions: providerOptionsDecl,
                                abortSignal: ac.signal,
                            };
                            const openReasoning = new Set();
                            stOpts.onChunk = (ev) => {
                                try {
                                    const obj = ev;
                                    const p = obj?.chunk;
                                    if (p?.type === 'reasoning-start') {
                                        const id = p.id ?? '0';
                                        if (!openReasoning.has(id)) {
                                            openReasoning.add(id);
                                            const header = `thinking [${turn + 1}]: `;
                                            process.stderr.write(this.colorThinking(header));
                                        }
                                    }
                                    else if (p?.type === 'reasoning-delta') {
                                        const seg = (typeof p.text === 'string' ? p.text : (typeof p.delta === 'string' ? p.delta : ''));
                                        if (seg.length > 0)
                                            process.stderr.write(this.colorThinking(seg));
                                    }
                                    else if (p?.type === 'reasoning-end') {
                                        const id = p.id ?? '0';
                                        if (openReasoning.has(id)) {
                                            openReasoning.delete(id);
                                            process.stderr.write(this.colorThinking('\n'));
                                        }
                                    }
                                }
                                catch { }
                            };
                            result = streamText(stOpts);
                            streamInfo = await drainTextStream(result.textStream, () => resetIdle());
                        }
                        finally {
                            clearIdle();
                        }
                        const usageData = await result.usage;
                        inputTokens = usageData.inputTokens ?? 0;
                        outputTokens = usageData.outputTokens ?? 0;
                        cachedTokens = usageData.cachedTokens;
                        this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'ok', latency: Date.now() - iterationStart, provider, model, tokens: { inputTokens, outputTokens, totalTokens: (usageData.totalTokens ?? (inputTokens + outputTokens)), cachedTokens } });
                        if (this.options.verbose === true) {
                            try {
                                aggInput += inputTokens;
                                aggOutput += outputTokens;
                                aggCached += cachedTokens ?? 0;
                                aggLLMLatency += (Date.now() - iterationStart);
                            }
                            catch { }
                        }
                        let resp;
                        try {
                            resp = await result.response;
                        }
                        catch (err) {
                            const name = err instanceof Error ? err.name : 'Error';
                            const msg = err instanceof Error ? err.message : String(err);
                            const hint = this.options.traceLLM === true ? '' : ' Enable --trace-llm to see raw HTTP/SSE.';
                            const details = `streamed=${streamInfo.sawAny} (${streamInfo.totalChars} chars), tools=${executedTools.length}, tokens in=${inputTokens}, out=${outputTokens}, cached=${cachedTokens ?? 0}.`;
                            throw new Error(`${name}: ${msg}.${hint} Details: ${details}`);
                        }
                        {
                            const rawUnknown = resp.messages;
                            const raw = Array.isArray(rawUnknown) ? rawUnknown : [];
                            turnMessages = raw.map((m) => {
                                const roleValue = (typeof m.role === 'string' ? m.role : 'assistant');
                                const contentValue = typeof m.content === 'string' ? m.content : JSON.stringify(m.content);
                                let tcs;
                                if (Array.isArray(m.toolCalls)) {
                                    const arr = m.toolCalls;
                                    tcs = arr.map((tc) => ({ id: (tc.id ?? tc.toolCallId ?? ''), name: (tc.name ?? tc.function?.name ?? ''), parameters: (tc.arguments ?? tc.function?.arguments ?? {}) }));
                                }
                                const toolCallIdVal = typeof m.toolCallId === 'string' ? m.toolCallId : undefined;
                                return {
                                    role: roleValue,
                                    content: contentValue,
                                    toolCalls: tcs,
                                    toolCallId: toolCallIdVal,
                                    metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens: (usageData.totalTokens ?? (inputTokens + outputTokens)) }, timestamp: Date.now() }
                                };
                            });
                            const hasToolArtifacts = raw.some((mm) => (mm.role === 'tool') || (Array.isArray(mm.toolCalls) && mm.toolCalls.length > 0));
                            // Fallback print: if nothing streamed but assistant text exists, print it once
                            if (!streamInfo.sawAny) {
                                try {
                                    const assistantText = turnMessages
                                        .filter((m) => m.role === 'assistant' && typeof m.content === 'string' && m.content.length > 0)
                                        .map((m) => m.content)
                                        .join('\n');
                                    if (assistantText.length > 0)
                                        this.output(assistantText + (assistantText.endsWith('\n') ? '' : '\n'));
                                }
                                catch { }
                            }
                            // Verbose response line for this turn
                            if (this.options.verbose === true) {
                                try {
                                    const latency = Date.now() - iterationStart;
                                    const size = turnMessages.reduce((acc, m) => acc + m.content.length, 0);
                                    process.stderr.write(this.colorVerbose(`agent ← llm [${turn + 1}] input ${inputTokens}, output ${outputTokens}, cached ${cachedTokens ?? 0} tokens, tools ${executedTools.length}, latency ${latency} ms, size ${size} chars\n`));
                                    aggAssistantChars += size;
                                    aggToolCalls += executedTools.length;
                                    resLogged = true;
                                }
                                catch { }
                            }
                            if (hasToolArtifacts && !isFinalTurn) {
                                const nextMsgs = msgs.concat(raw.map((mm) => {
                                    const out = { role: mm.role, content: mm.content };
                                    if (Array.isArray(mm.toolCalls))
                                        out.toolCalls = mm.toolCalls;
                                    if (typeof mm.toolCallId === 'string')
                                        out.toolCallId = mm.toolCallId;
                                    return out;
                                }));
                                return runTurn(turn + 1, nextMsgs);
                            }
                            // Detect reasoning-only (no assistant text, no tool calls) in streaming path
                            {
                                const hasAssistantText = turnMessages.some((mm) => mm.role === 'assistant' && typeof mm.content === 'string' && mm.content.trim().length > 0);
                                const reasoningOnly = !hasAssistantText && !hasToolArtifacts && (outputTokens > 0);
                                if (reasoningOnly) {
                                    try {
                                        this.log('warn', `${provider}/${model} produced reasoning-only turn (no assistant text or tool calls)`);
                                        if (this.options.verbose === true) {
                                            process.stderr.write(this.colorVerbose(`[llm] note: reasoning-only (no text/toolcalls); outputTokens ${outputTokens}\n`));
                                        }
                                    }
                                    catch { }
                                }
                            }
                        }
                    }
                    else {
                        lastLLMCallStart = Date.now();
                        const gtOpts = {
                            model: llmProvider(model),
                            messages: msgsForThisCall,
                            tools: isFinalTurn ? undefined : sdkTools,
                            temperature: this.options.temperature,
                            topP: this.options.topP,
                            providerOptions: providerOptionsDecl,
                            abortSignal: AbortSignal.timeout(this.options.llmTimeout),
                        };
                        gtOpts.onStepFinish = (step) => {
                            try {
                                const s = step;
                                if (typeof s.reasoningText === 'string' && s.reasoningText.length > 0) {
                                    const line = `thinking [${turn + 1}]: ${s.reasoningText}`;
                                    process.stderr.write(this.colorThinking(line.endsWith('\n') ? line : (line + '\n')));
                                }
                            }
                            catch { }
                        };
                        const result = await generateText(gtOpts);
                        if (typeof result.text === 'string' && result.text.length > 0)
                            this.output(result.text + (result.text.endsWith('\n') ? '' : '\n'));
                        const usageMaybe = result.usage;
                        const usageData = usageMaybe ?? {};
                        inputTokens = usageData.inputTokens ?? 0;
                        outputTokens = usageData.outputTokens ?? 0;
                        cachedTokens = usageData.cachedTokens;
                        this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'ok', latency: Date.now() - iterationStart, provider, model, tokens: { inputTokens, outputTokens, totalTokens: (usageData.totalTokens ?? (inputTokens + outputTokens)), cachedTokens } });
                        if (this.options.verbose === true) {
                            try {
                                aggInput += inputTokens;
                                aggOutput += outputTokens;
                                aggCached += cachedTokens ?? 0;
                                aggLLMLatency += (Date.now() - iterationStart);
                            }
                            catch { }
                        }
                        const respObj = (result.response ?? {});
                        const raw2 = Array.isArray(respObj.messages) ? respObj.messages : [];
                        turnMessages = raw2.map((m) => {
                            const roleValue = (typeof m.role === 'string' ? m.role : 'assistant');
                            const contentValue = typeof m.content === 'string' ? m.content : JSON.stringify(m.content);
                            let tcs;
                            if (Array.isArray(m.toolCalls)) {
                                const arr = m.toolCalls;
                                tcs = arr.map((tc) => ({ id: (tc.id ?? tc.toolCallId ?? ''), name: (tc.name ?? tc.function?.name ?? ''), parameters: (tc.arguments ?? tc.function?.arguments ?? {}) }));
                            }
                            const toolCallIdVal = typeof m.toolCallId === 'string' ? m.toolCallId : undefined;
                            return {
                                role: roleValue,
                                content: contentValue,
                                toolCalls: tcs,
                                toolCallId: toolCallIdVal,
                                metadata: { provider, model, tokens: { inputTokens, outputTokens, totalTokens: (usageData.totalTokens ?? (inputTokens + outputTokens)) }, timestamp: Date.now() }
                            };
                        });
                        const hasToolArtifacts2 = raw2.some((mm) => (mm.role === 'tool') || (Array.isArray(mm.toolCalls) && mm.toolCalls.length > 0));
                        // Verbose response line for this turn (non-streaming)
                        if (this.options.verbose === true) {
                            try {
                                const latency = Date.now() - iterationStart;
                                const size = turnMessages.reduce((acc, m) => acc + m.content.length, 0);
                                process.stderr.write(this.colorVerbose(`agent ← llm [${turn + 1}] input ${inputTokens}, output ${outputTokens}, cached ${cachedTokens ?? 0} tokens, tools ${executedTools.length}, latency ${latency} ms, size ${size} chars\n`));
                                aggAssistantChars += size;
                                aggToolCalls += executedTools.length;
                                resLogged = true;
                            }
                            catch { }
                        }
                        if (hasToolArtifacts2 && !isFinalTurn) {
                            const nextMsgs = msgs.concat(raw2.map((mm) => {
                                const out = { role: mm.role, content: mm.content };
                                if (Array.isArray(mm.toolCalls))
                                    out.toolCalls = mm.toolCalls;
                                if (typeof mm.toolCallId === 'string')
                                    out.toolCallId = mm.toolCallId;
                                return out;
                            }));
                            return runTurn(turn + 1, nextMsgs);
                        }
                        // Detect reasoning-only (no assistant text, no tool calls) in non-streaming path
                        {
                            const hasAssistantText = turnMessages.some((mm) => mm.role === 'assistant' && typeof mm.content === 'string' && mm.content.trim().length > 0);
                            const reasoningOnly = !hasAssistantText && !hasToolArtifacts2 && (outputTokens > 0);
                            if (reasoningOnly) {
                                try {
                                    this.log('warn', `${provider}/${model} produced reasoning-only turn (no assistant text or tool calls)`);
                                    if (this.options.verbose === true) {
                                        process.stderr.write(this.colorVerbose(`[llm] note: reasoning-only (no text/toolcalls); outputTokens ${outputTokens}\n`));
                                    }
                                }
                                catch { }
                            }
                        }
                    }
                    if (this.options.verbose === true && !resLogged) {
                        try {
                            const latency = Date.now() - iterationStart;
                            const size = turnMessages.reduce((acc, m) => acc + m.content.length, 0);
                            process.stderr.write(this.colorVerbose(`agent ← llm [${turn + 1}] input ${inputTokens}, output ${outputTokens}, cached ${cachedTokens ?? 0} tokens, tools ${executedTools.length}, latency ${latency} ms, size ${size} chars\n`));
                            aggAssistantChars += size;
                            aggToolCalls += executedTools.length;
                        }
                        catch { }
                    }
                    // No tool artifacts: return
                    // If there is assistant text, return it; otherwise consider it a failure
                    if (turnMessages.length > 0)
                        return turnMessages;
                    throw new Error('Empty response from model');
                };
                const finalAppend = await runTurn(0, currentMessages);
                return { success: true, appendMessages: finalAppend };
            }
            catch (err) {
                const em = err instanceof Error ? err.message : String(err ?? 'Unknown error');
                const en = err instanceof Error ? err.name : 'Error';
                const stackTop = err instanceof Error && typeof err.stack === 'string' ? err.stack.split('\n')[1]?.trim() : undefined;
                const extra = this.options.verbose === true && stackTop ? ` (${stackTop})` : '';
                const advice = this.options.traceLLM === true ? '' : ' Hint: run with --trace-llm to inspect HTTP/SSE.';
                const waitedMs = (() => { try {
                    const now = Date.now();
                    const base = lastLLMCallStart && lastLLMCallStart > 0 ? lastLLMCallStart : attemptStart;
                    return now - base;
                }
                catch {
                    return 0;
                } })();
                this.log('warn', `${provider}/${model} failed: [${en}] ${em}${extra} (waited ${waitedMs} ms)${advice}`);
                this.callbacks.onAccounting?.({ type: 'llm', timestamp: Date.now(), status: 'failed', latency: 0, provider, model, tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 }, error: em });
                return { success: false, appendMessages: [], error: em };
            }
        };
        // Retry rounds over provider/model pairs
        let lastError;
        for (let round = 0; round < this.options.maxRetries; round += 1) {
            for (let i = 0; i < pairs.length; i += 1) {
                const res = await tryPair(i);
                if (res.success) {
                    if (this.options.verbose === true) {
                        try {
                            const mcpSummary = Array.from(mcpPerServer.entries()).map(([srv, cnt]) => `${srv} ${cnt}`).join(', ');
                            process.stderr.write(`[fin] finally: llm requests ${llmRequests} (tokens: ${aggInput} in, ${aggOutput} out, ${aggCached} cached, tool-calls ${aggToolCalls}, output-size ${aggAssistantChars} chars, latency-sum ${aggLLMLatency} ms), mcp requests ${mcpRequests}${mcpSummary.length > 0 ? ` (${mcpSummary})` : ''}\n`);
                        }
                        catch { }
                    }
                    return res;
                }
                lastError = res.error ?? lastError;
            }
        }
        if (this.options.verbose === true) {
            try {
                const mcpSummary = Array.from(mcpPerServer.entries()).map(([srv, cnt]) => `${srv} ${cnt}`).join(', ');
                process.stderr.write(`[fin] finally: llm requests ${llmRequests} (tokens: ${aggInput} in, ${aggOutput} out, ${aggCached} cached, tool-calls ${aggToolCalls}, output-size ${aggAssistantChars} chars, latency-sum ${aggLLMLatency} ms), mcp requests ${mcpRequests}${mcpSummary.length > 0 ? ` (${mcpSummary})` : ''}\n`);
            }
            catch { }
        }
        return { success: false, appendMessages: [], error: lastError ?? 'All providers and models failed' };
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
                    const isOpenRouter = typeof url === 'string' && url.includes('openrouter.ai');
                    const cfgAny = cfg;
                    if (isOpenRouter && cfgAny.custom != null && typeof requestInit.body === 'string') {
                        try {
                            const bodyObj = JSON.parse(requestInit.body);
                            const merged = (function mergeCustom(base, custom, strategy) {
                                const isObj = (v) => typeof v === 'object' && v !== null && !Array.isArray(v);
                                if (strategy === 'override')
                                    return { ...base, ...custom };
                                if (strategy === 'overlay') {
                                    const out = Object.entries(custom).reduce((acc, [k, v]) => { if (!(k in acc))
                                        acc[k] = v; return acc; }, { ...base });
                                    return out;
                                }
                                const deepMerge = (a, b) => Object.entries(b).reduce((acc, [k, v]) => {
                                    const av = acc[k];
                                    acc[k] = (isObj(av) && isObj(v)) ? deepMerge(av, v) : v;
                                    return acc;
                                }, { ...a });
                                return deepMerge(base, custom);
                            })(bodyObj, cfgAny.custom, cfgAny.mergeStrategy ?? 'overlay');
                            requestInit.body = JSON.stringify(merged);
                        }
                        catch { }
                    }
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
                return (model) => prov.responses(model);
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
                const prov = createOpenRouter({
                    apiKey: cfg.apiKey,
                    fetch: tracedFetch,
                    headers: {
                        'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
                        'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent-codex',
                        'User-Agent': 'ai-agent-codex/1.0',
                        ...(cfg.headers ?? {}),
                    },
                });
                return (model) => prov(model);
            }
            case 'ollama': {
                // Use community AI SDK 5 provider for Ollama (native API under /api)
                // Normalize base URL: users may specify OpenAI-compatible '/v1'; convert to native '/api'.
                const normalizeBaseUrl = (u) => {
                    const def = 'http://localhost:11434/api';
                    if (!u || typeof u !== 'string')
                        return def;
                    try {
                        let v = u.replace(/\/$/, '');
                        // Replace trailing /v1 with /api
                        if (/\/v1\/?$/.test(v))
                            return v.replace(/\/v1\/?$/, '/api');
                        // If already ends with /api, keep as-is
                        if (/\/api\/?$/.test(v))
                            return v;
                        // Otherwise, append /api
                        return v + '/api';
                    }
                    catch {
                        return def;
                    }
                };
                const prov = createOllama({ baseURL: normalizeBaseUrl(cfg.baseUrl), fetch: tracedFetch });
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
