use std::io;

use crate::ceil_log8;
use crate::node::{child_index, child_offset, skip_subtree_at, NodeReader};
use crate::ops::{
    contains_inner, difference_subtree, intersect_subtree, range_count, remove_range_subtree,
    symmetric_difference_subtree, union_subtree,
};

/// A compressed bitmap descriptor using an 8-way bit-tree.
///
/// Each internal node is a single byte whose 8 bits indicate which of its
/// 8 children are present. Empty subtrees are pruned entirely. The serialized
/// form IS the in-memory form — point queries traverse it in O(levels).
///
/// This is a lightweight `Copy` descriptor (~5 bytes). The actual tree data
/// lives in an external `&[u8]` / `&mut Vec<u8>` passed to each method.
#[derive(Copy, Clone, Debug)]
#[cfg_attr(feature = "serde", derive(serde::Serialize, serde::Deserialize))]
pub struct RawBitmap {
    universe_size: u32,
    levels: u8,
}

impl RawBitmap {
    /// Create an empty bitmap descriptor for the given universe size.
    pub fn empty(universe_size: u32) -> Self {
        Self {
            universe_size,
            levels: ceil_log8(universe_size) as u8,
        }
    }

    /// Build a `RawBitmap` directly from a sorted iterator of values,
    /// appending tree bytes to `out`.
    ///
    /// Values **must** be in ascending order (duplicates are tolerated).
    /// Builds the depth-first pre-order serialization in a single pass,
    /// pushing new node bytes and back-patching child bits as groups change.
    ///
    /// Cost: O(N * levels) time, O(output_size) space — no bitvec allocation.
    pub fn from_sorted_iter(
        iter: impl Iterator<Item = u32>,
        universe_size: u32,
        out: &mut Vec<u8>,
    ) -> Self {
        let levels = ceil_log8(universe_size);
        if levels == 0 {
            return Self::empty(universe_size);
        }

        let base = out.len();
        let mut prev_group = [u32::MAX; 11];
        let mut node_pos = [0usize; 11];

        for v in iter {
            for dl in (0..levels).rev() {
                let group = v.checked_shr(3 * (dl + 1)).unwrap_or(0);

                if group != prev_group[dl as usize] {
                    prev_group[dl as usize] = group;
                    node_pos[dl as usize] = out.len();
                    out.push(0);
                }

                out[node_pos[dl as usize]] |= 1u8 << child_index(dl, v);
            }
        }

        // If nothing was appended, the bitmap is empty.
        if out.len() == base {
            return Self::empty(universe_size);
        }

        Self {
            universe_size,
            levels: levels as u8,
        }
    }

    /// Build a `RawBitmap` with all values in the given range set,
    /// appending tree bytes to `out`.
    ///
    /// Values outside `0..universe_size` are clamped/ignored.
    pub fn from_range(
        range: impl std::ops::RangeBounds<u32>,
        universe_size: u32,
        out: &mut Vec<u8>,
    ) -> Self {
        use std::ops::Bound;

        let start = match range.start_bound() {
            Bound::Included(&n) => n,
            Bound::Excluded(&n) => n.saturating_add(1),
            Bound::Unbounded => 0,
        };
        let end = match range.end_bound() {
            Bound::Included(&n) => n.saturating_add(1),
            Bound::Excluded(&n) => n,
            Bound::Unbounded => universe_size,
        };
        let end = end.min(universe_size);

        if start >= end {
            return Self::empty(universe_size);
        }

        Self::from_sorted_iter(start..end, universe_size, out)
    }

    /// The universe size (exclusive upper bound on values).
    pub fn universe_size(&self) -> u32 {
        self.universe_size
    }

    /// The number of levels in the tree.
    pub fn levels(&self) -> u32 {
        self.levels as u32
    }

    /// Test whether `value` is in the bitmap.
    pub fn contains(&self, data: &[u8], value: u32) -> bool {
        if value >= self.universe_size || data.is_empty() {
            return false;
        }

        contains_inner(&mut NodeReader::new(data), self.levels(), value)
    }

