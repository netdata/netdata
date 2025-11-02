import crypto from 'node:crypto';

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
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

type AjvInstance = AjvClass;
type AjvErrorObject = ErrorObject<string, Record<string, unknown>>;
type AjvConstructor = new (options?: AjvOptions) => AjvInstance;
const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;

import { DEFAULT_TOOL_INPUT_SCHEMA, cloneJsonSchema } from './input-contract.js';
import { appendCallPathSegment } from './utils.js';

export interface PreloadedSubAgent {
  toolName: string;
  description?: string;
  usage?: string;
  inputFormat: 'text' | 'json';
  inputSchema?: Record<string, unknown>;
  hasExplicitInputSchema: boolean;
  promptPath: string; // canonical path
  systemTemplate: string; // stripped frontmatter contents (unexpanded, no FORMAT replacement)
  loaded: LoadedAgent; // Static snapshot runner (no hot-reload within a session)
}

type ChildInfo = PreloadedSubAgent;

function augmentSchemaWithReason(
  source: Record<string, unknown> | undefined,
  reasonProp: Record<string, unknown>
): Record<string, unknown> {
  const schema = cloneJsonSchema(source ?? {});
  const props = (() => {
    const container = (schema as { properties?: unknown }).properties;
    if (container !== undefined && container !== null && typeof container === 'object' && !Array.isArray(container)) {
      return container as Record<string, unknown>;
    }
    const created: Record<string, unknown> = {};
    (schema as { properties?: Record<string, unknown> }).properties = created;
    return created;
  })();
  if (props.reason === undefined || typeof props.reason !== 'object' || Array.isArray(props.reason)) {
    props.reason = reasonProp;
  }
  const requiredRaw = (schema as { required?: unknown }).required;
  const required = new Set<string>(
    Array.isArray(requiredRaw)
      ? requiredRaw.filter((value): value is string => typeof value === 'string')
      : [],
  );
  required.add('reason');
  (schema as { required?: string[] }).required = Array.from(required);
  return schema;
}

function buildFallbackSchema(
  reasonProp: Record<string, unknown>,
  usage?: string
): Record<string, unknown> {
  const schema = cloneJsonSchema(DEFAULT_TOOL_INPUT_SCHEMA);
  const props = (() => {
    const container = (schema as { properties?: unknown }).properties;
    if (container !== undefined && container !== null && typeof container === 'object' && !Array.isArray(container)) {
      return container as Record<string, unknown>;
    }
    const created: Record<string, unknown> = {};
    (schema as { properties?: Record<string, unknown> }).properties = created;
    return created;
  })();
  props.prompt = (() => {
    const original = props.prompt;
    if (original !== undefined && original !== null && typeof original === 'object' && !Array.isArray(original)) {
      if (typeof usage === 'string' && usage.trim().length > 0) {
        (original as { description?: string }).description = usage;
      }
      return original;
    }
    const created: Record<string, unknown> = { type: 'string' };
    if (typeof usage === 'string' && usage.trim().length > 0) {
      created.description = usage;
    }
    return created;
  })();
  props.reason = reasonProp;
  props.format = {
    type: 'string',
    enum: ['sub-agent'],
    default: 'sub-agent',
    description: 'Fixed format identifier; must be set to sub-agent.',
  };
  (schema as { additionalProperties?: boolean }).additionalProperties = false;
  const requiredRaw = (schema as { required?: unknown }).required;
  const required = new Set<string>(
    Array.isArray(requiredRaw)
      ? requiredRaw.filter((value): value is string => typeof value === 'string')
      : [],
  );
  required.add('prompt');
  required.add('reason');
  required.add('format');
  (schema as { required?: string[] }).required = Array.from(required);
  return schema;
}

export class SubAgentRegistry {
  private readonly children = new Map<string, ChildInfo>(); // toolName -> info

  constructor(children: readonly ChildInfo[], private readonly ancestors: string[] = []) {
    children.forEach((child) => {
      if (this.ancestors.includes(child.promptPath)) {
        throw new Error(`Recursion detected while loading sub-agent: ${child.promptPath}`);
      }
      if (this.children.has(child.toolName)) {
        throw new Error(`Duplicate sub-agent tool name '${child.toolName}' detected during preload.`);
      }
      this.children.set(child.toolName, child);
    });
  }

