/**
 * Plugin META test suite.
 * Tests plugin META enforcement, cache behavior, and onComplete execution.
 */

import fs from 'node:fs';
import path from 'node:path';

import type {
  AIAgentResult,
  AIAgentSessionConfig,
  Configuration,
  ConversationMessage,
  OrchestrationRuntimeAgent,
  TurnRequest,
} from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import { getResponseCache } from '../../../cache/cache-factory.js';
import { prepareFinalReportPluginDescriptors } from '../../../plugins/loader.js';
import {
  extractNonceFromMessages,
  expectTurnFailureSlugs,
  findTurnFailedMessage,
  invariant,
  makeTempDir,
  ROUTER_HANDOFF_TOOL,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

export const PLUGINS_TESTS: HarnessTest[] = [];

const FINAL_REPORT_TOOL = 'agent__final_report';
const VALID_REPORT_CONTENT = 'Valid final report content.';
const FIRST_REPORT_CONTENT = 'First final report content.';
const SECOND_REPORT_CONTENT = 'Second final report content.';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';
const TURN_FAILED_MARKER = 'TURN-FAILED';
const HARNESS_AGENT_CONTENT = '# harness agent';
const CACHE_HIT_FINAL_MATCH_MSG = 'Cache hit final report should match baseline content.';
const CACHE_HIT_LOG_MSG = 'Cache hit log should be present when META is valid.';
const CACHE_HIT_LLM_MSG = 'Cache hit should not emit LLM accounting entries.';
const META_PLUGIN_NAME = 'meta-required';
const META_PLUGIN_FILE = 'meta-required.js';
const META_TICKET_ID = 'T-123';
const SCENARIO_META_RETRY = 'suite-final-report-meta-missing-meta-only';
const SCENARIO_META_BEFORE_FINAL = 'suite-final-report-meta-before-final';
const SCENARIO_META_EXHAUSTION = 'suite-final-report-meta-missing-exhaustion';
const SCENARIO_META_CACHE_HIT = 'suite-final-report-meta-cache-hit';
const SCENARIO_META_CACHE_MISS = 'suite-final-report-meta-cache-missing-meta';
const SCENARIO_META_CACHE_INVALID = 'suite-final-report-meta-cache-invalid-meta';
const SCENARIO_META_ROUTER_LOCK = 'suite-final-report-meta-router-locked-tools';
const SCENARIO_META_INVALID = 'suite-final-report-meta-invalid-meta-only';
const SCENARIO_META_VALID_AND_INVALID = 'suite-final-report-meta-valid-and-invalid-same-turn';
const SCENARIO_META_ONCOMPLETE_READY = 'suite-final-report-meta-oncomplete-ready';
const SCENARIO_META_ONCOMPLETE_BLOCKED = 'suite-final-report-meta-oncomplete-blocked';
const SCENARIO_META_ONCOMPLETE_CACHE_HIT = 'suite-final-report-meta-oncomplete-cache-hit';
const SCENARIO_META_CACHE_HIT_MUTATION_SAFE = 'suite-final-report-meta-cache-hit-mutation-safe';
const CACHE_TTL_MS = 60_000;
const CACHE_AGENT_HASH = 'phase2-meta-cache-hash';
const CACHE_USER_PROMPT = 'Phase2 META cache prompt';
const META_CACHE_HIT_LABEL = 'meta-cache-hit';
const META_CACHE_MISS_LABEL = 'meta-cache-miss';
const META_CACHE_INVALID_LABEL = 'meta-cache-invalid';
const META_CACHE_ONCOMPLETE_LABEL = 'meta-cache-oncomplete';
const META_CACHE_MUTATION_LABEL = 'meta-cache-mutate';
const ROUTER_DEST_LABEL = 'router-destination';
const ROUTER_DEST_FILE = 'router-destination.ai';
const ONCOMPLETE_FILE_NAME = 'on-complete.json';

type PluginDescriptors = NonNullable<AIAgentSessionConfig['finalReportPluginDescriptors']>;

interface MetaPluginFixture {
  rootDir: string;
  descriptors: PluginDescriptors;
  agentPath: string;
  pluginName: string;
  cleanup: () => void;
}

interface OnCompletePluginFixture extends MetaPluginFixture {
  completionPath: string;
}

const cleanupTempDir = (dir: string): void => {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // best-effort cleanup for harness temp directories
  }
};

const buildMetaPluginSource = (pluginName: string): string => `
module.exports = function finalReportPluginFactory() {
  return {
    name: '${pluginName}',
    getRequirements() {
      return {
        schema: {
          type: 'object',
          additionalProperties: false,
          properties: {
            ticketId: { type: 'string' },
          },
          required: ['ticketId'],
        },
        systemPromptInstructions: 'Provide META JSON with ticketId.',
        xmlNextSnippet: 'META ${pluginName} requires ticketId.',
        finalReportExampleSnippet: '<ai-agent-NONCE-META plugin="${pluginName}">{"ticketId":"123"}</ai-agent-NONCE-META>',
      };
    },
    async onComplete() {},
  };
};
`.trim();

