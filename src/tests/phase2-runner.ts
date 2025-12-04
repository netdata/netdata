import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

import type { GlobalOverrides } from '../agent-loader.js';
import type { AccountingEntry, ConversationMessage, LogEntry, ReasoningLevel } from '../types.js';

import { loadAgent, LoadedAgentCache } from '../agent-loader.js';
import { sanitizeToolName } from '../utils.js';

import { phase2ModelConfigs } from './phase2-models.js';

type Tier = 1 | 2 | 3;

interface ScenarioDefinition {
  readonly id: 'basic' | 'multi';
  readonly label: string;
  readonly systemPrompt: string;
  readonly userPrompt: string;
  readonly reasoning?: ReasoningLevel;
  readonly minTurns: number;
  readonly requiredAgentTools?: readonly string[];
  readonly requireReasoningSignatures?: boolean;
}

interface ScenarioVariant {
  readonly scenario: ScenarioDefinition;
  readonly stream: boolean;
}

interface ScenarioRunResult {
  readonly modelLabel: string;
  readonly provider: string;
  readonly modelId: string;
  readonly tier: Tier;
  readonly scenarioId: string;
  readonly scenarioLabel: string;
  readonly stream: boolean;
  readonly reasoning?: ReasoningLevel;
  readonly success: boolean;
  readonly failureReasons: readonly string[];
  readonly error?: string;
  readonly accounting: readonly AccountingEntry[];
  readonly maxTurn?: number;
  readonly reasoningSummary?: {
    readonly requested: boolean;
    readonly present: boolean;
    readonly preserved: boolean;
  };
}

interface AccountingSummary {
  readonly tokensIn: number;
  readonly tokensOut: number;
  readonly totalTokens: number;
  readonly cacheReadTokens: number;
  readonly cacheWriteTokens: number;
  readonly requests: number;
  readonly latencyMs: number;
  readonly costUsd?: number;
}

const PROJECT_ROOT = process.cwd();
const TEST_AGENTS_DIR = path.resolve(PROJECT_ROOT, 'src', 'tests', 'phase2', 'test-agents');
const MASTER_AGENT_PATH = path.join(TEST_AGENTS_DIR, 'test-master.ai');
const DEFAULT_CONFIG_PATH = path.resolve(PROJECT_ROOT, 'neda/.ai-agent.json');
const FINAL_REPORT_INSTRUCTION = 'Call agent__final_report with status="success", report_format="text", and content set to the required text.';
const FINAL_REPORT_ARGS = 'agent__final_report(status="success", report_format="text", content=<value>)';
const BASIC_PROMPT = `${FINAL_REPORT_INSTRUCTION} The content must be exactly "test".`;
const BASIC_USER = `Invoke ${FINAL_REPORT_ARGS.replace('<value>', '"test"')} and produce no other output.`;
const MULTI_PROMPT = `You are a helpful CI tester helping the verification of multi-turn agentic operation.

You have 2 agents: agent1 and agent2. The test completes in 3 turns.

## Turns
1. You call just agent1 with the parameter "run"
2. You call just agent2 with the output of agent1
3. You provide your final-report with the output of agent2

For the test to complete successfully, you must call each agent alone, without any other tools.

## Process
Follow exactly these steps for a successful outcome:

1. Call ONLY "agent__test-agent1" with parameters {"prompt":"run","reason":"execute agent1","format":"sub-agent"}.
2. Once agent1 responds and you have its output, call ONLY "agent__test-agent2" with {"prompt":"I received this from agent1: [agent1 response]","reason":"execute agent2","format":"sub-agent"}.
3. Once agent2 responds and you have its output, call ONLY agent__final_report with status="success", report_format="text", and content="I received this from agent2: [agent2 response]".

Do not emit plain text.`;
const MULTI_USER = `CI verification scenario: follow the multi-turn process exactly.

1) Call ONLY "agent__test-agent1" with parameters {"prompt":"run","reason":"execute agent1","format":"sub-agent"}.
2) After agent__test-agent1 responds, call ONLY "agent__test-agent2" with {"prompt":"I received this from agent1: [agent1 response]","reason":"execute agent2","format":"sub-agent"}.
3) After agent__test-agent2 responds, call ONLY agent__final_report with status="success", report_format="text", and content="I received this from agent2: [agent2 response]".

If you skip a step or call any other tool, the CI test fails.`;

const REQUIRED_AGENT_TOOLS: readonly string[] = ['agent__test-agent1', 'agent__test-agent2'] as const;

