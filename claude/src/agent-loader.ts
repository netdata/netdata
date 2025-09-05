import fs from 'node:fs';
import path from 'node:path';

// keep type imports grouped at top
import type { OutputFormatId } from './formats.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from './types.js';
// no runtime format validation here; caller must pass a valid OutputFormatId

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults } from './config-resolver.js';
import { parseFrontmatter, parseList, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { resolveIncludes } from './include-resolver.js';
import { resolveEffectiveOptions } from './options-resolver.js';
import { buildEffectiveOptionsSchema } from './options-schema.js';

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
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId }
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

export interface LoadAgentOptions {
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

  const accountingFile: string | undefined = config.accounting?.file;

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
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId }): Promise<AIAgentResult> => {
      const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId };
      if (o.outputFormat === undefined) throw new Error('outputFormat is required');
      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets: selectedTargets,
        tools: selectedTools,
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
        temperature: eff.temperature,
        topP: eff.topP,
        maxOutputTokens: eff.maxOutputTokens,
        repeatPenalty: eff.repeatPenalty,
        maxRetries: eff.maxRetries,
        maxTurns: eff.maxToolTurns,
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

  const accountingFile: string | undefined = config.accounting?.file;

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
      llmTimeout: eff.llmTimeout,
      toolTimeout: eff.toolTimeout,
      maxRetries: eff.maxRetries,
      maxToolTurns: eff.maxToolTurns,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      stream: eff.stream,
      parallelToolCalls: eff.parallelToolCalls,
      maxConcurrentTools: eff.maxConcurrentTools,
      traceLLM: eff.traceLLM,
      traceMCP: eff.traceMCP,
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'; outputFormat: OutputFormatId }): Promise<AIAgentResult> => {
      const o = (opts ?? {}) as { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; trace?: AIAgentSessionConfig['trace']; renderTarget?: 'cli'|'slack'|'api'|'web'|'sub-agent'; outputFormat?: OutputFormatId };
      if (o.outputFormat === undefined) throw new Error('outputFormat is required');
      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets: selectedTargets,
        tools: selectedTools,
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
        temperature: eff.temperature,
        topP: eff.topP,
        maxRetries: eff.maxRetries,
        maxTurns: eff.maxToolTurns,
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
