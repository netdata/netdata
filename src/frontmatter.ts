import fs from 'node:fs';
import path from 'node:path';

import * as yaml from 'js-yaml';

import type { ReasoningLevel, CachingMode } from './types.js';

import { getFrontmatterAllowedKeys, OPTIONS_REGISTRY } from './options-registry.js';

export interface FrontmatterOptions {
  models?: string | string[];
  tools?: string | string[];
  agents?: string | string[];
  usage?: string;
  parallelToolCalls?: boolean;
  maxToolTurns?: number;
  maxToolCallsPerTurn?: number;
  maxRetries?: number;
  maxConcurrentTools?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  toolResponseMaxBytes?: number;
  reasoning?: ReasoningLevel;
  caching?: CachingMode;
}

export function parseFrontmatter(
  src: string,
  opts?: { baseDir?: string; strict?: boolean }
): {
  expectedOutput?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> },
  inputSpec?: { format: 'text' | 'json'; schema?: Record<string, unknown> },
  toolName?: string,
  options?: FrontmatterOptions,
  description?: string,
  usage?: string
} | undefined {
  // Allow a shebang on the first line (e.g., "#!/usr/bin/env ai-agent")
  let text = src;
  if (text.startsWith('#!')) {
    const nl = text.indexOf('\n');
    text = nl >= 0 ? text.slice(nl + 1) : '';
  }
  const m = /^---\n([\s\S]*?)\n---\n/.exec(text);
  if (m === null) return undefined;
  try {
    const rawUnknown: unknown = yaml.load(m[1]);
    if (typeof rawUnknown !== 'object' || rawUnknown === null) return undefined;
    const docObj = rawUnknown as {
      output?: { format?: string; schema?: unknown; schemaRef?: string };
      input?: { format?: string; schema?: unknown; schemaRef?: string };
      toolName?: unknown;
    } & Record<string, unknown>;
    let expectedOutput: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> } | undefined;
    let inputSpec: { format: 'text'|'json'; schema?: Record<string, unknown> } | undefined;
    let toolName: string | undefined;
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
          schemaObj = loadSchemaValue(s, ref, opts?.baseDir);
        }
        const fmt: 'json'|'markdown'|'text' = format === 'json' ? 'json' : (format === 'markdown' ? 'markdown' : 'text');
        expectedOutput = { format: fmt, schema: schemaObj };
      }
    }
    const raw = rawUnknown as Record<string, unknown>;
    // Strict validation of allowed top-level keys (driven by the options registry)
    if (opts?.strict !== false) {
      const fmAllowed = getFrontmatterAllowedKeys();
      const allowedTopLevel = new Set([
        'description', 'usage', 'output', 'input', 'toolName',
        ...fmAllowed,
      ]);
      const unknownKeys = Object.keys(raw).filter((k) => !allowedTopLevel.has(k));
      if (unknownKeys.length > 0) {
        const bad = unknownKeys.join(', ');
        throw new Error(`Unsupported frontmatter key(s): ${bad}. Remove these or move them to CLI/config. See README for valid keys.`);
      }
      // Explicitly reject runtime/app-global keys to guide users
      const forbidden = ['traceLLM','traceMCP','verbose','accounting','save','load','stream','targets'];
      const presentForbidden = forbidden.filter((k) => Object.prototype.hasOwnProperty.call(raw, k));
      if (presentForbidden.length > 0) {
        const bad = presentForbidden.join(', ');
        throw new Error(`Invalid frontmatter key(s): ${bad}. These are runtime application options; use CLI flags instead.`);
      }
    }
    // Parse input spec for sub-agent tools
    if (docObj.input !== undefined && typeof docObj.input.format === 'string') {
      const fmt = docObj.input.format.toLowerCase();
      if (fmt === 'text' || fmt === 'json') {
        let schemaObj: Record<string, unknown> | undefined;
        if (fmt === 'json') {
          const s: unknown = docObj.input.schema;
          const refVal: unknown = (docObj.input as { schemaRef?: unknown }).schemaRef;
          const ref: string | undefined = typeof refVal === 'string' ? refVal : undefined;
          schemaObj = loadSchemaValue(s, ref, opts?.baseDir);
        }
        inputSpec = { format: fmt, schema: schemaObj };
      }
    }
    if (typeof docObj.toolName === 'string' && docObj.toolName.trim().length > 0) {
      toolName = docObj.toolName.trim();
    }
    const options: FrontmatterOptions = {};
    if (typeof raw.models === 'string' || Array.isArray(raw.models)) options.models = raw.models as (string | string[]);
    if (typeof raw.tools === 'string' || Array.isArray(raw.tools)) options.tools = raw.tools as (string | string[]);
    if (typeof raw.agents === 'string' || Array.isArray(raw.agents)) options.agents = raw.agents as (string | string[]);
    if (typeof raw.parallelToolCalls === 'boolean') options.parallelToolCalls = raw.parallelToolCalls;
    if (typeof raw.maxToolTurns === 'number') options.maxToolTurns = raw.maxToolTurns;
    if (typeof raw.maxToolCallsPerTurn === 'number') options.maxToolCallsPerTurn = raw.maxToolCallsPerTurn;
    if (typeof raw.maxRetries === 'number') options.maxRetries = raw.maxRetries;
    if (typeof raw.maxConcurrentTools === 'number') options.maxConcurrentTools = raw.maxConcurrentTools;
    if (typeof raw.llmTimeout === 'number') options.llmTimeout = raw.llmTimeout;
    if (typeof raw.toolTimeout === 'number') options.toolTimeout = raw.toolTimeout;
    if (typeof raw.toolResponseMaxBytes === 'number') options.toolResponseMaxBytes = raw.toolResponseMaxBytes;
    if (typeof raw.maxOutputTokens === 'number') options.maxOutputTokens = raw.maxOutputTokens;
    if (typeof raw.repeatPenalty === 'number') options.repeatPenalty = raw.repeatPenalty;
    if (typeof raw.temperature === 'number') options.temperature = raw.temperature;
    if (typeof raw.topP === 'number') options.topP = raw.topP;
    if (typeof raw.reasoning === 'string') {
      const normalized = raw.reasoning.toLowerCase();
      if (normalized === 'minimal' || normalized === 'low' || normalized === 'medium' || normalized === 'high') {
        options.reasoning = normalized as ReasoningLevel;
      } else {
        throw new Error(`Invalid reasoning level '${raw.reasoning}' in frontmatter. Expected minimal, low, medium, or high.`);
      }
    }
    if (typeof raw.caching === 'string') {
      const normalizedCaching = raw.caching.toLowerCase();
      if (normalizedCaching === 'none' || normalizedCaching === 'full') {
        options.caching = normalizedCaching as CachingMode;
      } else {
        throw new Error(`Invalid caching mode '${raw.caching}' in frontmatter. Expected none or full.`);
      }
    }
    if (typeof raw.description === 'string') description = raw.description;
    if (typeof raw.usage === 'string') usage = raw.usage;
    return { expectedOutput, inputSpec, toolName, options, description, usage };
  } catch (e) {
    if (opts?.strict !== false) {
      if (e instanceof Error) throw e;
      throw new Error(String(e));
    }
    return undefined;
  }
}

