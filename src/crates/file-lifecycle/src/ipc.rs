//! The file-lifecycle substrate's IPC boundary: the ledger's message
//! vocabulary plus the one cross-process link it accepts.
//!
//! Two transport classes share this module:
//!
//! - The worker request/response enum pairs ([`CleanerRequest`],
//!   [`IndexerRequest`], [`UploaderRequest`], [`CatalogBuilderRequest`] and
//!   their responses) ride the unbounded tokio mpsc channels of
//!   [`crate::component::ComponentHandle`]; they derive no serde traits, so
//!   they can never cross a ferryboat link.
//! - [`accept_writer`] and [`WRITER_SOCKET_PATH`]: the ledger's end of the
//!   single ingestor → ledger WAL-event link, a ferryboat IPC connection
//!   carrying `wal::Message`. Those wire types live in the `wal` crate;
//!   this module owns only the accept plumbing.
//!
//! The WAL-event link runs at ferryboat's 8 MiB default size limit with no
//! compression: this end sets no override, so
//! `otel_bridge::IPC_MAX_MESSAGE_SIZE` (the supervisor↔worker ceiling) does not
//! apply.

use std::path::PathBuf;

use ferryboat::{Connection, Endpoint, Listener};
use file_registry::{FileId, Identity, SeqKey, TenantId};

/// Socket path for the writer → ledger link — a default, not the value
/// production uses.
pub const WRITER_SOCKET_PATH: &str = "/tmp/netdata-ledger-writer.sock";

/// Requests sent from the ledger to the cleaner.
///
/// Each request carries the `pipeline_id` of the pipeline that owns the file
/// so the cleaner can echo it back on the response, letting the run-loop
/// route the registry mutation to the right pipeline: `id.pipeline_id` for
/// requests built from a `file_registry::FileId`, the sending pipeline's own
/// id for the path-keyed catalog delete (catalog files carry no signal axis).
#[derive(Debug, Clone)]
pub enum CleanerRequest {
    /// Delete a WAL file that has been successfully indexed.
    DeleteWalFile {
        pipeline_id: u16,
        sequence: u64,
        path: PathBuf,
    },
    /// Delete an index file evicted by retention policy. Keyed by full
    /// [`SeqKey`]: its confirmation drives `evict_seq`, which destroys
    /// identity-keyed lifecycle state, so the echo must name the exact identity.
    DeleteIndexFile {
        pipeline_id: u16,
        sequence: SeqKey,
        path: PathBuf,
    },
    /// Delete a catalog file evicted by retention policy. Catalog files are
    /// not seq-keyed (they span multiple SFST seqs), so they're routed by
    /// path.
    DeleteCatalogFile { pipeline_id: u16, path: PathBuf },
}

/// Responses sent from the cleaner back to the ledger. `pipeline_id` is
/// echoed from the request so the run-loop can route the registry mutation to
/// the owning pipeline. `*Deleted` confirms the path is gone (or was never
/// there); `*Failed` leaves it on disk and carries the error string.
#[derive(Debug, Clone)]
pub enum CleanerResponse {
    /// A WAL file has been deleted.
    WalFileDeleted { pipeline_id: u16, sequence: u64 },
    /// An index file has been deleted.
    IndexFileDeleted { pipeline_id: u16, sequence: SeqKey },
    /// A catalog file has been deleted.
    CatalogFileDeleted { pipeline_id: u16, path: PathBuf },
    /// Failed to delete a WAL file.
    WalFileFailed {
        pipeline_id: u16,
        sequence: u64,
        error: String,
    },
    /// Failed to delete an index file.
    IndexFileFailed {
        pipeline_id: u16,
        sequence: SeqKey,
        error: String,
    },
    /// Failed to delete a catalog file.
    CatalogFileFailed {
        pipeline_id: u16,
        path: PathBuf,
        error: String,
    },
}

/// Requests sent from the ledger to the indexer.
#[derive(Debug, Clone)]
pub enum IndexerRequest {
    /// Index the given WAL file into an SFST.
    Index {
        /// The file's identity; the response carries it back, so neither
        /// side reads it out of a path.
        file_id: FileId,
        /// Path to the `.wal` file to index.
        wal_path: PathBuf,
        /// Path where the `.sfst` index should be written.
        sfst_path: PathBuf,
    },
}

/// Responses sent from the indexer back to the ledger.
#[derive(Debug, Clone)]
pub enum IndexerResponse {
    /// The SFST has been written successfully, at the request's `sfst_path`.
    Indexed {
        /// The indexed WAL file, as in the request.
        file_id: FileId,
        /// Cheap summary fields (min/max timestamp, record count, opaque content_meta).
        summary: sfst::Summary,
        /// Byte size of the written SFST file.
        size: file_registry::ByteSize,
    },
    /// Indexing failed for a file.
    IndexFailed {
        /// The WAL file that was not indexed, as in the request.
        file_id: FileId,
        error: String,
    },
}

/// Requests sent from the ledger to the uploader.
///
/// Each request carries the owning `pipeline_id` so the uploader can echo it
/// back, letting the run-loop route the response to the right pipeline.
/// A failed upload lands in the ledger's upload-retry queue, which re-sends
/// the stored request verbatim on its timer — hence the failure responses
/// carry every field the retry needs.
#[derive(Debug, Clone)]
pub enum UploaderRequest {
    /// Upload an index (SFST) file to remote object storage.
    Upload {
        pipeline_id: u16,
        seq: SeqKey,
        local_path: PathBuf,
        remote_key: String,
    },
    /// Upload a catalog file to remote object storage.
    UploadCatalog {
        pipeline_id: u16,
        local_path: PathBuf,
        remote_key: String,
        /// The single identity every entry in this catalog belongs to (a
        /// catalog is single-identity by construction — the accumulator scope
        /// key carries it). Echoed on the response so the ledger keys the
        /// remote-cataloged marks by full identity, not bare seq.
        identity: Identity,
        /// SFST sequence numbers contained in this catalog. Carried through to
        /// the response so the ledger can mark them remotely-cataloged (which
        /// gates their local eviction) only once the catalog is durably on the
        /// remote.
        seqs: Vec<u64>,
    },
}

