//! Integration tests for query pagination over an indexed journal
//! (file_index.rs): each test writes a fresh journal file, indexes it
//! with `FileIndexer::index`, and pages through it with
//! `find_log_entries`.
//!
//! Fixture: one journal per test at `<tmp>/<machine-id>/system.journal`
//! — the registry path grammar `File::from_path` parses
//! (`journal-registry/src/repository/file.rs`). Every entry is
//! stamped with `_SOURCE_REALTIME_TIMESTAMP=<microseconds>` and
//! `add_entry` receives the same value as realtime and monotonic
//! (`journal-core/src/file/writer.rs` `add_entry`). The same-timestamp and
//! mixed-timestamp tests index with `Some(&source_timestamp_field)`,
//! the rest with `None` (realtime fallback); both must paginate
//! identically because the two values coincide. Entries sharing a
//! timestamp keep their (timestamp, offset) sort order
//! (`src/file_indexer.rs` `collect_source_field_info` sort), so a run's
//! internal order is deterministic.
//!
//! Pinned contracts:
//!
//! - `LogEntryId.position` indexes the time-ordered entry list the
//!   query walks — the full list unless a filter narrows it — and is
//!   the pagination cursor: a forward page starts at
//!   `resume_position + 1`, a backward page at `resume_position - 1`
//!   (`src/file_index.rs` `find_log_entries` resume handling).
//!   Positions are absolute, not
//!   window-relative, so time-boundary queries report the entries'
//!   real indices.
//! - `Anchor::Timestamp` is inclusive in both directions: the entry at
//!   exactly the anchor starts the walk (`src/file_index.rs`
//!   `find_log_entries` anchor search).
//!   `Anchor::Head`/`Tail` resolve to the histogram's
//!   first-bucket start and exclusive end (`src/file_index.rs`
//!   `find_log_entries` Head/Tail arms, `src/histogram.rs`
//!   `Histogram::start_time`/`end_time`).
//! - `after` is inclusive, `before` exclusive (`src/file_index.rs`
//!   `find_log_entries` boundary checks); `limit` is an upper bound,
//!   never padded; limit 0
//!   returns empty without error (`src/file_index.rs` `find_log_entries`
//!   limit-0 check).
//! - Out-of-bounds resume positions return empty, never panic:
//!   forward clamps at the end-of-list check (`src/file_index.rs`
//!   `find_log_entries` Forward arm),
//!   backward rejects 0 and >= len up front (`src/file_index.rs`
//!   `find_log_entries` Backward arm).
//! - Same-timestamp runs are the hard case: the anchor search can only
//!   locate the run's edge, so stepping within a run works only
//!   through resume_position.
//!
//! Not pinned here: pagination with an active filter (positions then
//! index the filtered list and resume requires the filter to stay
//! unchanged between pages — the `resume_position` contract in
//! `src/file_index.rs`), regex filtering,
//! unlimited queries (every test sets a limit), and multi-file
//! pagination (one file per index throughout).

use journal_common::Seconds;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_index::{
    Anchor, Direction, FieldName, FileIndexer, LogQueryParamsBuilder, Microseconds,
};
use std::collections::HashSet;
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

