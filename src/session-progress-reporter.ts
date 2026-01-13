import type {
  AgentFailedEvent,
  AgentFinishedEvent,
  AgentStartedEvent,
  AgentUpdateEvent,
  ProgressEvent,
  ProgressMetrics,
  TaskStatusData,
  ToolFinishedEvent,
  ToolStartedEvent,
} from './types.js';

interface AgentStartPayload {
  callPath: string;
  agentId: string;
  agentPath: string;
  agentName?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  reason?: string;
}

interface AgentUpdatePayload {
  callPath: string;
  agentId: string;
  agentPath: string;
  agentName?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  message: string;
  taskStatus?: TaskStatusData;
}

interface AgentCompletionPayload {
  callPath: string;
  agentId: string;
  agentPath: string;
  agentName?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  metrics?: ProgressMetrics;
  error?: string;
}

interface ToolEventPayload {
  callPath: string;
  agentId: string;
  agentPath: string;
  tool: { name: string; provider: string };
  metrics?: ProgressMetrics;
  error?: string;
  status?: 'ok' | 'failed';
}

export class SessionProgressReporter {
  private readonly emit?: (event: ProgressEvent) => void;
  private readonly lastUpdateByCallPath = new Map<string, string>();

  public constructor(emit?: (event: ProgressEvent) => void) {
    this.emit = emit;
  }

  public agentStarted(payload: AgentStartPayload): void {
    if (this.emit === undefined) return;
    const event: AgentStartedEvent = {
      type: 'agent_started',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      agentName: payload.agentName,
      txnId: payload.txnId,
      parentTxnId: payload.parentTxnId,
      originTxnId: payload.originTxnId,
      reason: payload.reason,
      timestamp: Date.now(),
    };
    this.emit(event);
  }

  public agentUpdate(payload: AgentUpdatePayload): void {
    if (this.emit === undefined) return;
    const trimmed = payload.message.trim();
    if (trimmed.length === 0) return;
    const previous = this.lastUpdateByCallPath.get(payload.callPath);
    if (previous === trimmed) return;
    this.lastUpdateByCallPath.set(payload.callPath, trimmed);
    const event: AgentUpdateEvent = {
      type: 'agent_update',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      agentName: payload.agentName,
      txnId: payload.txnId,
      parentTxnId: payload.parentTxnId,
      originTxnId: payload.originTxnId,
      message: trimmed,
      taskStatus: payload.taskStatus,
      timestamp: Date.now(),
    };
    this.emit(event);
  }

  public agentFinished(payload: AgentCompletionPayload): void {
    if (this.emit === undefined) return;
    const event: AgentFinishedEvent = {
      type: 'agent_finished',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      agentName: payload.agentName,
      txnId: payload.txnId,
      parentTxnId: payload.parentTxnId,
      originTxnId: payload.originTxnId,
      metrics: payload.metrics,
      timestamp: Date.now(),
    };
    this.emit(event);
    this.lastUpdateByCallPath.delete(payload.callPath);
  }

  public agentFailed(payload: AgentCompletionPayload): void {
    if (this.emit === undefined) return;
    const event: AgentFailedEvent = {
      type: 'agent_failed',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      agentName: payload.agentName,
      txnId: payload.txnId,
      parentTxnId: payload.parentTxnId,
      originTxnId: payload.originTxnId,
      metrics: payload.metrics,
      error: payload.error,
      timestamp: Date.now(),
    };
    this.emit(event);
    this.lastUpdateByCallPath.delete(payload.callPath);
  }

  public toolStarted(payload: ToolEventPayload): void {
    if (this.emit === undefined) return;
    const event: ToolStartedEvent = {
      type: 'tool_started',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      tool: payload.tool,
      metrics: payload.metrics,
      timestamp: Date.now(),
    };
    this.emit(event);
  }

  public toolFinished(payload: ToolEventPayload): void {
    if (this.emit === undefined) return;
    const hasError = payload.error !== undefined && payload.error.length > 0;
    const fallbackStatus: 'ok' | 'failed' = hasError ? 'failed' : 'ok';
    const status: 'ok' | 'failed' = payload.status ?? fallbackStatus;
    const event: ToolFinishedEvent = {
      type: 'tool_finished',
      callPath: payload.callPath,
      agentId: payload.agentId,
      agentPath: payload.agentPath,
      tool: payload.tool,
      metrics: payload.metrics,
      status,
      error: payload.error,
      timestamp: Date.now(),
    };
    this.emit(event);
  }
}
