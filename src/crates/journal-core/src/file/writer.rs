//! The journal file's write side: [`JournalWriter`] turns entry items into
//! the on-disk object graph - data objects, field objects, the file-level
//! entry array and the per-data entry arrays - appending to an existing
//! [`JournalFile<MmapMut>`] (file/file.rs) through its `*_mut` accessors.
//! Creating the file is not this file's job: [`JournalFile::create`]
//! lays out the header and hash tables and
//! [`JournalFile::create_successor`] builds a rotation
//! successor; a writer is then placed on top with [`JournalWriter::new`],
//! and `add_entry` is the sole write path.
//!
//! Everything is appended at one strictly growing `append_offset` - the end
//! of the last written object - so all object and entry offsets share a
//! single file-global, write-ordered space. That is what binary search over
//! a data object's entry chain
//! ([`offset_array::Cursor`](crate::file::offset_array::Cursor)), cursor
//! seeking ([`JournalCursor`](crate::file::cursor::JournalCursor)) and the
//! filter's cross-chain offset comparisons
//! rely on (file/filter.rs).
//!
//! Identical payloads are deduplicated: the in-memory caches below first,
//! then the file's hash tables ([`JournalFile::find_data_offset`] /
//! [`JournalFile::find_field_offset`]); only a true miss
//! appends a new object.
//!
//! Concurrency and durability: the writer holds no locks and no interior
//! mutability - every mutating method takes `&mut self` - and the
//! `JournalFile` it drives admits one object view at a time
//! (the `GuardedCell` in-use flag) and is not `Sync` (its window manager
//! sits in an `UnsafeCell`), so the pair is used from a
//! single thread. The writer never syncs; appends reach disk only through
//! [`JournalFile::sync`], which journal-log-writer calls
//! per batch, on rotation and on drop
//! (journal-log-writer/src/log/mod.rs).
//!
//! Consumers (grep-verified): the only non-test consumer is
//! journal-log-writer (journal-log-writer/src/log/mod.rs); everything else
//! builds test fixtures (journal-core's
//! reader/file test modules, journal-index, journal-engine). netflow-plugin
//! drives the published sdk twin's `journal_sdk_core::JournalWriter`
//! (its tests in src/crates/netflow-plugin/src/facet_runtime.rs), and the
//! in-tree jf twin has its own same-named writer
//! (src/crates/jf/journal_file/src/writer.rs) - neither is this type.
// The flat `crate::file::{...}` import below pulls in the whole object
// vocabulary; most names go unused in this file - hence the blanket allow.
#![allow(unused_imports, dead_code)]

use super::mmap::MemoryMapMut;
use super::mmap::MmapMut;
use crate::error::{JournalError, Result};
use crate::file::{
    CompactEntryItem, DataHashTable, DataObject, DataObjectHeader, DataPayloadType, EntryObject,
    EntryObjectHeader, FieldHashTable, FieldObject, FieldObjectHeader, HashItem, HashTable,
    HashTableMut, HashableObject, HashableObjectMut, HeaderIncompatibleFlags, JournalFile,
    JournalFileOptions, JournalHeader, JournalState, ObjectHeader, ObjectType, RegularEntryItem,
    hash::jenkins_hash64, journal_hash_data,
};
use rand::{Rng, seq::IndexedRandom};
use rustc_hash::{FxHashMap, FxHasher};
use std::hash::Hasher;
use std::num::{NonZeroU64, NonZeroUsize};
use std::path::Path;
use zerocopy::{FromBytes, IntoBytes};

// Vestigial copy of file.rs's alignment constant; nothing below reads it -
// alignment is enforced by the accessors (`validate_offset_alignment` in
// file/file.rs)
const OBJECT_ALIGNMENT: u64 = 8;
const FIELD_CACHE_MAX_ENTRIES: usize = 1024;
const FIELD_CACHE_MAX_PAYLOAD_LEN: usize = 128;
const RECENT_DATA_CACHE_SLOTS: usize = 4096;
const RECENT_DATA_CACHE_MAX_PAYLOAD_LEN: usize = 256;

