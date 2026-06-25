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
//! uniformly across all methods so any backend stays usable there.
//!
//! [`Component`]: crate::component::Component

use std::future::Future;
use std::time::Duration;

/// Metadata returned by a successful [`Storage::write`].
pub struct WriteMeta {
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
pub enum StorageError {
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
pub trait Storage: Send + Sync + 'static {
    /// Write `data` to `key` and return the backend's reported metadata.
    fn write(
        &self,
        key: &str,
        data: Vec<u8>,
    ) -> impl Future<Output = Result<WriteMeta, StorageError>> + Send;

    /// List object keys (full paths) under `prefix`.
    fn list(&self, prefix: &str) -> impl Future<Output = Result<Vec<String>, StorageError>> + Send;

    /// Read the whole object at `key` into memory. `NotFound` is preserved for
    /// parity with the other methods, so a caller can distinguish an absent object
    /// from a transient failure if it needs to.
    fn read(&self, key: &str) -> impl Future<Output = Result<Vec<u8>, StorageError>> + Send;

    /// Probe a single object. `Ok(())` if present; `Err(StorageError::NotFound)`
    /// if absent; `Err(StorageError::Other)` for a transient/other failure.
    fn stat(&self, key: &str) -> impl Future<Output = Result<(), StorageError>> + Send;
}

/// Sentinel key for the startup reachability probe. Never written; a `stat` on
/// it round-trips to the backend and exercises credentials.
const STORAGE_PROBE_KEY: &str = ".netdata-otel-storage-probe";

/// Startup connectivity probe: confirm the configured backend is reachable and
/// the credentials are accepted, without requiring any object to exist.
///
/// A `stat` on [`STORAGE_PROBE_KEY`] returning `Ok` or `NotFound` both mean the
/// request reached the backend and was authorized — `NotFound` is the expected
/// case, since the sentinel is never written. Only `Other` signals a real
/// problem (bad credentials, wrong bucket, unreachable endpoint).
///
/// The call runs through the operator's retry layer, so a *temporary* failure
/// (e.g. an unreachable endpoint) is retried before it surfaces, while permanent
/// failures (auth, missing bucket) are not retried by opendal and surface
/// immediately. Because that retry window can be minutes, callers MUST run this
/// off the startup path (see `Ledger::new`, which spawns it).
pub async fn probe_reachable<S: Storage>(storage: &S) -> Result<(), StorageError> {
    match storage.stat(STORAGE_PROBE_KEY).await {
        Ok(()) | Err(StorageError::NotFound) => Ok(()),
        Err(other) => Err(other),
    }
}

/// opendal-backed [`Storage`]. Owns the `Operator` construction and retry layer.
#[derive(Clone)]
pub struct OpendalStorage {
    op: opendal::Operator,
}

impl OpendalStorage {
    /// Build from a storage URI, applying the standard retry layer. Parsing the
    /// URI here (not at `Ledger::new`) keeps opendal out of the ledger; the
    /// caller still gates construction on `storage.enabled` so a malformed URI
    /// can't abort a local-only deployment.
    pub fn new(uri: &str) -> std::io::Result<Self> {
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
    pub fn from_operator(op: opendal::Operator) -> Self {
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

    async fn read(&self, key: &str) -> Result<Vec<u8>, StorageError> {
        let buf = self.op.read(key).await?;
        Ok(buf.to_vec())
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
    /// Bytes every `read` call returns (the happy path). Empty by default.
    pub read_response: Vec<u8>,
    /// If `Some`, `read` fails with this message — drives the fetch-error path.
    pub read_error: Option<String>,
    /// If `true`, `read` returns `NotFound` — drives the absent-object path.
    /// Takes priority over `read_error`; a real backend never returns both, so
    /// tests set exactly one error mode at a time.
    pub read_not_found: bool,
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
            read_response: Vec::new(),
            read_error: None,
            read_not_found: false,
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

    async fn read(&self, _key: &str) -> Result<Vec<u8>, StorageError> {
        if self.read_not_found {
            return Err(StorageError::NotFound);
        }
        if let Some(msg) = &self.read_error {
            return Err(StorageError::Other(anyhow::anyhow!(msg.clone())));
        }
        Ok(self.read_response.clone())
    }

    async fn stat(&self, _key: &str) -> Result<(), StorageError> {
        match self.stat {
            MockStat::Found => Ok(()),
            MockStat::NotFound => Err(StorageError::NotFound),
            MockStat::Transient => Err(StorageError::Other(anyhow::anyhow!("transient"))),
        }
    }
}

#[cfg(test)]
mod storage_tests {
    use super::*;

    #[tokio::test]
    async fn probe_treats_found_and_notfound_as_reachable() {
        // A present sentinel and an absent one both prove reachability+auth.
        let found = MockStorage {
            stat: MockStat::Found,
            ..Default::default()
        };
        assert!(probe_reachable(&found).await.is_ok());

        let absent = MockStorage {
            stat: MockStat::NotFound,
            ..Default::default()
        };
        assert!(probe_reachable(&absent).await.is_ok());
    }

    #[tokio::test]
    async fn probe_reports_transient_failure_as_error() {
        let down = MockStorage {
            stat: MockStat::Transient,
            ..Default::default()
        };
        assert!(matches!(
            probe_reachable(&down).await,
            Err(StorageError::Other(_))
        ));
    }

    #[tokio::test]
    async fn read_returns_configured_bytes() {
        let s = MockStorage {
            read_response: b"hello".to_vec(),
            ..Default::default()
        };
        assert_eq!(s.read("any/key").await.unwrap(), b"hello");
    }

    #[tokio::test]
    async fn read_error_maps_to_other() {
        let s = MockStorage {
            read_error: Some("boom".to_owned()),
            ..Default::default()
        };
        assert!(matches!(
            s.read("any/key").await,
            Err(StorageError::Other(_))
        ));
    }

    #[tokio::test]
    async fn read_can_report_not_found() {
        let s = MockStorage {
            read_not_found: true,
            ..Default::default()
        };
        assert!(matches!(s.read("any/key").await, Err(StorageError::NotFound)));
    }
}
