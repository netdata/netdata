import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { Command } from 'commander';
import * as yaml from 'js-yaml';

import type { AIAgentSessionConfig, LogEntry, AccountingEntry, AIAgentCallbacks, ConversationMessage } from './types.js';

import { AIAgent } from './ai-agent.js';
import { loadConfiguration, resolveConfigPath } from './config.js';

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
    if (promptPath !== undefined) {
      let content = fs.readFileSync(promptPath, 'utf8');
      if (content.startsWith('#!')) {
        const nl = content.indexOf('\n');
        content = nl >= 0 ? content.slice(nl + 1) : '';
      }
      const fm = parseFrontmatter(content);
      if (fm?.options !== undefined) fmOptions = fm.options;
      if (typeof fm?.description === 'string') fmDesc = fm.description.trim();
      const fmUse = fm?.usage;
      if (typeof fmUse === 'string' && fmUse.trim().length > 0) {
        const inv = typeof candidate === 'string' && candidate.length > 0 ? candidate : 'ai-agent';
        // Add usage headline before defaults
        const usageLine = `Usage: ${inv} "${fmUse.trim()}"`;
        // Temporarily stash in description prefix for printing
        fmDesc = (fmDesc.length > 0 ? `${fmDesc}\n` : '') + usageLine;
      }
    }

    // Load configuration (best effort)
    let configPath: string | undefined = undefined;
    let configDefaults: Record<string, unknown> = {};
    try {
      const cfg = loadConfiguration(undefined);
      configDefaults = cfg.defaults ?? {};
      configPath = '.ai-agent.json'; // resolved by loader; exact path not returned, print standard name
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
    const stream = readBool('stream', 'stream', false);
    const parallelToolCalls = readBool('parallelToolCalls', 'parallelToolCalls', false);

    const inv = (typeof candidate === 'string' && candidate.length > 0) ? candidate : 'ai-agent';
    const usageText = (() => {
      if (fmOptions !== undefined && typeof fmOptions.usage === 'string') {
        const u = fmOptions.usage.trim();
        if (u.length > 0) return `${inv} "${u}"`;
      }
      return `${inv} "<user prompt>"`;
    })();

    // Build a frontmatter template object mirroring accepted keys exactly
    const toArray = (v: unknown): string[] => {
      if (Array.isArray(v)) return v.map((x) => String(x));
      if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
      return [];
    };
    const llmsKey: 'llms' | 'targets' = (fmOptions !== undefined && (Object.prototype.hasOwnProperty.call(fmOptions, 'llms'))) ? 'llms'
      : ((fmOptions !== undefined && (Object.prototype.hasOwnProperty.call(fmOptions, 'targets'))) ? 'targets' : 'llms');
    const llmsVal: string[] = (fmOptions !== undefined && llmsKey === 'llms') ? toArray(fmOptions.llms)
      : (fmOptions !== undefined && llmsKey === 'targets') ? toArray(fmOptions.targets)
      : [];
    const toolsVal: string[] = (fmOptions !== undefined) ? toArray(fmOptions.tools) : [];

    const fmTemplate: Record<string, unknown> = {};
    fmTemplate.description = fmDesc;
    fmTemplate.usage = (fmOptions !== undefined && typeof fmOptions.usage === 'string') ? fmOptions.usage : '';
    fmTemplate[llmsKey] = llmsVal;
    fmTemplate.tools = toolsVal;
    fmTemplate.temperature = temperature;
    fmTemplate.topP = topP;
    fmTemplate.llmTimeout = llmTimeout;
    fmTemplate.toolTimeout = toolTimeout;
    fmTemplate.maxRetries = maxRetries;
    fmTemplate.maxToolTurns = maxToolTurns;
    fmTemplate.stream = stream;
    fmTemplate.parallelToolCalls = parallelToolCalls;
    const traceLLM = readBool('traceLLM', 'traceLLM', false);
    const traceMCP = readBool('traceMCP', 'traceMCP', false);
    const verbose = readBool('verbose', 'verbose', false);
    fmTemplate.traceLLM = traceLLM;
    fmTemplate.traceMCP = traceMCP;
    fmTemplate.verbose = verbose;
    fmTemplate.accounting = (fmOptions !== undefined && typeof fmOptions.accounting === 'string') ? fmOptions.accounting : '';
    fmTemplate.save = (fmOptions !== undefined && typeof fmOptions.save === 'string') ? fmOptions.save : '';
    fmTemplate.load = (fmOptions !== undefined && typeof fmOptions.load === 'string') ? fmOptions.load : '';
    // Include output if present in frontmatter
    try {
      const contentForOutput = promptPath !== undefined ? fs.readFileSync(promptPath, 'utf8') : undefined;
      if (contentForOutput !== undefined) {
        let c = contentForOutput;
        if (c.startsWith('#!')) { const nl = c.indexOf('\n'); c = nl >= 0 ? c.slice(nl + 1) : ''; }
        const parsed = parseFrontmatter(c);
        if (parsed?.expectedOutput !== undefined) {
          const out: Record<string, unknown> = { format: parsed.expectedOutput.format };
          if (parsed.expectedOutput.schema !== undefined) out.schema = parsed.expectedOutput.schema;
          fmTemplate.output = out;
        }
      }
    } catch { /* ignore */ }

    const lines: string[] = [];
    lines.push('DESCRIPTION');
    if (fmDesc.length > 0) lines.push(fmDesc);
    lines.push('');
    lines.push('Usage:');
    lines.push(`   ${usageText}`);
    lines.push('');
    lines.push('Frontmatter Template:');
    lines.push('---');
    const yamlText = yaml.dump(fmTemplate, { lineWidth: 120 });
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
  .option('--llm-timeout <ms>', 'Timeout for LLM responses (ms)', '120000')
  .option('--tool-timeout <ms>', 'Timeout for tool execution (ms)', '60000')
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
      const fm = parseFrontmatter(resolvedSystemRaw) ?? parseFrontmatter(resolvedUserRaw);

      // Resolve numeric options with precedence: CLI > FM > config.defaults > hard default
      const srcMaxToolTurns = program.getOptionValueSource('maxToolTurns');
      const srcMaxRetries = program.getOptionValueSource('maxRetries');
      const srcLlmTimeout = program.getOptionValueSource('llmTimeout');
      const srcToolTimeout = program.getOptionValueSource('toolTimeout');
      const srcTemperature = program.getOptionValueSource('temperature');
      const srcTopP = program.getOptionValueSource('topP');

      const fmOptions: FrontmatterOptions | undefined = fm?.options;

      const maxToolTurns = srcMaxToolTurns === 'cli'
        ? parsePositiveInt(options.maxToolTurns, 'max-tool-turns')
        : (() => { const n = readFmNumber(fmOptions, 'maxToolTurns'); if (n !== undefined) { if (!(n > 0)) { console.error('Error: frontmatter maxToolTurns must be positive'); process.exit(4);} return Math.trunc(n); } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.maxToolTurns; return typeof v === 'number' ? v : 10; } catch { return 10; } })()); })();

      const maxRetries = srcMaxRetries === 'cli'
        ? parsePositiveInt(options.maxRetries, 'max-retries')
        : (() => { const n = readFmNumber(fmOptions, 'maxRetries'); if (n !== undefined) { if (!(n > 0)) { console.error('Error: frontmatter maxRetries must be positive'); process.exit(4);} return Math.trunc(n); } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.maxRetries; return typeof v === 'number' ? v : 3; } catch { return 3; } })()); })();

      const llmTimeout = srcLlmTimeout === 'cli'
        ? parsePositiveInt(options.llmTimeout, 'llm-timeout')
        : (() => { const n = readFmNumber(fmOptions, 'llmTimeout'); if (n !== undefined) { if (!(n > 0)) { console.error('Error: frontmatter llmTimeout must be positive'); process.exit(4);} return Math.trunc(n); } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.llmTimeout; return typeof v === 'number' ? v : 120000; } catch { return 120000; } })()); })();

      const toolTimeout = srcToolTimeout === 'cli'
        ? parsePositiveInt(options.toolTimeout, 'tool-timeout')
        : (() => { const n = readFmNumber(fmOptions, 'toolTimeout'); if (n !== undefined) { if (!(n > 0)) { console.error('Error: frontmatter toolTimeout must be positive'); process.exit(4);} return Math.trunc(n); } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.toolTimeout; return typeof v === 'number' ? v : 60000; } catch { return 60000; } })()); })();

      const temperature = srcTemperature === 'cli'
        ? parseFloat(options.temperature, 'temperature', 0, 2)
        : (() => { const n = readFmNumber(fmOptions, 'temperature'); if (n !== undefined) { if (!(n >= 0 && n <= 2)) { console.error('Error: frontmatter temperature must be between 0 and 2'); process.exit(4);} return n; } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.temperature; return typeof v === 'number' ? v : 0.7; } catch { return 0.7; } })()); })();

      const topP = srcTopP === 'cli'
        ? parseFloat(options.topP, 'top-p', 0, 1)
        : (() => { const n = readFmNumber(fmOptions, 'topP'); if (n !== undefined) { if (!(n >= 0 && n <= 1)) { console.error('Error: frontmatter topP must be between 0 and 1'); process.exit(4);} return n; } return (0 + ((): number => { try { const p = resolveConfigPath(cfgPath); const j = JSON.parse(fs.readFileSync(p, 'utf-8')) as { defaults?: Record<string, unknown> }; const v = j.defaults?.topP; return typeof v === 'number' ? v : 1.0; } catch { return 1.0; } })()); })();

      // Prompt variable substitution
      const vars = buildPromptVariables(maxToolTurns);
      const resolvedSystem = expandPrompt(stripFrontmatter(resolvedSystemRaw), vars);
      const resolvedUser = expandPrompt(stripFrontmatter(resolvedUserRaw), vars);

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

      // fmOptions already computed above
      const fmTargetsRaw = (fmOptions !== undefined && typeof fmOptions.llms === 'string') ? fmOptions.llms
        : (fmOptions !== undefined && typeof fmOptions.targets === 'string') ? fmOptions.targets
        : (fmOptions !== undefined && Array.isArray(fmOptions.llms)) ? fmOptions.llms.join(',')
        : (fmOptions !== undefined && Array.isArray(fmOptions.targets)) ? fmOptions.targets.join(',')
        : undefined;

      const fmToolsRaw = (fmOptions !== undefined && typeof fmOptions.tools === 'string') ? fmOptions.tools
        : (fmOptions !== undefined && Array.isArray(fmOptions.tools)) ? fmOptions.tools.join(',')
        : undefined;

      const targets = parsePairs(cliTargetsRaw ?? fmTargetsRaw);
      const toolList = parseList(cliToolsRaw ?? fmToolsRaw);

      if (targets.length === 0) {
        console.error('Error: No provider/model targets specified. Use --targets or frontmatter llms/targets.');
        process.exit(4);
      }
      if (toolList.length === 0) {
        console.error('Error: No MCP tools specified. Use --tools or frontmatter tools.');
        process.exit(4);
      }

      // Inject missing env vars from sidecar .ai-agent.env for required providers and tools
      const cfgResolved = resolveConfigPath(cfgPath);
      let rawCfg: { providers?: Record<string, unknown>; mcpServers?: Record<string, unknown> } = {};
      try { rawCfg = JSON.parse(fs.readFileSync(cfgResolved, 'utf-8')) as typeof rawCfg; } catch (e) {
        console.error(`Error: Failed to read/parse configuration file at ${cfgResolved}: ${e instanceof Error ? e.message : String(e)}`);
        process.exit(1);
      }
      const needProviders = Array.from(new Set(targets.map((t) => t.provider)));
      const needServers = toolList;
      const unresolved = new Set<string>();
      const scan = (val: unknown): void => {
        if (typeof val === 'string') {
          const matches = Array.from(val.matchAll(/\$\{([^}]+)\}/g));
          matches.forEach((mm) => {
            const name = mm[1];
            if (process.env[name] === undefined) unresolved.add(name);
          });
        } else if (Array.isArray(val)) { val.forEach(scan); }
        else if (val !== null && typeof val === 'object') { Object.values(val).forEach((x) => { scan(x); }); }
      };
      const providersObj: Record<string, unknown> | undefined = rawCfg.providers;
      if (providersObj !== undefined) {
        needProviders.forEach((p) => { const v = providersObj[p]; if (v !== undefined && typeof v === 'object') scan(v); });
      }
      const serversObj: Record<string, unknown> | undefined = rawCfg.mcpServers;
      if (serversObj !== undefined) {
        needServers.forEach((s) => { const v = serversObj[s]; if (v !== undefined && typeof v === 'object') scan(v); });
      }
      if (unresolved.size > 0) {
        const envPath = path.join(path.dirname(cfgResolved), '.ai-agent.env');
        const envMap: Record<string, string> = {};
        if (fs.existsSync(envPath)) {
          try {
            fs.readFileSync(envPath, 'utf-8').split(/\r?\n/).forEach((line) => {
              const t = line.trim();
              if (t.length === 0) return; if (t.startsWith('#')) return;
              const eq = t.indexOf('='); if (eq <= 0) return;
              const key = t.slice(0, eq).trim(); let val = t.slice(eq + 1).trim();
              if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) val = val.slice(1, -1);
              if (key.length > 0) envMap[key] = val;
            });
          } catch (e) {
            console.error(`Error: Failed to read .ai-agent.env at ${envPath}: ${e instanceof Error ? e.message : String(e)}`);
            process.exit(1);
          }
        }
        Array.from(unresolved).forEach((name) => {
          const ev = envMap[name];
          if (process.env[name] === undefined && typeof ev === 'string') {
            process.env[name] = ev;
          }
        });
        const still = Array.from(unresolved).filter((n) => { const v = process.env[n]; return v === undefined; });
        if (still.length > 0) {
          console.error(`Error: Missing required environment variables: ${still.join(', ')}. Define them in your shell or in ${path.join(path.dirname(cfgResolved), '.ai-agent.env')}`);
          process.exit(1);
        }
      }

      // Load full configuration (now that env is hydrated)
      const config = loadConfiguration(cfgPath);

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
        : config.accounting?.file;

      // Setup logging callbacks
      // Resolve additional booleans with precedence CLI > FM > config.defaults
      const effectiveParallelToolCalls = (typeof options.parallelToolCalls === 'boolean')
        ? options.parallelToolCalls
        : (fmOptions !== undefined && typeof fmOptions.parallelToolCalls === 'boolean')
          ? fmOptions.parallelToolCalls
          : (config.defaults?.parallelToolCalls ?? false);

      const effectiveStream = (typeof options.stream === 'boolean')
        ? options.stream
        : (fmOptions !== undefined && typeof fmOptions.stream === 'boolean')
          ? fmOptions.stream
          : (config.defaults?.stream ?? false);

      const effectiveTraceLLM = options.traceLlm === true ? true : (fmOptions !== undefined && typeof fmOptions.traceLLM === 'boolean' ? fmOptions.traceLLM : false);
      const effectiveTraceMCP = options.traceMcp === true ? true : (fmOptions !== undefined && typeof fmOptions.traceMCP === 'boolean' ? fmOptions.traceMCP : false);
      const effectiveVerbose = options.verbose === true ? true : (fmOptions !== undefined && typeof fmOptions.verbose === 'boolean' ? fmOptions.verbose : false);

      const callbacks = createCallbacks({ traceLlm: effectiveTraceLLM, traceMcp: effectiveTraceMCP, verbose: effectiveVerbose }, accountingFile);

      const sessionConfig: AIAgentSessionConfig = {
        config,
        targets,
        tools: toolList,
        systemPrompt: resolvedSystem,
        userPrompt: resolvedUser,
        expectedOutput: fm?.expectedOutput,
        conversationHistory,
        temperature,
        topP,
        maxRetries,
        maxTurns: maxToolTurns,
        llmTimeout,
        toolTimeout,
        parallelToolCalls: effectiveParallelToolCalls,
        stream: effectiveStream,
        callbacks,
        traceLLM: effectiveTraceLLM,
        traceMCP: effectiveTraceMCP,
        verbose: effectiveVerbose,
      };

      if (options.dryRun === true) {
        console.error('Dry run: configuration and MCP servers validated successfully.');
        process.exit(0);
      }

      // Create and run session
      const session = AIAgent.create(sessionConfig);
      const result = await session.run();

      if (!result.success) {
        console.error(`Error: ${result.error ?? ''}`);
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
function parsePositiveInt(value: unknown, name: string): number {
  const num = typeof value === 'string' ? Number.parseInt(value, 10) : Number(value);
  if (!Number.isFinite(num) || num <= 0) {
    console.error(`Error: --${name} must be positive`);
    process.exit(4);
  }
  return num;
}

function parseFloat(value: unknown, name: string, min: number, max: number): number {
  const num = typeof value === 'string' ? Number.parseFloat(value) : Number(value);
  if (!Number.isFinite(num) || num < min || num > max) {
    console.error(`Error: --${name} must be between ${String(min)} and ${String(max)}`);
    process.exit(4);
  }
  return num;
}

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

// Frontmatter parsing utilities
function parseFrontmatter(src: string): { expectedOutput?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> }, options?: FrontmatterOptions, description?: string, usage?: string } | undefined {
  const m = /^---\n([\s\S]*?)\n---\n/.exec(src);
  if (m === null) return undefined;
  try {
    const rawUnknown: unknown = yaml.load(m[1]);
    if (typeof rawUnknown !== 'object' || rawUnknown === null) return undefined;
    const docObj = rawUnknown as { output?: { format?: string; schema?: unknown; schemaRef?: string } } & Record<string, unknown>;
    let expectedOutput: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> } | undefined;
    let description: string | undefined;
    let usage: string | undefined;
    if (docObj.output !== undefined && typeof docObj.output.format === 'string') {
      const format = docObj.output.format.toLowerCase();
      if (format === 'json' || format === 'markdown' || format === 'text') {
        let schemaObj: Record<string, unknown> | undefined;
        if (format === 'json') {
          const s: unknown = docObj.output.schema;
          const refVal: unknown = (docObj.output as { schemaRef?: unknown }).schemaRef;
          const ref: string | undefined = typeof refVal === 'string' ? refVal : undefined;
          schemaObj = loadSchemaValue(s, ref);
        }
        const fmt: 'json'|'markdown'|'text' = format === 'json' ? 'json' : (format === 'markdown' ? 'markdown' : 'text');
        expectedOutput = { format: fmt, schema: schemaObj };
      }
    }
    const raw = rawUnknown as Record<string, unknown>;
    const options: FrontmatterOptions = {};
    if (typeof raw.llms === 'string' || Array.isArray(raw.llms)) options.llms = raw.llms as (string | string[]);
    if (typeof raw.targets === 'string' || Array.isArray(raw.targets)) options.targets = raw.targets as (string | string[]);
    if (typeof raw.tools === 'string' || Array.isArray(raw.tools)) options.tools = raw.tools as (string | string[]);
    if (typeof raw.load === 'string') options.load = raw.load;
    if (typeof raw.accounting === 'string') options.accounting = raw.accounting;
    if (typeof raw.parallelToolCalls === 'boolean') options.parallelToolCalls = raw.parallelToolCalls;
    if (typeof raw.stream === 'boolean') options.stream = raw.stream;
    if (typeof raw.traceLLM === 'boolean') options.traceLLM = raw.traceLLM;
    if (typeof raw.traceMCP === 'boolean') options.traceMCP = raw.traceMCP;
    if (typeof raw.verbose === 'boolean') options.verbose = raw.verbose;
    if (typeof raw.save === 'string') options.save = raw.save;
    if (typeof raw.maxToolTurns === 'number') options.maxToolTurns = raw.maxToolTurns;
    if (typeof raw.maxRetries === 'number') options.maxRetries = raw.maxRetries;
    if (typeof raw.llmTimeout === 'number') options.llmTimeout = raw.llmTimeout;
    if (typeof raw.toolTimeout === 'number') options.toolTimeout = raw.toolTimeout;
    if (typeof raw.temperature === 'number') options.temperature = raw.temperature;
    if (typeof raw.topP === 'number') options.topP = raw.topP;
    if (typeof raw.description === 'string') description = raw.description;
    if (typeof raw.usage === 'string') usage = raw.usage;
    return { expectedOutput, options, description, usage };
  } catch {
    return undefined;
  }
}

