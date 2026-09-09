//! Chunk-based file formats: zero-copy TOC codec + integrity container.
//!
//! The foundation for on-disk formats assembled from independently
//! addressable chunks — index files, catalogs, any artifact whose
//! reader wants O(1) random access to named sections of an mmap.
//! Two layers:
//!
//! - **Raw TOC codec** (this module): a table of contents mapping
//!   4-byte chunk IDs to byte ranges within a file. Designed for
//!   memory-mapped files: the TOC is parsed zero-copy via [`zerocopy`],
//!   and [`ChunkMeta`] exposes raw offsets so callers can issue
//!   `madvise` hints per chunk.
//! - **Integrity container** ([`container`]): a self-describing file
//!   framing on top of the TOC — magic + version + num_chunks header
//!   and a crc32 trailer per chunk — with a buffer-all builder and a
//!   streaming `Write + Seek` writer. Most consumers want this layer;
//!   the raw codec is for formats that bring their own header and
//!   integrity story.
//!
//! # On-disk TOC layout
//!
//! For N chunks the TOC occupies `(N + 1) × 12` bytes:
//!
//! ```text
//! entry[0]:  id(4) + offset_le(8)    chunk 0 starts at offset
//! entry[1]:  id(4) + offset_le(8)    chunk 1 starts at offset
//! ...
//! entry[N]:  0000 + offset_le(8)     end marker (zero id, offset = end of last chunk)
//! ```
//!
//! Offsets are little-endian and relative to one shared origin: the
//! start of the slice handed to [`Toc::parse`], which is the same
//! origin `base_offset` named at write time
//! ([`TocWriter::write_toc`]). For a standalone file that origin is
//! byte 0; for a TOC embedded in a larger file it is wherever the
//! embedding starts. The TOC may sit at any `toc_offset` past that
//! origin (the container places it after its 12-byte header); chunk
//! bodies follow the TOC.
//!
//! ```
//! use chunk_file::{Toc, TocWriter};
//!
//! // Plan, then stream chunk bodies in plan order.
//! let mut writer = TocWriter::new();
//! writer.plan(*b"HEAD", 3);
//! writer.plan(*b"BODY", 5);
//! let mut file = Vec::new();
//! let mut chunks = writer.write_toc(&mut file, 0)?;
//! chunks.write_chunk(*b"HEAD", b"abc")?;
//! chunks.write_chunk(*b"BODY", b"defgh")?;
//! chunks.finish()?;
//!
//! let toc = Toc::parse(&file, 0, 2)?;
//! assert_eq!(toc.data(&file, *b"BODY")?, b"defgh");
//! # Ok::<(), chunk_file::Error>(())
//! ```

pub mod container;

use std::collections::VecDeque;
use std::io::Write;

use zerocopy::{FromBytes, Immutable, IntoBytes, KnownLayout, Ref};

// ── Types ────────────────────────────────────────────────────────

/// A 4-byte chunk identifier (e.g. `*b"PRIM"`).
pub type ChunkId = [u8; 4];

/// The reserved end-marker id closing every TOC.
pub const END_MARKER_ID: ChunkId = [0; 4];

/// A single on-disk TOC entry.
///
/// Exactly 12 bytes with alignment 1.  The offset is stored as `[u8; 8]`
/// rather than `u64` so `#[repr(C)]` introduces no padding.
#[derive(FromBytes, IntoBytes, Immutable, KnownLayout, Copy, Clone, Debug)]
#[repr(C)]
pub struct TocEntry {
    /// Chunk identifier.  For the end-marker entry this is [`END_MARKER_ID`].
    pub id: ChunkId,
    /// Byte offset from the TOC's origin (see the crate docs),
    /// little-endian.
    offset_le: [u8; 8],
}

const _: () = assert!(size_of::<TocEntry>() == 12);
const _: () = assert!(align_of::<TocEntry>() == 1);

impl TocEntry {
    /// Create a new entry.
    #[inline]
    pub const fn new(id: ChunkId, offset: u64) -> Self {
        Self {
            id,
            offset_le: offset.to_le_bytes(),
        }
    }

    /// Read the offset as a native `u64`.
    #[inline]
    pub const fn offset(&self) -> u64 {
        u64::from_le_bytes(self.offset_le)
    }
}

