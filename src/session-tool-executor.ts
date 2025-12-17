import type { FinalReportManager } from './final-report-manager.js';
import type { ToolsOrchestrator } from './tools/tools.js';
import type {
  AccountingEntry,
  ConversationMessage,
  LogEntry,
} from './types.js';
import type { XmlToolTransport } from './xml-transport.js';

import { ContextGuard } from './context-guard.js';
import { TOOL_NO_OUTPUT } from './llm-messages.js';
import { addSpanEvent } from './telemetry/index.js';
import { truncateToBytes } from './truncation.js';
import {
  clampToolName,
  estimateMessagesBytes,
  parseJsonRecord,
  sanitizeToolName,
} from './utils.js';

export interface ToolExecutionState {
  toolFailureMessages: Map<string, string>;
  toolFailureFallbacks: string[];
  trimmedToolCallIds: Set<string>;
  turnHasSuccessfulToolOutput: boolean;
  incompleteFinalReportDetected: boolean;
  toolLimitExceeded: boolean;
  finalReportToolFailed: boolean;
  executedTools: number;
  executedNonProgressBatchTools: number;
  executedProgressBatchTools: number;
  unknownToolEncountered: boolean;
  lastTaskStatusCompleted?: boolean;
  productiveToolExecutedThisTurn: boolean;
  // Callback to signal task completion to TurnRunner for immediate final turn
  onTaskCompletion?: () => void;
  // Internal field for deferred batch completion handling
  pendingTaskCompletion?: boolean;
}

export interface SessionContext {
  agentId?: string;
  callPath?: string;
  txnId: string;
  parentTxnId?: string;
  originTxnId?: string;
  toolTimeout?: number;
  maxToolCallsPerTurn?: number;
  toolResponseMaxBytes?: number;
  stopRef?: { stopping: boolean };
  // cancel status provided via getter or similar mechanism since primitive boolean is pass-by-value
  isCanceled: () => boolean;
  taskStatusToolEnabled: boolean;
  finalReportToolName: string;
}

export type ToolResponseCapper = (result: string, context?: { server?: string; tool?: string; turn?: number; subturn?: number }) => string;

export type ToolExecutor = (
  toolName: string,
  parameters: Record<string, unknown>,
  options?: { toolCallId?: string }
) => Promise<string>;

export class SessionToolExecutor {
  private static readonly REMOTE_ORCHESTRATOR = 'agent:orchestrator';
  private static readonly REMOTE_AGENT_TOOLS = 'agent:tools';

  constructor(
    private readonly toolsOrchestrator: ToolsOrchestrator | undefined,
    private readonly contextGuard: ContextGuard,
    private readonly finalReportManager: FinalReportManager,
    private readonly xmlTransport: XmlToolTransport | undefined,
    private readonly log: (entry: LogEntry) => void,
    private readonly recordAccounting: (entry: AccountingEntry) => void,
    private readonly applyToolResponseCap: ToolResponseCapper | undefined,
    private readonly sessionContext: SessionContext,
    private readonly subAgents?: { hasTool: (name: string) => boolean; addInvoked: (name: string) => void }
  ) {}

