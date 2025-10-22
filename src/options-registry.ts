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
    default: 120000,
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
    default: 60000,
    description: 'How long to wait (ms) for each tool call to complete before aborting it; default 1 minute',
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
    key: 'maxConcurrentTools',
    default: 3,
    description: 'How many tools can run at the same time; limits parallel execution to avoid overwhelming your system',
    cli: { names: ['--max-concurrent-tools', '--maxConcurrentTools'], showInHelp: true },
    fm: { allowed: true, key: 'maxConcurrentTools' },
    config: { path: 'defaults.maxConcurrentTools' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 1, integer: true },
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
  boolDef({
    key: 'parallelToolCalls',
    description: 'Allow agent to call multiple tools at once; disable to run tools one at a time',
    default: false,
    cli: { names: ['--parallel-tool-calls', '--no-parallel-tool-calls'], showInHelp: true },
    fm: { allowed: true, key: 'parallelToolCalls' },
    config: { path: 'defaults.parallelToolCalls' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    flags: { allowNegation: true },
    render: { showInFrontmatterTemplate: true },
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
    description: 'Override settings for every agent/sub-agent (key=value). Supports models/tools/agents plus LLM knobs like temperature, topP, maxOutputTokens, llmTimeout, retries, stream, parallelToolCalls, reasoning, caching.',
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
