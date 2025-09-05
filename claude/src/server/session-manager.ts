import crypto from 'node:crypto';

import type { AIAgentCallbacks, AIAgentResult, ConversationMessage, LogEntry } from '../types.js';
import type { RunKey, RunMeta } from './types.js';

interface Callbacks { onTreeUpdate?: (runId: string) => void; onLog?: (entry: LogEntry) => void; }

export class SessionManager {
  private readonly runs = new Map<string, RunMeta>();
  private readonly results = new Map<string, AIAgentResult>();
  private readonly outputs = new Map<string, string>();
  private readonly callbacks: Callbacks;
  private readonly runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string }) => Promise<AIAgentResult>;

  public constructor(runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string }) => Promise<AIAgentResult>, callbacks: Callbacks = {}) {
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

  public listActiveRuns(): RunMeta[] {
    return Array.from(this.runs.values()).filter((r) => r.status === 'running');
  }

  public startRun(key: RunKey, systemPrompt: string, userPrompt: string, history?: ConversationMessage[]): string {
    const runId = crypto.randomUUID();
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
          callbacks: {
            onLog: (entry: LogEntry) => {
              const m = this.runs.get(runId);
              if (m) {
                m.updatedAt = Date.now();
                this.runs.set(runId, m);
              }
              this.callbacks.onLog?.(entry);
              this.callbacks.onTreeUpdate?.(runId);
            },
            onOutput: (t: string) => {
              outputBuf.push(t);
              this.outputs.set(runId, outputBuf.join(''));
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

    return runId;
  }
}
