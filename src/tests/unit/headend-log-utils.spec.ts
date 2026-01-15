import { describe, expect, it, vi } from 'vitest';

import type { HeadendContext } from '../../headends/types.js';

import { logHeadendEntry } from '../../headends/headend-log-utils.js';

describe('logHeadendEntry', () => {
  it('logs the entry when context exists', () => {
    const log = vi.fn();
    const context: HeadendContext = {
      log,
      shutdownSignal: new AbortController().signal,
      stopRef: { stopping: false },
    };
    const entry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: 0,
      subturn: 0,
      direction: 'response',
      type: 'tool',
      remoteIdentifier: 'test',
      fatal: false,
      message: 'hello',
    } as const;

    logHeadendEntry(context, entry);

    expect(log).toHaveBeenCalledTimes(1);
    expect(log).toHaveBeenCalledWith(entry);
  });

  it('skips logging when context is missing', () => {
    const entry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: 0,
      subturn: 0,
      direction: 'response',
      type: 'tool',
      remoteIdentifier: 'test',
      fatal: false,
      message: 'hello',
    } as const;

    expect(() => {
      logHeadendEntry(undefined, entry);
    }).not.toThrow();
  });
});
