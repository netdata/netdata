//! A typed inverted index over the merged global schema tree.
//!
//! Each leaf column (global [`NodeId`]) is homogeneously typed, so its postings
//! pick a container by kind: `Int`/`Double` use an ordered map (exact **and**
//! range), `Str`/`Bytes` a hash map (exact; array-any is automatic since a
//! collapsed array `tags[]` is one column), `Bool` two bitmaps, and the
//! valueless kinds (`Null`/`EmptyArray`/`EmptyKvlist`) a single presence bitmap.
//! Postings map a value to the `RoaringBitmap` of record positions that hold it.
//!
//! This is the in-memory experiment (single segment, record-order positions, no
//! time-window, no tiering); it proves the typed-index thesis — numeric ranges and
//! array-element queries — that the production SFST index cannot do.

use std::cmp::Ordering;
use std::collections::{BTreeMap, HashMap};
use std::path::Path;

use bincode::serde::decode_from_slice;
use roaring::RoaringBitmap;

use crate::{
    Error, FlattenedRequest, Flattener, Kind, Metrics, NodeId, SchemaTree, Value, sole_wal_file,
    tally,
};

/// A total-order wrapper over `f64` (via [`f64::total_cmp`]) so `Double` columns
/// can key an ordered map. NaN orders consistently; adequate for this experiment.
#[derive(Debug, Clone, Copy)]
struct OrdF64(f64);

impl PartialEq for OrdF64 {
    fn eq(&self, other: &Self) -> bool {
        self.0.total_cmp(&other.0) == Ordering::Equal
    }
}
impl Eq for OrdF64 {}
impl PartialOrd for OrdF64 {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}
impl Ord for OrdF64 {
    fn cmp(&self, other: &Self) -> Ordering {
        self.0.total_cmp(&other.0)
    }
}

/// One column's postings, shaped by the column's (homogeneous) kind.
enum Column {
    Int(BTreeMap<i64, RoaringBitmap>),
    Double(BTreeMap<OrdF64, RoaringBitmap>),
    Str(HashMap<String, RoaringBitmap>),
    Bytes(HashMap<Vec<u8>, RoaringBitmap>),
    Bool { t: RoaringBitmap, f: RoaringBitmap },
    /// `Null` / `EmptyArray` / `EmptyKvlist`: the kind is the value, so presence
    /// (which records have this column at all) is all there is to record.
    Presence(RoaringBitmap),
}

impl Column {
    /// Empty postings for a leaf `kind`.
    fn for_kind(kind: Kind) -> Column {
        match kind {
            Kind::Int => Column::Int(BTreeMap::new()),
            Kind::Double => Column::Double(BTreeMap::new()),
            Kind::Str => Column::Str(HashMap::new()),
            Kind::Bytes => Column::Bytes(HashMap::new()),
            Kind::Bool => Column::Bool {
                t: RoaringBitmap::new(),
                f: RoaringBitmap::new(),
            },
            Kind::Null | Kind::EmptyArray | Kind::EmptyKvlist => Column::Presence(RoaringBitmap::new()),
            Kind::Kvlist | Kind::Array => {
                unreachable!("interior kinds are never leaf columns")
            }
        }
    }

    /// Record `pos` under `value`. `value`'s kind always matches this column's
    /// kind, since the column was created from the leaf node's kind.
    fn insert(&mut self, value: &Value, pos: u32) {
        match (self, value) {
            (Column::Int(m), Value::Int(i)) => {
                m.entry(*i).or_default().insert(pos);
            }
            (Column::Double(m), Value::Double(d)) => {
                m.entry(OrdF64(*d)).or_default().insert(pos);
            }
            (Column::Str(m), Value::Str(s)) => {
                m.entry(s.clone()).or_default().insert(pos);
            }
            (Column::Bytes(m), Value::Bytes(b)) => {
                m.entry(b.clone()).or_default().insert(pos);
            }
            (Column::Bool { t, f }, Value::Bool(b)) => {
                if *b {
                    t.insert(pos);
                } else {
                    f.insert(pos);
                };
            }
            (Column::Presence(bm), Value::Null | Value::EmptyArray | Value::EmptyKvlist) => {
                bm.insert(pos);
            }
            _ => unreachable!("column kind matches its node's leaf kind"),
        }
    }

