//! Shared corpus/WAL/source helpers for the traces integration suites
//! (`traces_by_id.rs`, `traces_tags.rs`): OTLP span specs, the
//! flatten-encode WAL writer, and the three source shapes (sealed file,
//! in-memory chunk, tail).

// Each integration-test crate compiles its own copy of this module and
// uses a different subset of it.
#![allow(dead_code)]

use std::path::{Path, PathBuf};
use std::sync::Arc;

use file_registry::{ByteSize, MonotonicClock};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, InstrumentationScope, KeyValue, any_value::Value as Av,
};
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status};

use sfsq::Source;
use sfsq::traces::{SourceId, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage};

pub const TRACE: [u8; 16] = [0xABu8; 16];

pub fn kv_str(k: &str, v: &str) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::StringValue(v.into())),
        }),
    }
}

pub fn kv_int(k: &str, v: i64) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::IntValue(v)),
        }),
    }
}

pub fn kv_double(k: &str, v: f64) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::DoubleValue(v)),
        }),
    }
}

/// An attribute with NO value — flattens to a `Null` entry (a
/// dictionary value with no scalar kind).
pub fn kv_null(k: &str) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: None,
    }
}

#[derive(Clone)]
pub struct SpanSpec {
    /// The owning trace (defaults to the shared [`TRACE`]; the search
    /// suite builds multi-trace corpora).
    pub trace: [u8; 16],
    pub id: [u8; 8],
    pub parent: [u8; 8],
    pub start: u64,
    pub end: u64,
    pub name: &'static str,
    pub kind: i32,
    /// OTLP status `(code, message)`; `None` leaves status unset (no
    /// `status_code`/`status_message` entries are stored).
    pub status: Option<(i32, &'static str)>,
    /// W3C trace state, stored verbatim when non-empty.
    pub trace_state: &'static str,
    pub attrs: Vec<KeyValue>,
    pub events: Vec<(&'static str, Vec<KeyValue>)>,
    pub links: Vec<([u8; 16], [u8; 8], Vec<KeyValue>)>,
}

pub fn sp(id: u8, parent: u8, start: u64, name: &'static str) -> SpanSpec {
    SpanSpec {
        trace: TRACE,
        id: [id; 8],
        parent: [parent; 8],
        start,
        end: start + 50,
        name,
        kind: 0,
        status: None,
        trace_state: "",
        attrs: Vec::new(),
        events: Vec::new(),
        links: Vec::new(),
    }
}

pub fn to_otlp(s: &SpanSpec) -> Span {
    use opentelemetry_proto::tonic::trace::v1::span::{Event, Link};
    Span {
        trace_id: s.trace.to_vec(),
        span_id: s.id.to_vec(),
        parent_span_id: if s.parent == [0u8; 8] {
            Vec::new()
        } else {
            s.parent.to_vec()
        },
        start_time_unix_nano: s.start,
        end_time_unix_nano: s.end,
        name: s.name.into(),
        kind: s.kind,
        trace_state: s.trace_state.into(),
        status: s.status.map(|(code, message)| Status {
            code,
            message: message.into(),
        }),
        attributes: s.attrs.clone(),
        events: s
            .events
            .iter()
            .map(|(name, attrs)| Event {
                time_unix_nano: s.start + 1,
                name: (*name).into(),
                attributes: attrs.clone(),
                ..Default::default()
            })
            .collect(),
        links: s
            .links
            .iter()
            .map(|(tid, sid, attrs)| Link {
                trace_id: tid.to_vec(),
                span_id: sid.to_vec(),
                attributes: attrs.clone(),
                ..Default::default()
            })
            .collect(),
        ..Default::default()
    }
}

pub fn req(spans: &[SpanSpec]) -> ExportTraceServiceRequest {
    req_with(vec![kv_str("service.name", "svc")], None, spans)
}

/// A request with explicit resource attributes and an optional
/// instrumentation scope `(name, version, attributes)`.
pub fn req_with(
    resource_attrs: Vec<KeyValue>,
    scope: Option<(&str, &str, Vec<KeyValue>)>,
    spans: &[SpanSpec],
) -> ExportTraceServiceRequest {
    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes: resource_attrs,
                ..Default::default()
            }),
            scope_spans: vec![ScopeSpans {
                scope: scope.map(|(name, version, attributes)| InstrumentationScope {
                    name: name.into(),
                    version: version.into(),
                    attributes,
                    ..Default::default()
                }),
                spans: spans.iter().map(to_otlp).collect(),
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

/// Write requests into a fresh traces WAL under `dir`, one frame per
/// request, returning the WAL path. `meta_tag` differentiates part_key /
/// content_meta blobs across "streams" (the engine must not care).
pub fn write_wal(dir: &Path, reqs: Vec<ExportTraceServiceRequest>, meta_tag: &str) -> PathBuf {
    let sub = dir.join(format!("wal-{meta_tag}-{}", rand_suffix()));
    std::fs::create_dir_all(&sub).unwrap();
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
        &sub,
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
        if count == 0 {
            continue;
        }
        let base = clock.now_ns().as_u64();
        ng_flatten::normalize_trace_request(&mut r, base, None);
        let (flat, _) = ng_flatten::flatten_trace_request(r);
        let data = ng_flatten::encode_trace_frame(&flat).unwrap();
        writer
            .write_frame(
                0,
                meta_tag.as_bytes(),
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
    std::fs::read_dir(&sub)
        .unwrap()
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "wal"))
        .expect("a wal file was written")
}

pub fn rand_suffix() -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    static N: AtomicU64 = AtomicU64::new(0);
    format!("{}", N.fetch_add(1, Ordering::Relaxed))
}

/// The whole WAL's frame range (header to EOF — a shut-down test WAL).
pub fn whole_range(wal_path: &Path) -> wal::FrameRange {
    let len = std::fs::metadata(wal_path).unwrap().len();
    wal::FrameRange::new(wal::HEADER_SIZE as u64, len)
}

pub fn sealed_source(dir: &Path, wal_path: &Path, id: &str) -> TraceSource {
    let out = dir.join(format!("{id}.sfst"));
    ng_index::build_sfst_traces_file(wal_path, &out, &ng_index::Metrics::new()).unwrap();
    sealed_source_at(&out, id)
}

/// A sealed-file source over an ALREADY-WRITTEN `.sfst` — for suites
/// that doctor the sealed bytes (chunk corruption/removal) and must not
/// re-seal over their surgery when building the source vectors.
pub fn sealed_source_at(path: &Path, id: &str) -> TraceSource {
    let bytes = std::fs::read(path).unwrap();
    let summary = sfst::read_summary(&bytes).unwrap();
    TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new(id.to_string()),
        summary,
        source: Source::File(path.to_owned()),
        coverage: None,
    })
}

