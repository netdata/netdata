import crypto from 'node:crypto';

import type { TargetContextConfig } from '../context-guard.js';
import type { LLMClient } from '../llm-client.js';
import type {
  AccountingEntry,
  AIAgentSessionConfig,
  CachingMode,
  Configuration,
  ConversationMessage,
  MCPTool,
  ProviderReasoningValue,
  ReasoningLevel,
  TurnRequest,
  TurnResult,
} from '../types.js';
import type { ToolOutputConfig, ToolOutputExtractionResult, ToolOutputMode, ToolOutputTarget } from './types.js';

import { SessionTreeBuilder } from '../session-tree.js';
import { stripLeadingThinkBlock } from '../think-tag-filter.js';
import { TRUNCATE_PREVIEW_BYTES, buildTruncationPrefix, truncateToBytes, truncateToBytesWithInfo } from '../truncation.js';
import { estimateMessagesBytes } from '../utils.js';

import { computeChunkCount, splitIntoChunks } from './chunking.js';

interface ToolOutputExtractionContext {
  config: ToolOutputConfig;
  llmClient: LLMClient;
  targets: TargetContextConfig[];
  computeMaxOutputTokens: (contextWindow: number) => number;
  sessionTargets: ToolOutputTarget[];
  sessionNonce: string;
  sessionId: string;
  agentId?: string;
  callPath?: string;
  toolResponseMaxBytes?: number;
  temperature?: number | null;
  topP?: number | null;
  topK?: number | null;
  repeatPenalty?: number | null;
  maxOutputTokens?: number;
  reasoning?: ReasoningLevel;
  reasoningValue?: ProviderReasoningValue | null;
  llmTimeout?: number;
  toolTimeout?: number;
  caching?: CachingMode;
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSdk?: boolean;
  pricing?: Configuration['pricing'];
  countTokens: (value: string) => number;
  recordAccounting?: (entry: AccountingEntry) => void;
  fsRootDir: string;
  buildFsServerConfig: (rootDir: string) => { name: string; config: Configuration };
}

interface ExtractSource {
  handle: string;
  toolName: string;
  toolArgsJson: string;
  content: string;
  stats: { bytes: number; lines: number; tokens: number; avgLineBytes: number };
  sourceTarget?: ToolOutputTarget;
}

const NO_RELEVANT_DATA = 'NO RELEVANT DATA FOUND';
const SOURCE_TOOL_LABEL = 'SOURCE TOOL';
const WHAT_TO_EXTRACT_LABEL = 'WHAT TO EXTRACT';
const OUTPUT_FORMAT_LABEL = 'OUTPUT FORMAT (required)';
const OUTPUT_WRAPPER_LABEL = '- Emit exactly one XML final-report wrapper:';
const NO_RELEVANT_LABEL = '- If no relevant data exists, output:';
const MODE_AUTO: ToolOutputMode = 'auto';
const MODE_FULL_CHUNKED: Exclude<ToolOutputMode, 'auto'> = 'full-chunked';
const MODE_READ_GREP: Exclude<ToolOutputMode, 'auto'> = 'read-grep';
const MODE_TRUNCATE: Exclude<ToolOutputMode, 'auto'> = 'truncate';
const isNonAutoMode = (value: ToolOutputMode): value is Exclude<ToolOutputMode, 'auto'> => value !== MODE_AUTO;

