#![allow(clippy::field_reassign_with_default)]

use super::mmap::{MemoryMap, MemoryMapMut, WindowManager};
use crate::collections::HashMap;
use crate::error::{JournalError, Result};
use crate::file::guarded_cell::GuardedCell;
use crate::file::hash;
use crate::file::object::*;
use crate::file::offset_array;
use std::fs::{File, OpenOptions};
use std::marker::PhantomData;
use std::num::NonZeroU64;
use std::time::Duration;
use zerocopy::{ByteSlice, FromBytes};

use crate::file::value_guard::ValueGuard;

// Size to pad objects to (8 bytes)
const OBJECT_ALIGNMENT: u64 = 8;

pub trait BucketVisitor<'a> {
    type Object: JournalObject<&'a [u8]> + HashableObject;
    type Output;

    /// Called for each object in the bucket. Return Some(output) to stop iteration,
    /// or None to continue to the next object.
    fn visit(&mut self, object: &ValueGuard<'a, Self::Object>) -> Result<Option<Self::Output>>;
}

struct PayloadMatcher<'data, T> {
    payload: &'data [u8],
    hash: u64,
    _phantom: PhantomData<T>,
}

impl<'data, B: ByteSlice> PayloadMatcher<'data, DataObject<B>> {
    fn data_matcher(payload: &'data [u8], hash: u64) -> Self {
        Self {
            payload,
            hash,
            _phantom: PhantomData::<DataObject<B>>,
        }
    }
}

impl<'data, B: ByteSlice> PayloadMatcher<'data, FieldObject<B>> {
    fn field_matcher(payload: &'data [u8], hash: u64) -> Self {
        Self {
            payload,
            hash,
            _phantom: PhantomData::<FieldObject<B>>,
        }
    }
}

impl<'a, T> BucketVisitor<'a> for PayloadMatcher<'_, T>
where
    T: JournalObject<&'a [u8]> + HashableObject,
{
    type Object = T;
    type Output = NonZeroU64;

    fn visit(&mut self, object: &ValueGuard<'a, Self::Object>) -> Result<Option<Self::Output>> {
        if object.hash() == self.hash && object.get_payload() == self.payload {
            Ok(Some(object.offset()))
        } else {
            Ok(None)
        }
    }
}

#[derive(Debug, Clone)]
pub struct JournalFileOptions {
    machine_id: uuid::Uuid,
    boot_id: uuid::Uuid,
    seqnum_id: uuid::Uuid,
    file_id: uuid::Uuid,
    window_size: u64,
    data_hash_table_buckets: usize,
    field_hash_table_buckets: usize,
    enable_keyed_hash: bool,
}

impl JournalFileOptions {
    pub fn new(machine_id: uuid::Uuid, boot_id: uuid::Uuid, seqnum_id: uuid::Uuid) -> Self {
        let file_id = uuid::Uuid::new_v4();

        Self {
            machine_id,
            boot_id,
            seqnum_id,
            file_id,
            window_size: 64 * 1024,
            data_hash_table_buckets: 4096,
            field_hash_table_buckets: 512,
            enable_keyed_hash: true,
        }
    }

    /// Creates options with bucket sizes optimized based on previous utilization
    pub fn with_optimized_buckets(
        mut self,
        previous_utilization: Option<BucketUtilization>,
        max_file_size: Option<u64>,
    ) -> Self {
        let (data_buckets, field_buckets) = if let Some(utilization) = previous_utilization {
            let data_utilization = utilization.data_utilization();
            let field_utilization = utilization.field_utilization();

            let data_buckets = if data_utilization > 0.75 {
                (utilization.data_total * 2).next_power_of_two()
            } else if data_utilization < 0.25 && utilization.data_total > 4096 {
                (utilization.data_total / 2).next_power_of_two()
            } else {
                utilization.data_total
            };

            let field_buckets = if field_utilization > 0.75 {
                (utilization.field_total * 2).next_power_of_two()
            } else if field_utilization < 0.25 && utilization.field_total > 512 {
                (utilization.field_total / 2).next_power_of_two()
            } else {
                utilization.field_total
            };

            (data_buckets, field_buckets)
        } else {
            // Initial sizing based on rotation policy max file size
            let max_file_size = max_file_size.unwrap_or(8 * 1024 * 1024);

            // 16 MiB -> 4096 data buckets
            let data_buckets = (max_file_size / 4096).max(1024).next_power_of_two() as usize;
            let field_buckets = 128; // Assume ~8:1 data:field ratio

            (data_buckets, field_buckets)
        };

        self.data_hash_table_buckets = data_buckets;
        self.field_hash_table_buckets = field_buckets;
        self
    }

