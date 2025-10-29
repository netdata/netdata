import fs from 'node:fs';
import path from 'node:path';

import type { AIAgentSession } from './ai-agent.js';
// keep type imports grouped at top
import type { OutputFormatId } from './formats.js';
import type { PreloadedSubAgent } from './subagent-registry.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage, ProviderConfig, ProviderReasoningValue, RestToolConfig } from './types.js';
// no runtime format validation here; caller must pass a valid OutputFormatId

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults, type ResolvedConfigLayer } from './config-resolver.js';
import { parseFrontmatter, parseList, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { resolveIncludes } from './include-resolver.js';
import { DEFAULT_TOOL_INPUT_SCHEMA, DEFAULT_TOOL_INPUT_SCHEMA_JSON, cloneJsonSchema, cloneOptionalJsonSchema } from './input-contract.js';
import { isReservedAgentName } from './internal-tools.js';
import { resolveEffectiveOptions } from './options-resolver.js';
import { buildEffectiveOptionsSchema } from './options-schema.js';
import { openApiToRestTools, parseOpenAPISpec } from './tools/openapi-importer.js';
import { clampToolName, sanitizeToolName } from './utils.js';
import { mergeCallbacksWithPersistence } from './persistence.js';


export interface LoadedAgentSessionOptions {
  history?: ConversationMessage[];
  callbacks?: AIAgentCallbacks;
  trace?: AIAgentSessionConfig['trace'];
  renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent';
  outputFormat: OutputFormatId;
  abortSignal?: AbortSignal;
  stopRef?: { stopping: boolean };
  initialTitle?: string;
  ancestors?: string[];
  headendId?: string;
  telemetryLabels?: Record<string, string>;
  wantsProgressUpdates?: boolean;
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSdk?: boolean;
  verbose?: boolean;
  agentPath?: string;
  turnPathPrefix?: string;
}

export interface LoadedAgent {
  id: string; // canonical path (or synthetic when no file)
  promptPath: string;
  systemTemplate: string;
  description?: string;
  usage?: string;
  toolName?: string;
  expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> };
  input: { format: 'json' | 'text'; schema: Record<string, unknown> };
  outputSchema?: Record<string, unknown>;
  config: Configuration;
  targets: { provider: string; model: string }[];
  tools: string[];
  accountingFile?: string;
  subAgents: PreloadedSubAgent[];
  effective: {
    temperature: number;
    topP: number;
    maxOutputTokens?: number;
    repeatPenalty?: number;
    llmTimeout: number;
    toolTimeout: number;
    maxRetries: number;
    maxToolTurns: number;
    maxToolCallsPerTurn: number;
    toolResponseMaxBytes: number;
    stream: boolean;
    parallelToolCalls: boolean;
    maxConcurrentTools: number;
    traceLLM: boolean;
    traceMCP: boolean;
    traceSdk: boolean;
    verbose: boolean;
    mcpInitConcurrency?: number;
  };
  subTools: { name: string; description?: string }[]; // placeholder for future
  createSession: (
    systemPrompt: string,
    userPrompt: string,
    opts?: LoadedAgentSessionOptions
  ) => Promise<AIAgentSession>;
  run: (
    systemPrompt: string,
    userPrompt: string,
    opts?: LoadedAgentSessionOptions
  ) => Promise<AIAgentResult>;
}

export class LoadedAgentCache {
  private readonly cache = new Map<string, LoadedAgent>();

  get(key: string): LoadedAgent | undefined { return this.cache.get(key); }
  set(key: string, val: LoadedAgent): void { this.cache.set(key, val); }
}

function canonical(p: string): string {
  try { return fs.realpathSync(p); } catch { return p; }
}

const INCLUDE_DIRECTIVE_PATTERN = /\$\{include:[^}]+\}|\{\{include:[^}]+\}\}/;

function readFileText(p: string): string { return fs.readFileSync(p, 'utf-8'); }