    /// The positions recorded under `value` (empty if none / kind mismatch).
    fn get(&self, value: &Value) -> RoaringBitmap {
        match (self, value) {
            (Column::Int(m), Value::Int(i)) => m.get(i).cloned().unwrap_or_default(),
            (Column::Double(m), Value::Double(d)) => m.get(&OrdF64(*d)).cloned().unwrap_or_default(),
            (Column::Str(m), Value::Str(s)) => m.get(s).cloned().unwrap_or_default(),
            (Column::Bytes(m), Value::Bytes(b)) => m.get(b).cloned().unwrap_or_default(),
            (Column::Bool { t, f }, Value::Bool(b)) => if *b { t } else { f }.clone(),
            (Column::Presence(bm), Value::Null | Value::EmptyArray | Value::EmptyKvlist) => bm.clone(),
            _ => RoaringBitmap::new(),
        }
    }

    /// Distinct values present in this column.
    fn cardinality(&self) -> usize {
        match self {
            Column::Int(m) => m.len(),
            Column::Double(m) => m.len(),
            Column::Str(m) => m.len(),
            Column::Bytes(m) => m.len(),
            Column::Bool { t, f } => usize::from(!t.is_empty()) + usize::from(!f.is_empty()),
            Column::Presence(bm) => usize::from(!bm.is_empty()),
        }
    }

    /// The most frequent value and its record count — a representative sample for
    /// demo queries. `None` for an empty column.
    fn sample_value(&self) -> Option<(Value, u64)> {
        let pick = |it: &mut dyn Iterator<Item = (Value, u64)>| it.max_by_key(|(_, n)| *n);
        match self {
            Column::Int(m) => pick(&mut m.iter().map(|(k, b)| (Value::Int(*k), b.len()))),
            Column::Double(m) => pick(&mut m.iter().map(|(k, b)| (Value::Double(k.0), b.len()))),
            Column::Str(m) => pick(&mut m.iter().map(|(k, b)| (Value::Str(k.clone()), b.len()))),
            Column::Bytes(m) => pick(&mut m.iter().map(|(k, b)| (Value::Bytes(k.clone()), b.len()))),
            Column::Bool { t, f } => {
                pick(&mut [(Value::Bool(true), t.len()), (Value::Bool(false), f.len())].into_iter())
                    .filter(|(_, n)| *n > 0)
            }
            Column::Presence(bm) if !bm.is_empty() => Some((Value::Null, bm.len())),
            Column::Presence(_) => None,
        }
    }
}

/// Summary of one leaf column, for introspection/reporting.
#[derive(Debug, Clone)]
pub struct ColumnInfo {
    pub node: NodeId,
    pub path: String,
    pub kind: Kind,
    pub cardinality: usize,
}

/// The typed inverted index: the merged global tree plus one [`Column`] per leaf
/// node, indexed by `NodeId`.
pub struct Index {
    tree: SchemaTree,
    columns: Vec<Option<Column>>,
    records: u32,
}

impl Index {
    /// Total records indexed (the position universe).
    pub fn records(&self) -> u32 {
        self.records
    }

    pub fn tree(&self) -> &SchemaTree {
        &self.tree
    }

    fn column(&self, node: NodeId) -> Option<&Column> {
        self.columns.get(node as usize).and_then(|c| c.as_ref())
    }

    /// The leaf node whose collapsed path equals `path` (first match).
    pub fn node_for_path(&self, path: &str) -> Option<NodeId> {
        (0..self.tree.len() as NodeId)
            .find(|&id| self.tree.node(id).kind.is_leaf() && self.tree.path(id) == path)
    }

    /// All leaf columns with their path, kind, and cardinality.
    pub fn leaf_columns(&self) -> Vec<ColumnInfo> {
        (0..self.tree.len() as NodeId)
            .filter_map(|id| {
                let col = self.column(id)?;
                Some(ColumnInfo {
                    node: id,
                    path: self.tree.path(id),
                    kind: self.tree.node(id).kind,
                    cardinality: col.cardinality(),
                })
            })
            .collect()
    }

