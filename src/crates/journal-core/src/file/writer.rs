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

const OBJECT_ALIGNMENT: u64 = 8;
const FIELD_CACHE_MAX_ENTRIES: usize = 1024;
const FIELD_CACHE_MAX_PAYLOAD_LEN: usize = 128;
const RECENT_DATA_CACHE_SLOTS: usize = 4096;
const RECENT_DATA_CACHE_MAX_PAYLOAD_LEN: usize = 256;

#[derive(Debug, Clone, Copy)]
struct EntryItem {
    offset: NonZeroU64,
    hash: u64,
}

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

#[derive(Debug, Clone)]
struct RecentDataCacheEntry {
    payload: Box<[u8]>,
    item: EntryItem,
}

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

pub struct JournalWriter {
    tail_object_offset: NonZeroU64,
    append_offset: NonZeroU64,
    next_seqnum: u64,
    num_written_objects: u64,
    entry_items: Vec<EntryItem>,
    field_cache: FieldCache,
    recent_data_cache: RecentDataCache,
    first_entry_monotonic: Option<u64>,
    boot_id: uuid::Uuid,
}

impl JournalWriter {
    /// Get current file size in bytes
    pub fn current_file_size(&self) -> u64 {
        self.append_offset.get()
    }

    /// Get the monotonic timestamp of the first entry written to this file
    pub fn first_entry_monotonic(&self) -> Option<u64> {
        self.first_entry_monotonic
    }

    /// Get the next sequence number that will be written
    pub fn next_seqnum(&self) -> u64 {
        self.next_seqnum
    }

    /// Get the boot ID for this writer
    pub fn boot_id(&self) -> uuid::Uuid {
        self.boot_id
    }

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

    /// Creates a successor writer for a new journal file
    pub fn create_successor(&self, journal_file: &mut JournalFile<MmapMut>) -> Result<Self> {
        Self::new(journal_file, self.next_seqnum, self.boot_id)
    }

    pub fn add_entry(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        items: &[&[u8]],
        realtime: u64,
        monotonic: u64,
    ) -> Result<()> {
        let header = journal_file.journal_header_ref();
        assert!(header.has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash));

        // Write the data/field objects while computing the entry's xor-hash
        // and storing each data object's offset/hash
        let mut xor_hash = 0;
        {
            self.entry_items.clear();
            for payload in items {
                let entry_item = self.add_data(journal_file, payload)?;
                self.entry_items.push(entry_item);

                // Per journal file format spec: xor_hash always uses Jenkins lookup3,
                // even for files with HEADER_INCOMPATIBLE_KEYED_HASH flag set
                xor_hash ^= jenkins_hash64(payload);
            }

            self.entry_items
                .sort_unstable_by(|a, b| a.offset.cmp(&b.offset));
            self.entry_items.dedup_by(|a, b| a.offset == b.offset);
        }

        // write the entry itself
        let entry_offset = self.append_offset;
        let entry_size = {
            let size = Some(self.entry_items.len() as u64 * 16);
            let mut entry_guard = journal_file.entry_mut(entry_offset, size)?;

            entry_guard.header.seqnum = self.next_seqnum;
            entry_guard.header.xor_hash = xor_hash;
            entry_guard.header.boot_id = *self.boot_id.as_bytes();
            entry_guard.header.monotonic = monotonic;
            entry_guard.header.realtime = realtime;

            // set each entry item
            for (index, entry_item) in self.entry_items.iter().enumerate() {
                entry_guard
                    .items
                    .set(index, entry_item.offset, Some(entry_item.hash));
            }

            entry_guard.header.object_header.aligned_size()
        };
        self.object_added(journal_file, entry_offset, entry_size);

        self.append_to_entry_array(journal_file, entry_offset)?;
        for entry_item_index in 0..self.entry_items.len() {
            self.link_data_to_entry(journal_file, entry_offset, entry_item_index)?;
        }

        self.entry_added(journal_file.journal_header_mut(), realtime, monotonic);

        Ok(())
    }

    fn object_added(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        object_offset: NonZeroU64,
        object_size: u64,
    ) {
        self.tail_object_offset = object_offset;
        self.append_offset = object_offset.saturating_add(object_size);
        self.num_written_objects += 1;

        // Update arena_size immediately after writing, so subsequent reads
        // within the same write operation can find the newly written object.
        let header = journal_file.journal_header_mut();
        header.arena_size = self.append_offset.get() - header.header_size;
    }

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
                // We will have to write the new data object at the current
                // tail offset
                let data_offset = self.append_offset;
                let data_size = {
                    let mut data_guard =
                        journal_file.data_mut(data_offset, Some(payload.len() as u64))?;

                    data_guard.header.hash = hash;
                    data_guard.set_payload(payload);
                    data_guard.header.object_header.aligned_size()
                };

                self.object_added(journal_file, data_offset, data_size);

                // Update hash table
                journal_file.data_hash_table_set_tail_offset(hash, data_offset)?;

                // Add the field object, if we have any
                if let Some(equals_pos) = payload.iter().position(|&b| b == b'=') {
                    let field_offset = self.add_field(journal_file, &payload[..equals_pos])?;

                    // Link data object to the linked-list
                    {
                        let head_data_offset = {
                            let field_guard = journal_file.field_ref(field_offset)?;
                            field_guard.header.head_data_offset
                        };

                        let mut data_guard = journal_file.data_mut(data_offset, None)?;
                        data_guard.header.next_field_offset = head_data_offset;
                    }

                    // Link field to the head of the linked list
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
                // We will have to write the new field object at the current
                // tail offset
                let field_offset = self.append_offset;
                let field_size = {
                    let mut field_guard =
                        journal_file.field_mut(field_offset, Some(payload.len() as u64))?;

                    field_guard.header.hash = hash;
                    field_guard.set_payload(payload);
                    field_guard.header.object_header.aligned_size()
                };
                self.object_added(journal_file, field_offset, field_size);

                // Update hash table
                journal_file.field_hash_table_set_tail_offset(hash, field_offset)?;

                self.field_cache.insert(payload, field_offset);

                // Return the offset where we wrote the newly added data object
                Ok(field_offset)
            }
        }
    }

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
                // This shouldn't happen if counts are correct
                return Err(JournalError::InvalidOffsetArrayOffset);
            };

            array_offset = next_offset;
        }

        // Try to add to the tail array
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

                        // Drop the data guard to avoid borrow conflicts
                        drop(data_guard);

                        // Find the tail of the entry array chain and append
                        self.append_to_data_entry_array(
                            journal_file,
                            array_offset,
                            entry_offset,
                            current_count,
                        )?;

                        // Update the count
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
