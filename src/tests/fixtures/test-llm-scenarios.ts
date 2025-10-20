import type { ProviderTurnMetadata, TokenUsage } from '../../types.js';

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
      reasoning?: string[];
      providerMetadata?: ProviderTurnMetadata;
    }
  | {
      kind: 'final-report';
      assistantText?: string;
      reportContent: string;
      reportFormat: string;
      status?: 'success' | 'failure' | 'partial';
      reportContentJson?: Record<string, unknown>;
      tokenUsage?: TokenUsage;
      reasoning?: string[];
      finishReason?: string;
      providerMetadata?: ProviderTurnMetadata;
    }
  | {
      kind: 'text';
      assistantText: string;
      finishReason?: 'stop' | 'other';
      tokenUsage?: TokenUsage;
      reasoning?: string[];
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
const TOOL_REQUEST_TEXT = 'Requesting test to gather information.';
const TOOL_ARGUMENT_SUCCESS = 'phase-1-tool-success';
const CONCURRENCY_TIMEOUT_ARGUMENT = 'trigger-timeout';
const CONCURRENCY_SECOND_ARGUMENT = 'concurrency-second';
const BATCH_INVALID_INPUT_ARGUMENT = 'batch-missing-id';
const BATCH_UNKNOWN_TOOL = 'unknown__tool';
const BATCH_EXECUTION_ERROR_ARGUMENT = 'trigger-mcp-failure';
const STREAM_REASONING_STEPS = [
  'Deliberating over deterministic harness state.',
  'Confirming reasoning stream emission.',
] as const;
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
        failuresBeforeSuccess: 1,
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
                text: 'long-output',
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
        expectedTools: ['agent__progress_report', TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Providing progress and calling test.',
          toolCalls: [
            {
              toolName: 'agent__progress_report',
              callId: 'call-progress-1',
              assistantText: 'Reporting progress.',
              arguments: {
                progress: 'Step 1 complete',
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
        expectedTools: ['agent__final_report'],
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
                    tool: 'agent__progress_report',
                    parameters: { progress: 'Batch step started' },
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
    id: 'run-test-26',
    description: 'Batch invalid input detection (missing id).',
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
              assistantText: 'Batch entry lacks the required id field.',
              arguments: {
                calls: [
                  {
                    tool: TOOL_NAME,
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
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureStatus: 'model_error',
        failureRetryable: false,
        failureMessage: 'Simulated fatal error before manual retry.',
        response: {
          kind: 'tool-call',
          assistantText: 'Retry scenario executes after manual retry.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-retry-session',
              assistantText: 'Executing tool call after retry.',
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
          assistantText: 'Reporting retry outcome.',
          reportContent: `${RESULT_HEADING}Session retry succeeded after manual invocation.`,
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
    id: 'run-test-30',
    description: 'Streaming reasoning/thinking emission.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: ['agent__final_report'],
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
                    tool: 'agent__progress_report',
                    parameters: { progress: BATCH_PROGRESS_STATUS },
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
                calls: `[{"id":"s-1","tool":"agent__progress_report","parameters":{"progress":"${BATCH_STRING_PROGRESS}"}},{"id":"s-2","tool":"${TOOL_NAME}","parameters":{"text":"${BATCH_STRING_RESULT}"}}]`,
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
        expectedTools: ['agent__final_report'],
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
          reportContent: `${RESULT_HEADING}*Slack* message generated for delivery.`,
          reportFormat: SLACK_BLOCK_KIT_FORMAT,
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
          kind: 'tool-call',
          assistantText: 'Preparing Slack batch.',
          toolCalls: [
            {
              toolName: 'agent__final_report',
              callId: 'call-slack-messages',
              assistantText: 'Sending rich Slack content.',
              arguments: {
                status: 'success',
                report_format: SLACK_BLOCK_KIT_FORMAT,
                messages: [
                  'Primary message with *formatting*',
                  { blocks: [{ type: 'section', text: { type: 'mrkdwn', text: '*Detail* section with field' }, fields: [{ type: 'mrkdwn', text: 'Field line' }] }] },
                  JSON.stringify({ type: 'divider' }),
                  [
                    JSON.stringify({ type: 'header', text: { type: 'plain_text', text: 'Header Title' } }),
                    { type: 'context', elements: ['Context line 1', { text: 'Context line 2', type: 'mrkdwn' }] }
                  ]
                ],
                metadata: { slack: { footer: 'Existing metadata' } },
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
        expectedTools: ['agent__progress_report', 'agent__final_report'],
        response: {
          kind: 'tool-call',
          assistantText: 'Issuing progress update and Slack final report.',
          toolCalls: [
            {
              toolName: 'agent__progress_report',
              callId: 'call-progress-run-test-57',
              assistantText: 'Reporting current progress.',
              arguments: {
                progress: 'Analyzing deterministic harness outputs.',
              },
            },
            {
              toolName: 'agent__final_report',
              callId: 'call-slack-run-test-57',
              assistantText: 'Submitting Slack styled final report.',
              arguments: {
                status: 'success',
                report_format: SLACK_BLOCK_KIT_FORMAT,
                messages: [
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
                metadata: { slack: { footer: 'Existing footer' } },
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
          reportFormat: MARKDOWN_FORMAT,
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
        expectedTools: ['agent__progress_report', TOOL_NAME],
        response: {
          kind: 'tool-call',
          assistantText: 'Publishing progress and querying MCP tool.',
          toolCalls: [
            {
              toolName: 'agent__progress_report',
              callId: 'call-progress-run-test-58',
              assistantText: 'Progress update before invoking MCP tool.',
              arguments: {
                progress: 'Collecting metrics via test MCP tool.',
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
                  { id: 'p-1', tool: 'agent__progress_report', parameters: { progress: 'Starting large data export.' } },
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
          kind: 'tool-call',
          assistantText: 'Submitting final report without content to trigger validation.',
          toolCalls: [
            {
              toolName: 'agent__final_report',
              callId: 'call-invalid-final-report',
              assistantText: 'Attempted final report with missing content.',
              arguments: {
                status: STATUS_SUCCESS,
                report_format: MARKDOWN_FORMAT,
                report_content: '   ',
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
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
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
        expectedTools: ['agent__batch', LONG_TOOL_NAME, 'agent__final_report'],
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
                    tool: 'agent__progress_report',
                    parameters: { progress: 'Valid batch progress entry.' },
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
            {
              toolName: 'agent__final_report',
              callId: 'call-final-invalid',
              assistantText: 'Final report missing content should fail and require retry.',
              arguments: {
                status: STATUS_SUCCESS,
                report_format: MARKDOWN_FORMAT,
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
];

const scenarios = new Map<string, ScenarioDefinition>(SCENARIOS.map((scenario) => [scenario.id, scenario] as const));

export function getScenario(id: string): ScenarioDefinition | undefined {
  return scenarios.get(id);
}

export function listScenarioIds(): string[] {
  return [...scenarios.keys()];
}
