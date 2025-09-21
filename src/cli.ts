import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { Command, Option } from 'commander';
import * as yaml from 'js-yaml';

// Keep import order: builtins, external, type, internal
// (moved below to maintain import order)

import type { LoadAgentOptions } from './agent-loader.js';
import type { FrontmatterOptions } from './frontmatter.js';
import type { McpTransportSpec } from './headends/mcp-headend.js';
import type { Headend, HeadendLogSink } from './headends/types.js';
import type { LogEntry, AccountingEntry, AIAgentCallbacks, ConversationMessage, Configuration } from './types.js';
import type { CommanderError } from 'commander';

import { loadAgentFromContent } from './agent-loader.js';
import { AgentRegistry } from './agent-registry.js';
import { discoverLayers, resolveDefaults } from './config-resolver.js';
import { describeFormat, resolveFormatIdForCli } from './formats.js';
import { parseFrontmatter, stripFrontmatter, parseList, parsePairs, buildFrontmatterTemplate } from './frontmatter.js';
import { AnthropicCompletionsHeadend } from './headends/anthropic-completions-headend.js';
import { HeadendManager } from './headends/headend-manager.js';
import { McpHeadend } from './headends/mcp-headend.js';
import { OpenAICompletionsHeadend } from './headends/openai-completions-headend.js';
import { OpenAIToolHeadend } from './headends/openai-tool-headend.js';
import { RestHeadend } from './headends/rest-headend.js';
import { resolveIncludes } from './include-resolver.js';
import { formatLog } from './log-formatter.js';
import { makeTTYLogCallbacks } from './log-sink-tty.js';
import { getOptionsByGroup, formatCliNames, OPTIONS_REGISTRY } from './options-registry.js';
import { formatAgentResultHumanReadable } from './utils.js';

// FrontmatterOptions is sourced from frontmatter.ts (single definition)

// Centralized exit path to guarantee a single, reasoned exit
let hasExited = false;
function exitWith(code: number, reason: string, tag = 'EXIT-CLI'): never {
  try {
    // Always print a final, standardized fatal/summary line
    const msg = `[VRB] ← [0.0] agent ${tag}: ${reason} (fatal=true)`;
    // Use stderr for control lines
    process.stderr.write(`${msg}\n`);
  } catch { /* ignore */ }
  // Ensure we exit only once
  if (!hasExited) {
    hasExited = true;
    // eslint-disable-next-line n/no-process-exit
    process.exit(code);
  }
  // TypeScript: inform this never returns
  // eslint-disable-next-line @typescript-eslint/no-unsafe-return
  throw new Error('unreachable');
}

const program = new Command();
// Force commander to route exits through our single exit path
program.exitOverride((err: CommanderError) => {
  const code = err.exitCode;
  const msg = err.message;
  exitWith(code, `commander: ${msg}`, 'EXIT-COMMANDER');
});
function addOptionsFromRegistry(prog: Command): void {
  // Dynamically add options from the registry to avoid duplication
  // For booleans with negation, add each name separately; for others, combine names.
  const placeholderFor = (key: string, type: string): string => {
    if (type === 'boolean') return '';
    if (key.endsWith('Timeout')) return ' <ms>';
    if (key.endsWith('Bytes')) return ' <n>';
    if (key === 'maxRetries' || key === 'maxToolTurns' || key === 'maxConcurrentTools' || key === 'topP' || key === 'temperature') return ' <n>';
    if (key === 'models' || key === 'tools' || key === 'agents') return ' <list>';
    if (key === 'config' || key === 'accounting' || key === 'save' || key === 'load') return ' <filename>';
    return ' <value>';
  };
  OPTIONS_REGISTRY.forEach((def) => {
    const names = def.cli?.names ?? [];
    if (names.length === 0) return;
    if (def.type === 'boolean') {
      // Add each flag separately (handles --no-*)
      names.forEach((n) => {
        prog.option(n, def.description);
      });
    } else {
      const first = names[0] + placeholderFor(def.key, def.type);
      const rest = names.slice(1);
      const combined = [first, ...rest].join(', ');
      prog.addOption(new Option(combined, def.description));
    }
  });
}

const appendValue = <T>(value: T, previous?: T[]): T[] => {
  const base = Array.isArray(previous) ? [...previous] : [];
  base.push(value);
  return base;
};

const parsePort = (value: string): number => {
  const port = Number.parseInt(value, 10);
  if (!Number.isFinite(port) || port <= 0 || port > 65535) {
    throw new Error(`invalid port '${value}'`);
  }
  return port;
};

const parsePositive = (value: string): number => {
  const num = Number.parseInt(value, 10);
  if (!Number.isFinite(num) || num <= 0) {
    throw new Error(`invalid positive number '${value}'`);
  }
  return num;
};

