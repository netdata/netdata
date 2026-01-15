import type { SessionNode } from '../session-tree.js';

export type { ToolBudgetCallbacks, ToolBudgetReservation } from '../tools/tools.js';

export type ToolOutputMode = 'auto' | 'full-chunked' | 'read-grep' | 'truncate';

export interface ToolOutputConfigInput {
  enabled?: boolean;
  storeDir?: string;
  maxChunks?: number;
  overlapPercent?: number;
  avgLineBytesThreshold?: number;
  models?: string | string[];
}

export interface ToolOutputConfig {
  enabled: boolean;
  storeDir: string;
  maxChunks: number;
  overlapPercent: number;
  avgLineBytesThreshold: number;
  models?: { provider: string; model: string }[];
}

export interface ToolOutputStats {
  bytes: number;
  lines: number;
  tokens: number;
  avgLineBytes: number;
}

export interface ToolOutputEntry {
  handle: string;
  toolName: string;
  toolArgsJson: string;
  bytes: number;
  lines: number;
  tokens: number;
  createdAt: number;
  path: string;
  sourceTarget?: { provider: string; model: string };
}

export interface ToolOutputHandleResult {
  handle: string;
  message: string;
  stats: ToolOutputStats;
  entry: ToolOutputEntry;
  reason: 'size_cap' | 'token_budget' | 'reserve_failed';
}

export interface ToolOutputTarget {
  provider: string;
  model: string;
}

export interface ToolOutputExtractionResult {
  ok: boolean;
  text: string;
  mode: Exclude<ToolOutputMode, 'auto'>;
  childOpTree?: SessionNode;
  warning?: string;
}