  getTools(): MCPTool[] {
    return Array.from(this.children.values()).map((c) => {
      const inputSchema = (() => {
        const reasonProp = { type: 'string', minLength: 1, description: '3-7 words about the reason of running this tool' };
        if (c.hasExplicitInputSchema) {
          return augmentSchemaWithReason(c.inputSchema, reasonProp);
        }
        return buildFallbackSchema(reasonProp, c.usage);
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

  listToolNames(): string[] {
    return Array.from(this.children.keys()).map((name) => `agent__${name}`);
  }

  // Execute a child agent and return its serialized result (string) and child id
  async execute(
    exposedToolName: string,
    parameters: Record<string, unknown>,
    parentSession: Pick<AIAgentSessionConfig, 'config' | 'callbacks' | 'stream' | 'traceLLM' | 'traceMCP' | 'traceSdk' | 'verbose' | 'temperature' | 'topP' | 'llmTimeout' | 'toolTimeout' | 'maxRetries' | 'maxTurns' | 'toolResponseMaxBytes' | 'parallelToolCalls' | 'targets'> & {
      // extra trace/metadata for child
      trace?: { originId?: string; parentId?: string; callPath?: string; agentPath?: string; turnPath?: string };
      // control signals to propagate
      abortSignal?: AbortSignal;
      stopRef?: { stopping: boolean };
      // live updates: stream child opTree snapshots to parent orchestrator
      onChildOpTree?: (tree: SessionNode) => void;
      agentPath?: string;
      turnPathPrefix?: string;
    },
    opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string; parentTurnPath?: string }
  ): Promise<{ result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string }, opTree?: SessionNode }> {
    const name = exposedToolName.startsWith('agent__') ? exposedToolName.slice('agent__'.length) : exposedToolName;
    const info = this.children.get(name);
    if (info === undefined) throw new Error(`Unknown sub-agent: ${exposedToolName}`);

    // Validate and build user prompt from parameters
    const userPrompt: string = (() => {
      if (info.inputFormat === 'json') {
        if (info.inputSchema !== undefined) {
          const reasonProp = { type: 'string', minLength: 1, description: '3-7 words about the reason of running this tool' };
          const schemaForValidation = info.hasExplicitInputSchema
            ? augmentSchemaWithReason(info.inputSchema, reasonProp)
            : buildFallbackSchema(reasonProp, info.usage);
          try {
            const ajv: AjvInstance = new AjvCtor({ allErrors: true, strict: false });
            const validate = ajv.compile(schemaForValidation);
            const ok = validate(parameters);
            if (!ok) {
              const errors = Array.isArray(validate.errors) ? (validate.errors as AjvErrorObject[]) : [];
              const errs = errors.map((error) => `${error.instancePath} ${error.message ?? ''}`.trim()).join('; ');
              throw new Error(`invalid_parameters: ${errs}`);
            }
          } catch (error) {
            if (error instanceof Error && error.message.startsWith('invalid_parameters:')) throw error;
            throw new Error('invalid_parameters: validation_failed');
          }
        }
        try { return JSON.stringify(parameters); } catch { return '[unserializable-parameters]'; }
      }
      const formatParam = (parameters as { format?: unknown }).format;
      if (formatParam !== 'sub-agent') {
        throw new Error('invalid_parameters: format must be "sub-agent"');
      }
      const promptParam = (parameters as { prompt?: unknown }).prompt;
      if (typeof promptParam !== 'string' || promptParam.trim().length === 0) {
        throw new Error('invalid_parameters: prompt (string) is required');
      }
      return promptParam;
    })();
    const reason: string | undefined = (() => {
      const r = (parameters as { reason?: unknown }).reason;
      if (typeof r !== 'string' || r.trim().length === 0) {
        throw new Error('invalid_parameters: reason (string) is required');
      }
      return r;
    })();

    // Load child agent with layered configuration using the file path
    // CD to agent directory for proper layered config resolution
    let result: AIAgentResult;
    try {
      const loaded = info.loaded;
      // Build callbacks wrapper so parent observers still receive child events
      const orig = parentSession.callbacks;
      const cb: AIAgentCallbacks | undefined = orig === undefined ? undefined : {
        onLog: (e) => {
          const cloned = { ...e };
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
      const parentAgentPathCandidates = [
        parentSession.agentPath,
        parentSession.trace?.agentPath,
        parentSession.trace?.callPath,
      ].filter((value): value is string => typeof value === 'string' && value.length > 0);
      const parentAgentPath = parentAgentPathCandidates.length > 0 ? parentAgentPathCandidates[0] : 'agent';
      const childAgentName = info.toolName;
      const childAgentPath = appendCallPathSegment(parentAgentPath, childAgentName);
      const inheritedTurnPathCandidates = [
        opts?.parentTurnPath,
        parentSession.turnPathPrefix,
        parentSession.trace?.turnPath,
      ].filter((value): value is string => typeof value === 'string' && value.length > 0);
      const inheritedTurnPath = inheritedTurnPathCandidates.length > 0 ? inheritedTurnPathCandidates[0] : '';
      const childTrace = {
        selfId: crypto.randomUUID(),
        originId: parentSession.trace?.originId,
        parentId: parentSession.trace?.parentId,
        callPath: childAgentPath,
        agentPath: childAgentPath,
        turnPath: inheritedTurnPath,
      };
      const outputFormat = (() => {
        const requestedFormat = (parameters as { format?: unknown }).format;
        if (requestedFormat === 'sub-agent') return 'sub-agent';

        const fmt = loaded.expectedOutput?.format;
        if (fmt === 'json') return 'json';
        if (fmt === 'markdown') return 'markdown';
        if (fmt === 'text') return 'pipe';
        return 'markdown';
      })();
      result = await loaded.run(loaded.systemTemplate, userPrompt, {
        history,
        callbacks: cb,
        trace: childTrace,
        renderTarget: 'sub-agent',
        outputFormat,
        initialTitle: reason,
        agentPath: childAgentPath,
        turnPathPrefix: inheritedTurnPath,
        // propagate control signals from parent session
        abortSignal: parentSession.abortSignal,
        stopRef: parentSession.stopRef,
        // propagate ancestors to prevent recursion cycles in nested sessions
        ancestors: [...this.ancestors, info.promptPath],
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
    const firstAcct = result.accounting.find((a) => typeof a.txnId === 'string' || typeof a.originTxnId === 'string' || typeof a.parentTxnId === 'string' || typeof a.callPath === 'string');
    const trace: { originId?: string; parentId?: string; selfId?: string; callPath?: string } = { originId: firstAcct?.originTxnId, parentId: firstAcct?.parentTxnId, selfId: firstAcct?.txnId, callPath: firstAcct?.callPath };
    const payload: { result: string; child: ChildInfo; accounting: readonly AccountingEntry[]; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string }, opTree?: SessionNode } = { result: out, child: info, accounting: acct, conversation: convo, trace, opTree: (result as { opTree?: SessionNode }).opTree };
    return payload;
  }
}
