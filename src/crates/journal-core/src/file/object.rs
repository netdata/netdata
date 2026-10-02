//! On-disk object types of the systemd-journal format, byte-pinned to
//! systemd's journal-def.h (src/libsystemd/sd-journal/ in the systemd
//! tree) and parsed in place with zerocopy - no copies, no re-encoding.
//! Everything here is format surface: changing a field, its order or a
//! `#[repr]` changes the file format, and the near-twin
//! src/crates/jf/journal_file/src/object.rs has to change with it (its
//! `HashableObject` is thinner - `get_payload`, no `is_compressed`/
//! `decompress` - and its `JournalState` lacks `TryFrom`/`Display`).
//!
//! Shape, shared by every stored object: a 16-byte [`ObjectHeader`] (type
//! byte, per-object flags, reserved, size), the family header, then the
//! payload. Objects sit at 8-byte-aligned offsets in the arena after the
//! file header (the `OBJECT_ALIGNMENT` checks in `file/file.rs`), and
//! `size` covers the whole object,
//! rounded up to 8 ([`ObjectHeader::aligned_size`]). [`JournalHeader`] is
//! only the leading prefix of systemd's header: `open()` maps exactly
//! `size_of::<JournalHeader>()` bytes and checks the magic, so files whose
//! on-disk `header_size` is larger (see the commented-out fields below)
//! still parse
//! ([`JournalFile::open`](crate::file::JournalFile::open)).
//!
//! The two hash tables are objects too ([`DataHashTable`],
//! [`FieldHashTable`]): an [`ObjectHeader`] plus one [`HashItem`] (head/
//! tail offsets) per bucket, bucket = `hash % n_buckets`;
//! `next_hash_offset` chains a bucket's objects from its head.
//! [`EntryObject`]s list the data objects they reference, and the
//! entry-array chain ([`OffsetArrayObject`], walked by
//! file/offset_array.rs) orders entries for iteration.
//!
//! Two header-flag switches change encodings:
//! - `HeaderIncompatibleFlags::Compact`: u32 offsets for entry items
//!   ([`CompactEntryItem`]) and offset-array slots
//!   ([`OffsetsType::Compact`]), plus an 8-byte [`CompactDataFields`]
//!   prefix on data-object payloads. The hash tables ignore the flag.
//! - Compression: data-object payloads may be xz/lz4/zstd
//!   ([`ObjectFlags`], [`DataObject::decompress`]). This crate's writer
//!   sets neither Compact nor compression; both exist so files written by
//!   systemd still parse.
//!
//! Consumers (grep-verified): file/file.rs parses and bounds-checks every
//! object through [`JournalObject`] (its `from_data`/`from_data_mut` run
//! inside `JournalFile::journal_object_ref`/`journal_object_mut`, with
//! `is_compact` taken from the file header) and
//! stamps/validates the type byte (in `journal_object_mut`);
//! file/writer.rs builds and chains the objects (via its flat
//! `file/mod.rs` imports); file/value_guard.rs forwards the traits
//! through its one-borrow guard. `mod object` is private; other crates get
//! `EntryItemsType`, `HashableObject`, `HeaderIncompatibleFlags` and
//! `JournalState` (re-exported in `file/mod.rs`) plus `DataObject`
//! (also re-exported there),
//! the rest only through the crate-internal glob;
//! journal-index parses `DataObject` payloads for field values
//! (journal-index/src/field_types.rs, decompressing in
//! journal-index/src/file_index.rs).
use crate::error::{JournalError, Result};
use crate::file::offset_array::{Cursor, InlinedCursor, List};
use std::num::{NonZeroU32, NonZeroU64, NonZeroUsize};
use zerocopy::{
    ByteSlice, ByteSliceMut, FromBytes, Immutable, IntoBytes, KnownLayout, Ref, SplitByteSlice,
    SplitByteSliceMut,
};

/// Read-side view of an object that lives in a hash table: what bucket
/// walks and dedup-on-write need. The writer side is
/// `HashableObjectMut`; `ValueGuard<T>` implements it too
/// (file/value_guard.rs), so bucket visitors can work on guards.
pub trait HashableObject {
    /// Hash stored in the object header at write time, over the payload
    /// bytes (keyed or legacy per the file's `KeyedHash` flag, file/hash.rs).
    fn hash(&self) -> u64;

