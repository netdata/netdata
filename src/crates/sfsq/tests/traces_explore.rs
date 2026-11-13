//! The span explorer against a brute-force oracle over the spans it was
//! given: the histogram (stack, other, unset, errors, duration rows,
//! percentiles) and the rows (order, paging, columns) under every token
//! shape, across source layouts (sealed, two sealed, sealed and tail,
//! chunk and tail), plus the request errors, the stack cap, the
//! high-cardinality stack, the budget, unavailable and failed sources,
//! and cancellation.

mod common;

use std::collections::BTreeMap;
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, corrupt_chunk, kv_str, memory_source, req_with, sealed_source, sealed_source_at, sp,
    tail_source, unavailable_source, write_wal,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, ArrayValue, KeyValue, any_value::Value};
use sfsq::traces::{
    AttributeOwner, BuiltinField, CompareOp, Condition, ExploreData, ExploreQuery,
    ExploreRequestError, HEATMAP_ROWS, PartialReason, Predicate, PredicateTarget, PredicateValue,
    QueryStatus, RowKey, RowsSpec, STACK_DIMENSIONS_MAX, Token, TokenQuery, TraceSource, explore,
    heatmap_row_bounds,
};

const NS: u64 = 1_000_000_000;
/// The world's spans start in `[BASE, BASE + 10 s)`.
const BASE: u64 = 1_000 * NS;
const SPANS: usize = 120;
const NAMES: [&str; 5] = ["GET /a", "GET /b", "db.query", "publish", "consume"];
const SERVICES: [&str; 3] = ["svc-a", "svc-b", "svc-c"];
const DURATIONS: [u64; 6] = [700, 5_000, 80_000, 2_000_000, 300_000_000, 4_000_000_000];

/// One span of the world, as the oracle sees it.
#[derive(Clone)]
struct Truth {
    key: RowKey,
    duration: i64,
    service: &'static str,
    name: &'static str,
    kind: &'static str,
    status: &'static str,
    role: &'static str,
    method: Option<&'static str>,
}

fn trace_bytes(n: usize) -> [u8; 16] {
    let mut b = [0u8; 16];
    b[0] = 0x7;
    b[15] = n as u8;
    b
}

/// One span of the world: its spec, service and HTTP method.
#[derive(Clone)]
struct WorldSpan {
    spec: SpanSpec,
    service: &'static str,
    method: Option<&'static str>,
}

/// The world: 30 traces of 4 spans over three services. Some spans start
/// with their predecessor (row-order ties, within and across traces),
/// statuses and kinds cycle, and durations span six orders of magnitude.
fn world() -> Vec<WorldSpan> {
    let mut spans = Vec::with_capacity(SPANS);
    let mut start = BASE;
    for i in 0..SPANS {
        if i % 10 != 9 && i % 8 != 4 {
            start = BASE + (i as u64 * 83_333_331) % (10 * NS);
        }
        let first = (i / 4) * 4;
        let status = if i % 7 == 0 {
            Some((2, "boom"))
        } else if i % 5 == 0 {
            Some((1, ""))
        } else {
            None
        };
        let method = [Some("GET"), Some("POST"), None][i % 3];
        let spec = SpanSpec {
            trace: trace_bytes(i / 4),
            kind: [2, 3, 1, 5][i % 4],
            status,
            attrs: method
                .map(|m| kv_str("http.method", m))
                .into_iter()
                .collect(),
            end: start + DURATIONS[i % DURATIONS.len()],
            ..sp(
                (i + 1) as u8,
                if i == first { 0 } else { (first + 1) as u8 },
                start,
                NAMES[i % NAMES.len()],
            )
        };
        spans.push(WorldSpan {
            spec,
            service: SERVICES[(i / 4) % SERVICES.len()],
            method,
        });
    }
    spans
}

