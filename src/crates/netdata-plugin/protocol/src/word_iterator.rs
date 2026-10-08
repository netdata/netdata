//! Bottom layer of the crate's pluginsd parser stack: splits one pluginsd
//! line into space/tab-separated words, yielding borrowed zero-copy slices
//! with quotes stripped and escape bytes kept verbatim. `line_parser` takes
//! the first word as the keyword and keeps the rest of the line as raw
//! argument bytes via [`WordIterator::remainder`]; `message_parser`
//! re-iterates argument bytes into typed fields through the `next_*`
//! helpers.
//!
//! Splitting ports the agent's `quoted_strings_splitter`
//! (`src/libnetdata/line_splitter/line_splitter.h`, driven with
//! `isspace_map_pluginsd`) and shares its quote semantics: a word may open
//! with `'` or `"`, closes at the matching quote, and whatever follows the
//! closing quote starts a new word — an embedded quote therefore closes the
//! field early, while a quote of the other kind inside a field is an
//! ordinary byte. Differences from the C splitter:
//!
//! - Separators are only `' '` and `'\t'`; the C pluginsd set also splits
//!   on `\r`, `\n`, `\f`, `\v` and `=` (a `\n` never occurs: callers feed
//!   it bytes of a single line).
//! - Escapes work only inside a quoted field: the `\` + byte pair is
//!   skipped while scanning, so `\"` and `\'` cannot close the field, and
//!   the pair's bytes stay in the word (the C side keeps them too —
//!   backslash removal is unimplemented there). In an unquoted word the C
//!   splitter treats `\` as escaping the next byte, protecting it from
//!   splitting; here `\` is an ordinary byte.
//! - An unclosed quote turns the rest of the input into one word with the
//!   opening quote stripped, as on the C side; a lone quote at the end of
//!   the input yields an empty word on both sides.
//! - The C splitter stops after `PLUGINSD_MAX_WORDS` (30) words; this
//!   iterator yields all of them.
//!
//! Crate-private: only `line_parser` and `message_parser` use it.
use atoi::atoi;

/// True for the two word separators this iterator recognizes: space and tab.
fn is_whitespace(b: u8) -> bool {
    b == b' ' || b == b'\t'
}

/// Iterator over the words of one pluginsd line: borrows a byte buffer and
/// yields successive word slices from it. The splitting rules are on the
/// `Iterator` impl; see the module header for the agent-side contract.
pub(crate) struct WordIterator<'a> {
    buffer: &'a [u8],
    pos: usize,
}

impl<'a> WordIterator<'a> {
    /// Iterates over the words of `buffer`, starting at its first byte.
    pub(crate) fn new(buffer: &'a [u8]) -> Self {
        Self { buffer, pos: 0 }
    }

    /// The unconsumed tail of the buffer after its leading whitespace, as a
    /// borrowed slice; `b""` when only whitespace is left. Does not advance the
    /// iterator and leaves quotes and escapes in the bytes untouched.
    /// `line_parser` calls this once, after the first word (the keyword), to
    /// keep the rest of the line as raw argument bytes.
    pub(crate) fn remainder(&self) -> &'a [u8] {
        let mut pos = self.pos;

        while pos < self.buffer.len() && is_whitespace(self.buffer[pos]) {
            pos += 1;
        }

        if pos < self.buffer.len() {
            &self.buffer[pos..]
        } else {
            b""
        }
    }

    /// Consumes the next word, decoded to an owned `String` with invalid UTF-8
    /// replaced by U+FFFD. `None` only when the words are exhausted — unlike
    /// [`Self::next_str`], encoding problems never drop the word.
    pub(crate) fn next_string(&mut self) -> Option<String> {
        let s = self.next()?;

        Some(String::from_utf8_lossy(s).into_owned())
    }

    /// Consumes the next word and parses its leading decimal with `atoi`:
    /// `None` when the words are exhausted, the word does not start with a
    /// number, or the value overflows `u32`. Bytes after the digits are ignored
    /// (`"30s"` is 30); negative values are rejected for this unsigned type.
    pub(crate) fn next_u32(&mut self) -> Option<u32> {
        let s = self.next()?;

        atoi(s)
    }

    /// Like [`Self::next_u32`], parsing `u64`.
    pub(crate) fn next_u64(&mut self) -> Option<u64> {
        let s = self.next()?;

        atoi(s)
    }

    /// Like [`Self::next_u32`], parsing `usize`.
    pub(crate) fn next_usize(&mut self) -> Option<usize> {
        let s = self.next()?;

        atoi(s)
    }

    /// Consumes the next word as `&str`, valid UTF-8 only. `None` means the
    /// words are exhausted or the word was not valid UTF-8 — the word is
    /// consumed either way and not distinguishable afterwards; use
    /// [`Self::next_string`] for lossy decoding.
    pub(crate) fn next_str(&mut self) -> Option<&str> {
        let s = self.next()?;
        std::str::from_utf8(s).ok()
    }
}

impl<'a> Iterator for WordIterator<'a> {
    type Item = &'a [u8];

    /// Returns the next word, or `None` after the last one. Separators are
    /// skipped; a word opening with `'` or `"` ends at the matching quote, which
    /// is excluded from the word, and an unclosed quote yields the rest of the
    /// buffer without the opening quote.
    fn next(&mut self) -> Option<Self::Item> {
        while self.pos < self.buffer.len() && is_whitespace(self.buffer[self.pos]) {
            self.pos += 1;
        }

        if self.pos >= self.buffer.len() {
            return None;
        }

        let start = self.pos;

        if self.buffer[self.pos] == b'"' || self.buffer[self.pos] == b'\'' {
            let quote = self.buffer[self.pos];
            self.pos += 1; // Skip opening quote

            // Scan for the matching quote; a `\` + byte pair is skipped as a
            // unit, so an escaped quote cannot close the field.
            while self.pos < self.buffer.len() {
                if self.buffer[self.pos] == b'\\' && self.pos + 1 < self.buffer.len() {
                    self.pos += 2; // Skip escape sequence
                } else if self.buffer[self.pos] == quote {
                    let word = &self.buffer[start + 1..self.pos]; // Exclude quotes
                    self.pos += 1; // Skip closing quote
                    return Some(word);
                } else {
                    self.pos += 1;
                }
            }

            // Unclosed quote: the rest of the buffer becomes the word, without the opening quote.
            Some(&self.buffer[start + 1..])
        } else {
            // Plain word: no escape handling here, it runs to the next separator.
            while self.pos < self.buffer.len() && !is_whitespace(self.buffer[self.pos]) {
                self.pos += 1;
            }

            Some(&self.buffer[start..self.pos])
        }
    }
}
