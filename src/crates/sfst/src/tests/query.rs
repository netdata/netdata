//! Query API tests for [`IndexReader::matched_count`],
//! [`IndexReader::matched_positions`], [`IndexReader::facets`], and
//! [`IndexReader::timeline`].

use crate::*;

/// Compile `filter` against `reader` — test convenience for the
/// `BitmapFilter`-taking query methods.
fn bf(reader: &IndexReader<'_>, filter: Filter) -> BitmapFilter {
    reader.compile_filter(&filter, None).unwrap()
}

/// Compile `filter` + a full-text `query` against `reader`.
fn bfq(reader: &IndexReader<'_>, filter: Filter, query: &str) -> BitmapFilter {
    reader.compile_filter(&filter, Some(query)).unwrap()
}

/// Synthetic SFST for query tests. 6 logs, 1 second apart.
///
/// `level` (low-card): `info` at positions 0, 2, 4; `error` at 1, 3, 5.
/// `service` (low-card): `api` at 0, 1, 2; `worker` at 3, 4, 5.
fn build_query_fixture() -> Vec<u8> {
    let mut data = Vec::new();
    let lvl_info = treight::Bitmap::from_sorted_iter([0, 2, 4].into_iter(), 6, &mut data);
    let lvl_info_data = std::mem::take(&mut data);
    let lvl_error = treight::Bitmap::from_sorted_iter([1, 3, 5].into_iter(), 6, &mut data);
    let lvl_error_data = std::mem::take(&mut data);
    let svc_api = treight::Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 6, &mut data);
    let svc_api_data = std::mem::take(&mut data);
    let svc_worker = treight::Bitmap::from_sorted_iter([3, 4, 5].into_iter(), 6, &mut data);
    let svc_worker_data = data;

    // FST iteration order is lexicographic.
    // KvId 0=level=error, 1=level=info, 2=service=api, 3=service=worker.
    let primary_entries: Vec<(&str, BitmapValue)> = vec![
        (
            "level=error",
            BitmapValue {
                desc: lvl_error,
                data: lvl_error_data,
            },
        ),
        (
            "level=info",
            BitmapValue {
                desc: lvl_info,
                data: lvl_info_data,
            },
        ),
        (
            "service=api",
            BitmapValue {
                desc: svc_api,
                data: svc_api_data,
            },
        ),
        (
            "service=worker",
            BitmapValue {
                desc: svc_worker,
                data: svc_worker_data,
            },
        ),
    ];

    // Spread across 6 seconds for predictable bucketing.
    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_000_005,
        record_count: 6,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![6],
        },
        id_ranges: IdRanges {
            low_end: KvId(4),
            mid_end: KvId(4),
            high_end: KvId(4),
        },
        tree: SchemaTree::flat(
            &vec![
                FieldEntry {
                    name: "level".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
                FieldEntry {
                    name: "service".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };
    let timestamps: Vec<i64> = (0..6)
        .map(|i| 1_700_000_000i64 * 1_000_000_000 + i * 1_000_000_000)
        .collect();
    // Each log has one level + one service KvId (4 distinct combinations).
    let stream_entries: Vec<Vec<KvId>> = vec![
        vec![KvId(1), KvId(2)], // pos 0: info, api
        vec![KvId(0), KvId(2)], // pos 1: error, api
        vec![KvId(1), KvId(2)], // pos 2: info, api
        vec![KvId(0), KvId(3)], // pos 3: error, worker
        vec![KvId(1), KvId(3)], // pos 4: info, worker
        vec![KvId(0), KvId(3)], // pos 5: error, worker
    ];

    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(primary_entries).unwrap();
    writer
        .add_stream_batch(&StreamBatch::for_write(&stream_entries))
        .unwrap();
    writer.finish().unwrap().into_inner()
}

/// Window covering the fixture's whole log range (all 6 logs).
const FULL_WINDOW: std::ops::Range<i64> = FILE_MIN_NS..(FILE_MIN_NS + 6 * 1_000_000_000);

#[test]
fn matched_empty_filter_matches_all() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let count = reader
        .matched_count(&bf(&reader, Filter::new()), FULL_WINDOW)
        .unwrap();
    assert_eq!(count, 6);
}

#[test]
fn matched_single_selection() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let positions = reader
        .matched_positions(
            &bf(&reader, Filter::new().select("level", "info")),
            FULL_WINDOW,
        )
        .unwrap();
    assert_eq!(positions, vec![0, 2, 4]);
}

#[test]
fn matched_or_within_field() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // level=info OR level=error → all positions.
    let filter = bf(
        &reader,
        Filter::new()
            .select("level", "info")
            .select("level", "error"),
    );
    let count = reader.matched_count(&filter, FULL_WINDOW).unwrap();
    assert_eq!(count, 6);
}

#[test]
fn matched_and_across_fields() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // level=info AND service=worker → only position 4.
    let filter = bf(
        &reader,
        Filter::new()
            .select("level", "info")
            .select("service", "worker"),
    );
    let positions = reader.matched_positions(&filter, FULL_WINDOW).unwrap();
    assert_eq!(positions, vec![4]);
}

#[test]
fn matched_unknown_field_yields_empty() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Unknown field → no matches in this file (not an error).
    let positions = reader
        .matched_positions(
            &bf(&reader, Filter::new().select("nonexistent", "anything")),
            FULL_WINDOW,
        )
        .unwrap();
    assert!(positions.is_empty());
}

