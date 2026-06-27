//! Build an SFST index file from ng-flatten output — the augment-SFST path.
//!
//! Streams the flattened WAL, stringifies each typed entry into a `key=value`
//! pair (collapsed-array paths like `tags[]`, typed values rendered to strings),
//! and feeds the existing `sfst-indexer` [`RowIndex`] via the `KvSink` interface in
//! a single pass, then `build_and_write`s a standard SFST file. This reuses SFST's
//! proven build/query machinery while the keys/values now come from the typed,
//! array-collapsed flattening. (Types + a schema-tree chunk come in a later step.)

use std::fmt::Write as _;
use std::path::Path;

use bincode::serde::decode_from_slice;
use bumpalo::Bump;
use sfst_indexer::build_and_write;
use sfst_indexer::row_index::RowIndex;
use wal_otap::KvSink;

use crate::{Entry, Error, FlattenedRequest, Metrics, NodeId, SchemaTree, Value, sole_wal_file};

/// SFST cardinality-tier threshold (mirrors `sfst`'s default of 100).
const CARDINALITY_THRESHOLD: u32 = 100;

/// Stats from building an SFST from the flattened WAL.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct SfstStats {
    pub frames: u64,
    pub records: u64,
}

/// Render a typed value into SFST string form, appended to `out`. Mirrors SFST's
/// value formatting (strings raw, ints/doubles decimal, bools `true`/`false`,
/// bytes lowercase hex); the flatten-only empties render structurally.
fn stringify(value: &Value, out: &mut String) {
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

/// The leaf node id in `tree` whose collapsed path is `path` (first match).
fn leaf_node(tree: &SchemaTree, path: &str) -> Option<NodeId> {
    (0..tree.len() as NodeId).find(|&id| tree.node(id).kind.is_leaf() && tree.path(id) == path)
}

/// A record's timestamp: `time_unix_nano` (Int) if present, else
/// `observed_time_unix_nano`, else the frame's ingestion timestamp (`fallback`) —
/// the same priority `wal-otap` uses.
fn record_ts(
    record: &[Entry],
    time_node: Option<NodeId>,
    obs_node: Option<NodeId>,
    fallback: i64,
) -> i64 {
    let lookup = |node: Option<NodeId>| {
        node.and_then(|n| record.iter().find(|e| e.node == n))
            .and_then(|e| match e.value {
                Value::Int(i) => Some(i),
                _ => None,
            })
    };
    lookup(time_node).or_else(|| lookup(obs_node)).unwrap_or(fallback)
}

/// Build an SFST index file at `out_path` from the flattened WAL in `flat_dir`.
/// Single pass; phases timed: `read` / `deserialize` / `index` / `build`.
pub fn build_sfst(flat_dir: &Path, out_path: &Path, metrics: &Metrics) -> Result<SfstStats, Error> {
    let path = sole_wal_file(flat_dir)?;
    let mut reader = wal::Reader::open(&path)?;
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    let config = bincode::config::standard();
    let mut stats = SfstStats::default();
    let mut kv = String::new();

    loop {
        let frame = {
            let _t = metrics.scope("read");
            match reader.next_frame()? {
                Some(frame) => frame,
                None => break,
            }
        };
        stats.frames += 1;
        let fallback_ts = frame.timestamp_ns.as_u64() as i64;
        metrics.add_frames(1);
        metrics.add_bytes(frame.data.len() as u64);

        let flattened: FlattenedRequest = {
            let _t = metrics.scope("deserialize");
            decode_from_slice(frame.data, config)
                .map_err(|source| Error::BincodeDecode {
                    frame: stats.frames,
                    source,
                })?
                .0
        };

        let _t = metrics.scope("index");
        let tree = &flattened.tree;
        // Resolve each node's path once per frame (records reuse the same nodes).
        let paths: Vec<String> = (0..tree.len() as NodeId).map(|id| tree.path(id)).collect();
        let time_node = leaf_node(tree, "time_unix_nano");
        let obs_node = leaf_node(tree, "observed_time_unix_nano");

        let mut records = 0u64;
        for rg in &flattened.resources {
            for sg in &rg.scopes {
                for record in &sg.records {
                    records += 1;
                    let ts = record_ts(record, time_node, obs_node, fallback_ts);
                    // A record's columns = its own entries + its scope's + its resource's.
                    let mut tokens = Vec::new();
                    for e in rg.resource.iter().chain(sg.scope.iter()).chain(record.iter()) {
                        kv.clear();
                        kv.push_str(&paths[e.node as usize]);
                        kv.push('=');
                        stringify(&e.value, &mut kv);
                        tokens.push(row_index.intern(None, &kv));
                    }
                    row_index.row(ts, &tokens);
                }
            }
        }
        stats.records += records;
        metrics.add_records(records);
    }

    let _t = metrics.scope("build");
    build_and_write(&row_index, out_path)?;
    Ok(stats)
}
