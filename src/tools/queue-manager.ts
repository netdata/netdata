import type { QueueConfig } from '../types.js';

import { DEFAULT_QUEUE_CONCURRENCY } from '../config.js';

interface QueueState {
  capacity: number;
  inUse: number;
  waiters: Waiter[];
}

interface Waiter {
  resolve: (info: AcquireResult) => void;
  reject: (err: unknown) => void;
  startTs: number;
  queuedDepth: number;
  signal?: AbortSignal;
  abortListener?: () => void;
}

export interface AcquireContext {
  signal?: AbortSignal;
  agentId?: string;
  toolName?: string;
}

export interface AcquireResult {
  queued: boolean;
  waitMs: number;
  depth: number;
  capacity: number;
  queuedDepth: number;
}

export interface QueueStatus {
  capacity: number;
  inUse: number;
  waiting: number;
}

interface QueueListeners {
  onDepthChange?: (queue: string, info: QueueStatus) => void;
  onWaitComplete?: (queue: string, waitMs: number) => void;
}

class QueueManagerImpl {
  private queues = new Map<string, QueueState>();
  private listeners: QueueListeners = {};
  private configuredSnapshot = new Map<string, number>();
  private initialized = false;

  configure(map: Record<string, QueueConfig | undefined>): void {
    const incoming = this.normalizeQueues(map);
    incoming.forEach((capacity, name) => {
      const existing = this.queues.get(name);
      if (existing === undefined) {
        this.queues.set(name, { capacity, inUse: 0, waiters: [] });
        this.emitDepth(name);
      } else if (capacity > existing.capacity) {
        existing.capacity = capacity;
        this.emitDepth(name);
      }
      const prevSnapshot = this.configuredSnapshot.get(name) ?? 0;
      if (capacity > prevSnapshot) {
        this.configuredSnapshot.set(name, capacity);
      }
    });
    this.initialized = true;
  }

  reset(): void {
    this.queues.clear();
    this.configuredSnapshot.clear();
    this.initialized = false;
  }

  setListeners(listeners: QueueListeners): void {
    this.listeners = listeners;
  }

  async acquire(queueName: string, ctx?: AcquireContext): Promise<AcquireResult> {
    const state = this.queues.get(queueName);
    if (state === undefined) throw new Error(`Queue '${queueName}' is not configured`);
    if (ctx?.signal?.aborted === true) throw abortError();
    if (state.inUse < state.capacity) {
      state.inUse += 1;
      this.emitDepth(queueName);
      return { queued: false, waitMs: 0, depth: state.waiters.length, capacity: state.capacity, queuedDepth: 0 };
    }
    return await new Promise<AcquireResult>((resolve, reject) => {
      const waiter: Waiter = {
        resolve,
        reject,
        startTs: Date.now(),
        queuedDepth: state.waiters.length + 1,
      };
      const signal = ctx?.signal;
      if (signal !== undefined) {
        const listener = () => {
          this.removeWaiter(queueName, waiter);
          reject(abortError());
        };
        signal.addEventListener('abort', listener, { once: true });
        waiter.signal = signal;
        waiter.abortListener = listener;
      }
      state.waiters.push(waiter);
      this.emitDepth(queueName);
    });
  }

  release(queueName: string): void {
    const state = this.queues.get(queueName);
    if (state === undefined) return;
    if (state.inUse > 0) state.inUse -= 1;
    this.runNext(queueName, state);
  }

  getStatus(queueName: string): QueueStatus | undefined {
    const state = this.queues.get(queueName);
    if (state === undefined) return undefined;
    return { capacity: state.capacity, inUse: state.inUse, waiting: state.waiters.length };
  }

  private normalizeQueues(map: Record<string, QueueConfig | undefined>): Map<string, number> {
    const normalized = new Map<string, number>();
    Object.entries(map).forEach(([name, cfg]) => {
      const capacity = Math.max(1, Math.trunc(cfg?.concurrent ?? DEFAULT_QUEUE_CONCURRENCY));
      normalized.set(name, capacity);
    });
    if (!normalized.has('default')) {
      normalized.set('default', DEFAULT_QUEUE_CONCURRENCY);
    }
    return normalized;
  }

  private runNext(queueName: string, state: QueueState): void {
    // eslint-disable-next-line functional/no-loop-statements
    while (state.waiters.length > 0) {
      const waiter = state.waiters.shift();
      if (waiter === undefined) break;
      if (waiter.signal?.aborted === true) {
        this.clearAbort(waiter);
        waiter.reject(abortError());
        continue;
      }
      if (state.inUse < state.capacity) {
        state.inUse += 1;
        this.clearAbort(waiter);
        const waitMs = Date.now() - waiter.startTs;
        const result: AcquireResult = {
          queued: true,
          waitMs,
          depth: state.waiters.length,
          capacity: state.capacity,
          queuedDepth: waiter.queuedDepth,
        };
        try {
          waiter.resolve(result);
        } finally {
          this.listeners.onWaitComplete?.(queueName, waitMs);
        }
        this.emitDepth(queueName);
        return;
      }
      state.waiters.unshift(waiter);
      break;
    }
    this.emitDepth(queueName);
  }

  private removeWaiter(queueName: string, waiter: Waiter): void {
    const state = this.queues.get(queueName);
    if (state === undefined) return;
    const idx = state.waiters.indexOf(waiter);
    if (idx >= 0) {
      state.waiters.splice(idx, 1);
      this.emitDepth(queueName);
    }
    this.clearAbort(waiter);
  }

  private emitDepth(queue: string): void {
    const state = this.queues.get(queue);
    if (state === undefined) return;
    this.listeners.onDepthChange?.(queue, { capacity: state.capacity, inUse: state.inUse, waiting: state.waiters.length });
  }

  private clearAbort(waiter: Waiter): void {
    if (waiter.signal !== undefined && waiter.abortListener !== undefined) {
      waiter.signal.removeEventListener('abort', waiter.abortListener);
      waiter.abortListener = undefined;
    }
  }
}

const manager = new QueueManagerImpl();

export const queueManager = {
  configureQueues(map: Record<string, QueueConfig | undefined>): void {
    manager.configure(map);
  },
  reset(): void {
    manager.reset();
  },
  setListeners(listeners: QueueListeners): void {
    manager.setListeners(listeners);
  },
  acquire(queue: string, ctx?: AcquireContext): Promise<AcquireResult> {
    return manager.acquire(queue, ctx);
  },
  release(queue: string): void {
    manager.release(queue);
  },
  getQueueStatus(queue: string): QueueStatus | undefined {
    return manager.getStatus(queue);
  },
};

function abortError(): DOMException {
  return new DOMException('Queue wait aborted', 'AbortError');
}
