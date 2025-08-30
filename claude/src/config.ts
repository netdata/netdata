import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { z } from 'zod';

import type { Configuration } from './types.js';

const ProviderConfigSchema = z.object({
  apiKey: z.string().optional(),
  baseUrl: z.string().url().optional(),
  headers: z.record(z.string()).optional(),
  custom: z.record(z.unknown()).optional(),
  mergeStrategy: z.enum(['overlay','override','deep']).optional(),
});

const MCPServerConfigSchema = z.object({
  type: z.enum(['stdio', 'websocket', 'http', 'sse']),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  url: z.string().url().optional(),
  headers: z.record(z.string()).optional(),
  env: z.record(z.string()).optional(),
  enabled: z.boolean().optional(),
  toolSchemas: z.record(z.any()).optional(),
});

const ConfigurationSchema = z.object({
  providers: z.record(z.string(), ProviderConfigSchema),
  mcpServers: z.record(z.string(), MCPServerConfigSchema),
  accounting: z.object({ file: z.string() }).optional(),
  defaults: z
    .object({
      llmTimeout: z.number().positive().optional(),
      toolTimeout: z.number().positive().optional(),
      temperature: z.number().min(0).max(2).optional(),
      topP: z.number().min(0).max(1).optional(),
      parallelToolCalls: z.boolean().optional(),
      maxToolTurns: z.number().int().positive().optional(),
    })
    .optional(),
});


function expandEnv(str: string): string {
  return str.replace(/\$\{([^}]+)\}/g, (_m: string, name: string) => (process.env[name] ?? ''));
}

function expandDeep(obj: unknown, chain: string[] = []): unknown {
  if (chain.length > 10) {
    throw new Error('Environment variable expansion depth exceeded');
  }
  if (typeof obj === 'string') {
    return expandEnv(obj);
  }
  if (Array.isArray(obj)) {
    return obj.map(item => expandDeep(item, chain));
  }
  if (obj && typeof obj === 'object') {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
      result[key] = expandDeep(value, [...chain, key]);
    }
    return result;
  }
  return obj;
}

export function loadConfiguration(configPath?: string): Configuration {
  const defaultConfigPath = path.join(os.homedir(), '.ai-agent.json');
  const finalConfigPath = configPath ?? defaultConfigPath;
  
  let rawConfig: unknown;
  
  try {
    if (fs.existsSync(finalConfigPath)) {
      const configContent = fs.readFileSync(finalConfigPath, 'utf8');
      rawConfig = JSON.parse(configContent);
    } else {
      return {
        providers: {},
        mcpServers: {},
      };
    }
  } catch (error) {
    throw new Error(
      `Failed to load configuration from ${finalConfigPath}: ${error instanceof Error ? error.message : 'Unknown error'}`
    );
  }
  
  const expandedConfig = expandDeep(rawConfig);
  
  try {
    return ConfigurationSchema.parse(expandedConfig);
  } catch (error) {
    if (error instanceof z.ZodError) {
      const issues = error.issues.map(
        issue => `${issue.path.join('.')}: ${issue.message}`
      ).join(', ');
      throw new Error(`Configuration validation failed: ${issues}`);
    }
    throw new Error(`Configuration validation failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export function validateProviders(providers: string[], config: Configuration): void {
  for (const providerName of providers) {
    if (!(providerName in config.providers)) {
      throw new Error(`Provider "${providerName}" is not configured`);
    }
  }
}

export function validateMCPServers(servers: string[], config: Configuration): void {
  for (const serverName of servers) {
    if (!(serverName in config.mcpServers)) {
      throw new Error(`MCP server "${serverName}" is not configured`);
    }
    
    const serverConfig = config.mcpServers[serverName];
    if (serverConfig?.enabled === false) {
      throw new Error(`MCP server "${serverName}" is disabled`);
    }
  }
}

export function validatePrompts(systemPrompt: string, userPrompt: string): void {
  if (!systemPrompt.trim()) {
    throw new Error('System prompt cannot be empty');
  }
  if (!userPrompt.trim()) {
    throw new Error('User prompt cannot be empty');
  }
}