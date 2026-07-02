//! Signal-neutral flattening substrate shared by the `logs` and `traces` modules.
//!
//! A [`Flattener`] builds one [`SchemaTree`] (an arena of nodes interned by
//! `(parent, step, kind)`) while flattening a resource, a scope, and its records
//! or spans into it. Each leaf occurrence becomes an [`Entry`] `{ node, value }` —
//! the path is *not* stored per entry; it is recovered on demand from the tree
//! ([`SchemaTree::path`]). A node id is therefore a stable typed-column identity
//! (collapsed path + kind), shared across every row that has that column.
//!
//! This module owns everything signal-agnostic: the value model
//! ([`Kind`]/[`Value`]), the schema tree + builder, the W3C id newtypes
//! ([`TraceId`]/[`SpanId`]), the canonical `key=value` rendering ([`build_kv`])
//! that the writer (`ng-ingest`) and reader (`ng-index`) must agree on, and the
//! one bincode frame codec the per-signal `encode_log_frame`/`encode_trace_frame`
//! wrappers delegate to. The signal-specific request/record types and entry
//! points live in [`crate::logs`] and [`crate::traces`].

use std::collections::HashMap;
use std::hash::Hasher;

use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};

use opentelemetry_proto::tonic::common::v1::{
    AnyValue, InstrumentationScope, KeyValue, any_value::Value as Av,
};
use opentelemetry_proto::tonic::logs::v1::LogRecord;
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::Span;

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

    /// The node's upward edge as `(parent, step)`, or `None` at the root.
    /// Exposes the parent id + immediate step so a consumer can copy the tree
    /// node-by-node into another representation (e.g. `sfst::SchemaTree` at index
    /// time). Nodes are interned parent-before-child, so an ascending copy sees
    /// every parent already placed.
    pub fn edge(&self, id: NodeId) -> Option<(NodeId, &Step)> {
        self.nodes[id as usize]
            .edge
            .as_ref()
            .map(|e| (e.parent, &e.step))
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
    sanitized_keys: u64,
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
            sanitized_keys: 0,
        }
    }

    /// How many attribute keys were sanitized (`'='` → `'_'`, the key=value
    /// delimiter rule) so far. Callers log one aggregated warning per request
    /// when non-zero.
    pub fn sanitized_keys(&self) -> u64 {
        self.sanitized_keys
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
        out.push(Entry {
            node,
            value,
            hash: 0,
        });
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
        // '=' is the key=value delimiter everywhere downstream (the interner's
        // field split, the WAL-tail scan, `build_kv`); a key containing it
        // would inject a false field/value boundary. Every user-controlled key
        // passes through here, so rewrite '=' to '_' at this single choke
        // point — counted, so ingest can surface the rename.
        let step = Step::Field(if kv.key.contains('=') {
            self.sanitized_keys += 1;
            kv.key.replace('=', "_")
        } else {
            kv.key.clone()
        });
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
            let attrs = self.child(
                scope_node,
                Step::Field("attributes".to_string()),
                Kind::Kvlist,
            );
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
        // Per-row identifier/scalar fields are intentionally NOT emitted as entries —
        // they are carried as columns on [`Record`] (normalized/copied at ingest),
        // used for row ordering or per-row retrieval, not as indexed facets:
        // `time_unix_nano`/`observed_time_unix_nano` (→ `ts`/`observed_ts`)
        // and `trace_id`/`span_id` (near-unique identifiers). `flags` and
        // `dropped_attributes_count` are likewise carried on the record.
        if record.severity_number != 0 {
            self.scalar(
                "severity_number",
                Value::Int(record.severity_number as i64),
                &mut out,
            );
        }
        if !record.severity_text.is_empty() {
            self.scalar(
                "severity_text",
                Value::Str(record.severity_text.clone()),
                &mut out,
            );
        }
        if !record.event_name.is_empty() {
            self.scalar(
                "event_name",
                Value::Str(record.event_name.clone()),
                &mut out,
            );
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

    /// Flatten a span's own fields — the queryable scalar facets (`name`, `kind`,
    /// `status_code`) and `attributes.*`. Identifier/timing fields (`trace_id`,
    /// `span_id`, `parent_span_id`, start/duration, `flags`,
    /// `dropped_attributes_count`) are carried as per-row columns on [`crate::traces::SpanRecord`],
    /// not as entries — same split as [`Flattener::flatten_record`] for logs.
    ///
    /// Enum facets (`kind`, `status_code`) store **both** the raw OTLP int and a
    /// readable label, under a deliberate convention:
    /// - the clean name (`kind`, `status_code`) carries the user-facing **label**
    ///   (`SERVER`, `ERROR`, …) — what an operator queries;
    /// - the same name with a leading `_` (`_kind`, `_status_code`) carries the raw
    ///   **int** — lossless and forward-compatible, so an unknown future enum
    ///   variant still survives and stays queryable.
    ///
    /// The default variant is treated as absence (skipped). Only the *skip* mirrors
    /// logs' `severity_number != 0`; the dual label+raw-int *representation* is
    /// span-specific (`SpanKind`/`StatusCode` are closed enums whose readable label
    /// is worth indexing, unlike the open numeric `severity_number`). For a
    /// non-default value the raw int is always emitted; the label only when the
    /// variant is known.
    pub fn flatten_span(&mut self, span: &Span) -> Vec<Entry> {
        let mut out = Vec::new();

        if !span.name.is_empty() {
            self.scalar("name", Value::Str(span.name.clone()), &mut out);
        }

        // kind: 0 = UNSPECIFIED ⇒ absence, skip. (A consumer MAY treat UNSPECIFIED
        // as INTERNAL; we do not synthesize INTERNAL here — we emit nothing.)
        if span.kind != 0 {
            if let Some(label) = span_kind_label(span.kind) {
                self.scalar("kind", Value::Str(label.to_string()), &mut out);
            }
            self.scalar("_kind", Value::Int(span.kind as i64), &mut out);
        }

        // status.code: 0 = UNSET ⇒ absence, skip. (status.message is deferred.)
        if let Some(status) = &span.status {
            if status.code != 0 {
                if let Some(label) = status_code_label(status.code) {
                    self.scalar("status_code", Value::Str(label.to_string()), &mut out);
                }
                self.scalar("_status_code", Value::Int(status.code as i64), &mut out);
            }
        }

        if !span.attributes.is_empty() {
            let base = self.descend(ROOT, &["attributes"]);
            for kv in &span.attributes {
                self.flatten_kv(base, kv, &mut out);
            }
        }
        out
    }
}

/// Readable label for an OTLP `SpanKind`, or `None` for `UNSPECIFIED(0)` and any
/// unknown (future) variant — the caller still stores the raw int for those.
fn span_kind_label(kind: i32) -> Option<&'static str> {
    match kind {
        1 => Some("INTERNAL"),
        2 => Some("SERVER"),
        3 => Some("CLIENT"),
        4 => Some("PRODUCER"),
        5 => Some("CONSUMER"),
        _ => None,
    }
}