const buildMapSystemPrompt = (args: {
  toolName: string;
  toolArgsJson: string;
  stats: ExtractSource['stats'];
  chunkIndex: number;
  chunkTotal: number;
  overlapPercent: number;
  extract: string;
  nonce: string;
}): string => {
  return [
    'You are an internal extraction agent. You must extract information from a document chunk.',
    '',
    SOURCE_TOOL_LABEL,
    `- Name: ${args.toolName}`,
    `- Arguments (verbatim JSON): ${args.toolArgsJson}`,
    '',
    'DOCUMENT STATS',
    `- Bytes: ${String(args.stats.bytes)}`,
    `- Lines: ${String(args.stats.lines)}`,
    `- Tokens (estimate): ${String(args.stats.tokens)}`,
    '',
    'CHUNK INFO',
    `- Index: ${String(args.chunkIndex)} of ${String(args.chunkTotal)}`,
    `- Overlap: ${String(args.overlapPercent)}%`,
    '',
    WHAT_TO_EXTRACT_LABEL,
    args.extract,
    '',
    OUTPUT_FORMAT_LABEL,
    OUTPUT_WRAPPER_LABEL,
    `  <ai-agent-${args.nonce}-FINAL format="text"> ... </ai-agent-${args.nonce}-FINAL>`,
    '- Put your extracted result inside the wrapper.',
    NO_RELEVANT_LABEL,
    `  ${NO_RELEVANT_DATA}`,
    '  <short description of what kind of information is available in this chunk>',
  ].join('\n');
};

const buildReduceSystemPrompt = (args: {
  toolName: string;
  toolArgsJson: string;
  extract: string;
  chunkOutputs: string;
  nonce: string;
}): string => {
  return [
    'You are an internal extraction agent. You must synthesize multiple chunk extractions into one final answer.',
    '',
    SOURCE_TOOL_LABEL,
    `- Name: ${args.toolName}`,
    `- Arguments (verbatim JSON): ${args.toolArgsJson}`,
    '',
    WHAT_TO_EXTRACT_LABEL,
    args.extract,
    '',
    'CHUNK OUTPUTS',
    args.chunkOutputs,
    '',
    OUTPUT_FORMAT_LABEL,
    OUTPUT_WRAPPER_LABEL,
    `  <ai-agent-${args.nonce}-FINAL format="text"> ... </ai-agent-${args.nonce}-FINAL>`,
    NO_RELEVANT_LABEL,
    `  ${NO_RELEVANT_DATA}`,
    '  <short description of what kind of information is available across the chunks>',
  ].join('\n');
};

const buildReadGrepSystemPrompt = (args: {
  toolName: string;
  toolArgsJson: string;
  handle: string;
  rootDir: string;
  extract: string;
  nonce: string;
}): string => {
  return [
    'You are an internal extraction agent. You can only use Read and Grep tools.',
    '',
    'IMPORTANT: The handle is the filename.',
    'Use the filename exactly as given when calling Read or Grep.',
    '',
    SOURCE_TOOL_LABEL,
    `- Name: ${args.toolName}`,
    `- Arguments (verbatim JSON): ${args.toolArgsJson}`,
    `- Handle/filename: ${args.handle}`,
    `- Root directory: ${args.rootDir}`,
    '',
    WHAT_TO_EXTRACT_LABEL,
    args.extract,
    '',
    OUTPUT_FORMAT_LABEL,
    OUTPUT_WRAPPER_LABEL,
    `  <ai-agent-${args.nonce}-FINAL format="text"> ... </ai-agent-${args.nonce}-FINAL>`,
    NO_RELEVANT_LABEL,
    `  ${NO_RELEVANT_DATA}`,
    '  <short description of what kind of information is available in the file>',
  ].join('\n');
};

const extractFinalContent = (nonce: string, raw: string): string | undefined => {
  const cleaned = stripLeadingThinkBlock(raw).stripped;
  const openTagPrefix = `<ai-agent-${nonce}-FINAL`;
  const openIdx = cleaned.indexOf(openTagPrefix);
  if (openIdx === -1) return undefined;
  const afterOpen = cleaned.slice(openIdx);
  const openMatch = /^<ai-agent-[A-Za-z0-9\-]+[^>]*>/.exec(afterOpen);
  if (openMatch === null) return undefined;
  const contentStart = openIdx + openMatch[0].length;
  const closeTag = `</ai-agent-${nonce}-FINAL>`;
  const closeIdx = cleaned.indexOf(closeTag, contentStart);
  if (closeIdx === -1) {
    return cleaned.slice(contentStart).trim();
  }
  return cleaned.slice(contentStart, closeIdx).trim();
};