    /// Iterate over set bits in ascending order.
    pub fn iter<'a>(&self, data: &'a [u8]) -> Iter<'a> {
        if data.is_empty() {
            return Iter::empty();
        }

        let mut iter = Iter::empty();
        iter.data = data;

        let levels = self.levels();
        let child_level = levels - 1;
        if child_level == 0 {
            iter.leaf_bits = data[0];
            iter.pos = 1;
        } else {
            let root = data[0];
            iter.pos = 1;
            iter.stack[0] = IterFrame {
                node_bits: root,
                next_child: 0,
                base: 0,
                child_level,
            };
            iter.stack_len = 1;
            iter.advance_to_next_leaf();
        }

        iter
    }

    /// Count the number of set bits (population count).
    pub fn len(&self, data: &[u8]) -> u64 {
        if data.is_empty() {
            return 0;
        }

        NodeReader::new(data).skip_subtree(self.levels())
    }

    /// Returns `true` if no bits are set.
    pub fn is_empty(&self, data: &[u8]) -> bool {
        data.is_empty()
    }

    /// The smallest set value, or `None` if empty.
    pub fn min(&self, data: &[u8]) -> Option<u32> {
        if data.is_empty() {
            return None;
        }

        Some(NodeReader::new(data).min_value(self.levels()))
    }

    /// The largest set value, or `None` if empty.
    pub fn max(&self, data: &[u8]) -> Option<u32> {
        if data.is_empty() {
            return None;
        }

        Some(NodeReader::new(data).max_value(self.levels()))
    }

    /// Count the number of set bits within a range.
    pub fn range_cardinality(&self, data: &[u8], range: impl std::ops::RangeBounds<u32>) -> u64 {
        use std::ops::Bound;

        if data.is_empty() {
            return 0;
        }

        let start = match range.start_bound() {
            Bound::Included(&n) => n as u64,
            Bound::Excluded(&n) => n as u64 + 1,
            Bound::Unbounded => 0,
        };
        let end = match range.end_bound() {
            Bound::Included(&n) => n as u64 + 1,
            Bound::Excluded(&n) => n as u64,
            Bound::Unbounded => self.universe_size as u64,
        };

        if start >= end {
            return 0;
        }

        range_count(&mut NodeReader::new(data), self.levels(), 0, start, end)
    }

    /// Serialize to a writer.
    ///
    /// Wire format: `[universe_size: u32 LE][data_len: u32 LE][data bytes]`.
    pub fn serialize_into<W: io::Write>(&self, data: &[u8], mut writer: W) -> io::Result<()> {
        writer.write_all(&self.universe_size.to_le_bytes())?;
        writer.write_all(&(data.len() as u32).to_le_bytes())?;
        writer.write_all(data)?;
        Ok(())
    }

    /// Deserialize from a reader. Returns the descriptor and owned data.
    pub fn deserialize_from<R: io::Read>(mut reader: R) -> io::Result<(Self, Vec<u8>)> {
        let mut buf = [0u8; 4];
        reader.read_exact(&mut buf)?;
        let universe_size = u32::from_le_bytes(buf);

        reader.read_exact(&mut buf)?;
        let data_len = u32::from_le_bytes(buf) as usize;

        let mut data = vec![0u8; data_len];
        reader.read_exact(&mut data)?;

        let levels = ceil_log8(universe_size);

        Ok((
            Self {
                universe_size,
                levels: levels as u8,
            },
            data,
        ))
    }

    /// The number of bytes this bitmap occupies when serialized.
    pub fn serialized_size(&self, data: &[u8]) -> usize {
        8 + data.len()
    }

    /// Insert a value into the bitmap. No-op if already present.
    ///
    /// Panics if `value >= universe_size`.
    pub fn insert(&self, data: &mut Vec<u8>, value: u32) {
        assert!(
            value < self.universe_size,
            "value {value} out of bounds for universe_size {}",
            self.universe_size
        );

        let levels = self.levels();

        if data.is_empty() {
            data.reserve(levels as usize);
            for dl in (0..levels).rev() {
                data.push(1u8 << child_index(dl, value));
            }
            return;
        }

        let mut pos = 0;
        for dl in (0..levels).rev() {
            let child = child_index(dl, value);

            if dl == 0 {
                data[pos] |= 1u8 << child;
                return;
            }

            let node = data[pos];
            if node & (1u8 << child) != 0 {
                pos += 1;
                for sibling in 0..child {
                    if node & (1u8 << sibling) != 0 {
                        pos = skip_subtree_at(data, pos, dl);
                    }
                }
            } else {
                data[pos] |= 1u8 << child;
                pos += 1;
                for sibling in 0..child {
                    if node & (1u8 << sibling) != 0 {
                        pos = skip_subtree_at(data, pos, dl);
                    }
                }
                let path_len = dl as usize;
                let mut new_path = [0u8; 11];
                for (i, l) in (0..dl).rev().enumerate() {
                    new_path[i] = 1u8 << child_index(l, value);
                }
                data.splice(pos..pos, new_path[..path_len].iter().copied());
                return;
            }
        }
    }

    /// Remove a value from the bitmap. No-op if not present.
    ///
    /// Panics if `value >= universe_size`.
    pub fn remove(&self, data: &mut Vec<u8>, value: u32) {
        assert!(
            value < self.universe_size,
            "value {value} out of bounds for universe_size {}",
            self.universe_size
        );

        if data.is_empty() {
            return;
        }

        let levels = self.levels();
        let depth = levels as usize;
        let mut positions = [0usize; 11];
        let mut children = [0u8; 11];

        let mut pos = 0;
        for d in 0..depth {
            let dl = levels - 1 - d as u32;
            let child = child_index(dl, value);
            let node = data[pos];

            positions[d] = pos;
            children[d] = child;

            if node & (1u8 << child) == 0 {
                return;
            }

            if dl == 0 {
                break;
            }

            pos += 1;
            for sibling in 0..child {
                if node & (1u8 << sibling) != 0 {
                    pos = skip_subtree_at(data, pos, dl);
                }
            }
        }

        let leaf_depth = depth - 1;
        let leaf_pos = positions[leaf_depth];
        data[leaf_pos] &= !(1u8 << children[leaf_depth]);

        if data[leaf_pos] != 0 {
            return;
        }

        let mut remove_start = leaf_pos;
        for d in (0..leaf_depth).rev() {
            let parent_pos = positions[d];
            data[parent_pos] &= !(1u8 << children[d]);
            if data[parent_pos] != 0 {
                break;
            }
            remove_start = parent_pos;
        }

        data.drain(remove_start..leaf_pos + 1);
    }

    /// Remove all values in the given range from the bitmap.
    pub fn remove_range(&self, data: &mut Vec<u8>, range: impl std::ops::RangeBounds<u32>) {
        use std::ops::Bound;

        if data.is_empty() {
            return;
        }

        let start = match range.start_bound() {
            Bound::Included(&n) => n as u64,
            Bound::Excluded(&n) => n as u64 + 1,
            Bound::Unbounded => 0,
        };
        let end = match range.end_bound() {
            Bound::Included(&n) => n as u64 + 1,
            Bound::Excluded(&n) => n as u64,
            Bound::Unbounded => self.universe_size as u64,
        };

        if start >= end {
            return;
        }

        let mut out = Vec::with_capacity(data.len());
        remove_range_subtree(
            &mut NodeReader::new(data),
            self.levels(),
            0,
            start,
            end,
            &mut out,
        );
        *data = out;
    }

    /// Compute the union of two bitmaps, appending the result to `out`.
    /// Returns a new descriptor for the result.
    pub fn union(&self, a: &[u8], other: &RawBitmap, b: &[u8], out: &mut Vec<u8>) -> RawBitmap {
        assert_eq!(
            self.universe_size, other.universe_size,
            "universe_size mismatch: {} vs {}",
            self.universe_size, other.universe_size
        );

        if a.is_empty() {
            out.extend_from_slice(b);
            return *other;
        }
        if b.is_empty() {
            out.extend_from_slice(a);
            return *self;
        }

        let base = out.len();
        let mut ra = NodeReader::new(a);
        let mut rb = NodeReader::new(b);
        union_subtree(&mut ra, &mut rb, self.levels(), out);

        if out.len() == base {
            RawBitmap::empty(self.universe_size)
        } else {
            *self
        }
    }

    /// Compute the intersection of two bitmaps, appending the result to `out`.
    pub fn intersect(&self, a: &[u8], other: &RawBitmap, b: &[u8], out: &mut Vec<u8>) -> RawBitmap {
        assert_eq!(
            self.universe_size, other.universe_size,
            "universe_size mismatch: {} vs {}",
            self.universe_size, other.universe_size
        );

        if a.is_empty() || b.is_empty() {
            return RawBitmap::empty(self.universe_size);
        }

        let base = out.len();
        let mut ra = NodeReader::new(a);
        let mut rb = NodeReader::new(b);
        intersect_subtree(&mut ra, &mut rb, self.levels(), out);

        if out.len() == base {
            RawBitmap::empty(self.universe_size)
        } else {
            *self
        }
    }

    /// Compute the difference (self - other), appending the result to `out`.
    pub fn difference(
        &self,
        a: &[u8],
        other: &RawBitmap,
        b: &[u8],
        out: &mut Vec<u8>,
    ) -> RawBitmap {
        assert_eq!(
            self.universe_size, other.universe_size,
            "universe_size mismatch: {} vs {}",
            self.universe_size, other.universe_size
        );

        if a.is_empty() {
            return RawBitmap::empty(self.universe_size);
        }
        if b.is_empty() {
            out.extend_from_slice(a);
            return *self;
        }

        let base = out.len();
        let mut ra = NodeReader::new(a);
        let mut rb = NodeReader::new(b);
        difference_subtree(&mut ra, &mut rb, self.levels(), out);

        if out.len() == base {
            RawBitmap::empty(self.universe_size)
        } else {
            *self
        }
    }

    /// Compute the symmetric difference (self ^ other), appending the result to `out`.
    pub fn symmetric_difference(
        &self,
        a: &[u8],
        other: &RawBitmap,
        b: &[u8],
        out: &mut Vec<u8>,
    ) -> RawBitmap {
        assert_eq!(
            self.universe_size, other.universe_size,
            "universe_size mismatch: {} vs {}",
            self.universe_size, other.universe_size
        );

        if a.is_empty() {
            out.extend_from_slice(b);
            return *other;
        }
        if b.is_empty() {
            out.extend_from_slice(a);
            return *self;
        }

        let base = out.len();
        let mut ra = NodeReader::new(a);
        let mut rb = NodeReader::new(b);
        symmetric_difference_subtree(&mut ra, &mut rb, self.levels(), out);

        if out.len() == base {
            RawBitmap::empty(self.universe_size)
        } else {
            *self
        }
    }
}

