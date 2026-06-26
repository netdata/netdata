//! Unit tests for the row-scan evaluator's semantics. These exercise the
//! evaluation core against hand-built row sets; the WAL-file end and the
//! full equivalence against the SFST engine over identical frames are
//! covered by the property harness in `wal_equivalence.rs`.

use sfst::{FieldTier, Filter, Grid};
use wal_otap::KvSink;

use super::{ScanSink, WalScan};
use crate::logs::{LogsQuery, LogsQueryBuilder};

/// Build a [`WalScan`] from `(timestamp, pairs)` rows, going through the
/// same sink the real scan uses (string dedup included).
fn scan_from(rows: &[(i64, &[&str])]) -> WalScan {
    let mut sink = ScanSink::default();
    for &(ts, pairs) in rows {
        let tokens: Vec<u32> = pairs.iter().map(|kv| sink.intern(None, kv)).collect();
        sink.row(ts, &tokens);
    }
    sink.finish()
}

/// `[0, 100)` in one bucket — a window that covers every fixture row.
fn wide_grid() -> Grid {
    Grid::new(0, 100, 1)
}

fn query(grid: Grid) -> LogsQueryBuilder {
    LogsQueryBuilder::new(grid)
}

fn facet<'a>(shard: &'a crate::logs::LogsShard, field: &str) -> &'a sfst::FacetResult {
    shard
        .facets
        .iter()
        .find(|f| f.field == field)
        .unwrap_or_else(|| panic!("no facet for {field}"))
}

fn run(scan: &WalScan, q: LogsQuery) -> crate::logs::LogsShard {
    scan.evaluate(&q)
}

#[test]
fn empty_filter_matches_everything_in_window() {
    let scan = scan_from(&[
        (5, &["level=info"]),
        (15, &["level=error"]),
        (150, &["level=error"]), // outside the grid window
    ]);
    let shard = run(&scan, query(wide_grid()).build());
    assert_eq!(shard.matched, 2);
}

#[test]
fn window_is_half_open() {
    // Grid [10, 20): ts=10 in, ts=20 out.
    let scan = scan_from(&[(10, &["a=x"]), (20, &["a=x"])]);
    let shard = run(&scan, query(Grid::new(10, 10, 1)).build());
    assert_eq!(shard.matched, 1);
}

#[test]
fn filter_is_or_within_field_and_across_fields() {
    let scan = scan_from(&[
        (1, &["level=info", "host=a"]),
        (2, &["level=error", "host=a"]),
        (3, &["level=error", "host=b"]),
        (4, &["level=debug", "host=a"]),
    ]);
    let f = Filter::new()
        .select("level", "info")
        .select("level", "error")
        .select("host", "a");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    // (info OR error) AND host=a → rows 1, 2.
    assert_eq!(shard.matched, 2);
}

#[test]
fn filter_on_absent_field_matches_nothing() {
    let scan = scan_from(&[(1, &["a=x"])]);
    let f = Filter::new().select("missing", "x");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    assert_eq!(shard.matched, 0);
}

#[test]
fn pattern_matcher_is_full_value_anchored() {
    let scan = scan_from(&[(1, &["level=error"]), (2, &["level=err"])]);
    // `err` matches only the exact value "err"…
    let f = Filter::new().select_pattern("level", "err");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    assert_eq!(shard.matched, 1);
    // …while `err.*` matches both.
    let f = Filter::new().select_pattern("level", "err.*");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    assert_eq!(shard.matched, 2);
}

#[test]
fn full_text_query_is_unanchored_over_whole_pairs() {
    let scan = scan_from(&[
        (1, &["msg=alpha beta", "level=info"]),
        (2, &["msg=gamma", "level=info"]),
        (3, &["alphabet=x", "level=error"]),
    ]);
    // Matches inside a value (row 1) and inside a key (row 3).
    let shard = run(&scan, query(wide_grid()).query("alpha").build());
    assert_eq!(shard.matched, 2);

    // A global AND term: narrows the filter too.
    let f = Filter::new().select("level", "info");
    let shard = run(&scan, query(wide_grid()).query("alpha").filter(f).build());
    assert_eq!(shard.matched, 1);
}

#[test]
fn exact_filter_field_containing_eq_matches_nothing() {
    // External-review regression (divergence #1): the stored pair
    // `a=b=c` has field `a` and value `b=c` (first-`=` split). A filter
    // on field `a=b` with exact value `c` concatenates to the same
    // `a=b=c` string — but no field named `a=b` exists, so the SFST
    // path (gated on `locate_field`) matches nothing, and so must we.
    let scan = scan_from(&[(1, &["a=b=c"])]);
    let f = Filter::new().select("a=b", "c");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    assert_eq!(shard.matched, 0);

    // The legitimate spelling of the same selection still matches.
    let f = Filter::new().select("a", "b=c");
    let shard = run(&scan, query(wide_grid()).filter(f).build());
    assert_eq!(shard.matched, 1);
}

