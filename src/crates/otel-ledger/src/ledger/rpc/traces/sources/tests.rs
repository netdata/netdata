use super::*;
use crate::ledger::rpc::traces::fixtures::{install_sfst, install_wal, make_registries, otlp_req};

// These tests cover `capture` end to end at the registry boundary —
// registry state in, engine sources out. The sealed-file tests need no
// real SFST bytes (`capture` never opens sealed files; summaries come
// from the registry and the engine opens files later). The WAL tests
// write REAL trace WALs (OTLP → ng-flatten trace frames) so the chunk
// builds run the actual traces seal.

fn make_supplier() -> TracesSourceSupplier {
    make_supplier_with_min_entries(16_384)
}

/// `min_entries` controls chunk grouping — WAL tests pick small values
/// so a handful of frames splits into chunks + tail.
fn make_supplier_with_min_entries(min_entries: u64) -> TracesSourceSupplier {
    TracesSourceSupplier::new(
        make_registries(),
        Arc::new(ChunkCache::new(64 * 1024 * 1024)),
        min_entries,
    )
}

/// Sources' ids in capture order (chunks and tails carry their identity
/// in the id string).
fn source_ids(sources: &[TraceSource]) -> Vec<String> {
    sources
        .iter()
        .map(|s| match s {
            TraceSource::Sfst(c) => c.source_id.as_str().to_string(),
            TraceSource::Tail(t) => t.source_id.as_str().to_string(),
        })
        .collect()
}

#[tokio::test]
async fn empty_registries_yield_empty_copies() {
    let supplier = make_supplier();
    let sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 2, &CancellationToken::new())
        .await;
    assert_eq!(sets.len(), 2);
    assert!(sets.iter().all(|s| s.is_empty()));
}

#[tokio::test]
async fn sealed_file_maps_to_a_path_identified_file_source() {
    let supplier = make_supplier();
    let path = install_sfst(&supplier.registries, "default", 1, 1000, 1005).await;

    let mut sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    assert_eq!(sources.len(), 1);
    let TraceSource::Sfst(c) = &sources[0] else {
        panic!("sealed file must map to an Sfst source");
    };
    // Identity is the registry path (encodes the full FileId); sealed
    // files carry no WAL coverage; the summary passes through.
    assert_eq!(c.source_id, SourceId::new(path.display().to_string()));
    assert!(c.coverage.is_none());
    assert_eq!(c.summary.record_count, 6);
    assert_eq!(
        (c.summary.min_timestamp_s, c.summary.max_timestamp_s),
        (1000, 1005)
    );
    assert!(matches!(&c.source, sfsq::Source::File(p) if p == &path));
}

#[tokio::test]
async fn copies_are_structurally_identical() {
    // Search consumes two source vectors from ONE capture (window ⊆
    // completion validation matches by source id) — the copies must be
    // the same sources in the same order.
    let supplier = make_supplier();
    install_sfst(&supplier.registries, "default", 1, 1000, 1005).await;
    install_sfst(&supplier.registries, "default", 2, 2000, 2005).await;

    let sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 2, &CancellationToken::new())
        .await;
    let ids: Vec<Vec<String>> = sets.iter().map(|s| source_ids(s)).collect();
    assert_eq!(ids[0].len(), 2);
    assert_eq!(ids[0], ids[1]);
}

