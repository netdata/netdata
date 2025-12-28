import type { ToolKind } from '../tools/types.js';
import type { Configuration, RestToolConfig } from '../types.js';

import { sanitizeToolName } from '../utils.js';

export interface ToolCacheResolver {
  resolveTtlMs: (input: { kind: ToolKind; namespace: string; tool: string }) => number | undefined;
}

const resolveToolOverride = (toolsCache: Record<string, number> | undefined, toolName: string): number | undefined => {
  if (toolsCache === undefined) return undefined;
  if (Object.prototype.hasOwnProperty.call(toolsCache, toolName)) {
    return toolsCache[toolName];
  }
  const sanitized = sanitizeToolName(toolName);
  if (sanitized !== toolName && Object.prototype.hasOwnProperty.call(toolsCache, sanitized)) {
    return toolsCache[sanitized];
  }
  return undefined;
};

const resolveRestToolCache = (restTools: Record<string, RestToolConfig> | undefined, toolName: string): number | undefined => {
  if (restTools === undefined) return undefined;
  if (Object.prototype.hasOwnProperty.call(restTools, toolName)) {
    return restTools[toolName].cache;
  }
  const sanitized = sanitizeToolName(toolName);
  if (sanitized !== toolName && Object.prototype.hasOwnProperty.call(restTools, sanitized)) {
    return restTools[sanitized].cache;
  }
  return undefined;
};

const hasPositiveCache = (value: unknown): boolean => typeof value === 'number' && Number.isFinite(value) && value > 0;

const hasToolCacheConfig = (toolsCache: Record<string, number> | undefined): boolean => (
  toolsCache !== undefined && Object.values(toolsCache).some((value) => hasPositiveCache(value))
);

const hasMcpCacheConfig = (config: Configuration): boolean => (
  Object.values(config.mcpServers).some((server) => hasPositiveCache(server.cache) || hasToolCacheConfig(server.toolsCache))
);

const hasRestCacheConfig = (config: Configuration): boolean => (
  Object.values(config.restTools ?? {}).some((tool) => hasPositiveCache(tool.cache))
);

export const createToolCacheResolver = (config: Configuration): ToolCacheResolver | undefined => {
  if (!hasMcpCacheConfig(config) && !hasRestCacheConfig(config)) {
    return undefined;
  }
  return {
    resolveTtlMs: ({ kind, namespace, tool }) => {
      if (kind === 'mcp') {
        const server = Object.prototype.hasOwnProperty.call(config.mcpServers, namespace)
          ? config.mcpServers[namespace]
          : undefined;
        if (server === undefined) return undefined;
        const toolOverride = resolveToolOverride(server.toolsCache, tool);
        if (toolOverride !== undefined) return toolOverride;
        return server.cache;
      }
      if (kind === 'rest') {
        return resolveRestToolCache(config.restTools, tool);
      }
      return undefined;
    },
  };
};
