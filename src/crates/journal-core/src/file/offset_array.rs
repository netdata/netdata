//! Entry-array cursors for the journal file: the chain machinery behind
//! `JournalCursor`'s unfiltered stepping and `FilterExpr`'s per-match
//! scanning. One array is an `OffsetArrayObject` - an object header plus
//! `Option<NonZeroU64>` (compact: `Option<NonZeroU32>`) slots of
//! entry-object offsets, linked to the next array through
//! `OffsetArrayObjectHeader::next_offset_array` (`file/object.rs`).
//! A chain is addressed by [`List`]: head array offset plus the number
//! of entries it logically holds. Arrays grow by doubling when full -
//! 4096 slots for the first file-global array, 64 for a data object's
//! first one (allocated by `JournalWriter::append_to_entry_array` and
//! `link_data_to_entry`).
//!
//! Two producers build chains, and both list entry offsets in file
//! write order, strictly increasing end to end - each entry is written
//! at the writer's advancing append offset (`JournalWriter`'s
//! `add_entry`):
//! - the file-global entry chain: every entry of the file, head in
//!   `JournalHeader::entry_array_offset`, `total_items = n_entries`
//!   ([`JournalFile::entry_list`]; appended per entry by
//!   `JournalWriter::add_entry`);
//! - a data object's entry chain: the entries carrying that payload.
//!   The first entry is inlined in the data object itself
//!   (`DataObjectHeader::entry_offset`), the linked array holds the
//!   remaining `n_entries - 1` (the links live in `DataObjectHeader`;
//!   built by `JournalWriter::link_data_to_entry`),
//!   and [`InlinedCursor`] walks the pair - inlined entry first, arrays
//!   after - as the scan state inside
//!   [`FilterExpr::Match`](crate::file::filter::FilterExpr::Match).
//!
//! [`Node`] is a re-read view of one array; [`Cursor`] is a position in
//! a chain, the value
//! [`JournalCursor::array_cursor`](crate::file::cursor::JournalCursor::array_cursor)
//! keeps for the
//! unfiltered path. `next_until`/`previous_until`
//! continue from the current scan position and clamp to a needle (see
//! their docs) - the contract the filtered resolution
//! (`JournalCursor::resolve_filter_location`) obeys by rewinding with
//! `head()`/
//! `tail()` first. Errors: `InvalidOffsetArrayIndex` past a position's
//! logical length, `InvalidOffset` when a binary search probes an unset
//! slot, `EmptyOffsetArrayNode` for a zero-capacity array, and
//! `InvalidOffsetArrayOffset` when `Cursor::previous` cannot find its
//! array linked from the chain's head.
//!
//! Consumers (grep-verified): `List` also parses remapping entries
//! (`JournalReader::parse_remapping_entries_from_array`) and `Direction`
//! is flat re-exported beside `Location` (`file/mod.rs`, lib.rs);
//! cross-crate, journal-index
//! imports `InlinedCursor` and walks it outside its data-object value
//! guards (src/crates/journal-index/src/file_indexer.rs).
//!
//! Near twin: src/crates/jf/journal_file/src/offset_array.rs - same
//! types and stepping logic; only this copy adds the three
//! `collect_offsets` helpers and the `allocative` derives.
use super::mmap::MemoryMap;
use crate::error::{JournalError, Result};
use crate::file::JournalFile;
use std::num::{NonZeroU64, NonZeroUsize};

/// Walk direction shared by the stepping and partition-point APIs.
/// Flat re-exported beside `Location` (`file/mod.rs`, lib.rs).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Direction {
    Forward,
    Backward,
}

/// A view of one array in a chain, re-read from the file through
/// `JournalFile::offset_array_ref` on every construction. Helper for
/// the cursors below; nothing outside this file constructs one.
///
/// `remaining_items` is the count of chain entries from this array's
/// first slot to the chain's end - it turns the on-disk slot count
/// into a logical length (`len`) and decides whether entries
/// continue into a linked array (`has_next`).
pub struct Node {
    offset: NonZeroU64,
    next_offset: Option<NonZeroU64>,
    capacity: NonZeroUsize,
    // Chain entries from this array's first slot onward
    remaining_items: NonZeroUsize,
}

