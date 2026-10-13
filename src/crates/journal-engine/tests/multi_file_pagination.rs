//! Integration tests for multi-file pagination through the engine's
//! [`LogQuery::execute_page`]: each test
//! writes 2-3 fresh journal files, indexes them with
//! `FileIndexer::index`, and pages across the merged result, threading
//! `PaginationState` from page to page. The state type is never named
//! here - it is not re-exported from `logs` and
//! only arrives through `execute_page`'s return value.
//!
//! Fixture: `file1.journal`..`fileN.journal` in one machine-id dir.
//! Any `<name>.journal` parses as an active journal whose source is
//! derived from the basename
//! (`journal-registry/src/repository/file.rs` `File::from_path`),
//! and distinct names give distinct `File` identities (its derived
//! `Eq`/`Hash`, `journal-registry/src/repository/file.rs`)
//! - what keeps the per-file positions in
//! `PaginationState.file_positions` (`logs/query.rs` `PaginationState`) separate.
//! Most tests chain disjoint entry runs across the files (typically
//! 100 entries per file); overlaps, shared timestamps, an empty file,
//! a filter, time windows and boundary anchors are called out per
//! test. Every entry is stamped with
//! `_SOURCE_REALTIME_TIMESTAMP=<microseconds>` - the tests index that
//! field explicitly, so each file's entry list is time-ordered by it
//! (`journal-index/src/file_indexer.rs` `FileIndexer::index`). Tests
//! that need per-entry identity also stamp a unique `ENTRY_ID=fileN_<i>`:
//! the prefix identifies the source file in the distribution
//! assertions, the value is the dedup key wherever timestamps repeat -
//! the two non-overlapping tests dedup on timestamps alone and stamp
//! no `ENTRY_ID`. Several tests also stamp and index a per-file
//! `FILE` field that no assertion reads back.
//!
//! How a page is built (`logs/query.rs` `retrieve_log_entries`): files are
//! processed in stable temporal sort order - start_time ascending
//! forward, end_time descending backward (the sort in
//! `retrieve_log_entries`).
//! Each file walks only its own entry list from the per-file-resolved
//! anchor (Head/Tail map to that file's own histogram bounds;
//! Timestamp is the anchor value, inclusive in both directions; see
//! `journal-index/src/file_index.rs` `FileIndex::find_log_entries`),
//! and each per-file merge re-interleaves the results into one
//! globally timestamp-ordered page capped at the limit
//! (`logs/query.rs` `merge_log_entries`; ties go to the
//! already-collected side).
//! Page order is global regardless of processing
//! order; processing order only decides which entries survive the cap
//! - a file whose fetch is wholly displaced by the cap keeps no resume
//! position and is re-walked from its anchor on the next page
//! (the state merge in `retrieve_log_entries`), which the per-entry dedup
//! assertions prove loses nothing. The fixtures use microsecond-scale
//! timestamps inside one 3600 s histogram bucket, so every file's
//! Head/Tail bounds coincide and the walks effectively start at each
//! file's first/last entry.
//!
//! Pinned contracts:
//!
//! - `PaginationState` holds one position per file - the max
//!   (forward) / min (backward) `LogEntryId.position` among entries
//!   that actually landed in the page
//!   (`journal-index/src/file_index.rs` `LogEntryId::position`; the
//!   state merge in `retrieve_log_entries`),
//!   merged over the previous state so
//!   positions persist across pages where a file contributed nothing.
//! - Resume is per file, not global: forward starts at
//!   `resume_position + 1`, backward at `resume_position - 1`
//!   (the per-file params rebuild in `retrieve_log_entries`;
//!   `journal-index/src/file_index.rs` `FileIndex::find_log_entries`).
//! - `Anchor::Head`/`Tail` take the minimum start / maximum end across
//!   all files (the anchor resolution in `retrieve_log_entries`).
//! - Time boundaries apply during each file's walk: `after` inclusive,
//!   `before` exclusive (`LogQuery::with_after_usec`/`with_before_usec`;
//!   the boundary checks in `journal-index/src/file_index.rs`
//!   `FileIndex::find_log_entries`).
//! - A filtered query paginates each file's FILTERED entry list
//!   (`journal-index/src/file_index.rs` `LogEntryId::position`) and every page
//!   re-applies the current query's filter (the per-file params
//!   rebuild in `retrieve_log_entries`),
//!   so the filter must not change between pages.
//!
//! Not pinned here: unlimited queries (every test sets a limit),
//! limit 0 (the early return in `retrieve_log_entries`), regex search,
//! cancellation and
//! progress counters (`LogQuery::with_cancellation`/`with_progress`),
//! output-field
//! projection (`LogQuery::with_output_fields`), per-file read errors (logged
//! and skipped in `retrieve_log_entries`), the per-direction relevance
//! pre-filter (the anchor pre-filter in `retrieve_log_entries` - every file qualifies in
//! these fixtures), the prune/skip path (its break condition is never
//! met here, `logs/query.rs` `can_prune_file`), which file leads a
//! page when timestamps tie (assertions count per-file totals, not
//! page layout), and changing any query parameter between pages - the
//! state is bound to the configuration that produced it
//! (`PaginationState`'s `logs/query.rs` docs).

use journal_common::Seconds;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_engine::logs::query::LogQuery;
use journal_index::{
    Anchor, Direction, FieldName, FieldValuePair, FileIndexer, Filter, Microseconds,
};
use std::collections::HashSet;
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

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

/// Path of a test journal: `<tmp>/<machine-id>/<filename>` - the
/// registry grammar `File::from_path` parses
/// (`journal-registry/src/repository/file.rs`). All of a test's
/// files share the machine-id dir; distinct names keep their `File`
/// identities - and `PaginationState`'s per-file positions - separate.
fn create_test_journal_path(temp_dir: &TempDir, filename: &str) -> PathBuf {
    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let machine_dir = temp_dir.path().join(machine_id.to_string());
    fs::create_dir_all(&machine_dir).expect("create machine dir");
    machine_dir.join(filename)
}

/// Write `entries` into a fresh journal under `temp_dir` and return
/// the registry `File` for it.
///
/// Every entry is stamped with
/// `_SOURCE_REALTIME_TIMESTAMP=<microseconds>` and `add_entry`
/// receives the same value as realtime and monotonic
/// (`journal-core/src/file/writer.rs` `JournalWriter::add_entry`). Zero entries still
/// writes a valid journal - one that cannot be indexed (the
/// empty-file test relies on that).
fn create_test_journal(
    temp_dir: &TempDir,
    filename: &str,
    entries: Vec<TestEntry>,
) -> Result<File, Box<dyn std::error::Error>> {
    let journal_path = create_test_journal_path(temp_dir, filename);

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

    Ok(file)
}

