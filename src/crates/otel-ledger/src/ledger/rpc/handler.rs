//! `OtelLogsHandler` — typed `FunctionHandler` implementation.
//!
//! A thin adapter over the [`sfsq::logs`] query engine. Holds a shared,
//! read-only handle to the tenant registries: the run-loop's mutators
//! take brief write locks; this handler takes a read lock just long
//! enough to enumerate the SFST candidates whose time range overlaps the
//! request window, then drops it and runs the (sync) query off the
//! runtime thread via `spawn_blocking`.
//!
//! The engine is wire-neutral: it consumes a [`sfsq::logs::LogsQuery`]
//! and produces a [`sfsq::logs::LogsData`]. The netdata function wire
//! shape — the request/response types and the response envelope — lives
//! in [`super::wire`], and the mapping to and from the engine in
//! [`super::adapter`]. What stays here is the netdata-plugin glue: the
//! `FunctionHandler` impl, the capability declaration, the rt-level
//! args→payload shim, and the lock/scheduling dance.

use std::sync::Arc;

use async_trait::async_trait;
use bridge::function::{FunctionCallContext, FunctionHandler};
use file_registry::TenantId;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_types::HttpAccess;
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;

use sfsq::logs::{LogSource, SfstCandidate, Source, WalTail, run};

use super::adapter::{to_result, window_secs};
use super::wire::{InfoResponse, LogsResult, OtelLogsRequest, OtelLogsResponse};
use crate::chunk::ChunkCache;
use wal::prefix::{chunk_boundaries, tail_start};
use crate::registry::{TenantRegistries, WalDesc};

pub(crate) struct OtelLogsHandler {
    registries: Arc<RwLock<TenantRegistries>>,
    /// Shared with the ledger (which drops a WAL's chunks on rotation).
    chunk_cache: Arc<ChunkCache>,
    /// Minimum records per chunk when indexing an active WAL's prefix.
    min_entries: u64,
}

