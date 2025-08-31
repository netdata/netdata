import fs from 'node:fs';
import os from 'node:os';

import { Command } from 'commander';

import type { AIAgentCallbacks, AIAgentOptions, AIAgentRunOptions, AccountingEntry } from './types.js';

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
    let origLog: typeof console.log | undefined;
    let origInfo: typeof console.info | undefined;
    let origWarn: typeof console.warn | undefined;
    let origError: typeof console.error | undefined;
    let suppressed = false;
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

      const llmTimeoutRaw = options.llmTimeout;
      const toolTimeoutRaw = options.toolTimeout;
      const temperatureRaw = options.temperature;
      const topPRaw = options.topP;
      const llmTimeout = typeof llmTimeoutRaw === 'string' ? Number.parseInt(llmTimeoutRaw, 10) : Number(llmTimeoutRaw);
      const toolTimeout = typeof toolTimeoutRaw === 'string' ? Number.parseInt(toolTimeoutRaw, 10) : Number(toolTimeoutRaw);
      const temperature = typeof temperatureRaw === 'string' ? Number.parseFloat(temperatureRaw) : Number(temperatureRaw);
      const topP = typeof topPRaw === 'string' ? Number.parseFloat(topPRaw) : Number(topPRaw);
      const maxToolTurnsRaw = options.maxToolTurns;
      const maxToolTurns = typeof maxToolTurnsRaw === 'string' ? Number.parseInt(maxToolTurnsRaw, 10) : Number(maxToolTurnsRaw);
      const maxRetriesRaw = options.maxRetries;
      const maxRetries = typeof maxRetriesRaw === 'string' ? Number.parseInt(maxRetriesRaw, 10) : Number(maxRetriesRaw);

      if (!Number.isFinite(llmTimeout) || llmTimeout <= 0) { console.error('Error: --llm-timeout must be positive'); process.exit(4); }
      if (!Number.isFinite(toolTimeout) || toolTimeout <= 0) { console.error('Error: --tool-timeout must be positive'); process.exit(4); }
      if (!Number.isFinite(temperature) || temperature < 0 || temperature > 2) { console.error('Error: --temperature must be between 0 and 2'); process.exit(4); }
      if (!Number.isFinite(topP) || topP < 0 || topP > 1) { console.error('Error: --top-p must be between 0 and 1'); process.exit(4); }
      if (!Number.isFinite(maxToolTurns) || maxToolTurns <= 0) { console.error('Error: --max-tool-turns must be a positive integer'); process.exit(4); }
      if (!Number.isFinite(maxRetries) || maxRetries <= 0) { console.error('Error: --max-retries must be a positive integer'); process.exit(4); }

      const cfgPath = typeof options.config === 'string' && options.config.length > 0 ? options.config : undefined;
      const config = loadConfiguration(cfgPath);

      const agentOptions: AIAgentOptions = {
        configPath: cfgPath,
        llmTimeout,
        toolTimeout,
        temperature,
        topP,
        traceLLM: options.traceLlm === true,
        traceMCP: options.traceMcp === true,
        verbose: options.verbose === true,
        stream: typeof (options.stream) === 'boolean' ? (options.stream as boolean) : undefined,
        parallelToolCalls: typeof (options.parallelToolCalls) === 'boolean' ? (options.parallelToolCalls) : undefined,
        maxToolTurns,
        maxRetries,
      };

      // Resolve prompts
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

      const resolvedSystemRaw = await readPrompt(systemPrompt);
      const resolvedUserRaw = await readPrompt(userPrompt);

      // Prompt variable substitution
      function pad2(n: number): string { return n < 10 ? `0${n}` : String(n); }
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
        return `${y}-${m}-${da}T${hh}:${mm}:${ss}${sign}${tzh}:${tzm}`;
      }
      function detectTimezone(): string {
        try { return Intl.DateTimeFormat().resolvedOptions().timeZone ?? (process.env.TZ ?? 'UTC'); } catch { return process.env.TZ ?? 'UTC'; }
      }
      function detectOS(): string {
        try {
          const content = fs.readFileSync('/etc/os-release', 'utf-8');
          const match = content.match(/^PRETTY_NAME=\"?([^\"\n]+)\"?/m);
          if (match && match[1]) return `${match[1]} (kernel ${os.release()})`;
        } catch { /* ignore */ }
        return `${os.type()} ${os.release()}`;
      }
      function expandPrompt(str: string, vars: Record<string, string>): string {
        const replace = (s: string, re: RegExp) => s.replace(re, (_m, name: string) => (name in vars ? vars[name] : _m));
        let out = str;
        out = replace(out, /\$\{([A-Z_]+)\}/g);
        out = replace(out, /\{\{([A-Z_]+)\}\}/g);
        return out;
      }
      const now = new Date();
      const vars: Record<string, string> = {
        DATETIME: formatRFC3339Local(now),
        DAY: now.toLocaleDateString(undefined, { weekday: 'long' }),
        TIMEZONE: detectTimezone(),
        MAX_TURNS: String(maxToolTurns),
        OS: detectOS(),
        ARCH: process.arch,
        KERNEL: `${os.type()} ${os.release()}`,
        CD: process.cwd(),
        HOSTNAME: os.hostname(),
        USER: (() => { try { return os.userInfo().username; } catch { return process.env.USER || process.env.USERNAME || ''; } })(),
      };
      const resolvedSystem = expandPrompt(resolvedSystemRaw, vars);
      const resolvedUser = expandPrompt(resolvedUserRaw, vars);

      let conversationHistory: AIAgentRunOptions['conversationHistory'] | undefined = undefined;
      if (typeof options.load === 'string' && options.load.length > 0) {
        try {
          const loadPath = options.load;
          const content = fs.readFileSync(loadPath, 'utf-8');
          conversationHistory = JSON.parse(content) as AIAgentRunOptions['conversationHistory'];
        } catch (e) {
          console.error(`Error loading conversation from ${typeof options.load === 'string' ? options.load : ''}: ${e instanceof Error ? e.message : String(e)}`);
          process.exit(1);
        }
      }

      const runOptions: AIAgentRunOptions = {
        providers: providerList,
        models: modelList,
        tools: toolList,
        systemPrompt: resolvedSystem,
        userPrompt: resolvedUser,
        conversationHistory,
        dryRun: options.dryRun === true,
      };

      const accountingOpt = options.accounting;
      const accountingFile = (typeof accountingOpt === 'string' && accountingOpt.length > 0)
        ? accountingOpt
        : config.accounting?.file;
      type Level = 'debug' | 'info' | 'warn' | 'error';
      const threshold: Level = options.quiet === true ? 'error' : options.verbose === true ? 'debug' : 'info';
      const order: Record<Level, number> = { debug: 10, info: 20, warn: 30, error: 40 };
      const callbacks: AIAgentCallbacks = {
        onLog: (level, message) => {
          if (order[level as Level] >= order[threshold]) { process.stderr.write(`[${level}] ${message}\n`); }
        },
        onOutput: (text) => { process.stdout.write(text); },
        onAccounting: (entry: AccountingEntry) => {
          if (typeof accountingFile !== 'string' || accountingFile.length === 0) return;
          try {
            fs.appendFileSync(accountingFile, JSON.stringify(entry) + '\n', 'utf-8');
          } catch (e) {
            process.stderr.write(`[warn] Failed to write accounting entry: ${e instanceof Error ? e.message : String(e)}\n`);
          }
        },
      };

      // Suppress noisy provider console logs unless tracing is enabled
      const suppressAvailableTools = options.verbose === true && options.traceMcp !== true && options.traceLlm !== true;
      origLog = console.log;
      origInfo = console.info;
      origWarn = console.warn;
      origError = console.error;
      if (suppressAvailableTools) {
        console.log = (...args: unknown[]) => {
          if (args.some((a) => typeof a === 'string' && ((a as string).includes('Available tools') || (a as string).includes('NO TOOLS AVAILABLE')))) return;
          // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
          (origLog as typeof console.log)(...args as []);
        };
        console.info = (...args: unknown[]) => {
          if (args.some((a) => typeof a === 'string' && ((a as string).includes('Available tools') || (a as string).includes('NO TOOLS AVAILABLE')))) return;
          // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
          (origInfo as typeof console.info)(...args as []);
        };
        console.warn = (...args: unknown[]) => {
          if (args.some((a) => typeof a === 'string' && ((a as string).includes('Available tools') || (a as string).includes('NO TOOLS AVAILABLE')))) return;
          // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
          (origWarn as typeof console.warn)(...args as []);
        };
        console.error = (...args: unknown[]) => {
          if (args.some((a) => typeof a === 'string' && ((a as string).includes('Available tools') || (a as string).includes('NO TOOLS AVAILABLE')))) return;
          // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
          (origError as typeof console.error)(...args as []);
        };
        suppressed = true;
      }
      const agent = new AIAgent({ ...agentOptions, callbacks });
      const result = await agent.run(runOptions);
      if (!result.success) {
        console.error(`Error: ${result.error ?? ''}`);
        if ((result.error ?? '').includes('Configuration')) process.exit(1);
        else if ((result.error ?? '').includes('Max tool turns exceeded')) process.exit(5);
        else if ((result.error ?? '').includes('tool')) process.exit(3);
        else process.exit(2);
      }
      if (typeof options.save === 'string' && options.save.length > 0) {
        try {
          const savePath = options.save;
          fs.writeFileSync(savePath, JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (e) {
          console.error(`Error saving conversation to ${typeof options.save === 'string' ? options.save : ''}: ${e instanceof Error ? e.message : String(e)}`);
          process.exit(1);
        }
      }
      process.exit(0);
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      console.error(`Fatal error: ${msg}`);
      if (msg.includes('config')) process.exit(1);
      else if (msg.includes('argument')) process.exit(4);
      else if (msg.includes('tool')) process.exit(3);
      else process.exit(1);
    } finally {
      if (suppressed) {
        try { if (origLog) console.log = origLog; } catch {}
        try { if (origInfo) console.info = origInfo; } catch {}
        try { if (origWarn) console.warn = origWarn; } catch {}
        try { if (origError) console.error = origError; } catch {}
      }
    }
  });

process.on('uncaughtException', (e) => { console.error(`Uncaught exception: ${e instanceof Error ? e.message : String(e)}`); process.exit(1); });
process.on('unhandledRejection', (r) => { console.error(`Unhandled rejection: ${r instanceof Error ? r.message : String(r)}`); process.exit(1); });

program.parse();
