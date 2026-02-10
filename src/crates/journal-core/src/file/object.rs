use crate::error::{JournalError, Result};
use crate::file::offset_array::{Cursor, InlinedCursor, List};
use std::num::{NonZeroU32, NonZeroU64, NonZeroUsize};
use zerocopy::{
    ByteSlice, ByteSliceMut, FromBytes, Immutable, IntoBytes, KnownLayout, Ref, SplitByteSlice,
    SplitByteSliceMut,
};

pub trait HashableObject {
    /// Get the hash value of this object
    fn hash(&self) -> u64;

    /// Get the payload data for matching
    fn raw_payload(&self) -> &[u8];

    /// Check if the payload is compressed
    fn is_compressed(&self) -> bool;

    /// Decompress the payload into the provided buffer.
    /// Returns the number of decompressed bytes.
    fn decompress(&self, buf: &mut Vec<u8>) -> Result<usize>;

    /// Get the offset to the next object in the hash chain
    fn next_hash_offset(&self) -> Option<NonZeroU64>;

    /// Get the object type
    fn object_type() -> ObjectType;
}

pub trait HashableObjectMut: HashableObject {
    /// Set the offset to the next object in the hash chain
    fn set_next_hash_offset(&mut self, offset: NonZeroU64);

    /// Set the payload of the object
    fn set_payload(&mut self, data: &[u8]);
}

/// Trait for hash table operations
pub trait HashTable {
    /// The type of objects stored in this hash table
    type Object: HashableObject;

    /// Get the hash item for a given hash value
    fn hash_item_ref(&self, hash: u64) -> &HashItem;

    /// Get the length of the hash table (number of buckets)
    fn len(&self) -> usize;

    /// Make clippy happy
    fn is_empty(&self) -> bool {
        todo!()
    }
}

/// Trait for mutable hash table operations
pub trait HashTableMut: HashTable {
    /// Get a mutable reference to the hash item for a given hash value
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem;
}

pub struct DataHashTable<B: ByteSlice> {
    pub header: Ref<B, ObjectHeader>,
    pub items: Ref<B, [HashItem]>,
}

pub struct FieldHashTable<B: ByteSlice> {
    pub header: Ref<B, ObjectHeader>,
    pub items: Ref<B, [HashItem]>,
}

// Implement HashTable for DataHashTable
impl<B: ByteSlice> HashTable for DataHashTable<B> {
    type Object = DataObject<B>;

    fn hash_item_ref(&self, hash: u64) -> &HashItem {
        let bucket_index = hash as usize % self.items.len();
        &self.items[bucket_index]
    }

    fn len(&self) -> usize {
        self.items.len()
    }
}

// Implement HashTable for FieldHashTable
impl<B: ByteSlice> HashTable for FieldHashTable<B> {
    type Object = FieldObject<B>;

    fn hash_item_ref(&self, hash: u64) -> &HashItem {
        let bucket_index = hash as usize % self.items.len();
        &self.items[bucket_index]
    }

    fn len(&self) -> usize {
        self.items.len()
    }
}

// Implement HashTableMut for DataHashTable
impl<B: ByteSliceMut> HashTableMut for DataHashTable<B> {
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem {
        let bucket_index = hash as usize % self.items.len();
        &mut self.items[bucket_index]
    }
}

// Implement HashTableMut for FieldHashTable
impl<B: ByteSliceMut> HashTableMut for FieldHashTable<B> {
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem {
        let bucket_index = hash as usize % self.items.len();
        &mut self.items[bucket_index]
    }
}

// Implement JournalObject for DataHashTable
impl<B: SplitByteSlice> JournalObject<B> for DataHashTable<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(DataHashTable { header, items })
    }
}

// Implement JournalObjectMut for DataHashTable
impl<B: SplitByteSliceMut> JournalObjectMut<B> for DataHashTable<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(DataHashTable { header, items })
    }
}

// Implement JournalObject for FieldHashTable
impl<B: SplitByteSlice> JournalObject<B> for FieldHashTable<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(FieldHashTable { header, items })
    }
}

// Implement JournalObjectMut for FieldHashTable
impl<B: SplitByteSliceMut> JournalObjectMut<B> for FieldHashTable<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(FieldHashTable { header, items })
    }
}

