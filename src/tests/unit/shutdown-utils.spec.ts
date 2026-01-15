import { describe, expect, it, vi } from 'vitest';

import { handleHeadendShutdown } from '../../headends/shutdown-utils.js';

describe('handleHeadendShutdown', () => {
  it('marks stopRef and invokes close callback', () => {
    const stopRef = { stopping: false };
    const close = vi.fn();

    handleHeadendShutdown(stopRef, close);

    expect(stopRef.stopping).toBe(true);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('handles missing stopRef', () => {
    const close = vi.fn();

    handleHeadendShutdown(undefined, close);

    expect(close).toHaveBeenCalledTimes(1);
  });
});
