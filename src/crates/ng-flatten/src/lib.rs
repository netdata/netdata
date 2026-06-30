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
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
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
    /// `dropped_attributes_count`) are carried as per-row columns on [`SpanRecord`],
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

// ---------------------------------------------------------------------------
// Flattened-frame format: the OTLP grouping of a request's entries plus its
// schema tree, as stored (bincode) in one WAL frame. Written by `ng-ingest` at
// ingest, read by `ng-index` when building the SFST.
//
// NOTE: the frame payload carries NO inner schema version — it is just the
// bincode of `FlattenedRequest`. Any change to these types (e.g. new `Record`
// fields) is a breaking change: WAL frames written by an older `ng-ingest`
// cannot be decoded by the new `ng-index` (the new fields would consume bytes
// from the old payload). There is no migration; drain/regenerate WAL files on
// upgrade. Acceptable while ng-* is pre-GA (see the project SOW).
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
            /// ([`normalize_ids`]/[`normalize_trace_ids`]) already clears
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
            /// Hex (via [`Display`]) so `{:?}`/log output is the readable W3C id.
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(f, concat!(stringify!($ty), "({})"), self)
            }
        }
    };
}

id_newtype!(TraceId, TRACE_ID_LEN);
id_newtype!(SpanId, SPAN_ID_LEN);

/// One log record: its per-row scalar fields plus its flattened entries. The frame
/// is **lossless** — every `LogRecord` field is carried here, either as a per-row
/// column (the scalars below) or as flattened `entries`. What the SFST actually
/// stores is decided later at index time (`build_sfst`), not at flatten time.
///
/// Per-row columns (identifiers/scalars, NOT FST facets):
/// - `ts`: the resolved `time_unix_nano`. The caller MUST normalize timestamps before
///   flattening (see `ng-ingest::write_request`): `time_unix_nano` else
///   `observed_time_unix_nano` else a monotonic clock. A caller that skips
///   normalization and flattens a record with `time_unix_nano == 0` gets `ts == 0`
///   (a year-1970 row) — so always normalize first.
/// - `observed_ts`: the raw `observed_time_unix_nano` (0 if unset). Carried verbatim
///   for losslessness; `ts` is the value used for row ordering.
/// - `trace_id` / `span_id`: raw OTLP bytes (empty if unset). Carried as columns, NOT
///   flattened into `entries` (see [`Flattener::flatten_record`]) — they are
///   near-unique identifiers, wrong to FST-index.
/// - `flags` / `dropped_attributes_count`: carried for losslessness (the frame keeps
///   them); whether the SFST stores them is an index-time choice.
///
/// `ts` and `observed_ts` use a saturating `u64 → i64` cast (a value past `i64::MAX`
/// clamps rather than wrapping negative).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Record {
    pub ts: i64,
    pub observed_ts: i64,
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub entries: Vec<Entry>,
}

/// A flattened **traces** request — the span analog of [`FlattenedRequest`]. One
/// shared schema tree plus the OTLP span grouping. Distinct types (not generics)
/// keep the logs path untouched; resource/scope flattening is shared.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlattenedTraceRequest {
    pub tree: SchemaTree,
    pub resources: Vec<SpanResourceGroup>,
}

/// One resource and the scope groups under it (span analog of [`ResourceGroup`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<SpanScopeGroup>,
}

/// One scope and the spans under it (span analog of [`ScopeGroup`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanScopeGroup {
    pub scope: Vec<Entry>,
    pub spans: Vec<SpanRecord>,
}