// JAN_1_2024_MIDNIGHT is 2024-01-01 00:00:00 UTC in microseconds; tests
// derive offsets from it rather than writing raw epoch values.
const JAN_1_2024_MIDNIGHT: Microseconds = Microseconds(1704067200_000_000);

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
/// (`journal-core/src/file/writer.rs` `add_entry`): the field feeds the
/// source-timestamp ordering path, the header timestamps the realtime
/// fallback.
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
fn test_pagination_forward_with_same_timestamps() {
    // All 300 entries share one timestamp: the anchor search can only
    // locate where the run starts (`src/file_index.rs` `find_log_entries`
    // anchor search), so stepping
    // within the run works only through resume_position.
    const TOTAL_ENTRIES: usize = 300;
    const PAGE_SIZE: usize = 200;
    let same_timestamp = JAN_1_2024_MIDNIGHT;

    let entries: Vec<TestEntry> = (0..TOTAL_ENTRIES)
        .map(|i| {
            TestEntry::new(same_timestamp)
                .with_field("MESSAGE", format!("Entry {}", i))
                .with_field("ENTRY_ID", i.to_string())
        })
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_index = indexer
        .index(
            &file,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let mut all_offsets = Vec::new();
    let mut all_positions = HashSet::new();
    let mut resume_position = None;

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(PAGE_SIZE)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("First page: {} entries", results.len());
    assert_eq!(
        results.len(),
        PAGE_SIZE,
        "First page should return PAGE_SIZE entries"
    );

    for entry in &results {
        assert_eq!(entry.timestamp, same_timestamp);
        all_offsets.push(entry.offset);
        assert!(
            all_positions.insert(entry.position),
            "Position {} appeared twice",
            entry.position
        );
    }

    if let Some(last_entry) = results.last() {
        resume_position = Some(last_entry.position);
    }

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(PAGE_SIZE)
        .with_resume_position(resume_position.unwrap())
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("Second page: {} entries", results.len());
    assert_eq!(
        results.len(),
        TOTAL_ENTRIES - PAGE_SIZE,
        "Second page should return remaining entries"
    );

    for entry in &results {
        assert_eq!(entry.timestamp, same_timestamp);
        all_offsets.push(entry.offset);
        assert!(
            all_positions.insert(entry.position),
            "Position {} appeared twice",
            entry.position
        );
    }

    if let Some(last_entry) = results.last() {
        resume_position = Some(last_entry.position);
    }

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(PAGE_SIZE)
        .with_resume_position(resume_position.unwrap())
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("Third page: {} entries", results.len());
    assert_eq!(results.len(), 0, "Third page should be empty");

    assert_eq!(
        all_offsets.len(),
        TOTAL_ENTRIES,
        "Should have retrieved all entries"
    );

    let unique_offsets: HashSet<_> = all_offsets.iter().collect();
    assert_eq!(
        unique_offsets.len(),
        TOTAL_ENTRIES,
        "All offsets should be unique"
    );

    // Together the pages covered positions 0..299: no gaps, no repeats.
    assert_eq!(
        all_positions.len(),
        TOTAL_ENTRIES,
        "Should have unique positions"
    );
    for i in 0..TOTAL_ENTRIES {
        assert!(all_positions.contains(&i), "Position {} missing", i);
    }
}

#[test]
fn test_pagination_backward_with_same_timestamps() {
    // Anchor::Tail resolves to the histogram's exclusive end
    // (`src/file_index.rs` `find_log_entries` Tail arm, `src/histogram.rs`
    // `Histogram::end_time`), so the search lands
    // on the last entry of the same-timestamp run and pagination walks
    // it newest to oldest.
    const TOTAL_ENTRIES: usize = 300;
    const PAGE_SIZE: usize = 200;
    let same_timestamp = JAN_1_2024_MIDNIGHT;

    let entries: Vec<TestEntry> = (0..TOTAL_ENTRIES)
        .map(|i| {
            TestEntry::new(same_timestamp)
                .with_field("MESSAGE", format!("Entry {}", i))
                .with_field("ENTRY_ID", i.to_string())
        })
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_index = indexer
        .index(
            &file,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let mut all_offsets = Vec::new();
    let mut all_positions = HashSet::new();
    let mut resume_position = None;

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(PAGE_SIZE)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("First page: {} entries", results.len());
    assert_eq!(
        results.len(),
        PAGE_SIZE,
        "First page should return PAGE_SIZE entries"
    );

    for entry in &results {
        assert_eq!(entry.timestamp, same_timestamp);
        all_offsets.push(entry.offset);
        assert!(
            all_positions.insert(entry.position),
            "Position {} appeared twice",
            entry.position
        );
    }

    if let Some(last_entry) = results.last() {
        resume_position = Some(last_entry.position);
    }

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(PAGE_SIZE)
        .with_resume_position(resume_position.unwrap())
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("Second page: {} entries", results.len());
    assert_eq!(
        results.len(),
        TOTAL_ENTRIES - PAGE_SIZE,
        "Second page should return remaining entries"
    );

    for entry in &results {
        assert_eq!(entry.timestamp, same_timestamp);
        all_offsets.push(entry.offset);
        assert!(
            all_positions.insert(entry.position),
            "Position {} appeared twice",
            entry.position
        );
    }

    if let Some(last_entry) = results.last() {
        resume_position = Some(last_entry.position);
    }

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(PAGE_SIZE)
        .with_resume_position(resume_position.unwrap())
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("Third page: {} entries", results.len());
    assert_eq!(results.len(), 0, "Third page should be empty");

    assert_eq!(
        all_offsets.len(),
        TOTAL_ENTRIES,
        "Should have retrieved all entries"
    );

    let unique_offsets: HashSet<_> = all_offsets.iter().collect();
    assert_eq!(
        unique_offsets.len(),
        TOTAL_ENTRIES,
        "All offsets should be unique"
    );

    // Together the pages covered positions 0..299: no gaps, no repeats.
    assert_eq!(
        all_positions.len(),
        TOTAL_ENTRIES,
        "Should have unique positions"
    );
    for i in 0..TOTAL_ENTRIES {
        assert!(all_positions.contains(&i), "Position {} missing", i);
    }
}

#[test]
fn test_pagination_forward_with_mixed_timestamps() {
    // 150 entries at T plus 150 at T+1s: PAGE_SIZE 200 makes the first
    // page straddle the timestamp boundary, and the resume must continue
    // midway through the T+1s run, returning only its entries.
    const ENTRIES_PER_TIMESTAMP: usize = 150;
    const PAGE_SIZE: usize = 200;

    let mut entries = Vec::new();

    let timestamp1 = JAN_1_2024_MIDNIGHT;
    for i in 0..ENTRIES_PER_TIMESTAMP {
        entries.push(
            TestEntry::new(timestamp1)
                .with_field("MESSAGE", format!("Batch 1 Entry {}", i))
                .with_field("ENTRY_ID", format!("1-{}", i)),
        );
    }

    let timestamp2 = Microseconds(timestamp1.0 + 1_000_000);
    for i in 0..ENTRIES_PER_TIMESTAMP {
        entries.push(
            TestEntry::new(timestamp2)
                .with_field("MESSAGE", format!("Batch 2 Entry {}", i))
                .with_field("ENTRY_ID", format!("2-{}", i)),
        );
    }

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_index = indexer
        .index(
            &file,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let mut all_offsets = Vec::new();
    let mut all_positions = HashSet::new();

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(PAGE_SIZE)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("First page: {} entries", results.len());
    assert_eq!(results.len(), PAGE_SIZE);

    for entry in &results {
        all_offsets.push(entry.offset);
        assert!(all_positions.insert(entry.position));
    }

    let resume_position = results.last().unwrap().position;

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(PAGE_SIZE)
        .with_resume_position(resume_position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    println!("Second page: {} entries", results.len());
    assert_eq!(results.len(), 100);

    for entry in &results {
        assert_eq!(entry.timestamp, timestamp2);
        all_offsets.push(entry.offset);
        assert!(all_positions.insert(entry.position));
    }

    assert_eq!(all_offsets.len(), 300);
    let unique_offsets: HashSet<_> = all_offsets.iter().collect();
    assert_eq!(unique_offsets.len(), 300);
}

#[test]
fn test_pagination_empty_journal() {
    let entries = Vec::new();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    // No entries means no (timestamp, offset) pairs, and the histogram
    // build rejects empty input (`src/histogram.rs`
    // `Histogram::from_timestamp_offset_pairs` empty-input check) — an empty journal
    // is an indexing error, not an empty index.
    let result = indexer.index(&file, None, &[], Seconds(3600));
    assert!(result.is_err(), "Empty journal should fail to index");
}

#[test]
fn test_pagination_single_entry() {
    // One entry: returned at position 0 in both directions, and
    // resuming from it is empty both ways.
    let timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![TestEntry::new(timestamp).with_field("MESSAGE", "Single entry")];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(100)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return single entry");
    assert_eq!(results[0].timestamp, timestamp);
    assert_eq!(results[0].position, 0);

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(100)
        .with_resume_position(results[0].position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "No more entries after single entry");

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(100)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return single entry");
    assert_eq!(results[0].timestamp, timestamp);
    assert_eq!(results[0].position, 0);

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(100)
        .with_resume_position(0)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from position 0 should return empty"
    );
}

#[test]
fn test_pagination_two_entries() {
    // A large limit returns both entries with contiguous positions;
    // limit 1 pages them one at a time in both directions.
    let timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![
        TestEntry::new(timestamp).with_field("MESSAGE", "Entry 1"),
        TestEntry::new(timestamp).with_field("MESSAGE", "Entry 2"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(10)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 2, "Should return both entries");
    assert_eq!(results[0].position, 0);
    assert_eq!(results[1].position, 1);

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(1)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return first entry");
    assert_eq!(results[0].position, 0);

    let first_position = results[0].position;

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(1)
        .with_resume_position(first_position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return second entry");
    assert_eq!(results[0].position, 1);

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(1)
        .with_resume_position(1)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "No third entry");

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(10)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 2, "Should return both entries backward");

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(1)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return last entry");
    assert_eq!(results[0].position, 1);

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(1)
        .with_resume_position(1)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return first entry");
    assert_eq!(results[0].position, 0);
}

#[test]
fn test_pagination_limit_zero() {
    let timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![
        TestEntry::new(timestamp).with_field("MESSAGE", "Entry 1"),
        TestEntry::new(timestamp).with_field("MESSAGE", "Entry 2"),
        TestEntry::new(timestamp).with_field("MESSAGE", "Entry 3"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Limit 0 is a valid query returning empty, rejected before the
    // scan (`src/file_index.rs` `find_log_entries` limit-0 check) — no
    // error, no entries.
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(0)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "Limit 0 should return no results");

    // Same for backward
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(0)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "Limit 0 should return no results");
}

#[test]
fn test_pagination_limit_exact_match() {
    const TOTAL_ENTRIES: usize = 50;
    let timestamp = JAN_1_2024_MIDNIGHT;

    let entries: Vec<TestEntry> = (0..TOTAL_ENTRIES)
        .map(|i| TestEntry::new(timestamp).with_field("ENTRY_ID", i.to_string()))
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Limit == total: the full set returns, and resuming from the last
    // position is empty — no wraparound, no duplicate.
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(TOTAL_ENTRIES)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), TOTAL_ENTRIES, "Should return all entries");

    let last_position = results.last().unwrap().position;
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(TOTAL_ENTRIES)
        .with_resume_position(last_position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "No more entries after exact match");
}

#[test]
fn test_pagination_limit_exceeds_total() {
    const TOTAL_ENTRIES: usize = 10;
    const LARGE_LIMIT: usize = 1000;
    let timestamp = JAN_1_2024_MIDNIGHT;

    let entries: Vec<TestEntry> = (0..TOTAL_ENTRIES)
        .map(|i| TestEntry::new(timestamp).with_field("ENTRY_ID", i.to_string()))
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Limit beyond the total: returns exactly what exists, never padded.
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(LARGE_LIMIT)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        TOTAL_ENTRIES,
        "Should return all entries (not more than available)"
    );

    // Forward returns positions 0..9 in order.
    for (i, entry) in results.iter().enumerate() {
        assert_eq!(entry.position, i);
    }

    // Same for backward
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(LARGE_LIMIT)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        TOTAL_ENTRIES,
        "Should return all entries backward"
    );
}

