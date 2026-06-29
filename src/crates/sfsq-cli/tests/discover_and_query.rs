//! End-to-end tests over real on-disk WAL/SFST fixtures.
//!
//! Fixtures go through the production path: OTLP `ResourceLogs` →
//! `otel_ingestor::arrow_bridge::encode` → `wal::Writer` →
//! `sfst_indexer::index`. Files are FileId-named (the writer names WAL files;
//! a sealed SFST reuses its source WAL's FileId, sharing the sequence — exactly
//! the production relationship the dedup rule relies on), so the CLI's
//! directory-scan discovery picks them up.
//!
//! The strongest assertion here is tail-vs-sealed equivalence: querying a WAL
//! through the row-scanned tail must return the same rows as sealing it into an
//! SFST and querying that — which is the whole correctness premise of the
//! offline WAL path.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use file_registry::{FileId, TimestampNs};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;
use otel_logs_identity::{ServiceStream, compute_ns_hash};
use sfst::Filter;

use sfsq_cli::config::Dirs;
use sfsq_cli::discover::discover;
use sfsq_cli::query::build_query;
use sfsq_cli::run_query;

const BASE_S: u64 = 1_000_000;
const NS: u64 = 1_000_000_000;
/// The single stream a test `ResourceLogs` carries (`service.namespace`/`name`,
/// each empty when absent) — so the WAL file's `ns_hash` matches its data.
/// Mirrors `otel_ingestor::extract_stream` (private there); the
/// filename-hash assertions below would catch any drift between the two.
fn stream_of(rl: &ResourceLogs) -> ServiceStream {
    let mut namespace = "";
    let mut name = "";
    if let Some(res) = &rl.resource {
        for kv in &res.attributes {
            if let Some(AnyValue {
                value: Some(Value::StringValue(s)),
            }) = &kv.value
            {
                match kv.key.as_str() {
                    "service.namespace" => namespace = s,
                    "service.name" => name = s,
                    _ => {}
                }
            }
        }
    }
    ServiceStream::new(namespace, name)
}

fn sv(v: &str) -> AnyValue {
    AnyValue {
        value: Some(Value::StringValue(v.to_string())),
    }
}

fn kv(key: &str, value: &str) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(sv(value)),
    }
}

/// Monotonic-timestamp records so chunked/tail and whole-file total orders
/// coincide and rows compare exactly.
fn records(range: std::ops::Range<u64>) -> Vec<LogRecord> {
    let levels = ["info", "error", "warn"];
    range
        .map(|i| LogRecord {
            time_unix_nano: (BASE_S + i) * NS,
            severity_text: "INFO".to_string(),
            attributes: vec![
                kv("level", levels[(i % 3) as usize]),
                kv("row", &format!("r{i:04}")),
            ],
            ..LogRecord::default()
        })
        .collect()
}

fn batch(recs: Vec<LogRecord>) -> Vec<ResourceLogs> {
    batch_for(recs, "", "harness")
}

/// A batch carrying an explicit `service.namespace`/`service.name`, so the
/// sealed SFST's summary stream is `(namespace, name)`.
fn batch_for(recs: Vec<LogRecord>, namespace: &str, name: &str) -> Vec<ResourceLogs> {
    let mut attrs = vec![kv("service.name", name)];
    if !namespace.is_empty() {
        attrs.push(kv("service.namespace", namespace));
    }
    vec![ResourceLogs {
        resource: Some(Resource {
            attributes: attrs,
            ..Resource::default()
        }),
        scope_logs: vec![ScopeLogs {
            log_records: recs,
            ..ScopeLogs::default()
        }],
        ..ResourceLogs::default()
    }]
}

/// Write `batches` as one real WAL file (one frame per batch) into
/// `wal_tenant_dir`; return its path.
fn write_wal(wal_tenant_dir: &Path, batches: &[Vec<ResourceLogs>]) -> PathBuf {
    write_wal_seq(wal_tenant_dir, batches, 0)
}