/// A chunk's location and size within a file.
///
/// Exposes raw byte offsets so callers can compute memory ranges for
/// `madvise(MADV_WILLNEED)`, `madvise(MADV_SEQUENTIAL)`, etc.
#[derive(Copy, Clone, Debug, PartialEq, Eq)]
pub struct ChunkMeta {
    /// Chunk identifier.
    pub id: ChunkId,
    /// Byte offset of the chunk's first byte, from the TOC's origin
    /// (see the crate docs).
    pub offset: u64,
    /// Byte length of the chunk data.
    pub size: u64,
}

// ── Error ────────────────────────────────────────────────────────

#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The slice ends before the TOC does — a truncated file, or a
    /// `num_chunks` larger than the file can hold.
    #[error("TOC needs {expected} bytes, got {actual}")]
    TocTooShort { expected: usize, actual: usize },

    /// [`Toc::data`] found no entry with the requested id.
    #[error("chunk {id:?} not found")]
    ChunkNotFound { id: ChunkId },

    /// A TOC entry's offset points past the end of the file.
    #[error("offset {offset} at entry {index} exceeds data length {data_len}")]
    OffsetOutOfBounds {
        index: usize,
        offset: u64,
        data_len: usize,
    },

    /// TOC offsets must be strictly increasing (chunk sizes are the
    /// deltas between consecutive entries).
    #[error("non-monotonic offset at entry {index}: {prev} >= {curr}")]
    NonMonotonicOffset { index: usize, prev: u64, curr: u64 },

    /// The first chunk's offset points inside the TOC itself.
    #[error("first chunk offset {offset} overlaps the TOC (chunks start at {chunks_start})")]
    ChunkOverlapsToc { offset: u64, chunks_start: u64 },

    /// Two TOC entries carry the same id; lookups by id would be
    /// ambiguous. `index` is the later occurrence of the
    /// lexicographically-first duplicated id.
    #[error("duplicate chunk id {id:?} at entry {index}")]
    DuplicateChunkId { id: ChunkId, index: usize },

    /// The closing TOC entry doesn't carry [`END_MARKER_ID`].
    #[error("end-marker entry carries a non-zero id {id:?}")]
    BadEndMarker { id: ChunkId },

    /// A chunk entry (not the closing one) carries the reserved
    /// [`END_MARKER_ID`] — such a chunk would be unaddressable.
    #[error("chunk entry {index} uses the reserved zero end-marker id")]
    ReservedChunkId { index: usize },

    /// [`Toc::data`] was handed a `file` slice shorter than the one the
    /// offsets were validated against at parse time — a caller bug,
    /// surfaced as an error rather than an out-of-bounds panic.
    #[error("chunk {id:?} span ends at {end} but the file slice holds {file_len} bytes")]
    ChunkOutOfBounds {
        id: ChunkId,
        end: u64,
        file_len: usize,
    },

    /// [`ChunkWriter::write_chunk`] was called after every planned
    /// chunk had already been written.
    #[error("chunk {id:?} was not planned (all planned chunks already written)")]
    UnplannedChunk { id: ChunkId },

    /// [`ChunkWriter::write_chunk`] was called out of plan order.
    #[error("expected chunk {expected:?}, got {got:?}")]
    WrongChunkId { expected: ChunkId, got: ChunkId },

    /// A chunk's data doesn't match the size it was planned with.
    #[error("chunk {id:?}: expected {expected} bytes, got {got}")]
    WrongChunkSize {
        id: ChunkId,
        expected: u64,
        got: u64,
    },

    /// [`ChunkWriter::finish`] was called before every planned chunk
    /// was written.
    #[error("{remaining} planned chunks were not written")]
    IncompleteWrite { remaining: usize },

    /// Underlying I/O failed.
    #[error(transparent)]
    Io(#[from] std::io::Error),
}

// ── Toc (zero-copy reader) ──────────────────────────────────────

/// Zero-copy TOC parsed from a memory-mapped file.
///
/// Borrows directly from the underlying byte slice with no allocation.
pub struct Toc<'a> {
    /// N+1 entries: entries[0..N] are chunks, entries[N] is the end marker.
    entries: Ref<&'a [u8], [TocEntry]>,
}

impl std::fmt::Debug for Toc<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Toc")
            .field("num_chunks", &self.num_chunks())
            .finish()
    }
}