    /// A representative (most frequent) value for `node`, for demo queries.
    pub fn sample_value(&self, node: NodeId) -> Option<(Value, u64)> {
        self.column(node)?.sample_value()
    }

    /// Records where column `node` equals `value`. For a collapsed array column
    /// this is "any element equals `value`".
    pub fn eq(&self, node: NodeId, value: &Value) -> RoaringBitmap {
        self.column(node).map(|c| c.get(value)).unwrap_or_default()
    }

    /// Records where an `Int` column `node` has a value in `[lo, hi]` (the union
    /// over the ordered key range). Empty for a non-`Int` column.
    pub fn range_int(&self, node: NodeId, lo: i64, hi: i64) -> RoaringBitmap {
        let mut acc = RoaringBitmap::new();
        if let Some(Column::Int(m)) = self.column(node) {
            for (_, bm) in m.range(lo..=hi) {
                acc |= bm;
            }
        }
        acc
    }

    /// The min/max keys of an `Int` column (for choosing demo ranges).
    pub fn int_bounds(&self, node: NodeId) -> Option<(i64, i64)> {
        match self.column(node)? {
            Column::Int(m) => Some((*m.keys().next()?, *m.keys().next_back()?)),
            _ => None,
        }
    }
}

/// Incrementally builds an [`Index`] from per-frame [`FlattenedRequest`]s, merging
/// each frame's tree into one global tree and recording postings. A record's
/// columns are its own entries plus its scope's and resource's (so a resource/scope
/// value matches every record under it).
pub struct IndexBuilder {
    flattener: Flattener,
    columns: Vec<Option<Column>>,
    pos: u32,
}

impl Default for IndexBuilder {
    fn default() -> Self {
        Self::new()
    }
}

impl IndexBuilder {
    pub fn new() -> Self {
        Self {
            flattener: Flattener::new(),
            columns: Vec::new(),
            pos: 0,
        }
    }

    /// Merge one flattened frame and index every record under it.
    pub fn add(&mut self, frame: &FlattenedRequest) {
        let map = self.flattener.merge_tree(&frame.tree);
        for rg in &frame.resources {
            for sg in &rg.scopes {
                for record in &sg.records {
                    let pos = self.pos;
                    self.pos += 1;
                    for e in &rg.resource {
                        Self::insert(&mut self.columns, map[e.node as usize], &e.value, pos);
                    }
                    for e in &sg.scope {
                        Self::insert(&mut self.columns, map[e.node as usize], &e.value, pos);
                    }
                    for e in record {
                        Self::insert(&mut self.columns, map[e.node as usize], &e.value, pos);
                    }
                }
            }
        }
    }

    fn insert(columns: &mut Vec<Option<Column>>, node: NodeId, value: &Value, pos: u32) {
        let idx = node as usize;
        if idx >= columns.len() {
            columns.resize_with(idx + 1, || None);
        }
        columns[idx]
            .get_or_insert_with(|| Column::for_kind(value.kind()))
            .insert(value, pos);
    }

    pub fn finish(self) -> Index {
        Index {
            tree: self.flattener.into_tree(),
            columns: self.columns,
            records: self.pos,
        }
    }
}

