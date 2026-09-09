//! The high-level SFST writer — the write-side counterpart of
//! [`IndexReader`](crate::IndexReader).

use std::io::{Seek, Write};
use std::path::Path;

use crate::row_index::RowIndex;
use crate::{Error, Metadata, Summary, build};

/// Writes complete SFST files from a producer-filled [`RowIndex`].
///
/// A producer interns each row's `key=value` attributes into a [`RowIndex`]
/// (plus any per-row columns), then hands it to [`write_file`](Self::write_file)
/// (durable file) or [`write_into`](Self::write_into) (any `Write + Seek`
/// sink, e.g. an in-memory range build). The writer owns the whole build:
/// field-tier classification, time-sorting, FST construction, chunk packing,
/// and the canonical chunk order — callers never touch chunk mechanics.
///
/// `content_meta` is the producer's opaque identity blob, stored verbatim in
/// the [`Summary`] — never derived or inspected here.
pub struct IndexWriter;

impl IndexWriter {
    /// Build and durably write an SFST file at `out_path` (write to a temp
    /// file, fsync, rename, fsync the parent directory — a crash never leaves
    /// a partial file at the final name).
    ///
    /// Returns both the cheap-to-read [`Summary`] (which the registry stores
    /// inline) and the heavier [`Metadata`] (only needed for query planning
    /// and execution).
    pub fn write_file(
        row_index: &RowIndex,
        out_path: &Path,
        content_meta: Vec<u8>,
    ) -> Result<(Summary, Metadata), Error> {
        build::build_and_write(row_index, out_path, content_meta)
    }

    /// Stream an SFST into `sink` (positioned at offset 0), returning the sink
    /// plus the [`Summary`] / [`Metadata`] the file carries. Peak memory beyond
    /// the [`RowIndex`] itself is a single packed chunk, not the whole
    /// compressed file.
    pub fn write_into<W: Write + Seek>(
        row_index: &RowIndex,
        sink: W,
        content_meta: Vec<u8>,
    ) -> Result<(W, Summary, Metadata), Error> {
        build::build_into(row_index, sink, content_meta)
    }

}
