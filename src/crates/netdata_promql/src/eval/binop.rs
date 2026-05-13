// SPDX-License-Identifier: GPL-3.0-or-later
//
// Binary operators: arithmetic and comparison across scalar/vector
// combinations. Phase 1 supports one-to-one vector matching by signature
// only; group_left/group_right/on/ignoring with non-default cardinality
// are rejected by the lowering pass and never reach this code path.

use crate::plan::{BinopKind, ValueType};

use super::types::{EvalError, EvalResult, Sample, Series};

/// Apply a binary operator. `return_bool` toggles the Prometheus `bool`
/// modifier on comparison operators: when set, comparisons emit 0/1 rather
/// than filtering.
pub fn apply_binop(
    op: BinopKind,
    return_bool: bool,
    lhs: EvalResult,
    rhs: EvalResult,
) -> Result<EvalResult, EvalError> {
    use EvalResult::*;
    match (lhs, rhs) {
        (Scalar(a), Scalar(b)) => Ok(scalar_binop(op, a, b, return_bool)),
        (Scalar(a), InstantVector(v)) => Ok(scalar_vec(op, a, v, /* scalar_on_left */ true, return_bool)),
        (InstantVector(v), Scalar(b)) => Ok(scalar_vec(op, b, v, /* scalar_on_left */ false, return_bool)),
        (InstantVector(a), InstantVector(b)) => Ok(vec_vec(op, a, b, return_bool)),
        (l, r) => Err(EvalError::Type {
            context: "binary op",
            expected: ValueType::InstantVector,
            got: if l.value_type() != ValueType::InstantVector
                && l.value_type() != ValueType::Scalar
            {
                l.value_type()
            } else {
                r.value_type()
            },
        }),
    }
}

/// Scalar-on-scalar.
fn scalar_binop(op: BinopKind, a: f64, b: f64, return_bool: bool) -> EvalResult {
    EvalResult::Scalar(apply_op_or_bool(op, a, b, return_bool, /*compare_yields_present=*/true))
}

/// scalar `op` vector. Per Prometheus semantics, the result preserves the
/// vector's labels and applies the op element-wise.
fn scalar_vec(op: BinopKind, scalar: f64, vec: Vec<Series>, scalar_on_left: bool, return_bool: bool) -> EvalResult {
    let mut out = Vec::with_capacity(vec.len());
    'series: for mut s in vec.into_iter() {
        for sample in &mut s.samples {
            let (lhs, rhs) = if scalar_on_left {
                (scalar, sample.value)
            } else {
                (sample.value, scalar)
            };
            if op.is_comparison() && !return_bool {
                // Filter mode: drop the sample if the comparison is false.
                if !cmp(op, lhs, rhs) {
                    continue 'series;
                }
                // Keep the original LHS value (Prometheus rule).
                sample.value = if scalar_on_left { rhs } else { lhs };
            } else {
                sample.value = apply_op_or_bool(op, lhs, rhs, return_bool, true);
            }
        }
        if op.is_comparison() && !return_bool {
            // Drop measurement labels for comparison-as-filter: spec says
            // the metric name (__name__) is preserved but result labels
            // match the LHS; for scalar-vs-vector that's the vector's set.
            // No change required here -- the labels are already the vector's.
        }
        out.push(s);
    }
    EvalResult::InstantVector(out)
}