/// One span: its per-row columns plus its flattened entries (span analog of
/// [`Record`]). **Not yet lossless** — deferred (not carried): `events[]`,
/// `links[]`, `trace_state`, `status.message`, and the `dropped_events_count` /
/// `dropped_links_count` counters (the latter two land with the events/links
/// bodies, SOW Step 1b).
///
/// Per-row columns (NOT FST facets): `ts` = the resolved `start_time_unix_nano`
/// (the row-ordering key; callers MUST normalize first, see
/// [`normalize_span_timestamps`]); `duration` = `end - start` ns, clamped to 0 on
/// an unset/earlier end (see [`flatten_trace_into`]); `trace_id`/`span_id`/
/// `parent_span_id` raw OTLP bytes (empty if unset); `flags` /
/// `dropped_attributes_count` carried verbatim. There is no `observed_ts` (spans
/// have no observed time).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanRecord {
    pub ts: i64,
    pub duration: i64,
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub parent_span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
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
                    // clamps to i64::MAX rather than wrapping negative — keeps row
                    // ordering sane.
                    ts: i64::try_from(r.time_unix_nano).unwrap_or(i64::MAX),
                    observed_ts: i64::try_from(r.observed_time_unix_nano).unwrap_or(i64::MAX),
                    // Ingest normalization (normalize_ids) already cleared any
                    // wrong-length id to empty → from_bytes(empty) → UNSET.
                    trace_id: TraceId::from_bytes(&r.trace_id).unwrap_or_default(),
                    span_id: SpanId::from_bytes(&r.span_id).unwrap_or_default(),
                    flags: r.flags,
                    dropped_attributes_count: r.dropped_attributes_count,
                    entries: flattener.flatten_record(r),
                })
                .collect();
            scopes.push(ScopeGroup { scope, records });
        }
        resources.push(ResourceGroup { resource, scopes });
    }
    resources
}

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

/// Drop malformed trace/span ids at the ingest boundary. A non-empty id whose
/// length is not the spec width is **cleared** (set to absent, which the SFST
/// column later stores as the all-zero "unset/invalid" sentinel); conformant ids
/// (16/8 bytes) and absent ids (empty) pass through untouched. Returns the counts
/// so the caller can log one aggregated warning per request (avoids per-record
/// log floods).
///
/// The caller MUST run this (and [`normalize_timestamps`]) before [`flatten_request`]:
/// trace/span ids become fixed-stride per-row SFST columns, so a wrong-length id must
/// be cleared first. Shared by `ng-ingest` and the production OTel-logs ingestor.
pub fn normalize_ids(req: &mut ExportLogsServiceRequest) -> MalformedIds {
    let mut bad = MalformedIds::default();
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if !r.trace_id.is_empty() && r.trace_id.len() != TRACE_ID_LEN {
                    r.trace_id.clear();
                    bad.trace += 1;
                }
                if !r.span_id.is_empty() && r.span_id.len() != SPAN_ID_LEN {
                    r.span_id.clear();
                    bad.span += 1;
                }
            }
        }
    }
    bad
}

/// Ensure every log record has a usable `time_unix_nano` before flattening, applying
/// the OTLP single-timestamp rule at the observation point: keep `time_unix_nano` if
/// set, else fall back to `observed_time_unix_nano`, else synthesize one from
/// `fallback_base_ns` (typically the frame's ingestion timestamp).
///
/// The resolved value is written into `time_unix_nano` (not `observed_time_unix_nano`);
/// `Record.ts` is read straight from it. The synthesized fallback is
/// `fallback_base_ns + k` for the k-th timestamp-less record — strictly increasing, so
/// intra-frame ordering is preserved, **without** touching a shared clock per record
/// (the caller takes a single clock tick for the base, so concurrent ingest doesn't
/// serialize on the clock for the whole pass). These synthetic values are only used for
/// records lacking both event and observed time; they are not globally unique across
/// frames (acceptable — they tie-break deterministically). The caller MUST run this
/// before [`flatten_request`]. Shared by `ng-ingest` and the production OTel-logs ingestor.
pub fn normalize_timestamps(req: &mut ExportLogsServiceRequest, fallback_base_ns: u64) {
    let mut fallback_offset: u64 = 0;
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if r.time_unix_nano == 0 {
                    r.time_unix_nano = if r.observed_time_unix_nano != 0 {
                        r.observed_time_unix_nano
                    } else {
                        fallback_offset += 1;
                        fallback_base_ns.saturating_add(fallback_offset)
                    };
                }
            }
        }
    }
}

