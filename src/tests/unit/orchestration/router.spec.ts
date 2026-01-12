import { describe, it, expect } from "vitest";

import type {
  RouterConfig,
  RouterDestination,
} from "../../../orchestration/types.js";

import {
  RouterToolProvider,
  routeRequest,
  listRouterDestinations,
  getRouterDestination,
} from "../../../orchestration/router.js";

const ROUTER_HANDFOFF_TOOL = "router__handoff-to";
const DESTINATION_LEGAL = "legal";
const DESTINATION_TECH = "tech";

function createRouterConfig(destinations: RouterDestination[]): RouterConfig {
  return {
    destinations,
    routingStrategy: "task_type",
  };
}

describe("Router", () => {
  describe("RouterToolProvider", () => {
    describe("listTools", () => {
      it("returns empty array for empty destinations", () => {
        const config = createRouterConfig([]);
        const provider = new RouterToolProvider({ config });

        const tools = provider.listTools();

        expect(tools).toEqual([]);
      });

      it("returns router__handoff-to tool for single destination", () => {
        const config = createRouterConfig([
          // eslint-disable-next-line sonarjs/no-duplicate-string
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const tools = provider.listTools();

        expect(tools).toHaveLength(1);
        expect(tools[0]?.name).toBe(ROUTER_HANDFOFF_TOOL);
        const schema = tools[0]?.inputSchema as {
          properties?: { destination?: { enum?: string[] } };
        };
        expect(schema.properties?.destination?.enum).toEqual([
          DESTINATION_LEGAL,
        ]);
      });

      it("returns router__handoff-to tool with multiple destinations", () => {
        const config = createRouterConfig([
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
          // eslint-disable-next-line sonarjs/no-duplicate-string
          { name: DESTINATION_TECH, agent: "tech-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const tools = provider.listTools();

        expect(tools).toHaveLength(1);
        const schema = tools[0]?.inputSchema as {
          properties?: { destination?: { enum?: string[] } };
        };
        expect(schema.properties?.destination?.enum).toEqual([
          DESTINATION_LEGAL,
          DESTINATION_TECH,
        ]);
      });
    });

    describe("hasTool", () => {
      it("returns true for router__handoff-to", () => {
        const config = createRouterConfig([
          { name: "test", agent: "test-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        expect(provider.hasTool(ROUTER_HANDFOFF_TOOL)).toBe(true);
      });

      it("returns false for unknown tool", () => {
        const config = createRouterConfig([
          { name: "test", agent: "test-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        expect(provider.hasTool("unknown-tool")).toBe(false);
      });
    });

    describe("execute", () => {
      it("returns error for unknown tool", async () => {
        const config = createRouterConfig([]);
        const provider = new RouterToolProvider({ config });

        const result = await provider.execute("unknown-tool", {});

        expect(result.ok).toBe(false);
        expect(result.error).toContain("Unknown tool");
      });

      it("returns error for missing destination", async () => {
        const config = createRouterConfig([
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const result = await provider.execute(ROUTER_HANDFOFF_TOOL, {});

        expect(result.ok).toBe(false);
        expect(result.error).toContain("destination must be a string");
      });

      it("returns error for unknown destination", async () => {
        const config = createRouterConfig([
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const result = await provider.execute(ROUTER_HANDFOFF_TOOL, {
          destination: "unknown",
        });

        expect(result.ok).toBe(false);
        expect(result.error).toContain("Unknown destination");
      });

      it("selects destination successfully", async () => {
        const config = createRouterConfig([
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
          { name: DESTINATION_TECH, agent: "tech-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const result = await provider.execute(ROUTER_HANDFOFF_TOOL, {
          destination: "legal",
        });

        expect(result.ok).toBe(true);
        expect(result.result).toContain("Routed to destination: legal");
        expect((result.extras as Record<string, unknown>).destination).toBe(
          "legal",
        );
        expect((result.extras as Record<string, unknown>).agent).toBe(
          "legal-agent",
        );
      });

      it("tracks selected destination", async () => {
        const config = createRouterConfig([
          { name: DESTINATION_TECH, agent: "tech-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        await provider.execute(ROUTER_HANDFOFF_TOOL, { destination: "tech" });

        expect(provider.getSelectedDestination()).toBe("tech");
      });
    });

    describe("getInstructions", () => {
      it("returns empty string for empty destinations", () => {
        const config = createRouterConfig([]);
        const provider = new RouterToolProvider({ config });

        expect(provider.getInstructions()).toBe("");
      });

      it("returns instruction for destinations", () => {
        const config = createRouterConfig([
          { name: DESTINATION_LEGAL, agent: "legal-agent", taskTypes: [] },
          { name: DESTINATION_TECH, agent: "tech-agent", taskTypes: [] },
        ]);
        const provider = new RouterToolProvider({ config });

        const instructions = provider.getInstructions();

        expect(instructions).toContain(ROUTER_HANDFOFF_TOOL);
        expect(instructions).toContain("legal");
        expect(instructions).toContain("tech");
      });
    });
  });

  describe("routeRequest", () => {
    it("returns no destination for empty config", () => {
      const config = createRouterConfig([]);

      const result = routeRequest(config, { taskType: "test" });

      expect(result.destination).toBeNull();
      expect(result.agent).toBeNull();
      expect(result.reason).toBe("No destinations configured");
    });

    it("matches task type", () => {
      const config = createRouterConfig([
        {
          name: "legal",
          agent: "legal-agent",
          taskTypes: ["legal", "compliance"],
        },
        { name: "tech", agent: "tech-agent", taskTypes: ["technical"] },
      ]);

      const result = routeRequest(config, { taskType: "legal-question" });

      expect(result.destination).toBe("legal");
      expect(result.agent).toBe("legal-agent");
      expect(result.reason).toBe("Matched task type");
    });

    it("returns null for unmatched task type", () => {
      const config = createRouterConfig([
        { name: "legal", agent: "legal-agent", taskTypes: ["legal"] },
      ]);

      const result = routeRequest(config, { taskType: "other-task" });

      expect(result.destination).toBeNull();
    });
  });

  describe("listRouterDestinations", () => {
    it("returns destination names", () => {
      const config = createRouterConfig([
        { name: "dest1", agent: "agent1", taskTypes: [] },
        { name: "dest2", agent: "agent2", taskTypes: [] },
      ]);

      const destinations = listRouterDestinations(config);

      expect(destinations).toEqual(["dest1", "dest2"]);
    });

    it("returns empty for empty config", () => {
      const config = createRouterConfig([]);

      const destinations = listRouterDestinations(config);

      expect(destinations).toEqual([]);
    });
  });

  describe("getRouterDestination", () => {
    it("returns destination by name", () => {
      const config = createRouterConfig([
        { name: "legal", agent: "legal-agent", taskTypes: [] },
      ]);

      const dest = getRouterDestination(config, "legal");

      expect(dest).toBeDefined();
      expect(dest?.name).toBe("legal");
      expect(dest?.agent).toBe("legal-agent");
    });

    it("returns undefined for unknown name", () => {
      const config = createRouterConfig([
        { name: "legal", agent: "legal-agent", taskTypes: [] },
      ]);

      const dest = getRouterDestination(config, "unknown");

      expect(dest).toBeUndefined();
    });
  });
});
