//! Recursive subtree walkers behind `RawBitmap`'s query, mutation and
//! set-op methods. `raw` is the only importer of this module; bitmap.rs
//! and sfst reach these through `RawBitmap`/`Bitmap` methods. Everything
//! here is `pub(crate)`.
//!
//! Shared conventions:
//! - `level` is the height of the subtree at the reader's cursor and is
//!   decremented per step; a node at height `h` spans `8^h` values and
//!   child `c` of it starts at `base + c * 8^(h-1)` (`child_index`/
//!   `child_offset`, node.rs).
//! - `NodeReader` walks the serialized tree in pre-order (node byte, then
//!   present children in index order), so each call consumes exactly one
//!   subtree and later children are reached by skipping the preceding
//!   ones.
//! - Bytes appended to `out` are the result subtree's serialization;
//!   `RawBitmap`'s set-op methods in raw.rs snapshot `out.len()` around
//!   the call and treat the growth as the result's tree bytes — the
//!   convention sfst's set ops build on (`PosSet::or_assign`/
//!   `and_assign` in `sfst/src/index_reader.rs`).
use crate::node::{NodeReader, child_index};

/// Walk the tree to `value`, one node byte per level. `level` is the
/// remaining height, decremented first; the child comes from `value`'s
/// 3-bit field at that level, and a missing child is `false`. At the
/// leaf level (entry `level == 1`) that bit is the answer; higher up,
/// skip the preceding siblings' subtrees so the cursor lands on the
/// child's subtree and recurse. Matches `is_hit_1` in GNU idutils' fid.c.
pub(crate) fn contains_inner(nodes: &mut NodeReader, level: u32, value: u32) -> bool {
    let level = level - 1;
    let child = child_index(level, value);
    let node = nodes.next();

    if !node.has_child(child) {
        return false;
    }
    if level == 0 {
        return true;
    }

    // Skip preceding sibling subtrees.
    for sibling in 0..child {
        if node.has_child(sibling) {
            nodes.skip_subtree(level);
        }
    }
    contains_inner(nodes, level, value)
}

/// Count the set bits in the subtree at `level` whose values fall in
/// `[start, end)`. `base` is the value the subtree's root starts at (0 at
/// the tree root). Children fully inside the range are counted in one
/// shot by `skip_subtree` — its return value is the subtree's population
/// — fully outside ones are skipped, and partially overlapping ones
/// recurse; the leaf counts only its bits inside `[start, end)`, clamped
/// to the leaf's 8 values.
///
/// Arithmetic is `u64` because the span of an 11-level (u32-max) tree is
/// `8^11 = 2^33`, so `base` and the range offsets can exceed `u32::MAX`
/// even though stored values cannot.
pub(crate) fn range_count(
    reader: &mut NodeReader,
    level: u32,
    base: u64,
    start: u64,
    end: u64,
) -> u64 {
    let node = reader.next();
    let child_level = level - 1;

    if child_level == 0 {
        // Leaf: count only the bits within [start, end).
        let lo = start.saturating_sub(base).min(8) as u8;
        let hi = end.saturating_sub(base).min(8) as u8;
        let mask = ((1u16 << hi) - (1u16 << lo)) as u8;
        return (node.bits() & mask).count_ones() as u64;
    }

    let stride: u64 = 1u64 << (3 * child_level);
    let mut count = 0u64;

    for child in 0..8u8 {
        if !node.has_child(child) {
            continue;
        }

        let child_base = base + (child as u64) * stride;
        let child_end = child_base + stride;

        if child_end <= start || child_base >= end {
            // Fully outside: skip without counting.
            reader.skip_subtree(child_level);
        } else if child_base >= start && child_end <= end {
            // Fully inside: count everything in the subtree.
            count += reader.skip_subtree(child_level);
        } else {
            // Partial overlap: recurse.
            count += range_count(reader, child_level, child_base, start, end);
        }
    }

    count
}

