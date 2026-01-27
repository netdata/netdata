import { describe, expect, it } from 'vitest';

import type { ResolvedFinalReportPluginRequirement } from '../../plugins/types.js';

import { renderFinalReportInstructions } from '../../prompts/loader.js';
import { SLACK_BLOCK_KIT_MRKDWN_RULES, SLACK_BLOCK_KIT_SCHEMA } from '../../slack-block-kit.js';

const NONCE = 'cafebabe';
const FORMAT_ID = 'markdown';
const FORMAT_DESCRIPTION = 'Markdown output';
const EMPTY_SCHEMA: Record<string, unknown> = {};
const FINAL_SLOT = `${NONCE}-FINAL`;
const SLACK_FORMAT_ID = 'slack-block-kit';
const JSON_FORMAT_ID = 'json';
const JSON_SCHEMA_BLOCK = { type: 'object', additionalProperties: false, properties: { ticketId: { type: 'string' } } };
const JSON_EXAMPLE = '{ ... your JSON here ... }';
const EMPTY_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [];
const META_REQUIRED_PHRASE = '### META Requirements (Mandatory With FINAL)';
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
      renderFinalReportInstructions({
        formatId: FORMAT_ID,
        formatDescription: FORMAT_DESCRIPTION,
        expectedJsonSchema: EMPTY_SCHEMA,
        nonce: undefined as unknown as string,
        pluginRequirements: EMPTY_REQUIREMENTS,
      })
    ).toThrowError();
  });

  it('renders the final wrapper with the provided nonce and format', () => {
    const rendered = renderFinalReportInstructions({
      formatId: FORMAT_ID,
      formatDescription: FORMAT_DESCRIPTION,
      expectedJsonSchema: EMPTY_SCHEMA,
      nonce: NONCE,
      pluginRequirements: EMPTY_REQUIREMENTS,
    });

    expect(rendered).toContain(FINAL_SLOT);
    expect(rendered).toContain(`format="${FORMAT_ID}"`);
    // When no plugins configured, META should not be mentioned at all
    expect(rendered).not.toContain('META');
    expect(rendered).not.toContain('{{{slotId}}}');
    expect(rendered).not.toContain('{{{formatId}}}');
  });

  it('injects Slack mrkdwn guidance for slack-block-kit format', () => {
    const rendered = renderFinalReportInstructions({
      formatId: SLACK_FORMAT_ID,
      formatDescription: 'Slack Block Kit output',
      expectedJsonSchema: EMPTY_SCHEMA,
      slackSchema: SLACK_BLOCK_KIT_SCHEMA,
      nonce: NONCE,
      pluginRequirements: EMPTY_REQUIREMENTS,
    });

    expect(rendered).toContain(SLACK_BLOCK_KIT_MRKDWN_RULES);
  });

  it('inserts the provided schema block for json format', () => {
    const rendered = renderFinalReportInstructions({
      formatId: JSON_FORMAT_ID,
      formatDescription: 'JSON output',
      expectedJsonSchema: JSON_SCHEMA_BLOCK,
      nonce: NONCE,
      pluginRequirements: EMPTY_REQUIREMENTS,
    });

    expect(rendered).toContain('"ticketId"');
    expect(rendered).toContain(JSON_EXAMPLE);
  });

  it('renders META requirements when plugins are configured', () => {
    const rendered = renderFinalReportInstructions({
      formatId: FORMAT_ID,
      formatDescription: FORMAT_DESCRIPTION,
      expectedJsonSchema: EMPTY_SCHEMA,
      nonce: NONCE,
      pluginRequirements: SAMPLE_REQUIREMENTS,
    });

    expect(rendered).toContain(META_REQUIRED_PHRASE);
    expect(rendered).toContain(SAMPLE_META_WRAPPER);
    expect(rendered).toContain(SAMPLE_REQUIREMENTS[0].systemPromptInstructions);
    expect(rendered).toContain(SAMPLE_REQUIREMENTS[0].finalReportExampleSnippet);
  });
});