impl<B: ByteSlice> HashableObject for FieldObject<B> {
    fn hash(&self) -> u64 {
        self.header.hash
    }

    fn raw_payload(&self) -> &[u8] {
        &self.payload
    }

    fn is_compressed(&self) -> bool {
        false
    }

    fn decompress(&self, buf: &mut Vec<u8>) -> Result<usize> {
        buf.clear();
        buf.extend_from_slice(&self.payload);
        Ok(buf.len())
    }

    fn next_hash_offset(&self) -> Option<NonZeroU64> {
        self.header.next_hash_offset
    }

    fn object_type() -> ObjectType {
        ObjectType::Field
    }
}

impl HashableObjectMut for FieldObject<&mut [u8]> {
    fn set_next_hash_offset(&mut self, next_hash_offset: NonZeroU64) {
        self.header.next_hash_offset = Some(next_hash_offset);
    }

    fn set_payload(&mut self, data: &[u8]) {
        self.payload.copy_from_slice(data);
    }
}

impl<B: ByteSlice> HashableObject for DataObject<B> {
    fn hash(&self) -> u64 {
        self.header.hash
    }

    fn raw_payload(&self) -> &[u8] {
        self.raw_payload()
    }

    fn is_compressed(&self) -> bool {
        DataObject::is_compressed(self)
    }

    fn decompress(&self, buf: &mut Vec<u8>) -> Result<usize> {
        DataObject::decompress(self, buf)
    }

    fn next_hash_offset(&self) -> Option<NonZeroU64> {
        self.header.next_hash_offset
    }

    fn object_type() -> ObjectType {
        ObjectType::Data
    }
}

impl HashableObjectMut for DataObject<&mut [u8]> {
    fn set_next_hash_offset(&mut self, next_hash_offset: NonZeroU64) {
        self.header.next_hash_offset = Some(next_hash_offset);
    }

    fn set_payload(&mut self, data: &[u8]) {
        match &mut self.payload {
            DataPayloadType::Regular(payload) => {
                payload.copy_from_slice(data);
            }
            DataPayloadType::Compact { payload, .. } => {
                payload.copy_from_slice(data);
            }
        };
    }
}

/// Trait to standardize creation of journal objects from byte slices
pub trait JournalObject<B: SplitByteSlice>: Sized {
    /// Create a new journal object from a byte slice
    fn from_data(data: B, is_compact: bool) -> Option<Self>;
}

pub trait JournalObjectMut<B: SplitByteSliceMut>: JournalObject<B> {
    /// Create a new journal object from a byte slice
    fn from_data_mut(data: B, is_compact: bool) -> Option<Self>;
}

pub enum HeaderIncompatibleFlags {
    CompressedXz = 1 << 0,
    CompressedLz4 = 1 << 1,
    KeyedHash = 1 << 2,
    CompressedZstd = 1 << 3,
    Compact = 1 << 4,
}

pub enum HeaderCompatibleFlags {
    Sealed = 1 << 0,
    TailEntryBootId = 1 << 1,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum JournalState {
    Offline = 0,
    Online = 1,
    Archived = 2,
}

impl TryFrom<u8> for JournalState {
    type Error = JournalError;

    fn try_from(value: u8) -> Result<Self> {
        match value {
            0 => Ok(JournalState::Offline),
            1 => Ok(JournalState::Online),
            2 => Ok(JournalState::Archived),
            _ => Err(JournalError::InvalidJournalFileState),
        }
    }
}

impl std::fmt::Display for JournalState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            JournalState::Offline => write!(f, "OFFLINE"),
            JournalState::Online => write!(f, "ONLINE"),
            JournalState::Archived => write!(f, "ARCHIVED"),
        }
    }
}

