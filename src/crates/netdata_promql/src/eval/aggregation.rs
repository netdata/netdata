// SPDX-License-Identifier: GPL-3.0-or-later
//
// Aggregations: sum, avg, min, max, count, with optional by/without grouping.
//
// Phase 1 scope: instant-vector inputs only. Each input series produces a
// single output sample; the grouping clause determines which output series
// the sample lands in.

use std::collections::BTreeMap;

use crate::plan::{AggrKind, Grouping, ValueType};

use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

/// Apply an aggregation operator to an instant vector.
///
/// `param` carries the scalar argument for parametrized aggregators
/// (topk/bottomk/quantile). It must be `Some` for those and `None` for
/// the others; the lowering layer enforces this.
pub fn apply_aggregate(
    op: AggrKind,
    grouping: Option<&Grouping>,
    param: Option<f64>,
    inner: EvalResult,
) -> Result<EvalResult, EvalError> {
    let series = match inner {
        EvalResult::InstantVector(v) => v,
        other => {
            return Err(EvalError::Type {
                context: "aggregation operand",
                expected: ValueType::InstantVector,
                got: other.value_type(),
            })
        }
    };

    match op {
        AggrKind::TopK | AggrKind::BottomK => {
            let k = param.expect("lowering guarantees k for topk/bottomk");
            apply_topk_bottomk(op, grouping, k, series)
        }
        AggrKind::Quantile => {
            let phi = param.expect("lowering guarantees phi for quantile");
            apply_quantile(grouping, phi, series)
        }
        _ => apply_collapsing(op, grouping, series),
    }
}

