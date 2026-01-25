import { describe, expect, it } from 'vitest';

import { loadFinalReportInstructions } from '../../prompts/loader.js';

const NONCE = 'cafebabe';
const FORMAT_ID = 'markdown';
const FORMAT_DESCRIPTION = 'Markdown output';
const SCHEMA_BLOCK = '';
const FINAL_SLOT = `${NONCE}-FINAL`;

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
});

