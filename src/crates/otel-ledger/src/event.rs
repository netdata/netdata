use bridge::{LedgerRequest, LedgerResponse};

use crate::ipc::{CatalogBuilderResponse, CleanerResponse, IndexerResponse, UploaderResponse};

/// A unified event from any of the ledger's input sources.
pub enum LedgerEvent {
    /// A WAL message from the ingestor.
    WalMsg(wal::Message),
    /// A response from the indexer subprocess.
    IndexerResp(IndexerResponse),
    /// A response from the cleaner subprocess.
    CleanerResp(CleanerResponse),
    /// A response from the uploader subprocess.
    UploaderResp(UploaderResponse),
    /// A response from the catalog builder.
    CatalogBuilderResp(CatalogBuilderResponse),
    /// A request from the supervisor.
    SupervisorReq(LedgerRequest),
    /// A response produced by a spawned function-handler task that
    /// needs to be forwarded to the supervisor. The run-loop owns
    /// `self.supervisor`, so handlers funnel through this arm.
    OutboundResp(LedgerResponse),
    /// The upload-retry timer fired; re-issue any failed uploads whose
    /// backoff has elapsed.
    RetryTick,
}
