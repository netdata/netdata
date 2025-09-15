import fs from 'node:fs';
import path from 'node:path';

// keep type imports grouped at top
import type { OutputFormatId } from './formats.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage, OpenAPISpecConfig } from './types.js';
// no runtime format validation here; caller must pass a valid OutputFormatId

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults } from './config-resolver.js';
import { parseFrontmatter, parseList, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { resolveIncludes } from './include-resolver.js';
import { isReservedAgentName } from './internal-tools.js';
import { resolveEffectiveOptions } from './options-resolver.js';
import { buildEffectiveOptionsSchema } from './options-schema.js';
import { openApiToRestTools, parseOpenAPISpec } from './tools/openapi-importer.js';

export interface LoadedAgent {
  id: string; // canonical path (or synthetic when no file)
  promptPath: string;
  systemTemplate: string;
  description?: string;
  usage?: string;
  expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> };
  config: Configuration;
  targets: { provider: string; model: string }[];
  tools: string[];
  accountingFile?: string;
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
  run: (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }
  ) => Promise<AIAgentResult>;
}

export class AgentRegistry {
  private readonly cache = new Map<string, LoadedAgent>();

  get(key: string): LoadedAgent | undefined { return this.cache.get(key); }
  set(key: string, val: LoadedAgent): void { this.cache.set(key, val); }
}

function canonical(p: string): string {
  try { return fs.realpathSync(p); } catch { return p; }
}

function readFileText(p: string): string { return fs.readFileSync(p, 'utf-8'); }

