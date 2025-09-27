import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import Ajv from 'ajv';

import type { LoadedAgent } from './agent-loader.js';
import type { SessionNode } from './session-tree.js';
import type {
  AIAgentCallbacks,
  AIAgentResult,
  AIAgentSessionConfig,
  ConversationMessage,
  AccountingEntry,
  MCPTool,
} from './types.js';

import { loadAgentFromContent } from './agent-loader.js';
import { parseFrontmatter, stripFrontmatter, parsePairs } from './frontmatter.js';
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
  loaded?: LoadedAgent; // Static snapshot runner (no hot-reload within a session)
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
  private readonly opts: { traceLLM?: boolean; traceMCP?: boolean; verbose?: boolean };

  constructor(private readonly baseDir?: string, private readonly ancestors: string[] = [], opts?: { traceLLM?: boolean; traceMCP?: boolean; verbose?: boolean }) {
    this.opts = opts ?? {};
  }

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
    // const childAgents: string[] = (() => {
    //   const anyOpts = fm.options as Record<string, unknown> | undefined;
    //   if (anyOpts !== undefined) {
    //     const a = anyOpts.agents;
    //     if (Array.isArray(a)) return (a as unknown[]).map((x) => String(x));
    //     if (typeof a === 'string' && a.length > 0) return [a];
    //   }
    //   return [];
    // })();
    // Allow nested sub-agents; recursion cycles will be prevented by ancestors at load-time of child sessions.
    // Determine input format/schema
    const inputFmt = fm.inputSpec?.format ?? 'text';
    const inputSchema = fm.inputSpec?.schema;
    // Keep template unmodified; ai-agent will centrally handle ${FORMAT} and variables
    const systemTemplate = stripFrontmatter(content);
    // Enforce models presence in frontmatter; sub-agents must declare their own targets
    try {
      const models = parsePairs((fm.options as { models?: unknown } | undefined)?.models);
      if (!Array.isArray(models) || models.length === 0) {
        throw new Error('missing models');
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new Error(`Sub-agent '${id}' missing or invalid models: ${msg}`);
    }
    const info: ChildInfo = {
      toolName,
      description: fm.description,
      usage: fm.usage,
      inputFormat: inputFmt,
      inputSchema,
      promptPath: id,
      systemTemplate,
    };
    // Build a static snapshot runner now (no dynamic reload during session)
    try {
      const loaded = loadAgentFromContent(id, content, { baseDir: path.dirname(id), traceLLM: this.opts.traceLLM, traceMCP: this.opts.traceMCP, verbose: this.opts.verbose });
      info.loaded = loaded;
      // Prefer the fully resolved system template from the snapshot (includes resolved statically)
      info.systemTemplate = loaded.systemTemplate;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new Error(`Failed to load sub-agent '${id}' at registry time: ${msg}`);
    }
    this.children.set(toolName, info);
  }

  getTools(): MCPTool[] {
    return Array.from(this.children.values()).map((c) => {
      const inputSchema = (() => {
        const reasonProp = { type: 'string', minLength: 1, description: '3-7 words about the reason of running this tool' };
        if (c.inputFormat === 'json') {
          const base = (c.inputSchema ?? { type: 'object' });
          return { allOf: [ base, { type: 'object', required: ['reason'], properties: { reason: reasonProp } } ] };
        }
        return { type: 'object', additionalProperties: false, required: ['input', 'reason'], properties: { input: { type: 'string', minLength: 1, description: (typeof c.usage === 'string' && c.usage.length > 0) ? c.usage : 'User prompt for the sub-agent' }, reason: reasonProp } };
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
      // control signals to propagate
      abortSignal?: AbortSignal;
      stopRef?: { stopping: boolean };
      // live updates: stream child opTree snapshots to parent orchestrator
      onChildOpTree?: (tree: SessionNode) => void;
    },
    opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string }
  ): Promise<{ result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string }, opTree?: SessionNode }> {
    const name = exposedToolName.startsWith('agent__') ? exposedToolName.slice('agent__'.length) : exposedToolName;
    const info = this.children.get(name);
    if (info === undefined) throw new Error(`Unknown sub-agent: ${exposedToolName}`);

    // Validate and build user prompt from parameters
    const userPrompt: string = (() => {
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
      const v = (parameters as { input?: unknown; reason?: unknown }).input;
      if (typeof v !== 'string' || v.length === 0) {
        throw new Error('invalid_parameters: input (string) is required');
      }
      return v;
    })();
    const reason: string | undefined = (() => { const r = (parameters as { reason?: unknown }).reason; return typeof r === 'string' ? r : undefined; })();

    // Load child agent with layered configuration using the file path
    // CD to agent directory for proper layered config resolution
    let result: AIAgentResult;
    try {
      const loaded = info.loaded;
      if (loaded === undefined) throw new Error('sub-agent snapshot not available');
      // Build callbacks wrapper to prefix child logs
      const orig = parentSession.callbacks;
      const childPrefix = `child:${info.toolName}`;
      const cb: AIAgentCallbacks | undefined = orig === undefined ? undefined : {
        onLog: (e) => {
          const cloned = { ...e };
          // Preserve 'agent:title' identifier exactly so aggregators can detect titles per agent
          if (e.remoteIdentifier !== 'agent:title') {
            cloned.remoteIdentifier = `${childPrefix}:${e.remoteIdentifier}`;
          }
          // Prefix opTree-provided path labels with parent op path so logs are hierarchically greppable
          try {
            const parentPath = (opts !== undefined && typeof opts.parentOpPath === 'string' && opts.parentOpPath.length > 0) ? opts.parentOpPath : undefined;
            const existingPath = (cloned as { path?: string }).path;
            if (typeof existingPath === 'string' && existingPath.length > 0) {
              const prefix = typeof parentPath === 'string' && parentPath.length > 0 ? parentPath : undefined;
              if (typeof prefix === 'string' && prefix.length > 0) (cloned as { path?: string }).path = `${prefix}.${existingPath}`;
            } else if (typeof parentPath === 'string' && parentPath.length > 0) {
              (cloned as { path?: string }).path = parentPath;
            }
          } catch { /* ignore */ }
          orig.onLog?.(cloned);
        },
        onOutput: (t) => { orig.onOutput?.(t); },
        onThinking: (_t) => { /* Suppress sub-agent reasoning for external consumers */ },
        onAccounting: (a) => { orig.onAccounting?.(a); },
        onProgress: (event) => { orig.onProgress?.(event); },
        onOpTree: (tree) => {
          // Forward to parent session manager and to orchestrator live callback
          try { orig.onOpTree?.(tree); } catch (e) { try { process.stderr.write(`[warn] child onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {} }
          try { parentSession.onChildOpTree?.(tree as SessionNode); } catch (e) { try { process.stderr.write(`[warn] parent onChildOpTree callback failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {} }
        }
      };

      const history: ConversationMessage[] | undefined = undefined;
      // Ensure child gets a fresh selfId (span id)
      // Extend callPath with the child tool name so live status shows hierarchy correctly
      const basePath = parentSession.trace?.callPath;
      const callPath = (typeof basePath === 'string' && basePath.length > 0) ? `${basePath}->${info.toolName}` : info.toolName;
      const childTrace = { selfId: crypto.randomUUID(), originId: parentSession.trace?.originId, parentId: parentSession.trace?.parentId, callPath };
      result = await loaded.run(loaded.systemTemplate, userPrompt, {
        history,
        callbacks: cb,
        trace: childTrace,
        renderTarget: 'sub-agent',
        outputFormat: 'sub-agent',
        initialTitle: reason,
        // propagate control signals from parent session
        abortSignal: parentSession.abortSignal,
        stopRef: parentSession.stopRef,
        // propagate ancestors to prevent recursion cycles in nested sessions
        ancestors: [...this.ancestors, info.promptPath]
      });
    } finally {
      // no chdir in static mode
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
    const payload: { result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string }, opTree?: SessionNode } = { result: out, child: info, accounting: acct, conversation: convo, trace, opTree: (result as { opTree?: SessionNode }).opTree };
    return payload;
  }
}
