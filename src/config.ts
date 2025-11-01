import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { z } from 'zod';

import type { Configuration } from './types.js';

const ProviderModelOverridesSchema = z.object({
  temperature: z.number().min(0).max(2).nullable().optional(),
  topP: z.number().min(0).max(1).nullable().optional(),
  top_p: z.number().min(0).max(1).nullable().optional(),
});

const ReasoningLevelSchema = z.enum(['minimal','low','medium','high']);

const ProviderReasoningEntrySchema = z.union([z.string(), z.number()]);

const ProviderReasoningSchema = z
  .union([
    z.null(),
    ProviderReasoningEntrySchema,
    z.tuple([
      ProviderReasoningEntrySchema,
      ProviderReasoningEntrySchema,
      ProviderReasoningEntrySchema,
      ProviderReasoningEntrySchema,
    ]),
  ])
  .optional();

const ProviderModelConfigSchema = z.object({
  overrides: ProviderModelOverridesSchema.optional(),
  reasoning: ProviderReasoningSchema,
  contextWindow: z.number().int().positive().optional(),
  tokenizer: z.string().optional(),
  contextWindowBufferTokens: z.number().int().positive().optional(),
});

const ProviderConfigSchema = z.object({
  apiKey: z.string().optional(),
  baseUrl: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
  custom: z.record(z.string(), z.unknown()).optional(),
  mergeStrategy: z.enum(['overlay','override','deep']).optional(),
  type: z.enum(['openai','anthropic','google','openrouter','ollama','test-llm']),
  openaiMode: z.enum(['responses','chat']).optional(),
  models: z.record(z.string(), ProviderModelConfigSchema).optional(),
  toolsAllowed: z.array(z.string()).optional(),
  toolsDenied: z.array(z.string()).optional(),
  stringSchemaFormatsAllowed: z.array(z.string()).optional(),
  stringSchemaFormatsDenied: z.array(z.string()).optional(),
  reasoning: ProviderReasoningSchema,
  reasoningAutoStreamLevel: ReasoningLevelSchema.optional(),
  contextWindow: z.number().int().positive().optional(),
  tokenizer: z.string().optional(),
  contextWindowBufferTokens: z.number().int().positive().optional(),
});

const MCPServerConfigSchema = z.object({
  type: z.enum(['stdio', 'websocket', 'http', 'sse']).optional(),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  url: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
  env: z.record(z.string(), z.string()).optional(),
  enabled: z.boolean().optional(),
  toolSchemas: z.record(z.string(), z.unknown()).optional(),
  toolsAllowed: z.array(z.string()).optional(),
  toolsDenied: z.array(z.string()).optional(),
});

const EngageEnum = z.enum(['mentions','channel-posts','dms']);

const SlackRoutingDefaultSchema = z.object({
  agent: z.string(),
  engage: z.array(EngageEnum).optional(),
  promptTemplates: z.object({
    mention: z.string().optional(),
    dm: z.string().optional(),
    channelPost: z.string().optional(),
  }).partial().optional(),
  contextPolicy: z.object({
    channelPost: z.enum(['selfOnly','previousOnly','selfAndPrevious']).optional(),
  }).partial().optional(),
}).partial();

const SlackRoutingRuleSchema = z.object({
  channels: z.array(z.string()),
  agent: z.string(),
  engage: z.array(EngageEnum).optional(),
  promptTemplates: z.object({
    mention: z.string().optional(),
    dm: z.string().optional(),
    channelPost: z.string().optional(),
  }).partial().optional(),
  contextPolicy: z.object({
    channelPost: z.enum(['selfOnly','previousOnly','selfAndPrevious']).optional(),
  }).partial().optional(),
});

const SlackRoutingDenySchema = z.object({
  channels: z.array(z.string()),
  engage: z.array(EngageEnum).optional(),
});

const SlackRoutingSchema = z.object({
  default: SlackRoutingDefaultSchema.optional(),
  rules: z.array(SlackRoutingRuleSchema).optional(),
  deny: z.array(SlackRoutingDenySchema).optional(),
}).partial();

const SlackConfigSchema = z.object({
  enabled: z.boolean().optional(),
  mentions: z.boolean().optional(),
  dms: z.boolean().optional(),
  updateIntervalMs: z.number().int().positive().optional(),
  historyLimit: z.number().int().positive().optional(),
  historyCharsCap: z.number().int().positive().optional(),
  botToken: z.string().optional(),
  appToken: z.string().optional(),
  signingSecret: z.string().optional(),
  routing: SlackRoutingSchema.optional(),
});