/// The original Phase 1 path for sum/avg/min/max/count: bucket by
/// grouping key, accumulate, collapse each bucket to a single series.
fn apply_collapsing(
    op: AggrKind,
    grouping: Option<&Grouping>,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    // Bucket series by grouping key. The key is the projection of each
    // series's labels onto (or away from) the named labels in the clause.
    // `BTreeMap` keyed on signature gives stable iteration order.
    let mut buckets: BTreeMap<u64, Bucket> = BTreeMap::new();

    for s in series.into_iter() {
        let key_labels = project_labels(&s.labels, grouping);
        let key = labels_signature(&key_labels);
        // Aggregations operate on the first sample per input series. PromQL
        // instant-vector inputs to an aggregation are guaranteed to have
        // exactly one sample per series at the query time.
        let v = s.samples.first().map(|s| s.value).unwrap_or(f64::NAN);
        let ts = s.samples.first().map(|s| s.timestamp_ms).unwrap_or(0);
        buckets.entry(key).or_insert_with(|| Bucket::new(key_labels.clone(), ts)).accumulate(v);
    }

    let mut out: Vec<Series> = buckets
        .into_iter()
        .map(|(_, b)| b.finalize(op))
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// topk(k, expr) / bottomk(k, expr): bucket by grouping, sort each
/// bucket's series by value, keep top or bottom k. The output preserves
/// the **original** input series labels minus `__name__` -- distinct
/// from sum/avg, which collapse to grouping labels only.
fn apply_topk_bottomk(
    op: AggrKind,
    grouping: Option<&Grouping>,
    k_param: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if !k_param.is_finite() || k_param <= 0.0 {
        // Negative, zero, or NaN k: Prometheus emits no series.
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let k = k_param as usize;
    let descending = matches!(op, AggrKind::TopK);

    let mut by_bucket: BTreeMap<u64, Vec<Series>> = BTreeMap::new();
    for s in series.into_iter() {
        let key_labels = project_labels(&s.labels, grouping);
        let key = labels_signature(&key_labels);
        by_bucket.entry(key).or_default().push(s);
    }

    let mut out: Vec<Series> = Vec::new();
    for (_, mut bucket) in by_bucket.into_iter() {
        // Sort by sample value. NaN comparisons fall last with
        // `total_cmp`, which keeps NaN-only entries from masking real
        // candidates at the top.
        bucket.sort_by(|a, b| {
            let av = a.samples.first().map(|s| s.value).unwrap_or(f64::NAN);
            let bv = b.samples.first().map(|s| s.value).unwrap_or(f64::NAN);
            if descending {
                bv.total_cmp(&av)
            } else {
                av.total_cmp(&bv)
            }
        });
        // Drop any NaN-valued series from the kept slice (Prometheus
        // skips them).
        bucket.retain(|s| {
            s.samples
                .first()
                .map(|sm| !sm.value.is_nan())
                .unwrap_or(false)
        });
        bucket.truncate(k);
        for s in bucket.into_iter() {
            // Output preserves original labels minus __name__.
            let labels: Vec<(String, String)> = s
                .labels
                .into_iter()
                .filter(|(n, _)| n != "__name__")
                .collect();
            let signature = labels_signature(&labels);
            out.push(Series {
                labels,
                signature,
                samples: s.samples,
            });
        }
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// quantile(phi, expr): per group, compute the phi-quantile via linear
/// interpolation between adjacent ranked observations. Phi outside
/// [0, 1] clamps to ±Inf to match Prometheus.
fn apply_quantile(
    grouping: Option<&Grouping>,
    phi: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    let mut by_bucket: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>, i64)> = BTreeMap::new();
    for s in series.into_iter() {
        let key_labels = project_labels(&s.labels, grouping);
        let key = labels_signature(&key_labels);
        let v = s.samples.first().map(|s| s.value).unwrap_or(f64::NAN);
        let ts = s.samples.first().map(|s| s.timestamp_ms).unwrap_or(0);
        let entry = by_bucket
            .entry(key)
            .or_insert_with(|| (key_labels.clone(), Vec::new(), ts));
        if !v.is_nan() {
            entry.1.push(v);
        }
    }

    let mut out: Vec<Series> = Vec::new();
    for (_, (labels, mut values, ts)) in by_bucket.into_iter() {
        if values.is_empty() {
            continue;
        }
        // Sort ascending for the standard rank-based interpolation.
        values.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
        let value = compute_quantile(&values, phi);
        let signature = labels_signature(&labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: ts,
                value,
            }],
        });
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Standard linear-interpolation quantile over a pre-sorted slice.
/// Matches Prometheus' `quantile` aggregator: rank = phi * (n - 1),
/// lerp between floor(rank) and ceil(rank).
fn compute_quantile(sorted: &[f64], phi: f64) -> f64 {
    if phi.is_nan() {
        return f64::NAN;
    }
    if phi < 0.0 {
        return f64::NEG_INFINITY;
    }
    if phi > 1.0 {
        return f64::INFINITY;
    }
    let n = sorted.len();
    if n == 0 {
        return f64::NAN;
    }
    if n == 1 {
        return sorted[0];
    }
    let rank = phi * (n - 1) as f64;
    let lower = rank.floor() as usize;
    let upper = rank.ceil() as usize;
    if lower == upper {
        return sorted[lower];
    }
    let frac = rank - lower as f64;
    sorted[lower] + (sorted[upper] - sorted[lower]) * frac
}

struct Bucket {
    labels: Vec<(String, String)>,
    ts: i64,
    sum: f64,
    count: u64,
    min: f64,
    max: f64,
}

impl Bucket {
    fn new(labels: Vec<(String, String)>, ts: i64) -> Self {
        Self {
            labels,
            ts,
            sum: 0.0,
            count: 0,
            min: f64::INFINITY,
            max: f64::NEG_INFINITY,
        }
    }

    fn accumulate(&mut self, v: f64) {
        if v.is_nan() {
            return;
        }
        self.sum += v;
        self.count += 1;
        if v < self.min {
            self.min = v;
        }
        if v > self.max {
            self.max = v;
        }
    }

    fn finalize(self, op: AggrKind) -> Series {
        let value = if self.count == 0 {
            f64::NAN
        } else {
            match op {
                AggrKind::Sum => self.sum,
                AggrKind::Avg => self.sum / self.count as f64,
                AggrKind::Min => self.min,
                AggrKind::Max => self.max,
                AggrKind::Count => self.count as f64,
                // Parametrized aggregators are routed by `apply_aggregate`
                // before they reach this path; reaching here means a bug
                // in the dispatch.
                AggrKind::TopK | AggrKind::BottomK | AggrKind::Quantile => {
                    unreachable!("parametrized aggregator in Bucket::finalize: {:?}", op)
                }
            }
        };
        let signature = labels_signature(&self.labels);
        Series {
            labels: self.labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: self.ts,
                value,
            }],
        }
    }
}

