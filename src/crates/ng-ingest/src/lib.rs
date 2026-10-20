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
            max_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: true,
    }
}

/// Prepare `req` as a flattened frame ([`ng_flatten::prepare_log_frame`]: one
/// normalize walk, then flatten — which hashes each entry at emit time — and
/// bincode-encode) and append it as
/// a single WAL frame, returning the number of log records written. The
/// frame's `log_min/max_ts` carry the resolved timestamp range. A request with
/// zero log records writes no frame and returns `0`.
pub fn write_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: ExportLogsServiceRequest,
) -> anyhow::Result<usize> {
    // One clock tick for the synthetic-timestamp base; normalization then runs
    // lock-free (base + offset for any record lacking event/observed time).
    let fallback_base_ns = clock.now_ns().as_u64();
    // `None` bounds: this dev/bench tool enforces no ingestion time window — the
    // production window is applied only by the otel-ingestor service.
    let frame =
        ng_flatten::prepare_log_frame(req, fallback_base_ns, None).context("prepare flattened frame")?;
    // `ts_range` is None exactly when the request has no records.
    if frame.ts_range.is_none() {
        return Ok(0);
    }
    writer.write_frame(
        PART_KEY,
        &[],
        &frame.data,
        wal::FrameMeta {
            entry_count: frame.records,
            ingestion_ns: clock.now_ns(),
            log_ts_range: frame
                .ts_range
                .map(|(min, max)| (TimestampNs(min), TimestampNs(max))),
        },
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

/// Prepare `req` as a flattened **traces** frame
/// ([`ng_flatten::prepare_trace_frame`]: one normalize walk, then flatten —
/// which hashes each entry at emit time — and bincode-encode) and append it as
/// a single WAL frame, returning the number of spans written. The traces
/// analog of [`write_request`]. The frame's `log_min/max_ts` carry the
/// resolved span-start time range. A request with zero spans writes no frame
/// and returns `0`.
pub fn write_trace_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: ExportTraceServiceRequest,
) -> anyhow::Result<usize> {
    // One clock tick for the synthetic-timestamp base; normalization then runs
    // lock-free (base + offset for any span lacking a start time).
    let fallback_base_ns = clock.now_ns().as_u64();
    // `None` bounds: this dev/bench tool enforces no ingestion time window — the
    // production window is applied only by the otel-ingestor service.
    let frame = ng_flatten::prepare_trace_frame(req, fallback_base_ns, None)
        .context("prepare flattened trace frame")?;
    // `ts_range` is None exactly when the request has no spans.
    if frame.ts_range.is_none() {
        return Ok(0);
    }
    writer.write_frame(
        PART_KEY,
        &[],
        &frame.data,
        wal::FrameMeta {
            entry_count: frame.records,
            ingestion_ns: clock.now_ns(),
            log_ts_range: frame
                .ts_range
                .map(|(min, max)| (TimestampNs(min), TimestampNs(max))),
        },
    )?;
    Ok(frame.records)
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