    /// The payload bytes as stored - compressed form when the object's
    /// compression flags say so. Dedup compares these raw bytes plus the
    /// recomputed hash (the `PayloadMatcher` comparison in `file/file.rs`),
    /// so a compressed object cannot
    /// match a fresh uncompressed payload.
    fn raw_payload(&self) -> &[u8];

    /// Check if the payload is compressed
    fn is_compressed(&self) -> bool;

    /// Decompress the payload into the provided buffer.
    /// Returns the number of decompressed bytes.
    fn decompress(&self, buf: &mut Vec<u8>) -> Result<usize>;

    /// Offset of the next object in the same hash-table bucket chain,
    /// `None` at the chain end.
    fn next_hash_offset(&self) -> Option<NonZeroU64>;

    /// The object type byte this family stamps and validates
    /// (stamped/required by `JournalFile::journal_object_mut`).
    fn object_type() -> ObjectType;
}

/// Write-side counterpart of [`HashableObject`]: link a freshly appended
/// object into its bucket chain and fill its payload slot. The writer
/// links tail-to-new through `set_next_hash_offset`
/// (the writer's `*_hash_table_set_tail_offset` append path).
pub trait HashableObjectMut: HashableObject {
    /// Set the offset to the next object in the hash chain
    fn set_next_hash_offset(&mut self, offset: NonZeroU64);

    /// Overwrite the payload slot the object was sized for; `data` must
    /// match the slot's existing length or `copy_from_slice` panics.
    fn set_payload(&mut self, data: &[u8]);
}

/// Bucket access over a file-backed hash table: `hash_item_ref` selects
/// the bucket for `hash % len` - the format's bucket rule - and callers
/// walk its object chain from there
/// ([`JournalFile::visit_bucket`](crate::file::JournalFile::visit_bucket)).
pub trait HashTable {
    /// The type of objects stored in this hash table
    type Object: HashableObject;

    /// Get the hash item for a given hash value
    fn hash_item_ref(&self, hash: u64) -> &HashItem;

    /// Get the length of the hash table (number of buckets)
    fn len(&self) -> usize;

    /// Never called: exists only to satisfy clippy's `len_without_is_empty`;
    /// the default body panics.
    fn is_empty(&self) -> bool {
        todo!()
    }
}

/// Mutable bucket access for appends: the writer updates a bucket's
/// head/tail here when chaining a new object (its
/// `*_hash_table_set_tail_offset` appends).
pub trait HashTableMut: HashTable {
    /// Get a mutable reference to the hash item for a given hash value
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem;
}

/// The data hash table object: an [`ObjectHeader`] followed by one
/// [`HashItem`] per bucket. Buckets index [`DataObject`]s by the hash of
/// their full `NAME=VALUE` payload (hashed by `JournalWriter::add_data`);
/// lookups walk a
/// bucket from its `head_hash_offset`
/// ([`JournalFile::visit_bucket`](crate::file::JournalFile::visit_bucket)).
/// The layout is
/// identical with or without the COMPACT flag (`from_data` ignores it).
pub struct DataHashTable<B: ByteSlice> {
    pub header: Ref<B, ObjectHeader>,
    pub items: Ref<B, [HashItem]>,
}

/// The field hash table object, same shape as [`DataHashTable`] but
/// indexing [`FieldObject`]s by the hash of the bare field name
/// (hashed by `JournalWriter::add_field`).
pub struct FieldHashTable<B: ByteSlice> {
    pub header: Ref<B, ObjectHeader>,
    pub items: Ref<B, [HashItem]>,
}

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

impl<B: ByteSliceMut> HashTableMut for DataHashTable<B> {
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem {
        let bucket_index = hash as usize % self.items.len();
        &mut self.items[bucket_index]
    }
}

impl<B: ByteSliceMut> HashTableMut for FieldHashTable<B> {
    fn hash_item_mut(&mut self, hash: u64) -> &mut HashItem {
        let bucket_index = hash as usize % self.items.len();
        &mut self.items[bucket_index]
    }
}

impl<B: SplitByteSlice> JournalObject<B> for DataHashTable<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(DataHashTable { header, items })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for DataHashTable<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(DataHashTable { header, items })
    }
}

impl<B: SplitByteSlice> JournalObject<B> for FieldHashTable<B> {
    fn from_data(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(FieldHashTable { header, items })
    }
}

