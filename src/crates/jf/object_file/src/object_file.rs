use crate::hash;
use crate::object::*;
use crate::offset_array;
use error::{JournalError, Result};
// use std::backtrace::Backtrace;
use std::cell::{RefCell, UnsafeCell};
use std::fs::{File, OpenOptions};
use std::path::Path;
use window_manager::{MemoryMap, WindowManager, WindowManagerStatistics};
use zerocopy::FromBytes;

use crate::value_guard::ValueGuard;

// Size to pad objects to (8 bytes)
const OBJECT_ALIGNMENT: u64 = 8;

/// A reader for systemd journal files that efficiently maps small regions of the file into memory.
///
/// # Memory Management
///
/// This implementation uses a window-based memory mapping strategy similar to systemd's original
/// implementation. Instead of mapping the entire file, it maintains a small set of memory-mapped
/// windows and reuses them as needed.
///
/// # Concurrency and Safety
///
/// `ObjectFile` uses interior mutability to provide a safe API with the following characteristics:
///
/// - The window manager is wrapped in an `UnsafeCell` to allow mutation through a shared reference.
/// - A single `RefCell<bool>` guards access to ensure only one object can be active at a time.
/// - Methods like `data_object()` return a `ValueGuard<T>` that automatically releases the lock
///   when dropped.
///
/// This design ensures that memory safety is maintained even though references to memory-mapped
/// regions could be invalidated when new objects are created.
pub struct ObjectFile<M: MemoryMap> {
    file: File,
    file_size: u64,

    // Persistent memory maps for journal header and data/field hash tables
    header_map: M,
    data_hash_table_map: Option<M>,
    field_hash_table_map: Option<M>,

    // Window manager for other objects
    window_manager: UnsafeCell<WindowManager<M>>,

    // prev_backtrace: std::cell::RefCell<std::backtrace::Backtrace>,
    // backtrace: std::cell::RefCell<std::backtrace::Backtrace>,

    // Flag to track if any object is in use
    object_in_use: RefCell<bool>,
}

impl<M: MemoryMap> ObjectFile<M> {
    pub fn open(path: impl AsRef<Path>, window_size: u64) -> Result<Self> {
        debug_assert_eq!(window_size % OBJECT_ALIGNMENT, 0);

        // Open file and check its size
        let file = OpenOptions::new().read(true).write(false).open(&path)?;
        let file_size = file.metadata()?.len();

        // Create a memory map for the header
        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let header_map = M::create(&file, 0, header_size)?;
        let header = JournalHeader::ref_from_prefix(&header_map).unwrap().0;
        if header.signature != *b"LPKSHHRH" {
            return Err(JournalError::InvalidMagicNumber);
        }

        // Initialize the hash table maps if they exist
        let data_hash_table_map = header.map_data_hash_table(&file)?;
        let field_hash_table_map = header.map_field_hash_table(&file)?;

        // Create window manager for the rest of the objects
        let window_manager = UnsafeCell::new(WindowManager::new(window_size, 32));

        Ok(ObjectFile {
            file,
            file_size,
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
            // prev_backtrace: RefCell::new(std::backtrace::Backtrace::capture()),
            // backtrace: RefCell::new(std::backtrace::Backtrace::capture()),
            object_in_use: RefCell::new(false),
        })
    }

