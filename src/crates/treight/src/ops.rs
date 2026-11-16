//! Recursive subtree walkers behind `RawBitmap`'s query, range
//! construction, mutation and set-op methods. `raw` is the only importer of this module; bitmap.rs
//! and sfst reach these through `RawBitmap`/`Bitmap` methods. Everything
//! here is `pub(crate)`.
//!
//! Shared conventions:
//! - `level` is the height of the subtree at the reader's cursor and is
//!   decremented per step; a node at height `h` spans `8^h` values and
//!   child `c` of it starts at `base + c * 8^(h-1)` (`child_index`/
//!   `child_offset`, node.rs).
//! - `NodeReader` walks the serialized tree in pre-order (node byte, then
//!   present children in index order); later children are reached by
//!   skipping the preceding ones. A call consumes exactly one subtree
//!   unless its notes say otherwise: `range_count` stops past its range,
//!   and the set-op walkers may leave cursors mid-tree (`need` and bulk
//!   tails, below).
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
            nodes.skip(level);
        }
    }
    contains_inner(nodes, level, value)
}

/// Count the set bits in the subtree at `level` whose values fall in
/// `[start, end)`. `base` is the value the subtree's root starts at (0 at
/// the tree root). Children fully inside the range are counted in one
/// shot by `skip_subtree` — its return value is the subtree's population
/// — children before the range are skipped, the walk stops at the first
/// child past it (the reader's position is then undefined), and partially
/// overlapping ones recurse; the leaf counts only its bits inside
/// `[start, end)`, clamped to the leaf's 8 values.
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

        if child_base >= end {
            // This and every later child start past the range: stop without
            // skipping them. The reader is left mid-tree, which is safe: only
            // the subtree holding `end` stops early, every later sibling of
            // each ancestor starts past `end` too, so they stop as well, and
            // `range_cardinality` drops the reader.
            break;
        }

        if child_end <= start {
            // Fully before the range: skip without counting.
            reader.skip(child_level);
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
            reader.skip(child_level);
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

/// Bytes in a full subtree `level` levels tall, one per node:
/// 1 + 8 + ... + 8^(level-1) = (8^level - 1) / 7. Every one is `0xFF`.
fn full_subtree_len(level: u32) -> usize {
    (((1u64 << (3 * level)) - 1) / 7) as usize
}

/// Append the subtree at `level` that starts at value `base` and holds
/// exactly the values of `ranges`: half-open `(start, end)` pairs,
/// sorted, disjoint, non-empty, each intersecting this subtree. A child
/// inside a single range is a full subtree, whose pre-order bytes are all
/// `0xFF`, so it is written as one run without visiting its nodes; only
/// children on a range edge recurse. Cost tracks the bytes written, not
/// the values. `u64` arithmetic, as in `range_count`.
pub(crate) fn ranges_subtree(level: u32, base: u64, ranges: &[(u32, u32)], out: &mut Vec<u8>) {
    if level == 1 {
        let mut leaf = 0u8;
        for &(start, end) in ranges {
            let lo = (start as u64).saturating_sub(base).min(8) as u8;
            let hi = (end as u64 - base).min(8) as u8;
            leaf |= ((1u16 << hi) - (1u16 << lo)) as u8;
        }
        out.push(leaf);
        return;
    }

    let child_level = level - 1;
    let stride: u64 = 1u64 << (3 * child_level);
    let node_end = base + 8 * stride;

    let mut node = 0u8;
    for &(start, end) in ranges {
        let lo_child = ((start as u64).max(base) - base) / stride;
        let hi_child = ((end as u64).min(node_end) - 1 - base) / stride;
        node |= ((1u16 << (hi_child + 1)) - (1u16 << lo_child)) as u8;
    }
    out.push(node);

    // Ranges are sorted, so each child's ranges are the next contiguous
    // run of the slice and `i` only moves forward.
    let mut i = 0;
    for child in 0..8u8 {
        if node & (1 << child) == 0 {
            continue;
        }

        let child_base = base + (child as u64) * stride;
        let child_end = child_base + stride;

        while ranges[i].1 as u64 <= child_base {
            i += 1;
        }
        let mut j = i;
        while j < ranges.len() && (ranges[j].0 as u64) < child_end {
            j += 1;
        }

        match ranges[i..j] {
            [(start, end)] if start as u64 <= child_base && end as u64 >= child_end => {
                out.resize(out.len() + full_subtree_len(child_level), 0xFF);
            }
            _ => ranges_subtree(child_level, child_base, &ranges[i..j], out),
        }
    }
}

/// Copy the subtree at `level` from the reader to `out` verbatim:
/// `skip` advances the reader past it and the traversed byte range is
/// appended as-is.
pub(crate) fn copy_subtree(reader: &mut NodeReader, level: u32, out: &mut Vec<u8>) {
    let start = reader.pos;
    reader.skip(level);
    out.extend_from_slice(&reader.nodes[start..reader.pos]);
}

/// Child bits strictly above child bit `child` (a single-bit mask).
#[inline(always)]
fn above(child: u8) -> u8 {
    !(child | (child - 1))
}

/// Outcome of a pruning set-op walker: the subtree computed to nothing,
/// survived, or survived and finished the whole walk with a bulk tail copy
/// (see the walker notes below), after which the caller only fixes its own
/// node byte and returns `Done` too.
#[derive(Clone, Copy, PartialEq, Eq)]
pub(crate) enum Walk {
    Empty,
    Kept,
    Done,
}

/// Skip every child subtree in `children` (`level` levels tall).
#[inline(always)]
fn skip_children(reader: &mut NodeReader, mut children: u8, level: u32) {
    while children != 0 {
        reader.skip(level);
        children &= children - 1;
    }
}

// The four set-op walkers below share one shape: both readers advance in
// lockstep over the children present in either operand (the set bits of
// `node_a | node_b`, lowest first; AND/SUB may stop after the last shared
// one, see `need`), and the operations differ in the leaf byte op plus
// what a child present in only one operand does:
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
// nothing and back-patch the reserved node byte with the surviving child
// bits; AND reports survival with a `bool`, SUB/XOR with `Walk`, which
// adds `Done` for bulk tails.
//
// AND and SUB take a `need` flag: whether the caller reads on after this
// subtree (a later sibling or ancestor). Only then must the cursors end at
// the subtree's end; otherwise children after the last shared one are left
// unread, and the cursor positions are undefined afterwards. raw.rs passes
// `false` at the root, and a call passes `true` to every shared child but
// its last. SUB always consumes `a` (its children are copied), so its flag
// covers `b` only.
//
// Bulk tails: once one operand has nothing left to read, the rest of the
// other operand's bytes is the rest of the result, ancestors' later
// children included, so it is copied with one extend and the walk stops.
// OR and XOR read both cursors exactly, so "nothing left" is a cursor at
// its blob's end. SUB uses its flag: with `need_b` unset nothing of `b` is
// read again, so trailing A-only children start the tail. AND has no tail
// (one-sided children are dropped). Walkers report it as `Walk::Done`
// (`true` for OR); each caller ORs the copied children into its node byte
// and returns at once. Cursor positions are undefined after a tail.

/// Union of two subtrees (see the table above): leaf and node bytes are
/// OR'd, children unique to one side are copied verbatim, and nothing is
/// pruned — the result is a canonical non-empty subtree whenever the
/// inputs are. Returns `true` once a bulk tail finished the whole walk;
/// the node byte already holds every child.
pub(crate) fn union_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
) -> bool {
    let node_a = a.next().bits();
    let node_b = b.next().bits();
    out.push(node_a | node_b);
    if level == 1 {
        return false;
    }

    let child_level = level - 1;
    let mut children = node_a | node_b;
    while children != 0 {
        let child = children & children.wrapping_neg();
        children &= children - 1;
        if node_a & node_b & child != 0 {
            if union_subtree(a, b, child_level, out) {
                return true;
            }
        } else if node_a & child != 0 {
            copy_subtree(a, child_level, out);
        } else {
            copy_subtree(b, child_level, out);
        }
        if children != 0 && copy_tail(a, b, out) {
            return true;
        }
    }
    false
}

