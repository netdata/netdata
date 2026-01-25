/**
 * Format-specific test suite.
 * Tests for Slack block kit, JSON, and other output format handling.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';
const SLACK_BLOCK_KIT_FORMAT = 'slack-block-kit' as const;

/**
 * Format-specific tests covering:
 * - Slack block kit format (valid blocks structure)
 * - JSON format (valid object)
 * - Pipe format (plain text)
 * - Markdown format (with formatting elements)
 */
export const FORMAT_SPECIFIC_TESTS: HarnessTest[] = [];

// Test: Slack format with valid blocks (suite version)
FORMAT_SPECIFIC_TESTS.push({
  id: 'suite-format-slack-valid-blocks',
  description: 'Suite: Session accepts valid Slack block kit format',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    sessionConfig.outputFormat = SLACK_BLOCK_KIT_FORMAT;
    const toolCallId = FINAL_REPORT_CALL_ID;
    // Slack block kit format expects an array of messages, each with a blocks array
    const slackContent = JSON.stringify([{
      blocks: [
        { type: 'section', text: { type: 'mrkdwn', text: 'Test message' } },
      ],
    }]);
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: SLACK_BLOCK_KIT_FORMAT, report_content: slackContent },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: slackContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Valid Slack format should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.format === SLACK_BLOCK_KIT_FORMAT, 'Format should be slack-block-kit');
  },
} satisfies HarnessTest);

// Test: JSON format with valid object (suite version)
FORMAT_SPECIFIC_TESTS.push({
  id: 'suite-format-json-valid-object',
  description: 'Suite: Session accepts valid JSON format object',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    sessionConfig.outputFormat = 'json';
    const toolCallId = FINAL_REPORT_CALL_ID;
    const jsonContent = JSON.stringify({ status: 'complete', data: { key: 'value' } });
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'json', report_content: jsonContent },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: jsonContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Valid JSON format should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.format === 'json', 'Format should be json');
  },
} satisfies HarnessTest);

// Test: Pipe format with plain string (suite version)
// Note: 'text' is not a valid OutputFormatId; use 'pipe' for plain text output
FORMAT_SPECIFIC_TESTS.push({
  id: 'suite-format-pipe-plain',
  description: 'Suite: Session accepts pipe format for plain text',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    sessionConfig.outputFormat = 'pipe';
    const toolCallId = FINAL_REPORT_CALL_ID;
    const pipeContent = 'This is a plain text report.';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'pipe', report_content: pipeContent },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: pipeContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Pipe format should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.format === 'pipe', 'Format should be pipe');
    invariant(result.finalReport.content === 'This is a plain text report.', 'Pipe content should match');
  },
} satisfies HarnessTest);

// Test: Markdown format with formatting (suite version)
FORMAT_SPECIFIC_TESTS.push({
  id: 'suite-format-markdown-formatting',
  description: 'Suite: Session accepts markdown with formatting elements',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    sessionConfig.outputFormat = 'markdown';
    const toolCallId = FINAL_REPORT_CALL_ID;
    const mdContent = '# Title\n\n**Bold** and *italic* text.\n\n- List item 1\n- List item 2';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: mdContent },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: mdContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Markdown format should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.format === 'markdown', 'Format should be markdown');
  },
} satisfies HarnessTest);
