import { describe, expect, it } from 'vitest';

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

describe('prompts loader', () => {
  it('requires a session nonce', () => {
    expect(() =>
      loadFinalReportInstructions(FORMAT_ID, FORMAT_DESCRIPTION, SCHEMA_BLOCK, undefined)
    ).toThrowError();
  });

  it('renders the final wrapper with the provided nonce and format', () => {
    const rendered = loadFinalReportInstructions(
      FORMAT_ID,
      FORMAT_DESCRIPTION,
      SCHEMA_BLOCK,
      NONCE
    );

    expect(rendered).toContain(FINAL_SLOT);
    expect(rendered).toContain(`format="${FORMAT_ID}"`);
    expect(rendered).not.toContain('{{{slotId}}}');
    expect(rendered).not.toContain('{{{formatId}}}');
  });

  it('injects Slack mrkdwn guidance for slack-block-kit format', () => {
    const rendered = loadFinalReportInstructions(
      SLACK_FORMAT_ID,
      'Slack Block Kit output',
      SCHEMA_BLOCK,
      NONCE
    );

    expect(rendered).toContain(SLACK_BLOCK_KIT_MRKDWN_RULES);
  });

  it('inserts the provided schema block for json format', () => {
    const rendered = loadFinalReportInstructions(
      JSON_FORMAT_ID,
      'JSON output',
      JSON_SCHEMA_BLOCK,
      NONCE
    );

    expect(rendered).toContain(JSON_SCHEMA_BLOCK);
    expect(rendered).toContain(JSON_EXAMPLE);
  });
});
