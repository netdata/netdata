//! The roaring-bitmap entry sets behind the journal index: `Bitmap` wraps
//! the `roaring` crate's `u32` `RoaringBitmap`, and [`crate::FileIndex`]
//! stores one per indexed field=value pair in its `bitmaps` map.
//!
//! Flow: `file_indexer` builds each bitmap from the file's entry indices
//! (`build_entries_index`), `filter` AND/OR-combines them into the
//! entry set matching a query ([`crate::Filter::evaluate`]), and
//! `file_index` and `histogram` turn the surviving indices into entry
//! offsets ([`crate::FileIndex::find_log_entries`]) and time-bucket counts
//! ([`crate::Histogram::count_entries_in_time_range`]).
//! journal-engine uses [`Bitmap::insert_range`] the same way to get each
//! histogram bucket's unfiltered entry count
//! (`journal-engine/src/histogram.rs`).
//!
//! This file touches only `roaring` and `serde` (plus the feature-gated
//! `allocative` derive below); the crate's journal-core, journal-common
//! and journal-registry dependencies sit in the other modules. The
//! workspace pins `roaring` to Netdata's fork
//! (`src/crates/Cargo.toml`), which carries the allocative support the
//! crate's `allocative` feature forwards (`roaring/allocative`), and the
//! `serde` feature supplies the inner serialization that
//! `#[serde(transparent)]` below relies on.

use roaring::RoaringBitmap;
use serde::{Deserialize, Serialize};

/// A compressed set of `u32` journal entry indices, backed by the Roaring
/// bitmap compression scheme.
///
/// Newtype over `RoaringBitmap` with a public tuple field. The
/// `BitAnd`/`BitOrAssign` impls below intersect and union sets; `Deref` exposes
/// the rest of the `RoaringBitmap` API. Serialization is `transparent`, so
/// within a serialized `FileIndex` each bitmap is the inner bitmap's bytes
/// in Roaring's standard on-disk format.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
#[serde(transparent)]
pub struct Bitmap(pub RoaringBitmap);

impl Bitmap {
    /// Create an empty bitmap.
    pub fn new() -> Self {
        Self(RoaringBitmap::new())
    }

    /// Create a bitmap from a sorted iterator of entry indices.
    ///
    /// The values must be strictly increasing; otherwise this fails with
    /// `roaring::NonSortedIntegers`, whose `valid_until` reports how many
    /// values were consumed before the first out-of-order one. In-crate
    /// callers pre-sort (`build_entries_index` in `src/file_indexer.rs`).
    pub fn from_sorted_iter<I: IntoIterator<Item = u32>>(
        iterator: I,
    ) -> Result<Bitmap, roaring::NonSortedIntegers> {
        RoaringBitmap::from_sorted_iter(iterator).map(Bitmap)
    }

    /// Create a bitmap containing all integers in the given range.
    ///
    /// This associated constructor is distinct from the same-named
    /// `RoaringBitmap::insert_range` method reachable on an instance through
    /// `DerefMut`: that one inserts into an existing bitmap and returns the
    /// number of newly inserted values, a count this discards because the
    /// bitmap starts empty. [`crate::FileIndex::find_log_entries`] uses this
    /// as the full-coverage bitmap for filterless queries.
    pub fn insert_range<R>(range: R) -> Self
    where
        R: std::ops::RangeBounds<u32>,
    {
        let mut bitmap = Self::new();
        RoaringBitmap::insert_range(&mut bitmap, range);
        bitmap
    }
}

// Expose the whole `RoaringBitmap` API (len, contains, iter, is_empty,
// range_cardinality, optimize, ...) directly on `Bitmap`.
impl std::ops::Deref for Bitmap {
    type Target = RoaringBitmap;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl std::ops::DerefMut for Bitmap {
    fn deref_mut(&mut self) -> &mut Self::Target {
        &mut self.0
    }
}

impl From<RoaringBitmap> for Bitmap {
    fn from(bitmap: RoaringBitmap) -> Self {
        Self(bitmap)
    }
}

impl From<Bitmap> for RoaringBitmap {
    fn from(wrapper: Bitmap) -> Self {
        wrapper.0
    }
}

// Intersection (`&`) in every by-value/by-reference combination and union
// only as `|=` (there is no by-value `|` impl), each delegating to the
// matching `RoaringBitmap` operation. Only borrowed operands remain
// unconsumed; `filter` folds a resolved filter expression into a single
// entry set with `&=`/`|=` (`src/filter.rs` `FilterExpr::evaluate`).
impl std::ops::BitAndAssign<&Bitmap> for Bitmap {
    fn bitand_assign(&mut self, rhs: &Bitmap) {
        self.0 &= &rhs.0;
    }
}

impl std::ops::BitAndAssign<Bitmap> for Bitmap {
    fn bitand_assign(&mut self, rhs: Bitmap) {
        self.0 &= rhs.0;
    }
}

impl std::ops::BitOrAssign<&Bitmap> for Bitmap {
    fn bitor_assign(&mut self, rhs: &Bitmap) {
        self.0 |= &rhs.0;
    }
}

impl std::ops::BitOrAssign<Bitmap> for Bitmap {
    fn bitor_assign(&mut self, rhs: Bitmap) {
        self.0 |= rhs.0;
    }
}

impl std::ops::BitAnd for &Bitmap {
    type Output = Bitmap;

    fn bitand(self, rhs: &Bitmap) -> Bitmap {
        Bitmap(&self.0 & &rhs.0)
    }
}

impl std::ops::BitAnd<Bitmap> for &Bitmap {
    type Output = Bitmap;

    fn bitand(self, rhs: Bitmap) -> Bitmap {
        Bitmap(&self.0 & rhs.0)
    }
}

impl std::ops::BitAnd<&Bitmap> for Bitmap {
    type Output = Bitmap;

    fn bitand(self, rhs: &Bitmap) -> Bitmap {
        Bitmap(self.0 & &rhs.0)
    }
}

impl std::ops::BitAnd for Bitmap {
    type Output = Bitmap;

    fn bitand(self, rhs: Bitmap) -> Bitmap {
        Bitmap(self.0 & rhs.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_from_sorted_iter() {
        let bitmap = Bitmap::from_sorted_iter([0, 5, 10, 15]).expect("sorted iterator");

        assert_eq!(bitmap.len(), 4);
        assert!(bitmap.contains(5));
        assert!(!bitmap.contains(6));
    }

    #[test]
    fn test_from_sorted_iter_rejects_unsorted() {
        let result = Bitmap::from_sorted_iter([10, 5, 15]);
        assert!(result.is_err());
    }

    #[test]
    fn test_insert_range() {
        let bitmap = Bitmap::insert_range(10..15);

        assert_eq!(bitmap.len(), 5);
        assert!(bitmap.contains(10));
        assert!(bitmap.contains(14));
        assert!(!bitmap.contains(15));
    }

    #[test]
    fn test_bitwise_operations() {
        let bitmap1 = Bitmap::from_sorted_iter([1, 2, 3]).expect("sorted");
        let bitmap2 = Bitmap::from_sorted_iter([2, 3, 4]).expect("sorted");

        let intersection = &bitmap1 & &bitmap2;
        assert_eq!(intersection.len(), 2);

        let mut union = bitmap1.clone();
        union |= bitmap2;
        assert_eq!(union.len(), 4);
    }
}