#[test]
fn facets_show_all_values_with_self_exclusion() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Selecting `level=info` should NOT hide `level=error` from the
    // `level` facet — that's the whole point of self-exclusion.
    let filter = bf(&reader, Filter::new().select("level", "info"));
    // Window spans all 6 logs, so counts are unaffected by clipping.
    let results = reader
        .facets(
            &["level", "service"],
            &filter,
            FILE_MIN_NS..FILE_MIN_NS + 6 * 1_000_000_000,
        )
        .unwrap();

    // `level` facet sees both values (filter excluding `level` is
    // empty → full bitmap).
    let level = results.iter().find(|f| f.field == "level").unwrap();
    let level_counts: std::collections::HashMap<_, _> = level.values.iter().cloned().collect();
    assert_eq!(level_counts.get("info"), Some(&3));
    assert_eq!(level_counts.get("error"), Some(&3));

    // `service` facet sees both values under the filter `level=info`
    // (positions 0, 2, 4): service=api at pos 0, 2; service=worker at pos 4.
    let service = results.iter().find(|f| f.field == "service").unwrap();
    let svc_counts: std::collections::HashMap<_, _> = service.values.iter().cloned().collect();
    assert_eq!(svc_counts.get("api"), Some(&2));
    assert_eq!(svc_counts.get("worker"), Some(&1));
}

#[test]
fn facets_unknown_field_errors() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let err = reader
        .facets(
            &["nonexistent"],
            &bf(&reader, Filter::new()),
            FILE_MIN_NS..FILE_MIN_NS + 6 * 1_000_000_000,
        )
        .unwrap_err();
    assert!(matches!(err, Error::UnknownField(s) if s == "nonexistent"));
}

#[test]
fn facets_clip_to_window() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Window covers only positions 2 and 3 (logs at file_min + 2s and
    // + 3s). The fixture has level=info at 2 / error at 3, and
    // service=api at 2 / worker at 3 — so each facet value is counted
    // exactly once, vs. 3 each over the whole file.
    let window = (FILE_MIN_NS + 2 * 1_000_000_000)..(FILE_MIN_NS + 4 * 1_000_000_000);
    let results = reader
        .facets(&["level", "service"], &bf(&reader, Filter::new()), window)
        .unwrap();

    let level: std::collections::HashMap<_, _> = results
        .iter()
        .find(|f| f.field == "level")
        .unwrap()
        .values
        .iter()
        .cloned()
        .collect();
    assert_eq!(level.get("info"), Some(&1));
    assert_eq!(level.get("error"), Some(&1));

    let service: std::collections::HashMap<_, _> = results
        .iter()
        .find(|f| f.field == "service")
        .unwrap()
        .values
        .iter()
        .cloned()
        .collect();
    assert_eq!(service.get("api"), Some(&1));
    assert_eq!(service.get("worker"), Some(&1));
}

/// Fixture's file_min_ns — the first log's timestamp.
const FILE_MIN_NS: i64 = 1_700_000_000 * 1_000_000_000;

#[test]
fn timeline_buckets_match_filter() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // 6 logs spread across 6 seconds. Bucket width = 2 seconds.
    // Grid anchored at file_min, 3 buckets covering positions {0,1},
    // {2,3}, {4,5}.
    let timeline = reader
        .timeline(
            "level",
            &bf(&reader, Filter::new()),
            Grid::new(FILE_MIN_NS, 2 * 1_000_000_000, 3),
        )
        .unwrap();
    assert_eq!(timeline.grid.bucket_start_ns, FILE_MIN_NS);
    assert_eq!(timeline.grid.bucket_width_ns, 2_000_000_000);
    assert_eq!(timeline.buckets.len(), 3);
    // Dimensions are FST-iteration-order: "error", "info".
    assert_eq!(timeline.dimensions, vec!["error", "info"]);
    // Bucket 0 (pos 0-1): info=1, error=1
    assert_eq!(timeline.buckets[0].counts, vec![1, 1]);
    // Bucket 1 (pos 2-3): info=1, error=1
    assert_eq!(timeline.buckets[1].counts, vec![1, 1]);
    // Bucket 2 (pos 4-5): info=1, error=1
    assert_eq!(timeline.buckets[2].counts, vec![1, 1]);
}

#[test]
fn timeline_unset_counts_match_logs_missing_the_field() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Every log in the fixture has `level` set, so the unset
    // dimension should be zero across every bucket.
    let t = reader
        .timeline(
            "level",
            &bf(&reader, Filter::new()),
            Grid::new(FILE_MIN_NS, 2 * 1_000_000_000, 3),
        )
        .unwrap();
    assert!(t.buckets.iter().all(|b| b.unset == 0));
    // And the per-bucket dim sums equal the bucket totals (no
    // logs "fall off" the dimensions list — the partition is exact).
    for bucket in &t.buckets {
        let dim_sum: u64 = bucket.counts.iter().sum();
        // 2 logs per bucket in this fixture.
        assert_eq!(dim_sum + bucket.unset, 2);
    }
}

#[test]
fn timeline_excludes_own_field_from_filter() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Selecting `level=info` shouldn't collapse the `level` timeline
    // to a single dimension.
    let filter = bf(&reader, Filter::new().select("level", "info"));
    let timeline = reader
        .timeline(
            "level",
            &filter,
            Grid::new(FILE_MIN_NS, 6 * 1_000_000_000, 1),
        )
        .unwrap();
    // One bucket covering everything.
    assert_eq!(timeline.buckets.len(), 1);
    // Both dimensions visible.
    assert_eq!(timeline.dimensions, vec!["error", "info"]);
    // Counts reflect the full bitmap (filter excluded its own field).
    assert_eq!(timeline.buckets[0].counts, vec![3, 3]);
}

#[test]
fn timeline_invalid_bucket_width_errors() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let err = reader
        .timeline(
            "level",
            &bf(&reader, Filter::new()),
            Grid::new(FILE_MIN_NS, 0, 1),
        )
        .unwrap_err();
    assert!(matches!(err, Error::InvalidBucketWidth(0)));
}

