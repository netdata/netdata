// SPDX-License-Identifier: GPL-3.0-or-later
//
// Built-in PromQL functions: counter-family (rate, irate, increase,
// delta), histogram_quantile, _over_time family, per-sample math
// transforms, bounded transforms, vector restructuring, and more.
//
// Counter semantics: rate/irate/increase work directly on the deltas
// Netdata's storage already produced. We do not reconstruct a cumulative
// counter value, and we do not infer resets from sample-to-sample
// decreases -- if the storage flagged a reset, the delta at that bucket
// already reflects it.

use std::collections::BTreeMap;

use crate::plan::FuncKind;

use super::context::EvalContext;
use super::types::{EvalError, EvalResult, Sample, Series, labels_signature};

pub fn apply_call(
    ctx: &EvalContext,
    func: FuncKind,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    match func {
        FuncKind::Rate => apply_window_op(ctx, args, range_ms, WindowOp::Rate),
        FuncKind::Increase => apply_window_op(ctx, args, range_ms, WindowOp::Increase),
        FuncKind::IRate => apply_irate(ctx, args, range_ms),
        FuncKind::Delta => apply_delta(ctx, args, range_ms),
        FuncKind::HistogramQuantile => apply_histogram_quantile(args),
        FuncKind::AvgOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Avg),
        FuncKind::SumOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Sum),
        FuncKind::MinOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Min),
        FuncKind::MaxOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Max),
        FuncKind::CountOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Count),
        FuncKind::LastOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Last),
        FuncKind::FirstOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::First),
        FuncKind::PresentOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Present),
        FuncKind::StddevOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Stddev),
        FuncKind::StdvarOverTime => apply_over_time(ctx, args, range_ms, OverTimeOp::Stdvar),
        FuncKind::QuantileOverTime => apply_quantile_over_time(ctx, args, range_ms),
        FuncKind::PredictLinear => apply_predict_linear(ctx, args, range_ms),
        FuncKind::HoltWinters => apply_holt_winters(ctx, args, range_ms),
        // Per-sample math transforms.
        FuncKind::Abs => apply_per_sample(args, "abs", f64::abs),
        FuncKind::Ceil => apply_per_sample(args, "ceil", f64::ceil),
        FuncKind::Floor => apply_per_sample(args, "floor", f64::floor),
        FuncKind::Sgn => apply_per_sample(args, "sgn", sgn),
        FuncKind::Ln => apply_per_sample(args, "ln", f64::ln),
        FuncKind::Log2 => apply_per_sample(args, "log2", f64::log2),
        FuncKind::Log10 => apply_per_sample(args, "log10", f64::log10),
        FuncKind::Exp => apply_per_sample(args, "exp", f64::exp),
        FuncKind::Sqrt => apply_per_sample(args, "sqrt", f64::sqrt),
        // Bounded transforms / rounding.
        FuncKind::Clamp => apply_clamp(args),
        FuncKind::ClampMin => apply_clamp_min(args),
        FuncKind::ClampMax => apply_clamp_max(args),
        FuncKind::Round => apply_round(args),
        // Vector restructuring.
        FuncKind::Vector => apply_vector(args),
        FuncKind::Scalar => apply_scalar(args),
        FuncKind::Sort => apply_sort(args, false),
        FuncKind::SortDesc => apply_sort(args, true),
        // Time / timestamp. `time()` is plumbed through dispatch because the
        // evaluator needs the eval-context's at_ms; we encode that
        // requirement here by having dispatch.rs reject `Time` from this
        // path and produce the scalar directly.
        FuncKind::Time => Err(EvalError::Other(
            "time() is evaluated by the dispatcher".to_string(),
        )),
        FuncKind::Timestamp => apply_timestamp(args),
        // Range-vector reductions.
        FuncKind::Deriv => apply_deriv(ctx, args, range_ms),
        FuncKind::IDelta => apply_idelta(ctx, args, range_ms),
        FuncKind::Changes => apply_changes(ctx, args, range_ms),
        FuncKind::Resets => apply_resets(ctx, args, range_ms),
    }
}

/// Apply `compute` to a sliding window over the column-shaped
/// `(timestamps, values)` series at each grid timestamp. The window
/// is the half-open `(t - range_ms, t]`. Returns one `f64` per grid
/// timestamp; positions where `compute` returns `None` get NaN. Used
/// by every range-vector rollup to produce grid-aligned output in a
/// single pass over the underlying sample sequence.
fn rollup_two_pointer<F>(
    timestamps: &[i64],
    values: &[f64],
    grid_ts: &[i64],
    range_ms: i64,
    mut compute: F,
) -> Vec<f64>
where
    F: FnMut(&[i64], &[f64]) -> Option<f64>,
{
    debug_assert_eq!(timestamps.len(), values.len());
    let mut out = Vec::with_capacity(grid_ts.len());
    let mut left: usize = 0;
    let mut right: usize = 0;
    for &t in grid_ts {
        let lower = t.saturating_sub(range_ms);
        while right < timestamps.len() && timestamps[right] <= t {
            right += 1;
        }
        while left < right && timestamps[left] <= lower {
            left += 1;
        }
        let window_ts = &timestamps[left..right];
        let window_vals = &values[left..right];
        out.push(compute(window_ts, window_vals).unwrap_or(f64::NAN));
    }
    out
}

/// Windowed aggregator selector. The variants correspond to the seven
/// `*_over_time` functions.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum OverTimeOp {
    Avg,
    Sum,
    Min,
    Max,
    Count,
    Last,
    First,
    Present,
    Stddev,
    Stdvar,
}

impl OverTimeOp {
    fn name(self) -> &'static str {
        match self {
            OverTimeOp::Avg => "avg_over_time",
            OverTimeOp::Sum => "sum_over_time",
            OverTimeOp::Min => "min_over_time",
            OverTimeOp::Max => "max_over_time",
            OverTimeOp::Count => "count_over_time",
            OverTimeOp::Last => "last_over_time",
            OverTimeOp::First => "first_over_time",
            OverTimeOp::Present => "present_over_time",
            OverTimeOp::Stddev => "stddev_over_time",
            OverTimeOp::Stdvar => "stdvar_over_time",
        }
    }

    /// True if the output series should keep `__name__`. Prometheus
    /// strips `__name__` for the aggregating variants (they produce a
    /// different statistic than the source metric) but preserves it for
    /// `last_over_time` (the value is still an observation of the same
    /// metric, just the most recent in the window).
    /// `present_over_time` strips because the result is always 1 or
    /// nothing -- structurally a different quantity.
    fn keep_name(self) -> bool {
        matches!(self, OverTimeOp::Last | OverTimeOp::First)
    }
}

fn apply_over_time(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
    op: OverTimeOp,
) -> Result<EvalResult, EvalError> {
    match op {
        OverTimeOp::Avg => apply_over_time_typed::<AvgReducer>(ctx, args, range_ms),
        OverTimeOp::Sum => apply_over_time_typed::<SumReducer>(ctx, args, range_ms),
        OverTimeOp::Min => apply_over_time_typed::<MinReducer>(ctx, args, range_ms),
        OverTimeOp::Max => apply_over_time_typed::<MaxReducer>(ctx, args, range_ms),
        OverTimeOp::Count => apply_over_time_typed::<CountReducer>(ctx, args, range_ms),
        OverTimeOp::Last => apply_over_time_typed::<LastReducer>(ctx, args, range_ms),
        OverTimeOp::First => apply_over_time_typed::<FirstReducer>(ctx, args, range_ms),
        OverTimeOp::Present => apply_over_time_typed::<PresentReducer>(ctx, args, range_ms),
        OverTimeOp::Stddev => apply_over_time_typed::<StddevReducer>(ctx, args, range_ms),
        OverTimeOp::Stdvar => apply_over_time_typed::<StdvarReducer>(ctx, args, range_ms),
    }
}

