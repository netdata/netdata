//! The core of `treight`: [`RawBitmap`], the `Copy` descriptor of an
//! 8-way bit-tree, and [`Iter`], its ascending value iterator. The
//! descriptor holds just the universe size and the tree depth; the tree
//! bytes — the pre-order node-byte layout node.rs defines — live outside,
//! in the `&[u8]` / `&mut Vec<u8>` blob every method takes, and a
//! zero-byte blob is the empty set.
//!
//! This file owns the blob's shape: `from_sorted_iter`/`from_range` write
//! it from scratch, `remove_range` rewrites it through ops.rs's walker,
//! `insert`/`remove` patch it in place (the crate's only tree-byte
//! splice/drain), and `serialize_into`/`deserialize_from` define the
//! `[universe: u32 LE][len: u32 LE][tree bytes]` wire format. Queries and
//! set ops delegate to the walkers in node.rs/ops.rs; the one traversal
//! implemented here is the ascending `Iter` below. Every path keeps the
//! blob canonical — every byte non-zero, no empty subtrees — which is
//! what makes `is_empty` a blob-emptiness check and ties the length to
//! `estimate_data_size` (the `fuzz/fuzz_targets/against_roaring.rs` fuzz
//! target asserts the tie after arbitrary op sequences).
//! Consumers: bitmap.rs wraps the descriptor in the inverted-aware
//! `Bitmap`; roaring.rs (feature `roaring`) adds `RoaringBitmap` bridges;
//! sfst, sfsq and otel-ledger only ever touch `Bitmap` (grep-verified),
//! while the fuzz target drives `RawBitmap` directly.
use std::io;

use crate::ceil_log8;
use crate::node::{NodeReader, child_index, child_offset, skip_subtree_at};
use crate::ops::{
    contains_inner, difference_subtree, intersect_subtree, range_count, remove_range_subtree,
    symmetric_difference_subtree, union_subtree,
};

/// A compressed bitmap's descriptor: `universe_size`, the exclusive upper
/// bound on values, and `levels`, the tree depth `ceil_log8` derives from
/// it (lib.rs) — both fixed at construction. The tree bytes live outside,
/// in the `&[u8]` / `&mut Vec<u8>` each method takes, so this is a freely
/// `Copy` handle whose state is whatever blob it is paired with. Two
/// descriptors of the same universe are interchangeable — which is why
/// the set ops below return either operand's. The optional `serde` derive
/// covers the descriptor only; tree bytes travel beside it in the
/// consumer's payload (`BitmapValue` in `sfst/src/schema.rs`).
#[derive(Copy, Clone, Debug)]
#[cfg_attr(feature = "serde", derive(serde::Serialize, serde::Deserialize))]
pub struct RawBitmap {
    universe_size: u32,
    levels: u8,
}

impl RawBitmap {
    /// Create an empty descriptor for `universe_size`: the depth comes from
    /// `ceil_log8` (0 only for a zero-sized universe), and the tree blob is
    /// left empty — the first `insert` writes the whole root-to-leaf chain.
    pub fn empty(universe_size: u32) -> Self {
        Self {
            universe_size,
            levels: ceil_log8(universe_size) as u8,
        }
    }

    /// Build a bitmap from an ascending iterator of values, appending the
    /// tree bytes to `out` and returning the descriptor. The values **must**
    /// arrive in ascending order — a descending one appends a node byte for
    /// an index already emitted and corrupts the pre-order layout — while
    /// duplicates are tolerated (they re-OR a set bit). Values at or past
    /// the universe are not checked either.
    ///
    /// Single pass over the pre-order serialization: when a value's node
    /// index changes at some level, a zeroed node byte is pushed there and
    /// every value ORs its bit into the byte currently open at that level.
    /// Cost is O(values × levels) with no bitvec staging; an empty iterator
    /// appends nothing and yields an empty descriptor, and otherwise the
    /// bytes appended are exactly `estimate_data_size`'s count.
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
        // Per-level scratch, indexed by the loop's dl (≤ 10 for a u32
        // universe): u32::MAX forces the first value to open a byte at
        // every level.
        let mut prev_group = [u32::MAX; 11];
        let mut node_pos = [0usize; 11];

        for v in iter {
            for dl in (0..levels).rev() {
                // Node index at this level. `checked_shr` covers the
                // 11-level tree, whose root shift 3 × 11 = 33 is past u32:
                // the index is always 0 there.
                let group = v.checked_shr(3 * (dl + 1)).unwrap_or(0);

                if group != prev_group[dl as usize] {
                    prev_group[dl as usize] = group;
                    node_pos[dl as usize] = out.len();
                    out.push(0);
                }

                out[node_pos[dl as usize]] |= 1u8 << child_index(dl, v);
            }
        }

