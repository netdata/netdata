import type { OutputFormatId } from './formats.js';
import type { ToolCall } from './types.js';

import { renderXmlNextTemplate, renderXmlPastTemplate } from './llm-messages.js';
import { truncateUtf8WithNotice } from './utils.js';

export interface XmlSlotTemplate {
  slotId: string;            // NONCE-0001 or NONCE-FINAL/PROGRESS
  tools: string[];           // allowed tool names for this slot
}

export interface XmlNextPayload {
  nonce: string;
  turn: number;
  maxTurns?: number;
  tools: { name: string; schema?: Record<string, unknown> }[];
  slotTemplates: XmlSlotTemplate[];
  progressSlot?: { slotId: string };
  expectedFinalFormat: OutputFormatId | 'text';
  finalSchema?: Record<string, unknown>;
  // Retry info
  attempt: number;
  maxRetries: number;
  // Context window percentage (0-100)
  contextPercentUsed: number;
  // Whether external tools (MCP, REST, etc.) are available
  hasExternalTools: boolean;
}

export interface XmlPastEntry {
  slotId: string;
  tool: string;
  status: 'ok' | 'failed';
  durationMs?: number;
  request: string;
  response: string;
}

export interface XmlPastPayload {
  entries: XmlPastEntry[];
}

export interface ParsedXmlSlot {
  slotId: string;
  tool: string;
  rawPayload: string;
  // XML attributes extracted from opening tag (for final_report)
  statusAttr?: string;
  formatAttr?: string;
}

interface ParserState {
  buffer: string;
}

const OPEN_PREFIX = '<ai-agent-';

export function createXmlParser(): { parseChunk: (chunk: string, nonce: string, allowedSlots: Set<string>, allowedTools: Set<string>) => ParsedXmlSlot[]; flush: () => { slots: ParsedXmlSlot[]; leftover: string } } {
  const state: ParserState = { buffer: '' };

  const parseBuffer = (nonce: string, allowedSlots: Set<string>, allowedTools: Set<string>): ParsedXmlSlot[] => {
    const results: ParsedXmlSlot[] = [];
    let idx = state.buffer.indexOf(OPEN_PREFIX);
    // eslint-disable-next-line functional/no-loop-statements
    while (idx !== -1) {
      const afterOpen = state.buffer.slice(idx + OPEN_PREFIX.length);
      if (!afterOpen.startsWith(nonce + '-')) {
        // skip this occurrence
        state.buffer = state.buffer.slice(idx + OPEN_PREFIX.length);
        idx = state.buffer.indexOf(OPEN_PREFIX);
        continue;
      }
      const slotIdMatch = /^([A-Za-z0-9\-]+)\b[^>]*>/.exec(afterOpen);
      if (slotIdMatch === null) break; // incomplete
      const slotId = slotIdMatch[1];
      const openTagEnd = idx + OPEN_PREFIX.length + slotIdMatch[0].length;
      const closeTag = `</ai-agent-${slotId}>`;
      const closeIdx = state.buffer.indexOf(closeTag, openTagEnd);
      if (closeIdx === -1) break; // incomplete
      const openTag = state.buffer.slice(idx, openTagEnd);
      const toolMatch = /tool="([^"]+)"/.exec(openTag);
      // For FINAL slot, infer tool as agent__final_report if not specified
      const tool = toolMatch?.[1] ?? (slotId.endsWith('-FINAL') ? 'agent__final_report' : '');
      // Extract status and format attributes from opening tag
      const statusMatch = /status="([^"]+)"/.exec(openTag);
      const formatMatch = /format="([^"]+)"/.exec(openTag);
      const statusAttr = statusMatch?.[1];
      const formatAttr = formatMatch?.[1];
      const content = state.buffer.slice(openTagEnd, closeIdx);
      state.buffer = state.buffer.slice(closeIdx + closeTag.length);
      idx = state.buffer.indexOf(OPEN_PREFIX);
      if (!allowedSlots.has(slotId)) continue;
      if (!allowedTools.has(tool)) continue;
      if (content.trim().length === 0) continue;
      results.push({ slotId, tool, rawPayload: content, statusAttr, formatAttr });
    }
    return results;
  };

  return {
    parseChunk: (chunk: string, nonce: string, allowedSlots: Set<string>, allowedTools: Set<string>): ParsedXmlSlot[] => {
      state.buffer += chunk;
      return parseBuffer(nonce, allowedSlots, allowedTools);
    },
    flush: (): { slots: ParsedXmlSlot[]; leftover: string } => {
      // Return nothing parsed, but surface leftover so caller can log/feedback.
      const leftover = state.buffer;
      state.buffer = '';
      return { slots: [], leftover };
    },
  };
}

export function buildToolCallFromParsed(slot: ParsedXmlSlot, toolCallId: string): ToolCall {
  // Payload JSON parsing happens at orchestrator layer; here we just map basics
  return {
    id: toolCallId,
    name: slot.tool,
    // parameters are kept as raw string; orchestrator will parse/validate against schema
    parameters: slot.rawPayload as unknown as Record<string, unknown>,
  };
}

export function renderXmlNext(payload: XmlNextPayload): string {
  return renderXmlNextTemplate(payload);
}

export function renderXmlPast(past: XmlPastPayload): string {
  const truncated = {
    entries: past.entries.map((entry) => ({
      slotId: entry.slotId,
      tool: entry.tool,
      status: entry.status,
      durationText: entry.durationMs !== undefined ? ` duration="${(entry.durationMs / 1000).toFixed(2)}s"` : undefined,
      request: truncateUtf8WithNotice(entry.request, 4096),
      response: truncateUtf8WithNotice(entry.response, 4096),
    })),
  };
  return renderXmlPastTemplate(truncated);
}
