import crypto from 'node:crypto';

import { SpanKind, SpanStatusCode } from '@opentelemetry/api';

import type { AIAgentEvent, AIAgentEventCallbacks, AIAgentEventMeta, AIAgentResult, ConversationMessage, ProgressEvent } from '../types.js';
import { addSpanAttributes, addSpanEvent, recordSpanError, runWithSpan } from '../telemetry/index.js';
import { isPlainObject, warn } from '../utils.js';
import type { RunKey, RunMeta } from './types.js';

interface Callbacks {
  onTreeUpdate?: (runId: string) => void;
  onEvent?: (runId: string, event: AIAgentEvent, meta: AIAgentEventMeta) => void;
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
  private readonly runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentEventCallbacks; renderTarget?: 'slack' | 'api' | 'web' | 'embed'; outputFormat?: string; abortSignal?: AbortSignal; stopRef?: { stopping: boolean }; initialTitle?: string; headendId?: string; telemetryLabels?: Record<string, string> }) => Promise<AIAgentResult>;
  private readonly headendId?: string;
  private readonly telemetryLabels: Record<string, string>;

  public constructor(
    runner: (systemPrompt: string, userPrompt: string, opts: { history?: ConversationMessage[]; callbacks?: AIAgentEventCallbacks; renderTarget?: 'slack' | 'api' | 'web' | 'embed'; outputFormat?: string; abortSignal?: AbortSignal; initialTitle?: string; headendId?: string; telemetryLabels?: Record<string, string> }) => Promise<AIAgentResult>,
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
              onEvent: (event: AIAgentEvent, meta: AIAgentEventMeta) => {
                const runMeta = this.runs.get(runId);
                if (runMeta) {
                  runMeta.updatedAt = Date.now();
                  this.runs.set(runId, runMeta);
                }

                if (event.type === 'op_tree') {
                  try {
                    const ingressAttributes = this.ingress.get(runId);
                    if (ingressAttributes && event.tree && typeof event.tree === 'object') {
                      const baseAttrs = event.tree.attributes ?? {};
                      const existingIngress = isPlainObject(baseAttrs.ingress) ? baseAttrs.ingress : {};
                      event.tree.attributes = { ...baseAttrs, ingress: { ...existingIngress, ...ingressAttributes } };
                    }
                  } catch (e) {
                    warn(`opTree ingress enrichment failed: ${e instanceof Error ? e.message : String(e)}`);
                  }
                  this.opTrees.set(runId, event.tree);
                }

                if (event.type === 'output' && meta.isMaster) {
                  if (!(meta.source === 'finalize' && meta.pendingHandoffCount > 0)) {
                    outputBuf.push(event.text);
                    this.outputs.set(runId, outputBuf.join(''));
                  }
                }

                if (event.type === 'thinking') {
                  addSpanEvent('session.thinking', { length: event.text.length });
                }

                if (event.type === 'progress') {
                  const progressEvent: ProgressEvent = event.event;
                  addSpanEvent('session.progress', {
                    'progress.type': progressEvent.type,
                    'progress.call_path': progressEvent.callPath,
                    'progress.agent_id': 'agentId' in progressEvent ? progressEvent.agentId : undefined,
                  });
                }

                try {
                  this.callbacks.onEvent?.(runId, event, meta);
                } catch (e) {
                  warn(`callbacks.onEvent failed: ${e instanceof Error ? e.message : String(e)}`);
                }

                if (
                  event.type === 'log'
                  || event.type === 'op_tree'
                  || event.type === 'output'
                  || event.type === 'thinking'
                  || event.type === 'accounting'
                ) {
                  try { this.callbacks.onTreeUpdate?.(runId); } catch (e) { warn(`callbacks.onTreeUpdate failed: ${e instanceof Error ? e.message : String(e)}`); }
                  for (const fn of this.treeUpdateListeners) { try { fn(runId); } catch (e) { warn(`treeUpdate listener failed: ${e instanceof Error ? e.message : String(e)}`); } }
                }
              },
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