const ApiConfigSchema = z.object({
  enabled: z.boolean().optional(),
  port: z.number().int().positive().optional(),
  bearerKeys: z.array(z.string()).optional(),
});

const OutputFormatEnum = z.enum(['markdown','markdown+mermaid','slack-block-kit','tty','pipe','json','sub-agent']);

const FormatsConfigSchema = z.object({
  cli: OutputFormatEnum.optional(),
  slack: OutputFormatEnum.optional(),
  api: OutputFormatEnum.optional(),
  web: OutputFormatEnum.optional(),
  subAgent: OutputFormatEnum.optional(),
}).partial();

const TelemetryOtlpSchema = z.object({
  endpoint: z.string().optional(),
  timeoutMs: z.number().int().positive().optional(),
}).optional();

const TelemetryPrometheusSchema = z.object({
  enabled: z.boolean().optional(),
  host: z.string().optional(),
  port: z.number().int().positive().optional(),
}).optional();

const TelemetryTraceSamplerSchema = z.enum(['always_on', 'always_off', 'parent', 'ratio']);

const TelemetryTraceSchema = z.object({
  enabled: z.boolean().optional(),
  sampler: TelemetryTraceSamplerSchema.optional(),
  ratio: z.number().min(0).max(1).optional(),
}).optional();

const TelemetryLogFormatSchema = z.enum(['journald', 'logfmt', 'json', 'none']);

const TelemetryLogExtraSchema = z.enum(['otlp']);

const TelemetryLoggingOtlpSchema = z.object({
  endpoint: z.string().optional(),
  timeoutMs: z.number().int().positive().optional(),
}).optional();

const TelemetryLoggingSchema = z.object({
  formats: z.array(TelemetryLogFormatSchema).nonempty().optional(),
  extra: z.array(TelemetryLogExtraSchema).optional(),
  otlp: TelemetryLoggingOtlpSchema,
}).optional();

const TelemetrySchema = z.object({
  enabled: z.boolean().optional(),
  otlp: TelemetryOtlpSchema,
  prometheus: TelemetryPrometheusSchema,
  labels: z.record(z.string(), z.string()).optional(),
  traces: TelemetryTraceSchema,
  logging: TelemetryLoggingSchema,
}).optional();

const ConfigurationSchema = z.object({
  providers: z.record(z.string(), ProviderConfigSchema),
  mcpServers: z.record(z.string(), MCPServerConfigSchema),
  accounting: z.object({ file: z.string() }).optional(),
  pricing: z
    .record(
      z.string(), // provider
      z.record(
        z.string(), // model
        z.object({
          unit: z.enum(['per_1k','per_1m']).optional(),
          currency: z.literal('USD').optional(),
          prompt: z.number().nonnegative().optional(),
          completion: z.number().nonnegative().optional(),
          cacheRead: z.number().nonnegative().optional(),
          cacheWrite: z.number().nonnegative().optional(),
        })
      )
    )
    .optional(),
  defaults: z
    .object({
      llmTimeout: z.number().positive().optional(),
      toolTimeout: z.number().positive().optional(),
      temperature: z.number().min(0).max(2).optional(),
      topP: z.number().min(0).max(1).optional(),
      stream: z.boolean().optional(),
      parallelToolCalls: z.boolean().optional(),
      maxToolTurns: z.number().int().positive().optional(),
      maxToolCallsPerTurn: z.number().int().positive().optional(),
      maxRetries: z.number().int().positive().optional(),
      toolResponseMaxBytes: z.number().int().positive().optional(),
      mcpInitConcurrency: z.number().int().positive().optional(),
      contextWindowBufferTokens: z.number().int().positive().optional(),
      outputFormat: OutputFormatEnum.optional(),
      formats: FormatsConfigSchema.optional(),
    })
    .optional(),
  telemetry: TelemetrySchema,
  slack: SlackConfigSchema.optional(),
  api: ApiConfigSchema.optional(),
});

function hasMcpServers(x: unknown): x is { mcpServers: Record<string, unknown> } {
  return x !== null && typeof x === 'object' && 'mcpServers' in (x as Record<string, unknown>) && typeof (x as Record<string, unknown>).mcpServers === 'object' && (x as Record<string, unknown>).mcpServers !== null;
}

