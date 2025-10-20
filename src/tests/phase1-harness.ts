import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

import { WebSocketServer } from 'ws';

import type { LoadAgentOptions, LoadedAgent } from '../agent-loader.js';
import type { ResolvedConfigLayer } from '../config-resolver.js';
import type { PreloadedSubAgent } from '../subagent-registry.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, AccountingEntry, AgentFinishedEvent, Configuration, ConversationMessage, LogEntry, MCPServerConfig, MCPTool, ProviderConfig, TurnRequest, TurnResult, TurnStatus } from '../types.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import type { ChildProcess } from 'node:child_process';
import type { AddressInfo } from 'node:net';

import { loadAgent } from '../agent-loader.js';
import { AgentRegistry } from '../agent-registry.js';
import { AIAgentSession } from '../ai-agent.js';
import { loadConfiguration } from '../config.js';
import { parseFrontmatter, parseList, parsePairs } from '../frontmatter.js';
import { resolveIncludes } from '../include-resolver.js';
import { DEFAULT_TOOL_INPUT_SCHEMA } from '../input-contract.js';
import { LLMClient } from '../llm-client.js';
import { TestLLMProvider } from '../llm-providers/test-llm.js';
import { SubAgentRegistry } from '../subagent-registry.js';
import { MCPProvider } from '../tools/mcp-provider.js';
import { clampToolName, sanitizeToolName, formatToolRequestCompact, truncateUtf8WithNotice, formatAgentResultHumanReadable, setWarningSink, getWarningSink, warn } from '../utils.js';
import { createWebSocketTransport } from '../websocket-transport.js';

import { getScenario } from './fixtures/test-llm-scenarios.js';
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const MODEL_NAME = 'deterministic-model';
const PRIMARY_PROVIDER = 'primary';
const SECONDARY_PROVIDER = 'secondary';
const SUBAGENT_TOOL = 'agent__pricing-subagent';
const COVERAGE_CHILD_TOOL = 'coverage.child';
const COVERAGE_PARENT_BODY = 'Parent body.';
const COVERAGE_CHILD_BODY = 'Child body.';

const toError = (value: unknown): Error => (value instanceof Error ? value : new Error(String(value)));
const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

const defaultHarnessWarningSink = (message: string): void => {
  const prefix = '[warn] ';
  const output = process.stderr.isTTY ? `\x1b[33m${prefix}${message}\x1b[0m` : `${prefix}${message}`;
  try { process.stderr.write(`${output}\n`); } catch { /* ignore */ }
};

setWarningSink(defaultHarnessWarningSink);

const defaultPersistenceCallbacks = (configuration: Configuration, existing?: AIAgentCallbacks): AIAgentCallbacks => {
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

  const snapshotHandler = callbacks.onSessionSnapshot ?? (sessionsDir !== undefined ? async (payload) => {
    try {
      await fs.promises.mkdir(sessionsDir, { recursive: true });
      const json = JSON.stringify({
        version: payload.snapshot.version,
        reason: payload.reason,
        opTree: payload.snapshot.opTree,
      });
      const gz = gzipSync(Buffer.from(json, 'utf8'));
      const filePath = path.join(sessionsDir, `${payload.originId}.json.gz`);
      const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
      await fs.promises.writeFile(tmp, gz);
      await fs.promises.rename(tmp, filePath);
    } catch (error: unknown) {
      const message = toErrorMessage(error);
      const reason = payload.reason ?? 'unspecified';
      warn(`persistSessionSnapshot(${reason}) failed: ${message}`);
    }
  } : undefined);

  const ledgerHandler = callbacks.onAccountingFlush ?? (ledgerFile !== undefined ? async (payload) => {
    try {
      const dir = path.dirname(ledgerFile);
      await fs.promises.mkdir(dir, { recursive: true });
      if (payload.entries.length === 0) {
        return;
      }
      const lines = payload.entries.map((entry) => JSON.stringify(entry));
      await fs.promises.appendFile(ledgerFile, `${lines.join('\n')}\n`, 'utf8');
    } catch (error: unknown) {
      const message = toErrorMessage(error);
      warn(`final persistence failed: ${message}`);
    }
  } : undefined);

  const needsSnapshot = snapshotHandler !== callbacks.onSessionSnapshot;
  const needsLedger = ledgerHandler !== callbacks.onAccountingFlush;
  if (!needsSnapshot && !needsLedger) {
    return callbacks;
  }
  return {
    ...callbacks,
    ...(needsSnapshot && snapshotHandler !== undefined ? { onSessionSnapshot: snapshotHandler } : {}),
    ...(needsLedger && ledgerHandler !== undefined ? { onAccountingFlush: ledgerHandler } : {}),
  };
};

const baseCreate = AIAgentSession.create.bind(AIAgentSession);
(AIAgentSession as unknown as { create: (sessionConfig: AIAgentSessionConfig) => AIAgentSession }).create = (sessionConfig: AIAgentSessionConfig) => {
  const originalCallbacks = sessionConfig.callbacks;
  const callbacks = defaultPersistenceCallbacks(sessionConfig.config, originalCallbacks);
  const patchedConfig = callbacks === originalCallbacks ? sessionConfig : { ...sessionConfig, callbacks };
  return baseCreate(patchedConfig);
};

const COVERAGE_ALIAS_BASENAME = 'parent.ai';
const TRUNCATION_NOTICE = '[TRUNCATED]';
const CONFIG_FILE_NAME = 'config.json';
const INCLUDE_DIRECTIVE_TOKEN = '${include:';
const FINAL_REPORT_CALL_ID = 'call-final-report';
const RATE_LIMIT_FINAL_CONTENT = 'Final report after rate limit.';
const ADOPTED_FINAL_CONTENT = 'Adopted final report content.';
const ADOPTION_METADATA_ORIGIN = 'text-adoption';
const ADOPTION_CONTENT_VALUE = 'value';
const FINAL_REPORT_SANITIZED_CONTENT = 'Final report without sanitized tool calls.';
const JSON_ONLY_URL = 'https://example.com/resource';
const DEFAULT_PROMPT_SCENARIO = 'run-test-1' as const;
const BASE_DEFAULTS = {
  stream: false,
  maxToolTurns: 3,
  maxRetries: 2,
  llmTimeout: 10_000,
  toolTimeout: 10_000,
} as const;
const TMP_PREFIX = 'ai-agent-phase1-';
const SESSIONS_SUBDIR = 'sessions';
const BILLING_FILENAME = 'billing.jsonl';

function buildInMemoryConfigLayers(configuration: Configuration): ResolvedConfigLayer[] {
  const jsonClone = JSON.parse(JSON.stringify(configuration)) as Record<string, unknown>;
  return [{
    origin: '--config',
    jsonPath: '__in-memory-config__',
    envPath: '__in-memory-env__',
    json: jsonClone,
    env: {},
  }];
}

function deriveToolName(promptPath: string): string {
  const base = path.basename(promptPath, path.extname(promptPath));
  const sanitized = sanitizeToolName(base);
  const normalized = sanitized.length > 0 ? sanitized.toLowerCase() : sanitized;
  return clampToolName(normalized).name;
}

function preloadSubAgentFromPath(promptPath: string, configuration: Configuration): PreloadedSubAgent {
  const raw = fs.readFileSync(promptPath, 'utf-8');
  const fm = parseFrontmatter(raw, { baseDir: path.dirname(promptPath) });
  const layers = buildInMemoryConfigLayers(configuration);
  const loaded = loadAgent(promptPath, undefined, { configLayers: layers });
  const toolName = loaded.toolName ?? deriveToolName(loaded.promptPath);
  if (typeof loaded.description !== 'string' || loaded.description.trim().length === 0) {
    throw new Error(`Preloaded sub-agent '${loaded.promptPath}' missing description.`);
  }
  if (loaded.toolName === undefined) {
    (loaded as { toolName?: string }).toolName = toolName;
  }
  return {
    toolName,
    description: loaded.description,
    usage: loaded.usage,
    inputFormat: loaded.input.format === 'json' ? 'json' : 'text',
    inputSchema: loaded.input.schema,
    hasExplicitInputSchema: fm?.inputSpec !== undefined,
    promptPath: loaded.promptPath,
    systemTemplate: loaded.systemTemplate,
    loaded,
  };
}

function resolveScenarioDescription(id: string, provided?: string): string | undefined {
  if (provided !== undefined && provided.trim().length > 0) return provided;
  const scenario = getScenario(id);
  return scenario?.description;
}

function runWithTimeout<T>(promise: Promise<T>, timeoutMs: number, scenarioId: string, onTimeout?: () => void): Promise<T> {
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) return promise;
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    let timer: ReturnType<typeof setTimeout>;
    const finalize = (action: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      action();
    }
    timer = setTimeout(() => {
      finalize(() => {
        try { onTimeout?.(); } catch { /* ignore */ }
        reject(new Error(`Scenario ${scenarioId} timed out after ${String(timeoutMs)} ms`));
      });
    }, timeoutMs);
    void (async () => {
      try {
        const value = await promise;
        finalize(() => { resolve(value); });
      } catch (error: unknown) {
        finalize(() => { reject(toError(error)); });
      }
    })();
  });
}

function formatDurationMs(startMs: number, endMs: number): string {
  const seconds = (endMs - startMs) / 1000;
  return `${seconds.toFixed(RUN_LOG_DECIMAL_PRECISION)}s`;
}

function getEffectiveTimeoutMs(test: HarnessTest): number {
  if (typeof test.timeoutMs === 'number' && Number.isFinite(test.timeoutMs) && test.timeoutMs > 0) {
    return test.timeoutMs;
  }
  return DEFAULT_SCENARIO_TIMEOUT_MS;
}

const SUBAGENT_PRICING_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents/pricing-subagent.ai');
const SUBAGENT_PRICING_SUCCESS_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents-success/pricing-subagent-success.ai');
const SUBAGENT_SUCCESS_TOOL = 'agent__pricing-subagent-success';
const SUBAGENT_SCHEMA_FALLBACK_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents/schema-fallback.ai');
const SUBAGENT_SCHEMA_JSON_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents/schema-json.ai');
const AGENT_SCHEMA_DEFAULT_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/agents/schema-default.ai');
const AGENT_SCHEMA_JSON_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/agents/schema-json.ai');
const CONCURRENCY_TIMEOUT_ARGUMENT = 'trigger-timeout';
const CONCURRENCY_SECOND_ARGUMENT = 'concurrency-second';
const THROW_FAILURE_MESSAGE = 'Simulated provider throw for coverage.';
const BATCH_PROGRESS_RESPONSE = 'batch-progress-follow-up';
const BATCH_STRING_RESULT = 'batch-string-mode';
const FINAL_ANSWER_DELIVERED = 'Final answer delivered.';
const TRACEABLE_PROVIDER_URL = 'https://openrouter.ai/api/v1';
const RATE_LIMIT_WARNING_TOKEN = 'rate limit';
const CONFIG_FILENAME = '.ai-agent.json';
const FRONTMATTER_BODY_PREFIX = 'System body start.';
const FRONTMATTER_BODY_SUFFIX = 'System body end.';
const FRONTMATTER_INCLUDE_MARKER = 'INCLUDED RAW BLOCK';
const FRONTMATTER_INCLUDE_FLAG = 'includeFrontmatter: true';
const LONG_TOOL_NAME = `tool-${'x'.repeat(140)}`;
const FINAL_REPORT_RETRY_MESSAGE = 'Final report completed after mixed tools.';
const SANITIZER_VALID_ARGUMENT = 'sanitizer-valid-call';
const SANITIZER_REMOTE_IDENTIFIER = 'agent:sanitizer';
const EXIT_FINAL_REPORT_IDENTIFIER = 'agent:EXIT-FINAL-ANSWER';

const GITHUB_SERVER_PATH = path.resolve(__dirname, 'mcp/github-stdio-server.js');

const SLACK_OUTPUT_FORMAT = 'slack-block-kit' as const;

const DEFAULT_SCENARIO_TIMEOUT_MS = (() => {
  const raw = process.env.PHASE1_SCENARIO_TIMEOUT_MS;
  if (raw === undefined) return 10_000;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 10_000;
})();

const RUN_LOG_DECIMAL_PRECISION = 2;
const CONTENT_TYPE_JSON = 'application/json';
const CONTENT_TYPE_EVENT_STREAM = 'text/event-stream';
const COVERAGE_OPENROUTER_JSON_ID = 'coverage-openrouter-json';
const COVERAGE_OPENROUTER_SSE_ID = 'coverage-openrouter-sse';
const COVERAGE_OPENROUTER_SSE_NONBLOCKING_ID = 'coverage-openrouter-sse-nonblocking';
const COVERAGE_GENERIC_JSON_ID = 'coverage-generic-json';
const COVERAGE_SESSION_ID = 'coverage-session-snapshot';
const COVERAGE_ROUTER_PROVIDER = 'router-provider';
const COVERAGE_ROUTER_MODEL = 'router-model';
const COVERAGE_ROUTER_SSE_PROVIDER = 'router-sse';
const COVERAGE_ROUTER_SSE_MODEL = 'router-sse-model';
const OPENROUTER_RESPONSES_URL = 'https://openrouter.ai/api/v1/responses';
const COVERAGE_PROMPT = 'coverage';
const COVERAGE_WARN_SUBSTRING = 'persistSessionSnapshot(coverage-failure) failed';

