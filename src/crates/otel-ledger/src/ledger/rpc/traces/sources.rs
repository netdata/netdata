//! Live trace-query sources: the bridge between the traces pipeline's
//! [`TenantRegistries`] view and the `sfsq::traces` source types.
//!
//! One capture assembles, per requested range:
//!
//! - the registry snapshot's sealed SFSTs;
//! - each active WAL's durable prefix as chunk images plus a row-scanned
//!   tail, indexed off the registry lock through the shared [`ChunkCache`];
//! - with remote storage, the range's remote-only files, planned from
//!   catalogs read off the lock and downloaded once through the shared
//!   download cache.
//!
//! A remote file whose bytes could not be obtained, and each unreadable
//! catalog over its declared span, become unavailable sources. The engine
//! reports them as `remote_unavailable`.
//!
//! Descriptors are captured under one brief registry read lock, so each WAL
//! has one `valid_up_to` and the whole query sees one consistent durable
//! prefix. A WAL whose chunks do not index is refused whole for this
//! snapshot. The capture reports it ([`Capture::refused`]), because spans
//! quietly missing would read as data that does not exist.
//!
//! Source ids follow `sfsq::traces::sources`, which rejects duplicate ids
//! and overlapping WAL byte ranges. An id is always built from the file's
//! full `FileId`, never its directory:
//!
//! - a sealed file is `<stem>.sfst`;
//! - a WAL is `<stem>.wal`;
//! - a WAL's chunks are `<stem>.wal#chunk<index:06>`, zero-padded so id
//!   order stays numeric;
//! - a WAL's tail is `<stem>.wal#tail<start>`.
//!
//! Each WAL-derived source carries [`WalCoverage`] over the WAL's id. One
//! file therefore has one name wherever its bytes are served from.
//!
//! The supplier takes its window verbatim. The caller must canonicalize the
//! wire request's window. A raw `0..0` here is an empty query, deliberately.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use file_cache::CacheError;
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::query::{CatalogFile, RemotePlan};
use file_lifecycle::registry::{TenantRegistries, WalDesc};
use file_lifecycle::remote_read::RemoteRead;
use file_registry::{FileId, TenantId};
use otel_bridge::function::ProgressState;
use sfsq::traces::{
    SourceId, TraceSfstCandidate, TraceSource, TraceUnavailable, TraceWalTail, WalCoverage,
};
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;
use wal::prefix::tail_start;

use crate::ledger::wal_prefix::{WalPrefixes, index_chunk};

/// Filename extensions of the source-id derivation (the registries'
/// own on-disk extensions, which those crates keep private).
const SFST_EXT: &str = "sfst";
const WAL_EXT: &str = "wal";

/// One active WAL resolved to buildable parts: chunk images and the tail
/// range — everything needed to materialize its sources any number of
/// times without re-scanning.
struct ResolvedWal {
    id: FileId,
    path: PathBuf,
    chunks: Vec<ResolvedChunk>,
    /// The trailing un-chunked byte range, when non-empty.
    tail: Option<wal::FrameRange>,
}

/// One indexed chunk of that WAL: its boundary identity (`index` within
/// the WAL, byte `range`), the SFST summary from indexing, and the
/// `Arc`-shared byte image every copy that selected the WAL reuses.
struct ResolvedChunk {
    index: u32,
    range: wal::FrameRange,
    summary: sfst::Summary,
    bytes: Arc<Vec<u8>>,
}

/// One capture: a source vector per requested range, and the
/// download-cache pins that keep captured remote files in place. `sets[i]`
/// answers the caller's `ranges[i]`. The pins MUST live until the engine
/// has read the sources, so move them into the blocking closure that runs
/// it.
pub(crate) struct Capture {
    pub(crate) sets: Vec<Vec<TraceSource>>,
    pub(crate) pins: Vec<file_cache::CachedFile>,
    /// Parallel to `sets`: whether `ranges[i]` selected a WAL that was
    /// refused (its chunks would not index), so `sets[i]` lacks its data.
    pub(crate) refused: Vec<bool>,
}

