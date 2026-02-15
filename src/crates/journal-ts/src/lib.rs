//! `.ts` sidecar file: histogram + entry offsets for journal files.
//!
//! Uses `journal-chunk` for the TOC. Three chunks in columnar layout:
//!
//! ```text
//! [TOC: 48 bytes]   3 chunks + end marker
//! [HTIM chunk]      pco(M × u32 bucket_start_seconds, sorted)
//! [HCNT chunk]      pco(M × u32 running_counts, monotonic)
//! [EOFF chunk]      pco(N × u64 entry_offsets, in timestamp order)
//! ```
//!
//! `bucket_duration` is always 1 second (hardcoded invariant, not stored).

use std::io::Write;

use bumpalo::Bump;
use bumpalo::collections::Vec as BVec;
use journal_chunk::TocWriter;
use pco::ChunkConfig;
use pco::data_types::Number;
use pco::standalone::{DecompressorItem, FileDecompressor, simple_compress_into};

// ── Pco decompression into bump ──────────────────────────────────

/// Extension trait for decompressing pco-compressed data into a bump-allocated `BVec`.
pub trait DecompressPco<'bump, T: Number + Default>: Sized {
    /// Decompress pco-compressed bytes into a new `BVec` in the bump arena.
    ///
    /// Uses the low-level chunk API to get exact element counts rather than
    /// relying on the file-level `n_hint`.
    fn decompress_pco(src: &[u8], bump: &'bump Bump) -> Result<Self, Error>;
}

impl<'bump, T: Number + Default> DecompressPco<'bump, T> for BVec<'bump, T> {
    fn decompress_pco(src: &[u8], bump: &'bump Bump) -> Result<Self, Error> {
        let (fd, mut remaining) = FileDecompressor::new(src)?;
        let mut dst = BVec::with_capacity_in(fd.n_hint(), bump);

        while let DecompressorItem::Chunk(mut chunk) = fd.chunk_decompressor(remaining)? {
            let start = dst.len();
            dst.resize(start + chunk.n(), T::default());
            chunk.read(&mut dst[start..])?;
            remaining = chunk.into_src();
        }

        Ok(dst)
    }
}

// ── Chunk IDs ───────────────────────────────────────────────────

/// Chunk ID for the compressed histogram bucket-start-seconds column.
pub const HTIM: [u8; 4] = *b"HTIM";
/// Chunk ID for the compressed histogram running-counts column.
pub const HCNT: [u8; 4] = *b"HCNT";
/// Chunk ID for the compressed entry-offsets column.
pub const EOFF: [u8; 4] = *b"EOFF";

// ── Error ───────────────────────────────────────────────────────

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("journal-chunk error: {0}")]
    Chunk(#[from] journal_chunk::Error),

    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    #[error("pco error: {0}")]
    Pco(#[from] pco::errors::PcoError),

    #[error("chunk size mismatch: HTIM has {htim} entries, HCNT has {hcnt}")]
    ChunkSizeMismatch { htim: usize, hcnt: usize },
}

// ── Writer ──────────────────────────────────────────────────────

/// Accumulates (timestamp, entry_offset) pairs and writes them as a `.ts` file.
///
/// Allocates from a caller-owned [`Bump`].  The intended lifecycle is:
///
/// ```ignore
/// let bump = Bump::new();
/// loop {
///     let mut w = TimestampOffsetsWriter::new_in(&bump);
///     // ... push entries for one journal file ...
///     w.write_to(&mut file)?;
///     drop(w);
///     bump.reset();   // frees everything at once
/// }
/// ```
pub struct TimestampOffsetsWriter<'bump> {
    bump: &'bump Bump,
    timestamps: BVec<'bump, u64>,
    offsets: BVec<'bump, u64>,
    indexes: BVec<'bump, u64>,
    is_sorted: bool,
}

impl<'bump> TimestampOffsetsWriter<'bump> {
    /// Create an empty writer allocating from `bump`.
    pub fn new_in(bump: &'bump Bump) -> Self {
        Self {
            bump,
            timestamps: BVec::new_in(bump),
            offsets: BVec::new_in(bump),
            indexes: BVec::new_in(bump),
            is_sorted: true,
        }
    }

    /// Create a writer with pre-allocated capacity in `bump`.
    pub fn with_capacity_in(cap: usize, bump: &'bump Bump) -> Self {
        Self {
            bump,
            timestamps: BVec::with_capacity_in(cap, bump),
            offsets: BVec::with_capacity_in(cap, bump),
            indexes: BVec::with_capacity_in(cap, bump),
            is_sorted: true,
        }
    }

