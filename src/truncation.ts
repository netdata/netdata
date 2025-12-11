/**
 * Unified truncation functions for ai-agent.
 *
 * All functions:
 * - Use 50/50 split (first half + last half)
 * - Insert marker at truncation point: [···TRUNCATED N unit···]
 * - Only truncate payloads >= 512 bytes (smaller payloads not worth truncating)
 * - Return undefined when: payload doesn't fit AND (payload < 512 OR truncation failed)
 * - Return payload unchanged when it already fits target
 */

// Worst-case marker: [···TRUNCATED 9999999999 bytes···] ≈ 34 bytes
// Using 50 to be safe with any unit name variations and edge cases
const MARKER_OVERHEAD = 50;

// Minimum payload size for truncation - below this, truncation is not attempted
const MIN_PAYLOAD_BYTES = 512;
const MIN_JSON_STRING_BYTES = 128; // Minimum string size for JSON truncation

// Middle dot character for marker (U+00B7)
const MIDDLE_DOT = '·';

// Segmenter for proper Unicode character splitting (handles emoji, CJK correctly)
const segmenter = new Intl.Segmenter('en', { granularity: 'grapheme' });

/**
 * Split a string into grapheme clusters (proper Unicode characters).
 * Handles emoji, CJK, and other multi-byte characters correctly.
 */
function splitIntoGraphemes(str: string): string[] {
  return Array.from(segmenter.segment(str), (s) => s.segment);
}

/**
 * Build truncation marker with actual omitted count.
 */
function buildMarker(omittedCount: number, unit: string): string {
  return `[${MIDDLE_DOT}${MIDDLE_DOT}${MIDDLE_DOT}TRUNCATED ${String(omittedCount)} ${unit}${MIDDLE_DOT}${MIDDLE_DOT}${MIDDLE_DOT}]`;
}

/**
 * Find a safe UTF-8 byte boundary at or before the given byte offset.
 * Ensures we don't split in the middle of a multi-byte character.
 */
function findSafeUtf8Boundary(buffer: Buffer, targetOffset: number): number {
  if (targetOffset >= buffer.length) return buffer.length;
  if (targetOffset <= 0) return 0;

  let offset = targetOffset;

  // Walk backwards to find start of a character
  // UTF-8 continuation bytes start with 10xxxxxx (0x80-0xBF)
  // eslint-disable-next-line functional/no-loop-statements -- iterative byte scanning
  while (offset > 0 && (buffer[offset] & 0xc0) === 0x80) {
    offset--;
  }

  return offset;
}

/**
 * Find a safe UTF-8 byte boundary at or after the given byte offset.
 * Used for finding the start of the last portion.
 */
function findSafeUtf8BoundaryAfter(buffer: Buffer, targetOffset: number): number {
  if (targetOffset >= buffer.length) return buffer.length;
  if (targetOffset <= 0) return 0;

  let offset = targetOffset;

  // If we're in the middle of a character, move forward to next char start
  // eslint-disable-next-line functional/no-loop-statements -- iterative byte scanning
  while (offset < buffer.length && (buffer[offset] & 0xc0) === 0x80) {
    offset++;
  }

  return offset;
}

/**
 * Truncate string to target byte count.
 * Uses 50/50 split with marker in middle.
 *
 * @param payload - String to truncate
 * @param targetBytes - Target size in bytes
 * @returns Truncated string fitting target, original if already fits,
 *          or undefined if payload < 512 bytes and doesn't fit target
 */
