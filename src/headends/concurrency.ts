import { setImmediate as scheduleImmediate } from 'node:timers';

/**
 * Simple semaphore-style concurrency limiter for headend request handling.
 */
export class ConcurrencyLimiter {
  private readonly limit: number;
  private active = 0;
  private readonly queue: ((release: () => void) => void)[] = [];

  public constructor(limit: number) {
    if (!Number.isFinite(limit) || limit <= 0) {
      throw new Error(`Concurrency limit must be a positive finite number; received ${String(limit)}`);
    }
    this.limit = Math.floor(limit);
  }

  public get maxConcurrency(): number {
    return this.limit;
  }

  public async acquire(): Promise<() => void> {
    if (this.active < this.limit) {
      this.active += 1;
      return this.createRelease();
    }
    return await new Promise<() => void>((resolve) => {
      this.queue.push((release) => { resolve(release); });
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
    if (this.queue.length === 0) return;
    if (this.active >= this.limit) return;
    const next = this.queue.shift();
    if (next === undefined) return;
    this.active += 1;
    const release = this.createRelease();
    // schedule release delivery outside current call stack to avoid reentrancy surprises
    scheduleImmediate(() => { next(release); });
  }
}