fn truth(spans: &[WorldSpan]) -> Vec<Truth> {
    spans
        .iter()
        .map(|w| {
            let s = &w.spec;
            Truth {
                key: RowKey {
                    start_ns: s.start as i64,
                    trace_id: sfst::TraceId::from(s.trace),
                    span_id: sfst::SpanId::from(s.id),
                },
                duration: (s.end - s.start) as i64,
                service: w.service,
                name: s.name,
                kind: match s.kind {
                    1 => "INTERNAL",
                    2 => "SERVER",
                    3 => "CLIENT",
                    5 => "CONSUMER",
                    _ => unreachable!(),
                },
                status: match s.status {
                    Some((2, _)) => "ERROR",
                    Some((1, _)) => "OK",
                    _ => "UNSET",
                },
                role: match (s.parent == [0; 8], s.kind) {
                    (true, _) => "ROOT",
                    (false, 2 | 5) => "INBOUND",
                    (false, 3 | 4) => "OUTBOUND",
                    _ => "INTERNAL",
                },
                method: w.method,
            }
        })
        .collect()
}

/// One request per service, in span order.
fn requests(spans: &[WorldSpan]) -> Vec<ExportTraceServiceRequest> {
    SERVICES
        .iter()
        .map(|service| {
            let of: Vec<SpanSpec> = spans
                .iter()
                .filter(|w| w.service == *service)
                .map(|w| w.spec.clone())
                .collect();
            req_with(vec![kv_str("service.name", service)], None, &of)
        })
        .filter(|r| !r.resource_spans[0].scope_spans[0].spans.is_empty())
        .collect()
}

const LAYOUTS: [&str; 4] = [
    "single-sealed",
    "two-sealed",
    "sealed-plus-tail",
    "chunk-plus-tail",
];

/// The world in one named layout (sources are consumed per call).
fn layout(dir: &Path, name: &str) -> Vec<TraceSource> {
    let spans = world();
    let (early, late) = spans.split_at(SPANS / 2);
    let all = write_wal(dir, requests(&spans), "all");
    let early = write_wal(dir, requests(early), "early");
    let late = write_wal(dir, requests(late), "late");
    match name {
        "single-sealed" => vec![sealed_source(dir, &all, "one")],
        "two-sealed" => vec![
            sealed_source(dir, &early, "early"),
            sealed_source(dir, &late, "late"),
        ],
        "sealed-plus-tail" => vec![
            sealed_source(dir, &early, "early"),
            tail_source(&late, "late-tail"),
        ],
        "chunk-plus-tail" => vec![
            memory_source(&early, "early-chunk"),
            tail_source(&late, "late-tail"),
        ],
        other => panic!("unknown layout {other}"),
    }
}

/// Ten one-second buckets over the world.
fn grid() -> sfst::Grid {
    sfst::Grid::new(BASE as i64, NS as i64, 10)
}

fn text(v: &str) -> PredicateValue {
    PredicateValue::Text(v.to_string())
}

fn cond(target: PredicateTarget, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    Condition { target, op, values }
}

fn builtin(b: BuiltinField, op: CompareOp, value: &str) -> Condition {
    cond(PredicateTarget::Builtin(b), op, vec![text(value)])
}

fn service_is(service: &str) -> Condition {
    cond(
        PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
        CompareOp::Eq,
        vec![text(service)],
    )
}

fn positive(conditions: Vec<Condition>) -> Token {
    Token::positive(Predicate { conditions })
}

fn negated(conditions: Vec<Condition>) -> Token {
    Token::negated(Predicate { conditions })
}

