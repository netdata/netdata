//! Integration tests for `Filter` evaluation against a real indexed
//! journal (filter.rs): every test writes a fresh journal file, indexes
//! it with `FileIndexer::index`, and evaluates filters over the result.
//!
//! Fixture: one journal per test at `<tmp>/<machine-id>/system.journal`,
//! the registry path grammar `File::from_path` parses (absolute path,
//! `.journal` suffix, `system` source dir, machine-id dir above it —
//! `journal-registry/src/repository/file.rs`). Every entry is
//! stamped with `_SOURCE_REALTIME_TIMESTAMP=<microseconds>` and
//! `add_entry` receives the same value as realtime and monotonic
//! (`journal-core/src/file/writer.rs` `add_entry`), so indexing with
//! `Some(&source_field)` orders entries by that field while `None`
//! falls back to the journal realtime (`src/file_indexer.rs`
//! `collect_source_field_info`/`build_histogram`).
//! Indexing always uses 3600 s histogram buckets and builds bitmaps
//! only for the requested fields.
//!
//! Pinned behaviors and the contracts they guard:
//!
//! - Bitmap bits are entry indices into `FileIndex::entry_offsets` —
//!   positions in the index's time-ordered entry list, not file order
//!   (`src/file_indexer.rs` ordering pass; consumed in
//!   `src/file_index.rs` `find_log_entries`).
//! - Leaves match exactly: a field=value pair matches only that pair, a
//!   field-name leaf matches every value the field takes (the union of
//!   its pair bitmaps, `src/filter.rs` resolve-time `Field` leaf). An
//!   unknown field or pair matches nothing and never errors
//!   (`src/filter.rs` resolve-time `Pair` leaf).
//! - AND intersects, OR unions, and nested compounds resolve
//!   recursively before folding into one bitmap
//!   (`src/filter.rs` `FilterExpr::resolve`/`FilterExpr::evaluate`).
//! - `Filter::none()` is the no-filter sentinel: `is_none()` reports
//!   the marker, evaluating it yields an empty bitmap
//!   (`src/filter.rs` `Filter::none` and `FilterExpr::evaluate`) — a
//!   filter that matches nothing is not necessarily the sentinel.
//! - Empty AND and empty OR both collapse to `none()` at construction
//!   and match nothing (`src/filter.rs` `FilterExpr::and`/`or` collapse
//!   arms) — an OR with no branches is not match-all.
//! - The three non-filter tests pin the index surface the bitmaps rest
//!   on: bucketed seconds time range, `file_fields` vs indexed fields,
//!   entry count, and entry ordering (source-timestamp order when
//!   given a source field, realtime fallback otherwise).
//!
//! Not pinned here: case sensitivity (no mixed-case fixture; names and
//! values match verbatim — nothing in the index normalizes case,
//! `src/field_types.rs` `FieldName`/`FieldValuePair` derive the inner
//! string's `Eq`); the resolve-time None-child rules
//! (an AND with a `None` child is `None` outright, an OR skips `None`
//! children — `src/filter.rs` `FilterExpr::resolve`; only construction-time
//! flattening is exercised); time bounds, regex and pagination,
//! applied after the bitmap stage (`src/file_index.rs`
//! `find_log_entries`); a
//! field present in the journal but left out of the indexed list; and
//! consumers' `is_none()` skip path (journal-engine, otel-legacy-logs).

use journal_common::Seconds;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_index::{FieldName, FieldValuePair, FileIndexer, Filter, Microseconds};
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

// Readable timestamp helpers: JAN_1_2024_MIDNIGHT is 2024-01-01
// 00:00:00 UTC in microseconds; hours()/add_time() build offsets from it.
const JAN_1_2024_MIDNIGHT: Microseconds = Microseconds(1704067200_000_000);

fn hours(n: u64) -> Microseconds {
    Microseconds(n * 3600_000_000)
}

fn add_time(base: Microseconds, offset: Microseconds) -> Microseconds {
    Microseconds(base.0 + offset.0)
}

