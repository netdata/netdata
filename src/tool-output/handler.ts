import type { ToolOutputStore } from './store.js';
import type { ToolBudgetCallbacks, ToolBudgetReservation, ToolOutputConfig, ToolOutputHandleResult, ToolOutputTarget } from './types.js';

import { sanitizeTextForLLM } from '../utils.js';

import { formatHandleMessage } from './formatter.js';
import { computeToolOutputStats } from './stats.js';

interface ToolOutputHandleRequest {
  toolName: string;
  toolArgsJson: string;
  output: string;
  sizeLimitBytes?: number;
  budgetCallbacks?: ToolBudgetCallbacks;
  sourceTarget?: ToolOutputTarget;
  forceReason?: 'reserve_failed';
}

export class ToolOutputHandler {
  constructor(
    private readonly store: ToolOutputStore,
    private readonly config: ToolOutputConfig,
  ) {}

  async maybeStore(req: ToolOutputHandleRequest): Promise<ToolOutputHandleResult | undefined> {
    if (!this.config.enabled) return undefined;
    const sanitized = sanitizeTextForLLM(req.output);
    const countTokens = req.budgetCallbacks?.countTokens ?? ((text: string) => Math.ceil(text.length / 4));
    const stats = computeToolOutputStats(sanitized, countTokens);
    const sizeExceeded = typeof req.sizeLimitBytes === 'number' && req.sizeLimitBytes > 0 && stats.bytes > req.sizeLimitBytes;
    const preview = await this.previewBudget(req.budgetCallbacks, sanitized);
    const budgetExceeded = preview !== undefined && !preview.ok;
    const forced = req.forceReason === 'reserve_failed';
    if (!sizeExceeded && !budgetExceeded && !forced) return undefined;

    const entry = await this.store.store({
      toolName: req.toolName,
      toolArgsJson: req.toolArgsJson,
      content: sanitized,
      stats,
      sourceTarget: req.sourceTarget,
    });
    const message = formatHandleMessage(entry.handle, stats);
    const reason: ToolOutputHandleResult['reason'] = forced
      ? 'reserve_failed'
      : sizeExceeded
        ? 'size_cap'
        : 'token_budget';
    return {
      handle: entry.handle,
      message,
      stats,
      entry,
      reason,
    };
  }

  private async previewBudget(
    callbacks: ToolBudgetCallbacks | undefined,
    payload: string,
  ): Promise<ToolBudgetReservation | undefined> {
    return await callbacks?.previewToolOutput?.(payload);
  }
}
