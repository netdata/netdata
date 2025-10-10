import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { WebSocketServer } from 'ws';

import type { ResponseMessage } from '../llm-providers/base.js';
import type { ToolsOrchestrator } from '../tools/tools.js';
import type { AIAgentResult, AIAgentSessionConfig, AccountingEntry, Configuration, ConversationMessage, LogEntry, MCPTool, TokenUsage, TurnRequest, TurnResult, TurnStatus } from '../types.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import type { ModelMessage, ToolSet } from 'ai';
import type { AddressInfo } from 'node:net';

import { AIAgentSession } from '../ai-agent.js';
import { LLMClient } from '../llm-client.js';
import { BaseLLMProvider } from '../llm-providers/base.js';
import { InternalToolProvider } from '../tools/internal-provider.js';
import { sanitizeToolName } from '../utils.js';
import { createWebSocketTransport } from '../websocket-transport.js';

import { getScenario } from './fixtures/test-llm-scenarios.js';
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const MODEL_NAME = 'deterministic-model';
const PRIMARY_PROVIDER = 'primary';
const SECONDARY_PROVIDER = 'secondary';
const SUBAGENT_TOOL = 'agent__pricing-subagent';
const BASE_DEFAULTS = {
  stream: false,
  maxToolTurns: 3,
  maxRetries: 2,
  llmTimeout: 10_000,
  toolTimeout: 10_000,
} as const;
const TMP_PREFIX = 'ai-agent-phase1-';

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
    };
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
        finalize(() => { reject(error instanceof Error ? error : new Error(String(error))); });
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
const CONCURRENCY_TIMEOUT_ARGUMENT = 'trigger-timeout';
const CONCURRENCY_SECOND_ARGUMENT = 'concurrency-second';
const THROW_FAILURE_MESSAGE = 'Simulated provider throw for coverage.';
const BATCH_PROGRESS_RESPONSE = 'batch-progress-follow-up';
const BATCH_STRING_RESULT = 'batch-string-mode';
const TRACEABLE_PROVIDER_URL = 'https://openrouter.ai/api/v1';

const GITHUB_SERVER_PATH = path.resolve(__dirname, 'mcp/github-stdio-server.js');

const SLACK_OUTPUT_FORMAT = 'slack-block-kit' as const;

const DEFAULT_SCENARIO_TIMEOUT_MS = (() => {
  const raw = process.env.PHASE1_SCENARIO_TIMEOUT_MS;
  if (raw === undefined) return 10_000;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 10_000;
})();

const RUN_LOG_DECIMAL_PRECISION = 2;

