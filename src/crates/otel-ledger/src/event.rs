//! Event vocabulary for the ledger run-loop. `LedgerEvent` is everything the
//! single `tokio::select!` in [`crate::Ledger::run`] wakes on; its dispatch
//! match in the same function routes each variant to a `handle_*` method.
//! `PipelineResp` is the per-pipeline worker response `LedgerEvent` carries.
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
    /// (`wal/src/format.rs` `FileEvent`). Routed to its pipeline by the
    /// event's `pipeline_id()`, decoded to a `Signal` in
    /// `Ledger::handle_ingestor_msg` (unknown ids dropped). A failed recv
    /// here is fatal — the writer connection is never re-established — so
    /// the arm `?`s [`crate::Ledger::run`] out.
    WalMsg(wal::Message),
    /// A response from a per-pipeline worker (indexer or catalog builder),
    /// tagged with the owning `Signal` by its `spawn_forwarder` task
    /// (`ledger/pipeline.rs`). The shared workers (cleaner, uploader)
    /// have their own arms because their responses carry the pipeline id
    /// inline (`file-lifecycle/src/ipc.rs` `CleanerResponse` /
    /// `UploaderResponse`).
    PipelineResp(Signal, PipelineResp),
    /// A response from the (shared) cleaner; carries its owning pipeline id.
    CleanerResp(CleanerResponse),
    /// A response from the (shared) uploader; carries its owning pipeline id.
    UploaderResp(UploaderResponse),
    /// A request from the supervisor.
    SupervisorReq(LedgerRequest),
    /// A response produced by a spawned function-handler task or its progress
    /// bridge (`Ledger::dispatch_function_call` / `spawn_progress_bridge`,
    /// `ledger/rpc/dispatch.rs`) that needs to reach the supervisor. The
    /// run-loop exclusively owns `self.supervisor`, so handlers funnel
    /// through this arm.
    OutboundResp(LedgerResponse),
    /// The shared upload-retry timer fired (the 30 s `retry_timer` interval
    /// built in `Ledger::new`): re-issue any failed uploads whose backoff has
    /// elapsed (`Ledger::handle_retry_tick`).
    RetryTick,
}

/// A response funneled from one pipeline's per-signal workers into the
/// run-loop's single merged channel.
///
/// The indexer and catalog builder are per-pipeline, so their channel set is
/// dynamic across N pipelines — a thing a static `tokio::select!` cannot
/// express. Each per-pipeline worker's response stream is forwarded into one
/// shared channel, tagged with the owning `Signal`, which the run-loop
/// selects on in [`crate::Ledger::run`]. The shared cleaner/uploader keep
/// their own channels because there is exactly one of each.
pub enum PipelineResp {
    /// A response from a pipeline's seal/index worker.
    Indexer(IndexerResponse),
    /// A response from a pipeline's catalog builder.
    CatalogBuilder(CatalogBuilderResponse),
    /// A forwarded per-pipeline worker's response channel closed (its task
    /// ended). Treated as fatal: the run-loop logs and exits, and the agent
    /// restarts the whole plugin — the supervisor deliberately never
    /// restarts workers (`otel-plugin/src/supervisor.rs` `Supervisor::run`).
    WorkerGone { kind: &'static str },
}

impl PipelineResp {
    /// Worker-kind labels for [`PipelineResp::WorkerGone`], set by the
    /// per-pipeline forwarders (the `spawn_forwarder` call sites in
    /// `ledger/pipeline.rs`). Shared so a consumer that matches on a kind
    /// (the shutdown-flush drain compares against
    /// [`PipelineResp::CATALOG_BUILDER_KIND`] in
    /// `Ledger::flush_catalogs_on_shutdown`) can't silently drift from the
    /// label the forwarder emits.
    pub const INDEXER_KIND: &'static str = "indexer";
    pub const CATALOG_BUILDER_KIND: &'static str = "catalog-builder";
}