const buildOnCompletePluginSource = (pluginName: string, completionPath: string, mutatePluginData: boolean): string => `
const fs = require('node:fs');

module.exports = function finalReportPluginFactory() {
  return {
    name: '${pluginName}',
    getRequirements() {
      return {
        schema: {
          type: 'object',
          additionalProperties: false,
          properties: {
            ticketId: { type: 'string' },
          },
          required: ['ticketId'],
        },
        systemPromptInstructions: 'Provide META JSON with ticketId.',
        xmlNextSnippet: 'META ${pluginName} requires ticketId.',
        finalReportExampleSnippet: '<ai-agent-NONCE-META plugin="${pluginName}">{"ticketId":"123"}</ai-agent-NONCE-META>',
      };
    },
    async onComplete(context) {
      if (${mutatePluginData ? 'true' : 'false'}) {
        context.pluginData.ticketId = 123;
      }
      fs.writeFileSync(${JSON.stringify(completionPath)}, JSON.stringify({
        ticketId: context.pluginData.ticketId,
        finalContent: context.finalReport.content,
        fromCache: context.fromCache,
      }));
    },
  };
};
`.trim();

const createMetaPluginFixture = (): MetaPluginFixture => {
  const root = makeTempDir('final-meta');
  const agentPath = path.join(root, 'agent.ai');
  fs.writeFileSync(agentPath, HARNESS_AGENT_CONTENT);
  const pluginPath = path.join(root, META_PLUGIN_FILE);
  fs.writeFileSync(pluginPath, buildMetaPluginSource(META_PLUGIN_NAME));
  const prepared = prepareFinalReportPluginDescriptors(root, [META_PLUGIN_FILE]);
  return {
    rootDir: root,
    descriptors: prepared.descriptors,
    agentPath,
    pluginName: META_PLUGIN_NAME,
    cleanup: () => { cleanupTempDir(root); },
  };
};

const createOnCompletePluginFixture = (): OnCompletePluginFixture => {
  const root = makeTempDir('final-meta-complete');
  const agentPath = path.join(root, 'agent.ai');
  const completionPath = path.join(root, ONCOMPLETE_FILE_NAME);
  fs.writeFileSync(agentPath, HARNESS_AGENT_CONTENT);
  const pluginPath = path.join(root, META_PLUGIN_FILE);
  fs.writeFileSync(pluginPath, buildOnCompletePluginSource(META_PLUGIN_NAME, completionPath, false));
  const prepared = prepareFinalReportPluginDescriptors(root, [META_PLUGIN_FILE]);
  return {
    rootDir: root,
    descriptors: prepared.descriptors,
    agentPath,
    pluginName: META_PLUGIN_NAME,
    completionPath,
    cleanup: () => { cleanupTempDir(root); },
  };
};

const createMutatingOnCompletePluginFixture = (): OnCompletePluginFixture => {
  const root = makeTempDir('final-meta-complete-mutate');
  const agentPath = path.join(root, 'agent.ai');
  const completionPath = path.join(root, ONCOMPLETE_FILE_NAME);
  fs.writeFileSync(agentPath, HARNESS_AGENT_CONTENT);
  const pluginPath = path.join(root, META_PLUGIN_FILE);
  fs.writeFileSync(pluginPath, buildOnCompletePluginSource(META_PLUGIN_NAME, completionPath, true));
  const prepared = prepareFinalReportPluginDescriptors(root, [META_PLUGIN_FILE]);
  return {
    rootDir: root,
    descriptors: prepared.descriptors,
    agentPath,
    pluginName: META_PLUGIN_NAME,
    completionPath,
    cleanup: () => { cleanupTempDir(root); },
  };
};

const buildMetaWrapper = (nonce: string, pluginName: string, ticketId: string): string => (
  `<ai-agent-${nonce}-META plugin="${pluginName}">{"ticketId":"${ticketId}"}</ai-agent-${nonce}-META>`
);

const buildInvalidMetaWrapper = (nonce: string, pluginName: string): string => (
  `<ai-agent-${nonce}-META plugin="${pluginName}">{"ticketId":123}</ai-agent-${nonce}-META>`
);

interface ObservedTurnTools {
  invocation: number;
  isFinalTurn: boolean;
  toolNames: string[];
}

const captureTurnTools = (request: TurnRequest, invocation: number): ObservedTurnTools => ({
  invocation,
  isFinalTurn: request.turnMetadata?.isFinalTurn === true,
  toolNames: request.tools.map((tool) => tool.name),
});

const createRouterDestination = (rootDir: string): OrchestrationRuntimeAgent => {
  const promptPath = path.join(rootDir, ROUTER_DEST_FILE);
  fs.writeFileSync(promptPath, '# router destination');
  return {
    ref: ROUTER_DEST_LABEL,
    path: promptPath,
    agentId: ROUTER_DEST_LABEL,
    promptPath,
    systemTemplate: 'router destination',
    toolName: ROUTER_DEST_LABEL,
    expectedOutput: { format: 'markdown' },
    run: (_systemPrompt: string, _userPrompt: string, _opts) => Promise.resolve({
      success: true,
      conversation: [],
      logs: [],
      accounting: [],
    }),
  };
};

const countFinalReportToolCalls = (conversation: readonly ConversationMessage[]): number => conversation.reduce((count, message) => (
  count + (Array.isArray(message.toolCalls)
    ? message.toolCalls.filter((call) => call.name === FINAL_REPORT_TOOL).length
    : 0)
), 0);

const parseTurnFailureSlugs = (result: AIAgentResult, scenarioId: string, required: string[] = []): string[] => {
  const entry = expectTurnFailureSlugs(result.logs, scenarioId, required);
  const slugStr = typeof entry.details?.slugs === 'string' ? entry.details.slugs : '';
  return slugStr.split(',').filter((slug) => slug.length > 0);
};