const extractTextFromTurn = (result: TurnResult): string => {
  const lastAssistant = [...result.messages].filter((m) => m.role === 'assistant').pop();
  if (lastAssistant !== undefined && typeof lastAssistant.content === 'string') {
    return lastAssistant.content;
  }
  return result.response ?? '';
};

const computeCost = (
  pricing: Configuration['pricing'] | undefined,
  provider: string,
  model: string,
  tokens: { inputTokens: number; outputTokens: number; cacheReadInputTokens?: number; cacheWriteInputTokens?: number; cachedTokens?: number }
): { costUsd?: number } => {
  try {
    const table = pricing?.[provider]?.[model];
    if (table === undefined) return {};
    const denom = table.unit === 'per_1k' ? 1000 : 1_000_000;
    const cacheRead = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
    const cacheWrite = tokens.cacheWriteInputTokens ?? 0;
    const cost = (table.prompt ?? 0) * tokens.inputTokens
      + (table.completion ?? 0) * tokens.outputTokens
      + (table.cacheRead ?? 0) * cacheRead
      + (table.cacheWrite ?? 0) * cacheWrite;
    const normalized = cost / denom;
    return { costUsd: Number.isFinite(normalized) ? normalized : undefined };
  } catch {
    return {};
  }
};

export class ToolOutputExtractor {
  private readonly deps: ToolOutputExtractionContext;

  constructor(deps: ToolOutputExtractionContext) {
    this.deps = deps;
  }

  async extract(source: ExtractSource, extract: string, mode: ToolOutputMode | undefined, targets: ToolOutputTarget[]): Promise<ToolOutputExtractionResult> {
    const selectedMode = this.resolveMode(source, mode, targets);
    if (selectedMode === MODE_TRUNCATE) {
      const truncated = this.truncateResult(source.content, 'truncate');
      return { ok: true, text: truncated.text, mode: MODE_TRUNCATE, warning: truncated.warning };
    }
    if (selectedMode === MODE_READ_GREP) {
      const readGrepResult = await this.runReadGrep(source, extract, targets);
      if (readGrepResult.ok) return readGrepResult;
      const fallback = this.truncateResult(source.content, 'read-grep failed');
      return { ok: false, text: fallback.text, mode: MODE_TRUNCATE, warning: fallback.warning, childOpTree: readGrepResult.childOpTree };
    }
    const chunked = await this.runFullChunked(source, extract, targets);
    if (chunked.ok) return chunked;
    const fallback = this.truncateResult(source.content, 'full-chunked failed');
    return { ok: false, text: fallback.text, mode: MODE_TRUNCATE, warning: fallback.warning, childOpTree: chunked.childOpTree };
  }

  private resolveMode(source: ExtractSource, mode: ToolOutputMode | undefined, targets: ToolOutputTarget[]): Exclude<ToolOutputMode, 'auto'> {
    const override = mode ?? MODE_AUTO;
    if (isNonAutoMode(override)) return override;
    const maxChunks = Math.max(1, this.deps.config.maxChunks);
    const chunkBudget = this.computeChunkTokenBudget(source, targets);
    const chunkCount = computeChunkCount(source.stats.tokens, chunkBudget);
    if (source.stats.avgLineBytes >= this.deps.config.avgLineBytesThreshold) {
      return MODE_FULL_CHUNKED;
    }
    if (chunkCount <= maxChunks) {
      return MODE_FULL_CHUNKED;
    }
    return MODE_READ_GREP;
  }

  private computeChunkTokenBudget(source: ExtractSource, targets: ToolOutputTarget[]): number {
    const target = targets[0] ?? this.deps.sessionTargets[0];
    const context = this.lookupTargetContext(target);
    const maxOutputTokens = this.deps.computeMaxOutputTokens(context.contextWindow);
    const overheadTokens = this.estimatePromptOverheadTokens(source, this.deps.sessionNonce);
    const budget = context.contextWindow - context.bufferTokens - maxOutputTokens - overheadTokens;
    return Math.max(0, budget);
  }

