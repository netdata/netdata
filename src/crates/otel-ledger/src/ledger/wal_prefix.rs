//! Chunk boundaries of active WALs, kept current across queries.
//!
//! A query over an active WAL needs its chunk boundaries up to the
//! snapshot's `valid_up_to` (`wal::prefix`). Scanning the frame headers
//! from the start on every request repeats work: boundaries are
//! append-only, so [`WalPrefixes`] keeps one [`wal::prefix::ChunkFold`]
//! per WAL and scans only the frames written since the last call. Both
//! the request path and the chunk prebuilder (`super::prebuild`) read it.
//!
//! [`build_chunk`] is the one chunk build both of them hand to the shared
//! `ChunkCache`, so a request and the prebuilder racing for one chunk
//! join the same singleflight build.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use file_lifecycle::chunk::BuiltChunk;
use wal::prefix::{ChunkBoundary, ChunkFold};

/// Seals one WAL byte range into an SFST image (the signal's range seal,
/// e.g. `ng_index::build_sfst_traces_range`).
pub(crate) type ChunkSealFn =
    fn(&Path, wal::FrameRange) -> Result<(sfst::Summary, Vec<u8>), ng_index::Error>;

/// One WAL's fold and how far its frames were scanned.
struct WalPrefix {
    scanned_end: u64,
    fold: ChunkFold,
}

/// Per-WAL chunk boundaries for one signal, keyed by the WAL's `seq`
/// (process-global, so it names one WAL). See the module docs.
pub(crate) struct WalPrefixes {
    min_entries: u64,
    entries: std::sync::Mutex<HashMap<u64, Arc<tokio::sync::Mutex<WalPrefix>>>>,
}

impl WalPrefixes {
    /// `min_entries` is the signal's chunk threshold.
    pub(crate) fn new(min_entries: u64) -> Self {
        Self {
            min_entries,
            entries: std::sync::Mutex::new(HashMap::new()),
        }
    }

    /// The complete chunks of WAL `seq` (at `path`) whose ranges end at or
    /// before `valid_up_to`, in index order. Scans only the frames past
    /// what an earlier call already folded; callers of one WAL serialize
    /// on its entry, so the scan runs once.
    pub(crate) async fn chunks(
        &self,
        seq: u64,
        path: &Path,
        valid_up_to: u64,
    ) -> Result<Vec<ChunkBoundary>, String> {
        let entry = Arc::clone(self.entries.lock().unwrap().entry(seq).or_insert_with(|| {
            Arc::new(tokio::sync::Mutex::new(WalPrefix {
                scanned_end: wal::HEADER_SIZE as u64,
                fold: ChunkFold::default(),
            }))
        }));
        let mut prefix = entry.lock().await;
        if prefix.scanned_end < valid_up_to {
            let from = prefix.scanned_end;
            let scan_path: PathBuf = path.to_owned();
            let frames = tokio::task::spawn_blocking(move || {
                wal::scan_frame_boundaries(&scan_path, wal::FrameRange::new(from, valid_up_to))
            })
            .await
            .map_err(|e| format!("boundary scan task: {e}"))?
            .map_err(|e| format!("boundary scan: {e}"))?;
            let end = frames.last().map_or(from, |f| f.end_offset);
            prefix
                .fold
                .extend(&frames, wal::HEADER_SIZE as u64, self.min_entries);
            prefix.scanned_end = end;
        }
        Ok(prefix
            .fold
            .chunks()
            .iter()
            .filter(|c| c.range.end() <= valid_up_to)
            .copied()
            .collect())
    }

    /// Drop WAL `seq`'s entry — when its chunks are dropped, on rotation.
    /// A request racing the rotation can re-create it; that entry is a
    /// few boundaries, kept until restart, like the chunk cache's own
    /// rotation race.
    pub(crate) fn forget(&self, seq: u64) {
        self.entries.lock().unwrap().remove(&seq);
    }
}

/// Build `chunk` of the WAL at `path` with `seal` on a blocking thread,
/// cross-checking the sealed record count against the scan's.
pub(crate) async fn build_chunk(
    seal: ChunkSealFn,
    path: PathBuf,
    chunk: ChunkBoundary,
) -> Result<Arc<BuiltChunk>, String> {
    let (range, expected) = (chunk.range, chunk.entry_count);
    match tokio::task::spawn_blocking(move || seal(&path, range)).await {
        Ok(Ok((summary, bytes))) => {
            if u64::from(summary.record_count) != expected {
                Err(format!(
                    "chunk record count {} != expected {expected}",
                    summary.record_count
                ))
            } else {
                Ok(Arc::new(BuiltChunk {
                    summary,
                    bytes: Arc::new(bytes),
                }))
            }
        }
        Ok(Err(e)) => Err(format!("chunk seal: {e}")),
        Err(e) => Err(format!("chunk build task: {e}")),
    }
}

#[cfg(test)]
mod tests;