export function truncateToBytes(payload: string, targetBytes: number): string | undefined {
  const buffer = Buffer.from(payload, 'utf8');
  const inputBytes = buffer.length;

  // If payload already fits, return unchanged
  if (inputBytes <= targetBytes) {
    return payload;
  }

  // Only truncate payloads >= 512 bytes
  if (inputBytes < MIN_PAYLOAD_BYTES) {
    return undefined; // Too small to truncate, but doesn't fit
  }

  // Content budget = target - worst-case marker overhead
  const contentBudget = targetBytes - MARKER_OVERHEAD;
  if (contentBudget <= 0) {
    return undefined; // Target too small to fit even the marker
  }

  // Split 50/50
  const firstHalfBudget = Math.floor(contentBudget / 2);
  const lastHalfBudget = contentBudget - firstHalfBudget;

  // Find safe boundaries
  const firstEnd = findSafeUtf8Boundary(buffer, firstHalfBudget);
  const lastStart = findSafeUtf8BoundaryAfter(buffer, inputBytes - lastHalfBudget);

  // Extract portions
  const firstPart = buffer.subarray(0, firstEnd).toString('utf8');
  const lastPart = buffer.subarray(lastStart).toString('utf8');

  // Calculate actual omitted bytes
  const omittedBytes = inputBytes - firstEnd - (inputBytes - lastStart);
  const marker = buildMarker(omittedBytes, 'bytes');

  const result = firstPart + marker + lastPart;

  // Final check: result must fit target
  if (Buffer.byteLength(result, 'utf8') > targetBytes) {
    return undefined;
  }

  return result;
}

/**
 * Truncate string to target character count (grapheme clusters).
 * Uses 50/50 split with marker in middle.
 *
 * @param payload - String to truncate
 * @param targetChars - Target size in characters
 * @returns Truncated string fitting target, original if already fits,
 *          or undefined if payload < 512 bytes and doesn't fit target
 */
export function truncateToChars(payload: string, targetChars: number): string | undefined {
  const chars = splitIntoGraphemes(payload);
  const inputChars = chars.length;
  const inputBytes = Buffer.byteLength(payload, 'utf8');

  // If payload already fits, return unchanged
  if (inputChars <= targetChars) {
    return payload;
  }

  // Only truncate payloads >= 512 bytes
  if (inputBytes < MIN_PAYLOAD_BYTES) {
    return undefined; // Too small to truncate, but doesn't fit
  }

  // Content budget = target - marker overhead (in chars, marker is ~34 chars)
  const markerCharsOverhead = MARKER_OVERHEAD; // Marker is mostly ASCII, ~1 byte per char
  const contentBudget = targetChars - markerCharsOverhead;
  if (contentBudget <= 0) {
    return undefined; // Target too small to fit even the marker
  }

  // Split 50/50
  const firstHalfChars = Math.floor(contentBudget / 2);
  const lastHalfChars = contentBudget - firstHalfChars;

  // Extract portions
  const firstPart = chars.slice(0, firstHalfChars).join('');
  const lastPart = chars.slice(inputChars - lastHalfChars).join('');

  // Calculate actual omitted chars
  const omittedChars = inputChars - firstHalfChars - lastHalfChars;
  const marker = buildMarker(omittedChars, 'chars');

  const result = firstPart + marker + lastPart;
  const resultChars = splitIntoGraphemes(result).length;

  // Final check: result must fit target
  if (resultChars > targetChars) {
    return undefined;
  }

  return result;
}

/**
 * Truncate string to target token count.
 * Uses 50/50 split with marker in middle.
 *
 * @param payload - String to truncate
 * @param targetTokens - Target size in tokens
 * @param tokenizer - Optional tokenizer function, defaults to chars/4 approximation
 * @returns Truncated string fitting target, original if already fits,
 *          or undefined if payload < 512 bytes and doesn't fit target,
 *          or if tokenizer returns 0 for non-empty input
 */