/// Responses sent from the uploader back to the ledger. `pipeline_id` is echoed
/// from the request so the run-loop can route the registry mutation to the
/// owning pipeline.
#[derive(Debug, Clone)]
pub enum UploaderResponse {
    /// An SFST file has been uploaded successfully.
    Uploaded {
        pipeline_id: u16,
        seq: SeqKey,
        remote_key: String,
        /// Remote object validator (S3 ETag) returned by the write, if the
        /// backend supplied one.
        etag: Option<String>,
    },
    /// Failed to upload an SFST file. `local_path`/`remote_key` are carried so
    /// the retry queue can re-issue the upload without rebuilding them.
    UploadFailed {
        pipeline_id: u16,
        seq: SeqKey,
        local_path: PathBuf,
        remote_key: String,
        error: String,
    },
    /// A catalog file has been uploaded successfully. `seqs` are the SFSTs it
    /// covers.
    CatalogUploaded {
        pipeline_id: u16,
        local_path: PathBuf,
        remote_key: String,
        /// The catalog's single identity, echoed from the request so the marks
        /// are keyed by full identity (a recovered prior-instance catalog marks
        /// under its own identity, not the running process's).
        identity: Identity,
        seqs: Vec<u64>,
    },
    /// Failed to upload a catalog file. `seqs` are carried so a retry can be
    /// re-issued without re-reading the catalog.
    CatalogUploadFailed {
        pipeline_id: u16,
        local_path: PathBuf,
        remote_key: String,
        identity: Identity,
        seqs: Vec<u64>,
        error: String,
    },
}

/// Requests sent from the ledger to the catalog builder.
#[derive(Debug, Clone)]
pub enum CatalogBuilderRequest {
    /// Add a newly-uploaded SFST's catalog entry to the in-memory accumulator
    /// for its scope `(tenant_id, date, entry.id.machine_id, entry.id.instance_id)`.
    /// The builder may rotate and emit a new catalog file as a side effect
    /// (see [`CatalogBuilderResponse::Rotated`]).
    AddEntry {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        entry: otel_catalog::CatalogEntry,
    },
    /// Rotate every non-empty accumulator to a local catalog file now,
    /// regardless of the count/time triggers. Used on clean shutdown to persist
    /// in-flight accumulators before exit. Each rotated scope emits a
    /// [`CatalogBuilderResponse::Rotated`] (as usual), followed by a single
    /// [`CatalogBuilderResponse::FlushComplete`] once all scopes are handled.
    /// Only the local writes are guaranteed here.
    Flush,
}

/// Responses sent from the catalog builder back to the ledger.
#[derive(Debug, Clone)]
pub enum CatalogBuilderResponse {
    /// The entry joined the accumulator; no rotation was triggered.
    EntryAccepted { seq: u64 },
    /// A scope's accumulator rotated and a catalog file with its entries was
    /// written to `path` (count, time, or `Flush` trigger), in the place of
    /// a file with the same name if there was one. The accumulator for this
    /// scope is now empty. The ledger is responsible for registering the file
    /// and sending it to the uploader.
    Rotated {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        identity: file_registry::Identity,
        /// Highest SFST seq covered by the rotated catalog.
        max_seq: u64,
        /// Union `[min_timestamp_s, max_timestamp_s]` across all
        /// entries in the rotated catalog. Encoded into the filename
        /// so the planner can pre-filter without opening the file.
        min_timestamp_s: u32,
        max_timestamp_s: u32,
        path: std::path::PathBuf,
        size: file_registry::ByteSize,
        /// All SFST sequence numbers included in the rotated catalog file.
        seqs: Vec<u64>,
    },
    /// Rotation failed (serialization or local write). The accumulator is
    /// left intact; the next trigger (`AddEntry`, check tick, `Flush`)
    /// retries.
    RotationFailed {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        identity: file_registry::Identity,
        max_seq: u64,
        error: String,
    },
    /// The builder has finished a [`CatalogBuilderRequest::Flush`]: emitted
    /// exactly once, after every scope's
    /// [`Rotated`](CatalogBuilderResponse::Rotated) /
    /// [`RotationFailed`](CatalogBuilderResponse::RotationFailed). It signals
    /// "done", not "all succeeded". A scope whose rotation failed keeps its
    /// accumulator, and `FlushComplete` still follows.
    FlushComplete,
}

/// Accept the ingestor's WAL-event connection — the ledger side of the single
/// ingestor → ledger link, `Connection<(), wal::Message>`: the ledger only
/// receives (`()` is its never-used send half).
///
/// Exactly one writer connection is served: the function accepts once and
/// returns, dropping the listener — which also unlinks the Unix socket path
/// (ferryboat transport drop), so a second writer cannot attach.
///
/// The leading unlink is belt-and-suspenders: ferryboat's `Listener::open`
/// already removes a stale Unix socket file at the same path. Ignoring its
/// error is safe. Absence is the common case, and a file that survives
/// removal fails the bind below with its own error. Bind and accept errors
/// surface as `ferryboat::Error`.
pub async fn accept_writer(
    socket_path: &str,
) -> Result<Connection<(), wal::Message>, ferryboat::Error> {
    let _ = std::fs::remove_file(socket_path);
    let endpoint = Endpoint::ipc(socket_path);
    let mut listener = Listener::<(), wal::Message>::bind(endpoint).open()?;
    listener.accept().await
}