function makeTempDir(label: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}${label}-`));
}

let runTest16Paths: { sessionsDir: string; billingFile: string } | undefined;
let runTest20Paths: { baseDir: string; blockerFile: string; billingFile: string } | undefined;
let runTest54State: { received: JSONRPCMessage[]; serverPayloads: string[]; errors: string[] } | undefined;

function invariant(condition: boolean, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function assertRecord(value: unknown, message: string): asserts value is Record<string, unknown> {
  invariant(value !== null && typeof value === 'object' && !Array.isArray(value), message);
}

function isTurnStatus(value: unknown): value is TurnStatus {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const maybeType = (value as { type?: unknown }).type;
  return typeof maybeType === 'string';
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

const TEST_SCENARIOS: HarnessTest[] = [
  {
    id: 'run-test-1',
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
      invariant(toolMessages.some((message) => message.content.includes('[TRUNCATED]')), 'Truncation notice expected in run-test-8.');
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
      const sessionsDir = path.join(baseDir, 'sessions');
      const billingFile = path.join(baseDir, 'billing.jsonl');
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
      sessionConfig.subAgentPaths = [SUBAGENT_PRICING_PROMPT];
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
      const billingFile = path.join(blockerFile, 'billing.jsonl');
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
      const rateLimitLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Rate limited'));
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
      sessionConfig.subAgentPaths = [SUBAGENT_PRICING_SUCCESS_PROMPT];
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
      const retryLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:orchestrator' && typeof entry.message === 'string' && entry.message.includes('Final report retry detected'));
      invariant(retryLog !== undefined, 'Retry log expected for run-test-32.');
    },
  },
  {
    id: 'run-test-33',
    expect: (result) => {
      invariant(!result.success, 'Scenario run-test-33 should fail after exhausting synthetic retries.');
      invariant(typeof result.error === 'string' && result.error.includes('content_without_tools_or_final'), 'Expected synthetic retry exhaustion error for run-test-33.');
      const syntheticLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Synthetic retry: assistant returned content without tool calls and without final_report.'));
      invariant(syntheticLog !== undefined, 'Synthetic retry warning expected for run-test-33.');
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
      const rateLimitLog = result.logs.find((entry) => typeof entry.message === 'string' && entry.message.includes('Rate limited; suggested wait'));
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
        throw new Error(`Failed to parse GitHub tool response: ${error instanceof Error ? error.message : String(error)}`);
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
      if (address === null || typeof address === 'string' || typeof (address as AddressInfo).port !== 'number') {
        await new Promise<void>((resolve) => { server.close(() => { resolve(); }); });
        throw new Error('Unable to determine WebSocket server port for run-test-54.');
      }
      const port = (address as AddressInfo).port;
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
          errors.push(error instanceof Error ? error.message : String(error));
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
      const OPENROUTER_RESPONSES_URL = 'https://openrouter.ai/api/v1/responses';
      const OPENROUTER_DEBUG_SSE_URL = 'https://openrouter.ai/api/v1/debug-sse';
      const OPENROUTER_BINARY_URL = 'https://openrouter.ai/api/v1/binary';
      const CONTENT_TYPE_JSON = 'application/json';
      const CONTENT_TYPE_EVENT_STREAM = 'text/event-stream';
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
        finalData.afterJson = client.getLastActualRouting();
        finalData.costs = client.getLastCostInfo();
        await tracedFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer SSE', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        finalData.afterSse = client.getLastActualRouting();
        finalData.costs = client.getLastCostInfo();
        await tracedFetch('https://anthropic.mock/v1/messages', {
          method: 'POST',
          headers: { 'Content-Type': CONTENT_TYPE_JSON },
          body: JSON.stringify({ prompt: 'cache' }),
        });
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
        await coverageFetch(OPENROUTER_BINARY_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer BINARY', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        await coverageFetch(OPENROUTER_RESPONSES_URL, {
          method: 'POST',
          headers: { Authorization: 'Bearer JSONFAIL', 'Content-Type': CONTENT_TYPE_JSON },
          body: requestBody,
        });
        try {
          await tracedFetch(OPENROUTER_RESPONSES_URL, {
            method: 'POST',
            headers: { Authorization: 'Bearer FAIL', 'Content-Type': CONTENT_TYPE_JSON },
            body: requestBody,
          });
        } catch (error: unknown) {
          fetchError = error instanceof Error ? error : new Error(String(error));
        }


        const successResult: TurnResult = {
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          response: 'ack',
          tokens: { inputTokens: 200, outputTokens: 50, cacheReadInputTokens: 0, cacheWriteInputTokens: 42, totalTokens: 250 },
          latencyMs: 64,
          messages: [],
        };
        const clientState = client as unknown as { lastCostUsd?: number; lastUpstreamCostUsd?: number };
        clientState.lastCostUsd = 0.01234;
        clientState.lastUpstreamCostUsd = 0.006;
        client.setTurn(2, 0);
        (client as unknown as { logResponse: (req: TurnRequest, res: TurnResult, latencyMs: number) => void }).logResponse(request, successResult, 64);
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
      const routingAfterSse = report.routingAfterSse;
      assertRecord(routingAfterSse, 'Routing metadata expected for run-test-55.');
      invariant(routingAfterSse.provider === 'mistral', 'Routing metadata expected for run-test-55.');
    },
  },
  {
    id: 'run-test-56',
    description: 'Base LLM provider utilities and error mapping.',
    execute: async () => {
      class MockProvider extends BaseLLMProvider {
        public name = 'mock';
        public constructor(options?: { formatPolicy?: { allowed?: string[]; denied?: string[] } }) {
          super(options);
        }
        public executeTurn = (_request: TurnRequest): Promise<TurnResult> =>
          Promise.resolve(this.createFailureResult({ type: 'invalid_response', message: 'not implemented' }, 0));
        public sanitizeSchema = (schema: Record<string, unknown>): Record<string, unknown> =>
          (this as unknown as { sanitizeStringFormatsInSchema: (node: Record<string, unknown>) => Record<string, unknown> }).sanitizeStringFormatsInSchema(schema);
        public drainStream = (stream: unknown): Promise<string> =>
          (this as unknown as { drainTextStream: (s: unknown) => Promise<string> }).drainTextStream(stream);
        public convertResponseMessages = (
          messages: ResponseMessage[],
          provider: string,
          model: string,
          tokens: TokenUsage,
        ): ConversationMessage[] => this.convertResponseMessagesGeneric(messages, provider, model, tokens);
        public getTokenUsage = (usage: Record<string, unknown> | undefined): TokenUsage => this.extractTokenUsage(usage);
        public convertToolsPublic = (
          tools: MCPTool[],
          executor: (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }) => Promise<string>,
        ): ToolSet => this.convertTools(tools, executor);
        public filterToolsPublic = (tools: MCPTool[], isFinalTurn: boolean): MCPTool[] => this.filterToolsForFinalTurn(tools, isFinalTurn);
        public buildFinalMessages = (messages: ModelMessage[], isFinalTurn: boolean): ModelMessage[] => this.buildFinalTurnMessages(messages, isFinalTurn);
        public createTimeout = (timeoutMs: number) => this.createTimeoutController(timeoutMs);
        public createFailure = (status: TurnStatus, latencyMs: number): TurnResult => this.createFailureResult(status, latencyMs);
      }

      const provider = new MockProvider({ formatPolicy: { allowed: ['email'], denied: ['uuid'] } });
      const statusResults: TurnStatus[] = [];

      const originalDebug = process.env.DEBUG;
      const originalConsoleError = console.error;
      const debugLogs: string[] = [];
      console.error = (...args: unknown[]) => {
        debugLogs.push(args.map((value) => (typeof value === 'string' ? value : JSON.stringify(value))).join(' '));
      };
      process.env.DEBUG = 'true';

      const errorInputs: readonly Record<string, unknown>[] = [
        { statusCode: 429, headers: { 'retry-after': '2' }, message: 'Rate limit reached' },
        { statusCode: 401, message: 'Authentication failed' },
        { statusCode: 402, message: 'Quota exceeded' },
        { statusCode: 400, message: 'Invalid model request' },
        { name: 'TimeoutError', message: 'Request timeout' },
        { statusCode: 503, message: 'Network issue' },
        { message: 'Generic failure' },
        {
          statusCode: 400,
          responseBody: JSON.stringify({ error: { metadata: { raw: JSON.stringify({ error: { message: 'Nested provider error', status: 'MODEL_UNAVAILABLE' } }) }, message: 'Outer error' } }),
          message: 'Wrapper error',
        },
      ];
      errorInputs.forEach((input) => {
        statusResults.push(provider.mapError(input));
      });

      process.env.DEBUG = originalDebug;
      console.error = originalConsoleError;

      const tokenUsage = provider.getTokenUsage({
        inputTokens: '120',
        output_tokens: 45,
        cached_tokens: 10,
        cacheCreation: { ephemeral_5m_input_tokens: 7 },
      });

      const sanitizedSchema = provider.sanitizeSchema({
        type: ['string', 'null'],
        format: 'uuid',
        properties: {
          email: { type: 'string', format: 'email' },
        },
      });

      const toolSet = provider.convertToolsPublic(
        [
          {
            name: 'external_tool',
            description: 'Example tool',
            inputSchema: { type: 'object', properties: { id: { type: 'string', format: 'uuid' } } },
          },
        ],
        (_toolName: string, parameters: Record<string, unknown>) => Promise.resolve(JSON.stringify(parameters))
      );

      const filteredTools = provider.filterToolsPublic(
        [
          { name: 'agent__final_report', description: 'final', inputSchema: { type: 'object' } },
          { name: 'other_tool', description: 'other', inputSchema: { type: 'object' } },
        ],
        true,
      );

      const finalMessages = provider.buildFinalMessages([{ role: 'assistant', content: 'summary' } as unknown as ModelMessage], true);

      const timeoutController = provider.createTimeout(10);
      timeoutController.resetIdle();
      await new Promise((resolve) => setTimeout(resolve, 20));
      const timeoutAborted = timeoutController.controller.signal.aborted;

      const textStreamIterator = async function* () {
        yield await Promise.resolve('Hello');
        yield await Promise.resolve(' World');
      };
      const textStream = { textStream: { [Symbol.asyncIterator]: textStreamIterator } };
      const drainedText = await provider.drainStream(textStream);

      const failureResult = provider.createFailure({ type: 'network_error', message: 'downstream', retryable: true }, 25);

      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'json',
          content_json: {
            statusResults,
            tokenUsage,
            sanitizedSchema,
            toolNames: Object.keys(toolSet),
            filteredTools: filteredTools.map((tool) => tool.name),
            finalMessagesCount: finalMessages.length,
            timeoutAborted,
            drainedText,
            failureStatus: failureResult.status.type,
            debugLogs,
          },
          ts: Date.now(),
        },
      };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-56 should succeed.');
      const report = result.finalReport?.content_json;
      assertRecord(report, 'Report expected for run-test-56.');
      const statusResults = report.statusResults;
      invariant(Array.isArray(statusResults) && statusResults.length >= 7 && statusResults.every(isTurnStatus), 'All error branches should be exercised for run-test-56.');
      const types = statusResults.map((status) => status.type);
      invariant(types.includes('rate_limit') && types.includes('auth_error') && types.includes('quota_exceeded') && types.includes('model_error') && types.includes('timeout') && types.includes('network_error'), 'Expected mapped error types for run-test-56.');
      const tokenUsage = report.tokenUsage;
      assertRecord(tokenUsage, 'Token usage extraction should parse numeric strings for run-test-56.');
      invariant(tokenUsage.inputTokens === 120 && tokenUsage.outputTokens === 45, 'Token usage extraction should parse numeric strings for run-test-56.');
      const sanitizedSchema = report.sanitizedSchema;
      assertRecord(sanitizedSchema, 'UUID format should be stripped for run-test-56.');
      invariant(sanitizedSchema.format === undefined, 'UUID format should be stripped for run-test-56.');
      const filtered = report.filteredTools;
      invariant(Array.isArray(filtered) && filtered.length === 1 && filtered[0] === 'agent__final_report', 'Only final report tool should remain on final turn for run-test-56.');
      invariant(report.finalMessagesCount === 2, 'Final turn message should append instructions for run-test-56.');
      invariant(report.timeoutAborted === true, 'Timeout controller should abort after idle period for run-test-56.');
      invariant(report.drainedText === 'Hello World', 'Text stream should be drained correctly for run-test-56.');
      invariant(typeof report.failureStatus === 'string' && report.failureStatus === 'network_error', 'Failure result should preserve status type for run-test-56.');
      const debugLogs = report.debugLogs;
      invariant(Array.isArray(debugLogs) && debugLogs.length > 0, 'Debug logs should be captured when DEBUG=true for run-test-56.');
    },
  },
  {
    id: 'run-test-57',
    description: 'Internal tool provider batch handling and validations.',
    execute: async () => {
      const statusUpdates: string[] = [];
      const finalReports: unknown[] = [];
      const errorLogs: string[] = [];
      const titles: { title: string; emoji?: string }[] = [];

      const orchestratorStub = {
        hasTool: (tool: string) => tool === 'external_tool',
        executeWithManagement: (_tool: string, _args: Record<string, unknown>) => ({ latency: 7, result: 'external-result' }),
      } as unknown as ToolsOrchestrator;

      const slackProvider = new InternalToolProvider('internal-slack', {
        enableBatch: true,
        outputFormat: SLACK_OUTPUT_FORMAT,
        updateStatus: (text) => { statusUpdates.push(text); },
        setTitle: (title, emoji) => { titles.push({ title, emoji }); },
        setFinalReport: (payload) => { finalReports.push(payload); },
        logError: (message) => { errorLogs.push(message); },
        orchestrator: orchestratorStub,
        getCurrentTurn: () => 1,
        toolTimeoutMs: 1000,
      });

      await slackProvider.execute('agent__progress_report', { progress: 'Working' });

      await slackProvider.execute('agent__final_report', {
        status: 'success',
        report_format: SLACK_OUTPUT_FORMAT,
        messages: [
          'Primary message with **bold** text',
          JSON.stringify({ type: 'section', text: { type: 'mrkdwn', text: '*Secondary* content' } }),
          [JSON.stringify({ type: 'divider' })],
        ],
        metadata: { slack: { existing: true } },
      });

      let slackError: Error | undefined;
      try {
        await slackProvider.execute('agent__final_report', { status: 'success', report_format: SLACK_OUTPUT_FORMAT });
      } catch (error: unknown) {
        slackError = error instanceof Error ? error : new Error(String(error));
      }

      await slackProvider.execute('agent__final_report', { status: 'success', report_format: 'markdown', report_content: 'Mismatch format' });

      const jsonProvider = new InternalToolProvider('internal-json', {
        enableBatch: false,
        outputFormat: 'json',
        expectedJsonSchema: {
          type: 'object',
          required: ['status'],
          properties: {
            status: { enum: ['success', 'failure'] },
            details: { type: 'string' },
          },
        },
        updateStatus: (text) => { statusUpdates.push(text); },
        setTitle: (title, emoji) => { titles.push({ title, emoji }); },
        setFinalReport: (payload) => { finalReports.push(payload); },
        logError: (message) => { errorLogs.push(message); },
        orchestrator: orchestratorStub,
        getCurrentTurn: () => 2,
      });

      await jsonProvider.execute('agent__final_report', {
        status: 'success',
        report_format: 'json',
        content_json: { status: 'success', details: 'ok' },
      });

      let jsonMissing: Error | undefined;
      try {
        await jsonProvider.execute('agent__final_report', { status: 'success', report_format: 'json' });
      } catch (error: unknown) {
        jsonMissing = error instanceof Error ? error : new Error(String(error));
      }

      let jsonSchemaError: Error | undefined;
      try {
        await jsonProvider.execute('agent__final_report', {
          status: 'success',
          report_format: 'json',
          content_json: { details: 'missing status' },
        });
      } catch (error: unknown) {
        jsonSchemaError = error instanceof Error ? error : new Error(String(error));
      }

      const batchSuccess = await slackProvider.execute('agent__batch', {
        calls: '[{"id":"1","tool":"external_tool","args":{"value":1}}] trailing text',
      });

      const batchProgress = await slackProvider.execute('agent__batch', {
        calls: [
          { id: '2', tool: 'agent__progress_report', args: { progress: 'Batch update' } },
        ],
      });

      let batchUnknownTool: string | null = null;
      try {
        const unknownResult = await slackProvider.execute('agent__batch', {
          calls: [{ id: '3', tool: 'unknown', args: {} }],
        });
        batchUnknownTool = typeof unknownResult.result === 'string' ? unknownResult.result : JSON.stringify(unknownResult.result);
      } catch (error: unknown) {
        batchUnknownTool = error instanceof Error ? error.message : String(error);
      }

      let batchInvalid: Error | undefined;
      try {
        await slackProvider.execute('agent__batch', {
          calls: [{ id: '', tool: 'external_tool', args: {} }],
        });
      } catch (error: unknown) {
        batchInvalid = error instanceof Error ? error : new Error(String(error));
      }

      let batchEmpty: Error | undefined;
      try {
        await slackProvider.execute('agent__batch', { calls: [] });
      } catch (error: unknown) {
        batchEmpty = error instanceof Error ? error : new Error(String(error));
      }

      const constructorErrors: Error[] = [];
      try {
        new InternalToolProvider('bad-json', {
          enableBatch: false,
          outputFormat: 'markdown',
          expectedJsonSchema: { type: 'object' },
          updateStatus: () => { /* noop */ },
          setTitle: () => { /* noop */ },
          setFinalReport: () => { /* noop */ },
          logError: () => { /* noop */ },
          orchestrator: orchestratorStub,
          getCurrentTurn: () => 0,
        });
      } catch (error: unknown) {
        constructorErrors.push(error instanceof Error ? error : new Error(String(error)));
      }

      return {
        success: true,
        conversation: [],
        logs: [],
        accounting: [],
        finalReport: {
          status: 'success',
          format: 'json',
          content_json: {
            statusUpdates,
            finalReports,
            errorLogs,
            slackError: slackError?.message ?? null,
            jsonMissing: jsonMissing?.message ?? null,
            jsonSchemaError: jsonSchemaError?.message ?? null,
            batchSuccess: batchSuccess.result,
            batchProgress: batchProgress.result,
            batchUnknownTool,
            batchInvalid: batchInvalid?.message ?? null,
            batchEmpty: batchEmpty?.message ?? null,
            constructorErrors: constructorErrors.map((err) => err.message),
          },
          ts: Date.now(),
        },
      };
    },
    expect: (result) => {
      invariant(result.success, 'Scenario run-test-57 should succeed.');
      const report = result.finalReport?.content_json;
      assertRecord(report, 'Report expected for run-test-57.');
      const statusUpdates = report.statusUpdates;
      invariant(Array.isArray(statusUpdates) && statusUpdates.includes('Working') && statusUpdates.includes('Batch update'), 'Status updates should capture progress messages for run-test-57.');
      const finalReports = report.finalReports;
      invariant(Array.isArray(finalReports) && finalReports.length >= 2, 'Final reports should be recorded for run-test-57.');
      const errorLogs = report.errorLogs;
      invariant(Array.isArray(errorLogs) && errorLogs.length >= 2, 'Error logs should capture warnings for run-test-57.');
      invariant(typeof report.slackError === 'string' && report.slackError.includes('requires `messages`'), 'Slack final report missing content should throw for run-test-57.');
      invariant(typeof report.jsonMissing === 'string' && report.jsonMissing.includes('requires `content_json`'), 'JSON final report missing payload should throw for run-test-57.');
      invariant(typeof report.jsonSchemaError === 'string' && report.jsonSchemaError.includes('schema validation failed'), 'JSON schema enforcement should throw for run-test-57.');
      invariant(typeof report.batchSuccess === 'string' && report.batchSuccess.includes('external-result'), 'Batch success payload should include external result for run-test-57.');
      invariant(typeof report.batchProgress === 'string' && report.batchProgress.includes('ok'), 'Batch progress should return ok payload for run-test-57.');
      invariant(typeof report.batchUnknownTool === 'string' && report.batchUnknownTool.includes('Unknown tool'), 'Unknown tool errors should surface for run-test-57.');
      invariant(typeof report.batchInvalid === 'string' && report.batchInvalid.includes('invalid_batch_input'), 'Invalid batch entries should throw for run-test-57.');
      invariant(typeof report.batchEmpty === 'string' && report.batchEmpty.includes('empty_batch'), 'Empty batch should throw for run-test-57.');
      const ctorErrors = report.constructorErrors;
      invariant(Array.isArray(ctorErrors) && ctorErrors.length > 0 && ctorErrors.every((err) => typeof err === 'string'), 'Constructor validation should guard JSON schema misuse for run-test-57.');
      const [firstCtorError] = ctorErrors;
      invariant(firstCtorError.includes('JSON schema provided but output format is not json'), 'Constructor validation should guard JSON schema misuse for run-test-57.');
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
    const message = error instanceof Error ? error.message : String(error);
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
      const message = error instanceof Error ? error.message : String(error);
      const hint = result !== undefined ? formatFailureHint(result) : '';
      // eslint-disable-next-line no-console
      console.error(`${header} [FAIL] ${duration} - ${message}${hint}`);
      throw error;
    }
  }
  // eslint-disable-next-line no-console
  console.log('phase1 scenario: ok');
}

runPhaseOne().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  // eslint-disable-next-line no-console
  console.error(`phase1 scenario failed: ${message}`);
  process.exitCode = 1;
});