#[derive(Default, Debug, Clone, Copy, FromBytes, IntoBytes, Immutable, KnownLayout)]
#[repr(C)]
pub struct JournalHeader {
    pub signature: [u8; 8],                          // "LPKSHHRH"
    pub compatible_flags: u32,                       // Compatible extension flags
    pub incompatible_flags: u32,                     // Incompatible extension flags
    pub state: u8,                                   // File state (offline=0, online=1, archived=2)
    pub reserved: [u8; 7],                           // Reserved space
    pub file_id: [u8; 16],                           // Unique ID for this file
    pub machine_id: [u8; 16],                        // Machine ID this belongs to
    pub tail_entry_boot_id: [u8; 16],                // Boot ID of the last entry
    pub seqnum_id: [u8; 16],                         // Sequence number ID
    pub header_size: u64,                            // Size of the header
    pub arena_size: u64,                             // Size of the data arena
    pub data_hash_table_offset: Option<NonZeroU64>,  // Offset of the data hash table
    pub data_hash_table_size: Option<NonZeroU64>,    // Size of the data hash table
    pub field_hash_table_offset: Option<NonZeroU64>, // Offset of the field hash table
    pub field_hash_table_size: Option<NonZeroU64>,   // Size of the field hash table
    pub tail_object_offset: Option<NonZeroU64>,      // Offset of the last object
    pub n_objects: u64,                              // Number of objects
    pub n_entries: u64,                              // Number of entries
    pub tail_entry_seqnum: u64,                      // Sequence number of the last entry
    pub head_entry_seqnum: u64,                      // Sequence number of the first entry
    pub entry_array_offset: Option<NonZeroU64>,      // Offset of the entry array
    pub head_entry_realtime: u64,                    // Realtime timestamp of the first entry
    pub tail_entry_realtime: u64,                    // Realtime timestamp of the last entry
    pub tail_entry_monotonic: u64,                   // Monotonic timestamp of the last entry
}

/*
    NOTE: For the time being, we do not need the following fields.

    // Added in 187
    pub n_data: u64,   // Number of data objects
    pub n_fields: u64, // Number of field objects
    // Added in 189
    pub n_tags: u64,         // Number of tag objects
    pub n_entry_arrays: u64, // Number of entry array objects
    // Added in 246
    pub data_hash_chain_depth: u64, // Deepest chain in data hash table
    pub field_hash_chain_depth: u64, // Deepest chain in field hash table
    // Added in 252
    pub tail_entry_array_offset: u32, // Offset to the tail entry array
    pub tail_entry_array_n_entries: u32, // Number of entries in the tail entry array
    // Added in 254
    pub tail_entry_offset: u64, // Offset to the tail entry
*/

impl JournalHeader {
    pub fn has_incompatible_flag(&self, flag: HeaderIncompatibleFlags) -> bool {
        (self.incompatible_flags & flag as u32) != 0
    }

    pub fn has_compatible_flag(&self, flag: HeaderCompatibleFlags) -> bool {
        (self.compatible_flags & flag as u32) != 0
    }
}

pub enum ObjectFlags {
    CompressedXz = 1 << 0,
    CompressedLz4 = 1 << 1,
    CompressedZstd = 1 << 2,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum ObjectType {
    Unused = 0,
    Data = 1,
    Field = 2,
    Entry = 3,
    DataHashTable = 4,
    FieldHashTable = 5,
    EntryArray = 6,
    Tag = 7,
}

impl TryFrom<u8> for ObjectType {
    type Error = JournalError;

    fn try_from(value: u8) -> Result<Self> {
        match value {
            0 => Ok(ObjectType::Unused),
            1 => Ok(ObjectType::Data),
            2 => Ok(ObjectType::Field),
            3 => Ok(ObjectType::Entry),
            4 => Ok(ObjectType::DataHashTable),
            5 => Ok(ObjectType::FieldHashTable),
            6 => Ok(ObjectType::EntryArray),
            7 => Ok(ObjectType::Tag),
            _ => Err(JournalError::InvalidObjectType),
        }
    }
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct ObjectHeader {
    pub type_: u8,
    pub flags: u8,
    pub reserved: [u8; 6],
    pub size: u64,
}

impl ObjectHeader {
    pub fn xz_compressed(&self) -> bool {
        (self.flags & ObjectFlags::CompressedXz as u8) != 0
    }

    pub fn lz4_compressed(&self) -> bool {
        (self.flags & ObjectFlags::CompressedLz4 as u8) != 0
    }

    pub fn zstd_compressed(&self) -> bool {
        (self.flags & ObjectFlags::CompressedZstd as u8) != 0
    }