/// What a capture does with the offloaded files a range selects.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RemotePolicy {
    /// Download them, within the download cache; a footprint over its
    /// capacity fails the capture ([`CaptureError::TooLarge`]).
    Download,
    /// Fetch nothing: each becomes an unavailable source, which the
    /// engine reports as `remote_unavailable`. For the modes whose
    /// answer must stay cheap whatever the retention holds.
    Skip,
}

/// Why a capture failed as a whole. Everything narrower degrades into the
/// captured sources instead (an unavailable remote file, a refused WAL).
#[derive(Debug)]
pub(crate) enum CaptureError {
    /// The query's remote files exceed the download cache's capacity;
    /// `at_least` is the footprint counted when planning stopped.
    TooLarge { at_least: u64, capacity: u64 },
    /// The download cache could not evict to make room: its directory is
    /// broken.
    EvictionFailed,
    /// The remote planning task panicked.
    Planning(tokio::task::JoinError),
}

/// The capture half of the traces query path. It shares the process-wide
/// chunk cache and, when remote storage is configured, its download cache.
pub(crate) struct TracesSourceSupplier {
    registries: Arc<RwLock<TenantRegistries>>,
    chunk_cache: Arc<ChunkCache>,
    /// The active WALs' chunk boundaries, kept current across queries.
    prefixes: Arc<WalPrefixes>,
    /// Reading evicted files back; `None` when remote storage is disabled.
    remote: Option<RemoteRead>,
}

impl TracesSourceSupplier {
    pub(crate) fn new(
        registries: Arc<RwLock<TenantRegistries>>,
        chunk_cache: Arc<ChunkCache>,
        prefixes: Arc<WalPrefixes>,
        remote: Option<RemoteRead>,
    ) -> Self {
        Self {
            registries,
            chunk_cache,
            prefixes,
            remote,
        }
    }

    /// One snapshot, `copies` identical source vectors: a thin wrapper
    /// over [`capture_ranges`](Self::capture_ranges) repeating the same
    /// window.
    ///
    /// A cancelled call returns no copies, because the caller discards the
    /// result anyway. Sets the call's progress total (see
    /// [`capture_ranges`](Self::capture_ranges)).
    pub(crate) async fn capture(
        &self,
        tenant: &TenantId,
        time_range: std::ops::Range<u32>,
        copies: usize,
        remote_policy: RemotePolicy,
        cancel: &CancellationToken,
        progress: &ProgressState,
    ) -> Result<Capture, CaptureError> {
        self.capture_ranges(
            tenant,
            &vec![time_range; copies],
            remote_policy,
            cancel,
            progress,
        )
        .await
    }

