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

use std::collections::HashMap;

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
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
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
#[derive(Debug, Clone, PartialEq)]
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
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum Step {
    Field(String),
    ArrayElem,
}

#[derive(Debug, Clone)]
struct Edge {
    parent: NodeId,
    step: Step,
}

/// A schema-tree node: its [`Kind`] plus the upward edge to its parent (`None`
/// only at the root).
#[derive(Debug, Clone)]
pub struct Node {
    pub kind: Kind,
    edge: Option<Edge>,
}

/// One leaf occurrence: the schema node it belongs to and its value.
#[derive(Debug, Clone, PartialEq)]
pub struct Entry {
    pub node: NodeId,
    pub value: Value,
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
#[derive(Debug, Clone)]
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
        out.push(Entry { node, value });
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
        if record.time_unix_nano != 0 {
            self.scalar("time_unix_nano", Value::Int(record.time_unix_nano as i64), &mut out);
        }
        if record.observed_time_unix_nano != 0 {
            self.scalar(
                "observed_time_unix_nano",
                Value::Int(record.observed_time_unix_nano as i64),
                &mut out,
            );
        }
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