function makeTempDir(label: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}${label}-`));
}

let runTest16Paths: { sessionsDir: string; billingFile: string } | undefined;
let runTest20Paths: { baseDir: string; blockerFile: string; billingFile: string } | undefined;
let runTest54State: { received: JSONRPCMessage[]; serverPayloads: string[]; errors: string[] } | undefined;
let coverageOpenrouterJson: {
  accept?: string;
  referer?: string;
  title?: string;
  userAgent?: string;
  logs: LogEntry[];
  routed: { provider?: string; model?: string };
  cost: { costUsd?: number; upstreamInferenceCostUsd?: number };
  response: unknown;
} | undefined;
let coverageOpenrouterSse: {
  accept?: string;
  logs: LogEntry[];
  routed: { provider?: string; model?: string };
  cost: { costUsd?: number; upstreamInferenceCostUsd?: number };
  rawStream: string;
} | undefined;
let coverageOpenrouterSseNonBlocking: {
  raceResult: 'response' | 'timeout';
  elapsedBeforeMetadataMs: number;
  totalElapsedMs: number;
  routed: { provider?: string; model?: string };
  cost: { costUsd?: number; upstreamInferenceCostUsd?: number };
  logs: LogEntry[];
} | undefined;
let coverageGenericJson: {
  accept?: string;
  cacheWrite?: number;
  logs: LogEntry[];
} | undefined;
let coverageSessionSnapshot: {
  tempDir: string;
  filesAfterSuccess: string[];
  filesAfterFailure: string[];
  warnOutput: string;
} | undefined;

function invariant(condition: boolean, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function getPrivateMethod(instance: object, key: string): (...args: unknown[]) => unknown {
  const value = Reflect.get(instance as Record<string, unknown>, key);
  invariant(typeof value === 'function', `Private method '${key}' missing.`);
  return value as (...args: unknown[]) => unknown;
}

function getPrivateField(instance: object, key: string): unknown {
  return Reflect.get(instance as Record<string, unknown>, key);
}

function makeSuccessResult(content: string): AIAgentResult {
  return {
    success: true,
    conversation: [],
    logs: [],
    accounting: [],
    finalReport: {
      status: 'success',
      format: 'text',
      content,
      ts: Date.now(),
    },
  };
}

function createDeferred<T = void>(): { promise: Promise<T>; resolve: (value: T) => void; reject: (reason?: unknown) => void } {
  let resolveFn: ((value: T) => void) | undefined;
  let rejectFn: ((reason?: unknown) => void) | undefined;
  const promise = new Promise<T>((resolve, reject) => {
    resolveFn = resolve;
    rejectFn = reject;
  });
  const resolve = (value: T): void => { resolveFn?.(value); };
  const reject = (reason?: unknown): void => { rejectFn?.(reason); };
  return { promise, resolve, reject };
}

function createParentSessionStub(configuration: Configuration): Pick<AIAgentSessionConfig, 'config' | 'callbacks' | 'stream' | 'traceLLM' | 'traceMCP' | 'verbose' | 'temperature' | 'topP' | 'llmTimeout' | 'toolTimeout' | 'maxRetries' | 'maxTurns' | 'toolResponseMaxBytes' | 'parallelToolCalls' | 'targets'> {
  return {
    config: configuration,
    callbacks: undefined,
    stream: false,
    traceLLM: false,
    traceMCP: false,
    verbose: false,
    temperature: 0.7,
    topP: 1,
    llmTimeout: 10_000,
    toolTimeout: 10_000,
    maxRetries: 2,
    maxTurns: 10,
    toolResponseMaxBytes: 65_536,
    parallelToolCalls: false,
    targets: [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }],
  };
}

function makeBasicConfiguration(): Configuration {
  return {
    providers: {
      [PRIMARY_PROVIDER]: {
        type: 'test-llm',
        models: {
          [MODEL_NAME]: {},
        },
      },
    },
    mcpServers: {},
  };
}

function assertRecord(value: unknown, message: string): asserts value is Record<string, unknown> {
  invariant(value !== null && typeof value === 'object' && !Array.isArray(value), message);
}

function expectRecord(value: unknown, message: string): Record<string, unknown> {
  assertRecord(value, message);
  return value;
}


function isToolAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'tool' } {
  return entry.type === 'tool';
}

function isLlmAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'llm' } {
  return entry.type === 'llm';
}

interface HarnessTest {
  id: string;
  description?: string;
  timeoutMs?: number;
  configure?: (
    configuration: Configuration,
    sessionConfig: AIAgentSessionConfig,
    defaults: NonNullable<Configuration['defaults']>
  ) => void;
  execute?: (
    configuration: Configuration,
    sessionConfig: AIAgentSessionConfig,
    defaults: NonNullable<Configuration['defaults']>
  ) => Promise<AIAgentResult>;
  expect: (result: AIAgentResult) => void;
}

let overrideParentLoaded: LoadedAgent | undefined;
let overrideChildLoaded: LoadedAgent | undefined;
let registrySpawnSessionConfig: AIAgentSessionConfig | undefined;
let registryCoverageSummary: { listCount: number; aliasId?: string; toolName?: string; hasParent: boolean; hasToolAlias: boolean } | undefined;
let utilsCoverageSummary: { sanitized: string; clamped: { name: string; truncated: boolean }; formatted: string; truncated: string } | undefined;
let configLoadSummary: { providerKey?: string; localType?: string; remoteType?: string; localEnvToken?: string; defaultsStream?: boolean } | undefined;
let includeSummary: { resolved: string; forbiddenError?: string; depthError?: string } | undefined;
let configErrorSummary: { readError?: string; jsonError?: string; schemaError?: string; localLoadedKey?: string } | undefined;
let includeAsyncSummary: { staged: string; depth: number } | undefined;
let frontmatterErrorSummary: { strictError?: string; relaxedParsed?: boolean } | undefined;
let frontmatterSummary: { toolName?: string; outputFormat?: string; inputFormat?: string; description?: string; models?: string[] } | undefined;
let humanReadableSummaries: { json: string; text: string; successNoOutput: string; failure: string; truncatedZero: string } | undefined;
let overrideTargetsRef: { provider: string; model: string }[] | undefined;
const overrideLLMExpected = {
  temperature: 0.27,
  topP: 0.52,
  maxOutputTokens: 321,
  repeatPenalty: 1.9,
  llmTimeout: 7_654,
  toolTimeout: 4_321,
  maxRetries: 5,
  maxToolTurns: 11,
  maxToolCallsPerTurn: 4,
  maxConcurrentTools: 2,
  toolResponseMaxBytes: 777,
  mcpInitConcurrency: 6,
  stream: true,
  parallelToolCalls: true,
};

const TEST_SCENARIOS: HarnessTest[] = [
  {
    id: DEFAULT_PROMPT_SCENARIO,
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-1 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-1.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-1.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Tool execution succeeded'), 'Unexpected final report content for run-test-1.');

      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      invariant(firstAssistant !== undefined, 'Missing first assistant message for run-test-1.');
      const toolCallNames = (firstAssistant.toolCalls ?? []).map((call) => call.name);
      const expectedTool = 'test__test';
      const sanitizedExpectedTool = sanitizeToolName(expectedTool);
      invariant(
        toolCallNames.includes(expectedTool) || toolCallNames.includes(sanitizedExpectedTool),
        'Expected test__test tool call in first turn for run-test-1.',
      );

      const toolEntries = result.accounting.filter(isToolAccounting);
      const testEntry = toolEntries.find((entry) => entry.mcpServer === 'test' && entry.command === 'test__test');
      invariant(testEntry !== undefined, 'Expected accounting entry for test MCP server in run-test-1.');
      invariant(testEntry.status === 'ok', 'Test MCP tool accounting should be ok for run-test-1.');
      const llmLogs = result.logs.filter((entry) => entry.type === 'llm' && entry.direction === 'response');
      invariant(llmLogs.length > 0, 'LLM response log missing for run-test-1.');
      const hasStopReason = llmLogs.some((log) => log.message.includes('stop='));
      invariant(hasStopReason, 'LLM log should include stop reason for run-test-1.');
    },
  },
  {
    id: 'run-test-2',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-2 should still conclude the session.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-2.');
      invariant(finalReport.status === 'failure', 'Final report should indicate failure for run-test-2.');

      const toolEntries = result.accounting.filter(isToolAccounting);
      const failureEntry = toolEntries.find((entry) => entry.command === 'test__test');
      invariant(failureEntry !== undefined, 'Expected MCP accounting entry for run-test-2.');
      invariant(failureEntry.status === 'failed', 'Accounting entry must reflect MCP failure for run-test-2.');
    },
  },
  {
    id: 'run-test-3',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-3 expected success after retry.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-3.');
      invariant(finalReport.status === 'success', 'Final report should indicate success for run-test-3.');

      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length >= 2, 'LLM retries expected for run-test-3.');
    },
  },
  {
    id: 'run-test-4',
    configure: (configuration, sessionConfig) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: { type: 'test-llm' },
        [SECONDARY_PROVIDER]: { type: 'test-llm' },
      };
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: MODEL_NAME },
        { provider: SECONDARY_PROVIDER, model: MODEL_NAME },
      ];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-4 expected success.');
      const llmEntries = result.accounting.filter(isLlmAccounting);
      const providersUsed = new Set(llmEntries.map((entry) => entry.provider));
      invariant(providersUsed.has(SECONDARY_PROVIDER), 'Fallback provider should be used in run-test-4.');
    },
  },
  {
    id: 'run-test-5',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxRetries = 1;
      configuration.defaults = defaults;
      sessionConfig.maxRetries = 1;
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-5 should fail due to timeout.');
      invariant(typeof result.error === 'string' && result.error.includes('timeout'), 'Timeout error expected for run-test-5.');
    },
  },
  {
    id: 'run-test-6',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxRetries = 1;
      configuration.defaults = defaults;
      sessionConfig.maxRetries = 1;
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-6 should fail after retries exhausted.');
      const exitLog = result.logs.find((log) => log.remoteIdentifier === 'agent:EXIT-NO-LLM-RESPONSE');
      invariant(exitLog !== undefined, 'Expected EXIT-NO-LLM-RESPONSE log for run-test-6.');
      invariant(typeof exitLog.message === 'string' && exitLog.message.includes('No LLM response after'), 'Exit log should indicate retries exhausted for run-test-6.');
    },
  },
  {
    id: 'run-test-7',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.toolTimeout = 200;
      configuration.defaults = defaults;
      sessionConfig.toolTimeout = 200;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-7 completes with failure report.');
      const toolEntries = result.accounting.filter(isToolAccounting);
      const timeoutEntry = toolEntries.find((entry) => entry.command === 'test__test');
      invariant(timeoutEntry !== undefined, 'Expected MCP accounting entry for run-test-7.');
      invariant(timeoutEntry.status === 'failed', 'Tool timeout should be recorded as failure for run-test-7.');
    },
  },
  {
    id: 'run-test-8',
    configure: (configuration, sessionConfig, defaults) => {
      sessionConfig.toolResponseMaxBytes = 64;
      defaults.toolResponseMaxBytes = 64;
      configuration.defaults = defaults;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-8 expected success.');
      const toolMessages = result.conversation.filter((message) => message.role === 'tool');
      invariant(toolMessages.some((message) => message.content.includes(TRUNCATION_NOTICE)), 'Truncation notice expected in run-test-8.');
    },
  },
  {
    id: 'run-test-9',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-9 should complete with failure report.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should mark failure for run-test-9.');
      const log = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Unknown tool requested'));
      invariant(log !== undefined, 'Unknown tool warning log expected for run-test-9.');
    },
  },
  {
    id: 'run-test-10',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-10 expected success.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      invariant(firstAssistant !== undefined, 'Assistant message missing in run-test-10.');
      const toolCallNames = (firstAssistant.toolCalls ?? []).map((call) => call.name);
      invariant(toolCallNames.includes('agent__progress_report'), 'Progress report tool call expected in run-test-10.');
    },
  },
  {
    id: 'run-test-11',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-11 expected success.');
      const logs = result.logs;
      invariant(logs.some((log) => typeof log.message === 'string' && log.message.includes('agent__final_report(') && log.message.includes('report_format:json')), 'Final report log should capture JSON format attempt in run-test-11.');
    },
  },
  {
    id: 'run-test-12',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxToolTurns = 1;
      configuration.defaults = defaults;
      sessionConfig.maxTurns = 1;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-12 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should complete successfully for run-test-12.');
      const finalTurnLog = result.logs.find((log) => typeof log.remoteIdentifier === 'string' && log.remoteIdentifier.includes('primary:') && typeof log.message === 'string' && log.message.includes('final turn'));
      invariant(finalTurnLog !== undefined, 'LLM request log should reflect final turn for run-test-12.');
    },
  },
  {
    id: 'run-test-13',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-13 expected success.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      const hasBatchCall = firstAssistant?.toolCalls?.some((call) => call.name === 'agent__batch') ?? false;
      invariant(hasBatchCall, 'agent__batch tool call expected in run-test-13.');
      const batchAccounting = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchAccounting?.status === 'ok', 'Batch tool accounting should be ok for run-test-13.');
      const batchResult = result.conversation.find((message) => message.role === 'tool' && message.toolCallId === 'call-batch-success');
      const hasBatchResult = batchResult?.content.includes('batch-success') ?? false;
      invariant(hasBatchResult, 'Batch result should include test payload for run-test-13.');
    },
  },
  {
    id: 'run-test-14',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: false };
      sessionConfig.parallelToolCalls = false;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-14 should conclude with failure report.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should mark failure for run-test-14.');
      const log = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('error agent__batch'));
      invariant(log !== undefined, 'Batch execution failure log expected for run-test-14.');
      const batchAccounting = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchAccounting !== undefined && batchAccounting.status === 'failed', 'Batch accounting must record failure for run-test-14.');
    },
  },
  {
    id: 'run-test-15',
    configure: (configuration) => {
      configuration.pricing = {
        [PRIMARY_PROVIDER]: {
          [MODEL_NAME]: {
            unit: 'per_1k',
            prompt: 0.001,
            completion: 0.002,
          },
        },
      };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-15 expected success.');
      const llmEntry = result.accounting.filter(isLlmAccounting).at(-1);
      invariant(llmEntry !== undefined && typeof llmEntry.costUsd === 'number' && llmEntry.costUsd > 0, 'LLM accounting should include computed cost for run-test-15.');
      const summaryLog = result.logs.find((entry) => entry.remoteIdentifier === 'summary');
      invariant(summaryLog !== undefined && typeof summaryLog.message === 'string' && summaryLog.message.includes('cost total=$') && !summaryLog.message.includes('cost total=$0.00000'), 'Summary log should report non-zero cost for run-test-15.');
    },
  },
  {
    id: 'run-test-16',
    configure: (configuration) => {
      const baseDir = makeTempDir('persistence');
      const sessionsDir = path.join(baseDir, SESSIONS_SUBDIR);
      const billingFile = path.join(baseDir, BILLING_FILENAME);
      configuration.persistence = { sessionsDir, billingFile };
      runTest16Paths = { sessionsDir, billingFile };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-16 expected success.');
      invariant(runTest16Paths !== undefined, 'Persistence paths should be initialized for run-test-16.');
      const { sessionsDir, billingFile } = runTest16Paths;
      try {
        const sessionFiles = fs.existsSync(sessionsDir) ? fs.readdirSync(sessionsDir) : [];
        invariant(sessionFiles.length === 1 && sessionFiles[0].endsWith('.json.gz'), 'Session snapshot should be written for run-test-16.');
        invariant(fs.existsSync(path.join(sessionsDir, sessionFiles[0])), 'Session snapshot file missing for run-test-16.');
        invariant(fs.existsSync(billingFile) && fs.statSync(billingFile).size > 0, 'Billing ledger should be written for run-test-16.');
      } finally {
        fs.rmSync(path.dirname(sessionsDir), { recursive: true, force: true });
        runTest16Paths = undefined;
      }
    },
  },
  {
    id: 'run-test-17',
    configure: (configuration, sessionConfig) => {
      configuration.providers[PRIMARY_PROVIDER] = {
        type: 'test-llm',
        models: {
          [MODEL_NAME]: {
            overrides: {
              temperature: 0.42,
              topP: 0.85,
            },
          },
        },
      };
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-17 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-17.');
    },
  },
  {
    id: 'run-test-18',
    configure: (_configuration, sessionConfig) => {
      const controller = new AbortController();
      controller.abort();
      sessionConfig.abortSignal = controller.signal;
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-18 should cancel the session.');
      invariant(result.error === 'canceled', 'Run-test-18 should report canceled error.');
      const hasLlmLogs = result.logs.some((log) => typeof log.remoteIdentifier === 'string' && log.remoteIdentifier.includes('deterministic-model'));
      invariant(!hasLlmLogs, 'No LLM requests should execute for run-test-18.');
    },
  },
  {
    id: 'run-test-19',
    configure: (configuration, sessionConfig) => {
      configuration.providers[SECONDARY_PROVIDER] = { type: 'test-llm' };
      configuration.pricing = configuration.pricing ?? {};
      sessionConfig.subAgents = [preloadSubAgentFromPath(SUBAGENT_PRICING_PROMPT, configuration)];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-19 expected success.');
      const subAgentAccounting = result.accounting
        .filter(isToolAccounting)
        .find((entry) => entry.command === SUBAGENT_TOOL);
      invariant(subAgentAccounting !== undefined, 'Sub-agent accounting entry expected for run-test-19.');
      invariant(subAgentAccounting.status === 'failed', 'Sub-agent accounting should indicate failure for run-test-19.');
      const subAgentErrorLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes(SUBAGENT_TOOL) && entry.message.includes('error'));
      invariant(subAgentErrorLog !== undefined, 'Sub-agent failure log expected for run-test-19.');
    },
  },
  {
    id: 'run-test-20',
    configure: (configuration) => {
      const baseDir = makeTempDir('persistence-error');
      const blockerFile = path.join(baseDir, 'sessions-blocker');
      fs.writeFileSync(blockerFile, 'block');
      const billingFile = path.join(blockerFile, BILLING_FILENAME);
      configuration.persistence = { sessionsDir: blockerFile, billingFile };
      runTest20Paths = { baseDir, blockerFile, billingFile };
    },
    expect: (result) => {
      try {
        invariant(result.success, 'Scenario run-test-20 expected success.');
        invariant(runTest20Paths !== undefined, 'Persistence paths should be initialized for run-test-20.');
        const billingExists = fs.existsSync(runTest20Paths.billingFile);
        invariant(!billingExists, 'Billing ledger write should fail for run-test-20.');
      } finally {
        if (runTest20Paths !== undefined) {
          fs.rmSync(runTest20Paths.baseDir, { recursive: true, force: true });
          runTest20Paths = undefined;
        }
      }
    },
  },
  {
    id: 'run-test-21',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-21 expected success.');
      const rateLimitLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN));
      invariant(rateLimitLog !== undefined, 'Rate limit warning expected for run-test-21.');
      const retryLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:retry');
      invariant(retryLog !== undefined && typeof retryLog.message === 'string' && retryLog.message.includes('backing off'), 'Backoff log expected for run-test-21.');
    },
  },
  {
    id: 'run-test-22',
    configure: (configuration, sessionConfig) => {
      configuration.providers = {};
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-22 should fail due to provider validation.');
      invariant(typeof result.error === 'string' && result.error.includes('Unknown providers'), 'Provider validation error expected for run-test-22.');
    },
  },
  {
    id: 'run-test-23',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.stopRef = { stopping: true };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-23 expected graceful success.');
      invariant(result.finalReport === undefined, 'No final report expected for run-test-23.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length === 0, 'No assistant messages should be present for run-test-23.');
    },
  },
  {
    id: 'run-test-24',
    configure: (configuration, sessionConfig) => {
      sessionConfig.subAgents = [preloadSubAgentFromPath(SUBAGENT_PRICING_SUCCESS_PROMPT, configuration)];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-24 expected success.');
      const subAgentAccounting = result.accounting.filter(isToolAccounting).find((entry) => entry.command === SUBAGENT_SUCCESS_TOOL);
      invariant(subAgentAccounting !== undefined && subAgentAccounting.status === 'ok', 'Successful sub-agent accounting expected for run-test-24.');
      const subAgentLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:subagent' && typeof entry.message === 'string' && entry.message.includes(SUBAGENT_SUCCESS_TOOL) && !entry.message.includes('error'));
      invariant(subAgentLog !== undefined, 'Sub-agent success log expected for run-test-24.');
    },
  },
  {
    id: 'run-test-120',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-120 should succeed.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-120.');
      const finalContent = finalReport.content ?? '';
      invariant(finalContent.includes('Metadata propagation confirmed'), 'Unexpected final report content for run-test-120.');

      const llmEntry = result.accounting.filter(isLlmAccounting).at(-1);
      invariant(llmEntry !== undefined, 'LLM accounting entry expected for run-test-120.');
      invariant(llmEntry.actualProvider === 'router/fireworks', 'Actual provider should reflect metadata for run-test-120.');
      invariant(llmEntry.actualModel === 'fireworks-test', 'Actual model should reflect metadata for run-test-120.');
      invariant(typeof llmEntry.costUsd === 'number' && Math.abs(llmEntry.costUsd - 0.12345) < 1e-5, 'Reported cost should match metadata for run-test-120.');
      invariant(typeof llmEntry.upstreamInferenceCostUsd === 'number' && Math.abs(llmEntry.upstreamInferenceCostUsd - 0.06789) < 1e-5, 'Upstream cost should match metadata for run-test-120.');
      invariant(llmEntry.tokens.cacheWriteInputTokens === 42, 'Cache write tokens should reflect metadata for run-test-120.');

      const responseLogs = result.logs.filter((entry) => entry.type === 'llm' && entry.direction === 'response');
      const hasMetadataRemoteId = responseLogs.some((entry) => typeof entry.remoteIdentifier === 'string' && entry.remoteIdentifier.includes('router/fireworks'));
      invariant(hasMetadataRemoteId, 'LLM response log should include routed provider segment for run-test-120.');
    },
  },
  {
    id: 'run-test-25',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true, maxConcurrentTools: 1 };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.maxConcurrentTools = 1;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-25 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-25.');

      const toolEntries = result.accounting.filter(isToolAccounting).filter((entry) => entry.command === 'test__test');
      invariant(toolEntries.length === 2, 'Two tool executions expected for run-test-25.');
      const [firstToolEntry, secondToolEntry] = toolEntries;
      invariant(typeof firstToolEntry.latency === 'number' && firstToolEntry.latency >= 1_000, 'First tool latency should reflect timeout payload for run-test-25.');
      invariant(typeof secondToolEntry.latency === 'number' && secondToolEntry.latency < 500, 'Second tool latency should be short for run-test-25.');

      const logs = result.logs;
      const isMcpToolLog = (log: AIAgentResult['logs'][number]): boolean =>
        log.type === 'tool' &&
        typeof log.remoteIdentifier === 'string' &&
        log.remoteIdentifier.startsWith('mcp:');
      const firstRequestIndex = logs.findIndex(
        (log) =>
          isMcpToolLog(log) &&
          log.direction === 'request' &&
          typeof log.message === 'string' &&
          log.message.includes(CONCURRENCY_TIMEOUT_ARGUMENT)
      );
      invariant(firstRequestIndex !== -1, 'First tool request log missing for run-test-25.');
      const firstResponseIndex = logs.findIndex(
        (log, idx) =>
          idx > firstRequestIndex &&
          isMcpToolLog(log) &&
          log.direction === 'response' &&
          typeof log.message === 'string' &&
          log.message.startsWith('ok test__test')
      );
      invariant(firstResponseIndex !== -1, 'First tool response log missing for run-test-25.');
      const secondRequestIndex = logs.findIndex(
        (log, idx) =>
          idx > firstRequestIndex &&
          isMcpToolLog(log) &&
          log.direction === 'request' &&
          typeof log.message === 'string' &&
          log.message.includes(CONCURRENCY_SECOND_ARGUMENT)
      );
      invariant(secondRequestIndex !== -1, 'Second tool request log missing for run-test-25.');
      invariant(secondRequestIndex > firstResponseIndex, 'Second tool request should occur after first tool response for run-test-25.');
    },
  },
  {
    id: 'run-test-26',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-26 expected session success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should indicate failure for run-test-26.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-invalid-id'
      );
      const invalidContent = batchMessage?.content ?? '';
      invariant(invalidContent.includes('invalid_batch_input'), 'Batch tool message should include invalid_batch_input for run-test-26.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry !== undefined && batchEntry.status === 'failed' && typeof batchEntry.error === 'string' && batchEntry.error.startsWith('invalid_batch_input'), 'Batch accounting should record invalid input failure for run-test-26.');
    },
  },
  {
    id: 'run-test-27',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-27 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-27.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-unknown-tool'
      );
      const unknownContent = batchMessage?.content ?? '';
      invariant(unknownContent.includes('UNKNOWN_TOOL'), 'Batch tool message should include UNKNOWN_TOOL for run-test-27.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry !== undefined && batchEntry.status === 'ok', 'Batch accounting should record success for run-test-27.');
    },
  },
  {
    id: 'run-test-28',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-28 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should indicate failure for run-test-28.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-exec-error'
      );
      const errorContent = batchMessage?.content ?? '';
      invariant(errorContent.includes('EXECUTION_ERROR'), 'Batch tool message should include EXECUTION_ERROR for run-test-28.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry !== undefined && batchEntry.status === 'ok', 'Batch accounting should remain ok for run-test-28.');
    },
  },
  {
    id: 'run-test-29',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxRetries = 1;
      configuration.defaults = defaults;
      sessionConfig.maxRetries = 1;
    },
    execute: async (_configuration, sessionConfig) => {
      const initialSession = AIAgentSession.create(sessionConfig);
      const firstResult = await initialSession.run();
      if (firstResult.success) {
        return firstResult;
      }
      const retrySession = initialSession.retry();
      const secondResult = await retrySession.run();
      const augmented = secondResult as AIAgentResult & { _firstAttempt?: AIAgentResult };
      augmented._firstAttempt = firstResult;
      return augmented;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-29 expected success after retry.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-29.');
      const augmented = result as AIAgentResult & { _firstAttempt?: AIAgentResult };
      const firstAttempt = augmented._firstAttempt;
      invariant(firstAttempt !== undefined && !firstAttempt.success, 'First attempt should fail before retry for run-test-29.');
      invariant(typeof firstAttempt.error === 'string' && firstAttempt.error.includes('Simulated fatal error before manual retry.'), 'First attempt error message mismatch for run-test-29.');
      const successLog = result.logs.find((entry) => entry.type === 'tool' && entry.direction === 'response' && typeof entry.message === 'string' && entry.message.startsWith('ok test__test'));
      invariant(successLog !== undefined, 'Successful tool execution log expected after retry for run-test-29.');
    },
  },
  {
    id: 'run-test-30',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.stream = true;
      configuration.defaults = defaults;
      sessionConfig.stream = true;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-30 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-30.');
      const thinkingLog = result.logs.find((entry) => entry.severity === 'THK' && entry.remoteIdentifier === 'thinking');
      invariant(thinkingLog !== undefined, 'Thinking log expected for run-test-30.');
    },
  },
  {
    id: 'run-test-31',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-31 expected success after provider throw.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-31.');
      const thrownLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes(THROW_FAILURE_MESSAGE));
      invariant(thrownLog !== undefined, 'Thrown failure log expected for run-test-31.');
      const failedAttempt = result.accounting.filter(isLlmAccounting).find((entry) => entry.status === 'failed');
      invariant(failedAttempt !== undefined, 'LLM accounting failure expected for run-test-31.');
    },
  },
  {
    id: 'run-test-32',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = 'json';
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-32 expected success after retry.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success' && finalReport.format === 'json', 'Final report should indicate JSON success for run-test-32.');
      const toolFailureMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('final_report(json) requires'));
      invariant(toolFailureMessage !== undefined, 'Final report failure message expected for run-test-32.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts >= 2, 'Retry attempt expected for run-test-32.');
    },
  },
  {
    id: 'run-test-33',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.maxTurns = 1;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-33 should complete with a synthesized failure final report.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Synthesized failure final report expected for run-test-33.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('did not produce a final report'), 'Synthesized content should mention missing final report for run-test-33.');
      const syntheticLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Synthetic retry: assistant returned content without tool calls and without final_report.'));
      invariant(syntheticLog !== undefined, 'Synthetic retry warning expected for run-test-33.');
      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:final-turn');
      invariant(finalTurnLog !== undefined, 'Final-turn retry warning expected for run-test-33.');
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined, 'Synthesized EXIT-FINAL-ANSWER log expected for run-test-33.');
    },
  },
  {
    id: 'run-test-34',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-34 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-34.');
      const progressResult = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('agent__progress_report'));
      invariant(progressResult !== undefined, 'Batch progress entry expected for run-test-34.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(BATCH_PROGRESS_RESPONSE));
      invariant(toolMessage !== undefined, 'Batch tool output expected for run-test-34.');
    },
  },
  {
    id: 'run-test-35',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-35 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-35.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(BATCH_STRING_RESULT));
      invariant(toolMessage !== undefined, 'Batch string tool output expected for run-test-35.');
    },
  },
  {
    id: 'run-test-36',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-36 expected session completion.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should indicate failure for run-test-36.');
      const failureMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('empty_batch'));
      invariant(failureMessage !== undefined, 'Empty batch failure message expected for run-test-36.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry !== undefined && batchEntry.status === 'failed' && typeof batchEntry.error === 'string' && batchEntry.error.startsWith('empty_batch'), 'Batch accounting should record empty batch failure for run-test-36.');
    },
  },
  {
    id: 'run-test-37',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.maxRetries = 2;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-37 expected success after rate limit retry.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-37.');
      const rateLimitLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN));
      invariant(rateLimitLog !== undefined, 'Rate limit warning expected for run-test-37.');
      const retryLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:retry');
      invariant(retryLog !== undefined, 'Retry backoff log expected for run-test-37.');
    },
  },
  {
    id: 'run-test-45',
    configure: (configuration, sessionConfig) => {
      configuration.providers[PRIMARY_PROVIDER] = {
        type: 'test-llm',
        baseUrl: TRACEABLE_PROVIDER_URL,
      };
      configuration.pricing = {
        [PRIMARY_PROVIDER]: {
          [MODEL_NAME]: { unit: 'per_1k', prompt: 0.001, completion: 0.002 },
        },
      };
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.traceLLM = true;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-45 expected success with traced fetch.');
      const summaryCostLog = result.logs.find((entry) => entry.severity === 'FIN' && entry.remoteIdentifier === 'summary' && typeof entry.message === 'string' && entry.message.includes('cost total=$'));
      invariant(summaryCostLog !== undefined, 'Pricing summary log expected for run-test-45.');
    },
  },
  {
    id: 'run-test-46',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxConcurrentTools = 1;
      configuration.defaults = defaults;
      sessionConfig.maxConcurrentTools = 1;
      sessionConfig.tools = ['test'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-46 expected success.');
      const summaryLog = result.logs.find((entry) => entry.remoteIdentifier === 'summary' && entry.type === 'llm');
      invariant(summaryLog !== undefined, 'LLM summary log expected for run-test-46.');
    },
  },
  {
    id: 'run-test-38',
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-38 should fail on auth error.');
      invariant(typeof result.error === 'string' && result.error.includes('Authentication failed'), 'Auth failure error expected for run-test-38.');
    },
  },
  {
    id: 'run-test-39',
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-39 should fail on quota error.');
      invariant(typeof result.error === 'string' && result.error.toLowerCase().includes('quota'), 'Quota failure error expected for run-test-39.');
    },
  },
  {
    id: 'run-test-40',
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-40 should fail on timeout error.');
      invariant(typeof result.error === 'string' && result.error.toLowerCase().includes('timeout'), 'Timeout error expected for run-test-40.');
    },
  },
  {
    id: 'run-test-41',
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-41 should fail on network error.');
      invariant(typeof result.error === 'string' && result.error.toLowerCase().includes('network'), 'Network error expected for run-test-41.');
    },
  },
  {
    id: 'run-test-42',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.maxRetries = 2;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-42 expected success after model error retry.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-42.');
      const failedAttempt = result.accounting.filter(isLlmAccounting).find((entry) => entry.status === 'failed');
      invariant(failedAttempt !== undefined && typeof failedAttempt.error === 'string' && failedAttempt.error.includes('Invalid model request'), 'Model error accounting expected for run-test-42.');
    },
  },
  {
    id: 'run-test-47',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = 'json';
      sessionConfig.expectedOutput = {
        format: 'json',
        schema: {
          type: 'object',
          required: ['status'],
          properties: {
            status: { enum: ['success'] },
          },
        },
      };
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-47 should fail due to schema validation.');
      const schemaLog = result.logs.find((entry) => entry.severity === 'ERR' && typeof entry.message === 'string' && entry.message.includes('schema validation failed'));
      invariant(schemaLog !== undefined, 'Schema validation log expected for run-test-47.');
    },
  },
  {
    id: 'run-test-48',
    configure: (configuration, sessionConfig) => {
      configuration.providers[PRIMARY_PROVIDER] = { type: 'test-llm' };
      sessionConfig.traceMCP = true;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-48 expected success with trace logs.');
      const traceLog = result.logs.find((entry) => entry.severity === 'TRC' && typeof entry.message === 'string' && entry.message.includes('REQUEST test__test'));
      invariant(traceLog !== undefined, 'Trace log expected for run-test-48.');
    },
  },
  {
    id: 'run-test-50',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = SLACK_OUTPUT_FORMAT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-50 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-50.');
      const metadataCandidate = finalReport.metadata;
      const slackCandidate = (metadataCandidate !== undefined && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate))
        ? (metadataCandidate as { slack?: unknown }).slack
        : undefined;
      const slackMeta = (slackCandidate !== undefined && typeof slackCandidate === 'object' && !Array.isArray(slackCandidate))
        ? (slackCandidate as { messages?: unknown[] })
        : undefined;
      const messagesValue = slackMeta !== undefined ? slackMeta.messages : undefined;
      const messages = Array.isArray(messagesValue) ? messagesValue : [];
      invariant(messages.length > 0, 'Normalized Slack messages expected for run-test-50.');
      const first = messages.at(0);
      invariant(first !== undefined && typeof first === 'object', 'First Slack message should be an object for run-test-50.');
    },
  },
  {
    id: 'run-test-51',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = SLACK_OUTPUT_FORMAT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-51 should complete the session.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should indicate failure for run-test-51.');
      const errorLog = result.logs.find((entry) => entry.severity === 'ERR' && typeof entry.message === 'string' && entry.message.includes('requires `messages` or non-empty `content`'));
      invariant(errorLog !== undefined, 'Slack content error log expected for run-test-51.');
    },
  },
  {
    id: 'run-test-52',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = SLACK_OUTPUT_FORMAT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-52 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-52.');
      const metadataCandidate = finalReport.metadata;
      const slackCandidate = (metadataCandidate !== undefined && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate))
        ? (metadataCandidate as { slack?: unknown }).slack
        : undefined;
      const slackMeta = (slackCandidate !== undefined && typeof slackCandidate === 'object' && !Array.isArray(slackCandidate))
        ? (slackCandidate as { messages?: unknown[]; footer?: unknown })
        : undefined;
      const messagesValue = slackMeta !== undefined ? slackMeta.messages : undefined;
      const messages = Array.isArray(messagesValue) ? messagesValue : [];
      invariant(messages.length >= 2, 'Multiple Slack messages expected for run-test-52.');
    },
  },
  {
    id: 'run-test-53',
    description: 'GitHub search normalization coverage.',
    configure: (configuration, sessionConfig) => {
      configuration.mcpServers.github = {
        type: 'stdio',
        command: process.execPath,
        args: [GITHUB_SERVER_PATH],
      };
      sessionConfig.tools = ['test', 'github'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-53 expected success.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && message.toolCallId === 'call-github-search');
      invariant(toolMessage !== undefined && typeof toolMessage.content === 'string', 'GitHub tool response expected for run-test-53.');
      const toolPayload = toolMessage.content;
      let parsed: { q?: unknown; language?: unknown; repo?: unknown; path?: unknown } = {};
      try {
        parsed = JSON.parse(toolPayload) as { q?: unknown; language?: unknown; repo?: unknown; path?: unknown };
      } catch (error: unknown) {
        throw new Error(`Failed to parse GitHub tool response: ${toErrorMessage(error)}`);
      }
      const q = typeof parsed.q === 'string' ? parsed.q : '';
      invariant(q.includes('md5 helper'), 'Normalized query should include base term for run-test-53.');
      invariant(q.includes('repo:owner/project'), 'Normalized query should include repo qualifier for run-test-53.');
      invariant(q.includes('path:src'), 'Normalized query should include path qualifier for run-test-53.');
      invariant(q.includes('language:javascript'), 'Normalized query should include javascript language qualifier for run-test-53.');
      invariant(q.includes('language:python'), 'Normalized query should include python language qualifier for run-test-53.');
      invariant(q.includes('extension:jsx'), 'Normalized query should include jsx extension qualifier for run-test-53.');
      invariant(!q.includes('language:js'), 'Normalized query should not include raw js language qualifier for run-test-53.');
    },
  },
  {
    id: 'run-test-54',
    description: 'WebSocket transport round-trip coverage.',
    execute: async () => {
      runTest54State = undefined;
      const server = new WebSocketServer({ port: 0 });
      const address = server.address();
      if (address === null || typeof address === 'string') {
        await new Promise<void>((resolve) => { server.close(() => { resolve(); }); });
        throw new Error('Unable to determine WebSocket server port for run-test-54.');
      }
      const netAddress = address as AddressInfo;
      if (typeof netAddress.port !== 'number') {
        await new Promise<void>((resolve) => { server.close(() => { resolve(); }); });
        throw new Error('Unable to determine WebSocket server port for run-test-54.');
      }
      const port = netAddress.port;
      const serverPayloads: string[] = [];
      server.on('connection', (socket) => {
        setTimeout(() => {
          socket.send(JSON.stringify({ jsonrpc: '2.0', method: 'serverHello' }));
        }, 5);
        socket.on('message', (data) => {
          let text = '';
          if (typeof data === 'string') {
            text = data;
          } else if (Array.isArray(data)) {
            text = Buffer.concat(data).toString('utf8');
          } else if (data instanceof Buffer) {
            text = data.toString('utf8');
          }
          if (text.length > 0) {
            serverPayloads.push(text);
            socket.send(JSON.stringify({ jsonrpc: '2.0', method: 'ack', params: { payload: text } }));
          }
        });
      });
      const received: JSONRPCMessage[] = [];
      const errors: string[] = [];
      let transportClosed = false;
      let transport: Awaited<ReturnType<typeof createWebSocketTransport>> | undefined;
      try {
        transport = await createWebSocketTransport(`ws://127.0.0.1:${port.toString()}`);
        let helloResolve: (() => void) | undefined;
        const helloPromise = new Promise<void>((resolve) => { helloResolve = resolve; });
        transport.onmessage = (message) => {
          received.push(message);
          if (helloResolve !== undefined) {
            helloResolve();
            helloResolve = undefined;
          }
        };
        transport.onerror = (error) => {
          errors.push(error.message);
        };
        await transport.send({ jsonrpc: '2.0', method: 'clientPing', id: 'ping-1' });
        try {
          await Promise.race([
            helloPromise,
            new Promise<void>((_resolve, reject) => {
              setTimeout(() => { reject(new Error('serverHello not received within 1000ms')); }, 1000);
            }),
          ]);
        } catch (error: unknown) {
          errors.push(toErrorMessage(error));
          throw error;
        }
        await transport.close();
        transportClosed = true;
          return {
            success: true,
            conversation: [],
            logs: [],
            accounting: [],
          };
      } finally {
        if (transport !== undefined && !transportClosed) {
          try { await transport.close(); } catch { /* ignore */ }
        }
        await new Promise<void>((resolve) => { server.close(() => { resolve(); }); });
        runTest54State = {
          received: [...received],
          serverPayloads: [...serverPayloads],
          errors: [...errors],
        };
      }
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-54 should succeed.');
      invariant(runTest54State !== undefined, 'WebSocket state should be captured for run-test-54.');
      const { received, serverPayloads, errors } = runTest54State;
      runTest54State = undefined;
      invariant(errors.length === 0, 'No WebSocket errors expected for run-test-54.');
      const serverHello = received.find((message) => (message as { method?: unknown }).method === 'serverHello');
      const ackMessage = received.find((message) => (message as { method?: unknown }).method === 'ack');
      invariant(serverHello !== undefined || ackMessage !== undefined, 'WebSocket response expected for run-test-54.');
      const clientPayload = serverPayloads.find((payload) => payload.includes('clientPing'));
      invariant(clientPayload !== undefined, 'Client ping payload expected for run-test-54.');
    },
  },
  {
    id: 'run-test-55',
    description: 'LLM traced fetch logging and error handling.',
    timeoutMs: 5000,
    execute: async () => {
      const logs: LogEntry[] = [];
      const fetchCalls: { url: string; method: string }[] = [];
      const finalData: { afterJson?: { provider?: string; model?: string }; afterSse?: { provider?: string; model?: string }; costs?: { costUsd?: number; upstreamInferenceCostUsd?: number }; fetchError?: string } = {};
      const OPENROUTER_DEBUG_SSE_URL = 'https://openrouter.ai/api/v1/debug-sse';
      const OPENROUTER_BINARY_URL = 'https://openrouter.ai/api/v1/binary';
      const CONTENT_TYPE_OCTET = 'application/octet-stream';
      const pricing = {
        fireworks: {
          'gpt-fireworks': { unit: 'per_1k', prompt: 0.002, completion: 0.003 },
        },
        openrouter: {
          'gpt-mock': { unit: 'per_1k', prompt: 0.001, completion: 0.002, cacheRead: 0.0004, cacheWrite: 0.0005 },
        },
      } satisfies Partial<Record<string, Partial<Record<string, { unit?: 'per_1k' | 'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
      const client = new LLMClient(
        {
          openrouter: { type: 'openrouter' },
        },
        {
          traceLLM: true,
          onLog: (entry) => { logs.push(entry); },
          pricing,
        }
      );
      client.setTurn(1, 0);
      const plans: ({ type: 'success'; status: number; headers: Record<string, string>; body: string; cloneFails?: boolean } | { type: 'error'; error: Error })[] = [
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_JSON },
          body: JSON.stringify({
            provider: 'fireworks',
            model: 'gpt-fireworks',
            usage: { cost: 0.01234, cost_details: { upstream_inference_cost: 0.00456 } },
          }),
        },
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_EVENT_STREAM },
          body: 'data: {"provider":"mistral","model":"mixtral","usage":{"cost":0.015}}\n\ndata: {"usage":{"cost_details":{"upstream_inference_cost":0.006}}}\n\n',
        },
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_JSON },
          body: JSON.stringify({
            usage: { cache_creation: { ephemeral_5m_input_tokens: 42 } },
          }),
        },
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_EVENT_STREAM, authorization: 'Bearer SECRET12345', 'x-api-key': 'super-secret-key' },
          body: 'data: {"partial":true}\n\n',
          cloneFails: true,
        },
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_OCTET, authorization: 'Bearer ABC', 'x-goog-api-key': 'api-key-123' },
          body: '',
        },
        {
          type: 'success',
          status: 200,
          headers: { 'content-type': CONTENT_TYPE_JSON, authorization: 'Bearer SECRET1234567890', 'x-api-key': 'micro-key' },
          body: JSON.stringify({ hello: 'world' }),
          cloneFails: true,
        },
        {
          type: 'error',
          error: new Error('network down'),
        },
      ];
      const originalFetch = globalThis.fetch;
      globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const plan = plans.shift();
        if (plan === undefined) {
          return Promise.reject(new Error('Unexpected fetch call'));
        }
        const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
        const method = init?.method ?? 'GET';
        fetchCalls.push({ url, method });
        if (plan.type === 'success') {
          const headers = new Headers(plan.headers);
          const response = new Response(plan.body, { status: plan.status, headers });
          if (plan.cloneFails === true) {
            response.clone = () => { throw new Error('clone failed'); };
          }
          return Promise.resolve(response);
        }
        return Promise.reject(plan.error);
      };
      const request: TurnRequest = {
        provider: 'openrouter',
        model: 'gpt-mock',
        messages: [{ role: 'user', content: 'final' }],
        tools: [],
        toolExecutor: () => Promise.resolve(''),
        isFinalTurn: true,
        temperature: 0.5,
        topP: 0.8,
        maxOutputTokens: 150,
        stream: false,
        llmTimeout: 10_000,
      };

      const requestBody = JSON.stringify({ messages: [{ role: 'user', content: 'Hello' }] });

      let fetchError: Error | undefined;
      try {
        (client as unknown as { logRequest: (req: TurnRequest) => void }).logRequest(request);
        const tracedFetch = (client as unknown as { createTracedFetch: () => typeof fetch }).createTracedFetch();
        await tracedFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer TESTTOKEN', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await client.waitForMetadataCapture();
        const afterJsonRouting = client.getLastActualRouting();
        if (afterJsonRouting !== undefined) {
          finalData.afterJson = afterJsonRouting;
        } else {
          finalData.afterJson ??= {};
        }
        const costAfterJson = client.getLastCostInfo();
        if (costAfterJson !== undefined) {
          finalData.costs = costAfterJson;
        } else {
          finalData.costs ??= {};
        }
        await tracedFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer SSE', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await client.waitForMetadataCapture();
        const afterSseRouting = client.getLastActualRouting();
        if (afterSseRouting !== undefined) {
          finalData.afterSse = afterSseRouting;
        } else {
          finalData.afterSse ??= {};
        }
        const costAfterSse = client.getLastCostInfo();
        if (costAfterSse !== undefined) {
          finalData.costs = costAfterSse;
        }
        await tracedFetch('https://anthropic.mock/v1/messages', {
          method: 'POST',
          headers: { 'Content-Type': CONTENT_TYPE_JSON },
          body: JSON.stringify({ prompt: 'cache' }),
        });
        await client.waitForMetadataCapture();
        const coverageClient = new LLMClient(
          { openrouter: { type: 'openrouter' } },
          { traceLLM: true, onLog: (entry) => { logs.push(entry); } }
        );
        coverageClient.setTurn(1, 1);
        const coverageFetch = (coverageClient as unknown as { createTracedFetch: () => typeof fetch }).createTracedFetch();
        await coverageFetch(OPENROUTER_DEBUG_SSE_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer SSEFAIL', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await coverageClient.waitForMetadataCapture();
        await coverageFetch(OPENROUTER_BINARY_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer BINARY', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await coverageClient.waitForMetadataCapture();
        await coverageFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer JSONFAIL', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await coverageClient.waitForMetadataCapture();
        try {
          await tracedFetch(OPENROUTER_RESPONSES_URL, {
            method: 'POST',
            headers: { Authorization: 'Bearer FAIL', 'Content-Type': CONTENT_TYPE_JSON },
            body: requestBody,
          });
        } catch (error: unknown) {
          fetchError = toError(error);
        }
        await client.waitForMetadataCapture();


        const successResult: TurnResult = {
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          response: 'ack',
          tokens: { inputTokens: 200, outputTokens: 50, cacheReadInputTokens: 0, cacheWriteInputTokens: 42, totalTokens: 250 },
          latencyMs: 64,
          messages: [],
        };
        successResult.providerMetadata = {
          actualProvider: 'mistral',
          actualModel: 'mixtral',
          reportedCostUsd: 0.015,
          upstreamCostUsd: 0.006,
        };
        client.setTurn(2, 0);
        (client as unknown as { logResponse: (req: TurnRequest, res: TurnResult, latencyMs: number) => void }).logResponse(request, successResult, 64);
        const routingAfterSuccess = client.getLastActualRouting();
        if (routingAfterSuccess !== undefined) {
          finalData.afterSse = routingAfterSuccess;
        }
        const costsAfterSuccess = client.getLastCostInfo();
        if (costsAfterSuccess !== undefined) {
          finalData.costs = costsAfterSuccess;
        }
        client.setTurn(2, 1);
        const errorResult: TurnResult = {
          status: { type: 'auth_error', message: 'bad credentials' },
          latencyMs: 10,
          messages: [],
        };
        (client as unknown as { logResponse: (req: TurnRequest, res: TurnResult, latencyMs: number) => void }).logResponse(request, errorResult, 10);
        const quotaResult: TurnResult = {
          status: { type: 'quota_exceeded', message: 'quota hit' },
          latencyMs: 12,
          messages: [],
        };
        client.setTurn(2, 2);
        (client as unknown as { logResponse: (req: TurnRequest, res: TurnResult, latencyMs: number) => void }).logResponse(request, quotaResult, 12);
      } finally {
        globalThis.fetch = originalFetch;
      }
      return {
        success: true,
        conversation: [],
        logs,
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'json',
          content_json: {
            fetchCalls,
            routingAfterJson: finalData.afterJson,
            routingAfterSse: finalData.afterSse,
            costSnapshot: finalData.costs,
            fetchError: fetchError?.message ?? null,
          },
          ts: Date.now(),
        },
      };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-55 should succeed.');
      const trcRequestLog = result.logs.find((entry) => entry.severity === 'TRC' && entry.message.includes('LLM request'));
      invariant(trcRequestLog !== undefined, 'TRC request log expected for run-test-55.');
      const trcResponseLog = result.logs.find((entry) => entry.severity === 'TRC' && entry.message.includes('LLM response') && (entry.message.includes('raw-sse') || entry.message.includes('text/event-stream')));
      if (trcResponseLog === undefined) {
        // eslint-disable-next-line no-console
        console.error(JSON.stringify(result.logs, null, 2));
        invariant(false, 'TRC response log with SSE expected for run-test-55.');
      }
      const errorTrace = result.logs.find((entry) => entry.severity === 'TRC' && entry.message.includes('HTTP Error: network down'));
      invariant(errorTrace !== undefined, 'HTTP error trace expected for run-test-55.');
      const successLog = result.logs.find((entry) => entry.severity === 'VRB' && entry.message.includes('cacheW 42'));
      if (successLog === undefined) {
        // eslint-disable-next-line no-console
        console.error(JSON.stringify(result.logs, null, 2));
      }
      invariant(successLog !== undefined && successLog.message.includes('cost $') && successLog.message.includes('upstream'), 'Cost log expected for run-test-55.');
      const failureLog = result.logs.find((entry) => entry.severity === 'ERR' && entry.message.includes('AUTH_ERROR'));
      invariant(failureLog !== undefined, 'Failure log expected for run-test-55.');
      const report = result.finalReport?.content_json;
      assertRecord(report, 'Final data snapshot expected for run-test-55.');
      const fetchErrorMsg = typeof report.fetchError === 'string' ? report.fetchError : undefined;
      invariant(fetchErrorMsg === 'network down', 'Fetch error message expected for run-test-55.');
      const routingAfterJson = report.routingAfterJson;
      assertRecord(routingAfterJson, 'JSON routing metadata expected for run-test-55.');
      if (routingAfterJson.provider === undefined || routingAfterJson.model === undefined) {
        // eslint-disable-next-line no-console
        console.error('debug run-test-55 routingAfterJson', routingAfterJson, report);
      }
      invariant(routingAfterJson.provider === 'fireworks', 'JSON routing provider expected for run-test-55.');
      invariant(routingAfterJson.model === 'gpt-fireworks', 'JSON routing model expected for run-test-55.');
      const routingAfterSse = report.routingAfterSse;
      assertRecord(routingAfterSse, 'Routing metadata expected for run-test-55.');
      invariant(routingAfterSse.provider === 'mistral', 'Routing metadata expected for run-test-55.');
      invariant(routingAfterSse.model === 'mixtral', 'SSE routing model expected for run-test-55.');
      const costSnapshot = report.costSnapshot;
      assertRecord(costSnapshot, 'Cost metadata expected for run-test-55.');
      invariant(costSnapshot.costUsd === 0.015, 'Final cost expected for run-test-55.');
      invariant(costSnapshot.upstreamInferenceCostUsd === 0.006, 'Upstream inference cost expected for run-test-55.');
    },
  },
  {
    id: 'run-test-56',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.tools = ['test'];
      sessionConfig.maxRetries = 4;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-56 should succeed after retries.');
      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length >= 3, 'LLM accounting entries expected for run-test-56.');
      const failedEntries = llmEntries.filter((entry) => entry.status === 'failed');
      invariant(failedEntries.length >= 2, 'Multiple LLM failures expected for run-test-56.');
      const modelFailure = failedEntries.find((entry) => typeof entry.error === 'string' && entry.error.includes('Model failure during first attempt.'));
      invariant(modelFailure !== undefined, 'Model error accounting entry expected for run-test-56.');
      const rateLog = result.logs.find((entry) => entry.type === 'llm' && typeof entry.message === 'string' && entry.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN));
      invariant(rateLog !== undefined, 'Rate limit warning log expected for run-test-56.');
    },
  },
  (() => {
    let progressMessages: string[] = [];
    return {
      id: 'run-test-57',
      configure: (_configuration, sessionConfig) => {
        progressMessages = [];
        sessionConfig.outputFormat = SLACK_OUTPUT_FORMAT;
        sessionConfig.initialTitle = 'Initial Title';
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onProgress: (event) => {
            if (event.type === 'agent_update' && typeof event.message === 'string') {
              progressMessages.push(event.message);
            }
            existingCallbacks.onProgress?.(event);
          },
        };
      },
      expect: (result) => {
        invariant(result.success, 'Scenario run-test-57 should complete successfully.');
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-57.');
        const metadataCandidate = finalReport.metadata;
        const slackCandidate = metadataCandidate !== undefined && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate)
          ? (metadataCandidate as { slack?: unknown }).slack
          : undefined;
        const slackMeta = slackCandidate !== undefined && typeof slackCandidate === 'object' && !Array.isArray(slackCandidate)
          ? (slackCandidate as { messages?: unknown[] })
          : undefined;
        const messagesValue = slackMeta !== undefined ? slackMeta.messages : undefined;
        const messages = Array.isArray(messagesValue) ? messagesValue : [];
        invariant(messages.length >= 2, 'Normalized Slack messages expected for run-test-57.');
        invariant(progressMessages.some((message) => message.includes('Analyzing deterministic harness outputs.')), 'Progress update should be forwarded for run-test-57.');
      },
    };
  })(),
  {
    id: 'run-test-75',
    description: 'Flatten prompts, expand OpenAPI specs, and resolve env placeholders at load-time.',
    execute: (): Promise<AIAgentResult> => {
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}preload-`));
      const cleanup = () => {
        try { fs.rmSync(tempDir, { recursive: true, force: true }); } catch { /* ignore cleanup errors */ }
      };
      try {
        const partialDir = path.join(tempDir, 'partials');
        const specsDir = path.join(tempDir, 'specs');
        fs.mkdirSync(partialDir, { recursive: true });
        fs.mkdirSync(specsDir, { recursive: true });

        const nestedIncludeMarker = 'Nested include content.';
        const nestedIncludePath = path.join(partialDir, 'nested.md');
        fs.writeFileSync(nestedIncludePath, nestedIncludeMarker);
        const mainIncludePath = path.join(partialDir, 'main.md');
        fs.writeFileSync(mainIncludePath, ['Part1 include content.', '${include:nested.md}', ''].join('\n'));

        const subAgentPath = path.join(tempDir, 'sub-agent.ai');
        fs.writeFileSync(subAgentPath, [
          '---',
          'description: Sub-agent with includes',
          'models:',
          `  - primary/${MODEL_NAME}`,
          '---',
          'Sub-agent body start.',
          '${include:partials/nested.md}',
          '',
        ].join('\n'));

        const specPath = path.join(specsDir, 'catalog.json');
        const openApiSpec = {
          openapi: '3.0.0',
          info: { title: 'Catalog', version: '1.0.0' },
          servers: [{ url: 'https://example.com' }],
          paths: {
            '/items': {
              get: {
                operationId: 'listItems',
                summary: 'List items',
                responses: { '200': { description: 'OK' } },
              },
            },
          },
        };
        fs.writeFileSync(specPath, JSON.stringify(openApiSpec, null, 2));

        const mainPromptPath = path.join(tempDir, 'preload-main.ai');
        fs.writeFileSync(mainPromptPath, [
          '---',
          'description: Preload flatten agent',
          'models:',
          `  - primary/${MODEL_NAME}`,
          'tools:',
          '  - openapi:catalog',
          '  - test',
          'agents:',
          '  - sub-agent.ai',
          '---',
          'Main body start.',
          '${include:partials/main.md}',
          '',
        ].join('\n'));

        const configPath = path.join(tempDir, CONFIG_FILENAME);
        const config = {
          providers: {
            primary: {
              type: 'test-llm',
              apiKey: '${TEST_API_KEY}',
            },
          },
          mcpServers: {
            test: {
              type: 'stdio',
              command: '${MCP_COMMAND}',
              args: ['${MCP_ARG}'],
            },
          },
          openapiSpecs: {
            catalog: {
              spec: './specs/catalog.json',
            },
          },
          defaults: {
            temperature: 0.25,
          },
        };
        fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
        const envPath = path.join(tempDir, '.ai-agent.env');
        fs.writeFileSync(envPath, [
          'TEST_API_KEY=env-secret-key',
          'MCP_COMMAND=env-command',
          'MCP_ARG=env-arg',
          '',
        ].join('\n'));

        const loaded = loadAgent(mainPromptPath, undefined, { configPath });
        invariant(!loaded.systemTemplate.includes(INCLUDE_DIRECTIVE_TOKEN), 'System prompt should not contain include directives post-flatten.');
        invariant(loaded.systemTemplate.includes('Main body start.'), 'System prompt missing main body content.');
        invariant(loaded.systemTemplate.includes('Part1 include content.'), 'System prompt missing included content.');
        invariant(loaded.systemTemplate.includes(nestedIncludeMarker), 'System prompt missing nested include content.');

        const toolSet = new Set(loaded.tools);
        invariant(toolSet.has('test'), 'Expected MCP test tool to remain after preload.');
        invariant(toolSet.has('catalog_listItems'), 'OpenAPI-generated tool should be injected after preload.');
        invariant(!Array.from(toolSet.values()).some((tool) => tool.startsWith('openapi:')), 'openapi:* selectors must be removed after expansion.');

        const restTools = loaded.config.restTools ?? {};
        invariant(Object.prototype.hasOwnProperty.call(restTools, 'catalog_listItems'), 'Expanded REST tool catalog_listItems missing from configuration.');

        const providerConfig = loaded.config.providers.primary as { apiKey?: string };
        invariant(providerConfig.apiKey === 'env-secret-key', 'Provider API key should respect env substitution.');

        const mcpConfig = loaded.config.mcpServers.test;
        invariant(mcpConfig.command === 'env-command', 'MCP command should respect env substitution.');
        const mcpArgs = Array.isArray(mcpConfig.args) ? mcpConfig.args : [];
        invariant(mcpArgs.includes('env-arg'), 'MCP args should include substituted env argument.');

        invariant(loaded.subAgents.some((agent) => agent.promptPath.endsWith('sub-agent.ai')), 'Sub-agent should be registered after load.');

        const registry = new SubAgentRegistry(
          loaded.subAgents,
          []
        );
        const observedReads: string[] = [];
        const originalReadFileSync = fs.readFileSync;
        try {
          fs.readFileSync = ((...args: Parameters<typeof fs.readFileSync>) => {
            const [target] = args;
            if (typeof target === 'string') observedReads.push(target);
            return originalReadFileSync.apply(fs, args);
          }) as typeof fs.readFileSync;
          // Registry constructed with preloaded data; no runtime loading expected.
        } finally {
          fs.readFileSync = originalReadFileSync;
        }
        invariant(!observedReads.includes(envPath), 'Sub-agent load should not read env files at runtime.');
        const internalChildren = (registry as unknown as { children: Map<string, { systemTemplate: string }> }).children;
        invariant(internalChildren instanceof Map, 'Sub-agent registry children map missing.');
        const subAgentInfo = internalChildren.get('sub-agent');
        invariant(subAgentInfo !== undefined, 'Sub-agent child info missing.');
        invariant(!subAgentInfo.systemTemplate.includes('${include:'), 'Sub-agent system prompt should not contain include directives post-flatten.');
        invariant(subAgentInfo.systemTemplate.includes('Sub-agent body start.'), 'Sub-agent system prompt missing body content.');
        invariant(subAgentInfo.systemTemplate.includes(nestedIncludeMarker), 'Sub-agent system prompt missing nested include content.');

        return Promise.resolve({
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'text',
            content: 'preload-validated',
            ts: Date.now(),
          },
        });
      } finally {
        cleanup();
      }
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-75 expected successful preload validation.');
    },
  },
  (() => {
    return {
      id: 'run-test-76',
      description: 'Env fallback when high-priority env file is unreadable.',
      execute: (): Promise<AIAgentResult> => {
        const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}env-layer-`));
        const homeDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}env-home-`));
        const originalCwd = process.cwd();
        const originalHome = process.env.HOME;
        const originalUserProfile = process.env.USERPROFILE;
        let blockedEnvPath: string | undefined;
        const cleanup = () => {
          try {
            if (blockedEnvPath !== undefined) {
              try { fs.chmodSync(blockedEnvPath, 0o600); } catch { /* ignore chmod errors */ }
            }
            process.chdir(originalCwd);
            if (originalHome !== undefined) process.env.HOME = originalHome; else delete process.env.HOME;
            if (originalUserProfile !== undefined) process.env.USERPROFILE = originalUserProfile; else delete process.env.USERPROFILE;
          } finally {
            try { fs.rmSync(tempDir, { recursive: true, force: true }); } catch { /* ignore */ }
            try { fs.rmSync(homeDir, { recursive: true, force: true }); } catch { /* ignore */ }
          }
        };
        try {
          const promptPath = path.join(tempDir, 'env-fallback.ai');
          fs.writeFileSync(promptPath, [
            '---',
            'description: Env fallback agent',
            'models:',
            `  - primary/${MODEL_NAME}`,
            '---',
            'Env fallback body.',
            '',
          ].join('\n'));

          const configPath = path.join(tempDir, CONFIG_FILENAME);
          fs.writeFileSync(configPath, JSON.stringify({
            providers: {
              primary: {
                type: 'test-llm',
                apiKey: '${SPECIAL_TOKEN}',
              },
            },
          }, null, 2));

          blockedEnvPath = path.join(tempDir, '.ai-agent.env');
          fs.writeFileSync(blockedEnvPath, 'SPECIAL_TOKEN=blocked-secret');
          fs.chmodSync(blockedEnvPath, 0o000);

          const homeConfigDir = path.join(homeDir, '.ai-agent');
          fs.mkdirSync(homeConfigDir, { recursive: true });
          fs.writeFileSync(path.join(homeConfigDir, 'ai-agent.json'), JSON.stringify({
            providers: {
              primary: { type: 'test-llm', apiKey: '${SPECIAL_TOKEN}' }
            }
          }, null, 2));
          fs.writeFileSync(path.join(homeConfigDir, 'ai-agent.env'), 'SPECIAL_TOKEN=home-secret');

          process.env.HOME = homeDir;
          process.env.USERPROFILE = homeDir;
          process.chdir(tempDir);

          const loaded = loadAgent(promptPath);
          const providerConfig = loaded.config.providers.primary as { apiKey?: string };
          invariant(providerConfig.apiKey === 'home-secret', 'Env fallback should use readable lower-priority env file.');

          return Promise.resolve({
            success: true,
            conversation: [],
            logs: [],
            accounting: [],
            finalReport: {
              status: 'success',
              format: 'text',
              content: 'env-fallback-valid',
              ts: Date.now(),
            },
          });
        } finally {
          cleanup();
        }
      },
      expect: (result) => {
        invariant(result.success, 'Scenario run-test-76 expected success.');
      },
    };
  })(),
  {
    id: 'run-test-77',
    description: 'Agent frontmatter stripped while include content preserved.',
    execute: (): Promise<AIAgentResult> => {
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}frontmatter-include-`));
      const cleanup = () => {
        try { fs.rmSync(tempDir, { recursive: true, force: true }); } catch { /* ignore */ }
      };
      try {
        const includePath = path.join(tempDir, 'include.md');
        fs.writeFileSync(includePath, [
          '---',
          FRONTMATTER_INCLUDE_FLAG,
          '---',
          FRONTMATTER_INCLUDE_MARKER,
          '',
        ].join('\n'));

        const promptPath = path.join(tempDir, 'frontmatter-test.ai');
        fs.writeFileSync(promptPath, [
          '---',
          'description: Frontmatter strip test',
          `models:`,
          `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
          '---',
          FRONTMATTER_BODY_PREFIX,
          '${include:include.md}',
          FRONTMATTER_BODY_SUFFIX,
          '',
        ].join('\n'));

        const configPath = path.join(tempDir, CONFIG_FILENAME);
        fs.writeFileSync(configPath, JSON.stringify({
          providers: {
            [PRIMARY_PROVIDER]: { type: 'test-llm' },
          },
        }, null, 2));

        const loaded = loadAgent(promptPath, undefined, { configPath });
        const systemTemplate = loaded.systemTemplate;
        invariant(!systemTemplate.trimStart().startsWith('---'), 'System template must not start with frontmatter.');
        invariant(systemTemplate.includes(FRONTMATTER_BODY_PREFIX), 'System body prefix missing.');
        invariant(systemTemplate.includes(FRONTMATTER_BODY_SUFFIX), 'System body suffix missing.');
        invariant(systemTemplate.includes(FRONTMATTER_INCLUDE_FLAG), 'Included frontmatter content missing.');
        invariant(systemTemplate.includes(FRONTMATTER_INCLUDE_MARKER), 'Included raw block missing.');

        return Promise.resolve({
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'text',
            content: 'frontmatter-include-validated',
            ts: Date.now(),
          },
        });
      } finally {
        cleanup();
      }
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-77 expected success.');
    },
  },
  (() => {
    let progressMessages: string[] = [];
    return {
      id: 'run-test-58',
      configure: (_configuration, sessionConfig) => {
        progressMessages = [];
        sessionConfig.initialTitle = 'Harness Metrics';
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onProgress: (event) => {
            if (event.type === 'agent_update' && typeof event.message === 'string') {
              progressMessages.push(event.message);
            }
            existingCallbacks.onProgress?.(event);
          },
        };
      },
      expect: (result) => {
        invariant(result.success, 'Scenario run-test-58 expected success.');
        invariant(progressMessages.some((message) => message.includes('Collecting metrics via test MCP tool.')), 'Progress update should include MCP metrics message for run-test-58.');
        const llmEntries = result.accounting.filter(isLlmAccounting);
        invariant(llmEntries.length >= 1, 'LLM accounting entry expected for run-test-58.');
        const toolEntries = result.accounting.filter(isToolAccounting);
        invariant(toolEntries.some((entry) => entry.command === 'test__test'), 'Tool accounting entry for test__test expected for run-test-58.');
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-58.');
      },
    };
  })(),
  {
    id: 'run-test-59',
    configure: (configuration, sessionConfig) => {
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
      sessionConfig.tools = ['test', 'batch'];
      sessionConfig.toolResponseMaxBytes = 120;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-59 expected success.');
      const truncLog = result.logs.find((entry) => entry.severity === 'WRN' && typeof entry.message === 'string' && entry.message.includes('response exceeded max size'));
      invariant(truncLog !== undefined, 'Truncation warning expected for run-test-59.');
      const batchTool = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(TRUNCATION_NOTICE));
      invariant(batchTool !== undefined, 'Truncated tool response expected for run-test-59.');
    },
  },

  (() => {
    interface CapturedRequestSnapshot {
      temperature?: number;
      topP?: number;
      maxOutputTokens?: number;
      repeatPenalty?: number;
      messages: { role: string; content: string }[];
    };
    const HISTORY_SYSTEM = 'Legacy advisory instructions.';
    const HISTORY_ASSISTANT = 'Historical assistant answer.';
    const TRACE_CONTEXT = { selfId: 'txn-session', originId: 'txn-origin', parentId: 'txn-parent', callPath: 'call/path' } as const;
    let capturedRequests: CapturedRequestSnapshot[] = [];
    return {
      id: 'run-test-60',
      configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        configuration.providers[PRIMARY_PROVIDER] = configuration.providers[PRIMARY_PROVIDER] ?? { type: 'test-llm' };
        sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
        sessionConfig.tools = ['test'];
        sessionConfig.temperature = 0.21;
        sessionConfig.topP = 0.77;
        sessionConfig.maxOutputTokens = 123;
        sessionConfig.repeatPenalty = 1.1;
        sessionConfig.renderTarget = 'cli';
        sessionConfig.initialTitle = 'Session Config Coverage';
        sessionConfig.agentId = 'session-config-agent';
        sessionConfig.trace = { ...TRACE_CONTEXT };
        sessionConfig.traceLLM = true;
        sessionConfig.conversationHistory = [
          { role: 'system', content: HISTORY_SYSTEM },
          { role: 'assistant', content: HISTORY_ASSISTANT },
        ];
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedRequests = [];
        // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
        const originalExecuteTurn = LLMClient.prototype.executeTurn;
        LLMClient.prototype.executeTurn = async function(request: TurnRequest): Promise<TurnResult> {
          const snapshot: CapturedRequestSnapshot = {
            temperature: request.temperature,
            topP: request.topP,
            maxOutputTokens: request.maxOutputTokens,
            repeatPenalty: request.repeatPenalty,
            messages: request.messages.map((message) => ({
              role: message.role,
              content: typeof message.content === 'string' ? message.content : JSON.stringify(message.content),
            })),
          };
          capturedRequests.push(snapshot);
          return await originalExecuteTurn.call(this, request);
        };
        try {
          const session = AIAgentSession.create(sessionConfig);
          return await session.run();
        } finally {
          LLMClient.prototype.executeTurn = originalExecuteTurn;
        }
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-60 expected success.');
        invariant(capturedRequests.length >= 1, 'Captured request snapshot missing for run-test-60.');
        const first = capturedRequests[0];
        const systemMessage = first.messages.find((message) => message.role === 'system');
        invariant(systemMessage?.content.includes('TOOLS') === true, 'System message should be enhanced with tool instructions in run-test-60.');
        invariant(first.messages.some((message) => message.role === 'assistant' && message.content.includes(HISTORY_ASSISTANT)), 'Conversation history assistant message missing in request for run-test-60.');
        invariant(first.temperature !== undefined && Math.abs(first.temperature - 0.21) < 1e-6, 'Temperature propagation failed for run-test-60.');
        invariant(first.topP !== undefined && Math.abs(first.topP - 0.77) < 1e-6, 'topP propagation failed for run-test-60.');
        invariant(first.maxOutputTokens === 123, 'maxOutputTokens propagation failed for run-test-60.');
        invariant(first.repeatPenalty !== undefined && Math.abs(first.repeatPenalty - 1.1) < 1e-6, 'repeatPenalty propagation failed for run-test-60.');
        const hasHistoryInResult = result.conversation.some((message) => message.role === 'assistant' && message.content.includes(HISTORY_ASSISTANT));
        invariant(hasHistoryInResult, 'Historical assistant message should be present in result conversation for run-test-60.');
        const llmEntry = result.accounting.find(isLlmAccounting);
        invariant(llmEntry !== undefined, 'LLM accounting entry expected for run-test-60.');
        invariant(typeof llmEntry.txnId === 'string' && llmEntry.txnId.length > 0, 'LLM accounting should include txnId for run-test-60.');
        invariant(llmEntry.callPath === TRACE_CONTEXT.callPath, 'Trace context should set callPath on LLM accounting entry for run-test-60.');
        invariant(llmEntry.originTxnId === TRACE_CONTEXT.originId, 'Trace context should preserve originTxnId on LLM accounting entry for run-test-60.');
        invariant(llmEntry.parentTxnId === TRACE_CONTEXT.parentId, 'Trace context should preserve parentTxnId on LLM accounting entry for run-test-60.');
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-60.');
      },
    };
  })(),
  (() => {
    let capturedLogs: LogEntry[] = [];
    const TOOL_LIMIT_WARNING_MESSAGE = 'Tool calls per turn exceeded';
    return {
      id: 'run-test-61',
      configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig, defaults) => {
        defaults.maxToolCallsPerTurn = 1;
        sessionConfig.maxToolCallsPerTurn = 1;
        sessionConfig.tools = ['test'];
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedLogs = [];
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onLog: (entry) => {
            capturedLogs.push(entry);
            existingCallbacks.onLog?.(entry);
          },
        };
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-61 expected session completion.');
        const limitLog = capturedLogs.find((entry) => entry.remoteIdentifier === 'agent:limits' && typeof entry.message === 'string' && entry.message.includes(TOOL_LIMIT_WARNING_MESSAGE));
        invariant(limitLog !== undefined, 'Limit enforcement log expected for run-test-61.');
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.status === 'failure', 'Final report should indicate failure for run-test-61.');
      },
    };
  })(),
  (() => {
    let abortController: AbortController | undefined;
    return {
      id: 'run-test-62',
      configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig, defaults) => {
        abortController = new AbortController();
        sessionConfig.abortSignal = abortController.signal;
        defaults.maxRetries = 3;
        sessionConfig.maxRetries = 3;
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const controller = abortController;
        invariant(controller !== undefined, 'Abort controller should be initialized for run-test-62.');
        const session = AIAgentSession.create(sessionConfig);
        setTimeout(() => { controller.abort(); }, 25);
        return await session.run();
      },
      expect: (result: AIAgentResult) => {
        invariant(!result.success, 'Scenario run-test-62 should cancel.');
        invariant(result.error === 'canceled', 'Canceled error expected for run-test-62.');
        invariant(result.finalReport === undefined, 'No final report expected for run-test-62.');
      },
    };
  })(),
  (() => {
    interface InitRecord { name: string; start: number; end: number }
    const initRecords: InitRecord[] = [];
    return {
      id: 'run-test-63',
      configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const serverScript = path.resolve(__dirname, 'mcp/test-stdio-server.js');
        configuration.mcpServers = {
          ...configuration.mcpServers,
          alpha: { type: 'stdio', command: process.execPath, args: [serverScript] },
          beta: { type: 'stdio', command: process.execPath, args: [serverScript] },
        };
        sessionConfig.verbose = true;
        sessionConfig.traceMCP = true;
        sessionConfig.mcpInitConcurrency = 1;
        sessionConfig.tools = ['test'];
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        initRecords.length = 0;
        const providerProto = MCPProvider.prototype as unknown as { initializeServer: (this: MCPProvider, name: string, config: MCPServerConfig) => Promise<unknown> };
        // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
        const originalInitialize = providerProto.initializeServer;
        providerProto.initializeServer = async function(this: MCPProvider, name: string, config: MCPServerConfig) {
          const start = Date.now();
          initRecords.push({ name, start, end: start });
          const index = initRecords.length - 1;
          await new Promise((resolve) => { setTimeout(resolve, 15); });
          const result = await originalInitialize.call(this, name, config);
          initRecords[index].end = Date.now();
          return result;
        };
        try {
          const session = AIAgentSession.create(sessionConfig);
          return await session.run();
        } finally {
          providerProto.initializeServer = originalInitialize;
        }
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-63 expected success.');
        invariant(initRecords.length >= 2, 'Initialization records expected for run-test-63.');
        const sequential = initRecords.slice(1).every((record, index) => record.start >= initRecords[index].end);
        invariant(sequential, 'MCP warmup should honor init concurrency limit for run-test-63.');
        const traceLog = result.logs.find((entry) => entry.remoteIdentifier === 'trace:agent:agent');
        invariant(traceLog !== undefined, 'Trace log expected for run-test-63.');
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should indicate success for run-test-63.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-64',
      description: 'Base LLM provider error mapping coverage.',
      execute: () => {
        const providerConfig: ProviderConfig = { type: 'test-llm' };
        const provider = new TestLLMProvider(providerConfig);
        const statuses = {
          auth: provider.mapError({ status: 401, message: 'invalid api key', name: 'AuthError' }),
          quota: provider.mapError({ status: 402, message: 'quota exceeded', name: 'BillingError' }),
          timeout: provider.mapError({ name: 'TimeoutError', message: 'Request timeout occurred' }),
          network: provider.mapError({ status: 503, message: 'connection reset', name: 'NetworkError' }),
          model: provider.mapError({ status: 400, message: 'model unsupported', name: 'BadRequestError' }),
          nested: provider.mapError({ lastError: { response: { status: 429, data: { error: { message: 'Rate limit reached' } } }, message: 'Too Many Requests' } }),
          responseBody: provider.mapError({ status: 500, responseBody: '{"error":{"message":"Upstream invalid","code":"quota"}}', message: 'Server error' }),
        };
        return Promise.resolve({
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: { status: 'success', format: 'json', content_json: {
            auth: statuses.auth.type,
            quota: statuses.quota.type,
            timeout: statuses.timeout.type,
            network: statuses.network.type,
            model: statuses.model.type,
            nested: statuses.nested.type,
            responseBody: statuses.responseBody.type,
          }, ts: Date.now() },
        } as AIAgentResult);
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-64 expected success.');
        const data = result.finalReport?.content_json as Record<string, string> | undefined;
        invariant(data !== undefined, 'Final report data expected for run-test-64.');
        invariant(data.auth === 'auth_error', 'Auth error mapping mismatch for run-test-64.');
        invariant(data.quota === 'quota_exceeded', 'Quota error mapping mismatch for run-test-64.');
        invariant(data.timeout === 'timeout', 'Timeout mapping mismatch for run-test-64.');
        invariant(data.network === 'network_error', 'Network mapping mismatch for run-test-64.');
        invariant(data.model === 'model_error', 'Model error mapping mismatch for run-test-64.');
        invariant(data.nested === 'rate_limit', 'Nested error mapping mismatch for run-test-64.');
        invariant(data.responseBody === 'network_error', 'Response body mapping mismatch for run-test-64.');
      },
    };
  })(),
  (() => {
    const captured: { namespace: string; filtered: string[]; stdioError?: string; combined?: string } = { namespace: '', filtered: [] };
    return {
      id: 'run-test-65',
      description: 'MCP provider filtering and logging coverage.',
      execute: () => {
        const logs: LogEntry[] = [];
        const provider = new MCPProvider('deterministic', {}, { trace: true, verbose: true, onLog: (entry) => { logs.push(entry); } });
        const exposed = provider as unknown as {
          sanitizeNamespace: (name: string) => string;
          filterToolsForServer: (name: string, config: MCPServerConfig, tools: MCPTool[]) => MCPTool[];
          createStdioTransport: (name: string, config: MCPServerConfig) => unknown;
          log: (severity: 'VRB' | 'WRN' | 'ERR' | 'TRC', message: string, remoteIdentifier: string, fatal?: boolean) => void;
        };
        captured.namespace = exposed.sanitizeNamespace('Alpha-Server!!');
        const toolList: MCPTool[] = [
          { name: 'alphaTool', description: 'primary', inputSchema: {} },
          { name: 'betaTool', description: 'secondary', inputSchema: {} },
          { name: 'gammaTool', description: 'third', inputSchema: {} },
        ];
        const filtered = exposed.filterToolsForServer(
          'alpha',
          { type: 'stdio', command: 'node', toolsAllowed: ['alphaTool', 'GammaTool'], toolsDenied: ['betaTool'] } as MCPServerConfig,
          toolList
        );
        captured.filtered = filtered.map((tool) => tool.name);
        try {
          exposed.createStdioTransport('broken', { type: 'stdio' } as MCPServerConfig);
        } catch (error: unknown) {
          captured.stdioError = toErrorMessage(error);
        }
        exposed.log('TRC', 'synthetic trace', 'mcp:test');
        const serversField = provider as unknown as {
          servers: Map<string, { name: string; config: MCPServerConfig; tools: MCPTool[]; instructions?: string }>;
        };
        serversField.servers = new Map<string, { name: string; config: MCPServerConfig; tools: MCPTool[]; instructions?: string }>([
          [
            'alpha',
            {
              name: 'alpha',
              config: { type: 'stdio', command: 'node' } as MCPServerConfig,
              tools: [{ name: 'alphaTool', description: 'Alpha', inputSchema: {}, instructions: 'Alpha instruction' }],
              instructions: 'Server instructions',
            },
          ],
        ]);
        captured.combined = provider.getCombinedInstructions();
        return Promise.resolve({
          success: true,
          conversation: [],
          logs,
          accounting: [],
          finalReport: { status: 'success', format: 'json', content_json: captured, ts: Date.now() },
        } as AIAgentResult);
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-65 expected success.');
        const data = result.finalReport?.content_json as
          | { namespace: string; filtered: string[]; stdioError?: string; combined?: string }
          | undefined;
        invariant(data !== undefined, 'Combined data expected for run-test-65.');
        invariant(data.namespace === 'alpha_server', 'Namespace sanitization mismatch for run-test-65.');
        invariant(
          data.filtered.length === 2 && data.filtered.includes('alphaTool') && data.filtered.includes('gammaTool'),
          'Tool filtering mismatch for run-test-65.'
        );
        invariant(
          typeof data.stdioError === 'string' && data.stdioError.includes('requires a string'),
          'Expected stdio transport error for run-test-65.'
        );
        invariant(
          typeof data.combined === 'string' &&
            data.combined.includes('#### MCP Server: alpha') &&
            data.combined.includes('##### Tool: alpha__alphaTool'),
          'Combined instructions mismatch for run-test-65.'
        );
        invariant(
          result.logs.some((entry) => entry.remoteIdentifier === 'mcp:test' && entry.severity === 'TRC'),
          'Trace log expected for run-test-65.'
        );
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-66',
      description: 'Sub-agent recursion prevented by ancestors list.',
      configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const canonicalPath = fs.realpathSync(SUBAGENT_PRICING_PROMPT);
        sessionConfig.subAgents = [preloadSubAgentFromPath(canonicalPath, configuration)];
        sessionConfig.ancestors = [canonicalPath];
      },
      expect: (result: AIAgentResult) => {
        invariant(!result.success, 'Scenario run-test-66 should fail while creating session.');
        invariant(typeof result.error === 'string' && result.error.includes('Recursion detected'), 'Recursion detection message expected for run-test-66.');
        invariant(result.conversation.length === 0, 'No conversation messages expected for run-test-66.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-67',
      description: 'LLM client reports unknown provider errors.',
      execute: async () => {
        let caught: string | undefined;
        const client = new LLMClient({});
        try {
          await client.executeTurn({
            provider: 'missing-provider',
            model: 'unused',
            messages: [{ role: 'user', content: 'noop' }],
            tools: [],
            toolExecutor: () => Promise.resolve(''),
            temperature: 0,
            topP: 1,
            maxOutputTokens: 32,
            stream: false,
            isFinalTurn: true,
            llmTimeout: 1_000,
          });
        } catch (error: unknown) {
          caught = toErrorMessage(error);
        }
        return {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: { error: caught },
            ts: Date.now(),
          },
        };
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-67 should complete successfully.');
        const data = result.finalReport?.content_json as { error?: string } | undefined;
        invariant(data !== undefined && typeof data.error === 'string', 'Captured error expected for run-test-67.');
        invariant(data.error.includes('Unknown provider: missing-provider'), 'Unknown provider message mismatch for run-test-67.');
      },
    };
  })(),
  (() => {
    const ANTHROPIC_URL = 'https://anthropic.mock/v1/messages';
    return {
      id: 'run-test-68',
      description: 'LLM client enriches cache write tokens from traced fetch.',
      execute: async () => {
        const originalFetch = globalThis.fetch;
        const logs: LogEntry[] = [];
        globalThis.fetch = (_input?: RequestInfo | URL, _init?: RequestInit): Promise<Response> => {
          const body = JSON.stringify({
            usage: {
              cacheCreationInputTokens: 321,
            },
          });
          return Promise.resolve(new Response(body, {
            status: 200,
            headers: { 'content-type': 'application/json' },
          }));
        };
        try {
          const client = new LLMClient(
            {
              [PRIMARY_PROVIDER]: { type: 'test-llm' },
            },
            {
              traceLLM: true,
              onLog: (entry) => { logs.push(entry); },
            }
          );
          const tracedFetch = (client as unknown as { createTracedFetch: () => typeof fetch }).createTracedFetch();
          await tracedFetch(ANTHROPIC_URL, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ prompt: 'cache-enrich' }),
          });
          const clientInternals = client as unknown as { logRequest?: (req: TurnRequest) => void; lastCacheWriteInputTokens?: number };
          const originalLogRequest = typeof clientInternals.logRequest === 'function' ? clientInternals.logRequest : undefined;
          const cachedValue = clientInternals.lastCacheWriteInputTokens;
          if (originalLogRequest !== undefined) {
            clientInternals.logRequest = function(request: TurnRequest): void {
              originalLogRequest.call(this, request);
              if (typeof cachedValue === 'number') {
                clientInternals.lastCacheWriteInputTokens = cachedValue;
              }
            };
          }
          try {
            const request: TurnRequest = {
              provider: PRIMARY_PROVIDER,
              model: MODEL_NAME,
              messages: [
                { role: 'system', content: 'Phase 1 deterministic harness: respond using scripted outputs.' },
                { role: 'user', content: 'run-test-68' },
              ],
              tools: [],
              toolExecutor: () => Promise.resolve(''),
              temperature: 0,
              topP: 1,
              maxOutputTokens: 64,
              stream: false,
              isFinalTurn: true,
              llmTimeout: 5_000,
            };
            const turnResult = await client.executeTurn(request);
            const routing = client.getLastActualRouting() ?? {};
            const costs = client.getLastCostInfo() ?? {};
            return {
              success: true,
              conversation: [],
              logs,
              accounting: [],
              finalReport: {
                status: 'success',
                format: 'json',
                content_json: {
                  tokens: turnResult.tokens ?? null,
                  routing,
                  costs,
                },
                ts: Date.now(),
              },
            };
          } finally {
            if (originalLogRequest !== undefined) {
              clientInternals.logRequest = originalLogRequest;
            }
          }
        } finally {
          globalThis.fetch = originalFetch;
        }
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-68 expected success.');
        const data = result.finalReport?.content_json as
          | { tokens?: { cacheWriteInputTokens?: number; inputTokens?: number; outputTokens?: number; totalTokens?: number }; routing?: { provider?: string; model?: string }; costs?: { costUsd?: number; upstreamInferenceCostUsd?: number } }
          | undefined;
        invariant(data !== undefined, 'Final report data expected for run-test-68.');
        invariant(data.tokens !== undefined && data.tokens.cacheWriteInputTokens === 321, 'Cache write enrichment expected for run-test-68.');
        invariant(data.routing !== undefined && data.routing.provider === undefined && data.routing.model === undefined, 'Routing should remain empty for run-test-68.');
        invariant(data.costs !== undefined && data.costs.costUsd === undefined && data.costs.upstreamInferenceCostUsd === undefined, 'Cost info should remain empty for run-test-68.');
        invariant(result.logs.some((entry) => entry.severity === 'TRC' && typeof entry.remoteIdentifier === 'string' && entry.remoteIdentifier.startsWith('trace:')), 'Trace log expected for run-test-68.');
      },
    };
  })(),
  (() => {
    let capturedTopP: number | undefined;
    return {
      id: 'run-test-69',
      description: 'Model override snake-case top_p propagation.',
      configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        configuration.providers[PRIMARY_PROVIDER] = {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              overrides: { top_p: 0.66 },
            },
          },
        };
        sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
        sessionConfig.userPrompt = 'run-test-17';
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedTopP = undefined;
        // eslint-disable-next-line @typescript-eslint/unbound-method -- capture method for restoration after interception
        const originalExecute = LLMClient.prototype.executeTurn;
        LLMClient.prototype.executeTurn = async function(request: TurnRequest): Promise<TurnResult> {
          capturedTopP = request.topP;
          return await originalExecute.call(this, request);
        };
        try {
          const session = AIAgentSession.create(sessionConfig);
          return await session.run();
        } finally {
          LLMClient.prototype.executeTurn = originalExecute;
        }
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-69 expected success.');
        invariant(typeof capturedTopP === 'number' && Math.abs(capturedTopP - 0.66) < 1e-6, 'top_p override propagation failed for run-test-69.');
      },
    };
  })(),
  (() => {
    let capturedEvents: AgentFinishedEvent[] = [];
    return {
      id: 'run-test-70',
      description: 'Default callPath and agent labels when agentId is unset.',
      configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedEvents = [];
        sessionConfig.agentId = undefined;
        sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onProgress: (event) => {
            if (event.type === 'agent_finished') {
              capturedEvents.push(event);
            }
            existingCallbacks.onProgress?.(event);
          },
        };
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-70 expected success.');
        const finishedEvent = capturedEvents.at(-1);
        invariant(finishedEvent !== undefined, 'Agent finished event expected for run-test-70.');
        invariant(finishedEvent.callPath === 'agent', 'Default callPath should be "agent" for run-test-70.');
        invariant(finishedEvent.agentId === 'agent', 'Default agentId should be "agent" for run-test-70.');
        invariant(finishedEvent.agentName === 'agent', 'Default agentName should be "agent" for run-test-70.');
      },
    };
  })(),
  (() => {
    let capturedEvents: AgentFinishedEvent[] = [];
    return {
      id: 'run-test-71',
      description: 'Agent display name fallback when identifier is whitespace.',
      configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedEvents = [];
        sessionConfig.agentId = '  ';
        sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onProgress: (event) => {
            if (event.type === 'agent_finished') {
              capturedEvents.push(event);
            }
            existingCallbacks.onProgress?.(event);
          },
        };
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-71 expected success.');
        const finishedEvent = capturedEvents.at(-1);
        invariant(finishedEvent !== undefined, 'Agent finished event expected for run-test-71.');
        invariant(finishedEvent.callPath === '  ', 'Whitespace agentId should propagate to callPath for run-test-71.');
        invariant(finishedEvent.agentId === '  ', 'Whitespace agentId should remain unchanged for run-test-71.');
        invariant(finishedEvent.agentName === '  ', 'Whitespace agentId should yield same agentName for run-test-71.');
      },
    };
  })(),
  (() => {
    let capturedEvent: AgentFinishedEvent | undefined;
    let expectedAgentId = '';
    return {
      id: 'run-test-72',
      description: 'Trace callPath fallback to agentId when callPath is empty.',
      configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedEvent = undefined;
        expectedAgentId = sessionConfig.agentId ?? '';
        sessionConfig.trace = { ...(sessionConfig.trace ?? {}), callPath: '' };
        sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onProgress: (event) => {
            if (event.type === 'agent_finished') {
              capturedEvent = event;
            }
            existingCallbacks.onProgress?.(event);
          },
        };
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-72 expected success.');
        const event = capturedEvent;
        invariant(event !== undefined, 'Agent finished event expected for run-test-72.');
        invariant(event.callPath === expectedAgentId, 'CallPath should fallback to agentId when trace callPath is empty (run-test-72).');
        invariant(event.agentId === expectedAgentId, 'AgentId should remain unchanged for run-test-72.');
      },
    };
  })(),
  (() => {
    let capturedSessionConfig: AIAgentSessionConfig | undefined;
    const historyMessages: ConversationMessage[] = [
      { role: 'system', content: 'Historical system guidance.' },
      { role: 'assistant', content: 'Historical assistant output.' },
    ];
    const loaderCallbacks: AIAgentCallbacks = {
      onLog: () => undefined,
      onOutput: () => undefined,
      onThinking: () => undefined,
      onAccounting: () => undefined,
      onProgress: () => undefined,
      onOpTree: () => undefined,
    };
    const traceContext = { originId: 'origin-trace', parentId: 'parent-trace', callPath: 'loader/session' };
    const stopReference = { stopping: false };
    const ancestorsList = ['ancestor-alpha', 'ancestor-beta'];
    let loaderAbortSignal: AbortSignal | undefined;
    return {
      id: 'run-test-73',
      description: 'Agent loader propagates session configuration overrides.',
      execute: async () => {
        capturedSessionConfig = undefined;
        const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'phase1-loader-'));
        const configPath = path.join(tempDir, 'ai-agent.json');
        const configData = {
          providers: {
            [PRIMARY_PROVIDER]: { type: 'test-llm' },
          },
          mcpServers: {
            test: {
              type: 'stdio',
              command: process.execPath,
              args: ['-e', 'process.exit(0)'],
            },
          },
          defaults: {
            llmTimeout: 1_111,
            toolTimeout: 2_222,
            temperature: 0.11,
            topP: 0.21,
            maxOutputTokens: 999,
            repeatPenalty: 1.01,
            maxRetries: 2,
            maxToolTurns: 3,
            maxToolCallsPerTurn: 4,
            parallelToolCalls: false,
            maxConcurrentTools: 5,
            toolResponseMaxBytes: 5_555,
            stream: false,
            mcpInitConcurrency: 6,
          },
        };
        fs.writeFileSync(configPath, JSON.stringify(configData, null, 2), 'utf-8');
        const promptContent = [
          '---',
          'description: Loader harness coverage',
          `models:`,
          `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
          'tools:',
          '  - test',
          'llmTimeout: 3333',
          'toolTimeout: 4444',
          'maxRetries: 7',
          'maxToolTurns: 8',
          'maxToolCallsPerTurn: 9',
          'maxConcurrentTools: 10',
          'parallelToolCalls: true',
          'temperature: 0.31',
          'topP: 0.41',
          'maxOutputTokens: 1234',
          'repeatPenalty: 1.07',
          'toolResponseMaxBytes: 6666',
          'output:',
          '  format: json',
          '  schema:',
          '    type: object',
          '    properties:',
          '      ok:',
          '        type: boolean',
          '    required:',
          '      - ok',
          '---',
          'Loader scenario prompt body.',
        ].join('\n');
        const options = {
          baseDir: tempDir,
          configPath,
          stream: true,
          parallelToolCalls: false,
          maxRetries: 11,
          maxToolTurns: 12,
          maxToolCallsPerTurn: 16,
          maxConcurrentTools: 13,
          llmTimeout: 5_555,
          toolTimeout: 6_666,
          temperature: 0.51,
          topP: 0.61,
          maxOutputTokens: 7_777,
          repeatPenalty: 1.2,
          toolResponseMaxBytes: 8_888,
          mcpInitConcurrency: 14,
          traceLLM: true,
          traceMCP: true,
          verbose: true,
        };
        const controller = new AbortController();
        loaderAbortSignal = controller.signal;
        const originalCreate = AIAgentSession.create.bind(AIAgentSession);
        (AIAgentSession as unknown as { create: (sessionConfig: AIAgentSessionConfig) => AIAgentSession }).create = (sessionConfig: AIAgentSessionConfig) => {
          capturedSessionConfig = sessionConfig;
          return originalCreate(sessionConfig);
        };
        try {
          const registry = new AgentRegistry([], { configPath, verbose: true });
          const loaded = registry.loadFromContent(path.join(tempDir, 'loader.ai'), promptContent, options);
          await loaded.createSession(
            'Loader System Prompt',
            'Loader User Prompt',
            {
              history: historyMessages,
              callbacks: loaderCallbacks,
              trace: traceContext,
              renderTarget: 'web',
              outputFormat: 'json',
              abortSignal: controller.signal,
              stopRef: stopReference,
              initialTitle: 'Loader Session Title',
              ancestors: ancestorsList,
            }
          );
        } finally {
          (AIAgentSession as unknown as { create: (sessionConfig: AIAgentSessionConfig) => AIAgentSession }).create = originalCreate;
          fs.rmSync(tempDir, { recursive: true, force: true });
        }
        return {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'text',
            content: 'loader-session-config',
            ts: Date.now(),
          },
        };
      },
      expect: () => {
        invariant(capturedSessionConfig !== undefined, 'Session config capture expected for run-test-73.');
        const cfg = capturedSessionConfig;
        invariant(Object.prototype.hasOwnProperty.call(cfg.config.providers, PRIMARY_PROVIDER), 'Provider config missing for run-test-73.');
        const providerConfig = cfg.config.providers[PRIMARY_PROVIDER];
        invariant(providerConfig.type === 'test-llm', 'Provider type mismatch for run-test-73.');
        const hasPrimaryTarget = cfg.targets.some((entry) => entry.provider === PRIMARY_PROVIDER && entry.model === MODEL_NAME);
        invariant(hasPrimaryTarget, 'Targets mismatch for run-test-73.');
        invariant(cfg.tools.includes('test'), 'Tool selection mismatch for run-test-73.');
        invariant(cfg.agentId === 'loader', 'Agent identifier mismatch for run-test-73.');
        invariant(Array.isArray(cfg.subAgents), 'Sub-agent list expected array for run-test-73.');
        invariant(cfg.subAgents.length === 0, 'Sub-agent list expected empty for run-test-73.');
        invariant(cfg.systemPrompt === 'Loader System Prompt', 'System prompt mismatch for run-test-73.');
        invariant(cfg.userPrompt === 'Loader User Prompt', 'User prompt mismatch for run-test-73.');
        invariant(cfg.outputFormat === 'json', 'Output format mismatch for run-test-73.');
        invariant(cfg.renderTarget === 'web', 'Render target mismatch for run-test-73.');
        invariant(cfg.conversationHistory === historyMessages, 'Conversation history reference mismatch for run-test-73.');
        invariant(cfg.expectedOutput !== undefined, 'Expected output missing for run-test-73.');
        invariant(cfg.expectedOutput.format === 'json', 'Expected output format mismatch for run-test-73.');
        const callbacks = cfg.callbacks;
        invariant(callbacks !== undefined, 'Callbacks missing for run-test-73.');
        invariant(callbacks === loaderCallbacks, 'Callbacks reference mismatch for run-test-73.');
        invariant(cfg.trace === traceContext, 'Trace context mismatch for run-test-73.');
        invariant(cfg.stopRef === stopReference, 'Stop reference mismatch for run-test-73.');
        invariant(cfg.initialTitle === 'Loader Session Title', 'Initial title mismatch for run-test-73.');
        invariant(loaderAbortSignal !== undefined && cfg.abortSignal === loaderAbortSignal && !cfg.abortSignal.aborted, 'Abort signal mismatch for run-test-73.');
        invariant(cfg.ancestors === ancestorsList, 'Ancestors propagation mismatch for run-test-73.');
        invariant(cfg.temperature !== undefined, 'Temperature undefined for run-test-73.');
        invariant(Math.abs(cfg.temperature - 0.51) < 1e-6, 'Temperature override mismatch for run-test-73.');
        invariant(cfg.topP !== undefined, 'topP undefined for run-test-73.');
        invariant(Math.abs(cfg.topP - 0.61) < 1e-6, 'topP override mismatch for run-test-73.');
        invariant(cfg.maxOutputTokens === 7_777, `maxOutputTokens mismatch for run-test-73 (actual=${String(cfg.maxOutputTokens)})`);
        invariant(cfg.repeatPenalty !== undefined && Math.abs(cfg.repeatPenalty - 1.2) < 1e-6, 'repeatPenalty mismatch for run-test-73.');
        invariant(cfg.maxRetries === 11, 'maxRetries override mismatch for run-test-73.');
        invariant(cfg.maxTurns === 12, 'maxTurns override mismatch for run-test-73.');
        invariant(cfg.maxToolCallsPerTurn === 16, 'maxToolCallsPerTurn mismatch for run-test-73.');
        invariant(cfg.maxConcurrentTools === 13, 'maxConcurrentTools override mismatch for run-test-73.');
        invariant(cfg.llmTimeout === 5_555, 'llmTimeout override mismatch for run-test-73.');
        invariant(cfg.toolTimeout === 6_666, 'toolTimeout override mismatch for run-test-73.');
        invariant(cfg.parallelToolCalls !== undefined, 'parallelToolCalls undefined for run-test-73.');
        invariant(!cfg.parallelToolCalls, 'parallelToolCalls override mismatch for run-test-73.');
        invariant(cfg.stream !== undefined, 'stream undefined for run-test-73.');
        invariant(cfg.stream, 'stream override mismatch for run-test-73.');
        invariant(cfg.traceLLM !== undefined, 'traceLLM undefined for run-test-73.');
        invariant(cfg.traceLLM, 'traceLLM override mismatch for run-test-73.');
        invariant(cfg.traceMCP !== undefined, 'traceMCP undefined for run-test-73.');
        invariant(cfg.traceMCP, 'traceMCP override mismatch for run-test-73.');
        invariant(cfg.verbose !== undefined, 'verbose undefined for run-test-73.');
        invariant(cfg.verbose, 'verbose override mismatch for run-test-73.');
        invariant(cfg.toolResponseMaxBytes === 8_888, 'toolResponseMaxBytes override mismatch for run-test-73.');
        invariant(cfg.mcpInitConcurrency === 14, 'mcpInitConcurrency override mismatch for run-test-73.');
      },
    };
  })(),

  {
    id: 'run-test-94',
    description: 'Global model overrides propagate to parent and sub-agents.',
    execute: async () => {
      overrideParentLoaded = undefined;
      overrideChildLoaded = undefined;
      overrideTargetsRef = undefined;

      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}override-models-`));
      try {
        const configPath = path.join(tempDir, 'override-config.json');
        const configData = {
          providers: {
            [PRIMARY_PROVIDER]: { type: 'test-llm' },
          },
          mcpServers: {},
        } satisfies Configuration;
        fs.writeFileSync(configPath, JSON.stringify(configData, null, 2), 'utf-8');

        const childPath = path.join(tempDir, 'child.ai');
        const childContent = [
          '---',
          'description: Override child agent',
          'models:',
          '  - other-provider/child-model',
          '---',
          COVERAGE_CHILD_BODY,
        ].join('\n');
        fs.writeFileSync(childPath, childContent, 'utf-8');

        const parentPath = path.join(tempDir, 'parent.ai');
        const parentContent = [
          '---',
          'description: Override parent agent',
          'models:',
          '  - fallback/parent-model',
          'agents:',
          `  - ${path.basename(childPath)}`,
          '---',
          COVERAGE_PARENT_BODY,
        ].join('\n');
        fs.writeFileSync(parentPath, parentContent, 'utf-8');

        const overrideTargets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
        overrideTargetsRef = overrideTargets;
        const overrides: NonNullable<LoadAgentOptions['globalOverrides']> = {
          models: overrideTargets,
          temperature: overrideLLMExpected.temperature,
          topP: overrideLLMExpected.topP,
          maxOutputTokens: overrideLLMExpected.maxOutputTokens,
          repeatPenalty: overrideLLMExpected.repeatPenalty,
          llmTimeout: overrideLLMExpected.llmTimeout,
          toolTimeout: overrideLLMExpected.toolTimeout,
          maxRetries: overrideLLMExpected.maxRetries,
          maxToolTurns: overrideLLMExpected.maxToolTurns,
          maxToolCallsPerTurn: overrideLLMExpected.maxToolCallsPerTurn,
          maxConcurrentTools: overrideLLMExpected.maxConcurrentTools,
          toolResponseMaxBytes: overrideLLMExpected.toolResponseMaxBytes,
          mcpInitConcurrency: overrideLLMExpected.mcpInitConcurrency,
          stream: overrideLLMExpected.stream,
          parallelToolCalls: overrideLLMExpected.parallelToolCalls,
        };

        const registry = new AgentRegistry([], { configPath, globalOverrides: overrides });
        overrideParentLoaded = registry.loadFromContent(parentPath, parentContent, {
          configPath,
          baseDir: tempDir,
          globalOverrides: overrides,
        });
        invariant(overrideParentLoaded.subAgents.length === 1, 'Override scenario should preload exactly one sub-agent.');
        overrideChildLoaded = overrideParentLoaded.subAgents[0]?.loaded;
      } finally {
        fs.rmSync(tempDir, { recursive: true, force: true });
      }

      return Promise.resolve({
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'text',
          content: 'override-models',
          ts: Date.now(),
        },
      } satisfies AIAgentResult);
    },
    expect: () => {
      invariant(overrideTargetsRef !== undefined, 'Override targets reference missing for global override test.');
      invariant(overrideParentLoaded !== undefined, 'Parent loaded agent missing for global override test.');
      invariant(overrideParentLoaded.targets === overrideTargetsRef, 'Parent targets reference mismatch for global override test.');
      invariant(overrideParentLoaded.targets.every((entry) => entry.provider === PRIMARY_PROVIDER && entry.model === MODEL_NAME), 'Parent targets should align with override values.');
      invariant(Math.abs(overrideParentLoaded.effective.temperature - overrideLLMExpected.temperature) < 1e-6, 'Parent temperature should follow overrides.');
      invariant(Math.abs(overrideParentLoaded.effective.topP - overrideLLMExpected.topP) < 1e-6, 'Parent topP should follow overrides.');
      invariant(overrideParentLoaded.effective.maxOutputTokens === overrideLLMExpected.maxOutputTokens, 'Parent maxOutputTokens should follow overrides.');
      invariant(overrideParentLoaded.effective.repeatPenalty !== undefined && Math.abs(overrideParentLoaded.effective.repeatPenalty - overrideLLMExpected.repeatPenalty) < 1e-6, 'Parent repeatPenalty should follow overrides.');
      invariant(overrideParentLoaded.effective.llmTimeout === overrideLLMExpected.llmTimeout, 'Parent llmTimeout should follow overrides.');
      invariant(overrideParentLoaded.effective.maxRetries === overrideLLMExpected.maxRetries, 'Parent maxRetries should follow overrides.');
      invariant(overrideParentLoaded.effective.toolTimeout === overrideLLMExpected.toolTimeout, 'Parent toolTimeout should follow overrides.');
      invariant(overrideParentLoaded.effective.maxToolTurns === overrideLLMExpected.maxToolTurns, 'Parent maxToolTurns should follow overrides.');
      invariant(overrideParentLoaded.effective.maxToolCallsPerTurn === overrideLLMExpected.maxToolCallsPerTurn, 'Parent maxToolCallsPerTurn should follow overrides.');
      invariant(overrideParentLoaded.effective.maxConcurrentTools === overrideLLMExpected.maxConcurrentTools, 'Parent maxConcurrentTools should follow overrides.');
      invariant(overrideParentLoaded.effective.toolResponseMaxBytes === overrideLLMExpected.toolResponseMaxBytes, 'Parent toolResponseMaxBytes should follow overrides.');
      invariant(overrideParentLoaded.effective.mcpInitConcurrency === overrideLLMExpected.mcpInitConcurrency, 'Parent mcpInitConcurrency should follow overrides.');
      invariant(overrideParentLoaded.effective.stream === overrideLLMExpected.stream, 'Parent stream flag should follow overrides.');
      invariant(overrideParentLoaded.effective.parallelToolCalls === overrideLLMExpected.parallelToolCalls, 'Parent parallelToolCalls should follow overrides.');
      invariant(overrideChildLoaded !== undefined, 'Child loaded agent missing for global override test.');
      invariant(overrideChildLoaded.targets === overrideTargetsRef, 'Child targets reference mismatch for global override test.');
      invariant(overrideChildLoaded.targets.every((entry) => entry.provider === PRIMARY_PROVIDER && entry.model === MODEL_NAME), 'Child targets should be overridden to primary provider/model.');
      invariant(Math.abs(overrideChildLoaded.effective.temperature - overrideLLMExpected.temperature) < 1e-6, 'Child temperature should follow overrides.');
      invariant(Math.abs(overrideChildLoaded.effective.topP - overrideLLMExpected.topP) < 1e-6, 'Child topP should follow overrides.');
      invariant(overrideChildLoaded.effective.maxOutputTokens === overrideLLMExpected.maxOutputTokens, 'Child maxOutputTokens should follow overrides.');
      invariant(overrideChildLoaded.effective.repeatPenalty !== undefined && Math.abs(overrideChildLoaded.effective.repeatPenalty - overrideLLMExpected.repeatPenalty) < 1e-6, 'Child repeatPenalty should follow overrides.');
      invariant(overrideChildLoaded.effective.llmTimeout === overrideLLMExpected.llmTimeout, 'Child llmTimeout should follow overrides.');
      invariant(overrideChildLoaded.effective.maxRetries === overrideLLMExpected.maxRetries, 'Child maxRetries should follow overrides.');
      invariant(overrideChildLoaded.effective.toolTimeout === overrideLLMExpected.toolTimeout, 'Child toolTimeout should follow overrides.');
      invariant(overrideChildLoaded.effective.maxToolTurns === overrideLLMExpected.maxToolTurns, 'Child maxToolTurns should follow overrides.');
      invariant(overrideChildLoaded.effective.maxToolCallsPerTurn === overrideLLMExpected.maxToolCallsPerTurn, 'Child maxToolCallsPerTurn should follow overrides.');
      invariant(overrideChildLoaded.effective.maxConcurrentTools === overrideLLMExpected.maxConcurrentTools, 'Child maxConcurrentTools should follow overrides.');
      invariant(overrideChildLoaded.effective.toolResponseMaxBytes === overrideLLMExpected.toolResponseMaxBytes, 'Child toolResponseMaxBytes should follow overrides.');
      invariant(overrideChildLoaded.effective.mcpInitConcurrency === overrideLLMExpected.mcpInitConcurrency, 'Child mcpInitConcurrency should follow overrides.');
      invariant(overrideChildLoaded.effective.stream === overrideLLMExpected.stream, 'Child stream flag should follow overrides.');
      invariant(overrideChildLoaded.effective.parallelToolCalls === overrideLLMExpected.parallelToolCalls, 'Child parallelToolCalls should follow overrides.');
    },
  },



  {
    id: 'run-test-96',
    description: 'Agent registry spawnSession resolves aliases and produces sessions.',
    execute: async () => {
      registrySpawnSessionConfig = undefined;
      await Promise.resolve();
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}spawn-session-`));
      const originalCreate = AIAgentSession.create.bind(AIAgentSession);
      try {
        const configPath = path.join(tempDir, CONFIG_FILE_NAME);
        const configData = {
          providers: { [PRIMARY_PROVIDER]: { type: 'test-llm' } },
          mcpServers: {},
        } satisfies Configuration;
        fs.writeFileSync(configPath, JSON.stringify(configData, null, 2), 'utf-8');
        const agentPath = path.join(tempDir, 'spawn-parent.ai');
        const agentContent = [
          '---',
          'description: Spawn session coverage agent',
          `models:`,
          `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
          '---',
          'Spawn body.',
        ].join('\n');
        fs.writeFileSync(agentPath, agentContent, 'utf-8');
        const registry = new AgentRegistry([agentPath], { configPath });
        (AIAgentSession as unknown as { create: (cfg: AIAgentSessionConfig) => AIAgentSession }).create = (cfg: AIAgentSessionConfig) => {
          registrySpawnSessionConfig = cfg;
          return originalCreate(cfg);
        };
        const session = await registry.spawnSession({
          agentId: path.basename(agentPath),
          userPrompt: 'Spawn session test',
          format: 'markdown',
        });
        void session;
      } finally {
        (AIAgentSession as unknown as { create: (cfg: AIAgentSessionConfig) => AIAgentSession }).create = originalCreate;
        fs.rmSync(tempDir, { recursive: true, force: true });
      }
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: { status: 'success', format: 'text', content: 'registry-spawn-session', ts: Date.now() },
      };
    },
    expect: () => {
      invariant(registrySpawnSessionConfig !== undefined, 'Spawn session config missing for run-test-96.');
      invariant(registrySpawnSessionConfig.agentId === 'spawn-parent', 'Spawn session agentId should derive from filename.');
      invariant(registrySpawnSessionConfig.outputFormat === 'markdown', 'Spawn session output format should match request.');
    },
  },
  {
    id: 'run-test-95',
    description: 'Agent registry metadata helpers and utils coverage.',
    execute: async () => {
      registryCoverageSummary = undefined;
      utilsCoverageSummary = undefined;
      await Promise.resolve();
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}registry-utils-`));
      try {
        const configPath = path.join(tempDir, CONFIG_FILE_NAME);
        const configData = {
          providers: { [PRIMARY_PROVIDER]: { type: 'test-llm' } },
          mcpServers: {},
        } satisfies Configuration;
        fs.writeFileSync(configPath, JSON.stringify(configData, null, 2), 'utf-8');
        const childPath = path.join(tempDir, 'child.ai');
        const childContent = [
          '---',
          'description: Registry child agent',
          `toolName: ${COVERAGE_CHILD_TOOL}`,
          `models:`,
          `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
          '---',
          COVERAGE_CHILD_BODY,
        ].join('\n');
        fs.writeFileSync(childPath, childContent, 'utf-8');
        const parentPath = path.join(tempDir, 'parent.ai');
        const parentContent = [
          '---',
          'description: Registry parent agent',
          `models:`,
          `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
          'agents:',
          `  - ${path.basename(childPath)}`,
          '---',
          COVERAGE_PARENT_BODY,
        ].join('\n');
        fs.writeFileSync(parentPath, parentContent, 'utf-8');
        const registry = new AgentRegistry([parentPath], { configPath });
        registry.loadFromContent(path.join(tempDir, 'inline.ai'), parentContent, { configPath, baseDir: tempDir });
        const listed = registry.list();
        const aliasMetadata = registry.getMetadata(COVERAGE_ALIAS_BASENAME);
        const toolMetadata = registry.getMetadata(COVERAGE_CHILD_TOOL);
        registryCoverageSummary = {
          listCount: listed.length,
          aliasId: aliasMetadata?.id,
          toolName: toolMetadata?.toolName,
          hasParent: registry.has(parentPath),
          hasToolAlias: registry.has(COVERAGE_CHILD_TOOL),
        };
      } finally {
        fs.rmSync(tempDir, { recursive: true, force: true });
      }
      const sanitized = sanitizeToolName('  <|pref|>Bad Name!!!  ');
      const clamped = clampToolName('truncate-me', 5);
      const formatted = formatToolRequestCompact('demoTool', { alpha: ' value with spaces ', beta: 42, gamma: [1, 2, 3], delta: { nested: true } });
      const truncated = truncateUtf8WithNotice('x'.repeat(200), 40);
      utilsCoverageSummary = {
        sanitized,
        clamped,
        formatted,
        truncated,
      };
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: { status: 'success', format: 'text', content: 'registry-utils-coverage', ts: Date.now() },
      };
    },
    expect: () => {
      invariant(registryCoverageSummary !== undefined, 'Registry summary missing for run-test-95.');
      invariant(registryCoverageSummary.listCount >= 2, 'Registry list should include parent and inline agents.');
      invariant(typeof registryCoverageSummary.aliasId === 'string' && registryCoverageSummary.aliasId.includes('parent'), 'Alias should resolve to parent id.');
      invariant(registryCoverageSummary.toolName === COVERAGE_CHILD_TOOL, 'Tool metadata should expose child tool name.');
      invariant(registryCoverageSummary.hasParent, 'Registry should report parent agent present.');
      invariant(registryCoverageSummary.hasToolAlias, 'Registry should report tool alias present.');
      invariant(utilsCoverageSummary !== undefined, 'Utils summary missing for run-test-95.');
      invariant(utilsCoverageSummary.sanitized === 'Bad', 'sanitizeToolName should normalize input.');
      invariant(utilsCoverageSummary.clamped.truncated && utilsCoverageSummary.clamped.name === 'trunc', 'clampToolName should truncate and report flag.');
      invariant(utilsCoverageSummary.formatted.includes('alpha:value with spaces'), 'formatToolRequestCompact should include trimmed string values.');
      invariant(utilsCoverageSummary.formatted.includes('gamma:[3]'), 'formatToolRequestCompact should summarize arrays.');
      invariant(utilsCoverageSummary.truncated.startsWith(TRUNCATION_NOTICE), 'truncateUtf8WithNotice should prepend notice.');
    },
  },

  {
    id: 'run-test-97',
    description: 'formatAgentResultHumanReadable renders core output branches.',
    execute: async () => {
      humanReadableSummaries = undefined;
      await Promise.resolve();
      const baseResult: AIAgentResult = {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: undefined,
      };
      const successJson: AIAgentResult = {
        ...baseResult,
        finalReport: { status: 'success', format: 'json', content_json: { foo: 'bar' }, ts: Date.now() },
      };
      const successText: AIAgentResult = {
        ...baseResult,
        finalReport: { status: 'success', format: 'markdown', content: 'Text body', ts: Date.now() },
      };
      const successNoOutput: AIAgentResult = {
        ...baseResult,
        conversation: [
          { role: 'assistant', content: '', toolCalls: [{ id: 'call-1', name: 'demoTool', parameters: {} }] },
          { role: 'tool', toolCallId: 'call-1', content: 'tool output' },
        ],
        accounting: [
          {
            type: 'llm',
            provider: PRIMARY_PROVIDER,
            model: MODEL_NAME,
            tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
            latency: 12,
            status: 'ok',
            timestamp: Date.now(),
          },
        ],
        finalReport: undefined,
      };
      const failure: AIAgentResult = {
        ...baseResult,
        success: false,
        error: 'Failure reason',
        finalReport: undefined,
      };
      humanReadableSummaries = {
        json: formatAgentResultHumanReadable(successJson),
        text: formatAgentResultHumanReadable(successText),
        successNoOutput: formatAgentResultHumanReadable(successNoOutput),
        failure: formatAgentResultHumanReadable(failure),
        truncatedZero: truncateUtf8WithNotice('ignored', 0),
      };
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: { status: 'success', format: 'text', content: 'format-agent-result', ts: Date.now() },
      };
    },
    expect: () => {
      invariant(humanReadableSummaries !== undefined, 'Human readable summaries missing for run-test-97.');
      invariant(humanReadableSummaries.json === '{\n  "foo": "bar"\n}', 'JSON final report should be pretty printed.');
      invariant(humanReadableSummaries.text === 'Text body', 'Text final report should return raw content.');
      invariant(humanReadableSummaries.successNoOutput.startsWith('AGENT COMPLETED WITHOUT OUTPUT'), 'Success without output should include completion banner.');
      invariant(humanReadableSummaries.failure.includes('AGENT FAILED') && humanReadableSummaries.failure.includes('Failure reason'), 'Failure output should include banner and reason.');
      invariant(humanReadableSummaries.truncatedZero === '', 'truncateUtf8WithNotice should return empty string when limit is zero.');
    },
  },

  {
    id: 'run-test-98',
    description: 'Configuration loading, includes, and frontmatter parsing.',
    execute: async () => {
      configLoadSummary = undefined;
      await Promise.resolve();
      includeSummary = undefined;
      frontmatterSummary = undefined;
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}config-utils-`));
      const prevApiKey = process.env.TEST_CONFIG_API_KEY;
      const prevShould = process.env.SHOULD_NOT_EXPAND;
      process.env.TEST_CONFIG_API_KEY = 'secret-123';
      process.env.SHOULD_NOT_EXPAND = 'nope';
      try {
        const configPath = path.join(tempDir, CONFIG_FILE_NAME);
        const configData = {
          providers: {
            demo: {
              type: 'test-llm',
              apiKey: '${TEST_CONFIG_API_KEY}',
              custom: { dir: '${HOME}' },
            },
          },
          mcpServers: {
            local: { type: 'local', env: { TOKEN: '${SHOULD_NOT_EXPAND}' } },
            remote: { type: 'remote', url: 'https://example.com/sse' },
          },
        } as unknown as Configuration;
        fs.writeFileSync(configPath, JSON.stringify(configData, null, 2), 'utf-8');
        const loaded = loadConfiguration(configPath);
        const demoProvider = loaded.providers.demo;
        const localServer = loaded.mcpServers.local;
        const remoteServer = loaded.mcpServers.remote;
        configLoadSummary = {
          providerKey: demoProvider.apiKey,
          localType: localServer.type,
          remoteType: remoteServer.type,
          localEnvToken: localServer.env?.TOKEN,
          defaultsStream: loaded.defaults?.stream ?? false,
        };
        // Include resolution
        const includeBaseDir = tempDir;
        const subPath = path.join(includeBaseDir, 'sub.txt');
        const nestedPath = path.join(includeBaseDir, 'nested.txt');
        fs.writeFileSync(nestedPath, 'Nested content', 'utf-8');
        fs.writeFileSync(subPath, `Sub before {{include:${path.basename(nestedPath)}}} after`, 'utf-8');
        const baseContent = `Start ${INCLUDE_DIRECTIVE_TOKEN}${path.basename(subPath)}} End`;
        const resolved = resolveIncludes(baseContent, includeBaseDir);
        let forbiddenError: string | undefined;
        let depthError: string | undefined;
        try {
          resolveIncludes(`${INCLUDE_DIRECTIVE_TOKEN}.env}`, includeBaseDir);
        } catch (e) {
          forbiddenError = e instanceof Error ? e.message : String(e);
        }
        const depthPathA = path.join(includeBaseDir, 'a.txt');
        const depthPathB = path.join(includeBaseDir, 'b.txt');
        fs.writeFileSync(depthPathA, `${INCLUDE_DIRECTIVE_TOKEN}b.txt}`, 'utf-8');
        fs.writeFileSync(depthPathB, `${INCLUDE_DIRECTIVE_TOKEN}a.txt}`, 'utf-8');
        try {
          resolveIncludes(`${INCLUDE_DIRECTIVE_TOKEN}a.txt}`, includeBaseDir, 2);
        } catch (e) {
          depthError = e instanceof Error ? e.message : String(e);
        }
        includeSummary = {
          resolved,
          forbiddenError,
          depthError,
        };
        // Frontmatter parsing
        const schemaPath = path.join(tempDir, 'schema.json');
        fs.writeFileSync(schemaPath, JSON.stringify({ type: 'object', properties: { ok: { type: 'boolean' } } }, null, 2), 'utf-8');
        const prompt = [
          '#!/usr/bin/env ai-agent',
          '---',
          'description: Demo',
          'usage: Test usage',
          'models: demo/model',
          'output:',
          `  format: json`,
          `  schemaRef: ${path.basename(schemaPath)}`,
          'input:',
          '  format: json',
          '  schema: {"type":"object"}',
          '---',
          'Body here.',
        ].join('\n');
        const parsed = parseFrontmatter(prompt, { baseDir: tempDir });
        frontmatterSummary = {
          toolName: parsed?.toolName,
          outputFormat: parsed?.expectedOutput?.format,
          inputFormat: parsed?.inputSpec?.format,
          description: parsed?.description,
          models: parsePairs(parsed?.options?.models ?? []).map((t) => `${t.provider}/${t.model}`),
        };
        // Additional parse helpers coverage
        parseList(['one', ' two ']);
        try {
          parsePairs('bad');
        } catch {
          /* expected */
        }
      } finally {
        process.env.TEST_CONFIG_API_KEY = prevApiKey;
        process.env.SHOULD_NOT_EXPAND = prevShould;
        fs.rmSync(tempDir, { recursive: true, force: true });
      }
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: { status: 'success', format: 'text', content: 'config-include-frontmatter', ts: Date.now() },
      };
    },
    expect: () => {
      invariant(configLoadSummary !== undefined, 'Config summary missing for run-test-98.');
      invariant(configLoadSummary.providerKey === 'secret-123', 'Env placeholder should expand in config.');
      invariant(configLoadSummary.localType === 'stdio', 'Local type should normalize to stdio.');
      invariant(configLoadSummary.remoteType === 'sse', 'Remote type should infer from URL.');
      invariant(configLoadSummary.localEnvToken === '${SHOULD_NOT_EXPAND}', 'Env placeholders inside MCP env should not expand.');
      invariant(includeSummary !== undefined, 'Include summary missing for run-test-98.');
      invariant(includeSummary.resolved.includes('Nested content'), 'Include resolution should inline nested content.');
      invariant(typeof includeSummary.forbiddenError === 'string' && includeSummary.forbiddenError.includes('forbidden'), 'Forbidden include error expected.');
      invariant(typeof includeSummary.depthError === 'string' && includeSummary.depthError.includes('Maximum include depth'), 'Depth limit error expected.');
      invariant(frontmatterSummary !== undefined, 'Frontmatter summary missing for run-test-98.');
      invariant(frontmatterSummary.outputFormat === 'json', 'Frontmatter output format should be json.');
      invariant(frontmatterSummary.inputFormat === 'json', 'Frontmatter input format should be json.');
      invariant(Array.isArray(frontmatterSummary.models) && frontmatterSummary.models[0] === 'demo/model', 'Frontmatter models should parse via parsePairs.');
    },
  },

{
  id: 'run-test-99',
  description: 'Configuration error handling and strict frontmatter validation.',
  execute: async () => {
    configErrorSummary = undefined;
    await Promise.resolve();
    frontmatterErrorSummary = undefined;
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}config-errors-`));
    const prevApi = process.env.TEST_CONFIG_API_KEY;
    try {
      const missingPath = path.join(tempDir, 'missing-config.json');
      let readError: string | undefined;
      try { loadConfiguration(missingPath); } catch (e) { readError = e instanceof Error ? e.message : String(e); }
      const invalidPath = path.join(tempDir, 'invalid.json');
      fs.writeFileSync(invalidPath, '{ invalid', 'utf-8');
      let jsonError: string | undefined;
      try { loadConfiguration(invalidPath); } catch (e) { jsonError = e instanceof Error ? e.message : String(e); }
      const schemaPath = path.join(tempDir, 'schema-invalid.json');
      fs.writeFileSync(schemaPath, JSON.stringify({ providers: { demo: { apiKey: 123 } }, mcpServers: {} }, null, 2), 'utf-8');
      let schemaError: string | undefined;
      try { loadConfiguration(schemaPath); } catch (e) { schemaError = e instanceof Error ? e.message : String(e); }
      process.env.TEST_CONFIG_API_KEY = 'local-secret';
      const localDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}local-config-`));
      const localConfig = {
        providers: { demo: { type: 'test-llm', apiKey: '${TEST_CONFIG_API_KEY}' } },
        mcpServers: {},
      } as unknown as Configuration;
      fs.writeFileSync(path.join(localDir, '.ai-agent.json'), JSON.stringify(localConfig, null, 2));
      const prevCwd = process.cwd();
      let localLoadedKey: string | undefined;
      try {
        process.chdir(localDir);
        localLoadedKey = loadConfiguration().providers.demo.apiKey;
      } finally {
        process.chdir(prevCwd);
      }
      configErrorSummary = { readError, jsonError, schemaError, localLoadedKey };
    } finally {
      process.env.TEST_CONFIG_API_KEY = prevApi;
      fs.rmSync(tempDir, { recursive: true, force: true });
    }
    const strictFrontmatter = ['---', 'traceLLM: true', '---', 'body'].join('\n');
    let strictError: string | undefined;
    try { parseFrontmatter(strictFrontmatter); } catch (e) { strictError = e instanceof Error ? e.message : String(e); }
    let relaxedParsed = false;
    const relaxed = parseFrontmatter(strictFrontmatter, { strict: false });
    relaxedParsed = relaxed !== undefined;
    frontmatterErrorSummary = { strictError, relaxedParsed };
    return {
      success: true,
      conversation: [],
      logs: [],
      accounting: [],
      finalReport: { status: 'success', format: 'text', content: 'config-errors-frontmatter', ts: Date.now() },
    };
  },
  expect: () => {
    invariant(configErrorSummary !== undefined, 'Config errors summary missing for run-test-99.');
    invariant(typeof configErrorSummary.readError === 'string' && configErrorSummary.readError.includes('not found'), 'Read error should report missing file.');
    invariant(typeof configErrorSummary.jsonError === 'string' && configErrorSummary.jsonError.includes('Invalid JSON'), 'JSON error expected.');
    invariant(typeof configErrorSummary.schemaError === 'string' && configErrorSummary.schemaError.includes('Configuration validation failed'), 'Schema validation error expected.');
    invariant(configErrorSummary.localLoadedKey === 'local-secret', 'Local fallback config should load expanded key.');
    invariant(frontmatterErrorSummary !== undefined, 'Frontmatter error summary missing for run-test-99.');
    const fmSummary = frontmatterErrorSummary;
    invariant(typeof fmSummary.strictError === 'string' && fmSummary.strictError.includes('Unsupported frontmatter key'), 'Strict mode should reject forbidden keys.');
    invariant(fmSummary.relaxedParsed === true, 'Relaxed parsing should not throw.');
  },
},

{
  id: 'run-test-100',
  description: 'Async include staging and repeated normalize coverage.',
  execute: async () => {
    includeAsyncSummary = undefined;
    await Promise.resolve();
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}include-async-`));
    try {
      const stage1 = path.join(tempDir, 'stage1.txt');
      const stage2 = path.join(tempDir, 'stage2.txt');
      const stage3 = path.join(tempDir, 'stage3.txt');
      const stage3Name = path.basename(stage3);
      const stage2Name = path.basename(stage2);
      const stage1Name = path.basename(stage1);
      fs.writeFileSync(stage3, 'Stage3 final', 'utf-8');
      fs.writeFileSync(stage2, `Mid ${INCLUDE_DIRECTIVE_TOKEN}${stage3Name}}`, 'utf-8');
      fs.writeFileSync(stage1, `Top ${INCLUDE_DIRECTIVE_TOKEN}${stage2Name}}`, 'utf-8');
      const resolved = resolveIncludes(`${INCLUDE_DIRECTIVE_TOKEN}${stage1Name}}`, tempDir);
      includeAsyncSummary = { staged: resolved, depth: 3 };
      const configPath = path.join(tempDir, CONFIG_FILE_NAME);
      fs.writeFileSync(configPath, JSON.stringify({ providers: {}, mcpServers: {} }), 'utf-8');
      loadConfiguration(configPath);
      loadConfiguration(configPath);
    } finally {
      fs.rmSync(tempDir, { recursive: true, force: true });
    }
    return {
      success: true,
      conversation: [],
      logs: [],
      accounting: [],
      finalReport: { status: 'success', format: 'text', content: 'include-async', ts: Date.now() },
    };
  },
  expect: () => {
    invariant(includeAsyncSummary !== undefined, 'Include async summary missing for run-test-100.');
    invariant(includeAsyncSummary.staged.includes('Stage3 final'), 'Include resolution should stage deepest content.');
    invariant(includeAsyncSummary.depth === 3, 'Include depth should reflect nested stages.');
  },
},
  (() => {
    return {
      id: 'run-test-74',
      description: 'Final report tool error is surfaced to the model.',
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'Scenario run-test-74 expected success.');
        const toolMessage = result.conversation.find((message) => {
          return message.role === 'tool' && message.toolCallId === 'call-invalid-final-report';
        });
        invariant(toolMessage !== undefined, 'Final report tool response missing for run-test-74.');
        const content = typeof toolMessage.content === 'string' ? toolMessage.content : '';
        invariant(
          content.toLowerCase().includes('requires non-empty report_content'),
          'Final report validation error should be surfaced in tool response for run-test-74.'
        );
        const finalReport = result.finalReport;
        invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed after retry for run-test-74.');
      },
    };
  })(),
  {
    id: 'run-test-78',
    description: 'Stop reason aggregated into summary logs.',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-78 should complete successfully.');
      const summaryLog = result.logs.find((entry) => entry.severity === 'FIN' && entry.type === 'llm' && entry.remoteIdentifier === 'summary');
      invariant(summaryLog !== undefined, 'LLM summary log missing for run-test-78.');
      invariant(summaryLog.message.includes('stop reasons: max_tokens'), 'Summary log should include max_tokens stop reason for run-test-78.');
    },
  },
  {
    id: 'run-test-79',
    description: 'Agent without input spec uses fallback schema.',
    execute: () => {
      const configuration: Configuration = makeBasicConfiguration();
      const layers = buildInMemoryConfigLayers(configuration);
      const loaded = loadAgent(AGENT_SCHEMA_DEFAULT_PROMPT, undefined, { configLayers: layers });
      invariant(loaded.expectedOutput === undefined, 'Default agent should not define expected output.');
      const schemaJson = JSON.stringify(loaded.input.schema);
      invariant(schemaJson === JSON.stringify(DEFAULT_TOOL_INPUT_SCHEMA), 'Default agent input schema mismatch.');
      return Promise.resolve(makeSuccessResult('agent-default-schema'));
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-79 expected success.');
    },
  },
  {
    id: 'run-test-80',
    description: 'Agent explicit JSON schema and output are preserved and cloned.',
    execute: () => {
      const configuration: Configuration = makeBasicConfiguration();
      const layers = buildInMemoryConfigLayers(configuration);
      const raw = fs.readFileSync(AGENT_SCHEMA_JSON_PROMPT, 'utf-8');
      const fm = parseFrontmatter(raw, { baseDir: path.dirname(AGENT_SCHEMA_JSON_PROMPT) });
      const inputSchemaOriginal = expectRecord(fm?.inputSpec?.schema, 'Explicit agent input schema missing.');
      const outputSchemaOriginal = expectRecord(fm?.expectedOutput?.schema, 'Explicit agent expected output schema missing.');
      const expectedInputSchemaJson = JSON.stringify(inputSchemaOriginal);
      const expectedOutputSchemaJson = JSON.stringify(outputSchemaOriginal);
      const loaded = loadAgent(AGENT_SCHEMA_JSON_PROMPT, undefined, { configLayers: layers });
      invariant(loaded.input.format === 'json', 'Explicit agent input format should be json.');
      invariant(JSON.stringify(loaded.input.schema) === expectedInputSchemaJson, 'Explicit agent input schema altered during load.');
      const explicitOutput = loaded.expectedOutput;
      invariant(explicitOutput !== undefined, 'Explicit agent should define expected output schema.');
      invariant(JSON.stringify(explicitOutput.schema) === expectedOutputSchemaJson, 'Explicit agent output schema altered during load.');
      inputSchemaOriginal.properties = {};
      outputSchemaOriginal.properties = {};
      invariant(JSON.stringify(loaded.input.schema) === expectedInputSchemaJson, 'Loaded input schema should remain isolated from frontmatter mutations.');
      invariant(JSON.stringify(explicitOutput.schema) === expectedOutputSchemaJson, 'Loaded output schema should remain isolated from frontmatter mutations.');
      return Promise.resolve(makeSuccessResult('agent-explicit-json-schema'));
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-80 expected success.');
    },
  },
  {
    id: 'run-test-81',
    description: 'Fallback sub-agent schema enforces prompt, reason, and sub-agent format.',
    execute: async (): Promise<AIAgentResult> => {
      const configuration: Configuration = makeBasicConfiguration();
      const subAgent = preloadSubAgentFromPath(SUBAGENT_SCHEMA_FALLBACK_PROMPT, configuration);
      invariant(!subAgent.hasExplicitInputSchema, 'Fallback sub-agent should not report explicit schema.');
      const registry = new SubAgentRegistry([subAgent], []);
      const tools = registry.getTools();
      invariant(tools.length > 0, 'Fallback sub-agent tool missing.');
      const [tool] = tools;
      const schema = expectRecord(tool.inputSchema, 'Fallback sub-agent schema missing.');
      const propsContainer = expectRecord((schema as { properties?: unknown }).properties ?? {}, 'Fallback schema properties missing.');
      const formatDetails = expectRecord((propsContainer as { format?: unknown }).format, 'Fallback schema should expose format property.');
      const enumValues = formatDetails.enum;
      invariant(Array.isArray(enumValues) && enumValues.length === 1 && enumValues[0] === 'sub-agent', 'Fallback schema format enum should only contain sub-agent.');
      invariant(formatDetails.default === 'sub-agent', 'Fallback schema format default must be sub-agent.');
      const reasonDetails = expectRecord((propsContainer as { reason?: unknown }).reason, 'Fallback schema reason property missing.');
      invariant(reasonDetails.minLength === 1, 'Fallback schema reason must enforce minLength.');
      const requiredRaw = (schema as { required?: unknown }).required;
      const required = new Set<string>(Array.isArray(requiredRaw) ? requiredRaw.filter((value): value is string => typeof value === 'string') : []);
      invariant(required.has('prompt'), 'Fallback schema should require prompt.');
      invariant(required.has('reason'), 'Fallback schema should require reason.');
      invariant(required.has('format'), 'Fallback schema should require format.');
      const parentSession = createParentSessionStub(configuration);
      const ok = await registry.execute(tool.name, { prompt: 'Fallback prompt', reason: 'Fallback title', format: 'sub-agent' }, parentSession);
      invariant(typeof ok.result === 'string', 'Fallback sub-agent execution should return string result.');

      const expectFailure = async (args: Record<string, unknown>, fragment: string): Promise<void> => {
        let failed = false;
        try {
          await registry.execute(tool.name, args, parentSession);
        } catch (error) {
          failed = true;
          invariant(String(error).includes(fragment), `Failure message should mention ${fragment}`);
        }
        invariant(failed, 'Execution should have failed.');
      };

      await expectFailure({ reason: 'Missing prompt', format: 'sub-agent' }, 'prompt');
      await expectFailure({ prompt: 'Missing reason', format: 'sub-agent' }, 'reason');
      await expectFailure({ prompt: 'Wrong format', reason: 'Bad format', format: 'markdown' }, 'format');

      return makeSuccessResult('subagent-fallback-schema');
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-81 expected success.');
    },
  },
  {
    id: 'run-test-82',
    description: 'Explicit JSON sub-agent schema injects reason and enforces provided structure.',
    execute: async (): Promise<AIAgentResult> => {
      const configuration: Configuration = makeBasicConfiguration();
      const raw = fs.readFileSync(SUBAGENT_SCHEMA_JSON_PROMPT, 'utf-8');
      const fm = parseFrontmatter(raw, { baseDir: path.dirname(SUBAGENT_SCHEMA_JSON_PROMPT) });
      const explicitInputSchema = expectRecord(fm?.inputSpec?.schema, 'Explicit sub-agent input schema missing.');
      const expectedOutputOriginal = expectRecord(fm?.expectedOutput?.schema, 'Explicit sub-agent expected output schema missing.');
      const expectedOutputSchemaJson = JSON.stringify(expectedOutputOriginal);
      const subAgent = preloadSubAgentFromPath(SUBAGENT_SCHEMA_JSON_PROMPT, configuration);
      invariant(subAgent.hasExplicitInputSchema, 'Explicit sub-agent should report explicit schema.');
      const registry = new SubAgentRegistry([subAgent], []);
      const tools = registry.getTools();
      invariant(tools.length > 0, 'Explicit sub-agent tool missing.');
      const [tool] = tools;
      const schema = expectRecord(tool.inputSchema, 'Explicit sub-agent schema missing.');
      const propsContainer = expectRecord((schema as { properties?: unknown }).properties ?? {}, 'Explicit sub-agent properties missing.');
      const reasonDetails = expectRecord((propsContainer as { reason?: unknown }).reason, 'Explicit sub-agent schema should include reason property.');
      invariant(reasonDetails.minLength === 1, 'Explicit sub-agent reason must enforce minLength.');
      const requiredRaw = (schema as { required?: unknown }).required;
      const required = new Set<string>(Array.isArray(requiredRaw) ? requiredRaw.filter((value): value is string => typeof value === 'string') : []);
      invariant(required.has('reason'), 'Explicit sub-agent schema should require reason.');
      invariant(propsContainer.query !== undefined, 'Explicit sub-agent schema should preserve query property.');
      const schemaJsonBeforeMutation = JSON.stringify(schema);

      const parentSession = createParentSessionStub(configuration);
      const ok = await registry.execute(tool.name, { query: 'topic', limit: 2, reason: 'Explicit title' }, parentSession);
      invariant(typeof ok.result === 'string', 'Explicit sub-agent execution should succeed.');

      const expectFailure = async (args: Record<string, unknown>, fragment: string): Promise<void> => {
        let failed = false;
        try {
          await registry.execute(tool.name, args, parentSession);
        } catch (error) {
          failed = true;
          invariant(String(error).includes(fragment), `Failure message should mention ${fragment}`);
        }
        invariant(failed, 'Execution should have failed.');
      };

      await expectFailure({ query: 'missing reason' }, 'reason');
      await expectFailure({ reason: 'missing query' }, 'query');

      const explicitOutput = subAgent.loaded.expectedOutput;
      invariant(explicitOutput !== undefined, 'Explicit sub-agent should define expected output schema.');
      invariant(JSON.stringify(explicitOutput.schema) === expectedOutputSchemaJson, 'Explicit sub-agent output schema mismatch.');
      expectedOutputOriginal.properties = {};
      explicitInputSchema.properties = {};
      invariant(JSON.stringify(schema) === schemaJsonBeforeMutation, 'Explicit sub-agent input schema should remain isolated from frontmatter mutations.');
      invariant(JSON.stringify(explicitOutput.schema) === expectedOutputSchemaJson, 'Explicit sub-agent output schema should remain isolated from frontmatter mutations.');

      return makeSuccessResult('subagent-explicit-json-schema');
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-82 expected success.');
    },
  },
  {
    id: 'run-test-83',
    description: 'LLM client error mapping via executeTurn.',
    execute: async (): Promise<AIAgentResult> => {
      const scenarioCases = [
        { key: 'auth', scenario: 'run-test-83-auth' },
        { key: 'quota', scenario: 'run-test-83-quota' },
        { key: 'rate', scenario: 'run-test-83-rate' },
        { key: 'timeout', scenario: 'run-test-83-timeout' },
        { key: 'network', scenario: 'run-test-83-network' },
        { key: 'model', scenario: 'run-test-83-model' },
      ] as const;
      const statusSummary: Record<string, TurnStatus> = {};
      // eslint-disable-next-line functional/no-loop-statements
      for (const { key, scenario } of scenarioCases) {
        const client = new LLMClient({ [PRIMARY_PROVIDER]: { type: 'test-llm' } });
        client.setTurn(1, 0);
        const result = await client.executeTurn({
          provider: PRIMARY_PROVIDER,
          model: MODEL_NAME,
          messages: [
            { role: 'system', content: 'Phase 1 deterministic harness' },
            { role: 'user', content: scenario },
          ],
          tools: [],
          toolExecutor: () => Promise.resolve(''),
        });
        statusSummary[key] = result.status;
      }
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'json',
          content_json: statusSummary,
          ts: Date.now(),
        },
      };
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-83 expected success.');
      const data = result.finalReport?.content_json as Record<string, TurnStatus> | undefined;
      invariant(data !== undefined, 'Status summary expected for run-test-83.');
      const auth = data.auth as TurnStatus | undefined;
      invariant(auth !== undefined && auth.type === 'auth_error' && typeof auth.message === 'string' && auth.message.toLowerCase().includes('invalid api key'), 'Auth error mapping mismatch for run-test-83.');
      const quota = data.quota as TurnStatus | undefined;
      invariant(quota !== undefined && quota.type === 'quota_exceeded' && typeof quota.message === 'string' && quota.message.toLowerCase().includes('quota'), 'Quota error mapping mismatch for run-test-83.');
      const rate = data.rate as TurnStatus | undefined;
      invariant(rate !== undefined && rate.type === 'rate_limit', 'Rate limit mapping mismatch for run-test-83.');
      invariant(rate.retryAfterMs === 3000, 'Rate limit retryAfterMs mismatch for run-test-83.');
      const timeout = data.timeout as TurnStatus | undefined;
      invariant(timeout !== undefined && timeout.type === 'timeout' && typeof timeout.message === 'string', 'Timeout mapping mismatch for run-test-83.');
      const network = data.network as TurnStatus | undefined;
      const networkRetryable = (network as { retryable?: boolean } | undefined)?.retryable ?? true;
      invariant(network !== undefined && network.type === 'network_error' && networkRetryable, 'Network mapping mismatch for run-test-83.');
      const model = data.model as TurnStatus | undefined;
      invariant(model !== undefined && model.type === 'model_error' && typeof model.message === 'string', 'Model error mapping mismatch for run-test-83.');
    },
  },
  {
    id: 'run-test-84',
    description: 'Final turn without final report retries next provider.',
    configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      configuration.providers = {
        ...configuration.providers,
        [SECONDARY_PROVIDER]: { type: 'test-llm' },
      };
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: `${MODEL_NAME}-primary` },
        { provider: SECONDARY_PROVIDER, model: `${MODEL_NAME}-secondary` },
      ];
      sessionConfig.maxTurns = 1;
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          return {
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 5,
            messages: [
              { role: 'assistant', content: 'Continuing work...' },
            ],
            tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
          };
        }
        if (invocation === 2) {
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'success',
              format: 'markdown',
              content: FINAL_ANSWER_DELIVERED,
            };
          }
          const assistantMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: FINAL_ANSWER_DELIVERED,
                },
              },
            ],
          };
          const toolMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: FINAL_ANSWER_DELIVERED,
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage as ConversationMessage, toolMessage as ConversationMessage],
            tokens: { inputTokens: 12, outputTokens: 8, totalTokens: 20 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
          activeSession = AIAgentSession.create(sessionConfig);
          return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-84 expected success.');
      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:final-turn');
      if (finalTurnLog === undefined) {
        // eslint-disable-next-line no-console
        console.error('run-test-84 logs:', result.logs.map((entry) => ({ id: entry.remoteIdentifier, severity: entry.severity, message: entry.message })));
      }
      invariant(finalTurnLog !== undefined, 'Final-turn warning log expected for run-test-84.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed for run-test-84.');
    },
  },
  {
    id: 'run-test-85',
    description: 'Final report JSON schema mismatch surfaces payload preview.',
    configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.expectedOutput = {
        format: 'json',
        schema: {
          type: 'object',
          required: ['extracted_info'],
          additionalProperties: false,
          properties: {
            extracted_info: { type: 'string' },
          },
        },
      };
      sessionConfig.maxTurns = 1;
      sessionConfig.outputFormat = 'json';
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          return {
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 5,
            messages: [
              { role: 'assistant', content: 'Gathering data...' },
            ],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        if (invocation === 2) {
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'json'; content_json: Record<string, unknown> } }).finalReport = {
              status: 'success',
              format: 'json',
              content_json: { extracted_info: { quote: 'answer' } },
            };
          }
          const assistantMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'success',
                  report_format: 'json',
                  report_content: JSON.stringify({ extracted_info: { quote: 'answer' } }),
                },
              },
            ],
          };
          const toolMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: JSON.stringify({ extracted_info: { quote: 'answer' } }),
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage as ConversationMessage, toolMessage as ConversationMessage],
            tokens: { inputTokens: 9, outputTokens: 6, totalTokens: 15 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-85 expected success.');
      const ajvLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:ajv');
      invariant(ajvLog !== undefined, 'AJV warning expected for run-test-85.');
      invariant(typeof ajvLog.message === 'string' && ajvLog.message.includes('payload preview='), 'Payload preview missing in AJV warning for run-test-85.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.format === 'json', 'Final report should be json for run-test-85.');
    },
  },
  {
    id: 'run-test-86',
    description: 'Final turn tool filtering retains only agent__final_report.',
    configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
      sessionConfig.maxTurns = 2;
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      const observed: { isFinalTurn: boolean; input: string[]; output: string[] }[] = [];
      const proto = TestLLMProvider.prototype as unknown as {
        filterToolsForFinalTurn: (tools: MCPTool[], isFinalTurn?: boolean) => MCPTool[];
      };
      const originalFilter = proto.filterToolsForFinalTurn;
      proto.filterToolsForFinalTurn = function(this: TestLLMProvider, tools: MCPTool[], isFinalTurn?: boolean): MCPTool[] {
        const result = originalFilter.call(this, tools, isFinalTurn);
        observed.push({
          isFinalTurn: isFinalTurn === true,
          input: tools.map((tool) => tool.name),
          output: result.map((tool) => tool.name),
        });
        return result;
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        const result = await session.run();
        (result as { __observedTools?: { isFinalTurn: boolean; input: string[]; output: string[] }[] }).__observedTools = observed;
        return result;
      } finally {
        proto.filterToolsForFinalTurn = originalFilter;
      }
    },
    expect: (result: AIAgentResult & { __observedTools?: { isFinalTurn: boolean; input: string[]; output: string[] }[] }) => {
      invariant(result.success, 'Scenario run-test-86 expected success.');
      const observed = Array.isArray(result.__observedTools) ? result.__observedTools : [];
      const finalTurnObservation = observed.find((entry) => entry.isFinalTurn);
      invariant(finalTurnObservation !== undefined, 'Final turn observation missing for run-test-86.');
      invariant(finalTurnObservation.output.length === 1 && finalTurnObservation.output[0] === 'agent__final_report', 'Final turn tools not filtered to agent__final_report.');
      invariant(finalTurnObservation.input.some((tool) => tool !== 'agent__final_report'), 'Final turn input should include additional tools prior to filtering.');
      const nonFinalObservation = observed.find((entry) => !entry.isFinalTurn);
      invariant(nonFinalObservation !== undefined, 'Non-final turn observation missing for run-test-86.');
      invariant(nonFinalObservation.output.length > 1, 'Non-final turn should retain multiple tools.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed for run-test-86.');
    },
  },
  {
    id: 'run-test-87',
    description: 'Final report failure status is propagated.',
    configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          return {
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 5,
            messages: [
              { role: 'assistant', content: 'Assessing data reliability...' },
            ],
            tokens: { inputTokens: 7, outputTokens: 3, totalTokens: 10 },
          };
        }
        if (invocation === 2) {
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'failure',
              format: 'markdown',
              content: 'Investigation failed: upstream service unavailable.',
            };
          }
          const failureContent = 'Investigation failed: upstream service unavailable.';
          const assistantMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'failure',
                  report_format: 'markdown',
                  report_content: failureContent,
                },
              },
            ],
          };
          const toolMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: failureContent,
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage as ConversationMessage, toolMessage as ConversationMessage],
            tokens: { inputTokens: 9, outputTokens: 6, totalTokens: 15 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-87 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-87.');
      invariant(finalReport.status === 'failure', 'Final report status should be failure for run-test-87.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('upstream service unavailable'), 'Final report content mismatch for run-test-87.');
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined, 'EXIT-FINAL-ANSWER log missing for run-test-87.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && message.toolCallId === FINAL_REPORT_CALL_ID);
      invariant(toolMessage !== undefined, 'Tool response missing for run-test-87.');
    },
  },
  {
    id: 'run-test-88',
    description: 'Final report partial status is propagated.',
    configure: (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          return {
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 5,
            messages: [
              { role: 'assistant', content: 'Summarizing current findings...' },
            ],
            tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
          };
        }
        if (invocation === 2) {
          const partialContent = 'Partial success: gathered overview, missing detailed metrics.';
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'partial',
              format: 'markdown',
              content: partialContent,
            };
          }
          const assistantMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'partial',
                  report_format: 'markdown',
                  report_content: partialContent,
                },
              },
            ],
          };
          const toolMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: partialContent,
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage as ConversationMessage, toolMessage as ConversationMessage],
            tokens: { inputTokens: 8, outputTokens: 5, totalTokens: 13 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-88 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-88.');
      invariant(finalReport.status === 'partial', 'Final report status should be partial for run-test-88.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Partial success'), 'Final report content mismatch for run-test-88.');
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined, 'EXIT-FINAL-ANSWER log missing for run-test-88.');
    },
  },
  {
    id: 'run-test-89',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-89 expected success.');
      const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === SANITIZER_REMOTE_IDENTIFIER);
      invariant(sanitizerLog !== undefined, 'Sanitizer log expected for run-test-89.');
      invariant(
        typeof sanitizerLog.message === 'string' && sanitizerLog.message.includes('Dropped 1 invalid tool call'),
        'Sanitizer log should report dropped tool call for run-test-89.'
      );

      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      invariant(firstAssistant !== undefined, 'Missing assistant message for run-test-89.');
      const sanitizedCalls = firstAssistant.toolCalls ?? [];
      invariant(sanitizedCalls.length === 1, 'Exactly one tool call should remain after sanitization for run-test-89.');
      const retainedCall = sanitizedCalls[0];
      invariant(retainedCall.name === 'test__test', 'Retained tool call name mismatch for run-test-89.');
      const params = retainedCall.parameters;
      const textValue = (params as { text?: unknown }).text;
      invariant(typeof textValue === 'string', 'Retained tool call should include text field for run-test-89.');
      invariant(textValue === SANITIZER_VALID_ARGUMENT, 'Sanitized tool call payload mismatch for run-test-89.');
    },
  },
  {
    id: 'run-test-90',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'test__test',
                id: 'call-invalid-only',
                parameters: 'malformed' as unknown as Record<string, unknown>,
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 10, outputTokens: 4, totalTokens: 14 },
          };
        }
        if (invocation === 2) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'test__test',
                id: 'call-valid-after-sanitizer',
                parameters: { text: SANITIZER_VALID_ARGUMENT },
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 },
          };
        }
        if (invocation === 3) {
          const finalContent = 'Final report produced after sanitizer retry.';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: finalContent,
                },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: finalContent,
          };
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'success',
              format: 'markdown',
              content: finalContent,
            };
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 14, outputTokens: 8, totalTokens: 22 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-90 expected success.');
      const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === SANITIZER_REMOTE_IDENTIFIER);
      invariant(sanitizerLog !== undefined, 'Sanitizer log expected for run-test-90.');
      invariant(
        typeof sanitizerLog.message === 'string' && (
          sanitizerLog.message.includes('Invalid tool call dropped') ||
          sanitizerLog.message.includes('Dropped 1 invalid tool call')
        ),
        'Sanitizer message mismatch for run-test-90.',
      );
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length === 2, 'Two assistant messages expected after retry for run-test-90.');
      const firstAssistant = assistantMessages[0];
      invariant(firstAssistant.toolCalls !== undefined && firstAssistant.toolCalls.length === 1, 'Single sanitized tool call expected for run-test-90.');
      const retainedCall = firstAssistant.toolCalls[0];
      invariant(retainedCall.name === 'test__test', 'Retained tool call name mismatch for run-test-90.');
      const textValue = (retainedCall.parameters as { text?: unknown }).text;
      invariant(textValue === SANITIZER_VALID_ARGUMENT, 'Retained tool call arguments mismatch for run-test-90.');
      const systemMessages = result.conversation.filter((message) => message.role === 'system');
      invariant(
        systemMessages.some((message) => typeof message.content === 'string' && message.content.includes('invalid tool-call arguments')),
        'Retry system notice missing for run-test-90.'
      );
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts === 3, 'Three LLM attempts expected for run-test-90.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed after sanitizer retry (run-test-90).');
    },
  },
  {
    id: 'run-test-90-string',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'test__test',
                id: 'call-json-string',
                parameters: JSON.stringify({ text: SANITIZER_VALID_ARGUMENT }) as unknown as Record<string, unknown>,
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 9, outputTokens: 4, totalTokens: 13 },
          };
        }
        if (invocation === 2) {
          const finalContent = 'Final report after string arguments.';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: finalContent,
                },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: finalContent,
          };
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'success',
              format: 'markdown',
              content: finalContent,
            };
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-90-string expected success.');
      const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === SANITIZER_REMOTE_IDENTIFIER);
      invariant(sanitizerLog === undefined, 'No sanitizer warning expected for run-test-90-string.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length >= 1, 'Assistant message expected for run-test-90-string.');
      const firstAssistant = assistantMessages[0];
      invariant(firstAssistant.toolCalls !== undefined && firstAssistant.toolCalls.length === 1, 'Single tool call expected for run-test-90-string.');
      const firstCall = firstAssistant.toolCalls[0];
      invariant(firstCall.name === 'test__test', 'Tool call name mismatch for run-test-90-string.');
      const textValue = (firstCall.parameters as { text?: unknown }).text;
      invariant(textValue === SANITIZER_VALID_ARGUMENT, 'Parsed tool call arguments mismatch for run-test-90-string.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts === 2, 'Two LLM attempts expected for run-test-90-string.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed for run-test-90-string.');
    },
  },
  {
    id: 'run-test-90-rate-limit',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      let activeSession: AIAgentSession | undefined;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          return {
            status: { type: 'rate_limit', retryAfterMs: 1500 },
            latencyMs: 5,
            messages: [],
            tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
          };
        }
        if (invocation === 2) {
          const finalContent = RATE_LIMIT_FINAL_CONTENT;
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: FINAL_REPORT_CALL_ID,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: finalContent,
                },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: finalContent,
          };
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'success',
              format: 'markdown',
              content: finalContent,
            };
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        activeSession = AIAgentSession.create(sessionConfig);
        return await activeSession.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-90-rate-limit expected success.');
      const systemMessages = result.conversation.filter((message) => message.role === 'system');
      invariant(
        systemMessages.some((message) => typeof message.content === 'string' && message.content.includes('rate-limited the previous request')),
        'Rate-limit system notice missing for run-test-90-rate-limit.'
      );
      const attempts = result.accounting.filter(isLlmAccounting).length;
      invariant(attempts === 2, 'Two LLM attempts expected for run-test-90-rate-limit.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed for run-test-90-rate-limit.');
    },
  },
  {
    id: 'run-test-90-adopt-text',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const finalContent = ADOPTED_FINAL_CONTENT;
          const payload = {
            status: 'success',
            report_format: 'markdown',
            report_content: finalContent,
            metadata: { origin: ADOPTION_METADATA_ORIGIN },
            content_json: { key: ADOPTION_CONTENT_VALUE },
          };
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: `Here is the summary:\n${JSON.stringify(payload)}`,
          };
          return {
            status: { type: 'success', hasToolCalls: false, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 },
          };
        }
        return await originalExecuteTurn.call(this, request);
      };
      try {
        return await AIAgentSession.create(sessionConfig).run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-90-adopt-text expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-90-adopt-text.');
      invariant(finalReport.status === 'success', 'Final report status mismatch for run-test-90-adopt-text.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-90-adopt-text.');
      invariant(finalReport.content === ADOPTED_FINAL_CONTENT, 'Final report content mismatch for run-test-90-adopt-text.');
      invariant(finalReport.metadata !== undefined && finalReport.metadata.origin === ADOPTION_METADATA_ORIGIN, 'Final report metadata missing for run-test-90-adopt-text.');
      invariant(finalReport.content_json !== undefined && finalReport.content_json.key === ADOPTION_CONTENT_VALUE, 'Final report content_json missing for run-test-90-adopt-text.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length === 1, 'Single assistant message expected for run-test-90-adopt-text.');
      const adoptedCall = assistantMessages[0].toolCalls;
      invariant(adoptedCall !== undefined && adoptedCall.length === 1, 'Synthetic final report tool call missing for run-test-90-adopt-text.');
      invariant(adoptedCall[0].name === 'agent__final_report', 'Synthetic tool call name mismatch for run-test-90-adopt-text.');
      const adoptedParams = adoptedCall[0].parameters as { report_format?: unknown; metadata?: { origin?: string }; content_json?: { key?: string } };
      invariant(adoptedParams.report_format === 'markdown', 'Synthetic call report_format mismatch for run-test-90-adopt-text.');
      invariant(adoptedParams.metadata !== undefined && adoptedParams.metadata.origin === ADOPTION_METADATA_ORIGIN, 'Synthetic call metadata mismatch for run-test-90-adopt-text.');
      invariant(adoptedParams.content_json !== undefined && adoptedParams.content_json.key === ADOPTION_CONTENT_VALUE, 'Synthetic call content_json mismatch for run-test-90-adopt-text.');
    },
  },
  {
    id: 'run-test-90-json-content',
    description: 'Adopts final report when assistant returns JSON-only payload in content.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      await Promise.resolve();
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
        const payload = {
          status: 'success',
          report_format: 'json',
          content_json: {
            status: true,
            url: JSON_ONLY_URL,
            notes: ['entry-1', 'entry-2'],
          },
          metadata: {
            adopted: true,
          },
        };
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: JSON.stringify(payload, null, 2),
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 9, outputTokens: 9, totalTokens: 18 },
        });
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-90-json-content expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-90-json-content.');
      invariant(finalReport.status === 'success', 'Final report status mismatch for run-test-90-json-content.');
      invariant(finalReport.format === 'json', 'Final report format mismatch for run-test-90-json-content.');
      invariant(finalReport.content !== undefined, 'Final report content missing for run-test-90-json-content.');
      let parsedContent: Record<string, unknown> | undefined;
      try {
        parsedContent = JSON.parse(finalReport.content) as Record<string, unknown>;
      } catch {
        invariant(false, 'Final report content should be valid JSON for run-test-90-json-content.');
      }
      assertRecord(parsedContent, 'Parsed final report content should be an object for run-test-90-json-content.');
      invariant(parsedContent.status === true, 'Parsed final report content status mismatch for run-test-90-json-content.');
      invariant(parsedContent.url === JSON_ONLY_URL, 'Parsed final report content url mismatch for run-test-90-json-content.');
      invariant(Array.isArray(parsedContent.notes) && parsedContent.notes.length === 2, 'Parsed final report notes mismatch for run-test-90-json-content.');
      const frContentJson = finalReport.content_json;
      assertRecord(frContentJson, 'Final report content_json should be an object for run-test-90-json-content.');
      invariant(frContentJson.status === true, 'Final report content_json status mismatch for run-test-90-json-content.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length === 1, 'Single assistant message expected for run-test-90-json-content.');
      const syntheticCall = assistantMessages[0].toolCalls ?? [];
      invariant(syntheticCall.length === 1, 'Synthetic final report tool call missing for run-test-90-json-content.');
      const call = syntheticCall[0];
      invariant(call.name === 'agent__final_report', 'Synthetic tool call name mismatch for run-test-90-json-content.');
      const paramsUnknown = call.parameters;
      assertRecord(paramsUnknown, 'Synthetic call parameters missing for run-test-90-json-content.');
      const params = paramsUnknown;
      invariant(params.report_format === 'json', 'Synthetic call report_format mismatch for run-test-90-json-content.');
      invariant(typeof params.report_content === 'string' && params.report_content.includes('"status":true'), 'Synthetic call report_content mismatch for run-test-90-json-content.');
      const callContentJsonUnknown = params.content_json;
      assertRecord(callContentJsonUnknown, 'Synthetic call content_json missing for run-test-90-json-content.');
      invariant(callContentJsonUnknown.url === JSON_ONLY_URL, 'Synthetic call content_json mismatch for run-test-90-json-content.');
    },
  },
  {
    id: 'run-test-90-no-retry',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        void request;
        await Promise.resolve();
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [
            {
              role: 'assistant',
              content: '',
              toolCalls: [
                {
                  name: 'agent__final_report',
                  id: FINAL_REPORT_CALL_ID,
                  parameters: {
                    status: 'success',
                    report_format: 'markdown',
                    report_content: FINAL_REPORT_SANITIZED_CONTENT,
                  },
                },
              ],
            },
            { role: 'tool', toolCallId: FINAL_REPORT_CALL_ID, content: FINAL_REPORT_SANITIZED_CONTENT },
          ],
          tokens: { inputTokens: 11, outputTokens: 7, totalTokens: 18 },
        };
      };
      try {
        return await AIAgentSession.create(sessionConfig).run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-90-no-retry expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report missing for run-test-90-no-retry.');
      invariant(finalReport.content === FINAL_REPORT_SANITIZED_CONTENT, 'Final report content mismatch for run-test-90-no-retry.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-90-no-retry.');
      const systemNotices = result.conversation
        .filter((message) => message.role === 'system' && typeof message.content === 'string' && message.content.includes('plain text responses are ignored'));
      invariant(systemNotices.length === 0, 'No retry system notice expected for run-test-90-no-retry.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts === 1, 'Single LLM attempt expected for run-test-90-no-retry.');
      const warnLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('plain text responses are ignored'));
      invariant(warnLog === undefined, 'No retry warning log expected for run-test-90-no-retry.');
    },
  },
  {
    id: 'run-test-91',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      configuration.defaults = { ...configuration.defaults, parallelToolCalls: true };
      sessionConfig.parallelToolCalls = true;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-91 expected success.');
      const batchAccountingEntries = result.accounting.filter(isToolAccounting).filter((entry) => entry.command === 'agent__batch');
      const hasInvalidBatchAccounting = batchAccountingEntries.some((entry) => typeof entry.error === 'string' && entry.error.includes('invalid_batch_input'));
      invariant(hasInvalidBatchAccounting, 'Invalid batch handling signal expected for run-test-91.');
      const unknownToolLog = result.logs.find((entry) => entry.remoteIdentifier === 'assistant:tool' && typeof entry.message === 'string' && entry.message.includes('Unknown tool requested'));
      invariant(unknownToolLog !== undefined, 'Unknown tool warning expected for run-test-91.');
      const finalReportErrorLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('agent__final_report requires non-empty report_content'));
      invariant(finalReportErrorLog !== undefined, 'Final report validation log expected for run-test-91.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      invariant(firstAssistant !== undefined, 'Assistant message missing for run-test-91.');
      const toolCalls = firstAssistant.toolCalls ?? [];
      invariant(toolCalls.length === 4, 'Four tool calls expected in mixed scenario for run-test-91.');
      const longNameCall = toolCalls.find((call) => call.id === 'call-long-tool-name');
      invariant(longNameCall !== undefined, 'Long name tool call missing for run-test-91.');
      const expectedSanitized = sanitizeToolName(LONG_TOOL_NAME);
      invariant(longNameCall.name === expectedSanitized, 'Sanitized tool name mismatch for run-test-91.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report should succeed after mixed tools for run-test-91.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes(FINAL_REPORT_RETRY_MESSAGE), 'Final report content mismatch for run-test-91.');
    },
  },
  {
    id: 'run-test-101',
    description: 'Synthesizes a failure final report when tools keep failing on the final turn.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
      sessionConfig.maxRetries = 1;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        const failureCallId = `call-fail-${String(invocation)}`;
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              name: 'fetcher__fetch_url',
              id: failureCallId,
              parameters: { url: 'https://example.com/failure', reason: 'Fetch hiring data' },
            },
          ],
        };
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: failureCallId,
          content: '(tool failed: Connection closed)',
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 25,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 12, outputTokens: 0, cachedTokens: 0, totalTokens: 12 },
        });
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-101 should complete with a synthesized final report.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report expected for run-test-101.');
      invariant(finalReport.status === 'failure', 'Synthesized final report should carry failure status for run-test-101.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('did not produce a final report'), 'Failure summary should mention the missing final report for run-test-101.');
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined && typeof exitLog.message === 'string' && exitLog.message.includes('Synthesized failure final_report'), 'Synthesized final report exit log missing for run-test-101.');
    },
  },
  {
    id: 'run-test-102',
    description: 'Fails when agent__final_report tool call itself fails on the final turn.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
      sessionConfig.maxRetries = 1;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
        const failureCallId = 'call-final-report-fail';
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: failureCallId,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'markdown',
                report_content: '# Placeholder\n\nThis should fail.',
              },
            },
          ],
        };
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: failureCallId,
          content: '(tool failed: Schema validation error: report_content missing required section)',
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 30,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 2, cachedTokens: 0, totalTokens: 12 },
        });
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        return await session.run();
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult) => {
      invariant(!result.success, 'Scenario run-test-102 should report failure when final_report tool fails.');
      invariant(result.finalReport === undefined, 'No final report expected when agent__final_report fails in run-test-102.');
      invariant(result.error === 'final_report_failed', 'Failure error should be final_report_failed for run-test-102.');
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:EXIT-NO-LLM-RESPONSE');
      invariant(exitLog !== undefined && typeof exitLog.message === 'string' && exitLog.message.includes('final_report_failed'), 'EXIT-NO-LLM-RESPONSE log with final_report_failed reason expected for run-test-102.');
      const synthesizedFallbackLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Synthesized failure final_report'));
      invariant(synthesizedFallbackLog === undefined, 'Synthesized fallback log must not appear for run-test-102.');
    },
  },
  {
    id: 'run-test-103',
    description: 'REST tools are exposed with rest__ prefix and remain discoverable without hard-coded aliases.',
    configure: (configuration, sessionConfig) => {
      configuration.restTools = {
        'ask-netdata': {
          description: 'Ask Netdata documentation assistant.',
          method: 'POST',
          url: 'https://example.com/ask-netdata',
          headers: { 'content-type': 'application/json' },
          parametersSchema: {
            type: 'object',
            properties: {
              question: { type: 'string' },
            },
            required: [],
            additionalProperties: false,
          },
          bodyTemplate: {
            question: '${parameters.question}',
          },
        },
      };
      sessionConfig.tools = ['ask-netdata'];
    },
    execute: async (_configuration, sessionConfig) => {
      await Promise.resolve();
      const session = AIAgentSession.create(sessionConfig);
      const orchestrator = getPrivateField(session, 'toolsOrchestrator') as { listTools: () => MCPTool[]; hasTool: (name: string) => boolean } | undefined;
      invariant(orchestrator !== undefined, 'Tools orchestrator missing in run-test-103.');
      const listedNames = orchestrator.listTools().map((tool) => tool.name);
      invariant(listedNames.includes('rest__ask-netdata'), 'rest__ask-netdata should be exposed in listTools for run-test-103.');
      invariant(orchestrator.hasTool('rest__ask-netdata'), 'rest__ask-netdata should resolve via hasTool for run-test-103.');
      invariant(orchestrator.hasTool('ask-netdata'), 'ask-netdata should resolve via rest__ fallback for run-test-103.');
      return makeSuccessResult('REST tool exposure validated.');
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-103 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined && finalReport.status === 'success', 'Final report missing for run-test-103.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('REST tool exposure validated.'), 'Final report content mismatch for run-test-103.');
    },
  },
  {
    id: 'run-test-43',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.stopRef = { stopping: true };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-43 should finish gracefully.');
      invariant(result.finalReport === undefined, 'No final report expected for run-test-43.');
      const finLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:fin');
      invariant(finLog !== undefined, 'Finalization log expected for run-test-43.');
    },
  },
  {
    id: 'run-test-49',
    configure: (configuration, sessionConfig) => {
      configuration.providers[SECONDARY_PROVIDER] = { type: 'test-llm' };
      const stopRef = { stopping: false };
      sessionConfig.stopRef = stopRef;
      sessionConfig.maxRetries = 2;
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: MODEL_NAME },
        { provider: SECONDARY_PROVIDER, model: MODEL_NAME },
      ];
    },
    execute: async (_configuration, sessionConfig) => {
      const stopRef = sessionConfig.stopRef ?? { stopping: false };
      sessionConfig.stopRef = stopRef;
      const session = AIAgentSession.create(sessionConfig);
      setTimeout(() => { stopRef.stopping = true; }, 10);
      return await session.run();
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-49 should honor stop during rate limit backoff.');
      invariant(result.finalReport === undefined, 'No final report expected for run-test-49.');
      const finLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:fin');
      invariant(finLog !== undefined, 'Finalization log expected for run-test-49.');
    },
  },
  {
    id: COVERAGE_OPENROUTER_JSON_ID,
    description: 'Coverage: traced fetch metadata for OpenRouter JSON responses.',
    execute: async () => {
      coverageOpenrouterJson = undefined;
      const logs: LogEntry[] = [];
      const client = new LLMClient({}, { traceLLM: true, onLog: (entry) => { logs.push(entry); } });
      const tracedFetchFactory = getPrivateMethod(client, 'createTracedFetch');
      const tracedFetch = tracedFetchFactory.call(client) as typeof fetch;
      const originalFetch = globalThis.fetch;
      let capturedHeaderValues: Record<string, string | null> | undefined;
      let bodyJson: unknown;
      try {
        globalThis.fetch = ((_input, init) => {
          const headers = new Headers(init?.headers);
          capturedHeaderValues = {
            accept: headers.get('Accept'),
            referer: headers.get('HTTP-Referer'),
            title: headers.get('X-OpenRouter-Title'),
            userAgent: headers.get('User-Agent'),
          };
          return Promise.resolve(new Response(JSON.stringify({
            provider: COVERAGE_ROUTER_PROVIDER,
            model: COVERAGE_ROUTER_MODEL,
            usage: {
              cost: 1.23,
              cost_details: { upstream_inference_cost: 0.45 },
            },
          }), {
            status: 200,
            headers: { 'content-type': CONTENT_TYPE_JSON },
          }));
        }) as typeof fetch;
        const response = await tracedFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: {
            Authorization: 'Bearer TOKEN1234567890',
          },
          body: JSON.stringify({ prompt: COVERAGE_PROMPT }),
        });
        bodyJson = await response.json();
      } finally {
        globalThis.fetch = originalFetch;
      }
      coverageOpenrouterJson = {
        accept: capturedHeaderValues?.accept ?? undefined,
        referer: capturedHeaderValues?.referer ?? undefined,
        title: capturedHeaderValues?.title ?? undefined,
        userAgent: capturedHeaderValues?.userAgent ?? undefined,
        logs: [...logs],
        routed: client.getLastActualRouting() ?? {},
        cost: client.getLastCostInfo() ?? {},
        response: bodyJson,
      };
      return makeSuccessResult(COVERAGE_OPENROUTER_JSON_ID);
    },
    expect: (result) => {
      invariant(result.success, `Coverage ${COVERAGE_OPENROUTER_JSON_ID} should succeed.`);
      invariant(coverageOpenrouterJson !== undefined, 'Coverage data missing for openrouter json.');
      invariant(coverageOpenrouterJson.accept === CONTENT_TYPE_JSON, 'Accept header should default to application/json.');
      invariant(typeof coverageOpenrouterJson.referer === 'string' && coverageOpenrouterJson.referer.length > 0, 'Referer header missing for openrouter json.');
      invariant(typeof coverageOpenrouterJson.title === 'string' && coverageOpenrouterJson.title.length > 0, 'Title header missing for openrouter json.');
      invariant(typeof coverageOpenrouterJson.userAgent === 'string' && coverageOpenrouterJson.userAgent.length > 0, 'User-Agent header missing for openrouter json.');
      invariant(coverageOpenrouterJson.logs.some((log) => log.severity === 'TRC' && log.direction === 'request'), 'Trace request log expected for openrouter json.');
      invariant(coverageOpenrouterJson.logs.some((log) => log.severity === 'TRC' && log.direction === 'response'), 'Trace response log expected for openrouter json.');
      invariant(coverageOpenrouterJson.routed.provider === COVERAGE_ROUTER_PROVIDER, 'Actual provider mismatch for openrouter json.');
      invariant(coverageOpenrouterJson.routed.model === COVERAGE_ROUTER_MODEL, 'Actual model mismatch for openrouter json.');
      invariant(coverageOpenrouterJson.cost.costUsd === 1.23, 'Cost extraction mismatch for openrouter json.');
      invariant(coverageOpenrouterJson.cost.upstreamInferenceCostUsd === 0.45, 'Upstream cost mismatch for openrouter json.');
      const response = coverageOpenrouterJson.response as Record<string, unknown> | undefined;
      const responseModel = typeof response?.model === 'string' ? response.model : undefined;
      invariant(responseModel === COVERAGE_ROUTER_MODEL, 'Response body not preserved for openrouter json.');
    },
  },
  {
    id: COVERAGE_OPENROUTER_SSE_ID,
    description: 'Coverage: streaming SSE parser for traced fetch metadata.',
    execute: async () => {
      coverageOpenrouterSse = undefined;
      const logs: LogEntry[] = [];
      const client = new LLMClient({}, { traceLLM: true, onLog: (entry) => { logs.push(entry); } });
      const tracedFetchFactory = getPrivateMethod(client, 'createTracedFetch');
      const tracedFetch = tracedFetchFactory.call(client) as typeof fetch;
      const originalFetch = globalThis.fetch;
      let capturedHeaderValues: Record<string, string | null> | undefined;
      let rawStream = '';
      try {
        globalThis.fetch = ((_input, init) => {
          const headers = new Headers(init?.headers);
          capturedHeaderValues = {
            accept: headers.get('Accept'),
          };
          rawStream = `data: {"provider":"${COVERAGE_ROUTER_SSE_PROVIDER}","model":"${COVERAGE_ROUTER_SSE_MODEL}","usage":{"cost":0.4,"cost_details":{"upstream_inference_cost":0.2}}}\n\ndata: [DONE]\n`;
          return Promise.resolve(new Response(rawStream, {
            status: 200,
            headers: { 'content-type': CONTENT_TYPE_EVENT_STREAM },
          }));
        }) as typeof fetch;
        const response = await tracedFetch('https://openrouter.ai/api/v1/stream', {
          method: 'POST',
          headers: {
            Accept: CONTENT_TYPE_JSON,
          },
        });
        await response.text();
        await client.waitForMetadataCapture();
      } finally {
        globalThis.fetch = originalFetch;
      }
      coverageOpenrouterSse = {
        accept: capturedHeaderValues?.accept ?? undefined,
        logs: [...logs],
        routed: client.getLastActualRouting() ?? {},
        cost: client.getLastCostInfo() ?? {},
        rawStream,
      };
      return makeSuccessResult(COVERAGE_OPENROUTER_SSE_ID);
    },
    expect: (result) => {
      invariant(result.success, `Coverage ${COVERAGE_OPENROUTER_SSE_ID} should succeed.`);
      invariant(coverageOpenrouterSse !== undefined, 'Coverage data missing for openrouter sse.');
      invariant(coverageOpenrouterSse.accept === CONTENT_TYPE_JSON, 'Accept header should remain unchanged when preset.');
      invariant(coverageOpenrouterSse.logs.some((log) => typeof log.message === 'string' && log.message.includes('raw-sse')), 'Trace response should capture raw SSE content.');
      invariant(coverageOpenrouterSse.routed.provider === COVERAGE_ROUTER_SSE_PROVIDER, 'Actual provider mismatch for openrouter sse.');
      invariant(coverageOpenrouterSse.routed.model === COVERAGE_ROUTER_SSE_MODEL, 'Actual model mismatch for openrouter sse.');
      invariant(coverageOpenrouterSse.cost.costUsd === 0.4, 'Cost extraction mismatch for openrouter sse.');
      invariant(coverageOpenrouterSse.cost.upstreamInferenceCostUsd === 0.2, 'Upstream cost mismatch for openrouter sse.');
      invariant(coverageOpenrouterSse.rawStream.includes('[DONE]'), 'SSE payload missing sentinel.');
    },
  },
  {
    id: COVERAGE_OPENROUTER_SSE_NONBLOCKING_ID,
    description: 'Coverage: traced fetch returns before SSE metadata capture completes.',
    execute: async () => {
      coverageOpenrouterSseNonBlocking = undefined;
      const logs: LogEntry[] = [];
      const client = new LLMClient({}, { traceLLM: true, onLog: (entry) => { logs.push(entry); } });
      const tracedFetchFactory = getPrivateMethod(client, 'createTracedFetch');
      const tracedFetch = tracedFetchFactory.call(client) as typeof fetch;
      const originalFetch = globalThis.fetch;
      const gate = createDeferred();
      const encoder = new TextEncoder();
      try {
        globalThis.fetch = ((_input, init) => {
          const headers = new Headers(init?.headers);
          if (!headers.has('Accept')) headers.set('Accept', CONTENT_TYPE_JSON);
          const bodyStream = new ReadableStream<Uint8Array>({
            start(controller) {
              controller.enqueue(encoder.encode('data: {"provider":"mistral","model":"mixtral"}\n\n'));
              const flush = async (): Promise<void> => {
                try {
                  await gate.promise;
                  controller.enqueue(encoder.encode('data: {"usage":{"cost":0.21,"cost_details":{"upstream_inference_cost":0.09}}}\n\n'));
                  controller.enqueue(encoder.encode('data: [DONE]\n\n'));
                  controller.close();
                } catch (error: unknown) {
                  controller.error(error);
                }
              };
              void flush();
            },
          });
          return Promise.resolve(new Response(bodyStream, {
            status: 200,
            headers: { 'content-type': CONTENT_TYPE_EVENT_STREAM },
          }));
        }) as typeof fetch;

        const start = Date.now();
        const responsePromise = tracedFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Accept: CONTENT_TYPE_JSON },
        });
        const responseReady = (async () => {
          await responsePromise;
          return 'response' as const;
        })();
        const timeoutReady = new Promise<'timeout'>((resolve) => { setTimeout(() => { resolve('timeout'); }, 25); });
        const raceResult = await Promise.race([responseReady, timeoutReady]);
        const elapsedBeforeMetadata = Date.now() - start;
        const response = await responsePromise;
        const textPromise = response.text();
        coverageOpenrouterSseNonBlocking = {
          raceResult,
          elapsedBeforeMetadataMs: elapsedBeforeMetadata,
          totalElapsedMs: 0,
          routed: client.getLastActualRouting() ?? {},
          cost: client.getLastCostInfo() ?? {},
          logs: [...logs],
        };
        gate.resolve(undefined);
        await textPromise;
        await client.waitForMetadataCapture();
        coverageOpenrouterSseNonBlocking.totalElapsedMs = Date.now() - start;
        coverageOpenrouterSseNonBlocking.routed = client.getLastActualRouting() ?? {};
        coverageOpenrouterSseNonBlocking.cost = client.getLastCostInfo() ?? {};
        return makeSuccessResult(COVERAGE_OPENROUTER_SSE_NONBLOCKING_ID);
      } finally {
        globalThis.fetch = originalFetch;
      }
    },
    expect: (result) => {
      invariant(result.success, `Coverage ${COVERAGE_OPENROUTER_SSE_NONBLOCKING_ID} should succeed.`);
      invariant(coverageOpenrouterSseNonBlocking !== undefined, 'Coverage data missing for openrouter SSE non-blocking path.');
      invariant(coverageOpenrouterSseNonBlocking.raceResult === 'response', 'Traced fetch should resolve before metadata completion for SSE.');
      invariant(coverageOpenrouterSseNonBlocking.elapsedBeforeMetadataMs < 25, 'SSE traced fetch waited too long before returning response.');
      invariant(coverageOpenrouterSseNonBlocking.totalElapsedMs >= coverageOpenrouterSseNonBlocking.elapsedBeforeMetadataMs, 'Total elapsed time should exceed initial wait.');
      invariant(coverageOpenrouterSseNonBlocking.routed.provider === 'mistral', 'Routing metadata missing after metadata capture for SSE non-blocking coverage.');
      invariant(coverageOpenrouterSseNonBlocking.routed.model === 'mixtral', 'Routing model missing after metadata capture for SSE non-blocking coverage.');
      invariant(coverageOpenrouterSseNonBlocking.cost.costUsd === 0.21, 'Cost metadata mismatch for SSE non-blocking coverage.');
      invariant(coverageOpenrouterSseNonBlocking.cost.upstreamInferenceCostUsd === 0.09, 'Upstream cost metadata mismatch for SSE non-blocking coverage.');
    },
  },
  {
    id: COVERAGE_GENERIC_JSON_ID,
    description: 'Coverage: generic JSON cache-write token enrichment via traced fetch.',
    execute: async () => {
      coverageGenericJson = undefined;
      const logs: LogEntry[] = [];
      const client = new LLMClient({}, { traceLLM: true, onLog: (entry) => { logs.push(entry); } });
      const tracedFetchFactory = getPrivateMethod(client, 'createTracedFetch');
      const tracedFetch = tracedFetchFactory.call(client) as typeof fetch;
      const originalFetch = globalThis.fetch;
      let capturedHeaderValues: Record<string, string | null> | undefined;
      try {
        globalThis.fetch = ((_input, init) => {
          const headers = new Headers(init?.headers);
          capturedHeaderValues = {
            accept: headers.get('Accept'),
          };
          return Promise.resolve(new Response(JSON.stringify({
            usage: {
              cache_creation: { ephemeral_5m_input_tokens: 42 },
            },
          }), {
            status: 200,
            headers: { 'content-type': CONTENT_TYPE_JSON },
          }));
        }) as typeof fetch;
        const response = await tracedFetch('https://api.anthropic.com/v1/messages', {});
        await response.json();
      } finally {
        globalThis.fetch = originalFetch;
      }
      const cacheWrite = getPrivateField(client, 'lastCacheWriteInputTokens') as (number | undefined);
      coverageGenericJson = {
        accept: capturedHeaderValues?.accept ?? undefined,
        cacheWrite,
        logs: [...logs],
      };
      return makeSuccessResult(COVERAGE_GENERIC_JSON_ID);
    },
    expect: (result) => {
      invariant(result.success, `Coverage ${COVERAGE_GENERIC_JSON_ID} should succeed.`);
      invariant(coverageGenericJson !== undefined, 'Coverage data missing for generic json.');
      invariant(coverageGenericJson.accept === CONTENT_TYPE_JSON, 'Accept header should default to application/json for generic json.');
      invariant(coverageGenericJson.cacheWrite === 42, 'Cache write tokens not extracted from generic json.');
      invariant(coverageGenericJson.logs.some((log) => log.direction === 'response' && log.severity === 'TRC'), 'Trace response log expected for generic json.');
    },
  },
  {
    id: COVERAGE_SESSION_ID,
    description: 'Coverage: session snapshot persistence success and failure paths.',
    execute: async () => {
      coverageSessionSnapshot = undefined;
      const configuration = makeBasicConfiguration();
      configuration.defaults = { ...BASE_DEFAULTS };
      const defaults = configuration.defaults ?? BASE_DEFAULTS;
      const tempDir = makeTempDir(COVERAGE_SESSION_ID);
      configuration.persistence = { sessionsDir: tempDir };
      const snapshotWarnings: string[] = [];
      const previousWarningSink = getWarningSink();
      setWarningSink((message) => { snapshotWarnings.push(message); });

      try {
        const snapshotWriter = async (payload: Parameters<NonNullable<AIAgentCallbacks['onSessionSnapshot']>>[0]): Promise<void> => {
          const json = JSON.stringify({
            version: payload.snapshot.version,
            reason: payload.reason,
            opTree: payload.snapshot.opTree,
          });
          const gz = gzipSync(Buffer.from(json, 'utf8'));
          const filePath = path.join(tempDir, `${payload.originId}.json.gz`);
          await fs.promises.mkdir(tempDir, { recursive: true });
          await fs.promises.writeFile(filePath, gz);
        };

        const sessionConfig: AIAgentSessionConfig = {
          config: configuration,
          targets: [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }],
          tools: [],
          systemPrompt: 'Phase 1 deterministic harness: coverage snapshot.',
          userPrompt: DEFAULT_PROMPT_SCENARIO,
          outputFormat: 'markdown',
          stream: false,
          parallelToolCalls: false,
          maxTurns: 1,
          toolTimeout: defaults.toolTimeout,
          llmTimeout: defaults.llmTimeout,
          maxRetries: defaults.maxRetries,
          agentId: COVERAGE_SESSION_ID,
          abortSignal: new AbortController().signal,
          callbacks: { onSessionSnapshot: snapshotWriter },
        };

        const session = AIAgentSession.create(sessionConfig);
        const persistSessionSnapshot = getPrivateMethod(session, 'persistSessionSnapshot').bind(session) as (reason?: string) => Promise<unknown>;

        await persistSessionSnapshot('coverage-success');
        const filesAfterSuccess = await fs.promises.readdir(tempDir);

        const liveSessionConfig = getPrivateField(session, 'sessionConfig') as (AIAgentSessionConfig | undefined);
        const originalSnapshotHandler = liveSessionConfig?.callbacks?.onSessionSnapshot;
        if (liveSessionConfig?.callbacks !== undefined) {
          liveSessionConfig.callbacks.onSessionSnapshot = () => { throw new Error('snapshot-write-failure'); };
        }

        try {
          await persistSessionSnapshot('coverage-failure');
        } finally {
          if (liveSessionConfig?.callbacks !== undefined) {
            liveSessionConfig.callbacks.onSessionSnapshot = originalSnapshotHandler;
          }
        }

        const filesAfterFailure = await fs.promises.readdir(tempDir);
        coverageSessionSnapshot = {
          tempDir,
          filesAfterSuccess,
          filesAfterFailure,
          warnOutput: snapshotWarnings.join('\n'),
        };
      } finally {
        setWarningSink(previousWarningSink ?? defaultHarnessWarningSink);
      }
      return makeSuccessResult(COVERAGE_SESSION_ID);
    },
    expect: (result) => {
      invariant(result.success, `Coverage ${COVERAGE_SESSION_ID} should succeed.`);
      invariant(coverageSessionSnapshot !== undefined, 'Coverage data missing for session snapshot.');
      invariant(coverageSessionSnapshot.filesAfterSuccess.length > 0, 'Snapshot file not created in success path.');
      invariant(coverageSessionSnapshot.warnOutput.includes(COVERAGE_WARN_SUBSTRING), 'Warning output missing for snapshot failure.');
      invariant(coverageSessionSnapshot.filesAfterFailure.length === coverageSessionSnapshot.filesAfterSuccess.length, 'Failure path should not create additional snapshot files.');
      try { fs.rmSync(coverageSessionSnapshot.tempDir, { recursive: true, force: true }); } catch { /* ignore cleanup failure */ }
    },
  },
  {
    id: 'run-test-44',
    configure: (_configuration, sessionConfig) => {
      const stopRef = { stopping: false };
      sessionConfig.stopRef = stopRef;
      sessionConfig.maxRetries = 2;
    },
    execute: async (_configuration, sessionConfig) => {
      const stopRef = sessionConfig.stopRef ?? { stopping: false };
      sessionConfig.stopRef = stopRef;
      const session = AIAgentSession.create(sessionConfig);
      setTimeout(() => { stopRef.stopping = true; }, 10);
      return await session.run();
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-44 should honor graceful stop during rate limit.');
      invariant(result.finalReport === undefined, 'No final report expected for run-test-44.');
      const finLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:fin');
      invariant(finLog !== undefined, 'Finalization log expected for run-test-44.');
    },
  },
];