impl Node {
    /// Reads the array at `offset`; `EmptyOffsetArrayNode` if its
    /// capacity is zero.
    fn new<M: MemoryMap>(
        journal_file: &JournalFile<M>,
        offset: NonZeroU64,
        remaining_items: NonZeroUsize,
    ) -> Result<Self> {
        let array = journal_file.offset_array_ref(offset)?;
        let capacity =
            NonZeroUsize::new(array.capacity()).ok_or(JournalError::EmptyOffsetArrayNode)?;

        Ok(Self {
            offset,
            next_offset: array.header.next_offset_array,
            capacity,
            remaining_items,
        })
    }

    /// Get the offset of this array in the file
    pub fn offset(&self) -> NonZeroU64 {
        self.offset
    }

    /// Get the maximum number of items this array can hold
    pub fn capacity(&self) -> NonZeroUsize {
        self.capacity
    }

    /// Logical length: how many of the chain's remaining entries this
    /// array holds - `min(capacity, remaining_items)`.
    pub fn len(&self) -> NonZeroUsize {
        self.capacity.min(self.remaining_items)
    }

    /// Whether entries logically continue past this array: true when
    /// `remaining_items` exceeds the capacity and the header links a
    /// next array. A missing link while items remain ends the chain
    /// early.
    pub fn has_next(&self) -> bool {
        self.next_offset.is_some() && self.remaining_items > self.len()
    }

    /// The next array, carrying the chain entries left after this
    /// one; `None` at the end of the chain.
    pub fn next<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        if !self.has_next() {
            return Ok(None);
        }

        // `has_next` guarantees remaining_items > len, so the
        // subtraction cannot underflow; the guard is defensive.
        let next_offset = self.next_offset.unwrap();
        let remaining_items = {
            let n = self.remaining_items.get().saturating_sub(self.len().get());
            NonZeroUsize::new(n).ok_or(JournalError::EmptyOffsetArrayNode)?
        };
        let node = Self::new(journal_file, next_offset, remaining_items);

        Some(node).transpose()
    }

    /// The entry offset at `index`; `Ok(None)` for a slot not
    /// written yet, `InvalidOffsetArrayIndex` past the logical
    /// length.
    pub fn get<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        index: usize,
    ) -> Result<Option<NonZeroU64>> {
        if index >= self.len().get() {
            return Err(JournalError::InvalidOffsetArrayIndex);
        }

        let array = journal_file.offset_array_ref(self.offset)?;
        array.get(index, self.remaining_items.get())
    }

    /// Upper-bound binary search over `[left, right)`: the first
    /// index where `predicate` returns false, or `right` when it is
    /// true throughout.
    ///
    /// `predicate` means "keep scanning", so it must be monotone -
    /// true entries before false ones - matching the array's
    /// strictly increasing entry offsets (the writer appends entries
    /// at a growing offset).
    /// Probes re-read the array through the file; an unset slot
    /// mid-search reports `InvalidOffset`.
    pub fn partition_point<M, F>(
        &self,
        journal_file: &JournalFile<M>,
        left: usize,
        right: usize,
        predicate: F,
    ) -> Result<usize>
    where
        M: MemoryMap,
        F: Fn(NonZeroU64) -> Result<bool>,
    {
        let mut left = left;
        let mut right = right;

        debug_assert!(left <= right);
        debug_assert!(right <= self.len().get());

        while left != right {
            let mid = left.midpoint(right);
            let Some(offset) = self.get(journal_file, mid)? else {
                return Err(JournalError::InvalidOffset);
            };

            if predicate(offset)? {
                left = mid + 1;
            } else {
                right = mid;
            }
        }

        Ok(left)
    }

    /// Where scanning stops in `direction` within `[left, right)`:
    /// forward, the first index whose `predicate` is false (the
    /// first match for callers that match on `!predicate`);
    /// backward, the last index whose `predicate` is true. `None`
    /// when the range holds no such index.
    pub fn directed_partition_point<M, F>(
        &self,
        journal_file: &JournalFile<M>,
        left: usize,
        right: usize,
        predicate: F,
        direction: Direction,
    ) -> Result<Option<usize>>
    where
        M: MemoryMap,
        F: Fn(NonZeroU64) -> Result<bool>,
    {
        let index = self.partition_point(journal_file, left, right, predicate)?;

        Ok(match direction {
            Direction::Forward => {
                if index < self.len().get() {
                    Some(index)
                } else {
                    None
                }
            }
            Direction::Backward => {
                if index > 0 {
                    Some(index - 1)
                } else {
                    None
                }
            }
        })
    }
}

