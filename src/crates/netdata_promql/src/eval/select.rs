// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluation of vector and matrix selectors against the storage adapter.

use crate::storage::{Matcher, NdQuery};

use super::context::EvalContext;
use super::lookback::pick_latest_within_window;
use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

/// Evaluate a `Plan::VectorSelect` at `ctx.at_ms`.
///
/// Procedure: resolve candidates from storage, fetch samples in the
/// lookback window, pick the latest within window per series, drop series
/// with no qualifying sample.
pub fn eval_vector_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    offset_ms: i64,
) -> Result<EvalResult, EvalError> {
    let effective_t_ms = ctx.at_ms - offset_ms;
    let after_s = (effective_t_ms - ctx.lookback_ms) / 1000;
    let before_s = effective_t_ms / 1000;

    let q = match NdQuery::resolve(
        ctx.host_machine_guid.as_deref(),
        matchers,
        after_s,
        before_s,
        ctx.max_series,
    ) {
        Ok(q) => q,
        Err(crate::storage::ResolveError::Empty) => {
            return Ok(EvalResult::InstantVector(Vec::new()));
        }
        Err(e) => return Err(EvalError::Storage(e)),
    };

    if q.was_truncated() {
        return Err(EvalError::Other(
            "exceeded max_series during resolution; tighten the query or raise the cap".into(),
        ));
    }

    let mut out = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(view) = q.series(i) else { continue };
        let labels: Vec<(String, String)> = view
            .labels()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        let signature = labels_signature(&labels);

        // Fetch native-resolution samples in the lookback window.
        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };
        let samples: Vec<Sample> = samples_iter
            .map(|s| Sample {
                timestamp_ms: s.timestamp_ms,
                value: s.value,
            })
            .filter(|s| !s.value.is_nan())
            .collect();

        match pick_latest_within_window(&samples, effective_t_ms, ctx.lookback_ms) {
            Some(picked) => out.push(Series {
                labels,
                signature,
                // Stamp at the query time, not the sample's actual time.
                // Prometheus does this so a range query returns aligned
                // timestamps regardless of underlying collection cadence.
                samples: vec![Sample {
                    timestamp_ms: ctx.at_ms,
                    value: picked.value,
                }],
            }),
            None => continue,
        }
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Evaluate a `Plan::MatrixSelect`. Returns every sample in the range
/// window per series; no per-series collapse.
pub fn eval_matrix_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
) -> Result<EvalResult, EvalError> {
    let effective_t_ms = ctx.at_ms - offset_ms;
    let after_s = (effective_t_ms - range_ms) / 1000;
    let before_s = effective_t_ms / 1000;

    let q = match NdQuery::resolve(
        ctx.host_machine_guid.as_deref(),
        matchers,
        after_s,
        before_s,
        ctx.max_series,
    ) {
        Ok(q) => q,
        Err(crate::storage::ResolveError::Empty) => {
            return Ok(EvalResult::RangeVector(Vec::new()));
        }
        Err(e) => return Err(EvalError::Storage(e)),
    };

    if q.was_truncated() {
        return Err(EvalError::Other(
            "exceeded max_series during resolution; tighten the query or raise the cap".into(),
        ));
    }

    let mut out = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(view) = q.series(i) else { continue };
        let labels: Vec<(String, String)> = view
            .labels()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        let signature = labels_signature(&labels);

        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };
        let samples: Vec<Sample> = samples_iter
            .map(|s| Sample {
                timestamp_ms: s.timestamp_ms,
                value: s.value,
            })
            .filter(|s| !s.value.is_nan())
            .collect();

        if samples.is_empty() {
            continue;
        }
        out.push(Series {
            labels,
            signature,
            samples,
        });
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::RangeVector(out))
}
