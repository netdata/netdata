//! A batch written as one WAL frame reads back and decodes to a `FlattenedRequest`,
//! preserving the record grouping and the typed scalar/nested/array values through
//! the flatten + bincode + WAL round-trip.

use std::sync::Arc;

use file_registry::MonotonicClock;
use ng_flatten::{FlattenedRequest, Leaf, Value, decode_frame};
use ng_ingest::{PIPELINE_ID, count_log_records, one_file_config, write_request};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value,
};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;

fn av(v: any_value::Value) -> AnyValue {
    AnyValue { value: Some(v) }
}

fn kv(key: &str, v: any_value::Value) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(av(v)),
    }
}

/// A record carrying every scalar `AnyValue` kind plus a nested kvlist and array —
/// the shapes that must survive the WAL round-trip untouched.
fn log_record(body: &str, severity: i32) -> LogRecord {
    let nested = any_value::Value::KvlistValue(KeyValueList {
        values: vec![
            kv("inner_str", any_value::Value::StringValue("x".to_string())),
            kv("inner_int", any_value::Value::IntValue(7)),
        ],
    });
    let array = any_value::Value::ArrayValue(ArrayValue {
        values: vec![
            av(any_value::Value::IntValue(1)),
            av(any_value::Value::StringValue("two".to_string())),
        ],
    });
    LogRecord {
        time_unix_nano: 1_700_000_000_000_000_000,
        observed_time_unix_nano: 1_700_000_000_000_000_001,
        severity_number: severity,
        severity_text: "INFO".to_string(),
        body: Some(av(any_value::Value::StringValue(body.to_string()))),
        attributes: vec![
            kv("str", any_value::Value::StringValue("hello".to_string())),
            kv("int", any_value::Value::IntValue(42)),
            kv("double", any_value::Value::DoubleValue(3.5)),
            kv("bool", any_value::Value::BoolValue(true)),
            kv("bytes", any_value::Value::BytesValue(vec![0xde, 0xad, 0xbe, 0xef])),
            kv("nested", nested),
            kv("arr", array),
        ],
        ..Default::default()
    }
}

fn sample_request() -> ExportLogsServiceRequest {
    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![kv(
                    "service.name",
                    any_value::Value::StringValue("svc".to_string()),
                )],
                ..Default::default()
            }),
            scope_logs: vec![ScopeLogs {
                scope: Some(InstrumentationScope {
                    name: "scope".to_string(),
                    version: "1.0".to_string(),
                    ..Default::default()
                }),
                log_records: vec![log_record("first", 9), log_record("second", 17)],
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

/// The .wal file in `dir` (the writer names it; there is exactly one).
fn wal_file(dir: &std::path::Path) -> std::path::PathBuf {
    std::fs::read_dir(dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a .wal file in the output dir")
}

/// All leaves (resource + scope + every record) of a flattened request, resolved to
/// path+value in document order.
fn all_leaves(flattened: &FlattenedRequest) -> Vec<Leaf> {
    let tree = &flattened.tree;
    let mut out = Vec::new();
    for rg in &flattened.resources {
        out.extend(tree.resolve(&rg.resource));
        for sg in &rg.scopes {
            out.extend(tree.resolve(&sg.scope));
            for record in &sg.records {
                out.extend(tree.resolve(&record.entries));
            }
        }
    }
    out
}

/// All values at `path`, in document order.
fn at<'a>(leaves: &'a [Leaf], path: &str) -> Vec<&'a Value> {
    leaves.iter().filter(|l| l.path == path).map(|l| &l.value).collect()
}

#[test]
fn request_roundtrips_through_a_wal_frame() {
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(dir.path(), one_file_config(), seq, PIPELINE_ID).unwrap();
    let mut clock = MonotonicClock::new();

    let original = sample_request();
    let written = write_request(&mut writer, &mut clock, &original).unwrap();
    assert_eq!(written, count_log_records(&original));
    assert_eq!(written, 2);
    writer.shutdown_all().unwrap();

    // Frames are LZ4-compressed; `wal::Reader` decompresses transparently. The
    // payload is now a bincode `FlattenedRequest`, not protobuf.
    let mut reader = wal::Reader::open(&wal_file(dir.path())).unwrap();
    let frame = reader.next_frame().unwrap().expect("one frame written");
    assert_eq!(frame.entry_count as usize, written);

    let flattened = decode_frame(frame.data).expect("payload decodes as a flattened frame");
    // Grouping survives: one resource, one scope, two records.
    assert_eq!(flattened.resources.len(), 1);
    assert_eq!(flattened.resources[0].scopes.len(), 1);
    assert_eq!(flattened.resources[0].scopes[0].records.len(), 2);

    // time/observed are no longer entries — they resolve to the record's timestamp.
    for rec in &flattened.resources[0].scopes[0].records {
        assert_eq!(rec.ts, Some(1_700_000_000_000_000_000));
    }

    // Typed scalar / nested / array values survive the round-trip (two records, so
    // each shared column appears twice; bodies differ per record).
    let leaves = all_leaves(&flattened);
    assert!(at(&leaves, "time_unix_nano").is_empty(), "time is the row ts, not a facet");
    assert!(at(&leaves, "observed_time_unix_nano").is_empty());
    assert_eq!(at(&leaves, "resource.attributes.service.name"), [&Value::Str("svc".into())]);
    assert_eq!(at(&leaves, "scope.name"), [&Value::Str("scope".into())]);
    assert_eq!(at(&leaves, "scope.version"), [&Value::Str("1.0".into())]);
    assert_eq!(at(&leaves, "body"), [&Value::Str("first".into()), &Value::Str("second".into())]);
    assert_eq!(at(&leaves, "attributes.int"), [&Value::Int(42), &Value::Int(42)]);
    assert_eq!(at(&leaves, "attributes.double"), [&Value::Double(3.5), &Value::Double(3.5)]);
    assert_eq!(at(&leaves, "attributes.bool"), [&Value::Bool(true), &Value::Bool(true)]);
    assert_eq!(
        at(&leaves, "attributes.bytes"),
        [&Value::Bytes(vec![0xde, 0xad, 0xbe, 0xef]), &Value::Bytes(vec![0xde, 0xad, 0xbe, 0xef])],
    );
    assert_eq!(at(&leaves, "attributes.nested.inner_int"), [&Value::Int(7), &Value::Int(7)]);
    // Array indices collapse to `[]`: both elements per record, in order (2 records).
    assert_eq!(
        at(&leaves, "attributes.arr[]"),
        [
            &Value::Int(1),
            &Value::Str("two".into()),
            &Value::Int(1),
            &Value::Str("two".into()),
        ],
    );

    // Exactly one frame for one request.
    assert!(reader.next_frame().unwrap().is_none());
}

#[test]
fn empty_request_writes_no_frame() {
    let dir = tempfile::tempdir().unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(dir.path(), one_file_config(), seq, PIPELINE_ID).unwrap();
    let mut clock = MonotonicClock::new();

    // A request whose ResourceLogs carries zero log records writes nothing.
    let empty = ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            scope_logs: vec![ScopeLogs::default()],
            ..Default::default()
        }],
    };
    assert_eq!(write_request(&mut writer, &mut clock, &empty).unwrap(), 0);
    writer.shutdown_all().unwrap();

    // No file is created when no frame was written.
    let wal_files = std::fs::read_dir(dir.path())
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| e.path().extension().is_some_and(|x| x == "wal"))
        .count();
    assert_eq!(wal_files, 0);
}
