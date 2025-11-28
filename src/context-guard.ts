/**
 * Context Guard - Manages context window budget for LLM conversations
 *
 * Tracks token usage, evaluates budget constraints, and enforces
 * final turn when context limits are approached.
 */
import { Mutex } from 'async-mutex';

import type { ToolBudgetCallbacks, ToolBudgetReservation } from './tools/tools.js';
import type { ConversationMessage, TurnRequestContextMetrics } from './types.js';

import { estimateMessagesTokens, resolveTokenizer } from './tokenizer-registry.js';
import { estimateMessagesBytes } from './utils.js';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TargetContextConfig {
  provider: string;
  model: string;
  contextWindow: number;
  tokenizerId?: string;
  bufferTokens: number;
}

export interface ContextGuardBlockedEntry {
  provider: string;
  model: string;
  contextWindow: number;
  bufferTokens: number;
  maxOutputTokens: number;
  limit: number;
  projected: number;
}

export interface ContextGuardEvaluation {
  blocked: ContextGuardBlockedEntry[];
  projectedTokens: number;
}

export interface ContextGuardCallbacks {
  /** Called when context guard forces a final turn */
  onForcedFinalTurn?: (blocked: ContextGuardBlockedEntry[], trigger: 'tool_preflight' | 'turn_preflight') => void;
  /** Debug logging callback */
  onDebug?: (label: string, data: Record<string, unknown>) => void;
}

export interface ContextGuardConfig {
  targets: TargetContextConfig[];
  defaultContextWindow: number;
  defaultBufferTokens: number;
  maxOutputTokens?: number;
  callbacks?: ContextGuardCallbacks;
}

// ---------------------------------------------------------------------------
// ContextGuard Class
// ---------------------------------------------------------------------------

export class ContextGuard {
  static readonly DEFAULT_CONTEXT_WINDOW_TOKENS = 131072;
  static readonly DEFAULT_CONTEXT_BUFFER_TOKENS = 256;

  private readonly targets: TargetContextConfig[];
  private readonly defaultContextWindow: number;
  private readonly defaultBufferTokens: number;
  private readonly configuredMaxOutputTokens?: number;
  private readonly callbacks?: ContextGuardCallbacks;
  private readonly mutex = new Mutex();

  // Token tracking state
  private currentCtxTokens = 0;
  private pendingCtxTokens = 0;
  private newCtxTokens = 0;
  private schemaCtxTokens = 0;

  // Budget state
  private toolBudgetExceeded = false;
  private forcedFinalTurnReason?: 'context';
  private contextLimitWarningLogged = false;