#[test]
fn test_multi_file_pagination_forward_non_overlapping() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page: limit=150, should get all 100 from file1 + 50 from file2
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        150,
        "First page should contain exactly 150 entries"
    );

    // The merged page is globally timestamp-ordered across both files.
    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp <= first_page[i].timestamp,
            "Entries should be in ascending timestamp order"
        );
    }

    assert_eq!(
        first_page.first().unwrap().timestamp,
        100,
        "First entry should be at timestamp 100"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        249,
        "Last entry of first page should be at timestamp 249"
    );

    // Both files contributed, so the state starts tracking both.
    assert!(
        !state1.file_positions.is_empty(),
        "State should track positions"
    );

    // Second page draws only from file2: file1's resume is past its
    // end and yields nothing.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        50,
        "Second page should contain remaining 50 entries"
    );

    assert_eq!(
        second_page.first().unwrap().timestamp,
        250,
        "Second page should start at timestamp 250"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        299,
        "Second page should end at timestamp 299"
    );

    // Together the pages must cover each of the 200 entries exactly
    // once - no cross-file duplicates, none missed.
    let mut all_timestamps = HashSet::new();
    for entry in &first_page {
        assert!(
            all_timestamps.insert(entry.timestamp),
            "Found duplicate timestamp: {}",
            entry.timestamp
        );
    }
    for entry in &second_page {
        assert!(
            all_timestamps.insert(entry.timestamp),
            "Found duplicate timestamp: {}",
            entry.timestamp
        );
    }

    assert_eq!(
        all_timestamps.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_same_timestamps() {
    let temp_dir = TempDir::new().unwrap();

    // 150 entries per file, all at timestamp 1000 - one 300-entry run
    // split across two files. The anchor can only locate the run's
    // edge, so in-file stepping runs on resume positions
    // (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`) and the merge
    // tie-break (`logs/query.rs` `merge_log_entries`), not on time.
    let entries_file1: Vec<TestEntry> = (0..150)
        .map(|i| {
            TestEntry::new(Microseconds(1000))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (0..150)
        .map(|i| {
            TestEntry::new(Microseconds(1000))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page (limit 200): one file's whole 150-entry run plus the
    // first 50 of the other - with tied timestamps, which file leads
    // is a merge-tie artifact, so only totals are asserted.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(200)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        200,
        "First page should contain exactly 200 entries"
    );

    for entry in &first_page {
        assert_eq!(
            entry.timestamp, 1000,
            "All entries should have timestamp 1000"
        );
    }

    // Both files contributed, so both carry positions.
    assert_eq!(
        state1.file_positions.len(),
        2,
        "State should track positions for both files"
    );

    // Second page: the leading file's resume is past its end; the
    // other contributes its remaining 100.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(200)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        100,
        "Second page should contain remaining 100 entries"
    );

    for entry in &second_page {
        assert_eq!(
            entry.timestamp, 1000,
            "All entries should have timestamp 1000"
        );
    }

    // ENTRY_IDs must be unique across pages: all 300 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in &first_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    for entry in &second_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        300,
        "Should have retrieved all 300 unique entries"
    );

    // 150 from each file.
    let file1_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();

    assert_eq!(file1_entries, 150, "Should have 150 entries from file1");
    assert_eq!(file2_entries, 150, "Should have 150 entries from file2");

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(200)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_overlapping_timestamps() {
    let temp_dir = TempDir::new().unwrap();

    // Overlapping files: file1 holds 100-199, file2 150-249 -
    // timestamps 150-199 exist in both.
    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry at {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (150..250)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry at {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page (limit 120): file1's 50 exclusive entries (100-149),
    // then the overlap interleaved by timestamp - the cap falls
    // mid-overlap at 184, so page 1 never reaches file2's exclusive
    // range.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(120)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        120,
        "First page should contain exactly 120 entries"
    );

    // The merged page is globally timestamp-ordered across both files.
    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp <= first_page[i].timestamp,
            "Entries should be in ascending timestamp order"
        );
    }

    assert_eq!(
        first_page.first().unwrap().timestamp,
        100,
        "First entry should be at timestamp 100"
    );

    // Both files contributed, so the state starts tracking both.
    assert!(
        !state1.file_positions.is_empty(),
        "State should track positions"
    );

    // Second page: both files resume mid-run at 185 and finish their
    // own tails - file1 through 199, file2 through 249.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(120)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        80,
        "Second page should contain remaining 80 entries"
    );

    for i in 1..second_page.len() {
        assert!(
            second_page[i - 1].timestamp <= second_page[i].timestamp,
            "Entries should be in ascending timestamp order"
        );
    }

    // No gap: page 2 starts exactly where page 1 stopped.
    if !first_page.is_empty() && !second_page.is_empty() {
        assert!(
            first_page.last().unwrap().timestamp <= second_page.first().unwrap().timestamp,
            "Second page should continue from first page timestamp"
        );
    }

    assert_eq!(
        second_page.last().unwrap().timestamp,
        249,
        "Last entry should be at timestamp 249"
    );

    // ENTRY_IDs must be unique across pages: all 200 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in &first_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    for entry in &second_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    // 100 from each file - the overlap neither duplicated nor dropped
    // either file's entries.
    let file1_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();

    assert_eq!(file1_entries, 100, "Should have 100 entries from file1");
    assert_eq!(file2_entries, 100, "Should have 100 entries from file2");

    let mut all_timestamps = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        all_timestamps.insert(entry.timestamp);
    }

    // Every timestamp 100-249 is represented - shared timestamps
    // yield both files' entries (200 entries over 150 timestamps).
    for ts in 100..250 {
        assert!(all_timestamps.contains(&ts), "Missing timestamp: {}", ts);
    }

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(120)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_three_files() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (300..400)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
                .with_field("FILE", "3")
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2, index3];

    // First page: limit=125, should get all 100 from file1 + 25 from file2
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(125)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        125,
        "First page should contain exactly 125 entries"
    );

    assert_eq!(first_page.first().unwrap().timestamp, 100);
    assert_eq!(first_page.last().unwrap().timestamp, 224);

    // State: file1 done, file2 mid-run; file3 untouched, so only 2
    // positions.
    assert_eq!(
        state1.file_positions.len(),
        2,
        "State should track positions for 2 files"
    );

    // Second page: limit=125, should get 75 from file2 + 50 from file3
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(125)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        125,
        "Second page should contain exactly 125 entries"
    );

    assert_eq!(second_page.first().unwrap().timestamp, 225);
    assert_eq!(second_page.last().unwrap().timestamp, 349);

    // State now covers all 3 files - file1's position persisted even
    // though page 2 drew nothing from it.
    assert_eq!(
        state2.file_positions.len(),
        3,
        "State should track positions for all 3 files"
    );

    // Third page: remaining 50 from file3
    let (third_page, state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(125)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        50,
        "Third page should contain remaining 50 entries"
    );

    assert_eq!(third_page.first().unwrap().timestamp, 350);
    assert_eq!(third_page.last().unwrap().timestamp, 399);

    // ENTRY_IDs must be unique across pages: all 300 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in first_page
        .iter()
        .chain(second_page.iter())
        .chain(third_page.iter())
    {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        300,
        "Should have retrieved all 300 unique entries"
    );

    // 100 from each file.
    let file1_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();
    let file3_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file3_"))
        .count();

    assert_eq!(file1_count, 100, "Should have 100 entries from file1");
    assert_eq!(file2_count, 100, "Should have 100 entries from file2");
    assert_eq!(file3_count, 100, "Should have 100 entries from file3");

    let (fourth_page, _state4) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(125)
        .execute_page(Some(&state3))
        .unwrap();

    assert_eq!(
        fourth_page.len(),
        0,
        "Fourth page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_small_limit() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // limit 30: file2's fetches are displaced by the cap until page 4,
    // so the first pages drain file1 alone.
    let mut all_entry_ids = HashSet::new();
    let mut state = None;
    let mut page_count = 0;

    // Page until empty, threading the state through each round trip.
    loop {
        let (page, new_state) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
            .with_limit(30)
            .execute_page(state.as_ref())
            .unwrap();

        if page.is_empty() {
            break;
        }

        page_count += 1;

        // Each page stays timestamp-ordered.
        for i in 1..page.len() {
            assert!(
                page[i - 1].timestamp <= page[i].timestamp,
                "Page {} entries should be in ascending order",
                page_count
            );
        }

        // ENTRY_IDs must never repeat.
        for entry in &page {
            for field in &entry.fields {
                if field.field() == "ENTRY_ID" {
                    assert!(
                        all_entry_ids.insert(field.value().to_string()),
                        "Found duplicate ENTRY_ID: {}",
                        field.value()
                    );
                }
            }
        }

        state = Some(new_state);
    }

    // Should need 7 pages: 30+30+30+30+30+30+20 = 200 entries
    assert_eq!(page_count, 7, "Should need exactly 7 pages");

    assert_eq!(
        all_entry_ids.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    // 100 from each file - the cap's displaced fetches lost nothing.
    let file1_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();

    assert_eq!(file1_count, 100, "Should have 100 entries from file1");
    assert_eq!(file2_count, 100, "Should have 100 entries from file2");
}

#[test]
fn test_multi_file_pagination_limit_one() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..110)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (110..120)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // limit 1: every page boundary is a resume - 20 pages of one
    // entry each.
    let mut all_entry_ids = HashSet::new();
    let mut all_timestamps = Vec::new();
    let mut state = None;
    let mut page_count = 0;

    loop {
        let (page, new_state) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
            .with_limit(1)
            .execute_page(state.as_ref())
            .unwrap();

        if page.is_empty() {
            break;
        }

        page_count += 1;

        assert_eq!(page.len(), 1, "Each page should have exactly 1 entry");

        // ENTRY_IDs must never repeat; timestamps are collected for
        // the ordering check.
        for entry in &page {
            all_timestamps.push(entry.timestamp);
            for field in &entry.fields {
                if field.field() == "ENTRY_ID" {
                    assert!(
                        all_entry_ids.insert(field.value().to_string()),
                        "Found duplicate ENTRY_ID: {}",
                        field.value()
                    );
                }
            }
        }

        state = Some(new_state);
    }

    // Should need 20 pages for 20 entries
    assert_eq!(page_count, 20, "Should need exactly 20 pages");

    assert_eq!(
        all_entry_ids.len(),
        20,
        "Should have retrieved all 20 unique entries"
    );

    // The single-entry pages chain into one globally ascending stream
    // across the file boundary.
    for i in 1..all_timestamps.len() {
        assert!(
            all_timestamps[i - 1] <= all_timestamps[i],
            "Timestamps should be in ascending order"
        );
    }

    // Every timestamp 100-119 appears exactly once.
    let unique_timestamps: HashSet<_> = all_timestamps.into_iter().collect();
    assert_eq!(
        unique_timestamps.len(),
        20,
        "Should have 20 unique timestamps"
    );
    for ts in 100..120 {
        assert!(unique_timestamps.contains(&ts), "Missing timestamp: {}", ts);
    }
}

