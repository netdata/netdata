import path from 'node:path';
import { fileURLToPath } from 'node:url';

import type { AIAgentResult, AIAgentSessionConfig, AccountingEntry, Configuration } from '../types.js';

import { AIAgentSession } from '../ai-agent.js';
import { sanitizeToolName } from '../utils.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

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
      const expectedTool = 'toy__toy';
      const sanitizedExpectedTool = sanitizeToolName(expectedTool);
      invariant(
        toolCallNames.includes(expectedTool) || toolCallNames.includes(sanitizedExpectedTool),
        'Expected toy__toy tool call in first turn for run-test-1.',
      );

      const toolEntries = result.accounting.filter(isToolAccounting);
      const toyEntry = toolEntries.find((entry) => entry.mcpServer === 'toy' && entry.command === 'toy__toy');
      invariant(toyEntry !== undefined, 'Expected accounting entry for toy MCP server in run-test-1.');
      invariant(toyEntry.status === 'ok', 'Toy MCP tool accounting should be ok for run-test-1.');
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
      const failureEntry = toolEntries.find((entry) => entry.command === 'toy__toy');
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
];

async function runScenario(prompt: string): Promise<AIAgentResult> {
  const serverPath = path.resolve(__dirname, 'mcp/toy-stdio-server.js');

  const configuration: Configuration = {
    providers: {
      'phase1-provider': {
        type: 'test-llm',
      },
    },
    mcpServers: {
      toy: {
        type: 'stdio',
        command: process.execPath,
        args: [serverPath],
      },
    },
    defaults: {
      stream: false,
      maxToolTurns: 3,
      maxRetries: 2,
      llmTimeout: 10_000,
      toolTimeout: 10_000,
    },
  };

  const sessionConfig: AIAgentSessionConfig = {
    config: configuration,
    targets: [{ provider: 'phase1-provider', model: 'deterministic-model' }],
    tools: ['toy'],
    systemPrompt: 'Phase 1 deterministic harness: respond using scripted outputs.',
    userPrompt: prompt,
    outputFormat: 'markdown',
    stream: false,
    parallelToolCalls: false,
    maxTurns: 3,
    toolTimeout: 10_000,
    llmTimeout: 10_000,
    agentId: `phase1-${prompt}`,
  };

  const session = AIAgentSession.create(sessionConfig);
  return await session.run();
}

async function runPhaseOne(): Promise<void> {
  // eslint-disable-next-line functional/no-loop-statements
  for (const scenario of TEST_SCENARIOS) {
    const result = await runScenario(scenario.id);
    scenario.expect(result);
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
