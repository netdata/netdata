import { describe, expect, it } from 'vitest';

import type { AIAgentEvent, AIAgentEventMeta } from '../../types.js';

import { createHeadendEventState, markHandoffSeen, shouldAcceptFinalReport, shouldStreamOutput } from '../../headends/shared-event-filter.js';

const baseMeta = (overrides?: Partial<AIAgentEventMeta>): AIAgentEventMeta => ({
  isMaster: true,
  isFinal: true,
  pendingHandoffCount: 0,
  handoffConfigured: false,
  sequence: 1,
  ...overrides,
});

describe('shared-event-filter', () => {
  it('shouldStreamOutput suppresses non-master output', () => {
    const event: AIAgentEvent = { type: 'output', text: 'hi' };
    const meta = baseMeta({ isMaster: false });
    expect(shouldStreamOutput(event, meta)).toBe(false);
  });

  it('shouldStreamOutput suppresses finalize output when pending handoff exists', () => {
    const event: AIAgentEvent = { type: 'output', text: 'final' };
    const meta = baseMeta({ source: 'finalize', pendingHandoffCount: 1 });
    expect(shouldStreamOutput(event, meta)).toBe(false);
  });

  it('shouldStreamOutput allows finalize output when no pending handoff', () => {
    const event: AIAgentEvent = { type: 'output', text: 'final' };
    const meta = baseMeta({ source: 'finalize', pendingHandoffCount: 0 });
    expect(shouldStreamOutput(event, meta)).toBe(true);
  });

  it('shouldAcceptFinalReport respects handoff markers', () => {
    const state = createHeadendEventState();
    const meta = baseMeta({ sessionId: 'session-1' });
    expect(shouldAcceptFinalReport(state, meta)).toBe(true);
    markHandoffSeen(state, meta);
    expect(shouldAcceptFinalReport(state, meta)).toBe(false);
  });

  it('shouldAcceptFinalReport requires master + final', () => {
    const state = createHeadendEventState();
    expect(shouldAcceptFinalReport(state, baseMeta({ isMaster: false }))).toBe(false);
    expect(shouldAcceptFinalReport(state, baseMeta({ isFinal: false }))).toBe(false);
  });
});
