// SPDX-License-Identifier: GPL-3.0-or-later
//
// Counter-family functions (rate, irate, increase, delta) and
// `histogram_quantile`.
//
// Counter semantics per SOW-0017 Implications #2: rate/irate/increase
// work directly on the deltas Netdata's storage already produced. We do
// not reconstruct a cumulative counter value, and we do not infer resets
// from sample-to-sample decreases -- if the storage flagged a reset, the
// delta at that bucket already reflects it.

use std::collections::BTreeMap;

use crate::plan::FuncKind;

use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

pub fn apply_call(func: FuncKind, args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    match func {
        FuncKind::Rate => apply_window_op(args, WindowOp::Rate),
        FuncKind::Increase => apply_window_op(args, WindowOp::Increase),
        FuncKind::IRate => apply_irate(args),
        FuncKind::Delta => apply_delta(args),
        FuncKind::HistogramQuantile => apply_histogram_quantile(args),
        FuncKind::AvgOverTime => apply_over_time(args, OverTimeOp::Avg),
        FuncKind::SumOverTime => apply_over_time(args, OverTimeOp::Sum),
        FuncKind::MinOverTime => apply_over_time(args, OverTimeOp::Min),
        FuncKind::MaxOverTime => apply_over_time(args, OverTimeOp::Max),
        FuncKind::CountOverTime => apply_over_time(args, OverTimeOp::Count),
        FuncKind::LastOverTime => apply_over_time(args, OverTimeOp::Last),
        FuncKind::PresentOverTime => apply_over_time(args, OverTimeOp::Present),
    }
}

/// Windowed aggregator selector. The variants correspond to the seven
/// `*_over_time` functions in Phase 3b (SOW-0020).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum OverTimeOp {
    Avg,
    Sum,
    Min,
    Max,
    Count,
    Last,
    Present,
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
            OverTimeOp::Present => "present_over_time",
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
        matches!(self, OverTimeOp::Last)
    }
}

fn apply_over_time(args: Vec<EvalResult>, op: OverTimeOp) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, op.name())?;

    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some((value, ts_ms)) = compute_over_time(&s.samples, op) else {
            // Empty window (no non-NaN samples) -> drop the series. This
            // matches Prometheus' "no observation, no output" rule.
            continue;
        };
        let (labels, signature) = if op.keep_name() {
            let signature = labels_signature(&s.labels);
            (s.labels, signature)
        } else {
            strip_name(&s.labels)
        };
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: ts_ms,
                value,
            }],
        });
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Compute the windowed aggregate for one series. Returns the result and
/// the timestamp the output sample should carry (the most recent non-NaN
/// timestamp in the window). NaN samples are skipped; if no non-NaN
/// sample exists in the window, returns `None` so the caller drops the
/// series.
fn compute_over_time(samples: &[Sample], op: OverTimeOp) -> Option<(f64, i64)> {
    // Walk once: find the last non-NaN timestamp, count non-NaN samples,
    // and accumulate the running statistic.
    let mut count: u64 = 0;
    let mut sum: f64 = 0.0;
    let mut min_val: f64 = f64::INFINITY;
    let mut max_val: f64 = f64::NEG_INFINITY;
    let mut last_val: f64 = 0.0;
    let mut last_ts: i64 = 0;

    for s in samples {
        if s.value.is_nan() {
            continue;
        }
        count += 1;
        sum += s.value;
        if s.value < min_val {
            min_val = s.value;
        }
        if s.value > max_val {
            max_val = s.value;
        }
        // `last` follows iteration order; the matrix selector emits
        // samples chronologically.
        last_val = s.value;
        last_ts = s.timestamp_ms;
    }

    if count == 0 {
        return None;
    }

    let value = match op {
        OverTimeOp::Avg => sum / (count as f64),
        OverTimeOp::Sum => sum,
        OverTimeOp::Min => min_val,
        OverTimeOp::Max => max_val,
        OverTimeOp::Count => count as f64,
        OverTimeOp::Last => last_val,
        OverTimeOp::Present => 1.0,
    };
    Some((value, last_ts))
}

/// Distinguish rate (per-second) from increase (absolute) -- the inner
/// arithmetic is identical except for the division by the window width.
#[derive(Debug, Clone, Copy)]
enum WindowOp {
    Rate,
    Increase,
}