const buildAgentCacheKeyPayload = (sessionConfig: AIAgentSessionConfig): Record<string, unknown> => {
  const expectedSchema = sessionConfig.expectedOutput?.format === 'json'
    ? sessionConfig.expectedOutput.schema
    : undefined;
  return {
    v: 1,
    kind: 'agent',
    agentHash: sessionConfig.agentHash,
    request: {
      userPrompt: sessionConfig.userPrompt,
      format: sessionConfig.outputFormat,
      schema: expectedSchema,
    },
  };
};

const hasAgentCacheHitLog = (result: AIAgentResult): boolean => result.logs.some((entry) => entry.remoteIdentifier === 'agent:cache');

const countLlmAccountingEntries = (result: AIAgentResult): number => result.accounting.filter((entry) => entry.type === 'llm').length;

const findFinalReportPluginsLog = (result: AIAgentResult, pattern: string) => result.logs.find((entry) => (
  entry.remoteIdentifier === 'agent:final-report-plugins' && entry.message.includes(pattern)
));

interface CompletionPayload {
  ticketId: string;
  finalContent: string;
  fromCache: boolean;
}

const isRecord = (value: unknown): value is Record<string, unknown> => (
  value !== null && typeof value === 'object' && !Array.isArray(value)
);

const hasTurnFailureSlugs = (result: AIAgentResult, scenarioId: string): boolean => result.logs.some((entry) => {
  if (entry.remoteIdentifier !== scenarioId || !isRecord(entry.details)) {
    return false;
  }
  const slugs = entry.details.slugs;
  return typeof slugs === 'string' && slugs.length > 0;
});

const readCompletionPayload = (completionPath: string): CompletionPayload | undefined => {
  if (!fs.existsSync(completionPath)) {
    return undefined;
  }
  try {
    const raw = fs.readFileSync(completionPath, 'utf8');
    const parsed: unknown = JSON.parse(raw);
    if (!isRecord(parsed)) {
      return undefined;
    }
    const ticketId = parsed.ticketId;
    const finalContent = parsed.finalContent;
    const fromCache = parsed.fromCache;
    if (typeof ticketId !== 'string' || typeof finalContent !== 'string' || typeof fromCache !== 'boolean') {
      return undefined;
    }
    return { ticketId, finalContent, fromCache };
  } catch {
    return undefined;
  }
};

// Test: Final report without META triggers retry; META-only follow-up succeeds without replacing FINAL
PLUGINS_TESTS.push({
  id: SCENARIO_META_RETRY,
  description: 'Suite: Missing META triggers retry and accepts META-only completion',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 1;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-retry-final';
    const reportContent = FIRST_REPORT_CONTENT;

    try {
      return await runWithExecuteTurnOverride(sessionConfig, ({ request, invocation }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_RETRY);
        if (invocation === 1) {
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            response: '',
            messages: [
              {
                role: 'assistant',
                content: '',
                toolCalls: [{
                  id: finalToolCallId,
                  name: FINAL_REPORT_TOOL,
                  parameters: {
                    report_format: finalFormat,
                    report_content: reportContent,
                  },
                }],
              } as ConversationMessage,
              {
                role: 'tool',
                toolCallId: finalToolCallId,
                content: reportContent,
              } as ConversationMessage,
            ],
            tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
          });
        }

        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          latencyMs: 5,
          response: metaWrapper,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
        });
      });
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'META-only retry should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === FIRST_REPORT_CONTENT, 'Locked final report should be preserved');
    invariant(countFinalReportToolCalls(result.conversation) === 1, 'Final report tool call should occur only once');
    parseTurnFailureSlugs(result, SCENARIO_META_RETRY, ['final_meta_missing']);
    const nonce = extractNonceFromMessages(result.conversation, SCENARIO_META_RETRY);
    const turnFailed = findTurnFailedMessage(result.conversation, TURN_FAILED_MARKER);
    invariant(turnFailed !== undefined, 'TURN-FAILED feedback should be present');
    invariant(typeof turnFailed.content === 'string', 'TURN-FAILED feedback should be text');
    const turnFailedContent = turnFailed.content;
    invariant(turnFailedContent.includes(nonce), 'TURN-FAILED feedback should include the session nonce');
    invariant(!turnFailedContent.includes('NONCE'), 'TURN-FAILED feedback should not include the literal NONCE token');
  },
} satisfies HarnessTest);

