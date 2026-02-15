//! Startup recovery: replays pending work that was interrupted by a previous
//! shutdown or crash. Each function sends requests through the normal component
//! path via [`batch_recover`], so recovery and steady-state use the same code.

use std::time::{SystemTime, UNIX_EPOCH};

use wal::{ByteSize, FileId};

use crate::component::{ComponentHandle, batch_recover};
use crate::ipc::{
    CleanerRequest, CleanerResponse, IndexerRequest, IndexerResponse, UploaderRequest,
    UploaderResponse,
};
use crate::registry::Registry;

/// Index any WAL files that were archived but not yet indexed.
pub async fn recover_unindexed(
    registry: &mut Registry,
    indexer: &mut ComponentHandle<IndexerRequest, IndexerResponse>,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
) -> anyhow::Result<()> {
    let unindexed = registry.unindexed_ids();
    if unindexed.is_empty() {
        return Ok(());
    }

    tracing::info!("indexing {} unindexed WAL files", unindexed.len());

    let requests: Vec<_> = unindexed
        .iter()
        .map(|&id| IndexerRequest::FinalizeIndex {
            wal_path: registry.wal.dir().wal_path(id),
            index_path: registry.index.path(id),
        })
        .collect();

    batch_recover(requests, indexer, |resp| match resp {
        IndexerResponse::IndexFinalized { seq, .. } => {
            let wal_entry = registry.wal.remove_by_seq(seq);
            let created_at_ns = wal_entry
                .as_ref()
                .map(|w| w.created_at_ns)
                .unwrap_or_default();
            let id = wal_entry.map(|w| w.id).unwrap_or_else(|| {
                let dir = registry.wal.dir();
                FileId::new(dir.machine_id(), dir.boot_id(), seq, 0)
            });

            // Delete the now-redundant WAL file via the cleaner.
            let wal_path = registry.wal.dir().wal_path(id);
            let req = CleanerRequest::DeleteWalFile {
                sequence: seq,
                path: wal_path,
            };
            if let Err(e) = cleaner.send(req) {
                tracing::error!("recovery: failed to send WAL delete seq={seq}: {e}");
            }

            let index_file_path = registry.index.path(id);
            let index_size = ByteSize(
                std::fs::metadata(&index_file_path)
                    .map(|m| m.len())
                    .unwrap_or(0),
            );
            registry.index.track(id, created_at_ns, index_size);
            tracing::info!("recovery: index finalized seq={seq}");
        }
        IndexerResponse::IndexFailed {
            ref path,
            ref error,
        } => {
            tracing::error!(
                "recovery: indexing failed path={} error={error}",
                path.display()
            );
        }
    })
    .await?;

    tracing::info!("recovery indexing complete");
    Ok(())
}

/// Evict index files that exceed the retention policy.
pub async fn recover_retention(
    registry: &mut Registry,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    retention: &bridge::config::RetentionConfig,
) -> anyhow::Result<()> {
    let to_evict = registry.index.evaluate_retention(retention, now_ns());
    if to_evict.is_empty() {
        return Ok(());
    }

    tracing::info!("retention: evicting {} old index files", to_evict.len());

    let requests: Vec<_> = to_evict
        .iter()
        .filter_map(|&seq| {
            registry.index.get(seq).map(|entry| {
                let path = registry.index.path(entry.id);
                CleanerRequest::DeleteIndexFile {
                    sequence: seq,
                    path,
                }
            })
        })
        .collect();

    batch_recover(requests, cleaner, |resp| match resp {
        CleanerResponse::IndexFileDeleted { sequence } => {
            registry.index.remove(sequence);
            tracing::info!("recovery: index file evicted seq={sequence}");
        }
        CleanerResponse::IndexFileFailed { sequence, error } => {
            tracing::error!("recovery: index eviction failed seq={sequence} error={error}");
        }
        resp => {
            tracing::warn!("unexpected cleaner response during retention recovery: {resp:?}");
        }
    })
    .await
}

/// Upload index files that haven't been uploaded to remote storage yet.
pub async fn recover_unuploaded(
    registry: &mut Registry,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
) -> anyhow::Result<()> {
    let unuploaded = registry.unuploaded_ids();
    if unuploaded.is_empty() {
        return Ok(());
    }

    tracing::info!("uploading {} un-uploaded index files", unuploaded.len());

    let requests: Vec<_> = unuploaded
        .iter()
        .map(|&id| {
            let local_path = registry.index.path(id);
            let remote_key = id.to_filename("sfst");
            UploaderRequest::Upload {
                seq: id.seq,
                local_path,
                remote_key,
            }
        })
        .collect();

    batch_recover(requests, uploader, |resp| match resp {
        UploaderResponse::Uploaded { seq, remote_key } => {
            if let Some(entry) = registry.index.get(seq) {
                registry.remote.track(entry.id, remote_key);
            }
            tracing::info!("recovery: upload complete seq={seq}");
        }
        UploaderResponse::UploadFailed { seq, error } => {
            tracing::error!("recovery: upload failed seq={seq}: {error}");
        }
    })
    .await?;

    tracing::info!("recovery uploads complete");
    Ok(())
}

pub(crate) fn now_ns() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_nanos() as u64
}
