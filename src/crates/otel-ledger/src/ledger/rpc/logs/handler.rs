//! `OtelLogsHandler` — the typed `FunctionHandler` for the `otel-logs`
//! Function.
//!
//! A thin adapter over the wire-neutral [`sfsq::logs`] engine (a
//! [`sfsq::logs::LogsQuery`] in, a [`sfsq::logs::LogsData`] out): the
//! netdata wire shapes live in [`super::wire`], the mapping in
//! [`super::adapter`], and what stays here is the plugin glue — the
//! `FunctionHandler` impl, the capability declaration, and the
//! lock/scheduling discipline. [`OtelLogsHandler::on_call`] runs one
//! request flow: canonicalize the request, snapshot the overlapping
//! sources (sealed SFSTs + active WALs) under a brief registry read lock
//! dropped before any I/O, resolve WALs into in-memory chunks + tails,
//! fetch remote-only SFSTs, then run the sync query via `spawn_blocking`.
//! The registries are shared with the run loop's mutators (brief write
//! locks); this handler only reads. The LOGS-only GET args→payload shim
//! lives in the parent `rpc` module; traces installs its own.

use std::sync::Arc;

use async_trait::async_trait;
use bridge::function::{FunctionCallContext, FunctionHandler};
use file_registry::TenantId;
use netdata_plugin_protocol::FunctionDeclaration;
use netdata_plugin_types::HttpAccess;
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;

use file_lifecycle::remote_read::RemoteRead;
use sfsq::logs::{LogSource, LogsData, SfstCandidate, Source, WalTail, run};

use super::adapter::{stream_required_params, to_result, window_secs};
use super::wire::{
    CatalogFileEntry, FilesResponse, InfoResponse, OtelLogsRequest, OtelLogsResponse,
    SfstFileEntry, StreamId, TenantFiles, WalFileEntry,
};
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::{TenantRegistries, WalDesc};
use wal::prefix::{chunk_boundaries, tail_start};
use wal::registry::FileStatus;

/// Decode a file's opaque `content_meta` blob into the wire `StreamId`
/// shown by the `files: true` inventory. Falls back to empty namespace/name
/// when the blob is absent or unparseable, so a corrupt entry still lists.
fn stream_id_from_content_meta(content_meta: &[u8]) -> StreamId {
    let stream = otel_logs_identity::decode_content_meta_or_empty(content_meta);
    StreamId {
        namespace: stream.namespace,
        name: stream.name,
    }
}

/// Build the `files: true` inventory snapshot from a read-locked registry set.
/// Read-only; tenants and per-kind files are sorted for stable output — the
/// tenants map is a HashMap (arbitrary iteration order), and the per-kind
/// lists pin their order explicitly rather than trusting the file
/// registries' current iteration order.
fn build_files_response(tr: &TenantRegistries) -> FilesResponse {
    let mut tenants: Vec<TenantFiles> = tr
        .tenants
        .iter()
        .map(|(tid, reg)| {
            let mut wal: Vec<WalFileEntry> = reg
                .wal
                .values()
                .map(|f| WalFileEntry {
                    seq: f.id.seq,
                    ns_hash: format!("{:016x}", f.id.part_key),
                    stream: stream_id_from_content_meta(&f.content_meta),
                    status: match f.status {
                        FileStatus::Active => "active",
                        FileStatus::Archived => "archived",
                    },
                    size: f.size.as_u64(),
                    entry_count: f.entry_count,
                    min_ts_ns: f.min_timestamp_ns.as_u64(),
                    max_ts_ns: f.max_timestamp_ns.as_u64(),
                })
                .collect();
            wal.sort_by_key(|e| e.seq);

            let mut sfst: Vec<SfstFileEntry> = reg
                .sfst
                .values()
                .map(|f| {
                    let key = file_registry::SeqKey::from(&f.id);
                    SfstFileEntry {
                        seq: f.id.seq,
                        ns_hash: format!("{:016x}", f.id.part_key),
                        stream: stream_id_from_content_meta(&f.summary.content_meta),
                        size: f.size.as_u64(),
                        total_logs: f.summary.record_count,
                        min_ts_s: f.summary.min_timestamp_s,
                        max_ts_s: f.summary.max_timestamp_s,
                        rotated: reg.is_rotated(key),
                        uploaded: reg.is_uploaded(key),
                        remote_cataloged: reg.is_remote_cataloged(key),
                        pending_deletion: f.is_pending_deletion(),
                    }
                })
                .collect();
            sfst.sort_by_key(|e| e.seq);

            let mut catalog: Vec<CatalogFileEntry> = reg
                .catalog_files
                .iter()
                .map(|(path, f)| CatalogFileEntry {
                    file: path
                        .file_name()
                        .and_then(|n| n.to_str())
                        .unwrap_or_default()
                        .to_string(),
                    date: f.date.to_string(),
                    max_seq: f.max_seq,
                    size: f.size.as_u64(),
                    min_ts_s: f.min_timestamp_s,
                    max_ts_s: f.max_timestamp_s,
                    pending_deletion: f.is_pending_deletion(),
                })
                .collect();
            catalog.sort_by(|a, b| a.file.cmp(&b.file));

            TenantFiles {
                tenant: tid.as_str().to_string(),
                wal,
                sfst,
                catalog,
            }
        })
        .collect();
    tenants.sort_by(|a, b| a.tenant.cmp(&b.tenant));
    FilesResponse {
        version: 1,
        status: 200,
        tenants,
    }
}