/// One journal entry: a timestamp plus (field, value) pairs.
struct TestEntry {
    timestamp: Microseconds,
    fields: Vec<(String, String)>,
}

impl TestEntry {
    fn new(timestamp: Microseconds) -> Self {
        Self {
            timestamp,
            fields: Vec::new(),
        }
    }

    fn with_field(mut self, name: impl Into<String>, value: impl Into<String>) -> Self {
        self.fields.push((name.into(), value.into()));
        self
    }
}

/// Journal path in the registry layout `<dir>/<machine-id>/system.journal`:
/// `File::from_path` reads status from the suffix, source from the parent
/// dir and machine id from the dir above
/// (`journal-registry/src/repository/file.rs` `File::from_path`).
fn create_test_journal_path(temp_dir: &TempDir) -> PathBuf {
    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let machine_dir = temp_dir.path().join(machine_id.to_string());
    fs::create_dir_all(&machine_dir).expect("create machine dir");

    machine_dir.join("system.journal")
}

/// Write `entries` into a fresh journal file; returns the `File` plus
/// the `TempDir` backing it — indexing re-opens the journal by path, so
/// the dir must outlive it.
///
/// Each entry carries `_SOURCE_REALTIME_TIMESTAMP=<microseconds>` and
/// `add_entry` receives the same value as realtime and monotonic
/// (`journal-core/src/file/writer.rs` `add_entry`): the field is the
/// source-ordering input, the header timestamps the realtime fallback.
fn create_test_journal(
    entries: Vec<TestEntry>,
) -> Result<(TempDir, File), Box<dyn std::error::Error>> {
    let temp_dir = TempDir::new()?;
    let journal_path = create_test_journal_path(&temp_dir);

    let file =
        File::from_path(&journal_path).ok_or("Failed to create repository File from path")?;

    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let boot_id = Uuid::from_u128(0x11111111_1111_1111_1111_111111111111);
    let seqnum_id = Uuid::from_u128(0x22222222_2222_2222_2222_222222222222);

    let options = JournalFileOptions::new(machine_id, boot_id, seqnum_id);

    let mut journal_file = JournalFile::create(&file, options)?;
    let mut writer = JournalWriter::new(&mut journal_file, 1, boot_id)?;

    for entry in entries {
        let mut entry_data = Vec::new();

        entry_data.push(format!("_SOURCE_REALTIME_TIMESTAMP={}", entry.timestamp.0).into_bytes());

        for (field, value) in entry.fields {
            entry_data.push(format!("{}={}", field, value).into_bytes());
        }

        let entry_refs: Vec<&[u8]> = entry_data.iter().map(|v| v.as_slice()).collect();

        writer.add_entry(
            &mut journal_file,
            &entry_refs,
            entry.timestamp.0,
            entry.timestamp.0,
        )?;
    }

    Ok((temp_dir, file))
}

#[test]
fn test_filter_field_value_pair_single_match() {
    // Five entries with PRIORITY=3 on 0/2/4 interleaved with 6/7 — matches
    // scattered through the index rather than contiguous.
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3))).with_field("PRIORITY", "7"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(4))).with_field("PRIORITY", "3"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    let pair = FieldValuePair::parse("PRIORITY=3").unwrap();
    let filter = Filter::match_field_value_pair(pair);
    let bitmap = filter.evaluate(&file_index);

    // Should match entries 0, 2, and 4
    assert_eq!(bitmap.len(), 3);
    assert!(bitmap.contains(0));
    assert!(bitmap.contains(2));
    assert!(bitmap.contains(4));
    assert!(!bitmap.contains(1));
    assert!(!bitmap.contains(3));
}

#[test]
fn test_filter_field_name_matches_all_values() {
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "7"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field.clone()], Seconds(3600))
        .unwrap();

    // Field-presence leaf: matches entries carrying PRIORITY under any
    // value — the leaf unions the field's pair bitmaps (`src/filter.rs`
    // resolve-time `Field` leaf).
    let filter = Filter::match_field_name(priority_field);
    let bitmap = filter.evaluate(&file_index);

    // Should match all entries (0, 1, 2)
    assert_eq!(bitmap.len(), 3);
    assert!(bitmap.contains(0));
    assert!(bitmap.contains(1));
    assert!(bitmap.contains(2));
}

