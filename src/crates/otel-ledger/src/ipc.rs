//! Message types for communication between the ledger and its components.
//!
//! Component message types use plain tokio channels (no serialization needed).
//! The ingestor connection uses ferryboat IPC since it runs in a separate process.

use std::path::PathBuf;

use ferryboat::{Connection, Endpoint, Listener};

/// Default socket path for the writer → ledger connection.
pub const WRITER_SOCKET_PATH: &str = "/tmp/netdata-ledger-writer.sock";

/// Requests sent from the ledger to the cleaner.
#[derive(Debug, Clone)]
pub enum CleanerRequest {
    /// Delete a WAL file that has been successfully indexed.
    DeleteWalFile { sequence: u64, path: PathBuf },
    /// Delete an index file evicted by retention policy.
    DeleteIndexFile { sequence: u64, path: PathBuf },
}

/// Responses sent from the cleaner back to the ledger.
#[derive(Debug, Clone)]
pub enum CleanerResponse {
    /// A WAL file has been deleted.
    WalFileDeleted { sequence: u64 },
    /// An index file has been deleted.
    IndexFileDeleted { sequence: u64 },
    /// Failed to delete a WAL file.
    WalFileFailed { sequence: u64, error: String },
    /// Failed to delete an index file.
    IndexFileFailed { sequence: u64, error: String },
}

/// Requests sent from the ledger to the indexer.
#[derive(Debug, Clone)]
pub enum IndexerRequest {
    /// The file has been archived — finalize its index.
    FinalizeIndex {
        /// Path to the WAL .bin file.
        wal_path: PathBuf,
        /// Path where the .sfst index should be written.
        index_path: PathBuf,
    },
}

/// Responses sent from the indexer back to the ledger.
#[derive(Debug, Clone)]
pub enum IndexerResponse {
    /// The index for a file has been finalized successfully.
    IndexFinalized { seq: u64, path: PathBuf },
    /// Indexing failed for a file.
    IndexFailed { path: PathBuf, error: String },
}

/// Requests sent from the ledger to the uploader.
#[derive(Debug, Clone)]
pub enum UploaderRequest {
    /// Upload an index file to remote object storage.
    Upload {
        seq: u64,
        local_path: PathBuf,
        remote_key: String,
    },
}

/// Responses sent from the uploader back to the ledger.
#[derive(Debug, Clone)]
pub enum UploaderResponse {
    /// The file has been uploaded successfully.
    Uploaded { seq: u64, remote_key: String },
    /// Failed to upload the file.
    UploadFailed { seq: u64, error: String },
}

/// Accept a WAL event connection from the ingestor on the given socket path.
pub async fn accept_writer(
    socket_path: &str,
) -> Result<Connection<(), wal::format::WalMessage>, ferryboat::Error> {
    let _ = std::fs::remove_file(socket_path);
    let endpoint = Endpoint::ipc(socket_path);
    let mut listener = Listener::<(), wal::format::WalMessage>::bind(endpoint).open()?;
    listener.accept().await
}
