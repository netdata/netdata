mod config;
mod error;
mod format;
mod reader;
pub mod prefix;
pub mod registry;
mod seq;
mod writer;

pub use config::{Config, RotationConfig};
pub use error::{Error, Result};
pub use seq::{DEFAULT_RESERVE_BATCH, SeqAllocator, read_seq_highwater, write_seq_highwater};
/// Byte offset of the first frame — the lower bound for
/// [`Reader::open_range`] `start` and the start of a whole-prefix read.
pub use format::HEADER_SIZE;
pub use format::{FileEvent, Message};
pub use reader::{Frame, FrameBoundary, FrameRange, Reader, scan_frame_boundaries};
pub use registry::{File, Registry};
pub use writer::Writer;

/// Highest WAL sequence on disk across every tenant subdir of `base`.
/// Returns `0` when `base` is missing or empty. Used at process
/// startup to seed the seq counter so it stays monotonic across
/// restarts.
pub fn scan_max_sequence_recursive(base: &std::path::Path) -> std::io::Result<u64> {
    file_registry::scan_max_sequence_recursive(base, registry::WAL_EXT)
}
