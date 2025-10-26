import fs from 'node:fs';
import path from 'node:path';

import type { Configuration, MCPServerConfig, ProviderConfig, RestToolConfig, OpenAPISpecConfig, TelemetryConfig } from './types.js';

import { warn } from './utils.js';

type LayerOrigin = '--config' | 'cwd' | 'prompt' | 'binary' | 'home' | 'system';

export interface ResolvedConfigLayer {
  origin: LayerOrigin;
  jsonPath: string; // may not exist
  envPath: string;  // may not exist
  json?: Record<string, unknown>;
  env?: Record<string, string>;
}

class MissingVariableError extends Error {
  public readonly scope: 'provider' | 'mcp' | 'defaults' | 'accounting';

  public readonly id: string;

  public readonly origin: LayerOrigin;

  public readonly variable: string;

  constructor(scope: 'provider' | 'mcp' | 'defaults' | 'accounting', id: string, origin: LayerOrigin, variable: string, message: string) {
    super(message);
    this.scope = scope;
    this.id = id;
    this.origin = origin;
    this.variable = variable;
    this.name = 'MissingVariableError';
  }
}

interface ResolverOptions {
  verbose?: boolean;
  log?: (msg: string) => void;
}

function readJSONIfExists(p: string): Record<string, unknown> | undefined {
  try {
    if (fs.existsSync(p)) {
      const raw = fs.readFileSync(p, 'utf-8');
      return JSON.parse(raw) as Record<string, unknown>;
    }
  } catch (e) { warn(`failed to read or parse JSON config: ${p}: ${e instanceof Error ? e.message : String(e)}`); }
  return undefined;
}

function parseEnvFile(content: string): Record<string, string> {
  const out: Record<string, string> = {};
  const lines = content.split(/\r?\n/);
  lines.forEach((line) => {
    const l = line.trim();
    if (l.length === 0 || l.startsWith('#')) return;
    const eq = l.indexOf('=');
    if (eq <= 0) return;
    const keyRaw = l.slice(0, eq).trim();
    const key = keyRaw.startsWith('export ')
      ? keyRaw.slice('export '.length).trim()
      : keyRaw;
    let val = l.slice(eq + 1).trim();
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    if (key.length > 0) out[key] = val;
  });
  return out;
}

function readEnvIfExists(p: string): Record<string, string> | undefined {
  if (!fs.existsSync(p)) return undefined;
  try {
    const raw = fs.readFileSync(p, 'utf-8');
    return parseEnvFile(raw);
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e);
    warn(`failed to read env file: ${p}: ${message}`);
    return undefined;
  }
}

export function discoverLayers(opts?: { configPath?: string; promptPath?: string }): ResolvedConfigLayer[] {
  const layers: ResolvedConfigLayer[] = [];

  const cwd = process.cwd();
  const binDir = path.dirname(fs.realpathSync(process.execPath));
  const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
  const HIDDEN_JSON = '.ai-agent.json';
  const HIDDEN_ENV = '.ai-agent.env';
  const HOME_DIR = '.ai-agent';
  const SYSTEM_DIR = '/etc/ai-agent';

  const list: { origin: LayerOrigin; json: string; env: string }[] = [];

  if (typeof opts?.configPath === 'string' && opts.configPath.length > 0) {
    list.push({ origin: '--config', json: opts.configPath, env: path.join(path.dirname(opts.configPath), HIDDEN_ENV) });
  }
  list.push({ origin: 'cwd', json: path.join(cwd, HIDDEN_JSON), env: path.join(cwd, HIDDEN_ENV) });

  // Add prompt file directory as second priority (after cwd)
  if (typeof opts?.promptPath === 'string' && opts.promptPath.length > 0) {
    try {
      const promptDir = path.dirname(opts.promptPath);
      if (promptDir !== cwd) { // Only add if different from cwd
        list.push({ origin: 'prompt', json: path.join(promptDir, HIDDEN_JSON), env: path.join(promptDir, HIDDEN_ENV) });
      }
    } catch { /* ignore errors */ }
  }

  list.push({ origin: 'binary', json: path.join(binDir, HIDDEN_JSON), env: path.join(binDir, HIDDEN_ENV) });
  if (home.length > 0) {
    list.push({ origin: 'home', json: path.join(home, HOME_DIR, 'ai-agent.json'), env: path.join(home, HOME_DIR, 'ai-agent.env') });
  }
  list.push({ origin: 'system', json: path.join(SYSTEM_DIR, 'ai-agent.json'), env: path.join(SYSTEM_DIR, 'ai-agent.env') });

  // Highest priority first (as constructed)
  list.forEach((it) => {
    const layer: ResolvedConfigLayer = {
      origin: it.origin,
      jsonPath: it.json,
      envPath: it.env,
      json: readJSONIfExists(it.json),
      env: readEnvIfExists(it.env)
    };
    layers.push(layer);
  });
  return layers;
}

