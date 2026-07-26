use std::mem::size_of;

// Capacity-based diagnostics use a stable approximation instead of depending
// on private standard-library B-tree node layouts.
const BTREE_ENTRY_OVERHEAD_BYTES: usize = size_of::<usize>() * 4;

pub(crate) fn btree_container_overhead_bytes(len: usize) -> usize {
    len.saturating_mul(BTREE_ENTRY_OVERHEAD_BYTES)
}

pub(crate) fn hash_table_allocation_bytes(capacity: usize, element_size: usize) -> usize {
    capacity.saturating_mul(element_size.saturating_add(1))
}
