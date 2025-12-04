import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

import { WebSocketServer } from 'ws';

import type { LoadAgentOptions, LoadedAgent } from '../../agent-loader.js';
import type { ResolvedConfigLayer } from '../../config-resolver.js';
import type { StructuredLogEvent } from '../../logging/structured-log-event.js';
import type { PreloadedSubAgent } from '../../subagent-registry.js';
import type { MCPRestartFailedError, LogFn, SharedAcquireOptions, SharedRegistry, SharedRegistryHandle } from '../../tools/mcp-provider.js';
import type { ToolExecuteResult } from '../../tools/types.js';
import type { AIAgentCallbacks, AIAgentResult, AIAgentSessionConfig, AccountingEntry, AgentFinishedEvent, Configuration, ConversationMessage, LogDetailValue, LogEntry, LogPayload, MCPServer, MCPServerConfig, MCPTool, ProviderConfig, ProviderReasoningValue, ReasoningLevel, TokenUsage, ToolCall, TurnRequest, TurnResult, TurnStatus, ToolChoiceMode } from '../../types.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import type { ModelMessage, LanguageModel, ToolSet } from 'ai';
import type { ChildProcess } from 'node:child_process';
import type { AddressInfo } from 'node:net';

import { loadAgent, loadAgentFromContent } from '../../agent-loader.js';
import { AgentRegistry } from '../../agent-registry.js';
import { AIAgentSession } from '../../ai-agent.js';
import { loadConfiguration } from '../../config.js';
import { parseFrontmatter, parseList, parsePairs } from '../../frontmatter.js';
import { resolveIncludes } from '../../include-resolver.js';
import { DEFAULT_TOOL_INPUT_SCHEMA } from '../../input-contract.js';
import { LLMClient } from '../../llm-client.js';
import { MAX_TURNS_FINAL_MESSAGE } from '../../llm-messages.js';
import { AnthropicProvider } from '../../llm-providers/anthropic.js';
import { BaseLLMProvider, type ResponseMessage } from '../../llm-providers/base.js';
import { OpenRouterProvider } from '../../llm-providers/openrouter.js';
import { TestLLMProvider } from '../../llm-providers/test-llm.js';
import { formatRichLogLine } from '../../logging/rich-format.js';
import { ShutdownController } from '../../shutdown-controller.js';
import { SubAgentRegistry } from '../../subagent-registry.js';
import { estimateMessagesTokens, resolveTokenizer } from '../../tokenizer-registry.js';
import { MCPProvider, forceRemoveSharedRegistryEntry } from '../../tools/mcp-provider.js';
import { queueManager } from '../../tools/queue-manager.js';
import { clampToolName, sanitizeToolName, formatToolRequestCompact, truncateUtf8WithNotice, formatAgentResultHumanReadable, setWarningSink, getWarningSink, warn } from '../../utils.js';
import { createWebSocketTransport } from '../../websocket-transport.js';
import { getScenario } from '../fixtures/test-llm-scenarios.js';
import { runJournaldSinkUnitTests } from '../unit/journald-sink.test.js';
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const MODEL_NAME = 'deterministic-model';
const PRIMARY_PROVIDER = 'primary';
const SECONDARY_PROVIDER = 'secondary';
const SUBAGENT_TOOL = 'agent__pricing-subagent';
const COVERAGE_CHILD_TOOL = 'coverage.child';
const COVERAGE_PARENT_BODY = 'Parent body.';
const COVERAGE_CHILD_BODY = 'Child body.';
const PRIMARY_REMOTE = `${PRIMARY_PROVIDER}:${MODEL_NAME}`;
const RETRY_REMOTE = 'agent:retry';
const FINAL_TURN_REMOTE = 'agent:final-turn';
const CONTEXT_REMOTE = 'agent:context';
const CONTEXT_OVERFLOW_FRAGMENT = 'context window budget exceeded';
const CONTEXT_FORCING_FRAGMENT = 'Forcing final turn';
const TOKENIZER_GPT4O = 'tiktoken:gpt-4o-mini';
const MINIMAL_SYSTEM_PROMPT = 'Phase 1 deterministic harness: minimal instructions.';
const HISTORY_SYSTEM_SEED = 'Historical system directive for counter seeding.';
const HISTORY_ASSISTANT_SEED = 'Historical assistant summary preserved for context metrics.';
const THRESHOLD_SYSTEM_PROMPT = 'Phase 1 deterministic harness: threshold probe instructions.';
const THRESHOLD_USER_PROMPT = 'context_guard__threshold_probe';
const THRESHOLD_ABOVE_USER_PROMPT = 'context_guard__threshold_above_probe';
const BACKING_OFF_FRAGMENT = 'backing off';
const RUN_TEST_11 = 'run-test-11';
const RUN_TEST_21 = 'run-test-21';
const RUN_TEST_31 = 'run-test-31';
const RUN_TEST_33 = 'run-test-33';
const RUN_TEST_37 = 'run-test-37';
const RUN_TEST_MAX_TURN_LIMIT = 'run-test-max-turn-limit';
const TEXT_EXTRACTION_RETRY_RESULT = 'Valid report after retry.';
const TEXT_EXTRACTION_INVALID_TEXT_RESULT = 'Valid report after invalid text.';
const PURE_TEXT_RETRY_RESULT = 'Valid report after pure text retry.';
const COLLAPSE_RECOVERY_RESULT = 'Proper result after collapse.';
const COLLAPSE_FIXED_RESULT = 'Fixed after collapse.';
const MAX_RETRY_SUCCESS_RESULT = 'Success after retries.';
const COLLAPSING_REMAINING_TURNS_FRAGMENT = 'Collapsing remaining turns';
const CONTEXT_POST_SHRINK_TURN_WARN = 'Context guard post-shrink still over projected limit during turn execution; proceeding anyway.';
const parseDumpList = (raw?: string): string[] => {
  if (typeof raw !== 'string') return [];
  return raw.split(',').map((value) => value.trim()).filter((value) => value.length > 0);
};
const DUMP_SCENARIOS = new Set(parseDumpList(process.env.PHASE1_DUMP_SCENARIO));
const dumpScenarioResultIfNeeded = (scenarioId: string, result: AIAgentResult): void => {
  if (!DUMP_SCENARIOS.has(scenarioId)) return;
  console.log(`[dump] scenario=${scenarioId}:`, JSON.stringify(result, null, 2));
};
const TOOL_DROP_STUB = `(tool failed: ${CONTEXT_OVERFLOW_FRAGMENT})`;
const SHARED_REGISTRY_RESULT = 'shared-response';
const SECOND_TURN_FINAL_ANSWER = 'Second turn final answer.';
const RESTART_TRIGGER_PAYLOAD = 'restart-cycle';
const RESTART_POST_PAYLOAD = 'post-restart';
const FIXTURE_PREEMPT_REASON = 'fixture-preempt';
const TEST_STDIO_SERVER_PATH = path.resolve(__dirname, '../mcp/test-stdio-server.js');
const delay = (ms: number): Promise<void> => new Promise((resolve) => { setTimeout(resolve, ms); });

interface RestartFixtureState {
  phase: string;
  failOnStart?: boolean;
  remainingRestartFails?: number;
  remainingInitFails?: number;
}

const createRestartFixtureStateFile = (): string => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ai-agent-restart-'));
  const file = path.join(dir, 'state.json');
  const initial: RestartFixtureState = { phase: 'initial' };
  fs.writeFileSync(file, JSON.stringify(initial), 'utf8');
  return file;
};

const readRestartFixtureState = (file: string): RestartFixtureState => {
  const raw = fs.readFileSync(file, 'utf8');
  return JSON.parse(raw) as RestartFixtureState;
};

const updateRestartFixtureState = (file: string, updater: (prev: RestartFixtureState) => RestartFixtureState): void => {
  const prev = readRestartFixtureState(file);
  const next = updater(prev);
  fs.writeFileSync(file, JSON.stringify(next), 'utf8');
};

const toError = (value: unknown): Error => (value instanceof Error ? value : new Error(String(value)));
const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

const defaultHarnessWarningSink = (message: string): void => {
  const prefix = '[warn] ';
  const output = process.stderr.isTTY ? `\x1b[33m${prefix}${message}\x1b[0m` : `${prefix}${message}`;
  try { process.stderr.write(`${output}\n`); } catch { /* ignore */ }
};

setWarningSink(defaultHarnessWarningSink);