#[test]
fn test_multi_file_pagination_with_empty_file() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..150)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    // file2 is written with zero entries and then dropped: an empty
    // journal cannot be indexed.
    let entries_file2: Vec<TestEntry> = vec![];
    let _file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (150..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    // Indexing an empty journal fails with EmptyHistogramInput
    // (`journal-index/src/histogram.rs` `Histogram::from_timestamp_offset_pairs`),
    // so file2 is simply left
    // out of the query list.
    // Production behaves the same: per-file indexing failures are
    // logged and the file omitted (`indexing.rs` `batch_compute_file_indexes`).
    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    // No index for file2 - the query sees only file1 and file3.

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index3];

    // First page: limit=60, should get 50 from file1 + 10 from file3 (skipping empty file2)
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(60)
        .execute_page(None)
        .unwrap();

    assert_eq!(first_page.len(), 60, "First page should contain 60 entries");

    assert_eq!(first_page.first().unwrap().timestamp, 100);
    assert_eq!(first_page.last().unwrap().timestamp, 159);

    // Second page: remaining 40 from file3
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(60)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        40,
        "Second page should contain remaining 40 entries"
    );

    assert_eq!(second_page.first().unwrap().timestamp, 160);
    assert_eq!(second_page.last().unwrap().timestamp, 199);

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // Verify we got all 100 unique entries (none from empty file)
    assert_eq!(
        all_entry_ids.len(),
        100,
        "Should have retrieved all 100 unique entries"
    );

    let file1_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file3_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file3_"))
        .count();

    assert_eq!(file1_count, 50, "Should have 50 entries from file1");
    assert_eq!(file3_count, 50, "Should have 50 entries from file3");

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(60)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_reverse_file_order() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (300..400)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    // The files are handed to the query newest-first; the temporal
    // re-sort (`logs/query.rs` `retrieve_log_entries`) must undo that.
    let file_indexes = vec![index3, index2, index1];

    // Each page is still globally ascending: every per-file merge
    // re-interleaves by timestamp (`logs/query.rs` `merge_log_entries`), so
    // processing order never leaks into page order.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(None)
        .unwrap();

    assert_eq!(first_page.len(), 150);

    assert_eq!(
        first_page.first().unwrap().timestamp,
        100,
        "First entry should be at timestamp 100 from file1"
    );

    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp <= first_page[i].timestamp,
            "Entries should be in ascending order"
        );
    }

    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(second_page.len(), 150);

    for i in 1..second_page.len() {
        assert!(
            second_page[i - 1].timestamp <= second_page[i].timestamp,
            "Entries should be in ascending order"
        );
    }

    // No gap across the page boundary.
    assert!(
        first_page.last().unwrap().timestamp <= second_page.first().unwrap().timestamp,
        "Second page should continue from first page"
    );

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // All 300 entries covered exactly once - newest-first input
    // changed nothing.
    assert_eq!(
        all_entry_ids.len(),
        300,
        "Should have retrieved all 300 unique entries"
    );

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_limit(150)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(third_page.len(), 0, "Third page should be empty");
}

