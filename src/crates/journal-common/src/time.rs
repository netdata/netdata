//! Time units for journal timestamps.
//!
//! Provides type-safe wrappers for seconds and microseconds to prevent unit confusion.

use serde::{Deserialize, Serialize};
use std::cell::Cell;
use std::ops::{Add, Rem, Sub};

/// Timestamp in seconds since Unix epoch.
///
/// Used for histogram buckets, time ranges, and coarse-grained time operations.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Seconds(pub u32);

/// Timestamp in microseconds since Unix epoch.
///
/// Used for journal entry timestamps and fine-grained time operations.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Microseconds(pub u64);

impl Seconds {
    /// Create a timestamp from seconds.
    pub fn new(seconds: u32) -> Self {
        Self(seconds)
    }

    /// Get the current time as seconds since Unix epoch.
    pub fn now() -> Self {
        Self(
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time must be after UNIX_EPOCH")
                .as_secs() as u32,
        )
    }

    /// Get the raw seconds value.
    pub fn get(self) -> u32 {
        self.0
    }

    /// Convert to microseconds.
    pub fn to_microseconds(self) -> Microseconds {
        Microseconds(self.0 as u64 * 1_000_000)
    }

    /// Add two durations with saturation at the numeric bounds.
    pub fn saturating_add(self, other: Self) -> Self {
        Seconds(self.0.saturating_add(other.0))
    }

    /// Subtract two durations with saturation at the numeric bounds.
    pub fn saturating_sub(self, other: Self) -> Self {
        Seconds(self.0.saturating_sub(other.0))
    }

    /// Checked addition. Returns None if overflow occurred.
    pub fn checked_add(self, other: Self) -> Option<Self> {
        self.0.checked_add(other.0).map(Seconds)
    }

    /// Checked subtraction. Returns None if overflow occurred.
    pub fn checked_sub(self, other: Self) -> Option<Self> {
        self.0.checked_sub(other.0).map(Seconds)
    }

    /// Returns true if this duration is a multiple of the other duration.
    ///
    /// Useful for checking if bucket durations align.
    pub fn is_multiple_of(self, other: Self) -> bool {
        other.0 != 0 && self.0 % other.0 == 0
    }
}

impl Microseconds {
    /// Create a timestamp from microseconds.
    pub fn new(microseconds: u64) -> Self {
        Self(microseconds)
    }

    /// Get the current time as microseconds since Unix epoch.
    pub fn now() -> Self {
        Self(
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time must be after UNIX_EPOCH")
                .as_micros() as u64,
        )
    }

    /// Get the raw microseconds value.
    pub fn get(self) -> u64 {
        self.0
    }

    /// Convert to seconds (truncates).
    pub fn to_seconds(self) -> Seconds {
        Seconds((self.0 / 1_000_000) as u32)
    }

    /// Add two durations with saturation at the numeric bounds.
    pub fn saturating_add(self, other: Self) -> Self {
        Microseconds(self.0.saturating_add(other.0))
    }

    /// Subtract two durations with saturation at the numeric bounds.
    pub fn saturating_sub(self, other: Self) -> Self {
        Microseconds(self.0.saturating_sub(other.0))
    }

    /// Checked addition. Returns None if overflow occurred.
    pub fn checked_add(self, other: Self) -> Option<Self> {
        self.0.checked_add(other.0).map(Microseconds)
    }

    /// Checked subtraction. Returns None if overflow occurred.
    pub fn checked_sub(self, other: Self) -> Option<Self> {
        self.0.checked_sub(other.0).map(Microseconds)
    }

    /// Returns true if this duration is a multiple of the other duration.
    ///
    /// Useful for checking if bucket durations align.
    pub fn is_multiple_of(self, other: Self) -> bool {
        other.0 != 0 && self.0 % other.0 == 0
    }
}

impl From<Seconds> for Microseconds {
    fn from(s: Seconds) -> Self {
        s.to_microseconds()
    }
}

impl From<u32> for Seconds {
    fn from(s: u32) -> Self {
        Seconds(s)
    }
}

impl From<u64> for Microseconds {
    fn from(us: u64) -> Self {
        Microseconds(us)
    }
}

// Arithmetic operators for Seconds
impl Add for Seconds {
    type Output = Self;

    fn add(self, other: Self) -> Self {
        Seconds(self.0 + other.0)
    }
}

impl Sub for Seconds {
    type Output = Self;

    fn sub(self, other: Self) -> Self {
        Seconds(self.0 - other.0)
    }
}

impl Rem for Seconds {
    type Output = Self;

    fn rem(self, other: Self) -> Self {
        Seconds(self.0 % other.0)
    }
}

// Arithmetic operators for Microseconds
impl Add for Microseconds {
    type Output = Self;

    fn add(self, other: Self) -> Self {
        Microseconds(self.0 + other.0)
    }
}

impl Sub for Microseconds {
    type Output = Self;

    fn sub(self, other: Self) -> Self {
        Microseconds(self.0 - other.0)
    }
}

impl Rem for Microseconds {
    type Output = Self;

