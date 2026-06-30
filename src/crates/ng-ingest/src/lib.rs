//! Core of the `ng-ingest` receiver: flatten one OTLP export request and append it
//! as one WAL frame in the flattened-frame format (see `ng-flatten`).

use anyhow::Context as _;
use file_registry::{ByteSize, MonotonicClock, TimestampNs};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;

/// Every frame goes to one logical stream, hence one WAL file. The WAL treats
/// `part_key` as opaque; a single constant keeps everything in one file.
pub const PART_KEY: u64 = 0;

/// The opaque signal axis stamped into the file id (logs == 0). The WAL ascribes
/// no meaning to it here; it only labels the on-disk file name.
pub const PIPELINE_ID: u16 = 0;

/// The traces signal axis (== `bridge::signals::Signal::Traces.pipeline_id()`),
/// stamped into the traces WAL file id.
pub const TRACES_PIPELINE_ID: u16 = 1;

/// Count the log records across every `ResourceLogs`/`ScopeLogs` in a batch.
pub fn count_log_records(req: &ExportLogsServiceRequest) -> usize {
    req.resource_logs
        .iter()
        .flat_map(|rl| rl.scope_logs.iter())
        .map(|sl| sl.log_records.len())
        .sum()
}

/// One WAL file, no rotation, frames always LZ4-compressed: a single `part_key`
/// plus rotation thresholds set to effectively infinite so the writer never
/// starts a second file. `wal::Reader` decompresses transparently, so the
/// artifact stays directly consumable by a downstream reader.
pub fn one_file_config() -> wal::Config {
    wal::Config {
        rotation: wal::RotationConfig {
            max_log_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    }
}

/// Normalize timestamps and ids, flatten `req`, and append it as a single WAL frame
/// in the flattened-frame format, returning the number of log records written.
///
/// `req` is normalized **in place** first: [`ng_flatten::normalize_log_timestamps`] resolves
/// each record's `time_unix_nano`, and [`ng_flatten::normalize_log_ids`] drops malformed trace/span ids
/// (logging one aggregated warning per request). The request is then flattened into a
/// per-frame schema tree + typed entries, each entry's `xxhash64(key=value)` is
/// pre-computed (so the downstream SFST build rides the interner's fast path), and the
/// result is bincode-encoded as the frame payload. `log_min/max_ts` are left `ZERO`
/// (no per-frame time-range summary). A request with zero log records writes no frame
/// and returns `0`.
pub fn write_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: &mut ExportLogsServiceRequest,
) -> anyhow::Result<usize> {
    let entry_count = count_log_records(req);
    if entry_count == 0 {
        return Ok(0);
    }
    // One clock tick for the synthetic-timestamp base; normalize then runs
    // lock-free (base + offset for any record lacking event/observed time).
    let fallback_base_ns = clock.now_ns().as_u64();
    ng_flatten::normalize_log_timestamps(req, fallback_base_ns);
    let bad_ids = ng_flatten::normalize_log_ids(req);
    if bad_ids.any() {
        tracing::warn!(
            bad_trace_ids = bad_ids.trace,
            bad_span_ids = bad_ids.span,
            "dropped malformed trace/span ids at ingest (expected {}/{} bytes); stored as zero",
            ng_flatten::TRACE_ID_LEN,
            ng_flatten::SPAN_ID_LEN,
        );
    }
    let mut flattened = ng_flatten::flatten_log_request(req);
    ng_flatten::fill_log_hashes(&mut flattened);
    let data = ng_flatten::encode_log_frame(&flattened).context("encode flattened frame")?;
    let ingestion_ns = clock.now_ns();
    writer.write_frame(
        PART_KEY,
        &[],
        &data,
        entry_count,
        ingestion_ns,
        TimestampNs::ZERO,
        TimestampNs::ZERO,
    )?;
    Ok(entry_count)
}

/// Count the spans across every `ResourceSpans`/`ScopeSpans` in a batch.
pub fn count_spans(req: &ExportTraceServiceRequest) -> usize {
    req.resource_spans
        .iter()
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len())
        .sum()
}

/// Normalize span timestamps/ids, flatten `req`, and append it as a single WAL frame
/// in the flattened **traces** frame format, returning the number of spans written.
/// The traces analog of [`write_request`].
///
/// `req` is normalized in place first: [`ng_flatten::normalize_span_timestamps`]
/// resolves each span's `start_time_unix_nano`, and [`ng_flatten::normalize_trace_ids`]
/// drops malformed trace/span/parent ids. The request is then flattened
/// ([`ng_flatten::flatten_trace_request`]) and bincode-encoded as the frame payload.
///
/// Unlike [`write_request`] (which calls `fill_log_hashes`), **no hash-fill pass
/// runs** for traces — span `Entry.hash`es
/// stay 0. That only forfeits the seal-time interner fast path (a later seal is
/// slightly slower); it is not a frame-validity requirement. A request with zero
/// spans writes no frame and returns `0`.
pub fn write_trace_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: &mut ExportTraceServiceRequest,
) -> anyhow::Result<usize> {
    let span_count = count_spans(req);
    if span_count == 0 {
        return Ok(0);
    }
    let fallback_base_ns = clock.now_ns().as_u64();
    ng_flatten::normalize_span_timestamps(req, fallback_base_ns);
    let bad_ids = ng_flatten::normalize_trace_ids(req);
    if bad_ids.any() {
        tracing::warn!(
            bad_trace_ids = bad_ids.trace,
            bad_span_ids = bad_ids.span,
            "dropped malformed trace/span ids at ingest (expected {}/{} bytes); stored as zero",
            ng_flatten::TRACE_ID_LEN,
            ng_flatten::SPAN_ID_LEN,
        );
    }
    let flattened = ng_flatten::flatten_trace_request(req);
    let data = ng_flatten::encode_trace_frame(&flattened).context("encode flattened trace frame")?;
    let ingestion_ns = clock.now_ns();
    writer.write_frame(
        PART_KEY,
        &[],
        &data,
        span_count,
        ingestion_ns,
        TimestampNs::ZERO,
        TimestampNs::ZERO,
    )?;
    Ok(span_count)
}
