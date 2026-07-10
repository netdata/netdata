//! Durable-prefix partitioning: chunks + tail of an active WAL.
//!
//! A query that needs an active WAL — one still being written, not
//! yet rotated into a sealed index — works over its durable prefix in
//! two parts: fixed-entry **chunks** (indexed once, cached by the
//! consumer) and a bounded **tail** (re-scanned per query). This
//! module owns the partitioning rule, next to the
//! [`scan_frame_boundaries`](crate::scan_frame_boundaries) scan whose
//! output it folds; it is pure framing math — nothing here knows what
//! the frames contain or what the consumer builds from a chunk.
//!
//! Chunk boundaries are **append-only and immutable**: a boundary is
//! fixed by the frame entry counts up to it, so a durable bound
//! advancing only ever appends new higher-index chunks — it never
//! moves or invalidates an existing one. That is what lets consumers
//! memoize per-chunk artifacts keyed `(wal_seq, chunk_index)`.

use crate::FrameBoundary;

/// One complete chunk of a WAL's durable prefix: a contiguous,
/// frame-aligned byte range carrying at least the threshold's worth of
/// log records (the last frame can push it over).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ChunkBoundary {
    /// 0-based index within the WAL, dense and stable across queries.
    pub index: u32,
    /// The chunk's frame-aligned byte range, `[first frame, last frame end)`.
    pub range: crate::FrameRange,
    /// Log records in the chunk (>= `min_entries`).
    pub entry_count: u64,
}

/// Fold a frame-boundary scan into complete chunks of at least
/// `min_entries` records each.
///
/// `frames` are the boundaries from
/// [`scan_frame_boundaries`](crate::scan_frame_boundaries) over
/// `[start, valid_up_to)`, in file order; `start` is the offset of the
/// first frame (`crate::HEADER_SIZE` for a whole-prefix scan, or a prior
/// chunk's `end`). Each chunk extends to the first frame boundary at or
/// past the running `min_entries`, then a new chunk begins. Frames after
/// the last complete chunk (cumulative `< min_entries`) are **not**
/// returned — they are the *tail*, beginning at
/// `chunks.last().map_or(start, |c| c.range.end())`, and are evaluated per query
/// by the row scan rather than indexed.
///
/// Boundaries are a deterministic function of the entry counts, so a
/// longer prefix yields the same chunks plus possibly more — never a
/// different split of the same data.
pub fn chunk_boundaries(
    frames: &[FrameBoundary],
    start: u64,
    min_entries: u64,
) -> Vec<ChunkBoundary> {
    // `min_entries == 0` would make every frame its own chunk —
    // degenerate, and contrary to the >=16K design intent. The caller's
    // threshold is a config knob, so guard it in debug rather than at
    // runtime cost.
    debug_assert!(min_entries > 0, "min_entries must be positive");

    let mut chunks = Vec::new();
    let mut chunk_start = start;
    let mut acc: u64 = 0;
    for f in frames {
        acc += u64::from(f.entry_count);
        if acc >= min_entries {
            chunks.push(ChunkBoundary {
                index: chunks.len() as u32,
                range: crate::FrameRange::new(chunk_start, f.end_offset),
                entry_count: acc,
            });
            chunk_start = f.end_offset;
            acc = 0;
        }
    }
    chunks
}

/// The byte offset where the tail begins for a chunk list produced by
/// [`chunk_boundaries`] over the same `start`: the end of the last
/// complete chunk, or `start` when there are none.
pub fn tail_start(chunks: &[ChunkBoundary], start: u64) -> u64 {
    chunks.last().map_or(start, |c| c.range.end())
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── chunk_boundaries (pure) ───────────────────────────────────────

    fn fb(end_offset: u64, entry_count: u32) -> FrameBoundary {
        FrameBoundary {
            end_offset,
            entry_count,
        }
    }

    #[test]
    fn groups_frames_into_threshold_chunks() {
        // Threshold 10: frame counts 8,8,8,8 → chunk0 = frames[0,1] (16),
        // chunk1 = frames[2,3] (16). Offsets are the frames' ends.
        let frames = [fb(100, 8), fb(200, 8), fb(300, 8), fb(400, 8)];
        let chunks = chunk_boundaries(&frames, 50, 10);
        assert_eq!(chunks.len(), 2);
        assert_eq!(chunks[0].index, 0);
        assert_eq!(
            (
                chunks[0].range.start(),
                chunks[0].range.end(),
                chunks[0].entry_count
            ),
            (50, 200, 16)
        );
        assert_eq!(chunks[1].index, 1);
        assert_eq!(
            (
                chunks[1].range.start(),
                chunks[1].range.end(),
                chunks[1].entry_count
            ),
            (200, 400, 16)
        );
        // No leftover: the tail starts at the last chunk's end.
        assert_eq!(tail_start(&chunks, 50), 400);
    }

    #[test]
    fn last_frame_pushes_a_chunk_over_the_threshold() {
        // 5,3,7 with threshold 10: cumulative crosses 10 only at frame 2.
        let frames = [fb(100, 5), fb(150, 3), fb(230, 7)];
        let chunks = chunk_boundaries(&frames, 0, 10);
        assert_eq!(chunks.len(), 1);
        assert_eq!(
            (
                chunks[0].range.start(),
                chunks[0].range.end(),
                chunks[0].entry_count
            ),
            (0, 230, 15)
        );
    }

    #[test]
    fn frames_below_threshold_are_all_tail() {
        let frames = [fb(100, 5), fb(150, 3)];
        let chunks = chunk_boundaries(&frames, 64, 10);
        assert!(chunks.is_empty());
        // The whole range is tail, starting where the scan started.
        assert_eq!(tail_start(&chunks, 64), 64);
    }

    #[test]
    fn empty_scan_yields_no_chunks() {
        let chunks = chunk_boundaries(&[], 64, 10);
        assert!(chunks.is_empty());
        assert_eq!(tail_start(&chunks, 64), 64);
    }

    #[test]
    fn longer_prefix_only_appends_chunks() {
        // The first two frames must produce the same chunk whether or not
        // more frames follow — boundaries are append-only.
        let short = [fb(100, 6), fb(200, 6)];
        let long = [fb(100, 6), fb(200, 6), fb(300, 6), fb(400, 6)];
        let c_short = chunk_boundaries(&short, 0, 10);
        let c_long = chunk_boundaries(&long, 0, 10);
        assert_eq!(c_short.len(), 1);
        assert_eq!(c_long.len(), 2);
        assert_eq!(c_short[0], c_long[0]); // identical first chunk
    }
}
