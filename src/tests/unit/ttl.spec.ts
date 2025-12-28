import { describe, expect, it } from 'vitest';

import { parseCacheDurationMs, parseCacheDurationMsStrict, parseDurationMs, parseDurationMsStrict } from '../../cache/ttl.js';

describe('ttl duration parsing', () => {
  it('parses numeric milliseconds', () => {
    expect(parseDurationMs(1500)).toBe(1500);
    expect(parseDurationMs('1500')).toBe(1500);
  });

  it('parses duration units', () => {
    expect(parseDurationMs('1.5s')).toBe(1500);
    expect(parseDurationMs('2m')).toBe(120000);
    expect(parseDurationMs('1h')).toBe(3600000);
    expect(parseDurationMs('1d')).toBe(86400000);
    expect(parseDurationMs('1w')).toBe(7 * 24 * 60 * 60 * 1000);
    expect(parseDurationMs('1mo')).toBe(30 * 24 * 60 * 60 * 1000);
    expect(parseDurationMs('1y')).toBe(365 * 24 * 60 * 60 * 1000);
  });

  it('rejects cache-only values for generic durations', () => {
    expect(parseDurationMs('off')).toBeUndefined();
    expect(() => parseDurationMsStrict('off', 'llmTimeout')).toThrow();
  });

  it('allows cache off semantics for cache durations', () => {
    expect(parseCacheDurationMs('off')).toBe(0);
    expect(parseCacheDurationMsStrict('off', 'cache')).toBe(0);
  });
});
