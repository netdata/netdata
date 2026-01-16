/**
 * Fallback Tool Extraction and Execution from Leaked XML-like Content
 *
 * This module handles models that emit tool calls as XML-like patterns in text
 * instead of using native tool calling. It extracts, executes, and returns results.
 *
 * Supported patterns:
 * - <tool_calls>...</tool_calls>
 * - <tool_call>...</tool_call>
 * - <tools>...</tools>
 * - <function_call>...</function_call>
 * - <function>...</function>
 *
 * The module is self-contained with minimal dependencies on session state.
 */

import crypto from 'node:crypto';

import type { ConversationMessage, ToolCall } from './types.js';

import { isPlainObject, parseJsonValueDetailed, sanitizeToolName } from './utils.js';

// =============================================================================
// Types for the execution interface
// =============================================================================

/**
 * Function signature for executing a tool call.
 */
export type ToolExecutorFn = (
  toolName: string,
  parameters: Record<string, unknown>,
  options?: { toolCallId?: string }
) => Promise<string>;

/**
 * Execution state reference for tracking stats.
 */
export interface ExecutionStatsRef {
  executedTools: number;
  executedNonProgressBatchTools: number;
  executedProgressBatchTools: number;
  unknownToolEncountered: boolean;
}

/**
 * Input for processing leaked tool calls.
 */
export interface LeakedToolFallbackInput {
  /** The text content to scan for leaked tool calls */
  textContent: string;
  /** Function to execute tools - provided by the session */
  executor: ToolExecutorFn;
  /** Reference to execution state for reading stats after execution */
  executionStatsRef: ExecutionStatsRef;
  /** Optional allowlist of tools permitted for this turn */
  allowedToolNames?: Set<string>;
}

/**
 * Execution statistics from leaked tool call processing.
 */
export interface LeakedToolExecutionStats {
  executedTools: number;
  executedNonProgressBatchTools: number;
  executedProgressBatchTools: number;
  unknownToolEncountered: boolean;
}

/**
 * Result of processing leaked tool calls.
 */
export interface LeakedToolFallbackResult {
  /** Cleaned content after extraction (null if nothing remains) */
  cleanedContent: string | null;
  /** Whether there's meaningful text remaining after extraction */
  hasRemainingText: boolean;
  /** Extracted tool calls that were executed */
  toolCalls: ToolCall[];
  /** Tool result messages ready to add to conversation */
  toolResults: ConversationMessage[];
  /** Pattern names that matched (for logging) */
  patternsMatched: string[];
  /** Execution statistics from the fallback execution */
  executionStats: LeakedToolExecutionStats;
}

/**
 * Result of attempting to extract leaked tool calls from content.
 */
export interface LeakedToolExtractionResult {
  /** Cleaned content after removing XML tags, or null if all content was XML */
  content: string | null;
  /** Extracted tool calls, empty array if none found */
  toolCalls: ToolCall[];
  /** Names of tag patterns that matched (e.g., ['tools', 'tool_call']) */
  patternsMatched: string[];
}

/**
 * Tag patterns to search for, in order of preference.
 * Each pattern has an opening and closing tag.
 */
const TAG_PATTERNS: readonly { name: string; open: RegExp; close: string }[] = [
  { name: 'tool_calls', open: /<tool_calls>/gi, close: '</tool_calls>' },
  { name: 'tool_call', open: /<tool_call>/gi, close: '</tool_call>' },
  { name: 'tools', open: /<tools>/gi, close: '</tools>' },
  { name: 'function_call', open: /<function_call>/gi, close: '</function_call>' },
  { name: 'function', open: /<function>/gi, close: '</function>' },
];

const TASK_STATUS_FIELDS = new Set([
  'status',
  'done',
  'pending',
  'now',
  'ready_for_final_report',
  'need_to_run_more_tools',
]);

const escapeRegExp = (value: string): string => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const normalizeXmlValue = (raw: string): string | boolean | undefined => {
  const trimmed = raw.trim();
  if (trimmed.length === 0) return undefined;
  const lower = trimmed.toLowerCase();
  if (lower === 'true') return true;
  if (lower === 'false') return false;
  return trimmed;
};

const mergeParamValue = (
  target: Record<string, unknown>,
  name: string,
  value: string | boolean
): void => {
  const existing = target[name];
  if (typeof existing === 'string' && typeof value === 'string') {
    target[name] = existing.length > 0 ? `${existing}\n${value}` : value;
    return;
  }
  target[name] = value;
};