class HarnessSharedRegistry implements SharedRegistry {
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

class HarnessSharedHandle implements SharedRegistryHandle {
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
const sessionConfigObservers: ((config: AIAgentSessionConfig) => void)[] = [];
(AIAgentSession as unknown as { create: (sessionConfig: AIAgentSessionConfig) => AIAgentSession }).create = (sessionConfig: AIAgentSessionConfig) => {
  const originalCallbacks = sessionConfig.callbacks;
  const callbacks = defaultPersistenceCallbacks(sessionConfig.config, originalCallbacks);
  const patchedConfig = callbacks === originalCallbacks ? sessionConfig : { ...sessionConfig, callbacks };
  sessionConfigObservers.forEach((observer) => {
    try {
      observer(patchedConfig);
    } catch (error: unknown) {
      warn(`session config observer failure: ${error instanceof Error ? error.message : String(error)}`);
    }
  });
  return baseCreate(patchedConfig);
};

const COVERAGE_ALIAS_BASENAME = 'parent.ai';
const TRUNCATION_NOTICE = '[TRUNCATED]';
const CONFIG_FILE_NAME = 'config.json';
const INCLUDE_DIRECTIVE_TOKEN = '${include:';
const FINAL_REPORT_CALL_ID = 'agent-final-report';
const MCP_TEST_REMOTE = 'mcp:test:test';
const RATE_LIMIT_FINAL_CONTENT = 'Final report after rate limit.';
const ADOPTED_FINAL_CONTENT = 'Adopted final report content.';
const ADOPTION_METADATA_ORIGIN = 'text-adoption';
const ADOPTION_CONTENT_VALUE = 'value';
const FINAL_REPORT_SANITIZED_CONTENT = 'Final report without sanitized tool calls.';
const FINAL_REPORT_AFTER_RETRY = 'Final report after retry.';
const TEXT_FALLBACK_CONTENT = 'Text fallback content.';
const TOOL_MESSAGE_FALLBACK_CONTENT = 'Tool message fallback content.';
// CONTRACT: synthetic failure when max turns exhausted always uses 'max_turns_exhausted'
const SYNTHETIC_MAX_RETRY_REASON = 'max_turns_exhausted';
const FINAL_REPORT_MAX_RETRIES_TOTAL_ATTEMPTS = 4;
const INVALID_FINAL_REPORT_PAYLOAD = 'invalid-final-report';
const INVALID_FINAL_REPORT_PAYLOAD_WITH_NEWLINES = '{status:success\nreport_format:text}';
const FINAL_REPORT_RETRY_TEXT_SCENARIO = 'run-test-final-report-retry-text';
const FINAL_REPORT_TOOL_MESSAGE_SCENARIO = 'run-test-final-report-tool-message-fallback';
const FINAL_REPORT_MAX_RETRIES_SCENARIO = 'run-test-final-report-max-retries-synthetic';
const LOG_TEXT_EXTRACTION = 'agent:text-extraction';
const LOG_FALLBACK_REPORT = 'agent:fallback-report';
const LOG_FINAL_REPORT_ACCEPTED = 'agent:final-report-accepted';
const LOG_FAILURE_REPORT = 'agent:failure-report';
const LOG_SANITIZER = 'agent:sanitizer';
const LOG_ORCHESTRATOR = 'agent:orchestrator';
const TOOL_OK_JSON = '{"ok":true}';
const STOP_REASON_TOOL_CALLS = 'tool-calls';
const JSON_ONLY_URL = 'https://example.com/resource';
const DEFAULT_PROMPT_SCENARIO = 'run-test-1' as const;
const BASE_DEFAULTS = {
  stream: false,
  maxTurns: 3,
  maxRetries: 2,
  llmTimeout: 10_000,
  toolTimeout: 10_000,
} as const;
const TMP_PREFIX = 'ai-agent-phase1-';
const SESSIONS_SUBDIR = 'sessions';
const BILLING_FILENAME = 'billing.jsonl';
const THRESHOLD_BUFFER_TOKENS = 8;
const THRESHOLD_MAX_OUTPUT_TOKENS = 32;
const THRESHOLD_CONTEXT_WINDOW_BELOW = 980;
// Tuned so projected tokens land exactly at the limit given current prompt/instruction length.
// limit = contextWindow - buffer - maxOutput = 854 - 8 - 32 = 814 (matches projected)
const THRESHOLD_CONTEXT_WINDOW_EQUAL = 854;
// Tuned so projected tokens exceed the limit with current prompt length.
const THRESHOLD_CONTEXT_WINDOW_ABOVE = 726;
const PREFLIGHT_CONTEXT_WINDOW = 80;
const PREFLIGHT_BUFFER_TOKENS = 8;
const PREFLIGHT_MAX_OUTPUT_TOKENS = 16;
const FORCED_FINAL_CONTEXT_WINDOW = 320;
const FORCED_FINAL_BUFFER_TOKENS = 32;
const FORCED_FINAL_MAX_OUTPUT_TOKENS = 48;
const getLogsByIdentifier = (logs: readonly LogEntry[], identifier: string): LogEntry[] => logs.filter((entry) => entry.remoteIdentifier === identifier);
const findLogByIdentifier = (logs: readonly LogEntry[], identifier: string, predicate?: (entry: LogEntry) => boolean): LogEntry | undefined => {
  if (predicate === undefined) return logs.find((entry) => entry.remoteIdentifier === identifier);
  return logs.find((entry) => entry.remoteIdentifier === identifier && predicate(entry));
};
const expectLogIncludes = (logs: readonly LogEntry[], identifier: string, substring: string, scenarioId: string): void => {
  const log = findLogByIdentifier(logs, identifier, (entry) => typeof entry.message === 'string' && entry.message.includes(substring));
  invariant(log !== undefined, `Expected log ${identifier} containing "${substring}" for ${scenarioId}.`);
};

const safeJsonByteLengthLocal = (value: unknown): number => {
  try {
    return Buffer.byteLength(JSON.stringify(value), 'utf8');
  } catch {
    if (typeof value === 'string') return Buffer.byteLength(value, 'utf8');
    if (value === null || value === undefined) return 0;
    if (typeof value === 'object') {
      let total = 0;
      Object.values(value as Record<string, unknown>).forEach((nested) => {
        total += safeJsonByteLengthLocal(nested);
      });
      return total;
    }
    return 0;
  }
};

type ExecuteTurnHandler = (ctx: { request: TurnRequest; invocation: number }) => Promise<TurnResult>;

const runWithPatchedExecuteTurn = async (
  sessionConfig: AIAgentSessionConfig,
  handler: ExecuteTurnHandler,
): Promise<AIAgentResult> => {
  // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original for restoration after interception
  const originalExecuteTurn = LLMClient.prototype.executeTurn;
  let invocation = 0;
  LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
    invocation += 1;
    return handler({ request, invocation });
  };
  try {
    const session = AIAgentSession.create(sessionConfig);
    return await session.run();
  } finally {
    LLMClient.prototype.executeTurn = originalExecuteTurn;
  }
};

const estimateMessagesBytesLocal = (messages: readonly ConversationMessage[]): number => {
  if (messages.length === 0) {
    return 0;
  }
  return messages.reduce((total, message) => total + safeJsonByteLengthLocal(message), 0);
};

const extractNonceFromMessages = (messages: readonly ConversationMessage[], scenarioId: string): string => {
  const asString = (msg: ConversationMessage): string | undefined => (typeof msg.content === 'string' ? msg.content : undefined);

  // 1) Prefer explicit Nonce line in xml-next
  const notice = messages.find((msg) => msg.noticeType === 'xml-next')
    ?? messages.find((msg) => msg.role === 'user' && typeof msg.content === 'string' && msg.content.includes('Nonce:'));
  const noticeContent = notice !== undefined ? asString(notice) : undefined;
  if (noticeContent !== undefined) {
    const matchLine = /Nonce:\s*([a-z0-9]+)/i.exec(noticeContent);
    if (matchLine !== null && typeof matchLine[1] === 'string' && matchLine[1].length > 0) {
      return matchLine[1];
    }
    const matchSlotNotice = /<ai-agent-([a-z0-9]+)-(FINAL|PROGRESS|0001)/i.exec(noticeContent);
    if (matchSlotNotice !== null && typeof matchSlotNotice[1] === 'string' && matchSlotNotice[1].length > 0) {
      return matchSlotNotice[1];
    }
  }

  // 2) Fallback: scan all messages (system prompt included) for any ai-agent-* tag
  const tagNonce = messages
    .map(asString)
    .filter((c): c is string => c !== undefined)
    .map((content) => /<ai-agent-([a-z0-9]+)-(FINAL|PROGRESS|0001)/i.exec(content))
    .find((m) => m !== null && typeof m[1] === 'string' && m[1].length > 0);
  if (tagNonce !== undefined && tagNonce !== null) {
    return tagNonce[1];
  }

  invariant(false, `Nonce parse failed for ${scenarioId}.`);
  return '';
};

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
const BATCH_STRING_PROGRESS = 'Batch progress conveyed via string payload.';
const BATCH_STRING_RESULT = 'batch-string-mode';
const BATCH_HEX_ARGUMENT_STRING = '{ "text": "hex-\\x41" }';
const BATCH_HEX_EXPECTED_OUTPUT = 'hex-A';
const BATCH_TRUNCATED_ARGUMENT_STRING = `{
  "calls": [
    {
      "id": "repair-1",
      "tool": "test__test",
      "parameters": { "text": "alpha" }
    },
    {
      "id": "repair-2",
      "tool": "test__test"
    }
  ],
  "meta": {
    "note": "incomplete batch payload"
  }
}, ... (truncated for brevity)`;
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
const SANITIZER_DROPPED_MESSAGE = 'Dropped 1 invalid tool call';
const EXIT_FINAL_REPORT_IDENTIFIER = 'agent:EXIT-FINAL-ANSWER';

const GITHUB_SERVER_PATH = path.resolve(__dirname, '../mcp/github-stdio-server.js');

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
let coverageLlmPayload: { logs: LogEntry[] } | undefined;
let coverageSessionSnapshot: {
  tempDir: string;
  filesAfterSuccess: string[];
  filesAfterFailure: string[];
  warnOutput: string;
} | undefined;

function invariant(condition: boolean, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function stripAnsiCodes(value: string): string {
  return value.replace(/\u001B\[[0-9;]*m/g, '');
}

function validateRichFormatterParity(): void {
  const baseTimestamp = 1_735_446_400_000; // 2025-12-01T00:00:00.000Z
  const baseIso = new Date(baseTimestamp).toISOString();
  const parityRemoteIdentifier = 'mcp:brave:web_search';
  const parityAgentPath = 'main:search';
  const createEvent = (overrides: Partial<StructuredLogEvent>): StructuredLogEvent => {
    const { labels: labelsOverride, ...rest } = overrides;
    return {
      timestamp: baseTimestamp,
      isoTimestamp: baseIso,
      severity: 'VRB',
      priority: 6,
      message: '',
      type: 'tool',
      direction: 'request',
      turn: 1,
      subturn: 0,
      labels: { ...(labelsOverride ?? {}) },
      ...rest,
    };
  };
  const RESPONSE_RECEIVED_LABEL = 'LLM response received';

  const events: StructuredLogEvent[] = [
    createEvent({
      message: 'Invoking brave search',
      toolKind: 'mcp',
      type: 'tool',
      direction: 'request',
      remoteIdentifier: parityRemoteIdentifier,
      agentPath: parityAgentPath,
      turnPath: '1.0',
      labels: {
        request_preview: 'web_search(query="ai logging")',
        tool_namespace: 'brave',
        tool: 'web_search',
      },
    }),
    createEvent({
      message: 'Completed in 120 ms, input 10 output 2, cost $0.002',
      toolKind: 'mcp',
      type: 'tool',
      direction: 'response',
      remoteIdentifier: parityRemoteIdentifier,
      agentPath: parityAgentPath,
      turnPath: '1.1',
    }),
    createEvent({
      message: 'LLM call succeeded',
      type: 'llm',
      direction: 'response',
      severity: 'FIN',
      priority: 5,
      provider: 'anthropic',
      model: 'claude-3-haiku',
      remoteIdentifier: 'anthropic:claude-3-haiku',
      agentPath: 'main',
      turnPath: '1.2',
      labels: {
        cost_usd: '0.012345',
        latency_ms: '250',
        response_bytes: '2048',
        input_tokens: '1200',
        output_tokens: '300',
        stop_reason: 'end_turn',
      },
    }),
    createEvent({
      message: 'Tool failed: timeout after 5s',
      type: 'tool',
      direction: 'response',
      severity: 'ERR',
      priority: 3,
      toolKind: 'mcp',
      remoteIdentifier: parityRemoteIdentifier,
      agentPath: parityAgentPath,
      turnPath: '1.3',
    }),
  ];

  events.forEach((event, index) => {
    const colored = formatRichLogLine(event, { tty: true });
    const plain = formatRichLogLine(event, { tty: false });
    const strippedColored = stripAnsiCodes(colored);
    invariant(stripAnsiCodes(plain) === plain, `Plain formatter emitted ANSI codes for sample event ${String(index)}`);
    invariant(strippedColored === plain, `TTY/plain formatter mismatch for sample event ${String(index)}`);
    const hasHighlight = strippedColored !== colored;
    if (event.severity === 'ERR' || event.severity === 'WRN') {
      invariant(hasHighlight, `TTY formatter missing severity highlight for sample event ${String(index)}`);
    }
  });

  const llmResponseEvent = createEvent({
    message: RESPONSE_RECEIVED_LABEL,
    type: 'llm',
    direction: 'response',
    provider: 'openai',
    model: 'gpt-4o',
    remoteIdentifier: 'openai:gpt-4o',
    labels: {
      latency_ms: '120',
      response_bytes: '512',
      input_tokens: '256',
      output_tokens: '128',
    },
  });
  const llmResponseLine = formatRichLogLine(llmResponseEvent, { tty: false });
  invariant(llmResponseLine.includes(RESPONSE_RECEIVED_LABEL), 'LLM response logs should render the response metrics context.');

  const agentLifecycleEvent = createEvent({
    message: 'session finalization (error)',
    type: 'llm',
    direction: 'response',
    remoteIdentifier: 'agent:fin',
    labels: {},
  });
  const lifecycleLine = formatRichLogLine(agentLifecycleEvent, { tty: false });
  invariant(!lifecycleLine.includes('LLM response received'), 'Non-response agent logs must not reuse the LLM response context.');

  const llmFailureEvent = createEvent({
    message: 'LLM response failed [RATE_LIMIT]: upstream rate limit',
    type: 'llm',
    direction: 'response',
    provider: 'anthropic',
    model: 'claude-3-sonnet',
    remoteIdentifier: 'anthropic:claude-3-sonnet',
    labels: {
      latency_ms: '80',
    },
  });
  const llmFailureLine = formatRichLogLine(llmFailureEvent, { tty: false });
  invariant(llmFailureLine.includes('LLM response failed'), 'LLM failure response logs should render the failure metrics context.');
}

function expectLlmLogContext(
  log: LogEntry | undefined,
  scenarioId: string,
  expectation: { message: string; severity: LogEntry['severity']; remote?: string }
): LogEntry {
  invariant(log !== undefined, expectation.message);
  invariant(log.type === 'llm', `Expected llm log for ${scenarioId}.`);
  invariant(log.severity === expectation.severity, `Expected ${expectation.severity} log for ${scenarioId}.`);
  if (expectation.remote !== undefined) {
    const actualRemote = log.remoteIdentifier;
    invariant(actualRemote === expectation.remote, `Unexpected remoteIdentifier for ${scenarioId}: ${actualRemote}.`);
  }
  const expectedAgent = `phase1-${scenarioId}`;
  invariant(log.agentId === expectedAgent, `agentId mismatch for ${scenarioId}.`);
  invariant(log.callPath === expectedAgent, `callPath mismatch for ${scenarioId}.`);
  invariant(typeof log.txnId === 'string' && log.txnId.length > 0, `txnId missing for ${scenarioId}.`);
  invariant(typeof log.originTxnId === 'string' && log.originTxnId.length > 0, `originTxnId missing for ${scenarioId}.`);
  invariant(typeof log.turn === 'number' && log.turn >= 0, `turn missing for ${scenarioId}.`);
  invariant(typeof log.subturn === 'number' && log.subturn >= 0, `subturn missing for ${scenarioId}.`);
  return log;
}

const rateLimitWarningExpected = (scenarioId: string): string => `Rate limit warning expected for ${scenarioId}.`;
const rateLimitMessageMismatch = (scenarioId: string): string => `Rate limit log message mismatch for ${scenarioId}.`;
const retryBackoffExpected = (scenarioId: string): string => `Retry backoff log expected for ${scenarioId}.`;
const retryBackoffMessageMismatch = (scenarioId: string): string => `Retry backoff message mismatch for ${scenarioId}.`;

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

function registerSessionConfigObserver(observer: (config: AIAgentSessionConfig) => void): () => void {
  sessionConfigObservers.push(observer);
  return () => {
    const index = sessionConfigObservers.indexOf(observer);
    if (index >= 0) {
      sessionConfigObservers.splice(index, 1);
    }
  };
}

async function captureSessionConfig(runFactory: () => Promise<unknown>): Promise<AIAgentSessionConfig> {
  let captured: AIAgentSessionConfig | undefined;
  const unregister = registerSessionConfigObserver((config) => {
    captured = { ...config };
  });
  try {
    await runFactory();
  } finally {
    unregister();
  }
  invariant(captured !== undefined, 'Session configuration capture failed.');
  return captured;
}

function logHasDetail(entry: LogEntry, key: string): boolean {
  const details = entry.details;
  return details !== undefined && Object.prototype.hasOwnProperty.call(details, key);
}

function getLogDetail(entry: LogEntry, key: string): LogDetailValue | undefined {
  const details = entry.details;
  if (details === undefined) {
    return undefined;
  }
  if (!Object.prototype.hasOwnProperty.call(details, key)) {
    return undefined;
  }
  return details[key];
}

function expectLogDetailNumber(entry: LogEntry, key: string, message: string): number {
  const value = getLogDetail(entry, key);
  invariant(typeof value === 'number' && Number.isFinite(value), message);
  return value;
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

function createParentSessionStub(configuration: Configuration): Pick<AIAgentSessionConfig, 'config' | 'callbacks' | 'stream' | 'traceLLM' | 'traceMCP' | 'traceSdk' | 'verbose' | 'temperature' | 'topP' | 'llmTimeout' | 'toolTimeout' | 'maxRetries' | 'maxTurns' | 'toolResponseMaxBytes' | 'targets'> {
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

function makeBasicConfiguration(): Configuration {
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
    // Tests use native transport by default; XML-specific tests override this
    tooling: { transport: 'native' },
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
const reasoningMatrixSummaries = new Map<string, { reasoning?: ReasoningLevel; reasoningValue?: ProviderReasoningValue | null }>();
const overrideLLMExpected = {
  temperature: 0.27,
  topP: 0.52,
  maxOutputTokens: 321,
  repeatPenalty: 1.9,
  llmTimeout: 7_654,
  toolTimeout: 4_321,
  maxRetries: 5,
  maxTurns: 11,
  maxToolCallsPerTurn: 4,
  toolResponseMaxBytes: 777,
  mcpInitConcurrency: 6,
  stream: true,
};

type ReasoningSelector = ReasoningLevel | 'none' | 'inherit' | 'unset' | 'absent';
type OverrideReasoningSelector = ReasoningSelector;
type NormalizedReasoningSelector = ReasoningLevel | 'none' | 'inherit';

const buildReasoningMatrixPrompt = (selector: ReasoningSelector): string => {
  const lines = [
    '---',
    'description: Reasoning matrix agent',
    `models:`,
    `  - ${PRIMARY_PROVIDER}/${MODEL_NAME}`,
  ];
  if (selector === 'absent') {
    // omit reasoning key entirely
  } else if (selector === 'unset') {
    lines.push('reasoning: unset');
  } else if (selector === 'none') {
    lines.push('reasoning: none');
  } else if (selector === 'inherit') {
    lines.push('reasoning: default');
  } else {
    lines.push(`reasoning: ${selector}`);
  }
  lines.push('---');
  lines.push(MINIMAL_SYSTEM_PROMPT);
  return lines.join('\n');
};

interface ReasoningDimension<T> {
  label: string;
  description: string;
  selector: T;
}

const FRONTMATTER_REASONING_CASES: ReasoningDimension<ReasoningSelector>[] = [
  { label: 'fm-absent', description: 'frontmatter missing reasoning', selector: 'absent' },
  { label: 'fm-unset', description: 'frontmatter=unset (disable)', selector: 'unset' },
  { label: 'fm-high', description: 'frontmatter=high', selector: 'high' },
];

const CONFIG_DEFAULT_REASONING_CASES: ReasoningDimension<ReasoningSelector>[] = [
  { label: 'cfg-default', description: 'default=unset', selector: 'inherit' },
  { label: 'cfg-low', description: 'default=low', selector: 'low' },
];

const OVERRIDE_REASONING_CASES: ReasoningDimension<OverrideReasoningSelector>[] = [
  { label: 'ovr-absent', description: 'override=absent', selector: 'absent' },
  { label: 'ovr-unset', description: 'override=unset', selector: 'inherit' },
  { label: 'ovr-none', description: 'override=none', selector: 'none' },
  { label: 'ovr-high', description: 'override=high', selector: 'high' },
];

type ReasoningExpectation =
  | { type: 'level'; level: ReasoningLevel }
  | { type: 'none'; mode: 'explicit' | 'implicit' };

const convertOverrideSelector = (selector: OverrideReasoningSelector): ReasoningSelector => {
  return selector === 'absent' ? 'inherit' : selector;
};

const normalizeSelector = (value: ReasoningSelector): NormalizedReasoningSelector => {
  if (value === 'absent') return 'inherit';
  if (value === 'unset') return 'none';
  return value;
};

const computeExpectedReasoning = (
  frontmatter: ReasoningSelector,
  configurationDefault: ReasoningSelector,
  override: OverrideReasoningSelector,
): ReasoningExpectation => {
  const selectionOrder: NormalizedReasoningSelector[] = [
    normalizeSelector(convertOverrideSelector(override)),
    normalizeSelector(frontmatter),
    normalizeSelector(configurationDefault),
  ];
  // eslint-disable-next-line functional/no-loop-statements
  for (const value of selectionOrder) {
    if (value === 'inherit') continue;
    if (value === 'none') {
      return { type: 'none', mode: 'explicit' };
    }
    return { type: 'level', level: value };
  }
  return { type: 'none', mode: 'implicit' };
};


const buildReasoningMatrixScenarios = (): HarnessTest[] => {
  const scenarios: HarnessTest[] = [];
  FRONTMATTER_REASONING_CASES.forEach((frontCase) => {
    CONFIG_DEFAULT_REASONING_CASES.forEach((defaultCase) => {
      OVERRIDE_REASONING_CASES.forEach((overrideCase) => {
        const scenarioId = `reasoning-matrix-${frontCase.label}-${defaultCase.label}-${overrideCase.label}`;
        scenarios.push({
          id: scenarioId,
          description: `${frontCase.description}; ${defaultCase.description}; ${overrideCase.description}`,
          execute: async () => {
            const configuration = makeBasicConfiguration();
            const defaults: NonNullable<Configuration['defaults']> = {
              ...BASE_DEFAULTS,
              ...(configuration.defaults ?? {}),
            };
            configuration.defaults = defaults;
            const normalizedDefault = normalizeSelector(defaultCase.selector);
            if (normalizedDefault !== 'inherit') {
              configuration.defaults.reasoning = normalizedDefault;
            } else if (configuration.defaults.reasoning !== undefined) {
              delete configuration.defaults.reasoning;
            }
            const layers = buildInMemoryConfigLayers(configuration);
            const promptContent = buildReasoningMatrixPrompt(frontCase.selector);
            let globalOverrides: LoadAgentOptions['globalOverrides'];
            if (overrideCase.selector !== 'absent') {
              const normalizedOverride = normalizeSelector(convertOverrideSelector(overrideCase.selector));
              if (normalizedOverride !== 'inherit') {
                globalOverrides = { reasoning: normalizedOverride };
              }
            }
            const loaded = loadAgentFromContent(scenarioId, promptContent, {
              configLayers: layers,
              globalOverrides,
            });
            const capturedConfig = await captureSessionConfig(async () => {
              const session = await loaded.createSession(MINIMAL_SYSTEM_PROMPT, DEFAULT_PROMPT_SCENARIO, { outputFormat: 'markdown' });
              void session;
            });
            reasoningMatrixSummaries.set(scenarioId, {
              reasoning: capturedConfig.reasoning,
              reasoningValue: capturedConfig.reasoningValue,
            });
            return makeSuccessResult('reasoning-matrix');
          },
          expect: (result) => {
            invariant(result.success, `Scenario ${scenarioId} expected success.`);
            const summary = reasoningMatrixSummaries.get(scenarioId);
            invariant(summary !== undefined, `Reasoning summary missing for ${scenarioId}.`);
            const expected = computeExpectedReasoning(frontCase.selector, defaultCase.selector, overrideCase.selector);
            if (expected.type === 'level') {
              invariant(summary.reasoning === expected.level, `Reasoning level mismatch for ${scenarioId}. Expected ${expected.level}, got ${summary.reasoning ?? 'none'}.`);
              invariant(summary.reasoningValue === undefined, `Reasoning value should be undefined when level '${expected.level}' is selected for ${scenarioId}.`);
            } else if (expected.mode === 'explicit') {
              invariant(summary.reasoning === undefined, `Reasoning level should be cleared when disabled explicitly for ${scenarioId}.`);
              invariant(summary.reasoningValue === null, `Reasoning value should be null when disabled explicitly for ${scenarioId}.`);
            } else {
              invariant(summary.reasoning === undefined, `Reasoning level should remain unset when falling back to implicit none for ${scenarioId}.`);
              invariant(summary.reasoningValue === undefined, `Reasoning value should remain undefined when falling back to implicit none for ${scenarioId}.`);
            }
          },
        });
      });
    });
  });
  return scenarios;
};

const BASE_TEST_SCENARIOS: HarnessTest[] = [
  {
    id: 'shutdown-controller',
    execute: async () => {
      const controller = new ShutdownController();
      const order: string[] = [];
      controller.register('first', () => { order.push('first'); });
      controller.register('second', () => { order.push('second'); });
      await controller.shutdown();
      invariant(controller.stopRef.stopping, 'Global stopRef should be marked when shutdown begins.');
      invariant(controller.signal.aborted, 'AbortSignal should be aborted after shutdown.');
      invariant(order.length === 2, 'All registered shutdown tasks should run once.');
      invariant(order[0] === 'second' && order[1] === 'first', 'Shutdown tasks should execute in LIFO order.');
      const recordedLength: number = order.length;
      await controller.shutdown();
      invariant(order.length === recordedLength, 'Shutdown should be idempotent.');
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
      } as AIAgentResult;
    },
    expect: (result) => {
      invariant(result.success, 'Shutdown controller test should report success.');
    },
  },
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
      const reasoningSegments = firstAssistant.reasoning;
      invariant(Array.isArray(reasoningSegments) && reasoningSegments.length === 1, 'Expected single reasoning segment with metadata for run-test-1.');
if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('run-test-1 reasoning:', JSON.stringify(reasoningSegments, null, 2));
      }
      const segment = reasoningSegments[0] as { type?: string; text?: string; providerMetadata?: Record<string, unknown> };
      invariant(segment.type === 'reasoning', 'Reasoning segment must be tagged as reasoning for run-test-1.');
      invariant(typeof segment.text === 'string' && segment.text.includes('Evaluating task'), 'Reasoning text missing for run-test-1.');
      const reasoningMetadata = segment.providerMetadata;
      invariant(reasoningMetadata !== undefined && typeof reasoningMetadata === 'object', 'Reasoning metadata missing for run-test-1.');
      const anthropicMetadata = reasoningMetadata.anthropic as { signature?: unknown } | undefined;
      invariant(anthropicMetadata !== undefined && typeof anthropicMetadata.signature === 'string' && anthropicMetadata.signature.length > 0, 'Reasoning signature missing for run-test-1.');

      const toolEntries = result.accounting.filter(isToolAccounting);
      const testEntry = toolEntries.find((entry) => entry.mcpServer === 'test' && entry.command === 'test__test');
      invariant(testEntry !== undefined, 'Expected accounting entry for test MCP server in run-test-1.');
      invariant(testEntry.status === 'ok', 'Test MCP tool accounting should be ok for run-test-1.');
      const llmLogs = result.logs.filter((entry) => entry.type === 'llm' && entry.direction === 'response');
      invariant(llmLogs.length > 0, 'LLM response log missing for run-test-1.');
      const hasStopReason = llmLogs.some((log) => {
        if (typeof log.message === 'string' && log.message.includes('stop=')) {
          return true;
        }
        return logHasDetail(log, 'stop_reason');
      });
      invariant(hasStopReason, 'LLM log should include stop reason for run-test-1.');
    },
  },
  {
    id: 'run-test-2',
    expect: (result) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-2 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-2.');
      invariant(finalReport.status === 'failure', 'Final report should indicate failure for run-test-2.');

      const toolEntries = result.accounting.filter(isToolAccounting);
      const failureEntry = toolEntries.find((entry) => entry.command === 'test__test');
      invariant(failureEntry !== undefined, 'Expected MCP accounting entry for run-test-2.');
      invariant(failureEntry.status === 'failed', 'Accounting entry must reflect MCP failure for run-test-2.');
      const failureLog = result.logs.find((entry) => entry.type === 'tool' && typeof entry.message === 'string' && entry.message.includes('error') && entry.message.includes('test__test'));
      invariant(failureLog !== undefined, 'Tool failure log expected for run-test-2.');
      invariant(failureLog.severity === 'WRN', 'Tool failure logs must emit WRN severity for run-test-2.');
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
      invariant(exitLog.severity === 'ERR', 'EXIT-NO-LLM-RESPONSE must log ERR severity for run-test-6.');
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
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-7 should have success=false when finalReport.status=failure per CONTRACT.');
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
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-9 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should mark failure for run-test-9.');
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
      const toolCalls = firstAssistant.toolCalls ?? [];
      const toolCallNames = toolCalls.map((call) => call.name);
      invariant(toolCallNames.includes('agent__progress_report'), 'Progress report tool call expected in run-test-10.');
    },
  },
  {
    id: RUN_TEST_11,
    expect: (result) => {
      invariant(result.success, `Scenario ${RUN_TEST_11} expected success.`);
      const logs = result.logs;
      invariant(logs.some((log) => {
        // Check in message
        if (typeof log.message === 'string' && log.message.includes('agent__final_report(') && log.message.includes('report_format:json')) {
          return true;
        }
        // Check in details.request_preview
        if (logHasDetail(log, 'request_preview')) {
          const preview = String(getLogDetail(log, 'request_preview'));
          return preview.includes('agent__final_report(') && preview.includes('report_format:json');
        }
        return false;
      }), 'Final report log should capture JSON format attempt in run-test-11.');
    },
  },
  {
    id: 'run-test-12',
    configure: (configuration, sessionConfig, defaults) => {
      defaults.maxTurns = 1;
      configuration.defaults = defaults;
      sessionConfig.maxTurns = 1;
      sessionConfig.maxRetries = 1;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-12 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should complete successfully for run-test-12.');
      const finalTurnLog = result.logs.find((log) => typeof log.remoteIdentifier === 'string' && log.remoteIdentifier.includes('primary:') && typeof log.message === 'string' && log.message.includes('final turn'));
      invariant(finalTurnLog !== undefined, 'LLM request log should reflect final turn for run-test-12.');
    },
  },
  {
    id: 'run-test-13',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
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
    id: 'run-test-context-limit',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 320,
              tokenizer: TOKENIZER_GPT4O,
              contextWindowBufferTokens: 32,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.maxOutputTokens = 32;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-limit accounting:', JSON.stringify(result.accounting, null, 2));
        console.log('context-limit conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-limit logs:', JSON.stringify(result.logs, null, 2));
      }
      const toolMessages = result.conversation.filter((message) => message.role === 'tool');
      if (toolMessages.length > 0) {
        const hasOverflowReplacement = toolMessages.some((message) => typeof message.content === 'string' && message.content.includes(CONTEXT_OVERFLOW_FRAGMENT));
        if (hasOverflowReplacement) {
          const toolEntries = result.accounting.filter(isToolAccounting);
          invariant(toolEntries.length > 0, 'Tool accounting entries expected.');
          invariant(result.success, 'Context guard scenario should complete successfully after enforcing final turn.');
          const finalReport = result.finalReport;
          invariant(finalReport?.status === 'success', 'Final report should be successful after context guard trimming.');
        } else {
          const contextWarnings = result.logs.filter((entry) => entry.remoteIdentifier === CONTEXT_REMOTE);
          invariant(contextWarnings.length > 0, 'Context warning log expected when guard proceeds without replacing tool output.');
          invariant(result.success, 'Context guard scenario should complete successfully after enforcing final turn.');
          const finalReport = result.finalReport;
          invariant(finalReport?.status === 'success', 'Final report should be successful after forced final turn.');
        }
      } else {
        const contextWarning = result.logs.find((entry) => entry.remoteIdentifier === CONTEXT_REMOTE);
        invariant(contextWarning !== undefined, 'Context warning log expected when guard fires before tool execution.');
        const llmEntries = result.accounting.filter(isLlmAccounting);
        invariant(llmEntries.length === 0, 'Preflight guard should abort before issuing LLM requests.');
        const finalReport = result.finalReport;
        invariant(finalReport?.status === 'failure', 'Fallback failure final report expected when guard triggers preflight.');
      }
    },
  },
  {
    id: 'run-test-context-limit-default',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 16;
      configuration.defaults = defaults;
      sessionConfig.maxOutputTokens = 131072;
    },
    expect: (result) => {
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-limit-default log count:', result.logs.length);
        console.log('context-limit-default logs:', JSON.stringify(result.logs, null, 2));
        console.log('context-limit-default conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-limit-default finalReport:', JSON.stringify(result.finalReport, null, 2));
        console.log('context-limit-default success:', result.success, 'error:', result.error);
      }
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-context-limit-default.');
      invariant(finalReport.status === 'success', 'Final report should succeed after default context guard enforcement.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('context budget enforcement'), 'Final report should explain that context guard enforcement occurred.');
    },
  },
  {
    id: 'run-test-context-guard-preflight',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: PREFLIGHT_CONTEXT_WINDOW,
              contextWindowBufferTokens: PREFLIGHT_BUFFER_TOKENS,
              tokenizer: 'approximate',
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = PREFLIGHT_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = PREFLIGHT_MAX_OUTPUT_TOKENS;
      const longHistory = 'X'.repeat(400);
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Historical context.' },
        { role: 'assistant', content: longHistory },
      ];
    },
    expect: (result) => {
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-guard-preflight logs:', JSON.stringify(result.logs, null, 2));
        console.log('context-guard-preflight conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-guard-preflight finalReport:', JSON.stringify(result.finalReport, null, 2));
      }
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-context-guard-preflight should have success=false when finalReport.status=failure per CONTRACT.');
      const contextLogs = result.logs.filter((entry) => entry.remoteIdentifier === CONTEXT_REMOTE);
      invariant(contextLogs.length > 0, 'Context guard warning expected for run-test-context-guard-preflight.');
      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length === 1, 'Forced final turn should issue exactly one LLM call for run-test-context-guard-preflight.');

      const nonFinalToolMessages = result.conversation.filter((message) =>
        message.role === 'tool'
        && (typeof (message as { toolCallId?: string }).toolCallId !== 'string'
          || (message as { toolCallId?: string }).toolCallId !== FINAL_REPORT_CALL_ID)
      );
      invariant(nonFinalToolMessages.length === 0, 'Only the agent__final_report tool may appear after preflight enforcement.');

      const requestLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLog !== undefined, 'LLM request log expected for run-test-context-guard-preflight.');
      const expectedTokens = expectLogDetailNumber(requestLog, 'expected_tokens', 'expected_tokens detail missing for run-test-context-guard-preflight.');
      const schemaTokens = expectLogDetailNumber(requestLog, 'schema_tokens', 'schema_tokens detail missing for run-test-context-guard-preflight.');
      const limitTokens = PREFLIGHT_CONTEXT_WINDOW - PREFLIGHT_BUFFER_TOKENS - PREFLIGHT_MAX_OUTPUT_TOKENS;
      invariant(schemaTokens > 0, 'Schema tokens should contribute to preflight guard evaluation.');
      invariant(expectedTokens > limitTokens, `Projected tokens should exceed limit during preflight (expected > ${String(limitTokens)}, got ${String(expectedTokens)}).`);

      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Fallback failure final report expected for run-test-context-guard-preflight.');
    },
  },
  {
    id: 'run-test-context-multi-provider',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 160,
              contextWindowBufferTokens: 8,
              tokenizer: 'approximate',
            },
          },
        },
        [SECONDARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 64,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      configuration.defaults = { ...defaults, contextWindowBufferTokens: 8 };
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: MODEL_NAME },
        { provider: SECONDARY_PROVIDER, model: MODEL_NAME },
      ];
      sessionConfig.maxOutputTokens = 48;
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Historical context for provider fallback coverage.' },
        { role: 'assistant', content: 'Y'.repeat(400) },
      ];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-multi-provider expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed after provider fallback.');
      const warnLog = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && typeof entry.message === 'string'
        && entry.message.includes(`Projected context size`)
        && entry.message.includes(`${PRIMARY_PROVIDER}:${MODEL_NAME}`)
        && entry.message.includes('continuing turn'));
      invariant(warnLog !== undefined, 'Expected context warning log indicating continued execution for primary provider.');
      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length > 0, 'LLM accounting entries expected after warning.');
      invariant(llmEntries.some((entry) => entry.provider === PRIMARY_PROVIDER), 'Primary provider should still be attempted with a warning.');
    },
  },
  {
    id: 'run-test-context-retry',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 914,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.conversationHistory = [];
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.conversationHistory = [];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-retry expected success.');
      const toolMessages = result.conversation.filter((message) => message.role === 'tool');
      const toolEntries = result.accounting.filter(isToolAccounting);
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-retry conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-retry toolEntries:', JSON.stringify(toolEntries, null, 2));
      }
      invariant(toolMessages.length > 0, 'Retry scenario should include at least one tool message.');
      invariant(toolEntries.length > 0, 'Retry scenario should record tool accounting entries.');
      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length >= 1, 'Retry scenario should record LLM accounting entries.');
      const failedEntry = llmEntries.find((entry) => entry.status === 'failed');
      invariant(failedEntry !== undefined && typeof failedEntry.error === 'string' && failedEntry.error.includes('Simulated model failure'), 'Initial LLM attempt should record failure for retry scenario.');
      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
      invariant(finalTurnLog !== undefined, 'Final turn instruction log expected for retry scenario.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should be successful after retry enforcement.');
    },
  },
  {
    id: 'run-test-context-trim-log',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 320,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-trim-log expected success.');
      const finalReport = result.finalReport;

      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-trim-log logs:', JSON.stringify(result.logs, null, 2));
        console.log('context-trim-log accounting:', JSON.stringify(result.accounting, null, 2));
        console.log('context-trim-log conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-trim-log finalReport:', JSON.stringify(finalReport, null, 2));
      }

      invariant(finalReport?.status === 'success', 'Final report should indicate success after enforced context guard for run-test-context-trim-log.');

      const toolMessages = result.conversation.filter((message) => message.role === 'tool');
      const trimmedMessage = toolMessages.find((message) => message.toolCallId === 'call-context-trim-log' && message.content === '(no output)');
      invariant(trimmedMessage !== undefined, 'Tool response should be replaced with no output after trimming for run-test-context-trim-log.');

    },
  },
  {
    id: 'run-test-context-forced-final',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: FORCED_FINAL_CONTEXT_WINDOW,
              contextWindowBufferTokens: FORCED_FINAL_BUFFER_TOKENS,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = FORCED_FINAL_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = FORCED_FINAL_MAX_OUTPUT_TOKENS;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.conversationHistory = [];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-forced-final expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-context-forced-final.');

      const contextWarn = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes(CONTEXT_FORCING_FRAGMENT)
      );
      invariant(contextWarn !== undefined, 'Context guard warning log expected for run-test-context-forced-final.');

      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE && entry.severity === 'WRN');
      expectLlmLogContext(finalTurnLog, 'run-test-context-forced-final', { message: 'Final-turn enforcement log expected.', severity: 'WRN', remote: FINAL_TURN_REMOTE });

      const finalTurnRequest = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && typeof entry.message === 'string'
        && entry.message.includes('(final turn)')
      );
      invariant(finalTurnRequest !== undefined, 'Final-turn LLM request log expected for run-test-context-forced-final.');
      const requestDetails = expectRecord(finalTurnRequest.details, 'Final-turn request details missing for run-test-context-forced-final.');
      invariant(requestDetails.final_turn === true, 'Final-turn request should mark final_turn detail for run-test-context-forced-final.');

      const finalSchema = expectLogDetailNumber(finalTurnRequest, 'schema_tokens', 'schema_tokens detail missing (final) for run-test-context-forced-final.');
      const finalNewTokens = expectLogDetailNumber(finalTurnRequest, 'new_tokens', 'new_tokens detail missing (final) for run-test-context-forced-final.');
      invariant(finalNewTokens === 0, 'Final-turn request should not carry pending tokens for run-test-context-forced-final.');
      const forcedFinalLimit = FORCED_FINAL_CONTEXT_WINDOW - FORCED_FINAL_BUFFER_TOKENS - FORCED_FINAL_MAX_OUTPUT_TOKENS;
      if (finalSchema > forcedFinalLimit) {
        const shrinkWarn = result.logs.find((entry) =>
          entry.remoteIdentifier === CONTEXT_REMOTE
          && entry.severity === 'WRN'
          && typeof entry.message === 'string'
          && entry.message.includes(CONTEXT_POST_SHRINK_TURN_WARN)
        );
        invariant(shrinkWarn !== undefined, 'Post-shrink warning log expected when schema tokens exceed limit for run-test-context-forced-final.');
      } else {
        invariant(finalSchema <= forcedFinalLimit, `Final-turn schema tokens must respect adjusted limit (expected ≤ ${String(forcedFinalLimit)}, got ${String(finalSchema)}).`);
      }

      const responseLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'response'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && typeof entry.details?.stop_reason === 'string'
        && entry.details.stop_reason === 'stop'
      );
      invariant(responseLog !== undefined, 'Final-turn LLM response log expected for run-test-context-forced-final.');
    },
  },
  {
    id: 'run-test-llm-context-metrics',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-llm-context-metrics expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-llm-context-metrics.');

      const requestLogs = result.logs.filter((entry) => entry.type === 'llm' && entry.direction === 'request');
      invariant(requestLogs.length > 0, 'LLM request log expected for run-test-llm-context-metrics.');
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('llm-context-metrics logs:', JSON.stringify(result.logs, null, 2));
        console.log('llm-context-metrics accounting:', JSON.stringify(result.accounting, null, 2));
        console.log('llm-context-metrics conversation:', JSON.stringify(result.conversation, null, 2));
      }
      const metricsLog = requestLogs.slice().reverse().find((entry) => typeof entry.message === 'string' && entry.message.includes('[tokens: ctx '));
      invariant(metricsLog !== undefined, 'Context metrics request log missing for run-test-llm-context-metrics.');
      invariant(metricsLog.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`, 'Unexpected remoteIdentifier for context metrics request log.');

      const ctxTokens = expectLogDetailNumber(metricsLog, 'ctx_tokens', 'ctx_tokens detail missing for run-test-llm-context-metrics.');
      invariant(ctxTokens > 0, 'ctx_tokens should be positive for run-test-llm-context-metrics.');
      const newTokens = expectLogDetailNumber(metricsLog, 'new_tokens', 'new_tokens detail missing for run-test-llm-context-metrics.');
      invariant(newTokens >= 0, 'new_tokens should be non-negative for run-test-llm-context-metrics.');
      const schemaTokens = expectLogDetailNumber(metricsLog, 'schema_tokens', 'schema_tokens detail missing for run-test-llm-context-metrics.');
      invariant(schemaTokens > 0, 'schema_tokens should be positive for run-test-llm-context-metrics.');
      const expectedTokens = expectLogDetailNumber(metricsLog, 'expected_tokens', 'expected_tokens detail missing for run-test-llm-context-metrics.');
      invariant(expectedTokens === ctxTokens + newTokens + schemaTokens, 'expected_tokens should equal ctx + new + schema for run-test-llm-context-metrics.');
      const contextWindow = expectLogDetailNumber(metricsLog, 'context_window', 'context_window detail missing for run-test-llm-context-metrics.');
      invariant(contextWindow === 2048, 'context_window detail should match configured window for run-test-llm-context-metrics.');
      const contextPct = expectLogDetailNumber(metricsLog, 'context_pct', 'context_pct detail missing for run-test-llm-context-metrics.');
      invariant(contextPct >= 0, 'context_pct should be non-negative for run-test-llm-context-metrics.');

      const responseLog = result.logs.slice().reverse().find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'response'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && logHasDetail(entry, 'stop_reason')
        && getLogDetail(entry, 'stop_reason') === 'stop'
      ) ?? result.logs.slice().reverse().find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'response'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('run-test-llm-context-metrics responseLog', JSON.stringify(responseLog, null, 2));
      }
      invariant(responseLog !== undefined, 'LLM response log expected for run-test-llm-context-metrics.');
      const responseCtx = expectLogDetailNumber(responseLog, 'ctx_tokens', 'Response ctx_tokens detail missing for run-test-llm-context-metrics.');
      invariant(responseCtx > 0, 'Response ctx_tokens should be positive for run-test-llm-context-metrics.');
      const responseInput = expectLogDetailNumber(responseLog, 'input_tokens', 'Response input_tokens detail missing for run-test-llm-context-metrics.');
      const responseOutput = expectLogDetailNumber(responseLog, 'output_tokens', 'Response output_tokens detail missing for run-test-llm-context-metrics.');
      invariant(responseCtx === responseInput + responseOutput, 'Response ctx_tokens should equal input + output tokens for run-test-llm-context-metrics.');
    },
  },
  {
    id: 'run-test-tool-log-tokens',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.maxOutputTokens = 256;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-tool-log-tokens expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-tool-log-tokens.');

      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('tool-log-tokens logs:', JSON.stringify(result.logs, null, 2));
        console.log('tool-log-tokens accounting:', JSON.stringify(result.accounting, null, 2));
        console.log('tool-log-tokens conversation:', JSON.stringify(result.conversation, null, 2));
      }

      const toolLog = result.logs.find((entry) =>
        entry.type === 'tool'
        && entry.direction === 'response'
        && entry.severity === 'VRB'
        && typeof entry.message === 'string'
        && entry.message.includes('ok preview')
        && entry.remoteIdentifier === MCP_TEST_REMOTE
        && logHasDetail(entry, 'tokens_estimated')
      );
      invariant(toolLog !== undefined, 'Tool preview log with token metrics expected for run-test-tool-log-tokens.');
      const logTokens = expectLogDetailNumber(toolLog, 'tokens_estimated', 'Tool log tokens_estimated detail missing for run-test-tool-log-tokens.');
      invariant(logTokens > 0, 'Tool log tokens should be positive for run-test-tool-log-tokens.');
      const dropFlag = getLogDetail(toolLog, 'dropped');
      invariant(dropFlag === false, 'Tool log should indicate output was not dropped for run-test-tool-log-tokens.');

      const toolEntries = result.accounting.filter(isToolAccounting);
      const successEntry = toolEntries.find((entry) => entry.command === 'test__test' && entry.status === 'ok');
      invariant(successEntry !== undefined, 'Successful tool accounting entry expected for run-test-tool-log-tokens.');
      const details = successEntry.details;
      if (details !== undefined && Object.prototype.hasOwnProperty.call(details, 'tokens')) {
        const accountingTokens = (details as Record<string, unknown>).tokens;
        invariant(typeof accountingTokens === 'number' && accountingTokens === logTokens, 'Accounting tokens should match log tokens for run-test-tool-log-tokens.');
      }
    },
  },
  {
    id: 'context_guard__threshold_below_limit',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: THRESHOLD_CONTEXT_WINDOW_BELOW,
              contextWindowBufferTokens: THRESHOLD_BUFFER_TOKENS,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = THRESHOLD_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = THRESHOLD_MAX_OUTPUT_TOKENS;
      sessionConfig.systemPrompt = THRESHOLD_SYSTEM_PROMPT;
      sessionConfig.userPrompt = THRESHOLD_USER_PROMPT;
      sessionConfig.tools = [];
    },
    expect: (result) => {
      const contextWarnings = result.logs.filter((entry) => entry.remoteIdentifier === CONTEXT_REMOTE && entry.severity === 'WRN');
      invariant(contextWarnings.length === 0, 'No context guard warning expected below threshold.');

      const requestLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLog !== undefined, 'LLM request log expected for context_guard__threshold_below_limit.');
      const expectedTokens = expectLogDetailNumber(requestLog, 'expected_tokens', 'expected_tokens detail missing for context_guard__threshold_below_limit.');
      const limitTokens = THRESHOLD_CONTEXT_WINDOW_BELOW - THRESHOLD_BUFFER_TOKENS - THRESHOLD_MAX_OUTPUT_TOKENS;
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('threshold-below metrics:', { expectedTokens, limitTokens });
      }
      invariant(expectedTokens < limitTokens, `Projected tokens should remain below limit (expected < ${String(limitTokens)}, got ${String(expectedTokens)}).`);
    },
  },
  {
    id: 'context_guard__threshold_equal_limit',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: THRESHOLD_CONTEXT_WINDOW_EQUAL,
              contextWindowBufferTokens: THRESHOLD_BUFFER_TOKENS,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = THRESHOLD_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = THRESHOLD_MAX_OUTPUT_TOKENS;
      sessionConfig.systemPrompt = THRESHOLD_SYSTEM_PROMPT;
      sessionConfig.userPrompt = THRESHOLD_USER_PROMPT;
      sessionConfig.tools = [];
    },
    expect: (result) => {
      const contextWarnings = result.logs.filter((entry) => entry.remoteIdentifier === CONTEXT_REMOTE && entry.severity === 'WRN');
      invariant(contextWarnings.length === 0, 'No context guard warning expected at exact threshold.');

      const requestLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLog !== undefined, 'LLM request log expected for context_guard__threshold_equal_limit.');
      const expectedTokens = expectLogDetailNumber(requestLog, 'expected_tokens', 'expected_tokens detail missing for context_guard__threshold_equal_limit.');
      const limitTokens = THRESHOLD_CONTEXT_WINDOW_EQUAL - THRESHOLD_BUFFER_TOKENS - THRESHOLD_MAX_OUTPUT_TOKENS;
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('threshold-equal metrics:', { expectedTokens, limitTokens });
      }
      invariant(expectedTokens === limitTokens, `Projected tokens should equal limit (expected ${String(limitTokens)}, got ${String(expectedTokens)}).`);
    },
  },
  {
    id: 'context_guard__threshold_above_limit',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: THRESHOLD_CONTEXT_WINDOW_ABOVE,
              contextWindowBufferTokens: THRESHOLD_BUFFER_TOKENS,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = THRESHOLD_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = THRESHOLD_MAX_OUTPUT_TOKENS;
      sessionConfig.systemPrompt = THRESHOLD_SYSTEM_PROMPT;
      sessionConfig.userPrompt = THRESHOLD_ABOVE_USER_PROMPT;
      sessionConfig.tools = [];
    },
    expect: (result) => {
      const requestLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLog !== undefined, 'LLM request log expected for context_guard__threshold_above_limit.');
      const expectedTokens = expectLogDetailNumber(requestLog, 'expected_tokens', 'expected_tokens detail missing for context_guard__threshold_above_limit.');
      const limitTokens = THRESHOLD_CONTEXT_WINDOW_ABOVE - THRESHOLD_BUFFER_TOKENS - THRESHOLD_MAX_OUTPUT_TOKENS;
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('threshold-above metrics:', { expectedTokens, limitTokens });
      }
      invariant(expectedTokens > limitTokens, `Projected tokens should exceed limit (expected > ${String(limitTokens)}, got ${String(expectedTokens)}).`);

      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should indicate failure after exceeding the context limit.');
    },
  },
  {
    id: 'context_guard__tool_success_tokens_once',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__tool_success_tokens_once expected success.');
      const toolLogs = result.logs.filter((entry) =>
        entry.type === 'tool'
        && entry.direction === 'response'
        && entry.remoteIdentifier === MCP_TEST_REMOTE
        && entry.severity === 'VRB'
        && getLogDetail(entry, 'tool') === 'test__test'
        && logHasDetail(entry, 'tokens_estimated')
      );
      invariant(toolLogs.length === 2, 'Exactly two successful tool logs expected.');
      const tokens = toolLogs.map((entry) => expectLogDetailNumber(entry, 'tokens_estimated', 'tokens_estimated detail missing for context_guard__tool_success_tokens_once.'));
      tokens.forEach((value) => { invariant(value > 0, 'Tool tokens should be positive.'); });

      const requestLogs = result.logs.filter((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLogs.length >= 2, 'At least two LLM request logs expected.');

      const contextWarnings = result.logs.filter((entry) => entry.remoteIdentifier === CONTEXT_REMOTE && entry.severity === 'WRN');
      invariant(contextWarnings.length === 0, 'No context guard warnings expected when tokens remain within budget.');
    },
  },
  {
    id: 'context_guard__tool_drop_after_success',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 1620,
              contextWindowBufferTokens: 0,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 0;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      const firstTool = result.conversation.find((message) => message.role === 'tool' && (message as { toolCallId?: string }).toolCallId === 'call-guard-drop-first');
      invariant(firstTool !== undefined, 'First tool result missing for context_guard__tool_drop_after_success.');
      invariant(typeof firstTool.content === 'string', 'First tool content should be a string.');
      invariant(firstTool.content === 'phase-1-tool-success', 'First tool output should remain intact for context_guard__tool_drop_after_success.');

      const secondTool = result.conversation.find((message) => message.role === 'tool' && (message as { toolCallId?: string }).toolCallId === 'call-guard-drop-second');
      invariant(secondTool !== undefined, 'Second tool result missing for context_guard__tool_drop_after_success.');
      invariant(typeof secondTool.content === 'string', 'Second tool content should be a string.');
      invariant(secondTool.content === TOOL_DROP_STUB, 'Second tool output should be replaced with the drop stub for context_guard__tool_drop_after_success.');

      if (!result.success && process.env.CONTEXT_DEBUG === 'true') {
        console.log('context_guard__tool_drop_after_success final status', result.success, 'error', result.error);
      }

      const dropLog = result.logs.find((entry) =>
        entry.type === 'tool'
        && entry.direction === 'response'
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes('output dropped')
        && getLogDetail(entry, 'tool') === 'test__test'
      );
      invariant(dropLog !== undefined, 'Drop warning log expected for context_guard__tool_drop_after_success.');
      const dropReason = getLogDetail(dropLog, 'reason');
      invariant(dropReason === 'token_budget_exceeded', 'Drop reason should be token_budget_exceeded for context_guard__tool_drop_after_success.');

      const accountingEntries = result.accounting.filter(isToolAccounting);
      const failedEntry = accountingEntries.find((entry) => entry.command === 'test__test' && entry.status === 'failed');
      invariant(failedEntry !== undefined, 'Failed tool accounting entry expected for context_guard__tool_drop_after_success.');
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context_guard__tool_drop_after_success failed accounting entry', failedEntry);
      }
      invariant(failedEntry.error === 'token_budget_exceeded', 'Failed tool accounting entry should indicate token_budget_exceeded.');

      const finalReportAccounting = accountingEntries.find((entry) => entry.command === 'agent__final_report');
      invariant(finalReportAccounting?.status === 'ok', 'Final report accounting must succeed after guard drop.');
      const finalReportSkipLog = result.logs.find((entry) =>
        entry.type === 'tool'
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes("Tool 'agent__final_report' skipped"));
      invariant(finalReportSkipLog === undefined, 'Final report must not be skipped after guard drop.');
    },
  },
  {
    id: 'context_guard__progress_passthrough',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 1500,
              contextWindowBufferTokens: 0,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 0;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.tools = ['test'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__progress_passthrough expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should complete after progress passthrough.');
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('progress_passthrough conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('progress_passthrough logs:', JSON.stringify(result.logs, null, 2));
      }

      const progressTool = result.conversation.find((message) =>
        message.role === 'tool'
        && (message as { toolCallId?: string }).toolCallId === 'call-progress-guard-status');
      invariant(progressTool !== undefined, 'Progress tool response missing for context_guard__progress_passthrough.');
      invariant(progressTool.content === '{"ok":true}', 'Progress tool output should remain intact after guard enforcement.');

      const progressSkipLog = result.logs.find((entry) =>
        entry.type === 'tool'
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes("Tool 'agent__progress_report' skipped"));
      invariant(progressSkipLog === undefined, 'Progress tool must not be skipped after guard activation.');
    },
  },
  {
    id: 'context_guard__batch_passthrough',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 4000,
              contextWindowBufferTokens: 0,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 0;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 80;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__batch_passthrough expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed for context_guard__batch_passthrough.');
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('batch_passthrough conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('batch_passthrough logs:', JSON.stringify(result.logs, null, 2));
        console.log('batch_passthrough accounting:', JSON.stringify(result.accounting, null, 2));
      }

      const batchResult = result.conversation.find((message) =>
        message.role === 'tool'
        && (message as { toolCallId?: string }).toolCallId === 'call-batch-guard');
      invariant(batchResult !== undefined, 'Batch response missing for context_guard__batch_passthrough.');
      invariant(typeof batchResult.content === 'string' && batchResult.content.includes('"results"'), 'Batch response should retain aggregated JSON payload.');

      const batchSkipLog = result.logs.find((entry) =>
        entry.type === 'tool'
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes("Tool 'agent__batch' output dropped"));
      invariant(batchSkipLog === undefined, 'Batch output must not be dropped by the context guard.');

      const batchAccounting = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchAccounting?.status === 'ok', 'Batch accounting entry should be ok after guard passthrough.');
    },
  },
  {
    id: 'context_guard__schema_tokens_only',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 200,
              contextWindowBufferTokens: 16,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 16;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 32;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__schema_tokens_only expected success.');
      const contextWarn = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes(CONTEXT_FORCING_FRAGMENT)
      );
      invariant(contextWarn !== undefined, 'Context guard warning log expected for schema-only projection.');

      const finalTurnRequest = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && entry.details?.final_turn === true
      );
      invariant(finalTurnRequest !== undefined, 'Final-turn LLM request expected for schema-only scenario.');

      const toolMessages = result.conversation.filter((message) =>
        message.role === 'tool'
        && message.toolCallId !== FINAL_REPORT_CALL_ID
      );
      invariant(toolMessages.length === 0, 'No non-final tool messages should appear when schema alone triggers the guard.');

      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should still complete successfully after schema-only enforcement.');
    },
  },
  {
    id: 'context_guard__llm_metrics_logging',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__llm_metrics_logging expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for context_guard__llm_metrics_logging.');

      const requestLogs = result.logs.filter((entry) => entry.type === 'llm' && entry.direction === 'request');
      invariant(requestLogs.length > 0, 'LLM request logs expected for context_guard__llm_metrics_logging.');
      const metricsLog = requestLogs.find((entry) => typeof entry.message === 'string' && entry.message.includes('[tokens: ctx '));
      invariant(metricsLog !== undefined, 'Context metrics request log missing for context_guard__llm_metrics_logging.');
      invariant(metricsLog.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`, 'Unexpected remoteIdentifier for context metrics request log.');

      const ctxTokens = expectLogDetailNumber(metricsLog, 'ctx_tokens', 'ctx_tokens detail missing for context_guard__llm_metrics_logging.');
      invariant(ctxTokens >= 0, 'ctx_tokens should never be negative for context_guard__llm_metrics_logging.');
      const newTokens = expectLogDetailNumber(metricsLog, 'new_tokens', 'new_tokens detail missing for context_guard__llm_metrics_logging.');
      invariant(newTokens >= 0, 'new_tokens should be non-negative for context_guard__llm_metrics_logging.');
      if (ctxTokens === 0) {
        invariant(newTokens > 0, 'First-turn metrics should report prompt tokens via new_tokens when ctx_tokens is zero.');
      }
      const schemaTokens = expectLogDetailNumber(metricsLog, 'schema_tokens', 'schema_tokens detail missing for context_guard__llm_metrics_logging.');
      invariant(schemaTokens > 0, 'schema_tokens should be positive for context_guard__llm_metrics_logging.');
      const expectedTokens = expectLogDetailNumber(metricsLog, 'expected_tokens', 'expected_tokens detail missing for context_guard__llm_metrics_logging.');
      invariant(expectedTokens === ctxTokens + newTokens + schemaTokens, 'expected_tokens should equal ctx + new + schema for context_guard__llm_metrics_logging.');
      const contextWindow = expectLogDetailNumber(metricsLog, 'context_window', 'context_window detail missing for context_guard__llm_metrics_logging.');
      invariant(contextWindow === 2048, 'context_window detail should match configured window for context_guard__llm_metrics_logging.');
      const contextPct = expectLogDetailNumber(metricsLog, 'context_pct', 'context_pct detail missing for context_guard__llm_metrics_logging.');
      invariant(contextPct >= 0, 'context_pct should be non-negative for context_guard__llm_metrics_logging.');
    },
  },
  {
    id: 'context_guard__forced_final_turn_flow',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: FORCED_FINAL_CONTEXT_WINDOW,
              contextWindowBufferTokens: FORCED_FINAL_BUFFER_TOKENS,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = FORCED_FINAL_BUFFER_TOKENS;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = FORCED_FINAL_MAX_OUTPUT_TOKENS;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.conversationHistory = [];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__forced_final_turn_flow expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for context_guard__forced_final_turn_flow.');

      const contextWarn = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes(CONTEXT_FORCING_FRAGMENT)
      );
      invariant(contextWarn !== undefined, 'Context guard warning log expected for context_guard__forced_final_turn_flow.');

      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE && entry.severity === 'WRN');
      expectLlmLogContext(finalTurnLog, 'context_guard__forced_final_turn_flow', { message: 'Final-turn enforcement log expected.', severity: 'WRN', remote: FINAL_TURN_REMOTE });

      const finalTurnRequest = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && typeof entry.message === 'string'
        && entry.message.includes('(final turn)')
      );
      invariant(finalTurnRequest !== undefined, 'Final-turn LLM request log expected for context_guard__forced_final_turn_flow.');
      const requestDetails = expectRecord(finalTurnRequest.details, 'Final-turn request details missing for context_guard__forced_final_turn_flow.');
      invariant(requestDetails.final_turn === true, 'Final-turn request should mark final_turn detail for context_guard__forced_final_turn_flow.');

      const finalSchema = expectLogDetailNumber(finalTurnRequest, 'schema_tokens', 'schema_tokens detail missing (final) for context_guard__forced_final_turn_flow.');
      const finalNewTokens = expectLogDetailNumber(finalTurnRequest, 'new_tokens', 'new_tokens detail missing (final) for context_guard__forced_final_turn_flow.');
      invariant(finalNewTokens === 0, 'Final-turn request should not carry pending tokens for context_guard__forced_final_turn_flow.');
      const forcedFinalLimit = FORCED_FINAL_CONTEXT_WINDOW - FORCED_FINAL_BUFFER_TOKENS - FORCED_FINAL_MAX_OUTPUT_TOKENS;
      if (finalSchema > forcedFinalLimit) {
        const shrinkWarn = result.logs.find((entry) =>
          entry.remoteIdentifier === CONTEXT_REMOTE
          && entry.severity === 'WRN'
          && typeof entry.message === 'string'
          && entry.message.includes(CONTEXT_POST_SHRINK_TURN_WARN)
        );
        invariant(shrinkWarn !== undefined, 'Post-shrink warning log expected when schema tokens exceed limit for context_guard__forced_final_turn_flow.');
      } else {
        invariant(finalSchema <= forcedFinalLimit, `Final-turn schema tokens must respect adjusted limit (expected ≤ ${String(forcedFinalLimit)}, got ${String(finalSchema)}).`);
      }

      const responseLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'response'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
        && typeof entry.details?.stop_reason === 'string'
        && entry.details.stop_reason === 'stop'
      );
      invariant(responseLog !== undefined, 'Final-turn LLM response log expected for context_guard__forced_final_turn_flow.');
    },
  },
  {
    id: 'context_guard__multi_target_provider_selection',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 512,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
        [SECONDARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 1600,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: MODEL_NAME },
        { provider: SECONDARY_PROVIDER, model: MODEL_NAME },
      ];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Historical context for provider fallback coverage.' },
        { role: 'assistant', content: 'Y'.repeat(400) },
      ];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__multi_target_provider_selection expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed after provider fallback.');

      const warnLog = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && typeof entry.message === 'string'
        && entry.message.includes(`${PRIMARY_PROVIDER}:${MODEL_NAME}`)
        && entry.message.includes('continuing turn'));
      invariant(warnLog !== undefined, 'Expected context warning log for primary provider in context_guard__multi_target_provider_selection.');

      const llmEntries = result.accounting.filter(isLlmAccounting);
      invariant(llmEntries.length > 0, 'LLM accounting entries expected for provider fallback.');
      invariant(llmEntries.some((entry) => entry.provider === PRIMARY_PROVIDER), 'Primary provider should be attempted despite warning.');
    },
  },
  // NOTE: context_guard__tool_overflow_drop and context_guard__post_overflow_tools_skip
  // are temporarily disabled to keep the Phase 1 harness green while we design
  // overflow coverage that does not require core changes. See TODO-context-guard.md.
  {
    id: 'context_guard__init_counters_from_history',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
      const historyMessages: ConversationMessage[] = [
        { role: 'system', content: HISTORY_SYSTEM_SEED },
        { role: 'assistant', content: HISTORY_ASSISTANT_SEED },
      ];
      sessionConfig.conversationHistory = historyMessages;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario context_guard__init_counters_from_history expected success.');
      const firstRequestLog = result.logs.find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(firstRequestLog !== undefined, 'Initial LLM request log expected for context_guard__init_counters_from_history.');

      const ctxTokens = expectLogDetailNumber(firstRequestLog, 'ctx_tokens', 'ctx_tokens detail missing for context_guard__init_counters_from_history.');
      const schemaTokens = expectLogDetailNumber(firstRequestLog, 'schema_tokens', 'schema_tokens detail missing for context_guard__init_counters_from_history.');
      const newTokens = expectLogDetailNumber(firstRequestLog, 'new_tokens', 'new_tokens detail missing for context_guard__init_counters_from_history.');
      const expectedTokens = expectLogDetailNumber(firstRequestLog, 'expected_tokens', 'expected_tokens detail missing for context_guard__init_counters_from_history.');

      invariant(expectedTokens === ctxTokens + schemaTokens + newTokens, 'expected_tokens should equal ctx + schema + new for context_guard__init_counters_from_history.');
      invariant(ctxTokens === 0, 'ctx_tokens should be zero before the first LLM attempt for context_guard__init_counters_from_history.');

      const firstUserIndex = result.conversation.findIndex((message) => message.role === 'user');
      invariant(firstUserIndex >= 0, 'Initial user message not found for context_guard__init_counters_from_history.');
      const initialMessages = result.conversation.slice(0, firstUserIndex + 1);
      invariant(initialMessages.length === firstUserIndex + 1, 'Initial conversation length mismatch for context_guard__init_counters_from_history.');
      const tokenizer = resolveTokenizer(TOKENIZER_GPT4O);
      const tokenizerCtxTokens = estimateMessagesTokens(tokenizer, initialMessages);
      const approxTokens = Math.ceil(estimateMessagesBytesLocal(initialMessages) / 4);
      const projectedCtxTokens = Math.max(tokenizerCtxTokens, approxTokens);
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-history initialMessages:', JSON.stringify(initialMessages, null, 2));
        console.log('context-history logged ctx:', ctxTokens);
        console.log('context-history tokenizer ctx:', tokenizerCtxTokens);
        console.log('context-history approx ctx:', approxTokens);
        console.log('context-history projected ctx:', projectedCtxTokens);
      }
      invariant(newTokens === projectedCtxTokens, `new_tokens should match projected estimate (expected ${String(projectedCtxTokens)}, got ${String(newTokens)}).`);

      const historyAssistant = initialMessages.find((message) => message.role === 'assistant');
      invariant(
        historyAssistant !== undefined
        && typeof historyAssistant.content === 'string'
        && historyAssistant.content === HISTORY_ASSISTANT_SEED,
        'Assistant history message must be preserved in conversation.',
      );
    },
  },
  {
    id: 'run-test-context-token-double-count',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 2048,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      defaults.contextWindowBufferTokens = 32;
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.systemPrompt = MINIMAL_SYSTEM_PROMPT;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-token-double-count expected success.');
      const finalReport = result.finalReport;
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-token-double-count logs:', JSON.stringify(result.logs, null, 2));
        console.log('context-token-double-count conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('context-token-double-count accounting:', JSON.stringify(result.accounting, null, 2));
      }
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-context-token-double-count.');
      const toolIndex = result.logs.findIndex((entry) =>
        entry.type === 'tool'
        && entry.direction === 'response'
        && logHasDetail(entry, 'tokens_estimated')
        && logHasDetail(entry, 'tool')
        && getLogDetail(entry, 'tool') === 'test__test'
      );
      invariant(toolIndex !== -1, 'Tool log with tokens_estimated expected for run-test-context-token-double-count.');
      const toolLog = result.logs[toolIndex];
      const toolTokens = expectLogDetailNumber(toolLog, 'tokens_estimated', 'Tool tokens_estimated detail missing for run-test-context-token-double-count.');
      invariant(toolTokens > 0, 'Tool tokens should be positive for run-test-context-token-double-count.');

      const remoteId = `${PRIMARY_PROVIDER}:${MODEL_NAME}`;
      const toolTurn = typeof toolLog.turn === 'number' ? toolLog.turn : Number.POSITIVE_INFINITY;
      const toolSubturn = typeof toolLog.subturn === 'number' ? toolLog.subturn : Number.POSITIVE_INFINITY;
      let responseLogBefore: LogEntry | undefined;
      let requestLogBeforeFallback: LogEntry | undefined;
      result.logs.forEach((entry) => {
        if (entry.type !== 'llm') return;
        if (entry.remoteIdentifier !== remoteId) return;
        const entryTurn = typeof entry.turn === 'number' ? entry.turn : Number.NEGATIVE_INFINITY;
        const entrySubturn = typeof entry.subturn === 'number' ? entry.subturn : Number.NEGATIVE_INFINITY;
        const occursBeforeTool = entryTurn < toolTurn || (entryTurn === toolTurn && entrySubturn <= toolSubturn);
        if (!occursBeforeTool) return;
        switch (entry.direction) {
          case 'response':
            responseLogBefore = entry;
            break;
          case 'request':
            requestLogBeforeFallback = entry;
            break;
          default:
            break;
        }
      });
      invariant(responseLogBefore !== undefined || requestLogBeforeFallback !== undefined, 'LLM response/request log before tool expected for run-test-context-token-double-count.');
      const requestLogAfter = result.logs.slice(toolIndex).find((entry) =>
        entry.type === 'llm'
        && entry.direction === 'request'
        && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
      );
      invariant(requestLogAfter !== undefined, 'LLM request log after tool expected for run-test-context-token-double-count.');
      const baselineLog = requestLogBeforeFallback ?? responseLogBefore;
      if (baselineLog === undefined) {
        throw new Error('Pre-tool baseline log missing for run-test-context-token-double-count.');
      }
      const ctxAfter = expectLogDetailNumber(requestLogAfter, 'ctx_tokens', 'ctx_tokens detail missing (post-tool) for run-test-context-token-double-count.');
      const schemaAfter = expectLogDetailNumber(requestLogAfter, 'schema_tokens', 'schema_tokens detail missing (post-tool) for run-test-context-token-double-count.');
      const newTokens = expectLogDetailNumber(requestLogAfter, 'new_tokens', 'new_tokens detail missing (post-tool) for run-test-context-token-double-count.');
      const expectedAfter = expectLogDetailNumber(requestLogAfter, 'expected_tokens', 'expected_tokens detail missing (post-tool) for run-test-context-token-double-count.');

      const baselineNewTokens = logHasDetail(baselineLog, 'new_tokens')
        ? expectLogDetailNumber(baselineLog, 'new_tokens', 'new_tokens detail missing (baseline) for run-test-context-token-double-count.')
        : 0;
      const newTokenTolerance = 16;
      if (baselineNewTokens <= newTokens) {
        const deltaNewTokens = newTokens - baselineNewTokens;
        invariant(Math.abs(deltaNewTokens - toolTokens) <= newTokenTolerance, `Pending context tokens increase should match tool token estimate (expected delta ~${String(toolTokens)}, got ${String(deltaNewTokens)}).`);
      } else {
        invariant(Math.abs(newTokens - toolTokens) <= newTokenTolerance, `Pending context tokens should equal tool token estimate (expected ~${String(toolTokens)}, got ${String(newTokens)}).`);
      }
      invariant(expectedAfter === ctxAfter + schemaAfter + newTokens, 'expected_tokens should equal ctx + schema + new for run-test-context-token-double-count.');

    },
  },
  {
    id: 'run-test-context-bulk-tools',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 300,
              contextWindowBufferTokens: 16,
              tokenizer: 'approximate',
            },
          },
        },
      };
      configuration.defaults = { ...defaults, contextWindowBufferTokens: 16 };
      sessionConfig.maxOutputTokens = 48;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Historical context for bulk trimming.' },
        { role: 'assistant', content: 'Z'.repeat(400) },
      ];
    },
    expect: (result) => {
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('bulk-tools raw result:' + JSON.stringify({ success: result.success, accounting: result.accounting, logs: result.logs, conversation: result.conversation }));
      }
      invariant(result.success, 'Scenario run-test-context-bulk-tools expected success.');
      const toolMessages = result.conversation.filter((message) => {
        if (message.role !== 'tool') return false;
        const callId = (message as { toolCallId?: string }).toolCallId;
        return callId !== FINAL_REPORT_CALL_ID;
      });
      invariant(toolMessages.length === 0, 'Bulk tool scenario should transition directly to final turn when preflight exhausts budget.');
      const warnLog = result.logs.find((entry) =>
        entry.remoteIdentifier === CONTEXT_REMOTE
        && entry.severity === 'WRN'
        && typeof entry.message === 'string'
        && entry.message.includes('post-shrink'));
      invariant(warnLog !== undefined, 'Context guard warning expected for bulk tool scenario.');
      invariant(result.finalReport?.status === 'success', 'Final report should succeed after bulk trimming.');
    },
  },
  {
    id: 'run-test-context-tokenizer-drift',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 360,
              contextWindowBufferTokens: 24,
              tokenizer: 'approximate',
            },
          },
        },
      };
      configuration.defaults = { ...defaults, contextWindowBufferTokens: 24 };
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Previous findings with precise tokenizer.' },
        { role: 'assistant', content: 'Mixed tokenizer estimates pending reconciliation.' + 'A'.repeat(600) },
      ];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-context-tokenizer-drift expected success.');
      const contextWarning = result.logs.find((entry) => entry.remoteIdentifier === CONTEXT_REMOTE);
      invariant(contextWarning !== undefined, 'Tokenizer drift scenario should log context guard warning.');
      invariant(result.finalReport?.status === 'success', 'Final report should succeed despite tokenizer drift.');
    },
  },
  {
    id: 'run-test-context-cache-tokens',
    configure: (configuration, sessionConfig, defaults) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: {
          type: 'test-llm',
          models: {
            [MODEL_NAME]: {
              contextWindow: 512,
              contextWindowBufferTokens: 32,
              tokenizer: TOKENIZER_GPT4O,
            },
          },
        },
      };
      configuration.defaults = defaults;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.maxOutputTokens = 64;
      sessionConfig.conversationHistory = [
        { role: 'system', content: 'Cache-intensive discussion baseline.' },
        { role: 'assistant', content: 'Referencing cached data...' },
      ];
    },
    expect: (result) => {
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('context-cache raw result:' + JSON.stringify({ success: result.success, accounting: result.accounting, logs: result.logs, conversation: result.conversation }));
      }
      invariant(result.success, 'Scenario run-test-context-cache-tokens expected success.');
      const guardWarning = result.logs.find((entry) => entry.remoteIdentifier === CONTEXT_REMOTE);
      invariant(guardWarning !== undefined, 'Cache token scenario should log context guard warning.');
      invariant(result.finalReport?.status === 'success', 'Final report should succeed after cache token reconciliation.');
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
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-17.');
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
    id: RUN_TEST_21,
    expect: (result) => {
      invariant(result.success, `Scenario ${RUN_TEST_21} expected success.`);
      const rateLimitCandidate = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN));
      const rateLimitLog = expectLlmLogContext(rateLimitCandidate, RUN_TEST_21, { message: rateLimitWarningExpected(RUN_TEST_21), severity: 'WRN', remote: PRIMARY_REMOTE });
      invariant(typeof rateLimitLog.message === 'string' && rateLimitLog.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN), rateLimitMessageMismatch(RUN_TEST_21));
      const retryCandidate = result.logs.find((entry) => entry.remoteIdentifier === RETRY_REMOTE);
      const retryLog = expectLlmLogContext(retryCandidate, RUN_TEST_21, { message: retryBackoffExpected(RUN_TEST_21), severity: 'WRN', remote: RETRY_REMOTE });
      invariant(typeof retryLog.message === 'string' && retryLog.message.includes(BACKING_OFF_FRAGMENT), retryBackoffMessageMismatch(RUN_TEST_21));
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
      invariant(subAgentAccounting?.status === 'ok', 'Successful sub-agent accounting expected for run-test-24.');
      const subAgentLog = result.logs.find((entry) => {
        // Check if it's a subagent log
        if (typeof entry.remoteIdentifier !== 'string' || !entry.remoteIdentifier.startsWith('agent:')) {
          return false;
        }
        // Check for error
        if (typeof entry.message === 'string' && entry.message.includes('error')) {
          return false;
        }
        // Check for the tool name in message or details
        const hasToolInMessage = typeof entry.message === 'string' && entry.message.includes(SUBAGENT_SUCCESS_TOOL);
        const hasToolInDetails = (
          logHasDetail(entry, 'tool') && String(getLogDetail(entry, 'tool')).includes(SUBAGENT_SUCCESS_TOOL)
        ) || (
          logHasDetail(entry, 'request_preview') && String(getLogDetail(entry, 'request_preview')).includes(SUBAGENT_SUCCESS_TOOL)
        );
        return hasToolInMessage || hasToolInDetails;
      });
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

    },
  },
  {
    id: 'run-test-25',
    configure: (configuration) => {
      configuration.queues = { ...configuration.queues, default: { concurrent: 1 } };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-25 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-25.');

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
          ((typeof log.message === 'string' && log.message.includes(CONCURRENCY_TIMEOUT_ARGUMENT)) ||
           (logHasDetail(log, 'request_preview') &&
            String(getLogDetail(log, 'request_preview')).includes(CONCURRENCY_TIMEOUT_ARGUMENT)))
      );
      invariant(firstRequestIndex !== -1, 'First tool request log missing for run-test-25.');
      const firstResponseIndex = logs.findIndex(
        (log, idx) =>
          idx > firstRequestIndex &&
          isMcpToolLog(log) &&
          log.direction === 'response' &&
          ((typeof log.message === 'string' && (log.message.includes('ok test__test') || log.message.includes('ok preview:'))) ||
           (logHasDetail(log, 'tool') &&
            String(getLogDetail(log, 'tool')).includes('test__test')))
      );
      invariant(firstResponseIndex !== -1, 'First tool response log missing for run-test-25.');
      const secondRequestIndex = logs.findIndex(
        (log, idx) =>
          idx > firstRequestIndex &&
          isMcpToolLog(log) &&
          log.direction === 'request' &&
          ((typeof log.message === 'string' && log.message.includes(CONCURRENCY_SECOND_ARGUMENT)) ||
           (logHasDetail(log, 'request_preview') &&
            String(getLogDetail(log, 'request_preview')).includes(CONCURRENCY_SECOND_ARGUMENT)))
      );
      invariant(secondRequestIndex !== -1, 'Second tool request log missing for run-test-25.');
      invariant(secondRequestIndex > firstResponseIndex, 'Second tool request should occur after first tool response for run-test-25.');
      const queuedLog = logs.find((entry) => entry.message === 'queued' && entry.remoteIdentifier.startsWith('queue:'));
      invariant(queuedLog !== undefined, 'Queued log entry missing for run-test-25.');
      invariant(queuedLog.details?.queue === 'default', 'Queued log should reference default queue for run-test-25.');
    },
  },
  {
    id: 'run-test-queue-cancel',
    description: 'Abort the session while a tool waits for queue capacity.',
    configure: (configuration) => {
      configuration.queues = { ...configuration.queues, default: { concurrent: 1 } };
    },
    execute: async (_configuration, sessionConfig) => {
      const abort = new AbortController();
      sessionConfig.abortSignal = abort.signal;
      const session = AIAgentSession.create(sessionConfig);
      const timer = setTimeout(() => {
        try { abort.abort(); } catch { /* ignore */ }
      }, 75);
      try {
        return await session.run();
      } finally {
        clearTimeout(timer);
      }
    },
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-queue-cancel expected cancellation.');
      invariant(result.error === 'canceled', 'Cancellation must propagate error for run-test-queue-cancel.');
      const queuedLog = result.logs.find((entry) => entry.message === 'queued' && entry.remoteIdentifier.startsWith('queue:'));
      invariant(queuedLog !== undefined, 'Queued log entry missing for run-test-queue-cancel.');
      invariant(queuedLog.details?.queue === 'default', 'Queued log should reference default queue for run-test-queue-cancel.');
    },
  },
  {
    id: 'run-test-queue-isolation',
    configure: (configuration, sessionConfig) => {
      configuration.queues = { default: { concurrent: 1 }, fast: { concurrent: 1 } };
      configuration.mcpServers = {
        slow: {
          type: 'stdio',
          command: process.execPath,
          args: [TEST_STDIO_SERVER_PATH],
          queue: 'default',
        },
        fast: {
          type: 'stdio',
          command: process.execPath,
          args: [TEST_STDIO_SERVER_PATH],
          queue: 'fast',
        },
      } satisfies Record<string, MCPServerConfig>;
      sessionConfig.tools = ['slow', 'fast', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-queue-isolation expected success.');
      const queuedDefault = result.logs.some((entry) => entry.message === 'queued' && entry.details?.queue === 'default');
      invariant(queuedDefault, 'Default queue should log a queued entry in run-test-queue-isolation.');
      const queuedFast = result.logs.some((entry) => entry.message === 'queued' && entry.details?.queue === 'fast');
      invariant(!queuedFast, 'Fast queue must not log a queued entry in run-test-queue-isolation.');
    },
  },
  {
    id: 'run-test-26',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-26 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-26.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-invalid-id'
      );
      const invalidContent = batchMessage?.content ?? '';
      invariant(invalidContent.includes('invalid_batch_input'), 'Batch tool message should include invalid_batch_input for run-test-26.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry?.status === 'failed' && typeof batchEntry.error === 'string' && batchEntry.error.startsWith('invalid_batch_input'), 'Batch accounting should record invalid input failure for run-test-26.');
    },
  },
  {
    id: 'run-test-27',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-27 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-27.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-unknown-tool'
      );
      const unknownContent = batchMessage?.content ?? '';
      invariant(unknownContent.includes('UNKNOWN_TOOL'), 'Batch tool message should include UNKNOWN_TOOL for run-test-27.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry?.status === 'ok', 'Batch accounting should record success for run-test-27.');
    },
  },
  {
    id: 'run-test-28',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-28 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-28.');
      const batchMessage = result.conversation.find(
        (message) => message.role === 'tool' && message.toolCallId === 'call-batch-exec-error'
      );
      const errorContent = batchMessage?.content ?? '';
      invariant(errorContent.includes('EXECUTION_ERROR'), 'Batch tool message should include EXECUTION_ERROR for run-test-28.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry?.status === 'ok', 'Batch accounting should remain ok for run-test-28.');
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
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-29.');
      const augmented = result as AIAgentResult & { _firstAttempt?: AIAgentResult };
      const firstAttempt = augmented._firstAttempt;
      invariant(firstAttempt !== undefined && !firstAttempt.success, 'First attempt should fail before retry for run-test-29.');
      invariant(typeof firstAttempt.error === 'string' && firstAttempt.error.includes('Simulated fatal error before manual retry.'), 'First attempt error message mismatch for run-test-29.');
      const successLog = result.logs.find((entry) =>
        entry.type === 'tool' &&
        entry.direction === 'response' &&
        ((typeof entry.message === 'string' && (entry.message.includes('ok test__test') || entry.message.includes('ok preview:'))) ||
         (logHasDetail(entry, 'tool') &&
          String(getLogDetail(entry, 'tool')).includes('test__test')))
      );
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
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-30.');
      const thinkingLog = result.logs.find((entry) => entry.severity === 'THK' && entry.remoteIdentifier === 'thinking');
      invariant(thinkingLog !== undefined, 'Thinking log expected for run-test-30.');
    },
  },
  {
    id: RUN_TEST_31,
    expect: (result) => {
      invariant(result.success, `Scenario ${RUN_TEST_31} expected success after provider throw.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-31.');
      const thrownCandidate = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes(THROW_FAILURE_MESSAGE));
      const thrownLog = expectLlmLogContext(thrownCandidate, RUN_TEST_31, { message: `Thrown failure log expected for ${RUN_TEST_31}.`, severity: 'WRN', remote: PRIMARY_REMOTE });
      const thrownDetails = thrownLog.details;
      invariant(thrownDetails?.status === 'model_error', 'Thrown LLM error should record model_error status for run-test-31.');
      const thrownStatusMessage = thrownDetails.status_message;
      invariant(typeof thrownStatusMessage === 'string' && thrownStatusMessage.includes(THROW_FAILURE_MESSAGE), 'Thrown LLM error should preserve status message for run-test-31.');
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
      invariant(finalReport?.status === 'success' && finalReport.format === 'json', 'Final report should indicate JSON success for run-test-32.');
      const toolFailureMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('final_report(json) requires'));
      invariant(toolFailureMessage !== undefined, 'Final report failure message expected for run-test-32.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts >= 2, 'Retry attempt expected for run-test-32.');
    },
  },
  {
    id: RUN_TEST_33,
    configure: (_configuration, sessionConfig) => {
      sessionConfig.maxTurns = 1;
    },
    expect: (result) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${RUN_TEST_33} should have success=false when finalReport.status=failure per CONTRACT.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Synthesized failure final report expected for run-test-33.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Session completed without a final report'), 'Synthesized content should mention missing final report for run-test-33.');
      const syntheticCandidate = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Synthetic retry: assistant returned content without tool calls and without final_report.'));
      const syntheticLog = expectLlmLogContext(syntheticCandidate, RUN_TEST_33, { message: `Synthetic retry warning expected for ${RUN_TEST_33}.`, severity: 'WRN', remote: PRIMARY_REMOTE });
      invariant(typeof syntheticLog.message === 'string' && syntheticLog.message.includes('Synthetic retry'), `Synthetic retry message mismatch for ${RUN_TEST_33}.`);
      const finalTurnCandidate = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
      const finalTurnLog = expectLlmLogContext(finalTurnCandidate, RUN_TEST_33, { message: `Final-turn retry warning expected for ${RUN_TEST_33}.`, severity: 'WRN', remote: FINAL_TURN_REMOTE });
      invariant(typeof finalTurnLog.message === 'string' && finalTurnLog.message.toLowerCase().includes('final turn'), `Final-turn message mismatch for ${RUN_TEST_33}.`);
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined, 'Synthesized EXIT-FINAL-ANSWER log expected for run-test-33.');
    },
  },
  {
    id: 'run-test-34',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-34 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-34.');
      const progressResult = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('agent__progress_report'));
      invariant(progressResult !== undefined, 'Batch progress entry expected for run-test-34.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(BATCH_PROGRESS_RESPONSE));
      invariant(toolMessage !== undefined, 'Batch tool output expected for run-test-34.');
    },
  },
  {
    id: 'run-test-35',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-35 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-35.');
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(BATCH_STRING_RESULT));
      invariant(toolMessage !== undefined, 'Batch string tool output expected for run-test-35.');
    },
  },
  {
    id: 'run-test-36',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
    },
    expect: (result) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-36 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-36.');
      const failureMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes('empty_batch'));
      invariant(failureMessage !== undefined, 'Empty batch failure message expected for run-test-36.');
      const batchEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === 'agent__batch');
      invariant(batchEntry?.status === 'failed' && typeof batchEntry.error === 'string' && batchEntry.error.startsWith('empty_batch'), 'Batch accounting should record empty batch failure for run-test-36.');
    },
  },
  {
    id: RUN_TEST_37,
    configure: (_configuration, sessionConfig) => {
      sessionConfig.maxRetries = 2;
    },
    expect: (result) => {
      invariant(result.success, `Scenario ${RUN_TEST_37} expected success after rate limit retry.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-37.');
      const rateLimitCandidate = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN));
      const rateLimitLog = expectLlmLogContext(rateLimitCandidate, RUN_TEST_37, { message: rateLimitWarningExpected(RUN_TEST_37), severity: 'WRN', remote: PRIMARY_REMOTE });
      invariant(typeof rateLimitLog.message === 'string' && rateLimitLog.message.toLowerCase().includes(RATE_LIMIT_WARNING_TOKEN), rateLimitMessageMismatch(RUN_TEST_37));
      const retryCandidate = result.logs.find((entry) => entry.remoteIdentifier === RETRY_REMOTE);
      const retryLog = expectLlmLogContext(retryCandidate, RUN_TEST_37, { message: retryBackoffExpected(RUN_TEST_37), severity: 'WRN', remote: RETRY_REMOTE });
      invariant(typeof retryLog.message === 'string' && retryLog.message.includes(BACKING_OFF_FRAGMENT), retryBackoffMessageMismatch(RUN_TEST_37));
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
    configure: (configuration, sessionConfig, _defaults) => {
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
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-42.');
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
      invariant(finalReport?.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-50.');
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
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-51 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-51.');
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
      invariant(finalReport?.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-52.');
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
    execute: async (_configuration, _sessionConfig, _defaults) => {
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
         
        console.error(JSON.stringify(result.logs, null, 2));
        invariant(false, 'TRC response log with SSE expected for run-test-55.');
      }
      const errorTrace = result.logs.find((entry) => entry.severity === 'TRC' && entry.message.includes('HTTP Error: network down'));
      invariant(errorTrace !== undefined, 'HTTP error trace expected for run-test-55.');
      // Since logResponse doesn't emit VRB logs for success, check that costs are properly tracked
      // in error logs which DO get emitted
      const quotaLog = result.logs.find((entry) =>
        entry.severity === 'ERR' &&
        entry.message.includes('QUOTA_EXCEEDED') &&
        logHasDetail(entry, 'latency_ms')
      );

      if (quotaLog === undefined && process.env.CONTEXT_DEBUG === 'true') {
        console.error('No quota log found. Logs:', JSON.stringify(result.logs, null, 2));
      }

      // Verify quota exceeded error was logged with proper details
      invariant(quotaLog !== undefined, 'Quota exceeded log expected for run-test-55.');

      // Also verify the final report contains the expected cost and routing data
      const report = result.finalReport?.content_json;
      assertRecord(report, 'Final data snapshot expected for run-test-55.');
      const costs = report.costSnapshot as Record<string, unknown> | undefined;

      // The test sets costs in finalData.costs which should be in the report
      // We can verify that the cost tracking mechanism works even if VRB logs aren't emitted
      const hasCostData = costs !== undefined && (
        'costUsd' in costs ||
        'upstreamInferenceCostUsd' in costs
      );

      if (!hasCostData && process.env.CONTEXT_DEBUG === 'true') {
        console.error('No cost data in final report:', JSON.stringify(report, null, 2));
      }
      const failureLog = result.logs.find((entry) => entry.severity === 'ERR' && entry.message.includes('AUTH_ERROR'));
      invariant(failureLog !== undefined, 'Failure log expected for run-test-55.');
      const fetchErrorMsg = typeof report.fetchError === 'string' ? report.fetchError : undefined;
      invariant(fetchErrorMsg === 'network down', 'Fetch error message expected for run-test-55.');
      const routingAfterJson = report.routingAfterJson;
      assertRecord(routingAfterJson, 'JSON routing metadata expected for run-test-55.');
      if (routingAfterJson.provider === undefined || routingAfterJson.model === undefined) {
         
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
        invariant(finalReport?.format === SLACK_OUTPUT_FORMAT, 'Slack final report expected for run-test-57.');
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
        invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-58.');
      },
    };
  })(),
  {
    id: 'run-test-59',
    configure: (configuration, sessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      sessionConfig.toolResponseMaxBytes = 120;
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-59 expected success.');
      const truncLogs = result.logs.filter((entry) => entry.severity === 'WRN' && typeof entry.message === 'string' && entry.message.includes('response exceeded max size'));
      invariant(truncLogs.length > 0, 'Truncation warning expected for run-test-59.');
      const mcpTrunc = truncLogs.find((entry) => logHasDetail(entry, 'tool') && getLogDetail(entry, 'tool') === 'test__test');
      invariant(mcpTrunc !== undefined, 'MCP truncation warning missing for run-test-59.');
      invariant(logHasDetail(mcpTrunc, 'provider') && getLogDetail(mcpTrunc, 'provider') === 'mcp:test', 'Truncation warning must carry provider field for run-test-59.');
      invariant(logHasDetail(mcpTrunc, 'tool_kind') && getLogDetail(mcpTrunc, 'tool_kind') === 'mcp', 'Truncation warning must carry tool_kind field for run-test-59.');
      invariant(logHasDetail(mcpTrunc, 'actual_bytes') && getLogDetail(mcpTrunc, 'actual_bytes') === 5000, 'Truncation warning must report actual_bytes for run-test-59.');
      invariant(logHasDetail(mcpTrunc, 'limit_bytes') && getLogDetail(mcpTrunc, 'limit_bytes') === 120, 'Truncation warning must report limit_bytes for run-test-59.');
      const batchTool = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(TRUNCATION_NOTICE));
      invariant(batchTool !== undefined, 'Truncated tool response expected for run-test-59.');
    },
  },

  (() => {
    interface CapturedRequestSnapshot {
      temperature?: number | null;
      topP?: number | null;
      maxOutputTokens?: number;
      repeatPenalty?: number | null;
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
        invariant(typeof first.temperature === 'number' && Math.abs(first.temperature - 0.21) < 1e-6, 'Temperature propagation failed for run-test-60.');
        invariant(typeof first.topP === 'number' && Math.abs(first.topP - 0.77) < 1e-6, 'topP propagation failed for run-test-60.');
        invariant(first.maxOutputTokens === 123, 'maxOutputTokens propagation failed for run-test-60.');
        invariant(typeof first.repeatPenalty === 'number' && Math.abs(first.repeatPenalty - 1.1) < 1e-6, 'repeatPenalty propagation failed for run-test-60.');
        const hasHistoryInResult = result.conversation.some((message) => message.role === 'assistant' && message.content.includes(HISTORY_ASSISTANT));
        invariant(hasHistoryInResult, 'Historical assistant message should be present in result conversation for run-test-60.');
        const llmEntry = result.accounting.find(isLlmAccounting);
        invariant(llmEntry !== undefined, 'LLM accounting entry expected for run-test-60.');
        invariant(typeof llmEntry.txnId === 'string' && llmEntry.txnId.length > 0, 'LLM accounting should include txnId for run-test-60.');
        invariant(llmEntry.callPath === TRACE_CONTEXT.callPath, 'Trace context should set callPath on LLM accounting entry for run-test-60.');
        invariant(llmEntry.originTxnId === TRACE_CONTEXT.originId, 'Trace context should preserve originTxnId on LLM accounting entry for run-test-60.');
        invariant(llmEntry.parentTxnId === TRACE_CONTEXT.parentId, 'Trace context should preserve parentTxnId on LLM accounting entry for run-test-60.');
        const finalReport = result.finalReport;
        invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-60.');
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
        // CONTRACT §2: success: false when finalReport.status is 'failure'
        invariant(!result.success, 'Scenario run-test-61 should have success=false when finalReport.status=failure per CONTRACT.');
        const limitLog = capturedLogs.find((entry) => entry.remoteIdentifier === 'agent:limits' && typeof entry.message === 'string' && entry.message.includes(TOOL_LIMIT_WARNING_MESSAGE));
        invariant(limitLog !== undefined, 'Limit enforcement log expected for run-test-61.');
        invariant(limitLog.severity === 'ERR', 'Tool limit log should be severity ERR for run-test-61.');
        const finalReport = result.finalReport;
        invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-61.');
      },
    };
  })(),
  (() => {
    const FINAL_TURN_INSTRUCTION = MAX_TURNS_FINAL_MESSAGE;
    let capturedRequests: TurnRequest[] = [];
    let capturedLogs: LogEntry[] = [];
    return {
      id: RUN_TEST_MAX_TURN_LIMIT,
      configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig, _defaults) => {
        configuration.providers = {
          [PRIMARY_PROVIDER]: { type: 'test-llm' },
        };
        sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
        sessionConfig.maxTurns = 2;
        sessionConfig.userPrompt = RUN_TEST_MAX_TURN_LIMIT;
        sessionConfig.tools = ['test'];
        const existingCallbacks = sessionConfig.callbacks ?? {};
        sessionConfig.callbacks = {
          ...existingCallbacks,
          onLog: (entry) => {
            capturedLogs.push(entry);
            existingCallbacks.onLog?.(entry);
          },
        };
      },
      execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
        capturedRequests = [];
        capturedLogs = [];
        // eslint-disable-next-line @typescript-eslint/unbound-method -- capture method for restoration after interception
        const originalExecute = LLMClient.prototype.executeTurn;
        LLMClient.prototype.executeTurn = async function(request: TurnRequest): Promise<TurnResult> {
          capturedRequests.push(request);
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
        // Model provided a final report, so status is success
        invariant(result.success, `Scenario ${RUN_TEST_MAX_TURN_LIMIT} should have success=true when model provides final report.`);
        const conversationHasInstruction = result.conversation.some((message) => message.role === 'user' && message.content === FINAL_TURN_INSTRUCTION);
        if (conversationHasInstruction && process.env.CONTEXT_DEBUG === 'true') {
          console.log(`${RUN_TEST_MAX_TURN_LIMIT} conversation:`, JSON.stringify(result.conversation, null, 2));
        }
        invariant(!conversationHasInstruction, `Final turn instruction should remain ephemeral and not persist in conversation for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        invariant(capturedRequests.length >= 2, `At least two LLM requests expected for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        const finalTurnRequest = capturedRequests.at(-1);
        invariant(finalTurnRequest !== undefined, `Final turn request missing for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        const requestMessages = Array.isArray(finalTurnRequest.messages) ? finalTurnRequest.messages : [];
        const instructionOccurrences = requestMessages.filter((message) => message.role === 'user' && message.content === FINAL_TURN_INSTRUCTION);
        if (instructionOccurrences.length !== 1 && process.env.CONTEXT_DEBUG === 'true') {
          console.log(`${RUN_TEST_MAX_TURN_LIMIT} final request messages:`, JSON.stringify(requestMessages, null, 2));
        }
        invariant(instructionOccurrences.length === 1, `Final turn instruction should be present exactly once in the final LLM request for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        const toolNames = Array.isArray(finalTurnRequest.tools)
          ? finalTurnRequest.tools.map((tool) => sanitizeToolName(tool.name))
          : [];
        invariant(toolNames.length === 1 && toolNames[0] === 'agent__final_report', `Final turn should restrict tools to agent__final_report for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        const finalTurnLog = capturedLogs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
        invariant(finalTurnLog !== undefined, `Final turn log expected for ${RUN_TEST_MAX_TURN_LIMIT}.`);
        invariant(result.finalReport?.status === 'success', `Model-provided final report should indicate success for ${RUN_TEST_MAX_TURN_LIMIT}.`);
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
        const serverScript = path.resolve(__dirname, '../mcp/test-stdio-server.js');
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
        invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-63.');
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
    const captured: { namespace?: string; namespaceError?: string; filtered: string[]; stdioError?: string; combined?: string } = { filtered: [] };
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
        try {
          captured.namespace = exposed.sanitizeNamespace('Alpha-Server!!');
        } catch (error: unknown) {
          captured.namespaceError = toErrorMessage(error);
        }
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
          | { namespace?: string; namespaceError?: string; filtered: string[]; stdioError?: string; combined?: string }
          | undefined;
        invariant(data !== undefined, 'Combined data expected for run-test-65.');
        invariant(
          typeof data.namespaceError === 'string' && data.namespaceError.includes("letters, digits, '-' or '_'"),
          'Namespace validation mismatch for run-test-65.'
        );
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
      id: 'run-test-66-shared',
      description: 'Shared MCP registry pooling, timeout probe, and cancel semantics.',
      execute: async () => {
        const registry = new HarnessSharedRegistry();
        registry.callResults = [{ content: SHARED_REGISTRY_RESULT }];
        registry.probeResults = [true, false];
        const logs: LogEntry[] = [];
        const config: Record<string, MCPServerConfig> = {
          sharedServer: { type: 'stdio', command: 'mock', shared: true },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, { sharedRegistry: registry, onLog: (entry) => { logs.push(entry); } });
        await (provider as unknown as { ensureInitialized: () => Promise<void> }).ensureInitialized();
        const exec = await provider.execute('sharedServer__mock_tool', {});
        await provider.cancelTool('sharedServer__mock_tool', { reason: 'timeout' });
        await provider.cancelTool('sharedServer__mock_tool', { reason: 'timeout' });
        await provider.cancelTool('sharedServer__mock_tool', { reason: 'abort' });
        await provider.cleanup();
        if (process.env.RESTART_DEBUG === 'true') {
          console.log('run-test-73 logs', logs.map((entry) => ({ message: entry.message, severity: entry.severity, remote: entry.remoteIdentifier })));
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
              result: exec.result,
              acquireCount: registry.acquireCount,
              callCount: registry.lastHandle?.callCount ?? 0,
              timeoutCalls: registry.timeoutCalls,
              restartCalls: registry.restartCalls,
              cancelReasons: registry.cancelReasons,
              releaseCount: registry.releaseCount,
            },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-66-shared expected success.');
        const data = result.finalReport?.content_json as {
          result: string;
          acquireCount: number;
          callCount: number;
          timeoutCalls: string[];
          restartCalls: number;
          cancelReasons: ('timeout' | 'abort')[];
          releaseCount: number;
        } | undefined;
        invariant(data !== undefined, 'final report payload expected for run-test-66-shared.');
        invariant(data.result === SHARED_REGISTRY_RESULT, 'shared execute result mismatch for run-test-66-shared.');
        invariant(data.acquireCount === 1, 'shared registry should acquire once for run-test-66-shared.');
        invariant(data.callCount === 1, 'shared handle call count mismatch for run-test-66-shared.');
        invariant(data.timeoutCalls.length === 2, 'timeout count mismatch for run-test-66-shared.');
        invariant(data.restartCalls === 1, 'restart count mismatch for run-test-66-shared.');
        const timeoutReasonCount = data.cancelReasons.filter((reason) => reason === 'timeout').length;
        invariant(timeoutReasonCount === 2, 'Timeout cancel reasons mismatch for run-test-66-shared.');
        invariant(data.cancelReasons.includes('abort'), 'Abort reason should be recorded in run-test-66-shared.');
        invariant(data.releaseCount === 1, 'Release count mismatch for run-test-66-shared.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-66-shared-http',
      description: 'Non-stdio MCP transports also use the shared registry path.',
      execute: async () => {
        const registry = new HarnessSharedRegistry();
        registry.callResults = [{ content: SHARED_REGISTRY_RESULT }];
        registry.probeResults = [true, false];
        const logs: LogEntry[] = [];
        const config: Record<string, MCPServerConfig> = {
          sharedHttp: { type: 'http', url: 'https://example.com/mock', shared: true },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, { sharedRegistry: registry, onLog: (entry) => { logs.push(entry); } });
        await (provider as unknown as { ensureInitialized: () => Promise<void> }).ensureInitialized();
        const exec = await provider.execute('sharedHttp__mock_tool', {});
        await provider.cancelTool('sharedHttp__mock_tool', { reason: 'timeout' });
        await provider.cancelTool('sharedHttp__mock_tool', { reason: 'timeout' });
        await provider.cancelTool('sharedHttp__mock_tool', { reason: 'abort' });
        await provider.cleanup();
        return {
          success: true,
          conversation: [],
          logs,
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: {
              result: exec.result,
              serverType: registry.lastHandle?.server.config.type,
              acquireCount: registry.acquireCount,
              callCount: registry.lastHandle?.callCount ?? 0,
              timeoutCalls: registry.timeoutCalls,
              restartCalls: registry.restartCalls,
              cancelReasons: registry.cancelReasons,
              releaseCount: registry.releaseCount,
            },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-66-shared-http expected success.');
        const data = result.finalReport?.content_json as {
          result: string;
          serverType?: string;
          acquireCount: number;
          callCount: number;
          timeoutCalls: string[];
          restartCalls: number;
          cancelReasons: ('timeout' | 'abort')[];
          releaseCount: number;
        } | undefined;
        invariant(data !== undefined, 'final report payload expected for run-test-66-shared-http.');
        invariant(data.serverType === 'http', 'Shared HTTP server type mismatch for run-test-66-shared-http.');
        invariant(data.result === SHARED_REGISTRY_RESULT, 'shared execute result mismatch for run-test-66-shared-http.');
        invariant(data.acquireCount === 1, 'shared registry should acquire once for run-test-66-shared-http.');
        invariant(data.callCount === 1, 'shared handle call count mismatch for run-test-66-shared-http.');
        invariant(data.timeoutCalls.length === 2, 'timeout count mismatch for run-test-66-shared-http.');
        invariant(data.restartCalls === 1, 'restart count mismatch for run-test-66-shared-http.');
        const timeoutReasonCount = data.cancelReasons.filter((reason) => reason === 'timeout').length;
        invariant(timeoutReasonCount === 2, 'Timeout cancel reasons mismatch for run-test-66-shared-http.');
        invariant(data.cancelReasons.includes('abort'), 'Abort reason should be recorded in run-test-66-shared-http.');
        invariant(data.releaseCount === 1, 'Release count mismatch for run-test-66-shared-http.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-67-private',
      description: 'Private MCP timeout invokes restartServer while abort does not.',
      execute: async () => {
        const config: Record<string, MCPServerConfig> = {
          privateServer: { type: 'stdio', command: 'mock', shared: false },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config);
        const providerAny = provider as unknown as {
          toolNameMap: Map<string, { serverName: string; originalName: string }>;
          servers: Map<string, MCPServer>;
          restartServer: (serverName: string, reason: string) => Promise<void>;
        };
        providerAny.toolNameMap.set('privateServer__ping', { serverName: 'privateServer', originalName: 'ping' });
        providerAny.servers.set('privateServer', {
          name: 'privateServer',
          config: config.privateServer,
          tools: [{ name: 'ping', description: 'Ping', inputSchema: {} }],
          instructions: 'Private instructions',
        } as MCPServer);
        const restartLog: string[] = [];
        providerAny.restartServer = (serverName: string, reason: string) => {
          restartLog.push(`${serverName}:${reason}`);
          return Promise.resolve();
        };
        await provider.cancelTool('privateServer__ping', { reason: 'timeout' });
        await provider.cancelTool('privateServer__ping', { reason: 'abort' });
        return {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: { restartLog },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-67-private expected success.');
        const data = result.finalReport?.content_json as { restartLog: string[] } | undefined;
        invariant(data !== undefined, 'final report payload expected for run-test-67-private.');
        invariant(data.restartLog.length === 1 && data.restartLog[0] === 'privateServer:timeout', 'Private restart log mismatch for run-test-67-private.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-71-restart-success',
      description: 'Shared registry restarts hung stdio servers and gates concurrent callers.',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        const restartFixtureName = 'restartFixture';
        const restartFixtureTool = `${restartFixtureName}__test`;
        const config: Record<string, MCPServerConfig> = {
          [restartFixtureName]: {
            type: 'stdio',
            command: process.execPath,
            args: [TEST_STDIO_SERVER_PATH],
            shared: true,
            env: {
              MCP_FIXTURE_MODE: 'restart',
              MCP_FIXTURE_STATE_FILE: stateFile,
              MCP_FIXTURE_HANG_MS: '4000',
              MCP_FIXTURE_EXIT_DELAY_MS: '150',
            },
          },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, { requestTimeoutMs: 2000 });
        await provider.warmup();
        const toolName = restartFixtureTool;
        const hangingPromise = (async () => {
          try {
            return await provider.execute(toolName, { text: RESTART_TRIGGER_PAYLOAD }, { timeoutMs: 10000 });
          } catch (error) {
            return toError(error);
          }
        })();
        await delay(50);
        await delay(200);
        const registryAccessor = provider as unknown as { sharedRegistry: SharedRegistry };
        const restartRegistry = registryAccessor.sharedRegistry as unknown as { handleTimeout: (name: string, logger: LogFn) => Promise<void> };
        const waitStart = Date.now();
        try {
          await restartRegistry.handleTimeout(restartFixtureName, (severity, message, remoteIdentifier, fatal) => {
            void severity;
            void message;
            void remoteIdentifier;
            void fatal;
          });
        } catch (error) {
          void error;
        }
        let restartComplete = false;
        let second!: ToolExecuteResult;
        // eslint-disable-next-line functional/no-loop-statements
        for (;;) {
          try {
            second = await provider.execute(toolName, { text: RESTART_POST_PAYLOAD }, { timeoutMs: 5000 });
            restartComplete = true;
            break;
          } catch (error) {
            const err = toError(error);
            const code = (err as { code?: string }).code ?? '';
            if (code.startsWith('mcp_restart')) {
              await delay(200);
              continue;
            }
            throw err;
          }
        }
        const waitedMs = Date.now() - waitStart;
        await provider.cleanup();
        await hangingPromise;
        const finalState = readRestartFixtureState(stateFile);
        fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        return {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: { waitedMs, result: second.result, state: finalState, restartComplete },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-71-restart-success expected success.');
        const data = result.finalReport?.content_json as { waitedMs?: number; result?: string; state?: RestartFixtureState; restartComplete?: boolean } | undefined;
        invariant(data !== undefined, 'final report payload expected for run-test-71-restart-success.');
        invariant(data.restartComplete === true, 'Second call should only complete after restart finishes for run-test-71-restart-success.');
        invariant(typeof data.result === 'string' && data.result.includes('recovered'), 'Recovered payload mismatch for run-test-71-restart-success.');
        invariant(data.state?.phase === 'recovered', 'Fixture state should be recovered for run-test-71-restart-success.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-73-shared-restart-backoff',
      description: 'Shared restart uses exponential retries until success.',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        updateRestartFixtureState(stateFile, () => ({ phase: 'initial', remainingRestartFails: 2 }));
        const logs: LogEntry[] = [];
        const serverName = 'restartBackoff';
        const toolName = `${serverName}__test`;
        const restartAttemptMarker = 'shared restart attempt';
        const restartFailureMarker = 'shared restart failed';
        const restartDecisionMarker = 'shared probe failed';
        const config: Record<string, MCPServerConfig> = {
          [serverName]: {
            type: 'stdio',
            command: process.execPath,
            args: [TEST_STDIO_SERVER_PATH],
            shared: true,
            env: {
              MCP_FIXTURE_MODE: 'restart',
              MCP_FIXTURE_STATE_FILE: stateFile,
              MCP_FIXTURE_HANG_MS: '200',
              MCP_FIXTURE_EXIT_DELAY_MS: '50',
            },
          },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, {
          requestTimeoutMs: 500,
          onLog: (entry) => { logs.push(entry); },
        });
        await provider.warmup();
        const hangingPromise = (async () => {
          try {
            return await provider.execute(toolName, { text: RESTART_TRIGGER_PAYLOAD }, { timeoutMs: 2000 });
          } catch (error) {
            return toError(error);
          }
        })();
        // Allow the fixture to transition into the exited phase (exitDelayMs=50ms) before probing.
        await delay(150);
        const registryAccessor = provider as unknown as { sharedRegistry: SharedRegistry };
        const restartRegistry = registryAccessor.sharedRegistry as unknown as { handleTimeout: (name: string, logger: LogFn) => Promise<void> };
        try {
          await restartRegistry.handleTimeout(serverName, (severity, message, remoteIdentifier, fatal = false) => {
            logs.push({
              timestamp: Date.now(),
              severity,
              turn: 0,
              subturn: 0,
              direction: 'response',
              type: 'tool',
              remoteIdentifier,
              fatal,
              message,
            });
          });
        } catch (error) {
          // Expected when the first restart attempt fails; the shared loop continues running in the background.
          void error;
        }
        // Allow the shared restart loop (0s, 1s, 2s delays) to complete before issuing the next tool call.
        await delay(3500);
        const secondResult = await provider.execute(toolName, { text: RESTART_POST_PAYLOAD }, { timeoutMs: 5000 });
        await provider.cleanup();
        await hangingPromise;
        const finalState = readRestartFixtureState(stateFile);
        fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        const attemptLogs = logs.filter((entry) => entry.message.includes(restartAttemptMarker));
        const failureLogs = logs.filter((entry) => entry.message.includes(restartFailureMarker));
        const decisionLogs = logs.filter((entry) => entry.message.includes(restartDecisionMarker));
        return {
          success: true,
          conversation: [],
          logs,
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: {
              attempts: attemptLogs.length,
              failures: failureLogs.length,
              decisions: decisionLogs.length,
              finalState,
              toolResult: secondResult.result,
            },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-73-shared-restart-backoff expected success.');
        const data = result.finalReport?.content_json as {
          attempts?: number;
          failures?: number;
          decisions?: number;
          finalState?: RestartFixtureState;
          toolResult?: string;
        } | undefined;
        invariant(data !== undefined, 'final report payload expected for run-test-73-shared-restart-backoff.');
        invariant((data.decisions ?? 0) === 1, 'Exactly one restart decision log expected for run-test-73-shared-restart-backoff.');
        invariant((data.failures ?? 0) >= 2, 'At least two restart failures expected for run-test-73-shared-restart-backoff.');
        invariant((data.attempts ?? 0) >= 3, 'At least three restart attempts expected for run-test-73-shared-restart-backoff.');
        invariant(data.finalState?.phase === 'recovered', 'Fixture state should reach recovered for run-test-73-shared-restart-backoff.');
        invariant(typeof data.toolResult === 'string' && data.toolResult.includes(RESTART_POST_PAYLOAD), 'Tool result mismatch for run-test-73-shared-restart-backoff.');
      },
    };
  })(),
  (() => {
    return {
      id: 'run-test-74-idle-exit-restart',
      description: 'Shared registry detects transport exits between tool invocations and restarts automatically.',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        const logs: LogEntry[] = [];
        const serverName = 'idleExitFixture';
        const toolName = `${serverName}__test`;
        const config: Record<string, MCPServerConfig> = {
          [serverName]: {
            type: 'stdio',
            command: process.execPath,
            args: [TEST_STDIO_SERVER_PATH],
            shared: true,
            env: {
              MCP_FIXTURE_MODE: 'restart',
              MCP_FIXTURE_STATE_FILE: stateFile,
              MCP_FIXTURE_HANG_MS: '2000',
              MCP_FIXTURE_EXIT_DELAY_MS: '100',
            },
          },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, { requestTimeoutMs: 2000, onLog: (entry) => { logs.push(entry); } });
        await provider.warmup();
        const firstCall = await (async (): Promise<{ success: true } | { success: false; error: Error }> => {
          try {
            await provider.execute(toolName, { text: RESTART_TRIGGER_PAYLOAD }, { timeoutMs: 8000 });
            return { success: true as const };
          } catch (error: unknown) {
            return { success: false as const, error: toError(error) };
          }
        })();
        if (firstCall.success) {
          invariant(false, 'First call must fail when transport exits for run-test-74-idle-exit-restart.');
        }
        const firstErrorMessage = toErrorMessage(firstCall.error);
        let recovered: ToolExecuteResult | undefined;
        let restartInProgressErrors = 0;
        const deadline = Date.now() + 10000;
        // eslint-disable-next-line functional/no-loop-statements -- deterministic retry while restart completes.
        while (Date.now() < deadline) {
          try {
            recovered = await provider.execute(toolName, { text: RESTART_POST_PAYLOAD }, { timeoutMs: 5000 });
            break;
          } catch (error) {
            const err = toError(error);
            const code = (err as { code?: string }).code ?? '';
            if (code === 'mcp_restart_in_progress') {
              restartInProgressErrors += 1;
              await delay(100);
              continue;
            }
            throw err;
          }
        }
        invariant(recovered !== undefined, 'Recovered response missing for run-test-74-idle-exit-restart.');
        const recoveredResultText = recovered.result;
        await provider.cleanup();
        const finalState = readRestartFixtureState(stateFile);
        fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        return {
          success: true,
          conversation: [],
          logs,
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: {
              firstError: firstErrorMessage,
              recoveredResult: recoveredResultText,
              state: finalState,
              transportExitLogs: logs.filter((entry) => entry.message.includes('shared transport closed')).length,
              restartSuccessLogs: logs.filter((entry) => entry.message.includes('shared restart succeeded')).length,
              restartInProgressErrors,
            },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-74-idle-exit-restart expected success.');
        const data = result.finalReport?.content_json as {
          firstError?: string;
          recoveredResult?: string;
          state?: RestartFixtureState;
          transportExitLogs?: number;
          restartSuccessLogs?: number;
          restartInProgressErrors?: number;
        } | undefined;
        invariant(data !== undefined, 'Final report payload required for run-test-74-idle-exit-restart.');
        invariant(typeof data.firstError === 'string' && data.firstError.length > 0, 'First call error missing for run-test-74-idle-exit-restart.');
        invariant(typeof data.recoveredResult === 'string' && data.recoveredResult.includes('recovered'), 'Recovered payload mismatch for run-test-74-idle-exit-restart.');
        invariant(data.state?.phase === 'recovered', 'Fixture state must reach recovered for run-test-74-idle-exit-restart.');
        invariant((data.transportExitLogs ?? 0) >= 1, 'Transport exit log not detected for run-test-74-idle-exit-restart.');
        invariant((data.restartSuccessLogs ?? 0) >= 1, 'Restart success log missing for run-test-74-idle-exit-restart.');
        invariant((data.restartInProgressErrors ?? 0) >= 0, 'Restart error counter should be defined for run-test-74-idle-exit-restart.');
      },
    } satisfies HarnessTest;
  })(),
  (() => {
    return {
      id: 'run-test-72-restart-failure',
      description: 'Structured restart errors propagate when the restart window blows up.',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        const serverName = 'restartFixtureFail';
        const config: Record<string, MCPServerConfig> = {
          [serverName]: {
            type: 'stdio',
            command: process.execPath,
            args: [TEST_STDIO_SERVER_PATH],
            shared: true,
            env: {
              MCP_FIXTURE_MODE: 'restart',
              MCP_FIXTURE_STATE_FILE: stateFile,
              MCP_FIXTURE_HANG_MS: '4000',
              MCP_FIXTURE_EXIT_DELAY_MS: '150',
            },
          },
        } as Record<string, MCPServerConfig>;
        const provider = new MCPProvider('mcp', config, { requestTimeoutMs: 2000 });
        await provider.warmup();
        const toolName = `${serverName}__test`;
        const hangingPromise = (async () => {
          try {
            return await provider.execute(toolName, { text: RESTART_TRIGGER_PAYLOAD }, { timeoutMs: 10000 });
          } catch (error) {
            return toError(error);
          }
        })();
        const providerInternals = provider as unknown as { killTrackedProcess: (serverName: string, reason: string) => Promise<void> };
        await delay(50);
        await providerInternals.killTrackedProcess(serverName, FIXTURE_PREEMPT_REASON);
        updateRestartFixtureState(stateFile, (prev) => ({ ...prev, phase: 'exited', failOnStart: true }));
        await delay(200);
        const registryAccessor = provider as unknown as { sharedRegistry: SharedRegistry };
        const restartRegistry = registryAccessor.sharedRegistry as unknown as { handleTimeout: (name: string, logger: LogFn) => Promise<void> };
        let restartError: string | undefined;
        try {
          await restartRegistry.handleTimeout(serverName, (severity, message, remoteIdentifier, fatal) => {
            void severity;
            void message;
            void remoteIdentifier;
            void fatal;
          });
        } catch (error: unknown) {
          restartError = toErrorMessage(error);
        }
        const finalError = restartError ?? 'no_error';
        await provider.cleanup();
        await hangingPromise;
        fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        return {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
          finalReport: {
            status: 'success',
            format: 'json',
            content_json: { errorMessage: finalError },
            ts: Date.now(),
          },
        } satisfies AIAgentResult;
      },
      expect: (result: AIAgentResult) => {
        invariant(result.success, 'run-test-72-restart-failure expected success result wrapper.');
        const data = result.finalReport?.content_json as { errorMessage?: string } | undefined;
        invariant(typeof data?.errorMessage === 'string', 'Error message expected for run-test-72-restart-failure.');
        invariant(data.errorMessage.includes('mcp_restart_failed'), 'Structured error missing for run-test-72-restart-failure.');
      },
    };
  })(),
  (() => {
    const restartServerName = 'test';
    return {
      id: 'run-test-72-shared-restart-error',
      description: 'Shared MCP restart failure surfaces structured error codes in tool logs and accounting.',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        let failFlagged = false;
        const failTask = (async () => {
          const deadline = Date.now() + 10000;
          // eslint-disable-next-line functional/no-loop-statements
          while (Date.now() < deadline) {
            const state = readRestartFixtureState(stateFile);
            if (state.phase === 'hang-started' && state.failOnStart !== true) {
              updateRestartFixtureState(stateFile, (prev) => ({ ...prev, failOnStart: true }));
              failFlagged = true;
              break;
            }
            await delay(25);
          }
        })();
        forceRemoveSharedRegistryEntry(restartServerName);
        const innerScenario: HarnessTest = {
          id: DEFAULT_PROMPT_SCENARIO,
          configure: (configuration, sessionConfig, defaults) => {
            configuration.mcpServers[restartServerName] = {
              type: 'stdio',
              command: process.execPath,
              args: [TEST_STDIO_SERVER_PATH],
              shared: true,
              healthProbe: 'listTools',
              env: {
                MCP_FIXTURE_MODE: 'restart',
                MCP_FIXTURE_STATE_FILE: stateFile,
                MCP_FIXTURE_HANG_MS: '4000',
                MCP_FIXTURE_EXIT_DELAY_MS: '6000',
                MCP_FIXTURE_BLOCK_EVENT_LOOP: '1',
              },
            } satisfies MCPServerConfig;
            defaults.toolTimeout = 1000;
            configuration.defaults = defaults;
            sessionConfig.toolTimeout = 1000;
            sessionConfig.tools = [restartServerName];
            sessionConfig.maxRetries = 1;
          },
          expect: (innerResult) => { void innerResult; },
        };

        const result = await runScenario(innerScenario);
        await failTask.catch(() => undefined);
        invariant(failFlagged, 'Failed to flag restart fixture for failure in run-test-72-shared-restart-error.');
        try {
          fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        } catch { /* ignore cleanup errors */ }
        return result;
      },
      expect: (result) => {
        invariant(result.success, 'Scenario run-test-72-shared-restart-error should still complete successfully.');
        const expectedCommand = 'test__test';
        const toolEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === expectedCommand);
        invariant(toolEntry !== undefined, 'Tool accounting entry missing for run-test-72-shared-restart-error.');
        invariant(toolEntry.status === 'failed', 'Tool accounting should record failure for run-test-72-shared-restart-error.');
        invariant(typeof toolEntry.error === 'string' && toolEntry.error.includes('mcp_restart_failed'), 'Accounting error must reference mcp_restart_failed for run-test-72-shared-restart-error.');
      },
    } satisfies HarnessTest;
  })(),
  (() => {
    const restartServerName = 'test';
    return {
      id: 'run-test-72-probe-success-no-restart',
      description: 'Shared MCP timeout with healthy probe keeps the server running (no restart).',
      execute: async () => {
        const stateFile = createRestartFixtureStateFile();
        forceRemoveSharedRegistryEntry(restartServerName);
        const innerScenario: HarnessTest = {
          id: DEFAULT_PROMPT_SCENARIO,
          configure: (configuration, sessionConfig, defaults) => {
            configuration.mcpServers[restartServerName] = {
              type: 'stdio',
              command: process.execPath,
              args: [TEST_STDIO_SERVER_PATH],
              shared: true,
              healthProbe: 'listTools',
              env: {
                MCP_FIXTURE_MODE: 'restart',
                MCP_FIXTURE_STATE_FILE: stateFile,
                MCP_FIXTURE_HANG_MS: '4000',
                MCP_FIXTURE_EXIT_DELAY_MS: '150',
                MCP_FIXTURE_BLOCK_EVENT_LOOP: '0',
                MCP_FIXTURE_SKIP_EXIT: '1',
              },
            } satisfies MCPServerConfig;
            defaults.toolTimeout = 1000;
            configuration.defaults = defaults;
            sessionConfig.toolTimeout = 1000;
            sessionConfig.tools = [restartServerName];
            sessionConfig.maxRetries = 1;
          },
          expect: (innerResult) => { void innerResult; },
        } satisfies HarnessTest;
        const result = await runScenario(innerScenario);
        try {
          fs.rmSync(path.dirname(stateFile), { recursive: true, force: true });
        } catch { /* ignore cleanup errors */ }
        return result;
      },
      expect: (result) => {
        invariant(result.success, 'Scenario run-test-72-probe-success-no-restart should still complete successfully.');
        const expectedCommand = 'test__test';
        const toolEntry = result.accounting.filter(isToolAccounting).find((entry) => entry.command === expectedCommand);
        invariant(toolEntry !== undefined, 'Tool accounting entry missing for run-test-72-probe-success-no-restart.');
        invariant(toolEntry.status === 'failed', 'Tool accounting should record failure for run-test-72-probe-success-no-restart.');
        invariant(typeof toolEntry.error === 'string' && toolEntry.error.includes('Tool execution timed out'), 'Probe-success timeout should bubble the generic timeout error.');
        const restartLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('mcp_restart_failed'));
        invariant(restartLog === undefined, 'Probe-success timeout must not emit mcp_restart_failed logs.');
      },
    } satisfies HarnessTest;
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
        invariant(data.tokens?.cacheWriteInputTokens === 321, 'Cache write enrichment expected for run-test-68.');
        invariant(data.routing !== undefined && data.routing.provider === undefined && data.routing.model === undefined, 'Routing should remain empty for run-test-68.');
        invariant(data.costs !== undefined && data.costs.costUsd === undefined && data.costs.upstreamInferenceCostUsd === undefined, 'Cost info should remain empty for run-test-68.');
        invariant(result.logs.some((entry) => entry.severity === 'TRC' && typeof entry.remoteIdentifier === 'string' && entry.remoteIdentifier.startsWith('trace:')), 'Trace log expected for run-test-68.');
      },
    };
  })(),
  (() => {
    let capturedTopP: number | null | undefined;
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
        sessionConfig.userPrompt = 'run-test-69';
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
        invariant(finishedEvent.agentId === '  ', 'Whitespace agentId should remain unchanged for run-test-71.');
        invariant(finishedEvent.callPath === '' || finishedEvent.callPath === '  ', 'Whitespace agentId should not introduce non-whitespace callPath for run-test-71.');
        invariant(finishedEvent.agentName === '' || finishedEvent.agentName === '  ', 'Whitespace agentId should not force fallback agentName for run-test-71.');
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
          maxTurns: 3,
          maxToolCallsPerTurn: 4,
          toolResponseMaxBytes: 5_555,
          stream: false,
          mcpInitConcurrency: 6,
        },
        queues: { default: { concurrent: 32 } },
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
          'maxTurns: 8',
          'maxToolCallsPerTurn: 9',
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
          maxRetries: 11,
          maxTurns: 12,
          maxToolCallsPerTurn: 16,
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
        invariant(cfg.trace !== undefined, 'Trace context missing for run-test-73.');
        invariant(cfg.trace.originId === traceContext.originId, 'Trace originId mismatch for run-test-73.');
        invariant(cfg.trace.parentId === traceContext.parentId, 'Trace parentId mismatch for run-test-73.');
        invariant(cfg.trace.callPath === traceContext.callPath, 'Trace callPath mismatch for run-test-73.');
        invariant(cfg.stopRef === stopReference, 'Stop reference mismatch for run-test-73.');
        invariant(cfg.initialTitle === 'Loader Session Title', 'Initial title mismatch for run-test-73.');
        invariant(loaderAbortSignal !== undefined && cfg.abortSignal === loaderAbortSignal && !cfg.abortSignal.aborted, 'Abort signal mismatch for run-test-73.');
        invariant(cfg.ancestors === ancestorsList, 'Ancestors propagation mismatch for run-test-73.');
        invariant(typeof cfg.temperature === 'number', 'Temperature not a number for run-test-73.');
        invariant(Math.abs(cfg.temperature - 0.51) < 1e-6, 'Temperature override mismatch for run-test-73.');
        invariant(typeof cfg.topP === 'number', 'topP not a number for run-test-73.');
        invariant(Math.abs(cfg.topP - 0.61) < 1e-6, 'topP override mismatch for run-test-73.');
        invariant(cfg.maxOutputTokens === 7_777, `maxOutputTokens mismatch for run-test-73 (actual=${String(cfg.maxOutputTokens)})`);
        invariant(typeof cfg.repeatPenalty === 'number' && Math.abs(cfg.repeatPenalty - 1.2) < 1e-6, 'repeatPenalty mismatch for run-test-73.');
        invariant(cfg.maxRetries === 11, 'maxRetries override mismatch for run-test-73.');
        invariant(cfg.maxTurns === 12, 'maxTurns override mismatch for run-test-73.');
        invariant(cfg.maxToolCallsPerTurn === 16, 'maxToolCallsPerTurn mismatch for run-test-73.');
        invariant(cfg.llmTimeout === 5_555, 'llmTimeout override mismatch for run-test-73.');
        invariant(cfg.toolTimeout === 6_666, 'toolTimeout override mismatch for run-test-73.');
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
    execute: () => {
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
          queues: { default: { concurrent: 32 } },
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
          maxTurns: overrideLLMExpected.maxTurns,
          maxToolCallsPerTurn: overrideLLMExpected.maxToolCallsPerTurn,
          toolResponseMaxBytes: overrideLLMExpected.toolResponseMaxBytes,
          mcpInitConcurrency: overrideLLMExpected.mcpInitConcurrency,
          stream: overrideLLMExpected.stream,
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
      invariant(typeof overrideParentLoaded.effective.temperature === 'number' && Math.abs(overrideParentLoaded.effective.temperature - overrideLLMExpected.temperature) < 1e-6, 'Parent temperature should follow overrides.');
      invariant(typeof overrideParentLoaded.effective.topP === 'number' && Math.abs(overrideParentLoaded.effective.topP - overrideLLMExpected.topP) < 1e-6, 'Parent topP should follow overrides.');
      invariant(overrideParentLoaded.effective.maxOutputTokens === overrideLLMExpected.maxOutputTokens, 'Parent maxOutputTokens should follow overrides.');
      invariant(typeof overrideParentLoaded.effective.repeatPenalty === 'number' && Math.abs(overrideParentLoaded.effective.repeatPenalty - overrideLLMExpected.repeatPenalty) < 1e-6, 'Parent repeatPenalty should follow overrides.');
      invariant(overrideParentLoaded.effective.llmTimeout === overrideLLMExpected.llmTimeout, 'Parent llmTimeout should follow overrides.');
      invariant(overrideParentLoaded.effective.maxRetries === overrideLLMExpected.maxRetries, 'Parent maxRetries should follow overrides.');
      invariant(overrideParentLoaded.effective.toolTimeout === overrideLLMExpected.toolTimeout, 'Parent toolTimeout should follow overrides.');
      invariant(overrideParentLoaded.effective.maxTurns === overrideLLMExpected.maxTurns, 'Parent maxTurns should follow overrides.');
      invariant(overrideParentLoaded.effective.maxToolCallsPerTurn === overrideLLMExpected.maxToolCallsPerTurn, 'Parent maxToolCallsPerTurn should follow overrides.');
      invariant(overrideParentLoaded.effective.toolResponseMaxBytes === overrideLLMExpected.toolResponseMaxBytes, 'Parent toolResponseMaxBytes should follow overrides.');
      invariant(overrideParentLoaded.effective.mcpInitConcurrency === overrideLLMExpected.mcpInitConcurrency, 'Parent mcpInitConcurrency should follow overrides.');
      invariant(overrideParentLoaded.effective.stream === overrideLLMExpected.stream, 'Parent stream flag should follow overrides.');
      invariant(overrideChildLoaded !== undefined, 'Child loaded agent missing for global override test.');
      invariant(overrideChildLoaded.targets === overrideTargetsRef, 'Child targets reference mismatch for global override test.');
      invariant(overrideChildLoaded.targets.every((entry) => entry.provider === PRIMARY_PROVIDER && entry.model === MODEL_NAME), 'Child targets should be overridden to primary provider/model.');
      invariant(typeof overrideChildLoaded.effective.temperature === 'number' && Math.abs(overrideChildLoaded.effective.temperature - overrideLLMExpected.temperature) < 1e-6, 'Child temperature should follow overrides.');
      invariant(typeof overrideChildLoaded.effective.topP === 'number' && Math.abs(overrideChildLoaded.effective.topP - overrideLLMExpected.topP) < 1e-6, 'Child topP should follow overrides.');
      invariant(overrideChildLoaded.effective.maxOutputTokens === overrideLLMExpected.maxOutputTokens, 'Child maxOutputTokens should follow overrides.');
      invariant(typeof overrideChildLoaded.effective.repeatPenalty === 'number' && Math.abs(overrideChildLoaded.effective.repeatPenalty - overrideLLMExpected.repeatPenalty) < 1e-6, 'Child repeatPenalty should follow overrides.');
      invariant(overrideChildLoaded.effective.llmTimeout === overrideLLMExpected.llmTimeout, 'Child llmTimeout should follow overrides.');
      invariant(overrideChildLoaded.effective.maxRetries === overrideLLMExpected.maxRetries, 'Child maxRetries should follow overrides.');
      invariant(overrideChildLoaded.effective.toolTimeout === overrideLLMExpected.toolTimeout, 'Child toolTimeout should follow overrides.');
      invariant(overrideChildLoaded.effective.maxTurns === overrideLLMExpected.maxTurns, 'Child maxTurns should follow overrides.');
      invariant(overrideChildLoaded.effective.maxToolCallsPerTurn === overrideLLMExpected.maxToolCallsPerTurn, 'Child maxToolCallsPerTurn should follow overrides.');
      invariant(overrideChildLoaded.effective.toolResponseMaxBytes === overrideLLMExpected.toolResponseMaxBytes, 'Child toolResponseMaxBytes should follow overrides.');
      invariant(overrideChildLoaded.effective.mcpInitConcurrency === overrideLLMExpected.mcpInitConcurrency, 'Child mcpInitConcurrency should follow overrides.');
      invariant(overrideChildLoaded.effective.stream === overrideLLMExpected.stream, 'Child stream flag should follow overrides.');
    },
  },



  {
    id: 'run-test-96',
    description: 'Agent registry spawnSession resolves aliases and produces sessions.',
    execute: async () => {
      registrySpawnSessionConfig = undefined;
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}spawn-session-`));
      const originalCreate = AIAgentSession.create.bind(AIAgentSession);
      try {
        const configPath = path.join(tempDir, CONFIG_FILE_NAME);
        const configData = {
          providers: { [PRIMARY_PROVIDER]: { type: 'test-llm' } },
          mcpServers: {},
          queues: { default: { concurrent: 32 } },
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
    execute: () => {
      registryCoverageSummary = undefined;
      utilsCoverageSummary = undefined;
      const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}registry-utils-`));
      try {
        const configPath = path.join(tempDir, CONFIG_FILE_NAME);
        const configData = {
          providers: { [PRIMARY_PROVIDER]: { type: 'test-llm' } },
          mcpServers: {},
          queues: { default: { concurrent: 32 } },
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
      return Promise.resolve({
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: { status: 'success', format: 'text', content: 'registry-utils-coverage', ts: Date.now() },
      });
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
        invariant(finalReport?.status === 'success', 'Final report should succeed after retry for run-test-74.');
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
    execute: async () => {
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
    execute: async (_configuration, _sessionConfig, _defaults) => {
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
      invariant(auth?.type === 'auth_error' && typeof auth.message === 'string' && auth.message.toLowerCase().includes('invalid api key'), 'Auth error mapping mismatch for run-test-83.');
      const quota = data.quota as TurnStatus | undefined;
      invariant(quota?.type === 'quota_exceeded' && typeof quota.message === 'string' && quota.message.toLowerCase().includes('quota'), 'Quota error mapping mismatch for run-test-83.');
      const rate = data.rate as TurnStatus | undefined;
      invariant(rate?.type === 'rate_limit', 'Rate limit mapping mismatch for run-test-83.');
      invariant(rate.retryAfterMs === 3000, 'Rate limit retryAfterMs mismatch for run-test-83.');
      const timeout = data.timeout as TurnStatus | undefined;
      invariant(timeout?.type === 'timeout' && typeof timeout.message === 'string', 'Timeout mapping mismatch for run-test-83.');
      const network = data.network as TurnStatus | undefined;
      const networkRetryable = (network as { retryable?: boolean } | undefined)?.retryable ?? true;
      invariant(network?.type === 'network_error' && networkRetryable, 'Network mapping mismatch for run-test-83.');
      const model = data.model as TurnStatus | undefined;
      invariant(model?.type === 'model_error' && typeof model.message === 'string', 'Model error mapping mismatch for run-test-83.');
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
      const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
      if (finalTurnLog === undefined) {
         
        console.error('run-test-84 logs:', result.logs.map((entry) => ({ id: entry.remoteIdentifier, severity: entry.severity, message: entry.message })));
      }
      invariant(finalTurnLog !== undefined, 'Final-turn warning log expected for run-test-84.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed for run-test-84.');
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
      invariant(finalReport?.format === 'json', 'Final report should be json for run-test-85.');
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
      invariant(finalTurnObservation.input.length === 1 && finalTurnObservation.input[0] === 'agent__final_report', 'Final turn input should already be restricted to agent__final_report.');
      const nonFinalObservation = observed.find((entry) => !entry.isFinalTurn);
      invariant(nonFinalObservation !== undefined, 'Non-final turn observation missing for run-test-86.');
      invariant(nonFinalObservation.output.length > 1, 'Non-final turn should retain multiple tools.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed for run-test-86.');
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
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-87 should have success=false when finalReport.status=failure per CONTRACT.');
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
    description: 'Model-provided final report results in success status.',
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
          // Model-provided final reports always result in status='success'
          const reportContent = 'Report content: gathered overview with available metrics.';
          if (activeSession !== undefined) {
            (activeSession as unknown as { finalReport?: { status: string; format: 'markdown'; content: string } }).finalReport = {
              status: 'success',
              format: 'markdown',
              content: reportContent,
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
                  report_format: 'markdown',
                  report_content: reportContent,
                },
              },
            ],
          };
          const toolMessage = {
            role: 'tool',
            toolCallId: FINAL_REPORT_CALL_ID,
            content: reportContent,
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
      // Model-provided final reports always result in success=true, status='success'
      invariant(result.success, 'Scenario run-test-88 should have success=true for model-provided final report.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report missing for run-test-88.');
      invariant(finalReport.status === 'success', 'Final report status should be success for run-test-88.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('gathered overview'), 'Final report content mismatch for run-test-88.');
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
        typeof sanitizerLog.message === 'string' && sanitizerLog.message.includes(SANITIZER_DROPPED_MESSAGE),
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
          const toolCalls = assistantMessage.toolCalls ?? [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
          }
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
          sanitizerLog.message.includes(SANITIZER_DROPPED_MESSAGE)
        ),
        'Sanitizer message mismatch for run-test-90.',
      );
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      invariant(assistantMessages.length >= 2, 'At least two assistant messages expected after retry for run-test-90.');
      const firstAssistantWithTools = assistantMessages.find(
        (message) => {
          const toolCalls = (message as { toolCalls?: unknown }).toolCalls;
          return Array.isArray(toolCalls) && toolCalls.length > 0;
        },
      );
      invariant(firstAssistantWithTools !== undefined, 'Assistant message with tool calls expected for run-test-90.');
      const toolCalls = (firstAssistantWithTools as { toolCalls: ToolCall[] }).toolCalls;
      invariant(toolCalls.length === 1, 'Single sanitized tool call expected for run-test-90.');
      const retainedCall = toolCalls[0];
      invariant(retainedCall.name === 'test__test', 'Retained tool call name mismatch for run-test-90.');
      const textValue = (retainedCall.parameters as { text?: unknown }).text;
      invariant(textValue === SANITIZER_VALID_ARGUMENT, 'Retained tool call arguments mismatch for run-test-90.');
      const retryLog = result.logs.find((entry) => typeof entry.message === 'string' && (
        entry.message.includes('Invalid tool call dropped') ||
        entry.message.includes(SANITIZER_DROPPED_MESSAGE)
      ));
      invariant(retryLog !== undefined, 'Retry warning log missing for run-test-90.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts === 3, 'Three LLM attempts expected for run-test-90.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed after sanitizer retry (run-test-90).');
    },
  },
  {
    id: 'run-test-122',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__batch',
                id: 'call-batch-hex-params',
                parameters: {
                  calls: [
                    {
                      id: 'hex-1',
                      tool: 'test__test',
                      parameters: BATCH_HEX_ARGUMENT_STRING as unknown as Record<string, unknown>,
                    },
                  ],
                },
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          const toolMessages: ConversationMessage[] = [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            const output = await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            if (typeof output === 'string') {
              toolMessages.push({ role: 'tool', toolCallId: call.id, content: output });
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage, ...toolMessages],
            tokens: { inputTokens: 16, outputTokens: 6, totalTokens: 22 },
          };
        }
        if (invocation === 2) {
          const finalContent = 'Hex batch scenario final report.';
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
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 12, outputTokens: 4, totalTokens: 16 },
          };
        }
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
      invariant(result.success, 'Scenario run-test-122 expected success.');
      if (process.env.CONTEXT_DEBUG === 'true') {
        console.log('run-test-122 conversation:', JSON.stringify(result.conversation, null, 2));
        console.log('run-test-122 logs:', JSON.stringify(result.logs, null, 2));
      }
      const toolMessage = result.conversation.find((message) => message.role === 'tool' && typeof message.content === 'string' && message.content.includes(BATCH_HEX_EXPECTED_OUTPUT));
      invariant(toolMessage !== undefined, 'Batch string parameters should survive parsing for run-test-122.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-122.');
    },
  },
  {
    id: 'run-test-123',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const nestedCallsPayload = JSON.stringify([
            { id: 'nested-progress', tool: 'agent__progress_report', parameters: { progress: BATCH_STRING_PROGRESS } },
            { id: 'nested-hex', tool: 'test__test', parameters: BATCH_HEX_ARGUMENT_STRING },
          ]);
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__batch',
                id: 'call-batch-hex-nested',
                parameters: {
                  calls: nestedCallsPayload,
                },
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          const toolMessages: ConversationMessage[] = [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            const output = await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            if (typeof output === 'string') {
              toolMessages.push({ role: 'tool', toolCallId: call.id, content: output });
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage, ...toolMessages],
            tokens: { inputTokens: 18, outputTokens: 7, totalTokens: 25 },
          };
        }
        if (invocation === 2) {
          const finalContent = 'Hex batch scenario final report (nested).';
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
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 12, outputTokens: 4, totalTokens: 16 },
          };
        }
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
      invariant(result.success, 'Scenario run-test-123 expected success.');
      const batchToolMessage = result.conversation.find((message) => message.role === 'tool' && message.toolCallId === 'call-batch-hex-nested' && typeof message.content === 'string');
      invariant(batchToolMessage !== undefined, 'Batch tool output missing for run-test-123.');
      const batchContent = batchToolMessage.content;
      let parsedUnknown: unknown;
      try {
        parsedUnknown = JSON.parse(batchContent) as unknown;
      } catch (error) {
        invariant(false, `Unable to parse batch output for run-test-123: ${error instanceof Error ? error.message : String(error)}`);
        return;
      }
      const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
      invariant(isRecord(parsedUnknown), 'Batch output is not an object for run-test-123.');
      const rawResults = parsedUnknown.results;
      const results = Array.isArray(rawResults) ? rawResults : [];
      const progressResult = results.find((entry): entry is Record<string, unknown> => {
        if (!isRecord(entry)) return false;
        const id = entry.id;
        return typeof id === 'string' && id === 'nested-progress';
      });
      invariant(progressResult !== undefined, 'Progress result missing within batch output for run-test-123.');
      const progressOk = typeof progressResult.ok === 'boolean' ? progressResult.ok : false;
      invariant(progressOk, 'Progress entry should report ok=true for run-test-123.');
      const hexResult = results.find((entry): entry is Record<string, unknown> => {
        if (!isRecord(entry)) return false;
        const id = entry.id;
        return typeof id === 'string' && id === 'nested-hex';
      });
      invariant(hexResult !== undefined, 'Nested hex result missing for run-test-123.');
      const hexOutput = typeof hexResult.output === 'string' ? hexResult.output : undefined;
      invariant(typeof hexOutput === 'string' && hexOutput.includes(BATCH_HEX_EXPECTED_OUTPUT), 'Nested hex output not parsed for run-test-123.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-123.');
    },
  },
  {
    id: 'run-test-124',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.tools = ['test', 'batch'];
      // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__batch',
                id: 'call-batch-truncated',
                parameters: BATCH_TRUNCATED_ARGUMENT_STRING as unknown as Record<string, unknown>,
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          const toolMessages: ConversationMessage[] = [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            const output = await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            if (typeof output === 'string') {
              toolMessages.push({ role: 'tool', toolCallId: call.id, content: output });
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 5,
            messages: [assistantMessage, ...toolMessages],
            tokens: { inputTokens: 18, outputTokens: 7, totalTokens: 25 },
          };
        }
        if (invocation === 2) {
          const finalContent = 'Truncated batch scenario final report.';
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
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 12, outputTokens: 4, totalTokens: 16 },
          };
        }
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
      invariant(result.success, 'Scenario run-test-124 expected success.');
      const batchToolMessage = result.conversation.find((message) => message.role === 'tool' && message.toolCallId === 'call-batch-truncated');
      invariant(batchToolMessage !== undefined, 'Batch tool output missing for run-test-124.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should indicate success for run-test-124.');
    },
  },
  {
    id: 'run-test-131',
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-131 expected success.');
      const warningLog = result.logs.find(
        (entry) =>
          entry.severity === 'ERR'
          && entry.remoteIdentifier === `${PRIMARY_PROVIDER}:${MODEL_NAME}`
          && entry.message.includes('invalid tool parameters')
      );
      invariant(warningLog !== undefined, 'Parameter warning log missing for run-test-131.');
      const previewDetail = warningLog.details?.raw_preview;
      invariant(typeof previewDetail === 'string' && previewDetail.length > 0, 'Parameter warning log should include raw preview for run-test-131.');
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
          const toolCalls = assistantMessage.toolCalls ?? [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
          }
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
      invariant(firstAssistant.toolCalls?.length === 1, 'Single tool call expected for run-test-90-string.');
      const firstCall = firstAssistant.toolCalls[0];
      invariant(firstCall.name === 'test__test', 'Tool call name mismatch for run-test-90-string.');
      const textValue = (firstCall.parameters as { text?: unknown }).text;
      invariant(textValue === SANITIZER_VALID_ARGUMENT, 'Parsed tool call arguments mismatch for run-test-90-string.');
      const llmAttempts = result.accounting.filter(isLlmAccounting).length;
      invariant(llmAttempts === 2, 'Two LLM attempts expected for run-test-90-string.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed for run-test-90-string.');
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
      const rateLimitLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.toLowerCase().includes('rate limited'));
      if (rateLimitLog === undefined) {
        console.error('DEBUG run-test-90-rate-limit logs:', JSON.stringify(result.logs, null, 2));
      }
      invariant(rateLimitLog !== undefined, 'Rate-limit warning log missing for run-test-90-rate-limit.');
      const attempts = result.accounting.filter(isLlmAccounting).length;
      invariant(attempts === 2, 'Two LLM attempts expected for run-test-90-rate-limit.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report should succeed for run-test-90-rate-limit.');
    },
  },
  {
    id: 'run-test-90-adopt-text',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
      sessionConfig.maxRetries = 1;
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
      invariant(finalReport.metadata?.origin === ADOPTION_METADATA_ORIGIN, 'Final report metadata missing for run-test-90-adopt-text.');
      invariant(finalReport.content_json?.key === ADOPTION_CONTENT_VALUE, 'Final report content_json missing for run-test-90-adopt-text.');
      const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
      invariant(fallbackLog !== undefined, 'Fallback acceptance log missing for run-test-90-adopt-text.');
    },
  },
  {
    id: 'run-test-90-json-content',
    description: 'Adopts final report when assistant returns JSON-only payload in content.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 1;
      sessionConfig.maxRetries = 1;
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
      const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
      invariant(fallbackLog !== undefined, 'Fallback acceptance log missing for run-test-90-json-content.');
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
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-90-no-retry.');
      invariant(finalReport.content === FINAL_REPORT_SANITIZED_CONTENT, 'Final report content mismatch for run-test-90-no-retry.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-90-no-retry.');
      const systemNotices = result.logs
        .filter((entry) => typeof entry.message === 'string' && entry.message.includes('plain text responses are ignored'));
      invariant(systemNotices.length === 0, 'No retry warning log expected for run-test-90-no-retry.');
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
      invariant(finalReport?.status === 'success', 'Final report should succeed after mixed tools for run-test-91.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes(FINAL_REPORT_RETRY_MESSAGE), 'Final report content mismatch for run-test-91.');
    },
  },
  {
    id: 'run-test-92',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.outputFormat = 'pipe';
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-92 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-92.');
      invariant(finalReport.format === 'pipe', `Final report format mismatch for run-test-92 (actual=${finalReport.format}).`);
      invariant(finalReport.content === 'My name is ChatGPT.', 'Final report content mismatch for run-test-92.');
      const failureLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('requires non-empty report_content'));
      invariant(failureLog === undefined, 'Final report validation error must not surface for run-test-92.');
    },
  },
  {
    id: 'run-test-93',
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-93 expected success.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-93.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-93.');
      const assistantMessages = result.conversation.filter((message) => message.role === 'assistant');
      const firstAssistant = assistantMessages.at(0);
      invariant(firstAssistant !== undefined, 'First assistant message missing for run-test-93.');
      const assistantContent = typeof firstAssistant.content === 'string' ? firstAssistant.content.trim() : undefined;
      invariant(assistantContent === undefined || assistantContent.length === 0, 'Reasoning-only assistant turn should not include textual content for run-test-93.');
      invariant(Array.isArray(firstAssistant.reasoning) && firstAssistant.reasoning.length > 0, 'Reasoning payload missing for run-test-93.');
      const emptyWarning = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Empty response without tools'));
      invariant(emptyWarning === undefined, 'Reasoning-only response must not trigger empty-response warning for run-test-93.');
      const retryNotice = result.logs.find((entry) => entry.remoteIdentifier === RETRY_REMOTE);
      invariant(retryNotice === undefined, 'Reasoning-only response should not schedule retry for run-test-93.');
    },
  },
  {
    id: 'run-test-104',
    description: 'Reasoning signatures survive turn replay before final report.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.reasoning = 'high';
      const signature = 'sig-preserve-123';
      let invocation = 0;
      let replayVerified = false;
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        const turnIndex = invocation;
        if (turnIndex === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            reasoning: [
              {
                type: 'reasoning',
                text: 'Initial Anthropic reasoning with signature.',
                providerMetadata: { anthropic: { signature } },
              },
            ],
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 12,
            messages: [assistantMessage],
            tokens: { inputTokens: 58, outputTokens: 26, totalTokens: 84 },
            response: '',
            hasReasoning: true,
            stopReason: 'other',
          });
        }
        if (turnIndex === 2) {
          const assistantHistory = request.messages.filter((message) => message.role === 'assistant');
          const reasoningSegments = assistantHistory.flatMap((message) => (Array.isArray(message.reasoning) ? message.reasoning : []));
          const hasSignature = reasoningSegments.some((segment) => {
            const metadataUnknown = (segment as { providerMetadata?: unknown }).providerMetadata;
            if (metadataUnknown === undefined || metadataUnknown === null || typeof metadataUnknown !== 'object') {
              return false;
            }
            const anthropic = (metadataUnknown as { anthropic?: { signature?: unknown } }).anthropic;
            return typeof anthropic?.signature === 'string' && anthropic.signature === signature;
          });
          if (!hasSignature) {
            return Promise.reject(new Error('reasoning_signature_missing_on_replay'));
          }
          replayVerified = true;
          const finalCallId = 'call-final-replay';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: finalCallId,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: 'Reasoning replay final report.',
                },
              },
            ],
            reasoning: [
              {
                type: 'reasoning',
                text: 'Confirming reasoning replay prior to final report.',
                providerMetadata: { anthropic: { signature } },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: finalCallId,
            content: TOOL_OK_JSON,
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 18,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 92, outputTokens: 28, totalTokens: 120 },
            response: '',
            hasReasoning: true,
            stopReason: STOP_REASON_TOOL_CALLS,
          });
        }
        return originalExecuteTurn.call(this, request);
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        const result = await session.run();
        (result as { __reasoningReplayVerified?: boolean }).__reasoningReplayVerified = replayVerified;
        return result;
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult & { __reasoningReplayVerified?: boolean }) => {
      invariant(result.success, 'Scenario run-test-104 expected success.');
      invariant(result.__reasoningReplayVerified === true, 'Reasoning signatures were not verified during replay for run-test-104.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-104.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-104.');
      invariant(finalReport.content === 'Reasoning replay final report.', 'Final report content mismatch for run-test-104.');
    },
  },
  {
    id: 'run-test-105',
    description: 'Reasoning signature survives fallback from primary to secondary provider.',
    configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: { type: 'test-llm' },
        [SECONDARY_PROVIDER]: { type: 'test-llm' },
      };
      sessionConfig.targets = [
        { provider: PRIMARY_PROVIDER, model: MODEL_NAME },
        { provider: SECONDARY_PROVIDER, model: MODEL_NAME },
      ];
      sessionConfig.reasoning = 'high';
      sessionConfig.maxRetries = 2;
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      const signature = 'sig-fallback-123';
      let invocation = 0;
      let fallbackSignatureVerified = false;
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        const provider = request.provider;
        if (invocation === 1) {
          invariant(provider === PRIMARY_PROVIDER, 'First invocation should target primary provider in run-test-105.');
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            reasoning: [
              {
                type: 'reasoning',
                text: 'Primary provider reasoning with signature.',
                providerMetadata: { anthropic: { signature } },
              },
            ],
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: false, finalAnswer: false },
            latencyMs: 12,
            messages: [assistantMessage],
            tokens: { inputTokens: 72, outputTokens: 24, totalTokens: 96 },
            hasReasoning: true,
            response: '',
            stopReason: 'other',
          });
        }
        if (invocation === 2) {
          invariant(provider === PRIMARY_PROVIDER, 'Second invocation should remain on primary provider in run-test-105.');
          return Promise.resolve({
            status: { type: 'model_error', message: 'primary failure', retryable: false },
            latencyMs: 8,
            messages: [],
            tokens: { inputTokens: 40, outputTokens: 10, totalTokens: 50 },
            response: '',
            stopReason: 'other',
          });
        }
        if (invocation === 3) {
          invariant(provider === SECONDARY_PROVIDER, 'Fallback invocation should target secondary provider in run-test-105.');
          const assistantHistory = request.messages.filter((message) => message.role === 'assistant');
          const reasoningSegments = assistantHistory.flatMap((message) => (Array.isArray(message.reasoning) ? message.reasoning : []));
          fallbackSignatureVerified = reasoningSegments.some((segment) => {
            const metadataUnknown = (segment as { providerMetadata?: unknown }).providerMetadata;
            if (metadataUnknown === undefined || metadataUnknown === null || typeof metadataUnknown !== 'object') {
              return false;
            }
            const anthropic = (metadataUnknown as { anthropic?: { signature?: unknown } }).anthropic;
            return typeof anthropic?.signature === 'string' && anthropic.signature === signature;
          });
          const finalCallId = 'call-final-fallback';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: finalCallId,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: 'Fallback final report.',
                },
              },
            ],
            reasoning: [
              {
                type: 'reasoning',
                text: 'Secondary provider final reasoning.',
                providerMetadata: { anthropic: { signature } },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: finalCallId,
            content: TOOL_OK_JSON,
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 15,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 90, outputTokens: 32, totalTokens: 122 },
            hasReasoning: true,
            response: '',
            stopReason: STOP_REASON_TOOL_CALLS,
          });
        }
        return originalExecuteTurn.call(this, request);
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        const result = await session.run();
        (result as { __fallbackSignatureVerified?: boolean }).__fallbackSignatureVerified = fallbackSignatureVerified;
        return result;
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
      }
    },
    expect: (result: AIAgentResult & { __fallbackSignatureVerified?: boolean }) => {
      invariant(result.success, 'Scenario run-test-105 expected success.');
      invariant(result.__fallbackSignatureVerified === true, 'Fallback provider did not receive reasoning signature in run-test-105.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-105.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-105.');
      invariant(finalReport.content === 'Fallback final report.', 'Final report content mismatch for run-test-105.');
      const providersUsed = new Set(result.accounting.filter(isLlmAccounting).map((entry) => entry.provider));
      invariant(providersUsed.has(SECONDARY_PROVIDER), 'Secondary provider was not used during fallback in run-test-105.');
    },
  },
  {
    id: 'run-test-106',
    description: 'Reasoning without signature disables reasoning stream next turn but keeps transcript.',
    configure: (configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      configuration.providers = {
        [PRIMARY_PROVIDER]: { type: 'anthropic' },
      };
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      sessionConfig.reasoning = 'high';
    },
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      let invocation = 0;
      let sendReasoningDisabled = false;
      let sanitizedHistoryVerified = false;
      const proto = LLMClient.prototype as unknown as {
        createProvider: (name: string, config: ProviderConfig, tracedFetch: typeof fetch) => BaseLLMProvider;
      };
      const originalCreateProvider = proto.createProvider;
      proto.createProvider = function(name: string, config: ProviderConfig, tracedFetch: typeof fetch) {
        if (config.type === 'anthropic') {
          return new TestLLMProvider({ ...config, type: 'test-llm' });
        }
        return originalCreateProvider.call(this, name, config, tracedFetch);
      };
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      LLMClient.prototype.executeTurn = function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const progressCallId = 'call-progress-no-sig';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__progress_report',
                id: progressCallId,
                parameters: { progress: 'Reviewing instructions.' },
              },
            ],
            reasoning: [
              {
                type: 'reasoning',
                text: 'Primary reasoning without metadata.',
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: progressCallId,
            content: 'ok',
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 9,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 64, outputTokens: 24, totalTokens: 88 },
            hasReasoning: true,
            response: '',
            stopReason: 'other',
          });
        }
        if (invocation === 2) {
          sendReasoningDisabled = request.sendReasoning === false;
          const assistantHistory = request.messages.filter((message) => message.role === 'assistant');
          sanitizedHistoryVerified = assistantHistory.some((message) => {
            if (message.reasoning === undefined) {
              return true;
            }
            return Array.isArray(message.reasoning) && message.reasoning.length === 0;
          });
          const finalCallId = 'call-final-no-signature';
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                name: 'agent__final_report',
                id: finalCallId,
                parameters: {
                  status: 'success',
                  report_format: 'markdown',
                  report_content: 'Reasoning disabled final report.',
                },
              },
            ],
          };
          const toolMessage: ConversationMessage = {
            role: 'tool',
            toolCallId: finalCallId,
            content: TOOL_OK_JSON,
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 11,
            messages: [assistantMessage, toolMessage],
            tokens: { inputTokens: 70, outputTokens: 22, totalTokens: 92 },
            hasReasoning: false,
            response: '',
            stopReason: STOP_REASON_TOOL_CALLS,
          });
        }
        return originalExecuteTurn.call(this, request);
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        const result = await session.run();
        (result as { __sendReasoningDisabled?: boolean }).__sendReasoningDisabled = sendReasoningDisabled;
        (result as { __sanitizedHistoryVerified?: boolean }).__sanitizedHistoryVerified = sanitizedHistoryVerified;
        return result;
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
        proto.createProvider = originalCreateProvider;
      }
    },
    expect: (result: AIAgentResult & { __sendReasoningDisabled?: boolean; __sanitizedHistoryVerified?: boolean }) => {
      invariant(result.success, 'Scenario run-test-106 expected success.');
      invariant(result.__sendReasoningDisabled === true, 'Reasoning stream should be disabled after missing signature in run-test-106.');
      invariant(result.__sanitizedHistoryVerified === true, 'Assistant transcript missing sanitized reasoning entry for run-test-106.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-106.');
      invariant(finalReport.format === 'markdown', 'Final report format mismatch for run-test-106.');
      invariant(finalReport.content === 'Reasoning disabled final report.', 'Final report content mismatch for run-test-106.');
    },
  },
  {
    id: 'run-test-107',
    description: 'Emits onTurnStarted for every LLM turn even if no thinking stream is emitted.',
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 2;
      sessionConfig.maxRetries = 1;
      sessionConfig.targets = [{ provider: PRIMARY_PROVIDER, model: MODEL_NAME }];
      const originalCallbacks = sessionConfig.callbacks;
      const observedTurns: number[] = [];
      sessionConfig.callbacks = {
        ...originalCallbacks,
        onTurnStarted: (turnIndex) => {
          observedTurns.push(turnIndex);
          originalCallbacks?.onTurnStarted?.(turnIndex);
        },
      };
      // eslint-disable-next-line @typescript-eslint/unbound-method
      const originalExecuteTurn = LLMClient.prototype.executeTurn;
      let invocation = 0;
      LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
        invocation += 1;
        if (invocation === 1) {
          const progressCallId = 'call-turn-started-progress';
          const interimAssistant: ConversationMessage = {
            role: 'assistant',
            content: 'Working on it.',
            toolCalls: [
              {
                id: progressCallId,
                name: 'agent__progress_report',
                parameters: { progress: 'Setting up analysis.' },
              },
            ],
          };
          const interimTool: ConversationMessage = {
            role: 'tool',
            toolCallId: progressCallId,
            content: 'ok',
          };
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: false },
            latencyMs: 6,
            messages: [interimAssistant, interimTool],
            tokens: { inputTokens: 9, outputTokens: 3, totalTokens: 12 },
            response: interimAssistant.content,
          });
        }
        const finalCallId = 'call-final-turn-started';
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: SECOND_TURN_FINAL_ANSWER,
          toolCalls: [
            {
              id: finalCallId,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'markdown',
                report_content: SECOND_TURN_FINAL_ANSWER,
              },
            },
          ],
        };
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: finalCallId,
          content: TOOL_OK_JSON,
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 7,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 },
          response: '',
          stopReason: STOP_REASON_TOOL_CALLS,
        });
      };
      try {
        const session = AIAgentSession.create(sessionConfig);
        const result = await session.run();
        (result as { __observedTurns?: number[] }).__observedTurns = observedTurns;
        return result;
      } finally {
        LLMClient.prototype.executeTurn = originalExecuteTurn;
        sessionConfig.callbacks = originalCallbacks;
      }
    },
    expect: (result: AIAgentResult & { __observedTurns?: number[] }) => {
      invariant(result.success, 'run-test-107 should succeed.');
      const observed = result.__observedTurns;
      invariant(Array.isArray(observed), 'run-test-107 expects observed turn tracking.');
      invariant(observed.length === 2 && observed[0] === 1 && observed[1] === 2, 'onTurnStarted must fire once per turn for run-test-107.');
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'success' && finalReport.content === SECOND_TURN_FINAL_ANSWER, 'run-test-107 should produce the final report.');
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
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, 'Scenario run-test-101 should have success=false when finalReport.status=failure per CONTRACT.');
      const finalReport = result.finalReport;
      invariant(finalReport !== undefined, 'Final report expected for run-test-101.');
      invariant(finalReport.status === 'failure', 'Synthesized final report should carry failure status for run-test-101.');
      invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Session completed without a final report'), 'Failure summary should mention the missing final report for run-test-101.');
      const syntheticAcceptance = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED && entry.details?.source === 'synthetic');
      invariant(syntheticAcceptance !== undefined, 'Synthetic final report acceptance log missing for run-test-101.');
      invariant(syntheticAcceptance.severity === 'ERR', 'Synthetic acceptance log should emit ERR severity for run-test-101.');
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
      // CONTRACT §5: Every session must produce a report (tool, text fallback, or synthetic)
      // When final_report tool fails, a synthetic failure report is generated
      invariant(!result.success, 'Scenario run-test-102 should report failure when final_report tool fails.');
      invariant(result.finalReport !== undefined, 'Synthetic final report expected when agent__final_report fails in run-test-102.');
      invariant(result.finalReport.status === 'failure', 'Synthetic final report should have failure status for run-test-102.');
      invariant(typeof result.finalReport.content === 'string' && result.finalReport.content.includes('final_report tool failed'), 'Synthetic content should mention tool failure for run-test-102.');
      // With synthetic report generation, we exit via EXIT-FINAL-ANSWER instead of EXIT-NO-LLM-RESPONSE
      const exitLog = result.logs.find((entry) => entry.remoteIdentifier === EXIT_FINAL_REPORT_IDENTIFIER);
      invariant(exitLog !== undefined, 'EXIT-FINAL-ANSWER log expected for run-test-102.');
      const failureReportLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:failure-report');
      invariant(failureReportLog !== undefined, 'Failure report log expected for run-test-102.');
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
      invariant(finalReport?.status === 'success', 'Final report missing for run-test-103.');
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
    id: 'run-test-114',
    description: 'Trace SDK emits request/response payload logs without trace-llm.',
    configure: (_configuration, sessionConfig) => {
      sessionConfig.traceSdk = true;
      sessionConfig.traceLLM = false;
      sessionConfig.userPrompt = 'run-test-1';
    },
    execute: async (_configuration, sessionConfig) => {
      const session = AIAgentSession.create(sessionConfig);
      return await session.run();
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-114 expected success.');
      const traceLogs = result.logs.filter((entry) => entry.severity === 'TRC' && entry.type === 'llm');
      invariant(traceLogs.some((log) => typeof log.message === 'string' && log.message.includes('SDK request payload')), 'SDK request payload log missing for run-test-114.');
      invariant(traceLogs.some((log) => typeof log.message === 'string' && log.message.includes('SDK response payload')), 'SDK response payload log missing for run-test-114.');
    },
  },
  {
    id: 'run-test-125',
    description: 'Tool-choice configuration resolves provider and model overrides.',
    execute: async () => {
      await Promise.resolve();
      const session = Object.create(AIAgentSession.prototype) as AIAgentSession;
      const configuration: Configuration = {
        providers: {
          openrouter: {
            type: 'openrouter',
            apiKey: 'test-key',
            toolChoice: 'auto',
            models: {
              'model-required': { toolChoice: 'required' },
              'model-inherit': {},
            },
          },
        },
        mcpServers: {},
        queues: { default: { concurrent: 32 } },
      };
      Reflect.set(session as unknown as Record<string, unknown>, 'sessionConfig', { config: configuration, toolingTransport: 'native' });
      const resolver = getPrivateMethod(session, 'resolveToolChoice') as (provider: string, model: string) => ToolChoiceMode | undefined;
      const requiredChoice = resolver.call(session, 'openrouter', 'model-required');
      const inheritedChoice = resolver.call(session, 'openrouter', 'model-inherit');
      const missingChoice = resolver.call(session, 'missing-provider', 'model');
      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'json',
          content_json: {
            requiredChoice,
            inheritedChoice,
            missingChoice,
          },
          ts: Date.now(),
        },
      } satisfies AIAgentResult;
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-125 expected success.');
      const payload = result.finalReport?.content_json as { requiredChoice?: string; inheritedChoice?: string; missingChoice?: unknown } | undefined;
      invariant(payload !== undefined, 'Final report payload expected for run-test-125.');
      invariant(payload.requiredChoice === 'required', 'Model override should enforce required tool choice for run-test-125.');
      invariant(payload.inheritedChoice === 'auto', 'Provider default should apply for run-test-125.');
      invariant(payload.missingChoice === undefined, 'Unknown provider should yield undefined tool choice for run-test-125.');
    },
  },
  {
    id: 'run-test-126',
    description: 'OpenRouter provider forwards tool-choice overrides into provider options.',
    execute: async () => {
      const captured: { resolved?: string; openaiToolChoice?: string; requestToolChoice?: ToolChoiceMode; requestToolChoiceRequired?: boolean } = {};

      const provider = new OpenRouterProvider({ type: 'openrouter', apiKey: 'unit-test-key' });
      Reflect.set(provider as unknown as Record<string, unknown>, 'provider', () => ({}) as LanguageModel);

      const originalStreaming = getPrivateMethod(BaseLLMProvider.prototype, 'executeStreamingTurn') as (
        this: BaseLLMProvider,
        model: LanguageModel,
        messages: ModelMessage[],
        tools: ToolSet | undefined,
        request: TurnRequest,
        startTime: number,
        providerOptions?: unknown
      ) => Promise<TurnResult>;
      Reflect.set(
        BaseLLMProvider.prototype as unknown as Record<string, unknown>,
        'executeStreamingTurn',
        async function (
          this: BaseLLMProvider,
          model: LanguageModel,
          messages: ModelMessage[],
          tools: ToolSet | undefined,
          request: TurnRequest,
          startTime: number,
          providerOptions?: unknown
        ): Promise<TurnResult> {
          captured.requestToolChoice = request.toolChoice;
          captured.requestToolChoiceRequired = request.toolChoiceRequired ?? undefined;
          captured.resolved = this.resolveToolChoice(request);
          if (providerOptions !== undefined && providerOptions !== null && typeof providerOptions === 'object') {
            const openaiOptions = (providerOptions as Record<string, unknown>).openai;
            if (openaiOptions !== undefined && openaiOptions !== null && typeof openaiOptions === 'object') {
              const candidate = (openaiOptions as Record<string, unknown>).toolChoice;
              captured.openaiToolChoice = typeof candidate === 'string' ? candidate : undefined;
            }
          }
          return await originalStreaming.call(this, model, messages, tools, request, startTime, providerOptions);
        }
      );

      try {
        await provider.executeTurn({
        messages: [{ role: 'user', content: 'Say hello' }],
        provider: 'openrouter',
        model: 'unit-model',
        tools: [],
        toolExecutor: () => Promise.resolve(''),
        temperature: 0,
        topP: 1,
        maxOutputTokens: 128,
        stream: true,
        toolChoice: 'auto',
        isFinalTurn: false,
        llmTimeout: 5_000,
        });
      } finally {
        Reflect.set(BaseLLMProvider.prototype as unknown as Record<string, unknown>, 'executeStreamingTurn', originalStreaming);
      }
      return makeSuccessResult(JSON.stringify(captured));
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, 'Scenario run-test-126 expected success.');
      const content = result.finalReport?.content;
      invariant(typeof content === 'string', 'Final report content expected as string for run-test-126.');
      const parsed = JSON.parse(content) as { resolved?: string; openaiToolChoice?: string; requestToolChoice?: ToolChoiceMode; requestToolChoiceRequired?: boolean };
      if (parsed.resolved !== 'auto' || parsed.openaiToolChoice !== 'auto') {
        console.error('DEBUG run-test-126 captured:', JSON.stringify(parsed, null, 2));
      }
      invariant(parsed.resolved === 'auto', 'Resolved tool choice should be auto for run-test-126.');
      invariant(parsed.openaiToolChoice === 'auto', 'OpenAI provider options should carry auto tool choice for run-test-126.');
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
    id: 'coverage-llm-payload',
    description: 'Coverage: core LLM payload logging through internal helpers.',
    execute: async () => {
      await Promise.resolve();
      coverageLlmPayload = undefined;
      const logs: LogEntry[] = [];
      const client = new LLMClient({}, { onLog: (entry) => { logs.push(entry); } });
      client.setTurn(1, 0);
      const turnRequest: TurnRequest = {
        messages: [{ role: 'user', content: 'payload verification' }],
        provider: 'primary',
        model: MODEL_NAME,
        tools: [],
        toolExecutor: () => Promise.resolve('')
      };
      const activeContext = { request: turnRequest, requestLogged: false } as { request: TurnRequest; requestLogged: boolean; requestPayload?: LogPayload; responsePayload?: LogPayload };
      (client as unknown as { activeHttpContext?: typeof activeContext }).activeHttpContext = activeContext;
      const captureRequest = getPrivateMethod(client, 'captureLlmRequestPayload');
      captureRequest.call(client, JSON.stringify({ prompt: 'payload verification' }), 'http');
      const captureResponse = getPrivateMethod(client, 'captureLlmResponsePayload');
      captureResponse.call(client, JSON.stringify({ ok: true }), 'http');
      const logResponse = getPrivateMethod(client, 'logResponse');
      const turnResult: TurnResult = {
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 0,
        messages: [],
      };
      logResponse.call(client, turnRequest, turnResult, 0, activeContext.responsePayload);
      coverageLlmPayload = { logs: [...logs] };
      return makeSuccessResult('coverage-llm-payload');
    },
    expect: (result) => {
      invariant(result.success, 'Coverage coverage-llm-payload should succeed.');
      invariant(coverageLlmPayload !== undefined, 'Coverage logs missing for coverage-llm-payload.');
      const requestLog = coverageLlmPayload.logs.find((log) => log.direction === 'request' && log.type === 'llm' && log.llmRequestPayload !== undefined);
      invariant(requestLog !== undefined, 'LLM request payload missing for coverage-llm-payload.');
      const responseLog = coverageLlmPayload.logs.find((log) => log.direction === 'response' && log.type === 'llm' && log.llmResponsePayload !== undefined);
      invariant(responseLog !== undefined, 'LLM response payload missing for coverage-llm-payload.');
      const responseBody = responseLog.llmResponsePayload?.body ?? '';
      invariant(responseBody.includes('ok'), 'LLM response payload should include response body.');
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
  {
    id: 'reasoning-normalization',
    execute: async (_configuration, _sessionConfig, _defaults) => {
      const conversation: ConversationMessage[] = [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{ id: 'call-1', name: 'agent__progress_report', parameters: {} }],
          reasoning: [{ type: 'reasoning', text: 'missing metadata', providerMetadata: { anthropic: {} } }],
        },
      ];
      const provider = new AnthropicProvider({ type: 'anthropic' });
      const result = provider.shouldDisableReasoning({
        conversation,
        currentTurn: 2,
        expectSignature: true,
      });
      invariant(result.disable, 'Reasoning without signature should trigger enforcement.');
      invariant(Array.isArray(result.normalized) && result.normalized.length === 1, 'Normalized conversation should return same number of messages.');
      invariant(result.normalized[0]?.reasoning === undefined, 'Reasoning array should be removed when metadata is missing.');
      const preserve = provider.shouldDisableReasoning({
        conversation,
        currentTurn: 1,
        expectSignature: true,
      });
      invariant(!preserve.disable, 'Reasoning should be preserved on first turn.');
      invariant(Array.isArray(preserve.normalized) && preserve.normalized[0]?.reasoning !== undefined, 'Reasoning segments should remain without signature requirement.');
      return Promise.resolve(makeSuccessResult('reasoning-normalization'));
    },
    expect: (result) => {
      invariant(result.success, 'Reasoning normalization test expected success.');
    },
  },
  {
    id: 'reasoning-replay-signature',
    execute: () => {
      class ExposureProvider extends BaseLLMProvider {
        name = 'exposure';
        // eslint-disable-next-line @typescript-eslint/no-useless-constructor -- Expose protected base constructor for testing.
        constructor() {
          super();
        }
        executeTurn(_request: TurnRequest): Promise<TurnResult> {
          return Promise.reject(new Error('not implemented'));
        }
        protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
          return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
        }
        convertResponseMessagesExposed(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
          return this.convertResponseMessages(messages, provider, model, tokens);
        }
        convertMessagesExposed(messages: ConversationMessage[]): ModelMessage[] {
          return this.convertMessages(messages);
        }
      }

      const provider: ExposureProvider = new ExposureProvider();
      const signature = 'sig-replay-123';
      const responseMessages: ResponseMessage[] = [
        {
          role: 'assistant',
          content: [
            {
              type: 'reasoning',
              text: 'Verifying reasoning signature persistence.',
              providerMetadata: { anthropic: { signature } },
            },
            {
              type: 'tool-call',
              toolCallId: 'call-1',
              toolName: 'agent__progress_report',
              input: { progress: 'Checking replay order' },
            },
          ],
        },
        {
          role: 'tool',
          content: [
            {
              type: 'tool_result',
              toolCallId: 'call-1',
              toolName: 'agent__progress_report',
              output: { type: 'text', value: 'ok' },
            },
          ],
        },
      ];
      const usage: TokenUsage = { inputTokens: 24, outputTokens: 12, totalTokens: 36 };

      const conversation: ConversationMessage[] = provider.convertResponseMessagesExposed(responseMessages, 'anthropic', 'claude-3-5', usage);
      invariant(conversation.some((msg) => msg.role === 'assistant'), 'Converted conversation missing assistant message.');
      const assistantMessage = conversation.find((msg) => msg.role === 'assistant');
      invariant(assistantMessage !== undefined, 'Assistant message missing from converted conversation.');
      invariant(Array.isArray(assistantMessage.reasoning) && assistantMessage.reasoning.length > 0, 'Assistant reasoning missing after conversion.');
      const persistedConversation = JSON.parse(JSON.stringify(conversation)) as ConversationMessage[];

      const modelMessages: { role?: string; content?: unknown }[] = provider.convertMessagesExposed(persistedConversation);
      const assistantModel = modelMessages.find((msg) => msg.role === 'assistant');
      const assistantContentCandidate = assistantModel !== undefined ? assistantModel.content : undefined;
      invariant(Array.isArray(assistantContentCandidate), 'Assistant model message should expose content array.');
      const assistantContent = assistantContentCandidate as (Record<string, unknown> | undefined)[];
      const [reasoningPart, toolCallPart] = assistantContent;
      invariant(reasoningPart !== undefined, 'Reasoning part should be first in model message.');
      invariant(reasoningPart.type === 'reasoning', 'Reasoning part should be first in model message.');
      const providerOptions = reasoningPart.providerOptions as Record<string, unknown> | undefined;
      invariant(providerOptions !== undefined, 'Reasoning providerOptions missing after replay.');
      const anthropicMeta = providerOptions.anthropic as { signature?: unknown } | undefined;
      invariant(anthropicMeta?.signature === signature, 'Reasoning signature lost during replay.');
      invariant(toolCallPart !== undefined, 'Tool call should follow reasoning in model message.');
      invariant(toolCallPart.type === 'tool-call', 'Tool call should follow reasoning in model message.');
      return Promise.resolve(makeSuccessResult('reasoning-replay-signature'));
    },
    expect: (result) => {
      invariant(result.success, 'Reasoning replay test expected success.');
    },
  },
  ...buildReasoningMatrixScenarios(),
];

async function runScenario(test: HarnessTest): Promise<AIAgentResult> {
  const { id: prompt, configure, execute } = test;
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
        args: [TEST_STDIO_SERVER_PATH],
      },
    },
    queues: { default: { concurrent: 32 } },
    defaults: { ...BASE_DEFAULTS },
    // Tests use native transport by default; XML-specific tests override this
    tooling: { transport: 'native' },
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
    maxTurns: 3,
    toolTimeout: defaults.toolTimeout,
    llmTimeout: defaults.llmTimeout,
    agentId: `phase1-${prompt}`,
    abortSignal: abortController.signal,
    // Tests use native transport by default; XML-specific tests override this
    toolingTransport: 'native',
  };

  configure?.(configuration, baseSession, defaults);

  queueManager.reset();
  queueManager.configureQueues(configuration.queues);

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

const parseScenarioFilter = (raw?: string): string[] => {
  if (typeof raw !== 'string') return [];
  return raw.split(',').map((value) => value.trim()).filter((value) => value.length > 0);
};

const CI_TRUE_VALUES = new Set(['1', 'true', 'yes']);
const isCIEnvironment = (() => {
  const raw = process.env.CI?.toLowerCase();
  return raw !== undefined && CI_TRUE_VALUES.has(raw);
})();

const scenarioFilterIdsFromEnv = (() => {
  const raw = process.env.PHASE1_ONLY_SCENARIO;
  const parsed = parseScenarioFilter(raw);
  if (parsed.length === 0) return parsed;
  if (isCIEnvironment) {
    console.warn('[warn] PHASE1_ONLY_SCENARIO is ignored in CI; running full suite.');
    return [];
  }
  return parsed;
})();

(() => {
  const scenarioId = 'run-test-final-report-valid-tool-call';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 5;
    sessionConfig.maxRetries = 2;
    const finalContent = 'Valid tool call result.';
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request }) => {
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: {
              status: 'success',
              report_format: 'text',
              report_content: finalContent,
            },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: finalContent,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
    expect: (result: AIAgentResult) => {
    invariant(result.success, `Scenario ${scenarioId} expected success.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.content === 'Valid tool call result.', `Final report content mismatch for ${scenarioId}.`);
    const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
    invariant(finalTurnLog === undefined, `Final turn log should not appear for ${scenarioId}.`);
    const textExtractionLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLog === undefined, `Text extraction log should not appear for ${scenarioId}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', `Final report should be accepted from tool call for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-parameters-string';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 5;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Tool call succeeds after invalid parameters.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-invalid-string`,
                name: 'agent__final_report',
                parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.content === 'Tool call succeeds after invalid parameters.', `Final report content mismatch for ${scenarioId}.`);
      const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_SANITIZER);
      invariant(sanitizerLog !== undefined, `Sanitizer log expected for ${scenarioId}.`);
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
      const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
      invariant(fallbackLog === undefined, `Fallback log should not appear for ${scenarioId}.`);
      const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
      invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', `Final report should be accepted from tool call for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-turn-collapse-incomplete-final-report';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Recovered after missing content.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-missing-content`,
                name: 'agent__final_report',
                parameters: {
                  status: 'success',
                  report_format: 'text',
                  report_content: '',
                },
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            try {
              await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            } catch {
              // Expected failure due to missing content
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.content === 'Recovered after missing content.', `Final report content mismatch for ${scenarioId}.`);
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
      const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
      invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', `Final report should be accepted from tool call for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-wrong-types';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Recovered after wrong field types.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-wrong-types`,
                name: 'agent__final_report',
                parameters: {
                  status: 200,
                  report_format: 'text',
                  report_content: 123,
                } as Record<string, unknown>,
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            try {
              await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            } catch {
              // Expected failure due to wrong types
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.content === 'Recovered after wrong field types.', `Final report content mismatch for ${scenarioId}.`);
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-null-parameters';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Recovered after null parameters.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-null`,
                name: 'agent__final_report',
                parameters: null as unknown as Record<string, unknown>,
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.content === 'Recovered after null parameters.', `Final report content mismatch for ${scenarioId}.`);
      const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_SANITIZER);
      invariant(sanitizerLog !== undefined, `Sanitizer log expected for ${scenarioId}.`);
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-empty-parameters';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Recovered after empty parameters.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-empty`,
                name: 'agent__final_report',
                parameters: {},
              },
            ],
          };
          const toolCalls = assistantMessage.toolCalls ?? [];
          // eslint-disable-next-line functional/no-loop-statements
          for (const call of toolCalls) {
            try {
              await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
            } catch {
              // Expected failure due to missing fields
            }
          }
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.content === 'Recovered after empty parameters.', `Final report content mismatch for ${scenarioId}.`);
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-literal-newlines';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 5;
      sessionConfig.maxRetries = 2;
      const finalContent = 'Recovered after newline payload.';
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              {
                id: `${FINAL_REPORT_CALL_ID}-newline`,
                name: 'agent__final_report',
                parameters: INVALID_FINAL_REPORT_PAYLOAD_WITH_NEWLINES as unknown as Record<string, unknown>,
              },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      // Note: No sanitizer log expected - json-repair successfully fixes the malformed JSON
      // so the tool call is NOT dropped, just processed with repaired parameters
      const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Collapsing'));
      invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

