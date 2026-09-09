//! Oracle for the traces seal + trace-by-id read (SOW-20260630 Step 4).
//!
//! Drives the real pipeline — `ng-ingest::write_trace_request` (flatten + fill
//! hashes + WAL) → `ng_index::build_sfst_traces_file` (seal) →
//! `sfst::IndexReader::trace_by_id` (lookup + tree). Two layers, per DECISION 13:
//!  1. hand-built fixtures pin the tree-build edge cases (missing parents, multiple
//!     roots, duplicate span ids, clock skew, large fan-out) — these can't be
//!     triggered reliably in real data;
//!  2. a `#[ignore]`d self-consistency check runs against the re-captured real WAL:
//!     independently decode it, then assert every trace reconstructs to exactly its
//!     decoded span-id set.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use file_registry::{ByteSize, MonotonicClock, TimestampNs};
use ng_index::{Metrics, build_sfst_traces_file};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, KeyValue, KeyValueList, any_value::Value as Av,
};
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};
use sfst::{IndexReader, SpanId, TraceId};

fn kv(k: &str, v: &str) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::StringValue(v.into())),
        }),
    }
}

/// A key with an arbitrary (possibly nested) OTLP value.
fn kv_any(k: &str, v: Av) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue { value: Some(v) }),
    }
}

fn span(trace: [u8; 16], id: [u8; 8], parent: [u8; 8], start: u64, end: u64, name: &str) -> Span {
    Span {
        trace_id: trace.to_vec(),
        span_id: id.to_vec(),
        parent_span_id: parent.to_vec(),
        start_time_unix_nano: start,
        end_time_unix_nano: end,
        name: name.into(),
        ..Default::default()
    }
}