    pub fn is_compressed(&self) -> bool {
        self.zstd_compressed() | self.lz4_compressed() | self.xz_compressed()
    }

    pub fn aligned_size(&self) -> u64 {
        (self.size + 7) & !7
    }
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct FieldObjectHeader {
    pub object_header: ObjectHeader,
    pub hash: u64,
    pub next_hash_offset: Option<NonZeroU64>,
    pub head_data_offset: Option<NonZeroU64>,
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct OffsetArrayObjectHeader {
    pub object_header: ObjectHeader,
    pub next_offset_array: Option<NonZeroU64>,
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct HashItem {
    pub head_hash_offset: Option<NonZeroU64>,
    pub tail_hash_offset: Option<NonZeroU64>,
}

#[derive(Debug)]
pub struct FieldObject<B: ByteSlice> {
    pub header: Ref<B, FieldObjectHeader>,
    pub payload: B,
}

impl<B: SplitByteSlice> JournalObject<B> for FieldObject<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let (header, payload) = zerocopy::Ref::from_prefix(data).ok()?;
        Some(FieldObject { header, payload })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for FieldObject<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let (header, payload) = zerocopy::Ref::from_prefix(data).ok()?;
        Some(FieldObject { header, payload })
    }
}

pub enum OffsetsType<B: ByteSlice> {
    Regular(Ref<B, [Option<NonZeroU64>]>),
    Compact(Ref<B, [Option<NonZeroU32>]>),
}

impl<B: ByteSlice> OffsetsType<B> {
    pub fn get(&self, index: usize) -> Option<NonZeroU64> {
        match self {
            OffsetsType::Regular(offsets) => offsets[index],
            OffsetsType::Compact(offsets) => offsets[index].map(NonZeroU64::from),
        }
    }
}

impl<B: ByteSliceMut> OffsetsType<B> {
    pub fn set(&mut self, index: usize, value: NonZeroU64) {
        match self {
            OffsetsType::Regular(offsets) => offsets[index] = Some(value),
            OffsetsType::Compact(offsets) => offsets[index] = NonZeroU32::new(value.get() as u32),
        }
    }
}

impl<B: ByteSlice> std::fmt::Debug for OffsetsType<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            OffsetsType::Regular(items) => write!(f, "Regular({} items)", items.len()),
            OffsetsType::Compact(items) => write!(f, "Compact({} items)", items.len()),
        }
    }
}

pub struct OffsetArrayObject<B: ByteSlice> {
    pub header: Ref<B, OffsetArrayObjectHeader>,
    pub items: OffsetsType<B>,
}

impl<B: ByteSlice> OffsetArrayObject<B> {
    pub fn capacity(&self) -> usize {
        match &self.items {
            OffsetsType::Regular(offsets) => offsets.len(),
            OffsetsType::Compact(offsets) => offsets.len(),
        }
    }

    pub fn len(&self, remaining_items: usize) -> usize {
        self.capacity().min(remaining_items)
    }

    pub fn is_empty(&self, remaining_items: usize) -> bool {
        self.len(remaining_items) == 0
    }

    pub fn get(&self, index: usize, remaining_items: usize) -> Result<Option<NonZeroU64>> {
        if self.is_empty(remaining_items) {
            return Err(JournalError::EmptyOffsetArrayNode);
        }

        Ok(self.items.get(index))
    }

    pub fn collect_offsets(
        &self,
        start_index: usize,
        remaining_items: usize,
        offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        let len = self.len(remaining_items);

        if start_index >= len {
            return Err(JournalError::InvalidOffsetArrayIndex);
        }

        match &self.items {
            OffsetsType::Regular(s) => {
                offsets.extend(s[start_index..len].iter().filter_map(|&opt| opt));
            }
            OffsetsType::Compact(s) => {
                offsets.extend(
                    s[start_index..len]
                        .iter()
                        .filter_map(|&opt| opt.map(NonZeroU64::from)),
                );
            }
        }

        Ok(())
    }
}

impl<B: ByteSliceMut> OffsetArrayObject<B> {
    pub fn set(&mut self, index: usize, offset: NonZeroU64) -> Result<()> {
        if index >= self.capacity() {
            return Err(JournalError::OutOfBoundsIndex);
        }

        self.items.set(index, offset);
        Ok(())
    }
}