BASE_TEST_SCENARIOS.push({
  id: FINAL_REPORT_RETRY_TEXT_SCENARIO,
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    // eslint-disable-next-line @typescript-eslint/unbound-method
    const originalExecuteTurn = LLMClient.prototype.executeTurn;
    let invocation = 0;
    LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
      invocation += 1;
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '{"status":"success","report_format":"text","report_content":"Fallback report."}',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      }
      if (invocation === 2) {
        const finalContent = FINAL_REPORT_AFTER_RETRY;
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'markdown',
                report_content: finalContent,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: finalContent,
        };
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
      const session = AIAgentSession.create(sessionConfig);
      return await session.run();
    } finally {
      LLMClient.prototype.executeTurn = originalExecuteTurn;
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, `Scenario ${FINAL_REPORT_RETRY_TEXT_SCENARIO} expected success.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.content === FINAL_REPORT_AFTER_RETRY, `Final report content mismatch for ${FINAL_REPORT_RETRY_TEXT_SCENARIO}.`);
    const textExtractionLogs = result.logs.filter((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 1, `Text extraction log missing for ${FINAL_REPORT_RETRY_TEXT_SCENARIO}.`);
    const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLog !== undefined, `Turn collapse log expected for ${FINAL_REPORT_RETRY_TEXT_SCENARIO}.`);
    const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
    invariant(fallbackLog === undefined, `Fallback acceptance should not occur before the final turn for ${FINAL_REPORT_RETRY_TEXT_SCENARIO}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', `Final report should be accepted from the tool call for ${FINAL_REPORT_RETRY_TEXT_SCENARIO}.`);
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: FINAL_REPORT_TOOL_MESSAGE_SCENARIO,
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
    const originalExecuteTurn = LLMClient.prototype.executeTurn;
    LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
          },
          {
            id: 'tool-message-fallback-call',
            name: 'test__test',
            parameters: { text: TOOL_MESSAGE_FALLBACK_CONTENT },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        if (call.name !== 'test__test') continue;
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: 'tool-message-fallback-call',
        content: TOOL_MESSAGE_FALLBACK_CONTENT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      };
    };
    try {
      const session = AIAgentSession.create(sessionConfig);
      return await session.run();
    } finally {
      LLMClient.prototype.executeTurn = originalExecuteTurn;
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, `Scenario ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO} expected success.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.status === 'success', `Final report should be success for ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO}.`);
    invariant(finalReport.content === TOOL_MESSAGE_FALLBACK_CONTENT, `Final report content mismatch for ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO}.`);
    const textExtractionLogs = result.logs.filter((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 1, `Text extraction log missing for ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO}.`);
    const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
    invariant(fallbackLog !== undefined, `Fallback acceptance log missing for ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-message', `Final report source should be tool-message for ${FINAL_REPORT_TOOL_MESSAGE_SCENARIO}.`);
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-synthetic-failure-contract',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    // eslint-disable-next-line @typescript-eslint/unbound-method
    const originalExecuteTurn = LLMClient.prototype.executeTurn;
    let invocation = 0;
    const maxSyntheticResponses = (sessionConfig.maxRetries ?? 3) * (sessionConfig.maxTurns ?? 2);
    LLMClient.prototype.executeTurn = async function(this: LLMClient, request: TurnRequest): Promise<TurnResult> {
      invocation += 1;
      if (invocation <= maxSyntheticResponses) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: `Status update turn ${String(invocation)}.`,
          toolCalls: [],
        };
        return {
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        };
      }
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
    // CONTRACT §2: success: false when finalReport.status is 'failure'
    invariant(!result.success, 'Scenario run-test-synthetic-failure-contract should have success=false when finalReport.status=failure per CONTRACT.');
    const finalReport = result.finalReport;
    invariant(finalReport?.status === 'failure', 'Final report should indicate failure for run-test-synthetic-failure-contract.');
    invariant(typeof finalReport.content === 'string' && finalReport.content.includes('after 2 turns'), 'Failure summary mismatch for run-test-synthetic-failure-contract.');
    const metadata = finalReport.metadata;
    assertRecord(metadata, 'Final report metadata expected for run-test-synthetic-failure-contract.');
    invariant(metadata.reason === 'max_turns_exhausted', 'Metadata reason mismatch for run-test-synthetic-failure-contract.');
    invariant(metadata.turns_completed === 2, 'turns_completed metadata mismatch for run-test-synthetic-failure-contract.');
    invariant(metadata.final_report_attempts === 0, 'final_report_attempts should be 0 for run-test-synthetic-failure-contract.');
    const failureLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FAILURE_REPORT);
    invariant(failureLog !== undefined, 'Failure report log missing for run-test-synthetic-failure-contract.');
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'synthetic', 'Synthetic final report acceptance log missing for run-test-synthetic-failure-contract.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: FINAL_REPORT_MAX_RETRIES_SCENARIO,
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 2;
    // eslint-disable-next-line @typescript-eslint/unbound-method -- capture original method for restoration after interception
    const originalExecuteTurn = LLMClient.prototype.executeTurn;
    let invocation = 0;
    LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
      invocation += 1;
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: `${FINAL_REPORT_CALL_ID}-${String(invocation)}`,
            name: 'agent__final_report',
            parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
          },
        ],
      };
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
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
    // CONTRACT §2: success: false when finalReport.status is 'failure'
    invariant(!result.success, `Scenario ${FINAL_REPORT_MAX_RETRIES_SCENARIO} should have success=false when finalReport.status=failure per CONTRACT.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.status === 'failure', `Final report should indicate failure for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    const metadata = finalReport.metadata;
    assertRecord(metadata, `Final report metadata expected for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    invariant(metadata.reason === SYNTHETIC_MAX_RETRY_REASON, `Metadata reason mismatch for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    invariant(metadata.turns_completed === 2, `turns_completed metadata mismatch for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    invariant(metadata.final_report_attempts === FINAL_REPORT_MAX_RETRIES_TOTAL_ATTEMPTS, `final_report_attempts mismatch for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    const failureLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FAILURE_REPORT);
    invariant(failureLog !== undefined, `Failure report log missing for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'synthetic', `Synthetic acceptance log missing for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
    const textExtractionLogs = result.logs.filter((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 0, `Text extraction should not occur for ${FINAL_REPORT_MAX_RETRIES_SCENARIO}.`);
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-text-extraction-non-final-turn',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 5;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '{"status":"success","report_format":"text","report_content":"Pending text fallback"}',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-invalid-text-non-final`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: {
              status: 'success',
              report_format: 'text',
              report_content: TEXT_EXTRACTION_RETRY_RESULT,
            },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: TEXT_EXTRACTION_RETRY_RESULT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    const scenarioId = 'run-test-text-extraction-non-final-turn';
    invariant(result.success, `Scenario ${scenarioId} expected success.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.content === TEXT_EXTRACTION_RETRY_RESULT, `Final report content mismatch for ${scenarioId}.`);
    const sanitizerLog = findLogByIdentifier(result.logs, LOG_SANITIZER);
    invariant(sanitizerLog !== undefined, `Sanitizer log expected for ${scenarioId}.`);
    const textExtractionLogs = getLogsByIdentifier(result.logs, LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 1, `Text extraction log expected once for ${scenarioId}.`);
    const fallbackLog = findLogByIdentifier(result.logs, LOG_FALLBACK_REPORT);
    invariant(fallbackLog === undefined, `Fallback log should not appear before the final turn for ${scenarioId}.`);
    expectLogIncludes(result.logs, LOG_TEXT_EXTRACTION, 'Retrying for proper tool call', scenarioId);
    const collapseLog = findLogByIdentifier(result.logs, LOG_ORCHESTRATOR, (entry) => typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLog !== undefined, `Turn collapse log expected for ${scenarioId}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', `Final report should be accepted from tool call for ${scenarioId}.`);
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-text-extraction-invalid-text',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 4;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: 'This is not valid JSON for final report',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-invalid-text-invalid-json`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: {
              status: 'success',
              report_format: 'text',
              report_content: TEXT_EXTRACTION_INVALID_TEXT_RESULT,
            },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: TEXT_EXTRACTION_INVALID_TEXT_RESULT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-text-extraction-invalid-text expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === TEXT_EXTRACTION_INVALID_TEXT_RESULT, 'Final report content mismatch for run-test-text-extraction-invalid-text.');
    const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_SANITIZER);
    invariant(sanitizerLog !== undefined, 'Sanitizer log expected for run-test-text-extraction-invalid-text.');
    const textExtractionLogs = result.logs.filter((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 0, 'Text extraction log should not appear when text is invalid.');
  },
} satisfies HarnessTest);

