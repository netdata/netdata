import { describe, expect, it } from 'vitest';

import { ThinkTagStreamFilter } from '../../think-tag-filter.js';

describe('ThinkTagStreamFilter', () => {
  it('splits a leading <think> block from content', () => {
    const filter = new ThinkTagStreamFilter();
    const input = '<think>Planning note</think>Final output';
    const split = filter.process(input);

    expect(split.thinking).toBe('Planning note');
    expect(split.content).toBe('Final output');
  });

  it('passes through content when <think> is not leading', () => {
    const filter = new ThinkTagStreamFilter();
    const input = 'Hello <think>not leading</think> world';
    const split = filter.process(input);

    expect(split.thinking).toBe('');
    expect(split.content).toBe(input);
  });

  it('handles think blocks split across chunks', () => {
    const filter = new ThinkTagStreamFilter();
    const first = filter.process('<think>abc');
    const second = filter.process('def</think>tail');

    expect(first.thinking).toBe('');
    expect(first.content).toBe('');
    expect(second.thinking).toBe('abcdef');
    expect(second.content).toBe('tail');
  });
});
