use std::mem::size_of;

// Capacity-based diagnostics use a stable approximation instead of depending
// on private standard-library B-tree node layouts.
const BTREE_ENTRY_OVERHEAD_BYTES: usize = size_of::<usize>() * 4;

pub(crate) fn btree_container_overhead_bytes(len: usize) -> usize {
    len.saturating_mul(BTREE_ENTRY_OVERHEAD_BYTES)
}

pub(crate) fn hash_table_allocation_bytes(bucket_count: usize, element_size: usize) -> usize {
    bucket_count.saturating_mul(element_size.saturating_add(1))
}

pub(crate) fn hash_map_allocation_bytes(capacity: usize, element_size: usize) -> usize {
    let estimated_buckets = capacity.saturating_mul(8).div_ceil(7);
    hash_table_allocation_bytes(estimated_buckets, element_size)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hash_map_estimate_includes_load_factor_headroom() {
        assert_eq!(hash_map_allocation_bytes(0, 4), 0);
        assert_eq!(hash_map_allocation_bytes(7, 4), 8 * 5);
        assert_eq!(hash_map_allocation_bytes(14, 4), 16 * 5);
    }
}
