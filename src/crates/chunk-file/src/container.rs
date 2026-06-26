//! Integrity container: magic + version + TOC + per-chunk crc32.
//!
//! A self-describing framing for durable chunk-based artifacts —
//! files that outlive the process that wrote them (and possibly the
//! machine, once shipped to long-lived remote storage), where silent
//! corruption must surface as an error rather than wrong data. Each
//! file carries magic + version (self-describing) and a crc32 per
//! chunk (integrity-checked). Consumers supply their own magic and
//! format version and own the meaning of their chunk payloads.
//!
//! On-disk layout:
//!
//! ```text
//! [ magic: 4 bytes (consumer-supplied, e.g. "SFST" / "NCAT") ]
//! [ version: u32 LE (consumer-supplied format version)       ]
//! [ num_chunks: u32 LE                                       ]
//! [ TOC (see crate docs) at HEADER_SIZE                      ]
//! [ chunk payloads, each: <payload bytes> <crc32 u32 LE>     ]
//! ```
//!
//! The crc32 ([`crc32fast`]) covers the stored payload bytes only —
//! for compressed payloads that is the compressed form, which is where
//! at-rest / in-transit corruption happens; if the stored bytes verify,
//! decompression is deterministic. The TOC records each chunk's length
//! as `payload_len + 4` (CRC included in the span). TOC corruption is
//! caught at parse time by the TOC validators, and a corrupt offset
//! that still parses resolves the wrong span, whose CRC then fails.
//!
//! Two writers produce identical bytes:
//!
//! - [`ContainerBuilder`] buffers borrowed payloads and writes
//!   header → TOC → payloads in one pass. Right for small files whose
//!   chunks already exist in memory (catalogs).
//! - [`StreamingWriter`] reserves the header + TOC region, streams
//!   each chunk (and its crc32) to a `Write + Seek` sink as it is
//!   produced — so the producer can drop each payload immediately —
//!   and patches the TOC on [`finish`](StreamingWriter::finish). Right
//!   for large files built chunk-by-chunk (SFST indexes): peak memory
//!   is one chunk, not the whole file.

use std::collections::HashSet;
use std::io::{Seek, SeekFrom, Write};

use crate::{ChunkId, END_MARKER_ID, Toc, TocEntry, TocWriter};
use zerocopy::IntoBytes;

/// Fixed header size: magic(4) + version(4) + num_chunks(4).
pub const HEADER_SIZE: usize = 12;

/// Per-chunk crc32 trailer size.
const CRC_LEN: usize = 4;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The byte slice is shorter than the fixed header. First value is
    /// the actual length, second the required minimum.
    #[error("file too short ({0} bytes, need at least {1})")]
    TooShort(usize, usize),

    /// The first 4 bytes don't match the consumer's expected magic.
    #[error("invalid magic")]
    BadMagic,

    /// The header `version` field doesn't match the consumer's expected
    /// format version.
    #[error("unsupported version: {0}")]
    UnsupportedVersion(u32),

    /// The TOC failed to parse or validate — carries the raw layer's
    /// structured error so callers can match the specific failure
    /// (duplicate id, non-monotonic offset, out-of-bounds, ...).
    #[error("TOC error: {0}")]
    Toc(#[from] crate::Error),

    /// The container framing is malformed beyond the TOC itself —
    /// a chunkless header, an implausible chunk count, or a chunk span
    /// too short for its crc32 trailer.
    #[error("malformed container: {0}")]
    Malformed(String),

    /// A writer was driven outside its contract — wrong chunk count,
    /// duplicate or reserved id. A producer bug, never file corruption.
    #[error("writer misuse: {0}")]
    Misuse(String),

    /// No chunk with the requested id exists in the TOC.
    #[error("chunk '{}' not found", id_str(*id))]
    ChunkNotFound { id: ChunkId },

    /// A chunk's stored crc32 trailer doesn't match the crc32 computed
    /// over its payload bytes — the chunk is corrupt.
    #[error(
        "chunk '{}' CRC mismatch: stored {expected:#010x}, computed {actual:#010x}",
        id_str(*id)
    )]
    CrcMismatch {
        id: ChunkId,
        expected: u32,
        actual: u32,
    },

    /// Underlying I/O failed (streaming writer).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),
}

fn id_str(id: ChunkId) -> String {
    id.escape_ascii().to_string()
}

