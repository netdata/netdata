import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { Command, Option } from 'commander';
import * as yaml from 'js-yaml';

import type { LoadAgentOptions } from './agent-loader.js';
import type { FrontmatterOptions } from './frontmatter.js';
import type { McpTransportSpec } from './headends/mcp-headend.js';
import type { Headend, HeadendLogSink } from './headends/types.js';
import type { LogEntry, AccountingEntry, AIAgentCallbacks, ConversationMessage, Configuration, MCPTool, ProviderReasoningValue, ReasoningLevel, TelemetryLogExtra, TelemetryLogFormat, TelemetryTraceSampler } from './types.js';
import type { CommanderError } from 'commander';

import { AgentRegistry } from './agent-registry.js';
import { buildUnifiedConfiguration, discoverLayers, resolveDefaults } from './config-resolver.js';
import { formatPromptValue, resolveFormatIdForCli } from './formats.js';
import { parseFrontmatter, stripFrontmatter, parseList, parsePairs, buildFrontmatterTemplate } from './frontmatter.js';
import { AnthropicCompletionsHeadend } from './headends/anthropic-completions-headend.js';
import { HeadendManager } from './headends/headend-manager.js';
import { McpHeadend } from './headends/mcp-headend.js';
import { OpenAICompletionsHeadend } from './headends/openai-completions-headend.js';
import { RestHeadend } from './headends/rest-headend.js';
import { SlackHeadend } from './headends/slack-headend.js';
import { resolveIncludes } from './include-resolver.js';
import { formatLog } from './log-formatter.js';
import { makeTTYLogCallbacks } from './log-sink-tty.js';
import { getOptionsByGroup, formatCliNames, OPTIONS_REGISTRY } from './options-registry.js';
import { mergeCallbacksWithPersistence } from './persistence.js';
import { ShutdownController } from './shutdown-controller.js';
import { initTelemetry, shutdownTelemetry } from './telemetry/index.js';
import { buildTelemetryRuntimeConfig, type TelemetryOverrides } from './telemetry/runtime-config.js';
import { MCPProvider, shutdownSharedRegistry } from './tools/mcp-provider.js';
import { normalizeSchemaDraftTarget, schemaDraftDisplayName, validateSchemaAgainstDraft, type SchemaDraftTarget } from './utils/schema-validation.js';
// eslint-disable-next-line perfectionist/sort-imports
import { formatAgentResultHumanReadable, setWarningSink, warn } from './utils.js';
import { VERSION } from './version.generated.js';
import './setup-undici.js';

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
    process.exit(code);
  }
  // TypeScript: inform this never returns
  throw new Error('unreachable');
}

let telemetryInitialized = false;

const markTelemetryInitialized = (): void => {
  telemetryInitialized = true;
};

const ensureTelemetryShutdown = async (): Promise<void> => {
  if (!telemetryInitialized) return;
  telemetryInitialized = false;
  try {
    await shutdownTelemetry();
  } catch { /* ignore telemetry shutdown errors */ }
};

const exitAndShutdown = async (code: number, reason: string, tag: string): Promise<never> => {
  try {
    await shutdownController.shutdown();
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    try { process.stderr.write(`[warn] shutdown controller failed: ${message}\n`); } catch { /* ignore */ }
  }
  await ensureTelemetryShutdown();
  exitWith(code, reason, tag);
  throw new Error('unreachable');
};

const program = new Command();
const defaultWarningSink = (message: string): void => {
  const prefix = '[warn] ';
  const colored = process.stderr.isTTY ? `\x1b[33m${prefix}${message}\x1b[0m` : `${prefix}${message}`;
  try { process.stderr.write(`${colored}\n`); } catch { /* ignore */ }
};

setWarningSink(defaultWarningSink);

const EXIT_REASONING_TOKENS = 'EXIT-CLI-REASONING-TOKENS';
const REASONING_TOKENS_OPTION = 'reasoningTokens';
const REASONING_TOKENS_ALT_OPTION = 'reasoning-tokens';

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
    if (key === 'maxRetries' || key === 'maxToolTurns' || key === 'topP' || key === 'temperature') return ' <n>';
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
      const optInstance = new Option(combined, def.description);
      if (def.type === 'string[]') {
        optInstance.argParser((value: string, previous?: string[]) => appendValue(value, previous)).default(def.default ?? [], undefined);
      }
      prog.addOption(optInstance);
    }
  });
}

const appendValue = <T>(value: T, previous?: T[]): T[] => {
  const base = Array.isArray(previous) ? [...previous] : [];
  base.push(value);
  return base;
};

const normalizeListOption = (value: unknown): string | undefined => {
  if (Array.isArray(value)) {
    const filtered = value
      .map((v) => (typeof v === 'string' ? v : String(v)))
      .map((v) => v.trim())
      .filter((v) => v.length > 0);
    if (filtered.length === 0) return undefined;
    return filtered.join(',');
  }
  if (typeof value === 'string' && value.trim().length > 0) {
    return value.trim();
  }
  return undefined;
};

const parseTelemetryLabelPairs = (raw: unknown): Record<string, string> => {
  const entries = Array.isArray(raw) ? raw : typeof raw === 'string' ? [raw] : [];
  return entries.reduce<Record<string, string>>((acc, entry) => {
    if (typeof entry !== 'string') return acc;
    const trimmed = entry.trim();
    if (trimmed.length === 0) return acc;
    const eq = trimmed.indexOf('=');
    if (eq <= 0) {
      warn(`Invalid telemetry label '${trimmed}', expected key=value`);
      return acc;
    }
    const key = trimmed.slice(0, eq).trim();
    const value = trimmed.slice(eq + 1).trim();
    if (key.length === 0 || value.length === 0) {
      warn(`Invalid telemetry label '${trimmed}', expected key=value`);
      return acc;
    }
    acc[key] = value;
    return acc;
  }, {});
};

const normalizeTraceSampler = (value: unknown): TelemetryTraceSampler | undefined => {
  if (typeof value !== 'string') return undefined;
  const normalized = value.trim().toLowerCase().replace(/-/g, '_');
  if (normalized.length === 0) return undefined;
  switch (normalized) {
    case 'always_on':
    case 'always_off':
    case 'parent':
    case 'ratio':
      return normalized as TelemetryTraceSampler;
    default:
      return undefined;
  }
};

const parseStringArrayOption = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value
      .map((entry) => (typeof entry === 'string' ? entry : String(entry)))
      .map((entry) => entry.trim())
      .filter((entry) => entry.length > 0);
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    return trimmed.length > 0 ? [trimmed] : [];
  }
  return [];
};

const parseLogFormatsOption = (value: unknown): TelemetryLogFormat[] => {
  const entries = parseStringArrayOption(value);
  const formats: TelemetryLogFormat[] = [];
  entries.forEach((entry) => {
    const normalized = entry.toLowerCase();
    switch (normalized) {
      case 'journald':
      case 'logfmt':
      case 'json':
      case 'none':
        if (!formats.includes(normalized as TelemetryLogFormat)) {
          formats.push(normalized as TelemetryLogFormat);
        }
        break;
      default:
        warn(`Ignoring invalid --telemetry-log-format value '${entry}'`);
        break;
    }
  });
  return formats;
};

const parseLogExtraOption = (value: unknown): TelemetryLogExtra[] => {
  const entries = parseStringArrayOption(value);
  const extras: TelemetryLogExtra[] = [];
  entries.forEach((entry) => {
    const normalized = entry.toLowerCase();
    if (normalized === 'otlp') {
      if (!extras.includes('otlp')) extras.push('otlp');
    } else {
      warn(`Ignoring invalid --telemetry-log-extra value '${entry}'`);
    }
  });
  return extras;
};

const formatUnknownValue = (value: unknown): string => {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (value === null) return 'null';
  return `[${typeof value}]`;
};

