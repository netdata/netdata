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
        } => eval_vector_select(ctx, matchers, *offset_ms),

        Plan::MatrixSelect {
            matchers,
            range_ms,
            offset_ms,
        } => eval_matrix_select(ctx, matchers, *range_ms, *offset_ms),

        Plan::UnaryMinus(inner) => {
            let r = eval(ctx, inner)?;
            Ok(super::unary::negate(r))
        }

        Plan::Binop {
            op,
            return_bool,
            lhs,
            rhs,
        } => {
            let l = eval(ctx, lhs)?;
            let r = eval(ctx, rhs)?;
            super::binop::apply_binop(*op, *return_bool, l, r)
        }

        Plan::Aggregate {
            op,
            grouping,
            expr,
            ..
        } => {
            let inner = eval(ctx, expr)?;
            super::aggregation::apply_aggregate(*op, grouping.as_ref(), inner)
        }

        Plan::Call { func, .. } => Err(EvalError::NotYetImplemented(format!(
            "function {:?} -- chunk 4 of SOW-0017",
            func
        ))),
    }
}