impl std::fmt::Debug for Node {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let next_offset = self.next_offset.map(|x| x.get()).unwrap_or(0);

        f.debug_struct("Node")
            .field("offset", &format!("0x{:x}", self.offset))
            .field("next_offset", &format!("0x{:x}", next_offset))
            .field("capacity", &self.capacity)
            .field("len", &self.len())
            .field("remaining_items", &self.remaining_items)
            .finish()
    }
}

/// A handle to a chain of offset arrays: the head array's file
/// offset plus how many entries the chain logically holds
/// (`total_items`). `Copy` and cheap - arrays are resolved from the
/// file on demand.
///
/// Built for the file-global entry chain by [`JournalFile::entry_list`]
/// and for a data object's entry chain by
/// `DataObjectHeader::inlined_cursor` (`file/object.rs`). The
/// writer fills both in entry-write order, so entries sit at
/// strictly increasing offsets along the chain
/// (appended by `JournalWriter` as entries are written).
#[derive(Copy, Clone)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct List {
    head_offset: NonZeroU64,
    total_items: NonZeroUsize,
}

impl std::fmt::Debug for List {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("List")
            .field("head_offset", &format!("0x{:x}", self.head_offset))
            .field("total_items", &self.total_items)
            .finish()
    }
}

impl List {
    /// Create a new list from head offset and total items
    pub fn new(head_offset: NonZeroU64, total_items: NonZeroUsize) -> Self {
        Self {
            head_offset,
            total_items,
        }
    }

    /// The head array, with the whole chain's `total_items` as its
    /// remaining count.
    pub fn head<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        Node::new(journal_file, self.head_offset, self.total_items)
    }

    /// The tail array, found by walking the chain - O(chain length).
    pub fn tail<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        let mut current = self.head(journal_file)?;

        while let Some(next) = current.next(journal_file)? {
            current = next;
        }

        Ok(current)
    }

    /// A cursor at the chain's first entry.
    pub fn cursor_head(self) -> Cursor {
        Cursor::at_head(self)
    }

    /// A cursor at the chain's last entry (walks the chain to find
    /// the tail array).
    pub fn cursor_tail<M: MemoryMap>(self, journal_file: &JournalFile<M>) -> Result<Cursor> {
        Cursor::at_tail(journal_file, self)
    }

    /// The chain position where scanning stops in `direction`, as a
    /// cursor: forward, the first entry whose `predicate` is false;
    /// backward, the last entry whose `predicate` is true. `None`
    /// when no such entry exists in the arrays.
    ///
    /// `predicate` means "keep scanning" (`Node::partition_point`).
    /// Each array is binary-searched; the walk stops at the first
    /// array with a forward match, or after the last backward one -
    /// valid because the predicate is monotone across the whole
    /// chain, matching its strictly increasing entry offsets. Under
    /// that same monotonicity, a backward array without a match also
    /// ends the scan.
    ///
    /// Arrays only: a data object's inlined entry is out of scope
    /// here; `InlinedCursor::directed_partition_point` folds it in.
    pub fn directed_partition_point<M, F>(
        self,
        journal_file: &JournalFile<M>,
        predicate: F,
        direction: Direction,
    ) -> Result<Option<Cursor>>
    where
        M: MemoryMap,
        F: Fn(NonZeroU64) -> Result<bool>,
    {
        let mut last_cursor: Option<Cursor> = None;

        let mut node = self.head(journal_file)?;

        loop {
            let left = 0;
            let right = node.len().get();

            if let Some(index) =
                node.directed_partition_point(journal_file, left, right, &predicate, direction)?
            {
                let cursor = Cursor::at_position(
                    journal_file,
                    self,
                    node.offset,
                    index,
                    node.remaining_items,
                )?;

                match direction {
                    Direction::Forward => {
                        return Ok(Some(cursor));
                    }
                    Direction::Backward => {
                        // Backward: remember this match and keep
                        // going - a later array may hold a later one.
                        last_cursor = Some(cursor);

                        // A match on the array's last slot can still
                        // be beaten by the next array's first slots;
                        // anything earlier cannot, so stop here.
                        if index == node.len().get() - 1 && node.has_next() {
                            // Fall through to the next-array advance
                            // at the bottom of the loop.
                        } else {
                            return Ok(last_cursor);
                        }
                    }
                }
            } else if direction == Direction::Backward {
                // No true position in this array: under a monotone
                // predicate none can follow, so the scan is done.
                return Ok(last_cursor);
            }

            if let Some(nd) = node.next(journal_file)? {
                node = nd;
            } else {
                break;
            }
        }

        // For backward direction, return the last match we found (if any)
        if direction == Direction::Backward {
            return Ok(last_cursor);
        }

        // No match found in any array
        Ok(None)
    }

    /// Append every entry offset of the chain, head to tail, to
    /// `offsets`; capacity is reserved up front from `total_items`.
    pub fn collect_offsets<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        offsets.reserve(self.total_items.get());

        let mut node = self.head(journal_file)?;

        loop {
            {
                let array = journal_file.offset_array_ref(node.offset())?;
                let remaining_items = node.remaining_items.get();
                array.collect_offsets(0, remaining_items, offsets)?;
            }

            match node.next(journal_file)? {
                Some(next) => node = next,
                None => break,
            }
        }

        Ok(())
    }
}