impl OtelLogsHandler {
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        min_entries: u64,
    ) -> Self {
        Self {
            registries,
            chunk_cache,
            min_entries,
        }
    }

    /// Resolve one active-WAL descriptor into in-memory chunk SFST
    /// candidates plus the WAL-tail byte ranges to row-scan. Off the
    /// registry lock. Polls `cancel` between chunk builds (each build is
    /// one `spawn_blocking`); a cancelled call returns empty — the
    /// caller is about to discard the result anyway.
    ///
    /// Scans the durable prefix's frame headers, groups them into chunks
    /// at `min_entries`, and builds each through the cache (singleflight).
    /// A chunk that fails to build or parse makes the **whole WAL**
    /// un-queryable for this query: it returns no candidates and no tails,
    /// so none of the WAL's data is served. That data reappears once the
    /// WAL rotates into a sealed SFST — or, for a transient failure (e.g. a
    /// count mismatch on an actively-written WAL), on a later query, since
    /// build errors are not cached. The same empty result covers a WAL that
    /// can't be read at all (rotated/deleted under us).
    ///
    /// Refusing the whole WAL keeps **at most one tail per `file_seq`** (the
    /// trailing un-chunked suffix). The pagination cursor routes tails by
    /// `(file_seq, Part::Tail)`, which assumes one tail per WAL — covering a
    /// failed chunk with its own extra tail would break that (C-5 in
    /// docs/sfsq-readability-refactors.md).
    async fn resolve_wal(
        &self,
        wal: WalDesc,
        cancel: &CancellationToken,
    ) -> (Vec<SfstCandidate>, Vec<WalTail>) {
        let header = wal::HEADER_SIZE as u64;
        let scan_path = wal.path.clone();
        let valid_up_to = wal.valid_up_to;
        let frames = match tokio::task::spawn_blocking(move || {
            wal::scan_frame_boundaries(&scan_path, wal::FrameRange::new(header, valid_up_to))
        })
        .await
        {
            Ok(Ok(frames)) => frames,
            Ok(Err(e)) => {
                tracing::warn!(seq = wal.seq, "WAL boundary scan failed: {e}");
                return (Vec::new(), Vec::new());
            }
            Err(e) => {
                tracing::warn!(seq = wal.seq, "WAL boundary scan task failed: {e}");
                return (Vec::new(), Vec::new());
            }
        };

        let chunks = chunk_boundaries(&frames, header, self.min_entries);
        let mut candidates = Vec::new();
        for chunk in &chunks {
            if cancel.is_cancelled() {
                return (Vec::new(), Vec::new());
            }
            let seq = wal.seq;
            let path = wal.path.clone();
            let (range, expected) = (chunk.range, chunk.entry_count);
            // The build future: index the byte range on a blocking
            // thread and cross-check the record count (the truncation
            // check open_range defers). Runs once per (seq, index) under
            // singleflight; skipped entirely on a cache hit.
            let init = async move {
                match tokio::task::spawn_blocking(move || sfst_indexer::index_range(&path, range)).await {
                    Ok(Ok((summary, bytes))) => {
                        if u64::from(summary.total_logs) != expected {
                            Err(format!(
                                "chunk record count {} != expected {expected}",
                                summary.total_logs
                            ))
                        } else {
                            Ok(Arc::new(bytes))
                        }
                    }
                    Ok(Err(e)) => Err(format!("index_range: {e}")),
                    Err(e) => Err(format!("build task: {e}")),
                }
            };

            match self.chunk_cache.get_or_build(seq, chunk.index, init).await {
                Ok(bytes) => match sfst::IndexReader::open(&bytes[..]) {
                    Ok(reader) => candidates.push(SfstCandidate {
                        summary: reader.summary().clone(),
                        file_seq: seq,
                        // Distinguishes this chunk from the WAL's other
                        // chunks and tail, which share `seq`.
                        part: sfsq::logs::Part::Indexed(chunk.index),
                        source: Source::Memory(bytes),
                    }),
                    // Parsed-back failure is unexpected. Refuse the whole
                    // WAL this query rather than serve it partially.
                    Err(e) => {
                        tracing::warn!(
                            seq,
                            index = chunk.index,
                            "chunk parse failed; refusing to query this WAL: {e}"
                        );
                        return (Vec::new(), Vec::new());
                    }
                },
                // Build failed (decode error, count mismatch, panic).
                // Refuse the whole WAL this query; its data returns via the
                // sealed SFST after rotation (or next query for a transient
                // failure — build errors aren't cached).
                Err(e) => {
                    tracing::warn!(
                        seq,
                        index = chunk.index,
                        "chunk build failed; refusing to query this WAL: {e}"
                    );
                    return (Vec::new(), Vec::new());
                }
            }
        }

        // Every chunk indexed cleanly. The final tail after the last
        // complete chunk is the only row-scanned range (skip it when empty —
        // the prefix divided evenly into chunks).
        let mut tails = Vec::new();
        let tail_begin = tail_start(&chunks, header);
        if tail_begin < wal.valid_up_to {
            tails.push(WalTail {
                file_seq: wal.seq,
                path: wal.path.clone(),
                range: wal::FrameRange::new(tail_begin, wal.valid_up_to),
            });
        }
        (candidates, tails)
    }
}

