//! The node-byte vocabulary all tree walkers share. A serialized treight's
//! tree bytes are a depth-first pre-order walk of an 8-way bit-tree: one
//! byte per node, bit `i` set means child `i` (0..8) is present, and each
//! present child's own subtree bytes follow in ascending child order. A
//! non-empty tree is always exactly `ceil_log8(universe_size)` levels tall
//! (lib.rs), so the bottom level's bytes are leaves — there the bits are
//! value bits directly — and a child at level `level` covers value bits
//! `[3 * level, 3 * level + 3)`. Throughout, `level` is a subtree's height
//! in node bytes (1 = single leaf byte). Walkers assume the canonical form
//! (every byte non-zero, no empty subtrees) and never validate it: `raw.rs`
//! builds and patches these bytes, `ops.rs` reads two trees in lockstep.
/// Bits `[3 * level, 3 * level + 3)` of `value`: the child slot `value`
/// occupies in its parent node, whose children head subtrees `level` levels
/// tall (level 0 indexes a bit within a leaf byte).
#[inline]
pub(crate) fn child_index(level: u32, value: u32) -> u8 {
    ((value >> (3 * level)) & 7) as u8
}

/// The inverse of `child_index`: the base value of child slot `child` at
/// tree level `level`, i.e. `child << (3 * level)`.
#[inline]
pub(crate) fn child_offset(level: u32, child: u8) -> u32 {
    (child as u32) << (3 * level)
}

/// Advance `pos` past the subtree whose root node byte is `data[pos]` and
/// whose height is `level` node bytes (1 = single leaf byte); returns the
/// position just past it. The slice-position counterpart of
/// `NodeReader::skip_subtree`, used by `insert`/`remove` to step over
/// existing sibling subtrees in place.
pub(crate) fn skip_subtree_at(data: &[u8], mut pos: usize, level: u32) -> usize {
    let node = data[pos];
    pos += 1;
    let level = level - 1;
    if level == 0 {
        return pos;
    }
    for child in 0..8u8 {
        if node & (1 << child) != 0 {
            pos = skip_subtree_at(data, pos, level);
        }
    }
    pos
}

/// One node byte of the serialized tree: bit `i` set means child `i`
/// (0..8) is present. Every stored byte is non-zero — empty nodes are
/// pruned rather than stored — and the bottom level's bytes are leaves,
/// where the bits are value bits read directly via `bits()`.
#[derive(Clone, Copy)]
pub(crate) struct Node(u8);

impl Node {
    /// Whether child slot `child` (0..8) is present.
    #[inline]
    pub(crate) fn has_child(self, child: u8) -> bool {
        self.0 & (1u8 << child) != 0
    }

    /// The number of present children; for a leaf byte, its value count.
    #[inline]
    pub(crate) fn count_children(self) -> u32 {
        self.0.count_ones()
    }

    /// The lowest present child index; assumes at least one child is set.
    #[inline]
    pub(crate) fn min_child(self) -> u8 {
        self.0.trailing_zeros() as u8
    }

    /// The highest present child index; assumes at least one child is set.
    #[inline]
    pub(crate) fn max_child(self) -> u8 {
        7 - self.0.leading_zeros() as u8
    }

    /// The raw bits of this node.
    #[inline]
    pub(crate) fn bits(self) -> u8 {
        self.0
    }
}

/// Sequential cursor over a tree's node bytes. Callers drive it in lockstep
/// with the tree structure: `next()` reads `nodes[pos]` with no bounds or
/// structure checks. The fields are crate-visible so `ops.rs` can copy a
/// traversed range verbatim (`copy_subtree`).
pub(crate) struct NodeReader<'a> {
    pub(crate) nodes: &'a [u8],
    pub(crate) pos: usize,
}

impl<'a> NodeReader<'a> {
    /// Create a cursor positioned at the first byte of the tree data.
    pub(crate) fn new(data: &'a [u8]) -> Self {
        Self {
            nodes: data,
            pos: 0,
        }
    }
}

impl NodeReader<'_> {
    /// Read the next node and advance the cursor.
    #[inline]
    pub(crate) fn next(&mut self) -> Node {
        let node = Node(self.nodes[self.pos]);
        self.pos += 1;
        node
    }

    /// The smallest value in the subtree rooted at the next node byte, which
    /// is `level` levels tall. Reads only the leftmost path down the
    /// subtree — the lowest child at every level — leaving the remaining
    /// bytes unread.
    pub(crate) fn min_value(&mut self, level: u32) -> u32 {
        let node = self.next();
        let level = level - 1;

        let min_child = node.min_child();

        if level == 0 {
            return min_child as u32;
        }

        let child_value = self.min_value(level);
        child_offset(level, min_child) + child_value
    }

    /// The largest value in the subtree rooted at the next node byte, which
    /// is `level` levels tall. Skips the subtrees of every child below the
    /// highest one, then descends the highest child's path — the rightmost,
    /// so the cursor ends at the subtree's end.
    pub(crate) fn max_value(&mut self, level: u32) -> u32 {
        let node = self.next();
        let level = level - 1;

        let max_child = node.max_child();

        if level == 0 {
            return max_child as u32;
        }

        // Skip all children before the highest set child.
        for child in 0..max_child {
            if node.has_child(child) {
                self.skip_subtree(level);
            }
        }

        let child_value = self.max_value(level);
        child_offset(level, max_child) + child_value
    }

    /// Advance the cursor past the subtree rooted at the next node byte,
    /// which is `level` levels tall, returning how many values it contains:
    /// a leaf byte counts its set bits, an inner node sums its present
    /// children.
    ///
    /// Mirrors `skip_hits` in GNU idutils' `src/fid.c`; the C
    /// original only skips — returning the population is this
    /// implementation's addition.
    pub(crate) fn skip_subtree(&mut self, level: u32) -> u64 {
        let node = self.next();
        let level = level - 1;

        if level == 0 {
            return node.count_children() as u64;
        }

        let mut count = 0u64;
        for child in 0..8u8 {
            if node.has_child(child) {
                count += self.skip_subtree(level);
            }
        }
        count
    }
}