  private estimatePromptOverheadTokens(source: ExtractSource, nonce: string): number {
    const system = buildMapSystemPrompt({
      toolName: source.toolName,
      toolArgsJson: source.toolArgsJson,
      stats: source.stats,
      chunkIndex: 1,
      chunkTotal: 1,
      overlapPercent: this.deps.config.overlapPercent,
      extract: '<extract>',
      nonce,
    });
    const overhead = `${system}\n\n<chunk>`;
    return this.deps.countTokens(overhead);
  }

  private lookupTargetContext(target: ToolOutputTarget): TargetContextConfig {
    const direct = this.deps.targets.find((t) => t.provider === target.provider && t.model === target.model);
    if (direct !== undefined) return direct;
    return this.deps.targets[0] ?? {
      provider: target.provider,
      model: target.model,
      contextWindow: 8192,
      bufferTokens: 512,
    };
  }

  private async runFullChunked(source: ExtractSource, extract: string, targets: ToolOutputTarget[]): Promise<ToolOutputExtractionResult> {
    const child = new SessionTreeBuilder({
      traceId: crypto.randomUUID(),
      agentId: this.deps.agentId ?? 'tool_output',
      callPath: this.deps.callPath,
      sessionTitle: `tool_output:${source.handle}`,
    });
    child.beginTurn(1, { mode: MODE_FULL_CHUNKED });
    const endFailure = (message: string): ToolOutputExtractionResult => {
      try { child.endTurn(1); } catch { /* ignore double-end */ }
      child.endSession(false, message);
      return { ok: false, text: message, mode: MODE_FULL_CHUNKED, childOpTree: child.getSession() };
    };

    const target = targets[0] ?? this.deps.sessionTargets[0];
    const chunkBudget = this.computeChunkTokenBudget(source, targets);
    const totalTokens = source.stats.tokens;
    const chunkCount = computeChunkCount(totalTokens, chunkBudget);
    if (chunkCount > this.deps.config.maxChunks) {
      return endFailure('full-chunked requires too many chunks for configured maxChunks');
    }

    const bytesPerToken = totalTokens > 0 ? source.stats.bytes / totalTokens : 4;
    const chunkBytes = Math.max(512, Math.floor(chunkBudget * bytesPerToken));
    const chunks = splitIntoChunks(source.content, chunkBytes, this.deps.config.overlapPercent);
    if (chunks.length === 0) {
      return endFailure('full-chunked received empty content');
    }

    const chunkOutputs: string[] = [];
    // eslint-disable-next-line functional/no-loop-statements -- sequential map
    for (const chunk of chunks) {
      const opId = child.beginOp(1, 'llm', { provider: target.provider, model: target.model, chunk: chunk.index + 1 });
      const system = buildMapSystemPrompt({
        toolName: source.toolName,
        toolArgsJson: source.toolArgsJson,
        stats: source.stats,
        chunkIndex: chunk.index + 1,
        chunkTotal: chunks.length,
        overlapPercent: this.deps.config.overlapPercent,
        extract,
        nonce: this.deps.sessionNonce,
      });
      const messages: ConversationMessage[] = [
        { role: 'system', content: system },
        { role: 'user', content: chunk.text },
      ];
      const turnRequest = this.buildLlmRequest(messages, target);
      const result = await this.deps.llmClient.executeTurn(turnRequest);
      const text = extractTextFromTurn(result);
      const extracted = extractFinalContent(this.deps.sessionNonce, text);
      this.recordLlmOp(child, opId, target, result, messages, text);
      if (extracted === undefined) {
        return endFailure('full-chunked map failed to emit final report');
      }
      chunkOutputs.push(extracted);
    }

    const reduceOp = child.beginOp(1, 'llm', { provider: target.provider, model: target.model, stage: 'reduce' });
    const reduceSystem = buildReduceSystemPrompt({
      toolName: source.toolName,
      toolArgsJson: source.toolArgsJson,
      extract,
      chunkOutputs: chunkOutputs.map((out, idx) => `# Chunk ${String(idx + 1)}\n${out}`).join('\n\n'),
      nonce: this.deps.sessionNonce,
    });
    const reduceMessages: ConversationMessage[] = [
      { role: 'system', content: reduceSystem },
      { role: 'user', content: 'Synthesize the extracted content into a final answer.' },
    ];
    const reduceRequest = this.buildLlmRequest(reduceMessages, target);
    const reduceResult = await this.deps.llmClient.executeTurn(reduceRequest);
    const reduceText = extractTextFromTurn(reduceResult);
    const reduced = extractFinalContent(this.deps.sessionNonce, reduceText);
    this.recordLlmOp(child, reduceOp, target, reduceResult, reduceMessages, reduceText);
    child.endTurn(1);
    if (reduced === undefined) {
      child.endSession(false, 'full-chunked reduce failed to emit final report');
      return { ok: false, text: 'full-chunked reduce failed to emit final report', mode: MODE_FULL_CHUNKED, childOpTree: child.getSession() };
    }

    child.endSession(true);
    return { ok: true, text: reduced, mode: MODE_FULL_CHUNKED, childOpTree: child.getSession() };
  }

