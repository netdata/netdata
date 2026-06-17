//! Remote object-storage seam.
//!
//! [`Storage`] is the single abstraction the uploader and recovery code depend
//! on, so opendal is confined to this module's [`OpendalStorage`] impl and the
//! rest of the crate stays backend-agnostic and unit-testable with a mock.
//!
//! Native async-in-trait + generic dispatch (matching the crate's [`Component`]
//! trait) keeps calls monomorphized and zero-cost. The `+ Send` bound on each
//! returned future is needed because storage futures run in spawned upload
//! tasks (`write`) and under `join_all` during recovery (`list`); it is applied
//! uniformly across all three methods so any backend stays usable there.
//!
//! [`Component`]: crate::component::Component

use std::future::Future;
use std::time::Duration;

/// Metadata returned by a successful [`Storage::write`].
pub(crate) struct WriteMeta {
    /// Size the backend reports for the written object. Some backends report
    /// `0` (unknown); callers must treat only a non-zero mismatch as a failure.
    pub content_length: u64,
    /// Backend-reported ETag (e.g. S3), recorded on the catalog entry; `None`
    /// when the backend does not supply one.
    pub etag: Option<String>,
}

/// Error surfaced by a [`Storage`] operation.
///
/// `NotFound` is preserved as its own variant because recovery's catalog
/// reconcile branches three ways on it (present / missing / transient); folding
/// it into `Other` would turn "transient -> skip" into "missing -> re-upload".
///
/// `Debug` is derived (not delegated to `Display`) so `{:?}` on an `Other`
/// preserves the full `anyhow` source chain.
#[derive(Debug)]
pub(crate) enum StorageError {
    NotFound,
    Other(anyhow::Error),
}

impl std::fmt::Display for StorageError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            StorageError::NotFound => write!(f, "not found"),
            StorageError::Other(e) => write!(f, "{e}"),
        }
    }
}

/// Abstraction over the remote object store. Implemented for opendal by
/// [`OpendalStorage`]; mocked in tests.
pub(crate) trait Storage: Send + Sync + 'static {
    /// Write `data` to `key` and return the backend's reported metadata.
    fn write(
        &self,
        key: &str,
        data: Vec<u8>,
    ) -> impl Future<Output = Result<WriteMeta, StorageError>> + Send;

    /// List object keys (full paths) under `prefix`.
    fn list(&self, prefix: &str) -> impl Future<Output = Result<Vec<String>, StorageError>> + Send;

    /// Probe a single object. `Ok(())` if present; `Err(StorageError::NotFound)`
    /// if absent; `Err(StorageError::Other)` for a transient/other failure.
    fn stat(&self, key: &str) -> impl Future<Output = Result<(), StorageError>> + Send;
}

/// opendal-backed [`Storage`]. Owns the `Operator` construction and retry layer.
#[derive(Clone)]
pub(crate) struct OpendalStorage {
    op: opendal::Operator,
}

impl OpendalStorage {
    /// Build from a storage URI, applying the standard retry layer. Parsing the
    /// URI here (not at `Ledger::new`) keeps opendal out of the ledger; the
    /// caller still gates construction on `storage.enabled` so a malformed URI
    /// can't abort a local-only deployment.
    pub(crate) fn new(uri: &str) -> std::io::Result<Self> {
        let retry_layer = opendal::layers::RetryLayer::new()
            .with_min_delay(Duration::from_secs(1))
            .with_max_delay(Duration::from_secs(30))
            .with_max_times(10)
            .with_factor(2.0)
            .with_jitter()
            .with_notify(|err: &opendal::Error, dur: Duration| {
                tracing::warn!(
                    "remote storage operation failed, retrying in {:.1}s: {err}",
                    dur.as_secs_f64(),
                );
            });
        let op = opendal::Operator::from_uri(uri)
            .map_err(|e| std::io::Error::other(e.to_string()))?
            .layer(retry_layer);
        Ok(Self { op })
    }

