const RESERVED_INTERNAL_AGENT_NAMES = new Set<string>([
  'task_status',    // agent__task_status
  'final_report',   // agent__final_report
  'batch',          // agent__batch
  'tool_output',    // tool_output
]);

export function isReservedAgentName(name: string): boolean {
  return RESERVED_INTERNAL_AGENT_NAMES.has(name);
}
