import type { ExecutionTree } from './execution-tree.js';
import type { ToolsOrchestrator } from './tools/tools.js';
import type { LogEntry } from './types.js';

export function logSettings(tree: ExecutionTree, turn: number, settings: Record<string, unknown>): void {
  const entry: LogEntry = { timestamp: Date.now(), severity: 'VRB', turn, subturn: 0, direction: 'request', type: 'llm', remoteIdentifier: 'agent:settings', fatal: false, message: JSON.stringify(settings) };
  tree.recordLog(entry);
}

export function logToolsBanner(tree: ExecutionTree, turn: number, orch: ToolsOrchestrator): void {
  const tools = orch.listTools().map((t) => t.name);
  const rest = tools.filter((n) => n.startsWith('rest__'));
  const ag = tools.filter((n) => n.startsWith('agent__'));
  const mcp = tools.filter((n) => !n.startsWith('rest__') && !n.startsWith('agent__'));
  const entry: LogEntry = { timestamp: Date.now(), severity: 'VRB', turn, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:tools', fatal: false, message: `tools: mcp=${String(mcp.length)} [${mcp.join(', ')}]; rest=${String(rest.length)} [${rest.join(', ')}]; subagents=${String(ag.length)} [${ag.join(', ')}]` };
  tree.recordLog(entry);
}
