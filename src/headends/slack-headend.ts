import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";

import slackBolt from "@slack/bolt";

import type { RestExtraRoute } from "./rest-headend.js";
import type {
  Headend,
  HeadendClosedEvent,
  HeadendContext,
  HeadendDescription,
} from "./types.js";

import {
  loadAgent,
  LoadedAgentCache,
  type LoadAgentOptions,
  type LoadedAgent,
} from "../agent-loader.js";
import { parseDurationMs } from "../cache/ttl.js";
import { discoverLayers } from "../config-resolver.js";
import { SessionManager } from "../server/session-manager.js";
import { initSlackHeadend } from "../server/slack.js";
import { getTelemetryLabels } from "../telemetry/index.js";

import { ConcurrencyLimiter } from "./concurrency.js";

type BoltAppCtor = new (...args: unknown[]) => {
  start: () => Promise<void>;
  stop: () => Promise<void>;
  client: unknown;
};
type BoltLogLevel = Record<string, unknown>;

const App: BoltAppCtor = (slackBolt as { App: BoltAppCtor }).App;

const LogLevel: BoltLogLevel = (slackBolt as { LogLevel: BoltLogLevel })
  .LogLevel;

/* eslint-disable @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access, @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-unsafe-return, @typescript-eslint/strict-boolean-expressions, @typescript-eslint/no-unnecessary-condition, @typescript-eslint/no-confusing-void-expression, promise/prefer-await-to-then, @typescript-eslint/restrict-template-expressions, @typescript-eslint/non-nullable-type-assertion-style, @typescript-eslint/no-explicit-any, @typescript-eslint/no-unnecessary-type-assertion */

type SlackOpenerTone = "random" | "cheerful" | "formal" | "busy";
interface SlackHeadendOptions {
  agentPaths: string[];
  loadOptions: LoadAgentOptions;
  verbose?: boolean;
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSdk?: boolean;
  traceSlack?: boolean;
}

type SlackClient = unknown;

interface RoutingResolution {
  sessions: SessionManager;
  systemPrompt: string;
  promptTemplates?: { mention?: string; dm?: string; channelPost?: string };
  contextPolicy?: {
    channelPost?: "selfOnly" | "previousOnly" | "selfAndPrevious";
  };
}

interface CompiledRoute {
  channelNamePatterns: RegExp[];
  channelIdPatterns: RegExp[];
  agentPath: string;
  engage: Set<"mentions" | "channel-posts" | "dms">;
  promptTemplates?: { mention?: string; dm?: string; channelPost?: string };
  contextPolicy?: {
    channelPost?: "selfOnly" | "previousOnly" | "selfAndPrevious";
  };
}

interface CompiledRouting {
  default?: Omit<CompiledRoute, "channelNamePatterns" | "channelIdPatterns">;
  denies: {
    channelNamePatterns: RegExp[];
    channelIdPatterns: RegExp[];
    engage: Set<"mentions" | "channel-posts" | "dms">;
  }[];
  rules: CompiledRoute[];
}

interface LoadedAgentRecord {
  loaded: LoadedAgent;
  sessions: SessionManager;
}

export class SlackHeadend implements Headend {
  public readonly id = "slack:socket";
  public readonly kind = "slack";
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly agentPaths: string[];
  private readonly loadOptions: LoadAgentOptions;
  private readonly verbose: boolean;
  private readonly traceLLM: boolean;
  private readonly traceMCP: boolean;
  private readonly traceSdk: boolean;
  private readonly traceSlack: boolean;
  private readonly closeDeferred = this.createDeferred<HeadendClosedEvent>();
  private readonly agentCache = new Map<string, LoadedAgentRecord>();
  private readonly loaderCache = new LoadedAgentCache();
  private readonly userLabelCache = new Map<string, string>();
  private readonly slackLimiter = new ConcurrencyLimiter(10);
  private readonly runReleases = new Map<string, () => void>();
  private readonly registeredSessions = new WeakSet<SessionManager>();
  private readonly telemetryLabels: Record<string, string>;
  private readonly label: string;

  private context?: HeadendContext;
  private slackApp?: InstanceType<typeof App>;
  private slackClient?: SlackClient;
  private defaultSession?: SessionManager;
  private defaultSystemPrompt?: string;
  private resolveRoute?: (args: {
    kind: "mentions" | "channel-posts" | "dms";
    channelId: string;
    channelName?: string;
  }) => Promise<RoutingResolution | undefined>;
  private slackSigningSecret?: string;
  private slashRoute?: RestExtraRoute;
  private slashRouteRegistered = false;
  private fallbackPort?: number;
  private fallbackServer?: http.Server;
  private stopping = false;
  private shutdownSignal?: AbortSignal;
  private globalStopRef?: { stopping: boolean };
  private shutdownListener?: () => void;