/// Iterator over set bits of a `RawBitmap`.
///
/// Uses a stack-based DFS traversal over the compressed tree, avoiding
/// materialization of a full bitvec. Cost is proportional to the number
/// of tree nodes visited, not the universe size.
pub struct Iter<'a> {
    data: &'a [u8],
    pos: usize,
    stack: [IterFrame; 10],
    stack_len: usize,
    /// Remaining set bits in the current leaf byte.
    leaf_bits: u8,
    /// Base value for the current leaf (value = leaf_base + bit index).
    leaf_base: u32,
}

#[derive(Default, Clone, Copy)]
struct IterFrame {
    node_bits: u8,
    next_child: u8,
    base: u32,
    child_level: u32,
}

impl<'a> Iter<'a> {
    fn empty() -> Self {
        Self {
            data: &[],
            pos: 0,
            stack: [IterFrame::default(); 10],
            stack_len: 0,
            leaf_bits: 0,
            leaf_base: 0,
        }
    }

    /// Advance the DFS to the next leaf byte. Returns `true` if a leaf was found.
    fn advance_to_next_leaf(&mut self) -> bool {
        while self.stack_len > 0 {
            let frame = &mut self.stack[self.stack_len - 1];

            if frame.next_child >= 8 {
                self.stack_len -= 1;
                continue;
            }
            let remaining = frame.node_bits >> frame.next_child;
            if remaining == 0 {
                self.stack_len -= 1;
                continue;
            }

            let skip = remaining.trailing_zeros() as u8;
            let child = frame.next_child + skip;
            frame.next_child = child + 1;

            let new_base = frame.base + child_offset(frame.child_level, child);
            let child_level = frame.child_level;

            if child_level == 1 {
                self.leaf_bits = self.data[self.pos];
                self.pos += 1;
                self.leaf_base = new_base;
                return true;
            } else {
                let node_byte = self.data[self.pos];
                self.pos += 1;
                self.stack[self.stack_len] = IterFrame {
                    node_bits: node_byte,
                    next_child: 0,
                    base: new_base,
                    child_level: child_level - 1,
                };
                self.stack_len += 1;
            }
        }
        false
    }
}

impl Iterator for Iter<'_> {
    type Item = u32;

    fn next(&mut self) -> Option<u32> {
        loop {
            if self.leaf_bits != 0 {
                let bit = self.leaf_bits.trailing_zeros();
                self.leaf_bits &= self.leaf_bits - 1;
                return Some(self.leaf_base + bit);
            }
            if !self.advance_to_next_leaf() {
                return None;
            }
        }
    }
}
