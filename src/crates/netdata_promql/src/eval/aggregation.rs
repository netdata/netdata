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
pub fn apply_aggregate(
    op: AggrKind,
    grouping: Option<&Grouping>,
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
        let r = apply_aggregate(AggrKind::Sum, None, input).unwrap();
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
        let r = apply_aggregate(AggrKind::Count, None, input).unwrap();
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
        let avg = apply_aggregate(AggrKind::Avg, None, input.clone()).unwrap();
        let min = apply_aggregate(AggrKind::Min, None, input.clone()).unwrap();
        let max = apply_aggregate(AggrKind::Max, None, input).unwrap();
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
}
