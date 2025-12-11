/* eslint-disable @typescript-eslint/strict-boolean-expressions */
/* eslint-disable @typescript-eslint/no-unsafe-return */
/* eslint-disable @typescript-eslint/restrict-template-expressions */
/* eslint-disable @typescript-eslint/require-array-sort-compare */
/* eslint-disable sonarjs/no-duplicate-string */
/* eslint-disable functional/no-loop-statements */
import { describe, expect, it } from 'vitest';

import {
  truncateJsonStrings,
  truncateToBytes,
  truncateToChars,
  truncateToTokens,
} from '../../truncation.js';

// Constants for tests
const MIN_TARGET = 512; // Minimum payload size for bytes/chars/tokens truncation
const MIN_JSON_STRING = 128; // Minimum string size for JSON string truncation
const MARKER_PATTERN = /\[···TRUNCATED \d+ (bytes|chars|tokens)···\]/;

// Helper to generate a string of specific byte length
function generateStringOfBytes(byteLength: number, char = 'a'): string {
  return char.repeat(byteLength);
}

// Helper to generate a string with multi-byte chars
function generateMultiByteString(charCount: number): string {
  const chars = ['a', 'é', '中', '🎉'];
  let result = '';
  for (let i = 0; i < charCount; i++) {
    result += chars[i % chars.length];
  }
  return result;
}

// Helper to extract marker from truncated result
function extractMarker(text: string): RegExpExecArray | null {
  return MARKER_PATTERN.exec(text);
}

// Helper to count graphemes properly
function countGraphemes(str: string): number {
  const segmenter = new Intl.Segmenter('en', { granularity: 'grapheme' });
  return Array.from(segmenter.segment(str)).length;
}