    /// Wrap an existing operator without a retry layer — for tests that drive a
    /// real `Fs` backend and want deterministic fast failures.
    #[cfg(test)]
    pub(crate) fn from_operator(op: opendal::Operator) -> Self {
        Self { op }
    }
}

impl From<opendal::Error> for StorageError {
    fn from(e: opendal::Error) -> Self {
        // Only the genuine NotFound kind maps to NotFound; the retry layer has
        // already exhausted transient retries before an error reaches here, so
        // everything else is a permanent/exhausted failure.
        if e.kind() == opendal::ErrorKind::NotFound {
            StorageError::NotFound
        } else {
            StorageError::Other(e.into())
        }
    }
}

impl Storage for OpendalStorage {
    async fn write(&self, key: &str, data: Vec<u8>) -> Result<WriteMeta, StorageError> {
        let meta = self.op.write(key, data).await?;
        Ok(WriteMeta {
            content_length: meta.content_length(),
            etag: meta.etag().map(str::to_owned),
        })
    }

    async fn list(&self, prefix: &str) -> Result<Vec<String>, StorageError> {
        let entries = self.op.list(prefix).await?;
        Ok(entries.into_iter().map(|e| e.path().to_owned()).collect())
    }

    async fn stat(&self, key: &str) -> Result<(), StorageError> {
        self.op.stat(key).await.map(|_| ()).map_err(StorageError::from)
    }
}

/// In-memory [`Storage`] for unit tests: configurable write size/failure and
/// stat outcome. Shared by the uploader and recovery test modules.
#[cfg(test)]
#[derive(Clone)]
pub(crate) struct MockStorage {
    /// If `Some`, `write` reports this `content_length`; if `None`, echoes the
    /// written byte count (the happy path).
    pub write_content_length: Option<u64>,
    /// If `Some`, `write` fails with this message.
    pub write_error: Option<String>,
    /// What `stat` returns.
    pub stat: MockStat,
    /// Object keys every `list` call returns (prefix is ignored). Lets tests
    /// drive `reconcile_remote_uploads` without a real backend.
    pub list_response: Vec<String>,
    /// If `Some`, `list` fails with this message — drives the LIST-error path.
    pub list_error: Option<String>,
}

#[cfg(test)]
#[derive(Clone, Copy)]
pub(crate) enum MockStat {
    Found,
    NotFound,
    Transient,
}

#[cfg(test)]
impl Default for MockStorage {
    fn default() -> Self {
        Self {
            write_content_length: None,
            write_error: None,
            stat: MockStat::NotFound,
            list_response: Vec::new(),
            list_error: None,
        }
    }
}

#[cfg(test)]
impl Storage for MockStorage {
    async fn write(&self, _key: &str, data: Vec<u8>) -> Result<WriteMeta, StorageError> {
        if let Some(msg) = &self.write_error {
            return Err(StorageError::Other(anyhow::anyhow!(msg.clone())));
        }
        Ok(WriteMeta {
            content_length: self.write_content_length.unwrap_or(data.len() as u64),
            etag: Some("mock-etag".to_owned()),
        })
    }

    async fn list(&self, prefix: &str) -> Result<Vec<String>, StorageError> {
        if let Some(msg) = &self.list_error {
            return Err(StorageError::Other(anyhow::anyhow!(msg.clone())));
        }
        // Honor the prefix like a real backend, so multi-day reconcile tests
        // see each key only under its own date prefix.
        Ok(self
            .list_response
            .iter()
            .filter(|key| key.starts_with(prefix))
            .cloned()
            .collect())
    }

    async fn stat(&self, _key: &str) -> Result<(), StorageError> {
        match self.stat {
            MockStat::Found => Ok(()),
            MockStat::NotFound => Err(StorageError::NotFound),
            MockStat::Transient => Err(StorageError::Other(anyhow::anyhow!("transient"))),
        }
    }
}
