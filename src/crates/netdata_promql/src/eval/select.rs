// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluation of vector and matrix selectors against the storage adapter.

use crate::plan::AtMod;
use crate::storage::Matcher;

use super::context::EvalContext;
use super::types::{EvalError, EvalResult, Sample, Series};

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

    let q = match ctx.backend.resolve(
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

    // Lookback rule: of the samples the storage iterator returns for
    // each series in the `[after_s, before_s]` window, pick the latest
    // whose timestamp falls inside the precise millisecond window
    // `[effective_t_ms - lookback_ms, effective_t_ms]` and whose value
    // is not NaN. Walking forward and keeping a running "latest seen"
    // is equivalent to a reverse-scan because the shim guarantees
    // non-decreasing timestamps. SOW-0029 / SOW-0030.
    let earliest_ms = effective_t_ms.saturating_sub(ctx.lookback_ms);

    let mut out = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };

        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };

        // Single-pass fold over the per-series sample iterator. No
        // intermediate `Vec` allocation; we only need the value of
        // the latest qualifying sample.
        let mut picked_value: Option<f64> = None;
        for s in samples_iter {
            if s.value.is_nan() {
                continue;
            }
            if s.timestamp_ms < earliest_ms || s.timestamp_ms > effective_t_ms {
                // The shim's window check is in whole seconds, so a
                // few samples at the boundary may fall outside the
                // strict millisecond bounds; skip those.
                continue;
            }
            // Samples arrive in non-decreasing order, so each in-window
            // non-NaN sample is at least as recent as the previous one.
            // Overwrite unconditionally; the last write wins.
            picked_value = Some(s.value);
        }

        if let Some(value) = picked_value {
            out.push(Series {
                labels: meta.labels,
                signature: meta.signature,
                // Stamp at the query time, not the sample's actual time.
                // Prometheus does this so a range query returns aligned
                // timestamps regardless of underlying collection cadence.
                samples: vec![Sample {
                    timestamp_ms: ctx.at_ms,
                    value,
                }],
            });
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

    let q = match ctx.backend.resolve(
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

    // Use the precise millisecond window for the matrix selector too;
    // the second-resolution shim boundary may include samples just
    // outside the strict range.
    let earliest_ms = effective_t_ms.saturating_sub(range_ms);

    let mut out = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };

        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };
        let samples: Vec<Sample> = samples_iter
            .filter(|s| !s.value.is_nan())
            .filter(|s| s.timestamp_ms >= earliest_ms && s.timestamp_ms <= effective_t_ms)
            .map(|s| Sample {
                timestamp_ms: s.timestamp_ms,
                value: s.value,
            })
            .collect();

        if samples.is_empty() {
            continue;
        }
        out.push(Series {
            labels: meta.labels,
            signature: meta.signature,
            samples,
        });
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::RangeVector(out))
}