const parseXmlParameters = (content: string, toolName: string): Record<string, unknown> => {
  const parameters: Record<string, unknown> = {};
  const paramPattern = /<parameter\s+name\s*=\s*"([^"]+)"\s*>([\s\S]*?)<\/(?:parameter|\1)\s*>/gi;

  // eslint-disable-next-line functional/no-loop-statements -- parsing XML-like segments
  for (const match of content.matchAll(paramPattern)) {
    const rawName = match[1].trim();
    if (rawName.length === 0) continue;
    const normalized = normalizeXmlValue(match[2]);
    if (normalized === undefined) continue;
    mergeParamValue(parameters, rawName, normalized);
  }

  const stripped = content.replace(paramPattern, '');
  const isTaskStatusTool = sanitizeToolName(toolName).toLowerCase() === 'agent__task_status';
  if (!isTaskStatusTool) {
    return parameters;
  }

  const barePattern = /<([A-Za-z0-9_\-]+)"?\s*>([\s\S]*?)<\/\1\s*>/gi;
  // eslint-disable-next-line functional/no-loop-statements -- parsing XML-like segments
  for (const match of stripped.matchAll(barePattern)) {
    const rawName = match[1].trim();
    if (rawName.length === 0) continue;
    if (!TASK_STATUS_FIELDS.has(rawName)) continue;
    const normalized = normalizeXmlValue(match[2]);
    if (normalized === undefined) continue;
    mergeParamValue(parameters, rawName, normalized);
  }

  return parameters;
};

const extractToolTagOccurrences = (
  content: string,
  toolName: string
): { extracted: string[]; remaining: string } => {
  const extracted: string[] = [];
  let remaining = content;
  const escaped = escapeRegExp(toolName);
  const open = new RegExp(`<${escaped}\\s*>`, 'gi');
  const close = new RegExp(`</${escaped}\\s*>`, 'i');

  open.lastIndex = 0;
  // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition -- intentional loop with break
  while (true) {
    const openMatch = open.exec(remaining);
    if (openMatch === null) break;
    const openEnd = openMatch.index + openMatch[0].length;
    const closeMatch = remaining.slice(openEnd).match(close);
    if (closeMatch?.index === undefined) break;
    const closeIndex = openEnd + closeMatch.index;
    const innerContent = remaining.slice(openEnd, closeIndex).trim();
    if (innerContent.length > 0) {
      extracted.push(innerContent);
    }
    remaining = remaining.slice(0, openMatch.index) + remaining.slice(closeIndex + closeMatch[0].length);
    open.lastIndex = 0;
  }

  return { extracted, remaining };
};

const parseToolNameTagToolCall = (toolName: string, content: string): ToolCall | undefined => {
  const parameters = parseXmlParameters(content, toolName);
  if (Object.keys(parameters).length === 0) {
    return undefined;
  }
  return {
    id: crypto.randomUUID(),
    name: sanitizeToolName(toolName),
    parameters,
  };
};

/**
 * Normalize a parsed JSON object into a ToolCall.
 * Handles field name variations: function→name, tool→name, arguments→parameters.
 */
const normalizeToolCall = (obj: Record<string, unknown>): ToolCall | undefined => {
  // Extract tool name from various field names
  const nameCandidate = obj.name ?? obj.function ?? obj.tool;
  if (typeof nameCandidate !== 'string' || nameCandidate.trim().length === 0) {
    return undefined;
  }

  // Extract parameters from various field names
  const paramsCandidate = obj.parameters ?? obj.arguments ?? {};
  const parameters = isPlainObject(paramsCandidate) ? paramsCandidate : {};

  return {
    id: crypto.randomUUID(),
    name: sanitizeToolName(nameCandidate),
    parameters,
  };
};

/**
 * Parse JSON content and extract tool calls from it.
 * Handles single objects, arrays, and batch format.
 */
const parseToolCalls = (jsonContent: string): ToolCall[] => {
  const result = parseJsonValueDetailed(jsonContent);
  if (result.value === undefined) {
    return [];
  }

  const tools: ToolCall[] = [];

  // Handle array of tool calls
  if (Array.isArray(result.value)) {
    // eslint-disable-next-line functional/no-loop-statements -- iterating to normalize and filter
    for (const item of result.value) {
      if (isPlainObject(item)) {
        const normalized = normalizeToolCall(item);
        if (normalized !== undefined) {
          tools.push(normalized);
        }
      }
    }
    return tools;
  }

  // Handle single object
  if (isPlainObject(result.value)) {
    const normalized = normalizeToolCall(result.value);
    if (normalized !== undefined) {
      tools.push(normalized);
    }
  }

  return tools;
};

/**
 * Extract all occurrences of a specific tag pattern from content.
 * Returns the extracted JSON contents and the content with tags removed.
 */
const extractTagOccurrences = (
  content: string,
  pattern: { name: string; open: RegExp; close: string }
): { extracted: string[]; remaining: string } => {
  const extracted: string[] = [];
  let remaining = content;

  // Reset regex state
  pattern.open.lastIndex = 0;

  // Find all opening tags and their matching closing tags
  // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition -- intentional infinite loop with break
  while (true) {
    const openMatch = pattern.open.exec(remaining);
    if (openMatch === null) break;

    const openEnd = openMatch.index + openMatch[0].length;
    const closeIndex = remaining.toLowerCase().indexOf(pattern.close.toLowerCase(), openEnd);

    if (closeIndex === -1) {
      // No closing tag found, skip this opening tag
      break;
    }

    // Extract content between tags
    const innerContent = remaining.slice(openEnd, closeIndex).trim();
    if (innerContent.length > 0) {
      extracted.push(innerContent);
    }

    // Remove the entire tag from remaining content
    remaining = remaining.slice(0, openMatch.index) + remaining.slice(closeIndex + pattern.close.length);

    // Reset regex for next search on modified string
    pattern.open.lastIndex = 0;
  }

  return { extracted, remaining };
};