fn run(sources: Vec<TraceSource>, query: ExploreQuery) -> ExploreData {
    explore(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

type Selects = Box<dyn Fn(&Truth) -> bool>;

/// A stack key's value on a span, by the oracle (`None`: the span lacks it).
type StackValue = Box<dyn Fn(&Truth) -> Option<&'static str>>;

/// Per bucket: the named counts, `other`, `unset`.
type StackBuckets = Vec<(Vec<u64>, u64, u64)>;

/// Token queries and the spans each must select.
fn scopes() -> Vec<(&'static str, TokenQuery, Selects)> {
    let t3 = sfst::TraceId::from(trace_bytes(3)).to_string();
    vec![
        ("all", TokenQuery::default(), Box::new(|_| true)),
        (
            "service",
            TokenQuery {
                tokens: vec![positive(vec![service_is("svc-b")])],
            },
            Box::new(|t| t.service == "svc-b"),
        ),
        (
            "same-span",
            TokenQuery {
                tokens: vec![
                    positive(vec![builtin(BuiltinField::Kind, CompareOp::Eq, "SERVER")]),
                    positive(vec![builtin(BuiltinField::Status, CompareOp::Eq, "ERROR")]),
                ],
            },
            Box::new(|t| t.kind == "SERVER" && t.status == "ERROR"),
        ),
        (
            "in-group-not-equal",
            TokenQuery {
                tokens: vec![positive(vec![builtin(
                    BuiltinField::Status,
                    CompareOp::NotEq,
                    "UNSET",
                )])],
            },
            Box::new(|t| t.status != "UNSET"),
        ),
        (
            "negated-token",
            TokenQuery {
                tokens: vec![negated(vec![builtin(
                    BuiltinField::Name,
                    CompareOp::Eq,
                    "db.query",
                )])],
            },
            Box::new(|t| t.name != "db.query"),
        ),
        (
            "text",
            TokenQuery {
                tokens: vec![positive(vec![cond(
                    PredicateTarget::FreeText,
                    CompareOp::Contains,
                    vec![text("boom")],
                )])],
            },
            Box::new(|t| t.status == "ERROR"),
        ),
        (
            "role",
            TokenQuery {
                tokens: vec![positive(vec![builtin(
                    BuiltinField::Role,
                    CompareOp::Eq,
                    "INBOUND",
                )])],
            },
            Box::new(|t| t.role == "INBOUND"),
        ),
        (
            "trace-id",
            TokenQuery {
                tokens: vec![positive(vec![builtin(
                    BuiltinField::TraceId,
                    CompareOp::Eq,
                    &t3,
                )])],
            },
            Box::new(|t| t.key.trace_id == sfst::TraceId::from(trace_bytes(3))),
        ),
        (
            "not-trace-id",
            TokenQuery {
                tokens: vec![positive(vec![builtin(
                    BuiltinField::TraceId,
                    CompareOp::NotEq,
                    &t3,
                )])],
            },
            Box::new(|t| t.key.trace_id != sfst::TraceId::from(trace_bytes(3))),
        ),
        (
            "negated-trace-id",
            TokenQuery {
                tokens: vec![negated(vec![builtin(
                    BuiltinField::TraceId,
                    CompareOp::Eq,
                    &t3,
                )])],
            },
            Box::new(|t| t.key.trace_id != sfst::TraceId::from(trace_bytes(3))),
        ),
        (
            "negated-not-trace-id",
            TokenQuery {
                tokens: vec![negated(vec![builtin(
                    BuiltinField::TraceId,
                    CompareOp::NotEq,
                    &t3,
                )])],
            },
            Box::new(|t| t.key.trace_id == sfst::TraceId::from(trace_bytes(3))),
        ),
        (
            // A third of the spans lack the key: a negated token keeps them.
            "negated-absent-key",
            TokenQuery {
                tokens: vec![negated(vec![cond(
                    PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into()),
                    CompareOp::Eq,
                    vec![text("GET")],
                )])],
            },
            Box::new(|t| t.method != Some("GET")),
        ),
        (
            "duration",
            TokenQuery {
                tokens: vec![positive(vec![cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Gte,
                    vec![PredicateValue::Integer(2_000_000)],
                )])],
            },
            Box::new(|t| t.duration >= 2_000_000),
        ),
    ]
}

fn bucket_of(t: &Truth) -> usize {
    ((t.key.start_ns as u64 - BASE) / NS) as usize
}

/// The histogram's stack, other and unset per bucket, by the oracle.
fn expected_stack(
    selected: &[&Truth],
    value: impl Fn(&Truth) -> Option<&'static str>,
) -> (Vec<String>, StackBuckets) {
    let mut totals: BTreeMap<&str, u64> = BTreeMap::new();
    for t in selected {
        if let Some(v) = value(t) {
            *totals.entry(v).or_default() += 1;
        }
    }
    let mut ranked: Vec<(&str, u64)> = totals.into_iter().collect();
    ranked.sort_by(|a, b| b.1.cmp(&a.1).then(a.0.cmp(b.0)));
    ranked.truncate(STACK_DIMENSIONS_MAX);
    let dims: Vec<String> = ranked.iter().map(|(d, _)| d.to_string()).collect();
    let buckets = (0..10)
        .map(|b| {
            let in_bucket: Vec<&&Truth> = selected.iter().filter(|t| bucket_of(t) == b).collect();
            let counts: Vec<u64> = dims
                .iter()
                .map(|d| {
                    in_bucket
                        .iter()
                        .filter(|t| value(t) == Some(d.as_str()))
                        .count() as u64
                })
                .collect();
            let unset = in_bucket.iter().filter(|t| value(t).is_none()).count() as u64;
            let other = in_bucket.len() as u64 - unset - counts.iter().sum::<u64>();
            (counts, other, unset)
        })
        .collect();
    (dims, buckets)
}

