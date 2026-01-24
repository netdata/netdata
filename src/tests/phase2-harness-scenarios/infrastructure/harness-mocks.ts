/**
 * Mock classes and factory functions for the Phase 2 deterministic test harness.
 * Extracted from phase2-runner.ts for modularity.
 */

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { gzipSync } from 'node:zlib';

import type { ResolvedConfigLayer } from '../../../config-resolver.js';
import type { LogFn, MCPRestartFailedError, SharedAcquireOptions, SharedRegistry, SharedRegistryHandle } from '../../../tools/mcp-provider.js';
import type { AIAgentEventCallbacks, AIAgentResult, AIAgentSessionConfig, AccountingEntry, Configuration, MCPServer, MCPServerConfig, MCPTool } from '../../../types.js';

import { warn } from '../../../utils.js';

import { toErrorMessage } from './harness-helpers.js';

/** Prefix for temporary directories */
export const TMP_PREFIX = 'ai-agent-phase2-';

/** Default model name for tests */
export const MODEL_NAME = 'deterministic-model';

/** Primary provider name for tests */
export const PRIMARY_PROVIDER = 'primary';

/** Secondary provider name for tests */
export const SECONDARY_PROVIDER = 'secondary';

/** Base defaults for test sessions */
export const BASE_DEFAULTS = {
  stream: false,
  maxTurns: 3,
  maxRetries: 2,
  llmTimeout: 10_000,
  toolTimeout: 10_000,
} as const;

/**
 * Mock SharedRegistry implementation for testing MCP tool orchestration.
 */
export class HarnessSharedRegistry implements SharedRegistry {
  acquireCount = 0;
  timeoutCalls: string[] = [];
  restartCalls = 0;
  releaseCount = 0;
  cancelReasons: ('timeout' | 'abort')[] = [];
  callResults: unknown[] = [];
  probeResults: boolean[] = [];
  lastHandle?: HarnessSharedHandle;

  acquire(serverName: string, config: MCPServerConfig, opts: SharedAcquireOptions): Promise<SharedRegistryHandle> {
    this.acquireCount += 1;
    const baseTools: MCPTool[] = [{ name: 'mock_tool', description: 'mock', inputSchema: {} }];
    const tools = opts.filterTools(baseTools);
    const server: MCPServer = { name: serverName, config, tools, instructions: 'mock instructions' };
    const handle = new HarnessSharedHandle(this, server);
    handle.callResults = this.callResults;
    handle.probeResults = this.probeResults;
    handle.cancelStore = this.cancelReasons;
    this.lastHandle = handle;
    return Promise.resolve(handle);
  }

  getRestartError(): MCPRestartFailedError | undefined {
    return undefined;
  }
}

/**
 * Mock SharedRegistryHandle implementation for testing MCP tool orchestration.
 */
export class HarnessSharedHandle implements SharedRegistryHandle {
  public callResults: unknown[] = [];
  public probeResults: boolean[] = [];
  public cancelStore: ('timeout' | 'abort')[] = [];
  public callCount = 0;
  public released = false;

  constructor(private registry: HarnessSharedRegistry, public server: MCPServer) {}

  callTool(): Promise<unknown> {
    this.callCount += 1;
    return Promise.resolve(this.callResults.shift() ?? { content: 'mock-result' });
  }

  handleCancel(reason: 'timeout' | 'abort', logger: LogFn): Promise<void> {
    this.cancelStore.push(reason);
    if (reason !== 'timeout') return Promise.resolve();
    this.registry.timeoutCalls.push(this.server.name);
    const nextProbe = this.probeResults.shift();
    const healthy = nextProbe ?? true;
    if (!healthy) {
      this.registry.restartCalls += 1;
      logger('WRN', `mock restart for '${this.server.name}'`, `mcp:${this.server.name}`);
    }
    return Promise.resolve();
  }

  release(): void {
    if (!this.released) {
      this.released = true;
      this.registry.releaseCount += 1;
    }
  }
}

/**
 * Create a basic test configuration with a test-llm provider.
 */
export function makeBasicConfiguration(): Configuration {
  return {
    providers: {
      [PRIMARY_PROVIDER]: {
        type: 'test-llm',
        models: {
          [MODEL_NAME]: {
            contextWindow: 8192,
            tokenizer: 'approximate',
            contextWindowBufferTokens: 256,
          },
        },
      },
    },
    mcpServers: {},
    queues: { default: { concurrent: 32 } },
  };
}

/**
 * Create a successful AIAgentResult with the given content.
 */
export function makeSuccessResult(content: string): AIAgentResult {
  return {
    success: true,
    conversation: [],
    logs: [],
    accounting: [],
    finalReport: {
      format: 'text',
      content,
      ts: Date.now(),
    },
  };
}

/**
 * Create a parent session stub for sub-agent testing.
 */
