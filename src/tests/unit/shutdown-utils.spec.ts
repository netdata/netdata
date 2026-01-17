import { describe, expect, it, vi } from 'vitest';

import { handleHeadendShutdown, type StopRef, type StopReason } from '../../headends/shutdown-utils.js';

describe('handleHeadendShutdown', () => {
  it('marks stopRef and invokes close callback', () => {
    const stopRef: StopRef = { stopping: false };
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

  it('preserves existing reason when stopRef already has one', () => {
    const stopRef: StopRef = { stopping: false, reason: 'stop' };
    const close = vi.fn();

    handleHeadendShutdown(stopRef, close);

    expect(stopRef.stopping).toBe(true);
    expect(stopRef.reason).toBe('stop');
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('allows reason to be undefined initially', () => {
    const stopRef: StopRef = { stopping: false };
    const close = vi.fn();

    handleHeadendShutdown(stopRef, close);

    expect(stopRef.stopping).toBe(true);
    expect(stopRef.reason).toBeUndefined();
    expect(close).toHaveBeenCalledTimes(1);
  });
});

describe('StopRef interface', () => {
  it('accepts stopping field', () => {
    const ref: StopRef = { stopping: true };
    expect(ref.stopping).toBe(true);
  });

  it('accepts optional reason field with stop value', () => {
    const ref: StopRef = { stopping: true, reason: 'stop' };
    expect(ref.reason).toBe('stop');
  });

  it('accepts optional reason field with abort value', () => {
    const ref: StopRef = { stopping: true, reason: 'abort' };
    expect(ref.reason).toBe('abort');
  });

  it('accepts optional reason field with shutdown value', () => {
    const ref: StopRef = { stopping: true, reason: 'shutdown' };
    expect(ref.reason).toBe('shutdown');
  });
});

describe('StopReason type', () => {
  it('allows stop value', () => {
    const reason: StopReason = 'stop';
    expect(reason).toBe('stop');
  });

  it('allows abort value', () => {
    const reason: StopReason = 'abort';
    expect(reason).toBe('abort');
  });

  it('allows shutdown value', () => {
    const reason: StopReason = 'shutdown';
    expect(reason).toBe('shutdown');
  });
});