interface LoadAgentOptions {
  configPath?: string;
  verbose?: boolean;
  targets?: { provider: string; model: string }[];
  tools?: string[];
  agents?: string[];
  // Optional overrides (CLI precedence) for runtime knobs
  stream?: boolean;
  parallelToolCalls?: boolean;
  maxRetries?: number;
  maxToolTurns?: number;
  maxConcurrentTools?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
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

export function loadAgent(aiPath: string, registry?: AgentRegistry, options?: LoadAgentOptions): LoadedAgent {
  const reg = registry ?? new AgentRegistry();
  const id = canonical(aiPath);
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const baseDir = path.dirname(aiPath);
  const content = resolveIncludes(readFileText(aiPath), baseDir);
  const fm = parseFrontmatter(content, { baseDir: path.dirname(aiPath) });
  const systemTemplate = extractBodyWithoutFrontmatter(content);

  // Determine needed targets/tools: CLI/opts > frontmatter
  const fmModels = parsePairs(fm?.options?.models);
  const fmTools = parseList(fm?.options?.tools);
  const fmAgents = parseList(fm?.options?.agents);
  const optTargets = options?.targets;
  const selectedTargets = Array.isArray(optTargets) && optTargets.length > 0 ? optTargets : fmModels;
  const optTools = options?.tools;
  const selectedTools = Array.isArray(optTools) && optTools.length > 0 ? optTools : fmTools;
  const effAgents = Array.isArray(options?.agents) && options.agents.length > 0 ? options.agents : fmAgents;
  const selectedAgents = effAgents.map((rel) => path.resolve(path.dirname(aiPath), rel));
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));

  const layers = discoverLayers({ configPath: options?.configPath });
  const config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: selectedTools }, layers, { verbose: options?.verbose });
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
    // Skip internal tools (batch, append_notes, final_report)
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
    const e = ly.env ?? {};
    Object.keys(e).forEach((k) => { mergedEnv[k] = e[k]; });
  });
  Object.keys(process.env).forEach((k) => { const v = process.env[k]; if (typeof v === 'string') mergedEnv[k] = v; });
  const expandVars = (s: string): string => s.replace(/\$\{([^}]+)\}/g, (_m, name: string) => mergedEnv[name] ?? '');

  const eff = resolveEffectiveOptions({
    cli: {
      stream: options?.stream,
      parallelToolCalls: options?.parallelToolCalls,
      maxRetries: options?.maxRetries,
      maxToolTurns: options?.maxToolTurns,
      maxConcurrentTools: options?.maxConcurrentTools,
      llmTimeout: options?.llmTimeout,
      toolTimeout: options?.toolTimeout,
      temperature: options?.temperature,
      topP: options?.topP,
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
  const loaded: LoadedAgent = {
    id,
    promptPath: id,
    systemTemplate,
    description: fm?.description,
    usage: fm?.usage,
    expectedOutput: fm?.expectedOutput,
    config,
    targets: selectedTargets,
    tools: selectedTools,
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
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }): Promise<AIAgentResult> => {
      const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] };
      if (o.outputFormat === undefined) throw new Error('outputFormat is required');
      // Support dynamic OpenAPI tool import from config.openapiSpecs
      // PR-002: Only load OpenAPI tools if agent explicitly selects the provider
      let dynamicConfig = config;
      let dynamicTools = [...selectedTools];
      
      // Check if agent has selected any OpenAPI providers (format: "openapi:provider_name")
      const selectedOpenAPIProviders = new Set<string>();
      selectedTools.forEach(toolName => {
        if (typeof toolName === 'string' && toolName.startsWith('openapi:')) {
          const providerName = toolName.slice(8); // Remove 'openapi:' prefix
          selectedOpenAPIProviders.add(providerName);
        }
      });
      
      // Load OpenAPI specs from config (these should be local files, not URLs - PR-001)
      const openapiSpecs = (dynamicConfig as unknown as { openapiSpecs?: Record<string, OpenAPISpecConfig> }).openapiSpecs;
      if (o.callbacks?.onLog !== undefined && openapiSpecs !== undefined) {
        o.callbacks.onLog({
          timestamp: Date.now(),
          severity: 'VRB',
          turn: 0,
          subturn: 0,
          direction: 'request',
          type: 'tool',
          remoteIdentifier: 'openapi',
          fatal: false,
          message: `Found OpenAPI specs in config: ${JSON.stringify(Object.keys(openapiSpecs))}, selected providers: ${JSON.stringify(Array.from(selectedOpenAPIProviders))}`
        });
      }
      if (openapiSpecs !== undefined && selectedOpenAPIProviders.size > 0) {
        // Find the config file path to resolve relative paths
        const configLayer = layers.find(l => {
          const json = l.json;
          return json !== undefined && typeof json === 'object' && 'openapiSpecs' in json;
        });
        const configDir = configLayer !== undefined ? path.dirname(configLayer.jsonPath) : baseDir;
        
        const entries = Object.entries(openapiSpecs);
        // eslint-disable-next-line functional/no-loop-statements
        for (const [name, specCfg] of entries) {
          // PR-002: Only load tools for explicitly selected providers
          if (!selectedOpenAPIProviders.has(name)) {
            continue;
          }
          
          try {
            const loc = specCfg.spec;
            let text: string;
            // PR-001: Only support local files, not URLs
            if (loc.startsWith('http://') || loc.startsWith('https://')) {
              throw new Error('Remote OpenAPI specs violate PR-001. Use local cached files instead.');
            } else {
              // Resolve relative paths relative to the config file location
              const p = path.isAbsolute(loc) ? loc : path.resolve(configDir, loc);
              if (o.callbacks?.onLog !== undefined) {
                o.callbacks.onLog({
                  timestamp: Date.now(),
                  severity: 'VRB',
                  turn: 0,
                  subturn: 0,
                  direction: 'request',
                  type: 'tool',
                  remoteIdentifier: `openapi:${name}`,
                  fatal: false,
                  message: `Loading OpenAPI spec from: ${p} (loc=${loc}, configDir=${configDir})`
                });
              }
              text = readFileText(p);
            }
            const spec = parseOpenAPISpec(text);
            const toolsMap = openApiToRestTools(spec, { 
              toolNamePrefix: name, 
              baseUrlOverride: specCfg.baseUrl, 
              includeMethods: specCfg.includeMethods, 
              tagFilter: specCfg.tagFilter 
            });
            // Merge default headers (with ${VAR} expansion)
            const defaultHeadersRaw = specCfg.headers ?? {};
            const defaultHeaders: Record<string, string> = {};
            Object.entries(defaultHeadersRaw).forEach(([k, v]) => { defaultHeaders[k] = expandVars(v); });
            Object.values(toolsMap).forEach((t) => {
              const merged = { ...(defaultHeaders), ...(t.headers ?? {}) };
              t.headers = Object.keys(merged).length > 0 ? merged : undefined;
            });
            const mergedRest = { ...(dynamicConfig.restTools ?? {}), ...toolsMap };
            dynamicConfig = { ...dynamicConfig, restTools: mergedRest };
            // Add the generated tools (but not the openapi:provider selector itself)
            const newTools = Object.keys(toolsMap);
            dynamicTools = dynamicTools.filter(t => t !== `openapi:${name}`).concat(newTools);
            
            if (o.callbacks?.onLog !== undefined) {
              o.callbacks.onLog({
                timestamp: Date.now(),
                severity: 'VRB',
                turn: 0,
                subturn: 0,
                direction: 'response',
                type: 'tool',
                remoteIdentifier: `openapi:${name}`,
                fatal: false,
                message: `Loaded ${String(newTools.length)} tools from OpenAPI provider: ${name}`
              });
            }
          } catch (e) {
            throw new Error(`Failed to import OpenAPI spec '${name}': ${e instanceof Error ? e.message : String(e)}`);
          }
        }
      }

      const sessionConfig: AIAgentSessionConfig = {
        config: dynamicConfig,
        targets: selectedTargets,
        tools: dynamicTools,
        agentId: agentName,
        subAgentPaths: selectedAgents,
        systemPrompt,
        userPrompt,
        outputFormat: o.outputFormat,
        renderTarget: o.renderTarget,
        conversationHistory: o.history,
        expectedOutput: fm?.expectedOutput,
        callbacks: o.callbacks,
        trace: o.trace,
        stopRef: o.stopRef,
        initialTitle: o.initialTitle,
        // Propagate headend abort signal if present
        abortSignal: (opts as unknown as { abortSignal?: AbortSignal } | undefined)?.abortSignal,
        temperature: eff.temperature,
        topP: eff.topP,
        maxOutputTokens: eff.maxOutputTokens,
        repeatPenalty: eff.repeatPenalty,
        maxRetries: eff.maxRetries,
        maxTurns: eff.maxToolTurns,
        maxToolCallsPerTurn: eff.maxToolCallsPerTurn,
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
      const session = Agent.create(sessionConfig);
      return await session.run();
    }
  };
  reg.set(id, loaded);
  return loaded;
}

