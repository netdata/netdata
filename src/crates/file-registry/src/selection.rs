//! A signal-neutral description of a sealed file picked by query selection.

use std::path::PathBuf;

use crate::{FileId, FileSummary};

/// One sealed file the selector chose for a query: its identity, its cheap
/// summary, and where its bytes live on disk.
///
/// This is the substrate-level result of selection — it carries no
/// query-engine concepts (no chunk index, no in-memory/row-scan source). A
/// content-plane query engine converts it into its own candidate type at its
/// boundary (for OTel logs, `sfsq` implements `From<SelectedFile>` for its
/// `SfstCandidate`). Keeping selection engine-neutral lets the file-lifecycle
/// be reused by a second signal without depending on the logs engine.
///
/// Covers the sealed-SFST sources only — both local on-disk and remote files
/// materialized back to a local cache path. The active-WAL tail and the
/// in-memory chunks built from an unsealed WAL stay engine-specific and are
/// not represented here.
#[derive(Debug, Clone)]
pub struct SelectedFile {
    /// The file's identity. Its `part_key` (the partition key) is the single
    /// source of truth for the file's partition — read from here, never from
    /// the summary.
    pub id: FileId,
    /// The cheap summary lifted from the file (time range, record count, the
    /// opaque content-plane `content_meta`).
    pub summary: FileSummary,
    /// Where the file's bytes are: a local SFST path, or the local cache path a
    /// remote file was materialized to.
    pub path: PathBuf,
}