/// Build the typed inverted index from the flattened WAL in `flat_dir`. Phases
/// timed: `read` / `deserialize` / `index`.
pub fn build_index(flat_dir: &Path, metrics: &Metrics) -> Result<Index, Error> {
    let path = sole_wal_file(flat_dir)?;
    let mut reader = wal::Reader::open(&path)?;
    let mut builder = IndexBuilder::new();
    let mut frames = 0u64;
    let config = bincode::config::standard();

    loop {
        let frame = {
            let _t = metrics.scope("read");
            match reader.next_frame()? {
                Some(frame) => frame,
                None => break,
            }
        };
        frames += 1;
        metrics.add_frames(1);
        metrics.add_bytes(frame.data.len() as u64);

        let flattened: FlattenedRequest = {
            let _t = metrics.scope("deserialize");
            decode_from_slice(frame.data, config)
                .map_err(|source| Error::BincodeDecode { frame: frames, source })?
                .0
        };
        metrics.add_records(tally(&flattened).0);

        let _t = metrics.scope("index");
        builder.add(&flattened);
    }

    Ok(builder.finish())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::flatten_request;

    use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, InstrumentationScope, KeyValue, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;

    fn av(v: Av) -> AnyValue {
        AnyValue { value: Some(v) }
    }
    fn kv(key: &str, v: Av) -> KeyValue {
        KeyValue {
            key: key.into(),
            value: Some(av(v)),
        }
    }
    fn bits(bm: &RoaringBitmap) -> Vec<u32> {
        bm.iter().collect()
    }

    /// One request: service "svc", three records with a shared severity, an
    /// http.method string, and a collapsed `tags` array.
    fn request() -> ExportLogsServiceRequest {
        let rec = |sev: i32, method: &str, tags: Vec<&str>| LogRecord {
            severity_number: sev,
            attributes: vec![
                kv("http.method", Av::StringValue(method.into())),
                kv(
                    "tags",
                    Av::ArrayValue(ArrayValue {
                        values: tags.into_iter().map(|t| av(Av::StringValue(t.into()))).collect(),
                    }),
                ),
            ],
            ..Default::default()
        };
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    attributes: vec![kv("service.name", Av::StringValue("svc".into()))],
                    ..Default::default()
                }),
                scope_logs: vec![ScopeLogs {
                    scope: Some(InstrumentationScope {
                        name: "lib".into(),
                        ..Default::default()
                    }),
                    log_records: vec![
                        rec(9, "GET", vec!["x", "y"]),
                        rec(9, "POST", vec!["y"]),
                        rec(17, "GET", vec!["z"]),
                    ],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn build() -> Index {
        let mut b = IndexBuilder::new();
        b.add(&flatten_request(&request()));
        b.finish()
    }

    #[test]
    fn exact_match_returns_the_right_records() {
        let idx = build();
        assert_eq!(idx.records(), 3);
        // Records 0,1 have severity 9; record 2 has 17.
        let sev = idx.node_for_path("severity_number").unwrap();
        assert_eq!(bits(&idx.eq(sev, &Value::Int(9))), [0, 1]);
        assert_eq!(bits(&idx.eq(sev, &Value::Int(17))), [2]);
        // http.method "GET" is records 0 and 2.
        let method = idx.node_for_path("attributes.http.method").unwrap();
        assert_eq!(bits(&idx.eq(method, &Value::Str("GET".into()))), [0, 2]);
    }

    #[test]
    fn array_any_matches_any_element() {
        let idx = build();
        let tags = idx.node_for_path("attributes.tags[]").unwrap();
        // "y" is an element of records 0 and 1; "x" only of 0; "z" only of 2.
        assert_eq!(bits(&idx.eq(tags, &Value::Str("y".into()))), [0, 1]);
        assert_eq!(bits(&idx.eq(tags, &Value::Str("x".into()))), [0]);
        assert_eq!(bits(&idx.eq(tags, &Value::Str("z".into()))), [2]);
    }

    #[test]
    fn numeric_range_unions_the_key_range() {
        let idx = build();
        let sev = idx.node_for_path("severity_number").unwrap();
        assert_eq!(idx.int_bounds(sev), Some((9, 17)));
        // [10, 20] excludes 9, includes 17 → record 2 only.
        assert_eq!(bits(&idx.range_int(sev, 10, 20)), [2]);
        // [9, 17] covers everything.
        assert_eq!(bits(&idx.range_int(sev, 9, 17)), [0, 1, 2]);
    }

    #[test]
    fn and_of_two_predicates_intersects() {
        let idx = build();
        let method = idx.node_for_path("attributes.http.method").unwrap();
        let sev = idx.node_for_path("severity_number").unwrap();
        let get = idx.eq(method, &Value::Str("GET".into())); // {0, 2}
        let sev9 = idx.eq(sev, &Value::Int(9)); // {0, 1}
        assert_eq!(bits(&(&get & &sev9)), [0]);
    }

    #[test]
    fn resource_value_matches_every_record_under_it() {
        let idx = build();
        let svc = idx.node_for_path("resource.attributes.service.name").unwrap();
        assert_eq!(bits(&idx.eq(svc, &Value::Str("svc".into()))), [0, 1, 2]);
    }
}
