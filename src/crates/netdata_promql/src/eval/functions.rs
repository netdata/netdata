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
        FuncKind::StddevOverTime => apply_over_time(args, OverTimeOp::Stddev),
        FuncKind::StdvarOverTime => apply_over_time(args, OverTimeOp::Stdvar),
        FuncKind::QuantileOverTime => apply_quantile_over_time(args),
        FuncKind::PredictLinear => apply_predict_linear(args),
        FuncKind::HoltWinters => apply_holt_winters(args),
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
    // and accumulate the running statistic. `sum_of_squares` is tracked
    // for the variance family (stdvar/stddev_over_time) added in
    // SOW-0023; for the other variants it's computed-but-unused, a tiny
    // cost that keeps the single-pass shape.
    let mut count: u64 = 0;
    let mut sum: f64 = 0.0;
    let mut sum_sq: f64 = 0.0;
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
        sum_sq += s.value * s.value;
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
        OverTimeOp::Stdvar => {
            // Population variance: E[X^2] - (E[X])^2. Simple form;
            // catastrophic cancellation only matters on highly
            // concentrated distributions which are atypical for PromQL.
            let mean = sum / (count as f64);
            let var = sum_sq / (count as f64) - mean * mean;
            // Clamp negative results (round-off) to zero.
            if var < 0.0 {
                0.0
            } else {
                var
            }
        }
        OverTimeOp::Stddev => {
            let mean = sum / (count as f64);
            let var = sum_sq / (count as f64) - mean * mean;
            if var < 0.0 {
                0.0
            } else {
                var.sqrt()
            }
        }
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

// ---------------------------------------------------------------------------
// Two-arg over_time variants and predictive functions (SOW-0023)
// ---------------------------------------------------------------------------