/// Remove the values in `[start, end)` from the subtree at `level`,
/// appending the surviving subtree to `out`. Returns `true` if anything
/// survived. Children fully outside the range are copied verbatim, fully
/// inside ones are dropped, and partially overlapping ones recurse; like
/// the pruning set-op walkers, a node whose children all vanish pops its
/// reserved byte (nothing is appended) and a leaf survives only if bits
/// remain after masking out `[start, end)`.
pub(crate) fn remove_range_subtree(
    reader: &mut NodeReader,
    level: u32,
    base: u64,
    start: u64,
    end: u64,
    out: &mut Vec<u8>,
) -> bool {
    let node = reader.next();
    let child_level = level - 1;

    if child_level == 0 {
        // Leaf: mask out bits within [start, end).
        let lo = start.saturating_sub(base).min(8) as u8;
        let hi = end.saturating_sub(base).min(8) as u8;
        let mask = ((1u16 << hi) - (1u16 << lo)) as u8;
        let result = node.bits() & !mask;
        if result != 0 {
            out.push(result);
            return true;
        }
        return false;
    }

    let stride: u64 = 1u64 << (3 * child_level);
    let node_pos = out.len();
    out.push(0);

    let mut result_bits: u8 = 0;

    for child in 0..8u8 {
        if !node.has_child(child) {
            continue;
        }

        let child_base = base + (child as u64) * stride;
        let child_end = child_base + stride;

        if child_end <= start || child_base >= end {
            // Fully outside removal range: keep.
            copy_subtree(reader, child_level, out);
            result_bits |= 1 << child;
        } else if child_base >= start && child_end <= end {
            // Fully inside removal range: drop.
            reader.skip_subtree(child_level);
        } else {
            // Partial overlap: recurse.
            if remove_range_subtree(reader, child_level, child_base, start, end, out) {
                result_bits |= 1 << child;
            }
        }
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        true
    } else {
        out.pop();
        false
    }
}

/// Copy the subtree at `level` from the reader to `out` verbatim:
/// `skip_subtree` advances the reader past it and the traversed byte
/// range is appended as-is.
pub(crate) fn copy_subtree(reader: &mut NodeReader, level: u32, out: &mut Vec<u8>) {
    let start = reader.pos;
    reader.skip_subtree(level);
    out.extend_from_slice(&reader.nodes[start..reader.pos]);
}

// The four set-op walkers below share one shape: both readers advance in
// lockstep, one subtree per child position, and the operations differ in
// the leaf byte op plus what a child present in only one operand does:
//
//   Operation | Leaf op | A-only child | B-only child
//   ----------+---------+--------------+-------------
//   OR        | a | b   | copy         | copy
//   AND       | a & b   | skip         | skip
//   SUB       | a & !b  | copy         | skip
//   XOR       | a ^ b   | copy         | copy
//
// Both operands must be non-empty and equally tall — raw.rs guarantees
// that (empty-blob short-circuits plus an unconditional `assert_eq!` on
// universe sizes) before dispatch. OR never prunes: union always emits
// its `a|b` node byte. AND/SUB/XOR prune subtrees that compute to
// nothing, reporting survival with a `bool` and back-patching the
// reserved node byte with the surviving child bits.

/// Union of two subtrees (see the table above): leaf and node bytes are
/// OR'd, children unique to one side are copied verbatim, and nothing is
/// pruned — the result is a canonical non-empty subtree whenever the
/// inputs are.
pub(crate) fn union_subtree(a: &mut NodeReader, b: &mut NodeReader, level: u32, out: &mut Vec<u8>) {
    let node_a = a.next();
    let node_b = b.next();
    let child_level = level - 1;

    if child_level == 0 {
        out.push(node_a.bits() | node_b.bits());
        return;
    }

    out.push(node_a.bits() | node_b.bits());

    for child in 0..8u8 {
        let in_a = node_a.has_child(child);
        let in_b = node_b.has_child(child);

        match (in_a, in_b) {
            (true, true) => {
                union_subtree(a, b, child_level, out);
            }
            (true, false) => {
                copy_subtree(a, child_level, out);
            }
            (false, true) => {
                copy_subtree(b, child_level, out);
            }
            (false, false) => {}
        }
    }
}

