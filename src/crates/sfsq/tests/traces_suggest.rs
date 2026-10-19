//! `suggest` over real sources: sealed files of every tier, in-memory
//! chunks, WAL tails, unavailable and broken sources.
//!
//! Pins: ranking by match class then bytes across tiers and the tail;
//! the item cap with an exact `truncated`; newest-first early stops
//! (source budget, no new matches, deadline) with their reason and
//! `complete_after`; sources without the key cost no budget; unavailable
//! and broken sources are reported; cancellation is all-or-empty; zero
//! limits, the owner/key pairings of pin C4 and duplicate source ids are
//! rejected; keys partition by owner with every builtin (with or without
//! sources) and no internal field; builtin values are the storage labels,
//! the OTel defaults included, and virtual builtins have none; a window
//! prunes files but never a tail.

mod common;

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use common::{
    kv_double, kv_int, kv_str, memory_source, missing_source, req, req_with, sealed_source, sp,
    tail_source, unavailable_source, write_wal,
};
use sfsq::traces::{
    AttributeKey, AttributeOwner, BuiltinField, PartialReason, QueryStatus, SourceId, SuggestData,
    SuggestQuery, SuggestRequestError, Suggestion, TraceSfstCandidate, TraceSource, suggest,
};
use sfst::FieldTier;
use sfst::MatchClass::{self, *};
use tokio_util::sync::CancellationToken;

const BASE_S: u64 = 1_700_000_000;

/// One span per value of the span attribute `k`, at `at_s` seconds past
/// [`BASE_S`].
fn spans(values: &[&str], at_s: u64) -> Vec<common::SpanSpec> {
    values
        .iter()
        .enumerate()
        .map(|(i, v)| {
            let mut s = sp(i as u8 + 1, 0, (BASE_S + at_s) * 1_000_000_000, "op");
            s.attrs = vec![kv_str("k", v)];
            s
        })
        .collect()
}

fn q() -> SuggestQuery {
    SuggestQuery::values(AttributeOwner::Span, AttributeKey::Attribute("k".into()))
}