#[test]
fn test_multi_file_pagination_backward_non_overlapping() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page: limit=150, backward from tail, should get all 100 from file2 + 50 from file1
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(150)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        150,
        "First page should contain exactly 150 entries"
    );

    // The merged page is globally descending across both files.
    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp >= first_page[i].timestamp,
            "Entries should be in descending timestamp order"
        );
    }

    assert_eq!(
        first_page.first().unwrap().timestamp,
        299,
        "First entry should be at timestamp 299"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        150,
        "Last entry of first page should be at timestamp 150"
    );

    // Both files contributed, so the state starts tracking both.
    assert!(
        !state1.file_positions.is_empty(),
        "State should track positions"
    );

    // Second page draws only from file1: file2's resume reached
    // position 0, where the backward walk ends
    // (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`).
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(150)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        50,
        "Second page should contain remaining 50 entries"
    );

    assert_eq!(
        second_page.first().unwrap().timestamp,
        149,
        "Second page should start at timestamp 149"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        100,
        "Second page should end at timestamp 100"
    );

    // Together the pages must cover each of the 200 entries exactly
    // once.
    let mut all_timestamps = HashSet::new();
    for entry in &first_page {
        assert!(
            all_timestamps.insert(entry.timestamp),
            "Found duplicate timestamp: {}",
            entry.timestamp
        );
    }
    for entry in &second_page {
        assert!(
            all_timestamps.insert(entry.timestamp),
            "Found duplicate timestamp: {}",
            entry.timestamp
        );
    }

    assert_eq!(
        all_timestamps.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(150)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_backward_same_timestamps() {
    let temp_dir = TempDir::new().unwrap();

    // 150 entries per file, all at timestamp 1000 - one 300-entry run
    // split across two files, walked newest-first.
    let entries_file1: Vec<TestEntry> = (0..150)
        .map(|i| {
            TestEntry::new(Microseconds(1000))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (0..150)
        .map(|i| {
            TestEntry::new(Microseconds(1000))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page (limit 200): one file's whole run plus the last 50
    // of the other - with tied timestamps, which file leads is a
    // merge-tie artifact, so only totals are asserted.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(200)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        200,
        "First page should contain exactly 200 entries"
    );

    for entry in &first_page {
        assert_eq!(
            entry.timestamp, 1000,
            "All entries should have timestamp 1000"
        );
    }

    // Both files contributed, so both carry positions.
    assert_eq!(
        state1.file_positions.len(),
        2,
        "State should track positions for both files"
    );

    // Second page: the leading file's resume hit 0 (the backward
    // walk's end); the other contributes its remaining 100.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(200)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        100,
        "Second page should contain remaining 100 entries"
    );

    for entry in &second_page {
        assert_eq!(
            entry.timestamp, 1000,
            "All entries should have timestamp 1000"
        );
    }

    // ENTRY_IDs must be unique across pages: all 300 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in &first_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    for entry in &second_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        300,
        "Should have retrieved all 300 unique entries"
    );

    // 150 from each file.
    let file1_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();

    assert_eq!(file1_entries, 150, "Should have 150 entries from file1");
    assert_eq!(file2_entries, 150, "Should have 150 entries from file2");

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(200)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_backward_limit_one() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..110)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (110..120)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // limit 1 backward: 20 pages of one entry each, newest first.
    let mut all_entry_ids = HashSet::new();
    let mut all_timestamps = Vec::new();
    let mut state = None;
    let mut page_count = 0;

    loop {
        let (page, new_state) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
            .with_limit(1)
            .execute_page(state.as_ref())
            .unwrap();

        if page.is_empty() {
            break;
        }

        page_count += 1;

        assert_eq!(page.len(), 1, "Each page should have exactly 1 entry");

        // ENTRY_IDs must never repeat; timestamps are collected for
        // the ordering check.
        for entry in &page {
            all_timestamps.push(entry.timestamp);
            for field in &entry.fields {
                if field.field() == "ENTRY_ID" {
                    assert!(
                        all_entry_ids.insert(field.value().to_string()),
                        "Found duplicate ENTRY_ID: {}",
                        field.value()
                    );
                }
            }
        }

        state = Some(new_state);
    }

    // Should need 20 pages for 20 entries
    assert_eq!(page_count, 20, "Should need exactly 20 pages");

    assert_eq!(
        all_entry_ids.len(),
        20,
        "Should have retrieved all 20 unique entries"
    );

    // The single-entry pages chain into one globally descending
    // stream across the file boundary.
    for i in 1..all_timestamps.len() {
        assert!(
            all_timestamps[i - 1] >= all_timestamps[i],
            "Timestamps should be in descending order"
        );
    }

    // Every timestamp 100-119 appears exactly once.
    let unique_timestamps: HashSet<_> = all_timestamps.into_iter().collect();
    assert_eq!(
        unique_timestamps.len(),
        20,
        "Should have 20 unique timestamps"
    );
    for ts in 100..120 {
        assert!(unique_timestamps.contains(&ts), "Missing timestamp: {}", ts);
    }
}

#[test]
fn test_multi_file_pagination_anchor_timestamp_forward() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // Anchor 150 sits mid-file1: each file starts its walk at the
    // anchor, inclusive (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`).
    let anchor = Anchor::Timestamp(Microseconds(150));

    // First page: limit=80, should get 50 from file1 (150-199) + 30 from file2 (200-229)
    let (first_page, state1) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        80,
        "First page should contain exactly 80 entries"
    );

    assert_eq!(
        first_page.first().unwrap().timestamp,
        150,
        "First entry should be at timestamp 150 (anchor)"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        229,
        "Last entry should be at timestamp 229"
    );

    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp <= first_page[i].timestamp,
            "Entries should be in ascending order"
        );
    }

    // Second page: file2's remaining 70 (230-299); file1's resume is
    // past its end.
    let (second_page, state2) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        70,
        "Second page should contain remaining 70 entries (230-299)"
    );

    assert_eq!(
        second_page.first().unwrap().timestamp,
        230,
        "Second page should start at timestamp 230"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        299,
        "Second page should end at timestamp 299"
    );

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // 150 entries - everything from the anchor onward, nothing before
    // it.
    assert_eq!(
        all_entry_ids.len(),
        150,
        "Should have retrieved 150 unique entries from timestamp 150 onwards"
    );

    let (third_page, _state3) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_anchor_timestamp_backward() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // Anchor 250 sits mid-file2: each file walks backward from the
    // anchor, inclusive (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`) -
    // file2 from its 250 entry, file1 from its 199.
    let anchor = Anchor::Timestamp(Microseconds(250));

    // First page: limit=80, should get 51 from file2 (250-200) + 29 from file1 (199-171)
    let (first_page, state1) = LogQuery::new(&file_indexes, anchor, Direction::Backward)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        80,
        "First page should contain exactly 80 entries"
    );

    assert_eq!(
        first_page.first().unwrap().timestamp,
        250,
        "First entry should be at timestamp 250 (anchor)"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        171,
        "Last entry should be at timestamp 171"
    );

    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp >= first_page[i].timestamp,
            "Entries should be in descending order"
        );
    }

    // Second page: file2's resume hit 0; file1 contributes its
    // remaining 71 (170-100).
    let (second_page, state2) = LogQuery::new(&file_indexes, anchor, Direction::Backward)
        .with_limit(80)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        71,
        "Second page should contain remaining 71 entries (170-100)"
    );

    assert_eq!(
        second_page.first().unwrap().timestamp,
        170,
        "Second page should start at timestamp 170"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        100,
        "Second page should end at timestamp 100"
    );

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // 151 entries - everything from the anchor backward, nothing
    // after it.
    assert_eq!(
        all_entry_ids.len(),
        151,
        "Should have retrieved 151 unique entries from timestamp 250 backwards to 100"
    );

    let (third_page, _state3) = LogQuery::new(&file_indexes, anchor, Direction::Backward)
        .with_limit(80)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_anchor_timestamp_same_timestamps() {
    let temp_dir = TempDir::new().unwrap();

    // 100 entries per file, all at timestamp 150 - one 200-entry run
    // split across two files, anchored inside it.
    let entries_file1: Vec<TestEntry> = (0..100)
        .map(|i| {
            TestEntry::new(Microseconds(150))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (0..100)
        .map(|i| {
            TestEntry::new(Microseconds(150))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // Anchor 150 equals every entry's timestamp: each file's walk
    // starts at its run edge (`journal-index/src/file_index.rs`
    // `FileIndex::find_log_entries`),
    // so progress rests entirely on resume positions and the merge.
    let anchor = Anchor::Timestamp(Microseconds(150));

    // First page (limit 80): one file's first 80 entries; the other's
    // 80-entry fetch is displaced whole by the cap.
    let (first_page, state1) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        80,
        "First page should contain exactly 80 entries"
    );

    for entry in &first_page {
        assert_eq!(
            entry.timestamp, 150,
            "All entries should have timestamp 150"
        );
    }

    // Second page: the displaced file is re-walked from its start -
    // no position was recorded for it - and shares the page with the
    // first file's last 20.
    let (second_page, state2) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        80,
        "Second page should contain 80 entries"
    );

    for entry in &second_page {
        assert_eq!(
            entry.timestamp, 150,
            "All entries should have timestamp 150"
        );
    }

    // Third page: the displaced file's remaining 40.
    let (third_page, state3) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        40,
        "Third page should contain remaining 40 entries"
    );

    for entry in &third_page {
        assert_eq!(
            entry.timestamp, 150,
            "All entries should have timestamp 150"
        );
    }

    // ENTRY_IDs must be unique across pages: the re-walk must not
    // duplicate anything.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page
        .iter()
        .chain(second_page.iter())
        .chain(third_page.iter())
    {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // All 200 entries, each exactly once.
    assert_eq!(
        all_entry_ids.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    let (fourth_page, _state4) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state3))
        .unwrap();

    assert_eq!(
        fourth_page.len(),
        0,
        "Fourth page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_forward_with_time_boundaries() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (300..400)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2, index3];

    // Window [150, 350) across three files: file1 contributes 150-199
    // (50), file2 200-299 (100), file3 300-349 (50) - 200 entries.
    // after is inclusive, before exclusive
    // (`LogQuery::with_after_usec`/`with_before_usec`;
    // the boundary checks in `journal-index/src/file_index.rs`
    // `FileIndex::find_log_entries`).

    // First page (limit 80): 150-229.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_after_usec(150)
        .with_before_usec(350)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(first_page.len(), 80, "First page should contain 80 entries");

    assert_eq!(
        first_page.first().unwrap().timestamp,
        150,
        "First entry should be at timestamp 150"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        229,
        "Last entry should be at timestamp 229"
    );

    // Every entry inside [150, 350).
    for entry in &first_page {
        assert!(
            entry.timestamp >= 150 && entry.timestamp < 350,
            "Entry timestamp {} should be within [150, 350)",
            entry.timestamp
        );
    }

    // Second page (limit 80): 230-309.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_after_usec(150)
        .with_before_usec(350)
        .with_limit(80)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        80,
        "Second page should contain 80 entries"
    );

    assert_eq!(
        second_page.first().unwrap().timestamp,
        230,
        "Second page should start at timestamp 230"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        309,
        "Second page should end at timestamp 309"
    );

    // Every entry inside [150, 350).
    for entry in &second_page {
        assert!(
            entry.timestamp >= 150 && entry.timestamp < 350,
            "Entry timestamp {} should be within [150, 350)",
            entry.timestamp
        );
    }

    // Third page: the remaining 40 (310-349).
    let (third_page, state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_after_usec(150)
        .with_before_usec(350)
        .with_limit(80)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        40,
        "Third page should contain remaining 40 entries"
    );

    assert_eq!(
        third_page.first().unwrap().timestamp,
        310,
        "Third page should start at timestamp 310"
    );

    assert_eq!(
        third_page.last().unwrap().timestamp,
        349,
        "Third page should end at timestamp 349 (before boundary)"
    );

    // Every entry inside [150, 350).
    for entry in &third_page {
        assert!(
            entry.timestamp >= 150 && entry.timestamp < 350,
            "Entry timestamp {} should be within [150, 350)",
            entry.timestamp
        );
    }

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page
        .iter()
        .chain(second_page.iter())
        .chain(third_page.iter())
    {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // Exactly the 200 window entries.
    assert_eq!(
        all_entry_ids.len(),
        200,
        "Should have retrieved exactly 200 entries within time boundaries"
    );

    let (fourth_page, _state4) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_after_usec(150)
        .with_before_usec(350)
        .with_limit(80)
        .execute_page(Some(&state3))
        .unwrap();

    assert_eq!(
        fourth_page.len(),
        0,
        "Fourth page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_backward_overlapping_timestamps() {
    let temp_dir = TempDir::new().unwrap();

    // Overlapping files: file1 holds 100-199, file2 150-249 -
    // timestamps 150-199 exist in both.
    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry at {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (150..250)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry at {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2];

    // First page (limit 120) backward from the tail: file2's 50
    // exclusive entries (249-200), then the overlap walked backward,
    // interleaved - the cap falls mid-overlap at 165.
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(120)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        120,
        "First page should contain exactly 120 entries"
    );

    // The merged page is globally descending across both files.
    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp >= first_page[i].timestamp,
            "Entries should be in descending timestamp order"
        );
    }

    assert_eq!(
        first_page.first().unwrap().timestamp,
        249,
        "First entry should be at timestamp 249"
    );

    // Both files contributed, so the state starts tracking both.
    assert!(
        !state1.file_positions.is_empty(),
        "State should track positions"
    );

    // Second page: both files resume mid-run and finish - file1
    // through 100, file2 through 150.
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(120)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        80,
        "Second page should contain remaining 80 entries"
    );

    for i in 1..second_page.len() {
        assert!(
            second_page[i - 1].timestamp >= second_page[i].timestamp,
            "Entries should be in descending timestamp order"
        );
    }

    // No gap: page 2 starts exactly where page 1 stopped.
    if !first_page.is_empty() && !second_page.is_empty() {
        assert!(
            first_page.last().unwrap().timestamp >= second_page.first().unwrap().timestamp,
            "Second page should continue from first page timestamp"
        );
    }

    assert_eq!(
        second_page.last().unwrap().timestamp,
        100,
        "Last entry should be at timestamp 100"
    );

    // ENTRY_IDs must be unique across pages: all 200 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in &first_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    for entry in &second_page {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        200,
        "Should have retrieved all 200 unique entries"
    );

    // 100 from each file - the overlap neither duplicated nor dropped
    // either file's entries.
    let file1_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_entries: usize = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();

    assert_eq!(file1_entries, 100, "Should have 100 entries from file1");
    assert_eq!(file2_entries, 100, "Should have 100 entries from file2");

    let mut all_timestamps = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        all_timestamps.insert(entry.timestamp);
    }

    // Every timestamp 100-249 is represented - shared timestamps
    // yield both files' entries (200 entries over 150 timestamps).
    for ts in 100..250 {
        assert!(all_timestamps.contains(&ts), "Missing timestamp: {}", ts);
    }

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(120)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_backward_three_files() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
                .with_field("FILE", "1")
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
                .with_field("FILE", "2")
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (300..400)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
                .with_field("FILE", "3")
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let file_field = FieldName::new("FILE").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[file_field.clone(), entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[file_field, entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2, index3];

    // First page: limit=125, backward from tail, should get all 100 from file3 + 25 from file2
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(125)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        125,
        "First page should contain exactly 125 entries"
    );

    // The merged page is globally descending across all three files.
    for i in 1..first_page.len() {
        assert!(
            first_page[i - 1].timestamp >= first_page[i].timestamp,
            "Entries should be in descending order"
        );
    }

    assert_eq!(first_page.first().unwrap().timestamp, 399);
    assert_eq!(first_page.last().unwrap().timestamp, 275);

    // State: file3 done, file2 mid-run; file1 untouched, so only 2
    // positions.
    assert_eq!(
        state1.file_positions.len(),
        2,
        "State should track positions for 2 files"
    );

    // Second page: limit=125, should get 75 from file2 + 50 from file1
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(125)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        125,
        "Second page should contain exactly 125 entries"
    );

    for i in 1..second_page.len() {
        assert!(
            second_page[i - 1].timestamp >= second_page[i].timestamp,
            "Entries should be in descending order"
        );
    }

    assert_eq!(second_page.first().unwrap().timestamp, 274);
    assert_eq!(second_page.last().unwrap().timestamp, 150);

    // State now covers all 3 files - file3's position persisted even
    // though page 2 drew nothing from it.
    assert_eq!(
        state2.file_positions.len(),
        3,
        "State should track positions for all 3 files"
    );

    // Third page: remaining 50 from file1
    let (third_page, state3) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(125)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        50,
        "Third page should contain remaining 50 entries"
    );

    for i in 1..third_page.len() {
        assert!(
            third_page[i - 1].timestamp >= third_page[i].timestamp,
            "Entries should be in descending order"
        );
    }

    assert_eq!(third_page.first().unwrap().timestamp, 149);
    assert_eq!(third_page.last().unwrap().timestamp, 100);

    // ENTRY_IDs must be unique across pages: all 300 entries, each
    // exactly once.
    let mut all_entry_ids = HashSet::new();

    for entry in first_page
        .iter()
        .chain(second_page.iter())
        .chain(third_page.iter())
    {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    assert_eq!(
        all_entry_ids.len(),
        300,
        "Should have retrieved all 300 unique entries"
    );

    // 100 from each file.
    let file1_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();
    let file3_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file3_"))
        .count();

    assert_eq!(file1_count, 100, "Should have 100 entries from file1");
    assert_eq!(file2_count, 100, "Should have 100 entries from file2");
    assert_eq!(file3_count, 100, "Should have 100 entries from file3");

    let (fourth_page, _state4) = LogQuery::new(&file_indexes, Anchor::Tail, Direction::Backward)
        .with_limit(125)
        .execute_page(Some(&state3))
        .unwrap();

    assert_eq!(
        fourth_page.len(),
        0,
        "Fourth page should be empty (no more entries)"
    );
}