const BASE_SCENARIOS: readonly ScenarioDefinition[] = [
  {
    id: 'basic',
    label: 'basic-llm',
    systemPrompt: BASIC_PROMPT,
    userPrompt: BASIC_USER,
    minTurns: 1,
  },
  {
    id: 'multi',
    label: 'multi-turn',
    systemPrompt: MULTI_PROMPT,
    userPrompt: MULTI_USER,
    reasoning: 'high',
    minTurns: 3,
    requiredAgentTools: REQUIRED_AGENT_TOOLS,
    requireReasoningSignatures: true,
  },
] as const;

const STREAM_VARIANTS: readonly boolean[] = [false, true] as const;
const STOP_ON_FAILURE = process.env.PHASE2_STOP_ON_FAILURE === '1';
const TRACE_LLM = process.env.PHASE2_TRACE_LLM === '1';
const TRACE_SDK = process.env.PHASE2_TRACE_SDK === '1';
const TRACE_MCP = process.env.PHASE2_TRACE_MCP === '1';
const VERBOSE_LOGS = process.env.PHASE2_VERBOSE === '1';
const STREAM_ON_LABEL = 'stream-on';
const STREAM_OFF_LABEL = 'stream-off';
const TEMP_DISABLED_PROVIDERS = new Set<string>();
const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

const ensureFileExists = (description: string, candidate: string): void => {
  if (!fs.existsSync(candidate)) {
    throw new Error(`${description} not found at ${candidate}`);
  }
};

const parseTierFilter = (value: string): Set<Tier> => {
  const parts = value.split(',').map((part) => part.trim()).filter((part) => part.length > 0);
  if (parts.length === 0) {
    throw new Error('tier list cannot be empty');
  }
  const tiers = new Set<Tier>();
  parts.forEach((part) => {
    const parsed = Number.parseInt(part, 10);
    if (parsed !== 1 && parsed !== 2 && parsed !== 3) {
      throw new Error(`invalid tier value '${part}'`);
    }
    tiers.add(parsed as Tier);
  });
  return tiers;
};

const parseModelFilter = (value: string): Set<string> => {
  const parts = value.split(',').map((part) => part.trim()).filter((part) => part.length > 0);
  if (parts.length === 0) {
    throw new Error('model list cannot be empty');
  }
  return new Set(parts);
};

const scenarioVariants: readonly ScenarioVariant[] = (() => {
  const variants: ScenarioVariant[] = [];
  BASE_SCENARIOS.forEach((scenario) => {
    STREAM_VARIANTS.forEach((stream) => {
      variants.push({ scenario, stream });
    });
  });
  return variants;
})();

const buildAccountingSummary = (entries: readonly AccountingEntry[]): AccountingSummary => {
  let tokensIn = 0;
  let tokensOut = 0;
  let totalTokens = 0;
  let cacheReadTokens = 0;
  let cacheWriteTokens = 0;
  let latencyMs = 0;
  let requests = 0;
  let costUsd: number | undefined;
  entries.forEach((entry) => {
    if (entry.type === 'llm') {
      tokensIn += entry.tokens.inputTokens;
      tokensOut += entry.tokens.outputTokens;
      totalTokens += entry.tokens.totalTokens;
      const read = entry.tokens.cacheReadInputTokens ?? entry.tokens.cachedTokens ?? 0;
      const write = entry.tokens.cacheWriteInputTokens ?? 0;
      cacheReadTokens += read;
      cacheWriteTokens += write;
      latencyMs += entry.latency;
      requests += 1;
      if (typeof entry.costUsd === 'number') {
        costUsd = (costUsd ?? 0) + entry.costUsd;
      }
      if (typeof entry.upstreamInferenceCostUsd === 'number') {
        costUsd = (costUsd ?? 0) + entry.upstreamInferenceCostUsd;
      }
    }
  });
  if (costUsd !== undefined) {
    return {
      tokensIn,
      tokensOut,
      totalTokens,
      cacheReadTokens,
      cacheWriteTokens,
      requests,
      latencyMs,
      costUsd,
    };
  }
  return {
    tokensIn,
    tokensOut,
    totalTokens,
    cacheReadTokens,
    cacheWriteTokens,
    requests,
    latencyMs,
  };
};

const extractMaxLlmTurn = (logs: readonly LogEntry[]): number => {
  const turns = logs
    .filter((log) => log.type === 'llm' && log.direction === 'response')
    .map((log) => log.turn);
  if (turns.length === 0) return 0;
  return Math.max(...turns);
};

