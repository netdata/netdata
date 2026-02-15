use std::time::{SystemTime, UNIX_EPOCH};

/// A wall clock that guarantees monotonically increasing nanosecond timestamps.
pub(crate) struct MonotonicClock {
    pub(crate) last_ns: u64,
}

impl MonotonicClock {
    pub fn new() -> Self {
        Self { last_ns: 0 }
    }

    pub fn now_ns(&mut self) -> u64 {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock before Unix epoch")
            .as_nanos() as u64;

        self.last_ns = self.last_ns.max(now.saturating_sub(1)) + 1;
        self.last_ns
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn monotonicity() {
        let mut clock = MonotonicClock::new();
        let mut prev = 0;
        for _ in 0..10_000 {
            let ts = clock.now_ns();
            assert!(ts > prev, "timestamp must be strictly increasing");
            prev = ts;
        }
    }
}
