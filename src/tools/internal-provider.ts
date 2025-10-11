import Ajv from 'ajv';

import type { OutputFormatId } from '../formats.js';
import type { MCPTool } from '../types.js';
import type { ToolsOrchestrator } from './tools.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';

import { describeFormatParameter, formatPromptValue } from '../formats.js';

import { ToolProvider } from './types.js';

interface InternalToolProviderOptions {
  enableBatch: boolean;
  outputFormat: OutputFormatId;
  expectedOutputFormat?: OutputFormatId;
  expectedJsonSchema?: Record<string, unknown>;
  updateStatus: (text: string) => void;
  setTitle: (title: string, emoji?: string) => void;
  setFinalReport: (payload: { status: 'success'|'failure'|'partial'; format: string; content?: string; content_json?: Record<string, unknown>; metadata?: Record<string, unknown>; messages?: unknown[] }) => void;
  logError: (message: string) => void;
  orchestrator: ToolsOrchestrator;
  getCurrentTurn: () => number;
  toolTimeoutMs?: number;
}

const PROGRESS_TOOL = 'agent__progress_report';
const FINAL_REPORT_TOOL = 'agent__final_report';
const BATCH_TOOL = 'agent__batch';
const SLACK_BLOCK_KIT_FORMAT: OutputFormatId = 'slack-block-kit';
const REPORT_FORMAT_LABEL = '`report_format`';