    pub fn with_window_size(mut self, size: u64) -> Self {
        assert_eq!(size % OBJECT_ALIGNMENT, 0);
        assert_eq!(size % 4096, 0, "Window size must be page-aligned");
        self.window_size = size;
        self
    }

    pub fn with_data_hash_table_buckets(mut self, buckets: usize) -> Self {
        assert!(
            buckets.is_power_of_two(),
            "Hash table buckets should be a power of two"
        );
        self.data_hash_table_buckets = buckets;
        self
    }

    pub fn with_field_hash_table_buckets(mut self, buckets: usize) -> Self {
        assert!(
            buckets.is_power_of_two(),
            "Hash table buckets should be a power of two"
        );
        self.field_hash_table_buckets = buckets;
        self
    }

    pub fn with_keyed_hash(mut self, enabled: bool) -> Self {
        self.enable_keyed_hash = enabled;
        self
    }

    pub fn create<M: MemoryMapMut>(self, file: &crate::repository::File) -> Result<JournalFile<M>> {
        JournalFile::create(file, self)
    }
}

/// Hash table bucket utilization statistics
#[derive(Debug, Clone, Copy)]
pub struct BucketUtilization {
    pub data_occupied: usize,
    pub data_total: usize,
    pub field_occupied: usize,
    pub field_total: usize,
}

impl BucketUtilization {
    pub fn data_utilization(&self) -> f64 {
        if self.data_total == 0 {
            0.0
        } else {
            self.data_occupied as f64 / self.data_total as f64
        }
    }

    pub fn field_utilization(&self) -> f64 {
        if self.field_total == 0 {
            0.0
        } else {
            self.field_occupied as f64 / self.field_total as f64
        }
    }
}

///
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
/// - The window manager is wrapped in a `GuardedCell` which owns both the `WindowManager` and
///   its guard flag, providing interior mutability with integrated guard-based exclusion.
/// - The guard flag ensures only one object can be active at a time.
/// - Methods like `data_object()` return a `ValueGuard<T>` that automatically releases the guard
///   when dropped.
///
/// This design ensures that memory safety is maintained even though references to memory-mapped
/// regions could be invalidated when new objects are created.
pub struct JournalFile<M: MemoryMap> {
    // The validated File this journal represents
    file: crate::repository::File,

    // Persistent memory maps for journal header and data/field hash tables
    header_map: M,
    data_hash_table_map: Option<M>,
    field_hash_table_map: Option<M>,

    // Window manager for other objects (owns the guard flag internally)
    window_manager: GuardedCell<WindowManager<M>>,
}

fn map_hash_table<M: MemoryMap>(
    file: &File,
    offset: Option<NonZeroU64>,
    size: Option<NonZeroU64>,
) -> Result<Option<M>> {
    let (Some(offset), Some(size)) = (offset, size) else {
        return Ok(None);
    };

    if offset.get() <= std::mem::size_of::<JournalHeader>() as u64 {
        return Err(JournalError::InvalidObjectLocation);
    }
    if size.get() <= std::mem::size_of::<ObjectHeader>() as u64 {
        return Err(JournalError::InvalidObjectLocation);
    }

    let offset = offset.get() - std::mem::size_of::<ObjectHeader>() as u64;
    let size = std::mem::size_of::<ObjectHeader>() as u64 + size.get();
    M::create(file, offset, size).map(Some)
}

