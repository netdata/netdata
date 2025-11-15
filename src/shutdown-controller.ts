import type { LogEntry } from './types.js';

export type ShutdownTask = () => Promise<void> | void;

export class ShutdownController {
  private readonly abortController = new AbortController();
  private readonly globalStopRef = { stopping: false };
  private readonly tasks = new Map<string, ShutdownTask>();
  private stopping = false;
  private shutdownPromise?: Promise<void>;

  public get signal(): AbortSignal {
    return this.abortController.signal;
  }

  public get stopRef(): { stopping: boolean } {
    return this.globalStopRef;
  }

  public isStopping(): boolean {
    return this.stopping;
  }

  public register(name: string, task: ShutdownTask): () => void {
    this.tasks.set(name, task);
    return () => {
      this.tasks.delete(name);
    };
  }

  public async shutdown(opts: { logger?: (entry: LogEntry) => void } = {}): Promise<void> {
    if (this.shutdownPromise !== undefined) {
      await this.shutdownPromise;
      return;
    }
    this.shutdownPromise = this.performShutdown(opts);
    await this.shutdownPromise;
  }

  private async performShutdown(opts: { logger?: (entry: LogEntry) => void }): Promise<void> {
    if (this.stopping) return;
    this.stopping = true;
    this.globalStopRef.stopping = true;
    try {
      this.abortController.abort();
    } catch { /* ignore */ }
    const logger = opts.logger;
    const entries = Array.from(this.tasks.entries()).reverse();
    // eslint-disable-next-line functional/no-loop-statements -- ordered cleanup matters
    for (const [name, task] of entries) {
      try {
        await task();
      } catch (error) {
        if (logger !== undefined) {
          const message = error instanceof Error ? error.message : String(error);
          logger({
            timestamp: Date.now(),
            severity: 'WRN',
            turn: 0,
            subturn: 0,
            direction: 'response',
            type: 'tool',
            remoteIdentifier: 'shutdown',
            fatal: false,
            message: `shutdown task '${name}' failed: ${message}`,
          });
        }
      }
    }
  }
}