#[async_trait]
impl FunctionHandler for OtelLogsHandler {
    type Request = OtelLogsRequest;
    type Response = OtelLogsResponse;

    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        req: Self::Request,
    ) -> netdata_plugin_error::Result<Self::Response> {
        if req.info {
            return Ok(OtelLogsResponse::Info(InfoResponse::default()));
        }

        // Canonicalize the wire request into the neutral query (defaulting
        // + bucket alignment + grid), then enumerate the SFST candidates
        // overlapping the grid's window under a brief read lock — dropped
        // before any I/O.
        let last = req.last;
        let tenant = resolve_query_tenant(req.tenant.as_deref());
        // A malformed free-text `query` regex is a clean request error.
        let query = req.into_query().map_err(|e| {
            netdata_plugin_error::NetdataPluginError::FunctionHandler {
                message: format!("invalid query: {e}"),
            }
        })?;
        let time_range = window_secs(&query.grid());
        // Snapshot the candidate set under a brief read lock: on-disk
        // SFSTs plus the unindexed WALs overlapping the window, owned so
        // the lock drops before any I/O. `valid_up_to` is captured here,
        // once — every chunk and tail derives from this single value, so
        // the whole query sees one consistent durable prefix even as
        // ingestion advances it.
        let (mut sfst_candidates, wal_descs) = {
            let guard = self.registries.read().await;
            let q = file_registry::Query {
                time_range: time_range.clone(),
                stream: None,
            };
            guard.query_snapshot(&tenant, &q)
        };

        // Resolve each WAL into in-memory chunk SFSTs + a tail (off the
        // lock; chunk builds are singleflighted through the cache). The
        // chunk-building phase can be the slow one, so it polls the
        // call's cancellation token between builds.
        let mut wal_tails: Vec<WalTail> = Vec::new();
        for wal in wal_descs {
            let (chunks, tails) = self.resolve_wal(wal, &ctx.cancellation).await;
            sfst_candidates.extend(chunks);
            wal_tails.extend(tails);
        }

        // One mixed source list for the engine: indexed SFSTs (sealed +
        // in-memory chunks) and the row-scanned WAL tails. Order is
        // cosmetic — `run` re-partitions by kind and the stats merge is an
        // order-independent monoid, so it sorts purely by the cursor order.
        let sources: Vec<LogSource> = sfst_candidates
            .into_iter()
            .map(LogSource::Sfst)
            .chain(wal_tails.into_iter().map(LogSource::Tail))
            .collect();

        let (after, before) = (time_range.start, time_range.end);
        if sources.is_empty() {
            return Ok(OtelLogsResponse::Logs(LogsResult::empty_stub(
                after, before, last,
            )));
        }

        // The query is synchronous and CPU/IO-bound (opens + decompresses
        // SFSTs, row-scans the tails); run it and shape the neutral
        // result into the wire envelope off the runtime thread.
        //
        // Progress: total = number of query sources; the engine bumps the
        // done counter as each source's stats shard completes and the
        // bridge's 1 Hz ticker emits FUNCTION_PROGRESS lines from it.
        // Cancellation is cooperative — a `spawn_blocking` closure cannot
        // be aborted, so the engine polls the token per source and bails
        // early; the bridge's cancel `select!` already returns the 499 to
        // the caller and discards this partial result.
        ctx.progress.set_total(sources.len());
        let cancel = ctx.cancellation.clone();
        let done = ctx.progress.done_counter();
        let result = match tokio::task::spawn_blocking(move || {
            to_result(run(sources, query, cancel, done), last)
        })
        .await
        {
            Ok(result) => result,
            Err(e) => {
                tracing::warn!("otel-logs blocking task failed: {e}");
                LogsResult::empty_stub(after, before, last)
            }
        };

        Ok(OtelLogsResponse::Logs(result))
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut d = FunctionDeclaration::new("otel-logs", "Query OpenTelemetry logs");
        d.global = true;
        d.tags = Some("logs".to_string());
        d.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        d
    }
}

/// Resolve the request's tenant selector to the registry key the query
/// reads — the permissive query-side policy on the type itself
/// ([`TenantId::resolve_query`]): omitted/invalid falls back to the
/// default tenant, never an implicit all-tenant union, and the literal
/// `default` stays nameable (unlike ingest's strict validation).
fn resolve_query_tenant(raw: Option<&str>) -> TenantId {
    TenantId::resolve_query(raw)
}

/// Replicate the rt-level GET shim (`netdata-plugin/rt/src/lib.rs`):
/// when args carry `after:N` / `before:N` tokens, synthesize a JSON
/// payload with the parsed window plus an `info` flag determined by
/// whether the literal `info` token is in the args. Returns `None`
/// when no synthesis happened (no args, or the upstream rt shim
/// already produced a payload), in which case the caller falls back
/// to the original payload.
pub(super) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        if let Some(rest) = arg.strip_prefix("after:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("after".into(), serde_json::json!(v));
            }
        } else if let Some(rest) = arg.strip_prefix("before:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("before".into(), serde_json::json!(v));
            }
        }
    }

    serde_json::to_vec(&serde_json::Value::Object(map)).ok()
}

#[cfg(test)]
mod tests;
