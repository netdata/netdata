import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { afterEach, describe, expect, it, vi } from 'vitest';

import type { TargetContextConfig } from '../../context-guard.js';
import type { LLMClient } from '../../llm-client.js';
import type { ToolOutputConfig, ToolOutputTarget } from '../../tool-output/types.js';
import type { Configuration, TurnRequest, TurnResult } from '../../types.js';

import { ToolOutputExtractor } from '../../tool-output/extractor.js';
import { formatForGrep } from '../../tool-output/formatter.js';
import { ToolOutputHandler } from '../../tool-output/handler.js';
import { ToolOutputProvider } from '../../tool-output/provider.js';
import { ToolOutputStore } from '../../tool-output/store.js';

vi.mock('../../ai-agent.js', () => ({
  AIAgent: {
    create: vi.fn(() => ({})),
    run: vi.fn(() => Promise.resolve({
      success: true,
      finalReport: { format: 'text', content: 'read-grep-extracted', ts: Date.now() },
      opTree: {
        id: 'tool-output-child',
        traceId: 'trace-tool-output',
        agentId: 'tool_output.read_grep',
        callPath: 'tool_output.read_grep',
        sessionTitle: '',
        startedAt: Date.now(),
        turns: [],
      },
    })),
  },
}));

const makeTempDir = (): string => fs.mkdtempSync(path.join(os.tmpdir(), 'tool-output-'));

const cleanupTempRoot = (root: string): void => {
  try {
    fs.rmSync(root, { recursive: true, force: true });
  } catch {
    // ignore cleanup errors
  }
};

const buildConfig = (storeDir: string): ToolOutputConfig => ({
  enabled: true,
  storeDir,
  maxChunks: 1,
  overlapPercent: 10,
  avgLineBytesThreshold: 1000,
});

const baseConfiguration: Configuration = {
  providers: {},
  mcpServers: {},
  queues: { default: { concurrent: 1 } },
};

const baseTargets: TargetContextConfig[] = [
  {
    provider: 'test',
    model: 'model',
    contextWindow: 2048,
    bufferTokens: 0,
    tokenizerId: 'approximate',
  },
];

const sessionTargets: ToolOutputTarget[] = [{ provider: 'test', model: 'model' }];

const createStubLlmClient = (responses: string[]) => {
  let callIndex = 0;
  return {
    executeTurn: (_request: TurnRequest): Promise<TurnResult> => {
      const content = responses[Math.min(callIndex, responses.length - 1)];
      callIndex += 1;
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        messages: [{ role: 'assistant', content }],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      });
    },
  } as unknown as { executeTurn: (request: TurnRequest) => Promise<TurnResult> };
};

afterEach(() => {
  vi.clearAllMocks();
});

describe('ToolOutputHandler', () => {
  it('stores oversized output based on size cap', async () => {
    const root = makeTempDir();
    const store = new ToolOutputStore(root, 'session-size');
    const handler = new ToolOutputHandler(store, buildConfig(root));

    const result = await handler.maybeStore({
      toolName: 'test__tool',
      toolArgsJson: '{}',
      output: 'X'.repeat(200),
      sizeLimitBytes: 100,
    });

    expect(result).toBeDefined();
    expect(result?.reason).toBe('size_cap');
    expect(result?.message).toContain('tool_output(handle = "');
    const handle = result?.handle;
    if (handle === undefined) throw new Error('expected tool_output handle');
    const read = await store.read(handle);
    expect(read?.content).toBe('X'.repeat(200));
    await store.cleanup();
    cleanupTempRoot(root);
  });

  it('stores oversized output when budget preview fails', async () => {
    const root = makeTempDir();
    const store = new ToolOutputStore(root, 'session-budget');
    const handler = new ToolOutputHandler(store, buildConfig(root));

    const result = await handler.maybeStore({
      toolName: 'test__tool',
      toolArgsJson: '{}',
      output: 'Y'.repeat(200),
      sizeLimitBytes: 1000,
      budgetCallbacks: {
        previewToolOutput: () => Promise.resolve({ ok: false, tokens: 999, reason: 'token_budget_exceeded' }),
        reserveToolOutput: () => Promise.resolve({ ok: true, tokens: 1 }),
        canExecuteTool: () => true,
        countTokens: (text) => Math.ceil(text.length / 4),
      },
    });

    expect(result).toBeDefined();
    expect(result?.reason).toBe('token_budget');
    await store.cleanup();
    cleanupTempRoot(root);
  });
});

