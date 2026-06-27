//! `count_wal` reads back a WAL file and tallies frames + log records, with the
//! decoded count matching the frame headers. Frames are written here with the raw
//! `wal::Writer`, keeping this test independent of `ng-ingest`.

use std::sync::Arc;

use file_registry::TimestampNs;
use ng_index::{Metrics, count_wal};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use prost::Message;

/// A batch carrying `n` minimal log records.
fn request(n: usize) -> ExportLogsServiceRequest {
    let log_records = (0..n)
        .map(|i| LogRecord {
            time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
            severity_number: 9,
            ..Default::default()
        })
        .collect();
    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            scope_logs: vec![ScopeLogs {
                log_records,
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

fn wal_file(dir: &std::path::Path) -> std::path::PathBuf {
    std::fs::read_dir(dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a .wal file in the dir")
}

#[test]
fn counts_frames_and_records() {
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    // Default config compresses frames, so this also exercises the decompress path.
    let mut writer = wal::Writer::new(dir.path(), wal::Config::default(), seq, 0).unwrap();

    let counts = [3usize, 5, 2];
    for (i, &n) in counts.iter().enumerate() {
        let data = request(n).encode_to_vec();
        writer
            .write_frame(
                0,
                &[],
                &data,
                n,
                TimestampNs(i as u64 + 1),
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();

    let stats = count_wal(&wal_file(dir.path()), &Metrics::new()).unwrap();
    assert_eq!(stats.frames, 3);
    assert_eq!(stats.records, 10);
    assert_eq!(stats.header_records, 10);
    assert!(stats.consistent());
}

#[test]
fn missing_file_is_an_error() {
    let dir = tempfile::tempdir().unwrap();
    assert!(count_wal(&dir.path().join("nope.wal"), &Metrics::new()).is_err());
}
