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

import { sanitizeToolName, truncateUtf8WithNotice } from './utils.js';
import {
  buildToolCallFromParsed,
  createXmlParser,
  renderXmlNext,
  renderXmlPast,
  type XmlNextPayload,
  type XmlPastEntry,
  type XmlSlotTemplate,
} from './xml-tools.js';

// Transport mode type
export type XmlTransportMode = 'xml' | 'xml-final';

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
  private readonly mode: XmlTransportMode;
  private parser: ReturnType<typeof createXmlParser>;

  // Current turn state
  private nonce: string | undefined;
  private slots: XmlSlotTemplate[] | undefined;
  private allowedTools: Set<string> | undefined;

  // Entry tracking
  private pastEntries: XmlPastEntry[] = [];
  private thisTurnEntries: XmlPastEntry[] = [];

  constructor(mode: XmlTransportMode) {
    this.mode = mode;
    this.parser = createXmlParser();
  }

  /**
   * Get the transport mode.
   */
  getMode(): XmlTransportMode {
    return this.mode;
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

    const nonce = crypto.randomUUID().replace(/-/g, '').slice(0, 8);
    const maxCalls = Math.max(1, config.maxToolCallsPerTurn);
    const allTools = config.tools.map((t) => ({ ...t, name: sanitizeToolName(t.name) }));
    const xmlMode: 'xml' | 'xml-final' = this.mode;

    const includeProgress = config.progressToolEnabled && xmlMode !== 'xml-final';
    const finalReportToolName = config.finalReportToolName;

    const toolsForSlots = xmlMode === 'xml-final'
      ? []
      : allTools.filter((t) => {
          const n = sanitizeToolName(t.name);
          return n !== finalReportToolName && n !== 'agent__progress_report';
        });

    const slotToolNames = toolsForSlots.map((t) => t.name);
    const slotTemplatesBase: XmlSlotTemplate[] = slotToolNames.length === 0
      ? []
      : Array.from({ length: maxCalls }, (_v, idx) => ({
          slotId: `${nonce}-${String(idx + 1).padStart(4, '0')}`,
          tools: slotToolNames
        }));

    const finalSlotId = `${nonce}-FINAL`;
    const progressSlotId = `${nonce}-PROGRESS`;

    const slotTemplates: XmlSlotTemplate[] = [...slotTemplatesBase];
    slotTemplates.push({ slotId: finalSlotId, tools: [finalReportToolName] });
    if (includeProgress && slotToolNames.length > 0) {
      slotTemplates.push({ slotId: progressSlotId, tools: ['agent__progress_report'] });
    }

    const toolsForRender = allTools
      .filter((t) => {
        const n = sanitizeToolName(t.name);
        return n !== finalReportToolName && n !== 'agent__progress_report';
      })
      .map((t) => ({ name: t.name, schema: t.inputSchema }));

    const nextContent = renderXmlNext({
      nonce,
      turn: config.turn,
      maxTurns: config.maxTurns,
      tools: toolsForRender,
      slotTemplates,
      progressSlot: includeProgress && slotToolNames.length > 0 ? { slotId: progressSlotId } : undefined,
      finalReportSlot: { slotId: finalSlotId },
      mode: xmlMode,
      expectedFinalFormat: (config.resolvedFormat ?? 'markdown') as XmlNextPayload['expectedFinalFormat'],
      finalSchema: config.resolvedFormat === 'json' ? config.expectedJsonSchema : undefined,
    });

    const nextMessage: ConversationMessage = {
      role: 'user',
      content: nextContent,
      noticeType: 'xml-next'
    };

    let pastMessage: ConversationMessage | undefined;
    if (this.mode !== 'xml-final' && this.pastEntries.length > 0) {
      pastMessage = {
        role: 'user',
        content: renderXmlPast({ entries: this.pastEntries }),
        noticeType: 'xml-past'
      };
    }

    const allowedToolNames = new Set<string>();
    if (xmlMode === 'xml-final') {
      allowedToolNames.add(finalReportToolName);
      if (includeProgress && slotToolNames.length > 0) {
        allowedToolNames.add('agent__progress_report');
      }
    } else {
      allTools.forEach((t) => allowedToolNames.add(t.name));
      if (includeProgress && slotToolNames.length > 0) {
        allowedToolNames.add('agent__progress_report');
      }
      allowedToolNames.add(finalReportToolName);
    }

    // Store state for response parsing
    this.nonce = nonce;
    this.slots = slotTemplates;
    this.allowedTools = allowedToolNames;

    return {
      pastMessage,
      nextMessage,
      nonce,
      slotTemplates,
      allowedTools: allowedToolNames
    };
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

    const allowedSlots = new Set(this.slots.map((slot) => slot.slotId));
    const parsed = this.parser.parseChunk(content, this.nonce, allowedSlots, this.allowedTools);
    const flushResult = this.parser.flush();
    const parsedSlots = [...parsed, ...flushResult.slots];
    const errors: XmlPastEntry[] = [];

    // Handle leftover content (incomplete/malformed tags)
    if (flushResult.leftover.trim().length > 0) {
      const hasClosing = flushResult.leftover.includes('</ai-agent-');
      const capturedSlot = (() => {
        const match = /<ai-agent-([A-Za-z0-9\-]+)/.exec(flushResult.leftover);
        return match?.[1] ?? 'unknown';
      })();
      const reason = hasClosing
        ? `Tag ignored: slot '${capturedSlot}' does not match the current nonce/slot for this turn.`
        : `Malformed XML: missing closing tag for '${capturedSlot}'.`;

      callbacks.onTurnFailure(reason);

      const errorEntry: XmlPastEntry = {
        slotId: capturedSlot,
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
      if (content.includes('<ai-agent-')) {
        const missingClosing = !content.includes('</ai-agent-');
        const reason = missingClosing
          ? 'Malformed XML: missing closing tag for ai-agent-*.'
          : 'Malformed XML: nonce/slot/tool mismatch or empty content.';

        callbacks.onTurnFailure(reason);
        callbacks.onLog({ severity: 'WRN', message: reason });

        const capturedSlot = (() => {
          const match = /<ai-agent-([A-Za-z0-9\-]+)/.exec(content);
          return match?.[1] ?? 'unknown';
        })();

        const errorEntry: XmlPastEntry = {
          slotId: capturedSlot,
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

      if (asString !== undefined) {
        const parsedJson = (() => {
          try { return JSON.parse(asString) as Record<string, unknown>; } catch { return undefined; }
        })();

        if (parsedJson !== undefined) {
          call.parameters = parsedJson;
        } else if (call.name === finalReportToolName) {
          if (expectedFormat === 'json') {
            const baseReason = 'Final report payload is not valid JSON. Use the JSON schema from XML-NEXT.';
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
          // Non-JSON format: wrap as text content
          call.parameters = {
            status: 'success',
            report_format: expectedFormat,
            report_content: asString,
          } as Record<string, unknown>;
        } else {
          const baseReason = `Tool \`${call.name}\` payload is not valid JSON. Provide a JSON object.`;
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
    return this.mode === 'xml';
  }

  /**
   * Check if native tool calls should be merged with XML (xml-final mode).
   */
  shouldMergeNativeToolCalls(): boolean {
    return this.mode === 'xml-final';
  }
}
