use crate::raw::{Iter, RawBitmap};

/// A bitmap that may store its complement for better compression.
///
/// For sparse bitmaps (few bits set), the set bits are stored directly.
/// For dense bitmaps (most bits set), the *unset* bits are stored instead,
/// and the meaning is inverted. This keeps the underlying `RawBitmap` small
/// at both extremes of the density spectrum.
///
/// This is a lightweight `Copy` descriptor. The actual tree data lives in
/// an external `&[u8]` / `&mut Vec<u8>` passed to each method.
#[derive(Copy, Clone, Debug)]
#[cfg_attr(feature = "serde", derive(serde::Serialize, serde::Deserialize))]
pub struct Bitmap {
    inner: RawBitmap,
    inverted: bool,
}

impl Bitmap {
    /// Create an empty bitmap (no bits set).
    pub fn empty(universe_size: u32) -> Self {
        Self {
            inner: RawBitmap::empty(universe_size),
            inverted: false,
        }
    }

    /// Create a full bitmap (all bits set).
    pub fn full(universe_size: u32) -> Self {
        Self {
            inner: RawBitmap::empty(universe_size),
            inverted: true,
        }
    }

    /// Build from a sorted iterator of **set** values, appending tree bytes to `out`.
    pub fn from_sorted_iter(
        iter: impl Iterator<Item = u32>,
        universe_size: u32,
        out: &mut Vec<u8>,
    ) -> Self {
        Self {
            inner: RawBitmap::from_sorted_iter(iter, universe_size, out),
            inverted: false,
        }
    }

    /// Build from a sorted iterator of **unset** values (the complement),
    /// appending tree bytes to `out`.
    pub fn from_sorted_iter_complemented(
        complement_iter: impl Iterator<Item = u32>,
        universe_size: u32,
        out: &mut Vec<u8>,
    ) -> Self {
        Self {
            inner: RawBitmap::from_sorted_iter(complement_iter, universe_size, out),
            inverted: true,
        }
    }

    /// The universe size (exclusive upper bound on values).
    pub fn universe_size(&self) -> u32 {
        self.inner.universe_size()
    }

    /// Test whether `value` is in the bitmap.
    pub fn contains(&self, data: &[u8], value: u32) -> bool {
        if value >= self.inner.universe_size() {
            return false;
        }
        self.inner.contains(data, value) ^ self.inverted
    }

    /// Count the number of set bits.
    pub fn len(&self, data: &[u8]) -> u64 {
        if self.inverted {
            self.inner.universe_size() as u64 - self.inner.len(data)
        } else {
            self.inner.len(data)
        }
    }

    /// Returns `true` if no bits are set.
    pub fn is_empty(&self, data: &[u8]) -> bool {
        if self.inverted {
            self.inner.len(data) == self.inner.universe_size() as u64
        } else {
            self.inner.is_empty(data)
        }
    }

    /// Whether this bitmap uses inverted (complemented) representation.
    pub fn is_inverted(&self) -> bool {
        self.inverted
    }

    /// Access the underlying raw bitmap descriptor.
    pub fn inner(&self) -> RawBitmap {
        self.inner
    }

    /// Build a bitmap with all values in the given range set,
    /// appending tree bytes to `out`.
    pub fn from_range(
        range: impl std::ops::RangeBounds<u32>,
        universe_size: u32,
        out: &mut Vec<u8>,
    ) -> Self {
        use std::ops::Bound;

        let start = match range.start_bound() {
            Bound::Included(&n) => n,
            Bound::Excluded(&n) => n.saturating_add(1),
            Bound::Unbounded => 0,
        };
        let end = match range.end_bound() {
            Bound::Included(&n) => n.saturating_add(1).min(universe_size),
            Bound::Excluded(&n) => n.min(universe_size),
            Bound::Unbounded => universe_size,
        };

        if start >= end {
            return Self::empty(universe_size);
        }

        let range_len = (end - start) as u64;
        let half_universe = universe_size as u64 / 2;

        if range_len > half_universe {
            let complement = (0..start).chain(end..universe_size);
            Self::from_sorted_iter_complemented(complement, universe_size, out)
        } else {
            Self::from_sorted_iter(start..end, universe_size, out)
        }
    }

