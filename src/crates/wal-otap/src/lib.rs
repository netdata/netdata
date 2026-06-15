//! OTAP payload decoding for WAL frames: bytes → OTel log rows.
//!
//! The boundary crate between the payload-agnostic [`wal`] framing
//! layer and every consumer that needs the log *rows* inside the
//! frames. Two consumers exist today — the SFST indexer (builds an
//! inverted index from the rows) and the query-time WAL row scan
//! (evaluates queries over them directly) — and this crate is the
//! reason they can never disagree about **what rows a frame
//! contains**: both receive rows from the same `decode_frame` (the
//! crate-internal core that [`decode_file`] / [`decode_range`] drive),
//! differing only in how they evaluate them.
//!
//! Rows are delivered through the [`KvSink`] trait rather than as
//! materialized values, so each consumer keeps its own interning /
//! dedup strategy and the producer-side `_nd_kv_hash` fast path stays
//! intact (see [`KvSink::lookup_hash`]).
//!
//! [`decode_file`] / [`decode_range`] own the open-and-drain loop over
//! a WAL file (or a frame-aligned byte range of one), so consumers
//! don't each hand-write the same reader loop.

mod arrow_columns;
mod decode;
mod otap_frame;

pub use decode::KvSink;

use decode::decode_frame;

use std::path::Path;

/// Failure decoding a frame's OTAP payload into rows.
#[derive(Debug, thiserror::Error)]
pub enum DecodeError {
    /// Arrow IPC parsing failed while decoding an OTAP sub-stream
    /// (schema message, record batch, or column data).
    #[error("Arrow IPC error: {0}")]
    Arrow(#[from] arrow::error::ArrowError),

    /// An OTAP sub-stream ran out of bytes mid-header or mid-payload:
    /// the 1-byte tag + 4-byte length prefix was incomplete, or the
    /// declared length pointed past the end of the frame.
    #[error("truncated OTAP frame")]
    TruncatedOtapFrame,

    /// An OTAP sub-stream's 1-byte tag didn't map to any known
    /// [`ArrowPayloadType`](otap_df_pdata::proto::opentelemetry::arrow::v1::ArrowPayloadType).
    /// Usually means a newer protocol version produced a payload this
    /// build doesn't recognize.
    #[error("unknown OTAP payload type tag: {0}")]
    UnknownOtapTag(i32),
}

/// Failure reading-and-decoding a WAL file's rows: either the WAL
/// layer refused the bytes (framing, CRC, truncation) or a frame's
/// payload didn't decode.
#[derive(Debug, thiserror::Error)]
pub enum ReadError {
    #[error("WAL read failed: {0}")]
    Wal(#[from] wal::Error),

    #[error("frame decode failed: {0}")]
    Decode(#[from] DecodeError),
}

/// Counts from one decode pass — producers log these.
#[derive(Debug, Clone, Copy, Default)]
pub struct DecodeStats {
    /// Frames read from the WAL.
    pub frames: u64,
    /// Log rows delivered to the sink.
    pub rows: u64,
}

/// Decode every frame of the WAL file at `path` into `sink`.
pub fn decode_file<S: KvSink>(path: &Path, sink: &mut S) -> Result<DecodeStats, ReadError> {
    drain(&mut wal::Reader::open(path)?, sink)
}

/// Decode the frames in a frame-aligned byte `range` of the WAL file
/// at `path` into `sink` — the active-WAL query path; see
/// [`wal::FrameRange`] / [`wal::Reader::open_range`] for the
/// frame-boundary and durable-prefix soundness checks.
pub fn decode_range<S: KvSink>(
    path: &Path,
    range: wal::FrameRange,
    sink: &mut S,
) -> Result<DecodeStats, ReadError> {
    drain(&mut wal::Reader::open_range(path, range)?, sink)
}

fn drain<S: KvSink>(reader: &mut wal::Reader, sink: &mut S) -> Result<DecodeStats, ReadError> {
    let mut stats = DecodeStats::default();
    while let Some(frame) = reader.next_frame()? {
        stats.frames += 1;
        stats.rows += decode_frame(&frame, sink)? as u64;
    }
    Ok(stats)
}
