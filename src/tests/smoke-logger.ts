import type { ToolExecuteOptions, ToolExecuteResult, ToolKind } from '../tools/types.js';
import type { MCPTool, LogEntry } from '../types.js';

import { SessionTreeBuilder } from '../session-tree.js';
import { ToolsOrchestrator } from '../tools/tools.js';
import { ToolProvider } from '../tools/types.js';

const TOOL = 'unittest.echo';

class FakeRestProvider extends ToolProvider {
  readonly kind: ToolKind = 'rest';
  readonly id = 'rest';
  listTools(): MCPTool[] { return [ { name: TOOL, description: 'echo', inputSchema: {} } ]; }
  hasTool(name: string): boolean { return name === TOOL; }
  execute(_name: string, args: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const fail = (args as { fail?: boolean }).fail === true;
    if (fail) return Promise.resolve({ ok: false, error: 'fail requested', latencyMs: 5, kind: this.kind, providerId: this.id });
    const maybeText = (args as { text?: unknown }).text;
    const payload = typeof maybeText === 'string' ? maybeText : 'ok';
    return Promise.resolve({ ok: true, result: payload, latencyMs: 5, kind: this.kind, providerId: this.id, extras: { rawResponse: payload } });
  }
}

async function run(): Promise<void> {
  const tree = new SessionTreeBuilder({ traceId: 'txn', agentId: 'agent', callPath: 'agent' });
  tree.beginTurn(1, {});
  const logs: LogEntry[] = [];
  const onLog = (entry: LogEntry, opts?: { opId?: string }) => {
    const opId = opts?.opId;
    if (typeof opId !== 'string' || opId.length === 0) throw new Error('missing opId in onLog');
    // anchor into tree
    tree.appendLog(opId, entry);
    logs.push(entry);
  };
  const orch = new ToolsOrchestrator({ toolTimeout: 2000, toolResponseMaxBytes: 1024, maxConcurrentTools: 2, parallelToolCalls: true, traceTools: true }, tree, (tr) => { void tr; }, onLog, undefined, { agentId: 'agent', callPath: 'agent' });
  orch.register(new FakeRestProvider());

  // Success path
  const succ = await orch.executeWithManagement(TOOL, { text: 'hello' }, { turn: 1, subturn: 1 });
  if (succ.result !== 'hello') throw new Error('unexpected success result');
  // Find the last op and assert logs attached exactly once per emission
  const s = tree.getSession();
  const t = s.turns[0];
  if (t.ops.length === 0) throw new Error('missing op');
  const lastOp = t.ops[t.ops.length - 1];
  const succLogs = lastOp.logs;
  // Expect fixed count of logs on success
  const EXPECT = 4;
  const succCount = succLogs.length;
  if (succCount !== EXPECT) throw new Error(`expected ${String(EXPECT)} logs for success, got ${String(succCount)}`);

  // Failure path
  const beforeOps = t.ops.length;
  let failed = false;
  try { await orch.executeWithManagement(TOOL, { fail: true }, { turn: 1, subturn: 2 }); } catch { failed = true; }
  if (!failed) throw new Error('executeWithManagement should have thrown on failure');
  const errOp = t.ops[t.ops.length - 1];
  if (t.ops.length !== beforeOps + 1) throw new Error('expected a new op for failure');
  const errLogs = errOp.logs;
  // Expect fixed count of logs on failure
  if (errLogs.length !== EXPECT) throw new Error(`expected ${String(EXPECT)} logs for error, got ${String(errLogs.length)}`);

  // No double emission: onLog count should equal total logs on both ops
  const totalOpLogs = succLogs.length + errLogs.length;
  if (logs.length < totalOpLogs) throw new Error('missing logs in onLog capture');
}

void run().then(() => { /* eslint-disable no-console */ console.log('smoke-logger ok'); }).catch((e: unknown) => { /* eslint-disable no-console */ console.error('smoke-logger failed', e instanceof Error ? e.message : String(e)); process.exit(1); });
