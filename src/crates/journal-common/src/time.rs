//! Type-safe time units for the journal stack: the `Seconds`/`Microseconds`
//! newtypes, imported via the flat crate-root re-export ([`crate::Seconds`],
//! [`crate::Microseconds`]), not this path.

use serde::{Deserialize, Serialize};
use std::ops::{Add, Rem, Sub};

/// A count of seconds (`u32`): epoch seconds for timestamps (registry time
/// ranges, index `indexed_at`), plain seconds for durations (histogram
/// bucket widths). The tuple field is public; the stack reads `.0` directly.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Seconds(pub u32);

/// A count of microseconds (`u64`): epoch microseconds for journal entry
/// timestamps and histogram timestamp pairs. The tuple field is public;
/// `u64` spans ~584k years, so there is no practical wrap.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Microseconds(pub u64);

impl Seconds {
    /// Wraps a raw seconds count.
    pub fn new(seconds: u32) -> Self {
        Self(seconds)
    }

    /// Current wall-clock time as epoch seconds.
    ///
    /// Panics if the system clock is before the Unix epoch; past `u32`
    /// capacity (year 2106) the `u64`-to-`u32` cast keeps only the low bits.
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

    /// Converts to microseconds (exact).
    pub fn to_microseconds(self) -> Microseconds {
        Microseconds(self.0 as u64 * 1_000_000)
    }

    /// Adds `other`, saturating at `u32::MAX`.
    pub fn saturating_add(self, other: Self) -> Self {
        Seconds(self.0.saturating_add(other.0))
    }

    /// Subtracts `other`, saturating at 0.
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

    /// Returns `true` if `self` is an integer multiple of `other`.
    ///
    /// A zero divisor yields `false` rather than panicking (unlike
    /// `journal_common::compat::is_multiple_of`, which panics); zero is a
    /// multiple of every non-zero divisor. Used for bucket-duration
    /// compatibility in `journal-engine/src/indexing.rs`
    /// (`batch_compute_file_indexes`); journal-index instead checks histogram
    /// alignment through the compat function on raw integers
    /// (`journal-index/src/histogram.rs`
    /// `Histogram::count_entries_in_time_range`).
    pub fn is_multiple_of(self, other: Self) -> bool {
        other.0 != 0 && self.0 % other.0 == 0
    }
}

impl Microseconds {
    /// Wraps a raw microsecond count.
    pub fn new(microseconds: u64) -> Self {
        Self(microseconds)
    }

    /// Current wall-clock time as epoch microseconds.
    ///
    /// Panics if the system clock is before the Unix epoch.
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

    /// Convert to seconds (truncates). Past `u32` capacity (year 2106) the
    /// `u64`-to-`u32` cast keeps only the low bits.
    pub fn to_seconds(self) -> Seconds {
        Seconds((self.0 / 1_000_000) as u32)
    }

    /// Adds `other`, saturating at `u64::MAX`.
    pub fn saturating_add(self, other: Self) -> Self {
        Microseconds(self.0.saturating_add(other.0))
    }

    /// Subtracts `other`, saturating at 0.
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

    /// Returns `true` if `self` is an integer multiple of `other`.
    ///
    /// Same semantics as [`Seconds::is_multiple_of`]: a zero divisor yields
    /// `false`; zero is a multiple of every non-zero divisor.
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

// The `Add`/`Sub`/`Rem` impls below (both types) are plain integer
// arithmetic: `Add`/`Sub` panic on overflow/underflow when debug assertions
// are on (dev/test builds) and wrap silently in release; `%` by zero panics
// in every profile. Use the saturating_*/checked_* methods above when that is
// unacceptable.
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
        let _ = a - b;
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
        assert!(!g.is_multiple_of(h)); // zero divisor returns false, no panic
    }

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
        let _ = a - b;
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
        assert!(!g.is_multiple_of(h)); // zero divisor returns false, no panic
    }
}