  public createExecutor(turn: number, provider: string, state: ToolExecutionState, allowedToolNames?: Set<string>): ToolExecutor {
    let subturnCounter = 0;
    const maxToolCallsPerTurn = Math.max(
      1,
      this.sessionContext.maxToolCallsPerTurn ?? 10
    );

    const executor: ToolExecutor = async (
      toolName: string,
      parameters: Record<string, unknown>,
      options?: { toolCallId?: string }
    ): Promise<string> => {
      if (this.sessionContext.stopRef?.stopping === true) {
        throw new Error('stop_requested');
      }
      if (this.sessionContext.isCanceled()) {
        throw new Error('canceled');
      }

      const normalizedToolName = sanitizeToolName(toolName);
      const isFinalReportTool =
        normalizedToolName === this.sessionContext.finalReportToolName ||
        normalizedToolName === 'final_report';
      const isProgressTool = normalizedToolName === 'agent__task_status';

      const recordXmlEntry = (
        status: 'ok' | 'failed',
        response: string,
        latency: number
      ): void => {
        if (this.xmlTransport !== undefined) {
          this.xmlTransport.recordToolResult(
            normalizedToolName,
            parameters,
            status,
            response,
            latency,
            options?.toolCallId
          );
        }
      };

      if (isFinalReportTool) {
        this.finalReportManager.incrementAttempts();
        const nestedCalls = this.parseNestedCallsFromFinalReport(
          parameters,
          turn
        );
        if (nestedCalls !== undefined) {
          let nestedResult = TOOL_NO_OUTPUT;
          // eslint-disable-next-line functional/no-loop-statements
          for (const nestedCall of nestedCalls) {
            nestedResult = await executor(
              nestedCall.name,
              nestedCall.parameters
            );
          }
          return nestedResult;
        }
      }

      // Advance subturn for each tool call within this turn
      subturnCounter += 1;
      if (subturnCounter > maxToolCallsPerTurn) {
        const msg = `Tool calls per turn exceeded: limit=${String(maxToolCallsPerTurn)}. Switch strategy: avoid further tool calls this turn; either summarize progress or call ${this.sessionContext.finalReportToolName} to conclude.`;
        const warn: LogEntry = {
          timestamp: Date.now(),
          severity: 'WRN',
          turn,
          subturn: subturnCounter,
          direction: 'response',
          type: 'tool',
          remoteIdentifier: 'agent:limits',
          fatal: false,
          message: msg,
        };
        this.log(warn);
        state.toolLimitExceeded = true;
        throw new Error('tool_calls_per_turn_limit_exceeded');
      }

      const effectiveToolName = normalizedToolName;
      addSpanEvent('tool.call.requested', {
        'ai.tool.name': effectiveToolName,
      });
      const startTime = Date.now();

      try {
        if (
          allowedToolNames !== undefined &&
          !allowedToolNames.has(effectiveToolName)
        ) {
          const blocked: LogEntry = {
            timestamp: Date.now(),
            severity: 'WRN',
            turn,
            subturn: subturnCounter,
            direction: 'response',
            type: 'tool',
            remoteIdentifier: SessionToolExecutor.REMOTE_AGENT_TOOLS,
            fatal: false,
            message: `Tool '${effectiveToolName}' is not permitted for provider '${provider}'`,
          };
          this.log(blocked);
          throw new Error('tool_not_permitted');
        }
        // Internal tools and sub-agent execution handled by orchestrator/providers
        // We just need to check if the tool is permitted/known if we were filtering per-provider
        // But here we rely on the orchestrator to have the tool or not.

        const orchestrator = this.toolsOrchestrator;
        // eslint-disable-next-line @typescript-eslint/prefer-optional-chain
        if (orchestrator !== undefined && orchestrator.hasTool(effectiveToolName)) {
          // Count attempts only for known tools
          const isBatchTool = effectiveToolName === 'agent__batch';
          state.executedTools += 1;
          if (isProgressTool || isBatchTool) {
            state.executedProgressBatchTools += 1;
          }
          else {
            state.executedNonProgressBatchTools += 1;
          }
          const isSubAgentTool = this.subAgents?.hasTool(effectiveToolName) === true;
          
          const managed = await orchestrator.executeWithManagement(
            effectiveToolName,
            parameters,
            { turn, subturn: subturnCounter },
            {
              timeoutMs: this.sessionContext.toolTimeout,
              bypassConcurrency: isBatchTool,
              disableGlobalTimeout: isBatchTool,
            }
          );

          // For batch tools, count inner non-progress tools that were invoked
          if (isBatchTool && managed.result.length > 0) {
            const innerToolCount = this.countBatchInnerTools(managed.result, state);
            state.executedNonProgressBatchTools += innerToolCount;
            // Execute any deferred task completion callbacks after batch processing
            if (state.pendingTaskCompletion === true) {
              state.onTaskCompletion?.();
              state.pendingTaskCompletion = false;
            }
          }

          if (isSubAgentTool) {
            this.subAgents.addInvoked(effectiveToolName);
          }

          const providerLabel =
            typeof managed.providerLabel === 'string' &&
            managed.providerLabel.length > 0
              ? managed.providerLabel
              : SessionToolExecutor.REMOTE_AGENT_TOOLS;

          // Track task status tool usage
          if (effectiveToolName === 'agent__task_status') {
            // Check completion status from the tool parameters (not response)
            const status = typeof parameters.status === 'string' ? parameters.status : '';
            const isCompleted = status === 'completed';
            state.lastTaskStatusCompleted = isCompleted;
            // If task completion is signaled, trigger immediate final turn
            if (isCompleted) {
              state.onTaskCompletion?.();
            }
          }

          // Track when productive tools are executed successfully
          if (!isProgressTool && !isBatchTool && !isSubAgentTool && managed.ok) {
            state.productiveToolExecutedThisTurn = true;
          }

          const uncappedToolOutput =
            managed.result.length > 0
              ? managed.result
              : TOOL_NO_OUTPUT;
          // Batch tool is a container - inner tools already get capped individually, don't cap the container
          const toolOutput = (this.applyToolResponseCap !== undefined && !isBatchTool)
            ? this.applyToolResponseCap(uncappedToolOutput, { server: providerLabel, tool: effectiveToolName, turn, subturn: subturnCounter })
            : uncappedToolOutput;
          const callId =
            typeof options?.toolCallId === 'string' &&
            options.toolCallId.length > 0
              ? options.toolCallId
              : undefined;

          if (managed.dropped === true) {
            const failureReason = managed.reason ?? 'context_budget_exceeded';
            const failureTokens = this.estimateTokensForCounters([
              { role: 'tool', content: toolOutput },
            ]);
            this.contextGuard.addNewTokens(failureTokens);

            if (callId !== undefined) {
              state.toolFailureMessages.set(callId, toolOutput);
              if (
                !state.turnHasSuccessfulToolOutput &&
                (failureReason === 'context_budget_exceeded' ||
                  failureReason === 'token_budget_exceeded')
              ) {
                state.trimmedToolCallIds.add(callId);
              }
            } else {
              state.toolFailureFallbacks.push(toolOutput);
            }

            const failureEntry: AccountingEntry = {
              type: 'tool',
              timestamp: startTime,
              status: 'failed',
              latency: managed.latency,
              mcpServer: providerLabel,
              command: effectiveToolName,
              charactersIn: managed.charactersIn,
              charactersOut: managed.charactersOut,
              error: failureReason,
              agentId: this.sessionContext.agentId,
              callPath: this.sessionContext.callPath,
              txnId: this.sessionContext.txnId,
              parentTxnId: this.sessionContext.parentTxnId,
              originTxnId: this.sessionContext.originTxnId,
              details: {
                reason: failureReason,
                original_tokens: managed.tokens ?? 0,
                replacement_tokens: failureTokens,
              },
            };
            this.recordAccounting(failureEntry);

            addSpanEvent('tool.call.failure', {
              'ai.tool.name': effectiveToolName,
              'ai.tool.latency_ms': managed.latency,
              'ai.tool.failure.reason': failureReason,
            });
            recordXmlEntry('failed', toolOutput, managed.latency);
            return toolOutput;
          }

          const managedTokens =
            typeof managed.tokens === 'number' ? managed.tokens : undefined;
          const isInternalProvider = providerLabel === 'agent';
          const toolTokens =
            managedTokens ??
            this.estimateTokensForCounters([
              { role: 'tool', content: toolOutput },
            ]);

          if (!isInternalProvider) {
            const guardEvaluation = this.contextGuard.evaluate(toolTokens);

            if (process.env.CONTEXT_DEBUG === 'true') {
              const approxTokens = Math.ceil(
                estimateMessagesBytes([{ role: 'tool', content: toolOutput }]) / 4
              );
              let limitTokens: number | undefined;
              let contextWindow: number | undefined;
              if (guardEvaluation.blocked.length > 0) {
                const firstBlocked = guardEvaluation.blocked[0];
                limitTokens = firstBlocked.limit;
                contextWindow = firstBlocked.contextWindow;
              }
              console.log('context-guard/tool-eval', {
                toolTokens,
                approxTokens,
                contentLength: toolOutput.length,
                currentCtx: this.contextGuard.getCurrentTokens(),
                pendingCtx: this.contextGuard.getPendingTokens(),
                newCtx: this.contextGuard.getNewTokens(),
                projectedTokens: guardEvaluation.projectedTokens,
                limitTokens,
                contextWindow,
              });
            }

            if (guardEvaluation.blocked.length > 0) {
              const blockedEntries =
                guardEvaluation.blocked.length > 0
                  ? guardEvaluation.blocked
                  : [
                      {
                        provider: 'unknown',
                        model: 'unknown',
                        contextWindow: ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS,
                        bufferTokens: ContextGuard.DEFAULT_CONTEXT_BUFFER_TOKENS,
                        maxOutputTokens: this.contextGuard.computeMaxOutputTokens(
                          ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS
                        ),
                        limit: 0,
                        projected: guardEvaluation.projectedTokens,
                      },
                    ];
              const first = blockedEntries[0];
              const remainingTokens =
                first.limit > first.projected
                  ? first.limit - first.projected
                  : 0;

              const warnEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn,
                subturn: subturnCounter,
                direction: 'response',
                type: 'tool',
                remoteIdentifier: providerLabel,
                fatal: false,
                message: `Tool '${effectiveToolName}' output dropped: context window budget exceeded (${String(toolTokens)} tokens, limit ${String(first.limit)}).`,
                details: {
                  tool: effectiveToolName,
                  provider: providerLabel,
                  tokens_estimated: toolTokens,
                  projected_tokens: guardEvaluation.projectedTokens,
                  limit_tokens: first.limit,
                  remaining_tokens: remainingTokens,
                  reason: 'token_budget_exceeded',
                },
              };
              this.log(warnEntry);

              const renderedFailure = TOOL_NO_OUTPUT;
              const failureTokens = this.estimateTokensForCounters([
                { role: 'tool', content: renderedFailure },
              ]);
              this.contextGuard.addNewTokens(failureTokens);

              if (callId !== undefined) {
                state.toolFailureMessages.set(callId, renderedFailure);
                if (!state.turnHasSuccessfulToolOutput) {
                  state.trimmedToolCallIds.add(callId);
                }
              } else {
                state.toolFailureFallbacks.push(renderedFailure);
              }

              const failureEntry: AccountingEntry = {
                type: 'tool',
                timestamp: startTime,
                status: 'failed',
                latency: managed.latency,
                mcpServer: providerLabel,
                command: effectiveToolName,
                charactersIn: managed.charactersIn,
                charactersOut: managed.charactersOut,
                error: 'token_budget_exceeded',
                agentId: this.sessionContext.agentId,
                callPath: this.sessionContext.callPath,
                txnId: this.sessionContext.txnId,
                parentTxnId: this.sessionContext.parentTxnId,
                originTxnId: this.sessionContext.originTxnId,
                details: {
                  projected_tokens: first.projected,
                  limit_tokens: first.limit,
                  remaining_tokens: remainingTokens,
                  original_tokens: toolTokens,
                  replacement_tokens: failureTokens,
                },
              };
              this.recordAccounting(failureEntry);

              addSpanEvent('tool.call.failure', {
                'ai.tool.name': effectiveToolName,
                'ai.tool.latency_ms': managed.latency,
                'ai.tool.failure.reason': 'context_budget_exceeded',
              });
              recordXmlEntry('failed', renderedFailure, managed.latency);
              this.contextGuard.enforceFinalTurn(
                blockedEntries,
                'tool_preflight'
              );
              return renderedFailure;
            }
          } else if (process.env.CONTEXT_DEBUG === 'true') {
            const approxTokens = Math.ceil(
              estimateMessagesBytes([{ role: 'tool', content: toolOutput }]) / 4
            );
            console.log('context-guard/tool-eval', {
              toolTokens,
              approxTokens,
              contentLength: toolOutput.length,
              currentCtx: this.contextGuard.getCurrentTokens(),
              pendingCtx: this.contextGuard.getPendingTokens(),
              newCtx: this.contextGuard.getNewTokens(),
              projectedTokens:
                this.contextGuard.getCurrentTokens() +
                this.contextGuard.getPendingTokens() +
                this.contextGuard.getNewTokens(),
              limitTokens: undefined,
              contextWindow: undefined,
            });
          }

          this.contextGuard.addNewTokens(toolTokens);
          const successEntry: AccountingEntry = {
            type: 'tool',
            timestamp: startTime,
            status: 'ok',
            latency: managed.latency,
            mcpServer: providerLabel,
            command: effectiveToolName,
            charactersIn: managed.charactersIn,
            charactersOut: managed.charactersOut,
            agentId: this.sessionContext.agentId,
            callPath: this.sessionContext.callPath,
            txnId: this.sessionContext.txnId,
            parentTxnId: this.sessionContext.parentTxnId,
            originTxnId: this.sessionContext.originTxnId,
            details: { tokens: toolTokens },
          };
          this.recordAccounting(successEntry);
          state.turnHasSuccessfulToolOutput = true;

          if (managedTokens === undefined) {
            const toolLog: LogEntry = {
              timestamp: Date.now(),
              severity: 'VRB',
              turn,
              subturn: subturnCounter,
              direction: 'response',
              type: 'tool',
              remoteIdentifier: providerLabel,
              fatal: false,
              message: `Tool '${effectiveToolName}' completed (${String(managed.charactersOut)} bytes, ${String(toolTokens)} tokens).`,
              details: {
                latency_ms: managed.latency,
                characters_in: managed.charactersIn,
                characters_out: managed.charactersOut,
                tokens: toolTokens,
              },
            };
            this.log(toolLog);
          }

          addSpanEvent('tool.call.success', {
            'ai.tool.name': effectiveToolName,
            'ai.tool.latency_ms': managed.latency,
            'ai.tool.tokens': toolTokens,
          });
          recordXmlEntry('ok', toolOutput, managed.latency);
          return toolOutput;
        }

        // Unknown tool after all paths
        {
          const msg = `Unknown tool requested: ${effectiveToolName}`;
          const warn: LogEntry = {
            timestamp: Date.now(),
            severity: 'WRN',
            turn,
            subturn: 0,
            direction: 'response',
            type: 'llm',
            remoteIdentifier: 'assistant:tool',
            fatal: false,
            message: msg,
          };
          this.log(warn);
          state.unknownToolEncountered = true;
          addSpanEvent('tool.call.unknown', {
            'ai.tool.name': effectiveToolName,
          });
          throw new Error(`No server found for tool: ${effectiveToolName}`);
        }
      } catch (error) {
        const latency = Date.now() - startTime;
        const errorMsg = error instanceof Error ? error.message : String(error);
        addSpanEvent('tool.call.error', {
          'ai.tool.name': effectiveToolName,
          'ai.tool.error': errorMsg,
        });

        if (isFinalReportTool) {
          state.finalReportToolFailed = true;
          const serialized = (() => {
            try {
              return JSON.stringify(parameters);
            } catch {
              return '[unserializable-final-report-payload]';
            }
          })();
          const errLog: LogEntry = {
            timestamp: Date.now(),
            severity: 'ERR',
            turn,
            subturn: subturnCounter,
            direction: 'response',
            type: 'tool',
            remoteIdentifier: SessionToolExecutor.REMOTE_ORCHESTRATOR,
            fatal: false,
            message: `${this.sessionContext.finalReportToolName} failed: ${errorMsg}`,
            details: { payload: serialized },
          };
          this.log(errLog);
        }

        // Add failed tool accounting
        const accountingEntry: AccountingEntry = {
          type: 'tool',
          timestamp: startTime,
          status: 'failed',
          latency,
          mcpServer: 'unknown',
          command: toolName,
          charactersIn: JSON.stringify(parameters).length,
          charactersOut: 0,
          error: errorMsg,
          agentId: this.sessionContext.agentId,
          callPath: this.sessionContext.callPath,
          txnId: this.sessionContext.txnId,
          parentTxnId: this.sessionContext.parentTxnId,
          originTxnId: this.sessionContext.originTxnId,
        };
        this.recordAccounting(accountingEntry);

        // Check if this is an incomplete final_report error
        if (toolName === this.sessionContext.finalReportToolName) {
          state.incompleteFinalReportDetected = true;
        }

        // Return error message instead of throwing
        const limitMessage = `execution not allowed because the per-turn tool limit (${String(maxToolCallsPerTurn)}) was reached; retry this tool on the next turn if available.`;
        const failureDetail =
          errorMsg === 'tool_calls_per_turn_limit_exceeded'
            ? limitMessage
            : errorMsg;
        const renderedFailure = `(tool failed: ${failureDetail})`;

        if (
          typeof options?.toolCallId === 'string' &&
          options.toolCallId.length > 0
        ) {
          state.toolFailureMessages.set(options.toolCallId, renderedFailure);
          if (failureDetail.includes('per-turn tool limit')) {
            state.toolLimitExceeded = true;
          }
          if (
            !state.turnHasSuccessfulToolOutput &&
            failureDetail.toLowerCase().includes('context window budget exceeded')
          ) {
            state.trimmedToolCallIds.add(options.toolCallId);
          }
        } else {
          state.toolFailureFallbacks.push(renderedFailure);
        }
        recordXmlEntry('failed', renderedFailure, latency);
        return renderedFailure;
      }
    };

