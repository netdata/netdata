//! WAL → SFST indexing pipeline.
//!
//! The build side of the [`sfst`] format: everything that turns a WAL
//! file of OTAP frames into an on-disk (or in-memory) SFST index lives
//! here, so the format crate stays free of the WAL/Arrow decode stack
//! and format-only consumers never compile it.
//!
//! Two-phase build:
//!
//! - **Phase 1** (read) — [`wal_otap::decode_file`] decodes each WAL
//!   frame and streams its rows into the [`RowIndex`] (a
//!   [`wal_otap::KvSink`]), which interns `key=value` attributes and
//!   accumulates the string interner + per-attribute bitmaps + per-log
//!   entries + per-log timestamps.
//! - **Phase 2** (write) — [`build_and_write`] consumes the `RowIndex` and
//!   emits the on-disk SFST file through [`sfst`]'s public format
//!   vocabulary (chunk ids, magic, version, `pack`).
//!
//! The public entry points are [`index`] (defaults) and
//! [`index_with_options`] (cardinality threshold override). The frame
//! decode lives in [`wal_otap`] so other consumers — e.g. the
//! query-time WAL row scan in `sfsq` — share it rather than
//! reimplement it.

mod bitset;
mod error;
mod fst_builder;
pub mod kv_interner;
pub mod row_index;

pub use error::IndexError;
pub use fst_builder::{build_and_write, build_into};
pub use kv_interner::KvSlot;


use std::path::Path;

use bumpalo::Bump;

use row_index::RowIndex;
use sfst::{Metadata, Summary};

/// Initial capacity of the per-build bump arena that backs the
/// [`RowIndex`]'s interned strings and bitmaps. Sized so a typical
/// rotation-sized WAL builds without an arena grow.
const INDEX_ARENA_BYTES: usize = 32 * 1024 * 1024;

/// Result of indexing a WAL file.
///
/// The earliest log date is derivable from `summary.min_timestamp_s` — it
/// is not returned separately.
pub struct IndexResult {
    /// Cheap summary fields written into the SFST `SUMR` chunk and stored
    /// inline on the registry entry.
    pub summary: Summary,
    /// Heavy index metadata (histogram + id_ranges + field table) written
    /// into the `META` chunk. Used at query time, not by the registry.
    pub metadata: Metadata,
    /// Byte size of the written SFST file.
    pub size: u64,
}

/// Build a split-FST index from a WAL file using default settings.
///
/// Reads the WAL file at `wal_path` and writes the index to `sfst_path`.
pub fn index(wal_path: &Path, sfst_path: &Path) -> Result<IndexResult, IndexError> {
    index_with_options(wal_path, sfst_path, sfst::DEFAULT_CARDINALITY_THRESHOLD)
}

/// Build a split-FST index from a WAL file with an explicit cardinality
/// threshold.
///
/// Fields with fewer unique values than `cardinality_threshold` go into the
/// primary FST; fields above split into per-field secondary chunks.
pub fn index_with_options(
    wal_path: &Path,
    sfst_path: &Path,
    cardinality_threshold: u32,
) -> Result<IndexResult, IndexError> {
    let arena = Bump::with_capacity(INDEX_ARENA_BYTES);
    let mut row_index = RowIndex::new(&arena, cardinality_threshold);

    let stats = wal_otap::decode_file(wal_path, &mut row_index)?;

    tracing::info!(
        "WAL file read complete path={} frames={} logs={}",
        wal_path.display(),
        stats.frames,
        row_index.num_logs(),
    );

    let (summary, metadata) = build_and_write(&row_index, sfst_path, None)?;
    let size = std::fs::metadata(sfst_path)?.len();

    Ok(IndexResult {
        summary,
        metadata,
        size,
    })
}

/// Index a frame-aligned byte `range` of a WAL file into an **in-memory**
/// SFST, returning its [`Summary`] and the serialized bytes.
///
/// The same two-phase build as [`index`], but reading only the frames in
/// `range` (via [`wal::Reader::open_range`]) and serializing the result to
/// a `Vec<u8>` instead of a file. This is how a query builds an index over
/// a chunk of an active WAL; see [`wal::FrameRange`] / `open_range` for the
/// frame-boundary and durable-prefix soundness checks.
///
/// The returned bytes parse with [`sfst::IndexReader::open`]. The caller
/// cross-checks `summary.record_count` against the expected record count for
/// the range (the registry's `entry_count`) to confirm the prefix wasn't
/// truncated — the count check that [`wal::Reader::open_range`] defers.
pub fn index_range(
    wal_path: &Path,
    range: wal::FrameRange,
) -> Result<(Summary, Vec<u8>), IndexError> {
    let arena = Bump::with_capacity(INDEX_ARENA_BYTES);
    let mut row_index = RowIndex::new(&arena, sfst::DEFAULT_CARDINALITY_THRESHOLD);

    let stats = wal_otap::decode_range(wal_path, range, &mut row_index)?;
    tracing::debug!(
        "WAL range read complete path={} start={} end={} frames={} logs={}",
        wal_path.display(),
        range.start(),
        range.end(),
        stats.frames,
        row_index.num_logs(),
    );

    // Stream the chunks straight into the output buffer: each packed
    // chunk is written and dropped in turn, so beyond the arena and the
    // RowIndex only the accumulated output plus one packed chunk are
    // ever held — the old build-everything-then-copy model held every
    // packed chunk and then a second full copy in the output.
    let cursor = std::io::Cursor::new(Vec::new());
    let (cursor, summary, _metadata) = build_into(&row_index, cursor, None)?;

    Ok((summary, cursor.into_inner()))
}