impl<B: SplitByteSliceMut> JournalObjectMut<B> for FieldHashTable<B> {
    fn from_data_mut(data: B, _is_compact: bool) -> Option<Self> {
        let (header_data, items_data) = data.split_at(std::mem::size_of::<ObjectHeader>()).ok()?;

        let header = zerocopy::Ref::from_bytes(header_data).ok()?;
        let items = zerocopy::Ref::from_bytes(items_data).ok()?;

        Some(FieldHashTable { header, items })
    }
}

/// Field payloads are never compressed: `is_compressed` is false and
/// `decompress` copies the bytes verbatim.
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

/// `set_payload` fills the slot the object was sized for
/// (the writer's `add_data`); compact objects write after the
/// [`CompactDataFields`] prefix and leave it untouched. A length mismatch
/// panics in `copy_from_slice`.
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

/// Parses an object's bytes into a zerocopy view over the slice, without
/// copying. `is_compact` comes from the file header's Compact flag and
/// selects the payload/item encoding (see the family docs); hash-table
/// objects ignore it. `None` means the slice is too short or misaligned -
/// callers surface that as `JournalError::ZerocopyFailure`
/// (`JournalFile::journal_object_ref`/`journal_object_mut`).
pub trait JournalObject<B: SplitByteSlice>: Sized {
    /// Create a new journal object from a byte slice
    fn from_data(data: B, is_compact: bool) -> Option<Self>;
}

pub trait JournalObjectMut<B: SplitByteSliceMut>: JournalObject<B> {
    /// Mutable variant of [`JournalObject::from_data`], over writable bytes.
    fn from_data_mut(data: B, is_compact: bool) -> Option<Self>;
}

/// Bits of the journal header's `incompatible_flags` (systemd's
/// HEADER_INCOMPATIBLE_*): features a reader must understand to open the
/// file. [`HeaderIncompatibleFlags::KeyedHash`] switches object hashing to
/// SipHash-2-4 keyed by the header `file_id` (file/hash.rs); set at create
/// ([`JournalFile::create`](crate::file::JournalFile::create)), read to
/// pick the hash mode ([`JournalFile::hash`](crate::file::JournalFile::hash))
/// and asserted on every entry write
/// ([`JournalWriter::add_entry`](crate::file::JournalWriter::add_entry)).
/// [`HeaderIncompatibleFlags::Compact`] selects the compact encodings
/// (module docs). The compressed-Xz/Lz4/Zstd bits are set only by writers
/// that compress - not this crate's - and are consumed by
/// [`DataObject::decompress`].
pub enum HeaderIncompatibleFlags {
    CompressedXz = 1 << 0,
    CompressedLz4 = 1 << 1,
    KeyedHash = 1 << 2,
    CompressedZstd = 1 << 3,
    Compact = 1 << 4,
}

/// Bits of the journal header's `compatible_flags` (systemd's
/// HEADER_COMPATIBLE_*): optional features a reader may ignore. `Sealed`
/// marks an HMAC-sealed file, `TailEntryBootId` per-entry boot-id
/// tracking. Declared for format completeness; nothing in this crate
/// checks compatible flags (grep-verified).
pub enum HeaderCompatibleFlags {
    Sealed = 1 << 0,
    TailEntryBootId = 1 << 1,
}

/// The journal header's state byte: offline (no writer attached), online
/// (open), archived (rotated away). [`TryFrom`] rejects anything else with
/// `InvalidJournalFileState`; [`Display`](std::fmt::Display) prints the
/// uppercase names. Set to Online at create
/// ([`JournalFile::create`](crate::file::JournalFile::create));
/// journal-log-writer sets Archived on rotation
/// (journal-log-writer/src/log/mod.rs) and journal-index reads it
/// when opening a file for indexing
/// (journal-index/src/file_indexer.rs).
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

