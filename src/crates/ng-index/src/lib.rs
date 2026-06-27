//! Open a WAL file produced by `ng-ingest` and report stats over its frames.
//! Independent of `ng-ingest`: it reads via the `wal` crate and decodes each
//! frame payload as OTLP protobuf.

use std::path::Path;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

/// Stats gathered from one WAL file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct WalStats {
    /// Number of frames read.
    pub frames: u64,
    /// Total log records counted by decoding each frame's OTLP payload.
    pub records: u64,
    /// Total log records claimed by the frame headers (`entry_count`). Equals
    /// `records` for an intact file; a mismatch signals corruption or an
    /// encoding the reader does not understand.
    pub header_records: u64,
}

impl WalStats {
    /// Whether the decoded record count agrees with the frame headers.
    pub fn consistent(&self) -> bool {
        self.records == self.header_records
    }
}

/// Count the log records in one decoded OTLP batch. A private copy (this crate
/// shares no code with `ng-ingest`).
fn count_log_records(req: &ExportLogsServiceRequest) -> usize {
    req.resource_logs
        .iter()
        .flat_map(|rl| rl.scope_logs.iter())
        .map(|sl| sl.log_records.len())
        .sum()
}

/// Errors reading or decoding a WAL file.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("wal read failed: {0}")]
    Wal(#[from] wal::Error),
    #[error("frame {frame} payload is not a valid OTLP ExportLogsServiceRequest: {source}")]
    Decode {
        frame: u64,
        source: prost::DecodeError,
    },
}

/// Open a WAL file and tally its frames and log records.
///
/// `wal::Reader` decompresses LZ4 frames transparently, so each payload decodes
/// directly as an [`ExportLogsServiceRequest`].
pub fn count_wal(path: &Path) -> Result<WalStats, Error> {
    let mut reader = wal::Reader::open(path)?;
    let mut stats = WalStats::default();
    while let Some(frame) = reader.next_frame()? {
        stats.frames += 1;
        stats.header_records += frame.entry_count as u64;
        let req = ExportLogsServiceRequest::decode(frame.data).map_err(|source| Error::Decode {
            frame: stats.frames,
            source,
        })?;
        stats.records += count_log_records(&req) as u64;
    }
    Ok(stats)
}
