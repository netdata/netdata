import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { z } from 'zod';
const ProviderConfigSchema = z.object({
    apiKey: z.string().optional(),
    baseUrl: z.string().url().optional(),
    headers: z.record(z.string(), z.string()).optional(),
    custom: z.record(z.string(), z.unknown()).optional(),
    mergeStrategy: z.enum(['overlay', 'override', 'deep']).optional(),
    type: z.enum(['openai', 'anthropic', 'google', 'openrouter', 'ollama']).optional(),
    openaiMode: z.enum(['responses', 'chat']).optional(),
});
const MCPServerConfigSchema = z.object({
    type: z.enum(['stdio', 'websocket', 'http', 'sse']),
    command: z.string().optional(),
    args: z.array(z.string()).optional(),
    url: z.string().url().optional(),
    headers: z.record(z.string(), z.string()).optional(),
    env: z.record(z.string(), z.string()).optional(),
    enabled: z.boolean().optional(),
    toolSchemas: z.record(z.string(), z.unknown()).optional(),
});
const ConfigurationSchema = z.object({
    providers: z.record(z.string(), ProviderConfigSchema),
    mcpServers: z.record(z.string(), MCPServerConfigSchema),
    accounting: z.object({ file: z.string() }).optional(),
    defaults: z
        .object({
        llmTimeout: z.number().positive().optional(),
        toolTimeout: z.number().positive().optional(),
        temperature: z.number().min(0).max(2).optional(),
        topP: z.number().min(0).max(1).optional(),
        stream: z.boolean().optional(),
        parallelToolCalls: z.boolean().optional(),
        maxToolTurns: z.number().int().positive().optional(),
        maxRetries: z.number().int().positive().optional(),
    })
        .optional(),
});
function hasMcpServers(x) {
    return x !== null && typeof x === 'object' && 'mcpServers' in x && typeof x.mcpServers === 'object' && x.mcpServers !== null;
}
function expandEnv(str) {
    return str.replace(/\$\{([^}]+)\}/g, (_m, name) => (process.env[name] ?? ''));
}
function expandDeep(obj, chain = []) {
    if (typeof obj === 'string') {
        if (chain.includes('mcpServers') && (chain.includes('env') || chain.includes('headers')))
            return obj;
        return expandEnv(obj);
    }
    if (Array.isArray(obj))
        return obj.map((v) => expandDeep(v, chain));
    if (obj !== null && typeof obj === 'object') {
        const entries = Object.entries(obj);
        return entries.reduce((acc, [k, v]) => {
            acc[k] = expandDeep(v, [...chain, k]);
            return acc;
        }, {});
    }
    return obj;
}
function resolveConfigPath(configPath) {
    if (typeof configPath === 'string' && configPath.length > 0) {
        if (!fs.existsSync(configPath))
            throw new Error(`Configuration file not found: ${configPath}`);
        return configPath;
    }
    const local = path.join(process.cwd(), '.ai-agent.json');
    if (fs.existsSync(local))
        return local;
    const home = path.join(os.homedir(), '.ai-agent.json');
    if (fs.existsSync(home))
        return home;
    throw new Error('Configuration file not found. Create .ai-agent.json or pass --config');
}
export function loadConfiguration(configPath) {
    const resolved = resolveConfigPath(configPath);
    let raw;
    try {
        raw = fs.readFileSync(resolved, 'utf-8');
    }
    catch (e) {
        throw new Error(`Failed to read configuration file ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
    }
    let json;
    try {
        json = JSON.parse(raw);
    }
    catch (e) {
        throw new Error(`Invalid JSON in configuration file ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
    }
    let expanded;
    try {
        expanded = expandDeep(json);
    }
    catch (e) {
        throw new Error(`Environment variable expansion failed in ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
    }
    // Normalize MCP server configs to internal schema (without using any)
    let expandedNormalized = expanded;
    if (hasMcpServers(expanded)) {
        const srcServers = expanded.mcpServers;
        const serversObj = srcServers ?? {};
        const normalizedServers = Object.fromEntries(Object.entries(serversObj).map(([name, srv]) => {
            if (srv === null || typeof srv !== 'object')
                return [name, srv];
            const s = srv;
            const out = { ...s };
            // type normalization
            const typeVal = s.type;
            if (typeVal === 'local')
                out.type = 'stdio';
            else if (typeVal === 'remote') {
                const url = typeof s.url === 'string' ? s.url : '';
                out.type = url.includes('/sse') ? 'sse' : 'http';
            }
            // command array normalization
            const cmd = s.command;
            if (Array.isArray(cmd) && cmd.length > 0) {
                const existingArgs = Array.isArray(s.args) ? s.args.map((a) => String(a)) : [];
                out.command = String(cmd[0]);
                out.args = [...cmd.slice(1).map((a) => String(a)), ...existingArgs];
            }
            // environment -> env
            if (s.environment !== undefined && s.env === undefined)
                out.env = s.environment;
            // enabled default
            if (typeof s.enabled !== 'boolean')
                out.enabled = true;
            return [name, out];
        }));
        expandedNormalized = { ...expanded, mcpServers: normalizedServers };
    }
    const parsed = ConfigurationSchema.safeParse(expandedNormalized);
    if (!parsed.success) {
        const msgs = parsed.error.issues
            .map((issue) => `  ${issue.path.map((p) => String(p)).join('.')}: ${issue.message}`)
            .join('\n');
        throw new Error(`Configuration validation failed in ${resolved}:\n${msgs}`);
    }
    return parsed.data;
}
export function validateProviders(config, providers) {
    const missing = providers.filter((p) => !(p in config.providers));
    if (missing.length > 0)
        throw new Error(`Unknown providers: ${missing.join(', ')}`);
}
export function validateMCPServers(config, mcpServers) {
    const missing = mcpServers.filter((s) => !(s in config.mcpServers));
    if (missing.length > 0)
        throw new Error(`Unknown MCP servers: ${missing.join(', ')}`);
}
export function validatePrompts(systemPrompt, userPrompt) {
    if (systemPrompt === '-' && userPrompt === '-')
        throw new Error('Cannot use stdin (-) for both system and user prompts');
}
