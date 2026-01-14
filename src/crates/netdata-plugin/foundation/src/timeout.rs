//! Timeout management for async operations with thread-safe deadline modification.

use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

/// A thread-safe timeout that can be checked and extended from multiple threads.
///
/// This is useful for operations that need:
/// - Dynamic timeout extension (e.g., on progress updates)
/// - Parallel checks across multiple async tasks
/// - Future support for cancellation signals
///
/// Extensions are bounded: the remaining time will never exceed the initial budget.
#[derive(Debug, Clone)]
pub struct Timeout {
    start: Instant,
    budget_us: u64,
    deadline_us: Arc<AtomicU64>,
}

impl Timeout {
    /// Create a new timeout with the given budget.
    pub fn new(budget: Duration) -> Self {
        let start = Instant::now();
        let budget_us = budget.as_micros() as u64;

        Self {
            start,
            budget_us,
            deadline_us: Arc::new(AtomicU64::new(budget_us)),
        }
    }

    /// Check if the timeout has expired.
    pub fn is_expired(&self) -> bool {
        self.remaining().is_zero()
    }

    /// Get remaining time. Returns Duration::ZERO if expired.
    pub fn remaining(&self) -> Duration {
        let deadline_us = self.deadline_us.load(Ordering::Relaxed);
        let elapsed_us = self.start.elapsed().as_micros() as u64;

        if elapsed_us >= deadline_us {
            Duration::ZERO
        } else {
            Duration::from_micros(deadline_us - elapsed_us)
        }
    }

    /// Reset the timeout to the initial budget from the current time.
    ///
    /// This is useful for operations that report progress and should
    /// get their full timeout budget again.
    ///
    /// For example, if the initial timeout was 10 seconds and we're at t=5s with 5s remaining,
    /// calling reset() will give the operation another 10s (deadline becomes t=15s).
    pub fn reset(&self) {
        let elapsed_us = self.start.elapsed().as_micros() as u64;
        let deadline_us = elapsed_us + self.budget_us;
        self.deadline_us.store(deadline_us, Ordering::Relaxed);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::thread;

    #[test]
    fn test_timeout_not_expired() {
        let timeout = Timeout::new(Duration::from_secs(10));
        assert!(!timeout.is_expired());
        assert!(timeout.remaining() > Duration::ZERO);
    }

    #[test]
    fn test_timeout_expired() {
        let timeout = Timeout::new(Duration::from_micros(1));
        thread::sleep(Duration::from_millis(10));
        assert!(timeout.is_expired());
        assert_eq!(timeout.remaining(), Duration::ZERO);
    }

    #[test]
    fn test_timeout_reset() {
        let timeout = Timeout::new(Duration::from_millis(100));
        thread::sleep(Duration::from_millis(60));

        // Should have ~40ms remaining
        let remaining_before = timeout.remaining();
        assert!(remaining_before < Duration::from_millis(50));

        // Reset the timeout
        timeout.reset();

        // Should now have the full initial budget (~100ms) remaining
        let remaining_after = timeout.remaining();
        assert!(remaining_after >= Duration::from_millis(90));
        assert!(remaining_after <= Duration::from_millis(100));
    }

    #[test]
    fn test_timeout_clone_shared_deadline() {
        let timeout1 = Timeout::new(Duration::from_secs(10));
        let timeout2 = timeout1.clone();

        thread::sleep(Duration::from_millis(100));

        // Reset from one clone
        timeout1.reset();

        // Both should see the reset
        let remaining1 = timeout1.remaining();
        let remaining2 = timeout2.remaining();

        assert!((remaining1.as_secs() as i64 - remaining2.as_secs() as i64).abs() < 1);
        assert!(remaining1.as_secs() >= 9); // ~10 seconds (reset to initial budget)
    }

    #[tokio::test]
    async fn test_tokio_timeout_with_zero_duration() {
        use tokio::time::timeout;

        // Create an expired timeout
        let expired_timeout = Timeout::new(Duration::from_micros(1));
        thread::sleep(Duration::from_millis(10));
        assert_eq!(expired_timeout.remaining(), Duration::ZERO);

        // Verify tokio::time::timeout with ZERO duration times out immediately
        let result = timeout(expired_timeout.remaining(), async {
            tokio::time::sleep(Duration::from_millis(100)).await;
            "should_not_complete"
        })
        .await;

        // Should timeout immediately
        assert!(result.is_err(), "Expected timeout with Duration::ZERO");
    }

    #[tokio::test]
    async fn test_tokio_timeout_with_remaining_time() {
        use tokio::time::timeout;

        // Create a timeout with plenty of time
        let valid_timeout = Timeout::new(Duration::from_secs(10));
        assert!(valid_timeout.remaining() > Duration::ZERO);

        // Verify tokio::time::timeout with remaining time completes
        let result = timeout(valid_timeout.remaining(), async {
            tokio::time::sleep(Duration::from_millis(10)).await;
            "completed"
        })
        .await;

        // Should complete successfully
        assert!(result.is_ok(), "Expected completion with sufficient time");
        assert_eq!(result.unwrap(), "completed");
    }
}