fn encode_header(magic: [u8; 4], version: u32, num_chunks: u32) -> [u8; HEADER_SIZE] {
    let mut header = [0u8; HEADER_SIZE];
    header[0..4].copy_from_slice(&magic);
    header[4..8].copy_from_slice(&version.to_le_bytes());
    header[8..12].copy_from_slice(&num_chunks.to_le_bytes());
    header
}

// ── Reader ──────────────────────────────────────────────────────

/// Zero-copy view over a container file (typically an mmap).
///
/// [`open`](Container::open) parses only the header and TOC — the TOC
/// is borrowed in place, not materialized; payloads are resolved — and
/// CRC-verified — lazily, per chunk, so an mmap'd file is only paged in
/// where it is actually read.
pub struct Container<'a> {
    data: &'a [u8],
    toc: Toc<'a>,
}

impl<'a> Container<'a> {
    /// Open a container, validating length, magic, version and the TOC.
    ///
    /// `data` is the container's own byte range — for a container
    /// embedded at an offset inside a larger file, pass the sub-slice
    /// starting there (the coordinate space [`StreamingWriter`]
    /// established at write time).
    pub fn open(data: &'a [u8], magic: &[u8; 4], version: u32) -> Result<Self, Error> {
        if data.len() < HEADER_SIZE {
            return Err(Error::TooShort(data.len(), HEADER_SIZE));
        }
        if &data[0..4] != magic {
            return Err(Error::BadMagic);
        }
        let file_version = u32::from_le_bytes(data[4..8].try_into().unwrap());
        if file_version != version {
            return Err(Error::UnsupportedVersion(file_version));
        }

        let num_chunks = u32::from_le_bytes(data[8..12].try_into().unwrap());
        // Both writers refuse to produce a chunkless container, so a
        // zero here is corruption (or a foreign file), not a valid
        // degenerate case.
        if num_chunks == 0 {
            return Err(Error::Malformed(
                "container must have at least one chunk".to_string(),
            ));
        }
        // Defense in depth: a corrupted header claiming u32::MAX chunks
        // would otherwise demand a ≈48 GiB TOC. Each TOC entry is 12
        // bytes, so the on-disk body bounds the legal value.
        let max_chunks = data.len().saturating_sub(HEADER_SIZE) / size_of::<TocEntry>();
        if num_chunks as usize > max_chunks {
            return Err(Error::Malformed(format!(
                "num_chunks ({num_chunks}) exceeds plausible maximum ({max_chunks})"
            )));
        }
        let toc = Toc::parse(data, HEADER_SIZE, num_chunks)?;

        Ok(Self { data, toc })
    }

    /// Resolve a chunk's payload bytes, verifying its crc32 trailer.
    /// This is the single verified chokepoint — every consumer read
    /// goes through here, so corruption can't reach a deserializer.
    pub fn chunk(&self, id: ChunkId) -> Result<&'a [u8], Error> {
        let meta = self.toc.get(id).ok_or(Error::ChunkNotFound { id })?;
        // `Toc::parse` validated bounds and monotonicity, so the span is
        // in-bounds; the checked slicing keeps this panic-free
        // regardless.
        let end = meta.offset + meta.size; // u64: cannot wrap for parsed offsets
        let span = usize::try_from(meta.offset)
            .ok()
            .zip(usize::try_from(end).ok())
            .and_then(|(start, end)| self.data.get(start..end))
            .ok_or_else(|| Error::Malformed("chunk span out of bounds".to_string()))?;
        if span.len() < CRC_LEN {
            return Err(Error::Malformed(format!(
                "chunk '{}' shorter than its CRC trailer",
                id_str(id)
            )));
        }
        let (payload, crc_bytes) = span.split_at(span.len() - CRC_LEN);
        let expected = u32::from_le_bytes(crc_bytes.try_into().unwrap());
        let actual = crc32fast::hash(payload);
        if actual != expected {
            return Err(Error::CrcMismatch {
                id,
                expected,
                actual,
            });
        }
        Ok(payload)
    }

    /// Whether a chunk with `id` exists in the TOC (no CRC check).
    pub fn has_chunk(&self, id: ChunkId) -> bool {
        self.toc.get(id).is_some()
    }

    /// A chunk's on-disk placement ([`crate::ChunkMeta`]: container-relative
    /// `offset` plus `size`, the size **including** the 4-byte crc32
    /// trailer). Resolved from the TOC alone: no payload byte is read
    /// or verified, so this is the right primitive for layout decisions
    /// (page-cache advice, readahead hints) that must not fault payload
    /// pages in.
    pub fn chunk_meta(&self, id: ChunkId) -> Option<crate::ChunkMeta> {
        self.toc.get(id)
    }
}

