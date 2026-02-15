//! Zero-copy chunk file TOC for journal file formats.
//!
//! Provides a table of contents (TOC) that maps 4-byte chunk IDs to byte
//! ranges within a file.  Designed for memory-mapped files: the TOC is parsed
//! zero-copy via [`zerocopy`], and [`ChunkMeta`] exposes raw offsets so callers
//! can issue `madvise` hints per chunk.
//!
//! # On-disk TOC layout
//!
//! For N chunks the TOC occupies `(N + 1) × 12` bytes:
//!
//! ```text
//! entry[0]:  id(4) + offset_le(8)    chunk 0 starts at offset
//! entry[1]:  id(4) + offset_le(8)    chunk 1 starts at offset
//! ...
//! entry[N]:  __(4) + offset_le(8)    end marker (id unused, offset = end of last chunk)
//! ```
//!
//! All offsets are absolute from file start, little-endian.

use std::collections::VecDeque;
use std::io::Write;

use zerocopy::{FromBytes, Immutable, IntoBytes, KnownLayout, Ref};

// ── Types ────────────────────────────────────────────────────────

/// A 4-byte chunk identifier (e.g. `*b"PRIM"`).
pub type ChunkId = [u8; 4];

/// A single on-disk TOC entry.
///
/// Exactly 12 bytes with alignment 1.  The offset is stored as `[u8; 8]`
/// rather than `u64` so `#[repr(C)]` introduces no padding.
#[derive(FromBytes, IntoBytes, Immutable, KnownLayout, Copy, Clone, Debug)]
#[repr(C)]
pub struct TocEntry {
    /// Chunk identifier.  For the end-marker entry this field is unused.
    pub id: ChunkId,
    /// Absolute byte offset from file start, little-endian.
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
    /// Absolute byte offset of the chunk's first byte.
    pub offset: u64,
    /// Byte length of the chunk data.
    pub size: u64,
}

// ── Error ────────────────────────────────────────────────────────

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("TOC needs {expected} bytes, got {actual}")]
    TocTooShort { expected: usize, actual: usize },

    #[error("chunk {id:?} not found")]
    ChunkNotFound { id: ChunkId },

    #[error("offset {offset} at entry {index} exceeds data length {data_len}")]
    OffsetOutOfBounds {
        index: usize,
        offset: u64,
        data_len: usize,
    },

    #[error("non-monotonic offset at entry {index}: {prev} >= {curr}")]
    NonMonotonicOffset { index: usize, prev: u64, curr: u64 },

    #[error("expected chunk {expected:?}, got {got:?}")]
    WrongChunkId { expected: ChunkId, got: ChunkId },

    #[error("chunk {id:?}: expected {expected} bytes, got {got}")]
    WrongChunkSize {
        id: ChunkId,
        expected: u64,
        got: u64,
    },

    #[error("{remaining} planned chunks were not written")]
    IncompleteWrite { remaining: usize },

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

impl<'a> Toc<'a> {
    /// The byte size of a TOC with `num_chunks` chunks.
    pub const fn byte_size(num_chunks: usize) -> usize {
        (num_chunks + 1) * size_of::<TocEntry>()
    }

    /// Parse a TOC from the start of `data`, expecting `num_chunks` chunks.
    ///
    /// This is zero-copy: the returned [`Toc`] borrows directly from `data`.
    pub fn from_bytes(data: &'a [u8], num_chunks: u32) -> Result<Self, Error> {
        let n = num_chunks as usize;
        let toc_size = Self::byte_size(n);

        if data.len() < toc_size {
            return Err(Error::TocTooShort {
                expected: toc_size,
                actual: data.len(),
            });
        }

        let toc_bytes = &data[..toc_size];
        let entries: Ref<&[u8], [TocEntry]> =
            Ref::from_bytes(toc_bytes).map_err(|_| Error::TocTooShort {
                expected: toc_size,
                actual: toc_bytes.len(),
            })?;

        // Validate: monotonically increasing offsets, within bounds.
        let data_len = data.len();
        let num_entries = entries.len(); // N+1
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
    /// The `file` slice is typically the full memory-mapped file.  It need
    /// not be the same slice that was passed to [`from_bytes`](Toc::from_bytes).
    pub fn data<'f>(&self, file: &'f [u8], id: ChunkId) -> Result<&'f [u8], Error> {
        let meta = self.get(id).ok_or(Error::ChunkNotFound { id })?;
        let start = meta.offset as usize;
        let end = start + meta.size as usize;
        Ok(&file[start..end])
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
    pub fn plan(&mut self, id: ChunkId, size: u64) {
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
    /// chunk data.  The first chunk's data begins immediately after the TOC.
    pub fn write_toc<W: Write>(self, mut out: W) -> Result<ChunkWriter<W>, Error> {
        let toc_size = self.toc_byte_size();
        let mut current_offset = toc_size as u64;

        // Write N chunk entries.
        for chunk in &self.chunks {
            let entry = TocEntry::new(chunk.id, current_offset);
            out.write_all(IntoBytes::as_bytes(&entry))?;
            current_offset += chunk.size;
        }

        // Write end marker.
        let end_entry = TocEntry::new([0; 4], current_offset);
        out.write_all(IntoBytes::as_bytes(&end_entry))?;

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
                return Err(Error::ChunkNotFound { id });
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
        let mut cw = writer.write_toc(&mut buf).unwrap();
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

        let toc = Toc::from_bytes(&file, 1).unwrap();
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

        let toc = Toc::from_bytes(&file, 3).unwrap();
        assert_eq!(toc.num_chunks(), 3);

        for &(id, expected_data) in chunks {
            let meta = toc.get(id).unwrap();
            assert_eq!(meta.size, expected_data.len() as u64);
            assert_eq!(toc.data(&file, id).unwrap(), expected_data);
        }
    }

    #[test]
    fn iteration_order() {
        let chunks: &[(ChunkId, &[u8])] = &[
            (*b"AAAA", b"first"),
            (*b"BBBB", b"second"),
            (*b"CCCC", b"third"),
        ];
        let file = write_file(chunks);

        let toc = Toc::from_bytes(&file, 3).unwrap();
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
        let toc = Toc::from_bytes(&file, 1).unwrap();

        assert!(toc.get(*b"MISS").is_none());
        assert!(matches!(
            toc.data(&file, *b"MISS"),
            Err(Error::ChunkNotFound { .. })
        ));
    }

    #[test]
    fn toc_too_short() {
        let data = b"xxxx"; // not enough bytes for even 1 entry
        let result = Toc::from_bytes(data, 1);
        assert!(matches!(result, Err(Error::TocTooShort { .. })));
    }

    #[test]
    fn wrong_chunk_id_on_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"PRIM", 4);
        let mut cw = writer.write_toc(&mut buf).unwrap();

        let result = cw.write_chunk(*b"WRNG", b"data");
        assert!(matches!(result, Err(Error::WrongChunkId { .. })));
    }

    #[test]
    fn wrong_chunk_size_on_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"PRIM", 4);
        let mut cw = writer.write_toc(&mut buf).unwrap();

        let result = cw.write_chunk(*b"PRIM", b"too long data");
        assert!(matches!(result, Err(Error::WrongChunkSize { .. })));
    }

    #[test]
    fn incomplete_write() {
        let mut buf = Vec::new();
        let mut writer = TocWriter::new();
        writer.plan(*b"AAAA", 1);
        writer.plan(*b"BBBB", 1);
        let mut cw = writer.write_toc(&mut buf).unwrap();

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
        let toc = Toc::from_bytes(&file, 1).unwrap();

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
        let toc = Toc::from_bytes(&file, 0).unwrap();
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