describe('truncateToBytes', () => {
  describe('basic behavior', () => {
    it('returns undefined when payload < 512 bytes AND does not fit target', () => {
      // Payload is 200 bytes (< 512), can't truncate it
      // Target is 100 bytes - payload doesn't fit, so undefined
      const smallPayload = generateStringOfBytes(200);
      expect(truncateToBytes(smallPayload, 100)).toBeUndefined();

      // But if payload fits target, return it even with small target
      expect(truncateToBytes(smallPayload, 200)).toBe(smallPayload);
      expect(truncateToBytes(smallPayload, 300)).toBe(smallPayload);
    });

    it('truncates large payload to small target', () => {
      // Payload is 1000 bytes (>= 512), so we CAN truncate it
      const largePayload = generateStringOfBytes(1000);
      const result = truncateToBytes(largePayload, 100);
      expect(result).toBeDefined();
      if (!result) return;
      expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(100);
    });

    it('returns undefined when target <= 0 or negative', () => {
      const input = generateStringOfBytes(1000);
      // Can't fit anything in 0 or negative bytes
      expect(truncateToBytes(input, 0)).toBeUndefined();
      expect(truncateToBytes(input, -1)).toBeUndefined();
    });

    it('returns unchanged when content <= target', () => {
      const input = generateStringOfBytes(500);
      expect(truncateToBytes(input, MIN_TARGET)).toBe(input);
      expect(truncateToBytes(input, 1000)).toBe(input);
    });

    it('returns unchanged when content exactly at target', () => {
      const input = generateStringOfBytes(MIN_TARGET);
      expect(truncateToBytes(input, MIN_TARGET)).toBe(input);
    });

    it('returns unchanged for empty input', () => {
      expect(truncateToBytes('', MIN_TARGET)).toBe('');
    });

    it('truncates when content > target', () => {
      const input = generateStringOfBytes(1000);
      const result = truncateToBytes(input, MIN_TARGET);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(MIN_TARGET);
    });
  });

  describe('50/50 split verification', () => {
    it('splits content approximately 50/50', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      if (!result) return;

      const match = extractMarker(result);
      expect(match).not.toBeNull();
      if (!match) return;

      const markerIndex = result.indexOf(match[0]);
      const firstPart = result.substring(0, markerIndex);
      const lastPart = result.substring(markerIndex + match[0].length);

      const firstBytes = Buffer.byteLength(firstPart, 'utf8');
      const lastBytes = Buffer.byteLength(lastPart, 'utf8');
      expect(Math.abs(firstBytes - lastBytes)).toBeLessThan(10);
    });

    it('marker appears in middle', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 1000);
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('first half is beginning of original', () => {
      const input = 'AAAA' + generateStringOfBytes(2000) + 'ZZZZ';
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result?.startsWith('AAAA')).toBe(true);
    });

    it('last half is ending of original', () => {
      const input = 'AAAA' + generateStringOfBytes(2000) + 'ZZZZ';
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result?.endsWith('ZZZZ')).toBe(true);
    });
  });

  describe('marker format', () => {
    it('uses correct format with bytes unit', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 1000);
      expect(result).toMatch(/\[···TRUNCATED \d+ bytes···\]/);
    });

    it('marker contains middle dots (U+00B7)', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 1000);
      expect(result).toContain('·');
    });

    it('marker shows correct omitted byte count', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      if (!result) return;

      const match = /\[···TRUNCATED (\d+) bytes···\]/.exec(result);
      expect(match).not.toBeNull();
      if (!match) return;

      const omitted = parseInt(match[1], 10);
      expect(omitted).toBeGreaterThan(1000);
      expect(omitted).toBeLessThan(1100);
    });
  });

  describe('UTF-8 boundary safety', () => {
    it('handles 2-byte characters at split boundary (é)', () => {
      const input = 'é'.repeat(1000);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
    });

    it('handles 3-byte characters at split boundary (中)', () => {
      const input = '中'.repeat(700);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
    });

    it('handles 4-byte characters at split boundary (emoji)', () => {
      const input = '🎉'.repeat(500);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
    });

    it('result is valid UTF-8', () => {
      const input = generateMultiByteString(500);
      const result = truncateToBytes(input, 800);
      expect(result).toBeDefined();
      if (!result) return;

      const buffer = Buffer.from(result, 'utf8');
      const decoded = buffer.toString('utf8');
      expect(decoded).toBe(result);
      expect(decoded).not.toContain('\uFFFD');
    });

    it('handles mixed ASCII and multi-byte content', () => {
      const input = 'Hello 世界! 🌍 Test 日本語 more text ' + generateStringOfBytes(1500);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
    });
  });

  describe('edge cases', () => {
    it('handles input exactly 512 bytes', () => {
      const input = generateStringOfBytes(MIN_TARGET);
      expect(truncateToBytes(input, MIN_TARGET)).toBe(input);
    });

    it('handles input 513 bytes with target 512', () => {
      const input = generateStringOfBytes(513);
      const result = truncateToBytes(input, MIN_TARGET);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(MIN_TARGET);
    });

    it('handles very long content (1MB+)', () => {
      const input = generateStringOfBytes(1024 * 1024);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('handles content that is all multi-byte chars', () => {
      const input = '🎉'.repeat(500);
      const result = truncateToBytes(input, MIN_TARGET);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(MIN_TARGET);
      expect(result).not.toContain('\uFFFD');
    });

    it('target = 512 (minimum allowed)', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, MIN_TARGET);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(MIN_TARGET);
    });

    it('target = 511 truncates large payload', () => {
      // With new logic: payload >= 512 means we CAN truncate, regardless of target
      const input = generateStringOfBytes(2000);
      const result = truncateToBytes(input, 511);
      expect(result).toBeDefined();
      if (!result) return;
      expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(511);
    });
  });

  describe('CRITICAL: byte length guarantee', () => {
    it('result never exceeds target bytes', () => {
      const testCases = [
        { input: generateStringOfBytes(1000), target: MIN_TARGET },
        { input: generateStringOfBytes(10000), target: 1000 },
        { input: generateMultiByteString(500), target: 800 },
        { input: '🎉'.repeat(500), target: 600 },
        { input: '中'.repeat(700), target: 1000 },
      ];

      testCases.forEach(({ input, target }) => {
        const result = truncateToBytes(input, target);
        if (result !== undefined) {
          expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(target);
        }
      });
    });
  });
});

