//! MSRV compatibility helpers.
//!
//! The workspace MSRV is 1.91 (src/crates/Cargo.toml), above the 1.87.0 that
//! stabilized std's `is_multiple_of`; this shim remains only because its two
//! consumers (journal-core mmap.rs, journal-index histogram.rs) still use it.

/// Returns `true` if `value` is an integer multiple of `divisor`.
///
/// Equivalent to `value % divisor == 0`. std's inherent `is_multiple_of` is
/// defined only on unsigned integer primitives, so this shim is generic over
/// the bounds below (call sites use `u32` and `u64`).
///
/// # Panics
///
/// Panics if `divisor` is zero, like the `%` operator. std's method instead
/// defines `x.is_multiple_of(0)` as `x == 0`, so a blind call-site migration
/// changes behavior for a zero divisor.
///
/// # Migration path
///
/// Replace calls with the std method (`is_multiple_of(a, b)` becomes
/// `a.is_multiple_of(b)`) and delete this module.
///
/// # Examples
///
/// ```
/// use journal_common::compat::is_multiple_of;
///
/// assert!(is_multiple_of(100u32, 10));
/// assert!(is_multiple_of(100u32, 20));
/// assert!(!is_multiple_of(100u32, 7));
/// assert!(is_multiple_of(1000u64, 10));
/// ```
#[inline]
pub fn is_multiple_of<T>(value: T, divisor: T) -> bool
where
    T: std::ops::Rem<Output = T> + PartialEq + From<u8>,
{
    value % divisor == T::from(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_multiple_of_u32() {
        assert!(is_multiple_of(100u32, 10));
        assert!(is_multiple_of(100u32, 20));
        assert!(is_multiple_of(100u32, 100));
        assert!(is_multiple_of(0u32, 10));

        assert!(!is_multiple_of(100u32, 7));
        assert!(!is_multiple_of(100u32, 30));
    }

    #[test]
    fn test_is_multiple_of_u64() {
        assert!(is_multiple_of(1000u64, 10));
        assert!(is_multiple_of(1000u64, 100));
        assert!(is_multiple_of(0u64, 100));

        assert!(!is_multiple_of(1000u64, 7));
    }
}
