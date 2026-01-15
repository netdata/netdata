import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import type { GlobalOverrides } from "../agent-loader.js";
import type {
  AccountingEntry,
  AIAgentEvent,
  AIAgentEventCallbacks,
  AIAgentEventMeta,
  AIAgentResult,
  ConversationMessage,
  LogEntry,
  ReasoningLevel,
} from "../types.js";

import { loadAgent, LoadedAgentCache } from "../agent-loader.js";
import { isPlainObject, sanitizeToolName } from "../utils.js";

import { phase3ModelConfigs } from "./phase3-models.js";

type Tier = 1 | 2 | 3;

interface ScenarioDefinition {
  readonly id: string;
  readonly label: string;
  readonly systemPrompt: string;
  readonly userPrompt: string;
  readonly reasoning?: ReasoningLevel;
  readonly minTurns: number;
  readonly requiredAgentTools?: readonly string[];
  readonly requireReasoningSignatures?: boolean;
  readonly agentPath?: string;
  readonly streamModes?: readonly boolean[];
  readonly expectations?: ScenarioExpectations;
}

interface ToolOutputExpectation {
  readonly mode: "full-chunked" | "read-grep" | "truncate";
  readonly includes?: readonly string[];
}

interface ScenarioExpectations {
  readonly expectedFinalAgentId?: string;
  readonly expectedRouterSelectionAgent?: string;
  readonly expectedUserPromptIncludes?: readonly string[];
  readonly expectedChildPrompts?: readonly {
    readonly agentId: string;
    readonly includes: readonly string[];
  }[];
  readonly expectedToolOutputs?: readonly ToolOutputExpectation[];
}

interface ScenarioVariant {
  readonly scenario: ScenarioDefinition;
  readonly stream: boolean;
}

interface ScenarioRunResult {
  readonly modelLabel: string;
  readonly provider: string;
  readonly modelId: string;
  readonly tier: Tier;
  readonly scenarioId: string;
  readonly scenarioLabel: string;
  readonly stream: boolean;
  readonly reasoning?: ReasoningLevel;
  readonly success: boolean;
  readonly failureReasons: readonly string[];
  readonly error?: string;
  readonly accounting: readonly AccountingEntry[];
  readonly maxTurn?: number;
  readonly reasoningSummary?: {
    readonly requested: boolean;
    readonly present: boolean;
    readonly preserved: boolean;
  };
}

interface AccountingSummary {
  readonly tokensIn: number;
  readonly tokensOut: number;
  readonly totalTokens: number;
  readonly cacheReadTokens: number;
  readonly cacheWriteTokens: number;
  readonly requests: number;
  readonly latencyMs: number;
  readonly costUsd?: number;
}

const PROJECT_ROOT = process.cwd();
const PHASE3_MCP_ROOT = path.resolve(PROJECT_ROOT);
if (process.env.MCP_ROOT === undefined || process.env.MCP_ROOT.length === 0) {
  process.env.MCP_ROOT = PHASE3_MCP_ROOT;
}
const PHASE3_DUMP_LLM = process.env.PHASE3_DUMP_LLM === "1";
const PHASE3_DUMP_LLM_DIR =
  process.env.PHASE3_DUMP_LLM_DIR ??
  path.join(os.tmpdir(), "ai-agent-phase3-llm");
const TEST_AGENTS_DIR = path.resolve(
  PROJECT_ROOT,
  "src",
  "tests",
  "phase3",
  "test-agents",
);
const MASTER_AGENT_PATH = path.join(TEST_AGENTS_DIR, "test-master.ai");
const ORCH_ADVISORS_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-advisors.ai",
);
const ORCH_ADVISOR_FAILURE_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-advisor-failure.ai",
);
const ORCH_HANDOFF_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-handoff.ai",
);
const ORCH_ROUTER_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-router.ai",
);
const ORCH_ROUTER_HANDOFF_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-router-handoff.ai",
);
const ORCH_ADVISORS_HANDOFF_MASTER_PATH = path.join(
  TEST_AGENTS_DIR,
  "orchestration-advisors-handoff.ai",
);
const DEFAULT_CONFIG_PATH = path.resolve(PROJECT_ROOT, "neda/.ai-agent.json");
const FINAL_REPORT_INSTRUCTION =
  'Produce your final answer using the XML wrapper <ai-agent-{nonce}-FINAL format="text">...content...</ai-agent-{nonce}-FINAL> with the required content.';
const FINAL_REPORT_ARGS =
  '<ai-agent-{nonce}-FINAL format="text">CONTENT</ai-agent-{nonce}-FINAL>';
const TAG_ORIGINAL = "<original_user_request__";
const TAG_ADVISORY = "<advisory__";
const TAG_RESPONSE = "<response__";
const ATTR_AGENT_ADVISOR_OK = 'agent="advisor-ok"';
const ADVISOR_PROMPT_PREFIX =
  "You are the primary agent. Use advisory context if provided.\n\n";
const ATTR_AGENT_ORCH_HANDOFF = 'agent="orchestration-handoff"';
const ATTR_AGENT_ORCH_ROUTER = 'agent="orchestration-router"';
const ATTR_AGENT_ORCH_ROUTER_HANDOFF = 'agent="orchestration-router-handoff"';
const ATTR_AGENT_ORCH_ADVISORS_HANDOFF = 'agent="orchestration-advisors-handoff"';
const ATTR_AGENT_ROUTER_DEST = 'agent="router-destination"';
const ROUTER_NOTE = "ROUTER NOTE";
const ROUTER_DEST_REF = "./router-destination.ai";
const ROUTER_DEST_ID = "router-destination";
const HANDOFF_TARGET_ID = "handoff-target";
const ADVISOR_FAIL_REF = "./advisor-fail.ai";
const ADVISOR_FAILURE_MESSAGE = `Advisor consultation failed for ${ADVISOR_FAIL_REF}`;
const MASTER_HANDOFF_PAYLOAD = "MASTER-HANDOFF-PAYLOAD";
const ADVISOR_HANDOFF_PAYLOAD = "ADVISOR-HANDOFF-PAYLOAD";
const BASIC_PROMPT = `${FINAL_REPORT_INSTRUCTION} The content must be exactly "test".`;
const BASIC_USER = `Output exactly ${FINAL_REPORT_ARGS.replace("CONTENT", "test")} as your final response. No tool calls.`;
const MULTI_PROMPT = `You are a helpful CI tester helping the verification of multi-turn agentic operation.

You have 2 agents: agent1 and agent2. The test completes in 3 turns.

## Turns
1. You call just agent1 with the parameter "run"
2. You call just agent2 with the output of agent1
3. You provide your final-report with the output of agent2

For the test to complete successfully, you must call each agent alone, without any other tools.

## Process
Follow exactly these steps for a successful outcome:

1. Call ONLY "agent__test-agent1" with parameters {"prompt":"run","reason":"execute agent1","format":"sub-agent"}.
2. Once agent1 responds and you have its output, call ONLY "agent__test-agent2" with {"prompt":"I received this from agent1: [agent1 response]","reason":"execute agent2","format":"sub-agent"}.
3. Once agent2 responds and you have its output, emit your final answer as XML: <ai-agent-{nonce}-FINAL format="text">I received this from agent2: [agent2 response]</ai-agent-{nonce}-FINAL>. Do NOT call tools for the final answer.

Do not emit plain text.`;
const MULTI_USER = `CI verification scenario: follow the multi-turn process exactly.

1) Call ONLY "agent__test-agent1" with parameters {"prompt":"run","reason":"execute agent1","format":"sub-agent"}.
2) After agent__test-agent1 responds, call ONLY "agent__test-agent2" with {"prompt":"I received this from agent1: [agent1 response]","reason":"execute agent2","format":"sub-agent"}.
3) After agent__test-agent2 responds, emit your final answer as XML: <ai-agent-{nonce}-FINAL format="text">I received this from agent2: [agent2 response]</ai-agent-{nonce}-FINAL>. Do NOT call tools for the final answer.

If you skip a step or call any other tool, the CI test fails.`;

