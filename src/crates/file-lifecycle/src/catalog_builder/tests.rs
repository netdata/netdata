use super::*;
use crate::component::ComponentHandle;
use file_registry::FileId;
use otel_catalog::CatalogEntry;

fn machine() -> Uuid {
    Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}

fn instance() -> Uuid {
    Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}

fn date() -> NaiveDate {
    NaiveDate::from_ymd_opt(2026, 4, 17).unwrap()
}

fn entry_for(seq: u64) -> CatalogEntry {
    let (part_key, content_meta) = crate::test_helpers::identity_for("prod", "api");
    CatalogEntry {
        id: FileId::new(machine(), instance(), 0, seq, part_key),
        remote_key: format!("tenant1/sfst/2026-04-17/{seq}.sfst"),
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_600,
        record_count: 100,
        content_meta,
        size: ByteSize(1024),
        uploaded_at_ns: file_registry::TimestampNs(2_000_000_000),
        remote_etag: None,
    }
}

fn add_request(seq: u64) -> CatalogBuilderRequest {
    CatalogBuilderRequest::AddEntry {
        tenant_id: TenantId::from("tenant1"),
        date: date(),
        entry: entry_for(seq),
    }
}

struct Harness {
    handle: ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    cancel: CancellationToken,
    _tmp: tempfile::TempDir,
    base: PathBuf,
}

impl Harness {
    fn new(rotation_count: usize) -> Self {
        let tmp = tempfile::tempdir().unwrap();
        let base = tmp.path().to_path_buf();
        let args = CatalogBuilderArgs {
            catalog_base_dir: base.clone(),
            rotation_count,
        };
        let cancel = CancellationToken::new();
        let handle = ComponentHandle::spawn::<CatalogBuilder>(args, cancel.child_token());
        Self {
            handle,
            cancel,
            _tmp: tmp,
            base,
        }
    }

    async fn send_recv(&mut self, req: CatalogBuilderRequest) -> CatalogBuilderResponse {
        self.handle.send(req).unwrap();
        self.handle.recv().await.unwrap()
    }
}

impl Drop for Harness {
    fn drop(&mut self) {
        self.cancel.cancel();
    }
}

#[tokio::test]
async fn add_entry_below_threshold_is_accepted() {
    let mut h = Harness::new(3);
    let resp = h.send_recv(add_request(1)).await;
    assert!(matches!(
        resp,
        CatalogBuilderResponse::EntryAccepted { seq: 1 }
    ));

    let expected_path = scope_path(
        &h.base,
        &TenantId::from("tenant1"),
        date(),
        machine(),
        instance(),
        1,
        1_700_000_000,
        1_700_003_600,
    );
    assert!(!expected_path.exists(), "must not rotate below threshold");
}

#[tokio::test]
async fn rotation_fires_at_threshold_and_writes_file() {
    let mut h = Harness::new(3);
    assert!(matches!(
        h.send_recv(add_request(1)).await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));
    assert!(matches!(
        h.send_recv(add_request(2)).await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));

    let resp = h.send_recv(add_request(3)).await;
    match resp {
        CatalogBuilderResponse::Rotated {
            tenant_id,
            max_seq,
            path,
            seqs,
            ..
        } => {
            assert_eq!(tenant_id.as_str(), "tenant1");
            assert_eq!(max_seq, 3);
            let mut seen = seqs.clone();
            seen.sort();
            assert_eq!(seen, vec![1, 2, 3]);
            assert!(path.exists(), "rotated catalog file must exist on disk");
            let bytes = std::fs::read(&path).unwrap();
            let catalog = Catalog::from_container_bytes(&bytes).unwrap();
            assert_eq!(catalog.entries.len(), 3);
        }
        other => panic!("expected Rotated, got {other:?}"),
    }
}