/// A position in an offset-array chain: the chain (`list`), the array
/// (`array_offset`), the slot (`array_index`), and how many chain
/// entries remain from this array onward (`remaining_items`).
/// `Copy`; `JournalCursor` keeps one as the unfiltered stepping
/// engine's position (its `array_cursor` field).
#[derive(Clone, Copy)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Cursor {
    list: List,
    array_offset: NonZeroU64,
    array_index: usize,
    remaining_items: NonZeroUsize,
}

impl Cursor {
    pub fn head(&self) -> Self {
        Self::at_head(self.list)
    }

    /// Create a cursor at the head of the chain
    pub fn at_head(list: List) -> Self {
        Self {
            list,
            array_offset: list.head_offset,
            array_index: 0,
            remaining_items: list.total_items,
        }
    }

    /// A cursor at the chain's last entry; walks the chain to find
    /// the tail array.
    pub fn at_tail<M: MemoryMap>(journal_file: &JournalFile<M>, list: List) -> Result<Self> {
        let mut current_array = list.head(journal_file)?;

        while let Some(next_array) = current_array.next(journal_file)? {
            current_array = next_array;
        }

        Ok(Self {
            list,
            array_offset: current_array.offset,
            array_index: current_array.len().get() - 1,
            remaining_items: current_array.len(),
        })
    }

    /// A cursor at `array_index` of the array at `array_offset`,
    /// with the caller's `remaining_items`. Validates both: an
    /// unreadable or zero-capacity array is `Err`, an out-of-range
    /// index is `InvalidOffsetArrayIndex`. Used by
    /// `List::directed_partition_point` to pin a found position.
    pub fn at_position<M: MemoryMap>(
        journal_file: &JournalFile<M>,
        offset_array_list: List,
        array_offset: NonZeroU64,
        array_index: usize,
        remaining_items: NonZeroUsize,
    ) -> Result<Self> {
        debug_assert!(offset_array_list.total_items >= remaining_items);

        // Verify the array exists
        let array = Node::new(journal_file, array_offset, remaining_items)?;

        // Verify the index is valid
        if array_index >= array.len().get() {
            return Err(JournalError::InvalidOffsetArrayIndex);
        }

        Ok(Self {
            list: offset_array_list,
            array_offset,
            array_index,
            remaining_items,
        })
    }