    /// Append a (timestamp, entry_offset) pair.
    pub fn push(&mut self, timestamp_usec: u64, entry_offset: u64) {
        if self.is_sorted {
            if let Some(&last) = self.timestamps.last() {
                if timestamp_usec < last {
                    self.is_sorted = false;
                }
            }
        }
        self.timestamps.push(timestamp_usec);
        self.offsets.push(entry_offset);
    }

    /// Number of entries accumulated so far.
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    /// Whether no entries have been accumulated.
    pub fn is_empty(&self) -> bool {
        self.timestamps.is_empty()
    }

    /// Sort both columns by timestamp using a permutation index.
    fn sort_by_timestamp(&mut self) {
        let n = self.timestamps.len();

        // Build permutation: indexes[dest] = source.
        self.indexes.clear();
        self.indexes.extend(0..n as u64);
        self.indexes.sort_by_key(|&i| self.timestamps[i as usize]);

        // Apply source permutation to both columns via cycle-following.
        for i in 0..n {
            if self.indexes[i] as usize == i {
                continue;
            }
            let mut j = i;
            let tmp_ts = self.timestamps[i];
            let tmp_off = self.offsets[i];
            loop {
                let k = self.indexes[j] as usize;
                self.indexes[j] = j as u64; // mark visited
                if k == i {
                    self.timestamps[j] = tmp_ts;
                    self.offsets[j] = tmp_off;
                    break;
                }
                self.timestamps[j] = self.timestamps[k];
                self.offsets[j] = self.offsets[k];
                j = k;
            }
        }
    }

    /// Build histogram columns from sorted timestamps.
    ///
    /// Returns `(htim, hcnt)` where:
    /// - `htim[i]` is the bucket start time in seconds
    /// - `hcnt[i]` is the cumulative entry count through bucket `i`
    fn build_histogram(&self) -> (BVec<'bump, u32>, BVec<'bump, u32>) {
        let mut htim = BVec::<u32>::new_in(self.bump);
        let mut hcnt = BVec::<u32>::new_in(self.bump);

        if !self.timestamps.is_empty() {
            let mut current_sec = (self.timestamps[0] / 1_000_000) as u32;
            let mut count: u32 = 0;

            for &ts in self.timestamps.iter() {
                let sec = (ts / 1_000_000) as u32;
                if sec != current_sec {
                    htim.push(current_sec);
                    hcnt.push(count);
                    current_sec = sec;
                }
                count += 1;
            }

            // Push the last bucket.
            htim.push(current_sec);
            hcnt.push(count);
        }

        (htim, hcnt)
    }

    /// Sort if needed and compress into three columns (HTIM, HCNT, EOFF).
    ///
    /// The compressed buffers are allocated from the same bump arena.
    pub fn compress(&mut self) -> Result<CompressedTsSidecar<'bump>, Error> {
        if !self.is_sorted {
            self.sort_by_timestamp();
            self.is_sorted = true;
        }

        let (htim_vals, hcnt_vals) = self.build_histogram();

        let config = ChunkConfig::default();
        let htim = simple_compress_into(&htim_vals, &config, BVec::<u8>::new_in(self.bump))?;
        let hcnt = simple_compress_into(&hcnt_vals, &config, BVec::<u8>::new_in(self.bump))?;
        let eoff = simple_compress_into(&self.offsets, &config, BVec::<u8>::new_in(self.bump))?;
        Ok(CompressedTsSidecar { htim, hcnt, eoff })
    }

    /// Write a standalone `.ts` file to `w`. Sorts entries by timestamp if needed.
    ///
    /// This creates a 3-chunk TOC. To embed into a larger TOC, use
    /// [`compress()`](Self::compress) and write the chunks yourself.
    pub fn write_to<W: Write>(&mut self, w: W) -> Result<(), Error> {
        let compressed = self.compress()?;
        compressed.write_standalone(w)
    }
}

// ── Compressed blobs ────────────────────────────────────────────

/// The three pco-compressed columns, ready to be written as chunks.
///
/// Returned by [`TimestampOffsetsWriter::compress()`].  Can be written as a
/// standalone 3-chunk file via [`write_standalone()`](Self::write_standalone),
/// or embedded into a larger TOC using the [`HTIM`] / [`HCNT`] / [`EOFF`] chunk IDs.
///
/// Allocated from the same bump arena as the writer.
pub struct CompressedTsSidecar<'bump> {
    /// Compressed histogram bucket-start-seconds column (chunk ID: [`HTIM`]).
    pub htim: BVec<'bump, u8>,
    /// Compressed histogram running-counts column (chunk ID: [`HCNT`]).
    pub hcnt: BVec<'bump, u8>,
    /// Compressed entry-offsets column (chunk ID: [`EOFF`]).
    pub eoff: BVec<'bump, u8>,
}