/// One resolved entry item: a data object's offset plus the file-specific
/// hash computed for its payload - the pair written into the entry's item
/// array and reused by the dedup caches.
#[derive(Debug, Clone, Copy)]
struct EntryItem {
    offset: NonZeroU64,
    hash: u64,
}

/// Exact-byte cache of known field names: payload -> field-object offset.
/// Payloads longer than `FIELD_CACHE_MAX_PAYLOAD_LEN` are not cached, and
/// once `FIELD_CACHE_MAX_ENTRIES` entries are held the next insert of a
/// new name clears the whole map (both pinned by the tests below).
#[derive(Debug)]
struct FieldCache {
    entries: FxHashMap<Box<[u8]>, NonZeroU64>,
}

impl FieldCache {
    fn new() -> Self {
        Self {
            entries: FxHashMap::default(),
        }
    }

    fn get(&self, payload: &[u8]) -> Option<NonZeroU64> {
        self.entries.get(payload).copied()
    }

    fn insert(&mut self, payload: &[u8], offset: NonZeroU64) {
        if payload.len() > FIELD_CACHE_MAX_PAYLOAD_LEN {
            return;
        }

        if self.entries.len() >= FIELD_CACHE_MAX_ENTRIES && self.entries.get(payload).is_none() {
            self.entries.clear();
        }

        self.entries
            .insert(payload.to_vec().into_boxed_slice(), offset);
    }

    #[cfg(test)]
    fn len(&self) -> usize {
        self.entries.len()
    }
}

/// One `RecentDataCache` slot: the cached payload bytes and the
/// `EntryItem` they resolved to.
#[derive(Debug, Clone)]
struct RecentDataCacheEntry {
    payload: Box<[u8]>,
    item: EntryItem,
}

/// One-slot-per-bucket cache of recently resolved data payloads: the slot
/// is `FxHasher(payload) & (slot count - 1)` and remembers only the last
/// payload hashed there. A colliding payload overwrites the slot and `get`
/// re-checks the bytes, so a collision is a miss, never a false hit.
/// Payloads longer than `RECENT_DATA_CACHE_MAX_PAYLOAD_LEN` bypass the
/// cache. Sits in front of the hash-table walk in `add_data` for the hot
/// repeat-payload case.
#[derive(Debug)]
struct RecentDataCache {
    entries: Box<[Option<RecentDataCacheEntry>]>,
}

impl RecentDataCache {
    fn new() -> Self {
        debug_assert!(RECENT_DATA_CACHE_SLOTS.is_power_of_two());
        let entries = std::iter::repeat_with(|| None)
            .take(RECENT_DATA_CACHE_SLOTS)
            .collect::<Vec<_>>()
            .into_boxed_slice();
        Self { entries }
    }

    fn get(&self, payload: &[u8]) -> Option<EntryItem> {
        let entry = self.entries[self.slot(payload)].as_ref()?;
        (entry.payload.as_ref() == payload).then_some(entry.item)
    }

    fn insert(&mut self, payload: &[u8], item: EntryItem) {
        if payload.len() > RECENT_DATA_CACHE_MAX_PAYLOAD_LEN {
            return;
        }

        self.entries[self.slot(payload)] = Some(RecentDataCacheEntry {
            payload: payload.to_vec().into_boxed_slice(),
            item,
        });
    }

    fn slot(&self, payload: &[u8]) -> usize {
        let mut hasher = FxHasher::default();
        hasher.write(payload);
        (hasher.finish() as usize) & (self.entries.len() - 1)
    }
}

