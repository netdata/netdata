import type { FrontmatterOptions } from './frontmatter.js';
import type { Configuration, ReasoningLevel, ProviderReasoningValue, CachingMode } from './types.js';

interface CLIOverrides {
  stream?: boolean;
  maxRetries?: number;
  maxTurns?: number;
  maxToolCallsPerTurn?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number | null;
  topP?: number | null;
  topK?: number | null;
  maxOutputTokens?: number;
  repeatPenalty?: number | null;
  toolResponseMaxBytes?: number;
  mcpInitConcurrency?: number;
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSlack?: boolean;
  traceSdk?: boolean;
  verbose?: boolean;
  reasoning?: ReasoningLevel | 'none';
  reasoningValue?: ProviderReasoningValue | null;
  caching?: CachingMode;
}

interface GlobalLLMOverrides {
  stream?: boolean;
  maxRetries?: number;
  maxTurns?: number;
  maxToolCallsPerTurn?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number | null;
  topP?: number | null;
  topK?: number | null;
  maxOutputTokens?: number;
  repeatPenalty?: number | null;
  toolResponseMaxBytes?: number;
  mcpInitConcurrency?: number;
  reasoning?: ReasoningLevel | 'none' | 'inherit';
  reasoningValue?: ProviderReasoningValue | null;
  caching?: CachingMode;
}

interface DefaultsForUndefined {
  temperature?: number | null;
  topP?: number | null;
  topK?: number | null;
  maxOutputTokens?: number;
  repeatPenalty?: number | null;
  llmTimeout?: number;
  toolTimeout?: number;
  maxRetries?: number;
  maxTurns?: number;
  maxToolCallsPerTurn?: number;
  toolResponseMaxBytes?: number;
  reasoning?: ReasoningLevel | 'none';
  reasoningValue?: ProviderReasoningValue | null;
  caching?: CachingMode;
}

interface ResolvedEffectiveOptions {
  temperature: number | null;
  topP: number | null;
  topK: number | null;
  maxOutputTokens: number | undefined;
  repeatPenalty: number | null;
  llmTimeout: number;
  toolTimeout: number;
  maxRetries: number;
  maxTurns: number;
  maxToolCallsPerTurn: number;
  toolResponseMaxBytes: number;
  stream: boolean;
  traceLLM: boolean;
  traceMCP: boolean;
  traceSlack: boolean;
  traceSdk: boolean;
  verbose: boolean;
  mcpInitConcurrency?: number;
  reasoning?: ReasoningLevel;
  reasoningValue?: ProviderReasoningValue | null;
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

  const normalizeReasoningDirective = (value: unknown): ReasoningLevel | 'none' | 'inherit' | undefined => {
    if (value === undefined || value === null || typeof value !== 'string') return undefined;
    const normalized = value.trim().toLowerCase();
    if (normalized.length === 0) return undefined;
    if (normalized === 'none') return 'none';
    if (normalized === 'default' || normalized === 'unset' || normalized === 'inherit') return 'inherit';
    if (normalized === 'minimal' || normalized === 'low' || normalized === 'medium' || normalized === 'high') {
      return normalized as ReasoningLevel;
    }
    return undefined;
  };
  let reasoningDisabled = false;

  const normalizeCaching = (value: unknown): CachingMode | undefined => {
    if (typeof value !== 'string') return undefined;
    const normalized = value.toLowerCase();
    if (normalized === 'none' || normalized === 'full') {
      return normalized as CachingMode;
    }
    return undefined;
  };

  const normalizeReasoningValue = (value: unknown): ProviderReasoningValue | null | undefined => {
    if (value === null) return null;
    if (typeof value === 'number' && Number.isFinite(value)) {
      if (value <= 0) return null;
      return Math.trunc(value);
    }
    if (typeof value === 'string') {
      const trimmed = value.trim().toLowerCase();
      if (trimmed.length === 0) return undefined;
      if (trimmed === 'disabled' || trimmed === 'off' || trimmed === 'none') return null;
      const numeric = Number(trimmed);
      if (Number.isFinite(numeric)) {
        if (numeric <= 0) return null;
        return Math.trunc(numeric);
      }
    }
    return undefined;
  };

  const readReasoning = (): ReasoningLevel | undefined => {
    const sources: unknown[] = [
      getGlobalVal('reasoning'),
      getCliVal('reasoning'),
      fm?.reasoning,
      getDefUndef('reasoning'),
      getCfgDefault('reasoning'),
    ];
    // eslint-disable-next-line functional/no-loop-statements
    for (const value of sources) {
      const directive = normalizeReasoningDirective(value);
      if (directive === undefined || directive === 'inherit') continue;
      if (directive === 'none') {
        reasoningDisabled = true;
        return undefined;
      }
      return directive;
    }
    return undefined;
  };