    /// A fresh `Node` view of the array under the cursor; nothing is
    /// cached between calls.
    pub fn node<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        Node::new(journal_file, self.array_offset, self.remaining_items)
    }

    /// The entry offset at the cursor's position; `Ok(None)` for an
    /// unset slot.
    pub fn value<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<NonZeroU64>> {
        self.node(journal_file)?.get(journal_file, self.array_index)
    }

    /// The next chain position: the following slot, or the next
    /// array's first slot; `None` at the chain's end. Every call
    /// re-reads the current array through the file; crossing into
    /// the next array re-reads that one's header as well.
    pub fn next<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        let array_node = self.node(journal_file)?;

        // FIXME: overtly defensive/expensive...
        if self.array_index + 1 < array_node.len().get() {
            // Next item is in the same array
            return Ok(Some(Self {
                list: self.list,
                array_offset: self.array_offset,
                array_index: self.array_index + 1,
                remaining_items: self.remaining_items,
            }));
        }

        if !array_node.has_next() {
            return Ok(None);
        }

        let next_array = array_node.next(journal_file)?.unwrap();

        match NonZeroUsize::new(
            self.remaining_items
                .get()
                .saturating_sub(array_node.len().get()),
        ) {
            None => Ok(None),
            Some(remaining_items) => Ok(Some(Self {
                list: self.list,
                array_offset: next_array.offset,
                array_index: 0,
                remaining_items,
            })),
        }
    }

    /// The previous chain position: the preceding slot, the previous
    /// array's last slot, or `None` at the chain's head. Crossing
    /// arrays walks from the head to find the predecessor -
    /// O(chain length) per step, unlike `next` - and reports
    /// `InvalidOffsetArrayOffset` if the cursor's array is not
    /// linked from this chain's head.
    pub fn previous<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        if self.array_index > 0 {
            // Previous item is in the same array
            return Ok(Some(Self {
                list: self.list,
                array_offset: self.array_offset,
                array_index: self.array_index - 1,
                remaining_items: self.remaining_items,
            }));
        }

        if self.array_offset == self.list.head_offset {
            return Ok(None);
        }

        let mut node = self.list.head(journal_file)?;
        while node.has_next() {
            if node.next_offset == Some(self.array_offset) {
                return Ok(Some(Self {
                    list: self.list,
                    array_offset: node.offset,
                    array_index: node.len().get() - 1,
                    remaining_items: node.remaining_items,
                }));
            }

            node = node.next(journal_file)?.unwrap();
        }

        Err(JournalError::InvalidOffsetArrayOffset)
    }

    /// Append the entry offsets from the cursor's position to the
    /// chain's end. For each later array the collect range is
    /// derived from the previous node's remaining count rather than
    /// the next node's own - wider than needed at the chain's tail,
    /// but correct because unwritten slots read as `None` and are
    /// skipped (`OffsetArrayObject::collect_offsets`).
    pub fn collect_offsets<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        let mut node = self.node(journal_file)?;

        // Copy from position in the current array
        {
            let array = journal_file.offset_array_ref(node.offset())?;
            let remaining_items = node.remaining_items.get();
            array.collect_offsets(self.array_index, remaining_items, offsets)?;
        }

        // Copy from subsequent arrays
        while let Some(next_node) = node.next(journal_file)? {
            let array = journal_file.offset_array_ref(next_node.offset())?;
            let remaining_items = node.remaining_items.get();
            array.collect_offsets(0, remaining_items, offsets)?;
            node = next_node;
        }

        Ok(())
    }
}

impl std::fmt::Debug for Cursor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Cursor")
            .field("array_offset", &format!("0x{:x}", self.array_offset))
            .field("array_index", &self.array_index)
            .field("remaining_items", &self.remaining_items)
            .finish()
    }
}

/// A data object's entry chain as a step cursor: the inlined first
/// entry (`DataObjectHeader::entry_offset`) plus, when the object
/// holds more than one entry, a [`Cursor`] over the linked array
/// with the rest; `DataObjectHeader::inlined_cursor` assembles the
/// pair from the header's links. `at_inlined_offset` says which
/// side it is parked on; `value` switches accordingly.
///
/// `Copy`, so callers can detach it from a borrowed data object and
/// step it after the borrow is dropped - journal-index does
/// exactly that
/// (src/crates/journal-index/src/file_indexer.rs).
#[derive(Debug, Copy, Clone)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct InlinedCursor {
    inlined_offset: NonZeroU64,
    cursor: Option<Cursor>,
    at_inlined_offset: bool,
}

