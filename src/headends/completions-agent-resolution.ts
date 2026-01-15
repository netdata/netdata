import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';

type RegistryLike = Pick<AgentRegistry, 'getMetadata' | 'resolveAgentId'>;

export const resolveCompletionsAgent = (
  model: string,
  registry: RegistryLike,
  modelIdMap: Map<string, string>,
  refreshModelMap: () => void,
): AgentMetadata | undefined => {
  refreshModelMap();
  const direct = modelIdMap.get(model);
  if (direct !== undefined) {
    return registry.getMetadata(direct);
  }
  const resolved = registry.resolveAgentId(model);
  if (resolved !== undefined) {
    return registry.getMetadata(resolved);
  }
  return undefined;
};