/// Append-side state for one journal file; all offsets are non-zero and
/// 8-byte aligned.
pub struct JournalWriter {
    // Last object written (entry, data, field or offset array); committed
    // to the header's `tail_object_offset` per entry in `entry_added`
    tail_object_offset: NonZeroU64,
    // Append cursor: where the next object will be written; advanced by
    // `object_added`
    append_offset: NonZeroU64,
    // Sequence number of the next entry; bumped in `entry_added`
    next_seqnum: u64,
    // Objects written since the last entry; flushed into the header's
    // `n_objects` per entry, so an interrupted write never counts
    num_written_objects: u64,
    // Item resolutions of the entry being assembled; offset-sorted and
    // deduplicated before the entry object is written
    entry_items: Vec<EntryItem>,
    // Dedup caches behind `add_field` / `add_data` (docs at their types)
    field_cache: FieldCache,
    recent_data_cache: RecentDataCache,
    // Monotonic timestamp of this writer's first entry; in-memory only -
    // no header field stores it
    first_entry_monotonic: Option<u64>,
    // Boot id stamped into every entry header and the header's
    // `tail_entry_boot_id`
    boot_id: uuid::Uuid,
}

impl JournalWriter {
    /// The written arena's end - where the next object appends, and what
    /// `header_size + arena_size` add up to. Not the on-disk length: mmap
    /// windows map in whole chunks and the writable
    /// `MemoryMapMut`'s `create`
    /// extends the file past this. Rotation sizing reads it as the size
    /// signal (journal-log-writer/src/log/mod.rs).
    pub fn current_file_size(&self) -> u64 {
        self.append_offset.get()
    }

    /// Monotonic timestamp of the first entry written through this writer;
    /// `None` until the first `add_entry` lands, and still `None` for a
    /// writer resumed on a file that already has entries. In-memory only -
    /// the header has no head-entry-monotonic field -
    /// and nothing outside this file reads it today.
    pub fn first_entry_monotonic(&self) -> Option<u64> {
        self.first_entry_monotonic
    }

    /// Sequence number the next `add_entry` will write; incremented after
    /// each entry. journal-log-writer carries it into the successor file
    /// on rotation (journal-log-writer/src/log/mod.rs).
    pub fn next_seqnum(&self) -> u64 {
        self.next_seqnum
    }

    /// Boot id stamped into every entry header and the header's
    /// `tail_entry_boot_id` (a `JournalHeader` field); carried across
    /// rotation (journal-log-writer/src/log/mod.rs).
    pub fn boot_id(&self) -> uuid::Uuid {
        self.boot_id
    }

    /// Builds a writer on an already-opened journal file, resuming appends
    /// where the tail object ends: `append_offset = tail_object_offset +
    /// tail object size`. The tail object is usually an offset array -
    /// arrays are allocated after the entry they record - so the tail's
    /// stored size, not a fixed layout, decides where writing continues.
    /// Errors [`JournalError::InvalidMagicNumber`] when the header has no
    /// `tail_object_offset` - the variant name does not describe this case
    /// (see the variant's docs in `error.rs`). Unlike the jf twin
    /// (src/crates/jf/journal_file/src/writer.rs), `next_seqnum` and
    /// `boot_id` are caller-supplied rather than derived from the header.
    pub fn new(
        journal_file: &mut JournalFile<MmapMut>,
        next_seqnum: u64,
        boot_id: uuid::Uuid,
    ) -> Result<Self> {
        let append_offset = {
            let header = journal_file.journal_header_ref();

            let Some(tail_object_offset) = header.tail_object_offset else {
                return Err(JournalError::InvalidMagicNumber);
            };

            let tail_object = journal_file.object_header_ref(tail_object_offset)?;

            tail_object_offset.saturating_add(tail_object.size)
        };

        Ok(Self {
            // None cannot recur here: the early return above handled it
            tail_object_offset: journal_file
                .journal_header_ref()
                .tail_object_offset
                .unwrap(),
            append_offset,
            next_seqnum,
            num_written_objects: 0,
            entry_items: Vec::with_capacity(128),
            field_cache: FieldCache::new(),
            recent_data_cache: RecentDataCache::new(),
            first_entry_monotonic: None,
            boot_id,
        })
    }

    /// Writer state for a successor file: a fresh writer on `journal_file`
    /// carrying this writer's `next_seqnum` and `boot_id`; caches, the
    /// item buffer and `first_entry_monotonic` start empty. Making the
    /// successor FILE is [`JournalFile::create_successor`]'s job;
    /// journal-log-writer pairs the two with plain
    /// [`JournalWriter::new`] (journal-log-writer/src/log/mod.rs)
    /// and nothing calls this method today.
    pub fn create_successor(&self, journal_file: &mut JournalFile<MmapMut>) -> Result<Self> {
        Self::new(journal_file, self.next_seqnum, self.boot_id)
    }

