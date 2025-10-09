import type { TokenUsage } from '../../types.js';

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
    }
  | {
      kind: 'text';
      assistantText: string;
      finishReason?: 'stop' | 'other';
      tokenUsage?: TokenUsage;
      reasoning?: string[];
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
                    args: { progress: 'Batch step started' },
                  },
                  {
                    id: '2',
                    tool: TOOL_NAME,
                    args: { text: 'batch-success' },
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
                input: 'Need supporting pricing analysis.',
                reason: 'Augment pricing context',
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
                input: 'run-test-24-subagent',
                reason: 'Gather detailed pricing insights',
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
                    args: {
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
                    args: {
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
                    args: {
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
                    args: { progress: BATCH_PROGRESS_STATUS },
                  },
                  {
                    id: 'p-2',
                    tool: TOOL_NAME,
                    args: { text: BATCH_PROGRESS_RESPONSE },
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
                calls: `[{"id":"s-1","tool":"agent__progress_report","args":{"progress":"${BATCH_STRING_PROGRESS}"}},{"id":"s-2","tool":"${TOOL_NAME}","args":{"text":"${BATCH_STRING_RESULT}"}}]`,
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
    id: 'run-test-37',
    description: 'Rate limit error triggers retry.',
    systemPromptMustInclude: [SYSTEM_PROMPT_MARKER],
    turns: [
      {
        turn: 1,
        expectedTools: [TOOL_NAME],
        failuresBeforeSuccess: 1,
        failureThrows: true,
        failureMessage: 'Rate limit simulated.',
        failureError: {
          name: 'RateLimitError',
          status: 429,
          headers: { 'retry-after': '2' },
          retryAfter: 2,
          responseBody: JSON.stringify({ error: { message: 'Rate limited', metadata: { raw: JSON.stringify({ error: { status: 'rate_limit' } }) } } }),
        },
        failureStatus: 'rate_limit',
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
];

const scenarios = new Map<string, ScenarioDefinition>(SCENARIOS.map((scenario) => [scenario.id, scenario] as const));

export function getScenario(id: string): ScenarioDefinition | undefined {
  return scenarios.get(id);
}

export function listScenarioIds(): string[] {
  return [...scenarios.keys()];
}
