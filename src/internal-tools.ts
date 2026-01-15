const RESERVED_INTERNAL_AGENT_NAMES = new Set<string>([
  'task_status',    // agent__task_status
  'final_report',   // agent__final_report
  'batch',          // agent__batch
  'tool_output',    // tool_output
]);

/** Canonical internal tool name for final report submission */
export const FINAL_REPORT_TOOL = 'agent__final_report';

/** Aliases accepted for final report tool (normalized forms) */
export const FINAL_REPORT_TOOL_ALIASES = new Set<string>(['agent__final_report', 'agent-final-report']);

export function isReservedAgentName(name: string): boolean {
  return RESERVED_INTERNAL_AGENT_NAMES.has(name);
}