export function truncateToTokens(
  payload: string,
  targetTokens: number,
  tokenizer?: (s: string) => number
): string | undefined {
  // Default tokenizer: ~4 chars per token approximation
  const countTokens = tokenizer ?? ((s: string) => Math.ceil(s.length / 4));

  const inputTokens = countTokens(payload);
  const inputBytes = Buffer.byteLength(payload, 'utf8');

  // Guard against tokenizer returning 0 for non-empty input (division by zero)
  if (inputTokens <= 0 && payload.length > 0) {
    return undefined;
  }

  // If payload already fits, return unchanged
  if (inputTokens <= targetTokens) {
    return payload;
  }

  // Only truncate payloads >= 512 bytes
  if (inputBytes < MIN_PAYLOAD_BYTES) {
    return undefined; // Too small to truncate, but doesn't fit
  }

  // Calculate actual marker overhead using the provided tokenizer
  // Use worst-case marker (10-digit number) to measure true token cost
  const worstCaseMarker = buildMarker(9999999999, 'tokens');
  const markerTokensOverhead = countTokens(worstCaseMarker);
  const contentBudget = targetTokens - markerTokensOverhead;
  if (contentBudget <= 0) {
    return undefined; // Target too small to fit even the marker
  }

  // Convert token budget to char budget (approximate)
  const charsPerToken = payload.length / inputTokens;
  const contentChars = Math.floor(contentBudget * charsPerToken);

  const chars = splitIntoGraphemes(payload);
  const inputChars = chars.length;

  // Split 50/50
  const firstHalfChars = Math.floor(contentChars / 2);
  const lastHalfChars = contentChars - firstHalfChars;

  // Extract portions
  const firstPart = chars.slice(0, firstHalfChars).join('');
  const lastPart = chars.slice(inputChars - lastHalfChars).join('');

  // Calculate omitted tokens
  const omittedChars = inputChars - firstHalfChars - lastHalfChars;
  const omittedTokens = Math.ceil(omittedChars / charsPerToken);
  const marker = buildMarker(omittedTokens, 'tokens');

  const result = firstPart + marker + lastPart;
  const resultTokens = countTokens(result);

  // Final check: result must fit target
  if (resultTokens > targetTokens) {
    return undefined;
  }

  return result;
}

/**
 * Information about a JSON string found during scanning.
 */
interface JsonStringInfo {
  /** Start index in the original string (including opening quote) */
  start: number;
  /** End index in the original string (including closing quote) */
  end: number;
  /** Byte length of the string content (excluding quotes) */
  byteLength: number;
  /** The actual string content (excluding quotes) */
  content: string;
}

/**
 * Scan a JSON string to find all string literals > 128 bytes.
 * Handles escaped quotes and other escape sequences.
 */
function scanJsonStrings(json: string): JsonStringInfo[] {
  const strings: JsonStringInfo[] = [];
  let i = 0;

  // eslint-disable-next-line functional/no-loop-statements -- JSON string scanning requires stateful iteration
  while (i < json.length) {
    // Find start of a string
    if (json[i] === '"') {
      const start = i;
      i++; // Skip opening quote

      // Find end of string, handling escapes
      let content = '';
      // eslint-disable-next-line functional/no-loop-statements -- nested string content parsing
      while (i < json.length) {
        if (json[i] === '\\' && i + 1 < json.length) {
          // Escape sequence - include both chars in content
          content += json[i] + json[i + 1];
          i += 2;
        } else if (json[i] === '"') {
          // End of string
          break;
        } else {
          content += json[i];
          i++;
        }
      }

      const end = i + 1; // Include closing quote
      const byteLength = Buffer.byteLength(content, 'utf8');

      // Only track strings > 128 bytes for JSON truncation
      if (byteLength > MIN_JSON_STRING_BYTES) {
        strings.push({ start, end, byteLength, content });
      }

      i++; // Skip closing quote
    } else {
      i++;
    }
  }

  return strings;
}

/**
 * Truncate a JSON string value (content only, not quotes).
 * Returns the truncated content with marker.
 */
