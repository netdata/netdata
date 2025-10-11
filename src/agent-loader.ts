import fs from 'node:fs';
import path from 'node:path';

import type { AIAgentSession } from './ai-agent.js';
// keep type imports grouped at top
import type { OutputFormatId } from './formats.js';
import type { PreloadedSubAgent } from './subagent-registry.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage, RestToolConfig } from './types.js';
// no runtime format validation here; caller must pass a valid OutputFormatId

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults, type ResolvedConfigLayer } from './config-resolver.js';
import { parseFrontmatter, parseList, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { resolveIncludes } from './include-resolver.js';
import { isReservedAgentName } from './internal-tools.js';
import { resolveEffectiveOptions } from './options-resolver.js';
import { buildEffectiveOptionsSchema } from './options-schema.js';
import { openApiToRestTools, parseOpenAPISpec } from './tools/openapi-importer.js';
import { clampToolName, sanitizeToolName } from './utils.js';


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
    verbose: boolean;
    mcpInitConcurrency?: number;
  };
  subTools: { name: string; description?: string }[]; // placeholder for future
  createSession: (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ) => Promise<AIAgentSession>;
  run: (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
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


export interface LoadAgentOptions {
  configPath?: string;
  configLayers?: ResolvedConfigLayer[];
  verbose?: boolean;
  targets?: { provider: string; model: string }[];
  tools?: string[];
  agents?: string[];
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
  };
  ancestors?: string[];
}

const FALLBACK_INPUT_SCHEMA: Record<string, unknown> = Object.freeze({
  type: 'object',
  properties: {
    prompt: { type: 'string' },
    format: {
      type: 'string',
      enum: ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'],
      default: 'markdown',
    },
  },
  required: ['prompt'],
  additionalProperties: false,
});

const cloneSchemaStrict = (schema: Record<string, unknown>): Record<string, unknown> => JSON.parse(JSON.stringify(schema)) as Record<string, unknown>;
const cloneSchemaOptional = (schema?: Record<string, unknown>): Record<string, unknown> | undefined => (schema === undefined ? undefined : cloneSchemaStrict(schema));