#[test]
fn timeline_grid_before_file_yields_leading_zero_buckets() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Request grid starts 4 seconds before the file's first log and
    // runs 10 buckets of 1 second — so buckets 0..=3 cover times the
    // file has no data (expect zero counts), then buckets 4..=9 cover
    // the file's 6 logs one-per-bucket.
    let grid_start = FILE_MIN_NS - 4 * 1_000_000_000;
    let timeline = reader
        .timeline(
            "level",
            &bf(&reader, Filter::new()),
            Grid::new(grid_start, 1_000_000_000, 10),
        )
        .unwrap();
    assert_eq!(timeline.buckets.len(), 10);
    // Leading buckets all zero.
    for i in 0..4 {
        assert_eq!(
            timeline.buckets[i].counts,
            vec![0, 0],
            "bucket {i} should be empty"
        );
        assert_eq!(timeline.buckets[i].unset, 0);
    }
    // Each subsequent bucket holds one log; FST order puts "error"
    // first, then "info". Positions in the fixture: 0=info, 1=error,
    // 2=info, 3=error, 4=info, 5=error.
    let expected = [
        vec![0, 1], // pos 0: info
        vec![1, 0], // pos 1: error
        vec![0, 1], // pos 2: info
        vec![1, 0], // pos 3: error
        vec![0, 1], // pos 4: info
        vec![1, 0], // pos 5: error
    ];
    for (i, exp) in expected.iter().enumerate() {
        assert_eq!(
            timeline.buckets[i + 4].counts,
            *exp,
            "bucket {} mismatch",
            i + 4
        );
    }
}