const TOOL_OUTPUT_AGENT_PATH = path.join(TEST_AGENTS_DIR, "tool-output-agent.ai");
const TOOL_OUTPUT_FIXTURE_PATH = path.join(
  "src",
  "tests",
  "phase3",
  "fixtures",
  "tool-output-large.txt",
);
const TOOL_OUTPUT_READ_START = 0;
const TOOL_OUTPUT_READ_LINES = 200;
const TOOL_OUTPUT_MODE_AUTO = "auto";
const TOOL_OUTPUT_MODE_FULL = "full-chunked";
const TOOL_OUTPUT_MODE_READ_GREP = "read-grep";
const TOOL_OUTPUT_MODE_TRUNCATE = "truncate";
const TOOL_OUTPUT_SENTINEL_AUTO = "TOOL_OUTPUT_SENTINEL_AUTO=BRAVO";
const TOOL_OUTPUT_SENTINEL_FULL = "TOOL_OUTPUT_SENTINEL_FULL=CHARLIE";
const TOOL_OUTPUT_SENTINEL_READ_GREP = "TOOL_OUTPUT_SENTINEL_READ_GREP=DELTA";
const TOOL_OUTPUT_SENTINEL_TRUNCATE = "TOOL_OUTPUT_SENTINEL_TRUNCATE=ECHO";

const buildToolOutputSystemPrompt = (
  mode: typeof TOOL_OUTPUT_MODE_AUTO
  | typeof TOOL_OUTPUT_MODE_FULL
  | typeof TOOL_OUTPUT_MODE_READ_GREP
  | typeof TOOL_OUTPUT_MODE_TRUNCATE,
  sentinel: string,
): string => {
  return [
    "You are a CI test agent validating tool_output extraction.",
    "",
    "Follow these steps exactly:",
    "1) Call ONLY filesystem_cwd__Read with:",
    `   {\"file\":\"${TOOL_OUTPUT_FIXTURE_PATH}\",\"start\":${String(TOOL_OUTPUT_READ_START)},\"lines\":${String(TOOL_OUTPUT_READ_LINES)}}.`,
    "2) The tool result will be replaced with a tool_output handle message. Do not answer yet.",
    "3) Call tool_output with:",
    "   - handle: the handle from the tool_output message",
    `   - extract: \"Return the exact line containing ${sentinel}.\"`,
    `   - mode: \"${mode}\"`,
    "4) After tool_output returns, respond with the XML final report wrapper:",
    "   <ai-agent-{nonce}-FINAL format=\"text\">tool_output complete</ai-agent-{nonce}-FINAL>",
    "Do not call any other tools. Do not answer before tool_output completes.",
  ].join("\\n");
};

const buildToolOutputUserPrompt = (mode: string, sentinel: string): string => {
  return [
    "tool_output test run.",
    `- mode=${mode}`,
    `- sentinel=${sentinel}`,
    `- path=${TOOL_OUTPUT_FIXTURE_PATH}`,
    `- read.start=${String(TOOL_OUTPUT_READ_START)}`,
    `- read.lines=${String(TOOL_OUTPUT_READ_LINES)}`,
    "Follow the system prompt exactly.",
  ].join("\\n");
};

const REQUIRED_AGENT_TOOLS: readonly string[] = [
  "agent__test-agent1",
  "agent__test-agent2",
] as const;

const ADVISORS_PROMPT =
  ADVISOR_PROMPT_PREFIX +
  'Respond with EXACTLY "advisor-enrichment-ok" in ${FORMAT}.';

const ADVISORS_USER = `Advisor enrichment test.`;

const ADVISOR_FAILURE_PROMPT =
  ADVISOR_PROMPT_PREFIX +
  'Respond with EXACTLY "advisor-failure-ok" in ${FORMAT}.';

const ADVISOR_FAILURE_USER = `Advisor failure test.`;

const HANDOFF_PROMPT =
  'You are the primary agent.\n\n' +
  'Respond with EXACTLY "' + MASTER_HANDOFF_PAYLOAD + '" in ${FORMAT}.';

const HANDOFF_USER = `Handoff test.`;

const ROUTER_PROMPT = `You are a router. You MUST call router__handoff-to exactly once.

Call router__handoff-to with:
{"agent":"${ROUTER_DEST_REF}","message":"${ROUTER_NOTE}"}

Do not answer directly.`;

const ROUTER_USER = `Routing test.`;

const ROUTER_HANDOFF_PROMPT = `You are a router. You MUST call router__handoff-to exactly once.

Call router__handoff-to with:
{"agent":"${ROUTER_DEST_REF}","message":"${ROUTER_NOTE}"}

Do not answer directly.`;

const ROUTER_HANDOFF_USER = `Routing + handoff precedence test.`;

const ADVISORS_HANDOFF_PROMPT =
  ADVISOR_PROMPT_PREFIX +
  'Respond with EXACTLY "' + ADVISOR_HANDOFF_PAYLOAD + '" in ${FORMAT}.';

const ADVISORS_HANDOFF_USER = `Advisors + handoff test.`;