  public constructor(options: SlackHeadendOptions) {
    this.agentPaths = options.agentPaths;
    this.loadOptions = options.loadOptions;
    this.verbose = options.verbose === true;
    this.traceLLM = options.traceLLM === true;
    this.traceMCP = options.traceMCP === true;
    this.traceSdk = options.traceSdk === true;
    this.traceSlack = options.traceSlack === true;
    this.closed = this.closeDeferred.promise;
    this.telemetryLabels = { ...getTelemetryLabels(), headend: this.id };
    this.label = "Slack Socket Mode";
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: this.label };
  }

  public getSlashCommandRoute(): RestExtraRoute | undefined {
    return this.slashRoute;
  }

  public markSlashCommandRouteRegistered(): void {
    this.slashRouteRegistered = true;
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.slackApp !== undefined) return;
    this.context = context;
    this.shutdownSignal = context.shutdownSignal;
    this.globalStopRef = context.stopRef;
    if (this.shutdownSignal !== undefined) {
      const handler = () => {
        this.handleShutdownSignal();
      };
      this.shutdownSignal.addEventListener("abort", handler);
      this.shutdownListener = handler;
    }

    this.log("starting");

    const primaryAgentPath = this.agentPaths[0];
    if (primaryAgentPath === undefined) {
      throw new Error("Slack headend requires at least one --agent");
    }
    const primaryPath = primaryAgentPath;

    const {
      slackConfig,
      apiConfig,
      systemPrompt,
      defaultSessions,
      resolveRoute,
    } = this.initializeAgents(primaryPath);

    const slackBotTokenRaw = slackConfig.botToken;
    const slackAppTokenRaw = slackConfig.appToken;
    if (
      typeof slackBotTokenRaw !== "string" ||
      slackBotTokenRaw.length === 0 ||
      typeof slackAppTokenRaw !== "string" ||
      slackAppTokenRaw.length === 0
    ) {
      throw new Error("Slack configuration missing botToken/appToken");
    }
    const slackBotToken = slackBotTokenRaw;
    const slackAppToken = slackAppTokenRaw;
    this.slackSigningSecret =
      typeof slackConfig.signingSecret === "string" &&
      slackConfig.signingSecret.length > 0
        ? slackConfig.signingSecret
        : undefined;

    this.fallbackPort =
      typeof apiConfig.port === "number" && Number.isFinite(apiConfig.port)
        ? apiConfig.port
        : 8080;
    const slackApp = new App({
      token: slackBotToken,
      appToken: slackAppToken,
      socketMode: true,
      logLevel: this.traceSlack ? LogLevel.DEBUG : LogLevel.WARN,
    });
    this.slackApp = slackApp;
    this.slackClient = (slackApp as any).client;
    this.attachSocketModeEventHandlers();
    this.defaultSession = defaultSessions;
    this.defaultSystemPrompt = systemPrompt;
    this.resolveRoute = resolveRoute;

    const historyLimit =
      typeof slackConfig.historyLimit === "number"
        ? slackConfig.historyLimit
        : 100;
    const historyCharsCap =
      typeof slackConfig.historyCharsCap === "number"
        ? slackConfig.historyCharsCap
        : 100000;
    const updateIntervalMs =
      typeof slackConfig.updateIntervalMs === "number"
        ? slackConfig.updateIntervalMs
        : 2000;
    const enableMentions = slackConfig.mentions !== false;
    const enableDMs = slackConfig.dms !== false;
    const openerTone = this.parseOpenerTone(slackConfig.openerTone);

    initSlackHeadend({
      sessionManager: defaultSessions,
      app: this.slackApp,
      historyLimit,
      historyCharsCap,
      updateIntervalMs,
      enableMentions,
      enableDMs,
      systemPrompt,
      verbose: this.verbose,
      openerTone,
      resolveRoute: this.resolveRoute,
      acquireRunSlot: async () => await this.slackLimiter.acquire(),
      registerRunSlot: (session, runId, release) => {
        this.registerRunRelease(session, runId, release);
      },
    });

    this.slashRoute = this.createSlashCommandRoute();
    if (!this.slashRouteRegistered && this.slashRoute !== undefined) {
      const port = this.fallbackPort ?? 0;
      await this.startFallbackServer(port, this.slashRoute);
    }

    await this.slackApp.start();
    this.log("started");
  }

  public async stop(): Promise<void> {
    if (this.stopping) return;
    this.stopping = true;
    this.handleShutdownSignal();
    if (
      this.shutdownListener !== undefined &&
      this.shutdownSignal !== undefined
    ) {
      this.shutdownSignal.removeEventListener("abort", this.shutdownListener);
      this.shutdownListener = undefined;
    }
    const slackApp = this.slackApp;
    if (slackApp !== undefined) {
      try {
        await slackApp.stop();
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        this.log(`stop failed: ${message}`, "WRN");
      }
      this.slackApp = undefined;
      this.slackClient = undefined;
    }
    if (this.fallbackServer !== undefined) {
      await new Promise<void>((resolve) => {
        this.fallbackServer?.close(() => resolve());
      });
      this.fallbackServer = undefined;
    }
    this.closeDeferred.resolve({ reason: "stopped", graceful: true });
  }

  private async startFallbackServer(
    port: number,
    route: RestExtraRoute,
  ): Promise<void> {
    if (this.fallbackServer !== undefined) return;
    const server = http.createServer((req, res) => {
      void (async () => {
        const method = (req.method ?? "GET").toUpperCase();
        const urlRaw = req.url ?? "/";
        const url = new URL(urlRaw, `http://localhost:${String(port)}`);
        const pathNormalized = this.normalizePath(url.pathname);
        if (method === route.method && pathNormalized === route.path) {
          await route.handler({
            req,
            res,
            url,
            requestId: crypto.randomUUID(),
          });
          return;
        }
        res.statusCode = 404;
        res.end("not_found");
      })().catch((err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        this.log(`slash command fallback failed: ${message}`, "ERR");
        if (!res.writableEnded && !res.writableFinished) {
          res.statusCode = 500;
          res.end("internal_error");
        }
      });
    });
    await new Promise<void>((resolve, reject) => {
      server.once("error", reject);
      server.listen(port, () => resolve());
    });
    this.fallbackServer = server;
    this.log(`slash command fallback listening on port ${String(port)}`);
  }

  private createSlashCommandRoute(): RestExtraRoute | undefined {
    if (this.slackSigningSecret === undefined) return undefined;
    return {
      method: "POST",
      path: "/slack/commands",
      handler: async ({ req, res, url, requestId }) => {
        await this.handleSlashCommandRequest(req, res, url, requestId);
      },
    };
  }

  private async handleSlashCommandRequest(
    req: http.IncomingMessage,
    res: http.ServerResponse,
    url: URL,
    requestId: string,
  ): Promise<void> {
    try {
      const rawBody = await this.readRawBody(req);
      if (!this.verifySlackSignature(req, rawBody)) {
        res.statusCode = 401;
        res.end("invalid");
        return;
      }
      const params = new URLSearchParams(rawBody);
      const command = params.get("command");
      const text = params.get("text") ?? "";
      const userId = params.get("user_id") ?? "";
      const channelId = params.get("channel_id") ?? "";
      const teamId = params.get("team_id") ?? "";
      const responseUrl = params.get("response_url") ?? "";
      if (!command || !userId || !channelId) {
        res.statusCode = 400;
        res.end("bad");
        return;
      }

      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ response_type: "ephemeral", text: "Working…" }));

      void this.processSlashCommand({
        command,
        text,
        userId,
        channelId,
        teamId,
        responseUrl,
        requestId,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`slash command handler failed: ${message}`, "ERR");
      if (!res.writableEnded && !res.writableFinished) {
        res.statusCode = 500;
        res.end("error");
      }
    }
  }

  private parseOpenerTone(value: unknown): SlackOpenerTone | undefined {
    if (
      value === "random" ||
      value === "cheerful" ||
      value === "formal" ||
      value === "busy"
    ) {
      return value;
    }
    return undefined;
  }

  private async processSlashCommand(args: {
    command: string;
    text: string;
    userId: string;
    channelId: string;
    teamId: string;
    responseUrl: string;
    requestId: string;
  }): Promise<void> {
    try {
      const clientUnknown = this.slackClient;
      const routeResolver = this.resolveRoute;
      if (
        clientUnknown === undefined ||
        routeResolver === undefined ||
        this.defaultSession === undefined
      )
        return;
      const client = clientUnknown as any;

      const channelInfo = await (client as any).conversations.info({
        channel: args.channelId,
      });
      const channelName =
        typeof channelInfo?.channel?.name === "string"
          ? channelInfo.channel.name
          : undefined;
      const resolvedRoute = await routeResolver({
        kind: "channel-posts",
        channelId: args.channelId,
        channelName,
      });
      const activeSessions = resolvedRoute?.sessions ?? this.defaultSession;
      const activeSystem =
        resolvedRoute?.systemPrompt ?? this.defaultSystemPrompt ?? "";
      if (activeSessions === undefined || activeSystem.length === 0) return;

      const when = new Date().toLocaleString();
      const whoLabel = await this.lookupUserLabel(args.userId);
      const UNKNOWN = "unknown";
      const template =
        resolvedRoute?.promptTemplates?.channelPost ??
        [
          "You are responding to a channel post.",
          "Channel: {channel.name} ({channel.id})",
          "User: {user.label} ({user.id})",
          "Time: {ts}",
          "Message: {text}",
          "",
          "Constraints:",
          "1) Be accurate.",
          "2) Do not reveal internal data unless explicitly allowed for this channel persona.",
          "3) Prefer short, direct answers.",
        ].join("\n");
      const userPrompt = template
        .replace(/{channel\.name}/g, channelName ?? UNKNOWN)
        .replace(/{channel\.id}/g, args.channelId)
        .replace(/{user\.id}/g, args.userId)
        .replace(/{user\.label}/g, whoLabel)
        .replace(/{ts}/g, when)
        .replace(/{text}/g, args.text ?? "")
        .replace(/{message\.url}/g, "");

      const isMember = channelInfo?.channel?.is_member === true;

      let targetChannel = args.channelId;
      if (!isMember) {
        if (args.responseUrl !== "") {
          try {
            await fetch(args.responseUrl, {
              method: "POST",
              headers: { "content-type": "application/json" },
              body: JSON.stringify({
                response_type: "ephemeral",
                text: "Not a channel member; continuing in DM…",
              }),
            });
          } catch {
            /* ignore */
          }
        }
        const dmChannel = await this.openDmChannel(args.userId);
        if (!dmChannel) return;
        targetChannel = dmChannel;
      }

      const opener =
        whoLabel.length > 0 ? `Starting… (${whoLabel})` : "Starting…";
      const initial = await client.chat.postMessage({
        channel: targetChannel,
        text: opener,
      });
      const liveTs = String(initial.ts ?? Date.now());
      const release = await this.slackLimiter.acquire();
      let runId: string;
      try {
        runId = activeSessions.startRun(
          {
            source: "slack",
            teamId: args.teamId,
            channelId: targetChannel,
            threadTsOrSessionId: liveTs,
          },
          activeSystem,
          userPrompt,
          [],
          { initialTitle: undefined },
        );
      } catch (err) {
        release();
        throw err;
      }
      this.registerRunRelease(activeSessions, runId, release);

      void this.pollAndFinalize(activeSessions, runId, targetChannel, liveTs);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`slash command processing failed: ${message}`, "ERR");
      if (args.responseUrl !== "") {
        try {
          await fetch(args.responseUrl, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({
              response_type: "ephemeral",
              text: `Failed: ${message}`,
            }),
          });
        } catch {
          /* ignore */
        }
      }
    }
  }

  private async pollAndFinalize(
    sessionManager: SessionManager,
    runId: string,
    channel: string,
    ts: string,
  ): Promise<void> {
    const sleep = (ms: number) =>
      new Promise((resolve) => {
        setTimeout(resolve, ms);
      });
    const clientUnknown = this.slackClient;
    if (clientUnknown === undefined) return;
    const client = clientUnknown as any;
    const timeoutMs = 60000;
    const started = Date.now();
    // eslint-disable-next-line functional/no-loop-statements
    while (Date.now() - started < timeoutMs) {
      const meta = sessionManager.getRun(runId);
      if (meta !== undefined && meta.status !== "running") break;

      await sleep(500);
    }
    const meta = sessionManager.getRun(runId);
    const out = (() => {
      const direct = sessionManager.getOutput(runId);
      if (typeof direct === "string" && direct.length > 0) return direct;
      const result = sessionManager.getResult(runId);
      if (
        result !== undefined &&
        typeof result.finalReport?.content === "string" &&
        result.finalReport.content.length > 0
      ) {
        return result.finalReport.content;
      }
      if (
        meta !== undefined &&
        typeof meta.error === "string" &&
        meta.error.length > 0
      ) {
        return `❌ ${meta.error}`;
      }
      return "✅ Done";
    })();
    await client.chat.postMessage({
      channel,
      thread_ts: ts,
      text: out.substring(0, 3500),
    });
  }

  private async openDmChannel(userId: string): Promise<string | undefined> {
    try {
      const client = this.slackClient;
      if (client === undefined) return undefined;
      const dm = await (client as any).conversations.open({ users: userId });
      const channelId = dm?.channel?.id;
      return typeof channelId === "string" && channelId.length > 0
        ? channelId
        : undefined;
    } catch {
      return undefined;
    }
  }

  private async lookupUserLabel(userId: string): Promise<string> {
    const client = this.slackClient;
    if (client === undefined) return userId;
    const cached = this.userLabelCache.get(userId);
    if (cached) return cached;
    try {
      const info = await (client as any).users.info({ user: userId });
      const profile = info?.user?.profile;
      const display =
        typeof profile?.display_name === "string"
          ? profile.display_name.trim()
          : "";
      const real =
        typeof profile?.real_name === "string" ? profile.real_name.trim() : "";
      const email =
        typeof profile?.email === "string" ? profile.email.trim() : "";
      const primary =
        real.length > 0 ? real : display.length > 0 ? display : userId;
      const extras: string[] = [];
      if (display.length > 0 && display !== primary) extras.push(display);
      if (real.length > 0 && real !== primary && !extras.includes(real))
        extras.push(real);
      if (email.length > 0 && email !== primary && !extras.includes(email))
        extras.push(email);
      if (userId !== primary && !extras.includes(userId)) extras.push(userId);
      const label =
        extras.length > 0 ? `${primary} (${extras.join(", ")})` : primary;
      this.userLabelCache.set(userId, label);
      return label;
    } catch {
      return userId;
    }
  }

  private async readRawBody(req: http.IncomingMessage): Promise<string> {
    const chunks: Buffer[] = [];
    await new Promise<void>((resolve, reject) => {
      req.on("data", (chunk) => {
        chunks.push(Buffer.from(chunk));
      });
      req.on("end", () => resolve());
      req.on("error", (err) => reject(err));
    });
    return Buffer.concat(chunks).toString("utf8");
  }

  private verifySlackSignature(
    req: http.IncomingMessage,
    rawBody: string,
  ): boolean {
    if (this.slackSigningSecret === undefined) return false;
    const ts = req.headers["x-slack-request-timestamp"];
    const sig = req.headers["x-slack-signature"];
    if (typeof ts !== "string" || typeof sig !== "string") return false;
    const now = Math.floor(Date.now() / 1000);
    const age = Math.abs(now - Number.parseInt(ts, 10));
    if (!Number.isFinite(age) || age > 60 * 5) return false;
    const base = `v0:${ts}:${rawBody}`;
    const hmac = crypto
      .createHmac("sha256", this.slackSigningSecret)
      .update(base)
      .digest("hex");
    const expected = `v0=${hmac}`;
    return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(sig));
  }

  private initializeAgents(primaryAgentPath: string): {
    slackConfig: {
      botToken?: string;
      appToken?: string;
      signingSecret?: string;
      mentions?: boolean;
      dms?: boolean;
      updateIntervalMs?: number;
      historyLimit?: number;
      historyCharsCap?: number;
      openerTone?: "random" | "cheerful" | "formal" | "busy";
      routing?: unknown;
    };
    apiConfig: { port?: number };
    systemPrompt: string;
    defaultSessions: SessionManager;
    resolveRoute: (args: {
      kind: "mentions" | "channel-posts" | "dms";
      channelId: string;
      channelName?: string;
    }) => Promise<RoutingResolution | undefined>;
  } {
    const configPath = this.loadOptions.configPath;
    const layers = discoverLayers({ configPath, promptPath: primaryAgentPath });
    const mergeEnv = (
      a: Record<string, string> = {},
      b: Record<string, string> = {},
    ) => ({ ...b, ...a });
    const expandStrict = (
      obj: unknown,
      vars: Record<string, string>,
      section: string,
    ): unknown => {
      if (typeof obj === "string") {
        return obj.replace(/\$\{([^}]+)\}/g, (_m, n: string) => {
          const v = vars[n];
          if (v === undefined)
            throw new Error(
              `Unresolved variable \${${n}} in section '${section}'`,
            );
          return v;
        });
      }
      if (Array.isArray(obj))
        return obj.map((v) => expandStrict(v, vars, section));
      if (obj !== null && typeof obj === "object") {
        const out: Record<string, unknown> = {};
        Object.entries(obj as Record<string, unknown>).forEach(([k, v]) => {
          out[k] = expandStrict(v, vars, section);
        });
        return out;
      }
      return obj;
    };
    const resolveSection = (name: "slack" | "api"): Record<string, unknown> => {
      const found = layers.find(
        (ly) => ly.json && typeof ly.json[name] === "object",
      );
      if (!found) return {};
      const envVars = mergeEnv(
        found.env ?? {},
        process.env as Record<string, string>,
      );
      const rawVal = (found.json as Record<string, unknown>)[name];
      if (rawVal === undefined || rawVal === null) return {};
      return (
        (expandStrict(rawVal, envVars, name) as Record<string, unknown>) ?? {}
      );
    };
    const apiResolved = resolveSection("api") as { port?: number };
    const slackResolvedRaw = resolveSection("slack") as {
      botToken?: string;
      appToken?: string;
      signingSecret?: string;
      mentions?: boolean;
      dms?: boolean;
      updateIntervalMs?: number | string;
      historyLimit?: number;
      historyCharsCap?: number;
      openerTone?: "random" | "cheerful" | "formal" | "busy";
      routing?: unknown;
    };
    const parsedUpdateInterval = parseDurationMs(
      slackResolvedRaw.updateIntervalMs,
    );
    const slackResolved = {
      ...slackResolvedRaw,
      updateIntervalMs:
        parsedUpdateInterval ??
        (typeof slackResolvedRaw.updateIntervalMs === "number"
          ? slackResolvedRaw.updateIntervalMs
          : undefined),
    };

    const primaryLoaded = this.loadAgent(primaryAgentPath);
    const defaultSessions = primaryLoaded.sessions;

    const compiled = this.compileRouting(
      slackResolved.routing,
      primaryAgentPath,
    );

    const resolveRoute = (args: {
      kind: "mentions" | "channel-posts" | "dms";
      channelId: string;
      channelName?: string;
    }): Promise<RoutingResolution | undefined> => {
      // deny first
      const channelNameValue =
        typeof args.channelName === "string" && args.channelName.length > 0
          ? args.channelName
          : undefined;
      // eslint-disable-next-line functional/no-loop-statements
      for (const deny of compiled.denies) {
        if (deny.engage.size > 0 && !deny.engage.has(args.kind)) continue;
        if (deny.channelIdPatterns.some((re) => re.test(args.channelId)))
          return Promise.resolve(undefined);
        if (
          channelNameValue !== undefined &&
          deny.channelNamePatterns.some((re) => re.test(channelNameValue))
        )
          return Promise.resolve(undefined);
      }
      // eslint-disable-next-line functional/no-loop-statements
      for (const rule of compiled.rules) {
        if (rule.engage.size > 0 && !rule.engage.has(args.kind)) continue;
        const idMatch = rule.channelIdPatterns.some((re) =>
          re.test(args.channelId),
        );
        const nameMatch =
          channelNameValue !== undefined
            ? rule.channelNamePatterns.some((re) => re.test(channelNameValue))
            : false;
        if (idMatch || nameMatch) {
          const rec = this.agentCache.get(rule.agentPath);
          if (!rec) continue;
          return Promise.resolve({
            sessions: rec.sessions,
            systemPrompt: rec.loaded.systemTemplate,
            promptTemplates: rule.promptTemplates,
            contextPolicy: rule.contextPolicy,
          });
        }
      }
      const def = compiled.default;
      if (!def) return Promise.resolve(undefined);
      const engageSet = def.engage;
      const engageConfigured =
        engageSet.size > 0
          ? engageSet.has(args.kind)
          : args.kind === "mentions"
            ? slackResolved.mentions !== false
            : args.kind === "dms"
              ? slackResolved.dms !== false
              : false;
      if (!engageConfigured) return Promise.resolve(undefined);
      const rec = this.agentCache.get(def.agentPath);
      if (!rec) return Promise.resolve(undefined);
      return Promise.resolve({
        sessions: rec.sessions,
        systemPrompt: rec.loaded.systemTemplate,
        promptTemplates: def.promptTemplates,
        contextPolicy: def.contextPolicy,
      });
    };

    return {
      slackConfig: slackResolved,
      apiConfig: apiResolved,
      systemPrompt: primaryLoaded.loaded.systemTemplate,
      defaultSessions,
      resolveRoute,
    };
  }

  private compileRouting(
    rawRouting: unknown,
    agentPath: string,
  ): CompiledRouting {
    const routing = (rawRouting ?? {}) as Record<string, unknown>;
    const compiled: CompiledRouting = {
      default: undefined,
      denies: [],
      rules: [],
    };

    const escapeRegex = (s: string): string =>
      s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const globToRegex = (pat: string): RegExp => {
      const esc = escapeRegex(pat).replace(/\\\*/g, ".*").replace(/\\\?/g, ".");
      return new RegExp(`^${esc}$`, "i");
    };
    const parseChannels = (
      arr: unknown,
    ): { namePats: RegExp[]; idPats: RegExp[] } => {
      const a = Array.isArray(arr) ? arr : [];
      const namePats: RegExp[] = [];
      const idPats: RegExp[] = [];
      a.forEach((raw) => {
        const s = typeof raw === "string" ? raw.trim() : "";
        if (s.length === 0) return;
        if (s.startsWith("C") || s.startsWith("G")) {
          idPats.push(globToRegex(s));
          return;
        }
        const n = s.startsWith("#") ? s.slice(1) : s;
        namePats.push(globToRegex(n));
      });
      return { namePats, idPats };
    };
    const toEngage = (
      arr: unknown,
    ): Set<"mentions" | "channel-posts" | "dms"> =>
      new Set(
        (Array.isArray(arr) ? arr : []).filter(
          (x): x is "mentions" | "channel-posts" | "dms" =>
            x === "mentions" || x === "channel-posts" || x === "dms",
        ),
      );

    const resolveAgentPath = (p: string): string =>
      path.isAbsolute(p) ? p : path.resolve(path.dirname(agentPath), p);

    const preloadAgent = (p: string): void => {
      const abs = resolveAgentPath(p);
      if (this.agentCache.has(abs)) return;
      if (!fs.existsSync(abs)) {
        throw new Error(`Slack routing references missing agent file: ${abs}`);
      }
      const loaded = this.loadAgent(abs);
      this.agentCache.set(abs, loaded);
    };

    const defaultRouting = routing.default;
    if (defaultRouting && typeof defaultRouting === "object") {
      const def = defaultRouting as Record<string, unknown>;
      const engage = toEngage(def.engage);
      const agent =
        typeof def.agent === "string" && def.agent.length > 0
          ? def.agent
          : agentPath;
      preloadAgent(agent);
      compiled.default = {
        agentPath: resolveAgentPath(agent),
        engage,
        promptTemplates: def.promptTemplates as
          | { mention?: string; dm?: string; channelPost?: string }
          | undefined,
        contextPolicy: def.contextPolicy as
          | { channelPost?: "selfOnly" | "previousOnly" | "selfAndPrevious" }
          | undefined,
      };
    } else {
      preloadAgent(agentPath);
    }

    const denyList = Array.isArray(routing.deny)
      ? (routing.deny as Record<string, unknown>[])
      : [];
    denyList.forEach((deny) => {
      const { namePats, idPats } = parseChannels(deny.channels);
      compiled.denies.push({
        channelNamePatterns: namePats,
        channelIdPatterns: idPats,
        engage: toEngage(deny.engage),
      });
    });

    const rules = Array.isArray(routing.rules)
      ? (routing.rules as Record<string, unknown>[])
      : [];
    rules.forEach((rule) => {
      if (rule.enabled === false) return;
      const agent =
        typeof rule.agent === "string" && rule.agent.length > 0
          ? rule.agent
          : agentPath;
      preloadAgent(agent);
      const { namePats, idPats } = parseChannels(rule.channels);
      compiled.rules.push({
        channelNamePatterns: namePats,
        channelIdPatterns: idPats,
        agentPath: resolveAgentPath(agent),
        engage: toEngage(rule.engage),
        promptTemplates: rule.promptTemplates as
          | { mention?: string; dm?: string; channelPost?: string }
          | undefined,
        contextPolicy: rule.contextPolicy as
          | { channelPost?: "selfOnly" | "previousOnly" | "selfAndPrevious" }
          | undefined,
      });
    });

    return compiled;
  }

  private loadAgent(agentPath: string): LoadedAgentRecord {
    const abs = path.resolve(agentPath);
    const cached = this.agentCache.get(abs);
    if (cached) return cached;
    const loaded = loadAgent(abs, this.loaderCache, {
      ...this.loadOptions,
      verbose: this.verbose,
      traceLLM: this.traceLLM,
      traceMCP: this.traceMCP,
      traceSdk: this.traceSdk,
    });
    const sessions = new SessionManager(
      (systemPrompt, userPrompt, opts) => {
        const expectedJson = loaded.expectedOutput?.format === "json";
        const baseOpts = (opts ?? {}) as Record<string, unknown>;
        const renderTarget =
          typeof baseOpts.renderTarget === "string"
            ? baseOpts.renderTarget
            : undefined;
        const outputFormat = expectedJson
          ? "json"
          : renderTarget === "slack"
            ? "slack-block-kit"
            : "markdown";
        return loaded.run(systemPrompt, userPrompt, {
          ...baseOpts,
          outputFormat,
          headendId: this.id,
          telemetryLabels: this.telemetryLabels,
          wantsProgressUpdates: true,
        } as any);
      },
      {
        onEvent: (runId, event) => {
          void runId;
          if (event.type !== 'log') return;
          const entry = event.entry;
          entry.headendId = this.id;
          this.context?.log(entry);
        },
      },
      { headendId: this.id, telemetryLabels: this.telemetryLabels },
    );
    const record: LoadedAgentRecord = { loaded, sessions };
    this.agentCache.set(abs, record);
    return record;
  }

  private registerRunRelease(
    session: SessionManager,
    runId: string,
    release: () => void,
  ): void {
    if (!this.registeredSessions.has(session)) {
      session.onTreeUpdate((id) => {
        const maybeRelease = this.runReleases.get(id);
        if (maybeRelease === undefined) return;
        const meta = session.getRun(id);
        if (
          meta === undefined ||
          (meta.status !== "running" && meta.status !== "stopping")
        ) {
          this.runReleases.delete(id);
          maybeRelease();
        }
      });
      this.registeredSessions.add(session);
    }
    this.runReleases.set(runId, release);
  }

  private handleShutdownSignal(): void {
    if (this.globalStopRef !== undefined) {
      this.globalStopRef.stopping = true;
    }
    this.stopAllSessions("shutdown");
  }

  private stopAllSessions(reason: string): void {
    const seen = new Set<SessionManager>();
    const stopSessions = (sessions: SessionManager | undefined) => {
      if (sessions === undefined || seen.has(sessions)) return;
      seen.add(sessions);
      sessions.listActiveRuns().forEach((run) => {
        sessions.stopRun(run.runId, reason);
      });
    };
    stopSessions(this.defaultSession);
    this.agentCache.forEach(({ sessions }) => {
      stopSessions(sessions);
    });
  }

  private createDeferred<T>(): {
    promise: Promise<T>;
    resolve: (value: T) => void;
    reject: (reason?: unknown) => void;
  } {
    let resolve!: (value: T) => void;
    let reject!: (reason?: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    return { promise, resolve, reject };
  }

  private normalizePath(pathname: string): string {
    const cleaned = pathname.replace(/\\+/g, "/");
    if (cleaned === "" || cleaned === "/") return "/";
    return cleaned.endsWith("/") ? cleaned.slice(0, -1) : cleaned;
  }

  private attachSocketModeEventHandlers(): void {
    const receiver = (
      this.slackApp as unknown as {
        receiver?: {
          client?: {
            on: (event: string, handler: (...args: unknown[]) => void) => void;
          };
        };
      }
    )?.receiver;
    const client = receiver?.client;
    if (client === undefined) {
      this.log("Socket mode receiver not available for event handlers", "WRN");
      return;
    }

    client.on("connecting", () => {
      this.log("Slack connecting", "VRB");
    });

    client.on("authenticated", (_resp: unknown) => {
      this.log("Slack authenticated", "VRB");
    });

    client.on("connected", () => {
      this.log("Slack connected", "VRB");
    });

    client.on("reconnecting", () => {
      this.log("Slack reconnecting...", "VRB");
    });

    client.on("disconnecting", () => {
      this.log("Slack disconnecting", "VRB");
    });

    client.on("disconnected", (err: unknown) => {
      if (err instanceof Error) {
        this.log(`Slack disconnected: ${err.message}`, "WRN");
      } else {
        this.log("Slack disconnected", "VRB");
      }
    });

    client.on("error", (err: unknown) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`Slack socket error: ${message}`, "ERR");
    });
  }

  private log(
    message: string,
    severity: Parameters<HeadendContext["log"]>[0]["severity"] = "VRB",
  ): void {
    const entry: Parameters<HeadendContext["log"]>[0] = {
      timestamp: Date.now(),
      severity,
      turn: 0,
      subturn: 0,
      direction: "response",
      type: "tool",
      remoteIdentifier: "headend:slack",
      fatal: severity === "ERR",
      message: `${this.label}: ${message}`,
      headendId: this.id,
    };
    this.context?.log(entry);
  }
}
