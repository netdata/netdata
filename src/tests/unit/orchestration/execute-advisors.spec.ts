import { describe, expect, it, vi } from "vitest";

import type { AIAgentResult, Configuration, OrchestrationRuntimeAgent } from "../../../types.js";

import { executeAdvisors } from "../../../orchestration/advisors.js";
import { spawnOrchestrationChild } from "../../../orchestration/spawn-child.js";

vi.mock("../../../orchestration/spawn-child.js", () => ({
  spawnOrchestrationChild: vi.fn(),
}));

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

const buildAdvisor = (ref: string): OrchestrationRuntimeAgent => ({
  ref,
  path: ref,
  agentId: "advisor",
  promptPath: ref,
  systemTemplate: "Advisor system",
  run: () => Promise.reject(new Error("unused")),
});

describe("executeAdvisors", () => {
  it("returns failure block when advisor run is unsuccessful", async () => {
    const mockResult: AIAgentResult = {
      success: false,
      error: "boom",
      conversation: [],
      logs: [],
      accounting: [],
    };
    const mockSpawn = vi.mocked(spawnOrchestrationChild);
    mockSpawn.mockResolvedValueOnce(mockResult);

    const results = await executeAdvisors({
      advisors: [buildAdvisor("./advisor.ai")],
      userPrompt: "test",
      parentSession: baseParentSession,
    });

    expect(results).toHaveLength(1);
    const result = results[0];
    expect(result.success).toBe(false);
    expect(result.block).toContain("Advisor consultation failed for ./advisor.ai");
    expect(result.block).toContain("boom");
    expect(result.result).toBe(mockResult);
  });
});