impl std::fmt::Debug for Container<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Container")
            .field("len", &self.data.len())
            .field("toc", &self.toc)
            .finish()
    }
}

// ── Buffer-all writer ───────────────────────────────────────────

/// Assembles a container file from pre-built chunk payloads.
///
/// Chunks are written in the exact order they were added — producers
/// are free to group hot chunks into a prefix; the builder never
/// reorders. Each payload gains a trailing crc32 on write. For files
/// built chunk-by-chunk, prefer [`StreamingWriter`], which doesn't
/// require holding every payload at once.
///
/// ```
/// use chunk_file::container::{Container, ContainerBuilder};
///
/// let mut builder = ContainerBuilder::new(*b"DEMO", 1);
/// builder.add_chunk(*b"HEAD", b"hot prefix");
/// builder.add_chunk(*b"BODY", b"payload bytes");
/// let mut file = Vec::new();
/// builder.write_to(&mut file)?;
///
/// let container = Container::open(&file, b"DEMO", 1)?;
/// assert_eq!(container.chunk(*b"BODY")?, b"payload bytes");
/// # Ok::<(), Box<dyn std::error::Error>>(())
/// ```
pub struct ContainerBuilder<'a> {
    magic: [u8; 4],
    version: u32,
    chunks: Vec<(ChunkId, &'a [u8])>,
}

impl<'a> ContainerBuilder<'a> {
    /// Start a container with the consumer's magic and format version.
    pub fn new(magic: [u8; 4], version: u32) -> Self {
        Self {
            magic,
            version,
            chunks: Vec::new(),
        }
    }

    /// Append a chunk. Duplicate ids are a producer bug, rejected by
    /// [`write_to`](ContainerBuilder::write_to) before any byte is
    /// written.
    pub fn add_chunk(&mut self, id: ChunkId, payload: &'a [u8]) {
        self.chunks.push((id, payload));
    }

    /// Number of chunks added so far.
    pub fn num_chunks(&self) -> usize {
        self.chunks.len()
    }

    /// Serialize the container (header + TOC + payloads with crc32
    /// trailers) to `w`. At least one chunk must have been added — an
    /// empty chunk file has no reason to exist.
    pub fn write_to<W: Write>(&self, w: &mut W) -> std::io::Result<()> {
        if self.chunks.is_empty() {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "container must have at least one chunk",
            ));
        }
        // Reject duplicate or reserved ids before any byte is written —
        // a release build must not produce a file the reader will
        // reject.
        let mut seen = HashSet::with_capacity(self.chunks.len());
        for (id, _) in &self.chunks {
            if *id == END_MARKER_ID {
                return Err(std::io::Error::new(
                    std::io::ErrorKind::InvalidInput,
                    "chunk id must not be the reserved end-marker sentinel",
                ));
            }
            if !seen.insert(*id) {
                return Err(std::io::Error::new(
                    std::io::ErrorKind::InvalidInput,
                    format!("duplicate chunk id '{}'", id_str(*id)),
                ));
            }
        }

        let num_chunks = u32::try_from(self.chunks.len()).map_err(|_| {
            std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "chunk count exceeds u32::MAX",
            )
        })?;
        w.write_all(&encode_header(self.magic, self.version, num_chunks))?;

        let mut toc = TocWriter::new();
        for (id, payload) in &self.chunks {
            toc.plan(*id, (payload.len() + CRC_LEN) as u64);
        }
        let chunk_writer = toc
            .write_toc(&mut *w, HEADER_SIZE as u64)
            .map_err(io_from_raw)?;
        // Skip the raw ChunkWriter's per-chunk id/size validation: the
        // plan above was built from the same `self.chunks` this loop
        // walks, so the invariants hold by construction, and writing
        // payload + trailer directly avoids copying each payload into
        // an intermediate buffer (the streaming writer's pattern; the
        // byte-equality test pins that both writers agree).
        let w = chunk_writer.into_inner();
        for (_, payload) in &self.chunks {
            w.write_all(payload)?;
            w.write_all(&crc32fast::hash(payload).to_le_bytes())?;
        }
        w.flush()?;
        Ok(())
    }
}

