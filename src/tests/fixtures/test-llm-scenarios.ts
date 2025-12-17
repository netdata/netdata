import type { ProviderTurnMetadata, TokenUsage } from '../../types.js';
import type { ReasoningOutput } from 'ai';

export interface ScenarioToolCall {
  toolName: string;
  arguments: Record<string, unknown>;
  callId?: string;
  assistantText?: string;
}

export type ScenarioStepResponse =
  | {
      kind: 'tool-call';
      toolCalls: ScenarioToolCall[];
      assistantText?: string;
      finishReason?: 'tool-calls';
      tokenUsage?: TokenUsage;
      reasoning?: ReasoningOutput[];
      reasoningContent?: string | ReasoningOutput | ReasoningOutput[];
      providerMetadata?: ProviderTurnMetadata;
    }
  | {
      kind: 'final-report';
      assistantText?: string;
      reportContent: string;
      reportFormat: string;
      status?: 'success' | 'failure';
      reportContentJson?: Record<string, unknown>;
      reportMessages?: unknown[];  // For slack-block-kit: array of messages
      tokenUsage?: TokenUsage;
      reasoning?: ReasoningOutput[];
      reasoningContent?: string | ReasoningOutput | ReasoningOutput[];
      finishReason?: string;
      providerMetadata?: ProviderTurnMetadata;
    }
  | {
      kind: 'text';
      assistantText: string;
      finishReason?: 'stop' | 'other' | 'content-filter' | 'refusal';
      tokenUsage?: TokenUsage;
      reasoning?: ReasoningOutput[];
      reasoningContent?: string | ReasoningOutput | ReasoningOutput[];
      providerMetadata?: ProviderTurnMetadata;
    };

export interface ScenarioTurn {
  turn: number;
  expectedTools?: string[];
  response: ScenarioStepResponse;
  failuresBeforeSuccess?: number;
  failureStatus?: 'model_error' | 'network_error' | 'timeout' | 'invalid_response' | 'rate_limit';
  failureRetryable?: boolean;
  failureMessage?: string;
  failureError?: Record<string, unknown>;
  failureThrows?: boolean;
  allowMissingTools?: boolean;
  expectedTemperature?: number;
  expectedTopP?: number;
  failureRetryAfterMs?: number;
  expectedReasoning?: 'enabled' | 'disabled';
}

export interface ScenarioDefinition {
  id: string;
  description: string;
  systemPromptMustInclude?: string[];
  turns: ScenarioTurn[];
}

const DEFAULT_TOKEN_USAGE: TokenUsage = {
  inputTokens: 120,
  outputTokens: 40,
  totalTokens: 160,
};

const SYSTEM_PROMPT_MARKER = 'Phase 1 deterministic harness';
const TOOL_NAME = 'test__test';
const SUBAGENT_TOOL = 'agent__pricing-subagent';
const SUBAGENT_SUCCESS_TOOL = 'agent__pricing-subagent-success';
const TOOL_FINISH_REASON = 'tool-calls';
const RESULT_HEADING = '# Phase 1 Result\n\n';
const FINAL_RESPONSE_KIND = 'final-report' as const;
const MARKDOWN_FORMAT = 'markdown' as const;
const STATUS_SUCCESS = 'success' as const;
const STATUS_FAILURE = 'failure' as const;
const STATUS_IN_PROGRESS = 'in-progress' as const;
// Reserved for future XML scenarios
const TOOL_REQUEST_TEXT = 'Requesting test to gather information.';
const TOOL_ARGUMENT_SUCCESS = 'phase-1-tool-success';
const TOOL_ARGUMENT_LONG_OUTPUT = 'long-output';
const TOOL_ARGUMENT_CONTEXT_OVERFLOW = 'context-guard-600';
const CONCURRENCY_TIMEOUT_ARGUMENT = 'trigger-timeout';
const CONCURRENCY_SECOND_ARGUMENT = 'concurrency-second';
const BATCH_INVALID_INPUT_ARGUMENT = 'batch-missing-id';
const BATCH_UNKNOWN_TOOL = 'unknown__tool';
const BATCH_EXECUTION_ERROR_ARGUMENT = 'trigger-mcp-failure';
const STREAM_REASONING_STEPS: readonly ReasoningOutput[] = [
  {
    type: 'reasoning',
    text: 'Deliberating over deterministic harness state.',
    providerMetadata: { anthropic: { signature: 'stream-step-0' } },
  },
  {
    type: 'reasoning',
    text: 'Confirming reasoning stream emission.',
    providerMetadata: { anthropic: { signature: 'stream-step-1' } },
  },
];
const THROW_FAILURE_MESSAGE = 'Simulated provider throw for coverage.';
const FINAL_REPORT_JSON_ATTEMPT = 'Attempting JSON final report without structured payload.';
const FINAL_REPORT_SUCCESS_SUMMARY = 'Final report emitted after retry.';
const TEXT_ONLY_RESPONSE = 'Providing plain text without invoking final report tool.';
const BATCH_PROGRESS_STATUS = 'Batch progress update for coverage.';
const BATCH_PROGRESS_RESPONSE = 'batch-progress-follow-up';
const BATCH_STRING_PROGRESS = 'Batch progress conveyed via string payload.';
const BATCH_STRING_RESULT = 'batch-string-mode';
const SLACK_BLOCK_KIT_FORMAT = 'slack-block-kit' as const;
const RATE_LIMIT_FAILURE_MESSAGE = 'Rate limit simulated.' as const;
const SANITIZER_VALID_ARGUMENT = 'sanitizer-valid-call';
const LONG_TOOL_NAME = `tool-${'x'.repeat(140)}`;
const FINAL_REPORT_RETRY_MESSAGE = 'Final report completed after mixed tools.';
const CONTEXT_RETRY_FAILURE_MESSAGE = 'Simulated model failure before retry.';