/// Drop malformed span ids at the ingest boundary — the traces analog of
/// [`normalize_ids`]. Clears any non-empty `trace_id`/`span_id`/`parent_span_id`
/// whose length is not the spec width (16/8/8); the SFST column later stores a
/// cleared id as the all-zero "unset" sentinel. Malformed `parent_span_id`s are
/// counted under `span` (both are 8-byte span ids). Callers MUST run this before
/// [`flatten_trace_request`].
pub fn normalize_trace_ids(req: &mut ExportTraceServiceRequest) -> MalformedIds {
    let mut bad = MalformedIds::default();
    for rs in &mut req.resource_spans {
        for ss in &mut rs.scope_spans {
            for s in &mut ss.spans {
                if !s.trace_id.is_empty() && s.trace_id.len() != TRACE_ID_LEN {
                    s.trace_id.clear();
                    bad.trace += 1;
                }
                if !s.span_id.is_empty() && s.span_id.len() != SPAN_ID_LEN {
                    s.span_id.clear();
                    bad.span += 1;
                }
                if !s.parent_span_id.is_empty() && s.parent_span_id.len() != SPAN_ID_LEN {
                    s.parent_span_id.clear();
                    bad.span += 1;
                }
            }
        }
    }
    bad
}

/// Ensure every span has a usable `start_time_unix_nano` before flattening — the
/// traces analog of [`normalize_timestamps`]. Spans have no observed-time fallback,
/// so a zero start is synthesized from `fallback_base_ns + k` (strictly increasing
/// per frame, preserving intra-frame order). The resolved value is written back into
/// `start_time_unix_nano`; `SpanRecord.ts` reads it. Callers MUST run this before
/// [`flatten_trace_request`].
pub fn normalize_span_timestamps(req: &mut ExportTraceServiceRequest, fallback_base_ns: u64) {
    let mut fallback_offset: u64 = 0;
    for rs in &mut req.resource_spans {
        for ss in &mut rs.scope_spans {
            for s in &mut ss.spans {
                if s.start_time_unix_nano == 0 {
                    fallback_offset += 1;
                    s.start_time_unix_nano = fallback_base_ns.saturating_add(fallback_offset);
                }
            }
        }
    }
}

/// Flatten a request into its own per-frame tree (convenience over [`flatten_into`])
/// — the form stored in a flattened WAL frame. Callers MUST normalize record
/// timestamps first (see [`normalize_timestamps`] / [`Record`]); a record with
/// `time_unix_nano == 0` flattens to `ts == 0`.
pub fn flatten_request(request: &ExportLogsServiceRequest) -> FlattenedRequest {
    let mut flattener = Flattener::new();
    let resources = flatten_into(&mut flattener, request);
    FlattenedRequest {
        tree: flattener.into_tree(),
        resources,
    }
}

/// Span duration in nanoseconds (`end - start`), clamped to `0` when the end time
/// is unset (`0`) or precedes the start (clock skew). Saturates a `u64` past
/// `i64::MAX`. Absolute end is recoverable as `ts + duration` only when `ts` did
/// not saturate (start ≤ `i64::MAX`).
fn span_duration(span: &Span) -> i64 {
    if span.end_time_unix_nano == 0 || span.end_time_unix_nano < span.start_time_unix_nano {
        return 0;
    }
    i64::try_from(span.end_time_unix_nano - span.start_time_unix_nano).unwrap_or(i64::MAX)
}

