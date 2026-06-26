use super::*;

#[test]
fn into_query_maps_histogram_and_anchor_forms() {
    // Empty histogram → the builder's default; a cursor string parses to
    // Anchor::Cursor.
    let req: OtelLogsRequest =
        serde_json::from_slice(br#"{"histogram":"","anchor":"100:2:0:3"}"#).unwrap();
    let q = req.into_query().unwrap();
    assert_eq!(q.histogram_field(), "severity_text");
    assert!(matches!(
        q.anchor(),
        Some(Anchor::Cursor(c))
            if c.timestamp_ns == 100 && c.file_seq == 2 && c.part == sfsq::logs::Part::Indexed(0) && c.position == 3
    ));

    // Non-empty histogram is carried through; a bare µs integer becomes
    // an Anchor::Timestamp in nanoseconds.
    let req: OtelLogsRequest =
        serde_json::from_slice(br#"{"histogram":"service","anchor":5000}"#).unwrap();
    let q = req.into_query().unwrap();
    assert_eq!(q.histogram_field(), "service");
    assert!(matches!(q.anchor(), Some(Anchor::Timestamp(5_000_000))));

    // A malformed cursor string is dropped → no anchor.
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"anchor":"not-a-cursor"}"#).unwrap();
    let q = req.into_query().unwrap();
    assert!(q.anchor().is_none());
}

#[test]
fn into_query_wires_and_validates_full_text_query() {
    // A non-empty `query` is carried onto the engine query verbatim.
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"query":".*GoDaddy.*"}"#).unwrap();
    assert_eq!(req.into_query().unwrap().query(), Some(".*GoDaddy.*"));

    // An empty `query` (the UI default) carries no query.
    let req: OtelLogsRequest = serde_json::from_slice(br#"{}"#).unwrap();
    assert_eq!(req.into_query().unwrap().query(), None);

    // A malformed query regex is a hard error at the boundary.
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"query":"("}"#).unwrap();
    assert!(req.into_query().is_err());
}

#[test]
fn bucket_width_picks_from_curated_set() {
    // 15-minute window → 15s (largest in VALID_BUCKET_WIDTHS_S with
    // span/w >= TARGET_BUCKETS=60).
    assert_eq!(bucket_width_for_span_s(900), 15);
    // 1-minute window → 1s buckets (60 / 1 == 60).
    assert_eq!(bucket_width_for_span_s(60), 1);
    // Very small spans (< TARGET_BUCKETS seconds) → 1s fallback.
    assert_eq!(bucket_width_for_span_s(30), 1);
    // 1-hour window → 60s buckets (3600 / 60 == 60).
    assert_eq!(bucket_width_for_span_s(3600), 60);
    // 1-day window → 1800s (30-min) buckets (86400 / 1800 == 48 < 60,
    // 86400 / 900 == 96 >= 60 → 900s wins).
    assert_eq!(bucket_width_for_span_s(86400), 900);
}

#[test]
fn align_window_snaps_outward_to_bucket_boundaries() {
    // Identity when already aligned.
    assert_eq!(align_window(0, 900, 15), (0, 900));
    // Floor the `after`, ceil the `before`.
    assert_eq!(align_window(1, 14, 15), (0, 15));
    // Larger window — both bounds rounded outward.
    assert_eq!(align_window(7, 92, 15), (0, 105));
    // Consecutive 1-second shifts within the same bucket-width slot
    // produce the same aligned window — this is what kills the chart's
    // sub-bucket shape jitter across the UI's per-second polling.
    let a = align_window(1779995982, 1779996882, 15);
    let b = align_window(1779995983, 1779996883, 15);
    let c = align_window(1779995984, 1779996884, 15);
    assert_eq!(a, b);
    assert_eq!(b, c);
}

#[test]
fn window_secs_recovers_the_grid_window() {
    // 60 buckets × 15s starting at 1_700_000_000s.
    let after = 1_700_000_000i64;
    let grid = sfst::Grid::new(after * NS_PER_S, 15 * NS_PER_S, 60);
    let win = window_secs(&grid);
    assert_eq!(win, 1_700_000_000..1_700_000_900);
}

