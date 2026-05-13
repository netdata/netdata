// SPDX-License-Identifier: GPL-3.0-or-later
//
// Unary negation.

use super::types::{EvalResult, Series};

pub fn negate(r: EvalResult) -> EvalResult {
    match r {
        EvalResult::Scalar(v) => EvalResult::Scalar(-v),
        EvalResult::InstantVector(series) => {
            EvalResult::InstantVector(series.into_iter().map(negate_series).collect())
        }
        EvalResult::RangeVector(series) => {
            EvalResult::RangeVector(series.into_iter().map(negate_series).collect())
        }
    }
}

fn negate_series(mut s: Series) -> Series {
    for sample in &mut s.samples {
        sample.value = -sample.value;
    }
    s
}