impl InlinedCursor {
    /// Parked on the inlined entry, with the optional array chain
    /// for the remaining entries.
    pub fn new(inlined_offset: NonZeroU64, cursor: Option<Cursor>) -> Self {
        Self {
            inlined_offset,
            cursor,
            at_inlined_offset: true,
        }
    }

    /// Rewind to the inlined entry, resetting the array cursor to
    /// its chain's head.
    pub fn head(&self) -> Self {
        Self {
            inlined_offset: self.inlined_offset,
            cursor: self.cursor.as_ref().map(|c| c.head()),
            at_inlined_offset: true,
        }
    }

    /// Park on the chain's last entry (walks the chain); a
    /// single-entry object - no array cursor - stays on the inlined
    /// entry.
    pub fn tail<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Self> {
        // Start with a copy of the current cursor
        let mut result = *self;

        // If we have an entry array list cursor, move it to the tail
        if let Some(cursor) = self.cursor {
            result.cursor = Some(cursor.list.cursor_tail(journal_file)?);
            result.at_inlined_offset = false;
        }

        Ok(result)
    }

    /// The next entry: inlined -> array start, then slot by slot;
    /// `None` at the end.
    fn next<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        // Case 1: We're at the inlined entry, move to the first array entry
        if self.at_inlined_offset {
            if self.cursor.is_some() {
                return Ok(Some(Self {
                    inlined_offset: self.inlined_offset,
                    cursor: self.cursor,
                    at_inlined_offset: false,
                }));
            } else {
                return Ok(None);
            }
        }

        // Case 2: We're already in the entry array
        if let Some(current_cursor) = self.cursor.as_ref() {
            let next_cursor = current_cursor.next(journal_file)?;

            if next_cursor.is_some() {
                return Ok(Some(Self {
                    inlined_offset: self.inlined_offset,
                    cursor: next_cursor,
                    at_inlined_offset: false,
                }));
            } else {
                return Ok(None);
            }
        }

