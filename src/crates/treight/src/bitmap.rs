//! The inverted-aware bitmap layer over the raw tree: `Bitmap` pairs a
//! `RawBitmap` 8-way bit-tree descriptor with an `inverted` flag, so the
//! stored tree may encode either the set bits or their complement — sparse
//! sets pay only for their bits, dense sets only for their holes, and the
//! accessors and boolean ops flip meaning on the flag, keeping the choice of
//! representation invisible to callers. Tree bytes never live in the
//! descriptor: every method takes the external `&[u8]` / `&mut Vec<u8>` the
//! caller owns, so a `Bitmap` is a freely copyable, shareable handle.
//!
//! `and`/`or`/`and_not` dispatch on the two `inverted` flags (De Morgan
//! tables on those methods) to compute over the stored trees and return the
//! result in whichever form is natural. The convention callers can build on
//! — sfst's `PosSet` does (sfst/src/index_reader.rs `PosSet`) — is that the
//! bytes a call appends to `out` are exactly the result's tree bytes, so the
//! caller can swap its buffer wholesale after every call.
//!
//! Point mutation (`insert`/`remove`/`remove_range`), `min`/`max` and the
//! serialize/deserialize pair live on `RawBitmap` (raw.rs), which knows
//! nothing about inversion; this layer adds the inverted-aware query and
//! boolean surface instead. Consumers (grep-verified): sfst encodes value
//! bitmaps with `from_sorted_iter`/`from_sorted_iter_complemented`
//! (sfst/src/build.rs `remap_one_bitmap`) and `from_range`
//! (sfst/src/index_reader.rs `PosSet::range`), and persists them as
//! descriptor + tree bytes (sfst/src/schema.rs `BitmapValue`); sfsq and
//! otel-ledger touch `Bitmap` from tests only (sfsq/tests/common/mod.rs
//! `legacy_sfst_source`, otel-ledger/src/ledger/rpc/logs/handler/tests.rs
//! `bitmap_with`). The `serde` derive
//! covers the descriptor only — tree bytes ride along in the consumer's
//! payload beside it; the `roaring` feature (roaring.rs) bridges
//! `RoaringBitmap` and feeds the differential fuzz target
//! fuzz/fuzz_targets/against_roaring.rs.
use crate::raw::{Iter, RawBitmap};

/// A bitmap that may store its complement for better compression: a
/// [`RawBitmap`] tree plus an `inverted` flag, with every accessor flipping
/// meaning when the flag is set. Sparse bitmaps store their set bits
/// directly; dense ones store the holes (`from_range` picks automatically,
/// `full` stores nothing at all).
///
/// A lightweight `Copy` descriptor: the tree bytes live in the external
/// `&[u8]` / `&mut Vec<u8>` each method takes, so copies share data.
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

    /// Create a full bitmap (all bits set). Nothing is stored: `full` is an
    /// inverted descriptor over an empty tree, so it needs no tree bytes.
    pub fn full(universe_size: u32) -> Self {
        Self {
            inner: RawBitmap::empty(universe_size),
            inverted: true,
        }
    }

    /// Build from a sorted iterator of **set** values, appending tree bytes
    /// to `out`. Values must be ascending and within the universe — neither
    /// is checked (`RawBitmap::from_sorted_iter` trusts its input).
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

    /// Build from a sorted iterator of the values the bitmap does **not**
    /// contain (they become the stored tree), appending tree bytes to `out`;
    /// the result is inverted. Same ascending/universe requirements as
    /// [`from_sorted_iter`](Self::from_sorted_iter).
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

    /// Test whether `value` is in the bitmap. Values at or past the
    /// universe size return `false` — checked before the inversion is
    /// applied, so both representations agree.
    pub fn contains(&self, data: &[u8], value: u32) -> bool {
        if value >= self.inner.universe_size() {
            return false;
        }
        self.inner.contains(data, value) ^ self.inverted
    }

    /// Count the number of set bits. For the inverted form this is the
    /// universe size minus the stored complement's population.
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

    /// The complement over the universe. Free: the descriptor flips its
    /// `inverted` flag and keeps sharing the caller's existing tree bytes —
    /// nothing is copied or rebuilt.
    pub fn complement(&self) -> Bitmap {
        Bitmap {
            inner: self.inner,
            inverted: !self.inverted,
        }
    }

    /// Difference (`self \ other`) via `self ∩ complement(other)` —
    /// the [`and`](Self::and) dispatch already handles every
    /// inverted-flag combination.
    pub fn and_not(&self, a: &[u8], other: &Bitmap, b: &[u8], out: &mut Vec<u8>) -> Bitmap {
        self.and(a, &other.complement(), b, out)
    }

    /// Access the underlying raw bitmap descriptor — the *stored* tree. For
    /// an inverted bitmap this describes the complement, so it must be
    /// paired with the same tree bytes this bitmap reads.
    pub fn inner(&self) -> RawBitmap {
        self.inner
    }

    /// Build a bitmap with all values in the given range set, appending
    /// tree bytes to `out`. The end bound is clamped to the universe size;
    /// a range that is empty or starts at/after the clamped end yields an
    /// empty bitmap. Ranges covering more than half the universe store
    /// their complement instead (the descriptor comes back inverted) — the
    /// density trade [`from_sorted_iter_complemented`](Self::from_sorted_iter_complemented)
    /// exists for.
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

    /// Iterate over set bits in ascending order. Inverted bitmaps iterate
    /// through [`ComplementIter`], which walks the whole universe — a full
    /// pass costs O(universe_size), not the stored tree's size.
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

    /// Count the number of set bits within a range. Inverted bitmaps count
    /// the stored complement inside the universe-clamped range and subtract
    /// it from the range length.
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

    /// Intersection using De Morgan's dispatch:
    ///
    /// - N & N -> A intersect B (normal)
    /// - N & I -> A difference B (normal)
    /// - I & N -> B difference A (normal)
    /// - I & I -> A union B (inverted)
    ///
    /// N = normal, I = inverted; each row computes over the two stored
    /// trees and the result comes back in the listed form. The bytes
    /// appended to `out` are exactly the result's tree bytes (the full-side
    /// short-circuits copy the surviving operand's bytes into `out` first;
    /// the empty short-circuit appends none). The two operands must share a
    /// universe size: `debug_assert`ed before the dispatch (the raw ops
    /// also assert unconditionally once dispatched), but the empty/full
    /// short-circuits return without any check.
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

    /// Union using De Morgan's dispatch:
    ///
    /// - N | N -> A union B (normal)
    /// - N | I -> B difference A (inverted)
    /// - I | N -> A difference B (inverted)
    /// - I | I -> A intersect B (inverted)
    ///
    /// Same `out` convention and universe checks as [`and`](Self::and): the
    /// appended bytes are the result's tree bytes, with the empty
    /// short-circuits copying the surviving operand's bytes into `out` and
    /// the full ones appending none.
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

/// Iterator that yields, in ascending order, the values in
/// `0..universe_size` that are NOT in the raw bitmap — the read side of an
/// inverted [`Bitmap`]'s iteration. It walks `current` across the whole
/// universe and skips each value the raw iterator produces, so a full pass
/// costs O(universe_size) rather than the stored tree's size, and nothing
/// is materialized.
pub struct ComplementIter<'a> {
    raw_iter: Iter<'a>,
    /// The raw iterator's next stored value, primed on the first `next()`.
    /// `None` means exhausted; `started` distinguishes that from not yet
    /// primed.
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
