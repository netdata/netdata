import { describe, expect, it } from 'vitest';

import { renderReasoningMarkdown, type ReasoningTurnState } from './reasoning-markdown.js';

describe('renderReasoningMarkdown', () => {
  it('wraps thinking text with the required separators and keeps progress outside the block', () => {
    const turns: ReasoningTurnState[] = [
      {
        index: 1,
        summary: 'tokens →10 ←5',
        thinking: 'Let me think about OPAP\nand Adaptera.',
        updates: ['fireflies started', 'fireflies update Searching for OPAP meetings'],
      },
    ];
    const markdown = renderReasoningMarkdown({
      header: '## fireflies: txn-123',
      turns,
      summaryText: 'SUMMARY: fireflies, completed',
    });
    expect(markdown).toContain('\n\n---\nLet me think about OPAP');
    expect(markdown).toContain('Adaptera.\n---\n\n- fireflies started');
    expect(markdown.endsWith('\n')).toBe(true);
  });

  it('omits separators when no thinking text is present', () => {
    const turns: ReasoningTurnState[] = [
      { index: 1, updates: ['fireflies started'], thinking: undefined },
    ];
    const markdown = renderReasoningMarkdown({
      header: '## fireflies: txn-456',
      turns,
    });
    expect(markdown.includes('---')).toBe(false);
    expect(markdown).toContain('- fireflies started');
  });
});
