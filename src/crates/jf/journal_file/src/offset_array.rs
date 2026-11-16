use crate::file::JournalFile;
use error::{JournalError, Result};
use std::num::{NonZeroU64, NonZeroUsize};
use window_manager::MemoryMap;

/// Search direction for partition-point queries: `Forward` selects the first
/// match, `Backward` the last.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Direction {
    Forward,
    Backward,
}

/// A reference to a single array of offsets in the journal file
pub struct Node {
    offset: NonZeroU64,
    next_offset: Option<NonZeroU64>,
    capacity: NonZeroUsize,
    // Number of items remaining in this array and subsequent arrays
    remaining_items: NonZeroUsize,
}

impl Node {
    /// Create a new offset array reference; fails if the array is empty
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

    /// Get the number of valid items in this array (its capacity, capped by the
    /// items remaining in the chain)
    pub fn len(&self) -> NonZeroUsize {
        self.capacity.min(self.remaining_items)
    }

    /// Check whether the chain continues after this array: a next array exists
    /// and items remain beyond this array
    pub fn has_next(&self) -> bool {
        self.next_offset.is_some() && self.remaining_items > self.len()
    }

    /// Get the next array in the chain, if any
    pub fn next<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        if !self.has_next() {
            return Ok(None);
        }

        let next_offset = self.next_offset.unwrap();
        let remaining_items = {
            let n = self.remaining_items.get().saturating_sub(self.len().get());
            NonZeroUsize::new(n).ok_or(JournalError::EmptyOffsetArrayNode)?
        };
        let node = Self::new(journal_file, next_offset, remaining_items);

        Some(node).transpose()
    }

    /// Get the item at `index` (error when out of range, `None` for an empty slot)
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

    /// Binary search `[left, right)` for the first index whose item fails the
    /// predicate (`right` when all items in range pass it). The items in range
    /// must be partitioned: the predicate is true up to some point, false after.
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

    /// Find the partition boundary in the given direction: `Forward` returns the
    /// first index whose item fails the predicate, `Backward` the last index whose
    /// item satisfies it; `None` when there is no such index.
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

/// A chain of offset arrays, identified by the offset of its head array and the
/// total number of items across all arrays in the chain
#[derive(Copy, Clone)]
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

    /// Get the head array of this chain
    pub fn head<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        Node::new(journal_file, self.head_offset, self.total_items)
    }

    /// Traverse the chain and return its last array
    pub fn tail<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        let mut current = self.head(journal_file)?;

        while let Some(next) = current.next(journal_file)? {
            current = next;
        }

        Ok(current)
    }

    /// Get a cursor at the first position in the chain
    pub fn cursor_head(self) -> Cursor {
        Cursor::at_head(self)
    }

    /// Get a cursor at the last position in the chain
    pub fn cursor_tail<M: MemoryMap>(self, journal_file: &JournalFile<M>) -> Result<Cursor> {
        Cursor::at_tail(journal_file, self)
    }

    /// Search the whole chain for the first item that fails the predicate
    /// (`Forward`) or the last item that satisfies it (`Backward`), returning a
    /// cursor to it; `None` when there is no such item. The predicate returns
    /// `true` for an item while the search should continue past it.
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
                        // Remember this match and keep searching: a later array may hold a later one
                        last_cursor = Some(cursor);

                        // The match is this array's last item: keep checking the next array
                        if index == node.len().get() - 1 && node.has_next() {
                        } else {
                            return Ok(last_cursor);
                        }
                    }
                }
            } else if direction == Direction::Backward {
                // No match in this array: none can follow, so the last match is final
                return Ok(last_cursor);
            }

            if let Some(nd) = node.next(journal_file)? {
                node = nd;
            } else {
                break;
            }
        }

        if direction == Direction::Backward {
            return Ok(last_cursor);
        }

        // No match in any array (forward direction)
        Ok(None)
    }
}

/// A cursor pointing to a specific position within an offset array chain
#[derive(Clone, Copy)]
pub struct Cursor {
    list: List,
    array_offset: NonZeroU64,
    array_index: usize,
    remaining_items: NonZeroUsize,
}

impl Cursor {
    /// Return a fresh cursor at the head of the chain
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

    /// Create a cursor at the tail of the chain
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

