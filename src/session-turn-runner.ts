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

import type { OutputFormatId } from './formats.js';
import type { LLMClient } from './llm-client.js';
import type { SessionProgressReporter } from './session-progress-reporter.js';
import type { SessionNode, SessionTreeBuilder } from './session-tree.js';
import type { SubAgentRegistry } from './subagent-registry.js';
import type { ToolsOrchestrator } from './tools/tools.js';
import type { AIAgentResult, AIAgentSessionConfig, AccountingEntry, CallbackMeta, Configuration, ConversationMessage, LogDetailValue, LogEntry, MCPTool, ProviderReasoningMapping, ProviderReasoningValue, ReasoningLevel, ToolCall, TurnResult, TurnRetryDirective, TurnStatus } from './types.js';

import { ContextGuard, type ContextGuardBlockedEntry, type ContextGuardEvaluation } from './context-guard.js';
import { FINAL_REPORT_FORMAT_VALUES, type FinalReportManager, type FinalReportSource, type PendingFinalReportPayload } from './final-report-manager.js';
import { buildTurnFailedNotice, type TurnFailedNoticeEvent, type TurnFailedSlug } from './llm-messages-turn-failed.js';
import { buildXmlNextEvents, buildXmlNextNotice, type XmlNextNoticeEvent } from './llm-messages-xml-next.js';
import {
  FINAL_REPORT_JSON_REQUIRED,
  FINAL_REPORT_SLACK_MESSAGES_MISSING,
  buildSchemaMismatchFailure,
  formatJsonParseHint,
  formatSchemaMismatchSummary,
  isUnknownToolFailureMessage,
  isXmlFinalReportTagName,
  TOOL_NO_OUTPUT,
} from './llm-messages.js';
import {
  TOOL_SANITIZATION_FAILED_KEY,
  TOOL_SANITIZATION_ORIGINAL_PAYLOAD_KEY,
  TOOL_SANITIZATION_ORIGINAL_PAYLOAD_SHA256_KEY,
  TOOL_SANITIZATION_ORIGINAL_PAYLOAD_TRUNCATED_KEY,
  TOOL_SANITIZATION_REASON_KEY,
  type SessionToolExecutor,
  type ToolExecutionState,
} from './session-tool-executor.js';
import { normalizeSlackMessages, parseSlackBlockKitPayload } from './slack-block-kit.js';
import { addSpanAttributes, addSpanEvent, recordContextGuardMetrics, recordFinalReportMetrics, recordLlmMetrics, recordRetryCollapseMetrics, runWithSpan } from './telemetry/index.js';
import { ThinkTagStreamFilter } from './think-tag-filter.js';
import { processLeakedToolCalls, type LeakedToolFallbackResult } from './tool-call-fallback.js';
import { TRUNCATE_PREVIEW_BYTES, truncateToBytes } from './truncation.js';
import { estimateMessagesBytes, formatToolRequestCompact, parseJsonValueDetailed, sanitizeToolName, warn } from './utils.js';
import { XmlFinalReportFilter, type XmlToolTransport } from './xml-transport.js';

const TOOL_FAILED_PREFIX = '(tool failed:';
const TEXT_EXTRACTION_REMOTE_ID = 'agent:text-extraction';
const FINAL_REPORT_SOURCE_TOOL_MESSAGE: FinalReportSource = 'tool-message';

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
  readonly xmlTransport: XmlToolTransport;
  readonly subAgents?: SubAgentRegistry;
  readonly resolvedFormat?: OutputFormatId;
  readonly resolvedFormatPromptValue?: string;
  readonly resolvedFormatParameterDescription?: string;
  readonly resolvedUserPrompt?: string;
  readonly resolvedSystemPrompt?: string;
  readonly expectedJsonSchema?: Record<string, unknown>;
  readonly taskStatusToolEnabled: boolean;
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
  onOutput?: (chunk: string, meta?: CallbackMeta) => void;
  onThinking?: (chunk: string, meta?: CallbackMeta) => void;
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
  turnFailedEvents: TurnFailedNoticeEvent[];
  turnFailedCounter: number;
  xmlNextEvents: XmlNextNoticeEvent[];
  xmlNextCounter: number;
  toolFailureMessages: Map<string, string>;
  toolFailureFallbacks: string[];
  toolNameCorrections: Map<string, string>;
  trimmedToolCallIds: Set<string>;
  executedToolCallIds: Set<string>;
  finalReportToolFailedEver: boolean;
  finalReportToolFailedThisTurn: boolean;
  toolLimitExceeded: boolean;
  droppedInvalidToolCalls: number;
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
  lastFinalReportStatus?: string;
  finalReportInvalidFormat: boolean;
  finalReportSchemaFailed: boolean;
  pendingToolSelection?: { availableTools: MCPTool[]; allowedToolNames: Set<string>; toolsForTurn: MCPTool[] };
  currentCtxTokens?: number;
  pendingCtxTokens?: number;
  newCtxTokens?: number;
  schemaCtxTokens?: number;
  forcedFinalTurnReason?: 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion';
  lastTurnError?: string;
  lastTurnErrorType?: string;
  lastTaskStatusCompleted?: boolean;
  consecutiveProgressOnlyTurns?: number;
}

type FinalTurnLogReason =
    | 'context'
    | 'max_turns'
    | 'task_status_completed'
    | 'task_status_only'
    | 'retry_exhaustion'
    | 'incomplete_final_report'
    | 'final_report_attempt'
    | 'xml_wrapper_as_tool';

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
const STREAMED_OUTPUT_DEDUPE_MAX_CHARS = 200_000;

export { FINAL_REPORT_TOOL };
/**
 * TurnRunner encapsulates all turn iteration logic
 */
export class TurnRunner {
    private readonly ctx: TurnRunnerContext;
    private readonly callbacks: TurnRunnerCallbacks;
    private state: TurnRunnerState;
    private finalReportStreamed = false;
    private streamedOutputTail = '';