#[test]
fn test_pagination_resume_out_of_bounds() {
    // Out-of-bounds resume positions must yield empty results, never a
    // panic or error: forward clamps at the end-of-list check
    // (`src/file_index.rs` `find_log_entries` Forward arm), backward
    // rejects 0 and >= len up front
    // (`src/file_index.rs` `find_log_entries` Backward arm).
    const TOTAL_ENTRIES: usize = 10;
    let timestamp = JAN_1_2024_MIDNIGHT;

    let entries: Vec<TestEntry> = (0..TOTAL_ENTRIES)
        .map(|i| TestEntry::new(timestamp).with_field("ENTRY_ID", i.to_string()))
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Forward: Resume from the last valid position — a page starts one
    // past the resume point, so nothing remains.
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(10)
        .with_resume_position(TOTAL_ENTRIES - 1)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Resume from last position should return empty"
    );

    // Forward: Resume from position beyond total entries
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(10)
        .with_resume_position(TOTAL_ENTRIES)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Resume from beyond last position should return empty"
    );

    // Forward: Resume from way beyond total entries
    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_limit(10)
        .with_resume_position(999)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Resume from way beyond should return empty (not panic)"
    );

    // Backward: Resume from 0 has nothing before it.
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(10)
        .with_resume_position(0)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from position 0 should return empty"
    );

    // Backward: Resume from position equal to total entries
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(10)
        .with_resume_position(TOTAL_ENTRIES)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from position equal to total should return empty (not panic)"
    );

    // Backward: Resume from position beyond total entries
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(10)
        .with_resume_position(TOTAL_ENTRIES + 5)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from beyond total should return empty (not panic)"
    );

    // Backward: Resume from way beyond total entries
    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_limit(10)
        .with_resume_position(999)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from way beyond should return empty (not panic)"
    );
}