        // No values: leave `out` untouched and return the empty descriptor.
        if out.len() == base {
            return Self::empty(universe_size);
        }

        Self {
            universe_size,
            levels: levels as u8,
        }
    }

    /// Build a bitmap with every value in `range` set, appending the tree
    /// bytes to `out`. The end bound is clamped to the universe size; a
    /// range starting at or past the clamped end yields an empty
    /// descriptor. The clamped range feeds
    /// [`from_sorted_iter`](Self::from_sorted_iter) value by value — no
    /// complement trick here; `Bitmap::from_range` is the density-aware
    /// caller-facing version.
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

    /// The tree depth — `ceil_log8(universe_size)`, fixed per universe.
    pub fn levels(&self) -> u32 {
        self.levels as u32
    }

    /// Test whether `value` is set. Out-of-universe values and empty blobs
    /// return `false` — no panic, unlike `insert`/`remove`. The descent is
    /// ops.rs's `contains_inner` (the fid.c `is_hit_1` port): it walks the
    /// value's path, skipping lower siblings' subtrees on the way, so cost
    /// tracks the tree bytes before the path, not just the depth.
    pub fn contains(&self, data: &[u8], value: u32) -> bool {
        if value >= self.universe_size || data.is_empty() {
            return false;
        }

        contains_inner(&mut NodeReader::new(data), self.levels(), value)
    }

    /// Iterate over the set values in ascending order. An empty blob
    /// yields an empty iterator; see [`Iter`] for the walk itself.
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

    /// The number of set values: a full walk summing each leaf byte's bits
    /// (`NodeReader::skip_subtree`); 0 for an empty blob.
    pub fn len(&self, data: &[u8]) -> u64 {
        if data.is_empty() {
            return 0;
        }

        NodeReader::new(data).skip_subtree(self.levels())
    }

    /// `true` when nothing is set. O(1): blobs are canonical — a non-empty
    /// one always holds at least one value — so this is just an emptiness
    /// check on the blob.
    pub fn is_empty(&self, data: &[u8]) -> bool {
        data.is_empty()
    }

    /// The smallest set value, or `None` for an empty blob. O(levels):
    /// descends the leftmost path only (`NodeReader::min_value`).
    pub fn min(&self, data: &[u8]) -> Option<u32> {
        if data.is_empty() {
            return None;
        }

        Some(NodeReader::new(data).min_value(self.levels()))
    }

    /// The largest set value, or `None` for an empty blob. Skips every
    /// child subtree left of the rightmost path, then descends it
    /// (`NodeReader::max_value`) — cost tracks the skipped subtrees, not
    /// just the depth.
    pub fn max(&self, data: &[u8]) -> Option<u32> {
        if data.is_empty() {
            return None;
        }

        Some(NodeReader::new(data).max_value(self.levels()))
    }

    /// Count the set values within `range`. Bounds widen to u64 (safe at
    /// the deepest levels), an unbounded end means the universe size, and
    /// an empty range or blob is 0. ops.rs's `range_count` skips subtrees
    /// fully outside the range, counts subtrees fully inside, and
    /// recurses only where a child straddles an edge.
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

    /// Serialize to a writer: `[universe: u32 LE][blob len: u32 LE][tree
    /// bytes]`. The tree bytes are copied verbatim; the level count is not
    /// stored — `deserialize_from` rederives it from the universe size.
    pub fn serialize_into<W: io::Write>(&self, data: &[u8], mut writer: W) -> io::Result<()> {
        writer.write_all(&self.universe_size.to_le_bytes())?;
        writer.write_all(&(data.len() as u32).to_le_bytes())?;
        writer.write_all(data)?;
        Ok(())
    }

    /// Deserialize a `serialize_into` stream: universe size and blob
    /// length (u32 LE each), then the tree bytes. `levels` is rederived
    /// via `ceil_log8` rather than stored, and the blob is taken as-is —
    /// canonical form is assumed, never validated.
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

    /// Serialized length: the 8-byte header plus the tree bytes.
    pub fn serialized_size(&self, data: &[u8]) -> usize {
        8 + data.len()
    }

    /// Insert `value`. No-op if already present; panics if
    /// `value >= universe_size`.
    ///
    /// Walks the value's path from the root, stepping over lower siblings'
    /// subtrees with `skip_subtree_at` (node.rs). At the leaf level the
    /// value's bit is ORed in. If an inner level lacks the child link, the
    /// link bit is set and the missing subtree — a chain of single-child
    /// node bytes, one per remaining level down to the leaf — is spliced
    /// in at the walk position. An empty blob short-circuits: the whole
    /// root-to-leaf chain (`levels` bytes) is written directly.
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

    /// Remove `value`. No-op if absent; panics if `value >= universe_size`.
    ///
    /// Walks the path recording each node's position, returning untouched
    /// the moment a child link is missing. The leaf bit is cleared, then
    /// the walk backs up clearing each parent's link bit until one keeps
    /// other children (or the root empties). The cleared nodes are
    /// contiguous in the pre-order bytes — an emptied node had only the
    /// path child — so a single `drain` removes them; if the root empties
    /// too, the blob returns to zero bytes.
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

    /// Remove every value in `range`, rebuilding the tree into a fresh
    /// buffer via ops.rs's `remove_range_subtree` — subtrees fully outside
    /// the range are copied, fully inside are dropped, straddling ones
    /// recursed — and swapping it in. Empty blob or empty range is a
    /// no-op; removing everything leaves a zero-byte blob. Bounds widen to
    /// u64; an unbounded end means the universe size.
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

    /// Union of tree `a` (this descriptor) with tree `b` (`other`),
    /// appending the result's tree bytes to `out`. Universe sizes must
    /// match (asserted). An empty blob short-circuits: the surviving
    /// operand's bytes are copied to `out` and its descriptor returned.
    /// Otherwise ops.rs's `union_subtree` walks both trees in lockstep; a
    /// union of non-empty trees is never empty, so the result is always
    /// appended. The returned descriptor is either operand's — same
    /// universe, same levels.
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

    /// Intersection of the two trees, appending the result's tree bytes to
    /// `out`. Universe sizes must match (asserted). Either blob empty
    /// short-circuits to the empty descriptor with no bytes; ops.rs's
    /// `intersect_subtree` descends only children present in both, so
    /// disjoint trees append nothing and return the empty descriptor.
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

    /// Difference `a` − `b`, appending the result's tree bytes to `out`.
    /// Universe sizes must match (asserted). `b` empty copies `a` back out
    /// and returns `*self`; `a` empty is the empty descriptor with no
    /// bytes; ops.rs's `difference_subtree` copies children unique to `a`
    /// and drops those unique to `b`, so a `b` that covers `a` empties
    /// the result: nothing appended, empty descriptor returned.
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

    /// Symmetric difference `a` ^ `b`, appending the result's tree bytes
    /// to `out`. Universe sizes must match (asserted). Either blob empty
    /// copies the survivor back out and returns its descriptor; ops.rs's
    /// `symmetric_difference_subtree` copies children unique to one side
    /// and XORs shared leaves, so equal inputs append nothing and return
    /// the empty descriptor.
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

