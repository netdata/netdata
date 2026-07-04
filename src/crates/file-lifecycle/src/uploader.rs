//! Uploader component that copies index and catalog files to remote object
//! storage.
//!
//! Uploads run in spawned async tasks, but the number in flight at once is
//! bounded by a [`Semaphore`]: the receive loop acquires a permit before
//! spawning, so a recovery backlog (which enqueues every un-uploaded file at
//! once) can't fan out into thousands of simultaneous file reads + PUTs. Each
//! task buffers its whole file in memory, so the permit count also caps peak
//! upload memory. In-flight tasks are tracked in a [`JoinSet`] so a shutdown
//! drains them briefly instead of abandoning detached uploads mid-PUT.
//!
//! The backend is abstracted behind [`Storage`]; the component is generic over
//! it so production uses opendal and tests inject a mock.

use std::marker::PhantomData;
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::{Semaphore, mpsc};
use tokio::task::JoinSet;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{UploaderRequest, UploaderResponse};
use crate::storage::{Storage, WriteMeta};

/// How long a graceful shutdown waits for in-flight uploads to finish before
/// abandoning them. Abandoned uploads are idempotently re-driven on the next
/// restart (deterministic remote key, `storage.write` overwrites), so this is a
/// best-effort courtesy, not a correctness requirement.
const SHUTDOWN_DRAIN: Duration = Duration::from_secs(5);

/// Construction args for the uploader. The `Storage` bound lives on the
/// `Component` impl, not here, so this `pub` type doesn't leak the crate-private
/// trait into its signature.
pub struct UploaderArgs<S> {
    pub storage: S,
    /// Maximum uploads in flight at once. Bounds concurrent PUTs/sockets and,
    /// because each task buffers its whole file, peak upload memory.
    pub max_concurrent: usize,
}

// `fn() -> S` marker: `Uploader<S>` names the backend type for its `Component`
// impl but never owns an `S`.
pub struct Uploader<S>(PhantomData<fn() -> S>);

impl<S: Storage> Component for Uploader<S> {
    type Request = UploaderRequest;
    type Response = UploaderResponse;
    type Args = UploaderArgs<S>;

    async fn run(
        args: UploaderArgs<S>,
        mut rx: mpsc::UnboundedReceiver<UploaderRequest>,
        tx: mpsc::UnboundedSender<UploaderResponse>,
        cancel: CancellationToken,
    ) {
        let UploaderArgs {
            storage,
            max_concurrent,
        } = args;
        // Shared across upload tasks; each task clones the `Arc`, not the backend.
        let storage = Arc::new(storage);
        // `max(1)` guards against a misconfigured `0`, which would deadlock.
        let semaphore = Arc::new(Semaphore::new(max_concurrent.max(1)));
        let mut tasks: JoinSet<()> = JoinSet::new();

        loop {
            tokio::select! {
                biased;
                _ = cancel.cancelled() => break,
                // Reap finished tasks so the JoinSet doesn't accumulate handles.
                // (Permits are released when a task's `_permit` drops, not here.)
                Some(_) = tasks.join_next(), if !tasks.is_empty() => {}
                req = rx.recv() => {
                    let Some(request) = req else { break };

                    // Backpressure: wait for a free upload slot, staying
                    // responsive to shutdown. A finishing task releases its
                    // permit on drop, so this wakes without needing the JoinSet
                    // to be reaped first.
                    let permit = tokio::select! {
                        biased;
                        _ = cancel.cancelled() => break,
                        p = Arc::clone(&semaphore).acquire_owned() => match p {
                            Ok(p) => p,
                            Err(_) => break, // semaphore closed
                        }
                    };

                    let storage = Arc::clone(&storage);
                    let tx = tx.clone();
                    tasks.spawn(async move {
                        let _permit = permit; // released when this upload ends
                        let resp = process(&*storage, request).await;
                        let _ = tx.send(resp);
                    });
                }
            }
        }

        // Give in-flight uploads a brief window to finish (so their responses
        // land and PUTs aren't cut mid-flight), then drop `tasks`, aborting the
        // rest — those are re-driven idempotently on the next restart.
        let drain = async { while tasks.join_next().await.is_some() {} };
        if tokio::time::timeout(SHUTDOWN_DRAIN, drain).await.is_err() {
            tracing::warn!("uploader shutdown: abandoning in-flight uploads after drain timeout");
        }
    }
}