/// The OR/XOR bulk tail: if either cursor is at its blob's end, append
/// the other's remaining bytes and return `true`.
#[inline]
fn copy_tail(a: &mut NodeReader, b: &mut NodeReader, out: &mut Vec<u8>) -> bool {
    let rest = if b.pos == b.nodes.len() {
        &a.nodes[a.pos..]
    } else if a.pos == a.nodes.len() {
        &b.nodes[b.pos..]
    } else {
        return false;
    };
    out.extend_from_slice(rest);
    true
}

/// Intersection of two subtrees (see the table above): leaves AND
/// byte-wise, children unique to one side are skipped past, and subtrees
/// that intersect to nothing are pruned. Only children up to the last one
/// both sides share can contribute; the rest are skipped only if `need`
/// (see the `need` rule above). Returns `true` if the subtree survived.
pub(crate) fn intersect_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
    need: bool,
) -> bool {
    let node_a = a.next().bits();
    let node_b = b.next().bits();

    if level == 1 {
        // Leaf: AND the two bytes directly.
        let result = node_a & node_b;
        if result != 0 {
            out.push(result);
            return true;
        }
        return false;
    }

    let child_level = level - 1;
    let shared = node_a & node_b;
    if shared == 0 {
        if need {
            skip_children(a, node_a, child_level);
            skip_children(b, node_b, child_level);
        }
        return false;
    }

    // Inner node: reserve a slot for the result node byte.
    let node_pos = out.len();
    out.push(0);

    let last = 0x80u8 >> shared.leading_zeros();
    let tail = above(last);
    let mut result_bits: u8 = 0;
    let mut children = (node_a | node_b) & !tail;
    while children != 0 {
        let child = children & children.wrapping_neg();
        children &= children - 1;
        if shared & child != 0 {
            if intersect_subtree(a, b, child_level, out, need || child != last) {
                result_bits |= child;
            }
        } else if node_a & child != 0 {
            a.skip(child_level);
        } else {
            b.skip(child_level);
        }
    }
    if need {
        skip_children(a, node_a & tail, child_level);
        skip_children(b, node_b & tail, child_level);
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
/// pruned. `a` is consumed unless a bulk tail ends the walk; B-only
/// children after the last shared one are skipped only if `need_b` (see
/// the `need` and bulk-tail rules above).
pub(crate) fn difference_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
    need_b: bool,
) -> Walk {
    let node_a = a.next().bits();
    let node_b = b.next().bits();

    if level == 1 {
        let result = node_a & !node_b;
        if result != 0 {
            out.push(result);
            return Walk::Kept;
        }
        return Walk::Empty;
    }

    let child_level = level - 1;
    let node_pos = out.len();
    out.push(0);

    // Children up to the last shared one walk in lockstep; after it only
    // A-only children (copied) and B-only ones (skipped if needed) remain.
    let shared = node_a & node_b;
    let (last, tail) = if shared == 0 {
        (0, 0xFF)
    } else {
        let last = 0x80u8 >> shared.leading_zeros();
        (last, above(last))
    };
    let mut result_bits: u8 = 0;
    let mut children = (node_a | node_b) & !tail;
    while children != 0 {
        let child = children & children.wrapping_neg();
        children &= children - 1;
        if shared & child != 0 {
            match difference_subtree(a, b, child_level, out, need_b || child != last) {
                Walk::Empty => {}
                Walk::Kept => result_bits |= child,
                Walk::Done => {
                    // Only the last shared child can finish the walk, and
                    // the tail it copied holds this node's A-only tail too.
                    out[node_pos] = result_bits | child | (node_a & tail);
                    return Walk::Done;
                }
            }
        } else if node_a & child != 0 {
            copy_subtree(a, child_level, out);
            result_bits |= child;
        } else {
            b.skip(child_level);
        }
    }
    let mut a_tail = node_a & tail;
    if a_tail != 0 && !need_b {
        // Nothing of `b` is read again: the rest of `a` is the result's tail.
        out.extend_from_slice(&a.nodes[a.pos..]);
        a.pos = a.nodes.len();
        out[node_pos] = result_bits | a_tail;
        return Walk::Done;
    }
    result_bits |= a_tail;
    while a_tail != 0 {
        copy_subtree(a, child_level, out);
        a_tail &= a_tail - 1;
    }
    if need_b {
        skip_children(b, node_b & tail, child_level);
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        Walk::Kept
    } else {
        out.pop();
        Walk::Empty
    }
}

/// Symmetric difference `a ^ b` of two subtrees (see the table above):
/// leaves XOR byte-wise, children unique to either side are copied
/// verbatim (the other operand has nothing there), and equal leaves XOR
/// to zero, so empty subtrees are pruned; a bulk tail can end the walk
/// (see the rules above).
pub(crate) fn symmetric_difference_subtree(
    a: &mut NodeReader,
    b: &mut NodeReader,
    level: u32,
    out: &mut Vec<u8>,
) -> Walk {
    let node_a = a.next().bits();
    let node_b = b.next().bits();

    if level == 1 {
        let result = node_a ^ node_b;
        if result != 0 {
            out.push(result);
            return Walk::Kept;
        }
        return Walk::Empty;
    }

    let child_level = level - 1;
    let node_pos = out.len();
    out.push(0);

    let mut result_bits: u8 = 0;
    let mut children = node_a | node_b;
    while children != 0 {
        let child = children & children.wrapping_neg();
        children &= children - 1;
        if node_a & node_b & child != 0 {
            match symmetric_difference_subtree(a, b, child_level, out) {
                Walk::Empty => {}
                Walk::Kept => result_bits |= child,
                Walk::Done => {
                    // The copied tail holds every remaining child here.
                    out[node_pos] = result_bits | child | children;
                    return Walk::Done;
                }
            }
        } else if node_a & child != 0 {
            copy_subtree(a, child_level, out);
            result_bits |= child;
        } else {
            copy_subtree(b, child_level, out);
            result_bits |= child;
        }
        // Every remaining child is one-sided once a cursor is exhausted.
        if children != 0 && copy_tail(a, b, out) {
            out[node_pos] = result_bits | children;
            return Walk::Done;
        }
    }

    if result_bits != 0 {
        out[node_pos] = result_bits;
        Walk::Kept
    } else {
        out.pop();
        Walk::Empty
    }
}
