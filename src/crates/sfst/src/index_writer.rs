//! The high-level SFST writer — the write-side counterpart of
//! [`IndexReader`](crate::IndexReader). A thin facade: both entry points
//! delegate to the Phase-2 builder in `build.rs` (this file is its only
//! consumer; the format pipeline is documented there, down to the
//! low-level container writer in `writer.rs`). Production callers:
//! `ng-index/src/sfst_build.rs` — seal-time file builds and in-memory
//! range builds over active WALs — plus this crate's own tests.

use std::io::{Seek, Write};
use std::path::Path;

use crate::row_index::RowIndex;
use crate::{Error, Metadata, Summary, build};

/// Writes complete SFST files from a producer-filled [`RowIndex`].
///
/// A producer fills a [`RowIndex`] (interned `key=value` attributes,
/// timestamps, optional per-row columns — see `row_index.rs`) and hands
/// it to [`write_file`](Self::write_file) (durable file) or
/// [`write_into`](Self::write_into) (any `Write + Seek` sink). The writer
/// owns the whole Phase-2 build — time-sorting, FST construction, chunk
/// packing, the canonical chunk order — all driven by the cardinality
/// tiers the [`RowIndex`] already carries; callers never touch chunk
/// mechanics.
///
/// `content_meta` is the producer's opaque identity blob, stored verbatim in
/// the [`Summary`] — never derived or inspected here.
pub struct IndexWriter;

impl IndexWriter {
    /// Build and durably write an SFST file at `out_path`: stream the
    /// chunks to `<out_path>.tmp`, then fsync → rename → parent-dir fsync
    /// (`file_registry::durable::AtomicFile`). Any failure or crash reaps
    /// the temp, so `out_path` never holds a partial file.
    ///
    /// Returns the cheap-to-read [`Summary`] (the per-file summary a
    /// registry stores inline) and the heavier [`Metadata`] (the `META`
    /// chunk's query-time payload) — callers that only register the file
    /// can drop the metadata.
    pub fn write_file(
        row_index: &RowIndex,
        out_path: &Path,
        content_meta: Vec<u8>,
    ) -> Result<(Summary, Metadata), Error> {
        build::build_and_write(row_index, out_path, content_meta)
    }

    /// Stream an SFST into `sink` (positioned at offset 0), returning the
    /// sink plus the [`Summary`] / [`Metadata`] the file carries — the
    /// in-memory builds feed a `Cursor` and open the resulting bytes with
    /// [`IndexReader::open`](crate::IndexReader::open). Peak memory beyond
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
