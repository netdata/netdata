//! Integration tests for filter evaluation.
//!
//! These tests create actual journal files, index them, and verify that
//! filter evaluation produces correct results.

use journal_common::Seconds;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_core::repository::File;
use journal_index::{FieldName, FieldValuePair, FileIndexer, Filter, Microseconds};
use std::fs;
use std::path::PathBuf;
use tempfile::TempDir;
use uuid::Uuid;

// Helper constants and functions for creating readable timestamps
const JAN_1_2024_MIDNIGHT: Microseconds = Microseconds(1704067200_000_000);

fn hours(n: u64) -> Microseconds {
    Microseconds(n * 3600_000_000)
}

fn add_time(base: Microseconds, offset: Microseconds) -> Microseconds {
    Microseconds(base.0 + offset.0)
}

/// Test journal entry specification
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

/// Create a test journal file path that conforms to the expected format
fn create_test_journal_path(temp_dir: &TempDir) -> PathBuf {
    // Create a machine ID subdirectory
    let machine_id = Uuid::from_u128(0x12345678_1234_1234_1234_123456789abc);
    let machine_dir = temp_dir.path().join(machine_id.to_string());
    fs::create_dir_all(&machine_dir).expect("create machine dir");

    // Create journal file path in the format: <dir>/<machine_id>/system.journal
    machine_dir.join("system.journal")
}

/// Helper to create a test journal file with specified entries
fn create_test_journal(
    entries: Vec<TestEntry>,
) -> Result<(TempDir, File), Box<dyn std::error::Error>> {
    let temp_dir = TempDir::new()?;
    let journal_path = create_test_journal_path(&temp_dir);

    // Create a File object from the path
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

        // Add _SOURCE_REALTIME_TIMESTAMP first
        entry_data.push(format!("_SOURCE_REALTIME_TIMESTAMP={}", entry.timestamp.0).into_bytes());

        // Add all other fields
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

    // Return TempDir to keep it alive and the File for opening
    Ok((temp_dir, file))
}

#[test]
fn test_filter_field_value_pair_single_match() {
    // Create a journal with known entries
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

    // Create and evaluate filter for PRIORITY=3
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

    // Create filter that matches any PRIORITY field
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

    // Filter: PRIORITY=3 AND _HOSTNAME=server1
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

    // Filter: PRIORITY=3 OR PRIORITY=6
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

    // Create a None filter
    let filter = Filter::none();
    let bitmap = filter.evaluate(&file_index);

    // Should match nothing
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

    // Filter for a field that wasn't indexed
    let filter =
        Filter::match_field_value_pair(FieldValuePair::parse("NONEXISTENT_FIELD=value").unwrap());
    let bitmap = filter.evaluate(&file_index);

    // Should match nothing
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

    // Complex filter: (PRIORITY=3 AND _HOSTNAME=server1) OR SYSLOG_IDENTIFIER=kernel
    let filter = Filter::or(vec![
        Filter::and(vec![
            Filter::match_field_value_pair(FieldValuePair::parse("PRIORITY=3").unwrap()),
            Filter::match_field_value_pair(FieldValuePair::parse("_HOSTNAME=server1").unwrap()),
        ]),
        Filter::match_field_value_pair(FieldValuePair::parse("SYSLOG_IDENTIFIER=kernel").unwrap()),
    ]);

    let bitmap = filter.evaluate(&file_index);

    // Should match:
    // - Entry 0: PRIORITY=3 AND _HOSTNAME=server1
    // - Entry 1: SYSLOG_IDENTIFIER=kernel
    // - Entry 3: PRIORITY=3 AND _HOSTNAME=server1
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

    // Empty AND should produce a None filter
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

    // Empty OR should produce a None filter
    let filter = Filter::or(vec![]);
    let bitmap = filter.evaluate(&file_index);

    assert_eq!(bitmap.len(), 0);
    assert!(filter.is_none());
}

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

    // Verify file reference
    assert_eq!(file_index.file(), &file);

    // Verify time range (stored in seconds, with 1-hour bucket duration)
    // Start time should be rounded down to the nearest bucket
    assert_eq!(file_index.start_time().0, 1704067200); // Exact start
    // End time is start of last bucket + bucket_duration
    assert_eq!(file_index.end_time().0, 1704074400 + 3600); // 2 hours + bucket size

    // Verify file fields (all fields present in the journal)
    let file_fields = file_index.fields();
    assert!(file_fields.contains(&FieldName::new("PRIORITY").unwrap()));
    assert!(file_fields.contains(&FieldName::new("_HOSTNAME").unwrap()));
    assert!(file_fields.contains(&FieldName::new("MESSAGE").unwrap()));
    assert!(file_fields.contains(&FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap()));

    // Verify indexed fields (only fields we asked to index)
    assert!(file_index.is_indexed(&priority_field));
    assert!(file_index.is_indexed(&hostname_field));
    assert!(!file_index.is_indexed(&FieldName::new("MESSAGE").unwrap()));

    // Verify entry count
    assert_eq!(file_index.total_entries(), 3);

    // Verify bitmaps exist for indexed field values
    let bitmaps = file_index.bitmaps();
    assert!(bitmaps.contains_key(&FieldValuePair::parse("PRIORITY=3").unwrap()));
    assert!(bitmaps.contains_key(&FieldValuePair::parse("PRIORITY=6").unwrap()));
    assert!(bitmaps.contains_key(&FieldValuePair::parse("_HOSTNAME=server1").unwrap()));

    // MESSAGE field values should not be indexed
    assert!(!bitmaps.contains_key(&FieldValuePair::parse("MESSAGE=test message").unwrap()));
}

