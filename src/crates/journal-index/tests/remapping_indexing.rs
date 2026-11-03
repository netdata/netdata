//! Regression test for `FileIndexer::index` on journals carrying the
//! otel-plugin's `ND_REMAPPING=1` bookkeeping records
//! (`journal-core/src/field_map.rs` `REMAPPING_MARKER`) — the entries that
//! store the
//! OTel-to-systemd field-name mapping, and that the indexer must exclude
//! from its histogram, entry list and bitmaps (`src/file_indexer.rs`
//! `collect_remapping_entry_offsets`).
//!
//! The bug pinned here: `collect_remapping_entry_offsets`
//! (`src/file_indexer.rs`)
//! used to hold a `ValueGuard<DataObject>` — which keeps the journal file's
//! window-manager borrow alive — while calling `InlinedCursor::collect_offsets`
//! (`journal-core/src/file/offset_array.rs`). When the marker data object is
//! referenced by more than one entry, `collect_offsets` walks its entry-array
//! chain, re-borrows the window manager, and fails with
//! `JournalError::ValueGuardInUse` ("previous object is still in use",
//! `journal-core/src/error.rs`). That aborted indexing of every otel-plugin
//! journal — the otel-logs viewer came back empty — while regular systemd
//! journals lack the field, so the crate's other tests never exercised the
//! path.
//!
//! Pinned: indexing such a journal succeeds end-to-end — no `ValueGuardInUse`,
//! and a non-empty histogram (the plain entries survive the marker exclusion,
//! `src/file_indexer.rs` `collect_source_field_info`; see
//! `create_remapping_journal` for why that
//! matters). Scope: the `FileIndexer` path only — marker discovery, the
//! exclusion filters, and `load_fields` (`journal-core/src/file/file.rs`),
//! which maps every field to itself here (its identity pass) because the
//! fixture writes no `ND_<md5>` pairs. The reader-side parser
//! `JournalReader::load_remappings` (`journal-core/src/file/reader.rs`) is
//! not touched.
//!
//! Not pinned: the exclusion itself — the marker entries are stamped too, so
//! an indexer that wrongly kept them in the histogram would still pass —
//! marker-only data objects vanishing from bitmaps (`src/file_indexer.rs`
//! `build_entries_index`),
//! OTel-to-systemd name translation, and query evaluation (indexing only).

use journal_common::Seconds;
use journal_core::field_map::REMAPPING_MARKER;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_index::{FieldName, FileIndexer};
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

/// Path of the test journal: `<tmp>/<machine-id>/system.journal` — the
/// registry path grammar `File::from_path` parses
/// (`journal-registry/src/repository/file.rs` `File::from_path`).
fn create_test_journal_path(temp_dir: &TempDir) -> PathBuf {
    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let machine_dir = temp_dir.path().join(machine_id.to_string());
    fs::create_dir_all(&machine_dir).expect("create machine dir");
    machine_dir.join("system.journal")
}

/// Write `num_marker_entries` bookkeeping entries followed by
/// `num_real_entries` plain log entries into a fresh journal; returns the
/// `File` plus the `TempDir` backing it — indexing re-opens the journal by
/// path, so the dir must outlive it.
///
/// Every entry is stamped with `_SOURCE_REALTIME_TIMESTAMP=<microseconds>`
/// and `add_entry` receives the same value as realtime and monotonic
/// (`journal-core/src/file/writer.rs` `add_entry`), so the marker entries are
/// histogram candidates that a correct indexer filters out
/// (`src/file_indexer.rs` `collect_source_field_info`).
///
/// The marker payloads are identical, so the writer dedups them into a
/// single shared `ND_REMAPPING=1` DATA object
/// (`journal-core/src/file/writer.rs` `add_data` dedup); every reference beyond the
/// first lands in that object's entry-array (`link_data_to_entry` first
/// reference inlined, second in the array), so
/// `num_marker_entries >= 2` gives it a non-empty entry-array chain — the
/// shape that made `collect_remapping_entry_offsets` re-borrow the window
/// manager and fail.
///
/// The plain entries carry no marker, so they survive the exclusion into
/// histogram and bitmaps (`MESSAGE` resolves through the identity mapping
/// `load_fields` builds). They keep the histogram
/// non-empty: a correct indexer over marker entries alone would fail with
/// `EmptyHistogramInput` (`src/histogram.rs`
/// `Histogram::from_timestamp_offset_pairs` empty-input check) instead of
/// passing.
fn create_remapping_journal(num_marker_entries: u64, num_real_entries: u64) -> (TempDir, File) {
    let temp_dir = TempDir::new().expect("temp dir");
    let journal_path = create_test_journal_path(&temp_dir);
    let file = File::from_path(&journal_path).expect("File::from_path");

    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let boot_id = Uuid::from_u128(0x11111111_1111_1111_1111_111111111111);
    let seqnum_id = Uuid::from_u128(0x22222222_2222_2222_2222_222222222222);

    let options = JournalFileOptions::new(machine_id, boot_id, seqnum_id);
    let mut journal_file = JournalFile::create(&file, options).expect("create journal");
    let mut writer = JournalWriter::new(&mut journal_file, 1, boot_id).expect("writer");

    let mut timestamp = 1_000_000u64; // microseconds, strictly increasing

    // Bookkeeping entries: timestamp + the shared ND_REMAPPING=1 marker.
    for _ in 0..num_marker_entries {
        timestamp += 1;
        let ts_field = format!("_SOURCE_REALTIME_TIMESTAMP={timestamp}").into_bytes();
        let items: Vec<&[u8]> = vec![ts_field.as_slice(), REMAPPING_MARKER];
        writer
            .add_entry(&mut journal_file, &items, timestamp, timestamp)
            .expect("add marker entry");
    }

    // Plain log entries: no marker payload, so they survive the exclusion.
    for i in 0..num_real_entries {
        timestamp += 1;
        let ts_field = format!("_SOURCE_REALTIME_TIMESTAMP={timestamp}").into_bytes();
        let message = format!("MESSAGE=log entry {i}").into_bytes();
        let items: Vec<&[u8]> = vec![ts_field.as_slice(), message.as_slice()];
        writer
            .add_entry(&mut journal_file, &items, timestamp, timestamp)
            .expect("add real entry");
    }

    (temp_dir, file)
}

/// Indexing a journal whose `ND_REMAPPING=1` object spans multiple entries
/// must not fail with `ValueGuardInUse`.
#[test]
fn index_journal_with_multi_entry_remapping_marker() {
    // Two marker entries already produce an entry-array chain (the first
    // reference is inlined, the second lands in an array object,
    // `journal-core/src/file/writer.rs` `link_data_to_entry`) — the
    // minimal failing shape;
    // three gives margin against array-layout changes.
    let (_temp_dir, file) = create_remapping_journal(3, 5);

    let mut indexer = FileIndexer::default();
    let source = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let message = FieldName::new("MESSAGE").unwrap();

    let result = indexer.index(&file, Some(&source), &[message], Seconds(15));

    assert!(
        result.is_ok(),
        "indexing a journal with a multi-entry ND_REMAPPING marker must succeed, got: {:?}",
        result.err()
    );
}
