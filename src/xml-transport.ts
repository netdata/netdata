/**
 * XmlToolTransport - Manages XML-based tool calling protocol for AI agent sessions.
 *
 * This class encapsulates all state and logic for the XML final-report transport.
 * It handles:
 * - Turn lifecycle (resetting state between turns)
 * - Building XML-NEXT and XML-PAST prompt messages
 * - Parsing XML tool calls from LLM responses
 * - Recording tool execution results for XML-PAST
 */

import crypto from 'node:crypto';

import type { OutputFormatId } from './formats.js';
import type { ConversationMessage, MCPTool, ToolCall } from './types.js';

import { renderTurnFailedSlug, type TurnFailedSlug } from './llm-messages-turn-failed.js';
import { stripLeadingThinkBlock, ThinkTagStreamFilter } from './think-tag-filter.js';
import { TRUNCATE_PREVIEW_BYTES, truncateToBytes, truncateToChars } from './truncation.js';
import { sanitizeToolName } from './utils.js';
import {
  buildToolCallFromParsed,
  createXmlParser,
  renderXmlNext,
  type XmlNextPayload,
  type XmlPastEntry,
  type XmlSlotTemplate,
} from './xml-tools.js';

// XML tag prefix for detection
const XML_TAG_PREFIX = '<ai-agent-';

// Stop reasons indicating normal completion (accept final report without closing tag)
const NORMAL_COMPLETION_STOP_REASONS = new Set([
  'stop',       // OpenAI
  'end_turn',   // Anthropic
  'end',        // Generic
  'eos',        // End of sequence
]);

// Stop reasons indicating truncation (reject, should retry)
const TRUNCATION_STOP_REASONS = new Set([
  'length',     // OpenAI
  'max_tokens', // Anthropic
]);

// Structured output formats that cannot be truncated (must be complete)
const STRUCTURED_FORMATS = new Set<OutputFormatId>(['json', 'slack-block-kit']);


// Configuration for building messages
export interface XmlBuildMessagesConfig {
  turn: number;
  maxTurns: number | undefined;
  tools: readonly MCPTool[];
  maxToolCallsPerTurn: number;
  taskStatusToolEnabled: boolean;
  finalReportToolName: string;
  resolvedFormat: OutputFormatId | undefined;
  expectedJsonSchema: Record<string, unknown> | undefined;
  // Retry info
  attempt: number;
  maxRetries: number;
  // Context window info (for percentage calculation)
  contextPercentUsed: number;
  forcedFinalTurnReason?: 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion';
  finalTurnTools?: string[];
  noticeContent?: string;
  // Wasted turn tracking - consecutive task_status-only turns
  consecutiveProgressOnlyTurns?: number;
}

// Result from building messages
export interface XmlBuildMessagesResult {
  pastMessage: ConversationMessage | undefined;
  nextMessage: ConversationMessage;
  nonce: string;
  slotTemplates: XmlSlotTemplate[];
  allowedTools: Set<string>;
}

// Context for parsing responses
export interface XmlParseContext {
  turn: number;
  resolvedFormat: OutputFormatId | undefined;
  stopReason?: string;  // LLM stop reason - used to accept unclosed final reports on normal completion
  maxOutputTokens?: number;  // For truncation error messages
}

// Callback for logging/errors during parsing
export interface XmlParseCallbacks {
  recordTurnFailure: (slug: TurnFailedSlug, reason?: string) => void;
  logWarning: (entry: { severity: 'WRN'; message: string }) => void;
}

// Result from parsing a single message
export interface XmlParseMessageResult {
  toolCalls: ToolCall[] | undefined;
  errors: XmlPastEntry[];
}

// Result from recording a tool execution
export interface XmlToolResultEntry {
  slotId: string;
  tool: string;
  status: 'ok' | 'failed';
  durationMs: number;
  request: string;
  response: string;
}

/**
 * XmlToolTransport manages the XML tool calling protocol state and operations.
 */
export class XmlToolTransport {
  private parser: ReturnType<typeof createXmlParser>;

  private readonly mode = 'xml-final';

  // Session-level state: fixed nonce and incrementing slot counter
  private readonly sessionNonce: string;
  private nextSlotNumber = 1;

  // Current turn state
  private nonce: string | undefined;
  private slots: XmlSlotTemplate[] | undefined;
  private allowedTools: Set<string> | undefined;

  // Entry tracking
  private pastEntries: XmlPastEntry[] = [];
  private thisTurnEntries: XmlPastEntry[] = [];

