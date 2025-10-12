import type { SessionNode } from '../session-tree.js';
import type { SubAgentRegistry } from '../subagent-registry.js';
import type { MCPTool } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';

import { ToolProvider } from './types.js';

type ExecFn = (name: string, parameters: Record<string, unknown>, opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string }) => Promise<{
  result: string;
  // Optional extras for parent to inspect/record
  childAccounting?: readonly unknown[];
  childConversation?: unknown;
  childOpTree?: unknown;
}>;

export class AgentProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  constructor(public readonly id: string, private readonly agents: SubAgentRegistry, private readonly execFn: ExecFn) { super(); }

  listTools(): MCPTool[] { return this.agents.getTools(); }
  hasTool(name: string): boolean { return this.agents.hasTool(name); }

  async execute(name: string, parameters: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    const out = await this.execFn(name, parameters, { onChildOpTree: _opts?.onChildOpTree, parentOpPath: _opts?.parentOpPath });
    const latency = Date.now() - start;
    return { ok: true, result: out.result, latencyMs: latency, kind: this.kind, providerId: this.id, extras: { childAccounting: out.childAccounting, childConversation: out.childConversation, childOpTree: out.childOpTree } };
  }
}
