//! `RoaringBitmap` interop for [`RawBitmap`]: two thin delegation methods,
//! behind the optional `roaring` feature (module gate in lib.rs:5-6; the
//! dep is the netdata/roaring-rs fork, src/crates/Cargo.toml:118). Only
//! sfst enables the feature in the workspace
//! (src/crates/sfst/Cargo.toml:31); the standalone fuzz workspace turns it
//! on too, so cargo unifies the two roaring copies (fuzz/Cargo.toml:12-16).
//! No wrapper type, no `From`/`Into` impls, no bridge on inverted `Bitmap`.
//! Grep-verified callers: treight's own differential tests only
//! (tests_roaring.rs:526-582) — no production call site.
use crate::raw::RawBitmap;
use roaring::RoaringBitmap;

impl RawBitmap {
    /// Build from a `RoaringBitmap`, appending tree bytes to `out`.
    ///
    /// The universe is treight's, not roaring's: the caller picks
    /// `universe_size`, which becomes the descriptor's exclusive upper
    /// bound. `rb.iter()` yields ascending values, so `from_sorted_iter`'s
    /// sorted contract holds by construction; an empty `rb` appends nothing
    /// and returns the empty descriptor for that universe. No bounds check
    /// (unlike `from_range`): values at or past `universe_size` still emit
    /// node bytes — orphan subtrees beyond `8^levels`, or set bits `iter`
    /// reports but `contains` rejects past the universe — so the caller
    /// must keep `rb` within `0..universe_size`.
    pub fn from_roaring(rb: &RoaringBitmap, universe_size: u32, out: &mut Vec<u8>) -> Self {
        Self::from_sorted_iter(rb.iter(), universe_size, out)
    }

    /// Convert this bitmap to a `RoaringBitmap`: the tree's set bits (always
    /// strictly ascending) feed `RoaringBitmap::from_sorted_iter`, which
    /// fails only on non-ascending input — the `unwrap` asserts that
    /// iterator invariant, it cannot fire on a well-formed tree. An empty
    /// blob yields an empty `RoaringBitmap`, and the conversion round-trips
    /// [`from_roaring`](Self::from_roaring) exactly
    /// (tests_roaring.rs:571).
    pub fn to_roaring(&self, data: &[u8]) -> RoaringBitmap {
        RoaringBitmap::from_sorted_iter(self.iter(data)).unwrap()
    }
}