/// Project a series's labels onto (or away from) the grouping clause's
/// label list. Always drops `__name__` -- aggregation output never carries
/// a metric name.
fn project_labels(labels: &[(String, String)], grouping: Option<&Grouping>) -> Vec<(String, String)> {
    let kept: Vec<(String, String)> = match grouping {
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
    };
    kept
}

#[cfg(test)]
mod tests {
    use super::*;

    fn s(name: &str, dim: &str, val: f64) -> Series {
        let labels = vec![
            ("__name__".to_string(), name.to_string()),
            ("dimension".to_string(), dim.to_string()),
        ];
        finalize_series(labels, val)
    }

    fn s_with(group: &str, dim: &str, val: f64) -> Series {
        // Helper for grouping tests: carries an explicit `group` label so
        // we can exercise `by (group)` without colliding with the
        // __name__-dropping convention that aggregation output follows.
        let labels = vec![
            ("__name__".to_string(), "metric".to_string()),
            ("group".to_string(), group.to_string()),
            ("dimension".to_string(), dim.to_string()),
        ];
        finalize_series(labels, val)
    }

    fn finalize_series(labels: Vec<(String, String)>, val: f64) -> Series {
        let signature = labels_signature(&labels);
        Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: 1000,
                value: val,
            }],
        }
    }

    #[test]
    fn sum_without_grouping_collapses_to_one() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let r = apply_aggregate(AggrKind::Sum, None, None, input).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert!((v[0].samples[0].value - 6.0).abs() < 1e-9);
                assert!(v[0].labels.is_empty());
            }
            _ => panic!(),
        }
    }

    #[test]
    fn sum_by_dimension_keeps_one_per_dimension() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "a", 10.0),
            s("foo", "b", 2.0),
        ]);
        let r = apply_aggregate(
            AggrKind::Sum,
            Some(&Grouping::By(vec!["dimension".to_string()])),
            None,
            input,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 2);
                let mut values: Vec<f64> = v.iter().map(|s| s.samples[0].value).collect();
                values.sort_by(|a, b| a.partial_cmp(b).unwrap());
                assert!((values[0] - 2.0).abs() < 1e-9);
                assert!((values[1] - 11.0).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn count_yields_integer_count() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let r = apply_aggregate(AggrKind::Count, None, None, input).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].samples[0].value - 3.0).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn avg_min_max() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 5.0),
            s("foo", "c", 3.0),
        ]);
        let avg = apply_aggregate(AggrKind::Avg, None, None, input.clone()).unwrap();
        let min = apply_aggregate(AggrKind::Min, None, None, input.clone()).unwrap();
        let max = apply_aggregate(AggrKind::Max, None, None, input).unwrap();
        if let (EvalResult::InstantVector(a), EvalResult::InstantVector(b), EvalResult::InstantVector(c)) =
            (avg, min, max)
        {
            assert!((a[0].samples[0].value - 3.0).abs() < 1e-9);
            assert!((b[0].samples[0].value - 1.0).abs() < 1e-9);
            assert!((c[0].samples[0].value - 5.0).abs() < 1e-9);
        } else {
            panic!();
        }
    }

    // -------------------------------------------------------------------
    // Phase 3c: topk / bottomk / quantile (SOW-0021)
    // -------------------------------------------------------------------

    fn unwrap_vec(r: EvalResult) -> Vec<Series> {
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!("expected instant vector"),
        }
    }

    fn values_sorted(v: &[Series]) -> Vec<f64> {
        let mut out: Vec<f64> = v.iter().map(|s| s.samples[0].value).collect();
        out.sort_by(|a, b| a.partial_cmp(b).unwrap());
        out
    }

    #[test]
    fn topk_returns_top_k() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 4.0),
            s("foo", "c", 3.0),
            s("foo", "d", 2.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(2.0), input).unwrap());
        assert_eq!(v.len(), 2);
        assert_eq!(values_sorted(&v), vec![3.0, 4.0]);
        // __name__ stripped; `dimension` preserved (original labels).
        for s in &v {
            assert!(s.labels.iter().all(|(n, _)| n != "__name__"));
            assert!(s.labels.iter().any(|(n, _)| n == "dimension"));
        }
    }

    #[test]
    fn bottomk_returns_bottom_k() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 4.0),
            s("foo", "c", 3.0),
            s("foo", "d", 2.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::BottomK, None, Some(2.0), input).unwrap());
        assert_eq!(v.len(), 2);
        assert_eq!(values_sorted(&v), vec![1.0, 2.0]);
    }

    #[test]
    fn topk_zero_returns_empty() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 1.0)]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(0.0), input.clone()).unwrap());
        assert!(v.is_empty());
        // Negative or NaN k -> empty.
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(-1.0), input.clone()).unwrap());
        assert!(v.is_empty());
        let v = unwrap_vec(apply_aggregate(AggrKind::BottomK, None, Some(f64::NAN), input).unwrap());
        assert!(v.is_empty());
    }

    #[test]
    fn topk_skips_nan_values() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", f64::NAN),
            s("foo", "c", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(3.0), input).unwrap());
        // Only two non-NaN candidates; NaN dropped.
        assert_eq!(v.len(), 2);
    }

    #[test]
    fn topk_with_grouping_per_bucket() {
        // Two buckets by `group` -> "g1" and "g2". topk(1) per bucket
        // should give one series per bucket. We use a custom `group`
        // label rather than `__name__` because `project_labels`
        // deliberately strips `__name__` from grouping keys (aggregation
        // outputs don't carry a metric name; cumulative Prometheus
        // convention).
        let input = EvalResult::InstantVector(vec![
            s_with("g1", "a", 1.0),
            s_with("g1", "b", 2.0),
            s_with("g2", "a", 10.0),
            s_with("g2", "b", 20.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::TopK,
                Some(&Grouping::By(vec!["group".to_string()])),
                Some(1.0),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        let values = values_sorted(&v);
        assert_eq!(values, vec![2.0, 20.0]);
    }

    #[test]
    fn quantile_median() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
            s("foo", "e", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.5), input).unwrap());
        assert_eq!(v.len(), 1);
        assert!((v[0].samples[0].value - 3.0).abs() < 1e-9);
    }

    #[test]
    fn quantile_zero_returns_min_one_returns_max() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 7.0),
            s("foo", "c", 3.0),
        ]);
        let v0 = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.0), input.clone()).unwrap());
        assert_eq!(v0[0].samples[0].value, 1.0);
        let v1 = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(1.0), input).unwrap());
        assert_eq!(v1[0].samples[0].value, 7.0);
    }

    #[test]
    fn quantile_interpolates_between_samples() {
        // rank = 0.95 * (5 - 1) = 3.8, lerp(s[3], s[4], 0.8)
        // sorted: [1, 2, 3, 4, 5], so lerp(4, 5, 0.8) = 4.8.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
            s("foo", "e", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.95), input).unwrap());
        assert!((v[0].samples[0].value - 4.8).abs() < 1e-9, "got {}", v[0].samples[0].value);
    }

    #[test]
    fn quantile_out_of_range_clamps_to_infinity() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 1.0), s("foo", "b", 2.0)]);
        let v_neg = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(-0.5), input.clone()).unwrap());
        assert_eq!(v_neg[0].samples[0].value, f64::NEG_INFINITY);
        let v_high = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(2.0), input).unwrap());
        assert_eq!(v_high[0].samples[0].value, f64::INFINITY);
    }

    #[test]
    fn quantile_with_grouping() {
        let input = EvalResult::InstantVector(vec![
            s_with("g1", "a", 1.0),
            s_with("g1", "b", 3.0),
            s_with("g2", "a", 10.0),
            s_with("g2", "b", 30.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Quantile,
                Some(&Grouping::By(vec!["group".to_string()])),
                Some(0.5),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        let values = values_sorted(&v);
        assert_eq!(values, vec![2.0, 20.0]);
    }

    #[test]
    fn quantile_empty_bucket_drops() {
        // All NaN -> no observation -> bucket drops.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", f64::NAN),
            s("foo", "b", f64::NAN),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.5), input).unwrap());
        assert!(v.is_empty());
    }

    #[test]
    fn topk_with_non_integer_k_truncates() {
        // k=2.9 should behave as k=2 (Prometheus floors via `as usize`).
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(2.9), input).unwrap());
        assert_eq!(v.len(), 2);
    }
}
