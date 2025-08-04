#![allow(unused_imports, dead_code)]

use crate::{
    journal_hash_data, CompactEntryItem, DataHashTable, DataObject, DataObjectHeader,
    DataPayloadType, EntryObject, EntryObjectHeader, FieldHashTable, FieldObject,
    FieldObjectHeader, HashItem, HashTable, HashTableMut, HashableObject, HashableObjectMut,
    HeaderIncompatibleFlags, JournalFile, JournalFileOptions, JournalHeader, JournalState,
    ObjectHeader, ObjectType, RegularEntryItem,
};
use error::{JournalError, Result};
use memmap2::MmapMut;
use rand::{seq::IndexedRandom, Rng};
use std::num::{NonZeroU64, NonZeroUsize};
use std::path::Path;
use window_manager::MemoryMapMut;
use zerocopy::{FromBytes, IntoBytes};

const OBJECT_ALIGNMENT: u64 = 8;

#[derive(Debug, Clone, Copy)]
struct EntryItem {
    offset: NonZeroU64,
    hash: u64,
}

pub struct JournalWriter {
    tail_object_offset: NonZeroU64,
    append_offset: NonZeroU64,
    next_seqnum: u64,
    num_written_objects: u64,
    entry_items: Vec<EntryItem>,
    first_entry_monotonic: Option<u64>,
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

