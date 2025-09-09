import slackPkg from '@slack/bolt';
import express from 'express';

import { loadConfiguration } from '../config.js';
import { discoverLayers } from '../config-resolver.js';
import { buildApiRouter } from './api.js';
import { SessionManager } from './session-manager.js';
import { formatLog } from '../log-formatter.js';
import { initSlackHeadend } from './slack.js';
import { loadAgent } from '../agent-loader.js';
// No FORMAT resolution here; handled centrally in ai-agent using configuration
// no direct prompt file parsing here; handled by agent-loader and ai-agent

const { App, LogLevel } = slackPkg as any;

export async function startServer(agentPath: string, options?: { enableSlack?: boolean; enableApi?: boolean; verbose?: boolean; traceLlm?: boolean; traceMcp?: boolean }): Promise<void> {
  const config = loadConfiguration(undefined);

  // Load agent and compute system prompt from file body
  const loaded = loadAgent(agentPath, undefined, { verbose: options?.verbose, traceLLM: options?.traceLlm, traceMCP: options?.traceMcp });
  // Use system template loaded by agent-loader (single source of truth)
  let systemPrompt = loaded.systemTemplate;


  const runner = async (sys: string, user: string, opts: any) => {
    // Decide output format strictly per headend
    const expectedJson = loaded.expectedOutput?.format === 'json';
    let outputFormat: string;
    if (opts?.renderTarget === 'slack') {
      outputFormat = expectedJson ? 'json' : 'slack-block-kit';
    } else if (opts?.renderTarget === 'api') {
      outputFormat = expectedJson ? 'json' : 'markdown';
    } else if (opts?.renderTarget === 'web') {
      outputFormat = expectedJson ? 'json' : 'markdown';
    } else {
      outputFormat = expectedJson ? 'json' : 'markdown';
    }
    // Allow extra headend-specific fields (e.g., abortSignal) without narrowing here
    return loaded.run(sys, user, ({ ...opts, outputFormat } as any));
  };

  const logSink = (entry: any) => {
    const line = formatLog(entry, { color: true, verbose: options?.verbose === true, traceLlm: options?.traceLlm === true, traceMcp: options?.traceMcp === true });
    if (line.length > 0) { try { process.stderr.write(`${line}\n`); } catch { /* ignore */ } }
  };

  const sessions = new SessionManager(runner, { onLog: logSink });

  const app = express();
  app.get('/health', (_req: any, res: any) => { res.status(200).send('OK'); });
  // Central error handler: always log detailed errors to stderr
  // Must be registered after routes; we register it at the end as well for safety
  const errorHandler = (err: unknown, req: any, res: any, _next: any): void => {
    const message = err instanceof Error ? err.message : String(err);
    const stack = err instanceof Error && typeof err.stack === 'string' ? err.stack : undefined;
    const method = req?.method; const url = req?.originalUrl ?? req?.url; const ip = req?.ip;
    const bodyLen = (() => { try { return typeof req?.rawBody === 'string' ? Buffer.byteLength(req.rawBody, 'utf8') : JSON.stringify(req?.body ?? {}).length; } catch { return 0; } })();
    const line = `[SRV] ← [0.0] server api: ${method} ${url} from ${String(ip)} failed: ${message}${stack ? `\n${stack}` : ''} (bodySize=${String(bodyLen)} bytes)`;
    try { process.stderr.write(`${line}\n`); } catch { /* ignore */ }
    try { res.status(500).json({ error: 'internal_error', message }); } catch { /* ignore */ }
  };
  // Resolve slack/api with .ai-agent.env support (same paradigm as CLI)
  const layers = discoverLayers({ configPath: undefined });
  const mergeEnv = (a: Record<string, string> = {}, b: Record<string, string> = {}) => ({ ...b, ...a });
  const expandStrict = (obj: unknown, vars: Record<string, string>, section: string): unknown => {
    if (typeof obj === 'string') return obj.replace(/\$\{([^}]+)\}/g, (_m, n: string) => {
      const v = vars[n];
      if (v === undefined) throw new Error(`Unresolved variable \${${n}} in section '${section}' (Slack/API). Define it in .ai-agent.env or environment.`);
      return v;
    });
    if (Array.isArray(obj)) return obj.map((v) => expandStrict(v, vars, section));
    if (obj !== null && typeof obj === 'object') {
      const out: Record<string, unknown> = {};
      Object.entries(obj as Record<string, unknown>).forEach(([k, v]) => { out[k] = expandStrict(v, vars, section); });
      return out;
    }
    return obj;
  };
  const resolveSection = (name: 'slack' | 'api'): Record<string, unknown> => {
    const found = layers.find((ly) => ly.json && typeof ly.json[name] === 'object');
    if (!found) {
      const fallback = (config as unknown as Record<string, unknown>)[name];
      return (fallback as Record<string, unknown>) ?? {};
    }
    const envVars = mergeEnv(found.env ?? {}, process.env as Record<string, string>);
    const rawVal = (found.json as Record<string, unknown>)[name];
    if (rawVal === undefined || rawVal === null) return {};
    return (expandStrict(rawVal, envVars, name) as Record<string, unknown>) ?? {};
  };
  const apiResolved = resolveSection('api') as { enabled?: boolean; port?: number; bearerKeys?: string[] };
  const slackResolved = resolveSection('slack') as { enabled?: boolean; mentions?: boolean; dms?: boolean; updateIntervalMs?: number; historyLimit?: number; historyCharsCap?: number; botToken?: string; appToken?: string; openerTone?: 'random' | 'cheerful' | 'formal' | 'busy' };

  const apiCfg = apiResolved ?? (config.api ?? {});
  const wantApi = options?.enableApi ?? (apiCfg.enabled !== false);
  if (wantApi) {
    const bearerKeys = Array.isArray(apiCfg.bearerKeys) ? apiCfg.bearerKeys.filter((k): k is string => typeof k === 'string' && k.length > 0) : [];
    // Pass raw template; ai-agent will replace FORMAT based on configuration
    app.use('/api', buildApiRouter(sessions, { bearerKeys, systemPrompt }));
  }

  const slackCfg = slackResolved ?? (config.slack ?? {});
  const slackBotToken = slackCfg.botToken as string | undefined;
  const slackAppToken = slackCfg.appToken as string | undefined;
  const wantSlack = options?.enableSlack ?? (slackCfg.enabled !== false);
  if (wantSlack && slackBotToken && slackAppToken) {
    if (loaded.expectedOutput?.format === 'json') {
      throw new Error('Slack headend cannot run agents with JSON final_report. Use API/CLI for JSON outputs.');
    }
    const slackApp = new App({ token: slackBotToken, appToken: slackAppToken, socketMode: true, logLevel: LogLevel.WARN });
    // Pass raw template; ai-agent will replace FORMAT based on configuration
    const slackSystem = systemPrompt;
    initSlackHeadend({
      sessionManager: sessions,
      app: slackApp,
      historyLimit: slackCfg.historyLimit ?? 30,
      historyCharsCap: slackCfg.historyCharsCap ?? 8000,
      updateIntervalMs: slackCfg.updateIntervalMs ?? 2000,
      enableMentions: slackCfg.mentions ?? true,
      enableDMs: slackCfg.dms ?? true,
      systemPrompt: slackSystem,
      openerTone: slackCfg.openerTone ?? 'random',
      verbose: options?.verbose === true
    });
    await slackApp.start();
    try { process.stderr.write(`[SRV] ← [0.0] server slack: ⚡ Slack headend connected (Socket Mode).\n`); } catch { /* ignore */ }
  } else {
    try { process.stderr.write(`[SRV] ← [0.0] server slack: Slack headend not started (missing SLACK_BOT_TOKEN or SLACK_APP_TOKEN).\n`); } catch { /* ignore */ }
  }

  const port = apiCfg.port ?? 8080;
  app.listen(port, () => {
    try { process.stderr.write(`[SRV] ← [0.0] server api: Server listening on http://localhost:${String(port)}\n`); } catch { /* ignore */ }
  });
  // Register error handler last
  // eslint-disable-next-line @typescript-eslint/no-misused-promises
  app.use(errorHandler as any);
}