#[test]
fn test_multi_file_pagination_with_filter() {
    let temp_dir = TempDir::new().unwrap();

    // Each file holds 100 entries: 50 LEVEL=ERROR followed by 50
    // LEVEL=INFO.
    let mut entries_file1: Vec<TestEntry> = (100..150)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Error {}", i))
                .with_field("ENTRY_ID", format!("file1_error_{}", i))
                .with_field("LEVEL", "ERROR")
        })
        .collect();

    entries_file1.extend((150..200).map(|i| {
        TestEntry::new(Microseconds(i))
            .with_field("MESSAGE", format!("File1 Info {}", i))
            .with_field("ENTRY_ID", format!("file1_info_{}", i))
            .with_field("LEVEL", "INFO")
    }));

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    let mut entries_file2: Vec<TestEntry> = (200..250)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Error {}", i))
                .with_field("ENTRY_ID", format!("file2_error_{}", i))
                .with_field("LEVEL", "ERROR")
        })
        .collect();

    entries_file2.extend((250..300).map(|i| {
        TestEntry::new(Microseconds(i))
            .with_field("MESSAGE", format!("File2 Info {}", i))
            .with_field("ENTRY_ID", format!("file2_info_{}", i))
            .with_field("LEVEL", "INFO")
    }));

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let mut entries_file3: Vec<TestEntry> = (300..350)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Error {}", i))
                .with_field("ENTRY_ID", format!("file3_error_{}", i))
                .with_field("LEVEL", "ERROR")
        })
        .collect();

    entries_file3.extend((350..400).map(|i| {
        TestEntry::new(Microseconds(i))
            .with_field("MESSAGE", format!("File3 Info {}", i))
            .with_field("ENTRY_ID", format!("file3_info_{}", i))
            .with_field("LEVEL", "INFO")
    }));

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();
    let level_field = FieldName::new("LEVEL").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone(), level_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field.clone(), level_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[entry_id_field, level_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2, index3];

    // LEVEL=ERROR matches 50 entries per file - 150 total. Pagination
    // runs over each file's FILTERED entry list
    // (`journal-index/src/file_index.rs` `LogEntryId::position`), so resume positions
    // index the filtered order.
    let filter = Filter::match_field_value_pair(FieldValuePair::parse("LEVEL=ERROR").unwrap());

    // First page: limit=80, should get 50 from file1 + 30 from file2
    let (first_page, state1) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_filter(filter.clone())
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page.len(),
        80,
        "First page should contain 80 ERROR entries"
    );

    // Every entry LEVEL=ERROR - the filter holds page-wide.
    for entry in &first_page {
        let level_values: Vec<_> = entry
            .fields
            .iter()
            .filter(|f| f.field() == "LEVEL")
            .map(|f| f.value())
            .collect();
        assert_eq!(
            level_values,
            vec!["ERROR"],
            "All entries should have LEVEL=ERROR"
        );
    }

    assert_eq!(
        first_page.first().unwrap().timestamp,
        100,
        "First entry should be at timestamp 100"
    );

    assert_eq!(
        first_page.last().unwrap().timestamp,
        229,
        "Last entry should be at timestamp 229"
    );

    // Second page: file2's remaining 20 ERROR entries (230-249) plus
    // all 50 of file3's (300-349).
    let (second_page, state2) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_filter(filter.clone())
        .with_limit(80)
        .execute_page(Some(&state1))
        .unwrap();

    assert_eq!(
        second_page.len(),
        70,
        "Second page should contain remaining 70 ERROR entries"
    );

    // Every entry LEVEL=ERROR.
    for entry in &second_page {
        let level_values: Vec<_> = entry
            .fields
            .iter()
            .filter(|f| f.field() == "LEVEL")
            .map(|f| f.value())
            .collect();
        assert_eq!(
            level_values,
            vec!["ERROR"],
            "All entries should have LEVEL=ERROR"
        );
    }

    assert_eq!(
        second_page.first().unwrap().timestamp,
        230,
        "Second page should start at timestamp 230"
    );

    assert_eq!(
        second_page.last().unwrap().timestamp,
        349,
        "Second page should end at timestamp 349"
    );

    // ENTRY_IDs must never repeat.
    let mut all_entry_ids = HashSet::new();
    for entry in first_page.iter().chain(second_page.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    all_entry_ids.insert(field.value().to_string()),
                    "Found duplicate ENTRY_ID: {}",
                    field.value()
                );
            }
        }
    }

    // Exactly the 150 ERROR entries.
    assert_eq!(
        all_entry_ids.len(),
        150,
        "Should have retrieved exactly 150 ERROR entries"
    );

    // Every ENTRY_ID is an error entry.
    let error_count = all_entry_ids
        .iter()
        .filter(|id| id.contains("_error_"))
        .count();
    assert_eq!(error_count, 150, "All entries should be error entries");

    // 50 ERROR entries from each file.
    let file1_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file1_"))
        .count();
    let file2_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file2_"))
        .count();
    let file3_count = all_entry_ids
        .iter()
        .filter(|id| id.starts_with("file3_"))
        .count();

    assert_eq!(file1_count, 50, "Should have 50 ERROR entries from file1");
    assert_eq!(file2_count, 50, "Should have 50 ERROR entries from file2");
    assert_eq!(file3_count, 50, "Should have 50 ERROR entries from file3");

    let (third_page, _state3) = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
        .with_filter(filter)
        .with_limit(80)
        .execute_page(Some(&state2))
        .unwrap();

    assert_eq!(
        third_page.len(),
        0,
        "Third page should be empty (no more ERROR entries)"
    );
}