// Orchestration scenarios
const ORCHESTRATION_SCENARIOS: readonly ScenarioDefinition[] = [
  {
    id: "advisor-enrichment",
    label: "advisor-enrichment",
    systemPrompt: ADVISORS_PROMPT,
    userPrompt: ADVISORS_USER,
    minTurns: 1,
    agentPath: ORCH_ADVISORS_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: "orchestration-advisors",
      expectedUserPromptIncludes: [
        TAG_ORIGINAL,
        TAG_ADVISORY,
        ATTR_AGENT_ADVISOR_OK,
      ],
    },
  },
  {
    id: "advisor-failure",
    label: "advisor-failure",
    systemPrompt: ADVISOR_FAILURE_PROMPT,
    userPrompt: ADVISOR_FAILURE_USER,
    minTurns: 1,
    agentPath: ORCH_ADVISOR_FAILURE_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: "orchestration-advisor-failure",
      expectedUserPromptIncludes: [
        TAG_ORIGINAL,
        TAG_ADVISORY,
        ADVISOR_FAILURE_MESSAGE,
      ],
    },
  },
  {
    id: "handoff-delegation",
    label: "handoff-delegation",
    systemPrompt: HANDOFF_PROMPT,
    userPrompt: HANDOFF_USER,
    minTurns: 1,
    agentPath: ORCH_HANDOFF_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: HANDOFF_TARGET_ID,
      expectedChildPrompts: [
        {
          agentId: HANDOFF_TARGET_ID,
          includes: [
            TAG_ORIGINAL,
            TAG_RESPONSE,
            ATTR_AGENT_ORCH_HANDOFF,
          ],
        },
      ],
    },
  },
  {
    id: "router-selection",
    label: "router-selection",
    systemPrompt: ROUTER_PROMPT,
    userPrompt: ROUTER_USER,
    minTurns: 1,
    agentPath: ORCH_ROUTER_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: ROUTER_DEST_ID,
      expectedRouterSelectionAgent: ROUTER_DEST_REF,
      expectedChildPrompts: [
        {
          agentId: ROUTER_DEST_ID,
          includes: [
            TAG_ORIGINAL,
            TAG_ADVISORY,
            ATTR_AGENT_ORCH_ROUTER,
          ],
        },
      ],
    },
  },
  {
    id: "router-handoff-precedence",
    label: "router-handoff-precedence",
    systemPrompt: ROUTER_HANDOFF_PROMPT,
    userPrompt: ROUTER_HANDOFF_USER,
    minTurns: 1,
    agentPath: ORCH_ROUTER_HANDOFF_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: HANDOFF_TARGET_ID,
      expectedRouterSelectionAgent: ROUTER_DEST_REF,
      expectedChildPrompts: [
        {
          agentId: ROUTER_DEST_ID,
          includes: [
            TAG_ORIGINAL,
            TAG_ADVISORY,
            ATTR_AGENT_ORCH_ROUTER_HANDOFF,
          ],
        },
        {
          agentId: HANDOFF_TARGET_ID,
          includes: [TAG_RESPONSE, ATTR_AGENT_ROUTER_DEST],
        },
      ],
    },
  },
  {
    id: "advisors-handoff",
    label: "advisors-handoff",
    systemPrompt: ADVISORS_HANDOFF_PROMPT,
    userPrompt: ADVISORS_HANDOFF_USER,
    minTurns: 1,
    agentPath: ORCH_ADVISORS_HANDOFF_MASTER_PATH,
    expectations: {
      expectedFinalAgentId: HANDOFF_TARGET_ID,
      expectedUserPromptIncludes: [
        TAG_ORIGINAL,
        TAG_ADVISORY,
        ATTR_AGENT_ADVISOR_OK,
      ],
      expectedChildPrompts: [
        {
          agentId: HANDOFF_TARGET_ID,
          includes: [
            TAG_RESPONSE,
            ATTR_AGENT_ORCH_ADVISORS_HANDOFF,
          ],
        },
      ],
    },
  },
] as const;

const TOOL_OUTPUT_SCENARIOS: readonly ScenarioDefinition[] = [
  {
    id: "tool-output-auto",
    label: "tool-output-auto",
    systemPrompt: buildToolOutputSystemPrompt(
      TOOL_OUTPUT_MODE_AUTO,
      TOOL_OUTPUT_SENTINEL_AUTO,
    ),
    userPrompt: buildToolOutputUserPrompt(
      TOOL_OUTPUT_MODE_AUTO,
      TOOL_OUTPUT_SENTINEL_AUTO,
    ),
    minTurns: 2,
    agentPath: TOOL_OUTPUT_AGENT_PATH,
    streamModes: [false],
    expectations: {
      expectedToolOutputs: [
        {
          mode: TOOL_OUTPUT_MODE_FULL,
          includes: [TOOL_OUTPUT_SENTINEL_AUTO],
        },
      ],
    },
  },
  {
    id: "tool-output-full",
    label: "tool-output-full-chunked",
    systemPrompt: buildToolOutputSystemPrompt(
      TOOL_OUTPUT_MODE_FULL,
      TOOL_OUTPUT_SENTINEL_FULL,
    ),
    userPrompt: buildToolOutputUserPrompt(
      TOOL_OUTPUT_MODE_FULL,
      TOOL_OUTPUT_SENTINEL_FULL,
    ),
    minTurns: 2,
    agentPath: TOOL_OUTPUT_AGENT_PATH,
    streamModes: [false],
    expectations: {
      expectedToolOutputs: [
        {
          mode: TOOL_OUTPUT_MODE_FULL,
          includes: [TOOL_OUTPUT_SENTINEL_FULL],
        },
      ],
    },
  },
  {
    id: "tool-output-read-grep",
    label: "tool-output-read-grep",
    systemPrompt: buildToolOutputSystemPrompt(
      TOOL_OUTPUT_MODE_READ_GREP,
      TOOL_OUTPUT_SENTINEL_READ_GREP,
    ),
    userPrompt: buildToolOutputUserPrompt(
      TOOL_OUTPUT_MODE_READ_GREP,
      TOOL_OUTPUT_SENTINEL_READ_GREP,
    ),
    minTurns: 2,
    agentPath: TOOL_OUTPUT_AGENT_PATH,
    streamModes: [false],
    expectations: {
      expectedToolOutputs: [
        {
          mode: TOOL_OUTPUT_MODE_READ_GREP,
          includes: [TOOL_OUTPUT_SENTINEL_READ_GREP],
        },
      ],
    },
  },
  {
    id: "tool-output-truncate",
    label: "tool-output-truncate",
    systemPrompt: buildToolOutputSystemPrompt(
      TOOL_OUTPUT_MODE_TRUNCATE,
      TOOL_OUTPUT_SENTINEL_TRUNCATE,
    ),
    userPrompt: buildToolOutputUserPrompt(
      TOOL_OUTPUT_MODE_TRUNCATE,
      TOOL_OUTPUT_SENTINEL_TRUNCATE,
    ),
    minTurns: 2,
    agentPath: TOOL_OUTPUT_AGENT_PATH,
    streamModes: [false],
    expectations: {
      expectedToolOutputs: [
        {
          mode: TOOL_OUTPUT_MODE_TRUNCATE,
          includes: ["WARNING: tool_output fallback to truncate", TOOL_OUTPUT_SENTINEL_TRUNCATE],
        },
      ],
    },
  },
] as const;

const BASE_SCENARIOS: readonly ScenarioDefinition[] = [
  {
    id: "basic",
    label: "basic-llm",
    systemPrompt: BASIC_PROMPT,
    userPrompt: BASIC_USER,
    minTurns: 1,
  },
  {
    id: "multi",
    label: "multi-turn",
    systemPrompt: MULTI_PROMPT,
    userPrompt: MULTI_USER,
    reasoning: "high",
    minTurns: 3,
    requiredAgentTools: REQUIRED_AGENT_TOOLS,
    requireReasoningSignatures: true,
  },
] as const;

