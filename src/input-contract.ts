export type JsonSchema = Record<string, unknown>;

export const DEFAULT_TOOL_INPUT_SCHEMA: JsonSchema = Object.freeze({
  type: 'object',
  properties: {
    prompt: { type: 'string' },
    format: {
      type: 'string',
      enum: ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'],
      default: 'markdown',
    },
  },
  required: ['prompt'],
  additionalProperties: false,
});

export function cloneJsonSchema(schema: JsonSchema): JsonSchema {
  return JSON.parse(JSON.stringify(schema)) as JsonSchema;
}

export function cloneOptionalJsonSchema(schema?: JsonSchema): JsonSchema | undefined {
  return schema === undefined ? undefined : cloneJsonSchema(schema);
}

export const DEFAULT_TOOL_INPUT_SCHEMA_JSON = JSON.stringify(DEFAULT_TOOL_INPUT_SCHEMA);
