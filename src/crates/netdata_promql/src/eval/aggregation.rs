// SPDX-License-Identifier: GPL-3.0-or-later
//
// Aggregations: sum, avg, min, max, count, topk, bottomk, quantile,
// count_values, with optional by/without grouping. SOW-0031 made them
// grid-aware (iterate positions). SOW-0032 made them column-oriented:
// input series carry `values: Vec<f64>` indexed by grid position, and
// output series share the same `Arc<Vec<i64>>` timestamps as the input.

use std::collections::BTreeMap;
use std::sync::Arc;

use crate::plan::{AggrKind, Grouping, ValueType};

use super::types::{labels_signature, EvalError, EvalResult, Series};

/// Apply an aggregation operator to an instant vector.
///
/// `param` carries the scalar argument for parametrized aggregators
/// (topk/bottomk/quantile). `param_string` carries the string argument
/// for `count_values`. The lowering layer enforces that exactly one is
/// `Some` when required.
pub fn apply_aggregate(
    op: AggrKind,
    grouping: Option<&Grouping>,
    param: Option<f64>,
    param_string: Option<&str>,
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
        AggrKind::CountValues => {
            let label = param_string.expect("lowering guarantees label for count_values");
            apply_count_values(grouping, label, series)
        }
        AggrKind::LimitK => {
            let k = param.expect("lowering guarantees k for limitk");
            apply_limitk(grouping, k, series)
        }
        AggrKind::LimitRatio => {
            let ratio = param.expect("lowering guarantees ratio for limit_ratio");
            apply_limit_ratio(grouping, ratio, series)
        }
        AggrKind::Group => apply_group(grouping, series),
        // Sum/Avg/Min/Max/Count/Stddev/Stdvar all flow through the
        // shared collapsing path; their Bucket::finalize_value arms
        // produce the per-position output.
        _ => apply_collapsing(op, grouping, series),
    }
}

