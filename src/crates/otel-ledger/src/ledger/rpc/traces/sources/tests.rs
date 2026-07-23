use super::*;
use file_registry::{ByteSize, FileId, test_identity};

// These tests cover the sealed-file mapping and snapshot semantics of
// `capture` — registry state in, engine sources out. `capture` never
// opens sealed files (summaries come from the registry; the engine
// opens files later), so the tracked paths need no real SFST bytes.
// The WAL branch (chunk builds via the traces seal, whole-WAL refusal)
// needs real WAL fixtures and is exercised end-to-end when the data
// modes land (steps 1.2+).

fn make_supplier() -> TracesSourceSupplier {
    let tr = TenantRegistries::new(
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
    );
    TracesSourceSupplier::new(
        Arc::new(RwLock::new(tr)),
        Arc::new(ChunkCache::new(64 * 1024 * 1024)),
        16_384,
    )
}

fn summary(record_count: u32, min_s: u32, max_s: u32) -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        content_meta: Vec::new(),
    }
}

/// Track a sealed SFST under `tenant` and return its registry path.
async fn install_sfst(
    supplier: &TracesSourceSupplier,
    tenant: &str,
    seq: u64,
    min_s: u32,
    max_s: u32,
) -> std::path::PathBuf {
    let id = FileId::new(test_identity(), 1, seq, 7);
    let mut guard = supplier.registries.write().await;
    let reg = guard.get_or_create(&TenantId::from(tenant));
    let path = reg.sfst.file_path(id);
    reg.sfst.track(id, ByteSize(1024), summary(6, min_s, max_s));
    path
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
    let path = install_sfst(&supplier, "default", 1, 1000, 1005).await;

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
    install_sfst(&supplier, "default", 1, 1000, 1005).await;
    install_sfst(&supplier, "default", 2, 2000, 2005).await;

    let sets = supplier
        .capture(&TenantId::from("default"), 0..u32::MAX, 2, &CancellationToken::new())
        .await;
    let ids: Vec<Vec<&SourceId>> = sets
        .iter()
        .map(|s| {
            s.iter()
                .map(|src| match src {
                    TraceSource::Sfst(c) => &c.source_id,
                    TraceSource::Tail(t) => &t.source_id,
                })
                .collect()
        })
        .collect();
    assert_eq!(ids[0].len(), 2);
    assert_eq!(ids[0], ids[1]);
}

#[tokio::test]
async fn window_pruning_is_file_granular() {
    let supplier = make_supplier();
    install_sfst(&supplier, "default", 1, 1000, 1005).await;
    install_sfst(&supplier, "default", 2, 5000, 5005).await;

    let mut sets = supplier
        .capture(&TenantId::from("default"), 900..2000, 1, &CancellationToken::new())
        .await;
    let sources = sets.pop().unwrap();
    assert_eq!(
        sources.len(),
        1,
        "only the file overlapping the window is captured"
    );
}

#[tokio::test]
async fn cancelled_capture_returns_empty() {
    // The documented contract: a cancelled call returns empty, even
    // when sealed sources were already snapshotted.
    let supplier = make_supplier();
    install_sfst(&supplier, "default", 1, 1000, 1005).await;
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
    install_sfst(&supplier, "tenant-a", 1, 1000, 1005).await;

    let mut sets = supplier
        .capture(&TenantId::from("tenant-b"), 0..u32::MAX, 1, &CancellationToken::new())
        .await;
    assert!(
        sets.pop().unwrap().is_empty(),
        "another tenant's files are invisible"
    );
}
