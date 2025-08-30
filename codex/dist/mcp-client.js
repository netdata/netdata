import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { createWebSocketTransport } from './websocket-transport.js';
export class MCPClientManager {
    clients = new Map();
    processes = new Map();
    servers = new Map();
    trace = false;
    logger = (_msg) => { return; };
    traceLog(message) {
        try {
            process.stderr.write(`[mcp] ${message}\n`);
        }
        catch { }
    }
    constructor(opts) {
        this.trace = opts?.trace === true;
        if (typeof opts?.logger === 'function')
            this.logger = opts.logger;
    }
    async initializeServers(mcpServers) {
        const entries = Object.entries(mcpServers);
        const servers = await Promise.all(entries.map(async ([name, config]) => {
            const server = await this.initializeServer(name, config);
            this.servers.set(name, server);
            return server;
        }));
        return servers;
    }
    async initializeServer(name, config) {
        const client = new Client({ name: 'ai-agent', version: '1.0.0' }, { capabilities: { tools: {} } });
        let transport;
        if (this.trace)
            this.traceLog(`connect '${name}' type=${config.type} ${config.command ?? config.url ?? ''}`);
        switch (config.type) {
            case 'stdio':
                transport = this.createStdioTransport(name, config);
                break;
            case 'websocket':
                transport = await createWebSocketTransport(config.url ?? '', resolveHeaders(config.headers));
                break;
            case 'http': {
                if (config.url == null || config.url.length === 0)
                    throw new Error(`HTTP MCP server '${name}' requires a 'url'`);
                const { StreamableHTTPClientTransport } = await import('@modelcontextprotocol/sdk/client/streamableHttp.js');
                const reqInit = { headers: resolveHeaders(config.headers) };
                transport = new StreamableHTTPClientTransport(new URL(config.url), { requestInit: reqInit });
                break;
            }
            case 'sse':
                if (config.url == null || config.url.length === 0)
                    throw new Error(`SSE MCP server '${name}' requires a 'url'`);
                transport = new SSEClientTransport(new URL(config.url), resolveHeaders(config.headers));
                break;
            default:
                throw new Error('Unsupported transport type');
        }
        {
            const connTransport = transport;
            await client.connect(connTransport);
        }
        this.clients.set(name, client);
        const initInstructions = client.getInstructions() ?? '';
        if (this.trace)
            this.traceLog(`request '${name}': tools/list`);
        const toolsResponse = await client.listTools();
        if (this.trace)
            this.traceLog(`response '${name}': tools/list -> ${JSON.stringify(toolsResponse, null, 2)}`);
        const tools = toolsResponse.tools.map((t) => ({
            name: t.name,
            description: t.description ?? '',
            inputSchema: (t.inputSchema ?? t.parameters ?? {}),
            instructions: t.instructions,
        }));
        let instructions = initInstructions;
        try {
            if (this.trace)
                this.traceLog(`request '${name}': prompts/list`);
            const promptsResponse = await client.listPrompts();
            if (this.trace)
                this.traceLog(`response '${name}': prompts/list -> ${JSON.stringify(promptsResponse, null, 2)}`);
            const pr = promptsResponse;
            const listRaw = pr?.prompts;
            const list = Array.isArray(listRaw) ? listRaw : [];
            if (list.length > 0) {
                const promptText = list.map((p) => `${p.name}: ${p.description ?? ''}`).join('\n');
                {
                    const hasInstr = instructions.length > 0;
                    instructions = hasInstr ? `${instructions}\n${promptText}` : promptText;
                }
            }
        }
        catch { }
        if (this.trace)
            this.traceLog(`ready '${name}' with ${tools.length.toString()} tools`);
        return { name, config, tools, instructions };
    }
    createStdioTransport(name, config) {
        if (typeof config.command !== 'string' || config.command.length === 0)
            throw new Error(`Stdio MCP server '${name}' requires a string 'command'`);
        const configured = config.env ?? {};
        const effectiveEnv = {};
        Object.entries(configured).forEach(([k, v]) => {
            const raw = v;
            const resolved = raw.replace(/\$\{([^}]+)\}/g, (_m, varName) => process.env[varName] ?? '');
            if (resolved.length > 0)
                effectiveEnv[k] = resolved;
        });
        const transport = new StdioClientTransport({
            command: config.command,
            args: config.args ?? [],
            env: effectiveEnv,
            stderr: 'pipe',
        });
        try {
            if (transport.stderr != null) {
                transport.stderr.on('data', (d) => {
                    const line = d.toString('utf8').trimEnd();
                    if (this.trace)
                        this.traceLog(`stderr '${name}': ${line}`);
                });
            }
        }
        catch { }
        return transport;
    }
    async executeTool(serverName, toolCall, timeoutMs) {
        const client = this.clients.get(serverName);
        if (client == null)
            throw new Error(`MCP server '${serverName}' not initialized`);
        const start = Date.now();
        const argsStr = JSON.stringify(toolCall.parameters);
        const call = async () => {
            if (this.trace)
                this.traceLog(`callTool '${serverName}': ${toolCall.name}(${JSON.stringify(toolCall.parameters)})`);
            const res = await client.callTool({ name: toolCall.name, arguments: toolCall.parameters });
            if (this.trace)
                this.traceLog(`result '${serverName}': ${toolCall.name} -> ${JSON.stringify(res, null, 2)}`);
            return res;
        };
        const withTimeout = async (p) => {
            if (typeof timeoutMs !== 'number' || timeoutMs <= 0)
                return p;
            const timeoutPromise = new Promise((_resolve, reject) => { setTimeout(() => { reject(new Error('Tool execution timed out')); }, timeoutMs); });
            return (await Promise.race([p, timeoutPromise]));
        };
        try {
            const resp = await withTimeout(call());
            let result = '';
            const content = resp.content ?? [];
            result = content.map((c) => (c.type === 'text' ? c.text ?? '' : c.type === 'image' ? '[Image]' : `[${c.type}]`)).join('');
            const isError = resp.isError === true;
            return {
                toolCallId: toolCall.id,
                result,
                success: !isError,
                error: isError ? 'Tool execution failed' : undefined,
                metadata: {
                    latency: Date.now() - start,
                    charactersIn: argsStr.length,
                    charactersOut: result.length,
                    mcpServer: serverName,
                    command: toolCall.name,
                },
            };
        }
        catch (err) {
            return {
                toolCallId: toolCall.id,
                result: '',
                success: false,
                error: err instanceof Error ? err.message : String(err),
                metadata: {
                    latency: Date.now() - start,
                    charactersIn: argsStr.length,
                    charactersOut: 0,
                    mcpServer: serverName,
                    command: toolCall.name,
                },
            };
        }
    }
    async executeTools(toolCalls, maxConcurrent, timeoutMs) {
        if (typeof maxConcurrent === 'number' && maxConcurrent > 0)
            return this.executeWithConcurrency(toolCalls, maxConcurrent, timeoutMs);
        return Promise.all(toolCalls.map(({ serverName, toolCall }) => this.executeTool(serverName, toolCall, timeoutMs)));
    }
    async executeWithConcurrency(toolCalls, maxConcurrent, timeoutMs) {
        const results = new Array(toolCalls.length);
        let cursor = 0;
        const runWorker = async () => {
            const i = cursor;
            cursor += 1;
            if (i >= toolCalls.length)
                return;
            const { serverName, toolCall } = toolCalls[i];
            results[i] = await this.executeTool(serverName, toolCall, timeoutMs);
            await runWorker();
        };
        const workers = Array.from({ length: Math.min(maxConcurrent, toolCalls.length) }, () => { void runWorker(); });
        await Promise.all(workers);
        return results;
    }
    getAllTools() {
        return Array.from(this.servers.values()).flatMap((s) => s.tools);
    }
    getCombinedInstructions() {
        return Array.from(this.servers.values()).flatMap((s) => {
            const arr = [];
            if (typeof s.instructions === 'string' && s.instructions.length > 0)
                arr.push(`## TOOL ${s.name} INSTRUCTIONS\n${s.instructions}`);
            s.tools.forEach((t) => { if (typeof t.instructions === 'string' && t.instructions.length > 0)
                arr.push(`## TOOL ${t.name} INSTRUCTIONS\n${t.instructions}`); });
            return arr;
        }).join('\n\n');
    }
    getToolServerMapping() {
        const entries = Array.from(this.servers.entries()).flatMap(([serverName, s]) => s.tools.map((t) => [t.name, serverName]));
        return new Map(entries);
    }
    async cleanup() {
        await Promise.all(Array.from(this.clients.values()).map(async (c) => { try {
            await c.close();
        }
        catch { /* noop */ } }));
        await Promise.all(Array.from(this.processes.values()).map((proc) => { try {
            proc.kill('SIGTERM');
            setTimeout(() => { if (!proc.killed)
                proc.kill('SIGKILL'); }, 5000);
        }
        catch { /* noop */ } }));
        this.clients.clear();
        this.processes.clear();
        this.servers.clear();
    }
}
function resolveHeaders(headers) {
    if (headers === undefined)
        return headers;
    return Object.entries(headers).reduce((acc, [k, v]) => {
        const raw = v;
        const resolved = raw.replace(/\$\{([^}]+)\}/g, (_m, name) => process.env[name] ?? '');
        if (resolved.length > 0)
            acc[k] = resolved;
        return acc;
    }, {});
}