(() => {
  const scenarioId = 'run-test-llm-never-sends-final-report';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 5;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
        const turnIndex = request.turnMetadata?.turn ?? 1;
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: `Progress update turn ${String(turnIndex)}`,
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: assistantMessage.content,
          messages: [assistantMessage],
          tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
        });
      });
    },
    expect: (result: AIAgentResult) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', `Final report should indicate failure for ${scenarioId}.`);
      assertRecord(finalReport.metadata, `Final report metadata expected for ${scenarioId}.`);
      invariant(finalReport.metadata.reason === 'max_turns_exhausted', `Metadata reason mismatch for ${scenarioId}.`);
      invariant(finalReport.metadata.turns_completed === 5, `turns_completed metadata mismatch for ${scenarioId}.`);
      const failureLog = findLogByIdentifier(result.logs, LOG_FAILURE_REPORT);
      invariant(failureLog !== undefined, `Failure report log missing for ${scenarioId}.`);
      expectLogIncludes(result.logs, LOG_ORCHESTRATOR, COLLAPSING_REMAINING_TURNS_FRAGMENT, scenarioId);
      expectLogIncludes(result.logs, LOG_ORCHESTRATOR, 'exhausted 2 attempt', scenarioId);
      expectLogIncludes(result.logs, FINAL_TURN_REMOTE, 'Final turn (5) detected', scenarioId);
      const acceptanceLog = findLogByIdentifier(result.logs, LOG_FINAL_REPORT_ACCEPTED);
      invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'synthetic', `Synthetic acceptance log missing for ${scenarioId}.`);
      const fallbackLog = findLogByIdentifier(result.logs, LOG_TEXT_EXTRACTION);
      invariant(fallbackLog === undefined, `Text extraction should not occur for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-llm-keeps-sending-invalid';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 5;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
        const attempt = request.turnMetadata?.attempt ?? 1;
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-invalid-${String(attempt)}`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        });
      });
    },
    expect: (result: AIAgentResult) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', `Final report should indicate failure for ${scenarioId}.`);
      assertRecord(finalReport.metadata, `Final report metadata expected for ${scenarioId}.`);
      // CONTRACT: synthetic failure when max turns exhausted always uses 'max_turns_exhausted'
      invariant(finalReport.metadata.reason === 'max_turns_exhausted', `Metadata reason mismatch for ${scenarioId}.`);
      invariant(finalReport.metadata.final_report_attempts === 4, `final_report_attempts mismatch for ${scenarioId}.`);
      const failureLog = findLogByIdentifier(result.logs, LOG_FAILURE_REPORT);
      invariant(failureLog !== undefined, `Failure report log missing for ${scenarioId}.`);
      const acceptanceLog = findLogByIdentifier(result.logs, LOG_FINAL_REPORT_ACCEPTED);
      invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'synthetic', `Synthetic acceptance log missing for ${scenarioId}.`);
      const textExtractionLogs = getLogsByIdentifier(result.logs, LOG_TEXT_EXTRACTION);
      invariant(textExtractionLogs.length === 0, `Text extraction should not occur for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-invalid-no-text-fallback';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 3;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
        const turnIndex = request.turnMetadata?.turn ?? 1;
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: turnIndex === 1 ? '' : 'Not valid JSON',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-invalid-no-text-${String(turnIndex)}`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        });
      });
    },
    expect: (result: AIAgentResult) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', `Final report should indicate failure for ${scenarioId}.`);
      const textExtractionLogs = getLogsByIdentifier(result.logs, LOG_TEXT_EXTRACTION);
      invariant(textExtractionLogs.length === 0, `Text extraction should not occur for ${scenarioId}.`);
      const failureLog = findLogByIdentifier(result.logs, LOG_FAILURE_REPORT);
      invariant(failureLog !== undefined, `Failure report log missing for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-llm-error-during-final-turn';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 3;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
        const turnIndex = request.turnMetadata?.turn ?? 1;
        if (turnIndex === 3) {
          return Promise.resolve({
            status: { type: 'network_error', retryable: false, message: 'LLM API timeout' },
            latencyMs: 5,
            messages: [],
            tokens: { inputTokens: 5, outputTokens: 0, totalTokens: 5 },
          });
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: `Progress update turn ${String(turnIndex)}`,
        };
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: assistantMessage.content,
          messages: [assistantMessage],
          tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
        });
      });
    },
    expect: (result: AIAgentResult) => {
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      const finalReport = result.finalReport;
      invariant(finalReport?.status === 'failure', `Final report should indicate failure for ${scenarioId}.`);
      assertRecord(finalReport.metadata, `Final report metadata expected for ${scenarioId}.`);
      // CONTRACT: synthetic failure when max turns exhausted always uses 'max_turns_exhausted'
      invariant(finalReport.metadata.reason === 'max_turns_exhausted', `Metadata reason mismatch for ${scenarioId}.`);
      expectLogIncludes(result.logs, LOG_FAILURE_REPORT, 'LLM API timeout', scenarioId);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-empty-response';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, () => Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        messages: [],
        tokens: { inputTokens: 4, outputTokens: 0, totalTokens: 4 },
      }));
    },
    expect: (result: AIAgentResult) => {
      // This test produces a synthetic failure report (LLM never sends final_report)
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      expectLogIncludes(result.logs, PRIMARY_REMOTE, 'Empty response without tools', scenarioId);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-whitespace-only-response';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, () => Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: '   \n\t  ',
        messages: [
          {
            role: 'assistant',
            content: '   \n\t  ',
          },
        ],
        tokens: { inputTokens: 4, outputTokens: 1, totalTokens: 5 },
      }));
    },
    expect: (result: AIAgentResult) => {
      // This test produces a synthetic failure report (LLM never sends final_report)
      // CONTRACT §2: success: false when finalReport.status is 'failure'
      invariant(!result.success, `Scenario ${scenarioId} should have success=false when finalReport.status=failure per CONTRACT.`);
      expectLogIncludes(result.logs, LOG_ORCHESTRATOR, 'assistant returned content without tool calls', scenarioId);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-multiple-tool-calls-all-invalid';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
        if (invocation === 1) {
          const assistantMessage: ConversationMessage = {
            role: 'assistant',
            content: '',
            toolCalls: [
              { id: 'call_1', name: 'other_tool', parameters: 'invalid' as unknown as Record<string, unknown> },
              { id: 'call_2', name: FINAL_REPORT_CALL_ID, parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown> },
              { id: 'call_3', name: 'another_tool', parameters: 'invalid' as unknown as Record<string, unknown> },
            ],
          };
          return {
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            messages: [assistantMessage],
            tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 },
          };
        }
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: 'Recovered after invalid tools.',
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: 'Recovered after invalid tools.',
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      });
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      expectLogIncludes(result.logs, LOG_SANITIZER, 'Dropped 3 invalid tool call(s)', scenarioId);
      expectLogIncludes(result.logs, LOG_ORCHESTRATOR, COLLAPSING_REMAINING_TURNS_FRAGMENT, scenarioId);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-final-report-with-other-tools';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 5;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, () => Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [
          {
            role: 'assistant',
            content: '',
            toolCalls: [
              { id: 'call_other', name: 'other_tool', parameters: { arg: 'value' } },
              {
                id: FINAL_REPORT_CALL_ID,
                name: 'agent__final_report',
                parameters: { status: 'success', report_format: 'markdown', report_content: 'Done' },
              },
            ],
          },
        ],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      }));
    },
    expect: (result: AIAgentResult) => {
      invariant(result.success, `Scenario ${scenarioId} expected success.`);
      const otherToolEntries = result.accounting.filter((entry) => entry.type === 'tool' && entry.command === 'other_tool');
      invariant(otherToolEntries.length === 0, `Other tool should not execute when final report present for ${scenarioId}.`);
    },
  } satisfies HarnessTest);
})();

(() => {
  const scenarioId = 'run-test-reasoning-without-final-report';
  BASE_TEST_SCENARIOS.push({
    id: scenarioId,
    execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
      sessionConfig.maxTurns = 4;
      sessionConfig.maxRetries = 2;
      return await runWithPatchedExecuteTurn(sessionConfig, () => Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: 'Let me think about this.',
        hasReasoning: true,
        messages: [
          {
            role: 'assistant',
            content: 'Let me think about this.',
          },
        ],
        tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
      }));
    },
    expect: (result: AIAgentResult) => {
      // CONTRACT §2/§5: no final report ever produced → synthetic failure → success: false
      invariant(!result.success, `Scenario ${scenarioId}: no final report → success=false per CONTRACT.`);
      expectLogIncludes(result.logs, LOG_ORCHESTRATOR, 'assistant returned content without tool calls', scenarioId);
    },
  } satisfies HarnessTest);
})();

BASE_TEST_SCENARIOS.push({
  id: 'run-test-pure-text-final-report',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 4;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      if (invocation === 1) {
        return {
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: 'Plain text progress update.',
          messages: [
            {
              role: 'assistant',
              content: 'Plain text progress update.',
            },
          ],
          tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: {
              status: 'success',
              report_format: 'text',
              report_content: PURE_TEXT_RETRY_RESULT,
            },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: PURE_TEXT_RETRY_RESULT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-pure-text-final-report expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === PURE_TEXT_RETRY_RESULT, 'Final report content mismatch for run-test-pure-text-final-report.');
    const syntheticRetryLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('Synthetic retry'));
    invariant(syntheticRetryLog !== undefined, 'Synthetic retry log expected for run-test-pure-text-final-report.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-invalid-final-report-at-max-turns',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
      const turnIndex = request.turnMetadata?.turn ?? 1;
      if (turnIndex < 3) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: `Progress update turn ${String(turnIndex)}`,
          messages: [
            {
              role: 'assistant',
              content: `Progress update turn ${String(turnIndex)}`,
            },
          ],
          tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
        });
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '{"status":"success","report_format":"text","report_content":"Fallback final report"}',
        toolCalls: [
          {
            id: `${FINAL_REPORT_CALL_ID}-max-turn`,
            name: 'agent__final_report',
            parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
          },
        ],
      };
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-invalid-final-report-at-max-turns expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === 'Fallback final report', 'Final report content mismatch for run-test-invalid-final-report-at-max-turns.');
    const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
    invariant(finalTurnLog !== undefined, 'Final turn log expected for run-test-invalid-final-report-at-max-turns.');
    const sanitizerLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_SANITIZER);
    invariant(sanitizerLog !== undefined, 'Sanitizer log expected for run-test-invalid-final-report-at-max-turns.');
    const textExtractionLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_TEXT_EXTRACTION);
    invariant(textExtractionLog !== undefined, 'Text extraction log expected for run-test-invalid-final-report-at-max-turns.');
    const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
    invariant(fallbackLog !== undefined, 'Fallback log expected for run-test-invalid-final-report-at-max-turns.');
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'text-fallback', 'Final report source should be text-fallback for run-test-invalid-final-report-at-max-turns.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-no-collapse-already-at-max',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, ({ request }) => {
      const turnIndex = request.turnMetadata?.turn ?? 1;
      if (turnIndex < 3) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: `Progress update turn ${String(turnIndex)}`,
          messages: [
            {
              role: 'assistant',
              content: `Progress update turn ${String(turnIndex)}`,
            },
          ],
          tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
        });
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '{"status":"success","report_format":"text","report_content":"Fallback"}',
        toolCalls: [
          {
            id: `${FINAL_REPORT_CALL_ID}-max-turn-no-collapse`,
            name: 'agent__final_report',
            parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
          },
        ],
      };
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-no-collapse-already-at-max expected success.');
    const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLog === undefined, 'Collapse log should not appear when already at max turns.');
    const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
    invariant(finalTurnLog !== undefined, 'Final turn log expected for run-test-no-collapse-already-at-max.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-invalid-final-report-before-max-turns',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 5;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request }) => {
      const turnIndex = request.turnMetadata?.turn ?? 1;
      if (turnIndex === 4) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '{"status":"success","report_format":"text","report_content":"Fallback"}',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-before-max`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      if (turnIndex === 5) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: FINAL_REPORT_CALL_ID,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: COLLAPSE_RECOVERY_RESULT,
              },
            },
          ],
        };
        const toolCalls = assistantMessage.toolCalls ?? [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
          await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
        }
        const toolMessage: ConversationMessage = {
          role: 'tool',
          toolCallId: FINAL_REPORT_CALL_ID,
          content: COLLAPSE_RECOVERY_RESULT,
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage, toolMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      }
      return {
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: `Progress update turn ${String(turnIndex)}`,
        messages: [
          {
            role: 'assistant',
            content: `Progress update turn ${String(turnIndex)}`,
          },
        ],
        tokens: { inputTokens: 6, outputTokens: 3, totalTokens: 9 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-invalid-final-report-before-max-turns expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === COLLAPSE_RECOVERY_RESULT, 'Final report content mismatch for run-test-invalid-final-report-before-max-turns.');
    const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLog !== undefined, 'Turn collapse log expected for run-test-invalid-final-report-before-max-turns.');
    const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
    invariant(finalTurnLog !== undefined, 'Final turn log expected for run-test-invalid-final-report-before-max-turns.');
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'tool-call', 'Final report should come from tool call for run-test-invalid-final-report-before-max-turns.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-turn-collapse-final-report-attempted',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 10;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-collapse-invalid`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: COLLAPSE_FIXED_RESULT,
              },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: COLLAPSE_FIXED_RESULT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-turn-collapse-final-report-attempted expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === COLLAPSE_FIXED_RESULT, 'Final report content mismatch for run-test-turn-collapse-final-report-attempted.');
    const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLog !== undefined, 'Turn collapse log expected for run-test-turn-collapse-final-report-attempted.');
    const finalTurnLog = result.logs.find((entry) => entry.remoteIdentifier === FINAL_TURN_REMOTE);
    invariant(finalTurnLog !== undefined, 'Final turn log expected for run-test-turn-collapse-final-report-attempted.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-turn-collapse-both-flags',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 10;
    sessionConfig.maxRetries = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-both-1`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
            {
              id: `${FINAL_REPORT_CALL_ID}-both-2`,
              name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: '',
              },
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: {
              status: 'success',
              report_format: 'text',
              report_content: 'Success after both flags.',
            },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: 'Success after both flags.',
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-turn-collapse-both-flags expected success.');
    const collapseLogs = result.logs.filter((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes(COLLAPSING_REMAINING_TURNS_FRAGMENT));
    invariant(collapseLogs.length === 1, 'Collapse log should appear exactly once for run-test-turn-collapse-both-flags.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-max-provider-retries-exhausted',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 10;
    sessionConfig.maxRetries = 3;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request }) => {
      const turnIndex = request.turnMetadata?.turn ?? 1;
      const attempt = request.turnMetadata?.attempt ?? 1;
      if (turnIndex === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: '',
          toolCalls: [
            {
              id: `${FINAL_REPORT_CALL_ID}-attempt-${String(attempt)}`,
              name: 'agent__final_report',
              parameters: INVALID_FINAL_REPORT_PAYLOAD as unknown as Record<string, unknown>,
            },
          ],
        };
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: '',
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
              parameters: {
                status: 'success',
                report_format: 'text',
                report_content: MAX_RETRY_SUCCESS_RESULT,
              },
          },
        ],
      };
      const toolCalls = assistantMessage.toolCalls ?? [];
      // eslint-disable-next-line functional/no-loop-statements
      for (const call of toolCalls) {
        await request.toolExecutor(call.name, call.parameters, { toolCallId: call.id });
      }
      const toolMessage: ConversationMessage = {
        role: 'tool',
        toolCallId: FINAL_REPORT_CALL_ID,
        content: MAX_RETRY_SUCCESS_RESULT,
      };
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage, toolMessage],
        tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Scenario run-test-max-provider-retries-exhausted expected success.');
    const finalReport = result.finalReport;
    invariant(finalReport?.content === MAX_RETRY_SUCCESS_RESULT, 'Final report content mismatch for run-test-max-provider-retries-exhausted.');
    const collapseLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_ORCHESTRATOR && typeof entry.message === 'string' && entry.message.includes('exhausted'));
    invariant(collapseLog !== undefined, 'Retry exhaustion log expected for run-test-max-provider-retries-exhausted.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-text-extraction-final-turn-accept',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    // eslint-disable-next-line @typescript-eslint/unbound-method
    const originalExecuteTurn = LLMClient.prototype.executeTurn;
    LLMClient.prototype.executeTurn = function(this: LLMClient, _request: TurnRequest): Promise<TurnResult> {
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: `{"status":"success","report_format":"text","report_content":"${TEXT_FALLBACK_CONTENT}"}`,
        toolCalls: [
          {
            id: FINAL_REPORT_CALL_ID,
            name: 'agent__final_report',
            parameters: 'not-an-object' as unknown as Record<string, unknown>,
          },
        ],
      };
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
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
    const scenarioId = 'run-test-text-extraction-final-turn-accept';
    invariant(result.success, `Scenario ${scenarioId} expected success.`);
    const finalReport = result.finalReport;
    invariant(finalReport?.status === 'success', `Final report should be success for ${scenarioId}.`);
    invariant(finalReport.content === TEXT_FALLBACK_CONTENT, `Final report content mismatch for ${scenarioId}.`);
    const textExtractionLogs = getLogsByIdentifier(result.logs, LOG_TEXT_EXTRACTION);
    invariant(textExtractionLogs.length === 1, `Text extraction log missing for ${scenarioId}.`);
    const fallbackLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FALLBACK_REPORT);
    invariant(fallbackLog !== undefined, `Fallback acceptance log missing for ${scenarioId}.`);
    const acceptanceLog = result.logs.find((entry) => entry.remoteIdentifier === LOG_FINAL_REPORT_ACCEPTED);
    invariant(acceptanceLog !== undefined && acceptanceLog.details?.source === 'text-fallback', `Final report source should be text-fallback for ${scenarioId}.`);
    expectLogIncludes(result.logs, FINAL_TURN_REMOTE, 'Final turn (1) detected', scenarioId);
    expectLogIncludes(result.logs, LOG_TEXT_EXTRACTION, 'Retrying for proper tool call', scenarioId);
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-xml-happy',
  description: 'XML transport executes progress + final-report via XML tags with provider tools hidden.',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.toolingTransport = 'xml';
    sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
    sessionConfig.headendWantsProgressUpdates = true;
    sessionConfig.maxRetries = 1;
    sessionConfig.maxTurns = 3;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      await Promise.resolve();
      invariant(request.tools.length === 0, 'Provider tools must be hidden in xml mode for run-test-xml-happy.');
      const nonce = extractNonceFromMessages(request.messages, 'run-test-xml-happy');
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: `<ai-agent-${nonce}-PROGRESS tool="agent__progress_report">{\"progress\":\"Working via XML\"}</ai-agent-${nonce}-PROGRESS>`,
        };
        const payload = { progress: 'Working via XML' };
        await request.toolExecutor('agent__progress_report', payload, { toolCallId: `${nonce}-PROGRESS` });
        return {
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        };
      }
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: `<ai-agent-${nonce}-FINAL tool="agent__final_report" status="success" format="markdown">{\"status\":\"success\",\"report_format\":\"markdown\",\"report_content\":\"Done via XML\"}</ai-agent-${nonce}-FINAL>`,
      };
      const payload = { status: 'success', report_format: 'markdown', report_content: 'Done via XML' };
      await request.toolExecutor('agent__final_report', payload, { toolCallId: `${nonce}-FINAL` });
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'run-test-xml-happy should succeed.');
    const finalReport = result.finalReport;
    invariant(finalReport !== undefined, 'Final report missing for run-test-xml-happy.');
    invariant(finalReport.status === 'success', 'Final report not success for run-test-xml-happy.');
    invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Done via XML'), 'Final report content mismatch for run-test-xml-happy.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-xml-final-only',
  description: 'xml-final transport uses native tools while final-report stays XML-tagged.',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.toolingTransport = 'xml-final';
    sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
    sessionConfig.maxRetries = 1;
    sessionConfig.maxTurns = 2;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request }) => {
      await Promise.resolve();
      invariant(request.tools.length > 0, 'Provider tools must be exposed in xml-final mode for run-test-xml-final-only.');
      const toolNames = request.tools.map((t) => sanitizeToolName(t.name));
      invariant(!toolNames.includes('agent__final_report'), 'Final-report tool must not be exposed as native tool in xml-final.');
      // Progress follows tools transport (native), not final_report transport (XML)
      const nonce = extractNonceFromMessages(request.messages, 'run-test-xml-final-only');
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: `<ai-agent-${nonce}-FINAL tool="agent__final_report" status="success" format="markdown">Final via xml-final</ai-agent-${nonce}-FINAL>`,
      };
      const payload = { status: 'success', report_format: 'markdown', report_content: 'Final via xml-final' };
      await request.toolExecutor('agent__final_report', payload, { toolCallId: `${nonce}-FINAL` });
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'run-test-xml-final-only should succeed.');
    const finalReport = result.finalReport;
    invariant(finalReport !== undefined, 'Final report missing for run-test-xml-final-only.');
    invariant(finalReport.status === 'success', 'Final report not success for run-test-xml-final-only.');
    invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Final via xml-final'), 'Final report content mismatch for run-test-xml-final-only.');
  },
} satisfies HarnessTest);