const resolveInputContract = (
  spec?: { format: 'text' | 'json'; schema?: Record<string, unknown> }
): { format: 'json' | 'text'; schema: Record<string, unknown> } => {
  if (spec?.format === 'json') {
    const schema = spec.schema !== undefined ? cloneSchemaStrict(spec.schema) : cloneSchemaStrict(FALLBACK_INPUT_SCHEMA);
    return { format: 'json', schema };
  }
  if (spec?.format === 'text') {
    return { format: 'text', schema: cloneSchemaStrict(FALLBACK_INPUT_SCHEMA) };
  }
  return { format: 'json', schema: cloneSchemaStrict(FALLBACK_INPUT_SCHEMA) };
};


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
  const promptContent = flattened.content;
  const systemTemplate = flattened.systemTemplate;
  const fm = parseFrontmatter(promptContent, { baseDir });

  // Determine needed targets/tools: CLI/opts > frontmatter
  const fmModels = parsePairs(fm?.options?.models);
  const fmTools = parseList(fm?.options?.tools);
  const fmAgents = parseList(fm?.options?.agents);
  const optTargets = options?.targets;
  const selectedTargets = Array.isArray(optTargets) && optTargets.length > 0 ? optTargets : fmModels;
  const optTools = options?.tools;
  let selectedTools = Array.isArray(optTools) && optTools.length > 0 ? optTools : fmTools;
  const effAgents = Array.isArray(options?.agents) && options.agents.length > 0 ? options.agents : fmAgents;
  const selectedAgents = effAgents.map((rel) => path.resolve(baseDir, rel));
  const subAgentInfos: PreloadedSubAgent[] = [];
  const seenToolNames = new Set<string>();
  selectedAgents.forEach((agentPath) => {
    const absPath = canonical(agentPath);
    if (ancestorChain.includes(absPath)) {
      throw new Error(`Recursion detected while loading sub-agent: ${absPath}`);
    }
    const childLoaded = loadAgent(absPath, reg, {
      configLayers: layers,
      verbose: options?.verbose,
      defaultsForUndefined: options?.defaultsForUndefined,
      ancestors: [...ancestorChain, id],
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
    const childInfo: PreloadedSubAgent = {
      toolName: derivedToolName,
      description: childLoaded.description,
      usage: childLoaded.usage,
      inputFormat,
      inputSchema: childLoaded.input.schema,
      promptPath: childLoaded.promptPath,
      systemTemplate: childLoaded.systemTemplate,
      loaded: childLoaded,
    };
    if (childLoaded.toolName === undefined) {
      (childLoaded as { toolName?: string }).toolName = derivedToolName;
    }
    subAgentInfos.push(childInfo);
  });
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));
  const externalToolNames = Array.from(new Set(
    selectedTools.filter((tool) => !tool.includes(':') && !isReservedAgentName(tool))
  ));

  let config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: externalToolNames, restTools: externalToolNames }, layers, { verbose: options?.verbose });
  const dfl = resolveDefaults(layers);
  validateNoPlaceholders(config);
  // Apply persistence overrides from CLI
  if ((typeof options?.sessionsDir === 'string' && options.sessionsDir.length > 0) || (typeof options?.billingFile === 'string' && options.billingFile.length > 0)) {
    const p = { ...(config.persistence ?? {}) } as { sessionsDir?: string; billingFile?: string };
    if (typeof options.sessionsDir === 'string' && options.sessionsDir.length > 0) p.sessionsDir = options.sessionsDir;
    if (typeof options.billingFile === 'string' && options.billingFile.length > 0) p.billingFile = options.billingFile;
    (config as unknown as { persistence?: { sessionsDir?: string; billingFile?: string } }).persistence = p;
  }
  
  // Validate that all requested MCP tools exist in configuration
  const missingTools: string[] = [];
  selectedTools.forEach((toolName) => {
    // Skip special selectors like openapi:*, rest:*, agent:*
    if (toolName.includes(':')) return;
    // Skip internal tools (batch, progress_report, final_report)
    if (isReservedAgentName(toolName)) return;
    // Check both MCP servers and REST tools
    const inMcpServers = Object.prototype.hasOwnProperty.call(config.mcpServers, toolName);
    const inRestTools = config.restTools !== undefined && Object.prototype.hasOwnProperty.call(config.restTools, toolName);
    if (!inMcpServers && !inRestTools) {
      missingTools.push(toolName);
    }
  });
  if (missingTools.length > 0) {
    throw new Error(`Requested MCP tools not found in configuration: ${missingTools.join(', ')}. Check tool names in frontmatter or CLI arguments.`);
  }
  
  // Merge env files to enable ${VAR} expansion for OpenAPI defaults
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
      verbose: options?.verbose,
    },
    fm: fm?.options,
    configDefaults: dfl,
    defaultsForUndefined: options?.defaultsForUndefined,
  });

  // Validate effective options against schema derived from registry
  {
    const schema = buildEffectiveOptionsSchema();
    const parsed = schema.safeParse(eff);
    if (!parsed.success) {
      const msgs = parsed.error.issues.map((i) => `- ${i.path.join('.')}: ${i.message}`).join('\n');
      throw new Error(`Effective options validation failed:\n${msgs}`);
    }
  }

  const accountingFile: string | undefined = config.persistence?.billingFile ?? config.accounting?.file;

  const agentName = path.basename(id).replace(/\.[^.]+$/, '');
  const resolvedInput = resolveInputContract(fm?.inputSpec);
  const resolvedExpectedOutput = fm?.expectedOutput !== undefined
    ? { ...fm.expectedOutput, schema: cloneSchemaOptional(fm.expectedOutput.schema) }
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

  const resolvedOutputSchema = resolvedExpectedOutput?.format === 'json'
    ? cloneSchemaOptional(resolvedExpectedOutput.schema)
    : undefined;

  const createSession = (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ): Promise<AIAgentSession> => {
    const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] };
    if (o.outputFormat === undefined) throw new Error('outputFormat is required');

    let dynamicConfig = config;
    let dynamicTools = [...selectedTools];

    const sessionConfig: AIAgentSessionConfig = {
      config: dynamicConfig,
      targets: selectedTargets,
      tools: dynamicTools,
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
      verbose: eff.verbose,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    };
    return Promise.resolve(Agent.create(sessionConfig));
  };

  const runSession = async (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ): Promise<AIAgentResult> => {
    const session = await createSession(systemPrompt, userPrompt, opts);
    return await session.run();
  };
  const loaded: LoadedAgent = {
    id,
    promptPath: id,
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
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    subTools: [],
    createSession,
    run: runSession,
  };
  reg.set(id, loaded);
  return loaded;
}

