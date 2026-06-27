//! Open a WAL file produced by `ng-ingest`, decode each frame as OTLP, and
//! flatten every record into typed `Leaf`s (via `ng-flatten`). Independent of
//! `ng-ingest`: it reads via the `wal` crate.

use std::path::Path;

use ng_flatten::{flatten_record, flatten_resource, flatten_scope};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

mod perf;
pub use perf::{Metrics, Rss, read_rss};

// Re-export the flattened-leaf vocabulary so the binary (and any consumer) gets
// it from `ng-index` without depending on `ng-flatten` directly.
pub use ng_flatten::{Kind, Leaf, Value};

/// Flattened OTLP request, keeping OTLP's grouping so resource/scope leaves are
/// produced once per group rather than duplicated per record. A record's full
/// field set is its own leaves plus its scope's and resource's.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct FlattenedRequest {
    pub resources: Vec<ResourceGroup>,
}

/// One resource and the scope groups under it.
#[derive(Debug, Clone, PartialEq)]
pub struct ResourceGroup {
    pub resource: Vec<Leaf>,
    pub scopes: Vec<ScopeGroup>,
}

/// One scope and the records under it (each record = its own leaves only).
#[derive(Debug, Clone, PartialEq)]
pub struct ScopeGroup {
    pub scope: Vec<Leaf>,
    pub records: Vec<Vec<Leaf>>,
}

/// Flatten a decoded export request into the hierarchical [`FlattenedRequest`].
///
/// Defined here (not in `ng-flatten`) for now: walking the request structure is
/// an index-side concern; `ng-flatten` owns only the per-level transforms.
/// Resource is flattened once per `ResourceLogs`, scope once per `ScopeLogs`.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> FlattenedRequest {
    let mut resources = Vec::with_capacity(request.resource_logs.len());
    for rl in &request.resource_logs {
        let resource = rl.resource.as_ref().map(flatten_resource).unwrap_or_default();
        let mut scopes = Vec::with_capacity(rl.scope_logs.len());
        for sl in &rl.scope_logs {
            let scope = sl.scope.as_ref().map(flatten_scope).unwrap_or_default();
            let records = sl.log_records.iter().map(flatten_record).collect();
            scopes.push(ScopeGroup { scope, records });
        }
        resources.push(ResourceGroup { resource, scopes });
    }
    FlattenedRequest { resources }
}

/// A sampled record with its (cloned) resource + scope context, for `--print`.
#[derive(Debug, Clone)]
pub struct SampleRecord {
    pub resource: Vec<Leaf>,
    pub scope: Vec<Leaf>,
    pub own: Vec<Leaf>,
}

/// Stats gathered from one WAL file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct WalStats {
    /// Number of frames read.
    pub frames: u64,
    /// Total log records decoded.
    pub records: u64,
    /// Total flattened leaves (de-duplicated: resource/scope counted once per group).
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

/// Read a WAL file: decode each frame, flatten its records (hierarchically), and
/// tally stats — recording per-phase timings into `metrics`. Returns the stats
/// plus the first `sample_records` records with their resource/scope context (for
/// inspection).
///
/// Streaming: each frame is read (`wal::Reader` decompresses LZ4 transparently),
/// decoded, and flattened before the next — so memory stays bounded to one frame
/// plus the small sample. Phases timed: `read`, `decode`, `flatten`.
pub fn count_wal(
    path: &Path,
    sample_records: usize,
    metrics: &Metrics,
) -> Result<(WalStats, Vec<SampleRecord>), Error> {
    let mut reader = wal::Reader::open(path)?;
    let mut stats = WalStats::default();
    let mut sample: Vec<SampleRecord> = Vec::new();
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

        // Tally + sample (outside the timed flatten phase).
        let mut frame_records = 0u64;
        for rg in &flattened.resources {
            stats.leaves += rg.resource.len() as u64;
            for sg in &rg.scopes {
                stats.leaves += sg.scope.len() as u64;
                for record in &sg.records {
                    frame_records += 1;
                    stats.leaves += record.len() as u64;
                    if sample.len() < sample_records {
                        sample.push(SampleRecord {
                            resource: rg.resource.clone(),
                            scope: sg.scope.clone(),
                            own: record.clone(),
                        });
                    }
                }
            }
        }
        stats.records += frame_records;
        metrics.add_records(frame_records);
    }
    Ok((stats, sample))
}
