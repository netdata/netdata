/**
 * Task status test suite.
 * Tests for agent__task_status tool behavior and progress tracking.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  LOG_FAILURE_REPORT,
  runWithExecuteTurnOverride,
  TASK_ANALYSIS_COMPLETE,
  TASK_COMPLETE_TASK,
  TASK_COMPLETED_RESPONSE,
  TASK_CONTINUE_PROCESSING,
  TASK_STATUS_COMPLETED,
  TASK_STATUS_IN_PROGRESS,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const TASK_STATUS_TOOL = 'agent__task_status';
const STATUS_CALL_ID = 'status-call';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';

/**
 * Task status tests covering:
 * - Progress-only turns with task_status
 * - Task completion without final report
 * - Progress tracking across multiple turns
 * - Task status with other tools
 */
export const TASK_STATUS_TESTS: HarnessTest[] = [];

// Test: Task status advances session to next turn (suite version)
TASK_STATUS_TESTS.push({
  id: 'suite-task-status-advances-turn',
  description: 'Suite: Task status tool advances session to next turn',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const finalToolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: task_status only
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'reporting status',
          messages: [
            {
              role: 'assistant',
              content: 'reporting status',
              toolCalls: [{
                id: STATUS_CALL_ID,
                name: TASK_STATUS_TOOL,
                parameters: { status: TASK_STATUS_IN_PROGRESS, done: 'Step 1', pending: TASK_CONTINUE_PROCESSING, now: TASK_COMPLETE_TASK },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Turn 2: final report
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
              name: FINAL_REPORT_TOOL,
              parameters: { report_format: 'markdown', report_content: TASK_ANALYSIS_COMPLETE },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId: finalToolCallId,
            content: TASK_ANALYSIS_COMPLETE,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Task status should advance and complete successfully');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === TASK_ANALYSIS_COMPLETE, 'Final report content should match');
  },
} satisfies HarnessTest);

// Test: Progress-only turns exhaust maxTurns (suite version)
TASK_STATUS_TESTS.push({
  id: 'suite-task-status-exhausts-turns',
  description: 'Suite: Progress-only turns fail when maxTurns exhausted',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: false },
      latencyMs: 5,
      response: 'still processing',
      messages: [
        {
          role: 'assistant',
          content: 'still processing',
          toolCalls: [{
            id: STATUS_CALL_ID,
            name: TASK_STATUS_TOOL,
            parameters: { status: TASK_STATUS_IN_PROGRESS, done: 'Still working', pending: 'More steps', now: TASK_COMPLETE_TASK },
          }],
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      executionStats: { executedTools: 1, executedNonProgressBatchTools: 0, executedProgressBatchTools: 1, unknownToolEncountered: false },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'Progress-only turns should fail when maxTurns exhausted');
    invariant(result.finalReport !== undefined, 'Synthetic final report expected');
    const failureLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FAILURE_REPORT);
    invariant(failureLog !== undefined, 'Failure report log expected');
  },
} satisfies HarnessTest);

// Test: Task status completed without final report (suite version)
TASK_STATUS_TESTS.push({
  id: 'suite-task-status-completed-no-report',
  description: 'Suite: Task status with completed status but no final report fails',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: false },
      latencyMs: 5,
      response: TASK_COMPLETED_RESPONSE,
      messages: [
        {
          role: 'assistant',
          content: TASK_COMPLETED_RESPONSE,
          toolCalls: [{
            id: STATUS_CALL_ID,
            name: TASK_STATUS_TOOL,
            parameters: { status: TASK_STATUS_COMPLETED, done: 'All done', pending: 'None', now: 'Report' },
          }],
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      executionStats: { executedTools: 1, executedNonProgressBatchTools: 0, executedProgressBatchTools: 1, unknownToolEncountered: false },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'Task status completed without final report should fail');
    invariant(result.finalReport !== undefined, 'Synthetic final report expected');
    const failureLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FAILURE_REPORT);
    invariant(failureLog !== undefined, 'Failure report log expected');
  },
} satisfies HarnessTest);

// Test: Task status with real tool resets progress counter (suite version)
TASK_STATUS_TESTS.push({
  id: 'suite-task-status-with-real-tool',
  description: 'Suite: Task status combined with real tool allows session to continue',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const finalToolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: task_status + another tool
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'status with tool',
          messages: [
            {
              role: 'assistant',
              content: 'status with tool',
              toolCalls: [
                { id: STATUS_CALL_ID, name: TASK_STATUS_TOOL, parameters: { status: TASK_STATUS_IN_PROGRESS, done: 'Step 1', pending: 'Continue', now: 'Process' } },
                { id: 'tool-call', name: 'test__test', parameters: { text: 'test-result' } },
              ],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
          executionStats: { executedTools: 2, executedNonProgressBatchTools: 1, executedProgressBatchTools: 1, unknownToolEncountered: false },
        });
      }
      // Turn 2: final report
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
              name: FINAL_REPORT_TOOL,
              parameters: { report_format: 'markdown', report_content: TASK_ANALYSIS_COMPLETE },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId: finalToolCallId,
            content: TASK_ANALYSIS_COMPLETE,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Task status with real tool should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === TASK_ANALYSIS_COMPLETE, 'Final report content should match');
  },
} satisfies HarnessTest);
