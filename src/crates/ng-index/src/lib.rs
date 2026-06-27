//! Open a WAL file produced by `ng-ingest` and report stats over its frames.
//! Independent of `ng-ingest`: it reads via the `wal` crate and decodes each
//! frame payload as OTLP protobuf.

use std::path::Path;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;
use rayon::prelude::*;

mod perf;
pub use perf::{Metrics, Rss, read_rss};

/// Stats gathered from one WAL file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct WalStats {
    /// Number of frames read.
    pub frames: u64,
    /// Total log records counted by decoding each frame's OTLP payload.
    pub records: u64,
    /// Total log records claimed by the frame headers (`entry_count`). Equals
    /// `records` for an intact file; a mismatch signals corruption or an
    /// encoding the reader does not understand.
    pub header_records: u64,
}

impl WalStats {
    /// Whether the decoded record count agrees with the frame headers.
    pub fn consistent(&self) -> bool {
        self.records == self.header_records
    }
}

/// Count the log records in one decoded OTLP batch. A private copy (this crate
/// shares no code with `ng-ingest`).
fn count_log_records(req: &ExportLogsServiceRequest) -> usize {
    req.resource_logs
        .iter()
        .flat_map(|rl| rl.scope_logs.iter())
        .map(|sl| sl.log_records.len())
        .sum()
}

/// Errors reading or decoding a WAL file.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("wal read failed: {0}")]
    Wal(#[from] wal::Error),
    #[error("frame {frame} payload is not a valid OTLP ExportLogsServiceRequest: {source}")]
    Decode {
        frame: u64,
        source: prost::DecodeError,
    },
}

/// Read a WAL file and tally its frames and log records, recording per-phase
/// timings and throughput counters into `metrics`.
///
/// Pipeline: read up to `chunk_frames` frame payloads sequentially (`wal::Reader`
/// decompresses LZ4 transparently), decode that chunk in parallel on the rayon
/// pool, accumulate the decoded corpus, repeat; then count over the whole corpus.
/// The rayon step does nothing but decode — all timing/counting is on the calling
/// thread. Phases timed: `read`, `decode` (the parallel section), `process`.
///
/// `chunk_frames` must be >= 1.
pub fn count_wal(path: &Path, chunk_frames: usize, metrics: &Metrics) -> Result<WalStats, Error> {
    let mut reader = wal::Reader::open(path)?;
    let mut stats = WalStats::default();
    let mut decoded: Vec<ExportLogsServiceRequest> = Vec::new();

    loop {
        // Read: copy up to `chunk_frames` payloads out of the reader's reused
        // buffer into owned bytes (required to outlive the next read / cross threads).
        let mut chunk: Vec<Vec<u8>> = Vec::with_capacity(chunk_frames);
        {
            let _t = metrics.scope("read");
            while chunk.len() < chunk_frames {
                match reader.next_frame()? {
                    Some(frame) => {
                        stats.frames += 1;
                        stats.header_records += frame.entry_count as u64;
                        metrics.add_frames(1);
                        metrics.add_bytes(frame.data.len() as u64);
                        chunk.push(frame.data.to_vec());
                    }
                    None => break,
                }
            }
        }
        if chunk.is_empty() {
            break;
        }

        // Decode: parallel, decode-only. The global frame index (1-based) is
        // `base + local + 1`, preserved for error reporting.
        let base = decoded.len() as u64;
        let chunk_decoded: Vec<ExportLogsServiceRequest> = {
            let _t = metrics.scope("decode");
            chunk
                .par_iter()
                .enumerate()
                .map(|(i, buf)| {
                    ExportLogsServiceRequest::decode(buf.as_slice()).map_err(|source| {
                        Error::Decode {
                            frame: base + i as u64 + 1,
                            source,
                        }
                    })
                })
                .collect::<Result<Vec<_>, Error>>()?
        };
        decoded.extend(chunk_decoded);
    }

    // Process: count over the fully-decoded corpus (sequential, off the pool).
    {
        let _t = metrics.scope("process");
        for req in &decoded {
            let records = count_log_records(req) as u64;
            stats.records += records;
            metrics.add_records(records);
        }
    }

    Ok(stats)
}
