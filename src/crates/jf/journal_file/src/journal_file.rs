#![allow(clippy::field_reassign_with_default)]

use crate::hash;
use crate::object::*;
use crate::offset_array;
use error::{JournalError, Result};
use std::cell::{RefCell, UnsafeCell};
use std::fs::OpenOptions;
use std::path::Path;
use window_manager::{MemoryMap, MemoryMapMut, WindowManager};
use zerocopy::FromBytes;

#[cfg(debug_assertions)]
use std::backtrace::Backtrace;

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
/// `JournalFile` uses interior mutability to provide a safe API with the following characteristics:
///
/// - The window manager is wrapped in an `UnsafeCell` to allow mutation through a shared reference.
/// - A single `RefCell<bool>` guards access to ensure only one object can be active at a time.
/// - Methods like `data_object()` return a `ValueGuard<T>` that automatically releases the lock
///   when dropped.
///
/// This design ensures that memory safety is maintained even though references to memory-mapped
/// regions could be invalidated when new objects are created.
pub struct JournalFile<M: MemoryMap> {
    // Persistent memory maps for journal header and data/field hash tables
    header_map: M,
    data_hash_table_map: Option<M>,
    field_hash_table_map: Option<M>,

    // Window manager for other objects
    window_manager: UnsafeCell<WindowManager<M>>,

    // Flag to track if any object is in use
    object_in_use: RefCell<bool>,

    #[cfg(debug_assertions)]
    prev_backtrace: RefCell<Backtrace>,
    #[cfg(debug_assertions)]
    backtrace: RefCell<Backtrace>,
}

impl<M: MemoryMapMut> JournalFile<M> {
    pub fn create(path: impl AsRef<Path>, window_size: u64) -> Result<Self> {
        debug_assert_eq!(window_size % OBJECT_ALIGNMENT, 0);

        let file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(&path)?;

        let mut header = JournalHeader::default();
        header.signature = *b"LPKSHHRH";

        header.data_hash_table_offset = std::mem::size_of::<JournalHeader>() as u64
            + std::mem::size_of::<ObjectHeader>() as u64;
        header.data_hash_table_size = 4096 * std::mem::size_of::<HashItem>() as u64;

        header.field_hash_table_offset = header.data_hash_table_offset
            + header.data_hash_table_size
            + std::mem::size_of::<ObjectHeader>() as u64;
        header.field_hash_table_size = 512 * std::mem::size_of::<HashItem>() as u64;

        debug_assert_eq!(header.data_hash_table_offset % OBJECT_ALIGNMENT, 0);
        debug_assert_eq!(header.data_hash_table_size % OBJECT_ALIGNMENT, 0);
        let data_hash_table_map = header.map_data_hash_table(&file)?;

        debug_assert_eq!(header.field_hash_table_offset % OBJECT_ALIGNMENT, 0);
        debug_assert_eq!(header.field_hash_table_size % OBJECT_ALIGNMENT, 0);
        let field_hash_table_map = header.map_field_hash_table(&file)?;

        header.tail_object_offset = header.data_hash_table_offset + header.data_hash_table_size;
        header.n_objects = 2;

        debug_assert_eq!(header.tail_object_offset % OBJECT_ALIGNMENT, 0);

        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let mut header_map = M::create(&file, 0, header_size)?;
        {
            let header_mut = JournalHeader::mut_from_prefix(&mut header_map).unwrap().0;
            *header_mut = header;
        }

        // Create window manager for the rest of the objects
        let window_manager = UnsafeCell::new(WindowManager::new(file, window_size, 32)?);

        Ok(JournalFile {
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
            object_in_use: RefCell::new(false),

            #[cfg(debug_assertions)]
            prev_backtrace: RefCell::new(Backtrace::capture()),
            #[cfg(debug_assertions)]
            backtrace: RefCell::new(Backtrace::capture()),
        })
    }

    pub fn journal_header_mut(&mut self) -> &mut JournalHeader {
        JournalHeader::mut_from_prefix(&mut self.header_map)
            .unwrap()
            .0
    }

    pub fn data_hash_table_mut(&mut self) -> Option<HashTableObject<&mut [u8]>> {
        self.data_hash_table_map
            .as_mut()
            .map(|m| HashTableObject::<&mut [u8]>::from_data_mut(m, false))
    }