impl<M: MemoryMap> JournalFile<M> {
    pub fn visit_bucket<'a, H, V>(
        &'a self,
        hash_table: Option<H>,
        hash: u64,
        mut visitor: V,
    ) -> Result<Option<V::Output>>
    where
        H: HashTable<Object = V::Object>,
        V: BucketVisitor<'a>,
    {
        let hash_table = hash_table.ok_or(JournalError::MissingHashTable)?;
        let bucket = hash_table.hash_item_ref(hash);
        let mut object_offset = bucket.head_hash_offset;

        while let Some(offset) = object_offset {
            let object_guard = self.journal_object_ref::<V::Object>(offset)?;

            if let Some(output) = visitor.visit(&object_guard)? {
                return Ok(Some(output));
            }

            object_offset = object_guard.next_hash_offset();
        }

        Ok(None)
    }

    pub fn open(file: &crate::repository::File, window_size: u64) -> Result<Self> {
        debug_assert_eq!(window_size % OBJECT_ALIGNMENT, 0);

        // Open file and check its size
        let fd = OpenOptions::new()
            .read(true)
            .write(false)
            .open(file.path())?;

        // Create a memory map for the header
        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let header_map = M::create(&fd, 0, header_size)?;
        let header = JournalHeader::ref_from_prefix(&header_map).unwrap().0;
        if header.signature != *b"LPKSHHRH" {
            return Err(JournalError::InvalidMagicNumber);
        }

        // Initialize the hash table maps if they exist
        let data_hash_table_map = map_hash_table(
            &fd,
            header.data_hash_table_offset,
            header.data_hash_table_size,
        )?;
        let field_hash_table_map = map_hash_table(
            &fd,
            header.field_hash_table_offset,
            header.field_hash_table_size,
        )?;

        // Create window manager for the rest of the objects
        let window_manager = GuardedCell::new(WindowManager::new(fd, window_size, 16)?);

        Ok(JournalFile {
            file: file.clone(),
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
        })
    }

    pub fn file(&self) -> &crate::repository::File {
        &self.file
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
        let header = self.journal_header_ref();

        header.entry_array_offset.and_then(|head_offset| {
            std::num::NonZeroUsize::new(header.n_entries as usize)
                .map(|total_items| offset_array::List::new(head_offset, total_items))
        })
    }

    pub fn entry_offsets(&self, offsets: &mut Vec<NonZeroU64>) -> Result<()> {
        if let Some(entry_list) = self.entry_list() {
            entry_list.collect_offsets(self, offsets)?;
        }

        Ok(())
    }

    // Returns the data object offsets of the entry object at the specified
    // offset
    pub fn entry_data_object_offsets(
        &self,
        entry_offset: NonZeroU64,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        let entry_guard = self.entry_ref(entry_offset)?;
        entry_guard.collect_offsets(offsets)
    }

    pub fn journal_header_ref(&self) -> &JournalHeader {
        JournalHeader::ref_from_prefix(&self.header_map).unwrap().0
    }

    pub fn data_hash_table_map(&self) -> Option<&M> {
        self.data_hash_table_map.as_ref()
    }
    pub fn field_hash_table_map(&self) -> Option<&M> {
        self.field_hash_table_map.as_ref()
    }

    pub fn data_hash_table_ref(&self) -> Option<DataHashTable<&[u8]>> {
        self.data_hash_table_map
            .as_ref()
            .and_then(|m| DataHashTable::<&[u8]>::from_data(m, false))
    }

    pub fn field_hash_table_ref(&self) -> Option<FieldHashTable<&[u8]>> {
        self.field_hash_table_map
            .as_ref()
            .and_then(|m| FieldHashTable::<&[u8]>::from_data(m, false))
    }

    pub fn object_header_ref(&self, position: NonZeroU64) -> Result<&ObjectHeader> {
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = self.window_manager.borrow_mut_checked()?;
        let header_slice = window_manager.get_slice(position.get(), size_needed)?;
        Ok(ObjectHeader::ref_from_bytes(header_slice).unwrap())
    }

    fn journal_object_ref<'a, T>(&'a self, offset: NonZeroU64) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObject<&'a [u8]>,
    {
        let is_compact = self
            .journal_header_ref()
            .has_incompatible_flag(HeaderIncompatibleFlags::Compact);

        self.window_manager.with_guarded(offset, |wm| {
            // Get the object header to determine size
            let size_needed = {
                let header_slice =
                    wm.get_slice(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                let header = ObjectHeader::ref_from_bytes(header_slice).unwrap();
                header.size
            };

            // Get the full object data
            let data = wm.get_slice(offset.get(), size_needed)?;

            // Parse the object
            let value = T::from_data(data, is_compact).ok_or(JournalError::ZerocopyFailure)?;

            Ok(value)
        })
    }

    pub fn offset_array_ref(
        &self,
        offset: NonZeroU64,
    ) -> Result<ValueGuard<'_, OffsetArrayObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn field_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<'_, FieldObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn entry_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<'_, EntryObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn data_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<'_, DataObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn tag_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<'_, TagObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn find_data_offset(&self, hash: u64, payload: &[u8]) -> Result<Option<NonZeroU64>> {
        let visitor = PayloadMatcher::data_matcher(payload, hash);
        self.visit_bucket(self.data_hash_table_ref(), hash, visitor)
    }

    pub fn find_field_offset(&self, hash: u64, payload: &[u8]) -> Result<Option<NonZeroU64>> {
        let visitor = PayloadMatcher::field_matcher(payload, hash);
        self.visit_bucket(self.field_hash_table_ref(), hash, visitor)
    }

    /// Run a directed partition point query on a data object's entry array
    ///
    /// This finds the first/last entry (depending on direction) that satisfies the given predicate
    /// in the entry array chain of the data object.
    pub fn data_object_directed_partition_point<F>(
        &self,
        data_offset: NonZeroU64,
        predicate: F,
        direction: offset_array::Direction,
    ) -> Result<Option<NonZeroU64>>
    where
        F: Fn(NonZeroU64) -> Result<bool>,
    {
        let Some(cursor) = self.data_ref(data_offset)?.inlined_cursor() else {
            return Ok(None);
        };

        let Some(best_match) = cursor.directed_partition_point(self, predicate, direction)? else {
            return Ok(None);
        };

        best_match.value(self)
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
            next_field_offset: None,
        };

        // Find the first non-empty bucket
        iterator.advance_to_next_nonempty_bucket();

        iterator
    }

    pub fn load_fields(&self) -> Result<HashMap<String, String>> {
        let remapping_payload = b"ND_REMAPPING=1".as_slice();
        let hash = self.hash(remapping_payload);

        let mut field_map = HashMap::default();

        match self.find_data_offset(hash, remapping_payload) {
            Ok(Some(offset)) => {
                let Some(ic) = self.data_ref(offset)?.header.inlined_cursor() else {
                    return Err(JournalError::EmptyInlineCursor);
                };

                let mut entry_offsets = Vec::new();
                ic.collect_offsets(self, &mut entry_offsets)?;

                let mut data_offsets = Vec::new();
                for entry_offset in entry_offsets {
                    {
                        let entry_object = self.entry_ref(entry_offset)?;
                        data_offsets.clear();
                        entry_object.collect_offsets(&mut data_offsets)?;
                    }

                    for data_offset in data_offsets.iter().copied() {
                        let data_object = self.data_ref(data_offset)?;
                        let payload = data_object.payload_bytes();

                        if payload == remapping_payload {
                            continue;
                        }

                        let s = std::str::from_utf8(payload).expect("utf8 data");

                        let Some((field, value)) = s.split_once('=') else {
                            return Err(JournalError::InvalidField);
                        };

                        let systemd_name = String::from(field);
                        let otel_name = String::from(value);

                        field_map.insert(otel_name, systemd_name);
                    }
                }
            }
            Ok(None) => {
                // Just load fields from the field hash table
            }
            Err(e) => {
                return Err(e);
            }
        };

        for value_guard in self.fields() {
            let field = value_guard?;
            if field.payload.starts_with(b"ND") {
                continue;
            }
            let s = String::from_utf8(field.get_payload().to_vec()).expect("utf8 data");
            field_map.insert(s.clone(), s);
        }

        Ok(field_map)
    }

    /// Creates an iterator over all DATA objects for the specified field
    pub fn field_data_objects<'a>(
        &'a self,
        field_name: &'a [u8],
    ) -> Result<FieldDataIterator<'a, M>> {
        // Find the field offset by name
        let field_hash = self.hash(field_name);
        let Some(field_offset) = self.find_field_offset(field_hash, field_name)? else {
            return Ok(FieldDataIterator {
                journal: self,
                current_data_offset: None,
            });
        };

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
    pub fn entry_data_objects(&self, entry_offset: NonZeroU64) -> Result<EntryDataIterator<'_, M>> {
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
            entry_offset: Some(entry_offset),
            current_index: 0,
            total_items,
        })
    }

    /// Get hash table bucket utilization statistics
    pub fn bucket_utilization(&self) -> Option<BucketUtilization> {
        let data_hash_table = self.data_hash_table_ref()?;
        let data_total = data_hash_table.items.len();
        let data_occupied = data_hash_table
            .items
            .iter()
            .filter(|item| item.head_hash_offset.is_some())
            .count();

        let field_hash_table = self.field_hash_table_ref()?;
        let field_total = field_hash_table.items.len();
        let field_occupied = field_hash_table
            .items
            .iter()
            .filter(|item| item.head_hash_offset.is_some())
            .count();

        Some(BucketUtilization {
            data_occupied,
            data_total,
            field_occupied,
            field_total,
        })
    }

    /// Get the duration covered by all entries in the journal
    /// Returns None if the journal is empty or contains only one entry
    pub fn duration(&self) -> Option<Duration> {
        let header = self.journal_header_ref();

        if header.head_entry_realtime == 0 || header.tail_entry_realtime == 0 {
            return None;
        }

        if header.tail_entry_realtime <= header.head_entry_realtime {
            // Single entry or invalid state
            return None;
        }

        let duration_micros = header.tail_entry_realtime - header.head_entry_realtime;
        Some(Duration::from_micros(duration_micros))
    }
}