/// quantile_over_time(phi, range_vector): per series, sort non-NaN
/// samples ascending and apply the same linear-interpolation formula
/// used by the `quantile` aggregator (SOW-0021). Phi outside [0, 1]
/// clamps to +-Inf per Prometheus. Empty window drops the series.
fn apply_quantile_over_time(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let (phi, range) = expect_scalar_then_range(args, "quantile_over_time")?;
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        let mut values: Vec<f64> = s
            .samples
            .iter()
            .map(|sm| sm.value)
            .filter(|v| !v.is_nan())
            .collect();
        if values.is_empty() {
            continue;
        }
        values.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
        let value = crate::eval::aggregation::compute_quantile(&values, phi);
        let ts = s
            .samples
            .iter()
            .rev()
            .find(|sm| !sm.value.is_nan())
            .map(|sm| sm.timestamp_ms)
            .unwrap_or(0);
        let (labels, signature) = strip_name(&s.labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample { timestamp_ms: ts, value }],
        });
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// predict_linear(range_vector, t): per series, fit y = slope*x + b on
/// (timestamp_s - now_s, value) pairs of non-NaN samples via ordinary
/// least squares, then return `intercept + slope * t` -- the
/// extrapolated value `t` seconds in the future. Window with fewer
/// than 2 distinct timestamps drops the series.
fn apply_predict_linear(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
    let (range, t_secs) = expect_range_then_scalar(args, "predict_linear")?;
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        // Reference time = the most recent sample's timestamp; we
        // center x values around it so the slope/intercept arithmetic
        // doesn't blow up on epoch-scale Unix timestamps.
        let Some(last_ts_ms) = s.samples.iter().rev().find_map(|sm| {
            if sm.value.is_nan() {
                None
            } else {
                Some(sm.timestamp_ms)
            }
        }) else {
            continue;
        };
        let now_s = (last_ts_ms as f64) / 1000.0;
        let mut n = 0.0;
        let mut sum_x = 0.0;
        let mut sum_y = 0.0;
        let mut sum_xx = 0.0;
        let mut sum_xy = 0.0;
        let mut first_x = f64::NAN;
        let mut distinct_x = false;
        for sm in &s.samples {
            if sm.value.is_nan() {
                continue;
            }
            let x = (sm.timestamp_ms as f64) / 1000.0 - now_s;
            let y = sm.value;
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
            continue;
        }
        let denom = n * sum_xx - sum_x * sum_x;
        if denom == 0.0 {
            continue;
        }
        let slope = (n * sum_xy - sum_x * sum_y) / denom;
        let intercept = (sum_y - slope * sum_x) / n;
        let value = intercept + slope * t_secs;
        let (labels, signature) = strip_name(&s.labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: last_ts_ms,
                value,
            }],
        });
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// holt_winters(range_vector, sf, tf): double-exponential smoothing.
/// Both factors must be in (0, 1]; out-of-range params -> evaluation
/// error. Returns the smoothed value at the end of the window.
fn apply_holt_winters(args: Vec<EvalResult>) -> Result<EvalResult, EvalError> {
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
    let mut out = Vec::with_capacity(range.len());
    for s in range.into_iter() {
        // Walk non-NaN samples chronologically.
        let mut iter = s.samples.iter().filter(|sm| !sm.value.is_nan());
        let Some(first) = iter.next() else { continue };
        let Some(second) = iter.next() else { continue };
        let mut s_prev = first.value;
        let mut b_prev = second.value - first.value;
        let mut s_curr = second.value;
        let mut last_ts = second.timestamp_ms;
        // Update with the second sample first to set s_curr.
        s_curr = sf * second.value + (1.0 - sf) * (s_prev + b_prev);
        b_prev = tf * (s_curr - s_prev) + (1.0 - tf) * b_prev;
        s_prev = s_curr;
        for sm in iter {
            let s_new = sf * sm.value + (1.0 - sf) * (s_prev + b_prev);
            let b_new = tf * (s_new - s_prev) + (1.0 - tf) * b_prev;
            s_prev = s_new;
            b_prev = b_new;
            s_curr = s_new;
            last_ts = sm.timestamp_ms;
        }
        let (labels, signature) = strip_name(&s.labels);
        out.push(Series {
            labels,
            signature,
            samples: vec![Sample {
                timestamp_ms: last_ts,
                value: s_curr,
            }],
        });
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
            return Err(EvalError::Other(format!(
                "{func} requires three arguments"
            )));
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
            return Err(EvalError::Other(format!(
                "{func} requires three arguments"
            )));
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
            OverTimeOp::Stddev => FuncKind::StddevOverTime,
            OverTimeOp::Stdvar => FuncKind::StdvarOverTime,
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

    // -------------------------------------------------------------------
    // SOW-0023: stddev/stdvar/quantile_over_time, predict_linear,
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
        assert!((v[0].samples[0].value - 2.0).abs() < 1e-9, "got {}", v[0].samples[0].value);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn stddev_over_time_is_sqrt_of_stdvar() {
        let v = run_over_time(
            OverTimeOp::Stddev,
            vec![(0, 1.0), (1, 2.0), (2, 3.0), (3, 4.0), (4, 5.0)],
        );
        assert!((v[0].samples[0].value - 2.0_f64.sqrt()).abs() < 1e-9);
    }

    #[test]
    fn stddev_over_time_constant_series_is_zero() {
        let v = run_over_time(OverTimeOp::Stddev, vec![(0, 7.0), (1, 7.0), (2, 7.0)]);
        // Negative round-off clamped to zero in compute_over_time.
        assert!(v[0].samples[0].value.abs() < 1e-9);
    }

    #[test]
    fn stddev_over_time_skips_nan() {
        let v = run_over_time(
            OverTimeOp::Stddev,
            vec![(0, 1.0), (1, f64::NAN), (2, 5.0)],
        );
        // mean = 3, var = (1+25)/2 - 9 = 4 -> stddev 2.
        assert!((v[0].samples[0].value - 2.0).abs() < 1e-9);
    }

    fn run_quantile_over_time(phi: f64, samples: Vec<(i64, f64)>) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_quantile_over_time(vec![
            EvalResult::Scalar(phi),
            EvalResult::RangeVector(vec![s]),
        ])
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!(),
        }
    }

    #[test]
    fn quantile_over_time_median() {
        let v = run_quantile_over_time(
            0.5,
            vec![(0, 1.0), (1, 2.0), (2, 3.0), (3, 4.0), (4, 5.0)],
        );
        assert!((v[0].samples[0].value - 3.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn quantile_over_time_max_at_one_min_at_zero() {
        let samples = vec![(0, 1.0), (1, 7.0), (2, 3.0)];
        let v1 = run_quantile_over_time(1.0, samples.clone());
        assert_eq!(v1[0].samples[0].value, 7.0);
        let v0 = run_quantile_over_time(0.0, samples);
        assert_eq!(v0[0].samples[0].value, 1.0);
    }

    #[test]
    fn quantile_over_time_clamps_phi() {
        let samples = vec![(0, 1.0), (1, 2.0)];
        let v_neg = run_quantile_over_time(-0.5, samples.clone());
        assert_eq!(v_neg[0].samples[0].value, f64::NEG_INFINITY);
        let v_high = run_quantile_over_time(1.5, samples);
        assert_eq!(v_high[0].samples[0].value, f64::INFINITY);
    }

    fn run_predict_linear(samples: Vec<(i64, f64)>, t_secs: f64) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        let r = apply_predict_linear(vec![
            EvalResult::RangeVector(vec![s]),
            EvalResult::Scalar(t_secs),
        ])
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
        let v = run_predict_linear(
            vec![(0, 1.0), (1000, 3.0), (2000, 5.0), (3000, 7.0)],
            10.0,
        );
        assert_eq!(v.len(), 1);
        assert!((v[0].samples[0].value - 27.0).abs() < 1e-9, "got {}", v[0].samples[0].value);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
    }

    #[test]
    fn predict_linear_zero_t_returns_intercept_at_last_sample() {
        // With t=0 the prediction = the linear-fit value at the last
        // sample's reference time, which for a perfect line equals the
        // last sample's value.
        let v = run_predict_linear(
            vec![(0, 1.0), (1000, 3.0), (2000, 5.0)],
            0.0,
        );
        // The intercept at x=0 (the reference time = last sample's
        // time) is the predicted value. For y=2x+1 centered at the
        // last sample (x=2): y = 5 at x=2 -> in centered coordinates
        // intercept = 5.
        assert!((v[0].samples[0].value - 5.0).abs() < 1e-9, "got {}", v[0].samples[0].value);
    }

    #[test]
    fn predict_linear_single_sample_drops() {
        let v = run_predict_linear(vec![(0, 42.0)], 10.0);
        assert!(v.is_empty());
    }

    fn run_holt_winters(samples: Vec<(i64, f64)>, sf: f64, tf: f64) -> Vec<Series> {
        let s = series(vec![("__name__", "foo"), ("dim", "x")], samples);
        apply_holt_winters(vec![
            EvalResult::RangeVector(vec![s]),
            EvalResult::Scalar(sf),
            EvalResult::Scalar(tf),
        ])
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
        assert!((v[0].samples[0].value - 20.0).abs() < 1e-9);
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
        let r = apply_holt_winters(vec![
            EvalResult::RangeVector(vec![s.clone()]),
            EvalResult::Scalar(1.5),
            EvalResult::Scalar(0.5),
        ]);
        assert!(matches!(r, Err(EvalError::Other(_))));

        let r = apply_holt_winters(vec![
            EvalResult::RangeVector(vec![s.clone()]),
            EvalResult::Scalar(0.5),
            EvalResult::Scalar(0.0),
        ]);
        assert!(matches!(r, Err(EvalError::Other(_))));
    }

    #[test]
    fn two_arg_funcs_reject_wrong_shapes() {
        // quantile_over_time wants (scalar, range); passing two scalars
        // is a type error.
        let r = apply_quantile_over_time(vec![EvalResult::Scalar(0.5), EvalResult::Scalar(1.0)]);
        assert!(matches!(r, Err(EvalError::Type { .. })));
        // predict_linear wants (range, scalar); passing (scalar, scalar)
        // errors on the range slot.
        let r = apply_predict_linear(vec![EvalResult::Scalar(0.5), EvalResult::Scalar(1.0)]);
        assert!(matches!(r, Err(EvalError::Type { .. })));
    }
}