const extractTelemetryOverrides = (
  opts: Record<string, unknown>,
  sourceOf: (name: string) => ('cli' | 'default' | 'env' | 'implied' | undefined),
): TelemetryOverrides => {
  const overrides: TelemetryOverrides = {};

  if (sourceOf('telemetryEnabled') === 'cli') {
    overrides.enabled = opts.telemetryEnabled === true;
  }

  if (sourceOf('telemetryOtlpEndpoint') === 'cli') {
    const endpoint = typeof opts.telemetryOtlpEndpoint === 'string' ? opts.telemetryOtlpEndpoint.trim() : '';
    if (endpoint.length > 0) overrides.otlpEndpoint = endpoint;
    else overrides.otlpEndpoint = '';
  }

  if (sourceOf('telemetryOtlpTimeoutMs') === 'cli') {
    const raw = opts.telemetryOtlpTimeoutMs;
    const parsed = typeof raw === 'number' ? raw : (typeof raw === 'string' ? Number(raw) : Number.NaN);
    if (Number.isFinite(parsed) && parsed > 0) {
      overrides.otlpTimeoutMs = Math.trunc(parsed);
    } else if (raw !== undefined) {
      warn(`Ignoring invalid --telemetry-otlp-timeout-ms value '${formatUnknownValue(raw)}'`);
    }
  }

  if (sourceOf('telemetryPrometheusEnabled') === 'cli') {
    overrides.prometheusEnabled = opts.telemetryPrometheusEnabled === true;
  }

  if (sourceOf('telemetryPrometheusHost') === 'cli') {
    const host = typeof opts.telemetryPrometheusHost === 'string' ? opts.telemetryPrometheusHost.trim() : '';
    if (host.length > 0) overrides.prometheusHost = host;
    else overrides.prometheusHost = '';
  }

  if (sourceOf('telemetryPrometheusPort') === 'cli') {
    const raw = opts.telemetryPrometheusPort;
    const parsed = typeof raw === 'number' ? raw : (typeof raw === 'string' ? Number(raw) : Number.NaN);
    if (Number.isFinite(parsed) && parsed > 0) {
      overrides.prometheusPort = Math.trunc(parsed);
    } else if (raw !== undefined) {
      warn(`Ignoring invalid --telemetry-prometheus-port value '${formatUnknownValue(raw)}'`);
    }
  }

  if (sourceOf('telemetryLabels') === 'cli') {
    const labels = parseTelemetryLabelPairs(opts.telemetryLabels);
    if (Object.keys(labels).length > 0) overrides.labels = labels;
  }

  if (sourceOf('telemetryTracesEnabled') === 'cli') {
    overrides.tracesEnabled = opts.telemetryTracesEnabled === true;
  }

  if (sourceOf('telemetryTraceSampler') === 'cli') {
    const sampler = normalizeTraceSampler(opts.telemetryTraceSampler);
    if (sampler !== undefined) {
      overrides.traceSampler = sampler;
    } else if (opts.telemetryTraceSampler !== undefined) {
      warn(`Ignoring invalid --telemetry-trace-sampler value '${formatUnknownValue(opts.telemetryTraceSampler)}'`);
    }
  }

  if (sourceOf('telemetryTraceRatio') === 'cli') {
    const raw = opts.telemetryTraceRatio;
    const parsed = typeof raw === 'number' ? raw : (typeof raw === 'string' ? Number(raw) : Number.NaN);
    if (Number.isFinite(parsed) && parsed >= 0 && parsed <= 1) {
      overrides.traceSamplerRatio = parsed;
    } else if (raw !== undefined) {
      warn(`Ignoring invalid --telemetry-trace-ratio value '${formatUnknownValue(raw)}'`);
    }
  }

  if (sourceOf('telemetryLogFormat') === 'cli') {
    const formats = parseLogFormatsOption(opts.telemetryLogFormat);
    if (formats.length > 0) {
      overrides.logFormats = formats;
    }
  }

  if (sourceOf('telemetryLogExtra') === 'cli') {
    const extras = parseLogExtraOption(opts.telemetryLogExtra);
    if (extras.length > 0) {
      overrides.logExtra = extras;
    }
  }

  if (sourceOf('telemetryLoggingOtlpEndpoint') === 'cli') {
    const endpoint = typeof opts.telemetryLoggingOtlpEndpoint === 'string' ? opts.telemetryLoggingOtlpEndpoint.trim() : '';
    overrides.logOtlpEndpoint = endpoint.length > 0 ? endpoint : '';
  }

  if (sourceOf('telemetryLoggingOtlpTimeoutMs') === 'cli') {
    const raw = opts.telemetryLoggingOtlpTimeoutMs;
    const parsed = typeof raw === 'number' ? raw : (typeof raw === 'string' ? Number(raw) : Number.NaN);
    if (Number.isFinite(parsed) && parsed > 0) {
      overrides.logOtlpTimeoutMs = Math.trunc(parsed);
    } else if (raw !== undefined) {
      warn(`Ignoring invalid --telemetry-logging-otlp-timeout-ms value '${formatUnknownValue(raw)}'`);
    }
  }

  return overrides;
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

const getOptionSource = (name: string): 'cli' | 'default' | 'env' | 'implied' | undefined => {
  try {
    return program.getOptionValueSource(name) as 'cli' | 'default' | 'env' | 'implied' | undefined;
  } catch {
    return undefined;
  }
};

const indentMultiline = (text: string, spaces: number): string => {
  const pad = ' '.repeat(spaces);
  return text.split('\n').map((line) => `${pad}${line}`).join('\n');
};

// ASCII art banner with color support for TTY
function buildAsciiBanner(): string {
  const isTTY = process.stdout.isTTY;
  const lightGray = isTTY ? '\x1b[38;5;245m' : '';  // 256-color light gray for letters (█ ░)
  const darkGray = isTTY ? '\x1b[38;5;240m' : '';   // 256-color medium gray for ghosts
  const reset = isTTY ? '\x1b[0m' : '';

  // Ghost characters: ( ) . \ _ O o ' - /
  // Letter characters: █ ▀ ▄
  const colorLine = (line: string): string => {
    if (!isTTY) return line;
    let result = '';
    // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/prefer-for-of
    for (let i = 0; i < line.length; i++) {
      const ch = line[i];
      if (ch === '█' || ch === '▀' || ch === '▄' || ch === '░') {
        result += lightGray + ch + reset;
      } else if ('().\\_Oo\'-/'.includes(ch)) {
        result += darkGray + ch + reset;
      } else {
        result += ch;
      }
    }
    return result;
  };

  const banner = `
     ('-.    ░██                     ('-.             .-')     ('-.    ░██
    ( OO )         .-')             ( OO )__       __(  OO) __( OO )__ ░██
   ░██████)  ░██  (  OO) ░██████   (░████████) ░███████. ░████████  ░████████
    .   ░██  ░██░(██████      ░██  ░██\\  .░██ ░██  . ░██ ░██   \\░██.   ░██
   ░███████  ░██  .    ..░███████  ░██    ░██ ░█████████ ░██    ░██    ░██
  ░██   ░██  ░██        ░██   ░██  ░██   ░███ ░██ oo )   ░██    ░██    ░██
   ░█████░██ ░██         ░█████░██  ░█████░██  ░███████  ░██    ░██     ░████
                                          ░██
                                    ░███████
`;

  return banner.split('\n').map((line) => colorLine(line)).join('\n');
}

async function listMcpTools(targets: string[], promptPath: string | undefined, options: Record<string, unknown>): Promise<void> {
  const configPathValue = typeof options.config === 'string' && options.config.length > 0
    ? options.config
    : undefined;
  const wantsAll = targets.some((t) => t.toLowerCase() === 'all');
  const requested = wantsAll ? undefined : targets;

  const layers = (() => {
    try {
      return discoverLayers({ configPath: configPathValue, promptPath });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(1, `failed to discover configuration layers: ${message}`, 'EXIT-LIST-TOOLS-CONFIG');
    }
  })();

  const candidateNames = (() => {
    if (requested === undefined) {
      const names = new Set<string>();
      layers.forEach((layer) => {
        const json = layer.json as { mcpServers?: Record<string, unknown> } | undefined;
        if (json?.mcpServers !== undefined) {
          Object.keys(json.mcpServers).forEach((name) => { names.add(name); });
        }
      });
      return Array.from(names);
    }
    return requested;
  })();
  const unique = Array.from(new Set(candidateNames));

  if (unique.length === 0) {
    console.log('No MCP server names provided.');
    exitWith(0, 'no MCP servers requested for listing', 'EXIT-LIST-TOOLS');
  }

  const config = buildUnifiedConfiguration({ providers: [], mcpServers: unique, restTools: [] }, layers, { verbose: options.verbose === true });
  const servers = config.mcpServers;

  const missing = unique.filter((name) => !Object.prototype.hasOwnProperty.call(servers, name));
  if (missing.length > 0) {
    exitWith(3, `Unknown MCP servers: ${missing.join(', ')}`, 'EXIT-LIST-TOOLS-MISSING');
  }

  const traceMcpFlag = options.traceMcp === true;
  const verboseFlag = options.verbose === true;
  const schemaDraft = typeof options.schemaValidate === 'string' && options.schemaValidate.length > 0
    ? options.schemaValidate as SchemaDraftTarget
    : undefined;
  const schemaTotals = { total: 0, failed: 0 };
  const schemaLabel = schemaDraft !== undefined ? schemaDraftDisplayName(schemaDraft) : undefined;

  // eslint-disable-next-line functional/no-loop-statements
  for (const name of unique) {
    const serverConfig = servers[name];
    const provider = new MCPProvider('cli-list-tools', { [name]: serverConfig }, { trace: traceMcpFlag, verbose: verboseFlag });
    let tools: MCPTool[] = [];
    try {
      await provider.warmup();
      tools = provider.listTools();
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      await provider.cleanup().catch(() => undefined);
      exitWith(3, `failed to initialize MCP server '${name}': ${message}`, 'EXIT-LIST-TOOLS-INIT');
    }
    await provider.cleanup().catch(() => undefined);

    console.log(`MCP server '${name}':`);
    if (tools.length === 0) {
      console.log('  (no tools reported)');
      console.log('');
      continue;
    }
    tools.forEach((tool) => {
      const separator = tool.name.indexOf('__');
      const shortName = separator >= 0 ? tool.name.slice(separator + 2) : tool.name;
      const namespace = separator >= 0 ? tool.name.slice(0, separator) : undefined;
      const header = namespace !== undefined
        ? `  - ${shortName} (exposed as ${tool.name})`
        : `  - ${shortName}`;
      console.log(header);
      if (typeof tool.description === 'string' && tool.description.trim().length > 0) {
        console.log(`    description: ${tool.description.trim()}`);
      }
      const inputSchema = tool.inputSchema;
      try {
        const schemaJson = JSON.stringify(inputSchema, null, 2);
        console.log('    inputSchema:');
        console.log(indentMultiline(schemaJson, 6));
      } catch {
        console.log('    inputSchema: [unserializable]');
      }
      if (schemaDraft !== undefined) {
        schemaTotals.total += 1;
        const report = validateSchemaAgainstDraft(inputSchema, schemaDraft);
        if (report.ok) {
          console.log(`    schema validation (${schemaLabel ?? schemaDraft}): OK`);
        } else {
          schemaTotals.failed += 1;
          console.log(`    schema validation (${schemaLabel ?? schemaDraft}): FAIL`);
          const maxIssuesToShow = 5;
          report.errors.slice(0, maxIssuesToShow).forEach((issue) => {
            const location = issue.instancePath.length > 0 ? issue.instancePath : '(root)';
            const keyword = issue.keyword ?? 'unknown';
            const msg = issue.message ?? 'schema violation';
            const schemaPath = issue.schemaPath.length > 0 ? ` (${issue.schemaPath})` : '';
            console.log(`      - [${keyword}] ${msg} at ${location}${schemaPath}`);
          });
          if (report.errors.length > maxIssuesToShow) {
            const omittedCount = report.errors.length - maxIssuesToShow;
            console.log(`      ... ${String(omittedCount)} more issue(s) omitted ...`);
          }
        }
      }
      if (typeof tool.instructions === 'string' && tool.instructions.trim().length > 0) {
        console.log('    instructions:');
        console.log(indentMultiline(tool.instructions.trim(), 6));
      }
    });
    console.log('');
  }
  if (schemaDraft !== undefined && schemaTotals.total > 0) {
    const passed = schemaTotals.total - schemaTotals.failed;
    console.log(`Schema validation summary (${schemaLabel ?? schemaDraft}): ${String(passed)}/${String(schemaTotals.total)} passed, ${String(schemaTotals.failed)} failed.`);
    if (schemaTotals.failed > 0) {
      exitWith(5, `schema validation failed for ${String(schemaTotals.failed)} tool(s)`, 'EXIT-LIST-TOOLS-SCHEMA');
    }
  }
  exitWith(0, 'listed MCP tools', 'EXIT-LIST-TOOLS');
}

// Build a Resolved Defaults section using frontmatter + config (if available)
function buildResolvedDefaultsHelp(): string {
  try {
    const isTTY = process.stdout.isTTY;
    const green = isTTY ? '\x1b[92m' : '';
    const yellow = isTTY ? '\x1b[93m' : '';
    const gray = isTTY ? '\x1b[90m' : '';
    const white = isTTY ? '\x1b[97m' : '';
    const reset = isTTY ? '\x1b[0m' : '';

    const lines: string[] = [];

    // Add ASCII banner
    lines.push(buildAsciiBanner());
    lines.push('');
    lines.push(`${white}Version:${reset} ${VERSION}`);
    lines.push('');

    const args = process.argv.slice(2);
    // Find system prompt file: first non-option argument, or fallback to argv[1] (shebang use)
    // When called as "./neda/neda.ai --help", the .ai file is args[0]
    let candidate: string | undefined = args.find((a) => (typeof a === 'string' && a.length > 0 && !a.startsWith('-')));

    // Fallback: if running via shebang (node dist/cli.js), argv[1] is the script being executed
    if (candidate === undefined && typeof process.argv[1] === 'string') {
      candidate = process.argv[1];
    }

    let promptPath: string | undefined = undefined;
    if (typeof candidate === 'string' && candidate.length > 0) {
      const p = candidate.startsWith('@') ? candidate.slice(1) : candidate;
      // Check if it's a valid .ai file
      if (p.length > 0) {
        try {
          if (fs.existsSync(p) && fs.statSync(p).isFile()) {
            promptPath = p;
          }
        } catch {
          // ignore stat errors
        }
      }
    }

    // Read frontmatter from promptPath (if present)
    let fmOptions: FrontmatterOptions | undefined = undefined;
    let fmDesc = '';
    let fmDescOnly = '';
    let fmUsage = '';
    const isAgentMode = promptPath?.endsWith('.ai') ?? false;

    if (promptPath !== undefined) {
      try {
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
        }
      } catch {
        // Frontmatter parsing failed - continue without agent-specific info
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

    const inv = (typeof candidate === 'string' && candidate.length > 0) ? candidate : 'ai-agent';

    // Build description section
    if (isAgentMode) {
      lines.push(`${green}AGENT${reset}`);
      if (fmDesc.length > 0) {
        lines.push(fmDesc);
      }
      lines.push('');
      lines.push(`${green}USAGE${reset}`);
      if (fmUsage.length > 0) {
        lines.push(`  ${white}${inv}${reset} ${yellow}"${fmUsage}"${reset}`);
      } else {
        lines.push(`  ${white}${inv}${reset} ${yellow}"<user prompt>"${reset}`);
      }
    } else {
      lines.push(`${green}USAGE${reset}`);
      lines.push(`  ${white}ai-agent${reset} ${yellow}<system-prompt> <user-prompt>${reset}`);
      lines.push('');
      lines.push('  Create a new agent by adding frontmatter to a .ai file:');
      lines.push('');
    }

    // runtime toggles are CLI-only and not shown in template
    // Include input/output if present in frontmatter
    let inputBlock: { format: 'json'|'text'; schema?: Record<string, unknown> } | undefined;
    let outputBlock: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> } | undefined;
    // Skip this if frontmatter parsing already failed above
    if (fmOptions !== undefined) {
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
          if (parsed?.inputSpec !== undefined) {
            const inp: Record<string, unknown> = { format: parsed.inputSpec.format };
            if (parsed.inputSpec.schema !== undefined) inp.schema = parsed.inputSpec.schema;
            inputBlock = inp as { format: 'json'|'text'; schema?: Record<string, unknown> };
          }
        }
      } catch { /* ignore */ }
    }

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
        stream: false,
        traceLLM: false,
        traceMCP: false,
        traceSdk: false,
        verbose: false,
      },
      strings: {},
      input: inputBlock,
      output: outputBlock,
    });

    lines.push('');
    lines.push(`${green}${isAgentMode ? 'FLATTENED FRONTMATTER' : 'FRONTMATTER TEMPLATE'}${reset} ${gray}(copy & paste into your .ai file)${reset}`);
    lines.push('---');
    const dumpedUnknown = (yaml as unknown as { dump: (o: unknown, opts?: Record<string, unknown>) => unknown }).dump(fmTemplate, { lineWidth: 120 });
    const yamlText = typeof dumpedUnknown === 'string' ? dumpedUnknown : JSON.stringify(dumpedUnknown);
    yamlText.split('\n').forEach((line) => {
      if (line.trim().length > 0) {
        lines.push(line);
      }
    });
    lines.push('---');

    // Show config resolution in gray
    lines.push('');
    lines.push(`${green}CONFIGURATION${reset}`);
    if (promptPath !== undefined) lines.push(`${gray}  Prompt: ${promptPath}${reset}`);
    try {
      const layers = discoverLayers({ configPath: undefined, promptPath });
      lines.push(`${gray}  Config Files Resolution Order:${reset}`);
      layers.forEach((ly, idx) => {
        const jExists = fs.existsSync(ly.jsonPath);
        const eExists = fs.existsSync(ly.envPath);
        lines.push(`${gray}    ${String(idx + 1)}. ${ly.jsonPath} ${jExists ? '(found)' : '(missing)'} | ${ly.envPath} ${eExists ? '(found)' : '(missing)'}${reset}`);
      });
      lines.push('');
    } catch { /* ignore */ }

    return `\n${lines.join('\n')}\n`;
  } catch {
    return '';
  }
}