    constructor(ctx: TurnRunnerContext, callbacks: TurnRunnerCallbacks) {
        this.ctx = ctx;
        this.callbacks = callbacks;
        this.state = {
            currentTurn: 0,
            maxTurns: ctx.sessionConfig.maxTurns ?? 10,
            conversation: [],
            logs: [],
            accounting: [],
            turnFailedEvents: [],
            turnFailedCounter: 0,
            xmlNextEvents: [],
            xmlNextCounter: 0,
            toolFailureMessages: new Map<string, string>(),
            toolFailureFallbacks: [],
            toolNameCorrections: new Map<string, string>(),
            trimmedToolCallIds: new Set<string>(),
            executedToolCallIds: new Set<string>(),
            finalReportToolFailedEver: false,
            finalReportToolFailedThisTurn: false,
            toolLimitExceeded: false,
            droppedInvalidToolCalls: 0,
            plannedSubturns: new Map<number, number>(),
            finalTurnEntryLogged: false,
            childConversations: [],
            centralSizeCapHits: 0,
            llmAttempts: 0,
            llmSyntheticFailures: 0,
            lastFinalReportStatus: undefined,
            finalReportInvalidFormat: false,
            finalReportSchemaFailed: false,
            lastTaskStatusCompleted: undefined,
            consecutiveProgressOnlyTurns: 0,
        };
    }
    private resetStreamedOutputTail(): void {
        this.streamedOutputTail = '';
    }
    private appendStreamedOutputTail(chunk: string): void {
        if (chunk.length === 0)
            return;
        const next = this.streamedOutputTail + chunk;
        if (next.length <= STREAMED_OUTPUT_DEDUPE_MAX_CHARS) {
            this.streamedOutputTail = next;
            return;
        }
        this.streamedOutputTail = next.slice(-STREAMED_OUTPUT_DEDUPE_MAX_CHARS);
    }
    private hasStreamedFinalOutput(finalOutput: string): boolean {
        const candidate = finalOutput.trimEnd();
        if (candidate.length === 0)
            return false;
        const streamed = this.streamedOutputTail.trimEnd();
        if (streamed.length === 0)
            return false;
        return streamed.endsWith(candidate);
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
        this.state.finalReportInvalidFormat = false;
        this.state.finalReportSchemaFailed = false;
        // Use the passed childConversations array reference if provided, otherwise use the empty one from state
        if (childConversations !== undefined) {
            this.state.childConversations = childConversations;
        }
        // Use the passed plannedSubturns map reference if provided, so AIAgentSession.addLog can enrich logs
        if (plannedSubturns !== undefined) {
            this.state.plannedSubturns = plannedSubturns;
        }
        this.state.finalReportToolFailedEver = false;
        this.state.finalReportToolFailedThisTurn = false;
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
            this.ctx.xmlTransport.beginTurn();
            this.state.turnFailedEvents = [];
            this.state.turnFailedCounter = 0;
            this.state.xmlNextEvents = [];
            this.state.xmlNextCounter = 0;
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
            let lastError: string | undefined;
            let lastErrorType: string | undefined;
        let turnSuccessful = false;
        let lastTurnResult: (TurnResult & { shownThinking?: boolean; incompleteFinalReportDetected?: boolean }) | undefined;
            let turnHadFinalReportAttempt = false;
            let collapseLoggedThisTurn = false;
            const collapseRemainingTurns = (reason: 'incomplete_final_report' | 'final_report_attempt' | 'xml_wrapper_as_tool'): void => {
                if (collapseLoggedThisTurn)
                    return;
                if (currentTurn >= maxTurns)
                    return;
                const previousMaxTurns = maxTurns;
                maxTurns = currentTurn + 1;
                const collapseMessage = (() => {
                    switch (reason) {
                        case 'incomplete_final_report':
                            return `Incomplete final report detected at turn ${String(currentTurn)}; Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`;
                        case 'final_report_attempt':
                            return `Final report retry detected at turn ${String(currentTurn)}; Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`;
                        case 'xml_wrapper_as_tool':
                            return `XML wrapper called as tool at turn ${String(currentTurn)}; Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`;
                        default:
                            return `Collapsing remaining turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`;
                    }
                })();
                const adjustLog: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: REMOTE_ORCHESTRATOR,
                    fatal: false,
                    message: collapseMessage
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
                this.logEnteringFinalTurn(reason, currentTurn, this.state.maxTurns, maxTurns);
            };
            // Global attempts across all provider/model pairs for this turn
            let attempts = 0;
            let pairCursor = 0;
            let rateLimitedInCycle = 0;
            let maxRateLimitWaitMs = 0;
            this.state.finalReportToolFailedThisTurn = false;
            // turnSuccessful starts false; checked on subsequent iterations after potential success at line ~1184
            // eslint-disable-next-line functional/no-loop-statements
            while (attempts < maxRetries && !turnSuccessful) {
                // Reset per-attempt error state to avoid poisoning later attempts
                // (e.g., validation failure on attempt 1 should not block success on attempt 2)
                lastError = undefined;
                lastErrorType = undefined;
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
                const { provider, model } = pair;
                const logAttemptFailure = (): void => {
                    const failureInfo = this.buildTurnFailureInfo({
                        turn: currentTurn,
                        provider,
                        model,
                        lastError: typeof lastError === 'string' ? lastError : (lastErrorType ?? 'unknown'),
                        lastErrorType,
                        lastTurnResult,
                        attempts,
                        maxRetries,
                        maxTurns,
                        isFinalTurn: currentTurn === maxTurns,
                    });
                    this.logTurnFailure(failureInfo);
                };
                // Note: cycleIndex/cycleComplete tracking removed during extraction - may need to be reinstated
                let forcedFinalTurn = this.forcedFinalTurnReason !== undefined;
                let isFinalTurn = forcedFinalTurn || currentTurn === maxTurns;
                let deferTaskStatusOnlyFinal = false;
                const syncFinalTurnFlags = () => {
                    if (!forcedFinalTurn && this.forcedFinalTurnReason !== undefined) {
                        if (this.forcedFinalTurnReason === 'task_status_only' && deferTaskStatusOnlyFinal) {
                            return;
                        }
                        forcedFinalTurn = true;
                        isFinalTurn = true;
                    }
                };


                const toolSelection = this.selectToolsForTurn(provider, isFinalTurn);
                this.state.pendingToolSelection = toolSelection;
                this.schemaCtxTokens = this.estimateToolSchemaTokens(toolSelection.toolsForTurn);
                const providerContextStatus = this.evaluateContextForProvider(provider, model);
                if (providerContextStatus === 'final') {
                    const evaluation = this.evaluateContextGuard();
                    const blockedEntry = evaluation.blocked.find((entry) => entry.provider === provider && entry.model === model) ?? evaluation.blocked[0];
                    const enforceDueToBaseOverflow = (blockedEntry.projected - this.schemaCtxTokens) > blockedEntry.limit;
                    if (enforceDueToBaseOverflow) {
                        this.enforceContextFinalTurn(evaluation.blocked, 'turn_preflight');
                        syncFinalTurnFlags();
                        const finalToolSelection = this.selectToolsForTurn(provider, true);
                        this.state.pendingToolSelection = finalToolSelection;
                        this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
                        const postEnforce = this.evaluateContextGuard();
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
                                message: 'Context limit exceeded; forcing final turn.',
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
                }
                if (providerContextStatus === 'skip') {
                    const evaluation = this.evaluateContextGuard();
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
                    this.flushTurnFailureReasons(conversation);
                    let attemptConversation = [...conversation];
                    const allTools = this.ctx.toolsOrchestrator?.listTools() ?? [];
                    // Calculate context window usage percentage
                    const ctxGuard = this.ctx.contextGuard;
                    const currentTokens = ctxGuard.getCurrentTokens();
                    const pendingTokens = ctxGuard.getPendingTokens();
                    const newTokens = ctxGuard.getNewTokens();
                    const contextWindow = ctxGuard.getTargets()[0]?.contextWindow ?? ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS;
                    const contextPercentUsed = Math.min(100, Math.round((currentTokens + pendingTokens + newTokens) * 100 / contextWindow));

                    const xmlNextForcedReason = isFinalTurn
                        ? (this.forcedFinalTurnReason ?? 'max_turns')
                        : undefined;
                    const { events: xmlNextEvents, nextOrder } = buildXmlNextEvents({
                        nonce: this.ctx.xmlTransport.getSessionNonce(),
                        turn: currentTurn,
                        maxTurns,
                        attempt: attempts + 1,
                        maxRetries,
                        contextPercentUsed,
                        expectedFinalFormat: this.ctx.resolvedFormat ?? 'markdown',
                        hasExternalTools: allTools.some((tool) => tool.name !== FINAL_REPORT_TOOL),
                        taskStatusToolEnabled: this.ctx.taskStatusToolEnabled,
                        forcedFinalTurnReason: xmlNextForcedReason,
                    }, this.state.xmlNextCounter);
                    this.state.xmlNextEvents = xmlNextEvents;
                    this.state.xmlNextCounter = nextOrder;
                    const xmlNextContent = buildXmlNextNotice(xmlNextEvents);
                    const xmlResult = this.ctx.xmlTransport.buildMessages({
                        turn: currentTurn,
                        maxTurns,
                        tools: allTools,
                        maxToolCallsPerTurn: Math.max(1, this.ctx.sessionConfig.maxToolCallsPerTurn ?? 10),
                        taskStatusToolEnabled: this.ctx.taskStatusToolEnabled,
                        finalReportToolName: FINAL_REPORT_TOOL,
                        resolvedFormat: this.ctx.resolvedFormat,
                        expectedJsonSchema: this.ctx.expectedJsonSchema,
                        attempt: attempts + 1,  // attempts is 0-based, but we want 1-based for display
                        maxRetries,
                        contextPercentUsed,
                        forcedFinalTurnReason: xmlNextForcedReason,
                        noticeContent: xmlNextContent,
                    });
                    if (xmlResult.pastMessage !== undefined)
                        attemptConversation.push(xmlResult.pastMessage);
                    attemptConversation.push(xmlResult.nextMessage);
                    if (isFinalTurn) {
                        let logReason: FinalTurnLogReason = 'max_turns';
                        if (this.forcedFinalTurnReason === 'context') {
                            logReason = 'context';
                        } else if (this.forcedFinalTurnReason === 'task_status_completed') {
                            logReason = 'task_status_completed';
                        } else if (this.forcedFinalTurnReason === 'task_status_only') {
                            logReason = 'task_status_only';
                        } else if (this.forcedFinalTurnReason === 'retry_exhaustion') {
                            logReason = 'retry_exhaustion';
                        }
                        this.logEnteringFinalTurn(logReason, currentTurn, this.state.maxTurns, maxTurns);
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
                const guardEvaluation = this.evaluateContextGuard();
                if (guardEvaluation.blocked.length > 0) {
                    const blocked = guardEvaluation.blocked.find((entry) => entry.provider === provider && entry.model === model) ?? guardEvaluation.blocked[0];
                    const baseProjected = blocked.projected - this.schemaCtxTokens;
                    const enforceDueToBaseOverflow = baseProjected > blocked.limit;
                    if (enforceDueToBaseOverflow) {
                        this.enforceContextFinalTurn(guardEvaluation.blocked, 'turn_preflight');
                        syncFinalTurnFlags();
                        const finalToolSelection = this.selectToolsForTurn(provider, true);
                        this.state.pendingToolSelection = finalToolSelection;
                        this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
                        const postEnforceGuard = this.evaluateContextGuard();
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
                                message: 'Context limit exceeded during turn execution; proceeding with final turn.',
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
                }
                    const turnResult = await this.executeSingleTurn(attemptConversation, provider, model, isFinalTurn, currentTurn, logs, accounting, lastShownThinkingHeaderTurn, attempts, maxRetries, sessionReasoningLevel, sessionReasoningValue ?? undefined);
                    lastTurnResult = turnResult;
                    const { messages: sanitizedMessages, dropped: droppedInvalidToolCalls, finalReportAttempted, syntheticToolMessages } = this.sanitizeTurnMessages(turnResult.messages, { turn: currentTurn, provider, model, stopReason: turnResult.stopReason, maxOutputTokens: this.ctx.sessionConfig.maxOutputTokens });
                    this.state.droppedInvalidToolCalls = droppedInvalidToolCalls;
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
                    if (syntheticToolMessages.length > 0) {
                        const existingToolResults = new Set<string>();
                        sanitizedMessages.forEach((message) => {
                            if (message.role !== 'tool')
                                return;
                            const callId = message.toolCallId;
                            if (typeof callId === 'string' && callId.length > 0) {
                                existingToolResults.add(callId);
                            }
                        });
                        syntheticToolMessages.forEach((message) => {
                            const callId = message.toolCallId;
                            if (typeof callId !== 'string' || callId.length === 0)
                                return;
                            if (existingToolResults.has(callId))
                                return;
                            const override = this.state.toolFailureMessages.get(callId);
                            if (override !== undefined) {
                                message.content = override;
                                this.state.toolFailureMessages.delete(callId);
                            }
                            sanitizedMessages.push(message);
                            existingToolResults.add(callId);
                        });
                    }
                    this.applyToolNameCorrections(sanitizedMessages);
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
                    // Emit WRN for unknown tool calls and detect XML wrapper misuse
                    let xmlWrapperCalledAsTool = false;
                    const sessionNonce = this.ctx.xmlTransport.getSessionNonce();
                    try {
                        const internal = new Set([FINAL_REPORT_TOOL]);
                        if (this.ctx.taskStatusToolEnabled)
                            internal.add('agent__task_status');
                        if (this.ctx.sessionConfig.tools.includes('batch'))
                            internal.add('agent__batch');
                        const normalizeTool = (n: string): string => n.replace(/^<\|[^|]+\|>/, '').trim();
                        const assistantMessages = sanitizedMessages.filter((m) => m.role === 'assistant');
                        const assistantMsg = assistantMessages.length > 0 ? assistantMessages[assistantMessages.length - 1] : undefined;
                        if (assistantMsg?.toolCalls !== undefined && assistantMsg.toolCalls.length > 0) {
                            const calls = assistantMsg.toolCalls;
                            xmlWrapperCalledAsTool = calls.some((tc) => isXmlFinalReportTagName(normalizeTool(tc.name)));
                            calls.forEach((tc) => {
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
                    if (xmlWrapperCalledAsTool) {
                        const formatValue = this.ctx.resolvedFormat ?? 'markdown';
                        const wrapperTag = `ai-agent-${sessionNonce}-FINAL`;
                        const reason = `wrapper: <${wrapperTag} format="${formatValue}">`;
                        this.addTurnFailure('xml_wrapper_as_tool', reason);
                        collapseRemainingTurns('xml_wrapper_as_tool');
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
                        // Update context guard with actual response tokens - only on success
                        // On errors (especially model_error with no output), tokens may be 0
                        // and we should NOT wipe out the existing context tracking
                        if (turnResult.status.type === 'success') {
                            const cacheRead = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                            const cacheWrite = tokens.cacheWriteInputTokens ?? 0;
                            // Full context = inputTokens + cacheRead + cacheWrite + outputTokens
                            // This is the total context consumed by this conversation so far
                            // Note: schema may be in cache_write (first turn) or cache_read (subsequent)
                            // so we track total, not trying to separate messages from schema
                            const fullInputTokens = tokens.inputTokens + cacheRead + cacheWrite;
                            // outputTokens already includes reasoning tokens (they're part of output)
                            this.currentCtxTokens = fullInputTokens + tokens.outputTokens;
                            this.pendingCtxTokens = 0;
                        }
                    }
                    // Close hierarchical LLM op
                    try {
                        if (llmOpId !== undefined) {
                            const lastAssistant = [...sanitizedMessages].filter((m) => m.role === 'assistant').pop();
                            const respText = typeof lastAssistant?.content === 'string' ? lastAssistant.content : (turnResult.response ?? '');
                            const sz = respText.length > 0 ? Buffer.byteLength(respText, 'utf8') : 0;
                            this.ctx.opTree.setResponse(llmOpId, { payload: { textPreview: respText }, size: sz, truncated: false });
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
                                if (name === 'agent__task_status' || name === FINAL_REPORT_TOOL || name === 'agent__batch')
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
                        let textWithoutToolsFailureAdded = false;
                        let toolFailureDetected = sanitizedMessages.some((msg) => msg.role === 'tool' && typeof msg.content === 'string' && msg.content.trim().toLowerCase().startsWith(TOOL_FAILED_PREFIX));
                        if (toolFailureDetected) {
                            const failureToolMessage = sanitizedMessages
                                .filter((msg) => msg.role === 'tool' && typeof msg.content === 'string')
                                .find((msg) => {
                                const trimmed = msg.content.trim().toLowerCase();
                                return trimmed.startsWith(TOOL_FAILED_PREFIX);
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
                                                this.state.finalReportToolFailedThisTurn = true;
                                                this.state.finalReportToolFailedEver = true;
                                            }
                                        }
                                    }
                                    this.state.toolFailureMessages.delete(failureCallId);
                                }
                            }
                            // Fallback: if any failed tool call co-exists with an assistant-issued final_report call,
                            // mark the final report tool as failed even if the toolCallId mapping was lost.
                            if (!this.state.finalReportToolFailedEver) {
                                const assistantWithCall = sanitizedMessages.find((msg) => msg.role === 'assistant' && Array.isArray(msg.toolCalls));
                                const hasFinalReportCall = assistantWithCall?.toolCalls?.some((tc) => sanitizeToolName(tc.name) === FINAL_REPORT_TOOL) ?? false;
                                if (hasFinalReportCall) {
                                    this.state.finalReportToolFailedThisTurn = true;
                                    this.state.finalReportToolFailedEver = true;
                                }
                            }
                            const finalReportFailurePresent = sanitizedMessages.some((msg) => msg.role === 'tool' &&
                                typeof msg.content === 'string' &&
                                msg.content.includes(FINAL_REPORT_TOOL));
                            if (finalReportFailurePresent) {
                                this.state.finalReportToolFailedThisTurn = true;
                                this.state.finalReportToolFailedEver = true;
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
                                // Merge fallback execution stats into turnResult so turn validation sees them
                                const fbStats = fallbackResult.executionStats;
                                const existingStats = turnResult.executionStats ?? { executedTools: 0, executedNonProgressBatchTools: 0, executedProgressBatchTools: 0, unknownToolEncountered: false };
                                turnResult.executionStats = {
                                    executedTools: existingStats.executedTools + fbStats.executedTools,
                                    executedNonProgressBatchTools: existingStats.executedNonProgressBatchTools + fbStats.executedNonProgressBatchTools,
                                    executedProgressBatchTools: existingStats.executedProgressBatchTools + fbStats.executedProgressBatchTools,
                                    unknownToolEncountered: (existingStats.unknownToolEncountered === true) || fbStats.unknownToolEncountered,
                                };
                                this.applyToolNameCorrections(sanitizedMessages);
                                const extractionMessage = `Retrying for proper tool call after extracting ${String(fallbackResult.toolCalls.length)} leaked call(s).`;
                                this.log({
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: TEXT_EXTRACTION_REMOTE_ID,
                                    fatal: false,
                                    message: extractionMessage,
                                });
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
                            } else if (/report_/i.test(textContent) || /final_report/i.test(textContent)) {
                                this.log({
                                    timestamp: Date.now(),
                                    severity: 'WRN' as const,
                                    turn: currentTurn,
                                    subturn: 0,
                                    direction: 'response' as const,
                                    type: 'llm' as const,
                                    remoteIdentifier: TEXT_EXTRACTION_REMOTE_ID,
                                    fatal: false,
                                    message: 'Retrying for proper tool call after text-only final_report payload (no callable tools detected).',
                                });
                            }
                        }
                        const existingFinalReportSource = this.ctx.finalReportManager.getSource();
                        const canAdoptFinalReport = (existingFinalReportSource === undefined || existingFinalReportSource === 'synthetic') && sanitizedHasToolCalls;
                        if (canAdoptFinalReport) {
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

                                const formatParamRaw = params.report_format;
                                const formatParam = typeof formatParamRaw === 'string' && formatParamRaw.trim().length > 0 ? formatParamRaw.trim() : undefined;
                                const statusParamRaw = params.status;
                                const statusParam = typeof statusParamRaw === 'string' && statusParamRaw.trim().length > 0 ? statusParamRaw.trim().toLowerCase() : undefined;
                                const limitFailure = this.state.toolLimitExceeded || [...this.state.toolFailureMessages.values()]
                                    .some((msg) => typeof msg === 'string' && msg.includes('per-turn tool limit'));
                                const commitSource: FinalReportSource = (statusParam === 'failure' && (limitFailure || this.state.finalReportToolFailedEver)) ? 'synthetic' : 'tool-call';
                                const metadataCandidate = params.metadata;
                                let commitMetadata: Record<string, unknown> = (metadataCandidate !== null && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate))
                                    ? { ...(metadataCandidate as Record<string, unknown>) }
                                    : {};
                                const statusValue = statusParam ?? 'success';
                                commitMetadata = { ...commitMetadata, status: statusValue };

                                // Get raw payload: from XML transport (_rawPayload) or native fields
                                const rawPayloadValue = params._rawPayload;
                                const rawPayload = typeof rawPayloadValue === 'string' ? rawPayloadValue.trim() : undefined;

                                // Native transport fallbacks (when _rawPayload not present)
                                const contentValue = params.report_content;
                                const contentParam = typeof contentValue === 'string' && contentValue.trim().length > 0 ? contentValue.trim() : undefined;
                                const messagesValue = params.messages;
                                const messagesParam = Array.isArray(messagesValue) ? messagesValue : undefined;
                                const contentJsonCandidate = params.content_json;
                                const setFinalReportFailureMessage = (content: string): void => {
                                    const callId = typeof finalReportCall.id === 'string' ? finalReportCall.id : undefined;
                                    const existing = callId === undefined
                                        ? undefined
                                        : sanitizedMessages.find((msg) => msg.role === 'tool' && msg.toolCallId === callId);
                                    if (existing !== undefined) {
                                        existing.content = content;
                                        return;
                                    }
                                    sanitizedMessages.push({
                                        role: 'tool',
                                        content,
                                        toolCallId: callId,
                                    });
                                };

                                // Validate wrapper: format must match expected
                                const formatCandidate = formatParam ?? expectedFormat;
                                const finalFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate) ?? expectedFormat;
                                if (finalFormat !== expectedFormat) {
                                    const reason = `expected \"${expectedFormat}\", received \"${formatCandidate}\".`;
                                    this.addTurnFailure('final_report_format_mismatch', reason);
                                    this.logFinalReportDump(currentTurn, params, `format mismatch: expected ${expectedFormat}, got ${formatCandidate}`, rawContent);
                                    return false;
                                }

                                // =================================================================
                                // LAYER 2: Format-specific content processing
                                // Process raw payload based on format requirements
                                // =================================================================
                                let finalContent: string | undefined;
                                let contentJson: Record<string, unknown> | undefined;
                                if (contentJsonCandidate !== null && typeof contentJsonCandidate === 'object' && !Array.isArray(contentJsonCandidate)) {
                                    contentJson = contentJsonCandidate as Record<string, unknown>;
                                }

                                if (expectedFormat === 'sub-agent') {
                                    // SUB-AGENT: Opaque blob - no validation, no parsing
                                    // Whatever the model returns is passed through unchanged
                                    finalContent = rawPayload ?? contentParam ?? '';
                                    if (finalContent.length === 0) {
                                        this.addTurnFailure('final_report_content_missing');
                                        this.logFinalReportDump(currentTurn, params, 'empty sub-agent payload', rawContent);
                                        return false;
                                    }
                                } else if (expectedFormat === 'json') {
                                    // JSON: Parse and validate against schema
                                    // Source priority: rawPayload (XML) > content_json (native/string) > report_content (stringified JSON)
                                    const jsonSource = rawPayload
                                        ?? (typeof contentJsonCandidate === 'string' ? contentJsonCandidate : undefined)
                                        ?? contentParam;
                                    let jsonParseError: string | undefined;
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
                                        } else {
                                            jsonParseError = parsedJson.error ?? 'expected_json_object';
                                        }
                                    } else if (contentJsonCandidate !== null && typeof contentJsonCandidate === 'object' && !Array.isArray(contentJsonCandidate)) {
                                        // Native: content_json already parsed as object
                                        contentJson = contentJsonCandidate as Record<string, unknown>;
                                    }
                                    if (contentJson === undefined) {
                                        const safeJsonParseError: string | undefined = typeof jsonParseError === 'string' ? jsonParseError : undefined;
                                        const parseErrorValue: string = typeof safeJsonParseError === 'string' ? safeJsonParseError : 'missing_json_payload';
                                        const parseErrorHint: string = formatJsonParseHint(parseErrorValue);
                                        const finalReportJsonRequired = FINAL_REPORT_JSON_REQUIRED;
                                        const parseHint = `${finalReportJsonRequired} invalid_json: ${parseErrorHint}`;
                                        const parseReason = `invalid_json: ${parseErrorHint}`;
                                        this.log({
                                            timestamp: Date.now(),
                                            severity: 'ERR' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: REMOTE_ORCHESTRATOR,
                                            fatal: false,
                                            message: `final_report(json) ${parseHint}`
                                        });
                                        this.addTurnFailure('final_report_json_required', parseReason);
                                        this.logFinalReportDump(currentTurn, params, 'expected JSON content', rawContent);
                                        const failureMessage = `final_report(json) requires \`content_json\` (object). invalid_json: ${parseErrorHint}`;
                                        sanitizedMessages.push({
                                            role: 'tool',
                                            content: `${TOOL_FAILED_PREFIX} ${failureMessage})`,
                                            toolCallId: typeof finalReportCall.id === 'string' ? finalReportCall.id : undefined,
                                        });
                                        this.state.finalReportToolFailedThisTurn = true;
                                        this.state.finalReportToolFailedEver = true;
                                        toolFailureDetected = true;
                                        return false;
                                    }
                                    finalContent = JSON.stringify(contentJson);
                                } else if (expectedFormat === SLACK_BLOCK_KIT_FORMAT) {
                                    // SLACK-BLOCK-KIT: Parse JSON, expect messages array
                                    // Source: rawPayload (XML) > messages (native)
                                    const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
                                    const parsedSlack = parseSlackBlockKitPayload({
                                        rawPayload,
                                        messagesParam,
                                        contentParam,
                                    });

                                    const messagesArray = parsedSlack.messages;
                                    if (parsedSlack.repairs.length > 0) {
                                        this.log({
                                            timestamp: Date.now(),
                                            severity: 'WRN' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: REMOTE_SANITIZER,
                                            fatal: false,
                                            message: `agent__final_report slack-block-kit repaired via [${parsedSlack.repairs.join('>')}]`,
                                        });
                                    }

                                    if ((messagesArray === undefined || messagesArray.length === 0) && parsedSlack.fallbackLooksInvalid) {
                                        const slackError: string | undefined = typeof parsedSlack.error === 'string' ? parsedSlack.error : undefined;
                                        const slackErrorValue = slackError;
                                        const slackErrorHint: string = formatJsonParseHint(slackErrorValue);
                                        const finalReportSlackMissing = FINAL_REPORT_SLACK_MESSAGES_MISSING;
                                        const failureMessage = `${finalReportSlackMissing} invalid_json: ${slackErrorHint}`;
                                        const parseReason = `invalid_json: ${slackErrorHint}`;
                                        this.log({
                                            timestamp: Date.now(),
                                            severity: 'ERR' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: REMOTE_ORCHESTRATOR,
                                            fatal: false,
                                            message: `final_report(slack-block-kit) ${failureMessage}`
                                        });
                                        this.addTurnFailure('final_report_slack_messages_missing', parseReason);
                                        this.logFinalReportDump(currentTurn, params, 'expected messages array', rawContent);
                                        this.state.finalReportToolFailedThisTurn = true;
                                        this.state.finalReportToolFailedEver = true;
                                        toolFailureDetected = true;
                                        return false;
                                    }

                                    const normalization = normalizeSlackMessages(messagesArray ?? [], {
                                        fallbackText: parsedSlack.fallbackLooksInvalid ? undefined : (parsedSlack.fallbackText ?? undefined),
                                    });
                                    if (normalization.repairs.length > 0) {
                                        this.log({
                                            timestamp: Date.now(),
                                            severity: 'WRN' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: REMOTE_SANITIZER,
                                            fatal: false,
                                            message: `agent__final_report slack-block-kit mrkdwn repaired via [${normalization.repairs.join('>')}]`,
                                        });
                                    }

                                    if (normalization.messages.length === 0) {
                                        this.log({
                                            timestamp: Date.now(),
                                            severity: 'ERR' as const,
                                            turn: currentTurn,
                                            subturn: 0,
                                            direction: 'response' as const,
                                            type: 'llm' as const,
                                            remoteIdentifier: REMOTE_ORCHESTRATOR,
                                            fatal: false,
                                            message: 'final_report(slack-block-kit) requires `messages` or non-empty `content`'
                                        });
                                        this.addTurnFailure('final_report_slack_messages_missing');
                                        this.logFinalReportDump(currentTurn, params, 'expected messages array', rawContent);
                                        this.state.finalReportToolFailedThisTurn = true;
                                        this.state.finalReportToolFailedEver = true;
                                        toolFailureDetected = true;
                                        return false;
                                    }

                                    const slackValue = (commitMetadata as { slack?: unknown }).slack;
                                    const slackMetaExisting = isRecord(slackValue) ? slackValue : {};
                                    commitMetadata = { ...commitMetadata, slack: { ...slackMetaExisting, messages: normalization.messages } };
                                    finalContent = JSON.stringify(normalization.messages);

                                } else {
                                    if (finalFormat === 'json' && contentJson === undefined) {
                                        const failureMessage = 'final_report(json) requires `content_json` (object).';
                                        const failureToolMessage = `${TOOL_FAILED_PREFIX} ${failureMessage})`;
                                        setFinalReportFailureMessage(failureToolMessage);
                                        this.addTurnFailure('final_report_content_missing');
                                        this.state.finalReportToolFailedThisTurn = true;
                                        this.state.finalReportToolFailedEver = true;
                                        toolFailureDetected = true;
                                        return false;
                                    }
                                    // TEXT FORMATS (text, markdown, markdown+mermaid, tty, pipe)
                                    // Use raw payload or report_content directly
                                    finalContent = rawPayload ?? contentParam;
                                    if (finalContent === undefined || finalContent.length === 0) {
                                        const failureMessage = 'final_report requires non-empty report_content.';
                                        const failureToolMessage = `${TOOL_FAILED_PREFIX} ${failureMessage})`;
                                        setFinalReportFailureMessage(failureToolMessage);
                                        this.addTurnFailure('final_report_content_missing');
                                        this.state.finalReportToolFailedThisTurn = true;
                                        this.state.finalReportToolFailedEver = true;
                                        toolFailureDetected = true;
                                        this.logFinalReportDump(currentTurn, params, 'expected report_content', rawContent);
                                        return false;
                                    }
                                }

                                // =================================================================
                                // LAYER 3: Final Report construction
                                // Build clean final report object
                                // =================================================================

                                // Handle truncated output (unstructured formats only - structured would have been rejected)
                                const truncatedFlag = params._truncated as boolean | undefined;
                                if (truncatedFlag === true) {
                                    commitMetadata = { ...commitMetadata, truncated: true };
                                    finalContent = `${finalContent}\n\n[OUTPUT TRUNCATED: Response exceeded output token limit.]`;
                                }

                                // Build pending payload for pre-commit validation
                                const pendingPayload: PendingFinalReportPayload = {
                                    format: finalFormat,
                                    content: finalContent,
                                    content_json: contentJson,
                                    metadata: Object.keys(commitMetadata).length > 0 ? commitMetadata : undefined,
                                };

                                // Validate BEFORE commit to enable retry on validation failure
                                const preCommitSchema = this.ctx.sessionConfig.expectedOutput?.schema;
                                const preCommitValidation = this.ctx.finalReportManager.validatePayload(pendingPayload, preCommitSchema);
                                if (!preCommitValidation.valid) {
                                    const errs = preCommitValidation.errors ?? 'unknown validation error';
                                    const preview = preCommitValidation.payloadPreview;
                                    const mismatchMessage = buildSchemaMismatchFailure(errs, preview);
                                    const failureMessage = `final_report schema validation failed: ${mismatchMessage}`;
                                    this.log({
                                        timestamp: Date.now(),
                                        severity: 'ERR' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: 'agent:ajv',
                                        fatal: false,
                                        message: `pre-commit validation failed: ${failureMessage}`
                                    });
                                    setFinalReportFailureMessage(`${TOOL_FAILED_PREFIX} ${failureMessage}`);
                                    const schemaSummary = formatSchemaMismatchSummary(errs);
                                    this.addTurnFailure('final_report_schema_validation_failed', schemaSummary);
                                    this.state.finalReportToolFailedThisTurn = true;
                                    this.state.finalReportToolFailedEver = true;
                                    this.state.finalReportSchemaFailed = true;
                                    toolFailureDetected = true;
                                    // Set error flags to ensure retry even if other tools succeeded
                                    lastError = `invalid_response: ${failureMessage}`;
                                    lastErrorType = 'invalid_response';
                                    // DON'T commit - return false to trigger retry
                                    return false;
                                }

                                this.commitFinalReport(pendingPayload, commitSource);
                                sanitizedHasToolCalls = true;
                                return true;
                            };
                            if (!adoptFromToolCall()) {
                                if (!toolFailureDetected) {
                                    // No adoption possible; fall through to retry logic.
                                }
                            }
                        }
                        // Synthetic error: success with content but no tools and no final_report
                        if (this.finalReport === undefined) {
                        if (!sanitizedHasToolCalls && sanitizedHasText) {
                            lastError = 'invalid_response: content_without_tools_or_final';
                            lastErrorType = 'invalid_response';
                            this.addTurnFailure('content_without_tools_or_report');
                            textWithoutToolsFailureAdded = true;
                        }
                        }
                        // CONTRACT: Empty response without tool calls must NOT be added to conversation
                        // Check BEFORE pushing to conversation
                        const isEmptyWithoutTools = !turnResult.status.hasToolCalls &&
                            (turnResult.response === undefined || turnResult.response.trim().length === 0);
                        const isReasoningOnly = isEmptyWithoutTools && turnResult.hasReasoning === true;
                        const isStopReasonLength = turnResult.stopReason === 'length' || turnResult.stopReason === 'max_tokens';
                        const appendTruncationFailure = (): void => {
                            if (!isStopReasonLength)
                                return;
                            const tokenLimit = this.ctx.sessionConfig.maxOutputTokens;
                            const reason = typeof tokenLimit === 'number' ? `token_limit: ${String(tokenLimit)}` : undefined;
                            this.addTurnFailure('output_truncated', reason);
                        };
                        if (isEmptyWithoutTools && !isReasoningOnly) {
                            // Log warning and retry this turn on another provider/model
                            this.state.llmSyntheticFailures++;
                            lastError = 'invalid_response: empty_without_tools';
                            lastErrorType = 'invalid_response';
                            logAttemptFailure();
                            if (cycleComplete) {
                                rateLimitedInCycle = 0;
                                maxRateLimitWaitMs = 0;
                            }
                            appendTruncationFailure();
                            this.addTurnFailure('empty_response');
                            // do not mark turnSuccessful; continue retry loop
                            continue;
                        }
                        if (isReasoningOnly) {
                            const reasoningOnlySlug = isFinalTurn ? 'reasoning_only_final' : 'reasoning_only';
                            this.addTurnFailure(reasoningOnlySlug);
                        }
                        if (isFinalTurn && this.finalReport === undefined && turnHadFinalReportAttempt && sanitizedHasToolCalls) {
                            const toolMessageFallback = sanitizedMessages.find((msg) => msg.role === 'tool' &&
                                typeof msg.content === 'string' &&
                                msg.content.trim().length > 0 &&
                                !msg.content.trim().toLowerCase().startsWith(TOOL_FAILED_PREFIX));
                            if (toolMessageFallback !== undefined) {
                                const fallbackFormat = this.ctx.resolvedFormat ?? 'text';
                                const fallbackPayload: PendingFinalReportPayload = {
                                    format: fallbackFormat,
                                    content: toolMessageFallback.content,
                                    metadata: { reason: 'tool_message_fallback' },
                                };
                                // Validate fallback before commit
                                const fallbackSchema = this.ctx.sessionConfig.expectedOutput?.schema;
                                const fallbackValidation = this.ctx.finalReportManager.validatePayload(fallbackPayload, fallbackSchema);
                                if (fallbackValidation.valid) {
                                    this.commitFinalReport(fallbackPayload, FINAL_REPORT_SOURCE_TOOL_MESSAGE);
                                    this.log({
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: TEXT_EXTRACTION_REMOTE_ID,
                                        fatal: false,
                                        message: 'Adopted final report from tool message after invalid final_report call.',
                                    });
                                    this.log({
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: 'agent:fallback-report',
                                        fatal: false,
                                        message: 'Final report synthesized from tool message fallback.',
                                        details: { source: FINAL_REPORT_SOURCE_TOOL_MESSAGE }
                                    });
                                } else {
                                    // Fallback also failed validation - log and set retry flags
                                    const fallbackErrs = fallbackValidation.errors ?? 'unknown';
                                    this.log({
                                        timestamp: Date.now(),
                                        severity: 'WRN' as const,
                                        turn: currentTurn,
                                        subturn: 0,
                                        direction: 'response' as const,
                                        type: 'llm' as const,
                                        remoteIdentifier: 'agent:ajv',
                                        fatal: false,
                                        message: `tool_message_fallback validation failed: ${fallbackErrs}`,
                                    });
                                    const fallbackSummary = formatSchemaMismatchSummary(fallbackErrs);
                                    this.addTurnFailure('tool_message_fallback_schema_failed', fallbackSummary);
                                    // Set retry flags to ensure turn is not marked successful
                                    this.state.finalReportToolFailedThisTurn = true;
                                    this.state.finalReportToolFailedEver = true;
                                    lastError = `invalid_response: fallback validation failed: ${fallbackErrs}`;
                                    lastErrorType = 'invalid_response';
                                }
                            }
                        }
                        const finalReportReady = this.finalReport !== undefined;
        const executionStats = turnResult.executionStats ?? { executedTools: 0, executedNonProgressBatchTools: 0, executedProgressBatchTools: 0, unknownToolEncountered: false };
        const hasNonProgressTools = executionStats.executedNonProgressBatchTools > 0;
        const hasProgressOnly = executionStats.executedProgressBatchTools > 0 && executionStats.executedNonProgressBatchTools === 0;
        const nextProgressOnlyTurns = hasProgressOnly ? (this.state.consecutiveProgressOnlyTurns ?? 0) + 1 : 0;
        this.state.consecutiveProgressOnlyTurns = nextProgressOnlyTurns;

        if (nextProgressOnlyTurns >= 5 && this.forcedFinalTurnReason === undefined) {
            this.ctx.contextGuard.setTaskStatusOnlyReason();
            deferTaskStatusOnlyFinal = true;
            this.log({
                timestamp: Date.now(),
                severity: 'WRN' as const,
                turn: currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: REMOTE_ORCHESTRATOR,
                fatal: false,
                message: `Consecutive standalone task_status calls detected (count=${String(nextProgressOnlyTurns)}, threshold=5); forcing final turn.`,
                details: { consecutiveProgressOnlyTurns: nextProgressOnlyTurns, threshold: 5 },
            });
        }

        // Handle task status completion - force final turn if task is completed
        if (executionStats.executedProgressBatchTools > 0 && this.state.lastTaskStatusCompleted === true) {
            this.ctx.contextGuard.setTaskCompletionReason();
        }

        // Sync final turn flags after task status reason setting
        syncFinalTurnFlags();
                        // Add new messages to conversation (skipped above for empty responses per CONTRACT)
                        conversation.push(...sanitizedMessages);
                        // If we encountered an invalid response and still have retries in this turn, retry
                        // Skip retry if progress tools succeeded (standalone task_status consumes the turn per TODO design)
                        if (!finalReportReady && !hasNonProgressTools && !hasProgressOnly && lastErrorType === 'invalid_response' && attempts < maxRetries) {
                            // Collapse turns on retry if a final report was attempted but rejected
                            if (turnHadFinalReportAttempt && currentTurn < maxTurns) {
                                collapseRemainingTurns('final_report_attempt');
                            }
                            appendTruncationFailure();
                            logAttemptFailure();
                            continue;
                        }
                        // Check for retryFlags from turnResult (incompleteFinalReportDetected comes from tool execution)
                        const retryFlags = turnResult as { incompleteFinalReportDetected?: boolean; finalReportAttempted?: boolean };
                        const incompleteFinalReportDetected = retryFlags.incompleteFinalReportDetected === true;
                        const finalReportAttemptFlag = turnHadFinalReportAttempt || retryFlags.finalReportAttempted === true;
                        // If final report is already accepted, skip collapse and finalize immediately
                        if (finalReportReady) {
                            return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
                        }
                        // Only collapse remaining turns if final report is NOT ready
                        if (incompleteFinalReportDetected && currentTurn < maxTurns) {
                            collapseRemainingTurns('incomplete_final_report');
                        }
                        else if (finalReportAttemptFlag && currentTurn < maxTurns) {
                            collapseRemainingTurns('final_report_attempt');
                        }
                        // Determine turn success based on tool execution or accepted final report
                        // Progress-only (standalone task_status) also succeeds - turn is consumed per TODO design
                        // BUT: if final report validation failed, do NOT mark successful - retry instead
                        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
                        if ((finalReportReady || hasNonProgressTools || hasProgressOnly) && lastErrorType !== 'invalid_response') {
                            turnSuccessful = true;
                            break;
                        }

                        // Note: empty response check moved earlier (before conversation.push) per CONTRACT
                        // No final report yet
                        if (isFinalTurn) {
                            lastError = 'invalid_response: final_turn_no_final_answer';
                            lastErrorType = 'invalid_response';
                            logAttemptFailure();
                            if (cycleComplete) {
                                rateLimitedInCycle = 0;
                                maxRateLimitWaitMs = 0;
                            }
                            appendTruncationFailure();
                            this.addTurnFailure('final_turn_no_report');
                            continue;
                        }
                        // Non-final turns: no tools executed (progress-only already succeeded above)
                        lastError = 'invalid_response: no_tools';
                        if (!sanitizedHasText && isReasoningOnly) {
                            this.addTurnFailure('reasoning_only');
                        } else if (sanitizedHasText && !textWithoutToolsFailureAdded) {
                            this.addTurnFailure('content_without_tools_or_report');
                        }
                        lastErrorType = 'invalid_response';
                        appendTruncationFailure();
                        logAttemptFailure();
                        continue;
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
                            logAttemptFailure();
                            pairCursor += 1;
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
                                    logAttemptFailure();
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
                                logAttemptFailure();
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

                // If this attempt failed and we're about to retry (no earlier continue), emit the per-attempt failure log now.
                if (!turnSuccessful && attempts < maxRetries) {
                    logAttemptFailure();
                }
            }
            // Check if all attempts failed for this turn (retry exhaustion)
            if (!turnSuccessful) {
                // Calculate if this is a final turn (same logic as in executeSingleTurn)
                const forcedFinalTurn = this.forcedFinalTurnReason !== undefined;
                const isCurrentlyFinalTurn = forcedFinalTurn || currentTurn === maxTurns;
                
                // Rule 5: Force final turn on retry exhaustion instead of session failure
                // Exception: If already in final turn, fail normally (prevent infinite loop)
                if (isCurrentlyFinalTurn) {
                    // Already in final turn - fail session normally
                    const failureInfo = this.buildTurnFailureInfo({
                        turn: currentTurn,
                        provider: pairs[(attempts - 1) % pairs.length]?.provider ?? 'unknown',
                        model: pairs[(attempts - 1) % pairs.length]?.model ?? 'unknown',
                        lastError: typeof lastError === 'string' ? lastError : (lastErrorType ?? 'unknown'),
                        lastErrorType,
                        lastTurnResult,
                        attempts,
                        maxRetries,
                        maxTurns,
                        isFinalTurn: currentTurn === maxTurns,
                    });
                    this.logTurnFailure(failureInfo);
                    return this.finalizeSessionFailure(conversation, logs, accounting, failureInfo, currentTurn);
                }
                // Normal case: trigger graceful final turn
                this.ctx.contextGuard.setRetryExhaustedReason();
                // DO NOT mark turnSuccessful - proceed to next turn with forced final status
            }
            // End of retry loop - close turn if successful (turnSuccessful may be true due to retry exhaustion)
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
            // Preserve lastError for post-loop access
            this.state.lastTurnError = typeof lastError === 'string' ? lastError : undefined;
            this.state.lastTurnErrorType = typeof lastErrorType === 'string' ? lastErrorType : undefined;
        }
        // Max turns exceeded - synthesize final report if missing (unless final_report tool itself failed)
        if (this.finalReport === undefined && !this.state.finalReportToolFailedEver) {
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
        if (this.finalReport === undefined && this.state.finalReportToolFailedEver) {
            const currentTurn = this.state.currentTurn;
            const turnLabel = `${String(maxTurns)} turn${maxTurns === 1 ? '' : 's'}`;
            const savedError = this.state.lastTurnError;
            const baseSummary = `Session completed without a final report after ${turnLabel}. The final_report emission failed.`;
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
            severity: 'WRN',
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
    private get forcedFinalTurnReason(): 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion' | undefined { return this.ctx.contextGuard.getForcedFinalReason(); }
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
    private logEnteringFinalTurn(reason: FinalTurnLogReason, turn: number, originalMaxTurns: number, activeMaxTurns: number): void {
        if (this.state.finalTurnEntryLogged)
            return;
        const severity = reason === 'task_status_completed' ? 'VRB' as const : 'WRN' as const;
        const baseTurnLabel = `${String(turn)}/${String(originalMaxTurns)}`;
        const activeSuffix = activeMaxTurns !== originalMaxTurns ? `, active_max=${String(activeMaxTurns)}` : '';
        const message = `Final turn detected (turn=${baseTurnLabel}${activeSuffix}, reason=${reason}); removing all tools to force final-report.`;
        const warnEntry: LogEntry = {
            timestamp: Date.now(),
            severity,
            turn,
            subturn: 0,
            direction: 'request' as const,
            type: 'llm' as const,
            remoteIdentifier: REMOTE_FINAL_TURN,
            fatal: false,
            message,
            details: {
                final_turn_reason: reason,
                original_max_turns: originalMaxTurns,
                active_max_turns: activeMaxTurns,
            },
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
    private addTurnFailure(slug: TurnFailedSlug, reason?: string): void {
        const normalizedReason = typeof reason === 'string' ? reason.trim() : undefined;
        const existing = this.state.turnFailedEvents.find((entry) => entry.slug === slug);
        if (existing !== undefined) {
            if ((existing.reason === undefined || existing.reason.trim().length === 0) && normalizedReason !== undefined && normalizedReason.length > 0) {
                existing.reason = normalizedReason;
            }
            return;
        }
        this.state.turnFailedEvents.push({
            slug,
            reason: normalizedReason,
            order: this.state.turnFailedCounter,
        });
        this.state.turnFailedCounter += 1;
    }
    private flushTurnFailureReasons(conversation: ConversationMessage[]): void {
        if (this.state.turnFailedEvents.length === 0)
            return;
        const feedback = buildTurnFailedNotice(this.state.turnFailedEvents);
        conversation.push({ role: 'user', content: feedback });
        this.state.turnFailedEvents = [];
        this.state.turnFailedCounter = 0;
    }
    private encodeSnapshotPayload(payload: unknown, format: string): { format: string; encoding: 'base64'; value: string } | undefined {
        try {
            const serialized = JSON.stringify(payload);
            const encoded = Buffer.from(serialized, 'utf8').toString('base64');
            return { format, encoding: 'base64', value: encoded };
        }
        catch {
            try {
                const fallback = Buffer.from('[unavailable]', 'utf8').toString('base64');
                return { format, encoding: 'base64', value: fallback };
            }
            catch {
                return undefined;
            }
        }
    }
    private captureSdkRequestSnapshot(opId: string | undefined, payload: unknown): void {
        if (typeof opId !== 'string' || opId.length === 0)
            return;
        const encoded = this.encodeSnapshotPayload(payload, 'sdk');
        if (encoded === undefined)
            return;
        this.ctx.opTree.setRequest(opId, { kind: 'llm', payload: { sdk: encoded } });
        this.callbacks.onOpTree(this.ctx.opTree.getSession());
    }
    private captureSdkResponseSnapshot(opId: string | undefined, payload: unknown): void {
        if (typeof opId !== 'string' || opId.length === 0)
            return;
        const encoded = this.encodeSnapshotPayload(payload, 'sdk');
        if (encoded === undefined)
            return;
        this.ctx.opTree.setResponse(opId, { payload: { sdk: encoded } });
        this.callbacks.onOpTree(this.ctx.opTree.getSession());
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
        }
        catch {
            // ignore serialization errors
        }
    }


  private buildTurnFailureInfo(params: {
    turn: number;
    provider: string;
    model: string;
    lastError: string;
    lastErrorType?: string;
    lastTurnResult?: TurnResult;
    attempts: number;
    maxRetries: number;
    maxTurns: number;
    isFinalTurn: boolean;
  }): { turn: number; slugs: string[]; provider: string; model: string; message: string; rawResponse?: string; rawResponseTruncated?: boolean } {
    const { turn, provider, model, lastError, lastErrorType, lastTurnResult, attempts, maxRetries, maxTurns, isFinalTurn } = params;
    const slugs = new Set<string>();
    // Only add retries_exhausted when attempts actually reached maxRetries
    if (attempts >= maxRetries) slugs.add('retries_exhausted');
    if (isFinalTurn) slugs.add('final_turn');
    if (this.state.toolLimitExceeded) slugs.add('tool_limit');
    if (this.forcedFinalTurnReason === 'context') slugs.add('context_guard');
    const stats = lastTurnResult?.executionStats ?? { executedTools: 0, executedNonProgressBatchTools: 0, executedProgressBatchTools: 0, unknownToolEncountered: false };
    const hasNonProgressTools = stats.executedNonProgressBatchTools > 0;
    const hasProgressOnly = stats.executedProgressBatchTools > 0 && stats.executedNonProgressBatchTools === 0;
    const hadToolCalls = (lastTurnResult?.messages ?? []).some((m) => {
        const toolCalls = (m as { toolCalls?: unknown }).toolCalls;
        return Array.isArray(toolCalls) && toolCalls.length > 0;
    });
    if (hasProgressOnly) slugs.add('task_status_only');
    else if (!hasNonProgressTools) slugs.add('no_tools');
    if (this.finalReport === undefined) slugs.add('final_report_missing');
    const responseText = typeof lastTurnResult?.response === 'string' ? lastTurnResult.response.trim() : '';
    if (responseText.length === 0) slugs.add('empty_response');
    if (lastTurnResult?.hasReasoning === true && (responseText.length === 0)) slugs.add('reasoning_only');
    if (responseText.length > 0 && !hasNonProgressTools && this.finalReport === undefined) slugs.add('text_only');
    const stopReason = lastTurnResult?.stopReason;
    if (stopReason === 'length' || stopReason === 'max_tokens') slugs.add('stop_reason_length');
    if (lastErrorType === 'invalid_response') slugs.add('invalid_response');
    if (lastErrorType === 'rate_limit') slugs.add('rate_limited');
    if (lastErrorType === 'timeout') slugs.add('provider_timeout');
    if (lastErrorType === 'network_error') slugs.add('provider_network_error');
    if (lastErrorType === 'auth_error') slugs.add('auth_error');
    if (lastErrorType === 'quota_exceeded') slugs.add('quota_exceeded');
    if (lastErrorType === 'model_error') slugs.add('provider_error');
    // Tool error slugs inferred from toolFailureMessages/trimmed ids
    if (this.state.toolFailureMessages.size > 0 || this.state.toolFailureFallbacks.length > 0) slugs.add('tool_exec_failed');
    const anyUnknown = (lastTurnResult?.executionStats?.unknownToolEncountered === true) || (lastTurnResult?.messages ?? []).some((m) => m.role === 'tool' && typeof m.content === 'string' && isUnknownToolFailureMessage(m.content));
    if (anyUnknown) slugs.add('unknown_tool');
    if (this.state.trimmedToolCallIds.size > 0 || this.state.droppedInvalidToolCalls > 0 || (hadToolCalls && stats.executedTools === 0)) slugs.add('malformed_tool_call');
    if (this.state.finalReportToolFailedEver) {
        slugs.add('final_report_tool_failed');
    }

    if (this.state.finalReportInvalidFormat) {
        slugs.add('final_report_invalid_format');
    }
    if (this.state.finalReportSchemaFailed) {
        slugs.add('final_report_schema_fail');
    }
    if (this.state.lastFinalReportStatus === 'failure') {
        slugs.add('final_report_status_failure');
    }

    const rawResponseLimit = 131072; // ~128KB cap
    const rawResponse = typeof lastTurnResult?.response === 'string' ? lastTurnResult.response : undefined;
    let truncated = false;
    let clippedResponse: string | undefined = rawResponse;
    if (rawResponse !== undefined && Buffer.byteLength(rawResponse, 'utf8') > rawResponseLimit) {
      clippedResponse = truncateToBytes(rawResponse, rawResponseLimit) ?? '(tool failed: response exceeded max size)';
      truncated = clippedResponse !== rawResponse;
    }
    const msg = `Turn ${String(turn)} failed after ${String(attempts)} attempt${attempts === 1 ? '' : 's'} of ${String(maxRetries)} (maxTurns=${String(maxTurns)}); last_error=${lastError}`;
    return { turn, slugs: [...slugs], provider, model, message: msg, rawResponse: clippedResponse, rawResponseTruncated: truncated };
  }

  private isReasoningGuard(value: unknown): value is { disable: boolean; normalized: ConversationMessage[] } {
    if (value === null || typeof value !== 'object') return false;
    const disable = (value as { disable?: unknown }).disable;
    const normalized = (value as { normalized?: unknown }).normalized;
    return typeof disable === 'boolean' && Array.isArray(normalized);
  }

  private logTurnFailure(info: { turn: number; slugs: string[]; provider: string; model: string; message: string; rawResponse?: string; rawResponseTruncated?: boolean }): void {
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity: 'WRN',
      turn: info.turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: 'agent:turn-failure',
      fatal: false,
      message: `${info.message} [slugs=${info.slugs.join(',')}]`,
      details: {
        provider: info.provider,
        model: info.model,
        slugs: info.slugs.join(','),
        raw_response_truncated: Boolean(info.rawResponseTruncated),
        raw_response: info.rawResponse ?? '',
      },
    };
    this.log(entry);
  }

  private finalizeSessionFailure(
    conversation: ConversationMessage[],
    logs: LogEntry[],
    accounting: AccountingEntry[],
    failureInfo: { slugs: string[]; message: string; rawResponse?: string; rawResponseTruncated?: boolean },
    currentTurn: number
  ): AIAgentResult {
    this.flushTurnFailureReasons(conversation);
    const toolFailureNote = this.state.finalReportToolFailedEver ? ' The final_report emission failed.' : '';
    const summary = `${failureInfo.message}; slugs=${failureInfo.slugs.join(',')}.${toolFailureNote}`;
    this.logFailureReport(summary, currentTurn, {
        reason: 'session_failed',
        slugs: failureInfo.slugs.join(','),
        raw_response_truncated: failureInfo.rawResponseTruncated ?? false,
        turns_completed: currentTurn,
        final_report_attempts: this.ctx.finalReportManager.finalReportAttempts,
    });
    const fallbackFormat = this.ctx.resolvedFormat ?? 'text';
    this.commitFinalReport({
      format: fallbackFormat,
      content: summary,
      metadata: {
        reason: 'session_failed',
        slugs: failureInfo.slugs,
        raw_response_truncated: failureInfo.rawResponseTruncated ?? false,
        turns_completed: currentTurn,
        final_report_attempts: this.ctx.finalReportManager.finalReportAttempts,
      },
    }, 'synthetic');
    return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
  }
    private commitFinalReport(payload: PendingFinalReportPayload, source: FinalReportSource): void {
        this.state.finalReportInvalidFormat = false;
        this.state.finalReportSchemaFailed = false;
        const statusValueRaw = payload.metadata !== undefined && typeof payload.metadata === 'object'
            ? (payload.metadata as { status?: unknown }).status
            : undefined;
        const statusValue = typeof statusValueRaw === 'string' ? statusValueRaw.trim().toLowerCase() : undefined;
        this.state.lastFinalReportStatus = statusValue;
        this.log({
            timestamp: Date.now(),
            severity: 'VRB',
            turn: this.state.currentTurn,
            subturn: 0,
            direction: 'response',
            type: 'llm',
            remoteIdentifier: 'agent:debug',
            fatal: false,
            message: `commitFinalReport source=${source} format=${payload.format}${statusValue !== undefined ? ` status=${statusValue}` : ''}`
        });
        this.ctx.finalReportManager.commit(payload, source);
    }
    private logFinalReportAccepted(source: FinalReportSource, turn: number): void {
        const entry: LogEntry = {
            timestamp: Date.now(),
            severity: source === 'synthetic' ? 'ERR' : 'VRB',
            turn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: 'agent:final-report-accepted',
            fatal: false,
            message: source === 'synthetic'
                ? 'Synthetic final report generated.'
                : source === FINAL_REPORT_SOURCE_TOOL_MESSAGE
                    ? 'Final report accepted from tool message.'
                    : 'Final report accepted from tool call.',
            details: { source }
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
        const statusMeta = (() => {
            const metaStatus = fr.metadata !== undefined && typeof (fr.metadata as { status?: unknown }).status === 'string'
                ? ((fr.metadata as { status: string }).status.trim().toLowerCase())
                : undefined;
            return metaStatus;
        })();
        if (this.state.lastFinalReportStatus === undefined && statusMeta !== undefined) {
            this.state.lastFinalReportStatus = statusMeta;
        }
        // Validate structured formats before streaming/marking success
        if (fr.format === SLACK_BLOCK_KIT_FORMAT) {
            let slackMessages = ((): unknown[] | undefined => {
                const metaMsgs = (fr.metadata !== undefined && typeof fr.metadata === 'object')
                    ? ((fr.metadata as { slack?: { messages?: unknown[] } }).slack?.messages)
                    : undefined;
                if (Array.isArray(metaMsgs) && metaMsgs.length > 0) {
                    return metaMsgs;
                }
                if (typeof fr.content === 'string') {
                    try {
                        const parsed: unknown = JSON.parse(fr.content);
                        if (Array.isArray(parsed) && parsed.length > 0) {
                            const parsedArray = parsed as unknown[];
                            return parsedArray;
                        }
                        if (parsed !== null && typeof parsed === 'object') {
                            const messagesCandidate = (parsed as { messages?: unknown[] }).messages;
                            if (Array.isArray(messagesCandidate) && messagesCandidate.length > 0) {
                                const msgs: unknown[] = [...messagesCandidate];
                                return msgs;
                            }
                        }
                    }
                    catch { /* ignore parse errors */ }
                }
                return undefined;
            })();
            if ((slackMessages === undefined || slackMessages.length === 0) && typeof fr.content === 'string' && fr.content.trim().length > 0) {
                const fallbackMessages = [{ type: 'section', text: { type: 'mrkdwn', text: fr.content } }];
                const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
                const baseMeta: Record<string, unknown> = isRecord(fr.metadata)
                    ? { ...fr.metadata }
                    : {};
                const slackValue = baseMeta.slack;
                const slackMetaExisting: Record<string, unknown> = isRecord(slackValue)
                    ? slackValue
                    : {};
                const mergedSlack: Record<string, unknown> = { ...slackMetaExisting, messages: fallbackMessages };
                baseMeta.slack = mergedSlack;
                fr.metadata = baseMeta;
                slackMessages = fallbackMessages;
            }
            if (slackMessages === undefined || slackMessages.length === 0) {
                // Replace with synthetic failure report and log error for observability parity with tool adoption path
                this.log({
                    timestamp: Date.now(),
                    severity: 'ERR' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: REMOTE_ORCHESTRATOR,
                    fatal: false,
                    message: 'final_report(slack-block-kit) requires `messages` or non-empty `content`'
                });
                this.state.lastFinalReportStatus = 'failure';
                this.state.finalReportInvalidFormat = true;
                this.state.toolFailureMessages.set('final_report_invalid_format', 'slack_block_kit_missing_messages');
                const syntheticContent = 'Slack final report missing messages array.';
                this.commitFinalReport({
                    format: 'text',
                    content: syntheticContent,
                    metadata: { reason: 'invalid_final_report', original_format: fr.format }
                }, 'synthetic');
                this.state.finalReportToolFailedEver = true;
                return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
            }
        }
        // Validate schema: for 'json' format uses user-defined schema, for 'slack-block-kit' uses built-in schema
        const schema = this.ctx.sessionConfig.expectedOutput?.schema;
        const validationResult = this.ctx.finalReportManager.validateSchema(schema);
        if (!validationResult.valid) {
            const errs = validationResult.errors ?? 'unknown validation error';
            const payloadPreview = validationResult.payloadPreview;
            const mismatchMessage = buildSchemaMismatchFailure(errs, payloadPreview);
            const errMsg = `final_report schema validation failed: ${mismatchMessage}`;
            const errLog = {
                timestamp: Date.now(),
                severity: 'ERR' as const,
                turn: currentTurn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: 'agent:ajv',
                fatal: false,
                message: errMsg
            };
            this.log(errLog);
            this.state.lastFinalReportStatus = 'failure';
            this.state.finalReportSchemaFailed = true;
            this.state.toolFailureMessages.set('final_report_schema', errMsg);
            // Post-commit validation failure: replace with synthetic failure report.
            // NOTE: This is a safety net. Pre-commit validation should catch most cases and trigger retry.
            // Post-commit can't retry because finalizeWithCurrentFinalReport is called AFTER the retry
            // loop decides to finalize. Phase 2 cleanup will centralize final report logic to fix this.
            const source = this.ctx.finalReportManager.getSource();
            if (source !== 'synthetic') {
                this.log({
                    timestamp: Date.now(),
                    severity: 'ERR' as const,
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response' as const,
                    type: 'llm' as const,
                    remoteIdentifier: 'agent:ajv',
                    fatal: false,
                    message: 'Post-commit validation failed; replacing with synthetic failure report.'
                });
                this.ctx.finalReportManager.clear();
                const syntheticContent = `Final report validation failed: ${mismatchMessage}`;
                this.commitFinalReport({
                    format: 'text',
                    content: syntheticContent,
                    metadata: { reason: 'schema_validation_failed', original_format: fr.format, validation_errors: errs }
                }, 'synthetic');
                this.state.finalReportToolFailedEver = true;
                return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
            }
        }
        if (this.ctx.sessionConfig.renderTarget !== 'sub-agent' && this.callbacks.onOutput !== undefined) {
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
                if (!this.finalReportStreamed && !this.hasStreamedFinalOutput(finalOutput)) {
                    this.callbacks.onOutput(finalOutput, {
                        agentId: this.ctx.agentId,
                        callPath: this.ctx.callPath,
                        sessionId: this.ctx.txnId,
                        parentId: this.ctx.parentTxnId,
                        originId: this.ctx.originTxnId,
                    });
                    this.appendStreamedOutputTail(finalOutput);
                    if (!finalOutput.endsWith('\n')) {
                        this.callbacks.onOutput('\n', {
                            agentId: this.ctx.agentId,
                            callPath: this.ctx.callPath,
                            sessionId: this.ctx.txnId,
                            parentId: this.ctx.parentTxnId,
                            originId: this.ctx.originTxnId,
                        });
                        this.appendStreamedOutputTail('\n');
                    }
                    this.finalReportStreamed = true;
                }
                else {
                    this.finalReportStreamed = true;
                }
            }
        }
        const source = this.ctx.finalReportManager.getSource() ?? 'tool-call';
        this.log({
            timestamp: Date.now(),
            severity: 'VRB',
            turn: currentTurn,
            subturn: 0,
            direction: 'response' as const,
            type: 'llm' as const,
            remoteIdentifier: 'agent:debug',
            fatal: false,
            message: `finalize: source=${source} status_meta=${this.state.lastFinalReportStatus ?? 'none'} attempts=${String(this.ctx.finalReportManager.finalReportAttempts)} toolFailed=${this.state.finalReportToolFailedEver ? 'true' : 'false'} toolLimit=${this.state.toolLimitExceeded ? 'true' : 'false'}`
        });
        this.logFinalReportAccepted(source, currentTurn);
        const exitCode = this.forcedFinalTurnReason === 'context' ? 'EXIT-TOKEN-LIMIT' : 'EXIT-FINAL-ANSWER';
        const exitMessage = this.forcedFinalTurnReason === 'context'
            ? `Final report received after context guard triggered (${FINAL_REPORT_TOOL}), session complete`
            : `Final report received (${FINAL_REPORT_TOOL}), session complete`;
        // Transport-layer success: any model-provided report that passes validation is successful.
        // Content-level failure (model says "I couldn't find data") is semantic, not transport failure.
        // Synthetic reports (generated by system due to session failure) are the only transport failures.
        const success = source !== 'synthetic';
        this.logExit(exitCode, exitMessage, currentTurn, { fatal: !success, severity: success ? 'VRB' : 'ERR' });
        const metadataRecord = fr.metadata;
        const metadataReason = typeof metadataRecord?.reason === 'string' ? metadataRecord.reason : undefined;
        recordFinalReportMetrics({
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            headendId: this.ctx.headendId,
            source,
            turnsCompleted: currentTurn,
            finalReportAttempts: this.ctx.finalReportManager.finalReportAttempts,
            forcedFinalReason: this.forcedFinalTurnReason,
            syntheticReason: source === 'synthetic' ? metadataReason : undefined,
            customLabels: this.ctx.telemetryLabels,
        });
        this.emitFinalSummary(logs, accounting);
        const error = (() => {
            if (success) return undefined;
            if (typeof fr.content === 'string' && fr.content.length > 0) return fr.content;
            if (fr.content_json !== undefined) {
                try { return JSON.stringify(fr.content_json); } catch { /* ignore */ }
            }
            return undefined;
        })();
        const finalContentSize = (() => {
            if (fr.format === 'json' && fr.content_json !== undefined) {
                try { return JSON.stringify(fr.content_json).length; } catch { return 0; }
            }
            return typeof fr.content === 'string' ? fr.content.length : 0;
        })();
        const finalReportAccounting: AccountingEntry = {
            type: 'tool',
            timestamp: Date.now(),
            status: success ? 'ok' : 'failed',
            latency: Math.max(0, Date.now() - fr.ts),
            mcpServer: 'agent',
            command: FINAL_REPORT_TOOL,
            charactersIn: finalContentSize,
            charactersOut: 0,
            ...(error !== undefined ? { error } : {}),
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            txnId: this.ctx.txnId,
            parentTxnId: this.ctx.parentTxnId,
            originTxnId: this.ctx.originTxnId,
        };
        if (process.env.CONTEXT_DEBUG === 'true') {
            // Debug visibility for accounting expectations in phase1 harness
            console.log('final-report-accounting', finalReportAccounting);
        }
        accounting.push(finalReportAccounting);
        try {
            const accountingOpId = this.ctx.opTree.beginOp(currentTurn, 'system', { label: 'final-report' });
            this.ctx.opTree.appendAccounting(accountingOpId, finalReportAccounting);
            this.ctx.opTree.endOp(accountingOpId, success ? 'ok' : 'failed');
            this.callbacks.onOpTree(this.ctx.opTree.getSession());
        }
        catch (e) {
            warn(`final-report accounting append failed: ${e instanceof Error ? e.message : String(e)}`);
        }
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
                sources: status.sources,
            };
        }
        if (status.type === 'auth_error') {
            return {
                action: RETRY_ACTION_SKIP_PROVIDER,
                logMessage: `Authentication error: ${status.message}. Skipping provider ${remoteId}.`,
            };
        }
        if (status.type === 'quota_exceeded') {
            return {
                action: RETRY_ACTION_SKIP_PROVIDER,
                logMessage: `Quota exceeded for ${remoteId}; moving to next provider.`,
            };
        }
        if (status.type === 'timeout' || status.type === 'network_error') {
            const reason = status.message;
            const prettyType = status.type.replace('_', ' ');
            return {
                action: 'retry',
                backoffMs: Math.min(Math.max((attempt + 1) * 1_000, 1_000), 30_000),
                logMessage: `Transient ${prettyType} encountered on ${remoteId}: ${reason}; retrying shortly.`,
            };
        }
        if (status.type === 'model_error') {
            const retryable = status.retryable;
            if (retryable)
                return undefined;
            return {
                action: RETRY_ACTION_SKIP_PROVIDER,
                logMessage: `Non-retryable model error from ${remoteId}; skipping provider.`,
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
            toolNameCorrections: this.state.toolNameCorrections,
            trimmedToolCallIds: new Set(),
            executedToolCallIds: this.state.executedToolCallIds,
            turnHasSuccessfulToolOutput: false,
            incompleteFinalReportDetected: false,
            toolLimitExceeded: false,
            finalReportToolFailed: false,
            executedTools: 0,
            executedNonProgressBatchTools: 0,
            executedProgressBatchTools: 0,
            unknownToolEncountered: false,
            lastTaskStatusCompleted: this.state.lastTaskStatusCompleted,
            productiveToolExecutedThisTurn: false,
            onTaskCompletion: () => {
                // Force final turn immediately when task completion is detected
                this.ctx.contextGuard.setTaskCompletionReason();
            },
        };

        // Create tool executor for fallback execution
        const executor = this.ctx.sessionExecutor.createExecutor(turn, provider, executionState);

        // Delegate to the module, passing executionState as stats reference
        return processLeakedToolCalls({
            textContent,
            executor,
            executionStatsRef: executionState,
        });
    }

    private applyToolNameCorrections(messages: ConversationMessage[]): void {
        if (this.state.toolNameCorrections.size === 0) {
            return;
        }
        messages.forEach((message) => {
            if (message.role !== 'assistant') {
                return;
            }
            const toolCalls = (message as { toolCalls?: ToolCall[] }).toolCalls;
            if (!Array.isArray(toolCalls) || toolCalls.length === 0) {
                return;
            }
            toolCalls.forEach((call) => {
                const callId = call.id;
                if (typeof callId !== 'string' || callId.length === 0) {
                    return;
                }
                const corrected = this.state.toolNameCorrections.get(callId);
                if (corrected === undefined) {
                    return;
                }
                call.name = corrected;
                this.state.toolNameCorrections.delete(callId);
            });
        });
        this.state.toolNameCorrections.clear();
    }

    private sanitizeTurnMessages(messages: ConversationMessage[], context: { turn: number; provider: string; model: string; stopReason?: string; maxOutputTokens?: number }): { messages: ConversationMessage[]; dropped: number; finalReportAttempted: boolean; syntheticToolMessages: ConversationMessage[] } {
        let dropped = 0;
        let finalReportAttempted = false;
        const syntheticToolMessages: ConversationMessage[] = [];
        const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
        const formatPayload = (value: unknown): string => {
            if (typeof value === 'string') {
                return value;
            }
            try {
                return JSON.stringify(value);
            } catch {
                return String(value);
            }
        };
        const buildSanitizationParams = (reason: string, payload: string): { params: Record<string, unknown>; failureDetail: string } => {
            const truncated = truncateToBytes(payload, TRUNCATE_PREVIEW_BYTES);
            const safePayload = truncated ?? payload;
            const truncatedApplied = truncated !== undefined && truncated !== payload;
            const sha256 = truncatedApplied
                ? crypto.createHash('sha256').update(payload, 'utf8').digest('hex')
                : undefined;
            const truncationNote = truncatedApplied
                ? ` (truncated${sha256 !== undefined ? `, sha256=${sha256}` : ''})`
                : '';
            const failureDetail = `invalid tool call payload. ${reason}. Original payload${truncationNote}: ${safePayload}`;
            const params: Record<string, unknown> = {
                [TOOL_SANITIZATION_FAILED_KEY]: true,
                [TOOL_SANITIZATION_REASON_KEY]: reason,
                [TOOL_SANITIZATION_ORIGINAL_PAYLOAD_KEY]: safePayload,
                ...(truncatedApplied ? {
                    [TOOL_SANITIZATION_ORIGINAL_PAYLOAD_TRUNCATED_KEY]: true,
                    ...(sha256 !== undefined ? { [TOOL_SANITIZATION_ORIGINAL_PAYLOAD_SHA256_KEY]: sha256 } : {}),
                } : {}),
            };
            return { params, failureDetail };
        };
        const logInvalidToolCall = (reason: string, payload: string, name: string, id: string): void => {
            this.callbacks.log({
                timestamp: Date.now(),
                severity: 'ERR' as const,
                turn: context.turn,
                subturn: 0,
                direction: 'response' as const,
                type: 'llm' as const,
                remoteIdentifier: REMOTE_SANITIZER,
                fatal: false,
                message: `Invalid tool call payload sanitized: ${reason}`,
                details: { toolName: name, toolCallId: id, payload },
            });
        };
        const sanitized = messages.map((msg, _msgIndex) => {
            let rawToolCalls = msg.toolCalls;
            let parsedToolCalls;
            // XML transport parsing
            if (msg.role === 'assistant' && typeof msg.content === 'string') {
                const parseResult = this.ctx.xmlTransport.parseAssistantMessage(msg.content, { turn: context.turn, resolvedFormat: this.ctx.resolvedFormat, stopReason: context.stopReason, maxOutputTokens: context.maxOutputTokens }, {
                    onTurnFailure: (slug, reason) => { this.addTurnFailure(slug, reason); },
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
                        const id = crypto.randomUUID();
                        const payload = formatPayload(tc);
                        const reason = 'tool_call_not_object';
                        dropped++;
                        logInvalidToolCall(reason, payload, 'unknown', id);
                        const sanitized = buildSanitizationParams(reason, payload);
                        const failureMessage = `(tool failed: ${sanitized.failureDetail})`;
                        this.state.toolFailureMessages.set(id, failureMessage);
                        syntheticToolMessages.push({ role: 'tool', toolCallId: id, content: failureMessage });
                        calls.push({
                            name: 'unknown_tool',
                            id,
                            parameters: sanitized.params,
                        });
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
                    if (normalizedName.length === 0) {
                        const payload = formatPayload(tc);
                        const reason = 'tool_name_missing_or_invalid';
                        dropped++;
                        logInvalidToolCall(reason, payload, 'unknown', id);
                        if (isFinalReportCall) {
                            this.ctx.finalReportManager.incrementAttempts();
                        }
                        const sanitized = buildSanitizationParams(reason, payload);
                        const failureMessage = `(tool failed: ${sanitized.failureDetail})`;
                        this.state.toolFailureMessages.set(id, failureMessage);
                        syntheticToolMessages.push({ role: 'tool', toolCallId: id, content: failureMessage });
                        calls.push({
                            name: 'unknown_tool',
                            id,
                            parameters: sanitized.params,
                        });
                        return;
                    }
                    let parameters: Record<string, unknown> = {};
                    let invalidReason: string | undefined;
                    if (isRecord(tc.parameters)) {
                        parameters = tc.parameters;
                    }
                    else if (typeof tc.parameters === 'string') {
                        const parsed = (() => {
                            try { return parseJsonValueDetailed(tc.parameters); } catch { return { value: null, error: 'parse_failed', repairs: [] }; }
                        })();
                        if (parsed.value !== null && isRecord(parsed.value)) {
                            parameters = parsed.value;
                        }
                        else {
                            invalidReason = `parameters_json_invalid: ${formatJsonParseHint(parsed.error ?? 'expected_json_object')}`;
                        }
                    }
                    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for untrusted JSON
                    else if (tc.parameters !== undefined) {
                        invalidReason = `parameters_type_invalid: ${typeof tc.parameters}`;
                    }
                    if (invalidReason !== undefined) {
                        const payload = formatPayload(tc.parameters);
                        const reason = `tool=${normalizedName}; ${invalidReason}`;
                        dropped++;
                        logInvalidToolCall(reason, payload, normalizedName, id);
                        if (isFinalReportCall) {
                            this.ctx.finalReportManager.incrementAttempts();
                        }
                        const sanitized = buildSanitizationParams(reason, payload);
                        const failureMessage = `(tool failed: ${sanitized.failureDetail})`;
                        this.state.toolFailureMessages.set(id, failureMessage);
                        syntheticToolMessages.push({ role: 'tool', toolCallId: id, content: failureMessage });
                        calls.push({
                            name: normalizedName,
                            id,
                            parameters: sanitized.params,
                        });
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
        return { messages: sanitized, dropped, finalReportAttempted, syntheticToolMessages };
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
        if (toolsCount === 0)
            return undefined;
        const providerConfig = this.ctx.config.providers[provider];
        const modelChoice = providerConfig.models?.[model]?.toolChoice;
        if (modelChoice === 'auto' || modelChoice === 'required')
            return modelChoice;
        const providerChoice = providerConfig.toolChoice;
        if (providerChoice === 'auto' || providerChoice === 'required')
            return providerChoice;
        return undefined;
    }
    private resolveInterleaved(provider: string, model: string): boolean | string | undefined {
        const providerConfig = this.ctx.config.providers[provider];
        return providerConfig.models?.[model]?.interleaved;
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
        const overrideRepeatPenaltyCamel = overrides.repeatPenalty;
        if (overrideRepeatPenaltyCamel !== undefined) {
            result.repeatPenalty = overrideRepeatPenaltyCamel ?? null;
        }
        else {
            const overrideRepeatPenaltySnake = overrides.repeat_penalty;
            if (overrideRepeatPenaltySnake !== undefined) {
                result.repeatPenalty = overrideRepeatPenaltySnake ?? null;
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
        const callbackMeta = {
            agentId: this.ctx.agentId,
            callPath: this.ctx.callPath,
            sessionId: this.ctx.txnId,
            parentId: this.ctx.parentTxnId,
            originId: this.ctx.originTxnId,
        };
        const pendingSelection = this.state.pendingToolSelection;
        this.state.pendingToolSelection = undefined;
        const selection = pendingSelection ?? this.selectToolsForTurn(provider, isFinalTurn);
        const { toolsForTurn, availableTools, allowedToolNames } = selection;
        let disableReasoningForTurn = false;
        this.schemaCtxTokens = 0;
        const rawReasoningGuard = this.ctx.llmClient.shouldDisableReasoning(provider, {
            conversation,
            currentTurn,
            attempt,
            expectSignature: true,
        });
        const reasoningGuard = this.isReasoningGuard(rawReasoningGuard)
            ? rawReasoningGuard
            : { disable: false, normalized: conversation };
        if (reasoningGuard.disable) {
            conversation = reasoningGuard.normalized;
            disableReasoningForTurn = true;
        }
        addSpanAttributes({ 'ai.llm.reasoning.disabled': disableReasoningForTurn });
        let shownThinking = false;
        this.state.toolFailureMessages.clear();
        this.state.toolFailureFallbacks.length = 0;
        this.state.droppedInvalidToolCalls = 0;
        this.state.toolNameCorrections.clear();
        this.state.executedToolCallIds.clear();
        const executionState: ToolExecutionState = {
            toolFailureMessages: this.state.toolFailureMessages,
            toolFailureFallbacks: this.state.toolFailureFallbacks,
            toolNameCorrections: this.state.toolNameCorrections,
            trimmedToolCallIds: this.state.trimmedToolCallIds,
            executedToolCallIds: this.state.executedToolCallIds,
            turnHasSuccessfulToolOutput: false,
            incompleteFinalReportDetected: false,
            toolLimitExceeded: this.state.toolLimitExceeded,
            finalReportToolFailed: false,
            executedTools: 0,
            executedNonProgressBatchTools: 0,
            executedProgressBatchTools: 0,
            unknownToolEncountered: false,
            lastTaskStatusCompleted: this.state.lastTaskStatusCompleted,
            productiveToolExecutedThisTurn: false,
            onTaskCompletion: () => {
                // Force final turn immediately when task completion is detected
                this.ctx.contextGuard.setTaskCompletionReason();
            },
        };
        const baseExecutor = this.ctx.sessionExecutor.createExecutor(currentTurn, provider, executionState, allowedToolNames);
        let incompleteFinalReportDetected = false;
        const toolExecutor = async (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }): Promise<string> => {
            try {
                const result = await baseExecutor(toolName, parameters, options);
                incompleteFinalReportDetected = executionState.incompleteFinalReportDetected;
                this.state.toolLimitExceeded = executionState.toolLimitExceeded;
                // Sync task status state back to main state
                this.state.lastTaskStatusCompleted = executionState.lastTaskStatusCompleted;
                if (executionState.finalReportToolFailed) {
                    this.state.finalReportToolFailedThisTurn = true;
                    this.state.finalReportToolFailedEver = true;
                }
                return result;
            }
            catch (e) {
                incompleteFinalReportDetected = executionState.incompleteFinalReportDetected;
                this.state.toolLimitExceeded = executionState.toolLimitExceeded;
                // Sync task status state back to main state
                this.state.lastTaskStatusCompleted = executionState.lastTaskStatusCompleted;
                if (executionState.finalReportToolFailed) {
                    this.state.finalReportToolFailedThisTurn = true;
                    this.state.finalReportToolFailedEver = true;
                }
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
        let effectiveRepeatPenalty = this.ctx.sessionConfig.repeatPenalty;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime safety for dynamic key lookup
        if (modelOverrides.repeatPenalty !== undefined) {
            if (modelOverrides.repeatPenalty === null)
                effectiveRepeatPenalty = undefined;
            else
                effectiveRepeatPenalty = modelOverrides.repeatPenalty;
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
        if (this.ctx.sessionConfig.verbose === true) {
            const paramsLog: LogEntry = {
                timestamp: Date.now(),
                severity: 'VRB' as const,
                turn: currentTurn,
                subturn: 0,
                direction: 'request' as const,
                type: 'llm' as const,
                remoteIdentifier: 'agent:llm-params',
                fatal: false,
                message: JSON.stringify({
                    provider,
                    model,
                    temperature: effectiveTemperature ?? null,
                    topP: effectiveTopP ?? null,
                    topK: effectiveTopK ?? null,
                    repeatPenalty: effectiveRepeatPenalty ?? null,
                }),
            };
            this.log(paramsLog);
        }
        // Update context guard with current reasoning budget for accurate context evaluation
        this.ctx.contextGuard.setReasoningTokens(effectiveReasoningValue);
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
        const requestMessages = conversation;
        const requestMessagesBytes = estimateMessagesBytes(requestMessages);

        // Refresh schema token estimate right before issuing the LLM request so logs reflect the final tool set.
        const schemaToolsForRequest = toolsForTurn.length > 0 ? toolsForTurn : availableTools;
        this.schemaCtxTokens = this.estimateToolSchemaTokens(schemaToolsForRequest);
        // Pass reasoning value so context metrics include thinking budget tokens
        const metricsForRequest = this.ctx.contextGuard.buildMetrics(provider, model, effectiveReasoningValue);

        // Set currentCtxTokens to expected BEFORE the request, so tool budget reservation
        // knows about this turn's consumption. Will be overwritten with actual after response.
        // Schema is already in currentCtxTokens after first turn (via cache_write/cache_read).
        // IMPORTANT: Also zero out pending/new to avoid double-counting on retry if turn fails.
        this.currentCtxTokens = metricsForRequest.ctxTokens + metricsForRequest.newTokens;
        this.pendingCtxTokens = 0;
        this.newCtxTokens = 0;
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
        const llmTools = toolsForTurn;
        let xmlFilter: XmlFinalReportFilter | undefined;
        const thinkFilter = new ThinkTagStreamFilter();
        const nonce = this.ctx.xmlTransport.getNonce();
        if (nonce !== undefined) {
            xmlFilter = new XmlFinalReportFilter(nonce);
        }
        // Reset streaming flag for this attempt
        this.finalReportStreamed = false;
        this.resetStreamedOutputTail();

        const emitThinking = (thinkingChunk: string, isRootSession: boolean): void => {
            if (thinkingChunk.length === 0) return;
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
                this.callbacks.onThinking?.(thinkingChunk, callbackMeta);
            }
            try {
                const opId = this.callbacks.getCurrentLlmOpId();
                if (typeof opId === 'string')
                    this.ctx.opTree.appendReasoningChunk(opId, thinkingChunk);
            }
            catch (e) {
                warn(`appendReasoningChunk failed: ${e instanceof Error ? e.message : String(e)}`);
            }
        };

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
            repeatPenalty: effectiveRepeatPenalty,
            stream: effectiveStream,
            isFinalTurn,
            llmTimeout: this.ctx.sessionConfig.llmTimeout,
            toolChoice: this.resolveToolChoice(provider, model, llmTools.length),
            abortSignal: this.ctx.abortSignal,
            sendReasoning,
            interleaved: this.resolveInterleaved(provider, model),
            onChunk: (chunk: string, type: 'content' | 'thinking') => {
                const isRootSession = this.ctx.parentTxnId === undefined;
                if (type === 'content') {
                    const split = thinkFilter.process(chunk);
                    if (split.thinking.length > 0) {
                        emitThinking(split.thinking, isRootSession);
                    }
                    if (split.content.length === 0) return;
                    if (xmlFilter !== undefined) {
                        const filtered = xmlFilter.process(split.content);
                        if (filtered.length > 0 && this.callbacks.onOutput !== undefined) {
                            this.callbacks.onOutput(filtered, callbackMeta);
                            this.appendStreamedOutputTail(filtered);
                            // Only mark as streamed if we actually emitted content (and are inside the tag)
                            if (xmlFilter.hasStreamedContent) {
                                this.finalReportStreamed = true;
                            }
                        }
                    }
                    else if (this.callbacks.onOutput !== undefined) {
                        this.callbacks.onOutput(split.content, callbackMeta);
                        this.appendStreamedOutputTail(split.content);
                    }
                    return;
                }
                emitThinking(chunk, isRootSession);
            },
            reasoningLevel: effectiveReasoningLevel,
            reasoningValue: effectiveReasoningValue ?? null,
            turnMetadata,
            caching: this.ctx.sessionConfig.caching,
            contextMetrics: metricsForRequest,
        };
        const requestSnapshot = {
            provider,
            model,
            stream: effectiveStream,
            temperature: effectiveTemperature ?? null,
            topP: effectiveTopP ?? null,
            topK: effectiveTopK ?? null,
            maxOutputTokens: targetMaxOutputTokens ?? null,
            repeatPenalty: effectiveRepeatPenalty ?? null,
            reasoningLevel: effectiveReasoningLevel ?? null,
            reasoningValue: effectiveReasoningValue ?? null,
            interleaved: this.resolveInterleaved(provider, model) ?? null,
            sendReasoning: sendReasoning ?? null,
            toolChoice: request.toolChoice ?? null,
            tools: llmTools.map((tool) => ({
                name: tool.name,
                description: tool.description,
                inputSchema: tool.inputSchema,
            })),
            messages: requestMessages,
            turnMetadata,
        };
        this.captureSdkRequestSnapshot(this.callbacks.getCurrentLlmOpId(), requestSnapshot);
        this.debugLogRawConversation(provider, model, conversation);
        try {
            const result = await this.ctx.llmClient.executeTurn(request);
            const responseSnapshot = {
                status: result.status,
                stopReason: result.stopReason ?? null,
                response: result.response ?? null,
                messages: result.messages,
                hasReasoning: result.hasReasoning ?? false,
                hasContent: result.hasContent ?? false,
                tokens: result.tokens ?? null,
                providerMetadata: result.providerMetadata ?? null,
            };
            this.captureSdkResponseSnapshot(this.callbacks.getCurrentLlmOpId(), responseSnapshot);
            const tokens = result.tokens;
            const promptTokens = tokens?.inputTokens ?? 0;
            const completionTokens = tokens?.outputTokens ?? 0;
            const cacheReadTokens = tokens?.cacheReadInputTokens ?? tokens?.cachedTokens ?? 0;
            const cacheWriteTokens = tokens?.cacheWriteInputTokens ?? 0;
            const responseBytes = typeof result.response === 'string' && result.response.length > 0
                ? Buffer.byteLength(result.response, 'utf8')
                : estimateMessagesBytes(result.messages);
            if (result.status.type === 'success') {
                const toolResultIndexById = new Map<string, number>();
                result.messages.forEach((message, index) => {
                    if (message.role !== 'tool')
                        return;
                    const callId = message.toolCallId;
                    if (typeof callId !== 'string' || callId.length === 0)
                        return;
                    if (!toolResultIndexById.has(callId)) {
                        toolResultIndexById.set(callId, index);
                    }
                });
                const assistantToolCalls = result.messages
                    .filter((message) => message.role === 'assistant' && Array.isArray((message as { toolCalls?: ToolCall[] }).toolCalls))
                    .flatMap((message) => (message as { toolCalls?: ToolCall[] }).toolCalls ?? []);
                const normalizedCalls = assistantToolCalls.map((call) => ({
                    call,
                    normalized: sanitizeToolName(call.name),
                }));
                const hasFinalReportCall = normalizedCalls.some(({ normalized }) => normalized === FINAL_REPORT_TOOL
                    || FINAL_REPORT_TOOL_ALIASES.has(normalized)
                    || normalized === 'final_report');
                const shouldExecuteUnqualifiedCall = (call: ToolCall): boolean => {
                    if (this.state.toolNameCorrections.has(call.id)) {
                        return false;
                    }
                    if (this.state.executedToolCallIds.has(call.id)) {
                        return false;
                    }
                    return true;
                };
                const unqualifiedCalls = normalizedCalls
                    .filter(({ call }) => call.id.length > 0)
                    .filter(({ call }) => call.name.trim().length > 0)
                    .filter(({ call }) => !call.name.includes('__'))
                    .filter(({ normalized }) => !allowedToolNames.has(normalized))
                    .filter(({ normalized }) => !isXmlFinalReportTagName(normalized))
                    .filter(({ call }) => shouldExecuteUnqualifiedCall(call))
                    .filter(({ normalized }) => !hasFinalReportCall || normalized === FINAL_REPORT_TOOL
                    || FINAL_REPORT_TOOL_ALIASES.has(normalized)
                    || normalized === 'final_report')
                    .map(({ call }) => call);
                if (unqualifiedCalls.length > 0) {
                    const uniqueCalls = new Map<string, ToolCall>();
                    unqualifiedCalls.forEach((call) => {
                        if (!uniqueCalls.has(call.id)) {
                            uniqueCalls.set(call.id, call);
                        }
                    });
                    const extraToolResults: ConversationMessage[] = [];
                    // eslint-disable-next-line functional/no-loop-statements -- sequential execution preserves tool limits and ordering
                    for (const call of uniqueCalls.values()) {
                        const output = await toolExecutor(call.name, call.parameters, { toolCallId: call.id });
                        const toolMessage: ConversationMessage = { role: 'tool', toolCallId: call.id, content: output };
                        const existingIndex = toolResultIndexById.get(call.id);
                        if (existingIndex !== undefined) {
                            result.messages[existingIndex] = toolMessage;
                        }
                        else {
                            extraToolResults.push(toolMessage);
                        }
                    }
                    if (extraToolResults.length > 0) {
                        result.messages.push(...extraToolResults);
                    }
                }
            }
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
            const mergedExecutionStats = (() => {
                const baseStats = result.executionStats ?? {
                    executedTools: 0,
                    executedNonProgressBatchTools: 0,
                    executedProgressBatchTools: 0,
                    unknownToolEncountered: false,
                };
                return {
                    executedTools: baseStats.executedTools + executionState.executedTools,
                    executedNonProgressBatchTools: baseStats.executedNonProgressBatchTools + executionState.executedNonProgressBatchTools,
                    executedProgressBatchTools: baseStats.executedProgressBatchTools + executionState.executedProgressBatchTools,
                    unknownToolEncountered: (baseStats.unknownToolEncountered ?? false) || executionState.unknownToolEncountered,
                };
            })();
            return {
                ...result,
                shownThinking,
                incompleteFinalReportDetected,
                executionStats: mergedExecutionStats,
            };
        }
        catch (e) {
            throw e;
        }
            finally {
                if (xmlFilter !== undefined && this.callbacks.onOutput !== undefined) {
                    const left = xmlFilter.flush();
                    if (left.length > 0) {
                        this.callbacks.onOutput(left, {
                            agentId: this.ctx.agentId,
                            callPath: this.ctx.callPath,
                            sessionId: this.ctx.txnId,
                            parentId: this.ctx.parentTxnId,
                            originId: this.ctx.originTxnId,
                        });
                        this.appendStreamedOutputTail(left);
                        if (xmlFilter.hasStreamedContent) {
                            this.finalReportStreamed = true;
                        }
                    }
                }
        }
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
