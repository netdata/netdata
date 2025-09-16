const RESERVED_INTERNAL_AGENT_NAMES = new Set<string>([
  'progress_report', // agent__progress_report
  'final_report',    // agent__final_report
  'batch',           // agent__batch
]);

export function isReservedAgentName(name: string): boolean {
  return RESERVED_INTERNAL_AGENT_NAMES.has(name);
}

