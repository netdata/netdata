import { describe, expect, it } from 'vitest';

import { resolveFinalReportContent } from '../../headends/completions-response.js';

describe('resolveFinalReportContent', () => {
  it('prefers json content when format is json', () => {
    const result = resolveFinalReportContent('fallback', {
      format: 'json',
      content_json: { status: 'ok' },
      content: 'ignored',
    });
    expect(result).toBe('{"status":"ok"}');
  });

  it('falls back to content or output when json content is missing', () => {
    const result = resolveFinalReportContent('fallback', {
      format: 'json',
      content: 'report text',
    });
    expect(result).toBe('report text');

    const fallback = resolveFinalReportContent('fallback', 'not-an-object');
    expect(fallback).toBe('fallback');
  });
});