/// Map a raw-layer error into `io::Error` for the builder's `io::Result`
/// signature (write-side raw errors are producer bugs or I/O).
fn io_from_raw(e: crate::Error) -> std::io::Error {
    match e {
        crate::Error::Io(io) => io,
        other => std::io::Error::new(std::io::ErrorKind::InvalidInput, other.to_string()),
    }
}

// ── Streaming writer ────────────────────────────────────────────

/// Streams a container to a `Write + Seek` sink one chunk at a time.
///
/// The chunk **count** must be known up front (the TOC region is
/// reserved before the first payload); ids and sizes are recorded as
/// chunks stream. The producer packs a chunk, hands it to
/// [`write_chunk`](StreamingWriter::write_chunk), and drops it — peak
/// memory is one chunk, not the whole file. [`finish`](StreamingWriter::finish)
/// seeks back, writes the real TOC, and returns the sink positioned at
/// end of file.
///
/// ```
/// use std::io::Cursor;
/// use chunk_file::container::{Container, StreamingWriter};
///
/// let mut writer = StreamingWriter::new(Cursor::new(Vec::new()), *b"DEMO", 1, 2)?;
/// writer.write_chunk(*b"HEAD", b"hot prefix")?; // pack -> write -> drop, one at a time
/// writer.write_chunk(*b"BODY", b"payload bytes")?;
/// let file = writer.finish()?.into_inner();
///
/// let container = Container::open(&file, b"DEMO", 1)?;
/// assert_eq!(container.chunk(*b"HEAD")?, b"hot prefix");
/// # Ok::<(), Box<dyn std::error::Error>>(())
/// ```
pub struct StreamingWriter<W> {
    out: W,
    /// Sink position at construction — the container's first byte. All
    /// patch seeks are relative to it, so a container can be written at
    /// any offset within a larger file; TOC offsets stay
    /// container-relative either way.
    base: u64,
    num_chunks: u32,
    /// (id, on-disk span = payload + crc) per chunk written so far —
    /// the TOC patch in [`finish`](StreamingWriter::finish) replays it
    /// in write order.
    written: Vec<(ChunkId, u64)>,
    /// Ids written so far, for O(1) duplicate rejection.
    seen: HashSet<ChunkId>,
}

impl<W: Write + Seek> StreamingWriter<W> {
    /// Write the header and reserve the TOC region for `num_chunks`
    /// chunks. The container starts wherever the sink is currently
    /// positioned — the position is captured and the TOC patch seeks
    /// relative to it, so a container can be written at a non-zero
    /// offset within a larger file (the reader is then handed the
    /// sub-slice starting there); TOC offsets are container-relative
    /// either way.
    pub fn new(mut out: W, magic: [u8; 4], version: u32, num_chunks: u32) -> Result<Self, Error> {
        if num_chunks == 0 {
            return Err(Error::Misuse(
                "container must have at least one chunk".to_string(),
            ));
        }
        let base = out.stream_position()?;
        out.write_all(&encode_header(magic, version, num_chunks))?;
        // Reserve the TOC region; patched in finish(). The transient
        // zero buffer is 12 × (num_chunks + 1) bytes — noise next to
        // the chunk payloads.
        let toc_size = Toc::byte_size(num_chunks as usize);
        out.write_all(&vec![0u8; toc_size])?;
        Ok(Self {
            out,
            base,
            num_chunks,
            written: Vec::with_capacity(num_chunks as usize),
            seen: HashSet::with_capacity(num_chunks as usize),
        })
    }

    /// Stream one chunk: payload bytes followed by their crc32 trailer.
    /// Chunks land in call order; `id` must be unique.
    pub fn write_chunk(&mut self, id: ChunkId, payload: &[u8]) -> Result<(), Error> {
        if id == END_MARKER_ID {
            return Err(Error::Misuse(
                "chunk id must not be the reserved end-marker sentinel".to_string(),
            ));
        }
        if self.written.len() as u32 == self.num_chunks {
            return Err(Error::Misuse(format!(
                "chunk '{}' exceeds the planned count ({})",
                id_str(id),
                self.num_chunks
            )));
        }
        if !self.seen.insert(id) {
            return Err(Error::Misuse(format!(
                "duplicate chunk id '{}'",
                id_str(id)
            )));
        }
        self.out.write_all(payload)?;
        self.out
            .write_all(&crc32fast::hash(payload).to_le_bytes())?;
        self.written.push((id, (payload.len() + CRC_LEN) as u64));
        Ok(())
    }