    /// One snapshot, one source vector per entry in `ranges`.
    /// [`Capture::sets`] keeps the caller's order. Identical ranges are
    /// scanned once, and each copy gets its own clone of the vector. Each
    /// range is pruned by its own range through the registry's
    /// file-granular predicate.
    ///
    /// Per-range pruning exists because a merged trace depends on which
    /// files were captured: spans that live only in files outside the range
    /// are left out of it. So a pass whose window is narrower than
    /// another's must be handed its own file set. With the wider set, a
    /// trace can change its duration bin, drop out of the grid, or grow its
    /// error counts.
    ///
    /// Every range is answered under one read lock, so all copies observe
    /// one `valid_up_to`. A second capture call would break that guarantee.
    /// Each WAL resolves once, and its chunk bytes are `Arc`-shared by every
    /// copy that selected it.
    ///
    /// Remote files are planned first. Under [`RemotePolicy::Download`], a
    /// query whose remote footprint exceeds the download cache fails
    /// ([`CaptureError::TooLarge`]) before any WAL work or download, and the
    /// union of every range's remote files is then downloaded once. Under
    /// [`RemotePolicy::Skip`], nothing downloads and every planned remote
    /// file becomes an unavailable source. A cancelled call returns no
    /// copies.
    ///
    /// Sets `progress`'s total before the downloads: the sources of the
    /// distinct ranges, plus one unit per planned download (none under
    /// [`RemotePolicy::Skip`]). The count assumes one ticking engine pass
    /// per distinct range, one tick per source. The download part is an
    /// upper bound, as a cached file downloads nothing. When two ticking
    /// passes share one range, `done` can exceed the total.
    pub(crate) async fn capture_ranges(
        &self,
        tenant: &TenantId,
        ranges: &[std::ops::Range<u32>],
        remote_policy: RemotePolicy,
        cancel: &CancellationToken,
        progress: &ProgressState,
    ) -> Result<Capture, CaptureError> {
        let cancelled = || Capture {
            sets: Vec::new(),
            pins: Vec::new(),
            refused: Vec::new(),
        };
        if ranges.is_empty() {
            return Ok(cancelled());
        }
        // Distinct ranges only: copies over one range share its answer,
        // so the registry is scanned once per range, not once per copy.
        let mut distinct: Vec<file_registry::Query> = Vec::new();
        let mut per_copy: Vec<usize> = Vec::with_capacity(ranges.len());
        for range in ranges {
            let at = match distinct.iter().position(|d| &d.time_range == range) {
                Some(at) => at,
                None => {
                    distinct.push(file_registry::Query {
                        time_range: range.clone(),
                        partition_keys: Vec::new(),
                    });
                    distinct.len() - 1
                }
            };
            per_copy.push(at);
        }
        // Under one read lock, in memory only: every range's local snapshot
        // and, with remote storage, which catalogs to read and what each
        // range already serves locally.
        let (snapshots, remote_input) = {
            let guard = self.registries.read().await;
            let snapshots: Vec<(Vec<file_registry::SelectedFile>, Vec<WalDesc>)> = distinct
                .iter()
                .map(|q| guard.query_snapshot(tenant, q))
                .collect();
            let remote_input = self
                .remote
                .is_some()
                .then(|| guard.remote_plan_input(tenant, &distinct));
            (snapshots, remote_input)
        };

        let RemotePlan {
            mut per_range,
            union,
            unreadable,
        } = match (&self.remote, remote_input) {
            (Some(remote), Some(input)) => match remote_policy {
                RemotePolicy::Download => {
                    let capacity = remote.cache().capacity();
                    tokio::task::spawn_blocking(move || input.plan_within(capacity))
                        .await
                        .map_err(CaptureError::Planning)?
                        .map_err(|too_large| CaptureError::TooLarge {
                            at_least: too_large.at_least,
                            capacity: too_large.capacity,
                        })?
                }
                // Nothing downloads, so the cache's capacity is moot.
                RemotePolicy::Skip => tokio::task::spawn_blocking(move || input.plan())
                    .await
                    .map_err(CaptureError::Planning)?,
            },
            _ => RemotePlan::default(),
        };
        // Ranges with no plan (remote storage disabled) contribute no
        // remote entries.
        per_range.resize_with(distinct.len(), Vec::new);
        if cancel.is_cancelled() {
            return Ok(cancelled());
        }

        // Every WAL any range selected resolves once, keyed by its full
        // `FileId`. A refused one is absent for every copy alike (one
        // snapshot, one verdict).
        let mut resolved: HashMap<FileId, ResolvedWal> = HashMap::new();
        for wal in snapshots.iter().flat_map(|(_, wals)| wals) {
            if resolved.contains_key(&wal.id) {
                continue;
            }
            if let Some(r) = self.resolve_wal(wal, cancel).await {
                resolved.insert(wal.id, r);
            }
        }
        if cancel.is_cancelled() {
            return Ok(cancelled());
        }
        // `resolve_wal` returns `None` for a cancel too, but a cancelled
        // capture has returned above: what is missing now was refused.
        let refused_distinct: Vec<bool> = snapshots
            .iter()
            .map(|(_, wals)| wals.iter().any(|wal| !resolved.contains_key(&wal.id)))
            .collect();

        let mut distinct_sources: Vec<Vec<TraceSource>> = snapshots
            .iter()
            .map(|(sealed, wal_descs)| local_sources(sealed, wal_descs, &resolved))
            .collect();
        // The unreadable catalogs each range overlaps: their entries are
        // unknown, so each is one unavailable source over its span.
        let unreadable_per_range: Vec<Vec<&CatalogFile>> = distinct
            .iter()
            .map(|q| {
                unreadable
                    .iter()
                    .filter(|c| q.overlaps(c.min_timestamp_s, c.max_timestamp_s))
                    .collect()
            })
            .collect();
        let sources_total: usize = distinct_sources
            .iter()
            .zip(&per_range)
            .zip(&unreadable_per_range)
            .map(|((local, remote), catalogs)| local.len() + remote.len() + catalogs.len())
            .sum();
        let downloads = match remote_policy {
            RemotePolicy::Download => union.len(),
            RemotePolicy::Skip => 0,
        };
        progress.set_total(sources_total + downloads);

        let (downloaded, pins) = match &self.remote {
            Some(remote) if remote_policy == RemotePolicy::Download && !union.is_empty() => {
                match remote.fetch(union, cancel, progress.done_counter()).await {
                    Ok(fetched) => (fetched.files, fetched.pins),
                    Err(CacheError::Cancelled) => return Ok(cancelled()),
                    Err(CacheError::TooLarge {
                        footprint,
                        capacity,
                    }) => {
                        return Err(CaptureError::TooLarge {
                            at_least: footprint,
                            capacity,
                        });
                    }
                    Err(CacheError::EvictionFailed) => return Err(CaptureError::EvictionFailed),
                }
            }
            _ => (Vec::new(), Vec::new()),
        };
        if cancel.is_cancelled() {
            return Ok(cancelled());
        }

        // Remote files are named like local sealed files (`<stem>.sfst`), so
        // one file has one name wherever its bytes come from.
        let downloaded: HashMap<FileId, &file_registry::SelectedFile> =
            downloaded.iter().map(|f| (f.id, f)).collect();
        for ((sources, remote), catalogs) in distinct_sources
            .iter_mut()
            .zip(&per_range)
            .zip(&unreadable_per_range)
        {
            for entry in remote {
                let source_id = SourceId::new(entry.id.to_filename(SFST_EXT));
                sources.push(match downloaded.get(&entry.id) {
                    Some(file) => TraceSource::Sfst(TraceSfstCandidate {
                        source_id,
                        summary: file.summary.clone(),
                        source: sfsq::Source::File(file.path.clone()),
                        coverage: None,
                    }),
                    None => TraceSource::Unavailable(TraceUnavailable {
                        source_id,
                        summary: entry.summary(),
                    }),
                });
            }
            for catalog in catalogs {
                sources.push(TraceSource::Unavailable(TraceUnavailable {
                    source_id: SourceId::new(catalog_source_id(catalog)),
                    summary: sfst::Summary {
                        min_timestamp_s: catalog.min_timestamp_s,
                        max_timestamp_s: catalog.max_timestamp_s,
                        record_count: 0,
                        content_meta: Vec::new(),
                    },
                }));
            }
        }
        let sets = per_copy
            .iter()
            .map(|&at| distinct_sources[at].clone())
            .collect();
        let refused = per_copy.iter().map(|&at| refused_distinct[at]).collect();
        Ok(Capture {
            sets,
            pins,
            refused,
        })
    }

