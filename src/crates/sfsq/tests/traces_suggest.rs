//! `suggest` over real sources: sealed files of every tier, in-memory
//! chunks, WAL tails, unavailable and broken sources.
//!
//! Pins: ranking by match class then bytes across tiers and the tail;
//! the item cap with an exact `truncated`; newest-first early stops
//! (source budget, no new matches, deadline) with their reason and
//! `complete_after`; sources without the key cost no budget; unavailable
//! and broken sources are reported; cancellation is all-or-empty; zero
//! limits are rejected.

mod common;

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use common::{
    kv_str, memory_source, missing_source, req, sealed_source, sp, tail_source, unavailable_source,
    write_wal,
};
use sfsq::traces::{
    AttributeKey, AttributeOwner, AttributeRequestError, PartialReason, QueryStatus, SuggestData,
    SuggestQuery, Suggestion, TraceSource, suggest,
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
        assert!(matches!(err, AttributeRequestError::ZeroLimit), "{err}");
    }
}

/// A test renderer: lowercase owner word + `.` + name; builtins by
/// their lowercased variant name.
fn render(owner: AttributeOwner, key: &AttributeKey) -> String {
    match key {
        AttributeKey::Builtin(b) => format!("{b:?}").to_lowercase(),
        AttributeKey::Attribute(a) => format!("{}.{a}", format!("{owner:?}").to_lowercase()),
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
            SuggestQuery::keys(render).term(term).max_items(50),
        )
    };

    let data = keys("hi");
    assert_eq!(
        data.items[0],
        Suggestion {
            text: "span.hi".into(),
            truncated: false,
            class: Exact,
            tier: Some(FieldTier::Mid),
            enumerable: true,
        },
        "the bare name is exact; the higher tier wins"
    );

    // Users type attribute names, not owners.
    let data = keys("serv");
    assert_eq!(items(&data)[0], ("resource.service.name", Prefix));

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
        tier_of("span.k"),
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
fn the_window_prunes_sealed_and_unavailable_sources() {
    let dir = tempfile::tempdir().unwrap();
    let sources = three_files(dir.path(), &["c"], &["b"], &["a"]);
    let mut sources = sources;
    sources.push(unavailable_source(
        "remote",
        (BASE_S + 50) as u32,
        (BASE_S + 55) as u32,
    ));
    let ns = |s: u64| ((BASE_S + s) * 1_000_000_000) as i64;
    let window = sfsq::traces::TimeWindow::new(ns(15), ns(25)).unwrap();
    let data = run(sources, q().window(window));
    assert_eq!(items(&data), [("b", Prefix)]);
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
        SuggestQuery::keys(render).term("span.t"),
    );
    assert_eq!(
        data.items
            .iter()
            .find(|s| s.text == "span.t")
            .map(|s| s.tier),
        Some(Some(FieldTier::Mid))
    );
}