const normalizeToolName = (toolName: string): string => sanitizeToolName(toolName);

const findToolIndex = (logs: readonly LogEntry[], toolName: string): number => {
  const normalized = normalizeToolName(toolName);
  return logs.findIndex((log) => log.type === 'tool' && log.direction === 'request' && normalizeToolName(String(log.details?.tool ?? '')) === normalized);
};

const collectConversationToolCalls = (conversation: readonly ConversationMessage[]): Map<string, string> => {
  const map = new Map<string, string>();
  conversation.forEach((message) => {
    if (message.role !== 'assistant') return;
    const rawCalls = (message as { toolCalls?: unknown }).toolCalls;
    if (!Array.isArray(rawCalls)) return;
    rawCalls.forEach((call) => {
      if (call === null || typeof call !== 'object') return;
      const callObj = call as { id?: unknown; name?: unknown };
      const id = typeof callObj.id === 'string' && callObj.id.length > 0 ? callObj.id : undefined;
      const name = typeof callObj.name === 'string' ? callObj.name : undefined;
      if (id === undefined || name === undefined) return;
      map.set(id, normalizeToolName(name));
    });
  });
  return map;
};

const conversationHasToolCall = (conversation: readonly ConversationMessage[], toolName: string): boolean => {
  const normalized = normalizeToolName(toolName);
  const callMap = collectConversationToolCalls(conversation);
  if (callMap.size === 0) return false;
  return conversation.some((message) => {
    if (message.role !== 'tool') return false;
    const toolCallId = (message as { toolCallId?: unknown }).toolCallId;
    if (typeof toolCallId !== 'string' || toolCallId.length === 0) return false;
    return callMap.get(toolCallId) === normalized;
  });
};

const conversationToolOrder = (conversation: readonly ConversationMessage[], toolName: string): number => {
  const normalized = normalizeToolName(toolName);
  let order = 0;
  // eslint-disable-next-line functional/no-loop-statements -- imperative scan is clearer for ordered assistant turns
  for (const message of conversation) {
    if (message.role !== 'assistant') continue;
    const rawCalls = (message as { toolCalls?: unknown }).toolCalls;
    if (!Array.isArray(rawCalls)) continue;
    const hasMatch = rawCalls.some((call) => {
      if (call === null || typeof call !== 'object') return false;
      const nameValue = (call as { name?: unknown }).name;
      if (typeof nameValue !== 'string' || nameValue.length === 0) return false;
      return normalizeToolName(nameValue) === normalized;
    });
    if (hasMatch) {
      return order;
    }
    order += 1;
  }
  return -1;
};

const hasToolInvocation = (
  logs: readonly LogEntry[],
  conversation: readonly ConversationMessage[],
  toolName: string,
): boolean => {
  if (findToolIndex(logs, toolName) !== -1) return true;
  return conversationHasToolCall(conversation, toolName);
};

const hasAccountingEntries = (entries: readonly AccountingEntry[]): boolean => {
  return entries.some((entry) => entry.type === 'llm');
};

const isAssistantWithReasoning = (
  message: ConversationMessage
): message is ConversationMessage & { reasoning: NonNullable<ConversationMessage['reasoning']> } => {
  return message.role === 'assistant' && Array.isArray(message.reasoning) && message.reasoning.length > 0;
};

interface ReasoningSignatureStatus {
  readonly present: boolean;
  readonly preserved: boolean;
}

const analyzeReasoningSignatures = (messages: readonly ConversationMessage[]): ReasoningSignatureStatus => {
  const reasoningMessages = messages.filter(isAssistantWithReasoning);
  if (reasoningMessages.length === 0) {
    return { present: false, preserved: false };
  }

  const firstMessage = reasoningMessages[0];
  const firstHasSignature = firstMessage.reasoning.some((segment) => {
    const candidate = segment as { signature?: unknown };
    return typeof candidate.signature === 'string' && candidate.signature.length > 0;
  });
  const othersHaveSignature = reasoningMessages.slice(1).every((message) => {
    return message.reasoning.some((segment) => {
      const candidate = segment as { signature?: unknown };
      return typeof candidate.signature === 'string' && candidate.signature.length > 0;
    });
  });
  const preserved = firstHasSignature && othersHaveSignature;
  return { present: true, preserved };
};

