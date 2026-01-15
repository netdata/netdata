import { describe, expect, it, vi } from 'vitest';

import { signalHeadendClosed } from '../../headends/headend-close-utils.js';

describe('signalHeadendClosed', () => {
  it('resolves the close deferred on first signal', () => {
    const resolve = vi.fn();
    const reject = vi.fn();
    const event = { reason: 'stopped', graceful: true } as const;
    const closeDeferred = {
      promise: Promise.resolve(event),
      resolve,
      reject,
    };

    const result = signalHeadendClosed(false, closeDeferred, event);

    expect(result).toBe(true);
    expect(resolve).toHaveBeenCalledTimes(1);
    expect(resolve).toHaveBeenCalledWith(event);
  });

  it('does not resolve when already closed', () => {
    const resolve = vi.fn();
    const reject = vi.fn();
    const event = { reason: 'stopped', graceful: true } as const;
    const closeDeferred = {
      promise: Promise.resolve(event),
      resolve,
      reject,
    };

    const result = signalHeadendClosed(true, closeDeferred, event);

    expect(result).toBe(true);
    expect(resolve).not.toHaveBeenCalled();
  });
});