function deriveToolNameFromPath(p: string): string {
  const base = path.basename(p).replace(/\.[^.]+$/, '');
  const sanitized = sanitizeToolName(base);
  const normalized = sanitized.length > 0 ? sanitized.toLowerCase() : sanitized;
  return clampToolName(normalized).name;
}

function loadFlattenedPrompt(promptPath: string): { content: string; systemTemplate: string } {
  const raw = readFileText(promptPath);
  const baseDir = path.dirname(promptPath);
  const content = resolveIncludes(raw, baseDir);
  if (INCLUDE_DIRECTIVE_PATTERN.test(content)) {
    throw new Error(`Prompt '${promptPath}' still contains include directives after expansion. Check for circular includes or malformed syntax.`);
  }
  return { content, systemTemplate: extractBodyWithoutFrontmatter(content) };
}

function flattenPromptContent(content: string, baseDir?: string): { content: string; systemTemplate: string } {
  const resolved = resolveIncludes(content, baseDir);
  if (INCLUDE_DIRECTIVE_PATTERN.test(resolved)) {
    throw new Error('Provided prompt content still contains include directives after expansion.');
  }
  return { content: resolved, systemTemplate: extractBodyWithoutFrontmatter(resolved) };
}

export interface GlobalOverrides {
  // Arrays provided here are treated as immutable snapshots; do not mutate them after invoking load.
  models?: { provider: string; model: string }[];
  tools?: string[];
  agents?: string[];
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  maxRetries?: number;
  maxToolTurns?: number;
  maxToolCallsPerTurn?: number;
  maxConcurrentTools?: number;
  toolResponseMaxBytes?: number;
  stream?: boolean;
  parallelToolCalls?: boolean;
  mcpInitConcurrency?: number;
  reasoning?: 'minimal' | 'low' | 'medium' | 'high';
  reasoningValue?: ProviderReasoningValue | null;
  caching?: 'none' | 'full';
}

export interface LoadAgentOptions {
  configPath?: string;
  configLayers?: ResolvedConfigLayer[];
  verbose?: boolean;
  targets?: { provider: string; model: string }[];
  tools?: string[];
  agents?: string[];
  // Overrides applied to every agent/sub-agent. Override values take precedence over CLI/frontmatter/defaults.
  globalOverrides?: GlobalOverrides;
  // Optional overrides (CLI precedence) for runtime knobs
  stream?: boolean;
  parallelToolCalls?: boolean;
  maxRetries?: number;
  maxToolTurns?: number;
  maxToolCallsPerTurn?: number;
  maxConcurrentTools?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  toolResponseMaxBytes?: number;
  mcpInitConcurrency?: number;
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSdk?: boolean;
  reasoningValue?: ProviderReasoningValue | null;
  // Persistence overrides (CLI precedence)
  sessionsDir?: string;
  billingFile?: string;
  // Defaults applied only when frontmatter/config do not specify (for sub-agents)
  defaultsForUndefined?: {
    temperature?: number;
    topP?: number;
    maxOutputTokens?: number;
    repeatPenalty?: number;
    llmTimeout?: number;
    toolTimeout?: number;
    maxRetries?: number;
    maxToolTurns?: number;
    maxConcurrentTools?: number;
    toolResponseMaxBytes?: number;
    parallelToolCalls?: boolean;
    maxToolCallsPerTurn?: number;
    reasoning?: 'minimal' | 'low' | 'medium' | 'high';
    reasoningValue?: ProviderReasoningValue | null;
    caching?: 'none' | 'full';
  };
  providerOverrides?: Record<string, Partial<ProviderConfig>>;
  ancestors?: string[];
  reasoning?: 'minimal' | 'low' | 'medium' | 'high';
  caching?: 'none' | 'full';
}

