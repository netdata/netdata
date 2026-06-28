//! `ng-flatten`: flatten OTLP log data into a typed schema tree + per-record
//! entries — the OTLP analogue of the JSON flattener at `~/repos/tmp/schema`.
//!
//! A [`Flattener`] builds one [`SchemaTree`] (an arena of nodes interned by
//! `(parent, step, kind)`) while flattening a resource, a scope, and its records
//! into it. Each leaf occurrence becomes an [`Entry`] `{ node, value }` — the path
//! is *not* stored per entry; it is recovered on demand from the tree
//! ([`SchemaTree::path`]). A node id is therefore a stable typed-column identity
//! (collapsed path + kind), shared across every record that has that column.
//!
//! v1 is the OTLP value model only (no JSON-body parsing). A record's resource
//! attributes, scope, scalar fields, body, and log attributes fold into one
//! namespace with prefixes (`resource.attributes.*`, `scope.*`, `attributes.*`,
//! `body…`); array elements collapse to `[]`; every leaf keeps its OTLP type.
//!
//! Note: the path *string* can alias across different structures (a kvlist
//! `a:{b}` and a literal key `"a.b"` both render `a.b`), but their **nodes**
//! differ (distinct `steps`), so index by node id and display by path.
//!
//! This crate also owns the on-WAL **flattened-frame format**
//! ([`FlattenedRequest`] with [`encode_frame`]/[`decode_frame`]) and the canonical
//! `key=value` rendering ([`build_kv`]) shared by the writer (`ng-ingest`) and the
//! reader (`ng-index`): the bytes a producer hashes MUST match the bytes the SFST
//! builder keys on, so there is a single source of truth here.

use std::collections::HashMap;
use std::hash::Hasher;

use serde::{Deserialize, Serialize};

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, InstrumentationScope, KeyValue, any_value::Value as Av,
};
use opentelemetry_proto::tonic::logs::v1::LogRecord;
use opentelemetry_proto::tonic::resource::v1::Resource;

/// A node's identity within a [`SchemaTree`] — its arena index.
pub type NodeId = u32;

const ROOT: NodeId = 0;

/// The kind (type tag) of a schema-tree node. Leaf kinds carry a value; the
/// interior kinds [`Kind::Kvlist`]/[`Kind::Array`] have children instead.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum Kind {
    Null,
    Bool,
    Int,
    Double,
    Str,
    Bytes,
    EmptyKvlist,
    EmptyArray,
    Kvlist,
    Array,
}

impl Kind {
    /// True for value-bearing leaf kinds (everything but `Kvlist`/`Array`).
    pub fn is_leaf(self) -> bool {
        !matches!(self, Kind::Kvlist | Kind::Array)
    }
}

/// A flattened leaf value carrying its concrete OTLP-typed payload.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum Value {
    Null,
    Bool(bool),
    Int(i64),
    Double(f64),
    Str(String),
    Bytes(Vec<u8>),
    EmptyArray,
    EmptyKvlist,
}

impl Value {
    pub fn kind(&self) -> Kind {
        match self {
            Value::Null => Kind::Null,
            Value::Bool(_) => Kind::Bool,
            Value::Int(_) => Kind::Int,
            Value::Double(_) => Kind::Double,
            Value::Str(_) => Kind::Str,
            Value::Bytes(_) => Kind::Bytes,
            Value::EmptyArray => Kind::EmptyArray,
            Value::EmptyKvlist => Kind::EmptyKvlist,
        }
    }
}

/// How a node descends from its parent: a named field, or the merged array
/// element. Distinguishing these is what makes a path unambiguous (an array index
/// vs a key literally containing `[]`).
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum Step {
    Field(String),
    ArrayElem,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct Edge {
    parent: NodeId,
    step: Step,
}

/// A schema-tree node: its [`Kind`] plus the upward edge to its parent (`None`
/// only at the root).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    pub kind: Kind,
    edge: Option<Edge>,
}

/// One leaf occurrence: the schema node it belongs to and its value.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Entry {
    pub node: NodeId,
    pub value: Value,
    /// Pre-computed `xxhash64` of this entry's `key=value` rendering, filled by the
    /// index-feeding pipeline (`0` until then). Lets an SFST-style interner skip
    /// re-hashing on every occurrence — the `_nd_kv_hash` fast path.
    pub hash: u64,
}

