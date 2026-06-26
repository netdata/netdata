use bridge::{LedgerRequest, LedgerResponse};

use file_lifecycle::ipc::{
    CatalogBuilderResponse, CleanerResponse, IndexerResponse, UploaderResponse,
};

/// A unified event from any of the ledger's input sources.
pub enum LedgerEvent {
    /// A WAL message from the ingestor (routed to a pipeline by
    /// `event.file_id.pipeline_id`).
    WalMsg(wal::Message),
    /// A response from a per-pipeline worker (indexer or catalog builder),
    /// tagged with the owning pipeline id by its forwarder task. The shared
    /// workers (cleaner, uploader) have their own arms because their responses
    /// carry the pipeline id inline.
    PipelineResp(u16, PipelineResp),
    /// A response from the (shared) cleaner; carries its owning pipeline id.
    CleanerResp(CleanerResponse),
    /// A response from the (shared) uploader; carries its owning pipeline id.
    UploaderResp(UploaderResponse),
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

/// A response funneled from one pipeline's per-signal workers into the
/// run-loop's single merged channel.
///
/// The indexer and catalog builder are per-pipeline, so their channel set is
/// dynamic across N pipelines — a thing a static `tokio::select!` cannot
/// express. Each per-pipeline worker's response stream is forwarded into one
/// shared channel, tagged with the owning `pipeline_id`, which the run-loop
/// selects on. The shared cleaner/uploader keep their own channels because
/// there is exactly one of each.
pub enum PipelineResp {
    /// A response from a pipeline's seal/index worker.
    Indexer(IndexerResponse),
    /// A response from a pipeline's catalog builder.
    CatalogBuilder(CatalogBuilderResponse),
    /// A forwarded per-pipeline worker's response channel closed (its task
    /// ended). Treated as fatal by the run-loop, matching the pre-carve
    /// behavior where a dead worker tore down the ledger so the supervisor
    /// restarts it.
    WorkerGone { kind: &'static str },
}