/// The journal file header, the first bytes of the file. `open()` maps
/// exactly `size_of::<JournalHeader>()` bytes and checks the signature
/// ([`JournalFile::open`](crate::file::JournalFile::open)), so a file whose
/// on-disk `header_size` is larger
/// - systemd appended fields over the years, see the block below - still
/// parses; arena bounds use the on-disk value (read by
/// `journal_object_ref`/`journal_object_mut`). At
/// create the writer lays out header, data hash table, field hash table
/// and sets `header_size` to this struct's size
/// ([`JournalFile::create`](crate::file::JournalFile::create)).
/// The writer resumes appends from `tail_object_offset`
/// ([`JournalWriter::new`](crate::file::JournalWriter::new)).
#[derive(Default, Debug, Clone, Copy, FromBytes, IntoBytes, Immutable, KnownLayout)]
#[repr(C)]
pub struct JournalHeader {
    pub signature: [u8; 8],                          // magic "LPKSHHRH" (validated on open)
    pub compatible_flags: u32,                       // optional features: HeaderCompatibleFlags
    pub incompatible_flags: u32,                     // required features: HeaderIncompatibleFlags
    pub state: u8,                                   // JournalState: 0/1/2
    pub reserved: [u8; 7],                           // Reserved
    pub file_id: [u8; 16],                           // file id; the keyed-hash key (file/hash.rs)
    pub machine_id: [u8; 16],                        // machine id, from JournalFileOptions
    pub tail_entry_boot_id: [u8; 16],                // last entry's boot id (stamped by JournalWriter)
    pub seqnum_id: [u8; 16],                         // seqnum-space id (returned by JournalReader::get_seqnum)
    pub header_size: u64,                            // header size; arena after
    pub arena_size: u64,                             // arena size (advanced per object by JournalWriter)
    pub data_hash_table_offset: Option<NonZeroU64>,  // data hash table offset (laid out by JournalFile::create)
    pub data_hash_table_size: Option<NonZeroU64>,    // buckets * sizeof(HashItem)
    pub field_hash_table_offset: Option<NonZeroU64>, // field hash table offset (laid out by JournalFile::create)
    pub field_hash_table_size: Option<NonZeroU64>,   // buckets * sizeof(HashItem)
    pub tail_object_offset: Option<NonZeroU64>,      // last object (committed by JournalWriter::new)
    pub n_objects: u64,                              // object count (committed per entry by JournalWriter)
    pub n_entries: u64,                              // entry count (committed per entry by JournalWriter)
    pub tail_entry_seqnum: u64,                      // newest entry's seqnum (stamped per entry)
    pub head_entry_seqnum: u64,                      // oldest entry's seqnum (stamped on the first entry)
    pub entry_array_offset: Option<NonZeroU64>,      // entry-array chain head (read by JournalFile::entry_list)
    pub head_entry_realtime: u64,                    // oldest entry's realtime (stamped on the first entry)
    pub tail_entry_realtime: u64,                    // newest entry's realtime (stamped per entry)
    pub tail_entry_monotonic: u64,                   // newest entry monotonic (stamped per entry)
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

/// Per-object compression bits in [`ObjectHeader::flags`] (systemd's
/// OBJECT_COMPRESSED_*). Note the positions differ from the header-level
/// [`HeaderIncompatibleFlags`]: zstd is bit 2 here, bit 3 there.
/// Consulted by [`ObjectHeader::is_compressed`] and
/// [`DataObject::decompress`].
pub enum ObjectFlags {
    CompressedXz = 1 << 0,
    CompressedLz4 = 1 << 1,
    CompressedZstd = 1 << 2,
}

/// The type byte at the head of every object ([`ObjectHeader::type_`]),
/// matching systemd's OBJECT_* values. Each family's accessors stamp it on
/// append and require it back on reopen
/// (`JournalFile::journal_object_mut`); unknown
/// values are rejected by the `TryFrom` below.
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

/// The 16-byte prefix of every object in the file: type byte (see
/// [`ObjectType`]), per-object flags ([`ObjectFlags`]), 6 reserved bytes,
/// then the full object size including this header. The writer stamps
/// type and size on append and re-validates them on reopen
/// (`JournalFile::journal_object_mut`).
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

    /// The object's on-disk footprint: `size` rounded up to the 8-byte
    /// object alignment (`OBJECT_ALIGNMENT` in `file/file.rs`). Writers
    /// advance the append offset by this.
    pub fn aligned_size(&self) -> u64 {
        (self.size + 7) & !7
    }

    /// Rejects a size smaller than the header itself before callers map or
    /// parse the object - the corruption guard used by the generic object
    /// readers (`journal_object_ref`/`journal_object_mut`). Returns the
    /// size on success.
    pub fn validated_size(&self) -> crate::error::Result<u64> {
        let min_size = std::mem::size_of::<ObjectHeader>() as u64;

        if self.size < min_size {
            return Err(crate::error::JournalError::InvalidObjectSize(self.size));
        }

        Ok(self.size)
    }
}

