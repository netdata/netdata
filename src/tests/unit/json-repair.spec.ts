import { describe, expect, it } from 'vitest';

import { parseJsonRecordDetailed, parseJsonValueDetailed } from '../../utils.js';

describe('json repair pipeline', () => {
  it('repairs trailing comma via jsonrepair', () => {
    const input = '{"a":1,}';
    const result = parseJsonRecordDetailed(input);
    expect(result.value).toEqual({ a: 1 });
    expect(result.repairs).toContain('jsonrepair');
  });

  it('returns valid JSON untouched', () => {
    const result = parseJsonRecordDetailed('{"data":{"value":"ok"}}');
    expect(result.value).toEqual({ data: { value: 'ok' } });
    expect(result.repairs).toHaveLength(0);
  });

  it('repairs common JSON lint issues via jsonrepair', () => {
    const input = '{"data":{"value":"ok",},"extra":true}';
    const result = parseJsonRecordDetailed(input);
    expect(result.value).toEqual({ data: { value: 'ok' }, extra: true });
    expect(result.repairs).toContain('jsonrepair');
  });
});

describe('parseJsonValueDetailed', () => {
  it('returns original object without repairs', () => {
    const obj = { x: 1 };
    const result = parseJsonValueDetailed(obj);
    expect(result.value).toEqual(obj);
    expect(result.repairs).toEqual([]);
  });

  it('repairs backslash-newline (line continuation) in JSON strings', () => {
    // Model sometimes outputs backslash followed by literal newline (invalid JSON)
    // This should be converted to escaped newline \n
    const input = '{"text": "hello\\\nthere"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ text: 'hello\nthere' });
    expect(result.repairs).toContain('backslashNewlineFix');
  });

  it('repairs multiple backslash-newline occurrences', () => {
    const input = '{"a": "line1\\\nline2\\\nline3"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ a: 'line1\nline2\nline3' });
    expect(result.repairs).toContain('backslashNewlineFix');
  });

  it('does not modify JSON without backslash-newline', () => {
    const input = '{"text": "hello\\nthere"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ text: 'hello\nthere' });
    expect(result.repairs).not.toContain('backslashNewlineFix');
  });
});

describe('array extraction', () => {
  it('extracts array from markdown code fence', () => {
    const input = '```json\n[{"blocks": [{"type": "section"}]}]\n```';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual([{ blocks: [{ type: 'section' }] }]);
    // jsonrepair handles code fence extraction internally
    expect(result.repairs.length).toBeGreaterThan(0);
  });

  it('extracts array from text with leading non-JSON characters', () => {
    // Use binary characters that jsonrepair can't interpret as JSON
    const input = '\x00\x01\x02[{"blocks": [{"type": "section"}]}]';
    const result = parseJsonValueDetailed(input);
    // Should extract the array, not the inner object
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts array from XML wrapper', () => {
    const input = '<ai-agent-final format="slack-block-kit">\n[{"blocks": [{"type": "section"}]}]\n</ai-agent-final>';
    const result = parseJsonValueDetailed(input);
    // Should extract the array, not the inner object
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts nested array from malformed wrapper', () => {
    const input = 'Response: [{"items": [1, 2, 3]}, {"items": [4, 5]}] end';
    const result = parseJsonValueDetailed(input);
    // Should extract the full array with both elements
    expect(Array.isArray(result.value)).toBe(true);
    const arr = result.value as unknown[];
    expect(arr.length).toBe(2);
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts slack-block-kit array with nested blocks', () => {
    const input = '```json\n[\n  {\n    "blocks": [\n      {\n        "type": "header",\n        "text": {"type": "plain_text", "text": "Report"}\n      }\n    ]\n  }\n]\n```';
    const result = parseJsonValueDetailed(input);
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
  });

  it('returns valid array untouched', () => {
    const input = '[{"a": 1}, {"b": 2}]';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual([{ a: 1 }, { b: 2 }]);
    expect(result.repairs).toHaveLength(0);
  });

  it('extracts array when both array and object are present', () => {
    // Array extraction runs first in the queue, so arrays are preferred
    const input = 'Prefix [1, 2, 3] then {"key": "value"} suffix';
    const result = parseJsonValueDetailed(input);
    expect(Array.isArray(result.value)).toBe(true);
    expect(result.value).toEqual([1, 2, 3]);
  });
});
