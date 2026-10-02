//! The result of query selection: one sealed file chosen for a query,
//! ready for a query engine to open.
//!
//! Selection happens in `file-lifecycle`, which builds this type in both
//! of its producers: the per-tenant registry — `Registry::sfst_candidates`,
//! also returned by `Registry::query_snapshot` — wraps the tenant's local
//! sealed SFSTs (the path is the SFST registry's derived file path), and
//! `remote_read::RemoteRead::fetch` wraps remote catalog entries it
//! materializes into the shared download cache (path: the cache pin's
//! path; summary: the catalog's stored one). The active-WAL side of a
//! snapshot — the row-scanned tail and the in-memory chunks — is
//! engine-specific and never travels as this type.
//!
//! Consumers (grep-verified): `sfsq::logs` implements
//! `From<SelectedFile> for SfstCandidate`, which the `otel-ledger` logs
//! handler applies to both origins (local snapshot, fetched files); the
//! `otel-ledger` traces planner builds `TraceSfstCandidate`
//! (`Source::File`) from it directly.

use std::path::PathBuf;

use crate::{FileId, FileSummary};

/// One sealed file the selector chose for a query: its identity, its cheap
/// summary, and where its bytes live on disk.
///
/// Substrate-level and engine-neutral: it carries no query-engine concepts
/// (no chunk index, no in-memory/row-scan source) — the content plane maps
/// it to its own candidate type at its boundary. Owned data, so the
/// producer can drop its registry lock before any file I/O touches the
/// `path`.
#[derive(Debug, Clone)]
pub struct SelectedFile {
    /// The file's identity. The partition key lives in
    /// [`FileId::part_key`] — never in the summary — and the candidate
    /// filters read it there (the shared rule is
    /// [`Query::matches_partition`](crate::Query::matches_partition)).
    pub id: FileId,
    /// The cheap summary the selection filtered on: the SFST registry's
    /// inline copy for a local file, the catalog's stored summary for a
    /// fetched one. Its `[min, max]` second range is what
    /// [`Query::overlaps`](crate::Query::overlaps) matched; retention and
    /// empty-file behavior read the same fields (see [`FileSummary`]).
    pub summary: FileSummary,
    /// Where the bytes are: a local SFST path, or the cache path a remote
    /// file was materialized to (the producer's download pins must stay
    /// alive until the file is read, or the cache may evict it).
    pub path: PathBuf,
}