fn heatmap_row(duration: i64) -> usize {
    heatmap_row_bounds()
        .iter()
        .position(|b| b.is_none_or(|b| duration < b))
        .unwrap()
}

/// Every layout's histogram equals the oracle's, for every token shape
/// and two stack keys (a builtin and an attribute some spans lack).
#[test]
fn histogram_matches_the_oracle() {
    let truths = truth(&world());
    let dir = tempfile::tempdir().unwrap();
    for layout_name in LAYOUTS {
        let sources = layout(dir.path(), layout_name);
        for (scope_name, tokens, selects) in scopes() {
            let selected: Vec<&Truth> = truths.iter().filter(|t| selects(t)).collect();
            let stacks: [(&str, StackValue); 2] = [
                ("status.code", Box::new(|t: &Truth| Some(t.status))),
                ("attributes.http.method", Box::new(|t: &Truth| t.method)),
            ];
            for (stack, value) in stacks {
                let data = run(
                    sources.clone(),
                    ExploreQuery::new(grid())
                        .tokens(tokens.clone())
                        .histogram(stack),
                );
                let at = format!("{layout_name}/{scope_name}/{stack}");
                assert_eq!(data.status, QueryStatus::Complete, "{at}");
                let h = data.histogram.expect("asked for");
                let (dims, buckets) = expected_stack(&selected, &value);
                assert_eq!(h.dimensions, dims, "{at}");
                for (b, (bucket, (counts, other, unset))) in
                    h.buckets.iter().zip(&buckets).enumerate()
                {
                    assert_eq!(&bucket.counts, counts, "{at} bucket {b} counts");
                    assert_eq!(bucket.other, *other, "{at} bucket {b} other");
                    assert_eq!(bucket.unset, *unset, "{at} bucket {b} unset");
                    let mut rows = [0u64; HEATMAP_ROWS];
                    for t in selected.iter().filter(|t| bucket_of(t) == b) {
                        rows[heatmap_row(t.duration)] += 1;
                    }
                    assert_eq!(bucket.durations, rows, "{at} bucket {b} durations");
                }
                assert_eq!(h.total, selected.len() as u64, "{at} total");
                assert_eq!(
                    h.errors,
                    selected.iter().filter(|t| t.status == "ERROR").count() as u64,
                    "{at} errors"
                );
                assert_eq!(h.percentiles.is_some(), !selected.is_empty(), "{at}");
                if let Some(p) = h.percentiles {
                    let mut sorted: Vec<i64> = selected.iter().map(|t| t.duration).collect();
                    sorted.sort_unstable();
                    let exact = sorted[(50 * sorted.len()).div_ceil(100) - 1];
                    assert!((p.p50 - exact).abs() <= exact / 32, "{at} p50");
                }
            }
        }
    }
}

