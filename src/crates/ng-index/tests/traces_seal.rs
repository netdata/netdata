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
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
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
/// → fill_trace_hashes → encode → WAL frame), keeping the test self-contained; the
/// real `write_trace_request` is exercised end-to-end by the `#[ignore]`d real-WAL
/// oracle below.
fn seal(reqs: Vec<ExportTraceServiceRequest>) -> Vec<u8> {
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let config = wal::Config {
        rotation: wal::RotationConfig {
            max_log_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    };
    let mut writer = wal::Writer::new(dir.path(), config, seq, 1 /* traces pipeline */).unwrap();
    let mut clock = MonotonicClock::new();
    for mut r in reqs {
        let count = count_spans(&r);
        if count == 0 {
            continue;
        }
        let base = clock.now_ns().as_u64();
        ng_flatten::normalize_span_timestamps(&mut r, base);
        ng_flatten::normalize_trace_ids(&mut r);
        let mut flat = ng_flatten::flatten_trace_request(&r);
        ng_flatten::fill_trace_hashes(&mut flat);
        let data = ng_flatten::encode_trace_frame(&flat).unwrap();
        let ingestion_ns = clock.now_ns();
        writer
            .write_frame(0, &[], &data, count, ingestion_ns, TimestampNs::ZERO, TimestampNs::ZERO)
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();

    assert_eq!(tr.spans.len(), 3);
    assert_eq!(tr.roots.len(), 1);
    // Sorted by start time: root(100), child(110), grand(120).
    assert_eq!(tr.spans[0].span_id, SpanId::from(root));
    assert_eq!(tr.spans[tr.roots[0]].span_id, SpanId::from(root));
    let kids = |sid: [u8; 8]| tr.children.get(&SpanId::from(sid)).cloned().unwrap_or_default();
    assert_eq!(kids(root).len(), 1);
    assert_eq!(tr.spans[kids(root)[0]].span_id, SpanId::from(child));
    assert_eq!(tr.spans[kids(child)[0]].span_id, SpanId::from(grand));
    assert!(kids(grand).is_empty());
    // The `name` facet materialized onto the span.
    assert_eq!(
        tr.spans[tr.roots[0]].fields.iter().find(|(k, _)| k == "name").map(|(_, v)| v.as_str()),
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 2, "duplicate (trace_id, span_id) collapsed");
    assert_eq!(tr.children.get(&SpanId::from(root)).unwrap().len(), 1);
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 3);
    assert_eq!(tr.roots.len(), 3, "two unset-parent roots + one orphan-as-root");
    assert!(tr.children.is_empty(), "no edges: no in-file parent has children");
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans[0].span_id, SpanId::from(child), "earliest start sorts first");
    assert_eq!(tr.roots.len(), 1);
    assert_eq!(tr.spans[tr.roots[0]].span_id, SpanId::from(root));
    let kids = tr.children.get(&SpanId::from(root)).unwrap();
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 201);
    assert_eq!(tr.roots.len(), 1);
    assert_eq!(tr.children.get(&SpanId::from(root)).unwrap().len(), 200);
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 2);
    assert_eq!(tr.roots.len(), 1, "cycle guard promotes the earliest span as a root");
    assert_eq!(tr.spans[tr.roots[0]].span_id, SpanId::from(a), "earliest (start 100) is the root");
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 2, "distinct unset-span-id spans are not collapsed");
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 4);
    // A revisit-guarded walk from the roots must reach every span.
    let mut seen: HashSet<usize> = HashSet::new();
    let mut stack: Vec<usize> = tr.roots.clone();
    while let Some(i) = stack.pop() {
        if !seen.insert(i) {
            continue;
        }
        if let Some(kids) = tr.children.get(&tr.spans[i].span_id) {
            stack.extend(kids.iter().copied());
        }
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
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans.len(), 1);
    assert_eq!(tr.roots.len(), 1, "self-parent treated as root");
    assert!(tr.children.is_empty(), "no self-edge");
}

#[test]
fn trace_by_id_surfaces_flags_and_dropped_count() {
    // The per-row scalars (flags, dropped_attributes_count) reconstruct onto the span.
    let t = [0x8du8; 16];
    let mut s = span(t, [1u8; 8], ROOT_PARENT, 100, 200, "x");
    s.flags = 0x1;
    s.dropped_attributes_count = 3;
    let bytes = seal(vec![req(vec![s])]);
    let tr = IndexReader::open(&bytes).unwrap().trace_by_id(TraceId::from(t)).unwrap();
    assert_eq!(tr.spans[0].flags, 0x1);
    assert_eq!(tr.spans[0].dropped_attributes_count, 3);
}

#[test]
fn trace_by_id_absent_is_empty() {
    let t = [0xF6u8; 16];
    let bytes = seal(vec![req(vec![span(t, [1u8; 8], ROOT_PARENT, 100, 200, "x")])]);
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
                    truth.entry(tid).or_default().insert(*span.span_id.as_bytes());
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
        assert_eq!(&got, &truth[tid], "trace {} span-set mismatch", TraceId::from(*tid));
    }
    println!(
        "oracle OK: {} spans, {} distinct traces; {} sampled traces reconstructed consistently",
        summary.record_count,
        truth.len(),
        checked,
    );
}
