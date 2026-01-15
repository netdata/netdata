import type { ToolOutputStats } from './types.js';

const countLines = (text: string): number => {
  if (text.length === 0) return 0;
  let lines = 1;
  // eslint-disable-next-line functional/no-loop-statements -- faster than split for large blobs
  for (let i = 0; i < text.length; i += 1) {
    if (text.charCodeAt(i) === 10) {
      lines += 1;
    }
  }
  return lines;
};

export function computeToolOutputStats(text: string, countTokens: (value: string) => number): ToolOutputStats {
  const bytes = Buffer.byteLength(text, 'utf8');
  const lines = countLines(text);
  const tokens = Math.max(0, Math.trunc(countTokens(text)));
  const avgLineBytes = lines > 0 ? Math.ceil(bytes / lines) : bytes;
  return { bytes, lines, tokens, avgLineBytes };
}