    pub fn entry_list(&self) -> Result<offset_array::List<'_, M>> {
        offset_array::List::new(
            self,
            self.journal_header().entry_array_offset,
            self.journal_header().n_entries as usize,
        )
    }

    pub fn journal_header(&self) -> &JournalHeader {
        JournalHeader::ref_from_prefix(&self.header_map).unwrap().0
    }

    pub fn data_hash_table(&self) -> Option<HashTableObject<&[u8]>> {
        self.data_hash_table_map
            .as_ref()
            .map(|m| HashTableObject::<&[u8]>::from_data(m, false))
    }

    pub fn field_hash_table(&self) -> Option<HashTableObject<&[u8]>> {
        self.field_hash_table_map
            .as_ref()
            .map(|m| HashTableObject::<&[u8]>::from_data(m, false))
    }

    fn read_object_header(&self, position: u64) -> Result<&ObjectHeader> {
        debug_assert_ne!(position, 0);
        debug_assert_eq!(position % OBJECT_ALIGNMENT, 0);

        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;

        let window_manager = unsafe { &mut *self.window_manager.get() };
        let window = window_manager.get_window(&self.file, position, size_needed)?;

        let header_slice = window.get_slice(position, size_needed);
        Ok(ObjectHeader::ref_from_bytes(header_slice).unwrap())
    }

    fn read_object_data(&self, position: u64, size_needed: u64) -> Result<&[u8]> {
        debug_assert!(position < self.file_size);

        let window_manager = unsafe { &mut *self.window_manager.get() };
        let window = window_manager.get_window(&self.file, position, size_needed)?;

        let window_slice = window.get_slice(position, size_needed);
        Ok(window_slice)
    }

    fn read_journal_object<'a, T>(&'a self, position: u64) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObject<&'a [u8]>,
    {
        // Check if any object is already in use
        let mut is_in_use = self.object_in_use.borrow_mut();
        if *is_in_use {
            // eprintln!(
            //     "Value is in use. Current Backtrace: {:?}, Previous Backtrace: {:?}",
            //     self.backtrace.borrow().to_string(),
            //     self.prev_backtrace.borrow().to_string()
            // );
            return Err(JournalError::ValueGuardInUse);
        }

        // self.backtrace.swap(&self.prev_backtrace);
        // let _ = self.backtrace.replace(Backtrace::force_capture());

        let is_compact = self
            .journal_header()
            .has_incompatible_flag(HeaderIncompatibleFlags::Compact);

        let size_needed = {
            let header = self.read_object_header(position)?;
            header.size
        };

        let data = self.read_object_data(position, size_needed)?;
        let object = T::from_data(data, is_compact);

        // Mark as in use
        *is_in_use = true;

        Ok(ValueGuard::new(object, &self.object_in_use))
    }

    pub fn offset_array_object(
        &self,
        position: u64,
    ) -> Result<ValueGuard<OffsetArrayObject<&[u8]>>> {
        self.read_journal_object(position)
    }

    pub fn field_object(&self, position: u64) -> Result<ValueGuard<FieldObject<&[u8]>>> {
        self.read_journal_object(position)
    }

    pub fn entry_object(&self, position: u64) -> Result<ValueGuard<EntryObject<&[u8]>>> {
        self.read_journal_object(position)
    }

    pub fn data_object(&self, position: u64) -> Result<ValueGuard<DataObject<&[u8]>>> {
        self.read_journal_object(position)
    }

    pub fn tag_object(&self, position: u64) -> Result<ValueGuard<TagObject<&[u8]>>> {
        self.read_journal_object(position)
    }

    fn lookup_hash_table<'a, T, F>(
        &'a self,
        hash_table: Option<HashTableObject<&[u8]>>,
        data: &[u8],
        fetch_fn: F,
    ) -> Result<u64>
    where
        T: HashableObject,
        F: Fn(u64) -> Result<ValueGuard<'a, T>>,
    {
        let hash_table = hash_table.ok_or(JournalError::MissingHashTable)?;

        // Calculate hash using the appropriate algorithm
        let is_keyed_hash = self
            .journal_header()
            .has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash);

        let payload_hash = hash::journal_hash_data(
            data,
            is_keyed_hash,
            if is_keyed_hash {
                Some(&self.journal_header().file_id)
            } else {
                None
            },
        );

        // Find the right bucket in the hash table
        let hash_table_size = hash_table.items.len();
        let bucket_idx = (payload_hash % hash_table_size as u64) as usize;

        // Get the head object offset from the bucket
        let bucket = hash_table.items[bucket_idx];
        let mut object_offset = bucket.head_hash_offset;

        // Traverse the linked list of objects in this bucket
        while object_offset != 0 {
            match fetch_fn(object_offset) {
                Ok(object_guard) => {
                    // Check if this is the object we're looking for
                    if object_guard.hash() == payload_hash && object_guard.get_payload() == data {
                        return Ok(object_offset);
                    }

                    // Move to the next object in the chain
                    object_offset = object_guard.next_hash_offset();
                }
                Err(e) => {
                    return Err(e);
                }
            }
        }

        Err(JournalError::MissingObjectFromHashTable)
    }

    /// Finds a field object by name and returns its offset
    pub fn find_field_offset_by_name(&self, field_name: &[u8]) -> Result<u64> {
        self.lookup_hash_table::<FieldObject<&[u8]>, _>(
            self.field_hash_table(),
            field_name,
            |offset| self.field_object(offset),
        )
    }

    /// Finds a data object by payload and returns its offset
    pub fn find_data_offset_by_payload(&self, payload: &[u8]) -> Result<u64> {
        self.lookup_hash_table::<DataObject<&[u8]>, _>(self.data_hash_table(), payload, |offset| {
            self.data_object(offset)
        })
    }

    /// Run a directed partition point query on a data object's entry array
    ///
    /// This finds the first/last entry (depending on direction) that satisfies the given predicate
    /// in the entry array chain of the data object.
    pub fn data_object_directed_partition_point<F>(
        &self,
        data_offset: u64,
        predicate: F,
        direction: offset_array::Direction,
    ) -> Result<Option<u64>>
    where
        F: Fn(u64) -> Result<bool>,
    {
        let (inline_entry_offset, entry_array_offset, n_entries) = {
            let data_object = self.data_object(data_offset)?;

            (
                data_object.header.entry_offset,
                data_object.header.entry_array_offset,
                data_object.header.n_entries,
            )
        };

        if n_entries == 0 {
            return Ok(None);
        }

        let mut best_match: Option<u64> = None;

        if entry_array_offset != 0 {
            let list = offset_array::List::new(self, entry_array_offset, n_entries as usize - 1)?;

            if let Some(cursor) = list.directed_partition_point(&predicate, direction)? {
                best_match = Some(cursor.position()?);
            }
        }

        match direction {
            offset_array::Direction::Forward => {
                if !predicate(inline_entry_offset)? {
                    best_match = Some(inline_entry_offset);
                }
            }
            offset_array::Direction::Backward => {
                if best_match.is_none() && predicate(inline_entry_offset)? {
                    best_match = Some(inline_entry_offset);
                }
            }
        }

        Ok(best_match)
    }

    /// Creates an iterator over all offsets of an offset array list
    pub fn array_offsets(&self, position: u64) -> Result<OffsetArrayListIterator<'_, M>> {
        Ok(OffsetArrayListIterator {
            journal: self,
            offset: position,
            capacity: if position == 0 {
                0
            } else {
                self.offset_array_object(position)?.capacity()
            },
            index: 0,
        })
    }

    /// Creates an iterator over all entry offsets in the journal
    pub fn entry_offsets(&self) -> Result<OffsetArrayListIterator<'_, M>> {
        self.array_offsets(self.journal_header().entry_array_offset)
    }

    /// Creates an iterator over all field objects in the field hash table
    pub fn fields(&self) -> FieldIterator<'_, M> {
        // Get the field hash table
        let field_hash_table = self.field_hash_table();

        // Initialize with the first bucket
        let mut iterator = FieldIterator {
            journal: self,
            field_hash_table,
            current_bucket_index: 0,
            next_field_offset: 0,
        };

        // Find the first non-empty bucket
        iterator.advance_to_next_nonempty_bucket();

        iterator
    }

    /// Creates an iterator over all DATA objects for the specified field
    pub fn field_data_objects<'a>(
        &'a self,
        field_name: &'a [u8],
    ) -> Result<FieldDataIterator<'a, M>> {
        // Find the field offset by name
        let field_offset = self.find_field_offset_by_name(field_name)?;

        // Get the field object to access its head_data_offset
        let field_guard = self.field_object(field_offset)?;
        let head_data_offset = field_guard.header.head_data_offset;

        // Create the iterator
        Ok(FieldDataIterator {
            journal: self,
            current_data_offset: head_data_offset,
        })
    }

    /// Creates an iterator over all DATA objects for a specific entry
    pub fn entry_data_objects(&self, entry_offset: u64) -> Result<EntryDataIterator<'_, M>> {
        // Get the entry object to determine how many data items it has
        let entry_guard = self.entry_object(entry_offset)?;

        // Get the total number of items
        let total_items = match &entry_guard.items {
            EntryItemsType::Regular(items) => items.len(),
            EntryItemsType::Compact(items) => items.len(),
        };

        // Create the iterator
        Ok(EntryDataIterator {
            journal: self,
            entry_offset,
            current_index: 0,
            total_items,
        })
    }

    pub fn stats(&self) -> WindowManagerStatistics {
        let window_manager = unsafe { &mut *self.window_manager.get() };
        window_manager.stats()
    }
}

