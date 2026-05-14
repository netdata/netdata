// SPDX-License-Identifier: GPL-3.0-or-later
//
// Operator fusion for the `aggr(rollup(selector))` shape. SOW-0033.
//
// The canonical Grafana panel query -- `sum by (X) (rate(metric[5m]))`
// or similar -- composes three operators that, in the unfused path,
// each materialise their own per-input-series buffers:
//
//   1. Matrix selector → RangeVector with N series (one per input).
//   2. rate(...) → InstantVector with N series × G grid points
//      (the intermediate; pure waste).
//   3. avg/sum/... → InstantVector with M output buckets, M ≪ N.
//
// Fusion collapses these three stages into one streaming pass. For
// each input series we two-pointer-walk its storage samples, compute
// the rollup value at each grid point, project the series' labels
// into a bucket key, and accumulate the rollup value directly into
// the per-bucket per-grid-point accumulator slot. The intermediate
// `Vec<Series>` for the rollup never materialises; memory is
// `O(M × G)` instead of `O(N × G)`.
//
// The trait `IncrementalAggr` encapsulates the per-bucket accumulator
// behaviour (sum, avg via sum+count, etc.). The enum `RollupKind`
// dispatches into the existing `compute_*` helpers in functions.rs --
// SOW-0032's column-shaped slice signature lets us call them directly.

use std::collections::BTreeMap;
use std::sync::Arc;

use crate::plan::{AggrKind, FuncKind, Grouping};
use crate::storage::Matcher;

use super::context::EvalContext;
use super::types::{labels_signature, EvalError, EvalResult, Series};

// ---------------------------------------------------------------------------
// IncrementalAggr trait + impls
// ---------------------------------------------------------------------------

/// Streaming per-bucket aggregator. One accumulator slot is created
/// per (output-bucket × grid-position) cell; values from the rollup
/// stream flow in via `accumulate`, and after all input series have
/// been consumed the slot is collapsed to a single f64 via
/// `finalize`. NaN inputs are skipped (matches the existing
/// non-streaming aggregator behaviour).
pub(crate) trait IncrementalAggr {
    type Acc: Default + Clone;

    fn accumulate(acc: &mut Self::Acc, value: f64);

    fn finalize(acc: &Self::Acc) -> f64;
}

#[derive(Clone, Default)]
pub(crate) struct SumAcc {
    sum: f64,
    count: u64,
}

pub(crate) struct SumAggr;
impl IncrementalAggr for SumAggr {
    type Acc = SumAcc;
    fn accumulate(acc: &mut Self::Acc, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.count += 1;
    }
    fn finalize(acc: &Self::Acc) -> f64 {
        if acc.count == 0 {
            f64::NAN
        } else {
            acc.sum
        }
    }
}

pub(crate) struct AvgAggr;
impl IncrementalAggr for AvgAggr {
    type Acc = SumAcc;
    fn accumulate(acc: &mut Self::Acc, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.sum += value;
        acc.count += 1;
    }
    fn finalize(acc: &Self::Acc) -> f64 {
        if acc.count == 0 {
            f64::NAN
        } else {
            acc.sum / (acc.count as f64)
        }
    }
}

#[derive(Clone)]
pub(crate) struct MinMaxAcc {
    value: f64,
    seen: bool,
}

impl Default for MinMaxAcc {
    fn default() -> Self {
        Self {
            value: 0.0,
            seen: false,
        }
    }
}

pub(crate) struct MinAggr;
impl IncrementalAggr for MinAggr {
    type Acc = MinMaxAcc;
    fn accumulate(acc: &mut Self::Acc, value: f64) {
        if value.is_nan() {
            return;
        }
        if !acc.seen || value < acc.value {
            acc.value = value;
            acc.seen = true;
        }
    }
    fn finalize(acc: &Self::Acc) -> f64 {
        if acc.seen {
            acc.value
        } else {
            f64::NAN
        }
    }
}

pub(crate) struct MaxAggr;
impl IncrementalAggr for MaxAggr {
    type Acc = MinMaxAcc;
    fn accumulate(acc: &mut Self::Acc, value: f64) {
        if value.is_nan() {
            return;
        }
        if !acc.seen || value > acc.value {
            acc.value = value;
            acc.seen = true;
        }
    }
    fn finalize(acc: &Self::Acc) -> f64 {
        if acc.seen {
            acc.value
        } else {
            f64::NAN
        }
    }
}