// Completely replace help command to ensure our formatting always appears
program.helpInformation = function() {
  const before = buildResolvedDefaultsHelp();
  const after = (() => {
  const isTTY = process.stdout.isTTY;
  const cyan = isTTY ? '\x1b[96m' : '';
  const green = isTTY ? '\x1b[92m' : '';
  const yellow = isTTY ? '\x1b[93m' : '';
  const gray = isTTY ? '\x1b[90m' : '';
  const white = isTTY ? '\x1b[97m' : '';
  const reset = isTTY ? '\x1b[0m' : '';

  const lines: string[] = [];

  const byGroup = getOptionsByGroup();
  const order = ['Master Agent Overrides', 'Master Defaults', 'All Models Overrides', 'Global Controls'];

  lines.push('');
  lines.push(`${green}OPTIONS${reset}`);
  lines.push('');

  order.forEach((group) => {
    const list = byGroup.get(group);
    if (list === undefined || list.length === 0) return;

    const groupTitle = (
      group === 'Master Defaults' ? `${group} ${gray}(inherited by sub-agents when unset)${reset}`
      : group === 'Master Agent Overrides' ? `${group} ${gray}(strict - cannot be overridden)${reset}`
      : group === 'Global Controls' ? 'Global Application Controls'
      : group
    );

    lines.push(`  ${cyan}${groupTitle}${reset}`);
    list
      .filter((opt) => opt.cli?.showInHelp === true)
      .forEach((opt) => {
        const names = formatCliNames(opt);
        if (typeof names === 'string' && names.length > 0) {
          const def = opt.default;
          const defStr = typeof def === 'number' || typeof def === 'boolean' || (typeof def === 'string' && def.length > 0)
            ? ` ${gray}(default: ${String(def)})${reset}`
            : '';
          lines.push(`    ${green}${names}${reset}${defStr}`);
          lines.push(`      ${gray}${opt.description}${reset}`);
        }
      });
    lines.push('');
  });

  // Add examples section
  lines.push(`${green}EXAMPLES${reset}`);
  lines.push('');
  lines.push(`  ${white}Direct invocation with an agent file:${reset}`);
  lines.push(`    ${yellow}$ ./neda/neda.ai "research Acme Corp"${reset}`);
  lines.push('');
  lines.push(`  ${white}Direct invocation with inline prompts:${reset}`);
  lines.push(`    ${yellow}$ ai-agent "You are a helpful assistant" "List current directory"${reset}`);
  lines.push('');
  lines.push(`  ${white}Headend mode (multi-agent server):${reset}`);
  lines.push(`    ${yellow}$ ai-agent --agent neda/*.ai --api 8080 --mcp stdio${reset}`);
  lines.push('');
  lines.push(`  ${white}List available MCP tools:${reset}`);
  lines.push(`    ${yellow}$ ai-agent --list-tools all${reset}`);
  lines.push('');
  lines.push(`${gray}For more information: https://github.com/netdata/ai-agent${reset}`);
  lines.push('');

  return lines.join('\n');
  })();
  return before + after;
};