    pub fn new(journal_file: &mut JournalFile<MmapMut>) -> Result<Self> {
        let (append_offset, next_seqnum) = {
            let header = journal_file.journal_header_ref();

            let Some(tail_object_offset) = header.tail_object_offset else {
                return Err(JournalError::InvalidMagicNumber);
            };

            let tail_object = journal_file.object_header_ref(tail_object_offset)?;

            (
                tail_object_offset.saturating_add(tail_object.size),
                header.tail_entry_seqnum + 1,
            )
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
            first_entry_monotonic: None,
        })
    }

    pub fn add_entry(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        items: &[&[u8]],
        realtime: u64,
        monotonic: u64,
        boot_id: [u8; 16],
    ) -> Result<()> {
        let header = journal_file.journal_header_ref();
        assert!(header.has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash));

        // Write the data/field objects while computing the entry's xor-hash
        // and storing each data object's offset/hash
        let mut xor_hash = 0;
        {
            self.entry_items.clear();
            for payload in items {
                let offset = self.add_data(journal_file, payload)?;
                let hash = {
                    let data_guard = journal_file.data_ref(offset)?;
                    data_guard.hash()
                };

                let entry_item = EntryItem { offset, hash };
                self.entry_items.push(entry_item);

                xor_hash ^= journal_hash_data(payload, true, None);
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
            entry_guard.header.boot_id = boot_id;
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
        self.object_added(entry_offset, entry_size);

        self.append_to_entry_array(journal_file, entry_offset)?;
        for entry_item_index in 0..self.entry_items.len() {
            self.link_data_to_entry(journal_file, entry_offset, entry_item_index)?;
        }

        self.entry_added(
            journal_file.journal_header_mut(),
            realtime,
            monotonic,
            boot_id,
        );

        Ok(())
    }

    fn object_added(&mut self, object_offset: NonZeroU64, object_size: u64) {
        self.tail_object_offset = object_offset;
        self.append_offset = object_offset.saturating_add(object_size);
        self.num_written_objects += 1;
    }

    fn entry_added(
        &mut self,
        header: &mut JournalHeader,
        realtime: u64,
        monotonic: u64,
        boot_id: [u8; 16],
    ) {
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
        header.tail_entry_boot_id = boot_id;

        self.next_seqnum += 1;
        self.num_written_objects = 0;
    }

    fn add_data(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        payload: &[u8],
    ) -> Result<NonZeroU64> {
        let hash = journal_file.hash(payload);

        match journal_file.find_data_offset(hash, payload)? {
            Some(data_offset) => Ok(data_offset),
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

                self.object_added(data_offset, data_size);

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
                    };
                }

                Ok(data_offset)
            }
        }
    }

    fn add_field(
        &mut self,
        journal_file: &mut JournalFile<MmapMut>,
        payload: &[u8],
    ) -> Result<NonZeroU64> {
        let hash = journal_file.hash(payload);

        match journal_file.find_field_offset(hash, payload)? {
            Some(field_offset) => Ok(field_offset),
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
                self.object_added(field_offset, field_size);

                // Update hash table
                journal_file.field_hash_table_set_tail_offset(hash, field_offset)?;

                // Return the offset where we wrote the newly added data object
                Ok(field_offset)
            }
        }
    }

    fn allocate_new_array(
        &mut self,
        journal_file: &JournalFile<MmapMut>,
        capacity: NonZeroU64,
    ) -> Result<NonZeroU64> {
        // let new_capacity = previous_capacity.saturating_mul(NonZeroU64::new(2).unwrap());

        let array_offset = self.append_offset;
        let array_size = {
            let array_guard = journal_file.offset_array_mut(array_offset, Some(capacity))?;

            array_guard.header.object_header.aligned_size()
        };
        self.object_added(array_offset, array_size);

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
    use super::*;
    use crate::{load_boot_id, Direction, JournalFile, JournalReader, Location};
    use memmap2::Mmap;
    use std::collections::HashMap;
    use tempfile::NamedTempFile;

    fn generate_uuid() -> [u8; 16] {
        use rand::Rng;
        let mut rng = rand::rng();
        rng.random()
    }

    #[test]
    fn test_write_and_read_journal_entries() -> Result<()> {
        // Create test data - a hash map with key/values to add to the journal file
        let mut test_data = HashMap::new();
        test_data.insert(
            "MESSAGE",
            vec!["Hello, world!", "Another message", "Final message"],
        );
        test_data.insert("PRIORITY", vec!["6", "4", "3"]);
        test_data.insert(
            "_SYSTEMD_UNIT",
            vec!["test.service", "other.service", "test.service"],
        );
        test_data.insert("_PID", vec!["1234", "5678", "9999"]);

        // Create a temporary file for the journal
        let temp_file = NamedTempFile::new().map_err(JournalError::Io).unwrap();
        let journal_path = temp_file.path();

        // Step 1: Create and write to the journal
        let boot_id = load_boot_id().unwrap_or([1; 16]); // Use real boot_id or fallback
        let num_entries = test_data.values().next().unwrap().len();

        let options = JournalFileOptions::new(
            generate_uuid(),
            generate_uuid(),
            generate_uuid(),
            generate_uuid(),
        );
        let mut journal_file = JournalFile::create(journal_path, options)?;
        let iterations = 5000;
        for _ in 0..iterations {
            let mut writer = JournalWriter::new(&mut journal_file)?;

            // Write entries to the journal
            for i in 0..num_entries {
                let mut entry_data = Vec::new();

                // Build the entry data for this index
                for (key, values) in &test_data {
                    let kv_pair = format!("{}={}", key, values[i]);
                    entry_data.push(kv_pair.into_bytes());
                }

                // Convert to slice references for the writer
                let entry_refs: Vec<&[u8]> = entry_data.iter().map(|v| v.as_slice()).collect();

                // Write the entry with timestamps
                let realtime = 1000000 + (i as u64 * 1000); // Mock realtime in microseconds
                let monotonic = 500000 + (i as u64 * 1000); // Mock monotonic time

                writer.add_entry(&mut journal_file, &entry_refs, realtime, monotonic, boot_id)?;
            }
        }

        // Step 2: Read back and verify the journal contents
        {
            let journal_file = JournalFile::<Mmap>::open(journal_path, 8 * 1024)?;
            let mut reader = JournalReader::default();

            let hdr = journal_file.journal_header_ref();
            println!("Header: {:#?}", hdr);

            // Start from the head
            reader.set_location(Location::Head);

            let mut entries_read = 0;
            while reader.step(&journal_file, Direction::Forward)? {
                println!("Reading entry {}", entries_read);

                // Verify timestamps
                let realtime = reader.get_realtime_usec(&journal_file)?;
                let expected_realtime = 1000000 + ((entries_read % 3) * 1000);
                assert_eq!(
                    realtime, expected_realtime,
                    "Realtime mismatch for entry {}",
                    entries_read
                );

                let (seqnum, _seqnum_id) = reader.get_seqnum(&journal_file)?;
                assert_eq!(
                    seqnum,
                    entries_read + 1,
                    "Sequence number mismatch for entry {}",
                    entries_read
                );

                // Read all data for this entry
                let mut entry_fields = HashMap::new();
                reader.entry_data_restart();

                while let Some(data_guard) = reader.entry_data_enumerate(&journal_file)? {
                    let payload = data_guard.payload_bytes();
                    let payload_str = String::from_utf8_lossy(payload);

                    if let Some(eq_pos) = payload_str.find('=') {
                        let key = &payload_str[..eq_pos];
                        let value = &payload_str[eq_pos + 1..];
                        entry_fields.insert(key.to_string(), value.to_string());
                    }
                }

                // Verify the data matches what we wrote
                for (key, values) in &test_data {
                    let expected_value = &values[entries_read as usize % 3];
                    let actual_value = entry_fields.get(*key).unwrap_or_else(|| {
                        panic!("Missing key '{}' in entry {}", key, entries_read)
                    });

                    assert_eq!(
                        actual_value, expected_value,
                        "Value mismatch for key '{}' in entry {}",
                        key, entries_read
                    );
                }

                println!("Read entry {}", entries_read);
                entries_read += 1;
            }

            assert_eq!(
                entries_read as usize,
                num_entries * iterations,
                "Number of entries read doesn't match written"
            );
        }

        // Step 3: Test filtering by specific fields
        {
            let journal_file = JournalFile::<Mmap>::open(journal_path, 64 * 1024)?;
            let mut reader = JournalReader::default();

            // Test filtering by _SYSTEMD_UNIT=test.service
            reader.add_match(b"_SYSTEMD_UNIT=test.service");
            reader.set_location(Location::Head);

            let mut filtered_entries = 0;
            while reader.step(&journal_file, Direction::Forward)? {
                // Verify this entry actually contains the filter match
                reader.entry_data_restart();
                let mut found_match = false;

                while let Some(data_guard) = reader.entry_data_enumerate(&journal_file)? {
                    let payload = data_guard.payload_bytes();
                    if payload == b"_SYSTEMD_UNIT=test.service" {
                        found_match = true;
                        break;
                    }
                }

                assert!(
                    found_match,
                    "Filtered entry doesn't contain the expected field"
                );
                filtered_entries += 1;
            }

            // Should find 2 entries with _SYSTEMD_UNIT=test.service (entries 0 and 2)
            assert_eq!(filtered_entries, 2, "Expected 2 filtered entries");
        }

        println!("✅ All tests passed!");
        Ok(())
    }

    #[test]
    fn test_field_enumeration() -> Result<()> {
        // Create a simple journal with known fields
        let temp_file = NamedTempFile::new().map_err(JournalError::Io)?;
        let journal_path = temp_file.path();

        let test_fields = vec!["MESSAGE", "PRIORITY", "_SYSTEMD_UNIT"];
        let boot_id = [1; 16];

        // Write a single entry with multiple fields
        {
            let options = JournalFileOptions::new(
                generate_uuid(),
                generate_uuid(),
                generate_uuid(),
                generate_uuid(),
            );

            let mut journal_file = JournalFile::create(journal_path, options)?;
            let mut writer = JournalWriter::new(&mut journal_file)?;

            let entry_data = vec![
                b"MESSAGE=Test message".as_slice(),
                b"PRIORITY=6".as_slice(),
                b"_SYSTEMD_UNIT=test.service".as_slice(),
            ];

            writer.add_entry(&mut journal_file, &entry_data, 1000000, 500000, boot_id)?;
        }

        // Read back and enumerate fields
        {
            let journal_file = JournalFile::<Mmap>::open(journal_path, 8 * 1024)?;
            let mut reader = JournalReader::default();

            let mut found_fields = Vec::new();
            reader.fields_restart();

            while let Some(field_guard) = reader.fields_enumerate(&journal_file)? {
                let field_name = String::from_utf8_lossy(field_guard.payload);
                found_fields.push(field_name.to_string());
            }

            // Verify all expected fields were found
            for expected_field in &test_fields {
                assert!(
                    found_fields.contains(&expected_field.to_string()),
                    "Expected field '{}' not found. Found: {:?}",
                    expected_field,
                    found_fields
                );
            }
        }

        Ok(())
    }
}