/// Wrap spans in a single-resource/single-scope export request.
fn req(spans: Vec<Span>) -> ExportTraceServiceRequest {
    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes: vec![kv("service.name", "svc")],
                ..Default::default()
            }),
            scope_spans: vec![ScopeSpans {
                spans,
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

fn count_spans(req: &ExportTraceServiceRequest) -> usize {
    req.resource_spans
        .iter()
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len())
        .sum()
}

/// Ingest the requests into a traces WAL, then seal it into an SFST and return the
/// sealed bytes. Mirrors `ng-ingest::write_trace_request` inline (normalize → flatten
/// → emit-time hashes → encode → WAL frame), keeping the test self-contained; the
/// real `write_trace_request` is exercised end-to-end by the `#[ignore]`d real-WAL
/// oracle below.
fn seal(reqs: Vec<ExportTraceServiceRequest>) -> Vec<u8> {
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let config = wal::Config {
        rotation: wal::RotationConfig {
            max_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    };
    let mut writer = wal::Writer::new(
        dir.path(),
        config,
        seq,
        wal::FileStamp { pipeline_id: 1, payload_format: /* traces pipeline */
        ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT },
        wal::test_identity(),
    )
    .unwrap();
    let mut clock = MonotonicClock::new();
    for mut r in reqs {
        let count = count_spans(&r);
        if count == 0 {
            continue;
        }
        let base = clock.now_ns().as_u64();
        ng_flatten::normalize_trace_request(&mut r, base, None);
        let (flat, _) = ng_flatten::flatten_trace_request(r);
        let data = ng_flatten::encode_trace_frame(&flat).unwrap();
        let ingestion_ns = clock.now_ns();
        // The production content_meta: the version-tagged empty-ServiceStream
        // blob the unattributed stream carries (not a bare empty slice), so
        // the seal is exercised against production-shaped WAL headers.
        let content_meta =
            otel_logs_identity::encode_content_meta(&otel_logs_identity::ServiceStream::new(
                "", "",
            ))
            .expect("the empty identity always encodes");
        writer
            .write_frame(
                0,
                &content_meta,
                &data,
                wal::FrameMeta {
                    entry_count: count,
                    ingestion_ns,
                    log_ts_range: None,
                },
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();

    let wal_path = std::fs::read_dir(dir.path())
        .unwrap()
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a wal file was written");
    let out = dir.path().join("traces.sfst");
    build_sfst_traces_file(&wal_path, &out, &Metrics::new()).unwrap();
    std::fs::read(&out).unwrap()
}

#[test]
fn traces_build_refuses_logs_payload_format() {
    // The TRACES build must refuse a WAL stamped with the logs frame codec —
    // the cross-signal mixup the per-file format tag exists to catch.
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        dir.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 1,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        wal::test_identity(),
    )
    .unwrap();
    writer
        .write_frame(
            0,
            &[],
            b"x",
            wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();
    let wal_path = std::fs::read_dir(dir.path())
        .unwrap()
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .unwrap();
    match build_sfst_traces_file(&wal_path, &dir.path().join("t.sfst"), &Metrics::new()) {
        Err(ng_index::Error::PayloadFormat { found, expected }) => {
            assert_eq!(found, ng_flatten::LOG_FRAME_PAYLOAD_FORMAT);
            assert_eq!(expected, ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT);
        }
        other => panic!("expected PayloadFormat rejection, got {other:?}"),
    }
}

const ROOT_PARENT: [u8; 8] = [0u8; 8]; // unset parent = root

#[test]
fn trace_by_id_builds_linear_tree() {
    let t = [0xA1u8; 16];
    let (root, child, grand) = ([1u8; 8], [2u8; 8], [3u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, root, ROOT_PARENT, 100, 200, "root"),
        span(t, child, root, 110, 180, "child"),
        span(t, grand, child, 120, 160, "grand"),
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();

    assert_eq!(tr.spans.len(), 3);
    assert_eq!(tr.roots.len(), 1);
    // Sorted by start time: root(100), child(110), grand(120).
    assert_eq!(tr.spans[0].span_id, SpanId::from(root));
    assert_eq!(tr.spans[tr.roots[0]].span_id, SpanId::from(root));
    let kids = |sid: [u8; 8]| {
        let idx = tr
            .spans
            .iter()
            .position(|s| s.span_id == SpanId::from(sid))
            .expect("span present");
        tr.children[idx].clone()
    };
    assert_eq!(kids(root).len(), 1);
    assert_eq!(tr.spans[kids(root)[0]].span_id, SpanId::from(child));
    assert_eq!(tr.spans[kids(child)[0]].span_id, SpanId::from(grand));
    assert!(kids(grand).is_empty());
    // The `name` facet materialized onto the span.
    assert_eq!(
        tr.spans[tr.roots[0]]
            .fields
            .iter()
            .find(|(k, _)| k == "name")
            .map(|(_, v)| v.as_str()),
        Some("root"),
    );
}

#[test]
fn trace_by_id_collapses_duplicate_span_ids() {
    // A resent span (same trace_id + span_id) must collapse to one node.
    let t = [0xB2u8; 16];
    let (root, dup) = ([1u8; 8], [2u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, root, ROOT_PARENT, 100, 200, "root"),
        span(t, dup, root, 110, 150, "a"),
        span(t, dup, root, 110, 150, "a"),
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 2, "duplicate (trace_id, span_id) collapsed");
    let root_idx = tr
        .spans
        .iter()
        .position(|s| s.span_id == SpanId::from(root))
        .unwrap();
    assert_eq!(tr.children[root_idx].len(), 1);
}

#[test]
fn trace_by_id_forms_a_forest_from_orphans_and_multiple_roots() {
    // Two explicit roots (unset parent) + one orphan (parent absent from the file).
    let t = [0xC3u8; 16];
    let (r1, r2, orphan, missing) = ([1u8; 8], [2u8; 8], [3u8; 8], [9u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, r1, ROOT_PARENT, 100, 200, "root1"),
        span(t, r2, ROOT_PARENT, 105, 150, "root2"),
        span(t, orphan, missing, 110, 140, "orphan"),
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 3);
    assert_eq!(
        tr.roots.len(),
        3,
        "two unset-parent roots + one orphan-as-root"
    );
    assert!(
        tr.children.iter().all(|kids| kids.is_empty()),
        "no edges: no in-file parent has children"
    );
}

#[test]
fn trace_by_id_handles_clock_skew() {
    // Child starts before its parent (skew): sorted order puts the child first, but
    // the parent/child edge must still be built from the ids.
    let t = [0xD4u8; 16];
    let (root, child) = ([1u8; 8], [2u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, root, ROOT_PARENT, 200, 300, "root"),
        span(t, child, root, 100, 150, "child"),
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(
        tr.spans[0].span_id,
        SpanId::from(child),
        "earliest start sorts first"
    );
    assert_eq!(tr.roots.len(), 1);
    assert_eq!(tr.spans[tr.roots[0]].span_id, SpanId::from(root));
    let root_idx = tr
        .spans
        .iter()
        .position(|s| s.span_id == SpanId::from(root))
        .unwrap();
    let kids = &tr.children[root_idx];
    assert_eq!(tr.spans[kids[0]].span_id, SpanId::from(child));
}

#[test]
fn trace_by_id_handles_large_fan_out() {
    // One root with 200 direct children — the iterative build must handle wide
    // fan-out without recursion.
    let t = [0xE5u8; 16];
    let root = [1u8; 8];
    let mut spans = vec![span(t, root, ROOT_PARENT, 100, 999, "root")];
    for i in 0..200u64 {
        let sid = (i + 2).to_be_bytes(); // unique, never all-zero, never == root
        spans.push(span(t, sid, root, 100 + i, 200, "leaf"));
    }
    let bytes = seal(vec![req(spans)]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 201);
    assert_eq!(tr.roots.len(), 1);
    let root_idx = tr
        .spans
        .iter()
        .position(|s| s.span_id == SpanId::from(root))
        .unwrap();
    assert_eq!(tr.children[root_idx].len(), 200);
}

#[test]
fn trace_by_id_cycle_surfaces_all_spans_under_a_root() {
    // Pathological parent cycle A<->B: neither has an unset/absent parent, so there
    // is no natural root. The guard must still surface a root (the earliest span) so
    // no span is lost / unreachable.
    let t = [0x7cu8; 16];
    let (a, b) = ([1u8; 8], [2u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, a, b, 100, 200, "a"), // a's parent is b
        span(t, b, a, 110, 190, "b"), // b's parent is a
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 2);
    assert_eq!(
        tr.roots.len(),
        1,
        "cycle guard promotes the earliest span as a root"
    );
    assert_eq!(
        tr.spans[tr.roots[0]].span_id,
        SpanId::from(a),
        "earliest (start 100) is the root"
    );
}

#[test]
fn trace_by_id_keeps_distinct_unset_span_ids() {
    // Two spans that both lack a span_id (unset) are distinct spans, not a resend —
    // they must NOT be collapsed by the span-id dedup.
    let t = [0xafu8; 16];
    let mk = |name: &str, start: u64| Span {
        trace_id: t.to_vec(),
        span_id: vec![],        // unset
        parent_span_id: vec![], // root
        start_time_unix_nano: start,
        end_time_unix_nano: start + 10,
        name: name.into(),
        ..Default::default()
    };
    let bytes = seal(vec![req(vec![mk("a", 100), mk("b", 110)])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(
        tr.spans.len(),
        2,
        "distinct unset-span-id spans are not collapsed"
    );
    assert!(tr.spans.iter().all(|s| s.span_id == SpanId::UNSET));
    assert_eq!(tr.roots.len(), 2, "both are roots (unset parent)");
}

#[test]
fn trace_by_id_reaches_all_spans_despite_a_cyclic_component() {
    // A valid rooted pair (R->C) coexisting with a disjoint parent cycle (X<->Y).
    // `roots` is non-empty (R), so a naive "promote only when roots empty" guard
    // would leave X,Y unreachable. The reachability guard must surface them.
    let t = [0x5bu8; 16];
    let (r, c, x, y) = ([1u8; 8], [2u8; 8], [3u8; 8], [4u8; 8]);
    let bytes = seal(vec![req(vec![
        span(t, r, ROOT_PARENT, 100, 200, "root"),
        span(t, c, r, 110, 150, "child"),
        span(t, x, y, 120, 160, "x"), // x's parent is y
        span(t, y, x, 130, 170, "y"), // y's parent is x (cycle)
    ])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 4);
    // A revisit-guarded walk from the roots must reach every span.
    let mut seen: HashSet<usize> = HashSet::new();
    let mut stack: Vec<usize> = tr.roots.clone();
    while let Some(i) = stack.pop() {
        if !seen.insert(i) {
            continue;
        }
        stack.extend(tr.children[i].iter().copied());
    }
    assert_eq!(seen.len(), 4, "every span reachable from a root");
}

#[test]
fn trace_by_id_self_parent_is_a_root() {
    // A span that is its own parent must be a root (not a self-child), and carry no
    // self-edge in `children`.
    let t = [0x9eu8; 16];
    let s = [1u8; 8];
    let bytes = seal(vec![req(vec![span(t, s, s, 100, 200, "self")])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans.len(), 1);
    assert_eq!(tr.roots.len(), 1, "self-parent treated as root");
    assert!(tr.children.iter().all(|kids| kids.is_empty()), "no self-edge");
}

#[test]
fn trace_by_id_surfaces_flags_and_dropped_count() {
    // The per-row scalars (flags, dropped_attributes_count) reconstruct onto the span.
    let t = [0x8du8; 16];
    let mut s = span(t, [1u8; 8], ROOT_PARENT, 100, 200, "x");
    s.flags = 0x1;
    s.dropped_attributes_count = 3;
    let bytes = seal(vec![req(vec![s])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from(t))
        .unwrap();
    assert_eq!(tr.spans[0].flags, 0x1);
    assert_eq!(tr.spans[0].dropped_attributes_count, 3);
}

#[test]
fn trace_by_id_absent_is_empty() {
    let t = [0xF6u8; 16];
    let bytes = seal(vec![req(vec![span(
        t,
        [1u8; 8],
        ROOT_PARENT,
        100,
        200,
        "x",
    )])]);
    let tr = IndexReader::open(&bytes)
        .unwrap()
        .trace_by_id(TraceId::from([0x11u8; 16]))
        .unwrap();
    assert!(tr.spans.is_empty() && tr.roots.is_empty() && tr.children.is_empty());
}

#[test]
fn trace_by_id_separates_interleaved_traces() {
    // Two traces interleaved across one batch: each reconstructs only its own spans.
    let (ta, tb) = ([0x1au8; 16], [0x2bu8; 16]);
    let bytes = seal(vec![req(vec![
        span(ta, [1u8; 8], ROOT_PARENT, 100, 200, "a-root"),
        span(tb, [1u8; 8], ROOT_PARENT, 105, 210, "b-root"),
        span(ta, [2u8; 8], [1u8; 8], 110, 180, "a-child"),
    ])]);
    let reader = IndexReader::open(&bytes).unwrap();
    let a = reader.trace_by_id(TraceId::from(ta)).unwrap();
    let b = reader.trace_by_id(TraceId::from(tb)).unwrap();
    assert_eq!(a.spans.len(), 2);
    assert_eq!(b.spans.len(), 1);
}

/// Self-consistency oracle against the re-captured real traces WAL (DECISION 13).
/// Ignored by default (CI has no WAL); run with:
///   `cargo test -p ng-index --test traces_seal -- --ignored`
/// after re-capturing with `ng-ingest-traces`.
#[test]
#[ignore = "requires the re-captured traces WAL under $HOME/repos/tmp/ng"]
fn oracle_real_wal_self_consistency() {
    let dir = std::path::PathBuf::from(std::env::var("HOME").unwrap()).join("repos/tmp/ng");
    let wal_path = std::fs::read_dir(&dir)
        .expect("~/repos/tmp/ng exists")
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a traces wal file under ~/repos/tmp/ng");

    // Ground truth: independently decode the WAL → trace_id → set of span_ids.
    // Unset (all-zero) trace ids are not indexed, so they are excluded on both sides.
    let mut truth: HashMap<[u8; 16], HashSet<[u8; 8]>> = HashMap::new();
    let mut reader = wal::Reader::open(&wal_path).unwrap();
    while let Some(frame) = reader.next_frame().unwrap() {
        let flat = ng_flatten::decode_trace_frame(frame.data).unwrap();
        for rg in &flat.resources {
            for sg in &rg.scopes {
                for span in &sg.spans {
                    let tid = *span.trace_id.as_bytes();
                    if tid == [0u8; 16] {
                        continue;
                    }
                    truth
                        .entry(tid)
                        .or_default()
                        .insert(*span.span_id.as_bytes());
                }
            }
        }
    }
    assert!(!truth.is_empty(), "the WAL had no trace-bearing spans");

    // Seal + reopen.
    let out = tempfile::tempdir().unwrap();
    let sfst_path = out.path().join("traces.sfst");
    let (summary, _size) = build_sfst_traces_file(&wal_path, &sfst_path, &Metrics::new()).unwrap();
    let bytes = std::fs::read(&sfst_path).unwrap();
    let index = IndexReader::open(&bytes).unwrap();

    // Check a bounded, deterministic sample of traces: `trace_by_id` rebuilds the
    // reverse string table per call, so checking every distinct id would be far too
    // slow at 500K. A sorted sample of a few hundred still surfaces any systemic
    // seal/index/materialize bug (they'd fail uniformly, not per-trace).
    const SAMPLE: usize = 500;
    let mut ids: Vec<[u8; 16]> = truth.keys().copied().collect();
    ids.sort_unstable();
    let checked = ids.len().min(SAMPLE);
    for tid in &ids[..checked] {
        let tr = index.trace_by_id(TraceId::from(*tid)).unwrap();
        let got: HashSet<[u8; 8]> = tr.spans.iter().map(|s| *s.span_id.as_bytes()).collect();
        assert_eq!(
            &got,
            &truth[tid],
            "trace {} span-set mismatch",
            TraceId::from(*tid)
        );
    }
    println!(
        "oracle OK: {} spans, {} distinct traces; {} sampled traces reconstructed consistently",
        summary.record_count,
        truth.len(),
        checked,
    );
}

/// Round-trip oracle for the lossless fields (events, links,
/// trace_state, status_message, dropped counts): OTLP → WAL frame → seal →
/// `trace_by_id` returns every field, with per-event grouping/order intact,
/// while the flat `events.`/`links.` search tokens stay out of the span's
/// facet list (they are represented structurally).
#[test]
fn events_links_and_deferred_scalars_round_trip() {
    use opentelemetry_proto::tonic::trace::v1::Status;
    use opentelemetry_proto::tonic::trace::v1::span::{Event, Link};

    let trace = [0xE1u8; 16];
    let mut root = span(trace, [1; 8], [0; 8], 2_000, 2_500, "root");
    root.trace_state = "ot=th:8".into();
    root.status = Some(Status {
        code: 2,
        message: "disk full".into(),
    });
    root.dropped_events_count = 4;
    root.dropped_links_count = 1;
    root.events = vec![
        Event {
            time_unix_nano: 2_100,
            name: "exception".into(),
            attributes: vec![
                kv("exception.type", "IOError"),
                kv("exception.stacktrace", "at main()\n  at run()"),
            ],
            dropped_attributes_count: 7,
        },
        Event {
            time_unix_nano: 2_200,
            name: "retry".into(),
            attributes: vec![
                kv("policy", "backoff"),
                // Nested containers: platform fidelity = leaf tokens under the
                // collapsed paths (kvlist keys dotted, array elems as `[]`).
                kv_any(
                    "ctx",
                    Av::KvlistValue(KeyValueList {
                        values: vec![
                            kv("user", "u1"),
                            kv_any(
                                "ids",
                                Av::ArrayValue(ArrayValue {
                                    values: vec![
                                        AnyValue {
                                            value: Some(Av::IntValue(1)),
                                        },
                                        AnyValue {
                                            value: Some(Av::IntValue(2)),
                                        },
                                    ],
                                }),
                            ),
                        ],
                    }),
                ),
            ],
            dropped_attributes_count: 0,
        },
    ];
    root.links = vec![Link {
        trace_id: vec![0xB2; 16],
        span_id: vec![0xB3; 8],
        trace_state: "vendor=x".into(),
        attributes: vec![
            kv("messaging.operation", "publish"),
            kv_any(
                "route",
                Av::KvlistValue(KeyValueList {
                    values: vec![kv("queue", "q1")],
                }),
            ),
        ],
        dropped_attributes_count: 2,
        flags: 0x300,
    }];
    // A SPAN attribute deliberately named like the reserved event namespace:
    // it flattens under `attributes.events.name`, so the reader's `events.`
    // facet filter must NOT drop it.
    root.attributes.push(kv("events.name", "decoy"));
    // A second span, chronologically EARLIER but ingested after: the seal's
    // time remap reorders rows, and each span's events must stay its own.
    let mut early = span(trace, [2; 8], [1; 8], 1_000, 1_400, "child");
    early.events = vec![Event {
        time_unix_nano: 1_100,
        name: "cache-miss".into(),
        attributes: vec![],
        dropped_attributes_count: 0,
    }];

    let bytes = seal(vec![req(vec![root, early])]);
    let index = IndexReader::open(&bytes).unwrap();
    assert!(index.has_event_index(), "EVNB present");
    assert!(index.has_link_index(), "LNKB present");

    let tr = index.trace_by_id(TraceId::from(trace)).unwrap();
    assert_eq!(tr.spans.len(), 2);
    // Spans sort by start_ns: [child(1000), root(2000)].
    let child = &tr.spans[0];
    let root = &tr.spans[1];
    assert_eq!(root.span_id, SpanId::from([1; 8]));

    // Scalars (Decision 2A) are row facets.
    let field = |s: &sfst::TraceSpan, k: &str| -> Vec<String> {
        s.fields
            .iter()
            .filter(|(key, _)| key == k)
            .map(|(_, v)| v.clone())
            .collect()
    };
    assert_eq!(field(root, "trace_state"), ["ot=th:8"]);
    assert_eq!(field(root, "status_message"), ["disk full"]);
    assert_eq!(field(root, "status_code"), ["ERROR"]);

    // Structured events: grouping, order, per-event scalars, stripped attr keys.
    assert_eq!(root.dropped_events_count, 4);
    assert_eq!(root.dropped_links_count, 1);
    assert_eq!(root.events.len(), 2);
    let ev = &root.events[0];
    assert_eq!(
        (ev.time_unix_nano, ev.name.as_str(), ev.dropped_attributes_count),
        (2_100, "exception", 7)
    );
    assert_eq!(
        ev.attributes,
        vec![
            ("exception.type".to_string(), "IOError".to_string()),
            (
                "exception.stacktrace".to_string(),
                "at main()\n  at run()".to_string()
            ),
        ]
    );
    assert_eq!(root.events[1].name, "retry");
    // Nested containers regroup at platform fidelity: kvlist keys dotted,
    // array elements under the collapsed `[]` path, in original order,
    // attached to THIS event.
    assert_eq!(
        root.events[1].attributes,
        vec![
            ("policy".to_string(), "backoff".to_string()),
            ("ctx.user".to_string(), "u1".to_string()),
            ("ctx.ids[]".to_string(), "1".to_string()),
            ("ctx.ids[]".to_string(), "2".to_string()),
        ]
    );

    // Structured link: ids, verbatim trace_state, flags, dropped, attrs.
    assert_eq!(root.links.len(), 1);
    let link = &root.links[0];
    assert_eq!(link.trace_id, TraceId::from([0xB2; 16]));
    assert_eq!(link.span_id, SpanId::from([0xB3; 8]));
    assert_eq!(link.trace_state, "vendor=x");
    assert_eq!(link.flags, 0x300);
    assert_eq!(link.dropped_attributes_count, 2);
    assert_eq!(
        link.attributes,
        vec![
            ("messaging.operation".to_string(), "publish".to_string()),
            ("route.queue".to_string(), "q1".to_string()),
        ]
    );

    // The remapped earlier span kept ITS event (no cross-row bleed).
    assert_eq!(child.events.len(), 1);
    assert_eq!(child.events[0].name, "cache-miss");
    assert!(child.links.is_empty());
    assert_eq!((child.dropped_events_count, child.dropped_links_count), (0, 0));

    // Flat search tokens exist in the field table (searchable) but are excluded
    // from the reconstructed span's facet list (represented structurally).
    let fields = index.field_table();
    assert!(fields.iter().any(|f| f.name == "events.name"));
    assert!(
        fields
            .iter()
            .any(|f| f.name == "events.attributes.exception.stacktrace")
    );
    assert!(fields.iter().any(|f| f.name == "links.attributes.messaging.operation"));
    assert!(
        root.fields.iter().all(|(k, _)| !k.starts_with("events.")),
        "flat events.* tokens excluded from the structured span"
    );
    assert!(root.fields.iter().all(|(k, _)| !k.starts_with("links.")));
    // ...but a span ATTRIBUTE named like the reserved namespace lives under
    // `attributes.events.name` and must survive the filter.
    assert_eq!(field(root, "attributes.events.name"), ["decoy"]);
}

/// A corpus with no events/links (and zero dropped counts) writes neither
/// chunk — the additive-absence contract.
#[test]
fn no_events_no_links_no_chunks() {
    let trace = [0xE2u8; 16];
    let bytes = seal(vec![req(vec![span(
        trace,
        [1; 8],
        [0; 8],
        1_000,
        2_000,
        "plain",
    )])]);
    let index = IndexReader::open(&bytes).unwrap();
    assert!(!index.has_event_index());
    assert!(!index.has_link_index());
    let tr = index.trace_by_id(TraceId::from(trace)).unwrap();
    assert_eq!(tr.spans.len(), 1);
    assert!(tr.spans[0].events.is_empty());
    assert!(tr.spans[0].links.is_empty());
    assert_eq!(tr.spans[0].dropped_events_count, 0);
}

/// `Span.dropped_events_count > 0` with zero surviving events still writes the
/// chunk — the count must not silently vanish.
#[test]
fn dropped_count_alone_preserves_the_chunk() {
    let trace = [0xE3u8; 16];
    let mut s = span(trace, [1; 8], [0; 8], 1_000, 2_000, "lossy");
    s.dropped_events_count = 9;
    let bytes = seal(vec![req(vec![s])]);
    let index = IndexReader::open(&bytes).unwrap();
    assert!(index.has_event_index(), "EVNB carries the dropped count");
    assert!(!index.has_link_index());
    let tr = index.trace_by_id(TraceId::from(trace)).unwrap();
    assert!(tr.spans[0].events.is_empty());
    assert_eq!(tr.spans[0].dropped_events_count, 9);
}

/// Phase-2 bloom: the traces seal writes TBLM; membership answers have no
/// false negatives, and an absent id resolves to an empty trace via the bloom
/// pre-check (same observable result as before, cheaper path).
#[test]
fn seal_writes_trace_id_bloom() {
    let traces: Vec<[u8; 16]> = (1..=40u8).map(|i| [i; 16]).collect();
    let spans: Vec<Span> = traces
        .iter()
        .enumerate()
        .flat_map(|(i, t)| {
            let base = 1_000 + (i as u64) * 100;
            vec![
                span(*t, [1; 8], [0; 8], base, base + 50, "root"),
                span(*t, [2; 8], [1; 8], base + 10, base + 40, "child"),
            ]
        })
        .collect();
    let bytes = seal(vec![req(spans)]);
    let index = IndexReader::open(&bytes).unwrap();
    assert!(index.has_trace_id_bloom(), "TBLM present after the seal");

    let bloom = index.trace_id_bloom().unwrap();
    assert_eq!(bloom.distinct_ids(), 40);
    for t in &traces {
        assert!(bloom.might_contain(TraceId::from(*t)), "no false negatives");
        // The exact lookup still resolves the trace (bloom is a pre-check).
        assert_eq!(index.trace_by_id(TraceId::from(*t)).unwrap().spans.len(), 2);
    }

    // An absent id returns an empty trace through the bloom short-circuit.
    let absent = TraceId::from([0xEEu8; 16]);
    let tr = index.trace_by_id(absent).unwrap();
    assert!(tr.spans.is_empty() && tr.roots.is_empty());
}

// ── The trace rollup (TRSU) ─────────────────────────────────────────

#[test]
fn seal_writes_the_trace_rollup_with_honest_roots_and_stored_counts() {
    // Two traces: A has a true root (unset parent) + a child; B has NO
    // unset-parent span (a broken/partial trace) — its root columns must
    // be sentinels, never a synthesized guess. A's child span is stored
    // twice (a resend) and counts twice — stored-row semantics.
    let a_root = span([0xA; 16], [1; 8], [0; 8], 1_000, 2_000, "root-op");
    let a_child = span([0xA; 16], [2; 8], [1; 8], 1_200, 1_500, "child-op");
    let b_orphan = span([0xB; 16], [3; 8], [9; 8], 5_000, 6_000, "orphan-op");
    let bytes = seal(vec![req(vec![
        a_root,
        a_child.clone(),
        a_child,
        b_orphan,
    ])]);

    let reader = IndexReader::open(&bytes).unwrap();
    assert!(reader.has_trace_rollup());
    let rollup = reader.trace_rollup().unwrap();
    assert_eq!(rollup.len(), 2);

    // Rows sort by trace id: A (0x0A…) then B (0x0B…).
    assert_eq!(rollup.trace_ids.get(0), TraceId::from([0xA; 16]));
    assert_eq!(rollup.span_counts[0], 3, "the resent span counts twice");
    assert_eq!(rollup.min_start_ns[0], 1_000);
    assert_eq!(rollup.max_end_ns[0], 2_000);
    assert_eq!(rollup.root_is_true_root[0], 1);
    assert_eq!(rollup.root_span_ids.get(0), SpanId::from([1; 8]));
    // The root refs resolve through the file interner to the real values.
    let strings = reader.build_string_table(reader.field_table()).unwrap();
    let resolve = |id: u32| strings[id as usize].clone();
    assert_eq!(
        resolve(rollup.root_service_refs[0]),
        "resource.attributes.service.name=svc"
    );
    assert_eq!(resolve(rollup.root_name_refs[0]), "name=root-op");

    assert_eq!(rollup.trace_ids.get(1), TraceId::from([0xB; 16]));
    assert_eq!(rollup.root_is_true_root[1], 0, "no true root → honest absence");
    assert!(rollup.root_span_ids.get(1).is_unset());
    assert_eq!(rollup.root_service_refs[1], sfst::ROLLUP_NO_REF);

    // And the file stays fully readable by the pre-rollup paths — the
    // additive-chunk contract (assembly ignores TRSU entirely).
    let tr = reader.trace_by_id(TraceId::from([0xA; 16])).unwrap();
    assert_eq!(tr.spans.len(), 2, "assembly still dedups the resend");
}

#[test]
fn rollup_captures_error_status_kind_and_service_absence() {
    // Coverage for every capture arm: an ERROR-status root with a set
    // kind, plus a second resource group WITHOUT service.name (the
    // service ref must be the sentinel, not a neighbor's value).
    use opentelemetry_proto::tonic::trace::v1 as otlp;
    let mut root = span([0xC; 16], [1; 8], [0; 8], 1_000, 2_000, "err-root");
    root.kind = 2; // SERVER
    root.status = Some(otlp::Status {
        code: 2, // STATUS_CODE_ERROR
        message: "boom".into(),
    });
    let child = span([0xC; 16], [2; 8], [1; 8], 1_100, 1_200, "ok-child");

    let mut svcless = req(vec![span([0xD; 16], [3; 8], [0; 8], 9_000, 9_100, "svcless-root")]);
    svcless.resource_spans[0].resource = Some(Resource::default());

    let bytes = seal(vec![req(vec![root, child]), svcless]);
    let reader = IndexReader::open(&bytes).unwrap();
    let rollup = reader.trace_rollup().unwrap();
    assert_eq!(rollup.len(), 2);

    // Trace C: ERROR counted once (the child is OK), kind captured raw.
    assert_eq!(rollup.trace_ids.get(0), TraceId::from([0xC; 16]));
    assert_eq!(rollup.span_counts[0], 2);
    assert_eq!(rollup.error_counts[0], 1);
    assert_eq!(rollup.root_kinds[0], 2);
    assert_eq!(rollup.root_is_true_root[0], 1);

    // Trace D: a true root whose resource has NO service.name — the ref
    // is the sentinel, the name ref still resolves.
    assert_eq!(rollup.trace_ids.get(1), TraceId::from([0xD; 16]));
    assert_eq!(rollup.root_is_true_root[1], 1);
    assert_eq!(rollup.root_service_refs[1], sfst::ROLLUP_NO_REF);
    let strings = reader.build_string_table(reader.field_table()).unwrap();
    assert_eq!(
        strings[rollup.root_name_refs[1] as usize],
        "name=svcless-root"
    );
}

#[test]
fn all_unset_trace_ids_seal_without_a_rollup_chunk() {
    // The is-meaningful rule: a file whose spans all carry the unset
    // trace id (not a trace) writes no TRSU chunk at all.
    let s = span([0; 16], [1; 8], [0; 8], 1_000, 2_000, "no-trace");
    let bytes = seal(vec![req(vec![s])]);
    let reader = IndexReader::open(&bytes).unwrap();
    assert!(!reader.has_trace_rollup());
    assert!(reader.trace_rollup().is_err(), "no chunk to read");
}
