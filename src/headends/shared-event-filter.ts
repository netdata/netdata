import type { AIAgentEvent, AIAgentEventMeta } from '../types.js';

export interface HeadendEventState {
  handoffSessions: Set<string>;
}

export const createHeadendEventState = (): HeadendEventState => ({
  handoffSessions: new Set<string>(),
});

const eventSessionKey = (meta: AIAgentEventMeta): string | undefined =>
  meta.sessionId ?? meta.callPath ?? meta.agentId;

export const markHandoffSeen = (state: HeadendEventState, meta: AIAgentEventMeta): void => {
  const key = eventSessionKey(meta);
  if (key === undefined) return;
  state.handoffSessions.add(key);
};

export const shouldStreamMasterContent = (meta: AIAgentEventMeta): boolean => meta.isMaster;

export const shouldStreamTurnStarted = (meta: AIAgentEventMeta): boolean => meta.isMaster;

export const shouldStreamOutput = (event: AIAgentEvent, meta: AIAgentEventMeta): boolean => {
  if (!meta.isMaster) return false;
  if (event.type === 'output' && meta.source === 'finalize' && meta.pendingHandoffCount > 0) return false;
  return true;
};

export const shouldAcceptFinalReport = (
  state: HeadendEventState,
  meta: AIAgentEventMeta,
): boolean => {
  if (!meta.isMaster || !meta.isFinal) return false;
  const key = eventSessionKey(meta);
  if (key === undefined) return true;
  return !state.handoffSessions.has(key);
};