#[derive(Clone, Default)]
pub(crate) struct CountAcc {
    count: u64,
}

pub(crate) struct CountAggr;
impl IncrementalAggr for CountAggr {
    type Acc = CountAcc;
    fn accumulate(acc: &mut Self::Acc, value: f64) {
        if value.is_nan() {
            return;
        }
        acc.count += 1;
    }
    fn finalize(acc: &Self::Acc) -> f64 {
        // Distinct from sum/avg: a bucket with zero observations
        // still emits 0 (Prometheus `count` of zero observations is
        // an empty result; the upstream caller drops all-NaN
        // series, but a position that saw at least one bucket
        // input keeps a real 0).
        acc.count as f64
    }
}

// ---------------------------------------------------------------------------
// RollupKind: which compute_* helper to invoke for each (window) call
// ---------------------------------------------------------------------------

/// Subset of `FuncKind` that fuses cleanly with a streaming
/// aggregator. The rollups in this list compute a single f64 from a
/// window of samples without needing per-window state beyond what fits
/// in their `compute_*` helper.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RollupKind {
    Rate,
    Increase,
    Delta,
    IRate,
    AvgOverTime,
    SumOverTime,
    MinOverTime,
    MaxOverTime,
    CountOverTime,
    LastOverTime,
    PresentOverTime,
    StddevOverTime,
    StdvarOverTime,
}

impl RollupKind {
    /// Map a `FuncKind` to a fusable `RollupKind`. Returns `None` for
    /// functions that fuse poorly (predict_linear/holt_winters/
    /// quantile_over_time/deriv/idelta/changes/resets/
    /// histogram_quantile) -- those keep the unfused path.
    pub(crate) fn try_from_func(f: FuncKind) -> Option<Self> {
        Some(match f {
            FuncKind::Rate => RollupKind::Rate,
            FuncKind::Increase => RollupKind::Increase,
            FuncKind::Delta => RollupKind::Delta,
            FuncKind::IRate => RollupKind::IRate,
            FuncKind::AvgOverTime => RollupKind::AvgOverTime,
            FuncKind::SumOverTime => RollupKind::SumOverTime,
            FuncKind::MinOverTime => RollupKind::MinOverTime,
            FuncKind::MaxOverTime => RollupKind::MaxOverTime,
            FuncKind::CountOverTime => RollupKind::CountOverTime,
            FuncKind::LastOverTime => RollupKind::LastOverTime,
            FuncKind::PresentOverTime => RollupKind::PresentOverTime,
            FuncKind::StddevOverTime => RollupKind::StddevOverTime,
            FuncKind::StdvarOverTime => RollupKind::StdvarOverTime,
            _ => return None,
        })
    }

    /// True when the rollup strips `__name__` from its output. Matches
    /// the per-rollup convention in functions.rs. (Used for the
    /// fused-output label projection, though aggregation always drops
    /// `__name__` anyway, so this is currently informational.)
    pub(crate) fn strips_name(self) -> bool {
        !matches!(self, RollupKind::LastOverTime)
    }