const SCENARIOS: ScenarioDefinition[] = [
  {
    id: 'run-test-1',
    description: 'LLM + MCP success path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: TOOL_REQUEST_TEXT,
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-1',
              assistantText: 'Preparing test input.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
          reasoning: [
            {
              type: 'reasoning',
              text: 'Evaluating task before invoking tool.',
              providerMetadata: { anthropic: { signature: 'scenario-run-test-1-step-0' } },
            },
          ],
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Synthesizing final report from tool output.',
          reportContent: `${RESULT_HEADING}Tool execution succeeded and data was recorded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 30,
            totalTokens: 110,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-120',
    description: 'Provider metadata propagation for accounting and logging.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting router metadata back to caller.',
          reportContent: `${RESULT_HEADING}Metadata propagation confirmed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 60,
            outputTokens: 28,
            totalTokens: 88,
            cacheWriteInputTokens: 42,
          },
          providerMetadata: {
            actualProvider: 'router/fireworks',
            actualModel: 'fireworks-test',
            reportedCostUsd: 0.12345,
            upstreamCostUsd: 0.06789,
            cacheWriteInputTokens: 42,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-121',
    description: 'Anthropic reasoning stays enabled after a tool-only turn without thinking output.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedReasoning: 'enabled',
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering initial context before invoking tools.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-121-a',
              assistantText: 'Preparing data requirements.',
              arguments: {
                text: 'phase-1-tool-reasoning-initial',
              },
            },
          ],
          finishReason: TOOL_FINISH_REASON,
          tokenUsage: DEFAULT_TOKEN_USAGE,
          reasoning: [
            {
              type: 'reasoning',
              text: 'Planning initial tool usage.',
              providerMetadata: { anthropic: { signature: 'scenario-run-test-121-initial' } },
            },
          ],
        },
      },
      {
        turn: 2,
        expectedReasoning: 'enabled',
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Continuing with additional tool calls.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-121-b',
              assistantText: 'Gathering follow-up data.',
              arguments: {
                text: 'phase-1-tool-follow-up',
              },
            },
          ],
          finishReason: TOOL_FINISH_REASON,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 3,
        expectedReasoning: 'enabled',
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing results after mixed tool outputs.',
          reportContent: `${RESULT_HEADING}Reasoning remained enabled across tool-only turns.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 32,
            totalTokens: 122,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-2',
    description: 'LLM ok, MCP tool failure path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: TOOL_REQUEST_TEXT,
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-fail-1',
              assistantText: 'Triggering failure for testing.',
              arguments: {
                text: 'trigger-mcp-failure',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting MCP failure outcome.',
          reportContent: `${RESULT_HEADING}Tool execution failed as expected for testing.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 25,
            totalTokens: 95,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-3',
    description: 'LLM failure followed by retry success.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureStatus: 'network_error',
        failureRetryable: true,
        failureMessage: 'Simulated LLM failure (retry expected).',
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting test after retry.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-retry-1',
              assistantText: 'Preparing test input after retry.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after retry.',
          reportContent: `${RESULT_HEADING}LLM retry succeeded and data was recorded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 25,
            totalTokens: 95,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-4',
    description: 'LLM fallback to secondary provider.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureStatus: 'model_error',
        failureRetryable: false,
        failureMessage: 'Primary provider failed.',
        response: {
          kind: 'tool-call',
          assistantText: 'Fallback provider calling test.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-test-fallback-1',
              assistantText: 'Preparing test input after fallback.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Fallback provider produced final report.',
          reportContent: `${RESULT_HEADING}Fallback provider succeeded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 25,
            totalTokens: 95,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-5',
    description: 'LLM timeout path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        // With maxRetries=1, we need 2 failures to exhaust all attempts (initial + 1 retry)
        failuresBeforeSuccess: 2,
        failureStatus: 'timeout',
        failureRetryable: false,
        failureMessage: 'Simulated timeout.',
        response: {
          kind: 'tool-call',
          assistantText: 'Should not reach tool execution.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-timeout-1',
              assistantText: 'Unused call.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
    ],
  },
  {
    id: 'run-test-6',
    description: 'LLM exceeds max retries.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 3,
        failureStatus: 'model_error',
        failureRetryable: true,
        failureMessage: 'Repeated retryable failure.',
        response: {
          kind: 'tool-call',
          assistantText: 'Should not reach success.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-retry-limit',
              assistantText: 'Unused call.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
    ],
  },
  {
    id: 'run-test-7',
    description: 'MCP tool timeout path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Triggering MCP timeout.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-tool-timeout',
              assistantText: 'Waiting for timeout.',
              arguments: {
                text: 'trigger-timeout',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting timeout outcome.',
          reportContent: `${RESULT_HEADING}Tool timeout handled.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 60,
            outputTokens: 20,
            totalTokens: 80,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-8',
    description: 'MCP tool truncation path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting large payload.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-truncation',
              assistantText: 'Fetching extended payload.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Report after truncation.',
          reportContent: `${RESULT_HEADING}Truncated payload processed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 25,
            totalTokens: 95,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-9',
    description: 'Unknown tool scenario.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        allowMissingTools: true,
        response: {
          kind: 'tool-call',
          assistantText: 'Attempting unknown tool.',
          toolCalls: [
            {
              toolName: 'nonexistent_tool',
              callId: 'call-unknown',
              assistantText: 'This tool does not exist.',
              arguments: {
                text: 'irrelevant',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
    ],
  },
  {
    id: 'run-test-10',
    description: 'Progress report usage with MCP success.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__task_status', TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Providing progress and calling test.',
          toolCalls: [
            {
              toolName: 'agent__task_status',
              callId: 'call-progress-1',
              assistantText: 'Reporting progress.',
              arguments: {
                status: STATUS_IN_PROGRESS,
                done: 'Step 1 complete',
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-progress-2',
              assistantText: 'Querying test.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after progress.',
          reportContent: `${RESULT_HEADING}Progress captured successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 30,
            totalTokens: 110,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-11',
    description: 'Final report schema mismatch.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering information for mismatched report.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-final-mismatch',
              assistantText: 'Collecting data.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report with schema issue.',
          reportContent: `${RESULT_HEADING}Report delivered with unexpected format.`,
          reportFormat: 'json',
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 25,
            totalTokens: 95,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-12',
    description: 'Final-turn enforced without additional tool calls.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Providing final report due to final-turn lock.',
          reportContent: `${RESULT_HEADING}Final turn completed without tool access.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 60,
            outputTokens: 20,
            totalTokens: 80,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-13',
    description: 'Batch tooling success with progress + MCP call.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing batched tool calls.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-success',
              assistantText: 'Batch includes progress and test calls.',
              arguments: {
                calls: [
                  {
                    id: '1',
                    tool: 'agent__task_status',
                    parameters: { status: STATUS_IN_PROGRESS, done: 'Batch step started', pending: 'Process batch operations', now: 'Complete batch processing' },
                  },
                  {
                    id: '2',
                    tool: TOOL_NAME,
                    parameters: { text: 'batch-success' },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing batched tool output.',
          reportContent: `${RESULT_HEADING}Batch execution completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 32,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-14',
    description: 'Batch tooling invalid input path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting malformed batch payload.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-invalid',
              assistantText: 'Batch call missing identifiers.',
              arguments: {
                calls: [],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting invalid batch outcome.',
          reportContent: `${RESULT_HEADING}Batch validation failed as expected.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 26,
            totalTokens: 96,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-15',
    description: 'Pricing coverage warning scenario.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering data for pricing coverage.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-pricing-1',
              assistantText: 'Collecting payload.',
              arguments: {
                text: 'pricing-coverage',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report noting pricing coverage.',
          reportContent: `${RESULT_HEADING}Pricing coverage flow completed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 28,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-16',
    description: 'Persistence outputs (sessions + billing).',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting data for persistence validation.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-persistence-1',
              assistantText: 'Echoing persistence payload.',
              arguments: {
                text: 'persistence-check',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing persistence check.',
          reportContent: `${RESULT_HEADING}Persistence artifacts recorded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 28,
            totalTokens: 106,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-17',
    description: 'Model overrides and temperature/topP handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        expectedTemperature: 0.42,
        expectedTopP: 0.85,
        response: {
          kind: 'tool-call',
          assistantText: 'Executing with overridden sampling parameters.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-overrides-1',
              assistantText: 'Collecting override data.',
              arguments: {
                text: 'overrides-check',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting override results.',
          reportContent: `${RESULT_HEADING}Model overrides applied successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 30,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-18',
    description: 'Abort signal cancellation path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [],
        allowMissingTools: true,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Abort scenario should not reach LLM.',
          reportContent: `${RESULT_HEADING}Abort signal scenario fallback.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-19',
    description: 'Pricing aggregation with sub-agents.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [SUBAGENT_TOOL, TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering data including sub-agent context.',
          toolCalls: [
            {
              toolName: SUBAGENT_TOOL,
              callId: 'call-subagent-1',
              assistantText: 'Delegating pricing analysis to sub-agent.',
              arguments: {
                prompt: 'Need supporting pricing analysis.',
                reason: 'Augment pricing context',
                format: 'sub-agent',
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-pricing-subagent',
              assistantText: 'Collecting pricing data.',
              arguments: {
                text: 'pricing-subagent',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarising pricing coverage with sub-agent.',
          reportContent: `${RESULT_HEADING}Pricing coverage with sub-agent evaluated.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 84,
            outputTokens: 32,
            totalTokens: 116,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-20',
    description: 'Persistence error handling path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Triggering persistence error.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-persistence-error',
              assistantText: 'Echoing persistence error payload.',
              arguments: {
                text: 'persistence-error',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting persistence error handling.',
          reportContent: `${RESULT_HEADING}Persistence failure captured.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 30,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-21',
    description: 'Rate limit handling with backoff.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureStatus: 'rate_limit',
        failureMessage: 'Simulated rate limit',
        failureRetryable: true,
        failureRetryAfterMs: 25,
        response: {
          kind: 'tool-call',
          assistantText: 'Retrying after rate limit.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-rate-limit-1',
              assistantText: 'Attempt after rate limit.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Completed after rate limit backoff.',
          reportContent: `${RESULT_HEADING}Rate limit recovered successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 72,
            outputTokens: 28,
            totalTokens: 100,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-22',
    description: 'Invalid provider configuration.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Configuration failure path.',
          reportContent: `${RESULT_HEADING}Configuration failure encountered.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-23',
    description: 'Graceful stop without LLM execution.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Graceful stop scenario.',
          reportContent: `${RESULT_HEADING}Graceful stop executed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-24',
    description: 'Sub-agent success execution.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [SUBAGENT_SUCCESS_TOOL, TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing sub-agent successfully.',
          toolCalls: [
            {
              toolName: SUBAGENT_SUCCESS_TOOL,
              callId: 'call-subagent-success',
              assistantText: 'Delegating to pricing sub-agent.',
              arguments: {
                prompt: 'run-test-24-subagent',
                reason: 'Gather detailed pricing insights',
                format: 'sub-agent',
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-subagent-test',
              assistantText: 'Collecting supporting data.',
              arguments: {
                text: 'subagent-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Sub-agent results summarised.',
          reportContent: `${RESULT_HEADING}Sub-agent completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 30,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-25',
    description: 'Tool concurrency saturation queue handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Issuing two MCP calls with enforced queue.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-concurrency-1',
              assistantText: 'First call triggers the timeout payload.',
              arguments: {
                text: CONCURRENCY_TIMEOUT_ARGUMENT,
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-concurrency-2',
              assistantText: 'Second call should wait for the slot.',
              arguments: {
                text: CONCURRENCY_SECOND_ARGUMENT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting queued tool execution results.',
          reportContent: `${RESULT_HEADING}Queued tool execution completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 34,
            totalTokens: 122,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-queue-cancel',
    description: 'Tool queue cancellation path (abort while waiting).',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Issue two MCP calls so the second waits on the queue.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-queue-cancel-1',
              assistantText: 'First call triggers the timeout payload.',
              arguments: {
                text: CONCURRENCY_TIMEOUT_ARGUMENT,
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-queue-cancel-2',
              assistantText: 'Second call should be queued until cancellation.',
              arguments: {
                text: CONCURRENCY_SECOND_ARGUMENT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Cancellation path still reports queued work.',
          reportContent: `${RESULT_HEADING}Queued tool execution canceled mid-flight.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 26,
            totalTokens: 106,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-queue-isolation',
    description: 'Batch call saturates default queue while fast queue remains free.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Batch call mixes slow and fast queues.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-queue-isolation',
              arguments: {
                calls: [
                  { id: 'slow-1', tool: 'slow__test', parameters: { text: CONCURRENCY_TIMEOUT_ARGUMENT } },
                  { id: 'slow-2', tool: 'slow__test', parameters: { text: CONCURRENCY_SECOND_ARGUMENT } },
                  { id: 'fast-1', tool: 'fast__test', parameters: { text: 'fast-call' } },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Batch execution completed with isolated queues.',
          reportContent: `${RESULT_HEADING}Slow queue congestion did not affect the fast queue.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 32,
            totalTokens: 122,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-26',
    description: 'Batch invalid input detection (missing tool).',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting batch payload missing identifiers.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-invalid-id',
              assistantText: 'Batch entry lacks the required tool field.',
              arguments: {
                calls: [
                  {
                    // Missing 'tool' field - should trigger invalid_batch_input validation
                    parameters: {
                      text: BATCH_INVALID_INPUT_ARGUMENT,
                    },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Finalising invalid batch outcome.',
          reportContent: `${RESULT_HEADING}Batch input validation failed as expected.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 76,
            outputTokens: 28,
            totalTokens: 104,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-27',
    description: 'Batch unknown tool handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting batch payload with unknown tool.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-unknown-tool',
              assistantText: 'Entry targets an unknown tool to trigger error result.',
              arguments: {
                calls: [
                  {
                    id: 'u-1',
                    tool: BATCH_UNKNOWN_TOOL,
                    parameters: {
                      note: 'unknown-tool',
                    },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarising unknown tool batch outcome.',
          reportContent: `${RESULT_HEADING}Batch unknown tool result captured successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 30,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-28',
    description: 'Batch execution error propagation.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting batch payload that triggers execution error.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-exec-error',
              assistantText: 'Entry will surface execution failure from MCP tool.',
              arguments: {
                calls: [
                  {
                    id: 'e-1',
                    tool: TOOL_NAME,
                    parameters: {
                      text: BATCH_EXECUTION_ERROR_ARGUMENT,
                    },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarising execution error result.',
          reportContent: `${RESULT_HEADING}Batch captured downstream execution error.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 30,
            totalTokens: 110,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-29',
    description: 'Session-level retry via retry() API.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        // With maxTurns=1, first session is in final turn from start.
        // maxRetries=2 means 2 attempts, both throw.
        // Retry exhaustion in final turn → actual session failure (not graceful exhaustion).
        // retry() reuses LLMClient, so counter=2 persists.
        // Retry session's turn 1: counter=2 >= 2, so succeeds with final-report.
        failuresBeforeSuccess: 2,
        failureThrows: true,
        failureMessage: 'Simulated fatal error before manual retry.',
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Session retry succeeded after manual invocation.',
          reportContent: `${RESULT_HEADING}Session retry succeeded after manual invocation.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-30',
    description: 'Streaming reasoning/thinking emission.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Streaming final report with reasoning.',
          reportContent: `${RESULT_HEADING}Streaming reasoning scenario completed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
          reasoning: [...STREAM_REASONING_STEPS],
        },
      },
    ],
  },
  {
    id: 'run-test-31',
    description: 'Provider throws before retry succeeds.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureMessage: THROW_FAILURE_MESSAGE,
        response: {
          kind: 'tool-call',
          assistantText: 'Retry after thrown provider failure.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-throw-retry-1',
              assistantText: 'Executing after provider throw.',
              arguments: {
                text: 'throw-retry-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after thrown failure recovery.',
          reportContent: `${RESULT_HEADING}Thrown failure path recovered successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 28,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-32',
    description: 'Invalid JSON final report followed by retry.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering data before invalid final report.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-final-json-tool',
              assistantText: 'Collecting details ahead of final report.',
              arguments: {
                text: 'final-json-step',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: FINAL_REPORT_JSON_ATTEMPT,
          reportContent: `${RESULT_HEADING}Invalid JSON final report attempt.`,
          reportFormat: 'json',
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 24,
            totalTokens: 94,
          },
        },
      },
      {
        turn: 3,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: FINAL_REPORT_SUCCESS_SUMMARY,
          reportContent: `${RESULT_HEADING}Final report succeeded after retry.`,
          reportFormat: 'json',
          status: STATUS_SUCCESS,
          reportContentJson: {
            outcome: 'success',
            summary: 'Final report succeeded after retry.',
          },
          tokenUsage: {
            inputTokens: 72,
            outputTokens: 26,
            totalTokens: 98,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-33',
    description: 'Plain text response triggers synthetic retry.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        allowMissingTools: true,
        response: {
          kind: 'text',
          assistantText: TEXT_ONLY_RESPONSE,
          finishReason: 'stop',
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-34',
    description: 'Batch call with progress report entry.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing batch with progress report.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-progress',
              assistantText: 'Batch includes progress entry and tool call.',
              arguments: {
                calls: [
                  {
                    id: 'p-1',
                    tool: 'agent__task_status',
                    parameters: { status: STATUS_IN_PROGRESS, done: BATCH_PROGRESS_STATUS, pending: 'Complete batch processing', now: 'Finalize batch operations' },
                  },
                  {
                    id: 'p-2',
                    tool: TOOL_NAME,
                    parameters: { text: BATCH_PROGRESS_RESPONSE },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting batch progress outcome.',
          reportContent: `${RESULT_HEADING}Batch progress update processed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 30,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-35',
    description: 'Batch calls provided as JSON string.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting batch payload as string.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-string',
              assistantText: 'String payload contains progress and tool work.',
              arguments: {
                calls: `[{"id":"s-1","tool":"agent__task_status","parameters":{"status":"${STATUS_IN_PROGRESS}","done":"${BATCH_STRING_PROGRESS}","pending":"Continue processing","now":"Complete task"}},{"id":"s-2","tool":"${TOOL_NAME}","parameters":{"text":"${BATCH_STRING_RESULT}"}}]`,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarising string batch execution.',
          reportContent: `${RESULT_HEADING}Batch string payload executed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 84,
            outputTokens: 32,
            totalTokens: 116,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-36',
    description: 'Empty batch payload triggers failure.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Submitting empty batch payload.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-empty',
              assistantText: 'Empty batch should fail immediately.',
              arguments: {
                calls: '',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarising empty batch failure.',
          reportContent: `${RESULT_HEADING}Empty batch payload rejected successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 74,
            outputTokens: 28,
            totalTokens: 102,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-45',
    description: 'Traced fetch and pricing enrichment.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing under traced fetch.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-trace-fetch',
              assistantText: 'Collecting data for traced fetch.',
              arguments: {
                text: 'trace-fetch-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting traced fetch outcome.',
          reportContent: `${RESULT_HEADING}Traced fetch completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 28,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-37',
    description: 'Rate limit error triggers retry.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureMessage: RATE_LIMIT_FAILURE_MESSAGE,
        failureStatus: 'rate_limit',
        failureRetryAfterMs: 2000,
        response: {
          kind: 'tool-call',
          assistantText: 'Executing after rate limit retry.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-rate-limit-retry',
              assistantText: 'Collecting data after rate limit.',
              arguments: {
                text: 'rate-limit-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after rate limit retry.',
          reportContent: `${RESULT_HEADING}Rate limit recovery completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 28,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-38',
    description: 'Auth error fails fast.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 999,
        failureThrows: true,
        failureMessage: 'Authentication failed.',
        failureError: {
          name: 'AuthError',
          status: 401,
          responseBody: JSON.stringify({ error: { message: 'Auth failed' } }),
        },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Auth failure fallback (unreachable).',
          reportContent: `${RESULT_HEADING}Auth failure.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-39',
    description: 'Quota exceeded error handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 999,
        failureThrows: true,
        failureMessage: 'Quota exceeded.',
        failureError: {
          name: 'QuotaError',
          status: 402,
          responseBody: JSON.stringify({ error: { message: 'Quota exceeded' } }),
        },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Quota failure fallback (unreachable).',
          reportContent: `${RESULT_HEADING}Quota failure.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-40',
    description: 'Timeout error handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 999,
        failureThrows: true,
        failureMessage: 'Request timed out.',
        failureError: {
          name: 'TimeoutError',
          statusText: 'Gateway Timeout',
          message: 'Request timed out.',
        },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Timeout fallback (unreachable).',
          reportContent: `${RESULT_HEADING}Timeout failure.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-41',
    description: 'Network error handling.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 999,
        failureThrows: true,
        failureMessage: 'Network connection lost.',
        failureError: {
          name: 'NetworkError',
          code: 'ECONNRESET',
          message: 'Network connection lost.',
        },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Network failure fallback (unreachable).',
          reportContent: `${RESULT_HEADING}Network failure.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-42',
    description: 'Model error with retryable path.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureMessage: 'Invalid model request.',
        failureError: {
          status: 400,
          message: 'Invalid model request.',
        },
        response: {
          kind: 'tool-call',
          assistantText: 'Retrying after model error.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-model-error-retry',
              assistantText: 'Collecting data after model error.',
              arguments: {
                text: 'model-error-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after model error retry.',
          reportContent: `${RESULT_HEADING}Model error retry succeeded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 30,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-46',
    description: 'Persistence paths and concurrency summary.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'First tool call for persistence coverage.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-persistence',
              assistantText: 'Collecting data for persistence.',
              arguments: {
                text: 'persistence-coverage',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting persistence coverage.',
          reportContent: `${RESULT_HEADING}Persistence coverage completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 28,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-43',
    description: 'Immediate graceful stop without LLM call.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [],
  },
  {
    id: 'run-test-44',
    description: 'Rate limit backoff aborted by stop request.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureMessage: RATE_LIMIT_FAILURE_MESSAGE,
        failureStatus: 'rate_limit',
        failureRetryAfterMs: 2000,
        response: {
          kind: 'tool-call',
          assistantText: 'Executing after rate limit retry with stop.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-rate-limit-stop',
              assistantText: 'Collecting data after stop.',
              arguments: {
                text: 'rate-limit-stop-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after stop-induced retry.',
          reportContent: `${RESULT_HEADING}Stop request honored during rate limit backoff.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 30,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-49',
    description: 'Graceful stop during rate limit backoff.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureMessage: RATE_LIMIT_FAILURE_MESSAGE,
        failureStatus: 'rate_limit',
        failureRetryAfterMs: 2000,
        response: {
          kind: 'tool-call',
          assistantText: 'Executing after stop-induced retry.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-stop-retry',
              assistantText: 'Collecting data after stop retry.',
              arguments: {
                text: 'stop-retry-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after stop retry.',
          reportContent: `${RESULT_HEADING}Stop retry completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 30,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-48',
    description: 'Internal provider trace logging.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Triggering trace logging.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-trace-tools',
              assistantText: 'Collecting data for trace logging.',
              arguments: {
                text: 'trace-tools-success',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting trace logging.',
          reportContent: `${RESULT_HEADING}Trace logging completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 78,
            outputTokens: 30,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-47',
    description: 'Internal provider schema validation failure.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Invalid JSON final report to trigger schema failure.',
          reportContent: `${RESULT_HEADING}Schema failure.`,
          reportFormat: 'json',
          status: STATUS_SUCCESS,
          reportContentJson: {
            status: 'failure',
            detail: 'Schema should reject this payload.',
          },
        },
      },
    ],
  },
  {
    id: 'run-test-50',
    description: 'Slack block kit final report fallback.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Delivering Slack formatted summary.',
          reportContent: '*Slack* message generated for delivery.',
          reportFormat: SLACK_BLOCK_KIT_FORMAT,
          reportMessages: [{ blocks: [{ type: 'header', text: { type: 'plain_text', text: 'Phase 1 Result' } }, { type: 'section', text: { type: 'mrkdwn', text: '*Slack* message generated for delivery.' } }] }],
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-51',
    description: 'Slack block kit final report missing content.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Slack report without content.',
          reportContent: ' ',
          reportFormat: SLACK_BLOCK_KIT_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-52',
    description: 'Slack block kit messages normalization via tool call.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Sending rich Slack content.',
          reportFormat: SLACK_BLOCK_KIT_FORMAT,
          reportContent: `${RESULT_HEADING}Slack block kit messages normalized.`,
          reportMessages: [
            'Primary message with *formatting*',
            { blocks: [{ type: 'section', text: { type: 'mrkdwn', text: '*Detail* section with field' }, fields: [{ type: 'mrkdwn', text: 'Field line' }] }] },
            JSON.stringify({ type: 'divider' }),
            [
              JSON.stringify({ type: 'header', text: { type: 'plain_text', text: 'Header Title' } }),
              { type: 'context', elements: ['Context line 1', { text: 'Context line 2', type: 'mrkdwn' }] }
            ]
          ],
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Slack tool call completed.',
          reportContent: `${RESULT_HEADING}Slack tool call completed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-53',
    description: 'GitHub search normalization coverage.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['github__search_code'],
        response: {
          kind: 'tool-call',
          assistantText: 'Querying GitHub code search.',
          toolCalls: [
            {
              toolName: 'github__search_code',
              callId: 'call-github-search',
              assistantText: 'Running repository search.',
              arguments: {
                query: 'md5 helper',
                repo: 'owner/project',
                path: 'src',
                language: 'js OR jsx | python',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'GitHub search normalized.',
          reportContent: `${RESULT_HEADING}GitHub search completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-56',
    description: 'Provider retry paths across multiple failure types.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureStatus: 'model_error',
        failureRetryable: true,
        failureMessage: 'Model failure during first attempt.',
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Attempting primary tool call after transient model error.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-run-test-56-1',
              assistantText: 'Retrying test tool after model error.',
              arguments: {
                text: 'retry-after-model-error',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        failuresBeforeSuccess: 1,
        failureStatus: 'rate_limit',
        failureRetryAfterMs: 200,
        failureMessage: 'Rate limit encountered.',
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Retrying after rate limit.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-run-test-56-2',
              assistantText: 'Collecting data following rate limit.',
              arguments: {
                text: 'retry-after-rate-limit',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 3,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Concluding after retries.',
          reportContent: `${RESULT_HEADING}Retries completed successfully after handling model and rate limit errors.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-57',
    description: 'Progress updates and Slack final report via tool calls.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__task_status'],
        response: {
          kind: 'tool-call',
          assistantText: 'Issuing progress update and Slack final report.',
          toolCalls: [
            {
              toolName: 'agent__task_status',
              callId: 'call-progress-run-test-57',
              assistantText: 'Reporting current progress.',
              arguments: {
                status: STATUS_IN_PROGRESS,
                done: 'Analyzing deterministic harness outputs.',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Slack final report delivered.',
          reportContent: `${RESULT_HEADING}Slack report dispatched successfully.`,
          reportFormat: SLACK_BLOCK_KIT_FORMAT,
          reportMessages: [
            'Primary summary with _structured_ content.',
            {
              blocks: [
                {
                  type: 'section',
                  text: { type: 'mrkdwn', text: '*Detailed* findings and next actions.' },
                  fields: [{ type: 'mrkdwn', text: 'Next: Validate coverage.' }],
                },
                { type: 'divider' },
                { type: 'context', elements: ['Context item 1', { type: 'mrkdwn', text: 'Context item 2' }] },
              ],
            },
          ],
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-58',
    description: 'Progress metrics, summaries, and accounting aggregation.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__task_status', TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Publishing progress and querying MCP tool.',
          toolCalls: [
            {
              toolName: 'agent__task_status',
              callId: 'call-progress-run-test-58',
              assistantText: 'Progress update before invoking MCP tool.',
              arguments: {
                status: STATUS_IN_PROGRESS,
                done: 'Collecting metrics via test MCP tool.',
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-tool-run-test-58',
              assistantText: 'Retrieving MCP data for metrics.',
              arguments: {
                text: 'metrics-request-payload',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing collected metrics.',
          reportContent: `${RESULT_HEADING}Metrics collected from MCP tool and progress updates noted.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-59',
    description: 'Tool response size cap and truncation.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing batch with oversized tool output.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-cap',
              assistantText: 'Batch includes progress and large MCP output.',
              arguments: {
                calls: [
                  { id: 'p-1', tool: 'agent__task_status', parameters: { status: 'starting', done: 'Starting large data export.', pending: 'Process remaining data', now: 'Complete export' } },
                  { id: 'p-2', tool: TOOL_NAME, parameters: { text: 'X'.repeat(5000) } }
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Large tool output processed with size cap.',
          reportContent: `${RESULT_HEADING}Tool response was truncated to respect size limits.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-60',
    description: 'Conversation history and sampling parameter propagation.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        expectedTemperature: 0.21,
        expectedTopP: 0.77,
        response: {
          kind: 'tool-call',
          assistantText: 'Referencing prior conversation before invoking tool.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-history-prop',
              assistantText: 'Replaying historical context.',
              arguments: {
                text: 'history-propagation',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing with propagated parameters.',
          reportContent: `${RESULT_HEADING}Conversation history respected and parameters applied.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-61',
    description: 'Tool calls per turn limit enforcement.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Attempting multiple tool calls within a single turn.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-limit-first',
              assistantText: 'First call within limit.',
              arguments: {
                text: 'first-tool-call',
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-limit-second',
              assistantText: 'Second call exceeding limit.',
              arguments: {
                text: 'second-tool-call',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting enforcement of per-turn tool call limits.',
          reportContent: `${RESULT_HEADING}Tool call limit enforced; adjust strategy before retrying.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-max-turn-limit',
    description: 'Session max-turn enforcement when no final report is produced.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting preliminary data before final turn.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'max-turn-call-1',
              assistantText: 'Gathering initial evidence.',
              arguments: {
                text: 'max-turn-initial',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final turn reached; providing mandatory report.',
          reportContent: `${RESULT_HEADING}Max turns reached; summarizing with available information.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-62',
    description: 'Rate limit waits interrupted by abort signal.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 3,
        failureStatus: 'rate_limit',
        failureRetryable: true,
        failureRetryAfterMs: 2000,
        response: {
          kind: 'tool-call',
          assistantText: 'Would proceed after rate limit backoff if not aborted.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-rate-limit-abort',
              assistantText: 'Placeholder call after rate limit.',
              arguments: {
                text: 'rate-limit-abort',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
    ],
  },
  {
    id: 'run-test-63',
    description: 'Verbose configuration summary and MCP warmup concurrency.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting data after verbose configuration summary.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-verbose-concurrency',
              assistantText: 'Executing after MCP warmup.',
              arguments: {
                text: 'verbose-concurrency',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing verbose configuration and concurrency checks.',
          reportContent: `${RESULT_HEADING}Verbose settings logged and MCP warmup sequential as configured.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-68',
    description: 'Cache write enrichment passthrough.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Returning final response without cache info.',
          reportContent: `${RESULT_HEADING}Cache write enrichment scenario.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 50,
            outputTokens: 20,
            totalTokens: 70,
          },
        },
      },
    ],
  },

  {
    id: 'run-test-78',
    description: 'Stop reason emitted as max_tokens.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Token limit reached while finalizing the report.',
          reportContent: `${RESULT_HEADING}Max tokens stop triggered.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          finishReason: 'max_tokens',
          tokenUsage: {
            inputTokens: 120,
            outputTokens: 540,
            totalTokens: 660,
          },
        },
      },
    ],
  },


  {
    id: 'run-test-83-auth',
    description: 'LLM client mapError auth error.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { status: 401, message: 'Invalid API key', name: 'AuthError' },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Authentication issue resolved.',
          reportContent: `${RESULT_HEADING}Auth error scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-83-quota',
    description: 'LLM client mapError quota exceeded.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { status: 402, message: 'Quota exceeded', name: 'BillingError' },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Quota issue acknowledged.',
          reportContent: `${RESULT_HEADING}Quota error scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-83-rate',
    description: 'LLM client mapError rate limit.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { status: 429, message: 'Rate limit hit', name: 'RateLimitError', headers: { 'retry-after': '3' } },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Rate limit cleared.',
          reportContent: `${RESULT_HEADING}Rate limit scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-83-timeout',
    description: 'LLM client mapError timeout.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { name: 'TimeoutError', message: 'Request timed out' },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Timeout scenario complete.',
          reportContent: `${RESULT_HEADING}Timeout scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-83-network',
    description: 'LLM client mapError network error.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { name: 'NetworkError', message: 'Network connection lost' },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Network issue resolved.',
          reportContent: `${RESULT_HEADING}Network error scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-83-model',
    description: 'LLM client mapError model error.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureError: { status: 400, message: 'Invalid request payload', name: 'BadRequestError' },
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Model error handled.',
          reportContent: `${RESULT_HEADING}Model error scenario resolved.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },


  {
    id: 'run-test-74',
    description: 'Final report validation error surfaced to LLM.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Submitting final report without content to trigger validation.',
          reportContent: '   ',
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Retrying with a valid final report.',
          reportContent: `${RESULT_HEADING}Final report accepted after retry.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },

  {
    id: 'run-test-24-subagent',
    description: 'Sub-agent internal success path.',
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Sub-agent delivering final report.',
          reportContent: `${RESULT_HEADING}Sub-agent provided pricing insights.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-89',
    description: 'Sanitize invalid tool calls before persisting conversation history.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: '',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-invalid-1',
              assistantText: '',
              // Intentionally broken: arguments encoded as string to trigger sanitizer drop.
               
              arguments: 'not-an-object' as unknown as Record<string, unknown>,
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-valid-1',
              assistantText: 'Providing well-formed arguments after malformed entry.',
              arguments: {
                text: SANITIZER_VALID_ARGUMENT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting sanitized tool call completion.',
          reportContent: `${RESULT_HEADING}Sanitizer scenario completed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 24,
            totalTokens: 94,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-91',
    description: 'Mixed tool calls with batch successes and failures.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch', LONG_TOOL_NAME],
        allowMissingTools: true,
        response: {
          kind: 'tool-call',
          assistantText: 'Issuing mixed tool calls including batch, oversized name, and invalid final report.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-valid-mixed',
              assistantText: 'Valid batch entry should succeed.',
              arguments: {
                calls: [
                  {
                    id: 'm-1',
                    tool: 'agent__task_status',
                    parameters: { status: STATUS_IN_PROGRESS, done: 'Valid batch progress entry.', pending: 'Complete batch validation', now: 'Validate batch processing' },
                  },
                  {
                    id: 'm-2',
                    tool: TOOL_NAME,
                    parameters: { text: 'batch-mixed-success' },
                  },
                ],
              },
            },
            {
              toolName: 'agent__batch',
              callId: 'call-batch-invalid-mixed',
              assistantText: 'Invalid batch entry should trigger validation failure.',
              arguments: {
                calls: [
                  {
                    id: '',
                    tool: TOOL_NAME,
                    parameters: {
                      text: 'invalid-batch-entry',
                    },
                  },
                ],
              },
            },
            {
              toolName: LONG_TOOL_NAME,
              callId: 'call-long-tool-name',
              assistantText: 'Tool name exceeds clamp limit to trigger truncation.',
              arguments: {
                text: 'sanitizer-valid-call',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Completing final report after resolving mixed tool outcomes.',
          reportContent: `${RESULT_HEADING}${FINAL_REPORT_RETRY_MESSAGE}`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 32,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-92',
    description: 'LLM returns batched final report payload via nested calls array.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__task_status'],
        response: {
          kind: 'tool-call',
          assistantText: 'Preparing batched final report call.',
          toolCalls: [
            {
              toolName: 'agent__task_status',
              callId: 'call-batched-final-progress',
              assistantText: 'Bundling progress update before final report.',
              arguments: {
                status: STATUS_IN_PROGRESS,
                done: 'Providing name',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
          reasoning: [
            {
              type: 'reasoning',
              text: 'Batching required tool invocations before delivering final report.',
            },
          ],
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report emitted after batched tool execution.',
          reportContent: 'My name is ChatGPT.',
          reportFormat: 'pipe',
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 30,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-93',
    description: 'LLM emits reasoning-only turn that must not trigger retry before final report.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'text',
          assistantText: '',
          finishReason: 'other',
          reasoning: [
            {
              type: 'reasoning',
              text: 'Analyzing request before producing final report.',
              providerMetadata: { anthropic: { signature: 'reasoning-only-turn-1' } },
            },
          ],
          tokenUsage: {
            inputTokens: 70,
            outputTokens: 20,
            totalTokens: 90,
          },
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Delivering final report after reasoning-only turn.',
          reportContent: `${RESULT_HEADING}Reasoning-only turn accepted without retry.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          reasoning: [
            {
              type: 'reasoning',
              text: 'Finalizing response after reasoning-only analysis.',
              providerMetadata: { anthropic: { signature: 'reasoning-only-turn-2' } },
            },
          ],
          tokenUsage: {
            inputTokens: 85,
            outputTokens: 34,
            totalTokens: 119,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-124',
    description: 'LLM provides reasoning_content without assistant text or tool calls.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'text',
          assistantText: '',
          finishReason: 'stop',
          tokenUsage: {
            inputTokens: 72,
            outputTokens: 42,
            totalTokens: 114,
          },
          reasoningContent: 'Evaluating instructions and preparing the final report tool invocation.',
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Completing request after reasoning-only turn.',
          reportContent: `${RESULT_HEADING}Reasoning-content only response handled successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 36,
            totalTokens: 124,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-131',
    description: 'LLM parameter warning logging coverage.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: TOOL_REQUEST_TEXT,
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-param-warning',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          providerMetadata: {
            parameterWarnings: [
              {
                toolCallId: 'call-param-warning',
                toolName: TOOL_NAME,
                reason: 'invalid_json_parameters',
                rawPreview: '{"text":"broken',
                source: 'content',
              },
            ],
          },
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Parameter warnings logged.',
          reportContent: `${RESULT_HEADING}Parameter warning coverage.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-context-limit',
    description: 'Context guard forces final turn when tool output would overflow.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Attempting to fetch large dataset.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-overflow',
              assistantText: 'Retrieving expansive payload.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Context window nearly exhausted; delivering final report.',
          reportContent: `${RESULT_HEADING}Final answer provided without additional tool data.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 30,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-limit-default',
    description: 'Context guard fires using the internal default context window when output tokens reserve is too large.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Attempting to fetch dataset without explicit window config.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-overflow',
              assistantText: 'Retrieving expansive payload with default window.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Internal guard triggered; providing final report.',
          reportContent: `${RESULT_HEADING}Final answer issued after default context budget enforcement.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 32,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-bulk-tools',
    description: 'Context guard trims newest tool outputs when multiple large results arrive in a single turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering multiple large datasets before trimming.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-bulk-1',
              assistantText: 'Fetching dataset alpha.',
              arguments: { text: TOOL_ARGUMENT_LONG_OUTPUT },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-context-bulk-2',
              assistantText: 'Fetching dataset beta.',
              arguments: { text: TOOL_ARGUMENT_LONG_OUTPUT },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-context-bulk-3',
              assistantText: 'Fetching dataset gamma.',
              arguments: { text: TOOL_ARGUMENT_LONG_OUTPUT },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Bulk tool outputs trimmed; delivering final report.',
          reportContent: `${RESULT_HEADING}Bulk fetch completed with trimmed context to stay within limits.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 32,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-tokenizer-drift',
    description: 'Context guard tolerates tokenizer drift while still enforcing final turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting data with approximate token estimates.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-drift',
              assistantText: 'Pulling drift-prone payload.',
              arguments: { text: TOOL_ARGUMENT_LONG_OUTPUT },
            },
          ],
          tokenUsage: {
            inputTokens: 160,
            outputTokens: 60,
            totalTokens: 220,
          },
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Tokenizer drift accounted for; responding within limits.',
          reportContent: `${RESULT_HEADING}Final answer provided after reconciling tokenizer estimates.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 28,
            totalTokens: 118,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-cache-tokens',
    description: 'Context guard accounts for cache read/write tokens while enforcing limits.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Loading cached insights prior to finalization.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-cache',
              assistantText: 'Retrieving cached payload.',
              arguments: { text: TOOL_ARGUMENT_LONG_OUTPUT },
            },
          ],
          tokenUsage: {
            inputTokens: 140,
            outputTokens: 50,
            totalTokens: 190,
            cacheWriteInputTokens: 45,
          },
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Cache tokens reconciled; delivering cached summary.',
          reportContent: `${RESULT_HEADING}Final answer incorporates cached context within window.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 92,
            outputTokens: 30,
            totalTokens: 122,
            cacheReadInputTokens: 40,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-multi-provider',
    description: 'Context guard skips primary provider and succeeds with secondary.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Primary provider lacks budget; attempting tool call.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-fallback',
              assistantText: 'Requesting expansive payload to test fallback.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Secondary provider completed work after primary budget exhaustion.',
          reportContent: `${RESULT_HEADING}Secondary provider produced the final report successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 92,
            outputTokens: 31,
            totalTokens: 123,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-retry',
    description: 'Context guard handles LLM retry before enforcing final turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        failuresBeforeSuccess: 1,
        failureStatus: 'model_error',
        failureRetryable: true,
        failureMessage: CONTEXT_RETRY_FAILURE_MESSAGE,
        response: {
          kind: 'tool-call',
          assistantText: 'Retrying after failure and fetching large dataset.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-retry',
              assistantText: 'Gathering expansive payload on retry.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Context guard enforced final turn following retry.',
          reportContent: `${RESULT_HEADING}Final answer produced after handling retry and trimming tool output.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 32,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-trim-log',
    description: 'Context guard trims a single oversized tool payload and emits warning telemetry before final turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting a large payload prior to finalising.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-trim-log',
              assistantText: 'Requesting oversized dataset for trim coverage.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Trim applied; proceeding with final report.',
          reportContent: `${RESULT_HEADING}Trimmed payload and completed within the context window.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 84,
            outputTokens: 28,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-forced-final',
    description: 'Context guard enforces a forced final turn after trimming cannot restore headroom.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting extensive data before forced final turn.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-forced-final',
              assistantText: 'Aggregating oversized payload that risks the context budget.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Forced final turn executed after guard intervention.',
          reportContent: `${RESULT_HEADING}Final answer provided after the context guard forced a concluding turn.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 26,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-llm-context-metrics',
    description: 'LLM request logs expose ctx/new/schema expected metrics before final turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering data prior to final report for metrics inspection.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-llm-context-metrics',
              assistantText: 'Fetching summary payload for metrics coverage.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Returning final report after verifying context metrics.',
          reportContent: `${RESULT_HEADING}Context metrics telemetry validated successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 26,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-tool-log-tokens',
    description: 'Tool success log records tokenizer estimate for emitted payload.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Invoking test tool to capture token metrics.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-tool-log-tokens',
              assistantText: 'Executing deterministic tool for token logging coverage.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Tool completed; sharing final report after logging tokens.',
          reportContent: `${RESULT_HEADING}Tool token metrics captured successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 80,
            outputTokens: 24,
            totalTokens: 104,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-token-double-count',
    description: 'Single tool output should contribute tokens once to pending context before next LLM turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering concise data before continuing.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-double-count',
              assistantText: 'Requesting small payload for context accounting.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Providing final report after verifying context counters.',
          reportContent: `${RESULT_HEADING}Final answer delivered with correct context accounting.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 26,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__init_counters_from_history',
    description: 'Conversation history seeds context counters before the first LLM attempt.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Reviewing prior context before fetching new data.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-history-seed',
              assistantText: 'Requesting supplemental insight with history awareness.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report generated after validating history-seeded counters.',
          reportContent: `${RESULT_HEADING}Historical context respected while producing the final answer.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 26,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__schema_tokens_only',
    description: 'Context guard forces final turn based solely on schema tokens.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Schema-only overflow resolved by proceeding directly to final report.',
          reportContent: `${RESULT_HEADING}Final answer produced after schema guard enforcement.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 60,
            outputTokens: 24,
            totalTokens: 84,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__llm_metrics_logging',
    description: 'LLM request logs expose ctx/new/schema expected metrics before final turn.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering data prior to final report for metrics inspection.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-cg-llm-metrics',
              assistantText: 'Fetching summary payload for metrics coverage.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Returning final report after verifying context metrics.',
          reportContent: `${RESULT_HEADING}Context metrics telemetry validated successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 86,
            outputTokens: 26,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__forced_final_turn_flow',
    description: 'Context guard enforces a forced final turn after trimming cannot restore headroom.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Collecting extensive data before forced final turn.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-cg-forced-final',
              assistantText: 'Aggregating oversized payload that risks the context budget.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Forced final turn executed after guard intervention.',
          reportContent: `${RESULT_HEADING}Final answer provided after the context guard forced a concluding turn.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 82,
            outputTokens: 26,
            totalTokens: 108,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__multi_target_provider_selection',
    description: 'Context guard skips primary provider and succeeds with secondary.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Primary provider lacks budget; attempting tool call.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-cg-multi-provider',
              assistantText: 'Requesting expansive payload to test fallback.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Fallback provider completed final report after guard intervention.',
          reportContent: `${RESULT_HEADING}Final answer delivered using secondary provider once guard skipped the primary target.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 28,
            totalTokens: 116,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__tool_success_tokens_once',
    description: 'Two successful tool outputs in one turn should contribute tokens exactly once.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Gathering multiple datapoints before concluding.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-double-success-1',
              assistantText: 'Collecting first datapoint.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-context-double-success-2',
              assistantText: 'Collecting second datapoint.',
              arguments: {
                text: `${TOOL_ARGUMENT_SUCCESS}-secondary`,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Reporting combined findings from both tool outputs.',
          reportContent: `${RESULT_HEADING}Aggregated the two tool responses without double counting tokens.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 84,
            outputTokens: 28,
            totalTokens: 112,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__tool_drop_after_success',
    description: 'Second heavy tool output must be dropped once the first reservation consumes the budget.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting two heavy tool outputs to exercise the context guard.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-guard-drop-first',
              assistantText: 'Issuing first heavy payload.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-guard-drop-second',
              assistantText: 'Issuing second heavy payload.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after context guard drop enforcement.',
          reportContent: `${RESULT_HEADING}Second heavy tool output dropped after guard enforcement.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 28,
            totalTokens: 118,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__progress_passthrough',
    description: 'Progress updates must still execute even after the context guard drops another tool.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting a heavy tool output followed by a progress update.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-progress-guard-heavy',
              assistantText: 'Issuing heavy payload before progress update.',
              arguments: {
                text: TOOL_ARGUMENT_CONTEXT_OVERFLOW,
              },
            },
            {
              toolName: 'agent__task_status',
              callId: 'call-progress-guard-status',
              assistantText: 'Streaming progress after guard activation.',
              arguments: {
                status: STATUS_IN_PROGRESS,
                done: 'Still collecting GitHub issues despite guard pressure.',
                pending: 'Continue processing',
                now: 'Guard passthrough test',
                ready_for_final_report: false,
                need_to_run_more_tools: true,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing after guard enforcement and progress update.',
          reportContent: `${RESULT_HEADING}Progress report streamed successfully even though the guard trimmed other tools.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 28,
            totalTokens: 118,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__batch_passthrough',
    description: 'Batch wrapper response should never be dropped; only nested calls are budgeted.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__batch'],
        response: {
          kind: 'tool-call',
          assistantText: 'Executing batched heavy requests.',
          toolCalls: [
            {
              toolName: 'agent__batch',
              callId: 'call-batch-guard',
              assistantText: 'Batch issues multiple heavy calls.',
              arguments: {
                calls: [
                  {
                    id: '1',
                    tool: TOOL_NAME,
                    parameters: { text: TOOL_ARGUMENT_CONTEXT_OVERFLOW },
                  },
                  {
                    id: '2',
                    tool: TOOL_NAME,
                    parameters: { text: TOOL_ARGUMENT_CONTEXT_OVERFLOW },
                  },
                  {
                    id: '3',
                    tool: TOOL_NAME,
                    parameters: { text: TOOL_ARGUMENT_CONTEXT_OVERFLOW },
                  },
                ],
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Summarizing batched heavy outputs.',
          reportContent: `${RESULT_HEADING}Batch response remained available even after guard activation.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 92,
            outputTokens: 30,
            totalTokens: 122,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__tool_overflow_drop',
    description: 'Oversized tool output is dropped and replaced with a failure stub.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting an extended dataset expected to overflow context.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-overflow-drop',
              assistantText: 'Attempting to stream a large payload.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Proceeding after overflow drop to summarise available data.',
          reportContent: `${RESULT_HEADING}Context guard trimmed the oversized tool payload and continued safely.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 90,
            outputTokens: 30,
            totalTokens: 120,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__post_overflow_tools_skip',
    description: 'After an overflow, subsequent tool calls are skipped with budget_exceeded_prior_tool.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Issuing two tool calls; the first will overflow the budget.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-context-overflow-first',
              assistantText: 'Fetching a verbose dataset that should overflow.',
              arguments: {
                text: TOOL_ARGUMENT_LONG_OUTPUT,
              },
            },
            {
              toolName: TOOL_NAME,
              callId: 'call-context-overflow-second',
              assistantText: 'Attempting a follow-up query after overflow.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Concluding after overflow prevented additional tool execution.',
          reportContent: `${RESULT_HEADING}Second tool request was skipped because the budget had already been exceeded.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 88,
            outputTokens: 28,
            totalTokens: 116,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-refusal',
    description: 'Provider refuses output with content-filter stop reason; should be treated as failure.',
    turns: [
      {
        turn: 1,
        response: {
          kind: 'text',
          assistantText: '',
          finishReason: 'content-filter',
          tokenUsage: {
            inputTokens: 64,
            outputTokens: 0,
            totalTokens: 64,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__threshold_probe',
    description: 'Minimal final report to measure context guard thresholds.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Delivering the threshold probe summary.',
          reportContent: `${RESULT_HEADING}Threshold probe completed without additional tool usage.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 48,
            outputTokens: 20,
            totalTokens: 68,
          },
        },
      },
    ],
  },
  {
    id: 'context_guard__threshold_above_probe',
    description: 'Final report with failure status for above-limit context threshold tests.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Context limit exceeded at start; reporting failure.',
          reportContent: `${RESULT_HEADING}Context window exceeded before processing could begin.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 48,
            outputTokens: 20,
            totalTokens: 68,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-context-guard-preflight',
    description: 'Context guard fires at preflight due to history exceeding available budget; forced final turn produces failure report.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        // Turn 2 because conversationHistory includes an assistant message
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Context budget exceeded; unable to complete task.',
          reportContent: `${RESULT_HEADING}Context budget exceeded before processing could begin.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: {
            inputTokens: 24,
            outputTokens: 12,
            totalTokens: 36,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-69',
    description: 'Model override snake-case top_p propagation (no temperature requirement).',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTopP: 0.66,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Verifying top_p override propagation.',
          reportContent: `${RESULT_HEADING}top_p override applied successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: {
            inputTokens: 48,
            outputTokens: 20,
            totalTokens: 68,
          },
        },
      },
    ],
  },
  {
    id: 'run-test-invalid-response-clears-after-success',
    description: 'Early invalid responses in a turn are cleared once a tool-call attempt succeeds.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 2,
        failureStatus: 'invalid_response',
        failureMessage: 'Simulated empty response.',
        response: {
          kind: 'tool-call',
          assistantText: 'Calling tool after prior invalid responses.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-invalid-clear-1',
              assistantText: 'Proceeding with real tool call.',
              arguments: {
                text: TOOL_ARGUMENT_SUCCESS,
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Final report after clearing invalid state.',
          reportContent: `${RESULT_HEADING}Invalid responses were cleared after successful tool call.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  // -------------------------------------------------------------------------
  // Leaked tool extraction scenarios (fallback for <tools> / <tool_call> etc.)
  // -------------------------------------------------------------------------
  {
    id: 'run-test-leaked-tools-success',
    description: 'Leaked <tools> XML pattern is extracted and executed successfully without TURN_FAILED.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        // allowMissingTools: true because the response is 'text' kind with embedded <tools>
        allowMissingTools: true,
        response: {
          kind: 'text',
          assistantText: `I will call the test tool now.\n\n<tools>{"name": "${TOOL_NAME}", "arguments": {"text": "${TOOL_ARGUMENT_SUCCESS}"}}</tools>`,
          finishReason: 'stop',
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Tool executed via fallback extraction.',
          reportContent: `${RESULT_HEADING}Leaked tool pattern was extracted and executed successfully.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-leaked-tool-call-success',
    description: 'Leaked <tool_call> XML pattern is extracted and executed successfully.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        allowMissingTools: true,
        response: {
          kind: 'text',
          assistantText: `Calling test tool via <tool_call> format.\n\n<tool_call>\n{"name": "${TOOL_NAME}", "arguments": {"text": "${TOOL_ARGUMENT_SUCCESS}"}}\n</tool_call>`,
          finishReason: 'stop',
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Completed after <tool_call> extraction.',
          reportContent: `${RESULT_HEADING}Leaked <tool_call> pattern extracted and executed.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-leaked-tools-unknown-tool',
    description: 'Leaked <tools> with unknown tool name surfaces error but no TURN_FAILED_NO_TOOLS message.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        allowMissingTools: true,
        response: {
          kind: 'text',
          assistantText: `<tools>{"name": "unknown__nonexistent_tool", "arguments": {}}</tools>`,
          finishReason: 'stop',
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Unknown tool error received; providing summary.',
          reportContent: `${RESULT_HEADING}Unknown tool was reported; fallback extraction worked.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  // =====================================================================
  // Budget Truncation Tests
  // =====================================================================
  {
    id: 'run-test-size-cap-truncation',
    description: 'Tool response exceeding toolResponseMaxBytes is truncated with marker.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Fetching large payload that will be truncated by size cap.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-size-cap',
              arguments: {
                text: 'budget-truncatable',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Received truncated tool output.',
          reportContent: `${RESULT_HEADING}Tool output was truncated to fit size cap.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-size-cap-small-payload-passes',
    description: 'Small tool response that fits size cap passes through unchanged.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Fetching small payload that fits the cap.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-small-fits',
              arguments: {
                text: 'small-fits',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Received full tool output without truncation.',
          reportContent: `${RESULT_HEADING}Small payload passed through unchanged.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-size-cap-small-over-limit-fails',
    description: 'Payload between cap and 512B minimum returns failure stub.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Fetching payload that exceeds cap but is too small to truncate.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-small-over',
              arguments: {
                text: 'small-over-limit',
              },
            },
          ],
          tokenUsage: DEFAULT_TOKEN_USAGE,
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Tool output was replaced with failure stub.',
          reportContent: `${RESULT_HEADING}Tool response exceeded max size and could not be truncated.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_FAILURE,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
  {
    id: 'run-test-budget-truncation-preserves-output',
    description: 'Tool output truncated (not dropped) when exceeds token budget but truncation fits.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        response: {
          kind: 'tool-call',
          assistantText: 'Fetching large payload that exceeds token budget.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-budget-truncate',
              arguments: {
                text: 'budget-truncatable',
              },
            },
          ],
          tokenUsage: {
            inputTokens: 800,
            outputTokens: 100,
            totalTokens: 900,
          },
          finishReason: TOOL_FINISH_REASON,
        },
      },
      {
        turn: 2,
        expectedTools: [],
        response: {
          kind: FINAL_RESPONSE_KIND,
          assistantText: 'Received budget-truncated tool output with marker.',
          reportContent: `${RESULT_HEADING}Tool output was truncated to fit token budget.`,
          reportFormat: MARKDOWN_FORMAT,
          status: STATUS_SUCCESS,
          tokenUsage: DEFAULT_TOKEN_USAGE,
        },
      },
    ],
  },
];

const scenarios = new Map<string, ScenarioDefinition>(SCENARIOS.map((scenario) => [scenario.id, scenario] as const));

export function getScenario(id: string): ScenarioDefinition | undefined {
  return scenarios.get(id);
}

export function listScenarioIds(): string[] {
  return [...scenarios.keys()];
}
