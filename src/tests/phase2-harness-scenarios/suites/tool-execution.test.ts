/**
 * Tool execution test suite.
 * Tests for MCP tool execution, unknown tools, and tool batching.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  expectTurnFailureContains,
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

/**
 * Tool execution tests covering:
 * - MCP tool invocation and results
 * - Unknown tool handling
 * - Tool batching and parallel execution
 * - Tool error handling
 */
export const TOOL_EXECUTION_TESTS: HarnessTest[] = [];

// Test: Unknown tool fails session (suite version)
TOOL_EXECUTION_TESTS.push({
  id: 'suite-tool-unknown-tool-fails',
  description: 'Suite: Session fails when model calls unknown tool',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: false },
      latencyMs: 5,
      response: 'Calling unknown tool',
      messages: [
        {
          role: 'assistant',
          content: 'Calling unknown tool',
          toolCalls: [{ id: 'call-1', name: 'unknown_tool', parameters: {} }],
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'Unknown tool should fail session');
    expectTurnFailureContains(result.logs, 'suite-tool-unknown-tool-fails', ['unknown_tool', 'retries_exhausted']);
  },
} satisfies HarnessTest);

// Test: Invalid tool parameters fails (suite version)
TOOL_EXECUTION_TESTS.push({
  id: 'suite-tool-invalid-schema-fails',
  description: 'Suite: Session fails when tool is called with invalid parameters',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: false },
      latencyMs: 5,
      response: 'Calling tool with invalid params',
      messages: [
        {
          role: 'assistant',
          content: 'Calling tool with invalid params',
          // eslint-disable-next-line @typescript-eslint/no-explicit-any, @typescript-eslint/no-unsafe-assignment -- intentionally testing invalid params
          toolCalls: [{ id: 'call-1', name: 'agent__task_status', parameters: null as any }],
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'Invalid tool params should fail session');
    expectTurnFailureContains(result.logs, 'suite-tool-invalid-schema-fails', ['retries_exhausted']);
  },
} satisfies HarnessTest);

// Test: Task status tool advances turn (suite version)
TOOL_EXECUTION_TESTS.push({
  id: 'suite-tool-task-status-advances-turn',
  description: 'Suite: Task status tool call advances to next turn',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const reportContent = 'Task completed successfully.';
    const finalToolCallId = 'final-report-call';
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: Return task_status to advance
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'Status update',
          messages: [
            {
              role: 'assistant',
              content: 'Status update',
              toolCalls: [{
                id: 'status-call',
                name: 'agent__task_status',
                parameters: { status: 'in-progress', done: 'Step 1', pending: 'Step 2', now: 'Working' },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Turn 2: Return final report
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        response: '',
        messages: [
          {
            role: 'assistant',
            content: '',
            toolCalls: [{
              id: finalToolCallId,
              name: 'agent__final_report',
              parameters: { report_format: 'markdown', report_content: reportContent },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId: finalToolCallId,
            content: reportContent,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Task status should advance and complete');
    invariant(result.finalReport !== undefined, 'Final report should be present');
    invariant(result.finalReport.content === 'Task completed successfully.', 'Final report content should match');
  },
} satisfies HarnessTest);