/// Flatten a decoded **traces** request INTO a shared [`Flattener`] (span analog of
/// [`flatten_into`]). Resource is flattened once per `ResourceSpans`, scope once per
/// `ScopeSpans`, reusing the signal-neutral [`Flattener::flatten_resource`] /
/// [`Flattener::flatten_scope`]. Each [`SpanRecord`]'s `ts` is read from
/// `start_time_unix_nano`, which the caller is expected to have normalized (see
/// [`normalize_span_timestamps`]).
pub fn flatten_trace_into(
    flattener: &mut Flattener,
    request: &ExportTraceServiceRequest,
) -> Vec<SpanResourceGroup> {
    let mut resources = Vec::with_capacity(request.resource_spans.len());
    for rs in &request.resource_spans {
        let resource = rs
            .resource
            .as_ref()
            .map(|r| flattener.flatten_resource(r))
            .unwrap_or_default();
        let mut scopes = Vec::with_capacity(rs.scope_spans.len());
        for ss in &rs.scope_spans {
            let scope = ss
                .scope
                .as_ref()
                .map(|s| flattener.flatten_scope(s))
                .unwrap_or_default();
            let spans = ss
                .spans
                .iter()
                .map(|sp| SpanRecord {
                    ts: i64::try_from(sp.start_time_unix_nano).unwrap_or(i64::MAX),
                    duration: span_duration(sp),
                    // Ingest normalization (normalize_trace_ids) already cleared any
                    // wrong-length id to empty → from_bytes(empty) → UNSET.
                    trace_id: TraceId::from_bytes(&sp.trace_id).unwrap_or_default(),
                    span_id: SpanId::from_bytes(&sp.span_id).unwrap_or_default(),
                    parent_span_id: SpanId::from_bytes(&sp.parent_span_id).unwrap_or_default(),
                    flags: sp.flags,
                    dropped_attributes_count: sp.dropped_attributes_count,
                    entries: flattener.flatten_span(sp),
                })
                .collect();
            scopes.push(SpanScopeGroup { scope, spans });
        }
        resources.push(SpanResourceGroup { resource, scopes });
    }
    resources
}