    pub fn field_hash_table_mut(&mut self) -> Option<HashTableObject<&mut [u8]>> {
        self.field_hash_table_map
            .as_mut()
            .map(|m| HashTableObject::<&mut [u8]>::from_data_mut(m, false))
    }

    fn object_header_mut(&self, position: u64) -> Result<&mut ObjectHeader> {
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let header_slice = window_manager.get_slice_mut(position, size_needed)?;
        Ok(ObjectHeader::mut_from_bytes(header_slice).unwrap())
    }

    fn object_data_mut(&self, position: u64, size_needed: u64) -> Result<&mut [u8]> {
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let object_slice = window_manager.get_slice_mut(position, size_needed)?;
        Ok(object_slice)
    }

    fn journal_object_mut<'a, T>(
        &'a self,
        type_: ObjectType,
        position: u64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObjectMut<&'a mut [u8]>,
    {
        // Check if any object is already in use
        let mut is_in_use = self.object_in_use.borrow_mut();
        if *is_in_use {
            #[cfg(debug_assertions)]
            {
                eprintln!(
                    "Value is in use. Current Backtrace: {:?}, Previous Backtrace: {:?}",
                    self.backtrace.borrow().to_string(),
                    self.prev_backtrace.borrow().to_string()
                );
            }
            return Err(JournalError::ValueGuardInUse);
        }

        #[cfg(debug_assertions)]
        {
            self.backtrace.swap(&self.prev_backtrace);
            let _ = self.backtrace.replace(Backtrace::force_capture());
        }

        let is_compact = self
            .journal_header_ref()
            .has_incompatible_flag(HeaderIncompatibleFlags::Compact);

        let size_needed = match size {
            Some(size) => {
                let header = self.object_header_mut(position)?;
                header.type_ = type_ as u8;
                header.size = size;
                size
            }
            None => {
                let header = self.object_header_ref(position)?;
                if header.type_ != type_ as u8 {
                    return Err(JournalError::InvalidObjectType);
                }
                header.size
            }
        };

        let data = self.object_data_mut(position, size_needed)?;
        let object = T::from_data_mut(data, is_compact);

        // Mark as in use
        *is_in_use = true;
        Ok(ValueGuard::new(object, &self.object_in_use))
    }

    pub fn offset_array_mut(
        &self,
        position: u64,
        capacity: Option<u64>,
    ) -> Result<ValueGuard<OffsetArrayObject<&mut [u8]>>> {
        let size = capacity.map(|c| {
            let mut size = std::mem::size_of::<OffsetArrayObjectHeader>() as u64;

            let is_compact = self
                .journal_header_ref()
                .has_incompatible_flag(HeaderIncompatibleFlags::Compact);
            if is_compact {
                size += c * std::mem::size_of::<u32>() as u64;
            } else {
                size += c * std::mem::size_of::<u64>() as u64;
            }

            size
        });

        let offset_array = self.journal_object_mut(ObjectType::EntryArray, position, size);
        offset_array
    }

    pub fn field_mut(
        &self,
        position: u64,
        size: Option<u64>,
    ) -> Result<ValueGuard<FieldObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<FieldObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Field, position, size)
    }

    pub fn entry_mut(&self, position: u64) -> Result<ValueGuard<EntryObject<&mut [u8]>>> {
        self.journal_object_mut(ObjectType::Entry, position, None)
    }

    pub fn data_mut(
        &self,
        position: u64,
        size: Option<u64>,
    ) -> Result<ValueGuard<DataObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Data, position, size)
    }

    pub fn tag_mut(&self, position: u64, new: bool) -> Result<ValueGuard<TagObject<&mut [u8]>>> {
        let size = if new {
            Some(std::mem::size_of::<TagObjectHeader>() as u64)
        } else {
            None
        };
        self.journal_object_mut(ObjectType::Tag, position, size)
    }
}

impl<M: MemoryMap> JournalFile<M> {
    pub fn open(path: impl AsRef<Path>, window_size: u64) -> Result<Self> {
        debug_assert_eq!(window_size % OBJECT_ALIGNMENT, 0);

        // Open file and check its size
        let file = OpenOptions::new().read(true).write(false).open(&path)?;

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
        let window_manager = UnsafeCell::new(WindowManager::new(file, window_size, 32)?);

        Ok(JournalFile {
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
            object_in_use: RefCell::new(false),

            #[cfg(debug_assertions)]
            prev_backtrace: RefCell::new(Backtrace::capture()),
            #[cfg(debug_assertions)]
            backtrace: RefCell::new(Backtrace::capture()),
        })
    }