    return executor;
  }

  private estimateTokensForCounters(messages: { role: string; content: string }[]): number {
    return this.contextGuard.estimateTokens(messages as ConversationMessage[]);
  }

  public parseNestedCallsFromFinalReport(
    parameters: Record<string, unknown>,
    turn: number
  ): { id: string; name: string; parameters: Record<string, unknown> }[] | undefined {
    const rawCalls = (parameters as { calls?: unknown }).calls;
    if (!Array.isArray(rawCalls) || rawCalls.length === 0) {
      return undefined;
    }
    const parsed: { id: string; name: string; parameters: Record<string, unknown> }[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const entryUnknown of rawCalls) {
      if (
        entryUnknown === null ||
        typeof entryUnknown !== 'object' ||
        Array.isArray(entryUnknown)
      ) {
        return undefined;
      }
      const entry = entryUnknown as {
        id?: unknown;
        tool?: unknown;
        parameters?: unknown;
      };
      const toolRaw = entry.tool;
      if (typeof toolRaw !== 'string' || toolRaw.trim().length === 0) {
        return undefined;
      }
      const sanitizedTool = sanitizeToolName(toolRaw);
      const clampResult = clampToolName(sanitizedTool) as {
        name: string;
        truncated: boolean;
      };
      const clampedToolName = clampResult.name;
      const idRaw = entry.id;
      const toolCallId = (() => {
        if (typeof idRaw === 'string' && idRaw.length > 0) {
          return idRaw;
        }
        if (typeof idRaw === 'number' && Number.isFinite(idRaw)) {
          return Math.trunc(idRaw).toString();
        }
        return 'call-' + (parsed.length + 1).toString();
      })();
      const parametersCandidate = entry.parameters;
      const parsedParameters = parseJsonRecord(parametersCandidate);
      if (parsedParameters === undefined) {
        const preview = this.previewRawParameter(parametersCandidate);
        const logEntry: LogEntry = {
          timestamp: Date.now(),
          severity: 'ERR',
          turn,
          subturn: 0,
          direction: 'response',
          type: 'tool',
          remoteIdentifier: 'agent:final_report',
          fatal: false,
          message: `agent__final_report nested call '${toolCallId}' parameters invalid (raw preview: ${preview})`,
        };
        this.log(logEntry);
      }
      const toolParameters = parsedParameters ?? {};
      parsed.push({
        id: toolCallId,
        name: clampedToolName,
        parameters: toolParameters,
      });
    }
    return parsed.length > 0 ? parsed : undefined;
  }

  public previewRawParameter(raw: unknown): string {
    if (raw === undefined) {
      return 'undefined';
    }
    if (typeof raw === 'string') {
      return truncateToBytes(raw, 512) ?? raw;
    }
    try {
      const serialized = JSON.stringify(raw);
      return truncateToBytes(serialized, 512) ?? serialized;
    } catch {
      const fallback = Object.prototype.toString.call(raw);
      return truncateToBytes(fallback, 512) ?? fallback;
    }
  }

  /**
   * Count non-progress, non-batch tools that were invoked within a batch result.
   * Per design: any MCP/REST/Subagent tool (excluding progress and batch) that is
   * actually invoked counts toward turn success, even if execution fails.
   */
  private countBatchInnerTools(batchResult: string, state: ToolExecutionState): number {
    try {
      const parsed = JSON.parse(batchResult) as unknown;
      if (parsed === null || typeof parsed !== 'object') return 0;
      const results = Array.isArray(parsed) ? parsed : (parsed as Record<string, unknown>).results;
      if (!Array.isArray(results)) return 0;

      let count = 0;
      // eslint-disable-next-line functional/no-loop-statements
      for (const entry of results) {
        if (entry === null || typeof entry !== 'object') continue;
        const tool = (entry as Record<string, unknown>).tool;
        if (typeof tool !== 'string') continue;
        
        // Check if this is a task_status tool inside batch
        const normalized = sanitizeToolName(tool);
        if (normalized === 'agent__task_status') {
          // Check completion status from call parameters
          const callParams = (entry as Record<string, unknown>).parameters;
          if (typeof callParams === 'object' && callParams !== null) {
            const status = typeof (callParams as Record<string, unknown>).status === 'string' ? (callParams as Record<string, unknown>).status : '';
            const isCompleted = status === 'completed';
            if (isCompleted) {
              state.lastTaskStatusCompleted = true;
              // Don't trigger completion callback immediately - defer until batch completes
              state.pendingTaskCompletion = true;
            }
          }
          // Exclude task_status from count
          continue;
        }
        
        if (normalized === 'agent__batch') continue;
        count += 1;
      }
      return count;
    } catch {
      return 0;
    }
  }
}