const validateScenarioResult = (
  result: ScenarioRunResult,
  runResult: { result?: Awaited<ReturnType<ReturnType<typeof loadAgent>['run']>>; error?: string }
): ScenarioRunResult => {
  if (runResult.error !== undefined) {
    return {
      ...result,
      success: false,
      failureReasons: [...result.failureReasons, runResult.error],
      error: runResult.error,
    };
  }
  const session = runResult.result;
  if (session === undefined) {
    const reason = 'missing session result';
    return { ...result, success: false, failureReasons: [...result.failureReasons, reason], error: reason };
  }

  const failures: string[] = [];
  if (!session.success) {
    failures.push(`session unsuccessful: ${session.error ?? 'unknown error'}`);
  }
  if (session.finalReport === undefined) {
    failures.push('final report missing or not successful');
  }
  if (!hasToolInvocation(session.logs, session.conversation, 'agent__final_report')) {
    failures.push('final report tool was not invoked');
  }
  const maxTurn = extractMaxLlmTurn(session.logs);
  const accountingOk = hasAccountingEntries(session.accounting);
  if (!accountingOk) {
    failures.push('no llm accounting entries recorded');
  }

  const scenario = BASE_SCENARIOS.find((candidate) => candidate.id === result.scenarioId);
  let reasoningSummary: ScenarioRunResult['reasoningSummary'];
  const reasoningRequested = scenario?.reasoning !== undefined;
  if (scenario !== undefined) {
    if (maxTurn < scenario.minTurns) {
      failures.push(`expected at least ${String(scenario.minTurns)} turns, observed ${String(maxTurn)}`);
    }
    if (Array.isArray(scenario.requiredAgentTools)) {
      const tools = scenario.requiredAgentTools.filter((value): value is string => typeof value === 'string');
      const toolInvocationStatus = tools.map((toolName) => ({
        name: toolName,
        logIndex: findToolIndex(session.logs, toolName),
        conversationIndex: conversationToolOrder(session.conversation, toolName),
        invoked: hasToolInvocation(session.logs, session.conversation, toolName),
      }));
      toolInvocationStatus.forEach((status) => {
        if (!status.invoked) {
          failures.push(`required sub-agent tool '${status.name}' was not invoked`);
        }
      });
      if (toolInvocationStatus.every((status) => status.invoked) && tools.length >= 2) {
        const first = toolInvocationStatus[0];
        const second = toolInvocationStatus[1];
        const firstOrder = first.logIndex >= 0 ? first.logIndex : first.conversationIndex;
        const secondOrder = second.logIndex >= 0 ? second.logIndex : second.conversationIndex;
        if (firstOrder >= 0 && secondOrder >= 0 && firstOrder > secondOrder) {
          failures.push('agent2 ran before agent1');
        }
      }
    }
    if (reasoningRequested) {
      const { present, preserved } = analyzeReasoningSignatures(session.conversation);
      reasoningSummary = { requested: true, present, preserved };
      if (scenario.requireReasoningSignatures === true && result.provider === 'anthropic') {
        if (!present) {
          console.warn(
            `[WARN] ${result.modelLabel} :: ${scenario.label} :: requested reasoning but provider returned no reasoning segments`
          );
        } else if (!preserved) {
          failures.push('reasoning signatures missing or not preserved across turns');
          const assistantReasoning = session.conversation
            .filter(isAssistantWithReasoning)
            .map((message, index) => {
              const segments = message.reasoning;
              const signatureFlags = segments.map((segment) => {
                const signature = (segment as unknown as { signature?: unknown }).signature;
                return typeof signature === 'string';
              });
              return {
                index,
                segments: segments.length,
                signatures: signatureFlags,
                raw: segments.map((segment) => ({
                  keys: Object.keys(segment as unknown as Record<string, unknown>),
                  providerMetadata: segment.providerMetadata,
                  textSnippet: segment.text.slice(0, 80),
                })),
              };
            });
          console.warn(
            `[WARN] ${result.modelLabel} :: ${scenario.label} :: reasoning signature map ${JSON.stringify(assistantReasoning)}`
          );
        }
      }
    }
  }

  reasoningSummary ??= { requested: reasoningRequested, present: false, preserved: false };

  return {
    ...result,
    success: failures.length === 0,
    failureReasons: failures,
    accounting: session.accounting,
    maxTurn,
    reasoningSummary,
  };
};

