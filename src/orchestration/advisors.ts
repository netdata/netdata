import type { AIAgentResult, AIAgentSessionConfig, ConversationMessage, OrchestrationRuntimeAgent } from '../types.js';

import { buildAdvisoryBlock } from './prompt-tags.js';
import { spawnOrchestrationChild } from './spawn-child.js';

export interface AdvisorExecutionResult {
  advisorRef: string;
  advisorAgentId: string;
  success: boolean;
  conversation: ConversationMessage[];
  content: string;
  block: string;
  error?: string;
  result?: AIAgentResult;
}

export interface ExecuteAdvisorsOptions {
  advisors: OrchestrationRuntimeAgent[];
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

const extractAdvisorContent = (result: AIAgentResult): string => {
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

const buildFailureContent = (advisorRef: string, error: string): string =>
  `Advisor consultation failed for ${advisorRef}: ${error}`;

export async function executeAdvisors(
  opts: ExecuteAdvisorsOptions,
): Promise<AdvisorExecutionResult[]> {
  const { advisors, userPrompt, parentSession, ancestors } = opts;
  if (advisors.length === 0) {
    return [];
  }
  const tasks = advisors.map(async (advisor) => {
    try {
      const result = await spawnOrchestrationChild({
        agent: advisor,
        systemTemplate: advisor.systemTemplate,
        userPrompt,
        parentSession,
        ancestors,
      });
      if (!result.success) {
        const message = result.error ?? "unknown error";
        const content = buildFailureContent(advisor.ref, message);
        return {
          advisorRef: advisor.ref,
          advisorAgentId: advisor.agentId,
          success: false,
          conversation: result.conversation,
          content,
          block: buildAdvisoryBlock(advisor.agentId, content),
          error: message,
          result,
        } as AdvisorExecutionResult;
      }
      const content = extractAdvisorContent(result);
      const agentLabel = result.finalAgentId ?? advisor.agentId;
      return {
        advisorRef: advisor.ref,
        advisorAgentId: agentLabel,
        success: true,
        conversation: result.conversation,
        content,
        block: buildAdvisoryBlock(agentLabel, content),
        result,
      } as AdvisorExecutionResult;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const content = buildFailureContent(advisor.ref, message);
      return {
        advisorRef: advisor.ref,
        advisorAgentId: advisor.agentId,
        success: false,
        conversation: [],
        content,
        block: buildAdvisoryBlock(advisor.agentId, content),
        error: message,
      } as AdvisorExecutionResult;
    }
  });

  return await Promise.all(tasks);
}
