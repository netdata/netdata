/**
 * XmlToolTransport - Manages XML-based tool calling protocol for AI agent sessions.
 *
 * This class encapsulates all state and logic for the XML tool transport modes ('xml' and 'xml-final').
 * It handles:
 * - Turn lifecycle (resetting state between turns)
 * - Building XML-NEXT and XML-PAST prompt messages
 * - Parsing XML tool calls from LLM responses
 * - Recording tool execution results for XML-PAST
 */

import crypto from 'node:crypto';

import type { OutputFormatId } from './formats.js';
import type { ConversationMessage, MCPTool, ToolCall } from './types.js';

import {
  XML_FINAL_REPORT_NOT_JSON,
  xmlMalformedMismatch,
  xmlMissingClosingTag,
  xmlSlotMismatch,
  xmlToolPayloadNotJson,
} from './llm-messages.js';
import { sanitizeToolName, truncateUtf8WithNotice } from './utils.js';
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

// Configuration for building messages
export interface XmlBuildMessagesConfig {
  turn: number;
  maxTurns: number | undefined;
  tools: readonly MCPTool[];
  maxToolCallsPerTurn: number;
  progressToolEnabled: boolean;
  finalReportToolName: string;
  resolvedFormat: OutputFormatId | undefined;
  expectedJsonSchema: Record<string, unknown> | undefined;
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
}

// Callback for logging/errors during parsing
export interface XmlParseCallbacks {
  onTurnFailure: (reason: string) => void;
  onLog: (entry: { severity: 'WRN'; message: string }) => void;
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