const STREAM_VARIANTS: readonly boolean[] = [false, true] as const;
const STOP_ON_FAILURE = process.env.PHASE3_STOP_ON_FAILURE === "1";
const TRACE_LLM = process.env.PHASE3_TRACE_LLM === "1";
const TRACE_SDK = process.env.PHASE3_TRACE_SDK === "1";
const TRACE_MCP = process.env.PHASE3_TRACE_MCP === "1";
const VERBOSE_LOGS = process.env.PHASE3_VERBOSE === "1";
const STREAM_ON_LABEL = "stream-on";
const STREAM_OFF_LABEL = "stream-off";
const TEMP_DISABLED_PROVIDERS = new Set<string>();
const toErrorMessage = (value: unknown): string =>
  value instanceof Error ? value.message : String(value);

const sanitizeFileComponent = (value: string): string =>
  value.replace(/[^a-zA-Z0-9._-]+/g, "_");

const ensureFileExists = (description: string, candidate: string): void => {
  if (!fs.existsSync(candidate)) {
    throw new Error(`${description} not found at ${candidate}`);
  }
};

const parseTierFilter = (value: string): Set<Tier> => {
  const parts = value
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
  if (parts.length === 0) {
    throw new Error("tier list cannot be empty");
  }
  const tiers = new Set<Tier>();
  parts.forEach((part) => {
    const parsed = Number.parseInt(part, 10);
    if (parsed !== 1 && parsed !== 2 && parsed !== 3) {
      throw new Error(`invalid tier value '${part}'`);
    }
    tiers.add(parsed as Tier);
  });
  return tiers;
};

const parseModelFilter = (value: string): Set<string> => {
  const parts = value
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
  if (parts.length === 0) {
    throw new Error("model list cannot be empty");
  }
  return new Set(parts);
};

const isBooleanArray = (value: unknown): value is readonly boolean[] => {
  return Array.isArray(value) && value.every((item) => typeof item === "boolean");
};

const resolveStreamModes = (scenario: ScenarioDefinition): readonly boolean[] => {
  if (isBooleanArray(scenario.streamModes) && scenario.streamModes.length > 0) {
    return scenario.streamModes;
  }
  if (scenario.requiredAgentTools !== undefined) {
    return [false];
  }
  return STREAM_VARIANTS;
};

const scenarioVariants: readonly ScenarioVariant[] = (() => {
  const variants: ScenarioVariant[] = [];
  BASE_SCENARIOS.forEach((scenario) => {
    const streams = resolveStreamModes(scenario);
    streams.forEach((stream) => {
      variants.push({ scenario, stream });
    });
  });
  TOOL_OUTPUT_SCENARIOS.forEach((scenario) => {
    const streams = resolveStreamModes(scenario);
    streams.forEach((stream) => {
      variants.push({ scenario, stream });
    });
  });
  // Orchestration scenarios run in stream-off mode only (router needs state)
  ORCHESTRATION_SCENARIOS.forEach((scenario) => {
    variants.push({ scenario, stream: false });
  });
  return variants;
})();

const buildAccountingSummary = (
  entries: readonly AccountingEntry[],
): AccountingSummary => {
  let tokensIn = 0;
  let tokensOut = 0;
  let totalTokens = 0;
  let cacheReadTokens = 0;
  let cacheWriteTokens = 0;
  let latencyMs = 0;
  let requests = 0;
  let costUsd: number | undefined;
  entries.forEach((entry) => {
    if (entry.type === "llm") {
      tokensIn += entry.tokens.inputTokens;
      tokensOut += entry.tokens.outputTokens;
      totalTokens += entry.tokens.totalTokens;
      const read =
        entry.tokens.cacheReadInputTokens ?? entry.tokens.cachedTokens ?? 0;
      const write = entry.tokens.cacheWriteInputTokens ?? 0;
      cacheReadTokens += read;
      cacheWriteTokens += write;
      latencyMs += entry.latency;
      requests += 1;
      if (typeof entry.costUsd === "number") {
        costUsd = (costUsd ?? 0) + entry.costUsd;
      }
      if (typeof entry.upstreamInferenceCostUsd === "number") {
        costUsd = (costUsd ?? 0) + entry.upstreamInferenceCostUsd;
      }
    }
  });
  if (costUsd !== undefined) {
    return {
      tokensIn,
      tokensOut,
      totalTokens,
      cacheReadTokens,
      cacheWriteTokens,
      requests,
      latencyMs,
      costUsd,
    };
  }
  return {
    tokensIn,
    tokensOut,
    totalTokens,
    cacheReadTokens,
    cacheWriteTokens,
    requests,
    latencyMs,
  };
};

const extractMaxLlmTurn = (logs: readonly LogEntry[]): number => {
  const turns = logs
    .filter((log) => log.type === "llm" && log.direction === "response")
    .map((log) => log.turn);
  if (turns.length === 0) return 0;
  return Math.max(...turns);
};

const normalizeToolName = (toolName: string): string =>
  sanitizeToolName(toolName);

const findToolIndex = (logs: readonly LogEntry[], toolName: string): number => {
  const normalized = normalizeToolName(toolName);
  return logs.findIndex(
    (log) =>
      log.type === "tool" &&
      log.direction === "request" &&
      normalizeToolName(String(log.details?.tool ?? "")) === normalized,
  );
};

const collectConversationToolCalls = (
  conversation: readonly ConversationMessage[],
): Map<string, string> => {
  const map = new Map<string, string>();
  conversation.forEach((message) => {
    if (message.role !== "assistant") return;
    const rawCalls = message.toolCalls;
    if (!Array.isArray(rawCalls)) return;
    rawCalls.forEach((call) => {
      if (call.id.length === 0 || call.name.length === 0) return;
      map.set(call.id, normalizeToolName(call.name));
    });
  });
  return map;
};

const collectToolResults = (
  conversation: readonly ConversationMessage[],
  toolName: string,
): string[] => {
  const normalized = normalizeToolName(toolName);
  const callMap = collectConversationToolCalls(conversation);
  return conversation.flatMap((message) => {
    if (message.role !== "tool") return [];
    const toolCallId = message.toolCallId;
    if (typeof toolCallId !== "string" || toolCallId.length === 0) return [];
    if (callMap.get(toolCallId) !== normalized) return [];
    return [message.content];
  });
};

const TOOL_OUTPUT_STRATEGY_REGEX = /STRATEGY:([a-z-]+)/;

const extractToolOutputMode = (payload: string): string | undefined => {
  const match = TOOL_OUTPUT_STRATEGY_REGEX.exec(payload);
  return match?.[1];
};

