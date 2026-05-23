// SPDX-License-Identifier: GPL-3.0-or-later
//
// Whole-range evaluation grid.
//
// The evaluator precomputes the list of evaluation timestamps once and
// carries it on the EvalContext. Each operator evaluates over the entire
// grid in a single pass: selectors emit one sample per series per grid
// point, rollups two-pointer-sweep over bulk-fetched raw samples, and
// all per-position operators (binops, aggregations, transforms, label
// ops) work element-wise across the grid.
//
// An instant query builds a single-point grid; the same code path serves
// both shapes.

use std::sync::Arc;

#[derive(Debug, Clone)]
pub struct Grid {
    /// Start of the evaluation range, in Unix milliseconds. For an
    /// instant query, equals `end_ms` and `timestamps[0]`.
    pub start_ms: i64,
    /// End of the evaluation range, in Unix milliseconds. Inclusive.
    pub end_ms: i64,
    /// Step between grid points, in milliseconds. Zero for an instant
    /// query.
    pub step_ms: i64,
    /// Precomputed timestamps. For range queries this is `(end_ms -
    /// start_ms) / step_ms + 1` entries; for instant queries it is one.
    /// `Arc` so sub-eval contexts (subqueries) can construct nested
    /// grids without cloning the outer one.
    pub timestamps: Arc<Vec<i64>>,
}

impl Grid {
    /// Single-point grid at `at_ms`. Used for instant queries and for
    /// pinned `@` modifier evaluation.
    pub fn instant(at_ms: i64) -> Self {
        Self {
            start_ms: at_ms,
            end_ms: at_ms,
            step_ms: 0,
            timestamps: Arc::new(vec![at_ms]),
        }
    }

    /// Range grid: `[start_ms, end_ms]` at `step_ms` stride. Both ends
    /// inclusive. `step_ms` must be positive; `end_ms >= start_ms`.
    pub fn range(start_ms: i64, end_ms: i64, step_ms: i64) -> Self {
        debug_assert!(step_ms > 0, "step_ms must be positive");
        debug_assert!(end_ms >= start_ms, "end_ms must be >= start_ms");
        let span = end_ms.saturating_sub(start_ms);
        let n = (span / step_ms) as usize + 1;
        let mut ts = Vec::with_capacity(n);
        let mut t = start_ms;
        loop {
            ts.push(t);
            let next = match t.checked_add(step_ms) {
                Some(n) => n,
                None => break,
            };
            if next > end_ms {
                break;
            }
            t = next;
        }
        Self {
            start_ms,
            end_ms,
            step_ms,
            timestamps: Arc::new(ts),
        }
    }

    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    #[inline]
    pub fn is_empty(&self) -> bool {
        self.timestamps.is_empty()
    }

    /// True for a single-point grid (instant query or @-pinned eval).
    #[inline]
    pub fn is_instant(&self) -> bool {
        self.timestamps.len() == 1
    }

    /// Convenience: the single grid timestamp when `is_instant()`.
    /// Returns the first timestamp for any other grid (callers usually
    /// want `start_ms` in that case).
    #[inline]
    pub fn first_ms(&self) -> i64 {
        self.timestamps[0]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn instant_grid_has_one_timestamp() {
        let g = Grid::instant(12_345);
        assert_eq!(g.len(), 1);
        assert!(g.is_instant());
        assert_eq!(g.timestamps[0], 12_345);
        assert_eq!(g.start_ms, 12_345);
        assert_eq!(g.end_ms, 12_345);
        assert_eq!(g.step_ms, 0);
    }

    #[test]
    fn range_grid_endpoints_inclusive() {
        let g = Grid::range(0, 100, 25);
        assert_eq!(g.len(), 5);
        assert_eq!(*g.timestamps, vec![0, 25, 50, 75, 100]);
    }

    #[test]
    fn range_grid_uneven_end_truncates() {
        // 0, 30, 60, 90 — 120 would overshoot 110.
        let g = Grid::range(0, 110, 30);
        assert_eq!(*g.timestamps, vec![0, 30, 60, 90]);
    }

    #[test]
    fn range_grid_single_step() {
        // start == end with positive step produces one point.
        let g = Grid::range(50, 50, 10);
        assert_eq!(g.len(), 1);
        assert_eq!(g.timestamps[0], 50);
    }

    #[test]
    fn one_hour_at_15s_has_241_points() {
        // The canonical "Grafana 1h @ 15s" range: 3600s/15s = 240 steps
        // + 1 endpoint = 241.
        let g = Grid::range(0, 3_600_000, 15_000);
        assert_eq!(g.len(), 241);
    }
}