impl<M: MemoryMapMut> JournalFile<M> {
    /// Syncs all file data to disk, ensuring all changes are persisted
    ///
    /// This performs a two-step sync process:
    /// 1. Flushes memory-mapped regions to the file page cache (msync)
    /// 2. Syncs the file page cache to physical disk (fdatasync)
    pub fn sync(&mut self) -> Result<()> {
        // Flush memory-mapped header to file page cache
        self.header_map.flush()?;

        // Sync file page cache to disk
        let window_manager = self.window_manager.get_mut();
        window_manager.sync()?;

        Ok(())
    }

    /// Creates a successor journal file with optimized bucket sizes based on this file's utilization
    pub fn create_successor(
        &self,
        file: &crate::repository::File,
        max_file_size: Option<u64>,
    ) -> Result<Self> {
        let header = self.journal_header_ref();
        let bucket_utilization = self.bucket_utilization();

        let options = JournalFileOptions::new(
            uuid::Uuid::from_bytes(header.machine_id),
            uuid::Uuid::from_bytes(header.tail_entry_boot_id),
            uuid::Uuid::from_bytes(header.seqnum_id),
        )
        .with_window_size(8 * 1024 * 1024)
        .with_optimized_buckets(bucket_utilization, max_file_size)
        .with_keyed_hash(header.has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash));