fn apply_over_time_typed<R: OverTimeReducer>(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, R::NAME)?;
    let Some(window_ms) = range_ms else {
        return apply_over_time_legacy::<R>(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |ts, vals| compute_over_time::<R>(ts, vals).map(|(v, _)| v),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let labels = if R::KEEP_NAME {
            s.labels
        } else {
            strip_name(&s.labels).0
        };
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn apply_over_time_legacy<R: OverTimeReducer>(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some((value, ts_ms)) = compute_over_time::<R>(&s.timestamps, &s.values) else {
            continue;
        };
        let labels = if R::KEEP_NAME {
            s.labels
        } else {
            strip_name(&s.labels).0
        };
        out.push(Series::scalar(labels, ts_ms, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

// ---------------------------------------------------------------------------
// OverTimeReducer trait + per-op accumulators.
//
// Each `*_over_time` operator is a separate impl with its own Acc shape,
// so the per-sample fold touches only the state that operator actually
// reads. This drops dead accumulator updates (e.g. `avg_over_time`
// no longer maintains min/max/sum_sq) and gives LLVM enough static
// information per impl to
// inline `step` into the calling loop and, where the reducer permits,
// auto-vectorize the value-side reduction.
//
// `step` handles NaN per the reducer's own semantic (every existing
// reducer skips NaN samples). `finalize` returns the (value, ts) pair
// the caller stamps onto the output sample; `None` means "no non-NaN
// sample was observed in the window" -- the caller drops that
// position. The (f64, i64) shape is uniform across reducers because
// `apply_over_time_legacy` consumes the `i64` to stamp single-window
// output series; ten of the eleven call sites discard it.
// ---------------------------------------------------------------------------

pub(crate) trait OverTimeReducer {
    type Acc: Default;
    /// Function name for error messages.
    const NAME: &'static str;
    /// True if the output series should preserve `__name__`.
    const KEEP_NAME: bool;
    fn step(acc: &mut Self::Acc, ts: i64, value: f64);
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)>;
}

#[derive(Default)]
pub(crate) struct AvgAcc {
    sum: f64,
    count: u64,
    last_ts: i64,
}
pub(crate) struct AvgReducer;
impl OverTimeReducer for AvgReducer {
    type Acc = AvgAcc;
    const NAME: &'static str = "avg_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.count += 1;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.count == 0 {
            None
        } else {
            Some((acc.sum / acc.count as f64, acc.last_ts))
        }
    }
}

#[derive(Default)]
pub(crate) struct SumAcc {
    sum: f64,
    count: u64,
    last_ts: i64,
}
pub(crate) struct SumReducer;
impl OverTimeReducer for SumReducer {
    type Acc = SumAcc;
    const NAME: &'static str = "sum_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.count += 1;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.count == 0 {
            None
        } else {
            Some((acc.sum, acc.last_ts))
        }
    }
}

pub(crate) struct MinAcc {
    value: f64,
    last_ts: i64,
    seen: bool,
}
impl Default for MinAcc {
    fn default() -> Self {
        Self {
            value: 0.0,
            last_ts: 0,
            seen: false,
        }
    }
}
pub(crate) struct MinReducer;
impl OverTimeReducer for MinReducer {
    type Acc = MinAcc;
    const NAME: &'static str = "min_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        if !acc.seen || value < acc.value {
            acc.value = value;
        }
        acc.seen = true;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.seen {
            Some((acc.value, acc.last_ts))
        } else {
            None
        }
    }
}

pub(crate) struct MaxAcc {
    value: f64,
    last_ts: i64,
    seen: bool,
}
impl Default for MaxAcc {
    fn default() -> Self {
        Self {
            value: 0.0,
            last_ts: 0,
            seen: false,
        }
    }
}
pub(crate) struct MaxReducer;
impl OverTimeReducer for MaxReducer {
    type Acc = MaxAcc;
    const NAME: &'static str = "max_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        if !acc.seen || value > acc.value {
            acc.value = value;
        }
        acc.seen = true;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.seen {
            Some((acc.value, acc.last_ts))
        } else {
            None
        }
    }
}

#[derive(Default)]
pub(crate) struct CountAcc {
    count: u64,
    last_ts: i64,
}
pub(crate) struct CountReducer;
impl OverTimeReducer for CountReducer {
    type Acc = CountAcc;
    const NAME: &'static str = "count_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.count += 1;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.count == 0 {
            None
        } else {
            Some((acc.count as f64, acc.last_ts))
        }
    }
}

pub(crate) struct LastAcc {
    value: f64,
    ts: i64,
    seen: bool,
}
impl Default for LastAcc {
    fn default() -> Self {
        Self {
            value: 0.0,
            ts: 0,
            seen: false,
        }
    }
}
pub(crate) struct LastReducer;
impl OverTimeReducer for LastReducer {
    type Acc = LastAcc;
    const NAME: &'static str = "last_over_time";
    const KEEP_NAME: bool = true;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.value = value;
        acc.ts = ts;
        acc.seen = true;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.seen {
            Some((acc.value, acc.ts))
        } else {
            None
        }
    }
}

pub(crate) struct FirstAcc {
    value: f64,
    ts: i64,
    seen: bool,
}
impl Default for FirstAcc {
    fn default() -> Self {
        Self {
            value: 0.0,
            ts: 0,
            seen: false,
        }
    }
}
pub(crate) struct FirstReducer;
impl OverTimeReducer for FirstReducer {
    type Acc = FirstAcc;
    const NAME: &'static str = "first_over_time";
    const KEEP_NAME: bool = true;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        if !acc.seen {
            acc.value = value;
            acc.ts = ts;
            acc.seen = true;
        }
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.seen {
            Some((acc.value, acc.ts))
        } else {
            None
        }
    }
}

#[derive(Default)]
pub(crate) struct PresentAcc {
    last_ts: i64,
    seen: bool,
}
pub(crate) struct PresentReducer;
impl OverTimeReducer for PresentReducer {
    type Acc = PresentAcc;
    const NAME: &'static str = "present_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.seen = true;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.seen {
            Some((1.0, acc.last_ts))
        } else {
            None
        }
    }
}

#[derive(Default)]
pub(crate) struct StddevAcc {
    sum: f64,
    sum_sq: f64,
    count: u64,
    last_ts: i64,
}
pub(crate) struct StddevReducer;
impl OverTimeReducer for StddevReducer {
    type Acc = StddevAcc;
    const NAME: &'static str = "stddev_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.sum_sq += value * value;
        acc.count += 1;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.count == 0 {
            return None;
        }
        let mean = acc.sum / acc.count as f64;
        let var = acc.sum_sq / acc.count as f64 - mean * mean;
        Some((if var < 0.0 { 0.0 } else { var.sqrt() }, acc.last_ts))
    }
}

pub(crate) struct StdvarReducer;
impl OverTimeReducer for StdvarReducer {
    type Acc = StddevAcc;
    const NAME: &'static str = "stdvar_over_time";
    const KEEP_NAME: bool = false;
    #[inline]
    fn step(acc: &mut Self::Acc, ts: i64, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.sum_sq += value * value;
        acc.count += 1;
        acc.last_ts = ts;
    }
    fn finalize(acc: &Self::Acc) -> Option<(f64, i64)> {
        if acc.count == 0 {
            return None;
        }
        let mean = acc.sum / acc.count as f64;
        let var = acc.sum_sq / acc.count as f64 - mean * mean;
        Some((if var < 0.0 { 0.0 } else { var }, acc.last_ts))
    }
}

/// Compute the windowed aggregate for one series, generic over the
/// per-op reducer. Returns the (value, ts) pair the output should carry,
/// or `None` when no non-NaN sample was observed.
#[inline]
pub(crate) fn compute_over_time<R: OverTimeReducer>(
    timestamps: &[i64],
    values: &[f64],
) -> Option<(f64, i64)> {
    debug_assert_eq!(timestamps.len(), values.len());
    let mut acc = <R::Acc as Default>::default();
    for i in 0..values.len() {
        R::step(&mut acc, timestamps[i], values[i]);
    }
    R::finalize(&acc)
}

/// Distinguish rate (per-second) from increase (absolute) -- the inner
/// arithmetic is identical except for the division by the window width.
#[derive(Debug, Clone, Copy)]
pub(crate) enum WindowOp {
    Rate,
    Increase,
}

fn expect_one_range_vector(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<Vec<Series>, EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires a range-vector argument")))?;
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly one argument"
        )));
    }
    match first {
        EvalResult::RangeVector(s) => Ok(s),
        other => Err(EvalError::Type {
            context: func,
            expected: crate::plan::ValueType::RangeVector,
            got: other.value_type(),
        }),
    }
}

