//! The mmap-backed journal object layer: [`JournalFile`] is the type every
//! other `file/*` module builds on. It opens or creates systemd-format
//! journal files (`LPKSHHRH` signature; layout in file/object.rs), keeps the
//! header and both hash tables in persistent memory maps, and reaches every
//! other object through a bounded set of mmap windows (file/mmap.rs).
//!
//! Object-offset validity (`journal_object_ref`/`_mut`): a journal object
//! lives at a non-zero, 8-byte-aligned offset at or past the header, whose
//! header-declared size fits the arena (`header_size + arena_size`). Two
//! deliberate exceptions: new objects are written with an explicit size and
//! may extend the file (`journal_object_mut`; `MemoryMapMut` grows it,
//! file/mmap.rs), and the hash-table maps bypass this path via
//! `map_hash_table` below.
//!
//! Concurrency: not `Sync` - the window manager sits in a `GuardedCell`
//! (file/guarded_cell.rs) that admits one object at a time, wrapped in a
//! `ValueGuard` (file/value_guard.rs) that releases the guard on drop.
//! Conflicting access fails with `JournalError::ValueGuardInUse` rather
//! than racing.
//!
//! Consumers: `JournalWriter` (file/writer.rs) writes through the `*_mut`
//! accessors; journal-log-writer creates files via [`JournalFileOptions`]
//! and rotates via `create_successor`; journal-index, journal-engine and
//! netflow-plugin read files via `open` and the typed accessors. Field-name
//! remappings (`ND_REMAPPING=1` entries) are parsed by `load_fields` below;
//! the shared registry type lives in field_map.rs.
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

// Objects are 8-byte aligned/padded (systemd journal format)
const OBJECT_ALIGNMENT: u64 = 8;

/// Rejects offsets that are not 8-byte aligned; `MisalignedOffset` carries the offset.
fn validate_offset_alignment(offset: NonZeroU64) -> Result<()> {
    if offset.get() % OBJECT_ALIGNMENT != 0 {
        return Err(JournalError::MisalignedOffset(offset.get()));
    }
    Ok(())
}

/// Visitor over one hash-bucket chain: `visit` is called for each object
/// from the bucket head; a `Some` output stops the walk (and becomes
/// `visit_bucket`'s result), `None` continues to the next link.
pub trait BucketVisitor<'a> {
    type Object: JournalObject<&'a [u8]> + HashableObject;
    type Output;

    /// Called for each object along the chain; returning `Some` stops the walk.
    fn visit(&mut self, object: &ValueGuard<'a, Self::Object>) -> Result<Option<Self::Output>>;
}

/// Bucket visitor used by `find_data_offset`/`find_field_offset`: matches a
/// chain object with the expected stored hash and raw payload bytes, and
/// returns its offset as the output.
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
        if object.hash() == self.hash && object.raw_payload() == self.payload {
            Ok(Some(object.offset()))
        } else {
            Ok(None)
        }
    }
}

/// Header configuration for a journal file, consumed only by `create`.
/// Defaults: 64 KiB windows, 4096 data / 512 field hash-table buckets,
/// keyed hash enabled. `file_id` is a fresh UUIDv4 per options value (not
/// settable); `boot_id` is stored in the header's `tail_entry_boot_id`.
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
    /// Header IDs for the new file; all other settings keep their defaults.
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

    /// Sizes the hash tables for a successor file. With utilization data:
    /// double a table above 75% occupancy, halve it below 25% (only while
    /// larger than the 4096/512 defaults), else keep it. Without: derive
    /// data buckets from `max_file_size` (default 8 MiB) and fix field
    /// buckets at 128.
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
            let field_buckets = 128; // Fixed; ~8:1 vs data only at the 8 MiB default floor

            (data_buckets, field_buckets)
        };

        self.data_hash_table_buckets = data_buckets;
        self.field_hash_table_buckets = field_buckets;
        self
    }

    /// Sets the journal's mmap window size; must be a multiple of 8 and of
    /// the 4096-byte page (asserted).
    pub fn with_window_size(mut self, size: u64) -> Self {
        assert_eq!(size % OBJECT_ALIGNMENT, 0);
        assert_eq!(size % 4096, 0, "Window size must be page-aligned");
        self.window_size = size;
        self
    }

    /// Bucket count; must be a power of two (asserted) - bucket lookup is
    /// `hash % buckets` (object.rs).
    pub fn with_data_hash_table_buckets(mut self, buckets: usize) -> Self {
        assert!(
            buckets.is_power_of_two(),
            "Hash table buckets should be a power of two"
        );
        self.data_hash_table_buckets = buckets;
        self
    }

    /// Bucket count; must be a power of two (asserted) - bucket lookup is
    /// `hash % buckets` (object.rs).
    pub fn with_field_hash_table_buckets(mut self, buckets: usize) -> Self {
        assert!(
            buckets.is_power_of_two(),
            "Hash table buckets should be a power of two"
        );
        self.field_hash_table_buckets = buckets;
        self
    }

    /// Sets the header's `KeyedHash` flag; see `JournalFile::hash`.
    pub fn with_keyed_hash(mut self, enabled: bool) -> Self {
        self.enable_keyed_hash = enabled;
        self
    }

    /// Convenience wrapper around `JournalFile::create`.
    pub fn create<M: MemoryMapMut>(self, file: &crate::repository::File) -> Result<JournalFile<M>> {
        JournalFile::create(file, self)
    }
}