/// As [`write_wal`], but with an explicit starting sequence so multiple WAL
/// files (sealed separately) get distinct FileIds. The dir must hold no other
/// `*.wal` at call time (seal + remove the previous one first).
/// Flatten + encode a batch as one ng-flatten WAL frame payload, returning
/// `(bytes, record_count)` — the ng counterpart of the old
/// `otel_ingestor::arrow_bridge::encode`. Fixtures set explicit timestamps, so no
/// timestamp normalization is needed here.
fn encode_ng_frame(batch: Vec<ResourceLogs>) -> (Vec<u8>, usize) {
    let count: usize = batch
        .iter()
        .flat_map(|rl| &rl.scope_logs)
        .map(|sl| sl.log_records.len())
        .sum();
    let request = ExportLogsServiceRequest {
        resource_logs: batch,
    };
    let mut flattened = ng_flatten::flatten_request(&request);
    ng_flatten::fill_hashes(&mut flattened);
    let data = ng_flatten::encode_frame(&flattened).expect("encode flattened frame");
    (data, count)
}

fn write_wal_seq(wal_tenant_dir: &Path, batches: &[Vec<ResourceLogs>], seq_start: u64) -> PathBuf {
    std::fs::create_dir_all(wal_tenant_dir).unwrap();
    let seq = Arc::new(wal::SeqAllocator::ephemeral(seq_start));
    let mut writer =
        wal::Writer::new(wal_tenant_dir, wal::Config::default(), seq, 0).expect("writer");
    for (i, b) in batches.iter().enumerate() {
        let (data, count) = encode_ng_frame(b.clone());
        let ingestion = TimestampNs((BASE_S + 500 + i as u64) * NS);
        let stream = stream_of(&b[0]);
        let part_key = otel_logs_identity::part_key(&stream);
        let content_meta =
            otel_logs_identity::encode_content_meta(&stream).expect("identity encodes");
        writer
            .write_frame(
                part_key,
                &content_meta,
                &data,
                count,
                ingestion,
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .expect("write_frame");
    }
    writer.shutdown_all().expect("shutdown");

    let mut wals: Vec<PathBuf> = std::fs::read_dir(wal_tenant_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|x| x == "wal"))
        .collect();
    assert_eq!(wals.len(), 1, "fixture must land in a single WAL file");
    wals.pop().unwrap()
}

/// Seal `wal_path` into a FileId-named SFST under `sfst_tenant_dir` (same
/// FileId as the WAL → same sequence, mirroring production).
fn seal_to_sfst(wal_path: &Path, sfst_tenant_dir: &Path) -> PathBuf {
    std::fs::create_dir_all(sfst_tenant_dir).unwrap();
    let id = FileId::parse(wal_path).expect("wal fileid");
    let out = sfst_tenant_dir.join(id.to_filename("sfst"));
    ng_index::build_sfst_file(wal_path, &out, &ng_index::Metrics::new()).expect("index");
    out
}

/// Discover + run a "match everything" query, returning (matched, row bodies).
fn query_all(dirs: &Dirs) -> (u64, Vec<sfst::MaterializedRow>) {
    let d = discover(dirs, "default", None, 0..u32::MAX).expect("discover");
    let q = build_query(0..u32::MAX, Filter::new(), None, 10_000);
    let data = run_query(d.sources, q);
    let rows = data.rows.into_iter().map(|(_, r)| r).collect();
    (data.matched, rows)
}

#[test]
fn tail_path_returns_records_and_filters() {
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    write_wal(&dirs.wal.join("default"), &[batch(records(0..90))]);

    let (matched, rows) = query_all(&dirs);
    assert_eq!(matched, 90, "all records should match via the WAL tail");
    assert_eq!(rows.len(), 90);

    // A filter narrows: level cycles info/error/warn over 90 rows → 30 each.
    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    let q = build_query(
        0..u32::MAX,
        Filter::new().select("attributes.level", "error"),
        None,
        10_000,
    );
    assert_eq!(run_query(d.sources, q).matched, 30);
}

#[test]
fn limit_returns_newest_n_and_reports_total_matched() {
    // The CLI's headline default behavior: return the newest `--limit` rows
    // (newest-first) while `matched` reports the full window total.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    write_wal(&dirs.wal.join("default"), &[batch(records(0..30))]);

    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    let data = run_query(d.sources, build_query(0..u32::MAX, Filter::new(), None, 5));

    assert_eq!(data.matched, 30, "matched reports the full window total");
    assert_eq!(data.rows.len(), 5, "returned is capped to the limit");

    // The 5 newest timestamps, newest-first.
    let ts: Vec<i64> = data.rows.iter().map(|(_, r)| r.timestamp_ns).collect();
    let expected: Vec<i64> = (25..30).rev().map(|i| ((BASE_S + i) * NS) as i64).collect();
    assert_eq!(ts, expected);
}

