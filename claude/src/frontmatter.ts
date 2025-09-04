import fs from 'node:fs';
import path from 'node:path';

import * as yaml from 'js-yaml';

export interface FrontmatterOptions {
  llms?: string | string[];
  targets?: string | string[];
  tools?: string | string[];
  agents?: string | string[];
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

export function parseFrontmatter(
  src: string,
  opts?: { baseDir?: string }
): {
  expectedOutput?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> },
  inputSpec?: { format: 'text' | 'json'; schema?: Record<string, unknown> },
  toolName?: string,
  options?: FrontmatterOptions,
  description?: string,
  usage?: string
} | undefined {
  const m = /^---\n([\s\S]*?)\n---\n/.exec(src);
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
    if (typeof raw.llms === 'string' || Array.isArray(raw.llms)) options.llms = raw.llms as (string | string[]);
    if (typeof raw.targets === 'string' || Array.isArray(raw.targets)) options.targets = raw.targets as (string | string[]);
    if (typeof raw.tools === 'string' || Array.isArray(raw.tools)) options.tools = raw.tools as (string | string[]);
    if (typeof raw.agents === 'string' || Array.isArray(raw.agents)) options.agents = raw.agents as (string | string[]);
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
    if (typeof raw.toolResponseMaxBytes === 'number') options.toolResponseMaxBytes = raw.toolResponseMaxBytes;
    if (typeof raw.temperature === 'number') options.temperature = raw.temperature;
    if (typeof raw.topP === 'number') options.topP = raw.topP;
    if (typeof raw.description === 'string') description = raw.description;
    if (typeof raw.usage === 'string') usage = raw.usage;
    return { expectedOutput, inputSpec, toolName, options, description, usage };
  } catch {
    return undefined;
  }
}

export function loadSchemaValue(v: unknown, schemaRef?: string, baseDir?: string): Record<string, unknown> | undefined {
  try {
    if (v !== null && v !== undefined && typeof v === 'object') return v as Record<string, unknown>;
    if (typeof v === 'string') {
      try { return JSON.parse(v) as Record<string, unknown>; } catch { /* ignore */ }
      try { return yaml.load(v) as Record<string, unknown>; } catch { /* ignore */ }
    }
    if (typeof schemaRef === 'string' && schemaRef.length > 0) {
      try {
        const resolvedPath = path.resolve(baseDir ?? process.cwd(), schemaRef);
        const content = fs.readFileSync(resolvedPath, 'utf-8');
        if (/\.json$/i.test(resolvedPath)) return JSON.parse(content) as Record<string, unknown>;
        if (/\.(ya?ml)$/i.test(resolvedPath)) return yaml.load(content) as Record<string, unknown>;
        try { return JSON.parse(content) as Record<string, unknown>; } catch { /* ignore */ }
        return yaml.load(content) as Record<string, unknown>;
      } catch {
        return undefined;
      }
    }
  } catch { /* ignore */ }
  return undefined;
}

export function stripFrontmatter(src: string): string {
  const m = /^---\n([\s\S]*?)\n---\n/;
  return src.replace(m, '');
}

export function readFmNumber(opts: FrontmatterOptions | undefined, key: keyof FrontmatterOptions): number | undefined {
  if (opts === undefined) return undefined;
  const v = opts[key];
  if (v === undefined) return undefined;
  const n = Number(v);
  if (!Number.isFinite(n)) return undefined;
  return n;
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
  numbers: {
    temperature: number;
    topP: number;
    llmTimeout: number;
    toolTimeout: number;
    toolResponseMaxBytes: number;
    maxRetries: number;
    maxToolTurns: number;
  };
  booleans: {
    stream: boolean;
    parallelToolCalls: boolean;
    traceLLM: boolean;
    traceMCP: boolean;
    verbose: boolean;
  };
  strings: {
    accounting?: string;
    save?: string;
    load?: string;
  };
  output?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> };
}): Record<string, unknown> {
  const toArray = (v: unknown): string[] => {
    if (Array.isArray(v)) return v.map((x) => String(x));
    if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter((s) => s.length > 0);
    return [];
  };

  const fm = args.fmOptions;
  const llmsKey: 'llms' | 'targets' = (fm !== undefined && (Object.prototype.hasOwnProperty.call(fm, 'llms'))) ? 'llms'
    : ((fm !== undefined && (Object.prototype.hasOwnProperty.call(fm, 'targets'))) ? 'targets' : 'llms');
  const llmsVal: string[] = (fm !== undefined && llmsKey === 'llms') ? toArray(fm.llms)
    : (fm !== undefined && llmsKey === 'targets') ? toArray(fm.targets)
    : [];
  const toolsVal: string[] = (fm !== undefined) ? toArray(fm.tools) : [];
  const agentsVal: string[] = (fm !== undefined) ? toArray(fm.agents) : [];

  const tpl: Record<string, unknown> = {};
  tpl.description = args.description;
  tpl.usage = args.usage;
  tpl[llmsKey] = llmsVal;
  tpl.tools = toolsVal;
  // Always include agents so users see the key in the template
  tpl.agents = agentsVal.length > 0 ? agentsVal : [];
  tpl.temperature = args.numbers.temperature;
  tpl.topP = args.numbers.topP;
  tpl.llmTimeout = args.numbers.llmTimeout;
  tpl.toolTimeout = args.numbers.toolTimeout;
  tpl.toolResponseMaxBytes = args.numbers.toolResponseMaxBytes;
  tpl.maxRetries = args.numbers.maxRetries;
  tpl.maxToolTurns = args.numbers.maxToolTurns;
  // Exclude runtime/CLI-wide toggles from frontmatter template to avoid confusion
  // (traceLLM, traceMCP, verbose, accounting, save, load, stream)
  // Keep parallelToolCalls as it influences agent tool behavior.
  tpl.parallelToolCalls = args.booleans.parallelToolCalls;
  if (args.output !== undefined) tpl.output = args.output;
  return tpl;
}