    /// Resolve one active WAL into chunk images and the tail range, or
    /// `None` to refuse the whole WAL (see the module docs). Polls `cancel`
    /// between chunks. A cancelled call also returns `None`, on purpose:
    /// the capture's result is discarded either way.
    async fn resolve_wal(&self, wal: &WalDesc, cancel: &CancellationToken) -> Option<ResolvedWal> {
        // Poll before the boundary scan too — it is a blocking file read
        // a cancelled call shouldn't pay for.
        if cancel.is_cancelled() {
            return None;
        }
        let header = wal::HEADER_SIZE as u64;
        // Boundaries up to this snapshot's bound.
        let boundaries = match self
            .prefixes
            .chunks(wal.id.seq, &wal.path, wal.valid_up_to)
            .await
        {
            Ok(boundaries) => boundaries,
            Err(e) => {
                tracing::warn!(seq = wal.id.seq, "traces WAL {e}");
                return None;
            }
        };
        // The boundary scan is itself a blocking phase — poll on the way
        // out of it, then again before each chunk.
        if cancel.is_cancelled() {
            return None;
        }

        let mut chunks = Vec::with_capacity(boundaries.len());
        for chunk in &boundaries {
            if cancel.is_cancelled() {
                return None;
            }
            let seq = wal.id.seq;
            // Traces chunk indexing for this byte range, through the
            // shared cache. (seq, index) keys never collide with logs
            // because seqs are process-global (one shared highwater).
            let init = index_chunk(ng_index::index_traces_range, wal.path.clone(), *chunk);
            match self.chunk_cache.get_or_index(seq, chunk.index, init).await {
                Ok(indexed) => chunks.push(ResolvedChunk {
                    index: chunk.index,
                    range: chunk.range,
                    summary: indexed.summary.clone(),
                    bytes: Arc::clone(&indexed.bytes),
                }),
                Err(e) => {
                    tracing::warn!(
                        seq,
                        index = chunk.index,
                        "traces chunk indexing failed; refusing this WAL: {e}"
                    );
                    return None;
                }
            }
        }

        let tail_begin = tail_start(&boundaries, header);
        let tail = (tail_begin < wal.valid_up_to)
            .then(|| wal::FrameRange::new(tail_begin, wal.valid_up_to));
        Some(ResolvedWal {
            id: wal.id,
            path: wal.path.clone(),
            chunks,
            tail,
        })
    }
}