/// Synthetic SFST with a multi-valued field. 3 logs, 1 second apart.
///
/// `lang` (low-card, multi-valued): log 0 carries BOTH `en` and `fr`,
/// log 1 carries `en`, log 2 has no `lang` at all.
/// `svc` (low-card): `a` at positions 0, 2; `b` at 1.
fn build_multivalued_fixture() -> Vec<u8> {
    let mut data = Vec::new();
    let lang_en = treight::Bitmap::from_sorted_iter([0, 1].into_iter(), 3, &mut data);
    let lang_en_data = std::mem::take(&mut data);
    let lang_fr = treight::Bitmap::from_sorted_iter([0].into_iter(), 3, &mut data);
    let lang_fr_data = std::mem::take(&mut data);
    let svc_a = treight::Bitmap::from_sorted_iter([0, 2].into_iter(), 3, &mut data);
    let svc_a_data = std::mem::take(&mut data);
    let svc_b = treight::Bitmap::from_sorted_iter([1].into_iter(), 3, &mut data);
    let svc_b_data = data;

    // FST iteration order is lexicographic.
    // KvId 0=lang=en, 1=lang=fr, 2=svc=a, 3=svc=b.
    let primary_entries: Vec<(&str, BitmapValue)> = vec![
        (
            "lang=en",
            BitmapValue {
                desc: lang_en,
                data: lang_en_data,
            },
        ),
        (
            "lang=fr",
            BitmapValue {
                desc: lang_fr,
                data: lang_fr_data,
            },
        ),
        (
            "svc=a",
            BitmapValue {
                desc: svc_a,
                data: svc_a_data,
            },
        ),
        (
            "svc=b",
            BitmapValue {
                desc: svc_b,
                data: svc_b_data,
            },
        ),
    ];

    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_000_002,
        record_count: 3,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![3],
        },
        id_ranges: IdRanges {
            low_end: KvId(4),
            mid_end: KvId(4),
            high_end: KvId(4),
        },
        tree: SchemaTree::flat(
            &vec![
                FieldEntry {
                    name: "lang".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
                FieldEntry {
                    name: "svc".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };
    let timestamps: Vec<i64> = (0..3)
        .map(|i| 1_700_000_000i64 * 1_000_000_000 + i * 1_000_000_000)
        .collect();
    let stream_entries: Vec<Vec<KvId>> = vec![
        vec![KvId(0), KvId(1), KvId(2)], // pos 0: lang=en, lang=fr, svc=a
        vec![KvId(0), KvId(3)],          // pos 1: lang=en, svc=b
        vec![KvId(2)],                   // pos 2: svc=a (no lang)
    ];

    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(primary_entries).unwrap();
    writer
        .add_stream_batch(&StreamBatch::for_write(&stream_entries))
        .unwrap();
    writer.finish().unwrap().into_inner()
}

/// `unset` must count logs *lacking the field*, not `total − Σ(counts)`:
/// a multi-valued log inflates the per-dimension sum and the subtraction
/// silently eats the unset count (saturating at zero).
#[test]
fn timeline_unset_with_multivalued_field() {
    let data = build_multivalued_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let window_ns = 3 * 1_000_000_000;

    // Fast path (no filter), one bucket covering all 3 logs. Log 0 counts
    // in both dimensions (en and fr); log 2 lacks `lang` entirely.
    let t = reader
        .timeline(
            "lang",
            &bf(&reader, Filter::new()),
            Grid::new(FILE_MIN_NS, window_ns, 1),
        )
        .unwrap();
    assert_eq!(t.dimensions, vec!["en", "fr"]);
    assert_eq!(t.buckets.len(), 1);
    assert_eq!(t.buckets[0].counts, vec![2, 1]); // dim_sum (3) == bucket_total (3)
    assert_eq!(t.buckets[0].unset, 1); // ...but log 2 still lacks the field

    // Filtered path (svc=a → positions {0, 2}). Log 0 still counts in
    // both dimensions; log 2 matches the filter and lacks `lang`.
    let t = reader
        .timeline(
            "lang",
            &bf(&reader, Filter::new().select("svc", "a")),
            Grid::new(FILE_MIN_NS, window_ns, 1),
        )
        .unwrap();
    assert_eq!(t.buckets[0].counts, vec![1, 1]);
    assert_eq!(t.buckets[0].unset, 1);
}

#[test]
fn materialize_rows_resolves_timestamp_and_attributes() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Fixture positions: 0 = (info, api), 3 = (error, worker); 1s apart
    // starting at FILE_MIN_NS. Stream KvIds resolve via the reverse
    // string table to "level=…"/"service=…" pairs.
    let rows = reader.materialize_rows(&[0, 3]).unwrap();
    assert_eq!(rows.len(), 2);

    assert_eq!(rows[0].timestamp_ns, FILE_MIN_NS);
    assert_eq!(
        rows[0].fields,
        vec![
            ("level".to_string(), "info".to_string()),
            ("service".to_string(), "api".to_string()),
        ]
    );

    assert_eq!(rows[1].timestamp_ns, FILE_MIN_NS + 3 * 1_000_000_000);
    assert_eq!(
        rows[1].fields,
        vec![
            ("level".to_string(), "error".to_string()),
            ("service".to_string(), "worker".to_string()),
        ]
    );
}

#[test]
fn materialize_field_matches_materialize_rows() {
    // Column-direct resolution must yield exactly the values the row-major
    // path extracts for the same field, across every position.
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let positions: Vec<u32> = (0..reader.summary().record_count).collect();
    let rows = reader.materialize_rows(&positions).unwrap();

    for field in ["level", "service"] {
        let direct = reader.materialize_field(field, &positions).unwrap();
        for (i, row) in rows.iter().enumerate() {
            let expected: Vec<String> = row
                .fields
                .iter()
                .filter(|(k, _)| k == field)
                .map(|(_, v)| v.clone())
                .collect();
            assert_eq!(direct[i], expected, "field {field} at position {i}");
        }
    }

    // Absent field → all-empty (parity with materialize_rows omitting it).
    let absent = reader.materialize_field("nope", &positions).unwrap();
    assert!(absent.iter().all(Vec::is_empty));
}

#[test]
fn materialize_field_multivalued() {
    // A multi-valued field returns every value at a position; absence is empty.
    let data = build_multivalued_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let got = reader.materialize_field("lang", &[0, 1, 2]).unwrap();
    // FST iteration is sorted, so en precedes fr.
    assert_eq!(got[0], vec!["en".to_string(), "fr".to_string()]);
    assert_eq!(got[1], vec!["en".to_string()]);
    assert!(got[2].is_empty(), "pos 2 has no lang");
}

#[test]
fn timeline_totals_counts_per_bucket() {
    // 6 logs at FILE_MIN_NS + {0..5}s.
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let all = bf(&reader, Filter::new());

    // 2-second buckets → two logs per bucket.
    let totals = reader
        .timeline_totals(&all, Grid::new(FILE_MIN_NS, 2 * 1_000_000_000, 3))
        .unwrap();
    assert_eq!(totals, vec![2, 2, 2]);

    // One bucket covering everything.
    let one = reader
        .timeline_totals(&all, Grid::new(FILE_MIN_NS, 6 * 1_000_000_000, 1))
        .unwrap();
    assert_eq!(one, vec![6]);

    // A grid entirely before the data yields empty buckets (natural clamp).
    let before = reader
        .timeline_totals(
            &all,
            Grid::new(FILE_MIN_NS - 6 * 1_000_000_000, 1_000_000_000, 3),
        )
        .unwrap();
    assert_eq!(before, vec![0, 0, 0]);

    // With a filter, the per-bucket totals still sum to the matched count.
    let filtered = bf(&reader, Filter::new().select("service", "api"));
    let totals = reader
        .timeline_totals(&filtered, Grid::new(FILE_MIN_NS, 2 * 1_000_000_000, 3))
        .unwrap();
    let matched = reader.matched_count(&filtered, FULL_WINDOW).unwrap();
    assert_eq!(totals.iter().sum::<u64>(), matched);
}

#[test]
fn materialize_rows_preserves_position_order() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Requested order is honored: one row per position, in order.
    let rows = reader.materialize_rows(&[5, 1]).unwrap();
    assert_eq!(rows.len(), 2);
    // pos 5 = (error, worker), pos 1 = (error, api).
    assert_eq!(rows[0].timestamp_ns, FILE_MIN_NS + 5 * 1_000_000_000);
    assert_eq!(
        rows[0].fields[0],
        ("level".to_string(), "error".to_string())
    );
    assert_eq!(rows[1].timestamp_ns, FILE_MIN_NS + 1_000_000_000);
    assert_eq!(
        rows[1].fields[1],
        ("service".to_string(), "api".to_string())
    );
}

#[test]
fn materialize_rows_errors_on_out_of_range_position() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // An out-of-range position (99 >= 6 logs) means the index is corrupt;
    // it must error, not silently skip — a skip would shorten the result
    // and misalign a caller pairing positions with rows by index.
    let err = reader.materialize_rows(&[5, 99, 1]).unwrap_err();
    assert!(
        matches!(err, crate::Error::CorruptIndex(_)),
        "expected CorruptIndex, got {err:?}"
    );
    assert!(
        format!("{err:?}").contains("99"),
        "error should name the offending position: {err:?}"
    );
}

#[test]
fn materialize_rows_empty_input_yields_empty() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    assert!(reader.materialize_rows(&[]).unwrap().is_empty());
}

#[test]
fn timeline_absent_field_routes_all_logs_to_unset() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // A field not present in this file: no dimensions, and every
    // matching log falls into `unset`. 6 logs over 3 two-second
    // buckets → 2 per bucket, all unset.
    let timeline = reader
        .timeline(
            "nonexistent",
            &bf(&reader, Filter::new()),
            Grid::new(FILE_MIN_NS, 2 * 1_000_000_000, 3),
        )
        .unwrap();
    assert!(timeline.dimensions.is_empty());
    // No dimensions → empty `counts`; every matching log lands in `unset`.
    for bucket in &timeline.buckets {
        assert!(bucket.counts.is_empty());
        assert_eq!(bucket.unset, 2);
    }
    assert_eq!(timeline.buckets.len(), 3);
}