// Test: Invalid META triggers retry feedback and succeeds with META-only correction
PLUGINS_TESTS.push({
  id: SCENARIO_META_INVALID,
  description: 'Suite: Invalid META yields detailed TURN-FAILED feedback and succeeds after correction',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-invalid-final';

    try {
      return await runWithExecuteTurnOverride(sessionConfig, ({ request, invocation }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_INVALID);
        if (invocation === 1) {
          const invalidMetaWrapper = buildInvalidMetaWrapper(nonce, fixture.pluginName);
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            response: '',
            messages: [
              {
                role: 'assistant',
                content: invalidMetaWrapper,
                toolCalls: [{
                  id: finalToolCallId,
                  name: FINAL_REPORT_TOOL,
                  parameters: {
                    report_format: finalFormat,
                    report_content: VALID_REPORT_CONTENT,
                  },
                }],
              } as ConversationMessage,
            ],
            tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
          });
        }

        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          latencyMs: 5,
          response: metaWrapper,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 7, outputTokens: 4, totalTokens: 11 },
        });
      });
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Invalid META should be recoverable via META-only retry.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should remain locked.');
    invariant(countFinalReportToolCalls(result.conversation) === 1, 'Final report tool call should not repeat.');
    parseTurnFailureSlugs(result, SCENARIO_META_INVALID, ['final_meta_missing', 'final_meta_invalid']);
    const nonce = extractNonceFromMessages(result.conversation, SCENARIO_META_INVALID);
    const turnFailed = findTurnFailedMessage(result.conversation, TURN_FAILED_MARKER);
    invariant(turnFailed !== undefined, 'TURN-FAILED feedback should be present for invalid META.');
    invariant(typeof turnFailed.content === 'string', 'TURN-FAILED feedback should be text for invalid META.');
    const turnFailedContent = turnFailed.content;
    invariant(turnFailedContent.includes(nonce), 'TURN-FAILED feedback should include the session nonce.');
    invariant(turnFailedContent.includes('schema_mismatch='), 'TURN-FAILED feedback should include schema mismatch details.');
    invariant(!turnFailedContent.includes('NONCE'), 'TURN-FAILED feedback should not include literal NONCE token.');
  },
} satisfies HarnessTest);

// Test: Valid META keeps finalization ready even if an invalid META block is also present
PLUGINS_TESTS.push({
  id: SCENARIO_META_VALID_AND_INVALID,
  description: 'Suite: Valid META wins and invalid META does not trigger retries when finalization is ready',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-valid-and-invalid-final';

    try {
      return await runWithExecuteTurnOverride(sessionConfig, ({ request }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_VALID_AND_INVALID);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        const invalidMetaWrapper = buildInvalidMetaWrapper(nonce, fixture.pluginName);
        const combinedMeta = `${metaWrapper}\n${invalidMetaWrapper}`;
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: combinedMeta,
          messages: [
            {
              role: 'assistant',
              content: combinedMeta,
              toolCalls: [{
                id: finalToolCallId,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: finalFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        });
      });
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Valid META should keep finalization ready even if invalid META is also present.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should match.');
    invariant(countFinalReportToolCalls(result.conversation) === 1, 'Final report tool call should occur once.');
    invariant(!hasTurnFailureSlugs(result, SCENARIO_META_VALID_AND_INVALID), 'No TURN-FAILED slugs should be emitted when finalization is ready.');
    const turnFailed = findTurnFailedMessage(result.conversation, TURN_FAILED_MARKER);
    invariant(turnFailed === undefined, 'TURN-FAILED feedback should not be emitted when valid META is present.');
  },
} satisfies HarnessTest);

// Test: Router handoff tool is hidden during META-only retries when FINAL is locked
PLUGINS_TESTS.push({
  id: SCENARIO_META_ROUTER_LOCK,
  description: 'Suite: Router tool is removed when final report is locked for META-only retries',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const routerDestination = createRouterDestination(fixture.rootDir);
    sessionConfig.orchestration = {
      ...sessionConfig.orchestration,
      router: { destinations: [routerDestination] },
    };

    const observed: ObservedTurnTools[] = [];
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-router-lock-final';

    try {
      const result = await runWithExecuteTurnOverride(sessionConfig, ({ request, invocation }) => {
        observed.push(captureTurnTools(request, invocation));
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_ROUTER_LOCK);
        if (invocation === 1) {
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            response: '',
            messages: [
              {
                role: 'assistant',
                content: '',
                toolCalls: [{
                  id: finalToolCallId,
                  name: FINAL_REPORT_TOOL,
                  parameters: {
                    report_format: finalFormat,
                    report_content: FIRST_REPORT_CONTENT,
                  },
                }],
              } as ConversationMessage,
            ],
            tokens: { inputTokens: 9, outputTokens: 5, totalTokens: 14 },
          });
        }

        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          latencyMs: 5,
          response: metaWrapper,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 7, outputTokens: 4, totalTokens: 11 },
        });
      });

      return {
        ...result,
        __observedTools: observed,
      } as AIAgentResult & { __observedTools?: ObservedTurnTools[] };
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult & { __observedTools?: ObservedTurnTools[] }) => {
    invariant(result.success, 'Router META-only retry scenario should succeed.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    const observed = result.__observedTools ?? [];
    invariant(observed.length >= 2, 'Expected at least two LLM invocations for router META-only retry scenario.');
    const first = observed.find((entry) => entry.invocation === 1);
    const second = observed.find((entry) => entry.invocation === 2);
    invariant(first !== undefined && second !== undefined, 'Expected tool observations for both invocations.');
    invariant(first.isFinalTurn && second.isFinalTurn, 'Both invocations should be final turns.');
    invariant(first.toolNames.includes(ROUTER_HANDOFF_TOOL), 'Router tool should be available before FINAL is locked.');
    invariant(second.toolNames.includes(FINAL_REPORT_TOOL), 'Final report tool should remain available during META-only retries.');
    invariant(!second.toolNames.includes(ROUTER_HANDOFF_TOOL), 'Router tool must be removed when FINAL is locked for META-only retries.');
  },
} satisfies HarnessTest);

