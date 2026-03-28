use super::context::EvalContext;
use crate::enrichment::ValueExpr;
use anyhow::Result;

pub(super) fn eval_equals(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value() == right.to_string_value())
}

pub(super) fn eval_not_equals(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value() != right.to_string_value())
}

pub(super) fn eval_greater(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    eval_numeric_compare(context, left, right, ">", |left_num, right_num| {
        left_num > right_num
    })
}

pub(super) fn eval_greater_or_equal(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    eval_numeric_compare(context, left, right, ">=", |left_num, right_num| {
        left_num >= right_num
    })
}

pub(super) fn eval_less(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    eval_numeric_compare(context, left, right, "<", |left_num, right_num| {
        left_num < right_num
    })
}

pub(super) fn eval_less_or_equal(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    eval_numeric_compare(context, left, right, "<=", |left_num, right_num| {
        left_num <= right_num
    })
}

pub(super) fn eval_in(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    let left = left.to_string_value();
    let members = right
        .as_list()
        .ok_or_else(|| anyhow::anyhow!("right operand is not a list for 'in'"))?;
    Ok(members
        .iter()
        .any(|candidate| candidate.to_string_value() == left))
}

fn eval_numeric_compare<F>(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
    operator: &str,
    predicate: F,
) -> Result<bool>
where
    F: FnOnce(i64, i64) -> bool,
{
    let (left, right) = context.resolve_binary(left, right)?;
    let left_num = left
        .to_i64()
        .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '{operator}'"))?;
    let right_num = right
        .to_i64()
        .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '{operator}'"))?;
    Ok(predicate(left_num, right_num))
}
