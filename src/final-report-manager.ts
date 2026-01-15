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
import type { FinalReportPayload } from './types.js';
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

import { getFormatSchema } from './formats.js';
import { isPlainObject, parseJsonRecord } from './utils.js';

// Format constant to avoid string duplication
const FORMAT_SLACK_BLOCK_KIT: OutputFormatId = 'slack-block-kit';

// Reusable type definitions
export type { FinalReportPayload };
export type PendingFinalReportPayload = Omit<FinalReportPayload, 'ts'>;
export type FinalReportSource = 'tool-call' | 'tool-message' | 'synthetic';

export const FINAL_REPORT_FORMAT_VALUES = ['json', 'markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'sub-agent', 'text'] as const satisfies readonly FinalReportPayload['format'][];


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
   * Get the resolved format for this session
   */
  getResolvedFormat(): OutputFormatId | undefined {
    return this.config.resolvedFormat;
  }

  /**
   * Commit a final report to the manager
   */
  commit(payload: PendingFinalReportPayload, source: FinalReportSource): void {
    this.finalReport = {
      format: payload.format,
      content: payload.content,
      content_json: payload.content_json,
      metadata: payload.metadata,
      ts: Date.now()
    };
    this.finalReportSource = source;
  }

  /**
   * Clear any committed or pending final report
   */
  clear(): void {
    this.finalReport = undefined;
    this.finalReportSource = undefined;
  }

  /**
   * Validate a pending payload against the expected JSON schema BEFORE committing.
   * For 'json' format: validates against the provided user schema.
   * For 'slack-block-kit' format: validates against the built-in Block Kit schema.
   * Returns validation result with errors if invalid.
   *
   * Use this to validate before calling commit() to enable retry on validation failure.
   */
  validatePayload(payload: PendingFinalReportPayload, schema: Record<string, unknown> | undefined): ValidationResult {
    return this.validateFinalReportData(payload, schema);
  }

  /**
   * Validate the committed final report against the expected JSON schema.
   * For 'json' format: validates against the provided user schema.
   * For 'slack-block-kit' format: validates against the built-in Block Kit schema.
   * Returns validation result with errors if invalid.
   */
  validateSchema(schema: Record<string, unknown> | undefined): ValidationResult {
    const fr = this.finalReport;
    if (fr === undefined) {
      return { valid: true };
    }
    return this.validateFinalReportData(fr, schema);
  }

  /**
   * Internal validation logic shared by validatePayload and validateSchema.
   */
  private validateFinalReportData(
    fr: PendingFinalReportPayload,
    schema: Record<string, unknown> | undefined
  ): ValidationResult {
    // Determine what to validate based on format
    let dataToValidate: unknown;
    let schemaToUse: Record<string, unknown> | undefined;

    if (fr.format === 'json' && fr.content_json !== undefined && schema !== undefined) {
      // User-defined JSON schema validation
      dataToValidate = fr.content_json;
      schemaToUse = schema;
    } else if (fr.format === FORMAT_SLACK_BLOCK_KIT) {
      // Built-in slack-block-kit schema validation
      schemaToUse = getFormatSchema(FORMAT_SLACK_BLOCK_KIT);
      if (schemaToUse === undefined) {
        return { valid: true }; // No schema defined, skip validation
      }
      // Parse content as JSON (slack-block-kit stores JSON in content string)
      if (typeof fr.content === 'string' && fr.content.trim().length > 0) {
        try {
          dataToValidate = JSON.parse(fr.content.trim());
        } catch {
          return {
            valid: false,
            errors: `invalid_json: ${FORMAT_SLACK_BLOCK_KIT} content is not valid JSON`,
            payloadPreview: fr.content.length > 200 ? `${fr.content.slice(0, 200)}…` : fr.content
          };
        }
      } else {
        return { valid: true }; // No content to validate
      }
    } else {
      return { valid: true }; // No validation needed for this format
    }

    try {
      this.ajv = this.ajv ?? new AjvCtor({ allErrors: true, strict: false });
      const validate = this.ajv.compile(schemaToUse);
      const valid = validate(dataToValidate);

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
          const raw = JSON.stringify(dataToValidate);
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

    const pickString = (value: unknown): string | undefined =>
      (typeof value === 'string' ? value : undefined);

    const formatRaw = pickString(json.report_format) ?? pickString(json.format);
    const contentJsonRaw = json.content_json;
    const contentJson = isPlainObject(contentJsonRaw) ? contentJsonRaw : undefined;
    const contentCandidate = pickString(json.report_content) ?? pickString(json.content);

    const formatCandidate = (formatRaw ?? '').trim();
    const formatMatch = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate);
    if (formatMatch === undefined) return undefined;

    let finalContent: string | undefined = contentCandidate;
    if ((finalContent === undefined || finalContent.trim().length === 0) && contentJson !== undefined) {
      try {
        finalContent = JSON.stringify(contentJson);
      } catch {
        finalContent = undefined;
      }
    }
    if (finalContent === undefined || finalContent.trim().length === 0) return undefined;

    const metadata = isPlainObject(json.metadata) ? json.metadata : undefined;

    const payload: PendingFinalReportPayload = {
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