const conversationHasToolCall = (
  conversation: readonly ConversationMessage[],
  toolName: string,
): boolean => {
  const normalized = normalizeToolName(toolName);
  const callMap = collectConversationToolCalls(conversation);
  if (callMap.size === 0) return false;
  return conversation.some((message) => {
    if (message.role !== "tool") return false;
    const toolCallId = message.toolCallId;
    if (typeof toolCallId !== "string" || toolCallId.length === 0) return false;
    return callMap.get(toolCallId) === normalized;
  });
};

const conversationToolOrder = (
  conversation: readonly ConversationMessage[],
  toolName: string,
): number => {
  const normalized = normalizeToolName(toolName);
  let order = 0;
  // eslint-disable-next-line functional/no-loop-statements -- imperative scan is clearer for ordered assistant turns
  for (const message of conversation) {
    if (message.role !== "assistant") continue;
    const rawCalls = message.toolCalls;
    if (!Array.isArray(rawCalls)) continue;
    const hasMatch = rawCalls.some((call) => {
      if (call.name.length === 0) return false;
      return normalizeToolName(call.name) === normalized;
    });
    if (hasMatch) {
      return order;
    }
    order += 1;
  }
  return -1;
};

const hasToolInvocation = (
  logs: readonly LogEntry[],
  conversation: readonly ConversationMessage[],
  toolName: string,
): boolean => {
  if (findToolIndex(logs, toolName) !== -1) return true;
  return conversationHasToolCall(conversation, toolName);
};

const hasAccountingEntries = (entries: readonly AccountingEntry[]): boolean => {
  return entries.some((entry) => entry.type === "llm");
};

const isAssistantWithReasoning = (
  message: ConversationMessage,
): message is ConversationMessage & {
  reasoning: NonNullable<ConversationMessage["reasoning"]>;
} => {
  return (
    message.role === "assistant" &&
    Array.isArray(message.reasoning) &&
    message.reasoning.length > 0
  );
};

type ReasoningSegment = NonNullable<ConversationMessage["reasoning"]>[number];

const safeJsonSnippet = (value: unknown, limit = 80): string => {
  try {
    const serialized = JSON.stringify(value);
    if (typeof serialized === "string") return serialized.slice(0, limit);
  } catch {
    return "[unstringifiable]";
  }
  return "";
};

const getReasoningSignature = (segment: ReasoningSegment): string | undefined => {
  const value: unknown = segment;
  if (!isPlainObject(value)) return undefined;
  const signature = value.signature;
  if (typeof signature !== "string" || signature.length === 0) return undefined;
  return signature;
};

const describeReasoningSegment = (segment: ReasoningSegment): {
  readonly keys: string[];
  readonly providerMetadata?: unknown;
  readonly textSnippet: string;
} => {
  const segmentValue: unknown = segment;
  if (typeof segmentValue === "string") {
    return { keys: [], textSnippet: segmentValue.slice(0, 80) };
  }
  if (isPlainObject(segmentValue)) {
    const textValue = segmentValue.text;
    let textSnippet = "";
    if (typeof textValue === "string") {
      textSnippet = textValue.slice(0, 80);
    } else {
      textSnippet = safeJsonSnippet(segmentValue);
    }
    return {
      keys: Object.keys(segmentValue),
      providerMetadata: segmentValue.providerMetadata,
      textSnippet,
    };
  }
  return { keys: [], textSnippet: safeJsonSnippet(segmentValue) };
};

interface ReasoningSignatureStatus {
  readonly present: boolean;
  readonly preserved: boolean;
}

const analyzeReasoningSignatures = (
  messages: readonly ConversationMessage[],
): ReasoningSignatureStatus => {
  const reasoningMessages = messages.filter(isAssistantWithReasoning);
  if (reasoningMessages.length === 0) {
    return { present: false, preserved: false };
  }

  const firstMessage = reasoningMessages[0];
  const firstHasSignature = firstMessage.reasoning.some(
    (segment) => getReasoningSignature(segment) !== undefined,
  );
  const othersHaveSignature = reasoningMessages.slice(1).every((message) => {
    return message.reasoning.some(
      (segment) => getReasoningSignature(segment) !== undefined,
    );
  });
  const preserved = firstHasSignature && othersHaveSignature;
  return { present: true, preserved };
};

const findFirstUserContent = (
  messages: readonly ConversationMessage[],
): string | undefined =>
  messages.find(
    (message) => message.role === "user" && typeof message.content === "string",
  )?.content;

type ChildConversation = NonNullable<AIAgentResult["childConversations"]>[number];

const findChildConversation = (
  childConversations: readonly ChildConversation[] | undefined,
  agentId: string,
) => childConversations?.find((entry) => entry.agentId === agentId);

const parseSnippetList = (
  value: ScenarioExpectations["expectedUserPromptIncludes"],
): readonly string[] => {
  if (!Array.isArray(value)) return [];
  return value.filter((snippet): snippet is string => typeof snippet === "string");
};

const parseChildPromptExpectations = (
  value: ScenarioExpectations["expectedChildPrompts"],
): readonly { agentId: string; includes: readonly string[] }[] => {
  if (!Array.isArray(value)) return [];
  return value.flatMap((entry) => {
    if (!isPlainObject(entry)) return [];
    const agentId = entry.agentId;
    const includes = entry.includes;
    if (typeof agentId !== "string" || !Array.isArray(includes)) return [];
    const includeSnippets = includes.filter(
      (snippet): snippet is string => typeof snippet === "string",
    );
    return [{ agentId, includes: includeSnippets }];
  });
};

const parseToolOutputExpectations = (
  value: ScenarioExpectations["expectedToolOutputs"],
): readonly ToolOutputExpectation[] => {
  if (!Array.isArray(value)) return [];
  const allowedModes: readonly ToolOutputExpectation["mode"][] = [
    "full-chunked",
    "read-grep",
    "truncate",
  ];
  return value.flatMap((entry) => {
    if (!isPlainObject(entry)) return [];
    const rawMode = entry.mode;
    if (typeof rawMode !== "string" || rawMode.length === 0) return [];
    const mode = allowedModes.find((item) => item === rawMode);
    if (mode === undefined) return [];
    const includesRaw = entry.includes;
    const includes = Array.isArray(includesRaw)
      ? includesRaw.filter((item): item is string => typeof item === "string")
      : [];
    return [{ mode, includes }];
  });
};