/// A leaf resolved to its (collapsed) path string and value — the display form
/// of an [`Entry`]. See [`SchemaTree::resolve`].
#[derive(Debug, Clone, PartialEq)]
pub struct Leaf {
    pub path: String,
    pub value: Value,
}

/// The merged structure of the records flattened into it: an arena of [`Node`]s,
/// sized by structural variety, not data volume. Node id 0 is the root.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SchemaTree {
    nodes: Vec<Node>,
}

impl SchemaTree {
    pub fn node(&self, id: NodeId) -> &Node {
        &self.nodes[id as usize]
    }

    /// Total node count (interior + leaf).
    pub fn len(&self) -> usize {
        self.nodes.len()
    }

    pub fn is_empty(&self) -> bool {
        self.nodes.is_empty()
    }

    /// Number of leaf nodes — the count of distinct typed columns.
    pub fn columns(&self) -> usize {
        self.nodes.iter().filter(|n| n.kind.is_leaf()).count()
    }

    /// The root-first chain of steps leading to `id`.
    pub fn steps(&self, id: NodeId) -> Vec<Step> {
        let mut steps = Vec::new();
        let mut cur = id;
        while let Some(edge) = &self.nodes[cur as usize].edge {
            steps.push(edge.step.clone());
            cur = edge.parent;
        }
        steps.reverse();
        steps
    }

    /// The collapsed path of `id`: field steps joined with `.`, array steps as
    /// `[]` (no leading dot).
    pub fn path(&self, id: NodeId) -> String {
        let mut path = String::new();
        for step in self.steps(id) {
            match step {
                Step::Field(name) => {
                    if !path.is_empty() {
                        path.push('.');
                    }
                    path.push_str(&name);
                }
                Step::ArrayElem => path.push_str("[]"),
            }
        }
        path
    }

    /// Resolve entries to their display [`Leaf`]s (path + value).
    pub fn resolve(&self, entries: &[Entry]) -> Vec<Leaf> {
        entries
            .iter()
            .map(|e| Leaf {
                path: self.path(e.node),
                value: e.value.clone(),
            })
            .collect()
    }
}

/// Builds a [`SchemaTree`] while flattening a frame's resource, scope, and
/// records into it — interning shared columns across all of them.
pub struct Flattener {
    nodes: Vec<Node>,
    lookup: HashMap<(NodeId, Step, Kind), NodeId>,
}

impl Flattener {
    pub fn new() -> Self {
        // Root (node 0): a container with no parent edge.
        Self {
            nodes: vec![Node {
                kind: Kind::Kvlist,
                edge: None,
            }],
            lookup: HashMap::new(),
        }
    }

    /// Finish building, yielding the structure (the building index is dropped).
    pub fn into_tree(self) -> SchemaTree {
        SchemaTree { nodes: self.nodes }
    }

    /// Merge a foreign per-frame [`SchemaTree`] into this builder's (global) tree,
    /// returning a `local → global` node-id map indexed by the foreign [`NodeId`].
    ///
    /// This is the index-time counterpart to flattening: it rebuilds one global
    /// column space from many stored per-frame trees. A non-root node always has a
    /// smaller id than its children (children are interned after their parent), so
    /// a single ascending pass sees every parent already mapped. Interning each
    /// `(global_parent, step, kind)` reuses the same logic as flattening, so a
    /// column shared across frames collapses to one global node. Callers renumber
    /// entries through the returned map — an integer lookup per entry.
    pub fn merge_tree(&mut self, foreign: &SchemaTree) -> Vec<NodeId> {
        let mut map = vec![ROOT; foreign.len()];
        for local in 1..foreign.len() as NodeId {
            let node = foreign.node(local);
            let edge = node.edge.as_ref().expect("non-root node has a parent edge");
            let global_parent = map[edge.parent as usize];
            map[local as usize] = self.child(global_parent, edge.step.clone(), node.kind);
        }
        map
    }

    /// Get-or-create the child of `parent` reached via `(step, kind)`.
    fn child(&mut self, parent: NodeId, step: Step, kind: Kind) -> NodeId {
        let key = (parent, step, kind);
        if let Some(&id) = self.lookup.get(&key) {
            return id;
        }
        let id = self.nodes.len() as NodeId;
        self.nodes.push(Node {
            kind,
            edge: Some(Edge {
                parent,
                step: key.1.clone(),
            }),
        });
        self.lookup.insert(key, id);
        id
    }