/*
 * Offset array iteration
*/

/// Iterator that returns all offsets in an offset array list
pub struct OffsetArrayListIterator<'a, M: MemoryMap> {
    journal: &'a ObjectFile<M>,
    offset: u64,
    capacity: usize,
    index: usize,
}

impl<M: MemoryMap> Iterator for OffsetArrayListIterator<'_, M> {
    type Item = Result<u64>;

    fn next(&mut self) -> Option<Self::Item> {
        // If we've reached the end of offsets in the offset array list, return None
        if self.offset == 0 {
            return None;
        }

        // Check if we need to move to the next offset array
        if self.index >= self.capacity {
            // Get the next offset array offset
            let next_offset = match self.journal.offset_array_object(self.offset) {
                Ok(array_guard) => array_guard.header.next_offset_array,
                Err(e) => return Some(Err(e)),
            };

            // If there's no next offset array, we're done
            if next_offset == 0 {
                self.offset = 0;
                return None;
            }

            // Set up the next offset array
            match self.journal.offset_array_object(next_offset) {
                Ok(array_guard) => {
                    self.offset = next_offset;
                    self.capacity = array_guard.capacity();
                    self.index = 0;
                }
                Err(e) => return Some(Err(e)),
            }
        }

        // Get the current offset from the offset array
        let offset = match self.journal.offset_array_object(self.offset) {
            Ok(array_guard) => match &array_guard.items {
                OffsetsType::Regular(offsets) => {
                    if self.index < offsets.len() {
                        offsets[self.index]
                    } else {
                        0
                    }
                }
                OffsetsType::Compact(offsets) => {
                    if self.index < offsets.len() {
                        offsets[self.index] as u64
                    } else {
                        0
                    }
                }
            },
            Err(e) => return Some(Err(e)),
        };

        // Increment index for next iteration
        self.index += 1;

        // If offset is zero, we've reached the end of all entries
        if offset == 0 {
            self.offset = 0;
            return None;
        }

        Some(Ok(offset))
    }
}