impl<'a> Toc<'a> {
    /// The byte size of a TOC with `num_chunks` chunks.
    ///
    /// Plain arithmetic for trusted counts; [`parse`](Toc::parse)
    /// computes the size in `u64` itself so an untrusted on-disk count
    /// can't wrap `usize` on 32-bit targets.
    pub const fn byte_size(num_chunks: usize) -> usize {
        (num_chunks + 1) * size_of::<TocEntry>()
    }

    /// Parse the TOC found at `toc_offset` within `file`, expecting
    /// `num_chunks` chunks. Offsets in the entries are absolute from the
    /// start of `file`.
    ///
    /// This is zero-copy: the returned [`Toc`] borrows directly from
    /// `file`. Validates that every offset is in bounds and strictly
    /// increasing, that the first chunk starts past the TOC itself,
    /// that no chunk id repeats, and that the closing entry carries the
    /// zero end-marker id — so a corrupt TOC is rejected at parse time,
    /// not discovered as a bogus slice later.
    pub fn parse(file: &'a [u8], toc_offset: usize, num_chunks: u32) -> Result<Self, Error> {
        let n = num_chunks as usize;
        // Size the TOC in u64 so a hostile num_chunks can't wrap usize
        // on 32-bit targets (u32::MAX entries × 12 exceeds u32 range);
        // the cast below is safe once the length check bounds it by the
        // file size.
        let toc_size_u64 = (num_chunks as u64 + 1) * size_of::<TocEntry>() as u64;
        if (file.len() as u64) < toc_offset as u64 + toc_size_u64 {
            return Err(Error::TocTooShort {
                expected: usize::try_from(toc_size_u64).unwrap_or(usize::MAX),
                actual: file.len().saturating_sub(toc_offset),
            });
        }
        let toc_size = toc_size_u64 as usize;
        let toc_end = toc_offset + toc_size;

        let toc_bytes = &file[toc_offset..toc_end];
        let entries: Ref<&[u8], [TocEntry]> =
            Ref::from_bytes(toc_bytes).map_err(|_| Error::TocTooShort {
                expected: toc_size,
                actual: toc_bytes.len(),
            })?;

        // Validate: chunks start past the TOC, offsets monotonically
        // increasing and within bounds, end marker id is zero, no
        // duplicate ids.
        let data_len = file.len();
        let num_entries = entries.len(); // N+1
        if entries[0].offset() < toc_end as u64 {
            return Err(Error::ChunkOverlapsToc {
                offset: entries[0].offset(),
                chunks_start: toc_end as u64,
            });
        }
        for i in 0..num_entries {
            let offset = entries[i].offset();
            if offset > data_len as u64 {
                return Err(Error::OffsetOutOfBounds {
                    index: i,
                    offset,
                    data_len,
                });
            }
            if i > 0 && offset <= entries[i - 1].offset() {
                return Err(Error::NonMonotonicOffset {
                    index: i,
                    prev: entries[i - 1].offset(),
                    curr: offset,
                });
            }
            // The zero id is reserved for the end marker; a chunk entry
            // carrying it would be unaddressable (and ambiguous with
            // the marker).
            if i < n && entries[i].id == END_MARKER_ID {
                return Err(Error::ReservedChunkId { index: i });
            }
        }
        if entries[n].id != END_MARKER_ID {
            return Err(Error::BadEndMarker { id: entries[n].id });
        }
        // Duplicate-id check in O(n log n): sort (id, index) pairs and
        // compare neighbors. Today's TOCs hold dozens of entries, but
        // the format itself doesn't bound n — don't let the validator
        // become the ceiling for a future consumer. The tuple's
        // lexicographic Ord sorts by id then index, so equal-id pairs
        // are ordered by index *by the comparator* — the unstable sort
        // is still fully deterministic because the distinct indices
        // make every pair unique (stability only matters for elements
        // that compare equal). The reported index is therefore the
        // later occurrence of the sorted-first duplicated id.
        let mut ids: Vec<(ChunkId, usize)> = (0..n).map(|i| (entries[i].id, i)).collect();
        ids.sort_unstable();
        for pair in ids.windows(2) {
            if pair[0].0 == pair[1].0 {
                return Err(Error::DuplicateChunkId {
                    id: pair[1].0,
                    index: pair[1].1,
                });
            }
        }

        Ok(Toc { entries })
    }

