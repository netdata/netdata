// SPDX-License-Identifier: GPL-3.0-or-later
//
// Lower a `promql_parser::parser::Expr` into our `Plan` IR.
//
// Lowering is a single recursive walk that resolves token-id operators
// into `BinopKind`/`AggrKind`, translates the parser's `Matcher` and
// `MatchOp` into our storage-layer matcher types (compiling regexes
// against the process-wide cache), and rejects type errors that the
// AST allows but our evaluator does not support.

use std::sync::Arc;
use std::time::Duration;

use promql_parser::label::{MatchOp as ParserMatchOp, Matcher as ParserMatcher, METRIC_NAME};
use promql_parser::parser::token;
use promql_parser::parser::{
    AggregateExpr, BinaryExpr, Call, Expr, LabelModifier, MatrixSelector, Offset, ParenExpr,
    UnaryExpr, VectorSelector,
};

use crate::storage::{compile_regex, Matcher};

use super::ir::{
    AggrKind, BinopKind, Cardinality, FuncKind, Grouping, MatchKeys, MatchSpec, Plan, ValueType,
};

#[derive(Debug, thiserror::Error)]
pub enum LowerError {
    #[error("parse error: {0}")]
    Parse(String),

    #[error("unsupported in Phase 1: {0}")]
    Unsupported(String),

    #[error("invalid matcher: {0}")]
    InvalidMatcher(String),

    #[error("type error: {context}: expected {expected:?}, got {got:?}")]
    Type {
        context: &'static str,
        expected: ValueType,
        got: ValueType,
    },

    #[error("bad aggregation: {0}")]
    Aggregation(String),

    #[error("bad function call: {0}")]
    Call(String),
}

/// Top-level entry: parse a query string and lower the AST to a Plan.
pub fn lower_query(query: &str) -> Result<Plan, LowerError> {
    let expr = promql_parser::parser::parse(query).map_err(LowerError::Parse)?;
    lower(&expr)
}

/// Lower an already-parsed expression.
pub fn lower(expr: &Expr) -> Result<Plan, LowerError> {
    match expr {
        Expr::NumberLiteral(n) => Ok(Plan::Number(n.val)),
        Expr::Paren(ParenExpr { expr }) => lower(expr),
        Expr::VectorSelector(vs) => lower_vector_select(vs),
        Expr::MatrixSelector(MatrixSelector { vs, range }) => lower_matrix_select(vs, *range),
        Expr::Unary(UnaryExpr { expr }) => lower_unary(expr),
        Expr::Binary(b) => lower_binary(b),
        Expr::Aggregate(a) => lower_aggregate(a),
        Expr::Call(c) => lower_call(c),
        Expr::Subquery(_) => Err(LowerError::Unsupported(
            "subqueries are Phase 2".to_string(),
        )),
        Expr::StringLiteral(_) => Err(LowerError::Unsupported(
            "top-level string literals are not supported".to_string(),
        )),
        Expr::Extension(_) => Err(LowerError::Unsupported(
            "AST extensions are not supported".to_string(),
        )),
    }
}

fn lower_vector_select(vs: &VectorSelector) -> Result<Plan, LowerError> {
    if vs.at.is_some() {
        return Err(LowerError::Unsupported(
            "@ modifiers are Phase 2".to_string(),
        ));
    }
    let matchers = lower_matchers(vs.name.as_deref(), &vs.matchers.matchers)?;
    let offset_ms = offset_to_ms(vs.offset.as_ref());
    Ok(Plan::VectorSelect {
        matchers,
        offset_ms,
    })
}

fn lower_matrix_select(vs: &VectorSelector, range: Duration) -> Result<Plan, LowerError> {
    if vs.at.is_some() {
        return Err(LowerError::Unsupported(
            "@ modifiers are Phase 2".to_string(),
        ));
    }
    let matchers = lower_matchers(vs.name.as_deref(), &vs.matchers.matchers)?;
    let offset_ms = offset_to_ms(vs.offset.as_ref());
    let range_ms = duration_to_ms(range);
    if range_ms <= 0 {
        return Err(LowerError::InvalidMatcher(
            "matrix selector range must be positive".to_string(),
        ));
    }
    Ok(Plan::MatrixSelect {
        matchers,
        range_ms,
        offset_ms,
    })
}

fn lower_unary(expr: &Expr) -> Result<Plan, LowerError> {
    let inner = lower(expr)?;
    match inner.value_type() {
        ValueType::Scalar | ValueType::InstantVector => Ok(Plan::UnaryMinus(Arc::new(inner))),
        other => Err(LowerError::Type {
            context: "unary minus operand",
            expected: ValueType::Scalar,
            got: other,
        }),
    }
}

