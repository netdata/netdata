//! End-to-end round trip: build OTLP requests, flatten them into a flattened-frame
//! WAL via the shared `ng-flatten` path (the same encoding `ng-ingest` writes at
//! ingest), then build a standard SFST index from that WAL (`build_sfst`) and query
//! the record count back. Exercises flatten (emit-time hashes) + encode_log_frame, the `wal`
//! round trip (LZ4 on), the typed-entry interning, and the SFST emit. Independent of
//! `ng-ingest`: frames are written here with the raw `wal::Writer`.

use std::sync::Arc;

use file_registry::TimestampNs;
use ng_flatten::{encode_log_frame, flatten_log_request};
use ng_index::{Metrics, build_sfst, build_sfst_range};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use sfst::{IndexReader, SpanId, TraceId};

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
    let mut writer = wal::Writer::new(
        dir,
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    for (i, &n) in counts.iter().enumerate() {
        let (flattened, _) = flatten_log_request(request(n));
        let bytes = encode_log_frame(&flattened).unwrap();
        writer
            .write_frame(
                0,
                &[],
                &bytes,
                wal::FrameMeta {
                    entry_count: n,
                    ingestion_ns: TimestampNs(i as u64 + 1),
                    log_ts_range: None,
                },
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();
}

#[test]
fn wrong_payload_format_refuses_to_build() {
    // A WAL stamped with the traces frame codec must be refused by the LOGS
    // build before any bincode decode — the format check, not a decode error.
    let flat = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        flat.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    let (flattened, _) = flatten_log_request(request(1));
    let bytes = encode_log_frame(&flattened).unwrap();
    writer
        .write_frame(
            0,
            &[],
            &bytes,
            wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();

    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("mismatch.sfst");
    match build_sfst(flat.path(), &out, &Metrics::new()) {
        Err(ng_index::Error::PayloadFormat { found, expected }) => {
            assert_eq!(found, ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT);
            assert_eq!(expected, ng_flatten::LOG_FRAME_PAYLOAD_FORMAT);
        }
        other => panic!("expected PayloadFormat rejection, got {other:?}"),
    }
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
    let mut writer = wal::Writer::new(
        flat.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    let (flattened, _) = flatten_log_request(request_cols(N));
    let bytes = encode_log_frame(&flattened).unwrap();
    writer
        .write_frame(
            0,
            &[],
            &bytes,
            wal::FrameMeta {
                entry_count: N,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();

    // Build the SFST and re-open it through the format reader (COLS access).
    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("cols.sfst");
    build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    let data = std::fs::read(&out).unwrap();
    let reader = IndexReader::open(&data).unwrap();

    // The file carries the per-row column chunks; each is decoded independently.
    assert!(reader.has_per_row_columns());
    assert_eq!(
        reader.columns_table().names().collect::<Vec<_>>(),
        [
            "observed_ts",
            "trace_id",
            "span_id",
            "flags",
            "dropped_attributes_count"
        ],
    );
    let ts = reader.load_timestamps().unwrap();
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
    // Indexes six parallel columns by chronological position and derives the
    // source insertion index `i = N-1-p` — a range loop is the clearest form.
    #[allow(clippy::needless_range_loop)]
    for p in 0..N {
        assert_eq!(
            ts.as_slice()[p],
            (BASE + (p + 1) as u64) as i64,
            "chronological ts at {p}"
        );
        let i = N - 1 - p;
        assert_eq!(
            observed.0[p],
            (BASE + 1000 + i as u64) as i64,
            "observed at {p}"
        );
        assert_eq!(
            trace.get(p),
            TraceId::from([i as u8; 16]),
            "trace_id at {p}"
        );
        assert_eq!(span.get(p), SpanId::from([i as u8; 8]), "span_id at {p}");
        assert_eq!(flags.0[p], 0x100 | i as u32, "flags at {p}");
        assert_eq!(drac.0[p], i as u32, "dropped_attributes_count at {p}");
    }
}

/// Four records whose `poly` attribute alternates `Int`/`Str` (a polymorphic
/// path) while `n` is always `Int` — exercises the typed tree + D45–D47
/// coalescing end to end.
fn request_typed() -> ExportLogsServiceRequest {
    let log_records = (0..4)
        .map(|i| {
            let poly = if i % 2 == 0 {
                Av::IntValue(i)
            } else {
                Av::StringValue(format!("v{i}"))
            };
            LogRecord {
                time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
                severity_number: 9,
                attributes: vec![
                    KeyValue {
                        key: "poly".to_string(),
                        value: Some(AnyValue { value: Some(poly) }),
                    },
                    KeyValue {
                        key: "n".to_string(),
                        value: Some(AnyValue {
                            value: Some(Av::IntValue(i)),
                        }),
                    },
                ],
                ..Default::default()
            }
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
fn typed_tree_and_coalesced_kinds_roundtrip() {
    // One frame of typed records.
    let flat = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        flat.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    let (flattened, _) = flatten_log_request(request_typed());
    let bytes = encode_log_frame(&flattened).unwrap();
    writer
        .write_frame(
            0,
            &[],
            &bytes,
            wal::FrameMeta {
                entry_count: 4,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();

    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("typed.sfst");
    build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    let data = std::fs::read(&out).unwrap();
    let reader = IndexReader::open(&data).unwrap();

    // The typed schema tree is persisted (structure beyond the bare root).
    let tree = reader.tree();
    assert!(tree.len() > 1, "tree should carry leaf nodes");

    // Coalesced scalar kinds (D45–D47): the polymorphic Int+Str path → Str; the
    // always-Int path → Int.
    let scalars: std::collections::HashMap<String, sfst::ValueKind> =
        tree.derive_scalar_kinds().into_iter().collect();
    assert_eq!(scalars.get("attributes.poly"), Some(&sfst::ValueKind::Str));
    assert_eq!(scalars.get("attributes.n"), Some(&sfst::ValueKind::Int));

    // Derived field table: the polymorphic path collapses to a single entry.
    let names: Vec<&str> = reader.field_table().names().collect();
    assert!(names.contains(&"attributes.poly"));
    assert!(names.contains(&"attributes.n"));
    assert_eq!(
        names.iter().filter(|n| **n == "attributes.poly").count(),
        1,
        "polymorphic path must appear once in the derived field table"
    );
}

#[test]
fn missing_flattened_wal_is_an_error() {
    // An empty directory has no `.wal` file to read.
    let dir = tempfile::tempdir().unwrap();
    let out = dir.path().join("missing.sfst");
    assert!(build_sfst(dir.path(), &out, &Metrics::new()).is_err());
}

/// Write `num_frames` flattened frames (each `request_typed`, 4 records) to a
/// fresh WAL dir; returns the dir (kept alive) and the `.wal` path.
fn write_multiframe_flat_wal(num_frames: usize) -> (tempfile::TempDir, std::path::PathBuf) {
    let flat = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        flat.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    for i in 0..num_frames {
        let (flattened, _) = flatten_log_request(request_typed());
        let bytes = encode_log_frame(&flattened).unwrap();
        writer
            .write_frame(
                0,
                &[],
                &bytes,
                wal::FrameMeta {
                    entry_count: 4,
                    ingestion_ns: TimestampNs((i + 1) as u64),
                    log_ts_range: None,
                },
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();
    let wal_path = std::fs::read_dir(flat.path())
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .unwrap();
    (flat, wal_path)
}

/// The in-memory whole-file range build (`build_sfst_range`) is byte-identical to
/// the on-disk file build (`build_sfst`) over the same frames — same typed tree +
/// columns, deterministic, only the sink differs. This is the on-query/seal-time
/// contract parity the migration's index-feed swap depends on.
#[test]
fn build_sfst_range_whole_file_matches_file_build() {
    let (dir, wal_path) = write_multiframe_flat_wal(3);
    let file_len = std::fs::metadata(&wal_path).unwrap().len();

    let (mem_summary, mem_bytes) = build_sfst_range(
        &wal_path,
        wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
    )
    .unwrap();

    let out = dir.path().join("file.sfst");
    build_sfst(dir.path(), &out, &Metrics::new()).unwrap();
    let file_bytes = std::fs::read(&out).unwrap();

    assert_eq!(mem_summary.record_count, 12, "3 frames x 4 records");
    assert_eq!(
        mem_bytes, file_bytes,
        "in-memory range build differs from the file build"
    );
    let reader = IndexReader::open(&mem_bytes).unwrap();
    assert_eq!(reader.total_logs(), 12);
}

/// A frame-aligned split partitions the records exactly: the two chunk builds'
/// record counts sum to the whole. Exercises `build_sfst_range` over interior
/// frame ranges (the on-query active-WAL chunk case).
#[test]
fn build_sfst_range_split_partitions_records() {
    let (_dir, wal_path) = write_multiframe_flat_wal(4);
    let file_len = std::fs::metadata(&wal_path).unwrap().len();

    let frames = wal::scan_frame_boundaries(
        &wal_path,
        wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
    )
    .unwrap();
    assert!(frames.len() >= 2, "need multiple frames to split");
    let split = frames[frames.len() / 2 - 1].end_offset;

    let (a, _) = build_sfst_range(
        &wal_path,
        wal::FrameRange::new(wal::HEADER_SIZE as u64, split),
    )
    .unwrap();
    let (b, _) = build_sfst_range(&wal_path, wal::FrameRange::new(split, file_len)).unwrap();
    let (whole, _) = build_sfst_range(
        &wal_path,
        wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
    )
    .unwrap();

    assert_eq!(
        a.record_count + b.record_count,
        whole.record_count,
        "split chunks must partition the records exactly"
    );
    assert_eq!(whole.record_count, 16, "4 frames x 4 records");
}

/// Regression: an OTLP attribute key containing '=' (legal) used to inject a
/// false key=value boundary — the interner derived field `attributes.a` while
/// the schema-tree leaf path was `attributes.a=b`, so the leaf silently
/// dropped from the reader-derived field table (debug builds panicked on the
/// fill_field_stats round-trip assert). Keys are now sanitized ('=' → '_') at
/// flatten time, so the field survives sealing and is queryable.
#[test]
fn attribute_key_containing_eq_is_sanitized_and_queryable() {
    let req = ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            scope_logs: vec![ScopeLogs {
                log_records: vec![LogRecord {
                    time_unix_nano: 1_700_000_000_000_000_000,
                    severity_number: 9,
                    attributes: vec![KeyValue {
                        key: "a=b".to_string(),
                        value: Some(AnyValue {
                            value: Some(Av::StringValue("x".to_string())),
                        }),
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
            ..Default::default()
        }],
    };

    let flat = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        flat.path(),
        wal::Config::default(),
        seq,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        uuid::Uuid::from_u128(1),
        uuid::Uuid::from_u128(2),
    )
    .unwrap();
    let (flattened, sanitized) = flatten_log_request(req);
    assert_eq!(sanitized, 1);
    let bytes = encode_log_frame(&flattened).unwrap();
    writer
        .write_frame(
            0,
            &[],
            &bytes,
            wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();

    let out_dir = tempfile::tempdir().unwrap();
    let out = out_dir.path().join("eq.sfst");
    let sfst = build_sfst(flat.path(), &out, &Metrics::new()).unwrap();
    assert_eq!(sfst.records, 1);

    let bytes = std::fs::read(&out).unwrap();
    let reader = IndexReader::open(&bytes).unwrap();
    let names: Vec<&str> = reader
        .field_table()
        .iter()
        .map(|f| f.name.as_str())
        .collect();
    assert!(
        names.contains(&"attributes.a_b"),
        "sanitized field must survive sealing: {names:?}"
    );
    assert!(
        !names.iter().any(|n| n.contains('=')),
        "no '=' may survive in field names: {names:?}"
    );
}
