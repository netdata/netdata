import { escapeMarkdown } from './summary-utils.js';

export interface ReasoningTurnState {
  index: number;
  attempt?: number;
  reason?: string;
  summary?: string;
  thinking?: string;
  updates: string[];
  thinkingAfterProgress?: string;
  progressSeen?: boolean;
}

export interface RenderReasoningOptions {
  header?: string;
  turns: ReasoningTurnState[];
  summaryText?: string;
}

const dropTrailingBlanks = (lines: string[]): void => {
  // eslint-disable-next-line functional/no-loop-statements -- trimming trailing blanks requires iterative popping
  while (lines.length > 0 && lines[lines.length - 1] === '') {
    lines.pop();
  }
};

const appendBlankLines = (lines: string[], count: number): void => {
  // eslint-disable-next-line functional/no-loop-statements -- appending a fixed number of blanks is clearer with a loop
  for (let i = 0; i < count; i += 1) {
    lines.push('');
  }
};

const appendThinkingBlock = (lines: string[], text: string): void => {
  const normalized = escapeMarkdown(text.trim());
  if (normalized.length === 0) return;
  dropTrailingBlanks(lines);
  appendBlankLines(lines, 2);
  lines.push('---');
  normalized.split(/\r?\n/gu).forEach((segment) => {
    lines.push(segment);
  });
  lines.push('---');
  appendBlankLines(lines, 2);
};

export const renderReasoningMarkdown = ({ header, turns, summaryText }: RenderReasoningOptions): string => {
  if (header === undefined) return '';
  const lines: string[] = [header];
  turns.forEach((turn) => {
    dropTrailingBlanks(lines);
    appendBlankLines(lines, 1);
    const headingParts = [`Turn ${String(turn.index)}`];
    if (typeof turn.attempt === 'number' && Number.isFinite(turn.attempt)) {
      headingParts.push(`Attempt ${String(turn.attempt)}`);
    }
    if (typeof turn.reason === 'string' && turn.reason.trim().length > 0) {
      headingParts.push(escapeMarkdown(turn.reason.trim()));
    }
    lines.push(`### ${headingParts.join(', ')}`);
    if (typeof turn.summary === 'string' && turn.summary.length > 0) {
      lines.push(`(${turn.summary})`);
    }
    if (typeof turn.thinking === 'string' && turn.thinking.trim().length > 0) {
      appendThinkingBlock(lines, turn.thinking);
    }
    if (turn.updates.length > 0) {
      turn.updates.forEach((line) => {
        lines.push(`- ${line}`);
      });
    }
    if (typeof turn.thinkingAfterProgress === 'string' && turn.thinkingAfterProgress.trim().length > 0) {
      appendThinkingBlock(lines, turn.thinkingAfterProgress);
    }
  });
  if (typeof summaryText === 'string' && summaryText.length > 0) {
    dropTrailingBlanks(lines);
    appendBlankLines(lines, 1);
    lines.push(summaryText);
  }
  appendBlankLines(lines, 1);
  return lines.join('\n');
};
