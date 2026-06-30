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
    use opentelemetry_proto::tonic::logs::v1::LogRecord;
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
        normalize_log_timestamps(&mut req, 1000);
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
        let bad = normalize_log_ids(&mut req);
        assert_eq!((bad.trace, bad.span), (1, 1));
        assert!(bad.any());
        let recs = &req.resource_logs[0].scope_logs[0].log_records;
        assert_eq!(recs[0].trace_id.len(), 16);
        assert_eq!(recs[0].span_id.len(), 8);
        assert!(recs[1].trace_id.is_empty());
        assert!(recs[2].trace_id.is_empty() && recs[2].span_id.is_empty());
    }
}
