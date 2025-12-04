import crypto from 'node:crypto';

import { afterEach, describe, expect, it, vi } from 'vitest';

import type { MCPTool } from '../../types.js';

import { XmlToolTransport } from '../../xml-transport.js';

const MOCK_UUID = 'abcd1234-abcd-abcd-abcd-abcd12345678';
const NONCE = 'abcd1234';

const baseTools: MCPTool[] = [
  { name: 'mock_tool', description: 'mock', inputSchema: { type: 'object' } },
  { name: 'agent__final_report', description: 'final', inputSchema: { type: 'object' } },
  { name: 'agent__progress_report', description: 'progress', inputSchema: { type: 'object' } },
];

describe('XmlToolTransport', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('builds XML messages and carries past entries across turns', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();

    const first = transport.buildMessages({
      turn: 2,
      maxTurns: 5,
      tools: baseTools,
      maxToolCallsPerTurn: 2,
      progressToolEnabled: true,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
    });

    expect(first.nonce).toBe(NONCE);
    expect(first.slotTemplates).toHaveLength(1); // only final slot
    expect(first.allowedTools.has('mock_tool')).toBe(false);
    expect(first.allowedTools.has('agent__final_report')).toBe(true);
    expect(first.allowedTools.has('agent__progress_report')).toBe(false);
    expect(first.pastMessage).toBeUndefined();

    transport.recordToolResult('mock_tool', { foo: 'bar' }, 'ok', 'resp', 7, first.slotTemplates[0].slotId);
    transport.beginTurn();

    const second = transport.buildMessages({
      turn: 3,
      maxTurns: 5,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      progressToolEnabled: true,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
    });

    expect(second.pastMessage).toBeUndefined();
    expect(second.nextMessage.noticeType).toBe('xml-next');
  });

  it('parses assistant XML into tool calls', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();
    const build = transport.buildMessages({
      turn: 1,
      maxTurns: 3,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      progressToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: undefined,
      expectedJsonSchema: undefined,
    });

    const slotId = build.slotTemplates[0].slotId;
    const onFailure = vi.fn();
    const onLog = vi.fn();

    const result = transport.parseAssistantMessage(
      `<ai-agent-${slotId} tool="mock_tool">{"x":1}</ai-agent-${slotId}>`,
      { turn: 1, resolvedFormat: undefined },
      { onTurnFailure: onFailure, onLog }
    );

    expect(result.errors).toHaveLength(1);
    expect(result.toolCalls).toBeUndefined();
    expect(onFailure).toHaveBeenCalled();
    expect(onLog).toHaveBeenCalled();
  });

  it('records final-report JSON errors in xml-final mode', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();
    const build = transport.buildMessages({
      turn: 1,
      maxTurns: 2,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      progressToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'json',
      expectedJsonSchema: { type: 'object' },
    });

    const finalSlot = build.slotTemplates.find((s) => s.slotId.endsWith('FINAL'))?.slotId ?? `${NONCE}-FINAL`;
    const onFailure = vi.fn();
    const onLog = vi.fn();

    const parse = transport.parseAssistantMessage(
      `<ai-agent-${finalSlot} tool="agent__final_report">not-json</ai-agent-${finalSlot}>`,
      { turn: 1, resolvedFormat: 'json' },
      { onTurnFailure: onFailure, onLog }
    );

    expect(parse.toolCalls).toHaveLength(1);
    expect(parse.toolCalls?.[0].name).toBe('agent__final_report');
    expect(parse.errors).toHaveLength(0);
    expect(onFailure).not.toHaveBeenCalled();
    expect(onLog).not.toHaveBeenCalled();
  });
});
