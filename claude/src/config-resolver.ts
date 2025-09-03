import fs from 'node:fs';
import path from 'node:path';

import type { Configuration, MCPServerConfig, ProviderConfig } from './types.js';

type LayerOrigin = '--config' | 'cwd' | 'binary' | 'home' | 'system';

export interface ConfigLayer {
  origin: LayerOrigin;
  jsonPath: string; // may not exist
  envPath: string;  // may not exist
  json?: Record<string, unknown>;
  env?: Record<string, string>;
}

export interface ResolverOptions {
  verbose?: boolean;
  log?: (msg: string) => void;
}

function readJSONIfExists(p: string): Record<string, unknown> | undefined {
  try {
    if (fs.existsSync(p)) {
      const raw = fs.readFileSync(p, 'utf-8');
      return JSON.parse(raw) as Record<string, unknown>;
    }
  } catch { /* ignore */ }
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
  try {
    if (fs.existsSync(p)) {
      const raw = fs.readFileSync(p, 'utf-8');
      return parseEnvFile(raw);
    }
  } catch { /* ignore */ }
  return undefined;
}

export function discoverLayers(opts?: { configPath?: string }): ConfigLayer[] {
  const layers: ConfigLayer[] = [];

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
  list.push({ origin: 'binary', json: path.join(binDir, HIDDEN_JSON), env: path.join(binDir, HIDDEN_ENV) });
  if (home.length > 0) {
    list.push({ origin: 'home', json: path.join(home, HOME_DIR, 'ai-agent.json'), env: path.join(home, HOME_DIR, 'ai-agent.env') });
  }
  list.push({ origin: 'system', json: path.join(SYSTEM_DIR, 'ai-agent.json'), env: path.join(SYSTEM_DIR, 'ai-agent.env') });

  // Highest priority first (as constructed)
  list.forEach((it) => {
    const layer: ConfigLayer = {
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

function expandPlaceholders(obj: unknown, vars: (name: string) => string | undefined): unknown {
  if (typeof obj === 'string') {
    return obj.replace(/\$\{([^}]+)\}/g, (_m: string, name: string) => vars(name) ?? '');
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

function logWarn(opts: ResolverOptions | undefined, msg: string): void {
  if (opts?.verbose === true) opts.log?.(`[resolver] ${msg}`);
}
function buildMissingVarMsg(scope: 'provider'|'mcp', id: string, origin: LayerOrigin, name: string): string {
  return `missing env var '${name}' for ${scope} '${id}' at ${origin}; falling back to empty`;
}

export function resolveProvider(id: string, layers: ConfigLayer[], opts?: ResolverOptions): ProviderConfig | undefined {
  const found = layers.find((layer) => {
    const provs = layer.json?.providers as Record<string, unknown> | undefined;
    return provs !== undefined && Object.prototype.hasOwnProperty.call(provs, id);
  });
  if (found === undefined) return undefined;
  const j = found.json as { providers?: Record<string, unknown> } | undefined;
  const provs = j?.providers ?? {};
  const obj = provs[id] as Record<string, unknown>;
  const env = found.env ?? {};
  const vars = (name: string): string | undefined => (env[name] ?? process.env[name]);
  const expanded = expandPlaceholders(obj, (name: string) => {
    const v = vars(name);
    if (v === undefined) logWarn(opts, buildMissingVarMsg('provider', id, found.origin, name));
    return v;
  }) as ProviderConfig;
  return expanded;
}

export function resolveMCPServer(id: string, layers: ConfigLayer[], opts?: ResolverOptions): MCPServerConfig | undefined {
  const found = layers.find((layer) => {
    const srvs = layer.json?.mcpServers as Record<string, unknown> | undefined;
    return srvs !== undefined && Object.prototype.hasOwnProperty.call(srvs, id);
  });
  if (found === undefined) return undefined;
  const j = found.json as { mcpServers?: Record<string, unknown> } | undefined;
  const srvs = j?.mcpServers ?? {};
  const obj = srvs[id] as Record<string, unknown>;
  const env = found.env ?? {};
  const vars = (name: string): string | undefined => (env[name] ?? process.env[name]);
  const expanded = expandPlaceholders(obj, (name: string) => {
    const v = vars(name);
    if (v === undefined) logWarn(opts, buildMissingVarMsg('mcp', id, found.origin, name));
    return v;
  }) as MCPServerConfig;
  return expanded;
}

export function resolveDefaults(layers: ConfigLayer[]): NonNullable<Configuration['defaults']> {
  const out: NonNullable<Configuration['defaults']> = {};
  const keys = [
    'llmTimeout','toolTimeout','temperature','topP','stream','parallelToolCalls','maxToolTurns','maxRetries','toolResponseMaxBytes'
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

export function resolveAccounting(layers: ConfigLayer[]): Configuration['accounting'] {
  const found = layers.find((layer) => {
    const j = layer.json as { accounting?: { file?: string } } | undefined;
    return typeof j?.accounting?.file === 'string';
  });
  if (found === undefined) return undefined;
  const j = found.json as { accounting?: { file?: string } } | undefined;
  const acc = j?.accounting;
  return acc !== undefined && typeof acc.file === 'string' ? { file: acc.file } : undefined;
}

export function buildUnifiedConfiguration(
  needs: { providers: string[]; mcpServers: string[] },
  layers: ConfigLayer[],
  opts?: ResolverOptions
): Configuration {
  const providers: Record<string, ProviderConfig> = {};
  const mcpServers: Record<string, MCPServerConfig> = {};

  needs.providers.forEach((p) => {
    const conf = resolveProvider(p, layers, opts);
    if (conf !== undefined) providers[p] = conf;
  });
  needs.mcpServers.forEach((s) => {
    const conf = resolveMCPServer(s, layers, opts);
    if (conf !== undefined) mcpServers[s] = conf;
  });

  const defaults = resolveDefaults(layers);
  const accounting = resolveAccounting(layers);

  return { providers, mcpServers, defaults, accounting } as Configuration;
}
