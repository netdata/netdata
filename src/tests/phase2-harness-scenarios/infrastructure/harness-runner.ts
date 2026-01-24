/**
 * Core test runner logic for the Phase 2 deterministic test harness.
 * Extracted from phase2-runner.ts for modularity.
 */
/* eslint-disable import/order -- conflict with perfectionist/sort-imports on sibling vs parent order */

import os from 'node:os';

import type { AIAgentResult, AIAgentSessionConfig, TurnRequest, TurnResult } from '../../../types.js';
import type { ExecuteTurnHandler, HarnessTest, PassThroughHandler, ScenarioRunOutput, TestContext, TestRunResult } from './harness-types.js';
import type { ChildProcess } from 'node:child_process';

import { AIAgent, AIAgentSession } from '../../../ai-agent.js';
import { LLMClient } from '../../../llm-client.js';
import { shutdownSharedRegistry } from '../../../tools/mcp-provider.js';
import { queueManager } from '../../../tools/queue-manager.js';

import { delay, toError } from './harness-helpers.js';

/** Default timeout for individual test scenarios */
export const DEFAULT_SCENARIO_TIMEOUT_MS = (() => {
  const raw = process.env.PHASE1_SCENARIO_TIMEOUT_MS;
  if (raw === undefined) return 10_000;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 10_000;
})();

/** Environment variable to force all-sequential mode (for debugging) */
export const PHASE2_FORCE_SEQUENTIAL = process.env.PHASE2_FORCE_SEQUENTIAL === '1';

/** Environment variable to run only parallel or sequential tests: 'all' (default), 'parallel', 'sequential' */
export const PHASE2_MODE = (process.env.PHASE2_MODE ?? 'all') as 'all' | 'parallel' | 'sequential';

/** Maximum parallel test concurrency (uses availableParallelism if available, falls back to cpus().length for older Node.js) */
export const PHASE2_PARALLEL_CONCURRENCY = Math.min(
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime fallback for older Node.js versions
  Number(process.env.PHASE2_PARALLEL_CONCURRENCY) || Math.max(1, Math.floor(os.availableParallelism?.() ?? os.cpus().length) / 2),
  32
);

/**
 * Get the effective timeout for a test scenario.
 */
export function getEffectiveTimeoutMs(test: HarnessTest): number {
  if (typeof test.timeoutMs === 'number' && Number.isFinite(test.timeoutMs) && test.timeoutMs > 0) {
    return test.timeoutMs;
  }
  return DEFAULT_SCENARIO_TIMEOUT_MS;
}

/**
 * Run a session with a custom executeTurn handler using DI (dependency injection).
 * This replaces the prototype-patching approach.
 * The override is injected via sessionConfig.executeTurnOverride, which flows to LLMClient.
 * This is safe for parallel test execution since each session gets its own isolated override.
 */
export const runWithExecuteTurnOverride = async (
  sessionConfig: AIAgentSessionConfig,
  handler: ExecuteTurnHandler,
): Promise<AIAgentResult> => {
  let invocation = 0;
  sessionConfig.executeTurnOverride = async (request: TurnRequest): Promise<TurnResult> => {
    invocation += 1;
    return handler({ request, invocation });
  };
  const session = AIAgentSession.create(sessionConfig);
  return AIAgent.run(session);
};

/**
 * Run a session with pass-through capture using DI.
 * This allows tests to intercept requests, capture data, and optionally pass through to the actual provider.
 * The passThrough function creates a fresh LLMClient internally to call the real provider.
 * This is safe for parallel execution since each test gets its own isolated state.
 */
export const runWithPassThroughCapture = async (
  sessionConfig: AIAgentSessionConfig,
  handler: PassThroughHandler,
  context: TestContext,
): Promise<AIAgentResult> => {
  let invocation = 0;

  sessionConfig.executeTurnOverride = async (request: TurnRequest): Promise<TurnResult> => {
    invocation += 1;

    // Create a pass-through function that calls the actual provider
    const passThrough = async (): Promise<TurnResult> => {
      // Create a fresh LLMClient without override to call actual providers
      const passThroughClient = new LLMClient(sessionConfig.config.providers, {
        traceLLM: sessionConfig.traceLLM,
        traceSDK: sessionConfig.traceSdk,
        // No executeTurnOverride - calls actual providers
      });
      return passThroughClient.executeTurn(request);
    };

    // Let the handler decide whether to override or pass through
    const result = await handler({ request, invocation, context, passThrough });
    if (result !== undefined) {
      return result;
    }
    // Handler returned undefined - pass through to actual provider
    return passThrough();
  };

  const session = AIAgentSession.create(sessionConfig);
  return AIAgent.run(session);
};