function expandEnv(str: string): string {
  return str.replace(/\$\{([^}]+)\}/g, (_m: string, name: string) => (process.env[name] ?? ''));
}

function expandDeep(obj: unknown, chain: string[] = []): unknown {
  if (typeof obj === 'string') {
    if (chain.includes('mcpServers') && (chain.includes('env') || chain.includes('headers'))) return obj;
    return expandEnv(obj);
  }
  if (Array.isArray(obj)) return obj.map((v) => expandDeep(v, chain));
  if (obj !== null && typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>);
    return entries.reduce<Record<string, unknown>>((acc, [k, v]) => {
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
  let raw: string;
  try {
    raw = fs.readFileSync(resolved, 'utf-8');
  } catch (e) {
    throw new Error(`Failed to read configuration file ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
  }
  let json: unknown;
  try {
    json = JSON.parse(raw);
  } catch (e) {
    throw new Error(`Invalid JSON in configuration file ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
  }
  let expanded: unknown;
  try {
    expanded = expandDeep(json);
  } catch (e) {
    throw new Error(`Environment variable expansion failed in ${resolved}: ${e instanceof Error ? e.message : String(e)}`);
  }
  // Normalize MCP server configs to internal schema (without using any)
  let expandedNormalized: unknown = expanded;
  if (hasMcpServers(expanded)) {
    const srcServers = expanded.mcpServers as Record<string, unknown> | undefined;
    const serversObj = srcServers ?? {};
    const normalizedServers = Object.fromEntries(
      Object.entries(serversObj).map(([name, srv]) => {
        if (srv === null || typeof srv !== 'object') return [name, srv];
        const s = srv as Record<string, unknown>;
        const out: Record<string, unknown> = { ...s };
        // type normalization
        const typeVal = s.type ;
        if (typeVal === 'local') out.type = 'stdio';
        else if (typeVal === 'remote') {
          const url = typeof s.url === 'string' ? s.url : '';
          out.type = url.includes('/sse') ? 'sse' : 'http';
        }
        // Infer type if not specified
        if (out.type === undefined) {
          const url = s.url;
          if (typeof url === 'string' && url.length > 0) {
            // If URL is provided, infer HTTP or SSE transport
            out.type = url.includes('/sse') ? 'sse' : 'http';
          } else {
            // Default to stdio for local servers without URL
            out.type = 'stdio';
          }
        }
        // command array normalization
        const cmd = s.command ;
        if (Array.isArray(cmd) && cmd.length > 0) {
          const existingArgs = Array.isArray(s.args) ? (s.args as unknown[]).map((a) => String(a)) : [];
          out.command = String(cmd[0]);
          out.args = [...cmd.slice(1).map((a) => String(a)), ...existingArgs];
        }
        // environment -> env
        if (s.environment !== undefined && s.env === undefined) out.env = s.environment;
        // enabled default
        if (typeof s.enabled !== 'boolean') out.enabled = true;
        return [name, out];
      })
    );
    expandedNormalized = { ...(expanded as Record<string, unknown>), mcpServers: normalizedServers };
  }
  const parsed = ConfigurationSchema.safeParse(expandedNormalized);
  if (!parsed.success) {
    const msgs = parsed.error.issues
      .map((issue) => `  ${issue.path.map((p) => String(p)).join('.')}: ${issue.message}`)
      .join('\n');
    throw new Error(`Configuration validation failed in ${resolved}:\n${msgs}`);
  }
  return parsed.data as Configuration;
}

export function validateProviders(config: Configuration, providers: string[]): void {
  const missing = providers.filter((p) => !(p in config.providers));
  if (missing.length > 0) throw new Error(`Unknown providers: ${missing.join(', ')}`);
}

export function validateMCPServers(config: Configuration, mcpServers: string[]): void {
  // Allow special virtual tool selectors that aren't MCP servers
  const virtuals = new Set<string>(['batch']);
  const rest = new Set<string>(Object.keys(config.restTools ?? {}));
  const missing = mcpServers
    .filter((s) => !virtuals.has(s))
    .filter((s) => !rest.has(s))
    .filter((s) => !(s in config.mcpServers));
  if (missing.length > 0) throw new Error(`Unknown MCP servers: ${missing.join(', ')}`);
}

export function validatePrompts(systemPrompt: string, userPrompt: string): void {
  if (systemPrompt === '-' && userPrompt === '-') throw new Error('Cannot use stdin (-) for both system and user prompts');
}