/// Grid-aware path for sum/avg/min/max/count. At each grid position the
/// input series share a common timestamp; we bucket by grouping key,
/// accumulate over the position, and collapse to a per-bucket value.
/// The output series share the input's `timestamps` Arc.
fn apply_collapsing(
    op: AggrKind,
    grouping: Option<&Grouping>,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let n = series[0].len();
    let timestamps = Arc::clone(&series[0].timestamps);

    // Pre-compute per-series bucket key & projected labels so we don't
    // recompute them at each grid position.
    let bucket_keys: Vec<(u64, Vec<(String, String)>)> = series
        .iter()
        .map(|s| {
            let kl = project_labels(&s.labels, grouping);
            (labels_signature(&kl), kl)
        })
        .collect();

    // out_buckets: signature -> (labels, values_per_position).
    let mut out_buckets: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>)> = BTreeMap::new();

    for i in 0..n {
        let mut per_bucket: BTreeMap<u64, Bucket> = BTreeMap::new();
        for (j, s) in series.iter().enumerate() {
            let v = s.values[i];
            let (key, key_labels) = &bucket_keys[j];
            per_bucket
                .entry(*key)
                .or_insert_with(|| Bucket::new(key_labels.clone()))
                .accumulate(v);
        }
        for (key, bucket) in per_bucket {
            let value = bucket.finalize_value(op);
            let labels = bucket.labels.clone();
            let entry = out_buckets
                .entry(key)
                .or_insert_with(|| (labels, Vec::with_capacity(n)));
            entry.1.push(value);
        }
    }

    let mut out: Vec<Series> = out_buckets
        .into_iter()
        .map(|(_, (labels, values))| Series::new(labels, Arc::clone(&timestamps), values))
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// topk(k, expr) / bottomk(k, expr): at each grid position, bucket by
/// grouping, sort by value, keep top or bottom k. Output preserves
/// **all** original input labels including `__name__` (SOW-0035 --
/// distinct from the general aggregation convention which strips
/// `__name__`). A series that ranked at some positions but not others
/// gets NaN at the missing positions; the per-position drop-NaN rule
/// does not collapse the series across positions.
fn apply_topk_bottomk(
    op: AggrKind,
    grouping: Option<&Grouping>,
    k_param: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if !k_param.is_finite() || k_param <= 0.0 {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let k = k_param as usize;
    let descending = matches!(op, AggrKind::TopK);

    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let n = series[0].len();
    let timestamps = Arc::clone(&series[0].timestamps);

    let bucket_keys: Vec<u64> = series
        .iter()
        .map(|s| labels_signature(&project_labels(&s.labels, grouping)))
        .collect();

    // Pre-group input indices by bucket (same for every position).
    let mut by_bucket_template: BTreeMap<u64, Vec<usize>> = BTreeMap::new();
    for (j, &b) in bucket_keys.iter().enumerate() {
        by_bucket_template.entry(b).or_default().push(j);
    }

    // accum: input-series signature -> (labels, values), one f64 per
    // grid position. Output series get NaN at positions where the
    // input series did not rank in the top/bottom-k for its bucket.
    // SOW-0035: use the input series' own signature/labels so that
    // `__name__` is preserved on the output (Prometheus semantics).
    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>)> = BTreeMap::new();

    for i in 0..n {
        // Track which input indices ranked at this position. Initialise
        // every bucket pass with the rankers; then back-fill NaN for
        // input indices that did not rank.
        let mut ranked_value: std::collections::HashMap<usize, f64> =
            std::collections::HashMap::new();
        for (_, indices) in by_bucket_template.iter() {
            let mut candidates: Vec<usize> = indices
                .iter()
                .copied()
                .filter(|&j| !series[j].values[i].is_nan())
                .collect();
            candidates.sort_by(|&a, &b| {
                let av = series[a].values[i];
                let bv = series[b].values[i];
                if descending {
                    bv.total_cmp(&av)
                } else {
                    av.total_cmp(&bv)
                }
            });
            candidates.truncate(k);
            for j in candidates {
                ranked_value.insert(j, series[j].values[i]);
            }
        }

        for (j, s) in series.iter().enumerate() {
            let value = ranked_value.get(&j).copied();
            let entry = accum
                .entry(s.signature)
                .or_insert_with(|| (s.labels.clone(), Vec::with_capacity(n)));
            entry.1.push(value.unwrap_or(f64::NAN));
        }
    }

    let mut out: Vec<Series> = accum
        .into_iter()
        .filter_map(|(_, (labels, values))| {
            // Drop series that never ranked at any position.
            if values.iter().all(|v| v.is_nan()) {
                return None;
            }
            Some(Series::new(labels, Arc::clone(&timestamps), values))
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// quantile(phi, expr): per group at each grid position, compute the
/// phi-quantile via linear interpolation between adjacent ranked
/// observations. Phi outside [0, 1] clamps to ±Inf to match Prometheus.
fn apply_quantile(
    grouping: Option<&Grouping>,
    phi: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let n = series[0].len();
    let timestamps = Arc::clone(&series[0].timestamps);

    let bucket_keys: Vec<(u64, Vec<(String, String)>)> = series
        .iter()
        .map(|s| {
            let kl = project_labels(&s.labels, grouping);
            (labels_signature(&kl), kl)
        })
        .collect();

    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>)> = BTreeMap::new();
    for i in 0..n {
        let mut per_bucket: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>)> = BTreeMap::new();
        for (j, s) in series.iter().enumerate() {
            let v = s.values[i];
            let (key, key_labels) = &bucket_keys[j];
            let entry = per_bucket
                .entry(*key)
                .or_insert_with(|| (key_labels.clone(), Vec::new()));
            if !v.is_nan() {
                entry.1.push(v);
            }
        }
        for (key, (labels, mut values)) in per_bucket {
            let value = if values.is_empty() {
                f64::NAN
            } else {
                values.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));
                compute_quantile(&values, phi)
            };
            let entry = accum
                .entry(key)
                .or_insert_with(|| (labels, Vec::with_capacity(n)));
            entry.1.push(value);
        }
    }

    let mut out: Vec<Series> = accum
        .into_iter()
        .filter_map(|(_, (labels, values))| {
            if values.iter().all(|v| v.is_nan()) {
                return None;
            }
            Some(Series::new(labels, Arc::clone(&timestamps), values))
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// count_values(label, expr): per grid position, bucket input series by
/// their value (stringified) and emit one output series per bucket with
/// the supplied label set to that value-string and the series value
/// equal to the bucket count.
fn apply_count_values(
    grouping: Option<&Grouping>,
    label: &str,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let n = series[0].len();
    let timestamps = Arc::clone(&series[0].timestamps);

    let projected: Vec<Vec<(String, String)>> = series
        .iter()
        .map(|s| project_labels(&s.labels, grouping))
        .collect();

    // accum keyed by output signature (with the new label slot applied).
    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<f64>)> = BTreeMap::new();
    for i in 0..n {
        let mut per_bucket: BTreeMap<u64, (Vec<(String, String)>, u64)> = BTreeMap::new();
        for (j, s) in series.iter().enumerate() {
            let v = s.values[i];
            let value_str = format_value_for_label(v);
            let mut output_labels = projected[j].clone();
            output_labels.push((label.to_string(), value_str));
            output_labels.sort_by(|a, b| a.0.cmp(&b.0));
            let sig = labels_signature(&output_labels);
            let entry = per_bucket
                .entry(sig)
                .or_insert_with(|| (output_labels, 0));
            entry.1 += 1;
        }
        // Track which buckets contributed this position so we can pad
        // NaN for buckets that didn't.
        let mut touched: std::collections::HashSet<u64> = std::collections::HashSet::new();
        for (sig, (labels, count)) in per_bucket {
            touched.insert(sig);
            let entry = accum
                .entry(sig)
                .or_insert_with(|| (labels, Vec::with_capacity(n)));
            // Pad to current position with NaN if this bucket missed prior positions.
            while entry.1.len() < i {
                entry.1.push(f64::NAN);
            }
            entry.1.push(count as f64);
        }
        // Buckets that existed in prior positions but didn't appear this
        // position get a NaN.
        let existing_keys: Vec<u64> = accum.keys().copied().collect();
        for sig in existing_keys {
            let entry = accum.get_mut(&sig).unwrap();
            if entry.1.len() < i + 1 && !touched.contains(&sig) {
                while entry.1.len() < i + 1 {
                    entry.1.push(f64::NAN);
                }
            }
        }
    }

    // Right-pad any bucket that ran short.
    for (_, (_, vals)) in accum.iter_mut() {
        while vals.len() < n {
            vals.push(f64::NAN);
        }
    }

    let mut out: Vec<Series> = accum
        .into_iter()
        .map(|(_, (labels, values))| Series::new(labels, Arc::clone(&timestamps), values))
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// `group(v)` (SOW-0034). Per bucket per grid position, emit 1.0 if
/// any input series had a non-NaN value at that position; NaN
/// otherwise. Output labels follow the grouping projection (same as
/// sum/avg). Used in production as a join-key fabricator: takes a
/// labeled input and emits a singleton-per-group series of 1s that
/// can be joined against other vectors via vector matching.
fn apply_group(
    grouping: Option<&Grouping>,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let n = series[0].len();
    let timestamps = Arc::clone(&series[0].timestamps);

    let bucket_keys: Vec<(u64, Vec<(String, String)>)> = series
        .iter()
        .map(|s| {
            let kl = project_labels(&s.labels, grouping);
            (labels_signature(&kl), kl)
        })
        .collect();

    // accum: per bucket -> (labels, has_input_per_position).
    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<bool>)> = BTreeMap::new();
    for i in 0..n {
        for (j, s) in series.iter().enumerate() {
            let (key, key_labels) = &bucket_keys[j];
            let entry = accum
                .entry(*key)
                .or_insert_with(|| (key_labels.clone(), vec![false; n]));
            if !s.values[i].is_nan() {
                entry.1[i] = true;
            }
        }
    }

    let mut out: Vec<Series> = accum
        .into_iter()
        .filter_map(|(_, (labels, present))| {
            let values: Vec<f64> = present
                .into_iter()
                .map(|p| if p { 1.0 } else { f64::NAN })
                .collect();
            if values.iter().all(|v| v.is_nan()) {
                return None;
            }
            Some(Series::new(labels, Arc::clone(&timestamps), values))
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// `limitk(k, v)` (SOW-0034). Per bucket per grid position, pick the
/// first `k` input series in signature order (stable, deterministic,
/// independent of value). Output series carry the original input
/// labels minus `__name__` -- same convention as `topk`/`bottomk`.
///
/// At grid positions where a selected series happens to be NaN, the
/// output preserves the NaN; selection is by series identity, not by
/// per-position value.
fn apply_limitk(
    grouping: Option<&Grouping>,
    k_param: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if !k_param.is_finite() || k_param <= 0.0 {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let k = k_param as usize;
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }

    // Group input series indices by bucket-signature; within each
    // bucket, the input series whose stripped signatures sort first
    // are kept.
    let bucket_keys: Vec<u64> = series
        .iter()
        .map(|s| labels_signature(&project_labels(&s.labels, grouping)))
        .collect();
    let stripped_labels: Vec<Vec<(String, String)>> = series
        .iter()
        .map(|s| {
            s.labels
                .iter()
                .filter(|(n, _)| n != "__name__")
                .cloned()
                .collect()
        })
        .collect();
    let stripped_signatures: Vec<u64> = stripped_labels
        .iter()
        .map(|l| labels_signature(l))
        .collect();

    let mut by_bucket: BTreeMap<u64, Vec<usize>> = BTreeMap::new();
    for (j, &b) in bucket_keys.iter().enumerate() {
        by_bucket.entry(b).or_default().push(j);
    }

    // Select the kept input-series indices once (selection is
    // position-independent for limitk).
    let mut kept: Vec<usize> = Vec::new();
    for (_, mut indices) in by_bucket {
        indices.sort_by_key(|&j| stripped_signatures[j]);
        indices.truncate(k);
        kept.extend(indices);
    }

    let mut out: Vec<Series> = kept
        .into_iter()
        .map(|j| {
            Series::new(
                stripped_labels[j].clone(),
                Arc::clone(&series[j].timestamps),
                series[j].values.clone(),
            )
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// `limit_ratio(ratio, v)` (SOW-0034). Per bucket, select a
/// deterministic-random subset of input series whose
/// `hash01(signature)` falls below `ratio` (positive) or above
/// `1 + ratio` (negative). `ratio` outside `[-1, 1]` clamps to all-
/// selected. `ratio` NaN or zero -> empty result.
///
/// Like `limitk`, selection is series-level (not per-position), and
/// output labels are the input series' labels minus `__name__`.
fn apply_limit_ratio(
    grouping: Option<&Grouping>,
    ratio: f64,
    series: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    if ratio.is_nan() || ratio == 0.0 {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    if series.is_empty() {
        return Ok(EvalResult::InstantVector(Vec::new()));
    }
    let r = ratio.clamp(-1.0, 1.0);

    // Group by bucket. Within each bucket, select series by
    // `hash01(sig) < r` for positive r, or `hash01(sig) >= 1+r` for
    // negative r. The hash01 lives on the *stripped* signature so
    // that label projection doesn't change the selection.
    let bucket_keys: Vec<u64> = series
        .iter()
        .map(|s| labels_signature(&project_labels(&s.labels, grouping)))
        .collect();
    let stripped_labels: Vec<Vec<(String, String)>> = series
        .iter()
        .map(|s| {
            s.labels
                .iter()
                .filter(|(n, _)| n != "__name__")
                .cloned()
                .collect()
        })
        .collect();
    let stripped_signatures: Vec<u64> = stripped_labels
        .iter()
        .map(|l| labels_signature(l))
        .collect();

    let mut by_bucket: BTreeMap<u64, Vec<usize>> = BTreeMap::new();
    for (j, &b) in bucket_keys.iter().enumerate() {
        by_bucket.entry(b).or_default().push(j);
    }

    let mut kept: Vec<usize> = Vec::new();
    for (_, indices) in by_bucket {
        for j in indices {
            let h = hash01(stripped_signatures[j]);
            let select = if r >= 0.0 { h < r } else { h >= 1.0 + r };
            if select {
                kept.push(j);
            }
        }
    }

    let mut out: Vec<Series> = kept
        .into_iter()
        .map(|j| {
            Series::new(
                stripped_labels[j].clone(),
                Arc::clone(&series[j].timestamps),
                series[j].values.clone(),
            )
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Map a u64 signature to a deterministic `f64` in `[0, 1)`. Used by
/// `limit_ratio` for stable per-series selection. The output is not
/// bit-identical to Prometheus' choice of hash, but it satisfies the
/// spec (deterministic, uniform-ish across the input space).
fn hash01(sig: u64) -> f64 {
    (sig as f64) / (u64::MAX as f64)
}

/// Format a float for use as a PromQL label value. Mirrors Prometheus'
/// Go default formatting closely enough for typical inputs.
fn format_value_for_label(v: f64) -> String {
    if v.is_nan() {
        return "NaN".to_string();
    }
    if v.is_infinite() {
        return if v > 0.0 { "+Inf".to_string() } else { "-Inf".to_string() };
    }
    format!("{v}")
}

/// Standard linear-interpolation quantile over a pre-sorted slice.
/// Matches Prometheus' `quantile` aggregator: rank = phi * (n - 1),
/// lerp between floor(rank) and ceil(rank).
pub(crate) fn compute_quantile(sorted: &[f64], phi: f64) -> f64 {
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
    sum: f64,
    sum_sq: f64,
    count: u64,
    min: f64,
    max: f64,
}

impl Bucket {
    fn new(labels: Vec<(String, String)>) -> Self {
        Self {
            labels,
            sum: 0.0,
            sum_sq: 0.0,
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
        self.sum_sq += v * v;
        self.count += 1;
        if v < self.min {
            self.min = v;
        }
        if v > self.max {
            self.max = v;
        }
    }

    /// Reduce the bucket to a single f64 for one grid position. NaN
    /// when no non-NaN observation reached the bucket; matches the
    /// Prometheus "no observations -> drop the series at this point"
    /// semantics (the upstream caller filters all-NaN series).
    fn finalize_value(&self, op: AggrKind) -> f64 {
        if self.count == 0 {
            return f64::NAN;
        }
        let n = self.count as f64;
        match op {
            AggrKind::Sum => self.sum,
            AggrKind::Avg => self.sum / n,
            AggrKind::Min => self.min,
            AggrKind::Max => self.max,
            AggrKind::Count => n,
            AggrKind::Stdvar => {
                // Population variance: E[X²] - (E[X])². Clamp tiny
                // negative round-off to zero. Same formula as the
                // `stdvar_over_time` rollup.
                let mean = self.sum / n;
                let var = self.sum_sq / n - mean * mean;
                if var < 0.0 {
                    0.0
                } else {
                    var
                }
            }
            AggrKind::Stddev => {
                let mean = self.sum / n;
                let var = self.sum_sq / n - mean * mean;
                if var < 0.0 {
                    0.0
                } else {
                    var.sqrt()
                }
            }
            AggrKind::TopK
            | AggrKind::BottomK
            | AggrKind::Quantile
            | AggrKind::CountValues
            | AggrKind::LimitK
            | AggrKind::LimitRatio
            | AggrKind::Group => {
                unreachable!("parametrized/non-collapsing aggregator in Bucket::finalize_value: {:?}", op)
            }
        }
    }
}

/// Project a series's labels onto (or away from) the grouping
/// clause's label list. Drops `__name__` *unless the user explicitly
/// names it* in `by (__name__, ...)`; for `without (...)`, drops
/// `__name__` only when it appears in the `without` list. SOW-0037
/// brought this in line with Prometheus -- aggregation output
/// normally has no metric name, but `by (__name__, ...)` is a
/// deliberate request to keep it.
fn project_labels(labels: &[(String, String)], grouping: Option<&Grouping>) -> Vec<(String, String)> {
    let kept: Vec<(String, String)> = match grouping {
        None => Vec::new(),
        Some(Grouping::By(names)) => {
            let explicit_name = names.iter().any(|k| k == "__name__");
            labels
                .iter()
                .filter(|(n, _)| {
                    if n == "__name__" {
                        explicit_name
                    } else {
                        names.iter().any(|k| k == n)
                    }
                })
                .cloned()
                .collect()
        }
        Some(Grouping::Without(names)) => {
            let drop_name = names.iter().any(|k| k == "__name__");
            labels
                .iter()
                .filter(|(n, _)| {
                    if n == "__name__" {
                        !drop_name
                    } else {
                        !names.iter().any(|k| k == n)
                    }
                })
                .cloned()
                .collect()
        }
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
        Series::scalar(labels, 1000, val)
    }

    fn s_with(group: &str, dim: &str, val: f64) -> Series {
        let labels = vec![
            ("__name__".to_string(), "metric".to_string()),
            ("group".to_string(), group.to_string()),
            ("dimension".to_string(), dim.to_string()),
        ];
        Series::scalar(labels, 1000, val)
    }

    #[test]
    fn sum_without_grouping_collapses_to_one() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let r = apply_aggregate(AggrKind::Sum, None, None, None, input).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 1);
                assert!((v[0].values[0] - 6.0).abs() < 1e-9);
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
            None,
            input,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert_eq!(v.len(), 2);
                let mut values: Vec<f64> = v.iter().map(|s| s.values[0]).collect();
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
        let r = apply_aggregate(AggrKind::Count, None, None, None, input).unwrap();
        match r {
            EvalResult::InstantVector(v) => {
                assert!((v[0].values[0] - 3.0).abs() < 1e-9);
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
        let avg = apply_aggregate(AggrKind::Avg, None, None, None, input.clone()).unwrap();
        let min = apply_aggregate(AggrKind::Min, None, None, None, input.clone()).unwrap();
        let max = apply_aggregate(AggrKind::Max, None, None, None, input).unwrap();
        if let (EvalResult::InstantVector(a), EvalResult::InstantVector(b), EvalResult::InstantVector(c)) =
            (avg, min, max)
        {
            assert!((a[0].values[0] - 3.0).abs() < 1e-9);
            assert!((b[0].values[0] - 1.0).abs() < 1e-9);
            assert!((c[0].values[0] - 5.0).abs() < 1e-9);
        } else {
            panic!();
        }
    }

    fn unwrap_vec(r: EvalResult) -> Vec<Series> {
        match r {
            EvalResult::InstantVector(v) => v,
            _ => panic!("expected instant vector"),
        }
    }

    fn values_sorted(v: &[Series]) -> Vec<f64> {
        let mut out: Vec<f64> = v.iter().map(|s| s.values[0]).collect();
        out.sort_by(|a, b| a.partial_cmp(b).unwrap());
        out
    }

    #[test]
    fn topk_returns_top_k_preserving_name() {
        // SOW-0035: topk/bottomk preserve `__name__` (and every other
        // input label) on output, unlike sum/avg which collapse to
        // the grouping projection.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 4.0),
            s("foo", "c", 3.0),
            s("foo", "d", 2.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(2.0), None, input).unwrap());
        assert_eq!(v.len(), 2);
        assert_eq!(values_sorted(&v), vec![3.0, 4.0]);
        for s in &v {
            assert!(
                s.labels.iter().any(|(n, v)| n == "__name__" && v == "foo"),
                "__name__ must be preserved on topk/bottomk output: labels = {:?}",
                s.labels
            );
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
        let v = unwrap_vec(apply_aggregate(AggrKind::BottomK, None, Some(2.0), None, input).unwrap());
        assert_eq!(v.len(), 2);
        assert_eq!(values_sorted(&v), vec![1.0, 2.0]);
    }

    #[test]
    fn topk_zero_returns_empty() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 1.0)]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(0.0), None, input.clone()).unwrap());
        assert!(v.is_empty());
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(-1.0), None, input.clone()).unwrap());
        assert!(v.is_empty());
        let v = unwrap_vec(apply_aggregate(AggrKind::BottomK, None, Some(f64::NAN), None, input).unwrap());
        assert!(v.is_empty());
    }

    #[test]
    fn topk_skips_nan_values() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", f64::NAN),
            s("foo", "c", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(3.0), None, input).unwrap());
        assert_eq!(v.len(), 2);
    }

    #[test]
    fn topk_with_grouping_per_bucket() {
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
                None,
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
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.5), None, input).unwrap());
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - 3.0).abs() < 1e-9);
    }

    #[test]
    fn quantile_zero_returns_min_one_returns_max() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 7.0),
            s("foo", "c", 3.0),
        ]);
        let v0 = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.0), None, input.clone()).unwrap());
        assert_eq!(v0[0].values[0], 1.0);
        let v1 = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(1.0), None, input).unwrap());
        assert_eq!(v1[0].values[0], 7.0);
    }

    #[test]
    fn quantile_interpolates_between_samples() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
            s("foo", "e", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.95), None, input).unwrap());
        assert!((v[0].values[0] - 4.8).abs() < 1e-9, "got {}", v[0].values[0]);
    }

    #[test]
    fn quantile_out_of_range_clamps_to_infinity() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 1.0), s("foo", "b", 2.0)]);
        let v_neg = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(-0.5), None, input.clone()).unwrap());
        assert_eq!(v_neg[0].values[0], f64::NEG_INFINITY);
        let v_high = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(2.0), None, input).unwrap());
        assert_eq!(v_high[0].values[0], f64::INFINITY);
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
                None,
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
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", f64::NAN),
            s("foo", "b", f64::NAN),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Quantile, None, Some(0.5), None, input).unwrap());
        assert!(v.is_empty());
    }

    #[test]
    fn topk_with_non_integer_k_truncates() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::TopK, None, Some(2.9), None, input).unwrap());
        assert_eq!(v.len(), 2);
    }

    #[test]
    fn count_values_buckets_by_value() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 1.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::CountValues,
                None,
                None,
                Some("v"),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        let pairs: Vec<(String, f64)> = v
            .iter()
            .map(|s| {
                let label = s
                    .labels
                    .iter()
                    .find(|(n, _)| n == "v")
                    .map(|(_, v)| v.clone())
                    .unwrap_or_default();
                (label, s.values[0])
            })
            .collect();
        assert!(pairs.contains(&("1".to_string(), 2.0)));
        assert!(pairs.contains(&("2".to_string(), 1.0)));
        for s in &v {
            assert!(s.labels.iter().all(|(n, _)| n != "__name__"));
        }
    }

    #[test]
    fn count_values_all_same_collapses_to_one() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 5.0),
            s("foo", "b", 5.0),
            s("foo", "c", 5.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::CountValues,
                None,
                None,
                Some("v"),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 1);
        assert_eq!(v[0].values[0], 3.0);
    }

    #[test]
    fn count_values_with_grouping_partitions() {
        let input = EvalResult::InstantVector(vec![
            s_with("g1", "a", 1.0),
            s_with("g1", "b", 1.0),
            s_with("g1", "c", 2.0),
            s_with("g2", "a", 1.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::CountValues,
                Some(&Grouping::By(vec!["group".to_string()])),
                None,
                Some("v"),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 3);
    }

    #[test]
    fn count_values_nan_bucket() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", f64::NAN),
            s("foo", "b", f64::NAN),
            s("foo", "c", 1.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::CountValues,
                None,
                None,
                Some("v"),
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        let nan_count = v
            .iter()
            .find(|s| s.labels.iter().any(|(n, vl)| n == "v" && vl == "NaN"))
            .map(|s| s.values[0])
            .unwrap();
        assert_eq!(nan_count, 2.0);
    }

    // -----------------------------------------------------------------
    // SOW-0034: stddev / stdvar / limitk / limit_ratio / group
    // -----------------------------------------------------------------

    #[test]
    fn stddev_population_of_known_set() {
        // Population variance of {1, 2, 3, 4, 5}: mean = 3, var = 2,
        // stddev = sqrt(2) ≈ 1.41421356.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
            s("foo", "e", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Stddev, None, None, None, input).unwrap());
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - (2.0_f64).sqrt()).abs() < 1e-9, "got {}", v[0].values[0]);
        assert!(v[0].labels.is_empty());
    }

    #[test]
    fn stdvar_skips_nan_inputs() {
        // {1, NaN, 3, 5}; non-NaN count=3, mean=3, var = (4+0+4)/3 ≈ 2.6667.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", f64::NAN),
            s("foo", "c", 3.0),
            s("foo", "d", 5.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Stdvar, None, None, None, input).unwrap());
        assert_eq!(v.len(), 1);
        let expected = (16.0 / 3.0) / 1.0 - 9.0; // sum_sq/n - mean^2 = 35/3 - 9 = 8/3
        // sum_sq = 1 + 9 + 25 = 35; mean = 9/3 = 3; var = 35/3 - 9 = 8/3 ≈ 2.6667.
        let _ = expected;
        assert!((v[0].values[0] - 8.0 / 3.0).abs() < 1e-9, "got {}", v[0].values[0]);
    }

    #[test]
    fn stddev_by_grouping_per_bucket() {
        let input = EvalResult::InstantVector(vec![
            s_with("g1", "a", 1.0),
            s_with("g1", "b", 2.0),
            s_with("g1", "c", 3.0),
            s_with("g2", "a", 10.0),
            s_with("g2", "b", 20.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Stdvar,
                Some(&Grouping::By(vec!["group".to_string()])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        // g1: mean=2, var=2/3; g2: mean=15, var=25.
        let mut vals: Vec<f64> = v.iter().map(|s| s.values[0]).collect();
        vals.sort_by(|a, b| a.partial_cmp(b).unwrap());
        assert!((vals[0] - 2.0 / 3.0).abs() < 1e-9);
        assert!((vals[1] - 25.0).abs() < 1e-9);
    }

    #[test]
    fn group_emits_one_per_bucket() {
        // No grouping -> one output series whose value is 1.0.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 5.0),
            s("foo", "c", 99.0),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Group, None, None, None, input).unwrap());
        assert_eq!(v.len(), 1);
        assert_eq!(v[0].values[0], 1.0);
        assert!(v[0].labels.is_empty());
    }

    #[test]
    fn group_by_dimension_keeps_one_per_dimension() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "a", 2.0),
            s("foo", "b", 3.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Group,
                Some(&Grouping::By(vec!["dimension".to_string()])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
        for s in &v {
            assert_eq!(s.values[0], 1.0);
        }
    }

    #[test]
    fn group_drops_all_nan_bucket() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", f64::NAN),
            s("foo", "b", f64::NAN),
        ]);
        let v = unwrap_vec(apply_aggregate(AggrKind::Group, None, None, None, input).unwrap());
        assert!(v.is_empty());
    }

    #[test]
    fn limitk_picks_first_k_in_signature_order() {
        // Stable selection: limitk(2, ...) over 4 input series keeps 2.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitK, None, Some(2.0), None, input).unwrap(),
        );
        assert_eq!(v.len(), 2);
        // __name__ stripped on output (per topk/bottomk convention).
        for s in &v {
            assert!(s.labels.iter().all(|(n, _)| n != "__name__"));
        }
    }

    #[test]
    fn limitk_with_k_zero_returns_empty() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 1.0)]);
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitK, None, Some(0.0), None, input.clone()).unwrap(),
        );
        assert!(v.is_empty());
        // Negative k -> empty too.
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitK, None, Some(-3.0), None, input).unwrap(),
        );
        assert!(v.is_empty());
    }

    #[test]
    fn limitk_by_grouping_per_bucket() {
        // 4 series across 2 groups -> limitk(1) per group keeps 2 total.
        let input = EvalResult::InstantVector(vec![
            s_with("g1", "a", 1.0),
            s_with("g1", "b", 2.0),
            s_with("g2", "a", 10.0),
            s_with("g2", "b", 20.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::LimitK,
                Some(&Grouping::By(vec!["group".to_string()])),
                Some(1.0),
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 2);
    }

    #[test]
    fn limit_ratio_one_keeps_all() {
        // ratio=1 means every series whose hash < 1, i.e. all of them.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitRatio, None, Some(1.0), None, input).unwrap(),
        );
        assert_eq!(v.len(), 3);
    }

    #[test]
    fn limit_ratio_zero_returns_empty() {
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitRatio, None, Some(0.0), None, input.clone()).unwrap(),
        );
        assert!(v.is_empty());
        // NaN ratio -> empty.
        let v = unwrap_vec(
            apply_aggregate(AggrKind::LimitRatio, None, Some(f64::NAN), None, input).unwrap(),
        );
        assert!(v.is_empty());
    }

    // -----------------------------------------------------------------
    // SOW-0037: explicit `by (__name__, ...)` keeps __name__
    // -----------------------------------------------------------------

    #[test]
    fn sum_by_explicit_name_keeps_name() {
        // Two series with the same __name__ but different dimensions.
        // `sum by (__name__)` should collapse them into one bucket
        // keyed by __name__, preserving "foo" on the output.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Sum,
                Some(&Grouping::By(vec!["__name__".to_string()])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - 3.0).abs() < 1e-9);
        let name = v[0]
            .labels
            .iter()
            .find(|(n, _)| n == "__name__")
            .map(|(_, v)| v.as_str());
        assert_eq!(name, Some("foo"));
    }

    #[test]
    fn sum_by_explicit_name_and_dim_keeps_both() {
        let input = EvalResult::InstantVector(vec![s("foo", "a", 5.0)]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Sum,
                Some(&Grouping::By(vec![
                    "__name__".to_string(),
                    "dimension".to_string(),
                ])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 1);
        assert!(v[0]
            .labels
            .iter()
            .any(|(n, v)| n == "__name__" && v == "foo"));
        assert!(v[0]
            .labels
            .iter()
            .any(|(n, v)| n == "dimension" && v == "a"));
    }

    #[test]
    fn sum_without_dim_keeps_name_by_default() {
        // `sum without (dimension)` drops `dimension` and keeps
        // everything else -- including `__name__` (since the user
        // didn't list it in `without`).
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Sum,
                Some(&Grouping::Without(vec!["dimension".to_string()])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 1);
        assert_eq!(v[0].values[0], 3.0);
        assert!(v[0]
            .labels
            .iter()
            .any(|(n, v)| n == "__name__" && v == "foo"));
    }

    #[test]
    fn sum_without_explicit_name_drops_name() {
        // `sum without (__name__)` drops `__name__` and keeps
        // everything else.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "a", 4.0),
        ]);
        let v = unwrap_vec(
            apply_aggregate(
                AggrKind::Sum,
                Some(&Grouping::Without(vec!["__name__".to_string()])),
                None,
                None,
                input,
            )
            .unwrap(),
        );
        assert_eq!(v.len(), 1);
        assert!((v[0].values[0] - 5.0).abs() < 1e-9);
        assert!(v[0].labels.iter().all(|(n, _)| n != "__name__"));
        assert!(v[0]
            .labels
            .iter()
            .any(|(n, v)| n == "dimension" && v == "a"));
    }

    #[test]
    fn limit_ratio_complementary_negatives_sum_to_all() {
        // For any input, limit_ratio(r, v) and limit_ratio(r-1, v) together
        // cover the full input (no overlap, no gap). For r=0.4: positive
        // picks `hash < 0.4`, negative picks `hash >= 1 + (0.4 - 1) = 0.4`.
        // Concatenating yields every input series exactly once.
        let input = EvalResult::InstantVector(vec![
            s("foo", "a", 1.0),
            s("foo", "b", 2.0),
            s("foo", "c", 3.0),
            s("foo", "d", 4.0),
            s("foo", "e", 5.0),
        ]);
        let n_input = if let EvalResult::InstantVector(v) = &input { v.len() } else { 0 };
        let pos = unwrap_vec(
            apply_aggregate(AggrKind::LimitRatio, None, Some(0.4), None, input.clone())
                .unwrap(),
        );
        let neg = unwrap_vec(
            apply_aggregate(AggrKind::LimitRatio, None, Some(-0.6), None, input).unwrap(),
        );
        assert_eq!(pos.len() + neg.len(), n_input);
    }
}
