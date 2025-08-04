#![allow(unused_imports, clippy::field_reassign_with_default)]

use crate::hash;
use crate::object::*;
use crate::offset_array;
use error::{JournalError, Result};
use std::cell::{RefCell, UnsafeCell};
use std::fs::{File, OpenOptions};
use std::marker::PhantomData;
use std::num::NonZero;
use std::num::NonZeroI128;
use std::num::NonZeroU64;
use std::path::Path;
use window_manager::{MemoryMap, MemoryMapMut, WindowManager};
use zerocopy::{ByteSlice, FromBytes, SplitByteSlice, SplitByteSliceMut};

#[cfg(debug_assertions)]
use std::backtrace::Backtrace;

use crate::value_guard::ValueGuard;

#[cfg(target_os = "linux")]
pub fn load_machine_id() -> Result<[u8; 16]> {
    let content = std::fs::read_to_string("/etc/machine-id")?;
    let decoded = hex::decode(content.trim()).map_err(|_| JournalError::UuidSerde)?;
    let bytes: [u8; 16] = decoded.try_into().map_err(|_| JournalError::UuidSerde)?;
    Ok(bytes)
}

#[cfg(target_os = "macos")]
pub fn load_machine_id() -> Result<[u8; 16]> {
    use std::process::Command;

    let output = Command::new("system_profiler")
        .arg("SPHardwareDataType")
        .output()
        .map_err(|_| JournalError::UuidSerde)?;

    if output.status.success() {
        let output_str = String::from_utf8_lossy(&output.stdout);
        for line in output_str.lines() {
            if line.contains("Hardware UUID:") {
                if let Some(uuid_str) = line.split("Hardware UUID:").nth(1) {
                    let uuid_str = uuid_str.trim();
                    let hex_str: String = uuid_str.chars().filter(|c| *c != '-').collect();

                    if hex_str.len() == 32 {
                        let mut bytes = [0u8; 16];
                        for i in 0..16 {
                            let hex_pair = &hex_str[i * 2..i * 2 + 2];
                            bytes[i] = u8::from_str_radix(hex_pair, 16)
                                .map_err(|_| JournalError::UuidSerde)?;
                        }
                        return Ok(bytes);
                    }
                }
            }
        }
    }

    Err(JournalError::UuidSerde)
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn load_machine_id() -> Result<[u8; 16]> {
    Err(JournalError::UuidSerde)
}

#[cfg(target_os = "linux")]
pub fn load_boot_id() -> Result<[u8; 16]> {
    let content = std::fs::read_to_string("/proc/sys/kernel/random/boot_id")?;

    let uuid_str = content.trim();
    let hex_str: String = uuid_str.chars().filter(|c| *c != '-').collect();

    if hex_str.len() != 32 {
        return Err(JournalError::UuidSerde);
    }

    let mut bytes = [0u8; 16];
    for i in 0..16 {
        let hex_pair = &hex_str[i * 2..i * 2 + 2];
        bytes[i] = u8::from_str_radix(hex_pair, 16).map_err(|_| JournalError::UuidSerde)?;
    }

    Ok(bytes)
}

#[cfg(target_os = "macos")]
pub fn load_boot_id() -> Result<[u8; 16]> {
    use std::process::Command;

    let output = Command::new("sysctl")
        .arg("-n")
        .arg("kern.boottime")
        .output()
        .map_err(|_| JournalError::UuidSerde)?;

    if output.status.success() {
        let output_str = String::from_utf8_lossy(&output.stdout);
        // Parse "{ sec = 1753988677, usec = 131097 } Thu Jul 31 22:04:37 2025"
        // Extract sec and usec values
        if let (Some(sec_start), Some(usec_start)) =
            (output_str.find("sec = "), output_str.find("usec = "))
        {
            let sec_str = &output_str[sec_start + 6..];
            let sec_end = sec_str.find(',').unwrap_or(sec_str.len());
            let sec_str = &sec_str[..sec_end].trim();

            let usec_str = &output_str[usec_start + 7..];
            let usec_end = usec_str.find(' ').unwrap_or(usec_str.len());
            let usec_str = &usec_str[..usec_end].trim();

            if let (Ok(sec), Ok(usec)) = (sec_str.parse::<u64>(), usec_str.parse::<u64>()) {
                // Create a deterministic UUID from boot time
                // Use sec in first 8 bytes, usec in next 4 bytes, pad remaining with zeros
                let mut bytes = [0u8; 16];
                bytes[0..8].copy_from_slice(&sec.to_be_bytes());
                bytes[8..12].copy_from_slice(&(usec as u32).to_be_bytes());
                // bytes[12..16] remain zero-filled for consistency
                return Ok(bytes);
            }
        }
    }

    Err(JournalError::UuidSerde)
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn load_boot_id() -> Result<[u8; 16]> {
    Err(JournalError::UuidSerde)
}

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
    machine_id: [u8; 16],
    boot_id: [u8; 16],
    seqnum_id: [u8; 16],
    file_id: [u8; 16],
    window_size: u64,
    data_hash_table_buckets: usize,
    field_hash_table_buckets: usize,
    enable_keyed_hash: bool,
}

impl JournalFileOptions {
    pub fn new(
        machine_id: [u8; 16],
        boot_id: [u8; 16],
        seqnum_id: [u8; 16],
        file_id: [u8; 16],
    ) -> Self {
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

    pub fn create<M: MemoryMapMut>(self, path: impl AsRef<Path>) -> Result<JournalFile<M>> {
        JournalFile::create(path, self)
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
        let data_hash_table_map = map_hash_table(
            &file,
            header.data_hash_table_offset,
            header.data_hash_table_size,
        )?;
        let field_hash_table_map = map_hash_table(
            &file,
            header.field_hash_table_offset,
            header.field_hash_table_size,
        )?;

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
        let head_offset = self.journal_header_ref().entry_array_offset?;
        let total_items =
            std::num::NonZeroUsize::new(self.journal_header_ref().n_entries as usize)?;
        Some(offset_array::List::new(head_offset, total_items))
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
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let header_slice = window_manager.get_slice(position.get(), size_needed)?;
        Ok(ObjectHeader::ref_from_bytes(header_slice).unwrap())
    }

    fn object_data_ref(&self, offset: NonZeroU64, size_needed: u64) -> Result<&[u8]> {
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let object_slice = window_manager.get_slice(offset.get(), size_needed)?;
        Ok(object_slice)
    }

    fn journal_object_ref<'a, T>(&'a self, offset: NonZeroU64) -> Result<ValueGuard<'a, T>>
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
            let header = self.object_header_ref(offset)?;
            header.size
        };

        let data = self.object_data_ref(offset, size_needed)?;
        let Some(value) = T::from_data(data, is_compact) else {
            return Err(JournalError::ZerocopyFailure);
        };

        // Mark as in use
        *is_in_use = true;

        Ok(ValueGuard::new(offset, value, &self.object_in_use))
    }

    pub fn offset_array_ref(
        &self,
        offset: NonZeroU64,
    ) -> Result<ValueGuard<OffsetArrayObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn field_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<FieldObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn entry_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<EntryObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn data_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<DataObject<&[u8]>>> {
        self.journal_object_ref(offset)
    }

    pub fn tag_ref(&self, offset: NonZeroU64) -> Result<ValueGuard<TagObject<&[u8]>>> {
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
}

