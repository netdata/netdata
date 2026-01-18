import { describe, expect, it } from 'vitest';

import type { AIAgentResult, Configuration, ConversationMessage, OrchestrationRuntimeAgent } from '../../../types.js';

import { spawnOrchestrationChild } from '../../../orchestration/spawn-child.js';

const baseConfiguration: Configuration = {
  providers: {},
  mcpServers: {},
  queues: { default: { concurrent: 1 } },
};
const CHILD_PATH = './child.ai';

describe('spawnOrchestrationChild', () => {
  it('passes parent conversation history to child agent', async () => {
    const history: ConversationMessage[] = [
      { role: 'user', content: 'hello' },
      { role: 'assistant', content: 'hi' },
    ];
    let receivedHistory: ConversationMessage[] | undefined;
    const agent: OrchestrationRuntimeAgent = {
      ref: CHILD_PATH,
      path: CHILD_PATH,
      agentId: 'child',
      promptPath: CHILD_PATH,
      systemTemplate: 'child system',
      run: (_systemPrompt, _userPrompt, opts) => {
        receivedHistory = opts?.history;
        const result: AIAgentResult = {
          success: true,
          conversation: [],
          logs: [],
          accounting: [],
        };
        return Promise.resolve(result);
      },
    };

    await spawnOrchestrationChild({
      agent,
      systemTemplate: agent.systemTemplate,
      userPrompt: 'test prompt',
      parentSession: {
        config: baseConfiguration,
        callbacks: undefined,
        stream: false,
        traceLLM: false,
        traceMCP: false,
        traceSdk: false,
        verbose: false,
        temperature: null,
        topP: null,
        topK: null,
        llmTimeout: 0,
        toolTimeout: 0,
        maxRetries: 0,
        maxTurns: 1,
        toolResponseMaxBytes: 0,
        targets: [],
        tools: [],
        isMaster: true,
        pendingHandoffCount: 0,
        conversationHistory: history,
      },
    });

    expect(receivedHistory).toEqual(history);
  });
});