/// Flatten a traces request into its own per-frame tree (span analog of
/// [`flatten_request`]). Callers MUST normalize span timestamps + ids first (see
/// [`normalize_span_timestamps`] / [`normalize_trace_ids`]).
pub fn flatten_trace_request(request: &ExportTraceServiceRequest) -> FlattenedTraceRequest {
    let mut flattener = Flattener::new();
    let resources = flatten_trace_into(&mut flattener, request);
    FlattenedTraceRequest {
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

/// Encode a [`FlattenedTraceRequest`] to the bincode bytes stored in a traces WAL
/// frame — the span analog of [`encode_frame`], same config. (Span `Entry.hash`es
/// are left 0 unless a `fill_trace_hashes` pass runs first; that is a seal-time
/// fast-path optimization, not a frame validity requirement.)
pub fn encode_trace_frame(
    req: &FlattenedTraceRequest,
) -> Result<Vec<u8>, bincode::error::EncodeError> {
    bincode::serde::encode_to_vec(req, frame_config())
}

/// Decode a traces WAL frame's bincode payload back into a [`FlattenedTraceRequest`].
pub fn decode_trace_frame(
    bytes: &[u8],
) -> Result<FlattenedTraceRequest, bincode::error::DecodeError> {
    Ok(bincode::serde::decode_from_slice(bytes, frame_config())?.0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{ArrayValue, KeyValueList};
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Status};

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
        leaves
            .iter()
            .filter(|l| l.path == path)
            .map(|l| &l.value)
            .collect()
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
        // trace_id is a per-row column on `Record`, NOT a flattened entry.
        assert!(
            at(&leaves, "trace_id").is_empty(),
            "trace_id is a column, not a facet"
        );
        assert_eq!(at(&leaves, "attributes.str"), [&Value::Str("hello".into())]);
        assert_eq!(at(&leaves, "attributes.int"), [&Value::Int(42)]);
        assert_eq!(at(&leaves, "attributes.double"), [&Value::Double(3.5)]);
        assert_eq!(at(&leaves, "attributes.bool"), [&Value::Bool(true)]);
        assert_eq!(
            at(&leaves, "attributes.bytes"),
            [&Value::Bytes(vec![0xde, 0xad])]
        );
    }

    #[test]
    fn span_facets_carry_dual_enum_and_attributes() {
        let span = Span {
            name: "GET /x".into(),
            kind: 2, // SERVER
            trace_id: vec![0xaa; 16],
            status: Some(Status { code: 2, ..Default::default() }), // ERROR
            attributes: vec![kv("http.method", Av::StringValue("GET".into()))],
            ..Default::default()
        };
        let mut f = Flattener::new();
        let entries = f.flatten_span(&span);
        let tree = f.into_tree();
        let leaves = tree.resolve(&entries);

        assert_eq!(at(&leaves, "name"), [&Value::Str("GET /x".into())]);
        // dual representation: readable label under the clean name, raw int under `_`.
        assert_eq!(at(&leaves, "kind"), [&Value::Str("SERVER".into())]);
        assert_eq!(at(&leaves, "_kind"), [&Value::Int(2)]);
        assert_eq!(at(&leaves, "status_code"), [&Value::Str("ERROR".into())]);
        assert_eq!(at(&leaves, "_status_code"), [&Value::Int(2)]);
        assert_eq!(at(&leaves, "attributes.http.method"), [&Value::Str("GET".into())]);
        // trace_id is a per-row column on SpanRecord, not a facet.
        assert!(at(&leaves, "trace_id").is_empty(), "trace_id is a column, not a facet");
    }

    #[test]
    fn span_enum_default_skipped_unknown_keeps_raw_int() {
        // UNSPECIFIED kind (0) + UNSET status (0) → no enum facets at all.
        let mut f = Flattener::new();
        let e = f.flatten_span(&Span {
            name: "x".into(),
            kind: 0,
            status: Some(Status { code: 0, ..Default::default() }),
            ..Default::default()
        });
        let l = f.into_tree().resolve(&e);
        assert!(at(&l, "kind").is_empty() && at(&l, "_kind").is_empty());
        assert!(at(&l, "status_code").is_empty() && at(&l, "_status_code").is_empty());

        // Unknown future variant → raw int survives (forward-compat), no label.
        let mut f = Flattener::new();
        let e = f.flatten_span(&Span {
            kind: 99,
            status: Some(Status { code: 42, ..Default::default() }),
            ..Default::default()
        });
        let l = f.into_tree().resolve(&e);
        assert!(at(&l, "kind").is_empty(), "no label for an unknown kind");
        assert_eq!(at(&l, "_kind"), [&Value::Int(99)]);
        assert!(at(&l, "status_code").is_empty());
        assert_eq!(at(&l, "_status_code"), [&Value::Int(42)]);
    }

    /// Wrap one span in a single-resource/single-scope traces request.
    fn trace_req(span: Span, resource: Option<Resource>, scope: Option<InstrumentationScope>) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource,
                scope_spans: vec![ScopeSpans { scope, spans: vec![span], ..Default::default() }],
                ..Default::default()
            }],
        }
    }

    #[test]
    fn flatten_trace_request_carries_columns_and_groups() {
        let span = Span {
            trace_id: vec![0x11; 16],
            span_id: vec![0x22; 8],
            parent_span_id: vec![0x33; 8],
            flags: 0x100,
            name: "op".into(),
            kind: 3, // CLIENT
            start_time_unix_nano: 1_000,
            end_time_unix_nano: 1_500,
            dropped_attributes_count: 2,
            ..Default::default()
        };
        let req = trace_req(
            span,
            Some(Resource {
                attributes: vec![kv("service.name", Av::StringValue("api".into()))],
                ..Default::default()
            }),
            Some(InstrumentationScope { name: "lib".into(), ..Default::default() }),
        );
        let flat = flatten_trace_request(&req);
        let rg = &flat.resources[0];
        let sg = &rg.scopes[0];
        let sr = &sg.spans[0];

        // Per-row columns.
        assert_eq!(sr.ts, 1_000);
        assert_eq!(sr.duration, 500);
        assert_eq!(sr.trace_id, TraceId::from([0x11; 16]));
        assert_eq!(sr.span_id, SpanId::from([0x22; 8]));
        assert_eq!(sr.parent_span_id, SpanId::from([0x33; 8]));
        assert_eq!(sr.flags, 0x100);
        assert_eq!(sr.dropped_attributes_count, 2);

        // Resource/scope flattening is the shared (signal-neutral) path.
        assert_eq!(
            at(&flat.tree.resolve(&rg.resource), "resource.attributes.service.name"),
            [&Value::Str("api".into())]
        );
        assert_eq!(at(&flat.tree.resolve(&sg.scope), "scope.name"), [&Value::Str("lib".into())]);
        assert_eq!(at(&flat.tree.resolve(&sr.entries), "kind"), [&Value::Str("CLIENT".into())]);
    }

    #[test]
    fn span_duration_clamps_on_unset_or_skew() {
        let dur = |start, end| {
            let s = Span { start_time_unix_nano: start, end_time_unix_nano: end, ..Default::default() };
            flatten_trace_request(&trace_req(s, None, None)).resources[0].scopes[0].spans[0].duration
        };
        assert_eq!(dur(100, 250), 150);
        assert_eq!(dur(100, 0), 0, "unset end → 0");
        assert_eq!(dur(250, 100), 0, "skew (end < start) → 0");
    }

    #[test]
    fn normalize_trace_ids_and_span_timestamps() {
        let mut req = trace_req(
            Span {
                trace_id: vec![0x01, 0x02], // wrong length → cleared
                span_id: vec![0u8; 8],      // valid
                parent_span_id: vec![0x09], // wrong length → cleared
                start_time_unix_nano: 0,    // → synthesized
                ..Default::default()
            },
            None,
            None,
        );
        let bad = normalize_trace_ids(&mut req);
        assert_eq!(bad.trace, 1);
        assert_eq!(bad.span, 1, "malformed parent_span_id counted under span");
        let s = &req.resource_spans[0].scope_spans[0].spans[0];
        assert!(s.trace_id.is_empty() && s.parent_span_id.is_empty());
        assert_eq!(s.span_id.len(), 8, "valid span_id untouched");

        normalize_span_timestamps(&mut req, 5_000);
        assert_eq!(
            req.resource_spans[0].scope_spans[0].spans[0].start_time_unix_nano,
            5_001
        );
    }

    #[test]
    fn span_no_status_and_empty_name_emit_nothing() {
        // status: None (vs Some(UNSET)) and an empty name → those facets absent;
        // an unrelated facet (kind) still emits.
        let mut f = Flattener::new();
        let e = f.flatten_span(&Span { name: String::new(), kind: 2, status: None, ..Default::default() });
        let l = f.into_tree().resolve(&e);
        assert!(at(&l, "name").is_empty(), "empty name → no facet");
        assert!(
            at(&l, "status_code").is_empty() && at(&l, "_status_code").is_empty(),
            "status None → no facet"
        );
        assert_eq!(at(&l, "kind"), [&Value::Str("SERVER".into())]);
    }

    #[test]
    fn normalize_trace_ids_keeps_conformant() {
        let mut req = trace_req(
            Span {
                trace_id: vec![1u8; 16],
                span_id: vec![2u8; 8],
                parent_span_id: vec![3u8; 8],
                ..Default::default()
            },
            None,
            None,
        );
        assert_eq!(normalize_trace_ids(&mut req), MalformedIds::default(), "conformant ids untouched");
        let s = &req.resource_spans[0].scope_spans[0].spans[0];
        assert_eq!((s.trace_id.len(), s.span_id.len(), s.parent_span_id.len()), (16, 8, 8));
    }

    #[test]
    fn flatten_trace_into_shares_nodes_across_spans() {
        // The schema tree is shared, so the same (path, kind) interns to one node
        // across spans — not a per-span subtree.
        let span = |name: &str| Span {
            name: name.into(),
            attributes: vec![kv("http.method", Av::StringValue("GET".into()))],
            ..Default::default()
        };
        let req = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: None,
                scope_spans: vec![ScopeSpans {
                    scope: None,
                    spans: vec![span("a"), span("b")],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let mut f = Flattener::new();
        let groups = flatten_trace_into(&mut f, &req);
        let spans = &groups[0].scopes[0].spans;
        assert_eq!(spans.len(), 2);
        let node = |sr: &SpanRecord| {
            sr.entries.iter().find(|e| e.value == Value::Str("GET".into())).unwrap().node
        };
        assert_eq!(node(&spans[0]), node(&spans[1]), "shared (path,kind) interns once across spans");
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
        assert_eq!(
            at(&leaves, "attributes.user.name"),
            [&Value::Str("x".into())]
        );
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
            at(
                &tree.resolve(&resource_entries),
                "resource.attributes.service.name"
            ),
            [&Value::Str("svc".into())],
        );
        let scope_leaves = tree.resolve(&scope_entries);
        assert_eq!(at(&scope_leaves, "scope.name"), [&Value::Str("lib".into())]);
        assert_eq!(
            at(&scope_leaves, "scope.version"),
            [&Value::Str("1.0".into())]
        );
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
                                values: roles
                                    .into_iter()
                                    .map(|r| av(Av::StringValue(r.into())))
                                    .collect(),
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
        let a_roles: Vec<&Entry> = a
            .iter()
            .filter(|e| tree_a.path(e.node) == "attributes.user.roles[]")
            .collect();
        assert_eq!(a_roles.len(), 2, "two array elements under one node");
        assert_eq!(a_roles[0].node, a_roles[1].node, "share one local node");
        assert_eq!(
            map_a[a_roles[0].node as usize],
            map_a[a_roles[1].node as usize]
        );
        let b_roles = b
            .iter()
            .filter(|e| tree_b.path(e.node) == "attributes.user.roles[]")
            .count();
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
        assert_eq!(
            a[0].node, b[0].node,
            "severity_number must share one column node"
        );
        let tree = f.into_tree();
        assert_eq!(tree.path(a[0].node), "severity_number");
    }

    fn req_of(records: Vec<LogRecord>) -> ExportLogsServiceRequest {
        use opentelemetry_proto::tonic::logs::v1::{ResourceLogs, ScopeLogs};
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: records,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    #[test]
    fn normalize_timestamps_resolves_time_then_observed_then_base_offset() {
        let mut req = req_of(vec![
            // event time kept as-is
            LogRecord {
                time_unix_nano: 100,
                observed_time_unix_nano: 50,
                ..Default::default()
            },
            // no event time -> observed
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 77,
                ..Default::default()
            },
            // neither -> base + 1
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 0,
                ..Default::default()
            },
            // neither -> base + 2 (offset increments per fallback record)
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 0,
                ..Default::default()
            },
        ]);
        normalize_timestamps(&mut req, 1000);
        let ts: Vec<u64> = req.resource_logs[0].scope_logs[0]
            .log_records
            .iter()
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(ts, vec![100, 77, 1001, 1002]);
    }

    #[test]
    fn normalize_ids_clears_only_wrong_length() {
        let mut req = req_of(vec![
            // conformant widths kept
            LogRecord {
                trace_id: vec![9u8; 16],
                span_id: vec![9u8; 8],
                ..Default::default()
            },
            // absent kept absent
            LogRecord {
                trace_id: vec![],
                span_id: vec![],
                ..Default::default()
            },
            // wrong widths cleared
            LogRecord {
                trace_id: vec![1u8; 10],
                span_id: vec![2u8; 3],
                ..Default::default()
            },
        ]);
        let bad = normalize_ids(&mut req);
        assert_eq!((bad.trace, bad.span), (1, 1));
        assert!(bad.any());
        let recs = &req.resource_logs[0].scope_logs[0].log_records;
        assert_eq!(recs[0].trace_id.len(), 16);
        assert_eq!(recs[0].span_id.len(), 8);
        assert!(recs[1].trace_id.is_empty());
        assert!(recs[2].trace_id.is_empty() && recs[2].span_id.is_empty());
    }
}
