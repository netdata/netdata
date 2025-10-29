import type { SessionNode } from '../session-tree.js';
import type { MCPTool } from '../types.js';

export type ToolKind = 'mcp' | 'rest' | 'agent';

export interface ToolExecuteOptions {
  timeoutMs?: number;
  slot?: number;
  // When true, do not acquire/release a concurrency slot in the orchestrator
  // Useful for control-plane tools like agent__batch to avoid deadlocks.
  bypassConcurrency?: boolean;
  // When true, skip the orchestrator-level timeout wrapper (provider handles timing internally).
  disableGlobalTimeout?: boolean;
  // When true, provider should emit detailed trace logs
  trace?: boolean;
  // For sub-agents: stream live child opTree snapshots during execution
  onChildOpTree?: (tree: SessionNode) => void;
  // Parent op path label (e.g., 1.2). Used to prefix child log entry.path for hierarchical greppability
  parentOpPath?: string;
  // Parent execution context (turn/subturn) for hierarchical numbering
  parentContext?: ToolExecutionContext;
}

export interface ToolExecuteResult {
  ok: boolean;
  result?: string;
  error?: string;
  latencyMs: number;
  kind: ToolKind;
  namespace: string; // e.g., mcp server namespace, 'rest', 'subagent'
  // Optional extras for providers that produce rich results (e.g., sub-agents)
  extras?: Record<string, unknown>;
}

export abstract class ToolProvider {
  abstract readonly kind: ToolKind;
  abstract readonly namespace: string; // provider namespace (e.g., 'rest', server namespace)
  abstract listTools(): MCPTool[];
  abstract hasTool(name: string): boolean;
  abstract execute(name: string, parameters: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult>;
  // Optional warmup hook for providers that need async initialization (e.g., MCP)
  async warmup(): Promise<void> { /* default no-op */ }
  getInstructions(): string { return ''; }
  resolveLogProvider(_name: string): string {
    return this.namespace;
  }
  resolveToolIdentity(name: string): { namespace: string; tool: string } {
    return { namespace: this.namespace, tool: name };
  }
}

export interface ToolExecutionContext {
  turn: number;
  subturn: number;
}
