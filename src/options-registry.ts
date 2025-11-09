// Central options registry: single source of truth for option names, scopes, defaults, and rendering
// This module intentionally avoids importing other local modules to prevent cycles.

type OptionType = 'number' | 'boolean' | 'string' | 'string[]';
type OptionScope = 'masterOnly' | 'masterDefault' | 'allAgents' | 'global';

interface OptionDef {
  key: string; // internal camelCase key
  type: OptionType;
  default: number | boolean | string | string[] | undefined;
  description: string;
  cli?: { names: string[]; showInHelp?: boolean };
  fm?: { allowed: boolean; key?: string };
  config?: { path: string };
  scope: OptionScope;
  groups: string[]; // used for help grouping
  aliases?: string[];
  flags?: { allowNegation?: boolean };
  render?: { showInFrontmatterTemplate?: boolean };
  numeric?: { min?: number; max?: number; integer?: boolean };
}

// Helper to define boolean with common negation flag
function boolDef(init: Omit<OptionDef, 'type' | 'default'> & { default?: boolean }): OptionDef {
  return { ...init, type: 'boolean', default: init.default ?? false };
}

// Helper to define number
function numDef(init: Omit<OptionDef, 'type'> & { default: number }): OptionDef {
  return { ...init, type: 'number' };
}

// Helper to define string
function strDef(init: Omit<OptionDef, 'type'> & { default?: string }): OptionDef {
  return { ...init, type: 'string' };
}

// Helper to define string array (frontmatter/cli logical)
function strArrDef(init: Omit<OptionDef, 'type'> & { default?: string[] }): OptionDef {
  return { ...init, type: 'string[]' };
}

// Group name constants to dedupe strings
const G_MASTER_OVERRIDES = 'Master Agent Overrides';
const G_MASTER_DEFAULTS = 'Master Defaults';
const G_ALL_MODELS = 'All Models Overrides';
const G_GLOBAL = 'Global Controls';

