// SPDX-License-Identifier: GPL-3.0-or-later
//
// Top-level evaluator: dispatches one Plan node against an EvalContext.

use crate::plan::Plan;

use super::context::EvalContext;
use super::select::{eval_matrix_select, eval_vector_select};
use super::types::{EvalError, EvalResult};

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
            let mut evaled = Vec::with_capacity(args.len());
            for a in args {
                evaled.push(eval(ctx, a)?);
            }
            super::functions::apply_call(*func, evaled)
        }
    }
}