const shutdownController = new ShutdownController();
shutdownController.register('mcp-shared-registry', async () => {
  await shutdownSharedRegistry();
});

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support')
  .version(VERSION, '-V, --version', 'Output the current version')
  .configureHelp({
    // Suppress only Commander's default option formatting
    // We provide complete help via addHelpText('before') and addHelpText('after')
    formatHelp: (_cmd, _helper) => {
      // Return empty string to suppress Commander's default sections
      // Our custom help text in before/after hooks will provide everything
      return '';
    },
    // Ensure help command and --help are always recognized
    helpWidth: process.stdout.columns
  });

const agentOption = new Option('--agent <path>', 'Register an agent (.ai) file; repeat to add multiple agents')
  .argParser((value: string, previous: string[]) => appendValue(value, previous))
  .default([], undefined);

const apiHeadendOption = new Option('--api <port>', 'Start REST API headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const mcpHeadendOption = new Option('--mcp <transport>', 'Start MCP headend (stdio|http:port|sse:port|ws:port)')
  .argParser((value: string, previous: string[]) => appendValue(value, previous))
  .default([], undefined);

const openaiCompletionsHeadendOption = new Option('--openai-completions <port>', 'Start OpenAI chat completions headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const anthropicCompletionsHeadendOption = new Option('--anthropic-completions <port>', 'Start Anthropic messages headend on the given port (repeatable)')
  .argParser((value: string, previous: number[]) => appendValue(parsePort(value), previous))
  .default([], undefined);

const apiConcurrencyOption = new Option('--api-concurrency <n>', 'Maximum concurrent REST API sessions')
  .argParser(parsePositive);

const openaiCompletionsConcurrencyOption = new Option('--openai-completions-concurrency <n>', 'Maximum concurrent OpenAI chat sessions')
  .argParser(parsePositive);

const anthropicCompletionsConcurrencyOption = new Option('--anthropic-completions-concurrency <n>', 'Maximum concurrent Anthropic chat sessions')
  .argParser(parsePositive);

const slackHeadendOption = new Option('--slack', 'Start Slack Socket Mode headend');
const listToolsOption = new Option('--list-tools <server>', 'List tools for the specified MCP server (use "all" to list every server)')
  .argParser((value: string, previous: string[]) => appendValue(value, previous))
  .default([], undefined);
const schemaValidateOption = new Option('--schema-validate <draft>', 'Validate MCP tool schemas against the specified JSON Schema draft (e.g. draft-04, draft-07, draft-2019-09)')
  .argParser((value: string) => normalizeSchemaDraftTarget(value));

program.addOption(agentOption);
program.addOption(apiHeadendOption);
program.addOption(mcpHeadendOption);
program.addOption(openaiCompletionsHeadendOption);
program.addOption(anthropicCompletionsHeadendOption);
program.addOption(apiConcurrencyOption);
program.addOption(openaiCompletionsConcurrencyOption);
program.addOption(anthropicCompletionsConcurrencyOption);
program.addOption(slackHeadendOption);
program.addOption(listToolsOption);
program.addOption(schemaValidateOption);

interface HeadendModeConfig {
  agentPaths: string[];
  apiPorts: number[];
  mcpTargets: string[];
  openaiCompletionsPorts: number[];
  anthropicCompletionsPorts: number[];
  enableSlack: boolean;
  options: Record<string, unknown>;
}

const readCliTools = (opts: Record<string, unknown>): string | undefined => {
  const keys = ['tools', 'tool', 'mcp', 'mcpTool', 'mcpTools'] as const;
  const candidate = keys
    .map((key) => opts[key])
    .find((value): value is string => typeof value === 'string' && value.length > 0);
  return candidate;
};

const readOverrideValues = (opts: Record<string, unknown>): string[] => {
  const raw = opts.override;
  if (Array.isArray(raw)) {
    return raw.filter((value): value is string => typeof value === 'string' && value.length > 0);
  }
  if (typeof raw === 'string' && raw.length > 0) return [raw];
  return [];
};

const isReasoningLevel = (value: string): value is ReasoningLevel => {
  const normalized = value.toLowerCase();
  return normalized === 'minimal' || normalized === 'low' || normalized === 'medium' || normalized === 'high';
};

const normalizeReasoningKeyword = (value: unknown): ReasoningLevel | 'none' | 'default' | 'unset' | 'inherit' | undefined => {
  if (typeof value !== 'string') return undefined;
  const normalized = value.trim().toLowerCase();
  if (normalized.length === 0) return undefined;
  if (normalized === 'none' || normalized === 'default' || normalized === 'unset' || normalized === 'inherit') return normalized;
  if (isReasoningLevel(normalized)) return normalized;
  return undefined;
};

const parseReasoningOverrideStrict = (value: string): ReasoningLevel | 'none' => {
  const normalized = normalizeReasoningKeyword(value);
  if (normalized === undefined || normalized === 'default' || normalized === 'unset' || normalized === 'inherit') {
    throw new Error("reasoning override requires one of: none, minimal, low, medium, high");
  }
  return normalized;
};

const parseDefaultReasoningValue = (value: string): ReasoningLevel | 'none' | undefined => {
  const normalized = normalizeReasoningKeyword(value);
  if (normalized === undefined) {
    throw new Error("default reasoning requires one of: none, minimal, low, medium, high, default, unset, inherit");
  }
  if (normalized === 'default' || normalized === 'unset' || normalized === 'inherit') return undefined;
  return normalized;
};

const isCachingMode = (value: string): value is 'none' | 'full' => {
  const normalized = value.toLowerCase();
  return normalized === 'none' || normalized === 'full';
};

const parseToolingTransport = (value: unknown, source: string): 'native' | 'xml' | 'xml-final' => {
  if (typeof value !== 'string') {
    exitWith(4, `invalid ${source} tooling transport (expected native|xml|xml-final)`, 'EXIT-CLI-TOOLING-TRANSPORT');
  }
  const normalized = value.trim().toLowerCase();
  if (normalized === 'native' || normalized === 'xml' || normalized === 'xml-final') {
    return normalized;
  }
  exitWith(4, `invalid ${source} tooling transport '${value}' (use native|xml|xml-final)`, 'EXIT-CLI-TOOLING-TRANSPORT');
};

const parseCachingModeStrict = (value: string): 'none' | 'full' => {
  if (!isCachingMode(value)) {
    throw new Error("caching override requires 'none' or 'full'");
  }
  return value.toLowerCase() as 'none' | 'full';
};

const buildGlobalOverrides = (entries: readonly string[]): LoadAgentOptions['globalOverrides'] => {
  if (entries.length === 0) return undefined;
  // Downstream loaders assume the arrays here stay immutable; do not mutate after construction.
  const overrides: LoadAgentOptions['globalOverrides'] = {};
  const normalizeKey = (raw: string): string => {
    const lower = raw.trim();
    const canonicalMap: Record<string, string> = {
      models: 'models',
      tools: 'tools',
      agents: 'agents',
      temperature: 'temperature',
      topP: 'topP',
      'top-p': 'topP',
      maxOutputTokens: 'maxOutputTokens',
      'max-output-tokens': 'maxOutputTokens',
      repeatPenalty: 'repeatPenalty',
      'repeat-penalty': 'repeatPenalty',
      llmTimeout: 'llmTimeout',
      'llm-timeout': 'llmTimeout',
      'llm-timeout-ms': 'llmTimeout',
      toolTimeout: 'toolTimeout',
      'tool-timeout': 'toolTimeout',
      'tool-timeout-ms': 'toolTimeout',
      maxRetries: 'maxRetries',
      'max-retries': 'maxRetries',
      maxToolTurns: 'maxToolTurns',
      'max-tool-turns': 'maxToolTurns',
      maxToolCallsPerTurn: 'maxToolCallsPerTurn',
      'max-tool-calls-per-turn': 'maxToolCallsPerTurn',
      toolResponseMaxBytes: 'toolResponseMaxBytes',
      'tool-response-max-bytes': 'toolResponseMaxBytes',
      stream: 'stream',
      mcpInitConcurrency: 'mcpInitConcurrency',
      'mcp-init-concurrency': 'mcpInitConcurrency',
      reasoning: 'reasoning',
      [REASONING_TOKENS_OPTION]: 'reasoningTokens',
      [REASONING_TOKENS_ALT_OPTION]: 'reasoningTokens',
      caching: 'caching',
      contextWindow: 'contextWindow',
      'context-window': 'contextWindow',
    };
    return canonicalMap[lower] ?? lower;
  };
  const parseNumber = (key: string, raw: string, allowZero = true): number => {
    const num = Number(raw);
    if (!Number.isFinite(num)) throw new Error(`${key} override requires a numeric value`);
    if (!allowZero && num === 0) throw new Error(`${key} override cannot be zero`);
    return num;
  };
  const parseInteger = (key: string, raw: string): number => Math.trunc(parseNumber(key, raw));
  const parseBoolean = (key: string, raw: string): boolean => {
    const normalized = raw.trim().toLowerCase();
    if (normalized === 'true' || normalized === '1') return true;
    if (normalized === 'false' || normalized === '0') return false;
    throw new Error(`${key} override requires a boolean value (true|false)`);
  };

  entries.forEach((entry) => {
    const idx = entry.indexOf('=');
    if (idx <= 0 || idx === entry.length - 1) {
      throw new Error(`invalid override '${entry}': use key=value with keys like models`);
    }
    const rawKey = entry.slice(0, idx).trim();
    const value = entry.slice(idx + 1).trim();
    if (rawKey.length === 0 || value.length === 0) {
      throw new Error(`invalid override '${entry}': key and value must be non-empty`);
    }
    const key = normalizeKey(rawKey);
    switch (key) {
      case 'models': {
        const parsed = parsePairs(value);
        if (parsed.length === 0) throw new Error('models override requires at least one provider/model pair');
        overrides.models = parsed;
        break;
      }
      case 'tools': {
        const parsed = parseList(value);
        if (parsed.length === 0) throw new Error('tools override requires at least one tool');
        overrides.tools = parsed;
        break;
      }
      case 'agents': {
        const parsed = parseList(value);
        if (parsed.length === 0) throw new Error('agents override requires at least one agent');
        overrides.agents = parsed;
        break;
      }
      case 'temperature':
        overrides.temperature = parseNumber('temperature', value);
        break;
      case 'topP':
        overrides.topP = parseNumber('topP', value);
        break;
      case 'maxOutputTokens':
        overrides.maxOutputTokens = parseInteger('maxOutputTokens', value);
        break;
      case 'repeatPenalty':
        overrides.repeatPenalty = parseNumber('repeatPenalty', value);
        break;
      case 'llmTimeout':
        overrides.llmTimeout = parseInteger('llmTimeout', value);
        break;
      case 'toolTimeout':
        overrides.toolTimeout = parseInteger('toolTimeout', value);
        break;
      case 'maxRetries':
        overrides.maxRetries = parseInteger('maxRetries', value);
        break;
      case 'maxToolTurns':
        overrides.maxToolTurns = parseInteger('maxToolTurns', value);
        break;
      case 'maxToolCallsPerTurn':
        overrides.maxToolCallsPerTurn = parseInteger('maxToolCallsPerTurn', value);
        break;
      case 'toolResponseMaxBytes':
        overrides.toolResponseMaxBytes = parseInteger('toolResponseMaxBytes', value);
        break;
      case 'mcpInitConcurrency':
        overrides.mcpInitConcurrency = parseInteger('mcpInitConcurrency', value);
        break;
      case 'stream':
        overrides.stream = parseBoolean('stream', value);
        break;
      case 'reasoning': {
        const normalizedReasoning = value.trim().toLowerCase();
        if (normalizedReasoning === 'default' || normalizedReasoning === 'unset' || normalizedReasoning === 'inherit') {
          overrides.reasoning = 'inherit';
        } else {
          overrides.reasoning = parseReasoningOverrideStrict(value);
        }
        break;
      }
      case 'reasoningTokens': {
        const normalized = value.trim().toLowerCase();
        if (normalized === 'disabled' || normalized === 'off' || normalized === 'none') {
          overrides.reasoningValue = null;
        } else {
          const tokens = parseInteger('reasoningTokens', value);
          overrides.reasoningValue = tokens <= 0 ? null : tokens;
        }
        break;
      }
      case 'caching': {
        overrides.caching = parseCachingModeStrict(value);
        break;
      }
      case 'contextWindow': {
        const tokens = parseInteger('contextWindow', value);
        if (tokens <= 0) throw new Error('contextWindow override must be a positive integer');
        overrides.contextWindow = tokens;
        break;
      }
      default:
        throw new Error(`unsupported override key '${rawKey}'; expected keys like models, maxOutputTokens, temperature, contextWindow`);
    }
  });
  return overrides;
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
    && config.openaiCompletionsPorts.length === 0
    && config.anthropicCompletionsPorts.length === 0
    && !config.enableSlack
  ) {
    exitWith(4, 'no headends specified; add --api/--mcp/--openai-completions/--anthropic-completions/--slack', 'EXIT-HEADEND-NO-HEADENDS');
  }

  const configPathValue = typeof config.options.config === 'string' && config.options.config.length > 0
    ? config.options.config
    : undefined;
  const verbose = config.options.verbose === true;
  const traceLLMFlag = config.options.traceLlm === true;
  const traceMCPFlag = config.options.traceMcp === true;
  const traceSdkFlag = config.options.traceSdk === true;
  const traceSlackFlag = config.options.traceSlack === true;

  const overrideValues = readOverrideValues(config.options);
  let globalOverrides: LoadAgentOptions['globalOverrides'] = undefined;
  if (overrideValues.length > 0) {
    try {
      globalOverrides = buildGlobalOverrides(overrideValues);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, message, 'EXIT-HEADEND-BAD-OVERRIDE');
    }
  }

  let parsedTargets: LoadAgentOptions['targets'];
  const cliModels = normalizeListOption(config.options.models);
  if (cliModels !== undefined) {
    try {
      parsedTargets = parsePairs(cliModels);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, `invalid --models value: ${message}`, 'EXIT-HEADEND-BAD-MODELS');
    }
  }

  let parsedTools: LoadAgentOptions['tools'];
  const cliTools = normalizeListOption(readCliTools(config.options));
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
    traceSdk: traceSdkFlag,
  };
  const cliTransportRaw = config.options.toolingTransport;
  if (cliTransportRaw !== undefined) {
    loadOptions.toolingTransport = parseToolingTransport(cliTransportRaw, '--tooling-transport');
  }
  if (typeof config.options.reasoning === 'string' && config.options.reasoning.length > 0) {
    try {
      loadOptions.reasoning = parseReasoningOverrideStrict(config.options.reasoning);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, message, 'EXIT-HEADEND-REASONING');
    }
  }
  if (typeof config.options.defaultReasoning === 'string' && config.options.defaultReasoning.length > 0) {
    try {
      const parsedDefaultReasoning = parseDefaultReasoningValue(config.options.defaultReasoning);
      if (parsedDefaultReasoning !== undefined) {
        loadOptions.defaultsForUndefined = {
          ...(loadOptions.defaultsForUndefined ?? {}),
          reasoning: parsedDefaultReasoning,
        };
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, message, 'EXIT-HEADEND-DEFAULT-REASONING');
    }
  }
  const configOptionsRecord: Record<string, unknown> = config.options;
  const configReasoningTokens = configOptionsRecord[REASONING_TOKENS_OPTION]
    ?? configOptionsRecord[REASONING_TOKENS_ALT_OPTION]
    ?? configOptionsRecord.reasoningValue;
  if (configReasoningTokens !== undefined) {
    if (typeof configReasoningTokens === 'number' && Number.isFinite(configReasoningTokens)) {
      loadOptions.reasoningValue = configReasoningTokens <= 0 ? null : Math.trunc(configReasoningTokens);
    } else if (typeof configReasoningTokens === 'string') {
      const normalizedTokens = configReasoningTokens.trim().toLowerCase();
      if (normalizedTokens === 'disabled' || normalizedTokens === 'off' || normalizedTokens === 'none') {
        loadOptions.reasoningValue = null;
      } else {
        const numeric = Number(normalizedTokens);
        if (!Number.isFinite(numeric)) {
          exitWith(4, `invalid reasoningTokens value '${configReasoningTokens}' in config`, 'EXIT-HEADEND-REASONING-TOKENS');
        }
        loadOptions.reasoningValue = numeric <= 0 ? null : Math.trunc(numeric);
      }
    } else {
      exitWith(4, 'invalid reasoningTokens value in config', 'EXIT-HEADEND-REASONING-TOKENS');
    }
  }
  if (typeof config.options.caching === 'string' && config.options.caching.length > 0) {
    try {
      loadOptions.caching = parseCachingModeStrict(config.options.caching);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      exitWith(4, message, 'EXIT-HEADEND-CACHING');
    }
  }
  if (globalOverrides !== undefined) {
    // Share the same object so nested loads observe identical override identity.
    loadOptions.globalOverrides = globalOverrides;
  }
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
  const openaiCompletionsConcurrency = readConcurrency('openaiCompletionsConcurrency');
  const anthropicCompletionsConcurrency = readConcurrency('anthropicCompletionsConcurrency');

  let telemetryConfigSnapshot: Configuration | undefined;
  try {
    const telemetryLayers = discoverLayers({ configPath: configPathValue, promptPath: undefined });
    telemetryConfigSnapshot = buildUnifiedConfiguration({ providers: [], mcpServers: [], restTools: [] }, telemetryLayers, { verbose });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    exitWith(1, `failed to resolve configuration for telemetry: ${message}`, 'EXIT-HEADEND-TELEMETRY-CONFIG');
  }

  const telemetryOverrides = extractTelemetryOverrides(config.options, getOptionSource);
  const runtimeTelemetryConfig = buildTelemetryRuntimeConfig({
    configuration: telemetryConfigSnapshot,
    overrides: telemetryOverrides,
    mode: 'server',
  });
  await initTelemetry(runtimeTelemetryConfig);
  markTelemetryInitialized();

  const headends: Headend[] = [];
  const restHeadends: RestHeadend[] = [];

  let slackHeadend: SlackHeadend | undefined;
  if (config.enableSlack) {
    slackHeadend = new SlackHeadend({
      agentPaths: uniqueAgents,
      loadOptions,
      verbose,
      traceLLM: traceLLMFlag,
      traceMCP: traceMCPFlag,
      traceSlack: traceSlackFlag,
    });
  }

  config.apiPorts.forEach((port) => {
    const rest = new RestHeadend(registry, { port, concurrency: apiConcurrency });
    headends.push(rest);
    restHeadends.push(rest);
  });
  mcpSpecs.forEach((spec) => {
    headends.push(new McpHeadend({ registry, transport: spec }));
  });
  config.openaiCompletionsPorts.forEach((port) => {
    headends.push(new OpenAICompletionsHeadend(registry, { port, concurrency: openaiCompletionsConcurrency }));
  });
  config.anthropicCompletionsPorts.forEach((port) => {
    headends.push(new AnthropicCompletionsHeadend(registry, { port, concurrency: anthropicCompletionsConcurrency }));
  });

  if (slackHeadend !== undefined) {
    const route = slackHeadend.getSlashCommandRoute();
    if (route !== undefined && restHeadends.length > 0) {
      restHeadends[0].registerRoute(route);
      slackHeadend.markSlashCommandRouteRegistered();
    }
    headends.push(slackHeadend);
  }
  const ttyLog = makeTTYLogCallbacks({
    color: true,
    verbose: config.options.verbose === true,
    traceLlm: config.options.traceLlm === true,
    traceMcp: config.options.traceMcp === true,
    traceSdk: config.options.traceSdk === true,
    serverMode: true, // This is for server/headend mode
    explicitFormat: (() => {
      const formats = config.options.telemetryLogFormat;
      return Array.isArray(formats) && formats.length > 0 && typeof formats[0] === 'string'
        ? formats[0]
        : undefined;
    })(), // Check if user explicitly requested a format
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
    shutdownSignal: shutdownController.signal,
    stopRef: shutdownController.stopRef,
  });

  const unregisterHeadendTask = shutdownController.register('headends', async () => {
    await manager.stopAll();
  });
  let shutdownWatchdog: NodeJS.Timeout | undefined;
  const clearShutdownWatchdog = (): void => {
    if (shutdownWatchdog !== undefined) {
      clearTimeout(shutdownWatchdog);
      shutdownWatchdog = undefined;
    }
  };

  __RUNNING_SERVER = true;

  try {

    const stopManager = async () => {
      await manager.stopAll();
    };

    const handleSignal = async (signal: NodeJS.Signals): Promise<void> => {
      if (shutdownController.isStopping()) {
        emit(`received ${signal} during shutdown; forcing exit`, 'ERR');
        exitWith(1, `forced exit after ${signal}`, 'EXIT-HEADEND-FORCE');
      }
      emit(`received ${signal}, shutting down headends`, 'WRN');
      shutdownWatchdog = setTimeout(() => {
        emit('shutdown watchdog expired; forcing exit', 'ERR');
        exitWith(1, 'shutdown watchdog expired', 'EXIT-HEADEND-WATCHDOG');
      }, 30_000).unref();
      try {
        await shutdownController.shutdown({ logger: logSink });
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        emit(`shutdown controller failed: ${message}`, 'ERR');
      } finally {
        clearShutdownWatchdog();
      }
    };

  const registeredSignals: NodeJS.Signals[] = ['SIGINT', 'SIGTERM'];
  const signalHandlers = new Map<NodeJS.Signals, () => void>();
  registeredSignals.forEach((sig) => {
    const handler = () => { void handleSignal(sig); };
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
    await exitAndShutdown(1, `failed to start headends: ${message}`, 'EXIT-HEADEND-START');
    return;
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
    await exitAndShutdown(1, `headend '${desc.label}' failed: ${message}`, 'EXIT-HEADEND-FATAL');
    return;
  }
  emit('all headends stopped gracefully', 'FIN');
  emit('shutdown completed', 'FIN');
  try {
    process.stderr.write('[info] shutdown completed\n');
  } catch {
    /* ignore */
  }
  await ensureTelemetryShutdown();
  __RUNNING_SERVER = false;
  } finally {
    clearShutdownWatchdog();
    unregisterHeadendTask();
  }
}

