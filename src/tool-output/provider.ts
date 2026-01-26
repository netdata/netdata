import type { ToolExecuteOptions, ToolExecuteResult } from '../tools/types.js';
import type { MCPTool } from '../types.js';
import type { ToolOutputExtractor } from './extractor.js';
import type { ToolOutputStore } from './store.js';
import type { ToolOutputConfig, ToolOutputExtractionResult, ToolOutputMode, ToolOutputTarget } from './types.js';

import { renderPromptTemplate } from '../prompts/templates.js';
import { ToolProvider } from '../tools/types.js';
import { truncateToBytesWithInfo } from '../truncation.js';
import { parseJsonRecord } from '../utils.js';

import { resolveToolOutputTargets } from './config.js';
import { formatToolOutputFailure, formatToolOutputSuccess } from './formatter.js';

const TOOL_OUTPUT_NAME = 'tool_output';

export class ToolOutputProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  readonly namespace = 'tool-output' as const;
  private readonly toolSchema: MCPTool;

  constructor(
    private readonly store: ToolOutputStore,
    private readonly config: ToolOutputConfig,
    private readonly extractor: ToolOutputExtractor,
    private readonly sessionTargets: ToolOutputTarget[],
    private readonly toolResponseMaxBytes: number | undefined,
  ) {
    super();
    this.toolSchema = this.buildToolSchema();
  }

  listTools(): MCPTool[] {
    if (!this.config.enabled) return [];
    if (!this.store.hasEntries()) return [];
    return [this.toolSchema];
  }

  hasTool(name: string): boolean {
    return name === TOOL_OUTPUT_NAME;
  }

  override getInstructions(): string {
    if (!this.config.enabled) return '';
    if (!this.store.hasEntries()) return '';
    return renderPromptTemplate('toolOutputInstructions', {});
  }

  async execute(name: string, parameters: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    if (name !== TOOL_OUTPUT_NAME) {
      throw new Error(`Unknown tool: ${name}`);
    }
    const handle = typeof parameters.handle === 'string' ? parameters.handle.trim() : '';
    const extract = typeof parameters.extract === 'string' ? parameters.extract.trim() : '';
    const modeRaw = typeof parameters.mode === 'string' ? parameters.mode.trim() : undefined;
    const mode = (() => {
      if (modeRaw === undefined) return undefined;
      const allowed = new Set<ToolOutputMode>(['auto', 'full-chunked', 'read-grep', 'truncate']);
      return allowed.has(modeRaw as ToolOutputMode) ? (modeRaw as ToolOutputMode) : undefined;
    })();
    if (handle.length === 0 || extract.length === 0) {
      const failure = formatToolOutputFailure({
        toolName: 'unknown',
        handle: handle.length > 0 ? handle : '<missing>',
        mode: mode ?? 'auto',
        error: renderPromptTemplate('toolOutputErrorInvalidParams', {}),
      });
      return { ok: false, result: failure, latencyMs: Date.now() - start, kind: this.kind, namespace: this.namespace };
    }

    const read = await this.store.read(handle);
    if (read === undefined) {
      const body = renderPromptTemplate('toolOutputErrorHandleMissing', { handle });
      const message = formatToolOutputSuccess({ toolName: 'unknown', handle, mode: 'auto', body });
      return { ok: true, result: message, latencyMs: Date.now() - start, kind: this.kind, namespace: this.namespace };
    }

    const entry = read.entry;
    const targets = resolveToolOutputTargets(this.config, entry.sourceTarget, this.sessionTargets);
    let extraction: ToolOutputExtractionResult;
    try {
      extraction = await this.extractor.extract(
        {
          handle: entry.handle,
          toolName: entry.toolName,
          toolArgsJson: entry.toolArgsJson,
          content: read.content,
          stats: {
            bytes: entry.bytes,
            lines: entry.lines,
            tokens: entry.tokens,
            avgLineBytes: entry.lines > 0 ? Math.ceil(entry.bytes / entry.lines) : entry.bytes,
          },
          sourceTarget: entry.sourceTarget,
        },
        extract,
        mode,
        targets,
        {
          onChildOpTree: opts?.onChildOpTree,
          parentOpPath: opts?.parentOpPath,
          parentContext: opts?.parentContext,
        },
      );
    } catch (error) {
      const errText = error instanceof Error ? error.message : String(error);
      const failure = formatToolOutputFailure({
        toolName: entry.toolName,
        handle,
        mode: mode ?? 'auto',
        error: errText,
      });
      return { ok: false, result: failure, latencyMs: Date.now() - start, kind: this.kind, namespace: this.namespace };
    }

    const body = this.applyBodyCap(this.applyWarning(extraction), entry.toolName, handle, extraction.mode);
    const message = formatToolOutputSuccess({
      toolName: entry.toolName,
      handle,
      mode: extraction.mode,
      body,
    });

    return {
      ok: true,
      result: message,
      latencyMs: Date.now() - start,
      kind: this.kind,
      namespace: this.namespace,
      extras: extraction.childOpTree !== undefined ? { childOpTree: extraction.childOpTree } : undefined,
    };
  }

  async cleanup(): Promise<void> {
    await this.store.cleanup();
  }

  private buildToolSchema(): MCPTool {
    const raw = renderPromptTemplate('toolSchemaToolOutput', {});
    const parsed = parseJsonRecord(raw);
    if (parsed === undefined) {
      throw new Error('tool_output schema template did not produce a JSON object');
    }
    const description = typeof parsed.description === 'string'
      ? parsed.description
      : 'Extract information from a stored oversized tool output by handle.';
    return {
      name: TOOL_OUTPUT_NAME,
      description,
      inputSchema: parsed,
    };
  }

  private applyWarning(extraction: ToolOutputExtractionResult): string {
    if (extraction.warning === undefined) return extraction.text;
    return `${extraction.warning}\n${extraction.text}`;
  }

  private applyBodyCap(body: string, toolName: string, handle: string, mode: string): string {
    const limit = this.toolResponseMaxBytes;
    if (typeof limit !== 'number' || limit <= 0) return body;
    const header = formatToolOutputSuccess({ toolName, handle, mode, body: '' });
    const headerBytes = Buffer.byteLength(header, 'utf8');
    const budget = limit - headerBytes;
    if (budget <= 0) {
      return renderPromptTemplate('toolOutputTruncated', {});
    }
    const truncated = truncateToBytesWithInfo(body, budget);
    return truncated?.value ?? body;
  }
}