/// Field-object header. `hash`/`next_hash_offset` place the field name in
/// its field-hash-table bucket; `head_data_offset` points at the newest
/// data object sharing this field name, the head of the list threaded
/// through `DataObjectHeader::next_field_offset`
/// (maintained by `JournalWriter::add_data`, walked by
/// `JournalFile::field_data_objects`). The payload that
/// follows is the bare field name, e.g. `MESSAGE`.
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct FieldObjectHeader {
    pub object_header: ObjectHeader,
    pub hash: u64,
    pub next_hash_offset: Option<NonZeroU64>,
    pub head_data_offset: Option<NonZeroU64>,
}

/// Entry-array object header: the [`ObjectHeader`] plus the link to the
/// next array in the chain; the offset slots follow ([`OffsetsType`]).
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct OffsetArrayObjectHeader {
    pub object_header: ObjectHeader,
    pub next_offset_array: Option<NonZeroU64>,
}

/// One hash-table bucket: head and tail offsets of the chain of objects
/// hashing to this bucket. A zero (unset) pair is an empty bucket;
/// appends link the old tail to the new object and move the tail
/// (the writer's `*_hash_table_set_tail_offset` appends).
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct HashItem {
    pub head_hash_offset: Option<NonZeroU64>,
    pub tail_hash_offset: Option<NonZeroU64>,
}

/// A field object: a field name registered once per file and hashed into
/// the field hash table. The slice after the header is the payload - the
/// bare field name itself.
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

/// The offset slots of an entry-array object: u64 offsets in regular
/// files, u32 in compact ones (the header COMPACT flag). A zero slot reads
/// as `None` - the format's unset pointer - and ends iteration there
/// (unset slots are skipped by `file/offset_array.rs`'s collects).
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
    /// Stores `offset` at `index`. Compact slots keep only the low 32
    /// bits: an offset whose low 32 bits are zero (every 4 GiB boundary)
    /// is written as an unset slot.
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

/// One entry-array object: a chain node listing offsets - the file's
/// entry chain (from `JournalHeader::entry_array_offset`, via
/// `JournalFile::entry_list`)
/// or a data object's referencing-entry chain
/// ([`DataObjectHeader::inlined_cursor`]). `next_offset_array` links to
/// the next node and the [`OffsetsType`] slots follow. Chains are full
/// except the last node, which is why `len` takes the caller's
/// remaining-item count.
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

    /// Valid items in this node: its capacity, capped by
    /// `remaining_items` because only the chain's last node may be
    /// partially filled (see `remaining_items` in `file/offset_array.rs`).
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

// The struct name below is copy-pasted from JournalHeader's Debug impl:
// entry-array nodes print as "JournalHeader".
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

/// Entry-object header: the entry's sequence number and timestamps, the
/// boot it happened in, and `xor_hash` - the XOR of the items' Jenkins
/// payload hashes, computed key-independently so it matches across keyed
/// and legacy files (computed in `JournalWriter::add_entry`; algorithms in
/// file/hash.rs).
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct EntryObjectHeader {
    pub object_header: ObjectHeader,
    pub seqnum: u64,
    pub realtime: u64,
    pub monotonic: u64,
    pub boot_id: [u8; 16], // The boot the entry belongs to (16-byte id)
    pub xor_hash: u64,
}

/// One item of a regular (non-compact) entry object: the referenced
/// object's offset plus its hash. The item hash is written per the
/// format; this crate only ever reads the offsets back.
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct RegularEntryItem {
    pub object_offset: u64,
    pub hash: u64,
}

/// One item of a compact entry object: only the referenced object's
/// offset, as u32 - `EntryItemsType::set` panics on larger offsets.
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct CompactEntryItem {
    pub object_offset: u32,
}

/// The item array of an `EntryObject`, regular or compact per the
/// header COMPACT flag.
pub enum EntryItemsType<B: ByteSlice> {
    Regular(Ref<B, [RegularEntryItem]>),
    Compact(Ref<B, [CompactEntryItem]>),
}