/// Fixture with a value dense enough to be stored *complemented* (inverted
/// treight bitmap), mirroring what the writer's `remap_one_bitmap` does for
/// dense values. 6 logs:
///
/// `lvl` (low-card): `hi` at positions 0..=4 (5/6 → stored as the
/// complement `{5}`, inverted), `lo` at position 5.
/// `svc` (low-card): `a` at 0,1,2; `b` at 3,4,5.
fn build_complemented_fixture() -> Vec<u8> {
    let mut data = Vec::new();
    // `lvl=hi` covers 5 of 6 → store the complement {5} as an inverted bitmap.
    let lvl_hi = treight::Bitmap::from_sorted_iter_complemented([5].into_iter(), 6, &mut data);
    let lvl_hi_data = std::mem::take(&mut data);
    let lvl_lo = treight::Bitmap::from_sorted_iter([5].into_iter(), 6, &mut data);
    let lvl_lo_data = std::mem::take(&mut data);
    let svc_a = treight::Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 6, &mut data);
    let svc_a_data = std::mem::take(&mut data);
    let svc_b = treight::Bitmap::from_sorted_iter([3, 4, 5].into_iter(), 6, &mut data);
    let svc_b_data = data;

    // FST iteration order is lexicographic: lvl=hi(0), lvl=lo(1), svc=a(2), svc=b(3).
    let primary_entries: Vec<(&str, BitmapValue)> = vec![
        (
            "lvl=hi",
            BitmapValue {
                desc: lvl_hi,
                data: lvl_hi_data,
            },
        ),
        (
            "lvl=lo",
            BitmapValue {
                desc: lvl_lo,
                data: lvl_lo_data,
            },
        ),
        (
            "svc=a",
            BitmapValue {
                desc: svc_a,
                data: svc_a_data,
            },
        ),
        (
            "svc=b",
            BitmapValue {
                desc: svc_b,
                data: svc_b_data,
            },
        ),
    ];

    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_000_005,
        record_count: 6,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![6],
        },
        id_ranges: IdRanges {
            low_end: KvId(4),
            mid_end: KvId(4),
            high_end: KvId(4),
        },
        tree: SchemaTree::flat(
            &vec![
                FieldEntry {
                    name: "lvl".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
                FieldEntry {
                    name: "svc".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };
    let timestamps: Vec<i64> = (0..6)
        .map(|i| 1_700_000_000i64 * 1_000_000_000 + i * 1_000_000_000)
        .collect();
    let stream_entries: Vec<Vec<KvId>> = vec![
        vec![KvId(0), KvId(2)], // pos 0: hi, a
        vec![KvId(0), KvId(2)], // pos 1: hi, a
        vec![KvId(0), KvId(2)], // pos 2: hi, a
        vec![KvId(0), KvId(3)], // pos 3: hi, b
        vec![KvId(0), KvId(3)], // pos 4: hi, b
        vec![KvId(1), KvId(3)], // pos 5: lo, b
    ];

    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(primary_entries).unwrap();
    writer
        .add_stream_batch(&StreamBatch::for_write(&stream_entries))
        .unwrap();
    writer.finish().unwrap().into_inner()
}

#[test]
fn complemented_value_bitmap_counts_correctly() {
    let data = build_complemented_fixture();
    let reader = IndexReader::open(&data).unwrap();

    // `lvl=hi` is stored complemented (inverted). Exercise it through every
    // treight path and confirm the inverted representation is transparent.

    // Fast facet path: `range_cardinality` on the inverted bitmap.
    let facets = reader
        .facets(&["lvl"], &bf(&reader, Filter::new()), FULL_WINDOW)
        .unwrap();
    let lvl: std::collections::HashMap<_, _> = facets
        .iter()
        .find(|f| f.field == "lvl")
        .unwrap()
        .values
        .iter()
        .cloned()
        .collect();
    assert_eq!(lvl.get("hi"), Some(&5));
    assert_eq!(lvl.get("lo"), Some(&1));

    // matched_count / matched_positions: `from_value` on the inverted bitmap,
    // intersected with the (full) window range, then counted / iterated.
    let hi = bf(&reader, Filter::new().select("lvl", "hi"));
    assert_eq!(reader.matched_count(&hi, FULL_WINDOW).unwrap(), 5);
    assert_eq!(
        reader.matched_positions(&hi, FULL_WINDOW).unwrap(),
        vec![0, 1, 2, 3, 4]
    );

    // Intersection of the inverted bitmap with another field's set.
    let hi_and_a = bf(
        &reader,
        Filter::new().select("lvl", "hi").select("svc", "a"),
    );
    assert_eq!(reader.matched_count(&hi_and_a, FULL_WINDOW).unwrap(), 3);

    // Slow facet path: `value_counts_under` intersects the inverted bitmap
    // with a scope from a *different* filter field.
    let facets = reader
        .facets(
            &["lvl"],
            &bf(&reader, Filter::new().select("svc", "a")),
            FULL_WINDOW,
        )
        .unwrap();
    let lvl: std::collections::HashMap<_, _> = facets
        .iter()
        .find(|f| f.field == "lvl")
        .unwrap()
        .values
        .iter()
        .cloned()
        .collect();
    assert_eq!(lvl.get("hi"), Some(&3));
    assert_eq!(lvl.get("lo"), None); // lo ∩ {0,1,2} is empty → omitted
}

#[test]
fn timestamps_lookups() {
    // Ascending ns, with a duplicate at 20.
    let ts = Timestamps::new(vec![10, 20, 20, 40]);
    assert_eq!(ts.len(), 4);
    assert!(!ts.is_empty());

    // position -> time
    assert_eq!(ts.at(0), Some(10));
    assert_eq!(ts.at(3), Some(40));
    assert_eq!(ts.at(4), None); // out of range

    // time -> position window `[start, end)`
    assert_eq!(ts.window(20..40), (1, 3)); // positions 1,2 (both t=20)
    assert_eq!(ts.window(0..100), (0, 4)); // whole file
    assert_eq!(ts.window(100..200), (4, 4)); // past the last log → empty
    assert_eq!(ts.window(15..16), (1, 1)); // between values → empty

    // buckets [0,20), [20,40), [40,60) → positions {0}, {1,2}, {3}
    assert_eq!(
        ts.bucket_ranges(Grid::new(0, 20, 3)),
        vec![(0, 1), (1, 3), (3, 4)]
    );
    // no buckets → no ranges
    assert_eq!(
        ts.bucket_ranges(Grid::new(0, 20, 0)),
        Vec::<(u32, u32)>::new()
    );
}

#[test]
fn filter_from_selections_map() {
    let mut selections: std::collections::HashMap<String, Vec<String>> = Default::default();
    selections.insert("level".into(), vec!["info".into(), "error".into()]);
    selections.insert("service".into(), vec!["api".into()]);
    // A field with no values must be dropped (no constraint), not stored as
    // an empty selection that would collapse the filter to match-nothing.
    selections.insert("cleared".into(), vec![]);

    let filter = Filter::from(&selections);

    let expected = Filter::new()
        .select("level", "info")
        .select("level", "error")
        .select("service", "api");
    assert_eq!(filter, expected);
    assert!(!filter.has_field("cleared"));
}

// ── Regex (pattern) matchers ─────────────────────────────────────────

/// A `BitmapValue` over `universe` positions from a sorted position list.
fn bitmap_value(positions: &[u32], universe: u32) -> BitmapValue {
    let mut data = Vec::new();
    let desc = treight::Bitmap::from_sorted_iter(positions.iter().copied(), universe, &mut data);
    BitmapValue { desc, data }
}

/// Synthetic SFST exercising all three tiers, for pattern-resolution tests.
/// 6 logs, 1s apart, same time base as [`build_query_fixture`] (so
/// [`FULL_WINDOW`] applies).
///
/// - `level` (Low):  info @ {0,2,4}, error @ {1,3,5}.
/// - `host`  (Mid):  web1 @ {0,1}, web2 @ {2,3}, db1 @ {4,5}.
/// - `trace` (High): aaa @ {0,1}, bbb @ {2,3}, ccc @ {4,5} (via stream batch).
fn build_tiered_fixture() -> Vec<u8> {
    const N: u32 = 6;

    let primary_entries = vec![
        ("level=error", bitmap_value(&[1, 3, 5], N)),
        ("level=info", bitmap_value(&[0, 2, 4], N)),
    ];

    // Mid chunk: the `host` field's values + bitmaps (FST, lexicographic).
    let mid_host_entries = vec![
        ("host=db1", bitmap_value(&[4, 5], N)),
        ("host=web1", bitmap_value(&[0, 1], N)),
        ("host=web2", bitmap_value(&[2, 3], N)),
    ];

    // High chunk: `trace` values, all in the single stream batch (bit 0).
    let high_trace = HighField::for_write(
        &["trace=aaa", "trace=bbb", "trace=ccc"],
        vec![0b1, 0b1, 0b1],
    );

    // KvIds in tier order: low {error=0, info=1}, mid {db1=2, web1=3,
    // web2=4}, high {aaa=5, bbb=6, ccc=7}; `high_kv_id` = mid_end + local.
    let stream_entries: Vec<Vec<KvId>> = vec![
        vec![KvId(1), KvId(3), KvId(5)], // 0: info, web1, aaa
        vec![KvId(0), KvId(3), KvId(5)], // 1: error, web1, aaa
        vec![KvId(1), KvId(4), KvId(6)], // 2: info, web2, bbb
        vec![KvId(0), KvId(4), KvId(6)], // 3: error, web2, bbb
        vec![KvId(1), KvId(2), KvId(7)], // 4: info, db1, ccc
        vec![KvId(0), KvId(2), KvId(7)], // 5: error, db1, ccc
    ];

    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_000_005,
        record_count: N,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![6],
        },
        id_ranges: IdRanges {
            low_end: KvId(2),
            mid_end: KvId(5),
            high_end: KvId(8),
        },
        tree: SchemaTree::flat(
            &vec![
                FieldEntry {
                    name: "level".into(),
                    cardinality: 2,
                    tier: FieldTier::Low,
                },
                FieldEntry {
                    name: "host".into(),
                    cardinality: 3,
                    tier: FieldTier::Mid,
                },
                FieldEntry {
                    name: "trace".into(),
                    cardinality: 3,
                    tier: FieldTier::High,
                },
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };
    let timestamps: Vec<i64> = (0..N as i64)
        .map(|i| 1_700_000_000i64 * 1_000_000_000 + i * 1_000_000_000)
        .collect();

    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        mid_fields: 1,
        high_fields: 1,
        stream_batches: 1,
    };
    let mut writer = StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(primary_entries).unwrap();
    writer.add_mid_field(mid_host_entries).unwrap();
    writer.add_high_field(&high_trace).unwrap();
    writer
        .add_stream_batch(&StreamBatch::for_write(&stream_entries))
        .unwrap();
    writer.finish().unwrap().into_inner()
}

#[test]
fn pattern_low_card_is_full_value_anchored() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // `/info/` matches the whole value "info" (3 logs).
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "info")),
                FULL_WINDOW
            )
            .unwrap(),
        3
    );
    // `/inf/` does NOT — the match is anchored to the full value.
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "inf")),
                FULL_WINDOW
            )
            .unwrap(),
        0
    );
    // A substring search is the explicit `.*`.
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "inf.*")),
                FULL_WINDOW
            )
            .unwrap(),
        3
    );
}