function expandPlaceholders(obj: unknown, vars: (name: string) => string): unknown {
  if (typeof obj === 'string') {
    return obj.replace(/\$\{([^}]+)\}/g, (_m: string, name: string) => vars(name));
  }
  if (Array.isArray(obj)) return obj.map((v) => expandPlaceholders(v, vars));
  if (obj !== null && typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>);
    return entries.reduce<Record<string, unknown>>((acc, [k, v]) => {
      acc[k] = expandPlaceholders(v, vars);
      return acc;
    }, {});
  }
  return obj;
}

function buildMissingVarError(scope: 'provider'|'mcp'|'defaults'|'accounting', id: string, origin: LayerOrigin, name: string): MissingVariableError {
  const message = `Unresolved variable \${${name}} for ${scope} '${id}' at ${origin}. Define it in ${origin === '--config' ? 'the specified config path' : '.ai-agent.env or environment'}.`;
  return new MissingVariableError(scope, id, origin, name, message);
}

function resolveProvider(id: string, layers: ResolvedConfigLayer[], _opts?: ResolverOptions): ProviderConfig | undefined {
  let missingVarError: MissingVariableError | undefined;
  // eslint-disable-next-line functional/no-loop-statements -- Early exit once a valid provider configuration is resolved
  for (const layer of layers) {
    const provs = layer.json?.providers as Record<string, unknown> | undefined;
    if (provs === undefined || !Object.prototype.hasOwnProperty.call(provs, id)) continue;
    const obj = provs[id] as Record<string, unknown>;
    const env = layer.env ?? {};
    try {
      const expanded = expandPlaceholders(obj, (name: string) => {
        const envVal = Object.prototype.hasOwnProperty.call(env, name) ? env[name] : undefined;
        const v = envVal ?? process.env[name];
        if (v === undefined) throw buildMissingVarError('provider', id, layer.origin, name);
        return v;
      }) as Partial<ProviderConfig>;

      if (expanded.type === undefined) {
        const fallback: ProviderConfig['type'] | undefined = (() => {
          const normalized = id.toLowerCase();
          if (normalized === 'openai') return 'openai';
          if (normalized === 'anthropic') return 'anthropic';
          if (normalized === 'google') return 'google';
          if (normalized === 'openrouter') return 'openrouter';
          if (normalized === 'ollama') return 'ollama';
          if (normalized === 'test-llm') return 'test-llm';
          return undefined;
        })();
        if (fallback !== undefined) {
          expanded.type = fallback;
          warn(`provider '${id}' at ${layer.jsonPath} missing "type"; defaulting to '${fallback}'. Update configuration to include "type" explicitly.`);
        } else {
          throw new Error(`Provider '${id}' in ${layer.jsonPath} missing required "type" field. Add "type": "openai"|"anthropic"|"google"|"openrouter"|"ollama"|"test-llm".`);
        }
      }
      return expanded as ProviderConfig;
    } catch (error) {
      if (error instanceof MissingVariableError) {
        missingVarError ??= error;
        continue;
      }
      throw error;
    }
  }
  if (missingVarError !== undefined) throw missingVarError;
  return undefined;
}

