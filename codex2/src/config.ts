import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { z } from 'zod';

import type { Configuration } from './types.js';

const ProviderConfigSchema = z.object({
  apiKey: z.string().optional(),
  baseUrl: z.url().optional(),
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
  url: z.url().optional(),
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

function expandEnv(str: string): string {
  return str.replace(/\$\{([^}]+)\}/g, (_m, name: string) => process.env[name] ?? '');
}

function expandDeep(obj: unknown, chain: string[] = []): unknown {
  if (typeof obj === 'string') {
    // Keep literals in MCP env/headers; resolve later per server
    if (chain.includes('mcpServers') && (chain.includes('env') || chain.includes('headers'))) return obj;
    return expandEnv(obj);
  }
  if (Array.isArray(obj)) return obj.map((v) => expandDeep(v, chain));
  if (obj !== null && typeof obj === 'object') {
    const e = Object.entries(obj as Record<string, unknown>);
    return e.reduce<Record<string, unknown>>((acc, [k, v]) => {
      acc[k] = expandDeep(v, [...chain, k]);
      return acc;
    }, {});
  }
  return obj;
}

function resolveConfigPath(configPath?: string): string {
  if (typeof configPath === 'string' && configPath.length > 0) {
    if (!fs.existsSync(configPath)) throw new Error(`Configuration file not found: ${configPath}`);
    return configPath;
  }
  const local = path.join(process.cwd(), '.ai-agent.json');
  if (fs.existsSync(local)) return local;
  const home = path.join(os.homedir(), '.ai-agent.json');
  if (fs.existsSync(home)) return home;
  throw new Error('Configuration file not found. Create .ai-agent.json or pass --config');
}

export function loadConfiguration(configPath?: string): Configuration {
  const resolved = resolveConfigPath(configPath);
  const raw = fs.readFileSync(resolved, 'utf-8');
  let json: unknown;
  try { json = JSON.parse(raw); } catch (e) { throw new Error(`Invalid JSON in configuration file ${resolved}: ${e instanceof Error ? e.message : String(e)}`); }
  const expanded = expandDeep(json);

  // Normalize MCP servers (aliases, arrays, defaults)
  const obj: Record<string, unknown> = (typeof expanded === 'object' && expanded !== null) ? (expanded as Record<string, unknown>) : {};
  const serversRaw = (obj as { mcpServers?: unknown }).mcpServers;
  const mcp: Record<string, unknown> = (typeof serversRaw === 'object' && serversRaw !== null) ? (serversRaw as Record<string, unknown>) : {};
  const normalized = Object.fromEntries(
    Object.entries(mcp).map(([name, srv]) => {
      if (srv === null || srv === undefined || typeof srv !== 'object') return [name, srv];
      const s = srv as Record<string, unknown>;
      const out: Record<string, unknown> = { ...s };
      // type aliasing
      const t = s.type as string | undefined;
      if (t === 'local') out.type = 'stdio';
      else if (t === 'remote') out.type = (typeof s.url === 'string' && s.url.includes('/sse')) ? 'sse' : 'http';
      // command array normalization
      if (Array.isArray(s.command) && (s.command as unknown[]).length > 0) {
        const arr = s.command as unknown[];
        out.command = String(arr[0]);
        const extra = Array.isArray(s.args) ? (s.args as unknown[]) : [];
        out.args = [...arr.slice(1).map((v) => String(v)), ...extra.map((v) => String(v))];
      }
      // environment -> env
      if (s.environment !== undefined && s.env === undefined) out.env = s.environment;
      // enabled default
      if (typeof s.enabled !== 'boolean') out.enabled = true;
      return [name, out];
    })
  );
  const finalExpanded = { ...obj, mcpServers: normalized };

  const parsed = ConfigurationSchema.safeParse(finalExpanded);
  if (!parsed.success) {
    const msgs = parsed.error.issues.map((i) => `  ${i.path.join('.')}: ${i.message}`).join('\n');
    throw new Error(`Configuration validation failed in ${resolved}:\n${msgs}`);
  }
  return parsed.data;
}

export function validateProviders(config: Configuration, providers: string[]): void {
  const missing = providers.filter((p) => !(p in config.providers));
  if (missing.length) throw new Error(`Unknown providers: ${missing.join(', ')}`);
}

export function validateMCPServers(config: Configuration, servers: string[]): void {
  const missing = servers.filter((s) => !(s in config.mcpServers));
  if (missing.length) throw new Error(`Unknown MCP servers: ${missing.join(', ')}`);
}

export function validatePrompts(systemPrompt: string, userPrompt: string): void {
  if (systemPrompt === '-' && userPrompt === '-') throw new Error('Cannot use stdin (-) for both system and user prompts');
}
