use super::context::EvalContext;
use crate::enrichment::ValueExpr;
use anyhow::Result;

fn values_equal(
    left: &crate::enrichment::ResolvedValue,
    right: &crate::enrichment::ResolvedValue,
) -> bool {
    match (left.as_str(), right.as_str()) {
        (Some(left), Some(right)) => left == right,
        _ => left.as_cow_str() == right.as_cow_str(),
    }
}

pub(super) fn eval_equals(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(values_equal(&left, &right))
}

pub(super) fn eval_not_equals(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(!values_equal(&left, &right))
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
    let left = left.as_cow_str();
    let members = right
        .as_list()
        .ok_or_else(|| anyhow::anyhow!("right operand is not a list for 'in'"))?;
    Ok(members
        .iter()
        .any(|candidate| candidate.as_cow_str().as_ref() == left.as_ref()))
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

#[cfg(test)]
mod tests {
    use super::super::ConditionExpr;
    use super::*;

    #[test]
    fn eval_equals_matches_string_literals() {
        let matched = ConditionExpr::Equals(
            ValueExpr::StringLiteral("edge-router".to_string()),
            ValueExpr::StringLiteral("edge-router".to_string()),
        )
        .eval_with_context(None, None, None, None)
        .expect("equals evaluation");

        assert!(matched);
    }

    #[test]
    fn eval_in_matches_string_list_members() {
        let matched = ConditionExpr::In(
            ValueExpr::StringLiteral("edge-router".to_string()),
            ValueExpr::List(vec![
                ValueExpr::StringLiteral("core-router".to_string()),
                ValueExpr::StringLiteral("edge-router".to_string()),
            ]),
        )
        .eval_with_context(None, None, None, None)
        .expect("in evaluation");

        assert!(matched);
    }
}