  constructor(config: ContextGuardConfig) {
    this.targets = config.targets.length > 0 ? config.targets : [];
    this.defaultContextWindow = config.defaultContextWindow;
    this.defaultBufferTokens = config.defaultBufferTokens;
    this.configuredMaxOutputTokens = config.maxOutputTokens;
    this.callbacks = config.callbacks;

    this.debug('init-counters', {
      currentCtxTokens: this.currentCtxTokens,
      pendingCtxTokens: this.pendingCtxTokens,
      newCtxTokens: this.newCtxTokens,
      schemaCtxTokens: this.schemaCtxTokens,
    });
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Get the effective target configurations, falling back to defaults if none configured
   */
  getTargets(): TargetContextConfig[] {
    if (this.targets.length > 0) {
      return this.targets;
    }
    return [{
      provider: 'default',
      model: 'default',
      contextWindow: this.defaultContextWindow,
      tokenizerId: undefined,
      bufferTokens: this.defaultBufferTokens,
    }];
  }

  /**
   * Estimate token count for messages across all configured targets
   */
  estimateTokens(messages: ConversationMessage[]): number {
    if (messages.length === 0) {
      return 0;
    }
    let maxTokens = 0;
    const targets = this.getTargets();
    // eslint-disable-next-line functional/no-loop-statements
    for (const target of targets) {
      const tokenizer = resolveTokenizer(target.tokenizerId);
      const tokens = estimateMessagesTokens(tokenizer, messages);
      if (tokens > maxTokens) {
        maxTokens = tokens;
      }
    }
    const approxTokens = Math.ceil(estimateMessagesBytes(messages) / 4);
    return Math.max(maxTokens, approxTokens);
  }

  /**
   * Estimate token count for tool schemas
   */
  estimateToolSchemaTokens(tools: readonly { name: string; description?: string; instructions?: string; inputSchema?: unknown }[]): number {
    if (tools.length === 0) {
      return 0;
    }
    const serialized = tools.map((tool) => {
      const payload = {
        name: tool.name,
        description: tool.description,
        instructions: tool.instructions ?? '',
        inputSchema: tool.inputSchema,
      };
      return {
        role: 'system' as const,
        content: JSON.stringify(payload),
      } satisfies ConversationMessage;
    });
    const estimated = this.estimateTokens(serialized);
    this.debug('schema-estimate', { toolCount: tools.length, estimated });
    return estimated;
  }

  /**
   * Compute max output tokens for a given context window
   */
  computeMaxOutputTokens(contextWindow: number): number {
    if (this.configuredMaxOutputTokens !== undefined) {
      return this.configuredMaxOutputTokens;
    }
    return Math.max(0, Math.floor(contextWindow / 4));
  }

  /**
   * Evaluate if context budget is exceeded
   */
  evaluate(extraTokens = 0): ContextGuardEvaluation {
    const projectedTokens = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens + extraTokens;
    const blocked: ContextGuardBlockedEntry[] = [];
    const targets = this.getTargets();

    this.debug('evaluate', {
      projectedTokens,
      currentCtxTokens: this.currentCtxTokens,
      pendingCtxTokens: this.pendingCtxTokens,
      newCtxTokens: this.newCtxTokens,
      extraTokens,
    });

    // eslint-disable-next-line functional/no-loop-statements
    for (const target of targets) {
      const contextWindow = target.contextWindow;
      const bufferTokens = target.bufferTokens;
      const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
      const limit = Math.max(0, contextWindow - bufferTokens - maxOutputTokens);

      this.debug('evaluate-target', {
        provider: target.provider,
        model: target.model,
        contextWindow,
        bufferTokens,
        maxOutputTokens,
        limit,
      });

      if (limit > 0 && projectedTokens > limit) {
        blocked.push({
          provider: target.provider,
          model: target.model,
          contextWindow,
          bufferTokens,
          maxOutputTokens,
          limit,
          projected: projectedTokens,
        });
      }
    }

    return { blocked, projectedTokens };
  }

  /**
   * Evaluate context for a specific provider/model
   */
  evaluateForProvider(provider: string, model: string): 'ok' | 'skip' | 'final' {
    const targets = this.getTargets();
    const evaluation = this.evaluate(this.schemaCtxTokens);

    this.debug('provider-eval', {
      provider,
      model,
      blocked: evaluation.blocked.map((entry) => ({
        provider: entry.provider,
        model: entry.model,
        limit: entry.limit,
        projected: entry.projected,
      })),
    });

    if (evaluation.blocked.length === 0) {
      return 'ok';
    }

    const blockedKeys = new Set(evaluation.blocked.map((entry) => `${entry.provider}:${entry.model}`));
    const currentKey = `${provider}:${model}`;

    if (blockedKeys.size >= targets.length) {
      return 'final';
    }
    if (blockedKeys.has(currentKey)) {
      return 'skip';
    }
    if (targets.length === 1 && targets[0].provider === 'default') {
      return 'final';
    }
    return 'ok';
  }

  /**
   * Build context metrics for a turn request
   */
  buildMetrics(provider: string, model: string): TurnRequestContextMetrics {
    const targets = this.getTargets();
    const target = targets.find((cfg) => cfg.provider === provider && cfg.model === model) ?? targets[0];
    const contextWindow = target.contextWindow;
    const bufferTokens = target.bufferTokens;
    const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
    const ctxTokens = this.currentCtxTokens;
    const newTokens = this.pendingCtxTokens;
    const schemaTokens = this.schemaCtxTokens;
    const expectedTokens = ctxTokens + newTokens + schemaTokens;
    const expectedPct = contextWindow > 0
      ? Math.round(((expectedTokens + bufferTokens + maxOutputTokens) / contextWindow) * 100)
      : undefined;

    this.debug('build-metrics', {
      provider,
      model,
      ctxTokens,
      pendingCtxTokens: this.pendingCtxTokens,
      schemaCtxTokens: this.schemaCtxTokens,
      newTokens,
      schemaTokens,
      expectedTokens,
    });

    return {
      ctxTokens,
      newTokens,
      schemaTokens,
      expectedTokens,
      expectedPct,
      contextWindow,
      bufferTokens,
      maxOutputTokens,
    };
  }

  /**
   * Enforce final turn due to context limits
   */
  enforceFinalTurn(blocked: ContextGuardBlockedEntry[], trigger: 'tool_preflight' | 'turn_preflight'): void {
    this.debug('enforce', {
      trigger,
      blocked: blocked.map((entry) => ({
        provider: entry.provider,
        model: entry.model,
        limit: entry.limit,
        projected: entry.projected,
      })),
    });

    if (this.forcedFinalTurnReason === undefined) {
      this.forcedFinalTurnReason = 'context';
      this.contextLimitWarningLogged = false;
    }

    // Commit pending tokens
    if (this.pendingCtxTokens > 0) {
      this.currentCtxTokens += this.pendingCtxTokens;
      this.pendingCtxTokens = 0;
    }
    this.newCtxTokens = 0;

    // Notify callback
    this.callbacks?.onForcedFinalTurn?.(blocked, trigger);
  }

  /**
   * Create tool budget callbacks for the tools orchestrator
   */
  createToolBudgetCallbacks(): ToolBudgetCallbacks {
    return {
      reserveToolOutput: async (output: string): Promise<ToolBudgetReservation> => {
        return await this.mutex.runExclusive(() => {
          const tokens = this.estimateTokens([{ role: 'tool', content: output }]);
          const guard = this.evaluate(tokens);
          if (guard.blocked.length > 0) {
            if (!this.toolBudgetExceeded) {
              this.toolBudgetExceeded = true;
              this.enforceFinalTurn(guard.blocked, 'tool_preflight');
            }
            return { ok: false as const, tokens, reason: 'token_budget_exceeded' };
          }
          return { ok: true as const, tokens };
        });
      },
      canExecuteTool: () => !this.toolBudgetExceeded,
    };
  }

  // ---------------------------------------------------------------------------
  // Token Counter Management
  // ---------------------------------------------------------------------------

  /** Get current context tokens */
  getCurrentTokens(): number {
    return this.currentCtxTokens;
  }

  /** Get pending context tokens */
  getPendingTokens(): number {
    return this.pendingCtxTokens;
  }

  /** Get new context tokens accumulated this turn */
  getNewTokens(): number {
    return this.newCtxTokens;
  }

  /** Get schema context tokens */
  getSchemaTokens(): number {
    return this.schemaCtxTokens;
  }

  /** Set current context tokens */
  setCurrentTokens(tokens: number): void {
    this.currentCtxTokens = tokens;
  }

  /** Set pending context tokens */
  setPendingTokens(tokens: number): void {
    this.pendingCtxTokens = tokens;
  }

  /** Add new context tokens */
  addNewTokens(tokens: number): void {
    this.newCtxTokens += tokens;
  }

  /** Set new context tokens (absolute value) */
  setNewTokens(tokens: number): void {
    this.newCtxTokens = tokens;
  }

  /** Set schema context tokens */
  setSchemaTokens(tokens: number): void {
    this.schemaCtxTokens = tokens;
  }

  /** Reset new tokens counter (typically at turn end) */
  resetNewTokens(): void {
    this.newCtxTokens = 0;
  }

  /** Commit pending tokens to current */
  commitPendingTokens(): void {
    if (this.pendingCtxTokens > 0) {
      this.currentCtxTokens += this.pendingCtxTokens;
      this.pendingCtxTokens = 0;
    }
  }

  // ---------------------------------------------------------------------------
  // State Queries
  // ---------------------------------------------------------------------------

  /** Check if tool budget has been exceeded */
  isToolBudgetExceeded(): boolean {
    return this.toolBudgetExceeded;
  }

  /** Get the forced final turn reason, if any */
  getForcedFinalReason(): 'context' | undefined {
    return this.forcedFinalTurnReason;
  }

  /** Check if context limit warning has been logged */
  hasLoggedContextWarning(): boolean {
    return this.contextLimitWarningLogged;
  }

  /** Mark that context limit warning has been logged */
  markContextWarningLogged(): void {
    this.contextLimitWarningLogged = true;
  }

  /** Reset tool budget exceeded flag (for testing) */
  resetToolBudgetExceeded(): void {
    this.toolBudgetExceeded = false;
  }

  // ---------------------------------------------------------------------------
  // Private Helpers
  // ---------------------------------------------------------------------------

  private debug(label: string, data: Record<string, unknown>): void {
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log(`context-guard/${label}`, data);
    }
    this.callbacks?.onDebug?.(`context-guard/${label}`, data);
  }
}
