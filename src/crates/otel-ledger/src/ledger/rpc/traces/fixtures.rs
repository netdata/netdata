//! Shared test fixtures for the traces rpc suites (sources, handler):
//! registries construction, sealed-SFST registration, and REAL trace-WAL
//! fixtures (OTLP → ng-flatten trace frames) so chunk builds run the
//! actual traces seal. Test-only (`#[cfg(test)]` at the module decl).

use std::sync::Arc;

use file_registry::{ByteSize, FileId, MonotonicClock, TenantId, TimestampNs, test_identity};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status};
use tokio::sync::RwLock;

use file_lifecycle::registry::TenantRegistries;

/// Fresh empty registries over throwaway dirs.
pub(crate) fn make_registries() -> Arc<RwLock<TenantRegistries>> {
    Arc::new(RwLock::new(TenantRegistries::new(
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
    )))
}

pub(crate) fn summary(record_count: u32, min_s: u32, max_s: u32) -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        content_meta: Vec::new(),
    }
}

/// Track a sealed SFST under `tenant` and return its registry path.
/// The file itself is never written: `capture` maps registry state
/// (summaries come from `track`); the engine opens files later.
pub(crate) async fn install_sfst(
    registries: &Arc<RwLock<TenantRegistries>>,
    tenant: &str,
    seq: u64,
    min_s: u32,
    max_s: u32,
) -> std::path::PathBuf {
    let id = FileId::new(test_identity(), 1, seq, 7);
    let mut guard = registries.write().await;
    let reg = guard.get_or_create(&TenantId::from(tenant));
    let path = reg.sfst.file_path(id);
    reg.sfst.track(id, ByteSize(1024), summary(6, min_s, max_s));
    path
}

/// One OTLP request holding `span_count` spans of one synthetic trace,
/// starting at `base_ns`. Just enough shape for the seal to index. Span
/// 1 is the root (unset parent); spans 2.. parent to span 1.
pub(crate) fn otlp_req(trace_byte: u8, span_count: u8, base_ns: u64) -> ExportTraceServiceRequest {
    otlp_req_svc(trace_byte, span_count, base_ns, "svc")
}

