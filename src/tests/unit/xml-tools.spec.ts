import { describe, expect, it } from 'vitest';

import { createXmlParser, renderXmlNext } from '../../xml-tools.js';

const NONCE = 'abcd1234';
const SLOT_ONE = `${NONCE}-0001`;
const SLOT_TWO = `${NONCE}-0002`;
const ALLOWED_SLOTS = new Set([SLOT_ONE]);
const ALLOWED_TOOLS = new Set(['echo']);

describe('XML streaming parser', () => {
  it('parses a single complete tag', () => {
    const parser = createXmlParser();
    const res = parser.parseChunk(`<ai-agent-${SLOT_ONE} tool="echo">hello</ai-agent-${SLOT_ONE}>`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(1);
    expect(res[0]).toEqual({ slotId: 'abcd1234-0001', tool: 'echo', rawPayload: 'hello' });
  });

  it('ignores wrong nonce', () => {
    const parser = createXmlParser();
    const res = parser.parseChunk('<ai-agent-zzzz9999-0001 tool="echo">hello</ai-agent-zzzz9999-0001>', NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(0);
  });

  it('ignores disallowed slot', () => {
    const parser = createXmlParser();
    const res = parser.parseChunk('<ai-agent-abcd1234-0002 tool="echo">hello</ai-agent-abcd1234-0002>', NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(0);
  });

  it('ignores disallowed tool', () => {
    const parser = createXmlParser();
    const res = parser.parseChunk(`<ai-agent-${SLOT_ONE} tool="foo">hello</ai-agent-${SLOT_ONE}>`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(0);
  });

  it('handles chunked tag pieces', () => {
    const parser = createXmlParser();
    parser.parseChunk('<ai-agent-abcd1234-', NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    parser.parseChunk('0001 tool="echo">he', NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    const res = parser.parseChunk(`llo</ai-agent-${SLOT_ONE}>`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(1);
    expect(res[0].rawPayload).toBe('hello');
  });

  it('leaves incomplete tag until more arrives', () => {
    const parser = createXmlParser();
    const res1 = parser.parseChunk(`<ai-agent-${SLOT_ONE} tool="echo">hello`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res1).toHaveLength(0);
    const res2 = parser.parseChunk(`</ai-agent-${SLOT_ONE}>`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res2).toHaveLength(1);
  });

  it('ignores empty content', () => {
    const parser = createXmlParser();
    const res = parser.parseChunk(`<ai-agent-${SLOT_ONE} tool="echo">   </ai-agent-${SLOT_ONE}>`, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(0);
  });

  it('supports multiple tags in one chunk', () => {
    const parser = createXmlParser();
    const allowedSlots = new Set([SLOT_ONE, SLOT_TWO]);
    const res = parser.parseChunk(`<ai-agent-${SLOT_ONE} tool="echo">hi</ai-agent-${SLOT_ONE}><ai-agent-${SLOT_TWO} tool="echo">bye</ai-agent-${SLOT_TWO}>`, NONCE, allowedSlots, ALLOWED_TOOLS);
    expect(res.map((r) => r.rawPayload)).toEqual(['hi', 'bye']);
  });

  it('renders xml-final with only final-report and progress slot', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 1,
      maxTurns: 3,
      tools: [{ name: 'agent__final_report' }, { name: 'agent__progress_report' }, { name: 'mock_tool', schema: { type: 'object' } }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['agent__final_report'] }],
      progressSlot: undefined,
      finalReportSlot: { slotId: `${NONCE}-FINAL` },
      mode: 'xml-final',
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
    });
    expect(xml).toContain('Mode: final-report via XML; tools via native tool calls');
    expect(xml).toContain(`<ai-agent-${NONCE}-FINAL tool="agent__final_report"`);
    expect(xml).toContain('Tools are available via native tool calls');
  });
});