#[test]
fn facet_preserves_value_order_and_counts() {
    let f = sfst::FacetResult {
        field: "level".into(),
        values: vec![("error".into(), 3), ("info".into(), 5)],
    };
    let wire = facet_from_sfst(7, &f);
    assert_eq!(wire.id, "level");
    assert_eq!(wire.name, "level");
    assert_eq!(wire.order, 7);
    assert_eq!(wire.options.len(), 2);
    assert_eq!(wire.options[0].id, "error");
    assert_eq!(wire.options[0].count, 3);
    assert_eq!(wire.options[0].order, 0);
    assert_eq!(wire.options[1].id, "info");
    assert_eq!(wire.options[1].count, 5);
    assert_eq!(wire.options[1].order, 1);
}

#[test]
fn facet_with_no_values_yields_empty_options() {
    let f = sfst::FacetResult {
        field: "service".into(),
        values: Vec::new(),
    };
    let wire = facet_from_sfst(0, &f);
    assert!(wire.options.is_empty());
}

#[test]
fn histogram_emits_one_datapoint_per_bucket() {
    // 3 buckets × 2 value dimensions + the "(unset)" trailer.
    let t = sfst::Timeline {
        grid: sfst::Grid::new(1_700_000_000 * NS_PER_S, 2 * NS_PER_S, 3),
        dimensions: vec!["error".into(), "info".into()],
        buckets: vec![
            sfst::Bucket {
                counts: vec![1, 4],
                unset: 2,
            },
            sfst::Bucket {
                counts: vec![0, 3],
                unset: 1,
            },
            sfst::Bucket {
                counts: vec![2, 2],
                unset: 0,
            },
        ],
    };

    let h = histogram_from_sfst("level", &t);

    assert_eq!(h.id, "level");
    assert_eq!(h.chart.view.after, 1_700_000_000);
    assert_eq!(h.chart.view.before, 1_700_000_000 + 6);
    assert_eq!(h.chart.view.update_every, 2);
    assert_eq!(h.chart.view.chart_type, "stackedBar");
    assert_eq!(
        h.chart.view.dimensions.ids,
        vec!["error", "info", "(unset)"]
    );
    assert_eq!(
        h.chart.view.dimensions.units,
        vec!["events", "events", "events"]
    );

    // labels: ["time", value dims..., "(unset)"].
    assert_eq!(
        h.chart.result.labels,
        vec!["time", "error", "info", "(unset)"]
    );

    let dps = &h.chart.result.data;
    assert_eq!(dps.len(), 3);
    // Each DataPoint carries value dims + "(unset)" as the trailing triple.
    assert_eq!(dps[0].items, vec![[1, 0, 0], [4, 0, 0], [2, 0, 0]]);
    assert_eq!(dps[1].items, vec![[0, 0, 0], [3, 0, 0], [1, 0, 0]]);
    assert_eq!(dps[2].items, vec![[2, 0, 0], [2, 0, 0], [0, 0, 0]]);
}

#[test]
fn histogram_with_zero_buckets_still_well_formed() {
    let t = sfst::Timeline {
        grid: sfst::Grid::new(0, NS_PER_S, 0),
        dimensions: Vec::new(),
        buckets: Vec::new(),
    };
    let h = histogram_from_sfst("severity_text", &t);
    assert!(h.chart.result.data.is_empty());
    // Even with no value dims, the "(unset)" label is part of the
    // dimension list — that's the legacy wire shape's invariant
    // (result.labels = ["time"] + value-dims + ["(unset)"]).
    assert_eq!(h.chart.result.labels, vec!["time", "(unset)"]);
    assert_eq!(h.chart.view.dimensions.ids, vec!["(unset)"]);
}