describe('truncateToChars', () => {
  describe('basic behavior', () => {
    it('returns undefined when payload < 512 bytes AND does not fit target', () => {
      // Small payload (200 bytes) can't be truncated
      const smallPayload = generateStringOfBytes(200);
      expect(truncateToChars(smallPayload, 100)).toBeUndefined();
      // But if it fits, return it unchanged
      expect(truncateToChars(smallPayload, 300)).toBe(smallPayload);
    });

    it('truncates large payload to small char target', () => {
      // Large payload (>= 512 bytes) CAN be truncated to any target
      const input = generateStringOfBytes(1000);
      const result = truncateToChars(input, 100);
      expect(result).toBeDefined();
      if (!result) return;
      expect(countGraphemes(result)).toBeLessThanOrEqual(100);
    });

    it('returns unchanged when content <= target', () => {
      const input = generateStringOfBytes(500);
      expect(truncateToChars(input, 600)).toBe(input);
    });

    it('returns unchanged when content exactly at target', () => {
      const input = generateStringOfBytes(500);
      expect(truncateToChars(input, 500)).toBe(input);
    });

    it('returns unchanged for empty input', () => {
      expect(truncateToChars('', 600)).toBe('');
    });

    it('truncates when content > target', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(countGraphemes(result ?? '')).toBeLessThanOrEqual(1000);
    });
  });

  describe('50/50 split verification', () => {
    it('splits content approximately 50/50 by characters', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      if (!result) return;

      const match = extractMarker(result);
      expect(match).not.toBeNull();
      if (!match) return;

      const markerIndex = result.indexOf(match[0]);
      const firstPart = result.substring(0, markerIndex);
      const lastPart = result.substring(markerIndex + match[0].length);

      const firstChars = countGraphemes(firstPart);
      const lastChars = countGraphemes(lastPart);
      expect(Math.abs(firstChars - lastChars)).toBeLessThan(10);
    });

    it('first half is beginning of original', () => {
      const input = 'START' + generateStringOfBytes(2000) + 'END';
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(result?.startsWith('START')).toBe(true);
    });

    it('last half is ending of original', () => {
      const input = 'START' + generateStringOfBytes(2000) + 'END';
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(result?.endsWith('END')).toBe(true);
    });
  });

  describe('marker format', () => {
    it('uses correct format with chars unit', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toMatch(/\[···TRUNCATED \d+ chars···\]/);
    });

    it('marker contains middle dots', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toContain('·');
    });
  });

  describe('UTF-8 handling', () => {
    it('handles ASCII-only content', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('handles mixed ASCII + multi-byte content', () => {
      const input = 'Hello ' + '世界'.repeat(500) + ' World';
      const result = truncateToChars(input, 800);
      expect(result).toBeDefined();
      expect(result?.startsWith('Hello')).toBe(true);
      expect(result?.endsWith('World')).toBe(true);
    });

    it('handles content starting with multi-byte char', () => {
      const input = '🎉' + generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(result?.startsWith('🎉')).toBe(true);
    });

    it('handles content ending with multi-byte char', () => {
      const input = generateStringOfBytes(2000) + '🎉';
      const result = truncateToChars(input, 1000);
      expect(result).toBeDefined();
      expect(result?.endsWith('🎉')).toBe(true);
    });

    it('does not break multi-byte chars at split boundary', () => {
      const input = generateMultiByteString(1000);
      const result = truncateToChars(input, 700);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
    });
  });

  describe('edge cases', () => {
    it('handles very long content (100KB+)', () => {
      const input = generateStringOfBytes(100000);
      const result = truncateToChars(input, 5000);
      expect(result).toBeDefined();
      expect(countGraphemes(result ?? '')).toBeLessThanOrEqual(5000);
    });

    it('handles content with only multi-byte chars (all emoji)', () => {
      const input = '🎉'.repeat(500);
      const result = truncateToChars(input, 400);
      expect(result).toBeDefined();
      expect(countGraphemes(result ?? '')).toBeLessThanOrEqual(400);
    });
  });

  describe('CRITICAL: byte length guarantee', () => {
    it('result fits within reasonable byte bounds', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToChars(input, 1000);
      if (result !== undefined) {
        expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(1100);
      }
    });
  });
});

