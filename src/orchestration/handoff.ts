import type { LoadedAgent } from "../agent-loader.js";
import type { AIAgentResult, AIAgentSessionConfig } from "../types.js";
import type {
  HandoffConfig,
  HandoffTrigger,
  HandoffCondition,
} from "./types.js";

export type {
  HandoffConfig,
  HandoffTrigger,
  HandoffCondition,
} from "./types.js";

import { spawnOrchestrationChild } from "./spawn-child.js";

export interface HandoffResult {
  shouldHandoff: boolean;
  targetAgent: string | null;
  reason: string;
}

export interface ExecuteHandoffOptions {
  handoff: HandoffConfig;
  parentResult: AIAgentResult;
  agents: Map<string, LoadedAgent>;
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

export async function executeHandoff(
  opts: ExecuteHandoffOptions,
): Promise<AIAgentResult> {
  const { handoff, parentResult, agents, parentSession, ancestors } = opts;
  const targetAgentId = handoff.targetAgent;
  const agent = agents.get(targetAgentId);
  if (agent === undefined) {
    throw new Error(`Handoff target agent not found: ${targetAgentId}`);
  }
  const userPrompt = extractHandoffContent(parentResult);
  return await spawnOrchestrationChild({
    agent,
    systemTemplate: agent.systemTemplate,
    userPrompt,
    parentSession,
    ancestors,
  });
}

function extractHandoffContent(result: AIAgentResult): string {
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

export function checkHandoffConditions(
  trigger: HandoffTrigger,
  currentContext: {
    lastUserMessage?: string;
    toolFailures?: string[];
    turnCount?: number;
  },
): boolean {
  switch (trigger.kind) {
    case "explicit":
      return false;
    case "keyword": {
      const message = currentContext.lastUserMessage;
      if (message === undefined) return false;
      const lowerMessage = message.toLowerCase();
      return trigger.keywords.some((kw) =>
        lowerMessage.includes(kw.toLowerCase()),
      );
    }
    case "pattern":
      try {
        const regex = new RegExp(trigger.regex);
        return regex.test(currentContext.lastUserMessage ?? "");
      } catch {
        return false;
      }
    case "tool_failure":
      if (
        currentContext.toolFailures === undefined ||
        currentContext.toolFailures.length === 0
      ) {
        return false;
      }
      if (trigger.toolNames === undefined || trigger.toolNames.length === 0) {
        return true;
      }
      const toolNames = trigger.toolNames;
      return currentContext.toolFailures.some((failure) =>
        toolNames.includes(failure),
      );
    case "max_turns":
      return (currentContext.turnCount ?? 0) >= trigger.maxTurns;
    default:
      return false;
  }
}

export function evaluateCondition(
  condition: HandoffCondition,
  context: {
    confidence?: number;
    errorRate?: number;
    taskType?: string;
  },
): boolean {
  switch (condition.type) {
    case "confidence":
      if (condition.threshold === undefined) return true;
      return (context.confidence ?? 0) <= condition.threshold;
    case "error_rate":
      if (condition.threshold === undefined) return true;
      return (context.errorRate ?? 0) >= condition.threshold;
    case "task_type":
      if (
        condition.expectedTypes === undefined ||
        condition.expectedTypes.length === 0
      )
        return true;
      return condition.expectedTypes.includes(context.taskType ?? "");
    case "custom":
      return true;
    default:
      return false;
  }
}

export function createHandoffResult(
  handoff: HandoffConfig,
  context: {
    lastUserMessage?: string;
    toolFailures?: string[];
    turnCount?: number;
    confidence?: number;
    errorRate?: number;
    taskType?: string;
  },
): HandoffResult {
  const triggerMet = checkHandoffConditions(handoff.trigger, context);
  if (!triggerMet) {
    return {
      shouldHandoff: false,
      targetAgent: null,
      reason: "Trigger not met",
    };
  }

  if (handoff.condition !== undefined) {
    const conditionMet = evaluateCondition(handoff.condition, context);
    if (!conditionMet) {
      return {
        shouldHandoff: false,
        targetAgent: null,
        reason: "Condition not met",
      };
    }
  }

  return {
    shouldHandoff: true,
    targetAgent: handoff.targetAgent,
    reason: `Handoff triggered: ${handoff.trigger.kind}`,
  };
}
