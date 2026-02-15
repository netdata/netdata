use bridge::LedgerRequest;

use crate::ipc::{CleanerResponse, IndexerResponse, UploaderResponse};

/// A unified event from any of the ledger's input sources.
pub enum LedgerEvent {
    /// A WAL message from the ingestor.
    WalMsg(wal::format::WalMessage),
    /// A response from the indexer subprocess.
    IndexerResp(IndexerResponse),
    /// A response from the cleaner subprocess.
    CleanerResp(CleanerResponse),
    /// A response from the uploader subprocess.
    UploaderResp(UploaderResponse),
    /// A request from the supervisor.
    SupervisorReq(LedgerRequest),
}