// Test: META before FINAL is stored and finalization succeeds once FINAL arrives
PLUGINS_TESTS.push({
  id: SCENARIO_META_BEFORE_FINAL,
  description: 'Suite: META can arrive before FINAL and still satisfy finalization',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 1;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-before-final';
    const reportContent = VALID_REPORT_CONTENT;

    try {
      return await runWithExecuteTurnOverride(sessionConfig, ({ request, invocation }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_BEFORE_FINAL);
        if (invocation === 1) {
          const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: false, finalAnswer: true },
            latencyMs: 5,
            response: metaWrapper,
            messages: [
              {
                role: 'assistant',
                content: metaWrapper,
              } as ConversationMessage,
            ],
            tokens: { inputTokens: 9, outputTokens: 4, totalTokens: 13 },
          });
        }

        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: '',
          messages: [
            {
              role: 'assistant',
              content: '',
              toolCalls: [{
                id: finalToolCallId,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: finalFormat,
                  report_content: reportContent,
                },
              }],
            } as ConversationMessage,
            {
              role: 'tool',
              toolCallId: finalToolCallId,
              content: reportContent,
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
        });
      });
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'META before FINAL should succeed once FINAL arrives');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should match');
    const slugs = parseTurnFailureSlugs(result, SCENARIO_META_BEFORE_FINAL, ['final_report_missing']);
    invariant(!slugs.includes('final_meta_missing'), 'META should not be reported missing when provided before FINAL');
  },
} satisfies HarnessTest);

// Test: Exhaustion with FINAL present but META missing returns synthetic failure report
PLUGINS_TESTS.push({
  id: SCENARIO_META_EXHAUSTION,
  description: 'Suite: Missing META at exhaustion yields synthetic failure final report',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const fixture = createMetaPluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-exhaustion-final';
    const reportContent = SECOND_REPORT_CONTENT;

    try {
      return await runWithExecuteTurnOverride(sessionConfig, ({ request, invocation }) => {
        extractNonceFromMessages(request.messages, SCENARIO_META_EXHAUSTION);
        if (invocation === 1) {
          return Promise.resolve({
            status: { type: 'success', hasToolCalls: true, finalAnswer: true },
            latencyMs: 5,
            response: '',
            messages: [
              {
                role: 'assistant',
                content: '',
                toolCalls: [{
                  id: finalToolCallId,
                  name: FINAL_REPORT_TOOL,
                  parameters: {
                    report_format: finalFormat,
                    report_content: reportContent,
                  },
                }],
              } as ConversationMessage,
              {
                role: 'tool',
                toolCallId: finalToolCallId,
                content: reportContent,
              } as ConversationMessage,
            ],
            tokens: { inputTokens: 10, outputTokens: 5, totalTokens: 15 },
          });
        }

        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: true },
          latencyMs: 5,
          response: 'META still missing.',
          messages: [
            {
              role: 'assistant',
              content: 'META still missing.',
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 7, outputTokens: 3, totalTokens: 10 },
        });
      });
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult) => {
    invariant(!result.success, 'Missing META at exhaustion should fail');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    const slugs = parseTurnFailureSlugs(result, SCENARIO_META_EXHAUSTION, ['final_meta_missing']);
    invariant(slugs.includes('final_meta_missing'), 'Turn failure slugs should include final_meta_missing');
    const metadata = result.finalReport.metadata;
    const reason = typeof metadata?.reason === 'string' ? metadata.reason : '';
    invariant(reason === 'final_meta_missing', 'Synthetic failure reason should be final_meta_missing');
    const missingPluginsRaw = metadata?.missing_meta_plugins;
    const missingPlugins = Array.isArray(missingPluginsRaw)
      ? missingPluginsRaw.filter((value): value is string => typeof value === 'string')
      : typeof missingPluginsRaw === 'string'
        ? missingPluginsRaw.split(',').map((value) => value.trim()).filter((value) => value.length > 0)
        : [];
    invariant(missingPlugins.includes(META_PLUGIN_NAME), 'Synthetic failure should list missing META plugins');
  },
} satisfies HarnessTest);

// Test: onComplete runs only after finalization readiness (FINAL + META)
PLUGINS_TESTS.push({
  id: SCENARIO_META_ONCOMPLETE_READY,
  description: 'Suite: Plugin onComplete runs when finalization readiness is achieved',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const fixture = createOnCompletePluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-oncomplete-ready';

    try {
      const result = await runWithExecuteTurnOverride(sessionConfig, ({ request }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_ONCOMPLETE_READY);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: finalToolCallId,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: finalFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 9, outputTokens: 5, totalTokens: 14 },
        });
      });
      const completionPayload = readCompletionPayload(fixture.completionPath);
      return {
        ...result,
        __completionPayload: completionPayload,
      } as AIAgentResult & { __completionPayload?: CompletionPayload };
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult & { __completionPayload?: CompletionPayload }) => {
    invariant(result.success, 'onComplete-ready scenario should succeed.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    const completionPayload = result.__completionPayload;
    invariant(completionPayload !== undefined, 'onComplete payload should be written when finalization is ready.');
    invariant(completionPayload.ticketId === META_TICKET_ID, 'onComplete should receive plugin META.');
    invariant(completionPayload.finalContent === VALID_REPORT_CONTENT, 'onComplete should receive final report content.');
    invariant(!completionPayload.fromCache, 'onComplete should report fromCache=false for live runs.');
  },
} satisfies HarnessTest);