    pub fn hash(&self, data: &[u8]) -> u64 {
        let is_keyed_hash = self
            .journal_header_ref()
            .has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash);

        hash::journal_hash_data(
            data,
            is_keyed_hash,
            if is_keyed_hash {
                Some(&self.journal_header_ref().file_id)
            } else {
                None
            },
        )
    }

    pub fn entry_list(&self) -> Option<offset_array::List> {
        let head_offset = std::num::NonZeroU64::new(self.journal_header_ref().entry_array_offset)?;
        let total_items =
            std::num::NonZeroUsize::new(self.journal_header_ref().n_entries as usize)?;
        Some(offset_array::List::new(head_offset, total_items))
    }

    pub fn journal_header_ref(&self) -> &JournalHeader {
        JournalHeader::ref_from_prefix(&self.header_map).unwrap().0
    }

    pub fn data_hash_table_ref(&self) -> Option<HashTableObject<&[u8]>> {
        self.data_hash_table_map
            .as_ref()
            .and_then(|m| HashTableObject::<&[u8]>::from_data(m, false))
    }

    pub fn field_hash_table_ref(&self) -> Option<HashTableObject<&[u8]>> {
        self.field_hash_table_map
            .as_ref()
            .and_then(|m| HashTableObject::<&[u8]>::from_data(m, false))
    }

    fn object_header_ref(&self, position: u64) -> Result<&ObjectHeader> {
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let header_slice = window_manager.get_slice(position, size_needed)?;
        Ok(ObjectHeader::ref_from_bytes(header_slice).unwrap())
    }

    fn object_data_ref(&self, position: u64, size_needed: u64) -> Result<&[u8]> {
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let object_slice = window_manager.get_slice(position, size_needed)?;
        Ok(object_slice)
    }

    fn journal_object_ref<'a, T>(&'a self, position: u64) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObject<&'a [u8]>,
    {
        // Check if any object is already in use
        let mut is_in_use = self.object_in_use.borrow_mut();
        if *is_in_use {
            #[cfg(debug_assertions)]
            {
                eprintln!(
                    "Value is in use. Current Backtrace: {:?}, Previous Backtrace: {:?}",
                    self.backtrace.borrow().to_string(),
                    self.prev_backtrace.borrow().to_string()
                );
            }
            return Err(JournalError::ValueGuardInUse);
        }

        #[cfg(debug_assertions)]
        {
            self.backtrace.swap(&self.prev_backtrace);
            let _ = self.backtrace.replace(Backtrace::force_capture());
        }

        let is_compact = self
            .journal_header_ref()
            .has_incompatible_flag(HeaderIncompatibleFlags::Compact);

        let size_needed = {
            let header = self.object_header_ref(position)?;
            header.size
        };

        let data = self.object_data_ref(position, size_needed)?;
        let Some(object) = T::from_data(data, is_compact) else {
            return Err(JournalError::ZerocopyFailure);
        };

        // Mark as in use
        *is_in_use = true;

        Ok(ValueGuard::new(object, &self.object_in_use))
    }

    pub fn offset_array_ref(&self, position: u64) -> Result<ValueGuard<OffsetArrayObject<&[u8]>>> {
        self.journal_object_ref(position)
    }

    pub fn field_ref(&self, position: u64) -> Result<ValueGuard<FieldObject<&[u8]>>> {
        self.journal_object_ref(position)
    }

    pub fn entry_ref(&self, position: u64) -> Result<ValueGuard<EntryObject<&[u8]>>> {
        self.journal_object_ref(position)
    }

    pub fn data_ref(&self, position: u64) -> Result<ValueGuard<DataObject<&[u8]>>> {
        self.journal_object_ref(position)
    }

    pub fn tag_ref(&self, position: u64) -> Result<ValueGuard<TagObject<&[u8]>>> {
        self.journal_object_ref(position)
    }

    fn lookup_hash_table<'a, T, F>(
        &'a self,
        hash_table: Option<HashTableObject<&[u8]>>,
        data: &[u8],
        hash: u64,
        fetch_fn: F,
    ) -> Result<u64>
    where
        T: HashableObject,
        F: Fn(u64) -> Result<ValueGuard<'a, T>>,
    {
        let hash_table = hash_table.ok_or(JournalError::MissingHashTable)?;

        // Find the right bucket in the hash table
        let hash_table_size = hash_table.items.len();
        let bucket_idx = (hash % hash_table_size as u64) as usize;

        // Get the head object offset from the bucket
        let bucket = hash_table.items[bucket_idx];
        let mut object_offset = bucket.head_hash_offset;

        // Traverse the linked list of objects in this bucket
        while object_offset != 0 {
            match fetch_fn(object_offset) {
                Ok(object_guard) => {
                    // Check if this is the object we're looking for
                    if object_guard.hash() == hash && object_guard.get_payload() == data {
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
    pub fn find_field_offset_by_name(&self, field_name: &[u8], hash: u64) -> Result<u64> {
        self.lookup_hash_table::<FieldObject<&[u8]>, _>(
            self.field_hash_table_ref(),
            field_name,
            hash,
            |offset| self.field_ref(offset),
        )
    }

    /// Finds a data object by payload and returns its offset
    pub fn find_data_offset_by_payload(&self, payload: &[u8], hash: u64) -> Result<u64> {
        self.lookup_hash_table::<DataObject<&[u8]>, _>(
            self.data_hash_table_ref(),
            payload,
            hash,
            |offset| self.data_ref(offset),
        )
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
        let Some(cursor) = self.data_ref(data_offset)?.inlined_cursor() else {
            return Ok(None);
        };

        let best_match = cursor.directed_partition_point(self, predicate, direction)?;

        // Convert the result to an entry offset
        best_match.map(|c| c.value(self)).transpose()
    }

    /// Creates an iterator over all offsets of an offset array list
    pub fn array_offsets(&self, position: u64) -> Result<OffsetArrayListIterator<'_, M>> {
        Ok(OffsetArrayListIterator {
            journal: self,
            offset: position,
            capacity: if position == 0 {
                0
            } else {
                self.offset_array_ref(position)?.capacity()
            },
            index: 0,
        })
    }

    /// Creates an iterator over all entry offsets in the journal
    pub fn entry_offsets(&self) -> Result<OffsetArrayListIterator<'_, M>> {
        self.array_offsets(self.journal_header_ref().entry_array_offset)
    }

    /// Creates an iterator over all field objects in the field hash table
    pub fn fields(&self) -> FieldIterator<'_, M> {
        // Get the field hash table
        let field_hash_table = self.field_hash_table_ref();

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
        let field_hash = self.hash(field_name);
        let field_offset = self.find_field_offset_by_name(field_name, field_hash)?;

        // Get the field object to access its head_data_offset
        let field_guard = self.field_ref(field_offset)?;
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
        let entry_guard = self.entry_ref(entry_offset)?;

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
}

/*
 * Offset array iteration
*/

/// Iterator that returns all offsets in an offset array list
pub struct OffsetArrayListIterator<'a, M: MemoryMap> {
    journal: &'a JournalFile<M>,
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
            let next_offset = match self.journal.offset_array_ref(self.offset) {
                Ok(array_guard) => array_guard.header.next_offset_array,
                Err(e) => return Some(Err(e)),
            };

            // If there's no next offset array, we're done
            if next_offset == 0 {
                self.offset = 0;
                return None;
            }

            // Set up the next offset array
            match self.journal.offset_array_ref(next_offset) {
                Ok(array_guard) => {
                    self.offset = next_offset;
                    self.capacity = array_guard.capacity();
                    self.index = 0;
                }
                Err(e) => return Some(Err(e)),
            }
        }

        // Get the current offset from the offset array
        let offset = match self.journal.offset_array_ref(self.offset) {
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
    journal: &'a JournalFile<M>,
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
        match self.journal.field_ref(offset) {
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
    journal: &'a JournalFile<M>,
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
        match self.journal.data_ref(offset) {
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
    journal: &'a JournalFile<M>,
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
        match self.journal.entry_ref(self.entry_offset) {
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
                match self.journal.data_ref(data_offset) {
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
