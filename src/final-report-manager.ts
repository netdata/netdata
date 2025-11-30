/**
 * FinalReportManager - Manages final report state and validation for AI agent sessions.
 *
 * Responsibilities:
 * - Store and manage final report state (committed, pending, source)
 * - Validate final reports against expected schema (AJV)
 * - Extract final reports from text fallback
 * - Build reminder messages for the LLM
 * - Track final report attempts
 */

import Ajv from 'ajv';

import type { OutputFormatId } from './formats.js';
import type { AIAgentResult } from './types.js';
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

import { parseJsonRecord } from './utils.js';

// Reusable type definitions
export type FinalReportPayload = NonNullable<AIAgentResult['finalReport']>;
export type PendingFinalReportPayload = Omit<FinalReportPayload, 'ts'>;
export type FinalReportSource = 'tool-call' | 'text-fallback' | 'tool-message' | 'synthetic';

export const FINAL_REPORT_FORMAT_VALUES = ['json', 'markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'sub-agent', 'text'] as const satisfies readonly FinalReportPayload['format'][];

export const FINAL_REPORT_SOURCE_TEXT_FALLBACK = 'text-fallback' as const;
export const FINAL_REPORT_SOURCE_TOOL_MESSAGE = 'tool-message' as const;

// Internal pending report wrapper
interface PendingReport {
  source: typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE;
  payload: PendingFinalReportPayload;
}

// Configuration for FinalReportManager
export interface FinalReportManagerConfig {
  resolvedFormat?: OutputFormatId;
  resolvedFormatPromptValue?: string;
  resolvedFormatParameterDescription?: string;
  finalReportToolName: string;
}

// Validation result from AJV
export interface ValidationResult {
  valid: boolean;
  errors?: string;
  payloadPreview?: string;
}

// AJV types
type AjvInstance = AjvClass;
type AjvErrorObject = ErrorObject<string, Record<string, unknown>>;
type AjvConstructor = new (options?: AjvOptions) => AjvInstance;
const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;

export class FinalReportManager {
  // Committed final report
  private finalReport?: FinalReportPayload;
  private finalReportSource?: FinalReportSource;

  // Pending report (before acceptance)
  private pendingFinalReport?: PendingReport;

  // Attempt tracking
  private _finalReportAttempts = 0;

  // Configuration (mutable for format fields set after construction)
  private config: FinalReportManagerConfig;

  // AJV instance for schema validation (lazy initialized)
  private ajv?: AjvInstance;

  constructor(config: FinalReportManagerConfig) {
    this.config = config;
  }

  /**
   * Update format-related configuration (called after format resolution)
   */
  setResolvedFormat(
    format: OutputFormatId | undefined,
    promptValue: string | undefined,
    parameterDescription: string | undefined
  ): void {
    this.config = {
      ...this.config,
      resolvedFormat: format,
      resolvedFormatPromptValue: promptValue,
      resolvedFormatParameterDescription: parameterDescription,
    };
  }

  // Accessors for state
  get finalReportAttempts(): number {
    return this._finalReportAttempts;
  }

  incrementAttempts(): void {
    this._finalReportAttempts += 1;
  }

  /**
   * Check if a final report has been committed
   */
  hasReport(): boolean {
    return this.finalReport !== undefined;
  }

  /**
   * Check if there's a pending report waiting for acceptance
   */
  hasPending(): boolean {
    return this.pendingFinalReport !== undefined;
  }

  /**
   * Get the pending report (for compatibility with legacy code)
   */
  getPending(): { source: typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE; payload: PendingFinalReportPayload } | undefined {
    return this.pendingFinalReport;
  }

  /**
   * Get the committed final report
   */
  getReport(): FinalReportPayload | undefined {
    return this.finalReport;
  }

  /**
   * Get the source of the committed final report
   */
  getSource(): FinalReportSource | undefined {
    return this.finalReportSource;
  }

  /**
   * Get the status of the committed final report
   */
  getStatus(): 'success' | 'failure' | undefined {
    const status = this.finalReport?.status;
    if (status === 'success' || status === 'failure') {
      return status;
    }
    return undefined;
  }

  /**
   * Get the resolved format for this session
   */
  getResolvedFormat(): OutputFormatId | undefined {
    return this.config.resolvedFormat;
  }

  /**
   * Commit a final report to the manager
   */
  commit(payload: PendingFinalReportPayload, source: FinalReportSource): void {
    const ensureMissingPhrase = (content: string | undefined): string | undefined => {
      if (payload.status !== 'failure') return content;
      if (typeof content !== 'string') return content;
      const phrase = 'Session completed without a final report';
      return content.includes(phrase) ? content : `${phrase}. ${content}`.trim();
    };

    this.finalReport = {
      status: payload.status,
      format: payload.format,
      content: ensureMissingPhrase(payload.content),
      content_json: payload.content_json,
      metadata: payload.metadata,
      ts: Date.now()
    };
    this.finalReportSource = source;
    this.pendingFinalReport = undefined;
  }

  /**
   * Set a pending report (extracted from text or tool message)
   */
  setPending(payload: PendingFinalReportPayload, source: typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE): void {
    this.pendingFinalReport = { source, payload };
  }

  /**
   * Accept the pending report as the final report
   * Returns the source if accepted, undefined if no pending report
   */
  acceptPending(): (typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE) | undefined {
    if (this.pendingFinalReport === undefined) return undefined;
    const pending = this.pendingFinalReport;
    this.commit(pending.payload, pending.source);
    return pending.source;
  }

  /**
   * Clear any committed or pending final report
   */
  clear(): void {
    this.finalReport = undefined;
    this.finalReportSource = undefined;
    this.pendingFinalReport = undefined;
  }

  /**
   * Validate the committed final report against the expected JSON schema
   * Returns validation result with errors if invalid
   */
  validateSchema(schema: Record<string, unknown>): ValidationResult {
    const fr = this.finalReport;
    if (fr?.format !== 'json' || fr.content_json === undefined) {
      return { valid: true };
    }

    try {
      this.ajv = this.ajv ?? new AjvCtor({ allErrors: true, strict: false });
      const validate = this.ajv.compile(schema);
      const valid = validate(fr.content_json);

      if (valid) {
        return { valid: true };
      }

      const ajvErrors = Array.isArray(validate.errors) ? (validate.errors as AjvErrorObject[]) : [];
      const errors = ajvErrors.map((error) => {
        const pathValue = typeof error.instancePath === 'string' && error.instancePath.length > 0
          ? error.instancePath
          : (typeof error.schemaPath === 'string' ? error.schemaPath : '');
        const msg = typeof error.message === 'string' ? error.message : '';
        return `${pathValue} ${msg}`.trim();
      }).join('; ');

      const payloadPreview = (() => {
        try {
          const raw = JSON.stringify(fr.content_json);
          return typeof raw === 'string'
            ? (raw.length > 200 ? `${raw.slice(0, 200)}…` : raw)
            : undefined;
        } catch {
          return undefined;
        }
      })();

      return { valid: false, errors, payloadPreview };
    } catch (e) {
      return {
        valid: false,
        errors: `AJV validation failed: ${e instanceof Error ? e.message : String(e)}`
      };
    }
  }

  /**
   * Try to extract a final report from assistant text content
   * Returns the extracted payload if successful, undefined otherwise
   */
  tryExtractFromText(text: string): PendingFinalReportPayload | undefined {
    if (typeof text !== 'string') return undefined;
    const trimmedText = text.trim();
    if (trimmedText.length === 0) return undefined;

    const json = this.extractJsonRecordFromText(trimmedText);
    if (json === undefined) return undefined;

    const isRecord = (value: unknown): value is Record<string, unknown> =>
      value !== null && typeof value === 'object' && !Array.isArray(value);
    const pickString = (value: unknown): string | undefined =>
      (typeof value === 'string' ? value : undefined);

    const formatRaw = pickString(json.report_format) ?? pickString(json.format);
    const contentJsonRaw = json.content_json;
    const contentJson = isRecord(contentJsonRaw) ? contentJsonRaw : undefined;
    const contentCandidate = pickString(json.report_content) ?? pickString(json.content);

    const formatCandidate = (formatRaw ?? '').trim();
    const formatMatch = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate);
    if (formatMatch === undefined) return undefined;

    // Model-provided final reports always have status 'success'
    const normalizedStatus: 'success' | 'failure' = 'success';

    let finalContent: string | undefined = contentCandidate;
    if ((finalContent === undefined || finalContent.trim().length === 0) && contentJson !== undefined) {
      try {
        finalContent = JSON.stringify(contentJson);
      } catch {
        finalContent = undefined;
      }
    }
    if (finalContent === undefined || finalContent.trim().length === 0) return undefined;

    const metadata = isRecord(json.metadata) ? json.metadata : undefined;

    const payload: PendingFinalReportPayload = {
      status: normalizedStatus,
      format: formatMatch,
      content: finalContent,
    };
    if (contentJson !== undefined) {
      payload.content_json = contentJson;
    }
    if (metadata !== undefined) {
      payload.metadata = metadata;
    }
    return payload;
  }

  /**
   * Get the final output string for rendering
   */
  getFinalOutput(): string | undefined {
    const fr = this.finalReport;
    if (fr === undefined) return undefined;

    if (fr.format === 'json' && fr.content_json !== undefined) {
      try {
        return JSON.stringify(fr.content_json);
      } catch {
        return undefined;
      }
    }
    if (typeof fr.content === 'string' && fr.content.length > 0) {
      return fr.content;
    }
    return undefined;
  }

  /**
   * Extract a JSON record from text by finding balanced braces
   */
  private extractJsonRecordFromText(text: string): Record<string, unknown> | undefined {
    const length = text.length;
    let index = 0;
    // eslint-disable-next-line functional/no-loop-statements
    while (index < length) {
      const startIdx = text.indexOf('{', index);
      if (startIdx === -1) break;
      let depth = 0;
      let inString = false;
      let escape = false;
      // eslint-disable-next-line functional/no-loop-statements
      for (let cursor = startIdx; cursor < length; cursor += 1) {
        const ch = text[cursor];
        if (inString) {
          if (escape) {
            escape = false;
          } else if (ch === '\\') {
            escape = true;
          } else if (ch === '"') {
            inString = false;
          }
          continue;
        }
        if (ch === '"') {
          inString = true;
          continue;
        }
        if (ch === '{') {
          depth += 1;
          continue;
        }
        if (ch === '}') {
          depth -= 1;
          if (depth === 0) {
            const candidate = text.slice(startIdx, cursor + 1);
            const parsed = parseJsonRecord(candidate);
            if (parsed !== undefined) {
              return parsed;
            }
            break;
          }
          continue;
        }
      }
      index = startIdx + 1;
    }
    return undefined;
  }
}
