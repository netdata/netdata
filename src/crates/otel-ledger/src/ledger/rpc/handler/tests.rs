use super::*;
use file_registry::{ByteSize, FileId, ServiceStream, TenantId, TimestampNs};
use fst_index::FstIndex;
use serde_json::Value;
use sfst::BitmapValue;
use std::collections::HashMap;
use tokio_util::sync::CancellationToken;
use uuid::Uuid;

fn make_tenant_registries() -> TenantRegistries {
    TenantRegistries::new(
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
    )
}

fn make_handler(tr: TenantRegistries) -> OtelLogsHandler {
    OtelLogsHandler::new(
        Arc::new(RwLock::new(tr)),
        Arc::new(crate::chunk::ChunkCache::new(64 * 1024 * 1024)),
        16_384,
        None,
    )
}

fn make_ctx(transaction: &str) -> FunctionCallContext {
    FunctionCallContext::new(
        transaction.to_string(),
        bridge::function::ProgressState::new(),
        CancellationToken::new(),
    )
}

fn bitmap_with(positions: &[u32], universe: u32) -> BitmapValue {
    let mut data = Vec::new();
    let desc = treight::Bitmap::from_sorted_iter(positions.iter().copied(), universe, &mut data);
    BitmapValue { desc, data }
}

/// Write a 6-log SFST to `path` with two low-card fields:
///
/// - `severity_text`: `info` at 0/2/4, `error` at 1/3/5
/// - `service`: `api` at 0/1/2, `worker` at 3/4/5
///
/// Timestamps span 6 seconds starting at `min_s`.
fn write_test_sfst(path: &std::path::Path, min_s: u32) {
    let primary_entries: Vec<(&str, BitmapValue)> = vec![
        ("service=api", bitmap_with(&[0, 1, 2], 6)),
        ("service=worker", bitmap_with(&[3, 4, 5], 6)),
        ("severity_text=error", bitmap_with(&[1, 3, 5], 6)),
        ("severity_text=info", bitmap_with(&[0, 2, 4], 6)),
    ];
    let primary: FstIndex<BitmapValue> = FstIndex::build(primary_entries).unwrap();

    let summary = sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: min_s + 5,
        total_logs: 6,
        stream: ServiceStream::new("ns", "svc"),
    };
    let metadata = sfst::Metadata {
        histogram: sfst::Histogram {
            timestamps: vec![min_s],
            counts: vec![6],
        },
        id_ranges: sfst::IdRanges {
            low_end: sfst::KvId(4),
            mid_end: sfst::KvId(4),
            high_end: sfst::KvId(4),
        },
        fields: vec![
            sfst::FieldEntry {
                name: "service".into(),
                cardinality: 2,
                tier: sfst::FieldTier::Low,
            },
            sfst::FieldEntry {
                name: "severity_text".into(),
                cardinality: 2,
                tier: sfst::FieldTier::Low,
            },
        ]
        .into(),
    };
    let timestamps: Vec<i64> = (0..6)
        .map(|i| (min_s as i64) * 1_000_000_000 + i * 1_000_000_000)
        .collect();
    // FST key order is lex: KvId 0=service=api, 1=service=worker,
    // 2=severity_text=error, 3=severity_text=info.
    let stream_entries: Vec<Vec<sfst::KvId>> = vec![
        vec![sfst::KvId(0), sfst::KvId(3)], // pos 0: api, info
        vec![sfst::KvId(0), sfst::KvId(2)], // pos 1: api, error
        vec![sfst::KvId(0), sfst::KvId(3)], // pos 2: api, info
        vec![sfst::KvId(1), sfst::KvId(2)], // pos 3: worker, error
        vec![sfst::KvId(1), sfst::KvId(3)], // pos 4: worker, info
        vec![sfst::KvId(1), sfst::KvId(2)], // pos 5: worker, error
    ];

    let counts = sfst::ChunkCounts {
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = sfst::StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(&primary).unwrap();
    writer
        .add_stream_batch(&sfst::StreamBatch::for_write(&stream_entries))
        .unwrap();
    let buf = writer.finish().unwrap().into_inner();
    std::fs::write(path, &buf).unwrap();
}

/// Install a single SFST file under tenant `t`, returning the
/// machine/boot uuids used so callers can reason about seq.
fn install_sfst(tr: &mut TenantRegistries, tenant: &str, seq: u64, min_s: u32) -> (Uuid, Uuid) {
    let machine = Uuid::from_u128(0x11);
    let boot = Uuid::from_u128(0x22);
    let id = FileId::new(machine, boot, seq, 7);

    // get_or_create initializes the tenant subdir; we then write
    // the file at the registry's computed path and track it.
    let reg = tr.get_or_create(&TenantId::from(tenant));
    let path = reg.sfst.file_path(id);
    write_test_sfst(&path, min_s);
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    let summary = sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: min_s + 5,
        total_logs: 6,
        stream: ServiceStream::new("ns", "svc"),
    };
    reg.sfst.track(id, size, summary);
    (machine, boot)
}

/// Write a 3-log SFST with only a `service` field — no
/// `severity_text`. Used to exercise the histogram default field
/// being absent from a file: `service=api` at 0/1, `service=worker`
/// at 2. Timestamps span 3 seconds starting at `min_s`.
fn write_service_only_sfst(path: &std::path::Path, min_s: u32) {
    let primary_entries: Vec<(&str, BitmapValue)> = vec![
        ("service=api", bitmap_with(&[0, 1], 3)),
        ("service=worker", bitmap_with(&[2], 3)),
    ];
    let primary: FstIndex<BitmapValue> = FstIndex::build(primary_entries).unwrap();

    let summary = sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: min_s + 2,
        total_logs: 3,
        stream: ServiceStream::new("ns", "svc"),
    };
    let metadata = sfst::Metadata {
        histogram: sfst::Histogram {
            timestamps: vec![min_s],
            counts: vec![3],
        },
        id_ranges: sfst::IdRanges {
            low_end: sfst::KvId(2),
            mid_end: sfst::KvId(2),
            high_end: sfst::KvId(2),
        },
        fields: vec![sfst::FieldEntry {
            name: "service".into(),
            cardinality: 2,
            tier: sfst::FieldTier::Low,
        }]
        .into(),
    };
    let timestamps: Vec<i64> = (0..3)
        .map(|i| (min_s as i64) * 1_000_000_000 + i * 1_000_000_000)
        .collect();
    // KvId 0=service=api, 1=service=worker.
    let stream_entries: Vec<Vec<sfst::KvId>> = vec![
        vec![sfst::KvId(0)], // pos 0: api
        vec![sfst::KvId(0)], // pos 1: api
        vec![sfst::KvId(1)], // pos 2: worker
    ];

    let counts = sfst::ChunkCounts {
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = sfst::StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(&primary).unwrap();
    writer
        .add_stream_batch(&sfst::StreamBatch::for_write(&stream_entries))
        .unwrap();
    let buf = writer.finish().unwrap().into_inner();
    std::fs::write(path, &buf).unwrap();
}