/// Intersection of two subtrees (see the table above): leaves AND
/// byte-wise, children unique to one side are skipped past, and subtrees
/// that intersect to nothing are pruned. Returns `true` if the subtree
/// survived.
pub(crate) fn intersect_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
) -> bool {
    let node_a = a.next();
    let node_b = b.next();
    let child_level = level - 1;

    if child_level == 0 {
        // Leaf: AND the two bytes directly.
        let result = node_a.bits() & node_b.bits();
        if result != 0 {
            out.push(result);
            return true;
        }
        return false;
    }

    // Inner node: reserve a slot for the result node byte.
    let node_pos = out.len();
    out.push(0);

    let mut result_bits: u8 = 0;

    for child in 0..8u8 {
        let in_a = node_a.has_child(child);
        let in_b = node_b.has_child(child);

        match (in_a, in_b) {
            (true, true) => {
                if intersect_subtree(a, b, child_level, out) {
                    result_bits |= 1 << child;
                }
            }
            (true, false) => {
                a.skip_subtree(child_level);
            }
            (false, true) => {
                b.skip_subtree(child_level);
            }
            (false, false) => {}
        }
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        true
    } else {
        out.pop();
        false
    }
}

/// Set difference `a - b` of two subtrees (see the table above): leaves
/// keep `a & !b`, A-only children are copied verbatim (B has nothing to
/// remove there), B-only ones are skipped past, and empty results are
/// pruned. Returns `true` if the subtree survived.
pub(crate) fn difference_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
) -> bool {
    let node_a = a.next();
    let node_b = b.next();
    let child_level = level - 1;

    if child_level == 0 {
        let result = node_a.bits() & !node_b.bits();
        if result != 0 {
            out.push(result);
            return true;
        }
        return false;
    }

    let node_pos = out.len();
    out.push(0);

    let mut result_bits: u8 = 0;

    for child in 0..8u8 {
        let in_a = node_a.has_child(child);
        let in_b = node_b.has_child(child);

        match (in_a, in_b) {
            (true, true) => {
                if difference_subtree(a, b, child_level, out) {
                    result_bits |= 1 << child;
                }
            }
            (true, false) => {
                copy_subtree(a, child_level, out);
                result_bits |= 1 << child;
            }
            (false, true) => {
                b.skip_subtree(child_level);
            }
            (false, false) => {}
        }
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        true
    } else {
        out.pop();
        false
    }
}

/// Symmetric difference `a ^ b` of two subtrees (see the table above):
/// leaves XOR byte-wise, children unique to either side are copied
/// verbatim (the other operand has nothing there), and equal leaves XOR
/// to zero, so empty subtrees are pruned. Returns `true` if the subtree
/// survived.
pub(crate) fn symmetric_difference_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
) -> bool {
    let node_a = a.next();
    let node_b = b.next();
    let child_level = level - 1;

    if child_level == 0 {
        let result = node_a.bits() ^ node_b.bits();
        if result != 0 {
            out.push(result);
            return true;
        }
        return false;
    }

    let node_pos = out.len();
    out.push(0);

    let mut result_bits: u8 = 0;

    for child in 0..8u8 {
        let in_a = node_a.has_child(child);
        let in_b = node_b.has_child(child);

        match (in_a, in_b) {
            (true, true) => {
                if symmetric_difference_subtree(a, b, child_level, out) {
                    result_bits |= 1 << child;
                }
            }
            (true, false) => {
                copy_subtree(a, child_level, out);
                result_bits |= 1 << child;
            }
            (false, true) => {
                copy_subtree(b, child_level, out);
                result_bits |= 1 << child;
            }
            (false, false) => {}
        }
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        true
    } else {
        out.pop();
        false
    }
}
