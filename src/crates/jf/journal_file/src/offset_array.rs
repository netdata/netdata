use crate::file::JournalFile;
use error::{JournalError, Result};
use std::num::{NonZeroU64, NonZeroUsize};
use window_manager::MemoryMap;

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
    /// Create a new offset array reference
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

    /// Get the number of items available in this array
    pub fn len(&self) -> NonZeroUsize {
        self.capacity.min(self.remaining_items)
    }

    /// Check if this array has a next array in the chain
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

    /// Get an item at the specified index
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

    /// Returns the first index where the predicate returns false, or array length if
    /// the predicate is true for all elements
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

    /// Find the forward or backward (depending on direction) position that matches the predicate.
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

/// A linked list of offset arrays
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

    /// Get the tail array of this list by traversing from head to tail
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

    /// Finds the first/last array item position where the predicate function becomes false
    /// in a chain of offset arrays.
    ///
    /// # Parameters
    /// * `predicate` - Function that takes an array item value and returns true if the search should continue.
    /// * `direction` - Direction of the search (Forward or Backward)
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
                        // In backward direction, save this match and continue
                        // to ensure we'll find the last match
                        last_cursor = Some(cursor);

                        // If this match is at the end of the array and there's a next array,
                        // we should check the next array as well
                        if index == node.len().get() - 1 && node.has_next() {
                            // continue;
                        } else {
                            return Ok(last_cursor);
                        }
                    }
                }
            } else if direction == Direction::Backward {
                // No match in this array for backward direction
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

    /// Create a cursor at a specific position
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

    /// Get the current array this cursor points to
    pub fn node<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Node> {
        Node::new(journal_file, self.array_offset, self.remaining_items)
    }

    pub fn value<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<Option<NonZeroU64>> {
        self.node(journal_file)?.get(journal_file, self.array_index)
    }

    /// Move to the next position
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

    /// Move to the previous position
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

#[derive(Debug, Copy, Clone)]
pub struct InlinedCursor {
    inlined_offset: NonZeroU64,
    cursor: Option<Cursor>,
    at_inlined_offset: bool,
}

impl InlinedCursor {
    pub fn new(inlined_offset: NonZeroU64, cursor: Option<Cursor>) -> Self {
        Self {
            inlined_offset,
            cursor,
            at_inlined_offset: true,
        }
    }

    pub fn head(&self) -> Self {
        Self {
            inlined_offset: self.inlined_offset,
            cursor: self.cursor.as_ref().map(|c| c.head()),
            at_inlined_offset: true,
        }
    }

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

        unreachable!();
    }

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
                    // Choose the better match based on direction
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