    /// Descend a chain of (interior) field segments, returning the deepest node.
    fn descend(&mut self, mut node: NodeId, segments: &[&str]) -> NodeId {
        for &seg in segments {
            node = self.child(node, Step::Field(seg.to_string()), Kind::Kvlist);
        }
        node
    }

    /// Emit a leaf entry under `parent` via `step`.
    fn emit(&mut self, parent: NodeId, step: Step, value: Value, out: &mut Vec<Entry>) {
        let node = self.child(parent, step, value.kind());
        out.push(Entry { node, value, hash: 0 });
    }

    /// Flatten one OTLP `AnyValue` reached from `parent` via `step`.
    fn flatten_value(&mut self, parent: NodeId, step: Step, av: &AnyValue, out: &mut Vec<Entry>) {
        match &av.value {
            Some(Av::StringValue(s)) => self.emit(parent, step, Value::Str(s.clone()), out),
            Some(Av::BoolValue(b)) => self.emit(parent, step, Value::Bool(*b), out),
            Some(Av::IntValue(i)) => self.emit(parent, step, Value::Int(*i), out),
            Some(Av::DoubleValue(d)) => self.emit(parent, step, Value::Double(*d), out),
            Some(Av::BytesValue(b)) => self.emit(parent, step, Value::Bytes(b.clone()), out),
            Some(Av::ArrayValue(arr)) => {
                if arr.values.is_empty() {
                    self.emit(parent, step, Value::EmptyArray, out);
                } else {
                    // Array indices collapse to one element node (`[]`).
                    let node = self.child(parent, step, Kind::Array);
                    for v in &arr.values {
                        self.flatten_value(node, Step::ArrayElem, v, out);
                    }
                }
            }
            Some(Av::KvlistValue(kvl)) => {
                if kvl.values.is_empty() {
                    self.emit(parent, step, Value::EmptyKvlist, out);
                } else {
                    let node = self.child(parent, step, Kind::Kvlist);
                    for kv in &kvl.values {
                        self.flatten_kv(node, kv, out);
                    }
                }
            }
            None => self.emit(parent, step, Value::Null, out),
        }
    }

    fn flatten_kv(&mut self, parent: NodeId, kv: &KeyValue, out: &mut Vec<Entry>) {
        let step = Step::Field(kv.key.clone());
        match &kv.value {
            Some(av) => self.flatten_value(parent, step, av, out),
            None => self.emit(parent, step, Value::Null, out),
        }
    }

    /// Flatten a resource's attributes under `resource.attributes.*`.
    pub fn flatten_resource(&mut self, resource: &Resource) -> Vec<Entry> {
        let mut out = Vec::new();
        if !resource.attributes.is_empty() {
            let base = self.descend(ROOT, &["resource", "attributes"]);
            for kv in &resource.attributes {
                self.flatten_kv(base, kv, &mut out);
            }
        }
        out
    }

    /// Flatten an instrumentation scope: `scope.name`, `scope.version`,
    /// `scope.attributes.*`.
    pub fn flatten_scope(&mut self, scope: &InstrumentationScope) -> Vec<Entry> {
        let mut out = Vec::new();
        let scope_node = self.descend(ROOT, &["scope"]);
        if !scope.name.is_empty() {
            self.emit(
                scope_node,
                Step::Field("name".to_string()),
                Value::Str(scope.name.clone()),
                &mut out,
            );
        }
        if !scope.version.is_empty() {
            self.emit(
                scope_node,
                Step::Field("version".to_string()),
                Value::Str(scope.version.clone()),
                &mut out,
            );
        }
        if !scope.attributes.is_empty() {
            let attrs = self.child(scope_node, Step::Field("attributes".to_string()), Kind::Kvlist);
            for kv in &scope.attributes {
                self.flatten_kv(attrs, kv, &mut out);
            }
        }
        out
    }