function loadSchemaValue(v: unknown, schemaRef?: string): Record<string, unknown> | undefined {
  try {
    if (v !== null && v !== undefined && typeof v === 'object') return v as Record<string, unknown>;
    if (typeof v === 'string') {
      // Try JSON first, then YAML
      try { return JSON.parse(v) as Record<string, unknown>; } catch { /* ignore */ }
      try { return yaml.load(v) as Record<string, unknown>; } catch { /* ignore */ }
    }
    if (typeof schemaRef === 'string' && schemaRef.length > 0) {
      try {
        const path = schemaRef.startsWith('.') ? schemaRef : `./${schemaRef}`;
        const content = fs.readFileSync(path, 'utf-8');
        if (/\.json$/i.test(path)) return JSON.parse(content) as Record<string, unknown>;
        if (/\.(ya?ml)$/i.test(path)) return yaml.load(content) as Record<string, unknown>;
        // Fallback: attempt JSON then YAML
        try { return JSON.parse(content) as Record<string, unknown>; } catch { /* ignore */ }
        return yaml.load(content) as Record<string, unknown>;
      } catch {
        return undefined;
      }
    }
  } catch { /* ignore */ }
  return undefined;
}

function stripFrontmatter(src: string): string {
  const m = /^---\n([\s\S]*?)\n---\n/;
  return src.replace(m, '');
}