#[test]
fn test_source_timestamp_ordering() {
    // Create entries where source timestamp ordering differs from creation order
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(3))).with_field("PRIORITY", "3"), // +3 hours (created first, but should be ordered last)
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"), // +1 hour (created second, but should be ordered first)
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "7"), // +2 hours (created third, but should be ordered middle)
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();
    let source_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();

    // Index WITH source timestamp field - entries should be ordered by source time
    let file_index = indexer
        .index(&file, Some(&source_field), &[priority_field], Seconds(3600))
        .unwrap();

    // After indexing with source timestamp, entries should be reordered:
    // Index 0: +1 hour (original entry 1) - PRIORITY=6
    // Index 1: +2 hours (original entry 2) - PRIORITY=7
    // Index 2: +3 hours (original entry 0) - PRIORITY=3

    // Verify time range reflects source timestamp order (in seconds)
    let expected_start = (add_time(JAN_1_2024_MIDNIGHT, hours(1)).0 / 1_000_000) as u32; // +1 hour in seconds
    let expected_end = ((add_time(JAN_1_2024_MIDNIGHT, hours(3)).0 / 1_000_000) + 3600) as u32; // +3 hours + bucket duration
    assert_eq!(file_index.start_time().0, expected_start);
    assert_eq!(file_index.end_time().0, expected_end);

    // Verify bitmaps reflect the new ordering
    let bitmaps = file_index.bitmaps();

    // PRIORITY=6 should be at index 0 (entry with source_time=1_000_000)
    let priority_6_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=6").unwrap())
        .unwrap();
    assert_eq!(priority_6_bitmap.len(), 1);
    assert!(priority_6_bitmap.contains(0));

    // PRIORITY=7 should be at index 1 (entry with source_time=2_000_000)
    let priority_7_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=7").unwrap())
        .unwrap();
    assert_eq!(priority_7_bitmap.len(), 1);
    assert!(priority_7_bitmap.contains(1));

    // PRIORITY=3 should be at index 2 (entry with source_time=3_000_000)
    let priority_3_bitmap = bitmaps
        .get(&FieldValuePair::parse("PRIORITY=3").unwrap())
        .unwrap();
    assert_eq!(priority_3_bitmap.len(), 1);
    assert!(priority_3_bitmap.contains(2));
}

#[test]
fn test_indexing_without_source_timestamp() {
    // Create entries without specifying source timestamp field
    // They should be ordered by the journal's realtime timestamp instead
    let entries = vec![
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(0))).with_field("PRIORITY", "3"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(1))).with_field("PRIORITY", "6"),
        TestEntry::new(add_time(JAN_1_2024_MIDNIGHT, hours(2))).with_field("PRIORITY", "7"),
    ];

    let (_temp_dir, file) = create_test_journal(entries).unwrap();

    let mut indexer = FileIndexer::default();

    let priority_field = FieldName::new("PRIORITY").unwrap();

    // Index WITHOUT source timestamp field (None)
    let file_index = indexer
        .index(&file, None, &[priority_field], Seconds(3600))
        .unwrap();

    // Verify entries maintain their natural order
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