    /// Patch the reserved TOC region with the real entries and return
    /// the sink, positioned at end of file. Errors if fewer chunks were
    /// written than planned.
    pub fn finish(mut self) -> Result<W, Error> {
        if self.written.len() as u32 != self.num_chunks {
            return Err(Error::Misuse(format!(
                "{} planned chunks were not written",
                self.num_chunks as usize - self.written.len()
            )));
        }
        let toc_size = Toc::byte_size(self.num_chunks as usize);
        self.out
            .seek(SeekFrom::Start(self.base + HEADER_SIZE as u64))?;
        let mut offset = (HEADER_SIZE + toc_size) as u64;
        for (id, span) in &self.written {
            self.out
                .write_all(IntoBytes::as_bytes(&TocEntry::new(*id, offset)))?;
            offset += span;
        }
        self.out
            .write_all(IntoBytes::as_bytes(&TocEntry::new(END_MARKER_ID, offset)))?;
        self.out.seek(SeekFrom::End(0))?;
        self.out.flush()?;
        Ok(self.out)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    const MAGIC: [u8; 4] = *b"TEST";
    const VERSION: u32 = 7;

    fn build(chunks: &[(ChunkId, &[u8])]) -> Vec<u8> {
        let mut b = ContainerBuilder::new(MAGIC, VERSION);
        for (id, payload) in chunks {
            b.add_chunk(*id, payload);
        }
        let mut out = Vec::new();
        b.write_to(&mut out).unwrap();
        out
    }

    fn stream(chunks: &[(ChunkId, &[u8])]) -> Vec<u8> {
        let mut w =
            StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, chunks.len() as u32)
                .unwrap();
        for (id, payload) in chunks {
            w.write_chunk(*id, payload).unwrap();
        }
        w.finish().unwrap().into_inner()
    }

    const SAMPLE: &[(ChunkId, &[u8])] = &[
        (*b"AAAA", b"alpha payload"),
        (*b"BBBB", b""),
        (*b"CCCC", b"gamma"),
    ];

    #[test]
    fn roundtrip_multiple_chunks_preserves_order_and_payloads() {
        let bytes = build(SAMPLE);
        let c = Container::open(&bytes, &MAGIC, VERSION).unwrap();
        assert_eq!(c.chunk(*b"AAAA").unwrap(), b"alpha payload");
        assert_eq!(c.chunk(*b"BBBB").unwrap(), b"");
        assert_eq!(c.chunk(*b"CCCC").unwrap(), b"gamma");
        assert!(c.has_chunk(*b"AAAA"));
        assert!(!c.has_chunk(*b"ZZZZ"));

        // Physical order matches add order: AAAA's payload sits before
        // CCCC's in the file.
        let a_off = bytes
            .windows(b"alpha payload".len())
            .position(|w| w == b"alpha payload")
            .unwrap();
        let c_off = bytes.windows(5).position(|w| w == b"gamma").unwrap();
        assert!(a_off < c_off);
    }

    #[test]
    fn streaming_writer_produces_identical_bytes_to_builder() {
        assert_eq!(stream(SAMPLE), build(SAMPLE));
    }

    #[test]
    fn streaming_writer_enforces_count_and_unique_ids() {
        // Fewer chunks than planned.
        let mut w = StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, 2).unwrap();
        w.write_chunk(*b"AAAA", b"x").unwrap();
        assert!(matches!(w.finish(), Err(Error::Misuse(msg)) if msg.contains("not written")));