    const nextContent = renderXmlNext({
      nonce,
      turn: config.turn,
      maxTurns: config.maxTurns,
      tools: [],
      slotTemplates,
      progressSlot: undefined,
      mode: 'xml-final',
      expectedFinalFormat: (config.resolvedFormat ?? 'markdown') as XmlNextPayload['expectedFinalFormat'],
      finalSchema: config.resolvedFormat === 'json' ? config.expectedJsonSchema : undefined,
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

    // xml-final mode does not send past entries
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
   * Returns the extracted content if successful, undefined otherwise.
   */
  private tryExtractUnclosedFinalReport(
    content: string,
    stopReason: string | undefined,
    callbacks: XmlParseCallbacks
  ): { slotId: string; content: string } | undefined {
    if (this.nonce === undefined) return undefined;

    const finalSlotId = `${this.nonce}-FINAL`;
    const finalReportOpenTag = `<ai-agent-${finalSlotId}`;

    // Check if content has an unclosed final report opening tag
    const openTagIdx = content.indexOf(finalReportOpenTag);
    if (openTagIdx === -1) return undefined;

    // Check if there's a closing tag
    const closeTag = `</ai-agent-${finalSlotId}>`;
    if (content.includes(closeTag)) return undefined; // Has closing tag, let normal parsing handle it

    // Extract the opening tag to find where content starts
    const afterOpenTag = content.slice(openTagIdx);
    const openTagEndMatch = /^<ai-agent-[A-Za-z0-9\-]+[^>]*>/.exec(afterOpenTag);
    if (openTagEndMatch === null) return undefined; // Incomplete opening tag

    // Check if this is actually a final report tool
    if (!afterOpenTag.includes('tool="agent__final_report"')) return undefined;

    // Extract content after the opening tag
    const contentStart = openTagIdx + openTagEndMatch[0].length;
    const extractedContent = content.slice(contentStart).trim();

    if (extractedContent.length === 0) return undefined; // No content to extract

    // Check stop reason to decide whether to accept
    if (TRUNCATION_STOP_REASONS.has(stopReason ?? '')) {
      // Truncated - reject and retry
      callbacks.onLog({ severity: 'WRN', message: `Final report truncated (stopReason=${stopReason ?? 'unknown'}), will retry.` });
      return undefined;
    }

    if (NORMAL_COMPLETION_STOP_REASONS.has(stopReason ?? '')) {
      // Normal completion - accept without closing tag
      return { slotId: finalSlotId, content: extractedContent };
    }

    // Unknown/undefined stop reason - log warning but accept
    callbacks.onLog({ severity: 'WRN', message: `Accepting unclosed final report with unknown stopReason='${stopReason ?? 'undefined'}'.` });
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
    const unclosedFinal = this.tryExtractUnclosedFinalReport(content, context.stopReason, callbacks);
    if (unclosedFinal !== undefined) {
      const expectedFormat = context.resolvedFormat ?? 'text';
      const finalReportToolName = 'agent__final_report';

      // Try to parse as JSON first
      let parameters: Record<string, unknown>;
      try {
        parameters = JSON.parse(unclosedFinal.content) as Record<string, unknown>;
      } catch {
        // Not JSON - wrap as text content
        if (expectedFormat === 'json') {
          // JSON expected but got text - this is an error even for unclosed tags
          callbacks.onTurnFailure(XML_FINAL_REPORT_NOT_JSON);
          callbacks.onLog({ severity: 'WRN', message: XML_FINAL_REPORT_NOT_JSON });
          return {
            toolCalls: undefined,
            errors: [{
              slotId: unclosedFinal.slotId,
              tool: finalReportToolName,
              status: 'failed',
              request: truncateUtf8WithNotice(unclosedFinal.content, 4096),
              response: XML_FINAL_REPORT_NOT_JSON,
            }]
          };
        }
        parameters = {
          report_format: expectedFormat,
          report_content: unclosedFinal.content,
        };
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
    if (flushResult.leftover.trim().length > 0 && flushResult.leftover.includes(XML_TAG_PREFIX)) {
      const hasClosing = flushResult.leftover.includes('</ai-agent-');
      const slotMatch = /<ai-agent-([A-Za-z0-9\-]+)/.exec(flushResult.leftover);
      const capturedSlot = slotMatch?.[1] ?? `(partial: ${flushResult.leftover.slice(0, 60).replace(/\n/g, ' ')})`;
      const reason = hasClosing
        ? xmlSlotMismatch(capturedSlot)
        : xmlMissingClosingTag(capturedSlot);

      callbacks.onTurnFailure(reason);

      const errorEntry: XmlPastEntry = {
        slotId: slotMatch?.[1] ?? 'malformed',
        tool: 'agent__final_report',
        status: 'failed',
        request: truncateUtf8WithNotice(flushResult.leftover, 4096),
        response: reason,
      };
      errors.push(errorEntry);
      this.thisTurnEntries.push(errorEntry);

      callbacks.onLog({ severity: 'WRN', message: reason });
    }

    if (parsedSlots.length === 0) {
      // Check if there was an attempt at XML that failed
      if (content.includes(XML_TAG_PREFIX)) {
        const slotMatch = /<ai-agent-([A-Za-z0-9\-]+)/.exec(content);
        const missingClosing = !content.includes('</ai-agent-');
        // Extract snippet around the tag for debugging
        const tagIdx = content.indexOf(XML_TAG_PREFIX);
        const snippet = content.slice(tagIdx, tagIdx + 60).replace(/\n/g, ' ');
        const slotInfo = slotMatch?.[1] ?? `(partial: ${snippet})`;
        const reason = missingClosing
          ? xmlMissingClosingTag(slotInfo)
          : xmlMalformedMismatch(slotInfo);

        callbacks.onTurnFailure(reason);
        callbacks.onLog({ severity: 'WRN', message: reason });

        const errorEntry: XmlPastEntry = {
          slotId: slotMatch?.[1] ?? 'malformed',
          tool: 'agent__final_report',
          status: 'failed',
          request: truncateUtf8WithNotice(content, 4096),
          response: reason,
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
          const baseReason = xmlToolPayloadNotJson(call.name);
          callbacks.onTurnFailure(baseReason);

          const errorEntry: XmlPastEntry = {
            slotId: slot.slotId,
            tool: call.name,
            status: 'failed',
            request: truncateUtf8WithNotice(asString, 4096),
            response: baseReason,
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
      try { return truncateUtf8WithNotice(JSON.stringify(parameters), 4096); } catch { return '[unserializable]'; }
    })();
    const safeResponse = truncateUtf8WithNotice(response, 4096);

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
   * Check if native tool calls should be ignored (xml mode only).
   */
  shouldIgnoreNativeToolCalls(): boolean {
    return false;
  }

  /**
   * Check if native tool calls should be merged with XML (xml-final mode).
   */
  shouldMergeNativeToolCalls(): boolean {
    return true;
  }
}

/**
 * Filter for streaming XML final reports.
 * Strips <ai-agent-{nonce}-FINAL ...> tags and passes through content.
 */
export class XmlFinalReportFilter {
  private readonly openTagPrefix: string;
  private readonly closeTag: string;
  private buffer = '';
  private state: 'search_open' | 'buffering_open' | 'inside' | 'buffering_close' | 'done' = 'search_open';
  private _hasStreamedContent = false;

  constructor(nonce: string) {
    this.openTagPrefix = `<ai-agent-${nonce}-FINAL`;
    this.closeTag = `</ai-agent-${nonce}-FINAL>`;
  }

  get hasStreamedContent(): boolean {
    return this._hasStreamedContent;
  }

  process(chunk: string): string {
    let output = '';
    // eslint-disable-next-line functional/no-loop-statements
    for (const char of chunk) {
      if (this.state === 'search_open') {
        if (char === '<') {
          this.state = 'buffering_open';
          this.buffer = char;
        } else {
          output += char;
        }
      } else if (this.state === 'buffering_open') {
        this.buffer += char;
        // Check prefix match
        if (this.buffer.length <= this.openTagPrefix.length) {
          if (!this.openTagPrefix.startsWith(this.buffer)) {
            output += this.buffer;
            this.buffer = '';
            this.state = 'search_open';
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
            output += this.buffer;
            this.buffer = '';
            this.state = 'search_open';
          }
        }
      } else if (this.state === 'inside') {
        if (char === '<') {
          this.state = 'buffering_close';
          this.buffer = char;
        } else {
          output += char;
        }
      } else if (this.state === 'buffering_close') {
        this.buffer += char;
        if (!this.closeTag.startsWith(this.buffer)) {
          output += this.buffer;
          this.buffer = '';
          this.state = 'inside';
        } else if (this.buffer === this.closeTag) {
          this.buffer = '';
          this.state = 'done';
        }
      } else {
        output += char;
      }
    }
    return output;
  }

  flush(): string {
    const ret = this.buffer;
    this.buffer = '';
    return ret;
  }
}
