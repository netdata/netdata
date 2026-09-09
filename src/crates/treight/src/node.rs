/// Extract the 3-bit child index for `value` at the given tree `level`.
#[inline]
pub(crate) fn child_index(level: u32, value: u32) -> u8 {
    ((value >> (3 * level)) & 7) as u8
}

/// The value contribution of child index `child` at the given tree `level`.
#[inline]
pub(crate) fn child_offset(level: u32, child: u8) -> u32 {
    (child as u32) << (3 * level)
}

/// Advance `pos` past the subtree rooted at `data[pos]` which is `level` levels
/// tall (1 = single leaf byte). Returns the position immediately after the subtree.
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

/// A single node byte from a serialized treight.
///
/// Each bit indicates whether the corresponding child (0..8) is present.
#[derive(Clone, Copy)]
pub(crate) struct Node(u8);

impl Node {
    /// Test whether child `child` is present.
    #[inline]
    pub(crate) fn has_child(self, child: u8) -> bool {
        self.0 & (1u8 << child) != 0
    }

    /// The number of present children.
    #[inline]
    pub(crate) fn count_children(self) -> u32 {
        self.0.count_ones()
    }

    /// The lowest present child index.
    #[inline]
    pub(crate) fn min_child(self) -> u8 {
        self.0.trailing_zeros() as u8
    }

    /// The highest present child index.
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

/// Cursor for reading nodes sequentially from a serialized treight.
pub(crate) struct NodeReader<'a> {
    pub(crate) nodes: &'a [u8],
    pub(crate) pos: usize,
}

impl<'a> NodeReader<'a> {
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

    /// Find the minimum value in the subtree rooted at `level`.
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

    /// Find the maximum value in the subtree rooted at `level`.
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

    /// Skip over a subtree rooted at `level`, returning the number of values it contains.
    ///
    /// Matches `skip_hits` from `src/fid.c:280-293`.
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