describe('ToolOutputExtractor', () => {
  it('extracts via full-chunked map/reduce', async () => {
    const root = makeTempDir();
    const nonce = 'TESTNONCE';
    const llm = createStubLlmClient([
      `<ai-agent-${nonce}-FINAL format="text">chunk found</ai-agent-${nonce}-FINAL>`,
      `<ai-agent-${nonce}-FINAL format="text">chunk found 2</ai-agent-${nonce}-FINAL>`,
      `<ai-agent-${nonce}-FINAL format="text">final answer</ai-agent-${nonce}-FINAL>`,
    ]);

    // Need maxChunks > 1 to test map/reduce path
    const multiChunkConfig: ToolOutputConfig = { ...buildConfig(root), maxChunks: 10 };
    const extractor = new ToolOutputExtractor({
      config: multiChunkConfig,
      llmClient: llm as unknown as LLMClient,
      targets: baseTargets,
      computeMaxOutputTokens: () => 256,
      sessionTargets,
      sessionNonce: nonce,
      sessionId: 'session-full',
      agentId: 'agent',
      callPath: 'agent',
      toolResponseMaxBytes: 4000,
      temperature: null,
      topP: null,
      topK: null,
      repeatPenalty: null,
      maxOutputTokens: 256,
      reasoning: undefined,
      reasoningValue: undefined,
      llmTimeout: 0,
      toolTimeout: 0,
      caching: 'none',
      traceLLM: false,
      traceMCP: false,
      traceSdk: false,
      pricing: undefined,
      countTokens: (text) => Math.ceil(text.length / 4),
      fsRootDir: root,
      buildFsServerConfig: (rootDir) => ({ name: 'tool_output_fs', config: { ...baseConfiguration, mcpServers: { tool_output_fs: { type: 'stdio', command: process.execPath, args: [rootDir] } } } }),
    });

    // Content must be large enough to require multiple chunks to trigger reduce step
    // With contextWindow=2048, maxOutputTokens=256, we need content > ~1500 tokens
    // countTokens returns length/4, so we need ~6000+ bytes for 2 chunks
    const largeContent = 'X'.repeat(8000);
    const result = await extractor.extract({
      handle: 'handle-full',
      toolName: 'test__tool',
      toolArgsJson: '{}',
      content: largeContent,
      stats: { bytes: 8000, lines: 1, tokens: 2000, avgLineBytes: 8000 },
    }, 'find data', 'full-chunked', sessionTargets);

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('full-chunked');
    expect(result.text).toBe('final answer');
    expect(result.childOpTree).toBeDefined();
    cleanupTempRoot(root);
  });

  it('extracts via read-grep with mocked sub-agent', async () => {
    const root = makeTempDir();
    const extractor = new ToolOutputExtractor({
      config: buildConfig(root),
      llmClient: createStubLlmClient(['']) as unknown as LLMClient,
      targets: baseTargets,
      computeMaxOutputTokens: () => 256,
      sessionTargets,
      sessionNonce: 'READGREPNONCE',
      sessionId: 'session-read-grep',
      agentId: 'agent',
      callPath: 'agent',
      toolResponseMaxBytes: 4000,
      temperature: null,
      topP: null,
      topK: null,
      repeatPenalty: null,
      maxOutputTokens: 256,
      reasoning: undefined,
      reasoningValue: undefined,
      llmTimeout: 0,
      toolTimeout: 0,
      caching: 'none',
      traceLLM: false,
      traceMCP: false,
      traceSdk: false,
      pricing: undefined,
      countTokens: (text) => Math.ceil(text.length / 4),
      fsRootDir: root,
      buildFsServerConfig: (rootDir) => ({ name: 'tool_output_fs', config: { ...baseConfiguration, mcpServers: { tool_output_fs: { type: 'stdio', command: process.execPath, args: [rootDir] } } } }),
    });

    const result = await extractor.extract({
      handle: 'handle-read',
      toolName: 'test__tool',
      toolArgsJson: '{}',
      content: 'payload',
      stats: { bytes: 7, lines: 1, tokens: 2, avgLineBytes: 7 },
    }, 'find value', 'read-grep', sessionTargets);

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('read-grep');
    expect(result.text).toBe('read-grep-extracted');
    expect(result.childOpTree).toBeDefined();
    cleanupTempRoot(root);
  });
});

