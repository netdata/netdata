// SPDX-License-Identifier: GPL-3.0-or-later
//
// `absent` and `absent_over_time`.
//
// Both functions return a 1-valued series at every grid point where the
// inner expression yielded no data, with labels carried from the
// static `=` matchers on the inner selector (extracted at lowering time
// and stored in the `Plan::Absent` variant). At grid points where the
// inner produced any series with a non-NaN value, the output is NaN.
// If every grid point has data present, the output vector is empty.
//
// The two variants are folded into one evaluator path; the
// instant-vs-range distinction lives in the inner Plan's return type
// and the evaluator only needs to look at "is data present at this grid
// point?".

use super::context::EvalContext;
use super::types::{EvalError, EvalResult, Series};

pub fn eval_absent(
    ctx: &EvalContext,
    labels: &[(String, String)],
    inner: EvalResult,
) -> Result<EvalResult, EvalError> {
    let grid = &ctx.grid;
    let n = grid.len();

    // For each grid position, decide "is data present here?".
    let mut present = vec![false; n];

    match &inner {
        EvalResult::Scalar(v) => {
            // A scalar is always-present unless it's literally NaN.
            // (No grid position discrimination.)
            if !v.is_nan() {
                return Ok(EvalResult::InstantVector(Vec::new()));
            }
            // A NaN scalar = absent at every grid point: fall through
            // with `present` all false.
        }
        EvalResult::InstantVector(series) => {
            for s in series {
                for i in 0..s.values.len().min(n) {
                    if !s.values[i].is_nan() {
                        present[i] = true;
                    }
                }
            }
        }
        EvalResult::RangeVector(series) => {
            // For absent_over_time: the inner range vector spans the
            // matrix window for each grid point. Grid-aware selectors
            // emit samples across the whole [grid.start - range, grid.end]
            // window without per-grid-point segmentation, so to decide
            // presence at grid point `t` we'd need range_ms here. The
            // dispatch layer doesn't currently pass range_ms to absent;
            // approximate: if any series has any non-NaN sample at all,
            // mark every grid point as present. Refining this needs the
            // same range_ms threading that the rollup family uses --
            // tracked as a follow-up.
            let any_present = series.iter().any(|s| s.values.iter().any(|v| !v.is_nan()));
            if any_present {
                return Ok(EvalResult::InstantVector(Vec::new()));
            }
        }
    }

    // Build the output values. A grid point is "absent" when present[i]
    // is false; emit 1 there. Present points emit NaN.
    let mut absent_count = 0usize;
    let values: Vec<f64> = (0..n)
        .map(|i| {
            if present[i] {
                f64::NAN
            } else {
                absent_count += 1;
                1.0
            }
        })
        .collect();

    if absent_count == 0 {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }

    Ok(EvalResult::InstantVector(vec![Series::new(
        labels.to_vec(),
        std::sync::Arc::clone(&grid.timestamps),
        values,
    )]))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::eval::types::Series;

    fn ctx(t: i64) -> EvalContext {
        EvalContext {
            grid: std::sync::Arc::new(crate::eval::Grid::instant(t)),
            outer_start_ms: t,
            outer_end_ms: t,
            ..EvalContext::default()
        }
    }

    #[test]
    fn empty_inner_emits_one_synthetic_series() {
        let labels = vec![
            ("__name__".to_string(), "metric".to_string()),
            ("job".to_string(), "api".to_string()),
        ];
        let result =
            eval_absent(&ctx(1000), &labels, EvalResult::InstantVector(Vec::new())).unwrap();
        match result {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert_eq!(v[0].values[0], 1.0);
                assert_eq!(v[0].timestamps[0], 1000);
                assert_eq!(v[0].labels.len(), 2);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn non_empty_inner_emits_nothing() {
        let inner = EvalResult::InstantVector(vec![Series::scalar(vec![], 0, 1.0)]);
        let result = eval_absent(&ctx(0), &[], inner).unwrap();
        match result {
            EvalResult::InstantVector(v) => assert!(v.is_empty()),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn empty_range_emits_one_series() {
        let result = eval_absent(&ctx(2000), &[], EvalResult::RangeVector(Vec::new())).unwrap();
        match result {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert_eq!(v[0].timestamps[0], 2000);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }
}
