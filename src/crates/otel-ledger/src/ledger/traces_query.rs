//! Live trace-query sources: the bridge between the traces pipeline's
//! [`TenantRegistries`] view and the `sfsq::traces` source types. Feeds
//! the Tempo shim today (via [`tempo_shim::SourceSupplier`]) and is the
//! natural feed for any future traces query surface — the supplier is
//! Tempo-agnostic.
//!
//! Mirrors the logs handler's snapshot discipline
//! (`rpc/handler.rs::resolve_wal`): descriptors captured under ONE
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

use std::sync::Arc;

use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::registry::{TenantRegistries, WalDesc};
use file_registry::TenantId;
use sfsq::traces::{SourceId, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage};
use tokio::sync::RwLock;
use wal::prefix::{chunk_boundaries, tail_start};

/// One WAL resolved to buildable parts: everything needed to
/// materialize its sources any number of times without re-scanning.
struct ResolvedWal {
    path: std::path::PathBuf,
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

    /// Capture one consistent snapshot and materialize `copies`
    /// structurally identical source vectors from it (search validates
    /// window ⊆ completion by source id, so both roles must come from
    /// the same captured state). Chunk bytes are `Arc`-shared across
    /// copies.
    async fn capture(&self, copies: usize) -> Result<Vec<Vec<TraceSource>>, String> {
        // The Tempo wire has no tenant concept — the shim is locked to
        // the default tenant (recorded scaffold decision).
        let tenant = TenantId::resolve_query(None);
        let q = file_registry::Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        let (sealed, wal_descs) = {
            let guard = self.registries.read().await;
            guard.query_snapshot(&tenant, &q)
        };

        let mut resolved = Vec::with_capacity(wal_descs.len());
        for wal in wal_descs {
            if let Some(r) = self.resolve_wal(wal).await {
                resolved.push(r);
            }
        }

        let sets = (0..copies)
            .map(|_| {
                let mut sources: Vec<TraceSource> = Vec::new();
                for f in &sealed {
                    sources.push(TraceSource::Sfst(TraceSfstCandidate {
                        source_id: SourceId::new(f.path.display().to_string()),
                        summary: f.summary.clone(),
                        source: sfsq::Source::File(f.path.clone()),
                        coverage: None,
                    }));
                }
                for w in &resolved {
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
            .collect();
        Ok(sets)
    }

    /// Resolve one active WAL into chunk images + the tail range, or
    /// `None` to refuse the whole WAL (logs failure policy — see the
    /// module docs).
    async fn resolve_wal(&self, wal: WalDesc) -> Option<ResolvedWal> {
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

        let boundaries = chunk_boundaries(&frames, header, self.min_entries);
        let mut chunks = Vec::with_capacity(boundaries.len());
        for chunk in &boundaries {
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
            path: wal.path,
            chunks,
            tail,
        })
    }
}

#[async_trait::async_trait]
impl tempo_shim::SourceSupplier for TracesSourceSupplier {
    async fn snapshot(&self) -> Result<Vec<TraceSource>, String> {
        Ok(self.capture(1).await?.pop().expect("one copy requested"))
    }

    async fn snapshot_pair(&self) -> Result<(Vec<TraceSource>, Vec<TraceSource>), String> {
        let mut sets = self.capture(2).await?;
        let b = sets.pop().expect("two copies requested");
        let a = sets.pop().expect("two copies requested");
        Ok((a, b))
    }
}
