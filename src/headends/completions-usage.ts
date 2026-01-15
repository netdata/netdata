import type { AccountingEntry, LLMAccountingEntry } from '../types.js';

export interface LlmUsageTotals {
  input: number;
  output: number;
  total: number;
}

export const collectLlmUsage = (entries: AccountingEntry[]): LlmUsageTotals => {
  const usage = entries
    .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
    .reduce<{ input: number; output: number }>((acc, entry) => {
      acc.input += entry.tokens.inputTokens;
      acc.output += entry.tokens.outputTokens;
      return acc;
    }, { input: 0, output: 0 });
  return { ...usage, total: usage.input + usage.output };
};
