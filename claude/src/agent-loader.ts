import fs from 'node:fs';
import path from 'node:path';

import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from './types.js';

import { AIAgent as Agent } from './ai-agent.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults } from './config-resolver.js';
import { parseFrontmatter, parseList, parsePairs } from './frontmatter.js';

export interface LoadedAgent {
  id: string; // canonical path (or synthetic when no file)
  promptPath: string;
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
    llmTimeout: number;
    toolTimeout: number;
    maxRetries: number;
    maxToolTurns: number;
    toolResponseMaxBytes: number;
    stream: boolean;
    parallelToolCalls: boolean;
    traceLLM: boolean;
    traceMCP: boolean;
    verbose: boolean;
    mcpInitConcurrency?: number;
  };
  subTools: { name: string; description?: string }[]; // placeholder for future
  run: (
    systemPrompt: string,
    userPrompt: string,
    opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks }
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
  // Optional overrides (CLI precedence) for runtime knobs
  stream?: boolean;
  parallelToolCalls?: boolean;
  maxRetries?: number;
  maxToolTurns?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  toolResponseMaxBytes?: number;
  mcpInitConcurrency?: number;
  traceLLM?: boolean;
  traceMCP?: boolean;
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

  const content = readFileText(aiPath);
  const fm = parseFrontmatter(content, { baseDir: path.dirname(aiPath) });

  // Determine needed targets/tools: CLI/opts > frontmatter
  const fmTargets = parsePairs((fm?.options?.llms ?? fm?.options?.targets));
  const fmTools = parseList(fm?.options?.tools);
  const optTargets = options?.targets;
  const selectedTargets = Array.isArray(optTargets) && optTargets.length > 0 ? optTargets : fmTargets;
  const optTools = options?.tools;
  const selectedTools = Array.isArray(optTools) && optTools.length > 0 ? optTools : fmTools;
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));

  const layers = discoverLayers({ configPath: options?.configPath });
  const config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: selectedTools }, layers, { verbose: options?.verbose });
  const dfl = resolveDefaults(layers);
  validateNoPlaceholders(config);

  const readNum = (ovr: number | undefined, fmv: unknown, dkey: keyof NonNullable<Configuration['defaults']>, fallback: number): number => {
    if (typeof ovr === 'number' && Number.isFinite(ovr)) return ovr;
    const n = Number(fmv);
    if (Number.isFinite(n)) return n;
    const dv = dfl[dkey];
    if (typeof dv === 'number' && Number.isFinite(dv)) return dv;
    return fallback;
  };
  const readBool = (ovr: boolean | undefined, fmv: unknown, dkey: keyof NonNullable<Configuration['defaults']>, fallback: boolean): boolean => {
    if (typeof ovr === 'boolean') return ovr;
    if (typeof fmv === 'boolean') return fmv;
    const dv = dfl[dkey];
    if (typeof dv === 'boolean') return dv;
    return fallback;
  };

  const fmOpts = fm?.options;
  const fmTraceLLM = (fmOpts !== undefined && typeof fmOpts.traceLLM === 'boolean') ? fmOpts.traceLLM : undefined;
  const fmTraceMCP = (fmOpts !== undefined && typeof fmOpts.traceMCP === 'boolean') ? fmOpts.traceMCP : undefined;
  const fmVerbose = (fmOpts !== undefined && typeof fmOpts.verbose === 'boolean') ? fmOpts.verbose : undefined;

  const eff = {
    temperature: readNum(options?.temperature, fmOpts?.temperature, 'temperature', 0.7),
    topP: readNum(options?.topP, fmOpts?.topP, 'topP', 1.0),
    llmTimeout: readNum(options?.llmTimeout, fmOpts?.llmTimeout, 'llmTimeout', 120000),
    toolTimeout: readNum(options?.toolTimeout, fmOpts?.toolTimeout, 'toolTimeout', 60000),
    maxRetries: readNum(options?.maxRetries, fmOpts?.maxRetries, 'maxRetries', 3),
    maxToolTurns: readNum(options?.maxToolTurns, fmOpts?.maxToolTurns, 'maxToolTurns', 10),
    toolResponseMaxBytes: readNum(options?.toolResponseMaxBytes, fmOpts?.toolResponseMaxBytes, 'toolResponseMaxBytes', 12288),
    stream: readBool(options?.stream, fmOpts?.stream, 'stream', false),
    parallelToolCalls: readBool(options?.parallelToolCalls, fmOpts?.parallelToolCalls, 'parallelToolCalls', false),
    traceLLM: typeof options?.traceLLM === 'boolean' ? options.traceLLM : (fmTraceLLM ?? false),
    traceMCP: typeof options?.traceMCP === 'boolean' ? options.traceMCP : (fmTraceMCP ?? false),
    verbose: (options?.verbose === true) || (fmVerbose === true),
    mcpInitConcurrency: ((): number | undefined => {
      if (typeof options?.mcpInitConcurrency === 'number' && Number.isFinite(options.mcpInitConcurrency)) return Math.trunc(options.mcpInitConcurrency);
      const dv = config.defaults?.mcpInitConcurrency;
      if (typeof dv === 'number' && Number.isFinite(dv)) return Math.trunc(dv);
      return undefined;
    })()
  };

  const accountingFile: string | undefined = ((): string | undefined => {
    if (fmOpts !== undefined && typeof fmOpts.accounting === 'string' && fmOpts.accounting.length > 0) return fmOpts.accounting;
    return config.accounting?.file;
  })();

  const loaded: LoadedAgent = {
    id,
    promptPath: id,
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
      llmTimeout: eff.llmTimeout,
      toolTimeout: eff.toolTimeout,
      maxRetries: eff.maxRetries,
      maxToolTurns: eff.maxToolTurns,
      toolResponseMaxBytes: eff.toolResponseMaxBytes,
      stream: eff.stream,
      parallelToolCalls: eff.parallelToolCalls,
      traceLLM: eff.traceLLM,
      traceMCP: eff.traceMCP,
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    subTools: [],
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks }): Promise<AIAgentResult> => {
      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets: selectedTargets,
        tools: selectedTools,
        systemPrompt,
        userPrompt,
        conversationHistory: opts?.history,
        expectedOutput: fm?.expectedOutput,
        callbacks: opts?.callbacks,
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

export function loadAgentFromContent(id: string, content: string, options?: LoadAgentOptions & { baseDir?: string }): LoadedAgent {
  const reg = new AgentRegistry();
  const cached = reg.get(id);
  if (cached !== undefined) return cached;

  const fm = parseFrontmatter(content, { baseDir: options?.baseDir });
  const fmTargets = parsePairs((fm?.options?.llms ?? fm?.options?.targets));
  const fmTools = parseList(fm?.options?.tools);
  const optTargets2 = options?.targets;
  const selectedTargets = Array.isArray(optTargets2) && optTargets2.length > 0 ? optTargets2 : fmTargets;
  const optTools2 = options?.tools;
  const selectedTools = Array.isArray(optTools2) && optTools2.length > 0 ? optTools2 : fmTools;
  const needProviders = Array.from(new Set(selectedTargets.map((t) => t.provider)));

  const layers = discoverLayers({ configPath: options?.configPath });
  const config = buildUnifiedConfiguration({ providers: needProviders, mcpServers: selectedTools }, layers, { verbose: options?.verbose });
  const dfl = resolveDefaults(layers);
  validateNoPlaceholders(config);

  const readNum = (ovr: number | undefined, fmv: unknown, dkey: keyof NonNullable<Configuration['defaults']>, fallback: number): number => {
    if (typeof ovr === 'number' && Number.isFinite(ovr)) return ovr;
    const n = Number(fmv);
    if (Number.isFinite(n)) return n;
    const dv = dfl[dkey];
    if (typeof dv === 'number' && Number.isFinite(dv)) return dv;
    return fallback;
  };
  const readBool = (ovr: boolean | undefined, fmv: unknown, dkey: keyof NonNullable<Configuration['defaults']>, fallback: boolean): boolean => {
    if (typeof ovr === 'boolean') return ovr;
    if (typeof fmv === 'boolean') return fmv;
    const dv = dfl[dkey];
    if (typeof dv === 'boolean') return dv;
    return fallback;
  };

  const fmOpts2 = fm?.options;
  const fmTraceLLM2 = (fmOpts2 !== undefined && typeof fmOpts2.traceLLM === 'boolean') ? fmOpts2.traceLLM : undefined;
  const fmTraceMCP2 = (fmOpts2 !== undefined && typeof fmOpts2.traceMCP === 'boolean') ? fmOpts2.traceMCP : undefined;
  const fmVerbose2 = (fmOpts2 !== undefined && typeof fmOpts2.verbose === 'boolean') ? fmOpts2.verbose : undefined;

  const eff = {
    temperature: readNum(options?.temperature, fmOpts2?.temperature, 'temperature', 0.7),
    topP: readNum(options?.topP, fmOpts2?.topP, 'topP', 1.0),
    llmTimeout: readNum(options?.llmTimeout, fmOpts2?.llmTimeout, 'llmTimeout', 120000),
    toolTimeout: readNum(options?.toolTimeout, fmOpts2?.toolTimeout, 'toolTimeout', 60000),
    maxRetries: readNum(options?.maxRetries, fmOpts2?.maxRetries, 'maxRetries', 3),
    maxToolTurns: readNum(options?.maxToolTurns, fmOpts2?.maxToolTurns, 'maxToolTurns', 10),
    toolResponseMaxBytes: readNum(options?.toolResponseMaxBytes, fmOpts2?.toolResponseMaxBytes, 'toolResponseMaxBytes', 12288),
    stream: readBool(options?.stream, fmOpts2?.stream, 'stream', false),
    parallelToolCalls: readBool(options?.parallelToolCalls, fmOpts2?.parallelToolCalls, 'parallelToolCalls', false),
    traceLLM: typeof options?.traceLLM === 'boolean' ? options.traceLLM : (fmTraceLLM2 ?? false),
    traceMCP: typeof options?.traceMCP === 'boolean' ? options.traceMCP : (fmTraceMCP2 ?? false),
    verbose: (options?.verbose === true) || (fmVerbose2 === true),
    mcpInitConcurrency: ((): number | undefined => {
      if (typeof options?.mcpInitConcurrency === 'number' && Number.isFinite(options.mcpInitConcurrency)) return Math.trunc(options.mcpInitConcurrency);
      const dv = config.defaults?.mcpInitConcurrency;
      if (typeof dv === 'number' && Number.isFinite(dv)) return Math.trunc(dv);
      return undefined;
    })()
  };

  const accountingFile: string | undefined = ((): string | undefined => {
    if (fmOpts2 !== undefined && typeof fmOpts2.accounting === 'string' && fmOpts2.accounting.length > 0) return fmOpts2.accounting;
    return config.accounting?.file;
  })();

  const loaded: LoadedAgent = {
    id,
    promptPath: id,
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
      traceLLM: eff.traceLLM,
      traceMCP: eff.traceMCP,
      verbose: eff.verbose,
      mcpInitConcurrency: eff.mcpInitConcurrency,
    },
    run: async (systemPrompt: string, userPrompt: string, opts?: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks }): Promise<AIAgentResult> => {
      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets: selectedTargets,
        tools: selectedTools,
        systemPrompt,
        userPrompt,
        conversationHistory: opts?.history,
        expectedOutput: fm?.expectedOutput,
        callbacks: opts?.callbacks,
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
