//! Open a WAL file produced by `ng-ingest`, decode each frame as OTLP, and
//! flatten every record into typed `Leaf`s (via `ng-flatten`). Independent of
//! `ng-ingest`: it reads via the `wal` crate.

use std::path::Path;

use ng_flatten::flatten_log_record;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

mod perf;
pub use perf::{Metrics, Rss, read_rss};

// Re-export the flattened-leaf vocabulary so the binary (and any consumer) gets
// it from `ng-index` without depending on `ng-flatten` directly.
pub use ng_flatten::{Kind, Leaf, Value};

/// Stats gathered from one WAL file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct WalStats {
    /// Number of frames read.
    pub frames: u64,
    /// Total log records decoded.
    pub records: u64,
    /// Total flattened leaves across all records.
    pub leaves: u64,
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

/// Flatten every record in a decoded export request, one `Leaf` list per record
/// in document order (resource + scope context applied per record).
///
/// Defined here (not in `ng-flatten`) for now: walking the request structure is
/// an index-side concern; `ng-flatten` owns only the per-record transform.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> Vec<Vec<Leaf>> {
    let mut records = Vec::new();
    for rl in &request.resource_logs {
        let resource = rl.resource.as_ref();
        for sl in &rl.scope_logs {
            let scope = sl.scope.as_ref();
            for record in &sl.log_records {
                records.push(flatten_log_record(resource, scope, record));
            }
        }
    }
    records
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

/// Read a WAL file: decode each frame, flatten its records, and tally stats —
/// recording per-phase timings into `metrics`. Returns the stats plus the
/// flattened leaves of the first `sample_records` records (for inspection).
///
/// Streaming: each frame is read (`wal::Reader` decompresses LZ4 transparently),
/// decoded, and flattened before the next — so memory stays bounded to one frame
/// plus the small sample. Phases timed: `read`, `decode`, `flatten`.
pub fn count_wal(
    path: &Path,
    sample_records: usize,
    metrics: &Metrics,
) -> Result<(WalStats, Vec<Vec<Leaf>>), Error> {
    let mut reader = wal::Reader::open(path)?;
    let mut stats = WalStats::default();
    let mut sample: Vec<Vec<Leaf>> = Vec::new();
    loop {
        let frame = {
            let _t = metrics.scope("read");
            match reader.next_frame()? {
                Some(frame) => frame,
                None => break,
            }
        };
        stats.frames += 1;
        stats.header_records += frame.entry_count as u64;
        metrics.add_frames(1);
        metrics.add_bytes(frame.data.len() as u64);

        let req = {
            let _t = metrics.scope("decode");
            ExportLogsServiceRequest::decode(frame.data).map_err(|source| Error::Decode {
                frame: stats.frames,
                source,
            })?
        };

        let flattened = {
            let _t = metrics.scope("flatten");
            flatten_request(&req)
        };

        stats.records += flattened.len() as u64;
        stats.leaves += flattened.iter().map(|r| r.len() as u64).sum::<u64>();
        metrics.add_records(flattened.len() as u64);

        for record_leaves in flattened {
            if sample.len() >= sample_records {
                break;
            }
            sample.push(record_leaves);
        }
    }
    Ok((stats, sample))
}