/// Walking every page newest first lists each selected span exactly once,
/// in key order, ties included, with its columns; `matched` is the
/// histogram's total and `more` holds until the last page.
#[test]
fn rows_walk_every_span_once_in_key_order() {
    let truths = truth(&world());
    let columns = vec![
        "resource.attributes.service.name".to_string(),
        "name".to_string(),
        "kind".to_string(),
        "status.code".to_string(),
        "role".to_string(),
        "attributes.http.method".to_string(),
    ];
    let dir = tempfile::tempdir().unwrap();
    for layout_name in LAYOUTS {
        let sources = layout(dir.path(), layout_name);
        for (scope_name, tokens, selects) in scopes() {
            let mut expected: Vec<&Truth> = truths.iter().filter(|t| selects(t)).collect();
            expected.sort_by_key(|t| std::cmp::Reverse(t.key));
            let mut listed: Vec<(RowKey, i64, Vec<Vec<String>>)> = Vec::new();
            let mut before = None;
            loop {
                let data = run(
                    sources.clone(),
                    ExploreQuery::new(grid())
                        .tokens(tokens.clone())
                        .rows(RowsSpec {
                            limit: 7,
                            before,
                            columns: columns.clone(),
                        }),
                );
                let at = format!("{layout_name}/{scope_name}");
                assert_eq!(data.status, QueryStatus::Complete, "{at}");
                let rows = data.rows.expect("asked for");
                assert_eq!(rows.matched, expected.len() as u64, "{at} matched");
                assert!(rows.items.len() <= 7);
                for item in &rows.items {
                    listed.push((item.key, item.duration_ns, item.columns.clone()));
                }
                if !rows.more {
                    break;
                }
                before = Some(rows.items.last().expect("a page before more").key);
            }
            let want: Vec<(RowKey, i64, Vec<Vec<String>>)> = expected
                .iter()
                .map(|t| {
                    let one = |v: &str| vec![v.to_string()];
                    (
                        t.key,
                        t.duration,
                        vec![
                            one(t.service),
                            one(t.name),
                            one(t.kind),
                            one(t.status),
                            one(t.role),
                            t.method.map(one).unwrap_or_default(),
                        ],
                    )
                })
                .collect();
            assert_eq!(listed, want, "{layout_name}/{scope_name}");
        }
    }
}

/// The world has row-order ties: the walk must split them by key.
#[test]
fn the_world_has_ties() {
    let truths = truth(&world());
    let mut starts: Vec<i64> = truths.iter().map(|t| t.key.start_ns).collect();
    starts.sort_unstable();
    starts.dedup();
    assert!(starts.len() < truths.len());
}

/// More values than the stack names: the top ones by count, the rest in
/// `other`, and the counts still add up.
#[test]
fn the_stack_names_the_most_frequent_values() {
    let dir = tempfile::tempdir().unwrap();
    let mut spans = Vec::new();
    for i in 0..(STACK_DIMENSIONS_MAX + 5) {
        // Value `v{i}` on `i + 1` spans, so the order is strict.
        for j in 0..=i {
            spans.push(SpanSpec {
                trace: trace_bytes(i),
                attrs: vec![kv_str("tier", &format!("v{i:02}"))],
                ..sp((j + 1) as u8, 0, BASE + (j as u64) * 1000, "op")
            });
        }
    }
    let wal = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &spans)],
        "tiers",
    );
    let data = run(
        vec![sealed_source(dir.path(), &wal, "one")],
        ExploreQuery::new(grid()).histogram("attributes.tier"),
    );
    let h = data.histogram.unwrap();
    let want: Vec<String> = (5..STACK_DIMENSIONS_MAX + 5)
        .rev()
        .map(|i| format!("v{i:02}"))
        .collect();
    assert_eq!(h.dimensions, want);
    let other: u64 = h.buckets.iter().map(|b| b.other).sum();
    assert_eq!(other, (1..=5).sum::<u64>());
    let named: u64 = h.buckets.iter().flat_map(|b| &b.counts).sum();
    assert_eq!(named + other, h.total);
}

/// A file where the stack key is high-cardinality counts in `other` and
/// says so; another file still stacks.
#[test]
fn a_high_cardinality_stack_counts_as_other() {
    let dir = tempfile::tempdir().unwrap();
    let mut wide = Vec::new();
    for i in 0..1_200u64 {
        wide.push(SpanSpec {
            trace: trace_bytes((i / 250) as usize),
            attrs: vec![kv_str("user", &format!("u{i}"))],
            ..sp(((i % 250) + 1) as u8, 0, BASE + i * 1000, "op")
        });
    }
    let narrow = vec![SpanSpec {
        attrs: vec![kv_str("user", "u-narrow")],
        ..sp(1, 0, BASE + 5 * NS, "op")
    }];
    let wide = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &wide)],
        "wide",
    );
    let narrow = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &narrow)],
        "narrow",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wide, "wide"),
            sealed_source(dir.path(), &narrow, "narrow"),
        ],
        ExploreQuery::new(grid()).histogram("attributes.user"),
    );
    assert_eq!(
        data.status,
        QueryStatus::Complete.with(PartialReason::StackFieldHighCard)
    );
    let h = data.histogram.unwrap();
    assert_eq!(h.dimensions, ["u-narrow"]);
    assert_eq!(h.buckets.iter().map(|b| b.other).sum::<u64>(), 1_200);
    assert_eq!(h.total, 1_201);
}

