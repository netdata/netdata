/**
 * Configuration management with Zod validation and environment variable expansion
 */

import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

import { z } from 'zod';

import type { Configuration } from './types.js';

// Zod schema for configuration validation
const ProviderConfigSchema = z.object({
  apiKey: z.string().optional(),
  baseUrl: z.string().url().optional(),
  headers: z.record(z.string()).optional(),
});

const MCPServerConfigSchema = z.object({
  type: z.enum(['stdio', 'websocket', 'http', 'sse']),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  url: z.string().url().optional(),
  headers: z.record(z.string()).optional(),
  env: z.record(z.string()).optional(),
  enabled: z.boolean().optional(),
});

const DefaultsConfigSchema = z.object({
  llmTimeout: z.number().positive().optional(),
  toolTimeout: z.number().positive().optional(),
  temperature: z.number().min(0).max(2).optional(),
  topP: z.number().min(0).max(1).optional(),
  parallelToolCalls: z.boolean().optional(),
});

const ConfigurationSchema = z.object({
  providers: z.record(z.string(), ProviderConfigSchema),
  mcpServers: z.record(z.string(), MCPServerConfigSchema),
  accounting: z.object({
    file: z.string(),
  }).optional(),
  defaults: DefaultsConfigSchema.optional(),
});

/**
 * Expands environment variables in a string using ${VAR_NAME} syntax
 */
function expandEnvironmentVariables(str: string): string {
  return str.replace(/\$\{([^}]+)\}/g, (_, varName: string) => {
    const value = process.env[varName];
    if (value === undefined) {
      throw new Error(`Environment variable ${varName} is not defined`);
    }
    return value;
  });
}

/**
 * Recursively expands environment variables in an object
 * Per IMPLEMENTATION.md: expand for all strings except under mcpServers.*.(env|headers)
 */
function expandObjectEnvironmentVariables(obj: unknown, path: string[] = []): unknown {
  if (typeof obj === 'string') {
    // Check if we're in mcpServers.*.env or mcpServers.*.headers
    if (path.length >= 3 && 
        path[0] === 'mcpServers' && 
        (path[2] === 'env' || path[2] === 'headers')) {
      // Preserve literal placeholders for server process
      return obj;
    }
    return expandEnvironmentVariables(obj);
  }
  
  if (Array.isArray(obj)) {
    return obj.map((item, index) => expandObjectEnvironmentVariables(item, [...path, index.toString()]));
  }
  
  if (obj !== null && typeof obj === 'object') {
    const result: Record<string, unknown> = {};
    const entries = Object.entries(obj as Record<string, unknown>);
    return processEntriesRecursively(entries, 0, result, path);
  }
  
  return obj;
}

/**
 * Process object entries recursively to avoid for-of loops
 */
function processEntriesRecursively(
  entries: [string, unknown][],
  index: number,
  result: Record<string, unknown>,
  path: string[]
): Record<string, unknown> {
  if (index >= entries.length) {
    return result;
  }
  
  const [key, value] = entries[index] ?? ['', ''];
  result[key] = expandObjectEnvironmentVariables(value, [...path, key]);
  return processEntriesRecursively(entries, index + 1, result, path);
}

/**
 * Resolves the configuration file path according to the priority order:
 * 1. Explicit configPath parameter
 * 2. .ai-agent.json in current directory
 * 3. ~/.ai-agent.json in home directory
 * 4. Throw error if none found
 */
function resolveConfigPath(configPath?: string): string {
  if (configPath !== undefined && configPath !== '') {
    if (!fs.existsSync(configPath)) {
      throw new Error(`Configuration file not found: ${configPath}`);
    }
    return configPath;
  }
  
  // Try current directory
  const currentDirConfig = path.join(process.cwd(), '.ai-agent.json');
  if (fs.existsSync(currentDirConfig)) {
    return currentDirConfig;
  }
  
  // Try home directory
  const homeDirConfig = path.join(os.homedir(), '.ai-agent.json');
  if (fs.existsSync(homeDirConfig)) {
    return homeDirConfig;
  }
  
  throw new Error(
    'Configuration file not found. Tried:\n' +
    `  - ${currentDirConfig}\n` +
    `  - ${homeDirConfig}\n` +
    'Please create a configuration file or specify one with --config'
  );
}

/**
 * Loads and validates the configuration file
 */
export function loadConfiguration(configPath?: string): Configuration {
  const resolvedPath = resolveConfigPath(configPath);
  
  let rawContent: string;
  try {
    rawContent = fs.readFileSync(resolvedPath, 'utf-8');
  } catch (error) {
    throw new Error(`Failed to read configuration file ${resolvedPath}: ${String(error)}`);
  }
  
  let rawConfig: unknown;
  try {
    rawConfig = JSON.parse(rawContent);
  } catch (error) {
    throw new Error(`Invalid JSON in configuration file ${resolvedPath}: ${String(error)}`);
  }
  
  // Expand environment variables
  let expandedConfig: unknown;
  try {
    expandedConfig = expandObjectEnvironmentVariables(rawConfig);
  } catch (error) {
    throw new Error(`Environment variable expansion failed in ${resolvedPath}: ${String(error)}`);
  }
  
  // Validate schema
  const result = ConfigurationSchema.safeParse(expandedConfig);
  if (!result.success) {
    const errorMessages = result.error.errors.map(
      err => `  ${err.path.join('.')}: ${err.message}`
    ).join('\n');
    throw new Error(`Configuration validation failed in ${resolvedPath}:\n${errorMessages}`);
  }
  
  return result.data;
}

/**
 * Validates that required providers are defined in configuration
 */
export function validateProviders(config: Configuration, providers: string[]): void {
  const missingProviders = providers.filter(provider => !(provider in config.providers));
  if (missingProviders.length > 0) {
    throw new Error(`Unknown providers: ${missingProviders.join(', ')}. Available providers: ${Object.keys(config.providers).join(', ')}`);
  }
}

/**
 * Validates that required MCP servers are defined in configuration
 */
export function validateMCPServers(config: Configuration, mcpServers: string[]): void {
  const missingServers = mcpServers.filter(server => !(server in config.mcpServers));
  if (missingServers.length > 0) {
    throw new Error(`Unknown MCP servers: ${missingServers.join(', ')}. Available servers: ${Object.keys(config.mcpServers).join(', ')}`);
  }
}

/**
 * Validates that stdin is not used for both system and user prompts
 */
export function validatePrompts(systemPrompt: string, userPrompt: string): void {
  if (systemPrompt === '-' && userPrompt === '-') {
    throw new Error('Cannot use stdin (-) for both system and user prompts');
  }
}