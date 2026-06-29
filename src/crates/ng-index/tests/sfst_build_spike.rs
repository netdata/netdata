//! Spike: build a real SFST index file from our OWN `(timestamp, [key=value])`
//! rows using the existing `sfst-indexer`, then query it back. This is the
//! feasibility brick for the augment-SFST plan: it proves the SFST builder is
//! reusable with rows WE produce (e.g. stringified ng-flatten output), via
//! `RowIndex`'s inherent `intern` + `row` methods (the spike passes `None` for the
//! optional pre-computed hash).

use bumpalo::Bump;
use sfst::IndexReader;
use sfst::query::Filter;
use sfst_indexer::build_and_write;
use sfst_indexer::row_index::RowIndex;

/// Build an SFST file from `(timestamp_ns, &[key=value])` rows; return its bytes.
fn build_sfst(rows: &[(i64, &[&str])]) -> Vec<u8> {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 100);
    for &(ts, kvs) in rows {
        // None hash = always-safe intern path (dedup by full string).
        let tokens: Vec<_> = kvs.iter().map(|&kv| ri.intern(None, kv)).collect();
        ri.row(ts, &tokens);
    }
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("spike.sfst");
    build_and_write(&ri, &path, None).expect("build_and_write");
    std::fs::read(&path).expect("read sfst")
}

/// Count records matching `filter` across all time.
fn count(reader: &IndexReader, filter: Filter) -> u64 {
    let bf = reader
        .compile_filter(&filter, None)
        .expect("compile_filter");
    reader
        .matched_count(&bf, i64::MIN..i64::MAX)
        .expect("matched_count")
}

#[test]
fn build_sfst_from_our_rows_and_query_back() {
    // Timestamps deliberately out of arrival order, to exercise the index-build
    // time-sort.
    let rows: &[(i64, &[&str])] = &[
        (
            300,
            &["service=checkout", "http.method=GET", "http.status=200"],
        ),
        (
            100,
            &["service=checkout", "http.method=POST", "http.status=500"],
        ),
        (
            200,
            &["service=billing", "http.method=GET", "http.status=200"],
        ),
    ];

    let bytes = build_sfst(rows);
    let reader = IndexReader::open(&bytes).expect("open sfst");

    // Empty filter = every record.
    assert_eq!(count(&reader, Filter::new()), 3);

    // Exact matches.
    assert_eq!(
        count(&reader, Filter::new().select("service", "checkout")),
        2
    );
    assert_eq!(
        count(&reader, Filter::new().select("http.method", "GET")),
        2
    );
    assert_eq!(
        count(&reader, Filter::new().select("http.status", "500")),
        1
    );

    // Full-value regex (anchored ^(?:5..)$ -> matches "500").
    assert_eq!(
        count(&reader, Filter::new().select_pattern("http.status", "5..")),
        1
    );

    // AND across fields: checkout AND GET -> only the ts=300 row.
    let both = Filter::new()
        .select("service", "checkout")
        .select("http.method", "GET");
    assert_eq!(count(&reader, both), 1);
}