function resolveMCPServer(id: string, layers: ResolvedConfigLayer[], opts?: ResolverOptions): MCPServerConfig | undefined {
  let missingVarError: MissingVariableError | undefined;
  // eslint-disable-next-line functional/no-loop-statements -- Early exit once a valid MCP server configuration is resolved
  for (const layer of layers) {
    const srvs = layer.json?.mcpServers as Record<string, unknown> | undefined;
    if (srvs === undefined || !Object.prototype.hasOwnProperty.call(srvs, id)) continue;
    const obj = srvs[id] as Record<string, unknown>;
    const env = layer.env ?? {};
    try {
      const expanded = expandPlaceholders(obj, (name: string) => {
        const envVal = Object.prototype.hasOwnProperty.call(env, name) ? env[name] : undefined;
        const v = envVal ?? process.env[name];
        if (name === 'MCP_ROOT') {
          const resolved = typeof v === 'string' ? v : '';
          const trimmed = resolved.trim();
          if (trimmed.length > 0) return trimmed;
          if (opts?.verbose === true) {
            try {
              const msg = `[VRB] \u2192 [0.0] tool mcp:${id}: MCP_ROOT empty or blank; defaulting to current working directory: ${process.cwd()}\n`;
              // eslint-disable-next-line no-constant-binary-expression
              const out = process.stderr.isTTY ? `\x1b[90m${msg}\x1b[0m` : msg;
              process.stderr.write(out);
            } catch (e) { warn(`MCP_ROOT default warning write failed: ${e instanceof Error ? e.message : String(e)}`); }
          }
          return process.cwd();
        }
        if (v === undefined) throw buildMissingVarError('mcp', id, layer.origin, name);
        return v;
      }) as MCPServerConfig;
      return expanded;
    } catch (error) {
      if (error instanceof MissingVariableError) {
        missingVarError ??= error;
        continue;
      }
      throw error;
    }
  }
  if (missingVarError !== undefined) throw missingVarError;
  return undefined;
}

function resolveRestTool(id: string, layers: ResolvedConfigLayer[]): RestToolConfig | undefined {
  let missingVarError: MissingVariableError | undefined;
  // eslint-disable-next-line functional/no-loop-statements -- Early exit once a valid REST tool configuration is resolved
  for (const layer of layers) {
    const tools = layer.json?.restTools as Record<string, unknown> | undefined;
    if (tools === undefined || !Object.prototype.hasOwnProperty.call(tools, id)) continue;
    const raw = tools[id];
    const env = layer.env ?? {};
    try {
      const expanded = expandPlaceholders(raw, (name: string) => {
        if (name.startsWith('parameters.')) return `\${${name}}`;
        const envVal = Object.prototype.hasOwnProperty.call(env, name) ? env[name] : undefined;
        const v = envVal ?? process.env[name];
        if (v === undefined) throw buildMissingVarError('defaults', id, layer.origin, name);
        return v;
      }) as RestToolConfig;
      return expanded;
    } catch (error) {
      if (error instanceof MissingVariableError) {
        missingVarError ??= error;
        continue;
      }
      throw error;
    }
  }
  if (missingVarError !== undefined) throw missingVarError;
  return undefined;
}

export function resolveDefaults(layers: ResolvedConfigLayer[]): NonNullable<Configuration['defaults']> {
  const out: NonNullable<Configuration['defaults']> = {};
  const keys = [
    'llmTimeout',
    'toolTimeout',
    'temperature',
    'topP',
    'maxOutputTokens',
    'repeatPenalty',
    'stream',
    'parallelToolCalls',
    'maxToolTurns',
    'maxConcurrentTools',
    'maxRetries',
    'toolResponseMaxBytes',
    'mcpInitConcurrency'
  ] as const;
  keys.forEach((k) => {
    const found = layers.find((layer) => {
      const j = layer.json as { defaults?: Record<string, unknown> } | undefined;
      return j?.defaults !== undefined && Object.prototype.hasOwnProperty.call(j.defaults, k);
    });
    if (found !== undefined) {
      const j = found.json as { defaults?: Record<string, unknown> } | undefined;
      const dfl = j?.defaults;
      if (dfl !== undefined && Object.prototype.hasOwnProperty.call(dfl, k)) (out as Record<string, unknown>)[k] = dfl[k];
    }
  });
  return out;
}

