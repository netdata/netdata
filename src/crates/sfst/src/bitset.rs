/// A dense bitset over `u32` values, stored as plain `Vec<u64>` words.
/// Scratch sort for `remap_one_bitmap` in `build.rs`:
/// set a bit per remapped value, then replay the set (or unset) bits in
/// ascending order — O(n) to set + O(universe/64) to scan, cheaper than
/// `sort_unstable()` once cardinality reaches the max(universe/64, 256)
/// threshold in `build.rs` `remap_one_bitmap`. Unrelated to `treight::Bitmap`: no shared
/// layout or conversion — the iterators only feed
/// `Bitmap::from_sorted_iter` and `Bitmap::from_sorted_iter_complemented`.
pub struct Bitset {
    words: Vec<u64>,
}

impl Bitset {
    /// Allocate zeroed words covering `[0, universe_size)`.
    pub fn new(universe_size: u32) -> Self {
        let num_words = (universe_size as usize).div_ceil(64);
        Self {
            words: vec![0u64; num_words],
        }
    }

    /// Set the bit for `val`. Unchecked: values must be < `universe_size` —
    /// past the last word this panics on the word index; inside it, the
    /// stray bit leaks into `iter_ones` output.
    #[inline]
    pub fn set(&mut self, val: u32) {
        let idx = val as usize;
        self.words[idx / 64] |= 1u64 << (idx % 64);
    }

    /// Iterate set bits in ascending order.
    pub fn iter_ones(&self) -> BitsetOnesIter<'_> {
        BitsetOnesIter {
            words: &self.words,
            word_idx: 0,
            current_word: if self.words.is_empty() {
                0
            } else {
                self.words[0]
            },
        }
    }

    /// Iterate the unset bits in ascending order, below `universe_size`.
    /// Only the words allocated by `new` are scanned, so pass the same
    /// `universe_size` the bitset was created with.
    pub fn iter_zeros(&self, universe_size: u32) -> BitsetZerosIter<'_> {
        BitsetZerosIter {
            words: &self.words,
            word_idx: 0,
            current_word: if self.words.is_empty() {
                0
            } else {
                !self.words[0]
            },
            universe_size,
        }
    }
}

/// Ascending iterator over set bits; see `Bitset::iter_ones`.
pub struct BitsetOnesIter<'a> {
    words: &'a [u64],
    word_idx: usize,
    current_word: u64,
}

impl Iterator for BitsetOnesIter<'_> {
    type Item = u32;

    #[inline]
    fn next(&mut self) -> Option<u32> {
        loop {
            if self.current_word != 0 {
                let bit = self.current_word.trailing_zeros();
                self.current_word &= self.current_word - 1; // clear lowest set bit
                return Some((self.word_idx * 64 + bit as usize) as u32);
            }
            self.word_idx += 1;
            if self.word_idx >= self.words.len() {
                return None;
            }
            self.current_word = self.words[self.word_idx];
        }
    }
}

/// Ascending iterator over unset bits; see `Bitset::iter_zeros`.
pub struct BitsetZerosIter<'a> {
    words: &'a [u64],
    word_idx: usize,
    current_word: u64, // inverted: set bits represent zeros in the original
    universe_size: u32,
}

impl Iterator for BitsetZerosIter<'_> {
    type Item = u32;

    #[inline]
    fn next(&mut self) -> Option<u32> {
        loop {
            if self.current_word != 0 {
                let bit = self.current_word.trailing_zeros();
                self.current_word &= self.current_word - 1;
                let val = (self.word_idx * 64 + bit as usize) as u32;
                if val >= self.universe_size {
                    return None;
                }
                return Some(val);
            }
            self.word_idx += 1;
            if self.word_idx >= self.words.len() {
                return None;
            }
            self.current_word = !self.words[self.word_idx];
        }
    }
}