  private async runReadGrep(source: ExtractSource, extract: string, targets: ToolOutputTarget[]): Promise<ToolOutputExtractionResult> {
    const { name, config } = this.deps.buildFsServerConfig(this.deps.fsRootDir);
    const { AIAgent } = await import('../ai-agent.js');
    const systemPrompt = buildReadGrepSystemPrompt({
      toolName: source.toolName,
      toolArgsJson: source.toolArgsJson,
      handle: source.handle,
      rootDir: this.deps.fsRootDir,
      extract,
      nonce: this.deps.sessionNonce,
    });
    const childConfig: AIAgentSessionConfig = {
      config,
      targets: targets.length > 0 ? targets : this.deps.sessionTargets,
      tools: [name],
      systemPrompt,
      userPrompt: extract,
      outputFormat: 'pipe',
      maxTurns: 3,
      maxToolCallsPerTurn: 6,
      llmTimeout: this.deps.llmTimeout,
      toolTimeout: this.deps.toolTimeout,
      temperature: this.deps.temperature,
      topP: this.deps.topP,
      topK: this.deps.topK,
      repeatPenalty: this.deps.repeatPenalty,
      maxOutputTokens: this.deps.maxOutputTokens,
      reasoning: this.deps.reasoning,
      reasoningValue: this.deps.reasoningValue,
      caching: this.deps.caching,
      traceLLM: this.deps.traceLLM,
      traceMCP: this.deps.traceMCP,
      traceSdk: this.deps.traceSdk,
      verbose: false,
      agentId: 'tool_output.read_grep',
      toolResponseMaxBytes: this.deps.toolResponseMaxBytes,
      toolOutput: { enabled: false },
      callbacks: this.deps.recordAccounting !== undefined
        ? {
            onEvent: (event) => {
              if (event.type === 'accounting') {
                this.deps.recordAccounting?.(event.entry);
              }
            },
          }
        : undefined,
      headendId: 'tool_output',
      headendWantsProgressUpdates: false,
      isMaster: false,
    };

    const session = AIAgent.create(childConfig);
    const result = await AIAgent.run(session);
    const childOpTree = result.opTree;
    const content = result.finalReport?.content ?? '';
    if (!result.success || content.trim().length === 0) {
      const message = result.error ?? 'read-grep extraction failed';
      return { ok: false, text: message, mode: MODE_READ_GREP, childOpTree };
    }
    const isSynthetic = (result as { finalReportSource?: string }).finalReportSource === 'synthetic';
    if (isSynthetic) {
      return { ok: false, text: 'read-grep extraction produced synthetic final report', mode: MODE_READ_GREP, childOpTree };
    }
    return { ok: true, text: content.trim(), mode: MODE_READ_GREP, childOpTree };
  }

