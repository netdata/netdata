import type { AIAgentResult, AIAgentSessionConfig, OrchestrationRuntimeAgent } from '../types.js';

import { buildOriginalUserRequestBlock, buildResponseBlock, joinTaggedBlocks } from './prompt-tags.js';
import { spawnOrchestrationChild } from './spawn-child.js';

export interface ExecuteHandoffOptions {
  target: OrchestrationRuntimeAgent;
  parentResult: AIAgentResult;
  originalUserPrompt: string;
  parentAgentLabel: string;
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
      originId?: string;
      parentId?: string;
      callPath?: string;
      agentPath?: string;
      turnPath?: string;
    };
    abortSignal?: AbortSignal;
    stopRef?: { stopping: boolean };
  };
  ancestors?: string[];
}

const extractHandoffContent = (result: AIAgentResult): string => {
  if (
    result.finalReport?.format === 'json' &&
    result.finalReport.content_json !== undefined
  ) {
    return JSON.stringify(result.finalReport.content_json);
  }
  if (typeof result.finalReport?.content === 'string') {
    return result.finalReport.content;
  }
  const lastAssistant = result.conversation
    .filter((m) => m.role === 'assistant')
    .pop();
  return lastAssistant?.content ?? '';
};

export async function executeHandoff(
  opts: ExecuteHandoffOptions,
): Promise<AIAgentResult> {
  const { target, parentResult, originalUserPrompt, parentAgentLabel } = opts;
  const response = extractHandoffContent(parentResult);
  const prompt = joinTaggedBlocks([
    buildResponseBlock(parentAgentLabel, response),
    buildOriginalUserRequestBlock(originalUserPrompt),
  ]);
  return await spawnOrchestrationChild({
    agent: target,
    systemTemplate: target.systemTemplate,
    userPrompt: prompt,
    parentSession: opts.parentSession,
    ancestors: opts.ancestors,
  });
}
