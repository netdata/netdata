import type {
  ToolExecuteOptions,
  ToolExecuteResult,
  ToolKind,
} from "../tools/types.js";
import type { MCPTool } from "../types.js";
import type { RouterConfig, RouterDestination } from "./types.js";

import { ToolProvider } from "../tools/types.js";

export interface RoutingResult {
  destination: string | null;
  agent: string | null;
  reason: string;
}

export interface RouterToolProviderOptions {
  config: RouterConfig;
}

export class RouterToolProvider extends ToolProvider {
  readonly kind: ToolKind = "agent";
  readonly namespace = "router";
  private readonly config: RouterConfig;
  private selectedDestination: string | null = null;
  private static readonly HANDOFF_TOOL_NAME = "handoff-to";

  constructor(opts: RouterToolProviderOptions) {
    super();
    this.config = opts.config;
  }

  listTools(): MCPTool[] {
    if (this.config.destinations.length === 0) {
      return [];
    }
    const destinations = this.config.destinations.map((d) => d.name);
    return [
      {
        name: `${this.namespace}__${RouterToolProvider.HANDOFF_TOOL_NAME}`,
        description: `Select a destination for routing. Available destinations: ${destinations.join(", ")}`,
        inputSchema: {
          type: "object",
          properties: {
            destination: {
              type: "string",
              enum: destinations,
              description: "The destination to route to",
            },
          },
          required: ["destination"],
          additionalProperties: false,
        },
      },
    ];
  }

  hasTool(name: string): boolean {
    return (
      name === `${this.namespace}__${RouterToolProvider.HANDOFF_TOOL_NAME}`
    );
  }

  async execute(
    name: string,
    parameters: Record<string, unknown>,
    _opts?: ToolExecuteOptions,
  ): Promise<ToolExecuteResult> {
    const expectedName = `${this.namespace}__${RouterToolProvider.HANDOFF_TOOL_NAME}`;
    if (name !== expectedName) {
      return {
        ok: false,
        error: `Unknown tool: ${name}`,
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    const destination = parameters.destination;
    if (typeof destination !== "string") {
      return {
        ok: false,
        error: "destination must be a string",
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    const dest = this.config.destinations.find((d) => d.name === destination);
    if (dest === undefined) {
      return {
        ok: false,
        error: `Unknown destination: ${destination}`,
        latencyMs: 0,
        kind: this.kind,
        namespace: this.namespace,
      };
    }
    this.selectedDestination = destination;
    const result: ToolExecuteResult = {
      ok: true,
      result: `Routed to destination: ${destination}`,
      latencyMs: 0,
      kind: this.kind,
      namespace: this.namespace,
      extras: {
        destination: destination,
        agent: dest.agent,
      },
    };
    await Promise.resolve();
    return result;
  }

  getSelectedDestination(): string | null {
    return this.selectedDestination;
  }

  override getInstructions(): string {
    if (this.config.destinations.length === 0) {
      return "";
    }
    const destinations = this.config.destinations.map((d) => d.name).join(", ");
    return `Use the router__handoff-to tool to route to one of the available destinations: ${destinations}`;
  }
}

export function routeRequest(
  config: RouterConfig,
  context: {
    taskType?: string;
    taskDescription?: string;
  },
): RoutingResult {
  if (config.destinations.length === 0) {
    return {
      destination: null,
      agent: null,
      reason: "No destinations configured",
    };
  }

  switch (config.routingStrategy) {
    case "task_type": {
      const match = config.destinations.find((dest) =>
        dest.taskTypes.some((tt) => {
          const taskType = context.taskType;
          if (taskType === undefined) return false;
          return taskType.toLowerCase().includes(tt.toLowerCase());
        }),
      );
      if (match !== undefined) {
        return {
          destination: match.name,
          agent: match.agent,
          reason: "Matched task type",
        };
      }
      break;
    }
    case "priority": {
      const sorted = [...config.destinations].sort(
        (a, b) => (b.defaultPriority ?? 0) - (a.defaultPriority ?? 0),
      );
      return {
        destination: sorted[0]?.name ?? null,
        agent: sorted[0]?.agent ?? null,
        reason: "Highest priority",
      };
    }
    case "round_robin": {
      const now = Date.now();
      const index = now % config.destinations.length;
      const dest = config.destinations[index];
      return {
        destination: dest.name,
        agent: dest.agent,
        reason: "Round robin selection",
      };
    }
    case "dynamic": {
      const scored = config.destinations.map((dest) => ({
        dest,
        score: scoreDestination(dest, context),
      }));
      scored.sort((a, b) => b.score - a.score);
      if (scored[0].score > 0) {
        return {
          destination: scored[0].dest.name,
          agent: scored[0].dest.agent,
          reason: "Highest dynamic score",
        };
      }
      break;
    }
  }

  if (config.defaultDestination !== undefined) {
    const defaultDest = config.destinations.find(
      (d) => d.name === config.defaultDestination,
    );
    if (defaultDest !== undefined) {
      return {
        destination: defaultDest.name,
        agent: defaultDest.agent,
        reason: "Using default destination",
      };
    }
  }

  return { destination: null, agent: null, reason: "No matching destination" };
}

function scoreDestination(
  dest: RouterDestination,
  context: { taskType?: string; taskDescription?: string },
): number {
  let score = 0;
  const taskType = context.taskType ?? "";
  if (dest.taskTypes.includes(taskType)) {
    score += 10;
  }
  const taskDesc = context.taskDescription?.toLowerCase() ?? "";
  if (taskDesc.length > 0) {
    const desc = dest.description;
    if (desc !== undefined && taskDesc.includes(desc.toLowerCase())) {
      score += 5;
    }
  }
  return score;
}

export function listRouterDestinations(config: RouterConfig): string[] {
  return config.destinations.map((d) => d.name);
}

export function getRouterDestination(
  config: RouterConfig,
  destinationName: string,
): RouterDestination | undefined {
  return config.destinations.find((d) => d.name === destinationName);
}