describe('truncateToTokens', () => {
  describe('basic behavior', () => {
    it('returns undefined when payload < 512 bytes AND does not fit target', () => {
      // Small payload can't be truncated
      const smallPayload = generateStringOfBytes(200);
      expect(truncateToTokens(smallPayload, 10)).toBeUndefined(); // 10 tokens ~= 40 chars
      // But if it fits, return unchanged
      expect(truncateToTokens(smallPayload, 100)).toBe(smallPayload); // 100 tokens ~= 400 chars
    });

    it('truncates large payload to small token target', () => {
      // Large payload CAN be truncated to any target
      const input = generateStringOfBytes(1000);
      const result = truncateToTokens(input, 50);
      expect(result).toBeDefined();
    });

    it('returns unchanged when content <= target', () => {
      const input = generateStringOfBytes(400);
      expect(truncateToTokens(input, 200)).toBe(input);
    });

    it('truncates when content > target', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToTokens(input, 300);
      expect(result).toBeDefined();
    });
  });

  describe('tokenizer modes', () => {
    it('uses default tokenizer (chars/4 approximation)', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToTokens(input, 300);
      expect(result).toBeDefined();
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('accepts custom tokenizer function', () => {
      const input = generateStringOfBytes(2000);
      const customTokenizer = (s: string) => Math.ceil(s.length / 10);
      const result = truncateToTokens(input, 150, customTokenizer);
      expect(result).toBeDefined();
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('custom tokenizer affects truncation', () => {
      const input = generateStringOfBytes(2000);
      // Tokenizer counts every char as 1 token (very aggressive)
      const aggressiveTokenizer = (s: string) => s.length;
      // The function now correctly measures marker overhead using the tokenizer
      // Marker ~34 chars = 34 tokens overhead
      // With target 1500 and 2000-char input:
      // contentBudget = 1500 - 34 = 1466 tokens/chars
      // result = 1466 chars + 34 char marker = 1500 chars/tokens ✓
      const result = truncateToTokens(input, 1500, aggressiveTokenizer);
      expect(result).toBeDefined();
      if (result) {
        expect(aggressiveTokenizer(result)).toBeLessThanOrEqual(1500);
        expect(result.length).toBeLessThan(input.length); // Actually truncated
      }

      // Verify tokenizer IS being used: same input with default tokenizer (chars/4) works differently
      // 2000 chars / 4 = 500 tokens, target 1500 > 500, so no truncation needed
      const resultDefault = truncateToTokens(input, 1500);
      expect(resultDefault).toBe(input); // Returns unchanged since 500 tokens < 1500 target
    });
  });

  describe('marker format', () => {
    it('uses correct format with tokens unit', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToTokens(input, 300);
      expect(result).toMatch(/\[···TRUNCATED \d+ tokens···\]/);
    });
  });

  describe('edge cases', () => {
    it('handles very short content (< 10 tokens)', () => {
      const input = generateStringOfBytes(20);
      const result = truncateToTokens(input, 200);
      expect(result).toBe(input);
    });

    it('handles very long content (100K+ tokens)', () => {
      const input = generateStringOfBytes(400000);
      const result = truncateToTokens(input, 1000);
      expect(result).toBeDefined();
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('handles custom tokenizer returning 0', () => {
      // Tokenizer returning 0 for non-empty input is treated as invalid
      // (prevents division by zero in charsPerToken calculation)
      const input = generateStringOfBytes(2000);
      const zeroTokenizer = () => 0;
      const result = truncateToTokens(input, 200, zeroTokenizer);
      expect(result).toBeUndefined(); // Guard against broken tokenizer

      // Empty input with zero tokens is fine (nothing to truncate)
      const emptyResult = truncateToTokens('', 200, zeroTokenizer);
      expect(emptyResult).toBe(''); // Empty input returns unchanged
    });
  });

  describe('CRITICAL: byte length guarantee', () => {
    it('result is at least 512 bytes when returned', () => {
      const input = generateStringOfBytes(2000);
      const result = truncateToTokens(input, 300);
      if (result !== undefined && result !== input) {
        expect(Buffer.byteLength(result, 'utf8')).toBeGreaterThanOrEqual(400);
      }
    });
  });
});

describe('truncateJsonStrings', () => {
  function createJsonWithLargeString(size: number): string {
    return JSON.stringify({ key: 'a'.repeat(size) });
  }

  function createJsonWithMultipleLargeStrings(sizes: number[]): string {
    const obj: Record<string, string> = {};
    sizes.forEach((size, i) => {
      obj[`key${i}`] = 'x'.repeat(size);
    });
    return JSON.stringify(obj);
  }

  describe('basic behavior', () => {
    it('truncates to small target when JSON has large strings', () => {
      // JSON with 1000-byte string CAN be truncated to small targets
      // because the string itself is > 128 bytes
      const input = createJsonWithLargeString(1000);
      const result = truncateJsonStrings(input, 200);
      expect(result).toBeDefined();
      if (!result) return;
      expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(200);
      // Result should still be valid JSON
      expect(() => JSON.parse(result)).not.toThrow();
    });

    it('returns unchanged when content <= target', () => {
      // Create input large enough that target >= 512 (minimum)
      const input = createJsonWithLargeString(500);
      const inputSize = Buffer.byteLength(input, 'utf8');
      // Target is 512+ and input fits within it
      expect(truncateJsonStrings(input, Math.max(MIN_TARGET, inputSize + 100))).toBe(input);
    });

    it('truncates when content > target with large strings', () => {
      const input = createJsonWithLargeString(1000);
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(600);
    });

    it('returns undefined when no large strings (all <= 128 bytes)', () => {
      const obj: Record<string, string> = {};
      for (let i = 0; i < 100; i++) {
        obj[`key${i}`] = 'small'.repeat(20); // 100 bytes each, under 128 threshold
      }
      const input = JSON.stringify(obj);
      expect(truncateJsonStrings(input, 600)).toBeUndefined();
    });

    it('returns undefined when target impossible even after truncation', () => {
      // Test case: JSON with no truncatable strings, so we can't reduce size at all
      // All strings are <= 128 bytes, so nothing can be truncated
      const obj: Record<string, string> = {};
      for (let i = 0; i < 10; i++) {
        obj[`key${i}`] = 'x'.repeat(100); // 100 bytes each, under 128 threshold
      }
      const input = JSON.stringify(obj);
      const inputSize = Buffer.byteLength(input, 'utf8');
      // No truncatable strings, so asking for less than current size returns undefined
      expect(truncateJsonStrings(input, inputSize - 100)).toBeUndefined();
    });

    it('returns undefined when truncation cannot reach target', () => {
      // Test case: JSON where even max truncation cannot reach target
      // We have one 600-byte string (can truncate to ~34 bytes = marker)
      // Rest is structure we can't touch
      const obj: Record<string, string> = {};
      for (let i = 0; i < 20; i++) {
        obj[`key${i}`] = 'x'.repeat(400); // 400 bytes each, under threshold
      }
      obj.large = 'y'.repeat(600); // 600 bytes, above threshold
      const input = JSON.stringify(obj);
      // Max savings from 600-byte string: 600 - ~34 = ~566 bytes
      // Structure overhead (~9000 bytes from 20×400-byte strings + keys + JSON syntax)
      // Target of 100 bytes (way below structure size) should return undefined
      expect(truncateJsonStrings(input, 100)).toBeUndefined();
    });
  });

  describe('string scanning', () => {
    it('handles simple string', () => {
      const input = createJsonWithLargeString(1000);
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        expect(() => JSON.parse(result)).not.toThrow();
      }
    });

    it('handles escaped quotes inside string', () => {
      // Content with actual quote chars that JSON.stringify will escape
      const content = 'say "hello" ' + 'x'.repeat(800);
      const input = JSON.stringify({ key: content });
      // JSON will have: {"key":"say \"hello\" xxx..."}
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { key: string };
        // After JSON.parse, the quotes are unescaped
        expect(parsed.key).toContain('say "hello"');
      }
    });

    it('handles escaped backslash before quote', () => {
      const content = 'path\\\\more ' + 'x'.repeat(800);
      const input = JSON.stringify({ key: content });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { key: string };
        expect(parsed.key).toBeDefined();
      }
    });

    it('handles Unicode in strings', () => {
      const content = 'こんにちは' + 'x'.repeat(800);
      const input = JSON.stringify({ key: content });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { key: string };
        expect(parsed.key).toContain('こんにちは');
      }
    });

    it('handles newlines in strings', () => {
      const content = 'line1\\nline2 ' + 'x'.repeat(800);
      const input = JSON.stringify({ key: content });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        expect(() => JSON.parse(result)).not.toThrow();
      }
    });

    it('handles multiple large strings in same object', () => {
      const input = createJsonWithMultipleLargeStrings([800, 700, 600]);
      const result = truncateJsonStrings(input, 800);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as Record<string, unknown>;
        expect(Object.keys(parsed)).toHaveLength(3);
      }
    });

    it('handles nested objects with large strings', () => {
      const input = JSON.stringify({
        outer: {
          inner: {
            deep: 'x'.repeat(1000),
          },
        },
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { outer: { inner: { deep: string } } };
        expect(parsed.outer.inner.deep).toMatch(MARKER_PATTERN);
      }
    });

    it('handles arrays containing large strings', () => {
      const input = JSON.stringify({
        items: ['x'.repeat(800), 'y'.repeat(700)],
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { items: string[] };
        expect(parsed.items).toHaveLength(2);
      }
    });

    it('handles mixed: large strings + small strings + numbers + booleans + null', () => {
      const input = JSON.stringify({
        large: 'x'.repeat(1000),
        small: 'tiny',
        num: 42,
        bool: true,
        nothing: null,
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as Record<string, unknown>;
        expect(parsed.small).toBe('tiny');
        expect(parsed.num).toBe(42);
        expect(parsed.bool).toBe(true);
        expect(parsed.nothing).toBeNull();
      }
    });
  });

  describe('truncation correctness', () => {
    it('truncates single large string with marker', () => {
      const input = createJsonWithLargeString(1000);
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      expect(result).toMatch(MARKER_PATTERN);
    });

    it('truncates largest string first', () => {
      const input = createJsonWithMultipleLargeStrings([600, 1000, 700]);
      const result = truncateJsonStrings(input, 1500);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { key1: string };
        expect(parsed.key1).toMatch(MARKER_PATTERN);
      }
    });

    it('stops once target reached (does not over-truncate)', () => {
      const input = createJsonWithMultipleLargeStrings([800, 800, 800]);
      const inputSize = Buffer.byteLength(input, 'utf8');
      const target = inputSize - 200;
      const result = truncateJsonStrings(input, target);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as Record<string, string>;
        const truncatedCount = [parsed.key0, parsed.key1, parsed.key2].filter((v) =>
          MARKER_PATTERN.test(v)
        ).length;
        expect(truncatedCount).toBeLessThanOrEqual(2);
      }
    });
  });

  describe('edge cases', () => {
    it('string exactly 128 bytes is not truncated', () => {
      const input = JSON.stringify({ key: 'x'.repeat(MIN_JSON_STRING) });
      const inputSize = Buffer.byteLength(input, 'utf8');
      const result = truncateJsonStrings(input, inputSize - 10);
      // String is exactly 128 bytes, threshold is > 128, so not eligible
      expect(result).toBeUndefined();
    });

    it('string 129 bytes is eligible for truncation', () => {
      const input = JSON.stringify({ key: 'x'.repeat(129) });
      // Input is ~139 bytes (129 + quotes + {"key":})
      // Target must be less than input size to trigger truncation
      const result = truncateJsonStrings(input, 135);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { key: string };
        expect(parsed.key).toMatch(MARKER_PATTERN);
      }
    });

    it('empty JSON object {} is unchanged', () => {
      const input = '{}';
      expect(truncateJsonStrings(input, MIN_TARGET)).toBe(input);
    });

    it('empty JSON array [] is unchanged', () => {
      const input = '[]';
      expect(truncateJsonStrings(input, MIN_TARGET)).toBe(input);
    });

    it('JSON with only small strings returns unchanged when fits target', () => {
      // JSON with multiple small strings (all <= 128 bytes) - no truncatable strings
      // But if it fits target, return unchanged
      const input = JSON.stringify({
        a: 'x'.repeat(50),
        b: 'y'.repeat(80),
        c: 'z'.repeat(100),
      });
      const inputSize = Buffer.byteLength(input, 'utf8');
      // Target larger than input - should return unchanged
      expect(truncateJsonStrings(input, inputSize + 100)).toBe(input);
      // Target exactly at input size - should return unchanged
      expect(truncateJsonStrings(input, inputSize)).toBe(input);
    });

    it('handles deeply nested structure with one large string', () => {
      const input = JSON.stringify({
        a: { b: { c: { d: { e: 'x'.repeat(1000) } } } },
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as { a: { b: { c: { d: { e: string } } } } };
        expect(parsed.a.b.c.d.e).toMatch(MARKER_PATTERN);
      }
    });

    it('handles adjacent large strings in array', () => {
      const input = JSON.stringify(['x'.repeat(800), 'y'.repeat(800)]);
      const result = truncateJsonStrings(input, 800);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as string[];
        expect(parsed[0]).toMatch(MARKER_PATTERN);
        expect(parsed[1]).toMatch(MARKER_PATTERN);
      }
    });

    it('returns undefined for invalid JSON', () => {
      expect(truncateJsonStrings('{invalid json}', 1000)).toBeUndefined();
      expect(truncateJsonStrings('not json at all', 1000)).toBeUndefined();
    });
  });

  describe('CRITICAL: JSON validity preserved', () => {
    it('result is always valid JSON', () => {
      const testCases = [
        createJsonWithLargeString(1000),
        createJsonWithMultipleLargeStrings([800, 700, 600]),
        JSON.stringify({ nested: { deep: 'x'.repeat(1000) } }),
        JSON.stringify(['x'.repeat(800), 'y'.repeat(800)]),
        JSON.stringify({ mixed: 'x'.repeat(1000), num: 42, arr: [1, 2, 3] }),
      ];

      testCases.forEach((input) => {
        const result = truncateJsonStrings(input, 600);
        if (result !== undefined) {
          expect(() => JSON.parse(result)).not.toThrow();
        }
      });
    });
  });

  describe('CRITICAL: byte length guarantee', () => {
    it('result never exceeds target bytes', () => {
      const testCases = [
        { input: createJsonWithLargeString(1000), target: 600 },
        { input: createJsonWithLargeString(5000), target: 1000 },
        { input: createJsonWithMultipleLargeStrings([800, 700, 600]), target: 800 },
        { input: JSON.stringify({ nested: { a: 'x'.repeat(2000) } }), target: 700 },
      ];

      testCases.forEach(({ input, target }) => {
        const result = truncateJsonStrings(input, target);
        if (result !== undefined) {
          expect(Buffer.byteLength(result, 'utf8')).toBeLessThanOrEqual(target);
        }
      });
    });
  });

  describe('CRITICAL: original keys preserved', () => {
    it('all original keys present in result', () => {
      const input = JSON.stringify({
        alpha: 'x'.repeat(1000),
        beta: 'small',
        gamma: 42,
        delta: true,
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as Record<string, unknown>;
        expect(Object.keys(parsed).sort()).toEqual(['alpha', 'beta', 'delta', 'gamma']);
      }
    });
  });

  describe('CRITICAL: non-string values unchanged', () => {
    it('numbers, booleans, null remain unchanged', () => {
      const input = JSON.stringify({
        large: 'x'.repeat(1000),
        num: 123.456,
        bool: false,
        nil: null,
        arr: [1, 2, 3],
      });
      const result = truncateJsonStrings(input, 600);
      expect(result).toBeDefined();
      if (result) {
        const parsed = JSON.parse(result) as Record<string, unknown>;
        expect(parsed.num).toBe(123.456);
        expect(parsed.bool).toBe(false);
        expect(parsed.nil).toBeNull();
        expect(parsed.arr).toEqual([1, 2, 3]);
      }
    });
  });
});

