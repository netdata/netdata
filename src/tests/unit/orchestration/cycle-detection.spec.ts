import { describe, it, expect } from "vitest";

import type { LoadedAgent } from "../../../agent-loader.js";

import {
  detectOrchestrationCycles,
  validateOrchestrationGraph,
} from "../../../orchestration/cycle-detection.js";

function createMockAgent(
  orchestration?: {
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
    orchestration:
      orchestration === null
        ? undefined
        : {
            handoff: orchestration?.handoff ?? [],
            advisors: orchestration?.advisors ?? [],
            routerDestinations: orchestration?.routerDestinations ?? [],
          },
  };
}

describe("Cycle Detection", () => {
  describe("detectOrchestrationCycles", () => {
    it("returns no cycle for agents without orchestration", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent()],
        ["agent-b", createMockAgent()],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
      expect(result.cyclePath).toBeUndefined();
      expect(result.errorMessage).toBeUndefined();
    });

    it("returns no cycle for linear handoff chain", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-c"] })],
        ["agent-c", createMockAgent()],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });

    it("returns no cycle for linear router chain", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ routerDestinations: ["agent-b"] })],
        ["agent-b", createMockAgent({ routerDestinations: ["agent-c"] })],
        ["agent-c", createMockAgent()],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });

    it("detects two-agent cycle via handoff", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(true);
      expect(result.cyclePath).toBeDefined();
      expect(result.cyclePath).toContain("agent-a");
      expect(result.cyclePath).toContain("agent-b");
      expect(result.errorMessage).toContain("Orchestration cycle detected");
    });

    it("detects three-agent cycle via handoff", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-c"] })],
        ["agent-c", createMockAgent({ handoff: ["agent-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(true);
      expect(result.cyclePath).toBeDefined();
      expect(result.cyclePath?.length ?? 0).toBeGreaterThanOrEqual(3);
    });

    it("detects cycle via router destinations", () => {
      const agents = new Map<string, LoadedAgent>([
        ["router-a", createMockAgent({ routerDestinations: ["router-b"] })],
        ["router-b", createMockAgent({ routerDestinations: ["router-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(true);
    });

    it("detects mixed cycle (handoff + router)", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ routerDestinations: ["agent-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(true);
    });

    it("ignores advisors for cycle detection", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ advisors: ["agent-b"] })],
        ["agent-b", createMockAgent({ advisors: ["agent-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });

    it("ignores non-existent targets in cycle detection", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["non-existent"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-a"] })],
      ]);

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });

    it("handles empty agent map", () => {
      const agents = new Map<string, LoadedAgent>();

      const result = detectOrchestrationCycles(agents);

      expect(result.hasCycle).toBe(false);
    });
  });

  describe("validateOrchestrationGraph", () => {
    it("does not throw for acyclic graph", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-c"] })],
        ["agent-c", createMockAgent()],
      ]);

      expect(() => {
        validateOrchestrationGraph(agents);
      }).not.toThrow();
    });

    it("throws error for cyclic graph", () => {
      const agents = new Map<string, LoadedAgent>([
        ["agent-a", createMockAgent({ handoff: ["agent-b"] })],
        ["agent-b", createMockAgent({ handoff: ["agent-a"] })],
      ]);

      expect(() => {
        validateOrchestrationGraph(agents);
      }).toThrow("Orchestration cycle detected");
    });
  });
});