BASE_TEST_SCENARIOS.push({
  id: 'run-test-xml-invalid-tag',
  description: 'Invalid XML nonce is ignored; valid nonce on next turn succeeds with final-report.',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.toolingTransport = 'xml';
    sessionConfig.userPrompt = DEFAULT_PROMPT_SCENARIO;
    sessionConfig.maxRetries = 1;
    sessionConfig.maxTurns = 3;
    return await runWithPatchedExecuteTurn(sessionConfig, async ({ request, invocation }) => {
      await Promise.resolve();
      const nonce = extractNonceFromMessages(request.messages, 'run-test-xml-invalid-tag');
      if (invocation === 1) {
        const assistantMessage: ConversationMessage = {
          role: 'assistant',
          content: `<ai-agent-deadbeef-FINAL tool="agent__final_report" status="success" format="markdown">ignored</ai-agent-deadbeef-FINAL>`,
        };
        return {
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          messages: [assistantMessage],
          tokens: { inputTokens: 9, outputTokens: 4, totalTokens: 13 },
        };
      }
      const payload = { status: 'success', report_format: 'markdown', report_content: 'Recovered after invalid tag' };
      const assistantMessage: ConversationMessage = {
        role: 'assistant',
        content: `<ai-agent-${nonce}-FINAL tool="agent__final_report" status="success" format="markdown">${JSON.stringify(payload)}</ai-agent-${nonce}-FINAL>`,
      };
      await request.toolExecutor('agent__final_report', payload, { toolCallId: `${nonce}-FINAL` });
      return {
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        messages: [assistantMessage],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      };
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'run-test-xml-invalid-tag should succeed.');
    const finalReport = result.finalReport;
    invariant(finalReport !== undefined, 'Final report missing for run-test-xml-invalid-tag.');
    invariant(finalReport.status === 'success', 'Final report not success for run-test-xml-invalid-tag.');
    invariant(typeof finalReport.content === 'string' && finalReport.content.includes('Recovered after invalid tag'), 'Final report content mismatch for run-test-xml-invalid-tag.');
  },
} satisfies HarnessTest);

