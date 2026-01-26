import crypto from 'node:crypto';

import { afterEach, describe, expect, it, vi } from 'vitest';

import type { ResolvedFinalReportPluginRequirement } from '../../plugins/types.js';
import type { LogDetailValue, MCPTool } from '../../types.js';

import { XmlToolTransport } from '../../xml-transport.js';

const MOCK_UUID = 'abcd1234-abcd-abcd-abcd-abcd12345678';
const baseTools: MCPTool[] = [
  { name: 'mock_tool', description: 'mock', inputSchema: { type: 'object' } },
  { name: 'agent__final_report', description: 'final', inputSchema: { type: 'object' } },
  { name: 'agent__task_status', description: 'task_status', inputSchema: { type: 'object' } },
];
const SAMPLE_PLUGIN_NAME = 'support-metadata';
const SAMPLE_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [
  {
    name: SAMPLE_PLUGIN_NAME,
    schema: {
      type: 'object',
      additionalProperties: false,
      properties: {
        ticketId: { type: 'string' },
      },
      required: ['ticketId'],
    },
    systemPromptInstructions: 'Provide support metadata JSON.',
    xmlNextSnippet: 'Support META must include ticketId.',
    finalReportExampleSnippet: '<meta example>',
  },
];

interface XmlWarningEntry {
  severity: 'WRN';
  message: string;
  details?: Record<string, LogDetailValue>;
}

describe('XmlToolTransport META handling', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('extracts META blocks out-of-band without XML parser errors', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();
    transport.setFinalReportPluginRequirements(SAMPLE_REQUIREMENTS);
    const build = transport.buildMessages({
      turn: 1,
      maxTurns: 3,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      taskStatusToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 5,
      finalReportLocked: false,
      missingMetaPluginNames: [],
    });

    const metaWrapper = `<ai-agent-${build.nonce}-META plugin="${SAMPLE_PLUGIN_NAME}">{"ticketId":"123"}</ai-agent-${build.nonce}-META>`;
    const recordFailure = vi.fn();
    const logCalls: XmlWarningEntry[] = [];
    const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

    const result = transport.parseAssistantMessage(
      metaWrapper,
      { turn: 1, resolvedFormat: 'markdown' },
      { recordTurnFailure: recordFailure, logWarning },
    );

    expect(result.toolCalls).toBeUndefined();
    expect(result.errors).toHaveLength(0);
    expect(result.metaIssues).toHaveLength(0);
    expect(result.metaBlocks).toHaveLength(1);
    expect(result.metaBlocks[0]).toEqual({
      plugin: SAMPLE_PLUGIN_NAME,
      content: '{"ticketId":"123"}',
    });
    expect(recordFailure).not.toHaveBeenCalled();
    expect(logCalls).toHaveLength(0);
  });

  it('reports malformed META wrappers as meta issues when plugins are configured', () => {
    vi.spyOn(crypto, 'randomUUID').mockReturnValue(MOCK_UUID);
    const transport = new XmlToolTransport();
    transport.setFinalReportPluginRequirements(SAMPLE_REQUIREMENTS);
    const build = transport.buildMessages({
      turn: 1,
      maxTurns: 3,
      tools: baseTools,
      maxToolCallsPerTurn: 1,
      taskStatusToolEnabled: false,
      finalReportToolName: 'agent__final_report',
      resolvedFormat: 'markdown',
      expectedJsonSchema: undefined,
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 5,
      finalReportLocked: false,
      missingMetaPluginNames: [],
    });

    const malformedMeta = `<ai-agent-${build.nonce}-META plugin="${SAMPLE_PLUGIN_NAME}">{"ticketId":"123"}`;
    const recordFailure = vi.fn();
    const logCalls: XmlWarningEntry[] = [];
    const logWarning = vi.fn((entry: XmlWarningEntry) => { logCalls.push(entry); });

    const result = transport.parseAssistantMessage(
      malformedMeta,
      { turn: 1, resolvedFormat: 'markdown' },
      { recordTurnFailure: recordFailure, logWarning },
    );

    expect(result.toolCalls).toBeUndefined();
    expect(result.errors).toHaveLength(0);
    expect(result.metaBlocks).toHaveLength(0);
    expect(result.metaIssues).toHaveLength(1);
    expect(result.metaIssues[0]?.slug).toBe('final_meta_invalid');
    expect(recordFailure).not.toHaveBeenCalled();
    expect(logWarning).toHaveBeenCalled();
    expect(logCalls[0]?.details?.warning).toBe('meta_malformed');
  });
});
