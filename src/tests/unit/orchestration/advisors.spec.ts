import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AIAgentResult, Configuration, OrchestrationRuntimeAgent } from "../../../types.js";

const { mockSpawn } = vi.hoisted(() => ({ mockSpawn: vi.fn() }));

vi.mock("../../../orchestration/spawn-child.js", () => ({
  spawnOrchestrationChild: mockSpawn,
}));

beforeEach(() => {
  mockSpawn.mockReset();
  vi.resetModules();
});

const baseConfig: Configuration = {
  providers: {},
  mcpServers: {},
  queues: { default: { concurrent: 1 } },
};

const baseParentSession = {
  config: baseConfig,
  callbacks: undefined,
  stream: false,
  traceLLM: false,
  traceMCP: false,
  traceSdk: false,
  verbose: false,
  temperature: null,
  topP: null,
  topK: null,
  llmTimeout: 0,
  toolTimeout: 0,
  maxRetries: 0,
  maxTurns: 1,
  toolResponseMaxBytes: 0,
  targets: [],
  tools: [],
};

const ADVISOR_REF = "./advisor.ai";
const ADVISOR_REPLY = "advisor reply";

const buildAdvisor = (ref: string): OrchestrationRuntimeAgent => ({
  ref,
  path: ref,
  agentId: "advisor",
  promptPath: ref,
  systemTemplate: "Advisor system",
  toolName: "advisor",
  run: () => Promise.reject(new Error("unused")),
});

describe("executeAdvisors", () => {
  it("builds advisory block when advisor succeeds", async () => {
    const { executeAdvisors } = await import("../../../orchestration/advisors.js");
    const mockResult: AIAgentResult = {
      success: true,
      conversation: [{ role: "assistant", content: ADVISOR_REPLY }],
      logs: [],
      accounting: [],
      finalReport: {
        format: "text",
        content: ADVISOR_REPLY,
        ts: Date.now(),
      },
      finalAgentId: "advisor",
    };
    mockSpawn.mockResolvedValueOnce(mockResult);

    const results = await executeAdvisors({
      advisors: [buildAdvisor(ADVISOR_REF)],
      userPrompt: "test",
      parentSession: baseParentSession,
    });

    expect(results).toHaveLength(1);
    const result = results[0];
    expect(result.success).toBe(true);
    expect(result.block).toContain("<advisory__");
    expect(result.block).toContain(ADVISOR_REPLY);
  });

  it("returns failure block when advisor run is unsuccessful", async () => {
    const { executeAdvisors } = await import("../../../orchestration/advisors.js");
    const mockResult: AIAgentResult = {
      success: false,
      error: "boom",
      conversation: [],
      logs: [],
      accounting: [],
    };
    mockSpawn.mockResolvedValueOnce(mockResult);

    const results = await executeAdvisors({
      advisors: [buildAdvisor(ADVISOR_REF)],
      userPrompt: "test",
      parentSession: baseParentSession,
    });

    expect(results).toHaveLength(1);
    const result = results[0];
    expect(result.success).toBe(false);
    expect(result.block).toContain(`Advisor consultation failed for ${ADVISOR_REF}`);
    expect(result.block).toContain("boom");
    expect(result.result).toBe(mockResult);
  });
});
