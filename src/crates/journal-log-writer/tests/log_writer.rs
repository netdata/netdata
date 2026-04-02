//! Integration tests for journal log writer
//!
//! Tests cover:
//! - Basic entry writing
//! - File rotation (size-based, count-based)
//! - Retention policies

use journal_common::{Microseconds, load_machine_id, monotonic_now};
use journal_log_writer::{
    Config, EntryTimestamps, Log, LogLifecycleEvent, LogLifecycleObserver, RetentionPolicy,
    RotationPolicy,
};
use journal_registry::Origin;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::{Arc, Mutex, OnceLock};
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

fn journal_file_path(dir: &TempDir) -> PathBuf {
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

    assert_eq!(
        journal_files.len(),
        1,
        "expected exactly one journal file in {:?}",
        journal_dir
    );
    journal_files[0].path()
}

fn journal_file_paths(dir: &TempDir) -> Vec<PathBuf> {
    let machine_id = load_machine_id().unwrap();
    let journal_dir = dir.path().join(machine_id.as_simple().to_string());

    let mut journal_files: Vec<_> = fs::read_dir(&journal_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .and_then(|s| s.to_str())
                .map(|s| s == "journal")
                .unwrap_or(false)
        })
        .map(|e| e.path())
        .collect();
    journal_files.sort();
    journal_files
}

fn read_journal_json(path: &Path) -> Vec<serde_json::Value> {
    if !journalctl_available() {
        eprintln!("journalctl not available; skipping journalctl-backed assertions");
        return Vec::new();
    }

    let output = Command::new("journalctl")
        .arg("--output=json")
        .arg("--file")
        .arg(path)
        .output()
        .expect("failed to run journalctl");
    assert!(output.status.success(), "journalctl should succeed");

    String::from_utf8_lossy(&output.stdout)
        .lines()
        .filter(|line| !line.trim().is_empty())
        .map(|line| serde_json::from_str::<serde_json::Value>(line).unwrap())
        .collect()
}

fn journalctl_available() -> bool {
    static AVAILABLE: OnceLock<bool> = OnceLock::new();
    *AVAILABLE.get_or_init(|| {
        Command::new("journalctl")
            .arg("--version")
            .output()
            .map(|output| output.status.success())
            .unwrap_or(false)
    })
}

#[derive(Default)]
struct RecordingObserver {
    events: Mutex<Vec<LogLifecycleEvent>>,
}

impl LogLifecycleObserver for RecordingObserver {
    fn on_event(&self, event: &LogLifecycleEvent) {
        self.events
            .lock()
            .expect("lock observer events")
            .push(event.clone());
    }
}