/// One range's local sources: its sealed files, then each resolved WAL's
/// chunks (ascending index) and tail. A refused WAL contributes nothing
/// — it is simply absent from `resolved`.
fn local_sources(
    sealed: &[file_registry::SelectedFile],
    wal_descs: &[WalDesc],
    resolved: &HashMap<FileId, ResolvedWal>,
) -> Vec<TraceSource> {
    let mut sources: Vec<TraceSource> = Vec::new();
    for f in sealed {
        sources.push(TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(f.id.to_filename(SFST_EXT)),
            summary: f.summary.clone(),
            source: sfsq::Source::File(f.path.clone()),
            coverage: None,
        }));
    }
    for w in wal_descs.iter().filter_map(|d| resolved.get(&d.id)) {
        let wal_id: Arc<str> = w.id.to_filename(WAL_EXT).into();
        for c in &w.chunks {
            sources.push(TraceSource::Sfst(TraceSfstCandidate {
                source_id: SourceId::new(format!("{wal_id}#chunk{:06}", c.index)),
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
                source_id: SourceId::new(format!("{wal_id}#tail{}", range.start())),
                path: w.path.clone(),
                coverage: WalCoverage {
                    wal_id: Arc::clone(&wal_id),
                    range,
                },
            }));
        }
    }
    sources
}

/// An unreadable catalog's source id: its filename, which encodes its
/// identity, sequence and time span.
fn catalog_source_id(catalog: &CatalogFile) -> String {
    match catalog.path.file_name() {
        Some(name) => name.to_string_lossy().into_owned(),
        None => catalog.path.display().to_string(),
    }
}

#[cfg(test)]
mod tests;