impl<B: ByteSlice> std::fmt::Debug for OffsetArrayObject<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("JournalHeader")
            .field("header", &self.header)
            .finish()
    }
}

impl<B: SplitByteSlice> JournalObject<B> for OffsetArrayObject<B> {
    fn from_data(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data
            .split_at(std::mem::size_of::<OffsetArrayObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let items_type = if is_compact {
            let compact_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            OffsetsType::Compact(compact_items)
        } else {
            let regular_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            OffsetsType::Regular(regular_items)
        };

        Some(OffsetArrayObject {
            header,
            items: items_type,
        })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for OffsetArrayObject<B> {
    fn from_data_mut(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data
            .split_at(std::mem::size_of::<OffsetArrayObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let items_type = if is_compact {
            let compact_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            OffsetsType::Compact(compact_items)
        } else {
            let regular_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            OffsetsType::Regular(regular_items)
        };

        Some(OffsetArrayObject {
            header,
            items: items_type,
        })
    }
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct EntryObjectHeader {
    pub object_header: ObjectHeader,
    pub seqnum: u64,
    pub realtime: u64,
    pub monotonic: u64,
    pub boot_id: [u8; 16], // UUID/128-bit ID
    pub xor_hash: u64,
}

// For regular (non-compact) format - an array of these follows the header
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct RegularEntryItem {
    pub object_offset: u64,
    pub hash: u64,
}

// For compact format - an array of these follows the header
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct CompactEntryItem {
    pub object_offset: u32,
}

pub enum EntryItemsType<B: ByteSlice> {
    Regular(Ref<B, [RegularEntryItem]>),
    Compact(Ref<B, [CompactEntryItem]>),
}

impl<B: ByteSliceMut> EntryItemsType<B> {
    pub fn set(&mut self, index: usize, object_offset: NonZeroU64, hash: Option<u64>) {
        match self {
            EntryItemsType::Regular(entry_items) => {
                entry_items[index].object_offset = object_offset.get();
                entry_items[index].hash = hash.unwrap();
            }
            EntryItemsType::Compact(entry_items) => {
                debug_assert!(hash.is_none());
                assert!(object_offset.get() < u32::MAX as u64);
                entry_items[index].object_offset = object_offset.get() as u32;
            }
        }
    }
}

impl<B: ByteSlice> EntryItemsType<B> {
    pub fn get(&self, index: usize) -> u64 {
        match self {
            EntryItemsType::Regular(entry_items) => entry_items[index].object_offset,
            EntryItemsType::Compact(entry_items) => entry_items[index].object_offset as u64,
        }
    }

    pub fn len(&self) -> usize {
        match self {
            EntryItemsType::Regular(entry_items) => entry_items.len(),
            EntryItemsType::Compact(entry_items) => entry_items.len(),
        }
    }

    pub fn is_empty(&self) -> bool {
        match self {
            EntryItemsType::Regular(entry_items) => entry_items.is_empty(),
            EntryItemsType::Compact(entry_items) => entry_items.is_empty(),
        }
    }
}

impl<B: ByteSlice> std::fmt::Debug for EntryItemsType<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            EntryItemsType::Regular(items) => write!(f, "Regular({} items)", items.len()),
            EntryItemsType::Compact(items) => write!(f, "Compact({} items)", items.len()),
        }
    }
}

pub struct EntryObject<B: ByteSlice> {
    pub header: Ref<B, EntryObjectHeader>,
    pub items: EntryItemsType<B>,
}

impl<B: ByteSlice> EntryObject<B> {
    pub fn collect_offsets(&self, offsets: &mut Vec<NonZeroU64>) -> Result<()> {
        match &self.items {
            EntryItemsType::Regular(items) => {
                offsets.reserve(items.len());

                for item in items.iter() {
                    let offset =
                        NonZeroU64::new(item.object_offset).ok_or(JournalError::InvalidOffset)?;
                    offsets.push(offset);
                }
            }
            EntryItemsType::Compact(items) => {
                offsets.reserve(items.len());

                for item in items.iter() {
                    let offset = NonZeroU64::new(item.object_offset as u64)
                        .ok_or(JournalError::InvalidOffset)?;
                    offsets.push(offset);
                }
            }
        }

        Ok(())
    }
}

impl<B: ByteSlice> std::fmt::Debug for EntryObject<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("EntryObject")
            .field("header", &self.header)
            .field("items", &self.items)
            .finish()
    }
}