  constructor() {
    this.parser = createXmlParser();
    this.sessionNonce = crypto.randomUUID().replace(/-/g, '').slice(0, 8);
  }

  /**
   * Get the session nonce (fixed for entire session, used in system prompt).
   */
  getSessionNonce(): string {
    return this.sessionNonce;
  }

  /**
   * Get the current nonce (for tool result recording).
   */
  getNonce(): string | undefined {
    return this.nonce;
  }

  /**
   * Begin a new turn - moves thisTurnEntries to pastEntries and resets state.
   */
  beginTurn(): void {
    this.pastEntries = this.thisTurnEntries;
    this.thisTurnEntries = [];
  }

  /**
   * Build XML-NEXT and XML-PAST messages for the current turn.
   */
  buildMessages(config: XmlBuildMessagesConfig): XmlBuildMessagesResult {
    // Reset parser and entries for this turn
    this.parser = createXmlParser();
    this.thisTurnEntries = [];

    const nonce = this.sessionNonce;
    const finalReportToolName = config.finalReportToolName;
    const finalSlotId = `${nonce}-FINAL`;
    const slotTemplates: XmlSlotTemplate[] = [{ slotId: finalSlotId, tools: [finalReportToolName] }];

    // Agent can call tools if anything besides final_report is available
    const hasExternalTools = config.tools.some(t => t.name !== 'agent__final_report');

    const nextContent = typeof config.noticeContent === 'string' && config.noticeContent.trim().length > 0
      ? config.noticeContent
      : renderXmlNext({
      nonce,
      turn: config.turn,
      maxTurns: config.maxTurns,
      tools: [],
      slotTemplates,
      taskStatusToolEnabled: config.taskStatusToolEnabled,
      expectedFinalFormat: (config.resolvedFormat ?? 'markdown') as XmlNextPayload['expectedFinalFormat'],
      finalSchema: config.resolvedFormat === 'json' ? config.expectedJsonSchema : undefined,
      attempt: config.attempt,
      maxRetries: config.maxRetries,
      contextPercentUsed: config.contextPercentUsed,
      hasExternalTools,
      forcedFinalTurnReason: config.forcedFinalTurnReason,
      finalTurnTools: config.finalTurnTools,
      consecutiveProgressOnlyTurns: config.consecutiveProgressOnlyTurns,
    });

    const nextMessage: ConversationMessage = {
      role: 'user',
      content: nextContent,
      noticeType: 'xml-next'
    };

    const allowedToolNames = new Set<string>();
    allowedToolNames.add(finalReportToolName);

    // Store state for response parsing
    this.nonce = nonce;
    this.slots = slotTemplates;
    this.allowedTools = allowedToolNames;

    // xml-final transport does not send past entries
    const pastMessage: ConversationMessage | undefined = undefined;

    return {
      pastMessage,
      nextMessage,
      nonce,
      slotTemplates,
      allowedTools: allowedToolNames
    };
  }

