//! `treight` — tree + eight — is a compressed bitmap set over a fixed
//! universe, stored in an 8-way bit-tree: the layout used by GNU idutils
//! (node.rs/ops.rs match the fid.c routines they port). Each internal node
//! is one byte whose 8 bits say which of its 8 children exist, and empty
//! subtrees are pruned entirely. The serialized form IS the in-memory form
//! (`raw.rs`'s blob-shape contract): queries walk the stored bytes directly in O(levels),
//! and the blob is canonical — exactly [`estimate_data_size`] bytes for
//! the values it holds, whichever mutation path built it.
//!
//! API shape: every bitmap is a `Copy` descriptor (universe size + level
//! count) whose tree bytes live in an external `&[u8]` / `&mut Vec<u8>`
//! blob passed to each method. Values are `u32` indexes below the universe
//! size; a zero-byte blob is valid — the empty set for [`RawBitmap`], the
//! full set for an inverted [`Bitmap`].
//!
//! Modules:
//! - `raw` — [`RawBitmap`] and [`Iter`]: build from a sorted iterator or a
//!   range, query (contains, len, min/max, range cardinality), mutate
//!   (insert / remove / remove-range), boolean set ops, and the
//!   `[universe: u32 LE][len: u32 LE][bytes]` wire format.
//! - `node` — node-byte helpers (child index/offset, subtree skipping, the
//!   sequential read cursor); crate-internal.
//! - `ops` — the recursive subtree walkers those methods call; internal.
//! - `bitmap` — [`Bitmap`]: a `RawBitmap` wrapper that may store the
//!   *complement* so dense sets stay small too, with De Morgan dispatch
//!   for and/or; adds [`BitmapIter`] and [`ComplementIter`].
//! - `roaring` (feature `roaring`) — `RawBitmap::from_roaring` /
//!   `to_roaring` bridges.
//!
//! Public surface = the re-exports below plus the two free functions:
//! [`ceil_log8`] maps a universe size to tree depth;
//! [`estimate_data_size`] predicts the canonical blob length. Tests live
//! in `tests_bitmap` / `tests_raw` and the roaring differential tests in
//! `tests_roaring`; the fuzz workspace (fuzz/fuzz_targets/
//! against_roaring.rs) replays random op sequences against roaring.
//!
//! Features: `roaring` (the bridges above), `serde` (derives on both
//! descriptors — sfst embeds them in on-disk structs), `allocative`
//! (declared, unused in src).
//!
//! Consumers (grep-verified): `sfst` is the only real dependency — on-disk
//! log-index bitmaps as `treight::Bitmap`, set ops native, no roaring
//! round-trip (`PosSet` in `sfst/src/index_reader.rs`). `sfsq` and
//! `otel-ledger` use treight in tests only. The fuzz workspace pins the
//! same netdata/roaring-rs fork as the main workspace
//! (the `roaring` git pin in `src/crates/Cargo.toml`) so both sides test one
//! roaring implementation.
mod bitmap;
mod node;
mod ops;
mod raw;
#[cfg(feature = "roaring")]
mod roaring;

#[cfg(test)]
mod tests_bitmap;
#[cfg(test)]
mod tests_raw;
#[cfg(all(test, feature = "roaring"))]
mod tests_roaring;

pub use bitmap::{Bitmap, BitmapIter, ComplementIter};
pub use raw::{Iter, RawBitmap};

/// Tree depth for a universe of `universe_size` values: each level maps 3
/// bits of the value onto one of a node's 8 children, so any non-empty
/// universe needs at least one level and a u32 universe never needs more
/// than 11. Returns 0 only for `universe_size == 0` (no tree, empty blob).
///
/// Every construction path derives its descriptor's level count here:
/// `RawBitmap::empty`, `RawBitmap::from_sorted_iter`,
/// `RawBitmap::deserialize_from` (raw.rs), and [`estimate_data_size`].
pub fn ceil_log8(universe_size: u32) -> u32 {
    if universe_size == 0 {
        return 0;
    }

    let mut levels: u32 = 1;
    let mut n = universe_size - 1;
    while n >> 3 != 0 {
        n >>= 3;
        levels += 1;
    }

    levels
}

/// The number of tree bytes [`RawBitmap::from_sorted_iter`] would append
/// for `sorted_values` under `universe_size`, computed without building
/// the tree. Storage is canonical (no dead bytes), so this is the exact
/// blob size for these values, not an upper bound.
///
/// The count is one byte per distinct tree node: each node byte in the
/// pre-order serialization corresponds to one (level, node index) pair on
/// the values' paths, with the root (index 0 for every value) contributing
/// the final byte. An empty value list yields 0, matching the empty blob
/// `from_sorted_iter` leaves behind.
///
/// The values **must** be yielded in ascending order, duplicates tolerated
/// (as roaring iterators do). Use it to weigh converting a roaring bitmap
/// to treight:
///
/// ```ignore
/// let treight_bytes = treight::estimate_data_size(universe_size, roaring_bm.iter());
/// let roaring_bytes = roaring_bm.serialized_size();
/// if treight_bytes < roaring_bytes { /* convert */ }
/// ```
///
/// Pinned exact by the `estimate_data_size_is_exact` proptest
/// (`tests_roaring.rs`) and by the fuzz target's closing size check
/// (`fuzz/fuzz_targets/against_roaring.rs`), which
/// asserts a blob stays exactly this size after arbitrary mutation
/// sequences.
pub fn estimate_data_size(universe_size: u32, sorted_values: impl Iterator<Item = u32>) -> usize {
    let levels = ceil_log8(universe_size);
    if levels == 0 {
        return 0;
    }

    // Last-seen node index per non-root level; the root's index is always 0.

    // 11 levels is the u32 maximum (32 bits / 3 bits per level, rounded up);
    // raw.rs sizes its per-level arrays with the same bound.
    const MAX_INNER_LEVELS: usize = 11;
    let mut prev_node = [u32::MAX; MAX_INNER_LEVELS];
    let inner_levels = levels as usize - 1;

    let mut total: usize = 0;
    let mut any = false;

    for v in sorted_values {
        any = true;

        for (k, prev) in prev_node[..inner_levels].iter_mut().enumerate() {
            let node = v >> (3 * (k + 1));

            if node != *prev {
                total += 1;
                *prev = node;
            }
        }
    }

    if any {
        total += 1; // root byte
    }

    total
}
