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

use promql_parser::label::{METRIC_NAME, MatchOp as ParserMatchOp, Matcher as ParserMatcher};
use promql_parser::parser::token;
use promql_parser::parser::{
    AggregateExpr, BinaryExpr, Call, Expr, LabelModifier, MatrixSelector, Offset, ParenExpr,
    SubqueryExpr, UnaryExpr, VectorSelector,
};

use crate::storage::{Matcher, compile_regex};

use super::ir::{
    AggrKind, AtMod, BinopKind, Cardinality, FuncKind, FusedSource, Grouping, MatchKeys, MatchSpec,
    Plan, ValueType,
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
        Expr::Subquery(sq) => lower_subquery(sq),
        Expr::StringLiteral(_) => Err(LowerError::Unsupported(
            "top-level string literals are not supported".to_string(),
        )),
        Expr::Extension(_) => Err(LowerError::Unsupported(
            "AST extensions are not supported".to_string(),
        )),
    }
}

fn lower_vector_select(vs: &VectorSelector) -> Result<Plan, LowerError> {
    let matchers = lower_matchers(vs.name.as_deref(), &vs.matchers.matchers)?;
    let offset_ms = offset_to_ms(vs.offset.as_ref());
    let at = lower_at(vs.at.as_ref())?;
    Ok(Plan::VectorSelect {
        matchers,
        offset_ms,
        at,
    })
}

fn lower_matrix_select(vs: &VectorSelector, range: Duration) -> Result<Plan, LowerError> {
    let matchers = lower_matchers(vs.name.as_deref(), &vs.matchers.matchers)?;
    let offset_ms = offset_to_ms(vs.offset.as_ref());
    let range_ms = duration_to_ms(range);
    if range_ms <= 0 {
        return Err(LowerError::InvalidMatcher(
            "matrix selector range must be positive".to_string(),
        ));
    }
    let at = lower_at(vs.at.as_ref())?;
    Ok(Plan::MatrixSelect {
        matchers,
        range_ms,
        offset_ms,
        at,
    })
}