  private truncateResult(content: string, reason: string): { text: string; warning: string } {
    const warning = `WARNING: tool_output fallback to truncate (${reason}).`;
    const limit = this.deps.toolResponseMaxBytes;
    if (typeof limit !== 'number' || limit <= 0) {
      return { text: content, warning };
    }
    const prefixBytes = Buffer.byteLength(`${warning}\n`, 'utf8');
    const budget = limit - prefixBytes;
    if (budget <= 0) {
      return { text: '', warning };
    }
    const truncated = truncateToBytesWithInfo(content, budget);
    if (truncated?.truncated === true) {
      const marker = buildTruncationPrefix(truncated.omitted, 'bytes');
      return { text: `${marker}\n${truncated.value}`, warning };
    }
    return { text: content, warning };
  }

  private buildLlmRequest(messages: ConversationMessage[], target: ToolOutputTarget): TurnRequest {
    const toolExecutor = () => Promise.reject(new Error('tool_output extraction does not support tool calls'));
    return {
      messages,
      provider: target.provider,
      model: target.model,
      tools: [] as MCPTool[],
      toolExecutor,
      temperature: this.deps.temperature,
      topP: this.deps.topP,
      topK: this.deps.topK,
      maxOutputTokens: this.deps.maxOutputTokens,
      repeatPenalty: this.deps.repeatPenalty,
      stream: false,
      llmTimeout: this.deps.llmTimeout,
      reasoningLevel: this.deps.reasoning,
      reasoningValue: this.deps.reasoningValue,
      caching: this.deps.caching,
      sdkTrace: this.deps.traceSdk,
    };
  }

  private recordLlmOp(
    child: SessionTreeBuilder,
    opId: string,
    target: ToolOutputTarget,
    result: TurnResult,
    messages: ConversationMessage[],
    rawText: string,
  ): void {
    const tokens = result.tokens ?? { inputTokens: 0, outputTokens: 0, cachedTokens: 0, totalTokens: 0 };
    try {
      const cacheRead = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
      const cacheWrite = tokens.cacheWriteInputTokens ?? 0;
      const total = tokens.inputTokens + tokens.outputTokens + cacheRead + cacheWrite;
      tokens.totalTokens = Number.isFinite(total) ? total : tokens.totalTokens;
    } catch {
      // keep provider tokens
    }
    const meta = result.providerMetadata;
    const cost = computeCost(this.deps.pricing, meta?.actualProvider ?? target.provider, meta?.actualModel ?? target.model, tokens);
    const accounting: AccountingEntry = {
      type: 'llm',
      timestamp: Date.now(),
      status: result.status.type === 'success' ? 'ok' : 'failed',
      latency: result.latencyMs,
      provider: target.provider,
      model: target.model,
      actualProvider: meta?.actualProvider,
      actualModel: meta?.actualModel,
      costUsd: meta?.reportedCostUsd ?? cost.costUsd,
      upstreamInferenceCostUsd: meta?.upstreamCostUsd,
      stopReason: result.stopReason,
      tokens,
      error: result.status.type === 'success' ? undefined : ('message' in result.status ? result.status.message : result.status.type),
      agentId: this.deps.agentId,
      callPath: this.deps.callPath,
      txnId: this.deps.sessionId,
    };
    try {
      this.deps.recordAccounting?.(accounting);
    } catch {
      // accounting relay failures should not fail extraction
    }
    child.appendAccounting(opId, accounting);
    const messageBytes = estimateMessagesBytes(messages);
    child.setRequest(opId, { kind: 'llm', payload: { messages: messages.length, bytes: messageBytes }, size: messageBytes });
    const preview = truncateToBytes(rawText, TRUNCATE_PREVIEW_BYTES);
    const previewText = preview ?? rawText;
    const truncated = preview !== undefined && preview !== rawText;
    const responseBytes = rawText.length > 0 ? Buffer.byteLength(rawText, 'utf8') : 0;
    child.setResponse(opId, { payload: { textPreview: previewText }, size: responseBytes, truncated });
    child.endOp(opId, result.status.type === 'success' ? 'ok' : 'failed', { latency: result.latencyMs });
  }
}