fn parse_u64_field(row: &serde_json::Value, key: &str) -> Option<u64> {
    row.get(key)?.as_str()?.parse::<u64>().ok()
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

    if !journalctl_available() {
        eprintln!("journalctl not available; skipping test_boot_id_injection");
        return;
    }

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

    assert_eq!(
        journal_files.len(),
        1,
        "Should have created exactly one journal file"
    );

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

#[test]
fn test_write_without_machine_id_suffix() {
    let dir = TempDir::new().unwrap();
    let target_dir = dir.path().join("flows_raw");
    fs::create_dir_all(&target_dir).unwrap();
    let config = test_config().with_machine_id_suffix(false);
    let mut log = Log::new(&target_dir, config).unwrap();

    let entry = [b"MESSAGE=no machine id suffix" as &[u8], b"PRIORITY=6"];
    log.write_entry(&entry, None).unwrap();
    log.sync().unwrap();

    let root_files: Vec<_> = fs::read_dir(&target_dir)
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
    assert_eq!(
        root_files.len(),
        1,
        "expected one .journal file directly in configured directory"
    );

    let machine_id = load_machine_id().unwrap();
    let machine_id_dir = target_dir.join(machine_id.as_simple().to_string());
    assert!(
        !machine_id_dir.exists(),
        "machine-id subdirectory must not be created when suffix is disabled"
    );
}

#[test]
fn test_entry_realtime_override_is_clamped_monotonic() {
    if !journalctl_available() {
        eprintln!(
            "journalctl not available; skipping test_entry_realtime_override_is_clamped_monotonic"
        );
        return;
    }

    let dir = TempDir::new().unwrap();
    let config = test_config();
    let mut log = Log::new(dir.path(), config).unwrap();

    let first_entry = [b"MESSAGE=first" as &[u8], b"PRIORITY=6"];
    log.write_entry(&first_entry, None).unwrap();

    let second_entry = [b"MESSAGE=second" as &[u8], b"PRIORITY=6"];
    let ts = EntryTimestamps::default().with_entry_realtime_usec(1);
    log.write_entry_with_timestamps(&second_entry, ts).unwrap();
    log.sync().unwrap();

    let rows = read_journal_json(&journal_file_path(&dir));

    let mut first_rt = None;
    let mut second_rt = None;
    for row in rows {
        match row.get("MESSAGE").and_then(|v| v.as_str()) {
            Some("first") => first_rt = parse_u64_field(&row, "__REALTIME_TIMESTAMP"),
            Some("second") => second_rt = parse_u64_field(&row, "__REALTIME_TIMESTAMP"),
            _ => {}
        }
    }

    let first_rt = first_rt.expect("missing first entry realtime timestamp");
    let second_rt = second_rt.expect("missing second entry realtime timestamp");
    assert!(
        second_rt > first_rt,
        "second realtime timestamp must be strictly greater ({} !> {})",
        second_rt,
        first_rt
    );
}

#[test]
fn test_entry_monotonic_override_is_clamped_monotonic() {
    if !journalctl_available() {
        eprintln!(
            "journalctl not available; skipping test_entry_monotonic_override_is_clamped_monotonic"
        );
        return;
    }

    let dir = TempDir::new().unwrap();
    let config = test_config();
    let mut log = Log::new(dir.path(), config).unwrap();

    let first_entry = [b"MESSAGE=mono-first" as &[u8], b"PRIORITY=6"];
    log.write_entry(&first_entry, None).unwrap();

    let second_entry = [b"MESSAGE=mono-second" as &[u8], b"PRIORITY=6"];
    let ts = EntryTimestamps::default().with_entry_monotonic_usec(1);
    log.write_entry_with_timestamps(&second_entry, ts).unwrap();
    log.sync().unwrap();

    let rows = read_journal_json(&journal_file_path(&dir));

    let mut first_mono = None;
    let mut second_mono = None;
    for row in rows {
        match row.get("MESSAGE").and_then(|v| v.as_str()) {
            Some("mono-first") => first_mono = parse_u64_field(&row, "__MONOTONIC_TIMESTAMP"),
            Some("mono-second") => second_mono = parse_u64_field(&row, "__MONOTONIC_TIMESTAMP"),
            _ => {}
        }
    }

    let first_mono = first_mono.expect("missing first entry monotonic timestamp");
    let second_mono = second_mono.expect("missing second entry monotonic timestamp");
    assert!(
        second_mono > first_mono,
        "second monotonic timestamp must be strictly greater ({} !> {})",
        second_mono,
        first_mono
    );
}

#[test]
fn test_source_timestamp_is_preserved_with_entry_override() {
    if !journalctl_available() {
        eprintln!(
            "journalctl not available; skipping test_source_timestamp_is_preserved_with_entry_override"
        );
        return;
    }

    let dir = TempDir::new().unwrap();
    let config = test_config();
    let mut log = Log::new(dir.path(), config).unwrap();

    let source_ts = 123_456_u64;
    let entry = [b"MESSAGE=source-ts" as &[u8], b"PRIORITY=6"];
    let ts = EntryTimestamps::default()
        .with_entry_realtime_usec(1)
        .with_source_realtime_usec(source_ts);
    log.write_entry_with_timestamps(&entry, ts).unwrap();
    log.sync().unwrap();

    let rows = read_journal_json(&journal_file_path(&dir));
    let row = rows
        .iter()
        .find(|row| row.get("MESSAGE").and_then(|v| v.as_str()) == Some("source-ts"))
        .expect("missing source-ts entry");

    let stored_source_ts = parse_u64_field(row, "_SOURCE_REALTIME_TIMESTAMP")
        .expect("missing _SOURCE_REALTIME_TIMESTAMP");
    assert_eq!(stored_source_ts, source_ts);
}

#[test]
fn test_monotonic_override_remains_strict_after_restart() {
    if !journalctl_available() {
        eprintln!(
            "journalctl not available; skipping test_monotonic_override_remains_strict_after_restart"
        );
        return;
    }

    let dir = TempDir::new().unwrap();
    let config = test_config();

    let first_monotonic = 1_000_000_u64;
    {
        let mut log = Log::new(dir.path(), config).unwrap();
        let first = [b"MESSAGE=restart-first" as &[u8], b"PRIORITY=6"];
        let ts = EntryTimestamps::default()
            .with_entry_realtime_usec(first_monotonic)
            .with_entry_monotonic_usec(first_monotonic);
        log.write_entry_with_timestamps(&first, ts).unwrap();
        log.sync().unwrap();
    }

    {
        let mut log = Log::new(dir.path(), test_config()).unwrap();
        let second = [b"MESSAGE=restart-second" as &[u8], b"PRIORITY=6"];
        // Equal monotonic override must still be bumped above the persisted tail value.
        let ts = EntryTimestamps::default()
            .with_entry_realtime_usec(1)
            .with_entry_monotonic_usec(first_monotonic);
        log.write_entry_with_timestamps(&second, ts).unwrap();
        log.sync().unwrap();
    }

    let mut first_seen = None;
    let mut second_seen = None;

    for file in journal_file_paths(&dir) {
        for row in read_journal_json(&file) {
            match row.get("MESSAGE").and_then(|v| v.as_str()) {
                Some("restart-first") => {
                    first_seen = parse_u64_field(&row, "__MONOTONIC_TIMESTAMP");
                }
                Some("restart-second") => {
                    second_seen = parse_u64_field(&row, "__MONOTONIC_TIMESTAMP");
                }
                _ => {}
            }
        }
    }

    let first_seen = first_seen.expect("missing first entry monotonic timestamp");
    let second_seen = second_seen.expect("missing second entry monotonic timestamp");
    assert!(
        second_seen > first_seen,
        "second monotonic timestamp must be strictly greater after restart ({} !> {})",
        second_seen,
        first_seen
    );
}

#[test]
fn test_data_entry_preserves_timestamp_overrides_when_remapping_is_emitted() {
    if !journalctl_available() {
        eprintln!(
            "journalctl not available; skipping test_remapping_entry_respects_timestamp_overrides"
        );
        return;
    }

    let dir = TempDir::new().unwrap();
    let config = test_config();
    let mut log = Log::new(dir.path(), config).unwrap();

    let entry = [
        b"MESSAGE=remap-ts" as &[u8],
        b"PRIORITY=6",
        b"foo.bar=value",
    ];
    let realtime_override = Microseconds::now().get().saturating_add(1_000_000);
    let monotonic_override = monotonic_now()
        .expect("read monotonic clock")
        .get()
        .saturating_add(1_000_000);
    let ts = EntryTimestamps::default()
        .with_entry_realtime_usec(realtime_override)
        .with_entry_monotonic_usec(monotonic_override);
    log.write_entry_with_timestamps(&entry, ts).unwrap();
    log.sync().unwrap();

    let rows = read_journal_json(&journal_file_path(&dir));
    let remap_row = rows
        .iter()
        .find(|row| row.get("ND_REMAPPING").and_then(|v| v.as_str()) == Some("1"))
        .expect("missing remapping row");
    let data_row = rows
        .iter()
        .find(|row| row.get("MESSAGE").and_then(|v| v.as_str()) == Some("remap-ts"))
        .expect("missing data row");

    let remap_rt =
        parse_u64_field(remap_row, "__REALTIME_TIMESTAMP").expect("missing remap realtime");
    let data_rt = parse_u64_field(data_row, "__REALTIME_TIMESTAMP").expect("missing data realtime");
    let remap_mono =
        parse_u64_field(remap_row, "__MONOTONIC_TIMESTAMP").expect("missing remap monotonic");
    let data_mono =
        parse_u64_field(data_row, "__MONOTONIC_TIMESTAMP").expect("missing data monotonic");

    assert_eq!(remap_rt, realtime_override);
    assert_eq!(data_rt, realtime_override.saturating_add(1));
    assert_eq!(remap_mono, monotonic_override);
    assert_eq!(data_mono, monotonic_override.saturating_add(1));
}

#[test]
fn test_lifecycle_observer_reports_rotation_and_retention_deletion() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let config = Config::new(
        Origin {
            machine_id: None,
            namespace: None,
            source: journal_registry::Source::System,
        },
        RotationPolicy::default().with_number_of_entries(1),
        RetentionPolicy::default().with_number_of_journal_files(1),
    );
    let observer = Arc::new(RecordingObserver::default());
    let mut log = Log::new(dir.path(), config)
        .expect("create log")
        .with_lifecycle_observer(observer.clone());

    log.write_entry(&[b"MESSAGE=one"], None)
        .expect("write first entry");
    log.write_entry(&[b"MESSAGE=two"], None)
        .expect("write second entry");
    log.write_entry(&[b"MESSAGE=three"], None)
        .expect("write third entry");

    let events = observer
        .events
        .lock()
        .expect("lock observer events")
        .clone();
    let rotation_count = events
        .iter()
        .filter(|event| matches!(event, LogLifecycleEvent::Rotated { .. }))
        .count();
    let deleted_paths = events
        .iter()
        .find_map(|event| match event {
            LogLifecycleEvent::RetainedDeleted { paths } => Some(paths.clone()),
            _ => None,
        })
        .unwrap_or_default();

    assert_eq!(
        rotation_count, 2,
        "expected two rotations after three writes"
    );
    assert_eq!(deleted_paths.len(), 1, "expected one retained deletion");
    assert!(
        !deleted_paths[0].exists(),
        "retained file should be gone from disk: {}",
        deleted_paths[0].display()
    );
}

