//! PROOF SCAFFOLD (traces-proof SOW; revert with the skeleton).
//!
//! The traces pipeline's seal/index worker — the analogue of the logs
//! [`crate::indexer::Indexer`], with the SAME [`IndexerRequest`]/
//! [`IndexerResponse`] contract so the shared recovery path
//! (`recover_unindexed`) and the shared `handle_indexer_resp` drive it
//! unchanged. Only the seal body differs:
//!
//! - logs: `ng_index::build_sfst_file` decodes the WAL's ng-flatten frames and
//!   writes a full split-FST index;
//! - traces (here): NO content decode. The WAL frame header already carries a
//!   content-agnostic `entry_count` + per-frame `timestamp_ns`, so the seal sums
//!   `entry_count` → `record_count`, folds frame timestamps → min/max, copies the
//!   header's opaque `content_meta`, and writes a content-light SFST via
//!   `sfst::IndexWriter::write_summary_only`. That is enough for the substrate to track,
//!   catalog, upload, and recover the file like any other.
//!
//! Fakes (real traces feature must replace): timestamps are frame
//! ingestion-time, not span-time; the frame payload is never read (the file
//! holds only a `SUMR` summary, no queryable trace content).

use std::collections::{HashMap, VecDeque};
use std::path::Path;

use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};

/// The WAL `payload_format` id this seal expects. Stamped by the producer,
/// `otel_ingestor::trace_service::TRACES_PROOF_PAYLOAD_FORMAT`; pinned here
/// because the sibling workers share no crate. Format ids are append-only,
/// so the pin cannot drift.
const TRACES_PROOF_PAYLOAD_FORMAT: u16 = 2;

/// Tracks a single in-flight seal operation.
struct SealTask {
    seq: u64,
    started_at: Instant,
}

pub struct TracesIndexer;

impl Component for TracesIndexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = ();

    async fn run(
        _args: (),
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        let (done_tx, mut done_rx) = mpsc::unbounded_channel::<(u64, IndexerResponse)>();
        let mut in_flight: HashMap<u64, SealTask> = HashMap::new();
        let mut queue: VecDeque<IndexerRequest> = VecDeque::new();
        let max_concurrent: usize = 1;

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(req) => {
                        if in_flight.len() < max_concurrent {
                            start_seal(req, &mut in_flight, done_tx.clone());
                        } else {
                            queue.push_back(req);
                        }
                    }
                    None => break,
                },
                Some((seq, resp)) = done_rx.recv() => {
                    if let Some(task) = in_flight.remove(&seq) {
                        tracing::info!(
                            "traces seal done seq={} elapsed_ms={}",
                            task.seq,
                            task.started_at.elapsed().as_millis(),
                        );
                    }
                    let _ = tx.send(resp);

                    if let Some(req) = queue.pop_front() {
                        start_seal(req, &mut in_flight, done_tx.clone());
                    }
                }
            }
        }
    }
}

fn start_seal(
    req: IndexerRequest,
    in_flight: &mut HashMap<u64, SealTask>,
    done_tx: mpsc::UnboundedSender<(u64, IndexerResponse)>,
) {
    let IndexerRequest::Index {
        wal_path,
        sfst_path,
    } = req;

    let seq = file_registry::FileId::parse(&wal_path)
        .map(|id| id.seq)
        .unwrap_or(0);

    in_flight.insert(
        seq,
        SealTask {
            seq,
            started_at: Instant::now(),
        },
    );

    tracing::info!(
        "traces seal started wal={} index={}",
        wal_path.display(),
        sfst_path.display(),
    );

    tokio::task::spawn_blocking(move || {
        let resp = match seal_summary_only(&wal_path, &sfst_path) {
            Ok((summary, size)) => IndexerResponse::Indexed {
                seq,
                path: sfst_path,
                summary,
                size: file_registry::ByteSize(size),
            },
            Err(e) => {
                tracing::error!("traces seal failed wal={}: {e:#}", wal_path.display());
                IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: e.to_string(),
                }
            }
        };

        let _ = done_tx.send((seq, resp));
    });
}

/// Read a sealed traces WAL and write a content-light SFST (`SUMR` only).
///
/// Content-agnostic: it never decodes a frame payload, only the WAL frame
/// headers' `entry_count` + `timestamp_ns` and the file header's opaque
/// `content_meta`.
fn seal_summary_only(wal_path: &Path, sfst_path: &Path) -> anyhow::Result<(sfst::Summary, u64)> {
    let mut reader = wal::Reader::open(wal_path)?;
    let found = reader.header().payload_format;
    if found != TRACES_PROOF_PAYLOAD_FORMAT {
        anyhow::bail!(
            "WAL payload format {found} is not the traces proof format \
             {TRACES_PROOF_PAYLOAD_FORMAT}; refusing to seal a file written by a different codec"
        );
    }
    let content_meta = reader.header().content_meta.clone();

    let mut record_count: u64 = 0;
    let mut min_ns: Option<u64> = None;
    let mut max_ns: u64 = 0;
    while let Some(frame) = reader.next_frame()? {
        record_count += frame.entry_count as u64;
        let ts = frame.timestamp_ns.0;
        if ts != 0 {
            min_ns = Some(min_ns.map_or(ts, |m| m.min(ts)));
            max_ns = max_ns.max(ts);
        }
    }

    let summary = sfst::Summary {
        min_timestamp_s: (min_ns.unwrap_or(0) / 1_000_000_000) as u32,
        max_timestamp_s: (max_ns / 1_000_000_000) as u32,
        // `write_summary_only` / the Ledger require a non-empty file; a sealed
        // traces WAL always carries spans (entry_count > 0).
        record_count: record_count.min(u32::MAX as u64) as u32,
        content_meta,
    };

    // Write atomically (build in memory, then fsync+rename), matching the logs
    // indexer (`IndexWriter::write_file` uses `durable::AtomicFile`). A summary-only SFST is
    // tiny, so the in-memory buffer is cheap; a crash mid-write then leaves no
    // partial SFST that recovery would treat as a valid sealed file.
    let buf = sfst::IndexWriter::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)?
        .into_inner();
    file_registry::durable::write_atomic(sfst_path, &buf)?;
    // Read the size back from disk (rather than `buf.len()`), matching the logs
    // indexer — so the tracked size always reflects the on-disk file even if the
    // atomic-write path ever adds framing of its own.
    let size = std::fs::metadata(sfst_path)?.len();
    Ok((summary, size))
}