#[test]
fn test_pagination_anchor_before_all_entries() {
    // Anchor at 09:00 sits before every entry: forward still returns
    // all three; backward has nothing at-or-before the anchor and
    // returns none.
    let base_timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![
        TestEntry::new(Microseconds(base_timestamp.0 + 10 * 3600_000_000))
            .with_field("ENTRY_ID", "0"),
        TestEntry::new(Microseconds(base_timestamp.0 + 11 * 3600_000_000))
            .with_field("ENTRY_ID", "1"),
        TestEntry::new(Microseconds(base_timestamp.0 + 12 * 3600_000_000))
            .with_field("ENTRY_ID", "2"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Anchor at 09:00 (before all entries), going forward
    let anchor_timestamp = Microseconds(base_timestamp.0 + 9 * 3600_000_000);
    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Forward)
            .with_limit(10)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        3,
        "Forward from before all entries should return all entries"
    );

    // Anchor at 09:00, going backward
    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Backward)
            .with_limit(10)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Backward from before all entries should return no entries"
    );
}

#[test]
fn test_pagination_anchor_after_all_entries() {
    // Mirror: anchor at 13:00 sits after every entry — forward has
    // nothing after it; backward returns all three.
    let base_timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![
        TestEntry::new(Microseconds(base_timestamp.0 + 10 * 3600_000_000))
            .with_field("ENTRY_ID", "0"),
        TestEntry::new(Microseconds(base_timestamp.0 + 11 * 3600_000_000))
            .with_field("ENTRY_ID", "1"),
        TestEntry::new(Microseconds(base_timestamp.0 + 12 * 3600_000_000))
            .with_field("ENTRY_ID", "2"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Anchor at 13:00 (after all entries), going forward
    let anchor_timestamp = Microseconds(base_timestamp.0 + 13 * 3600_000_000);
    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Forward)
            .with_limit(10)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        0,
        "Forward from after all entries should return no entries"
    );

    // Anchor at 13:00, going backward
    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Backward)
            .with_limit(10)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        3,
        "Backward from after all entries should return all entries"
    );
}

