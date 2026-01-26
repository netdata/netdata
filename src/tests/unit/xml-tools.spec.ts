import { describe, expect, it } from 'vitest';

import type { ResolvedFinalReportPluginRequirement } from '../../plugins/types.js';

import { createXmlParser, renderXmlNext } from '../../xml-tools.js';

const NONCE = 'abcd1234';
const SLOT_ONE = `${NONCE}-0001`;
const SLOT_TWO = `${NONCE}-0002`;
const ALLOWED_SLOTS = new Set([SLOT_ONE]);
const ALLOWED_TOOLS = new Set(['echo']);
const ROUTER_HANDOFF_TOOL = 'router__handoff-to';
const META_REQUIRED_PHRASE = 'META REQUIRED WITH FINAL.';
const META_NONE_PHRASE = 'META: none required for this session.';
const EMPTY_PLUGIN_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [];
const SAMPLE_PLUGIN_NAME = 'support-metadata';
const SAMPLE_XML_NEXT_SNIPPET = 'Support META must include ticketId.';
const SAMPLE_META_WRAPPER = `<ai-agent-${NONCE}-META plugin="${SAMPLE_PLUGIN_NAME}">{...}</ai-agent-${NONCE}-META>`;
const SAMPLE_REQUIREMENTS: ResolvedFinalReportPluginRequirement[] = [
  {
    name: SAMPLE_PLUGIN_NAME,
    schema: { type: 'object', additionalProperties: false },
    systemPromptInstructions: 'Provide support metadata JSON.',
    xmlNextSnippet: SAMPLE_XML_NEXT_SNIPPET,
    finalReportExampleSnippet: `<ai-agent-${NONCE}-META plugin="${SAMPLE_PLUGIN_NAME}">{"ticketId":"123"}</ai-agent-${NONCE}-META>`,
  },
];

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

  it('renders xml-final with FINAL slot and format info', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 1,
      maxTurns: 3,
      tools: [{ name: 'final_xml_only' }, { name: 'agent__task_status' }, { name: 'mock_tool', schema: { type: 'object' } }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['final_xml_only'] }],
      taskStatusToolEnabled: false,
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
      finalReportPluginRequirements: EMPTY_PLUGIN_REQUIREMENTS,
      finalReportLocked: false,
      missingMetaPluginNames: [],
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 12,
      hasExternalTools: true,
    });
    // xml-final: XML-NEXT describes the XML wrapper with nonce-FINAL
    expect(xml).toContain(`${NONCE}-FINAL`);
    expect(xml).toContain('markdown');
    expect(xml).toContain(META_NONE_PHRASE);
    expect(xml).not.toContain('mock_tool');
  });

  it('renders META requirements section when plugins are configured', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 1,
      maxTurns: 3,
      tools: [{ name: 'final_xml_only' }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['final_xml_only'] }],
      taskStatusToolEnabled: false,
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
      finalReportPluginRequirements: SAMPLE_REQUIREMENTS,
      finalReportLocked: false,
      missingMetaPluginNames: [],
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 12,
      hasExternalTools: true,
    });

    expect(xml).toContain('## META Requirements');
    expect(xml).toContain(META_REQUIRED_PHRASE);
    expect(xml).toContain(SAMPLE_META_WRAPPER);
    expect(xml).toContain(SAMPLE_XML_NEXT_SNIPPET);
  });

  it('switches to META-only guidance when the final report is locked', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 2,
      maxTurns: 5,
      tools: [{ name: 'final_xml_only' }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['final_xml_only'] }],
      taskStatusToolEnabled: false,
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
      finalReportPluginRequirements: SAMPLE_REQUIREMENTS,
      finalReportLocked: true,
      missingMetaPluginNames: [SAMPLE_PLUGIN_NAME],
      attempt: 2,
      maxRetries: 3,
      contextPercentUsed: 40,
      hasExternalTools: true,
    });

    expect(xml).toContain('FINAL already accepted. Do NOT resend the FINAL wrapper.');
    expect(xml).toContain(`Missing META plugins: ${SAMPLE_PLUGIN_NAME}.`);
    expect(xml).toContain('## META Requirements — FINAL Already Accepted');
    expect(xml).toContain(SAMPLE_META_WRAPPER);
    expect(xml).not.toContain(META_REQUIRED_PHRASE);
    expect(xml).not.toContain(`<ai-agent-${NONCE}-FINAL`);
  });

  it('mentions router handoff option on final turn when allowed', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 3,
      maxTurns: 3,
      tools: [{ name: 'agent__final_report' }, { name: ROUTER_HANDOFF_TOOL }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['agent__final_report'] }],
      taskStatusToolEnabled: false,
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
      finalReportPluginRequirements: EMPTY_PLUGIN_REQUIREMENTS,
      finalReportLocked: false,
      missingMetaPluginNames: [],
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 90,
      hasExternalTools: true,
      forcedFinalTurnReason: 'max_turns',
      finalTurnTools: [ROUTER_HANDOFF_TOOL],
    });
    expect(xml).toContain(ROUTER_HANDOFF_TOOL);
    expect(xml).toContain(`OR call \`${ROUTER_HANDOFF_TOOL}\``);
  });

  it('does not mention router handoff when the final report is locked', () => {
    const xml = renderXmlNext({
      nonce: NONCE,
      turn: 3,
      maxTurns: 3,
      tools: [{ name: 'agent__final_report' }, { name: ROUTER_HANDOFF_TOOL }],
      slotTemplates: [{ slotId: `${NONCE}-FINAL`, tools: ['agent__final_report'] }],
      taskStatusToolEnabled: false,
      expectedFinalFormat: 'markdown',
      finalSchema: undefined,
      finalReportPluginRequirements: SAMPLE_REQUIREMENTS,
      finalReportLocked: true,
      missingMetaPluginNames: [SAMPLE_PLUGIN_NAME],
      attempt: 1,
      maxRetries: 3,
      contextPercentUsed: 90,
      hasExternalTools: true,
      forcedFinalTurnReason: 'max_turns',
      finalTurnTools: [ROUTER_HANDOFF_TOOL],
    });

    expect(xml).not.toContain(ROUTER_HANDOFF_TOOL);
  });

  it('skips <think> block and parses actual XML tag after it', () => {
    const parser = createXmlParser();
    // Simulate reasoning model output: <think> block mentions the tag as example, real tag follows
    const content = `<think>I'll use the <ai-agent-${SLOT_ONE} tool="echo"> wrapper.</think><ai-agent-${SLOT_ONE} tool="echo">real content</ai-agent-${SLOT_ONE}>`;
    const res = parser.parseChunk(content, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(1);
    expect(res[0].rawPayload).toBe('real content');
  });

  it('skips <think> block with leading whitespace', () => {
    const parser = createXmlParser();
    const content = `  \n\t<think>Example <ai-agent-${SLOT_ONE} tool="echo"> wrapper.</think><ai-agent-${SLOT_ONE} tool="echo">real content</ai-agent-${SLOT_ONE}>`;
    const res = parser.parseChunk(content, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(1);
    expect(res[0].rawPayload).toBe('real content');
  });

  it('handles content not starting with <think>', () => {
    const parser = createXmlParser();
    // Normal content without <think>
    const content = `Some preamble text. <ai-agent-${SLOT_ONE} tool="echo">hello</ai-agent-${SLOT_ONE}>`;
    const res = parser.parseChunk(content, NONCE, ALLOWED_SLOTS, ALLOWED_TOOLS);
    expect(res).toHaveLength(1);
    expect(res[0].rawPayload).toBe('hello');
  });
});
