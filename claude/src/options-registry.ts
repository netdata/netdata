// Central options registry: single source of truth for option names, scopes, defaults, and rendering
// This module intentionally avoids importing other local modules to prevent cycles.

export type OptionType = 'number' | 'boolean' | 'string' | 'string[]';
export type OptionScope = 'masterOnly' | 'masterDefault' | 'allAgents' | 'global';

export interface OptionDef {
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
    description: 'Provider/model targets',
    cli: { names: ['--models'], showInHelp: true },
    fm: { allowed: true, key: 'models' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),
  strArrDef({
    key: 'tools',
    default: [],
    description: 'MCP tools list',
    cli: { names: ['--tools', '--tool', '--mcp', '--mcp-tool', '--mcp-tools'], showInHelp: true },
    fm: { allowed: true, key: 'tools' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),
  strArrDef({
    key: 'agents',
    default: [],
    description: 'Sub-agent .ai files',
    cli: { names: ['--agents'], showInHelp: true },
    fm: { allowed: true, key: 'agents' },
    scope: 'masterOnly',
    groups: [G_MASTER_OVERRIDES],
  }),

  // Master Defaults
  numDef({
    key: 'llmTimeout',
    default: 120000,
    description: 'Timeout for LLM responses (ms)',
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
    description: 'Timeout for tool execution (ms)',
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
    description: 'Maximum MCP tool response size (bytes)',
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
    description: 'Maximum concurrent tool executions per agent/session',
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
    description: 'LLM temperature (0.0-2.0)',
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
    description: 'LLM top-p sampling (0.0-1.0)',
    cli: { names: ['--top-p', '--topP'], showInHelp: true },
    fm: { allowed: true, key: 'topP' },
    config: { path: 'defaults.topP' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 0, max: 1 },
  }),
  numDef({
    key: 'maxOutputTokens',
    default: 4096,
    description: 'Max output tokens (model-specific mapping)',
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
    description: 'Repeat penalty (mapped per provider)',
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
    description: 'Max retry rounds over provider/model list',
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
    description: 'Maximum tool turns (agent loop cap)',
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
    description: 'Maximum tool calls allowed within a single LLM turn',
    cli: { names: ['--max-tool-calls-per-turn'], showInHelp: true },
    fm: { allowed: true, key: 'maxToolCallsPerTurn' },
    config: { path: 'defaults.maxToolCallsPerTurn' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    numeric: { min: 1, integer: true },
  }),
  boolDef({
    key: 'parallelToolCalls',
    description: 'Allow LLM to plan multiple tool calls per turn',
    default: false,
    cli: { names: ['--parallel-tool-calls', '--no-parallel-tool-calls'], showInHelp: true },
    fm: { allowed: true, key: 'parallelToolCalls' },
    config: { path: 'defaults.parallelToolCalls' },
    scope: 'masterDefault',
    groups: [G_MASTER_DEFAULTS],
    flags: { allowNegation: true },
    render: { showInFrontmatterTemplate: true },
  }),

  // All Models Overrides
  boolDef({
    key: 'stream',
    description: 'Enable streaming LLM responses',
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
    description: 'Log LLM HTTP requests/responses (redacted)',
    default: false,
    cli: { names: ['--trace-llm'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'traceMCP',
    description: 'Log MCP requests/responses and server stderr',
    default: false,
    cli: { names: ['--trace-mcp'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  boolDef({
    key: 'verbose',
    description: 'Enable debug logging to stderr',
    default: false,
    cli: { names: ['--verbose'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),
  strDef({
    key: 'format',
    default: '',
    description: 'Output format hint (markdown, markdown+mermaid, slack, tty, pipe, json, sub-agent)',
    cli: { names: ['--format'], showInHelp: true },
    fm: { allowed: false },
    scope: 'allAgents',
    groups: [G_ALL_MODELS],
  }),

  // Global Application Controls
  boolDef({
    key: 'dryRun',
    description: 'Validate config and MCP only, no LLM requests',
    default: false,
    cli: { names: ['--dry-run'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  numDef({
    key: 'mcpInitConcurrency',
    default: Number.NaN, // resolved elsewhere if provided
    description: 'Max concurrent MCP server initializations',
    cli: { names: ['--mcp-init-concurrency'], showInHelp: true },
    fm: { allowed: false },
    config: { path: 'defaults.mcpInitConcurrency' },
    scope: 'global',
    groups: [G_GLOBAL],
    numeric: { min: 1, integer: true },
  }),
  boolDef({
    key: 'quiet',
    description: 'Only print errors to stderr',
    default: false,
    cli: { names: ['--quiet'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'config',
    default: undefined,
    description: 'Configuration file path',
    cli: { names: ['--config'], showInHelp: true },
    fm: { allowed: false },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'accounting',
    default: undefined,
    description: 'Accounting file path',
    cli: { names: ['--accounting'], showInHelp: true },
    fm: { allowed: false },
    config: { path: 'accounting.file' },
    scope: 'global',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'save',
    default: undefined,
    description: 'Save conversation to JSON file (master only)',
    cli: { names: ['--save'], showInHelp: true },
    fm: { allowed: false },
    scope: 'masterOnly',
    groups: [G_GLOBAL],
  }),
  strDef({
    key: 'load',
    default: undefined,
    description: 'Load conversation from JSON file (master only)',
    cli: { names: ['--load'], showInHelp: true },
    fm: { allowed: false },
    scope: 'masterOnly',
    groups: ['Global Controls'],
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