/// `wal_id` derives from the WAL's PATH (not the caller's source id) in
/// both WAL-derived helpers, mirroring production derivation — so a test
/// mixing a chunk and a tail of the same WAL genuinely exercises the
/// overlap protection instead of silently evading it via distinct ids.
pub fn memory_source(wal_path: &Path, id: &str) -> TraceSource {
    let range = whole_range(wal_path);
    let (summary, bytes) = ng_index::build_sfst_traces_range(wal_path, range).unwrap();
    TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new(id.to_string()),
        summary,
        source: Source::Memory(Arc::new(bytes)),
        coverage: Some(WalCoverage {
            wal_id: wal_path.display().to_string().into(),
            range,
        }),
    })
}

pub fn tail_source(wal_path: &Path, id: &str) -> TraceSource {
    let range = whole_range(wal_path);
    TraceSource::Tail(TraceWalTail {
        source_id: SourceId::new(id.to_string()),
        path: wal_path.to_owned(),
        coverage: WalCoverage {
            wal_id: wal_path.display().to_string().into(),
            range,
        },
    })
}

/// A minimal valid SFST WITHOUT a `TRSU` chunk — a hand-built
/// pre-rollup ("legacy") file for the no-mixed-units exclusion tests. Returns the
/// sealed-file source wrapping it.
pub fn legacy_sfst_source(dir: &Path, name: &str) -> TraceSource {
    let legacy_path = dir.join(format!("{name}.sfst"));
    let counts = sfst::ChunkCounts {
        columns: sfst::ColumnsPresent::default(),
        trace_id_index: false,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let summary = sfst::Summary {
        min_timestamp_s: 0,
        max_timestamp_s: 10,
        record_count: 1,
        content_meta: Vec::new(),
    };
    let metadata = sfst::Metadata {
        histogram: sfst::Histogram {
            timestamps: vec![0],
            counts: vec![1],
        },
        id_ranges: sfst::IdRanges {
            low_end: sfst::KvId(1),
            mid_end: sfst::KvId(1),
            high_end: sfst::KvId(1),
        },
        tree: sfst::SchemaTree::flat(
            &vec![sfst::FieldEntry {
                name: "name".into(),
                cardinality: 1,
                tier: sfst::FieldTier::Low,
            }]
            .into(),
        ),
        columns: sfst::ColumnsTable::default(),
    };
    let mut w = sfst::ChunkWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    w.summary(&summary).unwrap();
    w.metadata(&metadata).unwrap();
    w.timestamps(&[1_000_000_000]).unwrap();
    w.primary(vec![("name=legacy", {
        let mut data = Vec::new();
        let desc = treight::Bitmap::from_sorted_iter([0u32].into_iter(), 1, &mut data);
        sfst::BitmapValue { desc, data }
    })])
    .unwrap();
    w.add_stream_batch(&sfst::StreamBatch::for_write(&[vec![sfst::KvId(0)]]))
        .unwrap();
    let bytes = w.finish().unwrap().into_inner();
    std::fs::write(&legacy_path, &bytes).unwrap();

    let legacy_summary = sfst::read_summary_path(&legacy_path).unwrap();
    TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new(name),
        summary: legacy_summary,
        source: Source::File(legacy_path),
        coverage: None,
    })
}
