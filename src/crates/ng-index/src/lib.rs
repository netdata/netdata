//! `ng-index`: the experiment harness over a WAL of OTLP logs. Two stages, each a
//! CLI mode (see `main.rs`):
//!
//! - **`convert_wal`** reads the protobuf WAL written by `ng-ingest`, flattens each
//!   frame into a per-frame schema tree + entries (via `ng-flatten`), pre-computes
//!   each entry's `xxhash64(key=value)`, bincode-encodes the result, and appends it
//!   to a new *flattened WAL*. This stands in for flattening at the ingestor: the
//!   on-disk frame already holds the flattened form.
//! - **`build_sfst`** (see [`sfst_build`]) reads the flattened WAL and feeds the
//!   typed, array-collapsed entries into the existing `sfst-indexer` to emit a
//!   standard SFST index file — the augment-SFST path.
//!
//! Independent of `ng-ingest`: both stages go through the `wal` crate.

use std::hash::Hasher;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use file_registry::{ByteSize, TimestampNs};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

mod perf;
mod sfst_build;
pub use perf::{Metrics, Rss, read_rss};
pub use sfst_build::{SfstStats, build_sfst};

// Re-export the flattening vocabulary so the binary (and any consumer) gets it
// from `ng-index` without depending on `ng-flatten` directly.
pub use ng_flatten::{Entry, Flattener, Kind, Leaf, NodeId, SchemaTree, Value};

/// Every frame goes to one logical stream → one WAL file. The WAL treats
/// `part_key` as opaque; a single constant keeps everything in one file.
const PART_KEY: u64 = 0;
/// Opaque signal axis stamped into the flattened WAL's file name (logs == 0).
const PIPELINE_ID: u16 = 0;

/// A flattened frame: one schema tree shared by all its records, plus the OTLP
/// grouping. Resource/scope are flattened once per group; records hold only their
/// own entries. Every entry's `node` indexes into `tree`. This is what a flattened
/// WAL frame stores (bincode-encoded).
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct FlattenedRequest {
    pub tree: SchemaTree,
    pub resources: Vec<ResourceGroup>,
}

/// One resource and the scope groups under it.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<ScopeGroup>,
}

/// One scope and the records under it (each record = its own entries only).
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ScopeGroup {
    pub scope: Vec<Entry>,
    pub records: Vec<Vec<Entry>>,
}

/// Flatten one decoded request INTO a shared [`Flattener`], returning the
/// request's grouped entries. The tree stays in `flattener`, so it can span many
/// requests. Resource is flattened once per `ResourceLogs`, scope once per
/// `ScopeLogs`.
pub fn flatten_into(
    flattener: &mut Flattener,
    request: &ExportLogsServiceRequest,
) -> Vec<ResourceGroup> {
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
    resources
}

/// Flatten a request into its own per-frame tree (convenience over
/// [`flatten_into`]) — the form stored at phase-1 conversion time.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> FlattenedRequest {
    let mut flattener = Flattener::new();
    let resources = flatten_into(&mut flattener, request);
    FlattenedRequest {
        tree: flattener.into_tree(),
        resources,
    }
}

/// Stats from `convert_wal` (protobuf WAL → flattened WAL).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ConvertStats {
    /// Frames read (and re-written flattened).
    pub frames: u64,
    /// Log records decoded.
    pub records: u64,
    /// Leaf entries produced (one per leaf occurrence).
    pub leaves: u64,
    /// Records claimed by the source frame headers (`entry_count`).
    pub header_records: u64,
}

impl ConvertStats {
    /// Whether the decoded record count agrees with the source frame headers.
    pub fn consistent(&self) -> bool {
        self.records == self.header_records
    }
}

