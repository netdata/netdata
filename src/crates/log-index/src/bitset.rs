/// A simple bitset for sorting unique u32 values without comparison sort.
///
/// Set bits for each value, then iterate set bits in order — they come out
/// sorted. Much faster than `sort_unstable()` for dense bitmaps because
/// it's O(n) to set + O(universe/64) to scan, vs O(n log n) for comparison
/// sort.
pub struct Bitset {
    words: Vec<u64>,
}

impl Bitset {
    pub fn new(universe_size: u32) -> Self {
        let num_words = (universe_size as usize + 63) / 64;
        Self {
            words: vec![0u64; num_words],
        }
    }

    #[inline]
    pub fn set(&mut self, val: u32) {
        let idx = val as usize;
        self.words[idx / 64] |= 1u64 << (idx % 64);
    }

    #[inline]
    pub fn clear(&mut self, val: u32) {
        let idx = val as usize;
        self.words[idx / 64] &= !(1u64 << (idx % 64));
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

    /// Iterate zero bits in ascending order, up to `universe_size`.
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
