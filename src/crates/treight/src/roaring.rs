use crate::raw::RawBitmap;
use roaring::RoaringBitmap;

impl RawBitmap {
    /// Build from a `RoaringBitmap` with an explicit universe size,
    /// appending tree bytes to `out`.
    pub fn from_roaring(rb: &RoaringBitmap, universe_size: u32, out: &mut Vec<u8>) -> Self {
        Self::from_sorted_iter(rb.iter(), universe_size, out)
    }

    /// Convert this bitmap to a `RoaringBitmap`.
    pub fn to_roaring(&self, data: &[u8]) -> RoaringBitmap {
        RoaringBitmap::from_sorted_iter(self.iter(data)).unwrap()
    }
}