const resolveInputContract = (
  spec?: { format: 'text' | 'json'; schema?: Record<string, unknown> }
): { format: 'json' | 'text'; schema: Record<string, unknown> } => {
  if (spec?.format === 'json') {
    const schema = spec.schema !== undefined ? cloneJsonSchema(spec.schema) : cloneJsonSchema(DEFAULT_TOOL_INPUT_SCHEMA);
    return { format: 'json', schema };
  }
  if (spec?.format === 'text') {
    return { format: 'text', schema: cloneJsonSchema(DEFAULT_TOOL_INPUT_SCHEMA) };
  }
  return { format: 'json', schema: cloneJsonSchema(DEFAULT_TOOL_INPUT_SCHEMA) };
};

interface LoadAgentCoreArgs {
  cache: LoadedAgentCache;
  cacheKey: string;
  id: string;
  promptPath: string;
  promptContent: string;
  systemTemplate: string;
  baseDir: string;
  frontmatterBaseDir?: string;
  options?: LoadAgentOptions;
  layers: ResolvedConfigLayer[];
  ancestorChain: string[];
}

function loadAgentCore(args: LoadAgentCoreArgs): LoadedAgent {
  const {
    cache,
    cacheKey,
    id,
    promptPath,
    promptContent,
    systemTemplate,
    baseDir,
    frontmatterBaseDir,
    options,
    layers,
    ancestorChain,
  } = args;

  const loaded = constructLoadedAgent({
    id,
    promptPath,
    promptContent,
    systemTemplate,
    baseDir,
    registry: cache,
    options,
    layers,
    ancestorChain,
    frontmatterBaseDir,
  });
  cache.set(cacheKey, loaded);
  return loaded;
}


function containsPlaceholder(val: unknown): boolean {
  if (typeof val === 'string') return /\$\{[^}]+\}/.test(val);
  if (Array.isArray(val)) return val.some((v) => containsPlaceholder(v));
  if (val !== null && typeof val === 'object') return Object.values(val as Record<string, unknown>).some((v) => containsPlaceholder(v));
  return false;
}

function validateNoPlaceholders(config: Configuration): void {
  const provs = Object.entries(config.providers);
  provs.forEach(([name, cfg]) => {
    if (containsPlaceholder(cfg)) throw new Error(`Unresolved placeholders remain in provider '${name}'`);
  });
  const servers = Object.entries(config.mcpServers);
  servers.forEach(([name, cfg]) => {
    if (containsPlaceholder(cfg)) throw new Error(`Unresolved placeholders remain in MCP server '${name}'`);
  });
  if (config.accounting?.file !== undefined && containsPlaceholder(config.accounting.file)) {
    throw new Error('Unresolved placeholders remain in accounting.file');
  }
}

interface ConstructAgentArgs {
  id: string;
  promptPath: string;
  promptContent: string;
  systemTemplate: string;
  baseDir: string;
  registry: LoadedAgentCache;
  options?: LoadAgentOptions;
  layers: ResolvedConfigLayer[];
  ancestorChain: string[];
  frontmatterBaseDir?: string;
}

