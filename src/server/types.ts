// Shared types for the server headend. Keep small and focused.

export type Source = 'slack' | 'web' | 'api';

export interface RunKey {
  source: Source;
  teamId?: string;
  channelId?: string;
  threadTsOrSessionId: string; // slack: thread_ts|ts, web: sessionId, api: runId
}

export interface RunMeta {
  runId: string;
  key: RunKey;
  startedAt: number;
  updatedAt: number;
  status: 'running' | 'stopping' | 'succeeded' | 'failed' | 'canceled';
  error?: string;
  model?: string;
}

export interface SimpleAskRequestBody {
  prompt: string;
  systemPrompt?: string;
  model?: string;
  temperature?: number;
}

export interface SimpleAskResponseBody {
  runId: string;
  status: 'running' | 'succeeded' | 'failed';
  text?: string;
  error?: string;
}