        Self::create(file, options)
    }

    pub fn create(file: &crate::repository::File, options: JournalFileOptions) -> Result<Self> {
        let fd = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(file.path())?;

        // Calculate hash table sizes
        let data_hash_table_size =
            options.data_hash_table_buckets * std::mem::size_of::<HashItem>();
        let field_hash_table_size =
            options.field_hash_table_buckets * std::mem::size_of::<HashItem>();

        // Calculate hash table offsets
        let data_hash_table_offset = std::mem::size_of::<JournalHeader>() as u64
            + std::mem::size_of::<ObjectHeader>() as u64;
        let field_hash_table_offset = data_hash_table_offset
            + data_hash_table_size as u64
            + std::mem::size_of::<ObjectHeader>() as u64;

        // Create header with options configuration
        let mut header = JournalHeader::default();
        header.signature = *b"LPKSHHRH";

        // Set flags based on options configuration
        if options.enable_keyed_hash {
            header.incompatible_flags |= HeaderIncompatibleFlags::KeyedHash as u32;
        }

        // Set hash table configuration
        header.data_hash_table_offset = NonZeroU64::new(data_hash_table_offset);
        header.data_hash_table_size = NonZeroU64::new(data_hash_table_size as u64);
        header.field_hash_table_offset = NonZeroU64::new(field_hash_table_offset);
        header.field_hash_table_size = NonZeroU64::new(field_hash_table_size as u64);

        // Set other header fields
        header.tail_object_offset =
            NonZeroU64::new(data_hash_table_offset + data_hash_table_size as u64);
        header.header_size = std::mem::size_of::<JournalHeader>() as u64;
        header.n_objects = 2;
        header.arena_size =
            field_hash_table_offset + field_hash_table_size as u64 - header.header_size;

        // Set IDs from options
        header.machine_id = *options.machine_id.as_bytes();
        header.tail_entry_boot_id = *options.boot_id.as_bytes();
        header.file_id = *options.file_id.as_bytes();
        header.seqnum_id = *options.seqnum_id.as_bytes();

        // Create memory maps for hash tables
        let data_hash_table_map = map_hash_table(
            &fd,
            header.data_hash_table_offset,
            header.data_hash_table_size,
        )?;
        let field_hash_table_map = map_hash_table(
            &fd,
            header.field_hash_table_offset,
            header.field_hash_table_size,
        )?;

        // Create header memory map and write header
        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let mut header_map = M::create(&fd, 0, header_size)?;
        {
            let header_mut = JournalHeader::mut_from_prefix(&mut header_map).unwrap().0;
            *header_mut = header;
            // Set state to ONLINE as per journal file format spec
            header_mut.state = JournalState::Online as u8;
        }

        // Create window manager for the rest of the objects
        let window_manager = GuardedCell::new(WindowManager::new(fd, options.window_size, 32)?);

        let mut jf = JournalFile {
            file: file.clone(),
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
        };

        // write data hash table object header info
        {
            let offset = NonZeroU64::new(
                header.data_hash_table_offset.unwrap().get()
                    - std::mem::size_of::<ObjectHeader>() as u64,
            )
            .unwrap();
            let size = header.data_hash_table_size.unwrap().get()
                + std::mem::size_of::<ObjectHeader>() as u64;

            let object_header = jf.object_header_mut(offset)?;
            object_header.type_ = ObjectType::DataHashTable as u8;
            object_header.size = size
        }

        // write field hash table object header info
        {
            let offset = NonZeroU64::new(
                header.field_hash_table_offset.unwrap().get()
                    - std::mem::size_of::<ObjectHeader>() as u64,
            )
            .unwrap();
            let size = header.field_hash_table_size.unwrap().get()
                + std::mem::size_of::<ObjectHeader>() as u64;

            let object_header = jf.object_header_mut(offset)?;
            object_header.type_ = ObjectType::FieldHashTable as u8;
            object_header.size = size
        }

        // Sync to ensure the ONLINE state is persisted to disk
        jf.sync()?;

        Ok(jf)
    }

    pub fn journal_header_mut(&mut self) -> &mut JournalHeader {
        JournalHeader::mut_from_prefix(&mut self.header_map)
            .unwrap()
            .0
    }

    pub fn data_hash_table_mut(&mut self) -> Option<DataHashTable<&mut [u8]>> {
        self.data_hash_table_map
            .as_mut()
            .and_then(|m| DataHashTable::<&mut [u8]>::from_data_mut(m, false))
    }

    pub fn field_hash_table_mut(&mut self) -> Option<FieldHashTable<&mut [u8]>> {
        self.field_hash_table_map
            .as_mut()
            .and_then(|m| FieldHashTable::<&mut [u8]>::from_data_mut(m, false))
    }

    #[allow(clippy::mut_from_ref)]
    fn object_header_mut(&self, offset: NonZeroU64) -> Result<&mut ObjectHeader> {
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = self.window_manager.borrow_mut_checked()?;
        let header_slice = window_manager.get_slice_mut(offset.get(), size_needed)?;
        Ok(ObjectHeader::mut_from_bytes(header_slice).unwrap())
    }

    fn journal_object_mut<'a, T>(
        &'a self,
        type_: ObjectType,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObjectMut<&'a mut [u8]>,
    {
        let is_compact = self
            .journal_header_ref()
            .has_incompatible_flag(HeaderIncompatibleFlags::Compact);

        self.window_manager.with_guarded(offset, |wm| {
            // Get or set the size
            let size_needed = match size {
                Some(size) => {
                    // Setting object header for a new object
                    let header_slice =
                        wm.get_slice_mut(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                    let header = ObjectHeader::mut_from_bytes(header_slice).unwrap();
                    header.type_ = type_ as u8;
                    header.size = size;
                    size
                }
                None => {
                    // Reading existing object header
                    let header_slice =
                        wm.get_slice(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                    let header = ObjectHeader::ref_from_bytes(header_slice).unwrap();
                    if header.type_ != type_ as u8 {
                        return Err(JournalError::InvalidObjectType);
                    }
                    header.size
                }
            };

            // Get mutable object data
            let data = wm.get_slice_mut(offset.get(), size_needed)?;

            // Parse the mutable object
            let value = T::from_data_mut(data, is_compact).ok_or(JournalError::ZerocopyFailure)?;

            Ok(value)
        })
    }

    pub fn offset_array_mut(
        &self,
        offset: NonZeroU64,
        capacity: Option<NonZeroU64>,
    ) -> Result<ValueGuard<'_, OffsetArrayObject<&mut [u8]>>> {
        let size = capacity.map(|c| {
            let mut size = std::mem::size_of::<OffsetArrayObjectHeader>() as u64;

            let is_compact = self
                .journal_header_ref()
                .has_incompatible_flag(HeaderIncompatibleFlags::Compact);
            if is_compact {
                size += c.get() * std::mem::size_of::<u32>() as u64;
            } else {
                size += c.get() * std::mem::size_of::<u64>() as u64;
            }

            size
        });

        self.journal_object_mut(ObjectType::EntryArray, offset, size)
    }

    pub fn field_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, FieldObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<FieldObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Field, offset, size)
    }

    pub fn entry_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, EntryObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Entry, offset, size)
    }

    pub fn data_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, DataObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Data, offset, size)
    }

    pub fn tag_mut(
        &self,
        offset: NonZeroU64,
        new: bool,
    ) -> Result<ValueGuard<'_, TagObject<&mut [u8]>>> {
        let size = if new {
            Some(std::mem::size_of::<TagObjectHeader>() as u64)
        } else {
            None
        };
        self.journal_object_mut(ObjectType::Tag, offset, size)
    }
}