// Test: onComplete is blocked when finalization readiness is not achieved
PLUGINS_TESTS.push({
  id: SCENARIO_META_ONCOMPLETE_BLOCKED,
  description: 'Suite: Plugin onComplete does not run when META is missing',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const fixture = createOnCompletePluginFixture();
    sessionConfig.finalReportPluginDescriptors = fixture.descriptors;
    sessionConfig.agentPath = fixture.agentPath;
    const finalFormat = sessionConfig.outputFormat;
    const finalToolCallId = 'meta-oncomplete-blocked';

    try {
      const result = await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
        status: { type: 'success', hasToolCalls: true, finalAnswer: true },
        latencyMs: 5,
        response: '',
        messages: [
          {
            role: 'assistant',
            content: '',
            toolCalls: [{
              id: finalToolCallId,
              name: FINAL_REPORT_TOOL,
              parameters: {
                report_format: finalFormat,
                report_content: VALID_REPORT_CONTENT,
              },
            }],
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 8, outputTokens: 4, totalTokens: 12 },
      }));
      const completionPayload = readCompletionPayload(fixture.completionPath);
      return {
        ...result,
        __completionPayload: completionPayload,
      } as AIAgentResult & { __completionPayload?: CompletionPayload };
    } finally {
      fixture.cleanup();
    }
  },
  expect: (result: AIAgentResult & { __completionPayload?: CompletionPayload }) => {
    invariant(!result.success, 'onComplete-blocked scenario should fail due to missing META.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.__completionPayload === undefined, 'onComplete must not run when finalization readiness is missing.');
  },
} satisfies HarnessTest);

PLUGINS_TESTS.push({
  id: SCENARIO_META_ONCOMPLETE_CACHE_HIT,
  description: 'Suite: Plugin onComplete runs on cache hit with fromCache=true',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    const fixture = createOnCompletePluginFixture();
    const cacheDir = makeTempDir(META_CACHE_ONCOMPLETE_LABEL);
    const cachePath = path.join(cacheDir, 'cache.db');
    const baseConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      config: {
        ...sessionConfig.config,
        cache: { backend: 'sqlite', sqlite: { path: cachePath }, maxEntries: 10 },
      },
      userPrompt: CACHE_USER_PROMPT,
      agentHash: `${CACHE_AGENT_HASH}-oncomplete`,
      cacheTtlMs: CACHE_TTL_MS,
      maxTurns: 1,
      maxRetries: 1,
      finalReportPluginDescriptors: fixture.descriptors,
      agentPath: fixture.agentPath,
    };

    try {
      const first = await runWithExecuteTurnOverride({ ...baseConfig }, ({ request }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_ONCOMPLETE_CACHE_HIT);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: `${META_CACHE_ONCOMPLETE_LABEL}-final`,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: baseConfig.outputFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            },
          ],
          rawResponse: { id: `${META_CACHE_ONCOMPLETE_LABEL}-first`, model: request.model, tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 } },
          tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 },
        });
      });
      const completionBefore = readCompletionPayload(fixture.completionPath);

      const second = await runWithExecuteTurnOverride({ ...baseConfig }, () => {
        throw new Error('LLM should not be called on cache hit with valid META.');
      });
      const completionAfter = readCompletionPayload(fixture.completionPath);

      return {
        ...second,
        __cacheBaseline: {
          finalReport: first.finalReport,
        },
        __completionBefore: completionBefore,
        __completionAfter: completionAfter,
      } as AIAgentResult & {
        __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] };
        __completionBefore?: CompletionPayload;
        __completionAfter?: CompletionPayload;
      };
    } finally {
      fixture.cleanup();
      cleanupTempDir(cacheDir);
    }
  },
  expect: (result: AIAgentResult & {
    __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] };
    __completionBefore?: CompletionPayload;
    __completionAfter?: CompletionPayload;
  }) => {
    const baseline = result.__cacheBaseline;
    invariant(baseline?.finalReport !== undefined, 'Baseline final report missing for onComplete cache hit scenario.');
    invariant(result.success, 'onComplete cache hit scenario should succeed.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === baseline.finalReport.content, CACHE_HIT_FINAL_MATCH_MSG);
    invariant(hasAgentCacheHitLog(result), CACHE_HIT_LOG_MSG);
    invariant(countLlmAccountingEntries(result) === 0, CACHE_HIT_LLM_MSG);
    const before = result.__completionBefore;
    const after = result.__completionAfter;
    invariant(before !== undefined && after !== undefined, 'onComplete payload should be captured for cache hit.');
    invariant(before.ticketId === after.ticketId, 'onComplete cache hit should preserve ticketId.');
    invariant(after.fromCache, 'onComplete cache hit should report fromCache=true.');
  },
} satisfies HarnessTest);

