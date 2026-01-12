import crypto from 'node:crypto';

import type { OutputFormatId } from '../formats.js';
import type { SessionNode } from '../session-tree.js';
import type { SubAgentRegistry } from '../subagent-registry.js';
import type {
  AIAgentSessionConfig,
  ConversationMessage,
  AccountingEntry,
  AIAgentResult,
  OrchestrationRuntimeAgent,
} from '../types.js';

import { appendCallPathSegment } from '../utils.js';

export interface SpawnChildOptions {
  parentSession: Pick<
    AIAgentSessionConfig,
    | "config"
    | "callbacks"
    | "stream"
    | "traceLLM"
    | "traceMCP"
    | "traceSdk"
    | "verbose"
    | "temperature"
    | "topP"
    | "topK"
    | "llmTimeout"
    | "toolTimeout"
    | "maxRetries"
    | "maxTurns"
    | "toolResponseMaxBytes"
    | "targets"
    | "tools"
  > & {
    trace?: {
      selfId?: string;
      originId?: string;
      parentId?: string;
      callPath?: string;
      agentPath?: string;
      turnPath?: string;
    };
    abortSignal?: AbortSignal;
    stopRef?: { stopping: boolean };
    onChildOpTree?: (tree: SessionNode) => void;
    agentPath?: string;
    turnPathPrefix?: string;
  };
  parentOpPath?: string;
  parentTurnPath?: string;
}

export async function spawnChildAgent(
  registry: SubAgentRegistry,
  exposedToolName: string,
  parameters: Record<string, unknown>,
  opts: SpawnChildOptions,
): Promise<{
  result: string;
  conversation: ConversationMessage[];
  accounting: readonly AccountingEntry[];
  opTree?: SessionNode;
}> {
  const result = await registry.execute(
    exposedToolName,
    parameters,
    opts.parentSession,
    {
      parentOpPath: opts.parentOpPath,
      parentTurnPath: opts.parentTurnPath,
    },
  );
  return {
    result: result.result,
    conversation: result.conversation,
    accounting: result.accounting,
    opTree: result.opTree,
  };
}

export function createChildToolName(agentName: string): string {
  return `agent__${agentName}`;
}

export function parseAgentReference(ref: string): {
  name: string;
  isSubAgent: boolean;
} {
  if (ref.startsWith("agent__")) {
    return { name: ref.slice("agent__".length), isSubAgent: true };
  }
  return { name: ref, isSubAgent: false };
}

export interface SpawnChildAgentOptions {
  agent: OrchestrationRuntimeAgent;
  systemTemplate: string;
  userPrompt: string;
  parentSession: Pick<
    AIAgentSessionConfig,
    | "config"
    | "callbacks"
    | "stream"
    | "traceLLM"
    | "traceMCP"
    | "traceSdk"
    | "verbose"
    | "temperature"
    | "topP"
    | "topK"
    | "llmTimeout"
    | "toolTimeout"
    | "maxRetries"
    | "maxTurns"
    | "toolResponseMaxBytes"
    | "targets"
    | "tools"
  > & {
    trace?: {
      selfId?: string;
      originId?: string;
      parentId?: string;
      callPath?: string;
      agentPath?: string;
      turnPath?: string;
    };
    abortSignal?: AbortSignal;
    stopRef?: { stopping: boolean };
  };
  trace?: {
    originId?: string;
    parentId?: string;
    callPath?: string;
    agentPath?: string;
    turnPath?: string;
  };
  ancestors?: string[];
}

export async function spawnOrchestrationChild(
  opts: SpawnChildAgentOptions,
): Promise<AIAgentResult> {
  const loaded = opts.agent;
  const parentSession = opts.parentSession;
  const parentAgentPathCandidates = [
    parentSession.trace?.agentPath,
    parentSession.trace?.callPath,
  ].filter(
    (value): value is string => typeof value === "string" && value.length > 0,
  );
  const parentAgentPath =
    parentAgentPathCandidates.length > 0
      ? parentAgentPathCandidates[0]
      : 'agent';
  const childAgentPath = appendCallPathSegment(
    parentAgentPath,
    loaded.toolName ?? loaded.agentId,
  );
  const parentSelfId = parentSession.trace?.selfId;
  const childTrace = {
    selfId: crypto.randomUUID(),
    originId: opts.trace?.originId ?? parentSession.trace?.originId,
    parentId: parentSelfId ?? parentSession.trace?.parentId,
    callPath: childAgentPath,
    agentPath: childAgentPath,
    turnPath: opts.trace?.turnPath ?? parentSession.trace?.turnPath ?? '',
  };
  const format = loaded.expectedOutput?.format;
  const outputFormat: OutputFormatId =
    format === 'json' ? 'json' : format === 'text' ? 'pipe' : 'markdown';
  return await loaded.run(opts.systemTemplate, opts.userPrompt, {
    history: undefined,
    callbacks: parentSession.callbacks,
    trace: childTrace,
    renderTarget: 'sub-agent',
    outputFormat,
    initialTitle: `Consultation with ${loaded.agentId}`,
    agentPath: childAgentPath,
    turnPathPrefix: childTrace.turnPath,
    abortSignal: parentSession.abortSignal,
    stopRef: parentSession.stopRef,
    ancestors: opts.ancestors,
  });
}