function resolveAccounting(layers: ResolvedConfigLayer[], _opts?: ResolverOptions): Configuration['accounting'] {
  const found = layers.find((layer) => {
    const j = layer.json as { accounting?: { file?: string } } | undefined;
    return typeof j?.accounting?.file === 'string';
  });
  if (found === undefined) return undefined;
  const j = found.json as { accounting?: { file?: string } } | undefined;
  const acc = j?.accounting;
  if (acc === undefined || typeof acc.file !== 'string') return undefined;
  const env = found.env ?? {};
  const expandedFile = expandPlaceholders(acc.file, (name: string) => {
    const envVal = Object.prototype.hasOwnProperty.call(env, name) ? env[name] : undefined;
    const v = envVal ?? process.env[name];
    if (v === undefined) throw buildMissingVarError('accounting', 'file', found.origin, name);
    return v;
  }) as string;
  return { file: expandedFile };
}

function resolvePricing(layers: ResolvedConfigLayer[]): Configuration['pricing'] {
  const found = layers.find((layer) => {
    const j = layer.json as { pricing?: Record<string, unknown> } | undefined;
    return j?.pricing !== undefined;
  });
  if (found === undefined) return undefined;
  const j = found.json as { pricing?: Record<string, unknown> } | undefined;
  const pr = j?.pricing as Configuration['pricing'];
  return pr;
}

function resolveTelemetry(layers: ResolvedConfigLayer[]): TelemetryConfig | undefined {
  const found = layers.find((layer) => {
    const j = layer.json as { telemetry?: Record<string, unknown> } | undefined;
    return j?.telemetry !== undefined;
  });
  if (found === undefined) return undefined;
  const source = found.json as { telemetry?: Record<string, unknown> } | undefined;
  const telemetryRaw = source?.telemetry;
  if (telemetryRaw === undefined) return undefined;
  const env = found.env ?? {};
  const expanded = expandPlaceholders(telemetryRaw, (name: string) => {
    const envVal = Object.prototype.hasOwnProperty.call(env, name) ? env[name] : undefined;
    const v = envVal ?? process.env[name];
    return v ?? '';
  }) as TelemetryConfig;
  return expanded;
}

export function buildUnifiedConfiguration(
  needs: { providers: string[]; mcpServers: string[]; restTools: string[] },
  layers: ResolvedConfigLayer[],
  opts?: ResolverOptions
): Configuration {
  const providers: Record<string, ProviderConfig> = {};
  const mcpServers: Record<string, MCPServerConfig> = {};
  const restTools: Record<string, RestToolConfig> = {};
  const openapiSpecs: Record<string, OpenAPISpecConfig> = {};

  needs.providers.forEach((p) => {
    const conf = resolveProvider(p, layers, opts);
    if (conf !== undefined) providers[p] = conf;
  });
  needs.mcpServers.forEach((s) => {
    const conf = resolveMCPServer(s, layers, opts);
    if (conf !== undefined) mcpServers[s] = conf;
  });

  needs.restTools.forEach((r) => {
    const conf = resolveRestTool(r, layers);
    if (conf !== undefined) restTools[r] = conf;
  });

  // Merge openapiSpecs similarly; last file wins by key
  layers.forEach((layer) => {
    const j = layer.json as { openapiSpecs?: Record<string, unknown> } | undefined;
    const specs = j?.openapiSpecs;
    if (specs !== undefined) {
      const env = layer.env ?? {};
      Object.entries(specs).forEach(([name, raw]) => {
        const expanded = expandPlaceholders(raw, (varName: string) => {
          const envVal = Object.prototype.hasOwnProperty.call(env, varName) ? env[varName] : undefined;
          const v = envVal ?? process.env[varName];
          if (v === undefined) return '';
          return v;
        }) as OpenAPISpecConfig;
        openapiSpecs[name] = expanded;
      });
    }
  });

  const defaults = resolveDefaults(layers);
  const accounting = resolveAccounting(layers, opts);
  const pricing = resolvePricing(layers);
  const telemetry = resolveTelemetry(layers);

  return { providers, mcpServers, restTools, openapiSpecs, defaults, accounting, pricing, telemetry } as Configuration;
}