impl<B: SplitByteSlice> JournalObject<B> for EntryObject<B> {
    fn from_data(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data
            .split_at(std::mem::size_of::<EntryObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let items_type = if is_compact {
            let compact_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            EntryItemsType::Compact(compact_items)
        } else {
            let regular_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            EntryItemsType::Regular(regular_items)
        };

        Some(EntryObject {
            header,
            items: items_type,
        })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for EntryObject<B> {
    fn from_data_mut(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data
            .split_at(std::mem::size_of::<EntryObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let items_type = if is_compact {
            let compact_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            EntryItemsType::Compact(compact_items)
        } else {
            let regular_items = zerocopy::Ref::from_bytes(items_data).ok()?;
            EntryItemsType::Regular(regular_items)
        };

        Some(EntryObject {
            header,
            items: items_type,
        })
    }
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct DataObjectHeader {
    pub object_header: ObjectHeader,
    pub hash: u64,
    pub next_hash_offset: Option<NonZeroU64>,
    pub next_field_offset: Option<NonZeroU64>,
    pub entry_offset: Option<NonZeroU64>,
    pub entry_array_offset: Option<NonZeroU64>,
    pub n_entries: Option<NonZeroU64>,
}

impl DataObjectHeader {
    pub fn xz_compressed(&self) -> bool {
        self.object_header.xz_compressed()
    }

    pub fn lz4_compressed(&self) -> bool {
        self.object_header.lz4_compressed()
    }

    pub fn zstd_compressed(&self) -> bool {
        self.object_header.zstd_compressed()
    }

    pub fn is_compressed(&self) -> bool {
        self.object_header.is_compressed()
    }

    pub fn inlined_cursor(&self) -> Option<InlinedCursor> {
        let inlined_offset = self.entry_offset?;
        let cursor = match self.n_entries?.get() {
            1 => None,
            n => {
                let total_items = unsafe { NonZeroUsize::new_unchecked(n as usize - 1) };
                Some(Cursor::at_head(List::new(
                    self.entry_array_offset?,
                    total_items,
                )))
            }
        };
        Some(InlinedCursor::new(inlined_offset, cursor))
    }
}

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable, PartialEq, Eq)]
#[repr(C)]
pub struct CompactDataFields {
    pub tail_entry_array_offset: u32,
    pub tail_entry_array_n_entries: u32,
}

#[derive(PartialEq, Eq)]
pub enum DataPayloadType<B: ByteSlice> {
    Regular(B),
    Compact {
        compact_fields: Ref<B, CompactDataFields>,
        payload: B,
    },
}

impl<B: ByteSlice> std::fmt::Debug for DataPayloadType<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DataPayloadType::Regular(payload) => write!(f, "Regular({} bytes)", payload.len()),
            DataPayloadType::Compact {
                compact_fields,
                payload,
            } => write!(
                f,
                "Compact(fields: {:?}, payload: {} bytes)",
                compact_fields,
                payload.len()
            ),
        }
    }
}

// Complete Data Object structure
pub struct DataObject<B: ByteSlice> {
    pub header: Ref<B, DataObjectHeader>,
    pub payload: DataPayloadType<B>,
}

impl<B: ByteSlice> std::fmt::Debug for DataObject<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("DataObject")
            .field("header", &self.header)
            .field("payload", &self.payload)
            .finish()
    }
}