  /**
   * Try to extract an unclosed final report when stop reason indicates normal completion.
   * Returns:
   * - { slotId, content, truncated? } on success (possibly with truncated flag for unstructured)
   * - { truncatedRejected: true } when structured format was truncated (must retry)
   * - undefined when no final report found
   */
  private tryExtractUnclosedFinalReport(
    content: string,
    stopReason: string | undefined,
    resolvedFormat: OutputFormatId | undefined,
    maxOutputTokens: number | undefined,
    callbacks: XmlParseCallbacks
  ): { slotId: string; content: string; truncated?: boolean } | { truncatedRejected: true } | undefined {
    if (this.nonce === undefined) return undefined;

    const finalSlotId = `${this.nonce}-FINAL`;
    const finalReportOpenTag = `<ai-agent-${finalSlotId}`;
    const sanitizedContent = stripLeadingThinkBlock(content).stripped;

    // Check if content has an unclosed final report opening tag
    const openTagIdx = sanitizedContent.indexOf(finalReportOpenTag);
    if (openTagIdx === -1) return undefined;

    // Check if there's a closing tag
    const closeTag = `</ai-agent-${finalSlotId}>`;
    if (sanitizedContent.includes(closeTag)) return undefined; // Has closing tag, let normal parsing handle it

    // Extract the opening tag to find where content starts
    const afterOpenTag = sanitizedContent.slice(openTagIdx);
    const openTagEndMatch = /^<ai-agent-[A-Za-z0-9\-]+[^>]*>/.exec(afterOpenTag);
    if (openTagEndMatch === null) return undefined; // Incomplete opening tag
    // Tag name already identifies final report via -FINAL suffix, no need for tool attribute

    // Extract content after the opening tag
    const contentStart = openTagIdx + openTagEndMatch[0].length;
    const extractedContent = sanitizedContent.slice(contentStart).trim();

    if (extractedContent.length === 0) return undefined; // No content to extract

    // Check stop reason to decide whether to accept
    if (TRUNCATION_STOP_REASONS.has(stopReason ?? '')) {
      const isStructured = resolvedFormat !== undefined && STRUCTURED_FORMATS.has(resolvedFormat);

      if (isStructured) {
        // Structured format: MUST retry - log with content dump for diagnosis
        const preview = truncateToChars(extractedContent, 3000) ?? extractedContent;
        callbacks.logWarning({
          severity: 'WRN',
          message: `Final report truncated (stopReason=${stopReason ?? 'unknown'}, format=${resolvedFormat}, length=${String(extractedContent.length)} chars). Will retry.\nTruncated output:\n${preview}`
        });
        const reason = typeof maxOutputTokens === 'number' ? `token_limit: ${String(maxOutputTokens)} tokens` : undefined;
        callbacks.recordTurnFailure('xml_structured_output_truncated', reason);
        return { truncatedRejected: true };
      } else {
        // Unstructured format: Accept - log without dump (content visible in final report)
        callbacks.logWarning({
          severity: 'WRN',
          message: `Final report truncated (stopReason=${stopReason ?? 'unknown'}, format=${resolvedFormat ?? 'unknown'}, length=${String(extractedContent.length)} chars). Accepting truncated output.`
        });
        return { slotId: finalSlotId, content: extractedContent, truncated: true };
      }
    }

    if (NORMAL_COMPLETION_STOP_REASONS.has(stopReason ?? '')) {
      // Normal completion - accept without closing tag
      return { slotId: finalSlotId, content: extractedContent };
    }

    // Unknown/undefined stop reason - log warning but accept
    callbacks.logWarning({ severity: 'WRN', message: `Accepting unclosed final report with unknown stopReason='${stopReason ?? 'undefined'}'.` });
    return { slotId: finalSlotId, content: extractedContent };
  }