/// A file where the stack key is high-cardinality but the filter selects
/// none of its spans adds nothing, so the answer stays complete.
#[test]
fn a_high_cardinality_file_without_selected_spans_does_not_count() {
    let dir = tempfile::tempdir().unwrap();
    let wide: Vec<SpanSpec> = (0..1_200u64)
        .map(|i| SpanSpec {
            trace: trace_bytes((i / 250) as usize),
            attrs: vec![kv_str("user", &format!("u{i}"))],
            ..sp(((i % 250) + 1) as u8, 0, BASE + i * 1000, "op")
        })
        .collect();
    let narrow = vec![SpanSpec {
        attrs: vec![kv_str("user", "u-narrow")],
        ..sp(1, 0, BASE + 5 * NS, "other-op")
    }];
    let wide = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &wide)],
        "wide",
    );
    let narrow = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &narrow)],
        "narrow",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wide, "wide"),
            sealed_source(dir.path(), &narrow, "narrow"),
        ],
        ExploreQuery::new(grid())
            .tokens(TokenQuery {
                tokens: vec![positive(vec![builtin(
                    BuiltinField::Name,
                    CompareOp::Eq,
                    "other-op",
                )])],
            })
            .histogram("attributes.user"),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    let h = data.histogram.unwrap();
    assert_eq!(h.dimensions, ["u-narrow"]);
    assert_eq!(h.buckets.iter().map(|b| b.other).sum::<u64>(), 0);
    assert_eq!(h.total, 1);
}

#[test]
fn bad_requests_are_refused() {
    let query = || ExploreQuery::new(grid());
    let check = |q: ExploreQuery, want: fn(&ExploreRequestError) -> bool| match explore(
        Vec::new(),
        q,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    ) {
        Err(e) => assert!(want(&e), "{e}"),
        Ok(_) => panic!("accepted"),
    };
    check(
        query().tokens(TokenQuery {
            tokens: vec![positive(vec![builtin(
                BuiltinField::RootName,
                CompareOp::Eq,
                "x",
            )])],
        }),
        |e| matches!(e, ExploreRequestError::TraceLevelToken { .. }),
    );
    check(query().histogram("duration"), |e| {
        matches!(e, ExploreRequestError::NoStoredValues { .. })
    });
    check(query().histogram("status"), |e| {
        matches!(e, ExploreRequestError::UnknownKey { .. })
    });
    for limit in [0, 201] {
        check(
            query().rows(RowsSpec {
                limit,
                before: None,
                columns: Vec::new(),
            }),
            |e| matches!(e, ExploreRequestError::RowsLimit { .. }),
        );
    }
    check(
        query().rows(RowsSpec {
            limit: 10,
            before: None,
            columns: vec!["name".to_string(); 33],
        }),
        |e| matches!(e, ExploreRequestError::TooManyColumns { .. }),
    );
    check(
        query().rows(RowsSpec {
            limit: 10,
            before: None,
            columns: vec!["trace_id".to_string()],
        }),
        |e| matches!(e, ExploreRequestError::NoStoredValues { .. }),
    );
    check(ExploreQuery::new(sfst::Grid::new(0, 0, 10)), |e| {
        matches!(e, ExploreRequestError::EmptyGrid)
    });
    check(query(), |e| matches!(e, ExploreRequestError::NoSection));
}

fn tags(values: &[&str]) -> KeyValue {
    KeyValue {
        key: "tags".into(),
        value: Some(AnyValue {
            value: Some(Value::ArrayValue(ArrayValue {
                values: values
                    .iter()
                    .map(|v| AnyValue {
                        value: Some(Value::StringValue(v.to_string())),
                    })
                    .collect(),
            })),
        }),
        key_strindex: 0,
    }
}