fn apply_window_op(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
    op: WindowOp,
) -> Result<EvalResult, EvalError> {
    let name = match op {
        WindowOp::Rate => "rate",
        WindowOp::Increase => "increase",
    };
    let range = expect_one_range_vector(args, name)?;
    let Some(window_ms) = range_ms else {
        return apply_window_op_legacy(range, op);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |ts, vals| compute_window_op(ts, vals, op),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn apply_window_op_legacy(range: Vec<Series>, op: WindowOp) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_window_op(&s.timestamps, &s.values, op) else {
            continue;
        };
        let (labels, _) = strip_name(&s.labels);
        let ts = s.timestamps.last().copied().unwrap_or(0);
        out.push(Series::scalar(labels, ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

pub(crate) fn compute_window_op(timestamps: &[i64], values: &[f64], op: WindowOp) -> Option<f64> {
    if values.is_empty() {
        return None;
    }
    let total: f64 = values.iter().copied().filter(|v| !v.is_nan()).sum();
    match op {
        WindowOp::Increase => Some(total),
        WindowOp::Rate => {
            let first_ts = *timestamps.first()?;
            let last_ts = *timestamps.last()?;
            let span_ms = (last_ts - first_ts).max(0);
            if span_ms <= 0 {
                None
            } else {
                Some(total / (span_ms as f64 / 1000.0))
            }
        }
    }
}

/// irate: the per-second rate computed from the last two samples only.
/// Used for high-frequency change detection where rate() over a window
/// would smooth too aggressively.
fn apply_irate(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "irate")?;
    let Some(window_ms) = range_ms else {
        return apply_irate_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |ts, vals| compute_irate(ts, vals),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

pub(crate) fn compute_irate(timestamps: &[i64], values: &[f64]) -> Option<f64> {
    let n = values.len();
    if n < 2 {
        return None;
    }
    let last_v = values[n - 1];
    let last_ts = timestamps[n - 1];
    let prev_ts = timestamps[n - 2];
    let dt_ms = (last_ts - prev_ts).max(0);
    if dt_ms <= 0 || last_v.is_nan() {
        return None;
    }
    Some(last_v / (dt_ms as f64 / 1000.0))
}

fn apply_irate_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_irate(&s.timestamps, &s.values) else {
            continue;
        };
        let ts = s.timestamps.last().copied().unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// delta: gauge-style change over the window. Last sample minus first.
/// Phase 1 documented divergence: for INCREMENTAL dimensions where
/// stored values are already deltas, this returns the difference of two
/// deltas -- not what the user wants. Use rate()/increase() on counters
/// and delta() on gauges. We do not detect or warn.
fn apply_delta(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "delta")?;
    let Some(window_ms) = range_ms else {
        return apply_delta_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |_ts, vals| compute_delta(vals),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

pub(crate) fn compute_delta(values: &[f64]) -> Option<f64> {
    if values.len() < 2 {
        return None;
    }
    let first = *values.first()?;
    let last = *values.last()?;
    if first.is_nan() || last.is_nan() {
        return None;
    }
    Some(last - first)
}

fn apply_delta_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_delta(&s.values) else {
            continue;
        };
        let ts = s.timestamps.last().copied().unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// histogram_quantile(phi, instant_vector). Each input series carries an
/// `le` label encoding the cumulative bucket upper bound. We group by
/// every label except `le`, sort each group's buckets by `le`, find the
/// bucket where the cumulative count crosses `phi * total`, and linearly
/// interpolate within that bucket.
fn apply_histogram_quantile(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let mut it = args.into_iter();
    let phi = match it.next() {
        Some(EvalResult::Scalar(p)) => p,
        Some(other) => {
            return Err(EvalError::Type {
                context: "histogram_quantile phi",
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(
                "histogram_quantile requires (phi, vector)".to_string(),
            ));
        }
    };
    let vector = match it.next() {
        Some(EvalResult::InstantVector(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: "histogram_quantile vector",
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(
                "histogram_quantile requires (phi, vector)".to_string(),
            ));
        }
    };
    if it.next().is_some() {
        return Err(EvalError::Other(
            "histogram_quantile takes exactly two arguments".to_string(),
        ));
    }
    if !(0.0..=1.0).contains(&phi) || phi.is_nan() {
        return Err(EvalError::Other(format!(
            "histogram_quantile phi must be in [0, 1]; got {phi}"
        )));
    }

    // Group by labels minus `le`. The grouped buckets get sorted by `le`.
    let mut groups: BTreeMap<u64, (Vec<(String, String)>, Vec<HBucket>, i64)> = BTreeMap::new();
    let mut saw_le = false;
    for s in vector.into_iter() {
        let Some(le_str) = lookup_label(&s.labels, "le") else {
            continue;
        };
        saw_le = true;
        let le = parse_le(le_str);
        let Some(le) = le else { continue };
        let value = s.values.first().copied().unwrap_or(f64::NAN);
        let ts = s.timestamps.first().copied().unwrap_or(0);
        if value.is_nan() {
            continue;
        }
        // Project labels by dropping `le` (and __name__ -- histograms
        // shouldn't carry the source metric name into the output).
        let key_labels: Vec<(String, String)> = s
            .labels
            .iter()
            .filter(|(n, _)| n != "le" && n != "__name__")
            .cloned()
            .collect();
        let key = labels_signature(&key_labels);
        let entry = groups
            .entry(key)
            .or_insert_with(|| (key_labels, Vec::new(), ts));
        entry.1.push(HBucket { le, count: value });
        entry.2 = entry.2.max(ts);
    }

    if !saw_le {
        return Err(EvalError::Other(
            "histogram_quantile: no `le` label on any input series".to_string(),
        ));
    }

    let mut out = Vec::with_capacity(groups.len());
    for (_, (labels, mut buckets, ts)) in groups.into_iter() {
        buckets.sort_by(|a, b| a.le.partial_cmp(&b.le).unwrap_or(std::cmp::Ordering::Equal));
        let Some(value) = interpolate_quantile(&buckets, phi) else {
            continue;
        };
        out.push(Series::scalar(labels, ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn lookup_label<'a>(labels: &'a [(String, String)], name: &str) -> Option<&'a str> {
    labels
        .iter()
        .find(|(n, _)| n == name)
        .map(|(_, v)| v.as_str())
}

fn parse_le(le: &str) -> Option<f64> {
    match le {
        "+Inf" | "+inf" | "Inf" | "inf" => Some(f64::INFINITY),
        s => s.parse::<f64>().ok(),
    }
}

/// Linear-interpolation quantile over sorted cumulative buckets.
/// The Prometheus implementation:
///   - the total is the count of the last (+Inf) bucket
///   - find the bucket b where rank = phi * total crosses
///   - interpolate within b between its lower bound (prev bucket's le) and
///     its upper bound (b.le)
struct HBucket {
    le: f64,
    count: f64,
}

fn interpolate_quantile(buckets: &[HBucket], phi: f64) -> Option<f64> {
    if buckets.is_empty() {
        return None;
    }
    // Total is the cumulative count of the highest (+Inf) bucket. If +Inf
    // isn't present, fall back to the last bucket.
    let total = buckets.last().map(|b| b.count)?;
    if total <= 0.0 {
        return None;
    }
    let rank = phi * total;
    // Find first bucket whose cumulative count >= rank.
    let mut idx = buckets.len() - 1;
    for (i, b) in buckets.iter().enumerate() {
        if b.count >= rank {
            idx = i;
            break;
        }
    }
    let upper = buckets[idx].le;
    let upper_count = buckets[idx].count;
    if !upper.is_finite() {
        // Quantile lands in the +Inf bucket -- return the previous bucket's
        // upper bound to avoid emitting Infinity. Matches Prometheus.
        if idx == 0 {
            return Some(0.0);
        }
        return Some(buckets[idx - 1].le);
    }
    let (lower_le, lower_count) = if idx == 0 {
        (0.0, 0.0)
    } else {
        (buckets[idx - 1].le, buckets[idx - 1].count)
    };
    let bucket_width = upper - lower_le;
    let bucket_count = upper_count - lower_count;
    if bucket_width <= 0.0 || bucket_count <= 0.0 {
        return Some(upper);
    }
    let frac = (rank - lower_count) / bucket_count;
    Some(lower_le + bucket_width * frac.clamp(0.0, 1.0))
}

// ---------------------------------------------------------------------------
// Two-arg over_time variants and predictive functions
// ---------------------------------------------------------------------------

/// quantile_over_time(phi, range_vector): per series, sort non-NaN
/// samples ascending and apply the same linear-interpolation formula
/// used by the `quantile` aggregator. Phi outside [0, 1] clamps to
/// +-Inf per Prometheus. Empty window drops the series.
fn apply_quantile_over_time(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let (phi, range) = expect_scalar_then_range(args, "quantile_over_time")?;
    let Some(window_ms) = range_ms else {
        return apply_quantile_over_time_legacy(phi, range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |_ts, vals| compute_quantile_over_window(vals, phi),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_quantile_over_window(values: &[f64], phi: f64) -> Option<f64> {
    let mut sorted: Vec<f64> = values.iter().copied().filter(|v| !v.is_nan()).collect();
    if sorted.is_empty() {
        return None;
    }
    sorted.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
    Some(crate::eval::aggregation::compute_quantile(&sorted, phi))
}

fn apply_quantile_over_time_legacy(phi: f64, range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_quantile_over_window(&s.values, phi) else {
            continue;
        };
        let ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find(|(_, v)| !v.is_nan())
            .map(|(i, _)| s.timestamps[i])
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// predict_linear(range_vector, t): per series, fit y = slope*x + b on
/// (timestamp_s - now_s, value) pairs of non-NaN samples via ordinary
/// least squares, then return `intercept + slope * t` -- the
/// extrapolated value `t` seconds in the future. Window with fewer
/// than 2 distinct timestamps drops the series.
fn apply_predict_linear(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let (range, t_secs) = expect_range_then_scalar(args, "predict_linear")?;
    let Some(window_ms) = range_ms else {
        return apply_predict_linear_legacy(range, t_secs);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |ts, vals| compute_predict_linear(ts, vals, t_secs),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_predict_linear(timestamps: &[i64], values: &[f64], t_secs: f64) -> Option<f64> {
    let last_ts_ms = values.iter().enumerate().rev().find_map(|(i, v)| {
        if v.is_nan() {
            None
        } else {
            Some(timestamps[i])
        }
    })?;
    let now_s = (last_ts_ms as f64) / 1000.0;
    let mut n = 0.0;
    let mut sum_x = 0.0;
    let mut sum_y = 0.0;
    let mut sum_xx = 0.0;
    let mut sum_xy = 0.0;
    let mut first_x = f64::NAN;
    let mut distinct_x = false;
    for i in 0..values.len() {
        let v = values[i];
        if v.is_nan() {
            continue;
        }
        let x = (timestamps[i] as f64) / 1000.0 - now_s;
        let y = v;
        if first_x.is_nan() {
            first_x = x;
        } else if x != first_x {
            distinct_x = true;
        }
        n += 1.0;
        sum_x += x;
        sum_y += y;
        sum_xx += x * x;
        sum_xy += x * y;
    }
    if n < 2.0 || !distinct_x {
        return None;
    }
    let denom = n * sum_xx - sum_x * sum_x;
    if denom == 0.0 {
        return None;
    }
    let slope = (n * sum_xy - sum_x * sum_y) / denom;
    let intercept = (sum_y - slope * sum_x) / n;
    Some(intercept + slope * t_secs)
}

fn apply_predict_linear_legacy(range: Vec<Series>, t_secs: f64) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_predict_linear(&s.timestamps, &s.values, t_secs) else {
            continue;
        };
        let last_ts_ms = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts_ms, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// holt_winters(range_vector, sf, tf): double-exponential smoothing.
/// Both factors must be in (0, 1]; out-of-range params -> evaluation
/// error. Returns the smoothed value at the end of the window.
fn apply_holt_winters(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let (range, sf, tf) = expect_range_then_two_scalars(args, "holt_winters")?;
    if !(sf > 0.0 && sf <= 1.0) {
        return Err(EvalError::Other(format!(
            "holt_winters: smoothing factor must be in (0, 1]; got {sf}"
        )));
    }
    if !(tf > 0.0 && tf <= 1.0) {
        return Err(EvalError::Other(format!(
            "holt_winters: trend factor must be in (0, 1]; got {tf}"
        )));
    }
    let Some(window_ms) = range_ms else {
        return apply_holt_winters_legacy(range, sf, tf);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            |_ts, vals| compute_holt_winters(vals, sf, tf),
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_holt_winters(values: &[f64], sf: f64, tf: f64) -> Option<f64> {
    let mut iter = values.iter().copied().filter(|v| !v.is_nan());
    let first = iter.next()?;
    let second = iter.next()?;
    let mut s_prev = first;
    let mut b_prev = second - first;
    let mut s_curr = sf * second + (1.0 - sf) * (s_prev + b_prev);
    b_prev = tf * (s_curr - s_prev) + (1.0 - tf) * b_prev;
    s_prev = s_curr;
    for v in iter {
        let s_new = sf * v + (1.0 - sf) * (s_prev + b_prev);
        let b_new = tf * (s_new - s_prev) + (1.0 - tf) * b_prev;
        s_prev = s_new;
        b_prev = b_new;
        s_curr = s_new;
    }
    Some(s_curr)
}

fn apply_holt_winters_legacy(
    range: Vec<Series>,
    sf: f64,
    tf: f64,
) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_holt_winters(&s.values, sf, tf) else {
            continue;
        };
        let last_ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

// ---------------------------------------------------------------------------
// Two-arg argument-shape helpers
// ---------------------------------------------------------------------------

fn expect_scalar_then_range(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<(f64, Vec<Series>), EvalError> {
    let mut it = args.into_iter();
    let phi = match it.next() {
        Some(EvalResult::Scalar(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!(
                "{func} requires (scalar, range-vector)"
            )));
        }
    };
    let range = match it.next() {
        Some(EvalResult::RangeVector(s)) => s,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::RangeVector,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!(
                "{func} requires (scalar, range-vector)"
            )));
        }
    };
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly two arguments"
        )));
    }
    Ok((phi, range))
}

fn expect_range_then_scalar(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<(Vec<Series>, f64), EvalError> {
    let mut it = args.into_iter();
    let range = match it.next() {
        Some(EvalResult::RangeVector(s)) => s,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::RangeVector,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!(
                "{func} requires (range-vector, scalar)"
            )));
        }
    };
    let scalar = match it.next() {
        Some(EvalResult::Scalar(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!(
                "{func} requires (range-vector, scalar)"
            )));
        }
    };
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly two arguments"
        )));
    }
    Ok((range, scalar))
}

fn expect_range_then_two_scalars(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<(Vec<Series>, f64, f64), EvalError> {
    let mut it = args.into_iter();
    let range = match it.next() {
        Some(EvalResult::RangeVector(s)) => s,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::RangeVector,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!(
                "{func} requires (range-vector, scalar, scalar)"
            )));
        }
    };
    let sf = match it.next() {
        Some(EvalResult::Scalar(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!("{func} requires three arguments")));
        }
    };
    let tf = match it.next() {
        Some(EvalResult::Scalar(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
        None => {
            return Err(EvalError::Other(format!("{func} requires three arguments")));
        }
    };
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly three arguments"
        )));
    }
    Ok((range, sf, tf))
}

