//! Type-safe time units for the journal stack: `Seconds`/`Microseconds`
//! newtypes, the strictly monotonic `RealtimeClock`, and `monotonic_now`;
//! imported via the flat crate-root re-export ([`crate::Seconds`],
//! [`crate::Microseconds`], [`crate::RealtimeClock`],
//! [`crate::monotonic_now`]), not this path.

use serde::{Deserialize, Serialize};
use std::cell::Cell;
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

/// A strictly monotonic stream of wall-clock microseconds.
///
/// Wraps `SystemTime`-based wall-clock time but never returns a value at or
/// below its floor (the highest value returned or seeded): when the system
/// clock stalls or steps backwards (NTP correction), the next value is
/// `floor + 1µs` instead. `now()` and `observe()` share one floor, so
/// interleaving them keeps the combined stream strictly increasing — until
/// saturation at `u64::MAX`, where the +1µs bump can no longer move it.
///
/// The floor is a `Cell`, so both methods take only `&self`; that makes the
/// clock `Send` but `!Sync` — not shareable across threads.
#[derive(Debug)]
pub struct RealtimeClock {
    max_seen: Cell<u64>,
}

impl RealtimeClock {
    /// Creates a clock whose floor is the current system time.
    ///
    /// Panics if the system clock is before the Unix epoch (via
    /// `Microseconds::now()`).
    pub fn new() -> Self {
        Self::with_initial(Microseconds::now())
    }

    /// Creates a clock seeded with a known timestamp.
    ///
    /// Values at or below `initial` are bumped to `initial + 1µs` on the next
    /// read. journal-log-writer seeds this from the persisted chain's tail
    /// realtime timestamp on open (`journal-log-writer/src/log/mod.rs`
    /// `Log::new`).
    pub fn with_initial(initial: Microseconds) -> Self {
        Self {
            max_seen: Cell::new(initial.get()),
        }
    }

    /// Current wall-clock time as microseconds since the Unix epoch, kept
    /// strictly increasing.
    ///
    /// Returns the system time when it is ahead of the floor; otherwise
    /// `floor + 1µs` (saturating). Shares the floor with `observe()`.
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

    /// Admits an externally supplied timestamp, keeping the stream monotonic.
    ///
    /// Returns `candidate` when it is ahead of the floor; otherwise
    /// `floor + 1µs` (saturating). Shares the floor with `now()`.
    /// journal-log-writer routes caller-supplied entry realtime overrides
    /// through this (`journal-log-writer/src/log/mod.rs`
    /// `Log::capture_dual_timestamp`).
    pub fn observe(&self, candidate: Microseconds) -> Microseconds {
        let max = self.max_seen.get();
        let next = if candidate.get() > max {
            candidate.get()
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

/// Reads `CLOCK_MONOTONIC` as microseconds since boot.
///
/// Not tied to the Unix epoch and unaffected by system-clock jumps, but
/// it does not advance while the system is suspended. Matches the monotonic
/// half of systemd's journal dual timestamps. Consecutive calls can repeat
/// within one microsecond, so per-entry distinctness needs clamping —
/// journal-log-writer clamps against the last written monotonic value for the
/// boot (`journal-log-writer/src/log/mod.rs`
/// `Log::capture_dual_timestamp`).
///
/// The error carries the raw `clock_gettime` errno as `io::Error`;
/// effectively unreachable for `CLOCK_MONOTONIC` on Linux.
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
        // two now() calls within one wall-clock microsecond still differ by 1µs
        let initial = Microseconds::new(1000000);
        let clock = RealtimeClock::with_initial(initial);

        let t1 = clock.now();
        let t2 = clock.now();

        assert!(t2 > t1);
        assert_eq!(t2.get() - t1.get(), 1);
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
        let past = Microseconds::new(1000000);
        let clock = RealtimeClock::with_initial(past);

        // the real clock is ahead of the floor, so it wins
        let t1 = clock.now();
        assert!(t1.get() > past.get());
    }

    #[test]
    fn test_realtime_clock_observe_preserves_monotonicity() {
        let clock = RealtimeClock::with_initial(Microseconds::new(1_000_000));

        let t1 = clock.observe(Microseconds::new(900_000));
        assert_eq!(t1.get(), 1_000_001);

        let t2 = clock.observe(Microseconds::new(1_000_001));
        assert_eq!(t2.get(), 1_000_002);

        let t3 = clock.observe(Microseconds::new(1_500_000));
        assert_eq!(t3.get(), 1_500_000);
    }
}
