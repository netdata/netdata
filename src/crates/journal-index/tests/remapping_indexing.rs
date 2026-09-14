//! Regression test for indexing journals that contain the otel-plugin's
//! `ND_REMAPPING=1` bookkeeping field.
//!
//! `FileIndexer::collect_remapping_entry_offsets` used to hold a
//! `ValueGuard<DataObject>` (which keeps the journal file's window-manager
//! borrow alive) while calling `InlinedCursor::collect_offsets`. When the
//! `ND_REMAPPING=1` data object is referenced by more than one entry,
//! `collect_offsets` walks the entry-array chain, re-borrows the window
//! manager, and fails with `JournalError::ValueGuardInUse` ("previous object
//! is still in use"). That aborted indexing of every otel-plugin journal.
//!
//! Regular systemd journals do not contain the `ND_REMAPPING` field, so the
//! existing tests never exercised this path. This test reproduces the
//! multi-entry shape that triggered the bug and asserts indexing succeeds.

use journal_common::Seconds;
use journal_core::field_map::REMAPPING_MARKER;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_index::{FieldName, FileIndexer};
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

fn create_test_journal_path(temp_dir: &TempDir) -> PathBuf {
    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let machine_dir = temp_dir.path().join(machine_id.to_string());
    fs::create_dir_all(&machine_dir).expect("create machine dir");
    machine_dir.join("system.journal")
}

/// Build a journal that mirrors an otel-plugin journal: a handful of
/// `ND_REMAPPING=1` bookkeeping entries plus normal log entries.
///
/// The bookkeeping entries all share one `ND_REMAPPING=1` DATA object (journald
/// dedups by payload), so with `num_marker_entries >= 2` that object's
/// entry-array chain is non-empty — the shape that made
/// `collect_remapping_entry_offsets` re-borrow the window manager and fail.
///
/// The normal entries (which carry no marker) are excluded neither from the
/// histogram nor the bitmaps, so a fixed indexer produces a non-empty index
/// instead of erroring with `EmptyHistogramInput`.
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

    // Real log entries: no marker, so they survive into the index.
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
    // Two marker entries already produce an entry-array chain (one inlined
    // offset + one array object), the minimal shape that triggered the bug;
    // use a few more to be robust against array-layout changes.
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