impl CompressedTsSidecar<'_> {
    /// Write a standalone 3-chunk `.ts` file.
    pub fn write_standalone<W: Write>(&self, w: W) -> Result<(), Error> {
        let mut toc = TocWriter::new();
        toc.plan(HTIM, self.htim.len() as u64);
        toc.plan(HCNT, self.hcnt.len() as u64);
        toc.plan(EOFF, self.eoff.len() as u64);

        let mut cw = toc.write_toc(w)?;
        cw.write_chunk(HTIM, &self.htim)?;
        cw.write_chunk(HCNT, &self.hcnt)?;
        cw.write_chunk(EOFF, &self.eoff)?;
        cw.finish()?;

        Ok(())
    }
}

// ── Reader ──────────────────────────────────────────────────────

/// Histogram + entry offsets read from a `.ts` sidecar file.
///
/// Allocates from a caller-owned [`Bump`].
pub struct TsSidecar<'bump> {
    htim: BVec<'bump, u32>,
    hcnt: BVec<'bump, u32>,
    offsets: BVec<'bump, u64>,
}

impl<'bump> TsSidecar<'bump> {
    /// Parse a standalone `.ts` file (3-chunk TOC) from its raw bytes.
    pub fn from_bytes(data: &[u8], bump: &'bump Bump) -> Result<Self, Error> {
        let toc = journal_chunk::Toc::from_bytes(data, 3)?;

        let htim_compressed = toc.data(data, HTIM)?;
        let hcnt_compressed = toc.data(data, HCNT)?;
        let eoff_compressed = toc.data(data, EOFF)?;

        Self::from_chunks(htim_compressed, hcnt_compressed, eoff_compressed, bump)
    }

    /// Decompress from pre-extracted `HTIM`, `HCNT`, and `EOFF` chunk data.
    ///
    /// Use this when the chunks are embedded in a larger TOC — extract
    /// them with `toc.data(file, journal_ts::HTIM)` / `journal_ts::HCNT` /
    /// `journal_ts::EOFF` and pass them here.
    pub fn from_chunks(
        htim_compressed: &[u8],
        hcnt_compressed: &[u8],
        eoff_compressed: &[u8],
        bump: &'bump Bump,
    ) -> Result<Self, Error> {
        let htim = BVec::decompress_pco(htim_compressed, bump)?;
        let hcnt = BVec::decompress_pco(hcnt_compressed, bump)?;
        let offsets = BVec::decompress_pco(eoff_compressed, bump)?;

        if htim.len() != hcnt.len() {
            return Err(Error::ChunkSizeMismatch {
                htim: htim.len(),
                hcnt: hcnt.len(),
            });
        }

        Ok(Self {
            htim,
            hcnt,
            offsets,
        })
    }

    /// Number of entry offsets.
    pub fn len(&self) -> usize {
        self.offsets.len()
    }

    /// Whether the file contained zero entries.
    pub fn is_empty(&self) -> bool {
        self.offsets.is_empty()
    }

    /// Histogram bucket start times (seconds since epoch, sorted).
    pub fn bucket_starts(&self) -> &[u32] {
        &self.htim
    }

    /// Cumulative entry counts per histogram bucket (monotonic).
    pub fn running_counts(&self) -> &[u32] {
        &self.hcnt
    }

    /// Entry offsets into the journal file, in timestamp order.
    pub fn offsets(&self) -> &[u64] {
        &self.offsets
    }
}

