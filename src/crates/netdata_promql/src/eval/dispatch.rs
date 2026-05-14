// SPDX-License-Identifier: GPL-3.0-or-later
//
// Top-level evaluator: dispatches one Plan node against an EvalContext.

use crate::plan::{FuncKind, Plan};

use super::context::EvalContext;
use super::select::{eval_matrix_select, eval_vector_select};
use super::subquery::eval_subquery;
use super::types::{labels_signature, EvalError, EvalResult, Sample, Series};

/// Evaluate one plan node at `ctx.at_ms`.
pub fn eval(ctx: &EvalContext, plan: &Plan) -> Result<EvalResult, EvalError> {
    match plan {
        Plan::Number(v) => Ok(EvalResult::Scalar(*v)),

        Plan::VectorSelect {
            matchers,
            offset_ms,
            at,
        } => eval_vector_select(ctx, matchers, *offset_ms, at.as_ref()),

        Plan::MatrixSelect {
            matchers,
            range_ms,
            offset_ms,
            at,
        } => eval_matrix_select(ctx, matchers, *range_ms, *offset_ms, at.as_ref()),

        Plan::UnaryMinus(inner) => {
            let r = eval(ctx, inner)?;
            Ok(super::unary::negate(r))
        }

        Plan::Binop {
            op,
            return_bool,
            matching,
            lhs,
            rhs,
        } => {
            let l = eval(ctx, lhs)?;
            let r = eval(ctx, rhs)?;
            super::binop::apply_binop(*op, *return_bool, matching.as_ref(), l, r)
        }

        Plan::Aggregate {
            op,
            grouping,
            param,
            param_string,
            expr,
        } => {
            // Parametrized aggregators (topk/bottomk/quantile) take a
            // scalar param. Evaluate it before the inner vector so we
            // can fail fast on a malformed parameter expression.
            // `count_values` takes a string param that already lived
            // in the Plan IR -- no evaluation step needed.
            let param_value = match param {
                Some(p) => {
                    let r = eval(ctx, p)?;
                    match r {
                        super::types::EvalResult::Scalar(v) => Some(v),
                        other => {
                            return Err(super::types::EvalError::Type {
                                context: "aggregation parameter",
                                expected: crate::plan::ValueType::Scalar,
                                got: other.value_type(),
                            });
                        }
                    }
                }
                None => None,
            };
            let inner = eval(ctx, expr)?;
            super::aggregation::apply_aggregate(
                *op,
                grouping.as_ref(),
                param_value,
                param_string.as_deref(),
                inner,
            )
        }

        Plan::Call { func, args } => {
            // `time()` and `vector(s)` need the eval-context timestamp to
            // produce their output; the rest go through the
            // context-independent dispatcher in `functions::apply_call`.
            // Keeping the special cases here avoids threading `ctx`
            // through every per-sample helper.
            match func {
                FuncKind::Time => {
                    if !args.is_empty() {
                        return Err(EvalError::Other(
                            "time() expects no arguments".to_string(),
                        ));
                    }
                    return Ok(EvalResult::Scalar(ctx.at_ms as f64 / 1000.0));
                }
                FuncKind::Vector => {
                    if args.len() != 1 {
                        return Err(EvalError::Other(
                            "vector expects exactly 1 argument".to_string(),
                        ));
                    }
                    let v = match eval(ctx, &args[0])? {
                        EvalResult::Scalar(v) => v,
                        other => {
                            return Err(EvalError::Type {
                                context: "vector",
                                expected: crate::plan::ValueType::Scalar,
                                got: other.value_type(),
                            })
                        }
                    };
                    let series = Series {
                        labels: Vec::new(),
                        signature: labels_signature(&[]),
                        samples: vec![Sample {
                            timestamp_ms: ctx.at_ms,
                            value: v,
                        }],
                    };
                    return Ok(EvalResult::InstantVector(vec![series]));
                }
                _ => {}
            }
            let mut evaled = Vec::with_capacity(args.len());
            for a in args {
                evaled.push(eval(ctx, a)?);
            }
            super::functions::apply_call(*func, evaled)
        }

        Plan::Subquery {
            expr,
            range_ms,
            step_ms,
            offset_ms,
            at,
        } => eval_subquery(ctx, expr, *range_ms, *step_ms, *offset_ms, at.as_ref()),

        Plan::LabelOp { op, expr } => {
            let inner = eval(ctx, expr)?;
            super::labelops::apply_label_op(op, inner)
        }

        Plan::Absent { labels, expr } => {
            let inner = eval(ctx, expr)?;
            super::absent::eval_absent(ctx, labels, inner)
        }
    }
}