/// Like [`otlp_req`], with an explicit `service.name` resource attribute
/// — the search filter tests select on it.
pub(crate) fn otlp_req_svc(
    trace_byte: u8,
    span_count: u8,
    base_ns: u64,
    service: &str,
) -> ExportTraceServiceRequest {
    let spans = (1..=span_count)
        .map(|i| Span {
            trace_id: vec![trace_byte; 16],
            span_id: vec![i; 8],
            parent_span_id: if i == 1 { Vec::new() } else { vec![1u8; 8] },
            name: format!("span-{i}"),
            start_time_unix_nano: base_ns + u64::from(i) * 1_000,
            end_time_unix_nano: base_ns + u64::from(i) * 1_000 + 500,
            ..Default::default()
        })
        .collect();
    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes: vec![KeyValue {
                    key: "service.name".into(),
                    value: Some(AnyValue {
                        value: Some(Av::StringValue(service.into())),
                    }),
                }],
                ..Default::default()
            }),
            scope_spans: vec![ScopeSpans {
                spans,
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

/// Like [`otlp_req_svc`], with span 1 carrying an OTLP ERROR status —
/// for the overview's error-totals coverage.
pub(crate) fn otlp_req_err(
    trace_byte: u8,
    span_count: u8,
    base_ns: u64,
    service: &str,
) -> ExportTraceServiceRequest {
    let mut req = otlp_req_svc(trace_byte, span_count, base_ns, service);
    req.resource_spans[0].scope_spans[0].spans[0].status = Some(Status {
        code: 2, // STATUS_CODE_ERROR
        message: "boom".into(),
    });
    req
}

/// One trace whose spans start at the EXACT given times (ns) — for
/// tests where a trace's envelope start differs materially from its
/// newest span (the search rank key). Span 1 is the root.
pub(crate) fn otlp_req_at(
    trace_byte: u8,
    span_starts_ns: &[u64],
    service: &str,
) -> ExportTraceServiceRequest {
    // The fixture id space is u8 (span ids are vec![i; 8] with i: u8):
    // 256+ starts would wrap the count to 0 and the zip below would
    // silently build no spans.
    assert!(
        span_starts_ns.len() <= u8::MAX as usize,
        "{} spans would wrap the fixture's u8 count",
        span_starts_ns.len()
    );
    let mut req = otlp_req_svc(trace_byte, span_starts_ns.len() as u8, 0, service);
    let spans = &mut req.resource_spans[0].scope_spans[0].spans;
    for (span, &start) in spans.iter_mut().zip(span_starts_ns) {
        span.start_time_unix_nano = start;
        span.end_time_unix_nano = start + 500;
    }
    req
}

/// Write `reqs` into a fresh traces WAL (one frame per request) in a
/// throwaway dir and return the produced file's path. Mirrors the sfsq
/// integration suites' fixture recipe.
fn write_traces_wal(
    dir: &std::path::Path,
    reqs: Vec<ExportTraceServiceRequest>,
) -> std::path::PathBuf {
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let config = wal::Config {
        rotation: wal::RotationConfig {
            max_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    };
    let mut writer = wal::Writer::new(
        dir,
        config,
        seq,
        wal::FileStamp {
            pipeline_id: 1,
            payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
        },
        wal::test_identity(),
    )
    .unwrap();
    let mut clock = MonotonicClock::new();
    for mut r in reqs {
        let count: usize = r
            .resource_spans
            .iter()
            .flat_map(|rs| rs.scope_spans.iter())
            .map(|ss| ss.spans.len())
            .sum();
        let base = clock.now_ns().as_u64();
        ng_flatten::normalize_trace_request(&mut r, base, None);
        let (flat, _) = ng_flatten::flatten_trace_request(r);
        let data = ng_flatten::encode_trace_frame(&flat).unwrap();
        writer
            .write_frame(
                0,
                b"test-meta",
                &data,
                wal::FrameMeta {
                    entry_count: count,
                    ingestion_ns: clock.now_ns(),
                    log_ts_range: None,
                },
            )
            .unwrap();
    }
    writer.shutdown_all().unwrap();
    std::fs::read_dir(dir)
        .unwrap()
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a wal file was written")
}

/// Build a real traces WAL from `reqs`, install its bytes at the
/// registry's path for a fresh WAL id, and register it as an active
/// synced WAL (`valid_up_to` = the real file length). Returns the
/// registry path `capture` will resolve.
pub(crate) async fn install_wal(
    registries: &Arc<RwLock<TenantRegistries>>,
    tenant: &str,
    seq: u64,
    reqs: Vec<ExportTraceServiceRequest>,
) -> std::path::PathBuf {
    let frame_count = reqs.len() as u64;
    let entry_count: u64 = reqs
        .iter()
        .flat_map(|r| r.resource_spans.iter())
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len() as u64)
        .sum();
    let staging = tempfile::tempdir().unwrap();
    let written = write_traces_wal(staging.path(), reqs);
    let bytes = std::fs::read(&written).unwrap();

    let id = FileId::new(test_identity(), 1, seq, 7);
    let mut guard = registries.write().await;
    let reg = guard.get_or_create(&TenantId::from(tenant));
    let path = reg.wal.file_path(id);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, &bytes).unwrap();
    reg.wal
        .apply_event(&wal::FileEvent::Created {
            file_id: id,
            created_at_ns: TimestampNs(1_000),
            content_meta: b"test-meta".to_vec(),
        })
        .unwrap();
    reg.wal
        .apply_event(&wal::FileEvent::Synced {
            file_id: id,
            valid_up_to: ByteSize(bytes.len() as u64),
            frame_count,
            entry_count,
            min_timestamp_ns: TimestampNs(1),
            max_timestamp_ns: TimestampNs(u64::MAX),
        })
        .unwrap();
    path
}