const applyScenarioExpectations = (
  session: AIAgentResult,
  scenario: ScenarioDefinition | undefined,
  failures: string[],
): void => {
  if (scenario?.expectations === undefined) {
    return;
  }
  const expectations = scenario.expectations;
  if (
    typeof expectations.expectedFinalAgentId === "string" &&
    session.finalAgentId !== expectations.expectedFinalAgentId
  ) {
    failures.push(
      `expected finalAgentId ${expectations.expectedFinalAgentId}, got ${session.finalAgentId ?? "undefined"}`,
    );
  }
  if (
    typeof expectations.expectedRouterSelectionAgent === "string" &&
    session.routerSelection?.agent !== expectations.expectedRouterSelectionAgent
  ) {
    failures.push(
      `expected routerSelection.agent ${expectations.expectedRouterSelectionAgent}, got ${session.routerSelection?.agent ?? "undefined"}`,
    );
  }
  const userPromptIncludes = parseSnippetList(
    expectations.expectedUserPromptIncludes,
  );
  if (userPromptIncludes.length > 0) {
    const userContent = findFirstUserContent(session.conversation) ?? "";
    userPromptIncludes.forEach((snippetText) => {
      if (!userContent.includes(snippetText)) {
        failures.push(`expected user prompt to include "${snippetText}"`);
      }
    });
  }
  const childPromptExpectations = parseChildPromptExpectations(
    expectations.expectedChildPrompts,
  );
  if (childPromptExpectations.length > 0) {
    childPromptExpectations.forEach((expectation) => {
      const agentId = expectation.agentId;
      const entry = findChildConversation(
        session.childConversations,
        agentId,
      );
      if (entry === undefined) {
        failures.push(
          `expected child conversation for agent ${agentId}`,
        );
        return;
      }
      const childUser = findFirstUserContent(entry.conversation) ?? "";
      expectation.includes.forEach((snippetText) => {
        if (!childUser.includes(snippetText)) {
          failures.push(
            `expected child ${agentId} prompt to include "${snippetText}"`,
          );
        }
      });
    });
  }
  const toolOutputExpectations = parseToolOutputExpectations(
    expectations.expectedToolOutputs,
  );
  if (toolOutputExpectations.length > 0) {
    const toolOutputs = collectToolResults(session.conversation, "tool_output");
    if (toolOutputs.length === 0) {
      failures.push("expected tool_output results but none were found");
    } else {
      toolOutputExpectations.forEach((expectation) => {
        const matches = toolOutputs.some((payload) => {
          const mode = extractToolOutputMode(payload);
          if (mode !== expectation.mode) return false;
          if (expectation.includes === undefined || expectation.includes.length === 0) {
            return true;
          }
          return expectation.includes.every((snippet) => payload.includes(snippet));
        });
        if (!matches) {
          const snippetList = expectation.includes?.join(", ") ?? "no snippets";
          failures.push(
            `expected tool_output strategy ${expectation.mode} containing [${snippetList}]`,
          );
        }
      });
    }
  }
};

const validateScenarioResult = (
  result: ScenarioRunResult,
  runResult: {
    result?: Awaited<ReturnType<ReturnType<typeof loadAgent>["run"]>>;
    error?: string;
  },
): ScenarioRunResult => {
  if (runResult.error !== undefined) {
    return {
      ...result,
      success: false,
      failureReasons: [...result.failureReasons, runResult.error],
      error: runResult.error,
    };
  }
  const session = runResult.result;
  if (session === undefined) {
    const reason = "missing session result";
    return {
      ...result,
      success: false,
      failureReasons: [...result.failureReasons, reason],
      error: reason,
    };
  }

  const failures: string[] = [];
  if (!session.success) {
    failures.push(`session unsuccessful: ${session.error ?? "unknown error"}`);
  }
  if (session.finalReport === undefined) {
    failures.push("final report missing or not successful");
  }
  if (
    hasToolInvocation(session.logs, session.conversation, "agent__final_report")
  ) {
    failures.push(
      "final report tool was invoked (final answers must use XML wrapper)",
    );
  }
  const maxTurn = extractMaxLlmTurn(session.logs);
  const accountingOk = hasAccountingEntries(session.accounting);
  if (!accountingOk) {
    failures.push("no llm accounting entries recorded");
  }

  const scenario =
    BASE_SCENARIOS.find((candidate) => candidate.id === result.scenarioId) ??
    TOOL_OUTPUT_SCENARIOS.find(
      (candidate) => candidate.id === result.scenarioId,
    ) ??
    ORCHESTRATION_SCENARIOS.find(
      (candidate) => candidate.id === result.scenarioId,
    );
  let reasoningSummary: ScenarioRunResult["reasoningSummary"];
  const reasoningRequested = scenario?.reasoning !== undefined;
  if (scenario !== undefined) {
    if (maxTurn < scenario.minTurns) {
      failures.push(
        `expected at least ${String(scenario.minTurns)} turns, observed ${String(maxTurn)}`,
      );
    }
    if (Array.isArray(scenario.requiredAgentTools)) {
      const tools = scenario.requiredAgentTools.filter(
        (value): value is string => typeof value === "string",
      );
      const toolInvocationStatus = tools.map((toolName) => ({
        name: toolName,
        logIndex: findToolIndex(session.logs, toolName),
        conversationIndex: conversationToolOrder(
          session.conversation,
          toolName,
        ),
        invoked: hasToolInvocation(
          session.logs,
          session.conversation,
          toolName,
        ),
      }));
      toolInvocationStatus.forEach((status) => {
        if (!status.invoked) {
          failures.push(
            `required sub-agent tool '${status.name}' was not invoked`,
          );
        }
      });
      if (
        toolInvocationStatus.every((status) => status.invoked) &&
        tools.length >= 2
      ) {
        const first = toolInvocationStatus[0];
        const second = toolInvocationStatus[1];
        const firstOrder =
          first.logIndex >= 0 ? first.logIndex : first.conversationIndex;
        const secondOrder =
          second.logIndex >= 0 ? second.logIndex : second.conversationIndex;
        if (firstOrder >= 0 && secondOrder >= 0 && firstOrder > secondOrder) {
          failures.push("agent2 ran before agent1");
        }
      }
    }
    applyScenarioExpectations(session, scenario, failures);
    if (reasoningRequested) {
      const { present, preserved } = analyzeReasoningSignatures(
        session.conversation,
      );
      reasoningSummary = { requested: true, present, preserved };
      if (
        scenario.requireReasoningSignatures === true &&
        result.provider === "anthropic"
      ) {
        if (!present) {
          console.warn(
            `[WARN] ${result.modelLabel} :: ${scenario.label} :: requested reasoning but provider returned no reasoning segments`,
          );
        } else if (!preserved) {
          failures.push(
            "reasoning signatures missing or not preserved across turns",
          );
          const assistantReasoning = session.conversation
            .filter(isAssistantWithReasoning)
            .map((message, index) => {
              const segments = message.reasoning;
              const signatureFlags = segments.map(
                (segment) => getReasoningSignature(segment) !== undefined,
              );
              return {
                index,
                segments: segments.length,
                signatures: signatureFlags,
                raw: segments.map((segment) => describeReasoningSegment(segment)),
              };
            });
          console.warn(
            `[WARN] ${result.modelLabel} :: ${scenario.label} :: reasoning signature map ${JSON.stringify(assistantReasoning)}`,
          );
        }
      }
    }
  }

  reasoningSummary ??= {
    requested: reasoningRequested,
    present: false,
    preserved: false,
  };

  return {
    ...result,
    success: failures.length === 0,
    failureReasons: failures,
    accounting: session.accounting,
    maxTurn,
    reasoningSummary,
  };
};

