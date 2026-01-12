import { describe, it, expect } from "vitest";

import type { LoadedAgent } from "../../../agent-loader.js";

import {
  detectOrchestrationCycles,
  validateOrchestrationGraph,
} from "../../../orchestration/cycle-detection.js";

function createMockAgent(
  _orchestration?: {
    handoff?: string[];
    advisors?: string[];
    routerDestinations?: string[];
  } | null,
): LoadedAgent {
  return {
    id: "test-agent",
    promptPath: "test-agent.ai",
    agentHash: "hash",
    systemTemplate: "You are a test agent.",
    description: "Test agent",
    usage: "Test usage",
    input: { format: "json", schema: {} },
    config: {
      providers: {},
      mcpServers: {},
      queues: {},
    },
    targets: [],
    tools: [],
    subAgents: [],
    effective: {
      temperature: null,
      topP: null,
      topK: null,
      llmTimeout: 60000,
      toolTimeout: 30000,
      maxRetries: 3,
      maxTurns: 10,
      maxToolCallsPerTurn: 10,
      toolResponseMaxBytes: 100000,
      stream: false,
      traceLLM: false,
      traceMCP: false,
      traceSdk: false,
      verbose: false,
      repeatPenalty: null,
    },
    subTools: [],
    createSession: async () => {
      return await Promise.reject(new Error("Not implemented"));
    },
    run: async () => {
      return await Promise.reject(new Error("Not implemented"));
    },
  };
}

describe("Cycle Detection", () => {
  describe("detectOrchestrationCycles", () => {
    it("returns no cycle for agents without orchestration", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent1", createMockAgent()],
        ["agent2", createMockAgent()],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });

    it("returns no cycle for unrelated agents", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent1", createMockAgent()],
        ["agent2", createMockAgent()],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });
  });

  describe("validateOrchestrationGraph", () => {
    it("does not throw for agents without orchestration", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent1", createMockAgent()],
        ["agent2", createMockAgent()],
      ]);

      expect(() => {
        validateOrchestrationGraph(agents);
      }).not.toThrow();
    });

    it("does not throw for unrelated agents", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent1", createMockAgent()],
        ["agent2", createMockAgent()],
      ]);

      expect(() => {
        validateOrchestrationGraph(agents);
      }).not.toThrow();
    });
  });
});
