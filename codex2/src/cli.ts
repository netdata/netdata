import fs from 'node:fs';
import os from 'node:os';

import { Command } from 'commander';

import type { AccountingEntry, LogEntry } from './types.js';

import { AIAgent } from './ai-agent.js';
import { loadConfiguration, validateMCPServers, validateProviders, validatePrompts } from './config.js';

const program = new Command();

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support (codex2)')
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
      validatePrompts(systemPrompt, userPrompt);
      const providerList = providers.split(',').map((s) => s.trim()).filter(Boolean);
      const modelList = models.split(',').map((s) => s.trim()).filter(Boolean);
      const toolList = mcpTools.split(',').map((s) => s.trim()).filter(Boolean);
      if (providerList.length === 0 || modelList.length === 0 || toolList.length === 0) { console.error('Error: providers, models, and mcp-tools are required'); process.exit(4); }

      // Parse numeric options
      const llmTimeout = Number.parseInt(String(options.llmTimeout), 10);
      const toolTimeout = Number.parseInt(String(options.toolTimeout), 10);
      const temperature = Number.parseFloat(String(options.temperature));
      const topP = Number.parseFloat(String(options.topP));
      const maxToolTurns = Number.parseInt(String(options.maxToolTurns), 10);
      const maxRetries = Number.parseInt(String(options.maxRetries), 10);
      if (!Number.isFinite(llmTimeout) || llmTimeout <= 0) { console.error('Error: --llm-timeout must be positive'); process.exit(4); }
      if (!Number.isFinite(toolTimeout) || toolTimeout <= 0) { console.error('Error: --tool-timeout must be positive'); process.exit(4); }
      if (!Number.isFinite(temperature) || temperature < 0 || temperature > 2) { console.error('Error: --temperature must be between 0 and 2'); process.exit(4); }
      if (!Number.isFinite(topP) || topP < 0 || topP > 1) { console.error('Error: --top-p must be between 0 and 1'); process.exit(4); }
      if (!Number.isFinite(maxToolTurns) || maxToolTurns <= 0) { console.error('Error: --max-tool-turns must be a positive integer'); process.exit(4); }
      if (!Number.isFinite(maxRetries) || maxRetries <= 0) { console.error('Error: --max-retries must be a positive integer'); process.exit(4); }

      const resolvedConfigPath = typeof options.config === 'string' && options.config.length > 0 ? options.config : undefined;
      const config = loadConfiguration(resolvedConfigPath);
      validateProviders(config, providerList);
      validateMCPServers(config, toolList);

      // Resolve prompts (from file or stdin)
      const resolvePrompt = async (src: string): Promise<string> => {
        if (src === '-') {
          const data = await new Promise<string>((resolve, reject) => {
            try {
              let acc = '';
              process.stdin.setEncoding('utf-8');
              process.stdin.on('data', (c) => { acc += String(c); });
              process.stdin.on('end', () => { resolve(acc); });
            } catch (e) {
              reject(e instanceof Error ? e : new Error('stdin read failed'));
            }
          });
          return data;
        }
        if (src.startsWith('@')) return fs.readFileSync(src.slice(1), 'utf-8');
        return src;
      };
      const systemRaw = await resolvePrompt(systemPrompt);
      const userRaw = await resolvePrompt(userPrompt);

      // Prompt substitutions
      const formatRFC3339Local = (d: Date) => {
        const pad = (n: number, w = 2) => String(n).padStart(w, '0');
        const tz = -d.getTimezoneOffset();
        const sign = tz >= 0 ? '+' : '-';
        const hh = pad(Math.floor(Math.abs(tz) / 60));
        const mm = pad(Math.abs(tz) % 60);
        return String(d.getFullYear()) + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds()) + sign + hh + ':' + mm;
      };
      const detectTimezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
      const detectOS = () => `${os.type()} ${os.release()}`;
      const now = new Date();
      const vars: Record<string, string> = {
        DATETIME: formatRFC3339Local(now),
        DAY: now.toLocaleDateString(undefined, { weekday: 'long' }),
        TIMEZONE: detectTimezone(),
        MAX_TURNS: String(maxToolTurns),
        OS: detectOS(),
        ARCH: process.arch,
        KERNEL: os.type() + ' ' + os.release(),
        CD: process.cwd(),
        HOSTNAME: os.hostname(),
        USER: (() => { try { return os.userInfo().username; } catch { return undefined; } })() ?? (process.env.USER ?? process.env.USERNAME ?? ''),
      };
      const expandVars = (text: string) => text.replace(/\$\{([^}]+)\}/g, (_m: string, n: string) => vars[n] ?? _m);
      const systemResolved = expandVars(systemRaw);
      const userResolved = expandVars(userRaw);

      if (options.dryRun === true) {
        process.stderr.write('\x1b[90m[dry-run] config validated; skipping MCP and LLM calls\x1b[0m\n');
        process.exit(0);
      }

      // Accounting handler
      const accountingFile = typeof options.accounting === 'string' && options.accounting.length > 0 ? options.accounting : config.accounting?.file;
      const onAccounting = (entry: AccountingEntry) => {
        if (typeof accountingFile !== 'string' || accountingFile.length === 0) return;
        try { fs.appendFileSync(accountingFile, JSON.stringify(entry) + '\n', 'utf-8'); } catch (e) { process.stderr.write(`[warn] Failed to write accounting entry: ${e instanceof Error ? e.message : String(e)}\n`); }
      };

      // Logging handler with coloring (stderr only)
      const color = (s: string, code: string) => (process.stderr.isTTY ? code + s + '\x1b[0m' : s);
      const dirSymbol = (d: string) => (d === 'request' ? '→' : '←');
      const showVRB = options.verbose === true;
      const showTRC = (e: LogEntry) => ((e.type === 'llm' && options.traceLlm === true) || (e.type === 'mcp' && options.traceMcp === true));
      const onLog = (entry: LogEntry) => {
        if (entry.severity === 'ERR') {
          process.stderr.write(color('[ERR] ' + dirSymbol(entry.direction) + ' [' + String(entry.turn) + '.' + String(entry.subturn) + '] ' + entry.type + ' ' + entry.remoteIdentifier + ': ' + entry.message + '\n', '\x1b[31m'));
          return;
        }
        if (entry.severity === 'WRN') {
          process.stderr.write(color('[WRN] ' + dirSymbol(entry.direction) + ' [' + String(entry.turn) + '.' + String(entry.subturn) + '] ' + entry.type + ' ' + entry.remoteIdentifier + ': ' + entry.message + '\n', '\x1b[33m'));
          return;
        }
        if (entry.severity === 'VRB' && showVRB) {
          process.stderr.write(color('[VRB] ' + dirSymbol(entry.direction) + ' [' + String(entry.turn) + '.' + String(entry.subturn) + '] ' + entry.type + ' ' + entry.remoteIdentifier + ': ' + entry.message + '\n', '\x1b[90m'));
          return;
        }
        if (entry.severity === 'TRC' && showTRC(entry)) {
          process.stderr.write(color('[TRC] ' + dirSymbol(entry.direction) + ' [' + String(entry.turn) + '.' + String(entry.subturn) + '] ' + entry.type + ' ' + entry.remoteIdentifier + ': ' + entry.message + '\n', '\x1b[90m'));
        }
      };

      // Optional load conversation
      let conversationHistory: { role: string; content: string }[] | undefined;
      if (typeof options.load === 'string' && options.load.length > 0) {
        try { conversationHistory = JSON.parse(fs.readFileSync(options.load, 'utf-8')) as { role: string; content: string }[]; } catch (e) { console.error(`Error loading conversation from ${options.load}: ${e instanceof Error ? e.message : String(e)}`); process.exit(1); }
      }

      const session = await AIAgent.create({
        config, providers: providerList, models: modelList, tools: toolList,
        systemPrompt: systemResolved, userPrompt: userResolved, conversationHistory,
        llmTimeout, toolTimeout, temperature, topP,
        maxRetries, maxToolTurns, parallelToolCalls: options.parallelToolCalls !== false,
        stream: options.stream !== false,
        traceLLM: options.traceLlm === true,
        traceMCP: options.traceMcp === true,
        verbose: options.verbose === true,
        callbacks: { onOutput: (t) => process.stdout.write(t), onAccounting, onLog },
      });

      const result = await session.run();
      if (!result.success) {
        const msg = result.error ?? 'Unknown error';
        process.stderr.write(color('[ERR] ← [fin] llm/mcp: ' + msg + '\n', '\x1b[31m'));
        if (msg.includes('Configuration')) process.exit(1);
        else if (msg.includes('Max tool turns exceeded')) process.exit(5);
        else if (msg.includes('tool')) process.exit(3);
        else process.exit(2);
      }

      if (typeof options.save === 'string' && options.save.length > 0) {
        fs.writeFileSync(options.save, JSON.stringify(result.conversation, null, 2), 'utf-8');
      }
      process.exit(0);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      console.error(`Fatal error: ${msg}`);
      if (msg.includes('config')) process.exit(1);
      else if (msg.includes('argument')) process.exit(4);
      else if (msg.includes('tool')) process.exit(3);
      else process.exit(1);
    }
  });

process.on('uncaughtException', (e) => { console.error(`Uncaught exception: ${e instanceof Error ? e.message : String(e)}`); process.exit(1); });
process.on('unhandledRejection', (r) => { console.error(`Unhandled rejection: ${r instanceof Error ? r.message : String(r)}`); process.exit(1); });

program.parse();