    /// Flatten a log record's own fields — scalar fields, body, and attributes
    /// (`attributes.*`). Resource/scope context is flattened separately.
    pub fn flatten_record(&mut self, record: &LogRecord) -> Vec<Entry> {
        let mut out = Vec::new();

        // Queryable scalar fields. OTLP uses 0/"" for unset → treated as absent.
        // `time_unix_nano` / `observed_time_unix_nano` are intentionally NOT emitted
        // as entries: the record's timestamp is carried in `Record.ts` (normalized at
        // ingest) — used for row ordering, not as an indexed facet (matches `wal-otap`).
        if record.severity_number != 0 {
            self.scalar("severity_number", Value::Int(record.severity_number as i64), &mut out);
        }
        if !record.severity_text.is_empty() {
            self.scalar("severity_text", Value::Str(record.severity_text.clone()), &mut out);
        }
        if !record.event_name.is_empty() {
            self.scalar("event_name", Value::Str(record.event_name.clone()), &mut out);
        }
        if !record.trace_id.is_empty() {
            self.scalar("trace_id", Value::Bytes(record.trace_id.clone()), &mut out);
        }
        if !record.span_id.is_empty() {
            self.scalar("span_id", Value::Bytes(record.span_id.clone()), &mut out);
        }

        if let Some(body) = &record.body {
            self.flatten_value(ROOT, Step::Field("body".to_string()), body, &mut out);
        }
        if !record.attributes.is_empty() {
            let base = self.descend(ROOT, &["attributes"]);
            for kv in &record.attributes {
                self.flatten_kv(base, kv, &mut out);
            }
        }
        out
    }

    /// Emit a top-level scalar record field (a leaf directly under the root).
    fn scalar(&mut self, name: &str, value: Value, out: &mut Vec<Entry>) {
        self.emit(ROOT, Step::Field(name.to_string()), value, out);
    }
}

impl Default for Flattener {
    fn default() -> Self {
        Self::new()
    }
}

// ---------------------------------------------------------------------------
// Flattened-frame format: the OTLP grouping of a request's entries plus its
// schema tree, as stored (bincode) in one WAL frame. Written by `ng-ingest` at
// ingest, read by `ng-index` when building the SFST.
// ---------------------------------------------------------------------------

/// A flattened request: one schema tree shared by all its records, plus the OTLP
/// grouping. Resource/scope are flattened once per group; records hold only their
/// own entries. Every entry's `node` indexes into `tree`. This is the payload of a
/// flattened WAL frame (bincode-encoded via [`encode_frame`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlattenedRequest {
    pub tree: SchemaTree,
    pub resources: Vec<ResourceGroup>,
}

/// One resource and the scope groups under it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<ScopeGroup>,
}

/// One scope and the records under it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ScopeGroup {
    pub scope: Vec<Entry>,
    pub records: Vec<Record>,
}

/// One log record: its own entries plus its timestamp. `ts` is the record's
/// `time_unix_nano`. The caller MUST normalize timestamps before flattening (see
/// `ng-ingest::write_request`): `time_unix_nano` else `observed_time_unix_nano` else
/// a monotonic clock. A caller that skips normalization and flattens a record with
/// `time_unix_nano == 0` gets `ts == 0` (a year-1970 row) — so always normalize
/// first. The time fields are not flattened into `entries` (see
/// [`Flattener::flatten_record`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Record {
    pub ts: i64,
    pub entries: Vec<Entry>,
}

/// Flatten one decoded request INTO a shared [`Flattener`], returning the request's
/// grouped entries. The tree stays in `flattener`, so it can span many requests.
/// Resource is flattened once per `ResourceLogs`, scope once per `ScopeLogs`.
///
/// Each [`Record`]'s `ts` is read from `time_unix_nano`, which the caller is expected
/// to have normalized so it is always set (see `ng-ingest`).
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
                .map(|r| Record {
                    // Saturating: a u64 past i64::MAX (year ~2262 / adversarial input)
                    // clamps to i64::MAX rather than wrapping negative — mirrors
                    // wal-otap's cast and keeps row ordering sane.
                    ts: i64::try_from(r.time_unix_nano).unwrap_or(i64::MAX),
                    entries: flattener.flatten_record(r),
                })
                .collect();
            scopes.push(ScopeGroup { scope, records });
        }
        resources.push(ResourceGroup { resource, scopes });
    }
    resources
}

/// Flatten a request into its own per-frame tree (convenience over [`flatten_into`])
/// — the form stored in a flattened WAL frame. Callers MUST normalize record
/// timestamps first (see [`Record`]); a record with `time_unix_nano == 0` flattens
/// to `ts == 0`.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> FlattenedRequest {
    let mut flattener = Flattener::new();
    let resources = flatten_into(&mut flattener, request);
    FlattenedRequest {
        tree: flattener.into_tree(),
        resources,
    }
}