impl<M: MemoryMapMut> JournalFile<M> {
    pub fn create(path: impl AsRef<Path>, options: JournalFileOptions) -> Result<Self> {
        let file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(&path)?;

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
        header.machine_id = options.machine_id;
        header.tail_entry_boot_id = options.boot_id;
        header.file_id = options.file_id;
        header.seqnum_id = options.seqnum_id;

        // Create memory maps for hash tables
        let data_hash_table_map = map_hash_table(
            &file,
            header.data_hash_table_offset,
            header.data_hash_table_size,
        )?;
        let field_hash_table_map = map_hash_table(
            &file,
            header.field_hash_table_offset,
            header.field_hash_table_size,
        )?;

        // Create header memory map and write header
        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let mut header_map = M::create(&file, 0, header_size)?;
        {
            let header_mut = JournalHeader::mut_from_prefix(&mut header_map).unwrap().0;
            *header_mut = header;
        }

        // Create window manager for the rest of the objects
        let window_manager = UnsafeCell::new(WindowManager::new(file, options.window_size, 32)?);

        let jf = JournalFile {
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
            object_in_use: RefCell::new(false),

            #[cfg(debug_assertions)]
            prev_backtrace: RefCell::new(Backtrace::capture()),
            #[cfg(debug_assertions)]
            backtrace: RefCell::new(Backtrace::capture()),
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

    fn object_header_mut(&self, offset: NonZeroU64) -> Result<&mut ObjectHeader> {
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let header_slice = window_manager.get_slice_mut(offset.get(), size_needed)?;
        Ok(ObjectHeader::mut_from_bytes(header_slice).unwrap())
    }

    fn object_data_mut(&self, offset: NonZeroU64, size_needed: u64) -> Result<&mut [u8]> {
        let window_manager = unsafe { &mut *self.window_manager.get() };
        let object_slice = window_manager.get_slice_mut(offset.get(), size_needed)?;
        Ok(object_slice)
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
                let header = self.object_header_mut(offset)?;
                header.type_ = type_ as u8;
                header.size = size;
                size
            }
            None => {
                let header = self.object_header_ref(offset)?;
                if header.type_ != type_ as u8 {
                    return Err(JournalError::InvalidObjectType);
                }
                header.size
            }
        };

        let data = self.object_data_mut(offset, size_needed)?;
        let value = T::from_data_mut(data, is_compact).ok_or(JournalError::ZerocopyFailure)?;

        // Mark as in use
        *is_in_use = true;
        Ok(ValueGuard::new(offset, value, &self.object_in_use))
    }

    pub fn offset_array_mut(
        &self,
        offset: NonZeroU64,
        capacity: Option<NonZeroU64>,
    ) -> Result<ValueGuard<OffsetArrayObject<&mut [u8]>>> {
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

        let offset_array = self.journal_object_mut(ObjectType::EntryArray, offset, size);
        offset_array
    }

    pub fn field_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<FieldObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<FieldObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Field, offset, size)
    }

    pub fn entry_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<EntryObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Entry, offset, size)
    }

    pub fn data_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<DataObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Data, offset, size)
    }

    pub fn tag_mut(
        &self,
        offset: NonZeroU64,
        new: bool,
    ) -> Result<ValueGuard<TagObject<&mut [u8]>>> {
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