impl<B: ByteSliceMut> EntryItemsType<B> {
    /// Writer-side item store. Regular items take `(offset, Some(hash))` -
    /// the hash is unwrapped, `None` panics. Compact items take
    /// `(offset, None)` and panic on offsets past the u32 slot.
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

/// A journal entry: the [`EntryObjectHeader`] plus the array of offsets
/// to the data objects it references, regular or compact per the header
/// COMPACT flag. Entries themselves are ordered by the entry-array chain,
/// not by a hash table.
pub struct EntryObject<B: ByteSlice> {
    pub header: Ref<B, EntryObjectHeader>,
    pub items: EntryItemsType<B>,
}

impl<B: ByteSlice> EntryObject<B> {
    /// Pushes every item's object offset onto `offsets`, in item order; a
    /// zero item offset errors with `InvalidOffset`.
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

/// Data-object header: the object's hash and bucket-chain link, the
/// same-field list link (`next_field_offset`; head in
/// `FieldObjectHeader::head_data_offset`, linked newest-first by
/// `JournalWriter::add_data`), and the links to the entries referencing this
/// payload (see [`DataObjectHeader::inlined_cursor`]).
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

    /// Cursor over the entries referencing this data object. With one
    /// reference (`n_entries == 1`) it is just `entry_offset`; with more,
    /// `entry_offset` plus an entry-array chain holding the remaining
    /// `n_entries - 1` offsets. `None` when the links are unset (zero),
    /// i.e. the data object is not referenced by any entry yet. Consumed
    /// by `JournalFile::data_object_directed_partition_point`,
    /// `JournalFile::load_fields`, the filter's match resolution
    /// (`file/filter.rs`) and journal-index's indexer.
    pub fn inlined_cursor(&self) -> Option<InlinedCursor> {
        let inlined_offset = self.entry_offset?;
        let cursor = match self.n_entries?.get() {
            1 => None,
            n => {
                // n >= 2 here (n_entries is NonZero and not 1), so n - 1
                // fits NonZeroUsize and the unchecked constructor is safe.
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

/// The 8-byte prefix a compact data object carries ahead of its payload:
/// the tail of this object's entry-array chain, mirroring the header's
/// v252 tail-entry-array fields (systemd journal-def.h CompactDataField).
/// Parsed but never read by this crate.
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable, PartialEq, Eq)]
#[repr(C)]
pub struct CompactDataFields {
    pub tail_entry_array_offset: u32,
    pub tail_entry_array_n_entries: u32,
}

/// The payload slice of a [`DataObject`]: the whole rest of the object for
/// regular files; for compact ones the 8-byte [`CompactDataFields`] prefix
/// is split off first and only the remainder is the payload.
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

/// A data object: the payload is one `NAME=VALUE` record (the writer
/// splits the field name off at the first `=` in `add_data`), stored
/// possibly compressed per the object-header flags, with the header
/// linking it into its hash bucket, its field's data list and its
/// referencing entries.
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

    /// Decompress the stored payload per the object's compression flags:
    /// zstd via `ruzstd`, lz4 with the format's 8-byte little-endian
    /// uncompressed-length prefix, xz via `lzma_rust2`. Clears `buf` first
    /// and returns the decompressed length; decode failures are
    /// `DecompressorError`, an object with no compression flag is
    /// `UnknownCompressionMethod`.
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

            // Format framing (systemd): 8-byte little-endian uncompressed
            // size, then the lz4 block.
            if payload.len() < 8 {
                return Err(JournalError::DecompressorError);
            }

            let uncompressed_size = u64::from_le_bytes(payload[..8].try_into().unwrap()) as usize;
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

/// SHA-256 HMAC size in bytes (systemd's TAG_LENGTH).
pub const TAG_LENGTH: usize = 256 / 8;

/// A tag object: the HMAC-SHA256 seal of the file at a seqnum and epoch
/// (systemd's sealing feature). This crate parses tags but never writes or
/// verifies them.
#[derive(Debug, Copy, Clone, FromBytes, IntoBytes, KnownLayout, Immutable)]
#[repr(C)]
pub struct TagObjectHeader {
    pub object_header: ObjectHeader,
    pub seqnum: u64,
    pub epoch: u64,
    pub tag: [u8; TAG_LENGTH], // the HMAC-SHA256 bytes (TAG_LENGTH)
}

/// Header-only view of a tag object. Unlike the payload-carrying families,
/// parsing uses `Ref::from_bytes`, so the slice must be exactly
/// `TagObjectHeader`-sized (64 bytes).
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
    // The raw HMAC bytes as lowercase hex.
    pub fn tag_as_hex(&self) -> String {
        self.header
            .tag
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect()
    }
}