    fn rem(self, other: Self) -> Self {
        Microseconds(self.0 % other.0)
    }
}

impl std::fmt::Display for Seconds {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}s", self.0)
    }
}

impl std::fmt::Display for Microseconds {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}µs", self.0)
    }
}

/// A monotonic realtime clock that ensures timestamps always move forward.
///
/// Wraps `SystemTime` but guarantees each `now()` call returns a timestamp
/// strictly greater than all previous calls, even if the system clock jumps
/// backwards. When the clock goes backwards, it increments from the last seen
/// timestamp by one microsecond.
#[derive(Debug)]
pub struct RealtimeClock {
    max_seen: Cell<u64>,
}

impl RealtimeClock {
    /// Create a new realtime clock initialized with the current system time.
    pub fn new() -> Self {
        Self::with_initial(Microseconds::now())
    }

    /// Create a realtime clock initialized with a specific timestamp.
    ///
    /// Useful for resuming from a persisted state (e.g., last journal entry).
    pub fn with_initial(initial: Microseconds) -> Self {
        Self {
            max_seen: Cell::new(initial.get()),
        }
    }

    /// Get the current monotonic timestamp in microseconds since Unix epoch.
    ///
    /// Returns system time if it moved forward, otherwise returns last seen + 1µs.
    pub fn now(&self) -> Microseconds {
        let current = Microseconds::now();
        let max = self.max_seen.get();

        let next = if current.get() > max {
            current.get()
        } else {
            max.saturating_add(1)
        };

        self.max_seen.set(next);
        Microseconds::new(next)
    }

    /// Get the last seen timestamp without advancing the clock.
    pub fn last_seen(&self) -> Microseconds {
        Microseconds::new(self.max_seen.get())
    }
}

impl Default for RealtimeClock {
    fn default() -> Self {
        Self::new()
    }
}

