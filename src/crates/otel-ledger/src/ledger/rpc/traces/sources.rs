//! Live trace-query sources: the bridge between the traces pipeline's
//! [`TenantRegistries`] view and the `sfsq::traces` source types. Feeds
//! every one of the `otel-traces` Function's data modes.
//!
//! Mirrors the logs handler's snapshot discipline
//! (`rpc/logs/handler.rs::resolve_wal`): descriptors captured under ONE
//! brief registry read lock (one `valid_up_to` per WAL — the whole
//! query sees one consistent durable prefix), chunk SFSTs built OFF the
//! lock through the shared singleflight [`ChunkCache`] (traces seal:
//! [`ng_index::build_sfst_traces_range`]), and the logs failure policy:
//! a WAL whose chunks won't build/parse is refused WHOLE for this
//! snapshot (its data returns via the sealed SFST after rotation, or on
//! a later query — build errors aren't cached). Refusal is logged; the
//! engine's per-source `SourceFailure` accounting starts at the sources
//! it is given, so a refused WAL is a logged gap, exactly as it is for
//! logs.
//!
//! Source identity per `sfsq::traces::sources` docs: sealed files use
//! their path (which encodes the full `FileId`); a WAL's chunks and
//! tail derive `{path}#chunk{i}` / `{path}#tail{start}` with
//! [`WalCoverage`] over the WAL path, so the engine's overlap
//! validation sees every WAL-derived byte range.
//!
//! The supplier takes its window verbatim: canonicalizing the wire
//! request's window (the logs precedent defaults an unspecified
//! `after`/`before` to a recent window before querying) is the CALLER's
//! responsibility — the wire adapter of each data mode — not this
//! module's. A raw `0..0` here is an empty query, deliberately.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::{TenantRegistries, WalDesc};
use file_registry::TenantId;
use sfsq::traces::{SourceId, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage};
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;
use wal::prefix::{chunk_boundaries, tail_start};

/// One WAL resolved to buildable parts: everything needed to
/// materialize its sources any number of times without re-scanning.
struct ResolvedWal {
    path: PathBuf,
    chunks: Vec<ResolvedChunk>,
    /// The trailing un-chunked byte range, when non-empty.
    tail: Option<wal::FrameRange>,
}

struct ResolvedChunk {
    index: u32,
    range: wal::FrameRange,
    summary: sfst::Summary,
    bytes: Arc<Vec<u8>>,
}

pub(crate) struct TracesSourceSupplier {
    registries: Arc<RwLock<TenantRegistries>>,
    chunk_cache: Arc<ChunkCache>,
    min_entries: u64,
}

impl TracesSourceSupplier {
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

    /// Capture one consistent snapshot of `tenant`'s sources overlapping
    /// `time_range` (unix seconds; window pruning is file-granular) and
    /// materialize `copies` structurally identical source vectors from
    /// it. Search passes its COMPLETION range (the match window widened
    /// by the slack) and hands identical copies to both roles — the
    /// engine narrows the window role itself, and window ⊆ completion
    /// holds by identity; two captures could observe different
    /// `valid_up_to` and trip the engine's membership check. Chunk
    /// bytes are `Arc`-shared across copies.
    ///
    /// The chunk-building phase can be the slow one, so it polls `cancel`
    /// between builds (the logs handler's discipline); a cancelled call
    /// returns empty — the caller is about to discard the result anyway.
    pub(crate) async fn capture(
        &self,
        tenant: &TenantId,
        time_range: std::ops::Range<u32>,
        copies: usize,
        cancel: &CancellationToken,
    ) -> Vec<Vec<TraceSource>> {
        self.capture_ranges(tenant, &vec![time_range; copies], cancel)
            .await
    }

