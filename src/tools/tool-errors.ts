export type ToolErrorKind =
  | 'unknown_tool'
  | 'not_permitted'
  | 'invalid_parameters'
  | 'limit_exceeded'
  | 'canceled'
  | 'timeout'
  | 'transport_error'
  | 'execution_error'
  | 'internal_error';

export interface ToolErrorMeaning {
  executed: boolean;
  summary: string;
}

export const TOOL_ERROR_KIND_MEANINGS: Record<ToolErrorKind, ToolErrorMeaning> = {
  unknown_tool: {
    executed: false,
    summary: 'Tool name does not exist in the current session.',
  },
  not_permitted: {
    executed: false,
    summary: 'Tool call rejected by allowlist or policy.',
  },
  invalid_parameters: {
    executed: false,
    summary: 'Tool schema validation failed before execution.',
  },
  limit_exceeded: {
    executed: false,
    summary: 'Per-turn tool call limit reached before execution.',
  },
  canceled: {
    executed: false,
    summary: 'Tool call aborted before execution.',
  },
  timeout: {
    executed: true,
    summary: 'Tool call timed out after execution started.',
  },
  transport_error: {
    executed: true,
    summary: 'Tool transport/connection error during execution.',
  },
  execution_error: {
    executed: true,
    summary: 'Tool execution failed after being invoked.',
  },
  internal_error: {
    executed: true,
    summary: 'Unexpected error during tool execution.',
  },
};

export class ToolExecutionError extends Error {
  readonly kind: ToolErrorKind;
  readonly code?: string;
  readonly details?: Record<string, unknown>;

  constructor(kind: ToolErrorKind, message: string, opts?: { code?: string; details?: Record<string, unknown> }) {
    super(message);
    this.name = 'ToolExecutionError';
    this.kind = kind;
    if (opts?.code !== undefined) {
      this.code = opts.code;
    }
    if (opts?.details !== undefined) {
      this.details = opts.details;
    }
  }
}

export const isToolExecutionError = (value: unknown): value is ToolExecutionError =>
  value instanceof ToolExecutionError;

export const isToolExecutedErrorKind = (kind: ToolErrorKind): boolean =>
  TOOL_ERROR_KIND_MEANINGS[kind].executed;

const normalizeErrorMessage = (value: unknown): string => {
  if (value instanceof Error && typeof value.message === 'string') return value.message;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return value.toString();
  }
  if (value === null) return 'null';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return '[unserializable-error]';
    }
  }
  return 'unknown_error';
};

export const toToolExecutionError = (
  value: unknown,
  fallbackKind: ToolErrorKind = 'internal_error'
): ToolExecutionError => {
  if (isToolExecutionError(value)) return value;
  return new ToolExecutionError(fallbackKind, normalizeErrorMessage(value));
};