/// Iterator over a `RawBitmap`'s set values in ascending order, built by
/// `RawBitmap::iter`. A stack-based left-to-right DFS over the stored
/// tree: leaf bytes are consumed bit by bit in place, so cost tracks the
/// tree nodes visited, never the universe size, and nothing is
/// materialized.
pub struct Iter<'a> {
    /// The tree bytes being walked; empty for the do-nothing iterator.
    data: &'a [u8],
    /// Position of the next unread node byte in `data`.
    pos: usize,
    /// One frame per open node on the descent path, root first; at most
    /// `levels - 1` deep (10 for a u32 universe), which is the array's size.
    stack: [IterFrame; 10],
    /// Frames in use.
    stack_len: usize,
    /// Remaining set bits in the current leaf byte.
    leaf_bits: u8,
    /// Base value for the current leaf (value = leaf_base + bit index).
    leaf_base: u32,
}

/// One open node on the DFS stack: its node byte, the next child index to
/// scan, the base value of its subtree, and the height (in node bytes) of
/// its children's subtrees.
#[derive(Default, Clone, Copy)]
struct IterFrame {
    node_bits: u8,
    next_child: u8,
    base: u32,
    child_level: u32,
}

impl<'a> Iter<'a> {
    /// The all-zero iterator: what `RawBitmap::iter` returns as-is for an
    /// empty blob and fills in otherwise.
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

    /// Consume the next leaf byte: pop exhausted frames, scan each frame's
    /// node byte for its next set child (trailing-zeros skip), descend
    /// inner children by pushing a frame, and stop at the first leaf, whose
    /// bits move to `leaf_bits`. `false` when the tree is exhausted.
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

    /// Pop the lowest set bit of the current leaf byte (`leaf_base` plus
    /// its index), refilling from the DFS when the byte is exhausted.
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