/// Errors reading, decoding, or (de)serializing a WAL.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("wal error: {0}")]
    Wal(#[from] wal::Error),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
    #[error("no .wal file found in {0}")]
    NoWal(PathBuf),
    #[error("multiple .wal files in {0}; expected exactly one")]
    MultipleWal(PathBuf),
    #[error("frame {frame}: OTLP decode failed: {source}")]
    Decode {
        frame: u64,
        source: prost::DecodeError,
    },
    #[error("frame {frame}: bincode encode failed: {source}")]
    Encode {
        frame: u64,
        source: bincode::error::EncodeError,
    },
    #[error("frame {frame}: bincode decode failed: {source}")]
    BincodeDecode {
        frame: u64,
        source: bincode::error::DecodeError,
    },
    #[error("sfst build failed: {0}")]
    Sfst(#[from] sfst_indexer::IndexError),
}

/// One WAL file, no rotation, frames always LZ4-compressed — same shape as
/// `ng-ingest`'s, so the flattened WAL is a faithful stand-in for an ingestor that
/// stores the flattened form.
fn flat_wal_config() -> wal::Config {
    wal::Config {
        rotation: wal::RotationConfig {
            max_log_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    }
}

/// The single `.wal` file inside `dir` (the flattened WAL written by phase 1).
fn sole_wal_file(dir: &Path) -> Result<PathBuf, Error> {
    let mut found = None;
    for entry in std::fs::read_dir(dir)? {
        let path = entry?.path();
        if path.extension().is_some_and(|x| x == "wal") {
            if found.is_some() {
                return Err(Error::MultipleWal(dir.to_path_buf()));
            }
            found = Some(path);
        }
    }
    found.ok_or_else(|| Error::NoWal(dir.to_path_buf()))
}

/// Leaf/record totals for a flattened request — resource and scope counted once
/// per group, records once each.
fn tally(flattened: &FlattenedRequest) -> (u64, u64) {
    let mut records = 0u64;
    let mut leaves = 0u64;
    for rg in &flattened.resources {
        leaves += rg.resource.len() as u64;
        for sg in &rg.scopes {
            leaves += sg.scope.len() as u64;
            for record in &sg.records {
                records += 1;
                leaves += record.len() as u64;
            }
        }
    }
    (records, leaves)
}

/// Render a typed value into its SFST string form, appended to `out`: strings raw,
/// ints/doubles decimal, bools `true`/`false`, bytes lowercase hex; the flatten-only
/// empties render structurally. Shared by hash pre-computation and the SFST build so
/// both agree on the exact `key=value` bytes.
pub(crate) fn append_value(value: &Value, out: &mut String) {
    use std::fmt::Write as _;
    match value {
        Value::Null => {}
        Value::Bool(b) => out.push_str(if *b { "true" } else { "false" }),
        Value::Int(i) => {
            let _ = write!(out, "{i}");
        }
        Value::Double(d) => {
            let _ = write!(out, "{d}");
        }
        Value::Str(s) => out.push_str(s),
        Value::Bytes(b) => {
            for byte in b {
                let _ = write!(out, "{byte:02x}");
            }
        }
        Value::EmptyArray => out.push_str("[]"),
        Value::EmptyKvlist => out.push_str("{}"),
    }
}

/// Build `path=value` into `out` (cleared first).
pub(crate) fn build_kv(path: &str, value: &Value, out: &mut String) {
    out.clear();
    out.push_str(path);
    out.push('=');
    append_value(value, out);
}

/// `xxhash64(path=value)` with seed 0 — the hash `sfst-indexer`'s interner keys on.
fn hash_kv(path: &str, value: &Value, buf: &mut String) -> u64 {
    build_kv(path, value, buf);
    let mut h = twox_hash::XxHash64::default();
    h.write(buf.as_bytes());
    h.finish()
}

/// Fill every entry's `hash` with `xxhash64(key=value)` so the index build can ride
/// the interner's `lookup_hash` fast path instead of re-hashing per occurrence.
/// Paths are resolved once per node.
fn fill_hashes(flattened: &mut FlattenedRequest) {
    let paths: Vec<String> = {
        let tree = &flattened.tree;
        (0..tree.len() as NodeId).map(|id| tree.path(id)).collect()
    };
    let mut buf = String::new();
    for rg in &mut flattened.resources {
        for e in &mut rg.resource {
            e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
        }
        for sg in &mut rg.scopes {
            for e in &mut sg.scope {
                e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
            }
            for record in &mut sg.records {
                for e in record {
                    e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
                }
            }
        }
    }
}

/// Phase 1 — flatten the protobuf WAL at `in_path` into a flattened WAL in
/// `flat_dir`.
///
/// Per frame: prost-decode → [`flatten_request`] → bincode-encode → append to the
/// flattened WAL (carrying the source frame's ingestion timestamp + entry count).
/// Streaming: each frame is processed and dropped before the next. Phases timed:
/// `read` / `decode` / `flatten` / `hash` / `serialize` / `write`.
pub fn convert_wal(
    in_path: &Path,
    flat_dir: &Path,
    metrics: &Metrics,
) -> Result<ConvertStats, Error> {
    let mut reader = wal::Reader::open(in_path)?;
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(flat_dir, flat_wal_config(), seq, PIPELINE_ID)?;
    let mut stats = ConvertStats::default();
    let config = bincode::config::standard();

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
        let entry_count = frame.entry_count as usize;
        let ingestion_ns = frame.timestamp_ns;
        metrics.add_frames(1);
        metrics.add_bytes(frame.data.len() as u64);

        let req = {
            let _t = metrics.scope("decode");
            ExportLogsServiceRequest::decode(frame.data).map_err(|source| Error::Decode {
                frame: stats.frames,
                source,
            })?
        };

        let mut flattened = {
            let _t = metrics.scope("flatten");
            flatten_request(&req)
        };

        {
            // Pre-compute each entry's xxhash64(key=value) so the index build rides
            // the interner's lookup_hash fast path (the _nd_kv_hash analogue).
            let _t = metrics.scope("hash");
            fill_hashes(&mut flattened);
        }

        let (records, leaves) = tally(&flattened);
        stats.records += records;
        stats.leaves += leaves;
        metrics.add_records(records);

        let bytes = {
            let _t = metrics.scope("serialize");
            bincode::serde::encode_to_vec(&flattened, config).map_err(|source| Error::Encode {
                frame: stats.frames,
                source,
            })?
        };

        let _t = metrics.scope("write");
        writer.write_frame(
            PART_KEY,
            &[],
            &bytes,
            entry_count,
            ingestion_ns,
            TimestampNs::ZERO,
            TimestampNs::ZERO,
        )?;
    }

    writer.shutdown_all()?;
    Ok(stats)
}

