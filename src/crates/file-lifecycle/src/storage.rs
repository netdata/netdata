//! Remote object-storage seam.
//!
//! [`Storage`] is the single abstraction every remote access goes through —
//! the uploader's PUTs, the startup catalog diff-sync's LIST and downloads,
//! recovery's pre-mark LISTs/stats, and the query path's read-backs — so
//! opendal is confined to this module's [`OpendalStorage`] impl and the rest
//! of the crate stays backend-agnostic and unit-testable with a mock.
//!
//! Surface: `write` / `list` / `read` / `stat` and nothing else — there is no
//! delete. Objects age out through bucket-side lifecycle rules keyed on the
//! date-bucketed keys `remote_keys` builds (its module docs own the bucket
//! layout and that rationale). `list` is recursive over the whole `prefix`
//! sub-tree; `read` loads the whole object into memory; every error surfaces
//! as [`StorageError`].
//!
//! Native async-in-trait + generic dispatch (matching the crate's
//! [`Component`] trait) keeps calls monomorphized and zero-cost. The `+ Send`
//! bound on each returned future is needed because storage futures run in
//! spawned upload tasks (`write`) and under `join_all` during recovery
//! (`list`); it is applied uniformly across all methods so any backend stays
//! usable there. The trait bound `Send + Sync + 'static` makes a storage
//! handle shareable across tasks.
//!
//! [`OpendalStorage::new`] wraps the backend in opendal's retry layer — the
//! parameters and the ~3-minute per-operation backoff ceiling live on its
//! docs (the bound `bridge::config`'s `startup_op_timeout` should stay
//! above). The layer is transparent to callers; where a call must be bounded
//! tighter, the caller wraps it (`recovery::startup`'s per-op timeout,
//! `remote_read`'s per-download deadline).
//!
//! Journal safety: `StorageError`'s `Display` and the retry layer's notify
//! pass error text through `crate::redact`, so rendering an error into a
//! log line is credential-safe by construction; the `redact` module docs own
//! that contract.
//!
//! Consumers (grep-verified): `uploader` writes; `recovery::startup` LISTs
//! the catalog prefix and downloads bodies; `recovery::remote` LISTs per-day
//! SFST prefixes and stats catalogs, with `recovery::local`'s
//! `seed_from_catalog_files` re-fetching a corrupt body; `remote_read`'s
//! `RemoteRead<S: Storage>` (defaulting to [`OpendalStorage`]) reads objects
//! back through the shared download cache (`file-cache`). `otel-ledger`'s
//! `Ledger::new` constructs [`OpendalStorage`] behind
//! `remote_storage.enabled` and spawns [`probe_reachable`]; its pipeline
//! builders thread the cloned handle to recovery and each signal's
//! `RemoteRead`, and its tests drive `fs://` backends.
//!
//! [`Component`]: crate::component::Component

use std::future::Future;
use std::time::Duration;

use crate::redact::redact;

/// Metadata returned by a successful [`Storage::write`].
pub struct WriteMeta {
    /// Size the backend reports for the written object. Some backends report
    /// `0` (unknown); the uploader's size guard treats only a non-zero
    /// mismatch as a failed upload.
    pub content_length: u64,
    /// Backend-reported ETag (e.g. S3), recorded on the SFST's catalog entry
    /// via the uploader's `Uploaded` response; `None` when the backend does
    /// not supply one.
    pub etag: Option<String>,
}

/// Error surfaced by a [`Storage`] operation.
///
/// `NotFound` is preserved as its own variant because recovery's catalog
/// reconcile branches three ways on it (present / missing / transient); folding
/// it into `Other` would turn "transient -> skip" into "missing -> re-upload".
///
/// `Display` renders the FULL error source chain with URL query strings
/// stripped (see `crate::redact`) — safe to log, and complete enough to
/// diagnose (reqwest renders only a generic top-level message such as "error
/// sending request"; the cause is visible only deeper in the chain). `Debug`
/// is derived and UNREDACTED — it exists for test assertions and must not be
/// used to log real backend errors. Converting the error into another type
/// MUST flatten through this `Display` (`remote_read::read_error_to_anyhow`
/// is the pattern); the `crate::redact` module docs own the rationale.
#[derive(Debug)]
pub enum StorageError {
    NotFound,
    Other(anyhow::Error),
}

impl std::fmt::Display for StorageError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            StorageError::NotFound => write!(f, "not found"),
            StorageError::Other(e) => write!(f, "{}", redact(&format!("{e:#}"))),
        }
    }
}