export const OPTIONS_REGISTRY: OptionDef[] = [
  // Master Agent Overrides (strict)
  strArrDef({
    key: 'models',
    default: [],
    description: 'Which LLM models to use for the master agent (e.g., "openai/gpt-4o,anthropic/claude-3.5-sonnet"); tries each in order if one fails',
    cli: { names: ['--models'], showInHelp: true },
    fm: { allowed: true, key: 'models' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),
  strArrDef({
    key: 'tools',
    default: [],
    description: 'Which tools the master agent can use (MCP servers, sub-agents, etc.); comma-separated list',
    cli: { names: ['--tools', '--tool', '--mcp', '--mcp-tool', '--mcp-tools'], showInHelp: true },
    fm: { allowed: true, key: 'tools' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),
  strArrDef({
    key: 'agents',
    default: [],
    description: 'Sub-agent .ai files to load as tools for the master agent; enables multi-agent composition',
    cli: { names: ['--agents'], showInHelp: true },
    fm: { allowed: true, key: 'agents' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),

  // Master Defaults
  numDef({
    key: 'llmTimeout',
    default: 600000,
    description: 'How long to wait (ms) for the LLM to respond before giving up (resets each time a token arrives); default 2 minutes',
    cli: { names: ['--llm-timeout-ms', '--llmTimeoutMs'], showInHelp: true },
    fm: { allowed: true, key: 'llmTimeout' },
    config: { path: 'defaults.llmTimeout' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, integer: true },
  }),
  numDef({
    key: 'toolTimeout',
    default: 300000,
    description: 'How long to wait (ms) for each tool call to complete before aborting it; default 5 minutes',
    cli: { names: ['--tool-timeout-ms', '--toolTimeoutMs'], showInHelp: true },
    fm: { allowed: true, key: 'toolTimeout' },
    config: { path: 'defaults.toolTimeout' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, integer: true },
  }),
  numDef({
    key: 'toolResponseMaxBytes',
    default: 12288,
    description: 'Maximum size of tool output to keep; longer outputs get truncated to avoid overwhelming the LLM context',
    cli: { names: ['--tool-response-max-bytes', '--toolResponseMaxBytes'], showInHelp: true },
    fm: { allowed: true, key: 'toolResponseMaxBytes' },
    config: { path: 'defaults.toolResponseMaxBytes' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, integer: true },
  }),
  numDef({
    key: 'temperature',
    default: 0.7,
    description: 'Response creativity/variance (0=focused, 1=balanced, 2=wild); higher values produce more unexpected outputs',
    cli: { names: ['--temperature'], showInHelp: true },
    fm: { allowed: true, key: 'temperature' },
    config: { path: 'defaults.temperature' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, max: 2 },
  }),
  numDef({
    key: 'topP',
    default: 1.0,
    description: 'Token selection diversity (0.0-1.0); lower values use only top choices, higher values consider more alternatives',
    cli: { names: ['--top-p', '--topP'], showInHelp: true },
    fm: { allowed: true, key: 'topP' },
    config: { path: 'defaults.topP' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, max: 1 },
  }),
  strDef({
    key: 'reasoning',
    description: 'Reasoning effort level: minimal, low, medium, or high; leave unset to use provider defaults',
    cli: { names: ['--reasoning'], showInHelp: true },
    fm: { allowed: true, key: 'reasoning' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    render: { showInFrontmatterTemplate: true },
    default: undefined,
  }),
  strDef({
    key: 'reasoningTokens',
    description: 'Reasoning token budget for Anthropics thinking mode (0 disables).',
    cli: { names: ['--reasoning-tokens'], showInHelp: true },
    fm: { allowed: true, key: 'reasoningTokens' },
    config: { path: 'defaults.reasoningValue' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    default: undefined,
  }),
  numDef({
    key: 'maxOutputTokens',
    default: 4096,
    description: 'Maximum response length per turn; controls how long the agent\'s answers can be',
    cli: { names: ['--max-output-tokens'], showInHelp: true },
    fm: { allowed: true, key: 'maxOutputTokens' },
    config: { path: 'defaults.maxOutputTokens' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 1, integer: true },
  }),
  numDef({
    key: 'repeatPenalty',
    default: 1.1,
    description: 'Reduces repetitive text (1.0=off, higher=stronger); helps agent avoid repeating the same phrases',
    cli: { names: ['--repeat-penalty'], showInHelp: true },
    fm: { allowed: true, key: 'repeatPenalty' },
    config: { path: 'defaults.repeatPenalty' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0 },
  }),
  numDef({
    key: 'maxRetries',
    default: 3,
    description: 'How many times to retry when LLM calls fail; goes through all fallback models before giving up',
    cli: { names: ['--max-retries'], showInHelp: true },
    fm: { allowed: true, key: 'maxRetries' },
    config: { path: 'defaults.maxRetries' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, integer: true },
  }),
  numDef({
    key: 'maxToolTurns',
    default: 10,
    description: 'Maximum number of tool-using turns before forcing the agent to give a final answer; prevents infinite loops',
    cli: { names: ['--max-tool-turns'], showInHelp: true },
    fm: { allowed: true, key: 'maxToolTurns' },
    config: { path: 'defaults.maxToolTurns' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 1, integer: true },
  }),
  numDef({
    key: 'maxToolCallsPerTurn',
    default: 10,
    description: 'Maximum number of tools the agent can call in a single turn; caps parallel tool usage',
    cli: { names: ['--max-tool-calls-per-turn'], showInHelp: true },
    fm: { allowed: true, key: 'maxToolCallsPerTurn' },
    config: { path: 'defaults.maxToolCallsPerTurn' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 1, integer: true },
  }),
  strDef({
    key: 'caching',
    description: 'Anthropic caching mode: full (default behaviour) or none to disable cache reuse',
    cli: { names: ['--caching'], showInHelp: true },
    fm: { allowed: true, key: 'caching' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    render: { showInFrontmatterTemplate: true },
    default: 'full',
  }),

  // All Models Overrides
  strArrDef({
    key: 'override',
    default: [],
    description: 'Override settings for every agent/sub-agent (key=value). Supports models/tools/agents plus LLM knobs like temperature, topP, maxOutputTokens, llmTimeout, retries, stream, reasoning, caching.',
    cli: { names: ['--override'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'stream',
    description: 'Show response as it\'s generated (streaming) instead of waiting for complete answer',
    default: false,
    cli: { names: ['--stream', '--no-stream'], showInHelp: true },
    fm: { allowed: false },
    config: { path: 'defaults.stream' },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
    flags: { allowNegation: true },
  }),
  boolDef({
    key: 'traceLLM',
    description: 'Show detailed logs of all LLM API calls for debugging (verbose)',
    default: false,
    cli: { names: ['--trace-llm'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'traceMCP',
    description: 'Show detailed logs of all tool calls (MCP protocol) for debugging (verbose)',
    default: false,
    cli: { names: ['--trace-mcp'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'traceSlack',
    description: 'Show detailed logs of Slack bot communication for debugging (verbose)',
    default: false,
    cli: { names: ['--trace-slack'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'traceSdk',
    description: 'Dump raw UI/LLM payloads exchanged with the AI SDK for debugging',
    default: false,
    cli: { names: ['--trace-sdk'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'verbose',
    description: 'Show detailed execution logs including timing, tokens used, and internal state',
    default: false,
    cli: { names: ['--verbose'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  strDef({
    key: 'format',
    default: '',
    description: 'Output format (markdown, json, slack-block-kit, etc.); controls how the agent formats its response',
    cli: { names: ['--format'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),

  // Global Application Controls
  boolDef({
    key: 'dryRun',
    description: 'Check configuration and setup without actually running the agent; useful for testing config',
    default: false,
    cli: { names: ['--dry-run'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  numDef({
    key: 'mcpInitConcurrency',
    default: Number.NaN, // resolved elsewhere if provided
    description: 'How many MCP tool servers to initialize in parallel; lower values reduce system load during startup',
    cli: { names: ['--mcp-init-concurrency'], showInHelp: true },
    fm: { allowed: false },
    config: { path: 'defaults.mcpInitConcurrency' },
    scope: 'global',
    groups: [G_GLOBAL],
    numeric: { min: 1, integer: true },
  }),
  boolDef({
    key: 'quiet',
    description: 'Suppress all log output except critical errors; makes output cleaner',
    default: false,
    cli: { names: ['--quiet'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'config',
    default: undefined,
    description: 'Path to configuration file; overrides all auto-discovered config files',
    cli: { names: ['--config'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'sessionsDir',
    default: undefined,
    description: 'Where to save session data for resuming interrupted conversations',
    cli: { names: ['--sessions-dir'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'billingFile',
    default: undefined,
    description: 'Where to log token usage and costs; useful for tracking LLM API expenses',
    cli: { names: ['--billing-file'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'resume',
    default: undefined,
    description: 'Resume a previously interrupted session by providing its session ID',
    cli: { names: ['--resume'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  boolDef({
    key: 'telemetryEnabled',
    default: false,
    description: 'Enable telemetry (OTLP metrics / Prometheus). Telemetry is opt-in.',
    cli: { names: ['--telemetry-enabled'], showInHelp: true },
    config: { path: 'telemetry.enabled' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryOtlpEndpoint',
    default: undefined,
    description: 'OTLP gRPC endpoint for telemetry exports (e.g., grpc://localhost:4317)',
    cli: { names: ['--telemetry-otlp-endpoint'], showInHelp: true },
    config: { path: 'telemetry.otlp.endpoint' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryOtlpTimeoutMs',
    default: undefined,
    description: 'OTLP export timeout in milliseconds',
    cli: { names: ['--telemetry-otlp-timeout-ms'], showInHelp: true },
    config: { path: 'telemetry.otlp.timeoutMs' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  boolDef({
    key: 'telemetryPrometheusEnabled',
    default: false,
    description: 'Expose Prometheus /metrics endpoint for telemetry (requires telemetry enabled).',
    cli: { names: ['--telemetry-prometheus-enabled'], showInHelp: true },
    config: { path: 'telemetry.prometheus.enabled' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryPrometheusHost',
    default: undefined,
    description: 'Host interface for Prometheus /metrics endpoint',
    cli: { names: ['--telemetry-prometheus-host'], showInHelp: true },
    config: { path: 'telemetry.prometheus.host' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryPrometheusPort',
    default: undefined,
    description: 'Port for Prometheus /metrics endpoint',
    cli: { names: ['--telemetry-prometheus-port'], showInHelp: true },
    config: { path: 'telemetry.prometheus.port' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  boolDef({
    key: 'telemetryTracesEnabled',
    default: false,
    description: 'Enable OTLP tracing (requires telemetry enabled).',
    cli: { names: ['--telemetry-traces-enabled'], showInHelp: true },
    config: { path: 'telemetry.traces.enabled' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryTraceSampler',
    default: undefined,
    description: 'Tracing sampler (always_on, always_off, parent, ratio).',
    cli: { names: ['--telemetry-trace-sampler'], showInHelp: true },
    config: { path: 'telemetry.traces.sampler' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryTraceRatio',
    default: undefined,
    description: 'Sampling ratio (0-1) when sampler=ratio.',
    cli: { names: ['--telemetry-trace-ratio'], showInHelp: true },
    config: { path: 'telemetry.traces.ratio' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strArrDef({
    key: 'telemetryLabels',
    default: [],
    description: 'Additional telemetry labels (key=value). Specify multiple times for multiple labels.',
    cli: { names: ['--telemetry-label'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strArrDef({
    key: 'telemetryLogFormat',
    default: [],
    description: 'Preferred logging format(s): journald, logfmt, json, none (first valid entry wins).',
    cli: { names: ['--telemetry-log-format'], showInHelp: true },
    config: { path: 'telemetry.logging.formats' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strArrDef({
    key: 'telemetryLogExtra',
    default: [],
    description: 'Additional log sinks (e.g., otlp). Specify multiple times for multiple sinks.',
    cli: { names: ['--telemetry-log-extra'], showInHelp: true },
    config: { path: 'telemetry.logging.extra' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryLoggingOtlpEndpoint',
    default: undefined,
    description: 'Override OTLP endpoint for log exports (defaults to telemetry OTLP endpoint).',
    cli: { names: ['--telemetry-logging-otlp-endpoint'], showInHelp: true },
    config: { path: 'telemetry.logging.otlp.endpoint' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'telemetryLoggingOtlpTimeoutMs',
    default: undefined,
    description: 'Override OTLP timeout (ms) for log exports.',
    cli: { names: ['--telemetry-logging-otlp-timeout-ms'], showInHelp: true },
    config: { path: 'telemetry.logging.otlp.timeoutMs' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
];

export function getFrontmatterAllowedKeys(): string[] {
  return OPTIONS_REGISTRY.filter((o) => o.fm?.allowed === true).map((o) => o.fm?.key ?? o.key);
}

export function getOptionsByGroup(): Map<string, OptionDef[]> {
  return OPTIONS_REGISTRY.reduce((acc, opt) => {
    opt.groups.forEach((g) => {
      const list = acc.get(g) ?? [];
      acc.set(g, [...list, opt]);
    });
    return acc;
  }, new Map<string, OptionDef[]>());
}

export function formatCliNames(opt: OptionDef): string {
  const names = opt.cli?.names ?? [];
  return names.join(', ');
}