PLUGINS_TESTS.push({
  id: SCENARIO_META_CACHE_HIT_MUTATION_SAFE,
  description: 'Suite: Cache hit remains valid even if plugin onComplete mutates META',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    const fixture = createMutatingOnCompletePluginFixture();
    const cacheDir = makeTempDir(META_CACHE_MUTATION_LABEL);
    const cachePath = path.join(cacheDir, 'cache.db');
    const baseConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      config: {
        ...sessionConfig.config,
        cache: { backend: 'sqlite', sqlite: { path: cachePath }, maxEntries: 10 },
      },
      userPrompt: CACHE_USER_PROMPT,
      agentHash: `${CACHE_AGENT_HASH}-mutate`,
      cacheTtlMs: CACHE_TTL_MS,
      maxTurns: 1,
      maxRetries: 1,
      finalReportPluginDescriptors: fixture.descriptors,
      agentPath: fixture.agentPath,
    };

    try {
      const first = await runWithExecuteTurnOverride({ ...baseConfig }, ({ request }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_CACHE_HIT_MUTATION_SAFE);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: `${META_CACHE_MUTATION_LABEL}-final`,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: baseConfig.outputFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            },
          ],
          rawResponse: { id: `${META_CACHE_MUTATION_LABEL}-first`, model: request.model, tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 } },
          tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 },
        });
      });

      const second = await runWithExecuteTurnOverride({ ...baseConfig }, () => {
        throw new Error('LLM should not be called on cache hit when META is valid.');
      });

      return {
        ...second,
        __cacheBaseline: {
          finalReport: first.finalReport,
        },
      } as AIAgentResult & {
        __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] };
      };
    } finally {
      fixture.cleanup();
      cleanupTempDir(cacheDir);
    }
  },
  expect: (result: AIAgentResult & { __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] } }) => {
    const baseline = result.__cacheBaseline;
    invariant(baseline?.finalReport !== undefined, 'Baseline final report missing for cache hit mutation scenario.');
    invariant(result.success, 'Cache hit mutation scenario should succeed.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === baseline.finalReport.content, CACHE_HIT_FINAL_MATCH_MSG);
    invariant(hasAgentCacheHitLog(result), CACHE_HIT_LOG_MSG);
    invariant(countLlmAccountingEntries(result) === 0, CACHE_HIT_LLM_MSG);
  },
} satisfies HarnessTest);

PLUGINS_TESTS.push({
  id: SCENARIO_META_CACHE_HIT,
  description: 'Suite: Cache hit requires valid plugin META and skips LLM execution',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    const fixture = createMetaPluginFixture();
    const cacheDir = makeTempDir(META_CACHE_HIT_LABEL);
    const cachePath = path.join(cacheDir, 'cache.db');
    const baseConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      config: {
        ...sessionConfig.config,
        cache: { backend: 'sqlite', sqlite: { path: cachePath }, maxEntries: 10 },
      },
      userPrompt: CACHE_USER_PROMPT,
      agentHash: `${CACHE_AGENT_HASH}-hit`,
      cacheTtlMs: CACHE_TTL_MS,
      maxTurns: 1,
      maxRetries: 1,
      finalReportPluginDescriptors: fixture.descriptors,
      agentPath: fixture.agentPath,
    };

    try {
      const first = await runWithExecuteTurnOverride({ ...baseConfig }, ({ request }) => {
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_CACHE_HIT);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: `${META_CACHE_HIT_LABEL}-final`,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: baseConfig.outputFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            },
          ],
          rawResponse: { id: `${META_CACHE_HIT_LABEL}-first`, model: request.model, tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 } },
          tokens: { inputTokens: 10, outputTokens: 6, totalTokens: 16 },
        });
      });

      const second = await runWithExecuteTurnOverride({ ...baseConfig }, () => {
        throw new Error('LLM should not be called on cache hit with valid META.');
      });

      return {
        ...second,
        __cacheBaseline: {
          finalReport: first.finalReport,
        },
      } as AIAgentResult & { __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] } };
    } finally {
      fixture.cleanup();
      cleanupTempDir(cacheDir);
    }
  },
  expect: (result: AIAgentResult & { __cacheBaseline?: { finalReport?: AIAgentResult['finalReport'] } }) => {
    const baseline = result.__cacheBaseline;
    invariant(baseline?.finalReport !== undefined, 'Baseline final report missing for cache hit scenario.');
    invariant(result.success, 'Cache hit scenario should succeed.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === baseline.finalReport.content, CACHE_HIT_FINAL_MATCH_MSG);
    invariant(hasAgentCacheHitLog(result), CACHE_HIT_LOG_MSG);
    invariant(countLlmAccountingEntries(result) === 0, CACHE_HIT_LLM_MSG);
  },
} satisfies HarnessTest);

