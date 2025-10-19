import type { FrontmatterOptions } from './frontmatter.js';
import type { Configuration, ReasoningLevel, CachingMode } from './types.js';

interface CLIOverrides {
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
  traceSlack?: boolean;
  verbose?: boolean;
  reasoning?: ReasoningLevel;
  caching?: CachingMode;
}

interface GlobalLLMOverrides {
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
  reasoning?: ReasoningLevel;
  caching?: CachingMode;
}

interface DefaultsForUndefined {
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
  parallelToolCalls?: boolean;
  reasoning?: ReasoningLevel;
  caching?: CachingMode;
}

interface ResolvedEffectiveOptions {
  temperature: number;
  topP: number;
  maxOutputTokens: number | undefined;
  repeatPenalty: number | undefined;
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
  traceSlack: boolean;
  verbose: boolean;
  mcpInitConcurrency?: number;
  reasoning?: ReasoningLevel;
  caching?: CachingMode;
}

export function resolveEffectiveOptions(args: {
  cli: CLIOverrides | undefined;
  global?: GlobalLLMOverrides;
  fm: FrontmatterOptions | undefined;
  configDefaults: NonNullable<Configuration['defaults']>;
  defaultsForUndefined?: DefaultsForUndefined;
}): ResolvedEffectiveOptions {
  const { cli, global, fm, configDefaults, defaultsForUndefined } = args;

  const getCliVal = (name: string): unknown => (cli !== undefined ? (cli as Record<string, unknown>)[name] : undefined);
  const getGlobalVal = (name: string): unknown => (global !== undefined ? (global as Record<string, unknown>)[name] : undefined);
  const getDefUndef = (name: string): unknown => (defaultsForUndefined !== undefined ? (defaultsForUndefined as Record<string, unknown>)[name] : undefined);
  const getCfgDefault = (name: string): unknown => (configDefaults as Record<string, unknown>)[name];

  const normalizeReasoning = (value: unknown): ReasoningLevel | undefined => {
    if (typeof value !== 'string') return undefined;
    const normalized = value.toLowerCase();
    if (normalized === 'minimal' || normalized === 'low' || normalized === 'medium' || normalized === 'high') {
      return normalized as ReasoningLevel;
    }
    return undefined;
  };

  const normalizeCaching = (value: unknown): CachingMode | undefined => {
    if (typeof value !== 'string') return undefined;
    const normalized = value.toLowerCase();
    if (normalized === 'none' || normalized === 'full') {
      return normalized as CachingMode;
    }
    return undefined;
  };

  const readReasoning = (): ReasoningLevel | undefined => {
    const fromGlobal = normalizeReasoning(getGlobalVal('reasoning'));
    if (fromGlobal !== undefined) return fromGlobal;
    const fromCli = normalizeReasoning(getCliVal('reasoning'));
    if (fromCli !== undefined) return fromCli;
    const fmVal = normalizeReasoning(fm?.reasoning);
    if (fmVal !== undefined) return fmVal;
    const def = normalizeReasoning(getDefUndef('reasoning'));
    if (def !== undefined) return def;
    const cfg = normalizeReasoning(getCfgDefault('reasoning'));
    if (cfg !== undefined) return cfg;
    return undefined;
  };

  const readCaching = (): CachingMode => {
    const fromGlobal = normalizeCaching(getGlobalVal('caching'));
    if (fromGlobal !== undefined) return fromGlobal;
    const fromCli = normalizeCaching(getCliVal('caching'));
    if (fromCli !== undefined) return fromCli;
    const fmVal = normalizeCaching(fm?.caching);
    if (fmVal !== undefined) return fmVal;
    const def = normalizeCaching(getDefUndef('caching'));
    if (def !== undefined) return def;
    const cfg = normalizeCaching(getCfgDefault('caching'));
    if (cfg !== undefined) return cfg;
    return 'full';
  };

  const readNum = (name: string, fmVal: unknown, fallback: number): number => {
    const globalVal = getGlobalVal(name);
    if (typeof globalVal === 'number' && Number.isFinite(globalVal)) return globalVal;
    const cliVal = getCliVal(name);
    if (typeof cliVal === 'number' && Number.isFinite(cliVal)) return cliVal;
    const n = Number(fmVal);
    if (Number.isFinite(n)) return n;
    const def = getDefUndef(name);
    if (typeof def === 'number' && Number.isFinite(def)) return def;
    const dv = getCfgDefault(name);
    if (typeof dv === 'number' && Number.isFinite(dv)) return dv;
    return fallback;
  };

  const readBool = (name: string, fmVal: unknown, fallback: boolean): boolean => {
    const globalVal = getGlobalVal(name);
    if (typeof globalVal === 'boolean') return globalVal;
    const cliVal = getCliVal(name);
    if (typeof cliVal === 'boolean') return cliVal;
    if (typeof fmVal === 'boolean') return fmVal;
    const def = getDefUndef(name);
    if (typeof def === 'boolean') return def;
    const dv = getCfgDefault(name);
    if (typeof dv === 'boolean') return dv;
    return fallback;
  };

  const out: ResolvedEffectiveOptions = {
    temperature: readNum('temperature', fm?.temperature, 0.7),
    topP: readNum('topP', fm?.topP, 1.0),
    maxOutputTokens: ((): number | undefined => {
      const v = readNum('maxOutputTokens', (fm as { maxOutputTokens?: number } | undefined)?.maxOutputTokens, 4096);
      return Number.isNaN(v) ? undefined : Math.trunc(v);
    })(),
    repeatPenalty: ((): number | undefined => {
      const v = readNum('repeatPenalty', (fm as { repeatPenalty?: number } | undefined)?.repeatPenalty, 1.1);
      return Number.isNaN(v) ? undefined : v;
    })(),
    llmTimeout: readNum('llmTimeout', fm?.llmTimeout, 120000),
    toolTimeout: readNum('toolTimeout', fm?.toolTimeout, 60000),
    maxRetries: readNum('maxRetries', fm?.maxRetries, 3),
    maxToolTurns: readNum('maxToolTurns', fm?.maxToolTurns, 10),
    maxToolCallsPerTurn: readNum('maxToolCallsPerTurn', (fm as { maxToolCallsPerTurn?: number } | undefined)?.maxToolCallsPerTurn, 10),
    toolResponseMaxBytes: readNum('toolResponseMaxBytes', fm?.toolResponseMaxBytes, 12288),
    stream: ((): boolean => {
      const globalStream = getGlobalVal('stream');
      if (typeof globalStream === 'boolean') return globalStream;
      if (typeof cli?.stream === 'boolean') return cli.stream;
      const dv = configDefaults.stream;
      if (typeof dv === 'boolean') return dv;
      return false;
    })(),
    parallelToolCalls: readBool('parallelToolCalls', fm?.parallelToolCalls, false),
    maxConcurrentTools: readNum('maxConcurrentTools', (fm as { maxConcurrentTools?: number } | undefined)?.maxConcurrentTools, 3),
    traceLLM: cli?.traceLLM === true,
    traceMCP: cli?.traceMCP === true,
    traceSlack: cli?.traceSlack === true,
    verbose: cli?.verbose === true,
    mcpInitConcurrency: ((): number | undefined => {
      const globalConcurrency = getGlobalVal('mcpInitConcurrency');
      if (typeof globalConcurrency === 'number' && Number.isFinite(globalConcurrency)) return Math.trunc(globalConcurrency);
      if (typeof cli?.mcpInitConcurrency === 'number' && Number.isFinite(cli.mcpInitConcurrency)) return Math.trunc(cli.mcpInitConcurrency);
      const dv = configDefaults.mcpInitConcurrency;
      if (typeof dv === 'number' && Number.isFinite(dv)) return Math.trunc(dv);
      return undefined;
    })(),
    reasoning: readReasoning(),
    caching: readCaching(),
  };
  return out;
}
