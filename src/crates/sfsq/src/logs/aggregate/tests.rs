use super::*;

#[test]
fn eligible_facet_fields_honors_explicit_request() {
    // Explicit selections are kept as-is; no cardinality cap. A mid-card
    // field the user asked for survives (it's present and not high-card).
    let fields: sfst::FieldTable = vec![sfst::FieldEntry {
        name: "noisy".into(),
        cardinality: 500,
        tier: sfst::FieldTier::Mid,
    }]
    .into();
    let picked = eligible_facet_fields(&["noisy".to_string()], &fields);
    assert_eq!(picked, vec!["noisy".to_string()]);
}

#[test]
fn eligible_facet_fields_drops_high_card_and_unknown() {
    // A high-card field would make facets() error; an unknown field has
    // no entry. Both are dropped, leaving only the usable one.
    let fields: sfst::FieldTable = vec![
        sfst::FieldEntry {
            name: "trace_id".into(),
            cardinality: 50_000,
            tier: sfst::FieldTier::High,
        },
        sfst::FieldEntry {
            name: "service".into(),
            cardinality: 5,
            tier: sfst::FieldTier::Low,
        },
    ]
    .into();
    let picked = eligible_facet_fields(
        &[
            "trace_id".to_string(),
            "service".to_string(),
            "ghost".to_string(),
        ],
        &fields,
    );
    assert_eq!(picked, vec!["service".to_string()]);
}

#[test]
fn merge_sums_matched_and_drops_facet_high_card_in_any_shard() {
    // Shard A computed a `level` facet (Low here); shard B has `level`
    // High and produced no facet for it. The merge sums matched and drops
    // the `level` facet entirely — it's high-card in B, so offering it
    // would be inconsistent with `available_fields`.
    let shard_a = LogsShard {
        matched: 3,
        facets: vec![sfst::FacetResult {
            field: "level".into(),
            values: vec![("info".into(), 3)],
        }],
        timeline: None,
        fields: vec![sfst::FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: sfst::FieldTier::Low,
        }]
        .into(),
    };
    let shard_b = LogsShard {
        matched: 2,
        facets: Vec::new(),
        timeline: None,
        fields: vec![sfst::FieldEntry {
            name: "level".into(),
            cardinality: 50_000,
            tier: sfst::FieldTier::High,
        }]
        .into(),
    };

    let merged = LogsShard::merge(vec![shard_a, shard_b]);
    assert_eq!(merged.matched, 5);
    assert!(merged.facets.is_empty(), "high-card `level` facet dropped");
    assert!(merged.fields.get("level").unwrap().is_high_card());
    assert!(merged.timeline.is_none());
}

#[test]
fn merge_empty_is_identity() {
    let merged = LogsShard::merge(Vec::new());
    assert_eq!(merged.matched, 0);
    assert!(merged.facets.is_empty());
    assert!(merged.timeline.is_none());
    assert!(merged.fields.is_empty());
}

#[test]
fn merge_ignores_interspersed_default_shards() {
    // M-3 relies on this: a failed source degrades to `LogsShard::default()`,
    // so injecting default shards into the merge must not change the result
    // (they are the monoid identity). Stronger than `merge_empty_is_identity`,
    // which only covers the all-empty input.
    let real = || LogsShard {
        matched: 4,
        facets: vec![sfst::FacetResult {
            field: "level".into(),
            values: vec![("info".into(), 4)],
        }],
        timeline: None,
        fields: vec![sfst::FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: sfst::FieldTier::Low,
        }]
        .into(),
    };

    let alone = LogsShard::merge(vec![real()]);
    let with_defaults = LogsShard::merge(vec![LogsShard::default(), real(), LogsShard::default()]);

    assert_eq!(with_defaults.matched, alone.matched);
    assert_eq!(with_defaults.facets, alone.facets);
    assert_eq!(with_defaults.timeline, alone.timeline);
    assert_eq!(
        with_defaults.fields.get("level").map(|e| e.cardinality),
        alone.fields.get("level").map(|e| e.cardinality),
    );
}
