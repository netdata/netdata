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
use sfst::{IndexReader, Reader};

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
        let mut flattened = flatten_request(&request(n));
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

/// `n` records in reverse-chronological insertion order: record `i` gets the
/// `(n - i)`-th timestamp, so insertion order is the opposite of chronological
/// order — the build-time reorder must permute the columns to match. Each record
/// tags its `observed_ts`/`trace_id`/`span_id` with its insertion index `i` so the
/// permutation is verifiable.
fn request_cols(n: usize) -> ExportLogsServiceRequest {
    const BASE: u64 = 1_700_000_000_000_000_000;
    let log_records = (0..n)
        .map(|i| LogRecord {
            time_unix_nano: BASE + (n - i) as u64, // i=0 latest, i=n-1 earliest
            observed_time_unix_nano: BASE + 1000 + i as u64,
            trace_id: vec![i as u8; 16],
            span_id: vec![i as u8; 8],
            flags: 0x100 | i as u32,
            dropped_attributes_count: i as u32,
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

#[test]
fn per_row_columns_roundtrip_in_chronological_order() {
    const N: usize = 5;
    const BASE: u64 = 1_700_000_000_000_000_000;

    // Build a one-frame flattened WAL of reverse-chronological records.
    let flat = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(flat.path(), wal::Config::default(), seq, 0).unwrap();
    let mut flattened = flatten_request(&request_cols(N));
    fill_hashes(&mut flattened);
    let bytes = encode_frame(&flattened).unwrap();
    writer
        .write_frame(0, &[], &bytes, N, TimestampNs(1), TimestampNs::ZERO, TimestampNs::ZERO)
        .unwrap();
    writer.shutdown_all().unwrap();

    // Build the SFST and re-open it through the format reader (COLS access).
    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("cols.sfst");
    build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    let data = std::fs::read(&out).unwrap();
    let reader = Reader::open(&data).unwrap();

    // The file carries the per-row column chunks; each is decoded independently.
    assert!(reader.has_per_row_columns().unwrap());
    assert_eq!(
        reader.columns_table().unwrap().names().collect::<Vec<_>>(),
        ["observed_ts", "trace_id", "span_id", "flags", "dropped_attributes_count"],
    );
    let ts = reader.timestamps().unwrap();
    let observed = reader.observed_timestamps().unwrap();
    let trace = reader.trace_ids().unwrap();
    let span = reader.span_ids().unwrap();
    let flags = reader.flags().unwrap();
    let drac = reader.dropped_attribute_counts().unwrap();
    assert_eq!(ts.len(), N);
    assert_eq!(observed.len(), N);
    assert_eq!(trace.len(), N);
    assert_eq!(span.len(), N);
    assert_eq!(flags.len(), N);
    assert_eq!(drac.len(), N);

    // Every column is row-aligned with the chronological timestamps. At
    // chronological position `p` the ts is BASE+(p+1) and the source record is
    // insertion index `i = N-1-p`, so its id-tagged columns must read back as `i`.
    for p in 0..N {
        assert_eq!(ts[p], (BASE + (p + 1) as u64) as i64, "chronological ts at {p}");
        let i = N - 1 - p;
        assert_eq!(observed.0[p], (BASE + 1000 + i as u64) as i64, "observed at {p}");
        assert_eq!(trace.get(p), &[i as u8; 16][..], "trace_id at {p}");
        assert_eq!(span.get(p), &[i as u8; 8][..], "span_id at {p}");
        assert_eq!(flags.0[p], 0x100 | i as u32, "flags at {p}");
        assert_eq!(drac.0[p], i as u32, "dropped_attributes_count at {p}");
    }
}

#[test]
fn missing_flattened_wal_is_an_error() {
    // An empty directory has no `.wal` file to read.
    let dir = tempfile::tempdir().unwrap();
    let out = dir.path().join("missing.sfst");
    assert!(build_sfst(dir.path(), &out, &Metrics::new()).is_err());
}
