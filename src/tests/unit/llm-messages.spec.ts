import { describe, expect, it } from 'vitest';

import type { ResponseMessage } from '../../llm-providers/base.js';
import type { ConversationMessage, TokenUsage, TurnRequest, TurnResult } from '../../types.js';

import { renderTurnFailedSlug } from '../../llm-messages-turn-failed.js';
import {
  XML_WRAPPER_CALLED_AS_TOOL_RESULT,
  isXmlFinalReportTagName,
} from '../../llm-messages.js';
import { BaseLLMProvider } from '../../llm-providers/base.js';

class TestProvider extends BaseLLMProvider {
  name = 'test-provider';
  // eslint-disable-next-line @typescript-eslint/no-useless-constructor -- Expose protected base constructor for testing.
  constructor() {
    super();
  }
  executeTurn(_request: TurnRequest): Promise<TurnResult> {
    return Promise.reject(new Error('not implemented'));
  }

  protected convertResponseMessages(
    messages: ResponseMessage[],
    provider: string,
    model: string,
    tokens: TokenUsage,
  ): ConversationMessage[] {
    return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
  }

  public inject(messages: ConversationMessage[], tokens: TokenUsage): ConversationMessage[] {
    return this.injectMissingToolResults(messages, 'provider', 'model', tokens);
  }
}

describe('llm-messages helpers', () => {
  it('matches XML final report tag names regardless of case', () => {
    expect(isXmlFinalReportTagName('ai-agent-deadbeef-FINAL')).toBe(true);
    expect(isXmlFinalReportTagName('ai-agent-DEADBEEF-final')).toBe(true);
    expect(isXmlFinalReportTagName('ai-agent-deadbee-FINAL')).toBe(false);
    expect(isXmlFinalReportTagName('agent-deadbeef-FINAL')).toBe(false);
  });

  it('builds TURN-FAILED message with nonce and format', () => {
    const message = renderTurnFailedSlug('xml_wrapper_as_tool', 'wrapper: <ai-agent-cafebabe-FINAL format=\"markdown\">');
    expect(message).toContain('ai-agent-cafebabe-FINAL');
    expect(message).toContain('format="markdown"');
  });
});

describe('injectMissingToolResults for XML wrapper misuse', () => {
  const tokens: TokenUsage = { inputTokens: 1, outputTokens: 1, cachedTokens: 0, totalTokens: 2 };

  it('injects specific error when XML wrapper is called as a tool', () => {
    const provider = new TestProvider();
    const messages: ConversationMessage[] = [
      {
        role: 'assistant',
        content: 'attempting final xml',
        toolCalls: [{ id: 'call-1', name: 'ai-agent-deadbeef-FINAL', parameters: {} }],
      },
    ];

    const injected: ConversationMessage[] = provider.inject(messages, tokens);
    const toolMessage = injected.find(
      (m): m is ConversationMessage & { role: 'tool'; content: string } =>
        m.role === 'tool' && typeof m.content === 'string',
    );
    if (toolMessage === undefined) {
      throw new Error('Expected injected tool failure message for XML wrapper misuse');
    }
    expect(toolMessage.content).toContain(XML_WRAPPER_CALLED_AS_TOOL_RESULT);
  });
});