#[test]
fn test_filter_and_combination() {
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server1"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1)))
            .with_field("PRIORITY", "6")
            .with_field("_HOSTNAME", "server1"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server2"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server1"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let hostname_field = FieldName::new("_HOSTNAME").unwrap();
    let file_index = indexer
        .index(
            &file,
            None,
            &[priority_field, hostname_field],
            Seconds(3600),
        )
        .unwrap();

    // Entries 1 and 2 each fail exactly one conjunct, isolating the intersection.
    let filter = Filter::and(vec![
        Filter::match_field_value_pair(FieldValuePair::parse("PRIORITY=3").unwrap()),
        Filter::match_field_value_pair(FieldValuePair::parse("_HOSTNAME=server1").unwrap()),
    ]);

    let bitmap = filter.evaluate(&file_index);

    // Should match entries 0 and 3 (both have PRIORITY=3 AND _HOSTNAME=server1)
    assert_eq!(bitmap.len(), 2);
    assert!(bitmap.contains(0));
    assert!(bitmap.contains(3));
    assert!(!bitmap.contains(1)); // Has server1 but PRIORITY=6
    assert!(!bitmap.contains(2)); // Has PRIORITY=3 but server2
}

#[test]
fn test_filter_or_combination() {
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3))).with_field("PRIORITY", "7"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    let filter = Filter::or(vec![
        Filter::match_field_value_pair(FieldValuePair::parse("PRIORITY=3").unwrap()),
        Filter::match_field_value_pair(FieldValuePair::parse("PRIORITY=6").unwrap()),
    ]);

    let bitmap = filter.evaluate(&file_index);

    // Should match entries 0, 1, and 2
    assert_eq!(bitmap.len(), 3);
    assert!(bitmap.contains(0));
    assert!(bitmap.contains(1));
    assert!(bitmap.contains(2));
    assert!(!bitmap.contains(3)); // PRIORITY=7
}

#[test]
fn test_filter_none() {
    let entries =
        vec![TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3")];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // none() is the no-filter sentinel: consumers check is_none(), not the
    // result. Evaluating it directly yields an empty bitmap
    // (`src/filter.rs` `Filter::none` and `FilterExpr::evaluate`); the
    // unknown-pair test below pins the contrast —
    // empty without being the sentinel.
    let filter = Filter::none();
    let bitmap = filter.evaluate(&file_index);

    assert_eq!(bitmap.len(), 0);
    assert!(filter.is_none());
}

#[test]
fn test_filter_nonexistent_field() {
    let entries =
        vec![TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3")];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // A pair absent from both the journal and the index: evaluation is
    // infallible — the leaf resolves to "match nothing", not an error
    // (`src/filter.rs` resolve-time `Pair` leaf). is_none() stays false:
    // it reports the filter
    // shape, not matchability.
    let filter =
        Filter::match_field_value_pair(FieldValuePair::parse("NONEXISTENT_FIELD=value").unwrap());
    let bitmap = filter.evaluate(&file_index);

    assert_eq!(bitmap.len(), 0);
}

#[test]
fn test_filter_complex_nested() {
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server1")
            .with_field("SYSLOG_IDENTIFIER", "systemd"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1)))
            .with_field("PRIORITY", "6")
            .with_field("_HOSTNAME", "server1")
            .with_field("SYSLOG_IDENTIFIER", "kernel"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server2")
            .with_field("SYSLOG_IDENTIFIER", "systemd"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3)))
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server1")
            .with_field("SYSLOG_IDENTIFIER", "kernel"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let fields = vec![
        FieldName::new("PRIORITY").unwrap(),
        FieldName::new("_HOSTNAME").unwrap(),
        FieldName::new("SYSLOG_IDENTIFIER").unwrap(),
    ];
    let file_index = indexer.index(&file, None, &fields, Seconds(3600)).unwrap();

    // Nested tree: an AND compound as an OR operand — resolve recurses
    // through compounds, not just flat leaves (`src/filter.rs`
    // `FilterExpr::resolve`).
    let filter = Filter::or(vec![
        Filter::and(vec![
            Filter::match_field_value_pair(FieldValuePair::parse("PRIORITY=3").unwrap()),
            Filter::match_field_value_pair(FieldValuePair::parse("_HOSTNAME=server1").unwrap()),
        ]),
        Filter::match_field_value_pair(FieldValuePair::parse("SYSLOG_IDENTIFIER=kernel").unwrap()),
    ]);

    let bitmap = filter.evaluate(&file_index);

    // Entries 0 and 3 match the AND branch, 1 the OR leaf; 2 matches
    // neither — PRIORITY=3 but _HOSTNAME=server2, and SYSLOG_IDENTIFIER
    // is systemd, not kernel.
    assert_eq!(bitmap.len(), 3);
    assert!(bitmap.contains(0));
    assert!(bitmap.contains(1));
    assert!(!bitmap.contains(2));
    assert!(bitmap.contains(3));
}