export function createParentSessionStub(configuration: Configuration): Pick<
  AIAgentSessionConfig,
  | 'config'
  | 'callbacks'
  | 'stream'
  | 'traceLLM'
  | 'traceMCP'
  | 'traceSdk'
  | 'verbose'
  | 'temperature'
  | 'topP'
  | 'llmTimeout'
  | 'toolTimeout'
  | 'maxRetries'
  | 'maxTurns'
  | 'toolResponseMaxBytes'
  | 'targets'
> {
  return {
    config: configuration,
    callbacks: undefined,
    stream: false,
    traceLLM: false,
    traceMCP: false,
    traceSdk: false,
    verbose: false,
    temperature: 0.7,
    topP: 1,
    llmTimeout: 10_000,
    toolTimeout: 10_000,
    maxRetries: 2,
    maxTurns: 10,
    toolResponseMaxBytes: 65_536,
    targets: [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }],
  };
}

/**
 * Build in-memory config layers from a Configuration object.
 */
export function buildInMemoryConfigLayers(configuration: Configuration): ResolvedConfigLayer[] {
  const jsonClone = JSON.parse(JSON.stringify(configuration)) as Record<string, unknown>;
  return [{
    origin: '--config',
    jsonPath: '__in-memory-config__',
    envPath: '__in-memory-env__',
    json: jsonClone,
    env: {},
  }];
}

/**
 * Create a temporary directory with an optional label.
 */
export function makeTempDir(label: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}${label}-`));
}

/**
 * Create default persistence callbacks for session snapshot and billing.
 */
export const defaultPersistenceCallbacks = (
  configuration: Configuration,
  existing?: AIAgentEventCallbacks,
): AIAgentEventCallbacks => {
  const callbacks = existing ?? {};
  const persistence = configuration.persistence ?? {};
  const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
  const defaultBase = home.length > 0 ? path.join(home, '.ai-agent') : undefined;
  const sessionsDir = typeof persistence.sessionsDir === 'string' && persistence.sessionsDir.length > 0
    ? persistence.sessionsDir
    : defaultBase !== undefined ? path.join(defaultBase, 'sessions') : undefined;
  const ledgerFile = typeof persistence.billingFile === 'string' && persistence.billingFile.length > 0
    ? persistence.billingFile
    : configuration.accounting?.file ?? (defaultBase !== undefined ? path.join(defaultBase, 'accounting.jsonl') : undefined);

  const snapshotHandler = sessionsDir !== undefined ? (payload: { reason?: string; originId: string; snapshot: { version: number; opTree: unknown } }) => {
    try {
      fs.mkdirSync(sessionsDir, { recursive: true });
      const json = JSON.stringify({
        version: payload.snapshot.version,
        reason: payload.reason,
        opTree: payload.snapshot.opTree,
      });
      const gz = gzipSync(Buffer.from(json, 'utf8'));
      const filePath = path.join(sessionsDir, `${payload.originId}.json.gz`);
      const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
      fs.writeFileSync(tmp, gz);
      fs.renameSync(tmp, filePath);
    } catch (error: unknown) {
      const message = toErrorMessage(error);
      const reason = payload.reason ?? 'unspecified';
      warn(`persistSessionSnapshot(${reason}) failed: ${message}`);
    }
  } : undefined;

  const ledgerHandler = ledgerFile !== undefined ? (payload: { entries: AccountingEntry[] }) => {
    try {
      const dir = path.dirname(ledgerFile);
      fs.mkdirSync(dir, { recursive: true });
      if (payload.entries.length === 0) {
        return;
      }
      const lines = payload.entries.map((entry) => JSON.stringify(entry));
      fs.appendFileSync(ledgerFile, `${lines.join('\n')}\n`, 'utf8');
    } catch (error: unknown) {
      const message = toErrorMessage(error);
      warn(`final persistence failed: ${message}`);
    }
  } : undefined;

  const needsSnapshot = snapshotHandler !== undefined;
  const needsLedger = ledgerHandler !== undefined;
  if (!needsSnapshot && !needsLedger) {
    return callbacks;
  }
  const baseOnEvent = callbacks.onEvent;
  return {
    ...callbacks,
    onEvent: (event, meta) => {
      if (event.type === 'snapshot' && snapshotHandler !== undefined) {
        snapshotHandler(event.payload);
      }
      if (event.type === 'accounting_flush' && ledgerHandler !== undefined) {
        ledgerHandler(event.payload);
      }
      baseOnEvent?.(event, meta);
    },
  };
};

/**
 * Check if an accounting entry is for a tool call.
 */
export function isToolAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'tool' } {
  return entry.type === 'tool';
}

/**
 * Check if an accounting entry is for an LLM call.
 */
export function isLlmAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'llm' } {
  return entry.type === 'llm';
}
