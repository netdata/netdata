import type { LoadedAgent } from "../agent-loader.js";

export interface CycleDetectionResult {
  hasCycle: boolean;
  cyclePath?: string[];
  errorMessage?: string;
}

function buildAdjacencyList(
  agents: Map<string, LoadedAgent>,
): Map<string, string[]> {
  const adj = new Map<string, string[]>();

  // eslint-disable-next-line functional/no-loop-statements
  for (const [agentId, agent] of agents) {
    const orchestration = (
      agent as {
        orchestration?: { handoff?: string[]; routerDestinations?: string[] };
      }
    ).orchestration;
    if (orchestration === undefined) continue;

    const neighbors: string[] = [];

    const handoffs = orchestration.handoff;
    if (handoffs !== undefined) {
      // eslint-disable-next-line functional/no-loop-statements
      for (const handoff of handoffs) {
        if (agents.has(handoff)) {
          neighbors.push(handoff);
        }
      }
    }

    const destinations = orchestration.routerDestinations;
    if (destinations !== undefined) {
      // eslint-disable-next-line functional/no-loop-statements
      for (const dest of destinations) {
        if (agents.has(dest)) {
          neighbors.push(dest);
        }
      }
    }

    if (neighbors.length > 0) {
      adj.set(agentId, neighbors);
    }
  }

  return adj;
}

function dfs(
  node: string,
  adj: Map<string, string[]>,
  visited: Set<string>,
  recursionStack: Set<string>,
  path: string[],
): string | null {
  visited.add(node);
  recursionStack.add(node);
  path.push(node);

  const neighbors = adj.get(node) ?? [];
  // eslint-disable-next-line functional/no-loop-statements
  for (const neighbor of neighbors) {
    if (!visited.has(neighbor)) {
      const result = dfs(neighbor, adj, visited, recursionStack, path);
      if (result !== null) {
        return result;
      }
    } else if (recursionStack.has(neighbor)) {
      const cycleStartIndex = path.indexOf(neighbor);
      const cycle = path.slice(cycleStartIndex);
      cycle.push(neighbor);
      return cycle.join(" -> ");
    }
  }

  path.pop();
  recursionStack.delete(node);
  return null;
}

export function detectOrchestrationCycles(
  agents: Map<string, LoadedAgent>,
): CycleDetectionResult {
  const adj = buildAdjacencyList(agents);
  const visited = new Set<string>();
  const recursionStack = new Set<string>();

  // eslint-disable-next-line functional/no-loop-statements
  for (const agentId of adj.keys()) {
    if (!visited.has(agentId)) {
      const path: string[] = [];
      const cycle = dfs(agentId, adj, visited, recursionStack, path);
      if (cycle !== null) {
        return {
          hasCycle: true,
          cyclePath: cycle.split(" -> "),
          errorMessage: `Orchestration cycle detected: ${cycle}. Please remove the cycle to enable handoffs and routing.`,
        };
      }
    }
  }

  return { hasCycle: false };
}

export function validateOrchestrationGraph(
  agents: Map<string, LoadedAgent>,
): void {
  const result = detectOrchestrationCycles(agents);
  if (result.hasCycle) {
    throw new Error(
      result.errorMessage ?? "Unknown orchestration cycle detected",
    );
  }
}
