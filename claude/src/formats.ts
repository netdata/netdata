export type OutputFormatId = 'markdown' | 'markdown+mermaid' | 'slack' | 'tty' | 'pipe' | 'json' | 'sub-agent';

export interface OutputFormat {
  id: OutputFormatId;
  description: string;
}

export const OUTPUT_FORMATS: Record<OutputFormatId, OutputFormat> = {
  markdown: { id: 'markdown', description: 'markdown' },
  'markdown+mermaid': { id: 'markdown+mermaid', description: 'markdown with mermaid charts' },
  slack: { id: 'slack', description: 'Format your report in Slack mrkdwn, not GitHub markdown. Your report MUST comply with Slack message formatting and limits and be visually appealing.' },
  tty: { id: 'tty', description: 'fixed-width monospaced terminal, with ANSI colors, use ASCII-art for tables and diagrams - no markdown' },
  pipe: { id: 'pipe', description: 'plain text' },
  json: { id: 'json', description: 'json' },
  'sub-agent': { id: 'sub-agent', description: 'agent to agent communication, use optimal format' },
};

export function describeFormat(id: OutputFormatId): string {
  return OUTPUT_FORMATS[id].description;
}

export function resolveFormatIdForCli(override: string | undefined, expectedJson: boolean, isTTY: boolean): OutputFormatId {
  if (expectedJson) return 'json';
  if (typeof override === 'string' && override in OUTPUT_FORMATS) return override as OutputFormatId;
  return isTTY ? 'tty' : 'pipe';
}

export function resolveFormatIdForSlack(expectedJson: boolean): OutputFormatId {
  return expectedJson ? 'json' : 'slack';
}

export function resolveFormatIdForApi(override: string | undefined, expectedJson: boolean): OutputFormatId {
  if (expectedJson) return 'json';
  if (typeof override === 'string' && override in OUTPUT_FORMATS) return override as OutputFormatId;
  return 'markdown';
}