async function runScenario(test: HarnessTest): Promise<AIAgentResult> {
  const { id: prompt, configure, execute } = test;
  const serverPath = path.resolve(__dirname, 'mcp/test-stdio-server.js');
  const effectiveTimeout = getEffectiveTimeoutMs(test);
  const abortController = new AbortController();

  const baseConfiguration: Configuration = {
    providers: {
      [PRIMARY_PROVIDER]: {
        type: 'test-llm',
      },
    },
    mcpServers: {
      test: {
        type: 'stdio',
        command: process.execPath,
        args: [serverPath],
      },
    },
    defaults: { ...BASE_DEFAULTS },
  };

  const configuration: Configuration = JSON.parse(JSON.stringify(baseConfiguration)) as Configuration;
  configuration.defaults = { ...BASE_DEFAULTS, ...(configuration.defaults ?? {}) };
  const defaults = configuration.defaults;

  const baseSession: AIAgentSessionConfig = {
    config: configuration,
    targets: [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }],
    tools: ['test'],
    systemPrompt: 'Phase 1 deterministic harness: respond using scripted outputs.',
    userPrompt: prompt,
    outputFormat: 'markdown',
    stream: false,
    parallelToolCalls: false,
    maxTurns: 3,
    toolTimeout: defaults.toolTimeout,
    llmTimeout: defaults.llmTimeout,
    agentId: `phase1-${prompt}`,
    abortSignal: abortController.signal,
  };

  configure?.(configuration, baseSession, defaults);

  let abortOnTimeout: (() => void) | undefined;
  if (baseSession.abortSignal === undefined) {
    baseSession.abortSignal = abortController.signal;
    abortOnTimeout = () => {
      try { abortController.abort(); } catch { /* ignore */ }
    };
  } else if (baseSession.abortSignal === abortController.signal) {
    abortOnTimeout = () => {
      try { abortController.abort(); } catch { /* ignore */ }
    };
  }

  const failureResult = (error: unknown): AIAgentResult => {
    const message = toErrorMessage(error);
    return {
      success: false,
      error: message,
      conversation: [],
      logs: [],
      accounting: [],
    };
  };

  const runner = async (): Promise<AIAgentResult> => {
    if (typeof execute === 'function') {
      return await execute(configuration, baseSession, defaults);
    }

    let session;
    try {
      session = AIAgentSession.create(baseSession);
    } catch (error: unknown) {
      return failureResult(error);
    }

    try {
      return await session.run();
    } catch (error: unknown) {
      return failureResult(error);
    }
  };

  return await runWithTimeout(runner(), effectiveTimeout, prompt, abortOnTimeout);
}