/// Hash-table occupancy per table: `*_occupied` counts buckets with a
/// chain head. Produced by `JournalFile::bucket_utilization`, consumed by
/// `with_optimized_buckets`.
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

/// The in-memory handle for one journal file. Read-only callers use
/// `JournalFile<Mmap>` (`open`); writers use `JournalFile<MmapMut>`
/// (`create`), whose maps extend the file as objects are appended.
///
/// # Memory management
///
/// Only the file header and the two hash tables keep persistent maps; every
/// other object is reached through a bounded set of mmap windows (16 after
/// `open`, 32 after `create`) that the window manager remaps on demand
/// (file/mmap.rs), mirroring systemd's bounded-window model.
///
/// # Concurrency and safety
///
/// Interior mutability through a `GuardedCell` (file/guarded_cell.rs) that
/// owns the window manager and its guard flag. The guard admits exactly one
/// object at a time: object accessors return a `ValueGuard`
/// (file/value_guard.rs) that releases the guard on drop, and a second
/// concurrent or re-entrant access fails with
/// `JournalError::ValueGuardInUse`. The type is not `Sync` - use it from a
/// single thread (or move it while no guard is alive).
pub struct JournalFile<M: MemoryMap> {
    // Repository identity (path/status) this journal was opened/created for
    file: crate::repository::File,

    // Persistent maps: the file header and both hash tables (whole lifetime)
    header_map: M,
    data_hash_table_map: Option<M>,
    field_hash_table_map: Option<M>,

    // Window manager for every other object; owns the journal fd (`sync` goes through it)
    window_manager: GuardedCell<WindowManager<M>>,
}