  /**
   * Parse XML tool calls from an assistant message.
   * Returns parsed tool calls and any errors to record.
   */
  parseAssistantMessage(
    content: string,
    context: XmlParseContext,
    callbacks: XmlParseCallbacks
  ): XmlParseMessageResult {
    if (this.nonce === undefined || this.slots === undefined || this.allowedTools === undefined) {
      return { toolCalls: undefined, errors: [] };
    }

    // First, try to extract unclosed final report if stop reason indicates normal completion
    const unclosedFinal = this.tryExtractUnclosedFinalReport(
      content,
      context.stopReason,
      context.resolvedFormat,
      context.maxOutputTokens,
      callbacks
    );

    if (unclosedFinal !== undefined) {
      // Check if this was a structured format truncation that was rejected
      if ('truncatedRejected' in unclosedFinal) {
        // Structured format truncation - already reported error via recordTurnFailure, don't add more errors
        return { toolCalls: undefined, errors: [] };
      }

      const expectedFormat = context.resolvedFormat ?? 'text';
      const finalReportToolName = 'agent__final_report';

      // Try to parse as JSON first
      let parameters: Record<string, unknown>;
      try {
        const parsed = JSON.parse(unclosedFinal.content) as Record<string, unknown>;
        parameters = {
          status: 'success',
          report_format: expectedFormat,
          _rawPayload: unclosedFinal.content,
          content_json: parsed,
        };
      } catch {
        // Not JSON - wrap as text content
        if (expectedFormat === 'json') {
          // JSON expected but got text - this is an error even for unclosed tags
          const failureMessage = renderTurnFailedSlug('xml_final_report_not_json');
          callbacks.recordTurnFailure('xml_final_report_not_json');
          callbacks.logWarning({ severity: 'WRN', message: failureMessage });
          return {
            toolCalls: undefined,
            errors: [{
              slotId: unclosedFinal.slotId,
              tool: finalReportToolName,
              status: 'failed',
              request: truncateToBytes(unclosedFinal.content, TRUNCATE_PREVIEW_BYTES) ?? unclosedFinal.content,
              response: failureMessage,
            }]
          };
        }
        parameters = {
          report_format: expectedFormat,
          report_content: unclosedFinal.content,
        };
      }

      // Add truncated flag if content was truncated (unstructured format)
      if (unclosedFinal.truncated === true) {
        parameters._truncated = true;
      }

      const toolCall: ToolCall = {
        id: unclosedFinal.slotId,
        name: finalReportToolName,
        parameters,
      };

      return { toolCalls: [toolCall], errors: [] };
    }

    const allowedSlots = new Set(this.slots.map((slot) => slot.slotId));
    const parsed = this.parser.parseChunk(content, this.nonce, allowedSlots, this.allowedTools);
    const flushResult = this.parser.flush();
    const parsedSlots = [...parsed, ...flushResult.slots];
    const errors: XmlPastEntry[] = [];

    // Handle leftover content (incomplete/malformed tags)
    // Only flag as error if leftover actually contains an XML tag attempt
    const sanitizedLeftover = stripLeadingThinkBlock(flushResult.leftover).stripped;
    if (sanitizedLeftover.trim().length > 0 && sanitizedLeftover.includes(XML_TAG_PREFIX)) {
      const hasClosing = sanitizedLeftover.includes('</ai-agent-');
      const slotMatch = /<ai-agent-([A-Za-z0-9\-]+)/.exec(sanitizedLeftover);
      const capturedSlot = slotMatch?.[1] ?? `(partial: ${sanitizedLeftover.slice(0, 60).replace(/\n/g, ' ')})`;
      const slug: TurnFailedSlug = hasClosing ? 'xml_slot_mismatch' : 'xml_missing_closing_tag';
      const reasonDetail = `slot: ${capturedSlot}`;
      const reasonMessage = renderTurnFailedSlug(slug, reasonDetail);

      callbacks.recordTurnFailure(slug, reasonDetail);

      const errorEntry: XmlPastEntry = {
        slotId: slotMatch?.[1] ?? 'malformed',
        tool: 'agent__final_report',
        status: 'failed',
        request: truncateToBytes(sanitizedLeftover, TRUNCATE_PREVIEW_BYTES) ?? sanitizedLeftover,
        response: reasonMessage,
      };
      errors.push(errorEntry);
      this.thisTurnEntries.push(errorEntry);

      callbacks.logWarning({ severity: 'WRN', message: reasonMessage });
    }

    if (parsedSlots.length === 0) {
      // Check if there was an attempt at XML that failed
      const sanitizedContent = stripLeadingThinkBlock(content).stripped;
      if (sanitizedContent.includes(XML_TAG_PREFIX)) {
        const slotMatch = /<ai-agent-([A-Za-z0-9\-]+)/.exec(sanitizedContent);
        const missingClosing = !sanitizedContent.includes('</ai-agent-');
        // Extract snippet around the tag for debugging
        const tagIdx = sanitizedContent.indexOf(XML_TAG_PREFIX);
        const snippet = sanitizedContent.slice(tagIdx, tagIdx + 60).replace(/\n/g, ' ');
        const slotInfo = slotMatch?.[1] ?? `(partial: ${snippet})`;
        const slug: TurnFailedSlug = missingClosing ? 'xml_missing_closing_tag' : 'xml_malformed_mismatch';
        const reasonDetail = `slot: ${slotInfo}`;
        const reasonMessage = renderTurnFailedSlug(slug, reasonDetail);

        callbacks.recordTurnFailure(slug, reasonDetail);
        callbacks.logWarning({ severity: 'WRN', message: reasonMessage });

        const errorEntry: XmlPastEntry = {
          slotId: slotMatch?.[1] ?? 'malformed',
          tool: 'agent__final_report',
          status: 'failed',
          request: truncateToBytes(sanitizedContent, TRUNCATE_PREVIEW_BYTES) ?? sanitizedContent,
          response: reasonMessage,
        };
        errors.push(errorEntry);
        this.thisTurnEntries.push(errorEntry);
      }
      return { toolCalls: undefined, errors };
    }

    // Process valid slots into tool calls
    const validCalls: ToolCall[] = [];
    const finalReportToolName = 'agent__final_report';
    const expectedFormat = context.resolvedFormat ?? 'text';

    parsedSlots.forEach((slot) => {
      const call = buildToolCallFromParsed(slot, slot.slotId);
      const params = call.parameters as unknown;
      const asString = typeof params === 'string' ? params.trim() : undefined;

      // For final_report: build parameters with clean separation of wrapper vs payload
      // Wrapper fields (status, format) come from XML attributes
      // Payload is the raw content, untouched
      if (call.name === finalReportToolName && asString !== undefined) {
        const wrapperStatus = slot.statusAttr ?? 'success';
        const wrapperFormat = slot.formatAttr ?? expectedFormat;

        // Build clean parameters: wrapper fields + raw payload
        const finalParams: Record<string, unknown> = {
          status: wrapperStatus,
          report_format: wrapperFormat,
          _rawPayload: asString,  // Raw payload for downstream processing
        };
        call.parameters = finalParams;
        validCalls.push(call);
        return;
      }

      if (asString !== undefined) {
        const parsedJson = (() => {
          try { return JSON.parse(asString) as Record<string, unknown>; } catch { return undefined; }
        })();

        if (parsedJson !== undefined) {
          call.parameters = parsedJson;
        } else {
          const reasonDetail = `tool: ${call.name}`;
          const reasonMessage = renderTurnFailedSlug('xml_tool_payload_not_json', reasonDetail);
          callbacks.recordTurnFailure('xml_tool_payload_not_json', reasonDetail);

          const errorEntry: XmlPastEntry = {
            slotId: slot.slotId,
            tool: call.name,
            status: 'failed',
            request: truncateToBytes(asString, TRUNCATE_PREVIEW_BYTES) ?? asString,
            response: reasonMessage,
          };
          errors.push(errorEntry);
          this.thisTurnEntries.push(errorEntry);
          return;
        }
      }

      validCalls.push(call);
    });

    return {
      toolCalls: validCalls.length > 0 ? validCalls : undefined,
      errors
    };
  }