    /// Number of chunks (excludes the end marker).
    pub fn num_chunks(&self) -> usize {
        self.entries.len() - 1
    }

    /// Look up a chunk's metadata by ID.
    pub fn get(&self, id: ChunkId) -> Option<ChunkMeta> {
        let n = self.num_chunks();
        for i in 0..n {
            if self.entries[i].id == id {
                let offset = self.entries[i].offset();
                let end = self.entries[i + 1].offset();
                return Some(ChunkMeta {
                    id,
                    offset,
                    size: end - offset,
                });
            }
        }
        None
    }

    /// Look up a chunk's data slice within `file`.
    ///
    /// The `file` slice is typically the full memory-mapped file — the
    /// same slice [`parse`](Toc::parse) validated the offsets against.
    /// A shorter slice (caller bug) is rejected, never an
    /// out-of-bounds panic.
    pub fn data<'f>(&self, file: &'f [u8], id: ChunkId) -> Result<&'f [u8], Error> {
        let meta = self.get(id).ok_or(Error::ChunkNotFound { id })?;
        let end = meta.offset + meta.size; // u64: cannot wrap for parsed offsets
        let out_of_bounds = || Error::ChunkOutOfBounds {
            id,
            end,
            file_len: file.len(),
        };
        let range = usize::try_from(meta.offset).map_err(|_| out_of_bounds())?
            ..usize::try_from(end).map_err(|_| out_of_bounds())?;
        file.get(range).ok_or_else(out_of_bounds)
    }

    /// Iterate over all chunks' metadata, in TOC order.
    pub fn iter(&self) -> impl Iterator<Item = ChunkMeta> + '_ {
        let n = self.num_chunks();
        (0..n).map(move |i| {
            let offset = self.entries[i].offset();
            let end = self.entries[i + 1].offset();
            ChunkMeta {
                id: self.entries[i].id,
                offset,
                size: end - offset,
            }
        })
    }
}

// ── TocWriter ───────────────────────────────────────────────────

struct PlannedChunk {
    id: ChunkId,
    size: u64,
}

/// Build a TOC by planning chunks, then write it out.
pub struct TocWriter {
    chunks: Vec<PlannedChunk>,
}

impl TocWriter {
    pub fn new() -> Self {
        Self { chunks: Vec::new() }
    }

    /// Declare a chunk to be written.  Chunks are written in the order
    /// they are planned.
    ///
    /// Duplicate ids get debug-only protection here — the container
    /// writers guard at runtime before reaching this point
    /// ([`container::ContainerBuilder::write_to`] /
    /// [`container::StreamingWriter::write_chunk`]), and
    /// [`Toc::parse`] rejects duplicates at read time. Direct raw-layer
    /// producers are expected to have a fixed id set.
    pub fn plan(&mut self, id: ChunkId, size: u64) {
        debug_assert!(
            id != END_MARKER_ID,
            "chunk id must not be the reserved end-marker sentinel",
        );
        debug_assert!(
            !self.chunks.iter().any(|c| c.id == id),
            "duplicate chunk id: {:?}",
            id,
        );
        self.chunks.push(PlannedChunk { id, size });
    }

    /// The byte size the TOC will occupy on disk.
    pub fn toc_byte_size(&self) -> usize {
        Toc::byte_size(self.chunks.len())
    }

    /// Write the TOC to `out` and return a [`ChunkWriter`] for streaming
    /// chunk data.
    ///
    /// `base_offset` is the absolute file offset at which the TOC itself
    /// is being written (0 for a bare TOC file; the header size when a
    /// header precedes it). Entry offsets are absolute: the first
    /// chunk's data begins at `base_offset + toc_byte_size()`.
    pub fn write_toc<W: Write>(
        self,
        mut out: W,
        base_offset: u64,
    ) -> Result<ChunkWriter<W>, Error> {
        let toc_size = self.toc_byte_size();
        let mut current_offset = base_offset + toc_size as u64;

        // Write N chunk entries.
        for chunk in &self.chunks {
            let entry = TocEntry::new(chunk.id, current_offset);
            out.write_all(entry.as_bytes())?;
            current_offset += chunk.size;
        }

        // Write end marker.
        let end_entry = TocEntry::new(END_MARKER_ID, current_offset);
        out.write_all(end_entry.as_bytes())?;

        Ok(ChunkWriter {
            inner: out,
            remaining: self.chunks.into(),
        })
    }
}

