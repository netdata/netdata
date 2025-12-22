import { parseJsonValueDetailed } from './utils.js';

const SLACK_LIMITS = {
  maxMessages: 20,
  maxBlocks: 50,
  maxFields: 10,
  maxContext: 10,
  sectionText: 2900,
  headerText: 150,
  fieldText: 2000,
  contextText: 2000,
} as const;

export const SLACK_BLOCK_KIT_SCHEMA: Record<string, unknown> = {
  type: 'array',
  minItems: 1,
  maxItems: SLACK_LIMITS.maxMessages,
  items: {
    type: 'object',
    additionalProperties: true,
    required: ['blocks'],
    properties: {
      blocks: {
        type: 'array',
        minItems: 1,
        maxItems: SLACK_LIMITS.maxBlocks,
        items: {
          oneOf: [
            {
              type: 'object',
              additionalProperties: true,
              required: ['type'],
              properties: {
                type: { const: 'section' },
                text: {
                  type: 'object',
                  additionalProperties: true,
                  required: ['type', 'text'],
                  properties: {
                    type: { const: 'mrkdwn' },
                    text: { type: 'string', maxLength: SLACK_LIMITS.sectionText },
                  },
                },
                fields: {
                  type: 'array',
                  maxItems: SLACK_LIMITS.maxFields,
                  items: {
                    type: 'object',
                    additionalProperties: true,
                    required: ['type', 'text'],
                    properties: {
                      type: { const: 'mrkdwn' },
                      text: { type: 'string', maxLength: SLACK_LIMITS.fieldText },
                    },
                  },
                },
              },
              anyOf: [
                { required: ['text'] },
                { required: ['fields'] },
              ],
            },
            {
              type: 'object',
              additionalProperties: true,
              required: ['type'],
              properties: { type: { const: 'divider' } },
            },
            {
              type: 'object',
              additionalProperties: true,
              required: ['type', 'text'],
              properties: {
                type: { const: 'header' },
                text: {
                  type: 'object',
                  additionalProperties: true,
                  required: ['type', 'text'],
                  properties: {
                    type: { const: 'plain_text' },
                    text: { type: 'string', maxLength: SLACK_LIMITS.headerText },
                  },
                },
              },
            },
            {
              type: 'object',
              additionalProperties: true,
              required: ['type', 'elements'],
              properties: {
                type: { const: 'context' },
                elements: {
                  type: 'array',
                  minItems: 1,
                  maxItems: SLACK_LIMITS.maxContext,
                  items: {
                    type: 'object',
                    additionalProperties: true,
                    required: ['type', 'text'],
                    properties: {
                      type: { const: 'mrkdwn' },
                      text: { type: 'string', maxLength: SLACK_LIMITS.contextText },
                    },
                  },
                },
              },
            },
          ],
        },
      },
    },
  },
};

export type SlackMrkdwnRepair =
  | 'markdownBold'
  | 'markdownStrike'
  | 'markdownHeading'
  | 'markdownLink'
  | 'markdownTable'
  | 'escapedNewlines'
  | 'codeFenceLanguage'
  | 'escapeEntities'
  | 'stripFormatting'
  | 'stripHeadingMarkers'
  | 'stripMarkdownLinks'
  | 'truncateText'
  | 'dropInvalidBlock'
  | 'dropInvalidMessage'
  | 'truncateBlocks'
  | 'truncateMessages'
  | 'truncateFields'
  | 'truncateContext';

export interface SlackMrkdwnResult {
  text: string;
  repairs: SlackMrkdwnRepair[];
}

export interface SlackMessageNormalizationResult {
  messages: Record<string, unknown>[];
  repairs: SlackMrkdwnRepair[];
}

export interface SlackBlockKitParseResult {
  messages?: unknown[];
  repairs: string[];
  error?: string;
  fallbackText?: string;
  fallbackLooksInvalid: boolean;
}

interface SlackMrkdwnText extends Record<string, unknown> {
  type: 'mrkdwn';
  text: string;
}

