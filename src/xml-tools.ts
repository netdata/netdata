import type { ToolCall } from './types.js';

import { describeFormatParameter, getFormatSchema, type OutputFormatId } from './formats.js';
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
  finalReportSlot: { slotId: string };
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
  const { nonce, turn, maxTurns, tools, slotTemplates, progressSlot, finalReportSlot, mode, expectedFinalFormat, finalSchema } = payload;
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
      lines.push('Update the user about your current status and plan:');
      lines.push('');
      lines.push('```');
      lines.push(`<ai-agent-${progressSlot.slotId} tool="agent__progress_report">`);
      lines.push(`Found X, now searching for Y`);
      lines.push(`</ai-agent-${progressSlot.slotId}>`);
      lines.push('```');
      lines.push('');
    }
  }

  // Add format-specific instructions before the XML example
  const isTextFormat = expectedFinalFormat === 'text';
  const formatDescription = isTextFormat ? 'Plain text output.' : describeFormatParameter(expectedFinalFormat);
  // Get format schema: user-provided for json, built-in for other structured formats
  const formatSchema: Record<string, unknown> | undefined = (() => {
    if (expectedFinalFormat === 'json') return finalSchema;
    if (isTextFormat) return undefined;
    return getFormatSchema(expectedFinalFormat);
  })();

  lines.push('## Final Report');
  lines.push(`**Format: ${expectedFinalFormat}** — ${formatDescription}`);
  lines.push('');
  lines.push('Once your task is complete, provide your final report/answer using this output:');
  lines.push('');
  if (expectedFinalFormat === 'json' && finalSchema !== undefined) {
    // For JSON format: show the tool wrapper with user's schema nested in content_json
    const toolSchema = {
      type: 'object',
      required: ['status', 'report_format', 'content_json'],
      properties: {
        status: { type: 'string', enum: ['success', 'failure', 'partial'], description: 'Task completion status' },
        report_format: { type: 'string', const: 'json' },
        content_json: finalSchema,
      }
    };
    lines.push('Your response must be a JSON object matching this schema:');
    lines.push('```json');
    lines.push(JSON.stringify(toolSchema, null, 2));
    lines.push('```');
    lines.push('');
    lines.push('Wrap your JSON in these XML tags:');
  } else if (formatSchema !== undefined) {
    // For other structured formats (slack-block-kit): show the format schema
    lines.push('Your response must be a JSON object matching this schema:');
    lines.push('```json');
    lines.push(JSON.stringify(formatSchema, null, 2));
    lines.push('```');
    lines.push('');
    lines.push('Wrap your JSON in these XML tags:');
  }
  lines.push('```');
  lines.push(`<ai-agent-${finalReportSlot.slotId} tool="agent__final_report" status="success|failure|partial" format="${expectedFinalFormat}">`);
  if (expectedFinalFormat === 'json' || formatSchema !== undefined) {
    lines.push('{ ... your JSON here ... }');
  } else {
    lines.push(`[Your final report/answer here]`);
  }
  lines.push(`</ai-agent-${finalReportSlot.slotId}>`);
  lines.push('```');
  lines.push('');

  lines.push('**CRITICAL: Final Report Checklist**');
  lines.push('When ready to provide your final report, iterate through the following checklist:');
  lines.push('- [ ] The opening XML tag MUST be first in your response');
  lines.push('- [ ] The status XML attribute must be one of: success, failure, partial');
  lines.push('- [ ] Your report content/payload matches the expected format');
  lines.push('- [ ] Your output MUST end with the closing XML tag');
  lines.push('- [ ] Your entire report is between the opening and closing XML tags, not outside them');
  lines.push('The above checklist is mandatory for all final reports. Failure to comply with this checklist will result in rejection of your final report.');

  lines.push('## Mandatory Workflow');
  if (toolList.length > 0) {
    lines.push('You MUST now either:');
    lines.push('- Call one or more of your tools to collect data or perform actions together with the progress-report tool to update the user on your status');
    lines.push('- OR provide your final report');
  }
  else {
    lines.push('You MUST now provide your final report');
    lines.push('1. Include the onening tag `<ai-agent-*` with the correct slot ID and tool name');
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
