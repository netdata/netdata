import Ajv from 'ajv';

import type { OutputFormatId } from '../formats.js';
import type { MCPTool } from '../types.js';
import type { ToolsOrchestrator } from './tools.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

type AjvInstance = AjvClass;
type AjvErrorObject = ErrorObject<string, Record<string, unknown>>;
type AjvConstructor = new (options?: AjvOptions) => AjvInstance;
const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;

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
  disableProgressTool?: boolean;
}

const PROGRESS_TOOL = 'agent__progress_report';
const FINAL_REPORT_TOOL = 'agent__final_report';
const BATCH_TOOL = 'agent__batch';
const SLACK_BLOCK_KIT_FORMAT: OutputFormatId = 'slack-block-kit';
const REPORT_FORMAT_LABEL = '`report_format`';
const DEFAULT_PARAMETERS_DESCRIPTION = 'Parameters for selected tool';

export class InternalToolProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  private readonly ajv: AjvInstance = new AjvCtor({ allErrors: true, strict: false });
  private readonly formatId: OutputFormatId;
  private readonly formatDescription: string;
  private instructions: string;
  private cachedBatchSchemas?: { schemas: Record<string, unknown>[]; summaries: { name: string; required: string[] }[] };
  private readonly disableProgressTool: boolean;

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
    this.disableProgressTool = opts.disableProgressTool === true;
    this.instructions = this.buildInstructions();
  }

  listTools(): MCPTool[] {
    const tools: MCPTool[] = [];
    if (!this.disableProgressTool) {
      tools.push({
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
      });
    }
    tools.push(this.buildFinalReportTool());
    if (this.opts.enableBatch) {
      const { schemas } = this.ensureBatchSchemas();
      const fallbackItemSchema: Record<string, unknown> = {
        type: 'object',
        additionalProperties: true,
        required: ['id', 'tool', 'parameters'],
        properties: {
          id: { oneOf: [ { type: 'string', minLength: 1 }, { type: 'number' } ] },
          tool: { type: 'string', minLength: 1 },
          parameters: {
            type: 'object',
            additionalProperties: true,
            description: DEFAULT_PARAMETERS_DESCRIPTION
          }
        }
      };
      const itemsSchema = schemas.length > 0 ? { anyOf: schemas } : fallbackItemSchema;
      tools.push({
        name: BATCH_TOOL,
        description: 'Execute multiple tools in one call, reusing the schemas of all available tools.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['calls'],
          properties: {
            calls: {
              type: 'array',
              minItems: 1,
              items: itemsSchema
            }
          }
        }
      });
    }
    return tools;
  }

  hasTool(name: string): boolean {
    if (name === FINAL_REPORT_TOOL) return true;
    if (this.opts.enableBatch && name === BATCH_TOOL) return true;
    if (name === PROGRESS_TOOL) return !this.disableProgressTool;
    return false;
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
    const lines: string[] = ['### Internal Tools', ''];

    if (!this.disableProgressTool) {
      lines.push(`#### ${PROGRESS_TOOL} — Progress Updates for the user`);
      lines.push('- Use this tool to update the user on your progress.');
      lines.push('- Keep the `progress` message concise (≤20 words).');
      lines.push('- Always pair this tool with other tools you are invoking (except `agent__final_report`).');
      lines.push('- Do not call this tool by itself.');
      lines.push('- Do not pair this tool with `agent__final_report`');
    }

    if (this.opts.enableBatch) {
      lines.push('');
      lines.push(`#### ${BATCH_TOOL} — Parallel Tooling`);
      lines.push('- Use this helper to execute multiple tools in one request.');
      lines.push("- Each `calls[]` entry needs an `id`, the real tool name, and a `parameters` object that matches that tool's schema.");
      lines.push('- Example:');
      lines.push('  {');
      lines.push('    "calls": [');
      if (!this.disableProgressTool) {
        lines.push(`      { "id": 1, "tool": "${PROGRESS_TOOL}", "parameters": { "progress": "Researching Netdata and eBPF" } },`);
        lines.push('      { "id": 2, "tool": "tool1", "parameters": { "param1": "value1", "param2": "value2" } },');
        lines.push('      { "id": 3, "tool": "tool2", "parameters": { "param1": "value1" } }');
      } else {
        lines.push('      { "id": 1, "tool": "tool1", "parameters": { "param1": "value1", "param2": "value2" } },');
        lines.push('      { "id": 2, "tool": "tool2", "parameters": { "param1": "value1" } }');
      }
      lines.push('      (Tool names must match exactly and every required parameter must be present.)');
      lines.push('    ]');
      lines.push('  }');
      if (!this.disableProgressTool) {
        lines.push('- Do not combine `agent__progress_report` with `agent__final_report` in the same request; send the final report on its own.');
      }
    }

    lines.push('');
    lines.push(`#### ${FINAL_REPORT_TOOL} - Deliver Your Final Answer`);
    lines.push(`- You MUST call '${FINAL_REPORT_TOOL}' to provide your final answer to the user.`);
    lines.push('- Required fields:');
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
    lines.push('- Include optional `metadata` only when explicitly relevant.');
    lines.push('- After this tool succeeds, do not send additional assistant messages.');

    lines.push('');
    lines.push('*You run in agentic mode, interfacing with software tools.*');
    lines.push('');
    lines.push('**YOU MUST:**');
    lines.push('- **ALWAYS** respond with valid tool calls, even for your final answer.');
    lines.push(`- **ALWAYS** provide your final report to the user using the ${FINAL_REPORT_TOOL} tool, with the correct format.`);

    lines.push('**YOU MUST NOT:**');
    lines.push('- Do not provide any other output except calling your tools. Any other output you provide is ignored.');

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

  async execute(name: string, parameters: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    if (name === PROGRESS_TOOL && this.disableProgressTool) {
      throw new Error('agent__progress_report is disabled for this session');
    }
    if (name === 'agent__progress_report') {
      const progress = typeof (parameters.progress) === 'string' ? parameters.progress : '';
      this.opts.updateStatus(progress);
      return { ok: true, result: JSON.stringify({ ok: true }), latencyMs: Date.now() - start, kind: this.kind, providerId: this.id };
    }
    if (name === 'agent__final_report') {
      const status = (typeof parameters.status === 'string' ? parameters.status : 'success') as 'success'|'failure'|'partial';
      const requestedFormat = typeof parameters.report_format === 'string' ? parameters.report_format : (typeof parameters.format === 'string' ? parameters.format : undefined);
      if (requestedFormat !== undefined && requestedFormat !== this.formatId) {
        this.opts.logError(`agent__final_report: received report_format='${requestedFormat}', expected '${this.formatId}'. Proceeding with expected format.`);
      }
      const content = typeof parameters.report_content === 'string' ? parameters.report_content : (typeof parameters.content === 'string' ? parameters.content : undefined);
      const contentJson = (parameters.content_json !== null && typeof parameters.content_json === 'object' && !Array.isArray(parameters.content_json)) ? (parameters.content_json as Record<string, unknown>) : undefined;
      const metadata = (parameters.metadata !== null && typeof parameters.metadata === 'object' && !Array.isArray(parameters.metadata)) ? (parameters.metadata as Record<string, unknown>) : undefined;
      const rawMessages = Object.prototype.hasOwnProperty.call(parameters, 'messages') ? parameters.messages : undefined;

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
            const errsArr = Array.isArray(validate.errors) ? (validate.errors as AjvErrorObject[]) : [];
            const errs = errsArr.map((error) => `${error.instancePath} ${error.message ?? ''}`.trim()).join('; ');
            const samples = errsArr
              .map((error) => {
                const path = typeof error.instancePath === 'string' ? error.instancePath : '';
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
        } catch (error) {
          const msg = error instanceof Error ? error.message : String(error);
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
            const errs = Array.isArray(validate.errors)
              ? (validate.errors as AjvErrorObject[]).map((error) => {
                  const inst = typeof error.instancePath === 'string' ? error.instancePath : '';
                  const msg = typeof error.message === 'string' ? error.message : '';
                  return `${inst} ${msg}`.trim();
                }).join('; ')
              : '';
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
      const rawCalls = (parameters as { calls?: unknown }).calls;
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

      interface NormalizedCall { id: string; tool: string; parameters: Record<string, unknown> }
      const normalizedCalls: NormalizedCall[] = calls.map((cUnknown) => {
        const c = (cUnknown !== null && typeof cUnknown === 'object') ? (cUnknown as Record<string, unknown>) : {};
        const id = typeof c.id === 'string' ? c.id : (typeof c.id === 'number' ? String(c.id) : '');
        const tool = typeof c.tool === 'string' ? c.tool : '';
        const provided = (c.parameters !== null && typeof c.parameters === 'object') ? (c.parameters as Record<string, unknown>) : {};
        return { id, tool, parameters: provided };
      });

      const invalidEntry = normalizedCalls.find((entry) => entry.id.trim().length === 0 || entry.tool.trim().length === 0);
      if (invalidEntry !== undefined) {
        const errorMsg = 'invalid_batch_input: each call requires non-empty id and tool';
        this.opts.logError(errorMsg);
        throw new Error(errorMsg);
      }

      interface R { id: string; tool: string; ok: boolean; elapsedMs: number; output?: string; error?: { code: string; message: string } }
      const results: R[] = await Promise.all(normalizedCalls.map(async ({ id, tool, parameters: callParameters }) => {
        const t0 = Date.now();
        // Allow progress_report in batch, but not final_report or nested batch
        if (tool === 'agent__final_report' || tool === 'agent__batch') return { id, tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } };
        // Handle progress_report directly in batch
        if (tool === 'agent__progress_report') {
          const progress = typeof (callParameters.progress) === 'string' ? callParameters.progress : '';
          this.opts.updateStatus(progress);
          return { id, tool, ok: true, elapsedMs: Date.now() - t0, output: JSON.stringify({ ok: true }) };
        }
        try {
          if (!(this.opts.orchestrator.hasTool(tool))) return { id, tool, ok: false, elapsedMs: 0, error: { code: 'UNKNOWN_TOOL', message: `Unknown tool: ${tool}` } };
          const managed = await this.opts.orchestrator.executeWithManagement(tool, callParameters, { turn: batchTurn, subturn: 0 }, { timeoutMs: this.opts.toolTimeoutMs });
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

  private ensureBatchSchemas(): { schemas: Record<string, unknown>[]; summaries: { name: string; required: string[] }[] } {
    if (this.cachedBatchSchemas !== undefined) return this.cachedBatchSchemas;
    const schemas: Record<string, unknown>[] = [];
    const summaries: { name: string; required: string[] }[] = [];
    const idSchema: Record<string, unknown> = { oneOf: [ { type: 'string', minLength: 1 }, { type: 'number' } ] };

    const pushSchema = (toolName: string, rawSchema: unknown) => {
      const parametersSchema = this.cloneJsonSchema(rawSchema);
      const required = (() => {
        const rawRequired = Object.prototype.hasOwnProperty.call(parametersSchema, 'required')
          ? parametersSchema.required
          : undefined;
        if (!Array.isArray(rawRequired)) return [];
        return rawRequired.filter((item): item is string => typeof item === 'string');
      })();
      summaries.push({ name: toolName, required });
      schemas.push({
        type: 'object',
        additionalProperties: false,
        required: ['id', 'tool', 'parameters'],
        properties: {
          id: idSchema,
          tool: { type: 'string', const: toolName },
          parameters: parametersSchema
        }
      });
    };

    try {
      const available = this.opts.orchestrator.listTools();
      available.forEach((tool) => {
        const name = tool.name;
        if (!name || name === FINAL_REPORT_TOOL || name === BATCH_TOOL) return;
        pushSchema(name, tool.inputSchema);
      });
    } catch {
      // Ignore orchestrator failures; callers will use fallback schema
    }

    if (!this.disableProgressTool && !summaries.some((summary) => summary.name === PROGRESS_TOOL)) {
      pushSchema(PROGRESS_TOOL, {
        type: 'object',
        additionalProperties: false,
        required: ['progress'],
        properties: { progress: { type: 'string' } }
      });
    }

    this.cachedBatchSchemas = { schemas, summaries };
    return this.cachedBatchSchemas;
  }

  private cloneJsonSchema(schema: unknown): Record<string, unknown> {
    if (schema === null || typeof schema !== 'object') {
      return {
        type: 'object',
        additionalProperties: true,
        description: DEFAULT_PARAMETERS_DESCRIPTION
      };
    }
    try {
      const cloned = JSON.parse(JSON.stringify(schema)) as Record<string, unknown>;
      if (typeof cloned.type !== 'string') cloned.type = 'object';
      if (cloned.additionalProperties === undefined) cloned.additionalProperties = true;
      if (cloned.description === undefined) cloned.description = DEFAULT_PARAMETERS_DESCRIPTION;
      return cloned;
    } catch {
      return {
        type: 'object',
        additionalProperties: true,
        description: DEFAULT_PARAMETERS_DESCRIPTION
      };
    }
  }

  warmupWithOrchestrator(): void {
    if (!this.opts.enableBatch) return;
    this.cachedBatchSchemas = undefined;
    this.ensureBatchSchemas();
    this.instructions = this.buildInstructions();
  }
}
