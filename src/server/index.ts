import slackPkg from '@slack/bolt';
import express from 'express';
import crypto from 'node:crypto';
import path from 'node:path';
import fs from 'node:fs';

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

  // Helper: compile wildcard pattern to RegExp
  const escapeRegex = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const globToRegex = (pat: string): RegExp => {
    const esc = escapeRegex(pat).replace(/\\\*/g, '.*').replace(/\\\?/g, '.');
    return new RegExp(`^${esc}$`, 'i');
  };
  interface CompiledRule {
    channelNamePatterns: RegExp[];
    channelIdPatterns: RegExp[];
    agentPath: string;
    engage: Set<'mentions' | 'channel-posts' | 'dms'>;
    promptTemplates?: { mention?: string; dm?: string; channelPost?: string };
    contextPolicy?: { channelPost?: 'selfOnly' | 'previousOnly' | 'selfAndPrevious' };
  }
  interface CompiledRouting {
    default?: Omit<CompiledRule, 'channelNamePatterns' | 'channelIdPatterns'>;
    denies: { channelNamePatterns: RegExp[]; channelIdPatterns: RegExp[]; engage: Set<'mentions' | 'channel-posts' | 'dms'> }[];
    rules: CompiledRule[];
  }

  const compileRouting = (slackCfg: any): CompiledRouting => {
    const r = (slackCfg?.routing ?? {}) as Record<string, unknown>;
    const out: CompiledRouting = { default: undefined, denies: [], rules: [] };
    const parseChannels = (arr: unknown): { namePats: RegExp[]; idPats: RegExp[] } => {
      const a = Array.isArray(arr) ? arr : [];
      const namePats: RegExp[] = [];
      const idPats: RegExp[] = [];
      a.forEach((raw) => {
        const s = typeof raw === 'string' ? raw.trim() : '';
        if (s.length === 0) return;
        if (s.startsWith('C') || s.startsWith('G')) { idPats.push(globToRegex(s)); return; }
        const n = s.startsWith('#') ? s.slice(1) : s;
        namePats.push(globToRegex(n));
      });
      return { namePats, idPats };
    };
    const toEngage = (arr: unknown): Set<'mentions'|'channel-posts'|'dms'> => new Set((Array.isArray(arr) ? arr : []).filter((x): x is 'mentions'|'channel-posts'|'dms' => x === 'mentions' || x === 'channel-posts' || x === 'dms'));
    // default
    if (r.default && typeof r.default === 'object') {
      const d = r.default as Record<string, unknown>;
      const dEngage = toEngage(d.engage);
      const dAgent = (typeof d.agent === 'string' && d.agent.length > 0) ? d.agent : agentPath;
      out.default = {
        agentPath: dAgent,
        engage: dEngage,
        promptTemplates: (d.promptTemplates as any) ?? undefined,
        contextPolicy: (d.contextPolicy as any) ?? undefined,
      } as any;
    }
    // rules
    const rules = Array.isArray(r.rules) ? r.rules as Record<string, unknown>[] : [];
    rules.forEach((ru) => {
      const { namePats, idPats } = parseChannels(ru.channels);
      const engage = toEngage(ru.engage);
      const ruAgent = (typeof ru.agent === 'string' && ru.agent.length > 0) ? ru.agent : agentPath;
      out.rules.push({
        channelNamePatterns: namePats,
        channelIdPatterns: idPats,
        agentPath: ruAgent,
        engage,
        promptTemplates: (ru.promptTemplates as any) ?? undefined,
        contextPolicy: (ru.contextPolicy as any) ?? undefined,
      });
    });
    // deny
    const denies = Array.isArray(r.deny) ? r.deny as Record<string, unknown>[] : [];
    denies.forEach((de) => {
      const { namePats, idPats } = parseChannels(de.channels);
      const engage = toEngage(de.engage);
      out.denies.push({ channelNamePatterns: namePats, channelIdPatterns: idPats, engage });
    });
    return out;
  };

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
  const slackResolved = resolveSection('slack') as { enabled?: boolean; mentions?: boolean; dms?: boolean; updateIntervalMs?: number; historyLimit?: number; historyCharsCap?: number; botToken?: string; appToken?: string; openerTone?: 'random' | 'cheerful' | 'formal' | 'busy'; routing?: unknown };

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
  const slackSigningSecret = (slackCfg as { signingSecret?: string }).signingSecret;
  const wantSlack = options?.enableSlack ?? (slackCfg.enabled !== false);
  if (wantSlack && slackBotToken && slackAppToken) {
    if (loaded.expectedOutput?.format === 'json') {
      throw new Error('Slack headend cannot run agents with JSON final_report. Use API/CLI for JSON outputs.');
    }
    const slackApp = new App({ token: slackBotToken, appToken: slackAppToken, socketMode: true, logLevel: LogLevel.WARN });
    // Compile routing
    const compiled = compileRouting(slackCfg);
    // Preload agents referenced in routing (including default and current agentPath)
    const agentBaseDir = path.dirname(agentPath);
    const resolveAgentPath = (p: string): string => (path.isAbsolute(p) ? p : path.resolve(agentBaseDir, p));
    const agentCache = new Map<string, { loaded: ReturnType<typeof loadAgent>; sessions: SessionManager }>();
    const preload = (p: string): void => {
      const abs = resolveAgentPath(p);
      if (agentCache.has(abs)) return;
      if (!fs.existsSync(abs)) {
        throw new Error(`Slack routing references missing agent file: ${abs}`);
      }
      const loaded2 = loadAgent(abs, undefined, { verbose: options?.verbose, traceLLM: options?.traceLlm, traceMCP: options?.traceMcp });
      if (loaded2.expectedOutput?.format === 'json') throw new Error(`Slack headend cannot run JSON-output agent: ${abs}`);
      const run2 = async (sys: string, usr: string, opts: any) => {
        const expectedJson = loaded2.expectedOutput?.format === 'json';
        const outputFormat = expectedJson ? 'json' : (opts?.renderTarget === 'slack' ? 'slack-block-kit' : 'markdown');
        return loaded2.run(sys, usr, ({ ...opts, outputFormat } as any));
      };
      const sm = new SessionManager(run2, { onLog: logSink });
      agentCache.set(abs, { loaded: loaded2, sessions: sm });
    };
    // Default
    preload(compiled.default?.agentPath ?? agentPath);
    // Rules
    compiled.rules.forEach((ru) => { preload(ru.agentPath); });

    // Emit a minimal routing validation summary for sanity checks
    try {
      const fmtSet = (s: Set<'mentions'|'channel-posts'|'dms'> | undefined): string => {
        if (!s || s.size === 0) return '(none)';
        const arr = Array.from(s.values());
        arr.sort((a, b) => (a < b ? -1 : (a > b ? 1 : 0)));
        return arr.join(',');
      };
      const fmtRes = (arr: RegExp[]): string => (arr.length === 0 ? '-' : arr.map((r) => `/${r.source}/i`).join(' '));
      const lines: string[] = [];
      lines.push('[SRV] ← [0.0] server slack: routing summary:');
      if (compiled.default) {
        const abs = resolveAgentPath(compiled.default.agentPath);
        lines.push(`  default: agent=${abs} engage=[${fmtSet(compiled.default.engage)}]`);
      } else {
        lines.push('  default: (none)');
      }
      if (compiled.denies.length > 0) {
        lines.push('  denies:');
        compiled.denies.forEach((d, i) => {
          lines.push(`    [${String(i)}] engage=[${fmtSet(d.engage)}] namePats=${fmtRes(d.channelNamePatterns)} idPats=${fmtRes(d.channelIdPatterns)}`);
        });
      }
      if (compiled.rules.length > 0) {
        lines.push('  rules:');
        compiled.rules.forEach((ru, i) => {
          const abs = resolveAgentPath(ru.agentPath);
          lines.push(`    [${String(i)}] agent=${abs} engage=[${fmtSet(ru.engage)}] namePats=${fmtRes(ru.channelNamePatterns)} idPats=${fmtRes(ru.channelIdPatterns)}`);
        });
      }
      lines.push('');
      try { process.stderr.write(`${lines.join('\n')}\n`); } catch { /* ignore */ }
    } catch { /* ignore */ }

    // Resolver passed to headend
    const resolveRoute = async (args: { kind: 'mentions'|'channel-posts'|'dms'; channelId: string; channelName?: string }): Promise<{
      sessions: SessionManager; systemPrompt: string; promptTemplates?: { mention?: string; dm?: string; channelPost?: string }; contextPolicy?: { channelPost?: 'selfOnly'|'previousOnly'|'selfAndPrevious' }
    } | undefined> => {
      const { kind, channelId, channelName } = args;
      // deny first
      for (const d of compiled.denies) {
        if (d.engage.size > 0 && !d.engage.has(kind)) continue;
        if (d.channelIdPatterns.some((re) => re.test(channelId))) return undefined;
        if (channelName && d.channelNamePatterns.some((re) => re.test(channelName))) return undefined;
      }
      // rules
      for (const ru of compiled.rules) {
        if (ru.engage.size > 0 && !ru.engage.has(kind)) continue;
        const idMatch = ru.channelIdPatterns.some((re) => re.test(channelId));
        const nameMatch = channelName ? ru.channelNamePatterns.some((re) => re.test(channelName)) : false;
        if (idMatch || nameMatch) {
          const abs = resolveAgentPath(ru.agentPath);
          const rec = agentCache.get(abs);
          if (!rec) {
            try { process.stderr.write(`[SRV] ← [0.0] server slack: ⚠️  routing warning: agent not preloaded/resolved: ${abs}\n`); } catch { /* ignore */ }
            continue;
          }
          return { sessions: rec.sessions, systemPrompt: rec.loaded.systemTemplate, promptTemplates: ru.promptTemplates, contextPolicy: ru.contextPolicy };
        }
      }
      // default
      const d = compiled.default;
      const engageOk = d?.engage && d.engage.size > 0 ? d.engage.has(kind) : (kind === 'mentions' ? (slackCfg.mentions ?? true) : (kind === 'dms' ? (slackCfg.dms ?? true) : false));
      if (!engageOk) return undefined;
      const abs = resolveAgentPath(d?.agentPath ?? agentPath);
      const rec = agentCache.get(abs);
      if (!rec) return undefined;
      return { sessions: rec.sessions, systemPrompt: rec.loaded.systemTemplate, promptTemplates: d?.promptTemplates, contextPolicy: d?.contextPolicy };
    };

    initSlackHeadend({
      // default fallbacks retained for legacy behavior; resolver governs actual routing
      sessionManager: sessions,
      app: slackApp,
      historyLimit: slackCfg.historyLimit ?? 30,
      historyCharsCap: slackCfg.historyCharsCap ?? 8000,
      updateIntervalMs: slackCfg.updateIntervalMs ?? 2000,
      enableMentions: slackCfg.mentions ?? true,
      enableDMs: slackCfg.dms ?? true,
      systemPrompt: systemPrompt,
      openerTone: slackCfg.openerTone ?? 'random',
      verbose: options?.verbose === true,
      resolveRoute,
    });
    // Add Socket Mode reconnection event listeners
    // Access the SocketModeClient through the receiver
    const receiver = slackApp.receiver as any;
    if (receiver?.client?.on) {
      receiver.client.on('connected', () => {
        try { process.stderr.write(`[SRV] ← [0.0] server slack: ✅ Socket Mode reconnected to Slack.\n`); } catch { /* ignore */ }
      });
      receiver.client.on('disconnected', () => {
        try { process.stderr.write(`[SRV] ← [0.0] server slack: ⚠️  Socket Mode disconnected from Slack.\n`); } catch { /* ignore */ }
      });
      receiver.client.on('reconnecting', () => {
        try { process.stderr.write(`[SRV] ← [0.0] server slack: 🔄 Socket Mode reconnecting to Slack...\n`); } catch { /* ignore */ }
      });
    }
    
    await slackApp.start();
    try { process.stderr.write(`[SRV] ← [0.0] server slack: ⚡ Slack headend connected (Socket Mode).\n`); } catch { /* ignore */ }

    // Optional: Slash commands public endpoint (requires external HTTPS in production)
    if (typeof slackSigningSecret === 'string' && slackSigningSecret.length > 0) {
      // Capture raw body for signature verification
      const rawBodySaver = (req: any, _res: any, buf: Buffer): void => { try { req.rawBody = buf.toString('utf8'); } catch { /* ignore */ } };
      app.use('/slack/commands', express.urlencoded({ extended: false, verify: rawBodySaver }));
      const timingSafeEq = (a: Buffer, b: Buffer): boolean => { try { return crypto.timingSafeEqual(a, b); } catch { return false; } };
      const verifySlackSig = (req: any): boolean => {
        const ts = req.get('x-slack-request-timestamp');
        const sig = req.get('x-slack-signature');
        const body = typeof req.rawBody === 'string' ? req.rawBody : '';
        if (!ts || !sig || !body) return false;
        const fiveMin = 60 * 5;
        const now = Math.floor(Date.now() / 1000);
        const age = Math.abs(now - Number.parseInt(ts, 10));
        if (!Number.isFinite(age) || age > fiveMin) return false;
        const basestring = `v0:${ts}:${body}`;
        const hmac = crypto.createHmac('sha256', slackSigningSecret).update(basestring).digest('hex');
        const expected = `v0=${hmac}`;
        return timingSafeEq(Buffer.from(expected), Buffer.from(sig));
      };
      app.post('/slack/commands', async (req: any, res: any) => {
        try {
          if (!verifySlackSig(req)) { try { res.status(401).send('invalid'); } catch { /* ignore */ } return; }
          const body = req.body as Record<string, string>;
          const command = body.command; const text = body.text; const userId = body.user_id; const channelId = body.channel_id; const teamId = body.team_id; const responseUrl = body.response_url;
          if (!command || !userId || !channelId) { try { res.status(400).send('bad'); } catch { /* ignore */ } return; }
          // Fast ack (ephemeral)
          try { res.status(200).json({ response_type: 'ephemeral', text: 'Working…' }); } catch { /* ignore */ }
          // Resolve channel name and route
          const chInfo = await slackApp.client.conversations.info({ channel: channelId });
          const chName = chInfo?.channel?.name as string | undefined;
          const routeKind: 'mentions'|'channel-posts'|'dms' = 'channel-posts';
          const resolvedRoute = await resolveRoute({ kind: routeKind, channelId, channelName: chName });
          const activeSessions = resolvedRoute?.sessions ?? sessions;
          const activeSystem = resolvedRoute?.systemPrompt ?? systemPrompt;
          // Template
          const when = new Date().toLocaleString();
          const whoLabel = await (async () => {
            try {
              const u = await slackApp.client.users.info({ user: userId });
              const p = (u?.user?.profile ?? {}) as { display_name?: string; real_name?: string };
              const dn = (p.display_name ?? '').trim();
              const rn = (p.real_name ?? '').trim();
              return rn.length > 0 ? rn : (dn.length > 0 ? dn : userId);
            } catch { return userId; }
          })();
          const UNKNOWN = 'unknown';
          const tpl = resolvedRoute?.promptTemplates?.channelPost ?? [
            'You are responding to a channel post.',
            'Channel: {channel.name} ({channel.id})',
            'User: {user.label} ({user.id})',
            'Time: {ts}',
            'Message: {text}',
            '',
            'Constraints:',
            '1) Be accurate.',
            '2) Do not reveal internal data unless explicitly allowed for this channel persona.',
            '3) Prefer short, direct answers.',
          ].join('\n');
          const userPrompt = tpl
            .replace(/{channel\.name}/g, (chName ?? UNKNOWN))
            .replace(/{channel\.id}/g, channelId)
            .replace(/{user\.id}/g, userId)
            .replace(/{user\.label}/g, whoLabel)
            .replace(/{ts}/g, when)
            .replace(/{text}/g, text ?? '')
            .replace(/{message\.url}/g, '');
          // Decide target: channel if member, else DM fallback
          const isMember = await (async () => { try { const i = await slackApp.client.conversations.info({ channel: channelId }); return i?.channel?.is_member === true; } catch { return false; } })();
          let targetChannel = channelId; let threadTs: string | undefined = undefined;
          if (!isMember) {
            try { await fetch(responseUrl, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ response_type: 'ephemeral', text: 'Not a channel member; continuing in DM…' }) }); } catch { /* ignore */ }
            const dm = await slackApp.client.conversations.open({ users: userId });
            const dmChannel = dm?.channel?.id as string | undefined; if (!dmChannel) return;
            targetChannel = dmChannel;
          }
          const opener = `Starting… ${whoLabel !== undefined && whoLabel !== null && (typeof whoLabel === 'string' ? whoLabel.length > 0 : false) ? `(${whoLabel})` : ''}`;
          const initial = await slackApp.client.chat.postMessage({ channel: targetChannel, thread_ts: threadTs, text: opener });
          const liveTs = String(initial.ts ?? threadTs ?? Date.now());
          const runId = activeSessions.startRun({ source: 'slack', teamId, channelId: targetChannel, threadTsOrSessionId: liveTs }, activeSystem, userPrompt, [], { initialTitle: undefined });
          // Simple polling; post final on completion
          const started = Date.now(); const timeoutMs = 60_000;
          const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
          // eslint-disable-next-line functional/no-loop-statements
          while (Date.now() - started < timeoutMs) {
            const meta = activeSessions.getRun(runId);
            if (meta && meta.status !== 'running') break;
            // eslint-disable-next-line no-await-in-loop
            await sleep(500);
          }
          const resMeta = activeSessions.getRun(runId);
          const outText = (() => { const r = activeSessions.getOutput(runId) ?? ''; if (r.length > 0) return r; const rr = activeSessions.getResult(runId); if (rr?.finalReport?.content) return rr.finalReport.content; return resMeta?.error ? `❌ ${resMeta.error}` : '✅ Done'; })();
          await slackApp.client.chat.postMessage({ channel: targetChannel, thread_ts: liveTs, text: outText.substring(0, 3500) });
        } catch (e) {
          try { res.status(500).send('error'); } catch { /* ignore */ }
          try { process.stderr.write(`[SRV] ← [0.0] server slack: slash command failed: ${(e as Error).message}\n`); } catch { /* ignore */ }
        }
      });
      try { process.stderr.write(`[SRV] ← [0.0] server api: 🛰️  Slash commands endpoint mounted at /slack/commands (needs public HTTPS).\n`); } catch { /* ignore */ }
    }
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