/// Maps a hash table as the journal object it is stored as: the header
/// records the table payload's offset/size, and the map covers the payload
/// plus one `ObjectHeader` before it (`offset - sizeof::<ObjectHeader>()`
/// .. `offset + size`). `Ok(None)` when the header has no such table;
/// `JournalError::InvalidObjectLocation` when the payload offset does not
/// clear the `JournalHeader` region or the size does not exceed an
/// `ObjectHeader`.
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
    /// Walks one hash bucket's chain, calling `visitor` for each object
    /// until it returns `Some` (returned as the result) or the chain ends
    /// (`Ok(None)`). Errors `JournalError::MissingHashTable` on a `None`
    /// table. Only one object guard is live at a time while walking.
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

    /// Opens an existing journal file read-only. Validates only the
    /// `LPKSHHRH` signature (`JournalError::InvalidMagicNumber` otherwise) -
    /// state and the other header fields are not checked here. Maps the
    /// hash tables when the header points at them, then hands the opened fd
    /// to the window manager (16 windows max).
    pub fn open(file: &crate::repository::File, window_size: u64) -> Result<Self> {
        debug_assert_eq!(window_size % OBJECT_ALIGNMENT, 0);

        let fd = OpenOptions::new()
            .read(true)
            .write(false)
            .open(file.path())?;

        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let header_map = M::create(&fd, 0, header_size)?;
        let header = JournalHeader::ref_from_prefix(&header_map).unwrap().0;
        if header.signature != *b"LPKSHHRH" {
            return Err(JournalError::InvalidMagicNumber);
        }

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

    /// The file's hash for `data`: SipHash-2-4 keyed by the file's
    /// `file_id` when the header sets `KeyedHash`, else Jenkins lookup3 in
    /// systemd's word order (file/hash.rs). Keyed hashes are file-specific:
    /// a hash only compares within the file it was computed for.
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

    /// The journal's entry-array list covering all `n_entries` entries;
    /// `None` when the header has no `entry_array_offset` or zero entries
    /// (file/cursor.rs treats `None` as `InvalidOffsetArrayOffset`).
    pub fn entry_list(&self) -> Option<offset_array::List> {
        let header = self.journal_header_ref();

        header.entry_array_offset.and_then(|head_offset| {
            std::num::NonZeroUsize::new(header.n_entries as usize)
                .map(|total_items| offset_array::List::new(head_offset, total_items))
        })
    }

    /// Appends the offset of every entry in the entry-array chain to
    /// `offsets`, in array order; no-op on an empty or absent list.
    pub fn entry_offsets(&self, offsets: &mut Vec<NonZeroU64>) -> Result<()> {
        if let Some(entry_list) = self.entry_list() {
            entry_list.collect_offsets(self, offsets)?;
        }

        Ok(())
    }

    /// Appends the data-object offsets of the entry at `entry_offset` to
    /// `offsets`, in item order. A zero item offset fails with
    /// `JournalError::InvalidOffset` (EntryObject::collect_offsets,
    /// object.rs).
    pub fn entry_data_object_offsets(
        &self,
        entry_offset: NonZeroU64,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        let entry_guard = self.entry_ref(entry_offset)?;
        entry_guard.collect_offsets(offsets)
    }

    /// Borrows the header out of the persistent header map; the map is
    /// created with exactly `size_of::<JournalHeader>()`, so the parse
    /// cannot fail.
    pub fn journal_header_ref(&self) -> &JournalHeader {
        JournalHeader::ref_from_prefix(&self.header_map).unwrap().0
    }

    pub fn data_hash_table_map(&self) -> Option<&M> {
        self.data_hash_table_map.as_ref()
    }
    pub fn field_hash_table_map(&self) -> Option<&M> {
        self.field_hash_table_map.as_ref()
    }

    /// Typed view over the data hash-table map; `None` when the file has
    /// none or the bytes do not parse.
    pub fn data_hash_table_ref(&self) -> Option<DataHashTable<&[u8]>> {
        self.data_hash_table_map
            .as_ref()
            .and_then(|m| DataHashTable::<&[u8]>::from_data(m, false))
    }

    /// Typed view over the field hash-table map; `None` when the file has
    /// none or the bytes do not parse.
    pub fn field_hash_table_ref(&self) -> Option<FieldHashTable<&[u8]>> {
        self.field_hash_table_map
            .as_ref()
            .and_then(|m| FieldHashTable::<&[u8]>::from_data(m, false))
    }

    /// Reads one object header through the window manager (8-byte-aligned
    /// offsets only). Caution: the returned reference borrows the window
    /// manager without installing a guard - do not hold it across other
    /// `JournalFile` calls.
    pub fn object_header_ref(&self, position: NonZeroU64) -> Result<&ObjectHeader> {
        validate_offset_alignment(position)?;
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = self.window_manager.borrow_mut_checked()?;
        let header_slice = window_manager.get_slice(position.get(), size_needed)?;
        ObjectHeader::ref_from_bytes(header_slice).map_err(|_| JournalError::ZerocopyFailure)
    }

    /// Validates and parses the object at `offset` - 8-byte-aligned, not
    /// inside the file header, header-declared size inside the arena - and
    /// returns it as a `ValueGuard` holding the window-manager guard. The
    /// stored object type is not checked here; parse failures surface as
    /// `ZerocopyFailure`.
    fn journal_object_ref<'a, T>(&'a self, offset: NonZeroU64) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObject<&'a [u8]>,
    {
        let journal_header = self.journal_header_ref();
        let is_compact = journal_header.has_incompatible_flag(HeaderIncompatibleFlags::Compact);
        let header_size = journal_header.header_size;
        let arena_end = header_size + journal_header.arena_size;

        validate_offset_alignment(offset)?;

        // Objects cannot be located in the file header
        if offset.get() < header_size {
            return Err(JournalError::ObjectExceedsFileBounds);
        }

        self.window_manager.with_guarded(offset, |wm| {
            let size_needed = {
                let header_slice =
                    wm.get_slice(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                let header = ObjectHeader::ref_from_bytes(header_slice)
                    .map_err(|_| JournalError::ZerocopyFailure)?;
                header.validated_size()?
            };

            // Validate that the object doesn't exceed the journal's arena bounds
            let end_offset = offset
                .get()
                .checked_add(size_needed)
                .ok_or(JournalError::ObjectExceedsFileBounds)?;
            if end_offset > arena_end {
                return Err(JournalError::ObjectExceedsFileBounds);
            }

            let data = wm.get_slice(offset.get(), size_needed)?;

            let value = T::from_data(data, is_compact).ok_or(JournalError::ZerocopyFailure)?;

            Ok(value)
        })
    }

    // Typed object accessors: each validates and parses through
    // `journal_object_ref` and returns a `ValueGuard` holding the
    // window-manager guard; only one may be live at a time.
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

    /// Returns the data object whose stored hash and raw payload bytes
    /// match `hash`/`payload`, walking its hash bucket. Byte comparison is
    /// over stored bytes: a compressed data object matches only its
    /// compressed stream, not the plaintext. Errors `MissingHashTable`
    /// when the file has no data hash table.
    pub fn find_data_offset(&self, hash: u64, payload: &[u8]) -> Result<Option<NonZeroU64>> {
        let visitor = PayloadMatcher::data_matcher(payload, hash);
        self.visit_bucket(self.data_hash_table_ref(), hash, visitor)
    }

    /// Returns the field object whose stored hash and payload bytes match
    /// `hash`/`payload`; errors `MissingHashTable` when the file has no
    /// field hash table. `hash` must come from this file's `hash()`.
    pub fn find_field_offset(&self, hash: u64, payload: &[u8]) -> Result<Option<NonZeroU64>> {
        let visitor = PayloadMatcher::field_matcher(payload, hash);
        self.visit_bucket(self.field_hash_table_ref(), hash, visitor)
    }

    /// Runs a partition-point query over the entries referencing a data
    /// object: the first (`Forward`) or last (`Backward`) entry offset
    /// whose predicate holds. `Ok(None)` when the data object has no
    /// inlined entry cursor or nothing matches. No current caller in this
    /// crate tree (file/filter.rs keeps a commented-out reference).
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

    /// Iterates every field object in the field hash table: bucket order,
    /// then hash-chain order within each bucket. Yields nothing when the
    /// file has no field hash table; ends after the first read error.
    pub fn fields(&self) -> FieldIterator<'_, M> {
        let field_hash_table = self.field_hash_table_ref();

        let mut iterator = FieldIterator {
            journal: self,
            field_hash_table,
            current_bucket_index: 0,
            next_field_offset: None,
        };

        iterator.advance_to_next_nonempty_bucket();

        iterator
    }

    /// Builds the field-name compatibility map for this file: original
    /// (otel) name -> stored (systemd) journal name.
    ///
    /// Pass 1 - remapping entries: locates the data object whose payload is
    /// exactly `ND_REMAPPING=1` (duplicated literal; the constant lives in
    /// field_map.rs), walks every entry referencing it through the data
    /// object's inlined cursor, and parses each entry's other data payloads
    /// as `name=value` (first `=`), mapping `value` (original) -> `name`
    /// (stored). No name-shape gate: both `rdp::encode_full` normal names
    /// (`ND<checksum><structure>_<NAME>`) and MD5-fallback names (`ND_` +
    /// 32 hex) load - unlike `JournalReader::load_remappings`
    /// (file/reader.rs), which accepts only the 35-byte MD5-fallback shape
    /// and silently drops normal names; the two reader paths disagree. A
    /// payload without `=` fails with `InvalidField`; non-UTF-8 payloads
    /// panic (`expect`). The `_BOOT_ID=...` item journal-log-writer adds to
    /// every marker entry (journal-log-writer/src/log/mod.rs) is parsed
    /// like any mapping and contributes `<boot-id> -> _BOOT_ID`.
    ///
    /// Pass 2 - identity mappings: every field-table field whose payload
    /// does not start with `ND` (the marker and remapped names) is inserted
    /// as `name -> name`. This pass runs after pass 1 and overwrites on key
    /// collision (last wins - unlike `FieldMap::add_otel_mapping`, which
    /// keeps the first).
    ///
    /// Without a marker data object, only pass 2 runs. Errors: hash-table
    /// lookup failures propagate (`MissingHashTable`, ...); a marker data
    /// object with no referencing entries (no inlined cursor) fails with
    /// `EmptyInlineCursor`.
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
                        let payload = data_object.raw_payload();

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
                // No remapping entries: only the identity pass below runs
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
            let s = String::from_utf8(field.raw_payload().to_vec()).expect("utf8 data");
            field_map.insert(s.clone(), s);
        }

        Ok(field_map)
    }

    /// Iterates the DATA objects chained under `field_name` (the field
    /// object's `head_data_offset` list). The name is the stored (systemd)
    /// field name, hashed with this file's `hash()`. An unknown field
    /// yields an empty iterator, not an error.
    pub fn field_data_objects<'a>(
        &'a self,
        field_name: &'a [u8],
    ) -> Result<FieldDataIterator<'a, M>> {
        let field_hash = self.hash(field_name);
        let Some(field_offset) = self.find_field_offset(field_hash, field_name)? else {
            return Ok(FieldDataIterator {
                journal: self,
                current_data_offset: None,
            });
        };

        let field_guard = self.field_ref(field_offset)?;
        let head_data_offset = field_guard.header.head_data_offset;

        Ok(FieldDataIterator {
            journal: self,
            current_data_offset: head_data_offset,
        })
    }

    /// Iterates the DATA objects of the entry at `entry_offset`, in item
    /// order. A zero item offset yields `JournalError::InvalidOffset` and
    /// ends the iteration; any read error does the same - items are never
    /// skipped.
    pub fn entry_data_objects(&self, entry_offset: NonZeroU64) -> Result<EntryDataIterator<'_, M>> {
        let entry_guard = self.entry_ref(entry_offset)?;

        let total_items = match &entry_guard.items {
            EntryItemsType::Regular(items) => items.len(),
            EntryItemsType::Compact(items) => items.len(),
        };

        Ok(EntryDataIterator {
            journal: self,
            entry_offset: Some(entry_offset),
            current_index: 0,
            total_items,
        })
    }

    /// Occupied bucket counts per hash table (buckets with a chain head);
    /// `None` when either table is missing. Consumed by `create_successor`
    /// via `with_optimized_buckets`.
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

    /// Tail-minus-head entry realtime as a `Duration` (microseconds).
    /// `None` for an empty or single-entry journal, or when
    /// `tail_entry_realtime <= head_entry_realtime`.
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
    /// Persists the journal: msyncs the header map, then fdatasyncs the
    /// file through the fd the window manager owns. The hash-table maps and
    /// the windows reach disk only through that fdatasync.
    pub fn sync(&mut self) -> Result<()> {
        self.header_map.flush()?;

        let window_manager = self.window_manager.get_mut();
        window_manager.sync()?;

        Ok(())
    }

    /// Creates a fresh successor file for rotation: IDs copied from this
    /// header (machine, boot, seqnum; a NEW random `file_id`), bucket
    /// counts sized from this file's `bucket_utilization`, keyed-hash flag
    /// inherited, 8 MiB windows. The successor starts empty. Used by
    /// journal-log-writer on rotation (journal-log-writer/src/log/mod.rs).
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

    /// Creates a journal file at `file`'s path, truncating anything already
    /// there. Writes the header (`LPKSHHRH`, `KeyedHash` flag per options,
    /// hash-table locations/sizes, IDs, state ONLINE), stamps both
    /// hash-table object headers, then syncs. The arena spans the header
    /// through the end of the field hash table; `n_objects` counts the two
    /// tables. The window manager (32 windows) takes over the opened fd.
    pub fn create(file: &crate::repository::File, options: JournalFileOptions) -> Result<Self> {
        let fd = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(file.path())?;

        let data_hash_table_size =
            options.data_hash_table_buckets * std::mem::size_of::<HashItem>();
        let field_hash_table_size =
            options.field_hash_table_buckets * std::mem::size_of::<HashItem>();

        let data_hash_table_offset = std::mem::size_of::<JournalHeader>() as u64
            + std::mem::size_of::<ObjectHeader>() as u64;
        let field_hash_table_offset = data_hash_table_offset
            + data_hash_table_size as u64
            + std::mem::size_of::<ObjectHeader>() as u64;

        let mut header = JournalHeader::default();
        header.signature = *b"LPKSHHRH";

        if options.enable_keyed_hash {
            header.incompatible_flags |= HeaderIncompatibleFlags::KeyedHash as u32;
        }

        header.data_hash_table_offset = NonZeroU64::new(data_hash_table_offset);
        header.data_hash_table_size = NonZeroU64::new(data_hash_table_size as u64);
        header.field_hash_table_offset = NonZeroU64::new(field_hash_table_offset);
        header.field_hash_table_size = NonZeroU64::new(field_hash_table_size as u64);

        header.tail_object_offset =
            NonZeroU64::new(data_hash_table_offset + data_hash_table_size as u64);
        header.header_size = std::mem::size_of::<JournalHeader>() as u64;
        header.n_objects = 2;
        header.arena_size =
            field_hash_table_offset + field_hash_table_size as u64 - header.header_size;

        header.machine_id = *options.machine_id.as_bytes();
        header.tail_entry_boot_id = *options.boot_id.as_bytes();
        header.file_id = *options.file_id.as_bytes();
        header.seqnum_id = *options.seqnum_id.as_bytes();

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

        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let mut header_map = M::create(&fd, 0, header_size)?;
        {
            let header_mut = JournalHeader::mut_from_prefix(&mut header_map).unwrap().0;
            *header_mut = header;
            // ONLINE per the journal format; persisted by the sync below
            header_mut.state = JournalState::Online as u8;
        }

        let window_manager = GuardedCell::new(WindowManager::new(fd, options.window_size, 32)?);

        let mut jf = JournalFile {
            file: file.clone(),
            header_map,
            data_hash_table_map,
            field_hash_table_map,
            window_manager,
        };

        // Stamp the data hash table's object header (type + size)
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

        // Stamp the field hash table's object header (type + size)
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

        jf.sync()?;

        Ok(jf)
    }

    /// Mutable header access; writes land in the header map and reach disk
    /// only on `sync`.
    pub fn journal_header_mut(&mut self) -> &mut JournalHeader {
        JournalHeader::mut_from_prefix(&mut self.header_map)
            .unwrap()
            .0
    }

    /// Mutable typed view over the data hash-table map; `None` when the
    /// file has none or the bytes do not parse.
    pub fn data_hash_table_mut(&mut self) -> Option<DataHashTable<&mut [u8]>> {
        self.data_hash_table_map
            .as_mut()
            .and_then(|m| DataHashTable::<&mut [u8]>::from_data_mut(m, false))
    }

    /// Mutable typed view over the field hash-table map; `None` when the
    /// file has none or the bytes do not parse.
    pub fn field_hash_table_mut(&mut self) -> Option<FieldHashTable<&mut [u8]>> {
        self.field_hash_table_map
            .as_mut()
            .and_then(|m| FieldHashTable::<&mut [u8]>::from_data_mut(m, false))
    }

    /// Writes one object header through the window manager (8-byte-aligned
    /// offsets only). Like `object_header_ref`, the returned reference
    /// borrows the window manager without a guard; used briefly inside
    /// `create`.
    #[allow(clippy::mut_from_ref)]
    fn object_header_mut(&self, offset: NonZeroU64) -> Result<&mut ObjectHeader> {
        validate_offset_alignment(offset)?;
        let size_needed = std::mem::size_of::<ObjectHeader>() as u64;
        let window_manager = self.window_manager.borrow_mut_checked()?;
        let header_slice = window_manager.get_slice_mut(offset.get(), size_needed)?;
        ObjectHeader::mut_from_bytes(header_slice).map_err(|_| JournalError::ZerocopyFailure)
    }

    /// Opens or creates a mutable object view at `offset`. `Some(size)`
    /// initializes a new object header (type + size) with no bounds checks
    /// - the file is extended as needed (`MemoryMapMut::create`,
    /// file/mmap.rs). `None` reads the existing header and requires the
    /// stored type to match (`InvalidObjectType`), with the usual
    /// alignment/bounds validation.
    fn journal_object_mut<'a, T>(
        &'a self,
        type_: ObjectType,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'a, T>>
    where
        T: JournalObjectMut<&'a mut [u8]>,
    {
        validate_offset_alignment(offset)?;

        let journal_header = self.journal_header_ref();
        let is_compact = journal_header.has_incompatible_flag(HeaderIncompatibleFlags::Compact);
        let header_size = journal_header.header_size;
        let arena_end = header_size + journal_header.arena_size;

        // Objects cannot be located in the file header
        if offset.get() < header_size {
            return Err(JournalError::ObjectExceedsFileBounds);
        }

        self.window_manager.with_guarded(offset, |wm| {
            // Get or set the size
            let size_needed = match size {
                Some(size) => {
                    // Setting object header for a new object (no bounds check needed,
                    // the file will be extended as necessary)
                    let header_slice =
                        wm.get_slice_mut(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                    let header = ObjectHeader::mut_from_bytes(header_slice)
                        .map_err(|_| JournalError::ZerocopyFailure)?;
                    header.type_ = type_ as u8;
                    header.size = size;
                    size
                }
                None => {
                    // Reading existing object header
                    let header_slice =
                        wm.get_slice(offset.get(), std::mem::size_of::<ObjectHeader>() as u64)?;
                    let header = ObjectHeader::ref_from_bytes(header_slice)
                        .map_err(|_| JournalError::ZerocopyFailure)?;
                    if header.type_ != type_ as u8 {
                        return Err(JournalError::InvalidObjectType);
                    }
                    let size_needed = header.validated_size()?;

                    // Validate that the object doesn't exceed the journal's arena bounds
                    let end_offset = offset
                        .get()
                        .checked_add(size_needed)
                        .ok_or(JournalError::ObjectExceedsFileBounds)?;
                    if end_offset > arena_end {
                        return Err(JournalError::ObjectExceedsFileBounds);
                    }

                    size_needed
                }
            };

            let data = wm.get_slice_mut(offset.get(), size_needed)?;

            let value = T::from_data_mut(data, is_compact).ok_or(JournalError::ZerocopyFailure)?;

            Ok(value)
        })
    }

    /// Opens or creates an entry-array object at `offset`. `Some(capacity)`
    /// computes the on-disk size (`OffsetArrayObjectHeader` + `capacity`
    /// entries - u64 each, u32 under the Compact flag) and initializes the
    /// object; `None` opens the existing one.
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

    /// Opens or creates a field object; `Some(size)` is the payload length
    /// (a `FieldObjectHeader` is prepended).
    pub fn field_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, FieldObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<FieldObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Field, offset, size)
    }

    /// Opens or creates an entry object; `Some(size)` is the item-array
    /// length (a `DataObjectHeader` is prepended - entries share the
    /// data-object header layout).
    pub fn entry_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, EntryObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Entry, offset, size)
    }

    /// Opens or creates a data object; `Some(size)` is the payload length
    /// (a `DataObjectHeader` is prepended).
    pub fn data_mut(
        &self,
        offset: NonZeroU64,
        size: Option<u64>,
    ) -> Result<ValueGuard<'_, DataObject<&mut [u8]>>> {
        let size = size.map(|n| std::mem::size_of::<DataObjectHeader>() as u64 + n);
        self.journal_object_mut(ObjectType::Data, offset, size)
    }

    /// Opens or creates a tag object; `new` selects creation
    /// (`TagObjectHeader`-sized) vs open.
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