#[test]
fn pattern_anchored_not_substring_at_tail() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // `/rror/` must NOT match "error".
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "rror")),
                FULL_WINDOW
            )
            .unwrap(),
        0
    );
    // `/err.*/` matches "error" → {1,3,5}.
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "err.*")),
                FULL_WINDOW
            )
            .unwrap(),
        3
    );
}

#[test]
fn pattern_mid_card_resolves_via_chunk() {
    let data = build_tiered_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // host ~ /web.*/ → web1{0,1} ∪ web2{2,3}.
    let positions = reader
        .matched_positions(
            &bf(&reader, Filter::new().select_pattern("host", "web.*")),
            FULL_WINDOW,
        )
        .unwrap();
    assert_eq!(positions, vec![0, 1, 2, 3]);
}

#[test]
fn pattern_high_card_resolves_via_stream_batches() {
    let data = build_tiered_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // trace ~ /a.*/ → aaa{0,1}.
    assert_eq!(
        reader
            .matched_positions(
                &bf(&reader, Filter::new().select_pattern("trace", "a.*")),
                FULL_WINDOW
            )
            .unwrap(),
        vec![0, 1]
    );
    // trace ~ /[ab].*/ → aaa{0,1} ∪ bbb{2,3} — multiple matched values,
    // exercising the unioned mask + the bitset target scan.
    assert_eq!(
        reader
            .matched_positions(
                &bf(&reader, Filter::new().select_pattern("trace", "[ab].*")),
                FULL_WINDOW
            )
            .unwrap(),
        vec![0, 1, 2, 3]
    );
}