/**
 * Extract tool calls from leaked XML-like content.
 *
 * @param input - Raw content string from assistant message
 * @returns Object with cleaned content and extracted tool calls
 *
 * Valid states:
 * - content === input, toolCalls = [] : Nothing found, content unchanged
 * - content = "remaining", toolCalls = [...] : Extracted tools, some content remains
 * - content = null, toolCalls = [...] : Extracted tools, no content left
 */
export const tryExtractLeakedToolCalls = (
  input: string,
  options?: { allowedToolNames?: Set<string> }
): LeakedToolExtractionResult => {
  if (typeof input !== 'string') {
    return { content: input, toolCalls: [], patternsMatched: [] };
  }

  const allToolCalls: ToolCall[] = [];
  const patternsMatched: string[] = [];
  let workingContent = input;
  const allowedToolNames = options?.allowedToolNames;

  // Try each pattern
  // eslint-disable-next-line functional/no-loop-statements -- iterating over patterns
  for (const pattern of TAG_PATTERNS) {
    const { extracted, remaining } = extractTagOccurrences(workingContent, pattern);

    if (extracted.length > 0) {
      patternsMatched.push(pattern.name);
      workingContent = remaining;

      // Parse each extracted JSON block
      // eslint-disable-next-line functional/no-loop-statements -- iterating to parse extracted blocks
      for (const jsonContent of extracted) {
        const tools = parseToolCalls(jsonContent);
        allToolCalls.push(...tools);
      }
    }
  }

  if (allowedToolNames !== undefined && allowedToolNames.size > 0) {
    // eslint-disable-next-line functional/no-loop-statements -- iterating over allowed tools
    for (const toolName of allowedToolNames) {
      const { extracted, remaining } = extractToolTagOccurrences(workingContent, toolName);
      if (extracted.length > 0) {
        patternsMatched.push(toolName);
        workingContent = remaining;
        // eslint-disable-next-line functional/no-loop-statements -- iterating to parse extracted blocks
        for (const xmlContent of extracted) {
          const toolCall = parseToolNameTagToolCall(toolName, xmlContent);
          if (toolCall !== undefined) {
            allToolCalls.push(toolCall);
          }
        }
      }
    }
  }

  // If no patterns were found, return input unchanged
  if (patternsMatched.length === 0) {
    return { content: input, toolCalls: [], patternsMatched: [] };
  }

  // Clean up remaining content
  const cleanedContent = workingContent.trim();

  return {
    content: cleanedContent.length > 0 ? cleanedContent : null,
    toolCalls: allToolCalls,
    patternsMatched,
  };
};

// =============================================================================
// Main entry point: Extract and Execute leaked tool calls
// =============================================================================

/**
 * Extract leaked tool calls from content, execute them, and return results.
 *
 * This is the main entry point for the fallback mechanism. It:
 * 1. Scans text for XML-like tool call patterns
 * 2. Extracts and parses tool calls
 * 3. Executes each tool using the provided executor
 * 4. Returns tool results as conversation messages
 *
 * @param input - Text content and executor function
 * @returns Result with tool calls and results, or undefined if nothing found
 */
export const processLeakedToolCalls = async (
  input: LeakedToolFallbackInput
): Promise<LeakedToolFallbackResult | undefined> => {
  const { textContent, executor, executionStatsRef } = input;

  // Step 1: Extract tool calls from text
  const extraction = tryExtractLeakedToolCalls(textContent, {
    allowedToolNames: input.allowedToolNames,
  });

  // No tool calls found - nothing to do
  if (extraction.toolCalls.length === 0) {
    return undefined;
  }

  // Step 2: Execute each tool call and collect results
  const toolResults: ConversationMessage[] = [];

  // eslint-disable-next-line functional/no-loop-statements -- sequential execution required
  for (const toolCall of extraction.toolCalls) {
    try {
      const result = await executor(toolCall.name, toolCall.parameters, { toolCallId: toolCall.id });
      toolResults.push({
        role: 'tool',
        content: result,
        toolCallId: toolCall.id,
      });
    } catch (error) {
      // Tool execution failed - add error as result
      const errorMessage = error instanceof Error ? error.message : String(error);
      toolResults.push({
        role: 'tool',
        content: `(tool failed: ${errorMessage})`,
        toolCallId: toolCall.id,
      });
    }
  }

  // Step 3: Return complete result with execution stats from the state reference
  const hasRemainingText = extraction.content !== null && extraction.content.length > 0;

  return {
    cleanedContent: extraction.content,
    hasRemainingText,
    toolCalls: extraction.toolCalls,
    toolResults,
    patternsMatched: extraction.patternsMatched,
    executionStats: {
      executedTools: executionStatsRef.executedTools,
      executedNonProgressBatchTools: executionStatsRef.executedNonProgressBatchTools,
      executedProgressBatchTools: executionStatsRef.executedProgressBatchTools,
      unknownToolEncountered: executionStatsRef.unknownToolEncountered,
    },
  };
};
