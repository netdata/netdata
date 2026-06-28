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

/// Flatten `req` and append it as a single WAL frame in the flattened-frame format,
/// returning the number of log records written.
///
/// The request is flattened into a per-frame schema tree + typed entries, each
/// entry's `xxhash64(key=value)` is pre-computed (so the downstream SFST build rides
/// the interner's fast path), and the result is bincode-encoded as the frame
/// payload. `log_min/max_ts` are left `ZERO` (no per-frame time-range summary; the
/// real timestamps remain inside each record). A request with zero log records
/// writes no frame and returns `0`.
pub fn write_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: &ExportLogsServiceRequest,
) -> anyhow::Result<usize> {
    let entry_count = count_log_records(req);
    if entry_count == 0 {
        return Ok(0);
    }
    // Records with no time_unix_nano/observed_time_unix_nano get a monotonic
    // ingestion-clock timestamp here, so every record carries a concrete ts.
    let mut flattened = ng_flatten::flatten_request(req, || clock.now_ns().as_u64() as i64);
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
