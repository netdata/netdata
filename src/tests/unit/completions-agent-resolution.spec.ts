import { describe, expect, it, vi } from 'vitest';

import type { AgentMetadata, AgentRegistry } from '../../agent-registry.js';

import { resolveCompletionsAgent } from '../../headends/completions-agent-resolution.js';

type RegistryLike = Pick<AgentRegistry, 'getMetadata' | 'resolveAgentId'>;

const buildMeta = (id: string): AgentMetadata => ({
  id,
  promptPath: `${id}.ai`,
  input: { format: 'text', schema: {} },
});

describe('resolveCompletionsAgent', () => {
  it('resolves via modelIdMap when available', () => {
    const meta = buildMeta('agent-1');
    const registry: RegistryLike = {
      getMetadata: (agentId) => (agentId === meta.id ? meta : undefined),
      resolveAgentId: () => undefined,
    };
    const refresh = vi.fn();
    const modelIdMap = new Map<string, string>([['model-a', meta.id]]);

    const resolved = resolveCompletionsAgent('model-a', registry, modelIdMap, refresh);

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(resolved).toEqual(meta);
  });

  it('falls back to registry resolution when modelIdMap misses', () => {
    const meta = buildMeta('agent-2');
    const registry: RegistryLike = {
      getMetadata: (agentId) => (agentId === meta.id ? meta : undefined),
      resolveAgentId: (alias) => (alias === 'alias' ? meta.id : undefined),
    };
    let refreshed = 0;
    const modelIdMap = new Map<string, string>();

    const resolved = resolveCompletionsAgent('alias', registry, modelIdMap, () => { refreshed += 1; });

    expect(refreshed).toBe(1);
    expect(resolved).toEqual(meta);
  });
});
