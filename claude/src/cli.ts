import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { Command } from 'commander';
import * as yaml from 'js-yaml';

import type { LogEntry, AccountingEntry, AIAgentCallbacks, ConversationMessage } from './types.js';

import { loadAgentFromContent } from './agent-loader.js';
import { discoverLayers, resolveDefaults } from './config-resolver.js';
import { parseFrontmatter, stripFrontmatter, parseList, parsePairs, buildFrontmatterTemplate } from './frontmatter.js';
import { formatAgentResultHumanReadable } from './utils.js';

interface FrontmatterOptions {
  llms?: string | string[];
  targets?: string | string[];
  tools?: string | string[];
  load?: string;
  accounting?: string;
  usage?: string;
  parallelToolCalls?: boolean;
  stream?: boolean;
  traceLLM?: boolean;
  traceMCP?: boolean;
  verbose?: boolean;
  save?: string;
  maxToolTurns?: number;
  maxRetries?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  toolResponseMaxBytes?: number;
}

const program = new Command();
const ALIAS_FOR_TARGETS = 'Alias for --targets';
const ALIAS_FOR_TOOLS = 'Alias for --tools';

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
    let configPath: string | undefined = undefined;
    let configDefaults: Record<string, unknown> = {};
    try {
      const layers = discoverLayers({ configPath: undefined });
      configDefaults = resolveDefaults(layers) as Record<string, unknown>;
      configPath = '(layered)';
    } catch {
      // ignore
    }

    // Resolve display values (FM > config.defaults > internal)
    const readNum = (key: keyof FrontmatterOptions, ckey: string, fallback: number): number => {
      const fmv = fmOptions !== undefined ? (fmOptions[key] as unknown) : undefined;
      if (typeof fmv === 'number' && Number.isFinite(fmv)) return fmv;
      const cv = configDefaults[ckey];
      if (typeof cv === 'number' && Number.isFinite(cv)) return cv;
      return fallback;
    };
    const readBool = (key: keyof FrontmatterOptions, ckey: string, fallback: boolean): boolean => {
      const fmv = fmOptions !== undefined ? (fmOptions[key] as unknown) : undefined;
      if (typeof fmv === 'boolean') return fmv;
      const cv = configDefaults[ckey];
      if (typeof cv === 'boolean') return cv;
      return fallback;
    };

    const temperature = readNum('temperature', 'temperature', 0.7);
    const topP = readNum('topP', 'topP', 1.0);
    const llmTimeout = readNum('llmTimeout', 'llmTimeout', 120000);
    const toolTimeout = readNum('toolTimeout', 'toolTimeout', 60000);
    const maxRetries = readNum('maxRetries', 'maxRetries', 3);
    const maxToolTurns = readNum('maxToolTurns', 'maxToolTurns', 10);
    const toolResponseMaxBytes = readNum('toolResponseMaxBytes', 'toolResponseMaxBytes', 12288);
    const stream = readBool('stream', 'stream', false);
    const parallelToolCalls = readBool('parallelToolCalls', 'parallelToolCalls', false);

    const inv = (typeof candidate === 'string' && candidate.length > 0) ? candidate : 'ai-agent';
    const usageText = (() => {
      if (fmUsage.length > 0) return `${inv} "${fmUsage}"`;
      return `${inv} "<user prompt>"`;
    })();

    const traceLLM = readBool('traceLLM', 'traceLLM', false);
    const traceMCP = readBool('traceMCP', 'traceMCP', false);
    const verbose = readBool('verbose', 'verbose', false);
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
      },
      booleans: {
        stream,
        parallelToolCalls,
        traceLLM,
        traceMCP,
        verbose,
      },
      strings: {
        accounting: (fmOptions !== undefined && typeof fmOptions.accounting === 'string') ? fmOptions.accounting : '',
        save: (fmOptions !== undefined && typeof fmOptions.save === 'string') ? fmOptions.save : '',
        load: (fmOptions !== undefined && typeof fmOptions.load === 'string') ? fmOptions.load : '',
      },
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
    if (configPath !== undefined) lines.push(`Config: ${configPath}`);
    lines.push('');
    return `\n${lines.join('\n')}\n`;
  } catch {
    return '';
  }
}