function formatFailureHint(result: AIAgentResult): string {
  const hints: string[] = [];
  if (!result.success && typeof result.error === 'string' && result.error.length > 0) {
    hints.push(`session-error="${result.error}"`);
  }
  const lastLog = result.logs.at(-1);
  if (lastLog !== undefined) {
    const msg = typeof lastLog.message === 'string' ? lastLog.message : JSON.stringify(lastLog.message);
    hints.push(`last-log="${msg}"`);
  }
  if (result.finalReport !== undefined) {
    hints.push(`final-status=${result.finalReport.status}`);
  }
  return hints.length > 0 ? ` (${hints.join(' | ')})` : '';
}

async function runPhaseOne(): Promise<void> {
  const total = TEST_SCENARIOS.length;
  // eslint-disable-next-line functional/no-loop-statements
  for (let index = 0; index < total; index += 1) {
    const scenario = TEST_SCENARIOS[index];
    const runPrefix = `${String(index + 1)}/${String(total)}`;
    const description = resolveScenarioDescription(scenario.id, scenario.description);
    const baseLabel = `${runPrefix} ${scenario.id}`;
    const header = `[RUN] ${baseLabel}${description !== undefined ? `: ${description}` : ''}`;
    const startMs = Date.now();
    let result: AIAgentResult | undefined;
    try {
      result = await runScenario(scenario);
      scenario.expect(result);
      const duration = formatDurationMs(startMs, Date.now());
      // eslint-disable-next-line no-console
      console.log(`${header} [PASS] ${duration}`);
    } catch (error: unknown) {
      const duration = formatDurationMs(startMs, Date.now());
      const message = toErrorMessage(error);
      const hint = result !== undefined ? formatFailureHint(result) : '';
      // eslint-disable-next-line no-console
      console.error(`${header} [FAIL] ${duration} - ${message}${hint}`);
      throw error;
    }
  }
  const diagnosticProcess = process as typeof process & { _getActiveHandles?: () => unknown[] };
  const activeHandles = typeof diagnosticProcess._getActiveHandles === 'function'
    ? diagnosticProcess._getActiveHandles()
    : [];
  if (activeHandles.length > 0) {
    const childCleanup: Promise<void>[] = [];
    const shouldIgnore = (handle: unknown): boolean => {
      if (handle === null || typeof handle !== 'object') return false;
      const fd = (handle as { fd?: unknown }).fd;
      if (typeof fd === 'number' && (fd === 0 || fd === 1 || fd === 2)) return true;
      const ctorName = (handle as { constructor?: { name?: string } }).constructor?.name;
      if (ctorName === 'Socket') {
        const maybeLocal = (handle as { localAddress?: unknown; remoteAddress?: unknown });
        const local = typeof maybeLocal.localAddress === 'string' ? maybeLocal.localAddress : undefined;
        const remote = typeof maybeLocal.remoteAddress === 'string' ? maybeLocal.remoteAddress : undefined;
        if (local === undefined && remote === undefined) {
          return true;
        }
      }
      return false;
    };
    const formatHandle = (handle: unknown): string => {
      if (handle === null) return 'null';
      if (typeof handle !== 'object') return typeof handle;
      const name = (handle as { constructor?: { name?: string } }).constructor?.name ?? 'object';
      const parts: string[] = [];
      const pid = (handle as { pid?: unknown }).pid;
      if (typeof pid === 'number') { parts.push(`pid=${String(pid)}`); }
      const fd = (handle as { fd?: unknown }).fd;
      if (typeof fd === 'number') { parts.push(`fd=${String(fd)}`); }
      const maybeLocal = (handle as { localAddress?: unknown; remoteAddress?: unknown });
      const local = typeof maybeLocal.localAddress === 'string' ? maybeLocal.localAddress : undefined;
      const remote = typeof maybeLocal.remoteAddress === 'string' ? maybeLocal.remoteAddress : undefined;
      if (local !== undefined || remote !== undefined) {
        parts.push(`socket=${local ?? ''}->${remote ?? ''}`);
      }
      return parts.length > 0 ? `${name}(${parts.join(';')})` : name;
    };
    activeHandles.forEach((handle) => {
      if (handle === null || typeof handle !== 'object') return;
      const maybePid = (handle as { pid?: unknown }).pid;
      if (typeof maybePid === 'number') {
        const child = handle as ChildProcess;
        const exitPromise = new Promise<void>((resolve) => {
          const finish = (): void => { resolve(); };
          child.once('exit', finish);
          child.once('error', finish);
          setTimeout(() => { resolve(); }, 1000);
        });
        childCleanup.push(exitPromise);
        try {
          if (typeof child.kill === 'function') {
            try { child.kill('SIGTERM'); } catch { /* ignore */ }
            try { child.kill('SIGKILL'); } catch { /* ignore */ }
          }
        } catch { /* ignore */ }
        try { child.disconnect(); } catch { /* ignore */ }
        try { child.unref(); } catch { /* ignore */ }
        const stdioStreams = Array.isArray(child.stdio) ? child.stdio : [];
        stdioStreams.forEach((stream) => {
          if (stream === null || stream === undefined) return;
          const destroyStream = (stream as { destroy?: () => void }).destroy;
          if (typeof destroyStream === 'function') {
            try { destroyStream.call(stream); } catch { /* ignore */ }
          }
          const endStream = (stream as { end?: () => void }).end;
          if (typeof endStream === 'function') {
            try { endStream.call(stream); } catch { /* ignore */ }
          }
          const unrefStream = (stream as { unref?: () => void }).unref;
          if (typeof unrefStream === 'function') {
            try { unrefStream.call(stream); } catch { /* ignore */ }
          }
        });
        return;
      }
      const fd = (handle as { fd?: unknown }).fd;
      if (typeof fd === 'number' && (fd === 0 || fd === 1 || fd === 2)) {
        return;
      }
      const destroy = (handle as { destroy?: () => void }).destroy;
      if (typeof destroy === 'function') {
        try { destroy.call(handle); } catch { /* ignore */ }
      }
      const end = (handle as { end?: () => void }).end;
      if (typeof end === 'function') {
        try { end.call(handle); } catch { /* ignore */ }
      }
      const close = (handle as { close?: () => void }).close;
      if (typeof close === 'function') {
        try { close.call(handle); } catch { /* ignore */ }
      }
      const unref = (handle as { unref?: () => void }).unref;
      if (typeof unref === 'function') {
        try { unref.call(handle); } catch { /* ignore */ }
      }
    });
    await Promise.all(childCleanup);
    const remaining = typeof diagnosticProcess._getActiveHandles === 'function'
      ? diagnosticProcess._getActiveHandles()
      : [];
    const blockingHandles = remaining.filter((handle) => !shouldIgnore(handle));
    if (blockingHandles.length > 0) {
      const labels = blockingHandles.map(formatHandle);
      // eslint-disable-next-line no-console
      console.error(`[warn] lingering handles after cleanup: ${labels.join(', ')}`);
    }
  }
  // eslint-disable-next-line no-console
  console.log('phase1 scenario: ok');
}

runPhaseOne()
  .then(() => {
    process.exit(0);
  })
  .catch((error: unknown) => {
    const message = toErrorMessage(error);
    // eslint-disable-next-line no-console
    console.error(`phase1 scenario failed: ${message}`);
    process.exit(1);
  });
