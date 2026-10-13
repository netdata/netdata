//! `ng-index`: build a standard SFST index file from a flattened-frame WAL.
//!
//! The WAL of flattened frames is written by the OTel receivers — `otel-ingestor`
//! in production, `ng-ingest` standalone — in the frame format owned by `ng-flatten`.
//! This crate decodes it, feeds the typed, array-collapsed entries into an
//! [`sfst::RowIndex`], and seals a standard SFST file: logs via [`build_sfst`], traces
//! via [`build_sfst_traces_file`]. Consumers: `otel-ledger` and `sfsq`.

use std::path::{Path, PathBuf};

mod perf;
mod sfst_build;
pub use perf::{Metrics, Rss, read_rss};
pub use sfst_build::{
    SfstStats, build_sfst, build_sfst_file, build_sfst_range, build_sfst_traces_file,
    build_sfst_traces_range, to_sfst_tree,
};

// Re-export the flattening + frame vocabulary so consumers get it from `ng-index`
// alone rather than adding `ng-flatten`; in-tree, `sfst_build` reads it via `crate::`.
pub use ng_flatten::{
    Entry, FlattenedLogRequest, NodeId, SchemaTree, Value, build_kv, decode_log_frame,
};

/// Errors reading a flattened WAL or building an SFST from it.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    // Wrapper variants (`Wal`, `Io`, `Sfst`) are `transparent` — `#[from]` already
    // chains the source, so embedding it in the message too would print it twice.
    #[error(transparent)]
    Wal(#[from] wal::Error),
    #[error(transparent)]
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
    // The decode error is embedded in the message, NOT chained: a field named
    // `source` would be auto-chained by thiserror and then print twice in anyhow
    // chains. Display-only consumers (the ledger indexer logs `{e}`) need the
    // decode reason in the message itself. `frame` is 1-based within the WAL.
    #[error("frame {frame}: bincode decode failed: {err}")]
    BincodeDecode {
        frame: u64,
        err: bincode::error::DecodeError,
    },
    #[error(transparent)]
    Sfst(#[from] sfst::Error),
}

/// The single `.wal` file inside `dir`; `NoWal` if none, `MultipleWal` if several.
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
