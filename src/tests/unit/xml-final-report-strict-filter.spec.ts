import { describe, expect, it } from 'vitest';

import { XmlFinalReportStrictFilter } from '../../xml-transport.js';

describe('XmlFinalReportStrictFilter', () => {
  const nonce = 'af834d4d';
  const openTag = `<ai-agent-${nonce}-FINAL format="markdown">`;
  const closeTag = `</ai-agent-${nonce}-FINAL>`;

  it('drops content outside FINAL tag', () => {
    const filter = new XmlFinalReportStrictFilter(nonce);
    const chunk = `preamble ${openTag}report${closeTag} tail`;
    expect(filter.process(chunk)).toBe('report');
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('returns empty string when no tags are present', () => {
    const filter = new XmlFinalReportStrictFilter(nonce);
    expect(filter.process('Hello world')).toBe('');
    expect(filter.hasStreamedContent).toBe(false);
  });

  it('buffers opening tag split across chunks', () => {
    const filter = new XmlFinalReportStrictFilter(nonce);
    const part1 = openTag.slice(0, 12);
    const part2 = openTag.slice(12);
    expect(filter.process(part1)).toBe('');
    expect(filter.process(part2)).toBe('');
    expect(filter.process('Content')).toBe('Content');
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('preserves "<" characters inside content', () => {
    const filter = new XmlFinalReportStrictFilter(nonce);
    filter.process(openTag);
    expect(filter.process('<b>bold</b>')).toBe('<b>bold</b>');
    filter.process(closeTag);
    expect(filter.process('after')).toBe('');
  });

  it('skips leading think block and keeps only final content', () => {
    const filter = new XmlFinalReportStrictFilter(nonce);
    const content = 'Real report content';
    const chunk = `<think>Using ${openTag} in thinking.</think>${openTag}${content}${closeTag}`;
    expect(filter.process(chunk)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });
});