#[test]
fn test_pagination_anchor_in_middle_with_pagination() {
    // Anchor exactly on the middle entry: Anchor::Timestamp is
    // inclusive in both directions — the entry at the anchor starts
    // each walk — and pagination resumes through the rest.
    let base_timestamp = JAN_1_2024_MIDNIGHT;
    let entries = vec![
        TestEntry::new(Microseconds(base_timestamp.0 + 10 * 3600_000_000))
            .with_field("ENTRY_ID", "0"),
        TestEntry::new(Microseconds(base_timestamp.0 + 11 * 3600_000_000))
            .with_field("ENTRY_ID", "1"),
        TestEntry::new(Microseconds(base_timestamp.0 + 12 * 3600_000_000))
            .with_field("ENTRY_ID", "2"),
        TestEntry::new(Microseconds(base_timestamp.0 + 13 * 3600_000_000))
            .with_field("ENTRY_ID", "3"),
        TestEntry::new(Microseconds(base_timestamp.0 + 14 * 3600_000_000))
            .with_field("ENTRY_ID", "4"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    let anchor_timestamp = Microseconds(base_timestamp.0 + 12 * 3600_000_000);
    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Forward)
            .with_limit(2)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        2,
        "Should return 2 entries starting from anchor"
    );
    assert_eq!(results[0].position, 2);
    assert_eq!(results[1].position, 3);

    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Forward)
            .with_limit(2)
            .with_resume_position(results[1].position)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return remaining 1 entry");
    assert_eq!(results[0].position, 4);

    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Backward)
            .with_limit(2)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        2,
        "Should return 2 entries backward from anchor"
    );
    assert_eq!(results[0].position, 2);
    assert_eq!(results[1].position, 1);

    let params =
        LogQueryParamsBuilder::new(Anchor::Timestamp(anchor_timestamp), Direction::Backward)
            .with_limit(2)
            .with_resume_position(results[1].position)
            .build()
            .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 1, "Should return remaining 1 entry");
    assert_eq!(results[0].position, 0);
}

