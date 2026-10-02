//! Event vocabulary for the ledger run-loop. `LedgerEvent` is everything the
//! single `tokio::select!` in `Ledger::run` wakes on (ledger/mod.rs:383-423);
//! its dispatch match routes each variant to a `handle_*` method
//! (ledger/mod.rs:427-461). `PipelineResp` is the per-pipeline worker
//! response `LedgerEvent` carries.
use bridge::signals::Signal;
use bridge::{LedgerRequest, LedgerResponse};

use file_lifecycle::ipc::{
    CatalogBuilderResponse, CleanerResponse, IndexerResponse, UploaderResponse,
};

/// One input to the ledger run-loop: one variant per `tokio::select!` arm in
/// `Ledger::run`.
pub enum LedgerEvent {
    /// A WAL lifecycle message from the ingestor's writer connection —
    /// notification that a WAL file was Created/Synced/Closed, not payload
    /// (wal/src/format.rs:251). Routed to its pipeline by the event's
    /// `pipeline_id()`, decoded to a `Signal` in `handle_ingestor_msg`
    /// (unknown ids dropped; ledger/ingestor.rs:16-23). A failed recv here
    /// is fatal — the writer connection is never re-established — so the
    /// arm `?`s the run-loop out (ledger/mod.rs:384-386).
    WalMsg(wal::Message),
    /// A response from a per-pipeline worker (indexer or catalog builder),
    /// tagged with the owning `Signal` by its forwarder task
    /// (ledger/pipeline.rs:415-436). The shared workers (cleaner, uploader)
    /// have their own arms because their responses carry the pipeline id
    /// inline (file-lifecycle/src/ipc.rs:44-48,140-144).
    PipelineResp(Signal, PipelineResp),
    /// A response from the (shared) cleaner; carries its owning pipeline id.
    CleanerResp(CleanerResponse),
    /// A response from the (shared) uploader; carries its owning pipeline id.
    UploaderResp(UploaderResponse),
    /// A request from the supervisor.
    SupervisorReq(LedgerRequest),
    /// A response produced by a spawned function-handler task or its progress
    /// bridge (ledger/rpc/dispatch.rs:143,161-164) that needs to reach the
    /// supervisor. The run-loop exclusively owns `self.supervisor`, so
    /// handlers funnel through this arm.
    OutboundResp(LedgerResponse),
    /// The shared upload-retry timer fired (every 30 s; ledger/mod.rs:352):
    /// re-issue any failed uploads whose backoff has elapsed
    /// (`handle_retry_tick`, ledger/uploader.rs:199).
    RetryTick,
}

/// A response funneled from one pipeline's per-signal workers into the
/// run-loop's single merged channel.
///
/// The indexer and catalog builder are per-pipeline, so their channel set is
/// dynamic across N pipelines — a thing a static `tokio::select!` cannot
/// express. Each per-pipeline worker's response stream is forwarded into one
/// shared channel, tagged with the owning `Signal`, which the run-loop
/// selects on (ledger/mod.rs:389). The shared cleaner/uploader keep their own
/// channels because there is exactly one of each.
pub enum PipelineResp {
    /// A response from a pipeline's seal/index worker.
    Indexer(IndexerResponse),
    /// A response from a pipeline's catalog builder.
    CatalogBuilder(CatalogBuilderResponse),
    /// A forwarded per-pipeline worker's response channel closed (its task
    /// ended). Treated as fatal: the run-loop logs and exits, and the agent
    /// restarts the whole plugin — the supervisor deliberately never
    /// restarts workers (otel-plugin/src/supervisor.rs:521-522).
    WorkerGone { kind: &'static str },
}

impl PipelineResp {
    /// Worker-kind labels for [`PipelineResp::WorkerGone`], set by the
    /// per-pipeline forwarders (the `spawn_forwarder` call sites,
    /// ledger/pipeline.rs:328-341). Shared so a consumer that matches on a
    /// kind (the shutdown-flush drain checks the catalog-builder kind;
    /// ledger/catalog_builder.rs:172) can't silently drift from the label
    /// the forwarder emits.
    pub const INDEXER_KIND: &'static str = "indexer";
    pub const CATALOG_BUILDER_KIND: &'static str = "catalog-builder";
}