fn strip_name(labels: &[(String, String)]) -> (Vec<(String, String)>, u64) {
    let labels: Vec<(String, String)> = labels
        .iter()
        .filter(|(n, _)| n != "__name__")
        .cloned()
        .collect();
    let signature = labels_signature(&labels);
    (labels, signature)
}

// ---------------------------------------------------------------------------
// Per-sample transforms, bounded transforms, vector restructuring, etc.
// ---------------------------------------------------------------------------

/// Group A helper: apply a pure `f64 -> f64` transform to every sample of
/// every series in an instant vector. Drops `__name__` per Prometheus
/// convention.
fn apply_per_sample(
    args: Vec<EvalResult>,
    func: &'static str,
    op: fn(f64) -> f64,
) -> Result<EvalResult, EvalError> {
    let series = expect_one_instant_vector(args, func)?;
    let mut out = Vec::with_capacity(series.len());
    for s in series.into_iter() {
        let (labels, _) = strip_name(&s.labels);
        let values: Vec<f64> = s.values.iter().copied().map(op).collect();
        out.push(Series::new(labels, s.timestamps, values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Prometheus `sgn`: -1 for negative, 0 for zero (or NaN), +1 for positive.
fn sgn(v: f64) -> f64 {
    if v.is_nan() {
        f64::NAN
    } else if v > 0.0 {
        1.0
    } else if v < 0.0 {
        -1.0
    } else {
        0.0
    }
}

fn apply_clamp(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let (series, lo, hi) = expect_vec_then_two_scalars(args, "clamp")?;
    let lo = lo.min(hi);
    let hi = lo.max(hi);
    Ok(EvalResult::InstantVector(map_samples(series, |v| {
        if v.is_nan() {
            f64::NAN
        } else {
            v.clamp(lo, hi)
        }
    })))
}

fn apply_clamp_min(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let (series, lo) = expect_vec_then_scalar(args, "clamp_min")?;
    Ok(EvalResult::InstantVector(map_samples(series, |v| {
        if v.is_nan() { f64::NAN } else { v.max(lo) }
    })))
}

fn apply_clamp_max(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let (series, hi) = expect_vec_then_scalar(args, "clamp_max")?;
    Ok(EvalResult::InstantVector(map_samples(series, |v| {
        if v.is_nan() { f64::NAN } else { v.min(hi) }
    })))
}

/// `round(v)` rounds to the nearest integer; `round(v, n)` rounds to the
/// nearest multiple of `n`. Prometheus uses round-half-up; we mirror.
fn apply_round(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other("round requires an instant-vector argument".to_string()))?;
    let to_nearest = match it.next() {
        None => 1.0,
        Some(EvalResult::Scalar(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: "round to_nearest",
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
    };
    if it.next().is_some() {
        return Err(EvalError::Other(
            "round expects at most 2 arguments".to_string(),
        ));
    }
    let series = match first {
        EvalResult::InstantVector(s) => s,
        other => {
            return Err(EvalError::Type {
                context: "round",
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            });
        }
    };
    let scale = if to_nearest == 0.0 { 1.0 } else { to_nearest };
    Ok(EvalResult::InstantVector(map_samples(series, |v| {
        if v.is_nan() {
            f64::NAN
        } else {
            // Round-half-up via add-half-and-floor; matches Prometheus.
            (v / scale + 0.5).floor() * scale
        }
    })))
}

fn apply_vector(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other("vector requires a scalar argument".to_string()))?;
    if it.next().is_some() {
        return Err(EvalError::Other(
            "vector expects exactly 1 argument".to_string(),
        ));
    }
    let v = match first {
        EvalResult::Scalar(v) => v,
        other => {
            return Err(EvalError::Type {
                context: "vector",
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
    };
    // `vector(s)` returns one labelless series whose single sample has the
    // scalar value at timestamp 0; dispatch.rs stamps it at the eval time
    // when it materialises. We use 0 here and rely on the caller to fix the
    // timestamp -- but the simpler thing is to stamp now and skip the
    // post-fix. We don't have ctx access; the convention in Prometheus is
    // to use the eval time, and our dispatch already passes evaluated
    // results back without timestamp rewriting, so we stamp 0. Callers that
    // care (range queries) re-stamp on every step. Instant queries print
    // the timestamp as ctx.at_ms via the output layer regardless.
    let series = Series::scalar(Vec::new(), 0, v);
    Ok(EvalResult::InstantVector(vec![series]))
}

fn apply_scalar(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let series = expect_one_instant_vector(args, "scalar")?;
    let v = if series.len() == 1 {
        series[0].values.first().copied().unwrap_or(f64::NAN)
    } else {
        // Prometheus: scalar of zero or >1 series is NaN.
        f64::NAN
    };
    Ok(EvalResult::Scalar(v))
}

fn apply_sort(args: Vec<EvalResult>, descending: bool) -> Result<EvalResult, EvalError> {
    let mut series =
        expect_one_instant_vector(args, if descending { "sort_desc" } else { "sort" })?;
    series.sort_by(|a, b| {
        let av = a.values.first().copied().unwrap_or(f64::NAN);
        let bv = b.values.first().copied().unwrap_or(f64::NAN);
        // NaN goes to the end in both directions (matches Prometheus).
        match (av.is_nan(), bv.is_nan()) {
            (true, true) => std::cmp::Ordering::Equal,
            (true, false) => std::cmp::Ordering::Greater,
            (false, true) => std::cmp::Ordering::Less,
            (false, false) => {
                let ord = av.partial_cmp(&bv).unwrap_or(std::cmp::Ordering::Equal);
                if descending { ord.reverse() } else { ord }
            }
        }
    });
    Ok(EvalResult::InstantVector(series))
}

fn apply_timestamp(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let series = expect_one_instant_vector(args, "timestamp")?;
    let out: Vec<Series> = series
        .into_iter()
        .map(|s| {
            let (labels, _) = strip_name(&s.labels);
            // Per-position: emit value = sample's own timestamp in
            // seconds. Output keeps the grid-aligned timestamps Arc.
            let values: Vec<f64> = s.timestamps.iter().map(|&t| t as f64 / 1000.0).collect();
            Series::new(labels, s.timestamps, values)
        })
        .collect();
    Ok(EvalResult::InstantVector(out))
}

fn apply_deriv(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "deriv")?;
    let Some(window_ms) = range_ms else {
        return apply_deriv_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            compute_deriv,
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_deriv(timestamps: &[i64], values: &[f64]) -> Option<f64> {
    let finite: Vec<(i64, f64)> = timestamps
        .iter()
        .zip(values.iter())
        .filter(|&(_, v)| !v.is_nan())
        .map(|(&t, &v)| (t, v))
        .collect();
    if finite.len() < 2 {
        return None;
    }
    let last_ts = finite.last().unwrap().0 as f64 / 1000.0;
    let n = finite.len() as f64;
    let mut sx = 0.0;
    let mut sy = 0.0;
    let mut sxy = 0.0;
    let mut sxx = 0.0;
    for &(t, v) in &finite {
        let x = t as f64 / 1000.0 - last_ts;
        let y = v;
        sx += x;
        sy += y;
        sxy += x * y;
        sxx += x * x;
    }
    let denom = n * sxx - sx * sx;
    if denom == 0.0 {
        return None;
    }
    Some((n * sxy - sx * sy) / denom)
}

fn apply_deriv_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_deriv(&s.timestamps, &s.values) else {
            continue;
        };
        let last_ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn apply_idelta(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "idelta")?;
    let Some(window_ms) = range_ms else {
        return apply_idelta_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            compute_idelta,
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_idelta(_timestamps: &[i64], values: &[f64]) -> Option<f64> {
    let finite: Vec<f64> = values.iter().copied().filter(|v| !v.is_nan()).collect();
    if finite.len() < 2 {
        return None;
    }
    let last = finite[finite.len() - 1];
    let prev = finite[finite.len() - 2];
    Some(last - prev)
}

fn apply_idelta_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_idelta(&s.timestamps, &s.values) else {
            continue;
        };
        let last_ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn apply_changes(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "changes")?;
    let Some(window_ms) = range_ms else {
        return apply_changes_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            compute_changes,
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_changes(_timestamps: &[i64], values: &[f64]) -> Option<f64> {
    let finite: Vec<f64> = values.iter().copied().filter(|v| !v.is_nan()).collect();
    if finite.is_empty() {
        return None;
    }
    let mut count: f64 = 0.0;
    for w in finite.windows(2) {
        if w[0] != w[1] {
            count += 1.0;
        }
    }
    Some(count)
}

fn apply_changes_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_changes(&s.timestamps, &s.values) else {
            continue;
        };
        let last_ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn apply_resets(
    ctx: &EvalContext,
    args: Vec<EvalResult>,
    range_ms: Option<i64>,
) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "resets")?;
    let Some(window_ms) = range_ms else {
        return apply_resets_legacy(range);
    };
    let grid = &ctx.grid;
    let grid_ts = std::sync::Arc::clone(&grid.timestamps);
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let values = rollup_two_pointer(
            &s.timestamps,
            &s.values,
            &grid.timestamps,
            window_ms,
            compute_resets,
        );
        if values.iter().all(|v| v.is_nan()) {
            continue;
        }
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::new(labels, std::sync::Arc::clone(&grid_ts), values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

fn compute_resets(_timestamps: &[i64], values: &[f64]) -> Option<f64> {
    let finite: Vec<f64> = values.iter().copied().filter(|v| !v.is_nan()).collect();
    if finite.is_empty() {
        return None;
    }
    let mut count: f64 = 0.0;
    for w in finite.windows(2) {
        if w[1] < w[0] {
            count += 1.0;
        }
    }
    Some(count)
}

fn apply_resets_legacy(range: Vec<Series>) -> Result<EvalResult, EvalError> {
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_resets(&s.timestamps, &s.values) else {
            continue;
        };
        let last_ts = s
            .values
            .iter()
            .enumerate()
            .rev()
            .find_map(|(i, v)| {
                if v.is_nan() {
                    None
                } else {
                    Some(s.timestamps[i])
                }
            })
            .unwrap_or(0);
        let (labels, _) = strip_name(&s.labels);
        out.push(Series::scalar(labels, last_ts, value));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

// ---------------------------------------------------------------------------
// Shared argument-extraction helpers.
// ---------------------------------------------------------------------------

fn expect_one_instant_vector(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<Vec<Series>, EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires an instant-vector argument")))?;
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly 1 argument"
        )));
    }
    match first {
        EvalResult::InstantVector(s) => Ok(s),
        other => Err(EvalError::Type {
            context: func,
            expected: crate::plan::ValueType::InstantVector,
            got: other.value_type(),
        }),
    }
}

fn expect_vec_then_scalar(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<(Vec<Series>, f64), EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires (vector, scalar) arguments")))?;
    let second = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires a scalar second argument")))?;
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly 2 arguments"
        )));
    }
    let series = match first {
        EvalResult::InstantVector(s) => s,
        other => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            });
        }
    };
    let scalar = match second {
        EvalResult::Scalar(v) => v,
        other => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
    };
    Ok((series, scalar))
}