/// Iterator that walks through all field objects in the field hash table
pub struct FieldIterator<'a, M: MemoryMap> {
    journal: &'a ObjectFile<M>,
    field_hash_table: Option<HashTableObject<&'a [u8]>>,
    current_bucket_index: usize,
    next_field_offset: u64,
}

impl<M: MemoryMap> FieldIterator<'_, M> {
    /// Advances to the next non-empty bucket
    fn advance_to_next_nonempty_bucket(&mut self) {
        // If we don't have a hash table, there's nothing to iterate
        let Some(hash_table) = &self.field_hash_table else {
            return;
        };

        let items = &hash_table.items;
        let num_buckets = items.len();

        // Find the next non-empty bucket
        while self.current_bucket_index < num_buckets {
            let bucket = items[self.current_bucket_index];
            if bucket.head_hash_offset != 0 {
                // Found a non-empty bucket
                self.next_field_offset = bucket.head_hash_offset;
                return;
            }
            self.current_bucket_index += 1;
        }

        // No more non-empty buckets
        self.next_field_offset = 0;
    }
}

impl<'a, M: MemoryMap> Iterator for FieldIterator<'a, M> {
    type Item = Result<ValueGuard<'a, FieldObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        // If we've reached the end, return None
        if self.next_field_offset == 0 {
            return None;
        }