fn lower_binary(b: &BinaryExpr) -> Result<Plan, LowerError> {
    let op = binop_from_token(b.op.id())?;
    let lhs = lower(&b.lhs)?;
    let rhs = lower(&b.rhs)?;

    // Type-check operands first. Set operators are vector/vector only.
    use ValueType::*;
    match (lhs.value_type(), rhs.value_type()) {
        (Scalar | InstantVector, Scalar | InstantVector) => {}
        (l, r) => {
            return Err(LowerError::Type {
                context: "binary operator",
                expected: InstantVector,
                got: if l == RangeVector { l } else { r },
            });
        }
    }
    if op.is_set_op()
        && (lhs.value_type() != InstantVector || rhs.value_type() != InstantVector)
    {
        return Err(LowerError::Type {
            context: "set operator",
            expected: InstantVector,
            got: if lhs.value_type() != InstantVector {
                lhs.value_type()
            } else {
                rhs.value_type()
            },
        });
    }

    // Translate the matching modifier, if any. Cases:
    //   * No modifier -> default 1:1 matching (matches by every label
    //     except __name__).
    //   * on(...) / ignoring(...) -> MatchKeys::On / Ignoring.
    //   * group_left / group_right -> Cardinality::ManyToOne /
    //     OneToMany with include labels.
    //   * ManyToMany cardinality -> reject (Prometheus rejects too).
    //   * Set operators reject group_left/group_right.
    let mut return_bool = false;
    let matching = if let Some(modifier) = &b.modifier {
        return_bool = modifier.return_bool;
        let keys = match &modifier.matching {
            None => MatchKeys::Default,
            Some(LabelModifier::Include(ls)) => MatchKeys::On(ls.labels.clone()),
            Some(LabelModifier::Exclude(ls)) => MatchKeys::Ignoring(ls.labels.clone()),
        };
        use promql_parser::parser::VectorMatchCardinality as VMC;
        let (cardinality, include) = match &modifier.card {
            VMC::OneToOne => (Cardinality::OneToOne, Vec::new()),
            VMC::ManyToOne(ls) => {
                if op.is_set_op() {
                    return Err(LowerError::Unsupported(format!(
                        "set operator {:?} does not accept group_left/group_right",
                        op
                    )));
                }
                (Cardinality::ManyToOne, ls.labels.clone())
            }
            VMC::OneToMany(ls) => {
                if op.is_set_op() {
                    return Err(LowerError::Unsupported(format!(
                        "set operator {:?} does not accept group_left/group_right",
                        op
                    )));
                }
                (Cardinality::OneToMany, ls.labels.clone())
            }
            VMC::ManyToMany => {
                // The parser tags set operators (and/or/unless) with
                // ManyToMany by default. For arithmetic and comparison,
                // this is the unrepresentable case and we reject it
                // (Prometheus rejects it too). For set operators, we
                // treat ManyToMany as the natural cardinality: the join
                // key still selects which series participate, but no
                // single-side de-duplication is enforced.
                if op.is_set_op() {
                    (Cardinality::OneToOne, Vec::new())
                } else {
                    return Err(LowerError::Unsupported(
                        "many-to-many cardinality is not supported for arithmetic/comparison \
                         (Prometheus rejects it too)"
                            .to_string(),
                    ));
                }
            }
        };
        Some(MatchSpec {
            keys,
            cardinality,
            include,
        })
    } else if matches!(lhs.value_type(), InstantVector)
        || matches!(rhs.value_type(), InstantVector)
    {
        // Vector binops always carry a matching spec, defaulted.
        Some(MatchSpec::default())
    } else {
        None
    };

    if return_bool && !op.is_comparison() {
        return Err(LowerError::Parse(
            "bool modifier only applies to comparison operators".to_string(),
        ));
    }

    Ok(Plan::Binop {
        op,
        return_bool,
        matching,
        lhs: Arc::new(lhs),
        rhs: Arc::new(rhs),
    })
}