fn expect_vec_then_two_scalars(
    args: Vec<EvalResult>,
    func: &'static str,
) -> Result<(Vec<Series>, f64, f64), EvalError> {
    let mut it = args.into_iter();
    let first = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires (vector, scalar, scalar)")))?;
    let second = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires a 2nd scalar argument")))?;
    let third = it
        .next()
        .ok_or_else(|| EvalError::Other(format!("{func} requires a 3rd scalar argument")))?;
    if it.next().is_some() {
        return Err(EvalError::Other(format!(
            "{func} expects exactly 3 arguments"
        )));
    }
    let series = match first {
        EvalResult::InstantVector(s) => s,
        other => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            });
        }
    };
    let a = match second {
        EvalResult::Scalar(v) => v,
        other => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
    };
    let b = match third {
        EvalResult::Scalar(v) => v,
        other => {
            return Err(EvalError::Type {
                context: func,
                expected: crate::plan::ValueType::Scalar,
                got: other.value_type(),
            });
        }
    };
    Ok((series, a, b))
}

/// Apply a per-sample transform across an instant vector, dropping
/// `__name__` and preserving timestamps. Shared by clamp/round and
/// the other per-sample variants that need a non-`f64->f64` capture.
fn map_samples(series: Vec<Series>, op: impl Fn(f64) -> f64) -> Vec<Series> {
    let mut out: Vec<Series> = series
        .into_iter()
        .map(|s| {
            let (labels, _) = strip_name(&s.labels);
            let values: Vec<f64> = s.values.iter().copied().map(&op).collect();
            Series::new(labels, s.timestamps, values)
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn series(labels: Vec<(&str, &str)>, pairs: Vec<(i64, f64)>) -> Series {
        let owned: Vec<(String, String)> = labels
            .into_iter()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        let samples: Vec<Sample> = pairs
            .into_iter()
            .map(|(ts, v)| Sample {
                timestamp_ms: ts,
                value: v,
            })
            .collect();
        Series::from_samples(owned, samples)
    }

    #[test]
    fn rate_sums_deltas_per_second() {
        // 60 seconds of samples, each carrying delta=10 -> total 60 over 60s = 1/s.
        let mut samples = Vec::new();
        for i in 0..7 {
            samples.push((i * 10_000, 10.0));
        }
        let s = series(vec![("__name__", "foo")], samples);
        let r = apply_window_op(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
            WindowOp::Rate,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                let value = v[0].values[0];
                // 7 deltas of 10 = 70, span = 60s -> ~1.166/s.
                assert!((value - 70.0 / 60.0).abs() < 1e-6, "got {}", value);
                // __name__ stripped.
                assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn increase_sums_deltas_absolute() {
        let s = series(
            vec![("__name__", "foo")],
            vec![(0, 10.0), (1000, 5.0), (2000, 20.0)],
        );
        let r = apply_window_op(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
            WindowOp::Increase,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].values[0] - 35.0).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn irate_uses_last_two_samples() {
        let s = series(
            vec![("__name__", "foo")],
            vec![(0, 1.0), (5_000, 2.0), (10_000, 3.0)],
        );
        let r = apply_irate(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                // Last delta = 3.0 over 5s gap = 0.6/s.
                assert!((v[0].values[0] - 0.6).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn delta_returns_last_minus_first() {
        let s = series(
            vec![("__name__", "g")],
            vec![(0, 100.0), (10_000, 105.0), (20_000, 90.0)],
        );
        let r = apply_delta(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].values[0] - (-10.0)).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn histogram_quantile_interpolates_within_bucket() {
        // Buckets: le=1->1, le=2->4, le=5->10 cumulative.
        // total=10; phi=0.5 -> rank=5; falls in le=5 bucket.
        // Within bucket: lower=2 (count 4), upper=5 (count 10).
        // Fraction = (5-4)/(10-4) = 1/6.
        // Result = 2 + (5-2)*1/6 = 2.5.
        let buckets = vec![
            series(vec![("le", "1")], vec![(0, 1.0)]),
            series(vec![("le", "2")], vec![(0, 4.0)]),
            series(vec![("le", "5")], vec![(0, 10.0)]),
            series(vec![("le", "+Inf")], vec![(0, 10.0)]),
        ];
        let r = apply_histogram_quantile(vec![
            EvalResult::Scalar(0.5),
            EvalResult::InstantVector(buckets),
        ])
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert!(
                    (v[0].values[0] - 2.5).abs() < 1e-9,
                    "got {}",
                    v[0].values[0]
                );
            }
            _ => panic!(),
        }
    }

    #[test]
    fn histogram_quantile_without_le_fails() {
        let buckets = vec![series(vec![("__name__", "foo")], vec![(0, 1.0)])];
        let r = apply_histogram_quantile(vec![
            EvalResult::Scalar(0.5),
            EvalResult::InstantVector(buckets),
        ]);
        assert!(matches!(r, Err(EvalError::Other(_))));
    }

    #[test]
    fn histogram_quantile_groups_by_labels() {
        // Two histograms with different labels, three buckets each.
        // Each should produce its own quantile.
        let mut v = Vec::new();
        for tag in &["a", "b"] {
            for (le, c) in &[("1", 1.0), ("2", 2.0), ("+Inf", 2.0)] {
                v.push(series(
                    vec![("__name__", "h"), ("tag", tag), ("le", le)],
                    vec![(0, *c)],
                ));
            }
        }
        let r =
            apply_histogram_quantile(vec![EvalResult::Scalar(0.99), EvalResult::InstantVector(v)])
                .unwrap();
        match r {
            EvalResult::InstantVector(out) => {
                assert_eq!(out.len(), 2);
                // Each group's result carries the `tag` label but no `le` or
                // `__name__`.
                for s in &out {
                    assert!(s.labels.iter().any(|(n, _)| n == "tag"));
                    assert!(s.labels.iter().all(|(n, _)| n != "le"));
                    assert!(s.labels.iter().all(|(n, _)| n != "__name__"));
                }
            }
            _ => panic!(),
        }
    }

    #[test]
    fn rate_on_empty_drops_series() {
        let s = series(vec![("__name__", "foo")], vec![]);
        let r = apply_window_op(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
            WindowOp::Rate,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => assert_eq!(v.len(), 0),
            _ => panic!(),
        }
    }

    // -------------------------------------------------------------------
    // *_over_time family
    // -------------------------------------------------------------------

    fn run_over_time(op: OverTimeOp, samples: Vec<(i64, f64)>) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_over_time(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s])],
            None,
            op,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!("expected instant vector"),
        }
    }

    fn run_over_time_call(op: OverTimeOp, samples: Vec<(i64, f64)>) -> Vec<Series> {
        let func = match op {
            OverTimeOp::Avg => FuncKind::AvgOverTime,
            OverTimeOp::Sum => FuncKind::SumOverTime,
            OverTimeOp::Min => FuncKind::MinOverTime,
            OverTimeOp::Max => FuncKind::MaxOverTime,
            OverTimeOp::Count => FuncKind::CountOverTime,
            OverTimeOp::Last => FuncKind::LastOverTime,
            OverTimeOp::First => FuncKind::FirstOverTime,
            OverTimeOp::Present => FuncKind::PresentOverTime,
            OverTimeOp::Stddev => FuncKind::StddevOverTime,
            OverTimeOp::Stdvar => FuncKind::StdvarOverTime,
        };
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let ctx = EvalContext::default();
        let r = apply_call(&ctx, func, vec![EvalResult::RangeVector(vec![s])], None).unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        }
    }

    #[test]
    fn avg_over_time_computes_mean() {
        let v = run_over_time(OverTimeOp::Avg, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - 2.0).abs() < 1e-9);
        // __name__ stripped.
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
        // Timestamp is the most recent non-NaN sample's timestamp.
        assert_eq!(v[0].timestamps[0], 2000);
    }

    #[test]
    fn sum_over_time_computes_total() {
        let v = run_over_time(OverTimeOp::Sum, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - 6.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn min_max_over_time() {
        let v_min = run_over_time(OverTimeOp::Min, vec![(0, 5.0), (1000, 1.0), (2000, 3.0)]);
        assert_eq!(v_min[0].values[0], 1.0);
        let v_max = run_over_time(OverTimeOp::Max, vec![(0, 5.0), (1000, 1.0), (2000, 3.0)]);
        assert_eq!(v_max[0].values[0], 5.0);
    }

    #[test]
    fn count_over_time_counts_non_nan() {
        let v = run_over_time(
            OverTimeOp::Count,
            vec![(0, 1.0), (1000, f64::NAN), (2000, 3.0), (3000, 4.0)],
        );
        assert_eq!(v[0].values[0], 3.0);
    }

    #[test]
    fn last_over_time_preserves_name_and_returns_last_value() {
        let v = run_over_time(OverTimeOp::Last, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert_eq!(v[0].values[0], 3.0);
        // __name__ PRESERVED for last_over_time (Prometheus convention).
        assert!(
            v[0].labels
                .iter()
                .any(|(n, v)| n == "__name__" && v == "foo"),
            "labels = {:?}",
            v[0].labels
        );
    }

    #[test]
    fn last_over_time_skips_trailing_nan() {
        // The most recent NON-NaN sample wins; trailing NaN is ignored.
        let v = run_over_time(
            OverTimeOp::Last,
            vec![(0, 1.0), (1000, 2.0), (2000, f64::NAN)],
        );
        assert_eq!(v[0].values[0], 2.0);
        assert_eq!(v[0].timestamps[0], 1000);
    }

    #[test]
    fn present_over_time_returns_one() {
        let v = run_over_time(OverTimeOp::Present, vec![(0, 42.0), (1000, 100.0)]);
        assert_eq!(v[0].values[0], 1.0);
        // __name__ stripped.
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn over_time_nan_samples_skipped() {
        // NaNs do not contribute to sum/avg/min/max; count ignores them.
        let v = run_over_time(
            OverTimeOp::Avg,
            vec![(0, 1.0), (1000, f64::NAN), (2000, 3.0)],
        );
        assert!((v[0].values[0] - 2.0).abs() < 1e-9);
    }

    #[test]
    fn over_time_empty_window_drops_series() {
        // All NaN -> no observation -> series dropped (every variant).
        for op in [
            OverTimeOp::Avg,
            OverTimeOp::Sum,
            OverTimeOp::Min,
            OverTimeOp::Max,
            OverTimeOp::Count,
            OverTimeOp::Last,
            OverTimeOp::Present,
        ] {
            let v = run_over_time(op, vec![(0, f64::NAN), (1000, f64::NAN)]);
            assert!(v.is_empty(), "op {:?} should drop empty-window series", op);
        }
    }

    #[test]
    fn over_time_single_sample() {
        // A single sample is a valid window.
        for op in [
            OverTimeOp::Avg,
            OverTimeOp::Sum,
            OverTimeOp::Min,
            OverTimeOp::Max,
            OverTimeOp::Last,
        ] {
            let v = run_over_time(op, vec![(1000, 42.0)]);
            assert_eq!(v.len(), 1, "op {:?}", op);
            assert_eq!(v[0].values[0], 42.0, "op {:?}", op);
        }
        let v_count = run_over_time(OverTimeOp::Count, vec![(1000, 42.0)]);
        assert_eq!(v_count[0].values[0], 1.0);
    }

    #[test]
    fn over_time_dispatch_via_funckind() {
        // Verify the apply_call dispatch wires up every variant. Belt-and-
        // suspenders: each variant exercised through both apply_over_time
        // and apply_call so a refactor of the dispatch table doesn't
        // silently regress.
        let v = run_over_time_call(OverTimeOp::Avg, vec![(0, 2.0), (1000, 4.0)]);
        assert_eq!(v[0].values[0], 3.0);
        let v = run_over_time_call(OverTimeOp::Present, vec![(0, 1.0)]);
        assert_eq!(v[0].values[0], 1.0);
        let v = run_over_time_call(OverTimeOp::Last, vec![(0, 7.0)]);
        // last preserves __name__.
        assert!(v[0].labels.iter().any(|(n, _)| n == "__name__"));
    }

    #[test]
    fn over_time_requires_range_vector() {
        // Scalar input -> Type error.
        let r = apply_over_time(
            &EvalContext::default(),
            vec![EvalResult::Scalar(1.0)],
            None,
            OverTimeOp::Avg,
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
        // Instant vector input -> Type error.
        let s = series(vec![("__name__", "foo")], vec![(0, 1.0)]);
        let r = apply_over_time(
            &EvalContext::default(),
            vec![EvalResult::InstantVector(vec![s])],
            None,
            OverTimeOp::Avg,
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
    }

    // -------------------------------------------------------------------
    // stddev/stdvar/quantile_over_time, predict_linear,
    //          holt_winters
    // -------------------------------------------------------------------

    #[test]
    fn stdvar_over_time_computes_population_variance() {
        // [1,2,3,4,5] has mean 3, variance 2.
        let v = run_over_time(
            OverTimeOp::Stdvar,
            vec![(0, 1.0), (1, 2.0), (2, 3.0), (3, 4.0), (4, 5.0)],
        );
        assert_eq!(v.len(), 1);
        assert!(
            (v[0].values[0] - 2.0).abs() < 1e-9,
            "got {}",
            v[0].values[0]
        );
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn stddev_over_time_is_sqrt_of_stdvar() {
        let v = run_over_time(
            OverTimeOp::Stddev,
            vec![(0, 1.0), (1, 2.0), (2, 3.0), (3, 4.0), (4, 5.0)],
        );
        assert!((v[0].values[0] - 2.0_f64.sqrt()).abs() < 1e-9);
    }

    #[test]
    fn stddev_over_time_constant_series_is_zero() {
        let v = run_over_time(OverTimeOp::Stddev, vec![(0, 7.0), (1, 7.0), (2, 7.0)]);
        // Negative round-off clamped to zero in compute_over_time.
        assert!(v[0].values[0].abs() < 1e-9);
    }

    #[test]
    fn stddev_over_time_skips_nan() {
        let v = run_over_time(OverTimeOp::Stddev, vec![(0, 1.0), (1, f64::NAN), (2, 5.0)]);
        // mean = 3, var = (1+25)/2 - 9 = 4 -> stddev 2.
        assert!((v[0].values[0] - 2.0).abs() < 1e-9);
    }

    fn run_quantile_over_time(phi: f64, samples: Vec<(i64, f64)>) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_quantile_over_time(
            &EvalContext::default(),
            vec![EvalResult::Scalar(phi), EvalResult::RangeVector(vec![s])],
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        }
    }

    #[test]
    fn quantile_over_time_median() {
        let v = run_quantile_over_time(0.5, vec![(0, 1.0), (1, 2.0), (2, 3.0), (3, 4.0), (4, 5.0)]);
        assert!((v[0].values[0] - 3.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn quantile_over_time_max_at_one_min_at_zero() {
        let samples = vec![(0, 1.0), (1, 7.0), (2, 3.0)];
        let v1 = run_quantile_over_time(1.0, samples.clone());
        assert_eq!(v1[0].values[0], 7.0);
        let v0 = run_quantile_over_time(0.0, samples);
        assert_eq!(v0[0].values[0], 1.0);
    }

    #[test]
    fn quantile_over_time_clamps_phi() {
        let samples = vec![(0, 1.0), (1, 2.0)];
        let v_neg = run_quantile_over_time(-0.5, samples.clone());
        assert_eq!(v_neg[0].values[0], f64::NEG_INFINITY);
        let v_high = run_quantile_over_time(1.5, samples);
        assert_eq!(v_high[0].values[0], f64::INFINITY);
    }

    fn run_predict_linear(samples: Vec<(i64, f64)>, t_secs: f64) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_predict_linear(
            &EvalContext::default(),
            vec![EvalResult::RangeVector(vec![s]), EvalResult::Scalar(t_secs)],
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        }
    }

    #[test]
    fn predict_linear_on_perfect_line_extrapolates_exactly() {
        // y = 2x + 1 at t = 0, 1000, 2000, 3000 ms (so x = 0, 1, 2, 3
        // seconds). Last sample's x is the reference; t_secs=10 should
        // predict y at x = 3+10 = 13s -> 27 (since y = 2x + 1 anchored
        // at the reference: from last sample's perspective the line is
        // y_last + 2 * delta_seconds = 7 + 20 = 27).
        let v = run_predict_linear(vec![(0, 1.0), (1000, 3.0), (2000, 5.0), (3000, 7.0)], 10.0);
        assert_eq!(v.len(), 1);
        assert!(
            (v[0].values[0] - 27.0).abs() < 1e-9,
            "got {}",
            v[0].values[0]
        );
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn predict_linear_zero_t_returns_intercept_at_last_sample() {
        // With t=0 the prediction = the linear-fit value at the last
        // sample's reference time, which for a perfect line equals the
        // last sample's value.
        let v = run_predict_linear(vec![(0, 1.0), (1000, 3.0), (2000, 5.0)], 0.0);
        // The intercept at x=0 (the reference time = last sample's
        // time) is the predicted value. For y=2x+1 centered at the
        // last sample (x=2): y = 5 at x=2 -> in centered coordinates
        // intercept = 5.
        assert!(
            (v[0].values[0] - 5.0).abs() < 1e-9,
            "got {}",
            v[0].values[0]
        );
    }

    #[test]
    fn predict_linear_single_sample_drops() {
        let v = run_predict_linear(vec![(0, 42.0)], 10.0);
        assert!(v.is_empty());
    }

    fn run_holt_winters(samples: Vec<(i64, f64)>, sf: f64, tf: f64) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        apply_holt_winters(
            &EvalContext::default(),
            vec![
                EvalResult::RangeVector(vec![s]),
                EvalResult::Scalar(sf),
                EvalResult::Scalar(tf),
            ],
            None,
        )
        .map(|r| match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        })
        .unwrap()
    }

    #[test]
    fn holt_winters_two_samples_emits() {
        // With two samples, the recursion has just enough to produce
        // one smoothed output.
        let v = run_holt_winters(vec![(0, 10.0), (1, 20.0)], 0.5, 0.5);
        assert_eq!(v.len(), 1);
        // Just a sanity check on the value: with v0=10, v1=20:
        // s0=10, b0=10. After v1=20: s1=0.5*20+0.5*(10+10)=20.
        assert!((v[0].values[0] - 20.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn holt_winters_single_sample_drops() {
        let v = run_holt_winters(vec![(0, 10.0)], 0.5, 0.5);
        assert!(v.is_empty());
    }

    #[test]
    fn holt_winters_rejects_out_of_range_factors() {
        let s = series(vec![("__name__", "foo")], vec![(0, 1.0), (1, 2.0)]);
        let r = apply_holt_winters(
            &EvalContext::default(),
            vec![
                EvalResult::RangeVector(vec![s.clone()]),
                EvalResult::Scalar(1.5),
                EvalResult::Scalar(0.5),
            ],
            None,
        );
        assert!(matches!(r, Err(EvalError::Other(_))));

        let r = apply_holt_winters(
            &EvalContext::default(),
            vec![
                EvalResult::RangeVector(vec![s.clone()]),
                EvalResult::Scalar(0.5),
                EvalResult::Scalar(0.0),
            ],
            None,
        );
        assert!(matches!(r, Err(EvalError::Other(_))));
    }

    #[test]
    fn two_arg_funcs_reject_wrong_shapes() {
        // quantile_over_time wants (scalar, range); passing two scalars
        // is a type error.
        let r = apply_quantile_over_time(
            &EvalContext::default(),
            vec![EvalResult::Scalar(0.5), EvalResult::Scalar(1.0)],
            None,
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
        // predict_linear wants (range, scalar); passing (scalar, scalar)
        // errors on the range slot.
        let r = apply_predict_linear(
            &EvalContext::default(),
            vec![EvalResult::Scalar(0.5), EvalResult::Scalar(1.0)],
            None,
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
    }

    // Function unit tests --------------------------------------------------

    #[test]
    fn abs_strips_negatives() {
        let v = EvalResult::InstantVector(vec![
            series(vec![("__name__", "x"), ("dim", "a")], vec![(0, -5.0)]),
            series(vec![("__name__", "x"), ("dim", "b")], vec![(0, 3.0)]),
        ]);
        match apply_per_sample(vec![v], "abs", f64::abs).unwrap() {
            EvalResult::InstantVector(s) => {
                assert_eq!(s.len(), 2);
                assert!(s.iter().all(|x| x.values[0] >= 0.0));
                // __name__ stripped.
                assert!(
                    s.iter()
                        .all(|x| x.labels.iter().all(|(n, _)| n != "__name__"))
                );
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn sgn_handles_special_values() {
        assert_eq!(sgn(-3.0), -1.0);
        assert_eq!(sgn(0.0), 0.0);
        assert_eq!(sgn(7.5), 1.0);
        assert!(sgn(f64::NAN).is_nan());
    }

    #[test]
    fn clamp_bounds_in_both_directions() {
        let v = EvalResult::InstantVector(vec![
            series(vec![("__name__", "x"), ("dim", "a")], vec![(0, -10.0)]),
            series(vec![("__name__", "x"), ("dim", "b")], vec![(0, 7.0)]),
            series(vec![("__name__", "x"), ("dim", "c")], vec![(0, 50.0)]),
        ]);
        match apply_clamp(vec![v, EvalResult::Scalar(0.0), EvalResult::Scalar(10.0)]).unwrap() {
            EvalResult::InstantVector(s) => {
                assert_eq!(s[0].values[0].clamp(0.0, 10.0), s[0].values[0]);
                let vals: Vec<f64> = s.iter().map(|x| x.values[0]).collect();
                assert!(vals.iter().all(|&v| (0.0..=10.0).contains(&v)));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn round_nearest_default_is_one() {
        let v = EvalResult::InstantVector(vec![series(
            vec![("__name__", "x")],
            vec![(0, 2.4), (1000, 2.5), (2000, 2.6)],
        )]);
        match apply_round(vec![v]).unwrap() {
            EvalResult::InstantVector(s) => {
                let vals: Vec<f64> = s[0].values.iter().copied().collect();
                assert_eq!(vals, vec![2.0, 3.0, 3.0]);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn scalar_returns_nan_for_multi_series() {
        let v = EvalResult::InstantVector(vec![
            series(vec![("a", "1")], vec![(0, 1.0)]),
            series(vec![("a", "2")], vec![(0, 2.0)]),
        ]);
        match apply_scalar(vec![v]).unwrap() {
            EvalResult::Scalar(v) => assert!(v.is_nan()),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn scalar_returns_value_for_singleton() {
        let v = EvalResult::InstantVector(vec![series(vec![("a", "1")], vec![(0, 42.5)])]);
        match apply_scalar(vec![v]).unwrap() {
            EvalResult::Scalar(v) => assert_eq!(v, 42.5),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn sort_orders_ascending_nans_last() {
        let v = EvalResult::InstantVector(vec![
            series(vec![("a", "1")], vec![(0, 3.0)]),
            series(vec![("a", "2")], vec![(0, f64::NAN)]),
            series(vec![("a", "3")], vec![(0, 1.0)]),
        ]);
        match apply_sort(vec![v], false).unwrap() {
            EvalResult::InstantVector(s) => {
                let vals: Vec<f64> = s.iter().map(|x| x.values[0]).collect();
                assert_eq!(vals[0], 1.0);
                assert_eq!(vals[1], 3.0);
                assert!(vals[2].is_nan());
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn timestamp_returns_sample_ts_in_seconds() {
        let v = EvalResult::InstantVector(vec![series(
            vec![("__name__", "x")],
            vec![(12_345_000, 7.0)],
        )]);
        match apply_timestamp(vec![v]).unwrap() {
            EvalResult::InstantVector(s) => {
                assert_eq!(s[0].values[0], 12_345.0);
                assert!(s[0].labels.iter().all(|(n, _)| n != "__name__"));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn deriv_returns_slope_per_second() {
        // Linear increase of 1.0 per second over 5 seconds: slope = 1.
        let samples: Vec<(i64, f64)> = (0..5).map(|i| (i * 1000, i as f64)).collect();
        let v = EvalResult::RangeVector(vec![series(vec![("__name__", "x")], samples)]);
        match apply_deriv(&EvalContext::default(), vec![v], None).unwrap() {
            EvalResult::InstantVector(s) => {
                assert!((s[0].values[0] - 1.0).abs() < 1e-9);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn idelta_is_last_minus_previous() {
        let v = EvalResult::RangeVector(vec![series(
            vec![("__name__", "x")],
            vec![(0, 1.0), (1000, 5.0), (2000, 12.0)],
        )]);
        match apply_idelta(&EvalContext::default(), vec![v], None).unwrap() {
            EvalResult::InstantVector(s) => {
                assert_eq!(s[0].values[0], 7.0);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn changes_counts_transitions() {
        let v = EvalResult::RangeVector(vec![series(
            vec![("__name__", "x")],
            vec![(0, 1.0), (1000, 1.0), (2000, 2.0), (3000, 2.0), (4000, 3.0)],
        )]);
        match apply_changes(&EvalContext::default(), vec![v], None).unwrap() {
            EvalResult::InstantVector(s) => assert_eq!(s[0].values[0], 2.0),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn resets_counts_strict_decreases() {
        let v = EvalResult::RangeVector(vec![series(
            vec![("__name__", "x")],
            vec![(0, 5.0), (1000, 6.0), (2000, 1.0), (3000, 3.0), (4000, 0.0)],
        )]);
        match apply_resets(&EvalContext::default(), vec![v], None).unwrap() {
            EvalResult::InstantVector(s) => assert_eq!(s[0].values[0], 2.0),
            other => panic!("unexpected: {other:?}"),
        }
    }
}
