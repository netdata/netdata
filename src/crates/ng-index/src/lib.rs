//! `ng-index`: build a standard SFST index file from a flattened-frame WAL.
//!
//! The WAL of flattened frames is produced by `ng-ingest` (flatten-at-ingest). This
//! crate reads it and feeds the typed, array-collapsed entries into an
//! `sfst::RowIndex` to emit a standard SFST file — the augment-SFST path (see
//! [`build_sfst`]). The flattened-frame format itself (types, rendering, bincode
//! encode/decode) lives in `ng-flatten`.

use std::path::{Path, PathBuf};

mod perf;
mod sfst_build;
pub use perf::{Metrics, Rss, read_rss};
pub use sfst_build::{
    SfstStats, build_sfst, build_sfst_file, build_sfst_range, build_sfst_traces_file,
};

// Re-export the flattening + frame vocabulary so the binary (and any consumer) gets
// it from `ng-index` without depending on `ng-flatten` directly.
pub use ng_flatten::{
    Entry, FlattenedLogRequest, NodeId, SchemaTree, Value, build_kv, decode_log_frame,
};

/// Errors reading a flattened WAL or building an SFST from it.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("wal error: {0}")]
    Wal(#[from] wal::Error),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
    #[error("no .wal file found in {0}")]
    NoWal(PathBuf),
    #[error("multiple .wal files in {0}; expected exactly one")]
    MultipleWal(PathBuf),
    #[error(
        "WAL payload format {found} is not the expected {expected}; \
         refusing to decode frames written by a different codec"
    )]
    PayloadFormat { found: u16, expected: u16 },
    #[error("frame {frame}: bincode decode failed: {source}")]
    BincodeDecode {
        frame: u64,
        source: bincode::error::DecodeError,
    },
    #[error("sfst build failed: {0}")]
    Sfst(#[from] sfst::Error),
}

/// The single `.wal` file inside `dir` (the flattened WAL).
fn sole_wal_file(dir: &Path) -> Result<PathBuf, Error> {
    let mut found = None;
    for entry in std::fs::read_dir(dir)? {
        let path = entry?.path();
        if path.extension().is_some_and(|x| x == "wal") {
            if found.is_some() {
                return Err(Error::MultipleWal(dir.to_path_buf()));
            }
            found = Some(path);
        }
    }
    found.ok_or_else(|| Error::NoWal(dir.to_path_buf()))
}
