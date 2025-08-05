//! Integration tests for journal log writer
//!
//! Tests cover:
//! - Basic entry writing
//! - File rotation (size-based, count-based)
//! - Retention policies

use journal_common::load_machine_id;
use journal_log_writer::{Config, Log, RetentionPolicy, RotationPolicy};
use journal_registry::Origin;
use std::fs;
use tempfile::TempDir;

/// Helper to create a default test config
fn test_config() -> Config {
    let origin = Origin {
        machine_id: None,
        namespace: None,
        source: journal_registry::Source::System,
    };

    Config::new(
        origin,
        RotationPolicy::default(),
        RetentionPolicy::default(),
    )
}

/// Helper to count journal files in a directory
fn count_journal_files(dir: &TempDir) -> usize {
    let machine_id = load_machine_id().unwrap();
    let journal_dir = dir.path().join(machine_id.as_simple().to_string());

    fs::read_dir(&journal_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .and_then(|s| s.to_str())
                .map(|s| s == "journal")
                .unwrap_or(false)
        })
        .count()
}

#[test]
fn test_write_single_entry() {
    let dir = TempDir::new().unwrap();
    let config = test_config();

    let mut log = Log::new(dir.path(), config).unwrap();

    let entry = [b"MESSAGE=Hello, World!" as &[u8], b"PRIORITY=6"];

    log.write_entry(&entry, None).unwrap();
    log.sync().unwrap();

    // Verify file was created
    assert_eq!(count_journal_files(&dir), 1);
}

#[test]
fn test_write_multiple_entries() {
    let dir = TempDir::new().unwrap();
    let config = test_config();

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write 10 entries
    for i in 0..10 {
        let message = format!("MESSAGE=Entry {}", i);
        let entry = [message.as_bytes(), b"PRIORITY=6"];
        log.write_entry(&entry, None).unwrap();
    }

    log.sync().unwrap();

    // Should still be 1 file
    assert_eq!(count_journal_files(&dir), 1);
}

#[test]
fn test_rotation_by_entry_count() {
    let dir = TempDir::new().unwrap();

    // Rotate after 5 entries
    let rotation = RotationPolicy::default().with_number_of_entries(5);
    let config = test_config().with_rotation_policy(rotation);

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write 12 entries (should create 3 files: 5 + 5 + 2)
    for i in 0..12 {
        let message = format!("MESSAGE=Entry {}", i);
        let entry = [message.as_bytes(), b"PRIORITY=6"];
        log.write_entry(&entry, None).unwrap();
    }

    log.sync().unwrap();

    assert_eq!(count_journal_files(&dir), 3);
}

#[test]
fn test_rotation_by_file_size() {
    let dir = TempDir::new().unwrap();

    // Rotate at ~50KB (small for testing)
    let rotation = RotationPolicy::default().with_size_of_journal_file(50 * 1024);
    let config = test_config().with_rotation_policy(rotation);

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write entries with large messages to trigger size-based rotation
    for i in 0..100 {
        let message = format!(
            "MESSAGE=Entry {} with lots of padding: {}",
            i,
            "x".repeat(1000)
        );
        let entry = [message.as_bytes(), b"PRIORITY=6"];
        log.write_entry(&entry, None).unwrap();
    }

    log.sync().unwrap();

    // Should have rotated at least once
    assert!(count_journal_files(&dir) > 1);
}

#[test]
fn test_retention_by_file_count() {
    let dir = TempDir::new().unwrap();

    // Rotate after 3 entries, keep max 2 files
    let rotation = RotationPolicy::default().with_number_of_entries(3);
    let retention = RetentionPolicy::default().with_number_of_journal_files(2);
    let config = test_config()
        .with_rotation_policy(rotation)
        .with_retention_policy(retention);

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write 10 entries (should create 4 files, but keep only 2)
    for i in 0..10 {
        let message = format!("MESSAGE=Entry {}", i);
        let entry = [message.as_bytes(), b"PRIORITY=6"];
        log.write_entry(&entry, None).unwrap();
    }

    log.sync().unwrap();

    // Retention is enforced during rotation, so there might be 1 extra file
    // (the active file + retention limit). Check that we're at or near the limit.
    let file_count = count_journal_files(&dir);
    assert!(
        file_count <= 3,
        "Should have at most 3 files (active + retention limit), got {}",
        file_count
    );
}

#[test]
fn test_retention_by_total_size() {
    let dir = TempDir::new().unwrap();

    // Rotate after 5 entries, keep max 2 files based on actual data size
    // Note: Journal files pre-allocate space (sparse files), but retention
    // is based on actual data written (append_offset), not logical file size
    let rotation = RotationPolicy::default().with_number_of_entries(5);

    // Each small entry is ~50-100 bytes, plus journal overhead (~4KB per file)
    // Set limit to ~12KB to allow 2-3 files before triggering retention
    let retention = RetentionPolicy::default().with_size_of_journal_files(12 * 1024);

    let config = test_config()
        .with_rotation_policy(rotation)
        .with_retention_policy(retention);

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write 20 entries (creates 4 files of 5 entries each)
    for i in 0..20 {
        let message = format!("MESSAGE=Entry {}", i);
        let entry = [message.as_bytes(), b"PRIORITY=6"];
        log.write_entry(&entry, None).unwrap();
    }

    log.sync().unwrap();

    let file_count = count_journal_files(&dir);

    // Should have rotated (4 files), but retention should limit to 3
    // (oldest file deleted when total data size exceeds 12KB limit)
    assert!(
        file_count <= 3,
        "Size-based retention should limit files, got {}",
        file_count
    );
}

#[test]
fn test_empty_entry() {
    let dir = TempDir::new().unwrap();
    let config = test_config();

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write empty entry (should be no-op)
    let entry: [&[u8]; 0] = [];
    log.write_entry(&entry, None).unwrap();

    // Should not create any files (no rotation triggered)
    assert_eq!(count_journal_files(&dir), 0);
}

#[test]
fn test_boot_id_injection() {
    use journal_common::load_boot_id;
    use std::process::Command;

    let dir = TempDir::new().unwrap();
    let config = test_config();

    let mut log = Log::new(dir.path(), config).unwrap();

    // Write a single entry
    let entry = [b"MESSAGE=Test entry" as &[u8], b"PRIORITY=6"];
    log.write_entry(&entry, None).unwrap();
    log.sync().unwrap();

    // Find the created journal file
    let machine_id = load_machine_id().unwrap();
    let journal_dir = dir.path().join(machine_id.as_simple().to_string());
    let journal_files: Vec<_> = fs::read_dir(&journal_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .and_then(|s| s.to_str())
                .map(|s| s == "journal")
                .unwrap_or(false)
        })
        .collect();

    assert_eq!(journal_files.len(), 1, "Should have created exactly one journal file");

    let journal_path = journal_files[0].path();
    let boot_id = load_boot_id().unwrap();
    let expected_boot_id = boot_id.as_simple().to_string();

    // Use journalctl to verify _BOOT_ID field is present
    let output = Command::new("journalctl")
        .arg("--output=json")
        .arg("--file")
        .arg(&journal_path)
        .output()
        .expect("Failed to run journalctl");

    assert!(output.status.success(), "journalctl should succeed");

    let output_str = String::from_utf8_lossy(&output.stdout);

    // Check that the output contains the expected _BOOT_ID field
    let boot_id_field = format!("\"_BOOT_ID\":\"{}\"", expected_boot_id);
    assert!(
        output_str.contains(&boot_id_field),
        "_BOOT_ID field with value {} should be present in journal entry output",
        expected_boot_id
    );
}