/// Read a local file, write it to `remote_key`, and verify its size — the
/// shared body for both SFST and catalog uploads. Returns the [`WriteMeta`]
/// (its ETag is recorded on SFST catalog entries) or a stringified error for
/// the failure response.
async fn put_file<S: Storage>(
    storage: &S,
    local_path: &Path,
    remote_key: &str,
) -> Result<WriteMeta, String> {
    let data = tokio::fs::read(local_path)
        .await
        .map_err(|e| format!("read failed: {e}"))?;
    let len = data.len() as u64;
    let meta = storage
        .write(remote_key, data)
        .await
        .map_err(|e| e.to_string())?;
    // Defensive size check: some backends report a content_length of 0
    // (unknown), so only a non-zero mismatch is treated as a failed upload.
    let remote_len = meta.content_length;
    if remote_len != 0 && remote_len != len {
        return Err(format!("size mismatch: sent {len}, remote {remote_len}"));
    }
    Ok(meta)
}

/// Perform one upload request, producing the matching response.
///
/// `pipeline_id` is opaque to the uploader (it only moves bytes) and is echoed
/// verbatim onto the response so the run-loop can route the registry mutation
/// back to the owning pipeline.
async fn process<S: Storage>(storage: &S, request: UploaderRequest) -> UploaderResponse {
    match request {
        UploaderRequest::Upload {
            pipeline_id,
            seq,
            local_path,
            remote_key,
        } => {
            let start = Instant::now();
            tracing::info!("upload started seq={seq} remote_key={remote_key}");
            match put_file(storage, &local_path, &remote_key).await {
                Ok(meta) => {
                    tracing::info!(
                        "upload complete seq={seq} remote_key={remote_key} elapsed_ms={}",
                        start.elapsed().as_millis(),
                    );
                    UploaderResponse::Uploaded {
                        pipeline_id,
                        seq,
                        remote_key,
                        etag: meta.etag,
                    }
                }
                Err(error) => {
                    tracing::error!("upload failed seq={seq}: {error}");
                    UploaderResponse::UploadFailed {
                        pipeline_id,
                        seq,
                        local_path,
                        remote_key,
                        error,
                    }
                }
            }
        }
        UploaderRequest::UploadCatalog {
            pipeline_id,
            local_path,
            remote_key,
            identity,
            seqs,
        } => {
            let start = Instant::now();
            tracing::info!(
                path = %local_path.display(),
                remote_key = %remote_key,
                "catalog upload started",
            );
            match put_file(storage, &local_path, &remote_key).await {
                Ok(_) => {
                    tracing::info!(
                        remote_key = %remote_key,
                        elapsed_ms = start.elapsed().as_millis() as u64,
                        "catalog upload complete",
                    );
                    UploaderResponse::CatalogUploaded {
                        pipeline_id,
                        local_path,
                        remote_key,
                        identity,
                        seqs,
                    }
                }
                Err(error) => {
                    tracing::error!(
                        remote_key = %remote_key,
                        "catalog upload failed: {error}",
                    );
                    UploaderResponse::CatalogUploadFailed {
                        pipeline_id,
                        local_path,
                        remote_key,
                        identity,
                        seqs,
                        error,
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::MockStorage;
    use std::io::Write;

    /// A local file the uploader can read. Returns the file plus its byte length.
    fn temp_file(contents: &[u8]) -> (tempfile::NamedTempFile, u64) {
        let mut f = tempfile::NamedTempFile::new().unwrap();
        f.write_all(contents).unwrap();
        f.flush().unwrap();
        (f, contents.len() as u64)
    }

    fn sk(seq: u64) -> file_registry::SeqKey {
        file_registry::SeqKey::new(file_registry::test_identity(), seq)
    }

    fn upload_req(path: &Path) -> UploaderRequest {
        UploaderRequest::Upload {
            pipeline_id: 0,
            seq: sk(7),
            local_path: path.to_path_buf(),
            remote_key: "v2/key".to_owned(),
        }
    }

    #[tokio::test]
    async fn upload_happy_path_reports_etag() {
        let (file, _len) = temp_file(b"hello world");
        let storage = MockStorage::default(); // echoes len, etag Some
        let resp = process(&storage, upload_req(file.path())).await;
        match resp {
            UploaderResponse::Uploaded { seq, etag, .. } => {
                assert_eq!(seq, sk(7));
                assert_eq!(etag.as_deref(), Some("mock-etag"));
            }
            other => panic!("expected Uploaded, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn upload_size_mismatch_fails() {
        let (file, _len) = temp_file(b"hello"); // 5 bytes
        let storage = MockStorage {
            write_content_length: Some(99), // non-zero and != 5
            ..MockStorage::default()
        };
        let resp = process(&storage, upload_req(file.path())).await;
        match resp {
            UploaderResponse::UploadFailed { error, .. } => {
                assert!(error.contains("size mismatch"), "got: {error}");
            }
            other => panic!("expected UploadFailed, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn upload_zero_content_length_is_accepted() {
        let (file, _len) = temp_file(b"hello");
        let storage = MockStorage {
            write_content_length: Some(0), // backend reports "unknown"
            ..MockStorage::default()
        };
        let resp = process(&storage, upload_req(file.path())).await;
        assert!(
            matches!(resp, UploaderResponse::Uploaded { .. }),
            "0 content_length must pass the size guard"
        );
    }

    #[tokio::test]
    async fn upload_write_error_fails() {
        let (file, _len) = temp_file(b"hello");
        let storage = MockStorage {
            write_error: Some("boom".to_owned()),
            ..MockStorage::default()
        };
        let resp = process(&storage, upload_req(file.path())).await;
        match resp {
            UploaderResponse::UploadFailed { error, .. } => assert!(error.contains("boom")),
            other => panic!("expected UploadFailed, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn upload_read_error_fails() {
        // Non-existent local path -> read failure before any write.
        let storage = MockStorage::default();
        let resp = process(&storage, upload_req(Path::new("/nonexistent/path/x.sfst"))).await;
        match resp {
            UploaderResponse::UploadFailed { error, .. } => assert!(error.contains("read failed")),
            other => panic!("expected UploadFailed, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn catalog_upload_happy_path() {
        let (file, _len) = temp_file(b"catalog-bytes");
        let storage = MockStorage::default();
        let req = UploaderRequest::UploadCatalog {
            pipeline_id: 0,
            local_path: file.path().to_path_buf(),
            remote_key: "v2/catalog/key".to_owned(),
            identity: file_registry::test_identity(),
            seqs: vec![1, 2, 3],
        };
        let resp = process(&storage, req).await;
        match resp {
            UploaderResponse::CatalogUploaded { seqs, .. } => assert_eq!(seqs, vec![1, 2, 3]),
            other => panic!("expected CatalogUploaded, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn catalog_upload_error_carries_seqs() {
        let (file, _len) = temp_file(b"catalog-bytes");
        let storage = MockStorage {
            write_error: Some("nope".to_owned()),
            ..MockStorage::default()
        };
        let req = UploaderRequest::UploadCatalog {
            pipeline_id: 0,
            local_path: file.path().to_path_buf(),
            remote_key: "v2/catalog/key".to_owned(),
            identity: file_registry::test_identity(),
            seqs: vec![5],
        };
        let resp = process(&storage, req).await;
        match resp {
            UploaderResponse::CatalogUploadFailed { seqs, error, .. } => {
                assert_eq!(seqs, vec![5]);
                assert!(error.contains("nope"));
            }
            other => panic!("expected CatalogUploadFailed, got {other:?}"),
        }
    }
}