function loadSchemaValue(v: unknown, schemaRef?: string, baseDir?: string): Record<string, unknown> | undefined {
  try {
    if (v !== null && v !== undefined && typeof v === 'object') return v as Record<string, unknown>;
    if (typeof v === 'string') {
      try { return JSON.parse(v) as Record<string, unknown>; } catch (e) { try { process.stderr.write(`[warn] frontmatter JSON parse failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {} }
      try { return yaml.load(v) as Record<string, unknown>; } catch (e) { try { process.stderr.write(`[warn] frontmatter YAML parse failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {} }
    }
    if (typeof schemaRef === 'string' && schemaRef.length > 0) {
      try {
        const resolvedPath = path.resolve(baseDir ?? process.cwd(), schemaRef);
        const content = fs.readFileSync(resolvedPath, 'utf-8');
        if (/\.json$/i.test(resolvedPath)) return JSON.parse(content) as Record<string, unknown>;
        if (/\.(ya?ml)$/i.test(resolvedPath)) return yaml.load(content) as Record<string, unknown>;
        try { return JSON.parse(content) as Record<string, unknown>; } catch (e) { try { process.stderr.write(`[warn] frontmatter JSON parse failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {} }
        return yaml.load(content) as Record<string, unknown>;
      } catch {
        return undefined;
      }
    }
  } catch {
    return undefined;
  }
  return undefined;
}

export function stripFrontmatter(src: string): string {
  const m = /^---\n([\s\S]*?)\n---\n/;
  return src.replace(m, '');
}

// Extract the body of a prompt file, removing shebang and YAML frontmatter if present
export function extractBodyWithoutFrontmatter(src: string): string {
  let text = src;
  if (text.startsWith('#!')) {
    const nl = text.indexOf('\n');
    text = nl >= 0 ? text.slice(nl + 1) : '';
  }
  return stripFrontmatter(text);
}


export function parseList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((s) => (typeof s === 'string' ? s : String(s))).map((s) => s.trim()).filter((s) => s.length > 0);
  if (typeof value === 'string') return value.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
  return [];
}

export function parsePairs(value: unknown): { provider: string; model: string }[] {
  const arr = Array.isArray(value) ? value.map((v) => (typeof v === 'string' ? v : String(v))) : (typeof value === 'string' ? value.split(',') : []);
  return arr
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
    .map((token) => {
      const slash = token.indexOf('/');
      if (slash <= 0 || slash >= token.length - 1) {
        throw new Error(`Invalid provider/model pair '${token}'. Expected format provider/model.`);
      }
      const provider = token.slice(0, slash).trim();
      const model = token.slice(slash + 1).trim();
      if (provider.length === 0 || model.length === 0) {
        throw new Error(`Invalid provider/model pair '${token}'.`);
      }
      return { provider, model };
    });
}

// Build a YAML-ready frontmatter template object, kept in sync with FrontmatterOptions keys
export function buildFrontmatterTemplate(args: {
  fmOptions?: FrontmatterOptions;
  description: string;
  usage: string;
  // Deprecated granular defaults kept for backward compatibility; ignored for dynamic template
  numbers?: Record<string, number>;
  booleans?: Record<string, boolean>;
  strings?: Record<string, string | undefined>;
  input?: { format: 'text'|'json'; schema?: Record<string, unknown> };
  output?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> };
}): Record<string, unknown> {
  const toArray = (v: unknown): string[] => {
    if (Array.isArray(v)) return v.map((x) => String(x));
    if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
    return [];
  };

  const fm = args.fmOptions;
  const llmsVal: string[] = (fm !== undefined) ? toArray(fm.models) : [];
  const toolsVal: string[] = (fm !== undefined) ? toArray(fm.tools) : [];
  const agentsVal: string[] = (fm !== undefined) ? toArray(fm.agents) : [];

  const tpl: Record<string, unknown> = {};
  tpl.description = args.description;
  tpl.usage = args.usage;
  // Always include list keys
  tpl.models = llmsVal;
  tpl.tools = toolsVal;
  tpl.agents = agentsVal.length > 0 ? agentsVal : [];

  // Dynamically include ALL fm-allowed options from the OPTIONS_REGISTRY
  const fmAllowed = OPTIONS_REGISTRY.filter((o) => o.fm?.allowed === true);
  fmAllowed.forEach((opt) => {
    const key = opt.fm?.key ?? opt.key;
    // Avoid re-setting models/tools/agents already handled above
    if (key === 'models' || key === 'tools' || key === 'agents') return;
    const fmVal = (fm as Record<string, unknown> | undefined)?.[key];
    let val: unknown = undefined;
    switch (opt.type) {
      case 'number': {
        if (typeof fmVal === 'number' && Number.isFinite(fmVal)) val = fmVal;
        else if (typeof opt.default === 'number') val = opt.default;
        else val = 0;
        break;
      }
      case 'boolean': {
        if (typeof fmVal === 'boolean') val = fmVal; else if (typeof opt.default === 'boolean') val = opt.default; else val = false;
        break;
      }
      case 'string': {
        if (typeof fmVal === 'string') val = fmVal; else if (typeof opt.default === 'string') val = opt.default; else val = '';
        break;
      }
      case 'string[]': {
        if (Array.isArray(fmVal)) val = toArray(fmVal); else if (Array.isArray(opt.default)) val = opt.default; else val = [];
        break;
      }
      default:
        val = fmVal ?? (opt.default as unknown);
    }
    tpl[key] = val;
  });
  if (args.input !== undefined) tpl.input = args.input;
  if (args.output !== undefined) tpl.output = args.output;
  return tpl;
}
