//! End-to-end round trip: build OTLP requests, flatten them into a flattened-frame
//! WAL via the shared `ng-flatten` path (the same encoding `ng-ingest` writes at
//! ingest), then build a standard SFST index from that WAL (`build_sfst`) and query
//! the record count back. Exercises flatten + fill_hashes + encode_frame, the `wal`
//! round trip (LZ4 on), the typed-entry interning, and the SFST emit. Independent of
//! `ng-ingest`: frames are written here with the raw `wal::Writer`.

use std::sync::Arc;

use file_registry::TimestampNs;
use ng_flatten::{encode_frame, fill_hashes, flatten_request};
use ng_index::{Metrics, build_sfst};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use sfst::IndexReader;

/// A batch of `n` minimal log records, each with one attribute so flattening
/// yields entries beyond the scalar fields.
fn request(n: usize) -> ExportLogsServiceRequest {
    let log_records = (0..n)
        .map(|i| LogRecord {
            time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
            severity_number: 9,
            attributes: vec![KeyValue {
                key: "k".to_string(),
                value: Some(AnyValue {
                    value: Some(Av::IntValue(i as i64)),
                }),
            }],
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

/// Write each batch in `counts` as one flattened-frame WAL frame into `dir` — the
/// same flatten + hash + bincode encoding `ng-ingest` performs at ingest.
fn write_flattened_wal(dir: &std::path::Path, counts: &[usize]) {
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(dir, wal::Config::default(), seq, 0).unwrap();
    for (i, &n) in counts.iter().enumerate() {
        // Every record sets time_unix_nano, so the fallback is never invoked.
        let mut flattened = flatten_request(&request(n), || 0);
        fill_hashes(&mut flattened);
        let bytes = encode_frame(&flattened).unwrap();
        writer
            .write_frame(
                0,
                &[],
                &bytes,
                n,
                TimestampNs(i as u64 + 1),
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();
}

#[test]
fn flattened_wal_builds_and_roundtrips_an_sfst() {
    // 1. Write a flattened WAL of three batches (10 records total).
    let flat = tempfile::tempdir().unwrap();
    write_flattened_wal(flat.path(), &[3, 5, 2]);

    // 2. Build a standard SFST index from it.
    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("count.sfst");
    let sfst = build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    assert_eq!(sfst.frames, 3);
    assert_eq!(sfst.records, 10);

    // 3. Re-open the SFST and confirm the record count round-trips.
    let bytes = std::fs::read(&out).unwrap();
    let reader = IndexReader::open(&bytes).unwrap();
    assert_eq!(reader.total_logs(), 10);
}

#[test]
fn missing_flattened_wal_is_an_error() {
    // An empty directory has no `.wal` file to read.
    let dir = tempfile::tempdir().unwrap();
    let out = dir.path().join("missing.sfst");
    assert!(build_sfst(dir.path(), &out, &Metrics::new()).is_err());
}
