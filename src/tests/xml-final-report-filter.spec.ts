import { describe, expect, it } from 'vitest';

import { XmlFinalReportFilter } from '../xml-transport.js';

describe('XmlFinalReportFilter', () => {
  const nonce = 'af834d4d';
  const openTag = `<ai-agent-${nonce}-FINAL format="markdown">`;
  const closeTag = `</ai-agent-${nonce}-FINAL>`;

  it('passes through content when no tags are present', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const chunk = 'Hello world';
    expect(filter.process(chunk)).toBe('Hello world');
    expect(filter.hasStreamedContent).toBe(false);
  });

  it('strips complete tags in a single chunk', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'This is the report';
    const chunk = `${openTag}${content}${closeTag}`;
    
    expect(filter.process(chunk)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('strips tags split across chunks (opening tag)', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Content';
    
    // Split opening tag
    const part1 = openTag.slice(0, 10);
    const part2 = openTag.slice(10);
    
    expect(filter.process(part1)).toBe(''); // Buffering
    expect(filter.process(part2)).toBe(''); // Finished buffering, tag stripped
    expect(filter.process(content)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('strips tags split across chunks (closing tag)', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Content';
    
    // Process open tag and content
    filter.process(openTag);
    expect(filter.process(content)).toBe(content);
    
    // Split closing tag
    const part1 = closeTag.slice(0, 5);
    const part2 = closeTag.slice(5);
    
    expect(filter.process(part1)).toBe('');
    expect(filter.process(part2)).toBe('');
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('flushes buffer on mismatch', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const incomplete = '<ai-agent-WRONG';
    
    // The filter processes char-by-char.
    // It buffers <ai-agent- (valid prefix)
    // Then encounters 'W' (invalid) -> flushes buffer + 'W' + rest
    expect(filter.process(incomplete)).toBe(incomplete);
    
    // Next char mismatch
    expect(filter.process(' ')).toBe(' '); 
    expect(filter.hasStreamedContent).toBe(false);
  });

  it('handles content flowing after done state', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const chunk = `${openTag}Report${closeTag} Trailing`;
    expect(filter.process(chunk)).toBe('Report Trailing');
  });

  it('handles flush correctly', () => {
    const filter = new XmlFinalReportFilter(nonce);
    filter.process('<ai-agent-'); // Partial buffer
    expect(filter.flush()).toBe('<ai-agent-');
  });

  it('skips <think> block at start and extracts content after', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Real report content';
    // Model thinks about the tag, then provides the actual report
    const chunk = `<think>I'll use ${openTag} to wrap my answer.</think>${openTag}${content}${closeTag}`;
    expect(filter.process(chunk)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('skips <think> block with leading whitespace', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Whitespace think content';
    const chunk = `  \n\t<think>Example ${openTag} in thinking.</think>${openTag}${content}${closeTag}`;
    expect(filter.process(chunk)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('skips <think> block split across chunks', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Report after thinking';

    // First chunk has <think> and part of thinking content
    expect(filter.process('<think>Thinking about')).toBe('');
    // Second chunk completes </think> and has the real content
    expect(filter.process(` the tag...</think>${openTag}${content}${closeTag}`)).toBe(content);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('processes normally when content does not start with <think>', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Normal content';
    // Just normal content without <think>
    const chunk = `Some preamble ${openTag}${content}${closeTag}`;
    expect(filter.process(chunk)).toBe(`Some preamble ${content}`);
    expect(filter.hasStreamedContent).toBe(true);
  });

  it('handles content starting with < but not <think>', () => {
    const filter = new XmlFinalReportFilter(nonce);
    const content = 'Content here';
    // Starts with < but not <think>
    const chunk = `<other>stuff</other>${openTag}${content}${closeTag}`;
    expect(filter.process(chunk)).toBe(`<other>stuff</other>${content}`);
    expect(filter.hasStreamedContent).toBe(true);
  });
});