/// Render a typed value into its `key=value` string form, appended to `out`:
/// strings raw, ints/doubles decimal, bools `true`/`false`, bytes lowercase hex; the
/// flatten-only empties render structurally. This is the single canonical rendering
/// shared by hash pre-computation ([`fill_hashes`]) and the SFST build, so both
/// agree on the exact `key=value` bytes.
pub fn append_value(value: &Value, out: &mut String) {
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
pub fn build_kv(path: &str, value: &Value, out: &mut String) {
    out.clear();
    out.push_str(path);
    out.push('=');
    append_value(value, out);
}

/// `xxhash64(path=value)` with seed 0 — the hash an SFST-style interner keys on.
fn hash_kv(path: &str, value: &Value, buf: &mut String) -> u64 {
    build_kv(path, value, buf);
    let mut h = twox_hash::XxHash64::default();
    h.write(buf.as_bytes());
    h.finish()
}

/// Fill every entry's `hash` with `xxhash64(key=value)` so the index build can ride
/// the interner's `lookup_hash` fast path instead of re-hashing per occurrence.
/// Paths are resolved once per node.
pub fn fill_hashes(flattened: &mut FlattenedRequest) {
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
                for e in &mut record.entries {
                    e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
                }
            }
        }
    }
}

/// Bincode config for the flattened-frame payload (the standard fixed config).
fn frame_config() -> impl bincode::config::Config {
    bincode::config::standard()
}

/// Encode a [`FlattenedRequest`] to the bincode bytes stored in a WAL frame.
pub fn encode_frame(req: &FlattenedRequest) -> Result<Vec<u8>, bincode::error::EncodeError> {
    bincode::serde::encode_to_vec(req, frame_config())
}

