import { describe, expect, it } from 'vitest';

import { parseJsonRecordDetailed, parseJsonValueDetailed } from '../utils.js';

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
});
