//! A fixed-size log-linear histogram of durations: 16 linear sub-bins per
//! power of two, so a bin's midpoint is within 1/32 of every value in it.
//! Memory is per histogram, not per recorded duration. Used for the
//! overview's per-bucket trace-duration percentiles, and for the
//! explorer's span-duration percentiles and heatmap rows.

/// One time bucket's duration percentiles, nanoseconds.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct DurationPercentiles {
    pub p50: i64,
    pub p95: i64,
    pub p99: i64,
}

/// Linear sub-bins per power of two: a bin spans 1/16 of its lower
/// edge, so its midpoint is within 1/32 of every value in it.
const SUB_BINS: usize = 16;
/// Durations 0..16 get a bin each; each power of two from 2^4 to 2^62
/// then gets [`SUB_BINS`] (durations are non-negative `i64`).
const BINS: usize = SUB_BINS * (1 + 62 - 4 + 1);

/// Rows of the explorer's duration heatmap: row 0 holds everything under
/// 2^10 ns (about 1 µs), row `k` then `[2^(9+k), 2^(10+k))` ns, and the
/// last row everything from 2^40 ns (about 18 minutes). Every edge is a
/// power of two of at least 2^4, so no bin straddles one.
pub const HEATMAP_ROWS: usize = 32;
const HEATMAP_FIRST_BIT: u32 = 10;

/// Each heatmap row's exclusive upper bound in nanoseconds; `None` for the
/// last, unbounded row.
pub fn heatmap_row_bounds() -> [Option<i64>; HEATMAP_ROWS] {
    std::array::from_fn(|row| {
        (row + 1 < HEATMAP_ROWS).then(|| 1i64 << (HEATMAP_FIRST_BIT + row as u32))
    })
}

/// Durations counted per log-linear bin, plus the observed extremes.
#[derive(Debug, Clone)]
pub(crate) struct DurationHistogram {
    counts: Vec<u32>,
    n: u64,
    /// The smallest and largest recorded durations; `None` until one is.
    range: Option<(i64, i64)>,
}

impl DurationHistogram {
    pub(crate) fn new() -> Self {
        Self {
            counts: vec![0; BINS],
            n: 0,
            range: None,
        }
    }

    /// The bin of a duration (negatives clamp to 0).
    fn bin(duration_ns: i64) -> usize {
        let v = duration_ns.max(0) as u64;
        if v < SUB_BINS as u64 {
            return v as usize;
        }
        let log2 = 63 - v.leading_zeros() as usize;
        let sub = (v >> (log2 - 4)) as usize & (SUB_BINS - 1);
        SUB_BINS + (log2 - 4) * SUB_BINS + sub
    }

    /// The inclusive value range of a bin.
    fn bounds(bin: usize) -> (i64, i64) {
        if bin < SUB_BINS {
            return (bin as i64, bin as i64);
        }
        let log2 = (bin - SUB_BINS) / SUB_BINS + 4;
        let sub = ((bin - SUB_BINS) % SUB_BINS) as i64;
        let width = 1i64 << (log2 - 4);
        let lo = (1i64 << log2) + sub * width;
        (lo, lo + (width - 1))
    }

    pub(crate) fn record(&mut self, duration_ns: i64) {
        let bin = &mut self.counts[Self::bin(duration_ns)];
        *bin = bin.saturating_add(1);
        self.n += 1;
        self.range = Some(match self.range {
            None => (duration_ns, duration_ns),
            Some((min, max)) => (min.min(duration_ns), max.max(duration_ns)),
        });
    }

    /// Add `other`'s durations to these.
    pub(crate) fn merge(&mut self, other: &DurationHistogram) {
        for (bin, &count) in self.counts.iter_mut().zip(&other.counts) {
            *bin = bin.saturating_add(count);
        }
        self.n += other.n;
        self.range = match (self.range, other.range) {
            (None, range) | (range, None) => range,
            (Some((a, b)), Some((c, d))) => Some((a.min(c), b.max(d))),
        };
    }

    /// The recorded durations per heatmap row ([`HEATMAP_ROWS`]).
    pub(crate) fn heatmap_rows(&self) -> [u64; HEATMAP_ROWS] {
        let mut rows = [0u64; HEATMAP_ROWS];
        for (bin, &count) in self.counts.iter().enumerate() {
            if count == 0 {
                continue;
            }
            let (lo, _) = Self::bounds(bin);
            let row = if lo < 1 << HEATMAP_FIRST_BIT {
                0
            } else {
                let bit = 63 - (lo as u64).leading_zeros();
                ((bit - HEATMAP_FIRST_BIT + 1) as usize).min(HEATMAP_ROWS - 1)
            };
            rows[row] += u64::from(count);
        }
        rows
    }

    /// Nearest-rank percentile `p`: rank `ceil(p/100 x n)`, 1-based. The
    /// rank's bin holds the exact answer, so its midpoint clamped to the
    /// observed range is within 1/32 of it; equal durations answer that
    /// duration. `None` when nothing was recorded.
    pub(crate) fn percentile(&self, p: u64) -> Option<i64> {
        let (min, max) = self.range?;
        let rank = (p * self.n).div_ceil(100).max(1);
        let mut seen = 0u64;
        for (bin, &count) in self.counts.iter().enumerate() {
            seen += u64::from(count);
            if seen >= rank {
                let (lo, hi) = Self::bounds(bin);
                return Some((lo + (hi - lo) / 2).clamp(min, max));
            }
        }
        Some(max)
    }

