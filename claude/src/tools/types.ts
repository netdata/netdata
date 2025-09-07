import type { MCPTool } from '../types.js';

export type ToolKind = 'mcp' | 'rest' | 'agent';

export interface ToolExecuteOptions {
  timeoutMs?: number;
  slot?: number;
  // When true, do not acquire/release a concurrency slot in the orchestrator
  // Useful for control-plane tools like agent__batch to avoid deadlocks.
  bypassConcurrency?: boolean;
}

export interface ToolExecuteResult {
  ok: boolean;
  result?: string;
  error?: string;
  latencyMs: number;
  kind: ToolKind;
  providerId: string; // e.g., mcp server name, 'rest', 'subagent'
  // Optional extras for providers that produce rich results (e.g., sub-agents)
  extras?: Record<string, unknown>;
}

export abstract class ToolProvider {
  abstract readonly kind: ToolKind;
  abstract readonly id: string; // provider instance id (e.g., 'rest', server name)
  abstract listTools(): MCPTool[];
  abstract hasTool(name: string): boolean;
  abstract execute(name: string, args: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult>;
}

export interface ToolExecutionContext {
  turn: number;
  subturn: number;
}
