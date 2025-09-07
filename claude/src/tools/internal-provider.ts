import Ajv from 'ajv';

import type { MCPTool } from '../types.js';
import type { ToolsOrchestrator } from './tools.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';

import { ToolProvider } from './types.js';

export class InternalToolProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  private readonly ajv = new Ajv({ allErrors: true, strict: false });

  constructor(
    public readonly id: string,
    private readonly opts: {
      enableBatch: boolean;
      expectedJsonSchema?: Record<string, unknown>;
      // Callbacks into session
      appendNotes: (text: string, tags?: string[]) => void;
      setFinalReport: (payload: { status: 'success'|'failure'|'partial'; format: string; content?: string; content_json?: Record<string, unknown>; metadata?: Record<string, unknown>; messages?: unknown[] }) => void;
      orchestrator: ToolsOrchestrator;
      toolTimeoutMs?: number;
    }
  ) { super(); }

  listTools(): MCPTool[] {
    const tools: MCPTool[] = [
      {
        name: 'agent__append_notes',
        description: 'Housekeeping notes. Use sparingly for brief interim findings.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['text'],
          properties: {
            text: { type: 'string', minLength: 1 },
            tags: { type: 'array', items: { type: 'string' } },
          },
        },
      },
      {
        name: 'agent__final_report',
        description: 'Finish the session by returning the final answer.',
        inputSchema: {
          type: 'object',
          additionalProperties: true,
          required: ['status', 'format'],
          properties: {
            status: { type: 'string', enum: ['success', 'failure', 'partial'] },
            format: { type: 'string', enum: ['json', 'markdown', 'markdown+mermaid', 'slack', 'tty', 'pipe', 'sub-agent', 'text'] },
            content: { type: 'string' },
            content_json: { type: 'object' },
            metadata: { type: 'object' },
            messages: { type: 'array', items: { type: 'object', additionalProperties: true } }
          }
        }
      }
    ];
    if (this.opts.enableBatch) {
      tools.push({
        name: 'agent__batch',
        description: 'Execute multiple tools in one call (always parallel). Use exposed tool names.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['calls'],
          properties: {
            calls: {
              type: 'array',
              items: {
                type: 'object',
                additionalProperties: true,
                required: ['id', 'tool', 'args'],
                properties: {
                  id: { oneOf: [ { type: 'string', minLength: 1 }, { type: 'number' } ] },
                  tool: { type: 'string', minLength: 1, description: 'Exposed tool name (e.g., brave__brave_web_search)' },
                  args: { type: 'object', additionalProperties: true }
                }
              }
            }
          }
        }
      });
    }
    return tools;
  }

  hasTool(name: string): boolean {
    return name === 'agent__append_notes' || name === 'agent__final_report' || (this.opts.enableBatch && name === 'agent__batch');
  }

  async execute(name: string, args: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    if (name === 'agent__append_notes') {
      const text = typeof (args.text) === 'string' ? args.text : '';
      const tags = Array.isArray(args.tags) ? (args.tags as unknown[]).filter((t): t is string => typeof t === 'string') : undefined;
      if (text.trim().length > 0) this.opts.appendNotes(text, tags);
      return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    if (name === 'agent__final_report') {
      const status = (typeof args.status === 'string' ? args.status : 'success') as 'success'|'failure'|'partial';
      const format = (typeof args.format === 'string' ? args.format : 'markdown');
      const content = typeof args.content === 'string' ? args.content : undefined;
      const content_json = (args.content_json !== null && typeof args.content_json === 'object' && !Array.isArray(args.content_json)) ? (args.content_json as Record<string, unknown>) : undefined;
      const metadata = (args.metadata !== null && typeof args.metadata === 'object' && !Array.isArray(args.metadata)) ? (args.metadata as Record<string, unknown>) : undefined;
      const messages = Array.isArray(args.messages) ? args.messages : undefined;
      // Enforce required payload per format
      if (format === 'slack') {
        // Accept either structured messages or a plain content fallback, then normalize to metadata.slack.messages
        const normalizedMessages: unknown[] = (() => {
          if (Array.isArray(messages) && messages.length > 0) {
            const msgs: unknown[] = messages;
            return msgs.filter((m) => m !== null && typeof m === 'object');
          }
          if (typeof content === 'string' && content.trim().length > 0) {
            return [ { blocks: [ { type: 'section', text: { type: 'mrkdwn', text: content } } ] } ];
          }
          return [];
        })();
        if (normalizedMessages.length === 0) throw new Error('final_report(slack) requires `messages` or non-empty `content`.');
        const metaBase: Record<string, unknown> = (metadata ?? {});
        const slackVal = (metaBase as Record<string, unknown> & { slack?: unknown }).slack;
        const slackExisting: Record<string, unknown> = (slackVal !== undefined && slackVal !== null && typeof slackVal === 'object' && !Array.isArray(slackVal)) ? (slackVal as Record<string, unknown>) : {};
        const metaSlack: Record<string, unknown> = { ...slackExisting };
        metaSlack.messages = normalizedMessages;
        const mergedMeta: Record<string, unknown> = { ...metaBase, slack: metaSlack };
        this.opts.setFinalReport({ status, format, content, content_json, metadata: mergedMeta, messages: normalizedMessages });
        return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
      } else if (format === 'json') {
        if (content_json === undefined) throw new Error('final_report(json) requires `content_json` (object).');
        // Optional schema validation
        if (this.opts.expectedJsonSchema !== undefined) {
          const validate = this.ajv.compile(this.opts.expectedJsonSchema);
          const ok = validate(content_json);
          if (!ok) {
            const errs = (validate.errors ?? []).map((e) => {
              const inst = typeof e.instancePath === 'string' ? e.instancePath : '';
              const msg = typeof e.message === 'string' ? e.message : '';
              return `${inst} ${msg}`.trim();
            }).join('; ');
            throw new Error(`final_report(json) schema validation failed: ${errs}`);
          }
        }
      } else {
        if (typeof content !== 'string' || content.trim().length === 0) throw new Error(`final_report(${format}) requires non-empty content.`);
      }
      this.opts.setFinalReport({ status, format, content, content_json, metadata, messages });
      return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    if (this.opts.enableBatch && name === 'agent__batch') {
      // Minimal batch: always use orchestrator for inner calls
      const calls = Array.isArray((args as { calls?: unknown }).calls) ? ((args as { calls: unknown[] }).calls) : [];
      interface R { id: string; tool: string; ok: boolean; elapsedMs: number; output?: string; error?: { code: string; message: string } }
      const results: R[] = await Promise.all(calls.map(async (cUnknown) => {
        const c = (cUnknown !== null && typeof cUnknown === 'object') ? (cUnknown as Record<string, unknown>) : {};
        const id = typeof c.id === 'string' ? c.id : (typeof c.id === 'number' ? String(c.id) : '');
        const tool = typeof c.tool === 'string' ? c.tool : '';
        const a = (c.args !== null && typeof c.args === 'object') ? (c.args as Record<string, unknown>) : {};
        const t0 = Date.now();
        if (tool === 'agent__append_notes' || tool === 'agent__final_report' || tool === 'agent__batch') return { id, tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } };
        try {
          if (!(this.opts.orchestrator.hasTool(tool))) return { id, tool, ok: false, elapsedMs: 0, error: { code: 'UNKNOWN_TOOL', message: `Unknown tool: ${tool}` } };
          const managed = await this.opts.orchestrator.executeWithManagement(tool, a, { turn: 0, subturn: 0 }, { timeoutMs: this.opts.toolTimeoutMs });
          return { id, tool, ok: true, elapsedMs: managed.latency, output: managed.result };
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          return { id, tool, ok: false, elapsedMs: Date.now() - t0, error: { code: 'EXECUTION_ERROR', message: msg } };
        }
      }));
      const payload = JSON.stringify({ results });
      return { ok: true, result: payload, latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    throw new Error(`Unknown internal tool: ${name}`);
  }
}