    /// [`capture`](Self::capture) with a range PER copy: one snapshot,
    /// one source vector per entry in `ranges`, each pruned by its own
    /// range through the registry's own predicate.
    ///
    /// Per-range pruning exists because a merged trace envelope is a
    /// function of WHICH FILES were captured (the engine's
    /// file-granularity caveat: spans living only in files outside the
    /// range merge a truncated envelope). So a pass whose window is
    /// narrower than another's — the Functions view's window aggregate
    /// beside its page — must be handed its OWN file set, or it returns
    /// different numbers than the standalone mode does for the same
    /// window: a widened envelope can change a trace's duration bin,
    /// drop it from the grid, or grow the span/error totals.
    ///
    /// Every range is answered under ONE read lock, so all copies still
    /// observe one `valid_up_to` — the single-snapshot guarantee a
    /// second `capture` call would break — and each WAL resolves ONCE,
    /// its chunk bytes `Arc`-shared by every copy that selected it.
    pub(crate) async fn capture_ranges(
        &self,
        tenant: &TenantId,
        ranges: &[std::ops::Range<u32>],
        cancel: &CancellationToken,
    ) -> Vec<Vec<TraceSource>> {
        if ranges.is_empty() {
            return Vec::new();
        }
        // Distinct ranges only: copies over one range share its answer
        // (search's two roles always do), so the registry is scanned
        // once per range, not once per copy.
        let mut distinct: Vec<std::ops::Range<u32>> = Vec::new();
        let mut per_copy: Vec<usize> = Vec::with_capacity(ranges.len());
        for range in ranges {
            let at = match distinct.iter().position(|d| d == range) {
                Some(at) => at,
                None => {
                    distinct.push(range.clone());
                    distinct.len() - 1
                }
            };
            per_copy.push(at);
        }
        let snapshots: Vec<(Vec<file_registry::SelectedFile>, Vec<WalDesc>)> = {
            let guard = self.registries.read().await;
            distinct
                .into_iter()
                .map(|time_range| {
                    guard.query_snapshot(
                        tenant,
                        &file_registry::Query {
                            time_range,
                            partition_keys: Vec::new(),
                        },
                    )
                })
                .collect()
        };

        // Every WAL any range selected resolves ONCE, keyed by the path
        // that encodes its full `FileId`; a refused one is absent for
        // every copy alike (one snapshot, one verdict).
        let mut resolved: HashMap<PathBuf, ResolvedWal> = HashMap::new();
        for wal in snapshots.iter().flat_map(|(_, wals)| wals) {
            if resolved.contains_key(&wal.path) {
                continue;
            }
            if let Some(r) = self.resolve_wal(wal, cancel).await {
                resolved.insert(wal.path.clone(), r);
            }
        }
        // One check dominates the per-WAL ones: nothing between here and
        // the return blocks, so this is the last point cancellation can
        // save the copy-materialization work.
        if cancel.is_cancelled() {
            return Vec::new();
        }

        per_copy
            .into_iter()
            .map(|at| {
                let (sealed, wal_descs) = &snapshots[at];
                let mut sources: Vec<TraceSource> = Vec::new();
                for f in sealed {
                    sources.push(TraceSource::Sfst(TraceSfstCandidate {
                        source_id: SourceId::new(f.path.display().to_string()),
                        summary: f.summary.clone(),
                        source: sfsq::Source::File(f.path.clone()),
                        coverage: None,
                    }));
                }
                for w in wal_descs.iter().filter_map(|d| resolved.get(&d.path)) {
                    let wal_id: Arc<str> = w.path.display().to_string().into();
                    for c in &w.chunks {
                        sources.push(TraceSource::Sfst(TraceSfstCandidate {
                            source_id: SourceId::new(format!(
                                "{}#chunk{}",
                                w.path.display(),
                                c.index
                            )),
                            summary: c.summary.clone(),
                            source: sfsq::Source::Memory(c.bytes.clone()),
                            coverage: Some(WalCoverage {
                                wal_id: Arc::clone(&wal_id),
                                range: c.range,
                            }),
                        }));
                    }
                    if let Some(range) = w.tail {
                        sources.push(TraceSource::Tail(TraceWalTail {
                            source_id: SourceId::new(format!(
                                "{}#tail{}",
                                w.path.display(),
                                range.start()
                            )),
                            path: w.path.clone(),
                            coverage: WalCoverage {
                                wal_id: Arc::clone(&wal_id),
                                range,
                            },
                        }));
                    }
                }
                sources
            })
            .collect()
    }

    /// Resolve one active WAL into chunk images + the tail range, or
    /// `None` to refuse the whole WAL (logs failure policy — see the
    /// module docs). Polls `cancel` between chunk builds; a cancelled
    /// call returns `None` (indistinguishable from refusal on purpose —
    /// the capture's result is discarded either way).
    async fn resolve_wal(&self, wal: &WalDesc, cancel: &CancellationToken) -> Option<ResolvedWal> {
        // Poll before the boundary scan too — it is a blocking file read
        // a cancelled call shouldn't pay for.
        if cancel.is_cancelled() {
            return None;
        }
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
                tracing::warn!(seq = wal.seq, "traces WAL boundary scan failed: {e}");
                return None;
            }
            Err(e) => {
                tracing::warn!(seq = wal.seq, "traces WAL boundary scan task failed: {e}");
                return None;
            }
        };
        // The boundary scan is itself a blocking phase — poll on the way
        // out of it, then again before each chunk build.
        if cancel.is_cancelled() {
            return None;
        }

        let boundaries = chunk_boundaries(&frames, header, self.min_entries);
        let mut chunks = Vec::with_capacity(boundaries.len());
        for chunk in &boundaries {
            if cancel.is_cancelled() {
                return None;
            }
            let seq = wal.seq;
            let path = wal.path.clone();
            let (range, expected) = (chunk.range, chunk.entry_count);
            // The traces seal for the byte range; record-count
            // cross-check as in the logs path. Singleflighted through
            // the shared cache — (seq, index) keys never collide with
            // logs because seqs are process-global (shared highwater).
            let init = async move {
                match tokio::task::spawn_blocking(move || {
                    ng_index::build_sfst_traces_range(&path, range)
                })
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
                    Ok(Err(e)) => Err(format!("build_sfst_traces_range: {e}")),
                    Err(e) => Err(format!("build task: {e}")),
                }
            };
            match self.chunk_cache.get_or_build(seq, chunk.index, init).await {
                Ok(bytes) => match sfst::IndexReader::open(&bytes[..]) {
                    Ok(reader) => chunks.push(ResolvedChunk {
                        index: chunk.index,
                        range: chunk.range,
                        summary: reader.summary().clone(),
                        bytes,
                    }),
                    Err(e) => {
                        tracing::warn!(
                            seq,
                            index = chunk.index,
                            "traces chunk parse failed; refusing this WAL: {e}"
                        );
                        return None;
                    }
                },
                Err(e) => {
                    tracing::warn!(
                        seq,
                        index = chunk.index,
                        "traces chunk build failed; refusing this WAL: {e}"
                    );
                    return None;
                }
            }
        }

        let tail_begin = tail_start(&boundaries, header);
        let tail = (tail_begin < wal.valid_up_to)
            .then(|| wal::FrameRange::new(tail_begin, wal.valid_up_to));
        Some(ResolvedWal {
            path: wal.path.clone(),
            chunks,
            tail,
        })
    }
}

#[cfg(test)]
mod tests;
