import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import Ajv from 'ajv';

import type {
  AIAgentCallbacks,
  AIAgentResult,
  AIAgentSessionConfig,
  ConversationMessage,
  AccountingEntry,
  MCPTool,
} from './types.js';

import { loadAgent } from './agent-loader.js';
import { parseFrontmatter, stripFrontmatter } from './frontmatter.js';
import { isReservedAgentName } from './internal-tools.js';

interface ChildInfo {
  toolName: string;
  description?: string;
  usage?: string;
  inputFormat: 'text' | 'json';
  inputSchema?: Record<string, unknown>;
  promptPath: string; // canonical path
  systemTemplate: string; // stripped frontmatter contents (unexpanded, no FORMAT replacement)
  // A loader-produced runner will be resolved lazily when executed
}

function canonical(p: string): string {
  try { return fs.realpathSync(p); } catch { return p; }
}

function sanitizeToolName(name: string): string {
  return name
    .replace(/[^A-Za-z0-9_.-]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toLowerCase();
}

export class SubAgentRegistry {
  private readonly children = new Map<string, ChildInfo>(); // toolName -> info

  constructor(private readonly baseDir?: string, private readonly ancestors: string[] = []) {}

  load(paths: string[]): void {
    paths.forEach((p) => { this.loadOne(p); });
  }

  private loadOne(p: string): void {
    const resolved = path.resolve(this.baseDir ?? process.cwd(), p);
    const id = canonical(resolved);
    // Recursion prevention: cannot attach an agent that's already in ancestors
    if (this.ancestors.includes(id)) {
      throw new Error(`Recursion detected while loading sub-agent: ${id}`);
    }
    const content = fs.readFileSync(id, 'utf-8');
    let fm;
    try {
      fm = parseFrontmatter(content, { baseDir: path.dirname(id) });
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new Error(`Sub-agent '${id}' frontmatter error: ${msg}`);
    }
    // Refuse to load without description (usage is optional)
    if (fm === undefined || typeof fm.description !== 'string' || fm.description.trim().length === 0) {
      throw new Error(`Sub-agent '${id}' missing 'description' in frontmatter`);
    }
    // Determine tool name
    const fileBase = path.basename(id, path.extname(id));
    const baseName = fileBase;
    const toolName = sanitizeToolName(baseName);
    if (isReservedAgentName(toolName)) {
      throw new Error(`Sub-agent '${id}' uses a reserved tool name '${toolName}'`);
    }
    if (this.children.has(toolName)) {
      throw new Error(`Duplicate sub-agent tool name '${toolName}' from '${id}' conflicts with an existing sub-agent`);
    }
    // Max recursion = 0: sub-agents cannot declare further sub-agents
    const childAgents: string[] = (() => {
      const anyOpts = fm.options as Record<string, unknown> | undefined;
      if (anyOpts !== undefined) {
        const a = anyOpts.agents;
        if (Array.isArray(a)) return (a as unknown[]).map((x) => String(x));
        if (typeof a === 'string' && a.length > 0) return [a];
      }
      return [];
    })();
    if (childAgents.length > 0) {
      throw new Error(`Sub-agent '${id}' declares its own agents; recursion is not allowed`);
    }
    // Determine input format/schema
    const inputFmt = fm.inputSpec?.format ?? 'text';
    const inputSchema = fm.inputSpec?.schema;
    // Keep template unmodified; ai-agent will centrally handle ${FORMAT} and variables
    const systemTemplate = stripFrontmatter(content);
    const info: ChildInfo = {
      toolName,
      description: fm.description,
      usage: fm.usage,
      inputFormat: inputFmt,
      inputSchema,
      promptPath: id,
      systemTemplate,
    };
    this.children.set(toolName, info);
  }

  getTools(): MCPTool[] {
    return Array.from(this.children.values()).map((c) => {
      const inputSchema = ((): Record<string, unknown> => {
        if (c.inputFormat === 'json') return (c.inputSchema ?? { type: 'object' });
        return {
          type: 'object',
          additionalProperties: false,
          required: ['input'],
          properties: {
            input: { type: 'string', minLength: 1, description: (typeof c.usage === 'string' && c.usage.length > 0) ? c.usage : 'User prompt for the sub-agent' },
          },
        } as Record<string, unknown>;
      })();
      return {
        name: `agent__${c.toolName}`,
        description: c.description ?? `Sub-agent tool for ${c.toolName}`,
        inputSchema,
      } as MCPTool;
    });
  }

  public getPromptPaths(): string[] {
    return Array.from(this.children.values()).map((c) => c.promptPath);
  }

  hasTool(exposedToolName: string): boolean {
    const name = exposedToolName.startsWith('agent__') ? exposedToolName.slice('agent__'.length) : exposedToolName;
    return this.children.has(name);
  }

  // Execute a child agent and return its serialized result (string) and child id
  async execute(
    exposedToolName: string,
    parameters: Record<string, unknown>,
    parentSession: Pick<AIAgentSessionConfig, 'config' | 'callbacks' | 'stream' | 'traceLLM' | 'traceMCP' | 'verbose' | 'temperature' | 'topP' | 'llmTimeout' | 'toolTimeout' | 'maxRetries' | 'maxTurns' | 'toolResponseMaxBytes' | 'parallelToolCalls' | 'targets'> & {
      // extra trace/metadata for child
      trace?: { originId?: string; parentId?: string; callPath?: string };
    }
  ): Promise<{ result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } }> {
    const name = exposedToolName.startsWith('agent__') ? exposedToolName.slice('agent__'.length) : exposedToolName;
    const info = this.children.get(name);
    if (info === undefined) throw new Error(`Unknown sub-agent: ${exposedToolName}`);

    // Validate and build user prompt from parameters
    const userPrompt: string = ((): string => {
      if (info.inputFormat === 'json') {
        // Validate JSON params when schema provided
        if (info.inputSchema !== undefined) {
          try {
            const ajv = new Ajv({ allErrors: true, strict: false });
            const validate = ajv.compile(info.inputSchema);
            const ok = validate(parameters);
            if (!ok) {
              const errs = (validate.errors ?? []).map((e) => `${e.instancePath} ${e.message ?? ''}`.trim()).join('; ');
              throw new Error(`invalid_parameters: ${errs}`);
            }
          } catch (e) {
            if (e instanceof Error && e.message.startsWith('invalid_parameters:')) throw e;
            throw new Error('invalid_parameters: validation_failed');
          }
        }
        try { return JSON.stringify(parameters); } catch { return '[unserializable-parameters]'; }
      }
      const v = (parameters as { input?: unknown }).input;
      if (typeof v !== 'string' || v.length === 0) {
        throw new Error('invalid_parameters: input (string) is required');
      }
      return v;
    })();

    // Load child agent with layered configuration using the file path
    // CD to agent directory for proper layered config resolution
    const prevCwd = process.cwd();
    let result: AIAgentResult;
    try {
      process.chdir(path.dirname(info.promptPath));
      // Decide targets inheritance: only when child has no models in frontmatter
      let inheritTargets: { provider: string; model: string }[] | undefined = undefined;
      try {
        const raw = fs.readFileSync(info.promptPath, 'utf-8');
        const fm2 = parseFrontmatter(raw, { baseDir: path.dirname(info.promptPath) });
        const models = (fm2?.options as { models?: unknown } | undefined)?.models;
        const hasModels = (Array.isArray(models) && models.length > 0) || typeof models === 'string';
        inheritTargets = hasModels ? undefined : parentSession.targets;
      } catch { /* ignore */ }

      const loaded = loadAgent(info.promptPath, undefined, {
        configPath: undefined,
        verbose: parentSession.verbose ?? (parentSession.callbacks !== undefined),
        targets: inheritTargets,
        // All models overrides (propagate globally)
        stream: parentSession.stream,
        traceLLM: parentSession.traceLLM,
        traceMCP: parentSession.traceMCP,
        // Master defaults for sub-agents (apply only when undefined)
        defaultsForUndefined: {
          temperature: parentSession.temperature,
          topP: parentSession.topP,
          maxOutputTokens: (parentSession as { maxOutputTokens?: number }).maxOutputTokens,
          repeatPenalty: (parentSession as { repeatPenalty?: number }).repeatPenalty,
          llmTimeout: parentSession.llmTimeout,
          toolTimeout: parentSession.toolTimeout,
          maxRetries: parentSession.maxRetries,
          maxToolTurns: parentSession.maxTurns,
          toolResponseMaxBytes: parentSession.toolResponseMaxBytes,
          parallelToolCalls: parentSession.parallelToolCalls,
        },
      });

      // Build callbacks wrapper to prefix child logs
      const orig = parentSession.callbacks;
      const childPrefix = `child:${info.toolName}`;
      const cb: AIAgentCallbacks | undefined = orig === undefined ? undefined : {
        onLog: (e) => {
          const cloned = { ...e };
          cloned.remoteIdentifier = `${childPrefix}:${e.remoteIdentifier}`;
          orig.onLog?.(cloned);
        },
        onOutput: (t) => { orig.onOutput?.(t); },
        onThinking: (t) => { orig.onThinking?.(t); },
        onAccounting: (a) => { orig.onAccounting?.(a); }
      };

      const history: ConversationMessage[] | undefined = undefined;
      // Ensure child gets a fresh selfId (span id)
      const childTrace = { selfId: crypto.randomUUID(), originId: parentSession.trace?.originId, parentId: parentSession.trace?.parentId, callPath: parentSession.trace?.callPath };
      result = await loaded.run(info.systemTemplate, userPrompt, { history, callbacks: cb, trace: childTrace, renderTarget: 'sub-agent', outputFormat: 'sub-agent' });
    } finally {
      try { process.chdir(prevCwd); } catch { /* ignore */ }
    }

    // Prefer JSON content when available
    let out = '';
    if (result.finalReport?.format === 'json' && result.finalReport.content_json !== undefined) {
      out = JSON.stringify(result.finalReport.content_json);
    } else if (typeof result.finalReport?.content === 'string') {
      out = result.finalReport.content;
    } else {
      // Fallback: concatenate last assistant content
      const last = [...result.conversation].filter((m) => m.role === 'assistant').pop();
      out = last?.content ?? '';
    }

    const convo: ConversationMessage[] = result.conversation;
    const acct: AccountingEntry[] = result.accounting;
    const firstAcct = result.accounting.find((a) => typeof a.txnId === "string" || typeof a.originTxnId === "string" || typeof a.parentTxnId === "string" || typeof a.callPath === "string");
    const trace: { originId?: string; parentId?: string; selfId?: string; callPath?: string } = { originId: firstAcct?.originTxnId, parentId: firstAcct?.parentTxnId, selfId: firstAcct?.txnId, callPath: firstAcct?.callPath };
    const payload: { result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } } = { result: out, child: info, accounting: acct, conversation: convo, trace };
    return payload;
  }
}