fn run(sources: Vec<TraceSource>, query: SuggestQuery) -> SuggestData {
    suggest(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

fn items(data: &SuggestData) -> Vec<(&str, MatchClass)> {
    data.items
        .iter()
        .map(|s| (s.text.as_str(), s.class))
        .collect()
}

fn partial(reasons: &[PartialReason]) -> QueryStatus {
    QueryStatus::Partial(reasons.iter().copied().collect())
}

#[test]
fn values_rank_by_class_then_bytes_across_sources() {
    let dir = tempfile::tempdir().unwrap();
    let a = write_wal(
        dir.path(),
        vec![req(&spans(&["frontend", "cart"], 10))],
        "a",
    );
    let b = write_wal(
        dir.path(),
        vec![req(&spans(&["frontend-proxy", "checkout"], 20))],
        "b",
    );
    let c = write_wal(dir.path(), vec![req(&spans(&["Front"], 25))], "c");
    let t = write_wal(
        dir.path(),
        vec![req(&spans(&["my-frontend", "frnt-api"], 30))],
        "t",
    );
    let sources = vec![
        sealed_source(dir.path(), &a, "a"),
        memory_source(&b, "b"),
        sealed_source(dir.path(), &c, "c"),
        tail_source(&t, "t"),
    ];
    let data = run(sources, q().term("front"));
    assert_eq!(
        items(&data),
        [
            ("Front", Exact),
            ("frontend", Prefix),
            ("frontend-proxy", Prefix),
            ("my-frontend", Contains),
            ("frnt-api", Fuzzy1),
        ]
    );
    assert!(!data.truncated);
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.complete_after, None);
}

#[test]
fn a_high_cardinality_key_lists_its_best_items_with_an_exact_flag() {
    // 1,010 distinct values: the high tier's arena.
    let dir = tempfile::tempdir().unwrap();
    let names: Vec<String> = (0..1010).map(|i| format!("v{i:04}")).collect();
    let all: Vec<&str> = names.iter().map(String::as_str).collect();
    let reqs = all.chunks(200).map(|vs| req(&spans(vs, 10))).collect();
    let wal = write_wal(dir.path(), reqs, "h");
    let source = || vec![sealed_source(dir.path(), &wal, "h")];

    // A 3-character term gets no typo-tolerant matches, so exactly
    // v1000..v1009 match.
    let data = run(source(), q().term("v10").max_items(3));
    assert_eq!(
        items(&data),
        [("v1000", Prefix), ("v1001", Prefix), ("v1002", Prefix)]
    );
    assert!(data.truncated, "v1003..v1009 also matched");

    let data = run(source(), q().term("v10").max_items(10));
    assert_eq!(data.items.len(), 10);
    assert!(!data.truncated, "exactly ten matched");

    // No term: every value matches as a prefix; the first ones by bytes.
    let data = run(source(), q().max_items(2));
    assert_eq!(items(&data), [("v0000", Prefix), ("v0001", Prefix)]);
    assert!(data.truncated);
}

/// Three sealed files, newest first: `new` (at 30 s), `mid` (20 s),
/// `old` (10 s), each with its own values.
fn three_files(
    dir: &std::path::Path,
    new: &[&str],
    mid: &[&str],
    old: &[&str],
) -> Vec<TraceSource> {
    let w_new = write_wal(dir, vec![req(&spans(new, 30))], "new");
    let w_mid = write_wal(dir, vec![req(&spans(mid, 20))], "mid");
    let w_old = write_wal(dir, vec![req(&spans(old, 10))], "old");
    // The caller's order does not matter: the walk sorts newest first.
    vec![
        sealed_source(dir, &w_old, "old"),
        sealed_source(dir, &w_new, "new"),
        sealed_source(dir, &w_mid, "mid"),
    ]
}

#[test]
fn the_source_budget_stops_the_walk_and_says_from_when_it_is_complete() {
    let dir = tempfile::tempdir().unwrap();
    let sources = three_files(dir.path(), &["c"], &["b"], &["a"]);
    let data = run(sources.clone(), q().max_files(1));
    assert_eq!(items(&data), [("c", Prefix)]);
    assert_eq!(data.status, partial(&[PartialReason::FileBudget]));
    // Every span from one second past `mid`'s newest lives in `new`.
    assert_eq!(data.complete_after, Some((BASE_S + 20 + 1) as u32));

    let data = run(sources, q().max_files(3));
    assert_eq!(items(&data), [("a", Prefix), ("b", Prefix), ("c", Prefix)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

#[test]
fn sources_that_add_nothing_stop_the_walk() {
    let dir = tempfile::tempdir().unwrap();
    let sources = three_files(dir.path(), &["x"], &["x"], &["a"]);
    let data = run(sources, q().stale_files(1));
    assert_eq!(items(&data), [("x", Prefix)]);
    assert_eq!(data.status, partial(&[PartialReason::NoNewMatches]));
    assert_eq!(data.complete_after, Some((BASE_S + 10 + 1) as u32));
}

#[test]
fn sources_without_the_key_cost_no_budget() {
    let dir = tempfile::tempdir().unwrap();
    let w_new = write_wal(dir.path(), vec![req(&spans(&["c"], 30))], "new");
    let mut other = spans(&["ignored"], 20);
    other[0].attrs = vec![kv_str("other", "z")];
    let w_mid = write_wal(dir.path(), vec![req(&other)], "mid");
    let w_old = write_wal(dir.path(), vec![req(&spans(&["a"], 10))], "old");
    let sources = vec![
        sealed_source(dir.path(), &w_new, "new"),
        sealed_source(dir.path(), &w_mid, "mid"),
        sealed_source(dir.path(), &w_old, "old"),
    ];
    let data = run(sources, q().max_files(2).stale_files(1));
    assert_eq!(items(&data), [("a", Prefix), ("c", Prefix)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

#[test]
fn the_deadline_stops_after_the_newest_source() {
    let dir = tempfile::tempdir().unwrap();
    let sources = three_files(dir.path(), &["c"], &["b"], &["a"]);
    let progress = Arc::new(AtomicUsize::new(0));
    let data = suggest(
        sources.clone(),
        q().deadline(Instant::now()),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(items(&data), [("c", Prefix)]);
    assert_eq!(data.status, partial(&[PartialReason::Deadline]));
    assert_eq!(data.complete_after, Some((BASE_S + 20 + 1) as u32));
    assert_eq!(progress.load(Ordering::Relaxed), 3, "a stop ticks the rest");

    let later = Instant::now() + Duration::from_secs(600);
    let data = run(sources, q().deadline(later));
    assert_eq!(data.items.len(), 3);
    assert_eq!(data.status, QueryStatus::Complete);
}

#[test]
fn unavailable_and_broken_sources_are_reported_and_the_rest_served() {
    let dir = tempfile::tempdir().unwrap();
    let w = write_wal(dir.path(), vec![req(&spans(&["a"], 10))], "ok");
    let sources = vec![
        sealed_source(dir.path(), &w, "ok"),
        unavailable_source("remote", BASE_S as u32, BASE_S as u32 + 5),
        missing_source(dir.path(), "gone", BASE_S as u32, BASE_S as u32 + 5),
    ];
    let data = run(sources, q());
    assert_eq!(items(&data), [("a", Prefix)]);
    assert_eq!(
        data.status,
        partial(&[
            PartialReason::SourceFailure,
            PartialReason::RemoteUnavailable
        ])
    );
}

#[test]
fn cancellation_is_all_or_empty_and_zero_limits_are_rejected() {
    let dir = tempfile::tempdir().unwrap();
    let w = write_wal(dir.path(), vec![req(&spans(&["a"], 10))], "ok");
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = suggest(
        vec![sealed_source(dir.path(), &w, "ok")],
        q(),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.items.is_empty());
    assert_eq!(data.status, partial(&[PartialReason::Cancelled]));

    for query in [q().max_items(0), q().max_files(0), q().stale_files(0)] {
        let err = suggest(
            Vec::new(),
            query,
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err();
        assert!(matches!(err, SuggestRequestError::ZeroLimit), "{err}");
    }
}

#[test]
fn keys_match_by_rendered_or_bare_name_with_their_highest_tier() {
    let dir = tempfile::tempdir().unwrap();
    // `hi` has 150 values in one file (mid tier) and 5 in another (low).
    let hi = |n: usize, at_s: u64| -> Vec<common::SpanSpec> {
        (0..n)
            .map(|i| {
                let mut s = sp(
                    (i % 250) as u8 + 1,
                    0,
                    (BASE_S + at_s) * 1_000_000_000,
                    "op",
                );
                s.attrs = vec![kv_str("hi", &format!("h{i:03}"))];
                s
            })
            .collect()
    };
    let mid = write_wal(dir.path(), vec![req(&hi(150, 20))], "mid");
    let low = write_wal(dir.path(), vec![req(&hi(5, 10))], "low");
    let t = write_wal(dir.path(), vec![req(&spans(&["x"], 30))], "t");
    let sources = vec![
        sealed_source(dir.path(), &mid, "mid"),
        sealed_source(dir.path(), &low, "low"),
        tail_source(&t, "t"),
    ];
    let keys = |term: &str| {
        run(
            sources.clone(),
            SuggestQuery::keys().term(term).max_items(50),
        )
    };

    let data = keys("hi");
    assert_eq!(
        data.items[0],
        Suggestion {
            text: "attributes.hi".into(),
            truncated: false,
            class: Exact,
            tier: Some(FieldTier::Mid),
            enumerable: true,
            kinds: kinds_of(&[sfst::ValueKind::Str]),
        },
        "the bare name is exact; the higher tier wins"
    );

    // Users type attribute names, not owners.
    let data = keys("serv");
    assert_eq!(
        items(&data)[0],
        ("resource.attributes.service.name", Prefix)
    );

    // Every builtin is a key; one no source carries has no tier.
    let data = keys("");
    let tier_of = |text: &str| data.items.iter().find(|s| s.text == text).map(|s| s.tier);
    assert_eq!(tier_of("duration"), Some(None));
    let duration = data.items.iter().find(|s| s.text == "duration").unwrap();
    assert!(
        !duration.enumerable,
        "a virtual key has no values to suggest"
    );
    assert_eq!(tier_of("name"), Some(Some(FieldTier::Low)));
    assert_eq!(
        tier_of("attributes.k"),
        Some(Some(FieldTier::Low)),
        "from the tail"
    );
    assert_eq!(data.status, QueryStatus::Complete);
}

/// `n` spans carrying distinct values `v0000..` of span attribute `key`,
/// at `at_s` seconds past [`BASE_S`].
fn many(key: &str, n: usize, at_s: u64) -> Vec<common::SpanSpec> {
    (0..n)
        .map(|i| {
            let mut s = sp(
                (i % 250) as u8 + 1,
                0,
                (BASE_S + at_s) * 1_000_000_000,
                "op",
            );
            s.attrs = vec![kv_str(key, &format!("v{i:04}"))];
            s
        })
        .collect()
}

#[test]
fn the_deadline_can_stop_a_later_source_mid_scan() {
    // The newest source is scanned whole; the older one is cut at the
    // first check past the deadline, and counts as not visited.
    let dir = tempfile::tempdir().unwrap();
    let new = write_wal(dir.path(), vec![req(&spans(&["c"], 30))], "new");
    let reqs = many("k", 2000, 20).chunks(250).map(req).collect();
    let old = write_wal(dir.path(), reqs, "old");
    let progress = Arc::new(AtomicUsize::new(0));
    let data = suggest(
        vec![
            sealed_source(dir.path(), &old, "old"),
            sealed_source(dir.path(), &new, "new"),
        ],
        q().deadline_after_values(1000),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(data.status, partial(&[PartialReason::Deadline]));
    assert_eq!(data.complete_after, Some((BASE_S + 20 + 1) as u32));
    assert_eq!(
        progress.load(Ordering::Relaxed),
        2,
        "each source counted once"
    );
    assert!(!data.items.is_empty());
}

#[test]
fn the_window_prunes_sealed_and_unavailable_sources_but_never_the_tail() {
    let dir = tempfile::tempdir().unwrap();
    let sources = three_files(dir.path(), &["c"], &["b"], &["a"]);
    let mut sources = sources;
    sources.push(unavailable_source(
        "remote",
        (BASE_S + 50) as u32,
        (BASE_S + 55) as u32,
    ));
    let tail = write_wal(dir.path(), vec![req(&spans(&["t"], 40))], "tail");
    sources.push(tail_source(&tail, "tail"));
    let ns = |s: u64| ((BASE_S + s) * 1_000_000_000) as i64;
    let window = sfsq::traces::TimeWindow::new(ns(15), ns(25)).unwrap();
    let data = run(sources, q().window(window));
    assert_eq!(
        items(&data),
        [("b", Prefix), ("t", Prefix)],
        "a tail has no summary to prune by: it is read whole"
    );
    assert_eq!(data.status, QueryStatus::Complete);
}

#[test]
fn every_builtin_is_a_key_without_any_source() {
    let data = run(Vec::new(), SuggestQuery::keys().max_items(200));
    let mut keys: Vec<String> = data.items.into_iter().map(|s| s.text).collect();
    keys.sort();
    let mut want: Vec<String> = BuiltinField::ALL
        .iter()
        .map(|b| b.key().to_string())
        .collect();
    want.sort();
    assert_eq!(keys, want, "the builtin keys are static");
    assert_eq!(data.status, QueryStatus::Complete);
}

#[test]
fn a_tails_key_tier_follows_its_value_count() {
    let dir = tempfile::tempdir().unwrap();
    let t = write_wal(
        dir.path(),
        many("t", 150, 30).chunks(250).map(req).collect(),
        "t",
    );
    let data = run(
        vec![tail_source(&t, "t")],
        SuggestQuery::keys().term("attributes.t"),
    );
    assert_eq!(
        data.items
            .iter()
            .find(|s| s.text == "attributes.t")
            .map(|s| s.tier),
        Some(Some(FieldTier::Mid))
    );
}

/// A key's kinds are every leaf kind any visited source stored under it,
/// mixed in one source or across sources, sealed, chunk or tail, whatever
/// order the walk visits them in; it holds a number when any source
/// stored one; builtins carry none, whatever their storage holds.
#[test]
fn key_kinds_union_every_source() {
    let dir = tempfile::tempdir().unwrap();
    let mut old = sp(1, 0, (BASE_S + 10) * 1_000_000_000, "op");
    old.attrs = vec![
        kv_int("code", 200),
        kv_int("mixed", 3),
        kv_str("late", "z"),
        kv_str("label", "a"),
        kv_int("both", 1),
    ];
    // One source holding a key as a number and as text: both kinds.
    let mut old_text = sp(4, 0, (BASE_S + 10) * 1_000_000_000, "op");
    old_text.attrs = vec![kv_str("both", "x")];
    // Sources are visited newest first: `mixed` arrives as text and the
    // older file adds its number; `late` arrives as a number and the
    // older file adds its text.
    let mut new = sp(2, 0, (BASE_S + 20) * 1_000_000_000, "op");
    new.attrs = vec![kv_str("mixed", "x"), kv_int("late", 7)];
    new.events = vec![("retry", vec![kv_int("attempt", 2)])];
    let mut tail = sp(3, 0, (BASE_S + 30) * 1_000_000_000, "op");
    tail.attrs = vec![
        kv_double("ratio", 0.5),
        kv_str("tail_label", "b"),
        kv_int("tail_both", 1),
    ];
    let mut tail_text = sp(5, 0, (BASE_S + 30) * 1_000_000_000, "op");
    tail_text.attrs = vec![kv_str("tail_both", "y")];
    let old = write_wal(dir.path(), vec![req(&[old, old_text])], "old");
    let new = write_wal(dir.path(), vec![req(&[new])], "new");
    let tail = write_wal(dir.path(), vec![req(&[tail, tail_text])], "tail");
    let data = run(
        vec![
            sealed_source(dir.path(), &old, "old"),
            memory_source(&new, "new"),
            tail_source(&tail, "tail"),
        ],
        SuggestQuery::keys().max_items(200),
    );

    use sfst::ValueKind::{Double, Int, Str};
    let kinds = |text: &str| {
        data.items
            .iter()
            .find(|s| s.text == text)
            .unwrap_or_else(|| panic!("{text} is a key"))
            .kinds
    };
    for (text, want) in [
        ("events.attributes.attempt", kinds_of(&[Int])),
        ("attributes.both", kinds_of(&[Int, Str])),
        ("attributes.code", kinds_of(&[Int])),
        ("attributes.label", kinds_of(&[Str])),
        ("attributes.late", kinds_of(&[Int, Str])),
        ("attributes.mixed", kinds_of(&[Int, Str])),
        ("attributes.ratio", kinds_of(&[Double])),
        ("attributes.tail_both", kinds_of(&[Int, Str])),
        ("attributes.tail_label", kinds_of(&[Str])),
        ("name", kinds_of(&[])),
        ("kind", kinds_of(&[])),
        ("status.code", kinds_of(&[])),
    ] {
        assert_eq!(kinds(text), want, "{text}");
    }
    let mut numeric: Vec<&str> = data
        .items
        .iter()
        .filter(|s| s.kinds.has_number())
        .map(|s| s.text.as_str())
        .collect();
    numeric.sort();
    assert_eq!(
        numeric,
        [
            "attributes.both",
            "attributes.code",
            "attributes.late",
            "attributes.mixed",
            "attributes.ratio",
            "attributes.tail_both",
            "events.attributes.attempt",
        ]
    );
    for text in [
        "attributes.label",
        "attributes.tail_label",
        "name",
        "kind",
        "status.code",
    ] {
        assert!(data.items.iter().any(|s| s.text == text), "{text} is a key");
    }
    assert_eq!(data.status, QueryStatus::Complete);
}

/// The same frames served sealed and as a tail suggest the same keys
/// with the same kinds: both derive them from the same schema tree.
#[test]
fn sealed_and_tail_kinds_agree() {
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, (BASE_S + 10) * 1_000_000_000, "op");
    a.attrs = vec![
        kv_int("code", 200),
        kv_str("mixed", "x"),
        kv_double("ratio", 0.5),
    ];
    a.events = vec![("retry", vec![kv_int("attempt", 2)])];
    let mut b = sp(2, 0, (BASE_S + 11) * 1_000_000_000, "op");
    b.attrs = vec![kv_int("mixed", 1), kv_str("label", "l")];
    let wal = write_wal(dir.path(), vec![req(&[a, b])], "w");
    let kinds = |data: SuggestData| -> Vec<(String, Vec<sfst::ValueKind>)> {
        data.items
            .into_iter()
            .map(|s| (s.text, s.kinds.iter().collect()))
            .collect()
    };
    let query = || SuggestQuery::keys().max_items(200);
    let sealed = kinds(run(vec![sealed_source(dir.path(), &wal, "s")], query()));
    let tail = kinds(run(vec![tail_source(&wal, "t")], query()));
    assert_eq!(sealed, tail);
    use sfst::ValueKind::{Int, Str};
    assert!(sealed.contains(&("attributes.mixed".to_string(), vec![Int, Str])));
}

/// A kind set from its members.
fn kinds_of(list: &[sfst::ValueKind]) -> sfst::ValueKinds {
    list.iter().copied().collect()
}

/// Keys spread over every owner and all three source shapes: each lands
/// under its owner, every builtin is a key, the internal facets and the
/// bare `trace_state` field never surface (a span attribute named
/// `trace_state` does), and the answer does not depend on source order.
#[test]
fn keys_partition_by_owner_whatever_the_source_order() {
    let dir = tempfile::tempdir().unwrap();
    let mut a1 = sp(1, 0, BASE_S * 1_000_000_000, "op-a");
    a1.kind = 2;
    a1.status = Some((2, "boom"));
    a1.trace_state = "ot=th:8";
    a1.attrs = vec![kv_str("trace_state", "user-attr"), kv_int("retries", 3)];
    let wal_a = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc-a"), kv_str("host", "h1")],
            Some(("mylib", "1.2", vec![kv_str("lang", "rust")])),
            &[a1],
        )],
        "part-a",
    );
    let mut b1 = sp(2, 1, (BASE_S + 1) * 1_000_000_000, "op-b");
    b1.events = vec![("retry", vec![kv_int("attempt", 1)])];
    b1.links = vec![([0xCD; 16], [9; 8], vec![kv_str("rel", "follows")])];
    let wal_b = write_wal(dir.path(), vec![req(&[b1])], "part-b");
    let mut c1 = sp(3, 1, (BASE_S + 2) * 1_000_000_000, "op-c");
    c1.attrs = vec![kv_double("ratio", 0.5)];
    let wal_c = write_wal(dir.path(), vec![req(&[c1])], "part-c");
    let sources = vec![
        sealed_source(dir.path(), &wal_a, "sealed-a"),
        memory_source(&wal_b, "chunk-b"),
        tail_source(&wal_c, "tail-c"),
    ];

    let keys_of = |sources: Vec<TraceSource>| -> Vec<String> {
        let data = run(sources, SuggestQuery::keys().max_items(200));
        assert_eq!(data.status, QueryStatus::Complete);
        assert!(!data.truncated);
        let mut keys: Vec<String> = data.items.into_iter().map(|s| s.text).collect();
        keys.sort();
        keys
    };
    let keys = keys_of(sources.clone());
    for want in [
        "resource.attributes.host",
        "resource.attributes.service.name",
        "attributes.ratio",
        "attributes.retries",
        "attributes.trace_state",
        "scope.attributes.lang",
        "events.attributes.attempt",
        "links.attributes.rel",
    ] {
        assert!(keys.iter().any(|k| k == want), "{want} in {keys:?}");
    }
    for builtin in BuiltinField::ALL {
        let key = builtin.key().to_string();
        assert!(keys.contains(&key), "builtin {key} in {keys:?}");
    }
    for internal in ["_kind", "_status_code", "trace_state"] {
        assert!(
            !keys
                .iter()
                .any(|k| k.ends_with(internal) && !k.starts_with("attributes.")),
            "{internal} must not surface: {keys:?}"
        );
    }

    let mut reversed = sources;
    reversed.reverse();
    assert_eq!(keys_of(reversed), keys);
}

/// Dictionary-backed builtins suggest their stored labels; a virtual
/// builtin has no values and asking for them is a request error.
#[test]
fn builtin_values_are_storage_labels_and_virtual_builtins_have_none() {
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, BASE_S * 1_000_000_000, "op-a");
    a.kind = 2;
    a.status = Some((2, "boom"));
    a.events = vec![("retry", Vec::new())];
    // No kind and no status: the OTel defaults are stored values too.
    let b = sp(2, 1, BASE_S * 1_000_000_000, "op-b");
    let wal = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc")],
            Some(("mylib", "1.2", Vec::new())),
            &[a, b],
        )],
        "i",
    );
    let builtin = |field: BuiltinField| {
        SuggestQuery::values(AttributeOwner::Builtin, AttributeKey::Builtin(field))
    };
    let texts = |field: BuiltinField| -> Vec<String> {
        run(vec![sealed_source(dir.path(), &wal, "f")], builtin(field))
            .items
            .into_iter()
            .map(|s| s.text)
            .collect()
    };
    assert_eq!(texts(BuiltinField::Name), ["op-a", "op-b"]);
    assert_eq!(texts(BuiltinField::Kind), ["SERVER", "UNSPECIFIED"]);
    assert_eq!(texts(BuiltinField::Status), ["ERROR", "UNSET"]);
    assert_eq!(texts(BuiltinField::StatusMessage), ["boom"]);
    assert_eq!(texts(BuiltinField::InstrumentationName), ["mylib"]);
    assert_eq!(texts(BuiltinField::InstrumentationVersion), ["1.2"]);
    assert_eq!(texts(BuiltinField::EventName), ["retry"]);

    for virt in [
        BuiltinField::Duration,
        BuiltinField::SpanId,
        BuiltinField::ParentSpanId,
        BuiltinField::TraceId,
        BuiltinField::LinkSpanId,
        BuiltinField::LinkTraceId,
        BuiltinField::EventTimeSinceStart,
        BuiltinField::RootName,
        BuiltinField::RootServiceName,
        BuiltinField::TraceDuration,
    ] {
        let err = suggest(
            vec![sealed_source(dir.path(), &wal, "f")],
            builtin(virt),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err();
        assert!(
            matches!(err, SuggestRequestError::NotEnumerable(i) if i == virt),
            "virtual {virt:?} must be NotEnumerable"
        );
    }
}

/// The owner/key pairings of pin C4, the `Any` owner and duplicate
/// source ids are request errors before anything is read; a key no
/// source carries is a complete, empty answer.
#[test]
fn bad_requests_are_rejected_and_an_absent_key_is_empty() {
    let fails = |sources: Vec<TraceSource>, query: SuggestQuery| {
        suggest(
            sources,
            query,
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err()
    };
    assert!(matches!(
        fails(
            Vec::new(),
            SuggestQuery::values(
                AttributeOwner::Span,
                AttributeKey::Builtin(BuiltinField::Name)
            )
        ),
        SuggestRequestError::BuiltinKeyOutsideBuiltinOwner(AttributeOwner::Span)
    ));
    assert!(matches!(
        fails(
            Vec::new(),
            SuggestQuery::values(AttributeOwner::Builtin, AttributeKey::Attribute("k".into()))
        ),
        SuggestRequestError::AttributeKeyUnderBuiltinOwner(a) if a == "k"
    ));
    let dup = |id: &str| {
        TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(id.to_string()),
            summary: sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                record_count: 0,
                content_meta: Vec::new(),
            },
            source: sfsq::Source::File("/dev/null".into()),
            coverage: None,
        })
    };
    assert!(matches!(
        fails(vec![dup("same"), dup("same")], q()),
        SuggestRequestError::SourceSet(_)
    ));

    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&spans(&["x"], 0))], "a");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "f"), tail_source(&wal, "t")],
        SuggestQuery::values(AttributeOwner::Link, AttributeKey::Attribute("nope".into())),
    );
    assert!(data.items.is_empty());
    assert!(!data.truncated);
    assert_eq!(data.status, QueryStatus::Complete);
}
