import { describe, expect, it } from 'vitest';

import type { AccountingEntry } from '../../types.js';

import { collectLlmUsage } from '../../headends/completions-usage.js';

describe('collectLlmUsage', () => {
  it('sums LLM token usage and ignores non-LLM entries', () => {
    const entries: AccountingEntry[] = [
      {
        type: 'llm',
        timestamp: 1,
        status: 'ok',
        latency: 10,
        provider: 'openai',
        model: 'gpt-test',
        tokens: {
          inputTokens: 5,
          outputTokens: 7,
          totalTokens: 12,
        },
      },
      {
        type: 'tool',
        timestamp: 2,
        status: 'ok',
        latency: 5,
        mcpServer: 'mcp',
        command: 'noop',
        charactersIn: 0,
        charactersOut: 0,
      },
      {
        type: 'llm',
        timestamp: 3,
        status: 'failed',
        latency: 7,
        provider: 'openai',
        model: 'gpt-test',
        tokens: {
          inputTokens: 2,
          outputTokens: 3,
          totalTokens: 5,
        },
        error: 'boom',
      },
    ];

    expect(collectLlmUsage(entries)).toEqual({ input: 7, output: 10, total: 17 });
  });
});
