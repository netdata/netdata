//! End-to-end round trip: write a protobuf WAL, convert it to a flattened WAL
//! (`convert_wal`), then build a standard SFST index from that (`build_sfst`) and
//! query the record count back. Exercises the bincode (de)serialization, the `wal`
//! round trip (LZ4 on), the typed-entry interning, and the SFST emit. Frames are
//! written here with the raw `wal::Writer`, keeping this independent of `ng-ingest`.

use std::sync::Arc;

use file_registry::TimestampNs;
use ng_index::{Metrics, build_sfst, convert_wal};
use sfst::IndexReader;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use prost::Message;

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

fn sole_wal(dir: &std::path::Path) -> std::path::PathBuf {
    std::fs::read_dir(dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a .wal file in the dir")
}

#[test]
fn convert_then_build_sfst_roundtrips() {
    // 1. Write a protobuf WAL of three batches (default config compresses frames).
    let src = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(src.path(), wal::Config::default(), seq, 0).unwrap();
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

    // 2. Convert to a flattened WAL.
    let flat = tempfile::tempdir().unwrap();
    let convert = convert_wal(&sole_wal(src.path()), flat.path(), &Metrics::new()).unwrap();
    assert_eq!(convert.frames, 3);
    assert_eq!(convert.records, 10);
    assert_eq!(convert.header_records, 10);
    assert!(convert.consistent());
    // Each record flattens to 3 leaves: time_unix_nano, severity_number, attributes.k.
    assert_eq!(convert.leaves, 30);

    // 3. Build a standard SFST index from the flattened WAL.
    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("count.sfst");
    let sfst = build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    assert_eq!(sfst.frames, 3);
    assert_eq!(sfst.records, 10);

    // 4. Re-open the SFST and confirm the record count round-trips.
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