export function loadAgentFromContent(id: string, content: string, options?: LoadAgentOptions & { baseDir?: string }): LoadedAgent {
  const reg = new AgentRegistry();
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const contentWithIncludes = resolveIncludes(content, options?.baseDir);
  const fm = parseFrontmatter(contentWithIncludes, { baseDir: options?.baseDir });
  const systemTemplate = extractBodyWithoutFrontmatter(contentWithIncludes);
  const fmModels = parsePairs(fm?.options?.models);
  const fmTools = parseList(fm?.options?.tools);
  const fmAgents = parseList(fm?.options?.agents);
  const optTargets2 = options?.targets;
  const selectedTargets = Array.isArray(optTargets2) && optTargets2.length > 0 ? optTargets2 : fmModels;
  const optTools2 = options?.tools;
  const selectedTools = Array.isArray(optTools2) && optTools2.length > 0 ? optTools2 : fmTools;
  const effAgents2 = Array.isArray(options?.agents) && options.agents.length > 0 ? options.agents : fmAgents;
  const selectedAgents = effAgents2.map((rel) => path.resolve(options?.baseDir ?? process.cwd(), rel));
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));

  const layers = discoverLayers({ configPath: options?.configPath });
  const config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: selectedTools }, layers, { verbose: options?.verbose });
  const dfl = resolveDefaults(layers);
  validateNoPlaceholders(config);
  
  // Validate that all requested MCP tools exist in configuration
  const missingTools: string[] = [];
  selectedTools.forEach((toolName) => {
    // Skip special selectors like openapi:*, rest:*, agent:*
    if (toolName.includes(':')) return;
    // Skip internal tools (batch, append_notes, final_report)
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
      maxConcurrentTools: options?.maxConcurrentTools,
      llmTimeout: options?.llmTimeout,
      toolTimeout: options?.toolTimeout,
      temperature: options?.temperature,
      topP: options?.topP,
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

  const loaded: LoadedAgent = {
    id,
    promptPath: id,
    systemTemplate,
    description: fm?.description,
    usage: fm?.usage,
    expectedOutput: fm?.expectedOutput,
    config,
    targets: selectedTargets,
    tools: selectedTools,
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
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] }): Promise<AIAgentResult> => {
      const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; ancestors?: string[] };
      if (o.outputFormat === undefined) throw new Error('outputFormat is required');
      
      // Support dynamic OpenAPI tool import from config.openapiSpecs
      // PR-002: Only load OpenAPI tools if agent explicitly selects the provider
      let dynamicConfig = config;
      let dynamicTools = [...selectedTools];
      
      // Helper for expanding ${VAR} in strings
      const expandVars = (s: string): string => {
        // For loadAgentFromContent, we need to get env from layers
        const layers = discoverLayers({ configPath: options?.configPath });
        const mergedEnv: Record<string, string> = {};
        layers.forEach(l => { if (l.env !== undefined) Object.assign(mergedEnv, l.env); });
        return s.replace(/\$\{([^}]+)\}/g, (_m, name: string) => mergedEnv[name] ?? '');
      };
      
      // Check if agent has selected any OpenAPI providers (format: "openapi:provider_name")
      const selectedOpenAPIProviders = new Set<string>();
      selectedTools.forEach(toolName => {
        if (typeof toolName === 'string' && toolName.startsWith('openapi:')) {
          const providerName = toolName.slice(8); // Remove 'openapi:' prefix
          selectedOpenAPIProviders.add(providerName);
        }
      });
      
      // Load OpenAPI specs from config (these should be local files, not URLs - PR-001)
      const openapiSpecs = (dynamicConfig as unknown as { openapiSpecs?: Record<string, OpenAPISpecConfig> }).openapiSpecs;
      if (openapiSpecs !== undefined && selectedOpenAPIProviders.size > 0) {
        // For loadAgentFromContent, we need to find config from layers
        const layers = discoverLayers({ configPath: options?.configPath });
        const configLayer = layers.find(l => {
          const json = l.json;
          return json !== undefined && typeof json === 'object' && 'openapiSpecs' in json;
        });
        const configDir = configLayer !== undefined ? path.dirname(configLayer.jsonPath) : (options?.baseDir ?? process.cwd());
        
        const entries = Object.entries(openapiSpecs);
        // eslint-disable-next-line functional/no-loop-statements
        for (const [name, specCfg] of entries) {
          // PR-002: Only load tools for explicitly selected providers
          if (!selectedOpenAPIProviders.has(name)) {
            continue;
          }
          
          try {
            const loc = specCfg.spec;
            let text: string;
            // PR-001: Only support local files, not URLs
            if (loc.startsWith('http://') || loc.startsWith('https://')) {
              throw new Error('Remote OpenAPI specs violate PR-001. Use local cached files instead.');
            } else {
              // Resolve relative paths relative to the config file location
              const p = path.isAbsolute(loc) ? loc : path.resolve(configDir, loc);
              text = readFileText(p);
            }
            const spec = parseOpenAPISpec(text);
            const toolsMap = openApiToRestTools(spec, { 
              toolNamePrefix: name, 
              baseUrlOverride: specCfg.baseUrl, 
              includeMethods: specCfg.includeMethods, 
              tagFilter: specCfg.tagFilter 
            });
            // Merge default headers (with ${VAR} expansion)
            const defaultHeadersRaw = specCfg.headers ?? {};
            const defaultHeaders: Record<string, string> = {};
            Object.entries(defaultHeadersRaw).forEach(([k, v]) => { defaultHeaders[k] = expandVars(v); });
            Object.values(toolsMap).forEach((t) => {
              const merged = { ...(defaultHeaders), ...(t.headers ?? {}) };
              t.headers = Object.keys(merged).length > 0 ? merged : undefined;
            });
            const mergedRest = { ...(dynamicConfig.restTools ?? {}), ...toolsMap };
            dynamicConfig = { ...dynamicConfig, restTools: mergedRest };
            // Add the generated tools (but not the openapi:provider selector itself)
            const newTools = Object.keys(toolsMap);
            dynamicTools = dynamicTools.filter(t => t !== `openapi:${name}`).concat(newTools);
          } catch (e) {
            throw new Error(`Failed to import OpenAPI spec '${name}': ${e instanceof Error ? e.message : String(e)}`);
          }
        }
      }

      const sessionConfig: AIAgentSessionConfig = {
        config: dynamicConfig,
        targets: selectedTargets,
        tools: dynamicTools,
        agentId: path.basename(id).replace(/\.[^.]+$/, ''),
        subAgentPaths: selectedAgents,
        systemPrompt,
        userPrompt,
        outputFormat: o.outputFormat,
        renderTarget: o.renderTarget,
        conversationHistory: o.history,
        expectedOutput: fm?.expectedOutput,
        callbacks: o.callbacks,
        trace: o.trace,
        initialTitle: o.initialTitle,
        ancestors: Array.isArray(o.ancestors) ? o.ancestors : undefined,
        // Propagate control signals to child sessions
        abortSignal: o.abortSignal,
        stopRef: o.stopRef,
        temperature: eff.temperature,
        topP: eff.topP,
        maxRetries: eff.maxRetries,
        maxTurns: eff.maxToolTurns,
        maxToolCallsPerTurn: eff.maxToolCallsPerTurn,
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
      const session = Agent.create(sessionConfig);
      return await session.run();
    }
  };
  reg.set(id, loaded);
  return loaded;
}