/// Install a 3-log service-only SFST (see [`write_service_only_sfst`]).
fn install_service_only_sfst(tr: &mut TenantRegistries, tenant: &str, seq: u64, min_s: u32) {
    let id = FileId::new(Uuid::from_u128(0x11), Uuid::from_u128(0x22), seq, 7);
    let reg = tr.get_or_create(&TenantId::from(tenant));
    let path = reg.sfst.file_path(id);
    write_service_only_sfst(&path, min_s);
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    let summary = sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: min_s + 2,
        total_logs: 3,
        stream: ServiceStream::new("ns", "svc"),
    };
    reg.sfst.track(id, size, summary);
}

/// Write an SFST of `n` logs that all share timestamp `ts_s`, each
/// carrying `severity_text=info`. Used to exercise the same-timestamp
/// pagination tiebreaker — rows are distinguishable only by position.
fn write_same_ts_sfst(path: &std::path::Path, ts_s: u32, n: usize) {
    let positions: Vec<u32> = (0..n as u32).collect();
    let primary_entries: Vec<(&str, BitmapValue)> =
        vec![("severity_text=info", bitmap_with(&positions, n as u32))];
    let primary: FstIndex<BitmapValue> = FstIndex::build(primary_entries).unwrap();

    let summary = sfst::Summary {
        min_timestamp_s: ts_s,
        max_timestamp_s: ts_s,
        total_logs: n as u32,
        stream: ServiceStream::new("ns", "svc"),
    };
    let metadata = sfst::Metadata {
        histogram: sfst::Histogram {
            timestamps: vec![ts_s],
            counts: vec![n as u32],
        },
        id_ranges: sfst::IdRanges {
            low_end: sfst::KvId(1),
            mid_end: sfst::KvId(1),
            high_end: sfst::KvId(1),
        },
        fields: vec![sfst::FieldEntry {
            name: "severity_text".into(),
            cardinality: 1,
            tier: sfst::FieldTier::Low,
        }]
        .into(),
    };
    let timestamps: Vec<i64> = vec![(ts_s as i64) * 1_000_000_000; n];
    let stream_entries: Vec<Vec<sfst::KvId>> = (0..n).map(|_| vec![sfst::KvId(0)]).collect();

    let counts = sfst::ChunkCounts {
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = sfst::StreamWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    writer.summary(&summary).unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&timestamps).unwrap();
    writer.primary(&primary).unwrap();
    writer
        .add_stream_batch(&sfst::StreamBatch::for_write(&stream_entries))
        .unwrap();
    let buf = writer.finish().unwrap().into_inner();
    std::fs::write(path, &buf).unwrap();
}

fn install_same_ts_sfst(tr: &mut TenantRegistries, tenant: &str, seq: u64, ts_s: u32, n: usize) {
    let id = FileId::new(Uuid::from_u128(0x11), Uuid::from_u128(0x22), seq, 7);
    let reg = tr.get_or_create(&TenantId::from(tenant));
    let path = reg.sfst.file_path(id);
    write_same_ts_sfst(&path, ts_s, n);
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    let summary = sfst::Summary {
        min_timestamp_s: ts_s,
        max_timestamp_s: ts_s,
        total_logs: n as u32,
        stream: ServiceStream::new("ns", "svc"),
    };
    reg.sfst.track(id, size, summary);
}

