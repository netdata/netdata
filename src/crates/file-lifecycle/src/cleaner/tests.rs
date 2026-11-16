//! Tests for the cleaner: `process` called directly on real temp dirs — no
//! tokio runtime, no `Component` run loop — pinning the parent module's
//! contract.
//!
//! - A `DeleteCatalogFile` answers `CatalogFileDeleted` when the file is
//!   gone and when it was never there: `NotFound` counts as success, so a
//!   retried delete is harmless (the parent deletion-idempotency contract).
//! - A successful delete prunes the emptied ancestors of the
//!   `{base}/{date}/{tenant}/` layout, stopping at the first non-empty one
//!   and never touching the base dir (`max_levels = 2`): a surviving
//!   sibling catalog in the same tenant dir leaves the tenant and date
//!   dirs in place, another tenant's dir keeps the date dir, and an
//!   already-missing file prunes nothing when its sibling-bearing dir
//!   survives.
//!
//! Not pinned here: the WAL and index delete variants, the `*Failed`
//! responses, and the run loop's cancel and closed-channel exits.
use super::*;
use std::path::PathBuf;

/// Write a one-byte catalog file at `{base}/{date}/{tenant}/{name}` and
/// return its path. The byte is filler; what matters is which files
/// survive a delete, because a non-empty ancestor stops the prune walk.
fn write_catalog(base: &Path, date: &str, tenant: &str, name: &str) -> PathBuf {
    let dir = base.join(date).join(tenant);
    std::fs::create_dir_all(&dir).unwrap();
    let path = dir.join(name);
    std::fs::write(&path, b"x").unwrap();
    path
}

#[test]
fn delete_catalog_leaves_dir_when_siblings_remain() {
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let p1 = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");
    let _p2 = write_catalog(base, "2026-04-17", "tenant1", "b.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile {
        pipeline_id: 0,
        path: p1.clone(),
    });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(!p1.exists());

    // b.catalog keeps the tenant dir non-empty, so the prune walk aborts
    // there and both dirs survive.
    assert!(base.join("2026-04-17").join("tenant1").is_dir());
    assert!(base.join("2026-04-17").is_dir());
}

#[test]
fn delete_last_catalog_prunes_tenant_and_date_dirs() {
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let p = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile {
        pipeline_id: 0,
        path: p.clone(),
    });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(!p.exists());

    // Both ancestors are empty post-delete: tenant1/ pruned, then 2026-04-17/.
    assert!(!base.join("2026-04-17").join("tenant1").exists());
    assert!(!base.join("2026-04-17").exists());
    // Base dir itself stays — pruning stops at max_levels=2.
    assert!(base.is_dir());
}

#[test]
fn delete_last_catalog_keeps_date_dir_if_other_tenant_present() {
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let p1 = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");
    let _p2 = write_catalog(base, "2026-04-17", "tenant2", "a.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile {
        pipeline_id: 0,
        path: p1.clone(),
    });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));

    // tenant1/ pruned (empty); tenant2/ keeps the date dir non-empty (walk aborts there).
    assert!(!base.join("2026-04-17").join("tenant1").exists());
    assert!(base.join("2026-04-17").join("tenant2").is_dir());
    assert!(base.join("2026-04-17").is_dir());
}

#[test]
fn delete_missing_catalog_is_noop_and_does_not_prune() {
    // NotFound is treated as success, so the response is still
    // `CatalogFileDeleted` — and the prune walk still runs. It starts at
    // tenant1/, which a.catalog keeps non-empty, so the walk aborts there.
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let _sibling = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");
    let missing = base
        .join("2026-04-17")
        .join("tenant1")
        .join("missing.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile {
        pipeline_id: 0,
        path: missing.clone(),
    });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(base.join("2026-04-17").join("tenant1").is_dir());
}