macro_rules! impl_hash_table_set_tail_offset {
    (
        $method_name:ident,
        $hash_table_ref:ident,
        $hash_table_mut:ident,
        $object_mut:ident
    ) => {
        pub fn $method_name(&mut self, hash: u64, object_offset: NonZeroU64) -> Result<()> {
            let hash_item = {
                let Some(ht) = self.$hash_table_ref() else {
                    return Err(JournalError::MissingHashTable);
                };
                *ht.hash_item_ref(hash)
            };

            if let Some(tail_hash_offset) = hash_item.tail_hash_offset {
                let mut tail_object = self.$object_mut(tail_hash_offset, None)?;
                tail_object.set_next_hash_offset(object_offset);
            }

            let Some(mut ht) = self.$hash_table_mut() else {
                return Err(JournalError::MissingHashTable);
            };

            let hash_item = ht.hash_item_mut(hash);
            if hash_item.head_hash_offset.is_none() {
                hash_item.head_hash_offset = Some(object_offset);
            }
            hash_item.tail_hash_offset = Some(object_offset);

            Ok(())
        }
    };
}

impl<M: MemoryMapMut> JournalFile<M> {
    impl_hash_table_set_tail_offset!(
        data_hash_table_set_tail_offset,
        data_hash_table_ref,
        data_hash_table_mut,
        data_mut
    );