#[test]
fn test_pagination_with_time_boundaries() {
    let base_timestamp = JAN_1_2024_MIDNIGHT;
    let entries: Vec<TestEntry> = (0..20)
        .map(|i| {
            TestEntry::new(Microseconds(base_timestamp.0 + i * 3600_000_000))
                .with_field("ENTRY_ID", i.to_string())
        })
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Window [after = hour 5, before = hour 15): entries 5..14. Positions
    // stay absolute in the index's entry list, so page 1 reports 5..8 —
    // not 0..3 — and resume continues at 9.
    let after = Microseconds(base_timestamp.0 + 5 * 3600_000_000);
    let before = Microseconds(base_timestamp.0 + 15 * 3600_000_000);

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 4, "First page should return 4 entries");
    assert_eq!(results[0].position, 5);
    assert_eq!(results[3].position, 8);

    let mut all_results = results.clone();

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 4, "Second page should return 4 entries");
    assert_eq!(results[0].position, 9);
    assert_eq!(results[3].position, 12);

    all_results.extend(results.clone());

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        2,
        "Third page should return remaining 2 entries"
    );
    assert_eq!(results[0].position, 13);
    assert_eq!(results[1].position, 14);

    all_results.extend(results.clone());

    let params = LogQueryParamsBuilder::new(Anchor::Head, Direction::Forward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "Fourth page should be empty");

    assert_eq!(all_results.len(), 10);

    // Window edges pinned: after inclusive, before exclusive, in both
    // walk directions (`src/file_index.rs` `find_log_entries` boundary
    // checks).
    for entry in &all_results {
        assert!(
            entry.timestamp.0 >= after.0,
            "Entry timestamp should be >= after boundary"
        );
        assert!(
            entry.timestamp.0 < before.0,
            "Entry timestamp should be < before boundary"
        );
    }
}

#[test]
fn test_pagination_backward_with_time_boundaries() {
    let base_timestamp = JAN_1_2024_MIDNIGHT;
    let entries: Vec<TestEntry> = (0..20)
        .map(|i| {
            TestEntry::new(Microseconds(base_timestamp.0 + i * 3600_000_000))
                .with_field("ENTRY_ID", i.to_string())
        })
        .collect();

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();
    let file_index = indexer.index(&file, None, &[], Seconds(3600)).unwrap();

    // Same window walked backward: entries 14 down to 5, with the same
    // after-inclusive/before-exclusive edges (`src/file_index.rs`
    // `find_log_entries` Backward boundary checks).
    let after = Microseconds(base_timestamp.0 + 5 * 3600_000_000);
    let before = Microseconds(base_timestamp.0 + 15 * 3600_000_000);

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 4, "First page should return 4 entries");
    assert_eq!(results[0].position, 14);
    assert_eq!(results[3].position, 11);

    let mut all_results = results.clone();

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 4, "Second page should return 4 entries");
    assert_eq!(results[0].position, 10);
    assert_eq!(results[3].position, 7);

    all_results.extend(results.clone());

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(
        results.len(),
        2,
        "Third page should return remaining 2 entries"
    );
    assert_eq!(results[0].position, 6);
    assert_eq!(results[1].position, 5);

    all_results.extend(results.clone());

    let params = LogQueryParamsBuilder::new(Anchor::Tail, Direction::Backward)
        .with_after(after)
        .with_before(before)
        .with_limit(4)
        .with_resume_position(results.last().unwrap().position)
        .build()
        .unwrap();

    let results = file_index.find_log_entries(&file, &params).unwrap();
    assert_eq!(results.len(), 0, "Fourth page should be empty");

    assert_eq!(all_results.len(), 10);

    // Window edges pinned: after inclusive, before exclusive, in both
    // walk directions (`src/file_index.rs` `find_log_entries` boundary
    // checks).
    for entry in &all_results {
        assert!(
            entry.timestamp.0 >= after.0,
            "Entry timestamp should be >= after boundary"
        );
        assert!(
            entry.timestamp.0 < before.0,
            "Entry timestamp should be < before boundary"
        );
    }
}
