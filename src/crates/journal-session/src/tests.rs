use super::{Direction, JournalSession};
use journal_file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_registry::repository::File as RegistryFile;
use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use tempfile::TempDir;

type TestResult = Result<(), Box<dyn std::error::Error>>;

#[derive(Clone, Copy)]
struct TestEntry {
    realtime: u64,
    monotonic: u64,
    fields: &'static [&'static str],
}

fn test_uuid(seed: u8) -> [u8; 16] {
    [seed; 16]
}

fn seqnum_id_hex(seed: u8) -> String {
    format!("{seed:02x}").repeat(16)
}

fn archived_journal_path(
    dir: &TempDir,
    seq_seed: u8,
    head_seqnum: u64,
    head_realtime: u64,
) -> PathBuf {
    let journal_dir = dir
        .path()
        .join("11111111-1111-1111-1111-111111111111");
    std::fs::create_dir_all(&journal_dir).expect("create test journal directory");
    journal_dir.join(format!(
        "system@{}-{head_seqnum:016x}-{head_realtime:016x}.journal",
        seqnum_id_hex(seq_seed),
    ))
}

fn write_archived_journal(
    path: &Path,
    option_seed: u8,
    entries: &[TestEntry],
) -> Result<(), journal_file::JournalError> {
    let mut journal_file = JournalFile::create(
        path,
        JournalFileOptions::new(
            test_uuid(option_seed),
            test_uuid(option_seed.wrapping_add(1)),
            test_uuid(option_seed.wrapping_add(2)),
            test_uuid(option_seed.wrapping_add(3)),
        ),
    )?;
    let mut writer = JournalWriter::new(&mut journal_file)?;

    for entry in entries {
        let payloads = entry
            .fields
            .iter()
            .map(|field| field.as_bytes())
            .collect::<Vec<_>>();
        writer.add_entry(
            &mut journal_file,
            &payloads,
            entry.realtime,
            entry.monotonic,
            test_uuid(option_seed.wrapping_add(4)),
        )?;
    }

    Ok(())
}

fn read_payload_map(cursor: &mut super::Cursor) -> Result<BTreeMap<String, String>, super::SessionError> {
    let mut payloads = cursor.payloads()?;
    let mut fields = BTreeMap::new();
    while let Some(payload) = payloads.next()? {
        if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
            let key = String::from_utf8_lossy(&payload[..eq_pos]).into_owned();
            let value = String::from_utf8_lossy(&payload[eq_pos + 1..]).into_owned();
            fields.insert(key, value);
        }
    }
    Ok(fields)
}