/**
 * Format a failure hint from an AIAgentResult.
 */
export function formatFailureHint(result: AIAgentResult): string {
  const hints: string[] = [];
  if (!result.success && typeof result.error === 'string' && result.error.length > 0) {
    hints.push(`session-error="${result.error}"`);
  }
  const lastLog = result.logs.at(-1);
  if (lastLog !== undefined) {
    const msg = typeof lastLog.message === 'string' ? lastLog.message : JSON.stringify(lastLog.message);
    hints.push(`last-log="${msg}"`);
  }
  if (result.finalReport !== undefined) {
    hints.push(`final-status=${result.success ? 'success' : 'failure'}`);
  }
  return hints.length > 0 ? ` (${hints.join(' | ')})` : '';
}

/**
 * Run a parallel batch of test scenarios with limited concurrency.
 */
export async function runParallelBatch(
  scenarios: HarnessTest[],
  concurrency: number,
  runScenario: (test: HarnessTest) => Promise<ScenarioRunOutput>,
  dumpScenarioResultIfNeeded: (scenarioId: string, result: AIAgentResult) => void,
): Promise<TestRunResult[]> {
  const results: TestRunResult[] = [];
  const pending: Promise<void>[] = [];
  let nextIndex = 0;

  const runSingleScenario = async (scenario: HarnessTest): Promise<TestRunResult> => {
    const startMs = Date.now();
    let result: AIAgentResult | undefined;
    try {
      const output = await runScenario(scenario);
      result = output.result;
      dumpScenarioResultIfNeeded(scenario.id, result);
      scenario.expect(result, output.context);
      return { scenario, durationMs: Date.now() - startMs };
    } catch (error: unknown) {
      return { scenario, result, error: toError(error), durationMs: Date.now() - startMs };
    }
  };

  const startNext = async (): Promise<void> => {
    if (nextIndex >= scenarios.length) return;
    const scenario = scenarios[nextIndex];
    nextIndex += 1;
    const result = await runSingleScenario(scenario);
    results.push(result);
    await startNext();
  };

  // Start initial batch up to concurrency limit
  const initialBatch = Math.min(concurrency, scenarios.length);
  // eslint-disable-next-line functional/no-loop-statements
  for (let i = 0; i < initialBatch; i += 1) {
    pending.push(startNext());
  }

  await Promise.all(pending);
  return results;
}

/**
 * Clean up active handles (child processes, sockets, etc.) after test execution.
 */