function truncateJsonStringContent(content: string, targetBytes: number): string {
  const buffer = Buffer.from(content, 'utf8');
  const inputBytes = buffer.length;

  if (inputBytes <= targetBytes) {
    return content;
  }

  // Content budget = target - marker overhead
  const contentBudget = targetBytes - MARKER_OVERHEAD;
  if (contentBudget <= 0) {
    // Can't fit anything meaningful, just return marker
    return buildMarker(inputBytes, 'bytes');
  }

  // Split 50/50
  const firstHalfBudget = Math.floor(contentBudget / 2);
  const lastHalfBudget = contentBudget - firstHalfBudget;

  // Find safe boundaries
  const firstEnd = findSafeUtf8Boundary(buffer, firstHalfBudget);
  const lastStart = findSafeUtf8BoundaryAfter(buffer, inputBytes - lastHalfBudget);

  // Extract portions
  const firstPart = buffer.subarray(0, firstEnd).toString('utf8');
  const lastPart = buffer.subarray(lastStart).toString('utf8');

  // Calculate omitted bytes
  const omittedBytes = inputBytes - firstEnd - (inputBytes - lastStart);
  const marker = buildMarker(omittedBytes, 'bytes');

  return firstPart + marker + lastPart;
}

/**
 * Truncate JSON string values to bring total size under target bytes.
 * Works directly on stringified JSON - no parsing/re-serialization.
 * Only truncates string values > 128 bytes.
 *
 * @param payload - JSON string to truncate
 * @param targetBytes - Target size in bytes
 * @returns Truncated JSON string, or undefined if:
 *   - Invalid JSON
 *   - No large strings (> 128 bytes) to truncate
 *   - Cannot reach target even with max truncation
 */
export function truncateJsonStrings(payload: string, targetBytes: number): string | undefined {
  // Validate it's JSON first (before any early returns)
  try {
    JSON.parse(payload);
  } catch {
    return undefined;
  }

  const currentSize = Buffer.byteLength(payload, 'utf8');

  // If payload already fits, return unchanged
  if (currentSize <= targetBytes) {
    return payload;
  }

  // Find all large strings (> 128 bytes) that can be truncated
  const largeStrings = scanJsonStrings(payload);

  if (largeStrings.length === 0) {
    // No large strings to truncate - can't help
    return undefined;
  }

  // Sort by size descending (truncate largest first)
  largeStrings.sort((a, b) => b.byteLength - a.byteLength);

  // Process strings, tracking position shifts
  // Using reduce to accumulate state while processing each string
  const { result } = largeStrings.reduce(
    (state, strInfo) => {
      if (state.bytesToSave <= 0) return state;

      // Adjust positions for previous replacements
      const adjustedStart = strInfo.start + state.positionOffset;
      const adjustedEnd = strInfo.end + state.positionOffset;

      // Calculate how much to save from this string
      const maxSavingsFromThis = strInfo.byteLength - MARKER_OVERHEAD;
      const savingsNeeded = Math.min(state.bytesToSave, maxSavingsFromThis);
      const newContentSize = strInfo.byteLength - savingsNeeded;

      // Truncate the string content
      const truncatedContent = truncateJsonStringContent(strInfo.content, newContentSize);

      // Build new string with quotes
      const newString = '"' + truncatedContent + '"';
      const oldString = state.result.substring(adjustedStart, adjustedEnd);

      // Replace in result
      const newResult =
        state.result.substring(0, adjustedStart) + newString + state.result.substring(adjustedEnd);

      // Update tracking
      const oldLen = Buffer.byteLength(oldString, 'utf8');
      const newLen = Buffer.byteLength(newString, 'utf8');
      const actualSavings = oldLen - newLen;

      return {
        result: newResult,
        bytesToSave: state.bytesToSave - actualSavings,
        positionOffset: state.positionOffset + newString.length - oldString.length,
      };
    },
    { result: payload, bytesToSave: currentSize - targetBytes, positionOffset: 0 }
  );

  // Verify result is valid JSON
  try {
    JSON.parse(result);
  } catch {
    // Something went wrong - shouldn't happen but be safe
    return undefined;
  }

  // Final size check
  if (Buffer.byteLength(result, 'utf8') > targetBytes) {
    // Shouldn't happen with worst-case marker, but be safe
    return undefined;
  }

  return result;
}