describe('ToolOutputProvider', () => {
  it('returns a handle-not-found message for missing entries', async () => {
    const root = makeTempDir();
    const store = new ToolOutputStore(root, 'session-provider');
    const extractor = new ToolOutputExtractor({
      config: buildConfig(root),
      llmClient: createStubLlmClient(['']) as unknown as LLMClient,
      targets: baseTargets,
      computeMaxOutputTokens: () => 256,
      sessionTargets,
      sessionNonce: 'PROVIDER',
      sessionId: 'session-provider',
      agentId: 'agent',
      callPath: 'agent',
      toolResponseMaxBytes: 4000,
      temperature: null,
      topP: null,
      topK: null,
      repeatPenalty: null,
      maxOutputTokens: 256,
      reasoning: undefined,
      reasoningValue: undefined,
      llmTimeout: 0,
      toolTimeout: 0,
      caching: 'none',
      traceLLM: false,
      traceMCP: false,
      traceSdk: false,
      pricing: undefined,
      countTokens: (text) => Math.ceil(text.length / 4),
      fsRootDir: root,
      buildFsServerConfig: (rootDir) => ({ name: 'tool_output_fs', config: { ...baseConfiguration, mcpServers: { tool_output_fs: { type: 'stdio', command: process.execPath, args: [rootDir] } } } }),
    });

    const provider = new ToolOutputProvider(store, buildConfig(root), extractor, sessionTargets, 4000);
    const result = await provider.execute('tool_output', { handle: 'missing', extract: 'find' });

    expect(result.ok).toBe(true);
    expect(result.result).toContain('No stored tool output found');
    await store.cleanup();
    cleanupTempRoot(root);
  });
});

describe('formatForGrep', () => {
  it('pretty-prints minified JSON', () => {
    const minified = '{"key1":"value1","key2":"value2"}';
    const result = formatForGrep(minified);
    expect(result).toContain('\n');
    expect(result).toContain('"key1":');
    expect(result).toContain('"key2":');
  });

  it('pretty-prints JSON arrays', () => {
    const minified = '[{"a":1},{"b":2}]';
    const result = formatForGrep(minified);
    expect(result).toContain('\n');
  });

  it('splits XML tags with >< pattern', () => {
    const xml = '<root><item>value</item><item>value2</item></root>';
    const result = formatForGrep(xml);
    expect(result).toContain('>\n<');
    expect(result.split('\n').length).toBeGreaterThan(1);
  });

  it('normalizes whitespace between XML tags', () => {
    const xml = '<root>   <item>test</item>  <other/></root>';
    const result = formatForGrep(xml);
    expect(result).not.toContain('>   <');
    expect(result).toContain('>\n<');
  });

  it('replaces escaped \\n with actual newlines', () => {
    const content = 'line1\\nline2\\nline3';
    const result = formatForGrep(content);
    expect(result).toBe('line1\nline2\nline3');
  });

  it('replaces escaped \\n in JSON strings after pretty-print', () => {
    const json = '{"msg":"hello\\nworld"}';
    const result = formatForGrep(json);
    expect(result).toContain('hello\nworld');
  });

  it('breaks long lines at word boundaries', () => {
    // 250 words * 5 chars = 1250 chars, well over 1000 threshold
    const words = Array(250).fill('word').join(' ');
    const result = formatForGrep(words);
    const lines = result.split('\n');
    expect(lines.length).toBeGreaterThan(1);
    lines.forEach(line => {
      expect(line.length).toBeLessThanOrEqual(1200); // 1000 + some tolerance
    });
  });

  it('breaks long lines at symbols', () => {
    const urls = Array(50).fill('https://example.com/path/to/resource?query=value').join('');
    const result = formatForGrep(urls);
    const lines = result.split('\n');
    expect(lines.length).toBeGreaterThan(1);
  });

  it('force-breaks very long lines at UTF-8 boundaries', () => {
    // Create a line with no break characters
    const noBreaks = 'a'.repeat(3000);
    const result = formatForGrep(noBreaks);
    const lines = result.split('\n');
    expect(lines.length).toBeGreaterThan(1);
    lines.forEach(line => {
      expect(Buffer.byteLength(line, 'utf8')).toBeLessThanOrEqual(2000);
    });
  });

  it('preserves multi-byte UTF-8 characters without splitting them', () => {
    // Japanese characters (3 bytes each in UTF-8)
    const japanese = 'あ'.repeat(800); // 2400 bytes
    const result = formatForGrep(japanese);
    const lines = result.split('\n');
    expect(lines.length).toBeGreaterThan(1);
    // Each line should be valid UTF-8 (no split characters)
    lines.forEach(line => {
      expect(() => Buffer.from(line, 'utf8').toString()).not.toThrow();
    });
  });

  it('handles mixed JSON with long strings', () => {
    const longValue = 'x'.repeat(1500);
    const json = `{"data":"${longValue}"}`;
    const result = formatForGrep(json);
    const lines = result.split('\n');
    // Should be broken into multiple lines
    expect(lines.length).toBeGreaterThan(1);
  });

  it('leaves short content unchanged', () => {
    const short = 'hello world';
    const result = formatForGrep(short);
    expect(result).toBe('hello world');
  });

  it('handles empty content', () => {
    const result = formatForGrep('');
    expect(result).toBe('');
  });

  it('handles non-JSON content that looks like JSON but is invalid', () => {
    const invalid = '{not valid json';
    const result = formatForGrep(invalid);
    expect(result).toBe('{not valid json');
  });
});