#[tokio::test]
async fn accumulator_is_drained_on_rotation() {
    let mut h = Harness::new(2);
    assert!(matches!(
        h.send_recv(add_request(1)).await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));
    // Hits threshold
    let r = h.send_recv(add_request(2)).await;
    let first_path = match r {
        CatalogBuilderResponse::Rotated { path, .. } => path,
        other => panic!("expected Rotated, got {other:?}"),
    };

    // Next entry starts a fresh accumulator; one more to hit threshold again.
    assert!(matches!(
        h.send_recv(add_request(3)).await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));
    let r = h.send_recv(add_request(4)).await;
    let second_path = match r {
        CatalogBuilderResponse::Rotated { path, max_seq, .. } => {
            assert_eq!(max_seq, 4);
            path
        }
        other => panic!("expected Rotated, got {other:?}"),
    };

    assert_ne!(first_path, second_path);
    let second = Catalog::from_container_bytes(&std::fs::read(&second_path).unwrap()).unwrap();
    let seqs: Vec<u64> = second.entries.values().map(|e| e.id.seq).collect();
    let mut sorted = seqs.clone();
    sorted.sort();
    assert_eq!(sorted, vec![3, 4]);
}

#[tokio::test]
async fn rotation_below_one_is_still_honored() {
    // rotation_count = 1 rotates on every AddEntry.
    let mut h = Harness::new(1);
    let resp = h.send_recv(add_request(5)).await;
    match resp {
        CatalogBuilderResponse::Rotated { max_seq, seqs, .. } => {
            assert_eq!(max_seq, 5);
            assert_eq!(seqs, vec![5]);
        }
        other => panic!("expected Rotated, got {other:?}"),
    }
}

#[tokio::test]
async fn rotation_failure_preserves_accumulator() {
    // Point catalog_base_dir at a regular file. mkdir_all under it will
    // fail with ENOTDIR, causing the rotation to fail.
    let tmp = tempfile::tempdir().unwrap();
    let sentinel = tmp.path().join("not_a_dir");
    std::fs::write(&sentinel, b"").unwrap();

    let cancel = CancellationToken::new();
    let mut handle = ComponentHandle::spawn::<CatalogBuilder>(
        CatalogBuilderArgs {
            catalog_base_dir: sentinel,
            rotation_count: 1,
        },
        cancel.child_token(),
    );

    handle.send(add_request(1)).unwrap();
    let resp = handle.recv().await.unwrap();
    match resp {
        CatalogBuilderResponse::RotationFailed { max_seq, .. } => {
            assert_eq!(max_seq, 1);
        }
        other => panic!("expected RotationFailed, got {other:?}"),
    }

    // Next AddEntry should find the accumulator intact and try to rotate
    // again (still fails, but the entry was not lost).
    handle.send(add_request(2)).unwrap();
    let resp = handle.recv().await.unwrap();
    match resp {
        CatalogBuilderResponse::RotationFailed { max_seq, .. } => {
            // Accumulator now has seq 1 and 2; max_seq is 2.
            assert_eq!(max_seq, 2);
        }
        other => panic!("expected RotationFailed, got {other:?}"),
    }

    cancel.cancel();
}

#[tokio::test]
async fn distinct_scopes_rotate_independently() {
    let mut h = Harness::new(2);
    // tenant1 seq=1
    assert!(matches!(
        h.send_recv(add_request(1)).await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));

    // A different scope (different machine_id) — shouldn't trigger
    // tenant1's rotation.
    let other_machine = Uuid::from_u128(0x1111);
    let other_entry = CatalogEntry {
        id: FileId::new(other_machine, instance(), 0, 1, 0),
        ..entry_for(1)
    };
    assert!(matches!(
        h.send_recv(CatalogBuilderRequest::AddEntry {
            tenant_id: TenantId::from("tenant1"),
            date: date(),
            entry: other_entry,
        })
        .await,
        CatalogBuilderResponse::EntryAccepted { .. }
    ));

    // tenant1 seq=2 — now hits threshold for the original scope.
    let resp = h.send_recv(add_request(2)).await;
    match resp {
        CatalogBuilderResponse::Rotated {
            machine_id, seqs, ..
        } => {
            assert_eq!(machine_id, machine());
            let mut sorted = seqs.clone();
            sorted.sort();
            assert_eq!(sorted, vec![1, 2]);
        }
        other => panic!("expected Rotated, got {other:?}"),
    }
}
