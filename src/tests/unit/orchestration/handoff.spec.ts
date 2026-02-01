import { beforeEach, describe, expect, it, vi } from "vitest";

import type { SpawnChildAgentOptions } from "../../../orchestration/spawn-child.js";
import type { AIAgentResult, Configuration, OrchestrationRuntimeAgent } from "../../../types.js";

const { mockSpawn } = vi.hoisted(() => ({ mockSpawn: vi.fn() }));

vi.mock("../../../orchestration/spawn-child.js", () => ({
  spawnOrchestrationChild: mockSpawn,
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

const HANDOFF_REF = "./handoff-target.ai";

const buildTarget = (ref: string): OrchestrationRuntimeAgent => ({
  ref,
  path: ref,
  agentId: "handoff-target",
  promptPath: ref,
  systemTemplate: "handoff system",
  toolName: "handoff-target",
  run: () => Promise.reject(new Error("unused")),
});

beforeEach(() => {
  mockSpawn.mockReset();
  vi.resetModules();
});

describe("executeHandoff", () => {
  it("injects original user request and response blocks", async () => {
    const { executeHandoff } = await import("../../../orchestration/handoff.js");
    const mockResult: AIAgentResult = {
      success: true,
      conversation: [],
      logs: [],
      accounting: [],
      finalReport: {
        format: "text",
        content: "child-response",
        ts: Date.now(),
      },
    };
    mockSpawn.mockResolvedValueOnce(mockResult);

    const parentResult: AIAgentResult = {
      success: true,
      conversation: [],
      logs: [],
      accounting: [],
      finalReport: {
        format: "text",
        content: "parent-response",
        ts: Date.now(),
      },
    };

    await executeHandoff({
      target: buildTarget(HANDOFF_REF),
      parentResult,
      originalUserPrompt: "original request",
      parentAgentLabel: "parent-agent",
      parentSession: baseParentSession,
    });

    expect(mockSpawn).toHaveBeenCalledTimes(1);
    const call = mockSpawn.mock.calls[0] as [SpawnChildAgentOptions];
    const userPrompt = call[0].userPrompt;
    expect(userPrompt).toContain("<original_user_request__");
    expect(userPrompt).toContain("original request");
    expect(userPrompt).toContain("<response__");
    expect(userPrompt).toContain("parent-response");
  });
});
