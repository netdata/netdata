import type { SessionNode } from '../session-tree.js';
import type { SubAgentRegistry } from '../subagent-registry.js';
import type { MCPTool } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult, ToolExecutionContext } from './types.js';

import { ToolProvider } from './types.js';

type ExecFn = (name: string, parameters: Record<string, unknown>, opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string; parentContext?: ToolExecutionContext }) => Promise<{
  result: string;
  // Optional extras for parent to inspect/record
  childAccounting?: readonly unknown[];
  childConversation?: unknown;
  childOpTree?: unknown;
}>;

export class AgentProvider extends ToolProvider {
  readonly kind = 'agent' as const;
  constructor(public readonly namespace: string, private readonly agents: SubAgentRegistry, private readonly execFn: ExecFn) { super(); }

  listTools(): MCPTool[] { return this.agents.getTools(); }
  hasTool(name: string): boolean { return this.agents.hasTool(name); }

  override resolveToolIdentity(name: string): { namespace: string; tool: string } {
    const tool = name.startsWith('agent__') ? name.slice('agent__'.length) : name;
    return { namespace: this.namespace, tool };
  }

  async execute(name: string, parameters: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    const out = await this.execFn(name, parameters, {
      onChildOpTree: _opts?.onChildOpTree,
      parentOpPath: _opts?.parentOpPath,
      parentContext: _opts?.parentContext,
    });
    const latency = Date.now() - start;
    return { ok: true, result: out.result, latencyMs: latency, kind: this.kind, namespace: this.namespace, extras: { childAccounting: out.childAccounting, childConversation: out.childConversation, childOpTree: out.childOpTree } };
  }
}
