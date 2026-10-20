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

/// Compute the number of levels in the treight for a given universe size.
///
/// Returns 0 for universe_size == 0.
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

/// Compute the exact treight data size for a sorted sequence of values,
/// without actually building the tree.
///
/// This is useful for deciding whether converting a roaring bitmap to treight
/// would save space:
///
/// ```ignore
/// let treight_bytes = treight::estimate_data_size(universe_size, roaring_bm.iter());
/// let roaring_bytes = roaring_bm.serialized_size();
/// if treight_bytes < roaring_bytes { /* convert */ }
/// ```
///
/// The values **must** be yielded in ascending order (as roaring iterators do).
/// The result equals the length of the data that `RawBitmap::from_sorted_iter`
/// would append for these values.
pub fn estimate_data_size(universe_size: u32, sorted_values: impl Iterator<Item = u32>) -> usize {
    let levels = ceil_log8(universe_size);
    if levels == 0 {
        return 0;
    }

    // Track the last-seen node index at each level (excluding root).

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
