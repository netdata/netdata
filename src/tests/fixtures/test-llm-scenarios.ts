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
    }
  | {
      kind: 'final-report';
      assistantText?: string;
      reportContent: string;
      reportFormat: 'markdown';
      status?: 'success' | 'failure' | 'partial';
      tokenUsage?: TokenUsage;
    }
  | {
      kind: 'text';
      assistantText: string;
      finishReason?: 'stop' | 'other';
      tokenUsage?: TokenUsage;
    };

export interface ScenarioTurn {
  turn: number;
  expectedTools?: string[];
  response: ScenarioStepResponse;
  failuresBeforeSuccess?: number;
  failureMessage?: string;
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
const TOOL_NAME = 'toy__toy';
const TOOL_FINISH_REASON = 'tool-calls';
const RESULT_HEADING = '# Phase 1 Result\n\n';
const FINAL_RESPONSE_KIND = 'final-report' as const;
const MARKDOWN_FORMAT = 'markdown' as const;
const STATUS_SUCCESS = 'success' as const;
const STATUS_FAILURE = 'failure' as const;

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
          assistantText: 'Requesting toy to gather information.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-toy-1',
              assistantText: 'Preparing toy input.',
              arguments: {
                text: 'phase-1-tool-success',
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
          assistantText: 'Requesting toy to gather information.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-toy-fail-1',
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
        failureMessage: 'Simulated LLM failure (retry expected).',
        response: {
          kind: 'tool-call',
          assistantText: 'Requesting toy after retry.',
          toolCalls: [
            {
              toolName: TOOL_NAME,
              callId: 'call-toy-retry-1',
              assistantText: 'Preparing toy input after retry.',
              arguments: {
                text: 'phase-1-tool-success',
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
];

const scenarios = new Map<string, ScenarioDefinition>(SCENARIOS.map((scenario) => [scenario.id, scenario] as const));

export function getScenario(id: string): ScenarioDefinition | undefined {
  return scenarios.get(id);
}

export function listScenarioIds(): string[] {
  return [...scenarios.keys()];
}