    impl_hash_table_set_tail_offset!(
        field_hash_table_set_tail_offset,
        field_hash_table_ref,
        field_hash_table_mut,
        field_mut
    );
}

/// Iterator that walks through all field objects in the field hash table
pub struct FieldIterator<'a, M: MemoryMap> {
    journal: &'a JournalFile<M>,
    field_hash_table: Option<FieldHashTable<&'a [u8]>>,
    current_bucket_index: usize,
    next_field_offset: Option<NonZeroU64>,
}

impl<M: MemoryMap> FieldIterator<'_, M> {
    /// Advances to the next non-empty bucket
    fn advance_to_next_nonempty_bucket(&mut self) {
        // If we don't have a hash table, there's nothing to iterate
        let Some(hash_table) = &self.field_hash_table else {
            return;
        };

        let items = &hash_table.items;

        // Find the next non-empty bucket
        while self.current_bucket_index < items.len() {
            let bucket = items[self.current_bucket_index];
            if bucket.head_hash_offset.is_some() {
                self.next_field_offset = bucket.head_hash_offset;
                return;
            }
            self.current_bucket_index += 1;
        }

        // No more non-empty buckets
        self.next_field_offset = None;
    }
}

impl<'a, M: MemoryMap> Iterator for FieldIterator<'a, M> {
    type Item = Result<ValueGuard<'a, FieldObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        let offset = self.next_field_offset?;

