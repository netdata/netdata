mod config;
mod error;
mod format;
pub mod prefix;
mod reader;
pub mod registry;
mod seq;
mod writer;

pub use config::{Config, RotationConfig};
pub use error::{Error, Result};
/// Byte offset of the first frame — the lower bound for
/// [`Reader::open_range`] `start` and the start of a whole-prefix read.
pub use format::HEADER_SIZE;
/// Maximum size of a WAL file's opaque `content_meta` identity blob — a
/// per-file/per-partition header field, the same for every frame written to
/// that file; validation merely happens at the [`Writer::write_frame`]
/// boundary. The substrate hard-rejects a larger blob there; the producer
/// (content plane) MUST validate against this before writing so an oversized
/// identity drops a single frame rather than failing the batch.
pub use format::MAX_CONTENT_META_BYTES;
pub use format::{FileEvent, Message};
pub use reader::{Frame, FrameBoundary, FrameRange, Reader, scan_frame_boundaries};
pub use registry::{File, Registry};
pub use seq::{DEFAULT_RESERVE_BATCH, SeqAllocator, read_seq_highwater, write_seq_highwater};
pub use writer::{FileStamp, FrameMeta, Writer};

/// Deterministic opaque partition key for tests. The WAL treats `part_key` as
/// an opaque `u64` and never decodes it, so tests fabricate distinct keys per
/// logical stream without depending on the content-plane identity codec —
/// same label → same key, different label → (almost surely) different key.
#[cfg(test)]
pub(crate) fn opaque_part_key(namespace: &str, name: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    namespace.hash(&mut h);
    name.hash(&mut h);
    h.finish()
}

/// Highest WAL sequence on disk across every tenant subdir of `base`.
/// Returns `0` when `base` is missing or empty. Used at process
/// startup to seed the seq counter so it stays monotonic across
/// restarts.
pub fn scan_max_sequence_recursive(base: &std::path::Path) -> std::io::Result<u64> {
    file_registry::scan_max_sequence_recursive(base, registry::WAL_EXT)
}
