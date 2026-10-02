//! The strictly increasing wall clock of the substrate: the one source of
//! [`TimestampNs`] values with an ordering guarantee. A bare `TimestampNs`
//! carries none (the `types` module owns that contract), so anything that
//! must order timestamps against each other — WAL frame `ingestion_ns`,
//! fallback stamps for records without a usable time — takes its values from
//! one shared [`MonotonicClock`] instance.
//!
//! Not thread-safe by design: [`now_ns`](MonotonicClock::now_ns) takes
//! `&mut self`, with no interior mutability and no locking, so exclusive
//! access is the caller's problem. `otel-ingestor` serves the logs and
//! traces services from one process-wide instance behind
//! `Arc<Mutex<MonotonicClock>>` — the single-clock contract the wal writer
//! pins in `wal::writer::FrameMeta` — while the single-owner `ng-ingest`
//! binaries keep one instance each.
//!
//! Not the per-process file `seq` counter: `seq` orders files and is
//! re-seeded from disk at startup
//! ([`scan_max_sequence_recursive`](crate::scan_max_sequence_recursive));
//! this clock orders timestamps and never seeds from disk.
//!
//! Consumers (grep-verified): `otel-ingestor` (the shared clock above,
//! created in `src/crates/otel-ingestor/src/lib.rs` `run_ingestor` —
//! frame stamps, the ingestion-window base, fallback stamps,
//! idle-rotation sweeps),
//! `ng-ingest` (per-binary instances feeding `write_request`), `wal`
//! (consumes the resulting `TimestampNs` values; docs-only reference to this
//! type), and test/fixture code in `sfsq`, `otel-ledger`, `ng-index`, and
//! `ng-ingest` that needs realistic ordered stamps.
use std::time::{SystemTime, UNIX_EPOCH};

use crate::TimestampNs;

/// A wall clock whose nanosecond timestamps strictly increase across calls:
/// every value from one instance is strictly above every earlier value from
/// that same instance, and values from different instances carry no ordering
/// relationship.
///
/// Mechanically, each call samples the wall clock and returns
/// `max(last + 1, now)`, where `last` is the highest value the instance has
/// handed out (zero on a fresh clock, so the first call returns the wall
/// time itself). A stalled or backward-jumping system clock — the same
/// nanosecond twice, NTP stepping the clock back — is bridged by inventing
/// `last + 1`, so the output stays a usable wall timestamp: it tracks real
/// time whenever the system clock does and only runs ahead of it while the
/// clock misbehaves.
pub struct MonotonicClock {
    last_ns: u64,
}

impl MonotonicClock {
    pub fn new() -> Self {
        Self { last_ns: 0 }
    }

    /// The next strictly increasing timestamp for this instance (mechanism in
    /// the struct docs). The only failure mode is a panic when the system
    /// clock is before the Unix epoch — not a runtime condition on a working
    /// host. The value fits `u64` until roughly year 2554, where the
    /// `u128`→`u64` cast truncates.
    pub fn now_ns(&mut self) -> TimestampNs {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock before Unix epoch")
            .as_nanos() as u64;

        self.last_ns = self.last_ns.max(now.saturating_sub(1)) + 1;
        TimestampNs(self.last_ns)
    }
}

impl Default for MonotonicClock {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn monotonicity() {
        let mut clock = MonotonicClock::new();
        let mut prev = TimestampNs(0);
        for _ in 0..10_000 {
            let ts = clock.now_ns();
            assert!(ts.0 > prev.0, "timestamp must be strictly increasing");
            prev = ts;
        }
    }
}