        match self.journal.field_ref(offset) {
            Ok(field_guard) => {
                // Get the next field offset before we return the guard
                self.next_field_offset = field_guard.header.next_hash_offset;

                // If we've reached the end of the chain, move to the next bucket
                if self.next_field_offset.is_none() {
                    self.current_bucket_index += 1;
                    self.advance_to_next_nonempty_bucket();
                }

                Some(Ok(field_guard))
            }
            Err(e) => {
                self.next_field_offset = None;
                Some(Err(e))
            }
        }
    }
}

/// Iterator that walks through all DATA objects for a specific field
pub struct FieldDataIterator<'a, M: MemoryMap> {
    journal: &'a JournalFile<M>,
    current_data_offset: Option<NonZeroU64>,
}

impl<'a, M: MemoryMap> Iterator for FieldDataIterator<'a, M> {
    type Item = Result<ValueGuard<'a, DataObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        let data_offset = self.current_data_offset?;

        match self.journal.data_ref(data_offset) {
            Ok(data_guard) => {
                // Get the next data offset before we return the guard
                self.current_data_offset = data_guard.header.next_field_offset;
                Some(Ok(data_guard))
            }
            Err(e) => {
                self.current_data_offset = None;
                Some(Err(e))
            }
        }
    }
}

/// Iterator that walks through all DATA objects for a specific entry
pub struct EntryDataIterator<'a, M: MemoryMap> {
    journal: &'a JournalFile<M>,
    entry_offset: Option<NonZeroU64>,
    current_index: usize,
    total_items: usize,
}

impl<'a, M: MemoryMap> Iterator for EntryDataIterator<'a, M> {
    type Item = Result<ValueGuard<'a, DataObject<&'a [u8]>>>;

    fn next(&mut self) -> Option<Self::Item> {
        let entry_offset = self.entry_offset?;

        // If we've reached the end of the data indices, return None
        if self.current_index >= self.total_items {
            return None;
        }

        // Get the entry object to access the data offset
        match self.journal.entry_ref(entry_offset) {
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

                let data_offset = NonZeroU64::new(data_offset)?;

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