fn lower_aggregate(a: &AggregateExpr) -> Result<Plan, LowerError> {
    let op = aggr_from_token(a.op.id())?;
    let expr = lower(&a.expr)?;
    if expr.value_type() != ValueType::InstantVector {
        return Err(LowerError::Type {
            context: "aggregation operand",
            expected: ValueType::InstantVector,
            got: expr.value_type(),
        });
    }

    // Parametrized aggregators (topk/bottomk/quantile) require a scalar
    // first argument; the others reject any parameter.
    let lowered_param = match (&a.param, op.takes_param()) {
        (Some(p), true) => {
            let lp = lower(p)?;
            if lp.value_type() != ValueType::Scalar {
                return Err(LowerError::Aggregation(format!(
                    "{:?} parameter must be a scalar; got {:?}",
                    op,
                    lp.value_type()
                )));
            }
            Some(Arc::new(lp))
        }
        (Some(p), false) => {
            return Err(LowerError::Aggregation(format!(
                "{:?} does not accept a parameter; got {p}",
                op
            )));
        }
        (None, true) => {
            return Err(LowerError::Aggregation(format!(
                "{:?} requires a scalar parameter (the k or phi)",
                op
            )));
        }
        (None, false) => None,
    };

    let grouping = a.modifier.as_ref().map(|m| match m {
        LabelModifier::Include(ls) => Grouping::By(ls.labels.clone()),
        LabelModifier::Exclude(ls) => Grouping::Without(ls.labels.clone()),
    });
    Ok(Plan::Aggregate {
        op,
        grouping,
        param: lowered_param,
        expr: Arc::new(expr),
    })
}

fn lower_call(c: &Call) -> Result<Plan, LowerError> {
    let func = FuncKind::from_name(&c.func.name).ok_or_else(|| {
        LowerError::Call(format!(
            "unknown function {} (Phase 1 supports rate/irate/increase/delta/histogram_quantile and the *_over_time family)",
            c.func.name
        ))
    })?;
    // Phase 1 chunk 3 defers function evaluation to chunk 4; we still
    // lower the call so type checking surfaces user errors.
    let mut args = Vec::with_capacity(c.args.args.len());
    for a in &c.args.args {
        args.push(lower(a)?);
    }
    Ok(Plan::Call { func, args })
}

fn lower_matchers(
    name: Option<&str>,
    matchers: &[ParserMatcher],
) -> Result<Vec<Matcher>, LowerError> {
    let mut out = Vec::with_capacity(matchers.len() + name.is_some() as usize);
    if let Some(n) = name {
        out.push(Matcher::eq(METRIC_NAME, n));
    }
    for m in matchers {
        out.push(lower_one_matcher(m)?);
    }
    Ok(out)
}

fn lower_one_matcher(m: &ParserMatcher) -> Result<Matcher, LowerError> {
    Ok(match &m.op {
        ParserMatchOp::Equal => Matcher::eq(m.name.clone(), m.value.clone()),
        ParserMatchOp::NotEqual => Matcher::ne(m.name.clone(), m.value.clone()),
        ParserMatchOp::Re(r) => {
            // Round-trip through our regex cache so the storage layer and
            // the lowering layer share compiled patterns.
            let pattern = r.as_str().to_string();
            // Validate against our regex engine by compiling once.
            compile_regex(&pattern).map_err(|e| {
                LowerError::InvalidMatcher(format!("regex {pattern}: {e}"))
            })?;
            Matcher::re(m.name.clone(), pattern).map_err(|e| {
                LowerError::InvalidMatcher(format!("re matcher build: {e}"))
            })?
        }
        ParserMatchOp::NotRe(r) => {
            let pattern = r.as_str().to_string();
            compile_regex(&pattern).map_err(|e| {
                LowerError::InvalidMatcher(format!("regex {pattern}: {e}"))
            })?;
            Matcher::nre(m.name.clone(), pattern).map_err(|e| {
                LowerError::InvalidMatcher(format!("nre matcher build: {e}"))
            })?
        }
    })
}

fn offset_to_ms(o: Option<&Offset>) -> i64 {
    match o {
        None => 0,
        Some(Offset::Pos(d)) => duration_to_ms(*d),
        Some(Offset::Neg(d)) => -duration_to_ms(*d),
    }
}

fn duration_to_ms(d: Duration) -> i64 {
    // PromQL durations are bounded; saturating cast.
    let ms = d.as_millis();
    if ms > i64::MAX as u128 {
        i64::MAX
    } else {
        ms as i64
    }
}

fn binop_from_token(t: token::TokenId) -> Result<BinopKind, LowerError> {
    use token::*;
    Ok(match t {
        T_ADD => BinopKind::Add,
        T_SUB => BinopKind::Sub,
        T_MUL => BinopKind::Mul,
        T_DIV => BinopKind::Div,
        T_MOD => BinopKind::Mod,
        T_POW => BinopKind::Pow,
        T_EQLC => BinopKind::Eq,
        T_NEQ => BinopKind::Ne,
        T_LSS => BinopKind::Lt,
        T_LTE => BinopKind::Le,
        T_GTR => BinopKind::Gt,
        T_GTE => BinopKind::Ge,
        T_LAND => BinopKind::LAnd,
        T_LOR => BinopKind::LOr,
        T_LUNLESS => BinopKind::LUnless,
        other => {
            return Err(LowerError::Unsupported(format!(
                "binary operator token {other} is unrecognized"
            )))
        }
    })
}