    /// Create a cursor at a specific position, validating that the array exists
    /// and `array_index` is in bounds
    pub fn at_position<M: MemoryMap>(
        journal_file: &JournalFile<M>,
        offset_array_list: List,
        array_offset: NonZeroU64,
        array_index: usize,
        remaining_items: NonZeroUsize,
    ) -> Result<Self> {
        debug_assert!(offset_array_list.total_items >= remaining_items);

        let array = Node::new(journal_file, array_offset, remaining_items)?;

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

    /// Get the current array this cursor points to
    pub fn node<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        Node::new(journal_file, self.array_offset, self.remaining_items)
    }

    /// Get the item at this cursor's position
    pub fn value<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<NonZeroU64>> {
        self.node(journal_file)?.get(journal_file, self.array_index)
    }

    /// Return the cursor at the next position, or `None` at the end of the chain
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

    /// Return the cursor at the previous position, or `None` before the chain
    /// head. Fails when the cursor's array is not reachable from the head
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

/// Iterates the entries referencing a data object: the object's inlined entry
/// offset first, then the remaining offsets of its entry-array chain through the
/// wrapped [`Cursor`] (absent when the object is referenced by a single entry)
#[derive(Debug, Copy, Clone)]
pub struct InlinedCursor {
    inlined_offset: NonZeroU64,
    cursor: Option<Cursor>,
    at_inlined_offset: bool,
}

impl InlinedCursor {
    /// Create a cursor over an inlined entry and, optionally, its entry-array chain
    pub fn new(inlined_offset: NonZeroU64, cursor: Option<Cursor>) -> Self {
        Self {
            inlined_offset,
            cursor,
            at_inlined_offset: true,
        }
    }

    /// Return a copy rewound to the chain head, pointing at the inlined entry
    pub fn head(&self) -> Self {
        Self {
            inlined_offset: self.inlined_offset,
            cursor: self.cursor.as_ref().map(|c| c.head()),
            at_inlined_offset: true,
        }
    }

    /// Copy this cursor with the wrapped chain cursor moved to the chain tail
    pub fn tail<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Self> {
        let mut result = *self;

        if let Some(cursor) = self.cursor {
            result.cursor = Some(cursor.list.cursor_tail(journal_file)?);
            result.at_inlined_offset = false;
        }

        Ok(result)
    }

    fn next<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        // Case 1: at the inlined entry; step to the wrapped cursor's chain position
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

    fn previous<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<Self>> {
        if self.at_inlined_offset {
            return Ok(None);
        }

        if let Some(current_cursor) = self.cursor {
            // Try to move to the previous position in the array
            if let Some(prev_cursor) = current_cursor.previous(journal_file)? {
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

        unreachable!();
    }

    /// Get the current value: the inlined offset, or the wrapped cursor's chain item
    pub fn value<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<NonZeroU64>> {
        if self.at_inlined_offset {
            return Ok(Some(self.inlined_offset));
        }

        if let Some(cursor) = self.cursor {
            return cursor.value(journal_file);
        }

        unreachable!();
    }

    /// Step forward until the current value is at or after `offset` and return
    /// it; `None` when the iteration ends first
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

    /// Step backward until the current value is at or before `offset` and return
    /// it; `None` when the iteration ends first
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

    /// Run [`List::directed_partition_point`] over the inlined entry and the
    /// entry-array chain, returning the direction's best match.
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
        let mut best_match: Option<Self> = None;

        match direction {
            Direction::Forward => {
                if !predicate(self.inlined_offset)? {
                    return Ok(Some(self.head()));
                }
            }
            Direction::Backward => {
                if predicate(self.inlined_offset)? {
                    // A satisfying inlined entry is only a candidate; the chain may hold a better one
                    best_match = Some(self.head());
                }
            }
        }

        // Search the chain as well, when there is one
        if let Some(cursor) = self.cursor {
            let ic = cursor
                .list
                .directed_partition_point(journal_file, predicate, direction)?;

            if let Some(ic) = ic {
                let array_match = Self {
                    inlined_offset: self.inlined_offset,
                    cursor: Some(ic),
                    at_inlined_offset: false,
                };

                if best_match.is_none() {
                    best_match = Some(array_match);
                } else {
                    // Keep the earliest (Forward) or latest (Backward) of the two candidates
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
}
