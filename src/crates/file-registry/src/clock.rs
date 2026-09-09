use std::time::{SystemTime, UNIX_EPOCH};

use crate::TimestampNs;

/// A wall clock that guarantees monotonically increasing nanosecond timestamps.
///
/// Two consecutive calls always produce strictly increasing values, even
/// when `SystemTime::now()` returns the same nanosecond twice or jitters
/// backward. Each instance is independent — share one across components
/// that need a coherent ordering relationship between their timestamps.
pub struct MonotonicClock {
    last_ns: u64,
}

impl MonotonicClock {
    pub fn new() -> Self {
        Self { last_ns: 0 }
    }

    /// Return the next strictly-increasing nanosecond timestamp.
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
