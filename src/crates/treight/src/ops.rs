use crate::node::{child_index, NodeReader};

/// Recursive contains check. Matches `is_hit_1` from `src/fid.c:260-278`.
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

/// Count set bits in the subtree at `level` that fall within `[start, end)`.
///
/// `base` is the value offset of this subtree's root (0 for the tree root).
/// Uses u64 arithmetic to avoid overflow at the highest tree levels.
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

/// Remove values in `[start, end)` from the subtree at `level`, writing the
/// surviving nodes to `out`. Returns `true` if the subtree is non-empty after
/// removal.
///
/// Same structure as the set-operation walkers: children fully outside the
/// removal range are copied, children fully inside are skipped (dropped), and
/// children with partial overlap are recursed into.
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

/// Copy a subtree rooted at `level` from the reader to `out` verbatim.
///
/// Advances the reader past the subtree by calling `skip_subtree`, then copies
/// the raw bytes that were traversed.
pub(crate) fn copy_subtree(reader: &mut NodeReader, level: u32, out: &mut Vec<u8>) {
    let start = reader.pos;
    reader.skip_subtree(level);
    out.extend_from_slice(&reader.nodes[start..reader.pos]);
}

// The four set-operation walkers below share identical structure and differ
// in exactly three parameters:
//
//   Operation  | Leaf op   | A-only child | B-only child
//   -----------+-----------+--------------+-------------
//   OR  (union)| a | b     | copy         | copy
//   AND (inter)| a & b     | skip         | skip
//   SUB (diff) | a & !b    | copy         | skip
//   XOR (symd) | a ^ b     | copy         | copy
//
// They could be unified into a single generic walker parameterized by these
// three values, but are kept separate for readability.

/// Recursively compute the union of two subtrees at the given `level`.
///
/// Walks both trees in lockstep. Children present in both are recursed into
/// (OR-ing leaf bytes). Children unique to one side are bulk-copied. The result
/// is always non-empty when at least one input subtree is non-empty.
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

/// Recursively intersect two subtrees at the given `level`.
///
/// Walks both trees in lockstep, only descending into children present in both.
/// Subtrees unique to one side are skipped without expansion. Returns `true`
/// if the intersection produced any output.
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

/// Recursively compute the set difference (a - b) of two subtrees at `level`.
///
/// Children only in A are copied verbatim. Children only in B are skipped.
/// Children in both are recursed into with leaf op `a & !b`. Returns `true`
/// if the result is non-empty (needs pruning like intersect).
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

/// Recursively compute the symmetric difference (a ^ b) of two subtrees at `level`.
///
/// Children unique to one side are copied verbatim. Children in both are
/// recursed into with leaf op `a ^ b`. Returns `true` if the result is
/// non-empty (needs pruning since identical leaves XOR to zero).
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
