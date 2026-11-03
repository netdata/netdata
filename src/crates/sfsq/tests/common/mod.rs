//! Shared fixtures for the sfsq traces integration suites: every
//! `tests/traces_*.rs` binary declares `mod common;` (attributes,
//! by_id, gate, overview, rollup_tail, search, slowest); the
//! ng_wal_equivalence.rs logs harness builds its own fixtures instead.
//! Contents: OTLP span specs and request builders, a WAL writer running
//! the production normalize→flatten→encode pipeline, and `TraceSource`
//! constructors for the shapes the suites mix (sealed file, in-memory
//! chunk, tail, unavailable, missing, legacy no-rollup).

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
use sfsq::traces::{
    SourceId, TraceSfstCandidate, TraceSource, TraceUnavailable, TraceWalTail, WalCoverage,
};

/// The shared trace id (`0xAB` × 16): what [`sp`] puts on every span
/// unless a suite overrides `trace`.
pub const TRACE: [u8; 16] = [0xABu8; 16];

pub fn kv_str(k: &str, v: &str) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::StringValue(v.into())),
        }),
        key_strindex: 0,
    }
}

pub fn kv_int(k: &str, v: i64) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::IntValue(v)),
        }),
        key_strindex: 0,
    }
}

pub fn kv_double(k: &str, v: f64) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: Some(AnyValue {
            value: Some(Av::DoubleValue(v)),
        }),
        key_strindex: 0,
    }
}

/// An attribute with NO value — flattens to a `Null` entry, the key
/// stays present. Only traces_attributes.rs exercises this.
pub fn kv_null(k: &str) -> KeyValue {
    KeyValue {
        key: k.into(),
        value: None,
        key_strindex: 0,
    }
}

#[derive(Clone)]
pub struct SpanSpec {
    /// The owning trace (defaults to the shared [`TRACE`]; the search
    /// and gate suites build multi-trace corpora).
    pub trace: [u8; 16],
    pub id: [u8; 8],
    pub parent: [u8; 8],
    pub start: u64,
    pub end: u64,
    pub name: &'static str,
    pub kind: i32,
    /// OTLP status `(code, message)`; `None` sends no status object, which
    /// is stored as the OTel default (`status_code = UNSET`, no message).
    pub status: Option<(i32, &'static str)>,
    /// W3C trace state, stored verbatim when non-empty.
    pub trace_state: &'static str,
    pub attrs: Vec<KeyValue>,
    pub events: Vec<(&'static str, Vec<KeyValue>)>,
    pub links: Vec<([u8; 16], [u8; 8], Vec<KeyValue>)>,
}

/// The default spec: the shared [`TRACE`], byte-repeated `id`/`parent`
/// (`parent == 0` = root), a 50 ns duration, everything else empty.
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

/// A spec as its protobuf `Span` — what [`req`]/[`req_with`] embed.
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

/// The common request shape: one `service.name = "svc"` resource
/// attribute, no instrumentation scope, `spans` as its only scope's spans.
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

/// Write `reqs` into a fresh traces WAL under `dir` through the
/// production pipeline — `ng_flatten::normalize_trace_request`
/// (monotonic-clock fallback, no time bounds), `flatten_trace_request`,
/// `encode_trace_frame` — one frame per non-empty request (empty ones
/// are skipped), CRC and compression on, rotation disabled. Returns the
/// one `.wal` file. `meta_tag` becomes the stream's `content_meta`
/// (carried verbatim into sealed summaries) and the `wal-{tag}-{n}`
/// directory name; the engine never reads it.
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

/// A process-global counter — the `-N` in [`write_wal`]'s directory
/// names, so two WALs in one dir never collide.
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

/// Seal `wal_path` into `<id>.sfst` under `dir` with the production
/// sealer (`ng_index::build_sfst_traces_file`) and return the sealed
/// source over it — the standard "a sealed file" fixture.
pub fn sealed_source(dir: &Path, wal_path: &Path, id: &str) -> TraceSource {
    let out = dir.join(format!("{id}.sfst"));
    ng_index::build_sfst_traces_file(wal_path, &out, &ng_index::Metrics::new()).unwrap();
    sealed_source_at(&out, id)
}

/// Flip the first payload byte of chunk `id` inside a sealed SFST — a
/// CRC mismatch, the cheapest "corrupt in any way" (the gate and
/// overview suites corrupt `TRSU`/`TBLM`/`SPAN`/`TRCE` this way).
pub fn corrupt_chunk(path: &Path, id: [u8; 4]) {
    let mut bytes = std::fs::read(path).unwrap();
    let offset = {
        let container = chunk_file::container::Container::open(&bytes, b"SFST", 1).unwrap();
        let meta = container.chunk_meta(id).expect("chunk present");
        usize::try_from(meta.offset).unwrap()
    };
    bytes[offset] ^= 0xFF;
    std::fs::write(path, &bytes).unwrap();
}

/// A sealed-file source over an ALREADY-WRITTEN `.sfst` — for suites
/// that doctor the sealed bytes (chunk corruption) and must not
/// re-seal over their surgery when building the source vectors. Pattern:
/// seal once, corrupt in place, then source the doctored file.
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

/// A WAL tail over the whole range of a shut-down WAL ([`whole_range`]);
/// its `wal_id` follows [`memory_source`]'s rule.
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

/// A source whose bytes could not be obtained (a failed remote
/// download), known to span `[min_s, max_s]` from its catalog summary.
pub fn unavailable_source(id: &str, min_s: u32, max_s: u32) -> TraceSource {
    TraceSource::Unavailable(TraceUnavailable {
        source_id: SourceId::new(id.to_string()),
        summary: sfst::Summary {
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            record_count: 1,
            content_meta: Vec::new(),
        },
    })
}

/// A sealed source whose file is gone (a local file deleted before it
/// was opened): its bytes fail to map.
pub fn missing_source(dir: &Path, id: &str, min_s: u32, max_s: u32) -> TraceSource {
    TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new(id.to_string()),
        summary: sfst::Summary {
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            record_count: 1,
            content_meta: Vec::new(),
        },
        source: Source::File(dir.join(format!("{id}-missing.sfst"))),
        coverage: None,
    })
}

/// A minimal valid SFST WITHOUT the optional `TRSU` trace-rollup chunk —
/// a hand-built pre-rollup ("legacy") file. The overview and slowest
/// suites use it to pin the never-mixed rule: a rollup-less file
/// contributes no traces to rollup-backed results and the status reports
/// `RollupAbsent`. Returns the sealed-file source wrapping it.
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
