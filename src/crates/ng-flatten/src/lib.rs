//! `ng-flatten`: flatten OTLP log/trace data into a typed schema tree + per-row
//! entries — the OTLP analogue of the JSON flattener at `~/repos/tmp/schema`.
//!
//! The crate is split by signal boundary:
//! - [`common`] — the signal-neutral substrate: the value model
//!   ([`Kind`]/[`Value`]), the [`SchemaTree`] + [`Flattener`], the W3C id newtypes
//!   ([`TraceId`]/[`SpanId`]), the canonical `key=value` rendering ([`build_kv`]),
//!   and the bincode frame codec.
//! - [`logs`] — OTel logs: [`FlattenedLogRequest`]/[`Record`], [`flatten_log_request`],
//!   the log normalizers, and the log frame codec ([`encode_log_frame`]).
//! - [`traces`] — OTel traces: [`FlattenedTraceRequest`]/[`SpanRecord`],
//!   [`flatten_trace_request`], the span normalizers, and the trace frame codec.
//!
//! A [`Flattener`] builds one [`SchemaTree`] (an arena of nodes interned by
//! `(parent, step, kind)`) while flattening a resource, a scope, and its records or
//! spans into it. Each leaf occurrence becomes an [`Entry`] `{ node, value }` — the
//! path is *not* stored per entry; it is recovered on demand from the tree
//! ([`SchemaTree::path`]). A node id is therefore a stable typed-column identity
//! (collapsed path + kind), shared across every row that has that column.
//!
//! A row's resource attributes, scope, scalar facets, body, and attributes fold
//! into one namespace with prefixes (`resource.attributes.*`, `scope.*`,
//! `attributes.*`, `body…`); array elements collapse to `[]`; every leaf keeps its
//! OTLP type. The path *string* can alias across different structures (a kvlist
//! `a:{b}` and a literal key `"a.b"` both render `a.b`), but their **nodes** differ
//! (distinct `steps`), so index by node id and display by path.
//!
//! This crate also owns the on-WAL **flattened-frame format**: the writer
//! (`ng-ingest`) and the reader (`ng-index`) share the canonical `key=value` bytes,
//! so a producer hashes exactly what the SFST builder keys on — one source of truth.

pub mod common;
pub mod logs;
pub mod traces;