#[test]
fn invalid_pattern_on_absent_field_is_not_an_error() {
    // External-review regression (divergence #2): the SFST path
    // resolves an absent filter field to the empty set *before*
    // compiling its patterns, so a malformed pattern on an absent
    // field is not an error — the result is a normal zero-match shard
    // (facet and timeline structure intact), not the fields-only
    // degrade shape.
    let scan = scan_from(&[(1, &["level=info"])]);
    let f = Filter::new().select_pattern("missing", "(unclosed");
    let shard = run(
        &scan,
        query(wide_grid())
            .filter(f)
            .facet_fields(vec!["level".into()])
            .histogram_field("level")
            .build(),
    );
    assert_eq!(shard.matched, 0);
    // The facet exists, scoped to the (empty) rest of the filter — no
    // values survive, but the entry is present.
    assert_eq!(shard.facets.len(), 1);
    assert!(shard.facets[0].values.is_empty());
    // The timeline exists too: dimensions enumerated from the file,
    // all counts zero under the empty scope.
    let timeline = shard.timeline.expect("timeline");
    assert_eq!(timeline.dimensions, vec!["info"]);
    assert_eq!(timeline.buckets[0].counts, vec![0]);
    assert_eq!(timeline.buckets[0].unset, 0);
}

#[test]
fn invalid_pattern_degrades_to_fields_only() {
    let scan = scan_from(&[(1, &["a=x"])]);
    let f = Filter::new().select_pattern("a", "(unclosed");
    let shard = run(
        &scan,
        query(wide_grid())
            .filter(f)
            .facet_fields(vec!["a".into()])
            .build(),
    );
    assert_eq!(shard.matched, 0);
    assert!(shard.facets.is_empty());
    assert!(shard.timeline.is_none());
    assert!(shard.fields.get("a").is_some(), "field table still present");
}

#[test]
fn facet_excludes_its_own_selection() {
    let scan = scan_from(&[
        (1, &["level=info", "host=a"]),
        (2, &["level=error", "host=a"]),
        (3, &["level=error", "host=b"]),
    ]);
    let f = Filter::new().select("level", "error").select("host", "a");
    let shard = run(
        &scan,
        query(wide_grid())
            .filter(f)
            .facet_fields(vec!["level".into(), "host".into()])
            .build(),
    );
    // level facet: scoped by host=a only → info:1, error:1 (lex order).
    assert_eq!(
        facet(&shard, "level").values,
        vec![("error".to_string(), 1), ("info".to_string(), 1)]
    );
    // host facet: scoped by level=error only → a:1, b:1.
    assert_eq!(
        facet(&shard, "host").values,
        vec![("a".to_string(), 1), ("b".to_string(), 1)]
    );
}

#[test]
fn facet_counts_each_row_once_per_distinct_value() {
    // Row 1 carries tag=a twice (duplicate pair) and tag=b once: the
    // duplicate counts once (bitmap semantics), the two distinct values
    // count in both.
    let scan = scan_from(&[
        (1, &["tag=a", "tag=a", "tag=b"]),
        (2, &["tag=a"]),
        (3, &["other=x"]),
    ]);
    let shard = run(
        &scan,
        query(wide_grid()).facet_fields(vec!["tag".into()]).build(),
    );
    assert_eq!(
        facet(&shard, "tag").values,
        vec![("a".to_string(), 2), ("b".to_string(), 1)]
    );
}

#[test]
fn facets_skip_absent_and_high_card_fields() {
    // 1000 distinct values of `id` → high-card at the default threshold.
    let id_pairs: Vec<String> = (0..1000).map(|i| format!("id={i:04}")).collect();
    let id_refs: Vec<&str> = id_pairs.iter().map(String::as_str).collect();
    let scan = scan_from(&[(1, &id_refs[..]), (2, &["level=info"])]);

    let shard = run(
        &scan,
        query(wide_grid())
            .facet_fields(vec!["id".into(), "missing".into(), "level".into()])
            .build(),
    );
    assert_eq!(shard.facets.len(), 1, "only the eligible facet remains");
    assert_eq!(shard.facets[0].field, "level");
}

#[test]
fn facets_clip_to_the_window() {
    let scan = scan_from(&[(5, &["tag=in"]), (500, &["tag=out"])]);
    let shard = run(
        &scan,
        query(wide_grid()).facet_fields(vec!["tag".into()]).build(),
    );
    assert_eq!(facet(&shard, "tag").values, vec![("in".to_string(), 1)]);
}

