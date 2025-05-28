// // #![allow(unused_imports, dead_code)]

// use error::{JournalError, Result};
// use journal_file::{
//     journal_hash_data, CompactEntryItem, DataObject, DataObjectHeader, DataPayloadType,
//     EntryObject, EntryObjectHeader, FieldObject, FieldObjectHeader, HashItem, HashTableObject,
//     HeaderIncompatibleFlags, JournalFile, JournalHeader, JournalState, ObjectHeader, ObjectType,
//     RegularEntryItem,
// };
// use memmap2::MmapMut;
// use rand::{seq::IndexedRandom, Rng};
// use std::num::NonZeroU64;
// use std::path::Path;
// use window_manager::MemoryMapMut;
// use zerocopy::{FromBytes, IntoBytes};

// const OBJECT_ALIGNMENT: u64 = 8;

// #[derive(Default)]
// pub struct JournalWriter {
//     tail_offset: u64,
//     offsets_buffer: Vec<u64>,
//     hash_buffer: Vec<u64>,
// }

// impl JournalWriter {
//     pub fn new(journal_file: &mut JournalFile<MmapMut>) -> Result<Self> {
//         Ok(Self {
//             tail_offset: 0,
//             offsets_buffer: Vec::with_capacity(128),
//             hash_buffer: Vec::with_capacity(128),
//         })
//     }

//     pub fn add_entry(
//         &mut self,
//         journal_file: &mut JournalFile<MmapMut>,
//         items: &[&[u8]],
//         realtime: u64,
//         monotonic: u64,
//         boot_id: [u8; 16],
//     ) -> Result<u64> {
//         let header = journal_file.journal_header_ref();

//         let is_keyed_hash = header.has_incompatible_flag(HeaderIncompatibleFlags::KeyedHash);
//         let is_compact = header.has_incompatible_flag(HeaderIncompatibleFlags::Compact);
//         let file_id = header.file_id;

//         let mut current_offset = header.tail_object_offset;
//         if (current_offset == 0) || (current_offset % 8 != 0) {
//             return Err(JournalError::InvalidOffset);
//         }

//         let mut arena_size = header.arena_size;
//         if (current_offset == 0) || (current_offset % 8 != 0) {
//             return Err(JournalError::InvalidOffset);
//         }

//         // self.hash_buffer.clear();
//         // self.hash_buffer.extend(
//         //     items
//         //         .iter()
//         //         .map(|item| journal_hash_data(item, is_keyed_hash, None)),
//         // );

//         // for payload in items.iter() {
//         //     let hash = journal_file.hash(payload);
//         //     match journal_file.find_data_offset_by_payload(payload, hash) {
//         //         Ok(data_offset) => {
//         //             self.offsets_buffer.push(data_offset);
//         //         }
//         //         Err(JournalError::MissingObjectFromHashTable) => {
//         //             let size = payload.len() as u64;
//         //             let data_object = journal_file.data_mut(current_offset, Some(size))?;

//         //             current_offset += data_object.header.object_header.aligned_size();

//         //             data_object.
//         //         }
//         //         Err(e) => {
//         //             return Err(e);
//         //         }
//         //     };
//         // }

//         Ok(0)
//     }
// }