/// Abstraction over the remote object store. Implemented for opendal by
/// [`OpendalStorage`]; mocked in tests (`MockStorage`, `cfg(test)`).
///
/// Keys are full object paths in the scheme `remote_keys` builds; the trait
/// treats them as opaque strings. There is no delete: objects age out through
/// bucket-side lifecycle rules keyed on the date-bucketed layout
/// (`remote_keys`' module docs own the rationale).
pub trait Storage: Send + Sync + 'static {
    /// Overwrite `key` with `data` and return the backend's reported
    /// metadata. Writes are idempotent overwrites at deterministic keys,
    /// which is what lets the uploader re-drive an abandoned upload on the
    /// next restart.
    fn write(
        &self,
        key: &str,
        data: Vec<u8>,
    ) -> impl Future<Output = Result<WriteMeta, StorageError>> + Send;

    /// List object keys (full paths) under `prefix`, RECURSIVELY — every object
    /// at any depth below `prefix`, not just the immediate level. Directory
    /// placeholder entries (keys ending in `/`) may be returned by some backends
    /// and MUST be filtered by callers that want objects only. Leaf-prefix
    /// callers (`recovery::remote`'s per-day SFST prefixes) are unaffected by
    /// the recursive contract; the startup catalog sync depends on it (one
    /// LIST of the whole `catalog/` prefix spanning many date/tenant
    /// subdirectories).
    fn list(&self, prefix: &str) -> impl Future<Output = Result<Vec<String>, StorageError>> + Send;

    /// Read the whole object at `key` into memory — peak memory is the
    /// object's size (the shared download cache is sized on that basis).
    /// `NotFound` is preserved for parity with the other methods, so a caller
    /// can distinguish an absent object from a transient failure if it needs
    /// to. The retry layer alone can spend minutes on one call, so callers
    /// that need a bound wrap it (`remote_read`'s per-download deadline,
    /// `recovery::startup`'s per-op timeout).
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
/// A `stat` on `STORAGE_PROBE_KEY` returning `Ok` or `NotFound` both mean the
/// request reached the backend and was authorized — `NotFound` is the expected
/// case, since the sentinel is never written. Only `Other` signals a real
/// problem (bad credentials, wrong bucket, unreachable endpoint).
///
/// The call runs through the operator's retry layer, so a *temporary* failure
/// (e.g. an unreachable endpoint) is retried before it surfaces, while permanent
/// failures (auth, missing bucket) are not retried by opendal and surface
/// immediately. Because that retry window can run to the ~3-minute backoff
/// ceiling (module docs), callers MUST run this off the startup path (see
/// `Ledger::new`, which spawns it). The sole caller treats the result as a
/// diagnostic — logged, never fatal.
pub async fn probe_reachable<S: Storage>(storage: &S) -> Result<(), StorageError> {
    match storage.stat(STORAGE_PROBE_KEY).await {
        Ok(()) | Err(StorageError::NotFound) => Ok(()),
        Err(other) => Err(other),
    }
}

/// opendal-backed [`Storage`] — the production backend, constructed once per
/// process when remote storage is enabled (`Ledger::new`). Owns the
/// `Operator` construction and retry layer.
///
/// Cheap to clone (the `Operator` is `Arc`-backed): the ledger shares one
/// handle across the uploader's args, the reachability probe task, and each
/// signal's `RemoteRead`.
#[derive(Clone)]
pub struct OpendalStorage {
    op: opendal::Operator,
}