  /**
   * Record a tool execution result for XML-PAST.
   */
  recordToolResult(
    toolName: string,
    parameters: Record<string, unknown>,
    status: 'ok' | 'failed',
    response: string,
    durationMs: number,
    toolCallId?: string
  ): void {
    const slotId = typeof toolCallId === 'string' && toolCallId.length > 0
      ? toolCallId
      : `${this.nonce ?? 'xml'}-${String(this.thisTurnEntries.length + 1).padStart(4, '0')}`;

    const safeRequest = (() => {
      try {
        const json = JSON.stringify(parameters);
        return truncateToBytes(json, TRUNCATE_PREVIEW_BYTES) ?? json;
      } catch { return '[unserializable]'; }
    })();
    const safeResponse = truncateToBytes(response, TRUNCATE_PREVIEW_BYTES) ?? response;

    this.thisTurnEntries.push({
      slotId,
      tool: sanitizeToolName(toolName),
      status,
      durationMs,
      request: safeRequest,
      response: safeResponse
    });
  }

  /**
   * Add an error entry directly (for external error tracking).
   */
  addErrorEntry(entry: XmlPastEntry): void {
    this.thisTurnEntries.push(entry);
  }

  /**
   * Get entries recorded this turn.
   */
  getThisTurnEntries(): readonly XmlPastEntry[] {
    return this.thisTurnEntries;
  }

  /**
   * Get entries from the previous turn.
   */
  getPastEntries(): readonly XmlPastEntry[] {
    return this.pastEntries;
  }

  /**
   * Native tool calls are always honored and merged with XML final-report parsing.
   */
  shouldIgnoreNativeToolCalls(): boolean { return false; }

  /**
   * Native tool calls are always merged (single xml-final transport).
   */
  shouldMergeNativeToolCalls(): boolean { return true; }
}

/**
 * Filter for streaming XML final reports.
 * Strips <ai-agent-{nonce}-FINAL ...> tags and passes through content.
 *
 * Handles reasoning models that embed <think>...</think> inline in content.
 * Leading whitespace is tolerated; the <think> block is excluded before
 * processing XML tags so examples inside thinking are never parsed.
 */
export class XmlFinalReportFilter {
  private readonly openTagPrefix: string;
  private readonly closeTag: string;
  private buffer = '';
  private state: 'search_open' | 'buffering_open' | 'inside' | 'buffering_close' | 'done' = 'search_open';
  private _hasStreamedContent = false;
  private readonly thinkFilter = new ThinkTagStreamFilter();

  constructor(nonce: string) {
    this.openTagPrefix = `<ai-agent-${nonce}-FINAL`;
    this.closeTag = `</ai-agent-${nonce}-FINAL>`;
  }

  get hasStreamedContent(): boolean {
    return this._hasStreamedContent;
  }

