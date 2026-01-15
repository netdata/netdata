import { describe, expect, it, vi } from 'vitest';

import type { Socket } from 'node:net';

import { closeSockets } from '../../headends/socket-utils.js';

describe('closeSockets', () => {
  it('ends sockets immediately and destroys after delay', () => {
    vi.useFakeTimers();
    try {
      const end = vi.fn();
      const destroy = vi.fn();
      const socket = { end, destroy } as unknown as Socket;
      const sockets = new Set<Socket>([socket]);

      closeSockets(sockets, false);

      expect(end).toHaveBeenCalledTimes(1);
      expect(destroy).not.toHaveBeenCalled();

      vi.advanceTimersByTime(999);
      expect(destroy).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(destroy).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('destroys sockets immediately when forced', () => {
    vi.useFakeTimers();
    try {
      const end = vi.fn();
      const destroy = vi.fn();
      const socket = { end, destroy } as unknown as Socket;
      const sockets = new Set<Socket>([socket]);

      closeSockets(sockets, true);

      expect(end).toHaveBeenCalledTimes(1);
      vi.runAllTimers();
      expect(destroy).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