function constructLoadedAgent(args: ConstructAgentArgs): LoadedAgent {
  const {
    id,
    promptPath,
    promptContent,
    systemTemplate,
    baseDir,
    registry,
    options,
    layers,
    ancestorChain,
    frontmatterBaseDir,
  } = args;

  const fmBaseDir = frontmatterBaseDir ?? baseDir;
  const fm = parseFrontmatter(promptContent, { baseDir: fmBaseDir });

  const fmModels = parsePairs(fm?.options?.models);
  const fmTools = parseList(fm?.options?.tools);
  const fmAgents = parseList(fm?.options?.agents);
  // Only models overrides are active today; this structure accommodates future keys.
  const overrideTargets = Array.isArray(options?.globalOverrides?.models) && options.globalOverrides.models.length > 0
    ? options.globalOverrides.models
    : undefined;
  const selectedTargets = overrideTargets
    ?? (Array.isArray(options?.targets) && options.targets.length > 0 ? options.targets : fmModels);
  let selectedTools = Array.isArray(options?.tools) && options.tools.length > 0 ? [...options.tools] : [...fmTools];
  const effAgents = Array.isArray(options?.agents) && options.agents.length > 0 ? options.agents : fmAgents;
  const selectedAgents = effAgents.map((rel) => path.resolve(baseDir, rel));
  const subAgentInfos: PreloadedSubAgent[] = [];
  const seenToolNames = new Set<string>();
  let mergedDefaultsForChildren: NonNullable<LoadAgentOptions['defaultsForUndefined']> | undefined;

  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));
  const externalToolNames = Array.from(new Set(
    selectedTools.filter((tool) => !tool.includes(':') && !isReservedAgentName(tool))
  ));

  let config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: externalToolNames, restTools: externalToolNames }, layers, { verbose: options?.verbose });
  if (options?.providerOverrides !== undefined) {
    Object.entries(options.providerOverrides).forEach(([name, override]) => {
      const existing = config.providers[name];
      if (existing === undefined) {
        return;
      }
      const mergedCustom = {
        ...(existing.custom ?? {}),
        ...(override.custom ?? {}),
      } as Record<string, unknown>;
      const next: ProviderConfig = {
        ...existing,
        ...override,
        custom: mergedCustom,
      };
      config.providers[name] = next;
    });
  }
  const defaults = resolveDefaults(layers);
  validateNoPlaceholders(config);

  const sessionsDirOverride = options?.sessionsDir;
  const billingFileOverride = options?.billingFile;
  if ((typeof sessionsDirOverride === 'string' && sessionsDirOverride.length > 0) || (typeof billingFileOverride === 'string' && billingFileOverride.length > 0)) {
    const persistence = { ...(config.persistence ?? {}) } as { sessionsDir?: string; billingFile?: string };
    if (typeof sessionsDirOverride === 'string' && sessionsDirOverride.length > 0) persistence.sessionsDir = sessionsDirOverride;
    if (typeof billingFileOverride === 'string' && billingFileOverride.length > 0) persistence.billingFile = billingFileOverride;
    (config as unknown as { persistence?: { sessionsDir?: string; billingFile?: string } }).persistence = persistence;
  }

  const missingTools: string[] = [];
  selectedTools.forEach((toolName) => {
    if (toolName.includes(':')) return;
    if (isReservedAgentName(toolName)) return;
    const inMcpServers = Object.prototype.hasOwnProperty.call(config.mcpServers, toolName);
    const inRestTools = config.restTools !== undefined && Object.prototype.hasOwnProperty.call(config.restTools, toolName);
    if (!inMcpServers && !inRestTools) {
      missingTools.push(toolName);
    }
  });
  if (missingTools.length > 0) {
    throw new Error(`Requested MCP tools not found in configuration: ${missingTools.join(', ')}. Check tool names in frontmatter or CLI arguments.`);
  }

  const mergedEnv: Record<string, string> = {};
  layers.forEach((ly) => {
    const env = ly.env ?? {};
    Object.entries(env).forEach(([key, value]) => { mergedEnv[key] = value; });
  });
  Object.entries(process.env).forEach(([key, value]) => {
    if (typeof value === 'string') mergedEnv[key] = value;
  });
  const expandVars = (s: string): string => s.replace(/\$\{([^}]+)\}/g, (_m, name: string) => mergedEnv[name] ?? '');

  const eff = resolveEffectiveOptions({
    cli: {
      stream: options?.stream,
      parallelToolCalls: options?.parallelToolCalls,
      maxRetries: options?.maxRetries,
      maxToolTurns: options?.maxToolTurns,
      maxToolCallsPerTurn: options?.maxToolCallsPerTurn,
      maxConcurrentTools: options?.maxConcurrentTools,
      llmTimeout: options?.llmTimeout,
      toolTimeout: options?.toolTimeout,
      temperature: options?.temperature,
      topP: options?.topP,
      maxOutputTokens: options?.maxOutputTokens,
      repeatPenalty: options?.repeatPenalty,
      toolResponseMaxBytes: options?.toolResponseMaxBytes,
      mcpInitConcurrency: options?.mcpInitConcurrency,
      traceLLM: options?.traceLLM,
      traceMCP: options?.traceMCP,
      traceSdk: options?.traceSdk,
      verbose: options?.verbose,
      reasoning: options?.reasoning,
      caching: options?.caching,
    },
    global: options?.globalOverrides,
    fm: fm?.options,
    configDefaults: defaults,
    defaultsForUndefined: options?.defaultsForUndefined,
  });

  const schema = buildEffectiveOptionsSchema();
  const parsed = schema.safeParse(eff);
  if (!parsed.success) {
    const msgs = parsed.error.issues.map((issue) => `- ${issue.path.join('.')}: ${issue.message}`).join('\n');
    throw new Error(`Effective options validation failed:\n${msgs}`);
  }

  mergedDefaultsForChildren = (() => {
    const base = options?.defaultsForUndefined !== undefined ? { ...options.defaultsForUndefined } : {};
    if (options?.defaultsForUndefined?.reasoning !== undefined && base.reasoning === undefined) {
      base.reasoning = options.defaultsForUndefined.reasoning;
    }
    if (options?.defaultsForUndefined?.reasoningValue !== undefined && base.reasoningValue === undefined) {
      base.reasoningValue = options.defaultsForUndefined.reasoningValue;
    }
    if (eff.caching !== undefined && base.caching === undefined) base.caching = eff.caching;
    return Object.keys(base).length > 0 ? base : undefined;
  })();

  selectedAgents.forEach((agentPath) => {
    const absPath = canonical(agentPath);
    if (ancestorChain.includes(absPath)) {
      throw new Error(`Recursion detected while loading sub-agent: ${absPath}`);
    }
    const childLoaded = loadAgent(absPath, registry, {
      configLayers: layers,
      verbose: options?.verbose,
      defaultsForUndefined: mergedDefaultsForChildren,
      ancestors: [...ancestorChain, id],
      // Pass through overrides intact; downstream loaders treat arrays as immutable.
      globalOverrides: options?.globalOverrides,
    });
    const derivedToolName = childLoaded.toolName ?? deriveToolNameFromPath(childLoaded.promptPath);
    if (isReservedAgentName(derivedToolName)) {
      throw new Error(`Sub-agent '${childLoaded.promptPath}' uses a reserved tool name '${derivedToolName}'`);
    }
    if (seenToolNames.has(derivedToolName)) {
      throw new Error(`Duplicate sub-agent tool name '${derivedToolName}' detected while loading '${childLoaded.promptPath}'`);
    }
    seenToolNames.add(derivedToolName);
    if (typeof childLoaded.description !== 'string' || childLoaded.description.trim().length === 0) {
      throw new Error(`Sub-agent '${childLoaded.promptPath}' missing 'description' in frontmatter`);
    }
    const inputFormat = childLoaded.input.format === 'json' ? 'json' : 'text';
    const hasExplicitInputSchema = inputFormat === 'json' && JSON.stringify(childLoaded.input.schema) !== DEFAULT_TOOL_INPUT_SCHEMA_JSON;
    const childInfo: PreloadedSubAgent = {
      toolName: derivedToolName,
      description: childLoaded.description,
      usage: childLoaded.usage,
      inputFormat,
      inputSchema: childLoaded.input.schema,
      hasExplicitInputSchema,
      promptPath: childLoaded.promptPath,
      systemTemplate: childLoaded.systemTemplate,
      loaded: childLoaded,
    };
    if (childLoaded.toolName === undefined) {
      (childLoaded as { toolName?: string }).toolName = derivedToolName;
    }
    subAgentInfos.push(childInfo);
  });

  const accountingFile: string | undefined = config.persistence?.billingFile ?? config.accounting?.file;

  const resolvedInput = resolveInputContract(fm?.inputSpec);
  const resolvedExpectedOutput = fm?.expectedOutput !== undefined
    ? { ...fm.expectedOutput, schema: cloneOptionalJsonSchema(fm.expectedOutput.schema) }
    : undefined;
  const resolvedOutputSchema = resolvedExpectedOutput?.format === 'json'
    ? cloneOptionalJsonSchema(resolvedExpectedOutput.schema)
    : undefined;

  const selectedOpenAPIProviders = new Set<string>();
  selectedTools.forEach((toolName) => {
    if (typeof toolName === 'string' && toolName.startsWith('openapi:')) {
      const providerName = toolName.slice(8);
      selectedOpenAPIProviders.add(providerName);
    }
  });
  if (config.openapiSpecs !== undefined && selectedOpenAPIProviders.size > 0) {
    const configLayer = layers.find((ly) => {
      const json = ly.json as { openapiSpecs?: Record<string, unknown> } | undefined;
      return json?.openapiSpecs !== undefined;
    });
    const configDir = configLayer !== undefined ? path.dirname(configLayer.jsonPath) : baseDir;
    const generatedRestTools: Record<string, RestToolConfig> = {};
    const generatedNames: string[] = [];
    selectedOpenAPIProviders.forEach((providerName) => {
      const specCfg = config.openapiSpecs?.[providerName];
      if (specCfg === undefined) {
        throw new Error(`OpenAPI provider '${providerName}' not found in configuration.`);
      }
      const loc = specCfg.spec;
      if (typeof loc !== 'string' || loc.length === 0) {
        throw new Error(`OpenAPI provider '${providerName}' is missing a spec path.`);
      }
      if (loc.startsWith('http://') || loc.startsWith('https://')) {
        throw new Error(`Remote OpenAPI specs violate PR-001 (provider '${providerName}'). Use local cached files instead.`);
      }
      const specPath = path.isAbsolute(loc) ? loc : path.resolve(configDir, loc);
      const raw = readFileText(specPath);
      const spec = parseOpenAPISpec(raw);
      const toolsMap = openApiToRestTools(spec, {
        toolNamePrefix: providerName,
        baseUrlOverride: specCfg.baseUrl,
        includeMethods: specCfg.includeMethods,
        tagFilter: specCfg.tagFilter,
      });
      const defaultHeadersRaw = specCfg.headers ?? {};
      const defaultHeaders: Record<string, string> = {};
      Object.entries(defaultHeadersRaw).forEach(([key, value]) => {
        defaultHeaders[key] = expandVars(value);
      });
      Object.entries(toolsMap).forEach(([name, tool]) => {
        const mergedHeaders = { ...(defaultHeaders), ...(tool.headers ?? {}) };
        tool.headers = Object.keys(mergedHeaders).length > 0 ? mergedHeaders : undefined;
        if (Object.prototype.hasOwnProperty.call(generatedRestTools, name)) {
          throw new Error(`Duplicate OpenAPI tool name '${name}' generated for provider '${providerName}'.`);
        }
        generatedRestTools[name] = tool;
        generatedNames.push(name);
      });
    });
    if (Object.keys(generatedRestTools).length > 0) {
      const mergedRestTools = { ...(config.restTools ?? {}), ...generatedRestTools };
      config = { ...config, restTools: mergedRestTools };
    }
    if (generatedNames.length > 0) {
      selectedTools = selectedTools
        .filter((toolName) => !(typeof toolName === 'string' && toolName.startsWith('openapi:')))
        .concat(generatedNames);
    }
  }

  const agentName = path.basename(promptPath).replace(/\.[^.]+$/, '');

  const createSession = (
    systemPrompt: string,
    userPrompt: string,
    opts?: LoadedAgentSessionOptions
  ): Promise<AIAgentSession> => {
    const o = (opts ?? {}) as LoadedAgentSessionOptions;
    if (o.outputFormat === undefined) throw new Error('outputFormat is required');

    let sessionConfig: AIAgentSessionConfig = {
      config,
      targets: selectedTargets,
      tools: selectedTools,
      agentId: agentName,
      subAgents: subAgentInfos,
      systemPrompt,
      userPrompt,
      outputFormat: o.outputFormat,
      renderTarget: o.renderTarget,
      conversationHistory: o.history,
      expectedOutput: resolvedExpectedOutput,
      callbacks: o.callbacks,
      trace: o.trace,
      stopRef: o.stopRef,
      initialTitle: o.initialTitle,
      abortSignal: (opts as { abortSignal?: AbortSignal } | undefined)?.abortSignal,
      temperature: eff.temperature,
      topP: eff.topP,
      maxOutputTokens: eff.maxOutputTokens,
      repeatPenalty: eff.repeatPenalty,
      maxRetries: eff.maxRetries,
      maxTurns: eff.maxToolTurns,
      maxToolCallsPerTurn: eff.maxToolCallsPerTurn,
      maxConcurrentTools: eff.maxConcurrentTools,
      llmTimeout: eff.llmTimeout,
      toolTimeout: eff.toolTimeout,
      parallelToolCalls: eff.parallelToolCalls,
      stream: eff.stream,
      traceLLM: eff.traceLLM,
      traceMCP: eff.traceMCP,
      traceSdk: eff.traceSdk,
      verbose: eff.verbose,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      mcpInitConcurrency: eff.mcpInitConcurrency,
      reasoning: eff.reasoning,
      reasoningValue: eff.reasoningValue,
      caching: eff.caching,
      headendId: o.headendId ?? o.renderTarget ?? 'cli',
      headendWantsProgressUpdates: o.wantsProgressUpdates !== undefined ? o.wantsProgressUpdates : true,
      // Preserve the original reference (no clone) so recursion guards see identical identity.
      // Harness expectations rely on the session receiving the exact array instance from callers.
      ancestors: Array.isArray(o.ancestors) ? o.ancestors : ancestorChain,
    };
    const resolvedAgentPath = (() => {
      if (typeof o.agentPath === 'string' && o.agentPath.length > 0) return o.agentPath;
      if (typeof sessionConfig.agentPath === 'string' && sessionConfig.agentPath.length > 0) return sessionConfig.agentPath;
      return agentName;
    })();
    sessionConfig.agentPath = resolvedAgentPath;
    const resolvedTurnPathPrefix = (() => {
      if (typeof o.turnPathPrefix === 'string' && o.turnPathPrefix.length > 0) return o.turnPathPrefix;
      if (typeof sessionConfig.turnPathPrefix === 'string' && sessionConfig.turnPathPrefix.length > 0) return sessionConfig.turnPathPrefix;
      return '';
    })();
    sessionConfig.turnPathPrefix = resolvedTurnPathPrefix;
    if (config.telemetry?.labels !== undefined || o.telemetryLabels !== undefined) {
      const combinedLabels: Record<string, string> = { ...(config.telemetry?.labels ?? {}) };
      if (sessionConfig.headendId !== undefined && combinedLabels.headend === undefined) {
        combinedLabels.headend = sessionConfig.headendId;
      }
      if (o.telemetryLabels !== undefined) {
        Object.entries(o.telemetryLabels).forEach(([key, value]) => {
          if (typeof value === 'string') {
            combinedLabels[key] = value;
          }
        });
      }
      if (Object.keys(combinedLabels).length > 0) {
        sessionConfig.telemetryLabels = combinedLabels;
      }
    } else if (sessionConfig.headendId !== undefined) {
      sessionConfig.telemetryLabels = { headend: sessionConfig.headendId };
    }
    sessionConfig.callbacks = mergeCallbacksWithPersistence(sessionConfig.callbacks, config.persistence);
    if (typeof o.traceLLM === 'boolean') {
      sessionConfig.traceLLM = o.traceLLM;
    }
    if (typeof o.traceMCP === 'boolean') {
      sessionConfig.traceMCP = o.traceMCP;
    }
    if (typeof o.traceSdk === 'boolean') {
      sessionConfig.traceSdk = o.traceSdk;
    }
    if (typeof o.verbose === 'boolean') {
      sessionConfig.verbose = o.verbose;
    }
    sessionConfig.trace = {
      ...sessionConfig.trace,
      callPath: sessionConfig.trace?.callPath ?? resolvedAgentPath,
      agentPath: resolvedAgentPath,
      turnPath: resolvedTurnPathPrefix,
    };
    return Promise.resolve(Agent.create(sessionConfig));
  };

  const runSession = async (
    systemPrompt: string,
    userPrompt: string,
    opts?: LoadedAgentSessionOptions
  ): Promise<AIAgentResult> => {
    const session = await createSession(systemPrompt, userPrompt, opts);
    return await session.run();
  };

  return {
    id,
    promptPath,
    systemTemplate,
    description: fm?.description,
    usage: fm?.usage,
    toolName: fm?.toolName,
    expectedOutput: resolvedExpectedOutput,
    input: resolvedInput,
    outputSchema: resolvedOutputSchema,
    config,
    targets: selectedTargets,
    tools: selectedTools,
    subAgents: subAgentInfos,
    accountingFile,
    subTools: [],
    effective: {
      temperature: eff.temperature,
      topP: eff.topP,
      maxOutputTokens: eff.maxOutputTokens,
      repeatPenalty: eff.repeatPenalty,
      llmTimeout: eff.llmTimeout,
      toolTimeout: eff.toolTimeout,
      maxRetries: eff.maxRetries,
      maxToolTurns: eff.maxToolTurns,
      maxToolCallsPerTurn: eff.maxToolCallsPerTurn,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      stream: eff.stream,
      parallelToolCalls: eff.parallelToolCalls,
      maxConcurrentTools: eff.maxConcurrentTools,
      traceLLM: eff.traceLLM,
      traceMCP: eff.traceMCP,
      traceSdk: eff.traceSdk,
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    createSession,
    run: runSession,
  };
}

