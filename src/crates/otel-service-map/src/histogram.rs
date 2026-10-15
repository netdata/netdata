//! Sparse log-linear latency histogram.
//!
//! Values below 128 are exact. Above that, each power of two is split into
//! 64 equal buckets, so a bucket's width is at most 1/64 of its lower bound
//! and its midpoint is within ~0.8% of any value it holds. Only non-empty
//! buckets are stored: an edge's per-minute histogram stays a few dozen
//! entries, and histograms merge by adding counts, so percentiles over any
//! window are computed after merging minutes.

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

const SUB_BITS: u32 = 6;
const EXACT_LIMIT: u64 = 2 << SUB_BITS;

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Histogram {
    buckets: BTreeMap<u16, u64>,
}

impl Histogram {
    pub fn record(&mut self, value: u64) {
        *self.buckets.entry(bucket_index(value)).or_insert(0) += 1;
    }

    pub fn merge(&mut self, other: &Histogram) {
        for (&index, &count) in &other.buckets {
            *self.buckets.entry(index).or_insert(0) += count;
        }
    }

    pub fn count(&self) -> u64 {
        self.buckets.values().sum()
    }

    pub fn is_empty(&self) -> bool {
        self.buckets.is_empty()
    }

    /// The value at quantile `q` (0..=1): the midpoint of the bucket holding
    /// the `ceil(q * count)`-th smallest value. `None` when empty.
    pub fn quantile(&self, q: f64) -> Option<u64> {
        let total = self.count();
        if total == 0 {
            return None;
        }
        let rank = ((q.clamp(0.0, 1.0) * total as f64).ceil() as u64).max(1);
        let mut seen = 0u64;
        for (&index, &count) in &self.buckets {
            seen += count;
            if seen >= rank {
                return Some(bucket_midpoint(index));
            }
        }
        self.buckets.keys().next_back().map(|&i| bucket_midpoint(i))
    }
}

fn bucket_index(value: u64) -> u16 {
    if value < EXACT_LIMIT {
        return value as u16;
    }
    let msb = 63 - value.leading_zeros();
    let shift = msb - SUB_BITS;
    let mantissa = value >> shift;
    // `mantissa` is in [64, 128); `(shift << 6) + mantissa` continues the
    // exact range without gaps: shift 1 starts at index 128.
    ((shift as u16) << SUB_BITS) + mantissa as u16
}

fn bucket_bounds(index: u16) -> (u64, u64) {
    let index = u64::from(index);
    if index < EXACT_LIMIT {
        return (index, index + 1);
    }
    let shift = (index >> SUB_BITS) - 1;
    let mantissa = (index & ((1 << SUB_BITS) - 1)) + (1 << SUB_BITS);
    // The top bucket's upper bound is 2^64; saturate instead of wrapping.
    let high = (u128::from(mantissa + 1) << shift).min(u128::from(u64::MAX)) as u64;
    (mantissa << shift, high)
}

fn bucket_midpoint(index: u16) -> u64 {
    let (low, high) = bucket_bounds(index);
    if high - low == 1 {
        return low;
    }
    low + (high - low) / 2
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn index_and_bounds_agree() {
        let mut previous = None;
        for value in [
            0,
            1,
            127,
            128,
            129,
            255,
            256,
            1_000,
            65_535,
            1 << 40,
            u64::MAX,
        ] {
            let index = bucket_index(value);
            let (low, high) = bucket_bounds(index);
            assert!(
                low <= value && (value < high || high == u64::MAX),
                "value {value} outside [{low}, {high})"
            );
            if let Some(p) = previous {
                assert!(index >= p, "indices must not decrease");
            }
            previous = Some(index);
        }
    }

    #[test]
    fn indices_are_contiguous_across_the_exact_limit() {
        assert_eq!(bucket_index(127), 127);
        assert_eq!(bucket_index(128), 128);
        assert_eq!(bucket_bounds(128), (128, 130));
    }

    #[test]
    fn quantiles_stay_within_the_error_bound() {
        let mut h = Histogram::default();
        for v in 1..=10_000u64 {
            h.record(v * 37);
        }
        for (q, exact) in [(0.5, 5_000u64 * 37), (0.95, 9_500 * 37), (0.99, 9_900 * 37)] {
            let got = h.quantile(q).unwrap() as f64;
            let error = (got - exact as f64).abs() / exact as f64;
            assert!(error < 0.01, "q={q}: got {got}, exact {exact}");
        }
    }

    #[test]
    fn small_values_are_exact() {
        let mut h = Histogram::default();
        for v in [3, 3, 7, 100] {
            h.record(v);
        }
        assert_eq!(h.quantile(0.0), Some(3));
        assert_eq!(h.quantile(0.5), Some(3));
        assert_eq!(h.quantile(0.75), Some(7));
        assert_eq!(h.quantile(1.0), Some(100));
    }

    #[test]
    fn merge_equals_recording_everything_in_one() {
        let (mut a, mut b, mut all) = (
            Histogram::default(),
            Histogram::default(),
            Histogram::default(),
        );
        for v in 0..500u64 {
            let value = v * v;
            if v % 2 == 0 {
                a.record(value)
            } else {
                b.record(value)
            }
            all.record(value);
        }
        a.merge(&b);
        assert_eq!(a, all);
    }

    #[test]
    fn empty_has_no_quantile() {
        assert_eq!(Histogram::default().quantile(0.5), None);
    }
}
