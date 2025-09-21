import { setImmediate as scheduleImmediate } from 'node:timers';

interface QueueEntry {
  readonly resolve: (release: () => void) => void;
  readonly cleanup: () => void;
  aborted: boolean;
}

export interface ConcurrencyAcquireOptions {
  signal?: AbortSignal;
}

/**
 * Simple semaphore-style concurrency limiter for headend request handling.
 */
export class ConcurrencyLimiter {
  private readonly limit: number;
  private active = 0;
  private readonly queue: QueueEntry[] = [];

  public constructor(limit: number) {
    if (!Number.isFinite(limit) || limit <= 0) {
      throw new Error(`Concurrency limit must be a positive finite number; received ${String(limit)}`);
    }
    this.limit = Math.floor(limit);
  }

  public get maxConcurrency(): number {
    return this.limit;
  }

  public async acquire(options: ConcurrencyAcquireOptions = {}): Promise<() => void> {
    if (this.active < this.limit) {
      this.active += 1;
      return this.createRelease();
    }

    const { signal } = options;
    const createAbortError = (): Error => {
      const err = new Error('acquire aborted');
      err.name = 'AbortError';
      return err;
    };

    return await new Promise<() => void>((resolve, reject) => {
      if (signal?.aborted === true) {
        reject(createAbortError());
        return;
      }

      let entry: QueueEntry;

      const cleanup = (): void => {
        signal?.removeEventListener('abort', onAbort);
      };

      const onAbort = (): void => {
        entry.aborted = true;
        cleanup();
        this.removeFromQueue(entry);
        reject(createAbortError());
      };

      entry = {
        resolve: (release: () => void) => {
          cleanup();
          resolve(release);
        },
        cleanup,
        aborted: false,
      };

      if (signal !== undefined) {
        signal.addEventListener('abort', onAbort, { once: true });
      }

      this.queue.push(entry);
    });
  }

  private createRelease(): () => void {
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.active = Math.max(0, this.active - 1);
      this.flushQueue();
    };
  }

  private flushQueue(): void {
    if (this.active >= this.limit) return;
    // eslint-disable-next-line functional/no-loop-statements
    while (this.queue.length > 0 && this.active < this.limit) {
      const next = this.queue.shift();
      if (next === undefined || next.aborted) {
        continue;
      }
      this.active += 1;
      const release = this.createRelease();
      scheduleImmediate(() => { next.resolve(release); });
      return;
    }
  }

  private removeFromQueue(entry: QueueEntry): void {
    const idx = this.queue.indexOf(entry);
    if (idx >= 0) {
      this.queue.splice(idx, 1);
    }
  }
}
