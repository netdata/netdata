// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluation of vector and matrix selectors against the storage adapter.
//
// SOW-0031: selectors are grid-aware. One storage fetch per series per
// query covers the full evaluation window; the per-grid-point latest-in-
// lookback pick is a single sweep over the fetched samples (two-pointer
// for vector selectors, no per-step storage I/O).

use crate::plan::AtMod;
use crate::storage::Matcher;

use super::context::EvalContext;
use super::types::{EvalError, EvalResult, Sample, Series};

/// Resolve the `@` modifier (if any) against the current eval context.
/// `AtMod::AtTs(ms)` -> `ms`; `Start` -> outer range start; `End` ->
/// outer range end. When `at` is `None`, returns `None`, signalling that
/// the selector follows the grid timeline rather than a pinned point.
fn resolve_at(ctx: &EvalContext, at: Option<&AtMod>) -> Option<i64> {
    at.map(|a| match a {
        AtMod::AtTs(ms) => *ms,
        AtMod::Start => ctx.outer_start_ms,
        AtMod::End => ctx.outer_end_ms,
    })
}

/// Evaluate a `Plan::VectorSelect` across the eval grid.
///
/// For each resolved series, produces a grid-aligned `Series` whose
/// `samples` has length `grid.len()`. At grid point `t` the emitted
/// value is the latest non-NaN sample with timestamp in
/// `(t - lookback_ms, t]`, or `NaN` if no such sample exists (Prometheus
/// staleness rule).
///
/// `offset_ms` shifts the lookback window leftward by that many ms.
/// The `@` modifier, when present, pins every grid point to the
/// resolved timestamp (broadcast).
pub fn eval_vector_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    let grid = &ctx.grid;

    // When `@` pins the eval to a fixed timestamp, every grid point
    // gets the same sample. Build a single-point fetch and broadcast.
    let pinned = resolve_at(ctx, at);

    // Fetch window in milliseconds. For the grid case we fetch
    // [grid.start - offset - lookback, grid.end - offset] in one shot.
    // The shim works in whole seconds; rely on the precise-ms filter
    // below to drop boundary-overshoot samples.
    let (fetch_after_ms, fetch_before_ms) = match pinned {
        Some(t) => (t - offset_ms - ctx.lookback_ms, t - offset_ms),
        None => (
            grid.start_ms - offset_ms - ctx.lookback_ms,
            grid.end_ms - offset_ms,
        ),
    };
    let after_s = fetch_after_ms / 1000;
    let before_s = fetch_before_ms / 1000;

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

    let lookback_ms = ctx.lookback_ms;
    let mut out: Vec<Series> = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };
        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };

        // Materialize the per-series sample list once. Iterator-based
        // two-pointer is awkward when grid points can repeat samples
        // (lookback windows overlap), so a Vec keeps the code simple
        // and the allocation amortises against the single storage
        // fetch we just did.
        let samples: Vec<Sample> = samples_iter
            .filter(|s| !s.value.is_nan())
            .map(|s| Sample {
                timestamp_ms: s.timestamp_ms,
                value: s.value,
            })
            .collect();
        if samples.is_empty() {
            continue;
        }

        let out_samples: Vec<Sample> = if let Some(t) = pinned {
            // `@`-pinned: pick once, broadcast to every grid point.
            let picked = pick_latest_in_window(
                &samples,
                t.saturating_sub(offset_ms).saturating_sub(lookback_ms),
                t.saturating_sub(offset_ms),
            );
            let value = picked.unwrap_or(f64::NAN);
            grid.timestamps
                .iter()
                .map(|&ts| Sample {
                    timestamp_ms: ts,
                    value,
                })
                .collect()
        } else {
            // Grid-aligned: two-pointer scan over samples to find the
            // latest in window for each grid timestamp. Both the grid
            // and the samples are monotonically non-decreasing, so the
            // scan is O(n + g).
            grid_aligned_lookback(&samples, &grid.timestamps, offset_ms, lookback_ms)
        };

        // If every emitted sample is NaN, drop the series entirely.
        // Matches the "no qualifying sample, no output" rule from the
        // pre-SOW-0031 evaluator.
        if out_samples.iter().all(|s| s.value.is_nan()) {
            continue;
        }

        out.push(Series {
            labels: meta.labels,
            signature: meta.signature,
            samples: out_samples,
        });
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Single-pass two-pointer lookback. For each grid timestamp `t[i]`,
/// pick the latest non-NaN sample with timestamp in `(t - offset -
/// lookback, t - offset]`. Returns one `Sample` per grid timestamp
/// stamped at the grid time (Prometheus stamps the evaluation time,
/// not the underlying sample time).
fn grid_aligned_lookback(
    samples: &[Sample],
    grid_ts: &[i64],
    offset_ms: i64,
    lookback_ms: i64,
) -> Vec<Sample> {
    let mut out = Vec::with_capacity(grid_ts.len());
    let mut idx: usize = 0;
    let mut current: Option<f64> = None;
    let mut current_ts: i64 = i64::MIN;
    for &g in grid_ts {
        let upper = g.saturating_sub(offset_ms);
        let lower = upper.saturating_sub(lookback_ms);
        // Advance idx past every sample with ts <= upper, keeping the
        // most recent in `current`. Samples below `lower` will be
        // evicted on the next grid point's lower-bound check below.
        while idx < samples.len() && samples[idx].timestamp_ms <= upper {
            current = Some(samples[idx].value);
            current_ts = samples[idx].timestamp_ms;
            idx += 1;
        }
        // Evict if the latest seen is no longer within the lookback
        // window for this grid point.
        let value = if current.is_some() && current_ts > lower {
            current.unwrap()
        } else {
            f64::NAN
        };
        out.push(Sample {
            timestamp_ms: g,
            value,
        });
    }
    out
}

