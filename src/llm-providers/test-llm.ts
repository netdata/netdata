import type { ConversationMessage, ProviderConfig, TokenUsage, TurnRequest, TurnResult } from '../types.js';
import type { LanguageModelV2, LanguageModelV2CallOptions, LanguageModelV2Content, LanguageModelV2FinishReason, LanguageModelV2StreamPart, LanguageModelV2Usage } from '@ai-sdk/provider';

import { getScenario, listScenarioIds, type ScenarioDefinition, type ScenarioStepResponse, type ScenarioTurn } from '../tests/fixtures/test-llm-scenarios.js';
import { sanitizeToolName, truncateUtf8WithNotice } from '../utils.js';

import { BaseLLMProvider, type ResponseMessage } from './base.js';

const PROVIDER_NAME = 'test-llm';
const MARKDOWN_FORMAT = 'markdown' as const;
const TOOL_CALL_KIND = 'tool-call' as const;
const FINAL_REPORT_KIND = 'final-report' as const;

type ToolCallStep = Extract<ScenarioStepResponse, { kind: typeof TOOL_CALL_KIND }>;
type FinalReportStep = Extract<ScenarioStepResponse, { kind: typeof FINAL_REPORT_KIND }>;

interface StepContext {
  scenario: ScenarioDefinition;
  step: ScenarioTurn;
  scenarioId: string;
  turn: number;
  modelId: string;
}

interface ErrorDescriptor {
  message: string;
}

export class TestLLMProvider extends BaseLLMProvider {
  name = PROVIDER_NAME;
  private readonly attemptCounters = new Map<string, number>();

  constructor(_config: ProviderConfig) {
    super();
  }

  protected convertResponseMessages(messages: ResponseMessage[], provider: string, model: string, tokens: TokenUsage): ConversationMessage[] {
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }

  async executeTurn(request: TurnRequest): Promise<TurnResult> {
    const executionStart = Date.now();
    const scenarioId = this.extractScenarioId(request.messages);
    if (scenarioId === undefined) {
      return await this.createErrorTurn(request, 'No scenario command (e.g., "run-test-1") found in user messages.');
    }

    const scenario = getScenario(scenarioId);
    if (scenario === undefined) {
      return await this.createErrorTurn(request, `Unknown scenario '${scenarioId}'. Available: ${truncateUtf8WithNotice(listScenarioIds().join(', '), 2000)}`);
    }

    const validationError = this.validateScenarioContext(scenario, request);
    if (validationError !== undefined) {
      return await this.createErrorTurn(request, validationError.message, scenarioId);
    }

    const turn = this.computeTurnNumber(request.messages);
    const matchedStep = scenario.turns.find((entry) => entry.turn === turn);
    const activeStep = matchedStep ?? this.buildFallbackStep(turn, `Unexpected turn ${String(turn)} for scenario '${scenario.id}'.`);

    const errorFromTools = this.validateTools(activeStep, request);
    if (errorFromTools !== undefined) {
      return await this.createErrorTurn(request, errorFromTools.message, scenarioId);
    }

    const expectedTemperature = activeStep.expectedTemperature;
    if (expectedTemperature !== undefined) {
      const actualTemperature = request.temperature;
      if (typeof actualTemperature !== 'number' || Math.abs(actualTemperature - expectedTemperature) > 1e-6) {
        const expectedTempStr = String(expectedTemperature);
        const actualTempStr = typeof actualTemperature === 'number' ? String(actualTemperature) : 'undefined';
        return await this.createErrorTurn(request, `Expected temperature ${expectedTempStr} but received ${actualTempStr}.`, scenarioId);
      }
    }

    const expectedTopP = activeStep.expectedTopP;
    if (expectedTopP !== undefined) {
      const actualTopP = request.topP;
      if (typeof actualTopP !== 'number' || Math.abs(actualTopP - expectedTopP) > 1e-6) {
        const expectedTopPStr = String(expectedTopP);
        const actualTopPStr = typeof actualTopP === 'number' ? String(actualTopP) : 'undefined';
        return await this.createErrorTurn(request, `Expected topP ${expectedTopPStr} but received ${actualTopPStr}.`, scenarioId);
      }
    }

    const context: StepContext = {
      scenario,
      step: activeStep,
      scenarioId,
      turn,
      modelId: request.model,
    };

    const providerKey = request.provider;
    const attemptKey = `${scenarioId}:${String(activeStep.turn)}:${providerKey}`;
    const attemptCount = this.attemptCounters.get(attemptKey) ?? 0;
    const failuresAllowed = typeof activeStep.failuresBeforeSuccess === 'number' ? Math.max(0, Math.trunc(activeStep.failuresBeforeSuccess)) : 0;
    if (attemptCount < failuresAllowed) {
      this.attemptCounters.set(attemptKey, attemptCount + 1);
      const latency = Date.now() - executionStart;
      const failureStatus = activeStep.failureStatus ?? 'model_error';
      const failureMessage = activeStep.failureMessage ?? 'Simulated failure';
      const retryable = activeStep.failureRetryable !== false;
      switch (failureStatus) {
        case 'timeout':
          return {
            status: { type: 'timeout', message: failureMessage },
            latencyMs: latency,
            messages: [],
          };
        case 'invalid_response':
          return {
            status: { type: 'invalid_response', message: failureMessage },
            latencyMs: latency,
            messages: [],
          };
        case 'network_error':
          return {
            status: { type: 'network_error', message: failureMessage, retryable },
            latencyMs: latency,
            messages: [],
          };
        case 'rate_limit':
          return {
            status: {
              type: 'rate_limit',
              retryAfterMs: activeStep.failureRetryAfterMs,
            },
            latencyMs: latency,
            messages: [],
          };
        case 'model_error':
        default:
          return {
            status: { type: 'model_error', message: failureMessage, retryable },
            latencyMs: latency,
            messages: [],
          };
      }
    }
    this.attemptCounters.set(attemptKey, attemptCount);

    const model = createScenarioLanguageModel(context);
    const filteredTools = this.filterToolsForFinalTurn(request.tools, request.isFinalTurn === true);
    const tools = this.convertTools(filteredTools, request.toolExecutor);
    const messages = super.convertMessages(request.messages);
    const finalMessages = this.buildFinalTurnMessages(messages, request.isFinalTurn === true);

    const start = Date.now();
    if (request.stream === true) {
      return await super.executeStreamingTurn(model, finalMessages, tools, request, start, undefined);
    }
    return await super.executeNonStreamingTurn(model, finalMessages, tools, request, start, undefined);
  }

