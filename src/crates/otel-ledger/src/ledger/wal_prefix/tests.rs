//! Tests for [`WalPrefixes`] and the chunk prebuilder over real traces
//! WALs written by the shared fixtures.

use std::sync::Arc;
use std::time::Duration;

use file_lifecycle::chunk::ChunkCache;
use tokio_util::sync::CancellationToken;
use wal::prefix::chunk_boundaries;

use super::*;
use crate::ledger::prebuild::ChunkPrebuilder;
use crate::ledger::rpc::traces_fixtures::{install_wal, make_registries, otlp_req};

/// A traces WAL of five frames (1, 2, 3, 1, 2 spans): with a threshold of
/// 3 it folds into chunks plus a tail. Returns its path and the frames.
async fn five_frame_wal(seq: u64) -> (PathBuf, Vec<wal::FrameBoundary>) {
    let registries = make_registries();
    let reqs = [1u8, 2, 3, 1, 2]
        .iter()
        .enumerate()
        .map(|(i, &n)| otlp_req(0x10 + i as u8, n, (i as u64 + 1) * 1_000_000_000))
        .collect();
    let path = install_wal(&registries, "default", seq, reqs).await;
    let len = std::fs::metadata(&path).unwrap().len();
    let frames =
        wal::scan_frame_boundaries(&path, wal::FrameRange::new(wal::HEADER_SIZE as u64, len))
            .unwrap();
    assert_eq!(frames.len(), 5);
    (path, frames)
}

/// The chunks one whole fold yields up to `bound`.
fn folded_up_to(frames: &[wal::FrameBoundary], bound: u64) -> Vec<ChunkBoundary> {
    let within: Vec<_> = frames
        .iter()
        .copied()
        .filter(|f| f.end_offset <= bound)
        .collect();
    chunk_boundaries(&within, wal::HEADER_SIZE as u64, 3)
}

#[tokio::test]
async fn growing_bounds_fold_like_one_scan() {
    let (path, frames) = five_frame_wal(1).await;
    let prefixes = WalPrefixes::new(3);
    // Every bound in turn, as successive syncs would deliver them; each
    // answer equals a whole fold up to that bound.
    for f in &frames {
        let chunks = prefixes.chunks(1, &path, f.end_offset).await.unwrap();
        assert_eq!(chunks, folded_up_to(&frames, f.end_offset), "{f:?}");
    }
    // An older snapshot's bound, after the index went further, still sees
    // only its own chunks.
    let early = frames[2].end_offset;
    let chunks = prefixes.chunks(1, &path, early).await.unwrap();
    assert_eq!(chunks, folded_up_to(&frames, early));
}

#[tokio::test]
async fn a_forgotten_wal_is_scanned_again() {
    let (path, frames) = five_frame_wal(2).await;
    let end = frames.last().unwrap().end_offset;
    let prefixes = WalPrefixes::new(3);
    let before = prefixes.chunks(2, &path, end).await.unwrap();
    prefixes.forget(2);
    assert!(prefixes.entries.lock().unwrap().is_empty());
    assert_eq!(prefixes.chunks(2, &path, end).await.unwrap(), before);
}

#[tokio::test]
async fn built_chunks_carry_their_records_and_summary() {
    let (path, frames) = five_frame_wal(3).await;
    let end = frames.last().unwrap().end_offset;
    let chunk = WalPrefixes::new(3).chunks(3, &path, end).await.unwrap()[0];
    let built = build_chunk(ng_index::build_sfst_traces_range, path, chunk)
        .await
        .unwrap();
    assert_eq!(u64::from(built.summary.record_count), chunk.entry_count);
    assert_eq!(
        sfst::IndexReader::open(&built.bytes).unwrap().summary(),
        &built.summary
    );
}

#[tokio::test]
async fn the_prebuilder_builds_new_chunks_on_its_own() {
    let (path, frames) = five_frame_wal(4).await;
    let end = frames.last().unwrap().end_offset;
    let prefixes = Arc::new(WalPrefixes::new(3));
    let cache = Arc::new(ChunkCache::new(64 * 1024 * 1024));
    let cancel = CancellationToken::new();
    let prebuilder = ChunkPrebuilder::spawn(
        Arc::clone(&prefixes),
        ng_index::build_sfst_traces_range,
        Arc::clone(&cache),
        cancel.clone(),
    );
    prebuilder.synced(4, path, end);

    // An erring build succeeds only on a cache hit: every chunk the
    // prebuilder built answers without it.
    let expected = folded_up_to(&frames, end);
    assert!(!expected.is_empty());
    let deadline = tokio::time::Instant::now() + Duration::from_secs(10);
    for chunk in &expected {
        loop {
            let probe = cache
                .get_or_build(4, chunk.index, async {
                    Err::<Arc<BuiltChunk>, &str>("not built")
                })
                .await;
            if probe.is_ok() {
                break;
            }
            assert!(
                tokio::time::Instant::now() < deadline,
                "chunk {} was never prebuilt",
                chunk.index
            );
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    }
    cancel.cancel();
}