        // No more entries
        Ok(None)
    }

    /// The previous entry: slot by slot down to the array start,
    /// then the inlined entry; `None` before that.
    fn previous<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        if self.at_inlined_offset {
            return Ok(None);
        }

        if let Some(current_cursor) = self.cursor {
            // Try to move to the previous position in the array
            if let Some(prev_cursor) = current_cursor.previous(journal_file)? {
                // We can move back within the array
                let mut ic = *self;
                ic.cursor = Some(prev_cursor);
                return Ok(Some(ic));
            } else {
                // We're at the first array position, move to the inlined entry
                let mut ic = *self;
                ic.at_inlined_offset = true;
                return Ok(Some(ic));
            }
        }

        // Not at the inlined entry and no array cursor: impossible
        // by construction.
        unreachable!();
    }

    /// The entry offset under the cursor: the inlined entry, or the
    /// array slot.
    pub fn value<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<NonZeroU64>> {
        // Case 1: We're at the inlined entry
        if self.at_inlined_offset {
            return Ok(Some(self.inlined_offset));
        }

        // Case 2: We're in the entry array
        if let Some(cursor) = self.cursor {
            return cursor.value(journal_file);
        }

        unreachable!();
    }

    /// Advance until an entry offset at or after `offset` sits under
    /// the cursor and return it; `None` when the chain runs out
    /// first.
    ///
    /// The scan continues from the cursor's current position and
    /// clamps to the needle: the first position at/after it, which
    /// may be the position already held, returned without moving.
    /// Callers resolving a location from scratch rewind with
    /// `head()` first - the contract
    /// [`FilterExpr::head`](crate::file::filter::FilterExpr::head) +
    /// `next` live by and the cursor's filtered
    /// path (`JournalCursor::resolve_filter_location`) obeys. Entries are
    /// visited in ascending offset order.
    pub fn next_until<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        offset: NonZeroU64,
    ) -> Result<Option<NonZeroU64>> {
        let Some(current_offset) = self.value(journal_file)? else {
            return Ok(None);
        };

        if current_offset >= offset {
            return Ok(Some(current_offset));
        }

        while let Some(ic) = self.next(journal_file)? {
            *self = ic;

            let Some(current_offset) = self.value(journal_file)? else {
                break;
            };

            if current_offset >= offset {
                return Ok(Some(current_offset));
            }
        }

        Ok(None)
    }

    /// Retreat until an entry offset at or before `offset` sits
    /// under the cursor and return it; `None` when the chain is
    /// exhausted first.
    ///
    /// Mirror of `next_until`: the scan continues from the current
    /// position and clamps to the needle, a position already at or
    /// below it being returned without moving, so fresh resolutions
    /// rewind with `tail()` first
    /// ([`FilterExpr::tail`](crate::file::filter::FilterExpr::tail)). Entries
    /// are visited in descending offset order.
    pub fn previous_until<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        offset: NonZeroU64,
    ) -> Result<Option<NonZeroU64>> {
        let Some(current_offset) = self.value(journal_file)? else {
            return Ok(None);
        };

        if current_offset <= offset {
            return Ok(Some(current_offset));
        }

        while let Some(ic) = self.previous(journal_file)? {
            *self = ic;

            let Some(current_offset) = ic.value(journal_file)? else {
                break;
            };

            if current_offset <= offset {
                return Ok(Some(current_offset));
            }
        }

        Ok(None)
    }

    /// The best matching position across the inlined entry and the
    /// array chain: forward, the first entry whose `predicate` is
    /// false; backward, the last entry whose `predicate` is true.
    /// `None` when no such entry exists. `self` is not moved; the
    /// returned cursor is parked on the match.
    ///
    /// Forward short-circuits: a false predicate on the inlined
    /// entry is itself the first match, so the arrays are not
    /// searched. Backward keeps the inlined entry as a candidate
    /// and still searches the arrays, keeping the match at the
    /// higher offset.
    pub fn directed_partition_point<M, F>(
        &self,
        journal_file: &JournalFile<M>,
        predicate: F,
        direction: Direction,
    ) -> Result<Option<Self>>
    where
        M: MemoryMap,
        F: Fn(NonZeroU64) -> Result<bool>,
    {
        // Variables to track our best match
        let mut best_match: Option<Self> = None;

        // Handle the inlined entry based on direction
        match direction {
            Direction::Forward => {
                if !predicate(self.inlined_offset)? {
                    return Ok(Some(self.head()));
                }
            }
            Direction::Backward => {
                if predicate(self.inlined_offset)? {
                    // If predicate is true for inlined entry and we're going backward,
                    // this is potentially our best match
                    best_match = Some(self.head());
                }
            }
        }

        // If we have an array cursor, check it too using binary search
        if let Some(cursor) = self.cursor {
            let ic = cursor
                .list
                .directed_partition_point(journal_file, predicate, direction)?;

            if let Some(ic) = ic {
                // Create a new InlinedCursor with this array cursor
                let array_match = Self {
                    inlined_offset: self.inlined_offset,
                    cursor: Some(ic),
                    at_inlined_offset: false,
                };

                // Compare with our current best match
                if best_match.is_none() {
                    best_match = Some(array_match);
                } else {
                    // Only Backward gets here with a candidate in
                    // hand (forward sets the array match directly or
                    // short-circuited): keep the higher offset.
                    let best_offset = best_match.as_ref().unwrap().value(journal_file)?;
                    let array_offset = array_match.value(journal_file)?;

                    match direction {
                        Direction::Forward => {
                            if array_offset < best_offset {
                                best_match = Some(array_match);
                            }
                        }
                        Direction::Backward => {
                            if array_offset > best_offset {
                                best_match = Some(array_match);
                            }
                        }
                    }
                }
            }
        }

        Ok(best_match)
    }

    /// Append entry offsets to `offsets`: parked on the inlined
    /// entry, it comes first and then the whole chain from its
    /// head; parked in the array, only the chain from the current
    /// position on. A cursor-less single-entry object contributes
    /// just the inlined offset.
    pub fn collect_offsets<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        // Handle the inline offset first if we're at it
        if self.at_inlined_offset {
            offsets.push(self.inlined_offset);

            // If we have a cursor, collect all offsets from the beginning
            if let Some(cursor) = self.cursor {
                cursor.list.collect_offsets(journal_file, offsets)?;
            }
        } else if let Some(cursor) = self.cursor {
            // We're somewhere in the array chain, collect from current position
            cursor.collect_offsets(journal_file, offsets)?;
        }

        Ok(())
    }
}
