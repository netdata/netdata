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

/// Prepare `req` as a flattened frame ([`ng_flatten::prepare_log_frame`]: one
/// normalize walk, flatten, hash pre-compute, bincode-encode) and append it as
/// a single WAL frame, returning the number of log records written. The
/// frame's `log_min/max_ts` carry the resolved timestamp range. A request with
/// zero log records writes no frame and returns `0`.
pub fn write_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: &mut ExportLogsServiceRequest,
) -> anyhow::Result<usize> {
    // One clock tick for the synthetic-timestamp base; normalization then runs
    // lock-free (base + offset for any record lacking event/observed time).
    let fallback_base_ns = clock.now_ns().as_u64();
    let frame =
        ng_flatten::prepare_log_frame(req, fallback_base_ns).context("prepare flattened frame")?;
    // `ts_range` is None exactly when the request has no records.
    let Some((min_ts, max_ts)) = frame.ts_range else {
        return Ok(0);
    };
    let ingestion_ns = clock.now_ns();
    writer.write_frame(
        PART_KEY,
        &[],
        &frame.data,
        frame.records,
        ingestion_ns,
        TimestampNs(min_ts),
        TimestampNs(max_ts),
    )?;
    Ok(frame.records)
}

/// Append `req` to a request-dump stream as `u32-LE length + prost bytes` —
/// the `--dump-requests` corpus format. Requests are dumped pristine (before
/// ingest normalization) and in frame order, so a dump captured alongside a
/// WAL pairs with it entry-for-entry. Readers reverse this framing (the
/// ingest bench in `ng-flatten` carries its own copy of the trivial reader).
pub fn append_dumped_request(
    w: &mut impl std::io::Write,
    req: &ExportLogsServiceRequest,
) -> std::io::Result<()> {
    use prost::Message;
    let bytes = req.encode_to_vec();
    w.write_all(&(bytes.len() as u32).to_le_bytes())?;
    w.write_all(&bytes)
}

/// Decode a whole request-dump stream (the [`append_dumped_request`] format)
/// back into requests. Fails on a truncated tail or undecodable payload.
pub fn read_dumped_requests(mut bytes: &[u8]) -> anyhow::Result<Vec<ExportLogsServiceRequest>> {
    use prost::Message;
    let mut out = Vec::new();
    while !bytes.is_empty() {
        anyhow::ensure!(bytes.len() >= 4, "truncated length prefix");
        let len = u32::from_le_bytes(bytes[..4].try_into().unwrap()) as usize;
        bytes = &bytes[4..];
        anyhow::ensure!(bytes.len() >= len, "truncated request payload");
        out.push(ExportLogsServiceRequest::decode(&bytes[..len])?);
        bytes = &bytes[len..];
    }
    Ok(out)
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
/// ([`ng_flatten::flatten_trace_request`]), each entry's `xxhash64(key=value)` is
/// pre-computed at emit time by the flattener (exactly as the logs path), and
/// the result is bincode-encoded as the frame payload — so
/// the seal rides the interner's fast path. A request with zero spans writes no
/// frame and returns `0`.
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
    let (flattened, sanitized_keys) = ng_flatten::flatten_trace_request(req);
    if sanitized_keys > 0 {
        tracing::warn!(
            sanitized_keys,
            "rewrote '=' to '_' in attribute keys at ingest ('=' is the key=value delimiter)",
        );
    }
    let data =
        ng_flatten::encode_trace_frame(&flattened).context("encode flattened trace frame")?;
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

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};

    #[test]
    fn dumped_requests_round_trip() {
        let req = |n: usize| ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: (0..n)
                        .map(|i| LogRecord {
                            time_unix_nano: 1 + i as u64,
                            ..Default::default()
                        })
                        .collect(),
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let mut buf = Vec::new();
        append_dumped_request(&mut buf, &req(3)).unwrap();
        append_dumped_request(&mut buf, &req(1)).unwrap();
        let back = read_dumped_requests(&buf).unwrap();
        assert_eq!(back.len(), 2);
        assert_eq!(back[0], req(3));
        assert_eq!(back[1], req(1));
        assert!(read_dumped_requests(&buf[..buf.len() - 1]).is_err());
    }
}
