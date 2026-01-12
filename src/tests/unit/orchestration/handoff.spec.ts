import { describe, it, expect } from "vitest";

import type {
  HandoffConfig,
  HandoffTrigger,
  HandoffCondition,
} from "../../../orchestration/types.js";

import {
  checkHandoffConditions,
  evaluateCondition,
  createHandoffResult,
} from "../../../orchestration/handoff.js";

describe("Handoff", () => {
  describe("checkHandoffConditions", () => {
    describe("explicit trigger", () => {
      it("returns false for explicit trigger", () => {
        const trigger: HandoffTrigger = { kind: "explicit" };

        const result = checkHandoffConditions(trigger, {});

        expect(result).toBe(false);
      });
    });

    describe("keyword trigger", () => {
      it("matches keyword in message", () => {
        const trigger: HandoffTrigger = {
          kind: "keyword",
          keywords: ["escalate", "manager"],
        };

        const result = checkHandoffConditions(trigger, {
          lastUserMessage: "Please escalate this to a manager",
        });

        expect(result).toBe(true);
      });

      it("returns false when keyword not found", () => {
        const trigger: HandoffTrigger = {
          kind: "keyword",
          keywords: ["escalate"],
        };

        const result = checkHandoffConditions(trigger, {
          lastUserMessage: "Normal request",
        });

        expect(result).toBe(false);
      });

      it("returns false when message is undefined", () => {
        const trigger: HandoffTrigger = {
          kind: "keyword",
          keywords: ["escalate"],
        };

        const result = checkHandoffConditions(trigger, {});

        expect(result).toBe(false);
      });
    });

    describe("pattern trigger", () => {
      it("matches regex pattern", () => {
        const trigger: HandoffTrigger = {
          kind: "pattern",
          regex: "^error:",
        };

        const result = checkHandoffConditions(trigger, {
          lastUserMessage: "error: something went wrong",
        });

        expect(result).toBe(true);
      });

      it("returns false when pattern doesn't match", () => {
        const trigger: HandoffTrigger = {
          kind: "pattern",
          regex: "^error:",
        };

        const result = checkHandoffConditions(trigger, {
          lastUserMessage: "normal message",
        });

        expect(result).toBe(false);
      });

      it("returns false for invalid regex", () => {
        const trigger: HandoffTrigger = {
          kind: "pattern",
          regex: "[invalid{",
        };

        const result = checkHandoffConditions(trigger, {
          lastUserMessage: "test",
        });

        expect(result).toBe(false);
      });
    });

    describe("tool_failure trigger", () => {
      it("returns true when tool failed", () => {
        const trigger: HandoffTrigger = {
          kind: "tool_failure",
          toolNames: ["database"],
        };

        const result = checkHandoffConditions(trigger, {
          toolFailures: ["database"],
        });

        expect(result).toBe(true);
      });

      it("returns true for any tool failure when no specific names", () => {
        const trigger: HandoffTrigger = {
          kind: "tool_failure",
        };

        const result = checkHandoffConditions(trigger, {
          toolFailures: ["any-tool"],
        });

        expect(result).toBe(true);
      });

      it("returns false when no tool failures", () => {
        const trigger: HandoffTrigger = {
          kind: "tool_failure",
          toolNames: ["database"],
        };

        const result = checkHandoffConditions(trigger, {});

        expect(result).toBe(false);
      });

      it("returns false when different tool failed", () => {
        const trigger: HandoffTrigger = {
          kind: "tool_failure",
          toolNames: ["database"],
        };

        const result = checkHandoffConditions(trigger, {
          toolFailures: ["other-tool"],
        });

        expect(result).toBe(false);
      });
    });

    describe("max_turns trigger", () => {
      it("returns true at max turns", () => {
        const trigger: HandoffTrigger = {
          kind: "max_turns",
          maxTurns: 5,
        };

        const result = checkHandoffConditions(trigger, {
          turnCount: 5,
        });

        expect(result).toBe(true);
      });

      it("returns true beyond max turns", () => {
        const trigger: HandoffTrigger = {
          kind: "max_turns",
          maxTurns: 5,
        };

        const result = checkHandoffConditions(trigger, {
          turnCount: 10,
        });

        expect(result).toBe(true);
      });

      it("returns false before max turns", () => {
        const trigger: HandoffTrigger = {
          kind: "max_turns",
          maxTurns: 5,
        };

        const result = checkHandoffConditions(trigger, {
          turnCount: 3,
        });

        expect(result).toBe(false);
      });

      it("returns false when turnCount undefined", () => {
        const trigger: HandoffTrigger = {
          kind: "max_turns",
          maxTurns: 5,
        };

        const result = checkHandoffConditions(trigger, {});

        expect(result).toBe(false);
      });
    });
  });

  describe("evaluateCondition", () => {
    describe("confidence condition", () => {
      it("returns true when below threshold", () => {
        const condition: HandoffCondition = {
          type: "confidence",
          threshold: 0.8,
        };

        const result = evaluateCondition(condition, {
          confidence: 0.5,
        });

        expect(result).toBe(true);
      });

      it("returns false when above threshold", () => {
        const condition: HandoffCondition = {
          type: "confidence",
          threshold: 0.5,
        };

        const result = evaluateCondition(condition, {
          confidence: 0.8,
        });

        expect(result).toBe(false);
      });

      it("returns true when no threshold", () => {
        const condition: HandoffCondition = {
          type: "confidence",
        };

        const result = evaluateCondition(condition, {
          confidence: 0.9,
        });

        expect(result).toBe(true);
      });
    });

    describe("error_rate condition", () => {
      it("returns true when above threshold", () => {
        const condition: HandoffCondition = {
          type: "error_rate",
          threshold: 0.1,
        };

        const result = evaluateCondition(condition, {
          errorRate: 0.5,
        });

        expect(result).toBe(true);
      });

      it("returns false when below threshold", () => {
        const condition: HandoffCondition = {
          type: "error_rate",
          threshold: 0.5,
        };

        const result = evaluateCondition(condition, {
          errorRate: 0.1,
        });

        expect(result).toBe(false);
      });
    });

    describe("task_type condition", () => {
      it("returns true for exact match", () => {
        const condition: HandoffCondition = {
          type: "task_type",
          expectedTypes: ["legal", "compliance"],
        };

        const result = evaluateCondition(condition, {
          taskType: "legal",
        });

        expect(result).toBe(true);
      });

      it("returns false for non-matching task type", () => {
        const condition: HandoffCondition = {
          type: "task_type",
          expectedTypes: ["legal"],
        };

        const result = evaluateCondition(condition, {
          taskType: "technical",
        });

        expect(result).toBe(false);
      });

      it("returns true when expectedTypes is empty", () => {
        const condition: HandoffCondition = {
          type: "task_type",
        };

        const result = evaluateCondition(condition, {
          taskType: "any",
        });

        expect(result).toBe(true);
      });
    });
  });

  describe("createHandoffResult", () => {
    it("returns negative when trigger not met", () => {
      const handoff: HandoffConfig = {
        trigger: { kind: "keyword", keywords: ["escalate"] },
        targetAgent: "manager",
      };

      const result = createHandoffResult(handoff, {
        lastUserMessage: "Normal message",
      });

      expect(result.shouldHandoff).toBe(false);
      expect(result.targetAgent).toBeNull();
      expect(result.reason).toBe("Trigger not met");
    });

    it("returns negative when condition not met", () => {
      const handoff: HandoffConfig = {
        trigger: { kind: "keyword", keywords: ["escalate"] },
        targetAgent: "manager",
        condition: {
          type: "confidence",
          threshold: 0.9,
        },
      };

      const result = createHandoffResult(handoff, {
        lastUserMessage: "Please escalate",
        confidence: 0.95,
      });

      expect(result.shouldHandoff).toBe(false);
      expect(result.reason).toBe("Condition not met");
    });

    it("returns positive when trigger and condition met", () => {
      const handoff: HandoffConfig = {
        trigger: { kind: "keyword", keywords: ["escalate"] },
        targetAgent: "manager",
        condition: {
          type: "confidence",
          threshold: 0.9,
        },
      };

      const result = createHandoffResult(handoff, {
        lastUserMessage: "Please escalate",
        confidence: 0.5,
      });

      expect(result.shouldHandoff).toBe(true);
      expect(result.targetAgent).toBe("manager");
      expect(result.reason).toContain("Handoff triggered");
    });

    it("ignores condition when not defined", () => {
      const handoff: HandoffConfig = {
        trigger: { kind: "max_turns", maxTurns: 5 },
        targetAgent: "manager",
      };

      const result = createHandoffResult(handoff, {
        turnCount: 5,
      });

      expect(result.shouldHandoff).toBe(true);
    });
  });
});
