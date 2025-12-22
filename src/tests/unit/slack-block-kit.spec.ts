import { readFileSync } from 'node:fs';

import Ajv, { type Ajv as AjvClass, type Options as AjvOptions } from 'ajv';
import { describe, expect, it } from 'vitest';

import { normalizeSlackMessages, parseSlackBlockKitPayload, sanitizeSlackMrkdwn, SLACK_BLOCK_KIT_SCHEMA } from '../../slack-block-kit.js';

interface SlackTestBlock {
  text?: { text?: unknown };
  fields?: { text?: unknown }[];
}

describe('sanitizeSlackMrkdwn', () => {
  it('converts common markdown to Slack mrkdwn', () => {
    const input = '## Title\nSee [Docs](https://example.com) and **Bold** plus ***BoldItalic*** and ~~Strike~~ & <tag>\n|A|B|\n|---|---|\n|1|2|\n```json\n{"a":1}\n```';
    const result = sanitizeSlackMrkdwn(input);
    expect(result.text).toContain('*Title*');
    expect(result.text).toContain('<https://example.com|Docs>');
    expect(result.text).toContain('*Bold*');
    expect(result.text).toContain('*BoldItalic*');
    expect(result.text).toContain('~Strike~');
    expect(result.text).toContain('&amp;');
    expect(result.text).toContain('&lt;tag&gt;');
    expect(result.text).toContain('```');
    expect(result.text).not.toContain('```json');
    expect(result.text).not.toContain('## Title');
    expect(result.text).not.toContain('[Docs](');
  });
});

describe('normalizeSlackMessages', () => {
  it('preserves fields and sanitizes text', () => {
    const result = normalizeSlackMessages([
      {
        blocks: [
          {
            type: 'section',
            text: { type: 'mrkdwn', text: '## Heading' },
            fields: [{ type: 'mrkdwn', text: '**Key**' }],
          },
        ],
      },
      {
        blocks: [
          { type: 'header', text: { type: 'plain_text', text: '**Title** & Partners' } },
        ],
      },
    ]);

    const firstMessage = result.messages[0] as { blocks?: SlackTestBlock[] };
    const firstBlock = Array.isArray(firstMessage.blocks) ? firstMessage.blocks[0] : undefined;
    const firstText = typeof firstBlock?.text?.text === 'string' ? firstBlock.text.text : '';
    const firstFieldText = typeof firstBlock?.fields?.[0]?.text === 'string' ? firstBlock.fields[0].text : '';

    expect(firstText).toContain('*Heading*');
    expect(firstFieldText).toContain('*Key*');

    const headerMessage = result.messages[1] as { blocks?: SlackTestBlock[] };
    const headerBlock = Array.isArray(headerMessage.blocks) ? headerMessage.blocks[0] : undefined;
    const headerText = typeof headerBlock?.text?.text === 'string' ? headerBlock.text.text : '';
    expect(headerText).toBe('Title & Partners');
  });

  it('creates fallback message when input is empty', () => {
    const result = normalizeSlackMessages([], { fallbackText: '## Fallback' });
    expect(result.messages.length).toBe(1);
    const firstMessage = result.messages[0] as { blocks?: SlackTestBlock[] };
    const firstBlock = Array.isArray(firstMessage.blocks) ? firstMessage.blocks[0] : undefined;
    const text = typeof firstBlock?.text?.text === 'string' ? firstBlock.text.text : '';
    expect(text).toContain('*Fallback*');
  });

  it('inserts a placeholder when a message is dropped', () => {
    const result = normalizeSlackMessages([null]);
    expect(result.messages.length).toBe(1);
    const firstMessage = result.messages[0] as { blocks?: SlackTestBlock[] };
    const firstBlock = Array.isArray(firstMessage.blocks) ? firstMessage.blocks[0] : undefined;
    const text = typeof firstBlock?.text?.text === 'string' ? firstBlock.text.text : '';
    expect(text).toBe('[invalid block dropped]');
  });

  it('keeps the full message set for the 2ea853ad payload', () => {
    const rawPayload = readFileSync(new URL('../fixtures/slack-block-kit-2ea853ad.raw.txt', import.meta.url), 'utf-8');
    const parsed = parseSlackBlockKitPayload({ rawPayload });
    const normalized = normalizeSlackMessages(parsed.messages ?? [], { fallbackText: parsed.fallbackText });
    const flattened = JSON.stringify(normalized.messages);
    expect(flattened).toContain('Slack Block Kit Demo');
    expect(flattened).toContain('Welcome to the Slack Block Kit Formatting Demonstration');
  });
});

describe('parseSlackBlockKitPayload', () => {
  it('unwraps messages object from raw payload', () => {
    const rawPayload = '{"messages":[{"blocks":[{"type":"section","text":{"type":"mrkdwn","text":"Hello"}}]}]}';
    const result = parseSlackBlockKitPayload({ rawPayload });
    expect(Array.isArray(result.messages)).toBe(true);
    expect(result.messages?.length).toBe(1);
    expect(result.repairs).toContain('unwrapMessages');
  });

  it('parses JSON array from contentParam when raw payload is missing', () => {
    const contentParam = '[{"blocks":[{"type":"section","text":{"type":"mrkdwn","text":"Hi"}}]}]';
    const result = parseSlackBlockKitPayload({ contentParam });
    expect(Array.isArray(result.messages)).toBe(true);
    expect(result.messages?.length).toBe(1);
  });
});

describe('SLACK_BLOCK_KIT_SCHEMA', () => {
  it('accepts section blocks with both text and fields', () => {
    type AjvConstructor = new (options?: AjvOptions) => AjvClass;
    const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;
    const ajv = new AjvCtor({ allErrors: true });
    const validate = ajv.compile(SLACK_BLOCK_KIT_SCHEMA);
    const payload = [
      {
        blocks: [
          {
            type: 'section',
            text: { type: 'mrkdwn', text: 'Hello' },
            fields: [{ type: 'mrkdwn', text: 'Value' }],
          },
        ],
      },
    ];

    expect(validate(payload)).toBe(true);
  });
});
