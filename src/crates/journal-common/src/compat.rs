//! MSRV compatibility helpers.
//!
//! This module provides backports of newer Rust standard library features
//! to maintain compatibility with our current MSRV (Minimum Supported Rust Version).

/// Returns `true` if `value` is an integer multiple of `divisor`, and `false` otherwise.
///
/// This is a compatibility function for MSRV < 1.87.0 that provides the same functionality
/// as the built-in `u32::is_multiple_of()` method.
///
/// # Migration path
///
/// When MSRV >= 1.87.0, replace calls to this function with the standard library's
/// `is_multiple_of()` method:
/// ```ignore
/// // Old (MSRV < 1.87.0):
/// use journal_common::compat::is_multiple_of;
/// is_multiple_of(value, divisor)
///
/// // New (MSRV >= 1.87.0):
/// value.is_multiple_of(divisor)
/// ```
///
/// # Examples
///
/// ```
/// use journal_common::compat::is_multiple_of;
///
/// assert!(is_multiple_of(100u32, 10));
/// assert!(is_multiple_of(100u32, 20));
/// assert!(!is_multiple_of(100u32, 7));
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
