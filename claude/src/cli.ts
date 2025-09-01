import fs from 'node:fs';
import os from 'node:os';

import { Command } from 'commander';

import type { AIAgentSessionConfig, LogEntry, AccountingEntry, AIAgentCallbacks, ConversationMessage } from './types.js';

import { AIAgent } from './ai-agent.js';
import { loadConfiguration } from './config.js';

const program = new Command();

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support')
  .version('1.0.0')
  .argument('<providers>', 'Comma-separated list of LLM providers')
  .argument('<models>', 'Comma-separated list of model names')
  .argument('<mcp-tools>', 'Comma-separated list of MCP tools')
  .argument('<system-prompt>', 'System prompt (string, @filename, or - for stdin)')
  .argument('<user-prompt>', 'User prompt (string, @filename, or - for stdin)')
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
  .option('--max-tool-turns <n>', 'Maximum tool turns (agent loop cap)', '30')
  .action(async (providers: string, models: string, mcpTools: string, systemPrompt: string, userPrompt: string, options: Record<string, unknown>) => {
    try {
      if (systemPrompt === '-' && userPrompt === '-') {
        console.error('Error: cannot use stdin ("-") for both system and user prompts');
        process.exit(4);
      }
      
      const providerList = providers.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
      const modelList = models.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
      const toolList = mcpTools.split(',').map((s) => s.trim()).filter((s) => s.length > 0);

      if (providerList.length === 0 || modelList.length === 0 || toolList.length === 0) {
        console.error('Error: providers, models, and mcp-tools are required');
        process.exit(4);
      }

      // Parse numerical options
      const llmTimeout = parsePositiveInt(options.llmTimeout, 'llm-timeout');
      const toolTimeout = parsePositiveInt(options.toolTimeout, 'tool-timeout');
      const temperature = parseFloat(options.temperature, 'temperature', 0, 2);
      const topP = parseFloat(options.topP, 'top-p', 0, 1);
      const maxToolTurns = parsePositiveInt(options.maxToolTurns, 'max-tool-turns');
      const maxRetries = parsePositiveInt(options.maxRetries, 'max-retries');

      const cfgPath = typeof options.config === 'string' && options.config.length > 0 ? options.config : undefined;
      const config = loadConfiguration(cfgPath);

      // Resolve prompts
      const resolvedSystemRaw = await readPrompt(systemPrompt);
      const resolvedUserRaw = await readPrompt(userPrompt);

      // Prompt variable substitution
      const vars = buildPromptVariables(maxToolTurns);
      const resolvedSystem = expandPrompt(resolvedSystemRaw, vars);
      const resolvedUser = expandPrompt(resolvedUserRaw, vars);

      // Load conversation history if specified
      let conversationHistory: ConversationMessage[] | undefined = undefined;
      if (typeof options.load === 'string' && options.load.length > 0) {
        try {
          const content = fs.readFileSync(options.load, 'utf-8');
          conversationHistory = JSON.parse(content) as ConversationMessage[];
        } catch (e) {
          console.error(`Error loading conversation from ${options.load}: ${e instanceof Error ? e.message : String(e)}`);
          process.exit(1);
        }
      }

      // Setup accounting
      const accountingFile = (typeof options.accounting === 'string' && options.accounting.length > 0)
        ? options.accounting
        : config.accounting?.file;

      // Setup logging callbacks
      const callbacks = createCallbacks(options, accountingFile);

      // Create session configuration
      const sessionConfig: AIAgentSessionConfig = {
        config,
        providers: providerList,
        models: modelList,
        tools: toolList,
        systemPrompt: resolvedSystem,
        userPrompt: resolvedUser,
        conversationHistory,
        temperature,
        topP,
        maxRetries,
        maxTurns: maxToolTurns,
        llmTimeout,
        toolTimeout,
        parallelToolCalls: typeof options.parallelToolCalls === 'boolean' ? options.parallelToolCalls : undefined,
        stream: typeof options.stream === 'boolean' ? options.stream : undefined,
        callbacks,
        traceLLM: options.traceLlm === true,
        traceMCP: options.traceMcp === true,
        verbose: options.verbose === true,
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

      // Save conversation if requested
      if (typeof options.save === 'string' && options.save.length > 0) {
        try {
          fs.writeFileSync(options.save, JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (e) {
          console.error(`Error saving conversation to ${options.save}: ${e instanceof Error ? e.message : String(e)}`);
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
  if (value.startsWith('@')) return fs.readFileSync(value.slice(1), 'utf8');
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