    /// Compute the rollup value over one window. Threads through to
    /// the existing column-shaped helpers in functions.rs.
    pub(crate) fn compute(self, timestamps: &[i64], values: &[f64]) -> Option<f64> {
        match self {
            RollupKind::Rate => super::functions::compute_window_op(
                timestamps,
                values,
                super::functions::WindowOp::Rate,
            ),
            RollupKind::Increase => super::functions::compute_window_op(
                timestamps,
                values,
                super::functions::WindowOp::Increase,
            ),
            RollupKind::Delta => super::functions::compute_delta(values),
            RollupKind::IRate => super::functions::compute_irate(timestamps, values),
            RollupKind::AvgOverTime => {
                super::functions::compute_over_time::<super::functions::AvgReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::SumOverTime => {
                super::functions::compute_over_time::<super::functions::SumReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::MinOverTime => {
                super::functions::compute_over_time::<super::functions::MinReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::MaxOverTime => {
                super::functions::compute_over_time::<super::functions::MaxReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::CountOverTime => {
                super::functions::compute_over_time::<super::functions::CountReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::LastOverTime => {
                super::functions::compute_over_time::<super::functions::LastReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::PresentOverTime => {
                super::functions::compute_over_time::<super::functions::PresentReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::StddevOverTime => {
                super::functions::compute_over_time::<super::functions::StddevReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
            RollupKind::StdvarOverTime => {
                super::functions::compute_over_time::<super::functions::StdvarReducer>(
                    timestamps, values,
                )
                .map(|(v, _)| v)
            }
        }
    }
}

// ---------------------------------------------------------------------------
// FusableAggrKind: which aggregator dispatch into the fused trait
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum FusableAggrKind {
    Sum,
    Avg,
    Min,
    Max,
    Count,
}

impl FusableAggrKind {
    pub(crate) fn try_from_aggr(a: AggrKind) -> Option<Self> {
        Some(match a {
            AggrKind::Sum => FusableAggrKind::Sum,
            AggrKind::Avg => FusableAggrKind::Avg,
            AggrKind::Min => FusableAggrKind::Min,
            AggrKind::Max => FusableAggrKind::Max,
            AggrKind::Count => FusableAggrKind::Count,
            _ => return None,
        })
    }
}

// ---------------------------------------------------------------------------
// Label projection: shared between fused and unfused aggregators
// ---------------------------------------------------------------------------

/// Project a series's labels onto the grouping clause. Always drops
/// `__name__`; aggregation output never carries a metric name.
fn project_labels(
    labels: &[(String, String)],
    grouping: Option<&Grouping>,
) -> Vec<(String, String)> {
    match grouping {
        None => Vec::new(),
        Some(Grouping::By(names)) => labels
            .iter()
            .filter(|(n, _)| n != "__name__" && names.iter().any(|k| k == n))
            .cloned()
            .collect(),
        Some(Grouping::Without(names)) => labels
            .iter()
            .filter(|(n, _)| n != "__name__" && !names.iter().any(|k| k == n))
            .cloned()
            .collect(),
    }
}

// ---------------------------------------------------------------------------
// Fused evaluator entry point
// ---------------------------------------------------------------------------

/// Evaluate a fused `aggr(rollup(selector))` plan. The selector is
/// resolved against `ctx.backend` once; each input series is two-
/// pointer-windowed at the grid timestamps and its rollup value
/// flows directly into the per-bucket per-grid-point accumulator.
#[allow(clippy::too_many_arguments)]
pub fn eval_fused_aggr_rollup(
    ctx: &EvalContext,
    aggr: FusableAggrKind,
    grouping: Option<&Grouping>,
    rollup: RollupKind,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
    at: Option<&crate::plan::AtMod>,
) -> Result<EvalResult, EvalError> {
    match aggr {
        FusableAggrKind::Sum => {
            run_fused::<SumAggr>(ctx, grouping, rollup, matchers, range_ms, offset_ms, at)
        }
        FusableAggrKind::Avg => {
            run_fused::<AvgAggr>(ctx, grouping, rollup, matchers, range_ms, offset_ms, at)
        }
        FusableAggrKind::Min => {
            run_fused::<MinAggr>(ctx, grouping, rollup, matchers, range_ms, offset_ms, at)
        }
        FusableAggrKind::Max => {
            run_fused::<MaxAggr>(ctx, grouping, rollup, matchers, range_ms, offset_ms, at)
        }
        FusableAggrKind::Count => {
            run_fused::<CountAggr>(ctx, grouping, rollup, matchers, range_ms, offset_ms, at)
        }
    }
}

fn run_fused<A: IncrementalAggr>(
    ctx: &EvalContext,
    grouping: Option<&Grouping>,
    rollup: RollupKind,
    matchers: &[Matcher],
    range_ms: i64,
    offset_ms: i64,
    at: Option<&crate::plan::AtMod>,
) -> Result<EvalResult, EvalError> {
    let _ = rollup.strips_name(); // informational; aggregation always drops __name__
    let grid = &ctx.grid;
    let g_len = grid.len();
    let grid_timestamps = Arc::clone(&grid.timestamps);

    // Resolve the `@` modifier. `None` follows the grid timeline;
    // `Some(t)` pins every grid point to the resolved timestamp.
    let pinned = at.map(|a| match a {
        crate::plan::AtMod::AtTs(ms) => *ms,
        crate::plan::AtMod::Start => ctx.outer_start_ms,
        crate::plan::AtMod::End => ctx.outer_end_ms,
    });

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
            return Ok(EvalResult::InstantVector(Vec::new()));
        }
        Err(e) => return Err(EvalError::Storage(e)),
    };

    if q.was_truncated() {
        return Err(EvalError::Other(
            "exceeded max_series during resolution; tighten the query or raise the cap".into(),
        ));
    }

    // bucket signature -> (output labels, Vec<Acc> of length g_len).
    // BTreeMap keeps output ordering deterministic.
    let mut buckets: BTreeMap<u64, (Vec<(String, String)>, Vec<A::Acc>)> = BTreeMap::new();

    // The pinned grid timestamps for windowing: when `@`-pinned, every
    // grid point's window has the same right edge `(pinned - offset)`;
    // otherwise grid timestamps follow the grid.
    let window_anchors: Vec<i64> = match pinned {
        Some(t) => vec![t.saturating_sub(offset_ms); g_len],
        None => grid
            .timestamps
            .iter()
            .map(|&t| t.saturating_sub(offset_ms))
            .collect(),
    };

    for i in 0..q.len() {
        let Some(meta) = q.series_meta(i) else { continue };
        let Some(samples_iter) = q.open_samples(i, after_s, before_s, 0) else {
            continue;
        };

        // Materialise the series' raw samples into column form.
        // (NaN-filtered; mirrors eval_matrix_select.)
        let mut series_ts: Vec<i64> = Vec::new();
        let mut series_vals: Vec<f64> = Vec::new();
        for s in samples_iter {
            if s.value.is_nan() {
                continue;
            }
            if s.timestamp_ms < fetch_after_ms || s.timestamp_ms > fetch_before_ms {
                continue;
            }
            series_ts.push(s.timestamp_ms);
            series_vals.push(s.value);
        }
        if series_vals.is_empty() {
            continue;
        }

        // Bucket key from the projected labels.
        let bucket_labels = project_labels(&meta.labels, grouping);
        let bucket_sig = labels_signature(&bucket_labels);

        // Get-or-init the bucket's per-grid-point accumulator vector.
        let entry = buckets
            .entry(bucket_sig)
            .or_insert_with(|| (bucket_labels, vec![A::Acc::default(); g_len]));

        // Two-pointer windowing: for each grid anchor, find samples in
        // (anchor - range_ms, anchor], compute the rollup value, push
        // into the bucket's accumulator at this grid position.
        let mut left: usize = 0;
        let mut right: usize = 0;
        for (g_idx, &anchor) in window_anchors.iter().enumerate() {
            let lower = anchor.saturating_sub(range_ms);
            while right < series_ts.len() && series_ts[right] <= anchor {
                right += 1;
            }
            while left < right && series_ts[left] <= lower {
                left += 1;
            }
            let window_ts = &series_ts[left..right];
            let window_vals = &series_vals[left..right];
            if let Some(v) = rollup.compute(window_ts, window_vals) {
                A::accumulate(&mut entry.1[g_idx], v);
            }
        }
    }

    // Finalize: per bucket, walk the g_len accumulators and produce
    // a grid-aligned `Vec<f64>`. Drop buckets whose every output
    // value is NaN.
    let mut out: Vec<Series> = buckets
        .into_iter()
        .filter_map(|(_, (labels, accs))| {
            let values: Vec<f64> = accs.iter().map(|a| A::finalize(a)).collect();
            if values.iter().all(|v| v.is_nan()) {
                return None;
            }
            Some(Series::new(labels, Arc::clone(&grid_timestamps), values))
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

#[cfg(test)]
mod tests {
    use super::*;

    // ---- IncrementalAggr trait impls ----

    #[test]
    fn sum_aggr_ignores_nan_returns_total() {
        let mut acc = <SumAggr as IncrementalAggr>::Acc::default();
        SumAggr::accumulate(&mut acc, 1.0);
        SumAggr::accumulate(&mut acc, f64::NAN);
        SumAggr::accumulate(&mut acc, 3.0);
        assert_eq!(SumAggr::finalize(&acc), 4.0);
    }

    #[test]
    fn sum_aggr_empty_yields_nan() {
        let acc = <SumAggr as IncrementalAggr>::Acc::default();
        assert!(SumAggr::finalize(&acc).is_nan());
    }

    #[test]
    fn avg_aggr_divides_by_count() {
        let mut acc = <AvgAggr as IncrementalAggr>::Acc::default();
        AvgAggr::accumulate(&mut acc, 2.0);
        AvgAggr::accumulate(&mut acc, 4.0);
        AvgAggr::accumulate(&mut acc, 6.0);
        assert_eq!(AvgAggr::finalize(&acc), 4.0);
    }

    #[test]
    fn min_max_skip_nan_track_extreme() {
        let mut min_acc = <MinAggr as IncrementalAggr>::Acc::default();
        MinAggr::accumulate(&mut min_acc, 5.0);
        MinAggr::accumulate(&mut min_acc, f64::NAN);
        MinAggr::accumulate(&mut min_acc, 1.0);
        MinAggr::accumulate(&mut min_acc, 3.0);
        assert_eq!(MinAggr::finalize(&min_acc), 1.0);

        let mut max_acc = <MaxAggr as IncrementalAggr>::Acc::default();
        MaxAggr::accumulate(&mut max_acc, 5.0);
        MaxAggr::accumulate(&mut max_acc, 1.0);
        MaxAggr::accumulate(&mut max_acc, 10.0);
        MaxAggr::accumulate(&mut max_acc, f64::NAN);
        assert_eq!(MaxAggr::finalize(&max_acc), 10.0);
    }

    #[test]
    fn count_aggr_counts_non_nan() {
        let mut acc = <CountAggr as IncrementalAggr>::Acc::default();
        CountAggr::accumulate(&mut acc, 1.0);
        CountAggr::accumulate(&mut acc, f64::NAN);
        CountAggr::accumulate(&mut acc, 5.0);
        CountAggr::accumulate(&mut acc, 7.0);
        assert_eq!(CountAggr::finalize(&acc), 3.0);
    }

    // ---- End-to-end fused equivalence against the unfused path ----

    use crate::storage::{MemBackend, MemSeries, Sample as StorageSample};

    fn mk_backend_two_series() -> Arc<MemBackend> {
        // Two series: instance="a" and instance="b". Both with three
        // samples 1s apart. Identical samples => rate is identical;
        // sum(rate(...)) doubles each rate sample; avg keeps it the
        // same.
        let backend = Arc::new(MemBackend::new());
        let labels_a = vec![
            ("__name__".to_string(), "metric".to_string()),
            ("instance".to_string(), "a".to_string()),
        ];
        let labels_b = vec![
            ("__name__".to_string(), "metric".to_string()),
            ("instance".to_string(), "b".to_string()),
        ];
        let samples = vec![
            StorageSample {
                timestamp_ms: 0,
                value: 10.0,
                flags: 0,
            },
            StorageSample {
                timestamp_ms: 1000,
                value: 10.0,
                flags: 0,
            },
            StorageSample {
                timestamp_ms: 2000,
                value: 10.0,
                flags: 0,
            },
        ];
        backend.add_series(MemSeries {
            labels: labels_a,
            samples: samples.clone(),
        });
        backend.add_series(MemSeries {
            labels: labels_b,
            samples,
        });
        backend
    }

    fn ctx_at(backend: Arc<MemBackend>, at_ms: i64) -> EvalContext {
        EvalContext {
            grid: Arc::new(crate::eval::Grid::instant(at_ms)),
            outer_start_ms: at_ms,
            outer_end_ms: at_ms,
            backend,
            ..EvalContext::default()
        }
    }

    #[test]
    fn sum_rate_fuses_to_double_rate_per_series() {
        // Window (0, 2000] picks up samples at 1000 and 2000 (not at
        // 0). Two samples of value 10 over a 1s span -> rate = 20/s
        // per series. Sum across two series = 40/s.
        let backend = mk_backend_two_series();
        let ctx = ctx_at(backend, 2000);
        let matchers = vec![Matcher::eq("__name__", "metric")];
        let r = eval_fused_aggr_rollup(
            &ctx,
            FusableAggrKind::Sum,
            None,
            RollupKind::Rate,
            &matchers,
            2000,
            0,
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert!((v[0].values[0] - 40.0).abs() < 1e-9, "got {}", v[0].values[0]);
            }
            _ => panic!("expected instant vector"),
        }
    }

    #[test]
    fn avg_sum_over_time_fuses_correctly() {
        // sum_over_time over (0, 2000] sums the two samples in window
        // (at 1000 and 2000) = 20 per series. avg across two series = 20.
        let backend = mk_backend_two_series();
        let ctx = ctx_at(backend, 2000);
        let matchers = vec![Matcher::eq("__name__", "metric")];
        let r = eval_fused_aggr_rollup(
            &ctx,
            FusableAggrKind::Avg,
            None,
            RollupKind::SumOverTime,
            &matchers,
            2000,
            0,
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert!((v[0].values[0] - 20.0).abs() < 1e-9, "got {}", v[0].values[0]);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn fused_by_instance_keeps_two_buckets() {
        // sum by (instance) (rate(metric[2s])) => one bucket per instance,
        // each bucket carries that instance's rate = 20/s.
        let backend = mk_backend_two_series();
        let ctx = ctx_at(backend, 2000);
        let matchers = vec![Matcher::eq("__name__", "metric")];
        let grouping = Grouping::By(vec!["instance".to_string()]);
        let r = eval_fused_aggr_rollup(
            &ctx,
            FusableAggrKind::Sum,
            Some(&grouping),
            RollupKind::Rate,
            &matchers,
            2000,
            0,
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 2);
                for s in &v {
                    assert!(s.labels.iter().any(|(n, _)| n == "instance"));
                    assert!(s.labels.iter().all(|(n, _)| n != "__name__"));
                    assert!((s.values[0] - 20.0).abs() < 1e-9, "got {}", s.values[0]);
                }
            }
            _ => panic!(),
        }
    }

    #[test]
    fn count_aggr_over_three_series_returns_three() {
        // count(rate(metric[2s])) over two series should return 2.
        let backend = mk_backend_two_series();
        let ctx = ctx_at(backend, 2000);
        let matchers = vec![Matcher::eq("__name__", "metric")];
        let r = eval_fused_aggr_rollup(
            &ctx,
            FusableAggrKind::Count,
            None,
            RollupKind::Rate,
            &matchers,
            2000,
            0,
            None,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert_eq!(v[0].values[0], 2.0);
            }
            _ => panic!(),
        }
    }

    /// Equivalence test: the fused path must produce bit-identical
    /// output to the unfused `Plan::Aggregate{expr: Plan::Call{...}}`
    /// path for the same query against the same data. SOW-0033's
    /// primary correctness invariant.
    #[test]
    fn fused_matches_unfused_for_sum_rate() {
        use crate::eval::eval;
        use crate::plan::{AggrKind, FuncKind, Plan};
        use std::sync::Arc;

        let backend = mk_backend_two_series();
        let ctx = ctx_at(Arc::clone(&backend), 2000);

        // Build the unfused plan by hand: Plan::Aggregate{
        //   op: Sum, grouping: None,
        //   expr: Plan::Call{ Rate, [Plan::MatrixSelect{...}] }
        // }
        let matchers = vec![Matcher::eq("__name__", "metric")];
        let matrix = Plan::MatrixSelect {
            matchers: matchers.clone(),
            range_ms: 2000,
            offset_ms: 0,
            at: None,
        };
        let rate_call = Plan::Call {
            func: FuncKind::Rate,
            args: vec![matrix],
        };
        let unfused = Plan::Aggregate {
            op: AggrKind::Sum,
            grouping: None,
            param: None,
            param_string: None,
            expr: Arc::new(rate_call),
        };

        let unfused_r = eval(&ctx, &unfused).unwrap();
        let fused_r = eval_fused_aggr_rollup(
            &ctx,
            FusableAggrKind::Sum,
            None,
            RollupKind::Rate,
            &matchers,
            2000,
            0,
            None,
        )
        .unwrap();

        // Compare shape + per-position values.
        match (unfused_r, fused_r) {
            (EvalResult::InstantVector(u), EvalResult::InstantVector(f)) => {
                assert_eq!(u.len(), f.len(), "series count differs");
                for (us, fs) in u.iter().zip(f.iter()) {
                    assert_eq!(us.labels, fs.labels, "labels differ");
                    assert_eq!(us.values.len(), fs.values.len(), "values length differs");
                    for (uv, fv) in us.values.iter().zip(fs.values.iter()) {
                        match (uv.is_nan(), fv.is_nan()) {
                            (true, true) => continue,
                            (false, false) => assert!(
                                (uv - fv).abs() < 1e-9,
                                "value differs: unfused {} vs fused {}",
                                uv,
                                fv
                            ),
                            _ => panic!(
                                "NaN-mismatch: unfused {} vs fused {}",
                                uv, fv
                            ),
                        }
                    }
                }
            }
            _ => panic!("expected InstantVector on both sides"),
        }
    }
}
