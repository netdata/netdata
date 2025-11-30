export type OutputFormatId = 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';

interface OutputFormat {
  id: OutputFormatId;
  toolDescription: string;
  promptValue: string;
  parameterDescription: string;
  // Schema for formats requiring structured JSON output (shown in xml-final mode)
  inputSchema?: Record<string, unknown>;
}

// Slack Block Kit schema - shared between native tool and xml-final modes
const SLACK_BLOCK_KIT_SCHEMA: Record<string, unknown> = {
  type: 'object',
  required: ['report_format', 'messages'],
  properties: {
    report_format: { type: 'string', const: 'slack-block-kit' },
    messages: {
      type: 'array',
      minItems: 1,
      description: 'Array of Slack messages, each containing blocks',
      items: {
        type: 'object',
        required: ['blocks'],
        properties: {
          blocks: {
            type: 'array',
            description: 'Slack Block Kit blocks: section, header, divider, context',
            items: {
              type: 'object',
              description: 'Block types: section (mrkdwn text ≤2900 chars), header (plain_text ≤150), divider, context (mrkdwn ≤2000)'
            }
          }
        }
      }
    },
    metadata: { type: 'object' }
  }
};

const OUTPUT_FORMATS: Record<OutputFormatId, OutputFormat> = {
  markdown: {
    id: 'markdown',
    toolDescription: 'GitHub Markdown.',
    promptValue: 'GitHub Markdown',
    parameterDescription: 'Render the final report in GitHub Markdown.'
  },
  'markdown+mermaid': {
    id: 'markdown+mermaid',
    toolDescription: 'GitHub Markdown with Mermaid diagrams.',
    promptValue: 'GiHub Markdown with Mermaid diagrams',
    parameterDescription: 'Render GitHub Markdown and include Mermaid diagrams where useful.'
  },
  'slack-block-kit': {
    id: 'slack-block-kit',
    toolDescription: 'Slack Block Kit payload.',
    promptValue: 'Slack Block Kit JSON array of messages (not raw text or GitHub markdown).',
    parameterDescription: 'Produce Slack Block Kit array of messages. Use multiple Block Kit messages and Slack-mrkdwn sections/context (≤2000 chars), headers (plain_text ≤150), dividers, and fields (≤10). Do not emit raw text or GitHub markdown.',
    inputSchema: SLACK_BLOCK_KIT_SCHEMA
  },
  tty: {
    id: 'tty',
    toolDescription: 'TTY-compatible monospaced text with ANSI colours. Emit literal "\\x1b[...m" sequences for colour codes (not raw ESC characters).',
    promptValue: 'a TTY-compatible plain monospaced text response. Use literal "\\x1b[...m" sequences for ANSI colours and avoid decorative boxes. Do not output markdown. Do not wrap long lines.',
    parameterDescription: 'Render output for a monospaced terminal using ANSI colour codes. Emit literal "\\x1b[...m" sequences (do not insert raw ESC characters) for colour styling. No Markdown tables. No bounded boxes. Do not wrap long lines.'
  },
  pipe: {
    id: 'pipe',
    toolDescription: 'Plain text (no formatting).',
    promptValue: 'Plain text without any formatting or markdown. Do not wrap long lines.',
    parameterDescription: 'Return unformatted plain text suitable for shell piping into other tools. Do not wrap long lines.'
  },
  json: {
    id: 'json',
    toolDescription: 'JSON object.',
    promptValue: 'json',
    parameterDescription: 'Return JSON that matches the declared schema exactly.'
  },
  'sub-agent': {
    id: 'sub-agent',
    toolDescription: 'Internal agent-to-agent exchange format.',
    promptValue: 'Internal agent-to-agent exchange format (not user-facing).',
    parameterDescription: 'Use the optimal internal sub-agent communication format (not user-facing).'
  },
};

const isOutputFormatId = (value: string): value is OutputFormatId => Object.prototype.hasOwnProperty.call(OUTPUT_FORMATS, value);

export function describeFormat(id: OutputFormatId): string {
  return OUTPUT_FORMATS[id].toolDescription;
}

export function formatPromptValue(id: OutputFormatId): string {
  return OUTPUT_FORMATS[id].promptValue;
}

export function describeFormatParameter(id: OutputFormatId): string {
  return OUTPUT_FORMATS[id].parameterDescription;
}

export function getFormatSchema(id: OutputFormatId): Record<string, unknown> | undefined {
  return OUTPUT_FORMATS[id].inputSchema;
}

export function resolveFormatIdForCli(override: string | undefined, expectedJson: boolean, isTTY: boolean): OutputFormatId {
  if (expectedJson) return 'json';
  const normalized = typeof override === 'string' ? override.trim().toLowerCase() : undefined;
  if (normalized !== undefined && normalized.length > 0) {
    const aliasMap: Record<string, OutputFormatId> = {
      text: 'pipe',
      plain: 'pipe',
      plaintext: 'pipe',
      'plain-text': 'pipe',
      ansi: 'tty',
      terminal: 'tty',
    };
    const candidate = aliasMap[normalized] ?? normalized;
    if (isOutputFormatId(candidate)) {
      return candidate;
    }
  }
  return isTTY ? 'tty' : 'pipe';
}
