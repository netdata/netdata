import { describe, expect, it } from 'vitest';

import { buildHeadendModelId } from '../../headends/model-id-utils.js';

describe('buildHeadendModelId', () => {
  it('picks the first non-empty source and strips .ai', () => {
    const seen = new Set<string>();
    const id = buildHeadendModelId([undefined, 'agent.ai', 'fallback'], seen, '-');
    expect(id).toBe('agent');
    expect(seen.has('agent')).toBe(true);
  });

  it('dedups with the provided separator', () => {
    const seen = new Set<string>(['agent']);
    const dashed = buildHeadendModelId(['agent'], seen, '-');
    expect(dashed).toBe('agent-2');

    const underscored = buildHeadendModelId(['agent'], seen, '_');
    expect(underscored).toBe('agent_2');
  });
});
