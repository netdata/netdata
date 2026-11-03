//! Uploader response handling and the upload-retry drain.
//!
//! The ledger side of the shared uploader's IPC — the transport itself is
//! file-lifecycle's uploader.rs (semaphore-bounded PUTs). Responses arrive on
//! the run loop's `UploaderResp` arm and echo the request's `pipeline_id`.
//! `Uploaded` marks the seq uploaded on the tenant's registry and forwards a
//! catalog `AddEntry` — the entry's summary fields come straight from the
//! registry's `sfst::File`, no pending-metadata cache. `CatalogUploaded`
//! marks the covered seqs remote-cataloged, the stage retention's
//! SFST-eviction gate waits for (`ledger/retention.rs`
//! `evaluate_retention`). `*Failed` responses are
//! re-queued in the shared `upload_retry`
//! (src/crates/file-lifecycle/src/upload_retry.rs) while their local file
//! still exists; `handle_retry_tick` re-issues due entries with capped
//! exponential backoff.

use bridge::signals::Signal;
use file_registry::{SeqKey, TimestampNs};
use tokio::time::Instant;

use file_lifecycle::ipc::{CatalogBuilderRequest, UploaderRequest, UploaderResponse};
use file_lifecycle::recovery::now_ns;

use super::Ledger;
use file_lifecycle::helpers::{build_catalog_entry, date_from_summary};