/// Readable label for an OTLP `Status.StatusCode`, or `None` for `UNSET(0)` and
/// any unknown variant — the caller still stores the raw int for those.
fn status_code_label(code: i32) -> Option<&'static str> {
    match code {
        1 => Some("OK"),
        2 => Some("ERROR"),
        _ => None,
    }
}

impl Default for Flattener {
    fn default() -> Self {
        Self::new()
    }
}

/// A W3C trace id: a fixed 16-byte identifier. The all-zero value is the
/// OTLP/W3C "unset/invalid" sentinel. `Copy` + inline (no per-id heap allocation,
/// unlike the raw OTLP `Vec<u8>`) and `Serialize`/`Deserialize` (it is carried in
/// the WAL frame). Mirrors `sfst::TraceId`; the `ng-index` boundary converts
/// between them (this crate has no `sfst` dependency).
#[derive(Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Default, Serialize, Deserialize)]
pub struct TraceId([u8; TRACE_ID_LEN]);

/// A W3C span id: a fixed 8-byte identifier. See [`TraceId`] for the shared
/// semantics; used for both `Span.span_id` and `Span.parent_span_id`.
#[derive(Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Default, Serialize, Deserialize)]
pub struct SpanId([u8; SPAN_ID_LEN]);

/// Generate the shared id API (`from_bytes`/`as_bytes`/`is_unset`/`UNSET`,
/// `From<[u8; W]>`, hex `Display`) for the fixed-width id newtypes. Mirrors the
/// `sfst` id types so the conversion at the `ng-index` boundary is a byte copy.
macro_rules! id_newtype {
    ($ty:ident, $width:expr) => {
        impl $ty {
            /// The all-zero "unset/invalid" id (OTLP/W3C sentinel).
            pub const UNSET: Self = Self([0u8; $width]);

            /// Parse **exactly** `$width` bytes; `None` for any other length. An
            /// empty/wrong-length id is the caller's to map (commonly
            /// `.unwrap_or_default()` → [`UNSET`](Self::UNSET)). Ingest normalization
            /// (`normalize_log_request`/`normalize_trace_ids`) already clears
            /// wrong-length ids to empty, so a conformant id round-trips.
            pub fn from_bytes(bytes: &[u8]) -> Option<Self> {
                <[u8; $width]>::try_from(bytes).ok().map(Self)
            }

            /// The raw fixed-width bytes.
            pub fn as_bytes(&self) -> &[u8; $width] {
                &self.0
            }

            /// Whether this is the all-zero unset/invalid sentinel.
            pub fn is_unset(&self) -> bool {
                self.0 == [0u8; $width]
            }
        }

        impl From<[u8; $width]> for $ty {
            fn from(bytes: [u8; $width]) -> Self {
                Self(bytes)
            }
        }

        impl std::fmt::Display for $ty {
            /// Lowercase hex, the W3C trace-context text format.
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                for b in &self.0 {
                    write!(f, "{b:02x}")?;
                }
                Ok(())
            }
        }

        impl std::fmt::Debug for $ty {
            /// Hex (via [`std::fmt::Display`]) so `{:?}`/log output is the readable W3C id.
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(f, concat!(stringify!($ty), "({})"), self)
            }
        }
    };
}