const runScenarioVariant = async (
  configPath: string,
  modelLabel: string,
  provider: string,
  modelId: string,
  tier: Tier,
  variant: ScenarioVariant,
): Promise<ScenarioRunResult> => {
  const { scenario, stream } = variant;
  const cache = new LoadedAgentCache();
  const overrides: GlobalOverrides = {
    models: [{ provider, model: modelId }],
  };
  if (scenario.reasoning !== undefined) {
    overrides.reasoning = scenario.reasoning;
  }
  const baseResult: ScenarioRunResult = {
    modelLabel,
    provider,
    modelId,
    tier,
    scenarioId: scenario.id,
    scenarioLabel: scenario.label,
    stream,
    reasoning: scenario.reasoning,
    success: false,
    failureReasons: [],
    accounting: [],
    maxTurn: 0,
  };

  const masterPath = scenario.agentPath ?? MASTER_AGENT_PATH;

  try {
    const agent = loadAgent(masterPath, cache, {
      configPath,
      globalOverrides: overrides,
      stream,
      reasoning: scenario.reasoning,
    });
    const callbacks: AIAgentEventCallbacks | undefined = VERBOSE_LOGS
      ? {
          onEvent: (event: AIAgentEvent, _meta: AIAgentEventMeta) => {
            if (event.type !== "log") return;
            const entry: LogEntry = event.entry;
            const turnSegment = Number.isFinite(entry.turn)
              ? ` turn=${String(entry.turn)}`
              : "";
            console.log(
              `[LOG] ${entry.severity} ${entry.remoteIdentifier}${turnSegment}: ${entry.message}`,
            );
          },
        }
      : undefined;
    const result = await agent.run(scenario.systemPrompt, scenario.userPrompt, {
      outputFormat: "markdown",
      callbacks,
      traceLLM: TRACE_LLM,
      traceSdk: TRACE_SDK,
      traceMCP: TRACE_MCP,
      verbose: VERBOSE_LOGS,
    });
    const validated = validateScenarioResult(baseResult, { result });
    if (!validated.success) {
      dumpLlmRequests(result, validated);
    }
    return validated;
  } catch (error: unknown) {
    const message = toErrorMessage(error);
    return {
      ...baseResult,
      success: false,
      failureReasons: [message],
      error: message,
    };
  }
};

const describeReasoningStatus = (run: ScenarioRunResult): string => {
  const summary = run.reasoningSummary;
  const requested = summary?.requested ?? false;
  if (!requested) {
    return "not-requested";
  }
  const present = summary?.present ?? false;
  if (!present) {
    return "requested-absent";
  }
  const preserved = summary?.preserved ?? false;
  return preserved ? "preserved" : "missing-signatures";
};

const formatRunMetrics = (
  run: ScenarioRunResult,
  summaryOverride?: AccountingSummary,
): string => {
  const accounting = summaryOverride ?? buildAccountingSummary(run.accounting);
  const reasoning = describeReasoningStatus(run);
  const latencyPart = `latency=${String(accounting.latencyMs)}ms`;
  const requestsPart =
    accounting.requests > 0 ? `, requests=${String(accounting.requests)}` : "";
  const tokensPart = `tokens[in=${String(accounting.tokensIn)}, out=${String(accounting.tokensOut)}, cacheR=${String(accounting.cacheReadTokens)}, cacheW=${String(accounting.cacheWriteTokens)}, total=${String(accounting.totalTokens)}]`;
  const costPart =
    accounting.costUsd !== undefined
      ? `, costUsd≈${accounting.costUsd.toFixed(4)}`
      : "";
  return `reasoning=${reasoning} | ${latencyPart}${requestsPart} | ${tokensPart}${costPart}`;
};

const formatSummaryLine = (
  run: ScenarioRunResult,
  summaryOverride?: AccountingSummary,
): string => {
  const outcome = run.success ? "PASS" : "FAIL";
  const streamLabel = run.stream ? STREAM_ON_LABEL : STREAM_OFF_LABEL;
  return `[${outcome}] ${run.modelLabel} (${run.provider}:${run.modelId}) :: ${run.scenarioLabel} :: ${streamLabel} | ${formatRunMetrics(run, summaryOverride)}`;
};

const printSummary = (runs: readonly ScenarioRunResult[]): void => {
  console.log("Phase 3 Integration Summary");
  let totalTokensIn = 0;
  let totalTokensOut = 0;
  let totalCacheRead = 0;
  let totalCacheWrite = 0;
  let totalTokens = 0;
  let totalLatency = 0;
  let totalRequests = 0;
  let totalCost = 0;
  let costObserved = false;
  // eslint-disable-next-line functional/no-loop-statements
  for (const run of runs) {
    const summary = buildAccountingSummary(run.accounting);
    console.log(formatSummaryLine(run, summary));
    if (!run.success) {
      run.failureReasons.forEach((reason) => {
        console.log(`  - ${reason}`);
      });
    }
    totalTokensIn += summary.tokensIn;
    totalTokensOut += summary.tokensOut;
    totalCacheRead += summary.cacheReadTokens;
    totalCacheWrite += summary.cacheWriteTokens;
    totalTokens += summary.totalTokens;
    totalLatency += summary.latencyMs;
    totalRequests += summary.requests;
    if (summary.costUsd !== undefined) {
      totalCost += summary.costUsd;
      costObserved = true;
    }
  }
  const totalsLine = [
    `tokens[in=${String(totalTokensIn)}, out=${String(totalTokensOut)}, cacheR=${String(totalCacheRead)}, cacheW=${String(totalCacheWrite)}, total=${String(totalTokens)}]`,
    `latency=${String(totalLatency)}ms`,
    `requests=${String(totalRequests)}`,
  ];
  if (costObserved) {
    totalsLine.push(`costUsd≈${totalCost.toFixed(4)}`);
  }
  console.log(`Overall Totals: ${totalsLine.join(" | ")}`);
};

const printUsage = (): void => {
  console.log(
    "Usage: node dist/tests/phase3-runner.js [--config=path] [--tier=1,2,3] [--model=label,modelId] [--scenario=id,label]",
  );
  console.log("Environment override: PHASE3_CONFIG=/path/to/config.json");
  console.log(
    "Set PHASE3_STOP_ON_FAILURE=1 to halt immediately after the first failure (default: continue running all models).",
  );
  console.log(
    "Set PHASE3_DUMP_LLM=1 to dump LLM request payloads for failing scenarios to /tmp (override with PHASE3_DUMP_LLM_DIR).",
  );
};

const parseScenarioFilter = (value: string): Set<string> => {
  const parts = value
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
  if (parts.length === 0) {
    throw new Error("scenario list cannot be empty");
  }
  return new Set(parts);
};

const matchesScenarioFilter = (
  scenario: ScenarioDefinition,
  filter: Set<string> | undefined,
): boolean => {
  if (filter === undefined) return true;
  return filter.has(scenario.id) || filter.has(scenario.label);
};