export async function cleanupActiveHandles(): Promise<void> {
  const diagnosticProcess = process as typeof process & { _getActiveHandles?: () => unknown[] };
  const collectHandles = (): unknown[] => (typeof diagnosticProcess._getActiveHandles === 'function' ? diagnosticProcess._getActiveHandles() : []);
  const shouldIgnore = (handle: unknown): boolean => {
    if (handle === null || typeof handle !== 'object') return false;
    const fd = (handle as { fd?: unknown }).fd;
    if (typeof fd === 'number' && (fd === 0 || fd === 1 || fd === 2)) return true;
    const ctorName = (handle as { constructor?: { name?: string } }).constructor?.name;
    if (ctorName === 'Socket') {
      const maybeLocal = (handle as { localAddress?: unknown; remoteAddress?: unknown });
      const local = typeof maybeLocal.localAddress === 'string' ? maybeLocal.localAddress : undefined;
      const remote = typeof maybeLocal.remoteAddress === 'string' ? maybeLocal.remoteAddress : undefined;
      if (local === undefined && remote === undefined) {
        return true;
      }
    }
    return false;
  };
  const formatHandle = (handle: unknown): string => {
    if (handle === null) return 'null';
    if (typeof handle !== 'object') return typeof handle;
    const name = (handle as { constructor?: { name?: string } }).constructor?.name ?? 'object';
    const parts: string[] = [];
    const pid = (handle as { pid?: unknown }).pid;
    if (typeof pid === 'number') { parts.push(`pid=${String(pid)}`); }
    const fd = (handle as { fd?: unknown }).fd;
    if (typeof fd === 'number') { parts.push(`fd=${String(fd)}`); }
    const maybeLocal = (handle as { localAddress?: unknown; remoteAddress?: unknown });
    const local = typeof maybeLocal.localAddress === 'string' ? maybeLocal.localAddress : undefined;
    const remote = typeof maybeLocal.remoteAddress === 'string' ? maybeLocal.remoteAddress : undefined;
    if (local !== undefined || remote !== undefined) {
      parts.push(`socket=${local ?? ''}->${remote ?? ''}`);
    }
    return parts.length > 0 ? `${name}(${parts.join(';')})` : name;
  };

  let activeHandles = collectHandles();
  if (activeHandles.length === 0) return;

  // Run up to two cleanup passes to give child processes time to exit after SIGTERM/SIGKILL.
  // eslint-disable-next-line functional/no-loop-statements -- controlled cleanup loop is clearer than array methods here
  for (let pass = 0; pass < 2 && activeHandles.length > 0; pass += 1) {
    const childCleanup: Promise<void>[] = [];
    activeHandles.forEach((handle) => {
      if (handle === null || typeof handle !== 'object') return;
      const maybePid = (handle as { pid?: unknown }).pid;
      if (typeof maybePid === 'number') {
        const child = handle as ChildProcess;
        const exitPromise = new Promise<void>((resolve) => {
          const finish = (): void => { resolve(); };
          child.once('exit', finish);
          child.once('error', finish);
          setTimeout(() => { resolve(); }, 1500);
        });
        childCleanup.push(exitPromise);
        try {
          if (typeof child.kill === 'function') {
            try { child.kill('SIGTERM'); } catch { /* ignore */ }
            try { child.kill('SIGKILL'); } catch { /* ignore */ }
          }
        } catch { /* ignore */ }
        try { child.disconnect(); } catch { /* ignore */ }
        try { child.unref(); } catch { /* ignore */ }
        const stdioStreams = Array.isArray(child.stdio) ? child.stdio : [];
        stdioStreams.forEach((stream) => {
          if (stream === null || stream === undefined) return;
          const destroyStream = (stream as { destroy?: () => void }).destroy;
          if (typeof destroyStream === 'function') {
            try { destroyStream.call(stream); } catch { /* ignore */ }
          }
          const endStream = (stream as { end?: () => void }).end;
          if (typeof endStream === 'function') {
            try { endStream.call(stream); } catch { /* ignore */ }
          }
          const unrefStream = (stream as { unref?: () => void }).unref;
          if (typeof unrefStream === 'function') {
            try { unrefStream.call(stream); } catch { /* ignore */ }
          }
        });
        return;
      }
      const fd = (handle as { fd?: unknown }).fd;
      if (typeof fd === 'number' && (fd === 0 || fd === 1 || fd === 2)) {
        return;
      }
      const destroy = (handle as { destroy?: () => void }).destroy;
      if (typeof destroy === 'function') {
        try { destroy.call(handle); } catch { /* ignore */ }
      }
      const end = (handle as { end?: () => void }).end;
      if (typeof end === 'function') {
        try { end.call(handle); } catch { /* ignore */ }
      }
      const close = (handle as { close?: () => void }).close;
      if (typeof close === 'function') {
        try { close.call(handle); } catch { /* ignore */ }
      }
      const unref = (handle as { unref?: () => void }).unref;
      if (typeof unref === 'function') {
        try { unref.call(handle); } catch { /* ignore */ }
      }
    });
    await Promise.all(childCleanup);
    await delay(50);
    activeHandles = collectHandles();
  }

  const remaining = activeHandles.filter((handle) => !shouldIgnore(handle));
  if (remaining.length === 0) return;

  const labels = remaining.map(formatHandle);
  console.error(`[warn] lingering handles after cleanup: ${labels.join(', ')}`);
}

/**
 * Reset state between test batches.
 */
export async function resetStateBetweenBatches(): Promise<void> {
  await cleanupActiveHandles();
  queueManager.reset();
  await shutdownSharedRegistry();
  // Longer delay to let MCP server processes fully terminate and release resources
  await new Promise((resolve) => { setTimeout(resolve, 500); });
}

/**
 * Reset state between sequential tests.
 */
export async function resetStateBetweenSequentialTests(): Promise<void> {
  await cleanupActiveHandles();
  queueManager.reset();
  await shutdownSharedRegistry();
  await delay(50);
}