    /// p50, p95 and p99; `None` when nothing was recorded.
    pub(crate) fn percentiles(&self) -> Option<DurationPercentiles> {
        Some(DurationPercentiles {
            p50: self.percentile(50)?,
            p95: self.percentile(95)?,
            p99: self.percentile(99)?,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Every duration lands in a bin whose bounds hold it, the bins
    /// tile the range in order, and the extremes stay in range.
    #[test]
    fn histogram_bins_cover_every_duration() {
        assert_eq!(BINS, 960);
        for d in [0, 1, 15, 16, 17, 31, 32, 1_000_000, 1 << 62, i64::MAX] {
            let bin = DurationHistogram::bin(d);
            assert!(bin < BINS, "{d}");
            let (lo, hi) = DurationHistogram::bounds(bin);
            assert!(lo <= d && d <= hi, "{d} in [{lo}, {hi}]");
        }
        assert_eq!(DurationHistogram::bin(-5), 0, "negatives clamp to 0");
        assert_eq!(DurationHistogram::bin(i64::MAX), BINS - 1);
        for bin in 1..BINS {
            let (_, prev_hi) = DurationHistogram::bounds(bin - 1);
            let (lo, hi) = DurationHistogram::bounds(bin);
            assert_eq!(lo, prev_hi + 1, "bin {bin} follows bin {}", bin - 1);
            assert!(hi - lo <= lo / SUB_BINS as i64, "bin {bin} width");
        }
    }

    /// Against exact nearest rank over seeded populations (narrow,
    /// decade-wide and full-range) and edge values: within 1/32, inside
    /// the observed range, and exact for a single duration.
    #[test]
    fn percentiles_stay_within_the_histogram_bound() {
        let mut state = 0x9E37_79B9_7F4A_7C15u64;
        let mut next = move |bound: u64| {
            state = state
                .wrapping_mul(6364136223846793005)
                .wrapping_add(1442695040888963407);
            (state >> 11) % bound
        };
        let mut populations: Vec<Vec<i64>> =
            vec![vec![0, 15, 16, i64::MAX], vec![7; 5], vec![i64::MAX]];
        for (n, span) in [(10, 1_000), (1_000, 10_000_000), (5_000, 1 << 40)] {
            populations.push((0..n).map(|_| next(span) as i64).collect());
        }
        populations.push(
            (0..2_000)
                .map(|_| (next(u64::MAX >> 1) >> next(60)) as i64)
                .collect(),
        );
        for durations in populations {
            let mut h = DurationHistogram::new();
            for &d in &durations {
                h.record(d);
            }
            let mut sorted = durations.clone();
            sorted.sort_unstable();
            for p in [50, 95, 99] {
                let exact = sorted[(p as usize * sorted.len()).div_ceil(100) - 1];
                let got = h.percentile(p).unwrap();
                assert!(
                    (got - exact).abs() <= exact / 32,
                    "p{p}: {got} vs exact {exact} (n {})",
                    sorted.len()
                );
                assert!(sorted[0] <= got && got <= sorted[sorted.len() - 1]);
            }
        }
    }

    /// Each row holds exactly the durations between its bounds.
    #[test]
    fn heatmap_rows_follow_their_bounds() {
        let bounds = heatmap_row_bounds();
        assert_eq!(bounds[0], Some(1 << 10));
        assert_eq!(bounds[HEATMAP_ROWS - 2], Some(1 << 40));
        assert_eq!(bounds[HEATMAP_ROWS - 1], None);
        let row_of = |d: i64| bounds.iter().position(|b| b.is_none_or(|b| d < b)).unwrap();
        let durations = [
            0,
            1,
            1023,
            1024,
            2047,
            2048,
            1_000_000,
            (1 << 40) - 1,
            1 << 40,
            i64::MAX,
        ];
        let mut h = DurationHistogram::new();
        let mut expected = [0u64; HEATMAP_ROWS];
        for d in durations {
            h.record(d);
            expected[row_of(d)] += 1;
        }
        assert_eq!(h.heatmap_rows(), expected);
    }

    /// Merging equals recording both populations into one histogram.
    #[test]
    fn merge_equals_recording_everything_once() {
        let (a, b) = ([5, 900, 70_000], [3, 1 << 33, 12]);
        let (mut left, mut right, mut both) = (
            DurationHistogram::new(),
            DurationHistogram::new(),
            DurationHistogram::new(),
        );
        a.iter().for_each(|&d| left.record(d));
        b.iter().for_each(|&d| right.record(d));
        a.iter().chain(&b).for_each(|&d| both.record(d));
        left.merge(&right);
        assert_eq!(left.heatmap_rows(), both.heatmap_rows());
        assert_eq!(left.percentiles(), both.percentiles());
        let mut empty = DurationHistogram::new();
        empty.merge(&DurationHistogram::new());
        assert_eq!(empty.percentiles(), None);
    }

    #[test]
    fn an_empty_histogram_has_no_percentiles() {
        let h = DurationHistogram::new();
        assert_eq!(h.percentile(50), None);
        assert_eq!(h.percentiles(), None);
    }
}