// Build a Resolved Defaults section using frontmatter + config (if available)
function buildResolvedDefaultsHelp(): string {
  try {
    const args = process.argv.slice(2);
    // Find system prompt file: first non-option, or fallback to argv[1] (shebang use)
    let candidate: string | undefined = args.find((a) => (typeof a === 'string' && a.length > 0 && !a.startsWith('-')));
    if (candidate === undefined && typeof process.argv[1] === 'string') {
      candidate = process.argv[1];
    }
    let promptPath: string | undefined = undefined;
    if (typeof candidate === 'string' && candidate.length > 0) {
      const p = candidate.startsWith('@') ? candidate.slice(1) : candidate;
      if (p.length > 0 && fs.existsSync(p) && fs.statSync(p).isFile()) promptPath = p;
    }

    // Read frontmatter from promptPath (if present)
    let fmOptions: FrontmatterOptions | undefined = undefined;
    let fmDesc = '';
    let fmDescOnly = '';
    let fmUsage = '';
    if (promptPath !== undefined) {
      let content = fs.readFileSync(promptPath, 'utf8');
      if (content.startsWith('#!')) {
        const nl = content.indexOf('\n');
        content = nl >= 0 ? content.slice(nl + 1) : '';
      }
      const fm = parseFrontmatter(content);
      if (fm?.options !== undefined) fmOptions = fm.options;
      if (typeof fm?.description === 'string') { fmDesc = fm.description.trim(); fmDescOnly = fmDesc; }
      const fmUse = fm?.usage;
      if (typeof fmUse === 'string' && fmUse.trim().length > 0) {
        fmUsage = fmUse.trim();
        const inv = typeof candidate === 'string' && candidate.length > 0 ? candidate : 'ai-agent';
        // Add usage headline before defaults
        const usageLine = `Usage: ${inv} "${fmUsage}"`;
        // Temporarily stash in description prefix for printing
        fmDesc = (fmDesc.length > 0 ? `${fmDesc}\n` : '') + usageLine;
      }
    }

    // Load configuration defaults (best effort) via layered resolver
    let configDefaults: NonNullable<Configuration['defaults']> = {};
    try {
      const layers = discoverLayers({ configPath: undefined, promptPath });
      configDefaults = resolveDefaults(layers);
    } catch {
      // ignore
    }

    // Resolve display values (FM > config.defaults > internal)
    const readNum = (key: keyof FrontmatterOptions, ckey: string, fallback: number): number => {
      const fmv = fmOptions !== undefined ? (fmOptions[key] as unknown) : undefined;
      if (typeof fmv === 'number' && Number.isFinite(fmv)) return fmv;
      const cv = (configDefaults as Record<string, unknown>)[ckey];
      if (typeof cv === 'number' && Number.isFinite(cv)) return cv;
      return fallback;
    };
    const readBool = (key: keyof FrontmatterOptions, ckey: string, fallback: boolean): boolean => {
      const fmv = fmOptions !== undefined ? (fmOptions[key] as unknown) : undefined;
      if (typeof fmv === 'boolean') return fmv;
      const cv = (configDefaults as Record<string, unknown>)[ckey];
      if (typeof cv === 'boolean') return cv;
      return fallback;
    };

    const defaultOf = (key: string): number | boolean => {
      const def = OPTIONS_REGISTRY.find((o) => o.key === key)?.default;
      return (typeof def === 'number' || typeof def === 'boolean') ? def : 0;
    };
    const temperature = readNum('temperature', 'temperature', defaultOf('temperature') as number);
    const topP = readNum('topP', 'topP', defaultOf('topP') as number);
    const llmTimeout = readNum('llmTimeout', 'llmTimeout', defaultOf('llmTimeout') as number);
    const toolTimeout = readNum('toolTimeout', 'toolTimeout', defaultOf('toolTimeout') as number);
    const maxRetries = readNum('maxRetries', 'maxRetries', defaultOf('maxRetries') as number);
    const maxToolTurns = readNum('maxToolTurns', 'maxToolTurns', defaultOf('maxToolTurns') as number);
    const toolResponseMaxBytes = readNum('toolResponseMaxBytes', 'toolResponseMaxBytes', defaultOf('toolResponseMaxBytes') as number);
    const maxConcurrentTools = readNum('maxConcurrentTools', 'maxConcurrentTools', defaultOf('maxConcurrentTools') as number);
    const parallelToolCalls = readBool('parallelToolCalls', 'parallelToolCalls', defaultOf('parallelToolCalls') as boolean);

    const inv = (typeof candidate === 'string' && candidate.length > 0) ? candidate : 'ai-agent';
    const usageText = (() => {
      if (fmUsage.length > 0) return `${inv} "${fmUsage}"`;
      return `${inv} "<user prompt>"`;
    })();

    // runtime toggles are CLI-only and not shown in template
    // Include output if present in frontmatter
    let outputBlock: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> } | undefined;
    try {
      const contentForOutput = promptPath !== undefined ? fs.readFileSync(promptPath, 'utf8') : undefined;
      if (contentForOutput !== undefined) {
        let c = contentForOutput;
        if (c.startsWith('#!')) { const nl = c.indexOf('\n'); c = nl >= 0 ? c.slice(nl + 1) : ''; }
        const parsed = parseFrontmatter(c);
        if (parsed?.expectedOutput !== undefined) {
          const out: Record<string, unknown> = { format: parsed.expectedOutput.format };
          if (parsed.expectedOutput.schema !== undefined) out.schema = parsed.expectedOutput.schema;
          outputBlock = out as { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> };
        }
      }
    } catch { /* ignore */ }

    const fmTemplate = buildFrontmatterTemplate({
      fmOptions,
      description: fmDescOnly,
      usage: (fmUsage.length > 0 ? fmUsage : '<user prompt>'),
      numbers: {
        temperature,
        topP,
        llmTimeout,
        toolTimeout,
        toolResponseMaxBytes,
        maxRetries,
        maxToolTurns,
        maxConcurrentTools,
      },
      booleans: {
        stream: false,
        parallelToolCalls,
        traceLLM: false,
        traceMCP: false,
        verbose: false,
      },
      strings: {},
      output: outputBlock,
    });

    const lines: string[] = [];
    lines.push('DESCRIPTION');
    if (fmDesc.length > 0) lines.push(fmDesc);
    lines.push('');
    lines.push('Usage:');
    lines.push(`   ${usageText}`);
    lines.push('');
    lines.push('Frontmatter Template:');
    lines.push('---');
    const dumpedUnknown = (yaml as unknown as { dump: (o: unknown, opts?: Record<string, unknown>) => unknown }).dump(fmTemplate, { lineWidth: 120 });
    const yamlText = typeof dumpedUnknown === 'string' ? dumpedUnknown : JSON.stringify(dumpedUnknown);
    lines.push(yamlText.trimEnd());
    lines.push('---');
    // Also show prompt/config locations for context
    if (promptPath !== undefined) lines.push('', `Prompt: ${promptPath}`);
    try {
      const layers = discoverLayers({ configPath: undefined, promptPath });
      lines.push('Config Files Resolution Order:');
      layers.forEach((ly, idx) => {
        const jExists = fs.existsSync(ly.jsonPath);
        const eExists = fs.existsSync(ly.envPath);
        lines.push(` ${String(idx + 1)}. ${ly.jsonPath} ${jExists ? '(found)' : '(missing)'} | ${ly.envPath} ${eExists ? '(found)' : '(missing)'}`);
      });
      lines.push('');
    } catch { /* ignore */ }
    return `\n${lines.join('\n')}\n`;
  } catch {
    return '';
  }
}

// Inject description and resolved defaults before the standard help
program.addHelpText('before', buildResolvedDefaultsHelp);

