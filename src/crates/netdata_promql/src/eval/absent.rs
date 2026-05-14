// SPDX-License-Identifier: GPL-3.0-or-later
//
// `absent` and `absent_over_time`. SOW-0027 Group G.
//
// Both functions return a single 1-valued series when their inner
// expression yielded no data, with labels carried from the static `=`
// matchers on the inner selector (extracted at lowering time and
// stored in the Plan::Absent variant). When the inner expression
// produced any series, both functions return the empty vector.
//
// The two variants are folded into one evaluator path; the
// instant-vs-range distinction lives in the inner Plan's return
// type and the evaluator only needs to look at "did anything come
// back".

use super::context::EvalContext;
use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

pub fn eval_absent(
    ctx: &EvalContext,
    labels: &[(String, String)],
    inner: EvalResult,
) -> Result<EvalResult, EvalError> {
    let any = match &inner {
        EvalResult::InstantVector(s) => !s.is_empty(),
        EvalResult::RangeVector(s) => !s.is_empty(),
        EvalResult::Scalar(_) => true,
    };
    if any {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let labels: Vec<(String, String)> = labels.to_vec();
    let signature = labels_signature(&labels);
    Ok(EvalResult::InstantVector(vec![Series {
        labels,
        signature,
        samples: vec![Sample {
            timestamp_ms: ctx.at_ms(),
            value: 1.0,
        }],
    }]))
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
        let result = eval_absent(
            &ctx(1000),
            &labels,
            EvalResult::InstantVector(Vec::new()),
        )
        .unwrap();
        match result {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert_eq!(v[0].samples[0].value, 1.0);
                assert_eq!(v[0].samples[0].timestamp_ms, 1000);
                assert_eq!(v[0].labels.len(), 2);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn non_empty_inner_emits_nothing() {
        let inner = EvalResult::InstantVector(vec![Series {
            labels: vec![],
            signature: 0,
            samples: vec![Sample {
                timestamp_ms: 0,
                value: 1.0,
            }],
        }]);
        let result = eval_absent(&ctx(0), &[], inner).unwrap();
        match result {
            EvalResult::InstantVector(v) => assert!(v.is_empty()),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn empty_range_emits_one_series() {
        let result = eval_absent(
            &ctx(2000),
            &[],
            EvalResult::RangeVector(Vec::new()),
        )
        .unwrap();
        match result {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert_eq!(v[0].samples[0].timestamp_ms, 2000);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }
}