// Inject description and resolved defaults before the standard help
program.addHelpText('before', buildResolvedDefaultsHelp);

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support')
  .version('1.0.0')
  .argument('<system-prompt>', 'System prompt (string, @filename, or - for stdin)')
  .argument('<user-prompt>', 'User prompt (string, @filename, or - for stdin)')
  .option('--targets <list>', 'Comma-separated provider/model pairs (e.g., openai/gpt-4o,ollama/gpt-oss:20b)')
  .option('--llms <list>', ALIAS_FOR_TARGETS)
  .option('--provider-models <list>', ALIAS_FOR_TARGETS)
  .option('--tools <list>', 'Comma-separated list of MCP tools')
  .option('--tool <list>', ALIAS_FOR_TOOLS)
  .option('--mcp <list>', ALIAS_FOR_TOOLS)
  .option('--mcp-tool <list>', ALIAS_FOR_TOOLS)
  .option('--mcp-tools <list>', ALIAS_FOR_TOOLS)
  .option('--agents <list>', 'Comma-separated list of sub-agent .ai files (relative or absolute)')
  .option('--llm-timeout <ms>', 'Timeout for LLM responses (ms)', '120000')
  .option('--tool-timeout <ms>', 'Timeout for tool execution (ms)', '60000')
  .option('--tool-response-max-bytes <n>', 'Maximum MCP tool response size (bytes)', '12288')
  .option('--temperature <n>', 'LLM temperature (0.0-2.0)', '0.7')
  .option('--top-p <n>', 'LLM top-p sampling (0.0-1.0)', '1.0')
  .option('--save <filename>', 'Save conversation to JSON file')
  .option('--load <filename>', 'Load conversation from JSON file')
  .option('--config <filename>', 'Configuration file path')
  .option('--accounting <filename>', 'Override accounting file from config')
  .option('--dry-run', 'Validate config and MCP only, no LLM requests')
  .option('--verbose', 'Enable debug logging to stderr')
  .option('--quiet', 'Only print errors to stderr')
  .option('--trace-llm', 'Log LLM HTTP requests and responses (redacted)')
  .option('--trace-mcp', 'Log MCP requests, responses, and server stderr')
  .option('--stream', 'Enable streaming LLM responses')
  .option('--no-stream', 'Disable streaming; use non-streaming responses')
  .option('--parallel-tool-calls', 'Enable parallel tool calls')
  .option('--no-parallel-tool-calls', 'Disable parallel tool calls')
  .option('--max-retries <n>', 'Max retry rounds over provider/model list', '3')
  .option('--max-tool-turns <n>', 'Maximum tool turns (agent loop cap)', '10')
  .option('--mcp-init-concurrency <n>', 'Max concurrent MCP server initializations', undefined)
  .action(async (systemPrompt: string, userPrompt: string, options: Record<string, unknown>) => {
    try {
      if (systemPrompt === '-' && userPrompt === '-') {
        console.error('Error: cannot use stdin ("-") for both system and user prompts');
        process.exit(4);
      }
      

      const cfgPath = typeof options.config === 'string' && options.config.length > 0 ? options.config : undefined;

      // (moved) numeric options will be resolved after reading prompts/frontmatter

      // Resolve prompts and parse frontmatter for expected output and options
      const resolvedSystemRaw = await readPrompt(systemPrompt);
      const resolvedUserRaw = await readPrompt(userPrompt);
      const sysBaseDir = (() => { try { return (systemPrompt !== '-' && fs.existsSync(systemPrompt) && fs.statSync(systemPrompt).isFile()) ? path.dirname(systemPrompt) : undefined; } catch { return undefined; } })();
      const usrBaseDir = (() => { try { return (userPrompt !== '-' && fs.existsSync(userPrompt) && fs.statSync(userPrompt).isFile()) ? path.dirname(userPrompt) : undefined; } catch { return undefined; } })();
      const fmSys = parseFrontmatter(resolvedSystemRaw, { baseDir: sysBaseDir });
      const fmUsr = parseFrontmatter(resolvedUserRaw, { baseDir: usrBaseDir });
      const fm = fmSys ?? fmUsr;

      const fmOptions: FrontmatterOptions | undefined = fm?.options;
      // Removed local resolution of runtime knobs; agent-loader owns precedence

      // Determine effective targets and tools (CLI > FM)
      const cliTargetsRaw: string | undefined =
        (typeof options.targets === 'string' && options.targets.length > 0) ? options.targets
        : (typeof options.llms === 'string' && options.llms.length > 0) ? options.llms
        : (typeof options.providerModels === 'string' && options.providerModels.length > 0) ? options.providerModels
        : undefined;

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
      const fmTargetsRaw = (fmOptions !== undefined && typeof fmOptions.llms === 'string') ? fmOptions.llms
        : (fmOptions !== undefined && typeof fmOptions.targets === 'string') ? fmOptions.targets
        : (fmOptions !== undefined && Array.isArray(fmOptions.llms)) ? fmOptions.llms.join(',')
        : (fmOptions !== undefined && Array.isArray(fmOptions.targets)) ? fmOptions.targets.join(',')
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
        console.error('Error: No provider/model targets specified. Use --targets or frontmatter llms/targets.');
        process.exit(4);
      }
      // Allow LLM-only runs (no tools/agents). Tool availability rules are enforced later:
      // - MCP tools missing from config: fatal at validation
      // - MCP tools failing at runtime: continue without them
      // - Agents missing: load throws early (fatal)

      // Probe layered config locations (verbose only)
      try {
        const layers = discoverLayers({ configPath: cfgPath });
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

      const loaded = loadAgentFromContent('cli-main', fmSource, {
        configPath: cfgPath,
        verbose: options.verbose === true,
        targets,
        tools: toolList,
        agents: agentsList,
        baseDir: fmBaseDir,
        // CLI overrides take precedence
        temperature: optSrc('temperature') === 'cli' ? Number(options.temperature) : undefined,
        topP: optSrc('topP') === 'cli' ? Number(options.topP) : undefined,
        llmTimeout: optSrc('llmTimeout') === 'cli' ? Number(options.llmTimeout) : undefined,
        toolTimeout: optSrc('toolTimeout') === 'cli' ? Number(options.toolTimeout) : undefined,
        maxRetries: optSrc('maxRetries') === 'cli' ? Number(options.maxRetries) : undefined,
        maxToolTurns: optSrc('maxToolTurns') === 'cli' ? Number(options.maxToolTurns) : undefined,
        toolResponseMaxBytes: optSrc('toolResponseMaxBytes') === 'cli' ? Number(options.toolResponseMaxBytes) : undefined,
        parallelToolCalls: typeof options.parallelToolCalls === 'boolean' ? options.parallelToolCalls : undefined,
        stream: typeof options.stream === 'boolean' ? options.stream : undefined,
        traceLLM: options.traceLlm === true ? true : undefined,
        traceMCP: options.traceMcp === true ? true : undefined,
        mcpInitConcurrency: (typeof options.mcpInitConcurrency === 'string' && options.mcpInitConcurrency.length>0) ? Number(options.mcpInitConcurrency) : undefined,
      });

      // Prompt variable substitution using loader-effective max tool turns
      const vars = buildPromptVariables(loaded.effective.maxToolTurns);
      const resolvedSystem = expandPrompt(stripFrontmatter(resolvedSystemRaw), vars);
      const resolvedUser = expandPrompt(stripFrontmatter(resolvedUserRaw), vars);

// Load conversation history if specified
      let conversationHistory: ConversationMessage[] | undefined = undefined;
      const effLoad = (typeof options.load === 'string' && options.load.length > 0)
        ? options.load
        : (fmOptions !== undefined && typeof fmOptions.load === 'string') ? fmOptions.load
        : undefined;
      if (typeof effLoad === 'string' && effLoad.length > 0) {
        try {
          const content = fs.readFileSync(effLoad, 'utf-8');
          conversationHistory = JSON.parse(content) as ConversationMessage[];
        } catch (e) {
          console.error(`Error loading conversation from ${effLoad}: ${e instanceof Error ? e.message : String(e)}`);
          process.exit(1);
        }
      }

      // Setup accounting
      const accountingFile = (typeof options.accounting === 'string' && options.accounting.length > 0)
        ? options.accounting
        : (fmOptions !== undefined && typeof fmOptions.accounting === 'string') ? fmOptions.accounting
        : loaded.accountingFile ?? loaded.config.accounting?.file;

      // Setup logging callbacks (trace/verbose still taken from CLI/FM above)
      const effectiveTraceLLM = options.traceLlm === true ? true : (fmOptions !== undefined && typeof fmOptions.traceLLM === 'boolean' ? fmOptions.traceLLM : false);
      const effectiveTraceMCP = options.traceMcp === true ? true : (fmOptions !== undefined && typeof fmOptions.traceMCP === 'boolean' ? fmOptions.traceMCP : false);
      const effectiveVerbose = options.verbose === true ? true : (fmOptions !== undefined && typeof fmOptions.verbose === 'boolean' ? fmOptions.verbose : false);
      const callbacks = createCallbacks({ traceLlm: effectiveTraceLLM, traceMcp: effectiveTraceMCP, verbose: effectiveVerbose }, accountingFile);

      if (options.dryRun === true) {
        console.error('Dry run: configuration and MCP servers validated successfully.');
        process.exit(0);
      }

      // Create and run session via unified loader
      const result = await loaded.run(resolvedSystem, resolvedUser, { history: conversationHistory, callbacks });

      // Always print only the formatted human-readable output to stdout
      try {
        const out = formatAgentResultHumanReadable(result);
        process.stdout.write(out);
        if (!out.endsWith('\n')) process.stdout.write('\n');
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        console.error(`Formatting error: ${msg}`);
      }

      if (!result.success) {
        if (options.verbose === true) console.error(`Error: ${result.error ?? ''}`);
        // Exit logs are already emitted by the agent itself via callbacks
        if ((result.error ?? '').includes('Configuration')) process.exit(1);
        else if ((result.error ?? '').includes('Max tool turns exceeded')) process.exit(5);
        else if ((result.error ?? '').includes('tool')) process.exit(3);
        else process.exit(2);
      }

      // Save conversation if requested (CLI > FM)
      const effSave = (typeof options.save === 'string' && options.save.length > 0)
        ? options.save
        : (fmOptions !== undefined && typeof fmOptions.save === 'string') ? fmOptions.save
        : undefined;
      if (typeof effSave === 'string' && effSave.length > 0) {
        try {
          fs.writeFileSync(effSave, JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (e) {
          console.error(`Error saving conversation to ${effSave}: ${e instanceof Error ? e.message : String(e)}`);
          process.exit(1);
        }
      }

      // Successful completion - agent already logged EXIT-FINAL-ANSWER or similar
      process.exit(0);
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      console.error(`Fatal error: ${msg}`);
      // Agent should have already logged exit via callbacks, but log here if it didn't
      if (options.verbose === true && !msg.includes('EXIT-')) {
        const colorize = (text: string, colorCode: string): string => {
          return process.stderr.isTTY ? `${colorCode}${text}\x1b[0m` : text;
        };
        const formatted = colorize(`[VRB] ← [0.0] agent EXIT-UNKNOWN: Unexpected error in CLI: ${msg} (fatal=true)`, '\x1b[90m');
        process.stderr.write(`${formatted}\n`);
      }
      if (msg.includes('config')) process.exit(1);
      else if (msg.includes('argument')) process.exit(4);
      else if (msg.includes('tool')) process.exit(3);
      else process.exit(1);
    }
  });

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

function createCallbacks(options: Record<string, unknown>, accountingFile?: string): AIAgentCallbacks {

  // Helper for consistent coloring - MANDATORY in TTY according to DESIGN.md
  const colorize = (text: string, colorCode: string): string => {
    return process.stderr.isTTY ? `${colorCode}${text}\x1b[0m` : text;
  };
  
  // Helper for direction symbols to save space
  const dirSymbol = (direction: string): string => direction === 'request' ? '→' : '←';

  // Track thinking stream state to ensure proper newlines between logs
  let thinkingOpen = false;
  let lastCharWasNewline = true;

      return {
        onLog: (entry: LogEntry) => {
      // Ensure newline separation after thinking stream if needed
      if (entry.severity !== 'THK' && thinkingOpen && !lastCharWasNewline) {
        try { process.stderr.write('\n'); } catch {}
        lastCharWasNewline = true;
        thinkingOpen = false;
      }
      // Always show errors and warnings with colors
      const agentLabel = (() => {
        if (typeof entry.agentId === 'string' && entry.agentId.length > 0) {
          try { return ` (agent: ${path.basename(entry.agentId)})`; } catch { return ` (agent: ${entry.agentId})`; }
        }
        return '';
      })();
      if (entry.severity === 'ERR') {
        const formatted = colorize(`[ERR] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: ${entry.message}`, '\x1b[31m');
        process.stderr.write(`${formatted}\n`);
      }
      
      if (entry.severity === 'WRN') {
        const formatted = colorize(`[WRN] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: ${entry.message}`, '\x1b[33m');
        process.stderr.write(`${formatted}\n`);
      }
      
      // Show verbose only with --verbose flag (dark gray)
      if (entry.severity === 'VRB' && options.verbose === true) {
        const formatted = colorize(`[VRB] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: ${entry.message}`, '\x1b[90m');
        process.stderr.write(`${formatted}\n`);
      }

      // Final summary entries
      if (entry.severity === 'FIN') {
        const formatted = colorize(`[FIN] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: ${entry.message}`, '\x1b[36m');
        process.stderr.write(`${formatted}\n`);
      }
      
      // Show trace only with specific flags (dark gray)
      if (entry.severity === 'TRC') {
        if ((entry.type === 'llm' && options.traceLlm === true) || 
            (entry.type === 'mcp' && options.traceMcp === true)) {
          const formatted = colorize(`[TRC] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: ${entry.message}`, '\x1b[90m');
          process.stderr.write(`${formatted}\n`);
        }
      }
      
      // Show thinking header with light gray color
      if (entry.severity === 'THK') {
        const formatted = colorize(`[THK] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}${agentLabel}: `, '\x1b[2;37m');
        process.stderr.write(formatted);
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
    
    onAccounting: (entry: AccountingEntry) => {
      if (typeof accountingFile !== 'string' || accountingFile.length === 0) return;
      try {
        fs.appendFileSync(accountingFile, JSON.stringify(entry) + '\n', 'utf-8');
      } catch (e) {
        const message = colorize(`[warn] Failed to write accounting entry: ${e instanceof Error ? e.message : String(e)}`, '\x1b[33m');
        process.stderr.write(`${message}\n`);
      }
    },
  };
}

process.on('uncaughtException', (e) => { console.error(`Uncaught exception: ${e instanceof Error ? e.message : String(e)}`); process.exit(1); });
process.on('unhandledRejection', (r) => { console.error(`Unhandled rejection: ${r instanceof Error ? r.message : String(r)}`); process.exit(1); });

program.parse();