impl OpendalStorage {
    /// Build from a storage URI (e.g. `fs:///path`, `s3://bucket/?region=…`),
    /// wrapped in opendal's retry layer: exponential backoff 1 s → 30 s with
    /// jitter, at most 10 attempts — a worst-case ~3-minute backoff ceiling
    /// per operation, which `bridge::config`'s `startup_op_timeout` should
    /// stay above. Each retry is logged at `warn` with the redacted error
    /// text. Err only on URI parse failure, as a raw `std::io::Error`
    /// carrying opendal's error display — not a [`StorageError`], so it does
    /// not pass through the `Display` redaction.
    ///
    /// Parsing the URI here (not at `Ledger::new`) keeps opendal out of the
    /// ledger; the caller still gates construction on `remote_storage.enabled`
    /// so a malformed URI can't abort a local-only deployment.
    pub fn new(uri: &str) -> std::io::Result<Self> {
        // Worst-case backoff sum: 1+2+4+8+16+30·5 s ≈ 181 s ≈ 3 min per op.
        let retry_layer = opendal::layers::RetryLayer::new()
            .with_min_delay(Duration::from_secs(1))
            .with_max_delay(Duration::from_secs(30))
            .with_max_times(10)
            .with_factor(2.0)
            .with_jitter()
            .with_notify(|event: opendal::layers::RetryEvent<'_>| {
                tracing::warn!(
                    op = %event.op,
                    attempt = event.attempt,
                    "remote storage operation failed, retrying in {:.1}s: {}",
                    event.retry_after.as_secs_f64(),
                    redact(&event.err.to_string()),
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
        // `recursive(true)` walks the whole sub-tree under `prefix`; opendal's
        // plain `list` returns one level only. This is the mechanism behind the
        // trait's recursive contract.
        let entries = self.op.list_with(prefix).recursive(true).await?;
        Ok(entries.into_iter().map(|e| e.path().to_owned()).collect())
    }

    async fn read(&self, key: &str) -> Result<Vec<u8>, StorageError> {
        let buf = self.op.read(key).await?;
        Ok(buf.to_vec())
    }

    async fn stat(&self, key: &str) -> Result<(), StorageError> {
        self.op
            .stat(key)
            .await
            .map(|_| ())
            .map_err(StorageError::from)
    }
}

/// In-memory [`Storage`] for unit tests: configurable write/list/read/stat
/// outcomes. Shared by the uploader, recovery, and `remote_read` test modules.
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
    /// Object keys every `list` call returns, filtered by `starts_with(prefix)`
    /// like a real backend, so multi-day reconcile tests see each key only
    /// under its own date prefix. Lets tests drive `reconcile_remote_uploads`
    /// without a real backend.
    pub list_response: Vec<String>,
    /// If `Some`, `list` fails with this message — drives the LIST-error path.
    pub list_error: Option<String>,
    /// Per-key read bodies. A `read(key)` hitting this map returns its bytes;
    /// otherwise it falls back to `read_response`. Lets the diff-sync tests give
    /// each catalog object its own container bytes.
    pub read_bodies: std::collections::HashMap<String, Vec<u8>>,
    /// Bytes every `read` call returns when `read_bodies` has no entry for the
    /// key (the single-blob happy path). Empty by default.
    pub read_response: Vec<u8>,
    /// If `Some`, `read` fails with this message — drives the fetch-error path.
    pub read_error: Option<String>,
    /// If `true`, `read` returns `NotFound` — drives the absent-object path.
    /// Takes priority over `read_error`; a real backend never returns both, so
    /// tests set exactly one error mode at a time.
    pub read_not_found: bool,
    /// Counts `read` calls (shared, so a `.clone()` of the mock still counts) —
    /// lets recovery tests assert download counts (the no-op startup sync
    /// asserts zero).
    pub read_calls: std::sync::Arc<std::sync::atomic::AtomicUsize>,
    /// Per-key read failures, checked before every other read mode.
    pub read_key_errors: std::collections::HashMap<String, MockReadError>,
    /// Per-key read delays (`tokio::time::sleep`, so paused-clock tests can
    /// drive download deadlines).
    pub read_delays: std::collections::HashMap<String, Duration>,
}

/// A per-key [`MockStorage`] read failure.
#[cfg(test)]
#[derive(Clone, Copy)]
pub(crate) enum MockReadError {
    NotFound,
    Other,
}

/// What [`MockStorage`]'s `stat` returns: `Found` → `Ok(())`, `NotFound` →
/// `StorageError::NotFound`, `Transient` → `Other` (drives the probe's
/// transient-error path and recovery's log-and-skip).
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
            read_bodies: std::collections::HashMap::new(),
            read_response: Vec::new(),
            read_error: None,
            read_not_found: false,
            read_calls: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            read_key_errors: std::collections::HashMap::new(),
            read_delays: std::collections::HashMap::new(),
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
        // Flat `starts_with` over the pre-populated `list_response`: recursive-in
        // -effect, but it does NOT prove the real backend's `recursive(true)`
        // (a non-recursive `OpendalStorage::list` would still pass mock tests).
        // The real recursive contract is covered by `startup_sync_restores_after
        // _wipe`, which runs an fs-backed `OpendalStorage` over nested dirs.
        Ok(self
            .list_response
            .iter()
            .filter(|key| key.starts_with(prefix))
            .cloned()
            .collect())
    }

    async fn read(&self, key: &str) -> Result<Vec<u8>, StorageError> {
        self.read_calls
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        if let Some(delay) = self.read_delays.get(key) {
            tokio::time::sleep(*delay).await;
        }
        match self.read_key_errors.get(key) {
            Some(MockReadError::NotFound) => return Err(StorageError::NotFound),
            Some(MockReadError::Other) => {
                return Err(StorageError::Other(anyhow::anyhow!("mock read failure")));
            }
            None => {}
        }
        if self.read_not_found {
            return Err(StorageError::NotFound);
        }
        if let Some(msg) = &self.read_error {
            return Err(StorageError::Other(anyhow::anyhow!(msg.clone())));
        }
        if let Some(body) = self.read_bodies.get(key) {
            return Ok(body.clone());
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

    #[test]
    fn display_renders_redacted_full_chain() {
        // The chain's inner cause embeds a credential-bearing URL — Display
        // must surface BOTH levels (diagnosability) with the query stripped
        // (credential safety).
        let inner = anyhow::anyhow!(
            "error sending request for url (https://sts.amazonaws.com/?Action=AssumeRoleWithWebIdentity&WebIdentityToken=SENTINEL_JWT)"
        );
        let err = StorageError::Other(inner.context("upload failed"));
        let rendered = err.to_string();
        assert!(!rendered.contains("SENTINEL_JWT"), "leaked: {rendered}");
        assert!(
            rendered.contains("upload failed"),
            "outer context lost: {rendered}"
        );
        assert!(
            rendered
                .contains("error sending request for url (https://sts.amazonaws.com/?[REDACTED])"),
            "inner cause lost or unredacted: {rendered}"
        );
    }

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
        assert!(matches!(
            s.read("any/key").await,
            Err(StorageError::NotFound)
        ));
    }
}