PLUGINS_TESTS.push({
  id: SCENARIO_META_CACHE_MISS,
  description: 'Suite: Cache entry without META is rejected and forces LLM execution',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    const fixture = createMetaPluginFixture();
    const cacheDir = makeTempDir(META_CACHE_MISS_LABEL);
    const cachePath = path.join(cacheDir, 'cache.db');
    const baseConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      config: {
        ...sessionConfig.config,
        cache: { backend: 'sqlite', sqlite: { path: cachePath }, maxEntries: 10 },
      },
      userPrompt: CACHE_USER_PROMPT,
      agentHash: `${CACHE_AGENT_HASH}-miss`,
      cacheTtlMs: CACHE_TTL_MS,
      maxTurns: 1,
      maxRetries: 1,
      finalReportPluginDescriptors: fixture.descriptors,
      agentPath: fixture.agentPath,
    };

    try {
      const responseCache = getResponseCache(baseConfig.config.cache);
      invariant(responseCache !== undefined, 'Response cache should be available for META cache miss scenario.');
      const cacheKeyPayload = buildAgentCacheKeyPayload(baseConfig);
      const invalidPayload = {
        finalReport: {
          format: baseConfig.outputFormat,
          content: 'Cached final report without META.',
          ts: Date.now(),
        },
        conversation: [],
        childConversations: [],
      };
      await responseCache.set(
        cacheKeyPayload,
        CACHE_TTL_MS,
        invalidPayload,
        { kind: 'agent', agentName: META_CACHE_MISS_LABEL, agentHash: baseConfig.agentHash, format: baseConfig.outputFormat },
        Date.now(),
      );

      let llmCalls = 0;
      const result = await runWithExecuteTurnOverride({ ...baseConfig }, ({ request }) => {
        llmCalls += 1;
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_CACHE_MISS);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: `${META_CACHE_MISS_LABEL}-final`,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: baseConfig.outputFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            },
          ],
          rawResponse: { id: META_CACHE_MISS_LABEL, model: request.model, tokens: { inputTokens: 11, outputTokens: 6, totalTokens: 17 } },
          tokens: { inputTokens: 11, outputTokens: 6, totalTokens: 17 },
        });
      });

      return {
        ...result,
        __llmCalls: llmCalls,
      } as AIAgentResult & { __llmCalls?: number };
    } finally {
      fixture.cleanup();
      cleanupTempDir(cacheDir);
    }
  },
  expect: (result: AIAgentResult & { __llmCalls?: number }) => {
    invariant((result.__llmCalls ?? 0) > 0, 'LLM should be called when cache META is missing.');
    invariant(!hasAgentCacheHitLog(result), 'Agent cache hit log should be absent when cache META is missing.');
    const cacheMissLog = result.logs.find((entry) => entry.remoteIdentifier === 'agent:final-report-plugins' && entry.message.includes('cache miss'));
    invariant(cacheMissLog !== undefined, 'META cache miss log expected when META is missing from cache entry.');
  },
} satisfies HarnessTest);

PLUGINS_TESTS.push({
  id: SCENARIO_META_CACHE_INVALID,
  description: 'Suite: Cache entry with invalid META is rejected and forces LLM execution',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    const fixture = createMetaPluginFixture();
    const cacheDir = makeTempDir(META_CACHE_INVALID_LABEL);
    const cachePath = path.join(cacheDir, 'cache.db');
    const baseConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      config: {
        ...sessionConfig.config,
        cache: { backend: 'sqlite', sqlite: { path: cachePath }, maxEntries: 10 },
      },
      userPrompt: CACHE_USER_PROMPT,
      agentHash: `${CACHE_AGENT_HASH}-invalid`,
      cacheTtlMs: CACHE_TTL_MS,
      maxTurns: 1,
      maxRetries: 1,
      finalReportPluginDescriptors: fixture.descriptors,
      agentPath: fixture.agentPath,
    };

    try {
      const responseCache = getResponseCache(baseConfig.config.cache);
      invariant(responseCache !== undefined, 'Response cache should be available for META cache invalid scenario.');
      const cacheKeyPayload = buildAgentCacheKeyPayload(baseConfig);
      const invalidPayload = {
        finalReport: {
          format: baseConfig.outputFormat,
          content: 'Cached final report with invalid META.',
          ts: Date.now(),
        },
        pluginMetas: {
          [fixture.pluginName]: { ticketId: 123 },
        },
        conversation: [],
        childConversations: [],
      };
      await responseCache.set(
        cacheKeyPayload,
        CACHE_TTL_MS,
        invalidPayload,
        { kind: 'agent', agentName: META_CACHE_INVALID_LABEL, agentHash: baseConfig.agentHash, format: baseConfig.outputFormat },
        Date.now(),
      );

      let llmCalls = 0;
      const result = await runWithExecuteTurnOverride({ ...baseConfig }, ({ request }) => {
        llmCalls += 1;
        const nonce = extractNonceFromMessages(request.messages, SCENARIO_META_CACHE_INVALID);
        const metaWrapper = buildMetaWrapper(nonce, fixture.pluginName, META_TICKET_ID);
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: true },
          latencyMs: 5,
          response: `${metaWrapper}\n${VALID_REPORT_CONTENT}`,
          messages: [
            {
              role: 'assistant',
              content: metaWrapper,
              toolCalls: [{
                id: `${META_CACHE_INVALID_LABEL}-final`,
                name: FINAL_REPORT_TOOL,
                parameters: {
                  report_format: baseConfig.outputFormat,
                  report_content: VALID_REPORT_CONTENT,
                },
              }],
            },
          ],
          rawResponse: { id: META_CACHE_INVALID_LABEL, model: request.model, tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 } },
          tokens: { inputTokens: 12, outputTokens: 6, totalTokens: 18 },
        });
      });

      return {
        ...result,
        __llmCalls: llmCalls,
      } as AIAgentResult & { __llmCalls?: number };
    } finally {
      fixture.cleanup();
      cleanupTempDir(cacheDir);
    }
  },
  expect: (result: AIAgentResult & { __llmCalls?: number }) => {
    invariant(result.success, 'Cache invalid META scenario should succeed via LLM execution.');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant((result.__llmCalls ?? 0) > 0, 'LLM should be called when cache META is invalid.');
    invariant(!hasAgentCacheHitLog(result), 'Agent cache hit log should be absent when cache META is invalid.');
    const cacheMissLog = findFinalReportPluginsLog(result, 'META validation failure');
    invariant(cacheMissLog !== undefined, 'META validation failure log expected for invalid META cache entry.');
    invariant(cacheMissLog.message.includes('schema_mismatch='), 'META validation failure log should include schema mismatch details.');
  },
} satisfies HarnessTest);
