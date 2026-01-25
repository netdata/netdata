/**
 * Router handoff test suite.
 * Tests for router tool behavior and agent handoff patterns.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  ROUTER_CHILD_REPORT_CONTENT,
  ROUTER_HANDOFF_TOOL,
  ROUTER_PARENT_REPORT_CONTENT,
  ROUTER_ROUTE_MESSAGE,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';

/**
 * Router handoff tests covering:
 * - Basic router tool invocation
 * - Router with invalid parameters
 * - Router chain completion
 * - Handoff event propagation
 */
export const ROUTER_HANDOFF_TESTS: HarnessTest[] = [];

// Test: Router tool returns to parent (suite version)
ROUTER_HANDOFF_TESTS.push({
  id: 'suite-router-returns-to-parent',
  description: 'Suite: Router tool completes and returns control to parent',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const finalToolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: Router handoff
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: ROUTER_ROUTE_MESSAGE,
          messages: [
            {
              role: 'assistant',
              content: ROUTER_ROUTE_MESSAGE,
              toolCalls: [{
                id: 'router-call',
                name: ROUTER_HANDOFF_TOOL,
                parameters: { agent: 'child-agent', message: 'Process this task' },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Turn 2: Parent final report after child completes
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
              parameters: { report_format: 'markdown', report_content: ROUTER_PARENT_REPORT_CONTENT },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId: finalToolCallId,
            content: ROUTER_PARENT_REPORT_CONTENT,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Router handoff should complete successfully');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === ROUTER_PARENT_REPORT_CONTENT, 'Parent final report content should match');
  },
} satisfies HarnessTest);

// Test: Router with missing target fails gracefully (suite version)
ROUTER_HANDOFF_TESTS.push({
  id: 'suite-router-missing-target-fails',
  description: 'Suite: Router with missing target parameter triggers retry',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 2;
    const finalToolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: Invalid router call (missing target)
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'invalid router call',
          messages: [
            {
              role: 'assistant',
              content: 'invalid router call',
              toolCalls: [{
                id: 'router-call',
                name: ROUTER_HANDOFF_TOOL,
                // Missing agent parameter
                parameters: { message: 'Process this task' },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Turn 2: Valid final report after retry
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
              parameters: { report_format: 'markdown', report_content: ROUTER_PARENT_REPORT_CONTENT },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId: finalToolCallId,
            content: ROUTER_PARENT_REPORT_CONTENT,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Router retry should eventually succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Direct final report without router (suite version)
ROUTER_HANDOFF_TESTS.push({
  id: 'suite-router-skip-direct-report',
  description: 'Suite: Session can complete with direct final report (no router)',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const finalToolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
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
            parameters: { report_format: 'markdown', report_content: ROUTER_CHILD_REPORT_CONTENT },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId: finalToolCallId,
          content: ROUTER_CHILD_REPORT_CONTENT,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Direct final report should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === ROUTER_CHILD_REPORT_CONTENT, 'Final report content should match');
  },
} satisfies HarnessTest);
