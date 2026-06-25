//! Steady-state retry of failed uploads.
//!
//! `*Failed` uploader responses were previously logged and dropped, so a
//! transient remote outage stranded those files until the next process restart
//! re-drove `recover_unuploaded`. This queue re-issues failed SFST and catalog
//! uploads with capped exponential backoff, so uploads resume automatically
//! once the remote recovers.
//!
//! Per project decision, a *persistently* failing remote is allowed to
//! accumulate local files without bound (no disk ceiling); the operator is the
//! one responsible for fixing the remote, and is alerted via the warn/error
//! logs the retry tick emits.

use std::collections::HashMap;
use std::time::Duration;

use tokio::time::Instant;

use crate::ipc::UploaderRequest;

/// First retry delay; doubles each attempt up to [`MAX_BACKOFF`].
const BASE_BACKOFF: Duration = Duration::from_secs(30);
const MAX_BACKOFF: Duration = Duration::from_secs(600);
/// Attempt count past which the remote is treated as persistently unreachable
/// and the per-tick log escalates from `warn` to `error`.
pub const PERSISTENT_FAILURE_ATTEMPTS: u32 = 5;

#[derive(PartialEq, Eq, Hash, Clone)]
enum Key {
    Sfst(u64),
    Catalog(String),
}

struct Item {
    req: UploaderRequest,
    attempts: u32,
    next_attempt: Instant,
    /// A re-issued upload is awaiting its response; don't re-fire it until that
    /// response arrives (success removes it; failure re-arms it).
    in_flight: bool,
}

/// Re-issue queue for failed uploads, keyed so repeated failures of the same
/// file coalesce (bumping its backoff) instead of accumulating duplicates.
#[derive(Default)]
pub struct UploadRetry {
    items: HashMap<Key, Item>,
}

impl UploadRetry {
    pub fn len(&self) -> usize {
        self.items.len()
    }

    pub fn is_empty(&self) -> bool {
        self.items.is_empty()
    }

    /// Highest attempt count across pending items (0 if empty).
    pub fn max_attempts(&self) -> u32 {
        self.items.values().map(|i| i.attempts).max().unwrap_or(0)
    }

    /// Record a failed upload for retry. Repeated failures of the same file
    /// bump its attempt count (longer backoff) rather than duplicating it.
    pub fn record_failure(&mut self, req: UploaderRequest, now: Instant) {
        let key = Self::key_of(&req);
        let attempts = self
            .items
            .get(&key)
            .map(|i| i.attempts)
            .unwrap_or(0)
            .saturating_add(1);
        let next_attempt = now + backoff(attempts);
        self.items.insert(
            key,
            Item {
                req,
                attempts,
                next_attempt,
                in_flight: false,
            },
        );
    }

    /// Remove any pending retry for an SFST — either it uploaded successfully,
    /// or it can no longer be retried (its local file is gone).
    pub fn clear_sfst(&mut self, seq: u64) {
        self.items.remove(&Key::Sfst(seq));
    }

    /// Remove any pending retry for a catalog — uploaded successfully, or no
    /// longer retryable (its local file is gone).
    pub fn clear_catalog(&mut self, remote_key: &str) {
        self.items.remove(&Key::Catalog(remote_key.to_string()));
    }

    /// Requests whose backoff has elapsed and that aren't already in flight.
    /// Marks them in flight; they stay queued until a success removes them or a
    /// fresh failure re-arms them.
    pub fn take_due(&mut self, now: Instant) -> Vec<UploaderRequest> {
        let mut due = Vec::new();
        for item in self.items.values_mut() {
            if !item.in_flight && item.next_attempt <= now {
                item.in_flight = true;
                due.push(item.req.clone());
            }
        }
        due
    }

    fn key_of(req: &UploaderRequest) -> Key {
        match req {
            UploaderRequest::Upload { seq, .. } => Key::Sfst(*seq),
            UploaderRequest::UploadCatalog { remote_key, .. } => Key::Catalog(remote_key.clone()),
        }
    }
}

/// Capped exponential backoff: 30s, 60s, 120s, … up to 10 min.
fn backoff(attempts: u32) -> Duration {
    let shift = attempts.saturating_sub(1).min(5);
    let secs = BASE_BACKOFF.as_secs().saturating_mul(1u64 << shift);
    Duration::from_secs(secs.min(MAX_BACKOFF.as_secs()))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn upload(seq: u64) -> UploaderRequest {
        UploaderRequest::Upload {
            pipeline_id: 0,
            seq,
            local_path: format!("/tmp/{seq}.sfst").into(),
            remote_key: format!("k{seq}"),
        }
    }

    #[tokio::test]
    async fn failure_then_due_after_backoff_then_success_clears() {
        let mut q = UploadRetry::default();
        let t0 = Instant::now();

        q.record_failure(upload(1), t0);
        assert_eq!(q.len(), 1);
        // Not due before the first backoff window.
        assert!(q.take_due(t0).is_empty());

        // Due after 30s; taking marks it in flight so it won't re-fire.
        let due = q.take_due(t0 + Duration::from_secs(31));
        assert_eq!(due.len(), 1);
        assert!(q.take_due(t0 + Duration::from_secs(31)).is_empty());

        // Success (or abandonment) removes it.
        q.clear_sfst(1);
        assert!(q.is_empty());
    }

    #[tokio::test]
    async fn repeated_failures_coalesce_and_grow_backoff() {
        let mut q = UploadRetry::default();
        let t0 = Instant::now();

        q.record_failure(upload(7), t0);
        // Re-fire after the first window, then fail again.
        assert_eq!(q.take_due(t0 + Duration::from_secs(31)).len(), 1);
        q.record_failure(upload(7), t0 + Duration::from_secs(31));
        // Still a single coalesced entry, now with a longer (60s) backoff.
        assert_eq!(q.len(), 1);
        assert_eq!(q.max_attempts(), 2);
        assert!(
            q.take_due(t0 + Duration::from_secs(31 + 31)).is_empty(),
            "second attempt must wait the longer backoff"
        );
        assert_eq!(q.take_due(t0 + Duration::from_secs(31 + 61)).len(), 1);
    }
}
