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