const runScenarioVariant = async (
  configPath: string,
  modelLabel: string,
  provider: string,
  modelId: string,
  tier: Tier,
  variant: ScenarioVariant
): Promise<ScenarioRunResult> => {
  const { scenario, stream } = variant;
  const cache = new LoadedAgentCache();
  const overrides: GlobalOverrides = {
    models: [{ provider, model: modelId }],
  };
  if (scenario.reasoning !== undefined) {
    overrides.reasoning = scenario.reasoning;
  }
  const baseResult: ScenarioRunResult = {
    modelLabel,
    provider,
    modelId,
    tier,
    scenarioId: scenario.id,
    scenarioLabel: scenario.label,
    stream,
    reasoning: scenario.reasoning,
    success: false,
    failureReasons: [],
    accounting: [],
    maxTurn: 0,
  };

  try {
    const agent = loadAgent(MASTER_AGENT_PATH, cache, {
      configPath,
      globalOverrides: overrides,
      stream,
      reasoning: scenario.reasoning,
    });
    const callbacks = VERBOSE_LOGS
      ? {
          onLog: (entry: LogEntry) => {
            const turnSegment = Number.isFinite(entry.turn) ? ` turn=${String(entry.turn)}` : '';
            console.log(`[LOG] ${entry.severity} ${entry.remoteIdentifier}${turnSegment}: ${entry.message}`);
          },
        }
      : undefined;
    const result = await agent.run(scenario.systemPrompt, scenario.userPrompt, {
      outputFormat: 'markdown',
      callbacks,
      traceLLM: TRACE_LLM,
      traceSdk: TRACE_SDK,
      traceMCP: TRACE_MCP,
      verbose: VERBOSE_LOGS,
    });
    return validateScenarioResult(baseResult, { result });
  } catch (error: unknown) {
    const message = toErrorMessage(error);
    return { ...baseResult, success: false, failureReasons: [message], error: message };
  }
};

const describeReasoningStatus = (run: ScenarioRunResult): string => {
  const summary = run.reasoningSummary;
  const requested = summary?.requested ?? false;
  if (!requested) {
    return 'not-requested';
  }
  const present = summary?.present ?? false;
  if (!present) {
    return 'requested-absent';
  }
  const preserved = summary?.preserved ?? false;
  return preserved ? 'preserved' : 'missing-signatures';
};

const formatRunMetrics = (run: ScenarioRunResult, summaryOverride?: AccountingSummary): string => {
  const accounting = summaryOverride ?? buildAccountingSummary(run.accounting);
  const reasoning = describeReasoningStatus(run);
  const latencyPart = `latency=${String(accounting.latencyMs)}ms`;
  const requestsPart = accounting.requests > 0 ? `, requests=${String(accounting.requests)}` : '';
  const tokensPart = `tokens[in=${String(accounting.tokensIn)}, out=${String(accounting.tokensOut)}, cacheR=${String(accounting.cacheReadTokens)}, cacheW=${String(accounting.cacheWriteTokens)}, total=${String(accounting.totalTokens)}]`;
  const costPart = accounting.costUsd !== undefined ? `, costUsd≈${accounting.costUsd.toFixed(4)}` : '';
  return `reasoning=${reasoning} | ${latencyPart}${requestsPart} | ${tokensPart}${costPart}`;
};

const formatSummaryLine = (run: ScenarioRunResult, summaryOverride?: AccountingSummary): string => {
  const outcome = run.success ? 'PASS' : 'FAIL';
  const streamLabel = run.stream ? STREAM_ON_LABEL : STREAM_OFF_LABEL;
  return `[${outcome}] ${run.modelLabel} (${run.provider}:${run.modelId}) :: ${run.scenarioLabel} :: ${streamLabel} | ${formatRunMetrics(run, summaryOverride)}`;
};

