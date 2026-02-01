import { describe, expect, it } from 'vitest';

import { resolveCliOutputMode } from '../../cli-output-mode.js';

describe('cli output mode', () => {
  it('maps --chat to chat mode', () => {
    expect(resolveCliOutputMode(true)).toBe('chat');
  });

  it('maps --no-chat to agentic mode', () => {
    expect(resolveCliOutputMode(false)).toBe('agentic');
  });

  it('defaults to agentic when chat flag is unset', () => {
    expect(resolveCliOutputMode(undefined)).toBe('agentic');
  });
});