    /// Writes one entry and all its bookkeeping: resolves every item to a
    /// data object (`add_data`), appends the entry object at the arena
    /// end, records it in the file-level entry array, links every data
    /// object back to it, then commits the header's entry fields
    /// (`entry_added`).
    ///
    /// The header's [`HeaderIncompatibleFlags::KeyedHash`] flag is asserted,
    /// not handled: this writer only appends to keyed-hash files, which is
    /// what [`JournalFileOptions::new`]'s defaults set and what every
    /// producer in this tree enables. A failed call leaves the objects it
    /// already wrote on disk - reachable through the hash tables, since
    /// `add_data` registers them as it goes - but never registers an
    /// entry: the header's entry-level fields commit only in
    /// `entry_added`.
    pub fn add_entry(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        items: &[&[u8]],
        realtime: u64,
        monotonic: u64,
    ) -> Result<()> {
        let header = journal_file.journal_header_ref();
        assert!(header.has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash));

        // Resolve every item to a data object (dedup happens inside
        // `add_data`), accumulating the entry's xor-hash
        let mut xor_hash = 0;
        {
            self.entry_items.clear();
            for payload in items {
                let entry_item = self.add_data(journal_file, payload)?;
                self.entry_items.push(entry_item);

                // Format rule: the xor-hash always uses unkeyed Jenkins
                // lookup3, even for files hashed with the keyed variant
                // (systemd's format rule; see file/hash.rs)
                xor_hash ^= jenkins_hash64(payload);
            }

            // Items are stored offset-ascending and deduplicated: the same
            // payload twice in one entry references its data object once
            self.entry_items
                .sort_unstable_by(|a, b| a.offset.cmp(&b.offset));
            self.entry_items.dedup_by(|a, b| a.offset == b.offset);
        }

        // Append the entry object at the arena end
        let entry_offset = self.append_offset;
        let entry_size = {
            // Regular item layout: u64 offset + u64 hash per slot
            // (the `RegularEntryItem` layout); no producer in this tree
            // sets the Compact flag that would shrink items to u32 offsets
            let size = Some(self.entry_items.len() as u64 * 16);
            let mut entry_guard = journal_file.entry_mut(entry_offset, size)?;

            entry_guard.header.seqnum = self.next_seqnum;
            entry_guard.header.xor_hash = xor_hash;
            entry_guard.header.boot_id = *self.boot_id.as_bytes();
            entry_guard.header.monotonic = monotonic;
            entry_guard.header.realtime = realtime;

            // Stamp the item array: offset + hash per slot
            for (index, entry_item) in self.entry_items.iter().enumerate() {
                entry_guard
                    .items
                    .set(index, entry_item.offset, Some(entry_item.hash));
            }

            entry_guard.header.object_header.aligned_size()
        };
        self.object_added(journal_file, entry_offset, entry_size);

        // Record the entry in the file-level entry-array chain
        self.append_to_entry_array(journal_file, entry_offset)?;
        // Back-link every data object to this entry (its per-data chain)
        for entry_item_index in 0..self.entry_items.len() {
            self.link_data_to_entry(journal_file, entry_offset, entry_item_index)?;
        }

        // Commit the header's entry-level fields
        self.entry_added(journal_file.journal_header_mut(), realtime, monotonic);