  private extractScenarioId(messages: ConversationMessage[]): string | undefined {
    const userMessages = messages.filter((message) => message.role === 'user');
    const firstUser = userMessages.at(0);
    if (firstUser === undefined) return undefined;
    const trimmed = firstUser.content.trim();
    return trimmed.length > 0 ? trimmed : undefined;
  }

  private computeTurnNumber(messages: ConversationMessage[]): number {
    const assistantMessages = messages.filter((message) => message.role === 'assistant').length;
    return assistantMessages + 1;
  }

  private validateScenarioContext(scenario: ScenarioDefinition, request: TurnRequest): ErrorDescriptor | undefined {
    const systemPrompt = request.messages.find((message) => message.role === 'system');
    const requiredFragments = Array.isArray(scenario.systemPromptMustInclude) ? scenario.systemPromptMustInclude : [];
    if (requiredFragments.length === 0) return undefined;
    const systemContent = systemPrompt?.content ?? '';
    const missing = requiredFragments.filter((fragment) => !systemContent.includes(fragment));
    if (missing.length === 0) return undefined;
    return { message: `System prompt missing required fragments: ${missing.join(', ')}` };
  }

  private buildFallbackStep(turn: number, message: string): ScenarioTurn {
    return {
      turn,
      response: {
        kind: FINAL_REPORT_KIND,
        assistantText: 'Encountered scenario mismatch. Returning failure report.',
        reportContent: `# Scenario Failure\n\n${message}`,
        reportFormat: MARKDOWN_FORMAT,
        status: 'failure',
      },
    };
  }

  private validateTools(step: ScenarioTurn, request: TurnRequest): ErrorDescriptor | undefined {
    if (step.allowMissingTools === true) return undefined;
    const expected = Array.isArray(step.expectedTools) ? step.expectedTools : [];
    if (step.response.kind === FINAL_REPORT_KIND || expected.length === 0) return undefined;
    const available = new Set(request.tools.map((tool) => sanitizeToolName(tool.name)));
    const missing = expected
      .map((toolName) => sanitizeToolName(toolName))
      .filter((toolName) => !toolName.startsWith('agent__'))
      .filter((toolName) => !available.has(toolName));
    if (missing.length === 0) return undefined;
    return { message: `Expected tool(s) not available in request: ${missing.join(', ')}` };
  }

  private async createErrorTurn(request: TurnRequest, message: string, scenarioId?: string): Promise<TurnResult> {
    const fallback: ScenarioTurn = {
      turn: this.computeTurnNumber(request.messages),
      response: {
        kind: FINAL_REPORT_KIND,
        assistantText: 'Scenario validation failed. Returning error report.',
        reportContent: `# Test Harness Error\n\n${message}`,
        reportFormat: MARKDOWN_FORMAT,
        status: 'failure',
      },
    };

    const context: StepContext = {
      scenario: {
        id: scenarioId ?? 'unknown-scenario',
        description: 'Dynamic error fallback scenario',
        turns: [fallback],
      },
      step: fallback,
      scenarioId: scenarioId ?? 'unknown-scenario',
      turn: fallback.turn,
      modelId: request.model,
    };

    const model = createScenarioLanguageModel(context);
    const tools = this.convertTools([], request.toolExecutor);
    const messages = super.convertMessages(request.messages);
    const finalMessages = this.buildFinalTurnMessages(messages, true);
    const start = Date.now();
    return await super.executeNonStreamingTurn(model, finalMessages, tools, request, start, undefined);
  }
}

