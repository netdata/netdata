import { describe, expect, it, vi } from 'vitest';

import type { TurnRequest } from '../../types.js';

import { TestLLMProvider } from '../../llm-providers/test-llm.js';

const SYSTEM_PROMPT = 'Phase 1 deterministic harness: minimal instructions.';

describe('refusal stop_reason handling', () => {
  it('treats content-filter stop reason as invalid_response and discards output', async () => {
    const provider = new TestLLMProvider({ type: 'test-llm' });
    const toolExecutor = vi.fn().mockResolvedValue('ok');

    const request: TurnRequest = {
      messages: [
        { role: 'system', content: SYSTEM_PROMPT },
        { role: 'user', content: 'run-test-refusal' },
      ],
      provider: 'test-llm',
      model: 'deterministic-model',
      tools: [],
      toolExecutor,
      stream: false,
    };

    const result = await provider.executeTurn(request);

    expect(result.status.type).toBe('invalid_response');
    expect(result.stopReason).toBe('content-filter');
    expect(result.messages.length).toBe(0);
    expect(result.tokens).toBeDefined();
    expect(result.tokens?.inputTokens).toBeGreaterThan(0);
    expect(toolExecutor).not.toHaveBeenCalled();
  });
});