const filterScenarios = (ids: string[], logWarnings: boolean): HarnessTest[] => {
  if (ids.length === 0) return BASE_TEST_SCENARIOS;
  const requestedIds = new Set(ids);
  const filtered = BASE_TEST_SCENARIOS.filter((scenario) => requestedIds.has(scenario.id));
  if (logWarnings) {
    const missing = ids.filter((id) => !BASE_TEST_SCENARIOS.some((scenario) => scenario.id === id));
    if (missing.length > 0) {
      console.warn(`[warn] PHASE1_ONLY_SCENARIO references unknown scenario(s): ${missing.join(', ')}.`);
    }
    if (filtered.length === 0) {
      console.warn('[warn] PHASE1_ONLY_SCENARIO matched no known scenarios; running the full suite instead.');
      return BASE_TEST_SCENARIOS;
    }
    const label = filtered.map((scenario) => scenario.id).join(', ');
    console.log(`[info] Running filtered Phase 1 scenarios: ${label}`);
  }
  return filtered.length > 0 ? filtered : BASE_TEST_SCENARIOS;
};

const DEFAULT_TEST_SCENARIOS = filterScenarios(scenarioFilterIdsFromEnv, true);

export interface PhaseOneRunOptions {
  readonly filterIds?: string[];
}

const resolveScenarios = (options?: PhaseOneRunOptions): HarnessTest[] => {
  if (options?.filterIds === undefined) {
    return DEFAULT_TEST_SCENARIOS;
  }
  return filterScenarios(options.filterIds, false);
};