/// Translate the parser's `@` modifier into our `AtMod`. `At(SystemTime)`
/// resolves to a Unix-millisecond timestamp; the special `start()` /
/// `end()` forms carry through to the evaluator which resolves them
/// against the outer query range.
fn lower_at(at: Option<&promql_parser::parser::AtModifier>) -> Result<Option<AtMod>, LowerError> {
    use promql_parser::parser::AtModifier as P;
    Ok(match at {
        None => None,
        Some(P::Start) => Some(AtMod::Start),
        Some(P::End) => Some(AtMod::End),
        Some(P::At(time)) => {
            // SystemTime -> Unix ms. The parser allows timestamps that
            // pre-date UNIX_EPOCH; we encode them as negative i64
            // milliseconds.
            let dur = time.duration_since(std::time::UNIX_EPOCH);
            let ms = match dur {
                Ok(d) => i64::try_from(d.as_millis()).unwrap_or(i64::MAX),
                Err(neg) => {
                    let back = neg.duration();
                    -(i64::try_from(back.as_millis()).unwrap_or(i64::MAX))
                }
            };
            Some(AtMod::AtTs(ms))
        }
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
    if op.is_set_op() && (lhs.value_type() != InstantVector || rhs.value_type() != InstantVector) {
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
    } else if matches!(lhs.value_type(), InstantVector) || matches!(rhs.value_type(), InstantVector)
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

    // Three flavors of param:
    //   - count_values requires a string param (the bucket label name).
    //   - topk/bottomk/quantile require a scalar param (k or phi).
    //   - everything else rejects any param.
    let mut lowered_param: Option<Arc<Plan>> = None;
    let mut lowered_param_string: Option<String> = None;

    match (&a.param, op.takes_param(), op.takes_string_param()) {
        // String-param aggregator with a StringLiteral param: accept.
        (Some(p), false, true) => match p.as_ref() {
            Expr::StringLiteral(s) => {
                lowered_param_string = Some(s.val.clone());
            }
            other => {
                return Err(LowerError::Aggregation(format!(
                    "{:?} requires a string parameter (the bucket label name); got {:?}",
                    op, other
                )));
            }
        },
        // Scalar-param aggregator: lower as a Plan.
        (Some(p), true, false) => match p.as_ref() {
            Expr::StringLiteral(_) => {
                return Err(LowerError::Aggregation(format!(
                    "{:?} requires a scalar parameter (the k or phi); got a string",
                    op
                )));
            }
            _ => {
                let lp = lower(p)?;
                if lp.value_type() != ValueType::Scalar {
                    return Err(LowerError::Aggregation(format!(
                        "{:?} parameter must be a scalar; got {:?}",
                        op,
                        lp.value_type()
                    )));
                }
                lowered_param = Some(Arc::new(lp));
            }
        },
        // Non-parametrized aggregator with a param: reject.
        (Some(p), false, false) => {
            return Err(LowerError::Aggregation(format!(
                "{:?} does not accept a parameter; got {p}",
                op
            )));
        }
        // Parametrized aggregator without a param: reject.
        (None, true, false) => {
            return Err(LowerError::Aggregation(format!(
                "{:?} requires a scalar parameter (the k or phi)",
                op
            )));
        }
        (None, false, true) => {
            return Err(LowerError::Aggregation(format!(
                "{:?} requires a string parameter (the bucket label name)",
                op
            )));
        }
        (None, false, false) => {}
        // The two predicates are mutually exclusive by construction.
        (_, true, true) => unreachable!(),
    }

    let grouping = a.modifier.as_ref().map(|m| match m {
        LabelModifier::Include(ls) => Grouping::By(ls.labels.clone()),
        LabelModifier::Exclude(ls) => Grouping::Without(ls.labels.clone()),
    });

    // Try to fuse `aggr(rollup(matrix|subquery))` into a single streaming
    // plan node. The unfused Plan::Aggregate is returned when fusion
    // doesn't apply (parametrized aggregator, non-rollup inner, etc.).
    if lowered_param.is_none() && lowered_param_string.is_none() {
        if let Some(fused) = try_fuse_aggr_rollup(op, &grouping, &expr) {
            return Ok(fused);
        }
    }

    Ok(Plan::Aggregate {
        op,
        grouping,
        param: lowered_param,
        param_string: lowered_param_string,
        expr: Arc::new(expr),
    })
}

/// Detect `aggr(rollup(matrix|subquery))` and rewrite to
/// [`Plan::FusedAggrRollup`] when both the aggregator and the rollup
/// are on the supported list. Returns `None` (keeping the unfused
/// [`Plan::Aggregate`]) for any other shape.
fn try_fuse_aggr_rollup(aggr: AggrKind, grouping: &Option<Grouping>, expr: &Plan) -> Option<Plan> {
    // Aggregator must be one of sum/avg/min/max/count.
    match aggr {
        AggrKind::Sum | AggrKind::Avg | AggrKind::Min | AggrKind::Max | AggrKind::Count => {}
        _ => return None,
    }
    // Inner must be a Call into a fusable rollup, with a single
    // argument that is itself a matrix selector or subquery.
    let Plan::Call { func, args } = expr else {
        return None;
    };
    if args.len() != 1 {
        return None;
    }
    // The rollup must be on the fusion-friendly list.
    match func {
        FuncKind::Rate
        | FuncKind::Increase
        | FuncKind::Delta
        | FuncKind::IRate
        | FuncKind::AvgOverTime
        | FuncKind::SumOverTime
        | FuncKind::MinOverTime
        | FuncKind::MaxOverTime
        | FuncKind::CountOverTime
        | FuncKind::LastOverTime
        | FuncKind::PresentOverTime
        | FuncKind::StddevOverTime
        | FuncKind::StdvarOverTime => {}
        _ => return None,
    }
    let source = match &args[0] {
        Plan::MatrixSelect {
            matchers,
            range_ms,
            offset_ms,
            at,
        } => FusedSource::Matrix {
            matchers: matchers.clone(),
            range_ms: *range_ms,
            offset_ms: *offset_ms,
            at: at.clone(),
        },
        Plan::Subquery {
            expr,
            range_ms,
            step_ms,
            offset_ms,
            at,
        } => FusedSource::Subquery {
            expr: Arc::clone(expr),
            range_ms: *range_ms,
            step_ms: *step_ms,
            offset_ms: *offset_ms,
            at: at.clone(),
        },
        _ => return None,
    };
    Some(Plan::FusedAggrRollup {
        aggr,
        grouping: grouping.clone(),
        rollup: *func,
        source,
    })
}

/// Default step when the parser leaves `step` as `None`. Prometheus uses
/// the global evaluation interval; we adopt 1 second (the minimum
/// sensible value).
const DEFAULT_SUBQUERY_STEP_MS: i64 = 1000;

fn lower_subquery(sq: &SubqueryExpr) -> Result<Plan, LowerError> {
    let inner = lower(&sq.expr)?;
    if inner.value_type() != ValueType::InstantVector {
        return Err(LowerError::Type {
            context: "subquery inner expression",
            expected: ValueType::InstantVector,
            got: inner.value_type(),
        });
    }
    let range_ms = duration_to_ms(sq.range);
    if range_ms <= 0 {
        return Err(LowerError::InvalidMatcher(
            "subquery range must be positive".to_string(),
        ));
    }
    let step_ms = match sq.step {
        Some(d) => {
            let ms = duration_to_ms(d);
            if ms <= 0 {
                return Err(LowerError::InvalidMatcher(
                    "subquery step must be positive".to_string(),
                ));
            }
            ms
        }
        None => DEFAULT_SUBQUERY_STEP_MS,
    };
    // Reuse Prometheus' per-timeseries point cap. `range_ms / step_ms`
    // gives the count of grid points minus 1 (the inclusive endpoint),
    // but for the cap check the +1 is irrelevant -- we only want to
    // reject pathological step values, and the existing range-query
    // path uses the same "no greater than 11000" rule.
    let points = (range_ms / step_ms) as usize + 1;
    if points > crate::MAX_POINTS_PER_TIMESERIES {
        return Err(LowerError::InvalidMatcher(format!(
            "subquery exceeds maximum resolution of {} points per timeseries \
             ({}ms / {}ms = {} points)",
            crate::MAX_POINTS_PER_TIMESERIES,
            range_ms,
            step_ms,
            points,
        )));
    }
    let offset_ms = offset_to_ms(sq.offset.as_ref());
    let at = lower_at(sq.at.as_ref())?;
    Ok(Plan::Subquery {
        expr: Arc::new(inner),
        range_ms,
        step_ms,
        offset_ms,
        at,
    })
}

fn lower_call(c: &Call) -> Result<Plan, LowerError> {
    // label_replace / label_join take string-literal arguments that
    // don't lower to Plan nodes. Intercept here and produce a
    // Plan::LabelOp with the strings pre-extracted.
    if c.func.name == "label_replace" {
        return lower_label_replace(c);
    }
    if c.func.name == "label_join" {
        return lower_label_join(c);
    }
    // absent / absent_over_time need the static matcher labels
    // extracted at lowering so the evaluator can synthesise them
    // when the inner returns empty.
    if c.func.name == "absent" || c.func.name == "absent_over_time" {
        return lower_absent(c);
    }

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

/// Extract a parser-level `StringLiteral` from an `Expr`. Returns
/// `None` for any other expression shape.
fn as_string_literal(e: &Expr) -> Option<&str> {
    match e {
        Expr::StringLiteral(s) => Some(&s.val),
        _ => None,
    }
}

fn lower_label_replace(c: &Call) -> Result<Plan, LowerError> {
    if c.args.args.len() != 5 {
        return Err(LowerError::Call(format!(
            "label_replace requires 5 arguments (vector, dst, replacement, src, regex); got {}",
            c.args.args.len()
        )));
    }
    let inner = lower(&c.args.args[0])?;
    if inner.value_type() != ValueType::InstantVector {
        return Err(LowerError::Type {
            context: "label_replace vector argument",
            expected: ValueType::InstantVector,
            got: inner.value_type(),
        });
    }
    let dst = as_string_literal(&c.args.args[1])
        .ok_or_else(|| LowerError::Call("label_replace dst must be a string literal".into()))?
        .to_string();
    let replacement = as_string_literal(&c.args.args[2])
        .ok_or_else(|| {
            LowerError::Call("label_replace replacement must be a string literal".into())
        })?
        .to_string();
    let src = as_string_literal(&c.args.args[3])
        .ok_or_else(|| LowerError::Call("label_replace src must be a string literal".into()))?
        .to_string();
    let regex = as_string_literal(&c.args.args[4])
        .ok_or_else(|| LowerError::Call("label_replace regex must be a string literal".into()))?
        .to_string();
    // Validate the regex now so failures surface at parse time, not at
    // every step of a range query.
    compile_regex(&regex)
        .map_err(|e| LowerError::InvalidMatcher(format!("label_replace regex {regex}: {e}")))?;
    Ok(Plan::LabelOp {
        op: crate::plan::LabelOpKind::Replace {
            dst,
            replacement,
            src,
            regex,
        },
        expr: Arc::new(inner),
    })
}

/// Extract the static `=` matchers from an inner Plan to seed
/// `absent()`'s synthetic output labels. Only equality matchers
/// against non-`__name__` labels contribute; everything else (regex,
/// inequality, complex expressions, the metric name itself) is
/// ignored. Returns an empty vec when the inner is anything other
/// than a vector/matrix selector.
fn extract_absent_labels(p: &Plan) -> Vec<(String, String)> {
    use crate::storage::Matcher;
    let matchers = match p {
        Plan::VectorSelect { matchers, .. } => matchers,
        Plan::MatrixSelect { matchers, .. } => matchers,
        _ => return Vec::new(),
    };
    let mut out: Vec<(String, String)> = Vec::new();
    for m in matchers {
        // Only equality matchers contribute. `__name__` is kept so
        // `absent(metric{job="x"})` returns
        // `{__name__="metric", job="x"}` per Prometheus convention.
        if let Matcher::Eq { name, value } = m {
            out.push((name.clone(), value.clone()));
        }
    }
    out.sort_by(|a, b| a.0.cmp(&b.0));
    out
}

fn lower_absent(c: &Call) -> Result<Plan, LowerError> {
    if c.args.args.len() != 1 {
        return Err(LowerError::Call(format!(
            "{} requires exactly 1 argument",
            c.func.name
        )));
    }
    let inner = lower(&c.args.args[0])?;
    // absent expects an instant vector; absent_over_time expects a
    // range vector.
    let expected = if c.func.name == "absent_over_time" {
        ValueType::RangeVector
    } else {
        ValueType::InstantVector
    };
    if inner.value_type() != expected {
        return Err(LowerError::Type {
            context: "absent argument",
            expected,
            got: inner.value_type(),
        });
    }
    let labels = extract_absent_labels(&inner);
    Ok(Plan::Absent {
        labels,
        expr: Arc::new(inner),
    })
}

fn lower_label_join(c: &Call) -> Result<Plan, LowerError> {
    if c.args.args.len() < 3 {
        return Err(LowerError::Call(format!(
            "label_join requires at least 3 arguments (vector, dst, sep, srcs...); got {}",
            c.args.args.len()
        )));
    }
    let inner = lower(&c.args.args[0])?;
    if inner.value_type() != ValueType::InstantVector {
        return Err(LowerError::Type {
            context: "label_join vector argument",
            expected: ValueType::InstantVector,
            got: inner.value_type(),
        });
    }
    let dst = as_string_literal(&c.args.args[1])
        .ok_or_else(|| LowerError::Call("label_join dst must be a string literal".into()))?
        .to_string();
    let sep = as_string_literal(&c.args.args[2])
        .ok_or_else(|| LowerError::Call("label_join sep must be a string literal".into()))?
        .to_string();
    let mut srcs = Vec::with_capacity(c.args.args.len() - 3);
    for (i, a) in c.args.args.iter().enumerate().skip(3) {
        let s = as_string_literal(a).ok_or_else(|| {
            LowerError::Call(format!(
                "label_join source argument {} must be a string literal",
                i - 2
            ))
        })?;
        srcs.push(s.to_string());
    }
    Ok(Plan::LabelOp {
        op: crate::plan::LabelOpKind::Join { dst, sep, srcs },
        expr: Arc::new(inner),
    })
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
            compile_regex(&pattern)
                .map_err(|e| LowerError::InvalidMatcher(format!("regex {pattern}: {e}")))?;
            Matcher::re(m.name.clone(), pattern)
                .map_err(|e| LowerError::InvalidMatcher(format!("re matcher build: {e}")))?
        }
        ParserMatchOp::NotRe(r) => {
            let pattern = r.as_str().to_string();
            compile_regex(&pattern)
                .map_err(|e| LowerError::InvalidMatcher(format!("regex {pattern}: {e}")))?;
            Matcher::nre(m.name.clone(), pattern)
                .map_err(|e| LowerError::InvalidMatcher(format!("nre matcher build: {e}")))?
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
            )));
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
        T_COUNT_VALUES => AggrKind::CountValues,
        T_STDDEV => AggrKind::Stddev,
        T_STDVAR => AggrKind::Stdvar,
        T_LIMITK => AggrKind::LimitK,
        T_LIMIT_RATIO => AggrKind::LimitRatio,
        T_GROUP => AggrKind::Group,
        other => {
            return Err(LowerError::Unsupported(format!(
                "aggregation operator token {other} is unrecognized"
            )));
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
        lower_query(q)
            .err()
            .unwrap_or_else(|| panic!("expected error for {q}"))
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
            Plan::VectorSelect {
                matchers,
                offset_ms,
                ..
            } => {
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
            Plan::Binop {
                op, return_bool, ..
            } => {
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
    fn subquery_lowers_to_range_vector() {
        let p = lower_ok("foo[5m:1m]");
        match p {
            Plan::Subquery {
                range_ms,
                step_ms,
                offset_ms,
                at,
                ref expr,
            } => {
                assert_eq!(range_ms, 5 * 60 * 1000);
                assert_eq!(step_ms, 60 * 1000);
                assert_eq!(offset_ms, 0);
                assert_eq!(at, None);
                assert!(matches!(**expr, Plan::VectorSelect { .. }));
            }
            other => panic!("unexpected: {other:?}"),
        }
        assert_eq!(p.value_type(), ValueType::RangeVector);
    }

    #[test]
    fn subquery_defaults_step_to_one_second() {
        // The parser leaves `step` as None when omitted; we substitute
        // 1000 ms (the minimum sensible value).
        let p = lower_ok("foo[1m:]");
        match p {
            Plan::Subquery { step_ms, .. } => assert_eq!(step_ms, 1000),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_rejects_range_vector_inner() {
        // `foo[5m]` already lowers to a range vector; wrapping it in
        // a subquery would compose a range over a range, which
        // Prometheus also rejects. promql-parser catches this before
        // it reaches the lowering layer (Parse error from the AST
        // builder); the in-lowering type check is defense in depth
        // for any future caller that constructs a Plan::Subquery
        // directly.
        match lower_err("(foo[5m])[10m:1m]") {
            LowerError::Parse(msg) => {
                assert!(msg.contains("vector"), "msg = {msg}");
            }
            LowerError::Type {
                context,
                expected,
                got,
            } => {
                assert_eq!(context, "subquery inner expression");
                assert_eq!(expected, ValueType::InstantVector);
                assert_eq!(got, ValueType::RangeVector);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_rejects_oversized_point_count() {
        // 1m / 1ms = 60_001 points > 11_000 cap.
        match lower_err("foo[1m:1ms]") {
            LowerError::InvalidMatcher(msg) => {
                assert!(
                    msg.contains("11000 points") || msg.contains("11000"),
                    "msg = {msg}"
                );
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_carries_at_modifier() {
        let p = lower_ok("foo[5m:1m] @ 1234");
        match p {
            Plan::Subquery { at, .. } => assert_eq!(at, Some(AtMod::AtTs(1_234_000))),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_at_start_and_end_lower() {
        match lower_ok("foo[5m:1m] @ start()") {
            Plan::Subquery { at, .. } => assert_eq!(at, Some(AtMod::Start)),
            other => panic!("unexpected: {other:?}"),
        }
        match lower_ok("foo[5m:1m] @ end()") {
            Plan::Subquery { at, .. } => assert_eq!(at, Some(AtMod::End)),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_over_rate_lowers() {
        // `rate(metric[5m])[10m:1m]` -- the canonical alert-rule
        // shape. The inner is a Call (instant vector); the outer is
        // the subquery.
        let p = lower_ok("rate(metric[5m])[10m:1m]");
        match p {
            Plan::Subquery {
                expr,
                range_ms,
                step_ms,
                ..
            } => {
                assert_eq!(range_ms, 10 * 60 * 1000);
                assert_eq!(step_ms, 60 * 1000);
                match &*expr {
                    Plan::Call { func, .. } => assert_eq!(*func, FuncKind::Rate),
                    other => panic!("unexpected inner: {other:?}"),
                }
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn max_over_time_consumes_subquery() {
        // `max_over_time(sum(metric)[5m:1m])` -- the subquery
        // produces a range vector that `max_over_time` consumes.
        let p = lower_ok("max_over_time(sum(metric)[5m:1m])");
        match p {
            Plan::Call { func, args } => {
                assert_eq!(func, FuncKind::MaxOverTime);
                assert_eq!(args.len(), 1);
                match &args[0] {
                    Plan::Subquery {
                        range_ms,
                        step_ms,
                        expr,
                        ..
                    } => {
                        assert_eq!(*range_ms, 5 * 60 * 1000);
                        assert_eq!(*step_ms, 60 * 1000);
                        assert!(matches!(**expr, Plan::Aggregate { .. }));
                    }
                    other => panic!("unexpected arg: {other:?}"),
                }
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn subquery_carries_offset() {
        let p = lower_ok("foo[5m:1m] offset 10m");
        match p {
            Plan::Subquery { offset_ms, .. } => assert_eq!(offset_ms, 10 * 60 * 1000),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn at_modifier_lowers_to_atts() {
        let p = lower_ok("foo @ 1234.5");
        match p {
            Plan::VectorSelect { at, .. } => {
                assert_eq!(at, Some(AtMod::AtTs(1_234_500)));
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn at_start_and_end_lower() {
        let p_start = lower_ok("foo @ start()");
        match p_start {
            Plan::VectorSelect { at, .. } => assert_eq!(at, Some(AtMod::Start)),
            other => panic!("unexpected: {other:?}"),
        }
        let p_end = lower_ok("foo @ end()");
        match p_end {
            Plan::VectorSelect { at, .. } => assert_eq!(at, Some(AtMod::End)),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn at_on_matrix_selector_lowers() {
        let p = lower_ok("foo[5m] @ 100");
        // The matrix selector wraps a vector; the @ rides with the
        // matrix variant via its inner vs.
        match p {
            Plan::MatrixSelect { at, range_ms, .. } => {
                assert_eq!(at, Some(AtMod::AtTs(100_000)));
                assert_eq!(range_ms, 5 * 60 * 1000);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn set_operator_lowers_to_binop() {
        let plan = lower_query("foo and bar").unwrap();
        match plan {
            Plan::Binop { op, matching, .. } => {
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

    // Function lowering tests ------------------------------------------------

    #[test]
    fn group_a_math_functions_lower() {
        for name in [
            "abs", "ceil", "floor", "sgn", "ln", "log2", "log10", "exp", "sqrt",
        ] {
            let p = lower_ok(&format!("{name}(foo)"));
            match p {
                Plan::Call { func, args } => {
                    assert_eq!(FuncKind::from_name(name).unwrap(), func);
                    assert_eq!(args.len(), 1);
                    assert!(matches!(args[0], Plan::VectorSelect { .. }));
                }
                other => panic!("unexpected for {name}: {other:?}"),
            }
        }
    }

    #[test]
    fn clamp_family_lowers() {
        let p = lower_ok("clamp(foo, 1, 10)");
        match p {
            Plan::Call { func, args } => {
                assert_eq!(func, FuncKind::Clamp);
                assert_eq!(args.len(), 3);
            }
            other => panic!("unexpected: {other:?}"),
        }
        for name in ["clamp_min", "clamp_max"] {
            let p = lower_ok(&format!("{name}(foo, 5)"));
            match p {
                Plan::Call { args, .. } => assert_eq!(args.len(), 2),
                other => panic!("unexpected for {name}: {other:?}"),
            }
        }
    }

    #[test]
    fn round_with_and_without_to_nearest() {
        let p = lower_ok("round(foo)");
        match p {
            Plan::Call { func, args } => {
                assert_eq!(func, FuncKind::Round);
                assert_eq!(args.len(), 1);
            }
            other => panic!("unexpected: {other:?}"),
        }
        let p = lower_ok("round(foo, 0.5)");
        match p {
            Plan::Call { args, .. } => assert_eq!(args.len(), 2),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn vector_and_scalar_lower() {
        let p = lower_ok("vector(42)");
        match p {
            Plan::Call { func, .. } => assert_eq!(func, FuncKind::Vector),
            other => panic!("unexpected: {other:?}"),
        }
        let p = lower_ok("scalar(foo)");
        match p {
            Plan::Call { func, .. } => assert_eq!(func, FuncKind::Scalar),
            other => panic!("unexpected: {other:?}"),
        }
        // scalar() returns Scalar, not InstantVector.
        assert_eq!(lower_ok("scalar(foo)").value_type(), ValueType::Scalar);
        // time() also returns Scalar.
        assert_eq!(lower_ok("time()").value_type(), ValueType::Scalar);
    }

    #[test]
    fn sort_and_sort_desc_lower() {
        for name in ["sort", "sort_desc"] {
            let p = lower_ok(&format!("{name}(foo)"));
            match p {
                Plan::Call { func, .. } => {
                    assert_eq!(FuncKind::from_name(name).unwrap(), func);
                }
                other => panic!("unexpected for {name}: {other:?}"),
            }
        }
    }

    #[test]
    fn range_reductions_lower() {
        for name in ["deriv", "idelta", "changes", "resets"] {
            let p = lower_ok(&format!("{name}(foo[1m])"));
            match p {
                Plan::Call { func, args } => {
                    assert_eq!(FuncKind::from_name(name).unwrap(), func);
                    assert!(matches!(args[0], Plan::MatrixSelect { .. }));
                }
                other => panic!("unexpected for {name}: {other:?}"),
            }
        }
    }

    #[test]
    fn label_replace_lowers_to_label_op() {
        let p = lower_ok(r#"label_replace(foo, "dst", "$1", "src", "abc(\\d+)")"#);
        match p {
            Plan::LabelOp { op, .. } => match op {
                crate::plan::LabelOpKind::Replace {
                    dst,
                    replacement,
                    src,
                    regex,
                } => {
                    assert_eq!(dst, "dst");
                    assert_eq!(replacement, "$1");
                    assert_eq!(src, "src");
                    assert!(regex.contains("abc"));
                }
                other => panic!("unexpected op: {other:?}"),
            },
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn label_replace_rejects_bad_regex() {
        match lower_err(r#"label_replace(foo, "dst", "$1", "src", "(unclosed")"#) {
            LowerError::InvalidMatcher(msg) => assert!(msg.contains("label_replace")),
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn label_join_lowers() {
        let p = lower_ok(r#"label_join(foo, "joined", "-", "a", "b", "c")"#);
        match p {
            Plan::LabelOp { op, .. } => match op {
                crate::plan::LabelOpKind::Join { dst, sep, srcs } => {
                    assert_eq!(dst, "joined");
                    assert_eq!(sep, "-");
                    assert_eq!(srcs, vec!["a", "b", "c"]);
                }
                other => panic!("unexpected op: {other:?}"),
            },
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn absent_extracts_static_eq_matchers() {
        let p = lower_ok(r#"absent(metric{job="api", env="prod"})"#);
        match p {
            Plan::Absent { labels, .. } => {
                // sorted by name
                assert_eq!(
                    labels,
                    vec![
                        ("__name__".to_string(), "metric".to_string()),
                        ("env".to_string(), "prod".to_string()),
                        ("job".to_string(), "api".to_string()),
                    ]
                );
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn absent_over_complex_inner_has_empty_labels() {
        let p = lower_ok(r#"absent(rate(metric[5m]))"#);
        match p {
            Plan::Absent { labels, .. } => {
                assert!(
                    labels.is_empty(),
                    "expected empty labels for complex inner; got {:?}",
                    labels
                );
            }
            other => panic!("unexpected: {other:?}"),
        }
    }

    #[test]
    fn absent_over_time_requires_range_inner() {
        let p = lower_ok(r#"absent_over_time(metric{job="api"}[5m])"#);
        assert!(matches!(p, Plan::Absent { .. }));
        // promql-parser pre-rejects the bad type with a Parse error;
        // our in-lowering check is defense in depth.
        match lower_err("absent_over_time(foo)") {
            LowerError::Parse(msg) => {
                assert!(
                    msg.contains("matrix") || msg.contains("range"),
                    "msg = {msg}"
                );
            }
            LowerError::Type { expected, got, .. } => {
                assert_eq!(expected, ValueType::RangeVector);
                assert_eq!(got, ValueType::InstantVector);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }
}