        Ok(())
    }

    /// Advances the write cursor past a freshly written object: the
    /// writer's tail and `append_offset`, the live object counter, and the
    /// header's `arena_size` - that last one immediately, so the
    /// bounds-checked re-opens later in the same write (`data_mut` /
    /// `field_ref` with no size) accept the object just
    /// written. Entry-level header fields wait for `entry_added`.
    fn object_added(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        object_offset: NonZeroU64,
        object_size: u64,
    ) {
        self.tail_object_offset = object_offset;
        self.append_offset = object_offset.saturating_add(object_size);
        self.num_written_objects += 1;

        // Immediate, not deferred to `entry_added`: in-write re-opens
        // bounds-check against it
        let header = journal_file.journal_header_mut();
        header.arena_size = self.append_offset.get() - header.header_size;
    }

    /// Commits one entry into the header: `n_entries` and `n_objects`
    /// (the objects written since the last entry), `tail_object_offset`
    /// (usually the last offset array allocated for the entry, not the
    /// entry itself), and the tail-entry fields - seqnum, realtime,
    /// monotonic and boot id - on every entry, while the head-entry fields
    /// are stamped only while they are still zero (the file's first
    /// entry). Also records `first_entry_monotonic`, re-stamps
    /// `arena_size` (already current via `object_added`), bumps
    /// `next_seqnum` and resets the object counter.
    fn entry_added(&mut self, header: &mut JournalHeader, realtime: u64, monotonic: u64) {
        header.n_entries += 1;
        header.n_objects += self.num_written_objects;
        header.tail_object_offset = Some(self.tail_object_offset);
        header.arena_size = self.append_offset.get() - header.header_size;

        if header.head_entry_seqnum == 0 {
            header.head_entry_seqnum = self.next_seqnum;
        }
        if header.head_entry_realtime == 0 {
            header.head_entry_realtime = realtime;
        }
        if self.first_entry_monotonic.is_none() {
            self.first_entry_monotonic = Some(monotonic);
        }

        header.tail_entry_seqnum = self.next_seqnum;
        header.tail_entry_realtime = realtime;
        header.tail_entry_monotonic = monotonic;
        header.tail_entry_boot_id = *self.boot_id.as_bytes();

        self.next_seqnum += 1;
        self.num_written_objects = 0;
    }

    /// Resolves one item's payload to a data object and returns its offset
    /// and file-specific hash ([`JournalFile::hash`]).
    ///
    /// Dedup order: the recent-payload cache, then the data hash table
    /// ([`JournalFile::find_data_offset`]: hash-bucket walk plus raw-byte
    /// comparison). On a miss the object is appended at the arena
    /// end and registered in its hash bucket
    /// (`data_hash_table_set_tail_offset`); a
    /// `NAME=VALUE` payload also resolves its field object (`add_field`)
    /// and prepends the data object to that field's `head_data_offset`
    /// chain. Both hit and miss feed the recent cache.
    fn add_data(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        payload: &[u8],
    ) -> Result<EntryItem> {
        if let Some(entry_item) = self.recent_data_cache.get(payload) {
            return Ok(entry_item);
        }

        let hash = journal_file.hash(payload);

        match journal_file.find_data_offset(hash, payload)? {
            Some(data_offset) => {
                let entry_item = EntryItem {
                    offset: data_offset,
                    hash,
                };
                self.recent_data_cache.insert(payload, entry_item);
                Ok(entry_item)
            }
            None => {
                // Miss: append a fresh data object at the arena end
                let data_offset = self.append_offset;
                let data_size = {
                    let mut data_guard =
                        journal_file.data_mut(data_offset, Some(payload.len() as u64))?;

                    data_guard.header.hash = hash;
                    data_guard.set_payload(payload);
                    data_guard.header.object_header.aligned_size()
                };

                self.object_added(journal_file, data_offset, data_size);

                // Register the object in its hash bucket's chain
                journal_file.data_hash_table_set_tail_offset(hash, data_offset)?;

                // NAME=VALUE payloads also maintain a field object for the
                // name part
                if let Some(equals_pos) = payload.iter().position(|&b| b == b'=') {
                    let field_offset = self.add_field(journal_file, &payload[..equals_pos])?;

                    // Prepend the data object to the field's data chain
                    {
                        let head_data_offset = {
                            let field_guard = journal_file.field_ref(field_offset)?;
                            field_guard.header.head_data_offset
                        };

                        let mut data_guard = journal_file.data_mut(data_offset, None)?;
                        data_guard.header.next_field_offset = head_data_offset;
                    }

                    // Point the field at its new chain head
                    {
                        let mut field_guard = journal_file.field_mut(field_offset, None)?;
                        field_guard.header.head_data_offset = Some(data_offset);
                    }
                }

                let entry_item = EntryItem {
                    offset: data_offset,
                    hash,
                };
                self.recent_data_cache.insert(payload, entry_item);
                Ok(entry_item)
            }
        }
    }

    /// Resolves a field name (the bytes before the first `=`) to its field
    /// object: field-cache hit, else the field hash table
    /// ([`JournalFile::find_field_offset`]), else append a fresh object at
    /// the arena end and register it in its hash bucket
    /// (`field_hash_table_set_tail_offset`). Returns the
    /// field object's offset.
    fn add_field(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        payload: &[u8],
    ) -> Result<NonZeroU64> {
        if let Some(field_offset) = self.field_cache.get(payload) {
            return Ok(field_offset);
        }

        let hash = journal_file.hash(payload);

        match journal_file.find_field_offset(hash, payload)? {
            Some(field_offset) => {
                self.field_cache.insert(payload, field_offset);
                Ok(field_offset)
            }
            None => {
                // Miss: append a fresh field object at the arena end
                let field_offset = self.append_offset;
                let field_size = {
                    let mut field_guard =
                        journal_file.field_mut(field_offset, Some(payload.len() as u64))?;

                    field_guard.header.hash = hash;
                    field_guard.set_payload(payload);
                    field_guard.header.object_header.aligned_size()
                };
                self.object_added(journal_file, field_offset, field_size);

                // Register the object in its hash bucket's chain
                journal_file.field_hash_table_set_tail_offset(hash, field_offset)?;

                self.field_cache.insert(payload, field_offset);

                Ok(field_offset)
            }
        }
    }

    /// Appends a fresh offset-array object at the arena end with
    /// `capacity` slots (on-disk size computed by
    /// [`JournalFile::offset_array_mut`]) and returns
    /// its offset. Callers fill and link it themselves.
    fn allocate_new_array(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        capacity: NonZeroU64,
    ) -> Result<NonZeroU64> {
        // let new_capacity = previous_capacity.saturating_mul(NonZeroU64::new(2).unwrap());

        let array_offset = self.append_offset;
        let array_size = {
            let array_guard = journal_file.offset_array_mut(array_offset, Some(capacity))?;

            array_guard.header.object_header.aligned_size()
        };
        self.object_added(journal_file, array_offset, array_size);

        Ok(array_offset)
    }

    /// Records one entry offset in the file-level entry-array chain rooted
    /// at the header's `entry_array_offset`, one call per written entry:
    /// the first entry allocates the initial 4096-slot array; later ones
    /// fill the chain's tail array or, when it is full, allocate a new
    /// array at double capacity and link it via `next_offset_array`.
    /// Errors `EmptyOffsetArrayList` when the header points at an array
    /// but [`JournalFile::entry_list`] cannot walk it.
    fn append_to_entry_array(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        entry_offset: NonZeroU64,
    ) -> Result<()> {
        let entry_array_offset = journal_file.journal_header_ref().entry_array_offset;

        if entry_array_offset.is_none() {
            journal_file.journal_header_mut().entry_array_offset = {
                let array_offset =
                    self.allocate_new_array(journal_file, NonZeroU64::new(4096).unwrap())?;
                let mut array_guard = journal_file.offset_array_mut(array_offset, None)?;
                array_guard.set(0, entry_offset)?;
                Some(array_offset)
            };
        } else {
            let tail_node = {
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::EmptyOffsetArrayList)?;
                entry_list.tail(journal_file)?
            };

            if tail_node.len() < tail_node.capacity() {
                let mut array_guard = journal_file.offset_array_mut(tail_node.offset(), None)?;
                array_guard.set(tail_node.len().get(), entry_offset)?;
            } else {
                let new_array_offset = {
                    let new_capacity = tail_node.capacity().get().saturating_mul(2) as u64;
                    let new_array_offset = self
                        .allocate_new_array(journal_file, NonZeroU64::new(new_capacity).unwrap())?;
                    let mut array_guard = journal_file.offset_array_mut(new_array_offset, None)?;
                    array_guard.set(0, entry_offset)?;

                    new_array_offset
                };

                // Link the old tail to the new array
                {
                    let mut array_guard =
                        journal_file.offset_array_mut(tail_node.offset(), None)?;
                    array_guard.header.next_offset_array = Some(new_array_offset);
                }
            }
        }

        Ok(())
    }

    /// Appends an entry offset to one data object's entry-array chain.
    /// `array_offset` is the chain head; `current_count` is how many items
    /// the chain already holds (`n_entries - 1`, see `link_data_to_entry`).
    /// Walks the `next_offset_array` chain to the array covering the count
    /// and sets the next slot - or, when that tail array is full, appends
    /// a new array at double capacity and links it.
    fn append_to_data_entry_array(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        mut array_offset: NonZeroU64,
        entry_offset: NonZeroU64,
        current_count: u64,
    ) -> Result<()> {
        // Navigate to the tail of the array chain
        let mut current_index = 0u64;
        #[allow(unused_assignments)]
        let mut tail_offset = array_offset;

        loop {
            let array_guard = journal_file.offset_array_ref(array_offset)?;
            let capacity = array_guard.capacity() as u64;

            if current_index + capacity >= current_count {
                // This is the tail array
                tail_offset = array_offset;
                break;
            }

            current_index += capacity;

            let Some(next_offset) = array_guard.header.next_offset_array else {
                // The chain ends before the count is covered: corrupt
                // bookkeeping
                return Err(JournalError::InvalidOffsetArrayOffset);
            };

            array_offset = next_offset;
        }

        // The tail array's free room decides append vs grow
        let tail_capacity = {
            let tail_guard = journal_file.offset_array_ref(tail_offset)?;
            tail_guard.capacity() as u64
        };

        let entries_in_tail = current_count - current_index;

        if entries_in_tail < tail_capacity {
            // There's space in the tail array
            let mut tail_guard = journal_file.offset_array_mut(tail_offset, None)?;
            tail_guard.set(entries_in_tail as usize, entry_offset)?;
        } else {
            // Need to create a new array
            let new_capacity = NonZeroU64::new(tail_capacity * 2).unwrap(); // Double the size
            let new_array_offset = self.allocate_new_array(journal_file, new_capacity)?;

            // Link the old tail to the new array
            let mut tail_guard = journal_file.offset_array_mut(tail_offset, None)?;
            tail_guard.header.next_offset_array = Some(new_array_offset);
            drop(tail_guard);

            // Add entry to the new array
            let mut new_array_guard = journal_file.offset_array_mut(new_array_offset, None)?;
            new_array_guard.set(0, entry_offset)?;
        }

        Ok(())
    }

    /// Links one entry into a data object's entry chain - the write-side
    /// mirror of `DataObjectHeader::inlined_cursor`. The
    /// first referencing entry is inlined in the data object
    /// (`entry_offset`, `n_entries = 1`); the second allocates a 64-slot
    /// array that holds the second entry while the first stays inlined;
    /// every later entry appends to the array chain. `n_entries` therefore
    /// always counts the one inlined entry plus the array items, and
    /// chains stay write-ordered because entry offsets only grow.
    fn link_data_to_entry(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        entry_offset: NonZeroU64,
        entry_item_index: usize,
    ) -> Result<()> {
        let data_offset = self.entry_items[entry_item_index].offset;
        let mut data_guard = journal_file.data_mut(data_offset, None)?;

        match data_guard.header.n_entries {
            None => {
                data_guard.header.entry_offset = Some(entry_offset);
                data_guard.header.n_entries = NonZeroU64::new(1);
            }
            Some(n_entries) => {
                match n_entries.get() {
                    0 => {
                        // NonZeroU64: a zero count cannot be stored
                        unreachable!();
                    }
                    1 => {
                        drop(data_guard);

                        // Create new entry array with initial capacity
                        let array_capacity = NonZeroU64::new(64).unwrap();
                        let array_offset = self.allocate_new_array(journal_file, array_capacity)?;

                        // Load new array and set its first entry offset
                        {
                            let mut array_guard =
                                journal_file.offset_array_mut(array_offset, None)?;
                            array_guard.set(0, entry_offset)?;
                        }

                        // Update data object to point to the array
                        let mut data_guard = journal_file.data_mut(data_offset, None)?;
                        data_guard.header.entry_array_offset = Some(array_offset);
                        data_guard.header.n_entries = NonZeroU64::new(2);
                    }
                    x => {
                        // There's already an entry array, append to it
                        let current_count = x - 1;
                        let array_offset = data_guard.header.entry_array_offset.unwrap();

                        // The data guard holds the file's one object view;
                        // drop it before the array calls below, then re-open
                        drop(data_guard);

                        // Find the tail of the entry array chain and append
                        self.append_to_data_entry_array(
                            journal_file,
                            array_offset,
                            entry_offset,
                            current_count,
                        )?;

                        // Re-open the data object and bump the reference
                        // count (one inlined entry + the array items)
                        let mut data_guard = journal_file.data_mut(data_offset, None)?;
                        data_guard.header.n_entries = NonZeroU64::new(x + 1);
                    }
                }
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        EntryItem, FIELD_CACHE_MAX_ENTRIES, FIELD_CACHE_MAX_PAYLOAD_LEN, FieldCache,
        RECENT_DATA_CACHE_MAX_PAYLOAD_LEN, RecentDataCache,
    };
    use std::num::NonZeroU64;

    #[test]
    fn field_cache_hits_exact_field_names() {
        let mut cache = FieldCache::new();
        let offset = NonZeroU64::new(8).unwrap();

        cache.insert(b"FIELD", offset);

        assert_eq!(cache.get(b"FIELD"), Some(offset));
        assert_eq!(cache.get(b"OTHER"), None);
    }

    #[test]
    fn field_cache_skips_oversized_field_names() {
        let mut cache = FieldCache::new();
        let offset = NonZeroU64::new(16).unwrap();
        let oversized = vec![b'x'; FIELD_CACHE_MAX_PAYLOAD_LEN + 1];

        cache.insert(&oversized, offset);

        assert!(cache.get(&oversized).is_none());
        assert_eq!(cache.len(), 0);
    }

    #[test]
    fn field_cache_stays_bounded_after_capacity_is_exceeded() {
        let mut cache = FieldCache::new();

        for index in 0..FIELD_CACHE_MAX_ENTRIES {
            let key = format!("FIELD_{index}");
            cache.insert(key.as_bytes(), NonZeroU64::new((index + 1) as u64).unwrap());
        }

        assert_eq!(cache.len(), FIELD_CACHE_MAX_ENTRIES);

        cache.insert(b"FIELD_OVERFLOW", NonZeroU64::new(9_999).unwrap());

        assert_eq!(
            cache.get(b"FIELD_OVERFLOW"),
            Some(NonZeroU64::new(9_999).unwrap())
        );
        assert!(cache.get(b"FIELD_0").is_none());
        assert!(cache.len() <= FIELD_CACHE_MAX_ENTRIES);
    }

    #[test]
    fn recent_data_cache_hits_exact_payloads() {
        let mut cache = RecentDataCache::new();
        let item = EntryItem {
            offset: NonZeroU64::new(8).unwrap(),
            hash: 42,
        };

        cache.insert(b"FOO=bar", item);

        assert_eq!(cache.get(b"FOO=bar").unwrap().offset, item.offset);
        assert_eq!(cache.get(b"FOO=bar").unwrap().hash, item.hash);
        assert!(cache.get(b"FOO=baz").is_none());
    }

    #[test]
    fn recent_data_cache_skips_oversized_payloads() {
        let mut cache = RecentDataCache::new();
        let item = EntryItem {
            offset: NonZeroU64::new(16).unwrap(),
            hash: 7,
        };
        let oversized = vec![b'x'; RECENT_DATA_CACHE_MAX_PAYLOAD_LEN + 1];

        cache.insert(&oversized, item);

        assert!(cache.get(&oversized).is_none());
    }
}