pub use common::*;
pub use logs::*;
pub use traces::*;

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
    use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status};

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
        let entries = f.flatten_record(record);
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
    fn eq_in_attribute_keys_is_sanitized_and_counted() {
        // '=' is the key=value delimiter downstream; flatten_kv rewrites it
        // to '_' at every surface a user-controlled key can enter.
        let req = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    attributes: vec![kv("r=k", Av::StringValue("rv".into()))],
                    ..Default::default()
                }),
                scope_logs: vec![ScopeLogs {
                    scope: Some(InstrumentationScope {
                        attributes: vec![kv("s=k", Av::StringValue("sv".into()))],
                        ..Default::default()
                    }),
                    log_records: vec![LogRecord {
                        attributes: vec![kv("a=b", Av::StringValue("x".into()))],
                        body: Some(av(Av::KvlistValue(KeyValueList {
                            values: vec![kv("n=k=2", Av::IntValue(7))],
                        }))),
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let (flat, sanitized) = flatten_log_request(req);
        assert_eq!(sanitized, 4, "one per surface; multiple '='s count once");
        for path in [
            "resource.attributes.r_k",
            "scope.attributes.s_k",
            "attributes.a_b",
            "body.n_k_2",
        ] {
            node_for_path(&flat.tree, path);
        }
        for id in 0..flat.tree.len() as NodeId {
            let path = flat.tree.path(id);
            assert!(!path.contains('='), "'=' survived in path {path:?}");
        }

        // Traces: span attributes go through the same choke point.
        let span = Span {
            attributes: vec![kv("p=q", Av::StringValue("v".into()))],
            ..Default::default()
        };
        let (tflat, tsanitized) = flatten_trace_request(trace_req(span, None, None));
        assert_eq!(tsanitized, 1);
        node_for_path(&tflat.tree, "attributes.p_q");

        // Clean keys stay untouched and uncounted.
        let (_, zero) = flatten_log_request(ExportLogsServiceRequest::default());
        assert_eq!(zero, 0);
    }

    #[test]
    fn span_facets_carry_dual_enum_and_attributes() {
        let span = Span {
            name: "GET /x".into(),
            kind: 2, // SERVER
            trace_id: vec![0xaa; 16],
            status: Some(Status {
                code: 2,
                ..Default::default()
            }), // ERROR
            attributes: vec![kv("http.method", Av::StringValue("GET".into()))],
            ..Default::default()
        };
        let mut f = Flattener::new();
        let entries = f.flatten_span(span);
        let tree = f.into_tree();
        let leaves = tree.resolve(&entries);

        assert_eq!(at(&leaves, "name"), [&Value::Str("GET /x".into())]);
        // dual representation: readable label under the clean name, raw int under `_`.
        assert_eq!(at(&leaves, "kind"), [&Value::Str("SERVER".into())]);
        assert_eq!(at(&leaves, "_kind"), [&Value::Int(2)]);
        assert_eq!(at(&leaves, "status_code"), [&Value::Str("ERROR".into())]);
        assert_eq!(at(&leaves, "_status_code"), [&Value::Int(2)]);
        assert_eq!(
            at(&leaves, "attributes.http.method"),
            [&Value::Str("GET".into())]
        );
        // trace_id is a per-row column on SpanRecord, not a facet.
        assert!(
            at(&leaves, "trace_id").is_empty(),
            "trace_id is a column, not a facet"
        );
    }

    #[test]
    fn span_enum_default_skipped_unknown_keeps_raw_int() {
        // UNSPECIFIED kind (0) + UNSET status (0) → no enum facets at all.
        let mut f = Flattener::new();
        let e = f.flatten_span(Span {
            name: "x".into(),
            kind: 0,
            status: Some(Status {
                code: 0,
                ..Default::default()
            }),
            ..Default::default()
        });
        let l = f.into_tree().resolve(&e);
        assert!(at(&l, "kind").is_empty() && at(&l, "_kind").is_empty());
        assert!(at(&l, "status_code").is_empty() && at(&l, "_status_code").is_empty());

        // Unknown future variant → raw int survives (forward-compat), no label.
        let mut f = Flattener::new();
        let e = f.flatten_span(Span {
            kind: 99,
            status: Some(Status {
                code: 42,
                ..Default::default()
            }),
            ..Default::default()
        });
        let l = f.into_tree().resolve(&e);
        assert!(at(&l, "kind").is_empty(), "no label for an unknown kind");
        assert_eq!(at(&l, "_kind"), [&Value::Int(99)]);
        assert!(at(&l, "status_code").is_empty());
        assert_eq!(at(&l, "_status_code"), [&Value::Int(42)]);
    }

    /// Wrap one span in a single-resource/single-scope traces request.
    fn trace_req(
        span: Span,
        resource: Option<Resource>,
        scope: Option<InstrumentationScope>,
    ) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource,
                scope_spans: vec![ScopeSpans {
                    scope,
                    spans: vec![span],
                    ..Default::default()
                }],
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
            Some(InstrumentationScope {
                name: "lib".into(),
                ..Default::default()
            }),
        );
        let (flat, _) = flatten_trace_request(req);
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
            at(
                &flat.tree.resolve(&rg.resource),
                "resource.attributes.service.name"
            ),
            [&Value::Str("api".into())]
        );
        assert_eq!(
            at(&flat.tree.resolve(&sg.scope), "scope.name"),
            [&Value::Str("lib".into())]
        );
        assert_eq!(
            at(&flat.tree.resolve(&sr.entries), "kind"),
            [&Value::Str("CLIENT".into())]
        );
    }

    #[test]
    fn span_duration_clamps_on_unset_or_skew() {
        let dur = |start, end| {
            let s = Span {
                start_time_unix_nano: start,
                end_time_unix_nano: end,
                ..Default::default()
            };
            flatten_trace_request(trace_req(s, None, None)).0.resources[0].scopes[0].spans[0]
                .duration
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
        let e = f.flatten_span(Span {
            name: String::new(),
            kind: 2,
            status: None,
            ..Default::default()
        });
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
        assert_eq!(
            normalize_trace_ids(&mut req),
            MalformedIds::default(),
            "conformant ids untouched"
        );
        let s = &req.resource_spans[0].scope_spans[0].spans[0];
        assert_eq!(
            (s.trace_id.len(), s.span_id.len(), s.parent_span_id.len()),
            (16, 8, 8)
        );
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
        let groups = flatten_trace_into(&mut f, req);
        let spans = &groups[0].scopes[0].spans;
        assert_eq!(spans.len(), 2);
        let node = |sr: &SpanRecord| {
            sr.entries
                .iter()
                .find(|e| e.value == Value::Str("GET".into()))
                .unwrap()
                .node
        };
        assert_eq!(
            node(&spans[0]),
            node(&spans[1]),
            "shared (path,kind) interns once across spans"
        );
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
        let entries = f.flatten_record(record);
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
        let resource_entries = f.flatten_resource(resource);
        let scope_entries = f.flatten_scope(scope);
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
        let a = fa.flatten_record(LogRecord {
            severity_number: 9,
            attributes: vec![kv("only_a", Av::IntValue(1))],
            ..Default::default()
        });
        let tree_a = fa.into_tree();

        let mut fb = Flattener::new();
        let b = fb.flatten_record(LogRecord {
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
        let a = fa.flatten_record(record(vec![
            kv("http.method", Av::StringValue("GET".into())),
            user(7, vec!["admin", "ops"]),
        ]));
        let tree_a = fa.into_tree();

        let mut fb = Flattener::new();
        let b = fb.flatten_record(record(vec![
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
        let _ = fa.flatten_record(LogRecord {
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
        let a = f.flatten_record(make(9));
        let b = f.flatten_record(make(17));
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
        let norm = normalize_log_request(&mut req, 1000, None);
        let ts: Vec<u64> = req.resource_logs[0].scope_logs[0]
            .log_records
            .iter()
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(ts, vec![100, 77, 1001, 1002]);
        assert_eq!(norm.records, 4);
        // The frame range is min/max of the RESOLVED timestamps — the same
        // values the rows store, by construction.
        assert_eq!(norm.ts_range, Some((77, 1002)));
    }

    #[test]
    fn bounds_reject_out_of_window_records_inclusive() {
        // Inclusive window [1000, 2000] on the RESOLVED timestamp.
        let bounds = TimeBounds {
            min_ns: 1000,
            max_ns: 2000,
        };
        let mut req = req_of(vec![
            LogRecord {
                time_unix_nano: 999, // one below the lower bound -> rejected
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: 1000, // inclusive lower -> kept
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: 1500,
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: 2000, // inclusive upper -> kept
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: 2001, // one above the upper bound -> rejected
                ..Default::default()
            },
        ]);
        let norm = normalize_log_request(&mut req, 1500, Some(bounds));
        assert_eq!(norm.rejected, 2);
        assert_eq!(norm.records, 3);
        // Range and kept rows are computed over the SURVIVORS only.
        assert_eq!(norm.ts_range, Some((1000, 2000)));
        let kept: Vec<u64> = req.resource_logs[0].scope_logs[0]
            .log_records
            .iter()
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(kept, vec![1000, 1500, 2000]);
    }

    #[test]
    fn bounds_judge_resolved_timestamp_and_pass_synthesized() {
        // Window [900, 1100], "now" (fallback base) = 1000 sits inside it.
        let bounds = TimeBounds {
            min_ns: 900,
            max_ns: 1100,
        };
        let mut req = req_of(vec![
            // timestamp-less -> synthesized 1000+1 = 1001 (in window) -> kept
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 0,
                ..Default::default()
            },
            // resolves to observed 950 (in window) -> kept
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 950,
                ..Default::default()
            },
            // resolves to observed 500 (out of window) -> rejected
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 500,
                ..Default::default()
            },
        ]);
        let norm = normalize_log_request(&mut req, 1000, Some(bounds));
        assert_eq!(norm.rejected, 1);
        assert_eq!(norm.records, 2);
        let kept: Vec<u64> = req.resource_logs[0].scope_logs[0]
            .log_records
            .iter()
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(kept, vec![1001, 950]);
    }

    #[test]
    fn bounds_none_keeps_everything() {
        let mut req = req_of(vec![
            LogRecord {
                time_unix_nano: 1,
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: u64::MAX / 2,
                ..Default::default()
            },
        ]);
        let norm = normalize_log_request(&mut req, 1000, None);
        assert_eq!(norm.rejected, 0);
        assert_eq!(norm.records, 2);
    }

    #[test]
    fn bounds_reject_all_leaves_zero_records_and_no_range() {
        let bounds = TimeBounds {
            min_ns: 10_000,
            max_ns: 20_000,
        };
        let mut req = req_of(vec![
            LogRecord {
                time_unix_nano: 5,
                ..Default::default()
            },
            LogRecord {
                time_unix_nano: 30_000,
                ..Default::default()
            },
        ]);
        let norm = normalize_log_request(&mut req, 15_000, Some(bounds));
        assert_eq!(norm.rejected, 2);
        assert_eq!(norm.records, 0);
        assert_eq!(norm.ts_range, None);
        // Every record dropped → the emptied scope and its resource are pruned.
        assert!(req.resource_logs.is_empty());
    }

    #[test]
    fn bounds_keep_synthesized_with_zero_future_skew() {
        // future_skew = 0 → upper bound == now. A timestamp-less record
        // synthesizes now + k (past the bound) but is EXEMPT (server-stamped
        // "now"), so it is kept; a real client timestamp 1ns past `now` is
        // rejected; a real one exactly at `now` passes (inclusive).
        let now = 1_000_000u64;
        let bounds = TimeBounds {
            min_ns: now - 100,
            max_ns: now, // future_skew = 0
        };
        let mut req = req_of(vec![
            LogRecord {
                time_unix_nano: 0,
                observed_time_unix_nano: 0,
                ..Default::default()
            }, // synthesized now+1 -> exempt -> kept
            LogRecord {
                time_unix_nano: now + 1,
                ..Default::default()
            }, // real, 1ns future -> rejected
            LogRecord {
                time_unix_nano: now,
                ..Default::default()
            }, // real, at the boundary -> kept
        ]);
        let norm = normalize_log_request(&mut req, now, Some(bounds));
        assert_eq!(norm.rejected, 1);
        assert_eq!(norm.records, 2);
        let kept: Vec<u64> = req.resource_logs[0].scope_logs[0]
            .log_records
            .iter()
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(kept, vec![now + 1, now]); // synthesized(now+1), real boundary(now)
    }

    #[test]
    fn bounds_prune_scope_and_resource_emptied_by_drop() {
        // Two resources: one all in-window, one all out-of-window. After the
        // drop, the fully-rejected resource (and its empty scope) is pruned so
        // its attributes are never flattened into the frame.
        let bounds = TimeBounds {
            min_ns: 1_000,
            max_ns: 2_000,
        };
        let keep = req_of(vec![LogRecord {
            time_unix_nano: 1_500,
            ..Default::default()
        }])
        .resource_logs
        .remove(0);
        let dropped = req_of(vec![LogRecord {
            time_unix_nano: 9_999,
            ..Default::default()
        }])
        .resource_logs
        .remove(0);
        let mut req = ExportLogsServiceRequest {
            resource_logs: vec![keep, dropped],
        };
        let norm = normalize_log_request(&mut req, 1_500, Some(bounds));
        assert_eq!(norm.rejected, 1);
        assert_eq!(norm.records, 1);
        // The out-of-window resource is gone; only the surviving one remains.
        assert_eq!(req.resource_logs.len(), 1);
        assert_eq!(req.resource_logs[0].scope_logs[0].log_records.len(), 1);
        assert_eq!(
            req.resource_logs[0].scope_logs[0].log_records[0].time_unix_nano,
            1_500
        );
    }

    #[test]
    fn normalize_ts_range_spans_resource_logs_and_is_none_when_empty() {
        // Range aggregates across ResourceLogs/ScopeLogs boundaries.
        let mut req = ExportLogsServiceRequest {
            resource_logs: vec![
                req_of(vec![LogRecord {
                    time_unix_nano: 500,
                    ..Default::default()
                }])
                .resource_logs
                .remove(0),
                req_of(vec![
                    LogRecord {
                        time_unix_nano: 90,
                        ..Default::default()
                    },
                    LogRecord {
                        time_unix_nano: 700,
                        ..Default::default()
                    },
                ])
                .resource_logs
                .remove(0),
            ],
        };
        let norm = normalize_log_request(&mut req, 1000, None);
        assert_eq!(norm.records, 3);
        assert_eq!(norm.ts_range, Some((90, 700)));

        // No records: no range, nothing counted.
        let mut empty = ExportLogsServiceRequest::default();
        let norm = normalize_log_request(&mut empty, 1000, None);
        assert_eq!(norm.records, 0);
        assert_eq!(norm.ts_range, None);
        assert!(!norm.bad_ids.any());

        // Fallback offsets continue across ResourceLogs boundaries: the k-th
        // timestamp-less record overall gets base + k, regardless of which
        // ResourceLogs it sits in.
        let no_ts = || LogRecord::default();
        let mut req = ExportLogsServiceRequest {
            resource_logs: vec![
                req_of(vec![no_ts(), no_ts()]).resource_logs.remove(0),
                req_of(vec![no_ts()]).resource_logs.remove(0),
            ],
        };
        let norm = normalize_log_request(&mut req, 1000, None);
        let ts: Vec<u64> = req
            .resource_logs
            .iter()
            .flat_map(|rl| rl.scope_logs.iter())
            .flat_map(|sl| sl.log_records.iter())
            .map(|r| r.time_unix_nano)
            .collect();
        assert_eq!(ts, vec![1001, 1002, 1003]);
        assert_eq!(norm.ts_range, Some((1001, 1003)));
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
        let norm = normalize_log_request(&mut req, 1000, None);
        let bad = norm.bad_ids;
        assert_eq!((bad.trace, bad.span), (1, 1));
        assert!(bad.any());
        let recs = &req.resource_logs[0].scope_logs[0].log_records;
        assert_eq!(recs[0].trace_id.len(), 16);
        assert_eq!(recs[0].span_id.len(), 8);
        assert!(recs[1].trace_id.is_empty());
        assert!(recs[2].trace_id.is_empty() && recs[2].span_id.is_empty());
    }

    #[test]
    fn flatten_populates_and_dedups_entry_hashes() {
        // Two spans share `http.method=GET`; a resource attr is present too. At
        // flatten time, every entry (resource/scope/span) must carry its hash,
        // identical key=value must hash the same (enables the seal fast path), and
        // distinct key=value must differ.
        let span = |name: &str| Span {
            name: name.into(),
            attributes: vec![kv("http.method", Av::StringValue("GET".into()))],
            ..Default::default()
        };
        let req = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource {
                    attributes: vec![kv("service.name", Av::StringValue("api".into()))],
                    ..Default::default()
                }),
                scope_spans: vec![ScopeSpans {
                    scope: Some(InstrumentationScope {
                        name: "lib".into(),
                        ..Default::default()
                    }),
                    spans: vec![span("a"), span("b")],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let (flat, _) = flatten_trace_request(req);
        let rg = &flat.resources[0];
        assert!(!rg.scopes[0].scope.is_empty(), "scope entries present");
        // Every entry (resource ++ scope ++ span) carries exactly hash_kv(path,value)
        // — the emit-time contract the flattener implements. (Asserting `!= 0` would be
        // wrong: xxhash64 can legitimately produce 0.)
        let mut buf = String::new();
        for e in rg
            .resource
            .iter()
            .chain(rg.scopes[0].scope.iter())
            .chain(rg.scopes[0].spans.iter().flat_map(|s| s.entries.iter()))
        {
            assert_eq!(
                e.hash,
                crate::common::hash_kv(&flat.tree.path(e.node), &e.value, &mut buf),
                "fill sets each entry.hash = hash_kv(path, value)",
            );
        }
        let spans = &rg.scopes[0].spans;
        // `http.method=GET` interns to the same hash across both spans.
        let method_hash = |sr: &SpanRecord| {
            sr.entries
                .iter()
                .find(|e| e.value == Value::Str("GET".into()))
                .unwrap()
                .hash
        };
        assert_eq!(method_hash(&spans[0]), method_hash(&spans[1]));
        // A distinct facet (the span name "a") hashes differently.
        let name_hash = spans[0]
            .entries
            .iter()
            .find(|e| e.value == Value::Str("a".into()))
            .unwrap()
            .hash;
        assert_ne!(name_hash, method_hash(&spans[0]));
    }

    // ---- JSON-object string body rewrite (normalize_log_request) ----

    /// Normalize (so the JSON-object-string body rewrite runs) then flatten a
    /// single record with the given string body; return the normalization stats
    /// and the record's resolved leaves.
    fn flatten_string_body(body: &str) -> (LogNormalization, Vec<Leaf>) {
        let mut req = req_of(vec![LogRecord {
            body: Some(av(Av::StringValue(body.into()))),
            ..Default::default()
        }]);
        let norm = normalize_log_request(&mut req, 1000, None);
        let (flat, _) = flatten_log_request(req);
        let leaves = flat
            .tree
            .resolve(&flat.resources[0].scopes[0].records[0].entries);
        (norm, leaves)
    }

    #[test]
    fn json_object_string_body_flattens_to_typed_columns() {
        // A body string holding a JSON object explodes into typed `body.*`
        // leaves; every JSON type maps to the matching `Value`, and the raw
        // string is dropped (decision 1B).
        let body = r#"{
            "int": 7,
            "double": 3.5,
            "bool": true,
            "null": null,
            "str": "hi",
            "nested": {"inner": 1},
            "arr": [1, 2],
            "empty_obj": {},
            "empty_arr": []
        }"#;
        let (norm, leaves) = flatten_string_body(body);
        assert_eq!(norm.parsed_bodies, 1);
        // The raw string is gone — no bare `body` Str leaf survives.
        assert!(
            at(&leaves, "body").is_empty(),
            "raw string body must be dropped on successful parse"
        );
        assert_eq!(at(&leaves, "body.int"), [&Value::Int(7)]);
        assert_eq!(at(&leaves, "body.double"), [&Value::Double(3.5)]);
        assert_eq!(at(&leaves, "body.bool"), [&Value::Bool(true)]);
        assert_eq!(at(&leaves, "body.null"), [&Value::Null]);
        assert_eq!(at(&leaves, "body.str"), [&Value::Str("hi".into())]);
        assert_eq!(at(&leaves, "body.nested.inner"), [&Value::Int(1)]);
        // Array indices collapse to one `[]` element path, values in order.
        assert_eq!(at(&leaves, "body.arr[]"), [&Value::Int(1), &Value::Int(2)]);
        assert_eq!(at(&leaves, "body.empty_obj"), [&Value::EmptyKvlist]);
        assert_eq!(at(&leaves, "body.empty_arr"), [&Value::EmptyArray]);
    }

    #[test]
    fn empty_object_string_body_is_an_empty_kvlist_leaf() {
        // `{}` qualifies (it IS an object) and rewrites to a single empty-kvlist
        // leaf at `body`; no raw string remains.
        let (norm, leaves) = flatten_string_body("{}");
        assert_eq!(norm.parsed_bodies, 1);
        assert_eq!(at(&leaves, "body"), [&Value::EmptyKvlist]);
    }

    #[test]
    fn non_object_and_non_json_string_bodies_stay_verbatim() {
        // Every case that is not a JSON object stays a single verbatim `body`
        // Str leaf and is not counted. Numbers/bools/arrays/null fail the brace
        // pre-check; `{not json}` passes the pre-check but fails to parse; a
        // string with leading text before `{` fails the pre-check.
        for body in [
            "hello world", // plain non-JSON
            "42",          // parses to a number
            "true",        // parses to a bool
            "[1, 2]",      // parses to an array
            "null",        // parses to null
            "\"quoted\"",  // parses to a bare string
            "{not json}",  // passes brace pre-check, fails to parse
            "log: {\"a\": 1}", // leading text — fails the pre-check
        ] {
            let (norm, leaves) = flatten_string_body(body);
            assert_eq!(norm.parsed_bodies, 0, "body {body:?} must not be parsed");
            assert_eq!(
                at(&leaves, "body"),
                [&Value::Str(body.to_string())],
                "body {body:?} must stay a verbatim Str leaf"
            );
        }
    }

    #[test]
    fn whitespace_padded_json_object_body_is_parsed() {
        // The guard trims before the brace pre-check, so padding does not block
        // the rewrite; the padded raw string is dropped.
        let (norm, leaves) = flatten_string_body("  \n {\"a\": 1} \t ");
        assert_eq!(norm.parsed_bodies, 1);
        assert!(at(&leaves, "body").is_empty(), "padded raw string dropped");
        assert_eq!(at(&leaves, "body.a"), [&Value::Int(1)]);
    }

    #[test]
    fn no_recursive_reparse_of_string_values_inside_the_object() {
        // A string VALUE that itself looks like JSON stays a StringValue leaf —
        // only the top-level body string is parsed (decision 2A).
        let (norm, leaves) = flatten_string_body(r#"{"nested": "{\"x\": 1}"}"#);
        assert_eq!(norm.parsed_bodies, 1);
        assert_eq!(
            at(&leaves, "body.nested"),
            [&Value::Str("{\"x\": 1}".into())],
            "inner JSON-looking string must NOT be re-parsed"
        );
        // The inner string was not exploded into columns.
        assert!(at(&leaves, "body.nested.x").is_empty());
    }

    #[test]
    fn number_edges_map_to_int_or_double() {
        // i64 range stays Int (including i64::MAX and negatives); a u64 past
        // i64::MAX becomes a Double (decision 4B).
        let body = format!(
            r#"{{"max_i64": {}, "neg": -5, "over_i64": {}}}"#,
            i64::MAX,
            u64::MAX
        );
        let (norm, leaves) = flatten_string_body(&body);
        assert_eq!(norm.parsed_bodies, 1);
        assert_eq!(at(&leaves, "body.max_i64"), [&Value::Int(i64::MAX)]);
        assert_eq!(at(&leaves, "body.neg"), [&Value::Int(-5)]);
        assert_eq!(
            at(&leaves, "body.over_i64"),
            [&Value::Double(u64::MAX as f64)],
            "u64 > i64::MAX maps to Double"
        );
    }

    #[test]
    fn parsed_bodies_counts_only_rewritten_object_bodies() {
        // Across a request, only string bodies that are JSON objects are
        // rewritten and counted; other bodies (non-object strings, non-string
        // AnyValues, absent) are left untouched.
        let mut req = req_of(vec![
            LogRecord {
                body: Some(av(Av::StringValue(r#"{"a": 1}"#.into()))),
                ..Default::default()
            },
            LogRecord {
                body: Some(av(Av::StringValue(r#"{"b": 2}"#.into()))),
                ..Default::default()
            },
            LogRecord {
                body: Some(av(Av::StringValue("plain".into()))),
                ..Default::default()
            },
            LogRecord {
                body: Some(av(Av::StringValue("[1, 2]".into()))),
                ..Default::default()
            },
            // Non-string body (already structured) is never touched.
            LogRecord {
                body: Some(av(Av::IntValue(5))),
                ..Default::default()
            },
            // Absent body.
            LogRecord::default(),
        ]);
        let norm = normalize_log_request(&mut req, 1000, None);
        assert_eq!(norm.records, 6);
        assert_eq!(norm.parsed_bodies, 2);
        // Spot-check that a rewritten body flattened to a typed column.
        let (flat, _) = flatten_log_request(req);
        let leaves = flat
            .tree
            .resolve(&flat.resources[0].scopes[0].records[0].entries);
        assert_eq!(at(&leaves, "body.a"), [&Value::Int(1)]);
    }
}
