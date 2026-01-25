import { describe, expect, it } from 'vitest';

import { FinalReportManager } from '../../final-report-manager.js';

const FINAL_REPORT_TOOL = 'agent__final_report';
const SLACK_BLOCK_KIT_FORMAT = 'slack-block-kit' as const;
const JSON_OK_TRUE = '{"ok":true}';
const JSON_OK_INVALID = '{"ok":"yes"}';
const JSON_INVALID_CONTENT_JSON = { ok: 'yes' };
const JSON_SCHEMA = {
  type: 'object',
  required: ['ok'],
  additionalProperties: false,
  properties: {
    ok: { type: 'boolean' },
  },
} as const;

const createManager = () =>
  new FinalReportManager({
    finalReportToolName: FINAL_REPORT_TOOL,
  });

describe('FinalReportManager', () => {
  it('tracks attempts explicitly', () => {
    const manager = createManager();

    expect(manager.finalReportAttempts).toBe(0);
    manager.incrementAttempts();
    manager.incrementAttempts();
    expect(manager.finalReportAttempts).toBe(2);
  });

  it('reports empty-state accessors before any commit', () => {
    const manager = createManager();

    expect(manager.hasReport()).toBe(false);
    expect(manager.getReport()).toBeUndefined();
    expect(manager.getSource()).toBeUndefined();
    expect(manager.getFinalOutput()).toBeUndefined();
    expect(manager.validateSchema(JSON_SCHEMA).valid).toBe(true);
  });

  it('overwrites the committed final report on subsequent commits (current behavior)', () => {
    const manager = createManager();

    manager.commit(
      {
        format: 'markdown',
        content: 'first report',
        metadata: { status: 'success' },
      },
      'tool-call'
    );

    manager.commit(
      {
        format: 'markdown',
        content: 'second report',
        metadata: { status: 'success' },
      },
      'synthetic'
    );

    const committed = manager.getReport();
    expect(committed?.content).toBe('second report');
    expect(manager.getSource()).toBe('synthetic');
  });

  it('clears committed state when clear() is called', () => {
    const manager = createManager();

    manager.commit(
      {
        format: 'markdown',
        content: 'report to clear',
        metadata: { status: 'success' },
      },
      'tool-call'
    );

    manager.clear();

    expect(manager.hasReport()).toBe(false);
    expect(manager.getReport()).toBeUndefined();
    expect(manager.getSource()).toBeUndefined();
    expect(manager.getFinalOutput()).toBeUndefined();
  });

  it('validates JSON payloads against a provided schema', () => {
    const manager = createManager();

    const validResult = manager.validatePayload(
      {
        format: 'json',
        content: JSON_OK_TRUE,
        content_json: { ok: true },
      },
      JSON_SCHEMA
    );
    expect(validResult.valid).toBe(true);

    const invalidResult = manager.validatePayload(
      {
        format: 'json',
        content: JSON_OK_INVALID,
        content_json: JSON_INVALID_CONTENT_JSON,
      },
      JSON_SCHEMA
    );
    expect(invalidResult.valid).toBe(false);
    expect(invalidResult.errors).toContain('ok');
  });

  it('validates committed reports via validateSchema()', () => {
    const manager = createManager();

    manager.commit(
      {
        format: 'json',
        content: JSON_OK_TRUE,
        content_json: { ok: true },
      },
      'tool-call'
    );
    expect(manager.validateSchema(JSON_SCHEMA).valid).toBe(true);

    manager.commit(
      {
        format: 'json',
        content: JSON_OK_INVALID,
        content_json: JSON_INVALID_CONTENT_JSON,
      },
      'tool-call'
    );
    const invalidResult = manager.validateSchema(JSON_SCHEMA);
    expect(invalidResult.valid).toBe(false);
    expect(invalidResult.errors).toContain('ok');
  });

  it('fails slack-block-kit validation when content is not valid JSON', () => {
    const manager = createManager();

    const result = manager.validatePayload(
      {
        format: SLACK_BLOCK_KIT_FORMAT,
        content: 'not-json',
      },
      undefined
    );

    expect(result.valid).toBe(false);
    expect(result.errors).toContain('invalid_json');
  });

  it('extracts a final report payload from text containing a JSON object', () => {
    const manager = createManager();

    const extracted = manager.tryExtractFromText(
      'noise {"report_format":"markdown","report_content":"Hello","metadata":{"status":"success"}} tail'
    );

    expect(extracted?.format).toBe('markdown');
    expect(extracted?.content).toBe('Hello');
    expect(extracted?.metadata).toMatchObject({ status: 'success' });
  });

  it('prefers content_json when rendering final output for json format', () => {
    const manager = createManager();

    manager.commit(
      {
        format: 'json',
        content: JSON_OK_TRUE,
        content_json: { ok: true },
      },
      'tool-call'
    );

    const output = manager.getFinalOutput();
    expect(output).toBe(JSON_OK_TRUE);
  });
});
