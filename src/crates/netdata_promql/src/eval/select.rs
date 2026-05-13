// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluation of vector and matrix selectors against the storage adapter.

use crate::plan::AtMod;
use crate::storage::{Matcher, NdQuery};

use super::context::EvalContext;
use super::lookback::pick_latest_within_window;
use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

/// Resolve the `@` modifier (if any) against the current eval context.
/// `AtMod::AtTs(ms)` -> `ms`; `Start` -> outer range start; `End` ->
/// outer range end. When `at` is `None`, returns `ctx.at_ms`.
fn resolve_at(ctx: &EvalContext, at: Option<&AtMod>) -> i64 {
    match at {
        None => ctx.at_ms,
        Some(AtMod::AtTs(ms)) => *ms,
        Some(AtMod::Start) => ctx.outer_start_ms,
        Some(AtMod::End) => ctx.outer_end_ms,
    }
}

/// Evaluate a `Plan::VectorSelect` at `ctx.at_ms` (or at the `@`-resolved
/// time when an at-modifier is present).
///
/// Procedure: resolve candidates from storage, fetch samples in the
/// lookback window, pick the latest within window per series, drop series
/// with no qualifying sample.
pub fn eval_vector_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    let base_t_ms = resolve_at(ctx, at);
    let effective_t_ms = base_t_ms - offset_ms;
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
/// window per series; no per-series collapse. The `at` modifier shifts
/// the window's right edge to the resolved timestamp.
pub fn eval_matrix_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    let base_t_ms = resolve_at(ctx, at);
    let effective_t_ms = base_t_ms - offset_ms;
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