impl Default for TocWriter {
    fn default() -> Self {
        Self::new()
    }
}

// ── ChunkWriter ─────────────────────────────────────────────────

/// Sequential chunk writer.  Write each chunk's data in planned order.
pub struct ChunkWriter<W> {
    inner: W,
    remaining: VecDeque<PlannedChunk>,
}

impl<W: Write> ChunkWriter<W> {
    /// Write the next chunk.  The `id` must match the next planned chunk,
    /// and `data.len()` must match its planned size.
    pub fn write_chunk(&mut self, id: ChunkId, data: &[u8]) -> Result<(), Error> {
        let planned = match self.remaining.pop_front() {
            Some(p) => p,
            None => {
                return Err(Error::UnplannedChunk { id });
            }
        };

        if id != planned.id {
            return Err(Error::WrongChunkId {
                expected: planned.id,
                got: id,
            });
        }

        if data.len() as u64 != planned.size {
            return Err(Error::WrongChunkSize {
                id,
                expected: planned.size,
                got: data.len() as u64,
            });
        }

        self.inner.write_all(data)?;
        Ok(())
    }

    /// Return the underlying writer without the all-chunks-written
    /// check that [`finish`](ChunkWriter::finish) performs. For callers
    /// that enforce the plan by construction (they build the plan and
    /// the payload sequence from the same list) and write payloads
    /// directly to avoid intermediate copies.
    pub fn into_inner(self) -> W {
        self.inner
    }