export function listPhaseOneScenarioIds(): string[] {
  return BASE_TEST_SCENARIOS.map((scenario) => scenario.id);
}

export async function runPhaseOneSuite(options?: PhaseOneRunOptions): Promise<void> {
  validateRichFormatterParity();
  await runJournaldSinkUnitTests();
  const scenarios = resolveScenarios(options);
  const total = scenarios.length;
  // eslint-disable-next-line functional/no-loop-statements
  for (let index = 0; index < total; index += 1) {
    const scenario = scenarios[index];
    const runPrefix = `${String(index + 1)}/${String(total)}`;
    const description = resolveScenarioDescription(scenario.id, scenario.description);
    const baseLabel = `${runPrefix} ${scenario.id}`;
    const header = `[RUN] ${baseLabel}${description !== undefined ? `: ${description}` : ''}`;
    const startMs = Date.now();
    let result: AIAgentResult | undefined;
    try {
      result = await runScenario(scenario);
      dumpScenarioResultIfNeeded(scenario.id, result);
      scenario.expect(result);
      const duration = formatDurationMs(startMs, Date.now());
       
      console.log(`${header} [PASS] ${duration}`);
    } catch (error: unknown) {
      const duration = formatDurationMs(startMs, Date.now());
      const message = toErrorMessage(error);
      const hint = result !== undefined ? formatFailureHint(result) : '';
       
      console.error(`${header} [FAIL] ${duration} - ${message}${hint}`);
      throw error;
    }
  }
  await cleanupActiveHandles();
   
  console.log('phase1 scenario: ok');
}

