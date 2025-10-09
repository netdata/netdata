import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import type { AIAgentResult, AIAgentSessionConfig, AccountingEntry, Configuration } from '../types.js';

import { AIAgentSession } from '../ai-agent.js';
import { sanitizeToolName } from '../utils.js';

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
const SUBAGENT_PRICING_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents/pricing-subagent.ai');
const SUBAGENT_PRICING_SUCCESS_PROMPT = path.resolve(process.cwd(), 'src/tests/fixtures/subagents-success/pricing-subagent-success.ai');
const SUBAGENT_SUCCESS_TOOL = 'agent__pricing-subagent-success';
const CONCURRENCY_TIMEOUT_ARGUMENT = 'trigger-timeout';
const CONCURRENCY_SECOND_ARGUMENT = 'concurrency-second';
const THROW_FAILURE_MESSAGE = 'Simulated provider throw for coverage.';
const BATCH_PROGRESS_RESPONSE = 'batch-progress-follow-up';
const BATCH_STRING_RESULT = 'batch-string-mode';
const TRACEABLE_PROVIDER_URL = 'https://openrouter.ai/api/v1';

function makeTempDir(label: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), `${TMP_PREFIX}${label}-`));
}

let runTest16Paths: { sessionsDir: string; billingFile: string } | undefined;
let runTest20Paths: { baseDir: string; blockerFile: string; billingFile: string } | undefined;

function invariant(condition: boolean, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function isToolAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'tool' } {
  return entry.type === 'tool';
}

function isLlmAccounting(entry: AccountingEntry): entry is AccountingEntry & { type: 'llm' } {
  return entry.type === 'llm';
}

interface HarnessTest {
  id: string;
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
  };

  configure?.(configuration, baseSession, defaults);

  if (typeof execute === 'function') {
    return await execute(configuration, baseSession, defaults);
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

  let session;
  try {
    session = AIAgentSession.create(baseSession);
  } catch (error) {
    return failureResult(error);
  }

  try {
    return await session.run();
  } catch (error) {
    return failureResult(error);
  }
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
    const label = `${runPrefix} ${scenario.id}`;
    // eslint-disable-next-line no-console
    console.log(`[RUN] ${label}`);
    const result = await runScenario(scenario);
    try {
      scenario.expect(result);
      // eslint-disable-next-line no-console
      console.log(`[PASS] ${label}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const hint = formatFailureHint(result);
      // eslint-disable-next-line no-console
      console.error(`[FAIL] ${label}: ${message}${hint}`);
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