function readFmNumber(opts: FrontmatterOptions | undefined, key: keyof FrontmatterOptions): number | undefined {
  if (opts === undefined) return undefined;
  const v = opts[key];
  if (v === undefined) return undefined;
  const n = Number(v);
  if (!Number.isFinite(n)) {
    console.error(`Error: frontmatter ${key} must be a number`);
    process.exit(4);
  }
  return n;
}

function parseList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((s) => (typeof s === 'string' ? s : String(s))).map((s) => s.trim()).filter((s) => s.length > 0);
  if (typeof value === 'string') return value.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
  return [];
}

function parsePairs(value: unknown): { provider: string; model: string }[] {
  const arr = Array.isArray(value) ? value.map((v) => (typeof v === 'string' ? v : String(v))) : (typeof value === 'string' ? value.split(',') : []);
  return arr
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
    .map((token) => {
      const slash = token.indexOf('/');
      if (slash <= 0 || slash >= token.length - 1) {
        console.error(`Error: invalid provider/model pair '${token}'. Expected format provider/model.`);
        process.exit(4);
      }
      const provider = token.slice(0, slash).trim();
      const model = token.slice(slash + 1).trim();
      if (provider.length === 0 || model.length === 0) {
        console.error(`Error: invalid provider/model pair '${token}'.`);
        process.exit(4);
      }
      return { provider, model };
    });
}

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
      if (entry.severity === 'ERR') {
        const formatted = colorize(`[ERR] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[31m');
        process.stderr.write(`${formatted}\n`);
      }
      
      if (entry.severity === 'WRN') {
        const formatted = colorize(`[WRN] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[33m');
        process.stderr.write(`${formatted}\n`);
      }
      
      // Show verbose only with --verbose flag (dark gray)
      if (entry.severity === 'VRB' && options.verbose === true) {
        const formatted = colorize(`[VRB] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[90m');
        process.stderr.write(`${formatted}\n`);
      }

      // Final summary entries
      if (entry.severity === 'FIN') {
        const formatted = colorize(`[FIN] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[36m');
        process.stderr.write(`${formatted}\n`);
      }
      
      // Show trace only with specific flags (dark gray)
      if (entry.severity === 'TRC') {
        if ((entry.type === 'llm' && options.traceLlm === true) || 
            (entry.type === 'mcp' && options.traceMcp === true)) {
          const formatted = colorize(`[TRC] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[90m');
          process.stderr.write(`${formatted}\n`);
        }
      }
      
      // Show thinking header with light gray color
      if (entry.severity === 'THK') {
        const formatted = colorize(`[THK] ${dirSymbol(entry.direction)} [${String(entry.turn)}.${String(entry.subturn)}] ${entry.type} ${entry.remoteIdentifier}: `, '\x1b[2;37m');
        process.stderr.write(formatted);
        thinkingOpen = true;
        lastCharWasNewline = false;
        // The actual thinking text will follow via onThinking
      }
    },
    
    onOutput: (text: string) => { process.stdout.write(text); },
    
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