/// vector op vector, one-to-one matched by full label set (signature).
fn vec_vec(op: BinopKind, lhs: Vec<Series>, rhs: Vec<Series>, return_bool: bool) -> EvalResult {
    use std::collections::HashMap;
    let mut by_sig: HashMap<u64, &Series> = HashMap::with_capacity(rhs.len());
    for s in &rhs {
        by_sig.insert(s.signature, s);
    }
    let mut out = Vec::new();
    for lhs_series in lhs.into_iter() {
        let Some(rhs_series) = by_sig.get(&lhs_series.signature) else {
            continue;
        };
        // One-to-one: pair samples by index. The selectors only produce
        // one sample per series (instant vectors), so this is the common
        // case. Pad to the shorter length.
        let n = lhs_series.samples.len().min(rhs_series.samples.len());
        let mut samples = Vec::with_capacity(n);
        let mut keep = false;
        for i in 0..n {
            let a = lhs_series.samples[i].value;
            let b = rhs_series.samples[i].value;
            let ts = lhs_series.samples[i].timestamp_ms;
            if op.is_comparison() && !return_bool {
                if !cmp(op, a, b) {
                    continue;
                }
                samples.push(Sample {
                    timestamp_ms: ts,
                    value: a,
                });
                keep = true;
            } else {
                samples.push(Sample {
                    timestamp_ms: ts,
                    value: apply_op_or_bool(op, a, b, return_bool, true),
                });
                keep = true;
            }
        }
        if keep {
            let mut s = Series {
                labels: lhs_series.labels.clone(),
                signature: lhs_series.signature,
                samples,
            };
            // Prometheus: arithmetic between two vectors drops __name__.
            if !op.is_comparison() {
                s.labels.retain(|(n, _)| n != "__name__");
                s.signature = super::types::labels_signature(&s.labels);
            }
            out.push(s);
        }
    }
    out.sort_by_key(|s| s.signature);
    EvalResult::InstantVector(out)
}

fn apply_op_or_bool(op: BinopKind, a: f64, b: f64, return_bool: bool, _present: bool) -> f64 {
    if op.is_comparison() {
        let r = cmp(op, a, b);
        if return_bool {
            if r { 1.0 } else { 0.0 }
        } else {
            // In bool-mode-off, comparisons act as filters; this function is
            // only reached in bool-on or scalar/scalar paths. For
            // scalar/scalar with bool off, Prometheus' behavior is "if the
            // comparison holds, return the LHS; otherwise the result is
            // NaN" -- we follow that.
            if r { a } else { f64::NAN }
        }
    } else {
        arith(op, a, b)
    }
}

fn arith(op: BinopKind, a: f64, b: f64) -> f64 {
    match op {
        BinopKind::Add => a + b,
        BinopKind::Sub => a - b,
        BinopKind::Mul => a * b,
        BinopKind::Div => a / b,
        BinopKind::Mod => a % b,
        BinopKind::Pow => a.powf(b),
        _ => f64::NAN,
    }
}

fn cmp(op: BinopKind, a: f64, b: f64) -> bool {
    match op {
        BinopKind::Eq => a == b,
        BinopKind::Ne => a != b,
        BinopKind::Lt => a < b,
        BinopKind::Le => a <= b,
        BinopKind::Gt => a > b,
        BinopKind::Ge => a >= b,
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn instant(label: &str, val: f64) -> Series {
        let labels = vec![("__name__".to_string(), label.to_string())];
        let signature = crate::eval::types::labels_signature(&labels);
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
    fn scalar_add() {
        let r = apply_binop(BinopKind::Add, false, EvalResult::Scalar(2.0), EvalResult::Scalar(3.0)).unwrap();
        assert!(matches!(r, EvalResult::Scalar(v) if (v - 5.0).abs() < 1e-9));
    }

    #[test]
    fn scalar_gt_with_bool() {
        let r = apply_binop(BinopKind::Gt, true, EvalResult::Scalar(5.0), EvalResult::Scalar(3.0)).unwrap();
        assert!(matches!(r, EvalResult::Scalar(v) if (v - 1.0).abs() < 1e-9));
    }

    #[test]
    fn vector_times_scalar() {
        let v = EvalResult::InstantVector(vec![instant("foo", 4.0)]);
        let r = apply_binop(BinopKind::Mul, false, v, EvalResult::Scalar(2.0)).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert!((vs[0].samples[0].value - 8.0).abs() < 1e-9);
            }
            _ => panic!("expected instant vector"),
        }
    }

    #[test]
    fn vector_gt_scalar_filters() {
        let v = EvalResult::InstantVector(vec![instant("foo", 4.0), instant("bar", 1.0)]);
        let r = apply_binop(BinopKind::Gt, false, v, EvalResult::Scalar(2.0)).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert_eq!(vs[0].get("__name__"), Some("foo"));
            }
            _ => panic!("expected instant vector"),
        }
    }
}