/// A span with several values of the stack key counts under each, so the
/// stacked counts can exceed the total; the total still counts spans.
#[test]
fn a_span_with_several_stack_values_counts_under_each() {
    let dir = tempfile::tempdir().unwrap();
    let spans = vec![
        SpanSpec {
            attrs: vec![tags(&["a", "b"])],
            ..sp(1, 0, BASE, "op")
        },
        SpanSpec {
            attrs: vec![tags(&["a"])],
            ..sp(2, 0, BASE + 1000, "op")
        },
    ];
    let wal = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &spans)],
        "tags",
    );
    let data = run(
        vec![sealed_source(dir.path(), &wal, "tags")],
        ExploreQuery::new(grid()).histogram("attributes.tags[]"),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    let h = data.histogram.unwrap();
    assert_eq!(h.dimensions, ["a", "b"]);
    assert_eq!(h.buckets[0].counts, [2, 1]);
    assert_eq!((h.buckets[0].other, h.buckets[0].unset), (0, 0));
    assert_eq!(h.total, 2);
}

/// A window inside the world counts a span that starts exactly at its
/// start and leaves out one that starts exactly at its end, in every
/// layout, in the histogram and in the rows alike.
#[test]
fn a_window_counts_its_start_and_not_its_end() {
    let truths = truth(&world());
    let mut starts: Vec<i64> = truths.iter().map(|t| t.key.start_ns).collect();
    starts.sort_unstable();
    starts.dedup();
    let (start, end) = (starts[3], starts[40]);
    let mut expected: Vec<RowKey> = truths
        .iter()
        .filter(|t| (start..end).contains(&t.key.start_ns))
        .map(|t| t.key)
        .collect();
    expected.sort_by_key(|k| std::cmp::Reverse(*k));
    let dir = tempfile::tempdir().unwrap();
    for layout_name in LAYOUTS {
        let data = run(
            layout(dir.path(), layout_name),
            ExploreQuery::new(sfst::Grid::new(start, end - start, 1))
                .histogram("status.code")
                .rows(RowsSpec {
                    limit: 200,
                    before: None,
                    columns: Vec::new(),
                }),
        );
        assert_eq!(data.status, QueryStatus::Complete, "{layout_name}");
        assert_eq!(
            data.histogram.unwrap().total,
            expected.len() as u64,
            "{layout_name}"
        );
        let rows = data.rows.unwrap();
        let listed: Vec<RowKey> = rows.items.iter().map(|row| row.key).collect();
        assert_eq!(listed, expected, "{layout_name}");
        assert!(!rows.more, "{layout_name}");
    }
}

/// The budget leaves out the older sources and says so; the newer ones
/// still count.
#[test]
fn the_budget_stops_at_the_older_sources() {
    let dir = tempfile::tempdir().unwrap();
    let spans = world();
    let (early, late) = spans.split_at(SPANS / 2);
    let early = write_wal(dir.path(), requests(early), "early");
    let late = write_wal(dir.path(), requests(late), "late");
    let data = run(
        vec![
            sealed_source(dir.path(), &early, "early"),
            sealed_source(dir.path(), &late, "late"),
        ],
        ExploreQuery::new(grid())
            .histogram("status.code")
            .rows_ceiling_for_tests((SPANS / 2) as u64 + 10),
    );
    assert_eq!(
        data.status,
        QueryStatus::Complete.with(PartialReason::WorkCeiling)
    );
    // The newer file (the late half) counted; the older one did not.
    assert_eq!(data.histogram.unwrap().total, (SPANS / 2) as u64);
}

/// Under the budget the rows come from the newer sources it visited.
#[test]
fn the_budget_stops_the_rows_at_the_older_sources() {
    let dir = tempfile::tempdir().unwrap();
    let spans = world();
    let (early, late) = spans.split_at(SPANS / 2);
    let mut expected: Vec<RowKey> = truth(late).iter().map(|t| t.key).collect();
    expected.sort_by_key(|k| std::cmp::Reverse(*k));
    let early = write_wal(dir.path(), requests(early), "early");
    let late = write_wal(dir.path(), requests(late), "late");
    let data = run(
        vec![
            sealed_source(dir.path(), &early, "early"),
            sealed_source(dir.path(), &late, "late"),
        ],
        ExploreQuery::new(grid())
            .rows(RowsSpec {
                limit: 200,
                before: None,
                columns: Vec::new(),
            })
            .rows_ceiling_for_tests((SPANS / 2) as u64 + 10),
    );
    assert_eq!(
        data.status,
        QueryStatus::Complete.with(PartialReason::WorkCeiling)
    );
    let rows = data.rows.unwrap();
    assert_eq!(rows.matched, (SPANS / 2) as u64);
    let listed: Vec<RowKey> = rows.items.iter().map(|row| row.key).collect();
    assert_eq!(listed, expected);
}