#[test]
fn available_histograms_enumerates_fields_in_order() {
    // The engine already excludes high-card fields, so the converter is
    // a straight enumeration in field order.
    let fields: sfst::FieldTable = vec![
        sfst::FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: sfst::FieldTier::Low,
        },
        sfst::FieldEntry {
            name: "host".into(),
            cardinality: 200,
            tier: sfst::FieldTier::Mid,
        },
    ]
    .into();
    let av = available_histograms_from_fields(&fields);
    let names: Vec<&str> = av.iter().map(|a| a.name.as_str()).collect();
    assert_eq!(names, vec!["level", "host"]);
    assert_eq!(av[0].order, 0);
    assert_eq!(av[1].order, 1);
}

/// Multi-valued fields (repeated keys on one row) join into a single cell
/// in row order with duplicates skipped; the dedicated severity cell takes
/// the last `severity_text` pair (the indexer interns the projected
/// top-level severity after all attributes).
#[test]
fn build_table_joins_multivalued_fields_and_keeps_last_severity() {
    let cursor = sfsq::logs::Cursor {
        timestamp_ns: 1_000_000_000,
        file_seq: 1,
        part: sfsq::logs::Part::Indexed(0),
        position: 0,
    };
    let row = sfst::MaterializedRow {
        timestamp_ns: 1_000_000_000,
        fields: vec![
            ("severity_text".into(), "attr-noise".into()),
            ("tags".into(), "a".into()),
            ("tags".into(), "b".into()),
            ("tags".into(), "a".into()), // duplicate — skipped
            ("plain".into(), "x".into()),
            ("severity_text".into(), "ERROR".into()), // projected, last
        ],
    };
    let fields = vec!["tags".to_string(), "plain".to_string()];
    let (_, data) = build_table(&[(cursor, row)], &fields, &BTreeSet::new());

    let rows = data.as_array().unwrap();
    let cells = rows[0].as_array().unwrap();
    // [timestamp, severity, cursor, tags, plain]
    assert_eq!(cells[1], serde_json::json!("ERROR"));
    assert_eq!(cells[3], serde_json::json!("a, b"));
    assert_eq!(cells[4], serde_json::json!("x"));
}

fn dummy_cursor(timestamp_ns: i64) -> sfsq::logs::Cursor {
    sfsq::logs::Cursor {
        timestamp_ns,
        file_seq: 1,
        part: sfsq::logs::Part::Indexed(0),
        position: 0,
    }
}

#[test]
fn group_row_fields_dedups_and_preserves_stream_order() {
    let row = sfst::MaterializedRow {
        timestamp_ns: 0,
        fields: vec![
            ("a".into(), "1".into()),
            ("b".into(), "2".into()),
            ("a".into(), "1".into()), // exact duplicate — skipped
            ("a".into(), "3".into()), // new value — appended after "1"
        ],
    };
    let grouped = group_row_fields(&row);
    assert_eq!(grouped["a"], vec!["1", "3"]);
    assert_eq!(grouped["b"], vec!["2"]);
}

#[test]
fn build_row_cells_layout_join_severity_and_missing() {
    let cursor = dummy_cursor(5_000); // 5 µs
    let row = sfst::MaterializedRow {
        timestamp_ns: 5_000,
        fields: vec![
            ("severity_text".into(), "noise".into()),
            ("severity_text".into(), "ERROR".into()), // last wins
            ("tags".into(), "a".into()),
            ("tags".into(), "b".into()),
        ],
    };
    let fields = vec!["tags".to_string(), "absent".to_string()];
    let cells = build_row_cells(&cursor, &row, &fields);
    // [timestamp_µs, severity, cursor, tags, absent]
    assert_eq!(cells.len(), 5);
    assert_eq!(cells[0], serde_json::json!(5)); // 5_000 ns / 1_000
    assert_eq!(cells[1], serde_json::json!("ERROR")); // last severity_text
    assert_eq!(cells[2], serde_json::json!(cursor.encode()));
    assert_eq!(cells[3], serde_json::json!("a, b")); // multi-value join
    assert_eq!(cells[4], serde_json::Value::Null); // absent field
}