// Appends an object to its hash bucket chain: links the current tail to the
// new object (`set_next_hash_offset`) and updates the bucket's head (first
// insert) and tail. Errors `MissingHashTable` when the table is absent.
// Generated for the data and field tables below.
macro_rules! impl_hash_table_set_tail_offset {
    (
        $method_name:ident,
        $hash_table_ref:ident,
        $hash_table_mut:ident,
        $object_mut:ident
    ) => {
        /// Appends `object_offset` to the tail of the `hash` bucket chain.
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

/// Walks every field object in the field hash table: bucket order, then
/// hash-chain order within each bucket. Yields `Result` items; a read error
/// is yielded once and ends the iterator. Empty when the file has no field
/// hash table.
pub struct FieldIterator<'a, M: MemoryMap> {
    journal: &'a JournalFile<M>,
    field_hash_table: Option<FieldHashTable<&'a [u8]>>,
    current_bucket_index: usize,
    next_field_offset: Option<NonZeroU64>,
}

impl<M: MemoryMap> FieldIterator<'_, M> {
    /// Advances to the next non-empty bucket
    fn advance_to_next_nonempty_bucket(&mut self) {
        // No field hash table: nothing to iterate
        let Some(hash_table) = &self.field_hash_table else {
            return;
        };

        let items = &hash_table.items;

        while self.current_bucket_index < items.len() {
            let bucket = items[self.current_bucket_index];
            if bucket.head_hash_offset.is_some() {
                self.next_field_offset = bucket.head_hash_offset;
                return;
            }
            self.current_bucket_index += 1;
        }

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

/// Walks the DATA objects chained under one field (each data object's
/// `next_field_offset`). A read error is yielded once and ends the
/// iterator.
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

/// Walks the DATA items of one entry by index, re-reading the entry object
/// on each step. A zero item offset yields `JournalError::InvalidOffset`
/// and ends the iterator; other read errors do the same - items are never
/// skipped.
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

        if self.current_index >= self.total_items {
            return None;
        }

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

                let data_offset = match NonZeroU64::new(data_offset) {
                    Some(offset) => offset,
                    None => {
                        self.current_index = self.total_items;
                        return Some(Err(JournalError::InvalidOffset));
                    }
                };

                // Drop the entry guard before obtaining the data object
                drop(entry_guard);

                match self.journal.data_ref(data_offset) {
                    Ok(data_guard) => Some(Ok(data_guard)),
                    Err(e) => {
                        self.current_index = self.total_items;
                        Some(Err(e))
                    }
                }
            }
            Err(e) => {
                self.current_index = self.total_items;
                Some(Err(e))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::file::writer::JournalWriter;
    use tempfile::TempDir;

    fn test_uuid(seed: u8) -> uuid::Uuid {
        uuid::Uuid::from_bytes([seed; 16])
    }

    #[test]
    fn entry_data_iterator_reports_zero_offsets_as_invalid() {
        let dir = TempDir::new().expect("create temp dir");
        let journal_dir = dir.path().join("journals");
        std::fs::create_dir_all(&journal_dir).expect("create journal dir");
        let path = journal_dir.join("system.journal");
        let repo_file =
            crate::repository::File::from_path(&path).expect("test journal path should parse");

        let mut journal_file = JournalFile::create(
            &repo_file,
            JournalFileOptions::new(test_uuid(1), test_uuid(2), test_uuid(3)),
        )
        .expect("create journal");
        let mut writer =
            JournalWriter::new(&mut journal_file, 1, test_uuid(4)).expect("create writer");
        let payloads = [b"MESSAGE=test".as_slice(), b"PRIORITY=6".as_slice()];
        writer
            .add_entry(&mut journal_file, &payloads, 1_000_000, 100)
            .expect("write entry");

        let mut entry_offsets = Vec::new();
        journal_file
            .entry_offsets(&mut entry_offsets)
            .expect("collect entry offsets");
        let entry_offset = *entry_offsets.first().expect("journal entry");

        {
            let mut entry_guard = journal_file
                .entry_mut(entry_offset, None)
                .expect("entry guard");
            match &mut entry_guard.items {
                EntryItemsType::Regular(items) => items[0].object_offset = 0,
                EntryItemsType::Compact(items) => items[0].object_offset = 0,
            }
        }

        let mut iter = journal_file
            .entry_data_objects(entry_offset)
            .expect("entry iterator");
        assert!(matches!(
            iter.next(),
            Some(Err(JournalError::InvalidOffset))
        ));
        assert!(
            iter.next().is_none(),
            "iterator should stop after the error"
        );
    }
}
