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
  static readonly DEFAULT_CONTEXT_BUFFER_TOKENS = 8192;

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
  private forcedFinalTurnReason?: 'context' | 'max_turns' | 'task_status_completed' | 'retry_exhaustion';
  private contextLimitWarningLogged = false;

  // Current reasoning/thinking budget tokens (for extended thinking models)
  private currentReasoningTokens = 0;

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
    let filtered = tools.filter((tool) => tool.name !== 'agent__task_status' && tool.name !== 'agent__final_report');
    if (filtered.length === 0) {
      filtered = [...tools];
    }
    if (filtered.length === 0) {
      return 0;
    }
    const serialized = filtered.map((tool) => {
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
    const scaled = Math.ceil(estimated * 2.09);
    this.debug('schema-estimate', { toolCount: filtered.length, estimated: scaled });
    return scaled;
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
    // Schema is in currentCtxTokens after first turn (via cache_write/cache_read)
    // First turn will be slightly underestimated, subsequent turns accurate
    const projectedTokens = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens + extraTokens;
    const blocked: ContextGuardBlockedEntry[] = [];
    const targets = this.getTargets();

    this.debug('evaluate', {
      projectedTokens,
      currentCtxTokens: this.currentCtxTokens,
      pendingCtxTokens: this.pendingCtxTokens,
      newCtxTokens: this.newCtxTokens,
      schemaCtxTokens: this.schemaCtxTokens,
      extraTokens,
      currentReasoningTokens: this.currentReasoningTokens,
    });

    // eslint-disable-next-line functional/no-loop-statements
    for (const target of targets) {
      const contextWindow = target.contextWindow;
      const bufferTokens = target.bufferTokens;
      const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
      // Reasoning tokens are INSIDE max_output_tokens, not on top
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
    // Schema tokens are now included in evaluate() automatically
    const evaluation = this.evaluate();

    // If the only overflow comes from schema tokens, allow the request (never block)
    const target = targets.find((cfg) => cfg.provider === provider && cfg.model === model) ?? targets[0];
    const maxOutputTokens = this.computeMaxOutputTokens(target.contextWindow);
    // Reasoning tokens are INSIDE max_output_tokens, not on top
    const limit = Math.max(0, target.contextWindow - target.bufferTokens - maxOutputTokens);
    const baseProjected = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens;
    if (limit > 0 && baseProjected <= limit) {
      const blockedCurrent = evaluation.blocked.find((entry) => entry.provider === provider && entry.model === model);
      if (blockedCurrent !== undefined) {
        return 'ok';
      }
    }

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
   * @param reasoningValue - Optional reasoning/thinking budget tokens (for extended thinking models)
   */
  buildMetrics(provider: string, model: string, reasoningValue?: number | string | null): TurnRequestContextMetrics {
    const targets = this.getTargets();
    const target = targets.find((cfg) => cfg.provider === provider && cfg.model === model) ?? targets[0];
    const contextWindow = target.contextWindow;
    const bufferTokens = target.bufferTokens;
    const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
    const ctxTokens = this.currentCtxTokens;
    const newTokens = this.pendingCtxTokens;
    const schemaTokens = this.schemaCtxTokens;
    // Note: ctxTokens already includes schema from previous turns (tracked as total context)
    // For next turn, we add newTokens (new user input) but schema is already counted
    // in ctxTokens via cache_read/cache_write
    const expectedTokens = ctxTokens + newTokens;

    // Calculate reasoning tokens from the reasoning value (for display/logging only)
    const reasoningTokens = this.parseReasoningTokens(reasoningValue);

    // Reasoning tokens are INSIDE max_output_tokens, not on top
    // Schema is in ctx after first turn (via cache), so don't add separately
    // First turn will be slightly underestimated, subsequent turns accurate
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
      reasoningTokens,
      maxOutputTokens,
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
      reasoningTokens,
    };
  }

  /**
   * Parse reasoning value to extract token count
   */
  private parseReasoningTokens(reasoningValue?: number | string | null): number {
    if (reasoningValue === null || reasoningValue === undefined) {
      return 0;
    }
    if (typeof reasoningValue === 'number' && Number.isFinite(reasoningValue)) {
      return Math.max(0, Math.trunc(reasoningValue));
    }
    if (typeof reasoningValue === 'string') {
      const parsed = Number(reasoningValue.trim());
      if (Number.isFinite(parsed)) {
        return Math.max(0, Math.trunc(parsed));
      }
    }
    return 0;
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
            // Calculate available tokens: use the minimum limit across all blocked targets
            const baseProjected = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens;
            const minLimit = Math.min(...guard.blocked.map((entry) => entry.limit));
            const availableTokens = Math.max(0, minLimit - baseProjected);
            return { ok: false as const, tokens, reason: 'token_budget_exceeded', availableTokens };
          }
          // Accumulate tokens so subsequent tool reservations see the updated context
          this.newCtxTokens += tokens;
          return { ok: true as const, tokens };
        });
      },
      canExecuteTool: () => !this.toolBudgetExceeded,
      countTokens: (text: string): number => {
        // Use the same tokenizer logic as estimateTokens for consistency
        let maxTokens = 0;
        const targets = this.getTargets();
        // eslint-disable-next-line functional/no-loop-statements
        for (const target of targets) {
          const tokenizer = resolveTokenizer(target.tokenizerId);
          const tokens = tokenizer.countText(text);
          if (tokens > maxTokens) {
            maxTokens = tokens;
          }
        }
        const approxTokens = Math.ceil(text.length / 4);
        return Math.max(maxTokens, approxTokens);
      },
    };
  }

  /**
   * Get available token budget for schema (tools).
   * Used to decide whether to use full tools or shrink to final-only before the request.
   */
  getAvailableSchemaBudget(provider: string, model: string): number {
    const targets = this.getTargets();
    const target = targets.find((cfg) => cfg.provider === provider && cfg.model === model) ?? targets[0];
    const contextWindow = target.contextWindow;
    const bufferTokens = target.bufferTokens;
    const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
    // Reasoning tokens are INSIDE max_output_tokens, not on top
    const limit = Math.max(0, contextWindow - bufferTokens - maxOutputTokens);
    const baseProjected = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens;
    return Math.max(0, limit - baseProjected);
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

  /** Get current reasoning/thinking budget tokens */
  getReasoningTokens(): number {
    return this.currentReasoningTokens;
  }

  /**
   * Set reasoning/thinking budget tokens for context evaluation.
   * Should be called at the start of each turn with the effective reasoning value.
   */
  setReasoningTokens(value?: number | string | null): void {
    this.currentReasoningTokens = this.parseReasoningTokens(value);
  }

  // ---------------------------------------------------------------------------
  // State Queries
  // ---------------------------------------------------------------------------

  /** Check if tool budget has been exceeded */
  isToolBudgetExceeded(): boolean {
    return this.toolBudgetExceeded;
  }

  /** Get the forced final turn reason, if any */
  getForcedFinalReason(): 'context' | 'max_turns' | 'task_status_completed' | 'retry_exhaustion' | undefined {
    return this.forcedFinalTurnReason;
  }

  /** Set forced final turn reason for task completion */
  setTaskCompletionReason(): void {
    this.forcedFinalTurnReason = 'task_status_completed';
  }

  /** Set forced final turn reason for retry exhaustion */
  setRetryExhaustedReason(): void {
    // Don't overwrite explicit task completion signals
    if (this.forcedFinalTurnReason === 'task_status_completed') {
      return;
    }
    this.forcedFinalTurnReason = 'retry_exhaustion';
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