pub(crate) struct OtelLogsHandler {
    registries: Arc<RwLock<TenantRegistries>>,
    /// Shared with the ledger (which drops a WAL's chunks on rotation).
    chunk_cache: Arc<ChunkCache>,
    /// Minimum records per chunk when indexing an active WAL's prefix.
    min_entries: u64,
    /// Remote-read capability; `None` when remote storage is disabled.
    remote: Option<RemoteRead>,
}

impl OtelLogsHandler {
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        min_entries: u64,
        remote: Option<RemoteRead>,
    ) -> Self {
        Self {
            registries,
            chunk_cache,
            min_entries,
            remote,
        }
    }

    /// Resolve one active-WAL descriptor into in-memory chunk SFST
    /// candidates plus the byte range to row-scan as its tail. Runs off
    /// the registry lock, over the descriptor's `valid_up_to` durable
    /// bound. Polls `cancel` between chunk builds (each build is one
    /// `spawn_blocking`); a cancelled call returns empty — the caller is
    /// about to discard the result anyway.
    ///
    /// Scans the durable prefix's frame boundaries, groups them into
    /// chunks of at least `min_entries` records, and builds each through
    /// the chunk cache (singleflight per `(seq, index)`). Failure policy:
    /// a chunk that fails to build or parse makes the **whole WAL**
    /// un-queryable for this query — no candidates and no tails — rather
    /// than silently dropping only the broken range. The WAL's data
    /// reappears once it rotates into a sealed SFST — or, for a transient
    /// failure (e.g. a count mismatch on an actively-written WAL), on a
    /// later query, since build errors are not cached. The same empty
    /// result covers a WAL that can't be read at all (rotated/deleted
    /// under us).
    ///
    /// Refusing the whole WAL also keeps the **at most one tail per
    /// `file_seq`** invariant the engine assumes: the pagination cursor
    /// addresses a WAL's tail as `(file_seq, Part::Tail)`, so a second
    /// tail under one WAL would double-count the stats pass and misroute
    /// rows (the engine drops duplicate tails loudly; `sfsq::logs::run`).
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
                tracing::warn!(seq = wal.id.seq, "WAL boundary scan failed: {e}");
                return (Vec::new(), Vec::new());
            }
            Err(e) => {
                tracing::warn!(seq = wal.id.seq, "WAL boundary scan task failed: {e}");
                return (Vec::new(), Vec::new());
            }
        };

        let chunks = chunk_boundaries(&frames, header, self.min_entries);
        let mut candidates = Vec::new();
        for chunk in &chunks {
            if cancel.is_cancelled() {
                return (Vec::new(), Vec::new());
            }
            let seq = wal.id.seq;
            let path = wal.path.clone();
            let (range, expected) = (chunk.range, chunk.entry_count);
            // The build future: index the byte range on a blocking
            // thread and cross-check the record count (the truncation
            // check wal::Reader::open_range defers). Runs at most once
            // per (seq, index) — singleflight; skipped on a cache hit.
            let init = async move {
                match tokio::task::spawn_blocking(move || ng_index::build_sfst_range(&path, range))
                    .await
                {
                    Ok(Ok((summary, bytes))) => {
                        if u64::from(summary.record_count) != expected {
                            Err(format!(
                                "chunk record count {} != expected {expected}",
                                summary.record_count
                            ))
                        } else {
                            Ok(Arc::new(bytes))
                        }
                    }
                    Ok(Err(e)) => Err(format!("build_sfst_range: {e}")),
                    Err(e) => Err(format!("build task: {e}")),
                }
            };

            match self.chunk_cache.get_or_build(seq, chunk.index, init).await {
                Ok(bytes) => match sfst::IndexReader::open(&bytes[..]) {
                    Ok(reader) => candidates.push(SfstCandidate {
                        summary: reader.summary().clone(),
                        file_seq: seq,
                        // Chunks and the tail of one WAL share `seq`;
                        // the chunk index disambiguates them in the
                        // cursor order.
                        part: sfsq::logs::Part::Indexed(chunk.index),
                        source: Source::Memory(bytes),
                    }),
                    // Unexpected — the bytes were just built. Same
                    // policy: refuse the whole WAL (see above).
                    Err(e) => {
                        tracing::warn!(
                            seq,
                            index = chunk.index,
                            "chunk parse failed; refusing to query this WAL: {e}"
                        );
                        return (Vec::new(), Vec::new());
                    }
                },
                // Build failure (decode error, count mismatch, panic):
                // refuse the whole WAL — see the failure policy above.
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

        // Every chunk indexed cleanly. The records after the last
        // complete chunk are the WAL's one row-scanned tail (skipped when
        // empty — the prefix divided evenly into chunks).
        let mut tails = Vec::new();
        let tail_begin = tail_start(&chunks, header);
        if tail_begin < wal.valid_up_to {
            tails.push(WalTail {
                file_seq: wal.id.seq,
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
        mut req: Self::Request,
    ) -> netdata_plugin_error::Result<Self::Response> {
        if req.info {
            return Ok(OtelLogsResponse::Info(InfoResponse::default()));
        }

        // Inventory snapshot of the tracked storage files (read-only): a brief
        // read lock, build the response, drop it before returning. No query.
        if req.files {
            let guard = self.registries.read().await;
            return Ok(OtelLogsResponse::Files(build_files_response(&guard)));
        }

        // Canonicalize the wire request into the neutral query (defaulting
        // + bucket alignment + grid); the source snapshot below runs under
        // a brief read lock, dropped before any I/O.
        let last = req.last;
        let tenant = resolve_query_tenant(req.tenant.as_deref());
        // Pull the reserved stream-selector picks out of `selections`
        // before `into_query`, so the engine never row-filters on the
        // synthetic `__streams` facet; they drive file pruning instead.
        let partition_keys = req.take_partition_keys();
        // A malformed free-text `query` regex is a clean request error.
        let query = req.into_query().map_err(|e| {
            netdata_plugin_error::NetdataPluginError::FunctionHandler {
                message: format!("invalid query: {e}"),
            }
        })?;
        let time_range = window_secs(&query.grid());
        // The selector lists every stream in the window, independent of the
        // user's current pick, so it uses a time-only query (empty
        // partition_keys); `q` carries the user's filter for the data query.
        let stream_q = file_registry::Query {
            time_range: time_range.clone(),
            partition_keys: Vec::new(),
        };
        let q = file_registry::Query {
            time_range,
            partition_keys,
        };
        // Snapshot under a brief read lock, in memory only: on-disk SFSTs
        // plus the unindexed WALs (durable prefix only — a WAL without a
        // trustworthy `valid_up_to` bound is never served) overlapping the
        // window, the local half of the stream selector, and (with remote
        // storage) which catalog files to read and what each range already
        // serves locally. All owned, so the lock drops before any file is
        // read — catalogs included. `valid_up_to` is captured here, once —
        // every chunk and tail derives from this single value, so the whole
        // query sees one consistent durable prefix even as ingestion
        // advances it.
        let (mut sfst_candidates, wal_descs, local_streams, remote_input) = {
            let guard = self.registries.read().await;
            let (sfsts, wals) = guard.query_snapshot(&tenant, &q);
            let local_streams = guard.local_streams(&tenant, &stream_q);
            // Plan remote reads only when remote storage is configured —
            // planning anyway would put remote-only (evicted) streams in
            // the selector that the fetch below can never download.
            // Range 0 is the selector's window, range 1 the user's fetch.
            let remote_input = self
                .remote
                .is_some()
                .then(|| guard.remote_plan_input(&tenant, &[stream_q, q]));
            // Convert the signal-neutral selected files into the logs engine's
            // sealed-SFST candidates at this (logs-specific) boundary.
            let sfst_candidates: Vec<SfstCandidate> =
                sfsts.into_iter().map(SfstCandidate::from).collect();
            (sfst_candidates, wals, local_streams, remote_input)
        };

        // Read the catalogs off the lock (blocking file I/O) and finish the
        // selector with the remote-only streams. A catalog that cannot be read
        // is logged and skipped: the logs wire has no partial status. The
        // logs plan is unbounded in size, so a too-large query is refused
        // later by the download cache, mapped to its own message below.
        // A failed planning task is an error: answering without the remote
        // data and the remote-only streams would be a silent gap.
        let (selector_catalog, remote_cands) = match remote_input {
            Some(input) => match tokio::task::spawn_blocking(move || input.plan()).await {
                Ok(plan) => {
                    let [selector, fetch]: [Vec<otel_catalog::CatalogEntry>; 2] = plan
                        .per_range
                        .try_into()
                        .expect("a plan has one range per query");
                    (selector, fetch)
                }
                Err(e) => {
                    return Err(netdata_plugin_error::NetdataPluginError::FunctionHandler {
                        message: format!("otel-logs remote planning task failed: {e}"),
                    });
                }
            },
            None => (Vec::new(), Vec::new()),
        };
        let required_params = stream_required_params(local_streams.with_catalog(&selector_catalog));

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

        // Progress spans two phases: the remote fetch (one unit per planned
        // download; cache hits don't tick) then the engine scan (one unit per
        // source). `total` is set upfront — `remote_count` downloads + the
        // eventual scan-source count (local sources plus the fetched remotes)
        // — so the slow, network-bound fetch phase advances a real bar
        // instead of the indeterminate 1%. `total` is an upper bound (cache
        // hits and failed downloads leave `done` finishing just under it);
        // the bar caps at 99% regardless and completion is signaled by the
        // RESULT, so that is benign.
        let remote_count = remote_cands.len();
        let local_scan = sfst_candidates.len() + wal_tails.len();
        ctx.progress.set_total(local_scan + 2 * remote_count);
        let done = ctx.progress.done_counter();

        // Fetch any remote-only SFSTs (evicted locally) back through the
        // download cache and add them as sources. The returned pins move into
        // the blocking query run below, so the files stay in the cache until the
        // engine is done with them, even if this call is cancelled meanwhile.
        // Files that could not be downloaded are omitted (the logs wire has no
        // partial status); query-wide failures surface as actionable errors.
        let remote_pins: Vec<file_cache::CachedFile> = if let Some(remote) = &self.remote
            && !remote_cands.is_empty()
        {
            match remote
                .fetch(remote_cands, &ctx.cancellation, done.clone())
                .await
            {
                Ok(fetched) => {
                    // Convert the neutral selected files to engine candidates at
                    // the boundary, same as the local sealed ones.
                    sfst_candidates.extend(fetched.files.into_iter().map(SfstCandidate::from));
                    fetched.pins
                }
                Err(file_cache::CacheError::Cancelled) => {
                    // The bridge usually discards a cancelled call's
                    // response, but the cancel `select!` can race and
                    // deliver it — so it must be the same well-formed
                    // empty envelope as every other path, selector included.
                    let mut result = to_result(
                        LogsData::empty(query.histogram_field(), query.grid()),
                        last,
                    );
                    result.required_params = required_params;
                    return Ok(OtelLogsResponse::Logs(result));
                }
                Err(file_cache::CacheError::TooLarge {
                    footprint,
                    capacity,
                }) => {
                    return Err(netdata_plugin_error::NetdataPluginError::FunctionHandler {
                        message: format!(
                            "this query needs {footprint} bytes of remote log data, more than the \
                             read cache can hold ({capacity}); narrow the time window or stream filter"
                        ),
                    });
                }
                Err(file_cache::CacheError::EvictionFailed) => {
                    return Err(netdata_plugin_error::NetdataPluginError::FunctionHandler {
                        message: "remote-read cache directory is unwritable (eviction failed); \
                                  check its permissions and free space"
                            .to_string(),
                    });
                }
            }
        } else {
            Vec::new()
        };

        // One mixed source list for the engine: indexed SFSTs (sealed +
        // in-memory chunks) and the row-scanned WAL tails. Input order is
        // cosmetic — `run` evaluates each source by its variant and the
        // stats merge is an order-independent monoid, so rows sort purely
        // by the cursor order.
        let sources: Vec<LogSource> = sfst_candidates
            .into_iter()
            .map(LogSource::Sfst)
            .chain(wal_tails.into_iter().map(LogSource::Tail))
            .collect();

        // No `sources.is_empty()` special case: the engine's run over an
        // empty source set is I/O-free and yields the grid-aligned empty
        // `LogsData`, which `to_result` shapes into the same well-formed
        // envelope as a zero-match query — a full grid of zero-count
        // histogram buckets and the fixed table columns, which is what
        // the consuming UI needs to render an empty window sanely.
        //
        // The query is synchronous and CPU/IO-bound (opens + decompresses
        // SFSTs, row-scans the tails); run it and shape the neutral
        // result into the wire envelope off the runtime thread.
        //
        // Progress: `total` and the `done` counter were set up before the fetch
        // (above); the engine continues bumping `done` as each source's stats
        // shard completes, and the bridge's 250ms ticker emits FUNCTION_PROGRESS
        // lines from it. Cancellation is cooperative — a `spawn_blocking` closure
        // cannot be aborted, so the engine polls the token per source and bails
        // early; the bridge's cancel `select!` already returns the 499 to the
        // caller and discards this partial result.
        let cancel = ctx.cancellation.clone();
        // The failure fallback needs the grid and histogram field, but
        // `query` moves into the closure — hoist them out first (`Grid`
        // is `Copy`).
        let grid = query.grid();
        let histogram_field = query.histogram_field().to_owned();
        let mut result = match tokio::task::spawn_blocking(move || {
            let _pins = remote_pins;
            to_result(run(sources, query, cancel, done), last)
        })
        .await
        {
            Ok(result) => result,
            Err(e) => {
                tracing::warn!("otel-logs blocking task failed: {e}");
                to_result(LogsData::empty(histogram_field, grid), last)
            }
        };
        // Advertise the stream selector on every data response.
        result.required_params = required_params;

        Ok(OtelLogsResponse::Logs(result))
    }

    /// Advertise the function to the agent/Cloud: `global` (not scoped to
    /// one node), tagged `logs`, accessible to signed-in users of the same
    /// space, with the SENSITIVE_DATA flag since log payloads may carry
    /// sensitive content.
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
/// ([`TenantId::resolve_query`]): omitted, empty, or over-long values
/// fall back to the default tenant, and an unknown tenant simply
/// matches nothing — never an implicit all-tenant union. The literal
/// `default` stays nameable here, unlike ingest's strict validation.
fn resolve_query_tenant(raw: Option<&str>) -> TenantId {
    TenantId::resolve_query(raw)
}

#[cfg(test)]
mod tests;
