//! Shared, testable core for the `ng-index` OTLP→WAL receiver (see `main.rs` for
//! the binary and the experiment context).
//!
//! The reusable unit is "turn one OTLP export request into one WAL frame": the
//! payload is the request re-encoded as protobuf bytes — the simplest faithful
//! encoding for step 1. The richer type-preserving encoding is a later step.

use file_registry::{ByteSize, MonotonicClock, TimestampNs};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

/// Every frame goes to one logical stream, hence one WAL file. The WAL treats
/// `part_key` as opaque; a single constant keeps the experiment to one file.
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

/// One WAL file, no rotation: a single `part_key` plus rotation thresholds set to
/// effectively infinite so the writer never starts a second file.
pub fn one_file_config(compress: bool) -> wal::Config {
    wal::Config {
        rotation: wal::RotationConfig {
            max_log_entries: usize::MAX,
            max_file_size: ByteSize(u64::MAX),
            max_duration: None,
        },
        crc_enabled: true,
        compression_enabled: compress,
    }
}

/// Encode `req` as protobuf and append it as a single WAL frame, returning the
/// number of log records written.
///
/// `log_min/max_ts` are left `ZERO` for step 1 — there is no querying yet, so the
/// per-frame time-range summary is not needed (the real timestamps stay inside
/// each record in the payload regardless). A request with zero log records writes
/// no frame and returns `0`.
pub fn write_request(
    writer: &mut wal::Writer,
    clock: &mut MonotonicClock,
    req: &ExportLogsServiceRequest,
) -> wal::Result<usize> {
    let entry_count = count_log_records(req);
    if entry_count == 0 {
        return Ok(0);
    }
    let data = req.encode_to_vec();
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
