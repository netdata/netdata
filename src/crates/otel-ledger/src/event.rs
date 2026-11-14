//! Event vocabulary for the ledger run-loop.
use otel_bridge::signals::Signal;
use otel_bridge::{LedgerRequest, LedgerResponse};

use file_lifecycle::ipc::{
    CatalogBuilderResponse, CleanerResponse, IndexerResponse, UploaderResponse,
};

/// One input to the ledger run-loop.
pub enum LedgerEvent {
    /// A WAL lifecycle message from the ingestor's writer connection. It
    /// reports that a WAL file was created, synced or closed
    /// (`wal::FileEvent`), and carries no payload.
    WalMsg(wal::Message),
    /// The ingestor closed its end of the writer link: it stopped, or it is
    /// gone.
    WriterLinkClosed,
    /// A response from a per-pipeline worker (indexer or catalog builder),
    /// tagged with the owning `Signal`.
    PipelineResp(Signal, PipelineResp),
    /// A response from the (shared) cleaner; carries its owning pipeline id.
    CleanerResp(CleanerResponse),
    /// A response from the (shared) uploader; carries its owning pipeline id.
    UploaderResp(UploaderResponse),
    /// A request from the supervisor.
    SupervisorReq(LedgerRequest),
    /// A response from a spawned task that needs to reach the supervisor.
    /// The run-loop exclusively owns `self.supervisor`, so spawned tasks
    /// funnel through this arm.
    OutboundResp(LedgerResponse),
    /// The ledger's periodic timer fired.
    PeriodicTick,
}

/// A response funneled from one pipeline's per-signal workers into the
/// run-loop's single merged channel.
///
/// The indexer and catalog builder are per-pipeline, so their channel set is
/// dynamic across N pipelines. A static `tokio::select!` cannot express
/// that. Each per-pipeline worker's response stream is forwarded into one
/// shared channel, tagged with the owning `Signal`, which the run-loop
/// selects on in [`crate::Ledger::run`]. The shared cleaner/uploader keep
/// their own channels because there is exactly one of each.
pub enum PipelineResp {
    /// A response from a pipeline's indexer.
    Indexer(IndexerResponse),
    /// A response from a pipeline's catalog builder.
    CatalogBuilder(CatalogBuilderResponse),
    /// A forwarded per-pipeline worker's response channel closed (its task
    /// ended).
    WorkerGone { kind: &'static str },
}

impl PipelineResp {
    /// Worker-kind labels for [`PipelineResp::WorkerGone`]. Shared so code
    /// that matches on a kind can't silently drift from the label the
    /// forwarder emits.
    pub const INDEXER_KIND: &'static str = "indexer";
    pub const CATALOG_BUILDER_KIND: &'static str = "catalog-builder";
}
