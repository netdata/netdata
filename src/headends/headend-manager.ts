import { format } from 'node:util';

import type { LogEntry } from '../types.js';
import type { Headend, HeadendContext, HeadendDescription, HeadendLogSink } from './types.js';

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

const createDeferred = <T>(): Deferred<T> => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

export interface HeadendFatalEvent {
  headend: Headend;
  error: Error;
}

export interface HeadendManagerOptions {
  log?: HeadendLogSink;
  onFatal?: (event: HeadendFatalEvent) => void;
}

const noopLog: HeadendLogSink = () => { /* noop */ };

export class HeadendManager {
  private readonly headends: Headend[];
  private readonly log: HeadendLogSink;
  private readonly onFatal?: (event: HeadendFatalEvent) => void;
  private readonly active = new Set<Headend>();
  private readonly watchers: Promise<void>[] = [];
  private fatalResolved = false;
  private readonly fatalDeferred = createDeferred<HeadendFatalEvent | undefined>();
  private stopping = false;

  public constructor(headends: Headend[], opts: HeadendManagerOptions = {}) {
    this.headends = headends;
    this.log = opts.log ?? noopLog;
    this.onFatal = opts.onFatal;
  }

  public describe(): HeadendDescription[] {
    return this.headends.map((h) => h.describe());
  }

  public async startAll(): Promise<void> {
    await this.headends.reduce<Promise<void>>(async (prev, headend) => {
      await prev;
      await this.startHeadend(headend);
    }, Promise.resolve());
  }

  public async stopAll(): Promise<void> {
    this.stopping = true;
    const stops = Array.from(this.active).map(async (headend) => {
      try {
        await headend.stop();
      } catch (err) {
        this.emit(headend, 'WRN', 'response', `stop failed: ${this.safeError(err)}`);
      }
    });
    await Promise.allSettled(stops);
    await Promise.allSettled(this.watchers);
    if (!this.fatalResolved) {
      this.fatalResolved = true;
      this.fatalDeferred.resolve(undefined);
    }
  }

  public waitForFatal(): Promise<HeadendFatalEvent | undefined> {
    return this.fatalDeferred.promise;
  }

  private async startHeadend(headend: Headend): Promise<void> {
    const desc = headend.describe();
    this.emit(headend, 'VRB', 'request', `starting ${desc.label}`, false, desc);
    const ctx: HeadendContext = {
      log: (entry: LogEntry) => {
        const remote = typeof entry.remoteIdentifier === 'string' && entry.remoteIdentifier.length > 0
          ? entry.remoteIdentifier
          : `headend:${headend.kind}`;
        const merged: LogEntry = {
          ...entry,
          headendId: entry.headendId ?? desc.id,
          remoteIdentifier: remote,
        };
        this.log(merged);
      },
    };
    await headend.start(ctx);
    this.active.add(headend);
    this.watchers.push(this.watchHeadend(headend));
    this.emit(headend, 'VRB', 'response', `started ${desc.label}`, false, desc);
  }

  private async watchHeadend(headend: Headend): Promise<void> {
    try {
      const event = await headend.closed;
      this.active.delete(headend);
      if (event.reason === 'error') {
        this.emit(headend, 'ERR', 'response', `fatal error: ${event.error.message}`, true);
        this.registerFatal(headend, event.error);
      } else if (!event.graceful && !this.stopping) {
        const err = new Error('headend stopped unexpectedly');
        this.emit(headend, 'ERR', 'response', err.message, true);
        this.registerFatal(headend, err);
      } else {
        this.emit(headend, 'VRB', 'response', 'stopped', false);
      }
    } catch (err: unknown) {
      this.active.delete(headend);
      const error = err instanceof Error ? err : new Error(this.safeError(err));
      this.emit(headend, 'ERR', 'response', `fatal error: ${error.message}`, true);
      this.registerFatal(headend, error);
    }
  }

  private registerFatal(headend: Headend, error: Error): void {
    if (this.fatalResolved) return;
    this.fatalResolved = true;
    const event: HeadendFatalEvent = { headend, error };
    try { this.onFatal?.(event); } catch { /* ignore */ }
    this.fatalDeferred.resolve(event);
  }

  private emit(headend: Headend, severity: LogEntry['severity'], direction: LogEntry['direction'], message: string, fatal = false, desc?: HeadendDescription): void {
    const info = desc ?? headend.describe();
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity,
      turn: 0,
      subturn: 0,
      direction,
      type: 'tool',
      remoteIdentifier: `headend:${headend.kind}`,
      fatal,
      message,
      headendId: info.id,
    };
    this.log(entry);
  }

  private safeError(err: unknown): string {
    if (err instanceof Error) return err.message;
    try { return typeof err === 'string' ? err : format('%o', err); } catch { return 'unknown_error'; }
  }
}
