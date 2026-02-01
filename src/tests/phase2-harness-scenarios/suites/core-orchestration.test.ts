/**
 * Core orchestration test suite.
 * Tests for turn loops, retry loops, provider cycling, and session lifecycle.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  expectTurnFailureContains,
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

/**
 * Core orchestration tests covering:
 * - Turn loop control (maxTurns, turn exhaustion)
 * - Retry loop control (maxRetries, retry exhaustion)
 * - Provider cycling and fallback
 * - Session lifecycle (start, cancel, shutdown)
 */
export const CORE_ORCHESTRATION_TESTS: HarnessTest[] = [];

// Test: Retry exhaustion with no tools executed (suite version)
CORE_ORCHESTRATION_TESTS.push({
  id: 'suite-core-retry-exhaustion-no-tools',
  description: 'Suite: Session fails when retries exhausted without tools',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: false, finalAnswer: false },
      latencyMs: 5,
      response: 'plain text without tools',
      messages: [
        {
          role: 'assistant',
          content: 'plain text without tools',
        },
      ],
      tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'retry exhaustion with no tools should fail');
    expectTurnFailureContains(result.logs, 'suite-core-retry-exhaustion-no-tools', ['no_tools', 'final_report_missing', 'retries_exhausted']);
  },
} satisfies HarnessTest);

// Test: stopReason=stop still fails without tools/final report (agentic baseline)
CORE_ORCHESTRATION_TESTS.push({
  id: 'suite-core-stop-reason-stop-no-tools',
  description: 'Suite: stopReason=stop without tools still fails in agentic mode',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: false, finalAnswer: false },
      latencyMs: 5,
      response: 'plain text with stop reason',
      messages: [
        {
          role: 'assistant',
          content: 'plain text with stop reason',
        },
      ],
      tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
      stopReason: 'stop',
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'stopReason=stop without tools should still fail in agentic mode');
    expectTurnFailureContains(result.logs, 'suite-core-stop-reason-stop-no-tools', ['no_tools', 'final_report_missing', 'retries_exhausted']);
  },
} satisfies HarnessTest);

// Test: Max turn exhaustion fails session (suite version)
CORE_ORCHESTRATION_TESTS.push({
  id: 'suite-core-max-turn-exhaustion-fails',
  description: 'Suite: Session fails when max turns exhausted without final report',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: false, finalAnswer: false },
      latencyMs: 5,
      response: 'still no final report',
      messages: [
        { role: 'assistant', content: 'still no final report' },
      ],
      tokens: { inputTokens: 5, outputTokens: 2, totalTokens: 7 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'max turn exhaustion should fail session');
    expectTurnFailureContains(result.logs, 'suite-core-max-turn-exhaustion-fails', ['no_tools', 'final_report_missing', 'retries_exhausted']);
  },
} satisfies HarnessTest);

// Test: Empty response on final turn (suite version)
CORE_ORCHESTRATION_TESTS.push({
  id: 'suite-core-empty-response-final-turn',
  description: 'Suite: Session handles empty response on final turn gracefully',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: false, finalAnswer: false },
      latencyMs: 5,
      response: '',
      messages: [
        { role: 'assistant', content: '' },
      ],
      tokens: { inputTokens: 3, outputTokens: 0, totalTokens: 3 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'empty response on final turn should fail');
    expectTurnFailureContains(result.logs, 'suite-core-empty-response-final-turn', ['empty_response', 'retries_exhausted']);
  },
} satisfies HarnessTest);