export function loadAgent(aiPath: string, registry?: LoadedAgentCache, options?: LoadAgentOptions): LoadedAgent {
  const reg = registry ?? new LoadedAgentCache();
  const id = canonical(aiPath);
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const ancestorChain = Array.isArray(options?.ancestors) ? options.ancestors : [];
  if (ancestorChain.includes(id)) {
    throw new Error(`Recursion detected while loading agent: ${id}`);
  }

  const layers: ResolvedConfigLayer[] = options?.configLayers ?? discoverLayers({ configPath: options?.configPath, promptPath: aiPath });
  const baseDir = path.dirname(id);
  const flattened = loadFlattenedPrompt(id);

  return loadAgentCore({
    cache: reg,
    cacheKey: id,
    id,
    promptPath: id,
    promptContent: flattened.content,
    systemTemplate: flattened.systemTemplate,
    baseDir,
    frontmatterBaseDir: baseDir,
    options,
    layers,
    ancestorChain,
  });
}

export function loadAgentFromContent(
  id: string,
  content: string,
  options?: LoadAgentOptions & { baseDir?: string },
  cache?: LoadedAgentCache
): LoadedAgent {
  const reg = cache ?? new LoadedAgentCache();
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const ancestorChain = Array.isArray(options?.ancestors) ? options.ancestors : [];
  if (ancestorChain.includes(id)) {
    throw new Error(`Recursion detected while loading agent content: ${id}`);
  }

  const layers: ResolvedConfigLayer[] = options?.configLayers ?? discoverLayers({ configPath: options?.configPath, promptPath: id });
  const baseDir = options?.baseDir ?? process.cwd();
  const flattened = flattenPromptContent(content, options?.baseDir);

  return loadAgentCore({
    cache: reg,
    cacheKey: id,
    id,
    promptPath: id,
    promptContent: flattened.content,
    systemTemplate: flattened.systemTemplate,
    baseDir,
    frontmatterBaseDir: options?.baseDir,
    options,
    layers,
    ancestorChain,
  });
}