        // More chunks than planned.
        let mut w = StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, 1).unwrap();
        w.write_chunk(*b"AAAA", b"x").unwrap();
        assert!(matches!(
            w.write_chunk(*b"BBBB", b"y"),
            Err(Error::Misuse(msg)) if msg.contains("planned count")
        ));

        // Duplicate id.
        let mut w = StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, 2).unwrap();
        w.write_chunk(*b"AAAA", b"x").unwrap();
        assert!(matches!(
            w.write_chunk(*b"AAAA", b"y"),
            Err(Error::Misuse(msg)) if msg.contains("duplicate")
        ));

        // The reserved end-marker id is refused.
        let mut w = StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, 1).unwrap();
        assert!(matches!(
            w.write_chunk(crate::END_MARKER_ID, b"x"),
            Err(Error::Misuse(msg)) if msg.contains("reserved")
        ));

        // Zero chunks is rejected up front.
        assert!(matches!(
            StreamingWriter::new(Cursor::new(Vec::new()), MAGIC, VERSION, 0),
            Err(Error::Misuse(_))
        ));
    }

    #[test]
    fn streaming_writer_at_nonzero_base_appends_a_valid_container() {
        // A container embedded past a prefix: the TOC patch must land
        // relative to the captured base, and the produced bytes must
        // match a standalone build exactly.
        let prefix = b"sixteen bytes!!!";
        let mut cursor = Cursor::new(prefix.to_vec());
        cursor.seek(SeekFrom::End(0)).unwrap();
        let mut w = StreamingWriter::new(cursor, MAGIC, VERSION, SAMPLE.len() as u32).unwrap();
        for (id, payload) in SAMPLE {
            w.write_chunk(*id, payload).unwrap();
        }
        let bytes = w.finish().unwrap().into_inner();

        assert_eq!(&bytes[..prefix.len()], prefix, "prefix untouched");
        assert_eq!(&bytes[prefix.len()..], build(SAMPLE), "container identical");
        let c = Container::open(&bytes[prefix.len()..], &MAGIC, VERSION).unwrap();
        assert_eq!(c.chunk(*b"AAAA").unwrap(), b"alpha payload");
    }

    #[test]
    fn builder_rejects_reserved_end_marker_id() {
        let mut b = ContainerBuilder::new(MAGIC, VERSION);
        b.add_chunk(crate::END_MARKER_ID, b"x");
        let mut out = Vec::new();
        let err = b.write_to(&mut out).unwrap_err();
        assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
        assert!(out.is_empty());
    }

    #[test]
    fn builder_rejects_duplicate_ids_at_write() {
        // Must be a runtime error in every build profile — a release
        // producer must not write a file the reader will reject.
        let mut b = ContainerBuilder::new(MAGIC, VERSION);
        b.add_chunk(*b"AAAA", b"x");
        b.add_chunk(*b"AAAA", b"y");
        let mut out = Vec::new();
        let err = b.write_to(&mut out).unwrap_err();
        assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
        assert!(out.is_empty(), "no bytes written before the rejection");
    }

    #[test]
    fn open_rejects_zero_chunk_header() {
        // Hand-craft a header claiming zero chunks (no writer produces
        // one): magic + version + num_chunks=0 + a sentinel-only TOC.
        let mut bytes = Vec::new();
        bytes.extend_from_slice(&MAGIC);
        bytes.extend_from_slice(&VERSION.to_le_bytes());
        bytes.extend_from_slice(&0u32.to_le_bytes());
        bytes.extend_from_slice(&[0u8; 12]); // end marker only
        match Container::open(&bytes, &MAGIC, VERSION).err() {
            Some(Error::Malformed(msg)) => assert!(msg.contains("at least one chunk"), "{msg}"),
            other => panic!("expected Toc, got {other:?}"),
        }
    }

    #[test]
    fn flipping_any_toc_byte_never_serves_wrong_payloads() {
        // TOC corruption isn't directly checksummed; it is caught by the
        // parse validators or, for flips that still parse, by the
        // per-chunk CRC over the shifted span. Property: no single-byte
        // TOC flip may silently change what a chunk read returns.
        let clean = build(SAMPLE);
        let toc_end = HEADER_SIZE + Toc::byte_size(SAMPLE.len());
        for i in HEADER_SIZE..toc_end {
            for bit in (0..8).map(|b| 1u8 << b) {
                let mut corrupt = clean.clone();
                corrupt[i] ^= bit;
                let Ok(c) = Container::open(&corrupt, &MAGIC, VERSION) else {
                    continue; // rejected at parse — fine
                };
                for (id, payload) in SAMPLE {
                    // An Err is the corruption being caught — fine.
                    if let Ok(read) = c.chunk(*id) {
                        assert_eq!(
                            read, *payload,
                            "flip at byte {i} bit {bit:#04x} silently changed chunk payload"
                        );
                    }
                }
            }
        }
    }

    #[test]
    fn chunk_meta_is_toc_only_and_includes_the_crc_trailer() {
        let bytes = build(SAMPLE);
        let c = Container::open(&bytes, &MAGIC, VERSION).unwrap();

        // Spans tile the body exactly: first chunk starts right after
        // the TOC, each span is payload + 4-byte trailer, the last ends
        // at EOF.
        let mut expected_offset = (HEADER_SIZE + Toc::byte_size(SAMPLE.len())) as u64;
        for (id, payload) in SAMPLE {
            let meta = c.chunk_meta(*id).unwrap();
            assert_eq!(meta.offset, expected_offset);
            assert_eq!(meta.size, (payload.len() + 4) as u64);
            expected_offset += meta.size;
        }
        assert_eq!(expected_offset, bytes.len() as u64);
        assert_eq!(c.chunk_meta(*b"NOPE"), None);

        // TOC-only: a corrupted payload doesn't affect span resolution.
        let mut corrupt = bytes.clone();
        let last = corrupt.len() - 1;
        corrupt[last] ^= 0x01;
        let cc = Container::open(&corrupt, &MAGIC, VERSION).unwrap();
        assert_eq!(cc.chunk_meta(*b"CCCC"), c.chunk_meta(*b"CCCC"));
    }

    #[test]
    fn missing_chunk_is_not_found() {
        let bytes = build(&[(*b"AAAA", b"x")]);
        let c = Container::open(&bytes, &MAGIC, VERSION).unwrap();
        match c.chunk(*b"NOPE") {
            Err(Error::ChunkNotFound { id }) => assert_eq!(id, *b"NOPE"),
            other => panic!("expected ChunkNotFound, got {other:?}"),
        }
    }

    #[test]
    fn flipping_any_payload_or_crc_byte_fails_crc() {
        let clean = build(&[(*b"AAAA", b"some payload bytes"), (*b"BBBB", b"other")]);
        let payload_start = clean
            .windows(b"some payload bytes".len())
            .position(|w| w == b"some payload bytes")
            .unwrap();
        // Flip every byte of AAAA's payload + 4-byte CRC trailer, one at
        // a time; each flip must be caught.
        for i in payload_start..payload_start + b"some payload bytes".len() + CRC_LEN {
            let mut corrupt = clean.clone();
            corrupt[i] ^= 0x01;
            let cc = Container::open(&corrupt, &MAGIC, VERSION).unwrap();
            match cc.chunk(*b"AAAA") {
                Err(Error::CrcMismatch { id, .. }) => assert_eq!(id, *b"AAAA"),
                other => panic!("flip at {i}: expected CrcMismatch, got {other:?}"),
            }
            // The untouched chunk still verifies.
            assert_eq!(cc.chunk(*b"BBBB").unwrap(), b"other");
        }
    }

    #[test]
    fn open_rejects_bad_magic_version_and_short_input() {
        let bytes = build(&[(*b"AAAA", b"x")]);

        match Container::open(&bytes, b"OTHR", VERSION).err() {
            Some(Error::BadMagic) => {}
            other => panic!("expected BadMagic, got {other:?}"),
        }
        match Container::open(&bytes, &MAGIC, VERSION + 1).err() {
            Some(Error::UnsupportedVersion(v)) => assert_eq!(v, VERSION),
            other => panic!("expected UnsupportedVersion, got {other:?}"),
        }
        match Container::open(&bytes[..HEADER_SIZE - 1], &MAGIC, VERSION).err() {
            Some(Error::TooShort(..)) => {}
            other => panic!("expected TooShort, got {other:?}"),
        }
    }

    #[test]
    fn open_rejects_overlarge_num_chunks() {
        let mut bytes = build(&[(*b"AAAA", b"x")]);
        bytes[8..12].copy_from_slice(&u32::MAX.to_le_bytes());
        match Container::open(&bytes, &MAGIC, VERSION).err() {
            Some(Error::Malformed(msg)) => assert!(msg.contains("plausible maximum"), "{msg}"),
            other => panic!("expected Malformed, got {other:?}"),
        }
    }

    #[test]
    fn empty_builder_is_rejected_at_write() {
        let b = ContainerBuilder::new(MAGIC, VERSION);
        let mut out = Vec::new();
        assert!(b.write_to(&mut out).is_err());
    }
}