#[test]
fn pattern_mixed_with_exact_ors() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // level = "info" OR level ~ /err.*/ → all 6 logs. Exercises the
    // exact-lookup + pattern-enumeration paths combined on one field.
    let filter = Filter::new()
        .select("level", "info")
        .select_pattern("level", "err.*");
    assert_eq!(
        reader
            .matched_count(&bf(&reader, filter), FULL_WINDOW)
            .unwrap(),
        6
    );
}

#[test]
fn pattern_no_match_is_empty() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    assert_eq!(
        reader
            .matched_count(
                &bf(&reader, Filter::new().select_pattern("level", "xyz")),
                FULL_WINDOW
            )
            .unwrap(),
        0
    );
}

#[test]
fn invalid_pattern_is_hard_error() {
    let data = build_query_fixture();
    let reader = IndexReader::open(&data).unwrap();
    let filter = Filter::new().select_pattern("level", "(unclosed");
    assert!(matches!(
        reader.compile_filter(&filter, None),
        Err(Error::InvalidPattern(_))
    ));
}

#[test]
fn validate_catches_bad_pattern_without_a_file() {
    // Validation is file-independent: exacts always pass, a good pattern
    // passes, a bad one is InvalidPattern — the boundary check a consumer
    // runs before touching any file.
    assert!(
        Filter::new()
            .select("level", "error")
            .select_pattern("trace", "abc.*")
            .validate()
            .is_ok()
    );
    assert!(matches!(
        Filter::new()
            .select_pattern("trace", "(unclosed")
            .validate(),
        Err(Error::InvalidPattern(_))
    ));
}

#[test]
fn query_full_text_matches_key_value_across_tiers() {
    let data = build_tiered_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Unanchored over whole `key=value`: "web" lives only in the mid-card
    // host values (host=web1 @ {0,1}, host=web2 @ {2,3}).
    assert_eq!(
        reader
            .matched_positions(&bfq(&reader, Filter::new(), "web"), FULL_WINDOW)
            .unwrap(),
        vec![0, 1, 2, 3]
    );
    // High-card key match (trace=aaa @ {0,1}) — resolved via stream batches.
    assert_eq!(
        reader
            .matched_positions(&bfq(&reader, Filter::new(), "trace=aaa"), FULL_WINDOW)
            .unwrap(),
        vec![0, 1]
    );
    // The key part scopes to a field: "level=info" matches the low-card key.
    assert_eq!(
        reader
            .matched_positions(&bfq(&reader, Filter::new(), "level=info"), FULL_WINDOW)
            .unwrap(),
        vec![0, 2, 4]
    );
}

#[test]
fn query_ands_with_the_field_filter() {
    let data = build_tiered_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // level=info {0,2,4} AND query "web" {0,1,2,3} = {0,2}.
    let bf = bfq(&reader, Filter::new().select("level", "info"), "web");
    assert_eq!(reader.matched_count(&bf, FULL_WINDOW).unwrap(), 2);
    assert_eq!(
        reader.matched_positions(&bf, FULL_WINDOW).unwrap(),
        vec![0, 2]
    );
}

#[test]
fn query_narrows_facet_counts() {
    let data = build_tiered_fixture();
    let reader = IndexReader::open(&data).unwrap();
    // Query "web" restricts the scope to host=web* logs {0,1,2,3}; the
    // `level` facet over that scope is error:2 (pos 1,3), info:2 (pos 0,2).
    // Confirms the query folds into the aggregate, not just the page.
    let bf = bfq(&reader, Filter::new(), "web");
    let facets = reader.facets(&["level"], &bf, FULL_WINDOW).unwrap();
    assert_eq!(facets.len(), 1);
    assert_eq!(facets[0].field, "level");
    assert_eq!(
        facets[0].values,
        vec![("error".to_string(), 2), ("info".to_string(), 2)]
    );
}