  process(chunk: string): string {
    let output = '';
    const split = this.thinkFilter.process(chunk);
    if (split.content.length === 0) return '';
    // eslint-disable-next-line functional/no-loop-statements
    for (const char of split.content) {
      output += this.processNormalChar(char);
    }
    return output;
  }

  private processNormalChar(char: string): string {
    if (this.state === 'search_open') {
      if (char === '<') {
        this.state = 'buffering_open';
        this.buffer = char;
        return '';
      }
      return char;
    }

    if (this.state === 'buffering_open') {
      this.buffer += char;
      // Check prefix match
      if (this.buffer.length <= this.openTagPrefix.length) {
        if (!this.openTagPrefix.startsWith(this.buffer)) {
          const out = this.buffer;
          this.buffer = '';
          this.state = 'search_open';
          return out;
        }
      } else {
        // Buffer longer than prefix
        if (this.buffer.startsWith(this.openTagPrefix)) {
          if (char === '>') {
            // Tag close
            this.buffer = '';
            this.state = 'inside';
            this._hasStreamedContent = true;
          }
        } else {
          // Should not happen if prefix matched before, but safety
          const out = this.buffer;
          this.buffer = '';
          this.state = 'search_open';
          return out;
        }
      }
      return '';
    }

    if (this.state === 'inside') {
      if (char === '<') {
        this.state = 'buffering_close';
        this.buffer = char;
        return '';
      }
      return char;
    }

    if (this.state === 'buffering_close') {
      this.buffer += char;
      if (!this.closeTag.startsWith(this.buffer)) {
        const out = this.buffer;
        this.buffer = '';
        this.state = 'inside';
        return out;
      }
      if (this.buffer === this.closeTag) {
        this.buffer = '';
        this.state = 'done';
      }
      return '';
    }

    // state === 'done'
    return char;
  }

  flush(): string {
    const ret = this.buffer;
    this.buffer = '';
    return ret;
  }
}

/**
 * Strict filter for streaming XML final reports.
 * Only emits content that appears inside the FINAL tag.
 */
export class XmlFinalReportStrictFilter {
  private readonly openTagPrefix: string;
  private readonly closeTag: string;
  private buffer = '';
  private state: 'search_open' | 'buffering_open' | 'inside' | 'buffering_close' | 'done' = 'search_open';
  private _hasStreamedContent = false;
  private readonly thinkFilter = new ThinkTagStreamFilter();

  constructor(nonce: string) {
    this.openTagPrefix = `<ai-agent-${nonce}-FINAL`;
    this.closeTag = `</ai-agent-${nonce}-FINAL>`;
  }

  get hasStreamedContent(): boolean {
    return this._hasStreamedContent;
  }

  process(chunk: string): string {
    let output = '';
    const split = this.thinkFilter.process(chunk);
    if (split.content.length === 0) return '';
    // eslint-disable-next-line functional/no-loop-statements
    for (const char of split.content) {
      output += this.processNormalChar(char);
    }
    return output;
  }

  private processNormalChar(char: string): string {
    if (this.state === 'search_open') {
      if (char === '<') {
        this.state = 'buffering_open';
        this.buffer = char;
      }
      return '';
    }

    if (this.state === 'buffering_open') {
      this.buffer += char;
      if (this.buffer.length <= this.openTagPrefix.length) {
        if (!this.openTagPrefix.startsWith(this.buffer)) {
          this.buffer = '';
          this.state = 'search_open';
        }
      } else {
        if (this.buffer.startsWith(this.openTagPrefix)) {
          if (char === '>') {
            this.buffer = '';
            this.state = 'inside';
            this._hasStreamedContent = true;
          }
        } else {
          this.buffer = '';
          this.state = 'search_open';
        }
      }
      return '';
    }

    if (this.state === 'inside') {
      if (char === '<') {
        this.state = 'buffering_close';
        this.buffer = char;
        return '';
      }
      return char;
    }

    if (this.state === 'buffering_close') {
      this.buffer += char;
      if (!this.closeTag.startsWith(this.buffer)) {
        const out = this.buffer;
        this.buffer = '';
        this.state = 'inside';
        return out;
      }
      if (this.buffer === this.closeTag) {
        this.buffer = '';
        this.state = 'done';
      }
      return '';
    }

    return '';
  }

  flush(): string {
    if (this.state !== 'inside') {
      this.buffer = '';
      return '';
    }
    const ret = this.buffer;
    this.buffer = '';
    return ret;
  }
}
