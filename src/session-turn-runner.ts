/**
 * TurnRunner - Extracted turn orchestration logic from AIAgentSession
 *
 * Handles:
 * - Turn iteration and retry logic
 * - Provider/model cycling
 * - Context guard evaluation
 * - LLM call orchestration
 * - Message sanitization
 * - Final report extraction/adoption
 */
import crypto from 'node:crypto';

import type { ContextGuard, ContextGuardBlockedEntry, ContextGuardEvaluation } from './context-guard.js';
import type { FinalReportManager, FinalReportSource, PendingFinalReportPayload } from './final-report-manager.js';
import type { OutputFormatId } from './formats.js';
import type { LLMClient } from './llm-client.js';
import type { SessionProgressReporter } from './session-progress-reporter.js';
import type { SessionToolExecutor, ToolExecutionState } from './session-tool-executor.js';
import type { SessionNode, SessionTreeBuilder } from './session-tree.js';
import type { SubAgentRegistry } from './subagent-registry.js';
import type { LeakedToolFallbackResult } from './tool-call-fallback.js';
import type { ToolsOrchestrator } from './tools/tools.js';
import type { AIAgentResult, AIAgentSessionConfig, AccountingEntry, Configuration, ConversationMessage, LogDetailValue, LogEntry, MCPTool, ProviderReasoningMapping, ProviderReasoningValue, ReasoningLevel, ToolCall, TurnResult, TurnRetryDirective, TurnStatus } from './types.js';
import type { XmlToolTransport } from './xml-transport.js';

import { FINAL_REPORT_FORMAT_VALUES, FINAL_REPORT_SOURCE_TEXT_FALLBACK, FINAL_REPORT_SOURCE_TOOL_MESSAGE } from './final-report-manager.js';
import {
  CONTENT_GUIDANCE_JSON,
  CONTENT_GUIDANCE_SLACK,
  CONTENT_GUIDANCE_TEXT,
  CONTEXT_FINAL_MESSAGE,
  FINAL_REPORT_CONTENT_MISSING,
  FINAL_REPORT_JSON_REQUIRED,
  FINAL_REPORT_SLACK_MESSAGES_MISSING,
  TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT,
  FINAL_TURN_NOTICE,
  MAX_TURNS_FINAL_MESSAGE,
  TOOL_CALL_MALFORMED,
  TOOL_NO_OUTPUT,
  emptyResponseRetryNotice,
  finalReportFormatMismatch,
  finalReportReminder,
  toolReminderMessage,
  turnFailedPrefix,
} from './llm-messages.js';
import { addSpanAttributes, addSpanEvent, recordContextGuardMetrics, recordFinalReportMetrics, recordLlmMetrics, recordRetryCollapseMetrics, runWithSpan } from './telemetry/index.js';
import { processLeakedToolCalls } from './tool-call-fallback.js';
import { estimateMessagesBytes, formatToolRequestCompact, parseJsonValueDetailed, sanitizeToolName, warn } from './utils.js';
import { XmlFinalReportFilter } from './xml-transport.js';

/**
 * Immutable configuration and collaborators passed to TurnRunner
 */
export interface TurnRunnerContext {
  readonly sessionConfig: AIAgentSessionConfig;
  readonly config: Configuration;
  readonly agentId?: string;
  readonly callPath?: string;
  readonly agentPath: string;
  readonly txnId: string;
  readonly parentTxnId?: string;
  readonly originTxnId?: string;
  readonly headendId?: string;
  readonly telemetryLabels: Record<string, string>;
  readonly llmClient: LLMClient;
  readonly toolsOrchestrator?: ToolsOrchestrator;
  readonly contextGuard: ContextGuard;
  readonly finalReportManager: FinalReportManager;
  readonly opTree: SessionTreeBuilder;
  readonly progressReporter: SessionProgressReporter;
  readonly sessionExecutor: SessionToolExecutor;
  readonly xmlTransport?: XmlToolTransport;
  readonly subAgents?: SubAgentRegistry;
  readonly resolvedFormat?: OutputFormatId;
  readonly resolvedFormatPromptValue?: string;
  readonly resolvedFormatParameterDescription?: string;
  readonly resolvedUserPrompt?: string;
  readonly resolvedSystemPrompt?: string;
  readonly expectedJsonSchema?: Record<string, unknown>;
  readonly progressToolEnabled: boolean;
  readonly toolTransport: 'native' | 'xml' | 'xml-final';
  readonly abortSignal?: AbortSignal;
  readonly stopRef?: { stopping: boolean };
  isCanceled: () => boolean;
}

/**
 * Callbacks for side effects that need to update AIAgentSession state
 */
export interface TurnRunnerCallbacks {
  log: (entry: LogEntry, opts?: { opId?: string }) => void;
  onAccounting: (entry: AccountingEntry) => void;
  onOpTree: (tree: SessionNode) => void;
  onTurnStarted?: (turn: number) => void;
  onOutput?: (chunk: string) => void;
  onThinking?: (chunk: string) => void;
  setCurrentTurn: (turn: number) => void;
  setMasterLlmStartLogged: () => void;
  isMasterLlmStartLogged: () => boolean;
  setSystemTurnBegan: () => void;
  isSystemTurnBegan: () => boolean;
  setCurrentLlmOpId: (opId: string | undefined) => void;
  getCurrentLlmOpId: () => string | undefined;
  applyToolResponseCap: (result: string, limitBytes: number | undefined, logs: LogEntry[], context?: { server?: string; tool?: string; turn?: number; subturn?: number }) => string;
}

/**
 * Internal mutable state managed by TurnRunner
 */
interface TurnRunnerState {
  currentTurn: number;
  maxTurns: number;
  conversation: ConversationMessage[];
  logs: LogEntry[];
  accounting: AccountingEntry[];
  turnFailureReasons: string[];
  pendingRetryMessages: string[];
  toolFailureMessages: Map<string, string>;
  toolFailureFallbacks: string[];
  trimmedToolCallIds: Set<string>;
  plannedSubturns: Map<number, number>;
  finalTurnEntryLogged: boolean;
  childConversations: {
    agentId?: string;
    toolName: string;
    promptPath: string;
    conversation: ConversationMessage[];
    trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string };
  }[];
  centralSizeCapHits: number;
  llmAttempts: number;
  llmSyntheticFailures: number;
  pendingToolSelection?: { availableTools: MCPTool[]; allowedToolNames: Set<string>; toolsForTurn: MCPTool[] };
  currentCtxTokens?: number;
  pendingCtxTokens?: number;
  newCtxTokens?: number;
  schemaCtxTokens?: number;
  forcedFinalTurnReason?: 'context' | 'max_turns';
  lastTurnError?: string;
  lastTurnErrorType?: string;
}

// Static constants (non-LLM-facing)
const REMOTE_FINAL_TURN = 'agent:final-turn';
const REMOTE_CONTEXT = 'agent:context';
const REMOTE_ORCHESTRATOR = 'agent:orchestrator';
const REMOTE_SANITIZER = 'agent:sanitizer';
const REMOTE_FAILURE_REPORT = 'agent:failure-report';
const FINAL_REPORT_TOOL = 'agent__final_report';
const SLACK_BLOCK_KIT_FORMAT = 'slack-block-kit';
const FINAL_REPORT_TOOL_ALIASES = new Set(['agent__final_report', 'agent-final-report']);
const RETRY_ACTION_SKIP_PROVIDER = 'skip-provider';
export { FINAL_REPORT_TOOL };
/**
 * TurnRunner encapsulates all turn iteration logic
 */
export class TurnRunner {
    private readonly ctx: TurnRunnerContext;
    private readonly callbacks: TurnRunnerCallbacks;
    private state: TurnRunnerState;
    private finalReportStreamed = false;