/// Decode a flattened WAL frame's bincode payload back into a [`FlattenedRequest`].
pub fn decode_frame(bytes: &[u8]) -> Result<FlattenedRequest, bincode::error::DecodeError> {
    Ok(bincode::serde::decode_from_slice(bytes, frame_config())?.0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{ArrayValue, KeyValueList};

    fn av(v: Av) -> AnyValue {
        AnyValue { value: Some(v) }
    }
    fn kv(key: &str, v: Av) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(av(v)),
        }
    }
    /// All values at `path`, in document order (handles array-collapsed dups).
    fn at<'a>(leaves: &'a [Leaf], path: &str) -> Vec<&'a Value> {
        leaves.iter().filter(|l| l.path == path).map(|l| &l.value).collect()
    }

    /// The id of the first node whose collapsed path equals `path` (paths in the
    /// test data are unambiguous).
    fn node_for_path(tree: &SchemaTree, path: &str) -> NodeId {
        (0..tree.len() as NodeId)
            .find(|&id| tree.path(id) == path)
            .unwrap_or_else(|| panic!("no node for path {path:?}"))
    }

    #[test]
    fn record_fields_and_attrs_keep_their_types() {
        let record = LogRecord {
            severity_number: 9,
            severity_text: "INFO".into(),
            trace_id: vec![0xaa, 0xbb],
            attributes: vec![
                kv("str", Av::StringValue("hello".into())),
                kv("int", Av::IntValue(42)),
                kv("double", Av::DoubleValue(3.5)),
                kv("bool", Av::BoolValue(true)),
                kv("bytes", Av::BytesValue(vec![0xde, 0xad])),
            ],
            ..Default::default()
        };

        let mut f = Flattener::new();
        let entries = f.flatten_record(&record);
        let tree = f.into_tree();
        let leaves = tree.resolve(&entries);

        assert_eq!(at(&leaves, "severity_number"), [&Value::Int(9)]);
        assert_eq!(at(&leaves, "severity_text"), [&Value::Str("INFO".into())]);
        assert_eq!(at(&leaves, "trace_id"), [&Value::Bytes(vec![0xaa, 0xbb])]);
        assert_eq!(at(&leaves, "attributes.str"), [&Value::Str("hello".into())]);
        assert_eq!(at(&leaves, "attributes.int"), [&Value::Int(42)]);
        assert_eq!(at(&leaves, "attributes.double"), [&Value::Double(3.5)]);
        assert_eq!(at(&leaves, "attributes.bool"), [&Value::Bool(true)]);
        assert_eq!(at(&leaves, "attributes.bytes"), [&Value::Bytes(vec![0xde, 0xad])]);
    }

    #[test]
    fn nested_kvlist_flattens_and_array_collapses() {
        let record = LogRecord {
            attributes: vec![
                kv(
                    "user",
                    Av::KvlistValue(KeyValueList {
                        values: vec![
                            kv("id", Av::IntValue(7)),
                            kv("name", Av::StringValue("x".into())),
                        ],
                    }),
                ),
                kv(
                    "tags",
                    Av::ArrayValue(ArrayValue {
                        values: vec![
                            av(Av::StringValue("a".into())),
                            av(Av::StringValue("b".into())),
                        ],
                    }),
                ),
            ],
            ..Default::default()
        };

        let mut f = Flattener::new();
        let entries = f.flatten_record(&record);
        let tree = f.into_tree();
        let leaves = tree.resolve(&entries);

        assert_eq!(at(&leaves, "attributes.user.id"), [&Value::Int(7)]);
        assert_eq!(at(&leaves, "attributes.user.name"), [&Value::Str("x".into())]);
        // Array indices collapse to `[]`: both elements at one path/node, in order.
        assert_eq!(
            at(&leaves, "attributes.tags[]"),
            [&Value::Str("a".into()), &Value::Str("b".into())],
        );
    }

    #[test]
    fn resource_and_scope_levels() {
        let resource = Resource {
            attributes: vec![kv("service.name", Av::StringValue("svc".into()))],
            ..Default::default()
        };
        let scope = InstrumentationScope {
            name: "lib".into(),
            version: "1.0".into(),
            ..Default::default()
        };

        let mut f = Flattener::new();
        let resource_entries = f.flatten_resource(&resource);
        let scope_entries = f.flatten_scope(&scope);
        let tree = f.into_tree();

        assert_eq!(
            at(&tree.resolve(&resource_entries), "resource.attributes.service.name"),
            [&Value::Str("svc".into())],
        );
        let scope_leaves = tree.resolve(&scope_entries);
        assert_eq!(at(&scope_leaves, "scope.name"), [&Value::Str("lib".into())]);
        assert_eq!(at(&scope_leaves, "scope.version"), [&Value::Str("1.0".into())]);
    }

    #[test]
    fn merge_tree_unifies_columns_across_frames() {
        // Two per-frame trees: an overlapping column (severity_number) plus a
        // distinct attribute each. Merging both into one global builder must yield
        // a single column space where the shared column is ONE node and the
        // distinct ones survive — the index-time global-tree rebuild.
        let mut fa = Flattener::new();
        let a = fa.flatten_record(&LogRecord {
            severity_number: 9,
            attributes: vec![kv("only_a", Av::IntValue(1))],
            ..Default::default()
        });
        let tree_a = fa.into_tree();

        let mut fb = Flattener::new();
        let b = fb.flatten_record(&LogRecord {
            severity_number: 17,
            attributes: vec![kv("only_b", Av::StringValue("x".into()))],
            ..Default::default()
        });
        let tree_b = fb.into_tree();

        let mut global = Flattener::new();
        let map_a = global.merge_tree(&tree_a);
        let map_b = global.merge_tree(&tree_b);
        let gtree = global.into_tree();

        // severity_number is each record's first entry; it must map to the SAME
        // global node from both frames.
        let sev_a = map_a[a[0].node as usize];
        let sev_b = map_b[b[0].node as usize];
        assert_eq!(sev_a, sev_b, "shared column must merge to one global node");
        assert_eq!(gtree.path(sev_a), "severity_number");

        // The distinct attributes keep their own global nodes and paths.
        let only_a = map_a[a[1].node as usize];
        let only_b = map_b[b[1].node as usize];
        assert_ne!(only_a, only_b);
        assert_eq!(gtree.path(only_a), "attributes.only_a");
        assert_eq!(gtree.path(only_b), "attributes.only_b");
    }

    #[test]
    fn merge_tree_handles_arrays_nesting_and_schema_variety() {
        // Two frames share a nested `user` kvlist with a `roles` array but diverge
        // elsewhere (A: http.method, B: region). The merge must unify the shared
        // structural columns — including the nested-kvlist leaves and the
        // array-collapsed element — and keep the divergent ones distinct.
        let record = |attrs: Vec<KeyValue>| LogRecord {
            attributes: attrs,
            ..Default::default()
        };
        let user = |id: i64, roles: Vec<&str>| {
            kv(
                "user",
                Av::KvlistValue(KeyValueList {
                    values: vec![
                        kv("id", Av::IntValue(id)),
                        kv(
                            "roles",
                            Av::ArrayValue(ArrayValue {
                                values: roles.into_iter().map(|r| av(Av::StringValue(r.into()))).collect(),
                            }),
                        ),
                    ],
                }),
            )
        };

        let mut fa = Flattener::new();
        let a = fa.flatten_record(&record(vec![
            kv("http.method", Av::StringValue("GET".into())),
            user(7, vec!["admin", "ops"]),
        ]));
        let tree_a = fa.into_tree();

        let mut fb = Flattener::new();
        let b = fb.flatten_record(&record(vec![
            kv("region", Av::StringValue("eu".into())),
            user(42, vec!["viewer"]),
        ]));
        let tree_b = fb.into_tree();

        let mut global = Flattener::new();
        let map_a = global.merge_tree(&tree_a);
        let map_b = global.merge_tree(&tree_b);
        let gtree = global.into_tree();

        // Shared nested + array columns collapse to ONE global node from both frames.
        for path in ["attributes.user.id", "attributes.user.roles[]"] {
            let ga = map_a[node_for_path(&tree_a, path) as usize];
            let gb = map_b[node_for_path(&tree_b, path) as usize];
            assert_eq!(ga, gb, "{path} must merge to one global node");
            assert_eq!(gtree.path(ga), path);
        }
        // The shared interior kvlist node `attributes.user` also unifies.
        assert_eq!(
            map_a[node_for_path(&tree_a, "attributes.user") as usize],
            map_b[node_for_path(&tree_b, "attributes.user") as usize],
        );

        // Divergent columns stay distinct and keep their paths.
        let method = map_a[node_for_path(&tree_a, "attributes.http.method") as usize];
        let region = map_b[node_for_path(&tree_b, "attributes.region") as usize];
        assert_ne!(method, region);
        assert_eq!(gtree.path(method), "attributes.http.method");
        assert_eq!(gtree.path(region), "attributes.region");

        // Array-collapse survives the merge: frame A's two role values share one
        // local node, which maps to a single global `[]` column.
        let a_roles: Vec<&Entry> =
            a.iter().filter(|e| tree_a.path(e.node) == "attributes.user.roles[]").collect();
        assert_eq!(a_roles.len(), 2, "two array elements under one node");
        assert_eq!(a_roles[0].node, a_roles[1].node, "share one local node");
        assert_eq!(map_a[a_roles[0].node as usize], map_a[a_roles[1].node as usize]);
        let b_roles = b.iter().filter(|e| tree_b.path(e.node) == "attributes.user.roles[]").count();
        assert_eq!(b_roles, 1);

        // Global column space is the union: http.method (A) + region (B) +
        // {user.id, user.roles[]} (shared) = 4 leaf columns.
        assert_eq!(gtree.columns(), 4);
    }

    #[test]
    fn merge_tree_handles_empty_containers() {
        // Empty array / empty kvlist are their own leaf kinds; they must survive the
        // merge as distinct columns carrying the right kind.
        let mut fa = Flattener::new();
        let _ = fa.flatten_record(&LogRecord {
            attributes: vec![
                kv("empty_arr", Av::ArrayValue(ArrayValue { values: vec![] })),
                kv("empty_kv", Av::KvlistValue(KeyValueList { values: vec![] })),
            ],
            ..Default::default()
        });
        let tree_a = fa.into_tree();

        let mut global = Flattener::new();
        let map = global.merge_tree(&tree_a);
        let gtree = global.into_tree();

        let arr = map[node_for_path(&tree_a, "attributes.empty_arr") as usize];
        let kvn = map[node_for_path(&tree_a, "attributes.empty_kv") as usize];
        assert_eq!(gtree.node(arr).kind, Kind::EmptyArray);
        assert_eq!(gtree.node(kvn).kind, Kind::EmptyKvlist);
    }

    #[test]
    fn shared_column_node_across_records() {
        // The same (path, kind) interns to one node across records.
        let make = |sev: i32| LogRecord {
            severity_number: sev,
            ..Default::default()
        };
        let mut f = Flattener::new();
        let a = f.flatten_record(&make(9));
        let b = f.flatten_record(&make(17));
        assert_eq!(a[0].node, b[0].node, "severity_number must share one column node");
        let tree = f.into_tree();
        assert_eq!(tree.path(a[0].node), "severity_number");
    }
}