const SLACK_ENTITY_INNER = String.raw`(?:https?:\/\/[^>|\s]+(?:\|[^>]+)?|mailto:[^>|\s]+(?:\|[^>]+)?|@[A-Za-z0-9]+(?:\|[^>]+)?|#[A-Za-z0-9]+(?:\|[^>]+)?|!subteam\^[A-Za-z0-9]+(?:\|[^>]+)?|!here|!channel|!everyone|!date\^[^>]+)`;
const SLACK_ENTITY_CAPTURE = new RegExp(`(<${SLACK_ENTITY_INNER}>)`, 'g');
const SLACK_ENTITY_ONLY = new RegExp(`^<${SLACK_ENTITY_INNER}>$`);

const MARKDOWN_HEADING = /^\s{0,3}#{1,6}\s+(.+)$/gm;
const MARKDOWN_LINK = /\[([^\]]+)\]\((https?:\/\/[^)\s]+|mailto:[^)\s]+)\)/g;
const MARKDOWN_BOLD = /\*\*([^*]+)\*\*/g;
const MARKDOWN_BOLD_UNDERLINE = /__([^_]+)__/g;
const MARKDOWN_STRIKE = /~~([^~]+)~~/g;
const MARKDOWN_TABLE_SEPARATOR = /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/;

