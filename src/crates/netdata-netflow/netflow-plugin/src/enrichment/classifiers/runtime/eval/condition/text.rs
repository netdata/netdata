use super::context::EvalContext;
use crate::enrichment::ValueExpr;
use anyhow::{Context, Result};
use regex::Regex;

pub(super) fn eval_contains(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().contains(&right.to_string_value()))
}

pub(super) fn eval_starts_with(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().starts_with(&right.to_string_value()))
}

pub(super) fn eval_ends_with(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().ends_with(&right.to_string_value()))
}

pub(super) fn eval_matches(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
    compiled: Option<&Regex>,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    let left = left.to_string_value();
    match compiled {
        Some(regex) => Ok(regex.is_match(&left)),
        None => {
            let pattern = right.to_string_value();
            let regex = Regex::new(&pattern)
                .with_context(|| format!("invalid regex '{pattern}' for 'matches'"))?;
            Ok(regex.is_match(&left))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::ConditionExpr;
    use super::*;

    #[test]
    fn eval_matches_uses_compiled_regex() {
        let regex = Regex::new("^edge-").expect("compile regex");
        let matched = ConditionExpr::Matches(
            ValueExpr::StringLiteral("edge-1".to_string()),
            ValueExpr::StringLiteral("unused".to_string()),
            Some(regex),
        )
        .eval_with_context(None, None, None, None)
        .expect("match evaluation");

        assert!(matched);
    }

    #[test]
    fn eval_matches_reports_invalid_pattern_without_compiled_regex() {
        let err = ConditionExpr::Matches(
            ValueExpr::StringLiteral("edge-1".to_string()),
            ValueExpr::StringLiteral("[".to_string()),
            None,
        )
        .eval_with_context(None, None, None, None)
        .expect_err("invalid regex should fail");

        assert!(err.to_string().contains("invalid regex '['"));
    }
}