fn expect_one_range_vector(args: Vec<EvalResult>, func: &'static str) -> Result<Vec<Series>, EvalError> {
    let mut it = args.into_iter();
    let first = it.next().ok_or_else(|| {
        EvalError::Other(format!("{func} requires a range-vector argument"))
    })?;
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

fn apply_window_op(args: Vec<EvalResult>, op: WindowOp) -> Result<EvalResult, EvalError> {
    let name = match op {
        WindowOp::Rate => "rate",
        WindowOp::Increase => "increase",
    };
    let range = expect_one_range_vector(args, name)?;

    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let Some(value) = compute_window_op(&s.samples, op) else { continue };
        let (labels, signature) = strip_name(&s.labels);
        let ts = s.samples.last().map(|p| p.timestamp_ms).unwrap_or(0);
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

fn compute_window_op(samples: &[Sample], op: WindowOp) -> Option<f64> {
    if samples.is_empty() {
        return None;
    }
    // Netdata's storage hands us deltas for INCREMENTAL dimensions; the
    // sum of those deltas is the total increase over the window.
    let total: f64 = samples
        .iter()
        .map(|s| s.value)
        .filter(|v| !v.is_nan())
        .sum();
    match op {
        WindowOp::Increase => Some(total),
        WindowOp::Rate => {
            // Window width in seconds. Use the actual sample span; if the
            // span collapses to zero (single sample, or all samples at the
            // same timestamp), the rate is undefined.
            let first_ts = samples.first()?.timestamp_ms;
            let last_ts = samples.last()?.timestamp_ms;
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
fn apply_irate(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "irate")?;
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let n = s.samples.len();
        if n < 1 {
            continue;
        }
        // For stored deltas, the last sample IS the per-bucket delta.
        // irate is that delta divided by the bucket width. The bucket
        // width is the gap between the last two stored timestamps; if
        // we only have one sample we can't infer it.
        if n < 2 {
            continue;
        }
        let last = s.samples[n - 1];
        let prev = s.samples[n - 2];
        let dt_ms = (last.timestamp_ms - prev.timestamp_ms).max(0);
        if dt_ms <= 0 || last.value.is_nan() {
            continue;
        }
        let value = last.value / (dt_ms as f64 / 1000.0);
        let (labels, signature) = strip_name(&s.labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: last.timestamp_ms,
                value,
            }],
        });
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// delta: gauge-style change over the window. Last sample minus first.
/// Phase 1 documented divergence: for INCREMENTAL dimensions where
/// stored values are already deltas, this returns the difference of two
/// deltas -- not what the user wants. Use rate()/increase() on counters
/// and delta() on gauges. We do not detect or warn.
fn apply_delta(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let range = expect_one_range_vector(args, "delta")?;
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        if s.samples.len() < 2 {
            continue;
        }
        let first = s.samples.first().copied().unwrap();
        let last = s.samples.last().copied().unwrap();
        if first.value.is_nan() || last.value.is_nan() {
            continue;
        }
        let value = last.value - first.value;
        let (labels, signature) = strip_name(&s.labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: last.timestamp_ms,
                value,
            }],
        });
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
            })
        }
        None => {
            return Err(EvalError::Other(
                "histogram_quantile requires (phi, vector)".to_string(),
            ))
        }
    };
    let vector = match it.next() {
        Some(EvalResult::InstantVector(v)) => v,
        Some(other) => {
            return Err(EvalError::Type {
                context: "histogram_quantile vector",
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            })
        }
        None => {
            return Err(EvalError::Other(
                "histogram_quantile requires (phi, vector)".to_string(),
            ))
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
        let Some(le_str) = lookup_label(&s.labels, "le") else { continue };
        saw_le = true;
        let le = parse_le(le_str);
        let Some(le) = le else { continue };
        let value = s.samples.first().map(|p| p.value).unwrap_or(f64::NAN);
        let ts = s.samples.first().map(|p| p.timestamp_ms).unwrap_or(0);
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
        let entry = groups.entry(key).or_insert_with(|| (key_labels, Vec::new(), ts));
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
        let Some(value) = interpolate_quantile(&buckets, phi) else { continue };
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

fn strip_name(labels: &[(String, String)]) -> (Vec<(String, String)>, u64) {
    let labels: Vec<(String, String)> = labels
        .iter()
        .filter(|(n, _)| n != "__name__")
        .cloned()
        .collect();
    let signature = labels_signature(&labels);
    (labels, signature)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn series(labels: Vec<(&str, &str)>, samples: Vec<(i64, f64)>) -> Series {
        let owned: Vec<(String, String)> = labels
            .into_iter()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        let signature = labels_signature(&owned);
        Series {
            labels: owned,
            signature,
            samples: samples
                .into_iter()
                .map(|(ts, v)| Sample {
                    timestamp_ms: ts,
                    value: v,
                })
                .collect(),
        }
    }

    #[test]
    fn rate_sums_deltas_per_second() {
        // 60 seconds of samples, each carrying delta=10 -> total 60 over 60s = 1/s.
        let mut samples = Vec::new();
        for i in 0..7 {
            samples.push((i * 10_000, 10.0));
        }
        let s = series(vec![("__name__", "foo")], samples);
        let r = apply_window_op(vec![EvalResult::RangeVector(vec![s])], WindowOp::Rate).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                let value = v[0].samples[0].value;
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
        let r =
            apply_window_op(vec![EvalResult::RangeVector(vec![s])], WindowOp::Increase).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].samples[0].value - 35.0).abs() < 1e-9);
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
        let r = apply_irate(vec![EvalResult::RangeVector(vec![s])]).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                // Last delta = 3.0 over 5s gap = 0.6/s.
                assert!((v[0].samples[0].value - 0.6).abs() < 1e-9);
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
        let r = apply_delta(vec![EvalResult::RangeVector(vec![s])]).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].samples[0].value - (-10.0)).abs() < 1e-9);
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
                assert!((v[0].samples[0].value - 2.5).abs() < 1e-9, "got {}", v[0].samples[0].value);
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
        let r = apply_histogram_quantile(vec![
            EvalResult::Scalar(0.99),
            EvalResult::InstantVector(v),
        ])
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
        let r = apply_window_op(vec![EvalResult::RangeVector(vec![s])], WindowOp::Rate).unwrap();
        match r {
            EvalResult::InstantVector(v) => assert_eq!(v.len(), 0),
            _ => panic!(),
        }
    }

    // -------------------------------------------------------------------
    // *_over_time family (SOW-0020 chunk 1)
    // -------------------------------------------------------------------

    fn run_over_time(op: OverTimeOp, samples: Vec<(i64, f64)>) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_over_time(vec![EvalResult::RangeVector(vec![s])], op).unwrap();
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
            OverTimeOp::Present => FuncKind::PresentOverTime,
        };
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_call(func, vec![EvalResult::RangeVector(vec![s])]).unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        }
    }

    #[test]
    fn avg_over_time_computes_mean() {
        let v = run_over_time(OverTimeOp::Avg, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert!((v[0].samples[0].value - 2.0).abs() < 1e-9);
        // __name__ stripped.
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
        // Timestamp is the most recent non-NaN sample's timestamp.
        assert_eq!(v[0].samples[0].timestamp_ms, 2000);
    }

    #[test]
    fn sum_over_time_computes_total() {
        let v = run_over_time(OverTimeOp::Sum, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert!((v[0].samples[0].value - 6.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn min_max_over_time() {
        let v_min = run_over_time(OverTimeOp::Min, vec![(0, 5.0), (1000, 1.0), (2000, 3.0)]);
        assert_eq!(v_min[0].samples[0].value, 1.0);
        let v_max = run_over_time(OverTimeOp::Max, vec![(0, 5.0), (1000, 1.0), (2000, 3.0)]);
        assert_eq!(v_max[0].samples[0].value, 5.0);
    }

    #[test]
    fn count_over_time_counts_non_nan() {
        let v = run_over_time(
            OverTimeOp::Count,
            vec![(0, 1.0), (1000, f64::NAN), (2000, 3.0), (3000, 4.0)],
        );
        assert_eq!(v[0].samples[0].value, 3.0);
    }

    #[test]
    fn last_over_time_preserves_name_and_returns_last_value() {
        let v = run_over_time(OverTimeOp::Last, vec![(0, 1.0), (1000, 2.0), (2000, 3.0)]);
        assert_eq!(v.len(), 1);
        assert_eq!(v[0].samples[0].value, 3.0);
        // __name__ PRESERVED for last_over_time (Prometheus convention).
        assert!(
            v[0].labels.iter().any(|(n, v)| n == "__name__" && v == "foo"),
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
        assert_eq!(v[0].samples[0].value, 2.0);
        assert_eq!(v[0].samples[0].timestamp_ms, 1000);
    }

    #[test]
    fn present_over_time_returns_one() {
        let v = run_over_time(OverTimeOp::Present, vec![(0, 42.0), (1000, 100.0)]);
        assert_eq!(v[0].samples[0].value, 1.0);
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
        assert!((v[0].samples[0].value - 2.0).abs() < 1e-9);
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
            assert_eq!(v[0].samples[0].value, 42.0, "op {:?}", op);
        }
        let v_count = run_over_time(OverTimeOp::Count, vec![(1000, 42.0)]);
        assert_eq!(v_count[0].samples[0].value, 1.0);
    }

    #[test]
    fn over_time_dispatch_via_funckind() {
        // Verify the apply_call dispatch wires up every variant. Belt-and-
        // suspenders: each variant exercised through both apply_over_time
        // and apply_call so a refactor of the dispatch table doesn't
        // silently regress.
        let v = run_over_time_call(OverTimeOp::Avg, vec![(0, 2.0), (1000, 4.0)]);
        assert_eq!(v[0].samples[0].value, 3.0);
        let v = run_over_time_call(OverTimeOp::Present, vec![(0, 1.0)]);
        assert_eq!(v[0].samples[0].value, 1.0);
        let v = run_over_time_call(OverTimeOp::Last, vec![(0, 7.0)]);
        // last preserves __name__.
        assert!(v[0].labels.iter().any(|(n, _)| n == "__name__"));
    }

    #[test]
    fn over_time_requires_range_vector() {
        // Scalar input -> Type error.
        let r = apply_over_time(vec![EvalResult::Scalar(1.0)], OverTimeOp::Avg);
        assert!(matches!(r, Err(EvalError::Type { .. })));
        // Instant vector input -> Type error.
        let s = series(vec![("__name__", "foo")], vec![(0, 1.0)]);
        let r = apply_over_time(
            vec![EvalResult::InstantVector(vec![s])],
            OverTimeOp::Avg,
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
    }
}