fn aggr_from_token(t: token::TokenId) -> Result<AggrKind, LowerError> {
    use token::*;
    Ok(match t {
        T_SUM => AggrKind::Sum,
        T_AVG => AggrKind::Avg,
        T_MIN => AggrKind::Min,
        T_MAX => AggrKind::Max,
        T_COUNT => AggrKind::Count,
        T_TOPK => AggrKind::TopK,
        T_BOTTOMK => AggrKind::BottomK,
        T_QUANTILE => AggrKind::Quantile,
        T_COUNT_VALUES => {
            return Err(LowerError::Unsupported(
                "count_values is deferred to a follow-up SOW".to_string(),
            ))
        }
        other => {
            return Err(LowerError::Unsupported(format!(
                "aggregation operator token {other} is unrecognized"
            )))
        }
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn lower_ok(q: &str) -> Plan {
        lower_query(q).unwrap_or_else(|e| panic!("lower failed for {q}: {e}"))
    }

    fn lower_err(q: &str) -> LowerError {
        lower_query(q).err().unwrap_or_else(|| panic!("expected error for {q}"))
    }

    #[test]
    fn number_lowers_to_scalar() {
        let p = lower_ok("42");
        assert!(matches!(p, Plan::Number(v) if (v - 42.0).abs() < 1e-9));
        assert_eq!(p.value_type(), ValueType::Scalar);
    }

    #[test]
    fn vector_selector_with_name_and_matcher() {
        let p = lower_ok("http_requests_total{method=\"GET\"}");
        match p {
            Plan::VectorSelect { matchers, offset_ms } => {
                assert_eq!(offset_ms, 0);
                assert_eq!(matchers.len(), 2);
                assert_eq!(matchers[0].name(), "__name__");
                assert_eq!(matchers[1].name(), "method");
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn matrix_selector_carries_range() {
        let p = lower_ok("http_requests_total[5m]");
        match p {
            Plan::MatrixSelect { range_ms, .. } => {
                assert_eq!(range_ms, 5 * 60 * 1000);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn binop_lowers_arithmetic() {
        let p = lower_ok("foo + bar");
        match p {
            Plan::Binop { op, .. } => assert_eq!(op, BinopKind::Add),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn binop_lowers_comparison() {
        let p = lower_ok("foo > 5");
        match p {
            Plan::Binop { op, return_bool, .. } => {
                assert_eq!(op, BinopKind::Gt);
                assert!(!return_bool);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn aggregate_with_by() {
        let p = lower_ok("sum by (method) (http_requests_total)");
        match p {
            Plan::Aggregate { op, grouping, .. } => {
                assert_eq!(op, AggrKind::Sum);
                assert!(matches!(grouping, Some(Grouping::By(ls)) if ls == vec!["method"]));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn aggregate_with_without() {
        let p = lower_ok("avg without (instance) (foo)");
        match p {
            Plan::Aggregate { op, grouping, .. } => {
                assert_eq!(op, AggrKind::Avg);
                assert!(matches!(grouping, Some(Grouping::Without(ls)) if ls == vec!["instance"]));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn paren_is_transparent() {
        let p = lower_ok("(foo)");
        assert!(matches!(p, Plan::VectorSelect { .. }));
    }

    #[test]
    fn subquery_is_phase_2() {
        match lower_err("foo[5m:1m]") {
            LowerError::Unsupported(msg) => assert!(msg.contains("subquer")),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn set_operator_lowers_to_binop() {
        // SOW-0022 (Phase 3d): set operators are supported. The
        // lowering preserves the operator kind and attaches a default
        // matching spec.
        let plan = lower_query("foo and bar").unwrap();
        match plan {
            Plan::Binop {
                op,
                matching,
                ..
            } => {
                assert_eq!(op, BinopKind::LAnd);
                let m = matching.expect("set op carries a matching spec");
                assert_eq!(m.cardinality, Cardinality::OneToOne);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    // Note: `foo and on(x) group_left(y) bar` is rejected by the
    // promql-parser before it reaches lowering ("no grouping allowed for
    // 'and' operation"). The lowering's protection against group_left on
    // a set operator is therefore unreachable through normal parsing,
    // but kept as a defense-in-depth check.

    #[test]
    fn rate_lowers_to_call() {
        let p = lower_ok("rate(http_requests_total[5m])");
        match p {
            Plan::Call { func, args } => {
                assert_eq!(func, FuncKind::Rate);
                assert_eq!(args.len(), 1);
                assert!(matches!(args[0], Plan::MatrixSelect { .. }));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }
}
