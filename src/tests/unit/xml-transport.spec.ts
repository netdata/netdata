import crypto from 'node:crypto';

import { afterEach, describe, expect, it, vi } from 'vitest';

import type { LogDetailValue, MCPTool } from '../../types.js';

import { XmlToolTransport } from '../../xml-transport.js';

const MOCK_UUID = 'abcd1234-abcd-abcd-abcd-abcd12345678';
const NONCE = 'abcd1234';
const SLACK_BLOCK_FORMAT = 'slack-block-kit' as const;

const baseTools: MCPTool[] = [
  { name: 'mock_tool', description: 'mock', inputSchema: { type: 'object' } },
  { name: 'agent__final_report', description: 'final', inputSchema: { type: 'object' } },
  { name: 'agent__task_status', description: 'task_status', inputSchema: { type: 'object' } },
];

interface XmlWarningEntry {
  severity: 'WRN';
  message: string;
  details?: Record<string, LogDetailValue>;
}

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
      taskStatusToolEnabled: true,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 10,
    });

    expect(first.nonce).toBe(NONCE);
    expect(first.slotTemplates).toHaveLength(1); // only final slot
    expect(first.allowedTools.has('mock_tool')).toBe(false);
    expect(first.allowedTools.has('agent__final_report')).toBe(true);
    expect(first.allowedTools.has('agent__task_status')).toBe(false);
    expect(first.pastMessage).toBeUndefined();

    transport.recordToolResult('mock_tool', { foo: 'bar' }, 'ok', 'resp', 7, first.slotTemplates[0].slotId);
    transport.beginTurn();

    const second = transport.buildMessages({
      turn: 3,
      maxTurns: 5,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      taskStatusToolEnabled: true,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 15,
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
      taskStatusToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: undefined,
      expectedJsonSchema: undefined,
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 5,
    });

    const slotId = build.slotTemplates[0].slotId;
    const recordFailure = vi.fn();
    const logCalls: XmlWarningEntry[] = [];
    const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

    const result = transport.parseAssistantMessage(
      `<ai-agent-${slotId} tool="mock_tool">{"x":1}</ai-agent-${slotId}>`,
      { turn: 1, resolvedFormat: undefined },
      { recordTurnFailure: recordFailure, logWarning }
    );

    expect(result.errors).toHaveLength(1);
    expect(result.toolCalls).toBeUndefined();
    expect(recordFailure).toHaveBeenCalled();
    expect(logWarning).toHaveBeenCalled();
    expect(logCalls[0]?.details?.warning).toBeDefined();
    expect(logCalls[0]?.details?.action).toBeDefined();
  });

  it('records final-report JSON errors in xml-final mode', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();
    const build = transport.buildMessages({
      turn: 1,
      maxTurns: 2,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      taskStatusToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'json',
      expectedJsonSchema: { type: 'object' },
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 8,
    });

    const finalSlot = build.slotTemplates.find((s) => s.slotId.endsWith('FINAL'))?.slotId ?? `${NONCE}-FINAL`;
    const recordFailure = vi.fn();
    const logWarning = vi.fn();

    const parse = transport.parseAssistantMessage(
      `<ai-agent-${finalSlot} format="json">not-json</ai-agent-${finalSlot}>`,
      { turn: 1, resolvedFormat: 'json' },
      { recordTurnFailure: recordFailure, logWarning }
    );

    expect(parse.toolCalls).toHaveLength(1);
    expect(parse.toolCalls?.[0].name).toBe('agent__final_report');
    expect(parse.errors).toHaveLength(0);
    expect(recordFailure).not.toHaveBeenCalled();
    expect(logWarning).not.toHaveBeenCalled();
  });

  describe('xml mismatch failure paths', () => {
    it('records xml_malformed_mismatch for a non-final slot with a closing tag', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const nonFinalSlot = `${NONCE}-0001`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      const parse = transport.parseAssistantMessage(
        `<ai-agent-${nonFinalSlot} tool="agent__final_report">{"ok":true}</ai-agent-${nonFinalSlot}>`,
        { turn: 1, resolvedFormat: 'markdown' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors.length).toBeGreaterThan(0);
      expect(parse.errors[0]?.slotId).toBe(nonFinalSlot);
      expect(recordFailure).toHaveBeenCalled();
      expect(recordFailure.mock.calls[0]?.[0]).toBe('xml_malformed_mismatch');
      expect(recordFailure.mock.calls[0]?.[1]).toContain(nonFinalSlot);
      expect(logWarning).toHaveBeenCalled();
    });

    it('records xml_slot_mismatch when the final slot open tag is malformed but a closing tag is present', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      const build = transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const slotId = build.slotTemplates[0].slotId;
      const recordFailure = vi.fn<(slug: string, reason?: string) => void>();
      const logCalls: XmlWarningEntry[] = [];
      const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

      const malformedOpenTag =
        `<ai-agent-${slotId} tool="agent__final_report" {"ok":true}</ai-agent-${slotId}>`;
      const parse = transport.parseAssistantMessage(
        malformedOpenTag,
        { turn: 1, resolvedFormat: 'markdown' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors.length).toBeGreaterThan(0);
      const failureSlugs = recordFailure.mock.calls.map((call) => call[0]);
      expect(failureSlugs).toContain('xml_slot_mismatch');

      const warningSlugs = logCalls
        .map((entry) => entry.details?.slug)
        .filter((slug): slug is string => typeof slug === 'string');
      expect(warningSlugs).toContain('xml_slot_mismatch');
    });

    it('records xml_missing_closing_tag for a non-final slot without a closing tag', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const nonFinalSlot = `${NONCE}-0001`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      const parse = transport.parseAssistantMessage(
        `<ai-agent-${nonFinalSlot} tool="agent__final_report">{"ok":true}`,
        { turn: 1, resolvedFormat: 'markdown', stopReason: 'tool-calls' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors.length).toBeGreaterThan(0);
      expect(parse.errors[0]?.slotId).toBe(nonFinalSlot);
      expect(recordFailure).toHaveBeenCalled();
      expect(recordFailure.mock.calls[0]?.[0]).toBe('xml_missing_closing_tag');
      expect(recordFailure.mock.calls[0]?.[1]).toContain(nonFinalSlot);
      expect(logWarning).toHaveBeenCalled();
    });
  });

  describe('truncation handling (stopReason=length)', () => {
    it('rejects truncated structured output (json) and calls recordTurnFailure', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'json',
        expectedJsonSchema: { type: 'object' },
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logCalls: XmlWarningEntry[] = [];
      const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

      // Unclosed tag with stopReason=length and structured format
      const parse = transport.parseAssistantMessage(
        `<ai-agent-${finalSlot} format="json">{"incomplete": "json`,
        { turn: 1, resolvedFormat: 'json', stopReason: 'length', maxOutputTokens: 8192 },
        { recordTurnFailure: recordFailure, logWarning }
      );

      // Should reject - no tool calls returned
      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors).toHaveLength(0);
      // Should call recordTurnFailure with truncation message
      expect(recordFailure).toHaveBeenCalledTimes(1);
      expect(recordFailure.mock.calls[0][0]).toBe('xml_structured_output_truncated');
      expect(recordFailure.mock.calls[0][1]).toContain('8192 tokens');
      // Should log with content dump
      expect(logWarning).toHaveBeenCalledTimes(1);
      expect(logCalls[0].severity).toBe('WRN');
      expect(logCalls[0].details?.warning).toBe('final_report_truncated');
      expect(logCalls[0].details?.action).toBe('retry');
      expect(logCalls[0].details?.stop_reason).toBe('length');
      expect(logCalls[0].details?.format).toBe('json');
      expect(logCalls[0].details?.truncated).toBe(true);
    });

    it('rejects truncated structured output (slack-block-kit) and calls recordTurnFailure', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: SLACK_BLOCK_FORMAT,
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      // Unclosed tag with stopReason=length and structured format
      const parse = transport.parseAssistantMessage(
        `<ai-agent-${finalSlot} format="${SLACK_BLOCK_FORMAT}">[{"blocks": [`,
        { turn: 1, resolvedFormat: SLACK_BLOCK_FORMAT, stopReason: 'max_tokens', maxOutputTokens: 4096 },
        { recordTurnFailure: recordFailure, logWarning }
      );

      // Should reject - no tool calls returned
      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors).toHaveLength(0);
      // Should call recordTurnFailure
      expect(recordFailure).toHaveBeenCalledTimes(1);
      expect(recordFailure.mock.calls[0][0]).toBe('xml_structured_output_truncated');
      expect(recordFailure.mock.calls[0][1]).toContain('4096 tokens');
      // Should log
      expect(logWarning).toHaveBeenCalledTimes(1);
      const warningEntry = logWarning.mock.calls[0]?.[0] as XmlWarningEntry | undefined;
      expect(warningEntry?.details?.warning).toBe('final_report_truncated');
      expect(warningEntry?.details?.action).toBe('retry');
      expect(warningEntry?.details?.stop_reason).toBe('max_tokens');
      expect(warningEntry?.details?.format).toBe(SLACK_BLOCK_FORMAT);
      expect(warningEntry?.details?.truncated).toBe(true);
    });

    it('accepts truncated unstructured output (markdown) with truncated flag', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logCalls: XmlWarningEntry[] = [];
      const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

      const truncatedContent = '# Report\n\nThis is a truncated markdown report that got cut off mid-senten';

      // Unclosed tag with stopReason=length but unstructured format
      const parse = transport.parseAssistantMessage(
        `<ai-agent-${finalSlot} format="markdown">${truncatedContent}`,
        { turn: 1, resolvedFormat: 'markdown', stopReason: 'length', maxOutputTokens: 8192 },
        { recordTurnFailure: recordFailure, logWarning }
      );

      // Should accept - tool call returned with truncated flag
      expect(parse.toolCalls).toHaveLength(1);
      expect(parse.toolCalls?.[0].name).toBe('agent__final_report');
      expect(parse.toolCalls?.[0].parameters).toHaveProperty('_truncated', true);
      expect(parse.toolCalls?.[0].parameters).toHaveProperty('report_content', truncatedContent);
      expect(parse.errors).toHaveLength(0);
      // Should NOT call recordTurnFailure (we're accepting it)
      expect(recordFailure).not.toHaveBeenCalled();
      // Should log acceptance
      expect(logWarning).toHaveBeenCalledTimes(1);
      expect(logCalls[0].severity).toBe('WRN');
      expect(logCalls[0].details?.warning).toBe('final_report_truncated');
      expect(logCalls[0].details?.action).toBe('accept');
      expect(logCalls[0].details?.stop_reason).toBe('length');
      expect(logCalls[0].details?.format).toBe('markdown');
      expect(logCalls[0].details?.truncated).toBe(true);
    });

    it('accepts truncated unstructured output (tty) with truncated flag', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'tty',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      const parse = transport.parseAssistantMessage(
        `<ai-agent-${finalSlot} format="tty">Some tty output that was trunca`,
        { turn: 1, resolvedFormat: 'tty', stopReason: 'length' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      // Should accept
      expect(parse.toolCalls).toHaveLength(1);
      expect(parse.toolCalls?.[0].parameters).toHaveProperty('_truncated', true);
      expect(recordFailure).not.toHaveBeenCalled();
      expect(logWarning).toHaveBeenCalledTimes(1);
      const warningEntry = logWarning.mock.calls[0]?.[0] as XmlWarningEntry | undefined;
      expect(warningEntry?.details?.warning).toBe('final_report_truncated');
      expect(warningEntry?.details?.action).toBe('accept');
      expect(warningEntry?.details?.stop_reason).toBe('length');
      expect(warningEntry?.details?.format).toBe('tty');
      expect(warningEntry?.details?.truncated).toBe(true);
    });

    it('accepts normal completion (stopReason=stop) without truncated flag', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      // Unclosed tag with stopReason=stop (normal completion)
      const parse = transport.parseAssistantMessage(
        `<ai-agent-${finalSlot} format="markdown"># Complete Report\n\nThis is complete.`,
        { turn: 1, resolvedFormat: 'markdown', stopReason: 'stop' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      // Should accept without truncated flag
      expect(parse.toolCalls).toHaveLength(1);
      expect(parse.toolCalls?.[0].parameters).not.toHaveProperty('_truncated');
      expect(recordFailure).not.toHaveBeenCalled();
      expect(logWarning).not.toHaveBeenCalled();
    });

    it('ignores <think> block when extracting unclosed final report', () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
      const transport = new XmlToolTransport();
      transport.buildMessages({
        turn: 1,
        maxTurns: 2,
        tools: baseTools,
        maxToolCallsPerTurn: 1,
        taskStatusToolEnabled: false,
        finalReportToolName: 'agent__final_report',
        resolvedFormat: 'markdown',
        expectedJsonSchema: undefined,
        attempt: 1,
        maxRetries: 3,
        contextPercentUsed: 8,
      });

      const finalSlot = `${NONCE}-FINAL`;
      const recordFailure = vi.fn();
      const logWarning = vi.fn();

      const parse = transport.parseAssistantMessage(
        `  <think>Example <ai-agent-${finalSlot} format="markdown">not real</think>`,
        { turn: 1, resolvedFormat: 'markdown', stopReason: 'tool-calls' },
        { recordTurnFailure: recordFailure, logWarning }
      );

      expect(parse.toolCalls).toBeUndefined();
      expect(parse.errors).toHaveLength(0);
      expect(recordFailure).not.toHaveBeenCalled();
    });
  });
});
