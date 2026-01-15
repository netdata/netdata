import { countTokens as anthropicCountTokens } from '@anthropic-ai/tokenizer';
import { encoding_for_model, get_encoding } from '@dqbd/tiktoken';
import { fromPreTrained as createGeminiTokenizer } from '@lenml/tokenizer-gemini';

import type { ConversationMessage } from './types.js';
import type { TiktokenModel } from '@dqbd/tiktoken';

export interface Tokenizer {
  countText: (text: string) => number;
}

const APPROXIMATE_ID = 'approximate';
const MESSAGE_OVERHEAD_TOKENS = 4;
const TOOL_CALL_OVERHEAD_TOKENS = 2;

interface Encoding { encode: (input: string) => Uint32Array }

interface GeminiTokenizerInstance { encode: (input: string) => number[] }

const tokenizerCache = new Map<string, Tokenizer>();

const approximateTokenizer: Tokenizer = {
  countText: (text: string): number => {
    if (text.length === 0) return 0;
    // Rough heuristic: 4 characters ≈ 1 token, clamp to at least 1.
    return Math.max(1, Math.ceil(text.length / 4));
  },
};

const anthropicTokenizer: Tokenizer = {
  countText: (text: string): number => {
    if (text.length === 0) return 0;
    try {
      return anthropicCountTokens(text);
    } catch {
      return approximateTokenizer.countText(text);
    }
  },
};

let geminiTokenizerInstance: GeminiTokenizerInstance | undefined;

const geminiTokenizer: Tokenizer = {
  countText: (text: string): number => {
    if (text.length === 0) return 0;
    try {
      geminiTokenizerInstance ??= createGeminiTokenizer();
      const encoded = geminiTokenizerInstance.encode(text);
      return Array.isArray(encoded) ? encoded.length : approximateTokenizer.countText(text);
    } catch {
      return approximateTokenizer.countText(text);
    }
  },
};

const isEncoding = (value: unknown): value is Encoding => (
  value !== null
  && typeof value === 'object'
  && typeof (value as { encode?: unknown }).encode === 'function'
);

const getEncodingForModel = (model: string): Encoding | undefined => {
  try {
    const directCandidate: unknown = encoding_for_model(model as TiktokenModel);
    if (isEncoding(directCandidate)) {
      return directCandidate;
    }
  } catch {
    // ignore and fall back
  }
  try {
    const fallbackCandidate: unknown = get_encoding('cl100k_base');
    if (isEncoding(fallbackCandidate)) {
      return fallbackCandidate;
    }
  } catch {
    // ignore and defer to approximation
  }
  return undefined;
};

function createTiktokenTokenizer(model: string): Tokenizer {
  const encoding = getEncodingForModel(model);
  if (encoding === undefined) {
    return approximateTokenizer;
  }
  return {
    countText: (text: string): number => {
      if (text.length === 0) return 0;
      return encoding.encode(text).length;
    },
  };
}

function createTokenizer(id?: string): Tokenizer {
  const normalized = id?.trim() ?? '';
  if (normalized.length === 0) {
    return approximateTokenizer;
  }
  const lowerNormalized = normalized.toLowerCase();
  if (lowerNormalized === APPROXIMATE_ID) {
    return approximateTokenizer;
  }
  const cached = tokenizerCache.get(normalized);
  if (cached !== undefined) {
    return cached;
  }
  if (lowerNormalized.startsWith('tiktoken:')) {
    const model = normalized.slice('tiktoken:'.length).trim();
    const selectedModel = (model.length === 0 ? undefined : model) ?? 'gpt-4o';
    const tokenizer = createTiktokenTokenizer(selectedModel);
    tokenizerCache.set(normalized, tokenizer);
    return tokenizer;
  }
  if (lowerNormalized.startsWith('anthropic') || lowerNormalized.startsWith('claude')) {
    tokenizerCache.set(normalized, anthropicTokenizer);
    return anthropicTokenizer;
  }
  if (
    lowerNormalized.startsWith('gemini')
    || lowerNormalized.startsWith('google:gemini')
    || lowerNormalized.startsWith('google-gemini')
  ) {
    tokenizerCache.set(normalized, geminiTokenizer);
    return geminiTokenizer;
  }
  tokenizerCache.set(normalized, approximateTokenizer);
  return approximateTokenizer;
}

export function resolveTokenizer(id?: string): Tokenizer {
  let cacheKey = id ?? APPROXIMATE_ID;
  if (cacheKey.length === 0) {
    cacheKey = APPROXIMATE_ID;
  }
  const cached = tokenizerCache.get(cacheKey);
  if (cached !== undefined) {
    return cached;
  }
  const tokenizer = createTokenizer(id);
  tokenizerCache.set(cacheKey, tokenizer);
  return tokenizer;
}

function serializeMessage(message: ConversationMessage): string {
  const parts: string[] = [`role:${message.role}`];
  if (typeof message.content === 'string' && message.content.length > 0) {
    parts.push(message.content);
  }
  if (Array.isArray(message.toolCalls) && message.toolCalls.length > 0) {
    try {
      parts.push(JSON.stringify(message.toolCalls));
    } catch {
      parts.push('[toolCalls]');
    }
  }
  if (typeof message.toolCallId === 'string' && message.toolCallId.length > 0) {
    parts.push(`toolCallId:${message.toolCallId}`);
  }
  return parts.join('\n');
}

export function estimateMessageTokens(tokenizer: Tokenizer, message: ConversationMessage): number {
  const base = serializeMessage(message);
  const toolOverhead = Array.isArray(message.toolCalls) && message.toolCalls.length > 0
    ? TOOL_CALL_OVERHEAD_TOKENS * message.toolCalls.length
    : 0;
  return tokenizer.countText(base) + MESSAGE_OVERHEAD_TOKENS + toolOverhead;
}

export function estimateMessagesTokens(tokenizer: Tokenizer, messages: readonly ConversationMessage[]): number {
  return messages.reduce((total, message) => total + estimateMessageTokens(tokenizer, message), 0);
}
