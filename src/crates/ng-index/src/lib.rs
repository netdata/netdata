//! Open a WAL file produced by `ng-ingest`, decode each frame as OTLP, and
//! flatten it into a per-frame schema tree + entries (via `ng-flatten`).
//! Independent of `ng-ingest`: it reads via the `wal` crate.

use std::path::Path;

use ng_flatten::Flattener;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

mod perf;
pub use perf::{Metrics, Rss, read_rss};

// Re-export the flattening vocabulary so the binary (and any consumer) gets it
// from `ng-index` without depending on `ng-flatten` directly.
pub use ng_flatten::{Entry, Kind, Leaf, NodeId, SchemaTree, Value};

/// A flattened frame: one schema tree shared by all its records, plus the OTLP
/// grouping. Resource/scope are flattened once per group; records hold only their
/// own entries. Every entry's `node` indexes into `tree`.
#[derive(Debug, Clone)]
pub struct FlattenedRequest {
    pub tree: SchemaTree,
    pub resources: Vec<ResourceGroup>,
}

/// One resource and the scope groups under it.
#[derive(Debug, Clone)]
pub struct ResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<ScopeGroup>,
}

/// One scope and the records under it (each record = its own entries only).
#[derive(Debug, Clone)]
pub struct ScopeGroup {
    pub scope: Vec<Entry>,
    pub records: Vec<Vec<Entry>>,
}

/// Flatten a decoded export request into a per-frame [`FlattenedRequest`].
///
/// Defined here (not in `ng-flatten`) for now: walking the request structure is
/// an index-side concern. One [`Flattener`] builds the shared tree; resource is
/// flattened once per `ResourceLogs`, scope once per `ScopeLogs`.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> FlattenedRequest {
    let mut flattener = Flattener::new();
    let mut resources = Vec::with_capacity(request.resource_logs.len());
    for rl in &request.resource_logs {
        let resource = rl
            .resource
            .as_ref()
            .map(|r| flattener.flatten_resource(r))
            .unwrap_or_default();
        let mut scopes = Vec::with_capacity(rl.scope_logs.len());
        for sl in &rl.scope_logs {
            let scope = sl
                .scope
                .as_ref()
                .map(|s| flattener.flatten_scope(s))
                .unwrap_or_default();
            let records = sl
                .log_records
                .iter()
                .map(|r| flattener.flatten_record(r))
                .collect();
            scopes.push(ScopeGroup { scope, records });
        }
        resources.push(ResourceGroup { resource, scopes });
    }
    FlattenedRequest {
        tree: flattener.into_tree(),
        resources,
    }
}

/// A sampled record with its (resolved) resource + scope context, for `--print`.
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
    /// Total leaf entries across all records (one per leaf occurrence).
    pub leaves: u64,
    /// Total schema-tree nodes built across all frames (per-frame trees summed) —
    /// a measure of how compact the interned structure is vs `leaves`.
    pub tree_nodes: u64,
    /// Total log records claimed by the frame headers (`entry_count`). Equals
    /// `records` for an intact file.
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

/// Read a WAL file: decode each frame, flatten it (schema tree + entries), and
/// tally stats — recording per-phase timings into `metrics`. Returns the stats
/// plus the first `sample_records` records (resolved to paths) for inspection.
///
/// Streaming: each frame is read (`wal::Reader` decompresses LZ4 transparently),
/// decoded, and flattened before the next. Phases timed: `read`, `decode`,
/// `flatten`.
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
        stats.tree_nodes += flattened.tree.len() as u64;
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
                            resource: flattened.tree.resolve(&rg.resource),
                            scope: flattened.tree.resolve(&sg.scope),
                            own: flattened.tree.resolve(record),
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
