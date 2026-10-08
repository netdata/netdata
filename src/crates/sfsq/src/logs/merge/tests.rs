//! Unit tests for the cross-file merge helpers (the parent `merge`
//! module): the pure folds behind `LogsShard::merge` — facet values,
//! timeline buckets, and field tables. Fixtures are hand-built `sfst`
//! values; nothing is read from disk.
//!
//! Pins:
//!
//! - `merge_facet_results` — fields union by name and per-value counts
//!   sum across files; fields and values come out lexicographic
//!   (BTreeMap order, matching each file's FST iteration order) whatever
//!   the input order; empty input stays empty; a union over
//!   `MAX_FACET_VALUES` keeps the top-count values
//!   (lexicographically-first among ties), bounding the wire payload.
//! - `merge_timelines` — dimensions union in sorted order and each
//!   file's counts are reindexed onto the union columns before the
//!   bucket-wise sum (`unset` sums too); the merged grid is the inputs'
//!   shared grid; empty input yields `None`. Mismatched grids are never
//!   fed in — that precondition remains a runtime `assert` in the merge.
//! - `merge_field_tables` — every field is kept: a field high-card in
//!   any input is marked `High` instead of dropped (nested merges and
//!   the root `available_fields` drop must still see it), and the
//!   merged cardinality is the max across inputs.
use super::*;
use crate::logs::cursor::NS_PER_S;

/// Terse `sfst::Bucket` constructor for the timeline tests.
fn bucket(counts: Vec<u64>, unset: u64) -> sfst::Bucket {
    sfst::Bucket { counts, unset }
}

#[test]
fn merge_facet_results_unions_fields_and_sums_counts() {
    // File A: level={info:3, error:1}, service={api:4}
    // File B: level={info:2, warn:5}, host={a:1}
    // Counts sum per value across files; the union comes out sorted by
    // field and by value (BTreeMap order) although A's values aren't.
    let file_a = vec![
        sfst::FacetResult {
            field: "level".into(),
            values: vec![("info".into(), 3), ("error".into(), 1)],
        },
        sfst::FacetResult {
            field: "service".into(),
            values: vec![("api".into(), 4)],
        },
    ];
    let file_b = vec![
        sfst::FacetResult {
            field: "level".into(),
            values: vec![("info".into(), 2), ("warn".into(), 5)],
        },
        sfst::FacetResult {
            field: "host".into(),
            values: vec![("a".into(), 1)],
        },
    ];

    let merged = merge_facet_results(vec![file_a, file_b]);

    let field_names: Vec<&str> = merged.iter().map(|f| f.field.as_str()).collect();
    assert_eq!(field_names, vec!["host", "level", "service"]);

    let level = merged.iter().find(|f| f.field == "level").unwrap();
    assert_eq!(
        level.values,
        vec![("error".into(), 1), ("info".into(), 5), ("warn".into(), 5)]
    );

    let svc = merged.iter().find(|f| f.field == "service").unwrap();
    assert_eq!(svc.values, vec![("api".into(), 4)]);

    let host = merged.iter().find(|f| f.field == "host").unwrap();
    assert_eq!(host.values, vec![("a".into(), 1)]);
}

#[test]
fn merge_facet_results_empty_input_yields_empty() {
    let merged = merge_facet_results(Vec::new());
    assert!(merged.is_empty());
}

#[test]
fn merge_timelines_unions_dimensions_and_sums_buckets() {
    // Same grid (3 buckets × 2s starting at 100s).
    // File A dims [error, info], file B dims [debug, info].
    // Merged dims [debug, error, info] (BTreeSet order).
    let start = 100 * NS_PER_S;
    let width = 2 * NS_PER_S;

    let grid = sfst::Grid::new(start, width, 3);
    let a = sfst::Timeline {
        grid,
        dimensions: vec!["error".into(), "info".into()],
        buckets: vec![
            bucket(vec![1, 2], 1),
            bucket(vec![0, 3], 0),
            bucket(vec![4, 0], 2),
        ],
    };
    let b = sfst::Timeline {
        grid,
        dimensions: vec!["debug".into(), "info".into()],
        buckets: vec![
            bucket(vec![5, 0], 0),
            bucket(vec![1, 1], 3),
            bucket(vec![0, 0], 0),
        ],
    };

    let merged = merge_timelines(vec![a, b]).unwrap();
    assert_eq!(merged.grid.bucket_start_ns, start);
    assert_eq!(merged.grid.bucket_width_ns, width);
    assert_eq!(merged.dimensions, vec!["debug", "error", "info"]);

    // File A stores its counts as [error, info] but the union is
    // [debug, error, info]: each file is reindexed onto the union
    // columns before the bucket-wise sum, so adding by position would
    // scramble the counts. `unset` sums too.
    assert_eq!(merged.buckets[0].counts, vec![5, 1, 2]);
    assert_eq!(merged.buckets[0].unset, 1);
    assert_eq!(merged.buckets[1].counts, vec![1, 0, 4]);
    assert_eq!(merged.buckets[1].unset, 3);
    assert_eq!(merged.buckets[2].counts, vec![0, 4, 0]);
    assert_eq!(merged.buckets[2].unset, 2);
}