export function loadAgentFromContent(id: string, content: string, options?: LoadAgentOptions & { baseDir?: string }): LoadedAgent {
  const reg = new LoadedAgentCache();
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const ancestorChain = Array.isArray(options?.ancestors) ? options.ancestors : [];
  if (ancestorChain.includes(id)) {
    throw new Error(`Recursion detected while loading agent content: ${id}`);
  }

  const layers: ResolvedConfigLayer[] = options?.configLayers ?? discoverLayers({ configPath: options?.configPath, promptPath: id });
  const baseDir = options?.baseDir ?? process.cwd();
  const flattened = flattenPromptContent(content, options?.baseDir);
  const contentWithIncludes = flattened.content;
  const systemTemplate = flattened.systemTemplate;
  const fm = parseFrontmatter(contentWithIncludes, { baseDir: options?.baseDir });
  const fmModels = parsePairs(fm?.options?.models);
  const fmTools = parseList(fm?.options?.tools);
  const fmAgents = parseList(fm?.options?.agents);
  const optTargets2 = options?.targets;
  const selectedTargets = Array.isArray(optTargets2) && optTargets2.length > 0 ? optTargets2 : fmModels;
  const optTools2 = options?.tools;
  let selectedTools = Array.isArray(optTools2) && optTools2.length > 0 ? optTools2 : fmTools;
  const effAgents2 = Array.isArray(options?.agents) && options.agents.length > 0 ? options.agents : fmAgents;
  const selectedAgents = effAgents2.map((rel) => path.resolve(options?.baseDir ?? process.cwd(), rel));
  const subAgentInfos: PreloadedSubAgent[] = [];
  const seenToolNames = new Set<string>();
  selectedAgents.forEach((agentPath) => {
    const absPath = canonical(agentPath);
    const childLoaded = loadAgent(absPath, reg, {
      configLayers: layers,
      verbose: options?.verbose,
      defaultsForUndefined: options?.defaultsForUndefined,
      ancestors: [...ancestorChain, id],
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
    const childInfo: PreloadedSubAgent = {
      toolName: derivedToolName,
      description: childLoaded.description,
      usage: childLoaded.usage,
      inputFormat,
      inputSchema: childLoaded.input.schema,
      promptPath: childLoaded.promptPath,
      systemTemplate: childLoaded.systemTemplate,
      loaded: childLoaded,
    };
    if (childLoaded.toolName === undefined) {
      (childLoaded as { toolName?: string }).toolName = derivedToolName;
    }
    subAgentInfos.push(childInfo);
  });
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));
  const externalToolNames = Array.from(new Set(
    selectedTools.filter((tool) => !tool.includes(':') && !isReservedAgentName(tool))
  ));

  let config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: externalToolNames, restTools: externalToolNames }, layers, { verbose: options?.verbose });
  const dfl = resolveDefaults(layers);
  validateNoPlaceholders(config);

  const mergedEnvForContent: Record<string, string> = {};
  layers.forEach((ly) => {
    const env = ly.env ?? {};
    Object.entries(env).forEach(([key, value]) => { mergedEnvForContent[key] = value; });
  });
  Object.entries(process.env).forEach(([key, value]) => {
    if (typeof value === 'string') mergedEnvForContent[key] = value;
  });
  const expandVarsLocal = (s: string): string => s.replace(/\$\{([^}]+)\}/g, (_m, name: string) => mergedEnvForContent[name] ?? '');
  
  // Validate that all requested MCP tools exist in configuration
  const missingTools: string[] = [];
  selectedTools.forEach((toolName) => {
    // Skip special selectors like openapi:*, rest:*, agent:*
    if (toolName.includes(':')) return;
    // Skip internal tools (batch, progress_report, final_report)
    if (isReservedAgentName(toolName)) return;
    // Check both MCP servers and REST tools
    const inMcpServers = Object.prototype.hasOwnProperty.call(config.mcpServers, toolName);
    const inRestTools = config.restTools !== undefined && Object.prototype.hasOwnProperty.call(config.restTools, toolName);
    if (!inMcpServers && !inRestTools) {
      missingTools.push(toolName);
    }
  });
  if (missingTools.length > 0) {
    throw new Error(`Requested MCP tools not found in configuration: ${missingTools.join(', ')}. Check tool names in frontmatter or CLI arguments.`);
  }

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
      verbose: options?.verbose,
    },
    fm: fm?.options,
    configDefaults: dfl,
    defaultsForUndefined: options?.defaultsForUndefined,
  });

  // Validate effective options against schema derived from registry
  {
    const schema = buildEffectiveOptionsSchema();
    const parsed = schema.safeParse(eff);
    if (!parsed.success) {
      const msgs = parsed.error.issues.map((i) => `- ${i.path.join('.')}: ${i.message}`).join('\n');
      throw new Error(`Effective options validation failed:\n${msgs}`);
    }
  }

  const accountingFile: string | undefined = config.persistence?.billingFile ?? config.accounting?.file;

  const resolvedInput = resolveInputContract(fm?.inputSpec);
  const resolvedExpectedOutput = fm?.expectedOutput !== undefined
    ? { ...fm.expectedOutput, schema: cloneSchemaOptional(fm.expectedOutput.schema) }
    : undefined;
  const resolvedOutputSchema = resolvedExpectedOutput?.format === 'json'
    ? cloneSchemaOptional(resolvedExpectedOutput.schema)
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
        defaultHeaders[key] = expandVarsLocal(value);
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

  const createSession = (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ): Promise<AIAgentSession> => {
    const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] };
    if (o.outputFormat === undefined) throw new Error('outputFormat is required');

    let dynamicConfig = config;
    let dynamicTools = [...selectedTools];

    const sessionConfig: AIAgentSessionConfig = {
      config: dynamicConfig,
      targets: selectedTargets,
      tools: dynamicTools,
      agentId: path.basename(id).replace(/\.[^.]+$/, ''),
      subAgents: subAgentInfos,
      systemPrompt,
      userPrompt,
      outputFormat: o.outputFormat,
      renderTarget: o.renderTarget,
      conversationHistory: o.history,
      expectedOutput: resolvedExpectedOutput,
      callbacks: o.callbacks,
      trace: o.trace,
      initialTitle: o.initialTitle,
      ancestors: Array.isArray(o.ancestors) ? o.ancestors : undefined,
      abortSignal: o.abortSignal,
      stopRef: o.stopRef,
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
      verbose: eff.verbose,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    };
    return Promise.resolve(Agent.create(sessionConfig));
  };

  const runSession = async (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ): Promise<AIAgentResult> => {
    const session = await createSession(systemPrompt, userPrompt, opts);
    return await session.run();
  };

  const loaded: LoadedAgent = {
    id,
    promptPath: id,
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
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    createSession,
    run: runSession,
  };
  reg.set(id, loaded);
  return loaded;
}