#[test]
fn test_filter_empty_and() {
    let entries =
        vec![TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3")];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // and() collapses an empty operand list to none() at construction
    // (`src/filter.rs` `FilterExpr::and` collapse arm) — before any evaluation.
    let filter = Filter::and(vec![]);
    let bitmap = filter.evaluate(&file_index);

    assert_eq!(bitmap.len(), 0);
    assert!(filter.is_none());
}

#[test]
fn test_filter_empty_or() {
    let entries =
        vec![TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3")];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // Same collapse for or() (`src/filter.rs` `FilterExpr::or` collapse
    // arm): an OR with no branches matches nothing, not everything.
    let filter = Filter::or(vec![]);
    let bitmap = filter.evaluate(&file_index);

    assert_eq!(bitmap.len(), 0);
    assert!(filter.is_none());
}

// Not a filter test: pins the FileIndex surface filter results are read
// against — the file handle, bucketed time range, field sets, entry
// count, and the pair->bitmap map.
#[test]
fn test_file_index_metadata() {
    let entries = vec![
        TestEntry::new(JAN_1_2024_MIDNIGHT)
            .with_field("PRIORITY", "3")
            .with_field("_HOSTNAME", "server1"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1)))
            .with_field("PRIORITY", "6")
            .with_field("_HOSTNAME", "server1")
            .with_field("MESSAGE", "test message"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2)))
            .with_field("PRIORITY", "3")
            .with_field("MESSAGE", "another message"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let hostname_field = FieldName::new("_HOSTNAME").unwrap();
    let file_index = indexer
        .index(
            &file,
            None,
            &[priority_field.clone(), hostname_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    // The index keeps the exact File handle it was built from.
    assert_eq!(file_index.file(), &file);

    // Time range: histogram seconds over 3600 s buckets (`src/histogram.rs`
    // `Histogram::start_time`/`end_time`).
    // Start = first bucket start; midnight is already bucket-aligned.
    assert_eq!(file_index.start_time().0, 1704067200); // Exact start
    // End = last occupied bucket start + bucket duration.
    assert_eq!(file_index.end_time().0, 1704074400 + 3600); // 2 hours + bucket size

    // file_fields = every field in the journal, indexed or not —
    // _SOURCE_REALTIME_TIMESTAMP appears because the helper stamps it.
    let file_fields = file_index.fields();
    assert!(file_fields.contains(&FieldName::new("PRIORITY").unwrap()));
    assert!(file_fields.contains(&FieldName::new("_HOSTNAME").unwrap()));
    assert!(file_fields.contains(&FieldName::new("MESSAGE").unwrap()));
    assert!(file_fields.contains(&FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap()));

    // indexed_fields = exactly the requested list; MESSAGE exists in the
    // journal but was never requested.
    assert!(file_index.is_indexed(&priority_field));
    assert!(file_index.is_indexed(&hostname_field));
    assert!(!file_index.is_indexed(&FieldName::new("MESSAGE").unwrap()));

    assert_eq!(file_index.total_entries(), 3);

    // One bitmap per indexed field=value pair present in the journal.
    let bitmaps = file_index.bitmaps();
    assert!(bitmaps.contains_key(&FieldValuePair::parse("PRIORITY=3").unwrap()));
    assert!(bitmaps.contains_key(&FieldValuePair::parse("PRIORITY=6").unwrap()));
    assert!(bitmaps.contains_key(&FieldValuePair::parse("_HOSTNAME=server1").unwrap()));

    // MESSAGE was not requested: no bitmap for it — file_fields and the
    // bitmap keys are independent sets.
    assert!(!bitmaps.contains_key(&FieldValuePair::parse("MESSAGE=test message").unwrap()));
}

#[test]
fn test_source_timestamp_ordering() {
    // Entries written out of timestamp order (+3 h, +1 h, +2 h), so
    // indexing with the source field must reorder them by source time.
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3))).with_field("PRIORITY", "3"), // +3 hours (created first, but should be ordered last)
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"), // +1 hour (created second, but should be ordered first)
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "7"), // +2 hours (created third, but should be ordered middle)
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let source_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();

    // Indexing with the source field: positions come from the
    // source-timestamp-sorted list (`src/file_indexer.rs`
    // `collect_source_field_info`), not write order.
    let file_index = indexer
        .index(&file, Some(&source_field), &[priority_field], Seconds(3600))
        .unwrap();

    // Reordered mapping: +1 h -> 0 (PRIORITY=6), +2 h -> 1 (PRIORITY=7),
    // +3 h -> 2 (PRIORITY=3, though written first).

    // Verify time range reflects source timestamp order (in seconds)
    let expected_start = (add_time(JAN_1_2024_MIDNIGHT, hours(1)).0 / 1_000_000) as u32; // +1 hour in seconds
    let expected_end = ((add_time(JAN_1_2024_MIDNIGHT, hours(3)).0 / 1_000_000) + 3600) as u32; // +3 hours + bucket duration
    assert_eq!(file_index.start_time().0, expected_start);
    assert_eq!(file_index.end_time().0, expected_end);

    // Verify bitmaps reflect the new ordering
    let bitmaps = file_index.bitmaps();

    // PRIORITY=6 (the +1 h entry) is at index 0.
    let priority_6_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=6").unwrap())
        .unwrap();
    assert_eq!(priority_6_bitmap.len(), 1);
    assert!(priority_6_bitmap.contains(0));

    // PRIORITY=7 (the +2 h entry) is at index 1.
    let priority_7_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=7").unwrap())
        .unwrap();
    assert_eq!(priority_7_bitmap.len(), 1);
    assert!(priority_7_bitmap.contains(1));

    // PRIORITY=3 (the +3 h entry) is at index 2.
    let priority_3_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=3").unwrap())
        .unwrap();
    assert_eq!(priority_3_bitmap.len(), 1);
    assert!(priority_3_bitmap.contains(2));
}

#[test]
fn test_indexing_without_source_timestamp() {
    // No source-timestamp field passed to index(), though the fixture still
    // wrote _SOURCE_REALTIME_TIMESTAMP — indexing is just not told to use
    // it. Every entry's position falls back to the journal realtime
    // timestamp (`src/file_indexer.rs` `build_histogram` realtime
    // fallback).
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "7"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();

    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // Entries were written in increasing time order, so realtime order
    // equals write order:
    let bitmaps = file_index.bitmaps();

    // PRIORITY=3 should be at index 0
    let priority_3_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=3").unwrap())
        .unwrap();
    assert_eq!(priority_3_bitmap.len(), 1);
    assert!(priority_3_bitmap.contains(0));

    // PRIORITY=6 should be at index 1
    let priority_6_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=6").unwrap())
        .unwrap();
    assert_eq!(priority_6_bitmap.len(), 1);
    assert!(priority_6_bitmap.contains(1));

    // PRIORITY=7 should be at index 2
    let priority_7_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=7").unwrap())
        .unwrap();
    assert_eq!(priority_7_bitmap.len(), 1);
    assert!(priority_7_bitmap.contains(2));
}