export class InternalToolProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  private readonly ajv = new Ajv({ allErrors: true, strict: false });
  private readonly formatId: OutputFormatId;
  private readonly formatDescription: string;
  private readonly instructions: string;

  constructor(
    public readonly id: string,
    private readonly opts: InternalToolProviderOptions
  ) {
    super();
    this.formatId = opts.outputFormat;
    this.formatDescription = describeFormatParameter(this.formatId);
    const expected = opts.expectedOutputFormat;
    if (expected !== undefined && expected !== this.formatId) {
      throw new Error(`Output format mismatch: expectedOutput.format=${expected} but session outputFormat=${this.formatId}`);
    }
    if (opts.expectedJsonSchema !== undefined && this.formatId !== 'json') {
      throw new Error('JSON schema provided but output format is not json');
    }
    if (this.formatId === 'json' && expected !== undefined && expected !== 'json') {
      throw new Error(`JSON output required but expected format is ${expected}`);
    }
    this.instructions = this.buildInstructions();
  }

  listTools(): MCPTool[] {
    const tools: MCPTool[] = [
      {
        name: PROGRESS_TOOL,
        description: 'Report current progress to user (max 15 words). MUST call in parallel with primary actions for real-time visibility.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['progress'],
          properties: {
            progress: { type: 'string', description: 'Brief progress message (max 15 words)' }
          },
        },
      },
      this.buildFinalReportTool()
    ];
    if (this.opts.enableBatch) {
      tools.push({
        name: BATCH_TOOL,
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
    return name === PROGRESS_TOOL || name === FINAL_REPORT_TOOL || (this.opts.enableBatch && name === BATCH_TOOL);
  }

  override getInstructions(): string {
    return this.instructions;
  }

  getFormatInfo(): { formatId: OutputFormatId; promptValue: string; parameterDescription: string } {
    return {
      formatId: this.formatId,
      promptValue: formatPromptValue(this.formatId),
      parameterDescription: this.formatDescription,
    };
  }

  private buildInstructions(): string {
    const lines: string[] = ['## INTERNAL TOOLS', ''];
    lines.push(`- Finish ONLY by calling \`${FINAL_REPORT_TOOL}\` exactly once.`);
    lines.push(`- Do NOT end with plain text. The session ends only after \`${FINAL_REPORT_TOOL}\`.`);
    lines.push('- Arguments:');
    lines.push('  - `status`: one of `success`, `failure`, `partial`.');
    if (this.formatId === 'json') {
      lines.push(`  - ${REPORT_FORMAT_LABEL}: "json".`);
      lines.push('  - `content_json`: MUST match the required JSON Schema exactly.');
    } else if (this.formatId === SLACK_BLOCK_KIT_FORMAT) {
      lines.push(`  - ${REPORT_FORMAT_LABEL}: "${SLACK_BLOCK_KIT_FORMAT}".`);
      lines.push('  - `messages`: array of Slack Block Kit messages (no plain `report_content`).');
      lines.push('    • Up to 20 messages, each with ≤50 blocks. Sections/context mrkdwn ≤2000 chars; headers plain_text ≤150.');
    } else {
      lines.push(`  - ${REPORT_FORMAT_LABEL}: "${this.formatId}".`);
      lines.push('  - `report_content`: complete deliverable in the requested format.');
    }
    lines.push('');
    lines.push(`- Use tool \`${PROGRESS_TOOL}\` to provide real-time progress updates and next steps.`);
    lines.push('  - Real-time updates keep users informed about ongoing actions and plans.');
    lines.push(`  - **CRITICAL**: When calling tools, you MUST call \`${PROGRESS_TOOL}\` in PARALLEL (same batch/turn).`);
    lines.push(`  - **CRITICAL**: You MUST NEVER call \`${PROGRESS_TOOL}\` alone.`);
    lines.push('  - Keep progress brief: max 20 words.');
    lines.push('  - Examples of good parallel usage:');
    lines.push('    When using batch:');
    lines.push('    {');
    lines.push('      "calls": [');
    lines.push(`        { "id": 1, "tool": "${PROGRESS_TOOL}", "args": { "progress": "Searching documentation for XYZ" } },`);
    lines.push('        { "id": 2, "tool": "jina__jina_search_web", "args": { "query": "..." } }');
    lines.push('      ]');
    lines.push('    }');
    lines.push('    When calling multiple tools:');
    lines.push(`    - Call \`${PROGRESS_TOOL}\` with "Analyzing config files for ABC"`);
    lines.push('    - AND call your actual analysis tools in the same turn');
    if (this.opts.enableBatch) {
      lines.push('');
      lines.push(`- Use tool \`${BATCH_TOOL}\` to call multiple tools in one go.`);
      lines.push('  - `calls[]`: items with `id`, `tool` (exposed name), `args`. Execution is parallel.');
      lines.push('  - Keep `args` minimal; they are validated against real schemas.');
      lines.push('  - Example with progress_report + searches:');
      lines.push('    {');
      lines.push('      "calls": [');
      lines.push(`        { "id": 1, "tool": "${PROGRESS_TOOL}", "args": { "progress": "Researching Netdata and eBPF" } },`);
      lines.push('        { "id": 2, "tool": "jina__jina_search_web", "args": { "query": "Netdata real-time monitoring features", "num": 10 } },');
      lines.push('        { "id": 3, "tool": "jina__jina_search_arxiv", "args": { "query": "eBPF monitoring 2024", "num": 20 } },');
      lines.push('        { "id": 4, "tool": "brave__brave_web_search", "args": { "query": "Netdata Cloud pricing", "count": 8, "offset": 0 } },');
      lines.push('        { "id": 5, "tool": "brave__brave_local_search", "args": { "query": "coffee near Athens", "count": 5 } },');
      lines.push('        { "id": 6, "tool": "fetcher__fetch_url", "args": { "url": "https://www.netdata.cloud" } },');
      lines.push('        { "id": 7, "tool": "fetcher__fetch_url", "args": { "url": "https://learn.netdata.cloud/" } }');
      lines.push('      ]');
      lines.push('    }');
    }
    return lines.join('\n');
  }

  private buildFinalReportTool(): MCPTool {
    const statusProp = { type: 'string', enum: ['success', 'failure', 'partial'] } as const;
    const metadataProp = { type: 'object' };
    const baseDescription = 'You MUST use agent__final_report to provide your final response to the user request.';

    if (this.formatId === 'json') {
      const schema = this.opts.expectedJsonSchema ?? { type: 'object' };
      return {
        name: FINAL_REPORT_TOOL,
        description: `${baseDescription} Use the exact JSON expected. ${this.formatDescription}`.trim(),
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['status', 'report_format', 'content_json'],
          properties: {
            status: statusProp,
            report_format: { type: 'string', const: 'json', description: this.formatDescription },
            content_json: schema,
            metadata: metadataProp,
          },
        },
      };
    }

    if (this.formatId === SLACK_BLOCK_KIT_FORMAT) {
      const desc = `${baseDescription} Use Slack mrkdwn (not GitHub markdown). ${this.formatDescription}`.trim();
      return {
        name: FINAL_REPORT_TOOL,
        description: desc,
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['status', 'report_format', 'messages'],
          properties: {
            status: statusProp,
            report_format: { type: 'string', const: SLACK_BLOCK_KIT_FORMAT, description: this.formatDescription },
            messages: {
              type: 'array',
              minItems: 1,
              maxItems: 20,
              items: {
                type: 'object',
                additionalProperties: true,
                required: ['blocks'],
                properties: {
                  blocks: {
                    type: 'array',
                    minItems: 1,
                    maxItems: 50,
                    items: {
                      oneOf: [
                        {
                          type: 'object',
                          additionalProperties: true,
                          required: ['type', 'text'],
                          properties: {
                            type: { const: 'section' },
                            text: {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type', 'text'],
                              properties: {
                                type: { const: 'mrkdwn' },
                                text: { type: 'string', minLength: 1, maxLength: 2900 }
                              }
                            },
                            fields: {
                              type: 'array',
                              maxItems: 10,
                              items: {
                                type: 'object',
                                additionalProperties: true,
                                required: ['type', 'text'],
                                properties: {
                                  type: { const: 'mrkdwn' },
                                  text: { type: 'string', minLength: 1, maxLength: 2000 }
                                }
                              }
                            }
                          }
                        },
                        {
                          type: 'object',
                          additionalProperties: true,
                          required: ['type', 'text'],
                          properties: {
                            type: { const: 'header' },
                            text: {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type', 'text'],
                              properties: {
                                type: { const: 'plain_text' },
                                text: { type: 'string', minLength: 1, maxLength: 150 }
                              }
                            }
                          }
                        },
                        {
                          type: 'object',
                          additionalProperties: true,
                          required: ['type'],
                          properties: { type: { const: 'divider' } }
                        },
                        {
                          type: 'object',
                          additionalProperties: true,
                          required: ['type', 'elements'],
                          properties: {
                            type: { const: 'context' },
                            elements: {
                              type: 'array',
                              minItems: 1,
                              maxItems: 10,
                              items: {
                                type: 'object',
                                additionalProperties: true,
                                required: ['type', 'text'],
                                properties: {
                                  type: { const: 'mrkdwn' },
                                  text: { type: 'string', minLength: 1, maxLength: 2000 }
                                }
                              }
                            }
                          }
                        }
                      ]
                    }
                  }
                }
              }
            },
            metadata: metadataProp,
          },
        },
      };
    }

    const descSuffix = this.formatDescription.length > 0 ? this.formatDescription : this.formatId;
    return {
      name: FINAL_REPORT_TOOL,
      description: `${baseDescription} Final deliverable must be ${descSuffix}.`.trim(),
      inputSchema: {
        type: 'object',
        additionalProperties: false,
        required: ['status', 'report_format', 'report_content'],
        properties: {
          status: statusProp,
          report_format: { type: 'string', const: this.formatId, description: this.formatDescription },
          report_content: { type: 'string', minLength: 1, description: 'Complete final deliverable in the requested format.' },
          metadata: metadataProp,
        },
      },
    };
  }

  async execute(name: string, args: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    if (name === 'agent__progress_report') {
      const progress = typeof (args.progress) === 'string' ? args.progress : '';
      this.opts.updateStatus(progress);
      return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    if (name === 'agent__final_report') {
      const status = (typeof args.status === 'string' ? args.status : 'success') as 'success'|'failure'|'partial';
      const requestedFormat = typeof args.report_format === 'string' ? args.report_format : (typeof args.format === 'string' ? args.format : undefined);
      if (requestedFormat !== undefined && requestedFormat !== this.formatId) {
        this.opts.logError(`agent__final_report: received report_format='${requestedFormat}', expected '${this.formatId}'. Proceeding with expected format.`);
      }
      const content = typeof args.report_content === 'string' ? args.report_content : (typeof args.content === 'string' ? args.content : undefined);
      const contentJson = (args.content_json !== null && typeof args.content_json === 'object' && !Array.isArray(args.content_json)) ? (args.content_json as Record<string, unknown>) : undefined;
      const metadata = (args.metadata !== null && typeof args.metadata === 'object' && !Array.isArray(args.metadata)) ? (args.metadata as Record<string, unknown>) : undefined;
      const rawMessages = Object.prototype.hasOwnProperty.call(args, 'messages') ? args.messages : undefined;

      if (this.formatId === SLACK_BLOCK_KIT_FORMAT) {
        const asString = (value: unknown): string | undefined => {
          if (typeof value !== 'string') return undefined;
          const trimmed = value.trim();
          return trimmed.length > 0 ? trimmed : undefined;
        };
        const safeParseJson = (value: string): unknown => {
          try { return JSON.parse(value) as unknown; } catch { return value; }
        };
        const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
        const originalMessages = (() => {
          if (Array.isArray(rawMessages)) return rawMessages as unknown[];
          if (typeof rawMessages === 'string') {
            const trimmed = rawMessages.trim();
            if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
              const parsed = safeParseJson(trimmed);
              if (Array.isArray(parsed) || isObj(parsed)) return parsed;
            }
            return rawMessages;
          }
          if (rawMessages !== null && typeof rawMessages === 'object') return rawMessages;
          return undefined;
        })();
        const expandMessages = (candidate: unknown): Record<string, unknown>[] => {
          if (candidate === undefined || candidate === null) return [];
          const str = asString(candidate);
          const parsed = str !== undefined ? safeParseJson(str) : candidate;
          if (typeof parsed === 'string') {
            const text = asString(parsed);
            return text !== undefined ? [ { blocks: [ { type: 'section', text: { type: 'mrkdwn', text } } ] } ] : [];
          }
          if (Array.isArray(parsed)) {
            const expanded = parsed.flatMap((entry) => expandMessages(entry));
            if (expanded.length > 0) return expanded;
            const blocks = parsed.flatMap((entry) => {
              const blockCandidate: unknown = typeof entry === 'string' ? safeParseJson(entry) : entry;
              if (Array.isArray(blockCandidate)) return blockCandidate as unknown[];
              return blockCandidate !== undefined ? [blockCandidate] : [];
            });
            return blocks.length > 0 ? [ { blocks } ] : [];
          }
          if (isObj(parsed)) return [parsed];
          return [];
        };
        const normalizedMessages: Record<string, unknown>[] = (() => {
          const fromMessages = expandMessages(originalMessages).slice(0, 20);
          if (fromMessages.length > 0) return fromMessages;
          if (typeof content === 'string' && content.trim().length > 0) {
            return [ { blocks: [ { type: 'section', text: { type: 'mrkdwn', text: content } } ] } ];
          }
          return [];
        })();
        if (normalizedMessages.length === 0) throw new Error('final_report(slack-block-kit) requires `messages` or non-empty `content`.');
        const repairedMessages: Record<string, unknown>[] = (() => {
          const normalizeMrkdwn = (input: string): string => {
            if (input.includes('**')) {
              const pairs = (input.match(/\*\*/g) ?? []).length;
              if (pairs >= 2) return input.replace(/\*\*([^\*]+)\*\*/g, '*$1*');
            }
            return input;
          };
          const clamp = (s: unknown, max: number): string => {
            const raw = typeof s === 'string' ? s : (typeof s === 'number' || typeof s === 'boolean') ? String(s) : '';
            const normalized = normalizeMrkdwn(raw);
            return normalized.length <= max ? normalized : `${normalized.slice(0, Math.max(0, max - 1))}…`;
          };
          const toMrkdwn = (v: unknown, max: number) => ({ type: 'mrkdwn', text: clamp(v, max) });
          const asArr = (v: unknown): unknown[] => {
            if (Array.isArray(v)) return v;
            const str = asString(v);
            if (str !== undefined && (str.startsWith('[') || str.startsWith('{'))) {
              const parsed = safeParseJson(str);
              if (Array.isArray(parsed)) return parsed;
              if (isObj(parsed)) return [parsed];
            }
            return [];
          };
          const coerceBlock = (b: unknown): Record<string, unknown> | undefined => {
            if (typeof b === 'string') {
              const trimmed = asString(b);
              if (trimmed !== undefined) {
                const parsed = safeParseJson(trimmed);
                if (isObj(parsed)) return coerceBlock(parsed);
                if (Array.isArray(parsed)) {
                  return parsed.map(coerceBlock).find((x) => x !== undefined);
                }
                const text = clamp(trimmed, 2900);
                return text.length > 0 ? { type: 'section', text: { type: 'mrkdwn', text } } : undefined;
              }
            }
            if (typeof b !== 'object' || b === null) {
              const text = clamp(b, 2900);
              return text.length > 0 ? { type: 'section', text: { type: 'mrkdwn', text } } : undefined;
            }
            const blk = isObj(b) ? b : {};
            const bb = blk as { type?: unknown; text?: unknown; elements?: unknown; fields?: unknown };
            const typ: string = (typeof bb.type === 'string') ? bb.type : '';
            if (typ === 'divider') return { type: 'divider' };
            if (typ === 'header') {
              const raw = isObj(bb.text) ? (bb.text as { text?: unknown }).text : bb.text;
              return { type: 'header', text: { type: 'plain_text', text: clamp(raw, 150) } };
            }
            if (typ === 'context') {
              const elems = asArr(bb.elements).map((e) => toMrkdwn(isObj(e) ? (e as { text?: unknown }).text : e, 2000)).slice(0, 10);
              if (elems.length === 0) return undefined;
              return { type: 'context', elements: elems };
            }
            const textObj = isObj(bb.text) ? (bb.text as { text?: unknown }) : undefined;
            const rawText = (textObj?.text !== undefined ? textObj.text : bb.text);
            const fields = asArr(bb.fields).map((f) => toMrkdwn(isObj(f) ? (f as { text?: unknown }).text : f, 2000)).slice(0, 10);
            const out: Record<string, unknown> = { type: 'section' };
            const t = clamp(rawText, 2900);
            const mergedFieldText = fields
              .map((f) => (typeof f.text === 'string' ? f.text : ''))
              .filter((s) => s.length > 0)
              .join('\n');
            if (mergedFieldText.length > 0) {
              const combined = [t, mergedFieldText].filter((s) => s.length > 0).join('\n');
              if (combined.length > 0) (out).text = { type: 'mrkdwn', text: clamp(combined, 2900) } as unknown;
            } else if (t.length > 0) {
              (out).text = { type: 'mrkdwn', text: t } as unknown;
            }
            if ((out as { text?: unknown }).text === undefined) return undefined;
            return out;
          };
          const coerceMsg = (m: unknown): Record<string, unknown> | undefined => {
            if (typeof m === 'string') {
              const trimmed = asString(m);
              if (trimmed !== undefined) {
                const parsed = safeParseJson(trimmed);
                if (isObj(parsed) || Array.isArray(parsed)) return coerceMsg(parsed);
                const block = coerceBlock(trimmed);
                return block !== undefined ? { blocks: [block] } : undefined;
              }
            }
            const msg = isObj(m) ? m : {};
            const mm = msg as { blocks?: unknown };
            const blocksSource = mm.blocks !== undefined ? mm.blocks : msg;
            const blocks = asArr(blocksSource)
              .map(coerceBlock)
              .filter((x): x is Record<string, unknown> => x !== undefined)
              .slice(0, 50);
            if (blocks.length === 0) return undefined;
            return { blocks };
          };
          return normalizedMessages
            .map(coerceMsg)
            .filter((x): x is Record<string, unknown> => x !== undefined)
            .slice(0, 20);
        })();
        const slackSchema: Record<string, unknown> = {
          type: 'array', minItems: 1, maxItems: 20,
          items: {
            type: 'object', additionalProperties: true, required: ['blocks'],
            properties: {
              blocks: {
                type: 'array', minItems: 1, maxItems: 50,
                items: {
                  oneOf: [
                    { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'section' }, text: { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'mrkdwn' }, text: { type: 'string', maxLength: 2900 } } }, fields: { type: 'array', maxItems: 10, items: { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'mrkdwn' }, text: { type: 'string', maxLength: 2000 } } } } } },
                    { type: 'object', additionalProperties: true, required: ['type','fields'], properties: { type: { const: 'section' }, fields: { type: 'array', maxItems: 10, items: { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'mrkdwn' }, text: { type: 'string', maxLength: 2000 } } } } } },
                    { type: 'object', additionalProperties: true, required: ['type'], properties: { type: { const: 'divider' } } },
                    { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'header' }, text: { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'plain_text' }, text: { type: 'string', maxLength: 150 } } } } },
                    { type: 'object', additionalProperties: true, required: ['type','elements'], properties: { type: { const: 'context' }, elements: { type: 'array', minItems: 1, maxItems: 10, items: { type: 'object', additionalProperties: true, required: ['type','text'], properties: { type: { const: 'mrkdwn' }, text: { type: 'string', maxLength: 2000 } } } } } }
                  ]
                }
              }
            }
          }
        };
        try {
          const validate = this.ajv.compile(slackSchema);
          let ok = validate(repairedMessages);
          let finalMsgs: Record<string, unknown>[] = repairedMessages;
          if (!ok) { ok = validate(normalizedMessages); finalMsgs = normalizedMessages; }
          if (!ok) {
            const errsArr = validate.errors ?? [];
            const errs = errsArr.map((e) => `${e.instancePath} ${e.message ?? ''}`.trim()).join('; ');
            const samples = errsArr
              .map((e) => {
                const path = typeof e.instancePath === 'string' ? e.instancePath : '';
                const match = /^\/(\d+)\/blocks\/(\d+)/.exec(path);
                if (match === null) return undefined;
                const msgIdx = Number.parseInt(match[1], 10);
                const blockIdx = Number.parseInt(match[2], 10);
                if (!Number.isFinite(msgIdx) || !Number.isFinite(blockIdx)) return undefined;
                const msg = Array.isArray(finalMsgs) && msgIdx >= 0 && msgIdx < finalMsgs.length ? finalMsgs[msgIdx] : undefined;
                if (msg === undefined) return undefined;
                const msgRecord = msg;
                const blocksValueUnknown = Object.prototype.hasOwnProperty.call(msgRecord, 'blocks') ? msgRecord.blocks : undefined;
                if (!Array.isArray(blocksValueUnknown) || blockIdx < 0 || blockIdx >= blocksValueUnknown.length) return undefined;
                const blocksValue = blocksValueUnknown as unknown[];
                const block = blocksValue[blockIdx];
                try {
                  const json = JSON.stringify(block);
                  return `${path}=${json.slice(0, 300)}`;
                } catch {
                  return `${path}=[unserializable block]`;
                }
              })
              .filter((v): v is string => typeof v === 'string' && v.length > 0);
            const sampleStr = samples.length > 0 ? ` samples: ${samples.join(' | ')}` : '';
            throw new Error(`slack_invalid_blocks: ${errs}${sampleStr}`);
          }
          normalizedMessages.splice(0, normalizedMessages.length, ...finalMsgs);
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          throw new Error(msg);
        }
        const metaBase: Record<string, unknown> = (metadata ?? {});
        const slackVal = (metaBase as Record<string, unknown> & { slack?: unknown }).slack;
        const slackExisting: Record<string, unknown> = (slackVal !== undefined && slackVal !== null && typeof slackVal === 'object' && !Array.isArray(slackVal)) ? (slackVal as Record<string, unknown>) : {};
        const metaSlack: Record<string, unknown> = { ...slackExisting, messages: normalizedMessages };
        const mergedMeta: Record<string, unknown> = { ...metaBase, slack: metaSlack };
        this.opts.setFinalReport({ status, format: this.formatId, content, content_json: contentJson, metadata: mergedMeta });
        return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
      }

      if (this.formatId === 'json') {
        if (contentJson === undefined) throw new Error('final_report(json) requires `content_json` (object).');
        if (this.opts.expectedJsonSchema !== undefined) {
          const validate = this.ajv.compile(this.opts.expectedJsonSchema);
          const ok = validate(contentJson);
          if (!ok) {
            const errs = (validate.errors ?? []).map((e) => {
              const inst = typeof e.instancePath === 'string' ? e.instancePath : '';
              const msg = typeof e.message === 'string' ? e.message : '';
              return `${inst} ${msg}`.trim();
            }).join('; ');
            throw new Error(`final_report(json) schema validation failed: ${errs}`);
          }
        }
        this.opts.setFinalReport({ status, format: this.formatId, content, content_json: contentJson, metadata });
        return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
      }

      if (typeof content !== 'string' || content.trim().length === 0) throw new Error('agent__final_report requires non-empty report_content field.');
      this.opts.setFinalReport({ status, format: this.formatId, content, content_json: contentJson, metadata });
      return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    if (this.opts.enableBatch && name === 'agent__batch') {
      // Minimal batch: always use orchestrator for inner calls
      const rawCalls = (args as { calls?: unknown }).calls;
      let calls: unknown[] = [];
      const batchTurn = (() => {
        try {
          const t = this.opts.getCurrentTurn();
          return Number.isFinite(t) && t > 0 ? Math.trunc(t) : 1;
        } catch {
          return 1;
        }
      })();

      // Handle both array and JSON string formats
      if (Array.isArray(rawCalls)) {
        calls = rawCalls;
      } else if (typeof rawCalls === 'string') {
        // Try to extract and parse JSON array from the string
        // Look for array boundaries and try to parse
        const trimmed = rawCalls.trim();

        // Find the last valid closing bracket for the JSON array
        let lastValidIndex = -1;
        let bracketCount = 0;
        let inString = false;
        let escapeNext = false;

        // eslint-disable-next-line functional/no-loop-statements
        for (let i = 0; i < trimmed.length; i++) {
          const char = trimmed[i];

          if (!inString) {
            if (char === '[') bracketCount++;
            else if (char === ']') {
              bracketCount--;
              if (bracketCount === 0) {
                lastValidIndex = i;
              }
            }
            else if (char === '"') inString = true;
          } else {
            if (escapeNext) {
              escapeNext = false;
            } else if (char === '\\') {
              escapeNext = true;
            } else if (char === '"') {
              inString = false;
            }
          }
        }

        if (lastValidIndex > 0) {
          try {
            const jsonStr = trimmed.substring(0, lastValidIndex + 1);
            const parsed = JSON.parse(jsonStr) as unknown;
            if (Array.isArray(parsed)) {
              calls = parsed;
            }
          } catch {
            // Silently ignore parse errors
          }
        }
      }

      // Log error if batch is empty
      if (calls.length === 0) {
        const errorMsg = typeof rawCalls === 'string'
          ? `agent__batch received empty or unparseable calls array (raw string length: ${String(rawCalls.length)})`
          : `agent__batch received empty calls array (type: ${typeof rawCalls})`;

        // Log the error
        this.opts.logError(errorMsg);

        throw new Error(`empty_batch: ${errorMsg}`);
      }

      interface NormalizedCall { id: string; tool: string; args: Record<string, unknown> }
      const normalizedCalls: NormalizedCall[] = calls.map((cUnknown) => {
        const c = (cUnknown !== null && typeof cUnknown === 'object') ? (cUnknown as Record<string, unknown>) : {};
        const id = typeof c.id === 'string' ? c.id : (typeof c.id === 'number' ? String(c.id) : '');
        const tool = typeof c.tool === 'string' ? c.tool : '';
        const a = (c.args !== null && typeof c.args === 'object') ? (c.args as Record<string, unknown>) : {};
        return { id, tool, args: a };
      });

      const invalidEntry = normalizedCalls.find((entry) => entry.id.trim().length === 0 || entry.tool.trim().length === 0);
      if (invalidEntry !== undefined) {
        const errorMsg = 'invalid_batch_input: each call requires non-empty id and tool';
        this.opts.logError(errorMsg);
        throw new Error(errorMsg);
      }

      interface R { id: string; tool: string; ok: boolean; elapsedMs: number; output?: string; error?: { code: string; message: string } }
      const results: R[] = await Promise.all(normalizedCalls.map(async ({ id, tool, args: a }) => {
        const t0 = Date.now();
        // Allow progress_report in batch, but not final_report or nested batch
        if (tool === 'agent__final_report' || tool === 'agent__batch') return { id, tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } };
        // Handle progress_report directly in batch
        if (tool === 'agent__progress_report') {
          const progress = typeof (a.progress) === 'string' ? a.progress : '';
          this.opts.updateStatus(progress);
          return { id, tool, ok: true, elapsedMs: Date.now() - t0, output: JSON.stringify({ ok: true }) };
        }
        try {
          if (!(this.opts.orchestrator.hasTool(tool))) return { id, tool, ok: false, elapsedMs: 0, error: { code: 'UNKNOWN_TOOL', message: `Unknown tool: ${tool}` } };
          const managed = await this.opts.orchestrator.executeWithManagement(tool, a, { turn: batchTurn, subturn: 0 }, { timeoutMs: this.opts.toolTimeoutMs });
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