impl<B: SplitByteSlice> JournalObject<B> for DataObject<B> {
    fn from_data(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, remaining_data) = data
            .split_at(std::mem::size_of::<DataObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let payload = if is_compact {
            let (fields_data, payload_data) = remaining_data
                .split_at(std::mem::size_of::<CompactDataFields>())
                .ok()?;

            let compact_fields = zerocopy::Ref::from_bytes(fields_data).ok()?;

            DataPayloadType::Compact {
                compact_fields,
                payload: payload_data,
            }
        } else {
            DataPayloadType::Regular(remaining_data)
        };

        Some(DataObject { header, payload })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for DataObject<B> {
    fn from_data_mut(data: B, is_compact: bool) -> Option<Self> {
        let (header_data, remaining_data) = data
            .split_at(std::mem::size_of::<DataObjectHeader>())
            .ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;

        let payload = if is_compact {
            let (fields_data, payload_data) = remaining_data
                .split_at(std::mem::size_of::<CompactDataFields>())
                .ok()?;

            let compact_fields = zerocopy::Ref::from_bytes(fields_data).ok()?;

            DataPayloadType::Compact {
                compact_fields,
                payload: payload_data,
            }
        } else {
            DataPayloadType::Regular(remaining_data)
        };

        Some(DataObject { header, payload })
    }
}

impl<B: ByteSlice> DataObject<B> {
    pub fn raw_payload(&self) -> &[u8] {
        match &self.payload {
            DataPayloadType::Regular(payload) => payload,
            DataPayloadType::Compact { payload, .. } => payload,
        }
    }

    pub fn inlined_cursor(&self) -> Option<InlinedCursor> {
        self.header.inlined_cursor()
    }

    pub fn is_compressed(&self) -> bool {
        self.header.is_compressed()
    }

    pub fn xz_compressed(&self) -> bool {
        self.header.xz_compressed()
    }

    pub fn lz4_compressed(&self) -> bool {
        self.header.lz4_compressed()
    }

    pub fn zstd_compressed(&self) -> bool {
        self.header.zstd_compressed()
    }

    pub fn decompress(&self, buf: &mut Vec<u8>) -> Result<usize> {
        debug_assert!(self.is_compressed());

        if self.zstd_compressed() {
            use ruzstd::decoding::StreamingDecoder;
            use ruzstd::io::Read;

            let payload = self.raw_payload();
            let mut decoder =
                StreamingDecoder::new(payload).map_err(|_| JournalError::DecompressorError)?;

            buf.clear();
            decoder
                .read_to_end(buf)
                .map_err(|_| JournalError::DecompressorError)
        } else if self.lz4_compressed() {
            let payload = self.raw_payload();

            // First 8 bytes are the uncompressed size (little-endian u64)
            if payload.len() < 8 {
                return Err(JournalError::DecompressorError);
            }

            let uncompressed_size =
                u64::from_le_bytes(payload[..8].try_into().unwrap()) as usize;
            let compressed_data = &payload[8..];

            buf.clear();
            buf.resize(uncompressed_size, 0);

            lz4_flex::block::decompress_into(compressed_data, buf)
                .map_err(|_| JournalError::DecompressorError)
        } else if self.xz_compressed() {
            use lzma_rust2::XzReader;
            use std::io::Read;

            let payload = self.raw_payload();
            let mut decoder = XzReader::new(payload, false);

            buf.clear();
            decoder
                .read_to_end(buf)
                .map_err(|_| JournalError::DecompressorError)
        } else {
            Err(JournalError::UnknownCompressionMethod)
        }
    }
}

// SHA-256 HMAC is 32 bytes (256 bits)
pub const TAG_LENGTH: usize = 256 / 8;

#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct TagObjectHeader {
    pub object_header: ObjectHeader,
    pub seqnum: u64,
    pub epoch: u64,
    pub tag: [u8; TAG_LENGTH], // SHA-256 HMAC
}

pub struct TagObject<B: ByteSlice> {
    pub header: Ref<B, TagObjectHeader>,
}

impl<B: ByteSlice> std::fmt::Debug for TagObject<B> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("TagObject")
            .field("header", &self.header)
            .finish()
    }
}

impl<B: SplitByteSlice> JournalObject<B> for TagObject<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let header = zerocopy::Ref::from_bytes(data).ok()?;
        Some(TagObject { header })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for TagObject<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let header = zerocopy::Ref::from_bytes(data).ok()?;
        Some(TagObject { header })
    }
}

impl<B: ByteSlice> TagObject<B> {
    // Helper function to format tag as hex string
    pub fn tag_as_hex(&self) -> String {
        self.header
            .tag
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect()
    }
}