impl Ledger {
    pub(super) async fn handle_uploader_resp(&mut self, resp: UploaderResponse) {
        let pipeline_id = match &resp {
            UploaderResponse::Uploaded { pipeline_id, .. }
            | UploaderResponse::UploadFailed { pipeline_id, .. }
            | UploaderResponse::CatalogUploaded { pipeline_id, .. }
            | UploaderResponse::CatalogUploadFailed { pipeline_id, .. } => *pipeline_id,
        };
        // The uploader echoes back the raw `pipeline_id` it was handed (agnostic
        // IPC — it never learns the signal). Decode it at this boundary.
        let signal = match Signal::try_from(pipeline_id) {
            Ok(signal) => signal,
            Err(e) => {
                tracing::error!(%e, "uploader response for unknown signal; dropping");
                return;
            }
        };
        let pipeline = self.pipelines.get(signal);
        // Clone the handles so nothing holds a borrow of `self` past this
        // point — the arms below mutate `upload_retry` (a `&mut self` field)
        // and await on the registries lock.
        let registries = pipeline.registries().clone();
        let catalog_builder_tx = pipeline.catalog_builder_tx().clone();

        match resp {
            UploaderResponse::Uploaded {
                seq,
                remote_key,
                etag,
                ..
            } => {
                tracing::info!("upload complete seq={seq} remote_key={remote_key}");
                self.upload_retry.clear_sfst(seq);
                let (tenant_id, entry, date) = {
                    let mut registries = registries.write().await;
                    match registries.for_seq_mut(seq.seq) {
                        Some((tid, registry)) => {
                            let Some(sfst_file) = registry.sfst.get(seq.seq).cloned() else {
                                // Routed seq with no local SFST entry — no
                                // summary to build a catalog entry from.
                                // Mark uploaded (nothing re-attempts an
                                // entryless seq; recovery re-queues only
                                // tracked files) and return.
                                registry.mark_uploaded(seq);
                                return;
                            };
                            // Catalog partition date; empty/unrepresentable
                            // summaries fall back to today at the call site
                            // (`src/crates/file-lifecycle/src/helpers.rs`
                            // `date_from_summary`).
                            let date = date_from_summary(&sfst_file.summary)
                                .unwrap_or_else(|| chrono::Utc::now().date_naive());
                            let uploaded_at_ns = TimestampNs(now_ns());
                            let entry =
                                build_catalog_entry(&sfst_file, remote_key, uploaded_at_ns, etag);
                            registry.mark_uploaded(seq);
                            (tid, entry, date)
                        }
                        None => return,
                    }
                };

                let req = CatalogBuilderRequest::AddEntry {
                    tenant_id,
                    date,
                    entry,
                };
                // A failed send is only logged: the seq is in no catalog file,
                // so it never reaches the rotated stage and the next restart's
                // remote reconcile re-sends the AddEntry from the local summary
                // (`src/crates/file-lifecycle/src/recovery/remote.rs`
                // `reconcile_remote_uploads`).
                if let Err(e) = catalog_builder_tx.send(req) {
                    tracing::error!("failed to send catalog add entry seq={seq}: {e}");
                }
            }
            UploaderResponse::UploadFailed {
                seq,
                local_path,
                remote_key,
                error,
                ..
            } => {
                tracing::error!(seq = %seq, remote_key = %remote_key, "upload failed: {error}");
                // Retry only while the local source still exists. Commonly the
                // failure IS the vanished file (the uploader's read failed), or
                // retention evicted it meanwhile; either way a retry can never
                // succeed, so abandon it and drop the queue entry.
                if local_path.exists() {
                    self.upload_retry.record_failure(
                        UploaderRequest::Upload {
                            pipeline_id,
                            seq,
                            local_path,
                            remote_key,
                        },
                        Instant::now(),
                    );
                } else {
                    tracing::warn!(seq = %seq, "local index file gone; abandoning upload retry");
                    self.upload_retry.clear_sfst(seq);
                }
            }
            UploaderResponse::CatalogUploaded {
                local_path,
                remote_key,
                identity,
                seqs,
                ..
            } => {
                tracing::info!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    "catalog upload complete",
                );
                self.upload_retry.clear_catalog(&remote_key);
                // The catalog is durably remote, so the SFSTs it covers become
                // evictable — this mark is what retention's gate waits for
                // (`ledger/retention.rs` `evaluate_retention`). Route each seq
                // separately: an already
                // evicted sibling (its route is forgotten on eviction) must not
                // block marking the rest. Key each mark by the catalog's OWN
                // echoed identity, not this process's.
                if !seqs.is_empty() {
                    let mut registries = registries.write().await;
                    for seq in &seqs {
                        let key = SeqKey::new(identity, *seq);
                        if let Some((_tenant, registry)) = registries.for_seq_mut(*seq) {
                            // Marking under the catalog's own identity is always
                            // correct: a prior-instance file still on disk at
                            // that seq is exactly what the mark makes evictable;
                            // with no matching local SFST the mark is inert,
                            // bounded in-memory state (logged below). Deliberately
                            // not gated on a local-SFST lookup.
                            if registry.sfst.get(*seq).map(|e| SeqKey::from(&e.id)) != Some(key) {
                                tracing::debug!(
                                    mark = %key,
                                    "remote-cataloged mark has no matching local SFST (inert)"
                                );
                            }
                            registry.mark_remote_cataloged([key]);
                        }
                    }
                }
            }
            UploaderResponse::CatalogUploadFailed {
                local_path,
                remote_key,
                identity,
                seqs,
                error,
                ..
            } => {
                tracing::error!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    seqs = seqs.len(),
                    "catalog upload failed: {error}",
                );
                if local_path.exists() {
                    self.upload_retry.record_failure(
                        UploaderRequest::UploadCatalog {
                            pipeline_id,
                            local_path,
                            remote_key,
                            identity,
                            seqs,
                        },
                        Instant::now(),
                    );
                } else {
                    tracing::warn!(
                        remote_key = %remote_key,
                        "local catalog file gone; abandoning upload retry",
                    );
                    self.upload_retry.clear_catalog(&remote_key);
                }
            }
        }
    }

    /// Re-issue failed uploads whose backoff has elapsed, then log the pending
    /// backlog — `warn` normally, escalating to `error` once an item has been
    /// retried `PERSISTENT_FAILURE_ATTEMPTS` times (the threshold constant in
    /// `src/crates/file-lifecycle/src/upload_retry.rs`): the remote is then
    /// treated as persistently unreachable and local files accumulate until it
    /// recovers. Fired every 30s by the ledger's retry timer (created in
    /// [`Ledger::new`](super::Ledger::new), missed ticks skipped).
    pub(super) async fn handle_retry_tick(&mut self) {
        if self.upload_retry.is_empty() {
            return;
        }

        let due = self.upload_retry.take_due(Instant::now());
        let pending = self.upload_retry.len();
        let max_attempts = self.upload_retry.max_attempts();

        for req in due {
            let Some(uploader) = self.uploader.as_mut() else {
                break;
            };
            if let Err(e) = uploader.send(req) {
                // The uploader is gone (closed channel); re-arm so the item
                // isn't stranded `in_flight`. The run loop exits on the same
                // closed channel (`ledger/mod.rs` `Ledger::run`) and the next
                // restart's recovery re-drives un-uploaded files idempotently
                // (deterministic remote keys; uploads overwrite).
                tracing::error!("failed to re-issue upload: {e}");
                self.upload_retry.record_failure(e.0, Instant::now());
            }
        }

        if max_attempts >= file_lifecycle::upload_retry::PERSISTENT_FAILURE_ATTEMPTS {
            tracing::error!(
                pending,
                max_attempts,
                "remote storage appears persistently unreachable: {pending} upload(s) stuck and \
                 being retried; local index/catalog files will accumulate until the remote \
                 recovers — operator action required",
            );
        } else {
            tracing::warn!(
                pending,
                max_attempts,
                "remote storage uploads failing: {pending} upload(s) pending retry",
            );
        }
    }
}
