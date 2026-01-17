import type { ToolOutputStats } from './types.js';

// --- Grep-friendly content formatting ---

const SOFT_BREAK_CHARS = 1000;
const HARD_BREAK_BYTES = 2000;
const SOFT_BREAK_SCAN_RANGE = 200;

const BREAK_CHARS = new Set([
  ' ', ',', ';', ':', '/', '\\', '|', '?', '&', '=', '+', '-', '_',
  '(', ')', '[', ']', '{', '}', '"', "'", '<', '>',
]);

const isBreakChar = (char: string): boolean => BREAK_CHARS.has(char);

/**
 * Step 1a: Try to pretty-print JSON.
 */
const tryPrettyPrintJson = (content: string): { formatted: string; wasJson: boolean } => {
  const trimmed = content.trimStart();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
    return { formatted: content, wasJson: false };
  }
  try {
    const parsed: unknown = JSON.parse(content);
    const formatted = JSON.stringify(parsed, null, 2);
    return { formatted, wasJson: true };
  } catch {
    return { formatted: content, wasJson: false };
  }
};

/**
 * Step 1b: Split XML tags by inserting newlines between >< sequences.
 */
const splitXmlTags = (content: string): string => {
  if (!content.includes('><')) {
    return content;
  }
  return content.replace(/>\s*</g, '>\n<');
};

/**
 * Step 2: Replace escaped \n (literal backslash-n) with actual newlines.
 */
const replaceEscapedNewlines = (content: string): string => {
  return content.replace(/\\n/g, '\n');
};

/**
 * Find a break point, scanning backward then forward from target position.
 */
const findBreakPoint = (line: string, targetPos: number): number => {
  const minPos = Math.max(0, targetPos - SOFT_BREAK_SCAN_RANGE);
  // eslint-disable-next-line functional/no-loop-statements
  for (let i = targetPos; i >= minPos; i--) {
    if (isBreakChar(line[i])) {
      return i + 1;
    }
  }
  const maxPos = Math.min(line.length - 1, targetPos + SOFT_BREAK_SCAN_RANGE);
  // eslint-disable-next-line functional/no-loop-statements
  for (let i = targetPos + 1; i <= maxPos; i++) {
    if (isBreakChar(line[i])) {
      return i + 1;
    }
  }
  return -1;
};

const byteLength = (str: string): number => Buffer.byteLength(str, 'utf8');

/**
 * Find the last valid UTF-8 character boundary before maxBytes.
 */
const findUtf8Boundary = (line: string, maxBytes: number): number => {
  let bytes = 0;
  let lastValidIndex = 0;
  // eslint-disable-next-line functional/no-loop-statements
  for (let i = 0; i < line.length; i++) {
    const charBytes = byteLength(line[i]);
    if (bytes + charBytes > maxBytes) {
      break;
    }
    bytes += charBytes;
    lastValidIndex = i + 1;
  }
  return lastValidIndex;
};

/**
 * Step 3: Break lines at space/symbol if longer than SOFT_BREAK_CHARS.
 */
const softBreakLines = (content: string): string => {
  const lines = content.split('\n');
  const result: string[] = [];
  // eslint-disable-next-line functional/no-loop-statements
  for (const line of lines) {
    if (line.length <= SOFT_BREAK_CHARS) {
      result.push(line);
      continue;
    }
    let remaining = line;
    // eslint-disable-next-line functional/no-loop-statements
    while (remaining.length > SOFT_BREAK_CHARS) {
      const breakPoint = findBreakPoint(remaining, SOFT_BREAK_CHARS);
      if (breakPoint > 0 && breakPoint < remaining.length) {
        result.push(remaining.slice(0, breakPoint));
        remaining = remaining.slice(breakPoint);
      } else {
        break;
      }
    }
    if (remaining.length > 0) {
      result.push(remaining);
    }
  }
  return result.join('\n');
};

/**
 * Step 4: Force break lines at UTF-8 boundary if longer than HARD_BREAK_BYTES.
 */
const hardBreakLines = (content: string): string => {
  const lines = content.split('\n');
  const result: string[] = [];
  // eslint-disable-next-line functional/no-loop-statements
  for (const line of lines) {
    if (byteLength(line) <= HARD_BREAK_BYTES) {
      result.push(line);
      continue;
    }
    let remaining = line;
    // eslint-disable-next-line functional/no-loop-statements
    while (byteLength(remaining) > HARD_BREAK_BYTES) {
      const breakIndex = findUtf8Boundary(remaining, HARD_BREAK_BYTES);
      if (breakIndex > 0 && breakIndex < remaining.length) {
        result.push(remaining.slice(0, breakIndex));
        remaining = remaining.slice(breakIndex);
      } else {
        break;
      }
    }
    if (remaining.length > 0) {
      result.push(remaining);
    }
  }
  return result.join('\n');
};

/**
 * Format tool output content for grep-friendly line breaks.
 *
 * Progressive formatting:
 * 1. Pretty-print JSON or split XML tags
 * 2. Replace escaped \n with actual newlines
 * 3. Break at space/symbol if line > 1000 chars
 * 4. Force break at UTF-8 boundary if line > 2000 bytes
 */
export const formatForGrep = (content: string): string => {
  const { formatted: afterJson, wasJson } = tryPrettyPrintJson(content);
  const afterXml = wasJson ? afterJson : splitXmlTags(afterJson);
  const afterEscapes = replaceEscapedNewlines(afterXml);
  const afterSoftBreak = softBreakLines(afterEscapes);
  const afterHardBreak = hardBreakLines(afterSoftBreak);
  return afterHardBreak;
};

// --- Handle/message formatting ---

export function formatHandleMessage(handle: string, stats: ToolOutputStats): string {
  return `Tool output is too large (${String(stats.bytes)} bytes, ${String(stats.lines)} lines, ${String(stats.tokens)} tokens).\nCall tool_output(handle = "${handle}", extract = "what to extract").\nThe handle is a relative path under the tool_output root.\nProvide precise and detailed instructions in \`extract\` about what you are looking for.`;
}

export function formatToolOutputSuccess(args: {
  toolName: string;
  handle: string;
  mode: string;
  body: string;
}): string {
  return `ABSTRACT FROM TOOL OUTPUT ${args.toolName} WITH HANDLE ${args.handle}, STRATEGY:${args.mode}:\n\n${args.body}`;
}

export function formatToolOutputFailure(args: {
  toolName: string;
  handle: string;
  mode: string;
  error: string;
}): string {
  return `TOOL_OUTPUT FAILED FOR ${args.toolName} WITH HANDLE ${args.handle}, STRATEGY:${args.mode}:\n\n${args.error}`;
}
