import type { ToolOutputStats } from './types.js';

export function formatHandleMessage(handle: string, stats: ToolOutputStats): string {
  return `Tool output is too large (${String(stats.bytes)} bytes, ${String(stats.lines)} lines, ${String(stats.tokens)} tokens).\nCall tool_output(handle = "${handle}", extract = "what to extract").\nProvide precise and detailed instructions in \`extract\` about what you are looking for.`;
}

export function formatToolOutputSuccess(args: {
  toolName: string;
  handle: string;
  mode: string;
  body: string;
}): string {
  return `ABSTRACT FROM TOOL OUTPUT ${args.toolName} WITH HANDLE ${args.handle}, STRATEGY:${args.mode}:\n\n${args.body}`;
}

export function formatToolOutputFailure(args: {
  toolName: string;
  handle: string;
  mode: string;
  error: string;
}): string {
  return `TOOL_OUTPUT FAILED FOR ${args.toolName} WITH HANDLE ${args.handle}, STRATEGY:${args.mode}:\n\n${args.error}`;
}