#[test]
fn merge_timelines_empty_input_yields_none() {
    assert!(merge_timelines(Vec::new()).is_none());
}

#[test]
fn merge_field_tables_marks_field_high_if_high_in_any_file() {
    // File A: `level` is Low (card 3); File B: `level` is High (card 50k).
    // The merge KEEPS `level` but bumps it to High, so a nested merge and
    // the root-level available-fields drop still see it as unofferable.
    // The actual drop happens at the root, not here.
    let a: sfst::FieldTable = vec![
        sfst::FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: sfst::FieldTier::Low,
        },
        sfst::FieldEntry {
            name: "service".into(),
            cardinality: 5,
            tier: sfst::FieldTier::Low,
        },
    ]
    .into();
    let b: sfst::FieldTable = vec![
        sfst::FieldEntry {
            name: "level".into(),
            cardinality: 50_000,
            tier: sfst::FieldTier::High,
        },
        sfst::FieldEntry {
            name: "host".into(),
            cardinality: 10,
            tier: sfst::FieldTier::Low,
        },
    ]
    .into();

    let merged = merge_field_tables(&[a, b]);
    let names: Vec<&str> = merged.iter().map(|f| f.name.as_str()).collect();
    // All fields kept, sorted by name; `level` survives as High.
    assert_eq!(names, vec!["host", "level", "service"]);
    let level = merged.get("level").expect("level kept");
    assert!(level.is_high_card());
    assert_eq!(level.cardinality, 50_000);
}

#[test]
fn merge_field_tables_keeps_max_cardinality() {
    // Same field across files with different cardinalities: union keeps
    // the max as a conservative estimate.
    let a: sfst::FieldTable = vec![sfst::FieldEntry {
        name: "level".into(),
        cardinality: 3,
        tier: sfst::FieldTier::Low,
    }]
    .into();
    let b: sfst::FieldTable = vec![sfst::FieldEntry {
        name: "level".into(),
        cardinality: 20,
        tier: sfst::FieldTier::Mid,
    }]
    .into();
    let merged = merge_field_tables(&[a, b]);
    assert_eq!(merged.len(), 1);
    assert_eq!(merged[0].name, "level");
    assert_eq!(merged[0].cardinality, 20);
}

/// A union exceeding `MAX_FACET_VALUES` keeps the top values by count
/// (lexicographically-first among ties) and stays lexicographically
/// ordered — the wire payload is bounded no matter how many distinct
/// values the window unions (e.g. near-unique-per-log journald fields).
#[test]
fn merge_facet_results_caps_values_at_hard_limit() {
    // 1500 distinct values, zero-padded so lexicographic == numeric order.
    // v0000..v0199 are "popular" (count 5); the rest have count 1.
    let values: Vec<(String, u32)> = (0..1500)
        .map(|i| (format!("v{i:04}"), if i < 200 { 5 } else { 1 }))
        .collect();
    let file = vec![sfst::FacetResult {
        field: "seq".into(),
        values,
    }];

    let merged = merge_facet_results(vec![file]);
    let f = &merged[0];
    assert_eq!(f.values.len(), MAX_FACET_VALUES);
    // All popular values survive.
    assert!(f.values.iter().filter(|(_, c)| *c == 5).count() == 200);
    // Ties (count 1) keep the lexicographically-first 800: v0200..v0999.
    assert!(f.values.iter().any(|(v, _)| v == "v0999"));
    assert!(!f.values.iter().any(|(v, _)| v == "v1000"));
    // Output remains lexicographically ordered.
    assert!(f.values.windows(2).all(|w| w[0].0 < w[1].0));
}