    /// Iterate over set bits in ascending order.
    pub fn iter<'a>(&self, data: &'a [u8]) -> BitmapIter<'a> {
        if self.inverted {
            BitmapIter::Complement(ComplementIter {
                raw_iter: self.inner.iter(data),
                next_raw: None,
                current: 0,
                universe_size: self.inner.universe_size(),
                started: false,
            })
        } else {
            BitmapIter::Normal(self.inner.iter(data))
        }
    }

    /// Count the number of set bits within a range.
    pub fn range_cardinality(&self, data: &[u8], range: impl std::ops::RangeBounds<u32>) -> u64 {
        use std::ops::Bound;

        if self.inverted {
            let start = match range.start_bound() {
                Bound::Included(&n) => n,
                Bound::Excluded(&n) => n.saturating_add(1),
                Bound::Unbounded => 0,
            };
            let end = match range.end_bound() {
                Bound::Included(&n) => n.saturating_add(1),
                Bound::Excluded(&n) => n,
                Bound::Unbounded => self.inner.universe_size(),
            };
            let end = end.min(self.inner.universe_size());

            if start >= end {
                return 0;
            }

            let range_len = (end - start) as u64;
            let raw_count = self.inner.range_cardinality(data, start..end);
            range_len - raw_count
        } else {
            self.inner.range_cardinality(data, range)
        }
    }

    /// Intersection using De Morgan's dispatch.
    ///
    /// - N & N -> A intersect B (normal)
    /// - N & I -> A difference B (normal)
    /// - I & N -> B difference A (normal)
    /// - I & I -> A union B (inverted)
    pub fn and(&self, a: &[u8], other: &Bitmap, b: &[u8], out: &mut Vec<u8>) -> Bitmap {
        // Short-circuit: empty AND anything = empty.
        if self.is_empty(a) || other.is_empty(b) {
            return Bitmap::empty(self.inner.universe_size());
        }

        // Short-circuit: full AND anything = anything.
        if self.inverted && self.inner.is_empty(a) {
            out.extend_from_slice(b);
            return *other;
        }
        if other.inverted && other.inner.is_empty(b) {
            out.extend_from_slice(a);
            return *self;
        }

        debug_assert_eq!(
            self.inner.universe_size(),
            other.inner.universe_size(),
            "universe_size mismatch: {} vs {}",
            self.inner.universe_size(),
            other.inner.universe_size()
        );

        let (inner, inverted) = match (self.inverted, other.inverted) {
            (false, false) => (self.inner.intersect(a, &other.inner, b, out), false),
            (false, true) => (self.inner.difference(a, &other.inner, b, out), false),
            (true, false) => (other.inner.difference(b, &self.inner, a, out), false),
            (true, true) => (self.inner.union(a, &other.inner, b, out), true),
        };

        Bitmap { inner, inverted }
    }

    /// Union using De Morgan's dispatch.
    ///
    /// - N | N -> A union B (normal)
    /// - N | I -> B difference A (inverted)
    /// - I | N -> A difference B (inverted)
    /// - I | I -> A intersect B (inverted)
    pub fn or(&self, a: &[u8], other: &Bitmap, b: &[u8], out: &mut Vec<u8>) -> Bitmap {
        // Short-circuit: empty OR anything = anything.
        if self.is_empty(a) {
            out.extend_from_slice(b);
            return *other;
        }
        if other.is_empty(b) {
            out.extend_from_slice(a);
            return *self;
        }

        // Short-circuit: full OR anything = full.
        if self.inverted && self.inner.is_empty(a) {
            return *self;
        }
        if other.inverted && other.inner.is_empty(b) {
            return *other;
        }

        debug_assert_eq!(
            self.inner.universe_size(),
            other.inner.universe_size(),
            "universe_size mismatch: {} vs {}",
            self.inner.universe_size(),
            other.inner.universe_size()
        );

        let (inner, inverted) = match (self.inverted, other.inverted) {
            (false, false) => (self.inner.union(a, &other.inner, b, out), false),
            (false, true) => (other.inner.difference(b, &self.inner, a, out), true),
            (true, false) => (self.inner.difference(a, &other.inner, b, out), true),
            (true, true) => (self.inner.intersect(a, &other.inner, b, out), true),
        };

        Bitmap { inner, inverted }
    }
}

/// Iterator over set bits of a [`Bitmap`].
pub enum BitmapIter<'a> {
    /// Normal: yields values present in the raw bitmap.
    Normal(Iter<'a>),
    /// Complement: yields values in `0..universe_size` NOT in the raw bitmap.
    Complement(ComplementIter<'a>),
}

impl Iterator for BitmapIter<'_> {
    type Item = u32;

    fn next(&mut self) -> Option<u32> {
        match self {
            BitmapIter::Normal(iter) => iter.next(),
            BitmapIter::Complement(iter) => iter.next(),
        }
    }
}

/// Iterator that yields values in `0..universe_size` that are NOT in the raw bitmap.
pub struct ComplementIter<'a> {
    raw_iter: Iter<'a>,
    next_raw: Option<u32>,
    current: u32,
    universe_size: u32,
    started: bool,
}

impl Iterator for ComplementIter<'_> {
    type Item = u32;

    fn next(&mut self) -> Option<u32> {
        if !self.started {
            self.next_raw = self.raw_iter.next();
            self.started = true;
        }

        loop {
            if self.current >= self.universe_size {
                return None;
            }

            let val = self.current;
            self.current += 1;

            match self.next_raw {
                Some(raw_val) if raw_val == val => {
                    self.next_raw = self.raw_iter.next();
                    continue;
                }
                _ => return Some(val),
            }
        }
    }
}