// Grouped CLI options legend for clarity
program.addHelpText('after', () => {
  const byGroup = getOptionsByGroup();
  const order = ['Master Agent Overrides', 'Master Defaults', 'All Models Overrides', 'Global Controls'];
  const makeTitle = (group: string): string => (
    group === 'Master Defaults' ? `${group} (used by sub-agents when unset)`
    : group === 'Master Agent Overrides' ? `${group} (strict)`
    : group === 'Global Controls' ? 'Global Application Controls'
    : group
  );
  const body = order
    .map((group) => {
      const list = byGroup.get(group);
      if (list === undefined || list.length === 0) return undefined;
      const lines = [makeTitle(group)];
      list
        .filter((opt) => opt.cli?.showInHelp === true)
        .map((opt) => formatCliNames(opt))
        .filter((s) => typeof s === 'string' && s.length > 0)
        .forEach((s) => lines.push(`  ${s}`));
      lines.push('');
      return lines.join('\n');
    })
    .filter((s): s is string => typeof s === 'string');
  return `\n${['', ...body].join('\n')}`;
});

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support')
  .version('1.0.0')
  .hook('preSubcommand', () => { /* placeholder */ });

const agentOption = new Option('--agent <path>', 'Register an agent (.ai) file; repeat to add multiple agents')
  .argParser((value: string, previous: string[]) => appendValue(value, previous))
  .default([], undefined);

