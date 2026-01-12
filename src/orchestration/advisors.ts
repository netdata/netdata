import type { LoadedAgent } from "../agent-loader.js";
import type {
  AIAgentResult,
  AIAgentSessionConfig,
  ConversationMessage,
} from "../types.js";
import type { AdvisorConfig, AdvisorTrigger } from "./types.js";

import { spawnOrchestrationChild } from "./spawn-child.js";

export interface AdvisorResult {
  shouldConsult: boolean;
  advisorAgent: string;
  reason: string;
}

export interface AdvisorExecutionResult {
  advisorId: string;
  success: boolean;
  conversation: ConversationMessage[];
  result: string;
  error?: string;
}

export interface ExecuteAdvisorsOptions {
  advisors: AdvisorConfig[];
  agents: Map<string, LoadedAgent>;
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

function synthesizeAdvisorFailure(
  advisorId: string,
  error: string,
): ConversationMessage[] {
  return [
    {
      role: "user",
      content: `[Advisor Consultation Failed]\n\nAdvisor: ${advisorId}\nError: ${error}\n\nThe advisor could not be consulted due to the error above. Please proceed without this advisor's input.`,
    },
  ];
}

export async function executeAdvisors(
  opts: ExecuteAdvisorsOptions,
): Promise<AdvisorExecutionResult[]> {
  const {
    advisors,
    agents,
    systemTemplate,
    userPrompt,
    parentSession,
    ancestors,
  } = opts;
  if (advisors.length === 0) {
    return [];
  }
  const advisorPromises = advisors.map(async (advisorConfig) => {
    const agent = agents.get(advisorConfig.agent);
    if (agent === undefined) {
      const error = `Advisor agent not found: ${advisorConfig.agent}`;
      return {
        advisorId: advisorConfig.agent,
        success: false,
        conversation: synthesizeAdvisorFailure(advisorConfig.agent, error),
        result: "",
        error,
      } as AdvisorExecutionResult;
    }
    try {
      const result = await spawnOrchestrationChild({
        agent,
        systemTemplate,
        userPrompt,
        parentSession,
        ancestors,
      });
      const advisorContent = extractAdvisorContent(result);
      const conversation: ConversationMessage[] = [
        {
          role: "user",
          content: `[Advisor Consultation: ${advisorConfig.agent}]\n\n${advisorContent}`,
        },
      ];
      return {
        advisorId: advisorConfig.agent,
        success: true,
        conversation,
        result: advisorContent,
      } as AdvisorExecutionResult;
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : String(error);
      return {
        advisorId: advisorConfig.agent,
        success: false,
        conversation: synthesizeAdvisorFailure(
          advisorConfig.agent,
          errorMessage,
        ),
        result: "",
        error: errorMessage,
      } as AdvisorExecutionResult;
    }
  });
  return await Promise.all(advisorPromises);
}

function extractAdvisorContent(result: AIAgentResult): string {
  if (
    result.finalReport?.format === "json" &&
    result.finalReport.content_json !== undefined
  ) {
    return JSON.stringify(result.finalReport.content_json);
  }
  if (typeof result.finalReport?.content === "string") {
    return result.finalReport.content;
  }
  const lastAssistant = result.conversation
    .filter((m) => m.role === "assistant")
    .pop();
  return lastAssistant?.content ?? "";
}

export function checkAdvisorTrigger(
  trigger: AdvisorTrigger,
  currentContext: {
    lastUserMessage?: string;
    taskType?: string;
  },
): boolean {
  switch (trigger.kind) {
    case "always":
      return true;
    case "task_type": {
      const taskType = currentContext.taskType;
      if (taskType === undefined) return false;
      const lowerTaskType = taskType.toLowerCase();
      return trigger.taskTypes.some((tt) =>
        lowerTaskType.includes(tt.toLowerCase()),
      );
    }
    case "keyword": {
      const message = currentContext.lastUserMessage;
      if (message === undefined) return false;
      const lowerMessage = message.toLowerCase();
      return trigger.keywords.some((kw) =>
        lowerMessage.includes(kw.toLowerCase()),
      );
    }
    case "explicit":
      return false;
    default:
      return false;
  }
}

export function createAdvisorResult(
  advisor: AdvisorConfig,
  context: {
    lastUserMessage?: string;
    taskType?: string;
  },
): AdvisorResult {
  const shouldConsult = checkAdvisorTrigger(advisor.when, context);
  if (!shouldConsult) {
    return {
      shouldConsult: false,
      advisorAgent: "",
      reason: "Trigger not met",
    };
  }

  return {
    shouldConsult: true,
    advisorAgent: advisor.agent,
    reason: `Advisor triggered: ${advisor.when.kind}`,
  };
}

export function getConsultationPriority(advisor: AdvisorConfig): number {
  return advisor.priority ?? 0;
}
