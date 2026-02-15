use roaring::RoaringBitmap;
use treight::Bitmap;

use crate::bitset::Bitset;

/// Remap roaring bitmaps from insertion order to time-sorted order and
/// convert directly to treight bitmaps, skipping intermediate roaring
/// construction.
///
/// For each roaring bitmap:
/// 1. Iterate its set bits (insertion-order positions).
/// 2. Apply the remap to get time-sorted positions.
/// 3. Sort the remapped positions (the remap scatters values).
/// 4. Feed the sorted iterator into `Bitmap::from_sorted_iter`.
///
/// Dense bitmaps (cardinality > universe/2) store their complement instead,
/// following treight's inversion optimization.
///
/// Uses a hybrid strategy: bitset scan for dense bitmaps (O(universe/64)),
/// comparison sort for sparse bitmaps (O(n log n) but n is small).
pub fn roaring_to_treight(
    bitmaps: &[RoaringBitmap],
    remap: &[u32],
    universe_size: u32,
) -> (Vec<Bitmap>, Vec<u8>) {
    let half = universe_size as u64 / 2;
    let mut treight_data: Vec<u8> = Vec::new();
    let mut treight_bitmaps: Vec<Bitmap> = Vec::with_capacity(bitmaps.len());

    // Reusable bitset for sorting remapped values without comparison sort.
    // Used only for dense bitmaps where sort_unstable is expensive.
    // universe_size bits ≈ 62.5 KB for 500K logs — fits in L1 cache.
    let mut bitset = Bitset::new(universe_size);
    let mut remapped: Vec<u32> = Vec::new();

    // Threshold: use bitset when cardinality is large enough that
    // O(n log n) sort is slower than O(universe/64) bitset scan.
    let bitset_threshold = (universe_size as usize / 64).max(256);

    for rb in bitmaps {
        let card = rb.len() as u64;

        if rb.len() as usize >= bitset_threshold {
            // Dense bitmap: use bitset for O(n) sorting.
            for v in rb.iter() {
                bitset.set(remap[v as usize]);
            }

            let bitmap = if card > half {
                let complement = bitset.iter_zeros(universe_size);
                Bitmap::from_sorted_iter_complemented(complement, universe_size, &mut treight_data)
            } else {
                Bitmap::from_sorted_iter(bitset.iter_ones(), universe_size, &mut treight_data)
            };
            treight_bitmaps.push(bitmap);

            // Clear only the bits we set.
            for v in rb.iter() {
                bitset.clear(remap[v as usize]);
            }
        } else {
            // Sparse bitmap: remap + comparison sort is faster.
            remapped.clear();
            remapped.extend(rb.iter().map(|v| remap[v as usize]));
            remapped.sort_unstable();

            let bitmap = Bitmap::from_sorted_iter(
                remapped.iter().copied(),
                universe_size,
                &mut treight_data,
            );
            treight_bitmaps.push(bitmap);
        }
    }

    (treight_bitmaps, treight_data)
}