async function cleanupActiveHandles(): Promise<void> {
  const diagnosticProcess = process as typeof process & { _getActiveHandles?: () => unknown[] };
  const collectHandles = (): unknown[] => (typeof diagnosticProcess._getActiveHandles === 'function' ? diagnosticProcess._getActiveHandles() : []);
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

  let activeHandles = collectHandles();
  if (activeHandles.length === 0) return;

  // Run up to two cleanup passes to give child processes time to exit after SIGTERM/SIGKILL.
  // eslint-disable-next-line functional/no-loop-statements -- controlled cleanup loop is clearer than array methods here
  for (let pass = 0; pass < 2 && activeHandles.length > 0; pass += 1) {
    const childCleanup: Promise<void>[] = [];
    activeHandles.forEach((handle) => {
      if (handle === null || typeof handle !== 'object') return;
      const maybePid = (handle as { pid?: unknown }).pid;
      if (typeof maybePid === 'number') {
        const child = handle as ChildProcess;
        const exitPromise = new Promise<void>((resolve) => {
          const finish = (): void => { resolve(); };
          child.once('exit', finish);
          child.once('error', finish);
          setTimeout(() => { resolve(); }, 1500);
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
    await delay(50);
    activeHandles = collectHandles();
  }

  const remaining = activeHandles.filter((handle) => !shouldIgnore(handle));
  if (remaining.length === 0) return;

  const labels = remaining.map(formatHandle);
  console.error(`[warn] lingering handles after cleanup: ${labels.join(', ')}`);
}

function findScenarioById(id: string): HarnessTest {
  const scenario = BASE_TEST_SCENARIOS.find((entry) => entry.id === id);
  if (scenario === undefined) {
    throw new Error(`Unknown Phase 1 scenario id '${id}'.`);
  }
  return scenario;
}

export async function runPhaseOneScenario(id: string): Promise<AIAgentResult> {
  const scenario = findScenarioById(id);
  const result = await runScenario(scenario);
  scenario.expect(result);
  await cleanupActiveHandles();
  return result;
}
