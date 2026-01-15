import { describe, expect, it } from 'vitest';

import type { AgentMetadata, AgentRegistry } from '../../agent-registry.js';

import { refreshModelIdMap } from '../../headends/model-map-utils.js';

type RegistryLike = Pick<AgentRegistry, 'list'>;

const meta = (id: string): AgentMetadata => ({
  id,
  promptPath: `${id}.ai`,
  input: { format: 'text', schema: {} },
});

describe('refreshModelIdMap', () => {
  it('clears and repopulates the map using buildModelId', () => {
    const registry: RegistryLike = {
      list: () => [meta('a'), meta('b')],
    };
    const map = new Map<string, string>([['stale', 'stale']]);
    refreshModelIdMap(
      registry,
      map,
      (agent, seen) => {
        const name = `${agent.id}${seen.has(agent.id) ? '-dup' : ''}`;
        seen.add(agent.id);
        return name;
      },
    );

    expect(map.has('stale')).toBe(false);
    expect(map.get('a')).toBe('a');
    expect(map.get('b')).toBe('b');
  });
});
