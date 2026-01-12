import type { FrontmatterOptions } from "../frontmatter.js";

export interface HandoffConfig {
  trigger: HandoffTrigger;
  targetAgent: string;
  condition?: HandoffCondition;
  description?: string;
}

export type HandoffTrigger =
  | { kind: "explicit" }
  | { kind: "keyword"; keywords: string[] }
  | { kind: "pattern"; regex: string }
  | { kind: "tool_failure"; toolNames?: string[] }
  | { kind: "max_turns"; maxTurns: number };

export interface HandoffCondition {
  type: "confidence" | "error_rate" | "task_type" | "custom";
  threshold?: number;
  expectedTypes?: string[];
  customCheck?: string;
}

export interface AdvisorConfig {
  agent: string;
  when: AdvisorTrigger;
  priority?: number;
  maxConsultationsPerTurn?: number;
}

export type AdvisorTrigger =
  | { kind: "always" }
  | { kind: "task_type"; taskTypes: string[] }
  | { kind: "keyword"; keywords: string[] }
  | { kind: "explicit" };

export interface RouterDestination {
  name: string;
  agent: string;
  taskTypes: string[];
  description?: string;
  defaultPriority?: number;
}

export interface RouterConfig {
  destinations: RouterDestination[];
  defaultDestination?: string;
  routingStrategy: "round_robin" | "priority" | "task_type" | "dynamic";
}

export interface OrchestrationConfig {
  handoff?: HandoffConfig[];
  advisors?: AdvisorConfig[];
  router?: RouterConfig;
}

export function parseOrchestrationOptions(
  opts: FrontmatterOptions,
): OrchestrationConfig {
  const result: OrchestrationConfig = {};

  if (opts.handoff !== undefined) {
    const handoffs = Array.isArray(opts.handoff)
      ? opts.handoff
      : [opts.handoff];
    result.handoff = handoffs.map((h) => parseHandoffEntry(h));
  }

  if (opts.advisors !== undefined) {
    const advisors = Array.isArray(opts.advisors)
      ? opts.advisors
      : [opts.advisors];
    result.advisors = advisors.map((a) => parseAdvisorEntry(a));
  }

  if (opts.routerDestinations !== undefined) {
    const destinations = Array.isArray(opts.routerDestinations)
      ? opts.routerDestinations
      : [opts.routerDestinations];
    result.router = parseRouterDestinations(destinations);
  }

  return result;
}

function parseHandoffEntry(entry: string): HandoffConfig {
  if (entry.startsWith("keyword:")) {
    const keywords = entry
      .slice(8)
      .split(",")
      .map((k) => k.trim());
    return { trigger: { kind: "keyword", keywords }, targetAgent: "" };
  }
  if (entry.startsWith("pattern:")) {
    const regex = entry.slice(8);
    return { trigger: { kind: "pattern", regex }, targetAgent: "" };
  }
  return { trigger: { kind: "explicit" }, targetAgent: entry };
}

function parseAdvisorEntry(entry: string): AdvisorConfig {
  if (entry.startsWith("task_type:")) {
    const taskTypes = entry
      .slice(10)
      .split(",")
      .map((t) => t.trim());
    return { agent: "", when: { kind: "task_type", taskTypes } };
  }
  if (entry.startsWith("keyword:")) {
    const keywords = entry
      .slice(8)
      .split(",")
      .map((k) => k.trim());
    return { agent: "", when: { kind: "keyword", keywords } };
  }
  return { agent: entry, when: { kind: "always" } };
}

export function parseRouterDestinations(entries: string[]): RouterConfig {
  const destinations: RouterDestination[] = entries.map((entry) => {
    const [name, ...rest] = entry.split(":");
    const agent = rest.join(":") || "";
    return { name: name.trim(), agent, taskTypes: [] };
  });
  return {
    destinations,
    routingStrategy: "task_type",
  };
}
