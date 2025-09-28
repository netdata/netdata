import { clampToolName, sanitizeToolName as coreSanitizeToolName } from './utils.js';

export interface AgentSchemaSummary {
  id: string;
  toolName?: string;
  description?: string;
  usage?: string;
  input: { format: 'json' | 'text'; schema: Record<string, unknown> };
  outputSchema?: Record<string, unknown>;
}

const OPENAI_NAME_LIMIT = 64;

const cloneSchema = (schema: Record<string, unknown> | undefined): Record<string, unknown> | undefined => {
  if (schema === undefined) return undefined;
  return JSON.parse(JSON.stringify(schema)) as Record<string, unknown>;
};

const sanitizeToolName = (name: string): string => {
  const sanitized = coreSanitizeToolName(name);
  const normalized = sanitized.length > 0 ? sanitized.toLowerCase() : sanitized;
  const { name: clamped } = clampToolName(normalized, OPENAI_NAME_LIMIT);
  return clamped;
};

const buildDescription = (agent: AgentSchemaSummary): string | undefined => {
  if (typeof agent.description === 'string' && agent.description.trim().length > 0) {
    const trimmed = agent.description.trim();
    return trimmed.length > 512 ? `${trimmed.slice(0, 508)}...` : trimmed;
  }
  if (typeof agent.usage === 'string' && agent.usage.trim().length > 0) {
    const trimmed = agent.usage.trim();
    return trimmed.length > 512 ? `${trimmed.slice(0, 508)}...` : trimmed;
  }
  return undefined;
};

export const resolveToolName = (agent: AgentSchemaSummary): string => {
  const preferred = typeof agent.toolName === 'string' && agent.toolName.length > 0
    ? agent.toolName
    : agent.id;
  return sanitizeToolName(preferred);
};

export interface OpenAIToolDefinition {
  type: 'function';
  function: {
    name: string;
    description?: string;
    parameters: Record<string, unknown>;
    strict?: boolean;
  };
}

export const toOpenAIToolDefinition = (agent: AgentSchemaSummary): OpenAIToolDefinition => ({
  type: 'function',
  function: {
    name: resolveToolName(agent),
    description: buildDescription(agent),
    parameters: cloneSchema(agent.input.schema) ?? {},
    strict: true,
  }
});

export interface AnthropicToolDefinition {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

export const toAnthropicToolDefinition = (agent: AgentSchemaSummary): AnthropicToolDefinition => ({
  name: resolveToolName(agent),
  description: buildDescription(agent),
  input_schema: cloneSchema(agent.input.schema) ?? {},
});

export interface McpToolDefinition {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>;
  outputSchema?: Record<string, unknown>;
}

export const toMcpToolDefinition = (agent: AgentSchemaSummary): McpToolDefinition => ({
  name: resolveToolName(agent),
  description: buildDescription(agent),
  inputSchema: cloneSchema(agent.input.schema) ?? {},
  outputSchema: cloneSchema(agent.outputSchema),
});