#[tokio::test]
async fn same_timestamp_rows_paginate_without_dup_or_skip() {
    // 5 logs at one identical timestamp — pagination must rely on the
    // cursor's position tiebreaker, not the timestamp.
    let mut tr = make_tenant_registries();
    let ts_s = 1_700_000_000u32;
    install_same_ts_sfst(&mut tr, "tenant-a", 1, ts_s, 5);
    let h = make_handler(tr);
    let win = format!(r#""after": {}, "before": {}"#, ts_s, ts_s + 1);

    let pos_of = |cursor: &str| cursor.rsplit(':').next().unwrap().parse::<u32>().unwrap();

    // Walk backward in pages of 2, collecting every row's position.
    let mut anchor: Option<String> = None;
    let mut seen: Vec<u32> = Vec::new();
    loop {
        let anchor_field = anchor
            .as_ref()
            .map(|a| format!(r#","anchor":"{a}""#))
            .unwrap_or_default();
        let req: OtelLogsRequest = serde_json::from_slice(
            format!(r#"{{"info":false,"tenant":"tenant-a",{win},"last":2,"direction":"backward"{anchor_field}}}"#)
                .as_bytes(),
        )
        .unwrap();
        let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
        let d = v["data"].as_array().unwrap();
        if d.is_empty() {
            break;
        }
        for row in d {
            seen.push(pos_of(&row_ts_cursor(row).1));
        }
        anchor = Some(row_ts_cursor(d.last().unwrap()).1);
        if v["items"]["after"] == 0 {
            break;
        }
    }

    // Every position exactly once, newest-first — no duplicate, no skip,
    // despite all five rows sharing a timestamp.
    assert_eq!(seen, vec![4, 3, 2, 1, 0]);
}

#[tokio::test]
async fn files_request_returns_inventory_with_upload_state() {
    let mut tr = make_tenant_registries();
    install_sfst(&mut tr, "default", 1, 1_700_000_000);
    install_sfst(&mut tr, "default", 2, 1_700_000_100);
    // Mark seq 1's upload lifecycle so the response surfaces the registry's
    // per-seq flags (the reason this goes to the plugin, not the filesystem).
    {
        let reg = tr.get_or_create(&TenantId::from("default"));
        reg.mark_rotated(1);
        reg.mark_uploaded(1);
        reg.mark_remote_cataloged([1]);
        reg.sfst.mark_pending_deletion(2); // seq 2 queued for eviction (still tracked)
    }

    let h = make_handler(tr);
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"files": true}"#).unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();

    assert_eq!(v["status"], 200);
    let tenants = v["tenants"].as_array().unwrap();
    assert_eq!(tenants.len(), 1);
    assert_eq!(tenants[0]["tenant"], "default");
    assert!(tenants[0]["wal"].as_array().unwrap().is_empty());
    assert!(tenants[0]["catalog"].as_array().unwrap().is_empty());

    let sfst = tenants[0]["sfst"].as_array().unwrap();
    assert_eq!(sfst.len(), 2);
    // sorted by seq
    assert_eq!(sfst[0]["seq"], 1);
    assert_eq!(sfst[1]["seq"], 2);
    // summary + stream fields lifted from the SFST
    assert_eq!(sfst[0]["total_logs"], 6);
    assert_eq!(sfst[0]["min_ts_s"], 1_700_000_000u64);
    assert_eq!(sfst[0]["stream"]["namespace"], "ns");
    assert_eq!(sfst[0]["stream"]["name"], "svc");
    assert_eq!(sfst[0]["ns_hash"], "0000000000000007"); // FileId ns_hash = 7
    // seq 1 went through the upload lifecycle; seq 2 did not
    assert_eq!(sfst[0]["rotated"], true);
    assert_eq!(sfst[0]["uploaded"], true);
    assert_eq!(sfst[0]["remote_cataloged"], true);
    assert_eq!(sfst[1]["rotated"], false);
    assert_eq!(sfst[1]["uploaded"], false);
    assert_eq!(sfst[1]["remote_cataloged"], false);
    // pending_deletion surfaced from the sfst registry (seq 2 marked above)
    assert_eq!(sfst[0]["pending_deletion"], false);
    assert_eq!(sfst[1]["pending_deletion"], true);
}

#[tokio::test]
async fn files_request_includes_wal_and_catalog_entries() {
    use chrono::NaiveDate;

    let mut tr = make_tenant_registries();
    let machine = Uuid::from_u128(0xa1);
    let boot = Uuid::from_u128(0xb2);
    let stream = ServiceStream::new("walns", "walsvc");
    {
        let reg = tr.get_or_create(&TenantId::from("default"));
        // An active WAL: Created, then Synced sets entry_count + time range.
        let active = FileId::new(machine, boot, 10, 0xab);
        reg.wal
            .apply_event(&wal::FileEvent::Created {
                file_id: active,
                created_at_ns: TimestampNs(1_000),
                stream: stream.clone(),
            })
            .unwrap();
        reg.wal
            .apply_event(&wal::FileEvent::Synced {
                file_id: active,
                valid_up_to: ByteSize(512),
                frame_count: 1,
                entry_count: 5,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(200),
            })
            .unwrap();
        // An archived WAL: Created, then Closed seals it + sets size.
        let archived = FileId::new(machine, boot, 11, 0xcd);
        reg.wal
            .apply_event(&wal::FileEvent::Created {
                file_id: archived,
                created_at_ns: TimestampNs(2_000),
                stream: stream.clone(),
            })
            .unwrap();
        reg.wal
            .apply_event(&wal::FileEvent::Closed {
                file_id: archived,
                frame_count: 3,
                min_timestamp_ns: TimestampNs(300),
                max_timestamp_ns: TimestampNs(400),
                size: ByteSize(1234),
            })
            .unwrap();
        // A tracked catalog file.
        let cat = otel_catalog::File::new(
            NaiveDate::from_ymd_opt(2026, 6, 19).unwrap(),
            machine,
            boot,
            7,   // max_seq
            100, // min_ts_s
            200, // max_ts_s
            ByteSize(64),
        );
        reg.catalog_files.track(
            cat,
            std::path::PathBuf::from("/x/2026-06-19/default/cat-0000000007.catalog"),
        );
    }

    let h = make_handler(tr);
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"files": true}"#).unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();

    let t = &v["tenants"][0];
    assert_eq!(t["tenant"], "default");
    assert!(t["sfst"].as_array().unwrap().is_empty());

    // WAL: 2 entries, sorted by seq; active vs archived status surfaced.
    let wal = t["wal"].as_array().unwrap();
    assert_eq!(wal.len(), 2);
    assert_eq!(wal[0]["seq"], 10);
    assert_eq!(wal[0]["status"], "active");
    assert_eq!(wal[0]["entry_count"], 5);
    assert_eq!(wal[0]["max_ts_ns"], 200u64);
    assert_eq!(wal[0]["stream"]["name"], "walsvc");
    assert_eq!(wal[1]["seq"], 11);
    assert_eq!(wal[1]["status"], "archived");
    assert_eq!(wal[1]["size"], 1234u64);

    // Catalog: basename + ISO date + max_seq + pending_deletion.
    let catalog = t["catalog"].as_array().unwrap();
    assert_eq!(catalog.len(), 1);
    assert_eq!(catalog[0]["file"], "cat-0000000007.catalog");
    assert_eq!(catalog[0]["date"], "2026-06-19");
    assert_eq!(catalog[0]["max_seq"], 7);
    assert_eq!(catalog[0]["min_ts_s"], 100);
    assert_eq!(catalog[0]["pending_deletion"], false);
}

#[tokio::test]
async fn info_request_returns_capability_descriptor() {
    let h = make_handler(make_tenant_registries());
    let req: OtelLogsRequest = serde_json::from_slice(br#"{"info": true}"#).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], 200);
    assert!(
        v["accepted_params"]
            .as_array()
            .unwrap()
            .contains(&Value::String("after".into()))
    );
    assert!(v.get("facets").is_none());
}

#[tokio::test]
async fn empty_payload_defaults_to_data_request() {
    // Matches the legacy JournalRequest semantic: a POST body
    // without an `info` field is a data request, not capability
    // discovery. The UI's data POSTs rely on this.
    let req: OtelLogsRequest = serde_json::from_slice(b"{}").unwrap();
    assert!(!req.info);
    let h = make_handler(make_tenant_registries());
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    // Empty registry → empty Logs envelope, not the capability descriptor.
    assert!(v.get("facets").is_some());
    assert_eq!(v["items"]["matched"], 0);
}

#[tokio::test]
async fn no_sfst_yields_empty_envelope() {
    let h = make_handler(make_tenant_registries());
    let req: OtelLogsRequest =
        serde_json::from_slice(br#"{"info": false, "tenant": "tenant-a", "after": 100, "before": 200}"#).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], 200);
    assert!(v["facets"].as_array().unwrap().is_empty());
    assert_eq!(v["items"]["matched"], 0);
}

#[tokio::test]
async fn non_overlapping_window_yields_empty_envelope() {
    let mut tr = make_tenant_registries();
    install_sfst(&mut tr, "tenant-a", 1, 1_700_000_000);
    let h = make_handler(tr);

    // Request window is 1900..2000 — nowhere near the file's 1.7e9 span.
    let req: OtelLogsRequest =
        serde_json::from_slice(br#"{"info": false, "tenant": "tenant-a", "after": 1900, "before": 2000}"#).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert!(v["facets"].as_array().unwrap().is_empty());
    assert_eq!(v["items"]["matched"], 0);
}

#[tokio::test]
async fn populated_response_carries_facets_and_histogram() {
    let mut tr = make_tenant_registries();
    let min_s = 1_700_000_000;
    install_sfst(&mut tr, "tenant-a", 1, min_s);
    let h = make_handler(tr);

    let req: OtelLogsRequest = serde_json::from_slice(
        format!(
            r#"{{"info": false, "tenant": "tenant-a", "after": {}, "before": {}}}"#,
            min_s,
            min_s + 60
        )
        .as_bytes(),
    )
    .unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], 200);

    // Empty request → exactly one default facet, `severity_text`.
    // `service` is a low-card field too but isn't auto-surfaced; the
    // user adds it via the UI's "+ Add Filter Field" control.
    let facets = v["facets"].as_array().unwrap();
    let ids: Vec<&str> = facets.iter().map(|f| f["id"].as_str().unwrap()).collect();
    assert_eq!(ids, vec!["severity_text"]);

    // severity_text facet sees both values with count 3 each.
    let sev = facets.iter().find(|f| f["id"] == "severity_text").unwrap();
    let opts = sev["options"].as_array().unwrap();
    let counts: HashMap<&str, u64> = opts
        .iter()
        .map(|o| (o["id"].as_str().unwrap(), o["count"].as_u64().unwrap()))
        .collect();
    assert_eq!(counts.get("info"), Some(&3));
    assert_eq!(counts.get("error"), Some(&3));

    // Histogram defaulted to severity_text (one of DEFAULT_HISTOGRAM_FIELDS).
    assert_eq!(v["histogram"]["id"], "severity_text");
    // 6 logs spread across 6 seconds, all in-window.
    assert_eq!(v["items"]["matched"], 6);

    // available_histograms drops high-card (none here) but lists both fields.
    let avh = v["available_histograms"].as_array().unwrap();
    let avh_ids: Vec<&str> = avh.iter().map(|a| a["id"].as_str().unwrap()).collect();
    assert!(avh_ids.contains(&"service"));
    assert!(avh_ids.contains(&"severity_text"));

    // Row table: all 6 logs materialized, newest-first, no anchor.
    let data = v["data"].as_array().unwrap();
    assert_eq!(data.len(), 6);
    assert_eq!(v["items"]["returned"], 6);
    // First page from the newest edge → no newer rows, no older rows
    // (the whole file fits in one page of 200).
    assert_eq!(v["items"]["before"], 0);
    assert_eq!(v["items"]["after"], 0);
    // Columns: fixed timestamp/severity/cursor plus the attribute
    // fields; pagination points at the hidden cursor column.
    let columns = v["columns"].as_object().unwrap();
    assert!(columns.contains_key("timestamp"));
    assert!(columns.contains_key("severity"));
    assert!(columns.contains_key("cursor"));
    assert_eq!(columns["cursor"]["visible"], false);
    assert_eq!(v["pagination"]["column"], "cursor");
    // Low-card attribute fields are facetable (drive "+ Add Filter
    // Field"); the special columns are not.
    assert_eq!(columns["service"]["filter"], "facet");
    assert_eq!(columns["severity_text"]["filter"], "facet");
    assert_eq!(columns["timestamp"]["filter"], "none");
    assert_eq!(columns["cursor"]["filter"], "none");
    // Each row is a positional array: [ts_us, severity, cursor, …].
    // Rows are newest-first: row 0 is the last log (pos 5, error).
    let row0 = data[0].as_array().unwrap();
    assert_eq!(row0[0], (min_s as i64 + 5) * 1_000_000); // µs
    assert_eq!(row0[1], "error");
    // The cursor cell is the opaque "{ts_ns}:{seq}:{pos}" string.
    assert_eq!(
        row0[2],
        format!("{}:1:0:5", (min_s as i64 + 5) * 1_000_000_000)
    );
}

#[tokio::test]
async fn selection_filter_narrows_facet_counts_with_self_exclusion() {
    let mut tr = make_tenant_registries();
    let min_s = 1_700_000_000;
    install_sfst(&mut tr, "tenant-a", 1, min_s);
    let h = make_handler(tr);

    // Filter `service=api` (positions 0,1,2). The `severity_text` facet
    // should reflect that filter: info=2 (pos 0,2), error=1 (pos 1).
    // The `service` facet, by self-exclusion, should still see both
    // values at their full counts. Both facets are requested
    // explicitly (the default set is just `severity_text`).
    let payload = format!(
        r#"{{"info": false, "tenant": "tenant-a", "after": {a}, "before": {b}, "facets": ["severity_text", "service"], "selections": {{"service": ["api"]}}}}"#,
        a = min_s,
        b = min_s + 60
    );
    let req: OtelLogsRequest = serde_json::from_slice(payload.as_bytes()).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();

    let facets = v["facets"].as_array().unwrap();

    let sev = facets.iter().find(|f| f["id"] == "severity_text").unwrap();
    let sev_counts: HashMap<&str, u64> = sev["options"]
        .as_array()
        .unwrap()
        .iter()
        .map(|o| (o["id"].as_str().unwrap(), o["count"].as_u64().unwrap()))
        .collect();
    assert_eq!(sev_counts.get("info"), Some(&2));
    assert_eq!(sev_counts.get("error"), Some(&1));

    let svc = facets.iter().find(|f| f["id"] == "service").unwrap();
    let svc_counts: HashMap<&str, u64> = svc["options"]
        .as_array()
        .unwrap()
        .iter()
        .map(|o| (o["id"].as_str().unwrap(), o["count"].as_u64().unwrap()))
        .collect();
    // Self-exclusion: `api` and `worker` both visible at full counts.
    assert_eq!(svc_counts.get("api"), Some(&3));
    assert_eq!(svc_counts.get("worker"), Some(&3));
}

#[tokio::test]
async fn only_overlapping_file_contributes() {
    // Two files in the same tenant. The window matches only the
    // newer file's span — the older one's range is filtered out by
    // the candidate planner.
    let mut tr = make_tenant_registries();
    install_sfst(&mut tr, "tenant-a", 1, 1_600_000_000);
    install_sfst(&mut tr, "tenant-a", 99, 1_700_000_000);
    let h = make_handler(tr);

    let req: OtelLogsRequest =
        serde_json::from_slice(br#"{"info": false, "tenant": "tenant-a", "after": 1700000000, "before": 1700000100}"#)
            .unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    // Only the new file's 6 logs overlap the window.
    assert_eq!(v["items"]["matched"], 6);
}

#[tokio::test]
async fn multiple_overlapping_files_merge_counts_and_facets() {
    // Two SFSTs in the same tenant whose spans both fall inside the
    // request window. The planner returns both; the handler should
    // sum `matched` and union facet counts.
    //
    // Each file has 6 logs (3 info, 3 error; 3 api, 3 worker) so
    // the merged response should show 12 logs total.
    let mut tr = make_tenant_registries();
    let earlier = 1_700_000_000u32;
    let later = earlier + 100; // 6-second spans don't touch each other
    install_sfst(&mut tr, "tenant-a", 1, earlier);
    install_sfst(&mut tr, "tenant-a", 2, later);
    let h = make_handler(tr);

    // Window covers both files' spans. Request both facets
    // explicitly (the default set is just `severity_text`) so the
    // cross-file union is exercised on both fields.
    let payload = format!(
        r#"{{"info": false, "tenant": "tenant-a", "after": {a}, "before": {b}, "facets": ["severity_text", "service"]}}"#,
        a = earlier,
        b = later + 60
    );
    let req: OtelLogsRequest = serde_json::from_slice(payload.as_bytes()).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();

    // Both files contribute → 12 matched.
    assert_eq!(v["items"]["matched"], 12);

    // Facets union across files; per-value counts sum.
    let facets = v["facets"].as_array().unwrap();
    let sev = facets
        .iter()
        .find(|f| f["id"] == "severity_text")
        .expect("severity_text facet must be present in both files");
    // Each file has 3 `info` logs → merged 6.
    let info_count = sev["options"]
        .as_array()
        .unwrap()
        .iter()
        .find(|o| o["id"] == "info")
        .map(|o| o["count"].as_u64().unwrap())
        .unwrap_or(0);
    assert_eq!(info_count, 6);

    let svc = facets
        .iter()
        .find(|f| f["id"] == "service")
        .expect("service facet must be present");
    let svc_counts: HashMap<&str, u64> = svc["options"]
        .as_array()
        .unwrap()
        .iter()
        .map(|o| (o["id"].as_str().unwrap(), o["count"].as_u64().unwrap()))
        .collect();
    // Each file: 3 api, 3 worker → merged 6 each.
    assert_eq!(svc_counts.get("api"), Some(&6));
    assert_eq!(svc_counts.get("worker"), Some(&6));
}

#[tokio::test]
async fn file_without_histogram_field_routes_logs_to_unset() {
    // One file carries `severity_text` (6 logs); another lacks it
    // entirely (3 logs, only `service`). Both fall in the window. The
    // field-absent file's logs must all land in the histogram's
    // `(unset)` row, so the histogram total still equals `matched`
    // (9) rather than just the 6 logs that carry the field.
    let mut tr = make_tenant_registries();
    let earlier = 1_700_000_000u32;
    let later = earlier + 100; // disjoint spans
    install_sfst(&mut tr, "tenant-a", 1, earlier);
    install_service_only_sfst(&mut tr, "tenant-a", 2, later);
    let h = make_handler(tr);

    let payload = format!(
        r#"{{"info": false, "tenant": "tenant-a", "after": {a}, "before": {b}}}"#,
        a = earlier,
        b = later + 60
    );
    let req: OtelLogsRequest = serde_json::from_slice(payload.as_bytes()).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();

    assert_eq!(v["items"]["matched"], 9);

    // Sum every histogram count, and separately the trailing
    // "(unset)" column. `items` align to `labels[1..]`, so the unset
    // column is the last item in each datapoint.
    let result = &v["histogram"]["chart"]["result"];
    let n_items = result["labels"].as_array().unwrap().len() - 1;
    let unset_idx = n_items - 1;
    let mut total = 0u64;
    let mut unset_total = 0u64;
    for dp in result["data"].as_array().unwrap() {
        // Each datapoint is a flat array: [timestamp_ms, [v,arp,pa], …].
        let cols = dp.as_array().unwrap();
        for (i, it) in cols[1..].iter().enumerate() {
            let c = it[0].as_u64().unwrap();
            total += c;
            if i == unset_idx {
                unset_total += c;
            }
        }
    }
    // Histogram total tracks `matched`; the 3 field-absent logs are
    // all in "(unset)".
    assert_eq!(total, 9);
    assert_eq!(unset_total, 3);
}

#[tokio::test]
async fn no_time_bound_falls_back_to_recent_window() {
    // `(after=0, before=0)` is the legacy "no time bound" sentinel.
    // The effective-window helper should fall back to the last 15
    // minutes, so an SFST installed in that range produces a
    // populated response (rather than an empty stub).
    let now_s = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs() as u32;
    let recent = now_s.saturating_sub(300); // 5 min ago, inside the 15-min fallback

    let mut tr = make_tenant_registries();
    install_sfst(&mut tr, "tenant-a", 1, recent);
    let h = make_handler(tr);

    let req: OtelLogsRequest = serde_json::from_slice(br#"{"info": false, "tenant": "tenant-a"}"#).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    // Fixture has 6 logs — all should match (the file's range
    // [recent, recent+5] sits inside the 15-min fallback window).
    assert_eq!(v["items"]["matched"], 6);
}

#[tokio::test]
async fn no_time_bound_with_only_stale_data_yields_empty_envelope() {
    // `(after=0, before=0)` defaults to the last 15 minutes. The only
    // SFST is from 2024, so nothing overlaps the defaulted window. The
    // handler returns the empty envelope — it does not reach back to a
    // stale file just because the window came up empty.
    let mut tr = make_tenant_registries();
    let file_min_s = 1_700_000_000u32; // far in the past
    install_sfst(&mut tr, "tenant-a", 1, file_min_s);
    let h = make_handler(tr);

    let req: OtelLogsRequest = serde_json::from_slice(br#"{"info": false, "tenant": "tenant-a"}"#).unwrap();
    let resp = h.on_call(make_ctx("t1"), req).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["items"]["matched"], 0);
    assert!(v["facets"].as_array().unwrap().is_empty());
}

/// µs timestamp of the i-th fixture log (`min_s + i` seconds).
fn ts_us(min_s: u32, i: i64) -> i64 {
    (min_s as i64 + i) * 1_000_000
}

/// Pull the timestamp (cell 0) and cursor (cell 2) out of a row.
fn row_ts_cursor(row: &Value) -> (i64, String) {
    let cells = row.as_array().unwrap();
    (
        cells[0].as_i64().unwrap(),
        cells[2].as_str().unwrap().to_string(),
    )
}

#[tokio::test]
async fn backward_pagination_pages_without_overlap_or_gap() {
    let mut tr = make_tenant_registries();
    let min_s = 1_700_000_000;
    install_sfst(&mut tr, "tenant-a", 1, min_s); // 6 logs, 1s apart
    let h = make_handler(tr);
    let win = format!(r#""after": {}, "before": {}"#, min_s, min_s + 60);

    // Page 1: no anchor, backward → newest 3 (pos 5,4,3), newest-first.
    let p1: OtelLogsRequest =
        serde_json::from_slice(format!(r#"{{"info":false,"tenant":"tenant-a",{win},"last":3}}"#).as_bytes()).unwrap();
    let v1 = serde_json::to_value(&h.on_call(make_ctx("t1"), p1).await.unwrap()).unwrap();
    let d1 = v1["data"].as_array().unwrap();
    assert_eq!(d1.len(), 3);
    assert_eq!(v1["items"]["returned"], 3);
    assert_eq!(v1["items"]["after"], 1); // older rows remain
    assert_eq!(v1["items"]["before"], 0); // at the newest edge
    assert_eq!(row_ts_cursor(&d1[0]).0, ts_us(min_s, 5));
    assert_eq!(row_ts_cursor(&d1[1]).0, ts_us(min_s, 4));
    let (oldest_ts, anchor) = row_ts_cursor(&d1[2]);
    assert_eq!(oldest_ts, ts_us(min_s, 3));

    // Page 2: anchor = page 1's oldest row (pos 3), backward → pos 2,1,0.
    let p2: OtelLogsRequest = serde_json::from_slice(
        format!(r#"{{"info":false,"tenant":"tenant-a",{win},"last":3,"direction":"backward","anchor":"{anchor}"}}"#)
            .as_bytes(),
    )
    .unwrap();
    let v2 = serde_json::to_value(&h.on_call(make_ctx("t1"), p2).await.unwrap()).unwrap();
    let d2 = v2["data"].as_array().unwrap();
    assert_eq!(d2.len(), 3);
    assert_eq!(v2["items"]["after"], 0); // nothing older remains
    assert_eq!(v2["items"]["before"], 1); // newer rows exist (page 1)
    assert_eq!(row_ts_cursor(&d2[0]).0, ts_us(min_s, 2));
    assert_eq!(row_ts_cursor(&d2[1]).0, ts_us(min_s, 1));
    assert_eq!(row_ts_cursor(&d2[2]).0, ts_us(min_s, 0));

    // The anchor row (pos 3) is excluded from page 2 — no overlap, and
    // pos 2 immediately follows it — no gap.
    assert!(!d2.iter().any(|r| row_ts_cursor(r).0 == ts_us(min_s, 3)));
}

#[tokio::test]
async fn forward_pagination_returns_newer_rows_newest_first() {
    let mut tr = make_tenant_registries();
    let min_s = 1_700_000_000;
    install_sfst(&mut tr, "tenant-a", 1, min_s);
    let h = make_handler(tr);
    let win = format!(r#""after": {}, "before": {}"#, min_s, min_s + 60);

    // Forward from pos 2 (seq 1) → the rows strictly newer: pos 3,4,5,
    // returned newest-first.
    let anchor = format!("{}:1:0:2", (min_s as i64 + 2) * 1_000_000_000);
    let req: OtelLogsRequest = serde_json::from_slice(
        format!(r#"{{"info":false,"tenant":"tenant-a",{win},"last":3,"direction":"forward","anchor":"{anchor}"}}"#)
            .as_bytes(),
    )
    .unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
    let d = v["data"].as_array().unwrap();
    assert_eq!(d.len(), 3);
    assert_eq!(row_ts_cursor(&d[0]).0, ts_us(min_s, 5));
    assert_eq!(row_ts_cursor(&d[1]).0, ts_us(min_s, 4));
    assert_eq!(row_ts_cursor(&d[2]).0, ts_us(min_s, 3));
    assert_eq!(v["items"]["after"], 1); // pos 0,1,2 are older
    assert_eq!(v["items"]["before"], 0); // pos 5 is the newest row
}

#[tokio::test]
async fn histogram_click_numeric_anchor_navigates_to_time() {
    // Clicking a histogram bar sends `anchor` as a bare µs integer
    // (not a cursor string). It must deserialize and behave as a
    // "jump to this time" anchor: backward shows the newest rows up to
    // and including that time.
    let mut tr = make_tenant_registries();
    let min_s = 1_700_000_000;
    install_sfst(&mut tr, "tenant-a", 1, min_s); // 6 logs at +0..+5s
    let h = make_handler(tr);

    // Anchor at +3s, as microseconds, sent as a JSON integer.
    let anchor_us = (min_s as i64 + 3) * 1_000_000;
    let payload = format!(
        r#"{{"info":false,"tenant":"tenant-a","after":{a},"before":{b},"last":10,"direction":"backward","anchor":{anchor_us}}}"#,
        a = min_s,
        b = min_s + 60
    );
    let req: OtelLogsRequest = serde_json::from_slice(payload.as_bytes()).unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();

    // No deserialize error; rows up to and including +3s (pos 0..3),
    // newest-first.
    assert_eq!(v["status"], 200);
    let d = v["data"].as_array().unwrap();
    assert_eq!(d.len(), 4);
    assert_eq!(row_ts_cursor(&d[0]).0, ts_us(min_s, 3));
    assert_eq!(row_ts_cursor(&d[3]).0, ts_us(min_s, 0));
    assert_eq!(v["items"]["after"], 0); // nothing older than +0s
    assert_eq!(v["items"]["before"], 1); // +4s/+5s are newer
}

#[test]
fn patches_data_request_args_into_payload() {
    // No "info" token — data request. info must be false so the
    // handler runs the query path, not the capability descriptor.
    let args = vec![
        "after:100".to_string(),
        "before:200".to_string(),
        "slice:true".to_string(),
    ];
    let bytes = patch_args_into_payload(&args, None).unwrap();
    let req: OtelLogsRequest = serde_json::from_slice(&bytes).unwrap();
    assert_eq!(req.after, 100);
    assert_eq!(req.before, 200);
    assert!(!req.info);
}

#[test]
fn patches_info_request_args_into_payload() {
    // "info" token present — capability discovery.
    let args = vec![
        "info".to_string(),
        "after:100".to_string(),
        "before:200".to_string(),
    ];
    let bytes = patch_args_into_payload(&args, None).unwrap();
    let req: OtelLogsRequest = serde_json::from_slice(&bytes).unwrap();
    assert!(req.info);
    assert_eq!(req.after, 100);
    assert_eq!(req.before, 200);
}

#[test]
fn declaration_carries_legacy_flags() {
    let h = make_handler(make_tenant_registries());
    let d = h.declaration();
    assert_eq!(d.name, "otel-logs");
    assert!(d.global);
    assert_eq!(d.tags.as_deref(), Some("logs"));
    let access = d.access.unwrap();
    assert!(access.contains(HttpAccess::SIGNED_ID));
    assert!(access.contains(HttpAccess::SAME_SPACE));
    assert!(access.contains(HttpAccess::SENSITIVE_DATA));
}

// ── Tenant scoping (OTL-1) ───────────────────────────────────────

#[tokio::test]
async fn tenant_scoping_isolates_unions_nothing_and_defaults() {
    // Three tenants ingesting the same window: "tenant-a", "tenant-b",
    // and "default". Each query must read exactly one tenant's data.
    let min_s = 1_700_000_000u32;
    let mut tr = make_tenant_registries();
    install_sfst(&mut tr, "tenant-a", 1, min_s);
    install_sfst(&mut tr, "tenant-b", 2, min_s);
    install_sfst(&mut tr, "default", 3, min_s);
    let h = make_handler(tr);

    let win = format!(r#""after":{},"before":{}"#, min_s, min_s + 100);
    let matched = |v: &Value| v["items"]["matched"].as_u64().unwrap();

    // Explicit tenant: only that tenant's 6 rows — never the 18-row
    // union of all three.
    for tenant in ["tenant-a", "tenant-b"] {
        let req: OtelLogsRequest = serde_json::from_slice(
            format!(r#"{{"info":false,"tenant":"{tenant}",{win}}}"#).as_bytes(),
        )
        .unwrap();
        let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
        assert_eq!(matched(&v), 6, "tenant {tenant} must see only its own rows");
    }

    // Omitted tenant resolves to the literal "default" tenant.
    let req: OtelLogsRequest =
        serde_json::from_slice(format!(r#"{{"info":false,{win}}}"#).as_bytes()).unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
    assert_eq!(matched(&v), 6, "omitted tenant must read 'default' only");

    // Unknown tenant: empty grid-aligned stub, not an error.
    let req: OtelLogsRequest = serde_json::from_slice(
        format!(r#"{{"info":false,"tenant":"no-such-tenant",{win}}}"#).as_bytes(),
    )
    .unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
    assert_eq!(v["status"], 200);
    assert_eq!(matched(&v), 0);

    // Empty tenant string falls back to "default" (light hygiene).
    let req: OtelLogsRequest =
        serde_json::from_slice(format!(r#"{{"info":false,"tenant":"",{win}}}"#).as_bytes())
            .unwrap();
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
    assert_eq!(matched(&v), 6);
}

#[test]
fn accepted_params_advertise_tenant() {
    assert!(super::super::wire::ACCEPTED_PARAMS.contains(&"tenant"));
}

// ── remote-read query path (evicted SFST fetched back from remote) ───────────

/// Register a catalog entry for `id` (no local SFST) so `remote_candidates`
/// surfaces it. Mirrors the catalog-write fixture used in `query/tests.rs`.
fn track_remote_catalog(
    tr: &mut TenantRegistries,
    tenant: &str,
    id: FileId,
    remote_key: &str,
    min_s: u32,
    max_s: u32,
    size: u64,
) {
    use chrono::NaiveDate;
    let stream = ServiceStream::new("ns", "svc");
    // Production `build_catalog_entry` copies both id and stream from one SFST, so
    // the FileId's ns_hash always matches the stream's. Enforce it on the fixture.
    assert_eq!(
        id.ns_hash,
        stream.ns_hash(),
        "catalog fixture id.ns_hash must match its stream"
    );
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let entry = otel_catalog::CatalogEntry {
        id,
        remote_key: remote_key.to_string(),
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        total_logs: 6,
        stream: stream.clone(),
        size: ByteSize(size),
        uploaded_at_ns: TimestampNs(0),
        remote_etag: None,
    };
    let reg = tr.get_or_create(&TenantId::from(tenant));
    let mut catalog =
        otel_catalog::Catalog::new(TenantId::from(tenant), date, id.machine_id, id.boot_id);
    catalog.add(entry);
    let path = reg
        .catalog_files
        .file_path(date, id.machine_id, id.boot_id, id.seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let csize = ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, id.machine_id, id.boot_id, id.seq, min_s, max_s, csize),
        path,
    );
}

fn make_handler_with_remote(tr: TenantRegistries, remote: RemoteRead) -> OtelLogsHandler {
    OtelLogsHandler::new(
        Arc::new(RwLock::new(tr)),
        Arc::new(crate::chunk::ChunkCache::new(64 * 1024 * 1024)),
        16_384,
        Some(remote),
    )
}

/// End-to-end: a tenant whose only copy of an SFST is in remote object storage
/// (no local SFST/WAL) still answers a query — the handler fetches the file back
/// through the read cache and the engine serves all 6 logs. Uses a real
/// `OpendalStorage` over an `fs://` backend (exercises the real `Storage::read`).
#[tokio::test]
async fn remote_only_sfst_is_fetched_and_served() {
    let mut tr = make_tenant_registries();

    // Build a real SFST's bytes; do NOT install it locally. The FileId's ns_hash
    // matches the stream (as production `build_catalog_entry` guarantees).
    let id = FileId::new(
        Uuid::from_u128(0x11),
        Uuid::from_u128(0x22),
        1,
        ServiceStream::new("ns", "svc").ns_hash(),
    );
    let min_s = 1_700_000_000u32;
    let sfst_tmp = tempfile::NamedTempFile::new().unwrap();
    write_test_sfst(sfst_tmp.path(), min_s);
    let sfst_bytes = std::fs::read(sfst_tmp.path()).unwrap();

    // Place the object in an fs:// remote backend at its catalog remote_key.
    let remote_dir = tempfile::tempdir().unwrap().keep();
    let remote_key = "v1/tenants/default/sfst/seq1.sfst";
    let obj_path = remote_dir.join(remote_key);
    std::fs::create_dir_all(obj_path.parent().unwrap()).unwrap();
    std::fs::write(&obj_path, &sfst_bytes).unwrap();

    track_remote_catalog(&mut tr, "default", id, remote_key, min_s, min_s + 5, sfst_bytes.len() as u64);

    let storage =
        crate::storage::OpendalStorage::new(&format!("fs://{}", remote_dir.display())).unwrap();
    let cache =
        file_cache::FileCache::open(tempfile::tempdir().unwrap().keep(), 64 * 1024 * 1024).unwrap();
    let h = make_handler_with_remote(tr, RemoteRead::new(storage, cache));

    let req: OtelLogsRequest = serde_json::from_slice(
        format!(
            r#"{{"info":false,"tenant":"default","after":{},"before":{}}}"#,
            min_s - 10,
            min_s + 100
        )
        .as_bytes(),
    )
    .unwrap();
    // Keep a handle to the progress state so we can assert the fetch phase
    // advanced it. Build the context inline (make_ctx hides its ProgressState).
    let progress = bridge::function::ProgressState::new();
    let ctx = FunctionCallContext::new("t1".to_string(), progress.clone(), CancellationToken::new());
    let v = serde_json::to_value(&h.on_call(ctx, req).await.unwrap()).unwrap();
    assert_eq!(
        v["items"]["matched"], 6,
        "evicted remote SFST should be fetched and queried: {v:#}"
    );
    // #5: the remote-only stream is advertised in the window-scoped selector
    // (its data is fetchable, so the user can filter to it).
    let streams = v["required_params"]
        .as_array()
        .unwrap()
        .iter()
        .find(|p| p["id"] == "__streams")
        .and_then(|p| p["options"].as_array())
        .expect("stream selector present");
    assert!(
        streams.iter().any(|o| o["name"] == "ns/svc"),
        "remote-only stream must appear in the selector: {v:#}"
    );
    // One remote-only SFST, no local sources: total = 0 local + 2*1 remote
    // (one download unit + one scan unit). `done` reaches both — the fetch
    // closure counted the download, the engine counted the scan. Without
    // fetch-phase progress, `done` would only reach 1.
    let (done, total) = progress.load();
    assert_eq!(total, 2, "fetch + scan phases sized into total");
    assert_eq!(done, 2, "fetch phase advanced done, then the scan completed it");
}

/// When the remote object cannot be read, the query degrades gracefully (no
/// panic, no error) rather than failing — the bad source is simply omitted.
#[tokio::test]
async fn remote_fetch_failure_degrades() {
    let mut tr = make_tenant_registries();
    let id = FileId::new(
        Uuid::from_u128(0x11),
        Uuid::from_u128(0x22),
        1,
        ServiceStream::new("ns", "svc").ns_hash(),
    );
    let min_s = 1_700_000_000u32;
    // Catalog entry points at a remote_key that does not exist in the backend.
    let remote_dir = tempfile::tempdir().unwrap().keep();
    track_remote_catalog(&mut tr, "default", id, "missing/object.sfst", min_s, min_s + 5, 10);

    let storage =
        crate::storage::OpendalStorage::new(&format!("fs://{}", remote_dir.display())).unwrap();
    let cache =
        file_cache::FileCache::open(tempfile::tempdir().unwrap().keep(), 64 * 1024 * 1024).unwrap();
    let h = make_handler_with_remote(tr, RemoteRead::new(storage, cache));

    let req: OtelLogsRequest = serde_json::from_slice(
        format!(
            r#"{{"info":false,"tenant":"default","after":{},"before":{}}}"#,
            min_s - 10,
            min_s + 100
        )
        .as_bytes(),
    )
    .unwrap();
    // Degrades: the unreadable remote source is omitted, the query still answers.
    let v = serde_json::to_value(&h.on_call(make_ctx("t1"), req).await.unwrap()).unwrap();
    assert_eq!(v["items"]["matched"], 0, "unreadable remote source is omitted: {v:#}");
}