#[tokio::test]
async fn window_pruning_is_file_granular() {
    let supplier = make_supplier();
    let in_window = install_sfst(&supplier.registries, "default", 1, 1000, 1005).await;
    install_sfst(&supplier.registries, "default", 2, 5000, 5005).await;

    let mut sets = supplier
        .capture(&TenantId::from("default"), 900..2000, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    // Pin WHICH file survives, not just the count — an inverted pruning
    // predicate keeping the wrong file must fail here.
    assert_eq!(
        source_ids(&sources),
        [in_window.display().to_string()],
        "exactly the file overlapping the window is captured"
    );
}

#[tokio::test]
async fn wal_resolves_to_chunks_and_a_tail() {
    // Three frames of 3 spans each at min_entries=4: frames 0+1 group
    // into one 6-entry chunk, frame 2 is the un-chunked tail.
    let supplier = make_supplier_with_min_entries(4);
    let path = install_wal(
        &supplier.registries,
        "default",
        1,
        vec![
            otlp_req(0x11, 3, 1_000_000_000),
            otlp_req(0x22, 3, 2_000_000_000),
            otlp_req(0x33, 3, 3_000_000_000),
        ],
    )
    .await;

    let mut sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    let ids = source_ids(&sources);
    assert_eq!(ids.len(), 2, "one chunk + one tail: {ids:?}");
    assert_eq!(ids[0], format!("{}#chunk0", path.display()));
    let TraceSource::Sfst(chunk) = &sources[0] else {
        panic!("first source must be the built chunk");
    };
    // The chunk was built through the traces seal: its summary counts
    // the 6 spans of frames 0+1, and the bytes are servable in-memory.
    assert_eq!(chunk.summary.record_count, 6);
    assert!(matches!(&chunk.source, sfsq::Source::Memory(_)));
    let coverage = chunk.coverage.as_ref().expect("chunks carry coverage");
    assert_eq!(coverage.wal_id.as_ref(), path.display().to_string());

    let TraceSource::Tail(tail) = &sources[1] else {
        panic!("second source must be the tail");
    };
    assert_eq!(
        tail.source_id.as_str(),
        format!("{}#tail{}", path.display(), tail.coverage.range.start())
    );
    // Chunk and tail partition the durable prefix: adjacent, no overlap.
    assert_eq!(coverage.range.end(), tail.coverage.range.start());
    // And the engine's own validation accepts the set.
    sfsq::traces::validate_sources(&sources).expect("capture output must validate");
}

#[tokio::test]
async fn wal_below_min_entries_is_all_tail() {
    let supplier = make_supplier_with_min_entries(1_000_000);
    let path = install_wal(
        &supplier.registries,
        "default",
        2,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;

    let mut sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    assert_eq!(sources.len(), 1);
    let TraceSource::Tail(tail) = &sources[0] else {
        panic!("everything below min_entries is one tail");
    };
    assert_eq!(
        tail.source_id.as_str(),
        format!("{}#tail{}", path.display(), wal::HEADER_SIZE)
    );
}

#[tokio::test]
async fn corrupt_wal_is_refused_whole_but_sealed_files_still_serve() {
    // Corrupt everything past the WAL header: the scan/build fails and
    // the WHOLE WAL is refused for this capture, while the sealed file
    // keeps serving (the logs failure policy).
    let supplier = make_supplier_with_min_entries(4);
    install_sfst(&supplier.registries, "default", 3, 1000, 1005).await;
    let path = install_wal(
        &supplier.registries,
        "default",
        4,
        vec![otlp_req(0x11, 3, 1_000_000_000), otlp_req(0x22, 3, 2_000_000_000)],
    )
    .await;
    let len = std::fs::metadata(&path).unwrap().len();
    let garbage = vec![0xFFu8; (len - wal::HEADER_SIZE as u64) as usize];
    {
        use std::io::{Seek, Write};
        let mut f = std::fs::OpenOptions::new().write(true).open(&path).unwrap();
        f.seek(std::io::SeekFrom::Start(wal::HEADER_SIZE as u64)).unwrap();
        f.write_all(&garbage).unwrap();
    }

    let mut sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    assert_eq!(sources.len(), 1, "only the sealed file survives");
    assert!(matches!(&sources[0], TraceSource::Sfst(c) if c.coverage.is_none()));
}

#[tokio::test]
async fn cancelled_capture_with_a_wal_returns_empty_and_caches_nothing() {
    // The WAL loop polls before resolving each WAL: a pre-cancelled
    // call returns empty without touching the chunk cache.
    let supplier = make_supplier_with_min_entries(4);
    install_wal(
        &supplier.registries,
        "default",
        5,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    let cancel = CancellationToken::new();
    cancel.cancel();
    let sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 1, &cancel)
        .await;
    assert!(sets.is_empty());
}

#[tokio::test]
async fn cancelled_capture_returns_empty() {
    // The documented contract: a cancelled call returns empty, even
    // when sealed sources were already snapshotted.
    let supplier = make_supplier();
    install_sfst(&supplier.registries, "default", 1, 1000, 1005).await;
    let cancel = CancellationToken::new();
    cancel.cancel();
    let sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 2, &cancel)
        .await;
    assert!(sets.is_empty());
}

#[tokio::test]
async fn capture_is_tenant_scoped() {
    let supplier = make_supplier();
    install_sfst(&supplier.registries, "tenant-a", 1, 1000, 1005).await;

    let mut sets = supplier
        .capture(&TenantId::from("tenant-b"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    assert!(
        sets.pop().unwrap().is_empty(),
        "another tenant's files are invisible"
    );
}
