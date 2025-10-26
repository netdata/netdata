import crypto from 'node:crypto';

import { SpanKind, SpanStatusCode } from '@opentelemetry/api';

import type { AIAgentCallbacks, AIAgentResult, ConversationMessage, LogEntry, ProgressEvent } from '../types.js';
import { addSpanAttributes, addSpanEvent, recordSpanError, runWithSpan } from '../telemetry/index.js';
import { warn } from '../utils.js';
import type { RunKey, RunMeta } from './types.js';

interface Callbacks {
  onTreeUpdate?: (runId: string) => void;
  onLog?: (entry: LogEntry) => void;
  onProgress?: (runId: string, event: ProgressEvent) => void;
  onThinking?: (runId: string, chunk: string) => void;
}

export class SessionManager {
  private readonly runs = new Map<string, RunMeta>();
  private readonly results = new Map<string, AIAgentResult>();
  private readonly outputs = new Map<string, string>();
  // Deprecated: we no longer store flat logs/accounting; opTree is canonical
  private readonly opTrees = new Map<string, unknown>();
  private readonly ingress = new Map<string, Record<string, unknown>>();
  private readonly aborters = new Map<string, AbortController>();
  private readonly stopRefs = new Map<string, { stopping: boolean }>();
  private readonly callbacks: Callbacks;
  private readonly treeUpdateListeners = new Set<(runId: string) => void>();
  private readonly runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; headendId?: string; telemetryLabels?: Record<string, string> }) => Promise<AIAgentResult>;
  private readonly headendId?: string;
  private readonly telemetryLabels: Record<string, string>;

  public constructor(
    runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentCallbacks; renderTarget?: 'slack' | 'api' | 'web'; outputFormat?: string; abortSignal?: AbortSignal; initialTitle?: string; headendId?: string; telemetryLabels?: Record<string, string> }) => Promise<AIAgentResult>,
    callbacks: Callbacks = {},
    options: { headendId?: string; telemetryLabels?: Record<string, string> } = {},
  ) {
    this.runner = runner;
    this.callbacks = callbacks;
    this.headendId = options.headendId;
    const telemetryLabels = { ...(options.telemetryLabels ?? {}) };
    if (this.headendId !== undefined && telemetryLabels.headend === undefined) {
      telemetryLabels.headend = this.headendId;
    }
    this.telemetryLabels = telemetryLabels;
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

  public getOpTree(runId: string): unknown | undefined {
    return this.opTrees.get(runId);
  }

  // Subscribe to tree updates (opTree/logs/accounting). Returns an unsubscribe function.
  public onTreeUpdate(cb: (runId: string) => void): () => void {
    this.treeUpdateListeners.add(cb);
    return () => { this.treeUpdateListeners.delete(cb); };
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
      try { this.aborters.get(runId)?.abort(); } catch (e) { warn(`abort failed for run ${runId}: ${e instanceof Error ? e.message : String(e)}`); }
      this.callbacks.onTreeUpdate?.(runId);
      for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
    }
  }

  public stopRun(runId: string, reason?: string): void {
    const meta = this.runs.get(runId);
    if (meta?.status === 'running') {
      meta.status = 'stopping';
      meta.error = reason ?? 'stopping';
      meta.updatedAt = Date.now();
      this.runs.set(runId, meta);
      const ref = this.stopRefs.get(runId);
      if (ref) ref.stopping = true;
      this.callbacks.onTreeUpdate?.(runId);
      for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
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

    // Log concurrent runs for debugging
    const activeRuns = Array.from(this.runs.values()).filter(r => r.status === 'running');
    if (activeRuns.length > 1) {
      const channels = activeRuns.map(r => r.key.source === 'slack' ? r.key.channelId : 'non-slack').filter(Boolean);
      if (key.source === 'slack') {
        try {
          process.stderr.write(`[CONCURRENT] Starting run ${runId} for channel ${key.channelId} with ${String(activeRuns.length - 1)} other active runs: ${channels.join(', ')}\n`);
        } catch {}
      }
    }
    // Capture ingress metadata for later attachment to opTree
    const ing: Record<string, unknown> = { source: key.source };
    if (key.source === 'slack') {
      if (key.teamId) ing.teamId = key.teamId;
      if (key.channelId) ing.channelId = key.channelId;
      if (key.threadTsOrSessionId) ing.threadTs = key.threadTsOrSessionId;
    } else if (key.source === 'api') {
      ing.requestId = key.threadTsOrSessionId;
    }
    this.ingress.set(runId, ing);

    const outputBuf: string[] = [];

    const spanAttributes: Record<string, string> = {
      'ai.session.run_id': runId,
      'ai.session.source': key.source,
    };
    if (this.headendId !== undefined) spanAttributes['ai.session.headend_id'] = this.headendId;
    if (key.threadTsOrSessionId !== undefined) spanAttributes['ai.session.thread_id'] = key.threadTsOrSessionId;
    if (key.source === 'slack') {
      if (key.teamId !== undefined) spanAttributes['ai.session.slack.team_id'] = key.teamId;
      if (key.channelId !== undefined) spanAttributes['ai.session.slack.channel_id'] = key.channelId;
    } else if (key.source === 'api' && key.threadTsOrSessionId !== undefined) {
      spanAttributes['ai.session.api.request_id'] = key.threadTsOrSessionId;
    }
    Object.entries(this.telemetryLabels).forEach(([labelKey, labelValue]) => {
      spanAttributes[`telemetry.label.${labelKey}`] = labelValue;
    });

    void (async () => {
      try {
        const result = await runWithSpan('headend.session', { attributes: spanAttributes, kind: SpanKind.SERVER }, async (span) => {
          addSpanEvent('session.start', { run_id: runId, source: key.source });
          const res = await this.runner(systemPrompt, userPrompt, {
            history,
            renderTarget: key.source,
            abortSignal: aborter.signal,
            stopRef,
            initialTitle: opts?.initialTitle,
            headendId: this.headendId,
            telemetryLabels: this.telemetryLabels,
            callbacks: {
              onLog: (entry: LogEntry) => {
                const m = this.runs.get(runId);
                if (m) {
                  m.updatedAt = Date.now();
                  this.runs.set(runId, m);
                }
                try { this.callbacks.onLog?.(entry); } catch (e) { warn(`callbacks.onLog failed: ${e instanceof Error ? e.message : String(e)}`); }
                this.callbacks.onTreeUpdate?.(runId);
              },
              onOpTree: (tree) => {
                try {
                  const ingressAttributes = this.ingress.get(runId);
                  if (ingressAttributes && tree && typeof tree === 'object') {
                    const root = tree as Record<string, unknown>;
                    const attrs = (root.attributes as Record<string, unknown> | undefined) ?? {};
                    attrs.ingress = { ...(attrs.ingress as Record<string, unknown> ?? {}), ...ingressAttributes };
                    root.attributes = attrs;
                  }
                } catch (e) { warn(`opTree ingress enrichment failed: ${e instanceof Error ? e.message : String(e)}`); }
                this.opTrees.set(runId, tree);
                try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
              },
              onOutput: (t: string) => {
                outputBuf.push(t);
                this.outputs.set(runId, outputBuf.join(''));
                try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
              },
              onThinking: (chunk: string) => {
                addSpanEvent('session.thinking', { length: chunk.length });
                try { this.callbacks.onThinking?.(runId, chunk); } catch (e) { warn(`callbacks.onThinking failed: ${e instanceof Error ? e.message : String(e)}`); }
                try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
                for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
              },
              onProgress: (event: ProgressEvent) => {
                addSpanEvent('session.progress', {
                  'progress.type': event.type,
                  'progress.call_path': event.callPath,
                  'progress.agent_id': 'agentId' in event ? event.agentId : undefined,
                });
                try { this.callbacks.onProgress?.(runId, event); } catch (e) { warn(`callbacks.onProgress failed: ${e instanceof Error ? e.message : String(e)}`); }
              },
              onAccounting: (_a) => {
                this.callbacks.onTreeUpdate?.(runId);
                for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
              }
            }
          });
          addSpanAttributes({ 'ai.session.success': res.success });
          if (!res.success) {
            const message = res.error ?? 'unknown error';
            addSpanAttributes({ 'ai.session.error': message });
            span.setStatus({ code: SpanStatusCode.ERROR, message });
          }
          addSpanEvent('session.complete', { success: res.success });
          return res;
        });
        this.results.set(runId, result);
        const m = this.runs.get(runId);
        if (m) {
          const wasStopping = (m.status === 'stopping') || (this.stopRefs.get(runId)?.stopping === true);
          if (result.success) {
            m.status = 'succeeded';
            // Preserve user intent: if run was in stopping mode, mark reason for UI
            m.error = wasStopping ? 'stopped' : undefined;
          } else {
            m.status = 'failed';
            m.error = result.error ?? 'unknown error';
          }
          m.updatedAt = Date.now();
          this.runs.set(runId, m);
        }
        try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
        for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
      } catch (err: unknown) {
        recordSpanError(err);
        const m = this.runs.get(runId);
        if (m) {
          m.status = 'failed';
          m.error = err instanceof Error ? err.message : 'run failed';
          m.updatedAt = Date.now();
          this.runs.set(runId, m);
        }
        try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
        for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
      }
    })();
    this.aborters.set(runId, aborter);
    this.stopRefs.set(runId, stopRef);

    return runId;
  }
}