// Global flag: suppress fatal exits when running long-lived headends
let __RUNNING_SERVER = false;

program
  .argument('[system-prompt]', 'System prompt (string, @filename, or - for stdin)')
  .argument('[user-prompt]', 'User prompt (string, @filename, or - for stdin)')
  .option('--save-all <dir>', 'Save all agent and sub-agent conversations to directory')
  .option('--show-tree', 'Dump the full execution tree (ASCII) at the end')
  .action(async (systemPrompt: string | undefined, userPrompt: string | undefined, options: Record<string, unknown>) => {
    try {
      const agentFlags = Array.isArray(options.agent) ? (options.agent as string[]) : [];
      const apiPorts = Array.isArray(options.api) ? (options.api as number[]) : [];
      const mcpTargets = Array.isArray(options.mcp) ? (options.mcp as string[]) : [];
      const openaiCompletionsPorts = Array.isArray(options.openaiCompletions) ? (options.openaiCompletions as number[]) : [];
      const anthropicCompletionsPorts = Array.isArray(options.anthropicCompletions) ? (options.anthropicCompletions as number[]) : [];
      const listToolsTargets = Array.isArray(options.listTools) ? (options.listTools as string[]) : [];

      if (listToolsTargets.length > 0) {
        await listMcpTools(listToolsTargets, systemPrompt, options);
        return;
      }

      if (apiPorts.length > 0 || mcpTargets.length > 0 || openaiCompletionsPorts.length > 0 || anthropicCompletionsPorts.length > 0) {
        await runHeadendMode({
          agentPaths: agentFlags,
          apiPorts,
          mcpTargets,
          openaiCompletionsPorts,
          anthropicCompletionsPorts,
          enableSlack: options.slack === true,
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
      const optsRecord: Record<string, unknown> = options;
      const cliTargetsRaw: string | undefined = normalizeListOption(optsRecord.models);

      const cliToolsRaw: string | undefined = normalizeListOption(optsRecord.tools ?? optsRecord.tool ?? optsRecord.mcp ?? optsRecord.mcpTool ?? optsRecord.mcpTools);

      const overrideValuesDirect = readOverrideValues(options);
      let globalOverrides: LoadAgentOptions['globalOverrides'] = undefined;
      if (overrideValuesDirect.length > 0) {
        try {
          globalOverrides = buildGlobalOverrides(overrideValuesDirect);
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          exitWith(4, message, 'EXIT-CLI-BAD-OVERRIDE');
        }
      }

      const cliAgentsRaw: string | undefined = normalizeListOption(optsRecord.agents);

      // fmOptions already computed above
      const fmTargetsRaw = normalizeListOption(fmOptions?.models);

      const fmToolsRaw = normalizeListOption(fmOptions?.tools);
      const fmAgentsRaw = normalizeListOption(fmOptions?.agents);

      const targets = parsePairs(cliTargetsRaw ?? fmTargetsRaw);
      const toolList = parseList(cliToolsRaw ?? fmToolsRaw);
      const agentsList = parseList(cliAgentsRaw ?? fmAgentsRaw);

      if (targets.length === 0 && !(Array.isArray(globalOverrides?.models) && globalOverrides.models.length > 0)) {
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
      const optSrc = (name: string): 'cli' | 'default' | 'env' | 'implied' | undefined => getOptionSource(name);

      let cliReasoning: (ReasoningLevel | 'none') | undefined;
      if (optSrc('reasoning') === 'cli') {
        if (typeof options.reasoning === 'string' && options.reasoning.length > 0) {
          try {
            cliReasoning = parseReasoningOverrideStrict(options.reasoning);
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            exitWith(4, message, 'EXIT-CLI-REASONING');
          }
        } else {
          exitWith(4, 'invalid --reasoning value', 'EXIT-CLI-REASONING');
        }
      }

      let cliToolingTransport: 'native' | 'xml' | 'xml-final' | undefined;
      if (optSrc('toolingTransport') === 'cli') {
        cliToolingTransport = parseToolingTransport(options.toolingTransport, '--tooling-transport');
      }

      let cliReasoningValue: ProviderReasoningValue | null | undefined;
      if (optSrc(REASONING_TOKENS_OPTION) === 'cli' || optSrc(REASONING_TOKENS_ALT_OPTION) === 'cli') {
        const raw = options[REASONING_TOKENS_OPTION]
          ?? options[REASONING_TOKENS_ALT_OPTION];
        if (typeof raw === 'number' && Number.isFinite(raw)) {
          cliReasoningValue = raw <= 0 ? null : Math.trunc(raw);
        } else if (typeof raw === 'string') {
          const normalizedTokens = raw.trim().toLowerCase();
          if (normalizedTokens === 'disabled' || normalizedTokens === 'off' || normalizedTokens === 'none') {
            cliReasoningValue = null;
          } else {
            const numeric = Number(normalizedTokens);
            if (!Number.isFinite(numeric)) {
              exitWith(4, `invalid --reasoning-tokens value '${raw}'`, EXIT_REASONING_TOKENS);
            }
            cliReasoningValue = numeric <= 0 ? null : Math.trunc(numeric);
          }
        } else if (raw !== undefined) {
          exitWith(4, 'invalid --reasoning-tokens value', EXIT_REASONING_TOKENS);
        }
      }

      let cliDefaultReasoning: (ReasoningLevel | 'none') | undefined;
      if (optSrc('defaultReasoning') === 'cli') {
        if (typeof options.defaultReasoning === 'string' && options.defaultReasoning.length > 0) {
          try {
            cliDefaultReasoning = parseDefaultReasoningValue(options.defaultReasoning);
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            exitWith(4, message, 'EXIT-CLI-DEFAULT-REASONING');
          }
        } else {
          exitWith(4, 'invalid --default-reasoning value', 'EXIT-CLI-DEFAULT-REASONING');
        }
      }

      let cliCaching: ('none' | 'full') | undefined;
      if (optSrc('caching') === 'cli') {
        if (typeof options.caching === 'string' && isCachingMode(options.caching)) {
          cliCaching = options.caching.toLowerCase() as 'none' | 'full';
        } else {
          exitWith(4, `invalid --caching value '${typeof options.caching === 'string' ? options.caching : ''}'`, 'EXIT-CLI-CACHING');
        }
      }

      const cliDefaultsForUndefined = cliDefaultReasoning !== undefined ? { reasoning: cliDefaultReasoning } : undefined;

      // Derive agent id: from prompt filename when available, else 'cli-main'
      const fileAgentId = ((): string => {
        const isFile = (p: string): boolean => { try { return p !== '-' && fs.existsSync(p) && fs.statSync(p).isFile(); } catch { return false; } };
        if (isFile(systemPrompt)) return systemPrompt;
        if (isFile(userPrompt)) return userPrompt;
        return 'cli-main';
      })();

      const registry = new AgentRegistry([], {
        configPath: cfgPath,
        verbose: options.verbose === true,
        // Pass through the same overrides reference for registry + nested loads.
        globalOverrides,
        defaultsForUndefined: cliDefaultsForUndefined,
      });
      const loaded = registry.loadFromContent(fileAgentId, fmSource, {
        configPath: cfgPath,
        verbose: options.verbose === true,
        targets,
        tools: toolList,
        agents: agentsList,
        baseDir: fmBaseDir,
        // Keep overrides pointer intact for downstream recursion checks.
        globalOverrides,
        defaultsForUndefined: cliDefaultsForUndefined,
        // CLI overrides take precedence
        temperature: optSrc('temperature') === 'cli' ? Number(options.temperature) : undefined,
        topP: (optSrc('topP') === 'cli' || optSrc('top-p') === 'cli') ? Number(options.topP ?? options['top-p']) : undefined,
        llmTimeout: (optSrc('llmTimeoutMs') === 'cli' || optSrc('llm-timeout-ms') === 'cli') ? Number(options.llmTimeoutMs ?? options['llm-timeout-ms']) : undefined,
        toolTimeout: (optSrc('toolTimeoutMs') === 'cli' || optSrc('tool-timeout-ms') === 'cli') ? Number(options.toolTimeoutMs ?? options['tool-timeout-ms']) : undefined,
        maxRetries: optSrc('maxRetries') === 'cli' ? Number(options.maxRetries) : undefined,
        maxToolTurns: optSrc('maxToolTurns') === 'cli' ? Number(options.maxToolTurns) : undefined,
        toolResponseMaxBytes: (optSrc('toolResponseMaxBytes') === 'cli' || optSrc('tool-response-max-bytes') === 'cli') ? Number(options.toolResponseMaxBytes ?? options['tool-response-max-bytes']) : undefined,
        stream: typeof options.stream === 'boolean' ? options.stream : undefined,
        traceLLM: options.traceLlm === true ? true : undefined,
        traceMCP: options.traceMcp === true ? true : undefined,
        traceSdk: options.traceSdk === true ? true : undefined,
        mcpInitConcurrency: (typeof options.mcpInitConcurrency === 'string' && options.mcpInitConcurrency.length>0) ? Number(options.mcpInitConcurrency) : undefined,
        reasoning: cliReasoning,
        reasoningValue: cliReasoningValue,
        caching: cliCaching,
        toolingTransport: cliToolingTransport,
      });

      const cliTelemetryOverrides = extractTelemetryOverrides(options, getOptionSource);
      const runtimeTelemetryConfig = buildTelemetryRuntimeConfig({
        configuration: loaded.config,
        overrides: cliTelemetryOverrides,
        mode: 'cli',
      });
      await initTelemetry(runtimeTelemetryConfig);
      markTelemetryInitialized();
      const sessionTelemetryLabels = runtimeTelemetryConfig.labels;

      // Prompt variable substitution using loader-effective max tool turns
      const expectedJson = fm?.expectedOutput?.format === 'json';
      const fmtRaw: unknown = (() => { const rec: Record<string, unknown> = options; return Object.prototype.hasOwnProperty.call(rec, 'format') ? (rec as { format?: unknown }).format : undefined; })();
      const fmtOpt = typeof fmtRaw === 'string' && fmtRaw.length > 0 ? fmtRaw : undefined;
      const chosenFormatId = resolveFormatIdForCli(fmtOpt, expectedJson, process.stdout.isTTY ? true : false);
      const vars = buildPromptVariables(loaded.effective.maxToolTurns);
      vars.FORMAT = formatPromptValue(chosenFormatId);
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
          await exitAndShutdown(1, `failed to load conversation from ${effLoad}: ${msg}`, 'EXIT-CONVERSATION-LOAD');
          return;
        }
      }

      // Setup accounting
      const accountingFile = (typeof options.accounting === 'string' && options.accounting.length > 0)
        ? options.accounting
        : loaded.accountingFile ?? loaded.config.accounting?.file;

      // Setup logging callbacks (CLI-only)
      const effectiveTraceLLM = options.traceLlm === true;
      const effectiveTraceMCP = options.traceMcp === true;
      const effectiveTraceSdk = options.traceSdk === true;
      const effectiveVerbose = options.verbose === true;
      const callbacks = createCallbacks({ traceLlm: effectiveTraceLLM, traceMcp: effectiveTraceMCP, traceSdk: effectiveTraceSdk, verbose: effectiveVerbose }, loaded.config.persistence, accountingFile);

      if (options.dryRun === true) {
        await exitAndShutdown(0, 'dry run complete: configuration and MCP servers validated', 'EXIT-DRY-RUN');
        return;
      }

      // Create and run session via unified loader
      const result = await loaded.run(resolvedSystem, resolvedUser, {
        history: conversationHistory,
        callbacks,
        renderTarget: 'cli',
        outputFormat: chosenFormatId,
        telemetryLabels: sessionTelemetryLabels,
        wantsProgressUpdates: false,
      });

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
        await exitAndShutdown(code, `agent failure: ${err || 'unknown error'}`, 'EXIT-AGENT-FAILURE');
        return;
      }

      // Save conversation if requested (CLI-only)
      const effSave = (typeof options.save === 'string' && options.save.length > 0) ? options.save : undefined;
      if (typeof effSave === 'string' && effSave.length > 0) {
        try {
          fs.writeFileSync(effSave, JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          await exitAndShutdown(1, `failed to save conversation to ${effSave}: ${msg}`, 'EXIT-SAVE-CONVERSATION');
          return;
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
          await exitAndShutdown(1, `failed to save conversations to ${saveAllDir}: ${msg}`, 'EXIT-SAVE-ALL');
          return;
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
      await exitAndShutdown(0, 'success', 'EXIT-SUCCESS');
      return;
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      const code = msg.includes('config') ? 1
        : msg.includes('argument') ? 4
        : msg.includes('tool') ? 3
        : 1;
      await exitAndShutdown(code, `fatal error in CLI: ${msg}`, 'EXIT-UNKNOWN');
      return;
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
    throw new Error(`Prompt file not found: ${p}`);
  } else {
    const content = readIfFile(value);
    if (content !== undefined) return content;
    if (path.isAbsolute(value)) {
      throw new Error(`Prompt file not found: ${value}`);
    }
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

function createCallbacks(
  options: Record<string, unknown>,
  persistence?: { sessionsDir?: string; billingFile?: string },
  accountingFile?: string
): AIAgentCallbacks {
  const colorize = (text: string, colorCode: string): string => {
    return process.stderr.isTTY ? `${colorCode}${text}[0m` : text;
  };

  let thinkingOpen = false;
  let lastCharWasNewline = true;

  const ttyLog = makeTTYLogCallbacks({
    color: true,
    verbose: options.verbose === true,
    traceLlm: options.traceLlm === true,
    traceMcp: options.traceMcp === true,
    traceSdk: options.traceSdk === true,
    serverMode: false, // This is for interactive/console mode
    explicitFormat: (() => {
      const formats = options.telemetryLogFormat;
      return Array.isArray(formats) && formats.length > 0 && typeof formats[0] === 'string'
        ? formats[0]
        : undefined;
    })(), // Check if user explicitly requested a format
  });

  const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
  const defaultBase = home.length > 0 ? path.join(home, '.ai-agent') : undefined;
  const resolvedSessionsDir = persistence?.sessionsDir ?? (defaultBase !== undefined ? path.join(defaultBase, 'sessions') : undefined);
  const resolvedLedgerFile = accountingFile ?? persistence?.billingFile ?? (defaultBase !== undefined ? path.join(defaultBase, 'accounting.jsonl') : undefined);

  const baseCallbacks: AIAgentCallbacks = {
    onLog: (entry: LogEntry) => {
      if (entry.severity !== 'THK' && thinkingOpen && !lastCharWasNewline) {
        try { process.stderr.write('\n'); } catch { /* ignore */ }
        lastCharWasNewline = true;
        thinkingOpen = false;
      }
      ttyLog.onLog?.(entry);
      if (entry.severity === 'THK') {
        const header = formatLog(entry, {
          color: true,
        });
        try { process.stderr.write(`${header} `); } catch { /* ignore */ }
        thinkingOpen = true;
        lastCharWasNewline = false;
      }
    },
    onOutput: (text: string) => {
      if (options.verbose === true) {
        try { process.stderr.write(text); } catch { /* ignore */ }
      }
    },
    onThinking: (text: string) => {
      const colored = colorize(text, '\x1b[2;37m');
      process.stderr.write(colored);
      if (text.length > 0) {
        lastCharWasNewline = text.endsWith('\n');
        thinkingOpen = true;
      }
    },
    onAccounting: (_entry: AccountingEntry) => {
      // No per-entry file writes from CLI; consolidated flush occurs via onAccountingFlush.
    },
  };

  const persistenceConfig = {
    sessionsDir: resolvedSessionsDir,
    billingFile: resolvedLedgerFile,
  };

  return mergeCallbacksWithPersistence(baseCallbacks, persistenceConfig) ?? baseCallbacks;
}


process.on('uncaughtException', (e) => {
  const msg = e instanceof Error ? `${e.name}: ${e.message}` : String(e);
  if (__RUNNING_SERVER) {
    const warn = `[warn] uncaught exception (continuing): ${msg}`;
    try { process.stderr.write(`${warn}
`); } catch {}
    return;
  }
  void exitAndShutdown(1, `uncaught exception: ${msg}`, 'EXIT-UNCAUGHT-EXCEPTION');
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
  void exitAndShutdown(1, `unhandled rejection: ${msg}`, 'EXIT-UNHANDLED-REJECTION');
});

program.parse();
