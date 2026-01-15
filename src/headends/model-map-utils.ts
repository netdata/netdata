import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';

type RegistryLike = Pick<AgentRegistry, 'list'>;

export const refreshModelIdMap = (
  registry: RegistryLike,
  modelIdMap: Map<string, string>,
  buildModelId: (meta: AgentMetadata, seen: Set<string>) => string,
): void => {
  modelIdMap.clear();
  const seen = new Set<string>();
  registry.list().forEach((meta) => {
    const modelId = buildModelId(meta, seen);
    modelIdMap.set(modelId, meta.id);
  });
};
