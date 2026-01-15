import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { afterEach, describe, expect, it, vi } from 'vitest';

import type { TargetContextConfig } from '../../context-guard.js';
import type { LLMClient } from '../../llm-client.js';
import type { ToolOutputConfig, ToolOutputTarget } from '../../tool-output/types.js';
import type { Configuration, TurnRequest, TurnResult } from '../../types.js';

import { ToolOutputExtractor } from '../../tool-output/extractor.js';
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
  });
});

describe('ToolOutputExtractor', () => {
  it('extracts via full-chunked map/reduce', async () => {
    const root = makeTempDir();
    const nonce = 'TESTNONCE';
    const llm = createStubLlmClient([
      `<ai-agent-${nonce}-FINAL format="text">chunk found</ai-agent-${nonce}-FINAL>`,
      `<ai-agent-${nonce}-FINAL format="text">final answer</ai-agent-${nonce}-FINAL>`,
    ]);

    const extractor = new ToolOutputExtractor({
      config: buildConfig(root),
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

    const result = await extractor.extract({
      handle: 'handle-full',
      toolName: 'test__tool',
      toolArgsJson: '{}',
      content: 'line-1\nline-2',
      stats: { bytes: 12, lines: 2, tokens: 6, avgLineBytes: 6 },
    }, 'find data', 'full-chunked', sessionTargets);

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('full-chunked');
    expect(result.text).toBe('final answer');
    expect(result.childOpTree).toBeDefined();
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
  });
});