const printSummary = (runs: readonly ScenarioRunResult[]): void => {
  console.log('Phase 2 Integration Summary');
  let totalTokensIn = 0;
  let totalTokensOut = 0;
  let totalCacheRead = 0;
  let totalCacheWrite = 0;
  let totalTokens = 0;
  let totalLatency = 0;
  let totalRequests = 0;
  let totalCost = 0;
  let costObserved = false;
  // eslint-disable-next-line functional/no-loop-statements
  for (const run of runs) {
    const summary = buildAccountingSummary(run.accounting);
    console.log(formatSummaryLine(run, summary));
    if (!run.success) {
      run.failureReasons.forEach((reason) => {
        console.log(`  - ${reason}`);
      });
    }
    totalTokensIn += summary.tokensIn;
    totalTokensOut += summary.tokensOut;
    totalCacheRead += summary.cacheReadTokens;
    totalCacheWrite += summary.cacheWriteTokens;
    totalTokens += summary.totalTokens;
    totalLatency += summary.latencyMs;
    totalRequests += summary.requests;
    if (summary.costUsd !== undefined) {
      totalCost += summary.costUsd;
      costObserved = true;
    }
  }
  const totalsLine = [
    `tokens[in=${String(totalTokensIn)}, out=${String(totalTokensOut)}, cacheR=${String(totalCacheRead)}, cacheW=${String(totalCacheWrite)}, total=${String(totalTokens)}]`,
    `latency=${String(totalLatency)}ms`,
    `requests=${String(totalRequests)}`,
  ];
  if (costObserved) {
    totalsLine.push(`costUsd≈${totalCost.toFixed(4)}`);
  }
  console.log(`Overall Totals: ${totalsLine.join(' | ')}`);
};

const printUsage = (): void => {
  console.log('Usage: node dist/tests/phase2-runner.js [--config=path] [--tier=1,2,3] [--model=label,modelId]');
  console.log('Environment override: PHASE2_CONFIG=/path/to/config.json');
  console.log('Set PHASE2_STOP_ON_FAILURE=1 to halt immediately after the first failure (default: continue running all models).');
};

async function main(): Promise<void> {
  ensureFileExists('Master agent prompt', MASTER_AGENT_PATH);
  const argv = process.argv.slice(2);
  if (argv.includes('--help') || argv.includes('-h')) {
    printUsage();
    return;
  }

  let configPath = process.env.PHASE2_CONFIG ?? DEFAULT_CONFIG_PATH;
  let tierFilter: Set<Tier> | undefined;
  let tierThreshold: number | undefined;
  let modelFilter: Set<string> | undefined;

  argv.forEach((arg) => {
    if (arg.startsWith('--config=')) {
      configPath = path.resolve(arg.slice('--config='.length));
    } else if (arg.startsWith('--tier=')) {
      tierFilter = parseTierFilter(arg.slice('--tier='.length));
      tierThreshold = Math.max(...Array.from(tierFilter));
    } else if (arg.startsWith('--model=')) {
      modelFilter = parseModelFilter(arg.slice('--model='.length));
    } else {
      throw new Error(`unknown argument '${arg}'`);
    }
  });

  ensureFileExists('Configuration file', configPath);

  const runs: ScenarioRunResult[] = [];
  let abort = false;
  let executed = 0;
  // eslint-disable-next-line functional/no-loop-statements
  for (const model of phase2ModelConfigs) {
    if (abort) break;
    if (TEMP_DISABLED_PROVIDERS.has(model.provider)) {
      console.log(`[SKIP] ${model.label} (${model.provider}:${model.modelId}) disabled temporarily`);
      continue;
    }
    if (tierThreshold !== undefined && model.tier > tierThreshold) continue;
    if (modelFilter !== undefined) {
      const matches = modelFilter.has(model.label) || modelFilter.has(`${model.provider}/${model.modelId}`) || modelFilter.has(model.modelId);
      if (!matches) continue;
    }
    // eslint-disable-next-line functional/no-loop-statements
    for (const variant of scenarioVariants) {
      executed += 1;
      const streamLabel = variant.stream ? STREAM_ON_LABEL : STREAM_OFF_LABEL;
      const runIndexLabel = String(executed);
      const tierLabel = String(model.tier);
      console.log(`[RUN] #${runIndexLabel} ${model.label} (${model.provider}:${model.modelId}, tier ${tierLabel}) :: ${variant.scenario.label} :: ${streamLabel}`);
      const run = await runScenarioVariant(configPath, model.label, model.provider, model.modelId, model.tier, variant);
      runs.push(run);
      const summary = buildAccountingSummary(run.accounting);
      const outcomeLine = formatSummaryLine(run, summary);
      console.log(outcomeLine);
      if (!run.success) {
        run.failureReasons.forEach((reason) => {
          console.log(`  - ${reason}`);
        });
      }
      if (STOP_ON_FAILURE && !run.success) {
        abort = true;
        break;
      }
    }
  }

  printSummary(runs);
  if (runs.some((run) => !run.success)) {
    process.exitCode = 1;
  }
}

main().catch((error: unknown) => {
  console.error('phase2-runner failed', toErrorMessage(error));
  process.exit(1);
});