const escapeAmp = (value: string): string => (
  value.replace(/&(?!amp;|lt;|gt;|#\d+;|#x[0-9a-fA-F]+;)/g, '&amp;')
);

const escapePlain = (value: string): string => (
  escapeAmp(value)
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
);

const replaceWithRepair = (
  value: string,
  pattern: RegExp,
  replacement: string,
  repair: SlackMrkdwnRepair,
  repairs: Set<SlackMrkdwnRepair>
): string => {
  const next = value.replace(pattern, replacement);
  if (next !== value) repairs.add(repair);
  return next;
};

const normalizeEscapedNewlines = (value: string, repairs: Set<SlackMrkdwnRepair>): string => {
  const next = value
    .replace(/\\r\\n/g, '\n')
    .replace(/\\n/g, '\n')
    .replace(/\\t/g, '\t');
  if (next !== value) repairs.add('escapedNewlines');
  return next;
};

const stripCodeFenceLanguage = (value: string, repairs: Set<SlackMrkdwnRepair>): string => {
  const next = value.replace(/```[a-zA-Z0-9_-]+/g, '```');
  if (next !== value) repairs.add('codeFenceLanguage');
  return next;
};

const convertMarkdownTables = (value: string, repairs: Set<SlackMrkdwnRepair>): string => {
  const lines = value.split('\n');
  const out: string[] = [];
  let changed = false;
  let i = 0;
  // eslint-disable-next-line functional/no-loop-statements
  while (i < lines.length) {
    const line = lines[i] ?? '';
    const next = lines[i + 1];
    const isTableHeader = line.includes('|') && typeof next === 'string' && MARKDOWN_TABLE_SEPARATOR.test(next);
    if (isTableHeader && typeof next === 'string') {
      const tableLines: string[] = [line, next];
      i += 1;
      // eslint-disable-next-line functional/no-loop-statements
      while (i + 1 < lines.length && (lines[i + 1] ?? '').includes('|')) {
        tableLines.push(lines[i + 1] ?? '');
        i += 1;
      }
      out.push('```', ...tableLines, '```');
      changed = true;
      i += 1;
      continue;
    }
    out.push(line);
    i += 1;
  }
  if (changed) repairs.add('markdownTable');
  return out.join('\n');
};

const escapeSlackEntities = (value: string, repairs: Set<SlackMrkdwnRepair>): string => {
  const parts = value.split(SLACK_ENTITY_CAPTURE);
  const escaped = parts
    .map((part) => (SLACK_ENTITY_ONLY.test(part) ? part : escapePlain(part)))
    .join('');
  if (escaped !== value) repairs.add('escapeEntities');
  return escaped;
};

export const sanitizeSlackMrkdwn = (input: string): SlackMrkdwnResult => {
  const repairs = new Set<SlackMrkdwnRepair>();
  const base = stripCodeFenceLanguage(normalizeEscapedNewlines(input, repairs), repairs);
  const segments = base.split('```');
  const sanitizedSegments = segments.map((segment, index) => {
    if (index % 2 === 1) {
      return escapeSlackEntities(segment, repairs);
    }
    let output = segment;
    output = convertMarkdownTables(output, repairs);
    output = replaceWithRepair(output, MARKDOWN_HEADING, '*$1*', 'markdownHeading', repairs);
    output = replaceWithRepair(output, MARKDOWN_LINK, '<$2|$1>', 'markdownLink', repairs);
    output = replaceWithRepair(output, MARKDOWN_BOLD, '*$1*', 'markdownBold', repairs);
    output = replaceWithRepair(output, MARKDOWN_BOLD_UNDERLINE, '*$1*', 'markdownBold', repairs);
    output = replaceWithRepair(output, MARKDOWN_STRIKE, '~$1~', 'markdownStrike', repairs);
    return escapeSlackEntities(output, repairs);
  });
  return { text: sanitizedSegments.join('```'), repairs: Array.from(repairs) };
};

export const sanitizeSlackPlainText = (input: string): SlackMrkdwnResult => {
  const repairs = new Set<SlackMrkdwnRepair>();
  let output = normalizeEscapedNewlines(input, repairs);
  output = replaceWithRepair(output, MARKDOWN_LINK, '$1', 'stripMarkdownLinks', repairs);
  output = replaceWithRepair(output, MARKDOWN_HEADING, '$1', 'stripHeadingMarkers', repairs);
  const stripped = output.replace(/[`*_~]/g, '');
  if (stripped !== output) repairs.add('stripFormatting');
  output = escapePlain(stripped);
  return { text: output, repairs: Array.from(repairs) };
};

const asRecord = (value: unknown): value is Record<string, unknown> => (
  value !== null && typeof value === 'object' && !Array.isArray(value)
);

const asString = (value: unknown): string => {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
};

const clampText = (value: string, max: number, repairs: Set<SlackMrkdwnRepair>): string => {
  if (value.length <= max) return value;
  repairs.add('truncateText');
  return `${value.slice(0, Math.max(0, max - 3))}...`;
};

const parseMaybeJson = (value: unknown): unknown => {
  const raw = typeof value === 'string' ? value.trim() : undefined;
  if (raw === undefined || raw.length === 0) return value;
  if (!raw.startsWith('[') && !raw.startsWith('{')) return value;
  const parsed = parseJsonValueDetailed(raw, { preferArrayExtraction: true });
  return parsed.value ?? value;
};

const coerceMessagesArray = (
  value: unknown,
  repairs: Set<string>
): { messages?: unknown[]; error?: string } => {
  if (Array.isArray(value)) return { messages: value };
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (trimmed.length === 0) return { messages: undefined };
    const parsed = parseJsonValueDetailed(trimmed, { preferArrayExtraction: true });
    parsed.repairs.forEach((repair) => repairs.add(repair));
    const parsedValue = parsed.value;
    if (Array.isArray(parsedValue)) return { messages: parsedValue };
    if (parsedValue !== null && typeof parsedValue === 'object') {
      const msgs = (parsedValue as { messages?: unknown }).messages;
      if (Array.isArray(msgs)) {
        repairs.add('unwrapMessages');
        return { messages: msgs };
      }
    }
    return { messages: undefined, error: parsed.error };
  }
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const msgs = (value as { messages?: unknown }).messages;
    if (Array.isArray(msgs)) {
      repairs.add('unwrapMessages');
      return { messages: msgs };
    }
  }
  return { messages: undefined };
};

export const parseSlackBlockKitPayload = (input: {
  rawPayload?: string;
  messagesParam?: unknown;
  contentParam?: string | null;
}): SlackBlockKitParseResult => {
  const repairs = new Set<string>();
  let messages: unknown[] | undefined;
  let error: string | undefined;

  if (input.rawPayload !== undefined) {
    const parsed = parseJsonValueDetailed(input.rawPayload, { preferArrayExtraction: true });
    parsed.repairs.forEach((repair) => repairs.add(repair));
    const parsedValue = parsed.value;
    if (Array.isArray(parsedValue)) {
      messages = parsedValue;
    } else if (parsedValue !== null && typeof parsedValue === 'object') {
      const msgs = (parsedValue as { messages?: unknown }).messages;
      if (Array.isArray(msgs)) {
        repairs.add('unwrapMessages');
        messages = msgs;
      }
    } else if (parsed.error !== undefined) {
      error = parsed.error;
    }
  }

  if (messages === undefined && input.contentParam !== undefined && input.contentParam !== null) {
    const contentCandidate = input.contentParam.trim();
    if (contentCandidate.length > 0 && (contentCandidate.startsWith('[') || contentCandidate.startsWith('{'))) {
      const parsed = parseJsonValueDetailed(contentCandidate, { preferArrayExtraction: true });
      parsed.repairs.forEach((repair) => repairs.add(repair));
      const parsedValue = parsed.value;
      if (Array.isArray(parsedValue)) {
        messages = parsedValue;
      } else if (parsedValue !== null && typeof parsedValue === 'object') {
        const msgs = (parsedValue as { messages?: unknown }).messages;
        if (Array.isArray(msgs)) {
          repairs.add('unwrapMessages');
          messages = msgs;
        }
      } else if (parsed.error !== undefined) {
        error ??= parsed.error;
      }
    }
  }

  if (messages === undefined) {
    const coerced = coerceMessagesArray(input.messagesParam, repairs);
    messages = coerced.messages;
    error ??= coerced.error;
  }

  const fallbackContent = input.rawPayload ?? input.contentParam;
  const fallbackText = typeof fallbackContent === 'string' ? fallbackContent.trim() : '';
  const fallbackLooksJson = fallbackText.startsWith('[') || fallbackText.startsWith('{');
  const fallbackLooksXml = fallbackText.startsWith('<ai-agent-');
  const fallbackLooksInvalid = fallbackLooksJson || fallbackLooksXml;

  return {
    messages,
    repairs: Array.from(repairs),
    error,
    fallbackText,
    fallbackLooksInvalid,
  };
};

const normalizeMrkdwnValue = (value: unknown, max: number, repairs: Set<SlackMrkdwnRepair>): string => {
  const raw = asString(value);
  const { text, repairs: stepRepairs } = sanitizeSlackMrkdwn(raw);
  stepRepairs.forEach((repair) => repairs.add(repair));
  return clampText(text, max, repairs);
};

const normalizePlainTextValue = (value: unknown, max: number, repairs: Set<SlackMrkdwnRepair>): string => {
  const raw = asString(value);
  const { text, repairs: stepRepairs } = sanitizeSlackPlainText(raw);
  stepRepairs.forEach((repair) => repairs.add(repair));
  return clampText(text, max, repairs);
};

const normalizeContextElements = (value: unknown, repairs: Set<SlackMrkdwnRepair>): Record<string, unknown>[] => {
  const parsed = parseMaybeJson(value);
  const array: unknown[] = Array.isArray(parsed) ? parsed : [];
  const elements = array
    .map((entry): SlackMrkdwnText | undefined => {
      const textValue = asRecord(entry) ? entry.text : entry;
      const text = normalizeMrkdwnValue(textValue, SLACK_LIMITS.contextText, repairs);
      return text.length > 0 ? { type: 'mrkdwn', text } : undefined;
    })
    .filter((entry): entry is SlackMrkdwnText => entry !== undefined)
    .slice(0, SLACK_LIMITS.maxContext);
  if (array.length > SLACK_LIMITS.maxContext) repairs.add('truncateContext');
  return elements;
};

const normalizeFields = (value: unknown, repairs: Set<SlackMrkdwnRepair>): Record<string, unknown>[] => {
  const parsed = parseMaybeJson(value);
  const array: unknown[] = Array.isArray(parsed) ? parsed : [];
  const fields = array
    .map((entry): SlackMrkdwnText | undefined => {
      const textValue = asRecord(entry) ? entry.text : entry;
      const text = normalizeMrkdwnValue(textValue, SLACK_LIMITS.fieldText, repairs);
      return text.length > 0 ? { type: 'mrkdwn', text } : undefined;
    })
    .filter((entry): entry is SlackMrkdwnText => entry !== undefined)
    .slice(0, SLACK_LIMITS.maxFields);
  if (array.length > SLACK_LIMITS.maxFields) repairs.add('truncateFields');
  return fields;
};

const normalizeBlock = (value: unknown, repairs: Set<SlackMrkdwnRepair>): Record<string, unknown> | undefined => {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    const text = normalizeMrkdwnValue(value, SLACK_LIMITS.sectionText, repairs);
    return text.length > 0 ? { type: 'section', text: { type: 'mrkdwn', text } } : undefined;
  }
  if (!asRecord(value)) {
    repairs.add('dropInvalidBlock');
    return undefined;
  }
  const typeValue = typeof value.type === 'string' ? value.type : '';
  if (typeValue === 'divider') return { type: 'divider' };
  if (typeValue === 'header') {
    const headerText = normalizePlainTextValue(asRecord(value.text) ? value.text.text : value.text, SLACK_LIMITS.headerText, repairs);
    if (headerText.length === 0) return undefined;
    return { type: 'header', text: { type: 'plain_text', text: headerText } };
  }
  if (typeValue === 'context') {
    const elements = normalizeContextElements(value.elements, repairs);
    if (elements.length === 0) return undefined;
    return { type: 'context', elements };
  }
  if (typeValue === 'section' || typeValue === '') {
    const textValue = asRecord(value.text) ? value.text.text : value.text;
    const text = normalizeMrkdwnValue(textValue, SLACK_LIMITS.sectionText, repairs);
    const fields = normalizeFields(value.fields, repairs);
    if (text.length === 0 && fields.length === 0) return undefined;
    const out: Record<string, unknown> = { type: 'section' };
    if (text.length > 0) out.text = { type: 'mrkdwn', text };
    if (fields.length > 0) out.fields = fields;
    return out;
  }
  const fallbackJson = (() => {
    try {
      return JSON.stringify(value);
    } catch {
      return asString(value);
    }
  })();
  const fallbackText = normalizeMrkdwnValue(fallbackJson, SLACK_LIMITS.sectionText, repairs);
  return fallbackText.length > 0 ? { type: 'section', text: { type: 'mrkdwn', text: fallbackText } } : undefined;
};

const normalizeBlocks = (value: unknown, repairs: Set<SlackMrkdwnRepair>): Record<string, unknown>[] => {
  const parsed = parseMaybeJson(value);
  const array = Array.isArray(parsed) ? parsed : (parsed !== undefined ? [parsed] : []);
  const blocks = array
    .map((entry) => normalizeBlock(entry, repairs))
    .filter((entry): entry is Record<string, unknown> => entry !== undefined)
    .slice(0, SLACK_LIMITS.maxBlocks);
  if (array.length > SLACK_LIMITS.maxBlocks) repairs.add('truncateBlocks');
  return blocks;
};

const normalizeMessage = (value: unknown, repairs: Set<SlackMrkdwnRepair>): Record<string, unknown> | undefined => {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    const blocks = normalizeBlocks(value, repairs);
    return blocks.length > 0 ? { blocks } : undefined;
  }
  if (!asRecord(value)) {
    repairs.add('dropInvalidMessage');
    return undefined;
  }
  const blocks = value.blocks !== undefined ? normalizeBlocks(value.blocks, repairs) : normalizeBlocks(value, repairs);
  if (blocks.length === 0) {
    repairs.add('dropInvalidMessage');
    return undefined;
  }
  return { blocks };
};

export const normalizeSlackMessages = (
  inputMessages: unknown[],
  options?: { fallbackText?: string }
): SlackMessageNormalizationResult => {
  const repairs = new Set<SlackMrkdwnRepair>();
  const normalized = inputMessages
    .map((entry) => normalizeMessage(entry, repairs))
    .filter((entry): entry is Record<string, unknown> => entry !== undefined)
    .slice(0, SLACK_LIMITS.maxMessages);
  if (inputMessages.length > SLACK_LIMITS.maxMessages) repairs.add('truncateMessages');

  if (normalized.length === 0) {
    const fallback = options?.fallbackText ?? '';
    if (fallback.trim().length === 0) {
      return { messages: [], repairs: Array.from(repairs) };
    }
    const fallbackBlock = normalizeBlock(fallback, repairs);
    if (fallbackBlock === undefined) {
      return { messages: [], repairs: Array.from(repairs) };
    }
    return { messages: [{ blocks: [fallbackBlock] }], repairs: Array.from(repairs) };
  }
  return { messages: normalized, repairs: Array.from(repairs) };
};