// ── Tests ───────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trip_sorted() {
        let bump = Bump::new();
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        // 4 entries across 3 seconds (sec 1 has 2 entries)
        writer.push(1_000_000, 0x100); // sec 1
        writer.push(1_500_000, 0x150); // sec 1
        writer.push(2_000_000, 0x200); // sec 2
        writer.push(3_000_000, 0x300); // sec 3
        assert_eq!(writer.len(), 4);

        let mut buf = Vec::new();
        writer.write_to(&mut buf).unwrap();

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf, &rbump).unwrap();
        assert_eq!(reader.len(), 4);
        assert_eq!(reader.offsets(), &[0x100, 0x150, 0x200, 0x300]);

        // Histogram: 3 buckets
        assert_eq!(reader.bucket_starts(), &[1, 2, 3]);
        assert_eq!(reader.running_counts(), &[2, 3, 4]);
    }

    #[test]
    fn round_trip_unsorted() {
        let bump = Bump::new();
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        writer.push(3_000_000, 0x300); // sec 3
        writer.push(1_000_000, 0x100); // sec 1
        writer.push(4_000_000, 0x400); // sec 4
        writer.push(2_000_000, 0x200); // sec 2

        let mut buf = Vec::new();
        writer.write_to(&mut buf).unwrap();

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf, &rbump).unwrap();
        assert_eq!(reader.len(), 4);

        // Offsets must come back sorted by timestamp.
        assert_eq!(reader.offsets(), &[0x100, 0x200, 0x300, 0x400]);

        // One entry per second bucket.
        assert_eq!(reader.bucket_starts(), &[1, 2, 3, 4]);
        assert_eq!(reader.running_counts(), &[1, 2, 3, 4]);
    }

    #[test]
    fn round_trip_empty() {
        let bump = Bump::new();
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        assert!(writer.is_empty());

        let mut buf = Vec::new();
        writer.write_to(&mut buf).unwrap();

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf, &rbump).unwrap();
        assert!(reader.is_empty());
        assert_eq!(reader.len(), 0);
        assert_eq!(reader.bucket_starts().len(), 0);
        assert_eq!(reader.running_counts().len(), 0);
    }

    #[test]
    fn round_trip_large() {
        let bump = Bump::new();
        let count = 100_000;
        let mut writer = TimestampOffsetsWriter::with_capacity_in(count, &bump);

        // Each entry 1ms apart → 1000 entries per second bucket.
        for i in 0..count as u64 {
            writer.push(i * 1_000, i * 64);
        }
        assert_eq!(writer.len(), count);

        let mut buf = Vec::new();
        writer.write_to(&mut buf).unwrap();

        // Compressed should be significantly smaller than raw (2 × 100K × 8 = 1.6 MB).
        let raw_size = count * 8 * 2;
        assert!(
            buf.len() < raw_size / 2,
            "compressed {} should be < half of raw {}",
            buf.len(),
            raw_size,
        );

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf, &rbump).unwrap();
        assert_eq!(reader.len(), count);

        // Spot-check offsets.
        assert_eq!(reader.offsets()[0], 0);
        assert_eq!(reader.offsets()[count - 1], (count as u64 - 1) * 64);

        // 100K entries × 1ms = 100 seconds → 100 buckets of 1000 each.
        assert_eq!(reader.bucket_starts().len(), 100);
        assert_eq!(reader.bucket_starts()[0], 0);
        assert_eq!(reader.bucket_starts()[99], 99);
        assert_eq!(reader.running_counts()[0], 1000);
        assert_eq!(reader.running_counts()[99], 100_000);
    }

    #[test]
    fn compress_and_from_chunks() {
        let bump = Bump::new();
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        writer.push(1_000_000, 0x100); // sec 1
        writer.push(2_000_000, 0x200); // sec 2
        writer.push(3_000_000, 0x300); // sec 3

        let compressed = writer.compress().unwrap();

        let rbump = Bump::new();
        let reader =
            TsSidecar::from_chunks(&compressed.htim, &compressed.hcnt, &compressed.eoff, &rbump)
                .unwrap();
        assert_eq!(reader.len(), 3);
        assert_eq!(reader.offsets(), &[0x100, 0x200, 0x300]);
        assert_eq!(reader.bucket_starts(), &[1, 2, 3]);
        assert_eq!(reader.running_counts(), &[1, 2, 3]);
    }

    #[test]
    fn bump_reset_and_reuse() {
        let mut bump = Bump::new();

        // First cycle.
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        writer.push(1_000_000, 10); // sec 1
        writer.push(2_000_000, 20); // sec 2
        assert_eq!(writer.len(), 2);

        let mut buf1 = Vec::new();
        writer.write_to(&mut buf1).unwrap();
        drop(writer);
        bump.reset();

        // Second cycle — bump memory is reused.
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        writer.push(5_000_000, 50); // sec 5

        let mut buf2 = Vec::new();
        writer.write_to(&mut buf2).unwrap();

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf2, &rbump).unwrap();
        assert_eq!(reader.len(), 1);
        assert_eq!(reader.offsets()[0], 50);
        assert_eq!(reader.bucket_starts(), &[5]);
        assert_eq!(reader.running_counts(), &[1]);
    }

    #[test]
    fn same_second_multiple_entries() {
        let bump = Bump::new();
        let mut writer = TimestampOffsetsWriter::new_in(&bump);
        // All entries in the same second
        writer.push(1_000_000, 0x100);
        writer.push(1_200_000, 0x120);
        writer.push(1_500_000, 0x150);
        writer.push(1_999_999, 0x1FF);

        let mut buf = Vec::new();
        writer.write_to(&mut buf).unwrap();

        let rbump = Bump::new();
        let reader = TsSidecar::from_bytes(&buf, &rbump).unwrap();
        assert_eq!(reader.len(), 4);
        assert_eq!(reader.offsets(), &[0x100, 0x120, 0x150, 0x1FF]);
        assert_eq!(reader.bucket_starts(), &[1]);
        assert_eq!(reader.running_counts(), &[4]);
    }
}