#[test]
fn tail_equals_sealed_sfst() {
    // The core equivalence: row-scanning a WAL tail must equal querying the
    // SFST that WAL seals into.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal_path = write_wal(
        &dirs.wal.join("default"),
        &[batch(records(0..50)), batch(records(50..120))],
    );

    // Via tail (no SFST present yet).
    let (tail_matched, tail_rows) = query_all(&dirs);

    // Seal to SFST, remove the WAL → query the SFST only.
    seal_to_sfst(&wal_path, &dirs.sfst.join("default"));
    std::fs::remove_file(&wal_path).unwrap();
    let (sfst_matched, sfst_rows) = query_all(&dirs);

    assert_eq!(tail_matched, 120);
    assert_eq!(
        tail_matched, sfst_matched,
        "matched diverged tail vs sealed"
    );
    assert_eq!(tail_rows, sfst_rows, "rows diverged tail vs sealed");
}

#[test]
fn wal_stream_filter_matches_absent_namespace() {
    // Regression for the absent==empty stream-identity fix: the ingestor names an
    // absent-namespace WAL file with `compute_ns_hash(None, name)`. The CLI's
    // stream filter (an empty `--namespace` + a `--name`) must derive the same
    // hash via `ServiceStream::ns_hash` (empty→absent) and discover the file —
    // the pre-fix `compute_ns_hash(Some(""), name)` path missed it.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal_dir = dirs.wal.join("default");
    std::fs::create_dir_all(&wal_dir).unwrap();

    // The writer names the file by the stream's ns_hash; for a service.name
    // with no service.namespace that is compute_ns_hash(None, name), exactly as
    // the ingestor.
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(&wal_dir, wal::Config::default(), seq, 0).expect("writer");
    let (data, count) = encode_ng_frame(batch_for(records(0..10), "", "api"));
    let stream = ServiceStream::new("", "api");
    let part_key = otel_logs_identity::part_key(&stream);
    let content_meta = otel_logs_identity::encode_content_meta(&stream).expect("identity encodes");
    writer
        .write_frame(
            part_key,
            &content_meta,
            &data,
            count,
            TimestampNs((BASE_S + 500) * NS),
            TimestampNs::ZERO,
            TimestampNs::ZERO,
        )
        .expect("write_frame");
    writer.shutdown_all().expect("shutdown");

    // The file carries the absent-namespace hash (empty namespace collapses).
    let wal_path = std::fs::read_dir(&wal_dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .unwrap();
    assert_eq!(
        FileId::parse(&wal_path).unwrap().part_key,
        compute_ns_hash(None, Some("api")),
    );

    // An empty-namespace stream filter must match the absent-namespace file.
    let stream = ServiceStream::new("", "api");
    let d = discover(&dirs, "default", Some(&stream), 0..u32::MAX).expect("discover");
    assert_eq!(
        d.sources.len(),
        1,
        "absent-namespace WAL must match an empty-namespace stream filter"
    );

    // A different stream must be filtered out (the filter is real, not a no-op).
    let other = ServiceStream::new("", "worker");
    let d2 = discover(&dirs, "default", Some(&other), 0..u32::MAX).expect("discover");
    assert!(
        d2.sources.is_empty(),
        "a non-matching stream must be filtered out"
    );
}

#[test]
fn dedup_sfst_wins_over_wal_same_seq() {
    // WAL and its sealed SFST share a sequence; discovery must keep the SFST
    // and skip the WAL (no double counting).
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal_path = write_wal(&dirs.wal.join("default"), &[batch(records(0..40))]);
    seal_to_sfst(&wal_path, &dirs.sfst.join("default")); // WAL left in place

    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    assert_eq!(d.sources.len(), 1, "the shared seq must yield one source");
    assert_eq!(d.consulted.len(), 1);
    assert!(
        d.consulted[0].starts_with("sfst"),
        "SFST must win over WAL, got: {}",
        d.consulted[0]
    );

    let (matched, _) = query_all(&dirs);
    assert_eq!(matched, 40, "no double counting across WAL+SFST of one seq");
}

#[test]
fn torn_final_frame_returns_intact_prefix() {
    // A WAL truncated mid last frame must still serve the intact prefix and
    // never error.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    // Two frames: 40 then 60. Truncate into the second frame's payload.
    let wal_path = write_wal(
        &dirs.wal.join("default"),
        &[batch(records(0..40)), batch(records(40..100))],
    );
    let len = std::fs::metadata(&wal_path).unwrap().len();
    let f = std::fs::OpenOptions::new()
        .write(true)
        .open(&wal_path)
        .unwrap();
    f.set_len(len - 8).unwrap(); // chop a few bytes off the last frame
    drop(f);

    let (matched, rows) = query_all(&dirs);
    // Only the first intact frame's 40 records survive; no error/panic.
    assert_eq!(matched, 40, "intact prefix should be the first frame only");
    assert_eq!(rows.len(), 40);
}

#[test]
fn time_window_prunes_sfst() {
    // An SFST whose data is entirely outside the window is not consulted.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal_path = write_wal(&dirs.wal.join("default"), &[batch(records(0..30))]);
    seal_to_sfst(&wal_path, &dirs.sfst.join("default"));
    std::fs::remove_file(&wal_path).unwrap(); // SFST-only so pruning is observable

    // Data is around BASE_S..BASE_S+30 s. A window far in the future prunes it.
    let future = (BASE_S + 10_000) as u32;
    let d = discover(&dirs, "default", None, future..(future + 10)).unwrap();
    assert!(d.sources.is_empty(), "out-of-window SFST must be pruned");

    // A covering window includes it.
    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    assert_eq!(d.sources.len(), 1);
}

#[test]
fn sub_header_wal_is_skipped_not_panicked() {
    // A `.wal` shorter than the header is corrupt; discovery must skip it
    // (not trip FrameRange's debug_assert) and return no sources.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal_path = write_wal(&dirs.wal.join("default"), &[batch(records(0..10))]);
    std::fs::OpenOptions::new()
        .write(true)
        .open(&wal_path)
        .unwrap()
        .set_len(100)
        .unwrap();

    let d = discover(&dirs, "default", None, 0..u32::MAX).expect("discover must not error");
    assert!(
        d.sources.is_empty(),
        "sub-header WAL must be skipped, not read"
    );
}

#[test]
fn reverse_flips_row_order() {
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    write_wal(&dirs.wal.join("default"), &[batch(records(0..30))]);

    // The engine returns the page newest-first; `--reverse` is a CLI-side flip
    // to oldest-first (what main.rs does to data.rows).
    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    let q = build_query(0..u32::MAX, Filter::new(), None, 10_000);
    let newest_first: Vec<i64> = run_query(d.sources, q)
        .rows
        .into_iter()
        .map(|(_, r)| r.timestamp_ns)
        .collect();

    assert_eq!(newest_first.len(), 30);
    assert_eq!(
        newest_first[0],
        ((BASE_S + 29) * NS) as i64,
        "engine returns newest-first"
    );
    assert_eq!(*newest_first.last().unwrap(), (BASE_S * NS) as i64);

    let mut reversed = newest_first.clone();
    reversed.reverse();
    assert_eq!(
        reversed[0],
        (BASE_S * NS) as i64,
        "--reverse presents oldest-first"
    );
}

#[test]
fn stream_filter_selects_matching_sfst() {
    // Two SFSTs, distinct streams. A stream filter must select exactly one.
    let tmp = tempfile::tempdir().unwrap();
    let dirs = Dirs {
        wal: tmp.path().join("wal"),
        sfst: tmp.path().join("sfst"),
    };
    let wal = dirs.wal.join("default");
    let sfst = dirs.sfst.join("default");

    // Distinct seqs → distinct FileIds → two coexisting SFSTs. Seal + remove
    // each WAL before writing the next (write_wal_seq expects an empty dir).
    let a = write_wal_seq(&wal, &[batch_for(records(0..20), "nsA", "svcA")], 0);
    seal_to_sfst(&a, &sfst);
    std::fs::remove_file(&a).unwrap();
    let b = write_wal_seq(&wal, &[batch_for(records(20..50), "nsB", "svcB")], 1);
    seal_to_sfst(&b, &sfst);
    std::fs::remove_file(&b).unwrap();

    let stream_a = ServiceStream::new("nsA", "svcA");
    let d = discover(&dirs, "default", Some(&stream_a), 0..u32::MAX).unwrap();
    assert_eq!(d.sources.len(), 1, "only stream A's SFST should match");
    let q = build_query(0..u32::MAX, Filter::new(), None, 10_000);
    assert_eq!(run_query(d.sources, q).matched, 20);

    // No filter → both streams visible.
    let d = discover(&dirs, "default", None, 0..u32::MAX).unwrap();
    assert_eq!(d.sources.len(), 2);
    let q = build_query(0..u32::MAX, Filter::new(), None, 10_000);
    assert_eq!(run_query(d.sources, q).matched, 50);
}
