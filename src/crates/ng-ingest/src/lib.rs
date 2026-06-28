//! Core of the `ng-ingest` receiver: flatten one OTLP export request and append it
//! as one WAL frame in the flattened-frame format (see `ng-flatten`).

use anyhow::Context as _;
use file_registry::{ByteSize, MonotonicClock, TimestampNs};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;

/// Every frame goes to one logical stream, hence one WAL file. The WAL treats
/// `part_key` as opaque; a single constant keeps everything in one file.
pub const PART_KEY: u64 = 0;

/// The opaque signal axis stamped into the file id (logs == 0). The WAL ascribes
/// no meaning to it here; it only labels the on-disk file name.
pub const PIPELINE_ID: u16 = 0;

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

/// OTLP/W3C fixed id widths (`opentelemetry/proto/logs/v1/logs.proto`): a
/// `trace_id` is 16 bytes, a `span_id` is 8 bytes, each empty if unset.
const TRACE_ID_LEN: usize = 16;
const SPAN_ID_LEN: usize = 8;

/// How many malformed ids a single export request carried (wrong-length, non-empty).
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
struct MalformedIds {
    trace: u64,
    span: u64,
}

impl MalformedIds {
    fn any(self) -> bool {
        self.trace > 0 || self.span > 0
    }
}

/// Drop malformed trace/span ids at the ingest boundary. A non-empty id whose
/// length is not the spec width is **cleared** (set to absent, which the SFST
/// column later stores as the all-zero "unset/invalid" sentinel); conformant ids
/// (16/8 bytes) and absent ids (empty) pass through untouched. Returns the counts
/// so the caller can log one aggregated warning per request (avoids per-record
/// log floods).
fn normalize_ids(req: &mut ExportLogsServiceRequest) -> MalformedIds {
    let mut bad = MalformedIds::default();
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if !r.trace_id.is_empty() && r.trace_id.len() != TRACE_ID_LEN {
                    r.trace_id.clear();
                    bad.trace += 1;
                }
                if !r.span_id.is_empty() && r.span_id.len() != SPAN_ID_LEN {
                    r.span_id.clear();
                    bad.span += 1;
                }
            }
        }
    }
    bad
}

/// Ensure every log record has a usable `time_unix_nano` before flattening, applying
/// the OTLP single-timestamp rule (time else observed) at the observation point:
/// keep `time_unix_nano` if set, else fall back to `observed_time_unix_nano`, else
/// stamp the monotonic ingestion clock.
///
/// Notes: the resolved value is written into `time_unix_nano` (not
/// `observed_time_unix_nano`). The clock fallback is a per-record `now_ns()` —
/// strictly increasing, so intra-frame order is preserved, but it is not `wal-otap`'s
/// `ingestion_ns + row_offset` (a different value, same ordering guarantee).
fn normalize_timestamps(req: &mut ExportLogsServiceRequest, clock: &mut MonotonicClock) {
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if r.time_unix_nano == 0 {
                    r.time_unix_nano = if r.observed_time_unix_nano != 0 {
                        r.observed_time_unix_nano
                    } else {
                        clock.now_ns().as_u64()
                    };
                }
            }
        }
    }
}

/// Normalize timestamps and ids, flatten `req`, and append it as a single WAL frame
/// in the flattened-frame format, returning the number of log records written.
///
/// `req` is normalized **in place** first: [`normalize_timestamps`] resolves each
/// record's `time_unix_nano`, and [`normalize_ids`] drops malformed trace/span ids
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
    normalize_timestamps(req, clock);
    let bad_ids = normalize_ids(req);
    if bad_ids.any() {
        tracing::warn!(
            bad_trace_ids = bad_ids.trace,
            bad_span_ids = bad_ids.span,
            "dropped malformed trace/span ids at ingest \
             (expected {TRACE_ID_LEN}/{SPAN_ID_LEN} bytes); stored as zero",
        );
    }
    let mut flattened = ng_flatten::flatten_request(req);
    ng_flatten::fill_hashes(&mut flattened);
    let data = ng_flatten::encode_frame(&flattened).context("encode flattened frame")?;
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
