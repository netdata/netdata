use atoi::atoi;

/// Check if a byte is whitespace (space or tab)
fn is_whitespace(b: u8) -> bool {
    b == b' ' || b == b'\t'
}

/// Word iterator for parsing space-separated arguments
pub(crate) struct WordIterator<'a> {
    buffer: &'a [u8],
    pos: usize,
}

impl<'a> WordIterator<'a> {
    /// Create a new word iterator
    pub(crate) fn new(buffer: &'a [u8]) -> Self {
        Self { buffer, pos: 0 }
    }

    pub(crate) fn remainder(&self) -> &'a [u8] {
        let mut pos = self.pos;

        // Skip leading whitespace
        while pos < self.buffer.len() && is_whitespace(self.buffer[pos]) {
            pos += 1;
        }

        if pos < self.buffer.len() {
            &self.buffer[pos..]
        } else {
            b""
        }
    }

    pub(crate) fn next_string(&mut self) -> Option<String> {
        let s = self.next()?;

        Some(String::from_utf8_lossy(s).into_owned())
    }

    pub(crate) fn next_u32(&mut self) -> Option<u32> {
        let s = self.next()?;

        atoi(s)
    }

    pub(crate) fn next_u64(&mut self) -> Option<u64> {
        let s = self.next()?;

        atoi(s)
    }

    pub(crate) fn next_str(&mut self) -> Option<&str> {
        let s = self.next()?;
        std::str::from_utf8(s).ok()
    }
}

impl<'a> Iterator for WordIterator<'a> {
    type Item = &'a [u8];

    fn next(&mut self) -> Option<Self::Item> {
        // Skip whitespace
        while self.pos < self.buffer.len() && is_whitespace(self.buffer[self.pos]) {
            self.pos += 1;
        }

        if self.pos >= self.buffer.len() {
            return None;
        }

        let start = self.pos;

        // Check for quoted string
        if self.buffer[self.pos] == b'"' || self.buffer[self.pos] == b'\'' {
            let quote = self.buffer[self.pos];
            self.pos += 1; // Skip opening quote

            // Find closing quote (with basic escape support)
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

            // Unclosed quote - return rest of buffer
            Some(&self.buffer[start + 1..])
        } else {
            // Non-quoted word
            while self.pos < self.buffer.len() && !is_whitespace(self.buffer[self.pos]) {
                self.pos += 1;
            }

            Some(&self.buffer[start..self.pos])
        }
    }
}
