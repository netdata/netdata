//! Compressed bitmap for efficient set operations on entry indices.

use roaring::RoaringBitmap;
use serde::{Deserialize, Serialize};

/// A compressed bitmap representing a set of journal entry indices.
///
/// Wraps [`RoaringBitmap`] and supports bitwise AND/OR operations for combining filters.
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
    pub fn from_sorted_iter<I: IntoIterator<Item = u32>>(
        iterator: I,
    ) -> Result<Bitmap, roaring::NonSortedIntegers> {
        RoaringBitmap::from_sorted_iter(iterator).map(Bitmap)
    }

    /// Create a bitmap containing all integers in the given range.
    pub fn insert_range<R>(range: R) -> Self
    where
        R: std::ops::RangeBounds<u32>,
    {
        let mut bitmap = Self::new();
        RoaringBitmap::insert_range(&mut bitmap, range);
        bitmap
    }
}

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