#[test]
fn timeline_unset_is_exact_for_multivalued_fields() {
    // The multi-valued fixture from the SFST timeline tests: row 0
    // carries two values of `lang`, row 1 one, row 2 none.
    let scan = scan_from(&[
        (1, &["lang=en", "lang=fr", "svc=a"]),
        (2, &["lang=en", "svc=b"]),
        (3, &["svc=a"]),
    ]);
    let shard = run(&scan, query(wide_grid()).histogram_field("lang").build());
    let timeline = shard.timeline.expect("timeline");
    assert_eq!(timeline.dimensions, vec!["en", "fr"]);
    assert_eq!(timeline.buckets[0].counts, vec![2, 1]);
    // Row 0 counts in two dimensions but has the field — only row 2 is
    // unset. (`total − Σcounts` would wrongly yield 0.)
    assert_eq!(timeline.buckets[0].unset, 1);
}

#[test]
fn timeline_buckets_by_timestamp() {
    // Grid [0, 30) in three buckets of 10; edge timestamps land in the
    // bucket they open.
    let scan = scan_from(&[
        (0, &["level=a"]),
        (9, &["level=a"]),
        (10, &["level=a"]),
        (29, &["level=b"]),
    ]);
    let shard = run(
        &scan,
        query(Grid::new(0, 10, 3)).histogram_field("level").build(),
    );
    let timeline = shard.timeline.expect("timeline");
    assert_eq!(timeline.dimensions, vec!["a", "b"]);
    let counts: Vec<Vec<u64>> = timeline.buckets.iter().map(|b| b.counts.clone()).collect();
    assert_eq!(counts, vec![vec![2, 0], vec![1, 0], vec![0, 1]]);
}

#[test]
fn timeline_excludes_own_selection_and_keeps_zero_count_dimensions() {
    let scan = scan_from(&[
        (1, &["level=info", "host=a"]),
        (2, &["level=error", "host=a"]),
        (3, &["level=debug", "host=b"]),
    ]);
    let f = Filter::new().select("level", "error").select("host", "a");
    let shard = run(
        &scan,
        query(wide_grid())
            .filter(f)
            .histogram_field("level")
            .build(),
    );
    let timeline = shard.timeline.expect("timeline");
    // Dimensions enumerate every value of `level` in the file — `debug`
    // stays as a zero-count dimension (its rows fail host=a).
    assert_eq!(timeline.dimensions, vec!["debug", "error", "info"]);
    assert_eq!(timeline.buckets[0].counts, vec![0, 1, 1]);
    assert_eq!(timeline.buckets[0].unset, 0);
}

#[test]
fn timeline_absent_field_routes_matches_to_unset() {
    let scan = scan_from(&[(1, &["a=x"]), (2, &["a=y"])]);
    let shard = run(&scan, query(wide_grid()).histogram_field("missing").build());
    let timeline = shard.timeline.expect("timeline");
    assert!(timeline.dimensions.is_empty());
    assert_eq!(timeline.buckets[0].counts, Vec::<u64>::new());
    assert_eq!(timeline.buckets[0].unset, 2);
}

#[test]
fn timeline_high_card_field_yields_none() {
    let id_pairs: Vec<String> = (0..1000).map(|i| format!("id={i:04}")).collect();
    let id_refs: Vec<&str> = id_pairs.iter().map(String::as_str).collect();
    let scan = scan_from(&[(1, &id_refs[..])]);
    let shard = run(&scan, query(wide_grid()).histogram_field("id").build());
    assert!(shard.timeline.is_none());
}

#[test]
fn field_table_tiers_and_order_match_the_indexer_rule() {
    // 99 distinct → low; 100 → mid; 1000 → high (threshold 100, 10×).
    let mut pairs: Vec<String> = Vec::new();
    pairs.extend((0..99).map(|i| format!("zlow={i:03}")));
    pairs.extend((0..100).map(|i| format!("amid={i:03}")));
    pairs.extend((0..1000).map(|i| format!("high={i:04}")));
    let refs: Vec<&str> = pairs.iter().map(String::as_str).collect();
    let scan = scan_from(&[(1, &refs[..])]);

    let shard = run(&scan, query(wide_grid()).build());
    let entries: Vec<(&str, u32, FieldTier)> = shard
        .fields
        .iter()
        .map(|f| (f.name.as_str(), f.cardinality, f.tier))
        .collect();
    // Low → mid → high, sorted by name within each tier.
    assert_eq!(
        entries,
        vec![
            ("zlow", 99, FieldTier::Low),
            ("amid", 100, FieldTier::Mid),
            ("high", 1000, FieldTier::High),
        ]
    );
}