/// Gets the current monotonic timestamp in microseconds since boot.
///
/// Uses CLOCK_MONOTONIC which provides a monotonically increasing timestamp
/// that is not affected by system clock adjustments but does not count time
/// when the system is suspended.
///
/// This matches systemd's behavior for journal entry monotonic timestamps.
pub fn monotonic_now() -> std::io::Result<Microseconds> {
    use nix::sys::time::TimeValLike;
    use nix::time::ClockId;

    let ts = ClockId::CLOCK_MONOTONIC
        .now()
        .map_err(|e| std::io::Error::from_raw_os_error(e as i32))?;

    Ok(Microseconds::new(ts.num_microseconds() as u64))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_seconds_to_microseconds() {
        let seconds = Seconds::new(42);
        let micros = seconds.to_microseconds();
        assert_eq!(micros.get(), 42_000_000);
    }

    #[test]
    fn test_microseconds_to_seconds() {
        let micros = Microseconds::new(42_500_000);
        let seconds = micros.to_seconds();
        assert_eq!(seconds.get(), 42);
    }

    #[test]
    fn test_conversion_roundtrip() {
        let original = Seconds::new(100);
        let roundtrip = original.to_microseconds().to_seconds();
        assert_eq!(original, roundtrip);
    }

    #[test]
    fn test_from_conversions() {
        let s: Seconds = 42u32.into();
        assert_eq!(s.get(), 42);

        let us: Microseconds = 42000u64.into();
        assert_eq!(us.get(), 42000);
    }

    // Arithmetic operator tests for Seconds
    #[test]
    fn test_seconds_add() {
        let a = Seconds::new(10);
        let b = Seconds::new(20);
        assert_eq!(a + b, Seconds::new(30));
    }

    #[test]
    fn test_seconds_sub() {
        let a = Seconds::new(30);
        let b = Seconds::new(10);
        assert_eq!(a - b, Seconds::new(20));
    }

    #[test]
    #[should_panic]
    fn test_seconds_sub_underflow() {
        let a = Seconds::new(10);
        let b = Seconds::new(20);
        let _ = a - b; // Should panic
    }

    #[test]
    fn test_seconds_rem() {
        let a = Seconds::new(10);
        let b = Seconds::new(3);
        assert_eq!(a % b, Seconds::new(1));
    }

    #[test]
    fn test_seconds_saturating_add() {
        let a = Seconds::new(u32::MAX - 5);
        let b = Seconds::new(10);
        assert_eq!(a.saturating_add(b), Seconds::new(u32::MAX));
    }

    #[test]
    fn test_seconds_saturating_sub() {
        let a = Seconds::new(10);
        let b = Seconds::new(20);
        assert_eq!(a.saturating_sub(b), Seconds::new(0));
    }

    #[test]
    fn test_seconds_checked_add() {
        let a = Seconds::new(10);
        let b = Seconds::new(20);
        assert_eq!(a.checked_add(b), Some(Seconds::new(30)));

        let c = Seconds::new(u32::MAX);
        let d = Seconds::new(1);
        assert_eq!(c.checked_add(d), None);
    }

    #[test]
    fn test_seconds_checked_sub() {
        let a = Seconds::new(30);
        let b = Seconds::new(10);
        assert_eq!(a.checked_sub(b), Some(Seconds::new(20)));

        let c = Seconds::new(10);
        let d = Seconds::new(20);
        assert_eq!(c.checked_sub(d), None);
    }

    #[test]
    fn test_seconds_is_multiple_of() {
        let a = Seconds::new(60);
        let b = Seconds::new(15);
        assert!(a.is_multiple_of(b));

        let c = Seconds::new(60);
        let d = Seconds::new(17);
        assert!(!c.is_multiple_of(d));

        let e = Seconds::new(0);
        let f = Seconds::new(10);
        assert!(e.is_multiple_of(f));

        let g = Seconds::new(10);
        let h = Seconds::new(0);
        assert!(!g.is_multiple_of(h)); // Division by zero case
    }

    // Arithmetic operator tests for Microseconds
    #[test]
    fn test_microseconds_add() {
        let a = Microseconds::new(1000);
        let b = Microseconds::new(2000);
        assert_eq!(a + b, Microseconds::new(3000));
    }

    #[test]
    fn test_microseconds_sub() {
        let a = Microseconds::new(3000);
        let b = Microseconds::new(1000);
        assert_eq!(a - b, Microseconds::new(2000));
    }

    #[test]
    #[should_panic]
    fn test_microseconds_sub_underflow() {
        let a = Microseconds::new(1000);
        let b = Microseconds::new(2000);
        let _ = a - b; // Should panic
    }

    #[test]
    fn test_microseconds_rem() {
        let a = Microseconds::new(1000);
        let b = Microseconds::new(300);
        assert_eq!(a % b, Microseconds::new(100));
    }

    #[test]
    fn test_microseconds_saturating_add() {
        let a = Microseconds::new(u64::MAX - 5);
        let b = Microseconds::new(10);
        assert_eq!(a.saturating_add(b), Microseconds::new(u64::MAX));
    }

    #[test]
    fn test_microseconds_saturating_sub() {
        let a = Microseconds::new(1000);
        let b = Microseconds::new(2000);
        assert_eq!(a.saturating_sub(b), Microseconds::new(0));
    }

    #[test]
    fn test_microseconds_checked_add() {
        let a = Microseconds::new(1000);
        let b = Microseconds::new(2000);
        assert_eq!(a.checked_add(b), Some(Microseconds::new(3000)));

        let c = Microseconds::new(u64::MAX);
        let d = Microseconds::new(1);
        assert_eq!(c.checked_add(d), None);
    }

    #[test]
    fn test_microseconds_checked_sub() {
        let a = Microseconds::new(3000);
        let b = Microseconds::new(1000);
        assert_eq!(a.checked_sub(b), Some(Microseconds::new(2000)));

        let c = Microseconds::new(1000);
        let d = Microseconds::new(2000);
        assert_eq!(c.checked_sub(d), None);
    }

    #[test]
    fn test_microseconds_is_multiple_of() {
        let a = Microseconds::new(60000);
        let b = Microseconds::new(15000);
        assert!(a.is_multiple_of(b));

        let c = Microseconds::new(60000);
        let d = Microseconds::new(17000);
        assert!(!c.is_multiple_of(d));

        let e = Microseconds::new(0);
        let f = Microseconds::new(10000);
        assert!(e.is_multiple_of(f));

        let g = Microseconds::new(10000);
        let h = Microseconds::new(0);
        assert!(!g.is_multiple_of(h)); // Division by zero case
    }

    // RealtimeClock tests
    #[test]
    fn test_realtime_clock_monotonic() {
        let clock = RealtimeClock::new();
        let t1 = clock.now();
        let t2 = clock.now();
        let t3 = clock.now();

        assert!(t2 > t1);
        assert!(t3 > t2);
    }

    #[test]
    fn test_realtime_clock_with_initial() {
        let initial = Microseconds::new(1000000);
        let clock = RealtimeClock::with_initial(initial);

        assert_eq!(clock.last_seen(), initial);

        let t1 = clock.now();
        assert!(t1 >= initial);
    }

    #[test]
    fn test_realtime_clock_handles_same_time() {
        // Start with a specific timestamp
        let initial = Microseconds::new(1000000);
        let clock = RealtimeClock::with_initial(initial);

        // Even if system time doesn't advance, clock should increment
        let t1 = clock.now();
        let t2 = clock.now();

        assert!(t2 > t1);
        assert_eq!(t2.get() - t1.get(), 1); // Should increment by 1 microsecond
    }

    #[test]
    fn test_realtime_clock_last_seen() {
        let clock = RealtimeClock::new();

        let t1 = clock.now();
        assert_eq!(clock.last_seen(), t1);

        let t2 = clock.now();
        assert_eq!(clock.last_seen(), t2);
    }

    #[test]
    fn test_realtime_clock_forward_jump() {
        // Start with a timestamp in the past
        let past = Microseconds::new(1000000);
        let clock = RealtimeClock::with_initial(past);

        // When system time is ahead, it should use system time
        let t1 = clock.now();
        assert!(t1.get() > past.get());
    }
}
