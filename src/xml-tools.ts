import type { OutputFormatId } from './formats.js';
import type { ToolCall } from './types.js';

import {
  PROGRESS_TOOL_INSTRUCTIONS_BRIEF,
  PROGRESS_TOOL_WORKFLOW_LINE,
} from './llm-messages.js';
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
  mode: 'xml' | 'xml-final';
  expectedFinalFormat: OutputFormatId | 'text';
  finalSchema?: Record<string, unknown>;
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
      const tool = toolMatch?.[1] ?? '';
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
  const { nonce, turn, maxTurns, tools, slotTemplates, progressSlot, mode } = payload;
  const toolList = tools.filter((t) => t.name !== 'agent__final_report' && t.name !== 'agent__progress_report');
  const lines: string[] = [];
  lines.push('# System Notice');
  lines.push('');
  lines.push(`This is turn No ${String(turn)}${maxTurns !== undefined ? ` of ${String(maxTurns)}` : ''}.`);
  lines.push('');

  if (mode === 'xml') {
      lines.push('## Available Tools for this Turn');
    if (toolList.length > 0) {
      const slotIds = slotTemplates
        .filter((s) => s.tools.some((t) => toolList.map((tt) => tt.name).includes(t)))
        .map((s) => s.slotId);
      lines.push('Replace XXXX with one of the available slot numbers and include your JSON body inside the tag.');
      lines.push(`Slots available: ${slotIds.join(', ')}`);
      lines.push('');
      toolList.forEach((t) => {
        const exampleSlot = `${nonce}-XXXX`;
        lines.push(`- tool \`${t.name}\`, schema:`);
        lines.push('```');
        lines.push(`<ai-agent-${exampleSlot} tool="${t.name}">`);
        lines.push(JSON.stringify(t.schema ?? {}, null, 2));
        lines.push(`</ai-agent-${exampleSlot}>`);
        lines.push('```');
        lines.push('');
      });
    } else {
      lines.push('No tools are available for this turn. You MUST provide your final report now.');
      lines.push('');
    }

    if (progressSlot !== undefined) {
      lines.push('## Progress Updates');
      lines.push(PROGRESS_TOOL_INSTRUCTIONS_BRIEF);
      lines.push('');
      lines.push('```');
      lines.push(`<ai-agent-${progressSlot.slotId} tool="agent__progress_report">`);
      lines.push(`Found X, now searching for Y`);
      lines.push(`</ai-agent-${progressSlot.slotId}>`);
      lines.push('```');
      lines.push('');
    }
  }

  lines.push('## Mandatory Workflow');
  if (toolList.length > 0) {
    lines.push('You MUST now either:');
    lines.push(`- ${PROGRESS_TOOL_WORKFLOW_LINE}`);
    lines.push('- OR provide your final report');
  }
  else {
    lines.push('You MUST now provide your final report');
    lines.push('1. Include the opening tag `<ai-agent-*` with the correct slot ID and tool name');
    lines.push('2. Include your final report/answer matching the expected format');
    lines.push('3. Include the closing tag `</ai-agent-*` with the correct slot ID');
  }

  return lines.join('\n');
}

export function renderXmlPast(past: XmlPastPayload): string {
  const lines: string[] = [];
  lines.push('# System Notice');
  lines.push('');
  lines.push('## Previous Turn Tool Responses');
  lines.push('');
  past.entries.forEach((entry) => {
    const duration = entry.durationMs !== undefined ? ` duration="${(entry.durationMs / 1000).toFixed(2)}s"` : '';
    lines.push(`<ai-agent-${entry.slotId} tool="${entry.tool}" status="${entry.status}"${duration}>`);
    lines.push('<request>');
    lines.push(truncateUtf8WithNotice(entry.request, 4096));
    lines.push('</request>');
    lines.push('<response>');
    lines.push(truncateUtf8WithNotice(entry.response, 4096));
    lines.push('</response>');
    lines.push(`</ai-agent-${entry.slotId}>`);
    lines.push('');
  });
  return lines.join('\n');
}
