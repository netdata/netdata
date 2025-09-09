import crypto from 'node:crypto';

import type { AIAgentCallbacks, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry } from '../types.js';
import type { RunKey, RunMeta } from './types.js';

interface Callbacks { onTreeUpdate?: (runId: string) => void; onLog?: (entry: LogEntry) => void; }

export class SessionManager {
  private readonly runs = new Map<string, RunMeta>();
  private readonly results = new Map<string, AIAgentResult>();
  private readonly outputs = new Map<string, string>();
  private readonly logs = new Map<string, LogEntry[]>();
  private readonly accounting = new Map<string, AccountingEntry[]>();
  private readonly opTrees = new Map<string, unknown>();
  private readonly aborters = new Map<string, AbortController>();
  private readonly stopRefs = new Map<string, { stopping: boolean }>();
  private readonly callbacks: Callbacks;
  private readonly runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string }) => Promise<AIAgentResult>;

  public constructor(runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string; abortSignal?: AbortSignal; initialTitle?: string }) => Promise<AIAgentResult>, callbacks: Callbacks = {}) {
    this.runner = runner;
    this.callbacks = callbacks;
  }

  public getRun(runId: string): RunMeta | undefined {
    return this.runs.get(runId);
  }

  public getResult(runId: string): AIAgentResult | undefined {
    return this.results.get(runId);
  }

  public getOutput(runId: string): string | undefined {
    return this.outputs.get(runId);
  }

  public getLogs(runId: string): LogEntry[] {
    return this.logs.get(runId) ?? [];
  }

  public getAccounting(runId: string): AccountingEntry[] {
    return this.accounting.get(runId) ?? [];
  }
  public getOpTree(runId: string): unknown | undefined {
    return this.opTrees.get(runId);
  }

  public listActiveRuns(): RunMeta[] {
    return Array.from(this.runs.values()).filter((r) => r.status === 'running');
  }

  public cancelRun(runId: string, reason?: string): void {
    const meta = this.runs.get(runId);
    if (meta) {
      meta.status = 'canceled';
      meta.error = reason ?? 'canceled';
      meta.updatedAt = Date.now();
      this.runs.set(runId, meta);
      try { this.aborters.get(runId)?.abort(); } catch { /* ignore */ }
      this.callbacks.onTreeUpdate?.(runId);
    }
  }

  public stopRun(runId: string, reason?: string): void {
    const meta = this.runs.get(runId);
    if (meta && meta.status === 'running') {
      meta.status = 'stopping';
      meta.error = reason ?? 'stopping';
      meta.updatedAt = Date.now();
      this.runs.set(runId, meta);
      const ref = this.stopRefs.get(runId);
      if (ref) ref.stopping = true;
      this.callbacks.onTreeUpdate?.(runId);
    }
  }

  public startRun(key: RunKey, systemPrompt: string, userPrompt: string, history?: ConversationMessage[], opts?: { initialTitle?: string }): string {
    const runId = crypto.randomUUID();
    const aborter = new AbortController();
    const stopRef = { stopping: false };
    const meta: RunMeta = {
      runId,
      key,
      startedAt: Date.now(),
      updatedAt: Date.now(),
      status: 'running',
      model: undefined
    };
    this.runs.set(runId, meta);

    const outputBuf: string[] = [];

    void (async () => {
      try {
        const result = await this.runner(systemPrompt, userPrompt, {
          history,
          renderTarget: key.source,
          abortSignal: aborter.signal,
          stopRef,
          initialTitle: opts?.initialTitle,
          callbacks: {
          onLog: (entry: LogEntry) => {
              const m = this.runs.get(runId);
              if (m) {
                m.updatedAt = Date.now();
                this.runs.set(runId, m);
              }
              const arr = this.logs.get(runId) ?? [];
              arr.push(entry);
              this.logs.set(runId, arr);
              this.callbacks.onLog?.(entry);
            this.callbacks.onTreeUpdate?.(runId);
          },
          onOpTree: (tree) => {
            this.opTrees.set(runId, tree);
            this.callbacks.onTreeUpdate?.(runId);
          },
          onOutput: (t: string) => {
            outputBuf.push(t);
            this.outputs.set(runId, outputBuf.join(''));
            this.callbacks.onTreeUpdate?.(runId);
          },
            onAccounting: (a: AccountingEntry) => {
              const arr = this.accounting.get(runId) ?? [];
              arr.push(a);
              this.accounting.set(runId, arr);
              this.callbacks.onTreeUpdate?.(runId);
            }
          }
        });
        this.results.set(runId, result);
        const m = this.runs.get(runId);
        if (m) {
          m.status = result.success ? 'succeeded' : 'failed';
          m.error = result.success ? undefined : result.error ?? 'unknown error';
          m.updatedAt = Date.now();
          this.runs.set(runId, m);
        }
        this.callbacks.onTreeUpdate?.(runId);
      } catch (err: unknown) {
        const m = this.runs.get(runId);
        if (m) {
          m.status = 'failed';
          m.error = err instanceof Error ? err.message : 'run failed';
          m.updatedAt = Date.now();
          this.runs.set(runId, m);
        }
        this.callbacks.onTreeUpdate?.(runId);
      }
    })();
    this.aborters.set(runId, aborter);
    this.stopRefs.set(runId, stopRef);

    return runId;
  }
}