/// SFST with **two fields per mid/high tier**, so the tier-relative chunk index
/// advances past 0 — exercises `field_table_tiered`, `high_kv_id`, and the
/// high-card scan at index 1 (the single-field-per-tier fixtures never do).
/// 4 logs, same time base as [`build_query_fixture`].
///
/// Within a tier the derived field table is name-ordered, so the chunk /
/// KvId layout follows `host < region` (mid) and `req < trace` (high):
///
/// - `level`  (Low):    info @ {0,2}, error @ {1,3}
/// - `host`   (Mid 0):  h1 @ {0,1},  h2 @ {2,3}
/// - `region` (Mid 1):  r1 @ {0,2},  r2 @ {1,3}
/// - `req`    (High 0): q1 @ {0,1},  q2 @ {2,3}
/// - `trace`  (High 1): t1 @ {0,2},  t2 @ {1,3}
fn build_two_per_tier_fixture() -> Vec<u8> {
    const N: u32 = 4;

    let primary_entries = vec![
        ("level=error", bitmap_value(&[1, 3], N)),
        ("level=info", bitmap_value(&[0, 2], N)),
    ];
    let mid_host = vec![
        ("host=h1", bitmap_value(&[0, 1], N)),
        ("host=h2", bitmap_value(&[2, 3], N)),
    ];
    let mid_region = vec![
        ("region=r1", bitmap_value(&[0, 2], N)),
        ("region=r2", bitmap_value(&[1, 3], N)),
    ];
    // High values all sit in the single stream batch (mask bit 0).
    let high_req = HighField::for_write(&["req=q1", "req=q2"], vec![0b1, 0b1]);
    let high_trace = HighField::for_write(&["trace=t1", "trace=t2"], vec![0b1, 0b1]);

    // Tier- and name-aligned KvIds: low {error=0, info=1}; mid {h1=2, h2=3,
    // r1=4, r2=5}; high {q1=6, q2=7, t1=8, t2=9}. `trace` (high field 1)
    // therefore starts at KvId 8 = mid_end(6) + req's cardinality(2) — the base
    // `high_kv_id(1, 0)` must derive.
    let stream_entries: Vec<Vec<KvId>> = vec![
        vec![KvId(1), KvId(2), KvId(4), KvId(6), KvId(8)], // 0: info, h1, r1, q1, t1
        vec![KvId(0), KvId(2), KvId(5), KvId(6), KvId(9)], // 1: error, h1, r2, q1, t2
        vec![KvId(1), KvId(3), KvId(4), KvId(7), KvId(8)], // 2: info, h2, r1, q2, t1
        vec![KvId(0), KvId(3), KvId(5), KvId(7), KvId(9)], // 3: error, h2, r2, q2, t2
    ];

    let field = |name: &str, cardinality: u32, tier| FieldEntry {
        name: name.into(),
        cardinality,
        tier,
    };
    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_000_003,
        record_count: N,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![N],
        },
        id_ranges: IdRanges {
            low_end: KvId(2),
            mid_end: KvId(6),
            high_end: KvId(10),
        },
        tree: SchemaTree::flat(
            &vec![
                field("level", 2, FieldTier::Low),
                field("host", 2, FieldTier::Mid),
                field("region", 2, FieldTier::Mid),
                field("req", 2, FieldTier::High),
                field("trace", 2, FieldTier::High),
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };
    let timestamps: Vec<i64> = (0..N as i64)
        .map(|i| FILE_MIN_NS + i * 1_000_000_000)
        .collect();

    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        mid_fields: 2,
        high_fields: 2,
        stream_batches: 1,
    };
    let mut w = StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    w.summary(&summary).unwrap();
    w.metadata(&metadata).unwrap();
    w.timestamps(&timestamps).unwrap();
    w.primary(primary_entries).unwrap();
    // Chunks in the derived (name-sorted within tier) order.
    w.add_mid_field(mid_host).unwrap();
    w.add_mid_field(mid_region).unwrap();
    w.add_high_field(&high_req).unwrap();
    w.add_high_field(&high_trace).unwrap();
    w.add_stream_batch(&StreamBatch::for_write(&stream_entries))
        .unwrap();
    w.finish().unwrap().into_inner()
}

#[test]
fn second_chunk_per_tier_resolves() {
    let data = build_two_per_tier_fixture();
    let reader = IndexReader::open(&data).unwrap();

    // Second MID field (region): locate_field must return Mid(1) → loads mid_field(1).
    assert_eq!(
        reader
            .matched_positions(
                &bf(&reader, Filter::new().select("region", "r2")),
                FULL_WINDOW
            )
            .unwrap(),
        vec![1, 3]
    );

    // Second HIGH field (trace): locate_field High(1); field_values_or's base is
    // high_kv_id(1, 0) = 8 (mid_end + req's cardinality), then scan_high_positions.
    assert_eq!(
        reader
            .matched_positions(
                &bf(&reader, Filter::new().select("trace", "t2")),
                FULL_WINDOW
            )
            .unwrap(),
        vec![1, 3]
    );

    // Cross-tier AND spanning both second-of-tier chunks.
    assert_eq!(
        reader
            .matched_positions(
                &bf(
                    &reader,
                    Filter::new().select("region", "r1").select("trace", "t1")
                ),
                FULL_WINDOW
            )
            .unwrap(),
        vec![0, 2]
    );

    // materialize_rows → build_string_table walks all four secondary chunks; row 3
    // carries the second value of every field (tier index 1 for region and trace).
    let rows = reader.materialize_rows(&[3]).unwrap();
    let fields: std::collections::HashMap<String, String> =
        rows[0].fields.iter().cloned().collect();
    assert_eq!(fields.get("host").map(String::as_str), Some("h2"));
    assert_eq!(fields.get("region").map(String::as_str), Some("r2"));
    assert_eq!(fields.get("req").map(String::as_str), Some("q2"));
    assert_eq!(fields.get("trace").map(String::as_str), Some("t2"));
}