#[test]
fn iterates_multiple_files_in_chronological_order() -> TestResult {
    let dir = TempDir::new()?;
    let first_path = archived_journal_path(&dir, 0x11, 1, 1_000_000);
    let second_path = archived_journal_path(&dir, 0x22, 10, 2_000_000);

    write_archived_journal(
        &first_path,
        0x31,
        &[
            TestEntry {
                realtime: 1_000_000,
                monotonic: 100,
                fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 1_100_000,
                monotonic: 110,
                fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;
    write_archived_journal(
        &second_path,
        0x41,
        &[
            TestEntry {
                realtime: 2_000_000,
                monotonic: 200,
                fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 2_100_000,
                monotonic: 210,
                fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;

    assert!(
        RegistryFile::from_path(&first_path).is_some(),
        "failed to parse first journal path: {}",
        first_path.display()
    );
    assert!(
        RegistryFile::from_path(&second_path).is_some(),
        "failed to parse second journal path: {}",
        second_path.display()
    );

    let session = JournalSession::builder()
        .files(vec![second_path.clone(), first_path.clone()])
        .load_remappings(false)
        .build()?;

    let mut cursor = session.cursor(Direction::Forward)?;
    let mut seen = Vec::new();
    let mut messages = Vec::new();
    while cursor.step()? {
        seen.push(cursor.realtime_usec());
        let fields = read_payload_map(&mut cursor)?;
        messages.push(fields.get("MESSAGE").cloned().unwrap_or_default());
    }

    assert_eq!(seen, vec![1_000_000, 1_100_000, 2_000_000, 2_100_000]);
    assert_eq!(messages, vec!["first-a", "first-b", "second-a", "second-b"]);
    Ok(())
}

#[test]
fn applies_time_bounds_across_file_transitions() -> TestResult {
    let dir = TempDir::new()?;
    let first_path = archived_journal_path(&dir, 0x51, 1, 1_000_000);
    let second_path = archived_journal_path(&dir, 0x61, 10, 2_000_000);

    write_archived_journal(
        &first_path,
        0x71,
        &[
            TestEntry {
                realtime: 1_000_000,
                monotonic: 100,
                fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 1_100_000,
                monotonic: 110,
                fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;
    write_archived_journal(
        &second_path,
        0x81,
        &[
            TestEntry {
                realtime: 2_000_000,
                monotonic: 200,
                fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 2_100_000,
                monotonic: 210,
                fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;

    assert!(RegistryFile::from_path(&first_path).is_some());
    assert!(RegistryFile::from_path(&second_path).is_some());

    let session = JournalSession::builder()
        .files(vec![second_path, first_path])
        .load_remappings(false)
        .build()?;

    let mut cursor = session
        .cursor_builder()
        .direction(Direction::Forward)
        .since(1_050_000)
        .until(2_050_000)
        .build()?;

    let mut seen = Vec::new();
    while cursor.step()? {
        seen.push(cursor.realtime_usec());
    }

    assert_eq!(seen, vec![1_100_000, 2_000_000]);
    Ok(())
}

#[test]
fn replays_exact_match_filters_for_each_file() -> TestResult {
    let dir = TempDir::new()?;
    let first_path = archived_journal_path(&dir, 0x91, 1, 1_000_000);
    let second_path = archived_journal_path(&dir, 0xa1, 10, 2_000_000);

    write_archived_journal(
        &first_path,
        0xb1,
        &[
            TestEntry {
                realtime: 1_000_000,
                monotonic: 100,
                fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 1_100_000,
                monotonic: 110,
                fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;
    write_archived_journal(
        &second_path,
        0xc1,
        &[
            TestEntry {
                realtime: 2_000_000,
                monotonic: 200,
                fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
            },
            TestEntry {
                realtime: 2_100_000,
                monotonic: 210,
                fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
            },
        ],
    )?;

    assert!(RegistryFile::from_path(&first_path).is_some());
    assert!(RegistryFile::from_path(&second_path).is_some());

    let session = JournalSession::builder()
        .files(vec![second_path, first_path])
        .load_remappings(false)
        .build()?;

    let mut cursor = session
        .cursor_builder()
        .direction(Direction::Forward)
        .add_match(b"_SYSTEMD_UNIT=test.service")
        .build()?;

    let mut messages = Vec::new();
    while cursor.step()? {
        let fields = read_payload_map(&mut cursor)?;
        messages.push(fields.get("MESSAGE").cloned().unwrap_or_default());
    }

    assert_eq!(messages, vec!["first-a", "second-a"]);
    Ok(())
}

#[test]
fn non_matching_exact_filter_returns_empty_results() -> TestResult {
    let dir = TempDir::new()?;
    let first_path = archived_journal_path(&dir, 0xd1, 1, 1_000_000);
    let second_path = archived_journal_path(&dir, 0xe1, 10, 2_000_000);

    write_archived_journal(
        &first_path,
        0xf1,
        &[
            TestEntry {
                realtime: 1_000_000,
                monotonic: 100,
                fields: &["MESSAGE=first-a", "FLOW_VERSION=9"],
            },
            TestEntry {
                realtime: 1_100_000,
                monotonic: 110,
                fields: &["MESSAGE=first-b", "FLOW_VERSION=9"],
            },
        ],
    )?;
    write_archived_journal(
        &second_path,
        0x12,
        &[
            TestEntry {
                realtime: 2_000_000,
                monotonic: 200,
                fields: &["MESSAGE=second-a", "FLOW_VERSION=10"],
            },
            TestEntry {
                realtime: 2_100_000,
                monotonic: 210,
                fields: &["MESSAGE=second-b", "FLOW_VERSION=10"],
            },
        ],
    )?;

    let session = JournalSession::builder()
        .files(vec![second_path, first_path])
        .load_remappings(false)
        .build()?;

    let mut cursor = session
        .cursor_builder()
        .direction(Direction::Forward)
        .add_match(b"FLOW_VERSION=999")
        .build()?;

    assert!(!cursor.step()?);
    Ok(())
}