/// Linear scan to find the latest sample with timestamp in
/// `(lower_ms, upper_ms]`. Used only for the `@`-pinned single-point
/// path.
fn pick_latest_in_window(samples: &[Sample], lower_ms: i64, upper_ms: i64) -> Option<f64> {
    let mut picked: Option<f64> = None;
    for s in samples {
        if s.timestamp_ms <= lower_ms {
            continue;
        }
        if s.timestamp_ms > upper_ms {
            break;
        }
        picked = Some(s.value);
    }
    picked
}

/// Evaluate a `Plan::MatrixSelect` across the eval grid.
///
/// Returns a RangeVector spanning the full extended window
/// `[grid.start_ms - range_ms - offset_ms, grid.end_ms - offset_ms]`.
/// The downstream rollup uses two-pointer windowing over this sample
/// pool to emit one value per grid point.
///
/// SOW-0031 keeps the `EvalResult::RangeVector` shape unchanged; the
/// rollup functions know the range width because the dispatch layer
/// pulls it out of the Plan::MatrixSelect node and threads it through
/// `apply_call`.
pub fn eval_matrix_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    let grid = &ctx.grid;
    let pinned = resolve_at(ctx, at);

    // Fetch window: covers all per-grid-point matrix windows.
    let (fetch_after_ms, fetch_before_ms) = match pinned {
        Some(t) => (
            t.saturating_sub(offset_ms).saturating_sub(range_ms),
            t - offset_ms,
        ),
        None => (
            grid.start_ms
                .saturating_sub(offset_ms)
                .saturating_sub(range_ms),
            grid.end_ms - offset_ms,
        ),
    };
    let after_s = fetch_after_ms / 1000;
    let before_s = fetch_before_ms / 1000;

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

    let mut out = Vec::with_capacity(q.len());
    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };

        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };
        let samples: Vec<Sample> = samples_iter
            .filter(|s| !s.value.is_nan())
            .filter(|s| s.timestamp_ms >= fetch_after_ms && s.timestamp_ms <= fetch_before_ms)
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
