import { describe, expect, it } from 'vitest';

import type { ResolvedFinalReportPluginRequirement } from '../../plugins/types.js';

import { loadFinalReportInstructions } from '../../prompts/loader.js';
import { SLACK_BLOCK_KIT_MRKDWN_RULES } from '../../slack-block-kit.js';

const NONCE = 'cafebabe';
const FORMAT_ID = 'markdown';
const FORMAT_DESCRIPTION = 'Markdown output';
const SCHEMA_BLOCK = '';
const FINAL_SLOT = `${NONCE}-FINAL`;
const SLACK_FORMAT_ID = 'slack-block-kit';
const JSON_FORMAT_ID = 'json';
const JSON_SCHEMA_BLOCK = 'SCHEMA-BLOCK';
const JSON_EXAMPLE = '{ ... your JSON here ... }';
const EMPTY_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [];
const META_REQUIRED_PHRASE = 'META REQUIRED WITH FINAL.';
const META_NONE_PHRASE = 'META: none required for this session.';
const SAMPLE_META_WRAPPER = '<ai-agent-cafebabe-META plugin="support-metadata">{...}</ai-agent-cafebabe-META>';

const SAMPLE_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [
  {
    name: 'support-metadata',
    schema: {
      type: 'object',
      additionalProperties: false,
      required: ['ticketId'],
      properties: {
        ticketId: { type: 'string', minLength: 1 },
      },
    },
    systemPromptInstructions: 'Provide support metadata JSON with ticketId.',
    xmlNextSnippet: 'Support META must include ticketId.',
    finalReportExampleSnippet: '<ai-agent-cafebabe-META plugin="support-metadata">{"ticketId":"123"}</ai-agent-cafebabe-META>',
  },
];

describe('prompts loader', () => {
  it('requires a session nonce', () => {
    expect(() =>
      loadFinalReportInstructions(FORMAT_ID, FORMAT_DESCRIPTION, SCHEMA_BLOCK, undefined, EMPTY_REQUIREMENTS)
    ).toThrowError();
  });

  it('renders the final wrapper with the provided nonce and format', () => {
    const rendered = loadFinalReportInstructions(
      FORMAT_ID,
      FORMAT_DESCRIPTION,
      SCHEMA_BLOCK,
      NONCE,
      EMPTY_REQUIREMENTS,
    );

    expect(rendered).toContain(FINAL_SLOT);
    expect(rendered).toContain(`format="${FORMAT_ID}"`);
    expect(rendered).toContain(META_NONE_PHRASE);
    expect(rendered).not.toContain('{{{slotId}}}');
    expect(rendered).not.toContain('{{{formatId}}}');
  });

  it('injects Slack mrkdwn guidance for slack-block-kit format', () => {
    const rendered = loadFinalReportInstructions(
      SLACK_FORMAT_ID,
      'Slack Block Kit output',
      SCHEMA_BLOCK,
      NONCE,
      EMPTY_REQUIREMENTS,
    );

    expect(rendered).toContain(SLACK_BLOCK_KIT_MRKDWN_RULES);
  });

  it('inserts the provided schema block for json format', () => {
    const rendered = loadFinalReportInstructions(
      JSON_FORMAT_ID,
      'JSON output',
      JSON_SCHEMA_BLOCK,
      NONCE,
      EMPTY_REQUIREMENTS,
    );

    expect(rendered).toContain(JSON_SCHEMA_BLOCK);
    expect(rendered).toContain(JSON_EXAMPLE);
  });

  it('renders META requirements when plugins are configured', () => {
    const rendered = loadFinalReportInstructions(
      FORMAT_ID,
      FORMAT_DESCRIPTION,
      SCHEMA_BLOCK,
      NONCE,
      SAMPLE_REQUIREMENTS,
    );

    expect(rendered).toContain(META_REQUIRED_PHRASE);
    expect(rendered).toContain(SAMPLE_META_WRAPPER);
    expect(rendered).toContain(SAMPLE_REQUIREMENTS[0].systemPromptInstructions);
    expect(rendered).toContain(SAMPLE_REQUIREMENTS[0].finalReportExampleSnippet);
  });
});
