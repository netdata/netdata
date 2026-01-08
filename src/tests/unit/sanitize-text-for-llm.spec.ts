import { describe, expect, it } from 'vitest';

import { sanitizeTextForLLM } from '../../utils.js';

describe('sanitizeTextForLLM', () => {
  describe('passthrough behavior', () => {
    it('returns empty string unchanged', () => {
      expect(sanitizeTextForLLM('')).toBe('');
    });

    it('returns normal ASCII text unchanged', () => {
      const text = 'Hello, World! This is normal text.';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });

    it('returns text with newlines and special chars unchanged', () => {
      const text = 'Line 1\nLine 2\tTabbed\r\nWindows line';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });

    it('preserves valid emoji (surrogate pairs)', () => {
      const text = 'Hello 🎉 World 🚀 Test 😀';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });

    it('preserves Chinese characters', () => {
      const text = '你好世界';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });
  });

  describe('NFKC normalization (gated)', () => {
    it('normalizes Mathematical Bold text to ASCII', () => {
      // 𝗗𝗮𝘁𝗮𝗱𝗼𝗴 (Mathematical Bold) -> Datadog
      const mathBold = '𝗗𝗮𝘁𝗮𝗱𝗼𝗴';
      expect(sanitizeTextForLLM(mathBold)).toBe('Datadog');
    });

    it('normalizes Mathematical Bold in mixed text', () => {
      const text = 'Check out 𝗗𝗮𝘁𝗮𝗱𝗼𝗴 and 𝗦𝗰𝗶𝗲𝗻𝗰𝗲𝗟𝗼𝗴𝗶𝗰 comparison';
      const expected = 'Check out Datadog and ScienceLogic comparison';
      expect(sanitizeTextForLLM(text)).toBe(expected);
    });

    it('normalizes Mathematical Italic text', () => {
      // 𝐻𝑒𝑙𝑙𝑜 (Mathematical Italic) -> Hello
      const mathItalic = '𝐻𝑒𝑙𝑙𝑜';
      expect(sanitizeTextForLLM(mathItalic)).toBe('Hello');
    });

    it('does NOT apply NFKC when no math symbols present (preserves ligatures)', () => {
      // The ligature ﬁ should be preserved when no math symbols trigger NFKC
      const text = 'ﬁnd the ﬁle';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });

    it('does NOT apply NFKC when no math symbols present (preserves fullwidth)', () => {
      // Fullwidth A should be preserved when no math symbols trigger NFKC
      const text = 'Ａ fullwidth char';
      expect(sanitizeTextForLLM(text)).toBe(text);
    });
  });

  describe('unpaired surrogate removal', () => {
    it('removes unpaired high surrogate', () => {
      // \uD835 alone (high surrogate without low)
      const text = 'before\uD835after';
      expect(sanitizeTextForLLM(text)).toBe('beforeafter');
    });

    it('removes unpaired low surrogate', () => {
      // \uDC00 alone (low surrogate without high)
      const text = 'before\uDC00after';
      expect(sanitizeTextForLLM(text)).toBe('beforeafter');
    });

    it('removes multiple unpaired surrogates', () => {
      const text = 'a\uD800b\uDC00c\uD835d';
      expect(sanitizeTextForLLM(text)).toBe('abcd');
    });

    it('removes unpaired surrogate at start', () => {
      const text = '\uD835hello';
      expect(sanitizeTextForLLM(text)).toBe('hello');
    });

    it('removes unpaired surrogate at end', () => {
      const text = 'hello\uD835';
      expect(sanitizeTextForLLM(text)).toBe('hello');
    });

    it('preserves valid surrogate pairs while removing unpaired', () => {
      // Mix of valid emoji and unpaired surrogate
      const text = 'hello\uD835🎉world';
      expect(sanitizeTextForLLM(text)).toBe('hello🎉world');
    });
  });

  describe('edge cases', () => {
    it('handles string with only unpaired surrogates', () => {
      // low-high-high pattern: low has no preceding high, first high's next is another high (not low), second high has no next
      const text = '\uDC00\uD835\uD800';
      expect(sanitizeTextForLLM(text)).toBe('');
    });

    it('keeps valid pair when preceded by unpaired high', () => {
      // \uD835 is unpaired, but \uD800\uDC00 is valid pair (U+10000)
      const text = '\uD835\uD800\uDC00';
      expect(sanitizeTextForLLM(text)).toBe('𐀀');
    });

    it('handles consecutive valid surrogate pairs', () => {
      const text = '🎉🚀😀';
      expect(sanitizeTextForLLM(text)).toBe('🎉🚀😀');
    });

    it('handles math symbols followed by emoji', () => {
      // This will trigger NFKC, but emoji should still work
      const text = '𝗗𝗮𝘁𝗮🎉';
      const result = sanitizeTextForLLM(text);
      expect(result).toContain('Data');
      expect(result).toContain('🎉');
    });
  });
});
