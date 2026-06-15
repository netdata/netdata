//! Message types for communication between the ledger and its components.
//!
//! Component message types use plain tokio channels (no serialization needed).
//! The ingestor connection uses ferryboat IPC since it runs in a separate process.

use std::path::PathBuf;

use ferryboat::{Connection, Endpoint, Listener};
use file_registry::TenantId;

/// Default socket path for the writer → ledger connection.
pub const WRITER_SOCKET_PATH: &str = "/tmp/netdata-ledger-writer.sock";

/// Requests sent from the ledger to the cleaner.
#[derive(Debug, Clone)]
pub enum CleanerRequest {
    /// Delete a WAL file that has been successfully indexed.
    DeleteWalFile { sequence: u64, path: PathBuf },
    /// Delete an index file evicted by retention policy.
    DeleteIndexFile { sequence: u64, path: PathBuf },
    /// Delete a catalog file evicted by retention policy. Catalog files are
    /// not seq-keyed (they span multiple SFST seqs), so they're routed by
    /// path.
    DeleteCatalogFile { path: PathBuf },
}

/// Responses sent from the cleaner back to the ledger.
#[derive(Debug, Clone)]
pub enum CleanerResponse {
    /// A WAL file has been deleted.
    WalFileDeleted { sequence: u64 },
    /// An index file has been deleted.
    IndexFileDeleted { sequence: u64 },
    /// A catalog file has been deleted.
    CatalogFileDeleted { path: PathBuf },
    /// Failed to delete a WAL file.
    WalFileFailed { sequence: u64, error: String },
    /// Failed to delete an index file.
    IndexFileFailed { sequence: u64, error: String },
    /// Failed to delete a catalog file.
    CatalogFileFailed { path: PathBuf, error: String },
}

/// Requests sent from the ledger to the indexer.
#[derive(Debug, Clone)]
pub enum IndexerRequest {
    /// Build an SFST index for the given WAL file.
    Index {
        /// Path to the `.wal` file to index.
        wal_path: PathBuf,
        /// Path where the `.sfst` index should be written.
        sfst_path: PathBuf,
    },
}

/// Responses sent from the indexer back to the ledger.
#[derive(Debug, Clone)]
pub enum IndexerResponse {
    /// The SFST has been written successfully.
    Indexed {
        seq: u64,
        path: PathBuf,
        /// Cheap summary fields (min/max timestamp, total logs, stream).
        /// Stored on the registry entry on `track`; used by the uploader
        /// response handler to build the catalog entry directly from the
        /// registry without a pending-metadata side-channel.
        summary: sfst::Summary,
        /// Byte size of the written SFST file.
        size: file_registry::ByteSize,
    },
    /// Indexing failed for a file.
    IndexFailed { path: PathBuf, error: String },
}

/// Requests sent from the ledger to the uploader.
#[derive(Debug, Clone)]
pub enum UploaderRequest {
    /// Upload an index (SFST) file to remote object storage.
    Upload {
        seq: u64,
        local_path: PathBuf,
        remote_key: String,
    },
    /// Upload a catalog file to remote object storage.
    UploadCatalog {
        local_path: PathBuf,
        remote_key: String,
    },
}

/// Responses sent from the uploader back to the ledger.
#[derive(Debug, Clone)]
pub enum UploaderResponse {
    /// An SFST file has been uploaded successfully.
    Uploaded { seq: u64, remote_key: String },
    /// Failed to upload an SFST file.
    UploadFailed { seq: u64, error: String },
    /// A catalog file has been uploaded successfully.
    CatalogUploaded {
        local_path: PathBuf,
        remote_key: String,
    },
    /// Failed to upload a catalog file.
    CatalogUploadFailed {
        local_path: PathBuf,
        remote_key: String,
        error: String,
    },
}

/// Requests sent from the ledger to the catalog builder.
#[derive(Debug, Clone)]
pub enum CatalogBuilderRequest {
    /// Add a newly-uploaded SFST's catalog entry to the in-memory accumulator
    /// for its scope `(tenant_id, date, entry.id.machine_id, entry.id.boot_id)`.
    /// The builder may rotate and emit a new catalog file as a side effect
    /// (see [`CatalogBuilderResponse::Rotated`]).
    AddEntry {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        entry: otel_catalog::CatalogEntry,
    },
}

/// Responses sent from the catalog builder back to the ledger.
#[derive(Debug, Clone)]
pub enum CatalogBuilderResponse {
    /// The entry joined the accumulator; no rotation was triggered.
    EntryAccepted { seq: u64 },
    /// The accumulator reached the rotation threshold and a new catalog
    /// file was written to `path`. The accumulator for this scope is now
    /// empty. The ledger is responsible for registering the file and
    /// sending it to the uploader.
    Rotated {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        machine_id: uuid::Uuid,
        boot_id: uuid::Uuid,
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
    /// left intact so the next `AddEntry` will retry.
    RotationFailed {
        tenant_id: TenantId,
        date: chrono::NaiveDate,
        machine_id: uuid::Uuid,
        boot_id: uuid::Uuid,
        max_seq: u64,
        error: String,
    },
}

/// Accept a WAL event connection from the ingestor on the given socket path.
pub async fn accept_writer(
    socket_path: &str,
) -> Result<Connection<(), wal::Message>, ferryboat::Error> {
    let _ = std::fs::remove_file(socket_path);
    let endpoint = Endpoint::ipc(socket_path);
    let mut listener = Listener::<(), wal::Message>::bind(endpoint).open()?;
    listener.accept().await
}
