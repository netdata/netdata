import { describe, it, expect } from "vitest";

import type {
  AdvisorConfig,
  AdvisorTrigger,
} from "../../../orchestration/types.js";

import {
  checkAdvisorTrigger,
  createAdvisorResult,
  getConsultationPriority,
} from "../../../orchestration/advisors.js";

describe("Advisors", () => {
  describe("checkAdvisorTrigger", () => {
    describe("always trigger", () => {
      it("returns true for always trigger", () => {
        const trigger: AdvisorTrigger = { kind: "always" };

        const result = checkAdvisorTrigger(trigger, {});

        expect(result).toBe(true);
      });
    });

    describe("task_type trigger", () => {
      it("matches task type", () => {
        const trigger: AdvisorTrigger = {
          kind: "task_type",
          taskTypes: ["legal", "compliance"],
        };

        const result = checkAdvisorTrigger(trigger, {
          taskType: "legal-question",
        });

        expect(result).toBe(true);
      });

      it("case-insensitive match", () => {
        const trigger: AdvisorTrigger = {
          kind: "task_type",
          taskTypes: ["LEGAL"],
        };

        const result = checkAdvisorTrigger(trigger, {
          taskType: "legal-question",
        });

        expect(result).toBe(true);
      });

      it("returns false when task type doesn't match", () => {
        const trigger: AdvisorTrigger = {
          kind: "task_type",
          taskTypes: ["legal"],
        };

        const result = checkAdvisorTrigger(trigger, {
          taskType: "technical-issue",
        });

        expect(result).toBe(false);
      });

      it("returns false when task type is undefined", () => {
        const trigger: AdvisorTrigger = {
          kind: "task_type",
          taskTypes: ["legal"],
        };

        const result = checkAdvisorTrigger(trigger, {});

        expect(result).toBe(false);
      });
    });

    describe("keyword trigger", () => {
      it("matches keyword in message", () => {
        const trigger: AdvisorTrigger = {
          kind: "keyword",
          keywords: ["urgent", "asap"],
        };

        const result = checkAdvisorTrigger(trigger, {
          lastUserMessage: "This is an urgent request",
        });

        expect(result).toBe(true);
      });

      it("case-insensitive keyword match", () => {
        const trigger: AdvisorTrigger = {
          kind: "keyword",
          keywords: ["URGENT"],
        };

        const result = checkAdvisorTrigger(trigger, {
          lastUserMessage: "this is urgent please",
        });

        expect(result).toBe(true);
      });

      it("returns false when keyword not found", () => {
        const trigger: AdvisorTrigger = {
          kind: "keyword",
          keywords: ["urgent"],
        };

        const result = checkAdvisorTrigger(trigger, {
          lastUserMessage: "Normal request",
        });

        expect(result).toBe(false);
      });

      it("returns false when message is undefined", () => {
        const trigger: AdvisorTrigger = {
          kind: "keyword",
          keywords: ["urgent"],
        };

        const result = checkAdvisorTrigger(trigger, {});

        expect(result).toBe(false);
      });
    });

    describe("explicit trigger", () => {
      it("returns false for explicit trigger", () => {
        const trigger: AdvisorTrigger = { kind: "explicit" };

        const result = checkAdvisorTrigger(trigger, {});

        expect(result).toBe(false);
      });
    });
  });

  describe("createAdvisorResult", () => {
    it("returns negative result when trigger not met", () => {
      const advisor: AdvisorConfig = {
        agent: "test-advisor",
        when: { kind: "keyword", keywords: ["urgent"] },
      };

      const result = createAdvisorResult(advisor, {
        lastUserMessage: "Normal message",
      });

      expect(result.shouldConsult).toBe(false);
      expect(result.advisorAgent).toBe("");
      expect(result.reason).toBe("Trigger not met");
    });

    it("returns positive result when trigger met", () => {
      const advisor: AdvisorConfig = {
        agent: "legal-advisor",
        when: { kind: "keyword", keywords: ["legal"] },
      };

      const result = createAdvisorResult(advisor, {
        lastUserMessage: "Legal question here",
      });

      expect(result.shouldConsult).toBe(true);
      expect(result.advisorAgent).toBe("legal-advisor");
      expect(result.reason).toContain("Advisor triggered");
    });

    it("handles always trigger", () => {
      const advisor: AdvisorConfig = {
        agent: "always-advisor",
        when: { kind: "always" },
      };

      const result = createAdvisorResult(advisor, {});

      expect(result.shouldConsult).toBe(true);
      expect(result.advisorAgent).toBe("always-advisor");
    });
  });

  describe("getConsultationPriority", () => {
    it("returns priority when defined", () => {
      const advisor: AdvisorConfig = {
        agent: "test",
        when: { kind: "always" },
        priority: 10,
      };

      const priority = getConsultationPriority(advisor);

      expect(priority).toBe(10);
    });

    it("returns 0 when priority undefined", () => {
      const advisor: AdvisorConfig = {
        agent: "test",
        when: { kind: "always" },
      };

      const priority = getConsultationPriority(advisor);

      expect(priority).toBe(0);
    });

    it("sorts advisors by priority descending", () => {
      const advisors: AdvisorConfig[] = [
        { agent: "low", when: { kind: "always" }, priority: 1 },
        { agent: "high", when: { kind: "always" }, priority: 100 },
        { agent: "medium", when: { kind: "always" }, priority: 50 },
      ];

      const sorted = [...advisors].sort(
        (a, b) => getConsultationPriority(b) - getConsultationPriority(a),
      );

      expect(sorted[0]?.agent).toBe("high");
      expect(sorted[1]?.agent).toBe("medium");
      expect(sorted[2]?.agent).toBe("low");
    });
  });
});
