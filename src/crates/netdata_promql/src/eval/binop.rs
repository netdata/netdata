// SPDX-License-Identifier: GPL-3.0-or-later
//
// Binary operators: arithmetic, comparison, and set operators across
// scalar/vector combinations, with full PromQL vector matching.
//
// Vector matching (Phase 3d, SOW-0022) follows the Prometheus model:
// each lhs/rhs pair gets paired by a *matching key* projected from
// their labels per the `MatchSpec`. The default key is "all labels
// except __name__". `on(labels)` restricts to the listed labels;
// `ignoring(labels)` drops those (and __name__). Cardinality selects
// the join strategy: 1:1 errors on duplicate keys; M:1 (group_left)
// indexes rhs by key and pairs each lhs match; 1:M is symmetric.
// Set operators (and/or/unless) reuse the same key projection.

use std::collections::HashMap;

use crate::plan::{BinopKind, Cardinality, MatchKeys, MatchSpec, ValueType};

use super::types::{labels_signature, EvalError, EvalResult, Series};

/// Apply a binary operator. `return_bool` toggles the Prometheus `bool`
/// modifier on comparison operators. `matching` is `Some` for any binop
/// that involves a vector and carries the vector-matching spec; for
/// scalar/scalar it can be `None`.
pub fn apply_binop(
    op: BinopKind,
    return_bool: bool,
    matching: Option<&MatchSpec>,
    lhs: EvalResult,
    rhs: EvalResult,
) -> Result<EvalResult, EvalError> {
    use EvalResult::*;
    match (lhs, rhs) {
        (Scalar(a), Scalar(b)) => {
            if op.is_set_op() {
                return Err(EvalError::Type {
                    context: "set operator operands",
                    expected: ValueType::InstantVector,
                    got: ValueType::Scalar,
                });
            }
            Ok(scalar_binop(op, a, b, return_bool))
        }
        (Scalar(a), InstantVector(v)) => {
            if op.is_set_op() {
                return Err(EvalError::Type {
                    context: "set operator lhs",
                    expected: ValueType::InstantVector,
                    got: ValueType::Scalar,
                });
            }
            Ok(scalar_vec(op, a, v, true, return_bool))
        }
        (InstantVector(v), Scalar(b)) => {
            if op.is_set_op() {
                return Err(EvalError::Type {
                    context: "set operator rhs",
                    expected: ValueType::InstantVector,
                    got: ValueType::Scalar,
                });
            }
            Ok(scalar_vec(op, b, v, false, return_bool))
        }
        (InstantVector(a), InstantVector(b)) => {
            let default_spec = MatchSpec::default();
            let m = matching.unwrap_or(&default_spec);
            if op.is_set_op() {
                Ok(set_op(op, m, a, b))
            } else {
                vec_vec(op, return_bool, m, a, b)
            }
        }
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

// ---------------------------------------------------------------------------
// Matching-key projection
// ---------------------------------------------------------------------------

/// Project a series's labels onto the matching key. The output is a
/// fresh `Vec` rather than a slice because On/Ignoring may reorder or
/// filter; signature of the projection (FNV-1a over the sorted key
/// labels) is the join token.
fn project_key_labels(labels: &[(String, String)], keys: &MatchKeys) -> Vec<(String, String)> {
    match keys {
        MatchKeys::Default => labels
            .iter()
            .filter(|(n, _)| n != "__name__")
            .cloned()
            .collect(),
        MatchKeys::On(names) => labels
            .iter()
            .filter(|(n, _)| names.iter().any(|k| k == n))
            .cloned()
            .collect(),
        MatchKeys::Ignoring(names) => labels
            .iter()
            .filter(|(n, _)| n != "__name__" && !names.iter().any(|k| k == n))
            .cloned()
            .collect(),
    }
}

fn match_signature(labels: &[(String, String)], keys: &MatchKeys) -> u64 {
    let mut key = project_key_labels(labels, keys);
    key.sort_by(|a, b| a.0.cmp(&b.0));
    labels_signature(&key)
}

// ---------------------------------------------------------------------------
// Scalar paths (unchanged from Phase 1)
// ---------------------------------------------------------------------------

fn scalar_binop(op: BinopKind, a: f64, b: f64, return_bool: bool) -> EvalResult {
    EvalResult::Scalar(apply_op_or_bool(op, a, b, return_bool))
}

fn scalar_vec(
    op: BinopKind,
    scalar: f64,
    vec: Vec<Series>,
    scalar_on_left: bool,
    return_bool: bool,
) -> EvalResult {
    let mut out = Vec::with_capacity(vec.len());
    for mut s in vec.into_iter() {
        let mut kept_any = false;
        for v in s.values.iter_mut() {
            let (lhs, rhs) = if scalar_on_left {
                (scalar, *v)
            } else {
                (*v, scalar)
            };
            if op.is_comparison() && !return_bool {
                if !cmp(op, lhs, rhs) {
                    *v = f64::NAN;
                } else {
                    *v = if scalar_on_left { rhs } else { lhs };
                    kept_any = true;
                }
            } else {
                *v = apply_op_or_bool(op, lhs, rhs, return_bool);
                kept_any = true;
            }
        }
        // Comparison-as-filter that hits at no grid position drops
        // the series. Arithmetic and bool comparison always keep.
        if op.is_comparison() && !return_bool && !kept_any {
            continue;
        }
        out.push(s);
    }
    EvalResult::InstantVector(out)
}

// ---------------------------------------------------------------------------
// vector/vector arithmetic and comparison
// ---------------------------------------------------------------------------

fn vec_vec(
    op: BinopKind,
    return_bool: bool,
    spec: &MatchSpec,
    lhs: Vec<Series>,
    rhs: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    match spec.cardinality {
        Cardinality::OneToOne => vec_vec_one_to_one(op, return_bool, &spec.keys, lhs, rhs),
        Cardinality::ManyToOne => {
            // rhs is the "one" side: index by key, then pair each lhs.
            vec_vec_many_to_one(op, return_bool, spec, lhs, rhs, /*one_on_right=*/ true)
        }
        Cardinality::OneToMany => {
            // lhs is the "one" side: index by key, then pair each rhs.
            vec_vec_many_to_one(op, return_bool, spec, rhs, lhs, /*one_on_right=*/ false)
        }
    }
}

fn vec_vec_one_to_one(
    op: BinopKind,
    return_bool: bool,
    keys: &MatchKeys,
    lhs: Vec<Series>,
    rhs: Vec<Series>,
) -> Result<EvalResult, EvalError> {
    let mut rhs_by_key: HashMap<u64, &Series> = HashMap::with_capacity(rhs.len());
    for s in &rhs {
        let key = match_signature(&s.labels, keys);
        if rhs_by_key.insert(key, s).is_some() {
            return Err(EvalError::Other(format!(
                "found duplicate series on the right hand side of the operation for the match group"
            )));
        }
    }

    let mut seen_lhs_keys: HashMap<u64, ()> = HashMap::new();
    let mut out = Vec::new();
    for lhs_series in lhs.into_iter() {
        let key = match_signature(&lhs_series.labels, keys);
        if seen_lhs_keys.insert(key, ()).is_some() {
            return Err(EvalError::Other(format!(
                "found duplicate series on the left hand side of the operation for the match group"
            )));
        }
        let Some(rhs_series) = rhs_by_key.get(&key) else {
            continue;
        };
        if let Some(series) = combine_pair(op, return_bool, lhs_series, rhs_series, &[]) {
            out.push(series);
        }
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Many-to-one join. The "one" side is indexed by matching key; the
/// "many" side iterates. `one_on_right == true` is group_left (rhs is
/// the one side). Output labels come from the "many" side (with
/// `__name__` dropped for arithmetic) plus any `include` labels copied
/// from the "one" side. Operator order is preserved per the source
/// query: `lhs OP rhs` regardless of which side is the one.
fn vec_vec_many_to_one(
    op: BinopKind,
    return_bool: bool,
    spec: &MatchSpec,
    many: Vec<Series>,
    one: Vec<Series>,
    one_on_right: bool,
) -> Result<EvalResult, EvalError> {
    let mut one_by_key: HashMap<u64, &Series> = HashMap::with_capacity(one.len());
    for s in &one {
        let key = match_signature(&s.labels, &spec.keys);
        if one_by_key.insert(key, s).is_some() {
            return Err(EvalError::Other(
                "found duplicate series on the \"one\" side of a many-to-one match".to_string(),
            ));
        }
    }

    let mut out = Vec::new();
    for many_series in many.into_iter() {
        let key = match_signature(&many_series.labels, &spec.keys);
        let Some(one_series) = one_by_key.get(&key) else {
            continue;
        };
        // Compute values in operator positional order (lhs first).
        // group_left: many=lhs, one=rhs; group_right: one=lhs, many=rhs.
        let (lhs_values, rhs_values) = if one_on_right {
            (&many_series.values, &one_series.values)
        } else {
            (&one_series.values, &many_series.values)
        };
        let Some(values) = pair_values(op, return_bool, lhs_values, rhs_values) else {
            continue;
        };
        // Label source: always the many side; include labels from the
        // one side. Then strip __name__ for arithmetic (comparison-as-
        // filter keeps it).
        let mut labels = many_series.labels.clone();
        if !op.is_comparison() {
            labels.retain(|(n, _)| n != "__name__");
        }
        for inc in &spec.include {
            if let Some((_, v)) = one_series.labels.iter().find(|(n, _)| n == inc) {
                if let Some(slot) = labels.iter_mut().find(|(n, _)| n == inc) {
                    slot.1 = v.clone();
                } else {
                    labels.push((inc.clone(), v.clone()));
                }
            }
        }
        // Output series inherits the many-side's timestamps Arc.
        let timestamps = std::sync::Arc::clone(&many_series.timestamps);
        out.push(Series::new(labels, timestamps, values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::InstantVector(out))
}

/// Apply the binop element-wise to two value vectors, returning either
/// the resulting `Vec<f64>` (grid-aligned, NaN where comparison-as-
/// filter rejects) or `None` if every position fell out under
/// comparison-as-filter semantics. Arithmetic always returns `Some`.
fn pair_values(
    op: BinopKind,
    return_bool: bool,
    lhs: &[f64],
    rhs: &[f64],
) -> Option<Vec<f64>> {
    let n = lhs.len().min(rhs.len());
    let mut values = Vec::with_capacity(n);
    let mut kept_any = false;
    for i in 0..n {
        let a = lhs[i];
        let b = rhs[i];
        if op.is_comparison() && !return_bool {
            if cmp(op, a, b) {
                values.push(a);
                kept_any = true;
            } else {
                values.push(f64::NAN);
            }
        } else {
            values.push(apply_op_or_bool(op, a, b, return_bool));
            kept_any = true;
        }
    }
    if kept_any {
        Some(values)
    } else {
        None
    }
}

/// Build a single output series from a matched lhs/rhs pair. Returns
/// `None` for the comparison-as-filter case when no grid position kept
/// a value. `include` is the `group_left(...)` / `group_right(...)`
/// label list; these labels are copied from `rhs_series` onto the
/// result. The output series shares the lhs's timestamps Arc.
fn combine_pair(
    op: BinopKind,
    return_bool: bool,
    lhs_series: Series,
    rhs_series: &Series,
    include: &[String],
) -> Option<Series> {
    let values = pair_values(op, return_bool, &lhs_series.values, &rhs_series.values)?;

    // Result labels: start from lhs, drop __name__ for arithmetic,
    // then add include labels from rhs.
    let mut labels = lhs_series.labels;
    if !op.is_comparison() {
        labels.retain(|(n, _)| n != "__name__");
    }
    for inc in include {
        if let Some((_, v)) = rhs_series.labels.iter().find(|(n, _)| n == inc) {
            if let Some(slot) = labels.iter_mut().find(|(n, _)| n == inc) {
                slot.1 = v.clone();
            } else {
                labels.push((inc.clone(), v.clone()));
            }
        }
    }
    Some(Series::new(labels, lhs_series.timestamps, values))
}

// ---------------------------------------------------------------------------
// Set operators
// ---------------------------------------------------------------------------

fn set_op(op: BinopKind, spec: &MatchSpec, lhs: Vec<Series>, rhs: Vec<Series>) -> EvalResult {
    let mut rhs_keys: HashMap<u64, ()> = HashMap::with_capacity(rhs.len());
    for s in &rhs {
        rhs_keys.insert(match_signature(&s.labels, &spec.keys), ());
    }

    let mut out = Vec::new();
    match op {
        BinopKind::LAnd => {
            for s in lhs.into_iter() {
                let key = match_signature(&s.labels, &spec.keys);
                if rhs_keys.contains_key(&key) {
                    out.push(s);
                }
            }
        }
        BinopKind::LUnless => {
            for s in lhs.into_iter() {
                let key = match_signature(&s.labels, &spec.keys);
                if !rhs_keys.contains_key(&key) {
                    out.push(s);
                }
            }
        }
        BinopKind::LOr => {
            // All lhs series, plus any rhs series whose key is not
            // already present on the lhs.
            let mut lhs_keys: HashMap<u64, ()> = HashMap::with_capacity(lhs.len());
            for s in &lhs {
                lhs_keys.insert(match_signature(&s.labels, &spec.keys), ());
            }
            out.extend(lhs.into_iter());
            for s in rhs.into_iter() {
                let key = match_signature(&s.labels, &spec.keys);
                if !lhs_keys.contains_key(&key) {
                    out.push(s);
                }
            }
        }
        _ => {
            // Caller routes only set operators here.
            unreachable!("non-set-op in set_op: {:?}", op);
        }
    }
    out.sort_by_key(|s| s.signature);
    EvalResult::InstantVector(out)
}

// ---------------------------------------------------------------------------
// Element-wise math
// ---------------------------------------------------------------------------

fn apply_op_or_bool(op: BinopKind, a: f64, b: f64, return_bool: bool) -> f64 {
    if op.is_comparison() {
        let r = cmp(op, a, b);
        if return_bool {
            if r {
                1.0
            } else {
                0.0
            }
        } else if r {
            a
        } else {
            f64::NAN
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
        Series::scalar(labels, 1000, val)
    }

    fn s_with(labels: Vec<(&str, &str)>, val: f64) -> Series {
        let owned: Vec<(String, String)> = labels
            .into_iter()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        Series::scalar(owned, 1000, val)
    }

    #[test]
    fn scalar_add() {
        let r = apply_binop(
            BinopKind::Add,
            false,
            None,
            EvalResult::Scalar(2.0),
            EvalResult::Scalar(3.0),
        )
        .unwrap();
        assert!(matches!(r, EvalResult::Scalar(v) if (v - 5.0).abs() < 1e-9));
    }

    #[test]
    fn scalar_gt_with_bool() {
        let r = apply_binop(
            BinopKind::Gt,
            true,
            None,
            EvalResult::Scalar(5.0),
            EvalResult::Scalar(3.0),
        )
        .unwrap();
        assert!(matches!(r, EvalResult::Scalar(v) if (v - 1.0).abs() < 1e-9));
    }

    #[test]
    fn vector_times_scalar() {
        let v = EvalResult::InstantVector(vec![instant("foo", 4.0)]);
        let spec = MatchSpec::default();
        let r = apply_binop(BinopKind::Mul, false, Some(&spec), v, EvalResult::Scalar(2.0)).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert!((vs[0].values[0] - 8.0).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn vector_gt_scalar_filters() {
        let v = EvalResult::InstantVector(vec![instant("foo", 4.0), instant("bar", 1.0)]);
        let spec = MatchSpec::default();
        let r = apply_binop(BinopKind::Gt, false, Some(&spec), v, EvalResult::Scalar(2.0)).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert_eq!(vs[0].labels.iter().find(|(n, _)| n == "__name__").map(|(_, v)| v.as_str()), Some("foo"));
            }
            _ => panic!(),
        }
    }

    // ---------------------------------------------------------------
    // Phase 3d: vector matching
    // ---------------------------------------------------------------

    fn default_spec() -> MatchSpec {
        MatchSpec::default()
    }

    fn on_spec(labels: &[&str]) -> MatchSpec {
        MatchSpec {
            keys: MatchKeys::On(labels.iter().map(|s| s.to_string()).collect()),
            cardinality: Cardinality::OneToOne,
            include: Vec::new(),
        }
    }

    fn ignoring_spec(labels: &[&str]) -> MatchSpec {
        MatchSpec {
            keys: MatchKeys::Ignoring(labels.iter().map(|s| s.to_string()).collect()),
            cardinality: Cardinality::OneToOne,
            include: Vec::new(),
        }
    }

    #[test]
    fn default_matching_drops_name_so_different_metrics_match() {
        // a = {__name__="a", x="1"} value=2
        // b = {__name__="b", x="1"} value=3
        // Default matching joins by x="1"; a+b should produce one series.
        let lhs = EvalResult::InstantVector(vec![s_with(vec![("__name__", "a"), ("x", "1")], 2.0)]);
        let rhs = EvalResult::InstantVector(vec![s_with(vec![("__name__", "b"), ("x", "1")], 3.0)]);
        let r = apply_binop(BinopKind::Add, false, Some(&default_spec()), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert!((vs[0].values[0] - 5.0).abs() < 1e-9);
                // Arithmetic drops __name__.
                assert!(vs[0].labels.iter().all(|(n, _)| n != "__name__"));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn on_matches_only_listed_labels() {
        // Match only on `host`. The metric `region` label differs but
        // shouldn't matter.
        let lhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "a"), ("host", "h1"), ("region", "us")],
            2.0,
        )]);
        let rhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "b"), ("host", "h1"), ("region", "eu")],
            3.0,
        )]);
        let r = apply_binop(BinopKind::Add, false, Some(&on_spec(&["host"])), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert!((vs[0].values[0] - 5.0).abs() < 1e-9);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn ignoring_drops_listed_labels_from_key() {
        // Match by everything except `region`; lhs and rhs have
        // matching host but different region -> still match.
        let lhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "a"), ("host", "h1"), ("region", "us")],
            2.0,
        )]);
        let rhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "b"), ("host", "h1"), ("region", "eu")],
            3.0,
        )]);
        let r = apply_binop(
            BinopKind::Add,
            false,
            Some(&ignoring_spec(&["region"])),
            lhs,
            rhs,
        )
        .unwrap();
        match r {
            EvalResult::InstantVector(vs) => assert_eq!(vs.len(), 1),
            _ => panic!(),
        }
    }

    #[test]
    fn one_to_one_rejects_duplicate_rhs() {
        // Two rhs series collapse to the same key under default
        // matching; 1:1 rejects.
        let lhs = EvalResult::InstantVector(vec![s_with(vec![("x", "1")], 1.0)]);
        let rhs = EvalResult::InstantVector(vec![
            s_with(vec![("__name__", "b1"), ("x", "1")], 2.0),
            s_with(vec![("__name__", "b2"), ("x", "1")], 3.0),
        ]);
        let r = apply_binop(BinopKind::Add, false, Some(&default_spec()), lhs, rhs);
        assert!(matches!(r, Err(EvalError::Other(_))));
    }

    #[test]
    fn group_left_many_to_one_pairs_correctly() {
        // lhs has two series sharing `region` value; rhs has one
        // series per region. group_left(meta) copies the `meta` label
        // from rhs to each lhs result.
        let lhs = EvalResult::InstantVector(vec![
            s_with(vec![("__name__", "req"), ("host", "h1"), ("region", "us")], 10.0),
            s_with(vec![("__name__", "req"), ("host", "h2"), ("region", "us")], 20.0),
        ]);
        let rhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "info"), ("region", "us"), ("meta", "x")],
            2.0,
        )]);
        let spec = MatchSpec {
            keys: MatchKeys::On(vec!["region".to_string()]),
            cardinality: Cardinality::ManyToOne,
            include: vec!["meta".to_string()],
        };
        let r = apply_binop(BinopKind::Mul, false, Some(&spec), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 2);
                for s in &vs {
                    // Each result carries the lhs `host` and the
                    // included `meta` from rhs.
                    assert!(s.labels.iter().any(|(n, _)| n == "host"));
                    let meta = s.labels.iter().find(|(n, _)| n == "meta");
                    assert!(meta.is_some(), "missing meta on {:?}", s.labels);
                    assert_eq!(meta.unwrap().1, "x");
                }
                let values: Vec<f64> = vs.iter().map(|s| s.values[0]).collect();
                assert!(values.contains(&20.0) && values.contains(&40.0));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn group_right_one_to_many_symmetric() {
        // Mirror of the group_left test: now the "one" side is on the
        // left, and the rhs is "many". `metric_count(rhs) * metric_one(lhs)`.
        let lhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "info"), ("region", "us"), ("meta", "x")],
            2.0,
        )]);
        let rhs = EvalResult::InstantVector(vec![
            s_with(vec![("__name__", "req"), ("host", "h1"), ("region", "us")], 10.0),
            s_with(vec![("__name__", "req"), ("host", "h2"), ("region", "us")], 20.0),
        ]);
        let spec = MatchSpec {
            keys: MatchKeys::On(vec!["region".to_string()]),
            cardinality: Cardinality::OneToMany,
            include: vec!["meta".to_string()],
        };
        let r = apply_binop(BinopKind::Mul, false, Some(&spec), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 2);
                for s in &vs {
                    assert!(s.labels.iter().any(|(n, _)| n == "host"));
                    assert!(s.labels.iter().any(|(n, _)| n == "meta"));
                }
            }
            _ => panic!(),
        }
    }

    #[test]
    fn comparison_as_filter_keeps_lhs_labels() {
        let lhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "a"), ("x", "1")],
            5.0,
        )]);
        let rhs = EvalResult::InstantVector(vec![s_with(vec![("x", "1")], 3.0)]);
        let r = apply_binop(BinopKind::Gt, false, Some(&default_spec()), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                // __name__ preserved on comparison-as-filter.
                assert!(vs[0].labels.iter().any(|(n, _)| n == "__name__"));
            }
            _ => panic!(),
        }
    }

    // ---------------------------------------------------------------
    // Set operators
    // ---------------------------------------------------------------

    #[test]
    fn and_keeps_lhs_series_present_in_rhs() {
        let lhs = EvalResult::InstantVector(vec![
            s_with(vec![("x", "1")], 1.0),
            s_with(vec![("x", "2")], 2.0),
            s_with(vec![("x", "3")], 3.0),
        ]);
        let rhs = EvalResult::InstantVector(vec![
            s_with(vec![("x", "1")], 100.0),
            s_with(vec![("x", "3")], 100.0),
        ]);
        let r = apply_binop(BinopKind::LAnd, false, Some(&default_spec()), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 2);
                let xs: Vec<String> = vs
                    .iter()
                    .map(|s| s.labels.iter().find(|(n, _)| n == "x").unwrap().1.clone())
                    .collect();
                assert!(xs.contains(&"1".to_string()) && xs.contains(&"3".to_string()));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn unless_drops_lhs_series_present_in_rhs() {
        let lhs = EvalResult::InstantVector(vec![
            s_with(vec![("x", "1")], 1.0),
            s_with(vec![("x", "2")], 2.0),
        ]);
        let rhs = EvalResult::InstantVector(vec![s_with(vec![("x", "1")], 100.0)]);
        let r = apply_binop(BinopKind::LUnless, false, Some(&default_spec()), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                assert_eq!(
                    vs[0].labels.iter().find(|(n, _)| n == "x").unwrap().1,
                    "2"
                );
            }
            _ => panic!(),
        }
    }

    #[test]
    fn or_unions_keys_preferring_lhs() {
        let lhs = EvalResult::InstantVector(vec![s_with(vec![("x", "1")], 1.0)]);
        let rhs = EvalResult::InstantVector(vec![
            s_with(vec![("x", "1")], 100.0),
            s_with(vec![("x", "2")], 200.0),
        ]);
        let r = apply_binop(BinopKind::LOr, false, Some(&default_spec()), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 2);
                // lhs's x=1 wins for the shared key.
                let one = vs
                    .iter()
                    .find(|s| s.labels.iter().any(|(n, v)| n == "x" && v == "1"))
                    .unwrap();
                assert_eq!(one.values[0], 1.0);
            }
            _ => panic!(),
        }
    }

    #[test]
    fn set_op_with_on_clause_uses_listed_labels() {
        let lhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "a"), ("host", "h1"), ("env", "prod")],
            1.0,
        )]);
        let rhs = EvalResult::InstantVector(vec![s_with(
            vec![("__name__", "b"), ("host", "h1"), ("env", "staging")],
            2.0,
        )]);
        let spec = on_spec(&["host"]);
        let r = apply_binop(BinopKind::LAnd, false, Some(&spec), lhs, rhs).unwrap();
        match r {
            EvalResult::InstantVector(vs) => {
                assert_eq!(vs.len(), 1);
                // The kept series is the lhs verbatim (and keeps all its
                // labels including env=prod).
                let env = vs[0]
                    .labels
                    .iter()
                    .find(|(n, _)| n == "env")
                    .unwrap()
                    .1
                    .as_str();
                assert_eq!(env, "prod");
            }
            _ => panic!(),
        }
    }

    #[test]
    fn set_op_rejects_scalar_operand() {
        let r = apply_binop(
            BinopKind::LAnd,
            false,
            Some(&default_spec()),
            EvalResult::Scalar(1.0),
            EvalResult::InstantVector(vec![s_with(vec![("x", "1")], 1.0)]),
        );
        assert!(matches!(r, Err(EvalError::Type { .. })));
    }
}
