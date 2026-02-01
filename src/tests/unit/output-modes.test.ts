import { describe, expect, it } from 'vitest';

import type { AIAgentSessionConfig, Configuration, TurnResult } from '../../types.js';

import { extractNonceFromMessages } from '../phase2-harness-scenarios/infrastructure/harness-helpers.js';
import { runWithExecuteTurnOverride } from '../phase2-harness-scenarios/infrastructure/harness-runner.js';

const BASE_CONFIG: Configuration = {
  providers: {
    test: { type: 'test-llm' },
  },
  mcpServers: {},
  queues: { default: { concurrent: 1 } },
};

const buildSessionConfig = (overrides?: Partial<AIAgentSessionConfig>): AIAgentSessionConfig => ({
  config: BASE_CONFIG,
  targets: [{ provider: 'test', model: 'deterministic-model' }],
  tools: [],
  systemPrompt: 'unit-test-system',
  userPrompt: 'unit-test-user',
  outputFormat: 'markdown',
  maxTurns: 1,
  maxRetries: 1,
  stream: false,
  ...overrides,
});

const buildTurnResult = (content: string, stopReason: string): TurnResult => ({
  status: { type: 'success', hasToolCalls: false, finalAnswer: true },
  response: content,
  stopReason,
  latencyMs: 5,
  messages: [{ role: 'assistant', content }],
});

const CHAT_OUTPUT = 'hello from chat';
const PARTIAL_OUTPUT = 'partial output';
const FINAL_OUTPUT = 'final output';
const RETRY_OUTPUT = 'final after retry';

describe('output modes', () => {
  it('chat mode accepts stop=stop with output', async () => {
    const sessionConfig = buildSessionConfig({ outputMode: 'chat', maxRetries: 1 });
    const result = await runWithExecuteTurnOverride(sessionConfig, () => (
      Promise.resolve(buildTurnResult(CHAT_OUTPUT, 'stop'))
    ));
    expect(result.success).toBe(true);
    expect(result.finalReport?.content).toBe(CHAT_OUTPUT);
  });

  it('chat mode retries stop=stop with empty output', async () => {
    let attempts = 0;
    const sessionConfig = buildSessionConfig({ outputMode: 'chat', maxRetries: 2 });
    const result = await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      attempts = invocation;
      if (invocation === 1) {
        return Promise.resolve(buildTurnResult(' \n', 'stop'));
      }
      return Promise.resolve(buildTurnResult(RETRY_OUTPUT, 'stop'));
    });
    expect(attempts).toBe(2);
    expect(result.finalReport?.content).toBe(RETRY_OUTPUT);
  });

  it('chat mode continues on non-stop reason and succeeds on stop', async () => {
    let attempts = 0;
    const sessionConfig = buildSessionConfig({ outputMode: 'chat', maxRetries: 2 });
    const result = await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      attempts = invocation;
      if (invocation === 1) {
        return Promise.resolve(buildTurnResult(PARTIAL_OUTPUT, 'length'));
      }
      return Promise.resolve(buildTurnResult(FINAL_OUTPUT, 'stop'));
    });
    expect(attempts).toBe(2);
    expect(result.finalReport?.content).toBe(FINAL_OUTPUT);
  });

  it('chat mode retries when stop reason is undefined, then succeeds on stop', async () => {
    let attempts = 0;
    const sessionConfig = buildSessionConfig({ outputMode: 'chat', maxRetries: 2 });
    const result = await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      attempts = invocation;
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          response: PARTIAL_OUTPUT,
          latencyMs: 5,
          messages: [{ role: 'assistant', content: PARTIAL_OUTPUT }],
        });
      }
      return Promise.resolve(buildTurnResult(FINAL_OUTPUT, 'stop'));
    });
    expect(attempts).toBe(2);
    expect(result.finalReport?.content).toBe(FINAL_OUTPUT);
  });

  it('agentic mode does not accept stop=stop without tools or final report', async () => {
    const sessionConfig = buildSessionConfig({ outputMode: 'agentic', maxRetries: 0 });
    const result = await runWithExecuteTurnOverride(sessionConfig, () => (
      Promise.resolve(buildTurnResult('plain text', 'stop'))
    ));
    expect(result.success).toBe(false);
  });

  it('chat final report equals streamed output across turns and excludes META/reasoning', async () => {
    const outputEvents: Array<{ text: string; source?: string }> = [];
    const thinkingEvents: string[] = [];

    const sessionConfig = buildSessionConfig({
      outputMode: 'chat',
      maxTurns: 2,
      maxRetries: 0,
      stream: true,
      callbacks: {
        onEvent: (event, meta) => {
          if (event.type === 'output') {
            outputEvents.push({ text: event.text, source: meta.source });
          }
          if (event.type === 'thinking') {
            thinkingEvents.push(event.text);
          }
        },
      },
    });

    const scenarioId = 'unit-chat-final-report-stream';
    const turn1Chunk = 'Hello ';
    const turn2Chunk = 'world';
    const reasoningText = 'private reasoning';

    const result = await runWithExecuteTurnOverride(sessionConfig, ({ request }) => {
      const turn = request.turnMetadata?.turn ?? 1;

      if (turn === 1) {
        request.onChunk?.(turn1Chunk, 'content');
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          response: '',
          stopReason: 'stop',
          latencyMs: 5,
          executionStats: {
            executedTools: 1,
            executedNonProgressBatchTools: 0,
            executedProgressBatchTools: 1,
            unknownToolEncountered: false,
          },
          messages: [
            {
              role: 'assistant',
              content: '',
            },
          ],
        });
      }

      const nonce = extractNonceFromMessages(request.messages, scenarioId);
      const metaWrapper = `<ai-agent-${nonce}-META plugin="support-metadata">{"ticket":"T-1"}</ai-agent-${nonce}-META>`;
      const assistantContent = `<think>${reasoningText}</think>${turn2Chunk}${metaWrapper}`;

      request.onChunk?.(turn2Chunk, 'content');
      request.onChunk?.(reasoningText, 'thinking');

      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        response: assistantContent,
        stopReason: 'stop',
        latencyMs: 5,
        messages: [
          {
            role: 'assistant',
            content: assistantContent,
          },
        ],
      });
    });

    const streamedOutput = outputEvents
      .filter((entry) => entry.source === 'stream')
      .map((entry) => entry.text)
      .join('');

    expect(streamedOutput).toBe(`${turn1Chunk}${turn2Chunk}`);
    expect(result.finalReport?.content).toBe(streamedOutput);
    expect(result.finalReport?.content ?? '').not.toContain('META');
    expect(result.finalReport?.content ?? '').not.toContain('<ai-agent-');
    expect(result.finalReport?.content ?? '').not.toContain(reasoningText);
    expect(result.finalReport?.content ?? '').not.toContain('<think>');

    const finalizeOutputs = outputEvents.filter((entry) => entry.source === 'finalize');
    expect(finalizeOutputs.length).toBe(0);

    expect(thinkingEvents.join('')).toContain(reasoningText);
  });
});
