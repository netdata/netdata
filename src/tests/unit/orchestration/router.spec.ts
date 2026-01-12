import { describe, expect, it } from "vitest";

import { RouterToolProvider } from "../../../orchestration/router.js";

const ROUTER_HANDOFF_TOOL = "router__handoff-to";
const DESTINATION_ONE = "./router-destination.ai";
const DESTINATION_TWO = "./router-destination-two.ai";

describe("RouterToolProvider", () => {
  it("returns empty tool list when no destinations configured", () => {
    const provider = new RouterToolProvider({ config: { destinations: [] } });

    const tools = provider.listTools();

    expect(tools).toEqual([]);
  });

  it("lists router__handoff-to with destination enum", () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE, DESTINATION_TWO] },
    });

    const tools = provider.listTools();

    expect(tools).toHaveLength(1);
    expect(tools[0]?.name).toBe(ROUTER_HANDOFF_TOOL);
    const schema = tools[0]?.inputSchema as {
      properties?: { agent?: { enum?: string[] } };
    };
    expect(schema.properties?.agent?.enum).toEqual([
      DESTINATION_ONE,
      DESTINATION_TWO,
    ]);
  });

  it("recognizes the router tool name", () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    expect(provider.hasTool(ROUTER_HANDOFF_TOOL)).toBe(true);
    expect(provider.hasTool("unknown-tool")).toBe(false);
  });

  it("errors when called with unknown tool", async () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    const result = await provider.execute("unknown-tool", {});

    expect(result.ok).toBe(false);
    expect(result.error).toContain("Unknown tool");
  });

  it("errors when agent parameter is missing", async () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    const result = await provider.execute(ROUTER_HANDOFF_TOOL, {});

    expect(result.ok).toBe(false);
    expect(result.error).toContain("agent must be a string");
  });

  it("errors when agent is not in destinations", async () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    const result = await provider.execute(ROUTER_HANDOFF_TOOL, {
      agent: "./unknown.ai",
    });

    expect(result.ok).toBe(false);
    expect(result.error).toContain("Unknown agent");
  });

  it("routes successfully with optional message", async () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    const result = await provider.execute(ROUTER_HANDOFF_TOOL, {
      agent: DESTINATION_ONE,
      message: "route note",
    });

    expect(result.ok).toBe(true);
    expect(result.result).toContain(`Routed to agent: ${DESTINATION_ONE}`);
  });

  it("returns instruction text when destinations exist", () => {
    const provider = new RouterToolProvider({
      config: { destinations: [DESTINATION_ONE] },
    });

    const instructions = provider.getInstructions();

    expect(instructions).toContain(ROUTER_HANDOFF_TOOL);
    expect(instructions).toContain(DESTINATION_ONE);
  });
});