    /// Finish writing.  Returns an error if not all planned chunks were
    /// written.  On success, returns the underlying writer.
    pub fn finish(self) -> Result<W, Error> {
        if !self.remaining.is_empty() {
            return Err(Error::IncompleteWrite {
                remaining: self.remaining.len(),
            });
        }
        Ok(self.inner)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toc_entry_layout() {
        assert_eq!(size_of::<TocEntry>(), 12);
        assert_eq!(align_of::<TocEntry>(), 1);
    }

    #[test]
    fn toc_entry_roundtrip() {
        let entry = TocEntry::new(*b"TEST", 0x1234_5678_9ABC_DEF0);
        assert_eq!(entry.id, *b"TEST");
        assert_eq!(entry.offset(), 0x1234_5678_9ABC_DEF0);
    }

    /// Helper: write a complete file (TOC + chunks) and return the raw bytes.
    fn write_file(chunks: &[(ChunkId, &[u8])]) -> Vec<u8> {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        for &(id, data) in chunks {
            writer.plan(id, data.len() as u64);
        }
        let mut cw = writer.write_toc(&mut buf, 0).unwrap();
        for &(id, data) in chunks {
            cw.write_chunk(id, data).unwrap();
        }
        cw.finish().unwrap();
        buf
    }

    #[test]
    fn roundtrip_single_chunk() {
        let data = b"hello world";
        let file = write_file(&[(*b"PRIM", data)]);

        let toc = Toc::parse(&file, 0, 1).unwrap();
        assert_eq!(toc.num_chunks(), 1);

        let meta = toc.get(*b"PRIM").unwrap();
        assert_eq!(meta.id, *b"PRIM");
        assert_eq!(meta.size, data.len() as u64);

        let chunk_data = toc.data(&file, *b"PRIM").unwrap();
        assert_eq!(chunk_data, data);
    }

    #[test]
    fn roundtrip_multiple_chunks() {
        let chunks: &[(ChunkId, &[u8])] = &[
            (*b"META", b"metadata here"),
            (*b"PRIM", b"primary content"),
            (*b"HC\x00\x01", b"high-card chunk"),
        ];
        let file = write_file(chunks);

        let toc = Toc::parse(&file, 0, 3).unwrap();
        assert_eq!(toc.num_chunks(), 3);

        for &(id, expected_data) in chunks {
            let meta = toc.get(id).unwrap();
            assert_eq!(meta.size, expected_data.len() as u64);
            assert_eq!(toc.data(&file, id).unwrap(), expected_data);
        }
    }

    #[test]
    fn parse_at_nonzero_toc_offset() {
        // A 12-byte header precedes the TOC — the container layout.
        let mut file = b"HDRHDRHDRHDR".to_vec();
        let mut writer = TocWriter::new();
        writer.plan(*b"AAAA", 5);
        let mut cw = writer.write_toc(&mut file, 12).unwrap();
        cw.write_chunk(*b"AAAA", b"hello").unwrap();
        cw.finish().unwrap();

        let toc = Toc::parse(&file, 12, 1).unwrap();
        let meta = toc.get(*b"AAAA").unwrap();
        assert_eq!(meta.offset, 12 + Toc::byte_size(1) as u64);
        assert_eq!(toc.data(&file, *b"AAAA").unwrap(), b"hello");
    }

    #[test]
    fn iteration_order() {
        let chunks: &[(ChunkId, &[u8])] = &[
            (*b"AAAA", b"first"),
            (*b"BBBB", b"second"),
            (*b"CCCC", b"third"),
        ];
        let file = write_file(chunks);

        let toc = Toc::parse(&file, 0, 3).unwrap();
        let metas: Vec<ChunkMeta> = toc.iter().collect();

        assert_eq!(metas.len(), 3);
        assert_eq!(metas[0].id, *b"AAAA");
        assert_eq!(metas[0].size, 5);
        assert_eq!(metas[1].id, *b"BBBB");
        assert_eq!(metas[1].size, 6);
        assert_eq!(metas[2].id, *b"CCCC");
        assert_eq!(metas[2].size, 5);

        // Offsets are strictly increasing.
        assert!(metas[0].offset < metas[1].offset);
        assert!(metas[1].offset < metas[2].offset);
    }

    #[test]
    fn chunk_not_found() {
        let file = write_file(&[(*b"PRIM", b"data")]);
        let toc = Toc::parse(&file, 0, 1).unwrap();

        assert!(toc.get(*b"MISS").is_none());
        assert!(matches!(
            toc.data(&file, *b"MISS"),
            Err(Error::ChunkNotFound { .. })
        ));
    }

    #[test]
    fn toc_too_short() {
        let data = b"xxxx"; // not enough bytes for even 1 entry
        let result = Toc::parse(data, 0, 1);
        assert!(matches!(result, Err(Error::TocTooShort { .. })));
    }

    #[test]
    fn rejects_duplicate_chunk_ids() {
        // Hand-craft a TOC with the same id twice: AAAA at two entries.
        let mut file = Vec::new();
        let toc_size = Toc::byte_size(2) as u64;
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"AAAA", toc_size)));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"AAAA", toc_size + 1)));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(
            END_MARKER_ID,
            toc_size + 2,
        )));
        file.extend_from_slice(b"xy");

        assert!(matches!(
            Toc::parse(&file, 0, 2),
            Err(Error::DuplicateChunkId { id, index: 1 }) if id == *b"AAAA"
        ));
    }

    #[test]
    fn duplicate_report_is_deterministic_with_multiple_duplicated_ids() {
        // Two distinct ids each duplicated, with the lexicographically
        // smaller id appearing later in TOC order: BBBB@0, BBBB@1,
        // AAAA@2, AAAA@3. The validator reports the later occurrence of
        // the sorted-first duplicated id (AAAA, index 3) — pinned here
        // so the (id, index) tuple sort stays deterministic.
        let mut file = Vec::new();
        let toc_size = Toc::byte_size(4) as u64;
        for (k, id) in [*b"BBBB", *b"BBBB", *b"AAAA", *b"AAAA"]
            .into_iter()
            .enumerate()
        {
            file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(id, toc_size + k as u64)));
        }
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(
            END_MARKER_ID,
            toc_size + 4,
        )));
        file.extend_from_slice(b"wxyz");

        assert!(matches!(
            Toc::parse(&file, 0, 4),
            Err(Error::DuplicateChunkId { id, index: 3 }) if id == *b"AAAA"
        ));
    }

    #[test]
    fn data_with_short_file_slice_errors_instead_of_panicking() {
        let file = write_file(&[(*b"PRIM", b"data")]);
        let toc = Toc::parse(&file, 0, 1).unwrap();
        // A slice shorter than the one validated at parse time is a
        // caller bug — surfaced as an error, never an OOB panic.
        assert!(matches!(
            toc.data(&file[..file.len() - 1], *b"PRIM"),
            Err(Error::ChunkOutOfBounds { id, .. }) if id == *b"PRIM"
        ));
        // The full slice still works.
        assert_eq!(toc.data(&file, *b"PRIM").unwrap(), b"data");
    }

    #[test]
    fn rejects_reserved_zero_id_among_chunk_entries() {
        // A chunk entry carrying the end-marker id would be
        // unaddressable; gix's decoder rejected mid-TOC sentinels and
        // so do we.
        let mut file = Vec::new();
        let toc_size = Toc::byte_size(2) as u64;
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"AAAA", toc_size)));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(
            END_MARKER_ID,
            toc_size + 1,
        )));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(
            END_MARKER_ID,
            toc_size + 2,
        )));
        file.extend_from_slice(b"xy");

        assert!(matches!(
            Toc::parse(&file, 0, 2),
            Err(Error::ReservedChunkId { index: 1 })
        ));
    }

    #[test]
    fn rejects_nonzero_end_marker_id() {
        let mut file = Vec::new();
        let toc_size = Toc::byte_size(1) as u64;
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"AAAA", toc_size)));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"OOPS", toc_size + 2)));
        file.extend_from_slice(b"xy");

        assert!(matches!(
            Toc::parse(&file, 0, 1),
            Err(Error::BadEndMarker { id }) if id == *b"OOPS"
        ));
    }

    #[test]
    fn rejects_chunk_overlapping_toc() {
        // First chunk offset points inside the TOC itself.
        let mut file = Vec::new();
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(*b"AAAA", 4)));
        file.extend_from_slice(IntoBytes::as_bytes(&TocEntry::new(END_MARKER_ID, 25)));
        file.push(0);

        assert!(matches!(
            Toc::parse(&file, 0, 1),
            Err(Error::ChunkOverlapsToc { .. })
        ));
    }

    #[test]
    fn wrong_chunk_id_on_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"PRIM", 4);
        let mut cw = writer.write_toc(&mut buf, 0).unwrap();

        let result = cw.write_chunk(*b"WRNG", b"data");
        assert!(matches!(result, Err(Error::WrongChunkId { .. })));
    }

    #[test]
    fn wrong_chunk_size_on_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"PRIM", 4);
        let mut cw = writer.write_toc(&mut buf, 0).unwrap();

        let result = cw.write_chunk(*b"PRIM", b"too long data");
        assert!(matches!(result, Err(Error::WrongChunkSize { .. })));
    }

    #[test]
    fn incomplete_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"AAAA", 1);
        writer.plan(*b"BBBB", 1);
        let mut cw = writer.write_toc(&mut buf, 0).unwrap();

        cw.write_chunk(*b"AAAA", b"x").unwrap();
        // Forget to write BBBB.
        let result = cw.finish();
        assert!(matches!(
            result,
            Err(Error::IncompleteWrite { remaining: 1 })
        ));
    }

    #[test]
    fn zero_copy() {
        let file = write_file(&[(*b"PRIM", b"data")]);
        let toc = Toc::parse(&file, 0, 1).unwrap();

        // The TocEntry slice should point into the original file buffer.
        let entries_ptr = toc.entries.as_ptr() as usize;
        let file_start = file.as_ptr() as usize;
        let file_end = file_start + file.len();

        assert!(entries_ptr >= file_start);
        assert!(entries_ptr < file_end);
    }

    #[test]
    fn empty_file_no_chunks() {
        let file = write_file(&[]);
        let toc = Toc::parse(&file, 0, 0).unwrap();
        assert_eq!(toc.num_chunks(), 0);
        assert_eq!(toc.iter().count(), 0);
        assert!(toc.get(*b"PRIM").is_none());
    }

    #[test]
    fn toc_byte_size() {
        assert_eq!(Toc::byte_size(0), 12); // just end marker
        assert_eq!(Toc::byte_size(1), 24); // 1 entry + end marker
        assert_eq!(Toc::byte_size(3), 48); // 3 entries + end marker
    }

    #[test]
    fn toc_writer_byte_size() {
        let mut w = TocWriter::new();
        assert_eq!(w.toc_byte_size(), 12);
        w.plan(*b"AAAA", 100);
        assert_eq!(w.toc_byte_size(), 24);
        w.plan(*b"BBBB", 200);
        assert_eq!(w.toc_byte_size(), 36);
    }
}