const apiHeadendOption = new Option('--api <port>', 'Start REST API headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const mcpHeadendOption = new Option('--mcp <transport>', 'Start MCP headend (stdio|http:port|sse:port|ws:port)')
  .argParser((value: string, previous: string[]) => appendValue(value, previous))
  .default([], undefined);

const openaiToolHeadendOption = new Option('--openai-tool <port>', 'Start OpenAI tool headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const openaiCompletionsHeadendOption = new Option('--openai-completions <port>', 'Start OpenAI chat completions headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const anthropicCompletionsHeadendOption = new Option('--anthropic-completions <port>', 'Start Anthropic messages headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const apiConcurrencyOption = new Option('--api-concurrency <n>', 'Maximum concurrent REST API sessions')
  .argParser(parsePositive);

const openaiToolConcurrencyOption = new Option('--openai-tool-concurrency <n>', 'Maximum concurrent OpenAI tool sessions')
  .argParser(parsePositive);

const openaiCompletionsConcurrencyOption = new Option('--openai-completions-concurrency <n>', 'Maximum concurrent OpenAI chat sessions')
  .argParser(parsePositive);

const anthropicCompletionsConcurrencyOption = new Option('--anthropic-completions-concurrency <n>', 'Maximum concurrent Anthropic chat sessions')
  .argParser(parsePositive);

program.addOption(agentOption);
program.addOption(apiHeadendOption);
program.addOption(mcpHeadendOption);
program.addOption(openaiToolHeadendOption);
program.addOption(openaiCompletionsHeadendOption);
program.addOption(anthropicCompletionsHeadendOption);
program.addOption(apiConcurrencyOption);
program.addOption(openaiToolConcurrencyOption);
program.addOption(openaiCompletionsConcurrencyOption);
program.addOption(anthropicCompletionsConcurrencyOption);

interface HeadendModeConfig {
  agentPaths: string[];
  apiPorts: number[];
  mcpTargets: string[];
  openaiToolPorts: number[];
  openaiCompletionsPorts: number[];
  anthropicCompletionsPorts: number[];
  options: Record<string, unknown>;
}

const readCliTools = (opts: Record<string, unknown>): string | undefined => {
  const keys = ['tools', 'tool', 'mcp', 'mcpTool', 'mcpTools'] as const;
  const candidate = keys
    .map((key) => opts[key])
    .find((value): value is string => typeof value === 'string' && value.length > 0);
  return candidate;
};

async function runHeadendMode(config: HeadendModeConfig): Promise<void> {
  const uniqueAgents = Array.from(new Set(config.agentPaths.map((p) => path.resolve(p))));
  if (uniqueAgents.length === 0) {
    exitWith(4, 'headend mode requires at least one --agent <file>', 'EXIT-HEADEND-NO-AGENTS');
  }
  const missingAgents = uniqueAgents.filter((p) => !fs.existsSync(p));
  if (missingAgents.length > 0) {
    const missing = missingAgents.join(', ');
    exitWith(4, `agent file not found: ${missing}`, 'EXIT-HEADEND-MISSING-AGENT');
  }
  if (
    config.apiPorts.length === 0
    && config.mcpTargets.length === 0
    && config.openaiToolPorts.length === 0
    && config.openaiCompletionsPorts.length === 0
    && config.anthropicCompletionsPorts.length === 0
  ) {
    exitWith(4, 'no headends specified; add --api/--mcp/--openai-tool/--openai-completions/--anthropic-completions', 'EXIT-HEADEND-NO-HEADENDS');
  }

  const configPathValue = typeof config.options.config === 'string' && config.options.config.length > 0
    ? config.options.config
    : undefined;
  const verbose = config.options.verbose === true;
  const traceLLMFlag = config.options.traceLlm === true;
  const traceMCPFlag = config.options.traceMcp === true;

  let parsedTargets: LoadAgentOptions['targets'];
  const cliModels = typeof config.options.models === 'string' && config.options.models.length > 0
    ? config.options.models
    : undefined;
  if (cliModels !== undefined) {
    try {
      parsedTargets = parsePairs(cliModels);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, `invalid --models value: ${message}`, 'EXIT-HEADEND-BAD-MODELS');
    }
  }

  let parsedTools: LoadAgentOptions['tools'];
  const cliTools = readCliTools(config.options);
  if (cliTools !== undefined) {
    try {
      parsedTools = parseList(cliTools);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, `invalid tools specification: ${message}`, 'EXIT-HEADEND-BAD-TOOLS');
    }
  }

  const loadOptions: LoadAgentOptions = {
    configPath: configPathValue,
    verbose,
    traceLLM: traceLLMFlag,
    traceMCP: traceMCPFlag,
  };
  if (parsedTargets !== undefined) {
    loadOptions.targets = parsedTargets;
  }
  if (parsedTools !== undefined) {
    loadOptions.tools = parsedTools;
  }

  const registry = new AgentRegistry(uniqueAgents, loadOptions);
  const parseMcpTarget = (raw: string): McpTransportSpec => {
    if (raw === 'stdio') return { type: 'stdio' };
    const [kind, portRaw] = raw.split(':');
    if ((kind === 'http' || kind === 'sse' || kind === 'ws') && typeof portRaw === 'string' && portRaw.length > 0) {
      const port = Number.parseInt(portRaw, 10);
      if (!Number.isFinite(port) || port <= 0 || port > 65535) {
        exitWith(4, `invalid port '${portRaw}' for --mcp`, 'EXIT-HEADEND-MCP-PORT');
      }
      if (kind === 'http') return { type: 'streamable-http', port };
      if (kind === 'sse') return { type: 'sse', port };
      return { type: 'ws', port };
    }
    exitWith(4, `unsupported MCP transport '${raw}'`, 'EXIT-HEADEND-MCP-TRANSPORT');
  };
  const mcpSpecs = config.mcpTargets.map((raw) => parseMcpTarget(raw));

  const readConcurrency = (key: string): number | undefined => {
    const raw = config.options[key];
    if (typeof raw === 'number' && Number.isFinite(raw) && raw > 0) return Math.floor(raw);
    if (typeof raw === 'string' && raw.length > 0) {
      const parsed = Number.parseInt(raw, 10);
      if (Number.isFinite(parsed) && parsed > 0) return Math.floor(parsed);
    }
    return undefined;
  };

  const apiConcurrency = readConcurrency('apiConcurrency');
  const openaiToolConcurrency = readConcurrency('openaiToolConcurrency');
  const openaiCompletionsConcurrency = readConcurrency('openaiCompletionsConcurrency');
  const anthropicCompletionsConcurrency = readConcurrency('anthropicCompletionsConcurrency');

  const headends = [] as Headend[];
  config.apiPorts.forEach((port) => {
    headends.push(new RestHeadend(registry, { port, concurrency: apiConcurrency }));
  });
  mcpSpecs.forEach((spec) => {
    headends.push(new McpHeadend({ registry, transport: spec }));
  });
  config.openaiToolPorts.forEach((port) => {
    headends.push(new OpenAIToolHeadend(registry, { port, concurrency: openaiToolConcurrency }));
  });
  config.openaiCompletionsPorts.forEach((port) => {
    headends.push(new OpenAICompletionsHeadend(registry, { port, concurrency: openaiCompletionsConcurrency }));
  });
  config.anthropicCompletionsPorts.forEach((port) => {
    headends.push(new AnthropicCompletionsHeadend(registry, { port, concurrency: anthropicCompletionsConcurrency }));
  });
  const ttyLog = makeTTYLogCallbacks({
    color: true,
    verbose: config.options.verbose === true,
    traceLlm: config.options.traceLlm === true,
    traceMcp: config.options.traceMcp === true,
  });
  const logSink: HeadendLogSink = (entry) => { ttyLog.onLog?.(entry); };
  const emit = (message: string, severity: LogEntry['severity'] = 'VRB'): void => {
    logSink({
      timestamp: Date.now(),
      severity,
      turn: 0,
      subturn: 0,
      direction: 'response',
      type: 'tool',
      remoteIdentifier: 'headend:cli',
      fatal: severity === 'ERR',
      message,
      headendId: 'cli',
    });
  };

  const manager = new HeadendManager(headends, {
    log: logSink,
    onFatal: (event) => {
      const desc = event.headend.describe();
      emit(`headend ${desc.label} fatal error: ${event.error.message}`, 'ERR');
    },
  });

  __RUNNING_SERVER = true;

  const stopManager = async () => {
    await manager.stopAll();
  };

  const handleSignal = (signal: NodeJS.Signals) => {
    emit(`received ${signal}, shutting down headends`, 'WRN');
    void stopManager();
  };

  const registeredSignals: NodeJS.Signals[] = ['SIGINT', 'SIGTERM'];
  const signalHandlers = new Map<NodeJS.Signals, () => void>();
  registeredSignals.forEach((sig) => {
    const handler = () => { handleSignal(sig); };
    signalHandlers.set(sig, handler);
    process.once(sig, handler);
  });

  try {
    emit(`starting headends: ${headends.map((h) => h.describe().label).join(', ')}`);
    await manager.startAll();
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    await stopManager();
    emit(`failed to start headends: ${message}`, 'ERR');
    exitWith(1, `failed to start headends: ${message}`, 'EXIT-HEADEND-START');
  }

  const fatal = await manager.waitForFatal();
  await stopManager();
  signalHandlers.forEach((handler, sig) => {
    process.removeListener(sig, handler);
  });

  if (fatal !== undefined) {
    const desc = fatal.headend.describe();
    const message = fatal.error.message;
    emit(`headend ${desc.label} failed: ${message}`, 'ERR');
    exitWith(1, `headend '${desc.label}' failed: ${message}`, 'EXIT-HEADEND-FATAL');
  }
  emit('all headends stopped gracefully', 'FIN');
  __RUNNING_SERVER = false;
}

// Global flag: suppress fatal exits in server mode
let __RUNNING_SERVER = false;

program
  .command('server')
  .description('Start the server headend (Slack + REST API)')
  .argument('<agentPath>', 'Path to the .ai file to serve')
  .option('--slack', 'Enable Slack headend (Socket Mode)')
  .option('--no-slack', 'Disable Slack headend')
  .option('--api', 'Enable REST API headend')
  .option('--no-api', 'Disable REST API headend')
  .option('--trace-llm', 'Trace LLM requests/responses (server mode)')
  .option('--trace-mcp', 'Trace MCP requests/responses (server mode)')
  .action(async (agentPath: string, opts: Record<string, unknown>) => {
    try {
      __RUNNING_SERVER = true;
      const mod = await import('./server/index.js');
      const enableSlack = opts.slack === true ? true : (opts.slack === false ? false : undefined);
      const enableApi = opts.api === true ? true : (opts.api === false ? false : undefined);
      // Merge root (global) flags with subcommand flags so users can pass --verbose/--trace-llm either before or after 'server'
      const rootOptsObj = typeof program.opts === 'function' ? program.opts() : {};
      const rootOpts: Record<string, unknown> = rootOptsObj as Record<string, unknown>;
      const verbose = (opts as { verbose?: boolean }).verbose === true || (rootOpts.verbose === true);
      const traceLlm = (opts as { traceLlm?: boolean }).traceLlm === true || (rootOpts.traceLlm === true);
      const traceMcp = (opts as { traceMcp?: boolean }).traceMcp === true || (rootOpts.traceMcp === true);
      await (mod.startServer as (p: string, o?: { enableSlack?: boolean; enableApi?: boolean; verbose?: boolean; traceLlm?: boolean; traceMcp?: boolean }) => Promise<void>)(agentPath, {
        enableSlack,
        enableApi,
        verbose,
        traceLlm,
        traceMcp,
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      exitWith(1, `failed to start server: ${msg}`, 'EXIT-SERVER-START');
    }
  });

program
  .argument('[system-prompt]', 'System prompt (string, @filename, or - for stdin)')
  .argument('[user-prompt]', 'User prompt (string, @filename, or - for stdin)')
  .option('--save-all <dir>', 'Save all agent and sub-agent conversations to directory')
  .option('--show-tree', 'Dump the full execution tree (ASCII) at the end')
  .hook('preAction', () => { /* placeholder to ensure options added first */ })
  .action(async (systemPrompt: string | undefined, userPrompt: string | undefined, options: Record<string, unknown>) => {
    try {
      const agentFlags = Array.isArray(options.agent) ? (options.agent as string[]) : [];
      const apiPorts = Array.isArray(options.api) ? (options.api as number[]) : [];
      const mcpTargets = Array.isArray(options.mcp) ? (options.mcp as string[]) : [];
      const openaiToolPorts = Array.isArray(options.openaiTool) ? (options.openaiTool as number[]) : [];
      const openaiCompletionsPorts = Array.isArray(options.openaiCompletions) ? (options.openaiCompletions as number[]) : [];
      const anthropicCompletionsPorts = Array.isArray(options.anthropicCompletions) ? (options.anthropicCompletions as number[]) : [];

      if (apiPorts.length > 0 || mcpTargets.length > 0 || openaiToolPorts.length > 0 || openaiCompletionsPorts.length > 0 || anthropicCompletionsPorts.length > 0) {
        await runHeadendMode({
          agentPaths: agentFlags,
          apiPorts,
          mcpTargets,
          openaiToolPorts,
          openaiCompletionsPorts,
          anthropicCompletionsPorts,
          options,
        });
        return;
      }

      if (systemPrompt === undefined || userPrompt === undefined) {
        exitWith(4, 'system and user prompts are required when no headends are enabled', 'EXIT-INVALID-ARGS');
      }

      if (systemPrompt === '-' && userPrompt === '-') {
        exitWith(4, 'invalid arguments: cannot use stdin for both system and user prompts', 'EXIT-INVALID-ARGS');
      }
      

      const cfgPath = typeof options.config === 'string' && options.config.length > 0 ? options.config : undefined;

      // (moved) numeric options will be resolved after reading prompts/frontmatter

      // Resolve prompts and parse frontmatter for expected output and options
      let resolvedSystemRaw = await readPrompt(systemPrompt);
      let resolvedUserRaw = await readPrompt(userPrompt);
      const sysBaseDir = (() => { try { return (systemPrompt !== '-' && fs.existsSync(systemPrompt) && fs.statSync(systemPrompt).isFile()) ? path.dirname(systemPrompt) : undefined; } catch { return undefined; } })();
      const usrBaseDir = (() => { try { return (userPrompt !== '-' && fs.existsSync(userPrompt) && fs.statSync(userPrompt).isFile()) ? path.dirname(userPrompt) : undefined; } catch { return undefined; } })();
      resolvedSystemRaw = resolveIncludes(resolvedSystemRaw, sysBaseDir);
      resolvedUserRaw = resolveIncludes(resolvedUserRaw, usrBaseDir);
      const fmSys = parseFrontmatter(resolvedSystemRaw, { baseDir: sysBaseDir });
      const fmUsr = parseFrontmatter(resolvedUserRaw, { baseDir: usrBaseDir });
      const fm = fmSys ?? fmUsr;

      const fmOptions: FrontmatterOptions | undefined = fm?.options;
      // Removed local resolution of runtime knobs; agent-loader owns precedence

      // Determine effective targets and tools (CLI > FM)
      const cliTargetsRaw: string | undefined = (typeof options.models === 'string' && options.models.length > 0) ? options.models : undefined;

      const cliToolsRaw: string | undefined =
        (typeof options.tools === 'string' && options.tools.length > 0) ? options.tools
        : (typeof options.tool === 'string' && options.tool.length > 0) ? options.tool
        : (typeof options.mcp === 'string' && options.mcp.length > 0) ? options.mcp
        : (typeof options.mcpTool === 'string' && options.mcpTool.length > 0) ? options.mcpTool
        : (typeof options.mcpTools === 'string' && options.mcpTools.length > 0) ? options.mcpTools
        : undefined;

      const isPlainObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
      const hasKey = <K extends string>(obj: Record<string, unknown>, key: K): obj is Record<K, unknown> => Object.prototype.hasOwnProperty.call(obj, key);
      const cliAgentsRaw: string | undefined = ((): string | undefined => {
        if (isPlainObject(options) && hasKey(options, 'agents')) {
          const raw = options.agents;
          return (typeof raw === 'string' && raw.length > 0) ? raw : undefined;
        }
        return undefined;
      })();

      // fmOptions already computed above
      const fmTargetsRaw = (fmOptions !== undefined && typeof fmOptions.models === 'string') ? fmOptions.models
        : (fmOptions !== undefined && Array.isArray(fmOptions.models)) ? fmOptions.models.join(',')
        : undefined;

      const fmToolsRaw = (fmOptions !== undefined && typeof fmOptions.tools === 'string') ? fmOptions.tools
        : (fmOptions !== undefined && Array.isArray(fmOptions.tools)) ? fmOptions.tools.join(',')
        : undefined;
      const fmAgentsRaw = ((): string | undefined => {
        if (fmOptions !== undefined) {
          const fmAny = fmOptions as Record<string, unknown>;
          const a = fmAny.agents;
          if (typeof a === 'string' && a.length > 0) return a;
          if (Array.isArray(a)) return (a as unknown[]).map((x) => String(x)).join(',');
        }
        return undefined;
      })();

      const targets = parsePairs(cliTargetsRaw ?? fmTargetsRaw);
      const toolList = parseList(cliToolsRaw ?? fmToolsRaw);
      const agentsList = parseList(cliAgentsRaw ?? fmAgentsRaw);

      if (targets.length === 0) {
        console.error('Error: No provider/model targets specified. Use --models or frontmatter models.');
        process.exit(4);
      }
      // Allow LLM-only runs (no tools/agents). Tool availability rules are enforced later:
      // - MCP tools missing from config: fatal at validation
      // - MCP tools failing at runtime: continue without them
      // - Agents missing: load throws early (fatal)

      // Probe layered config locations (verbose only)
      try {
        // Use system prompt path if it's a file, otherwise user prompt path
        const promptPath = (systemPrompt !== '-' && fs.existsSync(systemPrompt) && fs.statSync(systemPrompt).isFile()) ? systemPrompt
          : (userPrompt !== '-' && fs.existsSync(userPrompt) && fs.statSync(userPrompt).isFile()) ? userPrompt
          : undefined;
        const layers = discoverLayers({ configPath: cfgPath, promptPath });
        if (options.verbose === true) {
                    layers.forEach((ly) => {
            const jExists = fs.existsSync(ly.jsonPath);
            const eExists = fs.existsSync(ly.envPath);
            const msg = `[VRB] → [0.0] llm agent:config: probing ${ly.origin}: json=${ly.jsonPath} ${jExists ? '(found)' : '(missing)'} env=${ly.envPath} ${eExists ? '(found)' : '(missing)'}\n`;
            try { process.stderr.write(process.stderr.isTTY ? `\x1b[90m${msg}\x1b[0m` : msg); } catch { }
          });
        }
      } catch { /* ignore */ }

      // Build unified configuration via agent-loader (single path)
      const fmSource = (fmSys !== undefined) ? resolvedSystemRaw : resolvedUserRaw;
      const fmBaseDir = (fmSys !== undefined) ? sysBaseDir : usrBaseDir;
      // Only override numeric options if explicitly provided via CLI (avoid overriding frontmatter with Commander defaults)
      const optSrc = (name: string): 'cli' | 'default' | 'env' | 'implied' | undefined => {
        try {
          return program.getOptionValueSource(name) as 'cli' | 'default' | 'env' | 'implied' | undefined;
        } catch {
          return undefined;
        }
      };

      // Derive agent id: from prompt filename when available, else 'cli-main'
      const fileAgentId = ((): string => {
        const isFile = (p: string): boolean => { try { return p !== '-' && fs.existsSync(p) && fs.statSync(p).isFile(); } catch { return false; } };
        if (isFile(systemPrompt)) return systemPrompt;
        if (isFile(userPrompt)) return userPrompt;
        return 'cli-main';
      })();

      const loaded = loadAgentFromContent(fileAgentId, fmSource, {
        configPath: cfgPath,
        verbose: options.verbose === true,
        targets,
        tools: toolList,
        agents: agentsList,
        baseDir: fmBaseDir,
        // CLI overrides take precedence
        temperature: optSrc('temperature') === 'cli' ? Number(options.temperature) : undefined,
        topP: (optSrc('topP') === 'cli' || optSrc('top-p') === 'cli') ? Number(options.topP ?? options['top-p']) : undefined,
        llmTimeout: (optSrc('llmTimeoutMs') === 'cli' || optSrc('llm-timeout-ms') === 'cli') ? Number(options.llmTimeoutMs ?? options['llm-timeout-ms']) : undefined,
        toolTimeout: (optSrc('toolTimeoutMs') === 'cli' || optSrc('tool-timeout-ms') === 'cli') ? Number(options.toolTimeoutMs ?? options['tool-timeout-ms']) : undefined,
        maxRetries: optSrc('maxRetries') === 'cli' ? Number(options.maxRetries) : undefined,
        maxToolTurns: optSrc('maxToolTurns') === 'cli' ? Number(options.maxToolTurns) : undefined,
        toolResponseMaxBytes: (optSrc('toolResponseMaxBytes') === 'cli' || optSrc('tool-response-max-bytes') === 'cli') ? Number(options.toolResponseMaxBytes ?? options['tool-response-max-bytes']) : undefined,
        maxConcurrentTools: (optSrc('maxConcurrentTools') === 'cli' || optSrc('max-concurrent-tools') === 'cli')
          ? (() => {
              const o = options as Record<string, unknown> & { maxConcurrentTools?: unknown };
              const v = o.maxConcurrentTools ?? o['max-concurrent-tools'];
              return Number(v);
            })()
          : undefined,
        parallelToolCalls: typeof options.parallelToolCalls === 'boolean' ? options.parallelToolCalls : undefined,
        stream: typeof options.stream === 'boolean' ? options.stream : undefined,
        traceLLM: options.traceLlm === true ? true : undefined,
        traceMCP: options.traceMcp === true ? true : undefined,
        mcpInitConcurrency: (typeof options.mcpInitConcurrency === 'string' && options.mcpInitConcurrency.length>0) ? Number(options.mcpInitConcurrency) : undefined,
      });

      // Prompt variable substitution using loader-effective max tool turns
      const expectedJson = fm?.expectedOutput?.format === 'json';
      const fmtRaw: unknown = (() => { const rec: Record<string, unknown> = options; return Object.prototype.hasOwnProperty.call(rec, 'format') ? (rec as { format?: unknown }).format : undefined; })();
      const fmtOpt = typeof fmtRaw === 'string' && fmtRaw.length > 0 ? fmtRaw : undefined;
      const chosenFormatId = resolveFormatIdForCli(fmtOpt, expectedJson, process.stdout.isTTY ? true : false);
      const vars = buildPromptVariables(loaded.effective.maxToolTurns);
      vars.FORMAT = describeFormat(chosenFormatId);
      const resolvedSystem = expandPrompt(stripFrontmatter(resolvedSystemRaw), vars);
      const resolvedUser = expandPrompt(stripFrontmatter(resolvedUserRaw), vars);

// Load conversation history if specified
      let conversationHistory: ConversationMessage[] | undefined = undefined;
      const effLoad = (typeof options.load === 'string' && options.load.length > 0) ? options.load : undefined;
      if (typeof effLoad === 'string' && effLoad.length > 0) {
        try {
          const content = fs.readFileSync(effLoad, 'utf-8');
          conversationHistory = JSON.parse(content) as ConversationMessage[];
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          exitWith(1, `failed to load conversation from ${effLoad}: ${msg}`, 'EXIT-CONVERSATION-LOAD');
        }
      }

      // Setup accounting
      const accountingFile = (typeof options.accounting === 'string' && options.accounting.length > 0)
        ? options.accounting
        : loaded.accountingFile ?? loaded.config.accounting?.file;

      // Setup logging callbacks (CLI-only)
      const effectiveTraceLLM = options.traceLlm === true;
      const effectiveTraceMCP = options.traceMcp === true;
      const effectiveVerbose = options.verbose === true;
      const callbacks = createCallbacks({ traceLlm: effectiveTraceLLM, traceMcp: effectiveTraceMCP, verbose: effectiveVerbose }, accountingFile);

      if (options.dryRun === true) {
        exitWith(0, 'dry run complete: configuration and MCP servers validated', 'EXIT-DRY-RUN');
      }

      // Create and run session via unified loader
      const result = await loaded.run(resolvedSystem, resolvedUser, { history: conversationHistory, callbacks, renderTarget: 'cli', outputFormat: chosenFormatId });

      // Always print only the formatted human-readable output to stdout
      try {
        let out = formatAgentResultHumanReadable(result);
        // If rendering to TTY, normalize simplified color hints like "[33m" to real ANSI ESC sequences
        if (chosenFormatId === 'tty') {
          const normalizeAnsi = (s: string): string => {
            let t = s;
            // Convert literal \x1b[ or \u001b[ sequences to real ESC + [
            t = t.replace(/\\x1b\[/gi, '\x1b[');
            t = t.replace(/\\u001b\[/gi, '\x1b[');
            // Do NOT convert bare "[33m" style sequences; show them as-is per request
            return t;
          };
          out = normalizeAnsi(out);
        }
        process.stdout.write(out);
        if (!out.endsWith('\n')) process.stdout.write('\n');
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        console.error(`Formatting error: ${msg}`);
      }

      if (!result.success) {
        const err = result.error ?? '';
        const code = err.includes('Configuration') ? 1
          : err.includes('Max tool turns exceeded') ? 5
          : err.includes('tool') ? 3
          : 2;
        // If requested, dump execution tree before exiting
        if (options.showTree === true && typeof result.treeAscii === 'string' && result.treeAscii.length > 0) {
          try { process.stderr.write(`\n=== Execution Tree ===\n${result.treeAscii}\n`); } catch { /* ignore */ }
        }
        exitWith(code, `agent failure: ${err || 'unknown error'}`, 'EXIT-AGENT-FAILURE');
      }

      // Save conversation if requested (CLI-only)
      const effSave = (typeof options.save === 'string' && options.save.length > 0) ? options.save : undefined;
      if (typeof effSave === 'string' && effSave.length > 0) {
        try {
          fs.writeFileSync(effSave, JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          exitWith(1, `failed to save conversation to ${effSave}: ${msg}`, 'EXIT-SAVE-CONVERSATION');
        }
      }

      // Save all conversations if requested (master + sub-agents)
      const saveAllDir = (typeof options.saveAll === 'string' && options.saveAll.length > 0) ? options.saveAll : undefined;
      if (typeof saveAllDir === 'string' && saveAllDir.length > 0) {
        try {
          fs.mkdirSync(saveAllDir, { recursive: true });
          // Derive origin dir name from first accounting entry with originTxnId or fallback
          const origin = ((): string => {
            const a = result.accounting.find((x) => typeof x.originTxnId === 'string' && x.originTxnId.length > 0);
            return a?.originTxnId ?? 'origin';
          })();
          const originDir = path.join(saveAllDir, origin);
          fs.mkdirSync(originDir, { recursive: true });
          // Save master conversation
          const masterName = 'master';
          const masterSelf = ((): string => {
            const a = result.accounting.find((x) => typeof x.txnId === 'string' && x.txnId.length > 0);
            return a?.txnId ?? 'self';
          })();
          const masterFile = path.join(originDir, `${masterSelf}__${masterName}.json`);
          fs.writeFileSync(masterFile, JSON.stringify(result.conversation, null, 2), 'utf-8');
          // Save each child
          (result.childConversations ?? []).forEach((c, idx) => {
            const agent = (c.agentId ?? c.toolName);
            const selfId = c.trace?.selfId ?? String(idx + 1);
            const fname = `${selfId}__${agent}.json`;
            const fpath = path.join(originDir, fname);
            fs.writeFileSync(fpath, JSON.stringify(c.conversation, null, 2), 'utf-8');
          });
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          exitWith(1, `failed to save conversations to ${saveAllDir}: ${msg}`, 'EXIT-SAVE-ALL');
        }
      }

      // Successful completion - optionally print execution trees
      if (options.showTree === true) {
        if (typeof result.opTreeAscii === 'string' && result.opTreeAscii.length > 0) {
          try { process.stderr.write(`\n=== Operation Tree ===\n${result.opTreeAscii}\n`); } catch { /* ignore */ }
        }
        if (typeof result.treeAscii === 'string' && result.treeAscii.length > 0) {
          try { process.stderr.write(`\n=== Execution Tree ===\n${result.treeAscii}\n`); } catch { /* ignore */ }
        }
      }
      // Agent already logged EXIT-FINAL-ANSWER or similar
      exitWith(0, 'success', 'EXIT-SUCCESS');
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      const code = msg.includes('config') ? 1
        : msg.includes('argument') ? 4
        : msg.includes('tool') ? 3
        : 1;
      exitWith(code, `fatal error in CLI: ${msg}`, 'EXIT-UNKNOWN');
    }
  });

// Add CLI options from the registry after base command is declared
addOptionsFromRegistry(program);

// Helper functions
// parsePositiveInt helper removed from run path; not needed

// kept for help-text parity; no longer used in run path
// parseFloat helper removed from run path; not needed

async function readPrompt(value: string): Promise<string> {
  if (value === '-') {
    const chunks: Buffer[] = [];
    await new Promise<void>((resolve, reject) => {
      process.stdin.on('data', (d) => chunks.push(Buffer.from(d)));
      process.stdin.on('end', () => { resolve(); });
      process.stdin.on('error', reject);
    });
    return Buffer.concat(chunks).toString('utf8');
  }
  const readIfFile = (p: string): string | undefined => {
    try {
      if (fs.existsSync(p) && fs.statSync(p).isFile()) {
        let c = fs.readFileSync(p, 'utf8');
        // Strip shebang if present
        if (c.startsWith('#!')) {
          const idx = c.indexOf('\n');
          c = idx >= 0 ? c.slice(idx + 1) : '';
        }
        return c;
      }
    } catch { /* ignore */ }
    return undefined;
  };
  if (value.startsWith('@')) {
    const p = value.slice(1);
    const content = readIfFile(p);
    if (content !== undefined) return content;
  } else {
    const content = readIfFile(value);
    if (content !== undefined) return content;
  }
  return value;
}

function buildPromptVariables(maxToolTurns: number): Record<string, string> {
  function pad2(n: number): string { return n < 10 ? `0${String(n)}` : String(n); }
  function formatRFC3339Local(d: Date): string {
    const y = d.getFullYear();
    const m = pad2(d.getMonth() + 1);
    const da = pad2(d.getDate());
    const hh = pad2(d.getHours());
    const mm = pad2(d.getMinutes());
    const ss = pad2(d.getSeconds());
    const tzMin = -d.getTimezoneOffset();
    const sign = tzMin >= 0 ? '+' : '-';
    const abs = Math.abs(tzMin);
    const tzh = pad2(Math.floor(abs / 60));
    const tzm = pad2(abs % 60);
    return `${String(y)}-${m}-${da}T${hh}:${mm}:${ss}${sign}${tzh}:${tzm}`;
  }
  function detectTimezone(): string {
    try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return process.env.TZ ?? 'UTC'; }
  }
  function detectOS(): string {
    try {
      const content = fs.readFileSync('/etc/os-release', 'utf-8');
      const match = /^PRETTY_NAME=\"?([^\"\n]+)\"?/m.exec(content);
      if (match?.[1] !== undefined) return `${match[1]} (kernel ${os.release()})`;
    } catch { /* ignore */ }
    return `${os.type()} ${os.release()}`;
  }

  const now = new Date();
  return {
    DATETIME: formatRFC3339Local(now),
    DAY: now.toLocaleDateString(undefined, { weekday: 'long' }),
    TIMEZONE: detectTimezone(),
    MAX_TURNS: String(maxToolTurns),
    OS: detectOS(),
    ARCH: process.arch,
    KERNEL: `${os.type()} ${os.release()}`,
    CD: process.cwd(),
    HOSTNAME: os.hostname(),
    USER: (() => { try { return os.userInfo().username; } catch { return process.env.USER ?? process.env.USERNAME ?? ''; } })(),
  };
}

function expandPrompt(str: string, vars: Record<string, string>): string {
  const replace = (s: string, re: RegExp) => s.replace(re, (_m, name: string) => (name in vars ? vars[name] : _m));
  let out = str;
  out = replace(out, /\$\{([A-Z_]+)\}/g);
  out = replace(out, /\{\{([A-Z_]+)\}\}/g);
  return out;
}

// Frontmatter helpers are imported from frontmatter.ts

function createCallbacks(options: Record<string, unknown>, _accountingFile?: string): AIAgentCallbacks {

  // Helper for consistent coloring - MANDATORY in TTY according to DESIGN.md
  const colorize = (text: string, colorCode: string): string => {
    return process.stderr.isTTY ? `${colorCode}${text}\x1b[0m` : text;
  };
  

  // Track thinking stream state to ensure proper newlines between logs
  let thinkingOpen = false;
  let lastCharWasNewline = true;

      const ttyLog = makeTTYLogCallbacks({ color: true, verbose: options.verbose === true, traceLlm: options.traceLlm === true, traceMcp: options.traceMcp === true });
      return {
        onLog: (entry: LogEntry) => {
          // Ensure newline separation after thinking stream if needed
          if (entry.severity !== 'THK' && thinkingOpen && !lastCharWasNewline) {
            try { process.stderr.write('\n'); } catch {}
            lastCharWasNewline = true;
            thinkingOpen = false;
          }
      // Delegate all non-THK logs to the shared TTY sink
      ttyLog.onLog?.(entry);
      
      // Show thinking header, including txn and stable path via formatter; then stream text via onThinking
      if (entry.severity === 'THK') {
        const header = formatLog(entry, { color: true, verbose: options.verbose === true, traceLlm: options.traceLlm === true, traceMcp: options.traceMcp === true });
        // formatLog already applies dim color for THK; do not add newline so streamed text follows
        try { process.stderr.write(`${header} `); } catch {}
        thinkingOpen = true;
        lastCharWasNewline = false;
        // The actual thinking text will follow via onThinking
      }
    },
    
    onOutput: (text: string) => {
      // Only mirror streamed output to stderr when verbose; keep stdout clean
      if (options.verbose === true) {
        try { process.stderr.write(text); } catch {}
      }
    },
    
    onThinking: (text: string) => { 
      const colored = colorize(text, '\x1b[2;37m'); // Light gray (dim white) for thinking
      process.stderr.write(colored);
      if (text.length > 0) {
        lastCharWasNewline = text.endsWith('\n');
        thinkingOpen = true;
      }
    },
    
    onAccounting: (_entry: AccountingEntry) => {
      // No per-entry file writes from CLI; final ledger is written by ai-agent at session end.
    },
  };
}

process.on('uncaughtException', (e) => {
  const msg = e instanceof Error ? `${e.name}: ${e.message}` : String(e);
  if (__RUNNING_SERVER) {
    const warn = `[warn] uncaught exception (continuing): ${msg}`;
    try { process.stderr.write(`${warn}
`); } catch {}
    return;
  }
  exitWith(1, `uncaught exception: ${msg}`, 'EXIT-UNCAUGHT-EXCEPTION');
});
process.on('unhandledRejection', (r) => {
  const msg = r instanceof Error ? `${r.name}: ${r.message}` : String(r);
  if (__RUNNING_SERVER) {
    // Slack Socket Mode often emits transient disconnects; do not crash server
    const warn = `[warn] unhandled rejection (continuing): ${msg}`;
    try { process.stderr.write(`${warn}
`); } catch {}
    return;
  }
  exitWith(1, `unhandled rejection: ${msg}`, 'EXIT-UNHANDLED-REJECTION');
});

program.parse();