#[test]
fn test_multi_file_pagination_anchor_at_file_boundary() {
    let temp_dir = TempDir::new().unwrap();

    let entries_file1: Vec<TestEntry> = (100..200)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File1 Entry {}", i))
                .with_field("ENTRY_ID", format!("file1_{}", i))
        })
        .collect();

    let file1 = create_test_journal(&temp_dir, "file1.journal", entries_file1).unwrap();

    // The files chain exactly: file2 starts where file1 ends (200),
    // file3 where file2 ends (300).
    let entries_file2: Vec<TestEntry> = (200..300)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File2 Entry {}", i))
                .with_field("ENTRY_ID", format!("file2_{}", i))
        })
        .collect();

    let file2 = create_test_journal(&temp_dir, "file2.journal", entries_file2).unwrap();

    let entries_file3: Vec<TestEntry> = (300..400)
        .map(|i| {
            TestEntry::new(Microseconds(i))
                .with_field("MESSAGE", format!("File3 Entry {}", i))
                .with_field("ENTRY_ID", format!("file3_{}", i))
        })
        .collect();

    let file3 = create_test_journal(&temp_dir, "file3.journal", entries_file3).unwrap();

    let mut indexer = FileIndexer::default();
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let entry_id_field = FieldName::new("ENTRY_ID").unwrap();

    let index1 = indexer
        .index(
            &file1,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index2 = indexer
        .index(
            &file2,
            Some(&source_timestamp_field),
            &[entry_id_field.clone()],
            Seconds(3600),
        )
        .unwrap();

    let index3 = indexer
        .index(
            &file3,
            Some(&source_timestamp_field),
            &[entry_id_field],
            Seconds(3600),
        )
        .unwrap();

    let file_indexes = vec![index1, index2, index3];

    // Anchor exactly on the 200 boundary: forward, file1's walk is
    // past its end, so page 1 draws wholly from file2 (200-279) and
    // file3 only joins from 300 on page 2.
    let anchor = Anchor::Timestamp(Microseconds(200));

    let (first_page_fwd, state1_fwd) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page_fwd.len(),
        80,
        "Forward from boundary should return 80 entries"
    );

    assert_eq!(
        first_page_fwd.first().unwrap().timestamp,
        200,
        "Forward should start at timestamp 200"
    );

    assert_eq!(
        first_page_fwd.last().unwrap().timestamp,
        279,
        "Forward should end at timestamp 279"
    );

    let (second_page_fwd, _state2_fwd) = LogQuery::new(&file_indexes, anchor, Direction::Forward)
        .with_limit(80)
        .execute_page(Some(&state1_fwd))
        .unwrap();

    assert_eq!(
        second_page_fwd.len(),
        80,
        "Second forward page should return 80 entries"
    );

    assert_eq!(
        second_page_fwd.first().unwrap().timestamp,
        280,
        "Second page should start at 280"
    );

    assert_eq!(
        second_page_fwd.last().unwrap().timestamp,
        359,
        "Second page should end at 359"
    );

    // Same anchor backward: file2's 200 entry is included (backward is
    // inclusive, `journal-index/src/file_index.rs` `FileIndex::find_log_entries`) and file1
    // continues below it.
    let (first_page_bwd, state1_bwd) = LogQuery::new(&file_indexes, anchor, Direction::Backward)
        .with_limit(80)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        first_page_bwd.len(),
        80,
        "Backward from boundary should return 80 entries"
    );

    assert_eq!(
        first_page_bwd.first().unwrap().timestamp,
        200,
        "Backward should start at timestamp 200 (inclusive)"
    );

    assert_eq!(
        first_page_bwd.last().unwrap().timestamp,
        121,
        "Backward should end at timestamp 121"
    );

    let (second_page_bwd, _state2_bwd) = LogQuery::new(&file_indexes, anchor, Direction::Backward)
        .with_limit(80)
        .execute_page(Some(&state1_bwd))
        .unwrap();

    assert_eq!(
        second_page_bwd.len(),
        21,
        "Second backward page should return remaining 21 entries"
    );

    assert_eq!(
        second_page_bwd.first().unwrap().timestamp,
        120,
        "Second backward page should start at 120"
    );

    assert_eq!(
        second_page_bwd.last().unwrap().timestamp,
        100,
        "Second backward page should end at 100"
    );

    // Anchor on the 300 boundary: forward walks file3 from its start;
    // backward takes file3's 300 entry, then file2's tail.
    let anchor_300 = Anchor::Timestamp(Microseconds(300));

    let (page_fwd_300, _) = LogQuery::new(&file_indexes, anchor_300, Direction::Forward)
        .with_limit(50)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        page_fwd_300.len(),
        50,
        "Forward from 300 should return 50 entries"
    );

    assert_eq!(
        page_fwd_300.first().unwrap().timestamp,
        300,
        "Should start at 300"
    );

    assert_eq!(
        page_fwd_300.last().unwrap().timestamp,
        349,
        "Should end at 349"
    );

    let (page_bwd_300, _) = LogQuery::new(&file_indexes, anchor_300, Direction::Backward)
        .with_limit(50)
        .execute_page(None)
        .unwrap();

    assert_eq!(
        page_bwd_300.len(),
        50,
        "Backward from 300 should return 50 entries"
    );

    assert_eq!(
        page_bwd_300.first().unwrap().timestamp,
        300,
        "Should start at 300"
    );

    assert_eq!(
        page_bwd_300.last().unwrap().timestamp,
        251,
        "Should end at 251"
    );

    // Each direction from 200 must be internally duplicate-free.
    let mut fwd_200_ids = HashSet::new();
    for entry in first_page_fwd.iter().chain(second_page_fwd.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    fwd_200_ids.insert(field.value().to_string()),
                    "Found duplicate in forward from 200: {}",
                    field.value()
                );
            }
        }
    }

    let mut bwd_200_ids = HashSet::new();
    for entry in first_page_bwd.iter().chain(second_page_bwd.iter()) {
        for field in &entry.fields {
            if field.field() == "ENTRY_ID" {
                assert!(
                    bwd_200_ids.insert(field.value().to_string()),
                    "Found duplicate in backward from 200: {}",
                    field.value()
                );
            }
        }
    }

    // Forward from 200: two pages of 80 = 160 entries (200-359)
    assert_eq!(
        fwd_200_ids.len(),
        160,
        "Forward from boundary 200 should return 160 unique entries (2 pages of 80)"
    );

    // Backward from 200: 80 + 21 = 101 entries (100-200, inclusive)
    assert_eq!(
        bwd_200_ids.len(),
        101,
        "Backward from boundary 200 should return 101 unique entries"
    );

    // The boundary entry (200) legitimately appears in BOTH
    // directions - dedup applies within a direction, not across them.
    assert!(
        fwd_200_ids.contains("file2_200"),
        "Forward should include boundary entry 200"
    );
    assert!(
        bwd_200_ids.contains("file2_200"),
        "Backward should include boundary entry 200"
    );
}
