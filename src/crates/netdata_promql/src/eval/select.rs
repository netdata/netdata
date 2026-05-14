// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluation of vector and matrix selectors against the storage adapter.
//
// SOW-0031: selectors are grid-aware. One storage fetch per series per
// query covers the full evaluation window; the per-grid-point latest-in-
// lookback pick is a single sweep over the fetched samples (two-pointer
// for vector selectors, no per-step storage I/O).
//
// SOW-0032: output is column-oriented. Vector selectors clone the grid's
// shared `Arc<Vec<i64>>` for every emitted series; matrix selectors
// build a fresh `Arc<Vec<i64>>` from each series' storage timestamps.

use std::sync::Arc;

use crate::plan::AtMod;
use crate::storage::Matcher;

use super::context::EvalContext;
use super::types::{EvalError, EvalResult, Series};

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
/// values has length `grid.len()`. At grid point `t` the emitted
/// value is the latest non-NaN sample with timestamp in
/// `(t - lookback_ms, t]`, or `NaN` if no such sample exists (Prometheus
/// staleness rule). All output series share `grid.timestamps` Arc.
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
    let pinned = resolve_at(ctx, at);

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
    // Shared timestamps Arc for every output series. SOW-0032: this is
    // the entire point of the column-oriented shape -- 100 series at
    // 241 grid points share one 192-byte allocation instead of 100
    // copies of the same column.
    let grid_timestamps = Arc::clone(&grid.timestamps);

    // Per-query drain buffers reused across the per-series loop
    // (SOW-0040). `drain_samples` clears them on each call and the
    // selector filters NaN inline.
    let mut raw_ts: Vec<i64> = Vec::new();
    let mut raw_vals: Vec<f64> = Vec::new();
    let mut timestamps_buf: Vec<i64> = Vec::new();
    let mut values_buf: Vec<f64> = Vec::new();

    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };
        q.drain_samples(i, after_s, before_s, 0, &mut raw_ts, &mut raw_vals);

        // Filter NaN once into per-series column buffers. The two-pointer
        // scan below indexes into these slices.
        timestamps_buf.clear();
        values_buf.clear();
        for k in 0..raw_vals.len() {
            let v = raw_vals[k];
            if v.is_nan() {
                continue;
            }
            timestamps_buf.push(raw_ts[k]);
            values_buf.push(v);
        }
        if values_buf.is_empty() {
            continue;
        }

        let out_values: Vec<f64> = if let Some(t) = pinned {
            // `@`-pinned: pick once, broadcast to every grid point.
            let value = pick_latest_in_window(
                &timestamps_buf,
                &values_buf,
                t.saturating_sub(offset_ms).saturating_sub(lookback_ms),
                t.saturating_sub(offset_ms),
            )
            .unwrap_or(f64::NAN);
            vec![value; grid.len()]
        } else {
            grid_aligned_lookback(
                &timestamps_buf,
                &values_buf,
                &grid.timestamps,
                offset_ms,
                lookback_ms,
            )
        };

        if out_values.iter().all(|v| v.is_nan()) {
            continue;
        }

        out.push(Series::new(meta.labels, Arc::clone(&grid_timestamps), out_values));
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Single-pass two-pointer lookback. For each grid timestamp `t[i]`,
/// pick the latest non-NaN sample with timestamp in `(t - offset -
/// lookback, t - offset]`. Returns a `Vec<f64>` aligned to the grid;
/// the caller stamps with grid timestamps.
fn grid_aligned_lookback(
    timestamps: &[i64],
    values: &[f64],
    grid_ts: &[i64],
    offset_ms: i64,
    lookback_ms: i64,
) -> Vec<f64> {
    let mut out = Vec::with_capacity(grid_ts.len());
    let mut idx: usize = 0;
    let mut current: Option<f64> = None;
    let mut current_ts: i64 = i64::MIN;
    for &g in grid_ts {
        let upper = g.saturating_sub(offset_ms);
        let lower = upper.saturating_sub(lookback_ms);
        while idx < timestamps.len() && timestamps[idx] <= upper {
            current = Some(values[idx]);
            current_ts = timestamps[idx];
            idx += 1;
        }
        let value = if current.is_some() && current_ts > lower {
            current.unwrap()
        } else {
            f64::NAN
        };
        out.push(value);
    }
    out
}

/// Linear scan to find the latest sample with timestamp in
/// `(lower_ms, upper_ms]`. Used only for the `@`-pinned single-point
/// path.
fn pick_latest_in_window(
    timestamps: &[i64],
    values: &[f64],
    lower_ms: i64,
    upper_ms: i64,
) -> Option<f64> {
    let mut picked: Option<f64> = None;
    for (i, &ts) in timestamps.iter().enumerate() {
        if ts <= lower_ms {
            continue;
        }
        if ts > upper_ms {
            break;
        }
        picked = Some(values[i]);
    }
    picked
}

/// Evaluate a `Plan::MatrixSelect` across the eval grid.
///
/// Returns a RangeVector spanning the full extended window
/// `[grid.start_ms - range_ms - offset_ms, grid.end_ms - offset_ms]`.
/// Each series carries its own `Arc<Vec<i64>>` of storage timestamps;
/// no sharing across series because storage samples are not grid-
/// aligned and timestamps differ per series. The downstream rollup uses
/// two-pointer windowing over this sample pool to emit one value per
/// grid point.
pub fn eval_matrix_select(
    ctx: &EvalContext,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    let grid = &ctx.grid;
    let pinned = resolve_at(ctx, at);

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
    let mut raw_ts: Vec<i64> = Vec::new();
    let mut raw_vals: Vec<f64> = Vec::new();
    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };
        q.drain_samples(i, after_s, before_s, 0, &mut raw_ts, &mut raw_vals);

        // The matrix selector keeps the per-series timestamps as its
        // own Arc (sample timestamps differ per series), so each pass
        // moves the filtered samples into freshly-allocated columns.
        let mut timestamps: Vec<i64> = Vec::with_capacity(raw_ts.len());
        let mut values: Vec<f64> = Vec::with_capacity(raw_vals.len());
        for k in 0..raw_vals.len() {
            let v = raw_vals[k];
            let t = raw_ts[k];
            if v.is_nan() {
                continue;
            }
            if t < fetch_after_ms || t > fetch_before_ms {
                continue;
            }
            timestamps.push(t);
            values.push(v);
        }

        if values.is_empty() {
            continue;
        }
        out.push(Series::new(meta.labels, Arc::new(timestamps), values));
    }

    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::RangeVector(out))
}