function createScenarioLanguageModel(context: StepContext): LanguageModelV2 {
  const { step } = context;
  return {
    specificationVersion: 'v2',
    provider: PROVIDER_NAME,
    modelId: context.modelId,
    supportedUrls: {},
    doGenerate(_options: LanguageModelV2CallOptions) {
      return Promise.resolve(buildResponse(step.response));
    },
    doStream(_options: LanguageModelV2CallOptions) {
      const response = buildResponse(step.response);
      const parts = convertContentToStream(response.content, response.finishReason, response.usage);
      const stream = new ReadableStream<LanguageModelV2StreamPart>({
        start(controller) {
          parts.forEach((part) => {
            controller.enqueue(part);
          });
          controller.close();
        },
      });
      return Promise.resolve({ stream });
    },
  };
}

function buildResponse(response: ScenarioStepResponse) {
  const usage: LanguageModelV2Usage = response.tokenUsage ?? {
    inputTokens: 64,
    outputTokens: 32,
    totalTokens: 96,
  };
  if (response.kind === TOOL_CALL_KIND) {
    const content: LanguageModelV2Content[] = buildToolCallContent(response);
    appendReasoningParts(content, response);
    return {
      content,
      finishReason: (response.finishReason ?? 'tool-calls') as LanguageModelV2FinishReason,
      usage,
      warnings: [],
    };
  }
  if (response.kind === FINAL_REPORT_KIND) {
    const content: LanguageModelV2Content[] = buildFinalReportContent(response);
    appendReasoningParts(content, response);
    return {
      content,
      finishReason: 'stop' as LanguageModelV2FinishReason,
      usage,
      warnings: [],
    };
  }
  const content: LanguageModelV2Content[] = [];
  if (response.assistantText.trim().length > 0) {
    content.push({ type: 'text', text: response.assistantText });
  }
  appendReasoningParts(content, response);
  return {
    content,
    finishReason: (response.finishReason ?? 'other') as LanguageModelV2FinishReason,
    usage,
    warnings: [],
  };
}

function buildToolCallContent(response: ToolCallStep): LanguageModelV2Content[] {
  const items: LanguageModelV2Content[] = [];
  if (typeof response.assistantText === 'string' && response.assistantText.trim().length > 0) {
    items.push({ type: 'text', text: response.assistantText });
  }
  response.toolCalls.forEach((call, index) => {
    const callId = call.callId ?? `call-${String(index + 1)}`;
    if (call.assistantText !== undefined && call.assistantText.trim().length > 0) {
      items.push({ type: 'text', text: call.assistantText });
    }
    items.push({
      type: 'tool-call',
      toolCallId: callId,
      toolName: call.toolName,
      input: JSON.stringify(call.arguments),
    });
  });
  return items;
}

function buildFinalReportContent(response: FinalReportStep): LanguageModelV2Content[] {
  const items: LanguageModelV2Content[] = [];
  if (typeof response.assistantText === 'string' && response.assistantText.trim().length > 0) {
    items.push({ type: 'text', text: response.assistantText });
  }
  items.push({
    type: 'tool-call',
    toolCallId: 'agent-final-report',
    toolName: 'agent__final_report',
    input: JSON.stringify({
      status: response.status ?? 'success',
      report_format: response.reportFormat,
      report_content: response.reportContent,
    }),
  });
  return items;
}

function appendReasoningParts(content: LanguageModelV2Content[], response: ScenarioStepResponse): void {
  if (!Array.isArray(response.reasoning) || response.reasoning.length === 0) return;
  response.reasoning.forEach((entry) => {
    if (typeof entry === 'string' && entry.trim().length > 0) {
      content.push({ type: 'reasoning', text: entry });
    }
  });
}

function convertContentToStream(
  content: LanguageModelV2Content[],
  finishReason: LanguageModelV2FinishReason,
  usage: LanguageModelV2Usage,
): LanguageModelV2StreamPart[] {
  const parts: LanguageModelV2StreamPart[] = [
    { type: 'stream-start', warnings: [] },
  ];
  let textIndex = 0;
  let reasoningIndex = 0;
  content.forEach((entry) => {
    if (entry.type === 'text') {
      textIndex += 1;
      const id = `text-${String(textIndex)}`;
      parts.push({ type: 'text-start', id });
      parts.push({ type: 'text-delta', id, delta: entry.text });
      parts.push({ type: 'text-end', id });
      return;
    }
    if (entry.type === 'reasoning') {
      reasoningIndex += 1;
      const id = `reasoning-${String(reasoningIndex)}`;
      parts.push({ type: 'reasoning-start', id });
      parts.push({ type: 'reasoning-delta', id, delta: entry.text });
      parts.push({ type: 'reasoning-end', id });
      return;
    }
    if (entry.type === TOOL_CALL_KIND || entry.type === 'tool-result') {
      parts.push(entry);
    }
  });
  parts.push({ type: 'finish', finishReason, usage });
  return parts;
}
