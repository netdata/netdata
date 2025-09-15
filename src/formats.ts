export type OutputFormatId = 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';

interface OutputFormat {
  id: OutputFormatId;
  description: string;
}

const OUTPUT_FORMATS: Record<OutputFormatId, OutputFormat> = {
  markdown: { id: 'markdown', description: 'markdown' },
  'markdown+mermaid': { id: 'markdown+mermaid', description: 'markdown with mermaid charts' },
  'slack-block-kit': { id: 'slack-block-kit', description: [
    'Slack Block Kit only. Finish by calling agent__final_report with `messages` (no plain content).',
    'Allowed blocks: header (plain_text ≤150), divider, section (mrkdwn ≤2000), context (mrkdwn ≤2000, ≤10), fields (≤10, mrkdwn ≤2000).',
    'Do not use markdown headers (#, ##), tables, HTML, or images.'
  ].join(' ') },
  tty: { id: 'tty', description: 'Fixed-width monospaced terminal, with ANSI colors, and ASCII-art for tables and diagrams. No markdown. ' +
     'When adding ANSI colors, emit literal \\x1b[...m codes (e.g., \\x1b[33m for yellow) instead of raw ESC characters. ' +
     'IMPORTANT: Do not create unecessary boxes that wrap the content, let it breathe - on tables that need vertical alignment remember that the ANSI colors do not count in the width. Do not wrap long lines - let them wrap naturally depending on terminal size.' },
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

