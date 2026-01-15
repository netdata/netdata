import { describe, expect, it } from 'vitest';

import { buildPromptSections } from '../../headends/completions-prompt.js';

describe('buildPromptSections', () => {
  it('formats system, history, and user sections', () => {
    const prompt = buildPromptSections({
      systemParts: ['system one', 'system two'],
      historyParts: ['User: hi', 'Assistant: hello'],
      lastUser: 'final request',
    });
    expect(prompt).toBe(
      'System context:\nsystem one\nsystem two\n\nConversation so far:\nUser: hi\nAssistant: hello\n\nUser request:\nfinal request',
    );
  });

  it('omits empty sections', () => {
    const prompt = buildPromptSections({
      systemParts: [],
      historyParts: [],
      lastUser: 'only user',
    });
    expect(prompt).toBe('User request:\nonly user');
  });
});