id_newtype!(TraceId, TRACE_ID_LEN);
id_newtype!(SpanId, SPAN_ID_LEN);

/// OTLP/W3C fixed id widths (`opentelemetry/proto/logs/v1/logs.proto`): a
/// `trace_id` is 16 bytes, a `span_id` is 8 bytes, each empty if unset.
pub const TRACE_ID_LEN: usize = 16;
pub const SPAN_ID_LEN: usize = 8;

/// How many malformed ids a single export request carried (wrong-length, non-empty).
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub struct MalformedIds {
    pub trace: u64,
    pub span: u64,
}

impl MalformedIds {
    pub fn any(self) -> bool {
        self.trace > 0 || self.span > 0
    }
}

/// Render a typed value into its `key=value` string form, appended to `out`:
/// strings raw, ints/doubles decimal, bools `true`/`false`, bytes lowercase hex; the
/// flatten-only empties render structurally. This is the single canonical rendering
/// shared by hash pre-computation (`fill_log_hashes`) and the SFST build, so both
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
pub(crate) fn hash_kv(path: &str, value: &Value, buf: &mut String) -> u64 {
    build_kv(path, value, buf);
    let mut h = twox_hash::XxHash64::default();
    h.write(buf.as_bytes());
    h.finish()
}

/// Bincode config for the flattened-frame payload (the standard fixed config).
fn frame_config() -> impl bincode::config::Config {
    bincode::config::standard()
}

/// Bincode-encode any flattened-frame payload `T`. The single codec the
/// per-signal `encode_*_frame` wrappers delegate to, so logs and traces ride the
/// identical [`frame_config`] — the two cannot drift apart.
pub(crate) fn encode<T: Serialize>(req: &T) -> Result<Vec<u8>, bincode::error::EncodeError> {
    bincode::serde::encode_to_vec(req, frame_config())
}

/// Bincode-decode a flattened-frame payload `T` — the counterpart to [`encode`].
pub(crate) fn decode<T: DeserializeOwned>(bytes: &[u8]) -> Result<T, bincode::error::DecodeError> {
    Ok(bincode::serde::decode_from_slice(bytes, frame_config())?.0)
}