        // Save the current offset to return
        let offset = self.next_field_offset;

        // Try to get the field object
        match self.journal.field_object(offset) {
            Ok(field_guard) => {
                // Get the next field offset before we return the guard
                self.next_field_offset = field_guard.header.next_hash_offset;

                // If we've reached the end of the chain, move to the next bucket
                if self.next_field_offset == 0 {
                    self.current_bucket_index += 1;
                    self.advance_to_next_nonempty_bucket();
                }

                Some(Ok(field_guard))
            }
            Err(e) => {
                // If we can't read the field, return the error and stop iteration
                self.next_field_offset = 0;
                Some(Err(e))
            }
        }
    }
}

/// Iterator that walks through all DATA objects for a specific field
pub struct FieldDataIterator<'a, M: MemoryMap> {
    journal: &'a ObjectFile<M>,
    current_data_offset: u64,
}

impl<'a, M: MemoryMap> Iterator for FieldDataIterator<'a, M> {
    type Item = Result<ValueGuard<'a, DataObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        // If we've reached the end, return None
        if self.current_data_offset == 0 {
            return None;
        }

        // Save the current offset to return
        let offset = self.current_data_offset;

        // Try to get the data object
        match self.journal.data_object(offset) {
            Ok(data_guard) => {
                // Get the next data offset before we return the guard
                self.current_data_offset = data_guard.header.next_field_offset;
                Some(Ok(data_guard))
            }
            Err(e) => {
                // If we can't read the data object, return the error and stop iteration
                self.current_data_offset = 0;
                Some(Err(e))
            }
        }
    }
}

/// Iterator that walks through all DATA objects for a specific entry
pub struct EntryDataIterator<'a, M: MemoryMap> {
    journal: &'a ObjectFile<M>,
    entry_offset: u64,
    current_index: usize,
    total_items: usize,
}

impl<'a, M: MemoryMap> Iterator for EntryDataIterator<'a, M> {
    type Item = Result<ValueGuard<'a, DataObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        // If we've reached the end of the data indices, return None
        if self.current_index >= self.total_items {
            return None;
        }

        // Get the entry object to access the data offset
        match self.journal.entry_object(self.entry_offset) {
            Ok(entry_guard) => {
                let idx = self.current_index;
                self.current_index += 1;

                let data_offset = match &entry_guard.items {
                    EntryItemsType::Regular(items) => {
                        if idx >= items.len() {
                            return None;
                        }
                        items[idx].object_offset
                    }
                    EntryItemsType::Compact(items) => {
                        if idx >= items.len() {
                            return None;
                        }
                        items[idx].object_offset as u64
                    }
                };

                // Drop the entry guard before obtaining the data object
                drop(entry_guard);

                // Try to get the data object
                match self.journal.data_object(data_offset) {
                    Ok(data_guard) => Some(Ok(data_guard)),
                    Err(e) => Some(Err(e)),
                }
            }
            Err(e) => {
                // If we can't read the entry, return the error and stop iteration
                self.current_index = self.total_items;
                Some(Err(e))
            }
        }
    }
}
