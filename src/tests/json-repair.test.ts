import { describe, expect, it } from 'vitest';

import type { OutputFormatId } from '../formats.js';
import type { ToolsOrchestrator } from '../tools/tools.js';

import { InternalToolProvider } from '../tools/internal-provider.js';
import { parseJsonRecordDetailed, parseJsonValueDetailed } from '../utils.js';

const stubOrchestrator = {
  warmup: () => Promise.resolve(),
  execute: () => Promise.resolve({ ok: true, result: '', elapsedMs: 0, kind: 'internal', namespace: 'internal' })
} as unknown as ToolsOrchestrator;

const baseOpts = {
  enableBatch: false,
  outputFormat: 'markdown' as OutputFormatId,
  expectedOutputFormat: 'markdown' as OutputFormatId,
  expectedJsonSchema: undefined,
  maxToolCallsPerTurn: 3,
  updateStatus: (text: string) => { void text; },
  setTitle: (title: string) => { void title; },
  setFinalReport: (payload: unknown) => { void payload; },
  logError: (message: string) => { void message; },
  orchestrator: stubOrchestrator,
  getCurrentTurn: () => 1,
  toolTimeoutMs: 1000,
  disableProgressTool: false,
};

describe('json repair pipeline', () => {
  it('repairs trailing comma via jsonrepair', () => {
    const input = '{"a":1,}';
    const result = parseJsonRecordDetailed(input);
    expect(result.value).toEqual({ a: 1 });
    expect(result.repairs).toContain('jsonrepair');
  });

  it('accepts raw final_report content', async () => {
    const captured: unknown[] = [];
    const provider = new InternalToolProvider('agent', {
      ...baseOpts,
      outputFormat: 'markdown',
      expectedOutputFormat: 'markdown',
      setFinalReport: (payload: { content?: string }) => { captured.push(payload); },
      logError: (msg: string) => { throw new Error(msg); },
    });

    const content = 'hello\nworld';

    const res = await provider.execute('agent__final_report', {
      report_format: 'text',
      report_content: content,
    });

    expect(res.ok).toBe(true);
    const final = captured[0] as { content?: string };
    expect(final.content).toBe(content);
  });

  it('repairs stringified nested JSON to satisfy schema', async () => {
    const captured: unknown[] = [];
    const schema = {
      type: 'object',
      required: ['data'],
      properties: {
        data: {
          type: 'object',
          required: ['value'],
          properties: {
            value: { type: 'string' },
          },
        },
      },
    } as const;

    const provider = new InternalToolProvider('agent', {
      ...baseOpts,
      outputFormat: 'json',
      expectedOutputFormat: 'json',
      expectedJsonSchema: schema as unknown as Record<string, unknown>,
      setFinalReport: (payload: { content_json?: Record<string, unknown> }) => { captured.push(payload); },
      logError: (msg: string) => { throw new Error(msg); },
    });

    const res = await provider.execute('agent__final_report', {
      report_format: 'json',
      content_json: { data: '{"value":"ok"}' },
      report_content: 'unused',
      encoding: 'raw',
    });

    expect(res.ok).toBe(true);
    const final = captured[0] as { content_json?: Record<string, unknown> };
    expect(final.content_json).toEqual({ data: { value: 'ok' } });
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