  const readReasoningValue = (): ProviderReasoningValue | null | undefined => {
    const fromGlobal = normalizeReasoningValue(getGlobalVal('reasoningValue'));
    if (fromGlobal !== undefined) return fromGlobal;
    const fromGlobalTokens = normalizeReasoningValue(getGlobalVal('reasoningTokens'));
    if (fromGlobalTokens !== undefined) return fromGlobalTokens;
    const fromCli = normalizeReasoningValue(getCliVal('reasoningValue'));
    if (fromCli !== undefined) return fromCli;
    const fromCliTokens = normalizeReasoningValue(getCliVal('reasoningTokens'));
    if (fromCliTokens !== undefined) return fromCliTokens;
    const fmTokens = normalizeReasoningValue(fm?.reasoningTokens);
    if (fmTokens !== undefined) return fmTokens;
    const def = normalizeReasoningValue(getDefUndef('reasoningValue'));
    if (def !== undefined) return def;
    const cfg = normalizeReasoningValue(getCfgDefault('reasoningValue'));
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

  // Read a nullable numeric parameter (supports 'null' meaning "don't send to model")
  // Priority: global > cli > fm > defaultsForUndefined > configDefaults > fallback
  // Returns: number (use this value), null (don't send), or the fallback
  const readNullableNum = (name: string, fmVal: unknown, fallback: number | null): number | null => {
    // Check each source in priority order
    const sources: unknown[] = [
      getGlobalVal(name),
      getCliVal(name),
      fmVal,
      getDefUndef(name),
      getCfgDefault(name),
    ];
    // eslint-disable-next-line functional/no-loop-statements
    for (const val of sources) {
      // Explicit null means "don't send"
      if (val === null) return null;
      // Finite number means "use this value"
      if (typeof val === 'number' && Number.isFinite(val)) return val;
      // Skip undefined/NaN and continue to next source
    }
    // No source provided a value, use fallback
    return fallback;
  };

  const reasoning = readReasoning();
  let reasoningValueResult: ProviderReasoningValue | null | undefined;
  // Reasoning may be disabled by normalizeReasoningDirective; the analyzer cannot track this across helper calls.
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  if (reasoningDisabled) {
    reasoningValueResult = null;
  } else {
    reasoningValueResult = readReasoningValue();
  }

  const out: ResolvedEffectiveOptions = {
    // Use readNullableNum for parameters that support "null" = don't send
    // New defaults: temperature=0.0, topP/topK/repeatPenalty=null (don't send)
    temperature: readNullableNum('temperature', fm?.temperature, 0.0),
    topP: readNullableNum('topP', fm?.topP, null),
    topK: ((): number | null => {
      const v = readNullableNum('topK', fm?.topK, null);
      return v !== null ? Math.trunc(v) : null;
    })(),
    maxOutputTokens: ((): number | undefined => {
      const v = readNum('maxOutputTokens', (fm as { maxOutputTokens?: number } | undefined)?.maxOutputTokens, 16384);
      return Number.isNaN(v) ? undefined : Math.trunc(v);
    })(),
    repeatPenalty: readNullableNum('repeatPenalty', (fm as { repeatPenalty?: number | null } | undefined)?.repeatPenalty, null),
    llmTimeout: readNum('llmTimeout', fm?.llmTimeout, 600000),
    toolTimeout: readNum('toolTimeout', fm?.toolTimeout, 300000),
    maxRetries: readNum('maxRetries', fm?.maxRetries, 3),
    maxTurns: readNum('maxTurns', fm?.maxTurns, 10),
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
    traceLLM: cli?.traceLLM === true,
    traceMCP: cli?.traceMCP === true,
    traceSlack: cli?.traceSlack === true,
    traceSdk: cli?.traceSdk === true,
    verbose: cli?.verbose === true,
    mcpInitConcurrency: ((): number | undefined => {
      const globalConcurrency = getGlobalVal('mcpInitConcurrency');
      if (typeof globalConcurrency === 'number' && Number.isFinite(globalConcurrency)) return Math.trunc(globalConcurrency);
      if (typeof cli?.mcpInitConcurrency === 'number' && Number.isFinite(cli.mcpInitConcurrency)) return Math.trunc(cli.mcpInitConcurrency);
      const dv = configDefaults.mcpInitConcurrency;
      if (typeof dv === 'number' && Number.isFinite(dv)) return Math.trunc(dv);
      return undefined;
    })(),
    reasoning,
    reasoningValue: reasoningValueResult,
    caching: readCaching(),
  };
  return out;
}