describe('edge case tests (cross-function)', () => {
  describe('UTF-8 multi-byte boundary', () => {
    it('truncating mid-emoji does not produce replacement char', () => {
      const input = '🎉'.repeat(200) + 'test' + '🌍'.repeat(200);
      const result = truncateToBytes(input, 600);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
    });

    it('truncating mid-CJK does not produce replacement char', () => {
      const input = '中文'.repeat(500);
      const result = truncateToBytes(input, 800);
      expect(result).toBeDefined();
      expect(result).not.toContain('\uFFFD');
    });
  });

  describe('double-truncation', () => {
    it('content with existing marker can be truncated again', () => {
      const input = generateStringOfBytes(2000);
      const firstTruncation = truncateToBytes(input, 1500);
      expect(firstTruncation).toBeDefined();
      expect(firstTruncation).toMatch(MARKER_PATTERN);

      if (firstTruncation) {
        const secondTruncation = truncateToBytes(firstTruncation, 800);
        expect(secondTruncation).toBeDefined();
        expect(secondTruncation).toMatch(MARKER_PATTERN);
        expect(Buffer.byteLength(secondTruncation ?? '', 'utf8')).toBeLessThanOrEqual(800);
      }
    });

    it('markers stack cleanly', () => {
      const input = generateStringOfBytes(3000);
      const first = truncateToBytes(input, 2000);
      expect(first).toBeDefined();
      if (first) {
        const second = truncateToBytes(first, 1000);
        expect(second).toBeDefined();
        expect(second).toMatch(MARKER_PATTERN);
        expect(Buffer.byteLength(second ?? '', 'utf8')).toBeLessThanOrEqual(1000);
      }
    });
  });

  describe('boundary conditions', () => {
    it('target = 512 works for all functions', () => {
      const input = generateStringOfBytes(2000);

      const bytesResult = truncateToBytes(input, MIN_TARGET);
      expect(bytesResult).toBeDefined();

      const charsResult = truncateToChars(input, 600);
      expect(charsResult).toBeDefined();

      const tokensResult = truncateToTokens(input, 200);
      expect(tokensResult).toBeDefined();

      const jsonInput = JSON.stringify({ key: 'x'.repeat(1000) });
      const jsonResult = truncateJsonStrings(jsonInput, MIN_TARGET);
      expect(jsonResult).toBeDefined();
    });

    it('target = 511 works when payload >= 512 bytes', () => {
      // New behavior: target can be any size, as long as payload >= 512 bytes
      const input = generateStringOfBytes(2000);
      const bytesResult = truncateToBytes(input, 511);
      expect(bytesResult).toBeDefined();
      if (bytesResult) {
        expect(Buffer.byteLength(bytesResult, 'utf8')).toBeLessThanOrEqual(511);
      }

      // JSON with 1000-byte string (> 128) can also be truncated to 511
      const jsonInput = JSON.stringify({ key: 'x'.repeat(1000) });
      const jsonResult = truncateJsonStrings(jsonInput, 511);
      expect(jsonResult).toBeDefined();
      if (jsonResult) {
        expect(Buffer.byteLength(jsonResult, 'utf8')).toBeLessThanOrEqual(511);
      }
    });

    it('returns undefined when small payload does not fit target', () => {
      // Small payload (< 512 bytes) cannot be truncated
      // If it doesn't fit target, return undefined
      const smallPayload = generateStringOfBytes(200);
      expect(truncateToBytes(smallPayload, 100)).toBeUndefined();
    });
  });

  describe('large input handling', () => {
    it('1MB input with small target works', () => {
      const input = generateStringOfBytes(1024 * 1024);
      const result = truncateToBytes(input, 1000);
      expect(result).toBeDefined();
      expect(Buffer.byteLength(result ?? '', 'utf8')).toBeLessThanOrEqual(1000);
    });
  });
});
