//! Tests for [`WalPrefixes`] and the chunk prebuilder over real traces
//! WALs written by the shared fixtures.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
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

/// Whether chunk `index` of WAL `seq` is cached: an erring build
/// succeeds only on a hit.
async fn cached(cache: &ChunkCache, seq: u64, index: u32) -> bool {
    cache
        .get_or_build(seq, index, async {
            Err::<Arc<BuiltChunk>, &str>("not built")
        })
        .await
        .is_ok()
}

/// Wait until `chunks` of WAL `seq` are all cached (`true`) or all gone
/// (`false`).
async fn await_cached(cache: &ChunkCache, seq: u64, chunks: &[ChunkBoundary], want: bool) {
    assert!(!chunks.is_empty());
    let deadline = tokio::time::Instant::now() + Duration::from_secs(10);
    for chunk in chunks {
        while cached(cache, seq, chunk.index).await != want {
            assert!(
                tokio::time::Instant::now() < deadline,
                "chunk {} of WAL {seq} never became cached={want}",
                chunk.index
            );
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    }
}

fn spawn_prebuilder(
    prefixes: &Arc<WalPrefixes>,
    seal: ChunkSealFn,
    cache: &Arc<ChunkCache>,
    cancel: &CancellationToken,
) -> ChunkPrebuilder {
    ChunkPrebuilder::spawn(
        Arc::clone(prefixes),
        seal,
        Arc::clone(cache),
        cancel.clone(),
    )
}

#[tokio::test]
async fn the_prebuilder_builds_new_chunks_on_its_own() {
    let (path, frames) = five_frame_wal(4).await;
    let end = frames.last().unwrap().end_offset;
    let prefixes = Arc::new(WalPrefixes::new(3));
    let cache = Arc::new(ChunkCache::new(64 * 1024 * 1024));
    let cancel = CancellationToken::new();
    let prebuilder = spawn_prebuilder(
        &prefixes,
        ng_index::build_sfst_traces_range,
        &cache,
        &cancel,
    );
    prebuilder.synced(4, path, end);
    await_cached(&cache, 4, &folded_up_to(&frames, end), true).await;
    cancel.cancel();
}

static SEALS: AtomicUsize = AtomicUsize::new(0);

/// The traces range seal, counted (only one test uses it).
fn counting_seal(
    path: &Path,
    range: wal::FrameRange,
) -> Result<(sfst::Summary, Vec<u8>), ng_index::Error> {
    SEALS.fetch_add(1, Ordering::SeqCst);
    ng_index::build_sfst_traces_range(path, range)
}

#[tokio::test]
async fn the_prebuilder_builds_each_chunk_once() {
    let (path, frames) = five_frame_wal(5).await;
    let (other, other_frames) = five_frame_wal(6).await;
    let (end, other_end) = (
        frames.last().unwrap().end_offset,
        other_frames.last().unwrap().end_offset,
    );
    let (chunks, other_chunks) = (
        folded_up_to(&frames, end),
        folded_up_to(&other_frames, other_end),
    );
    let prefixes = Arc::new(WalPrefixes::new(3));
    let cache = Arc::new(ChunkCache::new(64 * 1024 * 1024));
    let cancel = CancellationToken::new();
    let prebuilder = spawn_prebuilder(&prefixes, counting_seal, &cache, &cancel);

    prebuilder.synced(5, path.clone(), end);
    await_cached(&cache, 5, &chunks, true).await;
    assert_eq!(SEALS.load(Ordering::SeqCst), chunks.len());

    // A later sync does not rebuild a chunk the cache dropped; a query
    // builds it on demand. WALs are served in arrival order, so once
    // WAL 6's chunks are cached, WAL 5's second sync was handled.
    cache.drop_seq(5).await;
    prebuilder.synced(5, path, end);
    prebuilder.synced(6, other, other_end);
    await_cached(&cache, 6, &other_chunks, true).await;
    assert_eq!(
        SEALS.load(Ordering::SeqCst),
        chunks.len() + other_chunks.len()
    );
    assert!(!cached(&cache, 5, chunks[0].index).await);
    cancel.cancel();
}

#[tokio::test]
async fn forgetting_a_wal_drops_its_boundaries_and_chunks() {
    let (path, frames) = five_frame_wal(7).await;
    let end = frames.last().unwrap().end_offset;
    let chunks = folded_up_to(&frames, end);
    let prefixes = Arc::new(WalPrefixes::new(3));
    let cache = Arc::new(ChunkCache::new(64 * 1024 * 1024));
    let cancel = CancellationToken::new();
    let prebuilder = spawn_prebuilder(
        &prefixes,
        ng_index::build_sfst_traces_range,
        &cache,
        &cancel,
    );
    prebuilder.synced(7, path, end);
    await_cached(&cache, 7, &chunks, true).await;

    // The boundaries go first, then the chunks: once the chunks are
    // gone, so is the WAL's entry.
    prebuilder.forget(7);
    await_cached(&cache, 7, &chunks, false).await;
    assert!(prefixes.entries.lock().unwrap().is_empty());
    cancel.cancel();
}

#[tokio::test]
async fn a_forget_right_behind_a_sync_leaves_nothing() {
    // Whether the two land in one batch or the sync is built first, the
    // forget wins: WAL 8 ends with no boundaries and no chunks.
    let (path, frames) = five_frame_wal(8).await;
    let (other, other_frames) = five_frame_wal(9).await;
    let (end, other_end) = (
        frames.last().unwrap().end_offset,
        other_frames.last().unwrap().end_offset,
    );
    let chunks = folded_up_to(&frames, end);
    let prefixes = Arc::new(WalPrefixes::new(3));
    let cache = Arc::new(ChunkCache::new(64 * 1024 * 1024));
    let cancel = CancellationToken::new();
    let prebuilder = spawn_prebuilder(
        &prefixes,
        ng_index::build_sfst_traces_range,
        &cache,
        &cancel,
    );
    prebuilder.synced(8, path, end);
    prebuilder.forget(8);
    // Served in arrival order: once WAL 9 is built, WAL 8 was handled.
    prebuilder.synced(9, other, other_end);
    await_cached(&cache, 9, &folded_up_to(&other_frames, other_end), true).await;
    assert!(!cached(&cache, 8, chunks[0].index).await);
    let seqs: Vec<u64> = prefixes.entries.lock().unwrap().keys().copied().collect();
    assert_eq!(seqs, [9]);
    cancel.cancel();
}
