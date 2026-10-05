//! Minimal, always-on perf tracking for the index build: named phase timers plus
//! byte/frame/record counters ([`Metrics`]), summarized by [`Metrics::report`] at
//! end of run, plus a Linux-only RSS reader ([`read_rss`]) for the peak-memory line.
//!
//! Stages time themselves with [`Metrics::scope`] and feed the counters as work
//! happens — the `scope` names and `add_*` calls live in `sfst_build.rs`, and the
//! `ng-index` binary prints the report. Timing is per phase/frame, never per
//! record, so it does not perturb the work it measures.

use std::cell::{Cell, RefCell};
use std::time::{Duration, Instant};

/// Current and peak resident set size in KiB, from `/proc/self/status`.
#[derive(Debug, Clone, Copy)]
pub struct Rss {
    /// `VmRSS`: resident memory now.
    pub current_kb: u64,
    /// `VmHWM`: peak resident memory since process start.
    pub peak_kb: u64,
}

/// Read current/peak RSS. `None` off Linux or if the fields are absent.
#[cfg(target_os = "linux")]
pub fn read_rss() -> Option<Rss> {
    let status = std::fs::read_to_string("/proc/self/status").ok()?;
    let field = |key: &str| -> Option<u64> {
        status
            .lines()
            .find(|l| l.starts_with(key))?
            .split_whitespace()
            .nth(1)?
            .parse()
            .ok()
    };
    Some(Rss {
        current_kb: field("VmRSS:")?,
        peak_kb: field("VmHWM:")?,
    })
}

#[cfg(not(target_os = "linux"))]
pub fn read_rss() -> Option<Rss> {
    None
}

/// Per-run phase timings and throughput counters.
///
/// Single-threaded by construction (`Cell`/`RefCell` are not `Sync`). Every method
/// takes `&self` (interior mutability) so a live `Scope` guard and counter
/// updates coexist without borrow conflicts.
pub struct Metrics {
    start: Instant,
    phases: RefCell<Vec<(&'static str, Duration)>>,
    bytes: Cell<u64>,
    frames: Cell<u64>,
    records: Cell<u64>,
}

impl Metrics {
    pub fn new() -> Self {
        Self {
            start: Instant::now(),
            phases: RefCell::new(Vec::new()),
            bytes: Cell::new(0),
            frames: Cell::new(0),
            records: Cell::new(0),
        }
    }

    /// Start timing a phase; the elapsed time is added to `name` when the
    /// returned guard drops.
    pub fn scope(&self, name: &'static str) -> Scope<'_> {
        Scope {
            metrics: self,
            name,
            start: Instant::now(),
        }
    }

    /// WAL frame payload bytes read (decompressed), for the MiB/s throughput line.
    pub fn add_bytes(&self, n: u64) {
        self.bytes.set(self.bytes.get() + n);
    }

    /// WAL frames read (one flattened request per frame).
    pub fn add_frames(&self, n: u64) {
        self.frames.set(self.frames.get() + n);
    }

    /// Records indexed — log records in the logs path, spans in the traces path —
    /// feeding the logs/s rates in [`Metrics::report`].
    pub fn add_records(&self, n: u64) {
        self.records.set(self.records.get() + n);
    }

    fn add_phase(&self, name: &'static str, elapsed: Duration) {
        let mut phases = self.phases.borrow_mut();
        match phases.iter_mut().find(|(n, _)| *n == name) {
            Some(slot) => slot.1 += elapsed,
            None => phases.push((name, elapsed)),
        }
    }

    /// A human-readable summary: total wall time, per-phase time with its share,
    /// and overall throughput. Phases appear in first-seen order.
    pub fn report(&self) -> String {
        let total = self.start.elapsed();
        let secs = total.as_secs_f64();
        let records = self.records.get();
        let bytes = self.bytes.get();

        let mut out = format!(
            "perf: {:.3}s total | {} frames | {} records\n",
            secs,
            self.frames.get(),
            records,
        );
        for (name, elapsed) in self.phases.borrow().iter() {
            let phase_secs = elapsed.as_secs_f64();
            let share = if secs > 0.0 {
                phase_secs / secs * 100.0
            } else {
                0.0
            };
            // Total records ÷ that phase's time — the key number when judging an
            // optimization on a single stage; exact for the phase that counts records.
            let logs_per_s = if phase_secs > 0.0 {
                records as f64 / phase_secs
            } else {
                0.0
            };
            out.push_str(&format!(
                "  {name:<18} {phase_secs:>8.3}s  {share:>5.1}%  {logs_per_s:>12.0} logs/s\n",
            ));
        }
        if secs > 0.0 {
            // End-to-end headline: total records over total wall time. Adding
            // phases only lengthens the denominator, so the figure stays comparable.
            out.push_str(&format!(
                "throughput: {:.0} logs/s (end-to-end)  |  {:.1} MiB/s decoded\n",
                records as f64 / secs,
                bytes as f64 / secs / (1024.0 * 1024.0),
            ));
        }
        out
    }
}

impl Default for Metrics {
    fn default() -> Self {
        Self::new()
    }
}

/// RAII timer: adds its lifetime's elapsed time to a named phase on drop.
pub struct Scope<'a> {
    metrics: &'a Metrics,
    name: &'static str,
    start: Instant,
}

impl Drop for Scope<'_> {
    fn drop(&mut self) {
        self.metrics.add_phase(self.name, self.start.elapsed());
    }
}