#[test]
fn take_partition_keys_decodes_hex_skips_garbage_and_removes_key() {
    let mut req: OtelLogsRequest = serde_json::from_slice(
        br#"{"selections":{"__streams":["000000000000002a","ff","nothex"],"level":["error"]}}"#,
    )
    .unwrap();
    let hashes = req.take_partition_keys();
    // 0x2a = 42, 0xff = 255; the non-hex entry is skipped, not fatal.
    assert_eq!(hashes, vec![42, 255]);
    // The reserved key is consumed so the engine never sees it as a facet;
    // a genuine facet selection is left intact.
    assert!(!req.selections.contains_key("__streams"));
    assert_eq!(
        req.selections.get("level"),
        Some(&vec!["error".to_string()])
    );
}

#[test]
fn take_partition_keys_absent_selection_is_all() {
    let mut req: OtelLogsRequest = serde_json::from_slice(br#"{}"#).unwrap();
    // No selection → empty vec → Query reads it as "all streams".
    assert!(req.take_partition_keys().is_empty());
}

#[test]
fn stream_required_params_builds_all_default_selected_selector() {
    // Build neutral PartitionStats (opaque content_meta) — `stream_required_params`
    // decodes and sorts them, so this exercises the real codec + display path.
    let enc = |ns: &str, name: &str| {
        otel_logs_identity::encode_content_meta(&ServiceStream::new(ns, name)).unwrap()
    };
    let stats = vec![
        PartitionStat {
            part_key: 0x2a,
            content_meta: enc("prod", "api"),
            total_size: 2048,
            file_count: 2,
            min_timestamp_s: Some(100),
            max_timestamp_s: Some(160),
        },
        PartitionStat {
            part_key: 0,
            content_meta: enc("", ""),
            total_size: 0,
            file_count: 1,
            min_timestamp_s: None,
            max_timestamp_s: None,
        },
    ];
    let rp = stream_required_params(stats);
    assert_eq!(rp.len(), 1);
    match &rp[0] {
        RequiredParam::MultiSelection(ms) => {
            assert_eq!(ms.id, "__streams");
            assert_eq!(ms.type_, "multiselect");
            assert_eq!(ms.options.len(), 2);
            // Options are sorted by (namespace, name): the absent-namespace
            // ("","") stream sorts before ("prod","api"). This guards the decode
            // + sort the adapter performs over the substrate's part_key-ordered fold.
            assert_eq!(ms.options[0].name, "(unattributed)");
            assert_eq!(ms.options[1].name, "prod/api");
            // Every option pre-selected so the default view spans all streams.
            assert!(ms.options.iter().all(|o| o.default_selected));
            // Option id is the ns_hash as 16-digit lowercase hex.
            let api = ms.options.iter().find(|o| o.name == "prod/api").unwrap();
            assert_eq!(api.id, "000000000000002a");
            assert_eq!(api.pill, "2.0 KiB");
            assert_eq!(api.info, "2 files · spans 1m");
            // An all-empty stream renders explicitly, not as a blank label.
            let empty = ms
                .options
                .iter()
                .find(|o| o.id == "0000000000000000")
                .unwrap();
            assert_eq!(empty.name, "(unattributed)");
            assert_eq!(empty.info, "1 file");
        }
    }
}

#[test]
fn stream_required_params_empty_when_no_streams() {
    assert!(stream_required_params(Vec::new()).is_empty());
}

#[test]
fn humanize_bytes_uses_binary_units() {
    assert_eq!(humanize_bytes(0), "0 B");
    assert_eq!(humanize_bytes(512), "512 B");
    assert_eq!(humanize_bytes(2048), "2.0 KiB");
    assert_eq!(humanize_bytes(1024 * 1024 * 3 / 2), "1.5 MiB");
}

#[test]
fn build_row_cells_single_value_is_bare_and_no_severity_is_null() {
    let cursor = dummy_cursor(0);
    let row = sfst::MaterializedRow {
        timestamp_ns: 0,
        fields: vec![("host".into(), "api-1".into())],
    };
    let cells = build_row_cells(&cursor, &row, &["host".to_string()]);
    assert_eq!(cells[1], serde_json::Value::Null); // no severity_text
    assert_eq!(cells[3], serde_json::json!("api-1")); // single value, not joined
}