    constructor(ctx: TurnRunnerContext, callbacks: TurnRunnerCallbacks) {
        this.ctx = ctx;
        this.callbacks = callbacks;
        this.state = {
            currentTurn: 0,
            maxTurns: ctx.sessionConfig.maxTurns ?? 10,
            conversation: [],
            logs: [],
            accounting: [],
            turnFailureReasons: [],
            pendingRetryMessages: [],
            toolFailureMessages: new Map<string, string>(),
            toolFailureFallbacks: [],
            trimmedToolCallIds: new Set<string>(),
            plannedSubturns: new Map<number, number>(),
            finalTurnEntryLogged: false,
            childConversations: [],
            centralSizeCapHits: 0,
            llmAttempts: 0,
            llmSyntheticFailures: 0,
        };
    }
    /**
     * Main entry point - executes the agent loop
     * @param childConversations - Optional array reference from AIAgentSession; if provided, TurnRunner will use this
     *                             reference so that child conversation data populated via AgentProvider callbacks is preserved.
     * @param plannedSubturns - Optional map reference from AIAgentSession; if provided, TurnRunner will write planned subturn
     *                          counts into this map so AIAgentSession.addLog can enrich logs with max_subturns metadata.
     */
    async execute(
        initialConversation: ConversationMessage[],
        initialLogs: LogEntry[],
        initialAccounting: AccountingEntry[],
        startTurn: number,
        childConversations?: TurnRunnerState['childConversations'],
        plannedSubturns?: Map<number, number>
    ): Promise<AIAgentResult> {
        this.state.conversation = [...initialConversation];
        this.state.logs = [...initialLogs];
        this.state.accounting = [...initialAccounting];
        this.state.currentTurn = startTurn;
        // Use the passed childConversations array reference if provided, otherwise use the empty one from state
        if (childConversations !== undefined) {
            this.state.childConversations = childConversations;
        }
        // Use the passed plannedSubturns map reference if provided, so AIAgentSession.addLog can enrich logs
        if (plannedSubturns !== undefined) {
            this.state.plannedSubturns = plannedSubturns;
        }
        let finalReportToolFailed = false;
        const conversation = this.state.conversation;
        const logs = this.state.logs;
        const accounting = this.state.accounting;
        const seedConversationTokens = this.estimateTokensForCounters(conversation);
        this.currentCtxTokens = 0;
        this.pendingCtxTokens = seedConversationTokens;
        this.newCtxTokens = 0;
        if (process.env.CONTEXT_DEBUG === 'true') {
            console.log('context-guard/loop-init', {
                currentCtxTokens: this.currentCtxTokens,
                pendingCtxTokens: this.pendingCtxTokens,
                newCtxTokens: this.newCtxTokens,
                schemaCtxTokens: this.schemaCtxTokens,
                seedConversationTokens,
                turn: this.state.currentTurn,
            });
        }
        // Track the last turn where we showed thinking header
        let lastShownThinkingHeaderTurn = -1;
        const finalTurnRetryMessage = 'Final turn did not produce final_report; retrying with next provider/model in this turn.';
        let maxTurns = this.state.maxTurns;
        const maxRetries = this.ctx.sessionConfig.maxRetries ?? 3;
        const pairs = this.ctx.sessionConfig.targets;
        // Turn loop
        // eslint-disable-next-line functional/no-loop-statements
        for (let currentTurn = 1; currentTurn <= maxTurns; currentTurn++) {
            if (this.ctx.isCanceled())
                return this.finalizeCanceledSession(conversation, logs, accounting);
            if (this.ctx.stopRef?.stopping === true) {
                return this.finalizeGracefulStopSession(conversation, logs, accounting);
            }
            if (this.ctx.xmlTransport !== undefined) {
                this.ctx.xmlTransport.beginTurn();
            }
            this.state.turnFailureReasons = [];
            this.state.currentTurn = currentTurn;
            this.callbacks.setCurrentTurn(currentTurn);
            this.logTurnStart(currentTurn);
            try {
                this.callbacks.onTurnStarted?.(currentTurn);
            }
            catch (e) {
                warn(`onTurnStarted callback failed: ${e instanceof Error ? e.message : String(e)}`);
            }
            try {
                const turnAttrs = {
                    prompts: {
                        system: (() => { try {
                            return this.ctx.resolvedSystemPrompt ?? '';
                        }
                        catch {
                            return '';
                        } })(),
                        user: (() => { try {
                            return this.ctx.resolvedUserPrompt ?? '';
                        }
                        catch {
                            return '';
                        } })(),
                    }
                };
                this.ctx.opTree.beginTurn(currentTurn, turnAttrs);
                this.callbacks.onOpTree(this.ctx.opTree.getSession());
            }
            catch (e) {
                warn(`beginTurn/onOpTree failed: ${e instanceof Error ? e.message : String(e)}`);
            }
            this.ctx.llmClient.setTurn(currentTurn, 0);
            let lastError;
            let lastErrorType;
            let turnSuccessful = false;
            let finalTurnRetryWarnLogged = false;
            let turnHadFinalReportAttempt = false;
            let collapseLoggedThisTurn = false;
            const collapseRemainingTurns = (reason: 'incomplete_final_report' | 'final_report_attempt'): void => {
                if (collapseLoggedThisTurn)
                    return;
                if (currentTurn >= maxTurns)
                    return;
                const previousMaxTurns = maxTurns;
                maxTurns = currentTurn + 1;
                const adjustLog: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: REMOTE_ORCHESTRATOR,
                    fatal: false,
                    message: reason === 'incomplete_final_report'
                        ? `Incomplete final report detected at turn ${String(currentTurn)}; Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`
                        : `Final report retry detected at turn ${String(currentTurn)}; Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`
                };
                this.log(adjustLog);
                recordRetryCollapseMetrics({
                    agentId: this.ctx.agentId,
                    callPath: this.ctx.callPath,
                    headendId: this.ctx.headendId,
                    reason,
                    turn: currentTurn,
                    previousMaxTurns,
                    newMaxTurns: maxTurns,
                    customLabels: this.ctx.telemetryLabels,
                });
                collapseLoggedThisTurn = true;
                this.logEnteringFinalTurn('max_turns', maxTurns);
            };
            // Global attempts across all provider/model pairs for this turn
            let attempts = 0;
            let pairCursor = 0;
            let rateLimitedInCycle = 0;
            let maxRateLimitWaitMs = 0;
            // turnSuccessful starts false; checked on subsequent iterations after potential success at line ~1184
            // eslint-disable-next-line functional/no-loop-statements
            while (attempts < maxRetries && !turnSuccessful) {
                if (Boolean(this.ctx.stopRef?.stopping)) {
                    return this.finalizeGracefulStopSession(conversation, logs, accounting);
                }
                // Emit startup verbose line when master LLM runs (once per session)
                if (!this.callbacks.isMasterLlmStartLogged() && this.ctx.parentTxnId === undefined) {
                    try {
                        const ctx = (typeof this.ctx.callPath === 'string' && this.ctx.callPath.length > 0) ? this.ctx.callPath : (this.ctx.agentId ?? 'agent');
                        const promptText = typeof this.ctx.resolvedUserPrompt === 'string' ? this.ctx.resolvedUserPrompt : '';
                        const entry = {
                            timestamp: Date.now(),
                            severity: 'VRB' as const,
                            turn: currentTurn,
                            subturn: 0,
                            direction: 'request' as const,
                            type: 'llm' as const,
                            remoteIdentifier: 'agent:start',
                            fatal: false,
                            bold: true,
                            message: `${ctx}: ${promptText}`,
                        };
                        this.log(entry);
                    }
                    catch (e) {
                        warn(`verbose log failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                    this.callbacks.setMasterLlmStartLogged();
                }
                const pair = pairs[pairCursor % pairs.length];
                pairCursor += 1;
                const { provider, model } = pair;
                // Note: cycleIndex/cycleComplete tracking removed during extraction - may need to be reinstated
                let forcedFinalTurn = this.forcedFinalTurnReason !== undefined;
                let isFinalTurn = forcedFinalTurn || currentTurn === maxTurns;
                const syncFinalTurnFlags = () => {
                    if (!forcedFinalTurn && this.forcedFinalTurnReason !== undefined) {
                        forcedFinalTurn = true;
                        isFinalTurn = true;
                    }
                };
                const toolSelection = this.selectToolsForTurn(provider, isFinalTurn);
                this.state.pendingToolSelection = toolSelection;
                this.schemaCtxTokens = this.estimateToolSchemaTokens(toolSelection.toolsForTurn);
                const providerContextStatus = this.evaluateContextForProvider(provider, model);
                if (providerContextStatus === 'final') {
                    const evaluation = this.evaluateContextGuard(this.schemaCtxTokens);
                    this.enforceContextFinalTurn(evaluation.blocked, 'turn_preflight');
                    syncFinalTurnFlags();
                    const finalToolSelection = this.selectToolsForTurn(provider, true);
                    this.state.pendingToolSelection = finalToolSelection;
                    this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
                    const postEnforce = this.evaluateContextGuard(this.schemaCtxTokens);
                    if (postEnforce.blocked.length > 0) {
                        const [firstBlocked] = postEnforce.blocked;
                        const warnEntry = {
                            timestamp: Date.now(),
                            severity: 'WRN' as const,
                            turn: currentTurn,
                            subturn: 0,
                            direction: 'response' as const,
                            type: 'llm' as const,
                            remoteIdentifier: REMOTE_CONTEXT,
                            fatal: false,
                            message: 'Context guard post-shrink still over projected limit; continuing with forced final turn.',
                            details: {
                                projected_tokens: postEnforce.projectedTokens,
                                limit_tokens: firstBlocked.limit,
                                provider: firstBlocked.provider,
                                model: firstBlocked.model,
                            },
                        };
                        this.log(warnEntry);
                    }
                }
                if (providerContextStatus === 'skip') {
                    const evaluation = this.evaluateContextGuard(this.schemaCtxTokens);
                    const blocked = evaluation.blocked.find((entry) => entry.provider === provider && entry.model === model);
                    if (blocked !== undefined) {
                        const limitTokens = blocked.limit;
                        const remainingTokens = Number.isFinite(blocked.limit)
                            ? Math.max(0, blocked.limit - blocked.projected)
                            : undefined;
                        this.reportContextGuardEvent({
                            provider,
                            model,
                            trigger: 'turn_preflight',
                            outcome: 'skipped_provider',
                            limitTokens: blocked.limit,
                            projectedTokens: blocked.projected,
                            remainingTokens,
                        });
                        const warnEntry = {
                            timestamp: Date.now(),
                            severity: 'WRN' as const,
                            turn: currentTurn,
                            subturn: 0,
                            direction: 'response' as const,
                            type: 'llm' as const,
                            remoteIdentifier: REMOTE_CONTEXT,
                            fatal: false,
                            message: `Projected context size ${String(blocked.projected)} tokens exceeds limit ${String(blocked.limit)} for ${provider}:${model}; continuing turn but guard will enforce finalization.`,
                            details: {
                                projected_tokens: blocked.projected,
                                limit_tokens: limitTokens,
                                ...(remainingTokens !== undefined ? { remaining_tokens: remainingTokens } : {}),
                            },
                        };
                        this.log(warnEntry);
                    }
                }
                try {
                    // Build per-attempt conversation with optional guidance injection
                    let attemptConversation = [...conversation];
                    if (this.state.turnFailureReasons.length > 0) {
                        const feedback = turnFailedPrefix(this.state.turnFailureReasons);
                        attemptConversation.push({ role: 'user', content: feedback });
                        this.state.turnFailureReasons = [];
                    }
                    if (this.ctx.xmlTransport !== undefined) {
                        const allTools = this.ctx.toolsOrchestrator?.listTools() ?? [];
                        const xmlResult = this.ctx.xmlTransport.buildMessages({
                            turn: currentTurn,
                            maxTurns,
                            tools: allTools,
                            maxToolCallsPerTurn: Math.max(1, this.ctx.sessionConfig.maxToolCallsPerTurn ?? 10),
                            progressToolEnabled: this.ctx.progressToolEnabled,
                            finalReportToolName: FINAL_REPORT_TOOL,
                            resolvedFormat: this.ctx.resolvedFormat,
                            expectedJsonSchema: this.ctx.expectedJsonSchema,
                        });
                        if (xmlResult.pastMessage !== undefined)
                            attemptConversation.push(xmlResult.pastMessage);
                        attemptConversation.push(xmlResult.nextMessage);
                    }
                    // On the last allowed attempt within this turn, nudge the model to use tools
                    if ((attempts === maxRetries - 1) && currentTurn < (maxTurns - 1)) {
                        const excludeProgress = this.ctx.progressToolEnabled ? ' (excluding `agent__progress_report`)' : '';
                        attemptConversation.push({
                            role: 'user',
                            content: toolReminderMessage(excludeProgress, FINAL_REPORT_TOOL)
                        });
                    }
                    // Force the final-report instruction onto the conversation once we enter the last turn
                    if (isFinalTurn) {
                        const finalInstruction = this.forcedFinalTurnReason === 'context'
                            ? CONTEXT_FINAL_MESSAGE
                            : MAX_TURNS_FINAL_MESSAGE;
                        this.pushSystemRetryMessage(conversation, finalInstruction);
                        attemptConversation.push({
                            role: 'user',
                            content: finalInstruction,
                            metadata: { retryMessage: 'final-turn-instruction' },
                        });
                    }
                    if (isFinalTurn) {
                        this.logEnteringFinalTurn(forcedFinalTurn ? 'context' : 'max_turns', currentTurn);
                    }
                    this.state.llmAttempts++;
                    attempts += 1;
                    const cycleIndex = pairs.length > 0 ? (attempts - 1) % pairs.length : 0;
                    const cycleComplete = pairs.length > 0 ? (cycleIndex === pairs.length - 1) : false;
                    // Begin hierarchical LLM operation
                    const llmOpId = (() => { try {
                        return this.ctx.opTree.beginOp(currentTurn, 'llm', { provider, model, isFinalTurn });
                    }
                    catch {
                        return undefined;
                    } })();
                    this.callbacks.setCurrentLlmOpId(llmOpId);
                    try {
                        const msgBytes = (() => { try {
                            return new TextEncoder().encode(JSON.stringify(attemptConversation)).length;
                        }
                        catch {
                            return undefined;
                        } })();
                        if (llmOpId !== undefined)
                            this.ctx.opTree.setRequest(llmOpId, { kind: 'llm', payload: { messages: attemptConversation.length, bytes: msgBytes, isFinalTurn }, size: msgBytes });
                    }
                    catch (e) {
                        warn(`final turn warning log failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                    const sessionReasoningLevel = this.ctx.sessionConfig.reasoning;
                    const sessionReasoningValue = sessionReasoningLevel !== undefined
                        ? this.resolveReasoningValue(provider, model, sessionReasoningLevel, this.ctx.sessionConfig.maxOutputTokens)
                        : undefined;
                    if (this.newCtxTokens > 0) {
                        this.pendingCtxTokens += this.newCtxTokens;
                        this.newCtxTokens = 0;
                    }
                    const guardEvaluation = this.evaluateContextGuard(this.schemaCtxTokens);
                    if (guardEvaluation.blocked.length > 0) {
                        this.enforceContextFinalTurn(guardEvaluation.blocked, 'turn_preflight');
                        syncFinalTurnFlags();
                        const finalToolSelection = this.selectToolsForTurn(provider, true);
                        this.state.pendingToolSelection = finalToolSelection;
                        this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
                        const postEnforceGuard = this.evaluateContextGuard(this.schemaCtxTokens);
                        if (postEnforceGuard.blocked.length > 0) {
                            const [firstBlocked] = postEnforceGuard.blocked;
                            const warnEntry = {
                                timestamp: Date.now(),
                                severity: 'WRN' as const,
                                turn: currentTurn,
                                subturn: 0,
                                direction: 'response' as const,
                                type: 'llm' as const,
                                remoteIdentifier: REMOTE_CONTEXT,
                                fatal: false,
                                message: 'Context guard post-shrink still over projected limit during turn execution; proceeding anyway.',
                                details: {
                                    projected_tokens: postEnforceGuard.projectedTokens,
                                    limit_tokens: firstBlocked.limit,
                                    provider: firstBlocked.provider,
                                    model: firstBlocked.model,
                                },
                            };
                            this.log(warnEntry);
                        }
                    }
                    const turnResult = await this.executeSingleTurn(attemptConversation, provider, model, isFinalTurn, currentTurn, logs, accounting, lastShownThinkingHeaderTurn, attempts, maxRetries, sessionReasoningLevel, sessionReasoningValue ?? undefined);
                    const { messages: sanitizedMessages, dropped: droppedInvalidToolCalls, finalReportAttempted } = this.sanitizeTurnMessages(turnResult.messages, { turn: currentTurn, provider, model, stopReason: turnResult.stopReason });
                    // Mark turnResult with finalReportAttempted flag but defer collapseRemainingTurns
                    // until we know if the final report was actually accepted (done later in the flow)
                    if (finalReportAttempted) {
                        (turnResult.status as { finalReportAttempted?: boolean }).finalReportAttempted = true;
                        turnHadFinalReportAttempt = true;
                    }
                    sanitizedMessages.forEach((message) => {
                        if (message.role !== 'tool')
                            return;
                        const callId = message.toolCallId;
                        if (typeof callId !== 'string' || callId.length === 0)
                            return;
                        const override = this.state.toolFailureMessages.get(callId);
                        if (override === undefined)
                            return;
                        message.content = override;
                        this.state.toolFailureMessages.delete(callId);
                    });
                    if (this.state.toolFailureFallbacks.length > 0) {
                        sanitizedMessages.forEach((message) => {
                            if (this.state.toolFailureFallbacks.length === 0)
                                return;
                            if (message.role !== 'tool')
                                return;
                            const content = typeof message.content === 'string' ? message.content.trim() : '';
                            if (content.length > 0 && content !== TOOL_NO_OUTPUT)
                                return;
                            const fallback = this.state.toolFailureFallbacks.shift();
                            if (fallback === undefined)
                                return;
                            message.content = fallback;
                        });
                        this.state.toolFailureFallbacks.length = 0;
                    }
                    let reasoningHeaderEmitted = false;
                    if (!turnResult.shownThinking) {
                        const reasoningChunks: string[] = [];
                        sanitizedMessages.forEach((message) => {
                            if (message.role !== 'assistant')
                                return;
                            const segments = Array.isArray(message.reasoning) ? message.reasoning : [];
                            segments.forEach((segment) => {
                                const text = segment.text;
                                if (typeof text === 'string' && text.trim().length > 0) {
                                    reasoningChunks.push(text);
                                }
                            });
                        });
                        if (reasoningChunks.length > 0) {
                            reasoningHeaderEmitted = true;
                            if (lastShownThinkingHeaderTurn !== currentTurn) {
                                const thinkingHeader = {
                                    timestamp: Date.now(),
                                    severity: 'THK' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: 'thinking',
                                    fatal: false,
                                    message: 'reasoning output stream',
                                };
                                this.log(thinkingHeader);
                            }
                            if (this.ctx.parentTxnId === undefined) {
                                this.emitReasoningChunks(reasoningChunks);
                            }
                        }
                    }
                    if (turnResult.shownThinking || reasoningHeaderEmitted) {
                        lastShownThinkingHeaderTurn = currentTurn;
                    }
                    // Emit WRN for unknown tool calls
                    try {
                        const internal = new Set([FINAL_REPORT_TOOL]);
                        if (this.ctx.progressToolEnabled)
                            internal.add('agent__progress_report');
                        if (this.ctx.sessionConfig.tools.includes('batch'))
                            internal.add('agent__batch');
                        const normalizeTool = (n: string): string => n.replace(/^<\|[^|]+\|>/, '').trim();
                        const assistantMessages = sanitizedMessages.filter((m) => m.role === 'assistant');
                        const assistantMsg = assistantMessages.length > 0 ? assistantMessages[assistantMessages.length - 1] : undefined;
                        if (assistantMsg?.toolCalls !== undefined && assistantMsg.toolCalls.length > 0) {
                            (assistantMsg.toolCalls).forEach((tc) => {
                                const n = normalizeTool(tc.name);
                                const known = (this.ctx.toolsOrchestrator?.hasTool(n) ?? false) || internal.has(n);
                                if (!known) {
                                    const req = formatToolRequestCompact(n, tc.parameters);
                                    const warnLog = {
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: 'assistant:tool',
                                        fatal: false,
                                        message: `Unknown tool requested (not executed): ${req}`
                                    };
                                    this.log(warnLog);
                                }
                            });
                        }
                    }
                    catch (e) {
                        warn(`unknown tool warning failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                    // Record accounting for every attempt
                    {
                        const tokens = turnResult.tokens ?? { inputTokens: 0, outputTokens: 0, cachedTokens: 0, totalTokens: 0 };
                        try {
                            const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                            const w = tokens.cacheWriteInputTokens ?? 0;
                            const totalWithCache = tokens.inputTokens + tokens.outputTokens + r + w;
                            if (Number.isFinite(totalWithCache))
                                tokens.totalTokens = totalWithCache;
                        }
                        catch { /* keep provider totalTokens if something goes wrong */ }
                        const metadata = turnResult.providerMetadata;
                        const computeCost = () => {
                            try {
                                const pricing = (this.ctx.config.pricing ?? {});
                                const effectiveProvider = metadata?.actualProvider ?? provider;
                                const effectiveModel = metadata?.actualModel ?? model;
                                // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for missing config keys
                                const modelTable = pricing[effectiveProvider]?.[effectiveModel];
                                // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for missing config keys
                                if (modelTable === undefined)
                                    return {};
                                const denom = modelTable.unit === 'per_1k' ? 1000 : 1_000_000;
                                const pIn = modelTable.prompt ?? 0;
                                const pOut = modelTable.completion ?? 0;
                                const pRead = modelTable.cacheRead ?? 0;
                                const pWrite = modelTable.cacheWrite ?? 0;
                                const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                                const w = tokens.cacheWriteInputTokens ?? 0;
                                const cost = (pIn * tokens.inputTokens + pOut * tokens.outputTokens + pRead * r + pWrite * w) / denom;
                                return { costUsd: Number.isFinite(cost) ? cost : undefined };
                            }
                            catch {
                                return {};
                            }
                        };
                        const computed = computeCost();
                        const accountingEntry = {
                            type: 'llm' as const,
                            timestamp: Date.now(),
                            status: turnResult.status.type === 'success' ? 'ok' as const : 'failed' as const,
                            latency: turnResult.latencyMs,
                            provider,
                            model,
                            actualProvider: metadata?.actualProvider,
                            actualModel: metadata?.actualModel,
                            costUsd: metadata?.reportedCostUsd ?? computed.costUsd,
                            upstreamInferenceCostUsd: metadata?.upstreamCostUsd,
                            stopReason: turnResult.stopReason,
                            tokens,
                            error: turnResult.status.type !== 'success' ?
                                ('message' in turnResult.status ? turnResult.status.message : turnResult.status.type) : undefined,
                            agentId: this.ctx.agentId,
                            callPath: this.ctx.callPath,
                            txnId: this.ctx.txnId,
                            parentTxnId: this.ctx.parentTxnId,
                            originTxnId: this.ctx.originTxnId
                        };
                        accounting.push(accountingEntry);
                        try {
                            if (llmOpId !== undefined)
                                this.ctx.opTree.appendAccounting(llmOpId, accountingEntry);
                        }
                        catch (e) {
                            warn(`appendAccounting failed: ${e instanceof Error ? e.message : String(e)}`);
                        }
                        try {
                            this.callbacks.onAccounting(accountingEntry);
                        }
                        catch (e) {
                            warn(`onAccounting callback failed: ${e instanceof Error ? e.message : String(e)}`);
                        }
                        // Update context guard with actual response tokens
                        const cacheRead = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                        const responseCtxTokens = tokens.inputTokens + tokens.outputTokens + cacheRead;
                        this.currentCtxTokens = responseCtxTokens;
                        this.pendingCtxTokens = 0;
                        this.schemaCtxTokens = 0;
                    }
                    // Close hierarchical LLM op
                    try {
                        if (llmOpId !== undefined) {
                            const lastAssistant = [...sanitizedMessages].filter((m) => m.role === 'assistant').pop();
                            const respText = typeof lastAssistant?.content === 'string' ? lastAssistant.content : (turnResult.response ?? '');
                            const sz = respText.length > 0 ? Buffer.byteLength(respText, 'utf8') : 0;
                            this.ctx.opTree.setResponse(llmOpId, { payload: { textPreview: respText.slice(0, 4096) }, size: sz, truncated: respText.length > 4096 });
                            this.ctx.opTree.endOp(llmOpId, (turnResult.status.type === 'success') ? 'ok' : 'failed', { latency: turnResult.latencyMs });
                            this.callbacks.onOpTree(this.ctx.opTree.getSession());
                        }
                    }
                    catch (e) {
                        warn(`finalize LLM op failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                    this.callbacks.setCurrentLlmOpId(undefined);
                    // Capture planned subturns for this turn (enriches subsequent logs)
                    try {
                        const lastAssistantForSubturns = [...sanitizedMessages].filter((m) => m.role === 'assistant').pop();
                        if (lastAssistantForSubturns !== undefined && Array.isArray(lastAssistantForSubturns.toolCalls)) {
                            const toolCalls = lastAssistantForSubturns.toolCalls;
                            // Count only MCP tools (exclude internal agent__* and sub-agent tools)
                            const count = toolCalls.reduce((acc, tc) => {
                                const name = (typeof tc.name === 'string' ? tc.name : '').trim();
                                if (name.length === 0)
                                    return acc;
                                if (name === 'agent__progress_report' || name === FINAL_REPORT_TOOL || name === 'agent__batch')
                                    return acc;
                                // Count only known tools (non-internal) present in orchestrator
                                const isKnown = this.ctx.toolsOrchestrator?.hasTool(name) ?? false;
                                return isKnown ? acc + 1 : acc;
                            }, 0);
                            this.state.plannedSubturns.set(currentTurn, count);
                        }
                    }
                    catch (e) {
                        warn(`LLM accounting callback failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                    // Handle turn result based on status
                    if (turnResult.status.type === 'success') {
                        const assistantMessagesSanitized = sanitizedMessages.filter((m) => m.role === 'assistant');
                        const lastAssistantSanitized = assistantMessagesSanitized.length > 0 ? assistantMessagesSanitized[assistantMessagesSanitized.length - 1] : undefined;
                        const assistantForAdoption = lastAssistantSanitized;
                        if (assistantForAdoption !== undefined && Array.isArray(assistantForAdoption.toolCalls)) {
                            const expandedToolCalls = this.expandNestedLLMToolCalls(assistantForAdoption.toolCalls);
                            if (expandedToolCalls !== undefined) {
                                assistantForAdoption.toolCalls = expandedToolCalls;
                            }
                        }
                        let sanitizedHasToolCalls = assistantForAdoption !== undefined && Array.isArray(assistantForAdoption.toolCalls) && assistantForAdoption.toolCalls.length > 0;
                        const textContent = assistantForAdoption !== undefined && typeof assistantForAdoption.content === 'string' ? assistantForAdoption.content : undefined;
                        let sanitizedHasText = textContent !== undefined && textContent.length > 0;
                        const toolFailureDetected = sanitizedMessages.some((msg) => msg.role === 'tool' && typeof msg.content === 'string' && msg.content.trim().toLowerCase().startsWith('(tool failed:'));
                        if (toolFailureDetected) {
                            const failureToolMessage = sanitizedMessages
                                .filter((msg) => msg.role === 'tool' && typeof msg.content === 'string')
                                .find((msg) => {
                                const trimmed = msg.content.trim().toLowerCase();
                                return trimmed.startsWith('(tool failed:');
                            });
                            if (failureToolMessage !== undefined) {
                                const failureCallId = failureToolMessage.toolCallId;
                                if (typeof failureCallId === 'string') {
                                    const override = this.state.toolFailureMessages.get(failureCallId);
                                    if (typeof override === 'string' && override.length > 0) {
                                        failureToolMessage.content = override;
                                    }
                                    // Check if failed tool was the final report tool
                                    const owningAssistant = sanitizedMessages.find((msg) => msg.role === 'assistant' && Array.isArray(msg.toolCalls));
                                    const owningToolCalls = owningAssistant?.toolCalls;
                                    if (owningToolCalls !== undefined) {
                                        const relatedCall = owningToolCalls.find((tc) => tc.id === failureCallId);
                                        if (relatedCall !== undefined) {
                                            const failureToolNameNormalized = sanitizeToolName(relatedCall.name);
                                            if (failureToolNameNormalized === FINAL_REPORT_TOOL) {
                                                finalReportToolFailed = true;
                                            }
                                        }
                                    }
                                    this.state.toolFailureMessages.delete(failureCallId);
                                }
                            }
                        }
                        if (this.ctx.toolTransport === 'native' && this.finalReport === undefined && !sanitizedHasToolCalls && sanitizedHasText && assistantForAdoption !== undefined) {
                            const extracted = this.tryAdoptFinalReportFromText(assistantForAdoption, textContent);
                            if (extracted !== undefined) {
                                this.ctx.finalReportManager.setPending(extracted, FINAL_REPORT_SOURCE_TEXT_FALLBACK);
                                const warnEntry = {
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: 'agent:text-extraction',
                                    fatal: false,
                                    message: 'Final report extracted from text content after tool call rejection. Retrying for proper tool call.'
                                };
                                this.log(warnEntry);
                            }
                        }
                        // FALLBACK: Extract and execute tool calls from leaked XML-like patterns
                        // Handles models that emit <tools>, <tool_call>, etc. instead of native tool calls
                        if (!sanitizedHasToolCalls && sanitizedHasText && assistantForAdoption !== undefined && textContent !== undefined) {
                            const fallbackResult = await this.processLeakedToolCallsFallback(textContent, currentTurn, provider);
                            if (fallbackResult !== undefined) {
                                assistantForAdoption.toolCalls = fallbackResult.toolCalls;
                                assistantForAdoption.content = fallbackResult.cleanedContent ?? '';
                                sanitizedHasToolCalls = true;
                                sanitizedHasText = fallbackResult.hasRemainingText;
                                // Add tool results to messages
                                sanitizedMessages.push(...fallbackResult.toolResults);
                                // Log extraction
                                const patterns = fallbackResult.patternsMatched.map((p) => `<${p}>`).join(', ');
                                this.log({
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: REMOTE_SANITIZER,
                                    fatal: false,
                                    message: `Extracted and executed ${String(fallbackResult.toolCalls.length)} tool call(s) from leaked XML patterns (${patterns}).`,
                                });
                            }
                        }
                        if ((turnResult.status.finalAnswer || this.ctx.toolTransport !== 'native') && this.finalReport === undefined) {
                            const adoptFromToolCall = () => {
                                if (toolFailureDetected)
                                    return false;
                                const assistantWithCall = sanitizedMessages.find((msg) => msg.role === 'assistant' && Array.isArray(msg.toolCalls));
                                if (assistantWithCall === undefined)
                                    return false;
                                // Capture raw content for debugging dumps
                                const rawContent = typeof assistantWithCall.content === 'string' ? assistantWithCall.content : undefined;
                                const toolCalls = assistantWithCall.toolCalls;
                                if (toolCalls === undefined)
                                    return false;
                                const finalReportCall = toolCalls.find((call) => sanitizeToolName(call.name) === FINAL_REPORT_TOOL);
                                if (finalReportCall === undefined)
                                    return false;
                                const params = finalReportCall.parameters;
                                const expectedFormat = this.ctx.resolvedFormat ?? 'text';

                                // =================================================================
                                // LAYER 1: Transport/Wrapper extraction
                                // Extract wrapper fields (format) separately from payload
                                // Status is always 'success' for model-provided final reports
                                // =================================================================
                                const normalizedStatus: 'success' | 'failure' = 'success';
                                const formatParamRaw = params.report_format;
                                const formatParam = typeof formatParamRaw === 'string' && formatParamRaw.trim().length > 0 ? formatParamRaw.trim() : undefined;
                                const metadataCandidate = params.metadata;
                                const reportMetadata = (metadataCandidate !== null && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate)) ? metadataCandidate : undefined;

                                // Get raw payload: from XML transport (_rawPayload) or native fields
                                const rawPayloadValue = params._rawPayload;
                                const rawPayload = typeof rawPayloadValue === 'string' ? rawPayloadValue.trim() : undefined;

                                // Native transport fallbacks (when _rawPayload not present)
                                const contentValue = params.report_content;
                                const contentParam = typeof contentValue === 'string' && contentValue.trim().length > 0 ? contentValue.trim() : undefined;
                                const messagesValue = params.messages;
                                const messagesParam = Array.isArray(messagesValue) ? messagesValue : undefined;
                                const contentJsonCandidate = params.content_json;

                                // Validate wrapper: format must match expected
                                const formatCandidate = formatParam ?? expectedFormat;
                                const finalFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate) ?? expectedFormat;
                                if (finalFormat !== expectedFormat) {
                                    this.addTurnFailure(finalReportFormatMismatch(expectedFormat, formatCandidate));
                                    return false;
                                }

                                // =================================================================
                                // LAYER 2: Format-specific content processing
                                // Process raw payload based on format requirements
                                // =================================================================
                                let finalContent: string | undefined;
                                let contentJson: Record<string, unknown> | undefined;

                                if (expectedFormat === 'sub-agent') {
                                    // SUB-AGENT: Opaque blob - no validation, no parsing
                                    // Whatever the model returns is passed through unchanged
                                    finalContent = rawPayload ?? contentParam ?? '';
                                    if (finalContent.length === 0) {
                                        this.addTurnFailure(FINAL_REPORT_CONTENT_MISSING);
                                        this.logFinalReportDump(currentTurn, params, 'empty sub-agent payload', rawContent);
                                        return false;
                                    }
                                } else if (expectedFormat === 'json') {
                                    // JSON: Parse and validate against schema
                                    // Source: rawPayload (XML) > content_json (native) > contentParam
                                    const jsonSource = rawPayload ?? (typeof contentJsonCandidate === 'string' ? contentJsonCandidate : undefined);
                                    if (jsonSource !== undefined) {
                                        const parsedJson = parseJsonValueDetailed(jsonSource);
                                        if (parsedJson.value !== undefined && parsedJson.value !== null && typeof parsedJson.value === 'object' && !Array.isArray(parsedJson.value)) {
                                            contentJson = parsedJson.value as Record<string, unknown>;
                                            if (parsedJson.repairs.length > 0) {
                                                this.log({
                                                    timestamp: Date.now(),
                                                    severity: 'WRN' as const,
                                                    turn: currentTurn,
                                                    subturn: 0,
                                                    direction: 'response' as const,
                                                    type: 'llm' as const,
                                                    remoteIdentifier: REMOTE_SANITIZER,
                                                    fatal: false,
                                                    message: `agent__final_report JSON payload repaired via [${parsedJson.repairs.join('>')}]`,
                                                });
                                            }
                                        }
                                    } else if (contentJsonCandidate !== null && typeof contentJsonCandidate === 'object' && !Array.isArray(contentJsonCandidate)) {
                                        // Native: content_json already parsed as object
                                        contentJson = contentJsonCandidate as Record<string, unknown>;
                                    }
                                    if (contentJson === undefined) {
                                        this.addTurnFailure(FINAL_REPORT_JSON_REQUIRED);
                                        this.logFinalReportDump(currentTurn, params, 'expected JSON content', rawContent);
                                        return false;
                                    }
                                    finalContent = JSON.stringify(contentJson);
                                } else if (expectedFormat === SLACK_BLOCK_KIT_FORMAT) {
                                    // SLACK-BLOCK-KIT: Parse JSON, expect messages array
                                    // Source: rawPayload (XML) > messages (native)
                                    let messagesArray: unknown[] | undefined;
                                    if (rawPayload !== undefined) {
                                        // XML transport: parse JSON from raw payload
                                        const parsedJson = parseJsonValueDetailed(rawPayload);
                                        if (parsedJson.value !== undefined && parsedJson.value !== null && typeof parsedJson.value === 'object') {
                                            const parsed = parsedJson.value as Record<string, unknown>;
                                            // Could be {messages: [...]} or just [...]
                                            if (Array.isArray(parsed)) {
                                                messagesArray = parsed;
                                            } else if (Array.isArray(parsed.messages)) {
                                                messagesArray = parsed.messages;
                                            }
                                            if (parsedJson.repairs.length > 0) {
                                                this.log({
                                                    timestamp: Date.now(),
                                                    severity: 'WRN' as const,
                                                    turn: currentTurn,
                                                    subturn: 0,
                                                    direction: 'response' as const,
                                                    type: 'llm' as const,
                                                    remoteIdentifier: REMOTE_SANITIZER,
                                                    fatal: false,
                                                    message: `agent__final_report slack-block-kit repaired via [${parsedJson.repairs.join('>')}]`,
                                                });
                                            }
                                        }
                                    } else {
                                        // Native transport: use messages param directly
                                        messagesArray = messagesParam;
                                    }
                                    if (messagesArray === undefined || messagesArray.length === 0) {
                                        this.addTurnFailure(FINAL_REPORT_SLACK_MESSAGES_MISSING);
                                        this.logFinalReportDump(currentTurn, params, 'expected messages array', rawContent);
                                        return false;
                                    }
                                    finalContent = JSON.stringify(messagesArray);
                                } else {
                                    // TEXT FORMATS (text, markdown, markdown+mermaid, tty, pipe)
                                    // Use raw payload or report_content directly
                                    finalContent = rawPayload ?? contentParam;
                                    if (finalContent === undefined || finalContent.length === 0) {
                                        this.addTurnFailure(FINAL_REPORT_CONTENT_MISSING);
                                        this.logFinalReportDump(currentTurn, params, 'expected report_content', rawContent);
                                        return false;
                                    }
                                }

                                // =================================================================
                                // LAYER 3: Final Report construction
                                // Build clean final report object
                                // =================================================================
                                this.commitFinalReport({
                                    status: normalizedStatus,
                                    format: finalFormat,
                                    content: finalContent,
                                    content_json: contentJson,
                                    metadata: reportMetadata as Record<string, unknown> | undefined,
                                }, 'tool-call');
                                sanitizedHasToolCalls = true;
                                return true;
                            };
                            const adoptFromToolMessage = () => {
                                if (this.ctx.toolTransport !== 'native')
                                    return undefined;
                                if (toolFailureDetected)
                                    return undefined;
                                if ((this.ctx.resolvedFormat ?? 'text') === 'json')
                                    return undefined;
                                const toolMessage = sanitizedMessages.find((msg) => msg.role === 'tool' && typeof msg.content === 'string' && msg.content.trim().length > 0);
                                if (toolMessage === undefined || typeof toolMessage.content !== 'string')
                                    return undefined;
                                const normalizedContent = toolMessage.content.trim();
                                if (normalizedContent.length === 0)
                                    return undefined;
                                const finalFormat = this.ctx.resolvedFormat ?? 'text';
                                return {
                                    status: 'success' as const,
                                    format: (FINAL_REPORT_FORMAT_VALUES.find((value) => value === finalFormat) ?? 'text'),
                                    content: normalizedContent,
                                };
                            };
                            if (!adoptFromToolCall()) {
                                if (!toolFailureDetected) {
                                    const pending = adoptFromToolMessage();
                                    if (pending !== undefined) {
                                        this.ctx.finalReportManager.setPending(pending, FINAL_REPORT_SOURCE_TOOL_MESSAGE);
                                        const warnEntry = {
                                            timestamp: Date.now(),
                                            severity: 'WRN' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: 'agent:text-extraction',
                                            fatal: false,
                                            message: 'Final report extracted from tool output after tool call rejection. Retrying for proper tool call.'
                                        };
                                        this.log(warnEntry);
                                    }
                                }
                            }
                        }
                        if (droppedInvalidToolCalls > 0) {
                            const warnEntry = {
                                timestamp: Date.now(),
                                severity: 'WRN' as const,
                                turn: currentTurn,
                                subturn: 0,
                                direction: 'response' as const,
                                type: 'llm' as const,
                                remoteIdentifier: REMOTE_SANITIZER,
                                fatal: false,
                                message: `Dropped ${String(droppedInvalidToolCalls)} invalid tool call(s) due to malformed payload. Will retry.`,
                            };
                            this.log(warnEntry);
                            this.addTurnFailure(TOOL_CALL_MALFORMED);
                            lastError = 'invalid_response: malformed_tool_call';
                            lastErrorType = 'invalid_response';
                            if (finalReportAttempted) {
                                collapseRemainingTurns('final_report_attempt');
                            }
                        }
                        // Synthetic error: success with content but no tools and no final_report
                        if (this.finalReport === undefined) {
                            if (!turnResult.status.finalAnswer && !sanitizedHasToolCalls && sanitizedHasText) {
                                const syntheticMessage = 'Synthetic retry: assistant returned content without tool calls and without final_report.';
                                const providerWarnEntry = {
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: `${provider}:${model}`,
                                    fatal: false,
                                    message: syntheticMessage,
                                };
                                this.log(providerWarnEntry);
                                const orchestratorWarnEntry = {
                                    ...providerWarnEntry,
                                    remoteIdentifier: REMOTE_ORCHESTRATOR,
                                };
                                this.log(orchestratorWarnEntry);
                                if (isFinalTurn && !finalTurnRetryWarnLogged) {
                                    const agentWarn = {
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: REMOTE_FINAL_TURN,
                                        fatal: false,
                                        message: finalTurnRetryMessage
                                    };
                                    this.log(agentWarn);
                                    finalTurnRetryWarnLogged = true;
                                }
                                lastError = 'invalid_response: content_without_tools_or_final';
                                lastErrorType = 'invalid_response';
                                this.addTurnFailure(TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT);
                            }
                        }
                        // CONTRACT: Empty response without tool calls must NOT be added to conversation
                        // Check BEFORE pushing to conversation
                        const isEmptyWithoutTools = !turnResult.status.finalAnswer && !turnResult.status.hasToolCalls &&
                            (turnResult.response === undefined || turnResult.response.trim().length === 0);
                        if (isEmptyWithoutTools && turnResult.hasReasoning !== true) {
                            // Log warning and retry this turn on another provider/model
                            const metadata = turnResult.providerMetadata;
                            const remoteId = this.composeRemoteIdentifier(provider, model, metadata);
                            const warnEntry = {
                                timestamp: Date.now(),
                                severity: 'WRN' as const,
                                turn: currentTurn,
                                subturn: 0,
                                direction: 'response' as const,
                                type: 'llm' as const,
                                remoteIdentifier: remoteId,
                                fatal: false,
                                message: 'Empty response without tools; retrying with next provider/model in this turn.'
                            };
                            this.log(warnEntry);
                            this.state.llmSyntheticFailures++;
                            lastError = 'invalid_response: empty_without_tools';
                            lastErrorType = 'invalid_response';
                            if (cycleComplete) {
                                rateLimitedInCycle = 0;
                                maxRateLimitWaitMs = 0;
                            }
                            // CONTRACT: Only inject ephemeral retry message, do NOT add empty response
                            this.pushSystemRetryMessage(conversation, emptyResponseRetryNotice(FINAL_REPORT_TOOL));
                            // do not mark turnSuccessful; continue retry loop
                            continue;
                        }
                        // Add new messages to conversation (skipped above for empty responses per CONTRACT)
                        conversation.push(...sanitizedMessages);
                        if (this.state.turnFailureReasons.length > 0) {
                            const failureNotice = turnFailedPrefix(this.state.turnFailureReasons);
                            conversation.push({ role: 'user', content: failureNotice });
                            this.state.turnFailureReasons = [];
                        }
                        const finalReportReady = this.finalReport !== undefined;
                        // If we encountered an invalid response and still have retries in this turn, retry
                        if (!finalReportReady && lastErrorType === 'invalid_response' && attempts < maxRetries) {
                            // Collapse turns on retry if a final report was attempted but rejected
                            if (turnHadFinalReportAttempt && currentTurn < maxTurns) {
                                collapseRemainingTurns('final_report_attempt');
                            }
                            continue;
                        }
                        // Check for retryFlags from turnResult (incompleteFinalReportDetected comes from tool execution)
                        const retryFlags = turnResult as { incompleteFinalReportDetected?: boolean; finalReportAttempted?: boolean };
                        const finalReportAttemptFlag = turnHadFinalReportAttempt || retryFlags.finalReportAttempted === true;
                        // If final report is already accepted, skip collapse and finalize immediately
                        if (finalReportReady) {
                            return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
                        }
                        // Only collapse remaining turns if final report is NOT ready
                        if (retryFlags.incompleteFinalReportDetected === true && currentTurn < maxTurns) {
                            collapseRemainingTurns('incomplete_final_report');
                        }
                        else if (finalReportAttemptFlag && currentTurn < maxTurns) {
                            collapseRemainingTurns('final_report_attempt');
                        }
                        // Note: empty response check moved earlier (before conversation.push) per CONTRACT
                        if (turnResult.status.finalAnswer) {
                            // Accept pending report from text/tool-message extraction before checking status
                            // Only accept if no tool calls were dropped (dropped calls trigger retry)
                            // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime check for pending adoption
                            if (this.finalReport === undefined && droppedInvalidToolCalls === 0) {
                                const acceptedSource = this.ctx.finalReportManager.acceptPending();
                                if (acceptedSource !== undefined) {
                                    this.logFallbackAcceptance(acceptedSource, currentTurn);
                                }
                            }
                            const statusCandidate = this.getFinalReportStatus();
                            const finalReportStatus = statusCandidate === 'success' || statusCandidate === 'failure'
                                ? statusCandidate
                                : 'missing';
                            // Both 'success' and 'failure' are valid final report outcomes
                            // Only 'missing' (incomplete/malformed) should trigger retry
                            if (finalReportStatus !== 'missing') {
                                return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
                            }
                            const warnEntry = {
                                timestamp: Date.now(),
                                severity: 'WRN' as const,
                                turn: currentTurn,
                                subturn: 0,
                                direction: 'response' as const,
                                type: 'llm' as const,
                                remoteIdentifier: 'agent:final-report',
                                fatal: false,
                                message: `Final report incomplete (status='${finalReportStatus}'), retrying.`,
                            };
                            this.log(warnEntry);
                            // Track that the model attempted a final_report even though it was incomplete.
                            turnHadFinalReportAttempt = true;
                            collapseRemainingTurns('incomplete_final_report');
                            this.state.llmSyntheticFailures++;
                            lastError = `final_report_status_${finalReportStatus}`;
                            lastErrorType = 'invalid_response';
                            if (cycleComplete) {
                                rateLimitedInCycle = 0;
                                maxRateLimitWaitMs = 0;
                            }
                            this.pushSystemRetryMessage(conversation, this.buildFinalReportReminder());
                            continue;
                        }
                        else {
                            // Not a final answer yet
                            if (isFinalTurn) {
                                // On final turn, retry for final report
                                const metadata = turnResult.providerMetadata;
                                const remoteId = this.composeRemoteIdentifier(provider, model, metadata);
                                const warnEntry = {
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: remoteId,
                                    fatal: false,
                                    message: finalTurnRetryMessage
                                };
                                this.log(warnEntry);
                                if (!finalTurnRetryWarnLogged) {
                                    const agentWarn = {
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: REMOTE_FINAL_TURN,
                                        fatal: false,
                                        message: finalTurnRetryMessage
                                    };
                                    this.log(agentWarn);
                                    finalTurnRetryWarnLogged = true;
                                }
                                // Continue attempts loop (do not mark successful)
                                lastError = 'invalid_response: final_turn_no_final_answer';
                                lastErrorType = 'invalid_response';
                                if (cycleComplete) {
                                    rateLimitedInCycle = 0;
                                    maxRateLimitWaitMs = 0;
                                }
                                this.pushSystemRetryMessage(conversation, FINAL_TURN_NOTICE);
                                this.pushSystemRetryMessage(conversation, this.buildFinalReportReminder());
                                continue;
                            }
                            // Non-final turns: proceed to next turn; tools already executed if present
                            if (lastErrorType === 'invalid_response') {
                                continue;
                            }
                            turnSuccessful = true;
                            break;
                        }
                    }
                    else {
                        // Handle non-success status (rate_limit, auth_error, etc.)
                        // Set lastError/lastErrorType BEFORE retry directive handling so they're available if retries exhaust
                        lastError = 'message' in turnResult.status ? turnResult.status.message : turnResult.status.type;
                        lastErrorType = turnResult.status.type;
                        const remoteId = this.composeRemoteIdentifier(provider, model, turnResult.providerMetadata);
                        const directive = this.buildFallbackRetryDirective({ status: turnResult.status, remoteId, attempt: attempts });
                        if (directive !== undefined) {
                            if (directive.action === RETRY_ACTION_SKIP_PROVIDER) {
                                const skipEntry = {
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: remoteId,
                                    fatal: false,
                                    message: directive.logMessage ?? `Skipping provider ${remoteId}`,
                                };
                                this.log(skipEntry);
                                // Skip this provider and continue to next
                                continue;
                            }
                            if (directive.action === 'retry') {
                                // Special handling for rate_limit: track across cycle
                                if (turnResult.status.type === 'rate_limit') {
                                    rateLimitedInCycle += 1;
                                    const RATE_LIMIT_MIN_WAIT_MS = 1_000;
                                    const RATE_LIMIT_MAX_WAIT_MS = 60_000;
                                    const fallbackWait = Math.min(Math.max(attempts * 1_000, RATE_LIMIT_MIN_WAIT_MS), RATE_LIMIT_MAX_WAIT_MS);
                                    const recommendedWait = directive.backoffMs;
                                    const waitCandidate = typeof recommendedWait === 'number' && Number.isFinite(recommendedWait)
                                        ? recommendedWait
                                        : fallbackWait;
                                    const effectiveWaitMs = Math.min(Math.max(Math.round(waitCandidate), RATE_LIMIT_MIN_WAIT_MS), RATE_LIMIT_MAX_WAIT_MS);
                                    maxRateLimitWaitMs = Math.max(maxRateLimitWaitMs, effectiveWaitMs);
                                    const sources = turnResult.status.sources;
                                    const sourceDetails = Array.isArray(sources) && sources.length > 0
                                        ? ` Sources: ${sources.join(' | ')}`
                                        : '';
                                    const logMessage = directive.logMessage
                                        ?? `Rate limited; suggested wait ${String(effectiveWaitMs)}ms before retry.${sourceDetails}`;
                                    const warnEntry = {
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: remoteId,
                                        fatal: false,
                                        message: logMessage,
                                    };
                                    this.log(warnEntry);
                                    const waitText = effectiveWaitMs > 0 ? `${String(effectiveWaitMs)}ms` : 'a short delay';
                                    const retryNotice = directive.systemMessage
                                        ?? `System notice: upstream provider ${remoteId} rate-limited the previous request. Retrying after ${waitText}; no changes required unless rate limits persist.`;
                                    this.pushSystemRetryMessage(conversation, retryNotice);
                                    // Check if all providers in cycle are rate-limited
                                    if (cycleComplete) {
                                        const allRateLimited = rateLimitedInCycle >= pairs.length;
                                        const hasStopRef = this.ctx.stopRef !== undefined;
                                        if (allRateLimited && maxRateLimitWaitMs > 0 && (attempts < maxRetries || hasStopRef)) {
                                            const waitLog = {
                                                timestamp: Date.now(),
                                                severity: 'WRN' as const,
                                                turn: currentTurn,
                                                subturn: 0,
                                                direction: 'response' as const,
                                                type: 'llm' as const,
                                                remoteIdentifier: 'agent:retry',
                                                fatal: false,
                                                message: `All ${String(pairs.length)} providers rate-limited; backing off for ${String(maxRateLimitWaitMs)}ms before retry.`
                                            };
                                            this.log(waitLog);
                                            const sleepResult = await this.sleepWithAbort(maxRateLimitWaitMs);
                                            if (sleepResult === 'aborted_cancel') {
                                                return this.finalizeCanceledSession(conversation, logs, accounting);
                                            }
                                            if (sleepResult === 'aborted_stop') {
                                                return this.finalizeGracefulStopSession(conversation, logs, accounting);
                                            }
                                        }
                                        rateLimitedInCycle = 0;
                                        maxRateLimitWaitMs = 0;
                                    }
                                    continue;
                                }
                                // Non-rate-limit retry handling
                                const retryEntry = {
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: remoteId,
                                    fatal: false,
                                    message: directive.logMessage ?? `Retrying ${remoteId}`,
                                };
                                this.log(retryEntry);
                                if (directive.backoffMs !== undefined && directive.backoffMs > 0) {
                                    await this.sleepWithAbort(directive.backoffMs);
                                }
                                if (directive.systemMessage !== undefined) {
                                    this.pushSystemRetryMessage(conversation, directive.systemMessage);
                                }
                                continue;
                            }
                        }
                        // lastError/lastErrorType already set at top of else block
                    }
                }
                catch (e) {
                    const msg = e instanceof Error ? e.message : String(e);
                    warn(`Turn execution failed: ${msg}`);
                    lastError = msg;
                    lastErrorType = 'error';
                }
            }
            // Check if all attempts failed for this turn (retry exhaustion)
            if (!turnSuccessful) {
                const isInvalidResponse = lastErrorType === 'invalid_response' || (typeof lastError === 'string' && lastError.startsWith('invalid_response'));
                const isLlmError = lastErrorType === 'network_error' || lastErrorType === 'timeout';
                // For invalid_response errors, continue to next turn (or let loop exit on final turn to generate synthetic report)
                if (isInvalidResponse) {
                    if (currentTurn < maxTurns) {
                        const warnEntry = {
                            timestamp: Date.now(),
                            severity: 'WRN' as const,
                            turn: currentTurn,
                            subturn: 0,
                            direction: 'response' as const,
                            type: 'llm' as const,
                            remoteIdentifier: REMOTE_ORCHESTRATOR,
                            fatal: false,
                            message: `Turn ${String(currentTurn)} exhausted ${String(maxRetries)} attempt${maxRetries === 1 ? '' : 's'} with invalid responses; advancing to the next turn.`,
                        };
                        this.log(warnEntry);
                    }
                    // Continue to next turn, or let loop exit naturally on final turn (synthetic report will be generated)
                }
                else if (isLlmError && currentTurn === maxTurns) {
                    // On final turn with LLM errors (network_error, timeout), allow synthetic report generation
                    // Let loop exit naturally - synthetic report will be generated after the loop
                }
                else {
                    // All attempts failed for this turn - return failure immediately
                    const exitCode = (lastError?.includes('auth') === true) ? 'EXIT-AUTH-FAILURE' :
                        (lastError?.includes('quota') === true) ? 'EXIT-QUOTA-EXCEEDED' :
                            (lastError?.includes('timeout') === true) ? 'EXIT-INACTIVITY-TIMEOUT' :
                                'EXIT-NO-LLM-RESPONSE';
                    const reason = `No LLM response after ${String(maxRetries)} retries across ${String(pairs.length)} provider/model pairs: ${lastError ?? 'All targets failed'}`;
                    this.logExit(exitCode, reason, currentTurn);
                    this.emitFinalSummary(logs, accounting);
                    return {
                        success: false,
                        error: lastError ?? 'All provider/model targets failed',
                        conversation,
                        logs,
                        accounting,
                        childConversations: this.state.childConversations
                    };
                }
            }
            // End of retry loop - close turn if successful
            if (turnSuccessful) {
                try {
                    const lastAssistant = (() => {
                        try {
                            return [...conversation].filter((m) => m.role === 'assistant').pop();
                        }
                        catch {
                            return undefined;
                        }
                    })();
                    const assistantText = typeof lastAssistant?.content === 'string' ? lastAssistant.content : undefined;
                    const attrs = (typeof assistantText === 'string' && assistantText.length > 0) ? { assistant: { content: assistantText } } : {};
                    this.ctx.opTree.endTurn(currentTurn, attrs);
                    this.callbacks.onOpTree(this.ctx.opTree.getSession());
                }
                catch (e) {
                    warn(`endTurn/onOpTree failed: ${e instanceof Error ? e.message : String(e)}`);
                }
            }
            // Preserve lastError for post-loop access
            this.state.lastTurnError = typeof lastError === 'string' ? lastError : undefined;
            this.state.lastTurnErrorType = typeof lastErrorType === 'string' ? lastErrorType : undefined;
        }
        // Max turns exceeded - synthesize final report if missing (unless final_report tool itself failed)
        if (this.finalReport === undefined && !finalReportToolFailed) {
            // Before generating synthetic failure, check if there's a pending fallback we can accept
            const acceptedSource = this.ctx.finalReportManager.acceptPending();
            if (acceptedSource !== undefined) {
                const currentTurn = this.state.currentTurn;
                this.logFallbackAcceptance(acceptedSource, currentTurn);
                // Finalize with the accepted fallback report
                return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
            }
            const currentTurn = this.state.currentTurn;
            // Log the collapse
            const collapseEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: REMOTE_ORCHESTRATOR,
                fatal: false,
                message: `No final report after ${String(currentTurn)} turns; Collapsing remaining turns from ${String(maxTurns)} to ${String(currentTurn)}.`
            };
            this.log(collapseEntry);
            const turnLabel = `${String(maxTurns)} turn${maxTurns === 1 ? '' : 's'}`;
            const baseSummary = `Session completed without a final report after ${turnLabel}.`;
            let detailedSummary = baseSummary;
            const fallbackFormatCandidate = this.ctx.resolvedFormat ?? 'text';
            const fallbackFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === fallbackFormatCandidate) ?? 'text';
            // CONTRACT: synthetic failure when max turns exhausted always uses 'max_turns_exhausted'
            const savedError = this.state.lastTurnError;
            const finalReportAttempts = this.ctx.finalReportManager.finalReportAttempts;
            const metadataReason = 'max_turns_exhausted';
            if (savedError !== undefined && savedError.length > 0) {
                detailedSummary = `${baseSummary} Cause: ${savedError}`;
            }
            const failureDetails: Record<string, LogDetailValue> = {
                reason: metadataReason,
                turns_completed: maxTurns,
                final_report_attempts: finalReportAttempts,
            };
            if (savedError !== undefined && savedError.length > 0) {
                failureDetails.last_error = savedError;
            }
            this.logFailureReport(detailedSummary, this.state.currentTurn, failureDetails);
            this.commitFinalReport({
                status: 'failure',
                format: fallbackFormat,
                content: detailedSummary,
                metadata: {
                    reason: metadataReason,
                    turns_completed: maxTurns,
                    final_report_attempts: finalReportAttempts,
                    last_stop_reason: savedError ?? 'max_turns',
                },
            }, 'synthetic');
            return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, this.state.currentTurn);
        }
        // If final report tool failed, generate synthetic failure report per CONTRACT §5
        // Every session must produce a report (tool, text fallback, or synthetic)
        if (this.finalReport === undefined && finalReportToolFailed) {
            const currentTurn = this.state.currentTurn;
            const turnLabel = `${String(maxTurns)} turn${maxTurns === 1 ? '' : 's'}`;
            const savedError = this.state.lastTurnError;
            const baseSummary = `Session completed without a final report after ${turnLabel}. The final_report tool failed.`;
            const detailedSummary = (typeof savedError === 'string' && savedError.length > 0)
                ? `${baseSummary} Cause: ${savedError}`
                : baseSummary;
            const fallbackFormat = this.ctx.resolvedFormat ?? 'text';
            const failureDetails: Record<string, LogDetailValue> = {
                reason: 'final_report_tool_failed',
                turns_completed: maxTurns,
                final_report_attempts: this.ctx.finalReportManager.finalReportAttempts,
            };
            if (typeof savedError === 'string' && savedError.length > 0) {
                failureDetails.last_error = savedError;
            }
            this.logFailureReport(detailedSummary, currentTurn, failureDetails);
            this.commitFinalReport({
                status: 'failure',
                format: fallbackFormat,
                content: detailedSummary,
                metadata: {
                    reason: 'final_report_tool_failed',
                    turns_completed: maxTurns,
                    final_report_attempts: this.ctx.finalReportManager.finalReportAttempts,
                    last_stop_reason: savedError ?? 'tool_execution_failed',
                },
            }, 'synthetic');
            return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
        }
        this.logExit('EXIT-MAX-TURNS-NO-RESPONSE', `Max turns (${String(maxTurns)}) reached without final response`, this.state.currentTurn);
        this.emitFinalSummary(logs, accounting);
        return {
            success: false,
            error: 'Max tool turns exceeded',
            conversation,
            logs,
            accounting,
            childConversations: this.state.childConversations
        };
    }
    // ============= Helper Methods =============
    private log(entry: LogEntry, opts?: { opId?: string }): void {
        this.callbacks.log(entry, opts);
    }
    private logExit(exitCode: string, reason: string, turn: number, options?: { fatal?: boolean; severity?: LogEntry['severity'] }): void {
        const fatal = options?.fatal ?? true;
        const severity = options?.severity ?? (fatal ? 'ERR' : 'VRB');
        const logEntry: LogEntry = {
            timestamp: Date.now(),
            severity,
            turn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: `agent:${exitCode}`,
            fatal,
            message: `${exitCode}: ${reason} (fatal=${fatal ? 'true' : 'false'})`,
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            txnId: this.ctx.txnId,
            parentTxnId: this.ctx.parentTxnId,
            originTxnId: this.ctx.originTxnId
        };
        this.log(logEntry);
    }
    private logFailureReport(reason: string, turn: number, details?: Record<string, LogDetailValue>): void {
        const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'ERR',
            turn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: REMOTE_FAILURE_REPORT,
            fatal: false,
            message: reason,
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            txnId: this.ctx.txnId,
            parentTxnId: this.ctx.parentTxnId,
            originTxnId: this.ctx.originTxnId,
            ...(details !== undefined ? { details } : {}),
        };
        this.log(entry);
    }
    // Context guard delegation
    private get currentCtxTokens(): number { return this.ctx.contextGuard.getCurrentTokens(); }
    private set currentCtxTokens(value: number) { this.ctx.contextGuard.setCurrentTokens(value); }
    private get pendingCtxTokens(): number { return this.ctx.contextGuard.getPendingTokens(); }
    private set pendingCtxTokens(value: number) { this.ctx.contextGuard.setPendingTokens(value); }
    private get newCtxTokens(): number { return this.ctx.contextGuard.getNewTokens(); }
    private set newCtxTokens(value: number) { this.ctx.contextGuard.setNewTokens(value); }
    private get schemaCtxTokens(): number { return this.ctx.contextGuard.getSchemaTokens(); }
    private set schemaCtxTokens(value: number) { this.ctx.contextGuard.setSchemaTokens(value); }
    private get forcedFinalTurnReason(): 'context' | 'max_turns' | undefined { return this.ctx.contextGuard.getForcedFinalReason(); }
    private get finalReport() { return this.ctx.finalReportManager.getReport(); }
    private estimateTokensForCounters(messages: ConversationMessage[]): number {
        return this.ctx.contextGuard.estimateTokens(messages);
    }
    private estimateToolSchemaTokens(tools: MCPTool[]): number {
        return this.ctx.contextGuard.estimateToolSchemaTokens(tools);
    }
    private evaluateContextGuard(extraTokens = 0): ContextGuardEvaluation {
        return this.ctx.contextGuard.evaluate(extraTokens);
    }
    private evaluateContextForProvider(provider: string, model: string): 'ok' | 'skip' | 'final' {
        return this.ctx.contextGuard.evaluateForProvider(provider, model);
    }
    private enforceContextFinalTurn(blocked: ContextGuardBlockedEntry[], trigger: 'tool_preflight' | 'turn_preflight'): void {
        this.ctx.contextGuard.enforceFinalTurn(blocked, trigger);
    }
    private computeForcedFinalSchemaTokens(provider: string): number {
        const allTools = [...(this.ctx.toolsOrchestrator?.listTools() ?? [])];
        if (allTools.length === 0)
            return this.schemaCtxTokens;
        const filtered = this.filterToolsForProvider(allTools, provider).tools;
        if (filtered.length === 0)
            return this.schemaCtxTokens;
        const finalOnly = filtered.filter((tool: MCPTool) => sanitizeToolName(tool.name) === FINAL_REPORT_TOOL);
        const targetTools = finalOnly.length > 0 ? finalOnly : filtered;
        return this.estimateToolSchemaTokens(targetTools);
    }
    private filterToolsForProvider(tools: MCPTool[], provider: string): { tools: MCPTool[]; allowedNames: Set<string> } {
        void provider;
        const allowedNames = new Set<string>();
        const filtered = tools.map((tool: MCPTool) => {
            const sanitized = sanitizeToolName(tool.name);
            allowedNames.add(sanitized);
            return tool;
        });
        if (!allowedNames.has(FINAL_REPORT_TOOL)) {
            const fallback = tools.find((tool: MCPTool) => sanitizeToolName(tool.name) === FINAL_REPORT_TOOL);
            if (fallback !== undefined)
                allowedNames.add(FINAL_REPORT_TOOL);
        }
        return { tools: filtered, allowedNames };
    }
    private selectToolsForTurn(provider: string, isFinalTurn: boolean): { availableTools: MCPTool[]; allowedToolNames: Set<string>; toolsForTurn: MCPTool[] } {
        const allTools = [...(this.ctx.toolsOrchestrator?.listTools() ?? [])];
        const filteredSelection = this.filterToolsForProvider(allTools, provider);
        const availableTools = filteredSelection.tools;
        const allowedToolNames = filteredSelection.allowedNames;
        const toolsForTurn = isFinalTurn
            ? (() => {
                const filtered = availableTools.filter((tool: MCPTool) => sanitizeToolName(tool.name) === FINAL_REPORT_TOOL);
                return filtered.length > 0 ? filtered : availableTools;
            })()
            : availableTools;
        return { availableTools, allowedToolNames, toolsForTurn };
    }
    private logTurnStart(turn: number): void {
        const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'VRB' as const,
            turn,
            subturn: 0,
            direction: 'request' as const,
            type: 'llm' as const,
            remoteIdentifier: 'agent:turn-start',
            fatal: false,
            message: `Turn ${String(turn)} starting`,
        };
        this.log(entry);
    }
    private logEnteringFinalTurn(reason: string, turn: number): void {
        if (this.state.finalTurnEntryLogged)
            return;
        const message = reason === 'context'
            ? `Context guard enforced: restricting tools to \`${FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
            : `Final turn (${String(turn)}) detected: restricting tools to \`${FINAL_REPORT_TOOL}\`.`;
        const warnEntry: LogEntry = {
            timestamp: Date.now(),
            severity: 'WRN' as const,
            turn,
            subturn: 0,
            direction: 'request' as const,
            type: 'llm' as const,
            remoteIdentifier: REMOTE_FINAL_TURN,
            fatal: false,
            message,
        };
        this.log(warnEntry);
        this.state.finalTurnEntryLogged = true;
    }
    private reportContextGuardEvent(params: { provider: string; model: string; trigger: 'tool_preflight' | 'turn_preflight'; outcome: 'skipped_provider' | 'forced_final'; limitTokens?: number; projectedTokens: number; remainingTokens?: number }): void {
        const { provider, model, trigger, outcome, limitTokens, projectedTokens } = params;
        const directRemaining = params.remainingTokens;
        const hasDirectRemaining = typeof directRemaining === 'number' && Number.isFinite(directRemaining);
        const computedRemaining = hasDirectRemaining
            ? Math.max(0, directRemaining)
            : ((typeof limitTokens === 'number' && Number.isFinite(limitTokens))
                ? Math.max(0, limitTokens - projectedTokens)
                : undefined);
        recordContextGuardMetrics({
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            headendId: this.ctx.headendId,
            provider,
            model,
            trigger,
            outcome,
            limitTokens,
            projectedTokens,
            remainingTokens: computedRemaining,
        });
        const attributes: Record<string, number | string> = {
            'ai.context.trigger': trigger,
            'ai.context.outcome': outcome,
            'ai.context.provider': provider,
            'ai.context.model': model,
            'ai.context.projected_tokens': projectedTokens,
        };
        if (typeof limitTokens === 'number' && Number.isFinite(limitTokens)) {
            attributes['ai.context.limit_tokens'] = limitTokens;
        }
        if (computedRemaining !== undefined) {
            attributes['ai.context.remaining_tokens'] = computedRemaining;
        }
        addSpanEvent('context.guard', attributes);
    }
    private pushSystemRetryMessage(_conversation: ConversationMessage[], message: string): void {
        const trimmed = message.trim();
        if (trimmed.length === 0)
            return;
        if (this.state.pendingRetryMessages.includes(trimmed))
            return;
        this.state.pendingRetryMessages.push(trimmed);
    }
    private addTurnFailure(reason: string): void {
        const trimmed = reason.trim();
        if (trimmed.length === 0)
            return;
        this.state.turnFailureReasons.push(trimmed);
    }
    private logFinalReportDump(turn: number, params: Record<string, unknown>, context: string, rawContent?: string): void {
        try {
            const paramsDump = JSON.stringify(params, null, 2);
            this.log({
                timestamp: Date.now(),
                severity: 'WRN' as const,
                turn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: 'agent:final-report-dump',
                fatal: false,
                message: `Final report params (${context}): ${paramsDump}`,
            });
            // Also log raw content if available (for debugging XML issues)
            if (typeof rawContent === 'string' && rawContent.trim().length > 0) {
                this.log({
                    timestamp: Date.now(),
                    severity: 'WRN' as const,
                    turn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: 'agent:final-report-raw',
                    fatal: false,
                    message: `Raw assistant content:\n${rawContent}`,
                });
            }
        } catch {
            // ignore serialization errors
        }
    }
    private commitFinalReport(payload: PendingFinalReportPayload, source: FinalReportSource): void {
        this.ctx.finalReportManager.commit(payload, source);
    }
    private logFinalReportAccepted(source: string, turn: number): void {
        const entry: LogEntry = {
            timestamp: Date.now(),
            severity: source === 'tool-call' ? 'VRB' : (source === 'synthetic' ? 'ERR' : 'WRN'),
            turn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: 'agent:final-report-accepted',
            fatal: false,
            message: source === 'tool-call'
                ? 'Final report accepted from tool call.'
                : source === FINAL_REPORT_SOURCE_TEXT_FALLBACK
                    ? 'Final report accepted from text extraction fallback.'
                    : source === FINAL_REPORT_SOURCE_TOOL_MESSAGE
                        ? 'Final report accepted from tool-message fallback.'
                        : 'Synthetic final report generated.',
            details: { source }
        };
        this.log(entry);
    }
    private logFallbackAcceptance(source: typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE, turn: number): void {
        const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'WRN' as const,
            turn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: 'agent:fallback-report',
            fatal: false,
            message: source === FINAL_REPORT_SOURCE_TEXT_FALLBACK
                ? 'Accepting final report from text extraction as last resort (final turn, no valid tool call).'
                : 'Accepting final report from tool-message fallback as last resort (final turn, no valid tool call).',
        };
        this.log(entry);
    }
    private finalizeCanceledSession(conversation: ConversationMessage[], logs: LogEntry[], accounting: AccountingEntry[]): AIAgentResult {
        const errMsg = 'canceled';
        this.emitFinalSummary(logs, accounting);
        return { success: false, error: errMsg, conversation, logs, accounting, childConversations: this.state.childConversations };
    }
    private finalizeGracefulStopSession(conversation: ConversationMessage[], logs: LogEntry[], accounting: AccountingEntry[]): AIAgentResult {
        this.emitFinalSummary(logs, accounting);
        return { success: true, conversation, logs, accounting, childConversations: this.state.childConversations };
    }
    private finalizeWithCurrentFinalReport(conversation: ConversationMessage[], logs: LogEntry[], accounting: AccountingEntry[], currentTurn: number): AIAgentResult {
        const fr = this.finalReport;
        if (fr === undefined) {
            throw new Error('finalizeWithCurrentFinalReport called without final report');
        }
        const schema = this.ctx.sessionConfig.expectedOutput?.schema;
        if (schema !== undefined) {
            const validationResult = this.ctx.finalReportManager.validateSchema(schema);
            if (!validationResult.valid) {
                const errs = validationResult.errors ?? 'unknown validation error';
                const payloadPreview = validationResult.payloadPreview;
                const warnLog = {
                    timestamp: Date.now(),
                    severity: 'WRN' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: 'agent:ajv',
                    fatal: false,
                    message: `final_report JSON does not match schema: ${errs}${payloadPreview !== undefined ? `; payload preview=${payloadPreview}` : ''}`
                };
                this.log(warnLog);
            }
        }
        if (this.ctx.sessionConfig.renderTarget !== 'sub-agent' && this.callbacks.onOutput !== undefined) {
            if (!this.finalReportStreamed) {
                const finalOutput = (() => {
                    if (fr.format === 'json' && fr.content_json !== undefined) {
                        try {
                            return JSON.stringify(fr.content_json);
                        }
                        catch {
                            return undefined;
                        }
                    }
                    if (typeof fr.content === 'string' && fr.content.length > 0)
                        return fr.content;
                    return undefined;
                })();
                if (finalOutput !== undefined) {
                    this.callbacks.onOutput(finalOutput);
                    if (!finalOutput.endsWith('\n')) {
                        this.callbacks.onOutput('\n');
                    }
                }
            }
        }
        const source = this.ctx.finalReportManager.getSource() ?? 'tool-call';
        this.logFinalReportAccepted(source, currentTurn);
        const exitCode = this.forcedFinalTurnReason === 'context' ? 'EXIT-TOKEN-LIMIT' : 'EXIT-FINAL-ANSWER';
        const exitMessage = this.forcedFinalTurnReason === 'context'
            ? `Final report received after context guard triggered (${FINAL_REPORT_TOOL}), session complete`
            : `Final report received (${FINAL_REPORT_TOOL}), session complete`;
        this.logExit(exitCode, exitMessage, currentTurn, { fatal: false });
        const metadataRecord = fr.metadata;
        const metadataReason = typeof metadataRecord?.reason === 'string' ? metadataRecord.reason : undefined;
        recordFinalReportMetrics({
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            headendId: this.ctx.headendId,
            source,
            status: fr.status,
            turnsCompleted: currentTurn,
            finalReportAttempts: this.ctx.finalReportManager.finalReportAttempts,
            forcedFinalReason: this.forcedFinalTurnReason,
            syntheticReason: source === 'synthetic' ? metadataReason : undefined,
            customLabels: this.ctx.telemetryLabels,
        });
        this.emitFinalSummary(logs, accounting);
        // success: true for model-provided reports (status='success'), false for synthetic failures (status='failure')
        const success = fr.status === 'success';
        // When status is failure/partial, propagate content as error for display (string or JSON)
        const error = (() => {
            if (success) return undefined;
            if (typeof fr.content === 'string' && fr.content.length > 0) return fr.content;
            if (fr.content_json !== undefined) {
                try { return JSON.stringify(fr.content_json); } catch { /* ignore */ }
            }
            return undefined;
        })();
        return {
            success,
            error,
            conversation,
            logs,
            accounting,
            finalReport: fr,
            childConversations: this.state.childConversations
        };
    }
    private emitFinalSummary(_logs: LogEntry[], accounting: AccountingEntry[]): void {
        try {
            const llmEntries = accounting.filter((e): e is AccountingEntry & { type: 'llm' } => e.type === 'llm');
            const tokIn = llmEntries.reduce((s: number, e) => s + e.tokens.inputTokens, 0);
            const tokOut = llmEntries.reduce((s: number, e) => s + e.tokens.outputTokens, 0);
            const tokCacheRead = llmEntries.reduce((s: number, e) => s + (e.tokens.cacheReadInputTokens ?? e.tokens.cachedTokens ?? 0), 0);
            const tokCacheWrite = llmEntries.reduce((s: number, e) => s + (e.tokens.cacheWriteInputTokens ?? 0), 0);
            const tokTotal = tokIn + tokOut + tokCacheRead + tokCacheWrite;
            const llmLatencies = llmEntries.map((e) => e.latency);
            const llmLatencySum = llmLatencies.reduce((s, v) => s + v, 0);
            const llmLatencyAvg = llmEntries.length > 0 ? Math.round(llmLatencySum / llmEntries.length) : 0;
            const totalCost = llmEntries.reduce((s, e) => s + (e.costUsd ?? 0), 0);
            const totalUpstreamCost = llmEntries.reduce((s, e) => s + (e.upstreamInferenceCostUsd ?? 0), 0);
            const llmRequests = llmEntries.length;
            const llmFailures = llmEntries.filter((e) => e.status === 'failed').length;
            const pairStats = new Map<string, { total: number; ok: number; failed: number }>();
            llmEntries.forEach((e) => {
                const key = (e.actualProvider !== undefined && e.actualProvider.length > 0 && e.actualProvider !== e.provider)
                    ? `${e.provider}/${e.actualProvider}:${e.model}`
                    : `${e.provider}:${e.model}`;
                const curr = pairStats.get(key) ?? { total: 0, ok: 0, failed: 0 };
                curr.total += 1;
                if (e.status === 'failed')
                    curr.failed += 1;
                else
                    curr.ok += 1;
                pairStats.set(key, curr);
            });
            const pairsStr = [...pairStats.entries()]
                .map(([key, s]) => `${String(s.total)}x [${String(s.ok)}+${String(s.failed)}] ${key}`)
                .join(', ');
            const stopReasonStats = new Map<string, number>();
            llmEntries.forEach((entry) => {
                const reason = entry.stopReason;
                if (typeof reason === 'string' && reason.length > 0) {
                    stopReasonStats.set(reason, (stopReasonStats.get(reason) ?? 0) + 1);
                }
            });
            const stopReasonStr = [...stopReasonStats.entries()]
                .map(([reason, count]) => `${reason}(${String(count)})`)
                .join(', ');
            let msg = `requests=${String(llmRequests)} failed=${String(llmFailures)}, tokens prompt=${String(tokIn)} output=${String(tokOut)} cacheR=${String(tokCacheRead)} cacheW=${String(tokCacheWrite)} total=${String(tokTotal)}, cost total=$${totalCost.toFixed(5)} upstream=$${totalUpstreamCost.toFixed(5)}, latency sum=${String(llmLatencySum)}ms avg=${String(llmLatencyAvg)}ms, providers/models: ${pairsStr.length > 0 ? pairsStr : 'none'}`;
            if (stopReasonStr.length > 0) {
                msg += `, stop reasons: ${stopReasonStr}`;
            }
            const fin = {
                timestamp: Date.now(),
                severity: 'FIN' as const,
                turn: this.state.currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: 'summary',
                fatal: false,
                message: msg,
            };
            this.log(fin);
            // MCP summary
            const toolEntries = accounting.filter((e) => e.type === 'tool');
            const totalToolCharsIn = toolEntries.reduce((s, e) => s + e.charactersIn, 0);
            const totalToolCharsOut = toolEntries.reduce((s, e) => s + e.charactersOut, 0);
            const mcpRequests = toolEntries.length;
            const mcpFailures = toolEntries.filter((e) => e.status === 'failed').length;
            const byToolStats = new Map<string, { total: number; ok: number; failed: number }>();
            toolEntries.forEach((e) => {
                const key = `${e.mcpServer}:${e.command}`;
                const curr = byToolStats.get(key) ?? { total: 0, ok: 0, failed: 0 };
                curr.total += 1;
                if (e.status === 'failed')
                    curr.failed += 1;
                else
                    curr.ok += 1;
                byToolStats.set(key, curr);
            });
            const toolPairsStr = [...byToolStats.entries()]
                .map(([k, s]) => `${String(s.total)}x [${String(s.ok)}+${String(s.failed)}] ${k}`)
                .join(', ');
            const sizeCaps = this.state.centralSizeCapHits;
            const finMcp = {
                timestamp: Date.now(),
                severity: 'FIN' as const,
                turn: this.state.currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'tool' as const,
                remoteIdentifier: 'summary',
                fatal: false,
                message: `requests=${String(mcpRequests)}, failed=${String(mcpFailures)}, capped=${String(sizeCaps)}, bytes in=${String(totalToolCharsIn)} out=${String(totalToolCharsOut)}, providers/tools: ${toolPairsStr.length > 0 ? toolPairsStr : 'none'}`,
            };
            this.log(finMcp);
        }
        catch { /* swallow summary errors */ }
    }
    private composeRemoteIdentifier(provider: string, model: string, metadata?: { actualProvider?: string; actualModel?: string }): string {
        const providerSegment = metadata?.actualProvider !== undefined && metadata.actualProvider.length > 0 && metadata.actualProvider !== provider
            ? `${provider}/${metadata.actualProvider}`
            : provider;
        const modelSegment = metadata?.actualModel ?? model;
        return `${providerSegment}:${modelSegment}`;
    }
    private getFinalReportStatus(): 'success' | 'failure' | undefined {
        const status = this.finalReport?.status;
        if (status === 'success' || status === 'failure') {
            return status;
        }
        return undefined;
    }
    private buildFinalReportReminder(): string {
        const format = this.ctx.resolvedFormat ?? 'text';
        const formatDescription = this.ctx.resolvedFormatParameterDescription ?? 'the required format';
        const contentGuidance = (() => {
            if (format === 'json')
                return CONTENT_GUIDANCE_JSON;
            if (format === SLACK_BLOCK_KIT_FORMAT)
                return CONTENT_GUIDANCE_SLACK;
            return CONTENT_GUIDANCE_TEXT;
        })();
        return finalReportReminder(FINAL_REPORT_TOOL, format, formatDescription, contentGuidance);
    }
    private buildFallbackRetryDirective(input: { status: TurnStatus; remoteId: string; attempt: number }): TurnRetryDirective | undefined {
        const { status, remoteId, attempt } = input;
        if (status.type === 'rate_limit') {
            const wait = typeof status.retryAfterMs === 'number' && Number.isFinite(status.retryAfterMs)
                ? status.retryAfterMs
                : undefined;
            return {
                action: 'retry',
                backoffMs: wait,
                logMessage: `Rate limited; backing off ${wait !== undefined ? `${String(wait)}ms` : 'briefly'} before retry.`,
                systemMessage: `System notice: upstream provider ${remoteId} rate-limited the previous request. Retrying shortly; no changes required unless rate limits persist.`,
                sources: status.sources,
            };
        }
        if (status.type === 'auth_error') {
            return {
                action: RETRY_ACTION_SKIP_PROVIDER,
                logMessage: `Authentication error: ${status.message}. Skipping provider ${remoteId}.`,
                systemMessage: `System notice: authentication failed for ${remoteId}. This provider will be skipped; verify credentials before retrying later.`,
            };
        }
        if (status.type === 'quota_exceeded') {
            return {
                action: RETRY_ACTION_SKIP_PROVIDER,
                logMessage: `Quota exceeded for ${remoteId}; moving to next provider.`,
                systemMessage: `System notice: ${remoteId} exhausted its quota. Switching to the next configured provider.`,
            };
        }
        if (status.type === 'timeout' || status.type === 'network_error') {
            const reason = status.message;
            const prettyType = status.type.replace('_', ' ');
            return {
                action: 'retry',
                backoffMs: Math.min(Math.max((attempt + 1) * 1_000, 1_000), 30_000),
                logMessage: `Transient ${prettyType} encountered on ${remoteId}: ${reason}; retrying shortly.`,
                systemMessage: `System notice: ${remoteId} experienced a transient ${prettyType} (${reason}). Retrying shortly.`,
            };
        }
        return undefined;
    }
    private async sleepWithAbort(ms: number): Promise<'completed' | 'aborted_cancel' | 'aborted_stop'> {
        if (ms <= 0)
            return 'completed';
        if (this.ctx.isCanceled())
            return 'aborted_cancel';
        if (this.ctx.stopRef?.stopping === true)
            return 'aborted_stop';
        return await new Promise((resolve) => {
            let settled = false;
            let timer: ReturnType<typeof setTimeout> | undefined;
            let stopInterval: ReturnType<typeof setInterval> | undefined;
            let abortHandler: (() => void) | undefined;
            const finish = (result: 'completed' | 'aborted_cancel' | 'aborted_stop'): void => {
                if (settled)
                    return;
                settled = true;
                if (timer !== undefined)
                    clearTimeout(timer);
                if (stopInterval !== undefined)
                    clearInterval(stopInterval);
                if (abortHandler !== undefined && this.ctx.abortSignal !== undefined) {
                    try {
                        this.ctx.abortSignal.removeEventListener('abort', abortHandler);
                    }
                    catch { /* noop */ }
                }
                resolve(result);
            };
            timer = setTimeout(() => { finish('completed'); }, ms);
            if (this.ctx.abortSignal !== undefined) {
                abortHandler = () => { finish('aborted_cancel'); };
                this.ctx.abortSignal.addEventListener('abort', abortHandler, { once: true });
            }
            stopInterval = setInterval(() => {
                if (this.ctx.isCanceled()) {
                    finish('aborted_cancel');
                }
                else if (this.ctx.stopRef?.stopping === true) {
                    finish('aborted_stop');
                }
            }, 100);
        });
    }
    private tryAdoptFinalReportFromText(_assistantMessage: ConversationMessage, text: string | undefined): PendingFinalReportPayload | undefined {
        if (typeof text !== 'string')
            return undefined;
        const trimmedText = text.trim();
        if (trimmedText.length === 0)
            return undefined;
        return this.ctx.finalReportManager.tryExtractFromText(trimmedText);
    }

    /**
     * Process leaked tool calls from assistant content.
     * Creates a tool executor, extracts and executes tools, returns results.
     */
    private async processLeakedToolCallsFallback(
        textContent: string,
        turn: number,
        provider: string
    ): Promise<LeakedToolFallbackResult | undefined> {
        // Create execution state for the fallback executor
        const executionState: ToolExecutionState = {
            toolFailureMessages: new Map(),
            toolFailureFallbacks: [],
            trimmedToolCallIds: new Set(),
            turnHasSuccessfulToolOutput: false,
            incompleteFinalReportDetected: false,
        };

        // Create tool executor for fallback execution
        const executor = this.ctx.sessionExecutor.createExecutor(turn, provider, executionState);

        // Delegate to the module
        return processLeakedToolCalls({
            textContent,
            executor,
        });
    }

    private sanitizeTurnMessages(messages: ConversationMessage[], context: { turn: number; provider: string; model: string; stopReason?: string }): { messages: ConversationMessage[]; dropped: number; finalReportAttempted: boolean } {
        let dropped = 0;
        let finalReportAttempted = false;
        const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
        const sanitized = messages.map((msg, _msgIndex) => {
            let rawToolCalls = msg.toolCalls;
            let parsedToolCalls;
            // XML transport parsing
            if (this.ctx.xmlTransport !== undefined && msg.role === 'assistant' && typeof msg.content === 'string') {
                const parseResult = this.ctx.xmlTransport.parseAssistantMessage(msg.content, { turn: context.turn, resolvedFormat: this.ctx.resolvedFormat, stopReason: context.stopReason }, {
                    onTurnFailure: (reason) => { this.addTurnFailure(reason); },
                    onLog: (entry) => {
                        this.callbacks.log({
                            timestamp: Date.now(),
                            severity: entry.severity,
                            turn: context.turn,
                            subturn: 0,
                            direction: 'response' as const,
                            type: 'llm' as const,
                            remoteIdentifier: REMOTE_SANITIZER,
                            fatal: false,
                            message: entry.message,
                        });
                    },
                });
                if (parseResult.toolCalls !== undefined && parseResult.toolCalls.length > 0) {
                    // Merge XML-parsed tool calls with native tool calls (xml-final mode)
                    // Native tool calls execute first, then XML-parsed calls (final report)
                    if (rawToolCalls !== undefined && rawToolCalls.length > 0) {
                        rawToolCalls = [...rawToolCalls, ...parseResult.toolCalls];
                    } else {
                        rawToolCalls = parseResult.toolCalls;
                    }
                }
            }
            if (rawToolCalls !== undefined && Array.isArray(rawToolCalls)) {
                const calls: { name: string; id: string; parameters: Record<string, unknown> }[] = [];
                rawToolCalls.forEach((tc) => {
                    if (!isRecord(tc)) {
                        dropped++;
                        return;
                    }
                    const name = typeof tc.name === 'string' ? tc.name : '';
                    const id = typeof tc.id === 'string' ? tc.id : crypto.randomUUID();
                    // Check if this is a final report attempt BEFORE parameter validation
                    // so we track the attempt even if parameters are invalid
                    const normalizedName = sanitizeToolName(name);
                    const isFinalReportCall = normalizedName === FINAL_REPORT_TOOL || FINAL_REPORT_TOOL_ALIASES.has(normalizedName);
                    if (isFinalReportCall) {
                        finalReportAttempted = true;
                    }
                    let parameters = {};
                    if (isRecord(tc.parameters)) {
                        parameters = tc.parameters;
                    }
                    else if (typeof tc.parameters === 'string') {
                        try {
                            const parsed = parseJsonValueDetailed(tc.parameters);
                            if (parsed.value !== null && isRecord(parsed.value)) {
                                parameters = parsed.value;
                            }
                            else {
                                // JSON parsed but result is not an object (e.g., string, number, array)
                                // Increment attempts for final_report calls that are being dropped
                                if (isFinalReportCall) {
                                    this.ctx.finalReportManager.incrementAttempts();
                                }
                                dropped++;
                                return;
                            }
                        }
                        catch {
                            // Increment attempts for final_report calls that are being dropped
                            if (isFinalReportCall) {
                                this.ctx.finalReportManager.incrementAttempts();
                            }
                            dropped++;
                            return;
                        }
                    }
                    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for untrusted JSON
                    else if (tc.parameters !== undefined) {
                        // Non-string, non-object parameters that aren't undefined are invalid (including null)
                        // Increment attempts for final_report calls that are being dropped
                        if (isFinalReportCall) {
                            this.ctx.finalReportManager.incrementAttempts();
                        }
                        dropped++;
                        return;
                    }
                    calls.push({ name: normalizedName, id, parameters });
                });
                parsedToolCalls = calls;
            }
            const result = { ...msg };
            if (parsedToolCalls !== undefined) {
                result.toolCalls = parsedToolCalls;
            }
            return result;
        });
        return { messages: sanitized, dropped, finalReportAttempted };
    }
    private emitReasoningChunks(chunks: string[]): void {
        chunks.forEach((chunk: string) => {
            try {
                this.callbacks.onThinking?.(chunk);
            }
            catch (e) {
                warn(`onThinking callback failed: ${e instanceof Error ? e.message : String(e)}`);
            }
        });
    }
    private expandNestedLLMToolCalls(toolCalls: ToolCall[]): ToolCall[] | undefined {
        let mutated = false;
        const expanded: ToolCall[] = [];
        // eslint-disable-next-line functional/no-loop-statements
        for (const call of toolCalls) {
            const normalizedName = sanitizeToolName(call.name);
            if (normalizedName === FINAL_REPORT_TOOL) {
                const nested = this.ctx.sessionExecutor.parseNestedCallsFromFinalReport(call.parameters, this.state.currentTurn);
                if (nested !== undefined) {
                    expanded.push(...nested);
                    mutated = true;
                    continue;
                }
            }
            expanded.push(call);
        }
        return mutated ? expanded : undefined;
    }
    private resolveReasoningMapping(provider: string, model: string): ProviderReasoningMapping | undefined {
        const providerConfig = this.ctx.config.providers[provider];
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic provider lookup
        if (providerConfig === undefined)
            return undefined;
        const modelReasoning = providerConfig.models?.[model]?.reasoning;
        if (modelReasoning !== undefined && modelReasoning !== null)
            return modelReasoning;
        if (providerConfig.reasoning !== undefined && providerConfig.reasoning !== null)
            return providerConfig.reasoning;
        return undefined;
    }
    private resolveReasoningValue(provider: string, model: string, level: ReasoningLevel, maxOutputTokens: number | undefined): ProviderReasoningValue | undefined {
        const mapping = this.resolveReasoningMapping(provider, model);
        const result = this.ctx.llmClient.resolveReasoningValue(provider, { level, mapping, maxOutputTokens });
        return result === null ? undefined : result;
    }
    private resolveToolChoice(provider: string, model: string, toolsCount: number): 'auto' | 'required' | undefined {
        const transport = this.ctx.toolTransport;
        if (transport !== 'native') {
            if (toolsCount === 0)
                return undefined;
            return 'auto';
        }
        const providerConfig = this.ctx.config.providers[provider];
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic provider lookup
        if (providerConfig === undefined)
            return undefined;
        const modelChoice = providerConfig.models?.[model]?.toolChoice;
        if (modelChoice === 'auto' || modelChoice === 'required')
            return modelChoice;
        const providerChoice = providerConfig.toolChoice;
        if (providerChoice === 'auto' || providerChoice === 'required')
            return providerChoice;
        return undefined;
    }
    private resolveModelOverrides(provider: string, model: string): Record<string, number | null> {
        const providers = this.ctx.config.providers;
        const providerConfig = Object.prototype.hasOwnProperty.call(providers, provider)
            ? providers[provider]
            : undefined;
        if (providerConfig === undefined)
            return {};
        const modelConfig = providerConfig.models?.[model];
        const overrides = modelConfig?.overrides;
        if (overrides === undefined)
            return {};
        const result: Record<string, number | null> = {};
        const overrideTemperature = overrides.temperature;
        if (overrideTemperature !== undefined) {
            result.temperature = overrideTemperature ?? null;
        }
        const overrideTopPCamel = overrides.topP;
        if (overrideTopPCamel !== undefined) {
            result.topP = overrideTopPCamel ?? null;
        }
        else {
            const overrideTopPSnake = overrides.top_p;
            if (overrideTopPSnake !== undefined) {
                result.topP = overrideTopPSnake ?? null;
            }
        }
        const overrideTopKCamel = overrides.topK;
        if (overrideTopKCamel !== undefined) {
            result.topK = overrideTopKCamel ?? null;
        }
        else {
            const overrideTopKSnake = overrides.top_k;
            if (overrideTopKSnake !== undefined) {
                result.topK = overrideTopKSnake ?? null;
            }
        }
        return result;
    }
    // executeSingleTurn delegates to internal implementation
    private async executeSingleTurn(
        conversation: ConversationMessage[],
        provider: string,
        model: string,
        isFinalTurn: boolean,
        currentTurn: number,
        logs: LogEntry[],
        accounting: AccountingEntry[],
        lastShownThinkingHeaderTurn: number,
        attempt: number,
        maxAttempts: number,
        reasoningLevel: ReasoningLevel | undefined,
        reasoningValue: ProviderReasoningValue | undefined
    ): Promise<TurnResult & { shownThinking: boolean; incompleteFinalReportDetected: boolean }> {
        const attributes: Record<string, string | number | boolean> = {
            'ai.llm.provider': provider,
            'ai.llm.model': model,
            'ai.llm.is_final_turn': isFinalTurn,
        };
        if (typeof reasoningLevel === 'string' && reasoningLevel.length > 0) {
            attributes['ai.llm.reasoning.level'] = reasoningLevel;
        }
        if (typeof reasoningValue === 'number') {
            attributes['ai.llm.reasoning.value'] = reasoningValue;
        }
        return await runWithSpan('agent.llm.turn', { attributes }, () => this.executeSingleTurnInternal(conversation, provider, model, isFinalTurn, currentTurn, logs, accounting, lastShownThinkingHeaderTurn, attempt, maxAttempts, reasoningLevel, reasoningValue));
    }
    private async executeSingleTurnInternal(
        conversation: ConversationMessage[],
        provider: string,
        model: string,
        isFinalTurn: boolean,
        currentTurn: number,
        _logs: LogEntry[],
        _accounting: AccountingEntry[],
        lastShownThinkingHeaderTurn: number,
        attempt: number,
        maxAttempts: number,
        reasoningLevel: ReasoningLevel | undefined,
        reasoningValue: ProviderReasoningValue | undefined
    ): Promise<TurnResult & { shownThinking: boolean; incompleteFinalReportDetected: boolean }> {
        const pendingSelection = this.state.pendingToolSelection;
        this.state.pendingToolSelection = undefined;
        const selection = pendingSelection ?? this.selectToolsForTurn(provider, isFinalTurn);
        const toolsForTurn = selection.toolsForTurn;
        let disableReasoningForTurn = false;
        this.schemaCtxTokens = this.ctx.toolTransport === 'native' ? this.estimateToolSchemaTokens(toolsForTurn) : 0;
        const reasoningGuard = this.ctx.llmClient.shouldDisableReasoning(provider, {
            conversation,
            currentTurn,
            expectSignature: true,
        });
        if (reasoningGuard.disable) {
            conversation = reasoningGuard.normalized;
            disableReasoningForTurn = true;
        }
        addSpanAttributes({ 'ai.llm.reasoning.disabled': disableReasoningForTurn });
        let shownThinking = false;
        this.state.toolFailureMessages.clear();
        this.state.toolFailureFallbacks.length = 0;
        const executionState: ToolExecutionState = {
            toolFailureMessages: this.state.toolFailureMessages,
            toolFailureFallbacks: this.state.toolFailureFallbacks,
            trimmedToolCallIds: this.state.trimmedToolCallIds,
            turnHasSuccessfulToolOutput: false,
            incompleteFinalReportDetected: false
        };
        const baseExecutor = this.ctx.sessionExecutor.createExecutor(currentTurn, provider, executionState, selection.allowedToolNames);
        let incompleteFinalReportDetected = false;
        const toolExecutor = async (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }): Promise<string> => {
            try {
                const result = await baseExecutor(toolName, parameters, options);
                incompleteFinalReportDetected = executionState.incompleteFinalReportDetected;
                return result;
            }
            catch (e) {
                incompleteFinalReportDetected = executionState.incompleteFinalReportDetected;
                throw e;
            }
        };
        const modelOverrides = this.resolveModelOverrides(provider, model);
        let effectiveTemperature = this.ctx.sessionConfig.temperature;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic key lookup
        if (modelOverrides.temperature !== undefined) {
            if (modelOverrides.temperature === null)
                effectiveTemperature = undefined;
            else
                effectiveTemperature = modelOverrides.temperature;
        }
        let effectiveTopP = this.ctx.sessionConfig.topP;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic key lookup
        if (modelOverrides.topP !== undefined) {
            if (modelOverrides.topP === null)
                effectiveTopP = undefined;
            else
                effectiveTopP = modelOverrides.topP;
        }
        let effectiveTopK = this.ctx.sessionConfig.topK;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic key lookup
        if (modelOverrides.topK !== undefined) {
            if (modelOverrides.topK === null)
                effectiveTopK = undefined;
            else
                effectiveTopK = modelOverrides.topK;
        }
        const fallbackReasoningLevel = this.ctx.sessionConfig.reasoning;
        let effectiveReasoningLevel = reasoningLevel ?? fallbackReasoningLevel;
        const targetMaxOutputTokens = this.ctx.sessionConfig.maxOutputTokens;
        let effectiveReasoningValue = reasoningValue ?? (effectiveReasoningLevel !== undefined
            ? this.resolveReasoningValue(provider, model, effectiveReasoningLevel, targetMaxOutputTokens)
            : undefined);
        const reasoningActive = effectiveReasoningLevel !== undefined || effectiveReasoningValue !== undefined;
        if (disableReasoningForTurn) {
            if (reasoningActive) {
                const warnEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: 'agent:reasoning',
                    fatal: false,
                    message: 'Anthropic reasoning disabled for this turn: previous assistant tool call lacked signature metadata.',
                };
                this.log(warnEntry);
            }
            effectiveReasoningLevel = undefined;
            effectiveReasoningValue = undefined;
        }
        addSpanAttributes({
            'ai.llm.reasoning.level_effective': effectiveReasoningLevel ?? 'none',
            'ai.llm.reasoning.value_effective': effectiveReasoningValue ?? 'none',
        });
        const sendReasoning = disableReasoningForTurn ? false : undefined;
        const sessionStreamFlag = this.ctx.sessionConfig.stream === true;
        const autoEnableReasoningStream = this.ctx.llmClient.shouldAutoEnableReasoningStream(provider, effectiveReasoningLevel, {
            maxOutputTokens: targetMaxOutputTokens,
            reasoningActive,
            streamRequested: sessionStreamFlag,
        });
        let effectiveStream = sessionStreamFlag;
        if (!effectiveStream && autoEnableReasoningStream) {
            effectiveStream = true;
            const warnEntry = {
                timestamp: Date.now(),
                severity: 'WRN' as const,
                turn: currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: 'agent:reasoning',
                fatal: false,
                message: `Provider '${provider}' requires streaming for the current reasoning request; overriding stream=true.`,
            };
            this.log(warnEntry);
        }
        const requestMessages = this.mergePendingRetryMessages(conversation);
        const requestMessagesBytes = estimateMessagesBytes(requestMessages);
        this.state.pendingRetryMessages = [];
        const metricsForRequest = this.ctx.contextGuard.buildMetrics(provider, model);
        const turnMetadata: {
            attempt: number;
            maxAttempts: number;
            turn: number;
            isFinalTurn: boolean;
            reasoningLevel?: ReasoningLevel;
            reasoningValue?: ProviderReasoningValue | null;
        } = {
            attempt,
            maxAttempts,
            turn: currentTurn,
            isFinalTurn,
        };
        if (effectiveReasoningLevel !== undefined) {
            turnMetadata.reasoningLevel = effectiveReasoningLevel;
        }
        if (effectiveReasoningValue !== undefined) {
            turnMetadata.reasoningValue = effectiveReasoningValue;
        }
        const llmTools = (() => {
            if (this.ctx.toolTransport === 'native')
                return toolsForTurn;
            if (this.ctx.toolTransport === 'xml-final') {
                // In xml-final: final_report via XML, all other tools (including progress) stay native
                return toolsForTurn.filter((tool) => {
                    const name = sanitizeToolName(tool.name);
                    return name !== FINAL_REPORT_TOOL;
                });
            }
            return [];
        })();
        let xmlFilter: XmlFinalReportFilter | undefined;
        if (this.ctx.xmlTransport !== undefined && (this.ctx.toolTransport === 'xml-final' || this.ctx.toolTransport === 'xml')) {
            const nonce = this.ctx.xmlTransport.getNonce();
            if (nonce !== undefined) {
                xmlFilter = new XmlFinalReportFilter(nonce);
            }
        }
        // Reset streaming flag for this attempt
        this.finalReportStreamed = false;

        const request = {
            messages: requestMessages,
            provider,
            model,
            tools: llmTools,
            toolExecutor,
            temperature: effectiveTemperature,
            topP: effectiveTopP,
            topK: effectiveTopK,
            maxOutputTokens: targetMaxOutputTokens,
            repeatPenalty: this.ctx.sessionConfig.repeatPenalty,
            stream: effectiveStream,
            isFinalTurn,
            llmTimeout: this.ctx.sessionConfig.llmTimeout,
            toolChoice: this.resolveToolChoice(provider, model, llmTools.length),
            abortSignal: this.ctx.abortSignal,
            sendReasoning,
            onChunk: (chunk: string, type: 'content' | 'thinking') => {
                const isRootSession = this.ctx.parentTxnId === undefined;
                if (type === 'content' && this.callbacks.onOutput !== undefined) {
                    if (xmlFilter !== undefined) {
                        const filtered = xmlFilter.process(chunk);
                        if (filtered.length > 0) {
                            this.callbacks.onOutput(filtered);
                            // Only mark as streamed if we actually emitted content (and are inside the tag)
                            if (xmlFilter.hasStreamedContent) {
                                this.finalReportStreamed = true;
                            }
                        }
                    } else {
                        this.callbacks.onOutput(chunk);
                    }
                }
                else if (type === 'thinking') {
                    const trimmedThinking = chunk.trim();
                    if (isRootSession && trimmedThinking.length > 0) {
                        this.ctx.opTree.setLatestStatus(trimmedThinking);
                    }
                    if (!shownThinking && lastShownThinkingHeaderTurn !== currentTurn) {
                        const thinkingHeader = {
                            timestamp: Date.now(),
                            severity: 'THK' as const,
                            turn: currentTurn,
                            subturn: 0,
                            direction: 'response' as const,
                            type: 'llm' as const,
                            remoteIdentifier: 'thinking',
                            fatal: false,
                            message: 'reasoning output stream'
                        };
                        this.log(thinkingHeader);
                        shownThinking = true;
                    }
                    if (isRootSession) {
                        this.callbacks.onThinking?.(chunk);
                    }
                    try {
                        const opId = this.callbacks.getCurrentLlmOpId();
                        if (typeof opId === 'string')
                            this.ctx.opTree.appendReasoningChunk(opId, chunk);
                    }
                    catch (e) {
                        warn(`appendReasoningChunk failed: ${e instanceof Error ? e.message : String(e)}`);
                    }
                }
            },
            reasoningLevel: effectiveReasoningLevel,
            reasoningValue: effectiveReasoningValue ?? null,
            turnMetadata,
            caching: this.ctx.sessionConfig.caching,
            contextMetrics: metricsForRequest,
        };
        this.debugLogRawConversation(provider, model, conversation);
        try {
            const result = await this.ctx.llmClient.executeTurn(request);
            const tokens = result.tokens;
            const promptTokens = tokens?.inputTokens ?? 0;
            const completionTokens = tokens?.outputTokens ?? 0;
            const cacheReadTokens = tokens?.cacheReadInputTokens ?? tokens?.cachedTokens ?? 0;
            const cacheWriteTokens = tokens?.cacheWriteInputTokens ?? 0;
            const responseBytes = typeof result.response === 'string' && result.response.length > 0
                ? Buffer.byteLength(result.response, 'utf8')
                : estimateMessagesBytes(result.messages);
            const costInfo = this.ctx.llmClient.getLastCostInfo();
            const attempts = request.turnMetadata.attempt;
            const retries = attempts > 1 ? attempts - 1 : 0;
            recordLlmMetrics({
                agentId: this.ctx.agentId,
                callPath: this.ctx.callPath,
                headendId: this.ctx.headendId,
                provider,
                model,
                status: result.status.type === 'success' ? 'success' : 'error',
                errorType: result.status.type === 'success' ? undefined : result.status.type,
                latencyMs: result.latencyMs,
                promptTokens,
                completionTokens,
                cacheReadTokens,
                cacheWriteTokens,
                requestBytes: requestMessagesBytes,
                responseBytes,
                retries,
                costUsd: costInfo?.costUsd,
                reasoningLevel: effectiveReasoningLevel ?? this.ctx.sessionConfig.reasoning,
                customLabels: this.ctx.telemetryLabels,
            });
            addSpanAttributes({
                'ai.llm.latency_ms': result.latencyMs,
                'ai.llm.prompt_tokens': promptTokens,
                'ai.llm.completion_tokens': completionTokens,
                'ai.llm.cache_read_tokens': cacheReadTokens,
                'ai.llm.cache_write_tokens': cacheWriteTokens,
                'ai.llm.retry_count': retries,
                'ai.llm.request_bytes': requestMessagesBytes,
                'ai.llm.response_bytes': responseBytes,
            });
            if (typeof result.stopReason === 'string' && result.stopReason.length > 0) {
                addSpanAttributes({ 'ai.llm.stop_reason': result.stopReason });
            }
            if (result.status.type !== 'success') {
                addSpanAttributes({ 'ai.llm.status': result.status.type });
            }
            return { ...result, shownThinking, incompleteFinalReportDetected };
        }
        catch (e) {
            throw e;
        }
        finally {
            if (xmlFilter !== undefined && this.callbacks.onOutput !== undefined) {
                const left = xmlFilter.flush();
                if (left.length > 0) {
                    this.callbacks.onOutput(left);
                    if (xmlFilter.hasStreamedContent) {
                        this.finalReportStreamed = true;
                    }
                }
            }
        }
    }
    mergePendingRetryMessages(conversation: ConversationMessage[]): ConversationMessage[] {
        const cleaned = conversation.filter((msg: ConversationMessage) => msg.metadata?.retryMessage === undefined);
        if (cleaned.length !== conversation.length) {
            conversation.splice(0, conversation.length, ...cleaned);
        }
        if (this.state.pendingRetryMessages.length === 0) {
            return conversation;
        }
        const retryMessages = this.state.pendingRetryMessages.map((text: string) => ({ role: 'user' as const, content: text }));
        return [...conversation, ...retryMessages];
    }
    debugLogRawConversation(provider: string, model: string, payload: ConversationMessage[]): void {
        if (process.env.DEBUG !== 'true')
            return;
        try {
            warn(`[DEBUG] ${provider}:${model} raw_messages: ${JSON.stringify(payload, null, 2)}`);
        }
        catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            warn(`[DEBUG] ${provider}:${model} raw_messages: <unserializable: ${message}>`);
        }
    }
}
//# sourceMappingURL=session-turn-runner.js.map