const extractToolNamesFromPayload = (payload: unknown): string[] => {
  if (!isPlainObject(payload)) return [];
  const tools = payload.tools;
  if (!Array.isArray(tools)) return [];
  return tools
    .map((entry) => (isPlainObject(entry) ? entry.name : undefined))
    .filter((name): name is string => typeof name === "string");
};

const summarizeMessages = (payload: unknown): { role: string; preview: string }[] => {
  if (!isPlainObject(payload)) return [];
  const messages = payload.messages;
  if (!Array.isArray(messages)) return [];
  return messages
    .map((entry) => {
      if (!isPlainObject(entry)) return undefined;
      const role = typeof entry.role === "string" ? entry.role : "unknown";
      const content = entry.content;
      const preview =
        typeof content === "string"
          ? content.slice(0, 240)
          : Array.isArray(content)
            ? JSON.stringify(content).slice(0, 240)
            : "";
      return { role, preview };
    })
    .filter((entry): entry is { role: string; preview: string } => entry !== undefined);
};

const dumpLlmRequests = (
  session: AIAgentResult,
  run: ScenarioRunResult,
): void => {
  if (!PHASE3_DUMP_LLM) return;
  const entries = session.logs.filter((entry) => entry.llmRequestPayload !== undefined);
  if (entries.length === 0) {
    console.log(
      `[TRACE] ${run.modelLabel} :: ${run.scenarioLabel} produced no LLM request payloads to dump.`,
    );
    return;
  }
  fs.mkdirSync(PHASE3_DUMP_LLM_DIR, { recursive: true });
  const fileName = [
    sanitizeFileComponent(run.modelLabel),
    sanitizeFileComponent(run.scenarioId),
    run.stream ? "stream-on" : "stream-off",
  ].join("__");
  const filePath = path.join(PHASE3_DUMP_LLM_DIR, `phase3-${fileName}.jsonl`);
  const lines = entries.map((entry) => {
    const payload = entry.llmRequestPayload;
    const payloadBody = payload?.body;
    const parsed = (() => {
      if (typeof payloadBody !== "string" || payloadBody.length === 0) {
        return undefined;
      }
      try {
        return JSON.parse(payloadBody) as unknown;
      } catch {
        return undefined;
      }
    })();
    const tools = parsed !== undefined ? extractToolNamesFromPayload(parsed) : [];
    const messages = parsed !== undefined ? summarizeMessages(parsed) : [];
    return JSON.stringify({
      timestamp: entry.timestamp,
      turn: entry.turn,
      subturn: entry.subturn,
      remoteIdentifier: entry.remoteIdentifier,
      message: entry.message,
      details: entry.details,
      payload: payload,
      toolNames: tools,
      messagePreviews: messages,
    });
  });
  fs.writeFileSync(filePath, lines.join("\n"));
  console.log(
    `[TRACE] dumped ${String(entries.length)} LLM request payloads to ${filePath}`,
  );
};

async function main(): Promise<void> {
  const masterPaths = new Set<string>([
    MASTER_AGENT_PATH,
    ...TOOL_OUTPUT_SCENARIOS.map((scenario) => scenario.agentPath).filter(
      (pathValue): pathValue is string => typeof pathValue === "string",
    ),
    ...ORCHESTRATION_SCENARIOS.map((scenario) => scenario.agentPath).filter(
      (pathValue): pathValue is string => typeof pathValue === "string",
    ),
  ]);
  masterPaths.forEach((promptPath) => {
    ensureFileExists("Master agent prompt", promptPath);
  });
  ensureFileExists(
    "tool_output fixture",
    path.resolve(PROJECT_ROOT, TOOL_OUTPUT_FIXTURE_PATH),
  );
  const argv = process.argv.slice(2);
  if (argv.includes("--help") || argv.includes("-h")) {
    printUsage();
    return;
  }

  let configPath = process.env.PHASE3_CONFIG ?? DEFAULT_CONFIG_PATH;
  let tierFilter: Set<Tier> | undefined;
  let tierThreshold: number | undefined;
  let modelFilter: Set<string> | undefined;
  let scenarioFilter: Set<string> | undefined;

  argv.forEach((arg) => {
    if (arg.startsWith("--config=")) {
      configPath = path.resolve(arg.slice("--config=".length));
    } else if (arg.startsWith("--tier=")) {
      tierFilter = parseTierFilter(arg.slice("--tier=".length));
      tierThreshold = Math.max(...Array.from(tierFilter));
    } else if (arg.startsWith("--model=")) {
      modelFilter = parseModelFilter(arg.slice("--model=".length));
    } else if (arg.startsWith("--scenario=")) {
      scenarioFilter = parseScenarioFilter(arg.slice("--scenario=".length));
    } else {
      throw new Error(`unknown argument '${arg}'`);
    }
  });

  ensureFileExists("Configuration file", configPath);

  const runs: ScenarioRunResult[] = [];
  let abort = false;
  let executed = 0;
  // eslint-disable-next-line functional/no-loop-statements
  for (const model of phase3ModelConfigs) {
    if (abort) break;
    if (TEMP_DISABLED_PROVIDERS.has(model.provider)) {
      console.log(
        `[SKIP] ${model.label} (${model.provider}:${model.modelId}) disabled temporarily`,
      );
      continue;
    }
    if (tierThreshold !== undefined && model.tier > tierThreshold) continue;
    if (modelFilter !== undefined) {
      const matches =
        modelFilter.has(model.label) ||
        modelFilter.has(`${model.provider}/${model.modelId}`) ||
        modelFilter.has(model.modelId);
      if (!matches) continue;
    }
    // eslint-disable-next-line functional/no-loop-statements
    for (const variant of scenarioVariants) {
      if (!matchesScenarioFilter(variant.scenario, scenarioFilter)) {
        continue;
      }
      executed += 1;
      const streamLabel = variant.stream ? STREAM_ON_LABEL : STREAM_OFF_LABEL;
      const runIndexLabel = String(executed);
      const tierLabel = String(model.tier);
      console.log(
        `[RUN] #${runIndexLabel} ${model.label} (${model.provider}:${model.modelId}, tier ${tierLabel}) :: ${variant.scenario.label} :: ${streamLabel}`,
      );
      const run = await runScenarioVariant(
        configPath,
        model.label,
        model.provider,
        model.modelId,
        model.tier,
        variant,
      );
      runs.push(run);
      const summary = buildAccountingSummary(run.accounting);
      const outcomeLine = formatSummaryLine(run, summary);
      console.log(outcomeLine);
      if (!run.success) {
        run.failureReasons.forEach((reason) => {
          console.log(`  - ${reason}`);
        });
      }
      if (STOP_ON_FAILURE && !run.success) {
        abort = true;
        break;
      }
    }
  }

  printSummary(runs);
  if (runs.some((run) => !run.success)) {
    process.exitCode = 1;
  }
}

main().catch((error: unknown) => {
  console.error("phase3-runner failed", toErrorMessage(error));
  process.exit(1);
});