/// An unavailable source in the window and a failed one are reported;
/// the readable source still counts.
#[test]
fn unavailable_and_failed_sources_are_reported() {
    let dir = tempfile::tempdir().unwrap();
    let spans = world();
    let all = write_wal(dir.path(), requests(&spans), "all");
    let good = sealed_source(dir.path(), &all, "good");
    let broken_wal = write_wal(dir.path(), requests(&spans), "broken");
    let TraceSource::Sfst(broken) = sealed_source(dir.path(), &broken_wal, "broken") else {
        unreachable!()
    };
    let sfsq::Source::File(path) = &broken.source else {
        unreachable!()
    };
    corrupt_chunk(path, *b"TIMS");
    let data = run(
        vec![
            good,
            sealed_source_at(path, "broken"),
            unavailable_source("gone", 1_000, 1_005),
            unavailable_source("outside", 10, 20),
        ],
        ExploreQuery::new(grid()).histogram("status.code"),
    );
    assert_eq!(
        data.status,
        QueryStatus::Complete
            .with(PartialReason::SourceFailure)
            .with(PartialReason::RemoteUnavailable)
    );
    assert_eq!(data.histogram.unwrap().total, SPANS as u64);
}

/// A source whose page columns cannot be read leaves the page: the other
/// sources' rows still list, and the answer says a source failed.
#[test]
fn a_failed_column_read_drops_only_its_source() {
    let dir = tempfile::tempdir().unwrap();
    let wide: Vec<SpanSpec> = (0..1_200u64)
        .map(|i| SpanSpec {
            trace: trace_bytes((i / 250) as usize),
            attrs: vec![kv_str("user", &format!("u{i}"))],
            ..sp(((i % 250) + 1) as u8, 0, BASE + i * 1000, "op")
        })
        .collect();
    let narrow = vec![SpanSpec {
        trace: trace_bytes(9),
        attrs: vec![kv_str("user", "u-narrow")],
        ..sp(1, 0, BASE + 5 * NS, "op")
    }];
    let wide = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &wide)],
        "wide",
    );
    let narrow = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &narrow)],
        "narrow",
    );
    let TraceSource::Sfst(wide) = sealed_source(dir.path(), &wide, "wide") else {
        unreachable!()
    };
    let sfsq::Source::File(path) = &wide.source else {
        unreachable!()
    };
    // The user values of the wide file are high-cardinality: the only
    // high field, which nothing but the page's column read touches.
    corrupt_chunk(path, *b"HF\0\0");
    let data = run(
        vec![
            sealed_source_at(path, "wide"),
            sealed_source(dir.path(), &narrow, "narrow"),
        ],
        ExploreQuery::new(grid()).rows(RowsSpec {
            limit: 10,
            before: None,
            columns: vec!["attributes.user".to_string()],
        }),
    );
    assert_eq!(
        data.status,
        QueryStatus::Complete.with(PartialReason::SourceFailure)
    );
    let rows = data.rows.unwrap();
    assert_eq!(rows.matched, 1_201);
    assert!(!rows.more);
    let listed: Vec<(i64, Vec<Vec<String>>)> = rows
        .items
        .into_iter()
        .map(|row| (row.key.start_ns, row.columns))
        .collect();
    assert_eq!(
        listed,
        [((BASE + 5 * NS) as i64, vec![vec!["u-narrow".to_string()]])]
    );
}

/// With sources or without any (a cancelled capture hands the engine
/// none), a cancelled request answers nothing and says so.
#[test]
fn a_cancelled_request_answers_nothing() {
    let dir = tempfile::tempdir().unwrap();
    for sources in [layout(dir.path(), "two-sealed"), Vec::new()] {
        let cancel = CancellationToken::new();
        cancel.cancel();
        let data = explore(
            sources,
            ExploreQuery::new(grid()).histogram("status.code"),
            cancel,
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap();
        assert_eq!(
            data,
            ExploreData {
                status: QueryStatus::Complete.with(PartialReason::Cancelled),
                histogram: None,
                rows: None,
            }
        );
    }
}