#[test]
fn test_lifecycle_observer_skips_missing_retention_deletions() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let config = Config::new(
        Origin {
            machine_id: None,
            namespace: None,
            source: journal_registry::Source::System,
        },
        RotationPolicy::default().with_number_of_entries(1),
        RetentionPolicy::default().with_number_of_journal_files(1),
    );
    let observer = Arc::new(RecordingObserver::default());
    let mut log = Log::new(dir.path(), config)
        .expect("create log")
        .with_lifecycle_observer(observer.clone());

    log.write_entry(&[b"MESSAGE=one"], None)
        .expect("write first entry");
    log.write_entry(&[b"MESSAGE=two"], None)
        .expect("write second entry");

    let archived_path = journal_file_paths(&dir)
        .into_iter()
        .find(|path| path.to_string_lossy().contains('@'))
        .expect("archived path after first rotation");
    fs::remove_file(&archived_path).expect("remove archived file before retention");

    log.write_entry(&[b"MESSAGE=three"], None)
        .expect("write third entry");

    let events = observer.events.lock().expect("lock observer events");
    let retained = events
        .iter()
        .filter_map(|event| match event {
            LogLifecycleEvent::RetainedDeleted { paths } => Some(paths),
            _ => None,
        })
        .flatten()
        .cloned()
        .collect::<Vec<_>>();

    assert!(
        !retained.iter().any(|path| path == &archived_path),
        "missing files must not be reported as successful retention deletions"
    );
}
