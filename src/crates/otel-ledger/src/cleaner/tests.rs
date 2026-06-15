use super::*;
use std::path::PathBuf;

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

    let resp = process(CleanerRequest::DeleteCatalogFile { path: p1.clone() });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(!p1.exists());

    // Tenant dir still has b.catalog; date dir still has tenant1.
    assert!(base.join("2026-04-17").join("tenant1").is_dir());
    assert!(base.join("2026-04-17").is_dir());
}

#[test]
fn delete_last_catalog_prunes_tenant_and_date_dirs() {
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let p = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile { path: p.clone() });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(!p.exists());

    // Both tenant and date dirs were empty post-delete → pruned.
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

    let resp = process(CleanerRequest::DeleteCatalogFile { path: p1.clone() });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));

    // tenant1/ pruned (empty); date dir kept (tenant2/ still there).
    assert!(!base.join("2026-04-17").join("tenant1").exists());
    assert!(base.join("2026-04-17").join("tenant2").is_dir());
    assert!(base.join("2026-04-17").is_dir());
}

#[test]
fn delete_missing_catalog_is_noop_and_does_not_prune() {
    // If the file is already gone, remove_file returns Ok (NotFound is
    // treated as success). The prune walk then runs against the
    // surviving sibling-bearing dir and finds it non-empty → no-op.
    let tmp = tempfile::tempdir().unwrap();
    let base = tmp.path();
    let _sibling = write_catalog(base, "2026-04-17", "tenant1", "a.catalog");
    let missing = base
        .join("2026-04-17")
        .join("tenant1")
        .join("missing.catalog");

    let resp = process(CleanerRequest::DeleteCatalogFile {
        path: missing.clone(),
    });
    assert!(matches!(resp, CleanerResponse::CatalogFileDeleted { .. }));
    assert!(base.join("2026-04-17").join("tenant1").is_dir());
}
